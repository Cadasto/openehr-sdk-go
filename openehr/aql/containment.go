package aql

import (
	"fmt"
	"slices"
	"strings"
)

// Containment is a containment term in the FROM clause. It is either a single
// class expression (`OBSERVATION o[openEHR-EHR-OBSERVATION.body_temperature.v2]`)
// or — since REQ-117 — a whole containment expression: a chain of nested
// CONTAINS terms, a negated term, or a boolean junction of sibling operands.
//
// Construct a leaf with [Class] (no archetype predicate) or [Archetype], nest
// with [Containment.Contains] / [Containment.NotContains], and join siblings
// with [ContainsAnd] / [ContainsOr]. Every combinator returns a NEW value —
// a Containment is immutable once constructed, so one operand can be reused
// across several expressions. Pass the result to [Builder.Contains], or to
// [Builder.NotContains] to negate the connector from the FROM root.
//
// The zero value is not a valid term: [Builder.Build] refuses it (an operand
// with neither a class nor operands is unrepresentable) rather than silently
// dropping it.
//
// A junction may only END a containment chain. The grammar's `containsExpr`
// admits a parenthesised group as a whole alternative, which no CONTAINS may
// follow, so a junction with a further term after it in the same chain — from
// either [Containment.Contains] or [Builder.Contains] — is refused by
// [Builder.Build]. Write the deeper nesting inside the junction's operands.
//
// A junction always sits below a CONTAINS keyword here. A junction at the FROM
// ROOT (`FROM COMPOSITION c1 OR COMPOSITION c2`) is grammar-admitted and the
// parse side models it, but it has no builder entry point — the write side
// keeps a single root class, so the FROM root is never parenthesised and
// [Builder.From] / [Builder.FromEHR] stay unchanged.
type Containment struct {
	rmType      string
	alias       string
	archetypeID string

	// children are the nested CONTAINS terms of a class node, or the
	// operands of a junction node (which carries no class of its own).
	children []Containment

	// join is the boolean combinator across a junction node's operands.
	join containsJoin

	// negated marks this term as the right operand of a NOT CONTAINS
	// connector. The flag belongs to the connector, so the PARENT
	// consumes it when it writes the keyword — mirroring
	// parse.Containment.Negated.
	negated bool
}

// Archetype is a containment constraint: `<rmType> <alias>[<archetypeID>]`. An
// empty archetypeID emits `<rmType> <alias>` with no predicate.
func Archetype(rmType, alias, archetypeID string) Containment {
	return Containment{rmType: rmType, alias: alias, archetypeID: archetypeID}
}

// Class is a containment operand with no archetype predicate —
// `<rmType> <alias>`, the grammar's bare `classExprOperand` (REQ-117). It is
// [Archetype] with an empty archetypeID, spelled for the containment-algebra
// call sites where a third empty argument would read as a choice rather than
// an omission.
func Class(rmType, alias string) Containment {
	return Containment{rmType: rmType, alias: alias}
}

// Contains nests child below c with a CONTAINS connector (REQ-117):
// `Class("COMPOSITION", "c").Contains(Class("OBSERVATION", "o"))` emits
// `COMPOSITION c CONTAINS OBSERVATION o`. Repeated calls extend the chain in
// call order; c itself is not modified.
func (c Containment) Contains(child Containment) Containment {
	child.negated = false
	return c.withChild(child)
}

// NotContains nests child below c with a NOT CONTAINS connector — the
// grammar's `classExprOperand NOT CONTAINS containsExpr` (REQ-117), i.e. the
// absence of child below c. Otherwise identical to [Containment.Contains].
func (c Containment) NotContains(child Containment) Containment {
	child.negated = true
	return c.withChild(child)
}

// withChild returns c with child appended. The children slice is CLONED so
// two derivations from the same operand cannot write the same backing array
// (`base.Contains(x)` and `base.Contains(y)` are independent).
func (c Containment) withChild(child Containment) Containment {
	c.children = append(slices.Clone(c.children), child)
	return c
}

// ContainsAnd joins containment operands with AND — the grammar's
// `containsExpr AND containsExpr` (REQ-117), i.e. every operand must be
// present. A single operand is returned unchanged (there is nothing to
// join); no operands yields the zero [Containment], which [Builder.Build]
// refuses.
//
// AND binds tighter than OR, so an operand needs no parentheses unless the
// grouping is load-bearing — the emitter adds them exactly there. Negation
// attaches to a CONTAINS connector ([Containment.NotContains]), never to a
// junction operand: the grammar admits NOT only as `NOT? CONTAINS`.
func ContainsAnd(operands ...Containment) Containment {
	return containmentJunction(joinAnd, operands)
}

// ContainsOr joins containment operands with OR — the grammar's
// `containsExpr OR containsExpr` (REQ-117), i.e. at least one operand must be
// present. Same collapse rules as [ContainsAnd].
func ContainsOr(operands ...Containment) Containment {
	return containmentJunction(joinOr, operands)
}

func containmentJunction(j containsJoin, operands []Containment) Containment {
	switch len(operands) {
	case 0:
		// Nothing to join. The zero value fails Build loudly rather than
		// emitting an empty CONTAINS (no silent loss, REQ-117).
		return Containment{}
	case 1:
		return operands[0]
	default:
		return Containment{children: slices.Clone(operands), join: j}
	}
}

