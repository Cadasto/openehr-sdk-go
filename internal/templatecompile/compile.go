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
		templateID: opt.TemplateID(),
		concept:    opt.Concept(),
		uid:        opt.UID(),
		language:   opt.Language(),
		byPath:     make(map[string]*CompiledNode),
		byNodeID:   make(map[string][]*CompiledNode),
		byRMType:   make(map[string][]*CompiledNode),
	}

	w := walker{
		compiled: c,
		lookup:   o.Lookup,
		opts:     o,
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
		if err := w.attachAttributes(cn, v.Attributes()); err != nil {
			return nil, err
		}
	case *template.ComplexObject:
		cn.rmTypeName = v.RMTypeName()
		cn.nodeID = v.NodeID()
		cn.occurrences = v.Occurrences()
		cn.primitive = v.PrimitiveConstraint()
		if err := w.attachAttributes(cn, v.Attributes()); err != nil {
			return nil, err
		}
	case *template.Slot:
		cn.rmTypeName = v.RMTypeName()
		cn.nodeID = v.NodeID()
		cn.isSlot = true
		cn.slotIncludes = v.Includes()
		cn.slotExcludes = v.Excludes()
	default:
		return nil, fmt.Errorf("templatecompile: unhandled wire node type %T", n)
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
	return cn, nil
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
// all of the above).
func pathSegment(attrName string, card template.Cardinality, child template.Node, siblingIndex int) string {
	seg := "/" + attrName
	if card != template.Multiple {
		return seg
	}
	if ar, ok := child.(*template.ArchetypeRoot); ok && ar.ArchetypeID() != "" {
		return seg + "[" + ar.ArchetypeID() + "]"
	}
	if id := child.NodeID(); id != "" {
		return seg + "[" + id + "]"
	}
	if sl, ok := child.(*template.Slot); ok {
		if p := slotPathPredicate(sl); p != "" {
			return seg + "[" + p + "]"
		}
	}
	return seg + "[@" + strconv.Itoa(siblingIndex+1) + "]"
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
	if _, exists := w.compiled.byPath[cn.aqlPath]; exists {
		// AOM 1.4 admits C_SINGLE_ATTRIBUTE with multiple
		// `<children>` (alternatives); every alternative shares
		// the same AQL path. byPath only needs to resolve to one
		// representative for callers that ask "what's at this
		// path?" — by convention we keep the first. The
		// structural validator iterates alternatives via the
		// parent attribute's Children() directly, so dropping the
		// duplicate from byPath does not lose information.
		return nil
	}
	w.compiled.byPath[cn.aqlPath] = cn
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
