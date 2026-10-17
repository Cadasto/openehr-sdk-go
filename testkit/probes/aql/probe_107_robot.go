package aqlprobes

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// The Robot Framework reader behind PROBE-107.
//
// The upstream suites build each query at run time, so the reader runs just
// enough of Robot Framework to rebuild it (PROBE-107 Rebuild): it parses the
// space-separated format, keeps the variable scopes Robot keeps (local, test,
// suite), resolves variables, the backslash escapes and the built-ins the
// suites use, calls the keywords a suite defines with their arguments bound,
// runs every setup and teardown (a directory init file's, a suite file's, a
// test's and a keyword's), fills a DataDriver template from every row of the
// data file the suite names, and follows the handful of BuiltIn keywords that
// build text. A Robot construct it does not run is counted where a test meets
// it, never skipped.
//
// A run-time value is carried through the text as a reference (rtOpen, the
// variable name, rtClose) until the query is sent. A reference inside a quoted
// AQL literal is then replaced with one fixed literal, because the probe
// asserts admissibility, not results; a reference outside one leaves the query
// unrebuildable under a named reason, never dropped.
//
// The keywords a suite calls without defining them live upstream, in
// _resources/keywords/ and in libraries, which the corpus does not vendor.
// upstreamKeywords is the census of those resources at the pin AQL_SOURCE.txt
// records: every keyword there that sends an AQL query, and what the reader
// does with it, plus the keywords the suites call that send none.
// inertLibraryKeywords and requestsVerbs cover the library keywords. Any other
// keyword may send a query, so a call to it is counted, never run as a no-op.

// robotInitFile is the directory-level settings file of a Robot suite tree.
const robotInitFile = "__init__.robot"

// suiteFixedLiteral stands in for every run-time variable inside a quoted
// literal. Upstream fills them with the EHR, composition and system ids the
// suite just created; any well-formed UUID rebuilds the same query shape, and
// a fixed one keeps the rebuild deterministic.
const suiteFixedLiteral = "11111111-1111-1111-1111-111111111111"

// The run-time reference markers: private-use runes no suite text holds. A
// construct the reader does not resolve travels the same way, opened by
// rtUnsupported, so a query holding one is counted rather than rebuilt wrong.
const (
	rtOpen        = '\uE000'
	rtClose       = '\uE001'
	rtUnsupported = '\uE002'
)

func runtimeRef(name string) string { return string(rtOpen) + name + string(rtClose) }

func unsupportedRef(construct string) string {
	return string(rtUnsupported) + construct + string(rtClose)
}

// The reasons a query is counted rather than rebuilt (PROBE-107 Rebuild). The
// tag is what the tally keys on; suiteUnrebuiltWhy states each rule.
const (
	reasonUnquotedVariable = "unquoted-variable"
	reasonUnknownQueryText = "unknown-query-text"
	reasonQueryNotVendored = "query-text-not-vendored"
	reasonSendNotRebuilt   = "send-not-rebuilt"
	reasonUnknownKeyword   = "unknown-keyword"
	reasonUnsupported      = "unsupported-construct"
	reasonQueryNotText     = "query-not-text"
	reasonBodyNotJSON      = "request-body-not-json"
	reasonConditionalSend  = "conditional-send"
	reasonOutsideTest      = "outside-test"
	reasonNoTemplate       = "no-template"
	reasonSeveralTemplates = "several-templates"
	reasonDataFileMissing  = "data-file-missing"
	reasonDataFileShape    = "data-file-unsupported"
	reasonKeywordDepth     = "keyword-depth"
)

// suiteUnrebuiltWhy states each unrebuildable rule in prose, for whoever meets
// it after a refresh.
var suiteUnrebuiltWhy = map[string]string{
	reasonUnquotedVariable: "a run-time variable sits outside a quoted literal, so no fixed literal can stand in for it",
	reasonUnknownQueryText: "the query text comes from a variable or keyword the reader cannot rebuild",
	reasonQueryNotVendored: "the sending keyword takes its query text from its own upstream resource or from a data file the corpus does not vendor",
	reasonSendNotRebuilt: "a request that may send a query in a form the reader does not rebuild: an upstream keyword that sends a test variable, " +
		"or a library request to a query endpoint or to a URL known only at run time",
	reasonUnknownKeyword: "a keyword the suite does not define and the reader does not know, so it may send a query",
	reasonUnsupported: "a Robot construct the reader does not run: an inline IF, VAR, RETURN or [Return], a test template outside a data-driven suite, " +
		"a built-in variable it does not resolve, or an escape naming a lone UTF-16 surrogate, which Go text cannot hold",
	reasonQueryNotText:     "the query argument is a list or a dictionary, not text",
	reasonBodyNotJSON:      "the request body, as the reader holds it, does not decode as a JSON object with a text q member; sent as it is, an engine would refuse it",
	reasonConditionalSend:  "the query is sent inside a FOR, WHILE, IF or TRY block or a conditional Run Keyword, so whether it runs is unknown",
	reasonOutsideTest:      "the query is sent from the suite setup or teardown of a suite file or of a directory's init file, outside any test",
	reasonNoTemplate:       "a data-driven suite whose test case names no template keyword or template row",
	reasonSeveralTemplates: "a data-driven suite with more than one test case or template row",
	reasonDataFileMissing:  "the data file a data-driven suite names is not in the corpus, or is named in a form the reader cannot place",
	reasonDataFileShape:    "DataDriver options or a data-file column the reader does not know",
	reasonKeywordDepth:     "user keywords nest deeper than the reader follows",
}

// robotMaxDepth bounds user-keyword nesting; a suite that recurses deeper is
// counted under reasonKeywordDepth rather than followed forever.
const robotMaxDepth = 16

// upstreamKeyword is one keyword of the upstream keyword resources, as the
// census of those resources at the pin found it.
type upstreamKeyword struct {
	// at is where the keyword is defined: its file under _resources/keywords/
	// and its line, at the pin.
	at string
	// send is what the keyword sends.
	send sendForm
	// args are the keyword's [Arguments] names, in order, for a sender the
	// reader rebuilds; the query is the first of them unless send says else.
	args []string
}

// sendForm is what an upstream keyword sends, and so what the reader does
// when a suite calls it.
type sendForm int

const (
	// sendsNoQuery: the keyword sends no AQL query. Its value is a run-time
	// value.
	sendsNoQuery sendForm = iota
	// runsStoredQuery: the keyword runs a query stored earlier and sends no
	// query text; the query is counted where it was stored.
	runsStoredQuery
	// sendsText: the first argument is the query text, sent as it is.
	sendsText
	// sendsJSONBody: the first argument is a JSON request body; the engine
	// decodes it and runs its q member.
	sendsJSONBody
	// sendsAdHocBody: the keyword wraps the first argument as the body
	// {"q":"\u2026"}, with a query_parameters member when the second argument is
	// not empty; the engine decodes that body and runs its q member.
	sendsAdHocBody
	// sendsStoredBody: the keyword stores the first argument, wrapped as the
	// body {"q":"\u2026"} when the second argument (format) is json, its default,
	// or as plain text when it is text.
	sendsStoredBody
	// sendsPayload: the keyword sends the test or suite variable ${payload}
	// as the query text.
	sendsPayload
	// sendsUpstreamText: the keyword takes its query text from its own
	// resource or from a data file the corpus does not vendor; the send is
	// counted under reasonQueryNotVendored.
	sendsUpstreamText
	// sendsUnrebuilt: the keyword sends a query from a test variable in a
	// form the reader does not rebuild; the send is counted under
	// reasonSendNotRebuilt.
	sendsUnrebuilt
)

