package template

import (
	"fmt"
	"strings"
)

// Path is a parsed openEHR path. The zero value renders as "/" and
// resolves to the template root when passed to
// [OperationalTemplate.NodeAt]. A Path is decoupled from any
// particular template — the same Path can be applied to multiple
// OperationalTemplates. Construct non-root paths via
// [OperationalTemplate.ParsePath].
type Path struct {
	segments []pathSegment
}

type pathSegment struct {
	name      string
	predicate string // at-code or archetype-id; empty if no predicate
}

// String returns the canonical text form of the path. The root path
// renders as "/"; other paths reproduce the input format with
// predicates restored.
func (p Path) String() string {
	if len(p.segments) == 0 {
		return "/"
	}
	var b strings.Builder
	for _, s := range p.segments {
		b.WriteByte('/')
		b.WriteString(s.name)
		if s.predicate != "" {
			b.WriteByte('[')
			b.WriteString(s.predicate)
			b.WriteByte(']')
		}
	}
	return b.String()
}

// IsRoot reports whether the path points to the template root.
func (p Path) IsRoot() bool { return len(p.segments) == 0 }

// ParsePath parses an openEHR path string against the grammar subset
// REQ-100 § Path syntax defines. Accepts:
//
//   - Absolute paths starting with '/'
//   - Segments naming RM attributes ("/content", "/data/events")
//   - Optional predicate per segment ("/content[at0001]" or
//     "/content[openEHR-EHR-OBSERVATION.blood_pressure.v1]")
//
// Rejects (with ErrPathSyntax): relative paths, trailing slashes,
// empty segments, multi-predicate constructs, AQL projection syntax
// (predicates with name= / @ / quoted values).
//
// ParsePath validates grammar only — it does not check that segment
// names or predicates resolve against any OPT. The template receiver
// is retained for API symmetry with NodeAt and for future template-
// aware path validation (see ValidatePath in the REQ-100 follow-up
// plan); today the grammar is template-independent.
func (t *OperationalTemplate) ParsePath(s string) (Path, error) {
	if s == "" {
		return Path{}, fmt.Errorf("%w: empty path", ErrPathSyntax)
	}
	if !strings.HasPrefix(s, "/") {
		return Path{}, fmt.Errorf("%w: must start with /", ErrPathSyntax)
	}
	if s == "/" {
		return Path{}, nil
	}
	if strings.HasSuffix(s, "/") {
		return Path{}, fmt.Errorf("%w: trailing slash", ErrPathSyntax)
	}

	var (
		segs   []pathSegment
		name   strings.Builder
		pred   strings.Builder
		inPred bool
	)

	flush := func() error {
		if name.Len() == 0 {
			return fmt.Errorf("%w: empty segment", ErrPathSyntax)
		}
		segs = append(segs, pathSegment{name: name.String(), predicate: pred.String()})
		name.Reset()
		pred.Reset()
		return nil
	}

	// Skip leading slash.
	for i := 1; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '/' && !inPred:
			if err := flush(); err != nil {
				return Path{}, err
			}
		case c == '[' && !inPred:
			if name.Len() == 0 {
				return Path{}, fmt.Errorf("%w: predicate without name", ErrPathSyntax)
			}
			inPred = true
		case c == ']' && inPred:
			inPred = false
			if pred.Len() == 0 {
				return Path{}, fmt.Errorf("%w: empty predicate", ErrPathSyntax)
			}
			if i+1 < len(s) && s[i+1] != '/' {
				return Path{}, fmt.Errorf("%w: unexpected %q after ]", ErrPathSyntax, s[i+1])
			}
		case c == '[' && inPred, c == ']' && !inPred:
			return Path{}, fmt.Errorf("%w: unbalanced bracket", ErrPathSyntax)
		case inPred:
			switch c {
			case ',', '=', '\'', '"', '@':
				return Path{}, fmt.Errorf("%w: unsupported predicate construct %q", ErrPathSyntax, c)
			default:
				pred.WriteByte(c)
			}
		default:
			name.WriteByte(c)
		}
	}
	if inPred {
		return Path{}, fmt.Errorf("%w: unclosed [", ErrPathSyntax)
	}
	if err := flush(); err != nil {
		return Path{}, err
	}
	return Path{segments: segs}, nil
}

// ResolveOption tunes path-resolution behaviour. Construct via the
// With* functions in this package. Options compose; passing the same
// option twice is idempotent.
type ResolveOption func(*resolveOpts)

type resolveOpts struct {
	strictPaths bool
}

