package simplified

// REQ-053 — decode places the members of one container list in the Web
// Template's node order, and takes a folded ITEM_STRUCTURE's concrete subtype
// from the compiled template only where the template gives one it can build.

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// childAt follows one list attribute to its member with the given
// archetype_node_id, or to the single-valued attribute when id is "".
func childAt(t *testing.T, node map[string]any, attr, id string) map[string]any {
	t.Helper()
	if id == "" {
		m, ok := node[attr].(map[string]any)
		if !ok {
			t.Fatalf("no %s object under %v", attr, node["archetype_node_id"])
		}
		return m
	}
	list, _ := node[attr].([]any)
	for _, e := range list {
		if m, ok := e.(map[string]any); ok && m["archetype_node_id"] == id {
			return m
		}
	}
	t.Fatalf("no %s[%s] under %v", attr, id, node["archetype_node_id"])
	return nil
}

// listIDs returns the archetype_node_id of each member of node[attr].
func listIDs(node map[string]any, attr string) []string {
	list, _ := node[attr].([]any)
	ids := make([]string, 0, len(list))
	for _, e := range list {
		m, _ := e.(map[string]any)
		id, _ := m["archetype_node_id"].(string)
		ids = append(ids, id)
	}
	return ids
}

// TestREQ053DecodePlacesMembersInTemplateOrder — REQ-053: FLAT orders only the
// instances of one node, so decode places distinct nodes in template order,
// whichever placement first reaches a node: the leaf loop, a composite leaf
// (the interval OBSERVATION and its interval ELEMENTs) or an underscore family
// (the ADMIN_ENTRY, reached by its `_uid` alone). The instances of one node keep
// their :index order. Every FLAT id below sorts out of template order.
func TestREQ053DecodePlacesMembersInTemplateOrder(t *testing.T) {
	wt, compiled := conformanceWT(t)
	body := rmattrBody(map[string]any{
		rmattrAction + "/dv_text":                           "action",
		rmattrSection + "/conformance_admin_entry/_uid":     "9fcc1c70-9349-444d-b9cb-8fa817697f5e",
		rmattrIntervalCount + "/lower":                      3,
		rmattrIntervalCount + "/upper":                      9,
		rmattrIntervalEvent + "/interval_dv_time/lower":     "2021-12-21T14:00:00Z",
		rmattrIntervalEvent + "/interval_dv_time/upper":     "2021-12-21T15:00:00Z",
		rmattrIntervalEvent + "/interval_dv_duration/lower": "PT1H",
		rmattrIntervalEvent + "/interval_dv_duration/upper": "PT2H",
		rmattrIntervalEvent + "/time":                       "2021-12-21T16:02:58Z",
		rmattrIntervalObs + "/language|code":                "en",
		rmattrIntervalObs + "/language|terminology":         "ISO_639-1",
		rmattrEvent + "/dv_count":                           7,
		rmattrEvent + "/dv_text:0":                          "first",
		rmattrEvent + "/dv_text:1":                          "second",
		rmattrEvent + "/dv_text:2":                          "third",
	})
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	for name, opts := range map[string][]Option{
		"without a template": nil,
		"with a template":    {WithTemplate(compiled)},
	} {
		t.Run(name, func(t *testing.T) {
			comp, err := UnmarshalFlat(b, wt, opts...)
			if err != nil {
				t.Fatalf("UnmarshalFlat: %v", err)
			}
			section := childAt(t, canonicalTree(t, comp), "content", "openEHR-EHR-SECTION.conformance_section.v0")
			wantItems := []string{
				"openEHR-EHR-ACTION.conformance_action_.v0",
				"openEHR-EHR-ADMIN_ENTRY.conformance_admin_entry.v0",
				"openEHR-EHR-OBSERVATION.conformance_interval.v0",
				"openEHR-EHR-OBSERVATION.conformance_observation.v0",
			}
			if got := listIDs(section, "items"); !slices.Equal(got, wantItems) {
				t.Errorf("SECTION items = %v, want template order %v", got, wantItems)
			}

			interval := childAt(t, section, "items", "openEHR-EHR-OBSERVATION.conformance_interval.v0")
			ivData := childAt(t, childAt(t, childAt(t, interval, "data", ""), "events", "at0002"), "data", "")
			if got, want := listIDs(ivData, "items"), []string{"at0006", "at0008", "at0009"}; !slices.Equal(got, want) {
				t.Errorf("interval event items = %v, want template order %v", got, want)
			}

			obs := childAt(t, section, "items", "openEHR-EHR-OBSERVATION.conformance_observation.v0")
			obsData := childAt(t, childAt(t, childAt(t, obs, "data", ""), "events", "at0002"), "data", "")
			if got, want := listIDs(obsData, "items"), []string{"at0004", "at0004", "at0004", "at0008", "at0010"}; !slices.Equal(got, want) {
				t.Errorf("observation event items = %v, want template order %v", got, want)
			}
			var texts []string
			for _, e := range obsData["items"].([]any) {
				m := e.(map[string]any)
				if m["archetype_node_id"] == "at0004" {
					v, _ := m["value"].(map[string]any)
					s, _ := v["value"].(string)
					texts = append(texts, s)
				}
			}
			if want := []string{"first", "second", "third"}; !slices.Equal(texts, want) {
				t.Errorf("dv_text instances = %v, want their :index order %v", texts, want)
			}
		})
	}
}

