package instance_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// The OPTs below put one ordered value under an ELEMENT with a normal_range
// and a normal_status, for the REQ-107 Precedence 3 rule DV_ORDERED
// Normal_range_and_status_consistency: a value whose normal_status is N
// lies inside its normal_range, and one with any other normal_status lies
// outside it, as BASE Interval.has reads the range. The value's own
// constraint leaves it open, so the example value or a RandomFill draw
// lands where that constraint puts it, which the rule then narrows.

// normalStatus is a normal_status the OPT pins to code, a code of the
// openEHR normal statuses code set.
func normalStatus(code string) string {
	return optSingle("normal_status", optCodePhrase("openehr_normal_statuses", code))
}

// normalRangeOver is a normal_range over a DV_INTERVAL<boundRM> whose lower
// and upper bounds are the OPT nodes lower and upper, an empty one leaving
// that side open, and which carries attrs, such as excludesLower.
func normalRangeOver(boundRM, lower, upper string, attrs ...string) string {
	var all []string
	if lower != "" {
		all = append(all, optSingle("lower", lower))
	}
	if upper != "" {
		all = append(all, optSingle("upper", upper))
	}
	return optSingle("normal_range", optNode("DV_INTERVAL&lt;"+boundRM+"&gt;", "", append(all, attrs...)...))
}

// excludesLower and excludesUpper are a C_BOOLEAN on an interval's
// lower_included or upper_included that admits false alone, so the
// interval does not include that bound.
var (
	excludesLower = optSingle("lower_included", dvOnlyFalse)
	excludesUpper = optSingle("upper_included", dvOnlyFalse)
)

// integerFrom and realFrom are a C_INTEGER and a C_REAL admitting the
// closed range from lo to hi.
func integerFrom(lo, hi float64) string {
	return optPrimitive("INTEGER", "C_INTEGER", instance.IntRange(instance.Closed(lo), instance.Closed(hi)))
}

func realFrom(lo, hi float64) string {
	return optPrimitive("REAL", "C_REAL", instance.RealRange(instance.Closed(lo), instance.Closed(hi)))
}

// normalString renders an ordered value as the checks below compare it:
// its own value, as boundString renders a bound, then its DV_ORDERED
// attributes, as orderedString renders them.
func normalString(v rm.DataValue) string {
	return strings.TrimPrefix(boundString(v), "*") + "; " + orderedString(v)
}

// normalPosition returns where v, an ordered value or a bound, lies on
// the line the checks below order it on: a count's magnitude, a
// quantity's magnitude, a proportion's ratio, an ordinal's value, and a
// date's, a time's or a date-time's instant in seconds. A quantity also
// returns its units, which must match for two positions to compare. It
// returns false for any other value.
func normalPosition(v any) (pos float64, units string, ok bool) {
	instant := func(s string, layouts ...string) (float64, string, bool) {
		for _, layout := range layouts {
			if t, err := time.Parse(layout, s); err == nil {
				return float64(t.Unix()), "", true
			}
		}
		return 0, "", false
	}
	switch x := v.(type) {
	case rm.DVCount:
		return float64(x.Magnitude), "", true
	case rm.DVQuantity:
		return float64(x.Magnitude), x.Units, true
	case rm.DVProportion:
		return float64(x.Numerator) / float64(x.Denominator), "", x.Denominator != 0
	case rm.DVOrdinal:
		return float64(x.Value), "", true
	case rm.DVDate:
		return instant(x.Value, "2006-01-02")
	case rm.DVTime:
		return instant(x.Value, "15:04:05Z07:00", "15:04:05")
	case rm.DVDateTime:
		return instant(x.Value, time.RFC3339, "2006-01-02T15:04:05")
	case *rm.DVCount:
		return normalPosition(*x)
	case *rm.DVQuantity:
		return normalPosition(*x)
	case *rm.DVProportion:
		return normalPosition(*x)
	case *rm.DVOrdinal:
		return normalPosition(*x)
	case *rm.DVDate:
		return normalPosition(*x)
	case *rm.DVTime:
		return normalPosition(*x)
	case *rm.DVDateTime:
		return normalPosition(*x)
	}
	return 0, "", false
}

