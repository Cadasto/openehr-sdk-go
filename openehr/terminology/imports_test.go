package terminology_test

import (
	"go/build"
	"strings"
	"testing"
)

// REQ-034 § openEHR terminology vocabulary — the accessor MUST be a
// stdlib-only building block, so it sits *below* openehr/rm and RM-level
// consumers can adopt it without a dependency cycle. doc.go promises the
// same thing, and it joins the REQ-013 building-block-independence set on
// those terms.
//
// The rule enforced here is stdlib-only, not merely in-module-free: any
// import whose first path element contains a dot is a module path (standard
// library import paths never carry one), so it fails — this module's own
// packages (most pointedly openehr/rm, which would invert the dependency
// direction) and third-party modules alike. Enumerating a forbidden subset
// would let a new dependency slip in under a name nobody thought to list.
// Same rule as the sibling stdlib-only block, openehr/rm/rminfo.
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
	for _, imp := range pkg.Imports {
		first, _, _ := strings.Cut(imp, "/")
		if strings.Contains(first, ".") {
			t.Errorf("openehr/terminology imports %q — the package is stdlib-only so openehr/rm can import it (REQ-034; REQ-013 building-block independence)", imp)
		}
	}
}
