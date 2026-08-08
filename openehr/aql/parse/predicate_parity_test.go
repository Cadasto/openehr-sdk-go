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
	"testing"

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

	// --- VERSION position: versionPredicate's three alternatives ---
	{"version_latest", "SELECT v/data FROM VERSION v[LATEST_VERSION]"},
	{"version_all", "SELECT v/data FROM VERSION v[ALL_VERSIONS]"},
	{"version_standard", "SELECT v/data FROM VERSION v[commit_audit/time_committed > '2020']"},

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
