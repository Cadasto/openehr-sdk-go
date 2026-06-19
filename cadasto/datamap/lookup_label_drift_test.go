package datamap

import "testing"

// PROBE-058h — lookupChildPayload recovers a node whose datamap key was built
// with a different display label than the encoder derives. Empty() labels
// person_details.v2 at0001 "Demografische gegevens" (party-root term scope)
// while the encoder, re-scoped to the nested archetype, derives
// "Geboortegegevens"; without the nodeID-prefix fallback the labelled key
// misses and the cluster (e.g. the patient birth date) is silently dropped.
func TestLookupChildPayload_LabelDriftFallback(t *testing.T) {
	payload := map[string]any{
		"at0001|Demografische gegevens": map[string]any{"at0010": "1980-01-01"},
		"at0031":                        "at0310",
	}

	// Encoder label differs from the key's label → exact + bare both miss,
	// prefix fallback recovers it.
	if v, ok := lookupChildPayload(payload, "at0001", "Geboortegegevens"); !ok {
		t.Fatal("at0001 not found despite label drift — birth cluster would be dropped")
	} else if m, _ := v.(map[string]any); m["at0010"] != "1980-01-01" {
		t.Errorf("recovered wrong payload: %v", v)
	}

	// Exact labelled match still wins when the label agrees.
	if _, ok := lookupChildPayload(payload, "at0001", "Demografische gegevens"); !ok {
		t.Error("exact labelled key must still match")
	}

	// Bare key still matches.
	if v, ok := lookupChildPayload(payload, "at0031", "Geslacht"); !ok || v != "at0310" {
		t.Errorf("bare key match broke: ok=%v v=%v", ok, v)
	}

	// Genuinely absent node stays not-found.
	if _, ok := lookupChildPayload(payload, "at9999", "X"); ok {
		t.Error("absent node must not match")
	}
}
