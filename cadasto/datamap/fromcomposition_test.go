package datamap

import "testing"

// REQ-058 — read path: a canonical RM OBSERVATION composition decodes into the
// datamap shape (language/territory/composer/context + content → events →
// items, with CLUSTER/ELEMENT keyed by "<at-code>|<label>").

func obsComposition() map[string]any {
	return map[string]any{
		"_type":     "COMPOSITION",
		"language":  map[string]any{"_type": "CODE_PHRASE", "code_string": "nl"},
		"territory": map[string]any{"_type": "CODE_PHRASE", "code_string": "NL"},
		"composer":  map[string]any{"_type": "PARTY_IDENTIFIED", "name": "Dr. Jansen"},
		"context": map[string]any{
			"start_time": map[string]any{"_type": "DV_DATE_TIME", "value": "2026-05-27T10:00:00Z"},
			"setting":    map[string]any{"_type": "DV_CODED_TEXT", "defining_code": map[string]any{"code_string": "238"}},
		},
		"content": []any{
			map[string]any{
				"_type":             "OBSERVATION",
				"archetype_node_id": "openEHR-EHR-OBSERVATION.vital_signs.v1",
				"name":              map[string]any{"value": "vital_signs"},
				"data": map[string]any{
					"events": []any{
						map[string]any{
							"time": map[string]any{"_type": "DV_DATE_TIME", "value": "2026-05-27T10:00:00Z"},
							"data": map[string]any{
								"items": []any{
									map[string]any{
										"_type":             "CLUSTER",
										"archetype_node_id": "at0004",
										"name":              map[string]any{"value": "Tensie"},
										"items": []any{
											map[string]any{
												"_type":             "ELEMENT",
												"archetype_node_id": "at0006",
												"name":              map[string]any{"value": "Systolic"},
												"value":             map[string]any{"_type": "DV_QUANTITY", "magnitude": 120.0, "units": "mm[Hg]"},
											},
										},
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

func TestFromCompositionObservation(t *testing.T) {
	dm, err := FromComposition(nil, obsComposition())
	if err != nil {
		t.Fatalf("FromComposition: %v", err)
	}

	if dm["language"] != "nl" || dm["territory"] != "NL" || dm["composer"] != "Dr. Jansen" {
		t.Errorf("top-level: got language=%v territory=%v composer=%v", dm["language"], dm["territory"], dm["composer"])
	}

	ctx := dm["context"].(map[string]any)
	if ctx["start_time"] != "2026-05-27T10:00:00Z" {
		t.Errorf("context.start_time: got %v", ctx["start_time"])
	}
	if ctx["setting"] != "238" {
		t.Errorf("context.setting: got %v", ctx["setting"])
	}

	content := dm["content"].(map[string]any)
	root, ok := content["openEHR-EHR-OBSERVATION.vital_signs.v1|vital_signs"].(map[string]any)
	if !ok {
		t.Fatalf("content root key missing; keys=%v", keysOf(content))
	}
	events := root["events"].([]any)
	if len(events) != 1 {
		t.Fatalf("events: want 1, got %d", len(events))
	}
	ev := events[0].(map[string]any)
	if ev["time"] != "2026-05-27T10:00:00Z" {
		t.Errorf("event.time: got %v", ev["time"])
	}
	cluster, ok := ev["at0004|Tensie"].(map[string]any)
	if !ok {
		t.Fatalf("cluster key missing; event keys=%v", keysOf(ev))
	}
	elem, ok := cluster["at0006|Systolic"].(map[string]any)
	if !ok {
		t.Fatalf("element key missing; cluster keys=%v", keysOf(cluster))
	}
	if elem["magnitude"] != 120.0 || elem["units"] != "mm[Hg]" {
		t.Errorf("element value: got %#v", elem)
	}
	if _, leaked := elem["_type"]; leaked {
		t.Error("element value still carries _type (RM bookkeeping not stripped)")
	}
}

func TestFromCompositionRejectsNonComposition(t *testing.T) {
	if _, err := FromComposition(nil, map[string]any{"_type": "OBSERVATION"}); err == nil {
		t.Error("expected error for non-COMPOSITION input")
	}
	if _, err := FromComposition(nil, nil); err == nil {
		t.Error("expected error for nil composition")
	}
}

func keysOf(m map[string]any) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	return ks
}
