package aql

// version.go — REQ-163 § The version-predicate carrier. The write side of the
// `VERSION` class expression: the sealed predicate vocabulary and the
// VERSION-fixed containment constructor that carries it.
//
// The position is classExprOperand's OTHER alternative, and its bracket is a
// DIFFERENT grammar position from the one an ordinary class carries:
//
//	classExprOperand : IDENTIFIER variable=IDENTIFIER? pathPredicate?
//	                 | VERSION variable=IDENTIFIER? ('[' versionPredicate ']')?
//	versionPredicate : LATEST_VERSION | ALL_VERSIONS | standardPredicate
//
// predicate.go § the two guards states the consequence for the READ side —
// `versionPredicate` admits no node predicate, `pathPredicate` admits no
// LATEST_VERSION — and [ValidateVersionPredicate] is the guard it shipped for
// this position. Until this file, that guard had no write-side caller at all:
// `aql.Class("VERSION", "v")` was the only VERSION shape the builder could
// emit, which is exactly the shape the SDK's own aql_version_no_predicate
// advises against (REQ-161 § Checks, SPECPR-481).
//
// The vocabulary is a SEALED SUM rather than a string field because the
// production is a fixed three-way choice that does not recurse into its own
// position: a fourth shape would be a grammar change, not an extension.
//
// What the unexported marker methods actually buy is narrower than "the three
// constructors below are the only values that exist", and the difference is a
// REQ-025 one. Unexported methods block a foreign type from IMPLEMENTING the
// interface; they do not block EMBEDDING it. `type w struct{ aql.VersionPredicate }`
// satisfies VersionPredicate with a nil method set, and calling either method on
// it dereferences that nil. So the closed set is held by a CATALOGUE GATE,
// [derefVersionPredicate], which every dispatch site runs first and which refuses
// anything outside the three shapes — the same answer [derefValue] gives for the
// same shape on [Value].

import (
	"fmt"
	"strings"
)

// VersionPredicate is the bracket a `VERSION` class expression carries — the
// grammar's `versionPredicate : LATEST_VERSION | ALL_VERSIONS |
// standardPredicate` (REQ-163).
//
// The interface is SEALED and has exactly three shapes, one per grammar
// alternative: construct them with [LatestVersion], [AllVersions] and
// [VersionCompare], and pass the result to [Version]. `versionPredicate` does
// not recurse into its own position, so the choice is closed by the grammar
// rather than by policy.
//
// Sealing is by unexported methods, which stops a foreign type IMPLEMENTING the
// interface but not EMBEDDING it: a `struct{ aql.VersionPredicate }` satisfies
// this interface with nil methods. Such a value is caller input, so it is
// REFUSED at [Builder.Build] with an error wrapping [ErrInvalidQuery] rather
// than panicking (REQ-025) — see [derefVersionPredicate].
//
// A nil VersionPredicate denotes the PREDICATE-LESS form (`VERSION v`), which
// stays legal — see [Version].
type VersionPredicate interface {
	// versionBracket is the canonical text BETWEEN the brackets; the emitter
	// writes the brackets itself, so this must never carry one of its own.
	versionBracket() string
	// versionValidate reports a malformed predicate (an operator outside the
	// vocabulary, an empty path, a nil value) so [Builder.Build] surfaces it
	// as ErrInvalidQuery instead of emitting AQL the parser rejects. It is the
	// CARRIER's own check; the rendered text is separately held to
	// [ValidateVersionPredicate] (see [Containment.validateVersionPredicate]).
	versionValidate() error
}

// latestVersion is the `LATEST_VERSION` alternative.
type latestVersion struct{}

func (latestVersion) versionBracket() string { return "LATEST_VERSION" }

func (latestVersion) versionValidate() error { return nil }

// allVersions is the `ALL_VERSIONS` alternative.
type allVersions struct{}

func (allVersions) versionBracket() string { return "ALL_VERSIONS" }

func (allVersions) versionValidate() error { return nil }

// versionComparison is the `standardPredicate` alternative, carrying the
// LANDED [Comparison] vocabulary rather than a second structurally identical
// one: a WHERE comparison, the parsed class predicate
// ([parse.ClassExpr.PredicateComparison]) and this bracket then share ONE
// comparison model and ONE renderer, which is what keeps the spacing from
// drifting between them (REQ-163 § Canonical spellings).
type versionComparison struct{ cmp Comparison }

// versionBracket renders `<path> <op> <value>` — one space each side of the
// operator, no padding inside the brackets. It is [Comparison.expr] itself,
// not a copy of it, which is the whole point of reusing the shape.
func (v versionComparison) versionBracket() string { return v.cmp.expr() }

