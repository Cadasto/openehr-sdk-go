package webtemplate_test

import (
	"go/build"
	"strings"
	"testing"
)

// REQ-013 § Building-block independence — openehr/template/webtemplate MUST
// stay a pure clinical building block: a compiled OPT in, WebTemplate JSON
// out. It depends only on openehr/templatecompile,
// openehr/template/constraints, and the standard library — never on the
// wire / transport / auth / client / serialize layers. Mirrors
// TestTemplatecompileForbiddenImports and locks the contract documented in
// doc.go (REQ-106).
func TestWebtemplateForbiddenImports(t *testing.T) {
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
				t.Errorf("openehr/template/webtemplate MUST NOT import %q (REQ-013 building-block independence; matched forbidden prefix %q)", imp, bad)
			}
		}
	}
}
