package templatecompile

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/rm/rminfo"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
)

// Options tune the Compile step. The zero value uses sensible
// defaults: rminfo.Default for implicit attribute resolution, and
// "inject implicit attributes" enabled.
type Options struct {
	// Lookup is the RM info source used to inject implicit
	// attributes the OPT omits. Defaults to [rminfo.Default] when
	// nil.
	Lookup rminfo.Lookup

	// SkipImplicitAttributes disables RM-attribute injection.
	// Compiled nodes will then carry only the attributes the OPT
	// declared. Useful for tests and for round-trip serialisation
	// that needs to preserve the OPT's explicit-only shape.
	SkipImplicitAttributes bool
}

// Compile turns a parsed OPT into a walker-friendly compiled
// representation. The input is read-only — the returned Compiled
// tree shares no mutable state with opt (struct values are copied,
// slices are freshly allocated).
//
// Returns ErrInvalidInput when opt is nil or has no root. Returns
// any error surfaced by AQL path computation (none in v1).
func Compile(opt *template.OperationalTemplate, opts ...Options) (*Compiled, error) {
	if opt == nil {
		return nil, fmt.Errorf("%w: nil template", ErrInvalidInput)
	}
	if opt.Root() == nil {
		return nil, fmt.Errorf("%w: template has no root", ErrInvalidInput)
	}

	o := Options{}
	if len(opts) > 0 {
		o = opts[0]
	}
	if o.Lookup == nil {
		o.Lookup = rminfo.Default
	}

	c := &Compiled{
		templateID:    opt.TemplateID(),
		concept:       opt.Concept(),
		uid:           opt.UID(),
		language:      opt.Language(),
		byPath:        make(map[string]*CompiledNode),
		byNodeID:      make(map[string][]*CompiledNode),
		byRMType:      make(map[string][]*CompiledNode),
		byArchetypeID: make(map[string][]*CompiledNode),
	}

	w := walker{
		compiled: c,
		lookup:   o.Lookup,
		opts:     o,
		pathAttr: map[string]*template.Attribute{},
		seenPath: map[string]bool{},
	}
	root, err := w.compileNode(opt.Root(), nil, "")
	if err != nil {
		return nil, err
	}
	c.root = root
	return c, nil
}

// ErrInvalidInput is returned by [Compile] for nil templates or
// templates whose root could not be resolved.
var ErrInvalidInput = errors.New("templatecompile: invalid input")

// walker carries the per-call state shared across recursive node
// builds.
type walker struct {
	compiled *Compiled
	lookup   rminfo.Lookup
	opts     Options

	// currentAttr is the wire-side *template.Attribute under whose
	// Children() the immediately-recursed compileNode is descending.
	// Set by buildAttribute around the per-child loop and consumed by
	// registerPath to discriminate AOM 1.4 C_SINGLE_ATTRIBUTE
	// alternatives (legal path duplicates) from genuine cross-
	// attribute path collisions (an OPT bug).
	currentAttr *template.Attribute

	// pathAttr records which wire-side attribute first registered a
	// given AQL path. registerPath consults it on duplicates: same
	// attribute → alternative, keep first; different attribute →
	// reject with ErrInvalidInput.
	pathAttr map[string]*template.Attribute

	// seenPath marks every AQL path reached at node *entry* (pre-order).
	// byPath cannot serve here: registration is post-order (a node
	// registers after its own subtree), so at the time a second node
	// with the same path descends, the first node's own entry is not yet
	// in byPath while its descendants already are.
	seenPath map[string]bool

	// dupDepth > 0 while descending the subtree of a node whose AQL path
	// repeats one already seen. Two nodes may legally share a path — AOM
	// 1.4 C_SINGLE_ATTRIBUTE alternatives, and sibling nodes carrying the
	// same node_id (repeated CLUSTER/ELEMENT) — and when they do, their
	// descendants necessarily share paths too. Those descendants sit
	// under *different* attribute objects (one per parent), so the
	// currentAttr test in registerPath cannot recognise them; dupDepth
	// does.
	//
	// Scope of the relaxation, stated precisely: every node's *own* path
	// is still fully guarded, because the counter covers only the descent
	// and is dropped before the node registers itself. Inside a shared-path
	// subtree, however, descendants are admitted unconditionally — so a
	// genuine cross-attribute collision occurring *only* within the second
	// sibling's subtree (possible when siblings narrow the same archetype
	// differently) is admitted rather than reported. That is a deliberate
	// trade: the alternative is rejecting every legal shared-path template.
	// The first-walked instance of any such collision still errors, so a
	// bug present in both siblings is caught.
	dupDepth int
}

