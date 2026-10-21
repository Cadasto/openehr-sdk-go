package composition_test

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/composition"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// participationOPT is a COMPOSITION whose EVENT_CONTEXT holds one
// PARTICIPATION, with a DV_TEXT function and a DV_CODED_TEXT mode, each
// required.
const participationOPT = `<?xml version="1.0" encoding="utf-8"?>
<template xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xmlns="http://schemas.openehr.org/v1">
<language><terminology_id><value>ISO_639-1</value></terminology_id><code_string>en</code_string></language>
<template_id><value>participation</value></template_id><concept>participation</concept>
<definition><rm_type_name>COMPOSITION</rm_type_name><node_id>at0000</node_id>
<attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>context</rm_attribute_name>` + requiredExistence + `
<children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>EVENT_CONTEXT</rm_type_name><node_id></node_id>
<attributes xsi:type="C_MULTIPLE_ATTRIBUTE"><rm_attribute_name>participations</rm_attribute_name>
<children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>PARTICIPATION</rm_type_name><node_id></node_id>
<attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>function</rm_attribute_name>` + requiredExistence + `
<children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>DV_TEXT</rm_type_name><node_id></node_id></children></attributes>
<attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>mode</rm_attribute_name>` + requiredExistence + `
<children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>DV_CODED_TEXT</rm_type_name><node_id></node_id></children></attributes>
</children>
<cardinality><is_ordered>false</is_ordered><is_unique>false</is_unique><interval><lower_included>true</lower_included><lower_unbounded>false</lower_unbounded><upper_unbounded>true</upper_unbounded><lower>1</lower></interval></cardinality>
</attributes></children></attributes>
<archetype_id><value>openEHR-EHR-COMPOSITION.participation.v1</value></archetype_id></definition>
</template>`

// requiredExistence is the existence of an attribute the template requires.
const requiredExistence = `<existence><lower_included>true</lower_included><upper_included>true</upper_included>` +
	`<lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded><lower>1</lower><upper>1</upper></existence>`

// TestREQ101_REQ107_ContextParticipationBuildsAndSets checks that a
// COMPOSITION whose template puts a PARTICIPATION in its context's
// participations gets a Builder: the skeleton carries that participation,
// with the mode default openehr 193|not specified| and a PARTY_SELF
// performer (REQ-107 RM defaults), Set writes a function and a mode the
// template admits at their own paths, Build keeps them, and the composition
// passes ValidateComposition.
func TestREQ101_REQ107_ContextParticipationBuildsAndSets(t *testing.T) {
	t.Parallel()
	c := compileInline(t, participationOPT)
	b, err := composition.NewBuilder(t.Context(), c,
		composition.WithTerritory("NL"),
		composition.WithComposer(testComposer()),
	)
	if err != nil {
		t.Fatalf("NewBuilder: %v, want a Builder", err)
	}
	participation := func(comp *rm.Composition) rm.Participation {
		t.Helper()
		if comp.Context == nil || len(comp.Context.Participations) != 1 {
			t.Fatalf("COMPOSITION.context = %+v, want one participation", comp.Context)
		}
		return comp.Context.Participations[0]
	}
	skeleton, err := b.Build()
	if err != nil {
		t.Fatalf("Build of the bare skeleton: %v", err)
	}
	notSpecified := rm.CodePhrase{CodeString: "193", TerminologyID: rm.TerminologyID{Value: "openehr"}}
	seeded := participation(skeleton)
	if seeded.Mode == nil || seeded.Mode.DefiningCode != notSpecified || seeded.Mode.Value != "not specified" {
		t.Errorf("skeleton PARTICIPATION.mode = %+v, want openehr 193 not specified", seeded.Mode)
	}
	if _, ok := seeded.Performer.(*rm.PartySelf); !ok {
		t.Errorf("skeleton PARTICIPATION.performer is %T, want *rm.PartySelf", seeded.Performer)
	}

	const functionPath, modePath = "/context/participations/function", "/context/participations/mode"
	function := &rm.DVText{Value: "requester"}
	mode := &rm.DVCodedText{
		Value:        "face-to-face communication",
		DefiningCode: rm.CodePhrase{CodeString: "216", TerminologyID: rm.TerminologyID{Value: "openehr"}},
	}
	if err := b.Set(functionPath, function); err != nil {
		t.Fatalf("Set(%s) = %v, want nil", functionPath, err)
	}
	if err := b.Set(modePath, mode); err != nil {
		t.Fatalf("Set(%s) = %v, want nil", modePath, err)
	}
	comp, err := b.Build()
	if err != nil {
		t.Fatalf("Build after Set = %v, want nil", err)
	}
	built := participation(comp)
	if built.Function == nil || built.Function.GetValue() != function.Value {
		t.Errorf("built PARTICIPATION.function = %+v, want the %q Set was given", built.Function, function.Value)
	}
	if built.Mode == nil || built.Mode.DefiningCode != mode.DefiningCode {
		t.Errorf("built PARTICIPATION.mode = %+v, want the %+v Set was given", built.Mode, mode.DefiningCode)
	}
	if r := validation.ValidateComposition(comp, c); !r.OK {
		t.Errorf("ValidateComposition: %+v, want no issues", r.Issues)
	}
}
