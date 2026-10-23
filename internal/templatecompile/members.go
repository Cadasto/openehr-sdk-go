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
