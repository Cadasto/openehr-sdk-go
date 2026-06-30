package aql_test

// introspect_test.go pins the SDK-GAP-17 Phase 3a vocabulary unification:
// the WhereExpr / Value constructor helpers (Eq, Ne, …, Param, String,
// Int, Real, Bool, And, Or) return the EXPORTED concrete types
// (Comparison, Junction, ParamValue, StringValue, IntValue, RealValue,
// BoolValue) whose fields a consumer can read after a type assertion.
// Builder API itself is unchanged — these tests assert the introspection
// surface; the emitter parity is covered by builder_test.go.

import (
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
