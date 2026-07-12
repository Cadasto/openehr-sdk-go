package rm_test

import (
	"errors"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// REQ-120 — identifier parsing and derivation.

func TestParseObjectVersionID(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		wantErr bool
	}{
		{"canonical trunk", "87284370-2D4B-4e3d-A3F3-F303D2F4F34B::cdr.example::1", false},
		{"canonical branch", "abc::sys::1.1.1", false},
		{"no separators", "not-a-version", true},
		{"two parts", "obj::sys", true},
		{"empty segment", "obj::::1", true},
		{"bad version tree", "obj::sys::1.2", true},
		{"trailing extra", "obj::sys::1::extra", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := rm.ParseObjectVersionID(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ParseObjectVersionID(%q) = nil error, want error", tc.in)
				}
				if !errors.Is(err, rm.ErrMalformedID) {
					t.Errorf("error %v is not ErrMalformedID", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseObjectVersionID(%q) = %v, want nil", tc.in, err)
			}
		})
	}
}

func TestParseObjectVersionIDRoundTrip(t *testing.T) {
	const canonical = "87284370-2D4B-4e3d-A3F3-F303D2F4F34B::cdr.example::2.1.4"
	parsed, err := rm.ParseObjectVersionID(canonical)
	if err != nil {
		t.Fatalf("ParseObjectVersionID = %v", err)
	}
	if parsed.Value != canonical {
		t.Errorf("Value = %q, want %q", parsed.Value, canonical)
	}
	if got := rm.UIDValue(parsed.ObjectID()); got != "87284370-2D4B-4e3d-A3F3-F303D2F4F34B" {
		t.Errorf("ObjectID = %q", got)
	}
	if got := rm.UIDValue(parsed.CreatingSystemID()); got != "cdr.example" {
		t.Errorf("CreatingSystemID = %q", got)
	}
	if got := parsed.VersionTreeID().Value; got != "2.1.4" {
		t.Errorf("VersionTreeID = %q", got)
	}
	if !parsed.IsBranch() {
		t.Error("IsBranch = false, want true")
	}
}

func TestParseVersionTreeIDRoundTrip(t *testing.T) {
	parsed, err := rm.ParseVersionTreeID("2.1.4")
	if err != nil {
		t.Fatalf("ParseVersionTreeID = %v", err)
	}
	if parsed.Value != "2.1.4" {
		t.Errorf("Value = %q", parsed.Value)
	}
	if parsed.TrunkVersion() != "2" || parsed.BranchNumber() != "1" || parsed.BranchVersion() != "4" {
		t.Errorf("decomposition = %q/%q/%q", parsed.TrunkVersion(), parsed.BranchNumber(), parsed.BranchVersion())
	}
}

func TestObjectVersionIDDerivation(t *testing.T) {
	ovid := rm.ObjectVersionID{Value: "87284370-2D4B-4e3d-A3F3-F303D2F4F34B::cdr.example::2.1.4"}

	if got := rm.UIDValue(ovid.ObjectID()); got != "87284370-2D4B-4e3d-A3F3-F303D2F4F34B" {
		t.Errorf("ObjectID = %q", got)
	}
	if _, ok := ovid.ObjectID().(rm.Uuid); !ok {
		t.Errorf("ObjectID = %T, want rm.Uuid", ovid.ObjectID())
	}
	if got := rm.UIDValue(ovid.CreatingSystemID()); got != "cdr.example" {
		t.Errorf("CreatingSystemID = %q", got)
	}
	if _, ok := ovid.CreatingSystemID().(rm.InternetID); !ok {
		t.Errorf("CreatingSystemID = %T, want rm.InternetID", ovid.CreatingSystemID())
	}
	if got := ovid.VersionTreeID().Value; got != "2.1.4" {
		t.Errorf("VersionTreeID = %q", got)
	}
	if !ovid.IsBranch() {
		t.Error("IsBranch = false, want true for 3-part version tree")
	}
	// UID_BASED_ID inherited semantics: root == object_id; extension is
	// the joined remainder after the first "::".
	if got := rm.UIDValue(ovid.Root()); got != "87284370-2D4B-4e3d-A3F3-F303D2F4F34B" {
		t.Errorf("Root = %q", got)
	}
	if got := ovid.Extension(); got != "cdr.example::2.1.4" {
		t.Errorf("Extension = %q, want cdr.example::2.1.4", got)
	}
	if !ovid.HasExtension() {
		t.Error("HasExtension = false, want true")
	}

	trunk := rm.ObjectVersionID{Value: "obj::sys::1"}
	if trunk.IsBranch() {
		t.Error("trunk IsBranch = true, want false")
	}
}

