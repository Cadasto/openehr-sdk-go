package datamap

import "testing"

// encodeComposer: string blijft naam-only (backwards compatible); de expanded
// map levert een AQL-queryable external_ref + DV_IDENTIFIERs op.
func TestEncodeComposer(t *testing.T) {
	t.Run("string -> name only", func(t *testing.T) {
		got := encodeComposer("Dr. Jansen")
		if got["name"] != "Dr. Jansen" || got["external_ref"] != nil || got["identifiers"] != nil {
			t.Fatalf("composer = %v", got)
		}
	})
	t.Run("nil -> default", func(t *testing.T) {
		if got := encodeComposer(nil); got["name"] != "Cadasto SDK" {
			t.Fatalf("composer = %v", got)
		}
	})
	t.Run("expanded -> external_ref + identifiers", func(t *testing.T) {
		got := encodeComposer(map[string]any{
			"name": "Drs. A. van der Berg",
			"id":   "cli-123", "id_scheme": "lab24-clinician",
			"identifiers": []any{
				map[string]any{"id": "01000001", "type": "AGB"},
				map[string]any{"id": "tenant-1", "type": "lab24-tenant", "issuer": "lab24"},
			},
		})
		ref, _ := got["external_ref"].(map[string]any)
		if ref == nil {
			t.Fatalf("external_ref ontbreekt: %v", got)
		}
		id, _ := ref["id"].(map[string]any)
		if id["value"] != "cli-123" || id["scheme"] != "lab24-clinician" {
			t.Errorf("ref.id = %v", id)
		}
		ids, _ := got["identifiers"].([]any)
		if len(ids) != 2 {
			t.Fatalf("identifiers = %v", ids)
		}
		first, _ := ids[0].(map[string]any)
		if first["id"] != "01000001" || first["type"] != "AGB" {
			t.Errorf("identifier[0] = %v", first)
		}
	})
}
