package aql_test

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/importguard"
)

// TestAQLForbiddenImports guards three import rules for openehr/aql. REQ-162
// (docs/specifications/clinical-modeling.md § REQ-162, Building-block
// independence) gave the package its first in-module imports,
// openehr/aql/contain and the Go-internal openehr/aql/internal/semcheck, and a
// program that only builds and verifies AQL must still name no transport, no
// auth and no client.
//
//   - REQ-013 (docs/specifications/module-layout.md § REQ-013): neither the
//     package nor any package of this module it pulls in imports transport,
//     auth or openehr/client.
//   - REQ-162: nothing in that closure imports openehr/validation. The arrow
//     is validation to aql, never the reverse.
//   - REQ-162: the package's own non-test files do not import
//     openehr/serialize. This rule cannot cover what they pull in: contain
//     reaches openehr/rm, whose generated marshal files import
//     openehr/serialize/canxml.
//
// Ban lists, not an allow-list: the rules are "no wire layers" and "no
// validator", not "no new dependencies", so a legitimate new dependency need
// not amend the guard.
func TestAQLForbiddenImports(t *testing.T) {
	t.Parallel()
	closure := []struct {
		rule      string
		forbidden []string
	}{
		{rule: "REQ-013 building-block independence", forbidden: importguard.WireLayers()},
		{
			rule:      "REQ-162: the arrow is validation to aql, never the reverse",
			forbidden: []string{"github.com/cadasto/openehr-sdk-go/openehr/validation"},
		},
	}
	for _, r := range closure {
		violations, err := importguard.Scan(".", r.forbidden)
		if err != nil {
			t.Fatal(err)
		}
		for _, v := range violations {
			t.Errorf("openehr/aql MUST NOT pull in %q: %s imports it (forbidden entry %q; %s)", v.Import, v.Importer, v.Prefix, r.rule)
		}
	}

	serialize := []string{"github.com/cadasto/openehr-sdk-go/openehr/serialize"}
	imports, err := importguard.Imports(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, imp := range imports {
		if p, ok := importguard.Matches(imp, serialize); ok {
			t.Errorf("openehr/aql MUST NOT import %q in its own files (forbidden entry %q; REQ-162: openehr/aql never names openehr/serialize)", imp, p)
		}
	}
}
