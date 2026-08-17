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
		// The rows above carry no `]` inside the region, so they pass even with
		// the branch that recognises the region deleted — the `{` becomes ordinary
		// content and nothing is miscounted. A `]` in the body is what makes each
		// branch load-bearing: get the region wrong and that `]` is counted as the
		// class bracket's delimiter.
		{"regex_bracket_with_flags_tail", "a/b MATCHES {/]/; 'i'}", false, "the `; STRING` tail must be part of the token"},
		{"regex_bracket_after_escaped_slash", `a/b MATCHES {/a\/]b/}`, false, "`'\\\\/'` puts a `/` INSIDE the body, so the `]` is content"},
		{"regex_bracket_leading_space", "a/b MATCHES { /]/}", false, "`'{' WS*` — the space is inside the token"},
		{"regex_bracket_trailing_space", "a/b MATCHES {/]/ }", false, "…and `WS* '}'` at the other end"},
		// Two body closings are reachable and the SHORTER one also completes, so
		// the token boundary depends on taking the LONGEST. With the shorter
		// reading the `]` falls outside the token and is counted.
		{"regex_two_closings_longest_wins", `a/b MATCHES {/a\/}]b/}`, false, "longest token, not longest body"},
		{"regex_shorter_body_longer_tail", `a/b MATCHES {/a\/;'x/}'}`, false, "a shorter body whose `; STRING` tail reaches further"},
		{"bracket_in_string", "a/b=']'", false, "a bracket the literal makes content"},
		{"escaped_quote", `a/b='it\'s'`, false, "ESCAPE_SEQ keeps the literal open"},
		{"comment", "a/b='c' -- note\n AND d/e='f'", false, "the lexer SKIPS comments into the source text"},

		{"escapes_bracket", "a/b='c'] CONTAINS OBSERVATION o[d/e='f'", true, "issue #99's splice"},
		{"bare_close", "]", true, "closes the emitter's bracket"},
		{"unclosed_open", "a[at0001", true, "the emitted `]` closes the INNER bracket"},
		{"unterminated_string", "a/b='c", true, "the emitted `]` becomes literal content"},
		{"unterminated_dq", `a/b="c`, true, "the other delimiter"},
		// CONTAINED_REGEX matches as a whole token or the `{` is SYM_LEFT_CURLY,
		// so a `{` that opens no regex at all escapes nothing: it is a CONTAINED
		// malformation the parser rejects loudly, and § The class predicate
		// positions reserves refusal for the silent mode.
		{"brace_not_a_regex", "a/b MATCHES {x}", false, "`{` that starts no regex is ordinary content"},
		// …but a body still OPEN at the end of the text is a different case, and
		// was wrongly grouped with the one above. `]` is an ordinary body
		// character, so the run does not stop at the emitter's `]` — it closes on
		// whatever `/` … `}` comes later in the EMITTED QUERY. Verified: this
		// text beside a contained `at0001 -- /}` re-parses with the whole
		// CONTAINS term swallowed into the regex, err == nil. The earlier reading
		// ("the token cannot complete, so `{` is SYM_LEFT_CURLY") held only for
		// the suffixes it was checked against.
		{"regex_body_open_at_end", "a/b MATCHES {/re", true, "the body absorbs the emitted `]` and closes later"},
		{"regex_body_open_brace_inside", "a/b MATCHES {/x}", true, "`}` is an ordinary body char, so this is still open"},
		{"regex_body_bare_slash_only", "a/b MATCHES {/", true, "the body has not even started a character yet"},
		// A body that closes but whose TAIL cannot complete is NOT open: after the
		// closing `/` the tail admits only WS, `;`, a STRING or `}`, and a `]` is
		// none of them, so the token dies and the `]` stays a real delimiter.
		{"regex_tail_incomplete", "a/b MATCHES {/re/", false, "the tail cannot absorb a `]`"},
		{"regex_escaped_slash", `a/b MATCHES {/\/}`, false, "a body ending in a backslash is legal"},
		// A backslash reads BOTH ways. Read as `'\\/'` the token cannot
		// complete and the `]` would be counted; read as `~[/\n\r]` the next
		// `/` closes the body and the whole thing is one CONTAINED_REGEX, which
		// is what the parser does — ANTLR takes the longest match.
		{"regex_bracket_then_escaped_slash", `a/b MATCHES {/]\/}`, false, "the longest match makes the `]` regex content"},
		// …and the converse, which is why every `/` cannot simply be treated as
		// a candidate body end: a BARE `/` is no body character, so this token
		// cannot complete at all and the `]` is a real delimiter.
		{"brace_with_a_bare_slash", "a/b MATCHES {/a/]/}", true, "`{` falls back to SYM_LEFT_CURLY, so the `]` closes the bracket"},
		{"term_code_name_quote", "at0001,SNOMED::73211009|Crohn's disease|", false, "`~[|[\\]]+` admits an apostrophe"},
		{"term_code_name_brace", "at0001,X::1|dose{2}|", false, "and a brace"},
		{"term_code_name_bracket", "at0001,X::1|a]b|", true, "but NOT a bracket, so the `]` is a delimiter"},
		// `TERM_CODE_CHAR : NAME_CHAR | '.'`, and the `-` half was pinned while
		// the `.` half was not — so dropping `'.'` refused ordinary clinical AQL:
		// every ICD-10 code is dotted, the code then ends at the dot, the display
		// name is no longer stepped over whole, and its apostrophe reads as a
		// string delimiter. The parser reads this back unchanged.
		{"term_code_dotted_code_with_name", "at0001,ICD10::E11.9|Barrett's oesophagus|", false, "`TERM_CODE_CHAR` admits `.` as well as `-`"},
		{"comment_unterminated", "a/b='c' -- x", true, "the emitted `]` falls inside the comment"},
		{"comment_bare_unterminated", "a/b='c'--", true, "the `--`/EOF form, still open in the query"},
		{"comment_closed", "a/b='c' -- x\n", false, "a newline closes it inside the text"},
		{"comment_hides_bracket", "at0001 -- ]\n AND a/b='c'", false, "a `]` inside a closed comment is not a delimiter"},
		{"double_dash_not_a_comment", "a/b='c'--]", true, "`--x` is SYM_DOUBLE_DASH, so the `]` is real"},
		// The body is `~[\r\n]*` and only `'\r'? '\n'` closes the token, so a
		// body ending on a lone CR is no comment at all. Both directions of
		// that were wrong while the body was searched for the next `\n`:
		{"comment_body_bare_cr", "a/b='c' -- x\r", false, "no comment, so nothing is unterminated — a LOUD malformation"},
		{"comment_bare_cr_then_bracket", "a/b='c' -- x\r] AND d/e='f'\n", true, "…and the `]` after it is a real delimiter"},
		{"comment_crlf", "a/b='c' -- x\r\n", false, "`'\\r'? '\\n'` is the closing form"},
		// The CR rule matters in the direction where the delimiter sits INSIDE the
		// body: the rows above put it after the `\r`, so a mutant that lets a lone
		// CR close the token still passes them. Here it would swallow a real `]`.
		{"lone_cr_pseudo_comment_hides_a_bracket", "a/b='c' -- ] \r", true, "a lone CR closes no COMMENT, so this `]` is a delimiter"},
		// The BARE `'--' ('\r'? '\n' | EOF)` alternative — distinct from the
		// `'-- ' body` one every row above exercises.
		{"comment_bare_newline", "at0001--\n", false, "`'--' '\\n'` is a whole COMMENT"},
		{"comment_bare_crlf", "at0001--\r\n", false, "…and `'--' '\\r' '\\n'`"},
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
		// The four rows above do NOT reach the junction arm: each carries two
		// top-level comparison operators (or none), so the COUNT arms refuse them
		// first and the junction detection could be deleted whole — case-folding
		// included — with the suite still green. These do reach it: exactly ONE
		// top-level operator with a non-blank operand on each side, so every
		// count arm passes and only the junction arm is left to say no. The
		// parser rejects all three, so accepting them would breach closure.
		{"junction_after_one_comparison", "a/b = 1 AND at0002", true},
		{"junction_or_after_one_comparison", "a/b = 1 OR at0002", true},
		{"junction_lowercase_after_one_comparison", "a/b = 1 and at0002", true},
		// …and the anti-tightening half: the letters alone are not the keyword.
		{"path_containing_and", "a/brand > 1", false},
		{"path_prefixed_by_and", "a/android = 1", false},
		{"path_prefixed_by_or", "a/order = 1", false},
		{"path_containing_and_underscored", "a/b_and_c = 1", false},
		{"keyword_inside_a_literal", "a/b = 'x AND y'", false},
		{"keyword_inside_a_nested_bracket", "a[b='c' AND d='e']/f = 1", false},
		// A comment is the third region the keyword must not fire inside. The
		// other two the amendment names — a regex and a top-level TERM_CODE —
		// are structurally unreachable HERE: `versionPredicate` has no MATCHES
		// alternative, and `pathPredicateOperand` is
		// `primitive | objectPath | PARAMETER | ID_CODE | AT_CODE`, with
		// TERM_CODE in none of them. Both are reachable only inside a nested
		// bracket, where the depth test already covers them.
		{"keyword_inside_a_comment", "a/b = 1 -- x AND y\n", false},
		{"term_code_inside_a_nested_bracket", "a[at0001,X::1|x AND y|]/b = 1", false},

		// The SHAPE of `standardPredicate : objectPath COMPARISON_OPERATOR
		// pathPredicateOperand` — exactly one top-level operator with an
		// operand on each side. REQ-119 § The class predicate positions holds
		// this position to its whole production, so these are refused here
		// though the equivalent text stays loud in the class position.
		{"missing_left_operand", "= 1", true},
		{"missing_right_operand", "a/b =", true},
		{"two_comparisons", "a/b = 1 = 2", true},
		{"juxtaposed_comparisons", "a/b = 1 b = 2", true},
		// `!=` is ONE operator (`SYM_NE`), so it must open at its `!`. Counting
		// only the `=` left the `!` on the LEFT of the span, which read as a
		// non-blank objectPath and let `VERSION v[!= 1]` reach the parser.
		{"missing_left_operand_ne", "!= 1", true},
		{"missing_left_operand_ne_tight", "!=1", true},
		{"missing_right_operand_ne", "a/b !=", true},
		// …and the anti-tightening half. `<=` and `>=` are ONE operator; a scan
		// that counts their characters reads two comparisons and refuses AQL
		// the parser accepts.
		{"operator_le", "a/b <= 1", false},
		{"operator_ge", "a/b >= 1", false},
		{"operator_ne", "a/b != 1", false},
		{"operator_lt", "a/b < 1", false},
		{"operator_inside_a_literal", "a/b = 'x=y'", false},
		{"operator_inside_a_nested_bracket", "a[c=1]/b = 2", false},
		// The line the amendment draws: the position's SHAPE is decided, its
		// OPERANDS are not. `NOT a/b` is a malformed objectPath, and deciding
		// that costs the sub-grammar parser this REQ refuses to build —
		// `pathPart` reaches `nodePredicate`, which reaches `objectPath` again.
		// So it stays a LOUD parser error, and this row says so on purpose.
		{"malformed_operand_stays_loud", "NOT a/b = 1", false},
		// The same line, reached through the operator spellings rather than a
		// keyword: a lone `!` spells no token, so `!<=` is one `<=` operator with
		// `!` for a left operand — a malformed objectPath, not a missing one.
		// Refusing these would need the objectPath parser the exemption buys off,
		// so they stay loud, and these rows mark the boundary as a decision.
		{"lone_bang_is_a_malformed_operand", "!<= 1", false},
		{"double_bang_is_a_malformed_operand", "!!= 1", false},
		{"spaced_bang_is_a_malformed_operand", "! = 1", false},

		// `UNICODE_BOM` is skipped anywhere, not only at the start of input, so
		// the keyword comparison must see through it — the extractor produces
		// this text from a query that parses.
		{"bom_prefix", "\uFEFFLATEST_VERSION", false},
		{"bom_suffix", "LATEST_VERSION\uFEFF", false},
		// `UNICODE_BOM` has THREE alternatives and only the U+FEFF one was pinned,
		// so the other two could be dropped with the suite green. They are
		// upstream spelling quirks rather than BOMs \u2014 ANTLR's `\u` escape takes
		// exactly four hex digits, so `'\uEFBBBF'` is U+EFBB then the letters
		// `BF` \u2014 and the lexer skips what the rule SAYS, so the guard must accept
		// what the lexer skips.
		{"bom_utf8_written_form", "\uEFBB" + "BFLATEST_VERSION", false},
		{"bom_utf32_written_form", "\x00" + "FEFFLATEST_VERSION", false},
		// The bare-`--` COMMENT form reaches the keyword comparison through
		// StripPredicateTrivia, which is a separate arm from the `'-- ' body` one.
		{"keyword_then_bare_comment", "LATEST_VERSION--\n", false},
		{"keyword_then_bare_comment_crlf", "LATEST_VERSION--\r\n", false},
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