// compileNode walks one OPT node, computes its AQL path, recurses
// into its children, and registers the result in the indexes.
// parentPath is the AQL path of the enclosing node ("" for the root);
// segment is the path delta from parent to this node ("" for the
// root). The two-arg shape avoids re-concatenating per-call.
func (w *walker) compileNode(n template.Node, parent *CompiledNode, segment string) (*CompiledNode, error) {
	cn := &CompiledNode{parent: parent}

	parentPath := ""
	if parent != nil {
		parentPath = parent.aqlPath
	}
	switch parentPath {
	case "":
		// Root path is "/". Descend deltas append directly.
		if segment == "" {
			cn.aqlPath = "/"
		} else {
			cn.aqlPath = segment
		}
	default:
		if parentPath == "/" {
			cn.aqlPath = segment
		} else {
			cn.aqlPath = parentPath + segment
		}
	}

	// A repeated path means this node shares an AQL path with one already
	// walked (alternatives, or a same-node_id sibling). Mark the descent
	// so registerPath admits the descendants that necessarily collide;
	// the counter is dropped again before this node registers itself.
	inDup := w.seenPath[cn.aqlPath]
	w.seenPath[cn.aqlPath] = true
	if inDup {
		w.dupDepth++
	}
	err := w.descend(n, cn)
	if inDup {
		w.dupDepth--
	}
	if err != nil {
		return nil, err
	}

	if err := w.registerPath(cn); err != nil {
		return nil, err
	}
	if cn.nodeID != "" {
		w.compiled.byNodeID[cn.nodeID] = append(w.compiled.byNodeID[cn.nodeID], cn)
	}
	if cn.rmTypeName != "" {
		w.compiled.byRMType[cn.rmTypeName] = append(w.compiled.byRMType[cn.rmTypeName], cn)
	}
	if cn.archetypeID != "" {
		w.compiled.byArchetypeID[cn.archetypeID] = append(w.compiled.byArchetypeID[cn.archetypeID], cn)
	}
	return cn, nil
}

// descend copies the wire node's own fields onto cn and recurses into
// its children. Split out of compileNode so the caller can scope the
// duplicate-path subtree marker (dupDepth) to the recursion alone,
// leaving cn's own path registration subject to the full collision
// check.
func (w *walker) descend(n template.Node, cn *CompiledNode) error {
	// The template-level node name (REQ-116) is read via ObjectNode,
	// the interface that exists so this carry cannot miss one of the
	// two object kinds; slots are not ObjectNodes and cannot pin one.
	if on, ok := n.(template.ObjectNode); ok {
		cn.nodeName = on.NodeName()
	}
	switch v := n.(type) {
	case *template.ArchetypeRoot:
		cn.rmTypeName = v.RMTypeName()
		cn.nodeID = v.NodeID()
		cn.archetypeID = v.ArchetypeID()
		cn.occurrences = v.Occurrences()
		// Per-archetype-root terms live on the node; bindings flatten
		// to the Compiled aggregate (binding records carry their own
		// terminology + at-code/path, so collisions are non-issues).
		cn.terms = copyTerms(v.Terms())
		w.compiled.termBindings = append(w.compiled.termBindings, v.TermBindings()...)
		return w.attachAttributes(cn, v.Attributes())
	case *template.ComplexObject:
		cn.rmTypeName = v.RMTypeName()
		cn.nodeID = v.NodeID()
		cn.occurrences = v.Occurrences()
		cn.primitive = v.PrimitiveConstraint()
		return w.attachAttributes(cn, v.Attributes())
	case *template.Slot:
		cn.rmTypeName = v.RMTypeName()
		cn.nodeID = v.NodeID()
		cn.isSlot = true
		cn.slotIncludes = v.Includes()
		cn.slotExcludes = v.Excludes()
		cn.slotRules = v.SlotRules()
		return nil
	default:
		return fmt.Errorf("templatecompile: unhandled wire node type %T", n)
	}
}

