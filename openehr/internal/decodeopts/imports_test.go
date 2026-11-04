package decodeopts_test

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/importguard"
)

// TestDecodeoptsImportsOnlyTheStandardLibrary guards the edges this package
// sits between: openehr/serialize/canxml imports it to register the reader of
// its options, and openehr/aom/aom14 imports it to read them, so an import of
// either, or of any package of this module, could close a cycle. Every import
// of its own non-test files is therefore a standard-library package.
func TestDecodeoptsImportsOnlyTheStandardLibrary(t *testing.T) {
	t.Parallel()
	imports, err := importguard.Imports(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, imp := range imports {
		if !importguard.Standard(imp) {
			t.Errorf("openehr/internal/decodeopts imports %q; it imports only the standard library, so canxml and aom14 can import it without a cycle", imp)
		}
	}
}
