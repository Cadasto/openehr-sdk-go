package generic_test

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/importguard"
)

// TestGenericForbiddenImports guards REQ-013
// (docs/specifications/module-layout.md § REQ-013) for
// openehr/template/generic, the REQ-178 package that embeds the generic
// EHR_STATUS and FOLDER templates and compiles them. Today it imports
// openehr/aom/aom14, openehr/templatecompile and the standard library.
// Two rules hold:
//
//   - Neither the package nor any package of this module it pulls in imports
//     transport, auth or openehr/client.
//   - The package's own non-test files do not import openehr/serialize, the
//     rule every template-side building block keeps. The closure does reach
//     openehr/serialize/canxml, through aom14's reader; the tests import it
//     too, to read each document under canxml.WithStrict.
func TestGenericForbiddenImports(t *testing.T) {
	t.Parallel()
	violations, err := importguard.Scan(".", importguard.WireLayers())
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range violations {
		t.Errorf("openehr/template/generic MUST NOT pull in %q: %s imports it (forbidden entry %q; REQ-013 building-block independence)", v.Import, v.Importer, v.Prefix)
	}

	serialize := []string{"github.com/cadasto/openehr-sdk-go/openehr/serialize"}
	imports, err := importguard.Imports(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, imp := range imports {
		if p, ok := importguard.Matches(imp, serialize); ok {
			t.Errorf("openehr/template/generic MUST NOT import %q in its own files (forbidden entry %q; REQ-013: a template-side building block never imports openehr/serialize)", imp, p)
		}
	}
}
