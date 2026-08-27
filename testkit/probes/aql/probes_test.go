package aqlprobes_test

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aql"
	"github.com/cadasto/openehr-sdk-go/openehr/aql/lint"
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

// PROBE-088 — builder containment algebra, in-text paging, and (since REQ-163)
// the version-predicate, standing-predicate and typed-projection carriers. Each
// construct emits its committed golden, both builder styles agree, the
// PROBE-020 golden is unchanged, and combining the two paging channels — or
// emitting a SELECT that does not read back as the recorded projection — is a
// build-time error.
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
//
// Since REQ-163 the goldens include that REQ's three write-side carriers — the
// VERSION predicate bracket, the class-position standing comparison and the
// typed projection — so this is also where their byte-IDENTITY duty is met on the
// committed files (REQ-163 § Read-side mirror duty); the corpus grows with
// [aqlprobes.Probe088Constructs], so a new construct is picked up here for free.
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

// probe028Cases is PROBE-028's own three-cassette corpus — the cassette
// files under testkit/cassettes/aql/lint/, the vital_signs.opt template, and
// their pre-REQ-161 WantCodes baseline. Shared between TestProbe028AQLLint
// and PROBE-097 arm (b) so a deliberate PROBE-028 re-baseline (conformance.md
// § PROBE-097 arm (b)) is made ONCE: before this helper existed, arm (b)
// hand-copied this table, so a re-baseline of one would need to be applied
// twice, and a missed second edit would surface as a confusing
// "additivity/…" failure in PROBE-097 rather than where the change actually
// happened.
func probe028Cases(t *testing.T) []aqlprobes.LintCase {
	t.Helper()
	opt := loadOPT(t, "vital_signs")
	return []aqlprobes.LintCase{
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
}

// PROBE-028 — AQL lint stability. Each cassette query, linted against the SDK
// grammar profile (+ vital_signs.opt for the template-aware cases), yields a
// stable issue-code multiset.
func TestProbe028AQLLint(t *testing.T) {
	r, err := aqlprobes.Probe028AQLLint(probe028Cases(t))
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

// --- PROBE-097 — REQ-160/161/162 semantic and portability lint corpus ------
//
// Arm (a): each of the eight REQ-161 codes fires on a corpus query built to
// carry exactly that defect, with the REQ-161 severity and a span on the
// offending class expression, and stays silent on a near miss. The three
// mandatory negative cases the PROBE-097 wire assertion names explicitly —
// unknown-class suppression, archetype-mismatch suppression, and the
// aql_fanout_row_grain conservative firing rule — are pinned by name in
// probe097SilentCases.
//
// Arm (b): the PROBE-028 corpus (the exact three cassettes
// TestProbe028AQLLint above already wires) re-run under the completed
// REQ-161 linter gains no REQ-161 code — the controller's own precomputed
// prediction, recorded in docs/specifications/conformance.md's PROBE-097 row
// as "no re-baseline required".
//
// Arm (c), REQ-162 § Contract: for every corpus query expressible through the
// builder, (*aql.Builder).VerifyContainment's code multiset equals
// lint.LintString's containment-code subset over the emitted text.
// VerifyContainment never emits the three portability codes at all — REQ-162
// § Contract scopes parity to the five containment codes — so they have no
// row in probe097ParityCases by scope, not by omission; a builder QUERY can
// still trip one of the three (e.g. a bare VERSION operand raises
// aql_version_no_predicate on the read side), it is only the write-side
// finding that has no counterpart.
//
// What arm (c) can express WIDENED at REQ-163: the class-position version
// predicate, the class-position standing comparison and the typed projection
// are builder constructs now, so the REQ-163 rows at the end of
// probe097ParityCases carry those brackets through the same parity comparison
// — a bracket either side read differently would show up as a code the other
// does not report. What stays out of reach is the FROM ROOT: openehr/aql's FROM
// clause carries an RM type and an alias and nothing else, so a root archetype
// predicate (openehr/aql/verify.go's own doc comment records this), a root
// standing predicate (REQ-055 rule 6 keeps the WHERE spelling deliberately) and
// a root version predicate are all unspellable, before REQ-163 and after it.
// That is why arm (a)'s archetype-mismatch rows below are spelled in CONTAINS
// position throughout: that is the one spelling both arm (a) and arm (c) can
// share, rather than two different queries pinning the same code.

// probe097FireCases is PROBE-097 arm (a)'s firing table: one row per REQ-161
// code. Severities are REQ-161 § Checks, verbatim: impossible containment,
// not-containable, and archetype/class mismatch are Errors; the other five
// codes are Warnings.
func probe097FireCases() []aqlprobes.SemanticFireCase {
	return []aqlprobes.SemanticFireCase{
		{
			Name:      "impossible containment",
			Query:     "SELECT o FROM OBSERVATION o CONTAINS COMPOSITION c",
			Code:      "aql_impossible_containment",
			Severity:  lint.Error,
			SpanClass: "COMPOSITION",
			SpanNth:   1,
		},
		{
			Name:      "contains not containable",
			Query:     "SELECT c FROM COMPOSITION c CONTAINS DV_TEXT t",
			Code:      "aql_contains_not_containable",
			Severity:  lint.Error,
			SpanClass: "DV_TEXT",
			SpanNth:   1,
		},
		{
			// CONTAINS position (not the FROM root openehr/aql/lint/semantic_test.go
			// uses) so this row doubles as an arm-(c) parity row — see the
			// section doc comment above.
			Name:      "archetype class mismatch",
			Query:     "SELECT ev FROM COMPOSITION c CONTAINS EVALUATION ev[openEHR-EHR-OBSERVATION.body_temperature.v2]",
			Code:      "aql_archetype_class_mismatch",
			Severity:  lint.Error,
			SpanClass: "EVALUATION",
			SpanNth:   1,
		},
		{
			Name:      "unknown RM class",
			Query:     "SELECT c FROM COMPOSITION c CONTAINS FOO_BAR f",
			Code:      "aql_unknown_rm_class",
			Severity:  lint.Warning,
			SpanClass: "FOO_BAR",
			SpanNth:   1,
		},
		{
			Name:      "containment by reference",
			Query:     "SELECT c FROM FOLDER f CONTAINS COMPOSITION c",
			Code:      "aql_containment_by_reference",
			Severity:  lint.Warning,
			SpanClass: "COMPOSITION",
			SpanNth:   1,
		},
		{
			Name:      "VERSION with no version predicate",
			Query:     "SELECT v FROM VERSION v",
			Code:      "aql_version_no_predicate",
			Severity:  lint.Warning,
			SpanClass: "VERSION",
			SpanNth:   1,
		},
		{
			Name:      "VERSIONED_OBJECT operand unreferenced",
			Query:     "SELECT 1 FROM VERSIONED_COMPOSITION vo",
			Code:      "aql_versioned_object_unreferenced",
			Severity:  lint.Warning,
			SpanClass: "VERSIONED_COMPOSITION",
			SpanNth:   1,
		},
		{
			Name:      "AND junction fan-out row grain",
			Query:     "SELECT o1, o2 FROM COMPOSITION c CONTAINS (OBSERVATION o1 AND OBSERVATION o2)",
			Code:      "aql_fanout_row_grain",
			Severity:  lint.Warning,
			SpanClass: "OBSERVATION",
			SpanNth:   1, // the first (document-order) projected leaf, o1
		},
		{
			// The row that ARMS the probe's own boundary check: "OBSERVATION"
			// occurs three times in this query, and the middle one is a fragment
			// of the archetype HRID. Only a boundary-checked search skips it, so
			// SpanNth 2 means the CONTAINS operand here and the HRID fragment if
			// classSpan's boundaryOK call is ever removed — the exact case that
			// helper exists for, which no other corpus row reaches. The finding
			// itself is ordinary: OBSERVATION cannot contain OBSERVATION, and the
			// conforming archetype on the root raises nothing.
			Name:      "impossible containment past an HRID fragment of the span class",
			Query:     "SELECT o FROM OBSERVATION src[openEHR-EHR-OBSERVATION.body_temperature.v2] CONTAINS OBSERVATION o",
			Code:      "aql_impossible_containment",
			Severity:  lint.Error,
			SpanClass: "OBSERVATION",
			SpanNth:   2, // the CONTAINS operand; the HRID fragment is not an occurrence
		},
	}
}

// probe097SilentCases is PROBE-097 arm (a)'s near-miss and
// suppression-negative table — one plain near miss per code, plus the three
// mandatory negative cases the wire assertion names by name.
//
// ForCode / Mandatory are not decoration: they are what the probe's own
// completeness guards count, so the near-miss half of arm (a) cannot shrink
// below "one silence row per REQ-161 code, and all three named negatives"
// while still reporting pass. A row that guards no code in particular leaves
// ForCode empty.
func probe097SilentCases() []aqlprobes.SemanticSilentCase {
	return []aqlprobes.SemanticSilentCase{
		{
			Name:    "near miss: admissible pair",
			Query:   "SELECT o FROM COMPOSITION c CONTAINS OBSERVATION o",
			ForCode: "aql_impossible_containment",
		},
		{
			Name:    "near miss: containable target",
			Query:   "SELECT c FROM COMPOSITION c CONTAINS ELEMENT e",
			ForCode: "aql_contains_not_containable",
		},
		{
			Name:    "near miss: conforming archetype",
			Query:   "SELECT o FROM COMPOSITION c CONTAINS OBSERVATION o[openEHR-EHR-OBSERVATION.body_temperature.v2]",
			ForCode: "aql_archetype_class_mismatch",
		},
		{
			// Distinct shape from "near miss: containable target" above: both
			// were otherwise "COMPOSITION CONTAINS <containable class>" and moved
			// together under every mutation. This one adds a second, deeper hop
			// (CLUSTER CONTAINS ELEMENT) so the two rows exercise a different
			// pair. It carries the aql_unknown_rm_class claim — every class it
			// names is a KNOWN one, which is that code's near miss.
			Name:    "near miss: known containable class",
			Query:   "SELECT c FROM COMPOSITION c CONTAINS CLUSTER cl CONTAINS ELEMENT e2",
			ForCode: "aql_unknown_rm_class",
		},
		{
			Name:    "near miss: no reference hop",
			Query:   "SELECT f FROM FOLDER f CONTAINS FOLDER f2",
			ForCode: "aql_containment_by_reference",
		},
		{
			Name:    "near miss: explicit version tier",
			Query:   "SELECT v FROM VERSION v[LATEST_VERSION]",
			ForCode: "aql_version_no_predicate",
		},
		{
			Name:    "near miss: VERSIONED_OBJECT operand referenced",
			Query:   "SELECT vo/uid/value FROM VERSIONED_COMPOSITION vo",
			ForCode: "aql_versioned_object_unreferenced",
		},
		{
			// Mandatory negative case 3 (REQ-161's own named near miss): the
			// aql_fanout_row_grain conservative firing rule needs >= 2 projected
			// leaves; one is below the threshold. Want stays nil — unlike the two
			// suppression negatives, nothing survives this near miss.
			Name:      "mandatory negative: fan-out below the >=2 threshold",
			Query:     "SELECT o1 FROM COMPOSITION c CONTAINS (OBSERVATION o1 AND OBSERVATION o2)",
			ForCode:   "aql_fanout_row_grain",
			Mandatory: aqlprobes.NegFanoutConservativeFiring,
		},
		{
			Name:    "near miss: OR junction never fires the fan-out advisory",
			Query:   "SELECT o1, o2 FROM COMPOSITION c CONTAINS (OBSERVATION o1 OR OBSERVATION o2)",
			ForCode: "aql_fanout_row_grain",
		},
		{
			// Mandatory negative case 1: FOO_BAR, an unknown class, suppresses
			// BOTH of its adjacent pair checks — OBSERVATION->FOO_BAR and
			// FOO_BAR->COMPOSITION — leaving only its own aql_unknown_rm_class
			// finding. (There is no transitive OBSERVATION->COMPOSITION pair to
			// suppress: semcheck.Checker.Pair's own doc comment is explicit that
			// the walk is adjacency-only and a transitive pair MUST NOT be
			// synthesised, so that pair is never formed at all, suppressed or
			// not — an earlier version of this comment claimed otherwise.)
			//
			// This row's arm is redundant-by-construction, not fragile: REQ-160
			// makes the pair question total, so a pair with an unknown operand
			// answers UnknownClass regardless of Operand.Suppresses, and
			// Checker.Pair's own switch already returns nil for that verdict —
			// neutering Suppresses entirely does not change this row's outcome
			// (confirmed: only "fire/contains not containable" fails under that
			// mutation, recorded with the REQ-161 suppression rule it concerns).
			// The Suppresses mechanism itself IS pinned observably, just not by
			// this flavour of operand — see "mandatory negative: unknown-class
			// suppression (Never-arm observable pin)" below for the Never-verdict
			// flavour that does arm, and semcheck_test.go's
			// TestPairSuppressedByOperandVerdict for the engine-level pin of both
			// flavours directly.
			Name:      "mandatory negative: unknown-class suppression",
			Query:     "SELECT c FROM OBSERVATION o CONTAINS FOO_BAR f CONTAINS COMPOSITION c",
			Want:      []string{"aql_unknown_rm_class"},
			Mandatory: aqlprobes.NegUnknownClassSuppression,
		},
		{
			// The sibling row above names "unknown-class suppression" but cannot
			// fail on a mutation of the rule it names (Operand.Suppresses ->
			// return false), because REQ-160 totality already makes an unknown
			// operand's pair answer UnknownClass with or without that check. This
			// row pins the OTHER arm of the same Operand.Suppresses condition
			// (verdict != Admissible): a Never-verdict operand (DV_TEXT, not a
			// containment target) suppressing the pair checks on BOTH sides of
			// it. Confirmed: today's multiset is exactly
			// [aql_contains_not_containable]; under the Suppresses->false
			// mutation it gains aql_impossible_containment (from the
			// no-longer-suppressed COMPOSITION->DV_TEXT and DV_TEXT->ELEMENT
			// pairs), so this row genuinely arms.
			Name:      "mandatory negative: unknown-class suppression (Never-arm observable pin)",
			Query:     "SELECT c FROM COMPOSITION c CONTAINS DV_TEXT t CONTAINS ELEMENT e",
			Want:      []string{"aql_contains_not_containable"},
			Mandatory: aqlprobes.NegUnknownClassSuppression,
		},
		{
			// Mandatory negative case 2, declared-class arm: an unknown declared
			// class carrying a literal archetype predicate reports
			// aql_unknown_rm_class once, never the mismatch.
			Name:      "mandatory negative: archetype-mismatch suppression (unknown declared class)",
			Query:     "SELECT x FROM COMPOSITION c CONTAINS FOO_BAR x[openEHR-EHR-OBSERVATION.blood_pressure.v1]",
			Want:      []string{"aql_unknown_rm_class"},
			Mandatory: aqlprobes.NegArchetypeMismatchSuppression,
		},
		{
			// Mandatory negative case 2, HRID-type-segment arm: an unknown
			// archetype type segment on a KNOWN declared class also reports
			// only aql_unknown_rm_class.
			Name:      "mandatory negative: archetype-mismatch suppression (unknown HRID type segment)",
			Query:     "SELECT ev FROM COMPOSITION c CONTAINS EVALUATION ev[openEHR-EHR-FOOTYPE.x.v1]",
			Want:      []string{"aql_unknown_rm_class"},
			Mandatory: aqlprobes.NegArchetypeMismatchSuppression,
		},
	}
}

// probe097AdditivityCases is PROBE-097 arm (b): PROBE-028's own corpus
// ([probe028Cases], shared with TestProbe028AQLLint as noted there),
// unchanged, re-run under the completed REQ-161 linter. WantCodes is the
// pre-REQ-161 baseline from that same table; the controller ran all three
// through the completed linter and found zero REQ-161 delta, so the
// baseline is asserted unchanged rather than re-derived.
func probe097AdditivityCases(t *testing.T) []aqlprobes.LintCase {
	t.Helper()
	return probe028Cases(t)
}

// probe097ParityCases is PROBE-097 arm (c): the subset of probe097FireCases /
// probe097SilentCases expressible through the builder, translated into the
// equivalent containment tree. Every one of the five containment codes is
// exercised, including both mandatory-suppression-negative shapes — but this
// does NOT independently pin either suppression rule on the write side: one
// shared engine (openehr/aql/internal/semcheck) backs both adapters, so a
// mutation to the suppression logic moves both adapters together and these
// rows stay green regardless. What
// they actually pin is read/write PARITY for these shapes — that
// VerifyContainment reports aql_unknown_rm_class for them too, exactly as
// LintString does. The three portability codes are out of REQ-162
// § Contract's scope entirely (never producible by VerifyContainment), so
// they have no row here.
func probe097ParityCases() []aqlprobes.ParityCase {
	return []aqlprobes.ParityCase{
		{Name: "impossible containment", Build: func() *aql.Builder {
			return aql.NewBuilder().Select(aql.Col("o")).From("OBSERVATION", "o").
				Contains(aql.Class("COMPOSITION", "c"))
		}},
		{Name: "near miss: admissible pair", Build: func() *aql.Builder {
			return aql.NewBuilder().Select(aql.Col("o")).From("COMPOSITION", "c").
				Contains(aql.Class("OBSERVATION", "o"))
		}},
		{Name: "contains not containable", Build: func() *aql.Builder {
			return aql.NewBuilder().Select(aql.Col("c")).From("COMPOSITION", "c").
				Contains(aql.Class("DV_TEXT", "t"))
		}},
		{Name: "near miss: containable target", Build: func() *aql.Builder {
			return aql.NewBuilder().Select(aql.Col("c")).From("COMPOSITION", "c").
				Contains(aql.Class("ELEMENT", "e"))
		}},
		{Name: "archetype class mismatch", Build: func() *aql.Builder {
			return aql.NewBuilder().Select(aql.Col("ev")).From("COMPOSITION", "c").
				Contains(aql.Archetype("EVALUATION", "ev", "openEHR-EHR-OBSERVATION.body_temperature.v2"))
		}},
		{Name: "near miss: conforming archetype", Build: func() *aql.Builder {
			return aql.NewBuilder().Select(aql.Col("o")).From("COMPOSITION", "c").
				Contains(aql.Archetype("OBSERVATION", "o", "openEHR-EHR-OBSERVATION.body_temperature.v2"))
		}},
		{Name: "unknown RM class", Build: func() *aql.Builder {
			return aql.NewBuilder().Select(aql.Col("c")).From("COMPOSITION", "c").
				Contains(aql.Class("FOO_BAR", "f"))
		}},
		{
			// Mirrors the deeper-chain shape given to the "near miss: known
			// containable class" arm-(a) row so the two stay aligned.
			Name: "near miss: known containable class", Build: func() *aql.Builder {
				return aql.NewBuilder().Select(aql.Col("c")).From("COMPOSITION", "c").
					Contains(aql.Class("CLUSTER", "cl").Contains(aql.Class("ELEMENT", "e2")))
			},
		},
		{Name: "containment by reference", Build: func() *aql.Builder {
			return aql.NewBuilder().Select(aql.Col("c")).From("FOLDER", "f").
				Contains(aql.Class("COMPOSITION", "c"))
		}},
		{Name: "near miss: no reference hop", Build: func() *aql.Builder {
			return aql.NewBuilder().Select(aql.Col("f")).From("FOLDER", "f").
				Contains(aql.Class("FOLDER", "f2"))
		}},
		{Name: "mandatory negative: unknown-class suppression", Build: func() *aql.Builder {
			return aql.NewBuilder().Select(aql.Col("c")).From("OBSERVATION", "o").
				Contains(aql.Class("FOO_BAR", "f").Contains(aql.Class("COMPOSITION", "c")))
		}},
		{
			Name: "mandatory negative: unknown-class suppression (Never-arm observable pin)",
			Build: func() *aql.Builder {
				return aql.NewBuilder().Select(aql.Col("c")).From("COMPOSITION", "c").
					Contains(aql.Class("DV_TEXT", "t").Contains(aql.Class("ELEMENT", "e")))
			},
		},
		{Name: "mandatory negative: archetype-mismatch suppression (unknown declared class)", Build: func() *aql.Builder {
			return aql.NewBuilder().Select(aql.Col("x")).From("COMPOSITION", "c").
				Contains(aql.Archetype("FOO_BAR", "x", "openEHR-EHR-OBSERVATION.blood_pressure.v1"))
		}},
		{Name: "mandatory negative: archetype-mismatch suppression (unknown HRID type segment)", Build: func() *aql.Builder {
			return aql.NewBuilder().Select(aql.Col("ev")).From("COMPOSITION", "c").
				Contains(aql.Archetype("EVALUATION", "ev", "openEHR-EHR-FOOTYPE.x.v1"))
		}},

		// --- REQ-163: the rows that were builder-inexpressible ----------------
		//
		// Each carries a construct no builder program could spell before REQ-163,
		// through the same parity comparison as the rows above. The point is not
		// a new code — these shapes are containment-clean or carry a code the
		// corpus already covers — but that the new brackets and the new
		// projection do not make the two sides disagree: the read side re-parses
		// the emitted bracket text while the write side walks the recorded
		// carrier, so a bracket either side reads differently would surface as a
		// code the other does not report.
		{
			// REQ-161's motivating suppression shape, whose whole point is that
			// it needs both class-position carriers at once. Containment-clean on
			// both sides: EHR -> VERSIONED_COMPOSITION -> VERSION -> COMPOSITION
			// is the admissible version-tier route REQ-160's overlay table names.
			//
			// The "near miss:" prefix is the corpus's clean-row convention and is
			// load-bearing: TestProbe097GuardsCanFail's non-vacuity control
			// deletes every row carrying it, and a clean row spelled otherwise
			// would leave that control passing on a corpus it meant to empty.
			// It is also accurate — this is arm (a)'s
			// "near miss: VERSIONED_OBJECT operand referenced", now reachable
			// from the builder.
			Name: "near miss: VERSIONED_OBJECT referenced by a standing predicate (REQ-163)", Build: func() *aql.Builder {
				return aql.NewBuilder().Select(aql.Col("c/uid/value")).
					FromEHR("e", aql.Param("ehr_id")).
					Contains(aql.Class("VERSIONED_COMPOSITION", "vo").
						Predicated("uid/value", aql.OpEq, aql.Param("vo")).
						Contains(aql.Version("v", aql.AllVersions()).
							Contains(aql.Class("COMPOSITION", "c"))))
			},
		},
		{
			// The version-predicate carrier alone, in the shape that satisfies
			// the aql_version_no_predicate advisory the builder could not
			// previously satisfy at all. The advisory itself is a portability
			// code and out of arm (c)'s scope; what this row checks is that the
			// bracket leaves the CONTAINMENT verdicts alone on both sides.
			// Clean, so it carries the "near miss:" prefix for the reason the
			// row above records — and it is arm (a)'s
			// "near miss: explicit version tier" in CONTAINS position.
			Name: "near miss: explicit version tier in CONTAINS position (REQ-163)", Build: func() *aql.Builder {
				return aql.NewBuilder().Select(aql.Col("c/uid/value")).
					FromEHR("e", aql.Param("ehr_id")).
					Contains(aql.Version("v", aql.LatestVersion()).
						Contains(aql.Class("COMPOSITION", "c")))
			},
		},
		{
			// A standing predicate on a node whose PAIR is impossible: the
			// bracket must not hide the finding from either side. Mirrors the
			// "impossible containment" row above, with the new bracket added.
			Name: "REQ-163: a standing predicate on an impossible pair", Build: func() *aql.Builder {
				return aql.NewBuilder().Select(aql.Col("o")).From("OBSERVATION", "o").
					Contains(aql.Class("COMPOSITION", "c").
						Predicated("name/value", aql.OpEq, aql.String("Vital signs")))
			},
		},
		{
			// The typed projection over an unknown class: SELECT is the clause
			// REQ-163 rewrote, and the containment verdicts must be indifferent
			// to it. DISTINCT and an aggregate together, so the clause-level flag
			// rides along.
			Name: "REQ-163: a typed projection over an unknown class", Build: func() *aql.Builder {
				return aql.NewBuilder().Select(aql.CountStar().As("n")).Distinct().
					From("COMPOSITION", "c").
					Contains(aql.Class("FOO_BAR", "f"))
			},
		},
	}
}

// probe097Corpus assembles the full PROBE-097 corpus. Shared between
// TestProbe097SemanticLint and the guard controls below, which each mutate one
// arm of it — a control built from a hand-written stand-in would prove the
// guard fires on a toy corpus, not that it fires on the corpus that ships.
func probe097Corpus(t *testing.T) aqlprobes.SemanticCorpus {
	t.Helper()
	return aqlprobes.SemanticCorpus{
		Fire:       probe097FireCases(),
		Silent:     probe097SilentCases(),
		Additivity: probe097AdditivityCases(t),
		Parity:     probe097ParityCases(),
	}
}

// TestProbe097SemanticLint runs PROBE-097's full corpus — all three wire
// assertion arms — and asserts a clean pass.
func TestProbe097SemanticLint(t *testing.T) {
	r, err := aqlprobes.Probe097SemanticLint(probe097Corpus(t))
	if err != nil {
		t.Fatalf("Probe097: %v", err)
	}
	if r.Status != "pass" {
		t.Fatalf("Probe097 status=%q detail=%q", r.Status, r.Detail)
	}
	if r.Probe != "PROBE-097" {
		t.Errorf("Probe id = %q, want PROBE-097", r.Probe)
	}
}

// TestProbe097DetectsFireCodeDrift is the probe's own able-to-fail control:
// a firing row wired to the wrong code must fail the probe, not pass it
// silently.
func TestProbe097DetectsFireCodeDrift(t *testing.T) {
	// Additivity and Parity below are deliberately
	// minimal, one clean row each — NOT the full probe097AdditivityCases /
	// probe097ParityCases corpora. Running the full corpora here let this
	// control pass under an UNRELATED mutation (e.g. VerifyContainment ->
	// nil breaks real parity rows too), so a fail here could mean "fire-code
	// drift" or "something else entirely" and this control would not have
	// noticed the difference. Asserting Detail names the drifted fire row
	// (not just Status == "fail") closes that gap directly.
	corpus := aqlprobes.SemanticCorpus{
		Fire: []aqlprobes.SemanticFireCase{
			{
				Name:      "wrong code on purpose",
				Query:     "SELECT o FROM OBSERVATION o CONTAINS COMPOSITION c", // actually raises aql_impossible_containment
				Code:      "aql_contains_not_containable",
				Severity:  lint.Error,
				SpanClass: "COMPOSITION",
				SpanNth:   1,
			},
		},
		Silent: []aqlprobes.SemanticSilentCase{{Name: "clean", Query: "SELECT c FROM COMPOSITION c"}},
		Additivity: []aqlprobes.LintCase{
			{Name: "clean", Query: "SELECT c FROM COMPOSITION c", WantCodes: nil},
		},
		Parity: []aqlprobes.ParityCase{
			{Name: "clean pair", Build: func() *aql.Builder {
				return aql.NewBuilder().Select(aql.Col("o")).From("COMPOSITION", "c").
					Contains(aql.Class("OBSERVATION", "o"))
			}},
		},
	}
	r, err := aqlprobes.Probe097SemanticLint(corpus)
	if err != nil {
		t.Fatalf("Probe097: %v", err)
	}
	if r.Status != "fail" {
		t.Fatalf("expected fail on fire-code drift, got status=%q", r.Status)
	}
	if !strings.Contains(r.Detail, "fire/wrong code on purpose") {
		t.Fatalf("expected failure detail to name the drifted fire row (%q), got %q",
			"fire/wrong code on purpose", r.Detail)
	}
}

// --- PROBE-097 guard controls ----------------------------------------------
//
// TestProbe097DetectsFireCodeDrift above is the able-to-fail control for ONE
// path, the arm-(a) code multiset: it runs a one-row corpus, so it trips the
// completeness guards incidentally while asserting only that Detail names the
// drifted row. The guards themselves — the machinery that stops the corpus
// rotting — and the severity / span / silence assertions had no control at
// all: each could be deleted with `go test ./testkit/...` still green.
//
// The controls below close that. Each mutates the SHIPPING corpus in exactly
// the way one guard exists to catch (a toy stand-in would only prove the guard
// fires on a toy corpus) and asserts that guard's OWN message — a control
// asserting nothing but Status == "fail" passes on any failure, including one
// it caused for an unrelated reason.

// deleteRows returns s without the rows matching del, failing the test when
// nothing matched: a control that deletes nothing proves nothing, and a
// predicate keyed on a renamed row would silently become that.
func deleteRows[T any](t *testing.T, s []T, what string, del func(T) bool) []T {
	t.Helper()
	out := slices.DeleteFunc(slices.Clone(s), del)
	if len(out) == len(s) {
		t.Fatalf("no %s matched; this control has rotted", what)
	}
	return out
}

// fireRow addresses the named fire row so a control can mutate it in place,
// failing the test rather than no-opping when the row is gone.
func fireRow(t *testing.T, rows []aqlprobes.SemanticFireCase, name string) *aqlprobes.SemanticFireCase {
	t.Helper()
	i := slices.IndexFunc(rows, func(c aqlprobes.SemanticFireCase) bool { return c.Name == name })
	if i < 0 {
		t.Fatalf("no fire row named %q; this control has rotted", name)
	}
	return &rows[i]
}

const boundaryFireRow = "impossible containment past an HRID fragment of the span class"

func TestProbe097GuardsCanFail(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, *aqlprobes.SemanticCorpus)
		want   string // substring of Detail the guard under control must produce
	}{
		{
			// Arm (a), fire half: the per-code coverage guard.
			name: "a fire row is deleted",
			mutate: func(t *testing.T, c *aqlprobes.SemanticCorpus) {
				c.Fire = deleteRows(t, c.Fire, "fan-out fire row", func(f aqlprobes.SemanticFireCase) bool {
					return f.Code == "aql_fanout_row_grain"
				})
			},
			want: "fire: no fire case raises aql_fanout_row_grain",
		},
		{
			// Arm (a), silence half: the mirror per-code coverage guard.
			name: "the silence row for a code is deleted",
			mutate: func(t *testing.T, c *aqlprobes.SemanticCorpus) {
				c.Silent = deleteRows(t, c.Silent, "version-predicate silence row", func(s aqlprobes.SemanticSilentCase) bool {
					return s.ForCode == "aql_version_no_predicate"
				})
			},
			want: "silent: no silence case guards aql_version_no_predicate",
		},
		{
			// Arm (a), silence half: the three negatives the wire assertion
			// names explicitly, which no per-code count would notice the loss of
			// (aql_archetype_class_mismatch keeps its own near-miss row here).
			name: "a mandatory suppression negative is deleted",
			mutate: func(t *testing.T, c *aqlprobes.SemanticCorpus) {
				c.Silent = deleteRows(t, c.Silent, "archetype-mismatch suppression row", func(s aqlprobes.SemanticSilentCase) bool {
					return s.Mandatory == aqlprobes.NegArchetypeMismatchSuppression
				})
			},
			want: "silent: no silence case pins the archetype-mismatch suppression negative",
		},
		{
			// Arm (a), silence half: FIX for the vacuous-silence hole — a row
			// whose query fails Layer 1 never reaches a semantic check, so its
			// nil-vs-nil comparison asserted nothing before this guard.
			name: "a silence row asserts silence on a query that never parsed",
			mutate: func(_ *testing.T, c *aqlprobes.SemanticCorpus) {
				c.Silent = append(c.Silent, aqlprobes.SemanticSilentCase{
					Name:  "never parsed",
					Query: "SELECT o FROM OBSERVATION o CONTAINS", // dangling CONTAINS: aql_syntax
				})
			},
			want: "silent/never parsed: query never reached the REQ-161 checks (aql_syntax)",
		},
		{
			// Arm (a), silence half: the comparison itself.
			name: "a silence row wants a code its query does not raise",
			mutate: func(_ *testing.T, c *aqlprobes.SemanticCorpus) {
				c.Silent = append(c.Silent, aqlprobes.SemanticSilentCase{
					Name:  "wrong want",
					Query: "SELECT o FROM COMPOSITION c CONTAINS OBSERVATION o",
					Want:  []string{"aql_unknown_rm_class"},
				})
			},
			want: "silent/wrong want: semantic codes = []",
		},
		{
			// Arm (a), fire half: the severity comparison, uncovered until now —
			// an inverted comparison here would have been invisible.
			name: "a fire row carries the wrong severity",
			mutate: func(t *testing.T, c *aqlprobes.SemanticCorpus) {
				fireRow(t, c.Fire, "impossible containment").Severity = lint.Warning
			},
			want: "fire/impossible containment: aql_impossible_containment severity =",
		},
		{
			// Arm (a), fire half: the span comparison. SpanNth 1 on the
			// boundary row anchors on the FROM root while the finding spans the
			// CONTAINS operand.
			name: "a fire row is anchored on the wrong occurrence",
			mutate: func(t *testing.T, c *aqlprobes.SemanticCorpus) {
				fireRow(t, c.Fire, boundaryFireRow).SpanNth = 1
			},
			want: "aql_impossible_containment span =",
		},
		{
			// Arm (a), fire half: classSpan's own error path — an occurrence the
			// query does not have is reported, never silently treated as absent.
			name: "a fire row names an occurrence the query does not have",
			mutate: func(t *testing.T, c *aqlprobes.SemanticCorpus) {
				fireRow(t, c.Fire, boundaryFireRow).SpanNth = 3
			},
			want: `fewer than 3 boundary-matched occurrences of "OBSERVATION"`,
		},
		{
			// Arm (c): the containment-code union guard.
			name: "the parity row for a containment code is deleted",
			mutate: func(t *testing.T, c *aqlprobes.SemanticCorpus) {
				c.Parity = deleteRows(t, c.Parity, "containment-by-reference parity row", func(p aqlprobes.ParityCase) bool {
					return p.Name == "containment by reference"
				})
			},
			want: "parity: no parity case raises aql_containment_by_reference",
		},
		{
			// Arm (c): the non-vacuity guard. Every surviving row carries a
			// finding, so read and write could agree by both being non-empty but
			// never by both being empty — the false-positive direction goes
			// unwatched.
			name: "every parity row carries a finding",
			mutate: func(t *testing.T, c *aqlprobes.SemanticCorpus) {
				c.Parity = deleteRows(t, c.Parity, "clean parity row", func(p aqlprobes.ParityCase) bool {
					return strings.HasPrefix(p.Name, "near miss:")
				})
			},
			want: "parity: no parity case is clean",
		},
		{
			// Arm (c): fail closed on a caller-supplied nil Build (REQ-025) —
			// the assertion is as much "no panic" as it is the message.
			name: "a parity row has no Build",
			mutate: func(_ *testing.T, c *aqlprobes.SemanticCorpus) {
				c.Parity = append(c.Parity, aqlprobes.ParityCase{Name: "nil build"})
			},
			want: "parity/nil build: ParityCase.Build is nil",
		},
		{
			// Arm (c): the same, for a Build that returns a nil *aql.Builder —
			// aql.Builder.Build has no nil-receiver guard of its own.
			name: "a parity row builds a nil builder",
			mutate: func(_ *testing.T, c *aqlprobes.SemanticCorpus) {
				c.Parity = append(c.Parity, aqlprobes.ParityCase{
					Name:  "nil builder",
					Build: func() *aql.Builder { return nil },
				})
			},
			want: "parity/nil builder: Build() returned a nil *aql.Builder",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			corpus := probe097Corpus(t)
			tt.mutate(t, &corpus)
			r, err := aqlprobes.Probe097SemanticLint(corpus)
			if err != nil {
				t.Fatalf("Probe097: %v", err)
			}
			if r.Status != "fail" {
				t.Fatalf("expected fail, got status=%q detail=%q", r.Status, r.Detail)
			}
			if !strings.Contains(r.Detail, tt.want) {
				t.Fatalf("failure detail does not name the guard under control:\n  want substring: %s\n  got: %s",
					tt.want, r.Detail)
			}
		})
	}
}

