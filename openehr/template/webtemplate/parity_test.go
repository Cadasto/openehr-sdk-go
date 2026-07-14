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

// referenceStem is the vendored EHRbase parity fixture's filename stem.
// constrain_test is used (not corona_anamnese) because it compiles under
// templatecompile — see the archetype-reuse-under-slot gap in REQ-106.
const referenceStem = "constrain_test"

// loadReference decodes the vendored reference WebTemplate JSON.
func loadReference(t *testing.T) map[string]any {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(referenceDir, referenceStem+".webtemplate.json"))
	if err != nil {
		t.Fatalf("vendored reference fixture unreadable (PROBE-075): %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("reference is not valid JSON: %v", err)
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
