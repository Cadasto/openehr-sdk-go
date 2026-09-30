package composition_test

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/importguard"
)

// TestCompositionForbiddenImports guards REQ-013
// (docs/specifications/module-layout.md § REQ-013) for openehr/composition,
// the builder above openehr/instance and openehr/template. Two rules hold:
//
//   - Neither the package nor any package of this module it pulls in imports
//     transport, auth or openehr/client.
//   - The package's own non-test files do not import openehr/serialize, which
//     no template-side building block imports: a caller that wants wire bytes
//     imports a codec itself. This rule cannot cover what they pull in, since
//     openehr/rm's generated marshal files import openehr/serialize/canxml.
//
// Test files may import anything, canjson for round-trip checks for instance.
func TestCompositionForbiddenImports(t *testing.T) {
	t.Parallel()
	violations, err := importguard.Scan(".", importguard.WireLayers())
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range violations {
		t.Errorf("openehr/composition MUST NOT pull in %q: %s imports it (forbidden entry %q; REQ-013 building-block independence)", v.Import, v.Importer, v.Prefix)
	}

	serialize := []string{"github.com/cadasto/openehr-sdk-go/openehr/serialize"}
	imports, err := importguard.Imports(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, imp := range imports {
		if p, ok := importguard.Matches(imp, serialize); ok {
			t.Errorf("openehr/composition MUST NOT import %q in its own files (forbidden entry %q; REQ-013: a template-side building block never imports openehr/serialize)", imp, p)
		}
	}
}
