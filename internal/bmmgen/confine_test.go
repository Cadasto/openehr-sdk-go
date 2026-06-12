package bmmgen

import (
	"path/filepath"
	"testing"
)

func TestConfinePath(t *testing.T) {
	root := filepath.Join("out", "root")
	cases := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{"same dir", root, false},
		{"child", filepath.Join(root, "openehr", "rm"), false},
		{"file in child", filepath.Join(root, "openehr", "rm", "x_gen.go"), false},
		{"parent escape", filepath.Join(root, ".."), true},
		{"traversal escape", filepath.Join(root, "..", "..", "etc"), true},
		{"embedded traversal climbs out", filepath.Join(root, "a", "..", "..", "..", "evil"), true},
		{"absolute outside", filepath.Join(string(filepath.Separator)+"etc", "passwd"), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := confinePath(root, tc.path)
			if tc.wantErr && err == nil {
				t.Fatalf("confinePath(%q, %q) = nil, want error", root, tc.path)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("confinePath(%q, %q) = %v, want nil", root, tc.path, err)
			}
		})
	}
}

func TestSafeFileBase(t *testing.T) {
	good := []string{"data_structures", "rm_quantity", "a", "x_gen"}
	for _, s := range good {
		if !safeFileBase(s) {
			t.Errorf("safeFileBase(%q) = false, want true", s)
		}
	}
	bad := []string{"", ".", "..", "a/b", "../escape", "dir/", `a\b`, "openehr/rm"}
	for _, s := range bad {
		if safeFileBase(s) {
			t.Errorf("safeFileBase(%q) = true, want false", s)
		}
	}
}