// normalFacts are what DV_ORDERED Normal_range_and_status_consistency
// reads of one ordered value.
type normalFacts struct {
	status string
	// has is whether the normal_range has the value, as BASE Interval.has
	// reads it.
	has bool
}

// normalFactsOf returns the facts of v, and false when v has no
// normal_range or no normal_status, or when its value and a bound of the
// range do not lie on one line (normalPosition).
func normalFactsOf(v rm.DataValue) (normalFacts, bool) {
	switch o := v.(type) {
	case *rm.DVCount:
		return rangeFacts(o, o.NormalStatus, o.NormalRange)
	case *rm.DVQuantity:
		return rangeFacts(o, o.NormalStatus, o.NormalRange)
	case *rm.DVProportion:
		return rangeFacts(o, o.NormalStatus, o.NormalRange)
	case *rm.DVOrdinal:
		return rangeFacts(o, o.NormalStatus, o.NormalRange)
	case *rm.DVDate:
		return rangeFacts(o, o.NormalStatus, o.NormalRange)
	case *rm.DVTime:
		return rangeFacts(o, o.NormalStatus, o.NormalRange)
	case *rm.DVDateTime:
		return rangeFacts(o, o.NormalStatus, o.NormalRange)
	}
	return normalFacts{}, false
}

func rangeFacts[T rm.DVOrdered](v any, status *rm.CodePhrase, iv *rm.DVInterval[T]) (normalFacts, bool) {
	if status == nil || iv == nil {
		return normalFacts{}, false
	}
	pos, units, ok := normalPosition(v)
	if !ok {
		return normalFacts{}, false
	}
	has := true
	side := func(bound T, included bool, above bool) bool {
		b, bUnits, ok := normalPosition(bound)
		if !ok || bUnits != units {
			return false
		}
		if above {
			has = has && (pos > b || (included && pos == b))
		} else {
			has = has && (pos < b || (included && pos == b))
		}
		return true
	}
	if !iv.LowerUnbounded && !side(iv.Lower, iv.LowerIncluded, true) {
		return normalFacts{}, false
	}
	if !iv.UpperUnbounded && !side(iv.Upper, iv.UpperIncluded, false) {
		return normalFacts{}, false
	}
	return normalFacts{status: status.CodeString, has: has}, true
}

// checkNormalConsistency fails t unless v keeps DV_ORDERED
// Normal_range_and_status_consistency: its normal_status is N exactly when
// its normal_range has its value.
func checkNormalConsistency(t *testing.T, v rm.DataValue) {
	t.Helper()
	facts, ok := normalFactsOf(v)
	if !ok {
		t.Fatalf("%s: the value and its normal_range do not compare, want a pair the check can read", normalString(v))
	}
	if (facts.status == "N") != facts.has {
		t.Errorf("%s: normal_status %s with normal_range.has(value) = %t breaks Normal_range_and_status_consistency",
			normalString(v), facts.status, facts.has)
	}
}

// normalCase is one ordered value the checks below generate.
type normalCase struct {
	name  string
	value string // the ELEMENT's value node
	// want is the value as normalString renders it under ExampleFill, and
	// wantRandom under RandomFill where that differs.
	want, wantRandom string
	// asGenerated is set where the value and the bounds of its range are
	// no pair the generator can compare (REQ-107 § Contract Intervals), so
	// it is left as generated and the rule is not checked.
	asGenerated bool
}

