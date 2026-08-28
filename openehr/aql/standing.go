package aql

// standing.go — REQ-163 § The standing-predicate carrier. The write side of the
// `standardPredicate` alternative of an ordinary class expression's bracket:
//
//	classExprOperand : IDENTIFIER variable=IDENTIFIER? pathPredicate?
//	pathPredicate    : '[' (standardPredicate | archetypePredicate | nodePredicate) ']'
//	standardPredicate: objectPath COMPARISON_OPERATOR pathPredicateOperand
//
// Until this file [Containment] carried an RM type, an alias and an archetype
// id and nothing else, so REQ-161's own documented suppression shape —
// `… CONTAINS VERSIONED_COMPOSITION vo[uid/value = $vo] CONTAINS VERSION
// v[ALL_VERSIONS]` — was unbuildable and a caller who needed it had to
// hand-assemble the query text, the one construction route REQ-055's injection
// guard cannot cover.
//
// EXACTLY ONE comparison, and that bound is a property of the CARRIER rather
// than a claim about the position. `standardPredicate` has no junction
// alternative of its own, so nothing joins two of them; the bracket's THIRD
// alternative, `nodePredicate`, is defined over AND / OR recursively, so a
// junction class predicate is perfectly legal AQL — it simply has no builder
// carrier, before this REQ or after it (REQ-163 § Out of scope). The two are
// easy to conflate, which is why the difference is written down here.
//
// The comparison is the landed [Comparison] rather than a second structurally
// identical shape, so a WHERE comparison, the parsed class predicate
// ([parse.ClassExpr.PredicateComparison]) and this bracket share ONE model and
// ONE renderer — the property that keeps their spacing from drifting apart.

import (
	"fmt"
	"strings"
)

// Predicated returns c carrying a class-position STANDING predicate — the
// `standardPredicate` alternative of the class bracket (REQ-163), one
// `<path> <op> <value>` comparison:
//
//	Class("VERSIONED_COMPOSITION", "vo").Predicated("uid/value", OpEq, Param("vo"))
//	// -> VERSIONED_COMPOSITION vo[uid/value = $vo]
//
// The path is RELATIVE to the class expression and binds no FROM alias, so it
// is written without one; the value renders through the same canonical
// spellings a WHERE comparison uses. As everywhere in the containment algebra c
// itself is not modified — the result is a new value.
//
// Exactly ONE comparison per node. FOUR misuses have no valid tree to return,
// so each is recorded on the node and surfaced by [Builder.Build] as an error
// wrapping [ErrInvalidQuery] — the `invalid` route [Containment.withChild]
// already uses, never absorbed into a shape that means something else:
//
//   - a JUNCTION receiver. A junction carries no class of its own, so it has no
//     bracket position at all; the predicate belongs on the operand it
//     constrains.
//   - a node that already carries an ARCHETYPE predicate. The archetype and
//     standing forms are two mutually exclusive spellings of the ONE `[…]`
//     position, so emitting whichever the renderer reaches first would silently
//     DROP the other — and the dropped one is a row filter, so the query comes
//     back with more rows than the caller asked for.
//   - a SECOND standing predicate on a node already predicated. One bracket
//     renders one predicate and `standardPredicate` has no conjunction of its
//     own, so the first comparison would be dropped the same way. Join the
//     conditions in WHERE.
//   - a VERSION-spelled node. A comparison on a VERSION node is legal AQL, but
//     it is a different grammar position with a different accept set, and it
//     has its own carrier: write
//     `Version(alias, VersionCompare(path, op, value))`. See
//     [VersionCompare]. Routing the call there silently instead would change
//     the caller's accept set under them with no diagnostic, which is the
//     failure this one-carrier-per-position rule exists to prevent.
//
// A malformed comparison — an unknown operator, an empty path, a nil value, an
// operand outside `pathPredicateOperand` — and bracket text that could ESCAPE
// the emitter's own brackets are refused at [Builder.Build] too, by
// [Comparison.validate], [validatePredicateOperand] and [ValidatePathPredicate]
// respectively (see [Containment.validateStandingPredicate]).
//
// Edge whitespace is TRIMMED off path, as at [Col], [ColAs], [Builder.From] and
// [Builder.OrderBy]: the canonical bracket carries no padding (REQ-163
// § Canonical spellings), so storing it verbatim would emit a spelling the read
// side does not emit back and the identity round trip would fail.
//
// # One tolerated asymmetry
//
// path is not scanned for a JUNCTION, so `Predicated("uid/value = 1 AND x", OpEq,
// Int(2))` emits `c[uid/value = 1 AND x = 2]` — a two-condition bracket from a
// carrier that recorded one comparison. The VERSION side refuses the same shape,
// because [ValidateVersionPredicate] has an explicit junction check that
// [ValidatePathPredicate] does not: `nodePredicate`, the bracket's third
// alternative, IS defined over AND / OR, so a junction here is legal AQL at a
// position the builder has no carrier for (REQ-163 § Out of scope) rather than
// text the parser rejects. That puts it in the same class as [Col]'s leniency —
// loud, ordinary AQL that says what the caller wrote — and it is left tolerated
// on REQ-119's own rule that refusal is reserved for the silent-substitution
// mode. Written down here so it stops being rediscovered as a defect.
//
// The bracket's other alternatives — a node id, a name slot, a regex match, an
// AND / OR node predicate — are legal AQL that no builder carrier spells; they
// are out of scope for this REQ, so the write vocabulary here is bounded by the
// `standardPredicate` and `archetypePredicate` alternatives.
func (c Containment) Predicated(path string, op Operator, v Value) Containment {
	if err := c.standingRefusal(); err != nil {
		// The FIRST defect wins, matching [Containment.validateTree], which
		// reports the first structural defect it reaches: a later refusal must
		// not overwrite the diagnosis of an earlier one.
		if c.invalid == nil {
			c.invalid = err
		}
		return c
	}
	c.standingPred = &Comparison{Path: strings.TrimSpace(path), Op: op, Val: v}
	return c
}

