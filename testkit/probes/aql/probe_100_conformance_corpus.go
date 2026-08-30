package aqlprobes

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/aql/lint"
	"github.com/cadasto/openehr-sdk-go/openehr/aql/parse"
)

// PROBE-100 — the REQ-160 upstream admissibility corpus ratchet.
//
// The vendored corpus under testkit/cassettes/aql/conformance/ is the EHRbase
// Robot integration tests' FROM-family combination data: each CSV row is one
// FROM/CONTAINS shape a conformant engine demonstrably accepted and answered.
// This probe holds the REQ-160 relation to the looser-of-the-two position
// against that corpus rather than against a maintainer-kept row list — every
// row MUST parse under the SDK grammar profile and MUST NOT draw an
// Error-severity REQ-161 containment code.
//
// What the corpus stores is NOT a query. Upstream keeps the varying part of a
// query in the CSV and the invariant part in the consuming .robot suite, which
// substitutes the row into its own template. [conformanceSuites] is that
// mapping, transcribed verbatim; the reader rebuilds each row into the query
// the suite actually ran. Nothing about a row is inferred: a file the table
// does not name, or a header shape it does not expect, is a read error, so a
// corpus refresh that adds files fails until the reader learns them.
//
// Warnings are deliberately NOT asserted. The corpus records acceptance, and a
// Warning is an observation about a pair, never an admissibility refusal;
// pinning a golden warning list here would turn a ratchet into a
// change-detector. Severity is read from [lint.Issue.Severity] alone — the
// probe never encodes which code is an Error, so a re-grading in the REQ-161
// catalogue flows through instead of drifting against a copy kept here.

// conformanceEHRID is the fixed literal substituted for the `${ehr_id}` Robot
// variable two of the templates carry in their WHERE clause. Upstream fills it
// at run time with the EHR the suite just created; there is no EHR here, and
// the ratchet asserts admissibility rather than results, so any syntactically
// well-formed UUID reconstructs the same query shape. A fixed literal keeps the
// reconstruction deterministic and the failure output stable.
const conformanceEHRID = "11111111-1111-1111-1111-111111111111"

// conformancePinnedSuites are the CSVs pinned BY NAME: [Probe100ConformanceCorpus]
// fails a corpus in which any of them contributes no asserted row, each with the
// reason its pin exists, so none can go missing while the probe still reports
// pass. The per-family tripwire fires only when a whole directory empties, and
// an excluded row fails nothing — these three have failure modes exactly that
// quiet. The chaining suite is the reason the probe exists: REQ-160's
// compatibility guard was hand-picked from its chains, and generalising the
// guard to the corpus must not quietly drop it. The two EHR_STATUS suites are
// the only ones whose templates carry the `${ehr_id}` Robot variable, so they
// alone depend on the [conformanceEHRID] substitution: a regression there sends
// their rows to the exclusion list — no family tripwire notices, because
// EHR_STATUS/contains.csv still contributes — and only a pin that names them
// turns that into a failure with the file on it.
var conformancePinnedSuites = []struct {
	// Suite is the family-qualified CSV path, as [ConformanceFileCount] keys it.
	Suite string
	// Why finishes the failure sentence "<suite> contributes no asserted row; …".
	Why string
}{
	{
		Suite: "CONTAINS_A_D/from_contains_plus_contain_chaining.csv",
		Why:   "it carries the containment chains REQ-160's compatibility guard was drawn from",
	},
	{
		Suite: "EHR_STATUS/from_single_ehr.csv",
		Why:   "its template is one of the two carrying the ${ehr_id} substitution, whose regression excludes rows rather than failing a family",
	},
	{
		Suite: "EHR_STATUS/via_part.csv",
		Why:   "its template is one of the two carrying the ${ehr_id} substitution, whose regression excludes rows rather than failing a family",
	},
}

// conformanceRootFiles are the two non-CSV files the ingest writes at the
// corpus root — the provenance pin and the generated exclusion list. Anything
// else sitting there is an unlearned refresh, not a corpus file to ignore.
var conformanceRootFiles = []string{"AQL_SOURCE.txt", "EXCLUDED.txt"}