// containsJoin is the boolean combinator joining sibling containment
// operands. AND is the AQL default; OR is explicit.
type containsJoin int

const (
	joinAnd containsJoin = iota
	joinOr
)

func (j containsJoin) keyword() string {
	if j == joinOr {
		return "OR"
	}
	return "AND"
}

// isJunction reports whether c is a pure boolean grouping — operands only,
// no class of its own. Mirrors parse's isContainmentJunction.
func (c Containment) isJunction() bool { return c.rmType == "" && len(c.children) > 0 }

// classToken renders the class expression at this node (never the children).
func (c Containment) classToken() string {
	out := c.rmType
	if c.alias != "" {
		out += " " + c.alias
	}
	if c.archetypeID != "" {
		out += "[" + c.archetypeID + "]"
	}
	return out
}

// emit renders one containment term. The negated flag is consumed by the
// PARENT (which writes `NOT CONTAINS` instead of `CONTAINS`), so emit
// ignores it and renders the class plus its chained children.
//
// This mirrors parse's emitContainment byte-for-byte: openehr/aql cannot
// import openehr/aql/parse (the dependency runs the other way), so the two
// canonicalisers are held equivalent mechanically — every builder output is
// re-parsed and re-emitted in containment_roundtrip_test.go, which fails on
// any divergence.
func (c Containment) emit() string {
	// A junction keeps its parentheses wherever it appears in the builder
	// tree: it is always the right operand of a CONTAINS keyword or an
	// operand of another junction, never the bare FROM root.
	if c.isJunction() {
		return "(" + c.emitOperands() + ")"
	}
	var sb strings.Builder
	sb.WriteString(c.classToken())
	for _, ch := range c.children {
		sb.WriteString(ch.connector())
		sb.WriteString(ch.emit())
	}
	return sb.String()
}

// connector is the CONTAINS keyword introducing c from its parent, with its
// surrounding single spaces.
func (c Containment) connector() string {
	if c.negated {
		return " NOT CONTAINS "
	}
	return " CONTAINS "
}

// emitOperands renders a junction node's operands joined by its keyword,
// WITHOUT enclosing parentheses. An operand is parenthesised only where the
// grouping is load-bearing:
//
//   - an OR junction inside an AND junction (precedence: AND binds tighter);
//   - a CONTAINS chain, because the grammar's `CONTAINS containsExpr` right
//     operand is greedy — without the parentheses the following AND / OR
//     operand would re-parse INTO the chain.
//
// A same- or tighter-binding junction operand needs no grouping, mirroring
// [Junction]'s WHERE-side rule and parse's emitContainmentOperands.
func (c Containment) emitOperands() string {
	parts := make([]string, len(c.children))
	for i, ch := range c.children {
		switch {
		case ch.isJunction():
			inner := ch.emitOperands()
			if c.join == joinAnd && ch.join == joinOr {
				inner = "(" + inner + ")"
			}
			parts[i] = inner
		case len(ch.children) > 0:
			parts[i] = "(" + ch.emit() + ")"
		default:
			parts[i] = ch.emit()
		}
	}
	return strings.Join(parts, " "+c.join.keyword()+" ")
}

// validateTree walks the containment tree and reports the first structural
// defect: a class node missing its RM type or alias, any alias already
// recorded in seen (aliases are query-scoped, so two branches of a junction
// may not reuse one), and a junction in a non-final chain position. Mirrors
// parse's duplicateAlias walk so the read and write sides refuse the same
// trees.
func (c Containment) validateTree(seen map[string]bool) error {
	if !c.isJunction() {
		if c.rmType == "" || c.alias == "" {
			return fmt.Errorf("%w: CONTAINS requires an RM type and alias", ErrInvalidQuery)
		}
		if seen[c.alias] {
			return fmt.Errorf("%w: duplicate alias %q", ErrInvalidQuery, c.alias)
		}
		seen[c.alias] = true
		// REQ-117: a class node's children are a CONTAINS chain, so the
		// junction-placement rule applies. A junction node's children are
		// unordered operands — no CONTAINS keyword separates them — so the
		// rule deliberately does NOT apply there.
		if err := validateContainsChain(c.children); err != nil {
			return err
		}
	}
	for _, ch := range c.children {
		if err := ch.validateTree(seen); err != nil {
			return err
		}
	}
	return nil
}

// validateContainsChain refuses a junction anywhere but the END of a CONTAINS
// chain (REQ-117). The grammar takes a parenthesised group as a whole
// alternative — `containsExpr : classExprOperand (NOT? CONTAINS containsExpr)?
// | '(' containsExpr ')' | …` — so nothing may follow one with a CONTAINS
// keyword, and a junction with a further term after it emits AQL the parser
// rejects (`… CONTAINS (OBSERVATION o OR EVALUATION ev) NOT CONTAINS ACTION
// a`). Build fails closed here to keep the REQ-055 structural guarantee that
// the typed builders cannot emit syntactically invalid AQL; re-shaping the
// tree instead (distributing the tail under the junction's operands) would
// silently change what the query means, so the caller writes the nesting
// they meant.
func validateContainsChain(chain []Containment) error {
	for _, c := range chain[:max(len(chain)-1, 0)] {
		if c.isJunction() {
			return fmt.Errorf("%w: containment junction followed by a further CONTAINS term — a junction may "+
				"only end a containment chain; write the deeper nesting inside its operands", ErrInvalidQuery)
		}
	}
	return nil
}
