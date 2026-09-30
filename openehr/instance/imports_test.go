package instance_test

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/importguard"
)

// TestInstanceForbiddenImports guards three import rules for
// openehr/instance:
//
//   - REQ-013 (docs/specifications/module-layout.md § REQ-013): neither the
//     package nor any package of this module it pulls in imports transport,
//     auth or openehr/client.
//   - REQ-107 (docs/specifications/clinical-modeling.md § REQ-107): nothing in
//     that closure imports openehr/composition or openehr/validation, or a
//     package under either. The composition builder calls the generator, not
//     the reverse, and PROBE-027 checks the generator's output with the
//     validator, which is an independent check only while the generator does
//     not use it.
//   - REQ-013: the package's own non-test files do not import
//     openehr/serialize, which no template-side building block imports. This
//     rule cannot cover what they pull in, since openehr/rm's generated
//     marshal files import openehr/serialize/canxml.
//
// Test files may import anything, for cross-package probes for instance.
func TestInstanceForbiddenImports(t *testing.T) {
	t.Parallel()
	closure := []struct {
		rule      string
		forbidden []string
	}{
		{rule: "REQ-013 building-block independence", forbidden: importguard.WireLayers()},
		{
			rule: "REQ-107: the generator is independent of the builder and of the validator that checks its output",
			forbidden: []string{
				"github.com/cadasto/openehr-sdk-go/openehr/composition",
				"github.com/cadasto/openehr-sdk-go/openehr/validation",
			},
		},
	}
	for _, r := range closure {
		violations, err := importguard.Scan(".", r.forbidden)
		if err != nil {
			t.Fatal(err)
		}
		for _, v := range violations {
			t.Errorf("openehr/instance MUST NOT pull in %q: %s imports it (forbidden entry %q; %s)", v.Import, v.Importer, v.Prefix, r.rule)
		}
	}

	serialize := []string{"github.com/cadasto/openehr-sdk-go/openehr/serialize"}
	imports, err := importguard.Imports(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, imp := range imports {
		if p, ok := importguard.Matches(imp, serialize); ok {
			t.Errorf("openehr/instance MUST NOT import %q in its own files (forbidden entry %q; REQ-013: a template-side building block never imports openehr/serialize)", imp, p)
		}
	}
}
