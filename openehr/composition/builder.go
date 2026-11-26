package composition

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/cadasto/openehr-sdk-go/internal/rmaccess"
	tcimpl "github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/internal/templateinstance/builderseed"
	"github.com/cadasto/openehr-sdk-go/internal/templateinstance/rmwrite"
	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
)

// Builder accumulates path assignments over a skeleton. Returned by
// [NewBuilder]; finalised via [Builder.Build]. A Builder that NewBuilder
// did not make, such as the zero value, has no template to resolve paths
// against: its Set and Build return an error.
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

// errNotMade is the error Set and Build return on a Builder that
// [NewBuilder] did not make.
var errNotMade = errors.New("composition: Builder not made by NewBuilder")

// pendingAssignment is one queued (path, value) pair plus the compiled
// node the value binds: of the nodes the template allows at the path,
// the first node of the first binding tier that admits its type, which
// the validator tries first ([bindRMType]). Resolving
// the node and checking the type at Set time surfaces an unknown path
// and a type mismatch eagerly. Only where the template and the pinned RM
// give no type to check against, as under a parent the template leaves
// untyped or whose class the RM does not know, does a value of the wrong
// type fail later, when Build writes it, with ErrInvalidPath.
// RM-side navigation still happens at Build time so the caller can stack
// Set calls without each one mutating the graph.
type pendingAssignment struct {
	path     string
	rawValue any
	node     *tcimpl.CompiledNode
}