// attachAttributes builds the explicit attributes of cn from the
// OPT-declared list, then (unless disabled) injects implicit
// attributes for every RM-declared field the OPT did not name.
// Order: OPT-declared first (document order), then implicit
// (sorted by RM declaration order from rminfo).
func (w *walker) attachAttributes(cn *CompiledNode, declared []*template.Attribute) error {
	declaredByName := make(map[string]bool, len(declared))
	for _, a := range declared {
		declaredByName[a.Name()] = true
		ca, err := w.buildAttribute(cn, a)
		if err != nil {
			return err
		}
		cn.attributes = append(cn.attributes, ca)
	}
	if w.opts.SkipImplicitAttributes {
		return nil
	}
	if cn.rmTypeName == "" {
		return nil
	}
	// Walk RM declaration order so implicit attributes appear in
	// BMM-stable order regardless of OPT walk path.
	for _, attrName := range allAttributesInOrder(w.lookup, cn.rmTypeName) {
		if declaredByName[attrName] {
			continue
		}
		rm, ok := w.lookup.AttributeRMType(cn.rmTypeName, attrName)
		if !ok || rm == "" {
			continue
		}
		container, _ := w.lookup.IsContainer(cn.rmTypeName, attrName)
		// Skip implicit attributes the RM declares but does NOT
		// mandate — the composition builder only needs implicit
		// entries for required-but-OPT-silent fields. Non-required
		// implicit entries would inflate every node with optional
		// RM metadata that walker code does not need.
		if !slices.Contains(w.lookup.RequiredAttributes(cn.rmTypeName), attrName) {
			continue
		}
		card := template.Single
		if container {
			card = template.Multiple
		}
		cn.attributes = append(cn.attributes, &CompiledAttribute{
			name:        attrName,
			cardinality: card,
			rmTypeName:  rm,
			implicit:    true,
			required:    true,
		})
	}
	return nil
}

// buildAttribute compiles one OPT-declared attribute, recursing
// into its children. The attribute's BMM RM type is looked up so
// downstream consumers can resolve type-aware constraints without a
// separate rminfo query.
func (w *walker) buildAttribute(parent *CompiledNode, a *template.Attribute) (*CompiledAttribute, error) {
	rm, _ := w.lookup.AttributeRMType(parent.rmTypeName, a.Name())
	required := false
	if parent.rmTypeName != "" {
		required = slices.Contains(w.lookup.RequiredAttributes(parent.rmTypeName), a.Name())
	}
	ca := &CompiledAttribute{
		name:              a.Name(),
		cardinality:       a.Cardinality(),
		existence:         a.Existence(),
		childMultiplicity: a.ChildMultiplicity(),
		rmTypeName:        rm,
		required:          required,
	}
	prevAttr := w.currentAttr
	w.currentAttr = a
	defer func() { w.currentAttr = prevAttr }()
	for i, child := range a.Children() {
		segment := pathSegment(a.Name(), a.Cardinality(), child, i)
		cn, err := w.compileNode(child, parent, segment)
		if err != nil {
			return nil, err
		}
		ca.children = append(ca.children, cn)
	}
	return ca, nil
}

// pathSegment computes the path delta for descending from parent
// (attribute name + cardinality) into a child node. For single
// attributes the delta is "/name"; for multiple attributes the
// child contributes a predicate (archetype id, at-code, slot
// include pattern, or a 1-based sibling suffix when the OPT omits
// all of the above), with the template-level node name appended to
// an id-derived predicate per REQ-116 — see [namePredicated].
func pathSegment(attrName string, card template.Cardinality, child template.Node, siblingIndex int) string {
	seg := "/" + attrName
	if card != template.Multiple {
		// No named node in the vendored corpus sits under a single
		// attribute, and no reference golden predicates one, so the
		// bare form stands: a name never *creates* a predicate.
		return seg
	}
	if ar, ok := child.(*template.ArchetypeRoot); ok && ar.ArchetypeID() != "" {
		return seg + "[" + namePredicated(ar.ArchetypeID(), child) + "]"
	}
	if id := child.NodeID(); id != "" {
		return seg + "[" + namePredicated(id, child) + "]"
	}
	if sl, ok := child.(*template.Slot); ok {
		if p := slotPathPredicate(sl); p != "" {
			// Slots are not ObjectNodes and cannot pin a name.
			return seg + "[" + p + "]"
		}
	}
	// The synthetic positional key is our own fallback, never a shape
	// the reference emits, and it already disambiguates siblings. No
	// corpus node reaches it *with* a name; if one ever does, Phase 4
	// parity is where the reference's actual shape would surface.
	return seg + "[@" + strconv.Itoa(siblingIndex+1) + "]"
}

