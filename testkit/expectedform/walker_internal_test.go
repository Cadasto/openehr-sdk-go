package expectedform

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm/rminfo"
)

// TestREQ052_EmptyListRuleChecksOptionality pins that the empty-list rule
// turns on whether the attribute is optional, not on its class or name: on a
// synthetic class, an empty list and an empty byte list an optional attribute
// holds are dropped, as the encoder leaves them out, and the same two held by
// mandatory attributes are kept, as the encoder writes them. The class and
// attribute names are none the pinned RM uses, so a rule bound to DV_TEXT
// mappings or DV_MULTIMEDIA data keeps the optional pair, and a rule that
// ignores optionality drops the mandatory pair; either reddens this test.
func TestREQ052_EmptyListRuleChecksOptionality(t *testing.T) {
	data := map[string]rminfo.ClassMeta{
		"BLOB": {
			Attributes: map[string]rminfo.AttrMeta{
				"bytes":     {TypeName: "Octet", Container: true, DeclaredIn: "BLOB"},
				"notes":     {TypeName: "NOTE", Container: true, DeclaredIn: "BLOB"},
				"bytes_req": {TypeName: "Octet", Container: true, Required: true, DeclaredIn: "BLOB"},
				"notes_req": {TypeName: "NOTE", Container: true, Required: true, DeclaredIn: "BLOB"},
			},
			AttrOrder: []string{"bytes", "notes", "bytes_req", "notes_req"},
		},
		"NOTE": {Attributes: map[string]rminfo.AttrMeta{}, AttrOrder: []string{}},
	}
	absence := map[string]rminfo.AbsenceReason{"Octet": rminfo.AbsencePrimitive}
	w, err := newWalker(rminfo.NewWithAbsence(data, absence), true)
	if err != nil {
		t.Fatalf("newWalker: %v", err)
	}
	got, err := w.object(map[string]any{"bytes": "", "notes": []any{}, "bytes_req": "", "notes_req": []any{}}, "BLOB", "", "")
	if err != nil {
		t.Fatalf("object: %v", err)
	}
	for _, name := range []string{"bytes", "notes"} {
		if v, ok := got[name]; ok {
			t.Errorf("BLOB.%s = %v, want it removed: an empty list an optional attribute holds reads as absent", name, v)
		}
	}
	if v, ok := got["bytes_req"]; !ok || v != "" {
		t.Errorf("BLOB.bytes_req = %v (present %v), want the empty string kept: a mandatory list is written", v, ok)
	}
	if v, ok := got["notes_req"].([]any); !ok || len(v) != 0 {
		t.Errorf("BLOB.notes_req = %v, want the empty list kept: a mandatory list is written", got["notes_req"])
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

// TestREQ052_NullMandatoryDoubleIsZero pins the Double arm of the null rule:
// a null member declared mandatory with a Double type is expected as 0, the
// zero the encoder writes for it, and an optional one is removed. The pinned
// RM declares no mandatory Double, so a synthetic class carries both. Drop
// Double from the rule and the mandatory member is removed, reddening this
// test.
func TestREQ052_NullMandatoryDoubleIsZero(t *testing.T) {
	data := map[string]rminfo.ClassMeta{
		"MEASURE": {
			Attributes: map[string]rminfo.AttrMeta{
				"value":  {TypeName: "Double", Required: true, DeclaredIn: "MEASURE"},
				"spread": {TypeName: "Double", DeclaredIn: "MEASURE"},
			},
			AttrOrder: []string{"value", "spread"},
		},
	}
	absence := map[string]rminfo.AbsenceReason{"Double": rminfo.AbsencePrimitive}
	w, err := newWalker(rminfo.NewWithAbsence(data, absence), true)
	if err != nil {
		t.Fatalf("newWalker: %v", err)
	}
	got, err := w.object(map[string]any{"value": nil, "spread": nil}, "MEASURE", "", "")
	if err != nil {
		t.Fatalf("object: %v", err)
	}
	if v, ok := got["value"]; !ok || v != json.Number("0") {
		t.Errorf("MEASURE.value = %v (present %v), want 0: a null mandatory Double is the zero the encoder writes", v, ok)
	}
	if v, ok := got["spread"]; ok {
		t.Errorf("MEASURE.spread = %v, want it removed: a null optional Double reads as absent", v)
	}
}
