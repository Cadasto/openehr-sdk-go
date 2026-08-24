package lint_test

// semantic_test.go: the REQ-161 Layer-2 semantic containment checks — the
// read-side adapter over the REQ-160 relation.
//
// The class pairs come from REQ-160's acceptance table (already pinned by
// openehr/aql/contain's own tests) rather than being invented here, so these
// tests assert the ADAPTER — extraction, adjacency, suppression, spans,
// severities — and never re-assert an RM fact: `OBSERVATION CONTAINS
// COMPOSITION` is Never, `FOLDER CONTAINS COMPOSITION` is ByReference,
// `COMPOSITION CONTAINS ELEMENT` is Admissible, `DV_TEXT` is a
// Never-containability operand, `FOO_BAR` is UnknownClass.

import (
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aql/contain"
	"github.com/cadasto/openehr-sdk-go/openehr/aql/lint"
	"github.com/cadasto/openehr-sdk-go/openehr/aql/parse"
)

// the REQ-161 containment codes, spelled here independently of the
// implementation's constants: a test that imported the code strings from the
// package under test would pass through a rename that broke every consumer.
const (
	codeImpossible    = "aql_impossible_containment"
	codeNotContain    = "aql_contains_not_containable"
	codeUnknownClass  = "aql_unknown_rm_class"
	codeByReference   = "aql_containment_by_reference"
	codeMismatch      = "aql_archetype_class_mismatch"
	nonSemanticSample = "aql_from_archetype" // a pre-existing code, for additivity checks
)

func semanticCodes() []string {
	return []string{codeImpossible, codeNotContain, codeUnknownClass, codeByReference, codeMismatch}
}

// req161 filters a result down to the REQ-161 containment codes, in issue
// order, so a case can assert what the new group did without restating what
// Layer 2 already reported.
func req161(r lint.Result) []string {
	var out []string
	for _, i := range r.Issues {
		if slices.Contains(semanticCodes(), i.Code) {
			out = append(out, i.Code)
		}
	}
	return out
}

// classSpan is the span the nth (1-based) occurrence of an RM class token
// occupies in a single-line query — what [lint.Issue.Span] must carry for an
// issue about that class expression. Computed from the source text, not from
// the implementation, so a span that silently widened to the whole clause fails.
func classSpan(t *testing.T, query, rmType string, nth int) lint.Span {
	t.Helper()
	if strings.Contains(query, "\n") {
		t.Fatalf("classSpan assumes a single-line query, got %q", query)
	}
	if nth < 1 {
		t.Fatalf("classSpan: nth is 1-based, got %d", nth)
	}
	idx, from := 0, 0
	for range nth {
		i := strings.Index(query[from:], rmType)
		if i < 0 {
			t.Fatalf("query %q has fewer than %d occurrences of %q", query, nth, rmType)
		}
		idx = from + i
		from = idx + len(rmType)
	}
	col := len([]rune(query[:idx])) + 1
	return lint.Span{
		Start: parse.Position{Line: 1, Col: col},
		End:   parse.Position{Line: 1, Col: col + len([]rune(rmType))},
	}
}

// --- the four codes ----------------------------------------------------------