func TestHierObjectIDDerivation(t *testing.T) {
	h := rm.HierObjectID{Value: "1.2.840.113619.2.62::extension-part"}
	if got := rm.UIDValue(h.Root()); got != "1.2.840.113619.2.62" {
		t.Errorf("Root = %q", got)
	}
	if _, ok := h.Root().(rm.ISOOID); !ok {
		t.Errorf("Root = %T, want rm.ISOOID", h.Root())
	}
	if got := h.Extension(); got != "extension-part" {
		t.Errorf("Extension = %q", got)
	}
	if !h.HasExtension() {
		t.Error("HasExtension = false, want true")
	}

	bare := rm.HierObjectID{Value: "uk.nhs.scotland"}
	if bare.HasExtension() {
		t.Error("HasExtension = true, want false for no '::'")
	}
	if _, ok := bare.Root().(rm.InternetID); !ok {
		t.Errorf("Root = %T, want rm.InternetID", bare.Root())
	}
}

func TestVersionTreeID(t *testing.T) {
	trunk := rm.VersionTreeID{Value: "1"}
	if trunk.TrunkVersion() != "1" || trunk.BranchNumber() != "" || trunk.BranchVersion() != "" {
		t.Errorf("trunk decomposition wrong: %q/%q/%q", trunk.TrunkVersion(), trunk.BranchNumber(), trunk.BranchVersion())
	}
	if trunk.IsBranch() {
		t.Error("trunk IsBranch = true")
	}
	if !trunk.IsFirst() {
		t.Error("trunk(1) IsFirst = false")
	}

	branch := rm.VersionTreeID{Value: "2.1.4"}
	if branch.TrunkVersion() != "2" || branch.BranchNumber() != "1" || branch.BranchVersion() != "4" {
		t.Errorf("branch decomposition wrong: %q/%q/%q", branch.TrunkVersion(), branch.BranchNumber(), branch.BranchVersion())
	}
	if !branch.IsBranch() {
		t.Error("branch IsBranch = false")
	}
	if branch.IsFirst() {
		t.Error("branch(2.1.4) IsFirst = true")
	}

	for _, bad := range []string{"1.2", "0", "x", "1.0.1", ""} {
		if _, err := rm.ParseVersionTreeID(bad); err == nil {
			t.Errorf("ParseVersionTreeID(%q) = nil error, want error", bad)
		}
	}
	if _, err := rm.ParseVersionTreeID("1.2.3"); err != nil {
		t.Errorf("ParseVersionTreeID(1.2.3) = %v, want nil", err)
	}
}

func TestArchetypeID(t *testing.T) {
	a := rm.ArchetypeID{Value: "openEHR-EHR-OBSERVATION.lab_result-cholesterol.v1"}
	checks := map[string]struct{ got, want string }{
		"QualifiedRMEntity": {a.QualifiedRMEntity(), "openEHR-EHR-OBSERVATION"},
		"RMOriginator":      {a.RMOriginator(), "openEHR"},
		"RMName":            {a.RMName(), "EHR"},
		"RMEntity":          {a.RMEntity(), "OBSERVATION"},
		"DomainConcept":     {a.DomainConcept(), "lab_result-cholesterol"},
		"Specialisation":    {a.Specialisation(), "cholesterol"},
		"VersionID":         {a.VersionID(), "1"},
	}
	for name, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %q, want %q", name, c.got, c.want)
		}
	}

	unspecialised := rm.ArchetypeID{Value: "openEHR-EHR-OBSERVATION.lab_result.v1"}
	if got := unspecialised.Specialisation(); got != "" {
		t.Errorf("unspecialised Specialisation = %q, want empty", got)
	}

	if _, err := rm.ParseArchetypeID(a.Value); err != nil {
		t.Errorf("ParseArchetypeID(valid) = %v", err)
	}
	for _, bad := range []string{"no-dots", "openEHR-EHR.concept.v1", "openEHR-EHR-OBSERVATION.concept.1", "openEHR-EHR-OBSERVATION.concept"} {
		if _, err := rm.ParseArchetypeID(bad); err == nil {
			t.Errorf("ParseArchetypeID(%q) = nil error, want error", bad)
		}
	}
}

func TestTerminologyID(t *testing.T) {
	withVer := rm.TerminologyID{Value: "ICD10AM(2006)"}
	if withVer.Name() != "ICD10AM" || withVer.VersionID() != "2006" {
		t.Errorf("withVer = %q/%q", withVer.Name(), withVer.VersionID())
	}
	bare := rm.TerminologyID{Value: "SNOMED-CT"}
	if bare.Name() != "SNOMED-CT" || bare.VersionID() != "" {
		t.Errorf("bare = %q/%q", bare.Name(), bare.VersionID())
	}
	if _, err := rm.ParseTerminologyID("ICD10AM(2006)"); err != nil {
		t.Errorf("ParseTerminologyID(valid) = %v", err)
	}
	for _, bad := range []string{"", "name(unclosed", "name)"} {
		if _, err := rm.ParseTerminologyID(bad); err == nil {
			t.Errorf("ParseTerminologyID(%q) = nil error, want error", bad)
		}
	}
}

