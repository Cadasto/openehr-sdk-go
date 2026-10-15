package composition

import (
	"fmt"
	"strings"

	tcimpl "github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/internal/rmnames"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// bindRMType returns the node, among those the template allows at the
// compiled node's path ([alternativesAt]), whose RM type v's RM type
// conforms to, or ErrTypeMismatch (wrapped with diagnostic context) when
// there is none. The Go→RM-name reverse mapping is the generated
// rm.RMTypeName (ADR 0013). REQ-024, no reflection.
//
// v conforms to a node when its RM type is the node's type, or a class
// that descends from it in the pinned class hierarchy (REQ-101 Type
// enforcement, read the way the REQ-102 validator binds an alternative):
// a DV_CODED_TEXT conforms to a DV_TEXT node, and a concrete class to a
// node the OPT types as an abstract class, such as a DV_QUANTITY at
// DATA_VALUE or a POINT_EVENT at EVENT. tcimpl.ConformsTo answers it from
// the classes the compile found to conform to the node's type, so this
// package imports no class hierarchy of its own (REQ-013).
//
// As the validator does, bindRMType prefers a node that names v's own
// type to one v's type only descends from, and within each, the first in
// OPT order: under DV_TEXT then DV_CODED_TEXT, a DV_CODED_TEXT binds the
// DV_CODED_TEXT node.
//
// A typed DV_INTERVAL node declares the ITS-JSON parameterised name
// (DV_INTERVAL<DV_QUANTITY>), which is no class the hierarchy knows; the
// registry reverses only the bare name, so rmnames.TypedIntervalName
// supplies the parameterised one, and the interval names the node's type
// when its parameter is the node's own. An interface carrying a typed-nil
// pointer fails as unrecognised: a typed-nil is not a usable value at a
// builder Set site.
func bindRMType(node *tcimpl.CompiledNode, v any) (*tcimpl.CompiledNode, error) {
	got, ok := rm.RMTypeName(v)
	if !ok {
		return nil, fmt.Errorf("%w: value type %T is not a recognised RM concrete", ErrTypeMismatch, v)
	}
	param, typed := rmnames.TypedIntervalName(v)
	alternatives := alternativesAt(node)
	for _, alt := range alternatives {
		if want := alt.RMTypeName(); want == got || (typed && want == param) {
			return alt, nil
		}
	}
	for _, alt := range alternatives {
		if conforms, known := tcimpl.ConformsTo(alt, got); known && conforms {
			return alt, nil
		}
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

// alternativesAt returns, in OPT order, the nodes the template allows at
// node's path: node and every other child of node's parent attribute at
// that same path, the alternatives an OPT lists under one attribute,
// leaving out a child whose occurrences are 0..0
// ([tcimpl.AllowedChildNodes]). Compile indexes only the first of them
// under the path, so Compiled.NodeAt returns that one alone. The root,
// which has no parent attribute, is its own only alternative.
func alternativesAt(node *tcimpl.CompiledNode) []*tcimpl.CompiledNode {
	parent := node.Parent()
	if parent == nil {
		return []*tcimpl.CompiledNode{node}
	}
	attr, _ := findAttributeContaining(parent, node)
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
