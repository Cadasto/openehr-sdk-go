package templatecompile

import (
	"iter"
	"slices"

	"github.com/cadasto/openehr-sdk-go/openehr/rm/rminfo"
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

// MandatoryListChildren yields, in template order, the children that ask a
// list of the attribute a for a member while the list holds none, absent or
// present and empty: each child the template allows ([AllowedChildNodes]),
// other than a slot, whose occurrences lower bound is bounded and 1 or more
// ([LowerAtLeastOne]), a child ADL 1.4 calls mandatory in its container. A
// child that states no occurrences asks for none, as in [AsksForMember],
// and neither the attribute's existence nor its cardinality is read.
//
// It yields none where a holds no such list. That is so where the template
// writes a as single-valued: its children are alternatives, and its
// existence decides how often one occurs. It is so where the template
// prohibits a ([Prohibited]), which leaves its children nothing to count.
// And it is so where the RM types the items of a as primitive values, such
// as the octets of a DV_MULTIMEDIA's data: no item binds to a child there.
//
// It is the one reading of these children that the example generator and
// the template validator share. Under Required, the generator visits a list
// with such a child and builds the child's members. The validator reports
// each such child as missing where a list the template leaves optional by
// its existence holds no item. So a list Required leaves out is one the
// validator accepts as absent.
func MandatoryListChildren(a *CompiledAttribute) iter.Seq[*CompiledNode] {
	return func(yield func(*CompiledNode) bool) {
		if a.cardinality != template.Multiple || a.prohibited || primitiveItems(a) {
			return
		}
		for _, c := range a.allowed {
			if !c.IsSlot() && LowerAtLeastOne(c.Occurrences()) && !yield(c) {
				return
			}
		}
	}
}

// primitiveItems reports whether the RM types the items of the list a as
// primitive values, such as Octet or String, which have nothing below them,
// as the pinned Reference Model's absence table names them
// ([rminfo.AbsencePrimitive]); the template validator reads such a list's
// items by count alone. The Hash the RM declares for every Hash attribute
// is primitive too, but it is no list of such items: the validator reads a
// Hash entry by entry.
func primitiveItems(a *CompiledAttribute) bool {
	if a.rmTypeName == "" || a.rmTypeName == hashType {
		return false
	}
	ar, ok := rminfo.Default.(rminfo.AbsenceReporter)
	return ok && ar.AbsenceReason(a.rmTypeName) == rminfo.AbsencePrimitive
}

// hashType is the RM type the pinned Reference Model declares for every
// Hash attribute.
const hashType = "Hash"

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
