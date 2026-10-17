package aqlprobes

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"slices"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/aql/lint"
	"github.com/cadasto/openehr-sdk-go/openehr/aql/parse"
)

// PROBE-107 — the REQ-109 + REQ-160 upstream AQL suite admissibility corpus.
//
// PROBE-100 holds the REQ-160 relation against the FROM-family combination
// rows, rebuilt with templates transcribed into its reader. This probe widens
// the same position to every clause family: the vendored corpus under
// testkit/corpus/aql/suites/ carries the upstream Robot AQL suites themselves,
// so the reader takes each query from the suite that sends it and transcribes
// nothing (probe_107_robot.go is that reader).
//
// Each rebuilt query is classed by what its test asserts (REQ-109/REQ-160
// § PROBE-107 Classify). Only two classes are asserted: an answered query
// must parse under the SDK grammar profile and draw no Error-severity lint
// issue unless a reviewed decline covers it, and an answered-empty query
// must parse and may draw only REQ-161's containment Errors, which an engine
// answering with no rows is consistent with (REQ-160 § Overlay edges).
// Refused, not-ready and skipped queries are counted and never asserted: the
// SDK stays the looser of the two, and the corpus is not a rejection
// catalogue.
//
// Severity is read from lint.Issue.Severity alone, as in PROBE-100, so a
// re-grading in the REQ-161 catalogue flows through instead of drifting
// against a copy kept here.

// SuiteClass is what a suite test asserts about the query it sends.
type SuiteClass string

// The classes a rebuilt query falls into. Only answered and answered-empty
// queries are asserted by [Probe107SuiteCorpus]; the other three are counted.
const (
	// SuiteAnswered is a query whose test reads the rows the engine returns.
	SuiteAnswered SuiteClass = "answered"
	// SuiteAnsweredEmpty is a query whose test asserts that no row comes back.
	SuiteAnsweredEmpty SuiteClass = "answered-empty"
	// SuiteRefused is a query whose test expects an error response.
	SuiteRefused SuiteClass = "refused"
	// SuiteNotReady is a query whose test or suite carries an active
	// not-ready tag.
	SuiteNotReady SuiteClass = "not-ready"
	// SuiteSkipped is a query of a skipped suite or test.
	SuiteSkipped SuiteClass = "skipped"
)

// suiteClasses is every class, in report order.
var suiteClasses = []SuiteClass{SuiteAnswered, SuiteAnsweredEmpty, SuiteRefused, SuiteNotReady, SuiteSkipped}

// SuiteQuery is one query the vendored suites send, rebuilt from the suites
// alone.
type SuiteQuery struct {
	// Family is the clause-family directory directly under AQL/.
	Family string
	// Suite is the .robot file, relative to the corpus root (AQL/…).
	Suite string
	// Line is the 1-based line of the test-case row that sends the query,
	// directly or through a keyword the suite defines. For a data-driven
	// suite it is the template row, and Data names the data row.
	Line int
	// Data is the data-file row a data-driven test was filled from, as
	// `path:LINE` relative to the corpus root, or "" for any other test.
	Data string
	// Class is what the test asserts about the query.
	Class SuiteClass
	// Query is the rebuilt query text.
	Query string
}

// Where is the query's corpus coordinate: `suite:LINE`, followed by the data
// row for a data-driven test. It is enough to open the test from a CI log.
func (q SuiteQuery) Where() string {
	return suiteWhere(q.Suite, q.Line, q.Data)
}

// SuiteUnrebuilt is one query the reader could not rebuild. It is counted
// under its reason, never dropped.
type SuiteUnrebuilt struct {
	// Family, Suite, Line and Data locate the test or suite, as on
	// [SuiteQuery].
	Family string
	Suite  string
	Line   int
	Data   string
	// Reason is the tag of the rule that stopped the rebuild, and Why is that
	// rule in prose, so a caller logging the tally explains itself.
	Reason string
	Why    string
	// Detail names what the rule met: a variable, a keyword or a file.
	Detail string
}

