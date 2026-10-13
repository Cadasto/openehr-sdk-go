package validation

// rmfloor_chained_internal_test.go: REQ-102 § RM floor, ADR 0025 — where the
// template walk and the floor report the same code at the same path, the
// Result carries that issue once; nothing else is merged, and the template
// walk's issues come first, in walk order.

import (
	"encoding/json"
	"maps"
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

// REQ-102 — the dedupe rule on its own: a floor issue is dropped only when a
// template issue has its code at its path, exactly as written.
func TestWithoutRepeats_REQ102_Rule(t *testing.T) {
	const p, q = "/content[at0001]/name", "/content[at0002]/name"
	req := func(path string) Issue { return Issue{Code: "required", Path: path} }
	card := func(path string) Issue { return Issue{Code: "cardinality", Path: path} }
	coded := func(path string) Issue { return Issue{Code: "code_not_in_value_set", Path: path} }
	tests := []struct {
		name            string
		template, floor []Issue
		want            []string
	}{
		{
			name:     "a floor issue with a template issue's code and path is dropped",
			template: []Issue{req(p)},
			floor:    []Issue{req(p)},
			want:     []string{"required " + p},
		},
		{
			name:     "the same path under another code stays",
			template: []Issue{req(p)},
			floor:    []Issue{coded(p)},
			want:     []string{"required " + p, "code_not_in_value_set " + p},
		},
		{
			name:     "the same code at another path stays",
			template: []Issue{req(p)},
			floor:    []Issue{req(q)},
			want:     []string{"required " + p, "required " + q},
		},
		{
			name:     "predicates are compared as written, so siblings stay apart",
			template: []Issue{req("/items[1]/name")},
			floor:    []Issue{req("/items[2]/name"), req("/items/name")},
			want:     []string{"required /items[1]/name", "required /items[2]/name", "required /items/name"},
		},
		{
			name:     "two template issues with one code at one path both stay",
			template: []Issue{card(p), card(p)},
			floor:    []Issue{card(p)},
			want:     []string{"cardinality " + p, "cardinality " + p},
		},
		{
			name:     "two floor issues with one code at one path both stay",
			template: []Issue{},
			floor:    []Issue{req(p), req(p)},
			want:     []string{"required " + p, "required " + p},
		},
		{
			name:     "template issues first, then the floor's survivors, each in order",
			template: []Issue{card(q), req(p)},
			floor:    []Issue{coded(p), req(p), req(q)},
			want:     []string{"cardinality " + q, "required " + p, "code_not_in_value_set " + p, "required " + q},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := keys(withoutRepeats(slices.Clone(tc.template), tc.floor))
			if !slices.Equal(got, tc.want) {
				t.Errorf("withoutRepeats(%q, %q) = %q, want %q", keys(tc.template), keys(tc.floor), got, tc.want)
			}
		})
	}
}

// REQ-102, REQ-112, REQ-116 — through Validate: the template walk and the
// floor each report the empty language of both reused OBSERVATIONs as
// `required` at one name-predicated path, and the Result carries each once.
// The floor's own code at that path stays. The Result is the template walk's
// issues, in walk order, then the floor's, in walk order, less the repeats.
func TestValidate_REQ102_REQ112_EachDefectOnce(t *testing.T) {
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

	w := newWalker(c)
	w.walkNode(c.Root(), comp, "/")
	floor, _ := walkFloor(comp, "COMPOSITION", w.names, Limits{})
	got := Validate(comp, c).Issues

	for _, name := range []string{"Erste", "Zweite"} {
		path := "/content[openEHR-EHR-OBSERVATION.reused.v1, '" + name + "']/language"
		for _, layer := range []struct {
			name   string
			issues []Issue
		}{{"template walk", w.issues}, {"floor", floor}} {
			if countAt(layer.issues, "required", path) != 1 {
				t.Fatalf("%s: want one required at %s, so the defect is one both layers see; got %q", layer.name, path, keys(layer.issues))
			}
		}
		if n := countAt(got, "required", path); n != 1 {
			t.Errorf("Validate carries required at %s %d times, want once; issues %q", path, n, keys(got))
		}
		if n := countAt(got, "code_not_in_value_set", path); n != 1 {
			t.Errorf("Validate carries the floor's code_not_in_value_set at %s %d times, want once; issues %q", path, n, keys(got))
		}
	}

	seen := map[string]bool{}
	for _, k := range keys(w.issues) {
		seen[k] = true
	}
	want := keys(w.issues)
	for _, k := range keys(floor) {
		if !seen[k] {
			want = append(want, k)
		}
	}
	if !slices.Equal(keys(got), want) {
		t.Errorf("Validate issues = %q, want the template walk's then the floor's survivors, %q", keys(got), want)
	}
}

// REQ-102, REQ-116 — the template walk collects, over the whole walk, the
// names it binds to items that share an id, keyed as the floor reads them.
func TestWalker_REQ102_REQ116_CollectsBoundNames(t *testing.T) {
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
	w := newWalker(c)
	w.walkNode(c.Root(), comp, "/")
	want := pinnedNames{
		{attrPath: "/content", index: 0}: "Zweite",
		{attrPath: "/content", index: 1}: "Erste",
	}
	if !maps.Equal(w.names, want) {
		t.Errorf("walker names = %v, want %v", w.names, want)
	}
}

// REQ-102 — a CONSTRAINT_REF leaf constrains nothing the walk can check, so
// the template walk itself, with no floor, still walks the attributes of the
// CODE_PHRASE it binds there and reports a blank terminology_id and
// code_string as `required`. Inside Validate the floor reports the same
// issues and the repeats are dropped, so only the walk alone shows which
// walk found them.
func TestWalker_REQ102_ConstraintRefLeafWalksCodePhrase(t *testing.T) {
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
			w := newWalker(c)
			w.walkNode(c.Root(), comp, "/")
			if countAt(w.issues, "required", tc.want) != 1 {
				t.Errorf("template walk alone: no required at %s; issues %q", tc.want, keys(w.issues))
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