// upstreamKeywords is the census, keyed by normalised name: every keyword in
// the upstream keyword resources at the pin that sends an AQL query, directly
// or through another keyword, and every keyword there the vendored suites
// call that sends none. A keyword that only reads, lists or deletes stored
// queries (GET or DELETE /definition/query\u2026, (admin) delete stored query,
// DELETE /query/\u2026 ADMIN) sends no AQL and no suite calls it, so it is left
// out, and a call to it would be counted.
var upstreamKeywords = map[string]upstreamKeyword{
	// The senders the reader rebuilds: the suite passes the query text.
	robotNormalize("Set AQL And Execute Ad Hoc Query"): {
		at: "aql_keywords.robot:54", send: sendsAdHocBody, args: []string{"query_expr", "parameter"},
	},
	robotNormalize("Send Ad Hoc Request"): {
		at: "aql_keywords.robot:90", send: sendsJSONBody, args: []string{"aql_body", "req_headers"},
	},
	robotNormalize("Send Ad Hoc Request Through GET"): {
		at: "aql_keywords.robot:71", send: sendsText, args: []string{"q_param", "req_headers"},
	},
	robotNormalize("PUT /definition/query/{qualified_query_name}"): {
		at: "aql_query_keywords.robot:270", send: sendsStoredBody,
		args: []string{"query_to_store", "format", "multitenancy_token", "query_type"},
	},
	robotNormalize("PUT /definition/query/{qualified_query_name}/{version}"): {
		at: "aql_query_keywords.robot:325", send: sendsStoredBody,
		args: []string{"query_to_store", "format", "multitenancy_token"},
	},
	robotNormalize("PUT AQL Query With Qualified Name And Version Multitenancy"): {
		at: "aql_query_keywords.robot:442", send: sendsStoredBody,
		args: []string{"query_to_store", "format", "multitenancy_token"},
	},
	robotNormalize("GET /query/aql?q={query}"): {at: "aql_query_keywords.robot:425", send: sendsPayload},

	// The senders whose query text is not in the vendored suites.
	robotNormalize("Execute Ad Hoc Query"):           {at: "aql_keywords.robot:38", send: sendsUpstreamText},
	robotNormalize("Get EHR Status Time Committed"):  {at: "aql_keywords.robot:290", send: sendsUpstreamText},
	robotNormalize("Get Composition Time Committed"): {at: "aql_keywords.robot:300", send: sendsUpstreamText},
	robotNormalize("Get Observation Time Committed"): {at: "aql_keywords.robot:310", send: sendsUpstreamText},
	robotNormalize("execute ad-hoc query and check result (empty DB)"): {
		at: "aql_query_keywords.robot:97", send: sendsUpstreamText,
	},
	robotNormalize("execute invalid ad-hoc query and check result (empty DB)"): {
		at: "aql_query_keywords.robot:106", send: sendsUpstreamText,
	},
	robotNormalize("execute ad-hoc query and check result (loaded DB)"): {
		at: "aql_query_keywords.robot:114", send: sendsUpstreamText,
	},
	robotNormalize("execute ad-hoc query (no result comparison)"): {
		at: "aql_query_keywords.robot:123", send: sendsUpstreamText,
	},
	robotNormalize("execute ad-hoc query"):         {at: "aql_query_keywords.robot:130", send: sendsUpstreamText},
	robotNormalize("execute invalid ad-hoc query"): {at: "aql_query_keywords.robot:138", send: sendsUpstreamText},
	robotNormalize("Execute Query And Compare Actual Result With Expected"): {
		at: "aql_query_keywords.robot:2212", send: sendsUpstreamText,
	},

	// The senders of a test variable, in a form the reader does not rebuild.
	robotNormalize("POST /query/aql"):           {at: "aql_query_keywords.robot:211", send: sendsUnrebuilt},
	robotNormalize("POST /query/aql (REST)"):    {at: "aql_query_keywords.robot:231", send: sendsUnrebuilt},
	robotNormalize("POST /query (REST) - ECIS"): {at: "aql_query_keywords.robot:256", send: sendsUnrebuilt},

	// The stored-query runners.
	robotNormalize("GET /query/{qualified_query_name}"):           {at: "aql_query_keywords.robot:533", send: runsStoredQuery},
	robotNormalize("POST /query/{qualified_query_name}"):          {at: "aql_query_keywords.robot:573", send: runsStoredQuery},
	robotNormalize("GET /query/{qualified_query_name}/{version}"): {at: "aql_query_keywords.robot:614", send: runsStoredQuery},
	robotNormalize("POST /query/{qualified_query_name}/{version} With Query Params"): {
		at: "aql_query_keywords.robot:643", send: runsStoredQuery,
	},
	robotNormalize("GET /query/{qualified_query_name}/{version} With Query Params"): {
		at: "aql_query_keywords.robot:666", send: runsStoredQuery,
	},
	robotNormalize("POST /query/{qualified_query_name}/{version}"): {at: "aql_query_keywords.robot:689", send: runsStoredQuery},

	// The keywords the vendored suites call that send no query.
	robotNormalize("Upload OPT For AQL"):                                            {at: "aql_keywords.robot:150"},
	robotNormalize("Create EHR For AQL"):                                            {at: "aql_keywords.robot:173"},
	robotNormalize("Create EHR For AQL With Custom EHR Status"):                     {at: "aql_keywords.robot:197"},
	robotNormalize("Commit Composition For AQL"):                                    {at: "aql_keywords.robot:214"},
	robotNormalize("Admin Delete EHR For AQL"):                                      {at: "aql_keywords.robot:264"},
	robotNormalize("Update EHR Status For EHR"):                                     {at: "aql_keywords.robot:277"},
	robotNormalize("Commit Contribution For AQL"):                                   {at: "aql_keywords.robot:320"},
	robotNormalize("Update Composition For AQL"):                                    {at: "aql_keywords.robot:333"},
	robotNormalize("Delete Composition For AQL"):                                    {at: "aql_keywords.robot:352"},
	robotNormalize("Create Directory For AQL"):                                      {at: "aql_keywords.robot:361"},
	robotNormalize("Get File And Replace Dynamic Vars In File And Store As String"): {at: "aql_keywords.robot:394"},
	robotNormalize("GET /ehr/ehr_id/contribution/contribution_uid"):                 {at: "contribution_keywords.robot:372"},
	robotNormalize("generate random ehr_id"):                                        {at: "ehr_keywords.robot:972"},
	robotNormalize("compare json-strings"):                                          {at: "generic_keywords.robot:59"},
	robotNormalize("compare json-string with json-file"):                            {at: "generic_keywords.robot:99"},
	robotNormalize("Set Library Search Order For Tests"):                            {at: "generic_keywords.robot:743"},
}

// inertLibraryKeywords are the library keywords the vendored suites call that
// send no query and that the reader does not follow, with their library.
// Their values are run-time values. The BuiltIn keywords the reader follows
// are in robotFrame.builtin.
var inertLibraryKeywords = map[string]string{
	robotNormalize("Log"):                               "BuiltIn",
	robotNormalize("Evaluate"):                          "BuiltIn",
	robotNormalize("Fail"):                              "BuiltIn",
	robotNormalize("Get Length"):                        "BuiltIn",
	robotNormalize("Should Be Equal"):                   "BuiltIn",
	robotNormalize("Should Be Equal As Strings"):        "BuiltIn",
	robotNormalize("Should Match Regexp"):               "BuiltIn",
	robotNormalize("Append To List"):                    "Collections",
	robotNormalize("Dictionary Should Contain Key"):     "Collections",
	robotNormalize("Dictionary Should Not Contain Key"): "Collections",
	robotNormalize("Get Dictionary Keys"):               "Collections",
	robotNormalize("List Should Contain Value"):         "Collections",
	robotNormalize("Lists Should Be Equal"):             "Collections",
	robotNormalize("Split String"):                      "String",
	robotNormalize("Create Binary File"):                "OperatingSystem",
	robotNormalize("Create File"):                       "OperatingSystem",
	robotNormalize("Get Binary File"):                   "OperatingSystem",
	robotNormalize("Get File"):                          "OperatingSystem",
	robotNormalize("Remove File"):                       "OperatingSystem",
	robotNormalize("Get Value From Json"):               "JSONLibrary",
	robotNormalize("Update Value To Json"):              "JSONLibrary",
}

// requestsVerbs are the RequestsLibrary request keywords, each with its
// leading argument names: the session alias for the On Session forms, then
// url, then params for GET On Session. A request whose url is literal text
// with no /query in it sends no query; GET On Session to /query/aql sends the
// q entry of its params dictionary; any other request is counted under
// reasonSendNotRebuilt.
var requestsVerbs = map[string][]string{
	robotNormalize("GET On Session"):     {"alias", "url", "params"},
	robotNormalize("POST On Session"):    {"alias", "url"},
	robotNormalize("PUT On Session"):     {"alias", "url"},
	robotNormalize("PATCH On Session"):   {"alias", "url"},
	robotNormalize("DELETE On Session"):  {"alias", "url"},
	robotNormalize("HEAD On Session"):    {"alias", "url"},
	robotNormalize("OPTIONS On Session"): {"alias", "url"},
	robotNormalize("GET"):                {"url"},
	robotNormalize("POST"):               {"url"},
	robotNormalize("PUT"):                {"url"},
	robotNormalize("PATCH"):              {"url"},
	robotNormalize("DELETE"):             {"url"},
	robotNormalize("HEAD"):               {"url"},
	robotNormalize("OPTIONS"):            {"url"},
}

// robotNormalize is Robot's name normalisation for keywords, variables,
// settings and tags: case-insensitive, spaces and underscores ignored.
func robotNormalize(s string) string {
	return strings.Map(func(r rune) rune {
		if r == ' ' || r == '_' {
			return -1
		}
		return r
	}, strings.ToLower(s))
}

// ---- parsing ---------------------------------------------------------------

// robotStmt is one logical row: its cells, with every `...` continuation row
// joined on and comments stripped.
type robotStmt struct {
	line  int
	cells []string
}

// robotBlock is a test case or a keyword.
type robotBlock struct {
	name string
	line int
	body []robotStmt
}

// robotFile is one parsed suite or init file.
type robotFile struct {
	settings  []robotStmt
	variables []robotStmt
	tests     []robotBlock
	keywords  []robotBlock
}

// robotSeparator is the space-separated format's cell separator: two or more
// spaces, or a tab.
var robotSeparator = regexp.MustCompile(`[ \t]{2,}|\t`)

// splitRobotRow splits one physical line into cells, dropping the comment a
// cell starting with # opens.
func splitRobotRow(line string) []string {
	line = strings.Trim(line, " \t\r")
	if line == "" {
		return nil
	}
	cells := robotSeparator.Split(line, -1)
	if i := slices.IndexFunc(cells, func(c string) bool { return strings.HasPrefix(c, "#") }); i >= 0 {
		cells = cells[:i]
	}
	return cells
}

// robotSectionKind maps a section header to the section it opens.
func robotSectionKind(header string) (string, bool) {
	switch strings.Join(strings.Fields(strings.ToLower(strings.Trim(header, "* \t"))), " ") {
	case "setting", "settings":
		return "settings", true
	case "variable", "variables":
		return "variables", true
	case "test case", "test cases", "task", "tasks":
		return "tests", true
	case "keyword", "keywords":
		return "keywords", true
	case "comment", "comments":
		return "comments", true
	}
	return "", false
}