// versionValidate runs the shared [Comparison.validate] and then the bracket's
// OWN narrowing: `versionPredicate`'s standardPredicate arm reaches the same
// `pathPredicateOperand` the class bracket does, which admits no function call
// even though the WHERE value position the shared [Comparison] otherwise serves
// does ([validatePredicateOperand]).
func (v versionComparison) versionValidate() error {
	if err := v.cmp.validate(); err != nil {
		return err
	}
	return validatePredicateOperand(v.cmp.Val)
}

// LatestVersion is the `[LATEST_VERSION]` version predicate: the most recent
// version of each versioned object.
//
// Stating a tier explicitly is what REQ-161's aql_version_no_predicate
// advisory asks for — the tier a bare `VERSION` defaults to is unspecified
// ([SPECPR-481](https://openehr.atlassian.net/browse/SPECPR-481)), so a
// portable query says which it means.
func LatestVersion() VersionPredicate { return latestVersion{} }

// AllVersions is the `[ALL_VERSIONS]` version predicate: every version of each
// versioned object, not only the latest. Same portability argument as
// [LatestVersion].
func AllVersions() VersionPredicate { return allVersions{} }

// VersionCompare is the `standardPredicate` version predicate — ONE
// `<path> <op> <value>` comparison inside the VERSION bracket, e.g.
//
//	Version("v", VersionCompare("commit_audit/time_committed/value", OpGt, Param("since")))
//	// -> VERSION v[commit_audit/time_committed/value > $since]
//
// The path is RELATIVE to the VERSION object and binds no FROM alias, so it is
// written without one. Exactly one comparison: `standardPredicate` has no
// junction alternative of its own, and `versionPredicate` has none either, so
// there is nothing to join two with.
//
// path and op are held to the same rules a WHERE comparison is ([Comparison]),
// and the whole rendered bracket is additionally held to
// [ValidateVersionPredicate] at [Builder.Build] time. A malformed comparison —
// an unknown operator, an empty path, a nil value, an operand outside
// `pathPredicateOperand` — is refused there rather than emitted.
//
// Edge whitespace is TRIMMED off path, as at every other path-taking
// constructor ([Col], [ColAs], [Builder.From], [Builder.OrderBy]): the canonical
// bracket carries no padding (REQ-163 § Canonical spellings), and storing the
// padding verbatim would emit `v[  uid/value   = $v]` — text that re-parses to
// the same query but is not the spelling the read side emits back, so the
// identity round trip would fail on a query that is otherwise correct.
func VersionCompare(path string, op Operator, v Value) VersionPredicate {
	return versionComparison{cmp: Comparison{Path: strings.TrimSpace(path), Op: op, Val: v}}
}

// Version is a `VERSION` containment operand — `VERSION <alias>[<predicate>]`,
// the grammar's other `classExprOperand` alternative (REQ-163). Nest below it
// and join it exactly as any other [Containment]:
//
//	Version("v", LatestVersion()).Contains(Class("COMPOSITION", "c"))
//	// -> VERSION v[LATEST_VERSION] CONTAINS COMPOSITION c
//
// The RM type is the SDK's OWN spelling and never a caller string: the bracket
// is reachable only from the VERSION alternative, so fixing the type at the
// constructor is what makes "the predicate is on a VERSION node" true by
// construction rather than by a check the caller could fail.
//
// A nil pred is the PREDICATE-LESS form and stays legal: `Version("v", nil)`
// builds and emits exactly what `Class("VERSION", "v")` does, byte for byte.
// REQ-161 advises against that shape with a WARNING and does not refuse it, so
// neither does this — see [LatestVersion] for the advisory's own remedy.
//
// [Archetype] has no counterpart here on purpose: the VERSION alternative has
// no archetype slot at all, so an archetype predicate on a VERSION node is
// refused at [Builder.Build] (the landed rule in [Containment.validateTree]).
func Version(alias string, pred VersionPredicate) Containment {
	return Containment{rmType: "VERSION", alias: alias, versionPred: pred}
}

