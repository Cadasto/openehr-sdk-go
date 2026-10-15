package bmmgen

import (
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/bmm"
)

// TestREQ043_PinnedContainersHaveNoFiniteUpper checks the premise the
// Cardinality paragraph of bmm-conformance.md § Mapping rules rests on: the
// generator keeps only cardinality.lower of a container property (as
// AttrMeta.Required), because no container in the pinned BMMs has a finite
// upper bound. Every container property of every class in every pinned
// schema root must state upper_unbounded and no upper. A BMM bump that
// brings a finite bound fails here instead of the bound being dropped
// silently.
//
// REQ-043: § Mapping rules, Cardinality.
func TestREQ043_PinnedContainersHaveNoFiniteUpper(t *testing.T) {
	for _, root := range pinnedSchemaRoots {
		schema, err := bmm.LoadAll(root, bmm.FSResolver{Root: testResources})
		if err != nil {
			t.Fatalf("LoadAll(%s): %v", root, err)
		}
		finite, containers := finiteContainerUppers(schema)
		t.Logf("%s: %d container properties", root, containers)
		if containers == 0 {
			t.Errorf("%s: no container property found, so the check covered nothing", root)
		}
		for _, attr := range finite {
			t.Errorf("%s: %s does not state an unbounded upper; rminfo keeps only the lower bound, so the validation floor would then need to check the upper bound too (REQ-112)", root, attr)
		}
	}
}

// TestREQ043_FiniteContainerUppersReportsABound runs [finiteContainerUppers]
// on an inline schema whose container properties have a finite upper, an
// unbounded upper and no cardinality, on a class and on an interface. Each
// one that does not state an unbounded upper is reported.
func TestREQ043_FiniteContainerUppersReportsABound(t *testing.T) {
	const src = `{
		"bmm_version": "2.4", "rm_publisher": "example", "schema_name": "bounds", "rm_release": "1.0.0",
		"class_definitions": {
			"HOLDER": {
				"name": "HOLDER",
				"properties": {
					"bounded": {
						"_type": "P_BMM_CONTAINER_PROPERTY", "name": "bounded",
						"type_def": {"container_type": "List", "type": "String"},
						"cardinality": {"lower": 1, "upper": 5}
					},
					"open": {
						"_type": "P_BMM_CONTAINER_PROPERTY", "name": "open",
						"type_def": {"container_type": "List", "type": "String"},
						"cardinality": {"lower": 0, "upper_unbounded": true}
					},
					"unstated": {
						"_type": "P_BMM_CONTAINER_PROPERTY", "name": "unstated",
						"type_def": {"container_type": "List", "type": "String"}
					},
					"single": {
						"_type": "P_BMM_SINGLE_PROPERTY", "name": "single", "type": "String"
					}
				}
			},
			"SHAPE": {
				"_type": "P_BMM_INTERFACE", "name": "SHAPE",
				"properties": {
					"corners": {
						"_type": "P_BMM_CONTAINER_PROPERTY", "name": "corners",
						"type_def": {"container_type": "Set", "type": "String"},
						"cardinality": {"lower": 0, "upper": 4}
					}
				}
			}
		}
	}`
	schema, err := bmm.Load(strings.NewReader(src))
	if err != nil {
		t.Fatalf("bmm.Load(inline schema): %v", err)
	}
	finite, containers := finiteContainerUppers(schema)
	want := []string{
		"HOLDER.bounded (cardinality 1..5)",
		"HOLDER.unstated (no cardinality)",
		"SHAPE.corners (cardinality 0..4)",
	}
	if !slices.Equal(finite, want) {
		t.Errorf("finiteContainerUppers(inline schema) = %q, want %q", finite, want)
	}
	if containers != 4 {
		t.Errorf("finiteContainerUppers(inline schema) counted %d container properties, want 4", containers)
	}
}

// finiteContainerUppers returns, sorted, each container property of a class
// or primitive type in schema whose cardinality does not state an unbounded
// upper, as "CLASS.attribute (cardinality lower..upper)", or "(no
// cardinality)" when it states none. It also returns how many container
// properties it looked at.
func finiteContainerUppers(schema *bmm.Schema) (finite []string, containers int) {
	for _, classes := range []map[string]bmm.Class{schema.ClassDefinitions, schema.PrimitiveTypes} {
		for _, name := range slices.Sorted(maps.Keys(classes)) {
			props := declaredProperties(classes[name])
			for _, propName := range slices.Sorted(maps.Keys(props)) {
				cp, ok := props[propName].(*bmm.ContainerProperty)
				if !ok {
					continue
				}
				containers++
				card := cp.Cardinality
				switch {
				case card == nil:
					finite = append(finite, fmt.Sprintf("%s.%s (no cardinality)", name, propName))
				case card.Upper != nil || !card.UpperUnbounded:
					finite = append(finite, fmt.Sprintf("%s.%s (cardinality %d..%s)", name, propName, card.Lower, upperText(card)))
				}
			}
		}
	}
	slices.Sort(finite)
	return finite, containers
}

// declaredProperties returns the properties class c declares itself, for
// each class kind that carries properties.
func declaredProperties(c bmm.Class) map[string]bmm.Property {
	switch cls := c.(type) {
	case *bmm.SimpleClass:
		return cls.Properties
	case *bmm.Interface:
		return cls.Properties
	case *bmm.Enumeration:
		return cls.Properties
	default:
		return nil
	}
}

// upperText renders the upper bound of card: its value, "*" when it is
// unbounded, or "?" when it states neither.
func upperText(card *bmm.Cardinality) string {
	switch {
	case card.Upper != nil:
		return strconv.Itoa(*card.Upper)
	case card.UpperUnbounded:
		return "*"
	default:
		return "?"
	}
}