// standingRefusal reports the REQ-163 misuse of [Containment.Predicated] on
// this receiver, or nil when the node can carry the bracket. It looks at the
// RECEIVER only — the comparison's own shape is checked at [Builder.Build]
// time, where every other predicate defect is reported.
//
// Diagnostics stay value-free: none of the four names the predicate's path or
// value (REQ-119 § the redaction rule). The archetype id is named because it is
// the coordinate that identifies WHICH bracket is already occupied, exactly as
// the landed archetype-on-VERSION refusal names it.
func (c Containment) standingRefusal() error {
	switch {
	case c.isJunction():
		return fmt.Errorf("%w: a standing predicate on a containment junction — a junction carries no "+
			"class of its own, so it has no `[…]` position; put the predicate on the operand it "+
			"constrains", ErrInvalidQuery)
	case asciiKeyword(c.rmType, "VERSION"):
		return versionRoutingRefusal()
	case c.archetypeID != "":
		return fmt.Errorf("%w: a standing predicate beside the archetype predicate %q — the two are "+
			"mutually exclusive spellings of ONE `[…]` position, so one would be silently dropped; "+
			"keep one bracket and move the other condition to WHERE — or, under NOT CONTAINS, where "+
			"this node's alias binds no result row, restructure the term",
			ErrInvalidQuery, c.archetypeID)
	case c.standingPred != nil:
		return fmt.Errorf("%w: a second standing predicate on one class expression — `standardPredicate` "+
			"is ONE `objectPath COMPARISON_OPERATOR pathPredicateOperand` and the bracket holds one "+
			"predicate, so the first would be silently dropped; join the conditions in WHERE — or, "+
			"under NOT CONTAINS, where this node's alias binds no result row, restructure the term",
			ErrInvalidQuery)
	}
	return nil
}

// versionRoutingRefusal is REQ-163 § One carrier per grammar position: a
// comparison on a VERSION node is legal AQL and MUST be expressible, but
// through the VERSION bracket's own carrier and not through this one.
//
// It NAMES the constructor that does carry the shape, because the alternative
// to a named route is a grammar citation the caller cannot act on. The two
// accept sets genuinely differ in both directions — `versionPredicate` carries
// no node predicate, `pathPredicate` carries no LATEST_VERSION — so routing the
// call across silently would change the caller's accept set under them with no
// diagnostic. Refusing is loud, named, and breaks nothing: no such call builds
// today.
func versionRoutingRefusal() error {
	return fmt.Errorf("%w: a standing predicate on a VERSION class expression — that bracket is "+
		"`versionPredicate`, a different grammar position with a different accept set; carry the "+
		"comparison with aql.Version(alias, aql.VersionCompare(path, op, value))", ErrInvalidQuery)
}

