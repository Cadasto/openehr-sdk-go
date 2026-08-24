package semcheck_test

// semcheck_test.go: the REQ-161 rule engine itself — verdict→code
// classification, the operand/pair split, and the suppression rule between
// them. The two adapters (read-side lint, write-side builder verification) are
// tested where they live; what is pinned HERE is the shared decision they both
// consume, so a drift between them has to show up as a failure here first.
//
// The class pairs are taken from REQ-160's acceptance table (already pinned by
// openehr/aql/contain's own tests) rather than invented: OBSERVATION CONTAINS
// COMPOSITION is Never, FOLDER CONTAINS COMPOSITION is ByReference, COMPOSITION
// CONTAINS ELEMENT is Admissible, DV_TEXT is a Never-containability operand,
// FOO_BAR is UnknownClass.

import (
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aql/contain"
	"github.com/cadasto/openehr-sdk-go/openehr/aql/internal/semcheck"
)

func codesOf(fs []contain.Finding) []string {
	out := make([]string, len(fs))
	for i, f := range fs {
		out[i] = f.Code
	}
	return out
}

// TestSeverityOfIsTheOneCatalogue pins the single code→severity table both
// adapters read (REQ-161 § Checks). A severity that moves has to move here.
func TestSeverityOfIsTheOneCatalogue(t *testing.T) {
	t.Parallel()
	cases := []struct {
		code string
		want semcheck.Severity
	}{
		{semcheck.CodeImpossibleContainment, semcheck.Error},
		{semcheck.CodeNotContainable, semcheck.Error},
		{semcheck.CodeUnknownRMClass, semcheck.Warning},
		{semcheck.CodeContainmentByReference, semcheck.Warning},
		{semcheck.CodeArchetypeClassMismatch, semcheck.Error},
	}
	for _, tc := range cases {
		t.Run(tc.code, func(t *testing.T) {
			t.Parallel()
			got, ok := semcheck.SeverityOf(tc.code)
			if !ok {
				t.Fatalf("SeverityOf(%q): not in the catalogue", tc.code)
			}
			if got != tc.want {
				t.Errorf("SeverityOf(%q) = %v, want %v", tc.code, got, tc.want)
			}
		})
	}
	if _, ok := semcheck.SeverityOf("aql_not_a_code"); ok {
		t.Error(`SeverityOf("aql_not_a_code") reported a severity; an unlisted code MUST NOT resolve`)
	}
}

// TestCodeSpellings pins the wire-visible strings. They are consumer-facing
// identifiers (REQ-161 § Checks) and a typo is a silent contract break.
func TestCodeSpellings(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		semcheck.CodeImpossibleContainment:  "aql_impossible_containment",
		semcheck.CodeNotContainable:         "aql_contains_not_containable",
		semcheck.CodeUnknownRMClass:         "aql_unknown_rm_class",
		semcheck.CodeContainmentByReference: "aql_containment_by_reference",
		semcheck.CodeArchetypeClassMismatch: "aql_archetype_class_mismatch",
	}
	for got, want := range cases {
		if got != want {
			t.Errorf("code %q != %q", got, want)
		}
	}
}

