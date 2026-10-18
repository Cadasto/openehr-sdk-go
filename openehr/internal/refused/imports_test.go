package refused_test

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/importguard"
)

// TestRefusedImportsOnlyTheStandardLibrary guards the edges
// docs/specifications/module-layout.md § Dependency direction draws to this
// package (REQ-014): openehr/rm, openehr/rm/typereg and
// openehr/serialize/canjson all import it, so it sits below the RM and the
// codecs, and an import of any package of this module or a third-party module
// could close a cycle through them. Every import of its own non-test files is
// therefore a standard-library package.
//
// importguard.TestStandard is the can-fail control for the standard-library
// check.
func TestRefusedImportsOnlyTheStandardLibrary(t *testing.T) {
	t.Parallel()
	imports, err := importguard.Imports(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, imp := range imports {
		if !importguard.Standard(imp) {
			t.Errorf("openehr/internal/refused imports %q; it imports only the standard library, so the RM and the codecs can import it without a cycle (REQ-014)", imp)
		}
	}
}
