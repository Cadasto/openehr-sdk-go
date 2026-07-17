package datamap

import "testing"

// REQ-058 — read path: a canonical RM OBSERVATION composition decodes into the
// datamap shape (language/territory/composer/context + content → events →
// items, with CLUSTER/ELEMENT keyed by the bare "<at-code>" per SPEC §4.3).

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
	cluster, ok := ev["at0004"].(map[string]any)
	if !ok {
		t.Fatalf("cluster key missing; event keys=%v", keysOf(ev))
	}
	// DV_QUANTITY decodes to the bare magnitude (datamap short form); units are
	// template-derived and refilled on encode.
	if mag, ok := cluster["at0006"]; !ok || mag != 120.0 {
		t.Errorf("element value: got %#v (want 120.0)", cluster["at0006"])
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

// multiEntryRootKey is the "<archetype-id>|<label>" content-root key for the
// minimal_action_2 fixture's ACTION root, which the OPT constrains to
// occurrences 0..* (unbounded upper) — a repeatable content root, i.e. one
// archetype allowed to appear N times in the same COMPOSITION (REQ-0029: a
// persistent care_plan holding N pathway enrollments).
const multiEntryRootKey = "openEHR-EHR-ACTION.minimal_2.v1|Minimal 2"

// multiEntryPayload builds a distinct ACTION payload for the minimal_action_2
// root, distinguished by an explicit current_state code (the same short-form
// this OPT's ACTION already exercises in TestEncodeAction_CurrentStateFromPayload)
// so ToComposition's two emitted entries are not byte-identical.
func multiEntryPayload(code string) map[string]any {
	return map[string]any{
		"current_state": map[string]any{"code": code, "value": "active", "terminology": "openehr"},
	}
}

// PROBE-0782 proves REQ-0029 — a content-root key holding a []any of 2
// entry-maps round-trips: ToComposition emits 2 entries of that root, and
// FromComposition decodes back to a 2-element []any (instead of overwriting
// down to the last entry, the pre-REQ-0029 behavior).
func TestContentRoot_MultiEntryRoundTrip(t *testing.T) {
	opt := loadOPT(t, "minimal_action_2")
	dm := map[string]any{
		"context": map[string]any{"start_time": "2026-07-10T09:00:00Z"},
		"content": map[string]any{
			multiEntryRootKey: []any{
				multiEntryPayload("245"), // active
				multiEntryPayload("532"), // completed
			},
		},
	}

	comp, err := ToComposition(opt, dm)
	if err != nil {
		t.Fatalf("ToComposition: %v", err)
	}
	content, _ := comp["content"].([]any)
	if len(content) != 2 {
		t.Fatalf("content entries = %d, want 2", len(content))
	}

	back, err := FromComposition(opt, comp)
	if err != nil {
		t.Fatalf("FromComposition: %v", err)
	}
	backContent, _ := back["content"].(map[string]any)
	got, ok := backContent[multiEntryRootKey].([]any)
	if !ok || len(got) != 2 {
		t.Fatalf("decoded %s = %#v, want a 2-element []any", multiEntryRootKey, backContent[multiEntryRootKey])
	}
}

// PROBE-0783 proves REQ-0029 — backward compatibility: a content-root value
// that is a single map[string]any (the shape every template used before
// REQ-0029) still yields exactly one COMPOSITION content entry on encode and
// decodes back to a bare map, not a single-element []any.
func TestContentRoot_SingleMapStaysBareMap(t *testing.T) {
	opt := loadOPT(t, "minimal_action_2")
	dm := map[string]any{
		"context": map[string]any{"start_time": "2026-07-10T09:00:00Z"},
		"content": map[string]any{
			multiEntryRootKey: multiEntryPayload("245"),
		},
	}

	comp, err := ToComposition(opt, dm)
	if err != nil {
		t.Fatalf("ToComposition: %v", err)
	}
	content, _ := comp["content"].([]any)
	if len(content) != 1 {
		t.Fatalf("content entries = %d, want 1", len(content))
	}

	back, err := FromComposition(opt, comp)
	if err != nil {
		t.Fatalf("FromComposition: %v", err)
	}
	backContent, _ := back["content"].(map[string]any)
	if _, isList := backContent[multiEntryRootKey].([]any); isList {
		t.Fatalf("decoded %s is a []any, want a bare map for a single occurrence", multiEntryRootKey)
	}
	if _, ok := backContent[multiEntryRootKey].(map[string]any); !ok {
		t.Fatalf("decoded %s = %#v, want a map[string]any", multiEntryRootKey, backContent[multiEntryRootKey])
	}
}

// PROBE-0795 proves REQ-0029 — an ACTION's ism_transition (current_state +
// careflow_step) round-trips through ToComposition → FromComposition. Without
// this, GetComposition → FromComposition → (edit another pathway) →
// ToComposition → PUT silently resets every untouched pathway's careflow to
// the completed(532) default, because encodeAction falls back to that
// default whenever payload["current_state"] is absent.
func TestContentRoot_ActionIsmTransitionRoundTrip(t *testing.T) {
	opt := loadOPT(t, "minimal_action_2")
	dm := map[string]any{
		"context": map[string]any{"start_time": "2026-07-10T09:00:00Z"},
		"content": map[string]any{
			multiEntryRootKey: map[string]any{
				"current_state": map[string]any{"code": "245", "value": "active", "terminology": "openehr"},
				"careflow_step": map[string]any{"code": "prescribed", "value": "Prescribed", "terminology": "local"},
			},
		},
	}

	comp, err := ToComposition(opt, dm)
	if err != nil {
		t.Fatalf("ToComposition: %v", err)
	}

	back, err := FromComposition(opt, comp)
	if err != nil {
		t.Fatalf("FromComposition: %v", err)
	}
	backContent, _ := back["content"].(map[string]any)
	root, ok := backContent[multiEntryRootKey].(map[string]any)
	if !ok {
		t.Fatalf("decoded %s = %#v, want a map[string]any", multiEntryRootKey, backContent[multiEntryRootKey])
	}

	cs, ok := root["current_state"].(map[string]any)
	if !ok {
		t.Fatalf("current_state missing/wrong type: %#v", root["current_state"])
	}
	if cs["code"] != "245" || cs["value"] != "active" || cs["terminology"] != "openehr" {
		t.Errorf("current_state = %#v, want {code:245 value:active terminology:openehr}", cs)
	}

	step, ok := root["careflow_step"].(map[string]any)
	if !ok {
		t.Fatalf("careflow_step missing/wrong type: %#v", root["careflow_step"])
	}
	if step["code"] != "prescribed" || step["value"] != "Prescribed" || step["terminology"] != "local" {
		t.Errorf("careflow_step = %#v, want {code:prescribed value:Prescribed terminology:local}", step)
	}
}

// PROBE-0796 proves REQ-0029 — an ACTION with no ism_transition at all (a
// hand-built RM composition, or any pre-existing fixture that predates the
// current_state/careflow_step decode) still decodes without error and simply
// omits both keys — no panic on a missing/malformed ism_transition.
func TestDecodeAction_NoIsmTransition_NoPanic(t *testing.T) {
	opt := loadOPT(t, "minimal_action_2")
	comp := map[string]any{
		"_type": "COMPOSITION",
		"context": map[string]any{
			"start_time": map[string]any{"_type": "DV_DATE_TIME", "value": "2026-07-10T09:00:00Z"},
		},
		"content": []any{
			map[string]any{
				"_type":             "ACTION",
				"archetype_node_id": "openEHR-EHR-ACTION.minimal_2.v1",
				"name":              map[string]any{"value": "Minimal 2"},
				"time":              map[string]any{"_type": "DV_DATE_TIME", "value": "2026-07-10T09:00:00Z"},
				"description":       map[string]any{"items": []any{}},
				// no ism_transition at all
			},
		},
	}

	back, err := FromComposition(opt, comp)
	if err != nil {
		t.Fatalf("FromComposition: %v", err)
	}
	backContent, _ := back["content"].(map[string]any)
	root, ok := backContent[multiEntryRootKey].(map[string]any)
	if !ok {
		t.Fatalf("decoded %s = %#v, want a map[string]any", multiEntryRootKey, backContent[multiEntryRootKey])
	}
	if _, has := root["current_state"]; has {
		t.Errorf("current_state should be absent when ism_transition is absent, got %#v", root["current_state"])
	}
	if _, has := root["careflow_step"]; has {
		t.Errorf("careflow_step should be absent when ism_transition is absent, got %#v", root["careflow_step"])
	}
}

// PROBE-0544 proves REQ-0037 — decodeFeederAudit surfaces originating_system_audit.time
// (bron-verzendmoment, HL7 MSH-7) als platte string uit de RM DV_DATE_TIME.
func TestDecodeFeederAudit_Time(t *testing.T) {
	fa := decodeFeederAudit(map[string]any{
		"originating_system_audit": map[string]any{
			"system_id": "GLIMS",
			"time":      map[string]any{"_type": "DV_DATE_TIME", "value": "2026-07-16T08:10:00"},
		},
	})
	osa, ok := fa["originating_system_audit"].(map[string]any)
	if !ok {
		t.Fatalf("originating_system_audit missing: %v", fa)
	}
	if osa["time"] != "2026-07-16T08:10:00" {
		t.Errorf("time = %v, want plain string 2026-07-16T08:10:00", osa["time"])
	}
}
