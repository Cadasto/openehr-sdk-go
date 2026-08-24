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
	if len(pkg.GoFiles) == 0 {
		t.Fatal("ImportDir enumerated no non-test Go files; the guard is vacuous")
	}
	allowed := map[string]bool{
		"github.com/cadasto/openehr-sdk-go/openehr/rm":        true,
		"github.com/cadasto/openehr-sdk-go/openehr/rm/rminfo": true,
	}
	for _, imp := range pkg.Imports {
		if allowed[imp] {
			continue
		}
		// A dot in the first path element marks a module path; the standard
		// library never has one. This fails third-party imports too, closing
		// the "and the standard library" half of the REQ-013 clause.
		if first, _, _ := strings.Cut(imp, "/"); strings.Contains(first, ".") {
			t.Errorf("openehr/aql/contain MUST NOT import %q (REQ-013; allowed: openehr/rm, openehr/rm/rminfo, stdlib)", imp)
		}
	}
}
