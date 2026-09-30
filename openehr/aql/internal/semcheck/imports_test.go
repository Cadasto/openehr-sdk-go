package semcheck_test

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/importguard"
)

// TestSemcheckForbiddenImports guards REQ-013
// (docs/specifications/module-layout.md § REQ-013) for the shared rule engine
// openehr/aql/internal/semcheck. Two rules hold:
//
//   - Neither the package nor any package of this module it pulls in imports
//     transport, auth or openehr/client.
//   - The package's own non-test files import only openehr/aql/contain and
//     the standard library. It cannot cover what they pull in: contain
//     reaches openehr/rm, whose generated marshal files import
//     openehr/serialize/canxml.
//
// The second rule matters in both directions. Downward it keeps the engine
// free of transport, auth, client and serialize, like the relation below it.
// Upward it is what lets two adapters share the engine (REQ-162 § Contract,
// one engine, two adapters): an import of openehr/aql or openehr/aql/lint
// would put the engine above one of its adapters and make the other one's use
// of it a cycle. REQ-161: the lint imports the engine, never the reverse.
//
// importguard.TestStandard is the can-fail control for the standard-library
// check.
func TestSemcheckForbiddenImports(t *testing.T) {
	t.Parallel()
	violations, err := importguard.Scan(".", importguard.WireLayers())
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range violations {
		t.Errorf("openehr/aql/internal/semcheck MUST NOT pull in %q: %s imports it (forbidden entry %q; REQ-013 building-block independence)", v.Import, v.Importer, v.Prefix)
	}

	const allowed = "github.com/cadasto/openehr-sdk-go/openehr/aql/contain"
	imports, err := importguard.Imports(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, imp := range imports {
		if !importguard.Standard(imp) && imp != allowed {
			t.Errorf("openehr/aql/internal/semcheck MUST NOT import %q in its own files (REQ-013: semcheck imports only openehr/aql/contain and the standard library)", imp)
		}
	}
}