// parseRobotFile parses one file of the space-separated format. Empty and
// comment-only lines never break a `...` continuation, as in Robot. Robot
// reads a line that starts with a pipe in the pipe-separated format; the
// reader does not parse that format, so such a line is a read error rather
// than a row read wrong.
func parseRobotFile(name string, data []byte) (*robotFile, error) {
	f := &robotFile{}
	section := ""
	var stmts *[]robotStmt // the statement list rows go to
	var blocks *[]robotBlock
	for i, raw := range strings.Split(string(data), "\n") {
		n := i + 1
		if strings.HasPrefix(raw, "|") {
			return nil, fmt.Errorf("PROBE-107: %s:%d: a pipe-separated row; the reader parses only the space-separated format", name, n)
		}
		trimmed := strings.TrimLeft(raw, " \t")
		if strings.HasPrefix(trimmed, "*") && trimmed == raw {
			first := splitRobotRow(raw)
			kind, ok := robotSectionKind(first[0])
			if !ok {
				return nil, fmt.Errorf("PROBE-107: %s:%d: section header %q is none the reader knows", name, n, first[0])
			}
			section, stmts, blocks = kind, nil, nil
			switch kind {
			case "settings":
				stmts = &f.settings
			case "variables":
				stmts = &f.variables
			case "tests":
				blocks = &f.tests
			case "keywords":
				blocks = &f.keywords
			}
			continue
		}
		cells := splitRobotRow(raw)
		if len(cells) == 0 || section == "" || section == "comments" {
			continue
		}
		indented := trimmed != raw
		if cells[0] == "..." {
			if stmts == nil || len(*stmts) == 0 {
				// A continuation of a test or keyword name row: its data
				// opens the body.
				if stmts == nil {
					return nil, fmt.Errorf("PROBE-107: %s:%d: a continuation row with nothing to continue", name, n)
				}
				*stmts = append(*stmts, robotStmt{line: n})
			}
			last := &(*stmts)[len(*stmts)-1]
			last.cells = append(last.cells, cells[1:]...)
			continue
		}
		if blocks == nil {
			*stmts = append(*stmts, robotStmt{line: n, cells: cells})
			continue
		}
		if !indented {
			*blocks = append(*blocks, robotBlock{name: cells[0], line: n})
			b := &(*blocks)[len(*blocks)-1]
			stmts = &b.body
			if len(cells) > 1 {
				*stmts = append(*stmts, robotStmt{line: n, cells: cells[1:]})
			}
			continue
		}
		if stmts == nil {
			return nil, fmt.Errorf("PROBE-107: %s:%d: a row before any test case or keyword name", name, n)
		}
		*stmts = append(*stmts, robotStmt{line: n, cells: cells})
	}
	return f, nil
}

// robotSettings are the suite settings the reader acts on.
type robotSettings struct {
	dataDriver    *robotStmt // the `Library    DataDriver` row, nil when absent
	suiteSetup    *robotStmt // the setting row; its cells after the first are the call
	suiteTeardown *robotStmt
	testSetup     []string
	testTeardown  []string
	// hasTestSetup and hasTestTeardown report whether the file has the
	// setting row at all: a row, even an empty one, overrides the default an
	// init file above hands down.
	hasTestSetup    bool
	hasTestTeardown bool
	testTemplate    string
	forceTags       []string // Force Tags and Test Tags
	defaultTags     []string
}

func readRobotSettings(f *robotFile) robotSettings {
	var s robotSettings
	for i := range f.settings {
		st := &f.settings[i]
		if len(st.cells) == 0 {
			continue
		}
		args := st.cells[1:]
		switch robotNormalize(st.cells[0]) {
		case "library":
			if len(args) > 0 && args[0] == "DataDriver" {
				s.dataDriver = st
			}
		case "suitesetup":
			s.suiteSetup = st
		case "suiteteardown":
			s.suiteTeardown = st
		case "testsetup", "tasksetup":
			s.testSetup, s.hasTestSetup = args, true
		case "testteardown", "taskteardown":
			s.testTeardown, s.hasTestTeardown = args, true
		case "testtemplate", "tasktemplate":
			if len(args) > 0 {
				s.testTemplate = args[0]
			}
		case "forcetags", "testtags", "tasktags":
			s.forceTags = append(s.forceTags, args...)
		case "defaulttags":
			s.defaultTags = append(s.defaultTags, args...)
		}
	}
	return s
}

// robotInherited is what an __init__.robot hands down to every suite under its
// directory: its forced tags, whether its suite setup skips them, and its test
// setup and teardown, the defaults of every test below it that a nearer init
// file, the suite file or the test itself does not override.
type robotInherited struct {
	tags            []string
	skipped         bool
	testSetup       []string
	testTeardown    []string
	hasTestSetup    bool
	hasTestTeardown bool
}

// ---- values and variables --------------------------------------------------

// robotValue is what a Robot variable holds, as far as the reader can know.
type robotValue struct {
	list bool
	dict map[string]robotValue
	text string // the scalar text; it may carry run-time references
}

func (v robotValue) isText() bool { return !v.list && v.dict == nil }

func textValue(s string) robotValue { return robotValue{text: s} }

// robotVarRef is one variable occurrence in a string.
type robotVarRef struct {
	start, end int // byte offsets of the whole occurrence, items included
	sigil      byte
	name       string   // between the braces, unresolved
	items      []string // the [item] accessors after the closing brace
}

// findRobotVar finds the first unescaped variable at or after from: one of
// $ @ & % followed by a braced name, plus any [item] accessors.
func findRobotVar(s string, from int) (robotVarRef, bool) {
	for i := from; i+1 < len(s); i++ {
		c := s[i]
		if !strings.ContainsRune("$@&%", rune(c)) || s[i+1] != '{' || precedingBackslashes(s, i)%2 == 1 {
			continue
		}
		end, ok := matchingClose(s, i+2, '{', '}')
		if !ok {
			continue
		}
		ref := robotVarRef{start: i, sigil: c, name: s[i+2 : end]}
		j := end + 1
		for c != '%' && j < len(s) && s[j] == '[' {
			k, ok := matchingClose(s, j+1, '[', ']')
			if !ok {
				break
			}
			ref.items = append(ref.items, s[j+1:k])
			j = k + 1
		}
		ref.end = j
		return ref, true
	}
	return robotVarRef{}, false
}

func precedingBackslashes(s string, i int) int {
	n := 0
	for i--; i >= 0 && s[i] == '\\'; i-- {
		n++
	}
	return n
}

// matchingClose is the index of the close that balances an open already seen
// before from, skipping backslash-escaped characters.
func matchingClose(s string, from int, open, closing byte) (int, bool) {
	depth := 1
	for i := from; i < len(s); i++ {
		switch s[i] {
		case '\\':
			i++
		case open:
			depth++
		case closing:
			depth--
			if depth == 0 {
				return i, true
			}
		}
	}
	return 0, false
}

// robotHexEscapes is the number of hex digits each code-point escape takes.
var robotHexEscapes = map[byte]int{'x': 2, 'u': 4, 'U': 8}

