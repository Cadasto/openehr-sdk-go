package templatecompile

// defaults_from_model.go: the default values of the template's
// <constraints>, each placed on the compiled node its T_ATTRIBUTE addresses.

import (
	json "encoding/json/v2"
	"errors"
	"fmt"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/aom/aom14"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
)

// placeDefaultValues places each default value of the constraints tc on the
// node of c it addresses: the first child of the attribute named
// rm_attribute_name, the name read as [attributeName] reads it, on the node
// that the T_ATTRIBUTE's differential_path reaches ([differentialPathNode]),
// whose RM type is the T_COMPLEX_OBJECT's, or its default value's where it
// names none. A T_COMPLEX_OBJECT with no default value places nothing, so
// its T_ATTRIBUTE is not resolved. The node keeps a copy of the value, so c
// shares none with the model.
//
// A default value that addresses no node, as its path reaches no node or
// that node has no attribute of the name or the attribute no child of the RM
// type, is skipped, or, when strict, refused with an error wrapping
// ErrInvalidInput. In both modes a value that does not encode and decode as
// canonical JSON is refused with an error wrapping ErrInvalidInput. An error
// names the place, which is template structure, and not the value.
func placeDefaultValues(c *Compiled, tc *aom14.TConstraint, strict bool) error {
	if tc == nil {
		return nil
	}
	for i := range tc.Attributes {
		ta := &tc.Attributes[i]
		var (
			attr    *CompiledAttribute
			attrErr error
		)
		for j := range ta.Children {
			tco := &ta.Children[j]
			if tco.DefaultValue == nil {
				continue
			}
			if attr == nil && attrErr == nil {
				attr, attrErr = constrainedAttribute(c, ta)
			}
			rmType := strings.TrimSpace(tco.RMTypeName)
			if rmType == "" {
				rmType, _ = rm.RMTypeName(tco.DefaultValue)
			}
			target, err := defaultValueTarget(attr, attrErr, rmType)
			if err != nil {
				if strict {
					return fmt.Errorf("%w: default value at %q addresses no node: %w", ErrInvalidInput, ta.DifferentialPath, err)
				}
				continue
			}
			encoded, err := encodeDefaultValue(tco.DefaultValue)
			if err != nil {
				return fmt.Errorf("%w: default value of RM type %q at %q: %T does not encode and decode as canonical JSON",
					ErrInvalidInput, rmType, ta.DifferentialPath, tco.DefaultValue)
			}
			target.defaultValue = encoded
		}
	}
	return nil
}

// defaultValueTarget returns the first child of attr whose RM type is rmType,
// the node a default value of that RM type is placed on, or the error that
// says why there is none: attrErr, where the T_ATTRIBUTE addresses no
// attribute, and otherwise that attr has no such child.
func defaultValueTarget(attr *CompiledAttribute, attrErr error, rmType string) (*CompiledNode, error) {
	if attrErr != nil {
		return nil, attrErr
	}
	if rmType == "" {
		return nil, errors.New("neither the constraint object nor its default value names an RM type")
	}
	for _, child := range attr.children {
		if child.rmTypeName == rmType {
			return child, nil
		}
	}
	return nil, fmt.Errorf("attribute %q has no child of RM type %q", attr.name, rmType)
}

// constrainedAttribute returns the attribute of c that the T_ATTRIBUTE ta
// addresses: the attribute named rm_attribute_name on the node its
// differential_path reaches ([placeDefaultValues]). As [attributeName] reads
// an ELEMENT's null_flavor as null_flavour unless the ELEMENT declares
// both, a null_flavor the node does not hold finds its null_flavour. An
// error says why ta addresses no attribute.
func constrainedAttribute(c *Compiled, ta *aom14.TAttribute) (*CompiledAttribute, error) {
	node, err := differentialPathNode(c, ta.DifferentialPath)
	if err != nil {
		return nil, err
	}
	name := strings.TrimSpace(ta.RMAttributeName)
	attr := node.Attribute(name)
	if attr == nil && node.rmTypeName == "ELEMENT" && name == "null_flavor" {
		attr = node.Attribute("null_flavour")
	}
	if attr == nil {
		return nil, fmt.Errorf("the node has no attribute %q", name)
	}
	return attr, nil
}

// differentialPathNode returns the node of c that the differential_path path
// of a T_ATTRIBUTE reaches. A path that opens with the archetype id of c's
// root in brackets resolves from the rest, and one that opens with a slash
// from c's root, by the rules [Compiled.Resolve] applies, except that a
// segment's predicate may also carry a name ([parsePredicate]), which
// selects a node of that node id or archetype id whose template-level name
// it is. The path is read without its leading and trailing white space. An
// error says why path reaches no node.
func differentialPathNode(c *Compiled, path string) (*CompiledNode, error) {
	rest := strings.TrimSpace(path)
	switch {
	case strings.HasPrefix(rest, "["):
		id, after, ok := strings.Cut(rest[1:], "]")
		if !ok {
			return nil, errors.New("the archetype id that opens the path does not close")
		}
		if id = strings.TrimSpace(id); id != c.root.archetypeID {
			return nil, fmt.Errorf("the path opens with the archetype id %q, not the definition's %q", id, c.root.archetypeID)
		}
		rest = after
	case strings.HasPrefix(rest, "/"):
	default:
		return nil, errors.New("the path opens with neither the definition's archetype id nor a slash")
	}
	n := c.root
	if rest == "" || rest == "/" {
		return n, nil
	}
	for rest != "" {
		seg, after, err := nextSegment(rest)
		if err != nil {
			return nil, err
		}
		rest = after
		if n, err = stepTo(n, seg); err != nil {
			return nil, err
		}
	}
	return n, nil
}

