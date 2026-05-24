package templatecompile

import (
	"errors"
	"fmt"
	"slices"

	"github.com/cadasto/openehr-sdk-go/openehr/template"
)

// Compiled is a pre-processed, walker-friendly representation of an
// OPT. Each node carries its AQL path (computed once at compile
// time), implicit RM attributes injected from rminfo, and a back
// pointer to its parent. Construct via [Compile]; the zero value is
// not useful.
type Compiled struct {
	templateID string
	concept    string
	uid        string
	language   string
	root       *CompiledNode

	// Indexes built once during compile. byPath maps every node's
	// canonical AQL path string to the node; byNodeID groups nodes
	// by archetype node id (at-code); byRMType groups nodes by RM
	// type name. All three are non-nil after a successful compile.
	byPath   map[string]*CompiledNode
	byNodeID map[string][]*CompiledNode
	byRMType map[string][]*CompiledNode

	// Flat list of every term-binding record reached during compile,
	// in depth-first OPT order. Per-archetype-root term *definitions*
	// stay attached to their CompiledNode (see [CompiledNode.Term])
	// because at-codes are scoped to the enclosing archetype root —
	// flattening to a single map would collide (e.g. at0004 means
	// "Systolic" inside blood_pressure but "Rate" inside heart_rate).
	termBindings []template.TermBinding
}

// TemplateID returns the OPT's template id (e.g. "vital_signs").
func (c *Compiled) TemplateID() string { return c.templateID }

// Concept returns the OPT's concept (machine-readable concept slug).
func (c *Compiled) Concept() string { return c.concept }

// UID returns the OPT's uid value, or "" when absent.
func (c *Compiled) UID() string { return c.uid }

// Language returns the OPT's primary language code (ISO 639-1).
func (c *Compiled) Language() string { return c.language }

// Root returns the root compiled node. Never nil after a successful
// Compile.
func (c *Compiled) Root() *CompiledNode { return c.root }

// NodeAt resolves a path string by exact lookup in the pre-built
// byPath index (O(1)). Keys are fully qualified AQL paths computed
// at compile time — e.g. "/content[openEHR-EHR-OBSERVATION.blood_pressure.v1]",
// not predicate-less prefixes like "/content".
//
// This differs from [template.OperationalTemplate.NodeAt], which
// walks the wire tree and applies lenient first-child rules for
// predicate-less segments on multi-cardinality attributes. Use
// [CompiledNode.AQLPath] or wire NodeAt when you need tree-walk
// semantics; use NodeAt here when you already hold a canonical path.
//
// Returns [ErrPathNotFound] when the path string is not indexed.
func (c *Compiled) NodeAt(path string) (*CompiledNode, error) {
	n, ok := c.byPath[path]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrPathNotFound, path)
	}
	return n, nil
}

// AllByRMType returns every compiled node whose RMTypeName equals
// rm. Returned slice is a defensive copy; safe to mutate by caller.
// Iteration order is OPT document order (depth-first).
func (c *Compiled) AllByRMType(rm string) []*CompiledNode {
	return slices.Clone(c.byRMType[rm])
}

// AllByNodeID returns every compiled node whose NodeID equals
// nodeID (an at-code). Same iteration / copy semantics as
// AllByRMType.
func (c *Compiled) AllByNodeID(nodeID string) []*CompiledNode {
	return slices.Clone(c.byNodeID[nodeID])
}

// NumNodes returns the total number of unique [CompiledNode] entries
// reachable in the compiled tree (the size of the byPath index built
// during [Compile]). Useful as an independent "truth count" against
// which traversal code in [internal/templatecompile/walk] can assert
// it visited every node — comparing a walker's tally to this value
// detects subtree-pruning bugs that a second walker call cannot.
func (c *Compiled) NumNodes() int { return len(c.byPath) }

// Term looks up the at-code's term definition under the root
// archetype's terminology. Equivalent to [CompiledNode.Term] called
// on the root — convenience for callers operating at the
// COMPOSITION level. Returns (zero, false) when the code is not
// defined on the root archetype.
//
// Note: at-codes are scoped to their enclosing archetype root; the
// same code can have different meanings under different roots
// (e.g. at0004 = "Systolic" under blood_pressure but "Rate" under
// heart_rate). Use [CompiledNode.Term] for context-sensitive
// lookup, or [Compiled.NodeAt] to position first.
func (c *Compiled) Term(code string) (template.ArchetypeTerm, bool) {
	if c.root == nil {
		return template.ArchetypeTerm{}, false
	}
	return c.root.Term(code)
}

// TermBindings returns a defensive copy of every term-binding
// record reachable in the OPT, flattened across archetype roots.
// Order matches the depth-first walk of archetype roots.
func (c *Compiled) TermBindings() []template.TermBinding {
	return slices.Clone(c.termBindings)
}

// ErrPathNotFound is returned by [Compiled.NodeAt] when the path
// string does not resolve to a compiled node. Distinct from
// [template.ErrPathNotFound] — the compiled API operates over
// pre-computed AQL paths, so the resolution semantics differ
// (exact-match lookup, not tree-walk).
var ErrPathNotFound = errors.New("templatecompile: path not found in compiled template")
