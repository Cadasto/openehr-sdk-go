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
)

// The Robot Framework reader behind PROBE-107.
//
// The upstream suites build each query at run time, so the reader runs just
// enough of Robot Framework to rebuild it (PROBE-107 Rebuild): it parses the
// space-separated format, keeps the variable scopes Robot keeps (local, test,
// suite), resolves variables, Robot's backslash escapes and the built-ins,
// calls the keywords a suite defines with their arguments bound, fills a
// DataDriver template from every row of the data file the suite names, and
// follows the handful of BuiltIn keywords that build text. Every keyword it
// does not define is a no-op whose assigned variables become run-time values.
//
// A run-time value is carried through the text as a reference (rtOpen, the
// variable name, rtClose) until the query is sent. A reference inside a quoted
// AQL literal is then replaced with one fixed literal, because the probe
// asserts admissibility, not results; a reference outside one leaves the query
// unrebuildable under a named reason, never dropped.
//
// The query-sending keywords live upstream in _resources/keywords/, which the
// corpus does not vendor. suiteSenders names them and the argument each takes
// the query in, from that resource at the pin AQL_SOURCE.txt records.

// robotInitFile is the directory-level settings file of a Robot suite tree.
const robotInitFile = "__init__.robot"

// suiteFixedLiteral stands in for every run-time variable inside a quoted
// literal. Upstream fills them with the EHR, composition and system ids the
// suite just created; any well-formed UUID rebuilds the same query shape, and
// a fixed one keeps the rebuild deterministic.
const suiteFixedLiteral = "11111111-1111-1111-1111-111111111111"

// The run-time reference markers: two private-use runes no suite text holds.
const (
	rtOpen  = '\uE000'
	rtClose = '\uE001'
)

func runtimeRef(name string) string { return string(rtOpen) + name + string(rtClose) }

// The reasons a query is counted rather than rebuilt (PROBE-107 Rebuild). The
// tag is what the tally keys on; suiteUnrebuiltWhy states each rule.
const (
	reasonUnquotedVariable = "unquoted-variable"
	reasonUnknownQueryText = "unknown-query-text"
	reasonUnknownSender    = "unknown-sender"
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
	reasonUnknownSender:    "a keyword that looks like it sends a query is not one the reader knows",
	reasonQueryNotText:     "the query argument is a list or a dictionary, not text",
	reasonBodyNotJSON:      "the request body is not a JSON object with a text q member",
	reasonConditionalSend:  "the query is sent inside a FOR, WHILE, IF or TRY block or a conditional Run Keyword, so whether it runs is unknown",
	reasonOutsideTest:      "the query is sent from a suite setup or teardown, outside any test",
	reasonNoTemplate:       "a data-driven suite whose test case names no template keyword or template row",
	reasonSeveralTemplates: "a data-driven suite with more than one test case or template row",
	reasonDataFileMissing:  "the data file a data-driven suite names is not in the corpus, or is named in a form the reader cannot place",
	reasonDataFileShape:    "DataDriver options or a data-file column the reader does not know",
	reasonKeywordDepth:     "user keywords nest deeper than the reader follows",
}

// robotMaxDepth bounds user-keyword nesting; a suite that recurses deeper is
// counted under reasonKeywordDepth rather than followed forever.
const robotMaxDepth = 16

// suiteSender is a keyword that sends its query to the engine.
type suiteSender struct {
	// args are the keyword's argument names, in order; a call argument
	// `name=value` naming one of them is a named argument.
	args []string
	// query is the index in args of the argument that carries the query.
	query int
	// shape says how that argument carries it.
	shape senderShape
	// kwargs is set for a keyword that also takes free `name=value`
	// arguments, so any such cell is named rather than positional.
	kwargs bool
}

type senderShape int

const (
	// senderText: the argument is the query text.
	senderText senderShape = iota
	// senderJSONBody: the argument is a JSON request body; its q member is
	// the query.
	senderJSONBody
	// senderPayload: the keyword sends the test variable ${payload}.
	senderPayload
	// senderSessionGet: a RequestsLibrary GET; when its url is /query/aql,
	// the q entry of its params dictionary is the query.
	senderSessionGet
)

