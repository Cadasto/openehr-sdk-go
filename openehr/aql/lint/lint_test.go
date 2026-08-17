package lint_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/aql"
	"github.com/cadasto/openehr-sdk-go/openehr/aql/lint"
	"github.com/cadasto/openehr-sdk-go/openehr/aql/parse"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

func mustCompile(t *testing.T, fixture string) *templatecompile.Compiled {
	t.Helper()
	opt, err := template.ParseFile(fixtures.TemplateOptForName(fixture))
	if err != nil {
		t.Fatalf("ParseFile(%s): %v", fixture, err)
	}
	c, err := templatecompile.Compile(opt)
	if err != nil {
		t.Fatalf("Compile(%s): %v", fixture, err)
	}
	return c
}

// count reports how many issues carry the given code (a code that must fire
// exactly once needs a count, not just presence).
func count(r lint.Result, code string) int {
	n := 0
	for _, i := range r.Issues {
		if i.Code == code {
			n++
		}
	}
	return n
}

func codes(r lint.Result) []string {
	out := make([]string, len(r.Issues))
	for i, is := range r.Issues {
		out[i] = is.Code
	}
	return out
}

func has(r lint.Result, code string) bool {
	return slices.ContainsFunc(r.Issues, func(i lint.Issue) bool { return i.Code == code })
}

// --- Layer 1: syntax / empty -------------------------------------------------

func TestLintStringEmpty(t *testing.T) {
	r := lint.LintString("   ", nil)
	if r.OK() || !has(r, "aql_empty") {
		t.Fatalf("want aql_empty error, got %v", codes(r))
	}
}

func TestLintStringSyntax(t *testing.T) {
	r := lint.LintString("SELECT FROM EHR e", nil)
	if r.OK() || !has(r, "aql_syntax") {
		t.Fatalf("want aql_syntax error, got %v", codes(r))
	}
	if len(r.Issues) == 0 {
		t.Fatal("expected issues")
	}
	// REQ-109: Detail carries line:column before the ANTLR message.
	if !strings.Contains(r.Issues[0].Detail, "1:") {
		t.Fatalf("Detail missing position: %q", r.Issues[0].Detail)
	}
}

func TestLintUnparsedDocument(t *testing.T) {
	r := lint.Lint(&parse.Document{}, nil)
	if r.OK() || !has(r, "aql_syntax") {
		t.Fatalf("want aql_syntax for unparsed document, got %v", codes(r))
	}
}

// --- Layer 2: shape + params -------------------------------------------------

func TestLintCleanNoTemplate(t *testing.T) {
	r := lint.LintString(
		"SELECT o FROM EHR e CONTAINS OBSERVATION o[openEHR-EHR-OBSERVATION.blood_pressure.v1]",
		nil,
	)
	if !r.OK() {
		t.Fatalf("expected clean, got %v", codes(r))
	}
}

func TestLintUnknownAlias(t *testing.T) {
	// Path rooted at alias "x", but FROM binds "o".
	r := lint.LintString(
		"SELECT x/data FROM OBSERVATION o[openEHR-EHR-OBSERVATION.blood_pressure.v1]",
		nil,
	)
	if r.OK() || !has(r, "aql_unknown_alias") {
		t.Fatalf("want aql_unknown_alias, got %v", codes(r))
	}
}

