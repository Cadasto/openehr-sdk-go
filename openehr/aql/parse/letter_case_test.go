package parse_test

// REQ-109 · REQ-119
//
// letter_case_test.go holds the parser and the emitter to the grammar's letter
// case. The lexer spells every keyword, operator and function name with
// case-insensitive letter fragments, so the SDK grammar profile (REQ-109 §
// Syntax floor) must admit any casing of them, and REQ-119's canonical
// emission must give one spelling back whatever the source used. An RM class
// name is an IDENTIFIER, not a keyword: the parser admits it in any case and
// the emitter keeps it as written.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"unicode"

	"github.com/cadasto/openehr-sdk-go/openehr/aql/parse"
)

// letterCaseWordRE marks a respellable word in a [letterCaseTemplates] entry:
// `<<WORD>>` for a keyword, operator, function name or boolean, which Emit
// writes in one canonical spelling, and `<<:WORD>>` for a word Emit keeps as
// written (an RM class name, a version-predicate keyword).
var letterCaseWordRE = regexp.MustCompile(`<<(:?)([A-Za-z_]+)>>`)

// keywordTokenRE matches a lexer rule that spells its token with letter
// fragments only (`SELECT: S E L E C T ;`, `LATEST_VERSION : L A T E S T '_'
// V E R S I O N ;`), which is what makes the token case-insensitive.
var keywordTokenRE = regexp.MustCompile(`(?m)^([A-Z][A-Z0-9_]*)\s*:\s*([A-Z](?:\s+(?:[A-Z]|'_'))+)\s*;`)

