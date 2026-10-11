package validation_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// constraintRefCodePath is the issue path of the defining_code that a
// CONSTRAINT_REF constrains in the Demonstration.v1 fixture: element at0011
// ("Text That is Sourced From an External Terminology") of the first event.
const constraintRefCodePath = "/content[openEHR-EHR-OBSERVATION.demo.v1]/data/events[at0002]/data/items[at0004]/items[at0011]/value/defining_code"

// constraintRefCode decodes a fresh Demonstration.v1 composition and returns
// it with the defining_code its CONSTRAINT_REF leaf binds to.
func constraintRefCode(t *testing.T) (*rm.Composition, *rm.CodePhrase) {
	t.Helper()
	var comp rm.Composition
	mustDecodeJSON(t, fixtures.CompositionJSON("Demonstration.v1"), &comp)
	obs := comp.Content[0].(*rm.Observation)
	event := obs.Data.Events[0].(*rm.PointEvent[rm.ItemStructure])
	cluster := event.Data.(*rm.ItemTree).Items[1].(*rm.Cluster)
	element := cluster.Items[2].(*rm.Element)
	if element.ArchetypeNodeID != "at0011" {
		t.Fatalf("element archetype_node_id = %q, want at0011 (fixture drift)", element.ArchetypeNodeID)
	}
	coded, ok := element.Value.(*rm.DVCodedText)
	if !ok {
		t.Fatalf("at0011 value = %T, want *rm.DVCodedText (fixture drift)", element.Value)
	}
	return &comp, &coded.DefiningCode
}

// REQ-102 / REQ-103: a CONSTRAINT_REF leaf names an external value set, so
// its ConstraintRef constrains nothing the walker can check locally. The
// template walk still checks the RM-mandatory attributes of the CODE_PHRASE
// it binds there, as at a node the OPT leaves unconstrained: a blank
// terminology_id or code_string is `required` at that attribute's path.
// Template validation includes the RM floor (REQ-102 RM floor), which
// reports the same `required` there, carried once, and a blank code_string
// also as its own CODE_PHRASE rule at the code.
func TestREQ102_REQ103_ConstraintRefLeafChecksCodePhraseAttributes(t *testing.T) {
	t.Parallel()
	c := mustCompile(t, "Demonstration.v1")
	tests := []struct {
		name  string
		blank func(*rm.CodePhrase)
		want  []string // the issues under the code, as issueKeys writes them
	}{
		{name: "fixture as written", blank: func(*rm.CodePhrase) {}, want: []string{}},
		{
			name:  "blank terminology_id value",
			blank: func(cp *rm.CodePhrase) { cp.TerminologyID.Value = "" },
			want:  []string{"required " + constraintRefCodePath + "/terminology_id"},
		},
		{
			name:  "blank code_string",
			blank: func(cp *rm.CodePhrase) { cp.CodeString = "" },
			want: []string{
				"required " + constraintRefCodePath + "/code_string",
				"rm_invariant " + constraintRefCodePath,
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			comp, code := constraintRefCode(t)
			tc.blank(code)
			r := validation.ValidateComposition(comp, c)
			var under []validation.Issue
			for _, is := range r.Issues {
				if strings.HasPrefix(is.Path, constraintRefCodePath) {
					under = append(under, is)
				}
			}
			if got := issueKeys(under); !slices.Equal(got, tc.want) {
				t.Errorf("ValidateComposition(%s) issues under %s = %q, want %q", tc.name, constraintRefCodePath, got, tc.want)
			}
		})
	}
}
