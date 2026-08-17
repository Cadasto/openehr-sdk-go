package parse_test

// REQ-119 · PROBE-090 — issue #99.
//
// predicate_parity_test.go holds the bracket-text positions to the REQ-119
// closure property. Two distinct defects meet at these positions and this file
// separates them deliberately:
//
//   - EXTRACTION fidelity. ANTLR's GetText() concatenates default-channel
//     tokens and DROPS hidden-channel whitespace, so `[a/b='c' AND d/e='f']`
//     came back `a/b='c'ANDd/e='f'`, which re-lexes with `ANDd` as one
//     IDENTIFIER and does not parse at all. That is a LOUD closure break, and
//     the corpus below is written in the whitespace-bearing forms a canonical
//     corpus never reaches (which is exactly why it went unseen).
//   - EMISSION guarding. The bracket text is spliced VERBATIM, so text that
//     terminates the bracket early re-parses as a DIFFERENT query with no
//     error. That is the SILENT class, and it is asserted in
//     predicate_guard_test.go rather than here.
//
// The oracle here is round-trip IDENTITY, not parseability: a spliced or
// mangled predicate can parse perfectly well as something else.

import (
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aql"
	"github.com/cadasto/openehr-sdk-go/openehr/aql/parse"
)

// predicateCorpus is every bracket-text shape the grammar admits in a position
// this SDK re-emits verbatim, written WITH the whitespace the grammar requires
// between keyword tokens.
//
// `pathPredicate` (the class and path positions) admits `standardPredicate |
// archetypePredicate | nodePredicate`; `versionPredicate` (the VERSION
// position) admits `LATEST_VERSION | ALL_VERSIONS | standardPredicate` and no
// node predicate at all. Both are represented, because one struct field
// (ClassExpr.Predicate) carries both.
var predicateCorpus = []struct {
	name string
	in   string
}{
	// --- class position: nodePredicate's whitespace-bearing alternatives ---
	{"class_node_and", "SELECT c/x FROM COMPOSITION c[a/b='c' AND d/e='f']"},
	{"class_node_or", "SELECT c/x FROM COMPOSITION c[a/b='c' OR d/e='f']"},
	{"class_node_matches_regex", "SELECT c/x FROM COMPOSITION c[a/b MATCHES {/re/}]"},
	{"class_node_and_or", "SELECT c/x FROM COMPOSITION c[a/b='c' AND d/e='f' OR g/h='i']"},

	// --- class position: forms that already round-tripped, as positive controls ---
	{"class_at_code", "SELECT c/x FROM COMPOSITION c[at0001]"},
	{"class_at_code_name", "SELECT c/x FROM COMPOSITION c[at0001,'name']"},
	{"class_param", "SELECT c/x FROM COMPOSITION c[$p]"},
	{"class_standard_cmp", "SELECT e/x FROM EHR e[ehr_id/value=$id]"},
	{"class_nested_bracket", "SELECT c/x FROM COMPOSITION c[a[at0001]/b='c']"},
	// TERM_CODE's display-name section, `'|' ~[|[\]]+ '|'`. Its content excludes
	// brackets and NOTHING else, so an apostrophe in a coded term — ordinary
	// clinical AQL, and the shape of half the SNOMED CT display names in use —
	// is content, not a string delimiter. Absent from the corpus, that read
	// refused a query ParseQuery had just produced.
	{"class_term_code", "SELECT c/x FROM COMPOSITION c[at0001,SNOMED-CT::22298006]"},
	{"class_term_code_name", "SELECT c/x FROM COMPOSITION c[at0001,SNOMED-CT::22298006|myocardial infarction|]"},
	{"class_term_code_quote", "SELECT c/x FROM COMPOSITION c[at0001,SNOMED-CT::22298006|Barrett's oesophagus|]"},
	{"class_term_code_brace", "SELECT c/x FROM COMPOSITION c[at0001,SNOMED-CT::22298006|dose{2}|]"},
	// A regex body ending in a backslash: `SLASH_REGEX_CHAR` makes a bare `\` an
	// ordinary body character, so the following `/` closes the body.
	{"class_regex_trailing_escape", `SELECT c/x FROM COMPOSITION c[a/b MATCHES {/\/}]`},
	// …and the same backslash with a BRACKET in front of it. This row is the
	// one that fails when the scan commits to reading `\/` as an escaped slash:
	// under that single reading the token cannot complete, the `{` becomes
	// ordinary content, and the `]` inside the body is counted as the class
	// bracket's delimiter — refusing a regex ParseQuery accepts.
	{"class_regex_bracket_then_escape", `SELECT c/x FROM COMPOSITION c[a/b MATCHES {/]\/}]`},
	// A standing comparison whose objectPath carries WHITESPACE inside a nested
	// predicate. The class bracket re-emits from ClassExpr.Predicate, so this
	// row is identity either way; what it pins is aql.Comparison.Path, which no
	// other row can distinguish from a token-stream concatenation.
	{"class_standing_cmp_spaced_path", "SELECT c/x FROM COMPOSITION c[a[at0001, 'n']/b = 'c']"},

	// --- VERSION position: versionPredicate's three alternatives ---
	{"version_latest", "SELECT v/data FROM VERSION v[LATEST_VERSION]"},
	{"version_all", "SELECT v/data FROM VERSION v[ALL_VERSIONS]"},
	{"version_standard", "SELECT v/data FROM VERSION v[commit_audit/time_committed > '2020']"},
	// The VERSION bracket belongs to `classExprOperand`, not to
	// `versionPredicate`, so a span over the child rule alone starts after the
	// padding and ends before it. These rows are identity only while the
	// extractor spans the ENCLOSING brackets — and they are what makes the
	// keyword check's trivia-tolerance load-bearing rather than incidental.
	{"version_padded", "SELECT v/data FROM VERSION v[ LATEST_VERSION ]"},
	{"version_padded_all", "SELECT v/data FROM VERSION v[  ALL_VERSIONS  ]"},
	{"version_comment", "SELECT v/data FROM VERSION v[LATEST_VERSION -- note\n]"},
	{"version_standard_padded", "SELECT v/data FROM VERSION v[ commit_audit/time_committed > '2020' ]"},
	// `UNICODE_BOM` is skipped like WS and COMMENT and is not anchored to the
	// start of input, so this is a query ParseQuery produces — and the keyword
	// comparison must see through it or the extractor's own output is refused.
	{"version_bom", "SELECT v/data FROM VERSION v[\uFEFFLATEST_VERSION]"},

	// --- source formatting rides through verbatim ---
	// The lexer SKIPS whitespace and comments (they are discarded, not put on
	// a hidden channel), so both live in the CHARACTER stream sourceText reads.
	// Echoing them is the fidelity property, not a defect: the emitted text is
	// what the caller handed us, so it necessarily re-parses. A comment's
	// terminating newline rides through with it, which is what keeps
	// `-- note` from swallowing the rest of the emitted query.
	{"fmt_inner_spacing", "SELECT c/x FROM COMPOSITION c[a/b  =  'c']"},
	{"fmt_newline", "SELECT c/x FROM COMPOSITION c[a/b='c'\n\tAND d/e='f']"},
	{"fmt_comment", "SELECT c/x FROM COMPOSITION c[a/b='c' -- note\n AND d/e='f']"},

	// --- path positions: the same bracket text, reached through a path ---
	// Guarding these against splice stays out of scope by REQ-055 rule 3;
	// not CORRUPTING them is extraction fidelity, the opposite direction.
	{"path_select", "SELECT c/items[a/b='c' AND d/e='f']/value FROM COMPOSITION c"},
	// `identifiedPath : IDENTIFIER pathPredicate? …` — the predicate on the
	// ROOT identifier is a different extraction site from a segment's, and no
	// other row puts whitespace in it.
	{"path_root_predicate", "SELECT c[a/b='c' AND d/e='f']/value FROM COMPOSITION c"},
	{"path_segment_deep", "SELECT c/content[at0001]/items[a/b='c' OR d/e='f']/value FROM COMPOSITION c"},
	{"path_where", "SELECT c/x FROM COMPOSITION c WHERE c/items[a/b='c' AND d/e='f']/v = 1"},
	{"path_order_by", "SELECT c/x FROM COMPOSITION c ORDER BY c/items[a/b='c' AND d/e='f']/v ASC"},
	{"path_matches_regex", "SELECT c/items[a/b MATCHES {/re/}]/value FROM COMPOSITION c"},
}

