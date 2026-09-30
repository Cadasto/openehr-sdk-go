package templatecompile_test

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/importguard"
)

// TestTemplatecompileForbiddenImports guards REQ-013
// (docs/specifications/module-layout.md § REQ-013) for openehr/templatecompile,
// a clinical building block that takes a parsed OPT and returns a compiled
// template, as doc.go documents. Two rules hold:
//
//   - Neither the package nor any package of this module it pulls in imports
//     transport, auth or openehr/client.
//   - The package's own non-test files do not import openehr/serialize, the
//     rule every template-side building block keeps. Today nothing they pull
//     in imports it either: the closure reaches openehr/rm/rminfo but not
//     openehr/rm, whose generated marshal files are what bring
//     openehr/serialize/canxml into other blocks' closures.
func TestTemplatecompileForbiddenImports(t *testing.T) {
	t.Parallel()
	violations, err := importguard.Scan(".", importguard.WireLayers())
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range violations {
		t.Errorf("openehr/templatecompile MUST NOT pull in %q: %s imports it (forbidden entry %q; REQ-013 building-block independence)", v.Import, v.Importer, v.Prefix)
	}

	serialize := []string{"github.com/cadasto/openehr-sdk-go/openehr/serialize"}
	imports, err := importguard.Imports(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, imp := range imports {
		if p, ok := importguard.Matches(imp, serialize); ok {
			t.Errorf("openehr/templatecompile MUST NOT import %q in its own files (forbidden entry %q; REQ-013: a template-side building block never imports openehr/serialize)", imp, p)
		}
	}
}
