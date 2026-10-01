package serializeprobes

import (
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// TestREQ056_PROBE033_ListedXMLFixturesAreLoaded fails when loadXMLFixtureInputs
// drops a path fixtures.ListRMXML returned. PROBE-033 round-trips those
// canonical-XML fixtures; the loader's continue on an unrecognised body leaves
// the path out of the loaded inputs, and the round-trip test still passes.
// The failure names the missing relative path.
//
// REQ-056. Can-fail control: continue on one listed path inside
// loadXMLFixtureInputs and this test fails.
func TestREQ056_PROBE033_ListedXMLFixturesAreLoaded(t *testing.T) {
	rels, err := fixtures.ListRMXML()
	if err != nil {
		t.Fatalf("ListRMXML: %v", err)
	}
	if len(rels) == 0 {
		t.Fatal("ListRMXML returned no paths; the PROBE-033 fixture pin would assert nothing")
	}
	loaded, err := loadXMLFixtureInputs()
	if err != nil {
		t.Fatalf("loadXMLFixtureInputs: %v", err)
	}
	got := make(map[string]bool, len(loaded))
	for _, in := range loaded {
		rel, ok := strings.CutPrefix(in.Name, "fixture:")
		if !ok {
			continue
		}
		got[rel] = true
	}
	for _, rel := range rels {
		if !got[rel] {
			t.Errorf("loadXMLFixtureInputs() omitted %s, which fixtures.ListRMXML listed", rel)
		}
	}
}
