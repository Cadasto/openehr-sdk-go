package care

import "testing"

// REQ-058 — the typed bridge: a canonical-JSON composition (as the datamap
// codec emits) decodes into *rm.Composition and marshals back, preserving the
// clinical content. Hermetic (no CDR).

func canonicalComposition() map[string]any {
	cp := func(term, code string) map[string]any {
		return map[string]any{
			"_type":          "CODE_PHRASE",
			"terminology_id": map[string]any{"_type": "TERMINOLOGY_ID", "value": term},
			"code_string":    code,
		}
	}
	dvText := func(v string) map[string]any { return map[string]any{"_type": "DV_TEXT", "value": v} }

	return map[string]any{
		"_type":             "COMPOSITION",
		"archetype_node_id": "openEHR-EHR-COMPOSITION.encounter.v1",
		"name":              dvText("Encounter"),
		"archetype_details": map[string]any{
			"_type":        "ARCHETYPED",
			"archetype_id": map[string]any{"_type": "ARCHETYPE_ID", "value": "openEHR-EHR-COMPOSITION.encounter.v1"},
			"rm_version":   "1.0.2",
		},
		"language":  cp("ISO_639-1", "nl"),
		"territory": cp("ISO_3166-1", "NL"),
		"category":  map[string]any{"_type": "DV_CODED_TEXT", "value": "event", "defining_code": cp("openehr", "433")},
		"composer":  map[string]any{"_type": "PARTY_IDENTIFIED", "name": "Dr. Jansen"},
		"context": map[string]any{
			"_type":      "EVENT_CONTEXT",
			"start_time": map[string]any{"_type": "DV_DATE_TIME", "value": "2026-05-27T10:00:00Z"},
			"setting":    map[string]any{"_type": "DV_CODED_TEXT", "value": "other care", "defining_code": cp("openehr", "238")},
		},
		"content": []any{
			map[string]any{
				"_type":             "OBSERVATION",
				"archetype_node_id": "openEHR-EHR-OBSERVATION.vital_signs.v1",
				"name":              dvText("vital_signs"),
				"language":          cp("ISO_639-1", "nl"),
				"encoding":          cp("IANA_character-sets", "UTF-8"),
				"subject":           map[string]any{"_type": "PARTY_SELF"},
				"data": map[string]any{
					"_type":             "HISTORY",
					"archetype_node_id": "at0001",
					"name":              dvText("Event Series"),
					"origin":            map[string]any{"_type": "DV_DATE_TIME", "value": "2026-05-27T10:00:00Z"},
					"events": []any{
						map[string]any{
							"_type":             "POINT_EVENT",
							"archetype_node_id": "at0002",
							"name":              dvText("Any event"),
							"time":              map[string]any{"_type": "DV_DATE_TIME", "value": "2026-05-27T10:00:00Z"},
							"data": map[string]any{
								"_type":             "ITEM_TREE",
								"archetype_node_id": "at0003",
								"name":              dvText("Tree"),
								"items": []any{
									map[string]any{
										"_type":             "ELEMENT",
										"archetype_node_id": "at0004",
										"name":              dvText("Note"),
										"value":             dvText("Patient stable"),
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func TestBridgeRoundTrip(t *testing.T) {
	in := canonicalComposition()

	comp, err := compositionFromMap(in)
	if err != nil {
		t.Fatalf("compositionFromMap: %v", err)
	}

	out, err := compositionToMap(comp)
	if err != nil {
		t.Fatalf("compositionToMap: %v", err)
	}

	if out["_type"] != "COMPOSITION" {
		t.Fatalf("_type: got %v", out["_type"])
	}
	content, ok := out["content"].([]any)
	if !ok || len(content) != 1 {
		t.Fatalf("content: got %#v", out["content"])
	}
	obs := content[0].(map[string]any)
	if obs["_type"] != "OBSERVATION" || obs["archetype_node_id"] != "openEHR-EHR-OBSERVATION.vital_signs.v1" {
		t.Fatalf("observation: _type=%v node=%v", obs["_type"], obs["archetype_node_id"])
	}
	// The DV_TEXT element value survives the typed round-trip.
	if !containsLeaf(out, "Patient stable") {
		t.Error("element value 'Patient stable' lost in typed round-trip")
	}
}

func containsLeaf(v any, want string) bool {
	switch t := v.(type) {
	case map[string]any:
		for _, vv := range t {
			if containsLeaf(vv, want) {
				return true
			}
		}
	case []any:
		for _, vv := range t {
			if containsLeaf(vv, want) {
				return true
			}
		}
	case string:
		return t == want
	}
	return false
}
