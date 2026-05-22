package templateprobes_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	probes "github.com/cadasto/openehr-sdk-go/testkit/probes/template"
)

// PROBE-022 — fixture-driven assertion that the OPT parser resolves
// known paths to the expected RM types, node ids, and (for archetype
// roots) archetype ids. Uses the vital_signs.opt fixture vendored
// under openehr/template/testdata/.
func TestProbe022OPTPathResolution_VitalSigns(t *testing.T) {
	body := loadFixture(t, "vital_signs.opt")
	assertions := []probes.PathAssertion{
		{Path: "/", WantRMType: "COMPOSITION", WantNodeID: "at0000"},
		{Path: "/category", WantRMType: "DV_CODED_TEXT"},
		{Path: "/content", WantRMType: "OBSERVATION"},
		{
			Path:            "/content[openEHR-EHR-OBSERVATION.blood_pressure.v1]",
			WantRMType:      "OBSERVATION",
			WantArchetypeID: "openEHR-EHR-OBSERVATION.blood_pressure.v1",
		},
		// At-code predicate — every OBSERVATION archetype root in
		// vital_signs.opt carries at0000 as its own node id. Exercises
		// the at-code branch of matchesPredicate (REQ-100 § Resolution
		// semantics).
		{Path: "/content[at0000]", WantRMType: "OBSERVATION", WantNodeID: "at0000"},
		{Path: "/no_such_attribute", ExpectNotFound: true},
		{Path: "/content[at9999]", ExpectNotFound: true},
	}
	r, err := probes.Probe022OPTPathResolution(body, assertions)
	if err != nil {
		t.Fatalf("Probe022: %v", err)
	}
	if r.Status != "pass" {
		t.Fatalf("Probe022 status=%q detail=%q", r.Status, r.Detail)
	}
	if r.Probe != "PROBE-022" {
		t.Errorf("Probe id = %q, want PROBE-022", r.Probe)
	}
}

// PROBE-022 — second fixture body (clinical_notes.v0). Confirms the
// probe runs against structurally distinct OPTs, not just one.
func TestProbe022OPTPathResolution_ClinicalNote(t *testing.T) {
	body := loadFixture(t, "clinical_note.opt")
	assertions := []probes.PathAssertion{
		{Path: "/", WantRMType: "COMPOSITION"},
		{
			Path:            "/content[openEHR-EHR-OBSERVATION.story.v1]",
			WantRMType:      "OBSERVATION",
			WantArchetypeID: "openEHR-EHR-OBSERVATION.story.v1",
		},
	}
	r, err := probes.Probe022OPTPathResolution(body, assertions)
	if err != nil {
		t.Fatalf("Probe022: %v", err)
	}
	if r.Status != "pass" {
		t.Fatalf("Probe022 status=%q detail=%q", r.Status, r.Detail)
	}
}

// PROBE-022 — contradiction precedence. An assertion that sets both
// ExpectNotFound and a positive want (WantRMType / WantNodeID /
// WantArchetypeID) is a caller bug, not a fixture mismatch. The
// probe MUST satisfy ExpectNotFound first (the negative branch
// short-circuits before positive-want checks run), so a path that
// genuinely does not exist passes regardless of the positive wants.
// Documents the precedence rule for harness authors.
func TestPathAssertion_PrecedenceContradiction(t *testing.T) {
	body := loadFixture(t, "vital_signs.opt")
	r, err := probes.Probe022OPTPathResolution(body, []probes.PathAssertion{
		{Path: "/no_such_attribute", ExpectNotFound: true, WantRMType: "DV_TEXT"},
	})
	if err != nil {
		t.Fatalf("Probe022: %v", err)
	}
	// ExpectNotFound short-circuits; the positive WantRMType is
	// ignored on the negative branch. Validates the documented
	// precedence: negative-first.
	if r.Status != "pass" {
		t.Fatalf("ExpectNotFound must short-circuit before WantRMType; status=%q detail=%q", r.Status, r.Detail)
	}
}

// PROBE-022 — malformed OPT MUST surface as a failed probe Result
// (not a Go error), so cross-SDK harnesses can aggregate failures.
func TestProbe022OPTPathResolution_InvalidOPT(t *testing.T) {
	r, err := probes.Probe022OPTPathResolution([]byte("<bad/>"), []probes.PathAssertion{{Path: "/"}})
	if err != nil {
		t.Fatalf("expected probe Result, got error: %v", err)
	}
	if r.Status != "fail" {
		t.Errorf("status = %q, want fail for invalid OPT", r.Status)
	}
}

// PROBE-022 — caller misuse (empty assertions) is a Go error, not a
// probe failure; harnesses MUST not silently pass an empty list.
func TestProbe022OPTPathResolution_RejectsEmptyAssertions(t *testing.T) {
	body := loadFixture(t, "vital_signs.opt")
	_, err := probes.Probe022OPTPathResolution(body, nil)
	if err == nil {
		t.Fatal("expected Go error for nil assertions")
	}
}

// PROBE-022 — ExpectNotFound MUST be satisfied by ErrPathNotFound
// specifically, not by any error type. Self-review finding #1 from
// PR #10 multi-agent review.
func TestProbe022OPTPathResolution_ExpectNotFoundRequiresSentinel(t *testing.T) {
	body := loadFixture(t, "vital_signs.opt")
	// A syntactically invalid path triggers ParsePath (ErrPathSyntax)
	// before NodeAt; the probe MUST report this as a parse failure,
	// not silently accept it as "not found".
	r, err := probes.Probe022OPTPathResolution(body, []probes.PathAssertion{
		{Path: "no_leading_slash", ExpectNotFound: true},
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if r.Status != "fail" {
		t.Errorf("status = %q, want fail (ParsePath error must not satisfy ExpectNotFound)", r.Status)
	}
}

func loadFixture(t *testing.T, name string) []byte {
	t.Helper()
	_, here, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate test source path")
	}
	repoRoot := filepath.Join(filepath.Dir(here), "..", "..", "..")
	path := filepath.Join(repoRoot, "openehr", "template", "testdata", name)
	body, err := os.ReadFile(path) //nolint:gosec // fixture path is test-controlled
	if err != nil {
		t.Fatalf("read fixture %s: %v", path, err)
	}
	return body
}
