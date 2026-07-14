package simplified_test

// REQ-053 — STRUCTURED format and FLAT<->STRUCTURED interconversion (no OPT).
import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/serialize/simplified"
)

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

// TestFlatToStructuredShape: a suffixed leaf becomes an array of one object
// keyed by |suffix; a bare leaf becomes an array of one scalar.
func TestFlatToStructuredShape(t *testing.T) {
	flat := map[string]any{
		"vs/systolic|magnitude": float64(120),
		"vs/note":               "hi",
	}
	sb, err := simplified.FlatToStructured(mustJSON(t, flat))
	if err != nil {
		t.Fatalf("FlatToStructured: %v", err)
	}
	var s map[string]any
	if err := json.Unmarshal(sb, &s); err != nil {
		t.Fatalf("unmarshal structured: %v", err)
	}
	vs, ok := s["vs"].(map[string]any)
	if !ok {
		t.Fatalf("root object missing; got %#v", s)
	}
	arr, ok := vs["systolic"].([]any)
	if !ok || len(arr) != 1 {
		t.Fatalf("systolic = %#v, want 1-element array", vs["systolic"])
	}
	el, ok := arr[0].(map[string]any)
	if !ok || el["|magnitude"] != float64(120) {
		t.Errorf("systolic[0] = %#v, want {|magnitude:120}", arr[0])
	}
	note, ok := vs["note"].([]any)
	if !ok || len(note) != 1 || note[0] != "hi" {
		t.Errorf("note = %#v, want [\"hi\"]", vs["note"])
	}
}

// TestStructuredFlatRoundTrip: structured -> flat -> structured is identity.
func TestStructuredFlatRoundTrip(t *testing.T) {
	structured := map[string]any{
		"vs": map[string]any{
			"systolic": []any{map[string]any{"|magnitude": float64(120), "|unit": "mm[Hg]"}},
			"time":     []any{"2026-01-01T00:00:00"},
			"bp": []any{
				map[string]any{"sys": []any{map[string]any{"|magnitude": float64(120)}}},
				map[string]any{"sys": []any{map[string]any{"|magnitude": float64(130)}}},
			},
		},
	}
	fb, err := simplified.StructuredToFlat(mustJSON(t, structured))
	if err != nil {
		t.Fatalf("StructuredToFlat: %v", err)
	}
	sb, err := simplified.FlatToStructured(fb)
	if err != nil {
		t.Fatalf("FlatToStructured: %v", err)
	}
	var back map[string]any
	if err := json.Unmarshal(sb, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !reflect.DeepEqual(back, structured) {
		t.Errorf("round-trip mismatch:\n got  %#v\n want %#v", back, structured)
	}
}

// TestMarshalStructuredMinimalObs: STRUCTURED encode of a real composition
// produces a single root object keyed by the template id, with the
// observation present as an array.
func TestMarshalStructuredMinimalObs(t *testing.T) {
	comp, wt := genComposition(t, minimalObsOPT)
	data, err := simplified.MarshalStructured(comp, wt)
	if err != nil {
		t.Fatalf("MarshalStructured: %v", err)
	}
	var s map[string]any
	if err := json.Unmarshal(data, &s); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	root, ok := s[wt.Tree.ID].(map[string]any)
	if !ok {
		t.Fatalf("no root object for %q; keys=%v", wt.Tree.ID, s)
	}
	if _, ok := root["minimal"].([]any); !ok {
		t.Errorf("observation 'minimal' not an array; root=%#v", root)
	}
}
