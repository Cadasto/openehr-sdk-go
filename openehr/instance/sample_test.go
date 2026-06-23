package instance

import (
	"fmt"
	mrand "math/rand/v2"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/template/constraints"
)

func draws(pc constraints.PrimitiveConstraint, s sampler, n int) []any {
	out := make([]any, n)
	for i := range out {
		out[i] = sampleValue(pc, s)
	}
	return out
}

func distinct(vals []any) int {
	set := map[string]struct{}{}
	for _, v := range vals {
		set[fmt.Sprint(v)] = struct{}{}
	}
	return len(set)
}

// TestSampleValue_validReproducibleVaried covers the three SDK-GAP-14
// guarantees for the in-constraint sampler: every draw is valid against
// the constraint, a fixed seed is reproducible, and the values vary.
func TestSampleValue_validReproducibleVaried(t *testing.T) {
	ci := constraints.CInteger{Range: constraints.NumericRange{
		Lower: 10, Upper: 20, LowerInclusive: true, UpperInclusive: true,
	}}

	seqA := draws(ci, newSampler(mrand.NewPCG(1, 2)), 16)
	seqB := draws(ci, newSampler(mrand.NewPCG(1, 2)), 16)
	seqC := draws(ci, newSampler(mrand.NewPCG(9, 9)), 16)

	for _, v := range seqA {
		if vs := ci.Validate(v); len(vs) != 0 {
			t.Fatalf("sampled %v (%T) violates constraint: %v", v, v, vs)
		}
		n, ok := v.(int64)
		if !ok || n < 10 || n > 20 {
			t.Fatalf("sampled %v not an int64 in [10,20]", v)
		}
	}
	if fmt.Sprint(seqA) != fmt.Sprint(seqB) {
		t.Error("same seed must be reproducible")
	}
	if fmt.Sprint(seqA) == fmt.Sprint(seqC) {
		t.Error("different seeds should produce different sequences")
	}
	if distinct(seqA) < 2 {
		t.Errorf("expected varied values, got %v", seqA)
	}
}

// TestSampleValue_codeListMember confirms a coded-list draw is always a
// member, in the CodedTermRef shape applyPrimitiveExample expects.
func TestSampleValue_codeListMember(t *testing.T) {
	cp := constraints.CodePhrase{Terminology: "local", CodeList: []string{"at0001", "at0002", "at0003"}}
	for _, v := range draws(cp, newSampler(mrand.NewPCG(3, 4)), 12) {
		ref, ok := v.(constraints.CodedTermRef)
		if !ok {
			t.Fatalf("want CodedTermRef, got %T", v)
		}
		if len(cp.Validate(ref)) != 0 {
			t.Fatalf("code %q not a list member", ref.CodeString)
		}
	}
}

// TestSampleValue_unboundedFallsBackToExample confirms an unconstrained
// primitive yields the deterministic ExampleValue (no spurious draw).
func TestSampleValue_unboundedFallsBackToExample(t *testing.T) {
	cs := constraints.CString{} // no list, no pattern
	got := sampleValue(cs, newSampler(mrand.NewPCG(5, 6)))
	if got != cs.ExampleValue() {
		t.Errorf("unbounded CString: got %v, want ExampleValue %v", got, cs.ExampleValue())
	}
}

// TestSampleValue_quantityInRange covers the DvQuantity arm: a bounded
// unit range yields an in-range QuantityValue of the right units and the
// Precision:-1 sentinel applyPrimitiveExample expects.
func TestSampleValue_quantityInRange(t *testing.T) {
	c := constraints.DvQuantity{Units: []constraints.QuantityUnit{{
		Units:     "mm[Hg]",
		Magnitude: constraints.NumericRange{Lower: 40, Upper: 200, LowerInclusive: true, UpperInclusive: true},
	}}}
	for _, v := range draws(c, newSampler(mrand.NewPCG(11, 12)), 12) {
		q, ok := v.(constraints.QuantityValue)
		if !ok {
			t.Fatalf("want QuantityValue, got %T", v)
		}
		if q.Units != "mm[Hg]" || q.Precision != -1 {
			t.Fatalf("unexpected shape %+v", q)
		}
		if len(c.Validate(q)) != 0 {
			t.Fatalf("magnitude %v out of range", q.Magnitude)
		}
	}
}