// TestSemanticCodesFire is REQ-161 § Checks, one row per code: the code fires
// on a query built to carry exactly that defect, at that severity, with the
// Span on the offending class expression and no OTHER semantic code alongside.
func TestSemanticCodesFire(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		query     string
		code      string
		sev       lint.Severity
		spanClass string
		spanNth   int
		path      string
	}{
		{
			name:  "impossible containment",
			query: "SELECT o FROM OBSERVATION o CONTAINS COMPOSITION c",
			code:  codeImpossible, sev: lint.Error,
			spanClass: "COMPOSITION", spanNth: 1, path: "COMPOSITION c",
		},
		{
			name:  "non-containable target",
			query: "SELECT c FROM COMPOSITION c CONTAINS DV_TEXT t",
			code:  codeNotContain, sev: lint.Error,
			spanClass: "DV_TEXT", spanNth: 1, path: "DV_TEXT t",
		},
		{
			name:  "unknown RM class",
			query: "SELECT c FROM COMPOSITION c CONTAINS FOO_BAR f",
			code:  codeUnknownClass, sev: lint.Warning,
			spanClass: "FOO_BAR", spanNth: 1, path: "FOO_BAR f",
		},
		{
			name:  "containment by reference",
			query: "SELECT c FROM FOLDER f CONTAINS COMPOSITION c",
			code:  codeByReference, sev: lint.Warning,
			spanClass: "COMPOSITION", spanNth: 1, path: "COMPOSITION c",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := lint.LintString(tc.query, nil)
			if got := req161(r); !slices.Equal(got, []string{tc.code}) {
				t.Fatalf("semantic codes = %v, want exactly [%s] (full result %v)", got, tc.code, codes(r))
			}
			iss := r.Issues[slices.IndexFunc(r.Issues, func(i lint.Issue) bool { return i.Code == tc.code })]
			if iss.Severity != tc.sev {
				t.Errorf("%s severity = %v, want %v", tc.code, iss.Severity, tc.sev)
			}
			if want := classSpan(t, tc.query, tc.spanClass, tc.spanNth); iss.Span != want {
				t.Errorf("%s span = %+v, want %+v (on the class expression)", tc.code, iss.Span, want)
			}
			if iss.Path != tc.path {
				t.Errorf("%s path = %q, want %q", tc.code, iss.Path, tc.path)
			}
			if iss.Detail == "" {
				t.Errorf("%s carries no Detail", tc.code)
			}
			// Severity drives Result.OK(): the two Errors must make a result
			// not-OK, the two Warnings must not (REQ-109 § Result.OK).
			if wantOK := tc.sev == lint.Warning; r.OK() != wantOK {
				t.Errorf("%s: Result.OK() = %v, want %v", tc.code, r.OK(), wantOK)
			}
		})
	}
}

// TestSemanticNearMissesStaySilent is the other half of each row: an
// RM-admissible query draws no semantic finding. Conservatism is only worth
// anything if the checks are actually quiet on valid AQL (REQ-161 § Flagging
// policy).
func TestSemanticNearMissesStaySilent(t *testing.T) {
	t.Parallel()
	queries := map[string]string{
		"near miss for impossible":      "SELECT c FROM COMPOSITION c CONTAINS OBSERVATION o",
		"near miss for not-containable": "SELECT c FROM COMPOSITION c CONTAINS ELEMENT e",
		"near miss for unknown":         "SELECT c FROM COMPOSITION c CONTAINS CLUSTER cl",
		"near miss for by-reference":    "SELECT f FROM FOLDER f CONTAINS FOLDER f2",
		"deep admissible chain":         "SELECT e FROM EHR e CONTAINS COMPOSITION c CONTAINS OBSERVATION o CONTAINS CLUSTER cl",
		"self-nesting section":          "SELECT s FROM SECTION s CONTAINS SECTION s2",
		"instruction activity":          "SELECT i FROM INSTRUCTION i CONTAINS ACTIVITY a",
		"version tier":                  "SELECT v FROM VERSION v[all_versions] CONTAINS COMPOSITION c",
		"no containment at all":         "SELECT c FROM COMPOSITION c",
		"admissible NOT CONTAINS":       "SELECT c FROM EHR e CONTAINS COMPOSITION c NOT CONTAINS OBSERVATION o",
	}
	for name, q := range queries {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := req161(lint.LintString(q, nil)); len(got) != 0 {
				t.Errorf("%q raised %v, want no semantic finding", q, got)
			}
		})
	}
}

// --- archetype/class conformance (REQ-161) -----------------------------------
//
// The class/HRID pairs below are the exact rows openehr/aql/contain's
// TestArchetypeMatches already pins (REQ-160 § Archetype/class conformance),
// not invented here — these tests assert the ADAPTER: extraction of the
// literal predicate, the $param skip, the span/path/severity, and the
// once-per-class-expression suppression.

