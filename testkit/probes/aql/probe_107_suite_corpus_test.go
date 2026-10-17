package aqlprobes

import (
	"fmt"
	"io/fs"
	"maps"
	"regexp"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/cadasto/openehr-sdk-go/openehr/aql/lint"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// PROBE-107 (REQ-109 § Syntax floor, REQ-160 § Containable operands and
// § Overlay edges, REQ-161 § Checks): the upstream AQL suite admissibility
// corpus. These tests are internal so the controls can hand the reader a
// corpus built in memory and hand the probe its decline table and family list
// with one guard's input removed.

// suiteTally is what PROBE-107 Ratchet asks the running test to assert: the
// queries per class and per family, the unrebuilt ones per reason, and the
// answered queries each decline covers, keyed by the construct it names.
type suiteTally struct {
	classes   map[SuiteClass]int
	families  map[string]map[SuiteClass]int
	unrebuilt map[string]int
	declines  map[string]int
}

// probe107Pin is the tally of the vendored suite corpus at the pin its
// AQL_SOURCE.txt records. A refresh that moves any query, between classes,
// families or reasons, fails TestProbe107SuiteCorpus_REQ109_REQ160 until this
// pin and the PROBE-107 Status move together, deliberately.
var probe107Pin = suiteTally{
	classes: map[SuiteClass]int{
		SuiteAnswered: 909, SuiteAnsweredEmpty: 31, SuiteRefused: 6, SuiteNotReady: 7, SuiteSkipped: 0,
	},
	families: map[string]map[SuiteClass]int{
		"AGGREGATE_FUNCTIONS": {SuiteAnswered: 73},
		"EXTERNAL":            {SuiteAnswered: 304, SuiteAnsweredEmpty: 2, SuiteNotReady: 7},
		"FOLDER":              {SuiteAnswered: 36},
		"FROM":                {SuiteAnswered: 75, SuiteAnsweredEmpty: 2},
		"LIMIT":               {SuiteAnswered: 9, SuiteAnsweredEmpty: 1},
		"META":                {SuiteAnswered: 12},
		"MIXED":               {SuiteAnswered: 15},
		"ORDER_BY":            {SuiteAnswered: 68},
		"PARAMETER":           {SuiteAnswered: 10},
		"SELECT":              {SuiteAnswered: 167, SuiteAnsweredEmpty: 10, SuiteRefused: 5},
		"VERSION":             {SuiteAnswered: 31},
		"WHERE":               {SuiteAnswered: 109, SuiteAnsweredEmpty: 16, SuiteRefused: 1},
	},
	// The VERSION suites call Get Composition Time Committed (14 times) and
	// Get EHR Status Time Committed (twice), whose query text lives in the
	// upstream resource.
	unrebuilt: map[string]int{reasonQueryNotVendored: 16},
	declines:  map[string]int{"CONTAINS operand EVENT_CONTEXT": 25},
}

func tallySuiteCorpus(c SuiteCorpus, declines []suiteDecline) suiteTally {
	t := suiteTally{
		classes:   map[SuiteClass]int{},
		families:  map[string]map[SuiteClass]int{},
		unrebuilt: map[string]int{},
		declines:  map[string]int{},
	}
	for _, q := range c.Queries {
		t.classes[q.Class]++
		if t.families[q.Family] == nil {
			t.families[q.Family] = map[SuiteClass]int{}
		}
		t.families[q.Family][q.Class]++
	}
	for _, u := range c.Unrebuilt {
		t.unrebuilt[u.Reason]++
	}
	for i, n := range suiteDeclineCoverage(c, declines) {
		t.declines[declines[i].Construct] = n
	}
	return t
}

// diff lists every count on which got differs from want, a missing count
// reading as zero.
func (want suiteTally) diff(got suiteTally) []string {
	var out []string
	for _, class := range suiteClasses {
		if g, w := got.classes[class], want.classes[class]; g != w {
			out = append(out, fmt.Sprintf("class %s: %d queries, want %d", class, g, w))
		}
	}
	for _, fam := range slices.Sorted(maps.Keys(union(want.families, got.families))) {
		for _, class := range suiteClasses {
			if g, w := got.families[fam][class], want.families[fam][class]; g != w {
				out = append(out, fmt.Sprintf("family %s class %s: %d queries, want %d", fam, class, g, w))
			}
		}
	}
	for _, reason := range slices.Sorted(maps.Keys(union(want.unrebuilt, got.unrebuilt))) {
		if g, w := got.unrebuilt[reason], want.unrebuilt[reason]; g != w {
			out = append(out, fmt.Sprintf("unrebuilt %s: %d, want %d", reason, g, w))
		}
	}
	for _, construct := range slices.Sorted(maps.Keys(union(want.declines, got.declines))) {
		if g, w := got.declines[construct], want.declines[construct]; g != w {
			out = append(out, fmt.Sprintf("decline on %s: covers %d answered queries, want %d", construct, g, w))
		}
	}
	return out
}

func union[V any](a, b map[string]V) map[string]V {
	out := maps.Clone(a)
	if out == nil {
		out = map[string]V{}
	}
	maps.Copy(out, b)
	return out
}

func probe107Corpus(t *testing.T) SuiteCorpus {
	t.Helper()
	c, err := ReadSuiteCorpus(fixtures.AQLSuitesRoot())
	if err != nil {
		t.Fatalf("ReadSuiteCorpus: %v", err)
	}
	return c
}

// TestProbe107SuiteCorpus_REQ109_REQ160 runs PROBE-107 over the vendored
// suite corpus, logs what it covered, and asserts the pinned tally (PROBE-107
// Ratchet): a per-family tripwire fires only at zero, so a query moving
// between classes, or a family shrinking, would otherwise stay green.
func TestProbe107SuiteCorpus_REQ109_REQ160(t *testing.T) {
	t.Parallel()
	c := probe107Corpus(t)
	got := tallySuiteCorpus(c, suiteDeclines)
	for _, fam := range slices.Sorted(maps.Keys(got.families)) {
		t.Logf("family %s: %v", fam, got.families[fam])
	}
	t.Logf("classes: %v; declines: %v", got.classes, got.declines)
	for _, u := range c.Unrebuilt {
		t.Logf("unrebuilt %s: %s (%s) — %s", u.Where(), u.Reason, u.Detail, u.Why)
	}

	for _, d := range probe107Pin.diff(got) {
		t.Errorf("%s; the PROBE-107 pin records the corpus at AQL_SOURCE.txt — "+
			"re-baseline the pin and the Status together after a refresh", d)
	}

	r, err := Probe107SuiteCorpus(c)
	if err != nil {
		t.Fatalf("Probe107SuiteCorpus: %v", err)
	}
	if r.Status != "pass" {
		t.Fatalf("Probe107SuiteCorpus status = %q, detail = %q", r.Status, r.Detail)
	}
	if r.Probe != "PROBE-107" {
		t.Errorf("Probe id = %q, want PROBE-107", r.Probe)
	}
}

// TestProbe107RatchetDetectsAMovedQuery_REQ160 is the control for the tally
// assertion above: each axis of the pin must notice a query that moves on it.
func TestProbe107RatchetDetectsAMovedQuery_REQ160(t *testing.T) {
	t.Parallel()
	c := probe107Corpus(t)
	if d := probe107Pin.diff(tallySuiteCorpus(c, suiteDeclines)); len(d) != 0 {
		t.Fatalf("premise: the shipping corpus matches the pin, but it differs: %v", d)
	}
	firstOf := func(c SuiteCorpus, keep func(SuiteQuery) bool) int {
		i := slices.IndexFunc(c.Queries, keep)
		if i < 0 {
			t.Fatal("premise: the corpus holds no query to move")
		}
		return i
	}
	eventContext := func(q SuiteQuery) bool { return strings.Contains(q.Query, "CONTAINS EVENT_CONTEXT") }
	cases := []struct {
		name   string
		mutate func(c *SuiteCorpus)
		want   string
	}{
		{
			name: "answered_query_reclassed",
			mutate: func(c *SuiteCorpus) {
				c.Queries[firstOf(*c, func(q SuiteQuery) bool { return q.Class == SuiteAnswered })].Class = SuiteRefused
			},
			want: "class refused: 7 queries, want 6",
		},
		{
			name: "query_moved_between_families",
			mutate: func(c *SuiteCorpus) {
				c.Queries[firstOf(*c, func(q SuiteQuery) bool { return q.Family == "META" })].Family = "MIXED"
			},
			want: "family META class answered: 11 queries, want 12",
		},
		{
			name: "query_became_unrebuildable",
			mutate: func(c *SuiteCorpus) {
				i := firstOf(*c, func(q SuiteQuery) bool { return q.Family == "LIMIT" })
				c.Unrebuilt = append(c.Unrebuilt, SuiteUnrebuilt{Family: "LIMIT", Reason: reasonUnquotedVariable})
				c.Queries = slices.Delete(c.Queries, i, i+1)
			},
			want: "unrebuilt " + reasonUnquotedVariable + ": 1, want 0",
		},
		{
			name: "declined_query_lost",
			mutate: func(c *SuiteCorpus) {
				i := firstOf(*c, eventContext)
				c.Queries = slices.Delete(c.Queries, i, i+1)
			},
			want: "decline on CONTAINS operand EVENT_CONTEXT: covers 24 answered queries, want 25",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			moved := SuiteCorpus{Queries: slices.Clone(c.Queries), Unrebuilt: slices.Clone(c.Unrebuilt)}
			tc.mutate(&moved)
			d := probe107Pin.diff(tallySuiteCorpus(moved, suiteDeclines))
			if !slices.Contains(d, tc.want) {
				t.Fatalf("pin diff = %v, want it to contain %q", d, tc.want)
			}
		})
	}
}

