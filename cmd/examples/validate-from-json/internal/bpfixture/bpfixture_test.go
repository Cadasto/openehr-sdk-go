package bpfixture

import (
	"strings"
	"testing"
)

// TestCheckRefusesAnInvalidDocument holds the generator to its promise of
// never writing an invalid fixture: check passes the document Document
// builds, and refuses one that does not decode or misses what the Reference
// Model and the template require, listing each issue.
func TestCheckRefusesAnInvalidDocument(t *testing.T) {
	t.Parallel()

	compiled, err := compileVitalSigns()
	if err != nil {
		t.Fatalf("compileVitalSigns() error = %v", err)
	}
	valid, err := Document()
	if err != nil {
		t.Fatalf("Document() error = %v", err)
	}

	tests := []struct {
		name    string
		doc     string
		wantErr bool
	}{
		{name: "the built fixture", doc: string(valid)},
		{name: "a bare COMPOSITION", doc: `{"_type":"COMPOSITION"}`, wantErr: true},
		{name: "not JSON", doc: `not json`, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := check([]byte(tc.doc), compiled)
			if !tc.wantErr {
				if err != nil {
					t.Errorf("check(%s) = %v, want nil", tc.name, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("check(%q) = nil, want an error listing the issues", tc.doc)
			}
			// The first line says the document is not valid; one indented
			// "path [code] detail" line follows per issue.
			lines := strings.Split(err.Error(), "\n")
			if lines[0] != "the encoded composition is not valid:" || len(lines) < 2 {
				t.Errorf("check(%q) = %q, want a heading line and at least one issue line", tc.doc, err)
			}
		})
	}
}
