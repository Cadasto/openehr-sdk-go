package expectedform

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm/rminfo"
)

// TestREQ052_EmptyContainerRulesCheckTheirClass pins that the two empty-
// container rules hold only for the class REQ-052 names: an empty "mappings"
// is dropped only where DV_TEXT declares it, and an empty byte list only where
// DV_MULTIMEDIA declares it ("data", "integrity_check"). The pinned RM has no
// other class with such an attribute, so a synthetic one carries both, and
// both stay as written. Drop either class check and this test reddens.
func TestREQ052_EmptyContainerRulesCheckTheirClass(t *testing.T) {
	data := map[string]rminfo.ClassMeta{
		"BLOB": {
			Attributes: map[string]rminfo.AttrMeta{
				"data":     {TypeName: "Octet", Container: true, DeclaredIn: "BLOB"},
				"mappings": {TypeName: "NOTE", Container: true, DeclaredIn: "BLOB"},
			},
			AttrOrder: []string{"data", "mappings"},
		},
		"NOTE": {Attributes: map[string]rminfo.AttrMeta{}, AttrOrder: []string{}},
	}
	absence := map[string]rminfo.AbsenceReason{"Octet": rminfo.AbsencePrimitive}
	w, err := newWalker(rminfo.NewWithAbsence(data, absence), true)
	if err != nil {
		t.Fatalf("newWalker: %v", err)
	}
	got, err := w.object(map[string]any{"data": "", "mappings": []any{}}, "BLOB", "", "")
	if err != nil {
		t.Fatalf("object: %v", err)
	}
	if v, ok := got["data"]; !ok || v != "" {
		t.Errorf("BLOB.data = %v (present %v), want the empty string kept: the empty-bytes rule binds DV_MULTIMEDIA only", v, ok)
	}
	if v, ok := got["mappings"].([]any); !ok || len(v) != 0 {
		t.Errorf("BLOB.mappings = %v, want the empty list kept: the empty-mappings rule binds DV_TEXT only", got["mappings"])
	}
}