// TestArchetypeClassMismatchFires is task-brief test 2: a literal archetype
// predicate whose HRID entity does not conform to the declared class raises
// aql_archetype_class_mismatch, Error, with the Span on the offending class
// expression — exactly like the other four REQ-161 codes.
func TestArchetypeClassMismatchFires(t *testing.T) {
	t.Parallel()
	const q = "SELECT ev FROM EVALUATION ev[openEHR-EHR-OBSERVATION.blood_pressure.v1]"
	r := lint.LintString(q, nil)
	if got := req161(r); !slices.Equal(got, []string{codeMismatch}) {
		t.Fatalf("semantic codes = %v, want exactly [%s] (full result %v)", got, codeMismatch, codes(r))
	}
	iss := r.Issues[slices.IndexFunc(r.Issues, func(i lint.Issue) bool { return i.Code == codeMismatch })]
	if iss.Severity != lint.Error {
		t.Errorf("severity = %v, want Error", iss.Severity)
	}
	if want := classSpan(t, q, "EVALUATION", 1); iss.Span != want {
		t.Errorf("span = %+v, want %+v (on the declared class, not the archetype text)", iss.Span, want)
	}
	if iss.Path != "EVALUATION ev" {
		t.Errorf("path = %q, want %q", iss.Path, "EVALUATION ev")
	}
	if iss.Detail == "" {
		t.Error("carries no Detail")
	}
	if r.OK() {
		t.Error("an Error-severity mismatch must make the result not-OK")
	}
}

// TestArchetypeConformsStaysSilent is task-brief test 1: a literal archetype
// predicate whose HRID entity conforms to the declared class draws no
// semantic finding, at either end of REQ-160's acceptance row (ENTRY is the
// archetypeable ancestor, OBSERVATION is the entity itself).
func TestArchetypeConformsStaysSilent(t *testing.T) {
	t.Parallel()
	queries := []string{
		"SELECT o FROM ENTRY o[openEHR-EHR-OBSERVATION.blood_pressure.v1]",
		"SELECT o FROM OBSERVATION o[openEHR-EHR-OBSERVATION.blood_pressure.v1]",
	}
	for _, q := range queries {
		t.Run(q, func(t *testing.T) {
			t.Parallel()
			if got := req161(lint.LintString(q, nil)); len(got) != 0 {
				t.Errorf("%q raised %v, want silence", q, got)
			}
		})
	}
}

// TestArchetypeUnknownTypeSegmentWarnsNotMismatch is task-brief test 3: an
// unknown HRID type segment on a KNOWN declared class raises
// aql_unknown_rm_class (the second arm of that code) and never the mismatch
// Error — an unknown name is never wrong (REQ-161 § Flagging policy).
func TestArchetypeUnknownTypeSegmentWarnsNotMismatch(t *testing.T) {
	t.Parallel()
	const q = "SELECT o FROM ENTRY o[openEHR-EHR-FOOTYPE.x.v1]"
	got := req161(lint.LintString(q, nil))
	if !slices.Equal(got, []string{codeUnknownClass}) {
		t.Fatalf("semantic codes = %v, want exactly [%s]", got, codeUnknownClass)
	}
	if slices.Contains(got, codeMismatch) {
		t.Errorf("%s fired alongside an unknown type segment; it must be absent", codeMismatch)
	}
}

// TestArchetypeUnknownDeclaredClassReportsOnce is task-brief test 4: an
// unknown declared class carrying a literal archetype predicate reports
// aql_unknown_rm_class exactly ONCE for the class expression — the
// class-token arm's finding, not a second one from the archetype arm — and
// never the mismatch Error.
func TestArchetypeUnknownDeclaredClassReportsOnce(t *testing.T) {
	t.Parallel()
	const q = "SELECT f FROM FOO_BAR f[openEHR-EHR-OBSERVATION.blood_pressure.v1]"
	r := lint.LintString(q, nil)
	if got := count(r, codeUnknownClass); got != 1 {
		t.Errorf("%s fired %d times, want exactly 1; full result %v", codeUnknownClass, got, codes(r))
	}
	if has(r, codeMismatch) {
		t.Errorf("%s fired for an unknown declared class; it must not", codeMismatch)
	}
}

