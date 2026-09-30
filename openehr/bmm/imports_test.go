package bmm_test

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/importguard"
)

// TestBMMForbiddenImports guards REQ-013 (docs/specifications/module-layout.md
// § REQ-013) and REQ-045 (docs/specifications/bmm-conformance.md § REQ-045)
// for openehr/bmm: validators, archetype tools and code generators import the
// loader without transport, auth or any HTTP machinery. Two rules hold for the
// package and everything it pulls in:
//
//   - REQ-013: no package of this module in the closure imports transport,
//     auth or openehr/client.
//   - REQ-045: no package in the closure, the standard library's included,
//     imports net/http or a package under it. This walk follows the standard
//     library too, so net/http pulled in through expvar or net/rpc is found as
//     well; importguard.TestScanStd is its can-fail control.
func TestBMMForbiddenImports(t *testing.T) {
	t.Parallel()
	rules := []struct {
		rule      string
		scan      func(dir string, forbidden []string) ([]importguard.Violation, error)
		forbidden []string
	}{
		{rule: "REQ-013 building-block independence", scan: importguard.Scan, forbidden: importguard.WireLayers()},
		{rule: "REQ-045: the BMM loader needs no HTTP machinery", scan: importguard.ScanStd, forbidden: []string{"net/http"}},
	}
	for _, r := range rules {
		violations, err := r.scan(".", r.forbidden)
		if err != nil {
			t.Fatal(err)
		}
		for _, v := range violations {
			t.Errorf("openehr/bmm MUST NOT pull in %q: %s imports it (forbidden entry %q; %s)", v.Import, v.Importer, v.Prefix, r.rule)
		}
	}
}