// TestLintOrderBySelectAlias pins the REQ-117 lint acceptance: an ORDER BY key
// that names no FROM/CONTAINS alias is resolved against the SELECT `AS`
// aliases before aql_unknown_alias is raised. FROM is consulted first, and an
// AS alias labels a projected column — never a path root — so a key carrying a
// path tail stays unknown.
func TestLintOrderBySelectAlias(t *testing.T) {
	const from = " FROM OBSERVATION o[openEHR-EHR-OBSERVATION.blood_pressure.v1]"
	for _, tc := range []struct {
		name        string
		query       string
		wantUnknown bool
	}{
		{
			name:  "as_alias_hit",
			query: "SELECT o/data[at0001]/value/magnitude AS score" + from + " ORDER BY score DESC",
		},
		{
			name:  "as_alias_hit_no_direction",
			query: "SELECT o/data[at0001]/value/magnitude AS score" + from + " ORDER BY score",
		},
		{
			name:        "unknown_identifier",
			query:       "SELECT o/name/value" + from + " ORDER BY nope",
			wantUnknown: true,
		},
		{
			// The SELECT alias reuses a FROM alias: FROM wins, and either
			// resolution order leaves the query clean.
			name:  "select_alias_shadows_from_alias",
			query: "SELECT o/name/value AS o" + from + " ORDER BY o",
		},
		{
			name:        "as_alias_with_path_tail",
			query:       "SELECT o/name/value AS score" + from + " ORDER BY score/magnitude",
			wantUnknown: true,
		},
		{
			// The fallback is scoped to ORDER BY: a SELECT alias is not a
			// WHERE operand root.
			name:        "select_alias_not_visible_in_where",
			query:       "SELECT o/name/value AS score" + from + " WHERE score = 1",
			wantUnknown: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := lint.LintString(tc.query, nil)
			if got := has(r, "aql_unknown_alias"); got != tc.wantUnknown {
				t.Fatalf("aql_unknown_alias = %v, want %v (codes %v)", got, tc.wantUnknown, codes(r))
			}
		})
	}
}

// TestLintSelectAliasDoesNotBindClass pins the namespace separation behind
// "FROM wins" (REQ-117): a SELECT `AS` alias resolves an ORDER BY key but
// never enters the class-binding map, so Layer 3 cannot anchor a path to it.
func TestLintSelectAliasDoesNotBindClass(t *testing.T) {
	md := lint.Extract(mustParse(
		t,
		"SELECT o/name/value AS score FROM OBSERVATION o[openEHR-EHR-OBSERVATION.blood_pressure.v1] "+
			"ORDER BY score",
	))
	if _, ok := md.Aliases["score"]; ok {
		t.Errorf("SELECT alias leaked into the class-binding map: %v", md.Aliases)
	}
	if len(md.SelectAliases) != 1 || md.SelectAliases[0] != "score" {
		t.Errorf("SelectAliases = %v, want [score]", md.SelectAliases)
	}
}

// TestLintBooleanComparisonOperand pins the second REQ-117 lint acceptance: a
// boolean literal as a comparison operand is a literal, not a path. The SDK
// lexer lexes `true` / `false` as IDENTIFIER (the IDENTIFIER rule precedes
// BOOLEAN), so the operand used to reach the alias check as a pseudo-path and
// draw aql_unknown_alias.
func TestLintBooleanComparisonOperand(t *testing.T) {
	const head = "SELECT s/is_queryable FROM EHR e " +
		"CONTAINS COMPOSITION s[openEHR-EHR-COMPOSITION.encounter.v1] WHERE s/is_queryable "
	for _, tc := range []struct {
		name        string
		predicate   string
		wantUnknown bool
	}{
		{name: "true", predicate: "= true"},
		{name: "false", predicate: "= false"},
		{name: "uppercase", predicate: "!= TRUE"},
		{name: "null", predicate: "= null"},
		{name: "parameter", predicate: "= $flag"},
		// A genuine path operand keeps its alias check: `zz` binds nothing.
		{name: "unbound_path_operand", predicate: "= zz/other", wantUnknown: true},
		// …and a bound one stays clean.
		{name: "bound_path_operand", predicate: "= e/ehr_id/value"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := lint.LintString(head+tc.predicate, nil)
			if got := has(r, "aql_unknown_alias"); got != tc.wantUnknown {
				t.Fatalf("aql_unknown_alias = %v, want %v (codes %v)", got, tc.wantUnknown, codes(r))
			}
		})
	}
}

