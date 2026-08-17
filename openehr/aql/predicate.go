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
//     the silent mode, so a contained malformation is left to the parser. One
//     recorded exception escapes BOTH classes: a regex token that completes
//     here AND leaves its body reachable (`{/a\/}`) forms a longer token under
//     a cooperating suffix — a whole-query property no per-predicate scan can
//     decide (REQ-119 § Out of scope; issue #103 tracks the verify-after-emit
//     closure).
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
// V E R S I O N`), so `latest_version` is the same token. The fragments are
// ASCII (`S : [sS]`), so the fold is held to that alphabet — see
// [asciiKeyword] for the spelling Unicode folding wrongly admitted.
//
// This position is held to its whole PRODUCTION rather than to a necessary
// condition, and is the one place REQ-119 refuses a LOUD malformation: the
// three alternatives are two keywords and one comparison, so what the position
// admits STRUCTURALLY is decidable in a single pass and the closure clause
// governs. `standardPredicate : objectPath COMPARISON_OPERATOR
// pathPredicateOperand` is therefore checked as exactly ONE top-level
// comparison operator with a non-blank operand on each side — `= 1` and
// `a/b = 1 b = 2` are shapes no `versionPredicate` has.
//
// Its OPERANDS are a different matter and stay loud. `objectPath` recurses —
// `pathPart : IDENTIFIER pathPredicate?` reaches `nodePredicate`, which reaches
// `objectPath` again — so deciding whether `NOT a/b` is a legal left operand
// costs the sub-grammar parser § The class predicate positions refuses to
// build. The SHAPE is decided here; the operands are left to the parser.
func ValidateVersionPredicate(text string) error {
	if err := ValidatePathPredicate(text); err != nil {
		return err
	}
	body := StripPredicateTrivia(text)
	if asciiKeyword(body, "LATEST_VERSION") || asciiKeyword(body, "ALL_VERSIONS") {
		return nil
	}
	sc := scanPredicate(text)
	switch {
	case sc.topLevelJunction != "":
		return fmt.Errorf("%w: VERSION predicate %q joins operands with %q; `versionPredicate` is "+
			"`LATEST_VERSION | ALL_VERSIONS | standardPredicate` and has no junction alternative, "+
			"so this emits text the parser rejects", ErrInvalidQuery, RedactPredicateValues(text), sc.topLevelJunction)
	case sc.topLevelCmps == 0:
		return fmt.Errorf("%w: VERSION predicate %q is neither LATEST_VERSION, ALL_VERSIONS, "+
			"nor a `<path> <op> <operand>` comparison; `versionPredicate` admits no node predicate, "+
			"so this emits text the parser rejects", ErrInvalidQuery, RedactPredicateValues(text))
	case sc.topLevelCmps > 1:
		return fmt.Errorf("%w: VERSION predicate %q carries %d top-level comparison operators; "+
			"`standardPredicate` is ONE `objectPath COMPARISON_OPERATOR pathPredicateOperand` and "+
			"`versionPredicate` has no junction alternative to join two, so this emits text the "+
			"parser rejects", ErrInvalidQuery, RedactPredicateValues(text), sc.topLevelCmps)
	case StripPredicateTrivia(text[:sc.cmpAt]) == "" || StripPredicateTrivia(text[sc.cmpEnd:]) == "":
		return fmt.Errorf("%w: VERSION predicate %q leaves one side of its comparison operator "+
			"empty; `standardPredicate` requires an `objectPath` before it and a "+
			"`pathPredicateOperand` after it, so this emits text the parser rejects",
			ErrInvalidQuery, RedactPredicateValues(text))
	}
	return nil
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
// FIVE states make a character not the delimiter it looks like, and the scan
// tracks each so the guard neither miscounts nor refuses legal text. The list
// is over THAT property rather than over "states in which a bracket is
// content": stated the narrower way it licensed a real defect, because a region
// can be transparent to a quote while opaque to a bracket.
//
//   - A string literal. `[` and `]` inside `'…'` / `"…"` are content. An
//     UNTERMINATED one is refused, because the emitter's own `]` would fall
//     inside it and the literal would run on into the following clause — a
//     silent substitution rather than a loud error.
//   - A contained regex. `SLASH_REGEX_CHAR : ~[/\n\r] | ESCAPE_SEQ | '\\/'`
//     admits both brackets freely, so `a/b MATCHES {/[0-9]+/}` must not be
//     counted. It is matched as a WHOLE token or not at all, and as the LONGEST
//     one — see [skipContainedRegex]. A body still OPEN at the end of the text
//     is refused for the same reason an unterminated literal is: `]` is an
//     ordinary body character, so the run swallows the emitter's own `]` and
//     closes on a later `/` … `}` in the emitted query.
//   - A comment. `COMMENT` is SKIPPED rather than channelled, so it survives
//     into the source text and a `]` inside one is not a delimiter. One that no
//     newline closes INSIDE the text is refused — see [commentRun].
//   - A `TERM_CODE` display name. `('|' ~[|[\]]+ '|')?` excludes the brackets
//     and NOTHING else, so it is transparent to quotes, braces and dashes and
//     must be stepped over whole: reading the apostrophe in
//     `at0001,SNOMED-CT::22298006|Barrett's oesophagus|` as a string delimiter
//     refused a query ParseQuery had just produced. `TERM_CODE_CHAR` admits
//     `-`, so a `--` inside a term code is likewise not a comment.
//   - A nested path predicate. `objectPath` is `pathPart (‘/’ pathPart)*` and
//     `pathPart : IDENTIFIER pathPredicate?`, so `a[at0001]/b='c'` is a legal
//     predicate carrying a BALANCED bracket pair.
//
// Balance must hold in both directions. A trailing unclosed `[` is not merely a
// loud error: the emitter's `]` closes the INNER bracket and the outer one then
// swallows text up to whatever `]` appears later in the query, which is a
// substitution again.
func ValidatePathPredicate(text string) error {
	sc := scanPredicate(text)
	switch {
	case sc.unterminated != "":
		return fmt.Errorf("%w: predicate %q leaves a %s unterminated; the emitted `]` would fall "+
			"inside it and the predicate would run on into the following clause",
			ErrInvalidQuery, RedactPredicateValues(text), sc.unterminated)
	case sc.escapes:
		return fmt.Errorf("%w: predicate %q is not bracket-balanced; splicing it would terminate "+
			"the class predicate early and re-parse as a different query",
			ErrInvalidQuery, RedactPredicateValues(text))
	}
	return nil
}

// predicateScan is what one pass over a bracket text tells the two guards.
//
// The SHAPE fields below — topLevelCmps, cmpAt, cmpEnd and topLevelJunction —
// are complete only when the scan ran to the end of the text, i.e. when
// `escapes` is false and `unterminated` is empty. [scanPredicate] returns EARLY
// on either, leaving them counted over a prefix. [ValidateVersionPredicate] is
// sound because it runs [ValidatePathPredicate] — which refuses exactly those
// two cases — before it reads any shape field; reordering those checks would
// silently make the counts wrong.
type predicateScan struct {
	// escapes reports that the text can terminate the emitter's own bracket —
	// an unbalanced `]` (closing it early) or an unclosed `[` (letting it
	// close on a later one).
	escapes bool
	// unterminated names the construct left open ("" when none), which makes
	// the emitted `]` content rather than a delimiter.
	unterminated string
	// topLevelCmps counts the COMPARISON_OPERATORs outside every literal,
	// regex, comment, term code and nested bracket, and cmpAt/cmpEnd bound the
	// FIRST of them. `standardPredicate` is ONE comparison, so the COUNT — not
	// merely the presence — is what decides the shape.
	topLevelCmps  int
	cmpAt, cmpEnd int
	// topLevelJunction names the LAST nodePredicate-only keyword found outside
	// every literal, regex, comment, term code and nested bracket ("" when
	// none) — which one is immaterial, since any at all is a refusal.
	// `versionPredicate` has no junction alternative, so one there is never
	// legal — see [ValidateVersionPredicate].
	topLevelJunction string
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
			j, open := skipContainedRegex(text, i)
			if j > 0 {
				i = j
				continue
			}
			if open {
				// No token completes INSIDE the text, but the body is still
				// open at its end — and `]` is an ordinary body character. So
				// the run continues past the emitter's own `]` and closes on
				// whatever `/` … `}` appears later in the emitted query,
				// swallowing everything between: `{/x` spliced beside a
				// contained `at0001 -- /}` re-parsed with the whole CONTAINS
				// term absorbed into the regex. Exactly the reasoning
				// [commentRun] applies to a run closed only by the query's
				// EOF, at the one other region whose end this text does not
				// fix. Unreachable from parsed input — inside a class bracket
				// `{` can only begin CONTAINED_REGEX, so a query that parsed
				// carries a COMPLETE one — hence refusing it cannot tighten.
				sc.unterminated = "contained regex"
				return sc
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
		case '=', '<', '>', '!':
			// COMPARISON_OPERATOR is `SYM_EQ | SYM_NE | SYM_GT | SYM_GE |
			// SYM_LT | SYM_LE`, i.e. `= != > >= < <=`. `!=`, `<=` and `>=` are
			// ONE operator each, so the trailing `=` MUST NOT be counted a
			// second time — counted twice, `a/b <= 1` reads as two comparisons
			// and is refused.
			//
			// `!=` must open the operator at its `!`, not at its `=`. Counting
			// the `=` alone leaves the `!` on the LEFT of the span, so `!= 1`
			// read as a comparison with `!` for an objectPath — a non-blank
			// left operand — and `VERSION v[!= 1]` was emitted for the parser
			// to reject. A lone `!` spells no token at all, so it stays
			// ordinary content and the malformed operand it makes is left
			// loud, exactly as `NOT a/b = 1` is.
			n := 1
			switch {
			case c == '!':
				if i+1 >= len(text) || text[i+1] != '=' {
					i++ // not SYM_NE; no operator starts here
					continue
				}
				n = 2
			case (c == '<' || c == '>') && i+1 < len(text) && text[i+1] == '=':
				n = 2
			}
			if depth == 0 {
				if sc.topLevelCmps == 0 {
					sc.cmpAt, sc.cmpEnd = i, i+n
				}
				sc.topLevelCmps++
			}
			i += n
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
			// A nodePredicate-only keyword at the TOP level. Matched on word
			// boundaries so a path segment merely CONTAINING the letters —
			// `a/brand > 1`, `a/android = 1`, `a/b_and_c = 1` — is not
			// mistaken for one.
			if depth == 0 {
				if kw := junctionKeywordAt(text, i); kw != "" {
					sc.topLevelJunction = kw
					i += len(kw)
					continue
				}
			}
			i++
		}
	}
	if depth != 0 {
		sc.escapes = true
	}
	return sc
}

