package aql_test

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aql"
	"github.com/cadasto/openehr-sdk-go/openehr/aql/contain"
)

// The five REQ-161 containment codes REQ-162 § Contract scopes the builder
// verification to. Spelled here rather than imported: openehr/aql/internal/semcheck
// is internal to this subtree and reachable from a test in this package, but an
// external caller dispatches on these literals, so the test asserts what a
// caller can actually see.
const (
	codeImpossible     = "aql_impossible_containment"
	codeNotContainable = "aql_contains_not_containable"
	codeArchetype      = "aql_archetype_class_mismatch"
	codeUnknownClass   = "aql_unknown_rm_class"
	codeByReference    = "aql_containment_by_reference"
)

// The three REQ-161 PORTABILITY codes, which are read-side advisories only —
// REQ-162 § Contract scopes read/write parity to the containment codes above,
// and PROBE-097 § parity says so explicitly. The builder verification must
// never emit one.
var portabilityCodes = []string{
	"aql_version_no_predicate",
	"aql_versioned_object_unreferenced",
	"aql_fanout_row_grain",
}

// findingCodes is the sorted code multiset of fs — order-irrelevant, duplicates
// counted, which is the comparison REQ-162 § Contract's parity clause is stated
// in.
func findingCodes(fs []contain.Finding) []string {
	out := make([]string, 0, len(fs))
	for _, f := range fs {
		out = append(out, f.Code)
	}
	slices.Sort(out)
	return out
}

// TestFindingCarriesOnlyCodeAndDetail pins REQ-162 § Contract's field
// classification structurally: a builder tree has no source text to point into,
// so a finding carries a value-free Code and a value-bearing Detail — and no
// Span, Path, or severity. A field added to [contain.Finding] fails here, which
// is the point: the read side's richer [lint.Issue] is the shape that carries
// those, and the two models are deliberately not the same.
func TestFindingCarriesOnlyCodeAndDetail(t *testing.T) {
	t.Parallel()
	typ := reflect.TypeFor[contain.Finding]()
	var got []string
	for f := range typ.Fields() {
		got = append(got, f.Name)
	}
	if want := []string{"Code", "Detail"}; !slices.Equal(got, want) {
		t.Errorf("contain.Finding fields = %v, want exactly %v (REQ-162 § Contract: no Span, Path, or severity)", got, want)
	}
}

// TestVerifyContainmentRaisesEachCode is REQ-162 § Contract's catalogue arm:
// each of the FIVE containment codes is raised from a builder-constructed query
// carrying exactly that defect, and nothing else is.
//
// The defect queries are the read side's own fixtures (openehr/aql/lint's
// semantic_test.go), rebuilt through the containment algebra — the same pairs,
// so a divergence between the two adapters shows up as a different code here
// rather than only in the parity table.
func TestVerifyContainmentRaisesEachCode(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		build   func() *aql.Builder
		code    string
		details []string // substrings the value-bearing Detail must quote
	}{
		{
			name: "impossible containment: no route connects the pair",
			build: func() *aql.Builder {
				return aql.NewBuilder().Select(aql.Col("o")).From("OBSERVATION", "o").
					Contains(aql.Class("COMPOSITION", "c"))
			},
			code:    codeImpossible,
			details: []string{"OBSERVATION", "COMPOSITION"},
		},
		{
			name: "not containable: a known class that is no containment target",
			build: func() *aql.Builder {
				return aql.NewBuilder().Select(aql.Col("c")).From("COMPOSITION", "c").
					Contains(aql.Class("DV_TEXT", "t"))
			},
			code:    codeNotContainable,
			details: []string{"DV_TEXT"},
		},
		{
			name: "unknown RM class: not known to the relation",
			build: func() *aql.Builder {
				return aql.NewBuilder().Select(aql.Col("c")).From("COMPOSITION", "c").
					Contains(aql.Class("FOO_BAR", "f"))
			},
			code:    codeUnknownClass,
			details: []string{"FOO_BAR"},
		},
		{
			name: "containment by reference: resolvable only across a reference hop",
			build: func() *aql.Builder {
				return aql.NewBuilder().Select(aql.Col("c")).From("FOLDER", "f").
					Contains(aql.Class("COMPOSITION", "c"))
			},
			code:    codeByReference,
			details: []string{"FOLDER", "COMPOSITION"},
		},
		{
			name: "archetype/class mismatch: the HRID's type segment is another class",
			build: func() *aql.Builder {
				return aql.NewBuilder().Select(aql.Col("c")).From("COMPOSITION", "c").
					Contains(aql.Archetype("EVALUATION", "ev", "openEHR-EHR-OBSERVATION.body_temperature.v2"))
			},
			code:    codeArchetype,
			details: []string{"EVALUATION", "openEHR-EHR-OBSERVATION.body_temperature.v2"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			b := tc.build()
			// The defect never blocks construction (REQ-162 § Contract).
			if _, err := b.Build(); err != nil {
				t.Fatalf("Build() = %v, want nil — an RM defect must not become a Build error", err)
			}
			fs := b.VerifyContainment(nil)
			if got := findingCodes(fs); !slices.Equal(got, []string{tc.code}) {
				t.Fatalf("VerifyContainment codes = %v, want exactly [%s]", got, tc.code)
			}
			for _, want := range tc.details {
				if !strings.Contains(fs[0].Detail, want) {
					t.Errorf("Detail = %q, want it to quote %q (value-bearing per REQ-162 § Contract)", fs[0].Detail, want)
				}
			}
		})
	}
}

