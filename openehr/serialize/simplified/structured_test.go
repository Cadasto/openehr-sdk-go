package simplified_test

// REQ-053 — STRUCTURED format and FLAT<->STRUCTURED interconversion (no OPT).
import (
	"encoding/json"
	"reflect"
	"strings"
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

// TestUnderscoreKeysThroughStructured — REQ-140. The underscore RM attribute
// grammar must survive OPT-free FLAT ↔ STRUCTURED interconversion in both
// directions. It needs no special handling in `structured.go`: an `_`-prefixed
// path segment restructures like any other segment, so `_link:N` becomes an
// array member and a family's suffixed keys become `|`-prefixed members of the
// element object. This test pins that shape — and the one asymmetry it creates,
// which the decoder has to tolerate.
func TestUnderscoreKeysThroughStructured(t *testing.T) {
	flat := map[string]any{
		"t/_uid":                   "6e3a9506-b81c-4d74-a37f-1464fb7106b2::ehrbase.org::1",
		"t/_link:0|meaning":        "problem related note",
		"t/_link:0|type":           "problem",
		"t/_link:0|target":         "ehr://ehr.network/1",
		"t/_link:1|meaning":        "follow-up to",
		"t/_link:1|type":           "issue",
		"t/_link:1|target":         "ehr://ehr.network/2",
		"t/context/_end_time":      "2021-12-21T15:19:31.649613+01:00",
		"t/context/_location":      "microbiology lab 2",
		"t/obs/_work_flow_id|id":   "335645",
		"t/obs/_work_flow_id|type": "WORKFLOW",
		"t/e/_uid":                 "9fcc1c70-9349-444d-b9cb-8fa817697f5e",
	}
	sb, err := simplified.FlatToStructured(mustJSON(t, flat))
	if err != nil {
		t.Fatalf("FlatToStructured: %v", err)
	}
	var s map[string]any
	if err := json.Unmarshal(sb, &s); err != nil {
		t.Fatalf("unmarshal structured: %v", err)
	}
	root, ok := s["t"].(map[string]any)
	if !ok {
		t.Fatalf("no root object; got %#v", s)
	}
	// A scalar family is a one-element array of its bare value.
	uid, ok := root["_uid"].([]any)
	if !ok || len(uid) != 1 || uid[0] != flat["t/_uid"] {
		t.Errorf("_uid = %#v, want a 1-element array of the bare value", root["_uid"])
	}
	// An indexed family is an array member per instance, suffixes as
	// |-prefixed members.
	links, ok := root["_link"].([]any)
	if !ok || len(links) != 2 {
		t.Fatalf("_link = %#v, want a 2-element array", root["_link"])
	}
	first, ok := links[0].(map[string]any)
	if !ok || first["|meaning"] != "problem related note" || first["|target"] != "ehr://ehr.network/1" {
		t.Errorf("_link[0] = %#v", links[0])
	}
	// A family under a nested segment nests with it.
	ctx, ok := root["context"].([]any)
	if !ok || len(ctx) != 1 {
		t.Fatalf("context = %#v, want a 1-element array", root["context"])
	}
	ctxEl, _ := ctx[0].(map[string]any)
	if end, ok := ctxEl["_end_time"].([]any); !ok || len(end) != 1 || end[0] != flat["t/context/_end_time"] {
		t.Errorf("context[0]._end_time = %#v", ctxEl["_end_time"])
	}

	// Back to FLAT: every key returns, with the interconversion's usual
	// index normalisation — it re-spells *every* segment with an explicit
	// `:index`, so `t/_uid` comes back as `t/_uid:0` and `t/context/…` as
	// `t/context:0/…`. That is pre-existing behaviour for clinical segments and
	// the FLAT decoder treats `:0` and no index as one slot, which is why the
	// REQ-140 router admits `:0` on a single-valued family.
	fb, err := simplified.StructuredToFlat(sb)
	if err != nil {
		t.Fatalf("StructuredToFlat: %v", err)
	}
	var back map[string]any
	if err := json.Unmarshal(fb, &back); err != nil {
		t.Fatalf("unmarshal flat: %v", err)
	}
	want := map[string]any{
		"t/_uid:0":                     flat["t/_uid"],
		"t/_link:0|meaning":            flat["t/_link:0|meaning"],
		"t/_link:0|type":               flat["t/_link:0|type"],
		"t/_link:0|target":             flat["t/_link:0|target"],
		"t/_link:1|meaning":            flat["t/_link:1|meaning"],
		"t/_link:1|type":               flat["t/_link:1|type"],
		"t/_link:1|target":             flat["t/_link:1|target"],
		"t/context:0/_end_time:0":      flat["t/context/_end_time"],
		"t/context:0/_location:0":      flat["t/context/_location"],
		"t/obs:0/_work_flow_id:0|id":   flat["t/obs/_work_flow_id|id"],
		"t/obs:0/_work_flow_id:0|type": flat["t/obs/_work_flow_id|type"],
		"t/e:0/_uid:0":                 flat["t/e/_uid"],
	}
	if !reflect.DeepEqual(back, want) {
		t.Errorf("FLAT -> STRUCTURED -> FLAT mismatch:\n got  %#v\n want %#v", back, want)
	}

	// STRUCTURED -> FLAT -> STRUCTURED is the identity leg, and it holds for the
	// underscore grammar too.
	sb2, err := simplified.FlatToStructured(fb)
	if err != nil {
		t.Fatalf("FlatToStructured #2: %v", err)
	}
	var s2 map[string]any
	if err := json.Unmarshal(sb2, &s2); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(s2, s) {
		t.Errorf("STRUCTURED round-trip mismatch:\n got  %#v\n want %#v", s2, s)
	}
}

// TestValueDecorationsThroughStructured — REQ-140. The value-decoration
// families are the first with a *sub-path* inside the family (`/lower`,
// `/meaning`), so STRUCTURED nests one level deeper than C0's families did: the
// family is an array member and its sub-object is an array under that. Both
// directions are pinned OPT-free, including the interconversion's index
// normalisation — which now reaches the sub-path segment too (`/lower:0`), the
// spelling the decoder folds back onto the index-less one.
func TestValueDecorationsThroughStructured(t *testing.T) {
	flat := map[string]any{
		"t/e/_normal_range/lower|magnitude":                 20.5,
		"t/e/_normal_range/lower|unit":                      "unit",
		"t/e/_normal_range|lower_included":                  false,
		"t/e/_other_reference_ranges:0/lower|magnitude":     70.5,
		"t/e/_other_reference_ranges:0/meaning|code":        "260360000",
		"t/e/_other_reference_ranges:0/meaning|terminology": "SNOMED-CT",
		"t/e/_other_reference_ranges:0/meaning|value":       "very high",
		"t/e/_other_reference_ranges:0|upper_unbounded":     true,
		"t/e/_other_reference_ranges:1/meaning":             "high",
	}
	sb, err := simplified.FlatToStructured(mustJSON(t, flat))
	if err != nil {
		t.Fatalf("FlatToStructured: %v", err)
	}
	var s map[string]any
	if err := json.Unmarshal(sb, &s); err != nil {
		t.Fatalf("unmarshal structured: %v", err)
	}
	root, _ := s["t"].(map[string]any)
	el, _ := root["e"].([]any)
	if len(el) != 1 {
		t.Fatalf("e = %#v, want a 1-element array", root["e"])
	}
	leaf, _ := el[0].(map[string]any)
	// The scalar family is a one-element array; its bound is a nested array, and
	// its own boolean a |-prefixed member beside it.
	nr, ok := leaf["_normal_range"].([]any)
	if !ok || len(nr) != 1 {
		t.Fatalf("_normal_range = %#v, want a 1-element array", leaf["_normal_range"])
	}
	nrEl, _ := nr[0].(map[string]any)
	if nrEl["|lower_included"] != false {
		t.Errorf("_normal_range[0].|lower_included = %#v", nrEl["|lower_included"])
	}
	lower, ok := nrEl["lower"].([]any)
	if !ok || len(lower) != 1 {
		t.Fatalf("_normal_range[0].lower = %#v, want a 1-element array", nrEl["lower"])
	}
	if bound, _ := lower[0].(map[string]any); bound["|magnitude"] != 20.5 {
		t.Errorf("_normal_range[0].lower[0] = %#v", lower[0])
	}
	// The indexed family is an array member per instance.
	orr, ok := leaf["_other_reference_ranges"].([]any)
	if !ok || len(orr) != 2 {
		t.Fatalf("_other_reference_ranges = %#v, want a 2-element array", leaf["_other_reference_ranges"])
	}
	second, _ := orr[1].(map[string]any)
	if m, _ := second["meaning"].([]any); len(m) != 1 || m[0] != "high" {
		t.Errorf("_other_reference_ranges[1].meaning = %#v, want [\"high\"]", second["meaning"])
	}

	// Back to FLAT: every key returns, every segment re-spelled with an explicit
	// `:index` — the sub-path segments included.
	fb, err := simplified.StructuredToFlat(sb)
	if err != nil {
		t.Fatalf("StructuredToFlat: %v", err)
	}
	var back map[string]any
	if err := json.Unmarshal(fb, &back); err != nil {
		t.Fatalf("unmarshal flat: %v", err)
	}
	want := map[string]any{
		"t/e:0/_normal_range:0/lower:0|magnitude":               20.5,
		"t/e:0/_normal_range:0/lower:0|unit":                    "unit",
		"t/e:0/_normal_range:0|lower_included":                  false,
		"t/e:0/_other_reference_ranges:0/lower:0|magnitude":     70.5,
		"t/e:0/_other_reference_ranges:0/meaning:0|code":        "260360000",
		"t/e:0/_other_reference_ranges:0/meaning:0|terminology": "SNOMED-CT",
		"t/e:0/_other_reference_ranges:0/meaning:0|value":       "very high",
		"t/e:0/_other_reference_ranges:0|upper_unbounded":       true,
		"t/e:0/_other_reference_ranges:1/meaning:0":             "high",
	}
	if !reflect.DeepEqual(back, want) {
		t.Errorf("FLAT -> STRUCTURED -> FLAT mismatch:\n got  %#v\n want %#v", back, want)
	}

	// …and the identity leg holds from the STRUCTURED side.
	sb2, err := simplified.FlatToStructured(fb)
	if err != nil {
		t.Fatalf("FlatToStructured #2: %v", err)
	}
	var s2 map[string]any
	if err := json.Unmarshal(sb2, &s2); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(s2, s) {
		t.Errorf("STRUCTURED round-trip mismatch:\n got  %#v\n want %#v", s2, s)
	}
}

// TestUnderscoreKeysStructuredDecode — REQ-140. The index normalisation the
// interconversion applies must not break the codec: a STRUCTURED body carrying
// underscore families decodes through UnmarshalStructured to the same
// composition the FLAT spelling gives.
func TestUnderscoreKeysStructuredDecode(t *testing.T) {
	comp, wt := genComposition(t, minimalObsOPT)
	flat, err := simplified.MarshalFlat(comp, wt)
	if err != nil {
		t.Fatalf("MarshalFlat: %v", err)
	}
	var flatMap map[string]any
	if err := json.Unmarshal(flat, &flatMap); err != nil {
		t.Fatal(err)
	}
	// The fixture generator stamps a `uid` on every archetyped node, so the
	// baseline FLAT already carries the family under test.
	var sawUID bool
	for k := range flatMap {
		sawUID = sawUID || strings.HasSuffix(k, "/_uid")
	}
	if !sawUID {
		t.Fatal("fixture carries no `_uid` key — nothing under test")
	}
	structured, err := simplified.FlatToStructured(flat)
	if err != nil {
		t.Fatalf("FlatToStructured: %v", err)
	}
	back, err := simplified.UnmarshalStructured(structured, wt)
	if err != nil {
		t.Fatalf("UnmarshalStructured: %v", err)
	}
	again, err := simplified.MarshalFlat(back, wt)
	if err != nil {
		t.Fatalf("MarshalFlat #2: %v", err)
	}
	var againMap map[string]any
	if err := json.Unmarshal(again, &againMap); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(againMap, flatMap) {
		t.Errorf("FLAT -> STRUCTURED -> RM -> FLAT is not byte-idempotent:\n got  %#v\n want %#v", againMap, flatMap)
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
