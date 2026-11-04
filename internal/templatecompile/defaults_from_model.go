package templatecompile

// defaults_from_model.go: the default values of the template's
// <constraints>, each placed on the compiled node its T_ATTRIBUTE addresses.

import (
	json "encoding/json/v2"
	"fmt"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/aom/aom14"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
)

// placeDefaultValues places each default value of the constraints tc on the
// node of c it addresses: the first child, of the T_COMPLEX_OBJECT's RM type,
// of the attribute named rm_attribute_name on the node that the
// T_ATTRIBUTE's differential_path reaches, the name read as
// [attributeName] reads it. The path opens with the archetype
// id of c's root in brackets, and [Compiled.Resolve] resolves the rest. A
// T_COMPLEX_OBJECT with no default value places nothing, so its T_ATTRIBUTE
// is not resolved. The node keeps a copy of the value, so c shares none with
// the model.
//
// An error wraps ErrInvalidInput when the path of a default value does not
// open with the root's archetype id, does not parse or reaches no node, when
// that node has no attribute of the name or the attribute no child of the RM
// type, and when the value does not encode and decode as canonical JSON. It
// names the place, which is template structure, and not the value, and it
// wraps the error of the path's parse or resolution where one of those
// fails.
func placeDefaultValues(c *Compiled, tc *aom14.TConstraint) error {
	if tc == nil {
		return nil
	}
	for i := range tc.Attributes {
		ta := &tc.Attributes[i]
		var attr *CompiledAttribute
		for j := range ta.Children {
			tco := &ta.Children[j]
			if tco.DefaultValue == nil {
				continue
			}
			if attr == nil {
				a, err := constrainedAttribute(c, ta)
				if err != nil {
					return err
				}
				attr = a
			}
			rmType := strings.TrimSpace(tco.RMTypeName)
			node := firstChildOfType(attr, rmType)
			if node == nil {
				return fmt.Errorf("%w: default value at %q: attribute %q has no child of RM type %q",
					ErrInvalidInput, ta.DifferentialPath, attr.name, rmType)
			}
			encoded, err := encodeDefaultValue(tco.DefaultValue)
			if err != nil {
				return fmt.Errorf("%w: default value of RM type %q at %q: %T does not encode and decode as canonical JSON",
					ErrInvalidInput, rmType, ta.DifferentialPath, tco.DefaultValue)
			}
			node.defaultValue = encoded
		}
	}
	return nil
}

// constrainedAttribute returns the attribute of c that the T_ATTRIBUTE ta
// addresses: the attribute named rm_attribute_name on the node its
// differential_path reaches ([placeDefaultValues]). As [attributeName] reads
// an ELEMENT's null_flavor as null_flavour unless the ELEMENT declares
// both, a null_flavor the node does not hold finds its null_flavour.
func constrainedAttribute(c *Compiled, ta *aom14.TAttribute) (*CompiledAttribute, error) {
	where := ta.DifferentialPath
	id, rest, ok := strings.Cut(strings.TrimSpace(ta.DifferentialPath), "]")
	id, opened := strings.CutPrefix(id, "[")
	if !ok || !opened || strings.TrimSpace(id) != c.root.archetypeID {
		return nil, fmt.Errorf("%w: default value at %q: the path does not open with the definition's archetype id %q",
			ErrInvalidInput, where, c.root.archetypeID)
	}
	var p template.Path // the root path
	if rest != "" {
		var err error
		if p, err = template.ParsePath(rest); err != nil {
			return nil, fmt.Errorf("%w: default value at %q: %w", ErrInvalidInput, where, err)
		}
	}
	node, err := c.Resolve(p)
	if err != nil {
		return nil, fmt.Errorf("%w: default value at %q addresses no node: %w", ErrInvalidInput, where, err)
	}
	name := strings.TrimSpace(ta.RMAttributeName)
	attr := node.Attribute(name)
	if attr == nil && node.rmTypeName == "ELEMENT" && name == "null_flavor" {
		attr = node.Attribute("null_flavour")
	}
	if attr == nil {
		return nil, fmt.Errorf("%w: default value at %q: the node has no attribute %q", ErrInvalidInput, where, name)
	}
	return attr, nil
}

// firstChildOfType returns the first child of attr whose RM type is rmType,
// or nil when none is.
func firstChildOfType(attr *CompiledAttribute, rmType string) *CompiledNode {
	for _, child := range attr.children {
		if child.rmTypeName == rmType {
			return child
		}
	}
	return nil
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
