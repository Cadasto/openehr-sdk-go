package bmmgen

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/bmm"
)

// TestREQ043_PinnedContainersHaveNoFiniteUpper checks the premise the
// Cardinality paragraph of bmm-conformance.md § Mapping rules rests on: the
// generator keeps only cardinality.lower of a container property (as
// AttrMeta.Required), because no container in the pinned BMMs has a finite
// upper bound. No container property of a class, interface, enumeration or
// primitive type in any pinned schema root may state a finite upper. A BMM
// bump that brings a finite bound fails here instead of the bound being
// dropped silently. A container property with no cardinality passes: P_BMM
// reads a missing cardinality as |0..*|.
//
// REQ-041: § Mapping rules, Cardinality.
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
			t.Errorf("%s: %s states a finite upper; rminfo keeps only the lower bound, so the validation floor would then need to check the upper bound too (REQ-112)", root, attr)
		}
	}
}

// TestREQ043_FiniteContainerUppersReportsABound runs [finiteContainerUppers]
// on an inline schema whose container properties sit on a class, an
// interface, an enumeration and a primitive type. Each one that states a
// finite upper is reported. One with an unbounded upper, one with no
// cardinality and one whose cardinality states a lower and no upper are not:
// P_BMM reads a missing cardinality as |0..*|, and a cardinality with no
// upper carries no finite bound the generator could drop.
func TestREQ043_FiniteContainerUppersReportsABound(t *testing.T) {
	const src = `{
		"bmm_version": "2.4", "rm_publisher": "example", "schema_name": "bounds", "rm_release": "1.0.0",
		"primitive_types": {
			"PAIR": {
				"name": "PAIR",
				"properties": {
					"members": {
						"_type": "P_BMM_CONTAINER_PROPERTY", "name": "members",
						"type_def": {"container_type": "List", "type": "String"},
						"cardinality": {"lower": 2, "upper": 2}
					}
				}
			}
		},
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
					"lower_only": {
						"_type": "P_BMM_CONTAINER_PROPERTY", "name": "lower_only",
						"type_def": {"container_type": "List", "type": "String"},
						"cardinality": {"lower": 1}
					},
					"both": {
						"_type": "P_BMM_CONTAINER_PROPERTY", "name": "both",
						"type_def": {"container_type": "List", "type": "String"},
						"cardinality": {"lower": 0, "upper": 3, "upper_unbounded": true}
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
			},
			"STATUS": {
				"_type": "P_BMM_ENUMERATION_STRING", "name": "STATUS",
				"item_names": ["active", "retired"],
				"properties": {
					"aliases": {
						"_type": "P_BMM_CONTAINER_PROPERTY", "name": "aliases",
						"type_def": {"container_type": "List", "type": "String"},
						"cardinality": {"lower": 0, "upper": 2}
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
		"HOLDER.both (cardinality 0..3)",
		"HOLDER.bounded (cardinality 1..5)",
		"PAIR.members (cardinality 2..2)",
		"SHAPE.corners (cardinality 0..4)",
		"STATUS.aliases (cardinality 0..2)",
	}
	if !slices.Equal(finite, want) {
		t.Errorf("finiteContainerUppers(inline schema) = %q, want %q", finite, want)
	}
	for _, attr := range []string{"HOLDER.open", "HOLDER.unstated", "HOLDER.lower_only"} {
		if i := slices.IndexFunc(finite, func(f string) bool { return strings.HasPrefix(f, attr+" ") }); i >= 0 {
			t.Errorf("finiteContainerUppers(inline schema) reported %q; %s states no finite upper", finite[i], attr)
		}
	}
	if containers != 8 {
		t.Errorf("finiteContainerUppers(inline schema) counted %d container properties, want 8", containers)
	}
}

// finiteContainerUppers returns, sorted, each container property of a class
// or primitive type in schema whose cardinality states a finite upper, as
// "CLASS.attribute (cardinality lower..upper)". A property with no
// cardinality is not reported, because P_BMM reads a missing cardinality as
// |0..*|. Nor is one whose cardinality states no upper, with or without
// upper_unbounded: it carries no finite bound to drop. It also returns how
// many container properties it looked at.
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
				if card := cp.Cardinality; card != nil && card.Upper != nil {
					finite = append(finite, fmt.Sprintf("%s.%s (cardinality %d..%d)", name, propName, card.Lower, *card.Upper))
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
