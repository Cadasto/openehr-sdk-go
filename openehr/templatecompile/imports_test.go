package templatecompile_test

import (
	"go/build"
	"strings"
	"testing"
)

// REQ-013 § Building-block independence — openehr/templatecompile MUST
// stay a pure clinical building block: a parsed OPT in, a compiled
// template out. It depends only on openehr/template, openehr/rm/rminfo,
// and the internal compile engine, never on the wire / transport / auth /
// client layers. Mirrors TestCompositionForbiddenImports /
// TestValidationForbiddenImports and locks the contract documented in
// doc.go.
func TestTemplatecompileForbiddenImports(t *testing.T) {
	pkg, err := build.Default.ImportDir("./", 0)
	if err != nil {
		t.Fatalf("ImportDir: %v", err)
	}
	forbidden := []string{
		"openehr/serialize",
		"openehr/client",
		"github.com/cadasto/openehr-sdk-go/transport",
		"github.com/cadasto/openehr-sdk-go/auth",
	}
	for _, imp := range pkg.Imports {
		for _, bad := range forbidden {
			if strings.Contains(imp, bad) {
				t.Errorf("openehr/templatecompile MUST NOT import %q (REQ-013 building-block independence; matched forbidden prefix %q)", imp, bad)
			}
		}
	}
}