// TestSampleValue_ordinalMember covers the CDvOrdinal arm: every draw is a
// member value (int), validating against the constraint.
func TestSampleValue_ordinalMember(t *testing.T) {
	c := constraints.CDvOrdinal{Values: []constraints.OrdinalSymbol{
		{Value: 0, Symbol: constraints.CodedTermRef{Terminology: "local", CodeString: "at0"}},
		{Value: 1, Symbol: constraints.CodedTermRef{Terminology: "local", CodeString: "at1"}},
		{Value: 2, Symbol: constraints.CodedTermRef{Terminology: "local", CodeString: "at2"}},
	}}
	for _, v := range draws(c, newSampler(mrand.NewPCG(13, 14)), 12) {
		n, ok := v.(int)
		if !ok {
			t.Fatalf("want int, got %T", v)
		}
		if len(c.Validate(n)) != 0 {
			t.Fatalf("ordinal %d not a member", n)
		}
	}
}

// TestSampleValue_temporal covers the date/time/datetime arms: each sample
// is a string parsing under its own constraint validator.
func TestSampleValue_temporal(t *testing.T) {
	s := newSampler(mrand.NewPCG(15, 16))
	for _, pc := range []constraints.PrimitiveConstraint{constraints.CDate{}, constraints.CTime{}, constraints.CDateTime{}} {
		for range 8 {
			v := sampleValue(pc, s)
			if _, ok := v.(string); !ok {
				t.Fatalf("%T: want string, got %T", pc, v)
			}
			if len(pc.Validate(v)) != 0 {
				t.Fatalf("%T: sampled %q failed its own Validate", pc, v)
			}
		}
	}
}

// TestSampleValue_listIntersectRange covers filterInts + the list∩range
// path: only the in-range list member may be drawn.
func TestSampleValue_listIntersectRange(t *testing.T) {
	c := constraints.CInteger{
		List:  []int64{1, 5, 50, 500},
		Range: constraints.NumericRange{Lower: 10, Upper: 100, LowerInclusive: true, UpperInclusive: true},
	}
	for _, v := range draws(c, newSampler(mrand.NewPCG(17, 18)), 8) {
		if v != int64(50) {
			t.Fatalf("want 50 (sole in-range list member), got %v", v)
		}
	}
}

// TestSampleValue_oneSidedRange covers the one-sided arms of intSampleBounds:
// a lower-only range draws at/above the bound; an upper-only range at/below.
func TestSampleValue_oneSidedRange(t *testing.T) {
	lower := constraints.CInteger{Range: constraints.NumericRange{Lower: 1000, LowerInclusive: true, UpperUnbounded: true}}
	for _, v := range draws(lower, newSampler(mrand.NewPCG(19, 20)), 12) {
		if n := v.(int64); n < 1000 || len(lower.Validate(n)) != 0 {
			t.Fatalf("lower-bounded draw %d not in [1000, ∞)", n)
		}
	}
	upper := constraints.CInteger{Range: constraints.NumericRange{Upper: -1000, UpperInclusive: true, LowerUnbounded: true}}
	for _, v := range draws(upper, newSampler(mrand.NewPCG(21, 22)), 12) {
		if n := v.(int64); n > -1000 || len(upper.Validate(n)) != 0 {
			t.Fatalf("upper-bounded draw %d not in (-∞, -1000]", n)
		}
	}
}

// TestSampleValue_realInRange covers the CReal arm: a bounded range draws
// in [lo,hi) and validates; a list∩range draws the sole in-range member;
// an unbounded CReal varies via the small-value fallback.
func TestSampleValue_realInRange(t *testing.T) {
	bounded := constraints.CReal{Range: constraints.NumericRange{Lower: 1.0, Upper: 5.0, LowerInclusive: true, UpperInclusive: true}}
	seq := draws(bounded, newSampler(mrand.NewPCG(23, 24)), 16)
	for _, v := range seq {
		f, ok := v.(float64)
		if !ok || f < 1.0 || f >= 5.0 {
			t.Fatalf("bounded CReal draw %v (%T) not in [1.0,5.0)", v, v)
		}
		if len(bounded.Validate(f)) != 0 {
			t.Fatalf("bounded CReal draw %v failed Validate", f)
		}
	}
	if distinct(seq) < 2 {
		t.Errorf("bounded CReal produced no variation: %v", seq)
	}

	listRange := constraints.CReal{
		List:  []float64{0.5, 2.5, 9.5},
		Range: constraints.NumericRange{Lower: 1.0, Upper: 5.0, LowerInclusive: true, UpperInclusive: true},
	}
	for _, v := range draws(listRange, newSampler(mrand.NewPCG(25, 26)), 8) {
		if v != 2.5 {
			t.Fatalf("want 2.5 (sole in-range list member), got %v", v)
		}
	}

	unbounded := constraints.CReal{}
	if distinct(draws(unbounded, newSampler(mrand.NewPCG(27, 28)), 16)) < 2 {
		t.Error("unbounded CReal produced no variation")
	}
}