// TestProbe097RequiresEveryCorpusArm is the able-to-fail control for the
// corpus-shape guard: an arm omitted entirely is a CALLER error — reported as
// an error with no Result status, never as a green run over the arms that
// happen to remain.
func TestProbe097RequiresEveryCorpusArm(t *testing.T) {
	arms := []struct {
		name string
		drop func(*aqlprobes.SemanticCorpus)
	}{
		{"Fire", func(c *aqlprobes.SemanticCorpus) { c.Fire = nil }},
		{"Silent", func(c *aqlprobes.SemanticCorpus) { c.Silent = nil }},
		{"Additivity", func(c *aqlprobes.SemanticCorpus) { c.Additivity = nil }},
		{"Parity", func(c *aqlprobes.SemanticCorpus) { c.Parity = nil }},
	}
	for _, arm := range arms {
		t.Run(arm.name, func(t *testing.T) {
			corpus := probe097Corpus(t)
			arm.drop(&corpus)
			r, err := aqlprobes.Probe097SemanticLint(corpus)
			if err == nil {
				t.Fatalf("err = nil, want the corpus-shape error (status=%q detail=%q)", r.Status, r.Detail)
			}
			if !strings.Contains(err.Error(), "all four corpus fields") {
				t.Fatalf("err = %v, want the corpus-shape error", err)
			}
			if r.Status != "" {
				t.Fatalf("status = %q, want %q — a corpus-shape error is not a probe verdict", r.Status, "")
			}
			if r.Probe != "PROBE-097" {
				t.Errorf("Probe id = %q, want PROBE-097", r.Probe)
			}
		})
	}
}
