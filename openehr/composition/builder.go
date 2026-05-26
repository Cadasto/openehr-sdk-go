package composition

import (
	"context"
	"errors"
	"fmt"

	"github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/internal/templateinstance/rmwrite"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/validation/rmread"
)

// Builder accumulates path assignments over a skeleton. Returned by
// [NewBuilder]; finalised via [Builder.Build].
type Builder struct {
	compiled *templatecompile.Compiled
	skeleton *rm.Composition

	// pending captures every Set call in order; Build applies each
	// in turn and aggregates errors.
	pending []pendingAssignment

	// errs accumulates errors produced by Set / Build. Build joins
	// them via errors.Join so the caller sees every failed path in
	// one round-trip.
	errs []error
}

// pendingAssignment is one queued (path, value) pair plus the
// resolved compiled node. Pre-resolving the node at Set time means
// path-not-found is surfaced eagerly; type-checking and RM-side
// navigation still happen at Build time so the caller can stack Set
// calls without each one mutating the graph.
type pendingAssignment struct {
	path     string
	rawValue any
	node     *templatecompile.CompiledNode
}

// NewBuilder constructs a Builder seeded with NewSkeleton output.
// Composition options (WithComposer, WithTerritory, …) are forwarded
// to the underlying instance.Generate call. The Builder retains the
// compiled template for path lookups.
func NewBuilder(ctx context.Context, c *templatecompile.Compiled, opts ...Option) (*Builder, error) {
	if c == nil {
		return nil, fmt.Errorf("composition.NewBuilder: nil compiled template")
	}
	skel, err := NewSkeleton(ctx, c, opts...)
	if err != nil {
		return nil, err
	}
	return &Builder{
		compiled: c,
		skeleton: skel,
	}, nil
}

// TemplateID returns the OPT template id, suitable for the REST
// composition.WithTemplateID option.
func (b *Builder) TemplateID() string {
	if b == nil || b.compiled == nil {
		return ""
	}
	return b.compiled.TemplateID()
}

// Set assigns v at path. Path must resolve in the compiled template;
// v must match the compiled-node RM type. Errors are accumulated and
// surfaced from Build — Set never short-circuits the builder so the
// caller can stack assignments and recover every faulty path in one
// round-trip.
//
// Returns the same error it stored, primarily for callers that want
// to react to the first failure inline.
func (b *Builder) Set(path string, v any) error {
	if b == nil {
		return fmt.Errorf("composition: nil Builder")
	}
	if v == nil {
		err := fmt.Errorf("%w: %s: nil value", ErrTypeMismatch, path)
		b.errs = append(b.errs, err)
		return err
	}
	node, err := b.compiled.NodeAt(path)
	if err != nil {
		wrapped := fmt.Errorf("%w: %s", ErrUnknownPath, path)
		b.errs = append(b.errs, wrapped)
		return wrapped
	}
	if e := checkRMType(node, v); e != nil {
		wrapped := fmt.Errorf("%s: %w", path, e)
		b.errs = append(b.errs, wrapped)
		return wrapped
	}
	b.pending = append(b.pending, pendingAssignment{
		path:     path,
		rawValue: v,
		node:     node,
	})
	return nil
}

// SetText assigns &rm.DVText{Value: value} at path. Path must
// resolve to a DV_TEXT node in the OPT.
func (b *Builder) SetText(path, value string) error {
	return b.Set(path, &rm.DVText{Value: value})
}

// SetQuantity assigns a *rm.DVQuantity at path. Path must resolve
// to a DV_QUANTITY node in the OPT.
func (b *Builder) SetQuantity(path string, magnitude float64, units string) error {
	return b.Set(path, &rm.DVQuantity{
		Magnitude: rm.Real(magnitude),
		Units:     units,
	})
}