// suiteSenders are the query-sending keywords the vendored suites call, keyed
// by normalised name. Each argument list is the keyword's [Arguments] row in
// the upstream resource it lives in.
var suiteSenders = map[string]suiteSender{
	robotNormalize("Set AQL And Execute Ad Hoc Query"): {args: []string{"query_expr", "parameter"}, shape: senderText},
	robotNormalize("Send Ad Hoc Request"):              {args: []string{"aql_body", "req_headers"}, shape: senderJSONBody},
	robotNormalize("Send Ad Hoc Request Through GET"):  {args: []string{"q_param", "req_headers"}, shape: senderText},
	robotNormalize("PUT /definition/query/{qualified_query_name}"): {
		args: []string{"query_to_store", "format", "multitenancy_token", "query_type"}, shape: senderText,
	},
	robotNormalize("PUT /definition/query/{qualified_query_name}/{version}"): {
		args: []string{"query_to_store", "format", "multitenancy_token"}, shape: senderText,
	},
	robotNormalize("GET /query/aql?q={query}"): {args: []string{"multitenancy_token"}, shape: senderPayload},
	robotNormalize("GET On Session"): {
		args: []string{"alias", "url", "params", "expected_status", "msg"}, query: 2, shape: senderSessionGet, kwargs: true,
	},
}

