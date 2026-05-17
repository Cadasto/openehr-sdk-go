package ehr

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/transport"
)

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
