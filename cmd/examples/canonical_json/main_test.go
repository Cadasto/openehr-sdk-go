package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// TestRunTakesACompositionPath checks the optional argument: a named file is
// decoded instead of the vendored fixture, one without a name is printed
// rather than crashing the program, a missing one is reported, and a second
// argument is refused. The zero-argument run is held to the catalogue by the
// transcript test in cmd/examples.
func TestRunTakesACompositionPath(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "missing.json")
	// The RM makes name mandatory, but canjson decodes a document without
	// one, so a file the caller names may leave it out.
	nameless := filepath.Join(dir, "nameless.json")
	if err := os.WriteFile(nameless, []byte(`{"_type": "COMPOSITION"}`), 0o600); err != nil {
		t.Fatalf("write %s: %v", nameless, err)
	}
	tests := []struct {
		name    string
		args    []string
		wantErr string // empty for success
	}{
		{name: "another composition", args: []string{fixtures.CompositionJSON("vital_signs")}},
		{name: "composition without a name", args: []string{nameless}},
		{name: "missing file", args: []string{missing}, wantErr: missing},
		{name: "two paths", args: []string{"a.json", "b.json"}, wantErr: "usage"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := run(tc.args)
			switch {
			case tc.wantErr == "" && err != nil:
				t.Errorf("run(%q): %v, want no error", tc.args, err)
			case tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)):
				t.Errorf("run(%q): error %v, want one naming %q", tc.args, err, tc.wantErr)
			}
		})
	}
}