// TestLintSelectBooleanKeywordLiteral extends the REQ-117 boolean-literal
// acceptance to the SELECT column position: `SELECT true` is a projected
// literal, not a path rooted at a pseudo-alias, so it draws no
// aql_unknown_alias. A keyword carrying a path tail is a real path and keeps
// its alias check.
func TestLintSelectBooleanKeywordLiteral(t *testing.T) {
	const from = " FROM EHR e CONTAINS COMPOSITION s[openEHR-EHR-COMPOSITION.encounter.v1]"
	for _, tc := range []struct {
		name        string
		projection  string
		wantUnknown bool
	}{
		{name: "true", projection: "true"},
		{name: "false", projection: "false"},
		{name: "uppercase", projection: "TRUE"},
		{name: "null", projection: "null"},
		{name: "mixed_with_path", projection: "true, s/uid/value"},
		{name: "aliased", projection: "true AS flag"},
		// A keyword with a path tail is a path: `true` binds no class.
		{name: "keyword_path_tail", projection: "true/nested", wantUnknown: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := lint.LintString("SELECT "+tc.projection+from, nil)
			if got := has(r, "aql_unknown_alias"); got != tc.wantUnknown {
				t.Fatalf("aql_unknown_alias = %v, want %v (codes %v)", got, tc.wantUnknown, codes(r))
			}
		})
	}
}

func TestLintFromArchetypeWarning(t *testing.T) {
	r := lint.LintString("SELECT c FROM COMPOSITION c", nil)
	if !has(r, "aql_from_archetype") {
		t.Fatalf("want aql_from_archetype warning, got %v", codes(r))
	}
	if !r.OK() {
		t.Fatalf("warning must not make result not-OK: %v", codes(r))
	}
}

func TestLintUnboundParam(t *testing.T) {
	q := aql.NewQuery(
		"SELECT o FROM EHR e CONTAINS OBSERVATION o[openEHR-EHR-OBSERVATION.blood_pressure.v1] " +
			"WHERE e/ehr_id/value = $ehr_id",
	)
	// No Parameters bound at all.
	doc := mustParse(t, q.Q)
	r := lint.Lint(doc, &lint.Options{Query: &q})
	if r.OK() || !has(r, "aql_unbound_param") {
		t.Fatalf("want aql_unbound_param, got %v", codes(r))
	}
}

func TestLintUnusedParamWarning(t *testing.T) {
	q := aql.NewQuery(
		"SELECT o FROM EHR e CONTAINS OBSERVATION o[openEHR-EHR-OBSERVATION.blood_pressure.v1] " +
			"WHERE e/ehr_id/value = $ehr_id",
	)
	q.Parameters = map[string]any{"ehr_id": "x", "spurious": 1}
	doc := mustParse(t, q.Q)
	r := lint.Lint(doc, &lint.Options{Query: &q})
	if !has(r, "aql_unused_param") {
		t.Fatalf("want aql_unused_param warning, got %v", codes(r))
	}
	if !r.OK() {
		t.Fatalf("unused-param warning must not make result not-OK: %v", codes(r))
	}
}

// --- Layer 3: template -------------------------------------------------------

func TestLintArchetypeNotInTemplate(t *testing.T) {
	c := mustCompile(t, "vital_signs")
	r := lint.LintString(
		"SELECT o FROM OBSERVATION o[openEHR-EHR-OBSERVATION.lab_result.v1]",
		&lint.Options{Compiled: c},
	)
	if r.OK() || !has(r, "aql_archetype_not_in_template") {
		t.Fatalf("want aql_archetype_not_in_template, got %v", codes(r))
	}
}

func TestLintArchetypeInTemplateClean(t *testing.T) {
	c := mustCompile(t, "vital_signs")
	r := lint.LintString(
		"SELECT o FROM OBSERVATION o[openEHR-EHR-OBSERVATION.blood_pressure.v1]",
		&lint.Options{Compiled: c},
	)
	if !r.OK() || len(r.Issues) != 0 {
		t.Fatalf("expected clean, got %v", codes(r))
	}
}

// A real, structurally valid blood-pressure path (ending in the RM leaf
// /value/magnitude) must NOT warn.
func TestLintValidPathNoWarning(t *testing.T) {
	c := mustCompile(t, "vital_signs")
	r := lint.LintString(
		"SELECT o/data[at0001]/events[at0006]/data[at0003]/items[at0004]/value/magnitude "+
			"FROM OBSERVATION o[openEHR-EHR-OBSERVATION.blood_pressure.v1]",
		&lint.Options{Compiled: c},
	)
	if has(r, "aql_path_not_in_template") {
		t.Fatalf("valid path must not warn, got %v", codes(r))
	}
}

