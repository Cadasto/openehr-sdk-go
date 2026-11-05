package template

import (
	"fmt"
	"strings"
)

// Path is a parsed openEHR path. The zero value renders as "/" and
// resolves to the template root when passed to Compiled.Resolve in
// package templatecompile. A Path is not bound to any particular
// template; the same Path can be applied to multiple compiled
// templates. Construct non-root paths with [ParsePath].
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

// ParsePath parses an openEHR path string against the supported
// path grammar. It accepts:
//
//   - Absolute paths starting with '/'
//   - Segments naming RM attributes ("/content", "/data/events")
//   - Optional predicate per segment ("/content[at0001]" or
//     "/content[openEHR-EHR-OBSERVATION.blood_pressure.v1]")
//
// Rejects (with [ErrPathSyntax]): relative paths, trailing slashes,
// empty segments, multi-predicate constructs, AQL projection syntax
// (predicates with name= / @ / quoted values).
//
// ParsePath checks the grammar only; it does not check that segment
// names or predicates resolve against any template. To check a path
// against a template, compile the template with templatecompile.Compile
// and pass the Path to Compiled.ValidatePath, or to Compiled.Resolve
// when the resolved node is needed.
func ParsePath(s string) (Path, error) {
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
