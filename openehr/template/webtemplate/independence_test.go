package webtemplate_test

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/importguard"
)

// TestWebtemplateForbiddenImports guards REQ-013
// (docs/specifications/module-layout.md § REQ-013) for
// openehr/template/webtemplate, a clinical building block that takes a
// compiled OPT and returns WebTemplate JSON. Today it imports
// openehr/templatecompile, openehr/template/constraints, internal/templatecompile
// (for the name-predicate quoting it shares with the template compiler, so
// the two path builders cannot drift) and the standard library. Two rules
// hold:
//
//   - Neither the package nor any package of this module it pulls in imports
//     transport, auth or openehr/client.
//   - The package's own non-test files do not import openehr/serialize, the
//     rule every template-side building block keeps. Today nothing they pull
//     in imports it either: the closure reaches openehr/rm/rminfo but not
//     openehr/rm, whose generated marshal files are what bring
//     openehr/serialize/canxml into other blocks' closures.
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