// NewBuilder constructs a Builder seeded with NewSkeleton output.
// Composition options (WithComposer, WithTerritory, WithPolicy, …) are
// forwarded to the underlying instance.Generate call. The Builder retains
// the compiled template for path lookups.
//
// Set routes a value only into a node the skeleton already holds, and does
// not build a missing one, so the skeleton must carry a member of every
// optional archetype the template names, which only instance.Large builds.
// NewBuilder builds on instance.Large when no WithPolicy is given, and
// accepts only instance.Large. Its skeleton is Large short of the second
// member Large gives a node that may occur more than once and of the
// optional RM attributes Large writes where the template names none: one
// member of every optional archetype and of every use of one. It refuses
// instance.Medium and instance.Required, naming the policy, and any Policy
// value the instance package does not define, before generating anything,
// with an error wrapping ErrUnsupportedPolicy and no Builder. Only the
// refusal of an undefined value also wraps instance.ErrUnsupportedPolicy,
// since the instance package defines Medium and Required.
func NewBuilder(ctx context.Context, c *templatecompile.Compiled, opts ...Option) (*Builder, error) {
	if c == nil {
		// Wrap instance.ErrNilCompiled (the sentinel NewSkeleton /
		// instance.Generate return for a nil template) so external callers
		// can errors.Is against it uniformly across the three entry points.
		return nil, fmt.Errorf("composition.NewBuilder: %w", instance.ErrNilCompiled)
	}
	cfg := buildConfig(opts...)
	if !cfg.policySet {
		// Whether WithPolicy was given, not the policy's value, decides:
		// an explicit WithPolicy of the zero value is refused like any
		// other policy but Large.
		cfg.policy = instance.Large
	}
	switch cfg.policy {
	case instance.Large:
		// Its skeleton holds a member of every optional archetype, the
		// nodes Set routes a value into.
	case instance.Medium, instance.Required:
		return nil, fmt.Errorf("composition.NewBuilder: %w: %s", ErrUnsupportedPolicy, cfg.policy)
	default:
		return nil, errUndefinedPolicy("composition.NewBuilder", cfg.policy)
	}
	// The builder's seed is Large short of its repeats and of the optional
	// RM attributes Large adds where the template names none: Set routes a
	// value into a node by its path, and one member of each is the node.
	skel, err := newSkeleton(builderseed.With(ctx), c, cfg)
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
// Set returns ErrInvalidPath for a value it does not refuse with
// ErrTypeMismatch at a path it writes no value at:
//
//   - the composition root;
//   - a path that ends in a member of a multi-valued attribute, such as an
//     ELEMENT under ITEM_TREE.items or an EVENT under HISTORY.events; a path
//     that goes on to an attribute of such a member, such as the ELEMENT's
//     value, is one Set writes;
//   - a path through a positional segment, the [@N] the compiled template
//     gives a list member with no archetype id, node id or slot predicate,
//     such as a PARTICIPATION, where the template allows more than one child
//     for that list, or prohibits the child the segment names: no list item
//     says which child it was built from. Where the template allows that
//     child alone, the path is one Set writes, whatever children with
//     occurrences 0..0 stand beside it.
//
// Errors are accumulated and surfaced from Build. Set never
// short-circuits the builder so the caller can stack assignments and
// recover every faulty path in one round-trip.
//
// Returns the same error it stored, primarily for callers that want
// to react to the first failure inline. On a nil Builder, or one that
// NewBuilder did not make, Set returns an error and stores nothing.
func (b *Builder) Set(path string, v any) error {
	if b == nil {
		return errors.New("composition: nil Builder")
	}
	if b.compiled == nil {
		return errNotMade
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
	// the first, and the value binds the first node of the first tier that
	// admits its type, which the validator tries first (REQ-101, REQ-102).
	// They share the path, so Build reaches the same RM attribute through
	// any of them, and bindRMType checks that the attribute can hold the
	// value. A node below a later alternative, or below a later sibling
	// with the same node id, shares its path with the node below the first,
	// so NodeAt returns the one below the first, and the value binds against
	// the alternatives under that node's attribute alone (REQ-101 Type
	// enforcement). Whether the values set below one ISM_TRANSITION match a
	// single alternative is the validator's call, not Set's.
	bound, err := bindRMType(node, v)
	if err != nil {
		wrapped := fmt.Errorf("%s: %w", path, err)
		b.errs = append(b.errs, wrapped)
		return wrapped
	}
	// A path Set writes no value at is refused after the type checks, so a
	// value of the wrong type there keeps its ErrTypeMismatch (REQ-101 Type
	// enforcement).
	if err := checkWritable(path, bound); err != nil {
		b.errs = append(b.errs, err)
		return err
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
// instance: Build mutates it in place. A value whose path runs through an
// item the skeleton does not hold is written nowhere, and Build reports
// ErrInvalidPath for it: an attribute the skeleton leaves absent or empty,
// or an archetype id, node id or name that no member of a multi-valued
// attribute carries, such as a named sibling the skeleton left out.
//
// Build is repeatable: pending Set calls and accumulated errors are
// consumed by each invocation. A second Build with no intervening Set
// returns the same skeleton with a nil error. Builder users can
// chain Set → Build → Set → Build without errors stacking from a
// prior pass; per-call errors return value still surfaces the
// per-Set failures that drove the current Build to fail.
//
// On a nil Builder, or one that NewBuilder did not make, Build returns
// no composition and an error.
func (b *Builder) Build() (*rm.Composition, error) {
	if b == nil {
		return nil, errors.New("composition: nil Builder")
	}
	if b.compiled == nil {
		return nil, errNotMade
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
	// The drained array is not reused for the next Set calls: a Set made
	// while this pass ran has already started a new queue, and a shallow copy
	// of the Builder would share the array with this one.
	if len(passErrs) > 0 {
		return b.skeleton, errors.Join(passErrs...)
	}
	return b.skeleton, nil
}

// checkWritable returns an error wrapping ErrInvalidPath when node, the
// node a value set at path binds, is one the builder writes no value at:
//
//   - the composition root, which sits under no attribute and which the
//     builder does not replace;
//   - a member of a multi-valued attribute, which a value would replace or
//     add to; a node below such a member is writable;
//   - a node below a positional member of a multi-valued attribute that
//     the template prohibits, with occurrences 0..0, or for which it allows
//     more than one child. A positional member ([positional]) carries
//     nothing to match a list item by, and the skeleton's list order need
//     not follow the template's, so the builder cannot tell which item the
//     position names. Where the template allows that child alone, every
//     item the skeleton holds there was built from it, and the first
//     serves; a prohibited sibling builds no item. Where it prohibits the
//     child, the skeleton holds no item built from it, and the first item
//     is another child's. The type tiers do not refuse such a path, since
//     they bind the node the path ends at, which a prohibited ancestor
//     leaves allowed.
func checkWritable(path string, node *tcimpl.CompiledNode) error {
	parent := node.Parent()
	if parent == nil {
		return fmt.Errorf("%w: %s: the composition root takes no value", ErrInvalidPath, path)
	}
	if attr := findAttributeContaining(parent, node); attr != nil && attr.Cardinality() == template.Multiple {
		return fmt.Errorf("%w: %s ends in a member of the multi-valued attribute %q: extend the path to an attribute of that member",
			ErrInvalidPath, path, attr.Name())
	}
	// Every positional segment on the path counts, not only the nearest.
	for cur := parent; cur.Parent() != nil; cur = cur.Parent() {
		attr := findAttributeContaining(cur.Parent(), cur)
		if attr == nil || attr.Cardinality() != template.Multiple || !positional(cur) {
			continue
		}
		allowed := tcimpl.AllowedChildNodes(attr)
		if !slices.Contains(allowed, cur) {
			return fmt.Errorf("%w: %s: the positional segment %s names a child of the multi-valued attribute %q that the template prohibits: the skeleton holds no list item built from it",
				ErrInvalidPath, path, cur.AQLPath(), attr.Name())
		}
		if len(allowed) > 1 {
			return fmt.Errorf("%w: %s: the positional segment %s sits under the multi-valued attribute %q, for which the template allows more than one child: the builder cannot tell which list item it names",
				ErrInvalidPath, path, cur.AQLPath(), attr.Name())
		}
	}
	return nil
}

// positional reports whether the compile step keys n, a child of a
// multi-valued attribute, by its position among its siblings ([@N]), as it
// does a child with no archetype id, no node id and, for a slot, no include
// pattern to put in the path. A slot holds no attributes, so a slot is never
// a node on the way to the one a path binds.
func positional(n *tcimpl.CompiledNode) bool {
	return n.ArchetypeID() == "" && n.NodeID() == "" && (!n.IsSlot() || len(n.SlotIncludes()) == 0)
}

// applyAssignment navigates the skeleton along the compiled path's
// parent chain, then routes the assignment through rmwrite.EnsureSingle,
// since the path's last attribute is single-valued: Set refuses the root
// and a path that ends in a member of a multi-valued attribute
// (checkWritable), and applyAssignment refuses either again with
// ErrInvalidPath rather than index past its segments or write nothing.
func (b *Builder) applyAssignment(p pendingAssignment) error {
	parent := p.node.Parent()
	if parent == nil {
		// Set refuses the root (checkWritable); with no segment to write
		// through, refuse it here too.
		return fmt.Errorf("%w: cannot Set the composition root", ErrInvalidPath)
	}
	// Walk the parent chain from compiled root → parent collecting
	// (CompiledNode, attrName, predicate) segments. The leaf segment
	// supplies the attribute on `parent` to write into. Nothing keeps the
	// segments past this call, so a template of usual depth needs no heap
	// for them.
	var buf [segmentsOnStack]pathSegment
	segs, err := buildSegments(buf[:0], p.node)
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
	switch leaf.cardinality {
	case template.Single:
		if err := rmwrite.EnsureSingle(rmParent, leaf.attrName, p.rawValue); err != nil {
			return fmt.Errorf("%w: %w", ErrInvalidPath, err)
		}
	case template.Multiple:
		// Set refuses such a path (checkWritable), so a pending assignment
		// never ends here; refuse one rather than write nothing.
		return fmt.Errorf("%w: %s ends in a member of the multi-valued attribute %q", ErrInvalidPath, p.path, leaf.attrName)
	}
	return nil
}

// pathSegment captures one step in the parent → child walk: the
// attribute on the parent RM value, its cardinality, and any
// predicate (archetype id / at-code, and the name the template fixes
// on the child) the OPT pinned for the child selection on
// multi-valued attributes.
type pathSegment struct {
	attrName    string
	cardinality template.Cardinality
	// matchID is the predicate to match against (archetype_node_id /
	// archetype id). Empty for single-attr segments.
	matchID string
	// matchName is the name the template fixes on the child, which the
	// child's LOCATABLE.name value must equal: siblings that share
	// matchID differ by it alone. Empty when the template fixes none,
	// and whenever matchID is empty.
	matchName string
}

// segmentsOnStack is the depth of path applyAssignment keeps its segments
// for without the heap.
const segmentsOnStack = 16

// buildSegments produces the ordered (attr, predicate) list from
// compiled root to the supplied node, in dst's array where it has room.
// Walks up via Parent(), filling the list from its end.
func buildSegments(dst []pathSegment, node *tcimpl.CompiledNode) ([]pathSegment, error) {
	depth := 0
	for cur := node; cur != nil && cur.Parent() != nil; cur = cur.Parent() {
		depth++
	}
	segs := slices.Grow(dst[:0], depth)[:depth]
	i := depth
	for cur := node; cur != nil && cur.Parent() != nil; cur = cur.Parent() {
		parent := cur.Parent()
		// Locate the attribute on parent that holds `cur`.
		attr := findAttributeContaining(parent, cur)
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
			// internal/templatecompile/compile.go:childPath).
			if aid := cur.ArchetypeID(); aid != "" {
				seg.matchID = aid
			} else if nid := cur.NodeID(); nid != "" {
				seg.matchID = nid
			}
			// The compiled path carries the fixed name beside the id, as a
			// name predicate, and never on its own.
			if seg.matchID != "" {
				seg.matchName = cur.NodeName()
			}
		}
		i--
		segs[i] = seg
	}
	return segs, nil
}

// findAttributeContaining returns the CompiledAttribute on parent whose
// children include target, or nil when none does. It reads the compiled
// slices in place, without the copies the exported accessors make.
func findAttributeContaining(parent, target *tcimpl.CompiledNode) *tcimpl.CompiledAttribute {
	for _, attr := range tcimpl.NodeAttributes(parent) {
		if slices.Contains(tcimpl.ChildNodes(attr), target) {
			return attr
		}
	}
	return nil
}

// navigateTo follows segs from rmRoot, returning the RM value at the
// terminal segment. Each step:
//   - Single attr: rmaccess.Single(parent, attr) → descend.
//   - Multi attr: rmaccess.AppendMultiple(buf, parent, attr) → first child
//     whose archetype_node_id matches the segment's matchID and, when
//     the segment carries a matchName, whose name value equals it;
//     else the first child when matchID is empty (covers OPT-silent or
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
		// The items are read into a buffer on the stack, so a list of up
		// to eight items is read without allocating.
		var buf [8]any
		items, ok := rmaccess.AppendMultiple(buf[:0], cur, s.attrName)
		if !ok {
			return nil, fmt.Errorf("%w: multi-attribute %q absent on %T", ErrInvalidPath, s.attrName, cur)
		}
		if len(items) == 0 {
			return nil, fmt.Errorf("%w: multi-attribute %q empty on %T (NewSkeleton should have seeded at least one child)", ErrInvalidPath, s.attrName, cur)
		}
		if s.matchID == "" {
			// A positional segment: Set refuses one whose list the template
			// allows more than one child, or that names a child it prohibits
			// (checkWritable), so every item here was built from the one
			// child it allows, and the first serves.
			return items[0], nil
		}
		for _, it := range items {
			// Siblings that share the id differ by the name the template
			// fixes on each, so the name must match too when it fixes one.
			if archetypeNodeID(it) == s.matchID && (s.matchName == "" || locatableName(it) == s.matchName) {
				return it, nil
			}
		}
		// No sibling matches the predicate — fail rather than silently
		// route to items[0]. PR #19 review: an authoring API must not
		// mis-route Set into the wrong sibling when predicates diverge
		// (skeleton at-code vs path archetype id, multiple siblings,
		// slot-fill mismatch). The skeleton's slot-fill stamping (the
		// REQ-107 ".example.v1" heuristic) is exposed here as
		// ErrInvalidPath. The REQ-103 slot assertion grammar now drives
		// slot-fit in validation and instance synthesis; this builder
		// routing still matches by archetype-id equality and does not
		// yet consult that grammar.
		return nil, fmt.Errorf("%w: no sibling under %q matches predicate %q (siblings: %s)",
			ErrInvalidPath, s.attrName, tcimpl.NamePredicate(s.matchID, s.matchName), siblingIDs(items))
	}
	return nil, fmt.Errorf("%w: unsupported cardinality %v on attribute %q", ErrInvalidPath, s.cardinality, s.attrName)
}

// archetypeNodeID returns the LOCATABLE.archetype_node_id of the
// supplied RM value, or "" when the value is no LOCATABLE or is a nil
// pointer to one.
func archetypeNodeID(v any) string {
	if l, ok := v.(rm.Locatable); ok && !rm.IsTypedNil(v) {
		return l.GetArchetypeNodeID()
	}
	return ""
}

// locatableName returns the value of the LOCATABLE.name of v, or "" when v
// is no LOCATABLE, is a nil pointer to one, or carries no name.
func locatableName(v any) string {
	l, ok := v.(rm.Locatable)
	if !ok || rm.IsTypedNil(v) {
		return ""
	}
	name := l.GetName()
	if name == nil || rm.IsTypedNil(name) {
		return ""
	}
	return name.GetValue()
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