// A path with a wrong structural attribute (eventz) must warn, localised to
// the offending segment and path.
func TestLintBadPathWarns(t *testing.T) {
	const rawPath = "o/data[at0001]/eventz/value"
	c := mustCompile(t, "vital_signs")
	r := lint.LintString(
		"SELECT "+rawPath+" FROM OBSERVATION o[openEHR-EHR-OBSERVATION.blood_pressure.v1]",
		&lint.Options{Compiled: c},
	)
	if !has(r, "aql_path_not_in_template") {
		t.Fatalf("want aql_path_not_in_template warning, got %v", codes(r))
	}
	if !r.OK() {
		t.Fatalf("path warning must not make result not-OK: %v", codes(r))
	}
	var found bool
	for _, i := range r.Issues {
		if i.Code != "aql_path_not_in_template" {
			continue
		}
		found = true
		if i.Path != rawPath {
			t.Errorf("issue Path = %q, want %q", i.Path, rawPath)
		}
		if !strings.Contains(i.Detail, "eventz") {
			t.Errorf("Detail should name the diverging segment, got %q", i.Detail)
		}
	}
	if !found {
		t.Fatal("no aql_path_not_in_template issue to inspect")
	}
}

// TestLintWrongAtCodeLenientFallback documents Layer-3 predicate resolution:
// an unknown at-code on a multi-child segment falls back to the first child
// (mirroring template.NodeAt), so no path warning is emitted.
func TestLintWrongAtCodeLenientFallback(t *testing.T) {
	c := mustCompile(t, "vital_signs")
	r := lint.LintString(
		"SELECT o/data[at0001]/events[at9999]/data/items[at0004]/value/magnitude "+
			"FROM OBSERVATION o[openEHR-EHR-OBSERVATION.blood_pressure.v1]",
		&lint.Options{Compiled: c},
	)
	if has(r, "aql_path_not_in_template") {
		t.Fatalf("wrong at-code with first-child fallback must not warn, got %v", codes(r))
	}
}

// --- Review-driven coverage additions ---------------------------------------

// A referenced $param that IS bound produces no aql_unbound_param (the
// negative of TestLintUnboundParam — guards against an inverted condition).
func TestLintBoundParamClean(t *testing.T) {
	q := aql.NewQuery(
		"SELECT o FROM EHR e CONTAINS OBSERVATION o[openEHR-EHR-OBSERVATION.blood_pressure.v1] " +
			"WHERE e/ehr_id/value = $ehr_id",
	)
	q.Parameters = map[string]any{"ehr_id": "x"}
	r := lint.Lint(mustParse(t, q.Q), &lint.Options{Query: &q})
	if has(r, "aql_unbound_param") || has(r, "aql_unused_param") {
		t.Fatalf("bound+used param must be clean, got %v", codes(r))
	}
	if !r.OK() {
		t.Fatalf("expected OK, got %v", codes(r))
	}
}

// Each hasIdentifiableScope disjunct (param-archetype, VERSION) on its
// own suppresses aql_from_archetype — guards the OR-chain against losing a
// disjunct. (Literal archetype and EHR are already covered elsewhere.)
func TestLintIdentifiableScopeSuppressesWarning(t *testing.T) {
	for _, q := range []string{
		"SELECT c FROM COMPOSITION c[$arch]",    // ParamArchetype
		"SELECT v FROM VERSION v[all_versions]", // Version (no EHR)
	} {
		r := lint.LintString(q, nil)
		if has(r, "aql_from_archetype") {
			t.Errorf("%q: must not warn aql_from_archetype, got %v", q, codes(r))
		}
	}
}

// With multiple unused params, the aql_unused_param issues appear in
// deterministic sorted-key order (the sort is a no-op with a single param).
func TestLintUnusedParamsSorted(t *testing.T) {
	q := aql.NewQuery("SELECT o FROM OBSERVATION o[openEHR-EHR-OBSERVATION.blood_pressure.v1]")
	q.Parameters = map[string]any{"zeta": 1, "alpha": 2, "mike": 3}
	r := lint.Lint(mustParse(t, q.Q), &lint.Options{Query: &q})
	var details []string
	for _, i := range r.Issues {
		if i.Code == "aql_unused_param" {
			details = append(details, i.Detail)
		}
	}
	if len(details) != 3 {
		t.Fatalf("want 3 aql_unused_param issues, got %d: %v", len(details), codes(r))
	}
	// Detail embeds the key after a shared prefix, so lexicographic order of
	// Details == sorted-key order.
	if !slices.IsSorted(details) {
		t.Fatalf("aql_unused_param not in sorted order: %v", details)
	}
}

