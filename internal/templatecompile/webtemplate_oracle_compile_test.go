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
// name-predicated golden. As of Phase 3 the compiled paths carry the
// reference's name predicates (asserted below); the WebTemplate builder
// composes its own paths and is pinned separately. See
// openehr/template/webtemplate/deviations.md § Sibling `id` disambiguation.
// Provenance: testkit/cassettes/THIRD_PARTY_LICENSES.md.
func TestCompile_WebTemplateOracleOPTs(t *testing.T) {
	tests := []struct {
		templateID string
		// wantPaths are compiled paths that must resolve, with their
		// expected RM type — the fixture's load-bearing structure, in the
		// reference's name-predicated spelling (REQ-116 Phase 3).
		wantPaths map[string]string
		// goneePath is the pre-Phase-3 bare spelling of a path that now
		// carries a predicate. It must NOT resolve: the whole point of the
		// phase is that named nodes moved.
		gonePath string
	}{
		{
			// The four reused SECTION.adhoc.v1 siblings each resolve at
			// their own path now — the collision this plan exists to fix.
			// Before Phase 3 all four shared /content[…SECTION.adhoc.v1]
			// and only the first was reachable.
			templateID: "Corona_Anamnese",
			wantPaths: map[string]string{
				"/content[openEHR-EHR-SECTION.adhoc.v1,'Symptome']":                                                                   "SECTION",
				"/content[openEHR-EHR-SECTION.adhoc.v1,'Kontakt']":                                                                    "SECTION",
				"/content[openEHR-EHR-SECTION.adhoc.v1,'Risikogebiet']":                                                               "SECTION",
				"/content[openEHR-EHR-SECTION.adhoc.v1,'Allgemeine Angaben']":                                                         "SECTION",
				"/content[openEHR-EHR-SECTION.adhoc.v1,'Symptome']/items[openEHR-EHR-OBSERVATION.symptom_sign_screening.v0,'Husten']": "OBSERVATION",
			},
			gonePath: "/content[openEHR-EHR-SECTION.adhoc.v1]",
		},
		{
			// The reference golden name-predicates 24 of this fixture's paths
			// (30 segments) with no sibling archetype reuse anywhere — its
			// three /content children have distinct archetype ids and are all
			// predicated, which is why a collision-conditioned rule is wrong.
			templateID: "GECCO_Diagnose",
			wantPaths: map[string]string{
				"/content[openEHR-EHR-EVALUATION.problem_diagnosis.v1,'Vorliegende Diagnose']":      "EVALUATION",
				"/content[openEHR-EHR-EVALUATION.exclusion_specific.v1,'Ausgeschlossene Diagnose']": "EVALUATION",
				"/content[openEHR-EHR-EVALUATION.absence.v2,'Unbekannte Diagnose']":                 "EVALUATION",
			},
			gonePath: "/content[openEHR-EHR-EVALUATION.absence.v2]",
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
			// The bare spelling must be gone: leaving it resolvable would
			// mean the predicate never applied to that node.
			if tc.gonePath != "" {
				if _, err := c.NodeAt(tc.gonePath); err == nil {
					t.Errorf("NodeAt(%q) still resolves — the name predicate did not apply", tc.gonePath)
				}
			}
		})
	}
}

// REQ-116 Phase 3 — the four reused SECTION.adhoc.v1 siblings each occupy
// their own compiled path, and byPath resolves each to the node whose name
// the predicate carries. Before Phase 3 all four collided and byPath kept
// only the first; this is the DoD assertion for the phase, and the
// precondition every downstream consumer (REQ-102/107/053) needs in order
// to address the second-through-fourth occurrence at all.
func TestCompile_CoronaSiblingsResolveDistinctly(t *testing.T) {
	opt, err := template.ParseFile(fixtures.WebTemplateOpt("Corona_Anamnese"))
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	c, err := templatecompile.Compile(opt)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	sections := c.AllByArchetypeID("openEHR-EHR-SECTION.adhoc.v1")
	if len(sections) != 4 {
		t.Fatalf("AllByArchetypeID = %d nodes, want 4", len(sections))
	}
	seen := make(map[string]bool, len(sections))
	for _, s := range sections {
		if seen[s.AQLPath()] {
			t.Errorf("duplicate compiled path %q — siblings did not separate", s.AQLPath())
		}
		seen[s.AQLPath()] = true

		// Round-trip: the path resolves, and back to the same node.
		got, err := c.NodeAt(s.AQLPath())
		if err != nil {
			t.Errorf("NodeAt(%q): %v", s.AQLPath(), err)
			continue
		}
		if got != s {
			t.Errorf("NodeAt(%q) resolved to a different node (name %q, want %q)",
				s.AQLPath(), got.NodeName(), s.NodeName())
		}
	}
}

// REQ-116 Phase 3 / REQ-100 — name predicates must not turn the shared-path
// admission into dead code. Genuine AOM 1.4 alternatives still produce
// repeated paths (a DV_TEXT and a DV_CODED_TEXT alternative both landing on
// .../value), which only dupDepth admits — and they carry no name, so no
// predicate separates them. Asserting a real fixture still exercises that
// route keeps the two mechanisms honest: named siblings separate, un-named
// alternatives stay shared.
func TestCompile_AlternativesStillShareAPath(t *testing.T) {
	opt, err := template.ParseFile(fixtures.WebTemplateOpt("constrain_test"))
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	c, err := templatecompile.Compile(opt)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	// constrain_test pins no name anywhere, so nothing here may carry a
	// predicate — and its alternatives must still compile, not be rejected.
	const alt = "/content[openEHR-EHR-OBSERVATION.blood_pressure.v2]/protocol/items[at0014]/value"
	if _, err := c.NodeAt(alt); err != nil {
		t.Fatalf("NodeAt(%q): %v — alternatives no longer admitted", alt, err)
	}
	var named int
	for _, n := range c.AllByRMType("DV_TEXT") {
		if n.NodeName() != "" {
			named++
		}
	}
	if named != 0 {
		t.Errorf("constrain_test has %d named DV_TEXT nodes, want 0 — fixture changed", named)
	}
}
