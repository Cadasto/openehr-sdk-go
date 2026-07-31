package webtemplate_test

// PROBE-075 — the vendored EHRbase openEHR_SDK v2.3 reference is the
// WebTemplate structural-parity oracle (REQ-106, ADR-0014). The fixture is
// vendored in-repo; a missing file is repo corruption and fails the run.

import (
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// referenceDir is the vendored EHRbase parity fixture directory, relative
// to this package.
const referenceDir = "../../../testkit/cassettes/webtemplate"

// referenceStem is the first vendored EHRbase parity fixture's filename
// stem — the historical single oracle, kept as the default other tests
// load. constrain_test pins no template-level node name anywhere, so its
// golden carries zero name predicates across all 104 nodes, which is why
// it reached parity before REQ-116 existed. Since REQ-116 landed the
// matrix is parityFixtures (build_test.go): this stem plus the two
// name-pinning oracles Corona_Anamnese and GECCO_Diagnose.
const referenceStem = "constrain_test"

// loadReference decodes the vendored reference WebTemplate JSON.
func loadReference(t *testing.T) map[string]any {
	t.Helper()
	return loadReferenceStem(t, referenceStem)
}

// loadReferenceStem decodes the vendored reference WebTemplate JSON for one
// fixture stem (see parityFixtures).
func loadReferenceStem(t *testing.T, stem string) map[string]any {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(referenceDir, stem+".webtemplate.json"))
	if err != nil {
		t.Fatalf("vendored reference fixture %s unreadable (PROBE-075): %v", stem, err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("reference %s is not valid JSON: %v", stem, err)
	}
	return m
}

func TestReferenceFixtureLoads(t *testing.T) {
	ref := loadReference(t)
	if ref["version"] != "2.3" {
		t.Errorf("reference version = %v, want 2.3", ref["version"])
	}
	if _, ok := ref["tree"].(map[string]any); !ok {
		t.Fatalf("reference has no object tree; keys=%v", slices.Sorted(maps.Keys(ref)))
	}
}
