package instance_test

import (
	"fmt"
	"math"
	mrand "math/rand/v2"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// roundingSeeds is how many RandomFill seeds each rounding case runs.
const roundingSeeds = 40

// optQuantityValue is an ELEMENT template whose value is a C_DV_QUANTITY
// with the given list entries.
func optQuantityValue(entries ...string) string {
	body := ""
	for _, e := range entries {
		body += e
	}
	return optTemplate("ELEMENT", optSingle("value",
		`<children xsi:type="C_DV_QUANTITY"><rm_type_name>DV_QUANTITY</rm_type_name><node_id></node_id>`+body+`</children>`))
}

// quantityEntry is a C_DV_QUANTITY list entry in units whose magnitude
// element carries magnitude and whose precision element carries precision;
// "" leaves either out.
func quantityEntry(units, magnitude, precision string) string {
	out := "<list>"
	if magnitude != "" {
		out += "<magnitude>" + magnitude + "</magnitude>"
	}
	if precision != "" {
		out += "<precision>" + precision + "</precision>"
	}
	return out + "<units>" + units + "</units></list>"
}

// rangeOf is the body of an interval element from lo to hi.
func rangeOf(lo, hi instance.End) string {
	r := instance.RealRange(lo, hi)
	return r[len("<range>") : len(r)-len("</range>")]
}

// decimalsAtMost reports whether v has no more than n decimals.
func decimalsAtMost(v float64, n int) bool {
	scaled := v * math.Pow10(n)
	return math.Abs(scaled-math.Round(scaled)) < 1e-6
}

// eachRandomRun generates c under every policy with RandomFill, once per
// seed below roundingSeeds, and calls check with each root.
func eachRandomRun(t *testing.T, generate func(opts instance.Options) (any, error), check func(t *testing.T, what string, out any)) {
	t.Helper()
	for _, policy := range everyPolicy {
		for seed := range uint64(roundingSeeds) {
			opts := instance.Options{Policy: policy, ValueFill: instance.RandomFill, ValueSource: mrand.NewPCG(seed, corpusStream), Now: defaultsNow}
			what := fmt.Sprintf("%v/seed=%d", policy, seed)
			out, err := generate(opts)
			if err != nil {
				t.Fatalf("%s: Generate: %v", what, err)
			}
			check(t, what, out)
		}
	}
}

// TestREQ107_RandomQuantityIsRoundedInsideItsRange is the REQ-107
// Primitive-leaf value fill check on a DV_QUANTITY RandomFill draws: its
// magnitude is rounded to the template's precision, with no decimals where
// the entry's precision range admits 0 alone, one where it admits up to 1,
// and two otherwise, and is drawn from inside the entry's range, away from
// 0 and from the range's bounds, or from 1 to 100 where the range is open.
// The RM floor and the template validator pass.
func TestREQ107_RandomQuantityIsRoundedInsideItsRange(t *testing.T) {
	closed, noEnd := instance.Closed, instance.NoEnd
	cases := []struct {
		name     string
		entry    string
		decimals int
		lo, hi   float64 // the magnitude lies strictly between them
	}{
		{
			name:     "range 0..200 with precision 0",
			entry:    quantityEntry("/min", rangeOf(closed(0), closed(200)), rangeOf(closed(0), closed(0))),
			decimals: 0, lo: 0, hi: 200,
		},
		{
			// A whole number drawn here is often 0.
			name:     "range -5..5 with precision 0",
			entry:    quantityEntry("1", rangeOf(closed(-5), closed(5)), rangeOf(closed(0), closed(0))),
			decimals: 0, lo: -5, hi: 5,
		},
		{
			// A whole number drawn here is often a bound; 1 is the only one
			// strictly inside.
			name:     "range 0..2 with precision 0",
			entry:    quantityEntry("1", rangeOf(closed(0), closed(2)), rangeOf(closed(0), closed(0))),
			decimals: 0, lo: 0, hi: 2,
		},
		{
			name:     "range 0..10 with precision 0..1",
			entry:    quantityEntry("kg", rangeOf(closed(0), closed(10)), rangeOf(closed(0), closed(1))),
			decimals: 1, lo: 0, hi: 10,
		},
		{
			name:     "range 30..45 with no precision",
			entry:    quantityEntry("Cel", rangeOf(closed(30), closed(45)), ""),
			decimals: 2, lo: 30, hi: 45,
		},
		{
			name:     "range from 0 upward",
			entry:    quantityEntry("mm", rangeOf(closed(0), noEnd), ""),
			decimals: 2, lo: 0, hi: 100.000001,
		},
		{
			name:     "open range",
			entry:    quantityEntry("cm", "", ""),
			decimals: 2, lo: 0.999999, hi: 100.000001,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := compileOPTText(t, optQuantityValue(tc.entry), true)
			eachRandomRun(t, func(opts instance.Options) (any, error) {
				return instance.Generate(t.Context(), c, opts)
			}, func(t *testing.T, what string, out any) {
				var got []rm.DVQuantity
				visitRM(out, func(_ string, q rm.DVQuantity) { got = append(got, q) })
				if len(got) != 1 {
					t.Fatalf("%s: %d DV_QUANTITY generated, want 1", what, len(got))
				}
				m := float64(got[0].Magnitude)
				if !decimalsAtMost(m, tc.decimals) {
					t.Errorf("%s: DV_QUANTITY magnitude %v, want at most %d decimals", what, m, tc.decimals)
				}
				if m <= tc.lo || m >= tc.hi || m == 0 {
					t.Errorf("%s: DV_QUANTITY magnitude %v, want one strictly between %v and %v and not 0", what, m, tc.lo, tc.hi)
				}
				checkValid(t, out, c)
			})
		})
	}
}