// validateVersionPredicate holds a VERSION bracket to its own grammar position
// (REQ-163 § The version-predicate carrier). Four checks. Two are borrowed —
// they were already in force elsewhere and are applied here rather than
// re-invented — and two are this carrier's own machinery:
//
//   - the RM-type SPELLING, because `versionPredicate` is reachable from
//     classExprOperand's VERSION alternative alone. The rule is REQ-163's; the
//     FOLD is new machinery, not borrowed: the landed archetype-on-VERSION
//     refusal beside it uses the wider [strings.EqualFold], and the polarity
//     here is the opposite one, so [asciiKeyword] is chosen for this guard
//     rather than inherited from that one (see the comment on the check);
//   - the CATALOGUE, through [derefVersionPredicate] — also this carrier's own:
//     [VersionPredicate] is exported and sealed by unexported methods, which
//     blocks implementing it and not embedding it, so an out-of-catalogue value
//     reaches here and must be refused rather than dereferenced (REQ-025);
//   - the CARRIER's own shape, through the same [Comparison.validate] the WHERE
//     clause runs, plus the shared [validatePredicateOperand] — borrowed, both
//     of them;
//   - the RENDERED bracket text, through [ValidateVersionPredicate] — the very
//     guard `(*parse.Query).Emit` applies at this position, so Build/Emit
//     parity holds from the day the carrier lands rather than being reconciled
//     later (REQ-119 § Single-token identifier positions, applied to the one
//     position that had a guard and no caller). Borrowed.
//
// It runs only when the field is populated; an absent predicate is the legal
// predicate-less form, not a defect.
func (c Containment) validateVersionPredicate() error {
	// [asciiKeyword], not [strings.EqualFold] — and the difference is not
	// cosmetic, because this refusal has the OPPOSITE POLARITY to the
	// archetype-on-VERSION one beside it. There, a WIDER VERSION test refuses
	// more, which is the safe direction; here it would ACCEPT more, and
	// `VERſION` (Unicode-fold-equal to the keyword, unreadable to the lexer,
	// whose keyword fragments are ASCII) would carry a bracket to the wire.
	// [validateRMTypeToken] has already refused that spelling by the time this
	// runs, so today the two folds agree — the tighter test is what keeps them
	// agreeing if that ordering ever changes.
	if !asciiKeyword(c.rmType, "VERSION") {
		return fmt.Errorf("%w: a version predicate on a %q class expression; `versionPredicate` is "+
			"reachable only from classExprOperand's VERSION alternative, so this emits text the "+
			"parser rejects — build the node with aql.Version", ErrInvalidQuery, c.rmType)
	}
	pred, ok := derefVersionPredicate(c.versionPred)
	if !ok {
		return errOutOfCatalogueVersionPredicate()
	}
	if err := pred.versionValidate(); err != nil {
		return fmt.Errorf("CONTAINS version predicate: %w", err)
	}
	if err := ValidateVersionPredicate(pred.versionBracket()); err != nil {
		return fmt.Errorf("CONTAINS version predicate: %w", err)
	}
	return nil
}

// derefVersionPredicate normalises a [VersionPredicate] to the shape it denotes,
// reporting false for anything outside the sealed three-shape catalogue.
//
// It is [derefValue]'s answer to the same problem on [Value], and it exists for
// the same REQ-025 reason: an exported interface sealed by unexported methods
// cannot be IMPLEMENTED from outside the package, but it can be EMBEDDED —
// `struct{ aql.VersionPredicate }` satisfies VersionPredicate with a nil method
// set, and both marker methods then dereference that nil. That is caller input,
// so the library must fail closed with an error, not panic.
//
// It has no POINTER twins, unlike [derefValue]: the three shapes are unexported,
// so no caller outside this package can name one to take its address, and the
// package's own constructors return values. The dispatch tripwire's
// case-coverage sweep (parse/dispatch_tripwire_test.go) registers this switch
// against the derived VersionPredicate vocabulary, so a fourth shape landing
// without a case here fails the build rather than falling silently to the
// refusal below.
func derefVersionPredicate(p VersionPredicate) (VersionPredicate, bool) {
	switch x := p.(type) {
	case latestVersion, allVersions, versionComparison:
		return x, true
	}
	return nil, false // an untyped nil, or a shape outside the catalogue
}

// errOutOfCatalogueVersionPredicate is the refusal every dispatch site shares.
//
// It is value-free and names no type: the offending value is caller-supplied, so
// its Go type name is caller text and has no place in a diagnostic a consuming
// CDR logs (REQ-119 § the redaction rule). What it does name is the route back —
// the three constructors that DO produce a carriable shape.
func errOutOfCatalogueVersionPredicate() error {
	return fmt.Errorf("%w: a version predicate outside the sealed `versionPredicate` vocabulary — the "+
		"interface is satisfied by embedding it as well as by the three constructors, and an embedded "+
		"one carries no bracket; build the predicate with aql.LatestVersion, aql.AllVersions or "+
		"aql.VersionCompare", ErrInvalidQuery)
}