// robotUnescape resolves Robot's backslash escapes in literal text, as
// Robot's own unescaper does: a run of backslashes halves, and an odd run
// escapes what follows. \n, \r and \t make a newline, a carriage return and a
// tab, and \xhh, \uhhhh and \Uhhhhhhhh the code point their hex digits name.
// Any other escaped character stands for itself with the backslash dropped
// (\$ is $, \# is #, "\ " is a space), and so does the letter of a code-point
// escape whose hex digits are missing or name a code point beyond Unicode
// (\xZ1 is xZ1, \U00110000 is U00110000). A lone UTF-16 surrogate, which Go
// text cannot hold, becomes an unsupported reference, so a query that holds
// one is counted.
func robotUnescape(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] != '\\' {
			b.WriteByte(s[i])
			i++
			continue
		}
		j := i
		for j < len(s) && s[j] == '\\' {
			j++
		}
		run := j - i
		b.WriteString(strings.Repeat(`\`, run/2))
		i = j
		if run%2 == 0 || i >= len(s) {
			continue
		}
		switch c := s[i]; c {
		case 'n':
			b.WriteByte('\n')
			i++
		case 'r':
			b.WriteByte('\r')
			i++
		case 't':
			b.WriteByte('\t')
			i++
		case 'x', 'u', 'U':
			end := i + 1 + robotHexEscapes[c]
			if end > len(s) {
				continue
			}
			n, err := strconv.ParseUint(s[i+1:end], 16, 32)
			switch {
			case err != nil:
				continue // not all hex digits: the letter stands for itself
			case n > unicode.MaxRune:
				b.WriteString(s[i:end])
			case !utf8.ValidRune(rune(n)):
				b.WriteString(unsupportedRef(`\` + s[i:end]))
			default:
				b.WriteRune(rune(n))
			}
			i = end
		}
	}
	return b.String()
}

// robotBuiltin is the value of a built-in variable, by normalised name: the
// ones the suites use, Robot's integer variables (${0}, ${-1}), and ${\n},
// the line separator of the system that runs the suites, a newline on the
// Linux image upstream runs them in (its Dockerfile at the pin). Another
// number variable (${2.5}, ${0x1F}) and the ${/} and ${:} built-ins are not
// resolved: each is an unsupported reference, so a query that holds one is
// counted.
func robotBuiltin(key string) (string, bool) {
	switch key {
	case "empty":
		return "", true
	case "space":
		return " ", true
	case "true":
		return "True", true
	case "false":
		return "False", true
	case "none":
		return "None", true
	case `\n`:
		return "\n", true
	case "/", ":":
		return unsupportedRef("${" + key + "}"), true
	}
	if n, err := strconv.ParseInt(key, 10, 64); err == nil {
		return strconv.FormatInt(n, 10), true
	}
	if d := strings.TrimPrefix(key, "-"); d != "" && d[0] >= '0' && d[0] <= '9' {
		return unsupportedRef("${" + key + "}"), true
	}
	return "", false
}

// robotExtended splits an extended variable name (${base['key']},
// ${base.attr}) into its base and the rest, as Robot does.
var robotExtended = regexp.MustCompile(`^(.+?)([^\s\w].*)$`)

// robotDictKey is the one extended form the reader follows: a quoted key.
var robotDictKey = regexp.MustCompile(`^\[\s*(?:'([^']*)'|"([^"]*)")\s*\]$`)

// ---- the run ---------------------------------------------------------------

// suiteReader reads the corpus one suite at a time into out.
type suiteReader struct {
	fsys  fs.FS
	inits map[string]*robotInherited // by directory
	out   SuiteCorpus
}

// robotSuite is one suite file being run.
type robotSuite struct {
	r         *suiteReader
	path      string
	family    string
	file      *robotFile
	settings  robotSettings
	inherited robotInherited
	keywords  map[string]*robotBlock
	embedded  []robotEmbedded
	vars      map[string]robotValue // suite scope
	skipped   bool
}

// robotEmbedded is a keyword whose name embeds its arguments.
type robotEmbedded struct {
	kw    *robotBlock
	re    *regexp.Regexp
	names []string
}

// robotTest is one test being run; a suite setup or teardown runs as one with
// suiteLevel set.
type robotTest struct {
	s          *robotSuite
	vars       map[string]robotValue // test scope
	data       string
	notReady   bool
	skipped    bool
	suiteLevel bool
	sent       []*robotSent
	open       *robotSent // the last query sent; assertions after it apply to it
}

// robotSent is one rebuilt query, waiting for its test to finish.
type robotSent struct {
	line    int
	query   string
	refused bool
	empty   bool
}

// robotFrame is one keyword or test body being run.
type robotFrame struct {
	t      *robotTest
	locals map[string]robotValue
	site   int  // the test-case row this run descends from
	expect bool // inside Run Keyword And Expect Error
	cond   bool // inside a control block or a conditional Run Keyword
	// swallow is set inside a BuiltIn wrapper that swallows the failure of
	// what it runs (Run Keyword And Return Status, And Ignore Error, And Warn
	// On Failure): an assertion there cannot fail the test.
	swallow bool
	depth   int
}

func (s *robotSuite) unrebuilt(line int, data, reason, detail string) {
	s.r.out.Unrebuilt = append(s.r.out.Unrebuilt, SuiteUnrebuilt{
		Family: s.family, Suite: s.path, Line: line, Data: data,
		Reason: reason, Why: suiteUnrebuiltWhy[reason], Detail: detail,
	})
}

// count counts a send, or a construct that may send, under reason at the
// test-case row the frame runs, and closes the open query: an assertion after
// it may be about what it sent.
func (f *robotFrame) count(reason, detail string) {
	f.t.s.unrebuilt(f.site, f.t.data, reason, detail)
	f.t.open = nil
}

// readSuite parses and runs one suite file.
func (r *suiteReader) readSuite(p string) error {
	inherited, err := r.inheritedFor(path.Dir(p))
	if err != nil {
		return err
	}
	s, err := r.loadSuite(p)
	if err != nil {
		return err
	}
	s.inherited, s.skipped = inherited, inherited.skipped
	if !s.settings.hasTestSetup {
		s.settings.testSetup = inherited.testSetup
	}
	if !s.settings.hasTestTeardown {
		s.settings.testTeardown = inherited.testTeardown
	}

	s.runSuiteLevel(s.settings.suiteSetup, true)
	if s.settings.dataDriver != nil {
		if err := s.runDataDriven(); err != nil {
			return err
		}
	} else {
		for i := range s.file.tests {
			s.runTest(&s.file.tests[i], "", nil)
		}
	}
	s.runSuiteLevel(s.settings.suiteTeardown, false)
	return nil
}

// loadSuite parses a suite or init file and readies it to run: its keywords
// by name, and its Variables section evaluated in suite scope.
func (r *suiteReader) loadSuite(p string) (*robotSuite, error) {
	f, err := r.parse(p)
	if err != nil {
		return nil, err
	}
	s := &robotSuite{
		r:        r,
		path:     p,
		family:   suiteFamily(p),
		file:     f,
		settings: readRobotSettings(f),
		keywords: map[string]*robotBlock{},
		vars:     map[string]robotValue{},
	}
	for i := range f.keywords {
		kw := &f.keywords[i]
		if strings.Contains(kw.name, "${") {
			e, err := newRobotEmbedded(kw)
			if err != nil {
				return nil, fmt.Errorf("PROBE-107: %s:%d: %w", p, kw.line, err)
			}
			s.embedded = append(s.embedded, e)
			continue
		}
		if _, dup := s.keywords[robotNormalize(kw.name)]; !dup {
			s.keywords[robotNormalize(kw.name)] = kw
		}
	}
	s.initVariables()
	return s, nil
}

// suiteFamily is the clause-family directory a corpus file sits in, or ""
// for the init file directly under AQL/, which sits in none.
func suiteFamily(p string) string {
	if parts := strings.Split(p, "/"); len(parts) > 2 {
		return parts[1]
	}
	return ""
}

func (r *suiteReader) parse(p string) (*robotFile, error) {
	data, err := fs.ReadFile(r.fsys, p)
	if err != nil {
		return nil, fmt.Errorf("PROBE-107: read %s: %w", p, err)
	}
	return parseRobotFile(p, data)
}

// inheritedFor gathers what every __init__.robot from AQL/ down to dir hands
// down: forced tags accumulate, a skipping suite setup skips everything below
// it, and the nearest init file that sets a test setup or teardown gives the
// default.
func (r *suiteReader) inheritedFor(dir string) (robotInherited, error) {
	var out robotInherited
	parts := strings.Split(dir, "/")
	for i := range parts {
		d := strings.Join(parts[:i+1], "/")
		in, ok := r.inits[d]
		if !ok {
			var err error
			if in, err = r.readInit(d); err != nil {
				return out, err
			}
			r.inits[d] = in
		}
		out.tags = append(out.tags, in.tags...)
		out.skipped = out.skipped || in.skipped
		if in.hasTestSetup {
			out.testSetup, out.hasTestSetup = in.testSetup, true
		}
		if in.hasTestTeardown {
			out.testTeardown, out.hasTestTeardown = in.testTeardown, true
		}
	}
	return out, nil
}

// readInit reads the init file of dir, when it has one, and runs its suite
// setup and teardown with its own keywords and variables: a query either one
// sends is counted under reasonOutsideTest, once, however many suites sit
// below. Robot runs the teardown after the last suite below; the reader runs
// it straight after the setup, since nothing a suite below does reaches it.
// Its test setup and teardown are not run here: they are inherited, and run
// in each test below as that test's own.
func (r *suiteReader) readInit(dir string) (*robotInherited, error) {
	in := &robotInherited{}
	p := dir + "/" + robotInitFile
	if _, err := fs.Stat(r.fsys, p); errors.Is(err, fs.ErrNotExist) {
		return in, nil
	} else if err != nil {
		return nil, fmt.Errorf("PROBE-107: stat %s: %w", p, err)
	}
	s, err := r.loadSuite(p)
	if err != nil {
		return nil, err
	}
	if len(s.file.tests) > 0 {
		return nil, fmt.Errorf("PROBE-107: %s carries test cases; Robot refuses them in an init file", p)
	}
	in.tags = s.settings.forceTags
	in.testSetup, in.hasTestSetup = s.settings.testSetup, s.settings.hasTestSetup
	in.testTeardown, in.hasTestTeardown = s.settings.testTeardown, s.settings.hasTestTeardown
	s.runSuiteLevel(s.settings.suiteSetup, true)
	in.skipped = s.skipped
	s.runSuiteLevel(s.settings.suiteTeardown, false)
	return in, nil
}

// newRobotEmbedded compiles the name of a keyword with embedded arguments
// into a case-insensitive pattern. A custom argument pattern this regexp
// dialect cannot compile falls back to Robot's default, so a call to the
// keyword is still followed rather than lost.
func newRobotEmbedded(kw *robotBlock) (robotEmbedded, error) {
	e := robotEmbedded{kw: kw}
	compile := func(custom bool) (*regexp.Regexp, error) {
		var b strings.Builder
		b.WriteString("(?i)^")
		e.names = e.names[:0]
		pos := 0
		for {
			ref, ok := findRobotVar(kw.name, pos)
			if !ok || ref.sigil != '$' {
				break
			}
			b.WriteString(regexp.QuoteMeta(kw.name[pos:ref.start]))
			name, pattern, _ := strings.Cut(ref.name, ":")
			if pattern == "" || !custom {
				pattern = ".*?"
			}
			b.WriteString("(" + pattern + ")")
			e.names = append(e.names, name)
			pos = ref.end
		}
		b.WriteString(regexp.QuoteMeta(kw.name[pos:]) + "$")
		return regexp.Compile(b.String())
	}
	re, err := compile(true)
	if err != nil {
		re, err = compile(false)
	}
	if err != nil {
		return e, fmt.Errorf("keyword %q: %w", kw.name, err)
	}
	e.re = re
	return e, nil
}

// initVariables evaluates the Variables section in order, in suite scope. A
// scalar given several values joins them with a space, as Robot does.
func (s *robotSuite) initVariables() {
	t := &robotTest{s: s, vars: s.vars, suiteLevel: true}
	f := &robotFrame{t: t, locals: s.vars}
	for _, st := range s.file.variables {
		if len(st.cells) == 0 {
			continue
		}
		head := strings.TrimSuffix(strings.TrimSpace(st.cells[0]), "=")
		head = strings.TrimSpace(head)
		if len(head) < 3 || !strings.ContainsRune("$@&", rune(head[0])) || head[1] != '{' || !strings.HasSuffix(head, "}") {
			continue
		}
		key := robotNormalize(head[2 : len(head)-1])
		values := st.cells[1:]
		switch head[0] {
		case '$':
			parts := make([]string, len(values))
			for i, v := range values {
				parts[i] = f.replaceString(v)
			}
			s.vars[key] = textValue(strings.Join(parts, " "))
		case '@':
			s.vars[key] = robotValue{list: true}
		case '&':
			s.vars[key] = f.dictionary(values)
		}
	}
}

// runSuiteLevel runs a suite setup or teardown. A query it sends is counted
// under reasonOutsideTest, and a Skip in the setup skips the whole suite.
func (s *robotSuite) runSuiteLevel(st *robotStmt, setup bool) {
	if st == nil || len(st.cells) < 2 || robotNormalize(st.cells[1]) == "none" {
		return
	}
	t := &robotTest{s: s, vars: s.vars, suiteLevel: true}
	f := &robotFrame{t: t, locals: map[string]robotValue{}, site: st.line}
	f.call(st.line, st.cells[1], st.cells[2:])
	if setup && t.skipped {
		s.skipped = true
	}
}

// robotTestSettings are a test case's own settings.
type robotTestSettings struct {
	tags     []string
	hasTags  bool
	setup    []string
	teardown []string
	template string
	rows     []robotStmt // the body rows, settings excluded
}

func (s *robotSuite) testSettings(b *robotBlock) robotTestSettings {
	ts := robotTestSettings{setup: s.settings.testSetup, teardown: s.settings.testTeardown, template: s.settings.testTemplate}
	for _, st := range b.body {
		if len(st.cells) == 0 {
			continue
		}
		args := st.cells[1:]
		switch robotNormalize(st.cells[0]) {
		case "[tags]":
			ts.tags, ts.hasTags = append(ts.tags, args...), true
		case "[setup]":
			ts.setup = args
		case "[teardown]":
			ts.teardown = args
		case "[template]":
			ts.template = ""
			if len(args) > 0 {
				ts.template = args[0]
			}
		case "[documentation]", "[timeout]":
		default:
			ts.rows = append(ts.rows, st)
		}
	}
	if robotNormalize(ts.template) == "none" {
		ts.template = ""
	}
	return ts
}

// notReady reports whether a test with these settings carries an active
// not-ready tag, its own or its suite's.
func (s *robotSuite) notReady(ts robotTestSettings) bool {
	tags := slices.Concat(s.inherited.tags, s.settings.forceTags)
	if ts.hasTags {
		tags = append(tags, ts.tags...)
	} else {
		tags = append(tags, s.settings.defaultTags...)
	}
	return slices.ContainsFunc(tags, func(tag string) bool { return robotNormalize(tag) == "not-ready" })
}

// runTest runs one test case. A data-driven test passes the data row it was
// filled from as data, and its template row's cells already filled as args.
func (s *robotSuite) runTest(b *robotBlock, data string, filled *robotStmt) {
	ts := s.testSettings(b)
	t := &robotTest{s: s, vars: map[string]robotValue{}, data: data, notReady: s.notReady(ts), skipped: s.skipped}
	f := &robotFrame{t: t, locals: map[string]robotValue{}}
	f.site = b.line
	f.runFixture(b.line, ts.setup)
	switch {
	case filled != nil:
		f.site = filled.line
		f.call(filled.line, ts.template, filled.cells)
	case ts.template != "":
		// A test template outside a data-driven suite: the reader runs none
		// of its rows.
		f.site = b.line
		f.count(reasonUnsupported, "[Template]")
	default:
		f.runBody(ts.rows, true)
	}
	f.site = b.line
	f.runFixture(b.line, ts.teardown)
	t.finish()
}

// runFixture runs the call a setup or teardown setting names, or nothing
// when it names no keyword or NONE.
func (f *robotFrame) runFixture(line int, call []string) {
	if len(call) == 0 || robotNormalize(call[0]) == "none" {
		return
	}
	f.call(line, call[0], call[1:])
}

// finish classes every query the test sent (PROBE-107 Classify). Skipped wins
// over not-ready, not-ready over refused, refused over answered-empty. A test
// that meets an active Skip is reported skipped, so every query it sent is
// skipped, the ones sent before the Skip included.
func (t *robotTest) finish() {
	for _, q := range t.sent {
		class := SuiteAnswered
		switch {
		case t.skipped:
			class = SuiteSkipped
		case t.notReady:
			class = SuiteNotReady
		case q.refused:
			class = SuiteRefused
		case q.empty:
			class = SuiteAnsweredEmpty
		}
		t.s.r.out.Queries = append(t.s.r.out.Queries, SuiteQuery{
			Family: t.s.family, Suite: t.s.path, Line: q.line, Data: t.data, Class: class, Query: q.query,
		})
	}
}

// runBody runs a test or keyword body. Rows inside a FOR, WHILE, IF or TRY
// block run once, flagged conditional: the reader cannot know whether or how
// often they run. An inline IF, VAR, RETURN and [Return] are counted, not run.
func (f *robotFrame) runBody(body []robotStmt, testLevel bool) {
	depth := 0
	for _, st := range body {
		if len(st.cells) == 0 {
			continue
		}
		head := st.cells[0]
		if testLevel && depth == 0 {
			f.site = st.line
		}
		switch head {
		case "FOR":
			f.loopVariables(st.cells[1:])
			depth++
			continue
		case "WHILE", "TRY":
			depth++
			continue
		case "IF":
			if len(st.cells) > 2 { // an inline IF: its branches are on this row
				f.count(reasonUnsupported, "inline IF")
			} else {
				depth++
			}
			continue
		case "END":
			if depth > 0 {
				depth--
			}
			continue
		case "ELSE", "ELSE IF", "EXCEPT", "FINALLY", "BREAK", "CONTINUE":
			continue
		case "VAR", "RETURN":
			f.count(reasonUnsupported, head)
			continue
		}
		if strings.HasPrefix(head, "[") && strings.HasSuffix(head, "]") {
			if robotNormalize(head) == "[return]" {
				f.count(reasonUnsupported, head)
			}
			continue
		}
		saved := f.cond
		f.cond = f.cond || depth > 0
		f.exec(st)
		f.cond = saved
	}
}

func (f *robotFrame) loopVariables(header []string) {
	for _, c := range header {
		if strings.HasPrefix(c, "IN") {
			return
		}
		if name, ok := robotTargetName(c); ok {
			f.locals[robotNormalize(name)] = textValue(runtimeRef(name))
		}
	}
}

// setVariableValue is the value Set Variable gives: one value keeps its kind,
// several make a list, none is empty text.
func (f *robotFrame) setVariableValue(cells []string) robotValue {
	switch len(cells) {
	case 0:
		return textValue("")
	case 1:
		return f.resolve(cells[0])
	}
	return robotValue{list: true}
}

// robotTargetName is the variable name an assignment cell names: ${x}, ${x}=,
// @{x} or &{x}.
func robotTargetName(cell string) (string, bool) {
	c := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(cell), "="))
	if len(c) < 3 || !strings.ContainsRune("$@&", rune(c[0])) || c[1] != '{' {
		return "", false
	}
	end, ok := matchingClose(c, 2, '{', '}')
	if !ok || end != len(c)-1 {
		return "", false
	}
	return c[2:end], true
}