// junctionKeywordAt returns the nodePredicate-only keyword beginning at i, or
// "". `nodePredicate` reaches AND, OR and MATCHES; `versionPredicate` reaches
// none of them.
//
// The match is case-insensitive (the lexer builds keywords from
// case-insensitive letter fragments) and bounded on BOTH sides by a non-word
// character, so `a/brand > 1` and `a/b_and_c = 1` — both legal — do not match.
func junctionKeywordAt(s string, i int) string {
	if i > 0 && wordChar(s[i-1]) {
		return ""
	}
	for _, kw := range []string{"MATCHES", "AND", "OR"} {
		if len(s)-i < len(kw) || !strings.EqualFold(s[i:i+len(kw)], kw) {
			continue
		}
		if j := i + len(kw); j < len(s) && wordChar(s[j]) {
			continue
		}
		return s[i : i+len(kw)]
	}
	return ""
}

// asciiKeyword reports whether s spells the ASCII keyword kw under the
// lexer's own case rule.
//
// The fold is held to ASCII BY LENGTH: the grammar builds its keywords from
// case-insensitive ASCII letter fragments (`S : [sS]`), while
// [strings.EqualFold] folds the full Unicode simple-fold set — so
// `LATEſT_VERSION` (U+017F LATIN SMALL LETTER LONG S, which folds to `s`) was
// EqualFold-equal to the keyword while the lexer fails it with a
// token-recognition error, and the guard emitted text the parser rejects.
// Every rune outside ASCII is more than one byte, so byte-length equality
// with the ASCII keyword forces every folded pair ASCII. [junctionKeywordAt]
// needs no gate for the same reason in reverse: it folds a slice of exactly
// len(kw) BYTES, in which a multi-byte rune leaves too few runes to match.
func asciiKeyword(s, kw string) bool {
	return len(s) == len(kw) && strings.EqualFold(s, kw)
}