// TestVerifyContainmentCleanQueriesAreSilent pins the conservative direction of
// REQ-161 § Flagging policy on the write side: an RM-valid query — including the
// EHR/VERSION tier and the containment algebra's junction and NOT CONTAINS
// shapes — draws nothing at all. A false Error is worse than a missed defect,
// and this is the arm that fails when one is invented.
func TestVerifyContainmentCleanQueriesAreSilent(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		build func() *aql.Builder
	}{
		{
			name: "composition contains observation",
			build: func() *aql.Builder {
				return aql.NewBuilder().Select(aql.Col("o")).From("COMPOSITION", "c").
					Contains(aql.Class("OBSERVATION", "o"))
			},
		},
		{
			name: "ehr tier, chained",
			build: func() *aql.Builder {
				return aql.NewBuilder().Select(aql.Col("o")).FromEHR("e", aql.Param("ehr_id")).
					Contains(aql.Class("COMPOSITION", "c")).
					Contains(aql.Class("OBSERVATION", "o"))
			},
		},
		{
			name: "version tier",
			build: func() *aql.Builder {
				return aql.NewBuilder().Select(aql.Col("c")).From("EHR", "e").
					Contains(aql.Class("VERSION", "v").Contains(aql.Class("COMPOSITION", "c")))
			},
		},
		{
			name: "junction of two admissible operands",
			build: func() *aql.Builder {
				return aql.NewBuilder().Select(aql.Col("c")).From("COMPOSITION", "c").
					Contains(aql.ContainsOr(aql.Class("OBSERVATION", "o"), aql.Class("EVALUATION", "ev")))
			},
		},
		{
			name: "NOT CONTAINS over an admissible pair",
			build: func() *aql.Builder {
				return aql.NewBuilder().Select(aql.Col("c")).From("COMPOSITION", "c").
					NotContains(aql.Class("OBSERVATION", "o"))
			},
		},
		{
			name: "conforming archetype predicate",
			build: func() *aql.Builder {
				return aql.NewBuilder().Select(aql.Col("o")).From("COMPOSITION", "c").
					Contains(aql.Archetype("OBSERVATION", "o", "openEHR-EHR-OBSERVATION.body_temperature.v2"))
			},
		},
		{
			name: "$param archetype predicate is skipped, not lexed",
			build: func() *aql.Builder {
				return aql.NewBuilder().Select(aql.Col("o")).From("COMPOSITION", "c").
					Contains(aql.Archetype("OBSERVATION", "o", "$arch"))
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			b := tc.build()
			if _, err := b.Build(); err != nil {
				t.Fatalf("premise broken: Build() = %v, want nil", err)
			}
			if got := findingCodes(b.VerifyContainment(nil)); len(got) != 0 {
				t.Errorf("VerifyContainment codes = %v, want silence", got)
			}
		})
	}
}