// conformanceSuite maps one vendored CSV to the query template of the Robot
// suite that consumes it.
type conformanceSuite struct {
	// Family is the corpus family directory, which is the upstream
	// AQL_TESTS/FROM/<FAMILY>/ directory the consuming suite lives in.
	Family string
	// File is the CSV basename inside that family directory.
	File string
	// Header is the CSV's header row, verbatim and complete. The reader
	// compares it field for field, so an upstream column added, dropped or
	// renamed is a read error rather than a silently shifted substitution.
	Header []string
	// Vars are the leading Header columns substituted into Template, in
	// column order. The remaining Header columns name the suite's expected
	// result artefact and its row count — execution semantics, which this
	// corpus does not carry and this probe does not assert.
	Vars []string
	// Template is the consuming suite's query template, verbatim, including
	// its casing: upstream writes `contains` in lower case in one of them and
	// the AQL grammar is case-insensitive, so it is kept as written.
	Template string
	// Suite is the upstream .robot file Template was transcribed from,
	// relative to tests/robot/ — the provenance of every query this entry
	// reconstructs, and named in the failure output so a drift can be checked
	// against its source without consulting this table.
	Suite string
}

// key is the corpus-relative identity of the suite's CSV.
func (s conformanceSuite) key() string { return s.Family + "/" + s.File }

// conformanceSuites is the reconstruction table: for every vendored CSV, the
// template its consuming Robot suite substitutes the row into. Transcribed from
// the .robot sources at the pin recorded in the corpus's AQL_SOURCE.txt.
//
// The table — not the directory listing — is what the corpus is. A CSV present
// on disk but absent here cannot be reconstructed (no template), and an entry
// here with no CSV on disk is a corpus that lost a file; the reader refuses
// both, which is the ratchet's refresh arm.
var conformanceSuites = []conformanceSuite{
	{
		Family:   "AND_OR",
		File:     "from_simple_and_or.csv",
		Header:   []string{"${statement}", "${expected_file}", "${nr_of_results}"},
		Vars:     []string{"${statement}"},
		Template: "${statement}",
		Suite:    "AQL_TESTS/FROM/AND_OR/simple_and_or.robot",
	},
	{
		Family:   "CONTAINS_A_D",
		File:     "from_contains_plus_contain_chaining.csv",
		Header:   []string{"${from}", "${expected_file}", "${nr_of_results}"},
		Vars:     []string{"${from}"},
		Template: "SELECT o FROM ${from}",
		Suite:    "AQL_TESTS/FROM/CONTAINS_A_D/contains_plus_contain_chaining.robot",
	},
	{
		Family:   "CONTAINS_A_D",
		File:     "from_contains_with_repeating_types.csv",
		Header:   []string{"${from}", "${expected_file}", "${nr_of_results}"},
		Vars:     []string{"${from}"},
		Template: "SELECT o FROM COMPOSITION contains ${from}",
		Suite:    "AQL_TESTS/FROM/CONTAINS_A_D/contains_with_repeating_types.robot",
	},
	{
		Family:   "EHR_STATUS",
		File:     "contains.csv",
		Header:   []string{"${path}", "${expected_file}", "${nr_of_results}"},
		Vars:     []string{"${path}"},
		Template: "SELECT l/name/value FROM EHR e CONTAINS ${path}",
		Suite:    "AQL_TESTS/FROM/EHR_STATUS/contains.robot",
	},
	{
		Family:   "EHR_STATUS",
		File:     "from_single_ehr.csv",
		Header:   []string{"${path}", "${expected_file}", "${nr_of_results}"},
		Vars:     []string{"${path}"},
		Template: "SELECT s/${path} FROM EHR e CONTAINS EHR_STATUS s WHERE e/ehr_id/value = '${ehr_id}'",
		Suite:    "AQL_TESTS/FROM/EHR_STATUS/from_single_ehr.robot",
	},
	{
		Family:   "EHR_STATUS",
		File:     "via_part.csv",
		Header:   []string{"${path}", "${expected_file}", "${nr_of_results}"},
		Vars:     []string{"${path}"},
		Template: "SELECT e/ehr_status/${path} FROM EHR e WHERE e/ehr_id/value = '${ehr_id}'",
		Suite:    "AQL_TESTS/FROM/EHR_STATUS/via_part.robot",
	},
	{
		Family:   "PREDICATE_A_D",
		File:     "from_predicate_on_extracted_column.csv",
		Header:   []string{"${type}", "${predicate}", "${expected_file}", "${nr_of_results}"},
		Vars:     []string{"${type}", "${predicate}"},
		Template: "SELECT t FROM ${type} t ${predicate}",
		Suite:    "AQL_TESTS/FROM/PREDICATE_A_D/predicate_on_extracted_column.robot",
	},
	{
		Family:   "USABLE_RM_TYPES_A_D",
		File:     "from_abstract_types.csv",
		Header:   []string{"${type}", "${expected_file}", "${nr_of_results}"},
		Vars:     []string{"${type}"},
		Template: "SELECT t FROM ${type} t",
		Suite:    "AQL_TESTS/FROM/USABLE_RM_TYPES_A_D/from_abstract_types.robot",
	},
	{
		Family:   "USABLE_RM_TYPES_A_D",
		File:     "from_common_types.csv",
		Header:   []string{"${type}", "${expected_file}", "${nr_of_results}"},
		Vars:     []string{"${type}"},
		Template: "SELECT t FROM ${type} t",
		Suite:    "AQL_TESTS/FROM/USABLE_RM_TYPES_A_D/from_common_types.robot",
	},
	{
		Family: "USABLE_RM_TYPES_A_D",
		File:   "from_composition.csv",
		// The one CSV upstream wrote without a `${nr_of_results}` column.
		Header:   []string{"${type}", "${expected_file}"},
		Vars:     []string{"${type}"},
		Template: "SELECT t FROM ${type} t",
		Suite:    "AQL_TESTS/FROM/USABLE_RM_TYPES_A_D/from_composition.robot",
	},
	{
		Family:   "USABLE_RM_TYPES_A_D",
		File:     "from_item_structure_and_element_in_composition.csv",
		Header:   []string{"${type}", "${expected_file}", "${nr_of_results}"},
		Vars:     []string{"${type}"},
		Template: "SELECT t FROM COMPOSITION CONTAINS ${type} t",
		Suite:    "AQL_TESTS/FROM/USABLE_RM_TYPES_A_D/from_item_structure_and_element_in_composition.robot",
	},
	{
		Family: "USABLE_RM_TYPES_A_D",
		File:   "from_item_structure_composition.csv",
		// The one entry whose CSV name and suite name differ upstream
		// (…_composition.csv is consumed by …_in_composition.robot).
		Header:   []string{"${type}", "${expected_file}", "${nr_of_results}"},
		Vars:     []string{"${type}"},
		Template: "SELECT t FROM COMPOSITION CONTAINS ${type} t",
		Suite:    "AQL_TESTS/FROM/USABLE_RM_TYPES_A_D/from_item_structure_in_composition.robot",
	},
}

