package templatecompile_test

import (
	"os"
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// The vendored upstream EHRbase conformance OPT compiles. It is the
// regression guard for shared-path subtrees: its ACTION constrains
// ism_transition with two ISM_TRANSITION alternatives, whose current_state
// and careflow_step subtrees necessarily produce repeated AQL paths under
// different attribute objects. Before shared-path subtrees were admitted,
// Compile rejected this real reference template with a duplicate-path
// error. Corpus provenance: testkit/cassettes/flat-conformance/MANIFEST.txt.
func TestCompile_UpstreamFlatConformanceOPT(t *testing.T) {
	path := fixtures.FlatConformanceOpt()
	if _, err := os.Stat(path); err != nil {
		t.Skipf("FLAT conformance corpus absent (run 'make flat-conformance-sync'): %v", err)
	}

	opt, err := template.ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	c, err := templatecompile.Compile(opt)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if got := c.TemplateID(); got != "conformance-ehrbase.de.v0" {
		t.Errorf("TemplateID() = %q, want %q", got, "conformance-ehrbase.de.v0")
	}

	// The first alternative wins the shared path, and the node is reachable
	// (a nil here would mean the subtree was dropped rather than admitted).
	const shared = "/content[openEHR-EHR-SECTION.conformance_section.v0]" +
		"/items[openEHR-EHR-ACTION.conformance_action_.v0]" +
		"/ism_transition/current_state/defining_code"
	n, err := c.NodeAt(shared)
	if err != nil {
		t.Fatalf("NodeAt(%q): %v — shared-path subtree was not registered", shared, err)
	}
	if got := n.RMTypeName(); got != "CODE_PHRASE" {
		t.Errorf("RMTypeName() = %q, want CODE_PHRASE", got)
	}
}
