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
	body := stripPredicateTrivia(text)
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
			// `CONTAINED_REGEX` matches as a WHOLE token or not at all — ANTLR
			// takes the longest match, and when the token cannot complete, the
			// `{` falls back to `SYM_LEFT_CURLY` and a later `]` stays a real
			// delimiter. Verified: `[a/b MATCHES {/re]` is a LOUD error, not a
			// substitution. So a `{` that begins no complete regex is ordinary
			// CONTENT, and refusing it would refuse a contained malformation,
			// which this REQ reserves for the parser.
			if j, ok := skipContainedRegex(text, i); ok {
				i = j
				continue
			}
			i++
		case ':':
			// `TERM_CODE : TERM_CODE_CHAR+ … '::' TERM_CODE_CHAR+ ('|' … '|')?`
			// with `TERM_CODE_CHAR : NAME_CHAR | '.'`, and NAME_CHAR admits '-'.
			// So a `--` INSIDE a term code is not a comment: maximal munch takes
			// both dashes into the code. Consuming the tail here is what keeps
			// `at0001,X::1--` — which the parser reads back unchanged — from
			// being refused, while `at0001--` (no code, so the dashes ARE a
			// comment) still is.
			if i+1 < len(text) && text[i+1] == ':' {
				j := i + 2
				for j < len(text) && termCodeChar(text[j]) {
					j++
				}
				// The display-name section, if any. It is reached ONLY from
				// here: `|` occurs in no other predicate construct, so a
				// standalone case for it was unreachable and no mutation could
				// kill it.
				if j < len(text) && text[j] == '|' {
					if k, ok := skipTermCodeName(text, j); ok {
						j = k
					}
				}
				i = j
				continue
			}
			i++
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
			// A comment is SKIPPED by the lexer, not put on a hidden channel, so
			// it survives in the source text the extractor reads. Stepping over
			// it keeps a `]` inside a comment from being counted — but only when
			// a NEWLINE closes it inside this text. Closed by the end of the text
			// it is not closed at all: the text is spliced into the middle of a
			// query, so the run continues to the next newline THERE, swallowing
			// the emitter's own `]` exactly as an unterminated literal would.
			n, closed := commentRun(text, i)
			if n == 0 {
				i++
				continue
			}
			if !closed {
				sc.unterminated = "comment"
				return sc
			}
			i += n
		default:
			i++
		}
	}
	if depth != 0 {
		sc.escapes = true
	}
	return sc
}

