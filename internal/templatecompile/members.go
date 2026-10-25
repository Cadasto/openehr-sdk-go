package templatecompile

import (
	"slices"

	"github.com/cadasto/openehr-sdk-go/openehr/template"
)

// LowerAtLeastOne reports whether the interval m has a bounded lower bound
// of 1 or more. A nil interval, one the template does not state, has none.
// The template validator and the example generator read an existence, a
// cardinality or an occurrences interval this way when they decide what a
// template asks for.
func LowerAtLeastOne(m *template.Multiplicity) bool {
	return m != nil && !m.LowerUnbounded() && m.Lower() >= 1
}

// AsksForMember reports whether the template asks the multi-valued
// attribute a for at least one member: its cardinality
// ([CompiledAttribute.ChildMultiplicity]), or the occurrences of a child it
// allows ([AllowedChildNodes]), has a bounded lower bound of 1 or more
// ([LowerAtLeastOne]). A template that states no cardinality, and a child
// that states no occurrences, ask for no member. The attribute's existence
// asks for the list alone and is not read here. The template validator
// reads a list this way when it decides whether the template requires it,
// and the example generator when it decides whether only the list's
// existence asks for a member, so the two cannot drift apart.
func AsksForMember(a *CompiledAttribute) bool {
	if LowerAtLeastOne(a.ChildMultiplicity()) {
		return true
	}
	return slices.ContainsFunc(a.allowed, func(c *CompiledNode) bool {
		return LowerAtLeastOne(c.Occurrences())
	})
}

// HasMandatoryChild reports whether a child the template allows under the
// attribute a ([AllowedChildNodes]), other than a slot, has an occurrences
// lower bound that is bounded and 1 or more ([LowerAtLeastOne]): a child
// ADL 1.4 calls mandatory in its container. A child that states no
// occurrences counts for none, as in [AsksForMember], and the attribute's
// cardinality is not read. The example generator reads a multi-valued
// attribute this way when it decides whether a child makes the attribute
// required; the template validator counts the same children against their
// occurrences on an absent list, so the two cannot drift apart.
func HasMandatoryChild(a *CompiledAttribute) bool {
	return slices.ContainsFunc(a.allowed, func(c *CompiledNode) bool {
		return !c.IsSlot() && LowerAtLeastOne(c.Occurrences())
	})
}
