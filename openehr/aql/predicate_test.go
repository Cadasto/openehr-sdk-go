package aql_test

// REQ-119 · PROBE-090 — issue #99.
//
// The exported half of the class-predicate guards. REQ-119 requires the SDK to
// expose its validity checks so a caller — and any write path outside
// `openehr/aql` — can hold the same line the emitters hold, so these are public
// API and are tested as such. Their agreement with the real grammar is
// confronted in openehr/aql/parse (which may import the generated lexer; this
// package may not, REQ-013).

import (
	"errors"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aql"
)

func TestValidatePathPredicate(t *testing.T) {
	tests := []struct {
		name    string
		text    string
		wantErr bool
		// why names the scanner state the case exercises, so a failure says
		// which rule broke rather than only which string.
		why string
	}{
		{"standard_comparison", "ehr_id/value=$id", false, "the ordinary case"},
		{"node_code", "at0001", false, "a node predicate"},
		{"node_code_with_name", "at0001,'name'", false, "the STRING alternative"},
		{"junction", "a/b='c' AND d/e='f'", false, "AND is lawful inside a node predicate"},
		{"nested_predicate", "a[at0001]/b='c'", false, "objectPath nests pathPredicate — a BALANCED pair"},
		{"regex_char_class", "a/b MATCHES {/[0-9]+/}", false, "SLASH_REGEX_CHAR admits both brackets"},
		{"regex_quantifier", "a/b MATCHES {/a{2}/}", false, "and admits the closing brace too"},
		{"regex_flags_tail", "a/b MATCHES {/re/; 'i'}", false, "the optional `; STRING` tail"},
		{"bracket_in_string", "a/b=']'", false, "a bracket the literal makes content"},
		{"escaped_quote", `a/b='it\'s'`, false, "ESCAPE_SEQ keeps the literal open"},
		{"comment", "a/b='c' -- note\n AND d/e='f'", false, "the lexer SKIPS comments into the source text"},

		{"escapes_bracket", "a/b='c'] CONTAINS OBSERVATION o[d/e='f'", true, "issue #99's splice"},
		{"bare_close", "]", true, "closes the emitter's bracket"},
		{"unclosed_open", "a[at0001", true, "the emitted `]` closes the INNER bracket"},
		{"unterminated_string", "a/b='c", true, "the emitted `]` becomes literal content"},
		{"unterminated_dq", `a/b="c`, true, "the other delimiter"},
		// CONTAINED_REGEX matches as a whole token or the `{` is SYM_LEFT_CURLY,
		// so neither of these escapes anything: both are CONTAINED malformations
		// the parser rejects loudly, and § The class predicate positions reserves
		// refusal for the silent mode. Both were pinned as refusals and both
		// refused text `[a/b MATCHES {/re]` proves is loud, not substituted.
		{"brace_not_a_regex", "a/b MATCHES {x}", false, "`{` that starts no regex is ordinary content"},
		{"regex_body_unclosed", "a/b MATCHES {/re", false, "the token cannot complete, so `{` is SYM_LEFT_CURLY"},
		{"regex_escaped_slash", `a/b MATCHES {/\/}`, false, "a body ending in a backslash is legal"},
		{"term_code_name_quote", "at0001,SNOMED::73211009|Crohn's disease|", false, "`~[|[\\]]+` admits an apostrophe"},
		{"term_code_name_brace", "at0001,X::1|dose{2}|", false, "and a brace"},
		{"term_code_name_bracket", "at0001,X::1|a]b|", true, "but NOT a bracket, so the `]` is a delimiter"},
		{"comment_unterminated", "a/b='c' -- x", true, "the emitted `]` falls inside the comment"},
		{"comment_bare_unterminated", "a/b='c'--", true, "the `--`/EOF form, still open in the query"},
		{"comment_closed", "a/b='c' -- x\n", false, "a newline closes it inside the text"},
		{"comment_hides_bracket", "at0001 -- ]\n AND a/b='c'", false, "a `]` inside a closed comment is not a delimiter"},
		{"double_dash_not_a_comment", "a/b='c'--]", true, "`--x` is SYM_DOUBLE_DASH, so the `]` is real"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := aql.ValidatePathPredicate(tc.text)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ValidatePathPredicate(%q) = nil, want an error (%s)", tc.text, tc.why)
				}
				if !errors.Is(err, aql.ErrInvalidQuery) {
					t.Errorf("error %v does not wrap ErrInvalidQuery", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ValidatePathPredicate(%q) = %v, want nil (%s)", tc.text, err, tc.why)
			}
		})
	}
}