// termCodeChar spells `TERM_CODE_CHAR : NAME_CHAR | '.'`, i.e. a word
// character, '-' or '.'.
func termCodeChar(c byte) bool {
	return c == '_' || c == '-' || c == '.' ||
		(c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// skipTermCodeName steps over `TERM_CODE`'s optional trailing display name,
// `'|' ~[|[\]]+ '|'`, and reports whether one is there.
//
// The content class excludes both brackets, so a `]` before the closing `|`
// means this is NOT that section and the `]` is a real delimiter — verified:
// `c[at0001,X::1|a] CONTAINS …` is a loud token-recognition error, so falling
// through to ordinary scanning is both safe and correct.
//
// On failure the int is where the walk STOPPED — the `[` or `]` that proved the
// section absent, the empty interior's own `|`, or len(s). The scan ignores it
// (the bytes are rescanned as ordinary content), but [RedactPredicateValues]
// must not: the bytes walked are the display name the caller MEANT, i.e. value
// content, whether or not a closing `|` ever made them a token.
func skipTermCodeName(s string, i int) (int, bool) {
	for j := i + 1; j < len(s); j++ {
		switch s[j] {
		case '|':
			if j == i+1 {
				return j, false // `~[|[\]]+` needs at least one character
			}
			return j + 1, true
		case '[', ']':
			return j, false
		}
	}
	return len(s), false
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
// (';' WS* STRING)? WS* '}'` as a WHOLE token. It returns the end of the
// LONGEST token starting at i (0 when none completes), and whether the body is
// still OPEN at the end of the text.
//
// Whole-token or nothing is the point. ANTLR takes the longest match, so a `{`
// that cannot complete the production is simply `SYM_LEFT_CURLY` and every
// character after it — a `]` included — is lexed on its own terms. Reporting
// such a `{` as an unterminated region refused text that escapes nothing:
// `a/b MATCHES {x}` is a CONTAINED malformation, which § The class predicate
// positions requires be left to the parser.
//
// The longest match is over the whole TOKEN, not over the body. Returning the
// first tail that closes — with the body candidates walked longest-first —
// looks equivalent and is not, because the `(';' WS* STRING)?` tail lets a
// SHORTER body reach FURTHER: in `{/a\/;'x/}'}` the body closing at the second
// `/` completes with a bare `}` after 10 bytes, while the body closing at the
// escaped `/` runs its `;'x/}'` tail to 12. The lexer takes the 12; a scan that
// took the 10 met the leftover `'` and refused a regex ParseQuery accepts.
//
// The `}` cannot simply be searched for: `SLASH_REGEX_CHAR : ~[/\n\r] |
// ESCAPE_SEQ | '\\/'` admits `}` inside the pattern (`{/a{2}/}`), so the body
// is stepped over on its own terms.
func skipContainedRegex(s string, i int) (end int, open bool) {
	j := skipRegexSpace(s, i+1)
	if j >= len(s) || s[j] != '/' {
		return 0, false // no `/`, so the `{` opens no regex at all
	}
	ends, open := regexBody(s, j+1)
	for _, e := range ends {
		if k, ok := regexTail(s, e+1); ok && k > end {
			end = k
		}
	}
	return end, open
}

// regexBody returns every index at which `SLASH_REGEX`'s body can close, and
// whether the body is still OPEN at the end of the text — that is, whether the
// character AFTER the supplied text would still be inside the body.
//
// A backslash is read BOTH ways, and committing to one of them refused a regex
// the parser accepts. `~[/\n\r]` admits a backslash as an ORDINARY character
// while `ESCAPE_SEQ` and `'\\/'` consume the character after it, so in
// `{/]\/}` the escaped reading cannot complete the token and the ordinary one
// closes the body at the very next `/` — and ANTLR takes whichever reading
// yields the longest token. Always consuming the character after a backslash
// made that regex unwritable.
//
// The reachable boundary set is walked rather than every `/` being treated as
// a candidate, because a bare `/` is NOT a body character: `~[/\n\r]` excludes
// it and only `'\\/'` puts one inside. Treating them all as candidates would
// read `{/a/b/}` — which the lexer resolves to SYM_LEFT_CURLY — as a regex and
// step over a `]` that is a real delimiter, which is the silent direction.
//
// The reachable set is an INTERVAL `[start, m]`, which is what keeps the scan
// linear. An ordinary character advances it by one and an escape by two, and an
// escape's own first byte is ordinary, so no position is ever skipped over.
// Tracking `m` as an index rather than as a `[]bool` per candidate `{` matters:
// the bitmap cost `len(text)` bytes and a full-length walk for EVERY regex in
// the predicate, which is quadratic in time and allocation over text carrying
// many of them (a 1 MB predicate of `{/\/` ran 90 s and churned 246 GiB).
func regexBody(s string, start int) (ends []int, open bool) {
	m := start
	for p := start; p <= m && p < len(s); p++ {
		switch c := s[p]; c {
		case '\n', '\r':
			// `~[/\n\r]` excludes the line breaks and no escape spells one, so
			// the body cannot continue through here.
		case '/':
			if p > start { // SLASH_REGEX_CHAR+ needs at least one character
				ends = append(ends, p)
			}
		default:
			m = p + 1 // `~[/\n\r]`
			if c == '\\' && p+1 < len(s) && regexEscapeChar(s[p+1]) {
				m = p + 2 // ESCAPE_SEQ | '\\/'
			}
		}
	}
	return ends, m >= len(s)
}

// regexEscapeChar spells the character a backslash may consume inside a regex
// body: `ESCAPE_SEQ : '\\' ['"?abfnrtv\\*]`, plus `SLASH_REGEX_CHAR`'s own
// `'\\/'` alternative.
func regexEscapeChar(c byte) bool {
	return strings.IndexByte(`'"?abfnrtv\*/`, c) >= 0
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
//   - The BODY is `~[\r\n]*`, which excludes a lone carriage return as much as
//     a newline. Only `'\r'? '\n'` closes the token, so `-- x\r]` is
//     SYM_DOUBLE_DASH and the `]` after it is a delimiter — searching the body
//     for the next `\n` alone read one comment where the lexer sees none, in
//     both directions at once: it stepped over a real `]`, and it reported a
//     text ENDING at the `\r` as an unterminated comment when the parser
//     merely rejects it loudly.
//   - EOF is the end of the QUERY, not the end of this text — but only the BODY
//     alternative can reach it. A `'-- ' body` run that no newline closes here
//     goes on to close somewhere in the emitted query, absorbing the emitter's
//     own `]` on the way, which is why the caller refuses it. The BARE
//     alternative cannot: its terminator has to come IMMEDIATELY, and the byte
//     after the predicate is always `]` — never EOF and never a line break — so
//     a text ENDING at `--` starts no COMMENT at all. Reading the two
//     alternatives alike refused `a/b='c'--`, whose emission is a contained
//     LOUD malformation (verified: `no viable alternative`, under every suffix
//     including one carrying a later newline), and § The class predicate
//     positions forbids refusing those.
func commentRun(s string, i int) (int, bool) {
	if i+1 >= len(s) || s[i] != '-' || s[i+1] != '-' {
		return 0, false
	}
	rest := s[i+2:]
	switch {
	case rest == "":
		// `'--' ('\r'? '\n' | EOF)` with `]` coming next: SYM_DOUBLE_DASH, and
		// the `]` after it is a real delimiter.
		return 0, false
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
	body := rest[1:] // `~[\r\n]*`, then `'\r'? '\n'`
	switch n := strings.IndexAny(body, "\r\n"); {
	case n < 0:
		return len(s) - i, false // no terminator here: the run closes in the emitted query
	case body[n] == '\n':
		return len("-- ") + n + 1, true
	case n+1 < len(body) && body[n+1] == '\n':
		return len("-- ") + n + 2, true
	default:
		return 0, false // the body ended on a lone '\r', which closes nothing
	}
}

// RedactPredicateValues renders predicate bracket text for a DIAGNOSTIC, with
// the content of every opaque region replaced by an ellipsis and all of the
// structure kept:
//
//	ehr_id/value='9d3d…6666' AND x/y='   ->   ehr_id/value='…' AND x/y='…
//
// A refused predicate has to be named or the caller cannot tell WHICH one a
// builder assembling several was refused. But the class standing predicate is
// where openEHR carries the identifiable root — `[ehr_id/value='…']`,
// `[uid/value='…']`, `[subject/external_ref/id/value='…']` — and these errors
// are the return value of `Build` and `(*parse.Query).Emit`, i.e. the thing a
// consuming CDR logs and ships to an error tracker. Reproducing the body
// verbatim moved a patient identifier out of the request and into the log
// stream, where retention and access are a different question. Structure alone
// diagnoses the defect: every rule these guards enforce is about delimiters.
//
// The elided regions are exactly the states [ValidatePathPredicate]'s scan
// already tracks as CONTENT rather than as delimiters — a string literal, a
// contained regex, a comment body and a `TERM_CODE` display name — minus the
// nested bracket, which IS structure. Deriving the list that way rather than
// writing a second one is deliberate: a region the scan learns to step over is
// a region a caller's data can sit in, so the two lists cannot drift apart.
// Paths, operators, node codes, term codes and numeric literals are structure
// and ride through unchanged.
//
// An UNTERMINATED region — the commonest refusal — elides to the end of the
// text and keeps its opening delimiter, so the diagnostic still shows what was
// left open. A display name is elided whether or not its closing `|` ever
// arrived: a `]` inside `|…` means the section never became a token and the
// scan treats the bytes as ordinary — but they are still the display name the
// caller MEANT, and rendering them verbatim is the leak this function exists
// to prevent.
func RedactPredicateValues(text string) string {
	var b strings.Builder
	for i := 0; i < len(text); {
		switch c := text[i]; c {
		case '\'', '"':
			if j, ok := skipPredicateString(text, i); ok {
				elideRegion(&b, text[i:j], 1, 1) // '…'
				i = j
				continue
			}
			elideRegion(&b, text[i:], 1, 0) // '… — unterminated, runs to the end
			return b.String()
		case '{':
			// The braces are kept and the interior dropped whole rather than
			// spelled `{/…/}`: a complete token may carry WS and a `;'flags'`
			// tail, and a diagnostic that invents a shape the text does not have
			// is worse than one that says only "a regex region was here".
			if j, open := skipContainedRegex(text, i); j > 0 {
				elideRegion(&b, text[i:j], 1, 1) // {…}
				i = j
				continue
			} else if open {
				elideRegion(&b, text[i:], 1, 0) // {… — never closed here
				return b.String()
			}
			b.WriteByte(c) // SYM_LEFT_CURLY: ordinary content, not a region
			i++
		case '-':
			n, closed := commentRun(text, i)
			if n == 0 {
				b.WriteByte(c)
				i++
				continue
			}
			// `--` is kept, and so is the terminator, because it is what closes
			// the token: eliding it would render a closed comment as an open one.
			lead := 2
			if len(text) > i+2 && text[i+2] == ' ' {
				lead = 3
			}
			tail := 0
			if closed {
				tail = 1
				if n >= 2 && text[i+n-2] == '\r' {
					tail = 2
				}
			}
			elideRegion(&b, text[i:i+n], lead, tail)
			i += n
		case ':':
			// A term CODE is structure; only its display name is free text.
			if i+1 < len(text) && text[i+1] == ':' {
				j := i + 2
				for j < len(text) && termCodeChar(text[j]) {
					j++
				}
				b.WriteString(text[i:j])
				if j < len(text) && text[j] == '|' {
					// Elided on FAILURE too. A `]` (or the end of the text)
					// before the closing `|` means no display name ever became
					// a token — but the bytes walked are still the display name
					// the caller MEANT, and a refused `at0001,X::1|Ann Example]`
					// echoed them verbatim. The failure byte itself is a real
					// delimiter and stays; the scan rescans from it.
					k, ok := skipTermCodeName(text, j)
					if ok {
						elideRegion(&b, text[j:k], 1, 1) // |…|
					} else {
						elideRegion(&b, text[j:k], 1, 0) // |… — never closed
					}
					j = k
				}
				i = j
				continue
			}
			b.WriteByte(c)
			i++
		default:
			b.WriteByte(c)
			i++
		}
	}
	return b.String()
}

// elideRegion writes region with its interior replaced by one ellipsis, keeping
// lead bytes at the front and tail bytes at the back. An already-empty interior
// is left alone, so `”` does not become `'…'`.
func elideRegion(b *strings.Builder, region string, lead, tail int) {
	if len(region) <= lead+tail {
		b.WriteString(region)
		return
	}
	b.WriteString(region[:lead])
	b.WriteString("…")
	b.WriteString(region[len(region)-tail:])
}

// StripPredicateTrivia normalises what the lexer SKIPS out of predicate
// bracket text — `WS`, `COMMENT` and `UNICODE_BOM` — so that a comparison
// against the text sees what the PARSER sees. `LATEST_VERSION -- note\n` is
// the same token stream as `LATEST_VERSION`, and trimming whitespace alone
// refused it.
//
// Each interior trivia RUN — whitespace, comments and BOMs together — becomes
// ONE space, because skipped trivia still separates the tokens around it;
// leading and trailing runs are dropped. So `a/b =\n\t 1` and `a/b = 1` are
// the same text, and what this function returns never carries a raw line
// break, an AQL comment or a BOM that the lexer would have skipped — the
// canonical path suffix `openehr/aql/lint` renders rests on exactly that.
//
// The walk is lexer-state-AWARE, over the same regions [ValidatePathPredicate]'s
// scan steps over: a string literal, a contained regex token, a term code and
// its display name ride through VERBATIM, because inside those regions the
// lexer skips nothing — a `-- note\n` inside `'…'` is string CONTENT, and
// rewriting it would alter the value the predicate carries, not its trivia. A
// region left open at the end of the text (an unterminated literal, an open
// regex body) is content to the end for the same reason.
//
// It is exported because the read side now reports predicate text VERBATIM
// (REQ-119 § The emission closure property), so every consumer that COMPARES
// that text against something — rather than re-emitting it — needs the same
// trivia model. `openehr/aql/lint`'s template resolution is the case in force:
// it matches a segment predicate against a compiled OPT node id, and a
// `[ at0001 ]` that once arrived whitespace-collapsed now does not. Modelling
// trivia a second time in the consumer is what this export exists to prevent.
func StripPredicateTrivia(text string) string {
	var b strings.Builder
	pending := false // an interior trivia run collapses to one space
	content := func(s string) {
		if pending && b.Len() > 0 {
			b.WriteByte(' ')
		}
		pending = false
		b.WriteString(s)
	}
	for i := 0; i < len(text); {
		switch c := text[i]; c {
		case ' ', '\t', '\r', '\n':
			// The lexer's `WS : [ \t\r\n]+`, one byte at a time — adjacent
			// comments and BOMs fold into the same pending run.
			pending = true
			i++
		case '\'', '"':
			j, ok := skipPredicateString(text, i)
			if !ok {
				content(text[i:]) // unterminated: content to the end
				return b.String()
			}
			content(text[i:j])
			i = j
		case '{':
			// A complete CONTAINED_REGEX is ONE token — the `WS*` inside it
			// belongs to the token, not to the skipped channel, so the whole
			// span rides through untouched. See [skipContainedRegex] for why a
			// `{` completing no token is ordinary content.
			j, open := skipContainedRegex(text, i)
			switch {
			case j > 0:
				content(text[i:j])
				i = j
			case open:
				content(text[i:]) // open body: content to the end
				return b.String()
			default:
				content("{")
				i++
			}
		case ':':
			// A term code and its display name, stepped over whole exactly as
			// [scanPredicate] steps: `TERM_CODE_CHAR` admits `-`, so a `--`
			// inside the code is not a comment, and the display-name class
			// `~[|[\]]+` admits spaces, dashes and line breaks as CONTENT.
			if i+1 < len(text) && text[i+1] == ':' {
				j := i + 2
				for j < len(text) && termCodeChar(text[j]) {
					j++
				}
				if j < len(text) && text[j] == '|' {
					if k, ok := skipTermCodeName(text, j); ok {
						j = k
					}
				}
				content(text[i:j])
				i = j
				continue
			}
			content(":")
			i++
		case '-':
			// A COMMENT run is trivia whether or not a newline closed it here:
			// the lexer skips it either way, the closed one at its newline and
			// the open one at whatever ends the surrounding query. `--x` and a
			// `\r`-ended body are SYM_DOUBLE_DASH — content, not a comment.
			if n, _ := commentRun(text, i); n > 0 {
				pending = true
				i += n
				continue
			}
			content("-")
			i++
		default:
			if n := bomRun(text, i); n > 0 {
				pending = true
				i += n
				continue
			}
			content(text[i : i+1])
			i++
		}
	}
	return b.String()
}

// bomRun returns the length of the `UNICODE_BOM` run starting at i, or 0.
//
// `UNICODE_BOM` is skipped like `WS` and `COMMENT` and is NOT anchored to the
// start of input, so `VERSION v[<BOM>LATEST_VERSION]` is a query ParseQuery
// produces and whose predicate this package must therefore accept.
//
// Two of the vendored rule's three alternatives are upstream spelling quirks
// rather than BOMs — ANTLR's `\u` escape takes exactly four hex digits, so
// `'\uEFBBBF'` is U+EFBB followed by the letters `BF`, and `'\u0000FEFF'`
// is NUL followed by `FEFF`. They are matched as WRITTEN, because the lexer
// skips what the rule says and not what its comment labels it.
func bomRun(s string, i int) int {
	for _, form := range []string{"\uFEFF", "\uEFBB" + "BF", "\x00" + "FEFF"} {
		if strings.HasPrefix(s[i:], form) {
			return len(form)
		}
	}
	return 0
}
