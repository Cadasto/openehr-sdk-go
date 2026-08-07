package parse_test

// literal_roundtrip_test.go pins the value-position literal contracts of the
// REQ-117 fixed-point property that the corpus in roundtrip_test.go could not
// reach: the corpus is written in canonical form, so it never contained a
// string carrying a quote or a backslash, nor a real whose decimal rendering
// carries no fractional part. Both spellings emitted text this SDK's own
// parser rejects — a class of defect the round-trip corpus structurally cannot
// surface unless a case actually exercises the escape.

import (
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

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
		{"a\nb", `'a\nb'`},
		{"plain", `'plain'`},
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
			if !strings.Contains(err.Error(), "no AQL spelling") {
				t.Errorf("err = %v, want the no-AQL-spelling refusal", err)
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

// TestReservedFuncNamesTrackTheGrammar holds aql's reserved-word list honest
// against the vendored grammar it was derived from.
//
// `openehr/aql` cannot import the generated lexer (the dependency runs the
// other way), so the write side carries a hand-maintained list of keywords
// that shadow IDENTIFIER and therefore cannot name a value-position function.
// This test reads every token name out of the grammar file and asserts, for
// each, that the validator and the parser agree: a name the builder accepts
// MUST produce parseable AQL, and one it refuses MUST NOT. A keyword added
// upstream fails here instead of silently becoming emittable.
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
	var checked int
	for _, m := range names {
		name := m[1]
		// Only names shaped like an identifier can be spelled as a function
		// name at all; the symbol and composite tokens (SYM_*, STRING, …) are
		// not candidates a caller could pass to aql.Func.
		if strings.HasPrefix(name, "SYM_") || strings.HasPrefix(name, "URI_") {
			continue
		}
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
