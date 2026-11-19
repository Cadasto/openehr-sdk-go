package lint_test

// REQ-109
//
// letter_case_test.go holds the lint to the grammar's letter case: keywords,
// function names and RM class names are read in any case, so recasing one of
// them changes no finding, while an attribute name in a path is never folded
// and an upper-case letter in one is reported as aql_path_attribute_case.

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"testing"
	"unicode"

	"github.com/cadasto/openehr-sdk-go/openehr/aql/lint"
)

const codeAttributeCase = "aql_path_attribute_case"

// lintCaseWordRE marks a respellable word (keyword, function or RM class name)
// in a [lintCaseTemplates] entry.
var lintCaseWordRE = regexp.MustCompile(`<<([A-Za-z_]+)>>`)

func spellLintCase(tmpl string, fn func(string) string) string {
	return lintCaseWordRE.ReplaceAllStringFunc(tmpl, func(m string) string {
		return fn(m[2 : len(m)-2])
	})
}

func alternateLetters(s string) string {
	var b strings.Builder
	for i, r := range s {
		if i%2 == 0 {
			b.WriteRune(unicode.ToLower(r))
		} else {
			b.WriteRune(unicode.ToUpper(r))
		}
	}
	return b.String()
}

// codeSeverities is the sorted code:severity multiset of a result: what a
// recasing must leave unchanged (Detail and Path quote the spelling, so they
// may differ).
func codeSeverities(r lint.Result) []string {
	out := make([]string, len(r.Issues))
	for i, is := range r.Issues {
		out[i] = is.Code + ":" + is.Severity.String()
	}
	slices.Sort(out)
	return out
}

const bpArchetype = "[openEHR-EHR-OBSERVATION.blood_pressure.v1]"

// lintCaseTemplates are queries drawing the shape, semantic and path-shape
// findings whose firing reads a keyword, function or class name, plus clean
// queries whose silence does.
var lintCaseTemplates = []string{
	`<<SELECT>> e/ehr_id/value <<AS>> id <<FROM>> <<EHR>> e`,
	`<<SELECT>> c/uid/value <<AS>> id <<FROM>> <<EHR>> e <<CONTAINS>> <<COMPOSITION>> c <<WHERE>> e/ehr_id/value = $ehr ` +
		`<<ORDER>> <<BY>> c/uid/value <<DESC>> <<LIMIT>> 5 <<OFFSET>> 1`,
	`<<SELECT>> c/uid/value <<AS>> id <<FROM>> <<COMPOSITION>> c`,
	`<<SELECT>> c/uid/value <<AS>> id <<FROM>> <<EHR>> e <<CONTAINS>> <<FOLDER>> f <<CONTAINS>> <<COMPOSITION>> c`,
	`<<SELECT>> o/uid/value <<AS>> id <<FROM>> <<EHR>> e <<CONTAINS>> <<OBSERVATION>> o <<CONTAINS>> <<COMPOSITION>> c`,
	`<<SELECT>> o/uid/value <<AS>> id <<FROM>> <<EHR>> e <<CONTAINS>> <<EVALUATION>> o` + bpArchetype,
	`<<SELECT>> el/value <<AS>> v <<FROM>> <<EHR>> e <<CONTAINS>> <<ELEMENT>> el <<CONTAINS>> <<CLUSTER>> cl`,
	`<<SELECT>> c/uid/value <<AS>> id <<FROM>> <<EHR>> e <<CONTAINS>> <<BLUBBER>> c`,
	`<<SELECT>> v/uid/value <<AS>> id <<FROM>> <<EHR>> e <<CONTAINS>> <<VERSION>> v <<CONTAINS>> <<COMPOSITION>> c`,
	`<<SELECT>> c/uid/value <<AS>> id <<FROM>> <<EHR>> e <<CONTAINS>> <<VERSIONED_OBJECT>> vo <<CONTAINS>> <<COMPOSITION>> c`,
	`<<SELECT>> c/uid/value <<AS>> id, o/uid/value <<AS>> oid <<FROM>> <<EHR>> e <<CONTAINS>> <<COMPOSITION>> c ` +
		`<<CONTAINS>> <<OBSERVATION>> o` + bpArchetype,
	`<<SELECT>> o/data/events/data/items/value/magnitude <<AS>> m <<FROM>> <<EHR>> e <<CONTAINS>> <<OBSERVATION>> o` + bpArchetype,
	`<<SELECT>> c <<FROM>> <<EHR>> e <<CONTAINS>> <<COMPOSITION>> c`,
	`<<SELECT>> <<TOP>> 5 c/uid/value <<AS>> id <<FROM>> <<EHR>> e <<CONTAINS>> <<COMPOSITION>> c <<LIMIT>> 5`,
	`<<SELECT>> *, c/uid/value <<AS>> id <<FROM>> <<EHR>> e <<CONTAINS>> <<COMPOSITION>> c`,
	`<<SELECT>> <<COUNT>>(*) <<AS>> n, <<MAX>>(c/uid/value) <<AS>> mx <<FROM>> <<EHR>> e <<CONTAINS>> <<COMPOSITION>> c ` +
		`<<WHERE>> <<LENGTH>>(c/name/value) > 3 <<AND>> <<NOT>> <<EXISTS>> c/context <<OR>> c/name/value <<MATCHES>> {'a'}`,
	`<<SELECT>> c/uid/value <<AS>> id <<FROM>> <<EHR>> e <<CONTAINS>> <<COMPOSITION>> c <<CONTAINS>> <<SECTION>> s ` +
		`<<CONTAINS>> <<ADMIN_ENTRY>> a <<CONTAINS>> <<ITEM_TREE>> t`,
	`<<SELECT>> s/uid/value <<AS>> id <<FROM>> <<EHR>> e <<CONTAINS>> <<EHR_STATUS>> s`,
}

