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
	// The POINTER twin of three good arguments. The SELECT-side carrier is
	// pinned for this ("TERMINOLOGY 3 pointer strings" in emit_parity_test.go);
	// the value-side one was not, in the very commit that bound the rule to
	// BOTH carriers — so dropping the normalisation here left CI green.
	t.Run("pointer string arguments", func(t *testing.T) {
		call := aql.FuncCall{Name: aql.TerminologyFunc, Args: []aql.Value{
			&aql.StringValue{S: "expand"}, &aql.StringValue{S: "//fhir"}, &aql.StringValue{S: "url=x"},
		}}
		if err := aql.ValidateValue(call); err != nil {
			t.Errorf("ValidateValue with *StringValue arguments = %v, want nil", err)
		}
		out, err := aql.FormatWhere(aql.Compare(call, aql.OpEq, aql.String("y")))
		if err != nil {
			t.Fatalf("FormatWhere with *StringValue arguments: %v", err)
		}
		if want := "TERMINOLOGY('expand', '//fhir', 'url=x') = 'y'"; out != want {
			t.Errorf("out = %q, want %q", out, want)
		}
	})

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

// TestParameterNamesMustSpellThePARAMETERToken — REQ-119.
//
// `PARAMETER : '$' IDENTIFIER_CHAR` with `IDENTIFIER_CHAR : ALPHA_CHAR
// WORD_CHAR*`. This position had NO guard, though [aql.Param] is what
// § REQ-055 rule 4 designates as the channel for caller data.
//
// Two failure modes, and the second is why this is a fix rather than a
// tidy-up: `Param("a b")` emits `$a b`, which the parser rejects (loud), while
// `Param("p AND c/secret = 1")` emits text that parses CLEANLY as two
// predicates instead of one — a structure change through the channel the
// injection guard recommends, invisible to every round-trip check.
func TestParameterNamesMustSpellThePARAMETERToken(t *testing.T) {
	for name, param := range map[string]string{
		"empty":            "",
		"leading digit":    "1x",
		"leading dollar":   "$$x",
		"embedded space":   "a b",
		"embedded slash":   "a/b",
		"embedded dash":    "a-b",
		"whole predicate":  "p AND c/secret = 1",
		"closing brace":    "p} OR c/y = 1",
		"trailing newline": "p\n",
	} {
		t.Run("refused/"+name, func(t *testing.T) {
			if err := aql.ValidateValue(aql.Param(param)); !errors.Is(err, aql.ErrInvalidQuery) {
				t.Errorf("ValidateValue(Param(%q)) = %v, want ErrInvalidQuery", param, err)
			}
			if _, err := aql.FormatWhere(aql.Eq("c/x", aql.Param(param))); !errors.Is(err, aql.ErrInvalidQuery) {
				t.Errorf("FormatWhere with Param(%q) = %v, want ErrInvalidQuery", param, err)
			}
		})
	}

	// Positive controls — every spelling the token admits must keep working,
	// including the documented `$`-stripping in [aql.Param].
	for name, param := range map[string]string{
		"lower":            "ehr_id",
		"upper":            "EHRID",
		"mixed with digit": "p1",
		"underscored":      "a_b_c",
		"single letter":    "p",
		"dollar stripped":  "$ehr_id",
	} {
		t.Run("accepted/"+name, func(t *testing.T) {
			if err := aql.ValidateValue(aql.Param(param)); err != nil {
				t.Errorf("ValidateValue(Param(%q)) = %v, want nil", param, err)
			}
		})
	}
}

