package aql

// predicate.go — REQ-119, issue #99. The class BRACKET text the emitters splice
// verbatim, and the guards that keep splice text out of it.
//
// This position is not the single-token one identifier.go guards. One struct
// field ([parse.ClassExpr.Predicate]) carries TWO grammar positions:
//
//	classExprOperand : IDENTIFIER variable=IDENTIFIER? pathPredicate?
//	                 | VERSION variable=IDENTIFIER? ('[' versionPredicate ']')?
//	pathPredicate    : '[' (standardPredicate | archetypePredicate | nodePredicate) ']'
//	versionPredicate : LATEST_VERSION | ALL_VERSIONS | standardPredicate
//
// and they do not have the same accept set: `versionPredicate` admits no node
// predicate at all. A guard that treats the field uniformly is wrong in both
// directions at once — it lets `VERSION v[at0001]` reach the wire (the parser
// rejects it) and it refuses `VERSION v[LATEST_VERSION]` (which the extractor
// itself produces). So the two positions get two guards.
//
// Neither is a sub-grammar PARSER. `nodePredicate` lawfully carries AND, OR,
// quotes and spaces, so no alphabet separates a legal predicate from a splice
// and a conservative approximation would refuse predicates ParseQuery accepts —
// the tightening failure REQ-119 guards against as squarely as the splice. What
// IS decidable, and what the guards check, is the condition that actually
// distinguishes the two failure modes:
//
//   - Emission writes `"[" + Predicate + "]"`, so the ONLY way the text can
//     change the query's STRUCTURE is by terminating that bracket early. That
//     is the silent class — text the parser accepts as something else — and it
//     is exactly what [ValidatePathPredicate] refuses.
//   - Text that stays inside its brackets can at worst be a malformed
//     predicate, which the parser rejects LOUDLY. REQ-119 reserves refusal for
//     the silent mode, so a contained malformation is left to the parser.
//
// The rules are hand-derived because `openehr/aql` may not import the generated
// lexer (REQ-013 — the dependency runs the other way), and are held honest
// mechanically in openehr/aql/parse/predicate_guard_test.go: round-trip
// IDENTITY is the oracle, since a spliced predicate parses perfectly well as
// something else.

import (
	"fmt"
	"strings"
)

// ValidateVersionPredicate refuses bracket text that `versionPredicate` cannot
// carry, returning an error wrapping [ErrInvalidQuery].
//
// The position is `VERSION variable=IDENTIFIER? ('[' versionPredicate ']')?`
// and `versionPredicate : LATEST_VERSION | ALL_VERSIONS | standardPredicate` —
// three alternatives, none of them a node predicate. `VERSION v[at0001]` is
// therefore text the SDK's own parser rejects, which the REQ-119 closure
// property forbids emitting.
//
// The keyword test is case-INSENSITIVE because the lexer builds both keywords
// out of case-insensitive letter fragments (`LATEST_VERSION : L A T E S T '_'
// V E R S I O N`), so `latest_version` is the same token.
//
// For `standardPredicate : objectPath COMPARISON_OPERATOR pathPredicateOperand`
// the check is the NECESSARY condition — a top-level comparison operator —
// rather than a parse of the production. That direction is the safe one: no
// standard predicate lacks the operator, so nothing ParseQuery accepts is
// refused, and text that carries one but is malformed anyway stays a loud
// parser error.
func ValidateVersionPredicate(text string) error {
	if err := ValidatePathPredicate(text); err != nil {
		return err
	}
	body := strings.TrimSpace(text)
	if strings.EqualFold(body, "LATEST_VERSION") || strings.EqualFold(body, "ALL_VERSIONS") {
		return nil
	}
	if scanPredicate(text).topLevelCmp {
		return nil
	}
	return fmt.Errorf("%w: VERSION predicate %q is neither LATEST_VERSION, ALL_VERSIONS, "+
		"nor a `<path> <op> <operand>` comparison; `versionPredicate` admits no node predicate, "+
		"so this emits text the parser rejects", ErrInvalidQuery, text)
}

