package parse_test

import (
	"go/build"
	"strings"
	"testing"
)

// REQ-013 § Building-block independence — openehr/aql/parse MUST NOT depend on
// the wire / transport / auth / client layers. AQL parsing is a building block
// (CI validators, MCP tools, pre-flight checks) usable without an authenticated
// client. Non-test files only.
func TestAQLParseForbiddenImports(t *testing.T) {
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
				t.Errorf("openehr/aql/parse MUST NOT import %q (REQ-013; matched %q)", imp, bad)
			}
		}
	}
}
