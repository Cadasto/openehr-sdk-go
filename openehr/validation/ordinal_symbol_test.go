package validation_test

import (
	"cmp"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// ordinalValuePath is the issue path of the DV_ORDINAL that element at0015
// ("Ordinal") of the first event carries in the Demonstration.v1 fixture.
// Its C_DV_ORDINAL lists 0/at0038, 1/at0039, 2/at0040, 5/at0041, 9/at0042
// and 10/at0043, all in the local terminology.
const ordinalValuePath = "/content[openEHR-EHR-OBSERVATION.demo.v1]/data/events[at0002]/data/items[at0004]/items[at0015]/value"

// demonstrationOrdinal decodes a fresh Demonstration.v1 composition and
// returns it with the first event's at0015 element and the DV_ORDINAL that
// element holds.
func demonstrationOrdinal(t *testing.T) (*rm.Composition, *rm.Element, *rm.DVOrdinal) {
	t.Helper()
	var comp rm.Composition
	mustDecodeJSON(t, fixtures.CompositionJSON("Demonstration.v1"), &comp)
	obs := comp.Content[0].(*rm.Observation)
	event := obs.Data.Events[0].(*rm.PointEvent[rm.ItemStructure])
	cluster := event.Data.(*rm.ItemTree).Items[1].(*rm.Cluster)
	element := cluster.Items[11].(*rm.Element)
	if element.ArchetypeNodeID != "at0015" {
		t.Fatalf("element archetype_node_id = %q, want at0015 (fixture drift)", element.ArchetypeNodeID)
	}
	ord, ok := element.Value.(*rm.DVOrdinal)
	if !ok {
		t.Fatalf("at0015 value = %T, want *rm.DVOrdinal (fixture drift)", element.Value)
	}
	if ord.Value != 1 || ord.Symbol.DefiningCode.CodeString != "at0039" || ord.Symbol.DefiningCode.TerminologyID.Value != "local" {
		t.Fatalf("at0015 ordinal = %d %s::%s, want 1 local::at0039 (fixture drift)",
			ord.Value, ord.Symbol.DefiningCode.TerminologyID.Value, ord.Symbol.DefiningCode.CodeString)
	}
	return &comp, element, ord
}

// ordinalIssue is the part of an Issue the ordinal rows pin. The rows list
// them sorted by code, then path.
type ordinalIssue struct {
	Code, Path string
	Layer      validation.Layer
	Rule       string
}

// REQ-102 / REQ-103: at a DV_ORDINAL bound to a C_DV_ORDINAL the template
// validator checks the value and the symbol's defining code together, so a
// symbol that does not match its value, or names another terminology, is
// reported as primitive_not_in_list on the template layer. A defining code
// with no code string is left to the RM rules, which report it, and draws no
// primitive_* issue. A defining code with no terminology_id is still checked
// by its code, which matches a listed pair under REQ-103's rule, while the
// RM rules report the missing terminology; so a swapped code there is
// reported beside it. Each row runs with the ordinal held by pointer, as
// decoded, and by value.
func TestValidateComposition_REQ102_REQ103_OrdinalSymbolPair(t *testing.T) {
	t.Parallel()
	c := mustCompile(t, "Demonstration.v1")
	notInList := ordinalIssue{
		Code:  "primitive_not_in_list",
		Path:  ordinalValuePath,
		Layer: validation.LayerTemplate,
		Rule:  "not_in_list",
	}
	tests := []struct {
		name   string
		change func(*rm.DVOrdinal)
		want   []ordinalIssue
	}{
		{
			name:   "listed pair 1 local::at0039",
			change: func(*rm.DVOrdinal) {},
			want:   []ordinalIssue{},
		},
		{
			name:   "listed pair 2 local::at0040",
			change: func(o *rm.DVOrdinal) { o.Value = 2; o.Symbol.DefiningCode.CodeString = "at0040" },
			want:   []ordinalIssue{},
		},
		{
			name:   "value 1 with the symbol code of value 2",
			change: func(o *rm.DVOrdinal) { o.Symbol.DefiningCode.CodeString = "at0040" },
			want:   []ordinalIssue{notInList},
		},
		{
			name:   "listed code in another terminology",
			change: func(o *rm.DVOrdinal) { o.Symbol.DefiningCode.TerminologyID.Value = "SNOMED-CT" },
			want:   []ordinalIssue{notInList},
		},
		{
			name:   "unlisted value",
			change: func(o *rm.DVOrdinal) { o.Value = 3 },
			want:   []ordinalIssue{notInList},
		},
		{
			name:   "defining code with no code_string",
			change: func(o *rm.DVOrdinal) { o.Symbol.DefiningCode.CodeString = "" },
			want: []ordinalIssue{
				{Code: "required", Path: ordinalValuePath + "/symbol/defining_code/code_string", Layer: validation.LayerRM, Rule: "mandatory"},
			},
		},
		{
			name:   "defining code with no terminology_id",
			change: func(o *rm.DVOrdinal) { o.Symbol.DefiningCode.TerminologyID.Value = "" },
			want: []ordinalIssue{
				{Code: "required", Path: ordinalValuePath + "/symbol/defining_code/terminology_id", Layer: validation.LayerRM, Rule: "mandatory"},
			},
		},
		{
			name: "no terminology_id and the symbol code of value 2",
			change: func(o *rm.DVOrdinal) {
				o.Symbol.DefiningCode.TerminologyID.Value = ""
				o.Symbol.DefiningCode.CodeString = "at0040"
			},
			want: []ordinalIssue{
				notInList,
				{Code: "required", Path: ordinalValuePath + "/symbol/defining_code/terminology_id", Layer: validation.LayerRM, Rule: "mandatory"},
			},
		},
	}
	for _, tc := range tests {
		for _, form := range []string{"pointer", "value"} {
			t.Run(tc.name+" by "+form, func(t *testing.T) {
				t.Parallel()
				comp, element, ord := demonstrationOrdinal(t)
				tc.change(ord)
				if form == "value" {
					element.Value = *ord
				}
				r := validation.ValidateComposition(comp, c)
				got := []ordinalIssue{}
				for _, is := range r.Issues {
					if is.Path == ordinalValuePath || strings.HasPrefix(is.Path, ordinalValuePath+"/") {
						got = append(got, ordinalIssue{Code: is.Code, Path: is.Path, Layer: is.Layer, Rule: is.Rule})
					}
				}
				slices.SortFunc(got, func(a, b ordinalIssue) int {
					return cmp.Or(cmp.Compare(a.Code, b.Code), cmp.Compare(a.Path, b.Path))
				})
				if !slices.Equal(got, tc.want) {
					t.Errorf("ValidateComposition(%s, %s) issues under %s =\n  %+v\nwant\n  %+v", tc.name, form, ordinalValuePath, got, tc.want)
				}
			})
		}
	}
}

// symbolTerminologyID matches the terminology_id of a C_DV_ORDINAL symbol's
// defining code in an OPT, with the whitespace after it. The first group is
// what comes before it inside the symbol, which the replacement keeps.
var symbolTerminologyID = regexp.MustCompile(`(<symbol>\s*<value\s*/>\s*<defining_code>\s*)<terminology_id>\s*<value>[^<]*</value>\s*</terminology_id>\s*`)

// demonstrationSymbols is the number of ordinal symbols the Demonstration.v1
// OPT lists: six pairs at each of its four C_DV_ORDINAL nodes.
const demonstrationSymbols = 24

// mustCompileUnnamedSymbolTerminology compiles Demonstration.v1 with the
// terminology_id of every ordinal symbol removed from the OPT, as an OPT may
// leave it out. The parser then lists each pair with an empty terminology.
func mustCompileUnnamedSymbolTerminology(t *testing.T) *templatecompile.Compiled {
	t.Helper()
	raw, err := os.ReadFile(fixtures.TemplateOptForName("Demonstration.v1"))
	if err != nil {
		t.Fatalf("read Demonstration.v1 OPT: %v", err)
	}
	if n := len(symbolTerminologyID.FindAllIndex(raw, -1)); n != demonstrationSymbols {
		t.Fatalf("Demonstration.v1 OPT has %d ordinal symbol terminology_id elements, want %d (fixture drift)", n, demonstrationSymbols)
	}
	return mustCompileInline(t, string(symbolTerminologyID.ReplaceAll(raw, []byte("$1"))))
}

// primitiveIssueKeys returns "code path" for each primitive_* issue, sorted.
func primitiveIssueKeys(issues []validation.Issue) []string {
	keys := []string{}
	for _, is := range issues {
		if strings.HasPrefix(is.Code, "primitive_") {
			keys = append(keys, is.Code+" "+is.Path)
		}
	}
	slices.Sort(keys)
	return keys
}

// dvOrdinalType matches the type of a DV_ORDINAL in canonical JSON.
var dvOrdinalType = regexp.MustCompile(`"_type"\s*:\s*"DV_ORDINAL"`)

// REQ-102 / REQ-103: an OPT whose ordinal symbols name no terminology lists
// each pair with an empty terminology, which admits the terminology the
// instance carries. Under such a template the vendored composition and the
// generator's Large output, which both write local there, draw no
// primitive_* issue, and a symbol that does not match its value is still
// primitive_not_in_list.
func TestValidateComposition_REQ102_REQ103_OrdinalSymbolUnnamedTerminology(t *testing.T) {
	t.Parallel()
	c := mustCompileUnnamedSymbolTerminology(t)

	t.Run("vendored composition", func(t *testing.T) {
		t.Parallel()
		comp, _, _ := demonstrationOrdinal(t)
		if got := primitiveIssueKeys(validation.ValidateComposition(comp, c).Issues); len(got) != 0 {
			t.Errorf("ValidateComposition(Demonstration.v1) primitive issues = %q, want none", got)
		}
	})

	t.Run("generated Large instance", func(t *testing.T) {
		t.Parallel()
		root, err := generateExample(t, c)
		if err != nil {
			t.Fatalf("Generate(Demonstration.v1, Large): %v", err)
		}
		raw, err := canjson.Marshal(root)
		if err != nil {
			t.Fatalf("canjson.Marshal(%T): %v", root, err)
		}
		if !dvOrdinalType.Match(raw) {
			t.Fatalf("Generate(Demonstration.v1, Large) built no DV_ORDINAL, so the check below is vacuous")
		}
		if got := primitiveIssueKeys(validateRoot(root, c).Issues); len(got) != 0 {
			t.Errorf("ValidateComposition(generated Demonstration.v1) primitive issues = %q, want none", got)
		}
	})

	t.Run("swapped pair", func(t *testing.T) {
		t.Parallel()
		comp, _, ord := demonstrationOrdinal(t)
		ord.Symbol.DefiningCode.CodeString = "at0040"
		want := []string{"primitive_not_in_list " + ordinalValuePath}
		if got := primitiveIssueKeys(validation.ValidateComposition(comp, c).Issues); !slices.Equal(got, want) {
			t.Errorf("ValidateComposition(value 1 with local::at0040) primitive issues = %q, want %q", got, want)
		}
	})
}