// TestREQ053DecodeOrderKeepsUnmodelledMembers — REQ-053: a list member no Web
// Template node accounts for keeps its place after the modelled ones, in the
// order it was placed, so a list with no archetyped member is left as it is.
func TestREQ053DecodeOrderKeepsUnmodelledMembers(t *testing.T) {
	wt, _ := conformanceWT(t)
	tree := map[string]any{
		"_type": "COMPOSITION",
		"content": []any{
			map[string]any{"_type": "SECTION", "archetype_node_id": "openEHR-EHR-SECTION.unmodelled.v0"},
			map[string]any{"_type": "SECTION", "archetype_node_id": "openEHR-EHR-SECTION.conformance_section.v0"},
		},
		"links": []any{
			map[string]any{"_type": "LINK", "type": map[string]any{"value": "b"}},
			map[string]any{"_type": "LINK", "type": map[string]any{"value": "a"}},
		},
	}
	orderByTemplate(tree, wt)
	if got, want := listIDs(tree, "content"), []string{"openEHR-EHR-SECTION.conformance_section.v0", "openEHR-EHR-SECTION.unmodelled.v0"}; !slices.Equal(got, want) {
		t.Errorf("content = %v, want the modelled member first %v", got, want)
	}
	var links []string
	for _, e := range tree["links"].([]any) {
		links = append(links, e.(map[string]any)["type"].(map[string]any)["value"].(string))
	}
	if want := []string{"b", "a"}; !slices.Equal(links, want) {
		t.Errorf("links = %v, want the placed order %v", links, want)
	}
}

