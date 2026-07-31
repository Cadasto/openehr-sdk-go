package webtemplate_test

// REQ-116 / PROBE-075 — the two vendored oracles for the sibling-naming
// gap. Phase 0 vendored them and pinned the two failure modes (corona loud:
// ErrIDCollision; GECCO silent: unpredicated aqlPaths and spurious name
// leaves). Phases 3–4 closed both, and both fixtures have joined the
// PROBE-075 parity matrix, so these tests now guard the *mechanism* —
// distinct name-derived sibling ids, an exact predicate set, no `…/name`
// data leaves — while TestStructuralParity guards the trees whole.

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/template/webtemplate"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// oracleGolden decodes a vendored REQ-116 oracle's reference WebTemplate.
// The parity fixture has loadReference; the oracles need it by template id.
func oracleGolden(t *testing.T, templateID string) map[string]any {
	t.Helper()
	b, err := os.ReadFile(fixtures.WebTemplateReference(templateID))
	if err != nil {
		t.Fatalf("vendored oracle golden unreadable (REQ-116): %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("%s golden is not valid JSON: %v", templateID, err)
	}
	return m
}

// namePredicates reports how many aqlPath segments in a reference tree carry a
// name predicate ([archetype_id,'Name']) and how many distinct paths carry at
// least one. Heuristic: it counts ",'" occurrences anywhere in the path
// string, not just inside brackets — exact for the vendored goldens (no
// pinned name there contains ",'"), so a count drift on a re-vendored
// golden may mean a name with an embedded ",'" rather than a new predicate.
func namePredicates(ref map[string]any) (segments, paths int) {
	walkRefTree(ref, func(m map[string]any) {
		n := strings.Count(refStr(m, "aqlPath"), ",'")
		if n > 0 {
			segments += n
			paths++
		}
	})
	return segments, paths
}

// TestReferenceOracleGoldensLoad guards the vendored goldens themselves — a
// truncated or wrong-version file would otherwise surface as a confusing
// parity failure much later. Mirrors TestReferenceFixtureLoads.
func TestReferenceOracleGoldensLoad(t *testing.T) {
	for _, tc := range []struct {
		templateID       string
		wantSegments     int
		wantPredPaths    int
		wantTotalMinimum int
	}{
		{templateID: "Corona_Anamnese", wantSegments: 350, wantPredPaths: 213, wantTotalMinimum: 230},
		{templateID: "GECCO_Diagnose", wantSegments: 30, wantPredPaths: 24, wantTotalMinimum: 34},
	} {
		t.Run(tc.templateID, func(t *testing.T) {
			ref := oracleGolden(t, tc.templateID)
			if ref["version"] != "2.3" {
				t.Errorf("golden version = %v, want 2.3 (ADR 0014 pins the v2.3 reference)", ref["version"])
			}
			if got := ref["templateId"]; got != tc.templateID {
				t.Errorf("golden templateId = %v, want %q", got, tc.templateID)
			}
			tree := refTree(t, ref)
			nodes := 0
			walkRefTree(tree, func(map[string]any) { nodes++ })
			if nodes < tc.wantTotalMinimum {
				t.Errorf("golden has %d nodes, want >= %d (truncated fixture?)", nodes, tc.wantTotalMinimum)
			}
			segments, paths := namePredicates(tree)
			if segments != tc.wantSegments || paths != tc.wantPredPaths {
				t.Errorf("golden name predicates = %d segments over %d paths, want %d over %d",
					segments, paths, tc.wantSegments, tc.wantPredPaths)
			}
		})
	}
}

// Corona_Anamnese used to fail loudly: four SECTION.adhoc.v1 siblings reuse
// one archetype, and one level down eight
// OBSERVATION.symptom_sign_screening.v0 siblings under the Symptome section
// do the same. Each derived the same web id from the shared archetype
// concept term, so Build refused to emit ambiguous duplicates
// (ErrIDCollision on "screening-fragebogen_zur_symptomen_anzeichen").
//
// REQ-116 Phase 4 resolves it at the source: the id now comes from the
// template-level node name, which is distinct per sibling by construction.
// This asserts the *mechanism*, not just that Build stopped erroring —
// whole-tree parity is TestStructuralParity/Corona_Anamnese.
func TestBuild_CoronaAnamneseSiblingsGetDistinctIDs(t *testing.T) {
	c := compileFixture(t, fixtures.WebTemplateOpt("Corona_Anamnese"))
	wt, err := webtemplate.Build(c)
	if err != nil {
		t.Fatalf("Build(Corona_Anamnese) err = %v, want nil — REQ-116 Phase 4 removes the id collision", err)
	}

	// The four reused SECTIONs: distinct ids, each sanitised from its pinned
	// name rather than from the archetype concept term they all share.
	want := map[string]bool{
		"symptome": false, "kontakt": false, "risikogebiet": false, "allgemeine_angaben": false,
	}
	var sections int
	walkOurTree(wt.Tree, func(n *webtemplate.Node) {
		if n.RMType != "SECTION" || !strings.Contains(n.AQLPath, "SECTION.adhoc.v1") {
			return
		}
		sections++
		if _, ok := want[n.ID]; !ok {
			t.Errorf("SECTION id %q not derived from a pinned name; path %s", n.ID, n.AQLPath)
			return
		}
		if want[n.ID] {
			t.Errorf("SECTION id %q emitted twice — the collision is back", n.ID)
		}
		want[n.ID] = true
	})
	if sections != 4 {
		t.Errorf("found %d SECTION.adhoc.v1 nodes, want 4", sections)
	}
	for id, seen := range want {
		if !seen {
			t.Errorf("no SECTION emitted with id %q", id)
		}
	}

	// The id that used to collide — the eight screening OBSERVATIONs reusing
	// one archetype under Symptome — now yields eight distinct ids.
	seen := map[string]bool{}
	var screening int
	walkOurTree(wt.Tree, func(n *webtemplate.Node) {
		if n.RMType != "OBSERVATION" ||
			!strings.Contains(n.AQLPath, "SECTION.adhoc.v1,'Symptome']") ||
			!strings.Contains(n.AQLPath, "OBSERVATION.symptom_sign_screening.v0") {
			return
		}
		screening++
		if seen[n.ID] {
			t.Errorf("screening OBSERVATION id %q emitted twice — the collision is back", n.ID)
		}
		seen[n.ID] = true
	})
	if screening != 8 {
		t.Errorf("found %d reused screening OBSERVATIONs under Symptome, want 8", screening)
	}
}

// GECCO_Diagnose was the silent-divergence oracle: it built without error,
// so nothing surfaced when its aqlPaths disagreed with the reference — its
// golden name-predicates 24 paths this builder emitted bare, and this
// builder emitted four `…/name` data leaves the golden does not have.
//
// Both are closed (Phase 3 and Phase 4 task 2 respectively), and
// TestStructuralParity now covers this fixture whole-tree, so silence is no
// longer possible. What stays here is the mechanism detail parity does not
// spell out: the predicate set matches the golden exactly, and neither side
// carries a `…/name` node.
func TestBuild_GeccoDiagnoseMatchesGoldenPredicates(t *testing.T) {
	const templateID = "GECCO_Diagnose"

	c := compileFixture(t, fixtures.WebTemplateOpt(templateID))
	wt, err := webtemplate.Build(c)
	if err != nil {
		t.Fatalf("Build(%s) err = %v, want nil (collides on nothing; diverges on aqlPath only)", templateID, err)
	}

	ourPaths := make(map[string]bool)
	var ourNameLeaves int
	walkOurTree(wt.Tree, func(n *webtemplate.Node) {
		if strings.Contains(n.AQLPath, ",'") {
			ourPaths[n.AQLPath] = true
		}
		if strings.HasSuffix(n.AQLPath, "/name") {
			ourNameLeaves++
		}
	})
	// The gap, from the golden's side: 30 predicate segments over 24 paths.
	tree := refTree(t, oracleGolden(t, templateID))
	segments, paths := namePredicates(tree)
	if segments != 30 || paths != 24 {
		t.Errorf("golden name predicates = %d segments over %d paths, want 30 over 24", segments, paths)
	}

	// Phase 3 DoD for this oracle: every golden predicated path is
	// produced, exactly — missing = 0. A regression in the predicate rule
	// (wrong quoting, wrong trigger, a missed node kind) lands here first.
	refPaths := make(map[string]bool)
	walkRefTree(tree, func(m map[string]any) {
		if p := refStr(m, "aqlPath"); strings.Contains(p, ",'") {
			refPaths[p] = true
		}
	})
	for p := range refPaths {
		if !ourPaths[p] {
			t.Errorf("golden predicated path not produced: %s", p)
		}
	}
	// No surplus either: inventing a predicate the reference does not have
	// would break every consumer that addresses by path.
	for p := range ourPaths {
		if !refPaths[p] {
			t.Errorf("predicated path not in golden: %s", p)
		}
	}

	// The second manifestation, now closed: this builder used to export the
	// pinned name as a data leaf. The reference carries the name on the node
	// itself (its id and its predicate) and has no `…/name` child anywhere.
	refNameLeaves := 0
	walkRefTree(tree, func(m map[string]any) {
		if strings.HasSuffix(refStr(m, "aqlPath"), "/name") {
			refNameLeaves++
		}
	})
	if refNameLeaves != 0 {
		t.Errorf("golden has %d `…/name` leaves, want 0", refNameLeaves)
	}
	if ourNameLeaves != 0 {
		t.Errorf("built tree has %d `…/name` leaves, want 0 — the pinned name is not data", ourNameLeaves)
	}
}
