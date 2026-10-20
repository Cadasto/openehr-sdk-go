package instance

import (
	"strings"

	"github.com/cadasto/openehr-sdk-go/internal/bmmtype"
	tcimpl "github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/rminfo"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
)

// undescribed.go: which template nodes need content the template does not
// describe. A member is undescribed when no template node describes it: a
// slot fill, whose body the template leaves to the caller, or a LOCATABLE
// the walk builds from the BMM type of an attribute the template gives no
// allowed child, such as the placeholder ELEMENT of an ITEM_LIST whose
// items the template requires and leaves empty. A server that checks a
// composition against its template can find no template node for either.
// So under Large and Medium the walk builds an optional node only where
// it holds more than such content: an optional child that needs it gets
// no member of its own, an attribute that is not required and whose
// members would all be undescribed or need undescribed content is not
// visited, and a top-up takes a child that needs none where the attribute
// has one. The check reads a node's own attributes only. Below a node the
// template or the RM requires, the walk builds what it always builds,
// undescribed members included, so the described content around them is
// kept.

// needsUndescribed reports whether node, a template child, needs content
// the template does not describe: node is a slot, or an attribute of node
// that is required (visitRequired) and that the walk does not skip can only
// be given undescribed members (onlyUndescribed). An attribute the BMM
// marks mandatory that node does not carry, as under
// templatecompile.WithoutImplicitAttributes, counts as one with no allowed
// child. A primitive leaf needs none: the walk values it and stops there.
// The answer does not depend on the policy, and it is kept per node for
// the call.
func (g *generator) needsUndescribed(node *tcimpl.CompiledNode) bool {
	if node.IsSlot() {
		return true
	}
	if v, seen := g.undescribed[node]; seen {
		return v
	}
	v := g.ownContentUndescribed(node)
	if g.undescribed == nil {
		g.undescribed = make(map[*tcimpl.CompiledNode]bool)
	}
	g.undescribed[node] = v
	return v
}

// ownContentUndescribed reports whether a required attribute of node, one
// it carries or one the BMM marks mandatory that it does not carry, can
// only be given undescribed members. It reads node's own attributes and
// does not look below their children.
func (g *generator) ownContentUndescribed(node *tcimpl.CompiledNode) bool {
	if node.PrimitiveConstraint() != nil {
		return false
	}
	for _, attr := range tcimpl.NodeAttributes(node) {
		if !visitRequired(attr) || walkSkips(node, attr) {
			continue
		}
		if g.onlyUndescribed(node, attr) {
			return true
		}
	}
	// The mandatory attributes node does not carry, which
	// fillUncarriedRequired builds from their BMM type.
	class := bmmtype.Class(concreteFor(strings.TrimSpace(node.RMTypeName())))
	for _, name := range requiredAttributes(class) {
		if node.Attribute(name) != nil || compositionDefaultOwns(class, name) {
			continue
		}
		declared, ok := rminfo.Default.AttributeRMType(class, name)
		if ok && g.buildsLocatable(concreteFor(attrType(class, declared))) {
			return true
		}
	}
	return false
}

// onlyUndescribed reports whether attr of node can only be given
// undescribed members: it has no allowed template child and the member the
// walk builds from its BMM type is a LOCATABLE, or every allowed template
// child of it is a slot.
func (g *generator) onlyUndescribed(node *tcimpl.CompiledNode, attr *tcimpl.CompiledAttribute) bool {
	children := tcimpl.AllowedChildNodes(attr)
	if len(children) == 0 {
		return g.bmmMemberLocatable(node, attr)
	}
	for _, c := range children {
		if !c.IsSlot() {
			return false
		}
	}
	return true
}

// membersUndescribed reports whether every member the walk would build for
// attr of node is undescribed or needs undescribed content. With no allowed
// template child, the member is built from the attribute's BMM type, which
// is undescribed when it is a LOCATABLE. A single-valued attribute takes
// one child (bindingAlternative), so its member is that child's. A
// multi-valued attribute builds members of each allowed child, so every
// one of them has to be a slot or need undescribed content.
func (g *generator) membersUndescribed(node *tcimpl.CompiledNode, attr *tcimpl.CompiledAttribute) bool {
	children := tcimpl.AllowedChildNodes(attr)
	if len(children) == 0 {
		return g.bmmMemberLocatable(node, attr)
	}
	if attr.Cardinality() == template.Single {
		first := children[0]
		if tcimpl.IsAOMPrimitiveShortName(first.RMTypeName()) && !attrTypedByRMClass(node, attr) {
			// A primitive the walk writes onto node itself.
			return false
		}
		return g.needsUndescribed(bindingAlternative(children, builtRMType(first)))
	}
	for _, c := range children {
		if !g.needsUndescribed(c) {
			return false
		}
	}
	return true
}

// bmmMemberLocatable reports whether the member the walk builds for attr of
// node, which has no allowed child, from the attribute's BMM type, would be
// a LOCATABLE: the type materialiseImplicitSingle or
// materialiseImplicitMultiple builds.
func (g *generator) bmmMemberLocatable(node *tcimpl.CompiledNode, attr *tcimpl.CompiledAttribute) bool {
	if attr.Cardinality() == template.Multiple {
		return g.buildsLocatable(attr.RMTypeName())
	}
	return g.buildsLocatable(attrType(node.RMTypeName(), attr.RMTypeName()))
}

// buildsLocatable reports whether the value the generator builds for the
// RM type declared (newRMIfKnown) is a LOCATABLE. A type it builds nothing
// for, such as a BMM primitive, is not. The answer is kept per type for
// the call.
func (g *generator) buildsLocatable(declared string) bool {
	if declared == "" || declared == "String" {
		return false
	}
	if v, seen := g.locatableTypes[declared]; seen {
		return v
	}
	v, known := newRMIfKnown(declared)
	_, locatable := v.(rm.Locatable)
	locatable = known && locatable
	if g.locatableTypes == nil {
		g.locatableTypes = make(map[string]bool)
	}
	g.locatableTypes[declared] = locatable
	return locatable
}

// optionalUndescribed reports whether child is optional (childRequired) and
// needs content the template does not describe. Under Large and Medium such
// a child gets no member of its own.
func (g *generator) optionalUndescribed(child *tcimpl.CompiledNode) bool {
	return !childRequired(child) && g.needsUndescribed(child)
}

// firstDescribed returns the first of children that needs no content the
// template does not describe, or nil when every one does. A slot always
// does.
func (g *generator) firstDescribed(children []*tcimpl.CompiledNode) *tcimpl.CompiledNode {
	for _, c := range children {
		if !g.needsUndescribed(c) {
			return c
		}
	}
	return nil
}
