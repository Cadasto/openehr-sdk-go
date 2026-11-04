package instance_test

import (
	"fmt"
	mrand "math/rand/v2"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// TestREQ107_RandomFillDrawsAOneSidedIntegerRangeFromTheSamplersWindow is the
// REQ-107 Primitive-leaf value fill and Precedence 3 check that a C_INTEGER
// bounded on one side alone, and narrowed to a precision from -1 up, draws
// from the 100 whole numbers next to the side it bounds, as the sampler
// draws from any one-sided range that needs no narrowing, and not across
// the whole of the range it was narrowed to: `precision >= -5` draws from
// -1 to 99. The draws vary between seeds. ExampleFill keeps the value on the
// side the range bounds: the least value of a range bounded below. It holds
// under every policy, and with and without the implicit attributes.
func TestREQ107_RandomFillDrawsAOneSidedIntegerRangeFromTheSamplersWindow(t *testing.T) {
	const seeds = 40
	precision := func(body string) string {
		return optNode("DV_QUANTITY", "", optSingle("units", guardString("mm")),
			optSingle("precision", optPrimitive("INTEGER", "C_INTEGER", body)))
	}
	readPrecision := func(t *testing.T, out any) int64 {
		t.Helper()
		p := dvValue[*rm.DVQuantity](t, out).Precision
		if p == nil {
			t.Fatalf("DV_QUANTITY.precision is unset, want the value the OPT constrains")
		}
		return int64(*p)
	}
	cases := []struct {
		name  string
		value string // the ELEMENT's value node
		read  func(t *testing.T, out any) int64
		// lo and hi are the window every RandomFill draw lies in, and
		// example is the value ExampleFill takes.
		lo, hi, example int64
	}{
		{
			name:  "DV_QUANTITY precision at least -5",
			value: precision(instance.IntRange(instance.Closed(-5), instance.NoEnd)),
			read:  readPrecision, lo: -1, hi: 99, example: -1,
		},
		{
			// A bound that is exclusive moves the window with it.
			name:  "DV_QUANTITY precision above -5, excluding it",
			value: precision(instance.IntRange(instance.Open(-5), instance.NoEnd)),
			read:  readPrecision, lo: -1, hi: 99, example: -1,
		},
	}
	for _, tc := range cases {
		for _, implicit := range []bool{true, false} {
			c := compileOPTText(t, optTemplate("ELEMENT", optSingle("value", tc.value)), implicit)
			for _, policy := range everyPolicy {
				t.Run(fmt.Sprintf("%s/implicit=%t/%v", tc.name, implicit, policy), func(t *testing.T) {
					out, err := instance.Generate(t.Context(), c, instance.Options{
						Policy: policy, ValueFill: instance.ExampleFill, Now: defaultsNow,
					})
					if err != nil {
						t.Fatalf("Generate under ExampleFill: %v, want a root", err)
					}
					if got := tc.read(t, out); got != tc.example {
						t.Errorf("ExampleFill value = %d, want %d", got, tc.example)
					}
					seen := map[int64]bool{}
					for seed := range uint64(seeds) {
						out, err := instance.Generate(t.Context(), c, instance.Options{
							Policy: policy, ValueFill: instance.RandomFill, ValueSource: mrand.NewPCG(seed, 2), Now: defaultsNow,
						})
						if err != nil {
							t.Fatalf("Generate under RandomFill, seed %d: %v, want a root", seed, err)
						}
						got := tc.read(t, out)
						if got < tc.lo || got > tc.hi {
							t.Fatalf("RandomFill value for seed %d = %d, want one from %d to %d", seed, got, tc.lo, tc.hi)
						}
						seen[got] = true
						noFloorErrors(t, out)
					}
					if len(seen) < 2 {
						t.Errorf("RandomFill gave %d distinct value(s) over %d seeds, want it to vary", len(seen), seeds)
					}
				})
			}
		}
	}
}

// TestREQ107_REQ056_OneSidedIntegerRangePastTheIntegerRangeIsRefusedAtRead is
// the REQ-107 and REQ-056 check that a C_INTEGER bounded on one side alone
// at a value past the 32-bit range of an RM Integer, on a DV_QUANTITY's
// precision or on a DV_ORDINAL's value, never reaches the sampler: reading
// the template is refused with an error wrapping canxml.ErrInvalidShape.
func TestREQ107_REQ056_OneSidedIntegerRangePastTheIntegerRangeIsRefusedAtRead(t *testing.T) {
	precision := func(body string) string {
		return optNode("DV_QUANTITY", "", optSingle("units", guardString("mm")),
			optSingle("precision", optPrimitive("INTEGER", "C_INTEGER", body)))
	}
	ordinal := func(body string) string { return optPrimitive("DV_ORDINAL", "C_INTEGER", body) }
	for _, tc := range []struct {
		name  string
		value string // the ELEMENT's value node
	}{
		{"DV_QUANTITY precision at most 5000000000", precision(intRangeBody("", "5000000000", true))},
		{"DV_ORDINAL value at most 5000000000", ordinal(intRangeBody("", "5000000000", true))},
		{"DV_ORDINAL value at least -5000000000", ordinal(intRangeBody("-5000000000", "", true))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requireReadRefused(t, optTemplate("ELEMENT", optSingle("value", tc.value)))
		})
	}
}
