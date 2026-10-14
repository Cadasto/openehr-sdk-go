package bmmgen

import (
	"context"
	"maps"
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/bmm"
)

// TestREQ052_MandatoryFieldJSONTag pins the JSON tag the generator gives a
// mandatory field of each Go kind, on real RM and AOM 1.4 fields. A list,
// map, pointer or any field takes omitzero; an interface or type-parameter
// field takes omitempty; a field held by value keeps the bare tag. The flat
// wire struct of a class whose ancestor bears a marshaler copies its fields
// through the same rule, so one of its rows is pinned too.
//
// REQ-052 § An absent mandatory member: a mandatory member with no value is
// left out on encode, never written as null.
func TestREQ052_MandatoryFieldJSONTag(t *testing.T) {
	rmPlan, err := BuildPlanForTarget(context.Background(), TargetRM, bmm.FSResolver{Root: testResources})
	if err != nil {
		t.Fatalf("BuildPlanForTarget(RM): %v", err)
	}
	aomPlan, err := BuildPlanForTarget(context.Background(), TargetAOM14, bmm.FSResolver{Root: testResources})
	if err != nil {
		t.Fatalf("BuildPlanForTarget(AOM14): %v", err)
	}

	for _, tc := range []struct {
		name     string
		plan     *Plan
		fileBase string
		render   func(*Plan, *PlannedFile) ([]byte, error)
		field    string
	}{
		{
			name: "slice, container property: CLUSTER.items", plan: rmPlan, render: RenderFile,
			fileBase: "data_structures_representation",
			field:    "Items []Item `json:\"items,omitzero\"`",
		},
		{
			name: "slice of an interface: CONTRIBUTION.versions", plan: rmPlan, render: RenderFile,
			fileBase: "common_change_control",
			field:    "Versions []ObjectRefLike `json:\"versions,omitzero\"`",
		},
		{
			name: "map, generic property over Hash: RESOURCE_DESCRIPTION.details", plan: rmPlan, render: RenderFile,
			fileBase: "common_resource",
			field:    "Details map[string]ResourceDescriptionItem `json:\"details,omitzero\"`",
		},
		{
			name: "pointer that breaks a cycle: ARCHETYPE.ontology", plan: aomPlan, render: RenderFile,
			fileBase: "archetype",
			field:    "Ontology *ArchetypeOntology `json:\"ontology,omitzero\"`",
		},
		{
			name: "any: EXPR_LEAF.item", plan: aomPlan, render: RenderFile,
			fileBase: "archetype_assertion",
			field:    "Item any `json:\"item,omitzero\"`",
		},
		{
			name: "narrow Like interface: ELEMENT.name", plan: rmPlan, render: RenderFile,
			fileBase: "data_structures_representation",
			field:    "Name DVTextLike `json:\"name,omitempty\"`",
		},
		{
			name: "abstract class interface: RESOURCE_DESCRIPTION.parent_resource", plan: rmPlan, render: RenderFile,
			fileBase: "common_resource",
			field:    "ParentResource AuthoredResource `json:\"parent_resource,omitempty\"`",
		},
		{
			name: "type parameter: EVENT.data", plan: rmPlan, render: RenderFile,
			fileBase: "data_structures_history",
			field:    "Data T `json:\"data,omitempty\"`",
		},
		{
			name: "generic struct by value: OBSERVATION.data", plan: rmPlan, render: RenderFile,
			fileBase: "composition_content_entry",
			field:    "Data History[ItemStructure] `json:\"data\"`",
		},
		{
			name: "struct by value: CONTRIBUTION.uid", plan: rmPlan, render: RenderFile,
			fileBase: "common_change_control",
			field:    "UID HierObjectID `json:\"uid\"`",
		},
		{
			name: "primitive by value: ELEMENT.archetype_node_id", plan: rmPlan, render: RenderFile,
			fileBase: "data_structures_representation",
			field:    "ArchetypeNodeID string `json:\"archetype_node_id\"`",
		},
		{
			name: "interface in a flat wire struct: VERSIONED_FOLDER.owner_id", plan: rmPlan, render: RenderMarshalJSONFile,
			fileBase: "common_directory",
			field:    "OwnerID ObjectRefLike `json:\"owner_id,omitempty\"`",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var file *PlannedFile
			for _, f := range tc.plan.Files {
				if f.FileBase == tc.fileBase {
					file = f
					break
				}
			}
			if file == nil {
				t.Fatalf("%s file not in plan", tc.fileBase)
			}
			got, err := tc.render(tc.plan, file)
			if err != nil {
				t.Fatalf("render %s: %v", tc.fileBase, err)
			}
			if !fieldDecl(tc.field).Match(got) {
				t.Errorf("the rendered %s source does not declare the field %q", tc.fileBase, tc.field)
			}
		})
	}
}

