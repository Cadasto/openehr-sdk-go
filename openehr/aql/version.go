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
// position: a fourth shape would be a grammar change, not an extension. Sealing
// is by unexported marker methods on unexported types, so the three
// constructors below are the only values that exist.

import "fmt"

// VersionPredicate is the bracket a `VERSION` class expression carries — the
// grammar's `versionPredicate : LATEST_VERSION | ALL_VERSIONS |
// standardPredicate` (REQ-163).
//
// The interface is SEALED and has exactly three shapes, one per grammar
// alternative: construct them with [LatestVersion], [AllVersions] and
// [VersionCompare], and pass the result to [Version]. There is no fourth shape
// and no way to add one from outside this package — `versionPredicate` does not
// recurse into its own position, so the choice is closed by the grammar rather
// than by policy.
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

func (v versionComparison) versionValidate() error { return v.cmp.validate() }

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
// an unknown operator, an empty path, a nil value — is refused there rather
// than emitted.
func VersionCompare(path string, op Operator, v Value) VersionPredicate {
	return versionComparison{cmp: Comparison{Path: path, Op: op, Val: v}}
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
// (REQ-163 § The version-predicate carrier). Three checks, each already in
// force somewhere and applied here rather than re-invented:
//
//   - the RM-type SPELLING, because `versionPredicate` is reachable from
//     classExprOperand's VERSION alternative alone;
//   - the CARRIER's own shape, through the same [Comparison.validate] the
//     WHERE clause runs;
//   - the RENDERED bracket text, through [ValidateVersionPredicate] — the very
//     guard `(*parse.Query).Emit` applies at this position, so Build/Emit
//     parity holds from the day the carrier lands rather than being reconciled
//     later (REQ-119 § Single-token identifier positions, applied to the one
//     position that had a guard and no caller).
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
	if err := c.versionPred.versionValidate(); err != nil {
		return fmt.Errorf("CONTAINS version predicate: %w", err)
	}
	if err := ValidateVersionPredicate(c.versionPred.versionBracket()); err != nil {
		return fmt.Errorf("CONTAINS version predicate: %w", err)
	}
	return nil
}
