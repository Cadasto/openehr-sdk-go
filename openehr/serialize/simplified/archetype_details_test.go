package simplified

// REQ-053 — decode rebuilds archetype_details from the Web Template on every
// archetype root, in both decode modes and through STRUCTURED. REQ-140 — the
// third out-of-scope class: encode writes nothing for it, so a round trip
// rebuilds it.

import (
	"encoding/json"
	"errors"
	"maps"
	"os"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/template/webtemplate"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// Archetype roots of the PROBE-086 corpus template, as the Web Template
// names them.
const (
	adComposition = "openEHR-EHR-COMPOSITION.conformance_composition_.v0"
	adSection     = "openEHR-EHR-SECTION.conformance_section.v0"
	adObservation = "openEHR-EHR-OBSERVATION.conformance_observation.v0"
	adEvaluation  = "openEHR-EHR-EVALUATION.conformance_evaluation.v0"
	adCluster     = "openEHR-EHR-CLUSTER.conformance_cluster.v0"
)

// archetypeDetailsBody is an SDK-authored FLAT body against the corpus
// template that reaches every kind of archetype root decode must rebuild: the
// COMPOSITION, a SECTION, two ENTRYs, and a CLUSTER filling a slot inside the
// OBSERVATION's event data. The corpus's own cluster body is refused on decode
// (its `labresult/text_value` path is not in this Web Template), so it cannot
// serve here.
func archetypeDetailsBody() map[string]any {
	evaluation := rmattrSection + "/conformance_evaluation"
	return rmattrBody(map[string]any{
		rmattrEvent + "/conformance_cluster/labresult": "labresult 4",
		evaluation + "/dv_text":                        "dv_text in data",
		evaluation + "/language|code":                  "en",
		evaluation + "/language|terminology":           "ISO_639-1",
	})
}

// locatedNode is one object of a canonical tree that carries an
// archetype_node_id, with the path it was found at.
type locatedNode struct {
	path string
	obj  map[string]any
}

// locatableNodes returns every object in a canonical-JSON tree that carries an
// archetype_node_id, keyed by that id (one id may occur at several paths).
func locatableNodes(tree map[string]any) map[string][]locatedNode {
	out := map[string][]locatedNode{}
	var walk func(path string, n any)
	walk = func(path string, n any) {
		switch x := n.(type) {
		case map[string]any:
			if id, ok := x["archetype_node_id"].(string); ok {
				out[id] = append(out[id], locatedNode{path: path, obj: x})
			}
			for k, v := range x {
				walk(path+"/"+k, v)
			}
		case []any:
			for i, v := range x {
				walk(path+"["+strconv.Itoa(i)+"]", v)
			}
		}
	}
	walk("", tree)
	return out
}

// canonicalTree renders a decoded composition as a generic canonical-JSON map.
func canonicalTree(t *testing.T, comp *rm.Composition) map[string]any {
	t.Helper()
	b, err := canjson.Marshal(comp)
	if err != nil {
		t.Fatalf("canjson.Marshal: %v", err)
	}
	var tree map[string]any
	if err := json.Unmarshal(b, &tree); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	return tree
}

// wantArchetyped is the ARCHETYPED decode must rebuild for archetypeID, with
// template_id only when templateID is non-empty.
func wantArchetyped(archetypeID, templateID string) map[string]any {
	ad := map[string]any{
		"_type":        "ARCHETYPED",
		"archetype_id": map[string]any{"_type": "ARCHETYPE_ID", "value": archetypeID},
		"rm_version":   rm.Release,
	}
	if templateID != "" {
		ad["template_id"] = map[string]any{"_type": "TEMPLATE_ID", "value": templateID}
	}
	return ad
}

// TestREQ053_DecodeRebuildsArchetypeDetails — FLAT and STRUCTURED carry no key
// for archetype_details, so decode rebuilds it from the Web Template on every
// archetype root (COMPOSITION, SECTION, ENTRY, slot-filled CLUSTER), with
// rm_version = rm.Release and template_id on the COMPOSITION root only, and on
// no at-coded node. Neither value needs the compiled template, so both decode
// modes do it.
func TestREQ053_DecodeRebuildsArchetypeDetails(t *testing.T) {
	wt, compiled := conformanceWT(t)
	flat, err := json.Marshal(archetypeDetailsBody())
	if err != nil {
		t.Fatal(err)
	}
	structured, err := FlatToStructured(flat)
	if err != nil {
		t.Fatalf("FlatToStructured: %v", err)
	}

	cases := []struct {
		name   string
		decode func() (*rm.Composition, error)
	}{
		{"FLAT with template", func() (*rm.Composition, error) {
			return UnmarshalFlat(flat, wt, WithTemplate(compiled))
		}},
		{"FLAT without template", func() (*rm.Composition, error) { return UnmarshalFlat(flat, wt) }},
		{"STRUCTURED with template", func() (*rm.Composition, error) {
			return UnmarshalStructured(structured, wt, WithTemplate(compiled))
		}},
		{"STRUCTURED without template", func() (*rm.Composition, error) { return UnmarshalStructured(structured, wt) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			comp, err := tc.decode()
			if err != nil {
				t.Fatalf("decode: %v", err)
			}
			if comp.ArchetypeDetails == nil || comp.ArchetypeDetails.RMVersion != rm.Release {
				t.Errorf("COMPOSITION archetype_details = %+v, want rm_version %q", comp.ArchetypeDetails, rm.Release)
			}
			nodes := locatableNodes(canonicalTree(t, comp))

			for _, id := range []string{adComposition, adSection, adObservation, adEvaluation, adCluster} {
				if len(nodes[id]) == 0 {
					t.Errorf("no decoded node carries archetype_node_id %q — fixture changed?", id)
				}
			}
			var atCoded int
			for id, found := range nodes {
				_, parseErr := rm.ParseArchetypeID(id)
				for _, n := range found {
					got, has := n.obj["archetype_details"]
					if parseErr != nil {
						atCoded++
						if has {
							t.Errorf("at-coded node %s (%s) carries archetype_details %v, want none", n.path, id, got)
						}
						continue
					}
					templateID := ""
					if n.path == "" {
						templateID = wt.TemplateID
					}
					if want := wantArchetyped(id, templateID); !reflect.DeepEqual(got, want) {
						t.Errorf("archetype_details at %q (%s) = %v, want %v", n.path, id, got, want)
					}
				}
			}
			if atCoded == 0 {
				t.Error("no at-coded node was checked — the negative half asserted nothing")
			}
		})
	}
}

// TestREQ053_RebuildArchetypeDetailsOverwrites — decode owns archetype_details:
// the rebuild writes the Web Template's ARCHETYPED on every qualifying node
// whatever the tree held there, so rm_version is always rm.Release and the
// COMPOSITION root always carries the Web Template's template_id.
func TestREQ053_RebuildArchetypeDetailsOverwrites(t *testing.T) {
	wt, _ := conformanceWT(t)
	stale := func() map[string]any {
		return map[string]any{
			"_type":        "ARCHETYPED",
			"archetype_id": map[string]any{"_type": "ARCHETYPE_ID", "value": "openEHR-EHR-SECTION.stale.v9"},
			"template_id":  map[string]any{"_type": "TEMPLATE_ID", "value": "stale.template.v1"},
			"rm_version":   "1.0.4",
		}
	}
	section := map[string]any{
		"_type":             "SECTION",
		"archetype_node_id": adSection,
		"archetype_details": stale(),
	}
	tree := map[string]any{
		"_type":             "COMPOSITION",
		"archetype_node_id": adComposition,
		"archetype_details": stale(),
		"content":           []any{section},
	}
	rebuildArchetypeDetails(tree, wt)

	if got, want := tree["archetype_details"], wantArchetyped(adComposition, wt.TemplateID); !reflect.DeepEqual(got, want) {
		t.Errorf("root archetype_details = %v, want %v", got, want)
	}
	if got, want := section["archetype_details"], wantArchetyped(adSection, ""); !reflect.DeepEqual(got, want) {
		t.Errorf("SECTION archetype_details = %v, want %v", got, want)
	}
}

// TestREQ053_RebuildArchetypeDetailsNeedsATemplateNode — the rebuild qualifies
// an id only when the Web Template identifies a node by it and it has the
// ARCHETYPE_ID lexical form: an archetype-shaped id the template does not
// declare gets nothing, and an at-code the template does declare gets nothing.
func TestREQ053_RebuildArchetypeDetailsNeedsATemplateNode(t *testing.T) {
	wt, _ := conformanceWT(t)
	stray := map[string]any{"_type": "CLUSTER", "archetype_node_id": "openEHR-EHR-CLUSTER.not_in_template.v1"}
	atCoded := map[string]any{"_type": "HISTORY", "archetype_node_id": "at0001"}
	tree := map[string]any{
		"_type":             "COMPOSITION",
		"archetype_node_id": adComposition,
		"content":           []any{stray, atCoded},
	}
	rebuildArchetypeDetails(tree, wt)

	if _, has := stray["archetype_details"]; has {
		t.Errorf("node %q, not in the Web Template, gained archetype_details", stray["archetype_node_id"])
	}
	if _, has := atCoded["archetype_details"]; has {
		t.Errorf("at-coded node %q gained archetype_details", atCoded["archetype_node_id"])
	}
	if want := wantArchetyped(adComposition, wt.TemplateID); !reflect.DeepEqual(tree["archetype_details"], want) {
		t.Errorf("root archetype_details = %v, want %v", tree["archetype_details"], want)
	}
}

// TestREQ053_ArchetypeRootIDsFromTheWebTemplate pins the qualifying set per
// template: exactly the archetype ids the OPT declares (its archetype_id
// values), whether the Web Template carries one as a node id or, for the
// ITEM_TREE nested.en.v1 folds away, only as a path predicate. No at-code
// qualifies.
func TestREQ053_ArchetypeRootIDsFromTheWebTemplate(t *testing.T) {
	conformance, _ := conformanceWT(t)
	nested, _ := nestedWT(t)
	for _, tc := range []struct {
		name string
		wt   *webtemplate.WebTemplate
		want []string
	}{
		{"conformance", conformance, []string{
			"openEHR-EHR-ACTION.conformance_action_.v0",
			"openEHR-EHR-ADMIN_ENTRY.conformance_admin_entry.v0",
			adCluster,
			adComposition,
			adEvaluation,
			"openEHR-EHR-INSTRUCTION.conformance_instruction.v0",
			"openEHR-EHR-OBSERVATION.conformance_interval.v0",
			adObservation,
			adSection,
		}},
		{"nested", nested, []string{
			"openEHR-EHR-CLUSTER.nested.v1",
			"openEHR-EHR-CLUSTER.nested2.v1",
			"openEHR-EHR-COMPOSITION.nesting.v1",
			"openEHR-EHR-INSTRUCTION.nested.v1",
			nestedItemTree,
			"openEHR-EHR-SECTION.nested.v1",
		}},
		{"no tree", &webtemplate.WebTemplate{}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := slices.Sorted(maps.Keys(archetypeRootIDs(tc.wt)))
			if !slices.Equal(got, tc.want) {
				t.Errorf("archetypeRootIDs = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestREQ140_ArchetypeDetailsRoundTripIsRebuilt — archetype_details is the
// third attribute class out of FLAT's scope: encode writes nothing for it, so a
// canonical -> FLAT -> canonical round trip rebuilds it from the Web Template.
// An archetype_id that differs from its node id takes the node id, any other
// rm_version becomes rm.Release, the COMPOSITION's template_id becomes the Web
// Template's, and a template_id below the root is dropped.
func TestREQ140_ArchetypeDetailsRoundTripIsRebuilt(t *testing.T) {
	wt, compiled := conformanceWT(t)
	flat, err := json.Marshal(archetypeDetailsBody())
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := UnmarshalFlat(flat, wt, WithTemplate(compiled))
	if err != nil {
		t.Fatalf("UnmarshalFlat: %v", err)
	}

	// Give the source composition archetype_details that disagree with the Web
	// Template on every field the round trip is expected to rebuild.
	src := canonicalTree(t, decoded)
	src["archetype_details"] = map[string]any{
		"_type":        "ARCHETYPED",
		"archetype_id": map[string]any{"_type": "ARCHETYPE_ID", "value": adComposition},
		"template_id":  map[string]any{"_type": "TEMPLATE_ID", "value": "some.other.template.v1"},
		"rm_version":   "1.0.4",
	}
	sections := locatableNodes(src)[adSection]
	if len(sections) == 0 {
		t.Fatalf("decoded source has no node %q — fixture changed?", adSection)
	}
	section := sections[0].obj
	section["archetype_details"] = map[string]any{
		"_type":        "ARCHETYPED",
		"archetype_id": map[string]any{"_type": "ARCHETYPE_ID", "value": "openEHR-EHR-SECTION.not_this_node.v9"},
		"template_id":  map[string]any{"_type": "TEMPLATE_ID", "value": "nested.template.v1"},
		"rm_version":   "1.1.0",
	}
	b, err := json.Marshal(src)
	if err != nil {
		t.Fatal(err)
	}
	var comp rm.Composition
	if err := canjson.Unmarshal(b, &comp); err != nil {
		t.Fatalf("canjson.Unmarshal of the edited source: %v", err)
	}
	if comp.ArchetypeDetails == nil || comp.ArchetypeDetails.RMVersion != "1.0.4" {
		t.Fatalf("edited source lost its archetype_details before encode: %+v", comp.ArchetypeDetails)
	}

	encoded, err := MarshalFlat(&comp, wt)
	if err != nil {
		t.Fatalf("MarshalFlat: %v", err)
	}
	var keys map[string]any
	if err := json.Unmarshal(encoded, &keys); err != nil {
		t.Fatal(err)
	}
	for k := range keys {
		if strings.Contains(k, "archetype_details") || strings.Contains(k, "template_id") || strings.Contains(k, "rm_version") {
			t.Errorf("encode wrote %q; it writes nothing for archetype_details", k)
		}
	}

	back, err := UnmarshalFlat(encoded, wt, WithTemplate(compiled))
	if err != nil {
		t.Fatalf("UnmarshalFlat of the re-encoded body: %v", err)
	}
	nodes := locatableNodes(canonicalTree(t, back))
	for _, tc := range []struct {
		id, templateID string
	}{
		{adComposition, wt.TemplateID},
		{adSection, ""},
	} {
		if len(nodes[tc.id]) == 0 {
			t.Fatalf("round trip built no node %q", tc.id)
		}
		got := nodes[tc.id][0].obj["archetype_details"]
		if want := wantArchetyped(tc.id, tc.templateID); !reflect.DeepEqual(got, want) {
			t.Errorf("round-tripped archetype_details on %s = %v, want %v", tc.id, got, want)
		}
	}
}

// TestREQ053_ArchetypeDetailsLeavesPhantomsVisible — a sparse :index on an archetype
// root gap-fills an empty instance, and the phantom check must still see it as
// empty. An ARCHETYPED added before that check would make the phantom look
// populated, so the rebuild runs after it and the body is still refused.
func TestREQ053_ArchetypeDetailsLeavesPhantomsVisible(t *testing.T) {
	wt, compiled := conformanceWT(t)
	evaluation := rmattrSection + "/conformance_evaluation"
	body := rmattrBody(map[string]any{
		evaluation + ":0/dv_text":              "first",
		evaluation + ":0/language|code":        "en",
		evaluation + ":0/language|terminology": "ISO_639-1",
		evaluation + ":2/dv_text":              "third",
		evaluation + ":2/language|code":        "en",
		evaluation + ":2/language|terminology": "ISO_639-1",
	})
	flat, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	for name, opts := range map[string][]Option{
		"with template":    {WithTemplate(compiled)},
		"without template": nil,
	} {
		if _, err := UnmarshalFlat(flat, wt, opts...); !errors.Is(err, ErrUnknownPath) {
			t.Errorf("%s: UnmarshalFlat(evaluation :0 and :2, no :1) err = %v, want ErrUnknownPath", name, err)
		}
	}
}

// nestedItemTree is the archetyped ITEM_TREE of the nested.en.v1 template. It
// fills an ACTIVITY description, a structural wrapper the Web Template folds
// away: no Web Template node carries this id, only the paths of its children.
const nestedItemTree = "openEHR-EHR-ITEM_TREE.nested.v1"

// nestedWT compiles the nested.en.v1 corpus template and builds its Web
// Template.
func nestedWT(t *testing.T) (*webtemplate.WebTemplate, *templatecompile.Compiled) {
	t.Helper()
	opt, err := template.ParseFile(fixtures.TemplateOpt("nested.en.v1"))
	if err != nil {
		t.Fatalf("parse nested.en.v1 OPT: %v", err)
	}
	c, err := templatecompile.Compile(opt)
	if err != nil {
		t.Fatalf("compile nested.en.v1 OPT: %v", err)
	}
	wt, err := webtemplate.Build(c)
	if err != nil {
		t.Fatalf("build nested.en.v1 Web Template: %v", err)
	}
	return wt, c
}

// TestREQ053_DecodeRebuildsFoldedArchetypeRoots — the Web Template folds
// structural wrappers such as an ACTIVITY's ITEM_TREE and keeps an archetyped
// wrapper's id only as the predicate its children's paths carry. Decode
// rebuilds that wrapper with the id as its archetype_node_id, and it must get
// archetype_details like any other archetype root. The reference composition
// carries archetype_details on every node the decode must rebuild it on.
func TestREQ053_DecodeRebuildsFoldedArchetypeRoots(t *testing.T) {
	wt, compiled := nestedWT(t)
	raw, err := os.ReadFile(fixtures.CompositionJSON("nested.en.v1"))
	if err != nil {
		t.Fatal(err)
	}
	var ref rm.Composition
	if err := canjson.Unmarshal(raw, &ref); err != nil {
		t.Fatalf("canjson.Unmarshal reference: %v", err)
	}
	// Every archetype_node_id the reference gives archetype_details, the folded
	// ITEM_TREE among them.
	var refRoots []string
	for id, found := range locatableNodes(canonicalTree(t, &ref)) {
		for _, n := range found {
			if _, has := n.obj["archetype_details"]; has {
				refRoots = append(refRoots, id)
				break
			}
		}
	}
	if !slices.Contains(refRoots, nestedItemTree) {
		t.Fatalf("reference composition carries no archetype_details on %s — fixture changed?", nestedItemTree)
	}

	// The reference's root uid is a HIER_OBJECT_ID spelled as an
	// OBJECT_VERSION_ID, which encode refuses; it has no bearing on this test.
	ref.UID = nil
	flat, err := MarshalFlat(&ref, wt)
	if err != nil {
		t.Fatalf("MarshalFlat: %v", err)
	}
	for name, opts := range map[string][]Option{
		"with template":    {WithTemplate(compiled)},
		"without template": nil,
	} {
		t.Run(name, func(t *testing.T) {
			comp, err := UnmarshalFlat(flat, wt, opts...)
			if err != nil {
				t.Fatalf("UnmarshalFlat: %v", err)
			}
			nodes := locatableNodes(canonicalTree(t, comp))
			for _, id := range refRoots {
				found := nodes[id]
				if len(found) == 0 {
					t.Errorf("decode built no node with archetype_node_id %q", id)
					continue
				}
				for _, n := range found {
					templateID := ""
					if n.path == "" {
						templateID = wt.TemplateID
					}
					if got, want := n.obj["archetype_details"], wantArchetyped(id, templateID); !reflect.DeepEqual(got, want) {
						t.Errorf("archetype_details at %q (%s) = %v, want %v", n.path, id, got, want)
					}
				}
			}
		})
	}
}