// exec runs one row: its assignment targets, then its keyword call.
func (f *robotFrame) exec(st robotStmt) {
	cells := st.cells
	var targets []string
	for len(cells) > 0 {
		name, ok := robotTargetName(cells[0])
		if !ok {
			break
		}
		targets = append(targets, name)
		cells = cells[1:]
	}
	if len(cells) == 0 {
		return
	}
	v, known := f.call(st.line, cells[0], cells[1:])
	for _, name := range targets {
		value := v
		if !known || len(targets) > 1 {
			value = textValue(runtimeRef(name))
		}
		f.locals[robotNormalize(name)] = value
	}
}

// setScoped sets a variable in the scope a Set … Variable keyword names.
func (f *robotFrame) setScoped(scope, name string, v robotValue) {
	key := robotNormalize(name)
	switch scope {
	case "test":
		f.t.vars[key] = v
	case "suite":
		f.t.s.vars[key] = v
	default:
		f.locals[key] = v
	}
}

// lookup finds a variable in the scopes Robot searches: local, test, suite.
func (f *robotFrame) lookup(name string) (robotValue, bool) {
	key := robotNormalize(name)
	for _, scope := range []map[string]robotValue{f.locals, f.t.vars, f.t.s.vars} {
		if v, ok := scope[key]; ok {
			return v, true
		}
	}
	if s, ok := robotBuiltin(key); ok {
		return textValue(s), true
	}
	return robotValue{}, false
}

// resolve is Robot's scalar replacement: a value that is exactly one variable
// keeps that variable's value, list or dictionary included; anything else is
// text with every variable replaced and the escapes resolved.
func (f *robotFrame) resolve(s string) robotValue {
	if ref, ok := findRobotVar(s, 0); ok && ref.start == 0 && ref.end == len(s) {
		return f.varValue(ref)
	}
	return textValue(f.replaceString(s))
}