// TestArchetypeUnparseableHRIDStaysBelowError is task-brief test 5: a literal
// archetype predicate that lexes as an ARCHETYPE_HRID token (so it reaches
// [parse.ClassExpr.Archetype] as a literal, not a $param) but fails
// rm.ParseArchetypeID's stricter validation is treated as unknown, never as a
// proven mismatch. A minor version carrying an embedded dot (".v1.2") is such
// a case: VERSION_ID itself allows dots (resources/aql/grammar/active/AqlLexer.g4),
// but that shifts where rm.ParseArchetypeID's first/last-dot split lands, so
// its version segment no longer starts with "v" (REQ-120) and it reports
// ErrMalformedID — exactly the "(HRID unparseable)" row of the decision
// table, verified empirically against the parser rather than assumed.
func TestArchetypeUnparseableHRIDStaysBelowError(t *testing.T) {
	t.Parallel()
	const q = "SELECT o FROM ENTRY o[openEHR-EHR-OBSERVATION.blood_pressure.v1.2]"
	r := lint.LintString(q, nil)
	if got := req161(r); !slices.Equal(got, []string{codeUnknownClass}) {
		t.Fatalf("semantic codes = %v, want exactly [%s] (an unparseable HRID is treated as unknown, never a proven Error)",
			got, codeUnknownClass)
	}
	if !r.OK() {
		t.Error("a Warning-severity finding must not make the result not-OK")
	}
}

// TestArchetypeParamPredicateIsSkipped is task-brief test 6: a $param
// archetype predicate is skipped entirely — the CDR resolves the bound scope
// at execution (PROBE-021), the same skip openehr/aql/lint/resolve.go applies
// to aql_path_not_in_template — even when the declared class would otherwise
// be checked.
func TestArchetypeParamPredicateIsSkipped(t *testing.T) {
	t.Parallel()
	const q = "SELECT ev FROM EVALUATION ev[$arch]"
	if got := req161(lint.LintString(q, nil)); len(got) != 0 {
		t.Errorf("%q raised %v, want silence ($param is skipped)", q, got)
	}
}

// --- the suppression rule ----------------------------------------------------

// TestSuppressionRule is REQ-161 § Checks' suppression paragraph: an operand
// whose verdict is UnknownClass, or whose containability is Never, is reported
// ONCE through its own code and NO pair code is built on it.
//
// Each row asserts the exact semantic multiset, so a pair code leaking through
// fails — which is the failure mode the rule exists to prevent, since
// [contain.Relation.CanContain] is total and would answer such a pair Never or
// UnknownClass on its own.
//
// The middle-operand rows carry a second job: `OBSERVATION CONTAINS <suppressed>
// CONTAINS COMPOSITION` must stay at ONE finding even though
// OBSERVATION→COMPOSITION is itself Never — reachability composes, so only
// ADJACENT pairs are asked and no transitive pair is synthesised.
func TestSuppressionRule(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		query string
		want  []string
	}{
		{
			"unknown operand yields only the warning",
			"SELECT c FROM COMPOSITION c CONTAINS FOO_BAR f",
			[]string{codeUnknownClass},
		},
		{
			"unknown ancestor yields only the warning",
			"SELECT c FROM FOO_BAR f CONTAINS COMPOSITION c CONTAINS OBSERVATION o",
			[]string{codeUnknownClass},
		},
		{
			"unknown in the middle suppresses both pairs",
			"SELECT c FROM OBSERVATION o CONTAINS FOO_BAR f CONTAINS COMPOSITION c",
			[]string{codeUnknownClass},
		},
		{
			"data value operand yields only the error",
			"SELECT c FROM COMPOSITION c CONTAINS DV_TEXT t",
			[]string{codeNotContain},
		},
		{
			"data value in the middle suppresses both pairs",
			"SELECT c FROM OBSERVATION o CONTAINS DV_TEXT t CONTAINS COMPOSITION c",
			[]string{codeNotContain},
		},
		{
			// The FROM root is not a CONTAINS operand, so the catalogue
			// authorises no code for it (REQ-161 § Checks) — but it still
			// suppresses, so no Error is built on it either.
			"non-containable FROM root suppresses without a code",
			"SELECT t FROM DV_TEXT t CONTAINS ELEMENT e",
			nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := req161(lint.LintString(tc.query, nil)); !slices.Equal(got, tc.want) {
				t.Errorf("%q semantic codes = %v, want %v", tc.query, got, tc.want)
			}
		})
	}
}