// suiteStoredQueryRunners run a query stored earlier by one of the PUT
// senders. They send no query text of their own: the query is counted where
// it was stored.
var suiteStoredQueryRunners = []string{
	robotNormalize("GET /query/{qualified_query_name}"),
	robotNormalize("POST /query/{qualified_query_name}"),
	robotNormalize("GET /query/{qualified_query_name}/{version}"),
	robotNormalize("POST /query/{qualified_query_name}/{version}"),
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
// comment-only lines never break a `...` continuation, as in Robot.
func parseRobotFile(name string, data []byte) (*robotFile, error) {
	f := &robotFile{}
	section := ""
	var stmts *[]robotStmt // the statement list rows go to
	var blocks *[]robotBlock
	for i, raw := range strings.Split(string(data), "\n") {
		n := i + 1
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
	testTemplate  string
	forceTags     []string // Force Tags and Test Tags
	defaultTags   []string
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
			s.testSetup = args
		case "testteardown", "taskteardown":
			s.testTeardown = args
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
// directory: its forced tags, and whether its suite setup skips them.
type robotInherited struct {
	tags    []string
	skipped bool
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

// robotUnescape resolves Robot's backslash escapes in literal text: a run of
// backslashes halves, and an odd one escapes what follows — \n, \r, \t and the
// \x, \u, \U code points become their characters, and any other character
// stays as itself with the backslash dropped (\$ is $, \# is #).
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
		switch s[i] {
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
			width := map[byte]int{'x': 2, 'u': 4, 'U': 8}[s[i]]
			if i+1+width <= len(s) {
				if cp, err := strconv.ParseUint(s[i+1:i+1+width], 16, 32); err == nil {
					b.WriteRune(rune(cp))
					i += 1 + width
				}
			}
		}
	}
	return b.String()
}

// robotBuiltin is the value of a built-in variable, by normalised name.
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
	case "/":
		return "/", true
	case ":":
		return ":", true
	}
	// ${0}, ${-1}, ${2.5}, ${0x1F}: Robot's number variables.
	if n, err := strconv.ParseInt(key, 10, 64); err == nil {
		return strconv.FormatInt(n, 10), true
	}
	if _, err := strconv.ParseFloat(key, 64); err == nil {
		return key, true
	}
	if len(key) > 2 && key[0] == '0' && strings.ContainsRune("xob", rune(key[1])) {
		if n, err := strconv.ParseInt(key, 0, 64); err == nil {
			return strconv.FormatInt(n, 10), true
		}
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
	skipped bool
}

// robotFrame is one keyword or test body being run.
type robotFrame struct {
	t      *robotTest
	locals map[string]robotValue
	site   int  // the test-case row this run descends from
	expect bool // inside Run Keyword And Expect Error
	cond   bool // inside a control block or a conditional Run Keyword
	depth  int
	ret    *robotValue
}

func (s *robotSuite) unrebuilt(line int, data, reason, detail string) {
	s.r.out.Unrebuilt = append(s.r.out.Unrebuilt, SuiteUnrebuilt{
		Family: s.family, Suite: s.path, Line: line, Data: data,
		Reason: reason, Why: suiteUnrebuiltWhy[reason], Detail: detail,
	})
}

// readSuite parses and runs one suite file.
func (r *suiteReader) readSuite(p string) error {
	f, err := r.parse(p)
	if err != nil {
		return err
	}
	s := &robotSuite{
		r:        r,
		path:     p,
		family:   strings.Split(p, "/")[1],
		file:     f,
		settings: readRobotSettings(f),
		keywords: map[string]*robotBlock{},
		vars:     map[string]robotValue{},
	}
	if s.inherited, err = r.inheritedFor(path.Dir(p)); err != nil {
		return err
	}
	s.skipped = s.inherited.skipped
	for i := range f.keywords {
		kw := &f.keywords[i]
		if strings.Contains(kw.name, "${") {
			e, err := newRobotEmbedded(kw)
			if err != nil {
				return fmt.Errorf("PROBE-107: %s:%d: %w", p, kw.line, err)
			}
			s.embedded = append(s.embedded, e)
			continue
		}
		if _, dup := s.keywords[robotNormalize(kw.name)]; !dup {
			s.keywords[robotNormalize(kw.name)] = kw
		}
	}
	s.initVariables()

	s.runSuiteLevel(s.settings.suiteSetup, true)
	if s.settings.dataDriver != nil {
		if err := s.runDataDriven(); err != nil {
			return err
		}
	} else {
		for i := range f.tests {
			s.runTest(&f.tests[i], "", nil)
		}
	}
	s.runSuiteLevel(s.settings.suiteTeardown, false)
	return nil
}

func (r *suiteReader) parse(p string) (*robotFile, error) {
	data, err := fs.ReadFile(r.fsys, p)
	if err != nil {
		return nil, fmt.Errorf("PROBE-107: read %s: %w", p, err)
	}
	return parseRobotFile(p, data)
}

// inheritedFor gathers what every __init__.robot from AQL/ down to dir hands
// down: forced tags accumulate, and a skipping suite setup skips everything
// below it.
func (r *suiteReader) inheritedFor(dir string) (robotInherited, error) {
	var out robotInherited
	parts := strings.Split(dir, "/")
	for i := range parts {
		d := strings.Join(parts[:i+1], "/")
		in, ok := r.inits[d]
		if !ok {
			in = &robotInherited{}
			p := d + "/" + robotInitFile
			if _, err := fs.Stat(r.fsys, p); err == nil {
				f, err := r.parse(p)
				if err != nil {
					return out, err
				}
				if len(f.tests) > 0 {
					return out, fmt.Errorf("PROBE-107: %s carries test cases; Robot refuses them in an init file", p)
				}
				st := readRobotSettings(f)
				in.tags = st.forceTags
				in.skipped = st.suiteSetup != nil && len(st.suiteSetup.cells) > 1 &&
					robotNormalize(st.suiteSetup.cells[1]) == "skip"
			}
			r.inits[d] = in
		}
		out.tags = append(out.tags, in.tags...)
		out.skipped = out.skipped || in.skipped
	}
	return out, nil
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
	if len(ts.setup) > 0 && robotNormalize(ts.setup[0]) != "none" {
		f.site = b.line
		f.call(b.line, ts.setup[0], ts.setup[1:])
	}
	switch {
	case filled != nil:
		f.site = filled.line
		f.call(filled.line, ts.template, filled.cells)
	case ts.template != "":
		for _, row := range ts.rows {
			f.site = row.line
			f.call(row.line, ts.template, row.cells)
		}
	default:
		f.runBody(ts.rows, true)
	}
	if len(ts.teardown) > 0 && robotNormalize(ts.teardown[0]) != "none" {
		f.site = b.line
		f.call(b.line, ts.teardown[0], ts.teardown[1:])
	}
	t.finish()
}

// finish classes every query the test sent (PROBE-107 Classify). Skipped wins
// over not-ready, not-ready over refused, refused over answered-empty.
func (t *robotTest) finish() {
	for _, q := range t.sent {
		class := SuiteAnswered
		switch {
		case q.skipped:
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
// often they run.
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
				f.inlineIf(st)
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
		case "RETURN":
			v := f.returnValue(st.cells[1:])
			f.ret = &v
			if depth == 0 {
				return
			}
			continue
		}
		if strings.HasPrefix(head, "[") && strings.HasSuffix(head, "]") {
			if robotNormalize(head) == "[return]" {
				v := f.returnValue(st.cells[1:])
				f.ret = &v
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

// inlineIf runs the branches of `IF  cond  Keyword  args  ELSE  Keyword …`
// conditionally.
func (f *robotFrame) inlineIf(st robotStmt) {
	saved := f.cond
	f.cond = true
	defer func() { f.cond = saved }()
	cells := st.cells[2:]
	for len(cells) > 0 {
		end := slices.IndexFunc(cells, func(c string) bool { return c == "ELSE" || c == "ELSE IF" })
		branch := cells
		if end >= 0 {
			branch = cells[:end]
		}
		if len(branch) > 0 {
			f.exec(robotStmt{line: st.line, cells: branch})
		}
		if end < 0 {
			return
		}
		if cells[end] == "ELSE IF" {
			end++ // the condition
		}
		cells = cells[min(end+1, len(cells)):]
	}
}

func (f *robotFrame) returnValue(cells []string) robotValue {
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
	if cells[0] == "VAR" {
		f.varStatement(cells[1:])
		return
	}
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

// varStatement runs Robot's VAR syntax: `VAR  ${x}  value…  scope=…`.
func (f *robotFrame) varStatement(cells []string) {
	if len(cells) == 0 {
		return
	}
	name, ok := robotTargetName(cells[0])
	if !ok {
		return
	}
	scope, sep := "local", " "
	var values []string
	for _, c := range cells[1:] {
		switch {
		case strings.HasPrefix(c, "scope="):
			scope = robotNormalize(strings.TrimPrefix(c, "scope="))
		case strings.HasPrefix(c, "separator="):
			sep = f.replaceString(strings.TrimPrefix(c, "separator="))
		default:
			values = append(values, c)
		}
	}
	var v robotValue
	switch cells[0][0] {
	case '@':
		v = robotValue{list: true}
	case '&':
		v = f.dictionary(values)
	default:
		parts := make([]string, len(values))
		for i, c := range values {
			parts[i] = f.replaceString(c)
		}
		v = textValue(strings.Join(parts, sep))
	}
	f.setScoped(scope, name, v)
}

func (f *robotFrame) setScoped(scope, name string, v robotValue) {
	key := robotNormalize(name)
	switch scope {
	case "test", "task":
		f.t.vars[key] = v
	case "suite", "global":
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
	var b strings.Builder
	pos := 0
	for {
		ref, ok := findRobotVar(s, pos)
		if !ok {
			break
		}
		b.WriteString(robotUnescape(s[pos:ref.start]))
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
	b.WriteString(robotUnescape(s[pos:]))
	return b.String()
}

// varValue is the value of one variable occurrence. A variable the reader
// cannot find is a run-time value.
func (f *robotFrame) varValue(ref robotVarRef) robotValue {
	display := ref.name
	if ref.sigil == '%' || (strings.HasPrefix(ref.name, "{") && strings.HasSuffix(ref.name, "}")) {
		return textValue(runtimeRef(display)) // environment or inline Python
	}
	name := f.replaceString(ref.name)
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
// run-time values.
func (f *robotFrame) call(line int, name string, args []string) (v robotValue, known bool) {
	key := robotNormalize(name)
	s := f.t.s
	if kw := s.keywords[key]; kw != nil {
		return f.callUser(line, kw, args, nil)
	}
	for _, e := range s.embedded {
		if m := e.re.FindStringSubmatch(name); m != nil {
			embedded := map[string]robotValue{}
			for i, n := range e.names {
				embedded[robotNormalize(n)] = f.resolve(m[i+1])
			}
			return f.callUser(line, e.kw, args, embedded)
		}
	}
	if v, known, handled := f.builtin(line, key, args); handled {
		return v, known
	}
	if sender, ok := suiteSenders[key]; ok {
		f.send(line, sender, args)
		return robotValue{}, false
	}
	if !slices.Contains(suiteStoredQueryRunners, key) && looksLikeQuerySend(key, args) {
		f.t.s.unrebuilt(f.site, f.t.data, reasonUnknownSender, name)
		f.t.open = nil
	}
	return robotValue{}, false
}

// looksLikeQuerySend is the census net for a sending keyword the reader does
// not know: a name about queries or requests, or a RequestsLibrary session
// call addressed to a query endpoint.
func looksLikeQuerySend(key string, args []string) bool {
	if strings.Contains(key, "query") || strings.Contains(key, "request") {
		return true
	}
	return strings.HasSuffix(key, "onsession") &&
		slices.ContainsFunc(args, func(a string) bool { return strings.Contains(a, "/query") })
}

// builtin runs the BuiltIn keywords that build text, scope variables, wrap
// another keyword, skip a test or classify the query just sent.
func (f *robotFrame) builtin(line int, key string, args []string) (v robotValue, known, handled bool) {
	switch key {
	case "setvariable":
		return f.returnValue(args), true, true
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
	case "runkeyword", "runkeywordandreturnstatus", "runkeywordandignoreerror",
		"runkeywordandcontinueonfailure", "runkeywordandwarnonfailure", "runkeywordandreturn":
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
// assertion that may not run (a control block, a conditional Run Keyword) or
// that is expected to fail says nothing about that query.
func (f *robotFrame) classify(mark func(*robotSent)) {
	if f.cond || f.expect || f.t.open == nil {
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

// callUser runs a keyword the suite defines, with its arguments bound as
// Robot binds them: positional first, then `name=value` for a named argument.
// Call arguments resolve in the caller's scope, defaults in the keyword's.
func (f *robotFrame) callUser(line int, kw *robotBlock, args []string, embedded map[string]robotValue) (robotValue, bool) {
	if f.depth >= robotMaxDepth {
		f.t.s.unrebuilt(f.site, f.t.data, reasonKeywordDepth, kw.name)
		return robotValue{}, false
	}
	g := &robotFrame{
		t: f.t, locals: map[string]robotValue{}, site: f.site,
		expect: f.expect, cond: f.cond, depth: f.depth + 1,
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
	g.runBody(kw.body, false)
	if g.ret != nil {
		return *g.ret, true
	}
	return robotValue{}, false
}

// send rebuilds the query a sending keyword sends and records it, or counts
// why it cannot.
func (f *robotFrame) send(line int, sender suiteSender, args []string) {
	t := f.t
	raw, found := senderArg(sender, args)
	if sender.shape == senderSessionGet {
		url, ok := senderArgAt(sender, args, 1)
		if !ok || f.replaceString(url) != "/query/aql" {
			return // a GET to another endpoint sends no query
		}
	}
	t.open = nil
	var q robotValue
	switch sender.shape {
	case senderPayload:
		var ok bool
		if q, ok = f.lookup("payload"); !ok {
			q = textValue(runtimeRef("payload"))
		}
	case senderSessionGet:
		params := robotValue{}
		if found {
			params = f.resolve(raw)
		}
		qv, ok := params.dict["q"]
		if !ok {
			t.s.unrebuilt(f.site, t.data, reasonUnknownQueryText, "params")
			return
		}
		q = qv
	case senderText, senderJSONBody:
		if !found {
			t.s.unrebuilt(f.site, t.data, reasonUnknownQueryText, sender.args[sender.query])
			return
		}
		q = f.resolve(raw)
	}
	if !q.isText() {
		t.s.unrebuilt(f.site, t.data, reasonQueryNotText, sender.args[sender.query])
		return
	}
	text := q.text
	if sender.shape == senderJSONBody {
		var body map[string]any
		if err := json.Unmarshal([]byte(text), &body); err != nil {
			t.s.unrebuilt(f.site, t.data, reasonBodyNotJSON, sender.args[sender.query])
			return
		}
		s, ok := body["q"].(string)
		if !ok {
			t.s.unrebuilt(f.site, t.data, reasonBodyNotJSON, sender.args[sender.query])
			return
		}
		text = s
	}
	f.emit(text)
}

// senderArg is the raw call argument that carries the query: the named one,
// or the positional one at its index.
func senderArg(sender suiteSender, args []string) (string, bool) {
	return senderArgAt(sender, args, sender.query)
}

func senderArgAt(sender suiteSender, args []string, index int) (string, bool) {
	pos := 0
	for _, a := range args {
		if n, v, ok := robotNamedArg(a); ok && (slices.Contains(sender.args, n) || sender.kwargs && robotIdentifier.MatchString(n)) {
			if n == sender.args[index] {
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
	if name, whole := wholeRuntimeRef(text); whole {
		t.s.unrebuilt(f.site, t.data, reasonUnknownQueryText, name)
		return
	}
	query, outside := fixRuntimeRefs(text)
	if len(outside) > 0 {
		t.s.unrebuilt(f.site, t.data, reasonUnquotedVariable, strings.Join(outside, ", "))
		return
	}
	q := &robotSent{line: f.site, query: query, refused: f.expect, skipped: t.skipped}
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