// NamePredicate appends a template-level node name to an id-derived path
// predicate, yielding the reference's `archetype_id,'Name'` form
// (REQ-116); it returns id unchanged when name is empty.
//
// Exported inside the module because REQ-116 paths are composed by *two*
// builders — this package's [pathSegment] and the WebTemplate builder's
// own `predicate` — and a quoting rule duplicated across both is a rule
// that drifts.
//
// Measured across the vendored reference goldens: the name is always
// *appended* to an id (341 archetype-id + 9 at-code segments in the
// corona golden, 27 + 3 in GECCO) and never stands alone, which is why
// callers decorate an existing predicate rather than forming one.
//
// Quoting follows the goldens: the name sits in single quotes and commas
// inside it are literal — one corona name ("… zu Menschen, die dort
// waren") carries one. No vendored name contains a single quote or a
// backslash, so the escapes are the conventional AQL reading rather than
// a golden-verified rule; revisit if a corpus name ever needs them.
// Backslash is escaped first and quote second — the reverse order would
// re-escape the backslash just written for the quote. Escaping the
// backslash is not optional style: a name ending in `\` would otherwise
// emit `,'…\'`, whose closing quote every scanner honouring `\'` consumes
// as escaped, swallowing the rest of the path.
func NamePredicate(id, name string) string {
	if name == "" {
		return id
	}
	name = strings.ReplaceAll(name, `\`, `\\`)
	name = strings.ReplaceAll(name, "'", `\'`)
	return id + ",'" + name + "'"
}

// namePredicated is the wire-side adapter for [NamePredicate]: slots are
// not ObjectNodes and pin no name, so they pass through unchanged.
func namePredicated(id string, child template.Node) string {
	on, ok := child.(template.ObjectNode)
	if !ok {
		return id
	}
	return NamePredicate(id, on.NodeName())
}

// slotPathPredicate derives a stable bracket predicate for an
// ARCHETYPE_SLOT that omits node_id. Uses the first include
// assertion (regex escapes stripped) so sibling slots under the
// same attribute do not collide in byPath.
func slotPathPredicate(s *template.Slot) string {
	inc := s.Includes()
	if len(inc) == 0 || inc[0] == "" {
		return ""
	}
	return strings.ReplaceAll(inc[0], `\`, "")
}

// allAttributesInOrder returns the BMM-declared attributes of an RM
// type in deterministic order. The rminfo Lookup does not expose
// AttrOrder directly; we reconstruct it by intersecting the order
// returned by RequiredAttributes + alphabetical for the
// non-required tail. (This is good enough for stable injection
// order; the actual BMM order is preserved by Required first since
// implicit injection only emits required attributes.)
//
// The implementation is intentionally simple: rminfo.Default's
// RequiredAttributes already returns in BMM declaration order, so
// for the required subset (the only thing we actually inject) it
// suffices.
func allAttributesInOrder(l rminfo.Lookup, rmType string) []string {
	return l.RequiredAttributes(rmType)
}

func (w *walker) registerPath(cn *CompiledNode) error {
	if prev, exists := w.compiled.byPath[cn.aqlPath]; exists {
		// AOM 1.4 admits C_SINGLE_ATTRIBUTE with multiple
		// `<children>` (alternatives); every alternative shares
		// the same AQL path. Admit the collision when both
		// nodes were registered under the SAME wire attribute —
		// then we are looking at alternatives, not unrelated
		// subtrees colliding on a synthetic predicate. byPath
		// keeps the first; the structural validator walks the
		// remaining alternatives via the parent attribute's
		// Children() directly.
		if w.currentAttr != nil && w.pathAttr[cn.aqlPath] == w.currentAttr {
			return nil
		}
		// Inside the subtree of a node that legally repeated a path
		// (alternatives, or a same-node_id sibling), every descendant
		// repeats too, each under its own parent's attribute object —
		// so the test above cannot see them. Keep the first, as with
		// alternatives; the structural validator reaches the rest via
		// the parent attribute's Children().
		if w.dupDepth > 0 {
			return nil
		}
		return fmt.Errorf(
			"%w: duplicate AQL path %q (existing %s, new %s)",
			ErrInvalidInput, cn.aqlPath, prev.rmTypeName, cn.rmTypeName,
		)
	}
	w.compiled.byPath[cn.aqlPath] = cn
	w.pathAttr[cn.aqlPath] = w.currentAttr
	return nil
}

// copyTerms deep-copies per-archetype-root term maps so compile
// output does not alias mutable state from the wire tree.
func copyTerms(src map[string]template.ArchetypeTerm) map[string]template.ArchetypeTerm {
	if src == nil {
		return nil
	}
	out := make(map[string]template.ArchetypeTerm, len(src))
	for code, term := range src {
		items := maps.Clone(term.Items)
		out[code] = template.ArchetypeTerm{Code: term.Code, Items: items}
	}
	return out
}