// TestOperandDecision pins the per-operand half (REQ-161 § Checks): which code
// an operand raises, and whether it suppresses the pair checks it takes part in.
//
// The RoleRoot rows are the load-bearing ones: aql_contains_not_containable is
// scoped to a CONTAINS operand, so a non-containable FROM root raises nothing
// while still suppressing — whereas an UNKNOWN class raises in either role,
// because REQ-161 § Flagging policy forbids an unknown name being silent.
func TestOperandDecision(t *testing.T) {
	t.Parallel()
	ck := semcheck.New(nil)
	cases := []struct {
		name          string
		rmType        string
		role          semcheck.Role
		wantCodes     []string
		wantSuppress  bool
		wantContainOK contain.Verdict
	}{
		{
			name: "containable contained operand", rmType: "COMPOSITION", role: semcheck.RoleContained,
			wantCodes: nil, wantSuppress: false, wantContainOK: contain.Admissible,
		},
		{
			name: "containable root", rmType: "EHR", role: semcheck.RoleRoot,
			wantCodes: nil, wantSuppress: false, wantContainOK: contain.Admissible,
		},
		{
			name: "case-insensitive match", rmType: "composition", role: semcheck.RoleContained,
			wantCodes: nil, wantSuppress: false, wantContainOK: contain.Admissible,
		},
		{
			name: "data value as CONTAINS operand", rmType: "DV_TEXT", role: semcheck.RoleContained,
			wantCodes: []string{semcheck.CodeNotContainable}, wantSuppress: true, wantContainOK: contain.Never,
		},
		{
			name: "non-LOCATABLE as CONTAINS operand", rmType: "EVENT_CONTEXT", role: semcheck.RoleContained,
			wantCodes: []string{semcheck.CodeNotContainable}, wantSuppress: true, wantContainOK: contain.Never,
		},
		{
			name: "data value as FROM root raises nothing", rmType: "DV_TEXT", role: semcheck.RoleRoot,
			wantCodes: nil, wantSuppress: true, wantContainOK: contain.Never,
		},
		{
			name: "unknown class as CONTAINS operand", rmType: "FOO_BAR", role: semcheck.RoleContained,
			wantCodes: []string{semcheck.CodeUnknownRMClass}, wantSuppress: true, wantContainOK: contain.UnknownClass,
		},
		{
			name: "unknown class as FROM root still warns", rmType: "FOO_BAR", role: semcheck.RoleRoot,
			wantCodes: []string{semcheck.CodeUnknownRMClass}, wantSuppress: true, wantContainOK: contain.UnknownClass,
		},
		{
			name: "empty class names nothing", rmType: "", role: semcheck.RoleContained,
			wantCodes: nil, wantSuppress: true, wantContainOK: contain.UnknownClass,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			o := ck.Operand(tc.rmType, tc.role)
			if got := codesOf(o.Findings()); !slices.Equal(got, tc.wantCodes) {
				t.Errorf("Operand(%q, %v) findings = %v, want %v", tc.rmType, tc.role, got, tc.wantCodes)
			}
			if got := o.Suppresses(); got != tc.wantSuppress {
				t.Errorf("Operand(%q, %v).Suppresses() = %v, want %v", tc.rmType, tc.role, got, tc.wantSuppress)
			}
			if got := o.Verdict(); got != tc.wantContainOK {
				t.Errorf("Operand(%q, %v).Verdict() = %v, want %v", tc.rmType, tc.role, got, tc.wantContainOK)
			}
			if got := o.RMType(); got != tc.rmType {
				t.Errorf("Operand(%q, …).RMType() = %q", tc.rmType, got)
			}
		})
	}
}

// TestRoleAssignment pins the role-assignment rule ([semcheck.Role.Next]) —
// the contract that keeps the read-side and write-side adapters reporting the
// same multiset (REQ-162 § Contract).
//
// The load-bearing row is StepJunction from RoleRoot: a junction is boolean
// grouping, not a containment step, so its operands occupy the SAME position
// the junction does. `FROM A a OR B b` holds ROOT operands, and an adapter that
// labelled them contained for being junction children would make the two
// spellings of a root position disagree.
func TestRoleAssignment(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		from semcheck.Role
		step semcheck.Step
		want semcheck.Role
	}{
		{"CONTAINS from the FROM root", semcheck.RoleRoot, semcheck.StepContains, semcheck.RoleContained},
		{"CONTAINS below a CONTAINS", semcheck.RoleContained, semcheck.StepContains, semcheck.RoleContained},
		{"junction at the FROM root keeps roots", semcheck.RoleRoot, semcheck.StepJunction, semcheck.RoleRoot},
		{"junction under a CONTAINS keeps contained", semcheck.RoleContained, semcheck.StepJunction, semcheck.RoleContained},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.from.Next(tc.step); got != tc.want {
				t.Errorf("Role(%d).Next(%d) = %d, want %d", tc.from, tc.step, got, tc.want)
			}
		})
	}

	// A junction nested in a junction keeps propagating the enclosing position,
	// so `FROM A a OR (B b AND C c)` holds roots all the way down.
	deep := semcheck.RoleRoot.Next(semcheck.StepJunction).Next(semcheck.StepJunction)
	if deep != semcheck.RoleRoot {
		t.Errorf("nested root junctions gave role %d, want RoleRoot (%d)", deep, semcheck.RoleRoot)
	}
	// A CONTAINS anywhere along the way is one-way: nothing returns to root.
	back := semcheck.RoleRoot.Next(semcheck.StepContains).Next(semcheck.StepJunction)
	if back != semcheck.RoleContained {
		t.Errorf("a junction under a CONTAINS gave role %d, want RoleContained (%d)", back, semcheck.RoleContained)
	}
}