// replaceString replaces every variable in s with its text and resolves the
// escapes in the literal text between them.
func (f *robotFrame) replaceString(s string) string {
	return f.replaceVariables(s, robotUnescape)
}

// replaceName replaces every variable inside a variable's name and keeps the
// name's escapes, as Robot does when it looks a variable up: ${\n} is the
// built-in named backslash-n, not a variable named by a newline.
func (f *robotFrame) replaceName(s string) string {
	return f.replaceVariables(s, func(literal string) string { return literal })
}

// replaceVariables replaces every variable in s with its text and passes the
// literal text between them through literal.
func (f *robotFrame) replaceVariables(s string, literal func(string) string) string {
	var b strings.Builder
	pos := 0
	for {
		ref, ok := findRobotVar(s, pos)
		if !ok {
			break
		}
		b.WriteString(literal(s[pos:ref.start]))
		v := f.varValue(ref)
		if v.isText() {
			b.WriteString(v.text)
		} else {
			// Robot would print the list or dictionary; that text is not
			// rebuildable, so it travels as a run-time reference.
			b.WriteString(runtimeRef(s[ref.start:ref.end]))
		}
		pos = ref.end
	}
	b.WriteString(literal(s[pos:]))
	return b.String()
}

// varValue is the value of one variable occurrence. A variable the reader
// cannot find is a run-time value.
func (f *robotFrame) varValue(ref robotVarRef) robotValue {
	display := ref.name
	if ref.sigil == '%' || (strings.HasPrefix(ref.name, "{") && strings.HasSuffix(ref.name, "}")) {
		return textValue(runtimeRef(display)) // environment or inline Python
	}
	name := f.replaceName(ref.name)
	v, ok := f.lookup(name)
	if !ok {
		v, ok = f.extended(name)
	}
	if !ok {
		return textValue(runtimeRef(display))
	}
	for _, item := range ref.items {
		display += "[" + item + "]"
		next, found := robotValue{}, false
		if v.dict != nil {
			next, found = v.dict[f.replaceString(item)]
		}
		if !found {
			return textValue(runtimeRef(display))
		}
		v = next
	}
	return v
}

// extended follows Robot's extended variable syntax for a dictionary key,
// ${base['key']}; any other extended form is a run-time value.
func (f *robotFrame) extended(name string) (robotValue, bool) {
	m := robotExtended.FindStringSubmatch(name)
	if m == nil {
		return robotValue{}, false
	}
	base, ok := f.lookup(m[1])
	if !ok {
		return robotValue{}, false
	}
	if k := robotDictKey.FindStringSubmatch(m[2]); k != nil && base.dict != nil {
		if v, ok := base.dict[k[1]+k[2]]; ok {
			return v, true
		}
	}
	return textValue(runtimeRef(name)), true
}

// dictionary builds a dictionary from `key=value` cells, as Create
// Dictionary and a &{dict} variable row do.
func (f *robotFrame) dictionary(cells []string) robotValue {
	d := map[string]robotValue{}
	for _, c := range cells {
		if k, v, ok := robotNamedArg(c); ok {
			d[f.replaceString(k)] = f.resolve(v)
			continue
		}
		if v := f.resolve(c); v.dict != nil {
			maps.Copy(d, v.dict)
		}
	}
	return robotValue{dict: d}
}

// robotNamedArg splits `name=value` at the first unescaped equals sign.
func robotNamedArg(cell string) (name, value string, ok bool) {
	for i := 0; i < len(cell); i++ {
		if cell[i] == '=' && precedingBackslashes(cell, i)%2 == 0 {
			return cell[:i], cell[i+1:], i > 0
		}
	}
	return "", "", false
}

// call runs one keyword call and returns its value; known is false when the
// reader cannot know the value, and the assigned variables then hold
// run-time values. A keyword is, in this order, one the suite defines, a
// BuiltIn the reader follows, an upstream keyword of the census, a
// RequestsLibrary request or an inert library keyword; any other is counted.
func (f *robotFrame) call(line int, name string, args []string) (v robotValue, known bool) {
	key := robotNormalize(name)
	s := f.t.s
	if kw := s.keywords[key]; kw != nil {
		f.callUser(kw, args, nil)
		return robotValue{}, false
	}
	for _, e := range s.embedded {
		if m := e.re.FindStringSubmatch(name); m != nil {
			embedded := map[string]robotValue{}
			for i, n := range e.names {
				embedded[robotNormalize(n)] = f.resolve(m[i+1])
			}
			f.callUser(e.kw, args, embedded)
			return robotValue{}, false
		}
	}
	if v, known, handled := f.builtin(line, key, args); handled {
		return v, known
	}
	if k, ok := upstreamKeywords[key]; ok {
		f.upstream(name, k, args)
		return robotValue{}, false
	}
	if names, ok := requestsVerbs[key]; ok {
		f.request(name, names, args)
		return robotValue{}, false
	}
	if _, ok := inertLibraryKeywords[key]; !ok {
		f.count(reasonUnknownKeyword, name)
	}
	return robotValue{}, false
}

// builtin runs the BuiltIn keywords that build text, scope variables, wrap
// another keyword, skip a test or classify the query just sent.
func (f *robotFrame) builtin(line int, key string, args []string) (v robotValue, known, handled bool) {
	switch key {
	case "setvariable":
		return f.setVariableValue(args), true, true
	case "catenate":
		sep := " "
		if len(args) > 0 && strings.HasPrefix(args[0], "SEPARATOR=") {
			sep, args = f.replaceString(strings.TrimPrefix(args[0], "SEPARATOR=")), args[1:]
		}
		parts := make([]string, len(args))
		for i, a := range args {
			parts[i] = f.replaceString(a)
		}
		return textValue(strings.Join(parts, sep)), true, true
	case "replacevariables":
		if len(args) == 0 {
			return robotValue{}, false, true
		}
		if v := f.resolve(args[0]); v.isText() {
			return textValue(f.replaceString(v.text)), true, true
		}
		return robotValue{}, false, true
	case "createdictionary":
		return f.dictionary(args), true, true
	case "createlist":
		return robotValue{list: true}, true, true
	case "settestvariable", "settaskvariable":
		f.setVariable("test", args)
		return robotValue{}, false, true
	case "setsuitevariable", "setglobalvariable":
		f.setVariable("suite", args)
		return robotValue{}, false, true
	case "setlocalvariable":
		f.setVariable("local", args)
		return robotValue{}, false, true
	case "runkeywordandexpecterror":
		if len(args) > 1 {
			inner := f.with(true, f.cond)
			inner.call(line, args[1], args[2:])
		}
		return robotValue{}, false, true
	case "runkeywordandreturnstatus", "runkeywordandignoreerror", "runkeywordandwarnonfailure":
		// The wrapper swallows a failure, so an assertion inside it classes
		// nothing; a query sent inside it is sent as usual, and a Skip inside
		// it still skips, since Robot lets a skip through these wrappers.
		if len(args) > 0 {
			g := *f
			g.swallow = true
			g.call(line, args[0], args[1:])
		}
		return robotValue{}, false, true
	case "runkeyword", "runkeywordandcontinueonfailure", "runkeywordandreturn":
		if len(args) > 0 {
			f.call(line, args[0], args[1:])
		}
		return robotValue{}, false, true
	case "runkeywords":
		f.runKeywords(line, args)
		return robotValue{}, false, true
	case "runkeywordif", "runkeywordunless", "runkeywordandreturnif":
		f.runKeywordIf(line, args)
		return robotValue{}, false, true
	case "waituntilkeywordsucceeds":
		if len(args) > 2 {
			f.call(line, args[2], args[3:])
		}
		return robotValue{}, false, true
	case "skip":
		// A Skip that may not run (a control block, a conditional Run
		// Keyword) skips nothing the reader can count on.
		if !f.cond && !f.expect {
			f.t.skipped = true
		}
		return robotValue{}, false, true
	case "shouldbeempty":
		if len(args) > 0 && robotRowsRef(args[0]) {
			f.classify(func(q *robotSent) { q.empty = true })
		}
		return robotValue{}, false, true
	case "lengthshouldbe":
		if len(args) > 1 && robotRowsRef(args[0]) {
			if n := f.resolve(args[1]); n.isText() && strings.TrimSpace(n.text) == "0" {
				f.classify(func(q *robotSent) { q.empty = true })
			}
		}
		return robotValue{}, false, true
	case "shouldcontain":
		if len(args) > 1 && robotRefusedStatus.MatchString(strings.TrimSpace(f.replaceString(args[1]))) {
			f.classify(func(q *robotSent) { q.refused = true })
		}
		return robotValue{}, false, true
	}
	return robotValue{}, false, false
}

// robotRefusedStatus is the error message a test checks for when it expects
// the engine to answer with an error status instead of 200.
var robotRefusedStatus = regexp.MustCompile(`^[1-5][0-9][0-9] != 200$`)

// robotRowsRefs are the spellings of the returned rows a test asserts on,
// normalised.
var robotRowsRefs = []string{
	`${respbody['rows']}`, `${respbody["rows"]}`, `${respbody}[rows]`,
	`${respbodyactual['rows']}`, `${respbodyactual["rows"]}`, `${respbodyactual}[rows]`,
	`${respbodyrows}`,
}

func robotRowsRef(cell string) bool {
	return slices.Contains(robotRowsRefs, robotNormalize(cell))
}

