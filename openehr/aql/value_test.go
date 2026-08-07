package aql_test

// REQ-119 · PROBE-090
//
// value_test.go covers the value-position guards REQ-119 adds to the write side
// of the REQ-117 catalogue: the URI operand (the one position with no quoting to
// hide behind), the reserved-word function names, the pointer shapes that
// bypassed validation entirely, and the total value equality that replaced `==`
// when the vocabulary gained slice fields.

import (
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aql"
)

// TestMatchesURIRefusesUnspellableOperand — REQ-117. `MATCHES {uri}` emits its
// operand verbatim because the grammar's URI token is unquoted, so a `}` ends
// the operand early and the remainder becomes more query.
//
// The injection case is the one that matters: it produces VALID AQL that asks
// something different, so no round-trip, golden, or parser check downstream
// can catch it. Refusal at validation is the only place it is visible.
func TestMatchesURIRefusesUnspellableOperand(t *testing.T) {
	for name, uri := range map[string]string{
		"brace injection": "uri://a} OR c/y MATCHES {uri://b",
		"closing brace":   "http://example.org/x}",
		"opening brace":   "http://example.org/{x}",
		"no scheme":       "not a uri at all",
		"empty scheme":    "://example.org",
		"digit scheme":    "1http://example.org",
		"space in path":   "http://example.org/a b",
		"backslash":       `http://example.org/a\b`,
		"newline":         "http://example.org/a\nb",
		"angle bracket":   "http://example.org/<x>",
		"pipe":            "http://example.org/a|b",
		"scheme bad char": "ht_tp://example.org",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := aql.FormatWhere(aql.MatchesURI("c/x", uri))
			if err == nil {
				t.Fatalf("emitted MATCHES {%s} unrefused", uri)
			}
			if !errors.Is(err, aql.ErrInvalidQuery) {
				t.Errorf("err = %v, want ErrInvalidQuery", err)
			}
		})
	}
}

// TestMatchesURIAcceptsGrammarSpellableOperands is the positive control: the
// guard must not narrow the operand form the grammar actually admits.
func TestMatchesURIAcceptsGrammarSpellableOperands(t *testing.T) {
	for name, uri := range map[string]string{
		"http":          "http://example.org/path",
		"with query":    "http://example.org/p?a=1&b=2",
		"with fragment": "http://example.org/p#frag",
		"terminology":   "terminology://openehr.org/subsets/SNOMED-CT",
		"urn":           "urn:ietf:rfc:3986",
		"pct-encoded":   "http://example.org/a%20b",
		// A single quote is a URI sub-delim, so it needs no escaping and must
		// NOT be refused — the operand is unquoted, so nothing can be broken
		// out of by a character the URI token itself admits.
		"quote":       "http://example.org/'",
		"sub-delims":  "http://example.org/a!$&'()*+,;=",
		"port":        "http://example.org:8080/p",
		"plus scheme": "svn+ssh://example.org/repo",
		"rootless":    "mailto:someone@example.org",
	} {
		t.Run(name, func(t *testing.T) {
			got, err := aql.FormatWhere(aql.MatchesURI("c/x", uri))
			if err != nil {
				t.Fatalf("refused a spellable URI %q: %v", uri, err)
			}
			if want := "c/x MATCHES {" + uri + "}"; got != want {
				t.Errorf("emitted %q, want %q", got, want)
			}
		})
	}
}

