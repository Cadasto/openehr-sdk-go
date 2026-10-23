package composition_test

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/composition"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// guidelineOPT is a COMPOSITION whose one OBSERVATION names a required
// guideline_id, an OBJECT_REF whose namespace the template pins to
// guidelines.
const guidelineOPT = `<?xml version="1.0" encoding="utf-8"?>
<template xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xmlns="http://schemas.openehr.org/v1">
<language><terminology_id><value>ISO_639-1</value></terminology_id><code_string>en</code_string></language>
<template_id><value>guideline</value></template_id><concept>guideline</concept>
<definition><rm_type_name>COMPOSITION</rm_type_name><node_id>at0000</node_id>
<attributes xsi:type="C_MULTIPLE_ATTRIBUTE"><rm_attribute_name>content</rm_attribute_name>
<children xsi:type="C_ARCHETYPE_ROOT"><rm_type_name>OBSERVATION</rm_type_name><node_id>at0000</node_id>
<attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>guideline_id</rm_attribute_name>` + requiredExistence + `
<children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>OBJECT_REF</rm_type_name><node_id></node_id>
<attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>namespace</rm_attribute_name>` + requiredExistence + `
<children xsi:type="C_PRIMITIVE_OBJECT"><rm_type_name>STRING</rm_type_name><node_id></node_id><item xsi:type="C_STRING"><list>guidelines</list></item></children>
</attributes></children></attributes>
<archetype_id><value>openEHR-EHR-OBSERVATION.guideline.v1</value></archetype_id></children>
<cardinality><is_ordered>false</is_ordered><is_unique>false</is_unique><interval><lower_included>true</lower_included><lower_unbounded>false</lower_unbounded><upper_unbounded>true</upper_unbounded><lower>1</lower></interval></cardinality>
</attributes>
<archetype_id><value>openEHR-EHR-COMPOSITION.guideline.v1</value></archetype_id></definition>
</template>`

// TestREQ101_REQ107_GuidelineReferenceBuildsAndSets checks that a
// COMPOSITION whose template gives an OBSERVATION a guideline_id gets a
// Builder: the skeleton's guideline_id keeps the namespace the template
// pins and takes the type ANY and a HIER_OBJECT_ID whose value is drawn
// like a uid (REQ-107 RM defaults, Identity); Set replaces it with an
// OBJECT_REF the template admits, Build keeps that one, and the
// composition passes ValidateComposition.
func TestREQ101_REQ107_GuidelineReferenceBuildsAndSets(t *testing.T) {
	t.Parallel()
	c := compileInline(t, guidelineOPT)
	b, err := composition.NewBuilder(t.Context(), c,
		composition.WithTerritory("NL"),
		composition.WithComposer(testComposer()),
	)
	if err != nil {
		t.Fatalf("NewBuilder: %v, want a Builder", err)
	}
	observation := func(comp *rm.Composition) *rm.Observation {
		t.Helper()
		if len(comp.Content) != 1 {
			t.Fatalf("COMPOSITION.content has %d members, want 1", len(comp.Content))
		}
		obs, ok := comp.Content[0].(*rm.Observation)
		if !ok {
			t.Fatalf("COMPOSITION.content[0] is %T, want *rm.Observation", comp.Content[0])
		}
		return obs
	}
	skeleton, err := b.Build()
	if err != nil {
		t.Fatalf("Build of the bare skeleton: %v", err)
	}
	seeded := observation(skeleton)
	ref, ok := seeded.GuidelineID.(*rm.ObjectRef)
	if !ok {
		t.Fatalf("skeleton OBSERVATION.guideline_id is %T, want *rm.ObjectRef", seeded.GuidelineID)
	}
	id, ok := ref.ID.(*rm.HierObjectID)
	if !ok || id == nil || id.Value == "" {
		t.Fatalf("skeleton guideline_id id = %#v, want a HIER_OBJECT_ID with a value", ref.ID)
	}
	if uid, isHier := seeded.GetUID().(*rm.HierObjectID); isHier && uid != nil && uid.Value == id.Value {
		t.Errorf("skeleton guideline_id id %q is the OBSERVATION's own uid, want one drawn for it", id.Value)
	}
	if ref.Namespace != "guidelines" || ref.Type != "ANY" {
		t.Errorf("skeleton guideline_id = %s::%s, want guidelines::ANY", ref.Namespace, ref.Type)
	}

	const path = "/content[openEHR-EHR-OBSERVATION.guideline.v1]/guideline_id"
	set := &rm.ObjectRef{ID: &rm.GenericID{Value: "NICE-NG136", Scheme: "NICE"}, Namespace: "guidelines", Type: "GUIDELINE"}
	if err := b.Set(path, set); err != nil {
		t.Fatalf("Set(%s) = %v, want nil", path, err)
	}
	comp, err := b.Build()
	if err != nil {
		t.Fatalf("Build after Set = %v, want nil", err)
	}
	if got := observation(comp).GuidelineID; got != set {
		t.Errorf("built OBSERVATION.guideline_id = %#v, want the OBJECT_REF Set was given", got)
	}
	if r := validation.ValidateComposition(comp, c); !r.OK {
		t.Errorf("ValidateComposition: %+v, want no issues", r.Issues)
	}
}