// conformanceExclusion is one named rule under which a reconstructed row does
// not reach the ratchet. A row is never dropped silently: it either becomes an
// asserted query or is recorded against one of these reasons, so the asserted
// and excluded tallies always add up to the rows on disk.
//
// No row in the corpus at its current pin meets either rule. They exist for the
// refresh: an upstream row that stops reconstructing into a complete query must
// leave the ratchet by a rule someone wrote down, not by a reader that shrugs.
type conformanceExclusion struct {
	// Reason is the tag recorded against an excluded row.
	Reason string
	// Why states the rule in prose, for whoever meets it after a refresh.
	Why string
	// Excludes reports whether the substituted query q, built from the row's
	// variable values, falls under the rule.
	Excludes func(q string, values []string) bool
}

// conformanceExclusions are the exclusion rules, applied in order; the first
// match names the row's reason.
var conformanceExclusions = []conformanceExclusion{
	{
		Reason: "empty-variable-value",
		Why: "a template variable's cell is blank, so the row supplies no shape to substitute " +
			"and the reconstruction would be a different query from the one the suite ran",
		Excludes: func(_ string, values []string) bool {
			return slices.ContainsFunc(values, func(v string) bool { return strings.TrimSpace(v) == "" })
		},
	},
	{
		Reason: "unresolved-template-variable",
		Why: "the reconstruction still holds a ${…} placeholder: the suite template names a Robot " +
			"variable the CSV header does not supply and the fixed ${ehr_id} literal does not cover, " +
			"so the string is not a complete query",
		Excludes: func(q string, _ []string) bool { return strings.Contains(q, "${") },
	},
}

