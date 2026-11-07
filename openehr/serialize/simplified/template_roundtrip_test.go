package simplified_test

// REQ-053 — a FLAT or STRUCTURED body decoded with WithTemplate validates
// against the OPT it was written from. The formats fold every ITEM_STRUCTURE
// away, so decode takes the folded structure's concrete subtype from the
// compiled template; FLAT carries order only among the instances of one
// node, so decode places the members of one container list in the Web
// Template's node order; and the formats have no key for a structure with no
// member, so decode rebuilds, empty, one the template requires and lets hold
// no member; and the formats have no key for a node's name below the root, so
// decode gives a node the coded name the template determines for it; and the
// formats carry an ordinal's code and value but not its terminology, so decode
// gives the symbol the terminology of the template's pair with that code and
// value.

import (
	"cmp"
	"encoding/json"
	"maps"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/cadasto/openehr-sdk-go/openehr/aom/aom14"
	"github.com/cadasto/openehr-sdk-go/openehr/composition"
	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/rmpath"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/simplified"
	"github.com/cadasto/openehr-sdk-go/openehr/template/webtemplate"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// templateOf builds the Web Template and the compiled template of the corpus
// OPT with the template id id.
func templateOf(t *testing.T, id string) (*webtemplate.WebTemplate, *templatecompile.Compiled) {
	t.Helper()
	opt, err := readOPTFile(fixtures.TemplateOpt(id))
	if err != nil {
		t.Fatalf("readOPTFile(%s): %v", id, err)
	}
	return compileTemplate(t, id, opt)
}

// compileTemplate compiles opt, the template named id, and builds its Web
// Template.
func compileTemplate(t *testing.T, id string, opt *aom14.OperationalTemplate) (*webtemplate.WebTemplate, *templatecompile.Compiled) {
	t.Helper()
	compiled, err := templatecompile.Compile(opt)
	if err != nil {
		t.Fatalf("Compile(%s): %v", id, err)
	}
	wt, err := webtemplate.Build(compiled)
	if err != nil {
		t.Fatalf("Build(%s): %v", id, err)
	}
	return wt, compiled
}

// bmiTemplate builds the Web Template and the compiled template of the BMI
// corpus OPT. Its OBSERVATION protocol is an ITEM_LIST of three ELEMENTs whose
// FLAT ids (method, formula, comment) sort in the reverse of their template
// order (at0006, at0010, at0011).
func bmiTemplate(t *testing.T) (*webtemplate.WebTemplate, *templatecompile.Compiled) {
	t.Helper()
	return templateOf(t, "BMI")
}

// skeletonOf is the composition NewSkeleton writes for compiled under policy,
// on a pinned clock. It must validate against compiled.
func skeletonOf(t *testing.T, compiled *templatecompile.Compiled, policy instance.Policy) *rm.Composition {
	t.Helper()
	name := "Composer"
	comp, err := composition.NewSkeleton(t.Context(), compiled,
		composition.WithPolicy(policy),
		composition.WithComposer(&rm.PartyIdentified{Name: &name}),
		composition.WithTerritory("NL"),
		composition.WithNow(time.Date(2026, 10, 26, 9, 0, 0, 0, time.UTC)))
	if err != nil {
		t.Fatalf("NewSkeleton(%s): %v", compiled.TemplateID(), err)
	}
	if r := validation.ValidateComposition(comp, compiled); !r.OK {
		t.Fatalf("the written %s skeleton is invalid: %v", compiled.TemplateID(), r.Issues)
	}
	return comp
}

// bmiSkeleton is the composition NewSkeleton writes for the BMI template under
// instance.Medium, on a pinned clock.
func bmiSkeleton(t *testing.T, compiled *templatecompile.Compiled) *rm.Composition {
	t.Helper()
	return skeletonOf(t, compiled, instance.Medium)
}

// bmiRoundTrip encodes comp in format ("FLAT" or "STRUCTURED") and decodes it
// back with opts.
func bmiRoundTrip(t *testing.T, format string, comp *rm.Composition, wt *webtemplate.WebTemplate, opts ...simplified.Option) *rm.Composition {
	t.Helper()
	encode, decode := simplified.MarshalFlat, simplified.UnmarshalFlat
	if format == "STRUCTURED" {
		encode, decode = simplified.MarshalStructured, simplified.UnmarshalStructured
	}
	body, err := encode(comp, wt)
	if err != nil {
		t.Fatalf("Marshal %s: %v", format, err)
	}
	back, err := decode(body, wt, opts...)
	if err != nil {
		t.Fatalf("Unmarshal %s: %v", format, err)
	}
	return back
}

// decodeAs decodes flat, a FLAT body, in format ("FLAT" or "STRUCTURED", to
// which it is converted first) with opts.
func decodeAs(t *testing.T, format string, flat []byte, wt *webtemplate.WebTemplate, opts ...simplified.Option) *rm.Composition {
	t.Helper()
	body, decode := flat, simplified.UnmarshalFlat
	if format == "STRUCTURED" {
		var err error
		if body, err = simplified.FlatToStructured(flat); err != nil {
			t.Fatalf("FlatToStructured: %v", err)
		}
		decode = simplified.UnmarshalStructured
	}
	back, err := decode(body, wt, opts...)
	if err != nil {
		t.Fatalf("Unmarshal %s: %v", format, err)
	}
	return back
}

// bmiProtocol returns the canonical JSON of the BMI OBSERVATION's protocol.
func bmiProtocol(t *testing.T, comp *rm.Composition) map[string]any {
	t.Helper()
	b, err := canjson.Marshal(comp)
	if err != nil {
		t.Fatalf("canjson.Marshal: %v", err)
	}
	var doc struct {
		Content []struct {
			Protocol map[string]any `json:"protocol"`
		} `json:"content"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatalf("unmarshal canonical JSON: %v", err)
	}
	if len(doc.Content) != 1 || doc.Content[0].Protocol == nil {
		t.Fatalf("canonical JSON carries %d content items, want one OBSERVATION with a protocol", len(doc.Content))
	}
	return doc.Content[0].Protocol
}

// memberIDs returns the archetype_node_id of each member of a structure's
// items list, in list order.
func memberIDs(structure map[string]any) []string {
	items, _ := structure["items"].([]any)
	ids := make([]string, 0, len(items))
	for _, it := range items {
		m, _ := it.(map[string]any)
		id, _ := m["archetype_node_id"].(string)
		ids = append(ids, id)
	}
	return ids
}

// TestREQ053WithTemplateRoundTripValidatesBMI — REQ-053: the BMI skeleton,
// written as FLAT or STRUCTURED and decoded with WithTemplate, validates
// against the OPT. Its protocol comes back as the ITEM_LIST the template
// names, not the ITEM_TREE the path alone would suggest, and its members in
// the order they were written.
func TestREQ053WithTemplateRoundTripValidatesBMI(t *testing.T) {
	wt, compiled := bmiTemplate(t)
	comp := bmiSkeleton(t, compiled)
	written := bmiProtocol(t, comp)
	if written["_type"] != "ITEM_LIST" {
		t.Fatalf("the written protocol is a %v, want ITEM_LIST — fixture changed?", written["_type"])
	}
	for _, format := range []string{"FLAT", "STRUCTURED"} {
		t.Run(format, func(t *testing.T) {
			back := bmiRoundTrip(t, format, comp, wt, simplified.WithTemplate(compiled), simplified.WithStrict())
			for _, is := range validation.ValidateComposition(back, compiled).Issues {
				t.Errorf("decoded composition: %s at %s: %s", is.Code, is.Path, is.Detail)
			}
			got := bmiProtocol(t, back)
			if got["_type"] != "ITEM_LIST" {
				t.Errorf("decoded protocol _type = %v, want ITEM_LIST", got["_type"])
			}
			if g, w := memberIDs(got), memberIDs(written); !slices.Equal(g, w) {
				t.Errorf("decoded protocol members = %v, want the written order %v", g, w)
			}
		})
	}
}

// TestREQ053OPTFreeDecodeKeepsTheInference — REQ-053: without WithTemplate the
// path alone names the folded structure, and `items` stays ITEM_TREE. The
// members still come back in template order, which the Web Template alone
// gives.
func TestREQ053OPTFreeDecodeKeepsTheInference(t *testing.T) {
	wt, compiled := bmiTemplate(t)
	comp := bmiSkeleton(t, compiled)
	written := memberIDs(bmiProtocol(t, comp))
	for _, format := range []string{"FLAT", "STRUCTURED"} {
		t.Run(format, func(t *testing.T) {
			got := bmiProtocol(t, bmiRoundTrip(t, format, comp, wt))
			if got["_type"] != "ITEM_TREE" {
				t.Errorf("OPT-free decoded protocol _type = %v, want ITEM_TREE", got["_type"])
			}
			if g := memberIDs(got); !slices.Equal(g, written) {
				t.Errorf("OPT-free decoded protocol members = %v, want the written order %v", g, written)
			}
		})
	}
}

// canonicalDoc returns comp as its canonical JSON object.
func canonicalDoc(t *testing.T, comp *rm.Composition) map[string]any {
	t.Helper()
	b, err := canjson.Marshal(comp)
	if err != nil {
		t.Fatalf("canjson.Marshal: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatalf("unmarshal canonical JSON: %v", err)
	}
	return doc
}

// nodeAt follows path, a compiled template path whose segments are an
// attribute, with the archetype_node_id of a list member in brackets, from
// the canonical JSON object doc, and returns the object there.
func nodeAt(doc map[string]any, path string) (map[string]any, bool) {
	cur := doc
	for seg := range strings.SplitSeq(strings.TrimPrefix(path, "/"), "/") {
		attr, id, member := strings.Cut(seg, "[")
		if !member {
			next, ok := cur[attr].(map[string]any)
			if !ok {
				return nil, false
			}
			cur = next
			continue
		}
		id = strings.TrimSuffix(id, "]")
		list, _ := cur[attr].([]any)
		found := false
		for _, e := range list {
			if m, ok := e.(map[string]any); ok && m["archetype_node_id"] == id {
				cur, found = m, true
				break
			}
		}
		if !found {
			return nil, false
		}
	}
	return cur, true
}

// jsonText renders v as JSON for a failure message.
func jsonText(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return err.Error()
	}
	return string(b)
}

// emptyStructureCases are corpus skeletons that write, with no member, a
// structure the template requires and lets hold no member: an ITEM_LIST or an
// ITEM_TREE whose items it leaves free. FLAT and STRUCTURED have no key for
// such a structure.
var emptyStructureCases = []struct {
	template string
	policy   instance.Policy
	paths    []string
}{
	{template: "vital_signs", policy: instance.Medium, paths: []string{
		"/content[openEHR-EHR-OBSERVATION.blood_pressure.v1]/data/events[at0006]/state",
		"/content[openEHR-EHR-OBSERVATION.blood_pressure.v1]/protocol",
	}},
	{template: "sdk_crosscheck_structure.v1", policy: instance.Required, paths: []string{
		"/content[openEHR-EHR-OBSERVATION.sdk_structure.v1]/data/events[at0002]/data",
	}},
	{template: "cluster-slot.ehrbase.org.v0", policy: instance.Medium, paths: []string{
		"/content[openEHR-EHR-OBSERVATION.cluster_slot.v0]/data/events[at0002]/data",
	}},
}

// structureMembers returns the members the canonical JSON object s, an item
// structure, holds: its rows for an ITEM_TABLE, its items otherwise. An
// absent list and an empty one hold none.
func structureMembers(s map[string]any) []any {
	key := "items"
	if s["_type"] == "ITEM_TABLE" {
		key = "rows"
	}
	members, _ := s[key].([]any)
	return members
}

// writtenEmptyStructures returns the structure the skeleton comp writes at
// each of paths, each of which must hold no member: its list is absent, or
// present and empty where the template requires it by its existence.
func writtenEmptyStructures(t *testing.T, comp *rm.Composition, paths []string) map[string]map[string]any {
	t.Helper()
	doc := canonicalDoc(t, comp)
	written := make(map[string]map[string]any, len(paths))
	for _, p := range paths {
		s, ok := nodeAt(doc, p)
		if !ok {
			t.Fatalf("the written skeleton has no structure at %s — fixture changed?", p)
		}
		if members := structureMembers(s); len(members) > 0 {
			t.Fatalf("the written structure at %s holds %d members — fixture changed?", p, len(members))
		}
		written[p] = s
	}
	return written
}

// TestREQ053WithTemplateRebuildsRequiredEmptyList — REQ-053: the vital_signs
// skeleton writes the blood_pressure event's state and the protocol with no
// member, and with their items present and empty, which the template
// requires by its existence (existence 1..1, cardinality 0..*, no child or
// slots only), as "items":[]. A WithTemplate decode of its FLAT and
// STRUCTURED bodies rebuilds both structures as the generator writes them,
// "items":[] included, so the decoded structure carries the list the
// written one does.
func TestREQ053WithTemplateRebuildsRequiredEmptyList(t *testing.T) {
	paths := emptyStructureCases[0].paths
	wt, compiled := templateOf(t, emptyStructureCases[0].template)
	comp := skeletonOf(t, compiled, emptyStructureCases[0].policy)
	for p, s := range writtenEmptyStructures(t, comp, paths) {
		if items, ok := s["items"].([]any); !ok || len(items) != 0 {
			t.Fatalf("the written structure at %s = %s, want its items as [] — fixture changed?", p, jsonText(s))
		}
	}
	for _, format := range []string{"FLAT", "STRUCTURED"} {
		t.Run(format, func(t *testing.T) {
			doc := canonicalDoc(t, bmiRoundTrip(t, format, comp, wt, simplified.WithTemplate(compiled), simplified.WithStrict()))
			for _, p := range paths {
				got, ok := nodeAt(doc, p)
				if !ok {
					t.Errorf("decoded composition has no structure at %s, want one with items []", p)
					continue
				}
				if items, ok := got["items"].([]any); !ok || len(items) != 0 {
					t.Errorf("decoded structure at %s = %s, want its items as []: the template requires them by their existence", p, jsonText(got))
				}
			}
		})
	}
}

// TestREQ053WithTemplateRebuildsEmptyRequiredStructure — REQ-053: a structure
// the template requires and lets hold no member has no FLAT or STRUCTURED key,
// so a WithTemplate decode rebuilds it, as the skeleton wrote it, under the
// node that holds it: the round trip then validates against the OPT.
func TestREQ053WithTemplateRebuildsEmptyRequiredStructure(t *testing.T) {
	for _, tc := range emptyStructureCases {
		wt, compiled := templateOf(t, tc.template)
		comp := skeletonOf(t, compiled, tc.policy)
		written := writtenEmptyStructures(t, comp, tc.paths)
		for _, format := range []string{"FLAT", "STRUCTURED"} {
			t.Run(tc.template+"/"+format, func(t *testing.T) {
				back := bmiRoundTrip(t, format, comp, wt, simplified.WithTemplate(compiled), simplified.WithStrict())
				for _, is := range validation.ValidateComposition(back, compiled).Issues {
					t.Errorf("decoded composition: %s at %s: %s", is.Code, is.Path, is.Detail)
				}
				doc := canonicalDoc(t, back)
				for _, p := range tc.paths {
					got, ok := nodeAt(doc, p)
					if !ok {
						t.Errorf("decoded composition has no structure at %s, want the written %s", p, jsonText(written[p]))
						continue
					}
					if !reflect.DeepEqual(got, written[p]) {
						t.Errorf("decoded structure at %s = %s, want the written %s", p, jsonText(got), jsonText(written[p]))
					}
				}
			})
		}
	}
}

// TestREQ053OPTFreeDecodeRebuildsNoEmptyStructure — REQ-053: without
// WithTemplate the decode has no template to read a requirement from, and
// rebuilds no structure the body has no key for.
func TestREQ053OPTFreeDecodeRebuildsNoEmptyStructure(t *testing.T) {
	for _, tc := range emptyStructureCases {
		wt, compiled := templateOf(t, tc.template)
		comp := skeletonOf(t, compiled, tc.policy)
		writtenEmptyStructures(t, comp, tc.paths)
		for _, format := range []string{"FLAT", "STRUCTURED"} {
			t.Run(tc.template+"/"+format, func(t *testing.T) {
				doc := canonicalDoc(t, bmiRoundTrip(t, format, comp, wt))
				for _, p := range tc.paths {
					if got, ok := nodeAt(doc, p); ok {
						t.Errorf("OPT-free decode rebuilt %s at %s, want none", jsonText(got), p)
					}
				}
			})
		}
	}
}

// codedNameNode is the ELEMENT of sdk_crosscheck_values.v1 whose name the
// template constrains as a DV_CODED_TEXT: its text pinned to "Second option",
// its code one of the local at0017 and at0018, whose terms are "First option"
// and "Second option".
const codedNameNode = "/content[openEHR-EHR-OBSERVATION.sdk_values.v1]/data/events[at0002]/data/items[at0016]"

// nameAt returns the name of the node at path in comp's canonical JSON.
func nameAt(t *testing.T, comp *rm.Composition, path string) (map[string]any, bool) {
	t.Helper()
	node, ok := nodeAt(canonicalDoc(t, comp), path)
	if !ok {
		return nil, false
	}
	name, ok := node["name"].(map[string]any)
	return name, ok
}

// TestREQ053WithTemplateRebuildsCodedName — REQ-053, REQ-116: FLAT has no key
// for a node's name below the root, so a WithTemplate decode names the node
// from the template. Where the template constrains the name as a
// DV_CODED_TEXT and determines its code and text, decode rebuilds that coded
// name, as the skeleton wrote it, and the round trip validates against the
// OPT.
func TestREQ053WithTemplateRebuildsCodedName(t *testing.T) {
	wt, compiled := templateOf(t, "sdk_crosscheck_values.v1")
	comp := skeletonOf(t, compiled, instance.Medium)
	written, ok := nameAt(t, comp, codedNameNode)
	if !ok || written["_type"] != "DV_CODED_TEXT" {
		t.Fatalf("the written name at %s is %s, want a DV_CODED_TEXT — fixture changed?", codedNameNode, jsonText(written))
	}
	for _, format := range []string{"FLAT", "STRUCTURED"} {
		t.Run(format, func(t *testing.T) {
			back := bmiRoundTrip(t, format, comp, wt, simplified.WithTemplate(compiled), simplified.WithStrict())
			for _, is := range validation.ValidateComposition(back, compiled).Issues {
				t.Errorf("decoded composition: %s at %s: %s", is.Code, is.Path, is.Detail)
			}
			if got, _ := nameAt(t, back, codedNameNode); !reflect.DeepEqual(got, written) {
				t.Errorf("decoded name at %s = %s, want the written %s", codedNameNode, jsonText(got), jsonText(written))
			}
		})
	}
}

// TestREQ053WithTemplateRebuildsNoStructureThatNeedsAMember — REQ-053: where
// the template requires a member of the structure, a body with none lacks
// data, and decode rebuilds no empty structure for it: the validator's
// `required` is the right answer. The BMI event's data is an ITEM_TREE whose
// items ask for a member (cardinality 1..*, the BMI ELEMENT 1..1); the body
// keeps the event, by its time, and drops the BMI value.
func TestREQ053WithTemplateRebuildsNoStructureThatNeedsAMember(t *testing.T) {
	const (
		data  = "/content[openEHR-EHR-OBSERVATION.body_mass_index.v1]/data/events[at0002]/data"
		value = "result_report_date/body_mass_index:0/any_event:0/body_mass_index|"
	)
	wt, compiled := bmiTemplate(t)
	body, err := simplified.MarshalFlat(bmiSkeleton(t, compiled), wt)
	if err != nil {
		t.Fatalf("MarshalFlat: %v", err)
	}
	var flat map[string]any
	if err := json.Unmarshal(body, &flat); err != nil {
		t.Fatalf("unmarshal FLAT: %v", err)
	}
	dropped := 0
	for k := range flat {
		if strings.HasPrefix(k, value) {
			delete(flat, k)
			dropped++
		}
	}
	if dropped == 0 {
		t.Fatalf("the BMI body has no key %s… — fixture changed?", value)
	}
	if body, err = json.Marshal(flat); err != nil {
		t.Fatalf("marshal FLAT: %v", err)
	}
	for _, format := range []string{"FLAT", "STRUCTURED"} {
		t.Run(format, func(t *testing.T) {
			back := decodeAs(t, format, body, wt, simplified.WithTemplate(compiled), simplified.WithStrict())
			event := strings.TrimSuffix(data, "/data")
			doc := canonicalDoc(t, back)
			if _, ok := nodeAt(doc, event); !ok {
				t.Fatalf("decoded composition has no event at %s — the body should keep it", event)
			}
			if got, ok := nodeAt(doc, data); ok {
				t.Errorf("decode rebuilt %s at %s, want none: the template asks it for a member", jsonText(got), data)
			}
			issues := validation.ValidateComposition(back, compiled).Issues
			if !slices.ContainsFunc(issues, func(is validation.Issue) bool { return is.Code == "required" && is.Path == data }) {
				t.Errorf("ValidateComposition issues = %v, want `required` at %s", issues, data)
			}
		})
	}
}

// actionOf returns the one ACTION the minimal_action_2 composition comp holds.
func actionOf(t *testing.T, comp *rm.Composition) *rm.Action {
	t.Helper()
	if len(comp.Content) != 1 {
		t.Fatalf("composition holds %d content items, want the one ACTION", len(comp.Content))
	}
	a, ok := comp.Content[0].(*rm.Action)
	if !ok {
		t.Fatalf("content[0] = %T, want *rm.Action", comp.Content[0])
	}
	return a
}

// TestREQ053WithTemplateRoundTripKeepsInContextIsmTransition — REQ-053,
// REQ-106: the minimal_action_2 ACTION archetype constrains no ISM transition,
// but ACTION.ism_transition is RM-mandatory, and the skeleton writes one. The
// Web Template gives such an ACTION the reference's in-context
// `ism_transition` node with its `current_state` leaf, so FLAT and STRUCTURED
// carry the current state both ways, and the round trip validates against the
// OPT.
func TestREQ053WithTemplateRoundTripKeepsInContextIsmTransition(t *testing.T) {
	wt, compiled := templateOf(t, "minimal_action_2")
	comp := skeletonOf(t, compiled, instance.Medium)
	written := actionOf(t, comp).IsmTransition.CurrentState
	if written.DefiningCode.CodeString == "" {
		t.Fatalf("the written ACTION has no current_state code — fixture changed?")
	}
	for _, format := range []string{"FLAT", "STRUCTURED"} {
		t.Run(format, func(t *testing.T) {
			back := bmiRoundTrip(t, format, comp, wt, simplified.WithTemplate(compiled), simplified.WithStrict())
			for _, is := range validation.ValidateComposition(back, compiled).Issues {
				t.Errorf("decoded composition: %s at %s: %s", is.Code, is.Path, is.Detail)
			}
			if got := actionOf(t, back).IsmTransition.CurrentState; !reflect.DeepEqual(got, written) {
				t.Errorf("decoded ACTION current_state = %+v, want the written %+v", got, written)
			}
		})
	}
}

// TestREQ053WithTemplateOrdinalTerminology — REQ-053. FLAT and STRUCTURED carry
// an ordinal's code and value but not its terminology. Decode without the
// template rebuilds the symbol in local, as the reference does; a WithTemplate
// decode gives it the terminology of the template's C_DV_ORDINAL pair with that
// code and value, so a template whose ordinals are coded outside local decodes
// to the value it lists. The pair is found below a node the template names,
// a |raw ordinal keeps its own terminology, and a pair the template gives two
// terminologies stays local.
func TestREQ053WithTemplateOrdinalTerminology(t *testing.T) {
	const (
		id         = "Test_dv_ordinal_with_constraints.v0"
		valuePath  = "/content[openEHR-EHR-OBSERVATION.test123.v0]/data/events[at0002]/data/items[at0037]/value"
		localPair  = "<value>1</value>\n                            <symbol>\n<defining_code>\n    <terminology_id>\n        <value>local</value>\n    </terminology_id>\n    <code_string>at0039</code_string>\n</defining_code>\n                            </symbol>\n                        </list>"
		element    = "<node_id>at0037</node_id>"
		exactlyOne = "<existence><lower_included>true</lower_included><upper_included>true</upper_included>" +
			"<lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded><lower>1</lower><upper>1</upper></existence>"
	)
	raw, err := os.ReadFile(fixtures.TemplateOpt(id))
	if err != nil {
		t.Fatal(err)
	}
	for _, anchor := range []string{localPair, element} {
		if n := strings.Count(string(raw), anchor); n != 1 {
			t.Fatalf("%s holds %q %d times, want once", id, anchor, n)
		}
	}
	// pair spells the template's one ordinal pair with the value and the
	// terminology given; withPairs puts pairs in its place.
	pair := func(value, terminology string) string {
		return strings.Replace(strings.Replace(localPair, "<value>1</value>", "<value>"+value+"</value>", 1),
			"<value>local</value>", "<value>"+terminology+"</value>", 1)
	}
	withPairs := func(opt string, pairs ...string) string {
		return strings.Replace(opt, localPair, strings.Join(pairs, "\n                        <list>"), 1)
	}
	snomed := withPairs(string(raw), pair("1", "SNOMED-CT"))
	// named pins the name 'Schmerzgrad' on the ordinal's ELEMENT, so its path
	// in the compiled template carries the name.
	named := strings.Replace(snomed, element, element+
		`<attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>name</rm_attribute_name>`+exactlyOne+
		`<children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>DV_TEXT</rm_type_name>`+strings.ReplaceAll(exactlyOne, "existence", "occurrences")+`<node_id/>`+
		`<attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>value</rm_attribute_name>`+exactlyOne+
		`<children xsi:type="C_PRIMITIVE_OBJECT"><rm_type_name>STRING</rm_type_name>`+strings.ReplaceAll(exactlyOne, "existence", "occurrences")+`<node_id/>`+
		`<item xsi:type="C_STRING"><list>Schmerzgrad</list></item></children></attributes></children></attributes>`, 1)
	// apart gives the code local at 1 and SNOMED-CT at 2; twice gives it both
	// at 1, in either order.
	apart := withPairs(string(raw), pair("1", "local"), pair("2", "SNOMED-CT"))
	twice := []string{
		withPairs(string(raw), pair("1", "local"), pair("1", "SNOMED-CT")),
		withPairs(string(raw), pair("1", "SNOMED-CT"), pair("1", "local")),
	}

	compileEdited := func(t *testing.T, opt string) (*webtemplate.WebTemplate, *templatecompile.Compiled) {
		t.Helper()
		model, err := aom14.ReadOperationalTemplate(strings.NewReader(opt))
		if err != nil {
			t.Fatalf("ReadOperationalTemplate: %v", err)
		}
		return compileTemplate(t, id, model)
	}
	// terminologyOf returns the terminology of the decoded ordinal's symbol.
	terminologyOf := func(t *testing.T, comp *rm.Composition) string {
		t.Helper()
		items, err := rmpath.ItemsAtPath(comp, valuePath)
		if err != nil || len(items) != 1 {
			t.Fatalf("ItemsAtPath(%s) = %d items, %v; want the ordinal", valuePath, len(items), err)
		}
		ord, ok := items[0].(*rm.DVOrdinal)
		if !ok {
			t.Fatalf("%s holds a %T, want *rm.DVOrdinal", valuePath, items[0])
		}
		return ord.Symbol.DefiningCode.TerminologyID.Value
	}
	// encode writes the skeleton of compiled as FLAT, and returns the body and
	// the key of the ordinal, which the encoder writes as |raw.
	encode := func(t *testing.T, wt *webtemplate.WebTemplate, compiled *templatecompile.Compiled) (map[string]any, string) {
		t.Helper()
		body, err := simplified.MarshalFlat(skeletonOf(t, compiled, instance.Large), wt)
		if err != nil {
			t.Fatalf("MarshalFlat: %v", err)
		}
		var flat map[string]any
		if err := json.Unmarshal(body, &flat); err != nil {
			t.Fatal(err)
		}
		for key := range flat {
			if strings.HasSuffix(key, "|raw") {
				return flat, key
			}
		}
		t.Fatalf("MarshalFlat wrote no |raw ordinal: %v", slices.Sorted(maps.Keys(flat)))
		return nil, ""
	}
	// readerFlat spells the |raw ordinal at rawKey as the reference does, |code,
	// |value and |ordinal with no terminology, the ordinal value replaced by
	// ordinal where it is not nil.
	readerFlat := func(t *testing.T, flat map[string]any, rawKey string, ordinal any) []byte {
		t.Helper()
		frag, _ := flat[rawKey].(map[string]any)
		symbol, _ := frag["symbol"].(map[string]any)
		code, _ := symbol["defining_code"].(map[string]any)
		out := maps.Clone(flat)
		base := strings.TrimSuffix(rawKey, "|raw")
		delete(out, rawKey)
		out[base+"|code"], out[base+"|value"], out[base+"|ordinal"] = code["code_string"], symbol["value"], cmp.Or(ordinal, frag["value"])
		b, err := json.Marshal(out)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}

	for _, format := range []string{"FLAT", "STRUCTURED"} {
		t.Run(format, func(t *testing.T) {
			wt, compiled := compileEdited(t, snomed)
			flat, rawKey := encode(t, wt, compiled)
			body := readerFlat(t, flat, rawKey, nil)
			if got := terminologyOf(t, decodeAs(t, format, body, wt)); got != "local" {
				t.Errorf("decode without the template: symbol terminology %q, want local", got)
			}
			back := decodeAs(t, format, body, wt, simplified.WithTemplate(compiled))
			if got := terminologyOf(t, back); got != "SNOMED-CT" {
				t.Errorf("WithTemplate decode: symbol terminology %q, want SNOMED-CT, the template's", got)
			}
			for _, is := range validation.ValidateComposition(back, compiled).Issues {
				if is.Path == valuePath {
					t.Errorf("WithTemplate decode: %s: %s: %s", is.Path, is.Code, is.Detail)
				}
			}

			// A |raw ordinal is decoded as written, its terminology included.
			localRaw := maps.Clone(flat)
			frag := maps.Clone(flat[rawKey].(map[string]any))
			symbol := maps.Clone(frag["symbol"].(map[string]any))
			code := maps.Clone(symbol["defining_code"].(map[string]any))
			code["terminology_id"] = map[string]any{"_type": "TERMINOLOGY_ID", "value": "local"}
			symbol["defining_code"], frag["symbol"], localRaw[rawKey] = code, symbol, frag
			b, err := json.Marshal(localRaw)
			if err != nil {
				t.Fatal(err)
			}
			if got := terminologyOf(t, decodeAs(t, format, b, wt, simplified.WithTemplate(compiled))); got != "local" {
				t.Errorf("WithTemplate decode of a |raw ordinal in local: symbol terminology %q, want local, as written", got)
			}

			// Below a node the template names, the pair is still found.
			wtNamed, compiledNamed := compileEdited(t, named)
			flatNamed, rawKeyNamed := encode(t, wtNamed, compiledNamed)
			if !strings.Contains(rawKeyNamed, "schmerzgrad") {
				t.Fatalf("the ordinal's key %q does not carry the pinned name", rawKeyNamed)
			}
			if got := terminologyOf(t, decodeAs(t, format, readerFlat(t, flatNamed, rawKeyNamed, nil), wtNamed, simplified.WithTemplate(compiledNamed))); got != "SNOMED-CT" {
				t.Errorf("WithTemplate decode below a named node: symbol terminology %q, want SNOMED-CT", got)
			}

			// The code at each ordinal value takes that pair's terminology,
			// the value written as a JSON number or a numeric string.
			wtApart, compiledApart := compileEdited(t, apart)
			for _, tc := range []struct {
				ordinal any
				want    string
			}{
				{ordinal: 1, want: "local"},
				{ordinal: "2.0", want: "SNOMED-CT"},
			} {
				if got := terminologyOf(t, decodeAs(t, format, readerFlat(t, flat, rawKey, tc.ordinal), wtApart, simplified.WithTemplate(compiledApart))); got != tc.want {
					t.Errorf("WithTemplate decode at ordinal %v of local 1 and SNOMED-CT 2: symbol terminology %q, want %s", tc.ordinal, got, tc.want)
				}
			}

			// A code and value the template gives local and SNOMED-CT have no
			// single terminology, and the symbol stays local.
			for i, opt := range twice {
				wtTwice, compiledTwice := compileEdited(t, opt)
				if got := terminologyOf(t, decodeAs(t, format, body, wtTwice, simplified.WithTemplate(compiledTwice))); got != "local" {
					t.Errorf("WithTemplate decode, one pair in two terminologies (order %d): symbol terminology %q, want local", i, got)
				}
			}
		})
	}
}

// TestREQ053WithTemplateOrdinalIntervalTerminology — REQ-053. A bound of a
// DV_INTERVAL<DV_ORDINAL> leaf carries its code and value but not its
// terminology, like an ordinal leaf. A WithTemplate decode gives a bound built
// from its suffixes the terminology of the template's pair for that bound, and
// a |raw bound keeps its own; decode without the template leaves both local.
func TestREQ053WithTemplateOrdinalIntervalTerminology(t *testing.T) {
	const (
		id         = "Test_dv_interval_dv_count_lower_upper_constraint.v0"
		countType  = "DV_INTERVAL&lt;DV_COUNT&gt;"
		boundStart = "<children xsi:type=\"C_COMPLEX_OBJECT\">\n<rm_type_name>DV_COUNT</rm_type_name>"
		boundEnd   = "</attributes>\n                            </children>"
		exactlyOne = "<lower_included>true</lower_included><upper_included>true</upper_included>" +
			"<lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded><lower>1</lower><upper>1</upper>"
		// ordinalBound is a C_DV_ORDINAL whose one pair codes 111 in SNOMED-CT.
		ordinalBound = `<children xsi:type="C_DV_ORDINAL"><rm_type_name>DV_ORDINAL</rm_type_name><occurrences>` + exactlyOne +
			`</occurrences><node_id></node_id><list><value>1</value><symbol><defining_code><terminology_id><value>SNOMED-CT</value>` +
			`</terminology_id><code_string>111</code_string></defining_code></symbol></list></children>`
	)
	raw, err := os.ReadFile(fixtures.TemplateOpt(id))
	if err != nil {
		t.Fatal(err)
	}
	// Retarget the DV_INTERVAL<DV_COUNT> leaf to DV_INTERVAL<DV_ORDINAL>, each
	// bound a C_DV_ORDINAL in place of the C_DV_COUNT.
	opt := string(raw)
	if strings.Count(opt, countType) != 1 || strings.Count(opt, boundStart) != 2 {
		t.Fatalf("%s no longer holds one %s with two DV_COUNT bounds", id, countType)
	}
	opt = strings.Replace(opt, countType, "DV_INTERVAL&lt;DV_ORDINAL&gt;", 1)
	for range 2 {
		start := strings.Index(opt, boundStart)
		end := strings.Index(opt[start:], boundEnd)
		if end < 0 {
			t.Fatalf("%s: a DV_COUNT bound is not closed as expected", id)
		}
		opt = opt[:start] + ordinalBound + opt[start+end+len(boundEnd):]
	}
	model, err := aom14.ReadOperationalTemplate(strings.NewReader(opt))
	if err != nil {
		t.Fatalf("ReadOperationalTemplate: %v", err)
	}
	wt, compiled := compileTemplate(t, id, model)

	var leaf *webtemplate.Node
	var find func(n *webtemplate.Node)
	find = func(n *webtemplate.Node) {
		if strings.HasPrefix(n.RMType, "DV_INTERVAL") {
			leaf = n
		}
		for _, ch := range n.Children {
			find(ch)
		}
	}
	find(wt.Tree)
	if leaf == nil {
		t.Fatal("the Web Template has no DV_INTERVAL leaf")
	}
	key, ok := flatKeyOf(wt.Tree, leaf.AQLPath, nil)
	if !ok {
		t.Fatalf("no FLAT key for %s", leaf.AQLPath)
	}
	body, err := json.Marshal(map[string]any{
		"ctx/language": "en", "ctx/territory": "NL", "ctx/composer_name": "Composer",
		"ctx/time":          "2026-10-26T09:00:00Z",
		key + "/lower|code": "111", key + "/lower|value": "mild", key + "/lower|ordinal": 1,
		key + "/upper|raw": map[string]any{
			"_type": "DV_ORDINAL", "value": 1,
			"symbol": map[string]any{
				"_type": "DV_CODED_TEXT", "value": "mild",
				"defining_code": map[string]any{
					"_type": "CODE_PHRASE", "code_string": "111",
					"terminology_id": map[string]any{"_type": "TERMINOLOGY_ID", "value": "local"},
				},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	// terminologyAt returns the terminology of the decoded bound's symbol.
	terminologyAt := func(t *testing.T, comp *rm.Composition, end string) string {
		t.Helper()
		items, err := rmpath.ItemsAtPath(comp, leaf.AQLPath)
		if err != nil || len(items) != 1 {
			t.Fatalf("ItemsAtPath(%s) = %d items, %v; want the interval", leaf.AQLPath, len(items), err)
		}
		iv, ok := items[0].(*rm.DVInterval[rm.DVOrdered])
		if !ok {
			t.Fatalf("%s holds a %T, want *rm.DVInterval[rm.DVOrdered]", leaf.AQLPath, items[0])
		}
		bound := iv.Lower
		if end == "upper" {
			bound = iv.Upper
		}
		ord, ok := bound.(*rm.DVOrdinal)
		if !ok {
			t.Fatalf("%s %s bound is a %T, want *rm.DVOrdinal", leaf.AQLPath, end, bound)
		}
		return ord.Symbol.DefiningCode.TerminologyID.Value
	}

	for _, format := range []string{"FLAT", "STRUCTURED"} {
		t.Run(format, func(t *testing.T) {
			bare := decodeAs(t, format, body, wt)
			if lower := terminologyAt(t, bare, "lower"); lower != "local" {
				t.Errorf("decode without the template: lower bound terminology %q, want local", lower)
			}
			got := decodeAs(t, format, body, wt, simplified.WithTemplate(compiled))
			if lower, upper := terminologyAt(t, got, "lower"), terminologyAt(t, got, "upper"); lower != "SNOMED-CT" || upper != "local" {
				t.Errorf("WithTemplate decode: bound terminologies lower %q, upper %q; want SNOMED-CT, the template's, and local, the |raw bound's own", lower, upper)
			}
		})
	}
}
