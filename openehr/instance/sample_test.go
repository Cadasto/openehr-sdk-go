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