// classify applies an assertion to the query last sent in the test. An
// assertion that may not run (a control block, a conditional Run Keyword),
// that is expected to fail, or whose failure a wrapper swallows says nothing
// about that query.
func (f *robotFrame) classify(mark func(*robotSent)) {
	if f.cond || f.expect || f.swallow || f.t.open == nil {
		return
	}
	mark(f.t.open)
}

func (f *robotFrame) with(expect, cond bool) *robotFrame {
	g := *f
	g.expect, g.cond = f.expect || expect, cond
	return &g
}

// runKeywords runs `Run Keywords  kw  args  AND  kw  args`, or, without AND,
// each cell as a keyword of its own.
func (f *robotFrame) runKeywords(line int, args []string) {
	if !slices.Contains(args, "AND") {
		for _, a := range args {
			f.call(line, a, nil)
		}
		return
	}
	for len(args) > 0 {
		end := slices.Index(args, "AND")
		if end < 0 {
			end = len(args)
		}
		if end > 0 {
			f.call(line, args[0], args[1:end])
		}
		args = args[min(end+1, len(args)):]
	}
}

// runKeywordIf runs every branch of `Run Keyword If  cond  kw  args  ELSE IF
// cond  kw  args  ELSE  kw  args`, conditionally.
func (f *robotFrame) runKeywordIf(line int, args []string) {
	g := f.with(false, true)
	if len(args) == 0 {
		return
	}
	args = args[1:]
	for len(args) > 0 {
		end := slices.IndexFunc(args, func(c string) bool { return c == "ELSE" || c == "ELSE IF" })
		branch := args
		if end >= 0 {
			branch = args[:end]
		}
		if len(branch) > 0 {
			g.call(line, branch[0], branch[1:])
		}
		if end < 0 {
			return
		}
		if args[end] == "ELSE IF" {
			end++
		}
		args = args[min(end+1, len(args)):]
	}
}