// Where is the corpus coordinate of the test or suite that could not be
// rebuilt.
func (u SuiteUnrebuilt) Where() string {
	return suiteWhere(u.Suite, u.Line, u.Data)
}

func suiteWhere(suite string, line int, data string) string {
	w := fmt.Sprintf("%s:%d", suite, line)
	if data != "" {
		w += " (data " + data + ")"
	}
	return w
}

// SuiteCorpus is one whole read of the vendored suite corpus: every query a
// suite sends, either rebuilt into Queries or counted in Unrebuilt.
type SuiteCorpus struct {
	Queries   []SuiteQuery
	Unrebuilt []SuiteUnrebuilt
}

// suiteFamilies are the clause-family directories under AQL/ at the pin
// AQL_SOURCE.txt records. The reader refuses a family directory this list does
// not name, and the probe fails a listed family that contributes no answered
// query (PROBE-107 Ratchet), so a refresh that adds, empties or loses a family
// stops the probe until the list and the pin move together.
var suiteFamilies = []string{
	"AGGREGATE_FUNCTIONS", "EXTERNAL", "FOLDER", "FROM", "LIMIT", "META",
	"MIXED", "ORDER_BY", "PARAMETER", "SELECT", "VERSION", "WHERE",
}

// The two reasons a decline may give (PROBE-107 Answered). A decline records
// that the SDK refuses a query a conformant engine answers, and why that is
// not a defect: the query is outside foundation AQL (REQ-109 § Syntax floor),
// or it binds a CONTAINS operand through an engine extension (REQ-160
// § Containable operands). No third reason exists; a refusal neither covers
// is a defect in the profile or the relation.
const (
	declineGrammarExtension  = "an engine grammar extension outside foundation AQL"
	declineContainsExtension = "a CONTAINS operand an engine binds by an engine extension"
)

// suiteDecline is one reviewed decline: an Error the SDK raises on a query a
// conformant engine answers, recorded by name with its reason. Recording a
// decline does not widen the grammar profile or the relation (REQ-109
// § Syntax floor): it only stops the probe from reporting the refusal as a
// defect.
type suiteDecline struct {
	// Code is the Error-severity issue code the decline covers.
	Code string
	// Construct names, in prose, the construct the code fires on.
	Construct string
	// Reason is one of declineGrammarExtension and declineContainsExtension.
	Reason string
	// Covers reports whether the Error-severity issue i, raised on query q, is
	// the one this decline records. A query that does not parse is offered as
	// an aql_syntax issue.
	Covers func(q string, i lint.Issue) bool
}

// suiteDeclines is the decline table. Each entry needs a ruling: a decline
// widens nothing, but it does stop the probe from reporting a refusal, so an
// entry is added only for a construct the specification itself places outside
// foundation AQL or outside the RM's containable operands.
var suiteDeclines = []suiteDecline{
	{
		// EVENT_CONTEXT conforms only to PATHABLE and is reached by value
		// through COMPOSITION.context; an engine that answers it as a CONTAINS
		// operand binds it by an engine extension, and the default relation
		// still verdicts it Never (REQ-160 § Containable operands names this
		// very operand).
		Code:      codeContainsNotContainable,
		Construct: "CONTAINS operand EVENT_CONTEXT",
		Reason:    declineContainsExtension,
		Covers: func(_ string, i lint.Issue) bool {
			return i.Code == codeContainsNotContainable && issueClass(i) == "EVENT_CONTEXT"
		},
	},
}

// issueClass is the RM class a semantic issue concerns: lint.Issue.Path holds
// the class token, then its alias when there is one.
func issueClass(i lint.Issue) string {
	class, _, _ := strings.Cut(i.Path, " ")
	return class
}

// suiteContainmentErrors are REQ-161's Error-severity containment codes: the
// only Errors an answered-empty query may raise (PROBE-107 Answered-empty).
var suiteContainmentErrors = []string{
	codeImpossibleContainment, codeContainsNotContainable, codeArchetypeClassMismatch,
}

