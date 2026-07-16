package simplified_test

// REQ-053 — ctx/ context: composition-level metadata (language, territory,
// composer, time) is carried under the ctx/ prefix (FLAT) / a ctx object
// (STRUCTURED). Language + territory are mandatory on decode.
import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/simplified"
)

func TestContextEncodeAndRoundTrip(t *testing.T) {
	comp, wt := genComposition(t, vitalSignsOPT)

	f1, err := simplified.MarshalFlat(comp, wt)
	if err != nil {
		t.Fatalf("MarshalFlat: %v", err)
	}
	var m1 map[string]any
	if err := json.Unmarshal(f1, &m1); err != nil {
		t.Fatal(err)
	}
	// Mandatory + common context fields must be emitted (instance.Generate sets
	// language=en, territory=NL, composer="Test Composer", start_time set).
	wantCtx := map[string]any{
		"ctx/language":      "en",
		"ctx/territory":     "NL",
		"ctx/composer_name": "Test Composer",
	}
	for k, want := range wantCtx {
		if m1[k] != want {
			t.Errorf("%s = %#v, want %#v", k, m1[k], want)
		}
	}
	if _, ok := m1["ctx/time"]; !ok {
		t.Error("ctx/time missing")
	}

	// Round-trip: decode rebuilds the context, re-encode reproduces the FLAT.
	comp2, err := simplified.UnmarshalFlat(f1, wt)
	if err != nil {
		t.Fatalf("UnmarshalFlat: %v", err)
	}
	f2, err := simplified.MarshalFlat(comp2, wt)
	if err != nil {
		t.Fatalf("MarshalFlat #2: %v", err)
	}
	var m2 map[string]any
	if err := json.Unmarshal(f2, &m2); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"ctx/language", "ctx/territory", "ctx/composer_name", "ctx/time"} {
		if m1[k] != m2[k] {
			t.Errorf("ctx round-trip %s: %#v -> %#v", k, m1[k], m2[k])
		}
	}
}

// TestComposerSelfRoundTrip pins the PARTY_SELF composer branch end-to-end:
// encode emits ctx/composer_self, decode rebuilds PARTY_SELF, and the FLAT
// survives a second round-trip. (The generated fixtures always use
// PARTY_IDENTIFIED, so without this test the branch has zero coverage and the
// WithTemplate default would mask its loss.)
func TestComposerSelfRoundTrip(t *testing.T) {
	comp, wt := genComposition(t, minimalObsOPT)
	comp.Composer = &rm.PartySelf{}

	f1, err := simplified.MarshalFlat(comp, wt)
	if err != nil {
		t.Fatalf("MarshalFlat: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(f1, &m); err != nil {
		t.Fatal(err)
	}
	if m["ctx/composer_self"] != true {
		t.Fatalf("ctx/composer_self = %#v, want true (keys: %v)", m["ctx/composer_self"], m)
	}
	if _, ok := m["ctx/composer_name"]; ok {
		t.Error("ctx/composer_name emitted alongside composer_self")
	}
	comp2, err := simplified.UnmarshalFlat(f1, wt)
	if err != nil {
		t.Fatalf("UnmarshalFlat: %v", err)
	}
	if _, ok := comp2.Composer.(*rm.PartySelf); !ok {
		t.Errorf("decoded composer = %T, want *rm.PartySelf", comp2.Composer)
	}
	f2, err := simplified.MarshalFlat(comp2, wt)
	if err != nil {
		t.Fatalf("MarshalFlat #2: %v", err)
	}
	var m2 map[string]any
	if err := json.Unmarshal(f2, &m2); err != nil {
		t.Fatal(err)
	}
	if m2["ctx/composer_self"] != true {
		t.Errorf("composer_self lost on round-trip: %#v", m2["ctx/composer_self"])
	}
}

func TestDecodeMissingContextErrors(t *testing.T) {
	comp, wt := genComposition(t, vitalSignsOPT)
	f1, err := simplified.MarshalFlat(comp, wt)
	if err != nil {
		t.Fatalf("MarshalFlat: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(f1, &m); err != nil {
		t.Fatal(err)
	}
	// Strip context; the remaining content-only payload must be rejected.
	for k := range m {
		if strings.HasPrefix(k, "ctx/") {
			delete(m, k)
		}
	}
	stripped, _ := json.Marshal(m)
	if _, err := simplified.UnmarshalFlat(stripped, wt); !errors.Is(err, simplified.ErrMissingContext) {
		t.Fatalf("UnmarshalFlat(no ctx) err = %v, want ErrMissingContext", err)
	}
}

// TestContextStructuredShape checks ctx is grouped under a non-arrayified ctx
// object in STRUCTURED, and survives FLAT<->STRUCTURED interconversion.
func TestContextStructuredShape(t *testing.T) {
	comp, wt := genComposition(t, vitalSignsOPT)
	f1, err := simplified.MarshalFlat(comp, wt)
	if err != nil {
		t.Fatalf("MarshalFlat: %v", err)
	}
	s, err := simplified.FlatToStructured(f1)
	if err != nil {
		t.Fatalf("FlatToStructured: %v", err)
	}
	var sm map[string]any
	if err := json.Unmarshal(s, &sm); err != nil {
		t.Fatal(err)
	}
	ctx, ok := sm["ctx"].(map[string]any)
	if !ok {
		t.Fatalf("STRUCTURED has no ctx object: %v", sm["ctx"])
	}
	if ctx["language"] != "en" { // direct value, not an array
		t.Errorf("ctx.language = %#v, want \"en\" (non-arrayified)", ctx["language"])
	}
	back, err := simplified.StructuredToFlat(s)
	if err != nil {
		t.Fatalf("StructuredToFlat: %v", err)
	}
	var bm map[string]any
	if err := json.Unmarshal(back, &bm); err != nil {
		t.Fatal(err)
	}
	if bm["ctx/language"] != "en" || bm["ctx/territory"] != "NL" {
		t.Errorf("ctx lost through interconversion: language=%#v territory=%#v", bm["ctx/language"], bm["ctx/territory"])
	}
}