// SetCodedText assigns a *rm.DVCodedText at path. Path must resolve
// to a DV_CODED_TEXT node in the OPT.
func (b *Builder) SetCodedText(path, terminology, code, display string) error {
	return b.Set(path, &rm.DVCodedText{
		DVText: rm.DVText{Value: display},
		DefiningCode: rm.CodePhrase{
			CodeString:    code,
			TerminologyID: rm.TerminologyID{Value: terminology},
		},
	})
}

// Build finalises the graph. Pending assignments are applied in
// order against the skeleton; per-path failures accumulate via
// errors.Join. The returned *rm.Composition is the same skeleton
// instance — Build mutates it in place.
func (b *Builder) Build() (*rm.Composition, error) {
	if b == nil {
		return nil, fmt.Errorf("composition: nil Builder")
	}
	for _, p := range b.pending {
		if err := b.applyAssignment(p); err != nil {
			b.errs = append(b.errs, fmt.Errorf("%s: %w", p.path, err))
		}
	}
	if len(b.errs) > 0 {
		return b.skeleton, errors.Join(b.errs...)
	}
	return b.skeleton, nil
}

// applyAssignment navigates the skeleton along the compiled path's
// parent chain, then routes the assignment through rmwrite.EnsureSingle
// (single-attr leaves) or AppendMultiple (when the path resolves to
// a multi-attr container element — v1 leaves that path to a future
// follow-up since the common authoring case is leaf assignment).
func (b *Builder) applyAssignment(p pendingAssignment) error {
	parent := p.node.Parent()
	if parent == nil {
		// Root assignment — supplied value replaces the composition.
		// v1 does not support this (the skeleton root is the
		// builder's invariant); error rather than silently swap.
		return fmt.Errorf("%w: cannot Set the composition root", ErrInvalidPath)
	}
	// Walk the parent chain from compiled root → parent collecting
	// (CompiledNode, attrName, predicate) segments. The leaf segment
	// supplies the attribute on `parent` to write into.
	segs, err := buildSegments(p.node)
	if err != nil {
		return err
	}
	// Navigate skeleton root along segs[:len(segs)-1] to reach
	// parent's RM value. The last segment carries the attribute name
	// to write `p.rawValue` against.
	rmParent, err := navigateTo(b.skeleton, segs[:len(segs)-1])
	if err != nil {
		return err
	}
	leaf := segs[len(segs)-1]
	// Single attrs → EnsureSingle. Multi attrs hosting a child element
	// → the path identifies an existing instance; we replace via
	// EnsureSingle on its parent attribute (semantics: overwrite the
	// pre-allocated default slot). For the common leaf assignment
	// (DV_QUANTITY at ELEMENT.value) the parent is single, so this
	// path is the routine case.
	switch leaf.cardinality {
	case template.Single:
		if err := rmwrite.EnsureSingle(rmParent, parentRMType(rmParent), leaf.attrName, p.rawValue); err != nil {
			return fmt.Errorf("%w: %v", ErrInvalidPath, err)
		}
	case template.Multiple:
		// Replace-vs-append on multi-valued attrs is ambiguous from a
		// bare path; v1 surfaces this as ErrInvalidPath and asks the
		// caller to address a specific child slot (predicate-bearing
		// path) which Set already resolved. Implementation note: the
		// leaf node ALREADY identifies one specific child (predicates
		// in the path narrow it to one CompiledNode); the assignment
		// should walk to that child's index and overwrite. Phase 2
		// scope: only leaf-DV assignments are guaranteed, multi-attr
		// container assignment is a follow-up. Return a descriptive
		// error so the caller knows this is an intentional v1 gap.
		return fmt.Errorf("%w: multi-attribute container assignment not supported in v1 (path: %s)", ErrInvalidPath, p.path)
	}
	return nil
}

// pathSegment captures one step in the parent → child walk: the
// attribute on the parent RM value, its cardinality, and any
// predicate (archetype id / at-code) the OPT pinned for the child
// selection on multi-valued attributes.
type pathSegment struct {
	attrName    string
	cardinality template.Cardinality
	// matchID is the predicate to match against (archetype_node_id /
	// archetype id). Empty for single-attr segments.
	matchID string
}

