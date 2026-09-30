package aql_test

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/importguard"
)

// TestAQLForbiddenImports guards REQ-013 (docs/specifications/module-layout.md
// § REQ-013) for openehr/aql, in the scope REQ-162 sets
// (clinical-modeling.md § REQ-162, Building-block independence). REQ-162 gave
// the package its first in-module imports, openehr/aql/contain and the
// Go-internal openehr/aql/internal/semcheck, and a program that only builds
// and verifies AQL must still name no transport, no auth and no client. Two
// rules hold:
//
//   - Neither the package nor any package of this module it pulls in imports
//     transport, auth, openehr/client or openehr/validation. The arrow is
//     validation to aql, never the reverse.
//   - The package's own non-test files do not import openehr/serialize. This
//     rule cannot cover what they pull in: contain reaches openehr/rm, whose
//     generated marshal files import openehr/serialize/canxml.
//
// A ban list, not an allow-list: the rule is "no wire layers", not "no new
// dependencies", so a legitimate new dependency need not amend the guard.
func TestAQLForbiddenImports(t *testing.T) {
	t.Parallel()
	closure := append(importguard.WireLayers(), "github.com/cadasto/openehr-sdk-go/openehr/validation")
	violations, err := importguard.Scan(".", closure)
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range violations {
		t.Errorf("openehr/aql MUST NOT pull in %q: %s imports it (forbidden entry %q; REQ-013 building-block independence, in the REQ-162 scope)", v.Import, v.Importer, v.Prefix)
	}

	serialize := []string{"github.com/cadasto/openehr-sdk-go/openehr/serialize"}
	imports, err := importguard.Imports(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, imp := range imports {
		if p, ok := importguard.Matches(imp, serialize); ok {
			t.Errorf("openehr/aql MUST NOT import %q in its own files (forbidden entry %q; REQ-013 in the REQ-162 scope: openehr/aql never names openehr/serialize)", imp, p)
		}
	}
}