// TestVerifyContainmentJunctionNeverBecomesPredecessor pins
// containVerifier.chain's junction guard (openehr/aql/verify.go): a junction
// entry in a CONTAINS chain must never become the predecessor a FOLLOWING
// entry in the SAME chain is checked adjacent to — a junction decides
// nothing of its own ([contain.Finding]'s zero-Operand comment), so treating
// it as a predecessor would ask every subsequent pair of the zero Operand,
// which suppresses everything it takes part in and silently drops a real
// defect.
//
// [Builder.Build] refuses a junction with a further term after it (the
// grammar admits no such spelling), so this shape is reachable only by
// calling VerifyContainment directly without Build — exactly the case that
// method's own doc comment names ("verification judges whatever tree it is
// handed, including one Build would refuse"). [Builder.Contains] appends to
// the FROM clause's own top-level chain with no validation of its own (that
// is [Builder.Build]'s job), so two ordinary top-level calls reach it.
func TestVerifyContainmentJunctionNeverBecomesPredecessor(t *testing.T) {
	t.Parallel()
	b := aql.NewBuilder().Select(aql.Col("s")).From("OBSERVATION", "o").
		Contains(aql.ContainsOr(aql.Class("ELEMENT", "e1"), aql.Class("CLUSTER", "cl1"))).
		Contains(aql.Class("SECTION", "s"))

	if _, err := b.Build(); err == nil {
		t.Fatal("premise broken: Build() = nil, want an error — a junction may only END a chain")
	}

	// OBSERVATION→SECTION is a documented Never (semantic_test.go's file
	// doc). If the junction wrongly became SECTION's predecessor, the pair
	// would be asked of the zero Operand instead, which suppresses every
	// pair it takes part in, and this code would go missing — the junction's
	// OWN operands (checked against the true predecessor, OBSERVATION) stay
	// silent regardless, since OBSERVATION→ELEMENT and OBSERVATION→CLUSTER
	// are both Admissible, so they contribute no noise either way.
	if got := findingCodes(b.VerifyContainment(nil)); !slices.Equal(got, []string{codeImpossible}) {
		t.Errorf("VerifyContainment codes = %v, want exactly [%s] "+
			"(SECTION must be checked adjacent to the OBSERVATION root, not the junction)", got, codeImpossible)
	}
}

// TestVerifyContainmentNeverEmitsPortabilityCodes pins the five-not-eight scope
// of REQ-162 § Contract (PROBE-097 § parity): the three REQ-161 portability
// advisories are read-side only. The queries below are exactly the shapes that
// raise them on the read side — a bare VERSION, an unreferenced
// VERSIONED_COMPOSITION, and an AND junction with two projected operands — so
// this fails the moment the write side starts producing them.
func TestVerifyContainmentNeverEmitsPortabilityCodes(t *testing.T) {
	t.Parallel()
	builders := map[string]*aql.Builder{
		"bare VERSION (no version predicate)": aql.NewBuilder().Select(aql.Col("v")).From("EHR", "e").
			Contains(aql.Class("VERSION", "v")),
		"unreferenced VERSIONED_COMPOSITION": aql.NewBuilder().Select(aql.Col("e")).From("EHR", "e").
			Contains(aql.Class("VERSIONED_COMPOSITION", "vc")),
		"AND junction with two projected operands": aql.NewBuilder().
			Select(aql.Col("o"), aql.Col("ev")).From("COMPOSITION", "c").
			Contains(aql.ContainsAnd(aql.Class("OBSERVATION", "o"), aql.Class("EVALUATION", "ev"))),
	}
	for name, b := range builders {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			for _, f := range b.VerifyContainment(nil) {
				if slices.Contains(portabilityCodes, f.Code) {
					t.Errorf("write side emitted the read-side-only code %s (REQ-162 § Contract scopes it to lint)", f.Code)
				}
			}
		})
	}
}