// TestRootRoleIsSilentForNotContainable is the controller ruling in engine
// terms: a non-containable class in ANY root position raises nothing, while the
// same class as a CONTAINS operand raises the Error. Root silence is a
// documented missed defect (REQ-161 § Flagging policy — a false Error is worse),
// and it must not depend on which root spelling the adapter walked.
func TestRootRoleIsSilentForNotContainable(t *testing.T) {
	t.Parallel()
	ck := semcheck.New(nil)
	for _, rmType := range []string{"DV_TEXT", "DV_QUANTITY", "EVENT_CONTEXT"} {
		t.Run(rmType, func(t *testing.T) {
			t.Parallel()
			// Every route an adapter can take to a root position must agree.
			roots := []semcheck.Role{
				semcheck.RoleRoot,
				semcheck.RoleRoot.Next(semcheck.StepJunction),
				semcheck.RoleRoot.Next(semcheck.StepJunction).Next(semcheck.StepJunction),
			}
			for i, role := range roots {
				o := ck.Operand(rmType, role)
				if got := codesOf(o.Findings()); len(got) != 0 {
					t.Errorf("root route %d: %s reported %v, want nothing", i, rmType, got)
				}
				if !o.Suppresses() {
					t.Errorf("root route %d: %s does not suppress; no pair may be built on it", i, rmType)
				}
			}
			// The scope of that silence: as a CONTAINS operand the same class
			// is an Error.
			contained := ck.Operand(rmType, semcheck.RoleRoot.Next(semcheck.StepContains))
			if got := codesOf(contained.Findings()); !slices.Equal(got, []string{semcheck.CodeNotContainable}) {
				t.Errorf("%s as a CONTAINS operand = %v, want [%s]", rmType, got, semcheck.CodeNotContainable)
			}
		})
	}
}

// TestPairDecision pins the per-pair half over operands that both survived the
// operand checks: Never → Error code, ByReference → Warning code, Admissible →
// silence (REQ-161 § Checks).
func TestPairDecision(t *testing.T) {
	t.Parallel()
	ck := semcheck.New(nil)
	cases := []struct {
		ancestor, descendant string
		wantCodes            []string
	}{
		{"OBSERVATION", "COMPOSITION", []string{semcheck.CodeImpossibleContainment}},
		{"OBSERVATION", "OBSERVATION", []string{semcheck.CodeImpossibleContainment}},
		{"ELEMENT", "CLUSTER", []string{semcheck.CodeImpossibleContainment}},
		{"COMPOSITION", "EHR", []string{semcheck.CodeImpossibleContainment}},
		{"FOLDER", "COMPOSITION", []string{semcheck.CodeContainmentByReference}},
		{"FOLDER", "OBSERVATION", []string{semcheck.CodeContainmentByReference}},
		{"COMPOSITION", "ELEMENT", nil},
		{"EHR", "COMPOSITION", nil},
		{"SECTION", "SECTION", nil},
		{"VERSION", "COMPOSITION", nil},
	}
	for _, tc := range cases {
		t.Run(tc.ancestor+"_contains_"+tc.descendant, func(t *testing.T) {
			t.Parallel()
			a := ck.Operand(tc.ancestor, semcheck.RoleRoot)
			d := ck.Operand(tc.descendant, semcheck.RoleContained)
			if got := codesOf(ck.Pair(a, d)); !slices.Equal(got, tc.wantCodes) {
				t.Errorf("Pair(%s, %s) = %v, want %v", tc.ancestor, tc.descendant, got, tc.wantCodes)
			}
		})
	}
}

