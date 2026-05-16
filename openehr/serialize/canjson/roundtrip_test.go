package canjson_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
)

// cassetteDir resolves the vendored cassette directory relative to
// this test file's package — testkit/cassettes/canonical_json/
// sibling of openehr/.
const cassetteDir = "../../../testkit/cassettes/canonical_json"

// listCassettes returns the vendored cassette filenames.
func listCassettes(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(cassetteDir)
	if err != nil {
		t.Fatalf("read cassette dir %q: %v", cassetteDir, err)
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		out = append(out, e.Name())
	}
	return out
}

// TestRoundTripStableSimpleValues — decode → encode → decode → encode
// produces byte-stable output for representative leaf types and a
// composition shape without history. See [TestRoundTripCassettes]
// below for the broader cassette-wide round-trip (composition
// fixtures with history; polymorphic event dispatch settled in
// docs/adr/0003-rm-event-polymorphism.md).
//
// Stability is the load-bearing guarantee for hashing / signing /
// diffing (PROBE-030 sub-property). Byte equality vs an arbitrary
// upstream serializer is NOT promised — the SDK has its own
// canonical profile (REQ-052).
func TestRoundTripStableSimpleValues(t *testing.T) {
	cases := []struct {
		name string
		body []byte
		into func() any
	}{
		{
			name: "DV_QUANTITY",
			body: []byte(`{"_type":"DV_QUANTITY","magnitude":80.5,"units":"kg"}`),
			into: func() any { return new(rm.DVQuantity) },
		},
		{
			name: "DV_CODED_TEXT",
			body: []byte(`{"_type":"DV_CODED_TEXT","value":"event","defining_code":{"_type":"CODE_PHRASE","code_string":"433","terminology_id":{"_type":"TERMINOLOGY_ID","value":"openehr"}}}`),
			into: func() any { return new(rm.DVCodedText) },
		},
		{
			name: "Composition-without-history",
			body: []byte(`{
				"_type": "COMPOSITION",
				"archetype_node_id": "openEHR-EHR-COMPOSITION.encounter.v1",
				"name": {"_type":"DV_TEXT","value":"x"},
				"language": {"_type":"CODE_PHRASE","code_string":"en","terminology_id":{"_type":"TERMINOLOGY_ID","value":"ISO_639-1"}},
				"territory": {"_type":"CODE_PHRASE","code_string":"GB","terminology_id":{"_type":"TERMINOLOGY_ID","value":"ISO_3166-1"}},
				"category": {"_type":"DV_CODED_TEXT","value":"event","defining_code":{"_type":"CODE_PHRASE","code_string":"433","terminology_id":{"_type":"TERMINOLOGY_ID","value":"openehr"}}},
				"composer": {"_type":"PARTY_SELF"}
			}`),
			into: func() any { return new(rm.Composition) },
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v1 := tc.into()
			if err := canjson.Unmarshal(tc.body, v1); err != nil {
				t.Fatalf("first Unmarshal: %v", err)
			}
			b1, err := canjson.Marshal(v1)
			if err != nil {
				t.Fatalf("first Marshal: %v", err)
			}
			v2 := tc.into()
			if err := canjson.Unmarshal(b1, v2); err != nil {
				t.Fatalf("second Unmarshal: %v\nbody: %s", err, b1)
			}
			b2, err := canjson.Marshal(v2)
			if err != nil {
				t.Fatalf("second Marshal: %v", err)
			}
			if !bytes.Equal(b1, b2) {
				t.Errorf("round-trip not byte-stable:\n--- b1 ---\n%s\n--- b2 ---\n%s", b1, b2)
			}
		})
	}
}