// TestTerminologyFuncArity — the grammar gives TERMINOLOGY its own rule with
// exactly three STRING arguments, so a hand-assembled FuncCall that borrows
// the name with any other shape emits text the parser rejects.
func TestTerminologyFuncArity(t *testing.T) {
	for name, args := range map[string][]aql.Value{
		"too few":   {aql.String("a"), aql.String("b")},
		"too many":  {aql.String("a"), aql.String("b"), aql.String("c"), aql.String("d")},
		"none":      nil,
		"path arg":  {aql.String("a"), aql.String("b"), aql.Path("o/x")},
		"int arg":   {aql.Int(1), aql.String("b"), aql.String("c")},
		"param arg": {aql.Param("p"), aql.String("b"), aql.String("c")},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := aql.FormatWhere(aql.Compare(
				aql.Func(aql.TerminologyFunc, args...), aql.OpEq, aql.String("x")))
			if !errors.Is(err, aql.ErrInvalidQuery) {
				t.Errorf("err = %v, want ErrInvalidQuery", err)
			}
		})
	}
	// Positive control — the sanctioned constructor still emits.
	got, err := aql.FormatWhere(aql.Compare(
		aql.Terminology("expand", "//fhir", "url=x"), aql.OpEq, aql.String("y")))
	if err != nil {
		t.Fatalf("Terminology() refused: %v", err)
	}
	if !strings.HasPrefix(got, "TERMINOLOGY('expand', '//fhir', 'url=x')") {
		t.Errorf("emitted %q", got)
	}
}

// TestFuncNameRefusesReservedAndMalformed — a value-position function name
// must lex as the grammar's IDENTIFIER (or one of the *_FUNCTION_ID tokens).
// The aggregates are the trap: COUNT/MIN/MAX/SUM/AVG are real AQL functions,
// but only in SELECT, so `COUNT(o/y) > 5` was emitted with err == nil and
// then rejected by the SDK's own parser.
func TestFuncNameRefusesReservedAndMalformed(t *testing.T) {
	for name, fn := range map[string]string{
		"aggregate COUNT": "COUNT",
		"aggregate AVG":   "AVG",
		"keyword SELECT":  "SELECT",
		"keyword NOT":     "NOT",
		"keyword NULL":    "NULL",
		"leading digit":   "1FUNC",
		"embedded space":  "MY FUNC",
		"embedded paren":  "F(",
		"empty":           "",
		"blank":           "   ",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := aql.FormatWhere(aql.Compare(
				aql.Func(fn, aql.Path("o/x")), aql.OpGt, aql.Int(5)))
			if !errors.Is(err, aql.ErrInvalidQuery) {
				t.Errorf("Func(%q) err = %v, want ErrInvalidQuery", fn, err)
			}
		})
	}
	// Positive control — a grammar function and a plain identifier still emit.
	//
	// `true` / `false` belong here, NOT in the refusal table above: shadowing
	// depends on DECLARATION ORDER, and BOOLEAN is declared AFTER IDENTIFIER in
	// AqlLexer.g4, so `TRUE(o/x)` lexes as an IDENTIFIER and the grammar admits
	// it. Refusing them made Emit reject an AST ParseQuery had just produced —
	// TestReservedFuncNamesTrackTheGrammar's boolean cases pin the parser half.
	for _, fn := range []string{"LENGTH", "ABS", "CONCAT_WS", "my_func", "TRUE", "false"} {
		if _, err := aql.FormatWhere(aql.Compare(
			aql.Func(fn, aql.Path("o/x")), aql.OpGt, aql.Int(5))); err != nil {
			t.Errorf("Func(%q) refused: %v", fn, err)
		}
	}
}

// TestSelectFuncNameAdmitsAggregates — REQ-119. The SELECT-side name check is
// the value-position one minus the aggregates, because `aggregateFunctionCall`
// is reachable from `columnExpr` and from no value position. Emission of a
// projected call goes through it, so `SELECT COUNT(…)` must survive while
// `SELECT SELECT(…)` must not.
func TestSelectFuncNameAdmitsAggregates(t *testing.T) {
	for _, fn := range []string{"COUNT", "MIN", "MAX", "SUM", "AVG", "LENGTH", "my_func"} {
		if err := aql.ValidateSelectFuncName(fn); err != nil {
			t.Errorf("ValidateSelectFuncName(%q) = %v, want nil", fn, err)
		}
		if fn == "LENGTH" || fn == "my_func" {
			continue
		}
		// The same name is still refused in a value position.
		if _, err := aql.FormatWhere(aql.Compare(
			aql.Func(fn, aql.Path("o/x")), aql.OpGt, aql.Int(5))); !errors.Is(err, aql.ErrInvalidQuery) {
			t.Errorf("value-position Func(%q) err = %v, want ErrInvalidQuery", fn, err)
		}
	}
	for _, fn := range []string{"SELECT", "CONTAINS", "1FUNC", ""} {
		if err := aql.ValidateSelectFuncName(fn); !errors.Is(err, aql.ErrInvalidQuery) {
			t.Errorf("ValidateSelectFuncName(%q) = %v, want ErrInvalidQuery", fn, err)
		}
	}
}

