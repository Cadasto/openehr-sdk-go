package templatecompile

import (
	"fmt"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/template"
)

// ResolveOption tunes [Compiled.Resolve] and [Compiled.ValidatePath].
// Construct one with [WithStrictPaths]. Options compose; giving the same
// option twice is the same as giving it once, and a nil option is ignored.
type ResolveOption func(*resolveOptions)

type resolveOptions struct {
	strictPaths bool
}

// WithStrictPaths makes [Compiled.Resolve] refuse a segment without a
// predicate on an attribute that holds more than one child, with an error
// wrapping [template.ErrAmbiguousPath], where by default it takes the first
// child. Use it where a guess would hide a mistake, as in a validator or a
// code generator.
func WithStrictPaths() ResolveOption {
	return func(o *resolveOptions) { o.strictPaths = true }
}

// Resolve walks the compiled tree from the root along p, a path that
// [template.ParsePath] reads, and returns the node p reaches. The root path
// returns the root.
//
// Each segment of p names an attribute of the node reached so far: a declared
// attribute, or an implicit one, which holds no child. A segment's predicate
// selects the first child whose node id or, for an archetype root, archetype
// id equals it. A segment without a predicate takes the attribute's first
// child, in document order; with [WithStrictPaths] it is refused where the
// attribute holds more than one child. A slot does not descend.
//
// Resolve walks the tree, so a predicate reaches an alternative under a
// single-valued attribute by its node id, where [Compiled.NodeAt], which
// looks up the exact AQL path the compile wrote, returns the first of the
// alternatives that share that path.
//
// A path that reaches no node, through an attribute the node does not have,
// an attribute with no children, a predicate no child matches or a slot, and
// a path more than 128 levels deep, gives an error that wraps both
// [ErrPathNotFound] and [template.ErrPathNotFound]. An ambiguous segment under
// [WithStrictPaths] gives an error wrapping [template.ErrAmbiguousPath].
func (c *Compiled) Resolve(p template.Path, opts ...ResolveOption) (*CompiledNode, error) {
	if c == nil || c.root == nil {
		return nil, fmt.Errorf("%w: empty template", errNoNode)
	}
	var o resolveOptions
	for _, opt := range opts {
		if opt != nil {
			opt(&o)
		}
	}
	n := c.root
	for i, seg := range segmentsOf(p) {
		// Compile builds no node deeper than maxDepth, but a walk bounds
		// itself all the same (REQ-108): the node a segment reaches lies
		// i+1 levels below the root.
		if i >= maxDepth {
			return nil, fmt.Errorf("%w: path nesting exceeds %d levels", errNoNode, maxDepth)
		}
		if n.slot != nil {
			return nil, fmt.Errorf("%w: cannot descend into the slot at %q", errNoNode, n.aqlPath)
		}
		attr := n.Attribute(seg.name)
		if attr == nil {
			return nil, fmt.Errorf("%w: attribute %q", errNoNode, seg.name)
		}
		if len(attr.children) == 0 {
			return nil, fmt.Errorf("%w: attribute %q has no children", errNoNode, seg.name)
		}
		if seg.predicate == "" {
			if o.strictPaths && len(attr.children) > 1 {
				return nil, fmt.Errorf("%w: attribute %q has %d candidates (add a predicate)",
					template.ErrAmbiguousPath, seg.name, len(attr.children))
			}
			n = attr.children[0]
			continue
		}
		n = childMatching(attr.children, seg.predicate)
		if n == nil {
			return nil, fmt.Errorf("%w: predicate %q under %q", errNoNode, seg.predicate, seg.name)
		}
	}
	return n, nil
}

// ValidatePath reports whether p resolves on the compiled tree: it is
// [Compiled.Resolve] without the node, and returns the same errors.
func (c *Compiled) ValidatePath(p template.Path, opts ...ResolveOption) error {
	_, err := c.Resolve(p, opts...)
	return err
}

// childMatching returns the first of children whose node id or archetype id
// is predicate, which is not empty, or nil when none is.
func childMatching(children []*CompiledNode, predicate string) *CompiledNode {
	for _, child := range children {
		if child.nodeID == predicate || child.archetypeID == predicate {
			return child
		}
	}
	return nil
}

// noNode is the cause of every error Resolve returns for a path that reaches
// no node. It reads as ErrPathNotFound and matches both ErrPathNotFound and
// template.ErrPathNotFound, so a caller branches on the sentinel of the
// lookup it knows.
type noNode struct{}

func (noNode) Error() string   { return ErrPathNotFound.Error() }
func (noNode) Unwrap() []error { return []error{ErrPathNotFound, template.ErrPathNotFound} }

var errNoNode error = noNode{}

// pathSegment is one segment of a parsed path: an attribute name and the
// predicate that selects a child, "" when the segment has none.
type pathSegment struct {
	name, predicate string
}

// segmentsOf returns the segments of p, nil for the root path. A
// [template.Path] keeps its segments to itself, so they are read from its
// canonical text, in which a name holds no '/', '[' or ']' and a predicate
// holds no ']'.
func segmentsOf(p template.Path) []pathSegment {
	if p.IsRoot() {
		return nil
	}
	var segs []pathSegment
	for s := p.String(); s != ""; {
		s = s[1:] // the slash that opens the segment
		end := strings.IndexAny(s, "/[")
		if end < 0 {
			segs = append(segs, pathSegment{name: s})
			break
		}
		seg := pathSegment{name: s[:end]}
		s = s[end:]
		if s[0] == '[' {
			closing := strings.IndexByte(s, ']')
			if closing < 0 {
				closing = len(s)
			}
			seg.predicate = s[1:closing]
			s = s[min(closing+1, len(s)):]
		}
		segs = append(segs, seg)
	}
	return segs
}