// TestREQ107_NormalStatusKeepsTheValueOnItsSideOfTheNormalRange is the
// REQ-107 Precedence 3 check that an ordered value with a normal_range and
// a normal_status keeps DV_ORDERED Normal_range_and_status_consistency
// wherever the value and the range's bounds are a pair § Contract Intervals
// says the generator can compare: two DV_COUNT, two DV_QUANTITY with equal
// units, two DV_PROPORTION of one type by ratio, two DV_ORDINAL by value,
// and two dates, times or date-times. A status N puts the value inside the
// range and any other status outside it, reading the range's included
// flags; a value that already keeps the rule stays, a RandomFill draw
// included. A value narrowed takes the first value, in the order
// settleNormalRange documents, that its own OPT constraint accepts, and
// keeps the rest of the value: a quantity's units, a proportion's type and
// denominator, an ordinal's symbol. Any other pair, here a quantity
// against bounds in other units, a DV_SCALE and a DV_DURATION, is left as
// generated, and so is a value whose range, open on both sides, has no
// bound to pair it with. It holds under every policy, both value fills, and with and
// without the implicit attributes, and the root passes both validators.
func TestREQ107_NormalStatusKeepsTheValueOnItsSideOfTheNormalRange(t *testing.T) {
	const noRefs = "; other_reference_ranges []"
	cases := []normalCase{
		{
			name: "DV_COUNT magnitude 0..100, range [1..9], N",
			value: optNode("DV_COUNT", "", optSingle("magnitude", integerFrom(0, 100)),
				normalRangeOver("DV_COUNT", countValue("1"), countValue("9")), normalStatus("N")),
			want: "DV_COUNT 1; normal_status openehr_normal_statuses::N; normal_range [DV_COUNT 1 .. DV_COUNT 9]" + noRefs,
		},
		{
			name:  "DV_COUNT with no magnitude constraint, range [1..9], N",
			value: optNode("DV_COUNT", "", normalRangeOver("DV_COUNT", countValue("1"), countValue("9")), normalStatus("N")),
			want:  "DV_COUNT 1; normal_status openehr_normal_statuses::N; normal_range [DV_COUNT 1 .. DV_COUNT 9]" + noRefs,
		},
		{
			name: "DV_COUNT magnitude 0..100, range (1..9], N",
			value: optNode("DV_COUNT", "", optSingle("magnitude", integerFrom(0, 100)),
				normalRangeOver("DV_COUNT", countValue("1"), countValue("9"), excludesLower), normalStatus("N")),
			want: "DV_COUNT 2; normal_status openehr_normal_statuses::N; normal_range (DV_COUNT 1 .. DV_COUNT 9]" + noRefs,
		},
		{
			name: "DV_COUNT magnitude 0..100, range [0..9], H",
			value: optNode("DV_COUNT", "", optSingle("magnitude", integerFrom(0, 100)),
				normalRangeOver("DV_COUNT", countValue("0"), countValue("9")), normalStatus("H")),
			want:       "DV_COUNT 10; normal_status openehr_normal_statuses::H; normal_range [DV_COUNT 0 .. DV_COUNT 9]" + noRefs,
			wantRandom: "DV_COUNT 77; normal_status openehr_normal_statuses::H; normal_range [DV_COUNT 0 .. DV_COUNT 9]" + noRefs,
		},
		{
			name: "DV_QUANTITY magnitude 0..100 mm, range [1 mm..9 mm], N",
			value: optNode("DV_QUANTITY", "", optSingle("magnitude", realFrom(0, 100)), optSingle("units", guardString("mm")),
				normalRangeOver("DV_QUANTITY", quantityValue("1"), quantityValue("9")), normalStatus("N")),
			want: "DV_QUANTITY 1 mm; normal_status openehr_normal_statuses::N; normal_range [DV_QUANTITY 1 mm .. DV_QUANTITY 9 mm]" + noRefs,
		},
		{
			name: "DV_PROPORTION unitary numerator 0..100, range [1/1..9/1], N",
			value: optNode("DV_PROPORTION", "", optSingle("numerator", realFrom(0, 100)),
				optSingle("denominator", dvReal("1")), optSingle("type", dvInteger("1")),
				normalRangeOver("DV_PROPORTION", proportionValue("1"), proportionValue("9")), normalStatus("N")),
			want: "DV_PROPORTION 1/1 type 1; normal_status openehr_normal_statuses::N; " +
				"normal_range [DV_PROPORTION 1/1 type 1 .. DV_PROPORTION 9/1 type 1]" + noRefs,
		},
		{
			name: "DV_PROPORTION unitary numerator 0..100, range (1/1..2/1), N",
			value: optNode("DV_PROPORTION", "", optSingle("numerator", realFrom(0, 100)),
				optSingle("denominator", dvReal("1")), optSingle("type", dvInteger("1")),
				normalRangeOver("DV_PROPORTION", proportionValue("1"), proportionValue("2"), excludesLower, excludesUpper),
				normalStatus("N")),
			want: "DV_PROPORTION 1.5/1 type 1; normal_status openehr_normal_statuses::N; " +
				"normal_range (DV_PROPORTION 1/1 type 1 .. DV_PROPORTION 2/1 type 1)" + noRefs,
		},
		{
			name: "DV_ORDINAL value 1..3 with a symbol of its own, range [2..3], N",
			value: optNode("DV_ORDINAL", "", optSingle("value", integerFrom(1, 3)),
				optSingle("symbol", optCodedText("local", "at0001", "at0002", "at0003")),
				normalRangeOver("DV_ORDINAL", ordinalBound("2", "at0002"), ordinalBound("3", "at0003")), normalStatus("N")),
			want: "DV_ORDINAL 2 local::at0001; normal_status openehr_normal_statuses::N; " +
				"normal_range [*DV_ORDINAL 2 local::at0002 .. *DV_ORDINAL 3 local::at0003]" + noRefs,
			wantRandom: "DV_ORDINAL 3 local::at0002; normal_status openehr_normal_statuses::N; " +
				"normal_range [*DV_ORDINAL 2 local::at0002 .. *DV_ORDINAL 3 local::at0003]" + noRefs,
		},
		{
			name: "DV_DATE with a C_DATE, range [2020-01-01..2020-01-01], N",
			value: optNode("DV_DATE", "", optSingle("value", optPrimitive("DATE", "C_DATE", "<pattern>YYYY-MM-DD</pattern>")),
				normalRangeOver("DV_DATE", optNode("DV_DATE", ""), optNode("DV_DATE", "")), normalStatus("N")),
			want: "DV_DATE 2020-01-01; normal_status openehr_normal_statuses::N; " +
				"normal_range [*DV_DATE 2020-01-01 .. *DV_DATE 2020-01-01]" + noRefs,
		},
		{
			name:  "DV_DATE range [2020-01-01..2020-01-01], H",
			value: optNode("DV_DATE", "", normalRangeOver("DV_DATE", optNode("DV_DATE", ""), optNode("DV_DATE", "")), normalStatus("H")),
			want: "DV_DATE 2019-12-31; normal_status openehr_normal_statuses::H; " +
				"normal_range [*DV_DATE 2020-01-01 .. *DV_DATE 2020-01-01]" + noRefs,
		},
		{
			name:  "DV_TIME range (12:00:00..open), N",
			value: optNode("DV_TIME", "", normalRangeOver("DV_TIME", optNode("DV_TIME", ""), "", excludesLower), normalStatus("N")),
			want:  "DV_TIME 12:00:01; normal_status openehr_normal_statuses::N; normal_range (*DV_TIME 12:00:00 .. open)" + noRefs,
		},
		{
			name: "DV_DATE_TIME range [now..now], L",
			value: optNode("DV_DATE_TIME", "",
				normalRangeOver("DV_DATE_TIME", optNode("DV_DATE_TIME", ""), optNode("DV_DATE_TIME", "")), normalStatus("L")),
			want: "DV_DATE_TIME 2021-03-04T05:06:06Z; normal_status openehr_normal_statuses::L; " +
				"normal_range [*DV_DATE_TIME 2021-03-04T05:06:07Z .. *DV_DATE_TIME 2021-03-04T05:06:07Z]" + noRefs,
		},
		{
			// The bounds are the OPT's own C_REAL on each magnitude, which
			// the swapped pair 0..5 would break: the lower side admits 5..10.
			// Of the candidates 5, 4, 6, 20, 19 and 21, only 4 is outside the
			// range and below the value's own 6..30, so 21 is the first.
			name: "DV_QUANTITY magnitude 6..30 mm, range of bounds built per attribute 5..10 and 0..20, H",
			value: optNode("DV_QUANTITY", "", optSingle("magnitude", realFrom(6, 30)), optSingle("units", guardString("mm")),
				normalRangeOver("DV_QUANTITY", expandedQuantity(realFrom(5, 10)), expandedQuantity(realFrom(0, 20))), normalStatus("H")),
			want: "DV_QUANTITY 21 mm; normal_status openehr_normal_statuses::H; normal_range [DV_QUANTITY 5 mm .. DV_QUANTITY 20 mm]" + noRefs,
			wantRandom: "DV_QUANTITY 22.2349358320278 mm; normal_status openehr_normal_statuses::H; " +
				"normal_range [DV_QUANTITY 7.542736988380132 mm .. DV_QUANTITY 15.956046987772261 mm]" + noRefs,
		},
		{
			// As above for a DV_ORDINAL: the swapped pair would hand the lower
			// side 0 and the upper side's symbol.
			name: "DV_ORDINAL value 6..30, range of bounds built per attribute 5..10 and 0..20, H",
			value: optNode("DV_ORDINAL", "", optSingle("value", integerFrom(6, 30)),
				optSingle("symbol", optCodedText("local", "at0003")),
				normalRangeOver("DV_ORDINAL", expandedOrdinal(integerFrom(5, 10), "at0001"), expandedOrdinal(integerFrom(0, 20), "at0002")),
				normalStatus("H")),
			want: "DV_ORDINAL 21 local::at0003; normal_status openehr_normal_statuses::H; " +
				"normal_range [*DV_ORDINAL 5 local::at0001 .. *DV_ORDINAL 20 local::at0002]" + noRefs,
			wantRandom: "DV_ORDINAL 25 local::at0003; normal_status openehr_normal_statuses::H; " +
				"normal_range [*DV_ORDINAL 5 local::at0001 .. *DV_ORDINAL 20 local::at0002]" + noRefs,
		},
		{
			name: "DV_QUANTITY in mm, range in cm, N, left as generated",
			value: optNode("DV_QUANTITY", "", optSingle("magnitude", realFrom(0, 100)), optSingle("units", guardString("mm")),
				normalRangeOver("DV_QUANTITY",
					optNode("DV_QUANTITY", "", optSingle("magnitude", dvReal("1")), optSingle("units", guardString("cm"))),
					optNode("DV_QUANTITY", "", optSingle("magnitude", dvReal("9")), optSingle("units", guardString("cm")))),
				normalStatus("N")),
			want:        "DV_QUANTITY 0 mm; normal_status openehr_normal_statuses::N; normal_range [DV_QUANTITY 1 cm .. DV_QUANTITY 9 cm]" + noRefs,
			wantRandom:  "DV_QUANTITY 67.6455659667825 mm; normal_status openehr_normal_statuses::N; normal_range [DV_QUANTITY 1 cm .. DV_QUANTITY 9 cm]" + noRefs,
			asGenerated: true,
		},
		{
			name:        "DV_COUNT range named with no child, H, left as generated",
			value:       optNode("DV_COUNT", "", optSingle("normal_range"), normalStatus("H")),
			want:        "DV_COUNT 0; normal_status openehr_normal_statuses::H; normal_range (open .. open)" + noRefs,
			asGenerated: true,
		},
		{
			name: "DV_SCALE 0.5, range [1..3], N, left as generated",
			value: scaleValue("0.5", "at0001",
				normalRangeOver("DV_SCALE", scaleValue("1", "at0002"), scaleValue("3", "at0003")), normalStatus("N")),
			want: "DV_SCALE 0.5 local::at0001; normal_status openehr_normal_statuses::N; " +
				"normal_range [*DV_SCALE 1 local::at0002 .. *DV_SCALE 3 local::at0003]" + noRefs,
			asGenerated: true,
		},
		{
			name: "DV_DURATION range [P0D..P0D], H, left as generated",
			value: optNode("DV_DURATION", "",
				normalRangeOver("DV_DURATION", optNode("DV_DURATION", ""), optNode("DV_DURATION", "")), normalStatus("H")),
			want: "DV_DURATION P0D; normal_status openehr_normal_statuses::H; " +
				"normal_range [*DV_DURATION P0D .. *DV_DURATION P0D]" + noRefs,
			asGenerated: true,
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
					want := tc.want
					if opts.ValueFill == instance.RandomFill && tc.wantRandom != "" {
						want = tc.wantRandom
					}
					if got := normalString(el.Value); got != want {
						t.Errorf("ELEMENT.value:\n got %s\nwant %s", got, want)
					}
					if !tc.asGenerated {
						checkNormalConsistency(t, el.Value)
					}
					checkValid(t, out, c)
				})
			}
		}
	}
}

