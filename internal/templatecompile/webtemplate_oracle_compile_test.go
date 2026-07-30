package templatecompile_test

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// REQ-116 (Phase 0) / PROBE-075 — the vendored REQ-116 oracle OPTs compile.
// Corona_Anamnese is the archetype-reuse-under-slot regression guard: five
// SECTION.adhoc.v1 siblings reuse one archetype, so their subtrees produce
// repeated AQL paths that only shared-path admission (REQ-100) lets through —
// previously this class was claimed to compile with no in-tree guard.
// WebTemplate export for both still fails with ErrIDCollision until REQ-116
// lands; see openehr/template/webtemplate/deviations.md § Sibling `id`
// disambiguation. Provenance: testkit/cassettes/THIRD_PARTY_LICENSES.md.
func TestCompile_WebTemplateOracleOPTs(t *testing.T) {
	tests := []struct {
		templateID string
		// sharedPath is a compiled AQL path the fixture's reused siblings
		// collide on ("" when the fixture has no such collision).
		sharedPath string
		wantRMType string
	}{
		{
			templateID: "Corona_Anamnese",
			sharedPath: "/content[openEHR-EHR-SECTION.adhoc.v1]",
			wantRMType: "SECTION",
		},
		{
			// Name-predicated in the reference golden (30 aqlPath predicates)
			// without sibling archetype reuse — the second REQ-116 oracle.
			templateID: "GECCO_Diagnose",
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
			if tc.sharedPath == "" {
				return
			}
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
		})
	}
}
