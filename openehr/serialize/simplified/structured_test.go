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

// TestMappingsThroughStructured — REQ-140. `_mapping:N` is an indexed family
// *with* sub-objects, so STRUCTURED carries it as an array whose members nest
// `target` and `purpose` arrays. OPT-free, both directions.
func TestMappingsThroughStructured(t *testing.T) {
	flat := map[string]any{
		"t/e|code":                           "at0022",
		"t/e|value":                          "value3",
		"t/e/_mapping:0|match":               "=",
		"t/e/_mapping:0/target|code":         "21794005",
		"t/e/_mapping:0/target|terminology":  "SNOMED-CT",
		"t/e/_mapping:0/purpose|code":        "671",
		"t/e/_mapping:0/purpose|terminology": "openehr",
		"t/e/_mapping:0/purpose|value":       "research study",
		"t/e/_mapping:1|match":               "=",
		"t/e/_mapping:1/target|code":         "W.11.7",
		"t/e/_mapping:1/target|terminology":  "RTX",
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
	leaf, ok := el[0].(map[string]any)
	if !ok {
		t.Fatalf("e[0] = %#v, want an object (the family lives on it)", el[0])
	}
	maps, ok := leaf["_mapping"].([]any)
	if !ok || len(maps) != 2 {
		t.Fatalf("_mapping = %#v, want a 2-element array", leaf["_mapping"])
	}
	first, _ := maps[0].(map[string]any)
	if first["|match"] != "=" {
		t.Errorf("_mapping[0].|match = %#v", first["|match"])
	}
	target, ok := first["target"].([]any)
	if !ok || len(target) != 1 {
		t.Fatalf("_mapping[0].target = %#v, want a 1-element array", first["target"])
	}
	if tgt, _ := target[0].(map[string]any); tgt["|code"] != "21794005" {
		t.Errorf("_mapping[0].target[0] = %#v", target[0])
	}
	if second, _ := maps[1].(map[string]any); second["purpose"] != nil {
		t.Errorf("_mapping[1] invented a purpose: %#v", maps[1])
	}

	fb, err := simplified.StructuredToFlat(sb)
	if err != nil {
		t.Fatalf("StructuredToFlat: %v", err)
	}
	var back map[string]any
	if err := json.Unmarshal(fb, &back); err != nil {
		t.Fatalf("unmarshal flat: %v", err)
	}
	want := map[string]any{
		"t/e:0|code":                             "at0022",
		"t/e:0|value":                            "value3",
		"t/e:0/_mapping:0|match":                 "=",
		"t/e:0/_mapping:0/target:0|code":         "21794005",
		"t/e:0/_mapping:0/target:0|terminology":  "SNOMED-CT",
		"t/e:0/_mapping:0/purpose:0|code":        "671",
		"t/e:0/_mapping:0/purpose:0|terminology": "openehr",
		"t/e:0/_mapping:0/purpose:0|value":       "research study",
		"t/e:0/_mapping:1|match":                 "=",
		"t/e:0/_mapping:1/target:0|code":         "W.11.7",
		"t/e:0/_mapping:1/target:0|terminology":  "RTX",
	}
	if !reflect.DeepEqual(back, want) {
		t.Errorf("FLAT -> STRUCTURED -> FLAT mismatch:\n got  %#v\n want %#v", back, want)
	}
}

// TestBareLeafBesideMembersThroughStructured — REQ-053 / REQ-140. STRUCTURED
// gives one array element per FLAT segment, and that element is *either* the bare
// leaf value or an object holding the segment's members. A leaf spelled bare
// (`t/e`) that also carries a `|suffix`, an underscore family (`t/e/_uid`,
// `t/e/_mapping:0|match`) or a sub-path needs both at once, and until REQ-140
// Phase C3 the conversion refused it — a residual reachable from real corpus
// bodies (`…/dv_count` = 7 beside `…/dv_count/_normal_range/lower`) that predates
// the underscore families, since C0's `_uid` collided identically.
//
// The bare value now takes the `"|"` member on that object, which is the
// `"|"+suffix` convention with the empty suffix the FLAT key itself spells and is
// therefore reversible without an OPT. Round-tripped in both directions here for
// every shape that forced the collision.
func TestBareLeafBesideMembersThroughStructured(t *testing.T) {
	for name, flat := range map[string]map[string]any{
		// The scalar families carry the interconversion's explicit `:0` (every
		// segment is re-spelled with one — the equivalence the `_` router honours).
		"C1 _mapping":      {"t/e:0": "DV_TEXT value", "t/e:0/_mapping:0|match": "="},
		"C0 _uid":          {"t/e:0": "DV_TEXT value", "t/e:0/_uid:0": "9fcc1c70-9349-444d-b9cb-8fa817697f5e"},
		"optional suffix":  {"t/e:0": "DV_TEXT value", "t/e:0|formatting": "markdown"},
		"C3 DV_MULTIMEDIA": {"t/e:0": "http://med.tube.com/sample", "t/e:0|mediatype": "video/H261", "t/e:0|size": float64(504903212)},
		"C3 _accuracy":     {"t/e:0": "2022-01-12", "t/e:0/_accuracy:0": "P2D"},
	} {
		t.Run(name, func(t *testing.T) {
			s, err := simplified.FlatToStructured(mustJSON(t, flat))
			if err != nil {
				t.Fatalf("FlatToStructured: %v", err)
			}
			back, err := simplified.StructuredToFlat(s)
			if err != nil {
				t.Fatalf("StructuredToFlat: %v", err)
			}
			var got map[string]any
			if err := json.Unmarshal(back, &got); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, flat) {
				t.Errorf("FLAT -> STRUCTURED -> FLAT mismatch:\n got  %#v\n want %#v\n via  %s", got, flat, s)
			}
		})
	}
}

