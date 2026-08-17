package aql

// identifier.go — REQ-119. The single-token positions the emitters write
// VERBATIM, and the guards that keep splice text out of them.
//
// A path is emitted verbatim by contract (REQ-055 rule 3) and has no writable
// guard: a bracketed name predicate lawfully carries AND / OR / quotes, so no
// alphabet separates a path from an injection. These positions are different.
// `selectExpr : columnExpr (AS aliasName=IDENTIFIER)?` and `classExprOperand :
// IDENTIFIER variable=IDENTIFIER? …` admit exactly ONE token each, so the exact
// predicate — "this string lexes as a single IDENTIFIER" — IS writable, and the
// splice they otherwise admit re-parses as a different query with err == nil.
//
// The rules here are hand-derived because `openehr/aql` may not import the
// generated lexer (REQ-013 — the dependency runs the other way). They are held
// honest MECHANICALLY rather than by review: TestIdentifierGuardTracksTheGrammar
// in openehr/aql/parse walks every token name out of AqlLexer.g4, plus the
// spellings carried only in fragments, and asserts the guard accepts a string
// exactly when the parser reads it back unchanged.

import (
	"errors"
	"fmt"
	"strings"
)

// funcIDWords are the grammar's function-name tokens: STRING_FUNCTION_ID,
// NUMERIC_FUNCTION_ID, DATE_TIME_FUNCTION_ID and TERMINOLOGY (AqlLexer.g4
// §§ Functions), each declared BEFORE `IDENTIFIER` at :168.
//
// They are deliberately absent from [reservedNonFuncWords], because a FUNCTION
// name may be spelled with them — `functionCall` lists those very tokens as
// alternatives. An IDENTIFIER position may not: `LENGTH` there lexes as
// STRING_FUNCTION_ID and the text does not parse. The two sets are therefore
// kept apart and unioned at the point of use, so neither restates the other.
//
// `CONTAINS_STR` has no token name of its own — it is an alternative inside
// STRING_FUNCTION_ID — which is exactly the fragment-only case that made the
// first version of the reserved list wrong. The grammar confrontation walks
// those spellings too.
var funcIDWords = map[string]bool{
	"LENGTH": true, "CONTAINS_STR": true, "POSITION": true, "SUBSTRING": true,
	"CONCAT": true, "CONCAT_WS": true,
	"ABS": true, "MOD": true, "CEIL": true, "FLOOR": true, "ROUND": true,
	"NOW": true, "CURRENT_DATE": true, "CURRENT_TIME": true,
	"CURRENT_DATE_TIME": true, "CURRENT_TIMEZONE": true,
	"TERMINOLOGY": true,
}

// ValidateIdentifier refuses a string that does not lex as exactly one
// `IDENTIFIER` token, returning an error wrapping [ErrInvalidQuery].
//
// It guards the positions the emitters splice verbatim: a SELECT `AS` alias, a
// class alias, and an RM type. Left unguarded, `Alias: "x, c/y"` emitted
// `SELECT c/x AS x, c/y FROM …` with err == nil, which re-parses as TWO
// projections — REQ-119's silent-substitution class.
//
// Three things an alphabet check alone gets wrong, all of them observed against
// the real lexer rather than reasoned about:
//
//   - A reserved word passes the alphabet and then lexes as its own token.
//     Shadowing turns on DECLARATION ORDER, so the set is every keyword declared
//     before IDENTIFIER — which includes the function-name tokens ([funcIDWords])
//     that a function name MAY be spelled with and an identifier may not.
//   - `true` / `false` are NOT reserved here. BOOLEAN is declared at :232, after
//     IDENTIFIER, so they lex as identifiers and refusing them would reject an
//     alias `ParseQuery` itself produces.
//   - `at0001` and `id123` lex as AT_CODE / ID_CODE (:124-125), so they are
//     refused — but the prefixes there are LOWERCASE literals, unlike the
//     case-insensitive keyword fragments, so `AT0001` is a perfectly good alias.
func ValidateIdentifier(name string) error {
	if name == "" {
		return fmt.Errorf("%w: empty identifier", ErrInvalidQuery)
	}
	// IDENTIFIER : ALPHA_CHAR WORD_CHAR*. Run over the ORIGINAL bytes: Go's
	// Unicode case mapping folds some non-ASCII letters INTO ASCII (ı → I),
	// so checking a case-mapped copy would admit a spelling the lexer cannot
	// tokenise. Whitespace needs no separate branch — a leading space is not a
	// letter, and an inner one is not a word character.
	if !asciiLetter(name[0]) {
		return fmt.Errorf("%w: identifier %q does not start with a letter", ErrInvalidQuery, name)
	}
	for i := range len(name) {
		if c := name[i]; !wordChar(c) {
			return fmt.Errorf("%w: identifier %q carries %q, which no AQL identifier admits",
				ErrInvalidQuery, name, string([]byte{c}))
		}
	}
	if n := asciiUpper(name); reservedNonFuncWords[n] || funcIDWords[n] {
		return fmt.Errorf("%w: %q is a reserved AQL keyword and cannot be used as an identifier",
			ErrInvalidQuery, n)
	}
	if isCodeToken(name) {
		return fmt.Errorf("%w: identifier %q lexes as an archetype node code (%s…), not an IDENTIFIER",
			ErrInvalidQuery, name, name[:2])
	}
	return nil
}

