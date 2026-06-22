package ehr

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// TestVersionUIDDelegatesToCanonicalParser asserts the client helpers
// agree with the canonical rm.ObjectVersionID parser (REQ-120): no
// duplicate lexical logic.
func TestVersionUIDDelegatesToCanonicalParser(t *testing.T) {
	const raw = "87284370-2D4B-4e3d-A3F3-F303D2F4F34B::cdr.example::2"
	v := VersionUID(raw)
	ovID, err := rm.ParseObjectVersionID(raw)
	if err != nil {
		t.Fatalf("ParseObjectVersionID: %v", err)
	}
	if got, want := string(v.VersionedObjectID()), rm.UIDValue(ovID.ObjectID()); got != want {
		t.Errorf("VersionedObjectID = %q, canonical = %q", got, want)
	}
	if got, want := v.CreatingSystemID(), rm.UIDValue(ovID.CreatingSystemID()); got != want {
		t.Errorf("CreatingSystemID = %q, canonical = %q", got, want)
	}
	if got, want := v.VersionNumber(), ovID.VersionTreeID().Value; got != want {
		t.Errorf("VersionNumber = %q, canonical = %q", got, want)
	}
}

func TestExtractVersionUIDFromLocation(t *testing.T) {
	tests := []struct {
		name string
		loc  string
		want VersionUID
	}{
		{"empty", "", ""},
		{"relative", "/ehr/x/composition/uid::sys::1", "uid::sys::1"},
		{"relative trailing slash", "/ehr/x/composition/uid::sys::1/", "uid::sys::1"},
		{"absolute with query", "https://cdr.example/openehr/v1/ehr/x/composition/uid::sys::1?version=2", "uid::sys::1"},
		{"absolute with fragment", "https://cdr.example/ehr/x/composition/abc::x::3#frag", "abc::x::3"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := extractVersionUIDFromLocation(tc.loc); got != tc.want {
				t.Errorf("extractVersionUIDFromLocation(%q) = %q, want %q", tc.loc, got, tc.want)
			}
		})
	}
}

func TestNewVersionMetadataAbsoluteLocation(t *testing.T) {
	meta := NewVersionMetadata(&transport.Metadata{
		Location: "https://host/openehr/v1/ehr/e/composition/vo::sys::1?foo=bar",
	})
	if meta == nil {
		t.Fatal("expected non-nil metadata")
	}
	if meta.VersionUID != "vo::sys::1" {
		t.Errorf("VersionUID = %q, want vo::sys::1", meta.VersionUID)
	}
}

// TestVersionUIDMalformedSegments asserts the stricter (canonical-parser)
// behaviour: a non-three-part version-uid yields empty segments (REQ-120).
func TestVersionUIDMalformedSegments(t *testing.T) {
	for _, raw := range []string{"uid::sys", "uid::sys::0", "uid::sys::1.1", "garbage", ""} {
		v := VersionUID(raw)
		if v.VersionedObjectID() != "" || v.CreatingSystemID() != "" || v.VersionNumber() != "" {
			t.Errorf("%q: want empty segments, got %q/%q/%q",
				raw, v.VersionedObjectID(), v.CreatingSystemID(), v.VersionNumber())
		}
	}
}