// TestProbe107PinsTheNamedSuiteQueries_REQ160 pins by name the queries this
// corpus was vendored for: the FOLDER version-tier queries the FOLDER ->
// VERSIONED_COMPOSITION hop admits, the two answered-empty shapes an engine
// accepts and the relation verdicts Never, and the NOT CONTAINS query its test
// still tags not-ready.
func TestProbe107PinsTheNamedSuiteQueries_REQ160(t *testing.T) {
	t.Parallel()
	c := probe107Corpus(t)
	cases := []struct {
		name, suite, contains string
		class                 SuiteClass
		// errs is the Error-severity code multiset lint raises on each
		// matching query; anyErrs leaves it unchecked for a class the probe
		// never asserts.
		errs    []string
		anyErrs bool
	}{
		{
			name: "folder_version_tier", suite: "AQL/FOLDER/FROM_FOLDER_CONTAINS_COMPO/select_with_version.robot",
			contains: "FROM FOLDER f CONTAINS VERSION cv0[LATEST_VERSION] CONTAINS COMPOSITION c", class: SuiteAnswered,
		},
		{
			name: "ehr_status_contains_itself", suite: "AQL/FROM/EHR_STATUS/from_multiple_ehrs.robot",
			contains: "FROM EHR_STATUS eq CONTAINS EHR_STATUS eq", class: SuiteAnsweredEmpty,
			errs: []string{codeImpossibleContainment},
		},
		{
			name: "composition_contains_composition", suite: "AQL/FROM/PREDICATE_A_D/without_predicate.robot",
			contains: "CONTAINS COMPOSITION c1", class: SuiteAnsweredEmpty,
			errs: []string{codeImpossibleContainment},
		},
		{
			name: "not_contains_not_ready", suite: "AQL/EXTERNAL/query_test_set_4.robot",
			contains: "AND NOT CONTAINS COMPOSITION c2", class: SuiteNotReady, anyErrs: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			n := 0
			for _, q := range c.Queries {
				if q.Suite != tc.suite || !strings.Contains(q.Query, tc.contains) {
					continue
				}
				n++
				if q.Class != tc.class {
					t.Errorf("%s: class = %s, want %s (query %q)", q.Where(), q.Class, tc.class, q.Query)
				}
				var errs []string
				for _, i := range lint.LintString(q.Query, nil).Issues {
					if i.Severity == lint.Error {
						errs = append(errs, i.Code)
					}
				}
				if !tc.anyErrs && !slices.Equal(errs, tc.errs) {
					t.Errorf("%s: Error-severity codes = %v, want %v (query %q)", q.Where(), errs, tc.errs, q.Query)
				}
			}
			if n == 0 {
				t.Fatalf("no query in %s contains %q; the corpus lost the query this pin names", tc.suite, tc.contains)
			}
		})
	}
}

// suiteFixture is an in-memory corpus: the two ingest files, empty AQL/ and
// _resources/ trees, and files.
func suiteFixture(files map[string]string) fstest.MapFS {
	m := fstest.MapFS{
		"AQL_SOURCE.txt": {Data: []byte("commit: fixture\n")},
		"EXCLUDED.txt":   {Data: []byte("# none\n")},
		"AQL":            {Mode: fs.ModeDir | 0o755},
		"_resources":     {Mode: fs.ModeDir | 0o755},
	}
	for p, data := range files {
		m[p] = &fstest.MapFile{Data: []byte(data)}
	}
	return m
}

// renderSuiteCorpus prints each rebuilt query as `class: query`, with its
// data row when it has one, then each unrebuilt one as
// `unrebuilt reason (detail)`.
func renderSuiteCorpus(c SuiteCorpus) []string {
	var out []string
	for _, q := range c.Queries {
		s := fmt.Sprintf("%s: %s", q.Class, q.Query)
		if q.Data != "" {
			s += " [" + q.Data + "]"
		}
		out = append(out, s)
	}
	for _, u := range c.Unrebuilt {
		out = append(out, fmt.Sprintf("unrebuilt %s (%s)", u.Reason, u.Detail))
	}
	return out
}

