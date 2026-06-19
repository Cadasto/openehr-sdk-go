package rmpath

import (
	"errors"
	"fmt"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// Path-resolution sentinel errors. Detect with errors.Is.
var (
	// ErrPathNotFound is returned by ItemAtPath when the path resolves
	// to no item.
	ErrPathNotFound = errors.New("rmpath: path not found")
	// ErrPathAmbiguous is returned by ItemAtPath when the path resolves
	// to more than one item (use ItemsAtPath instead).
	ErrPathAmbiguous = errors.New("rmpath: path resolves to multiple items")
	// ErrPathSyntax is returned when the path string is not valid
	// openEHR path syntax.
	ErrPathSyntax = errors.New("rmpath: invalid path syntax")
)

// segment is one parsed path step: an attribute name with an optional
// archetype_node_id and/or name/value predicate.
type segment struct {
	attr   string
	nodeID string // archetype_node_id predicate ("" = no filter)
	name   string // name/value predicate ("" = no filter)
}

// ItemAtPath returns the single item at a unique path. It returns
// ErrPathNotFound when nothing matches, ErrPathAmbiguous when more than
// one item matches, and ErrPathSyntax for a malformed path. REQ-121.
func ItemAtPath(root rm.Locatable, path string) (any, error) {
	items, err := resolve(root, path)
	if err != nil {
		return nil, err
	}
	switch len(items) {
	case 0:
		return nil, fmt.Errorf("%w: %q", ErrPathNotFound, path)
	case 1:
		return items[0], nil
	default:
		return nil, fmt.Errorf("%w: %q (%d items)", ErrPathAmbiguous, path, len(items))
	}
}

// ItemsAtPath returns all items matching a (possibly non-unique) path,
// empty when none match. A malformed path returns ErrPathSyntax. REQ-121.
func ItemsAtPath(root rm.Locatable, path string) ([]any, error) {
	return resolve(root, path)
}

// PathExists reports whether the path resolves to at least one item.
// A malformed path reports false. REQ-121.
func PathExists(root rm.Locatable, path string) bool {
	items, err := resolve(root, path)
	return err == nil && len(items) > 0
}

// PathUnique reports whether the path resolves to exactly one item.
// A malformed path reports false. REQ-121.
func PathUnique(root rm.Locatable, path string) bool {
	items, err := resolve(root, path)
	return err == nil && len(items) == 1
}

// resolve walks root by the parsed path and returns the matching items.
func resolve(root rm.Locatable, path string) ([]any, error) {
	segs, err := parsePath(path)
	if err != nil {
		return nil, err
	}
	current := []any{root}
	for _, seg := range segs {
		next := make([]any, 0, len(current))
		for _, obj := range current {
			for _, child := range childrenAt(obj, seg.attr) {
				if matchesPredicate(child, seg) {
					next = append(next, child)
				}
			}
		}
		current = next
		if len(current) == 0 {
			break
		}
	}
	return current, nil
}

// matchesPredicate reports whether a candidate child satisfies the
// segment's archetype_node_id and/or name/value predicates.
func matchesPredicate(child any, seg segment) bool {
	if seg.nodeID != "" && nodeIDOf(child) != seg.nodeID {
		return false
	}
	if seg.name != "" && nameValueOf(child) != seg.name {
		return false
	}
	return true
}

// parsePath splits an openEHR path into segments. Splitting on "/"
// ignores slashes inside "[...]" predicates (e.g. name/value=…).
func parsePath(path string) ([]segment, error) {
	path = strings.TrimSpace(path)
	path = strings.TrimPrefix(path, "/")
	if path == "" {
		return nil, nil
	}
	var (
		segs  []segment
		depth int
		start int
	)
	flush := func(end int) error {
		raw := strings.TrimSpace(path[start:end])
		if raw == "" {
			return fmt.Errorf("%w: empty segment in %q", ErrPathSyntax, path)
		}
		seg, err := parseSegment(raw)
		if err != nil {
			return err
		}
		segs = append(segs, seg)
		return nil
	}
	for i, r := range path {
		switch r {
		case '[':
			depth++
		case ']':
			depth--
			if depth < 0 {
				return nil, fmt.Errorf("%w: unbalanced ']' in %q", ErrPathSyntax, path)
			}
		case '/':
			if depth == 0 {
				if err := flush(i); err != nil {
					return nil, err
				}
				start = i + 1
			}
		}
	}
	if depth != 0 {
		return nil, fmt.Errorf("%w: unterminated predicate in %q", ErrPathSyntax, path)
	}
	if err := flush(len(path)); err != nil {
		return nil, err
	}
	return segs, nil
}

// parseSegment parses one "attr" or "attr[predicate]" step.
func parseSegment(raw string) (segment, error) {
	i := strings.IndexByte(raw, '[')
	if i < 0 {
		return segment{attr: raw}, nil
	}
	if !strings.HasSuffix(raw, "]") {
		return segment{}, fmt.Errorf("%w: unterminated predicate in %q", ErrPathSyntax, raw)
	}
	attr := strings.TrimSpace(raw[:i])
	if attr == "" {
		return segment{}, fmt.Errorf("%w: predicate without attribute in %q", ErrPathSyntax, raw)
	}
	nodeID, name := parsePredicate(raw[i+1 : len(raw)-1])
	return segment{attr: attr, nodeID: nodeID, name: name}, nil
}

// parsePredicate interprets the text inside "[...]". It accepts a bare
// archetype_node_id, a quoted name, "node,'name'" / "node and
// name/value='name'", or a "name/value='x'" / "name='x'" expression.
func parsePredicate(s string) (nodeID, name string) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", ""
	}
	var parts []string
	switch {
	case strings.Contains(s, " and "):
		parts = strings.Split(s, " and ")
	default:
		parts = strings.Split(s, ",")
	}
	for _, p := range parts {
		p = strings.TrimSpace(p)
		switch {
		case p == "":
			continue
		case strings.ContainsRune(p, '='): // name/value='x' or name='x'
			name = unquote(strings.TrimSpace(p[strings.IndexByte(p, '=')+1:]))
		case isQuoted(p):
			name = unquote(p)
		default:
			nodeID = p
		}
	}
	return nodeID, name
}

func isQuoted(s string) bool {
	return len(s) >= 2 && (s[0] == '\'' || s[0] == '"') && s[len(s)-1] == s[0]
}

func unquote(s string) string {
	if isQuoted(s) {
		return s[1 : len(s)-1]
	}
	return s
}