// TestREQ052_MandatoryJSONTagByGoType pins [mandatoryJSONTag] on each Go type
// shape it tells apart, including the ones no pinned schema emits today.
func TestREQ052_MandatoryJSONTagByGoType(t *testing.T) {
	for _, tc := range []struct {
		goType      string
		polymorphic bool
		want        string
	}{
		{goType: "[]Item", want: "`json:\"m,omitzero\"`"},
		{goType: "map[string]string", want: "`json:\"m,omitzero\"`"},
		{goType: "*ArchetypeOntology", want: "`json:\"m,omitzero\"`"},
		{goType: "*Archetype", polymorphic: true, want: "`json:\"m,omitzero\"`"},
		{goType: "any", want: "`json:\"m,omitzero\"`"},
		{goType: "any /* TODO: unmapped X */", want: "`json:\"m,omitzero\"`"},
		{goType: "any /* TODO: unknown type X */", want: "`json:\"m,omitzero\"`"},
		{goType: "DVTextLike", polymorphic: true, want: "`json:\"m,omitempty\"`"},
		{goType: "T", polymorphic: true, want: "`json:\"m,omitempty\"`"},
		{goType: "HierObjectID", want: "`json:\"m\"`"},
		{goType: "History[ItemStructure]", want: "`json:\"m\"`"},
		{goType: "string", want: "`json:\"m\"`"},
	} {
		if got := mandatoryJSONTag("m", tc.goType, tc.polymorphic); got != tc.want {
			t.Errorf("mandatoryJSONTag(%q, %q, %t) = %s, want %s", "m", tc.goType, tc.polymorphic, got, tc.want)
		}
	}
}

// TestREQ052_IsOwnTypeParameter pins [isOwnTypeParameter], which makes a
// mandatory single property typed by its class's own generic parameter
// polymorphic, so its field takes omitempty. No field generated today
// depends on it alone, so only these rows hold the rule.
func TestREQ052_IsOwnTypeParameter(t *testing.T) {
	// holder builds a class named name with a mandatory property "item"
	// typed T, declaring the generic parameters params.
	holder := func(name string, params ...string) *bmm.SimpleClass {
		item := &bmm.SingleProperty{TypeName: "T"}
		item.Name = "item"
		item.IsMandatory = true
		c := &bmm.SimpleClass{}
		c.Name = name
		c.Properties = map[string]bmm.Property{"item": item}
		for _, p := range params {
			if c.GenericParameterDefs == nil {
				c.GenericParameterDefs = map[string]*bmm.GenericParameterDef{}
			}
			c.GenericParameterDefs[p] = &bmm.GenericParameterDef{Name: p}
		}
		return c
	}

	for _, tc := range []struct {
		owner *bmm.SimpleClass
		want  bool
	}{
		{owner: holder("GENERIC_HOLDER", "T"), want: true},
		{owner: holder("PLAIN_HOLDER"), want: false},
	} {
		typeName := tc.owner.Properties["item"].(*bmm.SingleProperty).TypeName
		if got := isOwnTypeParameter(tc.owner, typeName); got != tc.want {
			t.Errorf("isOwnTypeParameter(%s with generic parameters %v, %q) = %t, want %t",
				tc.owner.Name, slices.Sorted(maps.Keys(tc.owner.GenericParameterDefs)), typeName, got, tc.want)
		}
	}
}

// TestREQ052_GenericRootIsInterface pins [genericRootIsInterface] on the
// real RM plan. A mandatory generic property whose root is EVENT is held as
// a Go interface and takes omitempty; one whose root is a generic struct
// (HISTORY) or a container (Hash) is not. No field generated today depends
// on it alone, so only these rows hold the rule.
func TestREQ052_GenericRootIsInterface(t *testing.T) {
	rmPlan, err := BuildPlanForTarget(t.Context(), TargetRM, bmm.FSResolver{Root: testResources})
	if err != nil {
		t.Fatalf("BuildPlanForTarget(RM): %v", err)
	}

	for _, tc := range []struct {
		root string
		want bool
	}{
		{root: "EVENT", want: true},
		{root: "HISTORY", want: false},
		{root: "Hash", want: false},
	} {
		gt := &bmm.GenericType{
			RootType:          tc.root,
			GenericParameters: []bmm.Type{&bmm.SimpleType{TypeName: "ITEM_STRUCTURE"}},
		}
		if got := genericRootIsInterface(rmPlan, gt); got != tc.want {
			t.Errorf("genericRootIsInterface(RM plan, %s<ITEM_STRUCTURE>) = %t, want %t", tc.root, got, tc.want)
		}
	}
}