// letterCaseTemplates name every keyword token of the active lexer, every
// function name, and a spread of RM classes. TestKeywordCasingCoverage keeps
// the list complete against the grammar.
var letterCaseTemplates = []string{
	`<<SELECT>> <<DISTINCT>> c/uid/value <<AS>> id <<FROM>> <<:EHR>> e <<CONTAINS>> <<:COMPOSITION>> c ` +
		`<<WHERE>> c/name/value <<LIKE>> 'a*' ` +
		`<<ORDER>> <<BY>> c/uid/value <<DESCENDING>>, c/name/value <<DESC>>, c/uid/value <<ASCENDING>>, c/name/value <<ASC>> ` +
		`<<LIMIT>> 5 <<OFFSET>> 2`,
	`<<SELECT>> <<TOP>> 5 <<FORWARD>> c/uid/value <<AS>> id <<FROM>> <<:EHR>> e <<CONTAINS>> <<:COMPOSITION>> c`,
	`<<SELECT>> <<TOP>> 5 <<BACKWARD>> c/uid/value <<AS>> id <<FROM>> <<:EHR>> e <<CONTAINS>> <<:COMPOSITION>> c`,
	`<<SELECT>> c/uid/value <<AS>> id <<FROM>> <<:EHR>> e <<CONTAINS>> <<:COMPOSITION>> c ` +
		`<<CONTAINS>> (<<:OBSERVATION>> o[openEHR-EHR-OBSERVATION.blood_pressure.v1] <<OR>> <<:EVALUATION>> v <<AND>> <<:INSTRUCTION>> i)`,
	`<<SELECT>> c/uid/value <<AS>> id <<FROM>> <<:EHR>> e <<CONTAINS>> <<:COMPOSITION>> c <<NOT>> <<CONTAINS>> <<:ACTION>> a`,
	`<<SELECT>> c/uid/value <<AS>> id <<FROM>> <<:EHR>> e <<CONTAINS>> <<:COMPOSITION>> c <<CONTAINS>> <<:SECTION>> s ` +
		`<<CONTAINS>> <<:ADMIN_ENTRY>> a <<CONTAINS>> <<:ITEM_TREE>> t <<CONTAINS>> <<:CLUSTER>> cl <<CONTAINS>> <<:ELEMENT>> el`,
	`<<SELECT>> s/uid/value <<AS>> id <<FROM>> <<:EHR>> e <<CONTAINS>> <<:EHR_STATUS>> s`,
	`<<SELECT>> c/uid/value <<AS>> id <<FROM>> <<:EHR>> e <<CONTAINS>> <<:FOLDER>> f <<CONTAINS>> <<:COMPOSITION>> c`,
	`<<SELECT>> vo/uid/value <<AS>> id <<FROM>> <<:EHR>> e <<CONTAINS>> <<:VERSIONED_OBJECT>> vo <<CONTAINS>> <<:COMPOSITION>> c`,
	`<<SELECT>> v/uid/value <<AS>> id <<FROM>> <<:EHR>> e <<CONTAINS>> <<VERSION>> v[<<:LATEST_VERSION>>] <<CONTAINS>> <<:COMPOSITION>> c`,
	`<<SELECT>> v/uid/value <<AS>> id <<FROM>> <<:EHR>> e <<CONTAINS>> <<VERSION>> v[<<:ALL_VERSIONS>>] <<CONTAINS>> <<:COMPOSITION>> c`,
	`<<SELECT>> c/uid/value <<AS>> id <<FROM>> <<:EHR>> e <<CONTAINS>> <<:COMPOSITION>> c ` +
		`<<WHERE>> <<EXISTS>> c/uid/value <<AND>> <<NOT>> c/name/value = 'x' <<OR>> c/name/value != <<NULL>> ` +
		`<<AND>> c/x/y = <<TRUE>> <<OR>> c/x/y = <<FALSE>>`,
	`<<SELECT>> c/uid/value <<AS>> id <<FROM>> <<:EHR>> e <<CONTAINS>> <<:COMPOSITION>> c ` +
		`<<WHERE>> c/name/value <<MATCHES>> {'a', 'b'} <<OR>> c/x/code_string <<MATCHES>> {<<TERMINOLOGY>>('expand', 'hl7.org/fhir/4.0', 'url=x')} ` +
		`<<OR>> c/x/code_string <<MATCHES>> <<TERMINOLOGY>>('expand', 'hl7.org/fhir/4.0', 'url=x')`,
	`<<SELECT>> c/uid/value <<AS>> id <<FROM>> <<:EHR>> e <<CONTAINS>> <<:COMPOSITION>> c <<WHERE>> <<LENGTH>>(c/name/value) > 3`,
	`<<SELECT>> <<LENGTH>>(c/name/value) <<AS>> l, <<CONTAINS_STR>>(c/name/value, 'a') <<AS>> h, <<POSITION>>('a', c/name/value) <<AS>> p, ` +
		`<<SUBSTRING>>(c/name/value, 1, 2) <<AS>> s, <<CONCAT>>(c/name/value, 'x') <<AS>> k, <<CONCAT_WS>>('-', c/name/value, 'x') <<AS>> w ` +
		`<<FROM>> <<:EHR>> e <<CONTAINS>> <<:COMPOSITION>> c`,
	`<<SELECT>> <<ABS>>(o/x/magnitude) <<AS>> a, <<MOD>>(o/x/magnitude, 2) <<AS>> m, <<CEIL>>(o/x/magnitude) <<AS>> ce, ` +
		`<<FLOOR>>(o/x/magnitude) <<AS>> f, <<ROUND>>(o/x/magnitude, 1) <<AS>> r ` +
		`<<FROM>> <<:EHR>> e <<CONTAINS>> <<:OBSERVATION>> o[openEHR-EHR-OBSERVATION.blood_pressure.v1]`,
	`<<SELECT>> <<NOW>>() <<AS>> n, <<CURRENT_DATE>>() <<AS>> d, <<CURRENT_TIME>>() <<AS>> t, ` +
		`<<CURRENT_DATE_TIME>>() <<AS>> dt, <<CURRENT_TIMEZONE>>() <<AS>> tz <<FROM>> <<:EHR>> e`,
	`<<SELECT>> <<COUNT>>(*) <<AS>> n, <<COUNT>>(<<DISTINCT>> c/uid/value) <<AS>> d, <<MIN>>(o/x/magnitude) <<AS>> mn, ` +
		`<<MAX>>(o/x/magnitude) <<AS>> mx, <<SUM>>(o/x/magnitude) <<AS>> s, <<AVG>>(o/x/magnitude) <<AS>> a ` +
		`<<FROM>> <<:EHR>> e <<CONTAINS>> <<:COMPOSITION>> c <<CONTAINS>> <<:OBSERVATION>> o[openEHR-EHR-OBSERVATION.blood_pressure.v1]`,
}

// letterCasings are the respellings each template is read under, beside the
// upper-case spelling the templates are written in.
var letterCasings = []struct {
	name string
	fn   func(string) string
}{
	{"lower", strings.ToLower},
	{"title", func(s string) string { return strings.ToUpper(s[:1]) + strings.ToLower(s[1:]) }},
	{"alternating", func(s string) string { return alternateCase(s, false) }},
	{"alternating-upper-first", func(s string) string { return alternateCase(s, true) }},
}

func alternateCase(s string, upperFirst bool) string {
	var b strings.Builder
	for i, r := range s {
		if (i%2 == 0) == upperFirst {
			b.WriteRune(unicode.ToUpper(r))
		} else {
			b.WriteRune(unicode.ToLower(r))
		}
	}
	return b.String()
}

// spellLetterCase renders tmpl with normalised words (`<<WORD>>`) spelled by
// keyword and verbatim words (`<<:WORD>>`) spelled by verbatim.
func spellLetterCase(tmpl string, keyword, verbatim func(string) string) string {
	return letterCaseWordRE.ReplaceAllStringFunc(tmpl, func(m string) string {
		sub := letterCaseWordRE.FindStringSubmatch(m)
		if sub[1] == ":" {
			return verbatim(sub[2])
		}
		return keyword(sub[2])
	})
}

