package simplified_test

// REQ-053 — strict-decode, index-bound, and integer-precision guarantees. The
// codec must fail loudly on data it cannot faithfully represent, must not let a
// hostile :index force an unbounded allocation, and must not round large
// integers through float64.
import (
	"encoding/json"
	"errors"
	"maps"
	"slices"
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
	// Mutate the first repeatable content :index to an out-of-range value. A
	// content index is always followed by "/" (a deeper segment), which avoids
	// matching a ":0" inside a ctx/time timestamp value.
	mutated := strings.Replace(string(f1), ":0/", ":100000001/", 1)
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

// TestDecodeRejectsNegativeIndex: ":-1" collides with the internal "no index"
// sentinel and would silently drop one of two values resolving to the same
// slot; it must be rejected on decode and on OPT-free interconversion.
func TestDecodeRejectsNegativeIndex(t *testing.T) {
	comp, wt := genComposition(t, minimalObsOPT)
	f1, err := simplified.MarshalFlat(comp, wt)
	if err != nil {
		t.Fatalf("MarshalFlat: %v", err)
	}
	mutated := strings.Replace(string(f1), ":0/", ":-1/", 1)
	if mutated == string(f1) {
		t.Skip("no repeatable :index in fixture to mutate")
	}
	if _, err := simplified.UnmarshalFlat([]byte(mutated), wt); !errors.Is(err, simplified.ErrUnknownPath) {
		t.Errorf("UnmarshalFlat(:-1) err = %v, want ErrUnknownPath", err)
	}
	if _, err := simplified.FlatToStructured([]byte(mutated)); !errors.Is(err, simplified.ErrUnknownPath) {
		t.Errorf("FlatToStructured(:-1) err = %v, want ErrUnknownPath", err)
	}
}

// TestDecodeRejectsSparseIndex: ":2" with no ":0"/":1" would gap-fill phantom
// empty instances (which RM-mandatory completion could then decorate into
// seemingly valid data) — rejected instead.
func TestDecodeRejectsSparseIndex(t *testing.T) {
	comp, wt := genComposition(t, minimalObsOPT)
	f1, err := simplified.MarshalFlat(comp, wt)
	if err != nil {
		t.Fatalf("MarshalFlat: %v", err)
	}
	// Move the whole instance, not the first key: a single replacement splits
	// whichever multi-suffix leaf happens to sort first across :0 and :2, and the
	// resulting incomplete leaf fails as a datatype error before the index
	// sequence is ever examined — masking the property under test.
	mutated := strings.ReplaceAll(string(f1), ":0/", ":2/")
	if mutated == string(f1) {
		t.Skip("no repeatable :index in fixture to mutate")
	}
	if _, err := simplified.UnmarshalFlat([]byte(mutated), wt); !errors.Is(err, simplified.ErrUnknownPath) {
		t.Errorf("UnmarshalFlat(sparse :2) err = %v, want ErrUnknownPath", err)
	}
}

// TestDecodeRejectsIndexCollision: "a" (no index) and "a:0" are distinct JSON
// keys resolving to the same instance slot; last-write-wins would silently
// drop one value, so the collision is an error.
func TestDecodeRejectsIndexCollision(t *testing.T) {
	comp, wt := genComposition(t, minimalObsOPT)
	f1, err := simplified.MarshalFlat(comp, wt)
	if err != nil {
		t.Fatalf("MarshalFlat: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(f1, &m); err != nil {
		t.Fatal(err)
	}
	// Duplicate a whole leaf family, and pick it deterministically. Copying one
	// key leaves the un-indexed group holding a partial suffix set, and the
	// resulting datatype error ("missing required |code") fires before the slot
	// collision this test is about — masking it.
	var base string
	for _, k := range slices.Sorted(maps.Keys(m)) {
		if strings.Contains(k, ":0/") {
			base = k
			if i := strings.LastIndex(k, "|"); i >= 0 {
				base = k[:i]
			}
			break
		}
	}
	if base == "" {
		t.Skip("no repeatable :index in fixture to duplicate")
	}
	dupes := map[string]any{}
	for k, v := range m {
		if k == base || strings.HasPrefix(k, base+"|") {
			dupes[strings.Replace(k, ":0/", "/", 1)] = v
		}
	}
	maps.Copy(m, dupes)
	dup, _ := json.Marshal(m)
	if _, err := simplified.UnmarshalFlat(dup, wt); !errors.Is(err, simplified.ErrUnknownPath) {
		t.Errorf("UnmarshalFlat(index collision) err = %v, want ErrUnknownPath", err)
	}
}

// TestDecodeRejectsWrongTypedCtx: a ctx/ value of the wrong JSON type must be
// rejected, not coerced — a numeric composer_name would otherwise become an
// empty PARTY_IDENTIFIED name (silent authorship corruption).
func TestDecodeRejectsWrongTypedCtx(t *testing.T) {
	comp, wt := genComposition(t, minimalObsOPT)
	f1, err := simplified.MarshalFlat(comp, wt)
	if err != nil {
		t.Fatalf("MarshalFlat: %v", err)
	}
	for _, bad := range []struct {
		key string
		val any
	}{
		{"ctx/composer_name", 42},
		{"ctx/language", 42},
		{"ctx/territory", true},
		{"ctx/time", 42},
		{"ctx/composer_self", "true"},
		{"ctx/setting|code", 238}, // REQ-053: the setting pair is string-valued
		{"ctx/setting|value", 42},
	} {
		var m map[string]any
		if err := json.Unmarshal(f1, &m); err != nil {
			t.Fatal(err)
		}
		m[bad.key] = bad.val
		mutated, _ := json.Marshal(m)
		if _, err := simplified.UnmarshalFlat(mutated, wt); !errors.Is(err, simplified.ErrUnsupportedDatatype) {
			t.Errorf("UnmarshalFlat(%s = %v) err = %v, want ErrUnsupportedDatatype", bad.key, bad.val, err)
		}
	}
}

// TestMalformedOrderedSuffixNamesFlatKey — PR #86 review round 3. A malformed
// value for one of the pass-through DV_ORDERED / DV_QUANTIFIED / DV_AMOUNT
// suffixes used to escape as a raw canjson error — "decode /content/3:
// typereg.Decode …" — naming a canonical path the payload author never wrote and
// no FLAT key at all, which is undiagnosable from the payload side. The refusal
// now happens while the offending FLAT key is still in hand.
//
// Deliberately no sentinel: a malformed value is a defect in the payload, not a
// datatype or path the codec declines to model, and the PROBE-086 census counts
// only the latter (a non-sentinel error is a fault there, which is correct here).
func TestMalformedOrderedSuffixNamesFlatKey(t *testing.T) {
	comp, wt := genComposition(t, vitalSignsOPT)
	f1, err := simplified.MarshalFlat(comp, wt)
	if err != nil {
		t.Fatalf("MarshalFlat: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(f1, &m); err != nil {
		t.Fatal(err)
	}
	// Any DV_QUANTITY leaf will do; take it from the encoder's own output so the
	// test does not hard-code a fixture path.
	var base string
	for _, k := range slices.Sorted(maps.Keys(m)) {
		if rest, ok := strings.CutSuffix(k, "|magnitude"); ok {
			base = rest
			break
		}
	}
	if base == "" {
		t.Fatal("no |magnitude leaf in the encoded fixture")
	}
	m[base+"|accuracy"] = "abc" // a Real attribute, given a string
	mutated, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	_, err = simplified.UnmarshalFlat(mutated, wt)
	if err == nil {
		t.Fatal("UnmarshalFlat(|accuracy = \"abc\") = nil error, want a refusal")
	}
	for _, want := range []string{base, "|accuracy", "number"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not name %q", err, want)
		}
	}
	if errors.Is(err, simplified.ErrUnknownPath) || errors.Is(err, simplified.ErrUnsupportedDatatype) {
		t.Errorf("err = %v carries a modelled-gap sentinel; a malformed value is a payload defect", err)
	}
	// The same key with a well-formed value decodes, so the check bounds the kind
	// and nothing more.
	m[base+"|accuracy"] = 0.5
	m[base+"|accuracy_is_percent"] = true
	ok, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := simplified.UnmarshalFlat(ok, wt); err != nil {
		t.Errorf("well-formed |accuracy rejected: %v", err)
	}
}

// TestDecodeRejectsTrailingJSON: content after the first JSON object is an error,
// not silently ignored.
func TestDecodeRejectsTrailingJSON(t *testing.T) {
	_, wt := genComposition(t, minimalObsOPT)
	if _, err := simplified.UnmarshalFlat([]byte(`{"ctx/language":"en"} {"extra":1}`), wt); err == nil {
		t.Error("UnmarshalFlat with trailing JSON = nil error, want rejection")
	}
	if _, err := simplified.FlatToStructured([]byte(`{} 99`)); err == nil {
		t.Error("FlatToStructured with trailing JSON = nil error, want rejection")
	}
}

// TestStructuredToFlatRejectsMalformed: a non-array clinical child, a null
// array hole, and an element carrying no entries are errors, not silent drops.
func TestStructuredToFlatRejectsMalformed(t *testing.T) {
	if _, err := simplified.StructuredToFlat([]byte(`{"t":{"leaf":"not-an-array"}}`)); err == nil {
		t.Error("non-array clinical child = nil error, want rejection")
	}
	if _, err := simplified.StructuredToFlat([]byte(`{"t":{"leaf":[null]}}`)); err == nil {
		t.Error("null array hole = nil error, want rejection")
	}
	if _, err := simplified.StructuredToFlat([]byte(`{"t":"not-an-object"}`)); err == nil {
		t.Error("non-object root = nil error, want rejection")
	}
	if _, err := simplified.StructuredToFlat([]byte(`{"t":{"a":[{}]}}`)); err == nil {
		t.Error("empty-object element = nil error, want rejection (it would vanish)")
	}
}

// TestFlatToStructuredRejectsConflicts: two FLAT keys claiming the same
// STRUCTURED slot with incompatible shapes (bare scalar vs |suffix object, or
// a bare root value) must error deterministically — last-write-wins would make
// the output depend on Go map iteration order.
func TestFlatToStructuredRejectsConflicts(t *testing.T) {
	cases := []string{
		`{"t/leaf:0": "bare", "t/leaf:0|code": "c"}`,
		`{"t/a:0": "x", "t/a:0/b:0": "y"}`,
		`{"t": 5}`,
	}
	for _, in := range cases {
		if _, err := simplified.FlatToStructured([]byte(in)); err == nil {
			t.Errorf("FlatToStructured(%s) = nil error, want conflict rejection", in)
		}
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