// TestSampleValue_boolean covers the CBoolean arm: a single-valued
// constraint always yields that value; both-valid varies over enough
// draws; both-invalid falls back to the deterministic ExampleValue.
func TestSampleValue_boolean(t *testing.T) {
	trueOnly := constraints.CBoolean{TrueValid: true}
	for _, v := range draws(trueOnly, newSampler(mrand.NewPCG(29, 30)), 8) {
		if v != true {
			t.Fatalf("TrueValid-only must yield true, got %v", v)
		}
	}
	falseOnly := constraints.CBoolean{FalseValid: true}
	for _, v := range draws(falseOnly, newSampler(mrand.NewPCG(31, 32)), 8) {
		if v != false {
			t.Fatalf("FalseValid-only must yield false, got %v", v)
		}
	}
	both := constraints.CBoolean{TrueValid: true, FalseValid: true}
	if distinct(draws(both, newSampler(mrand.NewPCG(33, 34)), 24)) < 2 {
		t.Error("both-valid CBoolean should produce both true and false over 24 draws")
	}
	neither := constraints.CBoolean{}
	got := sampleValue(neither, newSampler(mrand.NewPCG(35, 36)))
	if got != neither.ExampleValue() {
		t.Errorf("both-invalid CBoolean: got %v, want ExampleValue %v", got, neither.ExampleValue())
	}
}

// TestSampleValue_enumerableDoesNotCollapse is the SDK-GAP-14 regression
// guard for the silent ExampleFill collapse: a multi-member enumerable
// constraint must actually draw values other than ExampleValue. A future
// filter/bounds bug that made the sampler always fall back would otherwise
// be invisible (output stays valid, just non-varying) — the end-to-end
// gap14 test would still pass on variation from a different leaf.
func TestSampleValue_enumerableDoesNotCollapse(t *testing.T) {
	c := constraints.CInteger{
		List:  []int64{10, 20, 30},
		Range: constraints.NumericRange{Lower: 1, Upper: 100, LowerInclusive: true, UpperInclusive: true},
	}
	ex := c.ExampleValue()
	seq := draws(c, newSampler(mrand.NewPCG(37, 38)), 24)
	sawNonExample := false
	for _, v := range seq {
		if v != ex {
			sawNonExample = true
			break
		}
	}
	if !sawNonExample {
		t.Errorf("RandomFill silently collapsed to ExampleValue %v for an enumerable constraint: %v", ex, seq)
	}
	if distinct(seq) < 2 {
		t.Errorf("enumerable constraint produced no variation: %v", seq)
	}
}

// TestSampleValue_quantityMagnitudeVaries pins the DvQuantity magnitude
// draw specifically (the gap14 end-to-end test only proves *some* leaf
// varies): across draws the in-range magnitude must take ≥2 distinct
// values, so a regression to a fixed example magnitude is caught here.
func TestSampleValue_quantityMagnitudeVaries(t *testing.T) {
	c := constraints.DvQuantity{Units: []constraints.QuantityUnit{{
		Units:     "mm[Hg]",
		Magnitude: constraints.NumericRange{Lower: 40, Upper: 200, LowerInclusive: true, UpperInclusive: true},
	}}}
	mags := map[float64]struct{}{}
	for _, v := range draws(c, newSampler(mrand.NewPCG(39, 40)), 16) {
		mags[v.(constraints.QuantityValue).Magnitude] = struct{}{}
	}
	if len(mags) < 2 {
		t.Errorf("DvQuantity magnitude did not vary across draws: %v", mags)
	}
}
