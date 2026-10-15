package composition

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/cadasto/openehr-sdk-go/internal/rmaccess"
	tcimpl "github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/internal/templateinstance/rmwrite"
	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
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

// pendingAssignment is one queued (path, value) pair plus the compiled
// node the value binds: of the nodes the template allows at the path,
// the one the validator would bind it to ([bindRMType]). Resolving
// the node and checking the type at Set time surfaces an unknown path
// eagerly, and a type mismatch wherever the template and the pinned RM
// give a type to check against. Where they give none, as under an
// attribute the RM types with a generic parameter that the parent's type
// name leaves open (the lower bound of a bare DV_INTERVAL), a value of
// the wrong type fails only when Build writes it, with ErrInvalidPath.
// RM-side navigation still happens at Build time so the caller can stack
// Set calls without each one mutating the graph.
type pendingAssignment struct {
	path     string
	rawValue any
	node     *tcimpl.CompiledNode
}

// NewBuilder constructs a Builder seeded with NewSkeleton output.
// Composition options (WithComposer, WithTerritory, …) are forwarded
// to the underlying instance.Generate call. The Builder retains the
// compiled template for path lookups.
func NewBuilder(ctx context.Context, c *templatecompile.Compiled, opts ...Option) (*Builder, error) {
	if c == nil {
		// Wrap instance.ErrNilCompiled (the sentinel NewSkeleton /
		// instance.Generate return for a nil template) so external callers
		// can errors.Is against it uniformly across the three entry points.
		return nil, fmt.Errorf("composition.NewBuilder: %w", instance.ErrNilCompiled)
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

// Set assigns v at path. Path must resolve in the compiled template,
// or Set returns ErrUnknownPath.
//
// Set accepts v when v binds a node the template allows at path, by the
// same rule the validator uses to match a value to the template, and the
// RM attribute that path writes can hold v. v binds a node when:
//
//   - v's RM type is the node's type, a typed interval such as
//     DV_INTERVAL<DV_QUANTITY> included;
//   - the node names an AOM 1.4 primitive short name and v is the data
//     value that carries it: a *rm.DVDuration at an ELEMENT value the
//     template types DURATION, and likewise DV_DATE at DATE, DV_TIME at
//     TIME, DV_DATE_TIME at DATE_TIME, DV_BOOLEAN at BOOLEAN and DV_COUNT
//     at INTEGER;
//   - v's RM type descends from the node's in the openEHR class
//     hierarchy: a *rm.DVCodedText at a DV_TEXT node, or a concrete data
//     value, such as a *rm.DVQuantity, at a node the template types as an
//     abstract class, such as DATA_VALUE;
//   - the template leaves the node untyped; or
//   - the node is a typed interval DV_INTERVAL<T> and v is an interval
//     each of whose bounds is a T or descends from T: a
//     DV_INTERVAL<DV_COUNT> at a DV_INTERVAL<DV_ORDERED> node.
//
// Where the template lists several alternatives at path under one
// attribute, v may bind any of them, except one whose occurrences are
// 0..0, which the template does not allow: at an ELEMENT value the
// template allows as DV_CODED_TEXT or DV_QUANTITY, both a
// *rm.DVCodedText and a *rm.DVQuantity are accepted.
//
// The RM attribute that path writes can hold v when v's RM type is, or
// descends from, the type the openEHR Reference Model gives that
// attribute on the RM type of the node above it. An ELEMENT's value
// holds any data value; the value of a DV_DURATION holds a String, so no
// RM value at all. Where the model types the attribute with a generic
// parameter that the template leaves open, v must conform to the
// parameter's constraint: the bounds of a DV_INTERVAL the template does
// not parameterise hold a DV_ORDERED.
//
// Set returns ErrTypeMismatch for any other value: one that binds none of
// those nodes (a *rm.DVText at a DV_CODED_TEXT node, say), one the RM
// attribute at path cannot hold (a *rm.DVDuration at the DURATION node
// under a DV_DURATION's value, or a *rm.DVText at a bound of a bare
// DV_INTERVAL), one at a path that writes no RM attribute (a template may
// constrain a function of the model as if it were an attribute, such as
// DV_PROPORTION.is_integral), one that is not an RM value (a Go string,
// even at a DURATION node), nil, and a nil pointer.
//
// Errors are accumulated and surfaced from Build. Set never
// short-circuits the builder so the caller can stack assignments and
// recover every faulty path in one round-trip.
//
// Returns the same error it stored, primarily for callers that want
// to react to the first failure inline.
func (b *Builder) Set(path string, v any) error {
	if b == nil {
		return errors.New("composition: nil Builder")
	}
	if v == nil {
		err := fmt.Errorf("%w: %s: nil value", ErrTypeMismatch, path)
		b.errs = append(b.errs, err)
		return err
	}
	node, err := b.compiled.NodeAt(path)
	if err != nil {
		// Wrap the compiled-template NodeAt cause via the multi-%w
		// form (Go 1.20+) so in-tree callers can errors.Is against
		// ErrUnknownPath AND against the inner templatecompile
		// sentinel (currently templatecompile.ErrPathNotFound — kept
		// distinct from the wire-parser's template.ErrPathNotFound by
		// design; see internal/templatecompile/compiled.go) on the
		// same error. The public API contract surface is unchanged:
		// external callers errors.Is against ErrUnknownPath.
		wrapped := fmt.Errorf("%w: %s: %w", ErrUnknownPath, path, err)
		b.errs = append(b.errs, wrapped)
		return wrapped
	}
	// The template may list several alternatives at path; NodeAt returns
	// the first, and the value binds the one the validator would bind it
	// to (REQ-101, REQ-102). They share the path, so Build reaches the
	// same RM attribute through any of them, and bindRMType checks that
	// the attribute can hold the value.
	bound, err := bindRMType(node, v)
	if err != nil {
		wrapped := fmt.Errorf("%s: %w", path, err)
		b.errs = append(b.errs, wrapped)
		return wrapped
	}
	b.pending = append(b.pending, pendingAssignment{
		path:     path,
		rawValue: v,
		node:     bound,
	})
	return nil
}

// SetText assigns &rm.DVText{Value: value} at path. Path must
// resolve to a node the template types as DV_TEXT or DATA_VALUE; a
// DV_CODED_TEXT node refuses it, since DV_TEXT does not descend from
// DV_CODED_TEXT.
func (b *Builder) SetText(path, value string) error {
	return b.Set(path, &rm.DVText{Value: value})
}

// SetQuantity assigns a *rm.DVQuantity at path. Path must resolve
// to a node the template types as DV_QUANTITY or as one of the classes
// it descends from: DV_AMOUNT, DV_QUANTIFIED, DV_ORDERED or DATA_VALUE.
func (b *Builder) SetQuantity(path string, magnitude float64, units string) error {
	return b.Set(path, &rm.DVQuantity{
		Magnitude: rm.Real(magnitude),
		Units:     units,
	})
}

// SetCodedText assigns a *rm.DVCodedText at path. Path must resolve
// to a node the template types as DV_CODED_TEXT, DV_TEXT or DATA_VALUE.
func (b *Builder) SetCodedText(path, terminology, code, display string) error {
	return b.Set(path, &rm.DVCodedText{
		Value: display,
		DefiningCode: rm.CodePhrase{
			CodeString:    code,
			TerminologyID: rm.TerminologyID{Value: terminology},
		},
	})
}

// Build finalises the graph. Pending assignments are applied in
// order against the skeleton; per-path failures accumulate via
// errors.Join. The returned *rm.Composition is the same skeleton
// instance: Build mutates it in place.
//
// Build is repeatable: pending Set calls and accumulated errors are
// consumed by each invocation. A second Build with no intervening Set
// returns the same skeleton with a nil error. Builder users can
// chain Set → Build → Set → Build without errors stacking from a
// prior pass; per-call errors return value still surfaces the
// per-Set failures that drove the current Build to fail.
func (b *Builder) Build() (*rm.Composition, error) {
	if b == nil {
		return nil, errors.New("composition: nil Builder")
	}
	// Drain accumulated state in one pass — pending consumed via the
	// range loop snapshot below; errs reset to capture only this
	// Build's failures.
	pending := b.pending
	b.pending = nil
	carriedErrs := b.errs
	b.errs = nil
	var passErrs []error
	passErrs = append(passErrs, carriedErrs...)
	for _, p := range pending {
		if err := b.applyAssignment(p); err != nil {
			passErrs = append(passErrs, fmt.Errorf("%s: %w", p.path, err))
		}
	}
	if len(passErrs) > 0 {
		return b.skeleton, errors.Join(passErrs...)
	}
	return b.skeleton, nil
}

// applyAssignment navigates the skeleton along the compiled path's
// parent chain, then routes the assignment through rmwrite.EnsureSingle
// when the path's last attribute is single-valued. A path that resolves
// to a member of a multi-valued attribute returns ErrInvalidPath: v1
// leaves container assignment to a future follow-up, since the common
// authoring case is leaf assignment.
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
	// Single attrs → EnsureSingle. A path ending in a multi-valued
	// attribute is refused with ErrInvalidPath (below). For the common
	// leaf assignment (DV_QUANTITY at ELEMENT.value) the parent is
	// single, so this path is the routine case.
	switch leaf.cardinality {
	case template.Single:
		if err := rmwrite.EnsureSingle(rmParent, leaf.attrName, p.rawValue); err != nil {
			return fmt.Errorf("%w: %w", ErrInvalidPath, err)
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
		// error so the caller knows this is an intentional v1 gap;
		// the leaf-DV path (extending the path with .value or
		// .magnitude / .units for DV_QUANTITY) covers the common
		// authoring case end-to-end today.
		return fmt.Errorf("%w: multi-attribute container assignment not supported in v1 (path: %s) — use a leaf path (e.g. extend with /value or /magnitude for DV_QUANTITY) to address the primitive directly", ErrInvalidPath, p.path)
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
func buildSegments(node *tcimpl.CompiledNode) ([]pathSegment, error) {
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
	slices.Reverse(segs)
	return segs, nil
}

// findAttributeContaining returns (parentAttr, child) where
// parentAttr is the CompiledAttribute on parent whose Children slice
// contains target.
func findAttributeContaining(parent, target *tcimpl.CompiledNode) (*tcimpl.CompiledAttribute, *tcimpl.CompiledNode) {
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
//   - Single attr: rmaccess.Single(parent, attr) → descend.
//   - Multi attr: rmaccess.Multiple(parent, attr) → first child
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
		v, ok := rmaccess.Single(cur, s.attrName)
		if !ok {
			return nil, fmt.Errorf("%w: attribute %q absent on %T", ErrInvalidPath, s.attrName, cur)
		}
		return v, nil
	case template.Multiple:
		items, ok := rmaccess.Multiple(cur, s.attrName)
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
		// No sibling matches the predicate — fail rather than silently
		// route to items[0]. PR #19 review: an authoring API must not
		// mis-route Set into the wrong sibling when predicates diverge
		// (skeleton at-code vs path archetype id, multiple siblings,
		// slot-fill mismatch). The skeleton's slot-fill stamping (the
		// REQ-107 ".example.v1" heuristic) is exposed here as
		// ErrInvalidPath. The REQ-104 assertion grammar now drives
		// slot-fit in validation and instance synthesis; this builder
		// routing still matches by archetype-id equality and does not
		// yet consult that grammar.
		return nil, fmt.Errorf("%w: no sibling under %q matches predicate %q (siblings: %s)",
			ErrInvalidPath, s.attrName, s.matchID, siblingIDs(items))
	}
	return nil, fmt.Errorf("%w: unsupported cardinality %v on attribute %q", ErrInvalidPath, s.cardinality, s.attrName)
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

// siblingIDs joins the archetype_node_id (or "<empty>") of each item
// for diagnostic output in descendOne's predicate-mismatch error path.
func siblingIDs(items []any) string {
	if len(items) == 0 {
		return "<none>"
	}
	parts := make([]string, 0, len(items))
	for _, it := range items {
		id := archetypeNodeID(it)
		if id == "" {
			id = "<empty>"
		}
		parts = append(parts, id)
	}
	return strings.Join(parts, ", ")
}
