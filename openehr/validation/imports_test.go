package validation_test

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/importguard"
)

// TestValidationForbiddenImports guards REQ-013
// (docs/specifications/module-layout.md § REQ-013) for openehr/validation and
// openehr/validation/rmread. The validator works on in-memory RM graphs:
// decoding wire bytes, network calls and authentication belong to its
// callers. Two rules hold for each of the two packages:
//
//   - Neither the package nor any package of this module it pulls in imports
//     transport, auth or openehr/client.
//   - The package's own non-test files do not import openehr/serialize. This
//     rule cannot cover what they pull in, since openehr/rm's generated
//     marshal files import openehr/serialize/canxml.
//
// rmread is checked in its own right, not only as an import of validation, so
// its rules still hold if validation stops importing it. Test files may import
// anything, for fixture decoding for instance.
func TestValidationForbiddenImports(t *testing.T) {
	t.Parallel()
	serialize := []string{"github.com/cadasto/openehr-sdk-go/openehr/serialize"}
	for _, pkg := range []struct{ name, dir string }{
		{name: "openehr/validation", dir: "."},
		{name: "openehr/validation/rmread", dir: "rmread"},
	} {
		violations, err := importguard.Scan(pkg.dir, importguard.WireLayers())
		if err != nil {
			t.Fatal(err)
		}
		for _, v := range violations {
			t.Errorf("%s MUST NOT pull in %q: %s imports it (forbidden entry %q; REQ-013 building-block independence)", pkg.name, v.Import, v.Importer, v.Prefix)
		}
		imports, err := importguard.Imports(pkg.dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range imports {
			if p, ok := importguard.Matches(imp, serialize); ok {
				t.Errorf("%s MUST NOT import %q in its own files (forbidden entry %q; REQ-013: a template-side building block never imports openehr/serialize)", pkg.name, imp, p)
			}
		}
	}
}
