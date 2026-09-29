package instance_test

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/importguard"
)

// TestInstanceForbiddenImports guards REQ-013
// (docs/specifications/module-layout.md § REQ-013) for openehr/instance, plus
// two dependency-direction bans:
//
//   - Neither the package nor any package of this module it pulls in imports
//     transport, auth or openehr/client.
//   - The package's own non-test files do not import openehr/serialize, which
//     REQ-013 bars from every template-side building block. This rule cannot
//     cover what they pull in, since openehr/rm's generated marshal files
//     import openehr/serialize/canxml.
//   - They do not import openehr/composition either: the REQ-101 builder
//     consumes instance, not the reverse.
//   - Nor openehr/validation: the validator checks what instance generates,
//     and reaches instance only from tests and probes.
//
// Test files may import anything, for cross-package probes for instance.
func TestInstanceForbiddenImports(t *testing.T) {
	t.Parallel()
	violations, err := importguard.Scan(".", importguard.WireLayers())
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range violations {
		t.Errorf("openehr/instance MUST NOT pull in %q: %s imports it (forbidden entry %q; REQ-013 building-block independence)", v.Import, v.Importer, v.Prefix)
	}

	own := []struct{ entry, rule string }{
		{
			entry: "github.com/cadasto/openehr-sdk-go/openehr/serialize",
			rule:  "REQ-013: a template-side building block never imports openehr/serialize",
		},
		{
			entry: "github.com/cadasto/openehr-sdk-go/openehr/composition",
			rule:  "the composition builder imports instance, not the reverse",
		},
		{
			entry: "github.com/cadasto/openehr-sdk-go/openehr/validation",
			rule:  "the validator checks instance output, not the reverse",
		},
	}
	imports, err := importguard.Imports(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, imp := range imports {
		for _, r := range own {
			if p, ok := importguard.Matches(imp, []string{r.entry}); ok {
				t.Errorf("openehr/instance MUST NOT import %q in its own files (forbidden entry %q; %s)", imp, p, r.rule)
			}
		}
	}
}
