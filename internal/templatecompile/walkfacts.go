package templatecompile

// walkfacts.go: what a validation walk asks of a compiled node and of a
// compiled attribute on every visit, resolved once when the template is
// compiled: the children an attribute allows and whether it is prohibited,
// and which RM classes conform to a node's RM type. What the pinned
// Reference Model says of the attributes of a class depends on the class
// alone, so it is not stored on each node: the walk reads it from a table of
// its own, built once per class. Everything here is set by [Compile] and
// never changed after it returns, so a compiled template is read by any
// number of goroutines with no lock.
//
// The functions below are package-level, not methods, so that the public
// alias of [CompiledNode] and [CompiledAttribute] in openehr/templatecompile
// gains no method: they are reachable inside this module only.

import (
	"slices"

	"github.com/cadasto/openehr-sdk-go/openehr/rm/rminfo"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
)

// resolveWalkFacts sets what a validation walk asks of cn, whose attributes
// are compiled: the children each attribute allows and whether it is
// prohibited, and the classes that conform to cn's RM type, which the
// package resolved once per class. It clips the attributes and the children
// it hands out, so a caller that appends to one cannot write into the
// compiled template.
func resolveWalkFacts(cn *CompiledNode) {
	cn.attributes = slices.Clip(cn.attributes)
	for _, a := range cn.attributes {
		a.children = slices.Clip(a.children)
		a.allowed, a.prohibited = allowedChildren(a.existence, a.children)
	}
	cn.conforming = conformingClasses[cn.rmTypeName]
	cn.conformingResolved = true
}

// allowedChildren returns, in their order, the children the template allows
// of an attribute with the existence existence and the children children,
// and whether it prohibits the attribute. A child is allowed when its
// occurrences state no upper bound of 0. The attribute is prohibited when
// its existence has a bounded upper bound of 0, or it has a child and allows
// none; an attribute with no child is open, not prohibited.
func allowedChildren(existence *template.Multiplicity, children []*CompiledNode) (allowed []*CompiledNode, prohibited bool) {
	allowed = children
	if slices.ContainsFunc(children, func(c *CompiledNode) bool { return !childAllowed(c) }) {
		allowed = slices.Clip(slices.DeleteFunc(slices.Clone(children), func(c *CompiledNode) bool { return !childAllowed(c) }))
	}
	prohibited = len(children) > 0 && len(allowed) == 0
	if existence != nil && !existence.UpperUnbounded() && existence.Upper() == 0 {
		prohibited = true
	}
	return allowed, prohibited
}

// childAllowed reports whether the template allows the child: its
// occurrences upper bound is unbounded, or bounded above 0, or it states
// none.
func childAllowed(c *CompiledNode) bool {
	occ := c.occurrences
	return occ == nil || occ.UpperUnbounded() || occ.Upper() != 0
}

// NodeAttributes returns the compiled attributes of n, as
// [CompiledNode.Attributes] does, without copying them: the caller must not
// change the slice.
func NodeAttributes(n *CompiledNode) []*CompiledAttribute { return n.attributes }

// ChildNodes returns the children of a, as [CompiledAttribute.Children]
// does, without copying them: the caller must not change the slice.
func ChildNodes(a *CompiledAttribute) []*CompiledNode { return a.children }

// AllowedChildNodes returns the children of a the template allows, in their
// order: those whose occurrences state no upper bound of 0. When it allows
// every child the slice is [ChildNodes]'s own. The caller must not change
// the slice.
func AllowedChildNodes(a *CompiledAttribute) []*CompiledNode { return a.allowed }

// Prohibited reports whether the template prohibits a: its existence has a
// bounded upper bound of 0, or it has a child and allows none of them
// ([AllowedChildNodes]). An attribute with no child is open, not prohibited.
func Prohibited(a *CompiledAttribute) bool { return a.prohibited }

// ConformsTo reports whether the bare RM class class conforms to n's RM
// type, which is that type or descends from it, as the pinned class
// hierarchy of [rminfo.Default] answers it, and whether the hierarchy knows
// both names, as [rminfo.Hierarchy.ConformsTo] does. n's type is looked up
// as written, so a parameterised type such as "DV_INTERVAL<DV_QUANTITY>",
// which is no class, is not known.
func ConformsTo(n *CompiledNode, class string) (conforms, known bool) {
	i, classKnown := classIndex[class]
	if !classKnown {
		return false, false
	}
	bits := n.conforming
	if !n.conformingResolved {
		bits = conformingClasses[n.rmTypeName]
	}
	if bits == nil {
		return false, false
	}
	return bits.has(i), true
}

// classBits is a set of the classes [rminfo.Default] knows, by their index
// in classIndex.
type classBits []uint64

// has reports whether the class of index i is in s.
func (s classBits) has(i int) bool { return s[i/64]&(1<<(i%64)) != 0 }

// classIndex gives each class [rminfo.Default] knows its index in a
// classBits, and conformingClasses, for each of them, the classes that
// conform to it: itself and every class that descends from it. Both are
// built once, when the package is initialised, and never changed.
var classIndex, conformingClasses = conformanceTables(rminfo.Default)

// conformanceTables builds the classIndex and conformingClasses of the class
// hierarchy l answers; both are empty when l answers none.
func conformanceTables(l rminfo.Lookup) (map[string]int, map[string]classBits) {
	h, ok := l.(rminfo.Hierarchy)
	if !ok {
		return map[string]int{}, map[string]classBits{}
	}
	known := l.KnownRMTypes()
	index := make(map[string]int, len(known))
	for i, class := range known {
		index[class] = i
	}
	words := (len(known) + 63) / 64
	conforming := make(map[string]classBits, len(known))
	for _, class := range known {
		conforming[class] = make(classBits, words)
	}
	for i, class := range known {
		ancestors, _ := h.Ancestors(class)
		for _, ancestor := range append(ancestors, class) {
			if bits, ok := conforming[ancestor]; ok {
				bits[i/64] |= 1 << (i % 64)
			}
		}
	}
	return index, conforming
}