// TestRootPositionIsSilentInEverySpelling pins the controller ruling on the
// role axis: the operand role is assigned by POSITION IN THE QUERY, never by
// shape in the containment tree.
//
// AQL spells a FROM root two ways — a single root (`FROM X x`) and a boolean
// junction at the root (`FROM X x OR Y y`) — and the parse tree represents them
// differently: the first lands on FromClause.Root, the second on
// FromClause.Junction, whose operands are junction CHILDREN. Labelling those
// children "contained" because of where they sit made the two spellings answer
// differently for the same query position, which is the bug these rows exist to
// keep fixed. Root positions are silent for aql_contains_not_containable in
// every spelling; the code is authorised only for a CONTAINS operand (REQ-161
// § Checks), and a false Error is worse than a missed defect (§ Flagging
// policy). The engine-side rule is [semcheck.Role.Next].
func TestRootPositionIsSilentInEverySpelling(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		query string
		want  []string
	}{
		{
			"single root",
			"SELECT t FROM DV_TEXT t",
			nil,
		},
		{
			"single root with a CONTAINS below it",
			"SELECT t FROM DV_TEXT t CONTAINS ELEMENT e",
			nil,
		},
		{
			"root-junction operand is a root, not a contained operand",
			"SELECT c FROM DV_TEXT t OR COMPOSITION c",
			nil,
		},
		{
			"root-junction operand in either arm order",
			"SELECT c FROM COMPOSITION c OR DV_TEXT t",
			nil,
		},
		{
			// Mixed AND/OR at the root does NOT flatten, so this operand is a
			// junction child of a junction child: the root role has to survive
			// two levels of inheritance.
			"nested root junction still holds roots",
			"SELECT c1 FROM COMPOSITION c1 OR (DV_TEXT t AND COMPOSITION c2)",
			nil,
		},
		{
			// The scope of the silence, in ONE query: the root-junction operand
			// DV_TEXT stays silent while DV_QUANTITY — a genuine CONTAINS
			// operand under the other arm — still raises the Error. Before the
			// role was assigned by position this query reported the code twice.
			"a root operand and a contained operand in one query",
			"SELECT c FROM DV_TEXT t OR COMPOSITION c CONTAINS DV_QUANTITY q",
			[]string{codeNotContain},
		},
		{
			// The counterpart: the same class as a CONTAINS operand IS an Error,
			// so these rows pin a scoped silence rather than a blanket one.
			"the same class under a CONTAINS is an Error",
			"SELECT c FROM COMPOSITION c CONTAINS DV_TEXT t",
			[]string{codeNotContain},
		},
		{
			// An UNKNOWN root is covered in every spelling either way, because
			// aql_unknown_rm_class fires on any class token in any position —
			// so the silence above is narrowly the known-but-non-containable
			// root, never an unreported unknown name.
			"an unknown root-junction operand still warns",
			"SELECT c1 FROM COMPOSITION c1 OR FOO_BAR f",
			[]string{codeUnknownClass},
		},
		{
			"an unknown single root still warns",
			"SELECT f FROM FOO_BAR f",
			[]string{codeUnknownClass},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := req161(lint.LintString(tc.query, nil)); !slices.Equal(got, tc.want) {
				t.Errorf("%q semantic codes = %v, want %v", tc.query, got, tc.want)
			}
		})
	}
}

// TestUnknownClassFiresOncePerOccurrence pins the "once per class-expression
// occurrence" wording (REQ-161 § Checks): two occurrences of the same unknown
// class are two findings, because each is a separate class expression a reader
// has to look at.
func TestUnknownClassFiresOncePerOccurrence(t *testing.T) {
	t.Parallel()
	const q = "SELECT c FROM COMPOSITION c CONTAINS (FOO_BAR f1 OR FOO_BAR f2)"
	r := lint.LintString(q, nil)
	if got := count(r, codeUnknownClass); got != 2 {
		t.Fatalf("%s fired %d times, want 2 (one per class expression); full result %v",
			codeUnknownClass, got, codes(r))
	}
	var spans []lint.Span
	for _, i := range r.Issues {
		if i.Code == codeUnknownClass {
			spans = append(spans, i.Span)
		}
	}
	if spans[0] == spans[1] {
		t.Errorf("both occurrences report span %+v; each MUST point at its own class expression", spans[0])
	}
	if want := classSpan(t, q, "FOO_BAR", 2); spans[1] != want {
		t.Errorf("second occurrence span = %+v, want %+v", spans[1], want)
	}
}

