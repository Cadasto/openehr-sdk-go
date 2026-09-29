package contain_test

import (
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/importguard"
)

// TestContainForbiddenImports guards REQ-013
// (docs/specifications/module-layout.md § REQ-013) for openehr/aql/contain, in
// the scope REQ-160 sets (clinical-modeling.md § REQ-160, Building-block
// independence). Two rules hold:
//
//   - Neither the package nor any package of this module it pulls in imports
//     transport, auth or openehr/client.
//   - The package's own non-test files import only openehr/rm,
//     openehr/rm/rminfo and the standard library. That also keeps out
//     openehr/aql and openehr/aql/lint, which sit above the relation, and
//     third-party modules. It cannot cover what they pull in: openehr/rm's
//     generated marshal files import openehr/serialize/canxml.
//
// importguard.TestStandard is the can-fail control for the standard-library
// check.
func TestContainForbiddenImports(t *testing.T) {
	t.Parallel()
	violations, err := importguard.Scan(".", importguard.WireLayers())
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range violations {
		t.Errorf("openehr/aql/contain MUST NOT pull in %q: %s imports it (forbidden entry %q; REQ-013 building-block independence)", v.Import, v.Importer, v.Prefix)
	}

	allowed := []string{
		"github.com/cadasto/openehr-sdk-go/openehr/rm",
		"github.com/cadasto/openehr-sdk-go/openehr/rm/rminfo",
	}
	imports, err := importguard.Imports(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, imp := range imports {
		if !importguard.Standard(imp) && !slices.Contains(allowed, imp) {
			t.Errorf("openehr/aql/contain MUST NOT import %q in its own files (REQ-013: aql/contain imports only openehr/rm, openehr/rm/rminfo and the standard library)", imp)
		}
	}
}