// ValidatePathPredicate refuses bracket text that can ESCAPE the brackets the
// emitter writes around it, returning an error wrapping [ErrInvalidQuery].
//
// Left unguarded, a caller string that closes the bracket early re-parses as a
// different query with err == nil — REQ-119's silent-substitution class:
//
//	Predicate: "a/b='c'] CONTAINS OBSERVATION o[d/e='f'"
//	  -> FROM COMPOSITION c[a/b='c'] CONTAINS OBSERVATION o[d/e='f']
//
// Three states make a bracket NOT a delimiter, and the scan tracks each so the
// guard neither miscounts nor refuses legal text:
//
//   - A string literal. `[` and `]` inside `'…'` / `"…"` are content. An
//     UNTERMINATED one is refused, because the emitter's own `]` would fall
//     inside it and the literal would run on into the following clause — a
//     silent substitution rather than a loud error.
//   - A contained regex. `SLASH_REGEX_CHAR : ~[/\n\r] | ESCAPE_SEQ | '\\/'`
//     admits both brackets freely, so `a/b MATCHES {/[0-9]+/}` must not be
//     counted.
//   - A nested path predicate. `objectPath` is `pathPart (‘/’ pathPart)*` and
//     `pathPart : IDENTIFIER pathPredicate?`, so `a[at0001]/b='c'` is a legal
//     predicate carrying a BALANCED bracket pair.
//
// Balance must hold in both directions. A trailing unclosed `[` is not merely a
// loud error: the emitter's `]` closes the INNER bracket and the outer one then
// swallows text up to whatever `]` appears later in the query, which is a
// substitution again.
//
// `TERM_CODE`'s trailing `|…|` section needs no case of its own — the grammar
// spells its content `~[|[\]]+`, which already excludes both brackets.
func ValidatePathPredicate(text string) error {
	sc := scanPredicate(text)
	switch {
	case sc.unterminated != "":
		return fmt.Errorf("%w: predicate %q leaves a %s unterminated; the emitted `]` would fall "+
			"inside it and the predicate would run on into the following clause",
			ErrInvalidQuery, text, sc.unterminated)
	case sc.escapes:
		return fmt.Errorf("%w: predicate %q is not bracket-balanced; splicing it would terminate "+
			"the class predicate early and re-parse as a different query",
			ErrInvalidQuery, text)
	}
	return nil
}

// predicateScan is what one pass over a bracket text tells the two guards.
type predicateScan struct {
	// escapes reports that the text can terminate the emitter's own bracket —
	// an unbalanced `]` (closing it early) or an unclosed `[` (letting it
	// close on a later one).
	escapes bool
	// unterminated names the construct left open ("" when none), which makes
	// the emitted `]` content rather than a delimiter.
	unterminated string
	// topLevelCmp reports a COMPARISON_OPERATOR outside every literal, regex
	// and nested bracket — the necessary condition for `standardPredicate`.
	topLevelCmp bool
}

// scanPredicate walks the bracket text once. It is a lexical scan, not a parse:
// it decides only which characters are DELIMITERS and which are content.
func scanPredicate(text string) predicateScan {
	var sc predicateScan
	depth := 0
	for i := 0; i < len(text); {
		switch c := text[i]; c {
		case '\'', '"':
			j, ok := skipPredicateString(text, i)
			if !ok {
				sc.unterminated = "string literal"
				return sc
			}
			i = j
		case '{':
			j, ok := skipPredicateRegex(text, i)
			if !ok {
				sc.unterminated = "contained regex"
				return sc
			}
			i = j
		case '[':
			depth++
			i++
		case ']':
			depth--
			if depth < 0 {
				sc.escapes = true
				return sc
			}
			i++
		case '=', '<', '>':
			// COMPARISON_OPERATOR is `= != > >= < <=`; every spelling carries
			// one of these three, and `!` occurs in no other token, so testing
			// them alone is the same set.
			if depth == 0 {
				sc.topLevelCmp = true
			}
			i++
		case '-':
			// A comment (`-- …` to end of line) is SKIPPED by the lexer, not
			// put on a hidden channel, so it survives in the source text the
			// extractor now reads. Stepping over it keeps a `]` inside a
			// comment from being counted as a delimiter.
			if n := commentLen(text, i); n > 0 {
				i += n
				continue
			}
			i++
		default:
			i++
		}
	}
	if depth != 0 {
		sc.escapes = true
	}
	return sc
}

