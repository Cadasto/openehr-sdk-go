package simplified_test

// REQ-053 — |raw bypass: a leaf supplied as a canonical fragment under |raw is
// decoded directly (regardless of the WT leaf type), then re-encodes to the
// normal suffixed form.
import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/serialize/simplified"
)

func TestDecodeAcceptsRawLeaf(t *testing.T) {
	comp, wt := genComposition(t, vitalSignsOPT)
	f1, err := simplified.MarshalFlat(comp, wt)
	if err != nil {
		t.Fatalf("MarshalFlat: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(f1, &m); err != nil {
		t.Fatal(err)
	}
	// Find the temperature quantity leaf base path and replace its |magnitude /
	// |unit suffixes with a single |raw canonical fragment.
	var base string
	for k := range m {
		if b, ok := strings.CutSuffix(k, "|magnitude"); ok && strings.HasSuffix(b, "temperature") {
			base = b
			break
		}
	}
	if base == "" {
		t.Skip("no temperature|magnitude leaf in fixture")
	}
	delete(m, base+"|magnitude")
	delete(m, base+"|unit")
	m[base+"|raw"] = map[string]any{"_type": "DV_QUANTITY", "magnitude": 37.5, "units": "°C"}

	raw, _ := json.Marshal(m)
	comp2, err := simplified.UnmarshalFlat(raw, wt)
	if err != nil {
		t.Fatalf("UnmarshalFlat(|raw): %v", err)
	}
	// Re-encoding materialises the fragment back into the normal suffixed form.
	f2, err := simplified.MarshalFlat(comp2, wt)
	if err != nil {
		t.Fatalf("MarshalFlat #2: %v", err)
	}
	var m2 map[string]any
	if err := json.Unmarshal(f2, &m2); err != nil {
		t.Fatal(err)
	}
	if m2[base+"|magnitude"] != 37.5 {
		t.Errorf("%s|magnitude = %#v, want 37.5 (from |raw fragment)", base, m2[base+"|magnitude"])
	}
	if m2[base+"|unit"] != "°C" {
		t.Errorf("%s|unit = %#v, want °C (from |raw fragment)", base, m2[base+"|unit"])
	}
}
