package composition

import (
	"fmt"
	"strings"

	tcimpl "github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/internal/rmbind"
)

// bindRMType returns the node, among those the template allows at the
// compiled node's path ([alternativesAt]), that v binds, or
// ErrTypeMismatch (wrapped with diagnostic context) when v binds none.
//
// It binds as the validator does, by the one rule both read
// ([rmbind.Bind]), so Set accepts exactly the values the validator binds
// at the path. The rule tries three tiers in order and returns the first
// node, in OPT order, of the first tier that admits v:
//
//  1. a node that names v's own RM type, a typed DV_INTERVAL<T> included;
//  2. a node that names the AOM 1.4 primitive short name whose canonical
//     DV wrapper v is: a DV_DURATION binds a DURATION node, the value the
//     generator writes there;
//  3. a node v's type descends from in the pinned class hierarchy (a
//     DV_CODED_TEXT at DV_TEXT, a DV_QUANTITY at DATA_VALUE, a
//     POINT_EVENT at EVENT), a node the OPT leaves untyped, or a typed
//     DV_INTERVAL<T> node each present bound of which conforms to T (a
//     DV_INTERVAL<DV_COUNT> at DV_INTERVAL<DV_ORDERED>).
//
// So under DV_TEXT then DV_CODED_TEXT, a DV_CODED_TEXT binds the
// DV_CODED_TEXT node. Conformance is read from the classes the compile
// found to conform to each node's type, so this package imports no class
// hierarchy of its own.
//
// v must be an RM value: rmbind.TypeName names it, with the ITS-JSON
// parameterised name for a typed DV_INTERVAL, and a value it cannot name
// fails as unrecognised. That covers a value that is no RM type, such as a
// Go string the validator would bind at a DURATION leaf, and an interface
// carrying a typed-nil pointer: neither is a usable value at a builder Set
// site.
func bindRMType(node *tcimpl.CompiledNode, v any) (*tcimpl.CompiledNode, error) {
	got, ok := rmbind.TypeName(v)
	if !ok {
		return nil, fmt.Errorf("%w: value type %T is not a recognised RM concrete", ErrTypeMismatch, v)
	}
	alternatives := alternativesAt(node)
	if bound := rmbind.Bind(alternatives, v, anyNode); bound != nil {
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