// ReadSuiteCorpus reads the vendored suite corpus rooted at root (the
// directory holding AQL/, _resources/, AQL_SOURCE.txt and EXCLUDED.txt) and
// rebuilds every query its suites send.
//
// Callers in this repository pass fixtures.AQLSuitesRoot(); the parameter is
// there so a downstream SDK can point the same reader at its own vendored
// copy.
//
// The returned error reports a corpus the reader cannot read at all: a
// missing ingest file, a file of a kind the corpus does not carry, a family
// directory the reader has not learned, a malformed suite or data file. A
// query the reader cannot rebuild is not an error: it is counted in
// [SuiteCorpus.Unrebuilt] under its reason.
func ReadSuiteCorpus(root string) (SuiteCorpus, error) {
	return readSuiteCorpus(os.DirFS(root))
}

// Probe107SuiteCorpus runs the PROBE-107 ratchet over c and aggregates every
// failure into one [Result], as [Probe100ConformanceCorpus] does.
//
// An answered query passes when it parses under the SDK grammar profile and
// draws no Error-severity lint issue, or when every Error it draws is one a
// reviewed decline records. An answered-empty query passes when it parses and
// its only Errors are containment Errors. Refused, not-ready and skipped
// queries are counted and never asserted. The probe also fails when a decline
// covers no answered query, and when a clause family contributes no answered
// query.
//
// It returns an error, with no verdict, when c holds no query at all.
func Probe107SuiteCorpus(c SuiteCorpus) (Result, error) { // PROBE-107 (REQ-109, REQ-160)
	return probe107(c, suiteDeclines, suiteFamilies)
}

// probe107 is [Probe107SuiteCorpus] over an explicit decline table and family
// list, so the controls can remove a guard's input without touching the
// package tables.
func probe107(c SuiteCorpus, declines []suiteDecline, families []string) (Result, error) {
	r := Result{Probe: "PROBE-107"}
	if len(c.Queries) == 0 {
		return r, errors.New("PROBE-107: the corpus rebuilt no query at all; " +
			"the ratchet has nothing to hold and would report pass vacuously")
	}

	var failures []string
	for _, d := range declines {
		if msg := d.malformed(); msg != "" {
			failures = append(failures, msg)
		}
	}

	covered := make([]int, len(declines))
	answered := map[string]int{}
	for _, q := range c.Queries {
		switch q.Class {
		case SuiteAnswered:
			answered[q.Family]++
			msg, by := checkSuiteAnswered(q, declines)
			if msg != "" {
				failures = append(failures, msg)
			}
			for _, i := range by {
				covered[i]++
			}
		case SuiteAnsweredEmpty:
			if msg := checkSuiteAnsweredEmpty(q); msg != "" {
				failures = append(failures, msg)
			}
		case SuiteRefused, SuiteNotReady, SuiteSkipped:
			// Counted by the caller's tally and never asserted (PROBE-107
			// Not asserted): the SDK stays the looser of the two.
		default:
			failures = append(failures, fmt.Sprintf("%s: class %q is none of %v; the reader classed it wrongly",
				q.Where(), q.Class, suiteClasses))
		}
	}

	// A decline that covers nothing records a refusal that no longer happens
	// (the corpus moved, or the SDK learned the construct); keeping it would
	// leave a stale exemption in place for the next query that needs a ruling.
	for i, d := range declines {
		if covered[i] == 0 {
			failures = append(failures, fmt.Sprintf("decline %s on %s covers no answered query; "+
				"remove it, or re-baseline the corpus that should still need it", d.Code, d.Construct))
		}
	}
	// The per-family tripwire, as in PROBE-100: a family that answers nothing
	// is a family this ratchet no longer holds the SDK to. Families come from
	// the list, not from what was read, so a family that vanished still has
	// to answer for it.
	for _, fam := range families {
		if answered[fam] == 0 {
			failures = append(failures, fmt.Sprintf("family %s: no answered query; "+
				"the ratchet no longer holds the SDK to this family", fam))
		}
	}

	if len(failures) > 0 {
		r.Status = "fail"
		r.Detail = strings.Join(failures, "; ")
		return r, nil
	}
	r.Status = "pass"
	return r, nil
}