// TestLintReadsKeywordsFunctionsAndClassesInAnyCase — recasing a keyword,
// function name or RM class name changes no finding: the lint draws the same
// codes at the same severities from every spelling.
func TestLintReadsKeywordsFunctionsAndClassesInAnyCase(t *testing.T) {
	t.Parallel()
	casings := []struct {
		name string
		fn   func(string) string
	}{
		{"lower", strings.ToLower},
		{"title", func(s string) string { return strings.ToUpper(s[:1]) + strings.ToLower(s[1:]) }},
		{"alternating", alternateLetters},
	}
	for _, tmpl := range lintCaseTemplates {
		upper := spellLintCase(tmpl, strings.ToUpper)
		want := codeSeverities(lint.LintString(upper, nil))
		for _, c := range casings {
			q := spellLintCase(tmpl, c.fn)
			t.Run(c.name+"/"+q, func(t *testing.T) {
				t.Parallel()
				if got := codeSeverities(lint.LintString(q, nil)); !slices.Equal(got, want) {
					t.Errorf("LintString(%q) = %v, want %v (the findings of %q)", q, got, want, upper)
				}
			})
		}
	}
}

// TestEHRRootInAnyCaseIsIdentifiableScope — the identifiable-scope check reads
// the EHR root in any case, while a query with no scope still draws it, so the
// check is not simply off.
func TestEHRRootInAnyCaseIsIdentifiableScope(t *testing.T) {
	t.Parallel()
	for _, root := range []string{"EHR", "ehr", "Ehr", "eHr"} {
		q := "SELECT e/ehr_id/value AS id FROM " + root + " e"
		if r := lint.LintString(q, nil); has(r, "aql_from_archetype") {
			t.Errorf("LintString(%q) codes = %v, want no aql_from_archetype: %s is the EHR root", q, codes(r), root)
		}
	}
	for _, class := range []string{"COMPOSITION", "composition"} {
		q := "SELECT c/uid/value AS id FROM " + class + " c"
		if r := lint.LintString(q, nil); !has(r, "aql_from_archetype") {
			t.Errorf("LintString(%q) codes = %v, want aql_from_archetype: no archetype, $param, VERSION or EHR root", q, codes(r))
		}
	}
}