// TestPointerShapedValuesAreValidated — REQ-119. `token` has a value receiver,
// so `*FuncCall` and friends satisfy [aql.Value] too, and
// [aql.MatchesExpr.Terminology] is itself a `*FuncCall` — pointer shapes are the
// API's own idiom, not an abuse.
//
// Before REQ-119 they fell through validateValue's type switch to `return nil`,
// so one `&` bypassed every value guard, and a typed-nil pointer PANICKED inside
// FormatWhere and EqualValues rather than being refused.
func TestPointerShapedValuesAreValidated(t *testing.T) {
	for name, v := range map[string]aql.Value{
		"&FuncCall reserved name": &aql.FuncCall{Name: "COUNT", Args: []aql.Value{aql.Path("o/y")}},
		"&RealValue +Inf":         &aql.RealValue{F: math.Inf(1)},
		"&RealValue NaN":          &aql.RealValue{F: math.NaN()},
		"&PathValue empty":        &aql.PathValue{},
		"&FuncCall bad arity":     &aql.FuncCall{Name: aql.TerminologyFunc, Args: []aql.Value{aql.String("a")}},
		"typed-nil *FuncCall":     (*aql.FuncCall)(nil),
		"nested typed-nil arg":    aql.Func("LENGTH", (*aql.PathValue)(nil)),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := aql.FormatWhere(aql.Compare(aql.Path("c/x"), aql.OpGt, v))
			if !errors.Is(err, aql.ErrInvalidQuery) {
				t.Errorf("err = %v, want ErrInvalidQuery", err)
			}
		})
	}
	// A pointer to a VALID shape must still emit, and must emit the same text
	// and compare equal to the value shape it points at.
	ptr := &aql.FuncCall{Name: "LENGTH", Args: []aql.Value{aql.Path("o/x")}}
	val := aql.Func("LENGTH", aql.Path("o/x"))
	got, err := aql.FormatWhere(aql.Compare(aql.Path("c/x"), aql.OpGt, ptr))
	if err != nil {
		t.Fatalf("a pointer to a valid FuncCall was refused: %v", err)
	}
	if want := "c/x > LENGTH(o/x)"; got != want {
		t.Errorf("emitted %q, want %q", got, want)
	}
	if !aql.EqualValues(ptr, val) || !aql.EqualValues(val, ptr) {
		t.Error("EqualValues does not see through a pointer shape")
	}
	// The typed-nil pointer is where `EqualValues` used to panic.
	var nilFn *aql.FuncCall
	if aql.EqualValues(nilFn, val) || aql.EqualValues(val, nilFn) {
		t.Error("a typed-nil pointer compared equal to a populated value")
	}
	if !aql.EqualValues(nilFn, nil) {
		t.Error("a typed-nil pointer should compare equal to a nil Value — both have no wire form")
	}
}