// skipTermCodeName steps over `TERM_CODE`'s optional trailing display name,
// `'|' ~[|[\]]+ '|'`, and reports whether one is there.
//
// The content class excludes both brackets, so a `]` before the closing `|`
// means this is NOT that section and the `]` is a real delimiter — verified:
// `c[at0001,X::1|a] CONTAINS …` is a loud token-recognition error, so falling
// through to ordinary scanning is both safe and correct.
// termCodeChar spells `TERM_CODE_CHAR : NAME_CHAR | '.'`, i.e. a word
// character, '-' or '.'.
func termCodeChar(c byte) bool {
	return c == '_' || c == '-' || c == '.' ||
		(c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func skipTermCodeName(s string, i int) (int, bool) {
	for j := i + 1; j < len(s); j++ {
		switch s[j] {
		case '|':
			if j == i+1 {
				return 0, false // `~[|[\]]+` needs at least one character
			}
			return j + 1, true
		case '[', ']':
			return 0, false
		}
	}
	return 0, false
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

// skipContainedRegex matches `CONTAINED_REGEX : '{' WS* SLASH_REGEX WS*
// (';' WS* STRING)? WS* '}'` as a WHOLE token, and reports whether one is
// there.
//
// Whole-token or nothing is the point. ANTLR takes the longest match, so a `{`
// that cannot complete the production is simply `SYM_LEFT_CURLY` and every
// character after it — a `]` included — is lexed on its own terms. Reporting
// such a `{` as an unterminated region refused text that escapes nothing:
// `a/b MATCHES {x}` is a CONTAINED malformation, which § The class predicate
// positions requires be left to the parser.
//
// The `}` cannot simply be searched for: `SLASH_REGEX_CHAR : ~[/\n\r] |
// ESCAPE_SEQ | '\\/'` admits `}` inside the pattern (`{/a{2}/}`), so the body
// is stepped over on its own terms.
func skipContainedRegex(s string, i int) (int, bool) {
	j := skipRegexSpace(s, i+1)
	if j >= len(s) || s[j] != '/' {
		return 0, false
	}
	end := regexBodyEnd(s, j+1)
	if end < 0 {
		return 0, false
	}
	return regexTail(s, end+1)
}

// regexBodyEnd returns the index of the `/` closing `SLASH_REGEX`'s body, or -1.
//
// A backslash consumes the character after it, covering both `ESCAPE_SEQ` and
// the explicit `'\\/'` alternative that keeps an escaped slash from closing the
// body. `~[/\n\r]` excludes the line breaks, so a body carrying one is no body
// at all.
func regexBodyEnd(s string, start int) int {
	for j := start; j < len(s); j++ {
		switch s[j] {
		case '\n', '\r':
			return -1
		case '\\':
			j++
		case '/':
			if j == start {
				return -1 // SLASH_REGEX_CHAR+ needs at least one character
			}
			return j
		}
	}
	return -1
}

// regexTail matches the token's remainder after the body's closing `/`:
// `WS* (';' WS* STRING)? WS* '}'`.
func regexTail(s string, j int) (int, bool) {
	j = skipRegexSpace(s, j)
	if j < len(s) && s[j] == ';' {
		j = skipRegexSpace(s, j+1)
		if j >= len(s) || (s[j] != '\'' && s[j] != '"') {
			return 0, false
		}
		var ok bool
		if j, ok = skipPredicateString(s, j); !ok {
			return 0, false
		}
		j = skipRegexSpace(s, j)
	}
	if j >= len(s) || s[j] != '}' {
		return 0, false
	}
	return j + 1, true
}

// skipRegexSpace steps over the `WS*` inside `CONTAINED_REGEX`. WHITESPACE
// ONLY: the region is a single lexer TOKEN, so a `COMMENT` — itself a token —
// cannot occur inside it, and skipping one here would model a stream the lexer
// never produces.
func skipRegexSpace(s string, i int) int {
	for i < len(s) && (s[i] == ' ' || s[i] == '\t' || s[i] == '\r' || s[i] == '\n') {
		i++
	}
	return i
}

// commentRun returns the length of the `COMMENT` run starting at i (0 when
// none does) and whether a NEWLINE closed it inside this text.
//
// The grammar is `COMMENT : '--' ' ' ~[\r\n]* ('\r'? '\n' | EOF) | '--' ('\r'?
// '\n' | EOF)`, and both halves matter:
//
//   - The SPACE is mandatory in the first alternative. `--x` is no comment at
//     all but `SYM_DOUBLE_DASH`, so the characters after it — a `]` included —
//     are ordinary tokens. Treating it as a comment skipped a real delimiter.
//   - EOF means the end of the QUERY, not the end of this text. A run that no
//     newline closes here goes on to close somewhere in the emitted query,
//     which is why the caller refuses it rather than stepping over it.
func commentRun(s string, i int) (int, bool) {
	if i+1 >= len(s) || s[i] != '-' || s[i+1] != '-' {
		return 0, false
	}
	rest := s[i+2:]
	switch {
	case rest == "":
		return len(s) - i, false // closed only by the query's own EOF
	case rest[0] == '\n':
		return 3, true
	case rest[0] == '\r':
		if len(rest) > 1 && rest[1] == '\n' {
			return 4, true
		}
		return 0, false // a lone '\r' does not close a COMMENT
	case rest[0] != ' ':
		return 0, false // SYM_DOUBLE_DASH, not a comment
	}
	if nl := strings.IndexByte(s[i:], '\n'); nl >= 0 {
		return nl + 1, true
	}
	return len(s) - i, false
}

// stripPredicateTrivia removes the comment runs the lexer discards, so a
// keyword comparison sees what the parser sees. `LATEST_VERSION -- note\n` is
// the same token stream as `LATEST_VERSION`, and trimming whitespace alone
// refused it.
func stripPredicateTrivia(text string) string {
	var b strings.Builder
	for i := 0; i < len(text); {
		if n, closed := commentRun(text, i); n > 0 && closed {
			i += n
			b.WriteByte(' ')
			continue
		}
		b.WriteByte(text[i])
		i++
	}
	return strings.TrimSpace(b.String())
}
