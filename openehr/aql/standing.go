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

import "fmt"

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
// Exactly ONE comparison per node. Three misuses have no valid tree to return,
// so each is recorded on the node and surfaced by [Builder.Build] as an error
// wrapping [ErrInvalidQuery] — the `invalid` route [Containment.withChild]
// already uses, never absorbed into a shape that means something else:
//
//   - a JUNCTION receiver. A junction carries no class of its own, so it has no
//     bracket position at all; the predicate belongs on the operand it
//     constrains.
//   - a node that already carries an ARCHETYPE predicate, or a second call on a
//     node already predicated. The archetype and standing forms are two
//     mutually exclusive spellings of the ONE `[…]` position, so emitting
//     whichever the renderer reaches first would silently DROP the other — and
//     the dropped one is a row filter, so the query comes back with more rows
//     than the caller asked for. A second standing predicate drops the first
//     the same way.
//   - a VERSION-spelled node. A comparison on a VERSION node is legal AQL, but
//     it is a different grammar position with a different accept set, and it
//     has its own carrier: write
//     `Version(alias, VersionCompare(path, op, value))`. See
//     [VersionCompare]. Routing the call there silently instead would change
//     the caller's accept set under them with no diagnostic, which is the
//     failure this one-carrier-per-position rule exists to prevent.
//
// A malformed comparison — an unknown operator, an empty path, a nil value —
// and bracket text that could ESCAPE the emitter's own brackets are refused at
// [Builder.Build] too, by [Comparison.validate] and [ValidatePathPredicate]
// respectively (see [Containment.validateStandingPredicate]).
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
	c.standingPred = &Comparison{Path: path, Op: op, Val: v}
	return c
}

// standingRefusal reports the REQ-163 misuse of [Containment.Predicated] on
// this receiver, or nil when the node can carry the bracket. It looks at the
// RECEIVER only — the comparison's own shape is checked at [Builder.Build]
// time, where every other predicate defect is reported.
//
// Diagnostics stay value-free: none of the three names the predicate's path or
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
			"keep one bracket and move the other condition to WHERE",
			ErrInvalidQuery, c.archetypeID)
	case c.standingPred != nil:
		return fmt.Errorf("%w: a second standing predicate on one class expression — `standardPredicate` "+
			"is ONE `objectPath COMPARISON_OPERATOR pathPredicateOperand` and the bracket holds one "+
			"predicate, so the first would be silently dropped; join the conditions in WHERE",
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
// Four checks. The first two RESTATE the [Containment.Predicated] refusals at
// the Build seam — deliberately, because [Containment] is a struct this package
// can populate directly and the combinator is not the only way in. The last two
// are the ones the combinator cannot make: they need the comparison, which the
// caller supplies after the receiver is chosen.
//
//   - the RM-type SPELLING, ASCII-case-insensitively, because a VERSION node's
//     bracket is the other production ([versionRoutingRefusal]);
//   - the ARCHETYPE field beside it, because one `[…]` position renders one
//     predicate and [Containment.classToken] would silently drop the other;
//   - the CARRIER's own shape, through the same [Comparison.validate] the WHERE
//     clause runs, so an unknown operator or a nil value is refused rather than
//     rendered as Go text;
//   - the RENDERED bracket text, through [ValidatePathPredicate] — the guard
//     this position already has, and the one `(*parse.Query).Emit` applies to
//     it — so Build/Emit parity holds from the day the carrier lands.
func (c Containment) validateStandingPredicate() error {
	// [asciiKeyword], not [strings.EqualFold]: this test ACCEPTS the node when
	// the spelling is not VERSION, so a wider fold accepts more, and `VERſION`
	// (Unicode-fold-equal, unreadable to a lexer whose keyword fragments are
	// ASCII) would carry a pathPredicate onto the VERSION alternative. Same
	// polarity, same reasoning, as [Containment.validateVersionPredicate].
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
	if err := ValidatePathPredicate(c.standingPred.expr()); err != nil {
		return fmt.Errorf("CONTAINS standing predicate: %w", err)
	}
	return nil
}