// TestREQ107_NormalRangeWithNoConsistentValueIsUnsatisfiable is the REQ-107
// Errors check that an ordered value whose own OPT constraint admits no
// value the generator can derive on the side of its normal_range its
// normal_status needs, inside for N and outside for any other status,
// makes Generate return an error wrapping ErrConstraintUnsatisfiable that
// names the RM type, the attribute and the OPT path, and no root, under
// every policy, both value fills, and with and without the implicit
// attributes. The error wraps neither ErrSlotFillUnsupported nor
// ErrArchetypeIDMissing.
func TestREQ107_NormalRangeWithNoConsistentValueIsUnsatisfiable(t *testing.T) {
	cases := []struct {
		name  string
		value string
		// want is the part of the error that names the RM type, the
		// attribute and the OPT path.
		want string
	}{
		{
			name: "DV_COUNT magnitude [5], range [1..9], H",
			value: optNode("DV_COUNT", "", optSingle("magnitude", dvInteger("5")),
				normalRangeOver("DV_COUNT", countValue("1"), countValue("9")), normalStatus("H")),
			want: "DV_COUNT.magnitude at /value/magnitude",
		},
		{
			name: "DV_COUNT magnitude 20..30, range [1..9], N",
			value: optNode("DV_COUNT", "", optSingle("magnitude", integerFrom(20, 30)),
				normalRangeOver("DV_COUNT", countValue("1"), countValue("9")), normalStatus("N")),
			want: "DV_COUNT.magnitude at /value/magnitude",
		},
		{
			name: "DV_ORDINAL value [2], range [1..3], L",
			value: optNode("DV_ORDINAL", "", optSingle("value", dvInteger("2")), optSingle("symbol", optCodedText("local", "at0002")),
				normalRangeOver("DV_ORDINAL", ordinalBound("1", "at0001"), ordinalBound("3", "at0003")), normalStatus("L")),
			want: "DV_ORDINAL.value at /value/value",
		},
	}
	for _, tc := range cases {
		for _, implicit := range []bool{true, false} {
			c := compileOPTText(t, optTemplate("ELEMENT", optSingle("value", tc.value)), implicit)
			for _, opts := range optionsFor(nil) {
				t.Run(fmt.Sprintf("%s/implicit=%t/%v/%v", tc.name, implicit, opts.Policy, opts.ValueFill), func(t *testing.T) {
					out, err := instance.Generate(t.Context(), c, opts)
					if !errors.Is(err, instance.ErrConstraintUnsatisfiable) {
						t.Fatalf("Generate: error %v, want one wrapping ErrConstraintUnsatisfiable", err)
					}
					if errors.Is(err, instance.ErrSlotFillUnsupported) || errors.Is(err, instance.ErrArchetypeIDMissing) {
						t.Errorf("Generate: error %v wraps ErrSlotFillUnsupported or ErrArchetypeIDMissing", err)
					}
					if !strings.Contains(err.Error(), tc.want) {
						t.Errorf("Generate: error %q, want it to name %q", err, tc.want)
					}
					if out != nil {
						t.Errorf("Generate: root %T alongside the error, want none", out)
					}
				})
			}
		}
	}
}
