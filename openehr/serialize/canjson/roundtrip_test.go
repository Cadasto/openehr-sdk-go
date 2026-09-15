package canjson_test

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
	"github.com/cadasto/openehr-sdk-go/testkit/wireequiv"
)

// listCassettes returns vendored composition JSON paths relative to
// testkit/cassettes (via testkit/fixtures discovery).
func listCassettes(t *testing.T) []fixtures.CompositionJSONRel {
	t.Helper()
	rels, err := fixtures.ListCompositionJSON()
	if err != nil {
		t.Fatalf("list cassettes: %v", err)
	}
	return rels
}

func cassetteFactory(t *testing.T, rel fixtures.CompositionJSONRel) func() any {
	t.Helper()
	f, ok := fixtures.FactoryForJSONRel(rel)
	if !ok {
		t.Fatalf("no factory for %s (_type %s)", rel.Rel, fixtures.FactoryHintForRel(rel.Rel))
	}
	return f
}

// TestRoundTripStableSimpleValues runs decode, encode (b1), decode (A),
// encode (b2), decode (B) and preserves the value for representative leaf
// types and a composition shape without history. See
// [TestRoundTripCassettes] below for the broader cassette-wide round
// trip (composition fixtures with history; polymorphic event dispatch
// settled in docs/adr/0003-rm-event-polymorphism.md).
//
// A and B straddle the second encode and MUST be equal by typed deep
// comparison; b1 and b2 MUST be wire-equivalent as a secondary check.
// Byte equality is not asserted: member order is not a contract and the
// SDK makes no byte-level promise (REQ-052). This is PROBE-030's
// package-level twin.
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
			v := tc.into()
			if err := canjson.Unmarshal(tc.body, v); err != nil {
				t.Fatalf("first Unmarshal: %v", err)
			}
			b1, err := canjson.Marshal(v)
			if err != nil {
				t.Fatalf("first Marshal: %v", err)
			}
			valueA := tc.into()
			if err := canjson.Unmarshal(b1, valueA); err != nil {
				t.Fatalf("second Unmarshal (A): %v\nbody: %s", err, b1)
			}
			b2, err := canjson.Marshal(valueA)
			if err != nil {
				t.Fatalf("second Marshal: %v", err)
			}
			valueB := tc.into()
			if err := canjson.Unmarshal(b2, valueB); err != nil {
				t.Fatalf("third Unmarshal (B): %v\nbody: %s", err, b2)
			}
			if !reflect.DeepEqual(valueA, valueB) {
				t.Errorf("round trip not value-stable across the re-encode:\n A %+v\n B %+v", valueA, valueB)
			}
			if ok, diff := wireequiv.Equivalent(b1, b2); !ok {
				t.Errorf("the two SDK encodes are not wire-equivalent: %s", diff)
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

// TestRoundTripCassettes is PROBE-030's package-level twin over the
// whole vendored corpus: decode, encode (b1), decode (A), encode (b2),
// decode (B). A and B straddle the second encode and MUST be equal by
// typed deep comparison; b1 and b2 MUST be wire-equivalent as a
// secondary check. Byte equality is not asserted (member order is not a
// contract, REQ-052). The SDK's own cassettes are all COMPOSITION;
// vendored upstream sets (e.g. ehrbase/) include EHR_STATUS and FOLDER,
// so the target factory is selected per cassette path.
func TestRoundTripCassettes(t *testing.T) {
	for _, rel := range listCassettes(t) {
		t.Run(rel.Rel, func(t *testing.T) {
			raw, err := os.ReadFile(fixtures.ResolveCompositionJSON(rel))
			if err != nil {
				t.Fatalf("read cassette: %v", err)
			}
			factory := cassetteFactory(t, rel)
			v := factory()
			if err := canjson.Unmarshal(raw, v); err != nil {
				t.Fatalf("first Unmarshal: %v", err)
			}
			b1, err := canjson.Marshal(v)
			if err != nil {
				t.Fatalf("first Marshal: %v", err)
			}
			valueA := factory()
			if err := canjson.Unmarshal(b1, valueA); err != nil {
				t.Fatalf("second Unmarshal (A): %v\nbody: %s", err, b1)
			}
			b2, err := canjson.Marshal(valueA)
			if err != nil {
				t.Fatalf("second Marshal: %v", err)
			}
			valueB := factory()
			if err := canjson.Unmarshal(b2, valueB); err != nil {
				t.Fatalf("third Unmarshal (B): %v\nbody: %s", err, b2)
			}
			if !reflect.DeepEqual(valueA, valueB) {
				t.Errorf("round trip not value-stable across the re-encode for %s", rel.Rel)
			}
			if ok, diff := wireequiv.Equivalent(b1, b2); !ok {
				t.Errorf("the two SDK encodes are not wire-equivalent for %s: %s", rel.Rel, diff)
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
