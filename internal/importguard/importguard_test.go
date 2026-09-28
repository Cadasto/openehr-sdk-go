package importguard_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/importguard"
)

const (
	mod      = "github.com/cadasto/openehr-sdk-go"
	fixtures = mod + "/internal/importguard/testdata"
)

// TestWireLayers pins the one REQ-013 wire-layer list the building-block
// guards share: exactly transport, auth and openehr/client, in a new slice on
// every call. A misspelled, missing or extra entry fails here, and TestMatches
// then checks that the matcher catches each entry.
func TestWireLayers(t *testing.T) {
	t.Parallel()
	want := []string{
		mod + "/transport",
		mod + "/auth",
		mod + "/openehr/client",
	}
	got := importguard.WireLayers()
	if !slices.Equal(got, want) {
		t.Fatalf("WireLayers() = %q, want %q", got, want)
	}
	got[0] = mod + "/changed-by-caller"
	if again := importguard.WireLayers(); !slices.Equal(again, want) {
		t.Errorf("WireLayers() after a caller changed an earlier result = %q, want %q", again, want)
	}
}

// TestScan is the can-fail control for the import-closure walk behind the
// REQ-013 guards of openehr/rm, openehr/serialize/canjson,
// openehr/serialize/canxml and openehr/template. Each fixture under testdata/
// fails if the part of Scan it pins is removed: the direct match, the walk
// into packages of this module, the importer it names, and reading each
// package once.
func TestScan(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		dir  string
		want []importguard.Violation
	}{
		{
			name: "direct import",
			dir:  "testdata/direct",
			want: []importguard.Violation{
				{Importer: fixtures + "/direct", Import: mod + "/transport", Prefix: mod + "/transport"},
			},
		},
		{
			// a reaches b directly and through c. b is read once, so its
			// import is reported once, and under b, not a.
			name: "import one package down",
			dir:  "testdata/indirect",
			want: []importguard.Violation{
				{Importer: fixtures + "/indirect/b", Import: mod + "/auth/basic", Prefix: mod + "/auth"},
			},
		},
		{
			name: "standard library only",
			dir:  "testdata/clean",
			want: nil,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := importguard.Scan(tc.dir, importguard.WireLayers())
			if err != nil {
				t.Fatalf("Scan(%q) error: %v", tc.dir, err)
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("Scan(%q) = %+v, want %+v", tc.dir, got, tc.want)
			}
		})
	}
}

// TestScanRefuses pins the errors that keep a REQ-013 guard from passing
// while checking nothing.
func TestScanRefuses(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		dir     string
		wantErr string // substring the error must carry
	}{
		{name: "no Go files", dir: "testdata/empty", wantErr: "vacuous"},
		{name: "test files only", dir: "testdata/testonly", wantErr: "vacuous"},
		{name: "module package missing", dir: "testdata/missing", wantErr: fixtures + "/nosuchpkg"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := importguard.Scan(tc.dir, importguard.WireLayers())
			if err == nil {
				t.Fatalf("Scan(%q) = %+v, nil; want an error containing %q", tc.dir, got, tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("Scan(%q) error = %q, want it to contain %q", tc.dir, err, tc.wantErr)
			}
		})
	}
}

// TestMatches pins both arms of the REQ-013 matcher for every WireLayers entry
// (the exact path and a sub-package), and the path boundary that keeps
// near-miss names out.
func TestMatches(t *testing.T) {
	t.Parallel()
	tests := []struct {
		imp       string
		wantEntry string // "" when imp must match nothing
	}{
		{imp: mod + "/transport", wantEntry: mod + "/transport"},
		{imp: mod + "/transport/retry", wantEntry: mod + "/transport"},
		{imp: mod + "/auth", wantEntry: mod + "/auth"},
		{imp: mod + "/auth/basic", wantEntry: mod + "/auth"},
		{imp: mod + "/openehr/client", wantEntry: mod + "/openehr/client"},
		{imp: mod + "/openehr/client/ehr", wantEntry: mod + "/openehr/client"},
		{imp: mod + "/authoring"},
		{imp: mod + "/openehr/clientele"},
		{imp: mod + "/openehr/rm/typereg"},
	}
	for _, tc := range tests {
		got, ok := importguard.Matches(tc.imp, importguard.WireLayers())
		if wantOK := tc.wantEntry != ""; got != tc.wantEntry || ok != wantOK {
			t.Errorf("Matches(%q) = %q, %t; want %q, %t", tc.imp, got, ok, tc.wantEntry, wantOK)
		}
	}
}
