package composition

import (
	"fmt"
	"strings"

	"github.com/cadasto/openehr-sdk-go/internal/bmmtype"
	tcimpl "github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/internal/rmbind"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/rminfo"
)

// bindRMType returns the node, among those the template allows at the
// compiled node's path ([alternativesAt]), that v binds, or
// ErrTypeMismatch (wrapped with diagnostic context) when v binds none, or
// when the path writes no RM attribute or one that cannot hold v
// ([attributeHolds]).
//
// It binds by the tier rule ([rmbind.Bind]), which tries three tiers in
// order and returns the first node, in OPT order, of the first tier that
// admits v. The validator tries that node first; where the tier holds more
// nodes, it binds v to the first of them under whose constraints v
// validates, so it can bind v to a later node than this one. The tiers:
//
//  1. a node that names v's own RM type, a typed DV_INTERVAL<T> included;
//  2. a node that names the AOM 1.4 primitive short name whose canonical
//     DV wrapper v is: a DV_DURATION binds a DURATION node;
//  3. a node v's type descends from in the pinned class hierarchy (a
//     DV_CODED_TEXT at DV_TEXT, a DV_QUANTITY at DATA_VALUE, a
//     POINT_EVENT at EVENT), a node the OPT leaves untyped, or a typed
//     DV_INTERVAL<T> node each present bound of which conforms to T (a
//     DV_INTERVAL<DV_COUNT> at DV_INTERVAL<DV_ORDERED>).
//
// So under DV_TEXT then DV_CODED_TEXT, a DV_CODED_TEXT binds the
// DV_CODED_TEXT node. The binding reads conformance from the classes the
// compile found to conform to each node's type.
//
// Set accepts less than the rule binds, in two ways. v must be an RM
// value: rmbind.TypeName names it, with the ITS-JSON parameterised name
// for a typed DV_INTERVAL, and a value it cannot name fails as
// unrecognised. That covers a value that is no RM type, such as a Go
// string the validator would bind at a DURATION leaf, and an interface
// carrying a typed-nil pointer: neither is a usable value at a builder
// Set site. And the path must write an RM attribute that can hold v,
// which the rule does not ask: a DV_DURATION binds the DURATION node
// under a DV_DURATION's value, but that attribute holds a String.
func bindRMType(node *tcimpl.CompiledNode, v any) (*tcimpl.CompiledNode, error) {
	got, ok := rmbind.TypeName(v)
	if !ok {
		return nil, fmt.Errorf("%w: value type %T is not a recognised RM concrete", ErrTypeMismatch, v)
	}
	// Every alternative sits under node's parent attribute, so one search
	// finds the attribute for both the alternatives and the attribute check.
	var attr *tcimpl.CompiledAttribute
	if parent := node.Parent(); parent != nil {
		attr = findAttributeContaining(parent, node)
	}
	alternatives := alternativesAt(node, attr)
	if bound := rmbind.Bind(alternatives, v, anyNode); bound != nil {
		if err := attributeHolds(bound, attr, v, got); err != nil {
			return nil, err
		}
		return bound, nil
	}
	switch len(alternatives) {
	case 0:
		return nil, fmt.Errorf("%w: value type %T (RM %q): the template allows no value at the path, since every node there has occurrences 0..0",
			ErrTypeMismatch, v, got)
	case 1:
		return nil, fmt.Errorf("%w: value type %T (RM %q) does not conform to path RM type %q",
			ErrTypeMismatch, v, got, alternatives[0].RMTypeName())
	default:
		names := make([]string, len(alternatives))
		for i, alt := range alternatives {
			names[i] = alt.RMTypeName()
		}
		return nil, fmt.Errorf("%w: value type %T (RM %q) conforms to none of the path RM types [%s]",
			ErrTypeMismatch, v, got, strings.Join(names, ", "))
	}
}

// anyNode admits every node to the binding: a Set value may bind any node
// the template allows at the path, a slot included.
func anyNode(*tcimpl.CompiledNode) bool { return true }