// TestEqualValues — REQ-117 made PathValue and FuncCall uncomparable by adding
// slice fields, so `==` panics on them and on any WhereExpr holding one.
// EqualValues is the total replacement.
func TestEqualValues(t *testing.T) {
	for name, tc := range map[string]struct {
		a, b aql.Value
		want bool
	}{
		"same string":      {aql.String("x"), aql.String("x"), true},
		"diff string":      {aql.String("x"), aql.String("y"), false},
		"same path":        {aql.Path("o/x"), aql.Path("o/x"), true},
		"diff path":        {aql.Path("o/x"), aql.Path("o/y"), false},
		"same func":        {aql.Func("LENGTH", aql.Path("o/x")), aql.Func("LENGTH", aql.Path("o/x")), true},
		"diff func args":   {aql.Func("LENGTH", aql.Path("o/x")), aql.Func("LENGTH", aql.Path("o/y")), false},
		"func case-folded": {aql.Func("length", aql.Path("o/x")), aql.Func("LENGTH", aql.Path("o/x")), true},
		"nested func":      {aql.Func("ABS", aql.Func("LENGTH", aql.Path("o/x"))), aql.Func("ABS", aql.Func("LENGTH", aql.Path("o/x"))), true},
		"int vs real":      {aql.Int(1), aql.Real(1), false},
		"string vs path":   {aql.String("x"), aql.Path("'x'"), false},
		"bool vs path":     {aql.Bool(true), aql.Path("true"), false},
		"null vs null":     {aql.Null(), aql.Null(), true},
		"param":            {aql.Param("p"), aql.Param("p"), true},
		"both nil":         {nil, nil, true},
		"one nil":          {nil, aql.Null(), false},
	} {
		t.Run(name, func(t *testing.T) {
			if got := aql.EqualValues(tc.a, tc.b); got != tc.want {
				t.Errorf("EqualValues(%v, %v) = %v, want %v", tc.a, tc.b, got, tc.want)
			}
			if got := aql.EqualValues(tc.b, tc.a); got != tc.want {
				t.Errorf("not symmetric: EqualValues(%v, %v) = %v, want %v", tc.b, tc.a, got, tc.want)
			}
		})
	}
}

// TestEqualValuesDoesNotPanicWhereEqualsWould pins the motivating case: the
// same comparison spelled with `==` panics.
func TestEqualValuesDoesNotPanicWhereEqualsWould(t *testing.T) {
	a, b := aql.Path("o/x"), aql.Path("o/x")
	if !aql.EqualValues(a, b) {
		t.Error("EqualValues on two identical paths = false")
	}
	func() {
		defer func() {
			if recover() == nil {
				t.Error("`==` on two PathValues did not panic; the doc claim in " +
					"Value § Comparability is stale and should be revisited")
			}
		}()
		_ = a == b //nolint:staticcheck // deliberately provoking the documented panic
	}()

	// § Comparability makes two further claims that were asserted only in prose.
	// Both are the reason EqualValues exists, so both are pinned here: a stale
	// claim would otherwise send a consumer looking for a panic that no longer
	// happens, or leave a real one undocumented.
	mustPanic := func(what string, f func()) {
		t.Helper()
		defer func() {
			if recover() == nil {
				t.Errorf("%s did not panic; the doc claim in Value § Comparability is stale", what)
			}
		}()
		f()
	}
	mustPanic("using a PathValue as a map key", func() {
		m := map[aql.Value]bool{}
		m[aql.Path("o/x")] = true
	})
	mustPanic("`==` on two Comparisons holding a PathValue", func() {
		x := aql.Compare(aql.Path("c/n"), aql.OpEq, aql.Path("o/x"))
		y := aql.Compare(aql.Path("c/n"), aql.OpEq, aql.Path("o/x"))
		_ = x == y //nolint:staticcheck // deliberately provoking the documented panic
	})
	mustPanic("`==` on two LikeExprs holding a PathValue", func() {
		x := aql.LikeExpr{Path: "c/n", Pattern: aql.Path("o/x")}
		y := aql.LikeExpr{Path: "c/n", Pattern: aql.Path("o/x")}
		_ = x == y //nolint:staticcheck // deliberately provoking the documented panic
	})
	// And the documented nil-arg collapse: token() skips an argument with no
	// wire form, so EqualValues cannot distinguish it from an absent one. The
	// emitters refuse such a value, so this only arises before validation.
	if !aql.EqualValues(aql.Func("F", nil), aql.Func("F")) {
		t.Error("the nil-argument collapse documented on EqualValues no longer holds")
	}
}