// skipPredicateString steps over a STRING token starting at the delimiter, and
// reports whether it was closed. ESCAPE_SEQ, UTF8CHAR and OCTAL_ESC all begin
// with a backslash, so one rule covers all three: a backslash consumes the byte
// after it, whatever that byte is.
func skipPredicateString(s string, i int) (int, bool) {
	quote := s[i]
	for j := i + 1; j < len(s); j++ {
		switch s[j] {
		case '\\':
			j++
		case quote:
			return j + 1, true
		}
	}
	return 0, false
}

// skipPredicateRegex steps over `CONTAINED_REGEX : '{' WS* SLASH_REGEX WS*
// (';' WS* STRING)? WS* '}'`, and reports whether it was well formed.
//
// The `}` cannot simply be searched for: `SLASH_REGEX_CHAR : ~[/\n\r] | …`
// admits `}` inside the pattern (`{/a{2}/}`), so the regex body has to be
// stepped over on its own terms. A `{` that begins no well-formed
// CONTAINED_REGEX is reported as unterminated — safe, because `pathPredicate`
// reaches `{` through `objectPath MATCHES CONTAINED_REGEX` and nowhere else, so
// no text ParseQuery accepts is refused by it.
func skipPredicateRegex(s string, i int) (int, bool) {
	j := skipPredicateSpace(s, i+1)
	if j >= len(s) || s[j] != '/' {
		return 0, false
	}
	j++
	closed := false
	for ; j < len(s); j++ {
		if s[j] == '\\' {
			j++
			continue
		}
		if s[j] == '/' {
			j++
			closed = true
			break
		}
	}
	if !closed {
		return 0, false
	}
	j = skipPredicateSpace(s, j)
	// The optional `; 'flags'` tail.
	if j < len(s) && s[j] == ';' {
		j = skipPredicateSpace(s, j+1)
		if j >= len(s) || (s[j] != '\'' && s[j] != '"') {
			return 0, false
		}
		var ok bool
		if j, ok = skipPredicateString(s, j); !ok {
			return 0, false
		}
		j = skipPredicateSpace(s, j)
	}
	if j >= len(s) || s[j] != '}' {
		return 0, false
	}
	return j + 1, true
}

// skipPredicateSpace steps over what the lexer discards between tokens — `WS`
// and a `--` comment — so the regex scan above lines up with the real one.
func skipPredicateSpace(s string, i int) int {
	for i < len(s) {
		switch s[i] {
		case ' ', '\t', '\r', '\n':
			i++
		case '-':
			n := commentLen(s, i)
			if n == 0 {
				return i
			}
			i += n
		default:
			return i
		}
	}
	return i
}

// commentLen returns the length of the `COMMENT` token starting at i, or 0 when
// none does. The grammar spells it `'--' ' ' ~[\r\n]* (…)` or `'--' (…)`, i.e.
// a double dash running to the end of the line (or of the input).
func commentLen(s string, i int) int {
	if i+1 >= len(s) || s[i] != '-' || s[i+1] != '-' {
		return 0
	}
	if nl := strings.IndexAny(s[i:], "\r\n"); nl >= 0 {
		return nl
	}
	return len(s) - i
}
