package validation_test

// limits_decoded_test.go: REQ-171 § One walk, REQ-112 § Driver — a document
// decoded with canjson.Unmarshal can reach each limit of the walk: a large
// one the visit limit, a deeply nested one the depth limit. The template
// check runs in the same walk as the Reference Model rules, so a max_nodes
// stop ends it too: a template defect past the stop is not reported until
// the caller raises the limit with WithLimits.

import (
	json "encoding/json/v2"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// bmiEventsPath is the path of the events of the BMI OBSERVATION, the
// first content item of the corpus BMI composition.
const bmiEventsPath = "/content[openEHR-EHR-OBSERVATION.body_mass_index.v1]/data/events"

// bmiDocument returns the corpus BMI composition parsed as a JSON tree, for
// a test to edit before it encodes the tree again.
func bmiDocument(t *testing.T) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(fixtures.CompositionJSON("BMI"))
	if err != nil {
		t.Fatalf("read the BMI composition: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse the BMI composition: %v", err)
	}
	return doc
}

// jsonAt returns the value of the JSON tree v at steps, each a member name
// or a list index, failing the test where the tree holds none.
func jsonAt(t *testing.T, v any, steps ...any) any {
	t.Helper()
	for i, step := range steps {
		switch s := step.(type) {
		case string:
			obj, ok := v.(map[string]any)
			if !ok {
				t.Fatalf("JSON at %v is %T, want an object holding %q", steps[:i], v, s)
			}
			if v, ok = obj[s]; !ok {
				t.Fatalf("JSON at %v holds no member %q", steps[:i], s)
			}
		case int:
			list, ok := v.([]any)
			if !ok || s >= len(list) {
				t.Fatalf("JSON at %v is %T, want a list holding index %d", steps[:i], v, s)
			}
			v = list[s]
		default:
			t.Fatalf("jsonAt step %v is %T, want a member name or a list index", step, step)
		}
	}
	return v
}

// objectAt returns the JSON object of v at steps, as jsonAt finds it.
func objectAt(t *testing.T, v any, steps ...any) map[string]any {
	t.Helper()
	at := jsonAt(t, v, steps...)
	obj, ok := at.(map[string]any)
	if !ok {
		t.Fatalf("JSON at %v is %T, want an object", steps, at)
	}
	return obj
}

// decodeDocument encodes doc and decodes the bytes with canjson.Unmarshal,
// as a server decodes a request body, and returns the decode error.
func decodeDocument(t *testing.T, doc map[string]any) (*rm.Composition, error) {
	t.Helper()
	body, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("encode the edited BMI composition: %v", err)
	}
	var comp rm.Composition
	if err := canjson.Unmarshal(body, &comp); err != nil {
		return nil, err
	}
	return &comp, nil
}

// repeatedEvents returns the corpus BMI composition with the one event of
// its BMI OBSERVATION held n times, the last copy giving its quantity the
// units "furlong", which the template does not allow there.
func repeatedEvents(t *testing.T, n int) map[string]any {
	t.Helper()
	doc := bmiDocument(t)
	history := objectAt(t, doc, "content", 0, "data")
	events, ok := history["events"].([]any)
	if !ok || len(events) != 1 {
		t.Fatalf("BMI composition events = %v, want a list of one event", history["events"])
	}
	// The copies share one tree, which encoding only reads; the last is
	// parsed afresh, so its edit reaches no other copy.
	raw, err := json.Marshal(events[0])
	if err != nil {
		t.Fatalf("encode the BMI event: %v", err)
	}
	var last map[string]any
	if err := json.Unmarshal(raw, &last); err != nil {
		t.Fatalf("parse the BMI event: %v", err)
	}
	objectAt(t, last, "data", "items", 0, "value")["units"] = "furlong"
	repeated := slices.Repeat(events, n)
	repeated[n-1] = last
	history["events"] = repeated
	return doc
}

// nestedClusterTree returns an ITEM_TREE whose one item is a CLUSTER nested
// levels deep: each level one CLUSTER holding the next through items, an
// ELEMENT at the bottom.
func nestedClusterTree(levels int) map[string]any {
	item := map[string]any{
		"_type":             "ELEMENT",
		"archetype_node_id": "at0002",
		"name":              map[string]any{"_type": "DV_TEXT", "value": "bottom"},
		"value":             map[string]any{"_type": "DV_BOOLEAN", "value": true},
	}
	for range levels {
		item = map[string]any{
			"_type":             "CLUSTER",
			"archetype_node_id": "at0003",
			"name":              map[string]any{"_type": "DV_TEXT", "value": "level"},
			"items":             []any{item},
		}
	}
	return map[string]any{
		"_type":             "ITEM_TREE",
		"archetype_node_id": "at0001",
		"name":              map[string]any{"_type": "DV_TEXT", "value": "Tree"},
		"items":             []any{item},
	}
}

