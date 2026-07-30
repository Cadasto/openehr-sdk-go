package templatecompile_test

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// REQ-116 (Phase 0) / PROBE-075 — the vendored REQ-116 oracle OPTs compile.
//
// Corona_Anamnese is the archetype-reuse-under-slot regression guard: four
// SECTION.adhoc.v1 siblings (and, one level down, eight reused screening
// OBSERVATIONs under Symptome) produce repeated AQL paths that only shared-path
// admission lets through — legal per REQ-116, where registerPath's duplicate
// rejection is REQ-100. Previously this class was claimed to compile with no
// in-tree guard.
//
// Build outcomes differ between the two and are pinned separately in
// openehr/template/webtemplate/req116_gap_test.go: Corona_Anamnese returns
// ErrIDCollision, GECCO_Diagnose builds but silently diverges from its
// name-predicated golden. Neither emits name predicates yet — asserted below
// as the compile-layer tripwire for plan Phase 3. See
// openehr/template/webtemplate/deviations.md § Sibling `id` disambiguation.
// Provenance: testkit/cassettes/THIRD_PARTY_LICENSES.md.
func TestCompile_WebTemplateOracleOPTs(t *testing.T) {
	tests := []struct {
		templateID string
		// sharedPath is a compiled AQL path the fixture's reused siblings
		// collide on ("" when the fixture has no such collision).
		sharedPath string
		wantRMType string
		// wantPaths are further compiled paths that must resolve, with their
		// expected RM type — the fixture's load-bearing structure.
		wantPaths map[string]string
		// notYetPath is the name-predicated form the reference golden uses for
		// one of wantPaths. It must NOT resolve until Phase 3 emits predicates
		// — the compile-layer tripwire.
		notYetPath string
	}{
		{
			templateID: "Corona_Anamnese",
			sharedPath: "/content[openEHR-EHR-SECTION.adhoc.v1]",
			wantRMType: "SECTION",
		},
		{
			// The reference golden name-predicates 24 of this fixture's paths
			// (30 segments) with no sibling archetype reuse anywhere — its
			// three /content children have distinct archetype ids and are all
			// predicated, which is why a collision-conditioned rule is wrong.
			templateID: "GECCO_Diagnose",
			wantPaths: map[string]string{
				"/content[openEHR-EHR-EVALUATION.problem_diagnosis.v1]":  "EVALUATION",
				"/content[openEHR-EHR-EVALUATION.exclusion_specific.v1]": "EVALUATION",
				"/content[openEHR-EHR-EVALUATION.absence.v2]":            "EVALUATION",
			},
			notYetPath: "/content[openEHR-EHR-EVALUATION.absence.v2,'Unbekannte Diagnose']",
		},
	}
	for _, tc := range tests {
		t.Run(tc.templateID, func(t *testing.T) {
			opt, err := template.ParseFile(fixtures.WebTemplateOpt(tc.templateID))
			if err != nil {
				t.Fatalf("ParseFile: %v", err)
			}
			c, err := templatecompile.Compile(opt)
			if err != nil {
				t.Fatalf("Compile: %v", err)
			}
			if got := c.TemplateID(); got != tc.templateID {
				t.Errorf("TemplateID() = %q, want %q", got, tc.templateID)
			}
			if tc.sharedPath != "" {
				// The first sibling wins the shared path and stays reachable (a
				// failure here would mean the reused subtree was dropped rather
				// than admitted).
				n, err := c.NodeAt(tc.sharedPath)
				if err != nil {
					t.Fatalf("NodeAt(%q): %v — shared-path subtree was not registered", tc.sharedPath, err)
				}
				if got := n.RMTypeName(); got != tc.wantRMType {
					t.Errorf("RMTypeName() = %q, want %q", got, tc.wantRMType)
				}
			}
			for path, wantRM := range tc.wantPaths {
				n, err := c.NodeAt(path)
				if err != nil {
					t.Errorf("NodeAt(%q): %v", path, err)
					continue
				}
				if got := n.RMTypeName(); got != wantRM {
					t.Errorf("NodeAt(%q).RMTypeName() = %q, want %q", path, got, wantRM)
				}
			}
			// Bare form today; the golden's predicated form resolves to
			// nothing. When Phase 3 lands this lookup succeeds, the assertion
			// fires, and wantPaths must move to the predicated spelling.
			if tc.notYetPath != "" {
				if _, err := c.NodeAt(tc.notYetPath); err == nil {
					t.Errorf("NodeAt(%q) resolved — REQ-116 Phase 3 has landed; "+
						"switch wantPaths to the predicated form and extend PROBE-075 parity", tc.notYetPath)
				}
			}
		})
	}
}
