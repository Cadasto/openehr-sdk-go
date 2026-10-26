package simplified

// REQ-053, REQ-116 — the template index gives a node a coded name only where
// the template determines one for every node at the node's path: alternatives
// and reused siblings that disagree leave the path with none, and the node
// takes its DV_TEXT name.

import (
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// codedValuesTemplate is the corpus template whose ELEMENT at0016 has a coded
// name: its text pinned to "Second option", its code one of the local at0017
// ("First option") and at0018 ("Second option").
const codedValuesTemplate = "sdk_crosscheck_values.v1"

// elementEnd matches the end of the ELEMENT at0016, the last member of its
// items list.
var elementEnd = regexp.MustCompile(`</children>\s*<cardinality>`)

// compileValues compiles the OPT text of codedValuesTemplate after edit.
func compileValues(t *testing.T, edit func(opt string) string) *templatecompile.Compiled {
	t.Helper()
	raw, err := os.ReadFile(fixtures.TemplateOpt(codedValuesTemplate))
	if err != nil {
		t.Fatalf("read %s: %v", codedValuesTemplate, err)
	}
	opt, err := template.ParseOPT(strings.NewReader(edit(string(raw))))
	if err != nil {
		t.Fatalf("ParseOPT(edited %s): %v", codedValuesTemplate, err)
	}
	compiled, err := templatecompile.Compile(opt)
	if err != nil {
		t.Fatalf("Compile(edited %s): %v", codedValuesTemplate, err)
	}
	return compiled
}

// withTwin returns an edit that adds, after the ELEMENT at0016, a reused
// sibling of it: a copy whose pinned name text is twinName.
func withTwin(t *testing.T, twinName string) func(opt string) string {
	return func(opt string) string {
		t.Helper()
		before, _, found := strings.Cut(opt, "<node_id>at0016</node_id>")
		start := strings.LastIndex(before, `<children xsi:type="C_COMPLEX_OBJECT">`)
		if !found || start < 0 {
			t.Fatalf("%s has no ELEMENT at0016 — fixture changed?", codedValuesTemplate)
		}
		loc := elementEnd.FindStringIndex(opt[start:])
		if loc == nil {
			t.Fatalf("cannot find the end of the ELEMENT at0016 in %s — fixture changed?", codedValuesTemplate)
		}
		end := start + loc[0] + len("</children>")
		element := opt[start:end]
		twin := strings.Replace(element, "<list>Second option</list>", "<list>"+twinName+"</list>", 1)
		if twin == element {
			t.Fatalf("the ELEMENT at0016 pins no name \"Second option\" — fixture changed?")
		}
		return opt[:end] + twin + opt[end:]
	}
}

// TestREQ053TemplateIndexCodedNameDisagreement — REQ-053: nodes that share a
// path but disagree on their coded name, one of them giving none included,
// leave the path with no coded name; nodes that agree keep it. A nil index is
// the OPT-free decode and gives no name.
func TestREQ053TemplateIndexCodedNameDisagreement(t *testing.T) {
	first := codedName{value: "First option", code: "at0017", terminology: "local"}
	second := codedName{value: "Second option", code: "at0018", terminology: "local"}
	cases := []struct {
		name string
		adds []codedName
		want codedName
	}{
		{name: "one node", adds: []codedName{first}, want: first},
		{name: "two nodes that agree", adds: []codedName{first, first}, want: first},
		{name: "two that disagree", adds: []codedName{first, second}, want: codedName{}},
		{name: "a node with none, then one with a coded name", adds: []codedName{{}, first}, want: codedName{}},
		{name: "a coded name, then a node with none", adds: []codedName{first, {}}, want: codedName{}},
		{name: "a disagreement that a third node repeats", adds: []codedName{first, second, first}, want: codedName{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			x := &templateIndex{names: map[string]string{"/n": "Coded name"}, codedNames: map[string]codedName{}}
			for _, c := range tc.adds {
				x.addCodedName("/n", c)
			}
			if got := x.codedNames["/n"]; got != tc.want {
				t.Errorf("codedNames after %v = %v, want %v", tc.adds, got, tc.want)
			}
			want := textJSON("Coded name")
			if tc.want != (codedName{}) {
				want = tc.want.json()
			}
			if got := x.nameJSON("/n"); !reflect.DeepEqual(got, want) {
				t.Errorf("nameJSON after %v = %v, want %v", tc.adds, got, want)
			}
		})
	}
	var none *templateIndex
	if got := none.nameJSON("/n"); got != nil {
		t.Errorf("nil index nameJSON = %v, want nil", got)
	}
}

// TestREQ053TemplateIndexCodedNameOfReusedSiblings — REQ-053, REQ-116: two
// reused siblings of the ELEMENT at0016 share its bare path but are told apart
// by their pinned names. Each keeps its own coded name under its compiled
// path, but the bare spelling decode composes from FLAT cannot say which
// sibling is meant, so where they disagree on the coded name, a sibling that
// gives none included, the bare path has none and decode gives the DV_TEXT
// rubric.
func TestREQ053TemplateIndexCodedNameOfReusedSiblings(t *testing.T) {
	original := codedName{value: "Second option", code: "at0018", terminology: "local"}
	cases := []struct {
		name     string
		twinName string
		twin     codedName
	}{
		{
			name: "a twin with another coded name", twinName: "First option",
			twin: codedName{value: "First option", code: "at0017", terminology: "local"},
		},
		{name: "a twin whose pinned text no code has", twinName: "Third option"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			compiled := compileValues(t, withTwin(t, tc.twinName))
			x := buildTemplateIndex(compiled)
			nodes := compiled.AllByNodeID("at0016")
			if len(nodes) != 2 {
				t.Fatalf("edited %s has %d nodes at0016, want 2", codedValuesTemplate, len(nodes))
			}
			bare := bareAQLPath(nodes[0].AQLPath())
			for _, n := range nodes {
				if n.AQLPath() == bare {
					t.Fatalf("sibling path %s carries no name predicate — the case tests nothing", n.AQLPath())
				}
				want := original
				if n.NodeName() == tc.twinName {
					want = tc.twin
				}
				if got := x.codedNames[n.AQLPath()]; got != want {
					t.Errorf("codedNames[%s] = %v, want %v", n.AQLPath(), got, want)
				}
			}
			if got := x.codedNames[bare]; got != (codedName{}) {
				t.Errorf("codedNames[%s] = %v, want none: the bare path cannot say which sibling is meant", bare, got)
			}
			if got, want := x.nameJSON(bare), textJSON("Coded name"); !reflect.DeepEqual(got, want) {
				t.Errorf("nameJSON(%s) = %v, want %v", bare, got, want)
			}
		})
	}
}

