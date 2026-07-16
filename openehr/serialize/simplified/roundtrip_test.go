package simplified_test

// REQ-053 — FLAT round-trip: comp -> FLAT -> comp' -> FLAT' must reproduce the
// same FLAT (the data the format carries survives, given the OPT).
import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/simplified"
)

// TestDecodeIdempotent guards against nondeterministic sibling ordering: the
// same FLAT input must decode to a byte-identical canonical composition every
// time (Go map iteration must not leak into the output order). vital_signs has
// many sibling leaves across several observations, so it exercises the paths
// that a map-order bug would perturb.
func TestDecodeIdempotent(t *testing.T) {
	comp, wt := genComposition(t, "../../../testkit/cassettes/templates/vital_signs.opt")
	f1, err := simplified.MarshalFlat(comp, wt)
	if err != nil {
		t.Fatalf("MarshalFlat: %v", err)
	}
	var first []byte
	for i := range 8 {
		c2, err := simplified.UnmarshalFlat(f1, wt)
		if err != nil {
			t.Fatalf("UnmarshalFlat run %d: %v", i, err)
		}
		b, err := canjson.Marshal(c2)
		if err != nil {
			t.Fatalf("canjson.Marshal run %d: %v", i, err)
		}
		if i == 0 {
			first = b
		} else if !bytes.Equal(first, b) {
			t.Fatalf("decode not idempotent at run %d (sibling order leaked from map iteration)", i)
		}
	}
}

const vitalSignsOPT = "../../../testkit/cassettes/templates/vital_signs.opt"

// TestFlatRoundTripVitalSigns exercises DV_QUANTITY on an ITEM_SINGLE branch
// (body_temperature): the leaf aqlPath is .../data[at0001]/item[at0004]/value,
// where the ITEM_STRUCTURE is an ITEM_SINGLE (attribute `item`, not `items`).
// A decode that always rebuilds ITEM_TREE drops the value silently; this test
// asserts the full FLAT key set survives the round-trip (not just re-encode
// stability), so that class of data loss is caught.
func TestFlatRoundTripVitalSigns(t *testing.T) {
	comp, wt := genComposition(t, vitalSignsOPT)

	f1, err := simplified.MarshalFlat(comp, wt)
	if err != nil {
		t.Fatalf("MarshalFlat #1: %v", err)
	}
	comp2, err := simplified.UnmarshalFlat(f1, wt)
	if err != nil {
		t.Fatalf("UnmarshalFlat: %v", err)
	}
	f2, err := simplified.MarshalFlat(comp2, wt)
	if err != nil {
		t.Fatalf("MarshalFlat #2: %v", err)
	}

	var m1, m2 map[string]any
	if err := json.Unmarshal(f1, &m1); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(f2, &m2); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(m1, m2) {
		t.Errorf("vital_signs FLAT round-trip lost/changed keys:\n F1 (%d keys) = %v\n F2 (%d keys) = %v",
			len(m1), sortedKeys(m1), len(m2), sortedKeys(m2))
	}
	// Guard the specific ITEM_SINGLE quantity leaf that regressed.
	var sawTempMagnitude bool
	for k := range m2 {
		if strings.HasSuffix(k, "temperature|magnitude") {
			sawTempMagnitude = true
		}
	}
	if !sawTempMagnitude {
		t.Error("body_temperature DV_QUANTITY (ITEM_SINGLE) magnitude missing after round-trip")
	}
}

func TestFlatRoundTrip(t *testing.T) {
	comp, wt := genComposition(t, minimalObsOPT)

	f1, err := simplified.MarshalFlat(comp, wt)
	if err != nil {
		t.Fatalf("MarshalFlat #1: %v", err)
	}
	comp2, err := simplified.UnmarshalFlat(f1, wt)
	if err != nil {
		t.Fatalf("UnmarshalFlat: %v", err)
	}
	f2, err := simplified.MarshalFlat(comp2, wt)
	if err != nil {
		t.Fatalf("MarshalFlat #2: %v", err)
	}

	var m1, m2 map[string]any
	if err := json.Unmarshal(f1, &m1); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(f2, &m2); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(m1, m2) {
		t.Errorf("FLAT round-trip mismatch:\n F1 = %v\n F2 = %v", m1, m2)
	}
	if len(m2) == 0 {
		t.Fatal("decoded composition re-encoded to an empty FLAT map")
	}
}

// TestStructuredRoundTrip exercises the STRUCTURED decode path
// (UnmarshalStructured -> structuredToFlat -> UnmarshalFlat): a composition
// encoded to STRUCTURED and decoded back re-encodes to the same FLAT.
func TestStructuredRoundTrip(t *testing.T) {
	comp, wt := genComposition(t, minimalObsOPT)

	want, err := simplified.MarshalFlat(comp, wt)
	if err != nil {
		t.Fatalf("MarshalFlat: %v", err)
	}
	s, err := simplified.MarshalStructured(comp, wt)
	if err != nil {
		t.Fatalf("MarshalStructured: %v", err)
	}
	comp2, err := simplified.UnmarshalStructured(s, wt)
	if err != nil {
		t.Fatalf("UnmarshalStructured: %v", err)
	}
	got, err := simplified.MarshalFlat(comp2, wt)
	if err != nil {
		t.Fatalf("MarshalFlat #2: %v", err)
	}

	var wm, gm map[string]any
	if err := json.Unmarshal(want, &wm); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(got, &gm); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(wm, gm) {
		t.Errorf("STRUCTURED round-trip mismatch:\n want %v\n got  %v", wm, gm)
	}
}