// setVariable runs Set Test, Suite and Local Variable: the name may be
// written ${x}, \${x} or $x; one value keeps its kind, several make a list,
// and none promotes the variable's current value.
func (f *robotFrame) setVariable(scope string, args []string) {
	if len(args) == 0 {
		return
	}
	raw := strings.TrimPrefix(args[0], `\`)
	if len(raw) > 1 && strings.ContainsRune("$@&", rune(raw[0])) && raw[1] != '{' {
		raw = raw[:1] + "{" + raw[1:] + "}"
	}
	name, ok := robotTargetName(raw)
	if !ok {
		return
	}
	values := slices.DeleteFunc(slices.Clone(args[1:]), func(c string) bool { return strings.HasPrefix(c, "children=") })
	var v robotValue
	switch len(values) {
	case 0:
		if v, ok = f.lookup(name); !ok {
			v = textValue(runtimeRef(name))
		}
	case 1:
		v = f.resolve(values[0])
	default:
		v = robotValue{list: true}
	}
	if f.t.suiteLevel && scope == "test" {
		scope = "suite"
	}
	f.setScoped(scope, name, v)
}

// robotArgSpec is one [Arguments] cell of a user keyword.
type robotArgSpec struct {
	name     string
	def      string
	hasDef   bool
	variadic bool // @{args}
	kwargs   bool // &{kwargs}
}

func robotArgSpecs(kw *robotBlock) []robotArgSpec {
	var specs []robotArgSpec
	for _, st := range kw.body {
		if len(st.cells) == 0 || robotNormalize(st.cells[0]) != "[arguments]" {
			continue
		}
		for _, c := range st.cells[1:] {
			target, def, hasDef := c, "", false
			if i := strings.Index(c, "}="); i >= 0 {
				target, def, hasDef = c[:i+1], c[i+2:], true
			}
			name, ok := robotTargetName(target)
			if !ok {
				continue
			}
			specs = append(specs, robotArgSpec{
				name: name, def: def, hasDef: hasDef, variadic: c[0] == '@', kwargs: c[0] == '&',
			})
		}
	}
	return specs
}

// robotKeywordFixtures are the [Setup] and [Teardown] rows of a user keyword,
// nil when it has none.
func robotKeywordFixtures(kw *robotBlock) (setup, teardown *robotStmt) {
	for i := range kw.body {
		st := &kw.body[i]
		if len(st.cells) == 0 {
			continue
		}
		switch robotNormalize(st.cells[0]) {
		case "[setup]":
			setup = st
		case "[teardown]":
			teardown = st
		}
	}
	return setup, teardown
}

// callUser runs a keyword the suite defines, with its arguments bound as
// Robot binds them: positional first, then `name=value` for a named argument.
// Call arguments resolve in the caller's scope, defaults in the keyword's.
// Its own [Setup] runs before its body and its [Teardown] after it, wherever
// the rows sit, in the keyword's scope. The keyword's value is a run-time
// value: RETURN is counted, not run.
func (f *robotFrame) callUser(kw *robotBlock, args []string, embedded map[string]robotValue) {
	if f.depth >= robotMaxDepth {
		f.t.s.unrebuilt(f.site, f.t.data, reasonKeywordDepth, kw.name)
		return
	}
	g := &robotFrame{
		t: f.t, locals: map[string]robotValue{}, site: f.site,
		expect: f.expect, cond: f.cond, swallow: f.swallow, depth: f.depth + 1,
	}
	maps.Copy(g.locals, embedded)
	specs := robotArgSpecs(kw)
	names := map[string]bool{}
	for _, sp := range specs {
		names[sp.name] = true
	}
	var positional []string
	named := map[string]string{}
	for _, a := range args {
		if n, v, ok := robotNamedArg(a); ok && names[n] {
			named[n] = v
			continue
		}
		positional = append(positional, a)
	}
	for _, sp := range specs {
		key := robotNormalize(sp.name)
		switch {
		case sp.variadic:
			g.locals[key] = robotValue{list: true}
			positional = nil
		case sp.kwargs:
			g.locals[key] = robotValue{dict: map[string]robotValue{}}
		case len(positional) > 0:
			g.locals[key] = f.resolve(positional[0])
			positional = positional[1:]
		default:
			if raw, ok := named[sp.name]; ok {
				g.locals[key] = f.resolve(raw)
			} else if sp.hasDef {
				g.locals[key] = g.resolve(sp.def)
			} else {
				g.locals[key] = textValue(runtimeRef(sp.name))
			}
		}
	}
	setup, teardown := robotKeywordFixtures(kw)
	if setup != nil {
		g.runFixture(setup.line, setup.cells[1:])
	}
	g.runBody(kw.body, false)
	if teardown != nil {
		g.runFixture(teardown.line, teardown.cells[1:])
	}
}

// upstream runs a call to a keyword of the census: it rebuilds the query a
// sender the reader knows sends, counts a send it cannot rebuild, and does
// nothing for a keyword that sends no query text.
func (f *robotFrame) upstream(name string, k upstreamKeyword, args []string) {
	switch k.send {
	case sendsNoQuery, runsStoredQuery:
	case sendsUpstreamText:
		f.count(reasonQueryNotVendored, name)
	case sendsUnrebuilt:
		f.count(reasonSendNotRebuilt, name)
	case sendsText, sendsPayload:
		if text, ok := f.queryText(k, args); ok {
			f.emit(text)
		}
	case sendsJSONBody:
		if text, ok := f.queryText(k, args); ok {
			f.emitBody(k.args[0], text)
		}
	case sendsAdHocBody:
		text, ok := f.queryText(k, args)
		if !ok {
			return
		}
		body := `{"q":"` + text + `"`
		if raw, found := argAt(k.args, false, args, 1); found {
			if p := f.replaceString(raw); p != "" {
				body += `,"query_parameters":` + p
			}
		}
		f.emitBody(k.args[0], body+"}")
	case sendsStoredBody:
		text, ok := f.queryText(k, args)
		if !ok {
			return
		}
		format := "json"
		if raw, found := argAt(k.args, false, args, 1); found {
			format = f.replaceString(raw)
		}
		switch format {
		case "json":
			f.emitBody(k.args[0], `{"q":"`+text+`"}`)
		case "text":
			f.emit(text)
		default:
			// Upstream builds a body only for json and text; any other format
			// leaves it unset, and the keyword fails before it sends.
			f.count(reasonUnknownQueryText, "format")
		}
	}
}

// queryText is the text a sender is called with, or false once it has counted
// why there is none. It closes the open query: an assertion after the call is
// about what the call sends.
func (f *robotFrame) queryText(k upstreamKeyword, args []string) (string, bool) {
	f.t.open = nil
	arg, q := "payload", robotValue{}
	if k.send == sendsPayload {
		q = f.testOrSuite(arg)
	} else {
		arg = k.args[0]
		raw, found := argAt(k.args, false, args, 0)
		if !found {
			f.count(reasonUnknownQueryText, arg)
			return "", false
		}
		q = f.resolve(raw)
	}
	if !q.isText() {
		f.count(reasonQueryNotText, arg)
		return "", false
	}
	return q.text, true
}

// testOrSuite is the value of a test or suite variable. A keyword resource
// reads such a variable from those two scopes only: a variable local to its
// caller is not visible to it, so it is a run-time value.
func (f *robotFrame) testOrSuite(name string) robotValue {
	key := robotNormalize(name)
	for _, scope := range []map[string]robotValue{f.t.vars, f.t.s.vars} {
		if v, ok := scope[key]; ok {
			return v
		}
	}
	return textValue(runtimeRef(name))
}

// emitBody sends the q member of a JSON request body, as an engine decodes
// it, or counts a body that is not a JSON object with a text q member under
// arg, the argument that carried the query.
func (f *robotFrame) emitBody(arg, body string) {
	var v map[string]any
	if err := json.Unmarshal([]byte(body), &v); err == nil {
		if q, ok := v["q"].(string); ok {
			f.emit(q)
			return
		}
	}
	f.count(reasonBodyNotJSON, arg)
}

// request runs a RequestsLibrary request (PROBE-107 Rebuild): a GET On
// Session to /query/aql sends the q entry of its params dictionary, a request
// whose url is literal text with no /query in it sends no query, and any
// other request is counted, since it may send one.
func (f *robotFrame) request(name string, names, args []string) {
	raw, _ := argAt(names, true, args, slices.Index(names, "url"))
	url := f.replaceString(raw)
	switch {
	case robotNormalize(name) == robotNormalize("GET On Session") && url == "/query/aql":
		f.t.open = nil
		raw, found := argAt(names, true, args, slices.Index(names, "params"))
		q, ok := robotValue{}, false
		if found {
			q, ok = f.resolve(raw).dict["q"]
		}
		switch {
		case !ok:
			f.count(reasonUnknownQueryText, "params")
		case !q.isText():
			f.count(reasonQueryNotText, "params")
		default:
			f.emit(q.text)
		}
	case !strings.ContainsAny(url, string([]rune{rtOpen, rtUnsupported})) && !strings.Contains(url, "/query"):
	default:
		f.count(reasonSendNotRebuilt, name)
	}
}

// argAt is the raw call argument bound to the parameter at index in names: the
// one passed as `name=value`, or the positional one at that index. With
// kwargs, any `identifier=value` cell is a named argument, as for a keyword
// that takes free named arguments.
func argAt(names []string, kwargs bool, args []string, index int) (string, bool) {
	pos := 0
	for _, a := range args {
		if n, v, ok := robotNamedArg(a); ok && (slices.Contains(names, n) || kwargs && robotIdentifier.MatchString(n)) {
			if n == names[index] {
				return v, true
			}
			continue
		}
		if pos == index {
			return a, true
		}
		pos++
	}
	return "", false
}

// robotIdentifier is a free keyword argument's name.
var robotIdentifier = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// emit records a rebuilt query on the test, or counts why it cannot.
func (f *robotFrame) emit(text string) {
	t := f.t
	if t.suiteLevel {
		t.s.unrebuilt(f.site, t.data, reasonOutsideTest, "")
		return
	}
	if f.cond {
		t.s.unrebuilt(f.site, t.data, reasonConditionalSend, "")
		return
	}
	if _, rest, found := strings.Cut(text, string(rtUnsupported)); found {
		construct, _, _ := strings.Cut(rest, string(rtClose))
		t.s.unrebuilt(f.site, t.data, reasonUnsupported, construct)
		return
	}
	if name, whole := wholeRuntimeRef(text); whole {
		t.s.unrebuilt(f.site, t.data, reasonUnknownQueryText, name)
		return
	}
	query, outside := fixRuntimeRefs(text)
	if len(outside) > 0 {
		t.s.unrebuilt(f.site, t.data, reasonUnquotedVariable, strings.Join(outside, ", "))
		return
	}
	q := &robotSent{line: f.site, query: query, refused: f.expect}
	t.sent = append(t.sent, q)
	t.open = q
}

// wholeRuntimeRef reports whether text is nothing but one run-time reference:
// the reader then holds no query text at all.
func wholeRuntimeRef(text string) (string, bool) {
	s := strings.TrimSpace(text)
	if len(s) < 2 || []rune(s)[0] != rtOpen || strings.IndexRune(s, rtClose) != len(s)-len(string(rtClose)) {
		return "", false
	}
	return strings.Trim(s, string([]rune{rtOpen, rtClose})), true
}

// fixRuntimeRefs replaces every run-time reference inside a quoted AQL
// literal with suiteFixedLiteral, and returns the names of those outside one.
func fixRuntimeRefs(text string) (string, []string) {
	if !strings.ContainsRune(text, rtOpen) {
		return text, nil
	}
	var b strings.Builder
	var outside []string
	var quote rune
	rs := []rune(text)
	for i := 0; i < len(rs); i++ {
		r := rs[i]
		switch {
		case r == rtOpen:
			end := slices.Index(rs[i:], rtClose)
			if end < 0 {
				end = len(rs) - i
			}
			name := string(rs[i+1 : i+end])
			if quote != 0 {
				b.WriteString(suiteFixedLiteral)
			} else {
				outside = append(outside, name)
			}
			i += end
			continue
		case quote != 0 && r == '\\' && i+1 < len(rs):
			b.WriteRune(r)
			i++
			r = rs[i]
		case quote != 0 && r == quote:
			quote = 0
		case quote == 0 && (r == '\'' || r == '"'):
			quote = r
		}
		b.WriteRune(r)
	}
	return b.String(), outside
}

// ---- data-driven suites ----------------------------------------------------

// runDataDriven runs a DataDriver suite: its single template test, filled
// from every row of the data file the suite names (PROBE-107 Rebuild). A
// template row cell naming a data-file column takes that row's cell, which
// Robot then resolves like any argument.
func (s *robotSuite) runDataDriven() error {
	dd := s.settings.dataDriver
	file, problem := s.dataFile(dd.cells[2:])
	switch {
	case problem == reasonDataFileShape:
		s.unrebuilt(dd.line, "", reasonDataFileShape, strings.Join(dd.cells[2:], " "))
		return nil
	case problem != "":
		s.unrebuilt(dd.line, "", problem, file)
		return nil
	}
	if len(s.file.tests) > 1 {
		s.unrebuilt(s.file.tests[1].line, "", reasonSeveralTemplates, fmt.Sprintf("%d test cases", len(s.file.tests)))
		return nil
	}
	if len(s.file.tests) == 0 {
		s.unrebuilt(dd.line, "", reasonNoTemplate, "no test case")
		return nil
	}
	b := &s.file.tests[0]
	ts := s.testSettings(b)
	switch {
	case ts.template == "" || len(ts.rows) == 0:
		s.unrebuilt(b.line, "", reasonNoTemplate, b.name)
		return nil
	case len(ts.rows) > 1:
		s.unrebuilt(ts.rows[1].line, "", reasonSeveralTemplates, fmt.Sprintf("%d template rows", len(ts.rows)))
		return nil
	}
	header, rows, err := readSuiteDataFile(s.r.fsys, file)
	if err != nil {
		return err
	}
	if i := slices.IndexFunc(header, func(h string) bool {
		name, ok := robotTargetName(h)
		return !ok || h[0] != '$' || name == ""
	}); i >= 0 {
		s.unrebuilt(dd.line, "", reasonDataFileShape, fmt.Sprintf("%s column %q", file, header[i]))
		return nil
	}
	template := ts.rows[0]
	for _, row := range rows {
		filled := robotStmt{line: template.line, cells: make([]string, len(template.cells))}
		for i, c := range template.cells {
			filled.cells[i] = c
			if col := slices.Index(header, c); col >= 0 {
				filled.cells[i] = row.cells[col]
			}
		}
		s.runTest(b, fmt.Sprintf("%s:%d", file, row.line), &filled)
	}
	return nil
}

// dataFile places the data file a DataDriver row names, relative to the
// corpus root. It returns a reason tag when it cannot.
func (s *robotSuite) dataFile(options []string) (file, problem string) {
	const projectRoot = "${PROJECT_ROOT}/tests/robot/"
	for _, o := range options {
		k, v, ok := robotNamedArg(o)
		switch {
		case !ok:
			return "", reasonDataFileShape
		case k == "file":
			file = v
		case k == "dialect" && v == "excel":
		case k == "encoding" && (strings.EqualFold(v, "utf-8") || strings.EqualFold(v, "utf_8")):
		default:
			return "", reasonDataFileShape
		}
	}
	switch {
	case file == "":
		return "", reasonDataFileMissing
	case strings.HasPrefix(file, projectRoot):
		file = path.Clean(strings.TrimPrefix(file, projectRoot))
	case !strings.Contains(file, "${") && !path.IsAbs(file):
		file = path.Join(path.Dir(s.path), file)
	default:
		return file, reasonDataFileMissing
	}
	if !fs.ValidPath(file) {
		return file, reasonDataFileMissing
	}
	if _, err := fs.Stat(s.r.fsys, file); err != nil {
		return file, reasonDataFileMissing
	}
	return file, ""
}

// suiteDataRow is one data row of a data file.
type suiteDataRow struct {
	line  int
	cells []string
}

// readSuiteDataFile reads a DataDriver data file in the excel dialect: comma
// separated, double-quoted, every row as wide as the header. Fully empty rows
// are left out, as DataDriver leaves them out.
func readSuiteDataFile(fsys fs.FS, p string) ([]string, []suiteDataRow, error) {
	data, err := fs.ReadFile(fsys, p)
	if err != nil {
		return nil, nil, fmt.Errorf("PROBE-107: read %s: %w", p, err)
	}
	r := csv.NewReader(strings.NewReader(strings.TrimPrefix(string(data), "\uFEFF")))
	header, err := r.Read()
	if err != nil {
		return nil, nil, fmt.Errorf("PROBE-107: read the header of %s: %w", p, err)
	}
	var rows []suiteDataRow
	for {
		record, err := r.Read()
		if errors.Is(err, io.EOF) {
			return header, rows, nil
		}
		if err != nil {
			return nil, nil, fmt.Errorf("PROBE-107: read %s: %w", p, err)
		}
		if !slices.ContainsFunc(record, func(c string) bool { return c != "" }) {
			continue
		}
		line, _ := r.FieldPos(0)
		rows = append(rows, suiteDataRow{line: line, cells: record})
	}
}
