package lint_test

import (
	"go/build"
	"strings"
	"testing"
)

// REQ-013 § Building-block independence — openehr/aql/lint MUST NOT depend on
// the wire / transport / auth / client / serialize layers, nor on
// openehr/validation (the dependency arrow is validation → lint, never the
// reverse). Lint is a building block (CI validators, MCP tools, pre-flight
// checks) usable without an authenticated client. Non-test files only.
func TestAQLLintForbiddenImports(t *testing.T) {
	pkg, err := build.Default.ImportDir("./", 0)
	if err != nil {
		t.Fatalf("ImportDir: %v", err)
	}
	forbidden := []string{
		"openehr/serialize",
		"openehr/client",
		"openehr/validation",
		"github.com/cadasto/openehr-sdk-go/transport",
		"github.com/cadasto/openehr-sdk-go/auth",
	}
	for _, imp := range pkg.Imports {
		for _, bad := range forbidden {
			if strings.Contains(imp, bad) {
				t.Errorf("openehr/aql/lint MUST NOT import %q (REQ-013; matched %q)", imp, bad)
			}
		}
	}
}
