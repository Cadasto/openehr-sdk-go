package parse_test

// REQ-119 · PROBE-090
//
// literal_roundtrip_test.go pins the value-position literal contracts of the
// REQ-117 fixed-point property that the corpus in roundtrip_test.go could not
// reach: the corpus is written in canonical form, so it never contained a
// string carrying a quote or a backslash, nor a real whose decimal rendering
// carries no fractional part. Both spellings emitted text this SDK's own
// parser rejects — a class of defect the round-trip corpus structurally cannot
// surface unless a case actually exercises the escape.

import (
	"errors"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/cadasto/openehr-sdk-go/openehr/aql"
	"github.com/cadasto/openehr-sdk-go/openehr/aql/parse"
)

// whereRoundTrip emits `WHERE c/n = <v>`, re-parses it, and returns the value
// the parser recovered. It fails the test if the emitted AQL does not parse —
// which is the whole point: an emitter that produces unparseable text is the
// defect under guard here.
func whereRoundTrip(t *testing.T, v aql.Value) (string, aql.Value) {
	t.Helper()
	pred, err := aql.FormatWhere(aql.Compare(aql.Path("c/n"), aql.OpEq, v))
	if err != nil {
		t.Fatalf("FormatWhere(%#v): %v", v, err)
	}
	q := "SELECT c/uid/value FROM COMPOSITION c WHERE " + pred
	doc, err := parse.ParseQuery(q)
	if err != nil {
		t.Fatalf("emitted AQL does not re-parse\n  emitted: %s\n  err: %v", q, err)
	}
	cmp, ok := doc.Where.(aql.Comparison)
	if !ok {
		t.Fatalf("WHERE is %T, want aql.Comparison", doc.Where)
	}
	return pred, cmp.Val
}