// TestPathAttributeCase — an attribute name with an upper-case letter is an
// Error wherever a path names one: a step of an identified path in any clause
// or function argument, and a relative path inside a predicate on an alias
// root, on a step or on a FROM / CONTAINS class. The span covers the step for
// a step of the identified path, and otherwise the path or class whose
// predicate carries it.
func TestPathAttributeCase(t *testing.T) {
	t.Parallel()
	const from = " FROM EHR e CONTAINS COMPOSITION c"
	const fromObs = " FROM EHR e CONTAINS OBSERVATION o" + bpArchetype
	cases := []struct {
		name  string
		q     string
		attrs []string // the reported attribute names, in result order
		spans []string // the source text each finding's span covers
	}{
		{
			name: "select step", q: "SELECT c/UID/value AS id" + from,
			attrs: []string{"UID"}, spans: []string{"UID"},
		},
		{
			name: "two steps of one path", q: "SELECT c/Context/Start_time/value AS t" + from,
			attrs: []string{"Context", "Start_time"}, spans: []string{"Context", "Start_time"},
		},
		{
			name: "where step", q: "SELECT c/uid/value AS id" + from + " WHERE c/Name/value = 'x'",
			attrs: []string{"Name"}, spans: []string{"Name"},
		},
		{
			name: "order by step", q: "SELECT c/uid/value AS id" + from + " ORDER BY c/context/start_time/Value",
			attrs: []string{"Value"}, spans: []string{"Value"},
		},
		{
			name: "aggregate argument", q: "SELECT COUNT(c/Uid/value) AS n" + from,
			attrs: []string{"Uid"}, spans: []string{"Uid"},
		},
		{
			name: "function argument in where", q: "SELECT c/uid/value AS id" + from + " WHERE LENGTH(c/name/Value) > 3",
			attrs: []string{"Value"}, spans: []string{"Value"},
		},
		{
			name: "right-hand path of a comparison", q: "SELECT c/uid/value AS id" + from + " WHERE c/uid/value = c/Uid/value",
			attrs: []string{"Uid"}, spans: []string{"Uid"},
		},
		{
			name:  "node predicate junction on a step",
			q:     "SELECT o/data[at0001]/events[at0006]/data[at0003]/items[at0004 and Name/value='Systolic']/value/magnitude AS m" + fromObs,
			attrs: []string{"Name"},
			spans: []string{"o/data[at0001]/events[at0006]/data[at0003]/items[at0004 and Name/value='Systolic']/value/magnitude"},
		},
		{
			name:  "matches predicate on a step",
			q:     "SELECT o/data[at0001]/events[name/Value matches {/any/}]/time/value AS t" + fromObs,
			attrs: []string{"Value"},
			spans: []string{"o/data[at0001]/events[name/Value matches {/any/}]/time/value"},
		},
		{
			name: "alias-root predicate", q: "SELECT c[Name/value='x']/uid/value AS id" + from,
			attrs: []string{"Name"}, spans: []string{"c[Name/value='x']/uid/value"},
		},
		{
			name: "standing predicate on the EHR class", q: "SELECT e/ehr_id/value AS id FROM EHR e[EHR_ID/value=$id]",
			attrs: []string{"EHR_ID"}, spans: []string{"EHR"},
		},
		{
			name:  "archetype and name on a class",
			q:     "SELECT c/uid/value AS id FROM EHR e CONTAINS COMPOSITION c[openEHR-EHR-COMPOSITION.encounter.v1 and Name/value='x']",
			attrs: []string{"Name"}, spans: []string{"COMPOSITION"},
		},
		{
			name:  "findings in document order across clauses",
			q:     "SELECT c/Uid/value AS id FROM EHR e[EHR_ID/value=$id] CONTAINS COMPOSITION c WHERE c/Name/value = 'x'",
			attrs: []string{"Uid", "EHR_ID", "Name"}, spans: []string{"Uid", "EHR", "Name"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := lint.LintString(tc.q, nil)
			var found []lint.Issue
			for _, is := range r.Issues {
				if is.Code == codeAttributeCase {
					found = append(found, is)
				}
			}
			if len(found) != len(tc.attrs) {
				t.Fatalf("LintString(%q) codes = %v, want %d %s finding(s) for %v", tc.q, codes(r), len(tc.attrs), codeAttributeCase, tc.attrs)
			}
			if r.OK() {
				t.Errorf("LintString(%q).OK() = true, want false: %s is an Error", tc.q, codeAttributeCase)
			}
			for i, is := range found {
				attr := tc.attrs[i]
				if is.Severity != lint.Error {
					t.Errorf("finding %d of %q: severity %v, want error", i, tc.q, is.Severity)
				}
				if want := fmt.Sprintf("%q", attr); !strings.Contains(is.Detail, want) {
					t.Errorf("finding %d of %q: Detail %q does not name the attribute %s", i, tc.q, is.Detail, want)
				}
				if want := fmt.Sprintf("%q", strings.ToLower(attr)); !strings.Contains(is.Detail, want) {
					t.Errorf("finding %d of %q: Detail %q does not offer the lower-case spelling %s", i, tc.q, is.Detail, want)
				}
				if got := spanText(t, tc.q, is.Span); got != tc.spans[i] {
					t.Errorf("finding %d of %q: span covers %q, want %q", i, tc.q, got, tc.spans[i])
				}
			}
		})
	}
}