// differentialSegment is one segment of a differential path: the name of an
// attribute and, where the segment has a predicate, the node id or
// archetype id and the template-level name it selects a child by, the name
// "" where the predicate gives none.
type differentialSegment struct {
	attr      string
	predicate bool
	id, name  string
}

// nextSegment reads the segment that opens path, a slash, an attribute name
// and an optional predicate in brackets, and returns it and the rest of
// path. Inside a predicate a closing bracket within single quotes, or after
// a backslash there, does not close it.
func nextSegment(path string) (differentialSegment, string, error) {
	if !strings.HasPrefix(path, "/") {
		return differentialSegment{}, "", fmt.Errorf("a segment does not open with a slash at %q", path)
	}
	path = path[1:]
	end := strings.IndexAny(path, "/[")
	if end < 0 {
		end = len(path)
	}
	seg := differentialSegment{attr: path[:end]}
	path = path[end:]
	if !strings.HasPrefix(path, "[") {
		return seg, path, nil
	}
	closing := predicateEnd(path)
	if closing < 0 {
		return differentialSegment{}, "", fmt.Errorf("the predicate of the segment %q does not close", seg.attr)
	}
	id, name, err := parsePredicate(path[1:closing])
	if err != nil {
		return differentialSegment{}, "", err
	}
	seg.predicate, seg.id, seg.name = true, id, name
	return seg, path[closing+1:], nil
}

// predicateEnd returns the index in s, which opens with the bracket of a
// predicate, of the bracket that closes it, or -1 when none does.
func predicateEnd(s string) int {
	quoted := false
	for i := 1; i < len(s); i++ {
		switch {
		case quoted && s[i] == '\\':
			i++ // the escaped byte
		case s[i] == '\'':
			quoted = !quoted
		case !quoted && s[i] == ']':
			return i
		}
	}
	return -1
}

// parsePredicate reads the text of a predicate, each run of white space in it
// read as one space: a node id or archetype id, alone or followed by a name
// in single quotes, after a comma (`at0004, 'Systolic'`) or in an
// `and name/value=` clause (`at0004 and name/value='Systolic'`). A backslash
// in the name escapes the byte after it.
func parsePredicate(text string) (id, name string, err error) {
	p := strings.Join(strings.Fields(text), " ")
	malformed := fmt.Errorf("the predicate %q is not a node id with an optional name", p)
	end := strings.IndexAny(p, ", ")
	if end < 0 {
		end = len(p)
	}
	id, rest := p[:end], strings.TrimPrefix(p[end:], " ")
	if id == "" {
		return "", "", malformed
	}
	switch {
	case rest == "":
		return id, "", nil
	case strings.HasPrefix(rest, ","):
		rest = rest[1:]
	case strings.HasPrefix(rest, "and name/value"):
		clause := strings.TrimPrefix(rest[len("and name/value"):], " ")
		var ok bool
		if rest, ok = strings.CutPrefix(clause, "="); !ok {
			return "", "", malformed
		}
	default:
		return "", "", malformed
	}
	name, ok := quotedName(strings.TrimSpace(rest))
	if !ok {
		return "", "", malformed
	}
	return id, name, nil
}

// quotedName returns the text that s, single quotes around it, holds, each
// backslash escaping the byte after it, and false when s is not quoted so.
func quotedName(s string) (string, bool) {
	if len(s) < 2 || s[0] != '\'' || s[len(s)-1] != '\'' {
		return "", false
	}
	var b strings.Builder
	for i := 1; i < len(s)-1; i++ {
		if s[i] == '\\' {
			i++
		}
		b.WriteByte(s[i])
	}
	return b.String(), true
}

// stepTo returns the child of n that seg selects: under n's attribute of the
// segment's name, the first child, where the segment has no predicate, or
// the first child whose node id or archetype id is the predicate's and whose
// template-level name, where the predicate gives one, is its name. A slot
// has no attributes, so a path does not descend into one.
func stepTo(n *CompiledNode, seg differentialSegment) (*CompiledNode, error) {
	attr := n.Attribute(seg.attr)
	if attr == nil {
		return nil, fmt.Errorf("no attribute %q at %q", seg.attr, n.aqlPath)
	}
	for _, child := range attr.children {
		if !seg.predicate {
			return child, nil
		}
		if (child.nodeID == seg.id || child.archetypeID == seg.id) && (seg.name == "" || child.nodeName == seg.name) {
			return child, nil
		}
	}
	if seg.name != "" {
		return nil, fmt.Errorf("no child %q named %q under %q at %q", seg.id, seg.name, seg.attr, n.aqlPath)
	}
	if seg.predicate {
		return nil, fmt.Errorf("no child %q under %q at %q", seg.id, seg.attr, n.aqlPath)
	}
	return nil, fmt.Errorf("attribute %q at %q has no children", seg.attr, n.aqlPath)
}

// encodeDefaultValue returns the canonical JSON of v, once it has decoded it
// back, so that [CompiledNode.DefaultValue] can decode it without failing.
func encodeDefaultValue(v rm.DataValue) ([]byte, error) {
	b, err := json.Marshal(v, typereg.EncodeOptions())
	if err != nil {
		return nil, err
	}
	if _, err := typereg.DecodeAs[rm.DataValue](b); err != nil {
		return nil, err
	}
	return b, nil
}