// TestPredicateSourceTextRoundTripsAsIdentity is the REQ-119 closure property
// at the bracket positions: what this SDK writes, it must read back — and read
// back as the SAME text, since a mangled predicate that still parses is the
// silent failure the identity oracle exists to catch.
func TestPredicateSourceTextRoundTripsAsIdentity(t *testing.T) {
	for _, tc := range predicateCorpus {
		t.Run(tc.name, func(t *testing.T) {
			q, err := parse.ParseQuery(tc.in)
			if err != nil {
				t.Fatalf("ParseQuery(%q) = %v; the corpus must be parseable AQL", tc.in, err)
			}
			out, err := q.Emit()
			if err != nil {
				t.Fatalf("Emit() after parsing %q = %v; a parsed query must re-emit", tc.in, err)
			}
			if out != tc.in {
				t.Errorf("round trip is not identity:\n  in  %q\n  out %q", tc.in, out)
			}
			if _, err := parse.ParseQuery(out); err != nil {
				t.Errorf("emitted text does not re-parse: %v\n  in  %q\n  out %q", err, tc.in, out)
			}
		})
	}
}

// TestPredicateRoundTripIsIdempotent is the weaker fixed-point property, kept
// beside the identity one so a future canonicalisation that legitimately
// rewrites a predicate still has to be STABLE.
func TestPredicateRoundTripIsIdempotent(t *testing.T) {
	for _, tc := range predicateCorpus {
		t.Run(tc.name, func(t *testing.T) {
			first, err := parse.ParseQuery(tc.in)
			if err != nil {
				t.Fatalf("ParseQuery(%q) = %v", tc.in, err)
			}
			once, err := first.Emit()
			if err != nil {
				t.Fatalf("first Emit() = %v", err)
			}
			second, err := parse.ParseQuery(once)
			if err != nil {
				t.Fatalf("re-parsing emitted %q = %v", once, err)
			}
			twice, err := second.Emit()
			if err != nil {
				t.Fatalf("second Emit() = %v", err)
			}
			if once != twice {
				t.Errorf("emit is not a fixed point:\n  once  %q\n  twice %q", once, twice)
			}
		})
	}
}