// TestNarrowOperandSlotsRefuseAWiderValue — REQ-119.
//
// Three positions are NARROWER than the general `terminal` value position, and
// all three delegated to validateValue, which enforces the wider set:
//
//	likeOperand    : STRING | PARAMETER
//	valueListItem  : primitive | PARAMETER | terminologyFunction
//	matchesOperand : … | terminologyFunction        (the bare, brace-less form)
//
// So `aql.Like` and `aql.Matches` — documented public constructors — emitted
// `c/x LIKE 5` and `c/x MATCHES {c/y}` with err == nil. It is the same argument
// the TERMINOLOGY arity guard already makes, one rule down.
func TestNarrowOperandSlotsRefuseAWiderValue(t *testing.T) {
	terminology := aql.Terminology("expand", "//fhir", "url=x")
	termCall, _ := terminology.(aql.FuncCall)

	t.Run("LIKE", func(t *testing.T) {
		for name, v := range map[string]aql.Value{
			"integer":       aql.Int(5),
			"real":          aql.Real(2),
			"boolean":       aql.Bool(true),
			"null":          aql.Null(),
			"path":          aql.Path("c/y"),
			"function call": aql.Func("LENGTH", aql.Path("c/y")),
			"terminology":   terminology,
		} {
			t.Run("refused/"+name, func(t *testing.T) {
				if _, err := aql.FormatWhere(aql.Like("c/x", v)); !errors.Is(err, aql.ErrInvalidQuery) {
					t.Errorf("Like with %T = %v, want ErrInvalidQuery", v, err)
				}
			})
		}
		for name, v := range map[string]aql.Value{
			"string":         aql.String("a%"),
			"parameter":      aql.Param("p"),
			"pointer string": &aql.StringValue{S: "a%"},
		} {
			t.Run("accepted/"+name, func(t *testing.T) {
				if _, err := aql.FormatWhere(aql.Like("c/x", v)); err != nil {
					t.Errorf("Like with %T = %v, want nil", v, err)
				}
			})
		}
	})

	t.Run("MATCHES value list", func(t *testing.T) {
		for name, v := range map[string]aql.Value{
			"path":          aql.Path("c/y"),
			"function call": aql.Func("LENGTH", aql.Path("c/y")),
		} {
			t.Run("refused/"+name, func(t *testing.T) {
				if _, err := aql.FormatWhere(aql.Matches("c/x", v)); !errors.Is(err, aql.ErrInvalidQuery) {
					t.Errorf("Matches with %T = %v, want ErrInvalidQuery", v, err)
				}
			})
		}
		// A path in ANY position of the list, not only the first.
		t.Run("refused/path after a good member", func(t *testing.T) {
			_, err := aql.FormatWhere(aql.Matches("c/x", aql.String("a"), aql.Path("c/y")))
			if !errors.Is(err, aql.ErrInvalidQuery) {
				t.Errorf("err = %v, want ErrInvalidQuery", err)
			}
		})
		for name, v := range map[string]aql.Value{
			"string":      aql.String("a"),
			"integer":     aql.Int(5),
			"real":        aql.Real(2.5),
			"boolean":     aql.Bool(false),
			"null":        aql.Null(),
			"parameter":   aql.Param("p"),
			"terminology": terminology,
			"pointer":     &aql.StringValue{S: "a"},
		} {
			t.Run("accepted/"+name, func(t *testing.T) {
				if _, err := aql.FormatWhere(aql.Matches("c/x", v)); err != nil {
					t.Errorf("Matches with %T = %v, want nil", v, err)
				}
			})
		}
	})

	t.Run("bare MATCHES terminology operand", func(t *testing.T) {
		bad := aql.MatchesExpr{Path: "c/x", Terminology: &aql.FuncCall{
			Name: "LENGTH", Args: []aql.Value{aql.Path("c/y")},
		}}
		if _, err := aql.FormatWhere(bad); !errors.Is(err, aql.ErrInvalidQuery) {
			t.Errorf("bare LENGTH() operand = %v, want ErrInvalidQuery", err)
		}
		good := aql.MatchesExpr{Path: "c/x", Terminology: &termCall}
		if _, err := aql.FormatWhere(good); err != nil {
			t.Errorf("bare TERMINOLOGY() operand = %v, want nil", err)
		}
	})
}

// TestTerminologyGuardTrimsTheName — REQ-119.
//
// [aql.FuncCall.token] emits the TRIMMED, upper-cased name, and checkFuncName
// trims before its own lookup — but the arity/argument gate read the raw field,
// so ` terminology ` slipped past it and then emitted `TERMINOLOGY(1, 2)`. The
// SELECT-side twin (validateSelectTerminology) already trimmed: the same rule,
// bound to one carrier and not the other.
func TestTerminologyGuardTrimsTheName(t *testing.T) {
	for _, name := range []string{" TERMINOLOGY", "TERMINOLOGY ", " terminology\t", "\nTerminology "} {
		t.Run(name, func(t *testing.T) {
			wrong := aql.FuncCall{Name: name, Args: []aql.Value{aql.Int(1), aql.Int(2)}}
			if err := aql.ValidateValue(wrong); !errors.Is(err, aql.ErrInvalidQuery) {
				t.Errorf("ValidateValue(%q with 2 int args) = %v, want ErrInvalidQuery", name, err)
			}
			right := aql.FuncCall{Name: name, Args: []aql.Value{
				aql.String("a"), aql.String("b"), aql.String("c"),
			}}
			if err := aql.ValidateValue(right); err != nil {
				t.Errorf("ValidateValue(%q with 3 string args) = %v, want nil", name, err)
			}
		})
	}
}