// TestAliasMayTakeAnyCaseButMustBeUsedAsDeclared — an alias declared in
// FROM / CONTAINS or by a SELECT `AS` may be lower, upper or mixed case, and
// every use must spell it as declared: a use that differs only in letter case
// is an unbound alias, whose Detail names the declared spelling.
func TestAliasMayTakeAnyCaseButMustBeUsedAsDeclared(t *testing.T) {
	t.Parallel()
	const bp = bpArchetype
	for _, q := range []string{
		"SELECT c/uid/value AS id FROM EHR e CONTAINS COMPOSITION c WHERE e/ehr_id/value = 'x' ORDER BY c/uid/value",
		"SELECT C/uid/value AS ID FROM EHR E CONTAINS COMPOSITION C WHERE E/ehr_id/value = 'x' ORDER BY C/uid/value",
		"SELECT COUNT(Obs) AS n, MAX(Obs/data[at0001]/events[at0006]/time/value) AS Latest FROM EHR Ehr CONTAINS COMPOSITION cOmP" +
			" CONTAINS OBSERVATION Obs" + bp + " WHERE Ehr/ehr_id/value = 'x' AND cOmP/name/value = 'y'",
		"SELECT c/uid/value AS Id FROM EHR e CONTAINS COMPOSITION c ORDER BY Id",
	} {
		if r := lint.LintString(q, nil); has(r, "aql_unknown_alias") {
			t.Errorf("LintString(%q) codes = %v, want no aql_unknown_alias: every alias is used as declared", q, codes(r))
		}
	}

	cases := []struct {
		name, q, alias, declared string
	}{
		{"upper use of a lower declaration", "SELECT C/uid/value AS id FROM EHR e CONTAINS COMPOSITION c", "C", "c"},
		{"lower use of an upper declaration", "SELECT c/uid/value AS id FROM EHR e CONTAINS COMPOSITION C", "c", "C"},
		{"where clause", "SELECT c/uid/value AS id FROM EHR e CONTAINS COMPOSITION c WHERE E/ehr_id/value = 'x'", "E", "e"},
		{"order by path", "SELECT c/uid/value AS id FROM EHR e CONTAINS COMPOSITION c ORDER BY C/uid/value", "C", "c"},
		{"mixed-case declaration", "SELECT obs/uid/value AS id FROM EHR e CONTAINS OBSERVATION Obs" + bp, "obs", "Obs"},
		{"order by a SELECT alias", "SELECT c/uid/value AS Id FROM EHR e CONTAINS COMPOSITION c ORDER BY id", "id", "Id"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := lint.LintString(tc.q, nil)
			if count(r, "aql_unknown_alias") != 1 || r.OK() {
				t.Fatalf("LintString(%q) codes = %v, OK = %v; want one aql_unknown_alias Error for %q", tc.q, codes(r), r.OK(), tc.alias)
			}
			for _, is := range r.Issues {
				if is.Code != "aql_unknown_alias" {
					continue
				}
				if want := fmt.Sprintf("%q", tc.declared); !strings.Contains(is.Detail, want) {
					t.Errorf("LintString(%q): Detail %q does not name the declared spelling %s", tc.q, is.Detail, want)
				}
			}
		})
	}

	// No spelling hint where no respelling would bind the alias: an alias
	// declared nowhere, and a SELECT `AS` alias used as a path root, which
	// stays unbound in its declared spelling too (only a bare ORDER BY key
	// resolves through a SELECT alias).
	for _, q := range []string{
		"SELECT x/uid/value AS id FROM EHR e CONTAINS COMPOSITION c",
		"SELECT c/uid/value AS Id FROM EHR e CONTAINS COMPOSITION c WHERE id/name/value = 'x'",
		"SELECT c/uid/value AS Id FROM EHR e CONTAINS COMPOSITION c ORDER BY id/value",
	} {
		r := lint.LintString(q, nil)
		if !has(r, "aql_unknown_alias") {
			t.Errorf("LintString(%q) codes = %v, want aql_unknown_alias", q, codes(r))
		}
		for _, is := range r.Issues {
			if is.Code == "aql_unknown_alias" && strings.Contains(is.Detail, "spelling") {
				t.Errorf("LintString(%q): Detail %q offers a spelling hint, want none: no respelling binds the alias here", q, is.Detail)
			}
		}
	}
}