// buildSegments produces the ordered (attr, predicate) list from
// compiled root to the supplied node. Walks up via Parent() and
// reverses.
func buildSegments(node *templatecompile.CompiledNode) ([]pathSegment, error) {
	var segs []pathSegment
	for cur := node; cur != nil && cur.Parent() != nil; cur = cur.Parent() {
		parent := cur.Parent()
		// Locate the attribute on parent that holds `cur`.
		attr, child := findAttributeContaining(parent, cur)
		if attr == nil {
			return nil, fmt.Errorf("%w: child %s not found under parent %s",
				ErrInvalidPath, cur.AQLPath(), parent.AQLPath())
		}
		seg := pathSegment{
			attrName:    attr.Name(),
			cardinality: attr.Cardinality(),
		}
		if attr.Cardinality() == template.Multiple {
			// Prefer archetype id over at-code as the predicate
			// (matches the path-segment policy in
			// internal/templatecompile/compile.go:pathSegment).
			if aid := child.ArchetypeID(); aid != "" {
				seg.matchID = aid
			} else if nid := child.NodeID(); nid != "" {
				seg.matchID = nid
			}
		}
		segs = append(segs, seg)
	}
	// Reverse — collected child→root, need root→child.
	for i, j := 0, len(segs)-1; i < j; i, j = i+1, j-1 {
		segs[i], segs[j] = segs[j], segs[i]
	}
	return segs, nil
}

// findAttributeContaining returns (parentAttr, child) where
// parentAttr is the CompiledAttribute on parent whose Children slice
// contains target.
func findAttributeContaining(parent, target *templatecompile.CompiledNode) (*templatecompile.CompiledAttribute, *templatecompile.CompiledNode) {
	for _, attr := range parent.Attributes() {
		for _, c := range attr.Children() {
			if c == target {
				return attr, c
			}
		}
	}
	return nil, nil
}

// navigateTo follows segs from rmRoot, returning the RM value at the
// terminal segment. Each step:
//   - Single attr: rmread.ReadSingle(parent, attr) → descend.
//   - Multi attr: rmread.ReadMultiple(parent, attr) → first child
//     whose archetype_node_id matches the segment's matchID, else
//     the first child when matchID is empty (covers OPT-silent or
//     non-archetype-keyed multi-attrs).
func navigateTo(rmRoot any, segs []pathSegment) (any, error) {
	cur := rmRoot
	for _, s := range segs {
		next, err := descendOne(cur, s)
		if err != nil {
			return nil, err
		}
		cur = next
	}
	return cur, nil
}

// descendOne descends one segment from cur. Errors surface
// ErrInvalidPath when the attribute is unknown or no child matches
// the predicate.
func descendOne(cur any, s pathSegment) (any, error) {
	switch s.cardinality {
	case template.Single:
		v, ok := rmread.ReadSingle(cur, parentRMType(cur), s.attrName)
		if !ok {
			return nil, fmt.Errorf("%w: attribute %q absent on %T", ErrInvalidPath, s.attrName, cur)
		}
		return v, nil
	case template.Multiple:
		items, ok := rmread.ReadMultiple(cur, parentRMType(cur), s.attrName)
		if !ok {
			return nil, fmt.Errorf("%w: multi-attribute %q absent on %T", ErrInvalidPath, s.attrName, cur)
		}
		if len(items) == 0 {
			return nil, fmt.Errorf("%w: multi-attribute %q empty on %T (NewSkeleton should have seeded at least one child)", ErrInvalidPath, s.attrName, cur)
		}
		if s.matchID == "" {
			return items[0], nil
		}
		for _, it := range items {
			if id := archetypeNodeID(it); id == s.matchID {
				return it, nil
			}
		}
		// Fall back to first item — accommodates skeletons that
		// stamped synthetic ids (e.g. slot fills with example archetype
		// ids) that the OPT-path predicate does not match exactly.
		return items[0], nil
	}
	return nil, fmt.Errorf("%w: unsupported cardinality %v on attribute %q", ErrInvalidPath, s.cardinality, s.attrName)
}