// attributeHolds returns nil when the RM attribute that node sits under,
// attr, can hold v, of RM type got; otherwise it returns ErrTypeMismatch,
// wrapped with the attribute and the type it holds. The pinned RM
// metadata ([rminfo.Default]) gives the attribute and its type.
//
// The attribute's type is the one the metadata gives the attribute,
// declared or inherited, on the class of node's parent: the type of its
// items for a container. Where that class types the attribute with a
// formal generic parameter, the parent's type name supplies the actual
// parameter ([bmmtype.Substitute]), so the lower bound of a
// DV_INTERVAL<DV_COUNT> holds a DV_COUNT. Where the name leaves the
// parameter open, the parameter's constraint takes its place
// ([bmmtype.ParameterConstraint]), so the lower bound of a bare
// DV_INTERVAL holds a DV_ORDERED, and the data of a bare POINT_EVENT an
// ITEM_STRUCTURE. The attribute can hold v when got is that type or
// descends from it in the pinned class hierarchy ([canHold]).
//
// A path at which the metadata knows the parent's class, but that class
// has no attribute of the name the template gives, writes no RM
// attribute, and is refused too: a template may constrain a BMM function,
// such as DV_PROPORTION.is_integral, as if it were an attribute.
//
// There is nothing to test against, and so no refusal, at the root, which
// sits under no attribute, or where attr is nil; under a parent the
// template leaves untyped or whose class the metadata does not know; and
// where the attribute's type is no class the hierarchy knows and no plain
// value type ([plainValueTypes]): Any, or a formal parameter left open
// that the schemas set no constraint, such as the T of VERSION.
func attributeHolds(node *tcimpl.CompiledNode, attr *tcimpl.CompiledAttribute, v any, got string) error {
	parent := node.Parent()
	if parent == nil || attr == nil {
		return nil
	}
	owner := parent.RMTypeName()
	if owner == "" {
		return nil
	}
	class := bmmtype.Class(owner)
	declared, ok := rminfo.Default.AttributeRMType(class, attr.Name())
	if !ok {
		if !classKnown(class) {
			return nil
		}
		return fmt.Errorf("%w: value type %T (RM %q): the path writes %q, which %s does not have as an RM attribute",
			ErrTypeMismatch, v, got, attr.Name(), owner)
	}
	held := bmmtype.Substitute(owner, declared)
	constraint, open := bmmtype.ParameterConstraint(class, held)
	open = open && constraint != ""
	target := held
	if open {
		target = constraint
	}
	if holds, known := canHold(target, got); holds || !known {
		return nil
	}
	what := held
	if open {
		what = fmt.Sprintf("its generic parameter %s, a %s", held, constraint)
	}
	return fmt.Errorf("%w: value type %T (RM %q): the path writes attribute %q of %s, which holds %s",
		ErrTypeMismatch, v, got, attr.Name(), owner, what)
}

// classKnown reports whether the pinned RM metadata knows class.
func classKnown(class string) bool {
	h, ok := rminfo.Default.(rminfo.Hierarchy)
	if !ok {
		return false
	}
	_, known := h.IsAbstract(class)
	return known
}

// canHold reports whether an attribute of RM type held can hold a value of
// RM type got: got is held or descends from it in the pinned class
// hierarchy, each looked up by its class, without generic parameters. An
// attribute of a plain value type ([plainValueTypes]) holds no RM value.
// known is false when held is no plain value type and the hierarchy does
// not know held's class or got's. It answers as the hierarchy of
// [rminfo.Default] does, from tables built once rather than a walk of the
// hierarchy on each call ([tcimpl.ClassConformsTo]).
func canHold(held, got string) (holds, known bool) {
	if plainValueTypes[held] {
		return false, true
	}
	return tcimpl.ClassConformsTo(bmmtype.Class(got), bmmtype.Class(held))
}

// plainValueTypes lists the BASE primitive types whose values are plain:
// a string, a number, a boolean, a character or a byte. No class an RM
// value has descends from one, so an attribute of one of these types
// holds no RM value. The pinned metadata knows them as primitive types,
// not as classes, and so does not answer conformance to them. The other
// BASE primitive types are left out, because RM classes descend from some
// of them in the BMM, as every class does from Any and DV_DURATION does
// from Iso8601_duration, and the pinned class hierarchy drops those edges.
var plainValueTypes = map[string]bool{
	"Boolean":   true,
	"Character": true,
	"Double":    true,
	"Integer":   true,
	"Integer64": true,
	"Octet":     true,
	"Real":      true,
	"String":    true,
}

// alternativesAt returns, in OPT order, the nodes the template allows at
// node's path: node and every other child of node's parent attribute,
// attr, at that same path, the alternatives an OPT lists under one
// attribute, leaving out a child whose occurrences are 0..0
// ([tcimpl.AllowedChildNodes]). Compile indexes only the first of them
// under the path, so Compiled.NodeAt returns that one alone. The root,
// which has no parent attribute, is its own only alternative, and so is a
// node whose attr is nil.
func alternativesAt(node *tcimpl.CompiledNode, attr *tcimpl.CompiledAttribute) []*tcimpl.CompiledNode {
	if attr == nil {
		return []*tcimpl.CompiledNode{node}
	}
	var alternatives []*tcimpl.CompiledNode
	for _, c := range tcimpl.AllowedChildNodes(attr) {
		if c.AQLPath() == node.AQLPath() {
			alternatives = append(alternatives, c)
		}
	}
	return alternatives
}
