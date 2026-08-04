package aqlprobes_test

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aql"
	"github.com/cadasto/openehr-sdk-go/openehr/aql/parse"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
	aqlprobes "github.com/cadasto/openehr-sdk-go/testkit/probes/aql"
)

// goldenWire reads the canonical reference-query golden owned by the aql
// package (openehr/aql/testdata/wire/).
func goldenWire(t *testing.T) string {
	t.Helper()
	return goldenWireNamed(t, "observations_by_archetype.aql")
}

// goldenWireNamed reads one wire golden owned by the aql package. The path is
// resolved relative to this test source file so it is independent of the
// working directory.
func goldenWireNamed(t *testing.T, name string) string {
	t.Helper()
	_, here, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	path := filepath.Join(filepath.Dir(here), "..", "..", "..",
		"openehr", "aql", "testdata", "wire", name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimRight(string(data), "\n")
}

func TestProbe020Passes(t *testing.T) {
	r, err := aqlprobes.Probe020BuilderStability(goldenWire(t))
	if err != nil {
		t.Fatalf("Probe020: %v", err)
	}
	if r.Status != "pass" {
		t.Fatalf("Probe020 status=%q detail=%q", r.Status, r.Detail)
	}
}

func TestProbe020DetectsGoldenDrift(t *testing.T) {
	r, err := aqlprobes.Probe020BuilderStability("SELECT x FROM EHR e")
	if err != nil {
		t.Fatalf("Probe020: %v", err)
	}
	if r.Status != "fail" {
		t.Fatalf("expected fail on golden drift, got status=%q", r.Status)
	}
}

// probe088Goldens reads one golden per construct PROBE-088 asserts.
func probe088Goldens(t *testing.T) map[string]string {
	t.Helper()
	goldens := make(map[string]string)
	for _, name := range aqlprobes.Probe088Constructs() {
		goldens[name] = goldenWireNamed(t, name+".aql")
	}
	return goldens
}

// PROBE-088 — builder containment algebra and in-text paging stability. The
// REQ-117 constructs emit their committed goldens, both builder styles agree,
// the PROBE-020 golden is unchanged, and combining the two paging channels is
// a build-time error.
func TestProbe088BuilderContainmentAndPaging(t *testing.T) {
	r, err := aqlprobes.Probe088BuilderContainmentAndPaging(probe088Goldens(t), goldenWire(t))
	if err != nil {
		t.Fatalf("Probe088: %v", err)
	}
	if r.Status != "pass" {
		t.Fatalf("Probe088 status=%q detail=%q", r.Status, r.Detail)
	}
	if r.Probe != "PROBE-088" {
		t.Errorf("Probe id = %q, want PROBE-088", r.Probe)
	}
}

func TestProbe088DetectsGoldenDrift(t *testing.T) {
	goldens := probe088Goldens(t)
	for name := range goldens {
		goldens[name] = "SELECT x FROM EHR e"
		break
	}
	r, err := aqlprobes.Probe088BuilderContainmentAndPaging(goldens, goldenWire(t))
	if err != nil {
		t.Fatalf("Probe088: %v", err)
	}
	if r.Status != "fail" {
		t.Fatalf("expected fail on golden drift, got status=%q", r.Status)
	}
}

// TestProbe088DetectsProbe020Drift locks the semver contract from the other
// side: a rewritten PROBE-020 golden fails PROBE-088 even though every REQ-117
// construct still matches (wire.md § REQ-055 — canonicalisation is semver).
func TestProbe088DetectsProbe020Drift(t *testing.T) {
	r, err := aqlprobes.Probe088BuilderContainmentAndPaging(probe088Goldens(t),
		"SELECT o FROM EHR e CONTAINS OBSERVATION o")
	if err != nil {
		t.Fatalf("Probe088: %v", err)
	}
	if r.Status != "fail" {
		t.Fatalf("expected fail on PROBE-020 golden drift, got status=%q", r.Status)
	}
}

// TestProbe020GoldenIsBytePinned asserts the committed PROBE-020 golden file
// equals the literal pinned in the probe package — the REQ-117 additions are
// semver-minor, so the pre-REQ-117 canonical form MUST be untouched.
func TestProbe020GoldenIsBytePinned(t *testing.T) {
	if got := goldenWire(t); got != aqlprobes.Probe020CanonicalQuery {
		t.Fatalf("PROBE-020 golden drifted:\n  file: %q\npinned: %q", got, aqlprobes.Probe020CanonicalQuery)
	}
}

// TestProbe088GoldensRoundTripThroughParse ties PROBE-088 to PROBE-087: every
// committed golden MUST parse with no catalogue gap (no aql.ErrIncompleteAST)
// and re-emit to the same bytes, so the builder's canonical form and the
// parser's are provably the same form (REQ-117).
func TestProbe088GoldensRoundTripThroughParse(t *testing.T) {
	for name, golden := range probe088Goldens(t) {
		t.Run(name, func(t *testing.T) {
			doc, err := parse.Parse(golden)
			if err != nil {
				t.Fatalf("Parse(%q): %v", golden, err)
			}
			if qerr := doc.QueryErr(); qerr != nil {
				t.Fatalf("QueryErr = %v (incomplete AST: %t), want nil",
					qerr, errors.Is(qerr, aql.ErrIncompleteAST))
			}
			emitted, err := doc.Query().Emit()
			if err != nil {
				t.Fatalf("Emit: %v", err)
			}
			if emitted != golden {
				t.Fatalf("golden is not the parser's canonical form:\n golden: %s\n  parse: %s", golden, emitted)
			}
		})
	}
}

// cassette reads an AQL lint cassette under testkit/cassettes/aql/lint/.
func cassette(t *testing.T, name string) string {
	t.Helper()
	_, here, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	path := filepath.Join(filepath.Dir(here), "..", "..", "cassettes", "aql", "lint", name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimRight(string(data), "\n")
}

func loadOPT(t *testing.T, name string) []byte {
	t.Helper()
	body, err := os.ReadFile(fixtures.TemplateOptForName(name))
	if err != nil {
		t.Fatalf("read OPT %q: %v", name, err)
	}
	return body
}

// PROBE-028 — AQL lint stability. Each cassette query, linted against the SDK
// grammar profile (+ vital_signs.opt for the template-aware cases), yields a
// stable issue-code multiset.
func TestProbe028AQLLint(t *testing.T) {
	opt := loadOPT(t, "vital_signs")
	cases := []aqlprobes.LintCase{
		{
			Name:      "valid",
			OPT:       opt,
			Query:     cassette(t, "valid.aql"),
			WantCodes: nil, // clean
		},
		{
			Name:      "missing_archetype",
			OPT:       opt,
			Query:     cassette(t, "missing_archetype.aql"),
			WantCodes: []string{"aql_archetype_not_in_template"},
		},
		{
			Name:      "bad_syntax",
			OPT:       nil, // Layer 1 only
			Query:     cassette(t, "bad_syntax.aql"),
			WantCodes: []string{"aql_syntax"},
		},
	}
	r, err := aqlprobes.Probe028AQLLint(cases)
	if err != nil {
		t.Fatalf("Probe028: %v", err)
	}
	if r.Status != "pass" {
		t.Fatalf("Probe028 status=%q detail=%q", r.Status, r.Detail)
	}
	if r.Probe != "PROBE-028" {
		t.Errorf("Probe id = %q, want PROBE-028", r.Probe)
	}
}

func TestProbe028DetectsCodeDrift(t *testing.T) {
	cases := []aqlprobes.LintCase{
		{
			Name:      "syntax_expected_clean",
			Query:     cassette(t, "bad_syntax.aql"),
			WantCodes: nil, // wrong on purpose — bad_syntax yields aql_syntax
		},
	}
	r, err := aqlprobes.Probe028AQLLint(cases)
	if err != nil {
		t.Fatalf("Probe028: %v", err)
	}
	if r.Status != "fail" {
		t.Fatalf("expected fail on code drift, got status=%q", r.Status)
	}
}
