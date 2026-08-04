package aql_test

// introspect_test.go pins the REQ-113 Phase 3a vocabulary unification:
// the WhereExpr / Value constructor helpers (Eq, Ne, …,
// Param, String, Int, Real, Bool, Null, And, Or) return the EXPORTED
// concrete types (Comparison, Junction, ParamValue, StringValue,
// IntValue, RealValue, BoolValue, NullValue) whose fields a consumer
// can read after a type assertion. Builder API itself is unchanged —
// these tests assert the introspection surface; the emitter parity
// is covered by builder_test.go.

import (
	"errors"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aql"
)

func TestComparisonExposesFields(t *testing.T) {
	w := aql.Eq("o/data[at0001]", aql.Param("threshold"))
	c, ok := w.(aql.Comparison)
	if !ok {
		t.Fatalf("Eq did not return aql.Comparison; got %T", w)
	}
	if c.Path != "o/data[at0001]" {
		t.Errorf("Comparison.Path = %q, want o/data[at0001]", c.Path)
	}
	if c.Op != aql.OpEq {
		t.Errorf("Comparison.Op = %q, want OpEq (%q)", c.Op, aql.OpEq)
	}
	pv, ok := c.Val.(aql.ParamValue)
	if !ok {
		t.Fatalf("Comparison.Val type %T; want aql.ParamValue", c.Val)
	}
	if pv.Name != "threshold" {
		t.Errorf("ParamValue.Name = %q, want threshold (without $)", pv.Name)
	}
}

func TestComparisonOperatorMatrix(t *testing.T) {
	for op, ctor := range map[aql.Operator]func(string, aql.Value) aql.WhereExpr{
		aql.OpEq: aql.Eq,
		aql.OpNe: aql.Ne,
		aql.OpGt: aql.Gt,
		aql.OpGe: aql.Ge,
		aql.OpLt: aql.Lt,
		aql.OpLe: aql.Le,
	} {
		c := ctor("p", aql.Int(1)).(aql.Comparison)
		if c.Op != op {
			t.Errorf("constructor for %q produced Op=%q", op, c.Op)
		}
	}
}

func TestJunctionExposesFields(t *testing.T) {
	w := aql.And(aql.Eq("a", aql.Int(1)), aql.Eq("b", aql.Int(2)))
	j, ok := w.(aql.Junction)
	if !ok {
		t.Fatalf("And did not return aql.Junction; got %T", w)
	}
	if j.Op != aql.OpAnd {
		t.Errorf("Junction.Op = %q, want OpAnd", j.Op)
	}
	if len(j.Terms) != 2 {
		t.Fatalf("Junction.Terms len = %d, want 2", len(j.Terms))
	}
	// Both terms are Comparison values; assert nested introspection works.
	for i, term := range j.Terms {
		if _, ok := term.(aql.Comparison); !ok {
			t.Errorf("Junction.Terms[%d] type %T; want aql.Comparison", i, term)
		}
	}

	o := aql.Or(aql.Eq("a", aql.Int(1)), aql.Eq("b", aql.Int(2))).(aql.Junction)
	if o.Op != aql.OpOr {
		t.Errorf("Or produced Junction.Op = %q, want OpOr", o.Op)
	}
}

func TestValueLiteralsExposeFields(t *testing.T) {
	if s := aql.String("hi").(aql.StringValue); s.S != "hi" {
		t.Errorf("StringValue.S = %q, want hi", s.S)
	}
	if i := aql.Int(42).(aql.IntValue); i.N != 42 {
		t.Errorf("IntValue.N = %d, want 42", i.N)
	}
	if r := aql.Real(3.14).(aql.RealValue); r.F != 3.14 {
		t.Errorf("RealValue.F = %v, want 3.14", r.F)
	}
	if b := aql.Bool(true).(aql.BoolValue); !b.B {
		t.Errorf("BoolValue.B = %v, want true", b.B)
	}
}

func TestParamValueStripsLeadingDollar(t *testing.T) {
	for _, in := range []string{"ehr_id", "$ehr_id"} {
		p := aql.Param(in).(aql.ParamValue)
		if p.Name != "ehr_id" {
			t.Errorf("Param(%q).Name = %q, want ehr_id", in, p.Name)
		}
	}
}