// --- NOT CONTAINS ------------------------------------------------------------

// TestNotContainsIsCheckedIdentically is REQ-161 § Checks: a `NOT CONTAINS`
// pair is checked exactly like a plain CONTAINS pair, because an impossible
// exclusion is trivially true — a dead constraint, and equally a defect.
func TestNotContainsIsCheckedIdentically(t *testing.T) {
	t.Parallel()
	const (
		plain    = "SELECT o FROM OBSERVATION o CONTAINS COMPOSITION c"
		negated  = "SELECT o FROM OBSERVATION o NOT CONTAINS COMPOSITION c"
		negDeep  = "SELECT c FROM EHR e CONTAINS COMPOSITION c NOT CONTAINS DV_TEXT t"
		negValid = "SELECT c FROM EHR e CONTAINS COMPOSITION c NOT CONTAINS OBSERVATION o"
	)
	if got := req161(lint.LintString(negated, nil)); !slices.Equal(got, []string{codeImpossible}) {
		t.Errorf("NOT CONTAINS over an impossible pair = %v, want [%s]", got, codeImpossible)
	}
	if a, b := req161(lint.LintString(plain, nil)), req161(lint.LintString(negated, nil)); !slices.Equal(a, b) {
		t.Errorf("CONTAINS reported %v but NOT CONTAINS reported %v; the two MUST be checked identically", a, b)
	}
	if got := req161(lint.LintString(negDeep, nil)); !slices.Equal(got, []string{codeNotContain}) {
		t.Errorf("NOT CONTAINS over a non-containable operand = %v, want [%s]", got, codeNotContain)
	}
	if got := req161(lint.LintString(negValid, nil)); len(got) != 0 {
		t.Errorf("NOT CONTAINS over an admissible pair = %v, want silence", got)
	}
}

// --- junctions ---------------------------------------------------------------

// TestJunctionOperandsUseEnclosingParent pins the junction rule (REQ-161
// § Checks): a junction operand is checked against the junction's ENCLOSING
// PARENT, never against the junction node — which carries no class of its own
// and no source position.
//
// [parse.Containment] has no parent pointer, so the parent has to be threaded
// down the recursion; these rows are what fails if it stops being. The mixed
// AND/OR row is the load-bearing one: same-operator junctions arrive
// pre-flattened (REQ-117) but mixed nesting does NOT, so the inner operands
// must still resolve to the outer COMPOSITION rather than to the AND node
// between them.
func TestJunctionOperandsUseEnclosingParent(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		query string
		want  []string
	}{
		{
			"one impossible operand of an OR",
			"SELECT o FROM OBSERVATION o CONTAINS (COMPOSITION c OR ELEMENT e)",
			[]string{codeImpossible},
		},
		{
			"pre-flattened same-operator junction, one finding per operand",
			"SELECT o FROM OBSERVATION o CONTAINS (COMPOSITION c1 OR COMPOSITION c2 OR ELEMENT e)",
			[]string{codeImpossible, codeImpossible},
		},
		{
			"mixed AND/OR nesting still resolves to the outer parent",
			"SELECT c FROM COMPOSITION c CONTAINS (OBSERVATION o AND (COMPOSITION c2 OR ELEMENT e))",
			[]string{codeImpossible},
		},
		{
			"admissible junction stays silent",
			"SELECT c FROM COMPOSITION c CONTAINS (OBSERVATION o OR EVALUATION ev)",
			nil,
		},
		{
			// A junction AT the FROM root has no enclosing parent, so there is
			// no pair to ask about — only the operand checks apply.
			"FROM-root junction forms no pair",
			"SELECT c1 FROM COMPOSITION c1 OR OBSERVATION o1",
			nil,
		},
		{
			"FROM-root junction still reports an unknown operand",
			"SELECT c1 FROM COMPOSITION c1 OR FOO_BAR f",
			[]string{codeUnknownClass},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := req161(lint.LintString(tc.query, nil)); !slices.Equal(got, tc.want) {
				t.Errorf("%q semantic codes = %v, want %v", tc.query, got, tc.want)
			}
		})
	}
}

