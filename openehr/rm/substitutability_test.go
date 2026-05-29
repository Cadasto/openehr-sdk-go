package rm

import (
	"encoding/json"
	"strings"
	"testing"
)

// REQ-058 — Substitutability on decode. A `DV_CODED_TEXT` payload MUST be
// accepted in any slot typed as `DV_TEXT` (its supertype) per openEHR RM
// §`data_types.text`: "Since `DV_CODED_TEXT` is a subtype of `DV_TEXT`,
// it can be used in place of it." The Go-side `DVText` struct retains
// only the supertype's fields; the descendant-specific `defining_code`
// is dropped silently (lossy on the Go decode boundary; lossless on the
// wire — the CDR still stores the full DV_CODED_TEXT). Fidelity round-
// trip via the `DataValueText` interface is tracked separately in the
// follow-up Phase-2 generator work.

func TestDVText_AcceptsDVCodedTextPayload(t *testing.T) {
	payload := []byte(`{
		"_type": "DV_CODED_TEXT",
		"value": "Body temperature",
		"defining_code": {
			"_type": "CODE_PHRASE",
			"terminology_id": { "_type": "TERMINOLOGY_ID", "value": "SNOMED-CT" },
			"code_string": "386725007"
		}
	}`)

	var dv DVText
	if err := json.Unmarshal(payload, &dv); err != nil {
		t.Fatalf("expected DV_CODED_TEXT payload to land in DVText slot, got %v", err)
	}
	if dv.Value != "Body temperature" {
		t.Errorf("Value = %q, want %q", dv.Value, "Body temperature")
	}
}

func TestDVText_RejectsUnrelatedType(t *testing.T) {
	// DV_QUANTITY is NOT a subtype of DV_TEXT — substitutability MUST NOT
	// extend to unrelated DataValue siblings.
	payload := []byte(`{"_type":"DV_QUANTITY","magnitude":4.7,"units":"umol/L"}`)
	var dv DVText
	err := json.Unmarshal(payload, &dv)
	if err == nil {
		t.Fatal("expected error for DV_QUANTITY in DV_TEXT slot, got nil")
	}
	if !strings.Contains(err.Error(), "DV_QUANTITY") {
		t.Errorf("error should mention the offending type, got: %v", err)
	}
}

// Cluster.Name is typed as the DataValueText marker interface (REQ-058
// Phase 2). A composition carrying a DV_CODED_TEXT in the cluster name
// slot MUST decode losslessly: the concrete type lands as `*DVCodedText`
// with `defining_code` intact, and DVTextValue returns the display.
func TestCluster_AcceptsDVCodedTextInName(t *testing.T) {
	payload := []byte(`{
		"_type": "CLUSTER",
		"archetype_node_id": "at0096",
		"name": {
			"_type": "DV_CODED_TEXT",
			"value": "Kreatinine",
			"defining_code": {
				"_type": "CODE_PHRASE",
				"terminology_id": { "_type": "TERMINOLOGY_ID", "value": "local" },
				"code_string": "KREA"
			}
		},
		"items": []
	}`)

	var c Cluster
	if err := json.Unmarshal(payload, &c); err != nil {
		t.Fatalf("expected DV_CODED_TEXT in Cluster.Name to decode, got %v", err)
	}
	if got := DVTextValue(c.Name); got != "Kreatinine" {
		t.Errorf("DVTextValue(c.Name) = %q, want %q", got, "Kreatinine")
	}
	if c.ArchetypeNodeID != "at0096" {
		t.Errorf("ArchetypeNodeID = %q, want %q", c.ArchetypeNodeID, "at0096")
	}
	// Lossless round-trip: concrete type is *DVCodedText, defining_code
	// preserved (the entire reason for Phase 2 — without interface-typing
	// the DV_CODED_TEXT payload would land in *DVText and lose
	// `defining_code` silently on re-marshal).
	coded, ok := c.Name.(*DVCodedText)
	if !ok {
		t.Fatalf("Cluster.Name = %T, want *DVCodedText (lossless dispatch)", c.Name)
	}
	if coded.DefiningCode.CodeString != "KREA" {
		t.Errorf("defining_code.code_string = %q, want %q", coded.DefiningCode.CodeString, "KREA")
	}
	if coded.DefiningCode.TerminologyID.Value != "local" {
		t.Errorf("defining_code.terminology_id = %q, want %q", coded.DefiningCode.TerminologyID.Value, "local")
	}
	// Re-marshal preserves the DV_CODED_TEXT — the wire shape stays the
	// same end-to-end (critical for CDR write/read round-trip).
	out, err := json.Marshal(&c)
	if err != nil {
		t.Fatalf("re-marshal: %v", err)
	}
	if !strings.Contains(string(out), `"DV_CODED_TEXT"`) || !strings.Contains(string(out), `"KREA"`) {
		t.Errorf("re-marshalled composition lost coded info: %s", out)
	}
}
