package aqlprobes_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aql"
	"github.com/cadasto/openehr-sdk-go/openehr/aql/contain"
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
// their WantCodes baseline. Shared between TestProbe028AQLLint
// and PROBE-097 arm (b) so a deliberate PROBE-028 re-baseline (conformance.md
// § PROBE-097 arm (b)) is made ONCE: before this helper existed, arm (b)
// hand-copied this table, so a re-baseline of one would need to be applied
// twice, and a missed second edit would surface as a confusing
// "additivity/…" failure in PROBE-097 rather than where the change actually
// happened.
//
// The baseline was pre-REQ-161 and gained nothing from that requirement. It
// took its one re-baseline from REQ-164 § Additivity: `valid.aql` and
// `missing_archetype.aql` each project a column with no `AS` alias — a defect
// those queries genuinely carry — so each gained aql_select_no_alias and
// nothing else. Neither carries an unpredicated repeating segment (valid.aql
// predicates every one) or a row bound, and `bad_syntax.aql` gains nothing at
// all: it fails at Layer 1, so Layer 2 never runs.
func probe028Cases(t *testing.T) []aqlprobes.LintCase {
	t.Helper()
	opt := loadOPT(t, "vital_signs")
	return []aqlprobes.LintCase{
		{
			Name:      "valid",
			OPT:       opt,
			Query:     cassette(t, "valid.aql"),
			WantCodes: []string{"aql_select_no_alias"},
		},
		{
			Name:      "missing_archetype",
			OPT:       opt,
			Query:     cassette(t, "missing_archetype.aql"),
			WantCodes: []string{"aql_archetype_not_in_template", "aql_select_no_alias"},
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
// as "no re-baseline required". That claim still holds over the corpus as it
// stands: probe028Cases has since taken a REQ-164 re-baseline (an unaliased
// projection on two cassettes), and no REQ-161 code is among what it gained.
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
			// tokenSpan's boundaryOK call is ever removed — the exact case that
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
			// Arm (a), fire half: tokenSpan's own error path — an occurrence the
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

// --- PROBE-099 — REQ-164 path-shape lint corpus -----------------------------
//
// PROBE-097's structure minus its read/write-parity arm: every REQ-164 code is
// read-side only, so there is no builder analogue to compare against.
//
// Arm (a): each of the five REQ-164 codes fires on a corpus query built to carry
// exactly that defect, with Warning severity and a span on the offending
// construct — and stays silent on a near miss. The three firing rows the wire
// assertion names are claimed by name (the AQL-FIT-04 audit's two
// verified-silent queries, which MUST now warn, and the WHERE-only clause-scope
// witness), as are the fifteen negatives it names.
//
// Arm (b): the PROBE-028 corpus ([probe028Cases], the same three cassettes
// TestProbe028AQLLint wires) re-run under the completed REQ-164 linter. Two of
// the three gained aql_select_no_alias — a defect those queries genuinely carry
// — which is the deliberate, recorded re-baseline REQ-161 § Additivity defines,
// recorded in conformance.md's PROBE-099 entry.
//
// The queries below are the shapes openehr/aql/lint's own pathshape_*_test.go
// files pin at unit level. They are re-spelled here rather than imported: a
// probe that shared its corpus with the package under test would pass through a
// change that moved both together.

const (
	// The archetype predicates keep aql_from_archetype off these queries, so a
	// whole-multiset assertion stays about the codes under test.
	probe099ObsArch  = "openEHR-EHR-OBSERVATION.blood_pressure.v1"
	probe099CompArch = "openEHR-EHR-COMPOSITION.encounter.v1"

	codePathRepeatingUnpredicated = "aql_path_repeating_unpredicated"
	codePagingNoOrderBy           = "aql_paging_no_order_by"
	codeSelectNoAlias             = "aql_select_no_alias"
	codeFanoutPathGrain           = "aql_fanout_path_grain"
	codeContainsRedundantStep     = "aql_contains_redundant_step"
)

// probe099VersionTierRow is the one shipping fire row whose result is NOT
// REQ-164-only: a bare VERSION operand carries REQ-161's
// aql_version_no_predicate beside this group's finding. The guard control for
// the group-only accounting addresses it by name.
const probe099VersionTierRow = "redundant version-tier step, with the REQ-161 advisory riding along"

// probe099FireCases is PROBE-099 arm (a)'s firing table: at least one row per
// REQ-164 code, plus the three rows the wire assertion names explicitly.
// Severities are REQ-164 § Path-shape checks, verbatim: all five codes are
// Warnings, which is what keeps Result.OK() true on a group-only result.
func probe099FireCases() []aqlprobes.PathShapeFireCase {
	return []aqlprobes.PathShapeFireCase{
		{
			// The audit projection. ONE repeating-segment finding, on `events`:
			// OBSERVATION.data types to HISTORY (single-valued), HISTORY.events
			// to EVENT as a container, and EVENT.data is the BMM generic
			// parameter `T`, where the walk stops. The unaliased column is a
			// second, genuine REQ-164 defect of the same query.
			Name:      "the audit's unpredicated repeating-segment projection",
			Query:     "SELECT o/data/events/data/items/value/magnitude FROM EHR e CONTAINS OBSERVATION o[" + probe099ObsArch + "]",
			Code:      codePathRepeatingUnpredicated,
			Want:      []string{codePathRepeatingUnpredicated, codeSelectNoAlias},
			Severity:  lint.Warning,
			SpanText:  "events",
			SpanNth:   1,
			Mandatory: aqlprobes.FireAuditRepeatingSegment,
		},
		{
			Name:      "the audit's LIMIT 50 OFFSET 100 query with no ORDER BY",
			Query:     "SELECT o/name/value AS name FROM OBSERVATION o[" + probe099ObsArch + "] LIMIT 50 OFFSET 100",
			Code:      codePagingNoOrderBy,
			Want:      []string{codePagingNoOrderBy},
			Severity:  lint.Warning,
			SpanText:  "", // neither paging channel has a position in the query text
			Mandatory: aqlprobes.FireAuditRowBound,
		},
		{
			// The clause-scope witness: the offending path appears ONLY in
			// WHERE. An implementation narrowed to the projection fails here —
			// this query's SELECT path is entirely single-valued.
			Name: "a repeating segment reached only through WHERE",
			Query: "SELECT o/name/value AS n FROM OBSERVATION o[" + probe099ObsArch + "] " +
				"WHERE o/data/events/time/value > '2020-01-01'",
			Code:      codePathRepeatingUnpredicated,
			Want:      []string{codePathRepeatingUnpredicated},
			Severity:  lint.Warning,
			SpanText:  "events",
			SpanNth:   1,
			Mandatory: aqlprobes.FireClauseScopeWhereOnly,
		},
		{
			// The envelope arm: the AQL text carries no row bound at all, and
			// the bound arrives on Options.Query — the same channel the
			// parameter-binding checks read.
			Name:     "a row bound carried by the request envelope alone",
			Query:    "SELECT o/name/value AS name FROM OBSERVATION o[" + probe099ObsArch + "]",
			Fetch:    20,
			Code:     codePagingNoOrderBy,
			Want:     []string{codePagingNoOrderBy},
			Severity: lint.Warning,
			SpanText: "",
		},
		{
			Name:     "an unaliased projection item",
			Query:    "SELECT o/name/value FROM OBSERVATION o[" + probe099ObsArch + "]",
			Code:     codeSelectNoAlias,
			Want:     []string{codeSelectNoAlias},
			Severity: lint.Warning,
			SpanText: "o/name/value",
			SpanNth:  1,
		},
		{
			// REQ-164 § The conservative segment walk's named firing witness:
			// the two paths diverge AT the alias, with an unpredicated container
			// on each branch (HISTORY.events and LOCATABLE.links). The two
			// repeating-segment findings are the fan-out's PREMISE, not a
			// bystander — which is why Want names the whole multiset.
			Name: "two projected paths descending into different repeating scopes",
			Query: "SELECT o/data/events/time AS t, o/links/meaning/value AS m " +
				"FROM OBSERVATION o[" + probe099ObsArch + "]",
			Code: codeFanoutPathGrain,
			Want: []string{
				codePathRepeatingUnpredicated, codePathRepeatingUnpredicated, codeFanoutPathGrain,
			},
			Severity: lint.Warning,
			SpanText: "o/links/meaning/value", // the LATER path of the pair
			SpanNth:  1,
		},
		{
			// REQ-164's own redundant-step example: `c` is unreferenced and
			// predicate-less, and every EHR -> OBSERVATION containment route
			// passes a COMPOSITION.
			Name:     "an unavoidable unreferenced intermediate",
			Query:    "SELECT o/name/value AS n FROM EHR e CONTAINS COMPOSITION c CONTAINS OBSERVATION o",
			Code:     codeContainsRedundantStep,
			Want:     []string{codeContainsRedundantStep},
			Severity: lint.Warning,
			SpanText: "COMPOSITION",
			SpanNth:  1,
		},
		{
			// The version tier is the only route from a container to its
			// payload, so the unstated-tier step is inert. It records a
			// deliberate coexistence: a bare VERSION operand also carries
			// REQ-161's aql_version_no_predicate, which REQ-164 § No
			// double-reporting leaves standing — the two report DIFFERENT
			// defects. That bystander is what makes this the one shipping row
			// whose result is not REQ-164-only.
			Name: probe099VersionTierRow,
			Query: "SELECT c/name/value AS n FROM VERSIONED_COMPOSITION vo[uid/value='x'] " +
				"CONTAINS VERSION v CONTAINS COMPOSITION c",
			Code:     codeContainsRedundantStep,
			Want:     []string{codeContainsRedundantStep},
			Severity: lint.Warning,
			// Boundary-matched, so the `VERSION` inside VERSIONED_COMPOSITION
			// earlier in the query is not an occurrence.
			SpanText: "VERSION",
			SpanNth:  1,
		},
	}
}

// probe099SilentCases is PROBE-099 arm (a)'s near-miss table — at least one row
// per REQ-164 code, and the fifteen negatives the wire assertion names.
//
// ForCode / Negative are not decoration: they are what the probe's completeness
// guards count, so the near-miss half of arm (a) cannot shrink below "one
// silence row per REQ-164 code, and all fifteen named negatives" while still
// reporting pass. Keeps is what makes a YIELDING near miss non-vacuous — three
// of these are not "nothing fires" but "another code owns this shape".
func probe099SilentCases() []aqlprobes.PathShapeSilentCase {
	return []aqlprobes.PathShapeSilentCase{
		{
			// Presence suffices, content is never judged: whether at0006 is the
			// RIGHT node id is Layer 3's question.
			Name:     "near miss: a predicated repeating segment",
			Query:    "SELECT o/data/events[at0006]/time AS t FROM OBSERVATION o[" + probe099ObsArch + "]",
			ForCode:  codePathRepeatingUnpredicated,
			Negative: aqlprobes.NegPredicatedSegment,
		},
		{
			// The total order is the remedy, so its presence silences the
			// finding however many channels bounded the rows — here both.
			Name: "near miss: ORDER BY beside a bound on both channels",
			Query: "SELECT o/name/value AS name FROM OBSERVATION o[" + probe099ObsArch + "] " +
				"ORDER BY o/name/value LIMIT 50 OFFSET 100",
			Fetch:    20,
			ForCode:  codePagingNoOrderBy,
			Negative: aqlprobes.NegOrderByPresent,
		},
		{
			// With no envelope supplied the envelope arm cannot fire, exactly as
			// it leaves the parameter-binding checks unable to. The positive
			// control is the fire row above spelling the SAME query with a
			// bound.
			Name:     "near miss: no request envelope, and no in-text bound",
			Query:    "SELECT o/name/value AS name FROM OBSERVATION o[" + probe099ObsArch + "]",
			ForCode:  codePagingNoOrderBy,
			Negative: aqlprobes.NegNilEnvelope,
		},
		{
			// One defect gets one finding: aql_deprecated_top already carries
			// the ORDER BY remedy for that clause.
			Name:     "near miss: a TOP-only row bound",
			Query:    "SELECT TOP 5 o/name/value AS name FROM OBSERVATION o[" + probe099ObsArch + "]",
			Keeps:    []string{"aql_deprecated_top"},
			ForCode:  codePagingNoOrderBy,
			Negative: aqlprobes.NegTopOnlyBound,
		},
		{
			Name: "near miss: every projection item aliased",
			Query: "SELECT o/name/value AS name, o/uid/value AS uid " +
				"FROM OBSERVATION o[" + probe099ObsArch + "]",
			ForCode:  codeSelectNoAlias,
			Negative: aqlprobes.NegAliasedProjection,
		},
		{
			// The star has nothing to alias; REQ-164 § No double-reporting gives
			// that shape to REQ-109's aql_select_star. The MIXED spelling is
			// used so the star is a projection item the check actually sees and
			// skips, rather than one a bare SELECT * never records.
			Name:     "near miss: a `*` item beside an aliased column",
			Query:    "SELECT *, o/name/value AS name FROM OBSERVATION o[" + probe099ObsArch + "]",
			Keeps:    []string{"aql_select_star"},
			ForCode:  codeSelectNoAlias,
			Negative: aqlprobes.NegStarItem,
		},
		{
			// One repeating scope, no product: the two paths' only container
			// sits in their common prefix. Each path still carries its own
			// repeating-segment finding, which is why Want is not nil — the
			// silence being asserted is the fan-out advisory's.
			Name: "near miss: repeating segments all in the common prefix",
			Query: "SELECT o/data/events/time AS a, o/data/events/name/value AS b " +
				"FROM OBSERVATION o[" + probe099ObsArch + "]",
			Want: []string{
				codePathRepeatingUnpredicated, codePathRepeatingUnpredicated,
			},
			ForCode:  codeFanoutPathGrain,
			Negative: aqlprobes.NegSharedRepeatingPrefix,
		},
		{
			// Two aliases never pair: that is the junction question,
			// aql_fanout_row_grain (REQ-161). Each alias's path carries its own
			// repeating-segment findings — `c/content` and its inherited
			// `links`, and `o`'s `events`.
			Name: "near miss: projected paths rooted on different aliases",
			Query: "SELECT c/content/links/meaning/value AS a, o/data/events/time AS b " +
				"FROM EHR e CONTAINS COMPOSITION c[" + probe099CompArch + "] " +
				"CONTAINS OBSERVATION o[" + probe099ObsArch + "]",
			Want: []string{
				codePathRepeatingUnpredicated, codePathRepeatingUnpredicated,
				codePathRepeatingUnpredicated,
			},
			ForCode:  codeFanoutPathGrain,
			Negative: aqlprobes.NegDifferentAliases,
		},
		{
			// The substance of the whole redundant-step rule: dropping SECTION
			// admits observations that sit directly in a composition's content,
			// so the step narrows the result and is not redundant. A check
			// written to the guidance sentence as stated would flag this.
			Name:     "near miss: an avoidable unreferenced intermediate",
			Query:    "SELECT o/name/value AS n FROM EHR e CONTAINS SECTION s CONTAINS OBSERVATION o",
			ForCode:  codeContainsRedundantStep,
			Negative: aqlprobes.NegAvoidableIntermediate,
		},
		{
			Name:     "near miss: an unreferenced leaf",
			Query:    "SELECT e/ehr_id/value AS id FROM EHR e CONTAINS COMPOSITION c",
			ForCode:  codeContainsRedundantStep,
			Negative: aqlprobes.NegUnreferencedLeaf,
		},
		{
			// REQ-164 § No double-reporting: the general case yields to the
			// specific. The overlay is what makes the skip observable — on the
			// default relation no VERSIONED_* class is ever unavoidable, so a
			// row without it would pass with the guard deleted.
			Name: "near miss: a VERSIONED_OBJECT-conforming operand keeps REQ-161's code",
			Query: "SELECT c/name/value AS n FROM SITE s CONTAINS VERSIONED_COMPOSITION vo " +
				"CONTAINS COMPOSITION c[" + probe099CompArch + "]",
			Relation: contain.Default().WithOverlay(contain.Edge{
				From: "SITE", To: "VERSIONED_COMPOSITION",
			}),
			Keeps:    []string{"aql_versioned_object_unreferenced"},
			ForCode:  codeContainsRedundantStep,
			Negative: aqlprobes.NegVersionedObjectOperand,
		},
		{
			// The walk cannot start, so the path goes unjudged rather than
			// judged against a guess. The class has its own code, which this
			// group adds nothing to.
			Name:     "near miss: the walk stops on a class the pin does not know",
			Query:    "SELECT x/data/events/time AS t FROM NOT_AN_RM_CLASS x",
			Keeps:    []string{"aql_unknown_rm_class"},
			ForCode:  codePathRepeatingUnpredicated,
			Negative: aqlprobes.NegWalkStopUnknownClass,
		},
		{
			// `items` is a container on SECTION and on ITEM_TREE; on OBSERVATION
			// it is not an attribute at all, so the walk stops at the first
			// segment.
			Name:     "near miss: the walk stops on an attribute the pin does not declare",
			Query:    "SELECT o/items/value AS v FROM OBSERVATION o[" + probe099ObsArch + "]",
			ForCode:  codePathRepeatingUnpredicated,
			Negative: aqlprobes.NegWalkStopUndeclaredAttribute,
		},
		{
			// EVENT.data is literally typed `T` on the pinned tables, and
			// REQ-048 leaves generic-parameter resolution out of scope. The
			// `items` below that stop IS a container on every class that
			// declares it, so a walk that guessed what `T` stands for would
			// report it — with `events` predicated here, the stop is the only
			// thing keeping this query silent.
			Name: "near miss: the walk stops at the generic parameter above a container",
			Query: "SELECT o/data/events[at0006]/data/items/value/magnitude AS m " +
				"FROM OBSERVATION o[" + probe099ObsArch + "]",
			ForCode:  codePathRepeatingUnpredicated,
			Negative: aqlprobes.NegWalkStopGenericParameter,
		},
		{
			// A `$param` archetype scope, whose extent the CDR resolves at
			// execution — the skip Layer 3 and the REQ-161 checks already apply
			// for the same reason. The same path under a LITERAL archetype is
			// the fire row above.
			Name:     "near miss: a $param archetype scope",
			Query:    "SELECT o/data/events/time AS t FROM OBSERVATION o[$arch]",
			ForCode:  codePathRepeatingUnpredicated,
			Negative: aqlprobes.NegWalkStopParamArchetype,
		},
	}
}

// probe099AdditivityCases is PROBE-099 arm (b): PROBE-028's own corpus
// ([probe028Cases], shared with TestProbe028AQLLint as noted there), re-run
// under the completed REQ-164 linter.
//
// Unlike PROBE-097 arm (b), this one DID re-baseline: valid.aql and
// missing_archetype.aql each project a column with no AS alias — a defect those
// queries genuinely carry — so each gained aql_select_no_alias and nothing else,
// while bad_syntax.aql gained nothing at all (it fails at Layer 1, so Layer 2
// never runs). That re-baseline is recorded in conformance.md's PROBE-099 entry,
// as REQ-164 § Additivity requires; probe028Cases carries it in the table
// itself.
func probe099AdditivityCases(t *testing.T) []aqlprobes.LintCase {
	t.Helper()
	return probe028Cases(t)
}

// probe099Corpus assembles the full PROBE-099 corpus. Shared between
// TestProbe099PathShapeLint and the guard controls below, which each mutate one
// arm of it — a control built from a hand-written stand-in would prove the guard
// fires on a toy corpus, not that it fires on the corpus that ships.
func probe099Corpus(t *testing.T) aqlprobes.PathShapeCorpus {
	t.Helper()
	return aqlprobes.PathShapeCorpus{
		Fire:       probe099FireCases(),
		Silent:     probe099SilentCases(),
		Additivity: probe099AdditivityCases(t),
	}
}

// TestProbe099PathShapeLint runs PROBE-099's full corpus — both wire-assertion
// arms — and asserts a clean pass.
func TestProbe099PathShapeLint(t *testing.T) {
	r, err := aqlprobes.Probe099PathShapeLint(probe099Corpus(t))
	if err != nil {
		t.Fatalf("Probe099: %v", err)
	}
	if r.Status != "pass" {
		t.Fatalf("Probe099 status=%q detail=%q", r.Status, r.Detail)
	}
	if r.Probe != "PROBE-099" {
		t.Errorf("Probe id = %q, want PROBE-099", r.Probe)
	}
}

// pathShapeFireRow addresses the named fire row so a control can mutate it in
// place, failing the test rather than no-opping when the row is gone.
func pathShapeFireRow(t *testing.T, rows []aqlprobes.PathShapeFireCase, name string) *aqlprobes.PathShapeFireCase {
	t.Helper()
	i := slices.IndexFunc(rows, func(c aqlprobes.PathShapeFireCase) bool { return c.Name == name })
	if i < 0 {
		t.Fatalf("no fire row named %q; this control has rotted", name)
	}
	return &rows[i]
}

const probe099AuditFireRow = "the audit's unpredicated repeating-segment projection"

// TestProbe099GuardsCanFail is the able-to-fail control suite. Each case mutates
// the SHIPPING corpus in exactly the way one guard exists to catch — a toy
// stand-in would only prove the guard fires on a toy corpus — and asserts that
// guard's OWN message, since a control asserting nothing but Status == "fail"
// passes on any failure, including one it caused for an unrelated reason.
func TestProbe099GuardsCanFail(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, *aqlprobes.PathShapeCorpus)
		want   string // substring of Detail the guard under control must produce
	}{
		{
			// Arm (a), fire half: the per-code coverage guard.
			name: "a fire row is deleted",
			mutate: func(t *testing.T, c *aqlprobes.PathShapeCorpus) {
				c.Fire = deleteRows(t, c.Fire, "fan-out fire row", func(f aqlprobes.PathShapeFireCase) bool {
					return f.Code == codeFanoutPathGrain
				})
			},
			want: "fire: no fire case raises aql_fanout_path_grain",
		},
		{
			// Arm (a), fire half: the named-row guard, which no per-code count
			// would notice the loss of (the paging code keeps its envelope row).
			name: "a fire row the wire assertion names is deleted",
			mutate: func(t *testing.T, c *aqlprobes.PathShapeCorpus) {
				c.Fire = deleteRows(t, c.Fire, "audit row-bound fire row", func(f aqlprobes.PathShapeFireCase) bool {
					return f.Mandatory == aqlprobes.FireAuditRowBound
				})
			},
			want: `fire: no fire case claims the "audit: the LIMIT-without-ORDER BY query" row`,
		},
		{
			// Arm (a), fire half: the group-only accounting behind REQ-164
			// § Acceptance's OK() claim. Keeping only the version-tier row —
			// whose result carries REQ-161's advisory too — leaves no row able
			// to make that claim.
			name: "no fire row yields a group-only result",
			mutate: func(t *testing.T, c *aqlprobes.PathShapeCorpus) {
				c.Fire = deleteRows(t, c.Fire, "group-only fire rows", func(f aqlprobes.PathShapeFireCase) bool {
					return f.Name != probe099VersionTierRow
				})
			},
			want: "fire: no fire case yields a group-only result",
		},
		{
			// Arm (a), fire half: the corpus-row check, which no linter
			// behaviour can trip — a row whose Want omits its own Code asserts
			// nothing about the code it exists for while still satisfying the
			// coverage guard.
			name: "a fire row's Want omits its own Code",
			mutate: func(t *testing.T, c *aqlprobes.PathShapeCorpus) {
				pathShapeFireRow(t, c.Fire, probe099AuditFireRow).Want = []string{codeSelectNoAlias}
			},
			want: "corpus row error: Want",
		},
		{
			// Arm (a), fire half: the severity comparison.
			name: "a fire row carries the wrong severity",
			mutate: func(t *testing.T, c *aqlprobes.PathShapeCorpus) {
				pathShapeFireRow(t, c.Fire, probe099AuditFireRow).Severity = lint.Error
			},
			want: "aql_path_repeating_unpredicated severity =",
		},
		{
			// Arm (a), fire half: the span comparison, on a code whose span
			// covers a path SEGMENT rather than a class token.
			name: "a fire row names an occurrence the query does not have",
			mutate: func(t *testing.T, c *aqlprobes.PathShapeCorpus) {
				pathShapeFireRow(t, c.Fire, probe099AuditFireRow).SpanNth = 2
			},
			want: `fewer than 2 boundary-matched occurrences of "events"`,
		},
		{
			// Arm (a), fire half: the zero-Span assertion is POSITIVE — a
			// paging row that named a span would have to find one.
			name: "a paging fire row expects a span",
			mutate: func(t *testing.T, c *aqlprobes.PathShapeCorpus) {
				row := pathShapeFireRow(t, c.Fire, "the audit's LIMIT 50 OFFSET 100 query with no ORDER BY")
				row.SpanText, row.SpanNth = "LIMIT", 1
			},
			want: "aql_paging_no_order_by span =",
		},
		{
			// …and its converse: a row that expected NO span on a code that
			// carries one.
			name: "a spanned fire row expects the zero Span",
			mutate: func(t *testing.T, c *aqlprobes.PathShapeCorpus) {
				pathShapeFireRow(t, c.Fire, probe099AuditFireRow).SpanText = ""
			},
			want: "want the zero Span",
		},
		{
			// Arm (a), silence half: the per-code coverage guard's mirror.
			name: "the silence row for a code is deleted",
			mutate: func(t *testing.T, c *aqlprobes.PathShapeCorpus) {
				c.Silent = deleteRows(t, c.Silent, "fan-out silence rows", func(s aqlprobes.PathShapeSilentCase) bool {
					return s.ForCode == codeFanoutPathGrain
				})
			},
			want: "silent: no silence case guards aql_fanout_path_grain",
		},
		{
			// Arm (a), silence half: the named-negative guard. The
			// repeating-segment code keeps four other silence rows, so no
			// per-code count would notice this one going.
			name: "a named negative is deleted",
			mutate: func(t *testing.T, c *aqlprobes.PathShapeCorpus) {
				c.Silent = deleteRows(t, c.Silent, "generic-parameter stop row", func(s aqlprobes.PathShapeSilentCase) bool {
					return s.Negative == aqlprobes.NegWalkStopGenericParameter
				})
			},
			want: `silent: no silence case pins the "walk stop: generic-parameter type" negative`,
		},
		{
			// Arm (a), silence half: the vacuous-silence hole — a row whose
			// query fails Layer 1 never reaches a REQ-164 check, so its
			// nil-vs-nil comparison would assert nothing.
			name: "a silence row asserts silence on a query that never parsed",
			mutate: func(_ *testing.T, c *aqlprobes.PathShapeCorpus) {
				c.Silent = append(c.Silent, aqlprobes.PathShapeSilentCase{
					Name:  "never parsed",
					Query: "SELECT o/name/value AS n FROM OBSERVATION o CONTAINS", // dangling CONTAINS
				})
			},
			want: "silent/never parsed: query never reached the REQ-164 checks (aql_syntax)",
		},
		{
			// Arm (a), silence half: the comparison itself.
			name: "a silence row wants a code its query does not raise",
			mutate: func(_ *testing.T, c *aqlprobes.PathShapeCorpus) {
				c.Silent = append(c.Silent, aqlprobes.PathShapeSilentCase{
					Name:  "wrong want",
					Query: "SELECT o/name/value AS n FROM OBSERVATION o[" + probe099ObsArch + "]",
					Want:  []string{codeSelectNoAlias},
				})
			},
			want: "silent/wrong want: path-shape codes = []",
		},
		{
			// Arm (a), silence half: the Keeps assertion, which is what stops a
			// YIELDING near miss passing on a query that had simply stopped
			// being linted.
			name: "a yielding silence row loses the code it yields to",
			mutate: func(_ *testing.T, c *aqlprobes.PathShapeCorpus) {
				c.Silent = append(c.Silent, aqlprobes.PathShapeSilentCase{
					Name:  "keeps nothing",
					Query: "SELECT o/name/value AS n FROM OBSERVATION o[" + probe099ObsArch + "]",
					Keeps: []string{"aql_deprecated_top"},
				})
			},
			want: "silent/keeps nothing: the shape yields to aql_deprecated_top",
		},
		{
			// Arm (b): the additivity baseline. A cassette re-baselined by
			// accident rather than by decision fails here.
			name: "an additivity baseline drifts",
			mutate: func(t *testing.T, c *aqlprobes.PathShapeCorpus) {
				i := slices.IndexFunc(c.Additivity, func(lc aqlprobes.LintCase) bool { return lc.Name == "valid" })
				if i < 0 {
					t.Fatal("no additivity row named \"valid\"; this control has rotted")
				}
				c.Additivity[i].WantCodes = nil
			},
			want: "additivity/valid: codes mismatch",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			corpus := probe099Corpus(t)
			tt.mutate(t, &corpus)
			r, err := aqlprobes.Probe099PathShapeLint(corpus)
			if err != nil {
				t.Fatalf("Probe099: %v", err)
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

// TestProbe099RequiresEveryCorpusArm is the able-to-fail control for the
// corpus-shape guard: an arm omitted entirely is a CALLER error — reported as an
// error with no Result status, never as a green run over the arms that remain.
func TestProbe099RequiresEveryCorpusArm(t *testing.T) {
	arms := []struct {
		name string
		drop func(*aqlprobes.PathShapeCorpus)
	}{
		{"Fire", func(c *aqlprobes.PathShapeCorpus) { c.Fire = nil }},
		{"Silent", func(c *aqlprobes.PathShapeCorpus) { c.Silent = nil }},
		{"Additivity", func(c *aqlprobes.PathShapeCorpus) { c.Additivity = nil }},
	}
	for _, arm := range arms {
		t.Run(arm.name, func(t *testing.T) {
			corpus := probe099Corpus(t)
			arm.drop(&corpus)
			r, err := aqlprobes.Probe099PathShapeLint(corpus)
			if err == nil {
				t.Fatalf("err = nil, want the corpus-shape error (status=%q detail=%q)", r.Status, r.Detail)
			}
			if !strings.Contains(err.Error(), "all three corpus fields") {
				t.Fatalf("err = %v, want the corpus-shape error", err)
			}
			if r.Status != "" {
				t.Fatalf("status = %q, want %q — a corpus-shape error is not a probe verdict", r.Status, "")
			}
			if r.Probe != "PROBE-099" {
				t.Errorf("Probe id = %q, want PROBE-099", r.Probe)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// PROBE-100 — REQ-160 upstream admissibility corpus ratchet.
// ---------------------------------------------------------------------------

// probe100Corpus reads the shipping corpus — the vendored CSVs under
// testkit/cassettes/aql/conformance/, reconstructed into queries by the
// reconstruction table in probe_100_conformance_corpus.go. The root comes from
// the fixtures package so the path is resolved from the cassettes root rather
// than from the working directory.
func probe100Corpus(t *testing.T) aqlprobes.ConformanceCorpus {
	t.Helper()
	c, err := aqlprobes.ReadConformanceCorpus(fixtures.AQLConformanceRoot())
	if err != nil {
		t.Fatalf("ReadConformanceCorpus: %v", err)
	}
	return c
}

// TestProbe100ConformanceCorpus runs the ratchet over the whole vendored
// corpus and logs the per-family tally, so a CI run records what the ratchet
// actually covered rather than only that it passed.
func TestProbe100ConformanceCorpus(t *testing.T) {
	corpus := probe100Corpus(t)
	for _, f := range corpus.FamilyCounts() {
		t.Logf("family %s: %d asserted, %d excluded", f.Family, f.Asserted, f.Excluded)
	}
	t.Logf("corpus total: %d asserted, %d excluded", len(corpus.Rows), len(corpus.Excluded))
	for _, e := range corpus.Excluded {
		t.Logf("excluded %s: %s — %s", e.Where(), e.Reason, e.Why)
	}

	// The shipping tally is pinned, not only logged. The probe's per-family
	// tripwire fires only at zero, so an intra-family shrink — 29 rows down
	// to 1 in USABLE_RM_TYPES_A_D, say — would stay green while the snapshot
	// PROBE-100's Status records silently drifted. These are that snapshot's
	// numbers; a corpus refresh re-baselines the pin and the Status prose
	// together, deliberately (the ratchet's refresh arm).
	if len(corpus.Rows) != 67 {
		t.Errorf("corpus reconstructs %d asserted rows, want the 67 the PROBE-100 pin records", len(corpus.Rows))
	}
	if len(corpus.Excluded) != 0 {
		t.Errorf("corpus excludes %d row(s), want 0 at this pin — the exclusion log above names each", len(corpus.Excluded))
	}

	r, err := aqlprobes.Probe100ConformanceCorpus(corpus)
	if err != nil {
		t.Fatalf("Probe100: %v", err)
	}
	if r.Status != "pass" {
		t.Fatalf("Probe100 status=%q detail=%q", r.Status, r.Detail)
	}
	if r.Probe != "PROBE-100" {
		t.Errorf("Probe id = %q, want PROBE-100", r.Probe)
	}
}

// TestProbe100CoversThePinnedSuites pins by name the CSVs whose loss the
// family tripwire would not notice. The probe carries the same guards; this
// asserts them at the corpus the probe is actually given, so a suite that
// stopped contributing fails here with the file named rather than as one line
// inside an aggregated probe Detail. The chaining suite is the one REQ-160's
// compatibility guard was hand-picked from; the two EHR_STATUS suites are the
// only templates carrying the ${ehr_id} substitution, whose regression sends
// their rows to the exclusion list — which fails no family, since
// EHR_STATUS/contains.csv still contributes.
func TestProbe100CoversThePinnedSuites(t *testing.T) {
	cases := []struct {
		family, file, why string
	}{
		{
			family: "CONTAINS_A_D", file: "from_contains_plus_contain_chaining.csv",
			why: "it carries the containment chains REQ-160's compatibility guard was drawn from",
		},
		{
			family: "EHR_STATUS", file: "from_single_ehr.csv",
			why: "its template depends on the ${ehr_id} substitution; a regression excludes rows, failing no family",
		},
		{
			family: "EHR_STATUS", file: "via_part.csv",
			why: "its template depends on the ${ehr_id} substitution; a regression excludes rows, failing no family",
		},
	}
	counts := probe100Corpus(t).FileCounts()
	for _, tc := range cases {
		t.Run(tc.family+"/"+tc.file, func(t *testing.T) {
			for _, f := range counts {
				if f.Family != tc.family || f.File != tc.file {
					continue
				}
				if f.Asserted == 0 {
					t.Fatalf("%s/%s: asserted = 0, want > 0 — %s (%d row(s) excluded)",
						tc.family, tc.file, tc.why, f.Excluded)
				}
				return
			}
			t.Fatalf("%s/%s is not in the corpus tally at all", tc.family, tc.file)
		})
	}
}

// TestProbe100DetectsInadmissibleRows is the probe's able-to-fail control for
// both halves of the wire assertion: a row that does not parse, and a row the
// REQ-161 containment checks refuse at Error severity. Each failure must name
// the row's corpus coordinate, since that is all a CI log carries.
func TestProbe100DetectsInadmissibleRows(t *testing.T) {
	cases := []struct {
		name string
		row  aqlprobes.ConformanceRow
	}{
		{
			name: "unparseable",
			row: aqlprobes.ConformanceRow{
				Family: "AND_OR", File: "from_simple_and_or.csv", Line: 2,
				Suite: "AQL_TESTS/FROM/AND_OR/simple_and_or.robot",
				Query: "SELECT o FROM",
			},
		},
		{
			// An OBSERVATION cannot contain a COMPOSITION under the REQ-160
			// relation, so this is the shape a corpus row would have to take
			// for the ratchet to bite.
			name: "impossible_containment",
			row: aqlprobes.ConformanceRow{
				Family: "CONTAINS_A_D", File: "from_contains_plus_contain_chaining.csv", Line: 3,
				Suite: "AQL_TESTS/FROM/CONTAINS_A_D/contains_plus_contain_chaining.robot",
				Query: "SELECT o FROM OBSERVATION o CONTAINS COMPOSITION c",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			corpus := probe100Corpus(t)
			corpus.Rows = append(corpus.Rows, tc.row)
			r, err := aqlprobes.Probe100ConformanceCorpus(corpus)
			if err != nil {
				t.Fatalf("Probe100: %v", err)
			}
			if r.Status != "fail" {
				t.Fatalf("status = %q, want fail — the ratchet did not bite", r.Status)
			}
			if !strings.Contains(r.Detail, tc.row.Where()) {
				t.Fatalf("detail does not name the row coordinate %s: %s", tc.row.Where(), r.Detail)
			}
		})
	}
}

// probe100ContainmentCodes is the REQ-162 § Contract five-code containment
// scope PROBE-100's wire assertion is written against, spelled out here because
// the probe's own containmentCodes() is unexported and this file is the
// external test package. It is the set the premise guard below sweeps for an
// Error: a code outside it — a REQ-161 portability advisory, a REQ-164
// path-shape finding — is not what PROBE-100 asserts on either way.
var probe100ContainmentCodes = []string{
	"aql_impossible_containment",
	"aql_contains_not_containable",
	"aql_archetype_class_mismatch",
	"aql_unknown_rm_class",
	"aql_containment_by_reference",
}

// TestProbe100PermitsWarningSeverityContainmentCodes is the mirror image of
// TestProbe100DetectsInadmissibleRows, and the control for the arm of the
// PROBE-100 wire assertion that says a Warning is NOT a refusal: "Warnings are
// permitted: aql_unknown_rm_class, aql_containment_by_reference and the
// portability advisories are observations about a pair, never admissibility
// refusals."
//
// Nothing else holds that arm. Whether any row of the shipping corpus warns at
// all is a property of the vendored data, not of this repository, so a
// Probe100ConformanceCorpus that regressed to failing on ANY containment code
// regardless of severity could leave the whole corpus run green and no named
// test would fail. Each case here appends one crafted row that provably draws
// its named Warning and no Error-severity containment code, so the severity
// gate has something it MUST let through.
//
// REQ-160 · REQ-161
func TestProbe100PermitsWarningSeverityContainmentCodes(t *testing.T) {
	cases := []struct {
		name string
		// code is the Warning the crafted row must raise — one of the two the
		// wire assertion names by code.
		code string
		row  aqlprobes.ConformanceRow
	}{
		{
			// A CONTAINS naming a class the pinned RM does not know. The
			// coordinate is the line a refresh of this CSV would add the row on:
			// its template is `SELECT t FROM COMPOSITION CONTAINS ${type} t`, so
			// a `${type}` cell naming an unmodelled class reconstructs exactly
			// this query. The engine answering such a row is precisely the case
			// REQ-160 must not verdict Never on.
			name: "unknown_rm_class",
			code: "aql_unknown_rm_class",
			row: aqlprobes.ConformanceRow{
				Family: "USABLE_RM_TYPES_A_D", File: "from_item_structure_and_element_in_composition.csv", Line: 6,
				Suite: "AQL_TESTS/FROM/USABLE_RM_TYPES_A_D/from_item_structure_and_element_in_composition.robot",
				Query: "SELECT t FROM COMPOSITION CONTAINS FOO_BAR t",
			},
		},
		{
			// FOLDER->COMPOSITION is containment by reference, not by value: a
			// real containment relation an engine answers, which REQ-161 grades
			// Warning because the pair costs a dereference rather than because
			// it is inadmissible. The chaining CSV's template is
			// `SELECT o FROM ${from}`, so a `${from}` cell reading
			// "FOLDER CONTAINS COMPOSITION o" reconstructs this query.
			name: "containment_by_reference",
			code: "aql_containment_by_reference",
			row: aqlprobes.ConformanceRow{
				Family: "CONTAINS_A_D", File: "from_contains_plus_contain_chaining.csv", Line: 12,
				Suite: "AQL_TESTS/FROM/CONTAINS_A_D/contains_plus_contain_chaining.robot",
				Query: "SELECT o FROM FOLDER CONTAINS COMPOSITION o",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// PREMISE, asserted on the very query the probe is handed below and
			// through the same lint call runConformanceRow makes
			// (lint.LintString with nil options). Without it this control is
			// vacuous: a crafted query that quietly stopped raising anything
			// would sail through a probe that had regressed to failing on any
			// containment code, and the test would pass for the wrong reason.
			// Same register as TestLintTopWithUnrepresentableCountAndEnvelopeFetch
			// in openehr/aql/lint/lint_test.go.
			res := lint.LintString(tc.row.Query, nil)
			raised := make([]string, 0, len(res.Issues))
			for _, iss := range res.Issues {
				raised = append(raised, fmt.Sprintf("%s/%v", iss.Code, iss.Severity))
			}
			// The named Warning is present AT Warning severity. Its presence
			// also proves the query parsed: lint.LintString short-circuits a
			// parse failure to aql_syntax and runs no REQ-161 check at all.
			found := false
			for _, iss := range res.Issues {
				if iss.Code != tc.code {
					continue
				}
				found = true
				if iss.Severity != lint.Warning {
					t.Fatalf("%s severity = %v, want %v — this control needs a row the wire assertion permits, not one it refuses",
						tc.code, iss.Severity, lint.Warning)
				}
			}
			if !found {
				t.Fatalf("lint raises %v on %q, want it to include %s at Warning severity — the premise of this control",
					raised, tc.row.Query, tc.code)
			}
			// And no Error-severity containment code, which is the half of the
			// wire assertion the probe actually refuses on.
			for _, iss := range res.Issues {
				if iss.Severity == lint.Error && slices.Contains(probe100ContainmentCodes, iss.Code) {
					t.Fatalf("lint raises Error-severity containment code %s on %q (all issues: %v); "+
						"this control needs a row the ratchet MUST admit, not one it must refuse",
						iss.Code, tc.row.Query, raised)
				}
			}

			corpus := probe100Corpus(t)
			corpus.Rows = append(corpus.Rows, tc.row)
			r, err := aqlprobes.Probe100ConformanceCorpus(corpus)
			if err != nil {
				t.Fatalf("Probe100: %v", err)
			}
			if r.Status != "pass" {
				t.Fatalf("status = %q (detail %q), want pass — %s is a Warning, and the wire assertion permits it: "+
					"an observation about a pair, never an admissibility refusal",
					r.Status, r.Detail, tc.code)
			}
		})
	}
}

// TestProbe100DetectsCorpusShrinkage is the able-to-fail control for the two
// coverage guards — the per-family empty tripwire and the named chaining suite.
// Both go dark quietly: every surviving row still passes on its own, so only a
// guard that counts what is missing can tell.
func TestProbe100DetectsCorpusShrinkage(t *testing.T) {
	cases := []struct {
		name string
		// drop reports whether a row is removed from the shipping corpus.
		drop func(aqlprobes.ConformanceRow) bool
		want string
	}{
		{
			name: "family_emptied",
			drop: func(r aqlprobes.ConformanceRow) bool { return r.Family == "EHR_STATUS" },
			want: "family EHR_STATUS: no asserted row",
		},
		{
			name: "chaining_suite_lost",
			drop: func(r aqlprobes.ConformanceRow) bool {
				return r.File == "from_contains_plus_contain_chaining.csv"
			},
			want: "CONTAINS_A_D/from_contains_plus_contain_chaining.csv contributes no asserted row",
		},
		// The two ${ehr_id} suites: dropping either leaves the EHR_STATUS
		// family alive through contains.csv, so only the named pin bites.
		{
			name: "ehr_id_suite_from_single_ehr_lost",
			drop: func(r aqlprobes.ConformanceRow) bool {
				return r.Family == "EHR_STATUS" && r.File == "from_single_ehr.csv"
			},
			want: "EHR_STATUS/from_single_ehr.csv contributes no asserted row",
		},
		{
			name: "ehr_id_suite_via_part_lost",
			drop: func(r aqlprobes.ConformanceRow) bool {
				return r.Family == "EHR_STATUS" && r.File == "via_part.csv"
			},
			want: "EHR_STATUS/via_part.csv contributes no asserted row",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			corpus := probe100Corpus(t)
			corpus.Rows = slices.DeleteFunc(corpus.Rows, tc.drop)
			r, err := aqlprobes.Probe100ConformanceCorpus(corpus)
			if err != nil {
				t.Fatalf("Probe100: %v", err)
			}
			if r.Status != "fail" {
				t.Fatalf("status = %q, want fail — the shrunken corpus still reported pass", r.Status)
			}
			if !strings.Contains(r.Detail, tc.want) {
				t.Fatalf("detail = %q, want it to contain %q", r.Detail, tc.want)
			}
		})
	}
}

// TestProbe100RequiresANonEmptyCorpus is the able-to-fail control for the
// whole-corpus tripwire: an empty corpus is a CALLER error — reported as an
// error with no Result status, never as a vacuous pass.
func TestProbe100RequiresANonEmptyCorpus(t *testing.T) {
	r, err := aqlprobes.Probe100ConformanceCorpus(aqlprobes.ConformanceCorpus{})
	if err == nil {
		t.Fatalf("err = nil, want the empty-corpus error (status=%q detail=%q)", r.Status, r.Detail)
	}
	if !strings.Contains(err.Error(), "reconstructed no rows") {
		t.Fatalf("err = %v, want the empty-corpus error", err)
	}
	if r.Status != "" {
		t.Fatalf("status = %q, want %q — an empty corpus is not a probe verdict", r.Status, "")
	}
	if r.Probe != "PROBE-100" {
		t.Errorf("Probe id = %q, want PROBE-100", r.Probe)
	}
}

// TestProbe100ReaderRefusesAnUnlearnedCorpus is the refresh arm of the ratchet:
// a corpus the reader has not learned MUST fail, not be partially read. Each
// case writes a throwaway corpus root holding exactly the named files.
func TestProbe100ReaderRefusesAnUnlearnedCorpus(t *testing.T) {
	const goodHeader = "${statement},${expected_file},${nr_of_results}\n"
	const goodRow = "\"SELECT o FROM EHR CONTAINS OBSERVATION o\",expected.json,1\n"
	cases := []struct {
		name  string
		files map[string]string
		want  string
	}{
		{
			name:  "unlearned_family",
			files: map[string]string{"NEW_FAMILY/from_something.csv": goodHeader + goodRow},
			want:  "not in the reconstruction table",
		},
		{
			name:  "unlearned_file",
			files: map[string]string{"AND_OR/from_something_new.csv": goodHeader + goodRow},
			want:  "not in the reconstruction table",
		},
		{
			name:  "unlearned_root_file",
			files: map[string]string{"NOTES.txt": "vendored by hand\n"},
			want:  "neither a family directory nor",
		},
		{
			name: "header_drift",
			files: map[string]string{
				"AND_OR/from_simple_and_or.csv": "${statement},${expected_file}\n" +
					"\"SELECT o FROM EHR CONTAINS OBSERVATION o\",expected.json\n",
			},
			want: "the reconstruction table expects",
		},
		{
			name:  "vendored_file_lost",
			files: map[string]string{"AND_OR/from_simple_and_or.csv": goodHeader + goodRow},
			want:  "the corpus does not carry it",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			// Every fixture corpus carries the two ingest files the reader
			// demands; their absence has its own test below.
			for name, body := range tc.files {
				path := filepath.Join(root, name)
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			for _, name := range []string{"AQL_SOURCE.txt", "EXCLUDED.txt"} {
				if err := os.WriteFile(filepath.Join(root, name), []byte("# fixture\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			c, err := aqlprobes.ReadConformanceCorpus(root)
			if err == nil {
				t.Fatalf("err = nil, want a refusal; read %d row(s)", len(c.Rows))
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want it to contain %q", err, tc.want)
			}
		})
	}
}

// TestProbe100ReaderRequiresTheIngestFiles is the able-to-fail control for the
// root-file presence arm: a corpus whose provenance pin or exclusion record
// went missing — a partial vendoring, or a copy that took only the family
// directories — is a read error naming the missing file, not a readable corpus.
func TestProbe100ReaderRequiresTheIngestFiles(t *testing.T) {
	for _, missing := range []string{"AQL_SOURCE.txt", "EXCLUDED.txt"} {
		t.Run(missing, func(t *testing.T) {
			root := probe100CorpusCopy(t)
			if err := os.Remove(filepath.Join(root, missing)); err != nil {
				t.Fatal(err)
			}
			_, err := aqlprobes.ReadConformanceCorpus(root)
			if err == nil {
				t.Fatal("err = nil, want a refusal for the missing ingest file")
			}
			if !strings.Contains(err.Error(), missing) {
				t.Fatalf("err = %v, want it to name %s", err, missing)
			}
		})
	}
}

// probe100CorpusCopy copies the shipping corpus — family directories and the
// ingest's root files — into a throwaway root a test may edit. The reader
// demands the whole table's worth of files plus the provenance pin and
// exclusion record, so a test about ONE row still needs a complete corpus
// around it.
func probe100CorpusCopy(t *testing.T) string {
	t.Helper()
	root, src := t.TempDir(), fixtures.AQLConformanceRoot()
	families, err := os.ReadDir(src)
	if err != nil {
		t.Fatal(err)
	}
	for _, fam := range families {
		if !fam.IsDir() {
			body, err := os.ReadFile(filepath.Join(src, fam.Name()))
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, fam.Name()), body, 0o644); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Join(root, fam.Name()), 0o755); err != nil {
			t.Fatal(err)
		}
		files, err := os.ReadDir(filepath.Join(src, fam.Name()))
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range files {
			body, err := os.ReadFile(filepath.Join(src, fam.Name(), f.Name()))
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, fam.Name(), f.Name()), body, 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	return root
}

// appendCorpusRow appends record as the final line of the named CSV under root
// and returns the 1-based line it lands on.
func appendCorpusRow(t *testing.T, root, name, record string) int {
	t.Helper()
	path := filepath.Join(root, name)
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := strings.TrimRight(string(body), "\n")
	if err := os.WriteFile(path, []byte(text+"\n"+record+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return strings.Count(text, "\n") + 2
}

// TestProbe100ReaderExcludesUnreconstructableRows covers the reader's exclusion
// rules, which no row of the corpus meets at its current pin. They are the
// refresh contract: a row that stops reconstructing into a complete query is
// recorded against a named reason and counted, never dropped on the quiet — so
// the asserted plus excluded tallies always add up to the rows on disk.
func TestProbe100ReaderExcludesUnreconstructableRows(t *testing.T) {
	const file = "AND_OR/from_simple_and_or.csv"
	cases := []struct {
		name   string
		record string
		want   string
	}{
		{
			name:   "blank_variable_cell",
			record: ",expected_from_simple_and_or_6.json,1",
			want:   "empty-variable-value",
		},
		{
			// A row that names a further Robot variable of its own: the
			// reconstruction still holds a placeholder, so it is not a query.
			name:   "row_names_another_robot_variable",
			record: `"SELECT o FROM EHR CONTAINS OBSERVATION o[${archetype}]",expected_from_simple_and_or_6.json,1`,
			want:   "unresolved-template-variable",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			baseline := probe100Corpus(t)
			root := probe100CorpusCopy(t)
			line := appendCorpusRow(t, root, file, tc.record)

			c, err := aqlprobes.ReadConformanceCorpus(root)
			if err != nil {
				t.Fatalf("ReadConformanceCorpus: %v", err)
			}
			if len(c.Rows) != len(baseline.Rows) {
				t.Errorf("asserted rows = %d, want %d — the excluded row must not reach the ratchet",
					len(c.Rows), len(baseline.Rows))
			}
			if len(c.Excluded) != 1 {
				t.Fatalf("excluded rows = %d, want 1: %+v", len(c.Excluded), c.Excluded)
			}
			got := c.Excluded[0]
			if got.Reason != tc.want {
				t.Errorf("reason = %q, want %q", got.Reason, tc.want)
			}
			if got.Why == "" {
				t.Errorf("Why = %q, want the rule's prose — an excluded row must explain itself", got.Why)
			}
			if want := fmt.Sprintf("%s:%d", file, line); got.Where() != want {
				t.Errorf("Where() = %q, want %q", got.Where(), want)
			}
			// The tally must show the exclusion where it happened, not only in
			// the corpus-wide total.
			for _, f := range c.FileCounts() {
				if f.Family+"/"+f.File == file && f.Excluded != 1 {
					t.Errorf("%s: excluded = %d, want 1", file, f.Excluded)
				}
			}
			r, err := aqlprobes.Probe100ConformanceCorpus(c)
			if err != nil {
				t.Fatalf("Probe100: %v", err)
			}
			if r.Status != "pass" {
				t.Fatalf("Probe100 status=%q detail=%q — an excluded row must not fail the ratchet",
					r.Status, r.Detail)
			}
		})
	}
}