// validateRMTypeToken applies [ValidateIdentifier] to a class's RM-TYPE
// position, admitting the one spelling that is not an identifier there:
// `VERSION`.
//
// The guard itself is right to refuse the word everywhere — `classExprOperand`
// has `VERSION variable=IDENTIFIER? …` as its own ALTERNATIVE beside the
// `IDENTIFIER …` form, so no identifier position admits it, and an alias
// spelled `VERSION` stays refused. But the RM-type position is where the write
// paths SPELL that alternative, and the two carriers differ:
//
//   - [parse.ClassExpr] has a Version FLAG. There the flag is authoritative and
//     the string must not be a second, contradictory carrier, so an unflagged
//     `RMType: "VERSION"` is refused: it emits text that re-parses WITH the flag
//     set, an AST the caller did not write.
//   - The builder has no such field. Here the spelling IS the carrier, so there
//     is no contradiction to refuse — and refusing it left the builder unable to
//     express `FROM VERSION v` or `CONTAINS VERSION v` at all, text both
//     ParseQuery and Emit round-trip. That is a Build/Emit parity break in the
//     TIGHTENING direction, which this REQ guards against as squarely as the
//     splice it was closing.
//
// A VERSION class takes no archetype predicate; that rule binds its callers,
// which carry the archetype field this one does not see.
func validateRMTypeToken(rmType string) error {
	// ASCII-gated fold: a Unicode fold-equal spelling (`VERſION`) is NOT the
	// keyword — the lexer's fragments are ASCII — and falling through to
	// [ValidateIdentifier] refuses it, where the bare fold accepted a token
	// the parser cannot read. See [asciiKeyword].
	if asciiKeyword(rmType, "VERSION") {
		return nil
	}
	return ValidateIdentifier(rmType)
}