// Lint is collect-all — one query tripping Layer-2 alias, Layer-2
// param, and Layer-3 archetype checks returns all three in a single pass.
func TestLintCollectAll(t *testing.T) {
	c := mustCompile(t, "vital_signs")
	q := aql.NewQuery(
		"SELECT x/foo FROM OBSERVATION o[openEHR-EHR-OBSERVATION.lab_result.v1] " +
			"WHERE o/data = $p",
	)
	r := lint.Lint(mustParse(t, q.Q), &lint.Options{Compiled: c, Query: &q})
	for _, want := range []string{
		"aql_unknown_alias", "aql_unbound_param", "aql_archetype_not_in_template",
	} {
		if !has(r, want) {
			t.Errorf("collect-all missing %s; got %v", want, codes(r))
		}
	}
}

// A nil *Document is guarded (no panic) and yields aql_syntax.
func TestLintNilDocument(t *testing.T) {
	r := lint.Lint(nil, nil)
	if r.OK() || !has(r, "aql_syntax") {
		t.Fatalf("Lint(nil) want aql_syntax, got %v", codes(r))
	}
}

// TestLintDeprecatedTop pins the advisory for the deprecated `SELECT TOP`
// modifier: openEHR QUERY Release-1.1.0 §4.4.3 deprecates it in favour of
// `LIMIT` with `ORDER BY` and announces its removal, so a query carrying one
// gets a Warning — advisory, not a rejection, because the construct is still
// legal AQL and the SDK parses and emits it faithfully (REQ-118).
// REQ-118 · REQ-109
func TestLintDeprecatedTop(t *testing.T) {
	for _, in := range []string{
		"SELECT TOP 5 e/ehr_id/value FROM EHR e",
		"SELECT TOP 5 BACKWARD e/ehr_id/value FROM EHR e",
		"SELECT DISTINCT TOP 5 e/ehr_id/value FROM EHR e",
	} {
		t.Run(in, func(t *testing.T) {
			res := lint.LintString(in, nil)
			if !has(res, "aql_deprecated_top") {
				t.Errorf("aql_deprecated_top not raised for %q: %+v", in, res.Issues)
			}
			if count(res, "aql_deprecated_top") != 1 {
				t.Errorf("aql_deprecated_top raised %d times, want exactly 1", count(res, "aql_deprecated_top"))
			}
			if has(res, "aql_top_with_limit") {
				t.Errorf("aql_top_with_limit raised without a LIMIT clause: %+v", res.Issues)
			}
			// A deprecation is advisory: Warnings do not make a result not-OK.
			if !res.OK() {
				t.Errorf("Result.OK() = false, want true (deprecation is a Warning): %+v", res.Issues)
			}
		})
	}
}

// TestLintTopWithLimit pins the Error for the combination the spec forbids
// outright — "It is not allowed to use TOP while also using LIMIT clause in the
// same query" (QUERY Release-1.1.0 §4.4.3). The parser and the emitter carry
// both faithfully; the lint gate is where the prohibition is reported, and it
// makes the result not-OK (REQ-118).
// REQ-118 · REQ-109
func TestLintTopWithLimit(t *testing.T) {
	const in = "SELECT TOP 5 e/ehr_id/value FROM EHR e LIMIT 10 OFFSET 2"
	res := lint.LintString(in, nil)
	if !has(res, "aql_top_with_limit") {
		t.Errorf("aql_top_with_limit not raised for %q: %+v", in, res.Issues)
	}
	// The deprecation still applies — the two codes are independent findings,
	// and lint is collect-all.
	if !has(res, "aql_deprecated_top") {
		t.Errorf("aql_deprecated_top not raised alongside the combination: %+v", res.Issues)
	}
	if res.OK() {
		t.Error("Result.OK() = true, want false (the combination is an Error)")
	}
}