// validateStandingPredicate holds a class bracket to its own grammar position
// (REQ-163 § The standing-predicate carrier). It runs only when the field is
// populated; an absent predicate is the ordinary bare class expression.
//
// Five checks. The first two RESTATE the [Containment.Predicated] refusals at
// the Build seam — deliberately, because [Containment] is a struct this package
// can populate directly and the combinator is not the only way in. The last
// three are the ones the combinator cannot make: they need the comparison, which
// the caller supplies after the receiver is chosen.
//
//   - the RM-type SPELLING, ASCII-case-insensitively, because a VERSION node's
//     bracket is the other production ([versionRoutingRefusal]);
//   - the ARCHETYPE field beside it, because one `[…]` position renders one
//     predicate and [Containment.classToken] would silently drop the other;
//   - the CARRIER's own shape, through the same [Comparison.validate] the WHERE
//     clause runs, so an unknown operator or a nil value is refused rather than
//     rendered as Go text;
//   - the OPERAND, through [validatePredicateOperand]: the bracket's accept set
//     is `pathPredicateOperand`, which is NARROWER than the WHERE value position
//     the shared [Comparison] otherwise admits;
//   - the RENDERED bracket text, through [ValidatePathPredicate] — the guard
//     this position already has, and the one `(*parse.Query).Emit` applies to
//     it — so Build/Emit parity holds from the day the carrier lands.
func (c Containment) validateStandingPredicate() error {
	// [asciiKeyword], not [strings.EqualFold]. The polarity here is the
	// OPPOSITE of [Containment.validateVersionPredicate]'s — that guard refuses
	// on a NON-match, this one refuses on a MATCH — so a wider fold refuses
	// MORE here, and the narrower fold is the one that lets a spelling through.
	// The argument for it is therefore not the other guard's; it is about which
	// carrier the node is ROUTED to.
	//
	// `VERſION` (U+017F, Unicode-fold-equal to the keyword, unreadable to a
	// lexer whose keyword fragments are ASCII) is not the VERSION keyword and
	// the parser reads it as an ordinary class. Under [strings.EqualFold] this
	// guard would route it away to a carrier that FIXES the RM type to
	// `VERSION` — telling the caller to write a node they did not write.
	// [asciiKeyword] keeps the accept set to the spellings the lexer actually
	// tokenises: same ASCII alphabet as every other write-side fold site, same
	// accept-set principle, opposite polarity.
	//
	// As at the version guard, [validateRMTypeToken] has already refused that
	// spelling by the time this runs, so today the two folds agree — this is
	// what keeps them agreeing if that ordering ever changes.
	if asciiKeyword(c.rmType, "VERSION") {
		return versionRoutingRefusal()
	}
	if c.archetypeID != "" {
		return fmt.Errorf("%w: a class expression carrying both a standing predicate and the archetype "+
			"predicate %q; `pathPredicate` renders ONE of its alternatives, so the other would be "+
			"silently dropped", ErrInvalidQuery, c.archetypeID)
	}
	if err := c.standingPred.validate(); err != nil {
		return fmt.Errorf("CONTAINS standing predicate: %w", err)
	}
	if err := validatePredicateOperand(c.standingPred.Val); err != nil {
		return fmt.Errorf("CONTAINS standing predicate: %w", err)
	}
	if err := ValidatePathPredicate(c.standingPred.expr()); err != nil {
		return fmt.Errorf("CONTAINS standing predicate: %w", err)
	}
	return nil
}

// validatePredicateOperand holds the RIGHT operand of a class-position bracket
// comparison to `pathPredicateOperand`, the production BOTH brackets reach:
//
//	standardPredicate    : objectPath COMPARISON_OPERATOR pathPredicateOperand
//	pathPredicateOperand : primitive | objectPath | PARAMETER | ID_CODE | AT_CODE
//
// A function call is not among those alternatives, so `Func("LENGTH", …)` — and
// [Terminology], which is the same [FuncCall] shape with a fixed name — emits
// text the SDK's OWN parser rejects (`v[commit_audit/time > LENGTH(x)]`). The
// value position in WHERE does admit one, which is exactly why the vocabulary
// is NOT forked: REQ-163 § The standing-predicate carrier makes the shared
// [Comparison] a MUST so a WHERE comparison, the parsed class predicate and the
// built bracket keep one model — so the narrowing is a guard at the two bracket
// positions, not a second operand type.
//
// ONE function serves both carriers ([versionComparison.versionValidate] and
// [Containment.validateStandingPredicate]) for the reason the carriers share a
// comparison at all: a rule written once cannot drift between the two.
//
// The shape is read through [DerefValue], the value catalogue's own normaliser,
// so a `*FuncCall` is judged as the [FuncCall] it denotes rather than waved
// through as a pointer (REQ-119 § the pointer-twin rule) — and not through
// reflection, which the idiom spec bans. A value the catalogue does not know is
// left alone here: [Comparison.validate] has already refused a nil operand, and
// [ValidateValue] owns the out-of-catalogue verdict, so duplicating it would put
// two refusals on one defect.
//
// The diagnostic is value-free: it names the grammar production and the shape,
// never the argument text (REQ-119 § the redaction rule).
func validatePredicateOperand(v Value) error {
	inner, known := DerefValue(v)
	if !known {
		return nil
	}
	if _, isCall := inner.(FuncCall); isCall {
		return fmt.Errorf("%w: a function call as the operand of a class-predicate comparison — "+
			"`pathPredicateOperand` is `primitive | objectPath | PARAMETER | ID_CODE | AT_CODE` and "+
			"admits no function call, so this emits text the parser rejects; bind the computed value "+
			"with aql.Param, or move the condition to WHERE, where the value position does admit one",
			ErrInvalidQuery)
	}
	return nil
}