// REQ-171 § One walk, Acceptance, REQ-112 § Driver — a large decoded
// document reaches the visit limit, and the stop ends the template check
// too. The corpus BMI composition with its one event held 8,000 times, the
// last carrying a unit the template does not allow, is decoded from JSON.
// Under the default limits the walk stops inside the events, before the
// last one: max_nodes once, as the last issue, its path the one entry of
// Coverage.Stopped, and no primitive_unit_unknown. The control: the same
// value under a raised visit limit is walked to the end, so the unit is
// reported and nothing stops the walk. Without it, the missing unit issue
// under the default would prove nothing.
func TestValidator_REQ171_DecodedDocumentReachesTheVisitLimit(t *testing.T) {
	t.Parallel()
	// 8,000 events reach well past the stop. Each event costs the walk about
	// 18 visits, so the default 100,000 stops it near event 5,550, which
	// leaves some 2,400 events after it. The stop stays before the last
	// event while an event costs more than 12.5 visits (100,000 / 8,000);
	// the check on the stop's path below fails when it does not. The stop's
	// own path is not pinned: it moves whenever the visits per event change.
	const n = 8_000
	lastEvent := fmt.Sprintf("%s[%d]", bmiEventsPath, n)
	lastValue := lastEvent + "/data/items[at0004]/value"
	c := mustCompile(t, "BMI")
	comp, err := decodeDocument(t, repeatedEvents(t, n))
	if err != nil {
		t.Fatalf("canjson.Unmarshal(BMI composition, %d events): %v", n, err)
	}

	call := fmt.Sprintf("ValidateComposition(BMI composition, %d events, the last in furlong)", n)
	r := validation.ValidateComposition(comp, c)
	got := pathsOf(r.Issues, "max_nodes")
	if len(got) != 1 {
		t.Fatalf("%s reports max_nodes %d times, want once; issues %q", call, len(got), issueKeys(r.Issues))
	}
	assertStoppedOnce(t, call, r, "max_nodes", got[0])
	if !strings.HasPrefix(got[0], bmiEventsPath+"[") || strings.HasPrefix(got[0], lastEvent+"/") {
		t.Errorf("%s stops at %q, want inside the events before the last, %q: raise n until the default limit falls short of it", call, got[0], lastEvent)
	}
	if last := r.Issues[len(r.Issues)-1]; last.Code != "max_nodes" {
		t.Errorf("%s ends with %s at %q, want max_nodes: the limit ends the walk", call, last.Code, last.Path)
	}
	if units := pathsOf(r.Issues, "primitive_unit_unknown"); len(units) != 0 {
		t.Errorf("%s reports primitive_unit_unknown at %q, want none: the unit sits past the stop", call, units)
	}

	call = fmt.Sprintf("Validate(BMI composition, %d events, the last in furlong) under Nodes 1,000,000", n)
	high := limited(validation.Limits{Nodes: 1_000_000}).Validate(comp, c)
	if units := pathsOf(high.Issues, "primitive_unit_unknown"); !slices.Equal(units, []string{lastValue}) {
		t.Errorf("%s reports primitive_unit_unknown at %q, want once, at %q", call, units, lastValue)
	}
	if stops := pathsOf(high.Issues, "max_nodes"); len(stops) != 0 {
		t.Errorf("%s reports max_nodes at %q, want none: the raised limit walks the document to the end", call, stops)
	}
	if len(high.Coverage.Stopped) != 0 {
		t.Errorf("%s Coverage.Stopped = %q, want empty", call, high.Coverage.Stopped)
	}
}

// REQ-171 § One walk, Acceptance, REQ-112 § Driver — a deeply nested
// decoded document reaches the depth limit. The corpus BMI composition
// whose other_context holds CLUSTERs nested to the default depth is decoded
// from JSON, and the walk reports max_depth once, at the first path where
// it stopped, which Coverage.Stopped lists.
func TestValidator_REQ171_REQ112_DecodedDocumentReachesTheDepthLimit(t *testing.T) {
	t.Parallel()
	// The window is narrow. The decoder refuses JSON nested deeper than 512
	// levels (maxDecodeDepth in openehr/rm/typereg/registry.go), and the walk
	// stops at 256 Reference Model levels. Each CLUSTER here costs two JSON
	// levels, its object and its items list, but one RM level. So the
	// composition decodes with at most 253 nested CLUSTERs, at 6 + 2 × 253 =
	// 512 JSON levels, and reaches the walk's limit from 252: /context and
	// /other_context are RM levels 1 and 2, the CLUSTERs 3 to 254, the
	// bottom ELEMENT 255, and its name 256, where the walk stops. Measured:
	// 251 CLUSTERs decode and stop nowhere, 252 and 253 report max_depth,
	// 254 are refused (514 > 512).
	clusters := defaultDepth - 4
	want := "/context/other_context" + strings.Repeat("/items[at0003]", clusters) + "/items[at0002]/name"
	c := mustCompile(t, "BMI")
	doc := bmiDocument(t)
	objectAt(t, doc, "context")["other_context"] = nestedClusterTree(clusters)
	comp, err := decodeDocument(t, doc)
	if err != nil {
		t.Fatalf("canjson.Unmarshal(BMI composition, %d nested CLUSTERs): %v; the decoder's nesting limit now sits below the walk's depth limit of %d, so no decoded document reaches it and the doc's claim is false", clusters, err, defaultDepth)
	}

	call := fmt.Sprintf("ValidateComposition(BMI composition, %d nested CLUSTERs)", clusters)
	r := validation.ValidateComposition(comp, c)
	assertStoppedOnce(t, call, r, "max_depth", want)
}