// WithStrictPaths enables strict-mode path resolution: a
// predicate-less segment that matches an attribute with multiple
// candidate children returns ErrAmbiguousPath instead of silently
// picking the first child (REQ-100's documented default). Use this
// in validators and code generators that must surface ambiguity to
// the caller rather than guess.
func WithStrictPaths() ResolveOption {
	return func(o *resolveOpts) { o.strictPaths = true }
}

// NodeAt resolves a parsed path against the OPT definition tree.
// Returns ErrPathNotFound (wrapped) when any segment fails to match
// an attribute, when a predicate fails to match a child node id or
// archetype id, or when descent encounters a non-descendable node.
// Returns ErrAmbiguousPath when [WithStrictPaths] is set and a
// predicate-less segment matches multiple candidate children.
//
// Match rules:
//   - Segment names match attribute names exactly (case-sensitive).
//   - Predicates match against Node.NodeID() (at-codes) or
//     ArchetypeRoot.ArchetypeID() (full slot-fill archetype id).
//   - When a segment has no predicate and the matched attribute has
//     multiple children, the first child (document order) is taken
//     deterministically in lenient mode. In strict mode the call
//     returns ErrAmbiguousPath.
//
// The root path (Path{}) returns the template root unchanged.
func (t *OperationalTemplate) NodeAt(p Path, opts ...ResolveOption) (Node, error) {
	if t == nil || t.root == nil {
		return nil, fmt.Errorf("%w: empty template", ErrPathNotFound)
	}
	if p.IsRoot() {
		return t.root, nil
	}
	o := resolveOpts{}
	for _, opt := range opts {
		opt(&o)
	}
	return walkPath(t.root, p.segments, &o)
}

// ValidatePath reports whether p resolves against the OPT — a
// composed shorthand for [OperationalTemplate.NodeAt] that discards
// the resolved node. Returns the same sentinels as NodeAt
// (ErrPathNotFound; ErrAmbiguousPath when strict). Convenience for
// code-generator preconditions and call-site assertions that do not
// need the resolved node.
func (t *OperationalTemplate) ValidatePath(p Path, opts ...ResolveOption) error {
	_, err := t.NodeAt(p, opts...)
	return err
}

func walkPath(n Node, segs []pathSegment, o *resolveOpts) (Node, error) {
	if len(segs) == 0 {
		return n, nil
	}
	co, ok := descendableObject(n)
	if !ok {
		return nil, fmt.Errorf("%w: cannot descend into %T at %q", ErrPathNotFound, n, segs[0].name)
	}
	seg := segs[0]
	var attr *Attribute
	for _, a := range co.attributes {
		if a.name == seg.name {
			attr = a
			break
		}
	}
	if attr == nil {
		return nil, fmt.Errorf("%w: attribute %q", ErrPathNotFound, seg.name)
	}
	if len(attr.children) == 0 {
		return nil, fmt.Errorf("%w: attribute %q has no children", ErrPathNotFound, seg.name)
	}

	var matched Node
	if seg.predicate != "" {
		for _, child := range attr.children {
			if matchesPredicate(child, seg.predicate) {
				matched = child
				break
			}
		}
		if matched == nil {
			return nil, fmt.Errorf("%w: predicate %q under %q", ErrPathNotFound, seg.predicate, seg.name)
		}
	} else {
		if o.strictPaths && len(attr.children) > 1 {
			return nil, fmt.Errorf("%w: attribute %q has %d candidates (add a predicate)",
				ErrAmbiguousPath, seg.name, len(attr.children))
		}
		matched = attr.children[0]
	}

	if len(segs) == 1 {
		return matched, nil
	}
	return walkPath(matched, segs[1:], o)
}

// descendableObject returns the embedded ComplexObject of a node
// when its concrete type exposes RM attributes — *ComplexObject and
// *ArchetypeRoot (which wraps *ComplexObject by composition). *Slot
// and *Attribute return (nil, false); empty-attribute *ComplexObject
// values are descendable here and rejected later in walkPath with
// a per-segment ErrPathNotFound message.
func descendableObject(n Node) (*ComplexObject, bool) {
	switch v := n.(type) {
	case *ComplexObject:
		return v, true
	case *ArchetypeRoot:
		return &v.ComplexObject, true
	default:
		return nil, false
	}
}

func matchesPredicate(n Node, pred string) bool {
	if n.NodeID() == pred && pred != "" {
		return true
	}
	if ar, ok := n.(*ArchetypeRoot); ok && ar.ArchetypeID() == pred {
		return true
	}
	return false
}