func emitOf(t *testing.T, q string) string {
	t.Helper()
	if _, err := parse.Parse(q); err != nil {
		t.Fatalf("Parse(%q) = %v, want a parsed document", q, err)
	}
	pq, err := parse.ParseQuery(q)
	if err != nil {
		t.Fatalf("ParseQuery(%q) = %v, want a structured query", q, err)
	}
	out, err := pq.Emit()
	if err != nil {
		t.Fatalf("ParseQuery(%q).Emit() = %v, want canonical text", q, err)
	}
	return out
}

// TestKeywordsFunctionsAndClassesParseInAnyLetterCase — every keyword,
// operator and function name parses in any letter case and emits in its one
// canonical spelling, and an RM class name parses in any case and emits as
// written.
func TestKeywordsFunctionsAndClassesParseInAnyLetterCase(t *testing.T) {
	t.Parallel()
	for _, tmpl := range letterCaseTemplates {
		for _, c := range letterCasings {
			q := spellLetterCase(tmpl, c.fn, c.fn)
			t.Run(c.name+"/"+q, func(t *testing.T) {
				t.Parallel()
				got := emitOf(t, q)
				// The reference keeps the respelt class names but writes every
				// keyword and function in upper case, so the two texts differ only
				// where the emitter failed to canonicalise a keyword.
				want := emitOf(t, spellLetterCase(tmpl, strings.ToUpper, c.fn))
				if got != want {
					t.Errorf("Emit of %q\n got  %q\n want %q (the keyword-upper-case spelling's emission)", q, got, want)
				}
				// Equal texts would also pass if the emitter re-spelt class
				// names, so the verbatim words are checked on their own.
				for _, m := range letterCaseWordRE.FindAllStringSubmatch(tmpl, -1) {
					if m[1] != ":" {
						continue
					}
					spelt := c.fn(m[2])
					if !regexp.MustCompile(`\b` + regexp.QuoteMeta(spelt) + `\b`).MatchString(got) {
						t.Errorf("Emit of %q = %q, want %s kept as written", q, got, spelt)
					}
				}
			})
		}
	}
}

// TestAliasSpellingSurvivesEmission — an alias is an IDENTIFIER, so it parses
// in any letter case and Emit writes it as declared, in FROM / CONTAINS, in
// every path that uses it and in a SELECT `AS`.
func TestAliasSpellingSurvivesEmission(t *testing.T) {
	t.Parallel()
	q := "select COUNT(Obs) as Total, cOmP/uid/value as CompUid from EHR Ehr contains COMPOSITION cOmP" +
		" contains OBSERVATION Obs[openEHR-EHR-OBSERVATION.blood_pressure.v1]" +
		" where Ehr/ehr_id/value = 'x' order by CompUid"
	want := "SELECT COUNT(Obs) AS Total, cOmP/uid/value AS CompUid FROM EHR Ehr CONTAINS COMPOSITION cOmP" +
		" CONTAINS OBSERVATION Obs[openEHR-EHR-OBSERVATION.blood_pressure.v1]" +
		" WHERE Ehr/ehr_id/value = 'x' ORDER BY CompUid ASC"
	if got := emitOf(t, q); got != want {
		t.Errorf("Emit of %q\n got  %q\n want %q", q, got, want)
	}
}

// TestKeywordCasingCoverage keeps letterCaseTemplates complete: every token
// the active lexer spells with case-insensitive letter fragments, and every
// keyword spelt only inside a fragment, appears in some template. A keyword the
// grammar gains fails here until a template exercises its casing.
func TestKeywordCasingCoverage(t *testing.T) {
	t.Parallel()
	src, err := os.ReadFile(filepath.Join("..", "..", "..", "resources", "aql", "grammar", "active", "AqlLexer.g4"))
	if err != nil {
		t.Fatalf("read grammar: %v", err)
	}
	tokens := keywordTokenRE.FindAllStringSubmatch(string(src), -1)
	if len(tokens) < 40 {
		t.Fatalf("only %d keyword tokens matched in AqlLexer.g4: the extraction regexp has drifted", len(tokens))
	}
	covered := map[string]bool{}
	for _, tmpl := range letterCaseTemplates {
		for _, m := range letterCaseWordRE.FindAllStringSubmatch(tmpl, -1) {
			covered[strings.ToUpper(m[2])] = true
		}
	}
	want := []string{}
	for _, m := range tokens {
		want = append(want, m[1])
	}
	for _, w := range keywordFragmentTexts {
		want = append(want, strings.ToUpper(w))
	}
	for _, w := range want {
		if !covered[w] {
			t.Errorf("keyword %s is in the lexer but no letterCaseTemplates entry spells it", w)
		}
	}
}