// TestBareStructuredMemberIsTheEmptySuffix — REQ-053. The bare value's STRUCTURED
// member is exactly `"|"`, and the element it sits on is an object only where the
// collision forced one: a leaf carrying nothing but a bare value stays a bare
// scalar, which is what every STRUCTURED body this codec emitted before Phase C3
// looks like.
func TestBareStructuredMemberIsTheEmptySuffix(t *testing.T) {
	plain, err := simplified.FlatToStructured(mustJSON(t, map[string]any{"t/e:0": "v"}))
	if err != nil {
		t.Fatalf("FlatToStructured: %v", err)
	}
	if got := string(plain); got != `{"t":{"e":["v"]}}` {
		t.Errorf("a bare-only leaf = %s, want the scalar spelling", got)
	}
	mixed, err := simplified.FlatToStructured(mustJSON(t, map[string]any{"t/e:0": "v", "t/e:0|formatting": "markdown"}))
	if err != nil {
		t.Fatalf("FlatToStructured: %v", err)
	}
	if got := string(mixed); got != `{"t":{"e":[{"|":"v","|formatting":"markdown"}]}}` {
		t.Errorf("a bare value beside a suffix = %s, want the \"|\" member", got)
	}
}

// TestPartyGrammarThroughStructured — REQ-140. The party families are the first
// to combine all three key shapes in one instance: own suffixes, an *indexed*
// suffix (`|identifiers_id:N` — the reference's inlined identifier spelling), and
// a sub-path object (`/relationship`). None of them needs a STRUCTURED rule of
// its own — an indexed suffix is opaque after the last `|`, so it survives as a
// `|`-prefixed member verbatim — and this pins that, in both directions,
// OPT-free.
func TestPartyGrammarThroughStructured(t *testing.T) {
	flat := map[string]any{
		"t/context/_health_care_facility|id":                   "9091",
		"t/context/_health_care_facility|id_scheme":            "HOSPITAL-NS",
		"t/context/_health_care_facility|id_namespace":         "HOSPITAL-NS",
		"t/context/_health_care_facility|name":                 "Hospital",
		"t/context/_health_care_facility/_identifier:0|id":     "122",
		"t/context/_health_care_facility/_identifier:0|issuer": "issuer",
		"t/context/_participation:0|function":                  "requester",
		"t/context/_participation:0|mode":                      "face-to-face communication",
		"t/context/_participation:0|identifiers_id:0":          "122",
		"t/context/_participation:0|identifiers_issuer:0":      "issuer",
		"t/context/_participation:1|function":                  "performer",
		"t/context/_participation:1/relationship|code":         "10",
		"t/context/_participation:1/relationship|value":        "mother",
	}
	sb, err := simplified.FlatToStructured(mustJSON(t, flat))
	if err != nil {
		t.Fatalf("FlatToStructured: %v", err)
	}
	var s map[string]any
	if err := json.Unmarshal(sb, &s); err != nil {
		t.Fatalf("unmarshal structured: %v", err)
	}
	ctx := s["t"].(map[string]any)["context"].([]any)[0].(map[string]any)
	// The nested `_identifier:N` is an array of its own inside the party object.
	hcf, ok := ctx["_health_care_facility"].([]any)
	if !ok || len(hcf) != 1 {
		t.Fatalf("_health_care_facility = %#v, want a 1-element array", ctx["_health_care_facility"])
	}
	party, _ := hcf[0].(map[string]any)
	if party["|name"] != "Hospital" {
		t.Errorf("_health_care_facility[0].|name = %#v", party["|name"])
	}
	ids, ok := party["_identifier"].([]any)
	if !ok || len(ids) != 1 {
		t.Fatalf("_identifier = %#v, want a 1-element array", party["_identifier"])
	}
	if first, _ := ids[0].(map[string]any); first["|id"] != "122" {
		t.Errorf("_identifier[0] = %#v", ids[0])
	}
	// The participation's *indexed suffix* stays one opaque member key.
	ps, ok := ctx["_participation"].([]any)
	if !ok || len(ps) != 2 {
		t.Fatalf("_participation = %#v, want a 2-element array", ctx["_participation"])
	}
	p0, _ := ps[0].(map[string]any)
	if p0["|identifiers_id:0"] != "122" {
		t.Errorf("_participation[0] = %#v, want the inlined |identifiers_id:0 member", ps[0])
	}
	// A performer's `/relationship` nests as its own array, like any sub-path.
	p1, _ := ps[1].(map[string]any)
	rel, ok := p1["relationship"].([]any)
	if !ok || len(rel) != 1 {
		t.Fatalf("_participation[1].relationship = %#v, want a 1-element array", p1["relationship"])
	}

	// Back to FLAT: every key returns, with the interconversion's usual
	// re-spelling of every *segment* with an explicit `:index` (the suffix is
	// untouched, so `|identifiers_id:0` comes back verbatim).
	fb, err := simplified.StructuredToFlat(sb)
	if err != nil {
		t.Fatalf("StructuredToFlat: %v", err)
	}
	var back map[string]any
	if err := json.Unmarshal(fb, &back); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{}
	for k, v := range flat {
		k = strings.Replace(k, "t/context/", "t/context:0/", 1)
		k = strings.Replace(k, "_health_care_facility|", "_health_care_facility:0|", 1)
		k = strings.Replace(k, "_health_care_facility/", "_health_care_facility:0/", 1)
		k = strings.Replace(k, "/relationship|", "/relationship:0|", 1)
		want[k] = v
	}
	if !reflect.DeepEqual(back, want) {
		t.Errorf("FLAT -> STRUCTURED -> FLAT:\n got  %#v\n want %#v", back, want)
	}
}