// TestREQ053ConcreteTypeTakesTheTemplateStructure — REQ-053: a folded
// ITEM_STRUCTURE takes the type the compiled template gives it where that type
// is an ITEM_STRUCTURE declaring the attribute the path goes on through; with no
// such answer the path decides, as it does without a template. Other abstract
// slots do not consult the template.
func TestREQ053ConcreteTypeTakesTheTemplateStructure(t *testing.T) {
	tests := []struct {
		name                               string
		parent, attr, wtType, tmplType, nx string
		want                               string
	}{
		{name: "template names ITEM_LIST", parent: "OBSERVATION", attr: "protocol", tmplType: "ITEM_LIST", nx: "items", want: "ITEM_LIST"},
		{name: "template names ITEM_LIST under an EVENT", parent: "POINT_EVENT", attr: "data", tmplType: "ITEM_LIST", nx: "items", want: "ITEM_LIST"},
		{name: "template names ITEM_TREE", parent: "ADMIN_ENTRY", attr: "data", tmplType: "ITEM_TREE", nx: "items", want: "ITEM_TREE"},
		{name: "no answer: items", parent: "OBSERVATION", attr: "protocol", nx: "items", want: "ITEM_TREE"},
		{name: "no answer: rows", parent: "OBSERVATION", attr: "protocol", nx: "rows", want: "ITEM_TABLE"},
		{name: "no answer: item", parent: "OBSERVATION", attr: "protocol", nx: "item", want: "ITEM_SINGLE"},
		{name: "template type lacks the next attribute", parent: "OBSERVATION", attr: "protocol", tmplType: "ITEM_SINGLE", nx: "items", want: "ITEM_TREE"},
		{name: "template type is no ITEM_STRUCTURE", parent: "OBSERVATION", attr: "protocol", tmplType: "CLUSTER", nx: "items", want: "ITEM_TREE"},
		{name: "content slot ignores the template", parent: "COMPOSITION", attr: "content", wtType: "OBSERVATION", tmplType: "EVALUATION", nx: "data", want: "OBSERVATION"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := concreteType(tc.parent, tc.attr, tc.wtType, tc.tmplType, tc.nx); got != tc.want {
				t.Errorf("concreteType(%q, %q, %q, %q, %q) = %q, want %q", tc.parent, tc.attr, tc.wtType, tc.tmplType, tc.nx, got, tc.want)
			}
		})
	}
}

// TestREQ053TemplateIndexDisagreementGivesNoType — REQ-053: nodes that share a
// path but disagree on their RM type, such as the alternatives of one
// single-valued attribute, leave the path with no type, so the walk infers it;
// nodes that agree keep it. A nil index is the OPT-free decode and answers
// nothing.
func TestREQ053TemplateIndexDisagreementGivesNoType(t *testing.T) {
	x := &templateIndex{names: map[string]string{}, rmTypes: map[string]string{}}
	x.addRMType("/protocol", "ITEM_LIST")
	x.addRMType("/protocol", "ITEM_TREE")
	x.addRMType("/protocol", "ITEM_LIST")
	if got := x.rmType("/protocol"); got != "" {
		t.Errorf("rmType after ITEM_LIST, ITEM_TREE, ITEM_LIST = %q, want no answer", got)
	}
	x.addRMType("/data", "ITEM_LIST")
	x.addRMType("/data", "ITEM_LIST")
	if got := x.rmType("/data"); got != "ITEM_LIST" {
		t.Errorf("rmType after two ITEM_LIST nodes = %q, want ITEM_LIST", got)
	}
	var none *templateIndex
	if got, name := none.rmType("/data"), none.name("/data"); got != "" || name != "" {
		t.Errorf("nil index answers rmType %q, name %q, want nothing", got, name)
	}
}

// TestREQ053TemplateIndexTypesTheBMIProtocol — REQ-053: the index the BMI
// template builds gives its folded protocol the ITEM_LIST the OPT names, at the
// path the walk rebuilds.
func TestREQ053TemplateIndexTypesTheBMIProtocol(t *testing.T) {
	opt, err := template.ParseFile(fixtures.TemplateOpt("BMI"))
	if err != nil {
		t.Fatalf("ParseFile(BMI): %v", err)
	}
	compiled, err := templatecompile.Compile(opt)
	if err != nil {
		t.Fatalf("Compile(BMI): %v", err)
	}
	const protocol = "/content[openEHR-EHR-OBSERVATION.body_mass_index.v1]/protocol"
	if got := buildTemplateIndex(compiled).rmType(protocol); got != "ITEM_LIST" {
		t.Errorf("rmType(%q) = %q, want ITEM_LIST", protocol, got)
	}
}