// ConformanceRow is one corpus row reconstructed into the query its consuming
// suite ran.
type ConformanceRow struct {
	// Family and File identify the CSV, corpus-relative.
	Family string
	File   string
	// Line is the row's 1-based line in that CSV (the header is line 1).
	Line int
	// Suite is the .robot file whose template built Query.
	Suite string
	// Query is the reconstructed query.
	Query string
}

// Where is the row's corpus coordinate, `FAMILY/file.csv:LINE` — enough to open
// the offending row from a CI log alone.
func (r ConformanceRow) Where() string {
	return fmt.Sprintf("%s/%s:%d", r.Family, r.File, r.Line)
}

// ConformanceExcluded is one row an exclusion rule held back.
type ConformanceExcluded struct {
	// Family, File and Line locate the row, as on [ConformanceRow].
	Family string
	File   string
	Line   int
	// Reason is the [conformanceExclusion] tag that matched, and Why is that
	// rule's prose. Both travel with the row so a caller logging the tally
	// explains itself without anyone opening this file.
	Reason string
	Why    string
}

// Where is the excluded row's corpus coordinate.
func (e ConformanceExcluded) Where() string {
	return fmt.Sprintf("%s/%s:%d", e.Family, e.File, e.Line)
}

// ConformanceCorpus is one whole read of the vendored corpus: every row either
// reconstructed into Rows or recorded in Excluded.
type ConformanceCorpus struct {
	Rows     []ConformanceRow
	Excluded []ConformanceExcluded
}

// ConformanceFileCount is one CSV's tally.
type ConformanceFileCount struct {
	Family   string
	File     string
	Asserted int
	Excluded int
}

// ConformanceFamilyCount is one family directory's tally.
type ConformanceFamilyCount struct {
	Family   string
	Asserted int
	Excluded int
}

// FileCounts tallies c per CSV, in [conformanceSuites] order, so the report a
// caller logs is stable across runs and machines.
func (c ConformanceCorpus) FileCounts() []ConformanceFileCount {
	index := map[string]int{}
	out := make([]ConformanceFileCount, 0, len(conformanceSuites))
	for _, s := range conformanceSuites {
		index[s.key()] = len(out)
		out = append(out, ConformanceFileCount{Family: s.Family, File: s.File})
	}
	for _, r := range c.Rows {
		if i, ok := index[r.Family+"/"+r.File]; ok {
			out[i].Asserted++
		}
	}
	for _, e := range c.Excluded {
		if i, ok := index[e.Family+"/"+e.File]; ok {
			out[i].Excluded++
		}
	}
	return out
}

// FamilyCounts tallies c per family directory, in [conformanceSuites] order.
func (c ConformanceCorpus) FamilyCounts() []ConformanceFamilyCount {
	index := map[string]int{}
	var out []ConformanceFamilyCount
	for _, f := range c.FileCounts() {
		i, ok := index[f.Family]
		if !ok {
			i = len(out)
			index[f.Family] = i
			out = append(out, ConformanceFamilyCount{Family: f.Family})
		}
		out[i].Asserted += f.Asserted
		out[i].Excluded += f.Excluded
	}
	return out
}

// ReadConformanceCorpus reads the vendored corpus rooted at root — the
// directory holding the family directories, AQL_SOURCE.txt and EXCLUDED.txt —
// and reconstructs every row into the query its consuming suite ran.
//
// Callers in this repository pass fixtures.AQLConformanceRoot(); the parameter
// is there so a downstream SDK can point the same reader at its own vendored
// copy.
//
// It is strict by design: an unlearned file at the root or in a family
// directory, a family directory the table does not name, a header that does not
// match field for field, a table entry with no file on disk, or a missing
// ingest file (AQL_SOURCE.txt, EXCLUDED.txt) is an error, not a skipped row —
// a corpus without its provenance pin or exclusion record is not a corpus.
func ReadConformanceCorpus(root string) (ConformanceCorpus, error) {
	var c ConformanceCorpus
	entries, err := os.ReadDir(root)
	if err != nil {
		return c, fmt.Errorf("PROBE-100: read corpus root: %w", err)
	}
	seen := map[string]bool{}
	rootSeen := map[string]bool{}
	for _, e := range entries {
		if !e.IsDir() {
			if slices.Contains(conformanceRootFiles, e.Name()) {
				rootSeen[e.Name()] = true
				continue
			}
			return c, fmt.Errorf("PROBE-100: %q at the corpus root is neither a family directory nor "+
				"one of the ingest's own files %v; teach the reader about it before vendoring it", e.Name(), conformanceRootFiles)
		}
		if err := readConformanceFamily(root, e.Name(), &c, seen); err != nil {
			return c, err
		}
	}
	for _, name := range conformanceRootFiles {
		if !rootSeen[name] {
			return c, fmt.Errorf("PROBE-100: the corpus root is missing %s — the ingest writes it beside "+
				"the family directories; a corpus without its provenance pin and exclusion record is not readable", name)
		}
	}
	for _, s := range conformanceSuites {
		if !seen[s.key()] {
			return c, fmt.Errorf("PROBE-100: the reconstruction table names %s but the corpus does not carry it; "+
				"a vendored CSV went missing, or the table outlived it", s.key())
		}
	}
	return c, nil
}

