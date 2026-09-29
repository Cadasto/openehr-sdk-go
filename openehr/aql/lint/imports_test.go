package lint_test

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/importguard"
)

// TestAQLLintForbiddenImports guards three import rules for openehr/aql/lint.
// Linting AQL is a building block for CI validators, MCP tools and pre-flight
// checks, usable without an authenticated client.
//
//   - REQ-013 (docs/specifications/module-layout.md § REQ-013): neither the
//     package nor any package of this module it pulls in imports transport,
//     auth or openehr/client.
//   - REQ-109 (docs/specifications/clinical-modeling.md § REQ-109,
//     Building-block independence): nothing in that closure imports
//     openehr/validation. The arrow is validation to lint, never the
//     reverse, so openehr/validation is banned from the whole closure, as for
//     openehr/aql.
//   - REQ-109: the package's own non-test files do not import
//     openehr/serialize. This rule cannot cover what they pull in: lint
//     imports openehr/aql, which reaches openehr/rm, whose generated marshal
//     files import openehr/serialize/canxml.
func TestAQLLintForbiddenImports(t *testing.T) {
	t.Parallel()
	closure := []struct {
		rule      string
		forbidden []string
	}{
		{rule: "REQ-013 building-block independence", forbidden: importguard.WireLayers()},
		{
			rule:      "REQ-109: the arrow is validation to lint, never the reverse",
			forbidden: []string{"github.com/cadasto/openehr-sdk-go/openehr/validation"},
		},
	}
	for _, r := range closure {
		violations, err := importguard.Scan(".", r.forbidden)
		if err != nil {
			t.Fatal(err)
		}
		for _, v := range violations {
			t.Errorf("openehr/aql/lint MUST NOT pull in %q: %s imports it (forbidden entry %q; %s)", v.Import, v.Importer, v.Prefix, r.rule)
		}
	}

	serialize := []string{"github.com/cadasto/openehr-sdk-go/openehr/serialize"}
	imports, err := importguard.Imports(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, imp := range imports {
		if p, ok := importguard.Matches(imp, serialize); ok {
			t.Errorf("openehr/aql/lint MUST NOT import %q in its own files (forbidden entry %q; REQ-109: aql/lint never imports openehr/serialize)", imp, p)
		}
	}
}
