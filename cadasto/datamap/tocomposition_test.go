package datamap

import (
	"bytes"
	"os"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/template"
)

// REQ-058 — write path: a datamap payload + its OPT produces a canonical RM
// COMPOSITION, and round-trips back through FromComposition without losing the
// supplied values. Uses bare at-code keys (the lookup falls back from
// "<at-code>|<label>" to bare "<at-code>").

func loadOPT(t *testing.T, name string) *template.OperationalTemplate {
	t.Helper()
	b, err := os.ReadFile("testdata/fixtures/" + name + ".opt")
	if err != nil {
		t.Fatal(err)
	}
	opt, err := template.ParseOPT(bytes.NewReader(b))
	if err != nil {
		t.Fatalf("ParseOPT(%s): %v", name, err)
	}
	return opt
}

func TestToCompositionRoundTrip(t *testing.T) {
	opt := loadOPT(t, "development-1")

	const archetypeID = "openEHR-EHR-OBSERVATION.development.v0"
	dm := map[string]any{
		"language":  "nl",
		"territory": "NL",
		"composer":  "Dr. Jansen",
		"context":   map[string]any{"start_time": "2026-02-01T09:30:00Z"},
		"content": map[string]any{
			archetypeID: map[string]any{ // bare-id key (no |label) — fallback lookup
				"events": []any{
					map[string]any{
						"time":   "2026-02-01T09:30:00Z",
						"at0004": "Patient developing normally", // DV_TEXT
						"at0006": 12.5,                          // DV_QUANTITY (short form)
						"at0008": 3,                             // DV_COUNT
						"at0013": true,                          // DV_BOOLEAN
					},
				},
			},
		},
	}

	comp, err := ToComposition(opt, dm)
	if err != nil {
		t.Fatalf("ToComposition: %v", err)
	}

	// Structural checks on the canonical composition.
	if comp["_type"] != "COMPOSITION" {
		t.Fatalf("_type: got %v", comp["_type"])
	}
	contentList, _ := comp["content"].([]any)
	if len(contentList) != 1 {
		t.Fatalf("content: want 1 entry, got %d", len(contentList))
	}
	entry := contentList[0].(map[string]any)
	if entry["_type"] != "OBSERVATION" || entry["archetype_node_id"] != archetypeID {
		t.Fatalf("entry: got _type=%v node=%v", entry["_type"], entry["archetype_node_id"])
	}

	// Round-trip back to datamap and confirm the values survived.
	dm2, err := FromComposition(nil, comp)
	if err != nil {
		t.Fatalf("FromComposition: %v", err)
	}
	content2 := dm2["content"].(map[string]any)
	var root map[string]any
	for _, v := range content2 {
		root = v.(map[string]any)
	}
	if root == nil {
		t.Fatalf("round-trip content empty")
	}
	events := root["events"].([]any)
	if len(events) != 1 {
		t.Fatalf("round-trip events: want 1, got %d", len(events))
	}
	ev := events[0].(map[string]any)
	if ev["time"] != "2026-02-01T09:30:00Z" {
		t.Errorf("round-trip time: got %v", ev["time"])
	}

	got := collectLeafValues(ev)
	assertContains(t, got, "Patient developing normally") // DV_TEXT
	assertContains(t, got, 12.5)                          // DV_QUANTITY magnitude
	assertContains(t, got, 3)                             // DV_COUNT magnitude (JSON number)
	assertContains(t, got, true)                          // DV_BOOLEAN
}

func TestToCompositionRequiresStartTime(t *testing.T) {
	opt := loadOPT(t, "development-1")
	_, err := ToComposition(opt, map[string]any{"content": map[string]any{}})
	if err == nil {
		t.Error("expected error when context.start_time is missing")
	}
}

// collectLeafValues gathers all scalar leaf values (recursing maps/slices) so
// the round-trip assertion is robust to the exact key labels.
func collectLeafValues(v any) []any {
	var out []any
	switch t := v.(type) {
	case map[string]any:
		for _, vv := range t {
			out = append(out, collectLeafValues(vv)...)
		}
	case []any:
		for _, vv := range t {
			out = append(out, collectLeafValues(vv)...)
		}
	default:
		out = append(out, v)
	}
	return out
}

func assertContains(t *testing.T, haystack []any, want any) {
	t.Helper()
	for _, v := range haystack {
		if v == want {
			return
		}
	}
	t.Errorf("round-trip lost value %v (got leaves %v)", want, haystack)
}