// readConformanceFamily reads every CSV of one family directory into c.
func readConformanceFamily(root, family string, c *ConformanceCorpus, seen map[string]bool) error {
	if !slices.ContainsFunc(conformanceSuites, func(s conformanceSuite) bool { return s.Family == family }) {
		return fmt.Errorf("PROBE-100: corpus family %q is not in the reconstruction table; "+
			"a refresh added a family whose query templates the reader has not learned", family)
	}
	entries, err := os.ReadDir(filepath.Join(root, family))
	if err != nil {
		return fmt.Errorf("PROBE-100: read corpus family %q: %w", family, err)
	}
	for _, e := range entries {
		key := family + "/" + e.Name()
		i := slices.IndexFunc(conformanceSuites, func(s conformanceSuite) bool { return s.key() == key })
		if i < 0 {
			return fmt.Errorf("PROBE-100: corpus file %s is not in the reconstruction table; "+
				"a refresh added a CSV whose consuming suite's query template the reader has not learned", key)
		}
		if err := readConformanceCSV(filepath.Join(root, family, e.Name()), conformanceSuites[i], c); err != nil {
			return err
		}
		seen[key] = true
	}
	return nil
}

// readConformanceCSV parses one CSV against its suite entry and appends each row
// to c, reconstructed or excluded. The header is compared field for field, and
// encoding/csv enforces the row width against it, so a shifted or renamed column
// cannot silently substitute the wrong value.
func readConformanceCSV(path string, s conformanceSuite, c *ConformanceCorpus) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("PROBE-100: open %s: %w", s.key(), err)
	}
	defer f.Close() //nolint:errcheck // read-only file

	r := csv.NewReader(f)
	header, err := r.Read()
	if err != nil {
		return fmt.Errorf("PROBE-100: read header of %s: %w", s.key(), err)
	}
	if !slices.Equal(header, s.Header) {
		return fmt.Errorf("PROBE-100: header of %s is %v, the reconstruction table expects %v; "+
			"a refresh reshaped the CSV and the reader's substitution is no longer trustworthy", s.key(), header, s.Header)
	}
	for {
		record, err := r.Read()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("PROBE-100: read %s: %w", s.key(), err)
		}
		line, _ := r.FieldPos(0)
		values := record[:len(s.Vars)]
		query, err := reconstructConformanceQuery(s, values)
		if err != nil {
			return err
		}
		if i := slices.IndexFunc(conformanceExclusions, func(x conformanceExclusion) bool {
			return x.Excludes(query, values)
		}); i >= 0 {
			c.Excluded = append(c.Excluded, ConformanceExcluded{
				Family: s.Family, File: s.File, Line: line,
				Reason: conformanceExclusions[i].Reason,
				Why:    conformanceExclusions[i].Why,
			})
			continue
		}
		c.Rows = append(c.Rows, ConformanceRow{
			Family: s.Family, File: s.File, Line: line,
			Suite: s.Suite, Query: query,
		})
	}
}

