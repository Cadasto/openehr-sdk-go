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

// TestScan is the can-fail control for the import-closure walk every REQ-013
// building-block guard runs. Each fixture under testdata/ fails if the part of
// Scan it pins is removed: the direct match, the walk into packages of this
// module, the importer it names, reading each package once, reading the files
// another build compiles (tagged, cgoonly, hostnone), and skipping the ones
// no build compiles (tagged).
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
			// tag.go, notlinux.go, a_plan9.go and other.go (package b) count.
			// gen.go (ignore tag) and a_plan9_test.go (a test) do not; each is
			// dropped by one rule only, and would add openehr/client/ehr or
			// transport/retry.
			name: "files another build compiles",
			dir:  "testdata/tagged",
			want: []importguard.Violation{
				{Importer: fixtures + "/tagged", Import: mod + "/auth", Prefix: mod + "/auth"},
				{Importer: fixtures + "/tagged", Import: mod + "/auth/basic", Prefix: mod + "/auth"},
				{Importer: fixtures + "/tagged", Import: mod + "/openehr/client", Prefix: mod + "/openehr/client"},
				{Importer: fixtures + "/tagged", Import: mod + "/transport", Prefix: mod + "/transport"},
			},
		},
		{
			// This machine compiles none of the three files, and the first
			// by name is package main: the package clause decides nothing.
			name: "no file this machine compiles",
			dir:  "testdata/hostnone",
			want: []importguard.Violation{
				{Importer: fixtures + "/hostnone", Import: mod + "/auth/basic", Prefix: mod + "/auth"},
				{Importer: fixtures + "/hostnone", Import: mod + "/transport", Prefix: mod + "/transport"},
			},
		},
		{
			name: "cgo-only package",
			dir:  "testdata/cgoonly",
			want: []importguard.Violation{
				{Importer: fixtures + "/cgoonly", Import: mod + "/transport", Prefix: mod + "/transport"},
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

// TestScanStd is the can-fail control for the standard-library walk behind
// the REQ-013 and REQ-045 guard of openehr/bmm: net/http reached only through
// expvar is found by ScanStd, and named under expvar, while Scan, which does
// not walk the standard library, cannot see it.
func TestScanStd(t *testing.T) {
	t.Parallel()
	const dir = "testdata/stdreach"
	forbidden := []string{"net/http"}
	want := []importguard.Violation{{Importer: "expvar", Import: "net/http", Prefix: "net/http"}}
	got, err := importguard.ScanStd(dir, forbidden)
	if err != nil {
		t.Fatalf("ScanStd(%q) error: %v", dir, err)
	}
	if !slices.Equal(got, want) {
		t.Errorf("ScanStd(%q, %q) = %+v, want %+v", dir, forbidden, got, want)
	}
	got, err = importguard.Scan(dir, forbidden)
	if err != nil {
		t.Fatalf("Scan(%q) error: %v", dir, err)
	}
	if len(got) != 0 {
		t.Errorf("Scan(%q, %q) = %+v, want none: Scan does not walk the standard library", dir, forbidden, got)
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
		{name: "ignore-tagged files only", dir: "testdata/ignoreonly", wantErr: "vacuous"},
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

// TestImports is the can-fail control for the own-imports reader behind the
// package-specific REQ-013 rules (the openehr/serialize ban, the allow-lists,
// the openehr/validation ban). It pins that Imports reads the same files as
// Scan and does not follow what they import: indirect lists b and c but not
// b's import of auth/basic.
func TestImports(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		dir  string
		want []string
	}{
		{name: "direct import", dir: "testdata/direct", want: []string{mod + "/transport"}},
		{
			name: "own imports only",
			dir:  "testdata/indirect",
			want: []string{fixtures + "/indirect/b", fixtures + "/indirect/c"},
		},
		{
			name: "files another build compiles",
			dir:  "testdata/tagged",
			want: []string{mod + "/auth", mod + "/auth/basic", mod + "/openehr/client", mod + "/transport", "strings"},
		},
		{
			name: "no file this machine compiles",
			dir:  "testdata/hostnone",
			want: []string{mod + "/auth/basic", mod + "/transport", "strings"},
		},
		{name: "cgo-only package", dir: "testdata/cgoonly", want: []string{"C", mod + "/transport"}},
		{name: "standard library only", dir: "testdata/clean", want: []string{"strings"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := importguard.Imports(tc.dir)
			if err != nil {
				t.Fatalf("Imports(%q) error: %v", tc.dir, err)
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("Imports(%q) = %q, want %q", tc.dir, got, tc.want)
			}
		})
	}
}

// TestImportsRefuses pins the errors that keep a guard built on Imports from
// passing while checking nothing.
func TestImportsRefuses(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		dir     string
		wantErr string // substring the error must carry
	}{
		{name: "no Go files", dir: "testdata/empty", wantErr: "vacuous"},
		{name: "test files only", dir: "testdata/testonly", wantErr: "vacuous"},
		{name: "ignore-tagged files only", dir: "testdata/ignoreonly", wantErr: "vacuous"},
		{name: "no such directory", dir: "testdata/nosuchdir", wantErr: "nosuchdir"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := importguard.Imports(tc.dir)
			if err == nil {
				t.Fatalf("Imports(%q) = %q, nil; want an error containing %q", tc.dir, got, tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("Imports(%q) error = %q, want it to contain %q", tc.dir, err, tc.wantErr)
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

// TestStandard is the can-fail control for the classifier behind the REQ-013
// standard-library allow-lists (openehr/terminology, openehr/aql/contain,
// openehr/aql/internal/semcheck): standard-library paths pass, and module
// paths of this module or any other, and the cgo pseudo-package, do not.
func TestStandard(t *testing.T) {
	t.Parallel()
	tests := []struct {
		imp  string
		want bool
	}{
		{imp: "iter", want: true},
		{imp: "slices", want: true},
		{imp: "go/build", want: true},
		{imp: "encoding/json/v2", want: true},
		{imp: "uuid", want: true},
		{imp: "C", want: false},
		{imp: mod + "/openehr/rm", want: false},
		{imp: "golang.org/x/text/unicode/norm", want: false},
		{imp: "example.com/foo", want: false},
	}
	for _, tc := range tests {
		if got := importguard.Standard(tc.imp); got != tc.want {
			t.Errorf("Standard(%q) = %t, want %t", tc.imp, got, tc.want)
		}
	}
}