// malformed reports why d is not a decline PROBE-107 accepts, or "": a
// decline must name the Error code, the construct it fires on and one of the
// two reasons.
func (d suiteDecline) malformed() string {
	switch {
	case d.Code == "":
		return fmt.Sprintf("decline on %q names no Error code", d.Construct)
	case strings.TrimSpace(d.Construct) == "":
		return fmt.Sprintf("decline %s names no construct", d.Code)
	case d.Reason != declineGrammarExtension && d.Reason != declineContainsExtension:
		return fmt.Sprintf("decline %s on %s gives the reason %q, which is neither %q nor %q",
			d.Code, d.Construct, d.Reason, declineGrammarExtension, declineContainsExtension)
	case d.Covers == nil:
		return fmt.Sprintf("decline %s on %s has no matcher", d.Code, d.Construct)
	}
	return ""
}

// coveringDecline is the index of the first decline in declines that covers
// issue i on query q, or -1. A decline covers only an issue of its own code.
func coveringDecline(declines []suiteDecline, q string, i lint.Issue) int {
	return slices.IndexFunc(declines, func(d suiteDecline) bool {
		return d.Covers != nil && d.Code == i.Code && d.Covers(q, i)
	})
}

// checkSuiteAnswered holds one answered query to PROBE-107 Answered. It
// returns "" when the query holds, and otherwise a message naming its corpus
// coordinate, the codes and the query; by lists the declines that covered an
// Error on it.
func checkSuiteAnswered(q SuiteQuery, declines []suiteDecline) (msg string, by []int) {
	if _, err := parse.ParseQuery(q.Query); err != nil {
		// A grammar-extension decline covers a query the profile refuses; the
		// refusal reaches it as the aql_syntax issue lint would raise.
		syntax := lint.Issue{Code: "aql_syntax", Severity: lint.Error, Detail: err.Error()}
		if i := coveringDecline(declines, q.Query, syntax); i >= 0 {
			return "", []int{i}
		}
		return fmt.Sprintf("%s: an answered query does not parse under the SDK grammar profile: %v (query %q)",
			q.Where(), err, q.Query), nil
	}
	var uncovered []string
	for _, i := range lint.LintString(q.Query, nil).Issues {
		if i.Severity != lint.Error {
			continue
		}
		if d := coveringDecline(declines, q.Query, i); d >= 0 {
			if !slices.Contains(by, d) {
				by = append(by, d)
			}
			continue
		}
		uncovered = append(uncovered, i.Code)
	}
	if len(uncovered) > 0 {
		return fmt.Sprintf("%s: an answered query draws Error-severity %v and no decline covers it — "+
			"the SDK must stay the looser of the two (query %q)", q.Where(), uncovered, q.Query), by
	}
	return "", by
}

// checkSuiteAnsweredEmpty holds one answered-empty query to PROBE-107
// Answered-empty: it parses, and any Error it raises is a containment Error.
func checkSuiteAnsweredEmpty(q SuiteQuery) string {
	if _, err := parse.ParseQuery(q.Query); err != nil {
		return fmt.Sprintf("%s: an answered-empty query does not parse under the SDK grammar profile: %v (query %q)",
			q.Where(), err, q.Query)
	}
	var other []string
	for _, i := range lint.LintString(q.Query, nil).Issues {
		if i.Severity == lint.Error && !slices.Contains(suiteContainmentErrors, i.Code) {
			other = append(other, i.Code)
		}
	}
	if len(other) > 0 {
		return fmt.Sprintf("%s: an answered-empty query draws Error-severity %v, which is not a containment Error "+
			"an empty answer is consistent with (query %q)", q.Where(), other, q.Query)
	}
	return ""
}

