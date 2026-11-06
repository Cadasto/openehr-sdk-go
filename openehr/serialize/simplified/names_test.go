package simplified_test

// REQ-053 — LOCATABLE.name repopulation on decode via WithTemplate: the decoded
// composition must be RM/OPT-conformant (every mandatory name present), not just
// format-idempotent.
import (
	"bytes"
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/rmpath"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/simplified"
	"github.com/cadasto/openehr-sdk-go/openehr/template/webtemplate"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

func nameRequiredIssues(r validation.Result) []string {
	var out []string
	for _, is := range r.Issues {
		if is.Code == "required" && strings.HasSuffix(is.Path, "/name") {
			out = append(out, is.Path)
		}
	}
	return out
}

func TestDecodeWithTemplateNamesAreConformant(t *testing.T) {
	const id = "Test_dv_quantity_open_constraint.v0"
	optB, err := os.ReadFile(fixtures.TemplateOpt(id))
	if err != nil {
		t.Fatal(err)
	}
	opt, err := fixtures.ReadOperationalTemplate(optB)
	if err != nil {
		t.Fatal(err)
	}
	compiled, err := templatecompile.Compile(opt)
	if err != nil {
		t.Fatal(err)
	}
	wt, err := webtemplate.Build(compiled)
	if err != nil {
		t.Fatal(err)
	}
	compB, err := os.ReadFile(fixtures.CompositionJSON(id))
	if err != nil {
		t.Fatal(err)
	}
	var comp rm.Composition
	if err := canjson.Unmarshal(compB, &comp); err != nil {
		t.Fatal(err)
	}
	flat, err := simplified.MarshalFlat(&comp, wt)
	if err != nil {
		t.Fatalf("MarshalFlat: %v", err)
	}

	// Bare decode: names are absent -> the RM-mandatory name surfaces as required.
	bare, err := simplified.UnmarshalFlat(flat, wt)
	if err != nil {
		t.Fatalf("UnmarshalFlat (bare): %v", err)
	}
	if got := nameRequiredIssues(validation.Validate(bare, compiled)); len(got) == 0 {
		t.Error("bare decode: expected missing-name issues (the gap WithTemplate closes), got none")
	}

	// WithTemplate: names repopulated AND the RM-mandatory attributes the formats
	// omit are completed from ctx defaults -> the decoded composition validates
	// against the OPT.
	named, err := simplified.UnmarshalFlat(flat, wt, simplified.WithTemplate(compiled))
	if err != nil {
		t.Fatalf("UnmarshalFlat (WithTemplate): %v", err)
	}
	if r := validation.Validate(named, compiled); !r.OK {
		t.Errorf("WithTemplate decode does not validate against the OPT; issues=%v", r.Issues)
	}

	// The option must pass through the STRUCTURED entry point too — deleting
	// the opts... delegation would regress STRUCTURED conformance with every
	// FLAT-only assertion still green.
	s, err := simplified.MarshalStructured(&comp, wt)
	if err != nil {
		t.Fatalf("MarshalStructured: %v", err)
	}
	namedS, err := simplified.UnmarshalStructured(s, wt, simplified.WithTemplate(compiled))
	if err != nil {
		t.Fatalf("UnmarshalStructured (WithTemplate): %v", err)
	}
	if r := validation.Validate(namedS, compiled); !r.OK {
		t.Errorf("WithTemplate STRUCTURED decode does not validate; issues=%v", r.Issues)
	}

	// Documented boundary (deviations.md): without ctx/time the mandatory
	// EVENT.time / HISTORY.origin have no source — the WithTemplate decode
	// still succeeds but must NOT claim OPT-validity. Pin it so a change in
	// either direction (erroring, or synthesising a timestamp) is deliberate.
	var noTime map[string]any
	if err := json.Unmarshal(flat, &noTime); err != nil {
		t.Fatal(err)
	}
	delete(noTime, "ctx/time")
	noTimeFlat, _ := json.Marshal(noTime)
	partial, err := simplified.UnmarshalFlat(noTimeFlat, wt, simplified.WithTemplate(compiled))
	if err != nil {
		t.Fatalf("UnmarshalFlat (WithTemplate, no ctx/time): %v", err)
	}
	if r := validation.Validate(partial, compiled); r.OK {
		t.Error("WithTemplate decode without ctx/time validated — the documented boundary moved; update deviations.md")
	}

	// Names must not leak into FLAT — the round-trip stays idempotent.
	flat2, err := simplified.MarshalFlat(named, wt)
	if err != nil {
		t.Fatalf("MarshalFlat #2: %v", err)
	}
	var m1, m2 map[string]any
	_ = json.Unmarshal(flat, &m1)
	_ = json.Unmarshal(flat2, &m2)
	if len(m1) != len(m2) {
		t.Errorf("naming changed the FLAT key set: %d -> %d", len(m1), len(m2))
	}
}

// dropRootTerm returns opt with the term definition for code removed from the
// archetype root archetypeID: the first after that root's archetype_id, which
// an OPT writes after every root nested in it and before its own terms. It
// fails the test unless the OPT names that root once and the term found is the
// root's own.
func dropRootTerm(t *testing.T, opt []byte, archetypeID, code string) []byte {
	t.Helper()
	id := []byte("<value>" + archetypeID + "</value>")
	if n := bytes.Count(opt, id); n != 1 {
		t.Fatalf("the OPT names %s %d times, want once", archetypeID, n)
	}
	at := bytes.Index(opt, id)
	start := bytes.Index(opt[at:], []byte(`<term_definitions code="`+code+`">`))
	if start < 0 || bytes.Contains(opt[at:at+start], []byte("<archetype_id>")) {
		t.Fatalf("%s defines no term %s of its own", archetypeID, code)
	}
	start += at
	end := bytes.Index(opt[start:], []byte("</term_definitions>"))
	if end < 0 {
		t.Fatalf("the term %s of %s is not closed", code, archetypeID)
	}
	return slices.Concat(opt[:start], opt[start+end+len("</term_definitions>"):])
}

// flatKeyOf returns the FLAT key of the Web Template node at aqlPath below n,
// whose ancestors' ids are ids: each id joined by "/", with ":0" on every
// segment below the root whose node repeats.
func flatKeyOf(n *webtemplate.Node, aqlPath string, ids []string) (string, bool) {
	seg := n.ID
	if ids != nil && n.Max != 1 {
		seg += ":0"
	}
	ids = append(slices.Clip(ids), seg)
	if n.AQLPath == aqlPath {
		return strings.Join(ids, "/"), true
	}
	for _, ch := range n.Children {
		if key, ok := flatKeyOf(ch, aqlPath, ids); ok {
			return key, true
		}
	}
	return "", false
}

// TestDecodeWithTemplateNameFromTheNearestArchetypeRoot — REQ-053. A
// WithTemplate decode names a node from the term its nearest enclosing
// archetype root defines for its node id, as the generator and the Web
// Template do. Where that archetype does not define the node id, the term an
// enclosing archetype defines under the same code names another concept, and
// the node gets no name from the template.
func TestDecodeWithTemplateNameFromTheNearestArchetypeRoot(t *testing.T) {
	raw, err := os.ReadFile("../../../testkit/corpus/webtemplate/Corona_Anamnese.opt")
	if err != nil {
		t.Fatal(err)
	}
	// The COMPOSITION defines at0002 as "Bericht ID"; story.v1 no longer does.
	raw = dropRootTerm(t, raw, "openEHR-EHR-OBSERVATION.story.v1", "at0002")
	opt, err := fixtures.ReadOperationalTemplate(raw)
	if err != nil {
		t.Fatal(err)
	}
	compiled, err := templatecompile.Compile(opt)
	if err != nil {
		t.Fatal(err)
	}
	wt, err := webtemplate.Build(compiled)
	if err != nil {
		t.Fatal(err)
	}
	// One value under the story event is enough for decode to rebuild the
	// event. MarshalFlat cannot write this template (its reused SECTION
	// siblings), so the key is composed from the Web Template's ids.
	const storyPath = "/content[openEHR-EHR-OBSERVATION.story.v1]/data[at0001]/events[at0002]/data[at0003]/items[at0004]/value"
	key, ok := flatKeyOf(wt.Tree, storyPath, nil)
	if !ok {
		t.Fatalf("the Web Template has no node at %s", storyPath)
	}
	flat, err := json.Marshal(map[string]any{
		"ctx/language": "de", "ctx/territory": "DE", "ctx/composer_name": "Dr. Test",
		"ctx/time": "2026-01-01T00:00:00Z", key: "Husten seit drei Tagen",
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := simplified.UnmarshalFlat(flat, wt, simplified.WithTemplate(compiled))
	if err != nil {
		t.Fatalf("UnmarshalFlat (WithTemplate): %v", err)
	}
	const eventPath = "/content[openEHR-EHR-OBSERVATION.story.v1]/data/events[at0002]"
	items, err := rmpath.ItemsAtPath(got, eventPath)
	if err != nil || len(items) != 1 {
		t.Fatalf("ItemsAtPath(%s) = %d items, %v; want the decoded event", eventPath, len(items), err)
	}
	event, ok := items[0].(rm.Locatable)
	if !ok {
		t.Fatalf("%s holds a %T, not a LOCATABLE", eventPath, items[0])
	}
	if name := event.GetName(); name != nil && !rm.IsTypedNil(name) {
		t.Errorf("decoded event %s name = %q, want none from the template", eventPath, name.GetValue())
	}
}
