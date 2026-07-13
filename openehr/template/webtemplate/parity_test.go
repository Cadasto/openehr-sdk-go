package webtemplate_test

// PROBE-075 — the vendored EHRbase openEHR_SDK v2.3 reference is the
// WebTemplate structural-parity oracle (REQ-106, ADR-0014). When the
// fixture is absent (fetch was blocked), the parity tests skip and the
// SDK-generated goldens (golden_test.go) remain the regression anchor.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// referenceDir is the vendored EHRbase parity fixture directory, relative
// to this package.
const referenceDir = "../../../testkit/cassettes/webtemplate"

// referenceStem is the vendored EHRbase parity fixture's filename stem.
// constrain_test is used (not corona_anamnese) because it compiles under
// templatecompile — see the archetype-reuse-under-slot gap in REQ-106.
const referenceStem = "constrain_test"

// loadReference decodes the reference WebTemplate JSON, skipping the test
// if the fixture is absent.
func loadReference(t *testing.T) map[string]any {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(referenceDir, referenceStem+".webtemplate.json"))
	if err != nil {
		t.Skipf("reference fixture absent — PROBE-075 deferred (ADR-0014): %v", err)
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
		t.Fatalf("reference has no object tree; keys=%v", mapKeys(ref))
	}
}

func mapKeys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
