package templatecompile

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/cadasto/openehr-sdk-go/internal/bmmtype"
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
// representation. The input is read-only: the returned Compiled
// tree shares no mutable state with opt (struct values are copied,
// slices are freshly allocated).
//
// Returns an error wrapping ErrInvalidInput when opt is nil, when it has
// no root, and when two nodes under different attributes have the same
// AQL path, which is a defect in the OPT. Nodes under one attribute that
// share a path (alternatives, or siblings with the same node id) and the
// descendants of such nodes are admitted, and [Compiled.NodeAt] resolves
// the path to the first one walked. Returns an error that does not wrap
// ErrInvalidInput when the tree holds a node of a kind Compile does not
// handle, which a parsed OPT does not contain.
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
		required: map[string][]string{},
		implicit: map[implicitKey]*CompiledAttribute{},
		paths:    map[string]pathMark{},
	}
	root, err := w.compileNode(opt.Root(), nil, "/", "")
	if err != nil {
		return nil, err
	}
	c.root = root
	return c, nil
}

// ErrInvalidInput is returned, wrapped, by [Compile] for a nil
// template, a template with no root, and a template in which two nodes
// under different attributes have the same AQL path.
var ErrInvalidInput = errors.New("templatecompile: invalid input")

// walker carries the per-call state shared across recursive node
// builds.
type walker struct {
	compiled *Compiled
	lookup   rminfo.Lookup
	opts     Options

	// required holds, for each bare RM class this call has met, the
	// attributes lookup says the class requires, so the lookup, which
	// builds a fresh list on every call, is asked once per class rather
	// than once per node and per declared attribute.
	required map[string][]string

	// implicit holds the implicit attribute made for each owner type and
	// attribute name this call has met, nil when the lookup gives the
	// attribute no RM type. An implicit attribute depends on nothing else:
	// it has no children, no existence and no node-specific fact, so the
	// nodes of one owner type share it.
	implicit map[implicitKey]*CompiledAttribute

	// descendants holds, for each bare class an OPT-declared attribute has
	// been looked up on and not found, every class the lookup says descends
	// from it, or nil when the lookup gives no verdict on the class
	// ([walker.descendantsOf]). Nil until the first such attribute.
	descendants map[string][]string

	// currentAttr is the wire-side *template.Attribute under whose
	// Children() the immediately-recursed compileNode is descending.
	// Set by buildAttribute around the per-child loop and consumed by
	// registerPath to discriminate AOM 1.4 C_SINGLE_ATTRIBUTE
	// alternatives (legal path duplicates) from genuine cross-
	// attribute path collisions (an OPT bug).
	currentAttr *template.Attribute

	// paths holds every AQL path reached at node *entry* (pre-order).
	// byPath cannot serve here: registration is post-order (a node
	// registers after its own subtree), so at the time a second node
	// with the same path descends, the first node's own entry is not yet
	// in byPath while its descendants already are. Each entry also
	// records which wire-side attribute first registered the path (see
	// [pathMark]).
	paths map[string]pathMark

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

// implicitKey names an implicit attribute: the RM type of its owner, as
// the OPT writes it, and the attribute's name.
type implicitKey struct{ owner, attr string }

// pathMark is what the walk knows of an AQL path it has reached: attr is
// the wire-side attribute that first registered the path, nil until a node
// registers it (and for the root, which no attribute holds). registerPath
// consults it on duplicates: same attribute → alternative, keep first;
// different attribute → reject with ErrInvalidInput.
type pathMark struct {
	attr *template.Attribute
}

// compileNode walks one OPT node, recurses into its children, and
// registers the result in the indexes. path is the node's AQL path ("/"
// for the root), which the caller computes ([childPath]). heldAs is the
// type the node's attributes are read on when the node names no RM type
// ([heldType]), "" for the root.
func (w *walker) compileNode(n template.Node, parent *CompiledNode, path, heldAs string) (*CompiledNode, error) {
	cn := &CompiledNode{parent: parent, aqlPath: path}

	// A repeated path means this node shares an AQL path with one already
	// walked (alternatives, or a same-node_id sibling). Mark the descent
	// so registerPath admits the descendants that necessarily collide;
	// the counter is dropped again before this node registers itself.
	_, inDup := w.paths[cn.aqlPath]
	if inDup {
		w.dupDepth++
	} else {
		w.paths[cn.aqlPath] = pathMark{}
	}
	err := w.descend(n, cn, heldAs)
	if inDup {
		w.dupDepth--
	}
	if err != nil {
		return nil, err
	}
	resolveWalkFacts(cn)

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
// check. heldAs is as for compileNode.
func (w *walker) descend(n template.Node, cn *CompiledNode, heldAs string) error {
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
		// Terms already copies each term and its items.
		cn.terms = v.Terms()
		w.compiled.termBindings = append(w.compiled.termBindings, v.TermBindings()...)
		return w.attachAttributes(cn, readAs(cn.rmTypeName, heldAs), v.Attributes())
	case *template.ComplexObject:
		cn.rmTypeName = v.RMTypeName()
		cn.nodeID = v.NodeID()
		cn.occurrences = v.Occurrences()
		cn.primitive = v.PrimitiveConstraint()
		return w.attachAttributes(cn, readAs(cn.rmTypeName, heldAs), v.Attributes())
	case *template.Slot:
		cn.rmTypeName = v.RMTypeName()
		cn.nodeID = v.NodeID()
		cn.slot = &slotInfo{
			includes: v.Includes(),
			excludes: v.Excludes(),
			rules:    v.SlotRules(),
		}
		return nil
	default:
		return fmt.Errorf("templatecompile: unhandled wire node type %T", n)
	}
}

// readAs returns the RM type the attributes of a node are read on: the
// type declared, the one the template writes for the node, or, where the
// template writes none, heldAs, the type of the attribute that holds the
// node ([heldType]).
func readAs(declared, heldAs string) string {
	if strings.TrimSpace(declared) == "" {
		return heldAs
	}
	return declared
}

// heldType returns the RM type an untyped child of an attribute is read
// as: attrType, the type the attribute has on a node whose bare class is
// class ([walker.attributeType]), or, where that is a formal generic
// parameter of class the node's type leaves open, such as the T of a
// POINT_EVENT's data, the type the BMM constrains that parameter to
// (ITEM_STRUCTURE). It is "" where neither is known.
func heldType(class, attrType string) string {
	if constraint, formal := bmmtype.ParameterConstraint(class, attrType); formal {
		return constraint
	}
	return attrType
}

// attachAttributes builds the explicit attributes of cn from the
// OPT-declared list, then (unless disabled) injects implicit
// attributes for every RM-declared field the OPT did not name.
// owner is the RM type the attributes are read on ([readAs]): cn's own,
// or, for a node that names none, the type of the attribute that holds it.
// Order: OPT-declared first (document order), then implicit
// (sorted by RM declaration order from rminfo).
func (w *walker) attachAttributes(cn *CompiledNode, owner string, declared []*template.Attribute) error {
	// rminfo knows each class by its bare BMM name; the OPT may declare a
	// generic instantiation (DV_INTERVAL<DV_QUANTITY>) that it does not.
	class := bmmtype.Class(owner)
	var required []string
	if owner != "" {
		required = w.requiredAttributes(class)
	}
	// The lookup lists the required attributes in RM declaration order,
	// so implicit attributes appear in BMM-stable order regardless of OPT
	// walk path. Only the attributes the RM mandates are injected: the
	// composition builder needs implicit entries for required-but-OPT-silent
	// fields alone, and optional ones would inflate every node with RM
	// metadata that walker code does not need.
	var implicit []string
	if !w.opts.SkipImplicitAttributes && owner != "" {
		implicit = required
	}
	// Room for every attribute up front, so appending never grows the
	// slice; a node with none keeps a nil slice.
	if n := len(declared) + countUndeclared(implicit, declared); n > 0 {
		cn.attributes = make([]*CompiledAttribute, 0, n)
	}
	for _, a := range declared {
		ca, err := w.buildAttribute(cn, owner, class, required, a)
		if err != nil {
			return err
		}
		cn.attributes = append(cn.attributes, ca)
	}
	for _, attrName := range implicit {
		if declares(declared, attrName) {
			continue
		}
		if ca := w.implicitAttribute(owner, class, attrName); ca != nil {
			cn.attributes = append(cn.attributes, ca)
		}
	}
	return nil
}

// declares reports whether declared holds an attribute named name.
func declares(declared []*template.Attribute, name string) bool {
	return slices.ContainsFunc(declared, func(a *template.Attribute) bool { return a.Name() == name })
}

// countUndeclared returns how many of names declared does not hold.
func countUndeclared(names []string, declared []*template.Attribute) int {
	n := 0
	for _, name := range names {
		if !declares(declared, name) {
			n++
		}
	}
	return n
}

// implicitAttribute returns the implicit attribute attr of a node whose
// attributes are read on owner ([readAs]) and whose bare class is class, or
// nil when the lookup gives the attribute no RM type. It is made once per
// owner type and attribute name in a call, and shared by the nodes of that
// type.
func (w *walker) implicitAttribute(owner, class, attr string) *CompiledAttribute {
	key := implicitKey{owner: owner, attr: attr}
	if ca, ok := w.implicit[key]; ok {
		return ca
	}
	var ca *CompiledAttribute
	if rm, ok := w.attributeType(owner, class, attr); ok && rm != "" {
		container, _ := w.lookup.IsContainer(class, attr)
		card := template.Single
		if container {
			card = template.Multiple
		}
		ca = &CompiledAttribute{
			name:        attr,
			cardinality: card,
			rmTypeName:  rm,
			implicit:    true,
			required:    true,
		}
	}
	w.implicit[key] = ca
	return ca
}

// requiredAttributes returns the attributes the lookup says the bare RM
// class class requires, in RM declaration order, asking the lookup once
// per class and call. The caller must not change the slice.
func (w *walker) requiredAttributes(class string) []string {
	required, ok := w.required[class]
	if !ok {
		required = w.lookup.RequiredAttributes(class)
		w.required[class] = required
	}
	return required
}

// buildAttribute compiles one OPT-declared attribute, recursing
// into its children. The attribute's BMM RM type is looked up so
// downstream consumers can resolve type-aware constraints without a
// separate rminfo query. owner is the RM type parent's attributes are
// read on ([readAs]), class its bare class, and required the attributes
// that class requires.
func (w *walker) buildAttribute(parent *CompiledNode, owner, class string, required []string, a *template.Attribute) (*CompiledAttribute, error) {
	rm, isProperty := w.attributeType(owner, class, a.Name())
	ca := &CompiledAttribute{
		name:              a.Name(),
		cardinality:       a.Cardinality(),
		existence:         a.Existence(),
		childMultiplicity: a.ChildMultiplicity(),
		rmTypeName:        rm,
		required:          slices.Contains(required, a.Name()),
		notInRM:           !isProperty && w.absentFrom(class, a.Name()),
	}
	prevAttr := w.currentAttr
	w.currentAttr = a
	defer func() { w.currentAttr = prevAttr }()
	children := a.Children()
	if len(children) > 0 {
		ca.children = make([]*CompiledNode, 0, len(children))
	}
	// The root's path is "/", and a child of the root starts its own.
	base := parent.aqlPath
	if base == "/" {
		base = ""
	}
	heldAs := heldType(class, rm)
	for i, child := range children {
		cn, err := w.compileNode(child, parent, childPath(base, a.Name(), a.Cardinality(), child, i), heldAs)
		if err != nil {
			return nil, err
		}
		ca.children = append(ca.children, cn)
	}
	return ca, nil
}

// attributeType resolves the RM type of attribute attr on a node whose
// attributes are read on owner ([readAs]) and whose bare class is class.
// rminfo is keyed by bare class name, so a generic instantiation is looked
// up by its class; and rminfo types an attribute the class declares with a
// formal parameter by that parameter's name (DV_INTERVAL.lower is "T"), so
// the actual parameter the declared name supplies takes its place
// (DV_QUANTITY for DV_INTERVAL<DV_QUANTITY>).
func (w *walker) attributeType(owner, class, attr string) (string, bool) {
	rm, ok := w.lookup.AttributeRMType(class, attr)
	if !ok {
		return "", false
	}
	return bmmtype.Substitute(owner, rm), true
}

// absentFrom reports whether attr, which the lookup does not give the bare
// class class as a property, is a name no class in class's hierarchy has
// (REQ-100, [CompiledAttribute.NotInRM]): it is not one of the functions
// [rminfo.IsNonStorableAttr] names for class, which a template may
// constrain as if it were an attribute, and it is neither a property nor
// such a function of any class descending from class, such as ITEM_TREE's
// items under an ITEM_STRUCTURE, or DV_QUANTITY's magnitude under a
// DV_AMOUNT, a function there that the predicate leaves out because its
// descendant stores it. The lookup's properties of class already
// hold those of its ancestors. A class the lookup does not know, and any
// class when the lookup gives no class hierarchy, gives no verdict, so its
// attributes are not marked.
func (w *walker) absentFrom(class, attr string) bool {
	if rminfo.IsNonStorableAttr(class, attr) {
		return false
	}
	descendants, ok := w.descendantsOf(class)
	if !ok {
		return false
	}
	for _, d := range descendants {
		if _, isProperty := w.lookup.AttributeRMType(d, attr); isProperty || rminfo.IsNonStorableAttr(d, attr) {
			return false
		}
	}
	return true
}

// descendantsOf returns every class the lookup says descends from the bare
// class class, abstract ones included, asking the lookup once per class and
// call. ok is false when the lookup does not know class or gives no class
// hierarchy ([rminfo.Hierarchy]).
func (w *walker) descendantsOf(class string) (descendants []string, ok bool) {
	if d, seen := w.descendants[class]; seen {
		return d, d != nil
	}
	if w.descendants == nil {
		w.descendants = map[string][]string{}
	}
	h, isHierarchy := w.lookup.(rminfo.Hierarchy)
	known := w.lookup.KnownRMTypes()
	if !isHierarchy || !slices.Contains(known, class) {
		w.descendants[class] = nil
		return nil, false
	}
	d := []string{}
	for _, k := range known {
		if conforms, _ := h.ConformsTo(k, class); conforms && k != class {
			d = append(d, k)
		}
	}
	w.descendants[class] = d
	return d, true
}

// childPath computes the AQL path of a child node reached from the
// parent path base ("" for the root) through the attribute attrName of
// cardinality card. For single attributes the child adds "/name"; for
// multiple attributes it also contributes a predicate (archetype id,
// at-code, slot include pattern, or a 1-based sibling suffix when the OPT
// omits all of the above), with the template-level node name appended to
// an id-derived predicate (see [namePredicated]). Each path is built in one
// concatenation.
func childPath(base, attrName string, card template.Cardinality, child template.Node, siblingIndex int) string {
	if card != template.Multiple {
		// No named node in the vendored corpus sits under a single
		// attribute, and no reference golden predicates one, so the
		// bare form stands: a name never *creates* a predicate.
		return base + "/" + attrName
	}
	if ar, ok := child.(*template.ArchetypeRoot); ok && ar.ArchetypeID() != "" {
		return base + "/" + attrName + "[" + namePredicated(ar.ArchetypeID(), child) + "]"
	}
	if id := child.NodeID(); id != "" {
		return base + "/" + attrName + "[" + namePredicated(id, child) + "]"
	}
	if sl, ok := child.(*template.Slot); ok {
		if p := slotPathPredicate(sl); p != "" {
			// Slots are not ObjectNodes and cannot pin a name.
			return base + "/" + attrName + "[" + p + "]"
		}
	}
	// The synthetic positional key is our own fallback, never a shape
	// the reference emits, and it already disambiguates siblings. No
	// corpus node reaches it *with* a name; if one ever does, Phase 4
	// parity is where the reference's actual shape would surface.
	return base + "/" + attrName + "[@" + strconv.Itoa(siblingIndex+1) + "]"
}

// NamePredicate appends a template-level node name to an id-derived path
// predicate, yielding the reference's `archetype_id,'Name'` form;
// it returns id unchanged when name is empty.
//
// It is exported inside the module because name-predicated paths are
// composed by two builders, this package's [childPath] and the
// WebTemplate builder's own `predicate`, and both must quote the same way.
//
// Measured across the vendored reference goldens: the name is always
// *appended* to an id (341 archetype-id + 9 at-code segments in the
// corona golden, 27 + 3 in GECCO) and never stands alone, which is why
// callers decorate an existing predicate rather than forming one.
//
// Quoting follows the goldens: the name sits in single quotes and commas
// inside it are literal; one corona name ("… zu Menschen, die dort
// waren") carries one. No vendored name contains a single quote or a
// backslash, so the escapes are the conventional AQL reading rather than
// a golden-verified rule; revisit if a corpus name ever needs them.
// Backslash is escaped first and quote second; the reverse order would
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
		if w.currentAttr != nil && w.paths[cn.aqlPath].attr == w.currentAttr {
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
	w.paths[cn.aqlPath] = pathMark{attr: w.currentAttr}
	return nil
}
