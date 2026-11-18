package aqlvalidation_test

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/importguard"
)

// TestAQLValidationForbiddenImports guards two import rules for
// openehr/validation/aqlvalidation. Linting AQL into the validation model is a
// building block for CI validators and pre-flight checks, usable without an
// authenticated client.
//
//   - REQ-013 (docs/specifications/module-layout.md § REQ-013): neither the
//     package nor any package of this module it pulls in imports transport,
//     auth or openehr/client.
//   - REQ-013 (the openehr/validation/aqlvalidation row): the package's own
//     non-test files do not import openehr/serialize. This rule cannot cover
//     what they pull in: the package imports openehr/validation and
//     openehr/aql, which reach openehr/rm, whose generated marshal files
//     import openehr/serialize/canxml.
func TestAQLValidationForbiddenImports(t *testing.T) {
	t.Parallel()
	violations, err := importguard.Scan(".", importguard.WireLayers())
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range violations {
		t.Errorf("openehr/validation/aqlvalidation MUST NOT pull in %q: %s imports it (forbidden entry %q; REQ-013 building-block independence)", v.Import, v.Importer, v.Prefix)
	}

	serialize := []string{"github.com/cadasto/openehr-sdk-go/openehr/serialize"}
	imports, err := importguard.Imports(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, imp := range imports {
		if p, ok := importguard.Matches(imp, serialize); ok {
			t.Errorf("openehr/validation/aqlvalidation MUST NOT import %q in its own files (forbidden entry %q; REQ-013: the AQL validation bridge never imports openehr/serialize)", imp, p)
		}
	}
}