// TestPairSuppressedByOperandVerdict is the suppression rule itself (REQ-161
// § Checks): an operand already reported through its own code MUST NOT also
// produce a pair code.
//
// Each row first asserts that the relation's own TOTAL pair question does
// answer Never or UnknownClass for the pair — otherwise the row would pass on
// an Admissible pair and prove nothing. That is the whole point: CanContain's
// short-circuit is real, and semcheck deliberately declines to map it.
func TestPairSuppressedByOperandVerdict(t *testing.T) {
	t.Parallel()
	rel := contain.Default()
	ck := semcheck.New(rel)
	cases := []struct {
		name                 string
		ancestor, descendant string
		ancestorRole         semcheck.Role
	}{
		{"never-containability descendant", "COMPOSITION", "DV_TEXT", semcheck.RoleRoot},
		{"never-containability ancestor", "DV_TEXT", "ELEMENT", semcheck.RoleRoot},
		{"unknown descendant", "COMPOSITION", "FOO_BAR", semcheck.RoleRoot},
		{"unknown ancestor", "FOO_BAR", "COMPOSITION", semcheck.RoleRoot},
		{"unknown beats never", "FOO_BAR", "DV_TEXT", semcheck.RoleRoot},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if v := rel.CanContain(tc.ancestor, tc.descendant); v != contain.Never && v != contain.UnknownClass {
				t.Fatalf("premise broken: CanContain(%s, %s) = %v, want Never or UnknownClass",
					tc.ancestor, tc.descendant, v)
			}
			a := ck.Operand(tc.ancestor, tc.ancestorRole)
			d := ck.Operand(tc.descendant, semcheck.RoleContained)
			if got := codesOf(ck.Pair(a, d)); len(got) != 0 {
				t.Errorf("Pair(%s, %s) = %v, want no pair finding (the operand code already reports it)",
					tc.ancestor, tc.descendant, got)
			}
		})
	}
}

// TestArchetypeDecision pins REQ-161's archetype/class-conformance check
// (Checker.Archetype): the class/HRID pairs are the exact rows
// openehr/aql/contain's TestArchetypeMatches already pins (REQ-160 §
// Archetype/class conformance), so this test asserts the CLASSIFICATION —
// which code, at which severity — never re-derives the RM fact.
func TestArchetypeDecision(t *testing.T) {
	t.Parallel()
	ck := semcheck.New(nil)
	cases := []struct {
		name   string
		rmType string
		hrid   string
		want   []string
	}{
		{"entity conforms to declared", "ENTRY", "openEHR-EHR-OBSERVATION.blood_pressure.v1", nil},
		{"entity equals declared", "OBSERVATION", "openEHR-EHR-OBSERVATION.blood_pressure.v1", nil},
		{"genuine mismatch", "EVALUATION", "openEHR-EHR-OBSERVATION.blood_pressure.v1", []string{semcheck.CodeArchetypeClassMismatch}},
		{"unknown type segment", "ENTRY", "openEHR-EHR-FOOTYPE.x.v1", []string{semcheck.CodeUnknownRMClass}},
		{"unparseable hrid", "ENTRY", "not-a-valid-archetype-id", []string{semcheck.CodeUnknownRMClass}},
		{"unknown declared class suppresses", "FOO_BAR", "openEHR-EHR-OBSERVATION.x.v1", nil},
		{"case-insensitive declared", "entry", "openEHR-EHR-OBSERVATION.x.v1", nil},
		{"empty rmType names nothing", "", "openEHR-EHR-OBSERVATION.blood_pressure.v1", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := codesOf(ck.Archetype(tc.rmType, tc.hrid)); !slices.Equal(got, tc.want) {
				t.Errorf("Archetype(%q, %q) = %v, want %v", tc.rmType, tc.hrid, got, tc.want)
			}
		})
	}
}

