package expectedform

import (
	"strings"
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

// TestREQ052_MemberFilterIsTheSharedWalkers pins that the expected form
// removes exactly the members the shared walker reports, the members
// canjson.WithUnmatched reports (REQ-170), and has no filter of its own. A
// member whose pointer the walker reported is removed, even one the RM
// defines; and a member the RM does not define that the walker did not report
// is an error, not a silent removal, so a disagreement between the two shows.
func TestREQ052_MemberFilterIsTheSharedWalkers(t *testing.T) {
	w, err := newWalker(rminfo.Default, true)
	if err != nil {
		t.Fatalf("newWalker: %v", err)
	}
	obj := func() map[string]any { return map[string]any{"_type": "DV_TEXT", "value": "v"} }

	w.unmatched = map[string]bool{"/value": true}
	got, err := w.object(obj(), "DV_TEXT", "", "")
	if err != nil {
		t.Fatalf("object(reported /value): %v", err)
	}
	if v, ok := got["value"]; ok {
		t.Errorf("DV_TEXT.value = %v, want it removed: the walker reported its pointer", v)
	}

	w.unmatched = map[string]bool{}
	in := obj()
	in["x-a"] = "planted"
	if _, err := w.object(in, "DV_TEXT", "", ""); err == nil || !strings.Contains(err.Error(), "/x-a") {
		t.Errorf("object(x-a not reported) error = %v, want one naming /x-a: the walk does not filter on its own", err)
	}
}