// TestPredicateSourceTextIsIdenticalOnBothExtractors — the SDK has TWO
// extractors over the same tree, and the fidelity fix had to land in both.
//
// [parse.Parse] builds the flat [parse.Document] view (the lint-only path,
// REQ-113 Tier 1) through `ast.go`; [parse.ParseQuery] builds the structured
// AST through `extract_query.go`. They are separate walks of the same parse
// tree, so a source-text rule applied to one and not the other is exactly the
// half-bound shape this REQ keeps paying for — and the whole corpus below only
// ever exercised the structured one.
func TestPredicateSourceTextIsIdenticalOnBothExtractors(t *testing.T) {
	for _, tc := range predicateCorpus {
		t.Run(tc.name, func(t *testing.T) {
			structured, err := parse.ParseQuery(tc.in)
			if err != nil {
				t.Fatalf("ParseQuery: %v", err)
			}
			flat, err := parse.Parse(tc.in)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}

			// Same classes, in the same order, carrying the same bracket text.
			var want []string
			for _, c := range structuredClasses(structured) {
				want = append(want, c.RMType+"|"+c.Alias+"|"+c.Predicate+"|"+c.Archetype)
			}
			var got []string
			for _, c := range flat.Classes {
				got = append(got, c.RMType+"|"+c.Alias+"|"+c.Predicate+"|"+c.Archetype)
			}
			if !slices.Equal(want, got) {
				t.Errorf("the two extractors disagree about the class predicates\n"+
					"  ParseQuery: %q\n  Parse:      %q", want, got)
			}

			// The path positions too. Document.Paths is the flat view's own
			// walk and has no structured counterpart to diff against, so the
			// property is asserted directly: text read from the CHARACTER
			// stream is a substring of the source, and a token-stream
			// concatenation — which drops the whitespace between keyword
			// tokens — is not.
			for _, ip := range flat.Paths {
				if !strings.Contains(tc.in, ip.Raw) {
					t.Errorf("Parse read path %q, which does not occur verbatim in the source; "+
						"the flat extractor is still reading the TOKEN stream", ip.Raw)
				}
				if ip.Predicate != "" && !strings.Contains(tc.in, ip.Predicate) {
					t.Errorf("Parse read path predicate %q, which does not occur verbatim "+
						"in the source", ip.Predicate)
				}
				// A SEGMENT's predicate is its own extraction site, reached
				// through a different context than the root's. Asserting the
				// root alone left both extractors' segment sites free to fall
				// back to the token stream.
				for _, seg := range ip.Segments {
					if seg.Predicate != "" && !strings.Contains(tc.in, seg.Predicate) {
						t.Errorf("Parse read path segment predicate %q, which does not occur "+
							"verbatim in the source", seg.Predicate)
					}
				}
			}

			// The STRUCTURED extractor's own path sites. The class diff above
			// covers only ClassExpr, and flat.Paths covers only the flat view,
			// so a SELECT path read from the token stream by ParseQuery alone
			// showed up in neither.
			//
			// With these, every sourceText site that can DIFFER from GetText()
			// fails a named test when it is swapped. The four that do not are
			// equivalent by construction rather than untested: the two
			// bare-keyword literal sites (`bareKeywordLiteral` accepts a single
			// IDENTIFIER, whose token text is its source span) and
			// bracketInterior's two nil-token fallbacks (the production emits
			// both brackets whenever the child rule is present).
			for _, item := range structured.Select.Items {
				pe, ok := item.Expr.(parse.PathExpr)
				if !ok {
					continue
				}
				if !strings.Contains(tc.in, pe.Raw) {
					t.Errorf("ParseQuery read SELECT path %q, which does not occur verbatim "+
						"in the source", pe.Raw)
				}
				if pe.Predicate != "" && !strings.Contains(tc.in, pe.Predicate) {
					t.Errorf("ParseQuery read SELECT path predicate %q, which does not occur "+
						"verbatim in the source", pe.Predicate)
				}
				for _, seg := range pe.Segments {
					if seg.Predicate != "" && !strings.Contains(tc.in, seg.Predicate) {
						t.Errorf("ParseQuery read SELECT path segment predicate %q, which does "+
							"not occur verbatim in the source", seg.Predicate)
					}
				}
			}

			// The STANDING COMPARISON's own path. It is lifted out of the same
			// class bracket, but re-emission reads ClassExpr.Predicate, so no
			// round-trip row can reach this site: it is read-side API and needs
			// its own assertion or a token-stream fallback here is invisible.
			for _, c := range structuredClasses(structured) {
				pc := c.PredicateComparison
				if pc == nil {
					continue
				}
				if !strings.Contains(tc.in, pc.Path) {
					t.Errorf("ParseQuery lifted standing-comparison path %q, which does not occur "+
						"verbatim in the source", pc.Path)
				}
				if pc.ParsedPath != nil && !strings.Contains(tc.in, pc.ParsedPath.Raw) {
					t.Errorf("ParseQuery lifted standing-comparison ParsedPath.Raw %q, which does "+
						"not occur verbatim in the source", pc.ParsedPath.Raw)
				}
				for _, seg := range structuredSegments(pc) {
					if seg.Predicate != "" && !strings.Contains(tc.in, seg.Predicate) {
						t.Errorf("ParseQuery lifted standing-comparison segment predicate %q, "+
							"which does not occur verbatim in the source", seg.Predicate)
					}
				}
			}
		})
	}
}

// structuredSegments returns a standing comparison's decomposed path segments,
// or nil when it carries none.
func structuredSegments(c *aql.Comparison) []aql.PathSegment {
	if c.ParsedPath == nil {
		return nil
	}
	return c.ParsedPath.Segments
}

// structuredClasses flattens the structured FROM tree in source order.
func structuredClasses(q *parse.Query) []parse.ClassExpr {
	var out []parse.ClassExpr
	var walk func(c *parse.Containment)
	walk = func(c *parse.Containment) {
		if c == nil {
			return
		}
		if c.Class.RMType != "" || c.Class.Version {
			out = append(out, c.Class)
		}
		for i := range c.Children {
			walk(&c.Children[i])
		}
	}
	if q.From.Root.RMType != "" || q.From.Root.Version {
		out = append(out, q.From.Root)
	}
	walk(q.From.Junction)
	walk(q.From.Contains)
	return out
}
