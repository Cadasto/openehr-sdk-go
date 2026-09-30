package terminology_test

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/importguard"
)

// TestTerminologyForbiddenImports guards REQ-013
// (docs/specifications/module-layout.md § REQ-013) and REQ-034 for
// openehr/terminology: the package imports only the standard library, so it
// sits below openehr/rm and RM-level code can import it without a cycle.
// doc.go promises the same. Two rules hold:
//
//   - Neither the package nor any package of this module it pulls in imports
//     transport, auth or openehr/client.
//   - Every import of the package's own non-test files is a standard-library
//     package. This rejects this module's packages, openehr/rm most
//     pointedly, and third-party modules alike: listing a forbidden subset
//     would let a new dependency in under a name nobody thought to list.
//
// Test files may import anything: the drift check re-hashes the pinned XML,
// for instance. importguard.TestStandard is the can-fail control for the
// standard-library check.
func TestTerminologyForbiddenImports(t *testing.T) {
	t.Parallel()
	violations, err := importguard.Scan(".", importguard.WireLayers())
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range violations {
		t.Errorf("openehr/terminology MUST NOT pull in %q: %s imports it (forbidden entry %q; REQ-013 building-block independence)", v.Import, v.Importer, v.Prefix)
	}

	imports, err := importguard.Imports(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, imp := range imports {
		if !importguard.Standard(imp) {
			t.Errorf("openehr/terminology MUST NOT import %q in its own files (REQ-013 and REQ-034: the package imports only the standard library, so openehr/rm can import it)", imp)
		}
	}
}