// TestStringLiteralEscapesRoundTrip — REQ-117. A string literal MUST emit as
// the grammar's STRING token and MUST decode back to the same Go string.
//
// The SQL-style `”` doubling the emitter used to produce is not an AQL
// escape: the lexer ends the token at the second quote, so `'O”Brien'` lexed
// as two adjacent STRING tokens and the query failed to parse. Any clinical
// literal carrying an apostrophe was affected.
func TestStringLiteralEscapesRoundTrip(t *testing.T) {
	for name, s := range map[string]string{
		"plain":            "Temperature",
		"apostrophe":       "O'Brien",
		"backslash":        `C:\temp`,
		"both":             `O'Brien\n`,
		"double quote":     `say "hi"`,
		"newline":          "line1\nline2",
		"tab":              "a\tb",
		"non-ascii":        "Grüße — 日本語",
		"leading quote":    "'quoted'",
		"only backslashes": `\\\`,
		"empty":            "",
	} {
		t.Run(name, func(t *testing.T) {
			pred, got := whereRoundTrip(t, aql.String(s))
			sv, ok := got.(aql.StringValue)
			if !ok {
				t.Fatalf("recovered %T, want aql.StringValue (emitted %s)", got, pred)
			}
			if sv.S != s {
				t.Errorf("round trip lost the value\n  in:  %q\n  wire: %s\n  out: %q", s, pred, sv.S)
			}
		})
	}
}

// TestStringLiteralEmitsGrammarEscapes pins the exact wire spelling, so a
// future "simplification" back to SQL doubling fails loudly rather than only
// showing up as a parse error somewhere downstream.
func TestStringLiteralEmitsGrammarEscapes(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"O'Brien", `'O\'Brien'`},
		{`C:\temp`, `'C:\\temp'`},
		{"plain", `'plain'`},
		// `?` and `*` have IDENTITY escapes (`\?`, `\*`) the token admits, so
		// escaping them would still round-trip — this exact-wire row is what
		// pins that they ride RAW, per the canonical form.
		{"a?b*c", `'a?b*c'`},
		{`say "hi"`, `'say "hi"'`},
		// All SEVEN C0 controls with an ESCAPE_SEQ spelling, not just \n. Each
		// round-trips fine when emitted raw, so the round-trip tests assert a
		// strictly weaker property than the guard enforces — dropping any one
		// escape left the whole suite green while emitted AQL stopped being
		// single-line and printable (value.go's stated purpose for the rule).
		{"a\ab", `'a\ab'`},
		{"a\bb", `'a\bb'`},
		{"a\fb", `'a\fb'`},
		{"a\nb", `'a\nb'`},
		{"a\rb", `'a\rb'`},
		{"a\tb", `'a\tb'`},
		{"a\vb", `'a\vb'`},
		// A byte beginning no valid UTF-8 sequence has only the OCTAL_ESC
		// spelling; a genuine U+FFFD rides through verbatim. utf8.RuneError IS
		// U+FFFD, so only the exact wire form separates the two cases — the
		// round-trip assertion cannot, since the octal form decodes back to the
		// same rune.
		{"a\xffb", `'a\377b'`},
		{"a\uFFFDb", "'a\uFFFDb'"},
	} {
		if got := aql.FormatValue(aql.String(tc.in)); got != tc.want {
			t.Errorf("FormatValue(%q) = %s, want %s", tc.in, got, tc.want)
		}
	}
}

// TestParsedStringEscapesDecode covers the read direction independently: AQL
// written by someone else, carrying each escape the lexer admits, MUST decode
// to the runes it denotes rather than surviving as raw backslash sequences.
func TestParsedStringEscapesDecode(t *testing.T) {
	for name, tc := range map[string]struct{ aqlText, want string }{
		"escaped quote":  {`'O\'Brien'`, "O'Brien"},
		"escaped bslash": {`'C:\\temp'`, `C:\temp`},
		"c escapes":      {`'a\nb\tc'`, "a\nb\tc"},
		"identity \\?":   {`'who\?'`, "who?"},
		"unicode":        {`'\u00e9t\u00e9'`, "été"},
		"octal":          {`'\101\102'`, "AB"},
		"no escapes":     {`'plain'`, "plain"},
		// UTF8CHAR is exactly four hex digits, so a non-BMP character has no
		// other spelling than a UTF-16 surrogate PAIR — which is how any
		// JSON/JavaScript-derived client writes an emoji or a CJK extension.
		// Decoding each half on its own yielded two U+FFFD, silently.
		// NB: the aqlText below is the AQL SOURCE, so the backslashes are
		// doubled in the Go literal — `\\ud83d` is the six characters the
		// lexer sees, not one rune.
		"surrogate pair":       {"'\\ud83d\\ude00'", "\U0001F600"},
		"surrogate pair twice": {"'\\ud83d\\ude00\\ud83d\\ude01'", "\U0001F600\U0001F601"},
		"pair among BMP":       {"'a\\ud83d\\ude00b'", "a\U0001F600b"},
		// An unpaired half denotes no character; U+FFFD is the lenient reading.
		"lone high surrogate": {"'\\ud83d'", "�"},
		"lone low surrogate":  {"'\\ude00'", "�"},
		"high then BMP":       {"'\\ud83d\\u0041'", "�A"},
		// TWO low halves: the leading one is not a high surrogate, so no pair
		// forms and both decode to U+FFFD. Without this the `hi > 0xDBFF` bound
		// could be dropped and the pair combined from the wrong half.
		"low then low": {"'\\ude00\\ude01'", "��"},
		// `\4`–`\7` lead at most TWO octal digits (OCTAL_ESC : '\\' [0-3] OCTAL
		// OCTAL | '\\' OCTAL OCTAL | '\\' OCTAL), so a third digit is literal.
		"octal 2-digit lead": {`'\477'`, "'7"},
		"octal max":          {`'\777'`, "?7"},
	} {
		t.Run(name, func(t *testing.T) {
			doc, err := parse.ParseQuery("SELECT c/uid/value FROM COMPOSITION c WHERE c/n = " + tc.aqlText)
			if err != nil {
				t.Fatalf("ParseQuery: %v", err)
			}
			got := doc.Where.(aql.Comparison).Val.(aql.StringValue).S
			if got != tc.want {
				t.Errorf("decoded %q, want %q", got, tc.want)
			}
		})
	}
}

// parseWhereString parses `WHERE c/n = <lit>` and returns the decoded string.
func parseWhereString(lit string) (string, error) {
	doc, err := parse.ParseQuery("SELECT c/uid/value FROM COMPOSITION c WHERE c/n = " + lit)
	if err != nil {
		return "", err
	}
	return doc.Where.(aql.Comparison).Val.(aql.StringValue).S, nil
}

// TestStringRoundTripIsAFixedPoint — REQ-119's closure property applied TWICE.
// Decoding a literal and re-emitting it MUST reach a fixed point, which is
// stronger than "the emitted text parses" and is what a caller who reads a
// query, edits one clause, and writes it back depends on.
//
// The escapes carrying an arbitrary BYTE are where this used to fail: `'\377'`
// decoded to a lone 0xFF, `token` copied it through raw, and the SECOND parse
// yielded U+FFFD — the lexer decodes its input to runes. The value came back
// changed with no error anywhere.
func TestStringRoundTripIsAFixedPoint(t *testing.T) {
	for name, lit := range map[string]string{
		"octal high byte":   `'\377'`,
		"octal 0x80":        `'\200'`,
		"octal utf8 pair":   `'\303\251'`, // a valid two-byte é
		"octal low":         `'\101'`,
		"nul":               `'\0'`,
		"surrogate pair":    "'\\ud83d\\ude00'",
		"escaped quote":     `'O\'Brien'`,
		"escaped backslash": `'C:\\temp'`,
		"plain":             `'plain'`,
	} {
		t.Run(name, func(t *testing.T) {
			first, err := parseWhereString(lit)
			if err != nil {
				t.Fatalf("first parse of %s: %v", lit, err)
			}
			wire := aql.FormatValue(aql.String(first))
			second, err := parseWhereString(wire)
			if err != nil {
				t.Fatalf("re-emitted %s does not parse: %v", wire, err)
			}
			if first != second {
				t.Errorf("not a fixed point\n  literal: %s\n  decode1: %q (% x)\n  re-emit: %s\n  decode2: %q (% x)",
					lit, first, first, wire, second, second)
			}
			if !utf8.ValidString(wire) {
				t.Errorf("emitted invalid UTF-8 (% x); an arbitrary byte must go out as OCTAL_ESC", wire)
			}
		})
	}
}

// TestInvalidUTF8StringSurvivesEmission is the write-side entry to the same
// property: a Go string is a byte string, so one lifted from a latin-1 column
// can carry a byte that begins no valid UTF-8 sequence. Emitted raw it survived
// the STRING token but not the round trip.
func TestInvalidUTF8StringSurvivesEmission(t *testing.T) {
	for name, s := range map[string]string{
		"lone 0xff":      "\xff",
		"0xff 0xfe":      "\xff\xfe",
		"truncated pair": "a\xc3",
		"latin-1 mixed":  "caf\xe9 \u00e9",
		"valid U+FFFD":   "\uFFFD",
	} {
		t.Run(name, func(t *testing.T) {
			wire, got := whereRoundTrip(t, aql.String(s))
			sv, ok := got.(aql.StringValue)
			if !ok {
				t.Fatalf("recovered %T, want aql.StringValue (emitted %s)", got, wire)
			}
			if sv.S != s {
				t.Errorf("round trip lost the value\n  in:   %q (% x)\n  wire: %s\n  out:  %q (% x)",
					s, s, wire, sv.S, sv.S)
			}
		})
	}
}

// TestScientificNotationExtraction — REQ-119. [numericPrimitiveAsValue] reads
// four numeric token shapes; nothing in the corpus ever exercised SCI_REAL or
// SCI_INTEGER, so dropping either from the extractor left every test in the
// module passing while `WHERE c/n = 1e3` became unparseable.
//
// Emission normalises to decimal (`1.5e3` → `1500.0`) — the REQ-055 canonical
// form — so the assertion is on the recovered VALUE and the fixed point, not on
// the text surviving unchanged.
func TestScientificNotationExtraction(t *testing.T) {
	for _, tc := range []struct {
		literal string
		want    float64
	}{
		{"1e3", 1000},
		{"1E3", 1000},
		{"1.5e3", 1500},
		{"1.5E3", 1500},
		{"2e+8", 2e8},
		{"1e-3", 0.001},
		{"-1e3", -1000},
		{"-1.5e-3", -0.0015},
		{"1e20", 1e20},
	} {
		t.Run(tc.literal, func(t *testing.T) {
			doc, err := parse.ParseQuery("SELECT c/uid/value FROM COMPOSITION c WHERE c/n = " + tc.literal)
			if err != nil {
				t.Fatalf("ParseQuery(%s): %v", tc.literal, err)
			}
			rv, ok := doc.Where.(aql.Comparison).Val.(aql.RealValue)
			if !ok {
				t.Fatalf("recovered %T, want aql.RealValue", doc.Where.(aql.Comparison).Val)
			}
			if rv.F != tc.want {
				t.Errorf("recovered %v, want %v", rv.F, tc.want)
			}
			out, err := doc.Emit()
			if err != nil {
				t.Fatalf("Emit: %v", err)
			}
			again, err := parse.ParseQuery(out)
			if err != nil {
				t.Fatalf("re-emitted %q does not parse: %v", out, err)
			}
			if got := again.Where.(aql.Comparison).Val.(aql.RealValue).F; got != tc.want {
				t.Errorf("second parse recovered %v, want %v (wire %q)", got, tc.want, out)
			}
		})
	}
}

// TestNegativeZeroRealKeepsItsSign — `rv.F != f` cannot catch a lost sign bit,
// since -0.0 == 0.0, so the negative-zero case needs [math.Signbit].
func TestNegativeZeroRealKeepsItsSign(t *testing.T) {
	wire, got := whereRoundTrip(t, aql.Real(math.Copysign(0, -1)))
	rv, ok := got.(aql.RealValue)
	if !ok {
		t.Fatalf("recovered %T, want aql.RealValue", got)
	}
	if !math.Signbit(rv.F) {
		t.Errorf("negative zero came back positive (wire %s)", wire)
	}
}

// TestDoubleUnaryMinusIsAccepted — `numericPrimitive : … | SYM_MINUS
// numericPrimitive` is RECURSIVE, so the grammar admits a repeated minus. The
// extractor descended one level only, so `- -5` arrived here as the text `--5`
// and was reported as a literal out of range for the value vocabulary — a wrong
// diagnosis for a nesting it simply did not walk.
func TestDoubleUnaryMinusIsAccepted(t *testing.T) {
	for _, tc := range []struct {
		literal string
		want    int64
	}{
		{"-5", -5},
		{"- -5", 5},
		{"- - -5", -5},
	} {
		t.Run(tc.literal, func(t *testing.T) {
			doc, err := parse.ParseQuery("SELECT c/uid/value FROM COMPOSITION c WHERE c/n = " + tc.literal)
			if err != nil {
				t.Fatalf("ParseQuery(%s): %v", tc.literal, err)
			}
			iv, ok := doc.Where.(aql.Comparison).Val.(aql.IntValue)
			if !ok {
				t.Fatalf("recovered %T, want aql.IntValue", doc.Where.(aql.Comparison).Val)
			}
			if iv.N != tc.want {
				t.Errorf("recovered %d, want %d", iv.N, tc.want)
			}
		})
	}
}

// TestFuncCallRoundTripsToAnEqualValue closes PROBE-090's "every value kind"
// claim for the shape it did not cover: the reserved-name refusals pin what is
// NOT emittable, and this pins that an admitted call comes back equal.
//
// It also exercises [aql.EqualValues] as the property's own comparison, which is
// what PROBE-090's wire assertion is written in terms of.
func TestFuncCallRoundTripsToAnEqualValue(t *testing.T) {
	for name, v := range map[string]aql.Value{
		"path arg":      aql.Func("LENGTH", aql.Path("o/x")),
		"literal arg":   aql.Func("ABS", aql.Int(3)),
		"real arg":      aql.Func("ABS", aql.Real(2)),
		"string arg":    aql.Func("LENGTH", aql.String("O'Brien")),
		"param arg":     aql.Func("LENGTH", aql.Param("p")),
		"nested call":   aql.Func("ABS", aql.Func("LENGTH", aql.Path("o/x"))),
		"multi arg":     aql.Func("CONCAT_WS", aql.String(","), aql.Path("o/x"), aql.Path("o/y")),
		"terminology":   aql.Terminology("expand", "//fhir", "url=x"),
		"lower-case in": aql.Func("length", aql.Path("o/x")),
	} {
		t.Run(name, func(t *testing.T) {
			// The LEFT operand: `functionCall` is one of the two things the
			// grammar admits there (the other is an identifiedPath), so this is
			// the only position that exercises a FuncCall on the read side.
			pred, err := aql.FormatWhere(aql.Compare(v, aql.OpGt, aql.Int(5)))
			if err != nil {
				t.Fatalf("FormatWhere(%#v): %v", v, err)
			}
			doc, err := parse.ParseQuery("SELECT c/uid/value FROM COMPOSITION c WHERE " + pred)
			if err != nil {
				t.Fatalf("emitted %q does not re-parse: %v", pred, err)
			}
			got := doc.Where.(aql.Comparison).Left
			if !aql.EqualValues(v, got) {
				t.Errorf("round trip did not recover an equal value\n  in:   %#v\n  wire: %s\n  out:  %#v",
					v, pred, got)
			}
		})
	}
}

// TestEveryValueKindRoundTripsToAnEqualValue makes PROBE-090's "every value kind
// survives emit → parse → equal value" literal, in the RIGHT-hand operand
// position where the grammar admits a bare literal. Bool / Null / Param / Path
// were the shapes no case covered.
func TestEveryValueKindRoundTripsToAnEqualValue(t *testing.T) {
	for name, v := range map[string]aql.Value{
		"bool true":   aql.Bool(true),
		"bool false":  aql.Bool(false),
		"null":        aql.Null(),
		"param":       aql.Param("ehr_id"),
		"path":        aql.Path("o/x"),
		"int":         aql.Int(-7),
		"int min":     aql.Int(math.MinInt64),
		"int max":     aql.Int(math.MaxInt64),
		"real":        aql.Real(37.5),
		"real whole":  aql.Real(2),
		"real huge":   aql.Real(math.MaxFloat64),
		"real tiny":   aql.Real(math.SmallestNonzeroFloat64),
		"string":      aql.String("plain"),
		"string odd":  aql.String(`O'Brien\`),
		"func":        aql.Func("LENGTH", aql.Path("o/x")),
		"terminology": aql.Terminology("expand", "//fhir", "url=x"),
	} {
		t.Run(name, func(t *testing.T) {
			pred, got := whereRoundTrip(t, v)
			if !aql.EqualValues(v, got) {
				t.Errorf("round trip did not recover an equal value\n  in:   %#v\n  wire: %s\n  out:  %#v",
					v, pred, got)
			}
		})
	}
}

// TestRealLiteralStaysReal — REQ-117. A real MUST emit text that re-lexes as
// the grammar's REAL, not its INTEGER.
//
// `strconv.FormatFloat(…, 'f', …)` drops the fractional part of a whole value,
// so `2.0` emitted `2` and came back an IntValue (a silent type change), while
// 1e20 emitted 21 digits that overflow int64 and came back refused with
// ErrIncompleteAST — the emitted text was not re-parseable at all, breaking
// the fixed-point property outright.
func TestRealLiteralStaysReal(t *testing.T) {
	for name, f := range map[string]float64{
		"whole":        2,
		"fractional":   37.5,
		"negative":     -0.5,
		"beyond int64": 1e20,
		"tiny":         0.000001,
		"zero":         0,
	} {
		t.Run(name, func(t *testing.T) {
			pred, got := whereRoundTrip(t, aql.Real(f))
			rv, ok := got.(aql.RealValue)
			if !ok {
				t.Fatalf("recovered %T, want aql.RealValue (emitted %s)", got, pred)
			}
			if rv.F != f {
				t.Errorf("round trip changed the value: %v -> %v (wire %s)", f, rv.F, pred)
			}
		})
	}
}

// TestNonFiniteRealRefused — a real with no AQL spelling must be refused
// before emission, not rendered as the Go text `+Inf` / `NaN`.
func TestNonFiniteRealRefused(t *testing.T) {
	for name, f := range map[string]float64{
		"+Inf": math.Inf(1),
		"-Inf": math.Inf(-1),
		"NaN":  math.NaN(),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := aql.FormatWhere(aql.Compare(aql.Path("c/n"), aql.OpEq, aql.Real(f)))
			if err == nil {
				t.Fatal("emitted a non-finite real, want ErrInvalidQuery")
			}
			// The sentinel is the contract; the message wording is not — a
			// prose pin here broke on every reword without adding assurance.
			if !errors.Is(err, aql.ErrInvalidQuery) {
				t.Errorf("err = %v, want ErrInvalidQuery", err)
			}
		})
	}
}

// TestIntLiteralRangeBoundaries — REQ-117 residual 1 refuses an INTEGER
// "beyond int64". math.MinInt64 is exactly representable, so refusing it was
// the guard overreaching: the extractor parsed the magnitude before applying
// the sign, and int64's range is asymmetric.
func TestIntLiteralRangeBoundaries(t *testing.T) {
	for name, tc := range map[string]struct {
		literal  string
		accepted bool
		want     int64
	}{
		"MinInt64":       {"-9223372036854775808", true, math.MinInt64},
		"MinInt64+1":     {"-9223372036854775807", true, math.MinInt64 + 1},
		"MaxInt64":       {"9223372036854775807", true, math.MaxInt64},
		"below MinInt64": {"-9223372036854775809", false, 0},
		"above MaxInt64": {"9223372036854775808", false, 0},
	} {
		t.Run(name, func(t *testing.T) {
			doc, err := parse.ParseQuery("SELECT c/uid/value FROM COMPOSITION c WHERE c/n = " + tc.literal)
			if !tc.accepted {
				if err == nil {
					t.Fatalf("%s was accepted; a literal outside int64 must be refused", tc.literal)
				}
				return
			}
			if err != nil {
				t.Fatalf("%s refused, but it is exactly representable: %v", tc.literal, err)
			}
			iv, ok := doc.Where.(aql.Comparison).Val.(aql.IntValue)
			if !ok {
				t.Fatalf("recovered %T, want aql.IntValue", doc.Where.(aql.Comparison).Val)
			}
			if iv.N != tc.want {
				t.Errorf("recovered %d, want %d", iv.N, tc.want)
			}
			out, err := doc.Emit()
			if err != nil {
				t.Fatalf("Emit: %v", err)
			}
			if _, err := parse.ParseQuery(out); err != nil {
				t.Errorf("re-emitted %q does not parse: %v", out, err)
			}
		})
	}
}

// tokenNameRE matches a lexer token declaration (`SELECT: S E L E C T ;`) but
// not a `fragment`, which never produces a token a parser rule can name.
var tokenNameRE = regexp.MustCompile(`(?m)^([A-Z][A-Z0-9_]*)\s*:`)

// keywordFragmentTexts are the keyword SPELLINGS that carry no token name of
// their own, so [tokenNameRE] cannot reach them: `BOOLEAN : SYM_TRUE |
// SYM_FALSE` names the token, while `true` / `false` live in fragments. They are
// exactly where the reserved list was wrong — BOOLEAN is declared AFTER
// IDENTIFIER, so the grammar admits `TRUE(o/x)` and refusing it made Emit reject
// an AST ParseQuery had just produced. Anything the grammar spells only inside a
// fragment belongs here, or the "every token name" claim is not the whole rule.
var keywordFragmentTexts = []string{"true", "TRUE", "false", "FALSE", "CONTAINS_STR"}

// TestReservedFuncNamesTrackTheGrammar holds aql's reserved-word list honest
// against the vendored grammar it was derived from.
//
// `openehr/aql` cannot import the generated lexer (the dependency runs the
// other way), so the write side carries a hand-maintained list of keywords
// that shadow IDENTIFIER and therefore cannot name a value-position function.
// This test reads every token name out of the grammar file — plus the keyword
// texts that live only in fragments, see [keywordFragmentTexts] — and asserts,
// for each, that the validator and the parser agree: a name the builder accepts
// MUST produce parseable AQL, and one it refuses MUST NOT. A keyword added
// upstream fails here instead of silently becoming emittable.
//
// Shadowing turns on DECLARATION ORDER, not on keyword-ness — only a token
// declared before `IDENTIFIER` shadows it — which is why the property under test
// is builder/parser AGREEMENT rather than membership of any list.
func TestReservedFuncNamesTrackTheGrammar(t *testing.T) {
	path := filepath.Join("..", "..", "..", "resources", "aql", "grammar", "active", "AqlLexer.g4")
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read grammar: %v", err)
	}
	names := tokenNameRE.FindAllStringSubmatch(string(src), -1)
	if len(names) < 40 {
		t.Fatalf("only %d token names matched — the extraction regexp has drifted", len(names))
	}
	candidates := slices.Clone(keywordFragmentTexts)
	for _, m := range names {
		// Only a name shaped like an identifier can be spelled as a function name
		// at all. `SYM_*` are the symbol tokens; nothing else is filtered, since
		// a composite token's NAME (STRING, BOOLEAN, URI, …) is itself a legal
		// identifier and so is a candidate the parser must agree about.
		if !strings.HasPrefix(m[1], "SYM_") {
			candidates = append(candidates, m[1])
		}
	}
	var checked int
	for _, name := range candidates {
		checked++
		t.Run(name, func(t *testing.T) {
			// TERMINOLOGY has its own grammar rule with a fixed arity and
			// argument type, so probing it with a path would measure the
			// argument rule rather than whether the NAME is admissible.
			args, argText := []aql.Value{aql.Path("o/x")}, "o/x"
			if name == aql.TerminologyFunc {
				args = []aql.Value{aql.String("a"), aql.String("b"), aql.String("c")}
				argText = "'a', 'b', 'c'"
			}
			builderErr := func() error {
				_, err := aql.FormatWhere(aql.Compare(aql.Func(name, args...), aql.OpGt, aql.Int(5)))
				return err
			}()
			_, parseErr := parse.ParseQuery(
				"SELECT c/uid/value FROM COMPOSITION c WHERE " + name + "(" + argText + ") > 5")

			switch {
			case builderErr == nil && parseErr != nil:
				t.Errorf("the builder emits %s(o/x) but the grammar refuses it: %v\n"+
					"add %q to reservedNonFuncWords in openehr/aql/value.go", name, parseErr, name)
			case builderErr != nil && parseErr == nil:
				t.Errorf("the builder refuses %s(o/x) but the grammar accepts it: %v\n"+
					"remove %q from reservedNonFuncWords in openehr/aql/value.go", name, builderErr, name)
			}
		})
	}
	if checked < 40 {
		t.Fatalf("only %d candidate names checked — the filter is too aggressive", checked)
	}
	t.Logf("checked %d grammar token names against the reserved list", checked)
}

// TestDoubleQuotedStringDecodes — the STRING token admits BOTH delimiters
// (`"..."` alongside `'...'`), and unquoteAQLString has always handled both —
// but no test asserted the VALUE. The one corpus row carrying a double-quoted
// literal asserts emit-idempotence, which stays green even if the `"` branch
// corrupts the value: quotes embedded in the string reach a fixed point too.
// Deleting that branch passed the whole suite while every double-quoted
// literal came back with its delimiters inside the value.
func TestDoubleQuotedStringDecodes(t *testing.T) {
	for name, tc := range map[string]struct{ lit, want string }{
		"plain":            {`"dq"`, "dq"},
		"apostrophe":       {`"O'Brien"`, "O'Brien"},
		"escaped dquote":   {`"say \"hi\""`, `say "hi"`},
		"escapes":          {`"a\nb"`, "a\nb"},
		"empty":            {`""`, ""},
		"single unchanged": {`'sq'`, "sq"},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := parseWhereString(tc.lit)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if got != tc.want {
				t.Errorf("decoded %q, want %q", got, tc.want)
			}
			// The canonical re-emission is single-quoted either way.
			wire := aql.FormatValue(aql.String(got))
			second, err := parseWhereString(wire)
			if err != nil || second != tc.want {
				t.Errorf("re-emission %s decoded %q err=%v, want %q", wire, second, err, tc.want)
			}
		})
	}
}

// TestDoubleQuotedTemporalLiteralRoundTrips — the DATE / TIME / DATETIME
// tokens admit both delimiters too, and stripSurroundingQuotes peeled single
// quotes only: `= "2026-01-01T00:00:00"` re-emitted as
// `'"2026-01-01T00:00:00"'` — the quotes EMBEDDED in the comparison value, a
// predicate that matches nothing, err == nil (REQ-119's silent class, found
// by review after the string sibling was fixed).
func TestDoubleQuotedTemporalLiteralRoundTrips(t *testing.T) {
	for name, tc := range map[string]struct{ lit, want string }{
		"datetime": {`"2026-01-01T00:00:00"`, "2026-01-01T00:00:00"},
		"date":     {`"2026-01-01"`, "2026-01-01"},
		"time":     {`"00:00:00"`, "00:00:00"},
		"single":   {`'2026-01-01T00:00:00'`, "2026-01-01T00:00:00"},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := parseWhereString(tc.lit)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if got != tc.want {
				t.Errorf("decoded %q, want %q — delimiters must never enter the value", got, tc.want)
			}
			wire := aql.FormatValue(aql.String(got))
			if strings.ContainsAny(wire, `"`) {
				t.Errorf("re-emission %s still carries a double quote", wire)
			}
			if second, err := parseWhereString(wire); err != nil || second != tc.want {
				t.Errorf("re-emission %s decoded %q err=%v, want %q", wire, second, err, tc.want)
			}
		})
	}
}

// FuzzStringLiteralRoundTrip is the committed form of the adversarial sweep
// the review rounds ran by hand: for ANY byte string, the emitted literal
// must re-parse to exactly the input. The seeds cover the classes that were
// wrong at least once — quotes, backslashes, C0 controls, invalid UTF-8,
// truncated multi-byte runes, escape-lookalike text.
func FuzzStringLiteralRoundTrip(f *testing.F) {
	for _, seed := range []string{
		"", "plain", "O'Brien", `C:\temp`, `say "hi"`, "a\nb\tc",
		"\x00\x1f\x7f", "\x80\x80", "\xF0\x9F", "tail\\", `\'\\`,
		"a\\u0041b", "\xF0\x9F\x98\x80", "Grüße — 日本語", "\ufffd",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, s string) {
		wire := aql.FormatValue(aql.String(s))
		got, err := parseWhereString(wire)
		if err != nil {
			t.Fatalf("String(%q) emitted %s, which does not parse: %v", s, wire, err)
		}
		if got != s {
			t.Fatalf("round trip changed the value\n  in:  %q (% x)\n  wire: %s\n  out: %q (% x)", s, s, wire, got, got)
		}
	})
}