// TestVerifyContainmentNilRelationIsDefault pins the signature's nil contract
// (REQ-162): nil means the REQ-160 default relation. Nil DISABLES nothing — the
// method has no "off" value, unlike lint's Options.Compiled / Options.Query.
func TestVerifyContainmentNilRelationIsDefault(t *testing.T) {
	t.Parallel()
	builders := []*aql.Builder{
		aql.NewBuilder().Select(aql.Col("o")).From("OBSERVATION", "o").Contains(aql.Class("COMPOSITION", "c")),
		aql.NewBuilder().Select(aql.Col("c")).From("FOLDER", "f").Contains(aql.Class("COMPOSITION", "c")),
		aql.NewBuilder().Select(aql.Col("c")).From("COMPOSITION", "c").Contains(aql.Class("FOO_BAR", "f")),
		aql.NewBuilder().Select(aql.Col("o")).From("COMPOSITION", "c").Contains(aql.Class("OBSERVATION", "o")),
	}
	for _, b := range builders {
		q, err := b.Build()
		if err != nil {
			t.Fatalf("premise broken: Build() = %v", err)
		}
		t.Run(q.Q, func(t *testing.T) {
			t.Parallel()
			nilRel := findingCodes(b.VerifyContainment(nil))
			explicit := findingCodes(b.VerifyContainment(contain.Default()))
			if !slices.Equal(nilRel, explicit) {
				t.Errorf("nil relation = %v but contain.Default() = %v", nilRel, explicit)
			}
		})
	}
	// …and nil is not a silent no-op: the first builder carries a real defect.
	if got := findingCodes(builders[0].VerifyContainment(nil)); !slices.Equal(got, []string{codeImpossible}) {
		t.Errorf("nil relation reported %v; nil selects the default relation, it does not disable the check", got)
	}
}

// TestVerifyContainmentHonoursOverlayRelation is the dialect arm of REQ-162's
// relation parameter: a deployment whose overlay genuinely admits a containment
// the pinned RM does not gets no false finding, for an Error verdict and for an
// unknown class alike (REQ-160 § Extensibility). Without this the parameter
// could be ignored and every other test here would still pass.
func TestVerifyContainmentHonoursOverlayRelation(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		build   func() *aql.Builder
		edge    contain.Edge
		wantDef []string
	}{
		{
			name: "overlay retires an impossible pair",
			build: func() *aql.Builder {
				return aql.NewBuilder().Select(aql.Col("o")).From("OBSERVATION", "o").
					Contains(aql.Class("COMPOSITION", "c"))
			},
			edge:    contain.Edge{From: "OBSERVATION", To: "COMPOSITION"},
			wantDef: []string{codeImpossible},
		},
		{
			name: "overlay retires an unknown class",
			build: func() *aql.Builder {
				return aql.NewBuilder().Select(aql.Col("c")).From("FOO_BAR", "f").
					Contains(aql.Class("COMPOSITION", "c"))
			},
			edge:    contain.Edge{From: "FOO_BAR", To: "COMPOSITION"},
			wantDef: []string{codeUnknownClass},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			b := tc.build()
			if got := findingCodes(b.VerifyContainment(nil)); !slices.Equal(got, tc.wantDef) {
				t.Fatalf("premise broken: default relation reports %v, want %v", got, tc.wantDef)
			}
			if got := findingCodes(b.VerifyContainment(contain.Default().WithOverlay(tc.edge))); len(got) != 0 {
				t.Errorf("overlay relation still reports %v", got)
			}
		})
	}
}