// suiteDeclineCoverage is the number of answered queries in c each decline
// covers, in table order: what PROBE-107 Ratchet asks the running test to
// assert.
func suiteDeclineCoverage(c SuiteCorpus, declines []suiteDecline) []int {
	n := make([]int, len(declines))
	for _, q := range c.Queries {
		if q.Class != SuiteAnswered {
			continue
		}
		_, by := checkSuiteAnswered(q, declines)
		for _, i := range by {
			n[i]++
		}
	}
	return n
}

// readSuiteCorpus is [ReadSuiteCorpus] over any file system, so the controls
// can hand it a corpus built in memory.
func readSuiteCorpus(fsys fs.FS) (SuiteCorpus, error) {
	files, err := listSuiteCorpus(fsys)
	if err != nil {
		return SuiteCorpus{}, err
	}
	r := &suiteReader{fsys: fsys, inits: map[string]*robotInherited{}}
	for _, p := range files.suites {
		if err := r.readSuite(p); err != nil {
			return SuiteCorpus{}, err
		}
	}
	return r.out, nil
}

// suiteCorpusFiles is what the corpus holds, sorted.
type suiteCorpusFiles struct {
	suites []string // AQL/**/*.robot except __init__.robot
}

// suiteRootFiles are the two files the ingest writes at the corpus root. A
// corpus without its provenance pin or exclusion record is not a corpus.
var suiteRootFiles = []string{"AQL_SOURCE.txt", "EXCLUDED.txt"}

// listSuiteCorpus walks the corpus and refuses what the ingest does not
// write: a root entry other than AQL/, _resources/ and the two ingest files, a
// file under AQL/ that is not a .robot suite, a file under _resources/ that is
// not a .csv data file, a suite outside a family directory, or a family
// directory not in [suiteFamilies].
func listSuiteCorpus(fsys fs.FS) (suiteCorpusFiles, error) {
	var out suiteCorpusFiles
	root, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return out, fmt.Errorf("PROBE-107: read corpus root: %w", err)
	}
	seen := map[string]bool{}
	for _, e := range root {
		name := e.Name()
		switch {
		case e.IsDir() && (name == "AQL" || name == "_resources"):
		case !e.IsDir() && slices.Contains(suiteRootFiles, name):
		default:
			return out, fmt.Errorf("PROBE-107: %q at the corpus root is none of AQL/, _resources/, %v; "+
				"teach the reader about it before vendoring it", name, suiteRootFiles)
		}
		seen[name] = true
	}
	for _, name := range append([]string{"AQL", "_resources"}, suiteRootFiles...) {
		if !seen[name] {
			return out, fmt.Errorf("PROBE-107: the corpus root is missing %s; the ingest writes it, "+
				"and a corpus without it is not readable", name)
		}
	}
	err = fs.WalkDir(fsys, "AQL", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("PROBE-107: walk %s: %w", p, err)
		}
		parts := strings.Split(p, "/")
		if d.IsDir() {
			if len(parts) == 2 && !slices.Contains(suiteFamilies, parts[1]) {
				return fmt.Errorf("PROBE-107: family directory %s is not one the reader knows %v; "+
					"a refresh added a family, so learn it and re-baseline the pin", p, suiteFamilies)
			}
			return nil
		}
		if !strings.HasSuffix(p, ".robot") {
			return fmt.Errorf("PROBE-107: %s is not a .robot suite; the ingest vendors suites only under AQL/", p)
		}
		if d.Name() == robotInitFile {
			return nil
		}
		if len(parts) < 3 {
			return fmt.Errorf("PROBE-107: suite %s sits outside a family directory", p)
		}
		out.suites = append(out.suites, p)
		return nil
	})
	if err != nil {
		return out, err
	}
	err = fs.WalkDir(fsys, "_resources", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("PROBE-107: walk %s: %w", p, err)
		}
		if !d.IsDir() && !strings.HasSuffix(p, ".csv") {
			return fmt.Errorf("PROBE-107: %s is not a .csv data file; the ingest vendors data files only under _resources/", p)
		}
		return nil
	})
	return out, err
}