// TestREQ053CodedNameNeedsACodedTextConstraint — REQ-053: a name the template
// constrains as a DV_TEXT gets no coded name, even where the OPT puts a
// defining_code on it, which the RM does not give a DV_TEXT. The unedited
// DV_CODED_TEXT constraint is the control.
func TestREQ053CodedNameNeedsACodedTextConstraint(t *testing.T) {
	const coded = "<rm_type_name>DV_CODED_TEXT</rm_type_name>"
	cases := []struct {
		name string
		edit func(opt string) string
		want codedName
	}{
		{
			name: "a DV_CODED_TEXT", edit: func(opt string) string { return opt },
			want: codedName{value: "Second option", code: "at0018", terminology: "local"},
		},
		{name: "a DV_TEXT with a defining_code", edit: func(opt string) string {
			// The ELEMENT at0015's value comes first, then at0016's name.
			i := strings.Index(opt, "<rm_attribute_name>name</rm_attribute_name>")
			if i < 0 || !strings.Contains(opt[i:], coded) {
				t.Fatalf("%s has no coded name — fixture changed?", codedValuesTemplate)
			}
			return opt[:i] + strings.Replace(opt[i:], coded, "<rm_type_name>DV_TEXT</rm_type_name>", 1)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			compiled := compileValues(t, tc.edit)
			nodes := compiled.AllByNodeID("at0016")
			if len(nodes) != 1 {
				t.Fatalf("%s has %d nodes at0016, want 1", codedValuesTemplate, len(nodes))
			}
			if got := codedNameOf(nodes[0]); got != tc.want {
				t.Errorf("codedNameOf(at0016) = %v, want %v", got, tc.want)
			}
		})
	}
}