// TestArchetypeSuppressedByUnknownDeclaredClass is the REQ-161 § Checks
// suppression rule in engine terms: when the declared class is itself
// UnknownClass, [Checker.Operand]'s class-token arm already raises
// aql_unknown_rm_class for the SAME class expression, so [Checker.Archetype]
// MUST raise nothing — not the mismatch Error, and not a second
// aql_unknown_rm_class. Combining the two calls' findings, as an adapter
// would, must total exactly ONE finding.
func TestArchetypeSuppressedByUnknownDeclaredClass(t *testing.T) {
	t.Parallel()
	ck := semcheck.New(nil)
	const rmType, hrid = "FOO_BAR", "openEHR-EHR-OBSERVATION.blood_pressure.v1"

	// Premise: the relation really does not know FOO_BAR, and ArchetypeMatches
	// on its own (ignoring the suppression) would still answer UnknownClass —
	// otherwise this test would prove nothing.
	if v := contain.Default().ArchetypeMatches(rmType, hrid); v != contain.UnknownClass {
		t.Fatalf("premise broken: ArchetypeMatches(%s, %s) = %v, want UnknownClass", rmType, hrid, v)
	}

	tokenFindings := ck.Operand(rmType, semcheck.RoleContained).Findings()
	archetypeFindings := ck.Archetype(rmType, hrid)
	all := append(append([]contain.Finding{}, tokenFindings...), archetypeFindings...)
	if got := codesOf(all); !slices.Equal(got, []string{semcheck.CodeUnknownRMClass}) {
		t.Errorf("combined findings = %v, want exactly one %s (the class-token arm's)", got, semcheck.CodeUnknownRMClass)
	}
}

// TestArchetypeUnknownSegmentDoesNotDuplicateTheClassToken is the OTHER half
// of the suppression rule: when the declared class IS known (so the
// class-token arm reports nothing), an unknown HRID type segment still gets
// exactly one aql_unknown_rm_class — the "second arm" — never zero and never
// two.
func TestArchetypeUnknownSegmentDoesNotDuplicateTheClassToken(t *testing.T) {
	t.Parallel()
	ck := semcheck.New(nil)
	const rmType, hrid = "ENTRY", "openEHR-EHR-FOOTYPE.x.v1"

	tokenFindings := ck.Operand(rmType, semcheck.RoleRoot).Findings()
	if len(tokenFindings) != 0 {
		t.Fatalf("premise broken: class-token arm reported %v for known class %s", codesOf(tokenFindings), rmType)
	}
	archetypeFindings := ck.Archetype(rmType, hrid)
	if got := codesOf(archetypeFindings); !slices.Equal(got, []string{semcheck.CodeUnknownRMClass}) {
		t.Errorf("Archetype(%s, %s) = %v, want exactly one %s", rmType, hrid, got, semcheck.CodeUnknownRMClass)
	}
}

