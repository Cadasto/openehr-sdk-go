package contain_test

import (
	"go/build"
	"strings"
	"testing"
)

// TestContainForbiddenImports pins the REQ-160 § Building-block independence
// (REQ-013) contract: openehr/aql/contain's non-test files MUST import only
// openehr/rm/rminfo, openehr/rm, and the standard library — never transport /
// auth / client / serialize, and never openehr/aql or openehr/aql/lint (the
// relation sits BELOW both).
func TestContainForbiddenImports(t *testing.T) {
	pkg, err := build.Default.ImportDir(".", 0)
	if err != nil {
		t.Fatalf("ImportDir: %v", err)
	}
	const mod = "github.com/cadasto/openehr-sdk-go/"
	allowed := map[string]bool{
		mod + "openehr/rm":        true,
		mod + "openehr/rm/rminfo": true,
	}
	for _, imp := range pkg.Imports {
		if !strings.HasPrefix(imp, mod) {
			continue // standard library (or a foundation dep without the module prefix)
		}
		if !allowed[imp] {
			t.Errorf("openehr/aql/contain MUST NOT import %q (REQ-013; allowed: openehr/rm, openehr/rm/rminfo, stdlib)", imp)
		}
	}
}
