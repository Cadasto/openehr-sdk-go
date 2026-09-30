package simplified_test

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/importguard"
)

// TestBuildingBlockIndependence guards REQ-013
// (docs/specifications/module-layout.md § REQ-013) for the FLAT and STRUCTURED
// codecs in openehr/serialize/simplified: converting a document to or from
// canonical RM is a standalone use, so the codecs work without the HTTP
// client, auth or transport. Neither the package nor any package of this
// module it pulls in may import:
//
//   - transport, auth or openehr/client;
//   - cadasto/, the Cadasto extras, which no package outside that subtree may
//     import (module-layout.md § cadasto/ cut line).
func TestBuildingBlockIndependence(t *testing.T) {
	t.Parallel()
	rules := []struct {
		rule      string
		forbidden []string
	}{
		{rule: "REQ-013 building-block independence", forbidden: importguard.WireLayers()},
		{
			rule:      "the cadasto/ cut line: no package outside cadasto/ imports it",
			forbidden: []string{"github.com/cadasto/openehr-sdk-go/cadasto"},
		},
	}
	for _, r := range rules {
		violations, err := importguard.Scan(".", r.forbidden)
		if err != nil {
			t.Fatal(err)
		}
		for _, v := range violations {
			t.Errorf("openehr/serialize/simplified MUST NOT pull in %q: %s imports it (forbidden entry %q; %s)", v.Import, v.Importer, v.Prefix, r.rule)
		}
	}
}