// TestRoundTripStructuralEquivalence asserts that the SDK round-trip
// preserves every JSON value present in the simple-value fixtures
// after normalising null / absent equivalence — the codec MUST NOT
// silently drop data on decode.
func TestRoundTripStructuralEquivalence(t *testing.T) {
	body := []byte(`{
		"_type": "COMPOSITION",
		"archetype_node_id": "x",
		"name": {"_type":"DV_TEXT","value":"x"},
		"language": {"_type":"CODE_PHRASE","code_string":"en","terminology_id":{"_type":"TERMINOLOGY_ID","value":"ISO_639-1"}},
		"territory": {"_type":"CODE_PHRASE","code_string":"GB","terminology_id":{"_type":"TERMINOLOGY_ID","value":"ISO_3166-1"}},
		"category": {"_type":"DV_CODED_TEXT","value":"event","defining_code":{"_type":"CODE_PHRASE","code_string":"433","terminology_id":{"_type":"TERMINOLOGY_ID","value":"openehr"}}},
		"composer": {"_type":"PARTY_SELF"}
	}`)
	var c rm.Composition
	if err := canjson.Unmarshal(body, &c); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	b, err := canjson.Marshal(&c)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	orig, err := normaliseJSON(body)
	if err != nil {
		t.Fatalf("normalise original: %v", err)
	}
	round, err := normaliseJSON(b)
	if err != nil {
		t.Fatalf("normalise round-trip: %v", err)
	}
	if !jsonEqual(orig, round) {
		oj, _ := json.MarshalIndent(orig, "", "  ")
		rj, _ := json.MarshalIndent(round, "", "  ")
		t.Errorf("structural mismatch:\n--- original ---\n%s\n--- round-trip ---\n%s", oj, rj)
	}
}

// TestRoundTripCassettes asserts byte-stable decode → encode → decode
// → encode across every vendored composition cassette (PROBE-030).
func TestRoundTripCassettes(t *testing.T) {
	for _, name := range listCassettes(t) {
		t.Run(name, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join(cassetteDir, name))
			if err != nil {
				t.Fatalf("read cassette: %v", err)
			}
			var c rm.Composition
			if err := canjson.Unmarshal(raw, &c); err != nil {
				t.Fatalf("first Unmarshal: %v", err)
			}
			b1, err := canjson.Marshal(&c)
			if err != nil {
				t.Fatalf("first Marshal: %v", err)
			}
			var c2 rm.Composition
			if err := canjson.Unmarshal(b1, &c2); err != nil {
				t.Fatalf("second Unmarshal: %v\nbody: %s", err, b1)
			}
			b2, err := canjson.Marshal(&c2)
			if err != nil {
				t.Fatalf("second Marshal: %v", err)
			}
			if !bytes.Equal(b1, b2) {
				t.Errorf("round-trip not byte-stable for %s:\n--- b1 ---\n%s\n--- b2 ---\n%s", name, b1, b2)
			}
		})
	}
}

// normaliseJSON parses data, strips map entries whose value is nil
// (the SDK treats null and absent equivalently on decode and emits
// absent on encode), and recursively normalises nested structures.
func normaliseJSON(data []byte) (any, error) {
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, err
	}
	return stripNulls(v), nil
}

// stripNulls recursively removes nil-valued entries from maps.
func stripNulls(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, val := range x {
			if val == nil {
				continue
			}
			cleaned := stripNulls(val)
			if cleaned == nil {
				continue
			}
			out[k] = cleaned
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = stripNulls(e)
		}
		return out
	default:
		return v
	}
}

// jsonEqual compares two normalised JSON values for structural
// equality.
func jsonEqual(a, b any) bool {
	switch ax := a.(type) {
	case map[string]any:
		bx, ok := b.(map[string]any)
		if !ok || len(ax) != len(bx) {
			return false
		}
		for k, av := range ax {
			bv, ok := bx[k]
			if !ok || !jsonEqual(av, bv) {
				return false
			}
		}
		return true
	case []any:
		bx, ok := b.([]any)
		if !ok || len(ax) != len(bx) {
			return false
		}
		for i := range ax {
			if !jsonEqual(ax[i], bx[i]) {
				return false
			}
		}
		return true
	default:
		return a == b
	}
}