// TestBuildUnchangedByImpossibleContainment is the permissiveness contract of
// REQ-162 § Contract, pinned: a query carrying a Never pair still builds without
// error and emits BYTE-IDENTICALLY to the pre-REQ-162 SDK. The expected strings
// were captured by running the builder at HEAD 089e387 — the commit before
// verify.go existed — not written from expectation.
//
// Build answers the grammar question and nothing else. A CDR is entitled to
// refuse such a query, and a caller is entitled to send it anyway (probing a
// dialect, reproducing a bug report, exercising a conformance corpus), so the
// SDK must not decide for them.
func TestBuildUnchangedByImpossibleContainment(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		build func() *aql.Builder
		want  string
	}{
		{
			name: "the plain Never pair",
			build: func() *aql.Builder {
				return aql.NewBuilder().Select(aql.Col("o")).From("OBSERVATION", "o").
					Contains(aql.Class("COMPOSITION", "c"))
			},
			want: "SELECT o FROM OBSERVATION o CONTAINS COMPOSITION c",
		},
		{
			name: "negated, so the exclusion is a dead constraint",
			build: func() *aql.Builder {
				return aql.NewBuilder().Select(aql.Col("o")).From("OBSERVATION", "o").
					NotContains(aql.Class("COMPOSITION", "c"))
			},
			want: "SELECT o FROM OBSERVATION o NOT CONTAINS COMPOSITION c",
		},
		{
			name: "four defects across a chain, a junction and an archetype predicate",
			build: func() *aql.Builder {
				return aql.NewBuilder().
					Select(aql.Col("o"), aql.Col("c")).
					From("OBSERVATION", "o").
					NotContains(aql.Class("FOO_BAR", "f")).
					Contains(aql.Class("COMPOSITION", "c").Contains(aql.ContainsOr(
						aql.Archetype("EVALUATION", "ev", "openEHR-EHR-OBSERVATION.body_temperature.v2"),
						aql.Class("DV_TEXT", "t"),
					)))
			},
			want: "SELECT o, c FROM OBSERVATION o NOT CONTAINS FOO_BAR f CONTAINS COMPOSITION c " +
				"CONTAINS (EVALUATION ev[openEHR-EHR-OBSERVATION.body_temperature.v2] OR DV_TEXT t)",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			b := tc.build()
			q, err := b.Build()
			if err != nil {
				t.Fatalf("Build() = %v, want nil (REQ-162 § Contract: the validation set is unchanged)", err)
			}
			if q.Q != tc.want {
				t.Errorf("emitted\n  %q\nwant\n  %q", q.Q, tc.want)
			}
			// (No ErrInvalidQuery assertion here: the err == nil check above
			// already carries it. An `errors.Is(err, …)` after a t.Fatalf on
			// err != nil is provably testing a nil error, so it asserts nothing.)
			//
			// The verification does see the defects — otherwise this test would
			// pass for the wrong reason (a check that never runs).
			if got := findingCodes(b.VerifyContainment(nil)); len(got) == 0 {
				t.Error("premise broken: VerifyContainment reported nothing on a defective query")
			}
		})
	}
}

// TestVerificationNeverRunsImplicitly pins the other half of REQ-162
// § Contract's Build clause: verification is opt-in, so nothing an existing code
// path does can trigger it, and calling it changes nothing.
//
// Build is the only path a pre-REQ-162 caller has; it returns a [aql.Query],
// whose fields carry AQL text and paging — there is no channel a finding could
// arrive on even if one were produced. What is observable, and asserted here, is
// that Build stays error-free and byte-stable across an explicit verification,
// and that verification is idempotent and does not mutate the builder.
func TestVerificationNeverRunsImplicitly(t *testing.T) {
	t.Parallel()
	b := aql.NewBuilder().Select(aql.Col("o")).From("OBSERVATION", "o").
		Contains(aql.Class("COMPOSITION", "c"))

	before, err := b.Build()
	if err != nil {
		t.Fatalf("Build() before verification = %v, want nil", err)
	}
	first := findingCodes(b.VerifyContainment(nil))
	second := findingCodes(b.VerifyContainment(nil))
	if !slices.Equal(first, second) {
		t.Errorf("VerifyContainment is not idempotent: %v then %v", first, second)
	}
	after, err := b.Build()
	if err != nil {
		t.Fatalf("Build() after verification = %v, want nil", err)
	}
	if before.Q != after.Q {
		t.Errorf("verification perturbed the builder: emitted %q then %q", before.Q, after.Q)
	}
	// Repeated Build calls on a defective query keep succeeding: no accumulated
	// state turns an RM verdict into a validation error on a later call.
	if _, err := b.Build(); err != nil {
		t.Errorf("third Build() = %v, want nil", err)
	}
}