func TestValidateVersionPredicate(t *testing.T) {
	tests := []struct {
		name    string
		text    string
		wantErr bool
	}{
		{"latest", "LATEST_VERSION", false},
		{"all", "ALL_VERSIONS", false},
		// The keywords are built from case-insensitive letter fragments.
		{"latest_lower", "latest_version", false},
		{"all_mixed", "All_Versions", false},
		{"padded", "  LATEST_VERSION  ", false},
		{"standard_predicate", "commit_audit/time_committed > '2020'", false},
		{"standard_predicate_tight", "commit_audit/time_committed>'2020'", false},
		{"standard_predicate_ne", "commit_audit/committer != $who", false},

		// `versionPredicate : LATEST_VERSION | ALL_VERSIONS | standardPredicate`
		// — no nodePredicate alternative at all.
		{"node_code", "at0001", true},
		{"node_code_with_name", "at0001,'name'", true},
		{"param", "$p", true},
		{"archetype", "openEHR-EHR-COMPOSITION.encounter.v1", true},
		// A near-miss keyword must not pass on a prefix match.
		{"keyword_prefix", "LATEST_VERSIONS", true},
		{"keyword_suffix", "XLATEST_VERSION", true},
		// The escape scan still applies here — and the row must carry a
		// top-level comparison operator, or the KEYWORD arm refuses it whether
		// or not the escape scan runs and the row tests nothing. That is what
		// the previous spelling did.
		{"escapes_bracket", "commit_audit/time > '2020'] CONTAINS COMPOSITION c[at0001", true},
		// `versionPredicate` has no junction alternative, so a top-level AND /
		// OR / MATCHES is never legal here — unlike the class position, where
		// nodePredicate makes it lawful. REQ-119 § Amends the VERSION position.
		{"junction_and", "a/b='c' AND d/e='f'", true},
		{"junction_or", "a/b='c' OR d/e='f'", true},
		{"junction_lowercase", "a/b='c' and d/e='f'", true},
		{"junction_matches", "a/b MATCHES {/re/}", true},
		// …and the anti-tightening half: the letters alone are not the keyword.
		{"path_containing_and", "a/brand > 1", false},
		{"path_prefixed_by_and", "a/android = 1", false},
		{"path_prefixed_by_or", "a/order = 1", false},
		{"path_containing_and_underscored", "a/b_and_c = 1", false},
		{"keyword_inside_a_literal", "a/b = 'x AND y'", false},
		{"keyword_inside_a_nested_bracket", "a[b='c' AND d='e']/f = 1", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := aql.ValidateVersionPredicate(tc.text)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ValidateVersionPredicate(%q) = nil, want an error", tc.text)
				}
				if !errors.Is(err, aql.ErrInvalidQuery) {
					t.Errorf("error %v does not wrap ErrInvalidQuery", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ValidateVersionPredicate(%q) = %v, want nil", tc.text, err)
			}
		})
	}
}

// TestValidatePredicateErrorsNameTheText keeps a refusal actionable: the caller
// gets the offending text back, not only a category.
func TestValidatePredicateErrorsNameTheText(t *testing.T) {
	const bad = "a/b='c'] CONTAINS OBSERVATION o[d/e='f'"
	for name, err := range map[string]error{
		"path":    aql.ValidatePathPredicate(bad),
		"version": aql.ValidateVersionPredicate(bad),
	} {
		if err == nil {
			t.Fatalf("%s: want an error", name)
		}
		if !strings.Contains(err.Error(), bad) {
			t.Errorf("%s: error %q does not quote the refused text", name, err)
		}
	}
}
