package webtemplate_test

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/importguard"
)

// TestWebtemplateForbiddenImports guards REQ-013
// (docs/specifications/module-layout.md § REQ-013) for
// openehr/template/webtemplate, a clinical building block that takes a
// compiled OPT and returns WebTemplate JSON (REQ-106). Today it imports
// openehr/templatecompile, openehr/template/constraints, internal/templatecompile
// (for the shared REQ-116 name-predicate quoting) and the standard library.
// Two rules hold:
//
//   - Neither the package nor any package of this module it pulls in imports
//     transport, auth or openehr/client.
//   - The package's own non-test files do not import openehr/serialize. This
//     rule cannot cover what they pull in, since openehr/rm's generated
//     marshal files import openehr/serialize/canxml.
func TestWebtemplateForbiddenImports(t *testing.T) {
	t.Parallel()
	violations, err := importguard.Scan(".", importguard.WireLayers())
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range violations {
		t.Errorf("openehr/template/webtemplate MUST NOT pull in %q: %s imports it (forbidden entry %q; REQ-013 building-block independence)", v.Import, v.Importer, v.Prefix)
	}

	serialize := []string{"github.com/cadasto/openehr-sdk-go/openehr/serialize"}
	imports, err := importguard.Imports(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, imp := range imports {
		if p, ok := importguard.Matches(imp, serialize); ok {
			t.Errorf("openehr/template/webtemplate MUST NOT import %q in its own files (forbidden entry %q; REQ-013: a template-side building block never imports openehr/serialize)", imp, p)
		}
	}
}