// TestLintNoTopClauseRaisesNeitherCode is the negative control: neither code
// may fire on a query that carries no TOP, including one using the LIMIT
// channel the spec recommends instead.
// REQ-118 · REQ-109
func TestLintNoTopClauseRaisesNeitherCode(t *testing.T) {
	for _, in := range []string{
		"SELECT e/ehr_id/value FROM EHR e",
		"SELECT e/ehr_id/value FROM EHR e ORDER BY e/time_created DESC LIMIT 10",
	} {
		t.Run(in, func(t *testing.T) {
			res := lint.LintString(in, nil)
			if has(res, "aql_deprecated_top") || has(res, "aql_top_with_limit") {
				t.Errorf("a TOP code fired on a query with no TOP clause: %+v", res.Issues)
			}
		})
	}
}

// TestLintTopWithUnrepresentableCount pins the split between PRESENCE and
// REPRESENTABILITY (REQ-118). A `TOP` count outside Go `int` leaves
// parse.Document.Top nil — nothing is truncated into a bound — but the query
// still USED the deprecated construct, and still pairs it with a LIMIT the spec
// forbids. Keying the checks on the decoded bound silenced both findings for
// exactly this query; they key on Document.HasTop instead.
// REQ-118 · REQ-109
func TestLintTopWithUnrepresentableCount(t *testing.T) {
	const oor = "SELECT TOP 99999999999999999999 e/ehr_id/value FROM EHR e LIMIT 10"
	res := lint.LintString(oor, nil)
	if !has(res, "aql_deprecated_top") {
		t.Errorf("aql_deprecated_top not raised for an out-of-range TOP: %v", codes(res))
	}
	if !has(res, "aql_top_with_limit") {
		t.Errorf("aql_top_with_limit not raised for an out-of-range TOP beside a LIMIT: %v", codes(res))
	}
	if res.OK() {
		t.Error("Result.OK() = true, want false (the TOP+LIMIT combination is an Error)")
	}
	// The flat view must still refuse to invent a bound: presence is known,
	// the count is not.
	doc, err := parse.Parse(oor)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !doc.HasTop {
		t.Error("Document.HasTop = false, want true (the clause is present in the source)")
	}
	if doc.Top != nil {
		t.Errorf("Document.Top = %+v, want nil (an unrepresentable count must not become a bound)", *doc.Top)
	}
}

// --- REQ-119 fallout: verbatim predicate text vs Layer-3 resolution ----------

// TestLintIssuePathIsCanonical — [lint.Issue.Path] is what a line-oriented
// report prints, and since REQ-119 the parsed path text is VERBATIM, so
// setting it from `p.Raw` put a predicate's comment and its raw newline into
// a single-line diagnostic. Both localised issue sites render through
// displayPath instead: trivia normalised, the alias and every value byte kept.
func TestLintIssuePathIsCanonical(t *testing.T) {
	find := func(r lint.Result, code string) *lint.Issue {
		for i := range r.Issues {
			if r.Issues[i].Code == code {
				return &r.Issues[i]
			}
		}
		return nil
	}

	t.Run("unknown alias (Layer 2)", func(t *testing.T) {
		r := lint.LintString("SELECT x/items[at0001 -- note\n]/value FROM OBSERVATION o", nil)
		is := find(r, "aql_unknown_alias")
		if is == nil {
			t.Fatalf("no aql_unknown_alias issue: %v", codes(r))
		}
		if want := "x/items[at0001]/value"; is.Path != want {
			t.Errorf("Issue.Path = %q, want %q", is.Path, want)
		}
	})

	t.Run("path not in template (Layer 3)", func(t *testing.T) {
		c := mustCompile(t, "nested.en.v1")
		q := "SELECT s/items[at0000]/activities[at0001]/description[at0000]" +
			"/items[at0002 -- pick\n]/bogus FROM SECTION s[openEHR-EHR-SECTION.nested.v1]"
		r := lint.LintString(q, &lint.Options{Compiled: c})
		is := find(r, "aql_path_not_in_template")
		if is == nil {
			t.Fatalf("no aql_path_not_in_template issue: %v (the row must diverge or it tests nothing)", codes(r))
		}
		want := "s/items[at0000]/activities[at0001]/description[at0000]/items[at0002]/bogus"
		if is.Path != want {
			t.Errorf("Issue.Path = %q, want %q", is.Path, want)
		}
	})
}

