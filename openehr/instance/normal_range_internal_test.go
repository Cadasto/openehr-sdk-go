package instance

import (
	"errors"
	"fmt"
	"math"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// normalStatusCode is a normal_status of code in the openEHR normal
// statuses code set.
func normalStatusCode(code string) *rm.CodePhrase {
	return &rm.CodePhrase{CodeString: code, TerminologyID: rm.TerminologyID{Value: "openehr_normal_statuses"}}
}

// closedRange is an interval that includes both its bounds.
func closedRange[T rm.DVOrdered](lower, upper T) *rm.DVInterval[T] {
	return &rm.DVInterval[T]{Lower: lower, Upper: upper, LowerIncluded: true, UpperIncluded: true}
}

// TestREQ107_NormalRangeSettleEdges pins how settleNormalRange treats the
// values the generator cannot reach through a synthetic OPT, with no OPT
// constraint on the value (a nil node): the temporal layouts it steps a
// bound in and those it does not, a range open on both sides, a pair that
// does not compare, a status that is not one of the normal statuses, the
// whole numerator a fraction needs, and the 32-bit range of an ordinal's
// value (§ Precedence 3, § Errors). Each row pins the value after the
// settle, or the error wrapping ErrConstraintUnsatisfiable where no
// candidate keeps the rule, in which case the value stays as it was.
func TestREQ107_NormalRangeSettleEdges(t *testing.T) {
	cases := []struct {
		name string
		v    any
		// value renders the settled value.
		value func(v any) string
		want  string
		// unsatisfiable is set where the settle is to fail.
		unsatisfiable bool
	}{
		{
			name: "DV_TIME H outside a whole day, which has every time",
			v: &rm.DVTime{
				Value: "12:00:00", NormalStatus: normalStatusCode("H"),
				NormalRange: closedRange[rm.DVOrdered](&rm.DVTime{Value: "00:00:00"}, &rm.DVTime{Value: "23:59:59"}),
			},
			value:         func(v any) string { return v.(*rm.DVTime).Value },
			want:          "12:00:00",
			unsatisfiable: true,
		},
		{
			name: "DV_TIME zoned, L, a second before the lower bound",
			v: &rm.DVTime{
				Value: "12:00:00+01:00", NormalStatus: normalStatusCode("L"),
				NormalRange: closedRange[rm.DVOrdered](&rm.DVTime{Value: "11:00:00Z"}, &rm.DVTime{Value: "11:30:00Z"}),
			},
			value: func(v any) string { return v.(*rm.DVTime).Value },
			want:  "10:59:59Z",
		},
		{
			name: "DV_DATE_TIME zoneless, N, the lower bound",
			v: &rm.DVDateTime{
				Value: "2021-03-05T00:00:00", NormalStatus: normalStatusCode("N"),
				NormalRange: closedRange[rm.DVOrdered](&rm.DVDateTime{Value: "2021-03-01T00:00:00"}, &rm.DVDateTime{Value: "2021-03-02T00:00:00"}),
			},
			value: func(v any) string { return v.(*rm.DVDateTime).Value },
			want:  "2021-03-01T00:00:00",
		},
		{
			name: "DV_DATE partial, H: a bound in a layout with no step gives only itself",
			v: &rm.DVDate{
				Value: "2020-01", NormalStatus: normalStatusCode("H"),
				NormalRange: closedRange[rm.DVOrdered](&rm.DVDate{Value: "2020-01"}, &rm.DVDate{Value: "2020-01"}),
			},
			value:         func(v any) string { return v.(*rm.DVDate).Value },
			want:          "2020-01",
			unsatisfiable: true,
		},
		{
			name: "DV_DATE_TIME zoned against zoneless bounds, H, left as generated",
			v: &rm.DVDateTime{
				Value: "2021-03-04T05:06:07Z", NormalStatus: normalStatusCode("H"),
				NormalRange: closedRange[rm.DVOrdered](&rm.DVDateTime{Value: "2021-03-01T00:00:00"}, &rm.DVDateTime{Value: "2021-03-09T00:00:00"}),
			},
			value: func(v any) string { return v.(*rm.DVDateTime).Value },
			want:  "2021-03-04T05:06:07Z",
		},
		{
			name: "DV_ORDINAL with DV_COUNT bounds, N, left as generated",
			v: &rm.DVOrdinal{
				Value: 7, NormalStatus: normalStatusCode("N"),
				NormalRange: closedRange[rm.DVOrdered](&rm.DVCount{Magnitude: 1}, &rm.DVCount{Magnitude: 3}),
			},
			value: func(v any) string { return fmt.Sprint(v.(*rm.DVOrdinal).Value) },
			want:  "7",
		},
		{
			name: "DV_COUNT H, range open on both sides, which has every value",
			v: &rm.DVCount{
				Magnitude: 5, NormalStatus: normalStatusCode("H"),
				NormalRange: &rm.DVInterval[rm.DVCount]{LowerUnbounded: true, UpperUnbounded: true},
			},
			value:         func(v any) string { return fmt.Sprint(v.(*rm.DVCount).Magnitude) },
			want:          "5",
			unsatisfiable: true,
		},
		{
			name: "DV_COUNT N, range open on both sides, kept",
			v: &rm.DVCount{
				Magnitude: 5, NormalStatus: normalStatusCode("N"),
				NormalRange: &rm.DVInterval[rm.DVCount]{LowerUnbounded: true, UpperUnbounded: true},
			},
			value: func(v any) string { return fmt.Sprint(v.(*rm.DVCount).Magnitude) },
			want:  "5",
		},
		{
			name: "DV_COUNT with a placeholder status, which is not N, one below the lower bound",
			v: &rm.DVCount{
				Magnitude: 5, NormalStatus: &rm.CodePhrase{CodeString: "at0000", TerminologyID: rm.TerminologyID{Value: "local"}},
				NormalRange: closedRange(rm.DVCount{Magnitude: 1}, rm.DVCount{Magnitude: 9}),
			},
			value: func(v any) string { return fmt.Sprint(v.(*rm.DVCount).Magnitude) },
			want:  "0",
		},
		{
			name: "DV_PROPORTION fraction over 3, N, the whole numerator next to the lower bound",
			v: &rm.DVProportion{
				Numerator: 0, Denominator: 3, Type: pkFraction, NormalStatus: normalStatusCode("N"),
				NormalRange: closedRange(rm.DVProportion{Numerator: 1, Denominator: 2, Type: pkFraction},
					rm.DVProportion{Numerator: 1, Denominator: 1, Type: pkFraction}),
			},
			value: func(v any) string { p := v.(*rm.DVProportion); return fmt.Sprintf("%v/%v", p.Numerator, p.Denominator) },
			want:  "2/3",
		},
		{
			// The upper bound's ratio is the greatest float64; on the
			// value's denominator its numerator overflows to infinity, which
			// is no number canonical JSON carries, and no finite numerator
			// over 3 lies above it.
			name: "DV_PROPORTION H, a bound whose ratio on the value's denominator overflows",
			v: &rm.DVProportion{
				Numerator: 0, Denominator: 3, Type: pkRatio, NormalStatus: normalStatusCode("H"),
				NormalRange: &rm.DVInterval[rm.DVProportion]{
					LowerUnbounded: true, UpperIncluded: true,
					Upper: rm.DVProportion{Numerator: math.MaxFloat64 / 2, Denominator: 0.5, Type: pkRatio},
				},
			},
			value:         func(v any) string { p := v.(*rm.DVProportion); return fmt.Sprintf("%v/%v", p.Numerator, p.Denominator) },
			want:          "0/3",
			unsatisfiable: true,
		},
		{
			name: "DV_ORDINAL H, range over every 32-bit Integer",
			v: &rm.DVOrdinal{
				Value: 5, NormalStatus: normalStatusCode("H"),
				NormalRange: closedRange[rm.DVOrdered](&rm.DVOrdinal{Value: math.MinInt32}, &rm.DVOrdinal{Value: math.MaxInt32}),
			},
			value:         func(v any) string { return fmt.Sprint(v.(*rm.DVOrdinal).Value) },
			want:          "5",
			unsatisfiable: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := settleNormalRange(nil, tc.v)
			switch {
			case tc.unsatisfiable && !errors.Is(err, ErrConstraintUnsatisfiable):
				t.Errorf("settleNormalRange: error %v, want one wrapping ErrConstraintUnsatisfiable", err)
			case !tc.unsatisfiable && err != nil:
				t.Errorf("settleNormalRange: error %v, want none", err)
			}
			if got := tc.value(tc.v); got != tc.want {
				t.Errorf("settleNormalRange: value %s, want %s", got, tc.want)
			}
		})
	}
}