// TestSubjectLeafThroughStructured — REQ-053. The ENTRY `subject` leaf is a
// *nested object* in STRUCTURED, not a scalar: its own suffixes are `|`-prefixed
// members and its `/relationship` and `_identifier:N` sub-objects are arrays
// beside them. OPT-free, both directions.
func TestSubjectLeafThroughStructured(t *testing.T) {
	flat := map[string]any{
		"t/obs/subject|id":                 "1234-5678",
		"t/obs/subject|id_namespace":       "EHR.NETWORK",
		"t/obs/subject|name":               "Silvia Blake",
		"t/obs/subject/_identifier:0|id":   "122",
		"t/obs/subject/relationship|code":  "10",
		"t/obs/subject/relationship|value": "mother",
	}
	sb, err := simplified.FlatToStructured(mustJSON(t, flat))
	if err != nil {
		t.Fatalf("FlatToStructured: %v", err)
	}
	var s map[string]any
	if err := json.Unmarshal(sb, &s); err != nil {
		t.Fatalf("unmarshal structured: %v", err)
	}
	obs := s["t"].(map[string]any)["obs"].([]any)[0].(map[string]any)
	subject, ok := obs["subject"].([]any)
	if !ok || len(subject) != 1 {
		t.Fatalf("subject = %#v, want a 1-element array", obs["subject"])
	}
	el, _ := subject[0].(map[string]any)
	if el["|name"] != "Silvia Blake" {
		t.Errorf("subject[0].|name = %#v", el["|name"])
	}
	for _, member := range []string{"_identifier", "relationship"} {
		if arr, ok := el[member].([]any); !ok || len(arr) != 1 {
			t.Errorf("subject[0].%s = %#v, want a 1-element array", member, el[member])
		}
	}

	fb, err := simplified.StructuredToFlat(sb)
	if err != nil {
		t.Fatalf("StructuredToFlat: %v", err)
	}
	var back map[string]any
	if err := json.Unmarshal(fb, &back); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{}
	for k, v := range flat {
		k = strings.Replace(k, "t/obs/", "t/obs:0/", 1)
		k = strings.Replace(k, "subject|", "subject:0|", 1)
		k = strings.Replace(k, "subject/", "subject:0/", 1)
		k = strings.Replace(k, "/relationship|", "/relationship:0|", 1)
		want[k] = v
	}
	if !reflect.DeepEqual(back, want) {
		t.Errorf("FLAT -> STRUCTURED -> FLAT:\n got  %#v\n want %#v", back, want)
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
