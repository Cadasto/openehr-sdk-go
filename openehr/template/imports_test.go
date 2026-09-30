package template_test

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/importguard"
)

// TestTemplateForbiddenImports guards two import rules. Each holds for
// openehr/template and for every package of this module it pulls in.
//
//   - REQ-013 (docs/specifications/module-layout.md § REQ-013): no transport,
//     auth or openehr/client. openehr/serialize is not forbidden here: REQ-013
//     does not bar openehr/template from it.
//   - REQ-100 (docs/specifications/clinical-modeling.md § REQ-100): the
//     package is importable without openehr/rm or openehr/aom/aom14, so
//     nothing it pulls in may import them either. RM class names reach it as
//     strings from the OPT XML, never as Go types.
func TestTemplateForbiddenImports(t *testing.T) {
	t.Parallel()
	rules := []struct {
		rule      string
		forbidden []string
	}{
		{rule: "REQ-013 building-block independence", forbidden: importguard.WireLayers()},
		{
			rule: "REQ-100: openehr/template is importable without the RM and AOM packages",
			forbidden: []string{
				"github.com/cadasto/openehr-sdk-go/openehr/rm",
				"github.com/cadasto/openehr-sdk-go/openehr/aom/aom14",
			},
		},
	}
	for _, r := range rules {
		violations, err := importguard.Scan(".", r.forbidden)
		if err != nil {
			t.Fatal(err)
		}
		for _, v := range violations {
			t.Errorf("openehr/template MUST NOT pull in %q: %s imports it (forbidden entry %q; %s)", v.Import, v.Importer, v.Prefix, r.rule)
		}
	}
}
