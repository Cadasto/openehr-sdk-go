package datamap

import "testing"

// PROBE-0797 proves REQ-058 — encodeOtherParticipations turns a datamap
// other_participations array into PARTICIPATION entries whose performer is a
// PARTY_IDENTIFIED with the AGB on external_ref (AQL-queryable), reusing the
// composer external_ref seam. Entries lacking a function or performer are
// dropped; an all-empty input yields nil (attribute omitted).
func TestEncodeOtherParticipations(t *testing.T) {
	t.Run("nil when absent", func(t *testing.T) {
		got, err := encodeOtherParticipations(map[string]any{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != nil {
			t.Fatalf("want nil, got %v", got)
		}
	})

	t.Run("requestor + organisation -> external_ref", func(t *testing.T) {
		got, err := encodeOtherParticipations(map[string]any{
			"other_participations": []any{
				map[string]any{
					"function": "requestor",
					"performer": map[string]any{
						"name": "de Vries, Peter", "id": "03012345",
						"id_scheme": "AGB", "id_namespace": "lab24", "id_type": "PERSON",
					},
				},
				map[string]any{
					"function": "requesting_organisation",
					"performer": map[string]any{
						"id": "03068975", "id_scheme": "AGB", "id_type": "ORGANISATION",
					},
				},
			},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("want 2 participations, got %d (%v)", len(got), got)
		}
		p0 := got[0].(map[string]any)
		if p0["_type"] != "PARTICIPATION" {
			t.Fatalf("p0 _type = %v", p0["_type"])
		}
		if fn := p0["function"].(map[string]any); fn["value"] != "requestor" {
			t.Fatalf("p0 function = %v", fn)
		}
		perf := p0["performer"].(map[string]any)
		if perf["_type"] != "PARTY_IDENTIFIED" || perf["name"] != "de Vries, Peter" {
			t.Fatalf("p0 performer = %v", perf)
		}
		ext := perf["external_ref"].(map[string]any)
		if ext["type"] != "PERSON" || ext["namespace"] != "lab24" {
			t.Fatalf("p0 external_ref = %v", ext)
		}
		id := ext["id"].(map[string]any)
		if id["value"] != "03012345" || id["scheme"] != "AGB" {
			t.Fatalf("p0 external_ref.id = %v", id)
		}
		// organisation
		p1 := got[1].(map[string]any)
		ext1 := p1["performer"].(map[string]any)["external_ref"].(map[string]any)
		if ext1["type"] != "ORGANISATION" || ext1["id"].(map[string]any)["value"] != "03068975" {
			t.Fatalf("p1 external_ref = %v", ext1)
		}
	})

	t.Run("skips entries without function or performer", func(t *testing.T) {
		got, err := encodeOtherParticipations(map[string]any{
			"other_participations": []any{
				map[string]any{"performer": map[string]any{"id": "x"}}, // no function
				map[string]any{"function": "requestor"},                // no performer
			},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != nil {
			t.Fatalf("want nil (all dropped), got %v", got)
		}
	})

	t.Run("malformed mode fails the encode", func(t *testing.T) {
		_, err := encodeOtherParticipations(map[string]any{
			"other_participations": []any{
				map[string]any{
					"function":  "requestor",
					"performer": map[string]any{"name": "de Vries, Peter"},
					"mode":      map[string]any{"value": "no code here"},
				},
			},
		})
		if err == nil {
			t.Fatal("want error for a mode with no usable code, got nil")
		}
	})
}

// PROBE-0798 proves REQ-058 — decodeOtherParticipations round-trips the encoded
// ENTRY.other_participations back into the datamap shape encodeOtherParticipations
// consumes (function + performer name + external_ref id/scheme/namespace/type),
// and is OPTIONAL: an entry without participations decodes to nil (attribute
// omitted, not emitted empty).
func TestDecodeOtherParticipations(t *testing.T) {
	t.Run("nil when absent", func(t *testing.T) {
		if got := decodeOtherParticipations(map[string]any{}); got != nil {
			t.Fatalf("want nil, got %v", got)
		}
	})

	t.Run("round-trip encode -> decode", func(t *testing.T) {
		in := map[string]any{
			"other_participations": []any{
				map[string]any{
					"function": "requestor",
					"performer": map[string]any{
						"name": "de Vries, Peter", "id": "03012345",
						"id_scheme": "AGB", "id_namespace": "lab24", "id_type": "PERSON",
					},
				},
			},
		}
		// encode into an ENTRY node, then decode back.
		parts, err := encodeOtherParticipations(in)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		node := map[string]any{"other_participations": parts}
		got := decodeOtherParticipations(node)
		if len(got) != 1 {
			t.Fatalf("want 1 participation, got %d (%v)", len(got), got)
		}
		p := got[0].(map[string]any)
		if p["function"] != "requestor" {
			t.Errorf("function = %v, want requestor", p["function"])
		}
		perf := p["performer"].(map[string]any)
		for k, want := range map[string]string{
			"name": "de Vries, Peter", "id": "03012345",
			"id_scheme": "AGB", "id_namespace": "lab24", "id_type": "PERSON",
		} {
			if perf[k] != want {
				t.Errorf("performer[%q] = %v, want %v", k, perf[k], want)
			}
		}
	})
}

// PROBE-0799 proves REQ-058 — encodeOtherParticipations/decodeOtherParticipations
// round-trip a performer identified by a HIER_OBJECT_ID (instead of the
// default scheme-bearing GENERIC_ID) together with a participation `mode`.
// Needed so an order's collection performer (an ORGANISATION collection-point,
// referenced by its own platform id rather than an external scheme code)
// round-trips structurally (order-collection-structural, Task 1). The
// HIER_OBJECT_ID branch is triggered by an explicit performer
// `id_type_id: "HIER_OBJECT_ID"`; `mode` follows the standard short/expanded
// coded-value rules (parseCodeField, same as cluster `_code`).
func TestOtherParticipationsHierObjectIDPerformerAndMode(t *testing.T) {
	in := map[string]any{
		"other_participations": []any{
			map[string]any{
				"function": "requesting_organisation",
				"performer": map[string]any{
					"name":         "Lab24 Prikpost Zuid",
					"id":           "PP-ZUID-01",
					"id_type_id":   "HIER_OBJECT_ID",
					"id_namespace": "local",
					"id_type":      "ORGANISATION",
				},
				"mode": map[string]any{
					"code": "face-to-face", "value": "Face-to-face communication", "terminology": "openehr",
				},
			},
		},
	}

	parts, err := encodeOtherParticipations(in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(parts) != 1 {
		t.Fatalf("want 1 participation, got %d (%v)", len(parts), parts)
	}
	p := parts[0].(map[string]any)

	// Encode-side: HIER_OBJECT_ID id (no scheme) + DV_CODED_TEXT mode.
	ext := p["performer"].(map[string]any)["external_ref"].(map[string]any)
	id := ext["id"].(map[string]any)
	if id["_type"] != "HIER_OBJECT_ID" {
		t.Fatalf("performer external_ref.id._type = %v, want HIER_OBJECT_ID", id["_type"])
	}
	if id["value"] != "PP-ZUID-01" {
		t.Fatalf("performer external_ref.id.value = %v", id["value"])
	}
	if _, hasScheme := id["scheme"]; hasScheme {
		t.Fatalf("HIER_OBJECT_ID must not carry scheme, got %v", id)
	}
	mode, ok := p["mode"].(map[string]any)
	if !ok || mode["_type"] != "DV_CODED_TEXT" {
		t.Fatalf("p.mode = %v, want DV_CODED_TEXT", p["mode"])
	}

	// Decode-side: round-trips to the exact same datamap shape.
	node := map[string]any{"other_participations": parts}
	got := decodeOtherParticipations(node)
	if len(got) != 1 {
		t.Fatalf("want 1 decoded participation, got %d (%v)", len(got), got)
	}
	gp := got[0].(map[string]any)

	wantPerformer := map[string]any{
		"name": "Lab24 Prikpost Zuid", "id": "PP-ZUID-01",
		"id_type_id": "HIER_OBJECT_ID", "id_namespace": "local", "id_type": "ORGANISATION",
	}
	perf := gp["performer"].(map[string]any)
	if len(perf) != len(wantPerformer) {
		t.Fatalf("decoded performer = %v, want %v", perf, wantPerformer)
	}
	for k, want := range wantPerformer {
		if perf[k] != want {
			t.Errorf("performer[%q] = %v, want %v", k, perf[k], want)
		}
	}

	wantMode := map[string]any{"code": "face-to-face", "value": "Face-to-face communication", "terminology": "openehr"}
	gotMode, _ := gp["mode"].(map[string]any)
	if len(gotMode) != len(wantMode) {
		t.Fatalf("decoded mode = %v, want %v", gotMode, wantMode)
	}
	for k, want := range wantMode {
		if gotMode[k] != want {
			t.Errorf("mode[%q] = %v, want %v", k, gotMode[k], want)
		}
	}

	if gp["function"] != "requesting_organisation" {
		t.Errorf("function = %v", gp["function"])
	}
}