// parentRMType returns the RM class name for the supplied parent
// value. rmread / rmwrite carry the parentType arg for future
// string-keyed dispatch; v1 uses Go concrete type only, so the value
// here is a diagnostic-only hint.
func parentRMType(v any) string {
	switch v.(type) {
	case *rm.Composition, rm.Composition:
		return "COMPOSITION"
	case *rm.Observation, rm.Observation:
		return "OBSERVATION"
	case *rm.Evaluation, rm.Evaluation:
		return "EVALUATION"
	case *rm.Instruction, rm.Instruction:
		return "INSTRUCTION"
	case *rm.Action, rm.Action:
		return "ACTION"
	case *rm.AdminEntry, rm.AdminEntry:
		return "ADMIN_ENTRY"
	case *rm.GenericEntry, rm.GenericEntry:
		return "GENERIC_ENTRY"
	case *rm.Section, rm.Section:
		return "SECTION"
	case *rm.Activity, rm.Activity:
		return "ACTIVITY"
	case *rm.EventContext, rm.EventContext:
		return "EVENT_CONTEXT"
	case *rm.History[rm.ItemStructure], rm.History[rm.ItemStructure]:
		return "HISTORY"
	case *rm.PointEvent[rm.ItemStructure], rm.PointEvent[rm.ItemStructure]:
		return "POINT_EVENT"
	case *rm.IntervalEvent[rm.ItemStructure], rm.IntervalEvent[rm.ItemStructure]:
		return "INTERVAL_EVENT"
	case *rm.ItemTree, rm.ItemTree:
		return "ITEM_TREE"
	case *rm.ItemList, rm.ItemList:
		return "ITEM_LIST"
	case *rm.ItemSingle, rm.ItemSingle:
		return "ITEM_SINGLE"
	case *rm.ItemTable, rm.ItemTable:
		return "ITEM_TABLE"
	case *rm.Cluster, rm.Cluster:
		return "CLUSTER"
	case *rm.Element, rm.Element:
		return "ELEMENT"
	case *rm.DVText, rm.DVText:
		return "DV_TEXT"
	case *rm.DVCodedText, rm.DVCodedText:
		return "DV_CODED_TEXT"
	case *rm.CodePhrase, rm.CodePhrase:
		return "CODE_PHRASE"
	}
	return ""
}

// archetypeNodeID returns the LOCATABLE.archetype_node_id of the
// supplied RM value, or "" when the value does not carry one.
// Closed type switch — REQ-024, no reflection.
func archetypeNodeID(v any) string {
	switch x := v.(type) {
	case *rm.Composition:
		return x.ArchetypeNodeID
	case *rm.Observation:
		return x.ArchetypeNodeID
	case *rm.Evaluation:
		return x.ArchetypeNodeID
	case *rm.Instruction:
		return x.ArchetypeNodeID
	case *rm.Action:
		return x.ArchetypeNodeID
	case *rm.AdminEntry:
		return x.ArchetypeNodeID
	case *rm.GenericEntry:
		return x.ArchetypeNodeID
	case *rm.Section:
		return x.ArchetypeNodeID
	case *rm.Activity:
		return x.ArchetypeNodeID
	case *rm.History[rm.ItemStructure]:
		return x.ArchetypeNodeID
	case *rm.PointEvent[rm.ItemStructure]:
		return x.ArchetypeNodeID
	case *rm.IntervalEvent[rm.ItemStructure]:
		return x.ArchetypeNodeID
	case *rm.ItemTree:
		return x.ArchetypeNodeID
	case *rm.ItemList:
		return x.ArchetypeNodeID
	case *rm.ItemSingle:
		return x.ArchetypeNodeID
	case *rm.ItemTable:
		return x.ArchetypeNodeID
	case *rm.Cluster:
		return x.ArchetypeNodeID
	case *rm.Element:
		return x.ArchetypeNodeID
	}
	return ""
}