// TestPathAttributeCaseIgnoresWhatIsNotAnAttribute — upper case outside an
// attribute name is never this code: class names, keywords, function names,
// aliases, archetype ids, node ids, literals, parameters, SELECT aliases and
// version-predicate keywords.
func TestPathAttributeCaseIgnoresWhatIsNotAnAttribute(t *testing.T) {
	t.Parallel()
	for _, q := range []string{
		"SELECT C/uid/value AS ID FROM EHR E CONTAINS COMPOSITION C",
		"Select Count(c/uid/value) As N From Ehr e Contains Composition c Where Length(c/name/value) > 3",
		"SELECT o/data[at0001]/events[at0006]/data[at0003]/items[at0004, 'Systolic']/value/magnitude AS Systolic" +
			" FROM EHR e[ehr_id/value=$EhrId] CONTAINS OBSERVATION o" + bpArchetype + " ORDER BY Systolic",
		"SELECT c/uid/value AS id FROM EHR e CONTAINS COMPOSITION c WHERE c/name/value = 'UPPER Case Value'" +
			" AND c/category/defining_code/code_string MATCHES {TERMINOLOGY('expand', 'HL7.org/FHIR/4.0', 'url=X')}",
		"SELECT v/uid/value AS id FROM EHR e CONTAINS VERSION v[Latest_Version] CONTAINS COMPOSITION c",
	} {
		if r := lint.LintString(q, nil); has(r, codeAttributeCase) {
			t.Errorf("LintString(%q) codes = %v, want no %s: no attribute name carries an upper-case letter", q, codes(r), codeAttributeCase)
		}
	}
}