// REQ-117: PathValue puts an identified path in a value position (a
// path-vs-path comparison RHS or a function argument). The write-side
// constructor sets Raw; the parser additionally decomposes alias +
// segments.
func TestPathValueIntrospection(t *testing.T) {
	v := aql.Path("b/y")
	pv, ok := v.(aql.PathValue)
	if !ok {
		t.Fatalf("aql.Path returned %T, want aql.PathValue", v)
	}
	if pv.Raw != "b/y" {
		t.Errorf("PathValue.Raw = %q, want b/y", pv.Raw)
	}
	if got := aql.FormatValue(v); got != "b/y" {
		t.Errorf("FormatValue(Path) = %q, want b/y (unquoted path text)", got)
	}
	// Read-side shape: alias + segments are introspectable through the
	// embedded aql.IdentifiedPath.
	structured := aql.PathValue{IdentifiedPath: aql.IdentifiedPath{
		Alias:    "b",
		Segments: []aql.PathSegment{{Name: "y"}},
		Raw:      "b/y",
	}}
	if structured.Alias != "b" || len(structured.Segments) != 1 {
		t.Errorf("PathValue embedded path = %+v, want alias b + one segment", structured.IdentifiedPath)
	}
	if w := aql.Eq("a/x", aql.Path("b/y")); mustFormat(t, w) != "a/x = b/y" {
		t.Errorf("path-vs-path comparison = %q, want a/x = b/y", mustFormat(t, w))
	}
}

// REQ-117: FuncCall is a function call in a value position — a comparison
// operand on either side, or a nested argument. Emission upper-cases the
// name and joins arguments with the package's `, ` convention.
func TestFuncCallIntrospection(t *testing.T) {
	v := aql.Func("length", aql.Path("o/name/value"))
	fc, ok := v.(aql.FuncCall)
	if !ok {
		t.Fatalf("aql.Func returned %T, want aql.FuncCall", v)
	}
	if fc.Name != "LENGTH" {
		t.Errorf("FuncCall.Name = %q, want LENGTH (canonical upper case)", fc.Name)
	}
	if len(fc.Args) != 1 {
		t.Fatalf("FuncCall.Args len = %d, want 1", len(fc.Args))
	}
	if got := aql.FormatValue(v); got != "LENGTH(o/name/value)" {
		t.Errorf("FormatValue(Func) = %q, want LENGTH(o/name/value)", got)
	}
	nested := aql.Func("CONCAT", aql.String("a"), aql.Param("p"), aql.Func("LENGTH", aql.Path("x/y")))
	if got := aql.FormatValue(nested); got != "CONCAT('a', $p, LENGTH(x/y))" {
		t.Errorf("nested FormatValue = %q, want CONCAT('a', $p, LENGTH(x/y))", got)
	}
}

// REQ-117: Compare builds a comparison whose LEFT operand is a structured
// value rather than a path — the write-side mirror of the parser's
// function-call WHERE LHS.
func TestCompareFunctionLeftOperand(t *testing.T) {
	w := aql.Compare(aql.Func("LENGTH", aql.Path("o/name/value")), aql.OpGt, aql.Int(5))
	c, ok := w.(aql.Comparison)
	if !ok {
		t.Fatalf("aql.Compare returned %T, want aql.Comparison", w)
	}
	if c.Path != "" {
		t.Errorf("Comparison.Path = %q, want empty for a value left operand", c.Path)
	}
	if _, ok := c.Left.(aql.FuncCall); !ok {
		t.Fatalf("Comparison.Left = %T, want aql.FuncCall", c.Left)
	}
	if got := mustFormat(t, w); got != "LENGTH(o/name/value) > 5" {
		t.Errorf("FormatWhere = %q, want LENGTH(o/name/value) > 5", got)
	}
}

// REQ-117: an empty left operand and a nil value stay build-time errors —
// the new shapes do not widen what the emitter will improvise.
func TestComparisonRejectsMalformedOperands(t *testing.T) {
	for name, w := range map[string]aql.WhereExpr{
		"empty_path":      aql.Eq("", aql.Int(1)),
		"nil_value":       aql.Eq("a/x", nil),
		"empty_path_val":  aql.Eq("a/x", aql.Path("  ")),
		"unnamed_func":    aql.Compare(aql.FuncCall{}, aql.OpEq, aql.Int(1)),
		"nil_func_arg":    aql.Compare(aql.FuncCall{Name: "LENGTH", Args: []aql.Value{nil}}, aql.OpEq, aql.Int(1)),
		"empty_func_path": aql.Compare(aql.Func("LENGTH", aql.Path("")), aql.OpEq, aql.Int(1)),
	} {
		if _, err := aql.FormatWhere(w); !errors.Is(err, aql.ErrInvalidQuery) {
			t.Errorf("%s: FormatWhere error = %v, want ErrInvalidQuery", name, err)
		}
	}
}

func mustFormat(t *testing.T, w aql.WhereExpr) string {
	t.Helper()
	got, err := aql.FormatWhere(w)
	if err != nil {
		t.Fatalf("FormatWhere: %v", err)
	}
	return got
}

// REQ-113: NullValue is a typed sentinel distinct from StringValue{"NULL"};
// its token form is the unquoted keyword.
func TestNullValueTokenIsUnquoted(t *testing.T) {
	v := aql.Null()
	if _, ok := v.(aql.NullValue); !ok {
		t.Fatalf("aql.Null() returned %T, want NullValue", v)
	}
	if got := aql.FormatValue(v); got != "NULL" {
		t.Errorf("FormatValue(Null) = %q, want NULL (no quotes)", got)
	}
}
