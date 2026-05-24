package walk

import (
	"errors"
	"fmt"
	"slices"

	"github.com/cadasto/openehr-sdk-go/internal/templatecompile"
)

// Visitor receives each node twice during a Walk: pre-order before
// any child is visited, then post-order after every child has been
// visited.
//
// PreHandle returning [SkipSubtree] prunes the subtree (no children
// visited, PostHandle not fired) while letting sibling traversal
// continue. Any other non-nil error aborts the walk. PostHandle's
// error semantics are simpler: a non-nil return aborts.
type Visitor interface {
	PreHandle(ctx *Context) error
	PostHandle(ctx *Context) error
}

// VisitorFunc is a convenience adapter for visitors that only care
// about one of the two phases. The unused phase is a no-op. Use as:
//
//	walk.Walk(c, walk.VisitorFunc{Pre: func(ctx *walk.Context) error { ... }})
type VisitorFunc struct {
	// Pre, when non-nil, runs in PreHandle. Nil means "no-op
	// pre-handler".
	Pre func(ctx *Context) error
	// Post, when non-nil, runs in PostHandle. Nil means "no-op
	// post-handler".
	Post func(ctx *Context) error
}

// PreHandle implements Visitor; delegates to f.Pre when set.
func (f VisitorFunc) PreHandle(ctx *Context) error {
	if f.Pre == nil {
		return nil
	}
	return f.Pre(ctx)
}

// PostHandle implements Visitor; delegates to f.Post when set.
func (f VisitorFunc) PostHandle(ctx *Context) error {
	if f.Post == nil {
		return nil
	}
	return f.Post(ctx)
}

// SkipSubtree is the sentinel error returned from [Visitor.PreHandle]
// to prune the subtree rooted at the current node. The walk
// continues with the next sibling; PostHandle is NOT fired for the
// pruned node (the convention is "post-handle = after the subtree
// is processed", and a pruned subtree was not processed).
//
// The name intentionally omits the "Err" prefix to mirror
// [filepath.SkipDir] / [filepath.SkipAll], which use the same
// signalling-via-sentinel pattern rather than denoting a true error.
var SkipSubtree = errors.New("walk: skip subtree") //nolint:staticcheck // ST1012: sentinel control value, see comment above

// Context carries the current walk position. It is rebuilt for each
// visited node — callers MUST NOT retain pointers past the visitor
// call; copy any fields they need.
type Context struct {
	node            *templatecompile.CompiledNode
	parent          *templatecompile.CompiledNode
	parentAttribute *templatecompile.CompiledAttribute
	depth           int
}

// Node returns the current compiled node. Never nil during a
// visitor call.
func (c *Context) Node() *templatecompile.CompiledNode { return c.node }

// Parent returns the immediate parent compiled node, or nil for the
// root.
func (c *Context) Parent() *templatecompile.CompiledNode { return c.parent }

// ParentAttribute returns the compiled attribute the current node
// hangs off, or nil for the root. Useful for visitors that need to
// know which RM attribute name the current node fills (e.g.
// "category", "content").
func (c *Context) ParentAttribute() *templatecompile.CompiledAttribute {
	return c.parentAttribute
}

// Path returns the canonical AQL path of the current node, cached
// at compile time.
func (c *Context) Path() string { return c.node.AQLPath() }

// Depth is the 0-based distance from the walk's starting node.
// The root of a [Walk] call is depth 0; its children are 1; etc.
func (c *Context) Depth() int { return c.depth }

// Walk performs a depth-first walk over c's compiled tree starting
// at the root. See package doc for full semantics.
//
// Returns ErrInvalidInput when c or v is nil; returns any non-nil
// non-[SkipSubtree] error surfaced by the visitor.
func Walk(c *templatecompile.Compiled, v Visitor) error {
	if c == nil {
		return fmt.Errorf("%w: nil compiled template", ErrInvalidInput)
	}
	if v == nil {
		return fmt.Errorf("%w: nil visitor", ErrInvalidInput)
	}
	if c.Root() == nil {
		return fmt.Errorf("%w: compiled template has no root", ErrInvalidInput)
	}
	return walk(c.Root(), nil, nil, 0, v)
}

// WalkSubtree starts the walk at the node addressed by startPath
// (an AQL path string as cached by [templatecompile.CompiledNode.AQLPath]).
// The start node itself is visited as depth 0; its parent context
// (Parent / ParentAttribute on [Context]) is derived from
// [templatecompile.CompiledNode.Parent], so visitors can still walk
// upward if needed.
//
// Returns ErrInvalidInput for nil arguments; returns the underlying
// [templatecompile.ErrPathNotFound] wrapped when startPath does not
// resolve.
func WalkSubtree(c *templatecompile.Compiled, startPath string, v Visitor) error {
	if c == nil {
		return fmt.Errorf("%w: nil compiled template", ErrInvalidInput)
	}
	if v == nil {
		return fmt.Errorf("%w: nil visitor", ErrInvalidInput)
	}
	start, err := c.NodeAt(startPath)
	if err != nil {
		return fmt.Errorf("walk: resolve %q: %w", startPath, err)
	}
	return walk(start, start.Parent(), parentAttributeOf(start), 0, v)
}

// ErrInvalidInput is returned for nil Compiled / Visitor arguments
// or for an unrooted Compiled tree.
var ErrInvalidInput = errors.New("walk: invalid input")

// walk is the recursive driver. Separate from Walk / WalkSubtree
// so both entry points share the same descent semantics.
func walk(
	node, parent *templatecompile.CompiledNode,
	parentAttr *templatecompile.CompiledAttribute,
	depth int,
	v Visitor,
) error {
	if node == nil {
		return nil
	}
	ctx := &Context{
		node:            node,
		parent:          parent,
		parentAttribute: parentAttr,
		depth:           depth,
	}
	if err := v.PreHandle(ctx); err != nil {
		if errors.Is(err, SkipSubtree) {
			return nil
		}
		return err
	}
	// *Slot leaves carry no descendable children — their slot-fill
	// semantics are opaque to the walker (REQ-104 surfaces them).
	if !node.IsSlot() {
		for _, attr := range node.Attributes() {
			for _, child := range attr.Children() {
				if err := walk(child, node, attr, depth+1, v); err != nil {
					return err
				}
			}
		}
	}
	return v.PostHandle(ctx)
}

// parentAttributeOf returns the [CompiledAttribute] under whose
// Children the given node hangs, or nil for the root. Used by
// WalkSubtree to seed the Context.ParentAttribute correctly.
func parentAttributeOf(n *templatecompile.CompiledNode) *templatecompile.CompiledAttribute {
	parent := n.Parent()
	if parent == nil {
		return nil
	}
	for _, attr := range parent.Attributes() {
		if slices.Contains(attr.Children(), n) {
			return attr
		}
	}
	return nil
}
