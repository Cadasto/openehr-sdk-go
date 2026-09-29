package rmread

import (
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
)

// unmodelledLocatables names the registered LOCATABLE concretes that rmread
// does not model, so ReadSingle reads none of their attributes, the inherited
// archetype_details included. The RM floor therefore does not descend into
// them. A reader added for one of them, or a new LOCATABLE registered without
// one, changes this list and fails TestReadSingle_ArchetypeDetailsParity.
var unmodelledLocatables = []string{"EHR_ACCESS"}

// TestReadSingle_ArchetypeDetailsParity checks that ReadSingle reads the
// inherited LOCATABLE.archetype_details on every LOCATABLE concrete rmread
// models (REQ-112). The RM floor reaches an ARCHETYPED node only through this
// reader, so a LOCATABLE left out of it leaves its archetype_details
// unchecked. The cases come from the live registry: a nil archetype_details
// reads as absent, a set one reads back as the same pointer. A LOCATABLE that
// Handles rejects must stay unreadable, as Handles promises, and must be one
// unmodelledLocatables names.
func TestReadSingle_ArchetypeDetailsParity(t *testing.T) {
	var modelled, unmodelled []string
	for _, name := range typereg.Default.Names() {
		ctor, _ := typereg.Default.Lookup(name)
		v := ctor()
		loc, ok := v.(rm.MutableLocatable)
		if !ok {
			continue
		}
		if !Handles(v) {
			unmodelled = append(unmodelled, name)
			if got, ok := ReadSingle(v, name, "archetype_details"); ok || got != nil {
				t.Errorf("ReadSingle(%T, %q) = (%v, %v), want (nil, false): Handles rejects the type, so no attribute of it is readable", v, "archetype_details", got, ok)
			}
			continue
		}
		modelled = append(modelled, name)
		if got, ok := ReadSingle(v, name, "archetype_details"); ok {
			t.Errorf("ReadSingle(%T with nil archetype_details, %q) = (%v, true), want ok=false", v, "archetype_details", got)
		}
		want := &rm.Archetyped{ArchetypeID: rm.ArchetypeID{Value: "openEHR-EHR-CLUSTER.x.v1"}, RMVersion: "1.1.0"}
		loc.SetArchetypeDetails(want)
		got, ok := ReadSingle(v, name, "archetype_details")
		if gotPtr, isPtr := got.(*rm.Archetyped); !ok || !isPtr || gotPtr != want {
			t.Errorf("ReadSingle(%T with archetype_details set, %q) = (%v, %v), want (%p, true)", v, "archetype_details", got, ok, want)
		}
	}
	if len(modelled) == 0 {
		t.Fatal("the registry yields no modelled LOCATABLE concretes; registrations missing?")
	}
	if !slices.Equal(unmodelled, unmodelledLocatables) {
		t.Errorf("registered LOCATABLE concretes Handles rejects = %v, want %v: update unmodelledLocatables together with the readers", unmodelled, unmodelledLocatables)
	}
}

// TestReadSingle_ArchetypeDetailsValueForm spot-checks the value form of a
// LOCATABLE: ReadSingle reads archetype_details from rm.T as it does from *rm.T
// (REQ-112).
func TestReadSingle_ArchetypeDetailsValueForm(t *testing.T) {
	d := &rm.Archetyped{ArchetypeID: rm.ArchetypeID{Value: "openEHR-EHR-EHR_STATUS.generic.v1"}, RMVersion: "1.1.0"}
	for _, v := range []any{
		rm.EHRStatus{ArchetypeDetails: d},
		rm.Composition{ArchetypeDetails: d},
		rm.Evaluation{ArchetypeDetails: d},
		rm.Person{ArchetypeDetails: d},
		rm.Cluster{ArchetypeDetails: d},
	} {
		got, ok := ReadSingle(v, "", "archetype_details")
		if gotPtr, isPtr := got.(*rm.Archetyped); !ok || !isPtr || gotPtr != d {
			t.Errorf("ReadSingle(%T, %q) = (%v, %v), want (%p, true)", v, "archetype_details", got, ok, d)
		}
	}
}

// TestReadSingle_Archetyped pins the ARCHETYPED reader (REQ-112). archetype_id
// (ARCHETYPE_ID) and rm_version (String) are value-typed: absent, null and
// empty all decode to the same zero value, so the reader reports both present
// and leaves the empty value to the floor's ARCHETYPED check, which reports it
// at archetype_id/value and at rm_version. template_id is an optional pointer,
// present only when set.
func TestReadSingle_Archetyped(t *testing.T) {
	tid := &rm.TemplateID{Value: "ehr_status.v1"}
	full := rm.Archetyped{
		ArchetypeID: rm.ArchetypeID{Value: "openEHR-EHR-EHR_STATUS.generic.v1"},
		RMVersion:   "1.1.0",
		TemplateID:  tid,
	}
	type read struct {
		attr string
		want any
		ok   bool
	}
	cases := []struct {
		name   string
		parent any
		reads  []read
	}{
		{
			name:   "zero, value form",
			parent: rm.Archetyped{},
			reads: []read{
				{attr: "archetype_id", want: rm.ArchetypeID{}, ok: true},
				{attr: "rm_version", want: "", ok: true},
				{attr: "template_id", want: (*rm.TemplateID)(nil), ok: false},
			},
		},
		{
			name:   "zero, pointer form",
			parent: &rm.Archetyped{},
			reads: []read{
				{attr: "archetype_id", want: rm.ArchetypeID{}, ok: true},
				{attr: "rm_version", want: "", ok: true},
				{attr: "template_id", want: (*rm.TemplateID)(nil), ok: false},
			},
		},
		{
			name:   "complete, pointer form",
			parent: &full,
			reads: []read{
				{attr: "archetype_id", want: full.ArchetypeID, ok: true},
				{attr: "rm_version", want: "1.1.0", ok: true},
				{attr: "template_id", want: tid, ok: true},
			},
		},
		{
			name:   "not an ARCHETYPED attribute",
			parent: full,
			reads:  []read{{attr: "archetype_node_id", want: nil, ok: false}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, r := range tc.reads {
				got, ok := ReadSingle(tc.parent, "ARCHETYPED", r.attr)
				if ok != r.ok || got != r.want {
					t.Errorf("ReadSingle(%T, %q) = (%#v, %v), want (%#v, %v)", tc.parent, r.attr, got, ok, r.want, r.ok)
				}
			}
		})
	}
}