// TestREQ107_RandomProportionIsRounded is the REQ-107 Primitive-leaf value
// fill check on a DV_PROPORTION RandomFill draws: its numerator and
// denominator are whole numbers where its precision or its kind asks for
// them, and are otherwise rounded as a quantity's magnitude is, to the
// decimals its precision gives up to two. The RM floor and the template
// validator pass.
func TestREQ107_RandomProportionIsRounded(t *testing.T) {
	r, closed := instance.RealRange, instance.Closed
	numerator := optProportionPart("numerator", r(closed(0), closed(10)))
	denominator := optProportionPart("denominator", r(closed(1), closed(10)))
	cases := []struct {
		name     string
		attrs    []string
		decimals int
	}{
		{
			name:     "ratio with no precision",
			attrs:    []string{numerator, denominator, optProportionPart("type", instance.IntList(0))},
			decimals: 2,
		},
		{
			name:     "ratio of precision 1",
			attrs:    []string{numerator, denominator, optProportionPart("type", instance.IntList(0)), optProportionPart("precision", instance.IntList(1))},
			decimals: 1,
		},
		{
			name:     "ratio of precision 0",
			attrs:    []string{numerator, denominator, optProportionPart("type", instance.IntList(0)), optProportionPart("precision", instance.IntList(0))},
			decimals: 0,
		},
		{
			name:     "ratio of precision 5",
			attrs:    []string{numerator, denominator, optProportionPart("type", instance.IntList(0)), optProportionPart("precision", instance.IntList(5))},
			decimals: 2,
		},
		{
			name:     "fraction",
			attrs:    []string{numerator, denominator, optProportionPart("type", instance.IntList(3))},
			decimals: 0,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := compileOPTText(t, optProportionOver(tc.attrs...), true)
			eachProportionRun(t, c, func(t *testing.T, what string, out any, p rm.DVProportion) {
				if !decimalsAtMost(float64(p.Numerator), tc.decimals) || !decimalsAtMost(float64(p.Denominator), tc.decimals) {
					t.Errorf("%s: DV_PROPORTION %s, want at most %d decimals on each operand", what, proportionText(p), tc.decimals)
				}
				checkValid(t, out, c)
			})
		})
	}
}
