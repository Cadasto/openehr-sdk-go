package webtemplate_test

// REQ-116 (Phase 0) / PROBE-075 — pins the two failure modes of the open
// sibling-naming gap against the vendored oracles, so the documented blocked
// state is guarded rather than observed. Every assertion here is expected to
// change when REQ-116 lands: corona must then build, GECCO must then emit the
// predicates its golden carries, and both fixtures join the PROBE-075 parity
// matrix (plan Phase 4 task 4).

import (
	"encoding/json"
	"errors"
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

// Corona_Anamnese fails loudly: four SECTION.adhoc.v1 siblings reuse one
// archetype, and one level down eight OBSERVATION.symptom_sign_screening.v0
// siblings under the Symptome section do the same. Each derives the same web
// id from the shared archetype concept term, so Build refuses to emit
// ambiguous duplicates. The collision reported first is the OBSERVATION one.
func TestBuild_CoronaAnamneseBlockedOnIDCollision(t *testing.T) {
	const collidingID = "screening-fragebogen_zur_symptomen_anzeichen"

	c := compileFixture(t, fixtures.WebTemplateOpt("Corona_Anamnese"))
	_, err := webtemplate.Build(c)
	if !errors.Is(err, webtemplate.ErrIDCollision) {
		t.Fatalf("Build(Corona_Anamnese) err = %v, want ErrIDCollision (REQ-116 open gap)", err)
	}
	// Pin *which* id collides, not just that something did: a collision that
	// moved to another subtree would mean the fixture or the id derivation
	// changed under us, and the documented mechanism no longer describes it.
	if !strings.Contains(err.Error(), collidingID) {
		t.Errorf("collision error = %q, want it to name %q (see deviations.md § Sibling `id` disambiguation)", err, collidingID)
	}
}

// GECCO_Diagnose is the silent-divergence oracle: it builds without error,
// so nothing surfaces when its aqlPaths disagree with the reference.
//
// Since REQ-116 Phase 3 the predicates themselves agree — every one of the
// golden's 24 name-predicated paths is now produced exactly. What remains
// is the second manifestation: this builder still exports the pinned name
// as a data leaf, so it emits four extra `…/name` DV_TEXT nodes the golden
// does not have (they inherit their named parent's predicate, which is why
// they show up here as extra predicated paths). That is plan Phase 4
// task 2; extending PROBE-075 to this fixture (Phase 4 task 4) is what
// finally makes any residual difference a visible failure.
func TestBuild_GeccoDiagnoseSilentlyDivergesFromGolden(t *testing.T) {
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
	// The only surplus may be the spurious `…/name` leaves below: they
	// inherit their named parent's predicate. Anything else means this
	// builder invented a predicate the reference does not have.
	for p := range ourPaths {
		if !refPaths[p] && !strings.HasSuffix(p, "/name") {
			t.Errorf("predicated path not in golden and not a `…/name` leaf: %s", p)
		}
	}

	// The second, easily-missed manifestation: this builder exports the pinned
	// name as a data leaf. The golden carries the name on the node itself and
	// has no `…/name` child anywhere.
	refNameLeaves := 0
	walkRefTree(tree, func(m map[string]any) {
		if strings.HasSuffix(refStr(m, "aqlPath"), "/name") {
			refNameLeaves++
		}
	})
	if refNameLeaves != 0 {
		t.Errorf("golden has %d `…/name` leaves, want 0", refNameLeaves)
	}
	if ourNameLeaves != 4 {
		t.Errorf("built tree has %d `…/name` leaves, want 4 spurious (see plan Phase 4 task 2)", ourNameLeaves)
	}
}