// TestJunctionOperandSpanNamesTheOperand pins that the span lands on the
// offending OPERAND, not on the junction: a junction node carries no position
// at all, so an implementation that reported against it would emit the zero
// Span and lose the diagnostic's location.
func TestJunctionOperandSpanNamesTheOperand(t *testing.T) {
	t.Parallel()
	const q = "SELECT o FROM OBSERVATION o CONTAINS (COMPOSITION c OR ELEMENT e)"
	for _, i := range lint.LintString(q, nil).Issues {
		if i.Code != codeImpossible {
			continue
		}
		if want := classSpan(t, q, "COMPOSITION", 1); i.Span != want {
			t.Errorf("span = %+v, want %+v (the junction operand)", i.Span, want)
		}
		if i.Span.IsZero() {
			t.Error("span is zero; the operand has a position even though the junction does not")
		}
		return
	}
	t.Fatalf("%s did not fire on %q", codeImpossible, q)
}

// --- Options.Relation --------------------------------------------------------

// TestNilRelationIsTheDefaultRelation pins REQ-161 § Relation supply: a nil
// Options.Relation means the REQ-160 default relation — nil DISABLES nothing,
// unlike Options.Compiled and Options.Query.
func TestNilRelationIsTheDefaultRelation(t *testing.T) {
	t.Parallel()
	queries := []string{
		"SELECT o FROM OBSERVATION o CONTAINS COMPOSITION c",
		"SELECT c FROM FOLDER f CONTAINS COMPOSITION c",
		"SELECT c FROM COMPOSITION c CONTAINS FOO_BAR f",
		"SELECT c FROM COMPOSITION c CONTAINS OBSERVATION o",
	}
	for _, q := range queries {
		t.Run(q, func(t *testing.T) {
			t.Parallel()
			nilOpts := req161(lint.LintString(q, nil))
			zeroOpts := req161(lint.LintString(q, &lint.Options{}))
			explicit := req161(lint.LintString(q, &lint.Options{Relation: contain.Default()}))
			if !slices.Equal(nilOpts, explicit) {
				t.Errorf("nil Options = %v but Relation: contain.Default() = %v", nilOpts, explicit)
			}
			if !slices.Equal(zeroOpts, explicit) {
				t.Errorf("zero Options = %v but Relation: contain.Default() = %v", zeroOpts, explicit)
			}
			if len(nilOpts) == 0 && q == queries[0] {
				t.Error("the semantic group did not run under a nil Options; nil selects the default, it does not disable")
			}
		})
	}
}

// TestOverlayRelationRetiresFindings is the dialect case (REQ-161 § Relation
// supply): a caller whose deployment genuinely admits a containment the pinned
// RM does not lints without a false finding — for an Error verdict and for an
// unknown class alike.
func TestOverlayRelationRetiresFindings(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		query   string
		edge    contain.Edge
		wantDef []string
	}{
		{
			name:    "overlay retires an impossible pair",
			query:   "SELECT o FROM OBSERVATION o CONTAINS COMPOSITION c",
			edge:    contain.Edge{From: "OBSERVATION", To: "COMPOSITION"},
			wantDef: []string{codeImpossible},
		},
		{
			name:    "overlay retires an unknown class",
			query:   "SELECT c FROM FOO_BAR f CONTAINS COMPOSITION c",
			edge:    contain.Edge{From: "FOO_BAR", To: "COMPOSITION"},
			wantDef: []string{codeUnknownClass},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := req161(lint.LintString(tc.query, nil)); !slices.Equal(got, tc.wantDef) {
				t.Fatalf("premise broken: default relation reports %v, want %v", got, tc.wantDef)
			}
			opts := &lint.Options{Relation: contain.Default().WithOverlay(tc.edge)}
			if got := req161(lint.LintString(tc.query, opts)); len(got) != 0 {
				t.Errorf("overlay relation still reports %v", got)
			}
		})
	}
}

