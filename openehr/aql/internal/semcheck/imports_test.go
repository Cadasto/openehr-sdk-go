package semcheck_test

import (
	"go/build"
	"strings"
	"testing"
)

// TestSemcheckForbiddenImports pins the building-block contract (REQ-013) for
// the shared rule engine: its non-test files MUST import only
// openehr/aql/contain and the standard library.
//
// The rule matters in both directions. Downward it keeps the engine free of
// transport / auth / client / serialize, like the relation below it. Upward it
// is what makes the engine SHAREABLE at all (REQ-162 § Contract — one engine,
// two adapters): an import of openehr/aql or openehr/aql/lint would put the
// engine above one of its own adapters and make the other one's use of it a
// cycle.
func TestSemcheckForbiddenImports(t *testing.T) {
	pkg, err := build.Default.ImportDir(".", 0)
	if err != nil {
		t.Fatalf("ImportDir: %v", err)
	}
	if len(pkg.GoFiles) == 0 {
		t.Fatal("ImportDir enumerated no non-test Go files; the guard is vacuous")
	}
	const allowed = "github.com/cadasto/openehr-sdk-go/openehr/aql/contain"
	for _, imp := range pkg.Imports {
		if imp == allowed {
			continue
		}
		// A dot in the first path element marks a module path; the standard
		// library never has one. This fails third-party imports too, closing
		// the "and the standard library" half of the REQ-013 clause.
		if first, _, _ := strings.Cut(imp, "/"); strings.Contains(first, ".") {
			t.Errorf("openehr/aql/internal/semcheck MUST NOT import %q (REQ-013; allowed: openehr/aql/contain, stdlib)", imp)
		}
	}
}
