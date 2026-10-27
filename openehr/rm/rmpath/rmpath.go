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
// one item matches, and ErrPathSyntax for a malformed path.
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
// empty when none match. A malformed path returns ErrPathSyntax.
func ItemsAtPath(root rm.Locatable, path string) ([]any, error) {
	return resolve(root, path)
}

// PathExists reports whether the path resolves to at least one item.
// A malformed path reports false.
func PathExists(root rm.Locatable, path string) bool {
	items, err := resolve(root, path)
	return err == nil && len(items) > 0
}

// PathUnique reports whether the path resolves to exactly one item.
// A malformed path reports false.
func PathUnique(root rm.Locatable, path string) bool {
	items, err := resolve(root, path)
	return err == nil && len(items) == 1
}

// resolve walks root by the parsed path and returns the matching items.
func resolve(root rm.Locatable, path string) ([]any, error) {
	// The path is read first, so a malformed one is refused whatever the
	// root holds.
	segs, err := parsePath(path)
	if err != nil {
		return nil, err
	}
	if root == nil || isNilPointer(root) {
		return nil, nil
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
	var quote byte // the quote of the string being read in a predicate, 0 outside one
	for i := 0; i < len(path); i++ {
		c := path[i]
		if quote != 0 {
			switch c {
			case '\\':
				i++ // the escaped character is part of the string
			case quote:
				quote = 0
			}
			continue
		}
		switch c {
		case '\'', '"':
			if depth > 0 {
				quote = c
			}
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
	if quote != 0 {
		return nil, fmt.Errorf("%w: unterminated string in %q", ErrPathSyntax, path)
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
	attr, pred, ok := strings.Cut(raw, "[")
	if !ok {
		return segment{attr: raw}, nil
	}
	pred, ok = strings.CutSuffix(pred, "]")
	if !ok {
		return segment{}, fmt.Errorf("%w: unterminated predicate in %q", ErrPathSyntax, raw)
	}
	attr = strings.TrimSpace(attr)
	if attr == "" {
		return segment{}, fmt.Errorf("%w: predicate without attribute in %q", ErrPathSyntax, raw)
	}
	nodeID, name, err := parsePredicate(pred)
	if err != nil {
		return segment{}, fmt.Errorf("%w in %q", err, raw)
	}
	return segment{attr: attr, nodeID: nodeID, name: name}, nil
}

// parsePredicate interprets the text inside "[...]": a bare
// archetype_node_id, a quoted name, or both, as "node,'name'" or
// "node and name/value='name'"; the name may also be written
// "name/value='name'" or "name='name'". Terms are separated by a comma or by
// "and", in any case, outside quoted text, so a name may hold either. A
// quoted name is read as AQL reads a string: a backslash makes the character
// after it part of the name, so 'Patient\'s weight' is the name
// Patient's weight. Any other predicate, such as two node ids, two names,
// "or", or an operator other than "=", is refused with ErrPathSyntax: read
// as one of these forms, it would match items it does not select.
func parsePredicate(s string) (nodeID, name string, err error) {
	haveID, haveName := false, false
	rest := strings.TrimSpace(s)
	for rest != "" {
		var term string
		term, rest = nextTerm(rest)
		if term == "" {
			continue
		}
		if quoted, ok := quotedString(term); ok {
			if haveName {
				return "", "", fmt.Errorf("%w: two names in predicate %q", ErrPathSyntax, s)
			}
			name, haveName = quoted, true
			continue
		}
		if lhs, rhs, ok := strings.Cut(term, "="); ok {
			lhs, rhs = strings.TrimSpace(lhs), strings.TrimSpace(rhs)
			if lhs != "name/value" && lhs != "name" {
				return "", "", fmt.Errorf("%w: unsupported comparison %q in predicate %q", ErrPathSyntax, term, s)
			}
			if haveName {
				return "", "", fmt.Errorf("%w: two names in predicate %q", ErrPathSyntax, s)
			}
			if quoted, ok := quotedString(rhs); ok {
				rhs = quoted
			} else if strings.ContainsAny(rhs, "='\" \t") {
				return "", "", fmt.Errorf("%w: unsupported comparison %q in predicate %q", ErrPathSyntax, term, s)
			}
			name, haveName = rhs, true
			continue
		}
		if strings.ContainsAny(term, "'\" \t\n") {
			return "", "", fmt.Errorf("%w: unsupported term %q in predicate %q", ErrPathSyntax, term, s)
		}
		if haveID {
			return "", "", fmt.Errorf("%w: two node ids in predicate %q", ErrPathSyntax, s)
		}
		nodeID, haveID = term, true
	}
	return nodeID, name, nil
}

// nextTerm returns the first term of the predicate text s, trimmed, and the
// text after the separator that ends it: a comma, or "and" in any case with
// white space on both sides, outside quoted text. parsePath has checked that
// every quoted string is closed.
func nextTerm(s string) (term, rest string) {
	var quote byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		if quote != 0 {
			switch c {
			case '\\':
				i++
			case quote:
				quote = 0
			}
			continue
		}
		switch {
		case c == '\'' || c == '"':
			quote = c
		case c == ',':
			return strings.TrimSpace(s[:i]), s[i+1:]
		case isSpace(c):
			if after := strings.TrimLeft(s[i:], " \t\n\r"); len(after) > 3 && strings.EqualFold(after[:3], "and") && isSpace(after[3]) {
				return strings.TrimSpace(s[:i]), after[4:]
			}
		}
	}
	return strings.TrimSpace(s), ""
}

// quotedString reports whether s is one quoted string, in single or double
// quotes, and returns its text with each backslash escape replaced by the
// character it escapes.
func quotedString(s string) (string, bool) {
	if len(s) < 2 || (s[0] != '\'' && s[0] != '"') {
		return "", false
	}
	quote := s[0]
	var b strings.Builder
	for i := 1; i < len(s); i++ {
		switch c := s[i]; c {
		case '\\':
			if i+1 == len(s) {
				return "", false
			}
			i++
			b.WriteByte(s[i])
		case quote:
			// The string ends here, and so must the term.
			return b.String(), i == len(s)-1
		default:
			b.WriteByte(c)
		}
	}
	return "", false
}

// isSpace reports whether c is ASCII white space.
func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}
