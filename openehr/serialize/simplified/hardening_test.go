package simplified_test

// REQ-053 — strict-decode, index-bound, and integer-precision guarantees. The
// codec must fail loudly on data it cannot faithfully represent, must not let a
// hostile :index force an unbounded allocation, and must not round large
// integers through float64.
import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/serialize/simplified"
)

// TestDecodeRejectsUnknownPath: a key that does not resolve to a Web Template
// node (wrong template / typo) is ErrUnknownPath, not a silently-empty comp.
func TestDecodeRejectsUnknownPath(t *testing.T) {
	_, wt := genComposition(t, minimalObsOPT)
	bogus := []byte(`{"not_this_template/nope/leaf": "x"}`)
	_, err := simplified.UnmarshalFlat(bogus, wt)
	if !errors.Is(err, simplified.ErrUnknownPath) {
		t.Fatalf("UnmarshalFlat(unknown path) err = %v, want ErrUnknownPath", err)
	}
}

// TestDecodeRejectsHugeIndex: a repeatable :index beyond the bound is an error
// rather than a huge slice allocation (both decode and interconversion).
func TestDecodeRejectsHugeIndex(t *testing.T) {
	comp, wt := genComposition(t, vitalSignsOPT)
	f1, err := simplified.MarshalFlat(comp, wt)
	if err != nil {
		t.Fatalf("MarshalFlat: %v", err)
	}
	// Mutate the first repeatable :index (encode only stamps :index on
	// repeatables) to an out-of-range value.
	mutated := strings.Replace(string(f1), ":0", ":100000001", 1)
	if mutated == string(f1) {
		t.Skip("no repeatable :index in fixture to mutate")
	}
	if _, err := simplified.UnmarshalFlat([]byte(mutated), wt); err == nil {
		t.Error("UnmarshalFlat(huge :index) = nil error, want bound error")
	}
	if _, err := simplified.FlatToStructured([]byte(mutated)); err == nil {
		t.Error("FlatToStructured(huge :index) = nil error, want bound error")
	}
}

// TestInterconvPreservesLargeInteger: a bare integer above 2^53 survives
// FLAT<->STRUCTURED interconversion exactly (json.Number, not float64).
func TestInterconvPreservesLargeInteger(t *testing.T) {
	const big = "9007199254740993" // 2^53 + 1, not representable as float64
	flat := []byte(`{"t/count:0": ` + big + `}`)
	s, err := simplified.FlatToStructured(flat)
	if err != nil {
		t.Fatalf("FlatToStructured: %v", err)
	}
	back, err := simplified.StructuredToFlat(s)
	if err != nil {
		t.Fatalf("StructuredToFlat: %v", err)
	}
	var m map[string]any
	dec := json.NewDecoder(strings.NewReader(string(back)))
	dec.UseNumber()
	if err := dec.Decode(&m); err != nil {
		t.Fatal(err)
	}
	if got, _ := m["t/count:0"].(json.Number); got.String() != big {
		t.Errorf("large integer round-trip = %v, want %s", m["t/count:0"], big)
	}
}
