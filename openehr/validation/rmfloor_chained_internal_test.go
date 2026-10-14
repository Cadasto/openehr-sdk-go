package validation

// rmfloor_chained_internal_test.go: REQ-102 § RM floor, REQ-171 § One walk —
// a template-driven validation applies the RM rules in the walk that binds
// the template, so each defect is one finding, at the path that walk writes,
// in the order of its steps.

import (
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// keys writes each issue as "code path", in the order given.
func keys(issues []Issue) []string {
	out := make([]string, len(issues))
	for i, iss := range issues {
		out[i] = iss.Code + " " + iss.Path
	}
	return out
}

// REQ-102 § RM floor, § Issue paths; REQ-112; REQ-116; REQ-171 § One walk —
// through Validate, the empty language of two OBSERVATIONs that reuse one
// archetype under the names Erste and Zweite is reported once at each, as
// the RM's `required`, beside the coded rule's own code: at the
// name-predicated path the walk that binds the template writes, since the RM
// rules run in that walk. The findings come in the walk's step order: at
// each node the RM rules of its class, then its attributes, those the
// template declares first.
func TestValidate_REQ102_REQ112_REQ116_REQ171_EachDefectOnceAtTheWalksPath(t *testing.T) {
	opt, err := template.ParseOPT(strings.NewReader(handoffOPT))
	if err != nil {
		t.Fatalf("ParseOPT: %v", err)
	}
	c, err := templatecompile.Compile(opt)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	comp := &rm.Composition{
		ArchetypeNodeID: "openEHR-EHR-COMPOSITION.reused.v1",
		Name:            rm.DVText{Value: "Reused"},
		Content:         []rm.ContentItem{handoffObservation("Zweite"), handoffObservation("Erste")},
	}
	got := Validate(comp, c).Issues

	const reused = "/content[openEHR-EHR-OBSERVATION.reused.v1, "
	var want []string
	want = append(want, "is_archetype_root /archetype_details", "code_not_in_value_set /category/defining_code", "code_not_in_value_set /language", "code_not_in_value_set /territory")
	for _, name := range []string{"Zweite", "Erste"} {
		at := reused + "'" + name + "']"
		want = append(want, "is_archetype_root "+at+"/archetype_details", "code_not_in_value_set "+at+"/language", "required "+at+"/language", "required "+at+"/data")
	}
	want = append(want, "required /language", "required /territory", "required /category", "required /composer")
	if !slices.Equal(keys(got), want) {
		t.Errorf("Validate issues = %q, want %q", keys(got), want)
	}
	for _, name := range []string{"Erste", "Zweite"} {
		path := reused + "'" + name + "']/language"
		if n := countAt(got, "required", path); n != 1 {
			t.Errorf("Validate carries required at %s %d times, want once; issues %q", path, n, keys(got))
		}
	}
}

// REQ-102 Primitive constraints — a CONSTRAINT_REF leaf constrains nothing
// the walk can check locally, so Validate still walks the attributes of the
// CODE_PHRASE it binds there and reports a blank terminology_id and
// code_string as `required`, once each, as at a node the template leaves
// unconstrained.
func TestValidate_REQ102_ConstraintRefLeafWalksCodePhrase(t *testing.T) {
	const code = "/content[openEHR-EHR-OBSERVATION.demo.v1]/data/events[at0002]/data/items[at0004]/items[at0011]/value/defining_code"
	opt, err := template.ParseFile(fixtures.TemplateOpt("Demonstration.v1"))
	if err != nil {
		t.Fatalf("parse Demonstration.v1: %v", err)
	}
	c, err := templatecompile.Compile(opt)
	if err != nil {
		t.Fatalf("compile Demonstration.v1: %v", err)
	}
	tests := []struct {
		name  string
		blank func(*rm.CodePhrase)
		want  string
	}{
		{name: "blank terminology_id", blank: func(cp *rm.CodePhrase) { cp.TerminologyID.Value = "" }, want: code + "/terminology_id"},
		{name: "blank code_string", blank: func(cp *rm.CodePhrase) { cp.CodeString = "" }, want: code + "/code_string"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			comp, cp := demonstrationConstraintRefCode(t)
			tc.blank(cp)
			issues := Validate(comp, c).Issues
			if countAt(issues, "required", tc.want) != 1 {
				t.Errorf("Validate: want one required at %s; issues %q", tc.want, keys(issues))
			}
		})
	}
}

// demonstrationConstraintRefCode decodes the Demonstration.v1 corpus
// composition and returns it with the defining_code its CONSTRAINT_REF leaf,
// ELEMENT at0011 of the first event, binds to.
func demonstrationConstraintRefCode(t *testing.T) (*rm.Composition, *rm.CodePhrase) {
	t.Helper()
	raw, err := os.ReadFile(fixtures.CompositionJSON("Demonstration.v1"))
	if err != nil {
		t.Fatalf("read Demonstration.v1: %v", err)
	}
	var comp rm.Composition
	if err := json.Unmarshal(raw, &comp); err != nil {
		t.Fatalf("decode Demonstration.v1: %v", err)
	}
	obs, ok := comp.Content[0].(*rm.Observation)
	if !ok {
		t.Fatalf("content[0] is %T, want *rm.Observation", comp.Content[0])
	}
	event, ok := obs.Data.Events[0].(*rm.PointEvent[rm.ItemStructure])
	if !ok {
		t.Fatalf("events[0] is %T, want *rm.PointEvent[rm.ItemStructure]", obs.Data.Events[0])
	}
	tree, ok := event.Data.(*rm.ItemTree)
	if !ok {
		t.Fatalf("event data is %T, want *rm.ItemTree", event.Data)
	}
	cluster, ok := tree.Items[1].(*rm.Cluster)
	if !ok {
		t.Fatalf("data items[1] is %T, want *rm.Cluster", tree.Items[1])
	}
	element, ok := cluster.Items[2].(*rm.Element)
	if !ok || element.ArchetypeNodeID != "at0011" {
		t.Fatalf("cluster items[2] is %T, want the ELEMENT at0011 (fixture drift)", cluster.Items[2])
	}
	coded, ok := element.Value.(*rm.DVCodedText)
	if !ok {
		t.Fatalf("at0011 value is %T, want *rm.DVCodedText", element.Value)
	}
	return &comp, &coded.DefiningCode
}

// countAt counts the issues that carry code at path.
func countAt(issues []Issue, code, path string) int {
	n := 0
	for _, i := range issues {
		if i.Code == code && i.Path == path {
			n++
		}
	}
	return n
}