// TestJunctionValidatesItsOperatorAndArity — REQ-119.
//
// [aql.Comparison] has always checked its operator; [aql.Junction] never checked
// its own, and BoolOp had no `known()` at all. The zero value is the reachable
// one — a hand-assembled Junction carries it by default — and it emitted two
// predicates joined by nothing.
//
// The term-less case is the same constructor-guards-it asymmetry the typed-nil
// work closed for terms: [aql.And] / [aql.Or] collapse the empty case to nil,
// the struct literal does not, and `NOT ()` reaches the wire.
func TestJunctionValidatesItsOperatorAndArity(t *testing.T) {
	a := aql.Eq("c/x", aql.Int(1))
	b := aql.Eq("c/y", aql.Int(2))

	for name, w := range map[string]aql.WhereExpr{
		"zero-value operator":    aql.Junction{Terms: []aql.WhereExpr{a, b}},
		"unknown operator":       aql.Junction{Op: aql.BoolOp("XOR"), Terms: []aql.WhereExpr{a, b}},
		"non-canonical casing":   aql.Junction{Op: aql.BoolOp("and"), Terms: []aql.WhereExpr{a, b}},
		"no terms":               aql.Junction{Op: aql.OpAnd},
		"no terms under a NOT":   aql.NotExpr{Operand: aql.Junction{Op: aql.OpAnd}},
		"no terms as an operand": aql.Junction{Op: aql.OpAnd, Terms: []aql.WhereExpr{a, aql.Junction{Op: aql.OpOr}}},
	} {
		t.Run("refused/"+name, func(t *testing.T) {
			out, err := aql.FormatWhere(w)
			if !errors.Is(err, aql.ErrInvalidQuery) {
				t.Fatalf("err = %v, want ErrInvalidQuery (emitted %q)", err, out)
			}
			if out != "" {
				t.Errorf("a refused junction still emitted %q", out)
			}
		})
	}

	for name, tc := range map[string]struct {
		w    aql.WhereExpr
		want string
	}{
		"AND of two":  {aql.Junction{Op: aql.OpAnd, Terms: []aql.WhereExpr{a, b}}, "c/x = 1 AND c/y = 2"},
		"OR of two":   {aql.Junction{Op: aql.OpOr, Terms: []aql.WhereExpr{a, b}}, "c/x = 1 OR c/y = 2"},
		"single term": {aql.Junction{Op: aql.OpAnd, Terms: []aql.WhereExpr{a}}, "c/x = 1"},
	} {
		t.Run("accepted/"+name, func(t *testing.T) {
			out, err := aql.FormatWhere(tc.w)
			if err != nil {
				t.Fatalf("err = %v, want nil", err)
			}
			if out != tc.want {
				t.Errorf("out = %q, want %q", out, tc.want)
			}
		})
	}
}

// TestFormatValueDoesNotPanicOnATypedNilArgument — REQ-119.
//
// [aql.FormatValue] is the unvalidated escape hatch, so a value with no wire
// form reaches it by design; [aql.FuncCall.token] then renders each ARGUMENT,
// and a typed-nil argument is the same pointer twin one level in. The existing
// no-panic test enumerates typed-nil values but never one nested inside a call.
func TestFormatValueDoesNotPanicOnATypedNilArgument(t *testing.T) {
	for name, arg := range map[string]aql.Value{
		"untyped nil":            nil,
		"typed-nil *PathValue":   (*aql.PathValue)(nil),
		"typed-nil *FuncCall":    (*aql.FuncCall)(nil),
		"typed-nil *StringValue": (*aql.StringValue)(nil),
	} {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("FormatValue panicked: %v", r)
				}
			}()
			if got, want := aql.FormatValue(aql.Func("LENGTH", arg)), "LENGTH()"; got != want {
				t.Errorf("FormatValue = %q, want %q (a nil argument renders as nothing)", got, want)
			}
		})
	}
}
