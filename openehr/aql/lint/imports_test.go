package lint_test

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/importguard"
)

// TestAQLLintForbiddenImports guards REQ-013
// (docs/specifications/module-layout.md § REQ-013) for openehr/aql/lint.
// Linting AQL is a building block for CI validators, MCP tools and pre-flight
// checks, usable without an authenticated client (REQ-109). Two rules hold:
//
//   - Neither the package nor any package of this module it pulls in imports
//     transport, auth, openehr/client or openehr/validation. The arrow is
//     validation to lint, never the reverse, so openehr/validation is banned
//     from the whole closure, as for openehr/aql.
//   - The package's own non-test files do not import openehr/serialize. This
//     rule cannot cover what they pull in: lint imports openehr/aql, which
//     reaches openehr/rm, whose generated marshal files import
//     openehr/serialize/canxml.
func TestAQLLintForbiddenImports(t *testing.T) {
	t.Parallel()
	closure := append(importguard.WireLayers(), "github.com/cadasto/openehr-sdk-go/openehr/validation")
	violations, err := importguard.Scan(".", closure)
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range violations {
		t.Errorf("openehr/aql/lint MUST NOT pull in %q: %s imports it (forbidden entry %q; REQ-013 building-block independence)", v.Import, v.Importer, v.Prefix)
	}

	serialize := []string{"github.com/cadasto/openehr-sdk-go/openehr/serialize"}
	imports, err := importguard.Imports(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, imp := range imports {
		if p, ok := importguard.Matches(imp, serialize); ok {
			t.Errorf("openehr/aql/lint MUST NOT import %q in its own files (forbidden entry %q; REQ-013: aql/lint never imports openehr/serialize)", imp, p)
		}
	}
}