// reconstructConformanceQuery substitutes one row's values into its suite's
// template, then the fixed [conformanceEHRID] for the `${ehr_id}` Robot
// variable two templates carry. Values go in verbatim — the corpus records what
// the engine accepted, so normalising the casing or spacing of a row would
// assert something other than what upstream ran.
func reconstructConformanceQuery(s conformanceSuite, values []string) (string, error) {
	query := s.Template
	for i, v := range s.Vars {
		if !strings.Contains(s.Template, v) {
			return "", fmt.Errorf("PROBE-100: reconstruction table entry %s lists variable %s, "+
				"which its template does not carry", s.key(), v)
		}
		query = strings.ReplaceAll(query, v, values[i])
	}
	return strings.ReplaceAll(query, "${ehr_id}", conformanceEHRID), nil
}

// Probe100ConformanceCorpus runs the ratchet over every reconstructed row of c
// and aggregates all failures into one [Result] (collect-all, like
// [Probe099PathShapeLint] — a single early failure would hide the rest of the
// corpus, and the point of a corpus ratchet is the whole picture).
func Probe100ConformanceCorpus(c ConformanceCorpus) (Result, error) {
	r := Result{Probe: "PROBE-100"}
	if len(c.Rows) == 0 {
		return r, errors.New("PROBE-100: the corpus reconstructed no rows at all; " +
			"the ratchet has nothing to hold and would report pass vacuously")
	}

	var failures []string
	for _, row := range c.Rows {
		if msg := runConformanceRow(row); msg != "" {
			failures = append(failures, msg)
		}
	}

	// The empty-corpus tripwire, per family. The len(c.Rows) == 0 check above
	// only catches a corpus that vanished whole; a family directory emptied by
	// a bad refresh would otherwise leave this probe green on the families that
	// survived. Families come from the reconstruction table, not from what was
	// read, so a family that reconstructed nothing still has to answer for it.
	for _, f := range c.FamilyCounts() {
		if f.Asserted == 0 {
			failures = append(failures, fmt.Sprintf(
				"family %s: no asserted row; the ratchet no longer covers this family (%d row(s) excluded)",
				f.Family, f.Excluded))
		}
	}
	// The named pins: suites whose loss no family tripwire would notice.
	for _, pin := range conformancePinnedSuites {
		if !slices.ContainsFunc(c.FileCounts(), func(f ConformanceFileCount) bool {
			return f.Family+"/"+f.File == pin.Suite && f.Asserted > 0
		}) {
			failures = append(failures, pin.Suite+" contributes no asserted row; "+pin.Why)
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

// runConformanceRow is the two-part assertion of the wire contract: the row
// parses, and the REQ-161 containment checks raise no Error on it. It returns
// "" when the row holds, and otherwise a message naming the corpus coordinate,
// the consuming suite and the query — the corpus is public upstream data, so
// printing it costs nothing and saves a round trip to the CSV.
func runConformanceRow(row ConformanceRow) string {
	if _, err := parse.ParseQuery(row.Query); err != nil {
		return fmt.Sprintf("%s: does not parse under the SDK grammar profile: %v (suite %s, query %q)",
			row.Where(), err, row.Suite, row.Query)
	}
	issues := conformanceContainmentErrors(lint.LintString(row.Query, nil))
	if len(issues) == 0 {
		return ""
	}
	codes := make([]string, 0, len(issues))
	for _, i := range issues {
		codes = append(codes, i.Code)
	}
	return fmt.Sprintf("%s: containment lint raises Error-severity %v on a shape a conformant engine "+
		"accepted and answered — REQ-160 must stay the looser of the two (suite %s, query %q)",
		row.Where(), codes, row.Suite, row.Query)
}

// conformanceContainmentErrors is the Error-severity subset of r's issues
// within the [containmentCodes] scope — the REQ-162 § Contract five, shared
// with PROBE-097 arm (c) so the two probes cannot drift apart on what
// "containment code" means.
//
// The scope filter is [filterCodes]; the severity comes from
// [lint.Issue.Severity] and nothing else. A probe that decided for itself which
// of those five is an Error would keep passing through a re-grading in the
// REQ-161 catalogue, which is exactly the drift a ratchet is supposed to catch.
func conformanceContainmentErrors(r lint.Result) []lint.Issue {
	scope := filterCodes(r, containmentCodes())
	if len(scope) == 0 {
		return nil
	}
	var out []lint.Issue
	for _, i := range r.Issues {
		if i.Severity == lint.Error && slices.Contains(scope, i.Code) {
			out = append(out, i)
		}
	}
	return out
}
