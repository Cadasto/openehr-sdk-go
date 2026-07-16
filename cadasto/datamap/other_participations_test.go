package datamap

import "testing"

// PROBE-0797 proves REQ-058 — encodeOtherParticipations turns a datamap
// other_participations array into PARTICIPATION entries whose performer is a
// PARTY_IDENTIFIED with the AGB on external_ref (AQL-queryable), reusing the
// composer external_ref seam. Entries lacking a function or performer are
// dropped; an all-empty input yields nil (attribute omitted).
func TestEncodeOtherParticipations(t *testing.T) {
	t.Run("nil when absent", func(t *testing.T) {
		if got := encodeOtherParticipations(map[string]any{}); got != nil {
			t.Fatalf("want nil, got %v", got)
		}
	})

	t.Run("requestor + organisation -> external_ref", func(t *testing.T) {
		got := encodeOtherParticipations(map[string]any{
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
		got := encodeOtherParticipations(map[string]any{
			"other_participations": []any{
				map[string]any{"performer": map[string]any{"id": "x"}}, // no function
				map[string]any{"function": "requestor"},                // no performer
			},
		})
		if got != nil {
			t.Fatalf("want nil (all dropped), got %v", got)
		}
	})
}
