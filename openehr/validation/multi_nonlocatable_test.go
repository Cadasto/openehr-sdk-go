package validation_test

import (
	"fmt"
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// REQ-102 — an item of a multi-valued attribute that is not a LOCATABLE
// (a PARTICIPATION, a DV_TEXT in ACTOR.languages, a PARTY_REF in
// ACTOR.roles) carries no archetype_node_id. The walker binds it to the
// first non-slot OPT child whose RM type admits it, walks it, and counts
// it toward that child's occurrences; it never reports such an item as
// slot_fill. The vendored OPTs constrain none of these attributes, so
// the tests build small synthetic ones.

// participationsOPT constrains COMPOSITION.context.participations to one
// PARTICIPATION child with occurrences 0..1. The child declares no
// attributes, so its BMM-mandatory function and performer are implicit.
const participationsOPT = `<?xml version="1.0" encoding="utf-8"?>
<template xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xmlns="http://schemas.openehr.org/v1">
  <language>
    <terminology_id><value>ISO_639-1</value></terminology_id>
    <code_string>en</code_string>
  </language>
  <template_id><value>participations</value></template_id>
  <concept>participations</concept>
  <definition>
    <rm_type_name>COMPOSITION</rm_type_name>
    <node_id>at0000</node_id>
    <attributes xsi:type="C_SINGLE_ATTRIBUTE">
      <rm_attribute_name>context</rm_attribute_name>
      <children xsi:type="C_COMPLEX_OBJECT">
        <rm_type_name>EVENT_CONTEXT</rm_type_name>
        <node_id></node_id>
        <attributes xsi:type="C_MULTIPLE_ATTRIBUTE">
          <rm_attribute_name>participations</rm_attribute_name>
          <children xsi:type="C_COMPLEX_OBJECT">
            <rm_type_name>PARTICIPATION</rm_type_name>
            <occurrences>
              <lower_included>true</lower_included>
              <upper_included>true</upper_included>
              <lower_unbounded>false</lower_unbounded>
              <upper_unbounded>false</upper_unbounded>
              <lower>0</lower>
              <upper>1</upper>
            </occurrences>
            <node_id></node_id>
          </children>
        </attributes>
      </children>
    </attributes>
    <archetype_id><value>openEHR-EHR-COMPOSITION.participations.v1</value></archetype_id>
  </definition>
</template>`

// actorOPT returns a PERSON OPT whose identities admit one PARTY_IDENTITY
// (at0001), whose roles admit one PARTY_REF, and whose languages hold the
// given children XML.
func actorOPT(languagesChildren string) string {
	return `<?xml version="1.0"?>
<template xmlns="http://schemas.openehr.org/v1" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <template_id><value>actor-lists</value></template_id>
  <concept>actor-lists</concept>
  <language><terminology_id><value>ISO_639-1</value></terminology_id><code_string>en</code_string></language>
  <definition>
    <rm_type_name>PERSON</rm_type_name>
    <node_id>at0000</node_id>
    <attributes xsi:type="C_MULTIPLE_ATTRIBUTE">
      <rm_attribute_name>identities</rm_attribute_name>
      <children xsi:type="C_COMPLEX_OBJECT">
        <rm_type_name>PARTY_IDENTITY</rm_type_name>
        <node_id>at0001</node_id>
      </children>
    </attributes>
    <attributes xsi:type="C_MULTIPLE_ATTRIBUTE">
      <rm_attribute_name>languages</rm_attribute_name>` + languagesChildren + `
    </attributes>
    <attributes xsi:type="C_MULTIPLE_ATTRIBUTE">
      <rm_attribute_name>roles</rm_attribute_name>
      <children xsi:type="C_COMPLEX_OBJECT">
        <rm_type_name>PARTY_REF</rm_type_name>
        <node_id></node_id>
      </children>
    </attributes>
    <archetype_id><value>openEHR-DEMOGRAPHIC-PERSON.person.v1</value></archetype_id>
  </definition>
</template>`
}

// languagesChild renders one C_COMPLEX_OBJECT child of ACTOR.languages;
// an empty rmType renders an untyped child.
func languagesChild(rmType string) string {
	return fmt.Sprintf(`
      <children xsi:type="C_COMPLEX_OBJECT">
        <rm_type_name>%s</rm_type_name>
        <node_id></node_id>
      </children>`, rmType)
}

// participationsComposition returns a composition that satisfies
// participationsOPT apart from what parts carries.
func participationsComposition(parts ...rm.Participation) *rm.Composition {
	return &rm.Composition{
		ArchetypeNodeID: "openEHR-EHR-COMPOSITION.participations.v1",
		Name:            rm.DVText{Value: "Encounter"},
		Category: rm.DVCodedText{
			DVText: rm.DVText{Value: "event"},
			DefiningCode: rm.CodePhrase{
				TerminologyID: rm.TerminologyID{Value: "openehr"},
				CodeString:    "433",
			},
		},
		Composer: rm.PartySelf{},
		Language: rm.CodePhrase{
			TerminologyID: rm.TerminologyID{Value: "ISO_639-1"},
			CodeString:    "en",
		},
		Territory: rm.CodePhrase{
			TerminologyID: rm.TerminologyID{Value: "ISO_3166-1"},
			CodeString:    "NL",
		},
		Context: &rm.EventContext{
			StartTime: rm.DVDateTime{Value: "2026-10-07T10:00:00Z"},
			Setting: rm.DVCodedText{
				DVText: rm.DVText{Value: "other care"},
				DefiningCode: rm.CodePhrase{
					TerminologyID: rm.TerminologyID{Value: "openehr"},
					CodeString:    "238",
				},
			},
			Participations: parts,
		},
	}
}

// validActor returns a PERSON that satisfies actorOPT with a DV_TEXT
// languages child.
func validActor() *rm.Person {
	return &rm.Person{
		ArchetypeNodeID: "openEHR-DEMOGRAPHIC-PERSON.person.v1",
		Name:            rm.DVText{Value: "Person"},
		Identities: []rm.PartyIdentity{{
			ArchetypeNodeID: "at0001",
			Name:            rm.DVText{Value: "Identity"},
			Details: &rm.ItemTree{
				ArchetypeNodeID: "at0002",
				Name:            rm.DVText{Value: "Details"},
			},
		}},
		Languages: []rm.DVTextLike{rm.DVText{Value: "en"}},
		Roles: []rm.PartyRef{{
			ID:        rm.HierObjectID{Value: "a3e8b1c2-0d4f-4e5a-9b6c-7d8e9f0a1b2c"},
			Namespace: "demographic",
			Type:      "ROLE",
		}},
	}
}

// issueKeys renders each issue as "code path", sorted, so a test can
// compare the whole set and print it on failure.
func issueKeys(issues []validation.Issue) []string {
	keys := make([]string, 0, len(issues))
	for _, iss := range issues {
		keys = append(keys, iss.Code+" "+iss.Path)
	}
	slices.Sort(keys)
	return keys
}

// REQ-102 — a PARTICIPATION under EVENT_CONTEXT.participations binds to
// the PARTICIPATION child by RM type: a valid one passes, one without
// its function is walked and gets `required` at …/function, and a
// second one exceeds the child's occurrences 0..1.
func TestValidateComposition_REQ102_ParticipationsBindByRMType(t *testing.T) {
	c := mustCompileSyntheticOPT(t, participationsOPT)
	noFunction := validParticipation()
	noFunction.Function = nil

	tests := []struct {
		name  string
		parts []rm.Participation
		want  []string
	}{
		{
			name:  "valid participation binds",
			parts: []rm.Participation{validParticipation()},
			want:  []string{},
		},
		{
			name:  "bound participation is walked",
			parts: []rm.Participation{noFunction},
			want:  []string{"required /context/participations[@1]/function"},
		},
		{
			name:  "bound participations count toward occurrences",
			parts: []rm.Participation{validParticipation(), validParticipation()},
			want:  []string{"cardinality /context/participations[@1]"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := validation.ValidateComposition(participationsComposition(tc.parts...), c)
			if got := issueKeys(r.Issues); !slices.Equal(got, tc.want) {
				t.Errorf("ValidateComposition issues = %q, want %q", got, tc.want)
			}
		})
	}
}

// REQ-102 — ACTOR.languages and ACTOR.roles items are not LOCATABLE: a
// matching DV_TEXT or PARTY_REF binds by RM type to the first non-slot
// child that admits it, and one no child admits is an rm_type_mismatch
// (one child) or an alternative_mismatch (two or more) at the item's
// own path, never a slot_fill. A LOCATABLE
// that matches no child still gets slot_fill. A typed-nil item has no
// value to bind, so no child takes it, an untyped one included.
func TestValidateDemographic_REQ102_ActorListsBindByRMType(t *testing.T) {
	dvText := languagesChild("DV_TEXT")
	dvCodedText := languagesChild("DV_CODED_TEXT")
	untyped := languagesChild("")
	dvTextSlot := `
      <children xsi:type="ARCHETYPE_SLOT">
        <rm_type_name>DV_TEXT</rm_type_name>
        <node_id>at9000</node_id>
      </children>`

	tests := []struct {
		name      string
		languages string
		mutate    func(p *rm.Person)
		want      []string
	}{
		{
			name:      "matching language and role bind",
			languages: dvText,
			want:      []string{},
		},
		{
			name:      "bound role is walked",
			languages: dvText,
			mutate:    func(p *rm.Person) { p.Roles[0].Namespace = "" },
			want:      []string{"required /roles[@1]/namespace"},
		},
		{
			name:      "one child that does not admit the item",
			languages: dvCodedText,
			want:      []string{"rm_type_mismatch /languages[@1]"},
		},
		{
			name:      "two children that do not admit the item",
			languages: dvCodedText + dvCodedText,
			want:      []string{"alternative_mismatch /languages[@1]"},
		},
		{
			name:      "a slot child is skipped for the next non-slot child",
			languages: dvTextSlot + dvText,
			mutate:    func(p *rm.Person) { p.Languages = []rm.DVTextLike{rm.DVText{}} },
			want:      []string{"required /languages[@1]/value"},
		},
		{
			name:      "typed-nil item is not bound",
			languages: dvText,
			mutate:    func(p *rm.Person) { p.Languages = []rm.DVTextLike{(*rm.DVText)(nil)} },
			want:      []string{"rm_type_mismatch /languages[@1]"},
		},
		{
			name:      "typed-nil item is not bound to an untyped child",
			languages: untyped,
			mutate:    func(p *rm.Person) { p.Languages = []rm.DVTextLike{(*rm.DVText)(nil)} },
			want:      []string{"rm_type_mismatch /languages[@1]"},
		},
		{
			name:      "LOCATABLE with an unknown node id is a slot_fill",
			languages: dvText,
			mutate:    func(p *rm.Person) { p.Identities[0].ArchetypeNodeID = "at9999" },
			want:      []string{"slot_fill /identities[at9999]"},
		},
		{
			name:      "LOCATABLE without a node id is a slot_fill",
			languages: dvText,
			mutate:    func(p *rm.Person) { p.Identities[0].ArchetypeNodeID = "" },
			want:      []string{"slot_fill /identities[@1]"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := mustCompileInline(t, actorOPT(tc.languages))
			p := validActor()
			if tc.mutate != nil {
				tc.mutate(p)
			}
			r := validation.ValidateDemographic(p, c)
			if got := issueKeys(r.Issues); !slices.Equal(got, tc.want) {
				t.Errorf("ValidateDemographic issues = %q, want %q", got, tc.want)
			}
		})
	}
}

// REQ-102 — a nil item has no type that says it is not a LOCATABLE, so
// it keeps the node-id path: a nil under COMPOSITION.content is still a
// slot_fill at its own path, not a type mismatch.
func TestValidateComposition_REQ102_NilContentItemKeepsSlotFill(t *testing.T) {
	c := mustCompile(t, "vital_signs")
	comp := validVitalSignsComposition()
	comp.Content = []rm.ContentItem{nil}

	r := validation.ValidateComposition(comp, c)
	want := []string{"slot_fill /content[@1]"}
	if got := issueKeys(r.Issues); !slices.Equal(got, want) {
		t.Errorf("ValidateComposition issues = %q, want %q", got, want)
	}
}
