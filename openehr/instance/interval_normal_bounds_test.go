package instance_test

// interval_normal_bounds_test.go: the REQ-107 order of an interval's bounds
// where a bound has a normal_range and a normal_status of its own, so the
// order must not undo what the bound's own walk settled
// (Normal_range_and_status_consistency).

import (
	"fmt"
	"math/rand/v2"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// plainCount is a DV_COUNT whose magnitude the OPT constrains with
// magnitude, a C_INTEGER, and nothing else.
func plainCount(magnitude string) string {
	return optNode("DV_COUNT", "", optSingle("magnitude", magnitude))
}

// normalCount is a DV_COUNT, as plainCount is, with a normal_range from lo
// to hi, both included, and a normal_status the OPT pins to status.
func normalCount(magnitude, lo, hi, status string) string {
	return optNode("DV_COUNT", "", optSingle("magnitude", magnitude),
		normalRangeOver("DV_COUNT", countValue(lo), countValue(hi)), normalStatus(status))
}

// normalQuantity is a DV_QUANTITY in mm, as expandedQuantity is, with a
// normal_range from lo to hi mm, both included, and a normal_status the OPT
// pins to status.
func normalQuantity(magnitude, lo, hi, status string) string {
	return optNode("DV_QUANTITY", "", optSingle("magnitude", magnitude), optSingle("units", guardString("mm")),
		normalRangeOver("DV_QUANTITY", quantityValue(lo), quantityValue(hi)), normalStatus(status))
}

// normalOrdinal is a DV_ORDINAL with the symbol code, as expandedOrdinal is,
// with a normal_range from lo to hi, both included, and a normal_status the
// OPT pins to status.
func normalOrdinal(value, code, lo, hi, status string) string {
	return optNode("DV_ORDINAL", "", optSingle("value", value), optSingle("symbol", optCodedText("local", code)),
		normalRangeOver("DV_ORDINAL", ordinalBound(lo, code), ordinalBound(hi, code)), normalStatus(status))
}

// dataValueOf is a bound as the data value the checks read: a pointer to a
// copy of a bound held by value, and a bound held by pointer as it is.
func dataValueOf[T rm.DVOrdered](b T) rm.DataValue {
	switch v := any(b).(type) {
	case rm.DVCount:
		return &v
	case rm.DVQuantity:
		return &v
	case rm.DVOrdinal:
		return &v
	}
	return any(b).(rm.DataValue)
}

// normalBoundString renders a bound as normalString does, without the
// other_reference_ranges, which no OPT here constrains and which stay empty.
func normalBoundString(v rm.DataValue) string {
	return strings.TrimSuffix(normalString(v), "; other_reference_ranges []")
}

// normalBoundsString renders iv as intervalString does, then each bound as
// normalBoundString does, so a bound's normal_status and normal_range show.
func normalBoundsString[T rm.DVOrdered](iv *rm.DVInterval[T]) string {
	return intervalString(iv) + " | lower " + normalBoundString(dataValueOf(iv.Lower)) + " | upper " + normalBoundString(dataValueOf(iv.Upper))
}

// normalBoundsPair is one DV_INTERVAL whose lower and upper bounds are the
// OPT nodes lower and upper, where a bound carries a normal_range and a
// normal_status.
type normalBoundsPair struct {
	name         string
	lower, upper string
	// lowerRule and upperRule are set where that side's bound carries a
	// normal_range and a normal_status, which it must keep consistent.
	lowerRule, upperRule bool
	// want is the interval and its bounds as normalBoundsString renders them
	// under ExampleFill, and wantRandom under RandomFill where that differs.
	want, wantRandom string
	// ordered is set where an ordered pair keeps each bound's normal_range
	// rule and fits each side's OPT constraint, so every RandomFill draw
	// keeps the bounds in order. It is not set where no such pair exists,
	// which leaves the pair as generated.
	ordered bool
	// unordered is set where no such ordered pair exists, so the bounds stay
	// as generated and break DV_INTERVAL.Limits_consistent (REQ-107
	// § Exceptions and known gaps), which is the one error either validator
	// reports.
	unordered bool
}

// runNormalBounds generates a DV_INTERVAL<boundRM> from each case, under
// every policy, both value fills, and with and without the implicit
// attributes, checks the whole result against the case, that each bound
// keeps Normal_range_and_status_consistency, and the root against both
// validators, and, for an ordered case, sweeps RandomFill seeds for the
// order and the consistency. T is the Go type of a bound and position gives
// the value the order reads from one.
func runNormalBounds[T rm.DVOrdered](t *testing.T, boundRM string, cases []normalBoundsPair, position func(T) float64) {
	t.Helper()
	for _, tc := range cases {
		for _, implicit := range []bool{true, false} {
			c := compileOPTText(t, optTemplate("ELEMENT", optSingle("value", ordInterval(boundRM, tc.lower, tc.upper))), implicit)
			generate := func(t *testing.T, opts instance.Options) *rm.DVInterval[T] {
				t.Helper()
				out, err := instance.Generate(t.Context(), c, opts)
				if err != nil {
					t.Fatalf("Generate: %v, want a root", err)
				}
				el, ok := out.(*rm.Element)
				if !ok {
					t.Fatalf("generated root is %T, want *rm.Element", out)
				}
				iv, ok := el.Value.(*rm.DVInterval[T])
				if !ok {
					t.Fatalf("ELEMENT.value is %T, want *rm.DVInterval[%s]", el.Value, boundRM)
				}
				if tc.unordered {
					checkValidBut(t, out, c, ruleBroken("DV_INTERVAL.Limits_consistent"))
				} else {
					checkValid(t, out, c)
				}
				if tc.lowerRule {
					checkNormalConsistency(t, dataValueOf(iv.Lower))
				}
				if tc.upperRule {
					checkNormalConsistency(t, dataValueOf(iv.Upper))
				}
				return iv
			}
			for _, opts := range optionsFor(nil) {
				t.Run(fmt.Sprintf("%s/implicit=%t/%v/%v", tc.name, implicit, opts.Policy, opts.ValueFill), func(t *testing.T) {
					want := tc.want
					if opts.ValueFill == instance.RandomFill && tc.wantRandom != "" {
						want = tc.wantRandom
					}
					if got := normalBoundsString(generate(t, opts)); got != want {
						t.Errorf("ELEMENT.value:\n got %s\nwant %s", got, want)
					}
				})
			}
			if !tc.ordered {
				continue
			}
			t.Run(fmt.Sprintf("%s/implicit=%t/RandomFill seeds", tc.name, implicit), func(t *testing.T) {
				for seed := range uint64(orderSeeds) {
					opts := instance.Options{Policy: instance.Large, ValueFill: instance.RandomFill, ValueSource: rand.NewPCG(seed, 1), Now: defaultsNow}
					iv := generate(t, opts)
					if lo, hi := position(iv.Lower), position(iv.Upper); lo > hi {
						t.Errorf("seed %d: %s, lower above upper", seed, normalBoundsString(iv))
					}
				}
			})
		}
	}
}

// TestREQ107_IntervalOrderKeepsEachBoundsNormalRange is the REQ-107
// Intervals and Precedence 3 check that putting the bounds of an interval in
// order does not undo what a bound's own walk settled: a bound with a
// normal_range and a normal_status keeps DV_ORDERED
// Normal_range_and_status_consistency at every value the order gives it. A
// pair fits a side only where the bound it gives that side keeps the rule,
// which narrows a value that does not to the lowest, for the lower side, or
// the highest, for the upper side, that does and that the side's OPT
// constraint accepts. Where no ordered pair fits both sides the bounds stay
// as generated, which breaks DV_INTERVAL.Limits_consistent alone. It holds
// under every policy, both value fills, and with and without the implicit
// attributes, and the root passes both validators.
func TestREQ107_IntervalOrderKeepsEachBoundsNormalRange(t *testing.T) {
	runNormalBounds(t, "DV_COUNT", []normalBoundsPair{
		{
			// The generated lower bound is 5, inside its range, and the upper
			// bound 0. Swapped, the lower bound would be 0 with the status N
			// outside its range [5..9].
			name:  "lower bound 0..10 in the range [5..9], N, upper 0..10",
			lower: normalCount(integerFrom(0, 10), "5", "9", "N"), upper: plainCount(integerFrom(0, 10)),
			lowerRule: true,
			want: "[DV_COUNT 5 .. DV_COUNT 5] | " +
				"lower DV_COUNT 5; normal_status openehr_normal_statuses::N; normal_range [DV_COUNT 5 .. DV_COUNT 9] | " +
				"upper DV_COUNT 5; normal_status none; normal_range none",
			wantRandom: "[DV_COUNT 5 .. DV_COUNT 8] | " +
				"lower DV_COUNT 5; normal_status openehr_normal_statuses::N; normal_range [DV_COUNT 5 .. DV_COUNT 9] | " +
				"upper DV_COUNT 8; normal_status none; normal_range none",
			ordered: true,
		},
		{
			// Every value of the lower side that keeps its rule is 5 or more,
			// and every value of the upper side is 3 or less.
			name:  "lower bound 0..10 in the range [5..9], N, upper 0..3, no ordered pair",
			lower: normalCount(integerFrom(0, 10), "5", "9", "N"), upper: plainCount(integerFrom(0, 3)),
			lowerRule: true,
			want: "[DV_COUNT 5 .. DV_COUNT 0] | " +
				"lower DV_COUNT 5; normal_status openehr_normal_statuses::N; normal_range [DV_COUNT 5 .. DV_COUNT 9] | " +
				"upper DV_COUNT 0; normal_status none; normal_range none",
			wantRandom: "[DV_COUNT 8 .. DV_COUNT 0] | " +
				"lower DV_COUNT 8; normal_status openehr_normal_statuses::N; normal_range [DV_COUNT 5 .. DV_COUNT 9] | " +
				"upper DV_COUNT 0; normal_status none; normal_range none",
			unordered: true,
		},
		{
			// The upper bound's highest value is 10, outside its range [0..4]
			// where the status is N, so the highest value that keeps the rule
			// is 4, not the first candidate, 0.
			name:  "lower bound 3..10, upper 0..10 in the range [0..4], N",
			lower: plainCount(integerFrom(3, 10)), upper: normalCount(integerFrom(0, 10), "0", "4", "N"),
			upperRule: true,
			want: "[DV_COUNT 3 .. DV_COUNT 4] | " +
				"lower DV_COUNT 3; normal_status none; normal_range none | " +
				"upper DV_COUNT 4; normal_status openehr_normal_statuses::N; normal_range [DV_COUNT 0 .. DV_COUNT 4]",
			ordered: true,
		},
		{
			name:  "lower bound 5..10, upper 0..10 in the range [0..4], N, no ordered pair",
			lower: plainCount(integerFrom(5, 10)), upper: normalCount(integerFrom(0, 10), "0", "4", "N"),
			upperRule: true,
			want: "[DV_COUNT 5 .. DV_COUNT 0] | " +
				"lower DV_COUNT 5; normal_status none; normal_range none | " +
				"upper DV_COUNT 0; normal_status openehr_normal_statuses::N; normal_range [DV_COUNT 0 .. DV_COUNT 4]",
			wantRandom: "[DV_COUNT 9 .. DV_COUNT 0] | " +
				"lower DV_COUNT 9; normal_status none; normal_range none | " +
				"upper DV_COUNT 0; normal_status openehr_normal_statuses::N; normal_range [DV_COUNT 0 .. DV_COUNT 4]",
			unordered: true,
		},
		{
			// A status other than N keeps the bound outside its range: the
			// lower bound is 10, and a swap would hand it 0.
			name:  "lower bound 0..10 in the range [0..9], H, upper 0..10",
			lower: normalCount(integerFrom(0, 10), "0", "9", "H"), upper: plainCount(integerFrom(0, 10)),
			lowerRule: true,
			want: "[DV_COUNT 10 .. DV_COUNT 10] | " +
				"lower DV_COUNT 10; normal_status openehr_normal_statuses::H; normal_range [DV_COUNT 0 .. DV_COUNT 9] | " +
				"upper DV_COUNT 10; normal_status none; normal_range none",
			ordered: true,
		},
		{
			name:  "both bounds 0..10, in the ranges [6..9] and [5..9], N",
			lower: normalCount(integerFrom(0, 10), "6", "9", "N"), upper: normalCount(integerFrom(0, 10), "5", "9", "N"),
			lowerRule: true, upperRule: true,
			want: "[DV_COUNT 6 .. DV_COUNT 6] | " +
				"lower DV_COUNT 6; normal_status openehr_normal_statuses::N; normal_range [DV_COUNT 6 .. DV_COUNT 9] | " +
				"upper DV_COUNT 6; normal_status openehr_normal_statuses::N; normal_range [DV_COUNT 5 .. DV_COUNT 9]",
			wantRandom: "[DV_COUNT 6 .. DV_COUNT 8] | " +
				"lower DV_COUNT 6; normal_status openehr_normal_statuses::N; normal_range [DV_COUNT 6 .. DV_COUNT 9] | " +
				"upper DV_COUNT 8; normal_status openehr_normal_statuses::N; normal_range [DV_COUNT 5 .. DV_COUNT 9]",
			ordered: true,
		},
	}, func(c rm.DVCount) float64 { return float64(c.Magnitude) })
	// A DV_INTERVAL<DV_ORDERED> holds each bound by pointer, as a count here.
	runNormalBounds(t, "DV_ORDERED", []normalBoundsPair{
		{
			name:  "DV_ORDERED lower bound 0..10 in the range [5..9], N, upper 0..10",
			lower: normalCount(integerFrom(0, 10), "5", "9", "N"), upper: plainCount(integerFrom(0, 10)),
			lowerRule: true,
			want: "[*DV_COUNT 5 .. *DV_COUNT 5] | " +
				"lower DV_COUNT 5; normal_status openehr_normal_statuses::N; normal_range [DV_COUNT 5 .. DV_COUNT 9] | " +
				"upper DV_COUNT 5; normal_status none; normal_range none",
			wantRandom: "[*DV_COUNT 5 .. *DV_COUNT 8] | " +
				"lower DV_COUNT 5; normal_status openehr_normal_statuses::N; normal_range [DV_COUNT 5 .. DV_COUNT 9] | " +
				"upper DV_COUNT 8; normal_status none; normal_range none",
			ordered: true,
		},
		{
			name:  "DV_ORDERED lower bound 3..10, upper 0..10 in the range [0..4], N",
			lower: plainCount(integerFrom(3, 10)), upper: normalCount(integerFrom(0, 10), "0", "4", "N"),
			upperRule: true,
			want: "[*DV_COUNT 3 .. *DV_COUNT 4] | " +
				"lower DV_COUNT 3; normal_status none; normal_range none | " +
				"upper DV_COUNT 4; normal_status openehr_normal_statuses::N; normal_range [DV_COUNT 0 .. DV_COUNT 4]",
			ordered: true,
		},
		{
			name:  "DV_ORDERED lower bound 0..10 in the range [5..9], N, upper 0..3, no ordered pair",
			lower: normalCount(integerFrom(0, 10), "5", "9", "N"), upper: plainCount(integerFrom(0, 3)),
			lowerRule: true,
			want: "[*DV_COUNT 5 .. *DV_COUNT 0] | " +
				"lower DV_COUNT 5; normal_status openehr_normal_statuses::N; normal_range [DV_COUNT 5 .. DV_COUNT 9] | " +
				"upper DV_COUNT 0; normal_status none; normal_range none",
			wantRandom: "[*DV_COUNT 8 .. *DV_COUNT 0] | " +
				"lower DV_COUNT 8; normal_status openehr_normal_statuses::N; normal_range [DV_COUNT 5 .. DV_COUNT 9] | " +
				"upper DV_COUNT 0; normal_status none; normal_range none",
			unordered: true,
		},
	}, func(b rm.DVOrdered) float64 { pos, _, _ := normalPosition(b); return pos })
	runNormalBounds(t, "DV_QUANTITY", []normalBoundsPair{
		{
			name:  "quantity lower bound 0..10 mm in the range [5..9] mm, N, upper 0..10 mm",
			lower: normalQuantity(realFrom(0, 10), "5", "9", "N"), upper: expandedQuantity(realFrom(0, 10)),
			lowerRule: true,
			want: "[DV_QUANTITY 5 mm .. DV_QUANTITY 5 mm] | " +
				"lower DV_QUANTITY 5 mm; normal_status openehr_normal_statuses::N; normal_range [DV_QUANTITY 5 mm .. DV_QUANTITY 9 mm] | " +
				"upper DV_QUANTITY 5 mm; normal_status none; normal_range none",
			wantRandom: "[DV_QUANTITY 5 mm .. DV_QUANTITY 6.764556596678251 mm] | " +
				"lower DV_QUANTITY 5 mm; normal_status openehr_normal_statuses::N; normal_range [DV_QUANTITY 5 mm .. DV_QUANTITY 9 mm] | " +
				"upper DV_QUANTITY 6.764556596678251 mm; normal_status none; normal_range none",
			ordered: true,
		},
		{
			name:  "quantity lower bound 0..10 mm in the range [5..9] mm, N, upper 0..3 mm, no ordered pair",
			lower: normalQuantity(realFrom(0, 10), "5", "9", "N"), upper: expandedQuantity(realFrom(0, 3)),
			lowerRule: true,
			want: "[DV_QUANTITY 5 mm .. DV_QUANTITY 0 mm] | " +
				"lower DV_QUANTITY 5 mm; normal_status openehr_normal_statuses::N; normal_range [DV_QUANTITY 5 mm .. DV_QUANTITY 9 mm] | " +
				"upper DV_QUANTITY 0 mm; normal_status none; normal_range none",
			wantRandom: "[DV_QUANTITY 6.764556596678251 mm .. DV_QUANTITY 1.0145450855238398 mm] | " +
				"lower DV_QUANTITY 6.764556596678251 mm; normal_status openehr_normal_statuses::N; normal_range [DV_QUANTITY 5 mm .. DV_QUANTITY 9 mm] | " +
				"upper DV_QUANTITY 1.0145450855238398 mm; normal_status none; normal_range none",
			unordered: true,
		},
	}, func(q rm.DVQuantity) float64 { return float64(q.Magnitude) })
	runNormalBounds(t, "DV_ORDINAL", []normalBoundsPair{
		{
			name:  "ordinal lower bound 0..10 in the range [5..9], N, upper 0..10",
			lower: normalOrdinal(integerFrom(0, 10), "at0001", "5", "9", "N"), upper: expandedOrdinal(integerFrom(0, 10), "at0001"),
			lowerRule: true,
			want: "[DV_ORDINAL 5 local::at0001 .. DV_ORDINAL 5 local::at0001] | " +
				"lower DV_ORDINAL 5 local::at0001; normal_status openehr_normal_statuses::N; normal_range [*DV_ORDINAL 5 local::at0001 .. *DV_ORDINAL 9 local::at0001] | " +
				"upper DV_ORDINAL 5 local::at0001; normal_status none; normal_range none",
			wantRandom: "[DV_ORDINAL 5 local::at0001 .. DV_ORDINAL 8 local::at0001] | " +
				"lower DV_ORDINAL 5 local::at0001; normal_status openehr_normal_statuses::N; normal_range [*DV_ORDINAL 5 local::at0001 .. *DV_ORDINAL 9 local::at0001] | " +
				"upper DV_ORDINAL 8 local::at0001; normal_status none; normal_range none",
			ordered: true,
		},
		{
			name:  "ordinal lower bound 0..10 in the range [5..9], N, upper 0..3, no ordered pair",
			lower: normalOrdinal(integerFrom(0, 10), "at0001", "5", "9", "N"), upper: expandedOrdinal(integerFrom(0, 3), "at0001"),
			lowerRule: true,
			want: "[DV_ORDINAL 5 local::at0001 .. DV_ORDINAL 0 local::at0001] | " +
				"lower DV_ORDINAL 5 local::at0001; normal_status openehr_normal_statuses::N; normal_range [*DV_ORDINAL 5 local::at0001 .. *DV_ORDINAL 9 local::at0001] | " +
				"upper DV_ORDINAL 0 local::at0001; normal_status none; normal_range none",
			wantRandom: "[DV_ORDINAL 8 local::at0001 .. DV_ORDINAL 0 local::at0001] | " +
				"lower DV_ORDINAL 8 local::at0001; normal_status openehr_normal_statuses::N; normal_range [*DV_ORDINAL 5 local::at0001 .. *DV_ORDINAL 9 local::at0001] | " +
				"upper DV_ORDINAL 0 local::at0001; normal_status none; normal_range none",
			unordered: true,
		},
	}, func(o rm.DVOrdinal) float64 { return float64(o.Value) })
}

// nestedBounds is one ordered value whose normal_range has a bound that
// carries a normal_range and a normal_status of its own.
type nestedBounds struct {
	name  string
	value string // the ELEMENT's value node
	// bounds returns the range of the generated value as intervalString
	// renders it and the bounds the check reads, as the bound that carries a
	// rule.
	bounds func(t *testing.T, v rm.DataValue) (rangeText string, rule rm.DataValue, plain rm.DataValue)
	// want and wantRandom are the range, then each bound as
	// normalBoundString renders it, under ExampleFill and RandomFill.
	want, wantRandom string
}

// TestREQ107_RangesWhoseBoundsCarryTheirOwnNormalRange is the REQ-107
// Intervals and Precedence 3 check that the order of the bounds of a
// normal_range keeps the normal_range and the normal_status a bound
// carries, as the order of an interval of its own does
// (TestREQ107_IntervalOrderKeepsEachBoundsNormalRange): the OPT names the
// range as a DV_INTERVAL over the bounds' class, which its walk puts in
// order before the value takes it. A REFERENCE_RANGE's range has no such
// case: its bounds are simple in the RM (Range_is_simple), so they carry
// no normal_range. It holds under every policy, both value fills, and with
// and without the implicit attributes, and the root passes both
// validators.
func TestREQ107_RangesWhoseBoundsCarryTheirOwnNormalRange(t *testing.T) {
	countLower, countUpper := normalCount(integerFrom(0, 10), "5", "9", "N"), plainCount(integerFrom(0, 10))
	cases := []nestedBounds{
		{
			name: "DV_COUNT normal_range over DV_COUNT",
			value: optNode("DV_COUNT", "", optSingle("magnitude", integerFrom(0, 100)),
				normalRangeOver("DV_COUNT", countLower, countUpper)),
			bounds: func(t *testing.T, v rm.DataValue) (string, rm.DataValue, rm.DataValue) {
				t.Helper()
				c, ok := v.(*rm.DVCount)
				if !ok || c.NormalRange == nil {
					t.Fatalf("ELEMENT.value is %T, want a *rm.DVCount with a normal_range", v)
				}
				return intervalString(c.NormalRange), dataValueOf(c.NormalRange.Lower), dataValueOf(c.NormalRange.Upper)
			},
			want: "[DV_COUNT 5 .. DV_COUNT 5] | " +
				"lower DV_COUNT 5; normal_status openehr_normal_statuses::N; normal_range [DV_COUNT 5 .. DV_COUNT 9] | " +
				"upper DV_COUNT 5; normal_status none; normal_range none",
			wantRandom: "[DV_COUNT 5 .. DV_COUNT 6] | " +
				"lower DV_COUNT 5; normal_status openehr_normal_statuses::N; normal_range [DV_COUNT 5 .. DV_COUNT 9] | " +
				"upper DV_COUNT 6; normal_status none; normal_range none",
		},
		{
			name: "DV_ORDINAL normal_range over DV_ORDINAL",
			value: optNode("DV_ORDINAL", "", optSingle("value", integerFrom(0, 100)), optSingle("symbol", optCodedText("local", "at0001")),
				normalRangeOver("DV_ORDINAL", normalOrdinal(integerFrom(0, 10), "at0001", "5", "9", "N"), expandedOrdinal(integerFrom(0, 10), "at0001"))),
			bounds: func(t *testing.T, v rm.DataValue) (string, rm.DataValue, rm.DataValue) {
				t.Helper()
				o, ok := v.(*rm.DVOrdinal)
				if !ok || o.NormalRange == nil {
					t.Fatalf("ELEMENT.value is %T, want a *rm.DVOrdinal with a normal_range", v)
				}
				lower, upper := o.NormalRange.Lower.(*rm.DVOrdinal), o.NormalRange.Upper.(*rm.DVOrdinal)
				return intervalString(o.NormalRange), lower, upper
			},
			want: "[*DV_ORDINAL 5 local::at0001 .. *DV_ORDINAL 5 local::at0001] | " +
				"lower DV_ORDINAL 5 local::at0001; normal_status openehr_normal_statuses::N; normal_range [*DV_ORDINAL 5 local::at0001 .. *DV_ORDINAL 9 local::at0001] | " +
				"upper DV_ORDINAL 5 local::at0001; normal_status none; normal_range none",
			wantRandom: "[*DV_ORDINAL 5 local::at0001 .. *DV_ORDINAL 8 local::at0001] | " +
				"lower DV_ORDINAL 5 local::at0001; normal_status openehr_normal_statuses::N; normal_range [*DV_ORDINAL 5 local::at0001 .. *DV_ORDINAL 9 local::at0001] | " +
				"upper DV_ORDINAL 8 local::at0001; normal_status none; normal_range none",
		},
	}
	for _, tc := range cases {
		for _, implicit := range []bool{true, false} {
			c := compileOPTText(t, optTemplate("ELEMENT", optSingle("value", tc.value)), implicit)
			for _, opts := range optionsFor(nil) {
				t.Run(fmt.Sprintf("%s/implicit=%t/%v/%v", tc.name, implicit, opts.Policy, opts.ValueFill), func(t *testing.T) {
					out, err := instance.Generate(t.Context(), c, opts)
					if err != nil {
						t.Fatalf("Generate: %v, want a root", err)
					}
					el, ok := out.(*rm.Element)
					if !ok {
						t.Fatalf("generated root is %T, want *rm.Element", out)
					}
					checkValid(t, out, c)
					rangeText, rule, plain := tc.bounds(t, el.Value)
					got := rangeText + " | lower " + normalBoundString(rule) + " | upper " + normalBoundString(plain)
					want := tc.want
					if opts.ValueFill == instance.RandomFill && tc.wantRandom != "" {
						want = tc.wantRandom
					}
					if got != want {
						t.Errorf("range:\n got %s\nwant %s", got, want)
					}
					checkNormalConsistency(t, rule)
				})
			}
		}
	}
}