// TestLintLayer3ResolvesPredicatesThroughTrivia is the regression for the one
// cross-package consequence of REQ-119 making the read side report bracket text
// VERBATIM: Layer 3 selects a compiled OPT child by comparing the segment
// predicate against its node id, and `[ at0001 ]` used to arrive
// whitespace-collapsed from `GetText()`.
//
// Compared raw, the named child becomes unreachable and the lenient first-child
// fallback descends a SIBLING. nested.en.v1 is the fixture that makes that
// observable rather than merely wrong: under `description[at0000]/items` the
// children are at0002 (which carries `value`) and at0000 (which carries
// `items`), so resolving to the wrong one turns a valid path into an
// `aql_path_not_in_template` warning. REQ-109 § Layer 3 calls this descent
// "predicate-aware" and warns only on high-confidence structural divergence.
func TestLintLayer3ResolvesPredicatesThroughTrivia(t *testing.T) {
	c := mustCompile(t, "nested.en.v1")
	const arch = "openEHR-EHR-SECTION.nested.v1"

	for _, tc := range []struct{ name, segment string }{
		{"no trivia", "items[at0000]"},
		{"padded", "items[ at0000 ]"},
		{"newline", "items[at0000\n]"},
		{"comment", "items[at0000 -- pick the cluster\n]"},
		{"bom", "items[\uFEFFat0000]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q := "SELECT s/items[at0000]/activities[at0001]/description[at0000]/" +
				tc.segment + "/items[at0001]/value FROM SECTION s[" + arch + "]"
			r := lint.LintString(q, &lint.Options{Compiled: c})
			if has(r, "aql_path_not_in_template") {
				t.Errorf("trivia inside the predicate changed Layer-3 resolution: %v\n  query %q",
					codes(r), q)
			}
		})
	}
}

// TestLintPathSuffixIsCanonical — [lint.Path.Suffix] is documented canonical and
// is what a diagnostic renders, so no skipped trivia — a raw newline, a run of
// padding, an AQL comment — may survive into it now that the source text is
// verbatim. Trivia BETWEEN tokens collapses to one space, not to nothing:
// `[a/b =\n 1]` names the same predicate as `[a/b = 1]`, never `[a/b=1]`.
func TestLintPathSuffixIsCanonical(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"SELECT o/items[at0001]/value FROM OBSERVATION o", "/items[at0001]/value"},
		{"SELECT o/items[ at0001 ]/value FROM OBSERVATION o", "/items[at0001]/value"},
		{"SELECT o/items[at0001\n]/value FROM OBSERVATION o", "/items[at0001]/value"},
		{"SELECT o/items[at0001 -- note\n]/value FROM OBSERVATION o", "/items[at0001]/value"},
		// Interior trivia collapses to ONE space — a raw newline inside the
		// predicate must not reach the line-oriented report.
		{"SELECT o/items[a/b =\n 1]/value FROM OBSERVATION o", "/items[a/b = 1]/value"},
		{"SELECT o/items[a/b -- note\n = 1]/value FROM OBSERVATION o", "/items[a/b = 1]/value"},
	} {
		doc := mustParse(t, tc.src)
		if len(doc.Paths) == 0 {
			t.Fatalf("Parse(%q) reported no paths", tc.src)
		}
		got, err := lint.Normalise(doc.Paths[0])
		if err != nil {
			t.Fatalf("Normalise: %v", err)
		}
		if got.Suffix != tc.want {
			t.Errorf("Suffix = %q, want %q (src %q)", got.Suffix, tc.want, tc.src)
		}
		// …while the SEGMENTS stay verbatim, which is what round-trip needs.
		if len(got.Segments) == 0 || got.Segments[0].Predicate == "" {
			t.Fatalf("Segments lost the predicate: %+v", got.Segments)
		}
	}
}
