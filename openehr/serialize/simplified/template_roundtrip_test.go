package simplified_test

// REQ-053 — a FLAT or STRUCTURED body decoded with WithTemplate validates
// against the OPT it was written from. The formats fold every ITEM_STRUCTURE
// away, so decode takes the folded structure's concrete subtype from the
// compiled template; FLAT carries order only among the instances of one
// node, so decode places the members of one container list in the Web
// Template's node order; and the formats have no key for a structure with no
// member, so decode rebuilds, empty, one the template requires and lets hold
// no member.

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/cadasto/openehr-sdk-go/openehr/composition"
	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/simplified"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/template/webtemplate"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// templateOf builds the Web Template and the compiled template of the corpus
// OPT with the template id id.
func templateOf(t *testing.T, id string) (*webtemplate.WebTemplate, *templatecompile.Compiled) {
	t.Helper()
	opt, err := template.ParseFile(fixtures.TemplateOpt(id))
	if err != nil {
		t.Fatalf("ParseFile(%s): %v", id, err)
	}
	return compileTemplate(t, id, opt)
}

// compileTemplate compiles opt, the template named id, and builds its Web
// Template.
func compileTemplate(t *testing.T, id string, opt *template.OperationalTemplate) (*webtemplate.WebTemplate, *templatecompile.Compiled) {
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

// writtenEmptyStructures returns the structure the skeleton comp writes at
// each of paths, each of which must hold no member.
func writtenEmptyStructures(t *testing.T, comp *rm.Composition, paths []string) map[string]map[string]any {
	t.Helper()
	doc := canonicalDoc(t, comp)
	written := make(map[string]map[string]any, len(paths))
	for _, p := range paths {
		s, ok := nodeAt(doc, p)
		if !ok {
			t.Fatalf("the written skeleton has no structure at %s — fixture changed?", p)
		}
		if _, has := s["items"]; has {
			t.Fatalf("the written structure at %s holds items — fixture changed?", p)
		}
		written[p] = s
	}
	return written
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
