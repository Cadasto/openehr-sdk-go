package validation_test

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/importguard"
)

// TestValidationForbiddenImports guards REQ-013
// (docs/specifications/module-layout.md § REQ-013) for openehr/validation.
// The validator works on in-memory RM graphs: decoding wire bytes, network
// calls and authentication belong to its callers. Two rules hold:
//
//   - Neither the package nor any package of this module it pulls in imports
//     transport, auth or openehr/client. The generated attribute access in
//     internal/rmaccess is one of those packages, so the closure scan covers
//     it too.
//   - The package's own non-test files do not import openehr/serialize. This
//     rule cannot cover what they pull in, since openehr/rm's generated
//     marshal files import openehr/serialize/canxml.
//
// Test files may import anything, for fixture decoding for instance.
func TestValidationForbiddenImports(t *testing.T) {
	t.Parallel()
	violations, err := importguard.Scan(".", importguard.WireLayers())
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range violations {
		t.Errorf("openehr/validation MUST NOT pull in %q: %s imports it (forbidden entry %q; REQ-013 building-block independence)", v.Import, v.Importer, v.Prefix)
	}
	serialize := []string{"github.com/cadasto/openehr-sdk-go/openehr/serialize"}
	imports, err := importguard.Imports(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, imp := range imports {
		if p, ok := importguard.Matches(imp, serialize); ok {
			t.Errorf("openehr/validation MUST NOT import %q in its own files (forbidden entry %q; REQ-013: a template-side building block never imports openehr/serialize)", imp, p)
		}
	}
}