// TestOptionsFieldsAreIndependent pins that the new field did not disturb the
// existing ones (REQ-161 § Additivity): setting Relation must not switch on or
// off anything Compiled or Query gate.
func TestOptionsFieldsAreIndependent(t *testing.T) {
	t.Parallel()
	const q = "SELECT c FROM COMPOSITION c WHERE c/name/value = $missing"
	bare := codes(lint.LintString(q, &lint.Options{}))
	withRel := codes(lint.LintString(q, &lint.Options{Relation: contain.Default()}))
	if !slices.Equal(bare, withRel) {
		t.Errorf("Relation changed the non-semantic codes: %v vs %v", bare, withRel)
	}
	if has(lint.LintString(q, &lint.Options{Relation: contain.Default()}), "aql_unbound_param") {
		t.Error("aql_unbound_param fired without Options.Query; Relation must not gate the param checks")
	}
}

// --- additivity --------------------------------------------------------------

// TestSemanticGroupIsAdditive is REQ-161 § Additivity: a query carrying NO
// semantic defect keeps its exact pre-existing issue-code multiset. The
// expectations below are the pre-REQ-161 outputs, written out in full rather
// than diffed, so a new code leaking into a clean query fails here.
func TestSemanticGroupIsAdditive(t *testing.T) {
	t.Parallel()
	cases := []struct {
		query string
		want  []string
	}{
		{
			// PROBE-028's `valid.aql` corpus query.
			"SELECT o/data[at0001]/events[at0006]/data[at0003]/items[at0004]/value/magnitude " +
				"FROM OBSERVATION o[openEHR-EHR-OBSERVATION.blood_pressure.v1]",
			nil,
		},
		{
			// PROBE-028's `missing_archetype.aql` corpus query (Layers 1–2 only).
			"SELECT o FROM OBSERVATION o[openEHR-EHR-OBSERVATION.lab_result.v1]",
			nil,
		},
		{"SELECT c FROM COMPOSITION c", []string{nonSemanticSample}},
		{"SELECT * FROM EHR e", []string{"aql_select_star"}},
		{
			"SELECT TOP 5 c FROM COMPOSITION c LIMIT 10",
			[]string{nonSemanticSample, "aql_deprecated_top", "aql_top_with_limit"},
		},
		{
			"SELECT o FROM EHR e CONTAINS OBSERVATION o[openEHR-EHR-OBSERVATION.blood_pressure.v1]",
			nil,
		},
		{
			"SELECT zz/data[at0001]/magnitude FROM COMPOSITION c",
			[]string{"aql_unknown_alias", nonSemanticSample},
		},
		{"SELECT c FROM COMPOSITION c[$arch]", nil},
		{"SELECT v FROM VERSION v[all_versions]", nil},
	}
	for _, tc := range cases {
		t.Run(tc.query, func(t *testing.T) {
			t.Parallel()
			r := lint.LintString(tc.query, nil)
			got := codes(r)
			if len(got) == 0 {
				got = nil
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("codes = %v, want %v (REQ-161 is additive: a clean query's multiset is unchanged)", got, tc.want)
			}
			for _, c := range semanticCodes() {
				if slices.Contains(got, c) {
					t.Errorf("%s fired on a query with no semantic defect", c)
				}
			}
		})
	}
}

// TestSemanticChecksNeedNoOptions pins that the group runs on the Layer-2
// surface alone — no compiled OPT, no aql.Query, no CDR. A caller that only
// has query text gets the finding.
func TestSemanticChecksNeedNoOptions(t *testing.T) {
	t.Parallel()
	r := lint.LintString("SELECT o FROM OBSERVATION o CONTAINS COMPOSITION c", nil)
	if !has(r, codeImpossible) {
		t.Fatalf("want %s from LintString with nil options, got %v", codeImpossible, codes(r))
	}
	if r.OK() {
		t.Error("an Error-severity semantic finding must make the result not-OK")
	}
}

// TestSemanticChecksSurviveALintOnlyDocument pins the [lint.Lint] entry point
// (a caller that parsed the document itself) reaching the same findings as
// [lint.LintString].
func TestSemanticChecksSurviveALintOnlyDocument(t *testing.T) {
	t.Parallel()
	const q = "SELECT o FROM OBSERVATION o CONTAINS COMPOSITION c"
	viaString := req161(lint.LintString(q, nil))
	viaDoc := req161(lint.Lint(mustParse(t, q), nil))
	if !slices.Equal(viaString, viaDoc) {
		t.Errorf("Lint reported %v but LintString reported %v", viaDoc, viaString)
	}
}
