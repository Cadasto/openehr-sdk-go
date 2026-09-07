package terminology_test

import (
	"go/build"
	"strings"
	"testing"
)

// REQ-034 § openEHR terminology vocabulary — the accessor sits *below*
// openehr/rm so RM-level consumers can adopt it, and the requirement names
// openehr/rm, openehr/serialize/*, openehr/client/*, transport/ and auth/ as
// forbidden. This tripwire is stricter than that list on purpose: the package
// is stdlib-only, so *any* import of this module's own path is a violation
// — enumerating a forbidden subset would let a new in-module import slip in
// under a name nobody thought to list. It joins the REQ-013
// building-block-independence set on the same terms.
//
// Non-test files only: a test file may import whatever it needs (Task 3's
// drift check re-hashes the pinned XML, for instance).
func TestTerminologyForbiddenImports(t *testing.T) {
	t.Parallel()
	pkg, err := build.Default.ImportDir("./", 0)
	if err != nil {
		t.Fatalf("ImportDir: %v", err)
	}
	if len(pkg.GoFiles) == 0 {
		t.Fatal("no non-test Go files enumerated — the tripwire is vacuous")
	}
	const module = "github.com/cadasto/openehr-sdk-go/"
	for _, imp := range pkg.Imports {
		if strings.Contains(imp, module) {
			t.Errorf("openehr/terminology MUST NOT import %q — the package is stdlib-only so openehr/rm can import it (REQ-034; REQ-013 building-block independence)", imp)
		}
	}
}