// TestProbe107ReaderRebuildsTheSuites_REQ109 is the control for each rule of
// PROBE-107 Rebuild and Classify: every case is a suite (AQL/WHERE/t.robot
// unless it names other files) whose rebuild or class depends on exactly one
// rule of the reader.
func TestProbe107ReaderRebuildsTheSuites_REQ109(t *testing.T) {
	t.Parallel()
	const dataDriven = "*** Settings ***\nLibrary    DataDriver\n" +
		"...    file=${PROJECT_ROOT}/tests/robot/_resources/w/combinations/t.csv\n...    dialect=excel\n\n"
	const csvPath = "_resources/w/combinations/t.csv"
	type rebuildCase struct {
		name  string
		suite string
		files map[string]string
		want  []string
	}
	cases := []rebuildCase{
		// --- Rebuild: a test case's own query.
		{
			name: "set_variable_then_send",
			suite: `*** Test Cases ***
SELECT x FROM LABEL l
    ${query}    Set Variable    SELECT c FROM COMPOSITION c
    Set AQL And Execute Ad Hoc Query    ${query}
`,
			want: []string{"answered: SELECT c FROM COMPOSITION c"},
		},
		{
			name: "continuation_row_carries_the_value",
			suite: `*** Test Cases ***
T
    ${query}    Set Variable
    ...    SELECT c FROM COMPOSITION c
    Set AQL And Execute Ad Hoc Query    ${query}
`,
			want: []string{"answered: SELECT c FROM COMPOSITION c"},
		},
		{
			// The text sender sends the argument as it is, so the rebuild
			// shows Robot's escapes alone.
			name: "robot_escapes_resolve",
			suite: `*** Test Cases ***
T
    ${query}    Set Variable    SELECT c FROM COMPOSITION c WHERE c/uid/value = \$id AND c/name/value = 'a\\b\#c'
    Send Ad Hoc Request Through GET    ${query}
`,
			want: []string{`answered: SELECT c FROM COMPOSITION c WHERE c/uid/value = $id AND c/name/value = 'a\b#c'`},
		},
		{
			// A resolved control character inside the JSON string of a request
			// body leaves the body undecodable, as the engine would find it;
			// between JSON tokens it is whitespace, and a printable character
			// is held as it is.
			name: "resolved_escape_in_a_json_body",
			suite: `*** Test Cases ***
T
    Set AQL And Execute Ad Hoc Query    SELECT c\nFROM COMPOSITION c
    Set AQL And Execute Ad Hoc Query    SELECT c${\n}FROM COMPOSITION c
    PUT /definition/query/{qualified_query_name}    SELECT c\tFROM COMPOSITION c
    Send Ad Hoc Request    {"q": "SELECT c\rFROM COMPOSITION c"}
    Send Ad Hoc Request    {\n"q": "SELECT c FROM COMPOSITION c"\t}
    Set AQL And Execute Ad Hoc Query    SELECT c FROM COMPOSITION c WHERE c/name/value = '\u00e9'
`,
			want: []string{
				"answered: SELECT c FROM COMPOSITION c",
				"answered: SELECT c FROM COMPOSITION c WHERE c/name/value = '\u00e9'",
				"unrebuilt " + reasonBodyNotJSON + " (query_expr)",
				"unrebuilt " + reasonBodyNotJSON + " (query_expr)",
				"unrebuilt " + reasonBodyNotJSON + " (query_to_store)",
				"unrebuilt " + reasonBodyNotJSON + " (aql_body)",
			},
		},
		{
			// Upstream wraps the argument as {"q":"…"} and the engine decodes
			// that JSON: one level of backslashes goes on each side.
			name: "ad_hoc_query_is_the_q_member_the_engine_decodes",
			suite: `*** Test Cases ***
T
    Set AQL And Execute Ad Hoc Query    SELECT s FROM SECTION s WHERE s/name/value LIKE 'Name\\\\\\\\*'
    Set AQL And Execute Ad Hoc Query    SELECT c FROM COMPOSITION c WHERE c/uid/value = \$id    parameter={"id": "x"}
`,
			want: []string{
				`answered: SELECT s FROM SECTION s WHERE s/name/value LIKE 'Name\\*'`,
				"answered: SELECT c FROM COMPOSITION c WHERE c/uid/value = $id",
			},
		},
		{
			name: "stored_query_is_the_q_member_unless_stored_as_text",
			suite: `*** Test Cases ***
T
    PUT /definition/query/{qualified_query_name}    SELECT c FROM COMPOSITION c WHERE c/name/value LIKE 'a\\\\*'
    PUT /definition/query/{qualified_query_name}/{version}    query_to_store=SELECT o FROM OBSERVATION o WHERE o/name/value LIKE 'a\\\\*'    format=text
    PUT AQL Query With Qualified Name And Version Multitenancy    SELECT e FROM EHR e    json
`,
			want: []string{
				`answered: SELECT c FROM COMPOSITION c WHERE c/name/value LIKE 'a\*'`,
				`answered: SELECT o FROM OBSERVATION o WHERE o/name/value LIKE 'a\\*'`,
				"answered: SELECT e FROM EHR e",
			},
		},
		{
			name: "json_body_the_engine_would_refuse_is_counted",
			suite: `*** Test Cases ***
T
    Set AQL And Execute Ad Hoc Query    SELECT c FROM COMPOSITION c WHERE c/name/value = "x"
    Set AQL And Execute Ad Hoc Query    SELECT c FROM COMPOSITION c    parameter={"id":
    PUT /definition/query/{qualified_query_name}    SELECT c FROM COMPOSITION c WHERE c/name/value = "x"
    PUT /definition/query/{qualified_query_name}    SELECT c FROM COMPOSITION c WHERE c/name/value = "x"    format=text
`,
			want: []string{
				`answered: SELECT c FROM COMPOSITION c WHERE c/name/value = "x"`,
				"unrebuilt " + reasonBodyNotJSON + " (query_expr)",
				"unrebuilt " + reasonBodyNotJSON + " (query_expr)",
				"unrebuilt " + reasonBodyNotJSON + " (query_to_store)",
			},
		},
		{
			name: "stored_query_in_a_format_upstream_does_not_send_is_counted",
			suite: `*** Test Cases ***
T
    PUT /definition/query/{qualified_query_name}    SELECT c FROM COMPOSITION c    format=xml
`,
			want: []string{"unrebuilt " + reasonUnknownQueryText + " (format)"},
		},
		{
			name: "empty_and_space_built_ins",
			suite: `*** Test Cases ***
T
    Set AQL And Execute Ad Hoc Query    SELECT${SPACE}c FROM COMPOSITION c${EMPTY}
`,
			want: []string{"answered: SELECT c FROM COMPOSITION c"},
		},
		{
			name: "catenate_with_and_without_separator",
			suite: `*** Test Cases ***
T
    ${cols}    Catenate    SEPARATOR=,${SPACE}    c/uid/value    c/name/value
    ${query}    Catenate    SELECT ${cols}    FROM COMPOSITION c
    Set AQL And Execute Ad Hoc Query    ${query}
`,
			want: []string{"answered: SELECT c/uid/value, c/name/value FROM COMPOSITION c"},
		},
		{
			name: "variables_section_rows_join_with_a_space",
			suite: `*** Variables ***
${q}    SELECT c
...     FROM COMPOSITION c

*** Test Cases ***
T
    Set Test Variable    ${query}    ${q}
    Set AQL And Execute Ad Hoc Query    ${query}
`,
			want: []string{"answered: SELECT c FROM COMPOSITION c"},
		},
		{
			name: "dictionary_item",
			suite: `*** Test Cases ***
T
    ${d}    Create Dictionary    tmp_query=SELECT c FROM COMPOSITION c
    ${query}    Set Variable    ${d["tmp_query"]}
    Set AQL And Execute Ad Hoc Query    ${query}
`,
			want: []string{"answered: SELECT c FROM COMPOSITION c"},
		},
		{
			name: "replace_variables",
			suite: `*** Test Cases ***
T
    ${tmp}    Set Variable    SELECT c FROM COMPOSITION c
    ${query}    Replace Variables    ${tmp}
    Set AQL And Execute Ad Hoc Query    ${query}
`,
			want: []string{"answered: SELECT c FROM COMPOSITION c"},
		},
		{
			name: "test_variable_reaches_a_keyword",
			suite: `*** Test Cases ***
T
    Set Test Variable    ${query}    SELECT c FROM COMPOSITION c
    Run Query

*** Keywords ***
Run Query
    Set AQL And Execute Ad Hoc Query    ${query}
`,
			want: []string{"answered: SELECT c FROM COMPOSITION c"},
		},
		{
			name: "named_and_default_arguments",
			suite: `*** Test Cases ***
T
    Execute Query    rows=2    path=c/uid/value

*** Keywords ***
Execute Query
    [Arguments]    ${path}    ${rows}    ${from}=COMPOSITION c
    Set AQL And Execute Ad Hoc Query    SELECT ${path} FROM ${from}
`,
			want: []string{"answered: SELECT c/uid/value FROM COMPOSITION c"},
		},
		{
			name: "embedded_argument_keyword",
			suite: `*** Test Cases ***
T
    Select Constant 2026

*** Keywords ***
Select Constant ${value}
    Set AQL And Execute Ad Hoc Query    SELECT '${value}' FROM EHR e
`,
			want: []string{"answered: SELECT '2026' FROM EHR e"},
		},
		{
			// A lookahead compiles in Robot's regexp dialect and not in Go's:
			// the keyword falls back to the default pattern rather than vanish.
			name: "embedded_argument_pattern_this_dialect_cannot_compile",
			suite: `*** Test Cases ***
T
    Select Constant 2026

*** Keywords ***
Select Constant ${value:(?=2)\d+}
    Set AQL And Execute Ad Hoc Query    SELECT '${value}' FROM EHR e
`,
			want: []string{"answered: SELECT '2026' FROM EHR e"},
		},
		{
			name: "run_time_variable_in_a_quoted_literal_takes_the_fixed_literal",
			suite: `*** Test Cases ***
T
    Set AQL And Execute Ad Hoc Query    SELECT c FROM COMPOSITION c WHERE c/uid/value = '${compo_id}::${system_id}::1'
`,
			want: []string{"answered: SELECT c FROM COMPOSITION c WHERE c/uid/value = '" + suiteFixedLiteral + "::" + suiteFixedLiteral + "::1'"},
		},
		// --- Rebuild: what is counted rather than rebuilt.
		{
			name: "run_time_variable_outside_a_quoted_literal_is_counted",
			suite: `*** Test Cases ***
T
    Set AQL And Execute Ad Hoc Query    SELECT c FROM COMPOSITION c LIMIT ${n}
`,
			want: []string{"unrebuilt " + reasonUnquotedVariable + " (n)"},
		},
		{
			name: "several_values_make_a_list",
			suite: `*** Test Cases ***
T
    ${query}    Set Variable    SELECT c    FROM COMPOSITION c
    Set AQL And Execute Ad Hoc Query    ${query}
`,
			want: []string{"unrebuilt " + reasonQueryNotText + " (query_expr)"},
		},
		{
			name: "query_text_from_a_keyword_the_reader_does_not_follow_is_counted",
			suite: `*** Test Cases ***
T
    ${query}    Get File    q.aql
    Set AQL And Execute Ad Hoc Query    ${query}
`,
			want: []string{"unrebuilt " + reasonUnknownQueryText + " (query)"},
		},
		{
			// Neither name says "query": the reader counts every keyword it
			// does not know, whatever it is called.
			name: "unknown_keyword_is_counted",
			suite: `*** Test Cases ***
T
    Send Stored Query Request    SELECT c FROM COMPOSITION c
    Fetch Rows    SELECT o FROM OBSERVATION o
    Set AQL And Execute Ad Hoc Query    SELECT e FROM EHR e
    Unknown Check
    Should Be Empty    ${resp_body['rows']}
`,
			want: []string{
				"answered: SELECT e FROM EHR e",
				"unrebuilt " + reasonUnknownKeyword + " (Send Stored Query Request)",
				"unrebuilt " + reasonUnknownKeyword + " (Fetch Rows)",
				"unrebuilt " + reasonUnknownKeyword + " (Unknown Check)",
			},
		},
		{
			name: "query_text_inside_an_upstream_resource_is_counted",
			suite: `*** Test Cases ***
T
    ${t1}    Get Composition Time Committed    ${uid}
    ${t2}    Get EHR Status Time Committed    ${uid}
    Set AQL And Execute Ad Hoc Query    SELECT c FROM COMPOSITION c WHERE c/context/start_time/value = '${t1}'
`,
			want: []string{
				"answered: SELECT c FROM COMPOSITION c WHERE c/context/start_time/value = '" + suiteFixedLiteral + "'",
				"unrebuilt " + reasonQueryNotVendored + " (Get Composition Time Committed)",
				"unrebuilt " + reasonQueryNotVendored + " (Get EHR Status Time Committed)",
			},
		},
		{
			name: "upstream_sender_in_a_form_the_reader_does_not_rebuild_is_counted",
			suite: `*** Test Cases ***
T
    Set Test Variable    ${payload}    {"q": "SELECT c FROM COMPOSITION c"}
    POST /query/aql (REST)    JSON
    Should Be Empty    ${resp_body['rows']}
`,
			want: []string{"unrebuilt " + reasonSendNotRebuilt + " (POST /query/aql (REST))"},
		},
		{
			// Library and upstream keywords that send no query leave the open
			// query open: the assertion after them still applies to it.
			name: "inert_keywords_send_nothing",
			suite: `*** Test Cases ***
T
    Set AQL And Execute Ad Hoc Query    SELECT c FROM COMPOSITION c
    Log    ${resp_body}
    Should Be Equal As Strings    ${resp_status_code}    200
    Upload OPT For AQL    t.opt
    GET /query/{qualified_query_name}    qualif_name=org.openehr::q
    Should Be Empty    ${resp_body['rows']}
`,
			want: []string{"answered-empty: SELECT c FROM COMPOSITION c"},
		},
		{
			name: "session_request_to_a_query_endpoint_is_counted",
			suite: `*** Test Cases ***
T
    ${resp}    GET On Session    ${SUT}    /definition/query/org.openehr::q
    ${resp}    GET On Session    ${SUT}    url=${url}
    ${resp}    POST On Session    ${SUT}    /query/aql    data={"q": "SELECT c FROM COMPOSITION c"}
    ${resp}    GET On Session    ${SUT}    /ehr/x    params=${params}
    ${resp}    DELETE On Session    ${SUT}    /ehr/x
`,
			want: []string{
				"unrebuilt " + reasonSendNotRebuilt + " (GET On Session)",
				"unrebuilt " + reasonSendNotRebuilt + " (GET On Session)",
				"unrebuilt " + reasonSendNotRebuilt + " (POST On Session)",
			},
		},
		{
			name: "sessionless_request_to_a_query_endpoint_is_counted",
			suite: `*** Test Cases ***
T
    ${resp}    GET    ${BASEURL}/query/aql    params=q=SELECT c FROM COMPOSITION c
    ${resp}    POST    url=http://localhost/query/aql    json=${body}
    ${resp}    GET    http://localhost/ehr
`,
			want: []string{
				"unrebuilt " + reasonSendNotRebuilt + " (GET)",
				"unrebuilt " + reasonSendNotRebuilt + " (POST)",
			},
		},
		{
			// Upstream documents ${payload} as a test variable: the sending
			// keyword cannot see a variable local to its caller.
			name: "payload_is_read_from_test_and_suite_scope_only",
			suite: `*** Test Cases ***
A
    ${payload}    Set Variable    SELECT c FROM COMPOSITION c
    GET /query/aql?q={query}
B
    Send Local Payload
C
    Set Suite Variable    ${payload}    SELECT e FROM EHR e
    GET /query/aql?q={query}

*** Keywords ***
Send Local Payload
    ${payload}    Set Variable    SELECT o FROM OBSERVATION o
    GET /query/aql?q={query}
`,
			want: []string{
				"answered: SELECT e FROM EHR e",
				"unrebuilt " + reasonUnknownQueryText + " (payload)",
				"unrebuilt " + reasonUnknownQueryText + " (payload)",
			},
		},
		{
			// Each test uses one construct the reader does not run; the
			// construct is counted where the test meets it.
			name: "unsupported_robot_constructs_are_counted",
			suite: `*** Test Cases ***
Inline If
    IF    ${flag}    Set AQL And Execute Ad Hoc Query    SELECT c FROM COMPOSITION c
Var
    VAR    ${query}    SELECT c FROM COMPOSITION c
Return
    ${query}    Build Query
    ${query}    Build Query The Old Way
Template
    [Template]    Set AQL And Execute Ad Hoc Query
    SELECT c FROM COMPOSITION c
Lone Surrogate
    Send Ad Hoc Request Through GET    SELECT c FROM COMPOSITION c WHERE c/name/value = '\uD800'
Path Separator
    Send Ad Hoc Request Through GET    SELECT c FROM COMPOSITION c WHERE c/name/value = 'a${/}b'
Float Number
    Send Ad Hoc Request Through GET    SELECT c FROM COMPOSITION c LIMIT ${2.5}

*** Keywords ***
Build Query
    RETURN    SELECT c FROM COMPOSITION c
Build Query The Old Way
    [Return]    SELECT c FROM COMPOSITION c
`,
			want: []string{
				"unrebuilt " + reasonUnsupported + " (inline IF)",
				"unrebuilt " + reasonUnsupported + " (VAR)",
				"unrebuilt " + reasonUnsupported + " (RETURN)",
				"unrebuilt " + reasonUnsupported + " ([Return])",
				"unrebuilt " + reasonUnsupported + " ([Template])",
				"unrebuilt " + reasonUnsupported + ` (\uD800)`,
				"unrebuilt " + reasonUnsupported + " (${/})",
				"unrebuilt " + reasonUnsupported + " (${2.5})",
			},
		},
		{
			name: "send_inside_a_control_block_is_counted",
			suite: `*** Test Cases ***
T
    IF    ${flag}
        Set AQL And Execute Ad Hoc Query    SELECT c FROM COMPOSITION c
    END
    Run Keyword If    ${flag}    Set AQL And Execute Ad Hoc Query    SELECT o FROM OBSERVATION o
`,
			want: []string{"unrebuilt " + reasonConditionalSend + " ()", "unrebuilt " + reasonConditionalSend + " ()"},
		},
		{
			name: "send_from_a_suite_setup_is_counted",
			suite: `*** Settings ***
Suite Setup    Set AQL And Execute Ad Hoc Query    SELECT c FROM COMPOSITION c

*** Test Cases ***
T
    Log    nothing sent
`,
			want: []string{"unrebuilt " + reasonOutsideTest + " ()"},
		},
		{
			// Every position a suite file runs a keyword from: its own suite
			// setup and teardown are counted, its test setup, teardown and
			// template are the defaults a test's own settings override.
			name: "every_suite_file_and_test_fixture_runs",
			suite: `*** Settings ***
Suite Setup    Log    nothing sent
Suite Teardown    Set AQL And Execute Ad Hoc Query    SELECT f FROM FOLDER f
Test Setup    Set AQL And Execute Ad Hoc Query    SELECT c FROM COMPOSITION c
Test Teardown    Set AQL And Execute Ad Hoc Query    SELECT o FROM OBSERVATION o

*** Test Cases ***
Defaults
    Log    body
Own Fixtures
    [Setup]    Set AQL And Execute Ad Hoc Query    SELECT e FROM EHR e
    [Teardown]    Set AQL And Execute Ad Hoc Query    SELECT s FROM SECTION s
    Log    body
No Fixtures
    [Setup]    NONE
    [Teardown]
    Set AQL And Execute Ad Hoc Query    SELECT p FROM POINT_EVENT p
`,
			files: map[string]string{
				"AQL/MIXED/t.robot": "*** Settings ***\nTest Template    Set AQL And Execute Ad Hoc Query\n\n" +
					"*** Test Cases ***\nT\n    SELECT c FROM COMPOSITION c\n",
			},
			want: []string{
				"answered: SELECT c FROM COMPOSITION c", "answered: SELECT o FROM OBSERVATION o",
				"answered: SELECT e FROM EHR e", "answered: SELECT s FROM SECTION s",
				"answered: SELECT p FROM POINT_EVENT p",
				"unrebuilt " + reasonUnsupported + " ([Template])",
				"unrebuilt " + reasonOutsideTest + " ()",
			},
		},
		{
			// A directory's init file runs its suite setup and teardown once,
			// around every suite below it, with its own keywords.
			name: "init_file_suite_setup_and_teardown_are_counted",
			suite: `*** Test Cases ***
T
    Log    nothing sent
`,
			files: map[string]string{
				"AQL/WHERE/__init__.robot": `*** Settings ***
Suite Setup    Set AQL And Execute Ad Hoc Query    SELECT c FROM COMPOSITION c
Suite Teardown    Send Query

*** Keywords ***
Send Query
    Set AQL And Execute Ad Hoc Query    SELECT o FROM OBSERVATION o
`,
				"AQL/WHERE/u.robot": "*** Test Cases ***\nU\n    Log    nothing sent\n",
			},
			want: []string{"unrebuilt " + reasonOutsideTest + " ()", "unrebuilt " + reasonOutsideTest + " ()"},
		},
		{
			// An init file's test setup and teardown are the defaults of every
			// test below it: a nearer init file, the suite file's own setting
			// and the test's own setting each override them.
			name: "init_file_test_setup_and_teardown_are_inherited",
			suite: `*** Test Cases ***
Inherited
    Log    body
Own Setup
    [Setup]    Set AQL And Execute Ad Hoc Query    SELECT e FROM EHR e
    Log    body
`,
			files: map[string]string{
				"AQL/WHERE/__init__.robot": "*** Settings ***\n" +
					"Test Setup    Set AQL And Execute Ad Hoc Query    SELECT c FROM COMPOSITION c\n" +
					"Test Teardown    Set AQL And Execute Ad Hoc Query    SELECT o FROM OBSERVATION o\n",
				"AQL/WHERE/u.robot": "*** Settings ***\n" +
					"Test Setup    Set AQL And Execute Ad Hoc Query    SELECT s FROM SECTION s\n\n" +
					"*** Test Cases ***\nSuite Setting\n    Log    body\n",
				"AQL/WHERE/NESTED/__init__.robot": "*** Settings ***\n" +
					"Test Setup    Set AQL And Execute Ad Hoc Query    SELECT p FROM POINT_EVENT p\n",
				"AQL/WHERE/NESTED/v.robot": "*** Test Cases ***\nNearer Init\n    Log    body\n",
			},
			want: []string{
				"answered: SELECT p FROM POINT_EVENT p", "answered: SELECT o FROM OBSERVATION o",
				"answered: SELECT c FROM COMPOSITION c", "answered: SELECT o FROM OBSERVATION o",
				"answered: SELECT e FROM EHR e", "answered: SELECT o FROM OBSERVATION o",
				"answered: SELECT s FROM SECTION s", "answered: SELECT o FROM OBSERVATION o",
			},
		},
		{
			// A keyword's own [Setup] and [Teardown] run before and after its
			// body, wherever the rows sit, with its arguments bound.
			name: "keyword_setup_and_teardown_run_around_its_body",
			suite: `*** Test Cases ***
T
    Run Query    SELECT o FROM OBSERVATION o
    Run Query Without Fixtures

*** Keywords ***
Run Query
    [Arguments]    ${closing}
    [Teardown]    Set AQL And Execute Ad Hoc Query    ${closing}
    [Setup]    Set AQL And Execute Ad Hoc Query    SELECT c FROM COMPOSITION c
    Set AQL And Execute Ad Hoc Query    SELECT e FROM EHR e
Run Query Without Fixtures
    [Setup]    NONE
    [Teardown]
    Set AQL And Execute Ad Hoc Query    SELECT s FROM SECTION s
`,
			want: []string{
				"answered: SELECT c FROM COMPOSITION c", "answered: SELECT e FROM EHR e",
				"answered: SELECT o FROM OBSERVATION o", "answered: SELECT s FROM SECTION s",
			},
		},
		{
			name: "keyword_recursion_is_counted",
			suite: `*** Test Cases ***
T
    Loop

*** Keywords ***
Loop
    Loop
`,
			want: []string{"unrebuilt " + reasonKeywordDepth + " (Loop)"},
		},
		// --- Rebuild: the other sending forms.
		{
			name: "ad_hoc_request_body",
			suite: `*** Test Cases ***
T
    Set Test Variable    ${query}    SELECT c FROM COMPOSITION c WHERE c/uid/value = \$id
    Set Test Variable    ${body}    {"q":"${query}","query_parameters":{"id": \"${uid}\"}}
    Send Ad Hoc Request    aql_body=${body}
`,
			want: []string{"answered: SELECT c FROM COMPOSITION c WHERE c/uid/value = $id"},
		},
		{
			name: "ad_hoc_request_body_not_json_is_counted",
			suite: `*** Test Cases ***
T
    Send Ad Hoc Request    aql_body={"q": ${query}}
`,
			want: []string{"unrebuilt " + reasonBodyNotJSON + " (aql_body)"},
		},
		{
			name: "ad_hoc_request_through_get_and_stored_query",
			suite: `*** Test Cases ***
T
    Send Ad Hoc Request Through GET    q_param=SELECT c FROM COMPOSITION c
    ${name}    PUT /definition/query/{qualified_query_name}/{version}
    ...    query_to_store=SELECT o FROM OBSERVATION o    format=text
    GET /query/{qualified_query_name}    qualif_name=${name}
`,
			want: []string{"answered: SELECT c FROM COMPOSITION c", "answered: SELECT o FROM OBSERVATION o"},
		},
		{
			name: "payload_and_session_get",
			suite: `*** Test Cases ***
T
    Set Test Variable    ${payload}    SELECT c FROM COMPOSITION c
    GET /query/aql?q={query}
    &{params}    Create Dictionary    q=SELECT o FROM OBSERVATION o    limit=1
    Query With Params    ${params}

*** Keywords ***
Query With Params
    [Arguments]    ${params}
    ${resp}    GET On Session    ${SUT}    /query/aql    params=${params}    headers=${h}
    ${resp}    GET On Session    ${SUT}    /ehr    params=${params}
`,
			want: []string{"answered: SELECT c FROM COMPOSITION c", "answered: SELECT o FROM OBSERVATION o"},
		},
		{
			name: "send_without_its_query_text_is_counted",
			suite: `*** Test Cases ***
T
    Set AQL And Execute Ad Hoc Query    parameter={"id": "x"}
    PUT /definition/query/{qualified_query_name}    format=text
    ${resp}    GET On Session    ${SUT}    /query/aql
    ${list}    Create List    SELECT c    FROM COMPOSITION c
    &{params}    Create Dictionary    q=${list}
    ${resp}    GET On Session    ${SUT}    /query/aql    params=${params}
`,
			want: []string{
				"unrebuilt " + reasonUnknownQueryText + " (query_expr)",
				"unrebuilt " + reasonUnknownQueryText + " (query_to_store)",
				"unrebuilt " + reasonUnknownQueryText + " (params)",
				"unrebuilt " + reasonQueryNotText + " (params)",
			},
		},
		// --- Rebuild: data-driven suites.
		{
			name: "data_driven_template_filled_from_every_row",
			suite: dataDriven + `*** Test Cases ***
SELECT ${path} LABEL
    [Template]    Execute Query
    ${path}    ${nr_of_results}

*** Keywords ***
Execute Query
    [Arguments]    ${path}    ${nr_of_results}
    ${query}    Set Variable    SELECT ${path} FROM EHR e WHERE e/ehr_id/value = '${ehr_id}'
    Set AQL And Execute Ad Hoc Query    ${query}
    Length Should Be    ${resp_body['rows']}    ${nr_of_results}
`,
			files: map[string]string{csvPath: "${query_nr},${path},${nr_of_results}\n" +
				"1.,e/ehr_id/value,1\n" +
				"2.,\"e/ehr_status/name/value\",0\n" +
				"3.,e/ehr_status/\\#x${SPACE}y,${EMPTY}2\n"},
			want: []string{
				"answered: SELECT e/ehr_id/value FROM EHR e WHERE e/ehr_id/value = '" + suiteFixedLiteral + "' [" + csvPath + ":2]",
				"answered-empty: SELECT e/ehr_status/name/value FROM EHR e WHERE e/ehr_id/value = '" + suiteFixedLiteral + "' [" + csvPath + ":3]",
				"answered: SELECT e/ehr_status/#x y FROM EHR e WHERE e/ehr_id/value = '" + suiteFixedLiteral + "' [" + csvPath + ":4]",
			},
		},
		{
			name: "data_driven_suite_with_no_template_is_counted",
			suite: dataDriven + `*** Test Cases ***
T
    Set AQL And Execute Ad Hoc Query    SELECT c FROM COMPOSITION c
`,
			files: map[string]string{csvPath: "${path}\nc\n"},
			want:  []string{"unrebuilt " + reasonNoTemplate + " (T)"},
		},
		{
			name: "data_driven_suite_with_two_tests_is_counted",
			suite: dataDriven + `*** Test Cases ***
A
    [Template]    Set AQL And Execute Ad Hoc Query
    ${path}
B
    [Template]    Set AQL And Execute Ad Hoc Query
    ${path}
`,
			files: map[string]string{csvPath: "${path}\nSELECT c FROM COMPOSITION c\n"},
			want:  []string{"unrebuilt " + reasonSeveralTemplates + " (2 test cases)"},
		},
		{
			name: "data_driven_suite_with_two_template_rows_is_counted",
			suite: dataDriven + `*** Test Cases ***
A
    [Template]    Set AQL And Execute Ad Hoc Query
    ${path}
    ${path}
`,
			files: map[string]string{csvPath: "${path}\nSELECT c FROM COMPOSITION c\n"},
			want:  []string{"unrebuilt " + reasonSeveralTemplates + " (2 template rows)"},
		},
		{
			name: "data_driven_suite_naming_a_missing_file_is_counted",
			suite: dataDriven + `*** Test Cases ***
A
    [Template]    Set AQL And Execute Ad Hoc Query
    ${path}
`,
			want: []string{"unrebuilt " + reasonDataFileMissing + " (" + csvPath + ")"},
		},
		{
			name: "data_driven_option_the_reader_does_not_know_is_counted",
			suite: "*** Settings ***\nLibrary    DataDriver    file=t.csv    delimiter=;\n\n" + `*** Test Cases ***
A
    [Template]    Set AQL And Execute Ad Hoc Query
    ${path}
`,
			want: []string{"unrebuilt " + reasonDataFileShape + " (file=t.csv delimiter=;)"},
		},
		{
			name: "data_file_column_the_reader_does_not_know_is_counted",
			suite: dataDriven + `*** Test Cases ***
A
    [Template]    Set AQL And Execute Ad Hoc Query
    ${path}
`,
			files: map[string]string{csvPath: "${path},[Tags]\nSELECT c FROM COMPOSITION c,not-ready\n"},
			want:  []string{"unrebuilt " + reasonDataFileShape + " (" + csvPath + ` column "[Tags]")`},
		},
		// --- Classify.
		{
			name: "refused_by_expect_error_on_a_continuation_row",
			suite: `*** Test Cases ***
T
    ${err}    Run Keyword And Expect Error    *
    ...    Set AQL And Execute Ad Hoc Query    SELECT c FROM COMPOSITION c
`,
			want: []string{"refused: SELECT c FROM COMPOSITION c"},
		},
		{
			name: "refused_by_the_status_check",
			suite: `*** Test Cases ***
T
    Set AQL And Execute Ad Hoc Query    SELECT c FROM COMPOSITION c
    Should Contain    ${err_msg}    422 != 200
`,
			want: []string{"refused: SELECT c FROM COMPOSITION c"},
		},
		{
			name: "answered_empty_by_should_be_empty",
			suite: `*** Test Cases ***
T
    Set AQL And Execute Ad Hoc Query    SELECT c FROM COMPOSITION c
    Should Be Empty    ${resp_body['rows']}
    Should Be Empty    ${diff}
`,
			want: []string{"answered-empty: SELECT c FROM COMPOSITION c"},
		},
		{
			name: "answered_empty_by_a_zero_length",
			suite: `*** Test Cases ***
A
    Set AQL And Execute Ad Hoc Query    SELECT c FROM COMPOSITION c
    Length Should Be    ${resp_body['rows']}    0
B
    Set AQL And Execute Ad Hoc Query    SELECT o FROM OBSERVATION o
    Length Should Be    ${resp_body['rows']}    ${0}
C
    Execute Query    query=SELECT e FROM EHR e    expected_rows_nr=0
D
    Set AQL And Execute Ad Hoc Query    SELECT s FROM SECTION s
    Length Should Be    ${resp_body['rows']}    2

*** Keywords ***
Execute Query
    [Arguments]    ${query}    ${expected_rows_nr}
    Set AQL And Execute Ad Hoc Query    ${query}
    Length Should Be    ${resp_body['rows']}    ${expected_rows_nr}
`,
			want: []string{
				"answered-empty: SELECT c FROM COMPOSITION c", "answered-empty: SELECT o FROM OBSERVATION o",
				"answered-empty: SELECT e FROM EHR e", "answered: SELECT s FROM SECTION s",
			},
		},
		{
			name: "assertions_apply_to_the_query_last_sent_and_only_when_they_run",
			suite: `*** Test Cases ***
T
    Set AQL And Execute Ad Hoc Query    SELECT c FROM COMPOSITION c
    IF    ${flag}
        Should Be Empty    ${resp_body['rows']}
    END
    Set AQL And Execute Ad Hoc Query    SELECT o FROM OBSERVATION o
    Should Be Empty    ${resp_body['rows']}
`,
			want: []string{"answered: SELECT c FROM COMPOSITION c", "answered-empty: SELECT o FROM OBSERVATION o"},
		},
		{
			name: "not_ready_by_an_active_tag_only",
			suite: `*** Test Cases ***
A
    [Tags]    not-ready
    Set AQL And Execute Ad Hoc Query    SELECT c FROM COMPOSITION c
B
    #[Tags]    not-ready
    Set AQL And Execute Ad Hoc Query    SELECT o FROM OBSERVATION o
`,
			want: []string{"not-ready: SELECT c FROM COMPOSITION c", "answered: SELECT o FROM OBSERVATION o"},
		},
		{
			name: "not_ready_by_suite_tags",
			suite: `*** Settings ***
Default Tags    not-ready

*** Test Cases ***
A
    Set AQL And Execute Ad Hoc Query    SELECT c FROM COMPOSITION c
B
    [Tags]    smoke
    Set AQL And Execute Ad Hoc Query    SELECT o FROM OBSERVATION o
`,
			files: map[string]string{
				"AQL/FROM/t.robot": "*** Settings ***\nForce Tags    NOT-READY\n\n*** Test Cases ***\nC\n" +
					"    Set AQL And Execute Ad Hoc Query    SELECT e FROM EHR e\n",
				"AQL/MIXED/__init__.robot": "*** Settings ***\nTest Tags    not-ready\n",
				"AQL/MIXED/t.robot":        "*** Test Cases ***\nD\n    Set AQL And Execute Ad Hoc Query    SELECT s FROM SECTION s\n",
			},
			want: []string{
				"not-ready: SELECT e FROM EHR e", "not-ready: SELECT s FROM SECTION s",
				"not-ready: SELECT c FROM COMPOSITION c", "answered: SELECT o FROM OBSERVATION o",
			},
		},
		{
			// Robot reports a test that meets an active Skip as skipped, so a
			// query it sent before the Skip is skipped too.
			name: "skipped_by_an_active_skip_only",
			suite: `*** Settings ***
#Suite Setup    Skip    commented out

*** Test Cases ***
A
    Set AQL And Execute Ad Hoc Query    SELECT c FROM COMPOSITION c
    Skip    not yet
    Set AQL And Execute Ad Hoc Query    SELECT o FROM OBSERVATION o
B
    Set AQL And Execute Ad Hoc Query    SELECT p FROM POINT_EVENT p
    IF    ${flag}
        Skip    maybe
    END
`,
			files: map[string]string{
				"AQL/FROM/t.robot": "*** Settings ***\nSuite Setup    Skip    not yet\n\n*** Test Cases ***\nC\n" +
					"    Set AQL And Execute Ad Hoc Query    SELECT e FROM EHR e\n",
				"AQL/MIXED/__init__.robot": "*** Settings ***\nSuite Setup    Skip\n",
				"AQL/MIXED/t.robot":        "*** Test Cases ***\nD\n    Set AQL And Execute Ad Hoc Query    SELECT s FROM SECTION s\n",
			},
			want: []string{
				"skipped: SELECT e FROM EHR e", "skipped: SELECT s FROM SECTION s",
				"skipped: SELECT c FROM COMPOSITION c", "skipped: SELECT o FROM OBSERVATION o",
				"answered: SELECT p FROM POINT_EVENT p",
			},
		},
	}
	// One case per escape Robot defines: each sequence makes the character it
	// names, and any other escaped character stands for itself. The text
	// sender sends the query as it is, so the rebuild shows the character; a
	// variable sigil that stands for itself starts no variable.
	for _, e := range []struct{ name, escape, char string }{
		{"newline", `\n`, "\n"},
		{"carriage_return", `\r`, "\r"},
		{"tab", `\t`, "\t"},
		{"hex_byte", `\x41`, "A"},
		{"four_digit_code_point", `\u00e9`, "\u00e9"},
		{"eight_digit_code_point", `\U0001F600`, "\U0001F600"},
		{"newline_built_in", `${\n}`, "\n"},
		{"dollar", `\${x}`, "${x}"},
		{"at_sign", `\@{x}`, "@{x}"},
		{"ampersand", `\&{x}`, "&{x}"},
		{"percent", `\%{x}`, "%{x}"},
		{"hash", `\#`, "#"},
		{"equals", `\=`, "="},
		{"pipe", `\|`, "|"},
		{"backslash", `\\`, `\`},
		{"space", `\ `, " "},
		{"hex_byte_without_its_digits", `\xZ1`, "xZ1"},
		{"code_point_beyond_unicode", `\U00110000`, "U00110000"},
	} {
		cases = append(cases, rebuildCase{
			name: "escape_" + e.name,
			suite: "*** Test Cases ***\nT\n    Send Ad Hoc Request Through GET    " +
				"SELECT c FROM COMPOSITION c WHERE c/name/value = 'a" + e.escape + "b'\n",
			want: []string{"answered: SELECT c FROM COMPOSITION c WHERE c/name/value = 'a" + e.char + "b'"},
		})
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			files := maps.Clone(tc.files)
			if files == nil {
				files = map[string]string{}
			}
			files["AQL/WHERE/t.robot"] = tc.suite
			c, err := readSuiteCorpus(suiteFixture(files))
			if err != nil {
				t.Fatalf("readSuiteCorpus: %v", err)
			}
			if got := renderSuiteCorpus(c); !slices.Equal(got, tc.want) {
				t.Fatalf("rebuilt:\n  %s\nwant:\n  %s", strings.Join(got, "\n  "), strings.Join(tc.want, "\n  "))
			}
		})
	}
}

// TestProbe107ReaderLocatesEachQuery_REQ109 pins the corpus coordinate a
// failure names: the test-case row that sends the query, and for a
// data-driven test the data row too. A send an init file counts is placed at
// the init file's setting row, under the family the init file sits in, or
// under none at AQL/.
func TestProbe107ReaderLocatesEachQuery_REQ109(t *testing.T) {
	t.Parallel()
	const initSend = "*** Settings ***\n%s    Set AQL And Execute Ad Hoc Query    SELECT e FROM EHR e\n"
	c, err := readSuiteCorpus(suiteFixture(map[string]string{
		"AQL/WHERE/t.robot": "*** Test Cases ***\nT\n    Log    first\n    Run Query\n\n" +
			"*** Keywords ***\nRun Query\n    Set AQL And Execute Ad Hoc Query    SELECT c FROM COMPOSITION c\n",
		"AQL/FROM/d.robot": "*** Settings ***\nLibrary    DataDriver    file=../../_resources/d.csv\n\n" +
			"*** Test Cases ***\nD\n    [Template]    Set AQL And Execute Ad Hoc Query\n    ${q}\n",
		"_resources/d.csv":         "${q}\nSELECT o FROM OBSERVATION o\n",
		"AQL/__init__.robot":       fmt.Sprintf(initSend, "Suite Setup"),
		"AQL/WHERE/__init__.robot": fmt.Sprintf(initSend, "Suite Teardown"),
	}))
	if err != nil {
		t.Fatalf("readSuiteCorpus: %v", err)
	}
	var got []string
	for _, q := range c.Queries {
		got = append(got, q.Family+" "+q.Where())
	}
	want := []string{"FROM AQL/FROM/d.robot:7 (data _resources/d.csv:2)", "WHERE AQL/WHERE/t.robot:4"}
	if !slices.Equal(got, want) {
		t.Fatalf("coordinates = %v, want %v", got, want)
	}
	got = nil
	for _, u := range c.Unrebuilt {
		got = append(got, u.Family+" "+u.Where()+" "+u.Reason)
	}
	want = []string{
		" AQL/__init__.robot:2 " + reasonOutsideTest,
		"WHERE AQL/WHERE/__init__.robot:2 " + reasonOutsideTest,
	}
	if !slices.Equal(got, want) {
		t.Fatalf("unrebuilt coordinates = %v, want %v", got, want)
	}
}

// TestProbe107ReaderRefusesAnUnlearnedCorpus_REQ160 is the refresh arm: a
// corpus the reader has not learned is a read error, never a partial read.
func TestProbe107ReaderRefusesAnUnlearnedCorpus_REQ160(t *testing.T) {
	t.Parallel()
	const suite = "*** Test Cases ***\nT\n    Set AQL And Execute Ad Hoc Query    SELECT c FROM COMPOSITION c\n"
	cases := []struct {
		name   string
		mutate func(m fstest.MapFS)
		want   string
	}{
		{"missing_provenance_pin", func(m fstest.MapFS) { delete(m, "AQL_SOURCE.txt") }, "missing AQL_SOURCE.txt"},
		{"missing_exclusion_record", func(m fstest.MapFS) { delete(m, "EXCLUDED.txt") }, "missing EXCLUDED.txt"},
		{"missing_resources", func(m fstest.MapFS) { delete(m, "_resources") }, "missing _resources"},
		{"unknown_root_file", func(m fstest.MapFS) { m["NOTES.md"] = &fstest.MapFile{} }, `"NOTES.md" at the corpus root`},
		{"file_under_aql_that_is_no_suite", func(m fstest.MapFS) { m["AQL/WHERE/notes.txt"] = &fstest.MapFile{} }, "is not a .robot suite"},
		{"file_under_resources_that_is_no_csv", func(m fstest.MapFS) { m["_resources/x.json"] = &fstest.MapFile{} }, "is not a .csv data file"},
		{"unknown_family", func(m fstest.MapFS) { m["AQL/GROUP_BY/t.robot"] = &fstest.MapFile{Data: []byte(suite)} }, "family directory AQL/GROUP_BY"},
		{"suite_outside_a_family", func(m fstest.MapFS) { m["AQL/t.robot"] = &fstest.MapFile{Data: []byte(suite)} }, "sits outside a family directory"},
		{
			"init_file_with_tests",
			func(m fstest.MapFS) { m["AQL/WHERE/__init__.robot"] = &fstest.MapFile{Data: []byte(suite)} },
			"carries test cases",
		},
		{
			"unknown_section",
			func(m fstest.MapFS) { m["AQL/WHERE/t.robot"] = &fstest.MapFile{Data: []byte("*** Cases ***\n")} },
			`section header "*** Cases ***"`,
		},
		{
			"pipe_separated_suite",
			func(m fstest.MapFS) {
				m["AQL/WHERE/t.robot"] = &fstest.MapFile{Data: []byte("*** Test Cases ***\n" +
					"| T | Set AQL And Execute Ad Hoc Query | SELECT c FROM COMPOSITION c |\n")}
			},
			"AQL/WHERE/t.robot:2: a pipe-separated row",
		},
		{
			"pipe_separated_init_file",
			func(m fstest.MapFS) {
				m["AQL/WHERE/__init__.robot"] = &fstest.MapFile{Data: []byte("| *** Settings *** |\n" +
					"| Suite Setup | Set AQL And Execute Ad Hoc Query | SELECT c FROM COMPOSITION c |\n")}
			},
			"AQL/WHERE/__init__.robot:1: a pipe-separated row",
		},
		{
			"malformed_data_file",
			func(m fstest.MapFS) {
				m["AQL/WHERE/t.robot"] = &fstest.MapFile{Data: []byte("*** Settings ***\n" +
					"Library    DataDriver    file=../../_resources/t.csv\n\n" +
					"*** Test Cases ***\nT\n    [Template]    Set AQL And Execute Ad Hoc Query\n    ${q}\n")}
				m["_resources/t.csv"] = &fstest.MapFile{Data: []byte("${q}\na,b\n")}
			},
			"PROBE-107: read _resources/t.csv",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m := suiteFixture(map[string]string{"AQL/WHERE/t.robot": suite})
			tc.mutate(m)
			_, err := readSuiteCorpus(m)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("readSuiteCorpus err = %v, want one containing %q", err, tc.want)
			}
		})
	}
	// Premise: the fixture itself reads cleanly, so each refusal above is the
	// mutation's doing.
	if _, err := readSuiteCorpus(suiteFixture(map[string]string{"AQL/WHERE/t.robot": suite})); err != nil {
		t.Fatalf("premise: the unmutated fixture fails to read: %v", err)
	}
}

// The crafted queries the probe controls hand PROBE-107, each with the
// Error-severity lint codes it draws (asserted as a premise below).
const (
	queryClean        = "SELECT c/uid/value FROM COMPOSITION c"
	queryUnparseable  = "SELECT c FROM"
	queryUnknownAlias = "SELECT x/uid/value FROM COMPOSITION c"
	queryImpossible   = "SELECT o FROM OBSERVATION o CONTAINS COMPOSITION c"
	queryEventContext = "SELECT ec/start_time/value FROM EHR e CONTAINS COMPOSITION c CONTAINS EVENT_CONTEXT ec"
	queryUnknownClass = "SELECT t FROM COMPOSITION CONTAINS FOO_BAR t"
	// queryOtherOperand binds a not-containable operand other than
	// EVENT_CONTEXT: the decline must not cover it.
	queryOtherOperand = "SELECT x FROM COMPOSITION c CONTAINS ISM_TRANSITION x"
	// queryEventContextOtherCode draws, besides the declined code, a second
	// Error whose issue names EVENT_CONTEXT: the decline must not cover it.
	queryEventContextOtherCode = "SELECT ec FROM EHR e CONTAINS EVENT_CONTEXT ec[openEHR-EHR-OBSERVATION.x.v1]"
)

// TestProbe107QueryPremises_REQ161 asserts what lint raises on the crafted queries,
// so a control below cannot pass because its query stopped drawing the code
// it was chosen for.
func TestProbe107QueryPremises_REQ161(t *testing.T) {
	t.Parallel()
	cases := []struct {
		query string
		// errs is the Error-severity code multiset; warn a Warning the query
		// must also raise; classes, when set, the RM class each Error names.
		errs    []string
		warn    string
		classes []string
	}{
		{query: queryClean},
		{query: queryUnparseable, errs: []string{"aql_syntax"}},
		{query: queryUnknownAlias, errs: []string{"aql_unknown_alias"}},
		{query: queryImpossible, errs: []string{codeImpossibleContainment}},
		{query: queryEventContext, errs: []string{codeContainsNotContainable}, classes: []string{"EVENT_CONTEXT"}},
		{query: queryUnknownClass, warn: codeUnknownRMClass},
		{query: queryOtherOperand, errs: []string{codeContainsNotContainable}, classes: []string{"ISM_TRANSITION"}},
		{
			query: queryEventContextOtherCode, errs: []string{codeContainsNotContainable, codeArchetypeClassMismatch},
			classes: []string{"EVENT_CONTEXT", "EVENT_CONTEXT"},
		},
	}
	for _, tc := range cases {
		var errs, warns, classes []string
		for _, i := range lint.LintString(tc.query, nil).Issues {
			if i.Severity == lint.Error {
				errs = append(errs, i.Code)
				classes = append(classes, issueClass(i))
			} else {
				warns = append(warns, i.Code)
			}
		}
		if !slices.Equal(errs, tc.errs) {
			t.Errorf("lint(%q) Errors = %v, want %v", tc.query, errs, tc.errs)
		}
		if tc.classes != nil && !slices.Equal(classes, tc.classes) {
			t.Errorf("lint(%q) Error classes = %v, want %v", tc.query, classes, tc.classes)
		}
		if tc.warn != "" && !slices.Contains(warns, tc.warn) {
			t.Errorf("lint(%q) Warnings = %v, want them to include %s", tc.query, warns, tc.warn)
		}
	}
}

// TestProbe107HoldsEachClassToItsRule_REQ109_REQ160 is the control for every
// guard of PROBE-107 Answered, Answered-empty, Not asserted and Ratchet: each
// case hands the probe a crafted corpus, decline table and family list, and
// removing the guard it names flips the verdict.
func TestProbe107HoldsEachClassToItsRule_REQ109_REQ160(t *testing.T) {
	t.Parallel()
	q := func(class SuiteClass, query string) SuiteQuery {
		return SuiteQuery{Family: "WHERE", Suite: "AQL/WHERE/t.robot", Line: 7, Class: class, Query: query}
	}
	clean := q(SuiteAnswered, queryClean)
	eventContext := q(SuiteAnswered, queryEventContext)
	syntaxDecline := suiteDecline{
		Code: "aql_syntax", Construct: "a truncated FROM", Reason: declineGrammarExtension,
		Covers: func(query string, _ lint.Issue) bool { return query == queryUnparseable },
	}
	withDecline := func(d suiteDecline) []suiteDecline { return []suiteDecline{d} }
	tweak := func(edit func(*suiteDecline)) []suiteDecline {
		d := suiteDeclines[0]
		edit(&d)
		return []suiteDecline{d}
	}
	where := []string{"WHERE"}
	cases := []struct {
		name     string
		queries  []SuiteQuery
		declines []suiteDecline
		families []string
		pass     bool
		detail   string
	}{
		{name: "clean_answered_query_passes", queries: []SuiteQuery{clean}, families: where, pass: true},
		{
			name: "answered_query_that_does_not_parse_fails", queries: []SuiteQuery{clean, q(SuiteAnswered, queryUnparseable)},
			families: where, detail: "AQL/WHERE/t.robot:7: an answered query does not parse",
		},
		{
			name: "answered_query_drawing_an_error_fails", queries: []SuiteQuery{clean, q(SuiteAnswered, queryUnknownAlias)},
			families: where, detail: "draws Error-severity [aql_unknown_alias] and no decline covers it",
		},
		{
			name: "answered_query_drawing_a_warning_passes", queries: []SuiteQuery{clean, q(SuiteAnswered, queryUnknownClass)},
			families: where, pass: true,
		},
		{
			name: "declined_error_passes", queries: []SuiteQuery{eventContext},
			declines: suiteDeclines, families: where, pass: true,
		},
		{
			name: "undeclined_event_context_fails", queries: []SuiteQuery{eventContext},
			families: where, detail: "draws Error-severity [" + codeContainsNotContainable + "]",
		},
		{
			name: "decline_covers_only_its_own_construct", queries: []SuiteQuery{eventContext, q(SuiteAnswered, queryImpossible)},
			declines: suiteDeclines, families: where, detail: "draws Error-severity [" + codeImpossibleContainment + "]",
		},
		{
			// The decline names its operand: the same code on another
			// not-containable operand is not covered.
			name: "decline_does_not_cover_another_operand", queries: []SuiteQuery{eventContext, q(SuiteAnswered, queryOtherOperand)},
			declines: suiteDeclines, families: where, detail: "draws Error-severity [" + codeContainsNotContainable + "]",
		},
		{
			// The decline names its code: another Error on the same operand is
			// not covered.
			name: "decline_does_not_cover_another_code_on_its_operand", queries: []SuiteQuery{q(SuiteAnswered, queryEventContextOtherCode)},
			declines: suiteDeclines, families: where, detail: "draws Error-severity [" + codeArchetypeClassMismatch + "]",
		},
		{
			name: "grammar_extension_decline_covers_a_refused_parse", queries: []SuiteQuery{clean, q(SuiteAnswered, queryUnparseable)},
			declines: withDecline(syntaxDecline), families: where, pass: true,
		},
		{
			name: "decline_covering_no_query_fails", queries: []SuiteQuery{clean},
			declines: suiteDeclines, families: where, detail: "covers no answered query",
		},
		{
			name: "decline_without_a_code_fails", queries: []SuiteQuery{clean, eventContext},
			declines: tweak(func(d *suiteDecline) { d.Code = "" }), families: where, detail: "names no Error code",
		},
		{
			name: "decline_without_a_construct_fails", queries: []SuiteQuery{eventContext},
			declines: tweak(func(d *suiteDecline) { d.Construct = " " }), families: where, detail: "names no construct",
		},
		{
			name: "decline_with_a_third_reason_fails", queries: []SuiteQuery{eventContext},
			declines: tweak(func(d *suiteDecline) { d.Reason = "the engine answers it" }), families: where,
			detail: `gives the reason "the engine answers it"`,
		},
		{
			name: "decline_without_a_matcher_fails", queries: []SuiteQuery{clean},
			declines: tweak(func(d *suiteDecline) { d.Covers = nil }), families: where, detail: "has no matcher",
		},
		{
			name: "answered_empty_containment_error_passes", queries: []SuiteQuery{clean, q(SuiteAnsweredEmpty, queryImpossible)},
			families: where, pass: true,
		},
		{
			name: "answered_empty_query_that_does_not_parse_fails", queries: []SuiteQuery{clean, q(SuiteAnsweredEmpty, queryUnparseable)},
			families: where, detail: "an answered-empty query does not parse",
		},
		{
			name: "answered_empty_other_error_fails", queries: []SuiteQuery{clean, q(SuiteAnsweredEmpty, queryUnknownAlias)},
			families: where, detail: "an answered-empty query draws Error-severity [aql_unknown_alias]",
		},
		{
			name: "refused_not_ready_and_skipped_are_never_asserted",
			queries: []SuiteQuery{
				clean, q(SuiteRefused, queryUnparseable), q(SuiteNotReady, queryUnknownAlias), q(SuiteSkipped, queryImpossible),
			},
			families: where, pass: true,
		},
		{
			name: "unknown_class_fails", queries: []SuiteQuery{clean, q("answered-maybe", queryClean)},
			families: where, detail: `class "answered-maybe" is none of`,
		},
		{
			name: "family_with_no_answered_query_fails", queries: []SuiteQuery{clean, {Family: "FROM", Class: SuiteRefused, Query: queryClean}},
			families: []string{"WHERE", "FROM"}, detail: "family FROM: no answered query",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r, err := probe107(SuiteCorpus{Queries: tc.queries}, tc.declines, tc.families)
			if err != nil {
				t.Fatalf("probe107: %v", err)
			}
			if got := r.Status == "pass"; got != tc.pass {
				t.Fatalf("status = %q (detail %q), want pass=%v", r.Status, r.Detail, tc.pass)
			}
			if !strings.Contains(r.Detail, tc.detail) {
				t.Fatalf("detail = %q, want it to contain %q", r.Detail, tc.detail)
			}
		})
	}
}

// TestProbe107RequiresANonEmptyCorpus_REQ160 is the control for the
// whole-corpus tripwire: no query at all is an error, never a vacuous pass.
func TestProbe107RequiresANonEmptyCorpus_REQ160(t *testing.T) {
	t.Parallel()
	r, err := Probe107SuiteCorpus(SuiteCorpus{})
	if err == nil || !strings.Contains(err.Error(), "rebuilt no query at all") {
		t.Fatalf("err = %v (status %q), want the empty-corpus error", err, r.Status)
	}
	if r.Status != "" || r.Probe != "PROBE-107" {
		t.Fatalf("result = %+v, want probe PROBE-107 with no verdict", r)
	}
}

// TestProbe107FamilyTripwireAsWired_REQ160 is the control for the family
// tripwire as the exported probe wires it: the shipped corpus with one
// family's answered queries reclassed fails, naming that family.
func TestProbe107FamilyTripwireAsWired_REQ160(t *testing.T) {
	t.Parallel()
	c := probe107Corpus(t)
	moved := SuiteCorpus{Queries: slices.Clone(c.Queries), Unrebuilt: c.Unrebuilt}
	n := 0
	for i, q := range moved.Queries {
		if q.Family == "META" && q.Class == SuiteAnswered {
			moved.Queries[i].Class = SuiteRefused
			n++
		}
	}
	if n == 0 {
		t.Fatal("premise: the META family contributes no answered query to reclass")
	}
	r, err := Probe107SuiteCorpus(moved)
	if err != nil {
		t.Fatalf("Probe107SuiteCorpus: %v", err)
	}
	const want = "family META: no answered query"
	if r.Status != "fail" || !strings.Contains(r.Detail, want) {
		t.Fatalf("Probe107SuiteCorpus with META's %d answered queries reclassed: status = %q, detail = %q, want a fail naming %q",
			n, r.Status, r.Detail, want)
	}
}

// TestProbe107ReaderKnowsEachKeywordItCalls_REQ109 holds every entry of the
// reader's keyword census to what the entry says: a sender it rebuilds sends
// its query, a sender it cannot rebuild is counted under its reason, and a
// keyword that sends no query leaves the query last sent open, so the
// assertion after it still classes that query.
func TestProbe107ReaderKnowsEachKeywordItCalls_REQ109(t *testing.T) {
	t.Parallel()
	const query = "SELECT c FROM COMPOSITION c"
	const sent = "    Set AQL And Execute Ad Hoc Query    " + query + "\n"
	const empty = "    Should Be Empty    ${resp_body['rows']}\n"
	at := regexp.MustCompile(`^[a-z0-9_.]+\.robot:[1-9][0-9]*$`)
	type keywordCase struct {
		name, body string
		want       []string
	}
	inert := func(name string) keywordCase {
		return keywordCase{name: name, body: sent + "    " + name + "\n" + empty, want: []string{"answered-empty: " + query}}
	}
	var cases []keywordCase
	for name, k := range upstreamKeywords {
		if !at.MatchString(k.at) {
			t.Errorf("upstreamKeywords[%q].at = %q, want file.robot:LINE", name, k.at)
		}
		switch k.send {
		case sendsNoQuery, runsStoredQuery:
			cases = append(cases, inert(name))
		case sendsUpstreamText:
			cases = append(cases, keywordCase{
				name, sent + "    " + name + "\n" + empty,
				[]string{"answered: " + query, "unrebuilt " + reasonQueryNotVendored + " (" + name + ")"},
			})
		case sendsUnrebuilt:
			cases = append(cases, keywordCase{
				name, sent + "    " + name + "\n" + empty,
				[]string{"answered: " + query, "unrebuilt " + reasonSendNotRebuilt + " (" + name + ")"},
			})
		case sendsText, sendsAdHocBody, sendsStoredBody:
			cases = append(cases, keywordCase{name, "    " + name + "    " + query + "\n", []string{"answered: " + query}})
		case sendsJSONBody:
			cases = append(cases, keywordCase{name, "    " + name + `    {"q": "` + query + `"}` + "\n", []string{"answered: " + query}})
		case sendsPayload:
			cases = append(cases, keywordCase{
				name, "    Set Test Variable    ${payload}    " + query + "\n    " + name + "\n",
				[]string{"answered: " + query},
			})
		default:
			t.Errorf("upstreamKeywords[%q]: send form %d has no case here; teach this test what it sends", name, k.send)
		}
	}
	for name := range inertLibraryKeywords {
		if _, dup := upstreamKeywords[name]; dup {
			t.Errorf("%q is both an upstream keyword and an inert library keyword", name)
		}
		cases = append(cases, inert(name))
	}
	for name, names := range requestsVerbs {
		if !slices.Contains(names, "url") {
			t.Errorf("requestsVerbs[%q] = %v names no url argument", name, names)
		}
		if _, dup := upstreamKeywords[name]; dup {
			t.Errorf("%q is both an upstream keyword and a RequestsLibrary verb", name)
		}
		if _, dup := inertLibraryKeywords[name]; dup {
			t.Errorf("%q is both an inert library keyword and a RequestsLibrary verb", name)
		}
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c, err := readSuiteCorpus(suiteFixture(map[string]string{"AQL/WHERE/t.robot": "*** Test Cases ***\nT\n" + tc.body}))
			if err != nil {
				t.Fatalf("readSuiteCorpus: %v", err)
			}
			if got := renderSuiteCorpus(c); !slices.Equal(got, tc.want) {
				t.Fatalf("calling %q rebuilt:\n  %s\nwant:\n  %s", tc.name, strings.Join(got, "\n  "), strings.Join(tc.want, "\n  "))
			}
		})
	}
}

// TestProbe107ShippedDeclinesAreWellFormed_REQ109 holds the shipped decline
// table to the rule each decline must meet: an Error code, a construct, and
// one of the two reasons.
func TestProbe107ShippedDeclinesAreWellFormed_REQ109(t *testing.T) {
	t.Parallel()
	for _, d := range suiteDeclines {
		if msg := d.malformed(); msg != "" {
			t.Errorf("shipped decline: %s", msg)
		}
	}
}