// TestZeroValuesAreUsable pins the fail-closed shapes (REQ-025: library code
// does not panic on caller input). The zero Checker must behave as New(nil),
// and the zero Operand — how an adapter spells "no enclosing parent" for a
// junction at the FROM root — must suppress every pair.
func TestZeroValuesAreUsable(t *testing.T) {
	t.Parallel()

	var zero semcheck.Checker
	nilRel := semcheck.New(nil)
	for _, rmType := range []string{"COMPOSITION", "DV_TEXT", "FOO_BAR", ""} {
		a := zero.Operand(rmType, semcheck.RoleContained)
		b := nilRel.Operand(rmType, semcheck.RoleContained)
		if !slices.Equal(codesOf(a.Findings()), codesOf(b.Findings())) || a.Verdict() != b.Verdict() {
			t.Errorf("zero Checker disagrees with New(nil) on %q: %v/%v vs %v/%v",
				rmType, codesOf(a.Findings()), a.Verdict(), codesOf(b.Findings()), b.Verdict())
		}
	}

	var noParent semcheck.Operand
	if len(noParent.Findings()) != 0 {
		t.Errorf("zero Operand reported %v; it names nothing", codesOf(noParent.Findings()))
	}
	if !noParent.Suppresses() {
		t.Error("zero Operand does not suppress; an adapter uses it for 'no enclosing parent'")
	}
	// The descendant here would raise aql_impossible_containment against a real
	// ancestor; with no parent there is no pair to ask about.
	if got := zero.Pair(noParent, zero.Operand("COMPOSITION", semcheck.RoleContained)); len(got) != 0 {
		t.Errorf("Pair(zero, COMPOSITION) = %v, want nothing", codesOf(got))
	}
}

// TestOverlayRelationRetiresFindings is the dialect case (REQ-160
// § Extensibility, REQ-161 § Relation supply): a consumer whose deployment
// really does nest FOO_BAR over COMPOSITION passes a relation carrying that
// edge and gets no finding — where the default relation reports the class
// unknown.
func TestOverlayRelationRetiresFindings(t *testing.T) {
	t.Parallel()

	def := semcheck.New(nil)
	if got := codesOf(def.Operand("FOO_BAR", semcheck.RoleContained).Findings()); !slices.Equal(got, []string{semcheck.CodeUnknownRMClass}) {
		t.Fatalf("premise broken: default relation reports %v for FOO_BAR, want aql_unknown_rm_class", got)
	}

	ck := semcheck.New(contain.Default().WithOverlay(contain.Edge{From: "FOO_BAR", To: "COMPOSITION"}))
	a := ck.Operand("FOO_BAR", semcheck.RoleRoot)
	if got := codesOf(a.Findings()); len(got) != 0 {
		t.Errorf("overlay endpoint FOO_BAR still reports %v", got)
	}
	if a.Suppresses() {
		t.Error("overlay endpoint FOO_BAR still suppresses; an overlay-named class is containable")
	}
	d := ck.Operand("COMPOSITION", semcheck.RoleContained)
	if got := codesOf(ck.Pair(a, d)); len(got) != 0 {
		t.Errorf("Pair(FOO_BAR, COMPOSITION) over the overlay = %v, want nothing", got)
	}
}

// TestFindingsCarryDetail pins that every finding carries the value-bearing
// Detail the REQ-109 issue model expects, naming the class(es) it concerns —
// an empty Detail would leave a consumer with a bare code.
func TestFindingsCarryDetail(t *testing.T) {
	t.Parallel()
	ck := semcheck.New(nil)
	obs := ck.Operand("OBSERVATION", semcheck.RoleRoot)
	comp := ck.Operand("COMPOSITION", semcheck.RoleContained)

	groups := [][]contain.Finding{
		ck.Operand("FOO_BAR", semcheck.RoleContained).Findings(),
		ck.Operand("DV_TEXT", semcheck.RoleContained).Findings(),
		ck.Pair(obs, comp),
		ck.Pair(ck.Operand("FOLDER", semcheck.RoleRoot), comp),
		ck.Archetype("EVALUATION", "openEHR-EHR-OBSERVATION.blood_pressure.v1"),
		ck.Archetype("ENTRY", "openEHR-EHR-FOOTYPE.x.v1"),
	}
	for _, fs := range groups {
		if len(fs) == 0 {
			t.Fatal("a group produced no finding; the premise of this test is broken")
		}
		for _, f := range fs {
			if f.Detail == "" {
				t.Errorf("%s carries an empty Detail", f.Code)
			}
			if _, ok := semcheck.SeverityOf(f.Code); !ok {
				t.Errorf("finding code %q is not in the severity catalogue", f.Code)
			}
		}
	}
}