// isCodeToken reports whether s lexes as `ID_CODE : 'id' CODE_STR` or
// `AT_CODE : 'at' CODE_STR`, both declared before IDENTIFIER.
//
// The `at` / `id` prefixes are literal lowercase in the grammar, NOT the
// case-insensitive letter fragments the keywords use, so the test is
// case-sensitive. Inside an identifier-shaped string CODE_STR reduces to a
// non-empty digit run, since the '.' groups it also admits cannot occur here.
func isCodeToken(s string) bool {
	if len(s) < 3 || (s[:2] != "at" && s[:2] != "id") {
		return false
	}
	for i := 2; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// ValidateArchetypeID refuses a string that does not lex as exactly one
// `ARCHETYPE_HRID` token, returning an error wrapping [ErrInvalidQuery].
//
// The containment predicate is the fourth verbatim position and the same defect
// class: `Archetype: "openEHR-EHR-COMPOSITION.x.v1] CONTAINS OBSERVATION[…"`
// emitted a whole extra CONTAINS term that re-parses cleanly. `archetypePredicate
// : ARCHETYPE_HRID | PARAMETER` is one token per alternative, so the exact guard
// is writable here exactly as it is for an identifier — use [ValidateValue] on a
// [ParamValue] for the `$param` alternative.
//
// The shape is
//
//	(NAMESPACE '::')? ID '-' ID '-' ID '.' CONCEPT '.v' VERSION_ID
//
// Keyword shadowing does NOT apply inside it: the pieces are lexer FRAGMENTS,
// not the IDENTIFIER token, so the whole string lexes as one ARCHETYPE_HRID and
// a segment spelled like a keyword is unremarkable.
func ValidateArchetypeID(id string) error {
	if id == "" {
		return fmt.Errorf("%w: empty archetype id", ErrInvalidQuery)
	}
	bad := func(why string) error {
		return fmt.Errorf("%w: archetype id %q %s; the grammar admits "+
			"(<namespace>::)?<rm>-<rm>-<class>.<concept>.v<version>", ErrInvalidQuery, id, why)
	}
	rest := id
	if ns, tail, ok := strings.Cut(id, "::"); ok {
		if err := checkHRIDNamespace(ns); err != nil {
			return bad(err.Error())
		}
		rest = tail
	}
	// CONCEPT admits no '.', and the namespace (which does) has been cut off,
	// so the first two dots are the structural ones and everything after the
	// second belongs to the version.
	parts := strings.SplitN(rest, ".", 3)
	if len(parts) != 3 {
		return bad("is not <root>.<concept>.v<version>")
	}
	root := strings.Split(parts[0], "-")
	if len(root) != 3 {
		return bad("does not carry exactly three '-'-separated root segments")
	}
	for _, seg := range root {
		if !identifierShaped(seg) {
			return bad(fmt.Sprintf("has a root segment %q that is not ALPHA_CHAR WORD_CHAR*", seg))
		}
	}
	// ARCHETYPE_CONCEPT_ID : ALPHA_CHAR NAME_CHAR*, and NAME_CHAR adds '-'.
	if !nameShaped(parts[1]) {
		return bad(fmt.Sprintf("has a concept id %q that is not ALPHA_CHAR NAME_CHAR*", parts[1]))
	}
	version, ok := strings.CutPrefix(parts[2], "v")
	if !ok {
		return bad("has no '.v' before its version")
	}
	if err := checkVersionID(version); err != nil {
		return bad(err.Error())
	}
	return nil
}

// checkHRIDNamespace spells `NAMESPACE : LABEL ('.' LABEL)*` with
// `LABEL : ALPHA_CHAR (NAME_CHAR | URI_PCT_ENCODED)*`.
func checkHRIDNamespace(ns string) error {
	if ns == "" {
		return errors.New("has an empty namespace")
	}
	for label := range strings.SplitSeq(ns, ".") {
		if label == "" || !asciiLetter(label[0]) {
			return fmt.Errorf("has a namespace label %q that does not start with a letter", label)
		}
		for i := 1; i < len(label); i++ {
			c := label[i]
			if c == '%' {
				if i+2 >= len(label) || !hexDigit(label[i+1]) || !hexDigit(label[i+2]) {
					return fmt.Errorf("has a namespace escape in %q that is not %%XX", label)
				}
				i += 2
				continue
			}
			if !nameChar(c) {
				return fmt.Errorf("has a namespace label %q carrying %q", label, string([]byte{c}))
			}
		}
	}
	return nil
}

// checkVersionID spells `VERSION_ID : DIGIT+ ('.' DIGIT+)*
// (('-rc' | '-alpha') ('.' DIGIT+)?)?`.
func checkVersionID(v string) error {
	for _, suffix := range []string{"-rc", "-alpha"} {
		head, tail, ok := strings.Cut(v, suffix)
		if !ok {
			continue
		}
		if tail != "" {
			rev, found := strings.CutPrefix(tail, ".")
			if !found || !digitRun(rev) {
				return fmt.Errorf("has a %s revision that is not '.'-separated digits", suffix)
			}
		}
		v = head
		break
	}
	if v == "" {
		return errors.New("has an empty version")
	}
	for part := range strings.SplitSeq(v, ".") {
		if !digitRun(part) {
			return fmt.Errorf("has a version segment %q that is not digits", part)
		}
	}
	return nil
}

// identifierShaped spells the IDENTIFIER_CHAR fragment; nameShaped adds '-'
// for ARCHETYPE_CONCEPT_ID. Neither consults the keyword set: inside an
// ARCHETYPE_HRID these are fragments, so nothing shadows them.
func identifierShaped(s string) bool {
	if s == "" || !asciiLetter(s[0]) {
		return false
	}
	for i := range len(s) {
		if !wordChar(s[i]) {
			return false
		}
	}
	return true
}

func nameShaped(s string) bool {
	if s == "" || !asciiLetter(s[0]) {
		return false
	}
	for i := range len(s) {
		if !nameChar(s[i]) {
			return false
		}
	}
	return true
}

// nameChar spells `NAME_CHAR : WORD_CHAR | '-'`.
func nameChar(c byte) bool { return wordChar(c) || c == '-' }

func digitRun(s string) bool {
	if s == "" {
		return false
	}
	for i := range len(s) {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}