func TestLocatableRefAsURI(t *testing.T) {
	path := "/items[at0002]"
	ref := rm.LocatableRef{
		ID:   rm.ObjectVersionID{Value: "87284370-2D4B-4e3d-A3F3-F303D2F4F34B::ABC::1"},
		Path: &path,
	}
	ref.Namespace = "ehr"
	if got := ref.AsURI(); got != "ehr:87284370-2D4B-4e3d-A3F3-F303D2F4F34B::ABC::1/items[at0002]" {
		t.Errorf("AsURI = %q", got)
	}

	noPath := rm.LocatableRef{ID: rm.HierObjectID{Value: "abc"}}
	noPath.Namespace = "ehr"
	if got := noPath.AsURI(); got != "ehr:abc" {
		t.Errorf("AsURI(no path) = %q", got)
	}
}

func TestObjectIDValue(t *testing.T) {
	tests := []struct {
		name   string
		id     rm.ObjectID
		want   string
		wantOK bool
	}{
		{"HierObjectID value", rm.HierObjectID{Value: "abc"}, "abc", true},
		{"HierObjectID pointer", &rm.HierObjectID{Value: "abc"}, "abc", true},
		{"HierObjectID nil pointer", (*rm.HierObjectID)(nil), "", false},
		{"HierObjectID empty value", rm.HierObjectID{}, "", true},

		{"ObjectVersionID value", rm.ObjectVersionID{Value: "obj::sys::1"}, "obj::sys::1", true},
		{"ObjectVersionID pointer", &rm.ObjectVersionID{Value: "obj::sys::1"}, "obj::sys::1", true},
		{"ObjectVersionID nil pointer", (*rm.ObjectVersionID)(nil), "", false},

		{"GenericID value", rm.GenericID{Value: "g1", Scheme: "local"}, "g1", true},
		{"GenericID pointer", &rm.GenericID{Value: "g1", Scheme: "local"}, "g1", true},
		{"GenericID nil pointer", (*rm.GenericID)(nil), "", false},

		{"ArchetypeID value", rm.ArchetypeID{Value: "openEHR-EHR-OBSERVATION.lab_result.v1"}, "openEHR-EHR-OBSERVATION.lab_result.v1", true},
		{"ArchetypeID pointer", &rm.ArchetypeID{Value: "openEHR-EHR-OBSERVATION.lab_result.v1"}, "openEHR-EHR-OBSERVATION.lab_result.v1", true},
		{"ArchetypeID nil pointer", (*rm.ArchetypeID)(nil), "", false},

		{"TemplateID value", rm.TemplateID{Value: "tmpl.v1"}, "tmpl.v1", true},
		{"TemplateID pointer", &rm.TemplateID{Value: "tmpl.v1"}, "tmpl.v1", true},
		{"TemplateID nil pointer", (*rm.TemplateID)(nil), "", false},

		{"TerminologyID value", rm.TerminologyID{Value: "SNOMED-CT"}, "SNOMED-CT", true},
		{"TerminologyID pointer", &rm.TerminologyID{Value: "SNOMED-CT"}, "SNOMED-CT", true},
		{"TerminologyID nil pointer", (*rm.TerminologyID)(nil), "", false},

		// ObjectID has an unexported discriminator method. The default arm
		// is reached by a nil interface value, or by an external type that
		// embeds one of the concrete IDs (inheriting the promoted
		// isObjectID() method, but as a distinct type it matches no case).
		{"nil ObjectID (unrecognized)", nil, "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := rm.ObjectIDValue(tc.id)
			if got != tc.want || ok != tc.wantOK {
				t.Errorf("ObjectIDValue(%#v) = (%q, %v), want (%q, %v)", tc.id, got, ok, tc.want, tc.wantOK)
			}
		})
	}
}

func TestIdentifierAccessorsNoPanicOnGarbage(t *testing.T) {
	// REQ-120 best-effort derivation methods must never panic, however
	// malformed the input. (A panic here fails the test outright.)
	for _, g := range []string{"", "!!!", "::::", "no-delimiters", "a.b", "x::"} {
		ov := rm.ObjectVersionID{Value: g}
		_, _, _, _, _ = ov.ObjectID(), ov.CreatingSystemID(), ov.VersionTreeID(), ov.IsBranch(), ov.Extension()
		a := rm.ArchetypeID{Value: g}
		_, _, _, _, _, _ = a.RMOriginator(), a.RMName(), a.RMEntity(), a.DomainConcept(), a.Specialisation(), a.VersionID()
		term := rm.TerminologyID{Value: g}
		_, _ = term.Name(), term.VersionID()
		h := rm.HierObjectID{Value: g}
		_, _, _ = h.Root(), h.Extension(), h.HasExtension()
		vt := rm.VersionTreeID{Value: g}
		_, _, _, _ = vt.TrunkVersion(), vt.BranchNumber(), vt.IsBranch(), vt.IsFirst()
	}
}