// TestVerifyContainmentJunctionsAndNegation pins the containment-algebra
// traversal REQ-162 § Contract requires the write side to walk: junction
// operands are checked against the junction's ENCLOSING parent, mixed AND/OR
// nesting recurses, and NOT CONTAINS is checked IDENTICALLY to CONTAINS —
// possibility does not care about the sign (REQ-161 § Checks).
func TestVerifyContainmentJunctionsAndNegation(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		build func() *aql.Builder
		want  []string
	}{
		{
			// Both operands are asked against COMPOSITION, not against the
			// junction: OBSERVATION is admissible, DV_TEXT is no containment
			// target at all (and suppresses its own pair).
			name: "OR junction: each operand takes the enclosing parent",
			build: func() *aql.Builder {
				return aql.NewBuilder().Select(aql.Col("c")).From("COMPOSITION", "c").
					Contains(aql.ContainsOr(aql.Class("OBSERVATION", "o"), aql.Class("DV_TEXT", "t")))
			},
			want: []string{codeNotContainable},
		},
		{
			// The junction sits at the top of the FROM root's chain, so the
			// enclosing parent is the ROOT — and the root here cannot contain
			// either operand.
			name: "AND junction under an impossible root",
			build: func() *aql.Builder {
				return aql.NewBuilder().Select(aql.Col("o")).From("OBSERVATION", "o").
					Contains(aql.ContainsAnd(aql.Class("COMPOSITION", "c1"), aql.Class("COMPOSITION", "c2")))
			},
			want: []string{codeImpossible, codeImpossible},
		},
		{
			// Mixed AND/OR nesting: the inner OR is a junction OPERAND of the
			// outer AND, and its own operands still take the outer junction's
			// enclosing parent.
			name: "nested mixed junction",
			build: func() *aql.Builder {
				return aql.NewBuilder().Select(aql.Col("c")).From("COMPOSITION", "c").
					Contains(aql.ContainsAnd(
						aql.Class("OBSERVATION", "o"),
						aql.ContainsOr(aql.Class("EVALUATION", "ev"), aql.Class("DV_TEXT", "t")),
					))
			},
			want: []string{codeNotContainable},
		},
		{
			name: "NOT CONTAINS over an impossible pair still fires",
			build: func() *aql.Builder {
				return aql.NewBuilder().Select(aql.Col("o")).From("OBSERVATION", "o").
					NotContains(aql.Class("COMPOSITION", "c"))
			},
			want: []string{codeImpossible},
		},
		{
			name: "NOT CONTAINS inside a nested chain",
			build: func() *aql.Builder {
				return aql.NewBuilder().Select(aql.Col("c")).From("COMPOSITION", "c").
					Contains(aql.Class("OBSERVATION", "o").NotContains(aql.Class("DV_TEXT", "t")))
			},
			want: []string{codeNotContainable},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			b := tc.build()
			if _, err := b.Build(); err != nil {
				t.Fatalf("premise broken: Build() = %v", err)
			}
			if got := findingCodes(b.VerifyContainment(nil)); !slices.Equal(got, tc.want) {
				t.Errorf("VerifyContainment codes = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestVerifyContainmentPairsAreChainAdjacent pins that the pair question follows
// the FLATTENED chain — the text [ast.build] actually emits — rather than
// fanning out from each entry's head (REQ-161 § Checks: adjacent pairs only, no
// synthesised transitive pair).
//
// The tree below nests COMPOSITION under FOLDER and appends OBSERVATION as a
// second top-level Contains, which emits as one four-step chain. The only
// reference hop in it is FOLDER→COMPOSITION. Pairing each entry against the
// chain HEAD instead would additionally ask FOLDER→OBSERVATION and report a
// second, spurious by-reference finding, so the count is the assertion.
func TestVerifyContainmentPairsAreChainAdjacent(t *testing.T) {
	t.Parallel()
	b := aql.NewBuilder().Select(aql.Col("o")).From("EHR", "e").
		Contains(aql.Class("FOLDER", "f").Contains(aql.Class("COMPOSITION", "c"))).
		Contains(aql.Class("OBSERVATION", "o"))
	q, err := b.Build()
	if err != nil {
		t.Fatalf("Build() = %v", err)
	}
	if want := "SELECT o FROM EHR e CONTAINS FOLDER f CONTAINS COMPOSITION c CONTAINS OBSERVATION o"; q.Q != want {
		t.Fatalf("premise broken: emitted %q, want %q", q.Q, want)
	}
	if got := findingCodes(b.VerifyContainment(nil)); !slices.Equal(got, []string{codeByReference}) {
		t.Errorf("codes = %v, want exactly [%s] — one per ADJACENT pair, none synthesised", got, codeByReference)
	}
}

// TestVerifyContainmentDegenerateBuilders pins REQ-025 on this entry point: a
// builder no [aql.Builder.Build] would accept is verified without a panic and
// without a manufactured finding. Verification is a diagnostic, so it must
// survive every tree a caller can hand it — including the ones Build refuses.
//
// Silence is NOT a property of a degenerate tree, and this test must not be read
// as claiming it is: a contained term is still checked on its own account, so
// `Select(Col("t")).Contains(Class("DV_TEXT","t"))` with no FROM at all
// correctly reports aql_contains_not_containable. Every fixture below is
// deliberately RM-CLEAN, so the invariant actually pinned is narrower and
// exact — no panic, and no finding MANUFACTURED out of a missing operand: a
// missing FROM root, a class-less term, or an empty junction decides nothing and
// suppresses the pairs around it rather than being reported against the empty
// name. A fixture added here that carries a real RM defect belongs in
// [TestVerifyContainmentRaisesEachCode] instead; adding one here would fail for
// the wrong reason.
func TestVerifyContainmentDegenerateBuilders(t *testing.T) {
	t.Parallel()
	cases := map[string]*aql.Builder{
		"nil builder":                 nil,
		"zero builder":                aql.NewBuilder(),
		"no FROM, only a containment": aql.NewBuilder().Select(aql.Col("o")).Contains(aql.Class("OBSERVATION", "o")),
		"zero Containment term":       aql.NewBuilder().Select(aql.Col("c")).From("COMPOSITION", "c").Contains(aql.Containment{}),
		"empty junction":              aql.NewBuilder().Select(aql.Col("c")).From("COMPOSITION", "c").Contains(aql.ContainsOr()),
		"junction in a non-final chain position": aql.NewBuilder().Select(aql.Col("c")).From("COMPOSITION", "c").
			Contains(aql.ContainsOr(aql.Class("OBSERVATION", "o"), aql.Class("EVALUATION", "ev"))).
			Contains(aql.Class("SECTION", "s")),
		"CONTAINS below a junction": aql.NewBuilder().Select(aql.Col("c")).From("COMPOSITION", "c").
			Contains(aql.ContainsOr(aql.Class("OBSERVATION", "o"), aql.Class("EVALUATION", "ev")).
				Contains(aql.Class("CLUSTER", "cl"))),
	}
	for name, b := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := findingCodes(b.VerifyContainment(nil)); len(got) != 0 {
				t.Errorf("VerifyContainment codes = %v, want none: this tree carries no RM defect, "+
					"so a finding here is one manufactured out of a missing operand", got)
			}
		})
	}
}
