package template_test

import (
	"math"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/template"
)

// TestREQ111_NewMultiplicityClosesAndRefuses pins NewMultiplicity, the
// constructor the compile step closes the model's occurrences, existence and
// cardinality intervals with: each bounded side closed as the parser closes
// it, and the intervals the parser refuses refused.
func TestREQ111_NewMultiplicityClosesAndRefuses(t *testing.T) {
	type bounds struct {
		lower, upper                   int
		lowerIncluded, upperIncluded   bool
		lowerUnbounded, upperUnbounded bool
	}
	closed := []struct {
		name                           string
		in                             bounds
		lower, upper                   int
		lowerUnbounded, upperUnbounded bool
	}{
		{name: "closed 0..1", in: bounds{lower: 0, upper: 1, lowerIncluded: true, upperIncluded: true}, lower: 0, upper: 1},
		{name: "excluded ends >0..<3", in: bounds{lower: 0, upper: 3}, lower: 1, upper: 2},
		{name: "open above 1..*", in: bounds{lower: 1, lowerIncluded: true, upperUnbounded: true}, lower: 1, upperUnbounded: true},
		{name: "open below, its included flag moves nothing", in: bounds{upper: 4, lowerIncluded: false, upperIncluded: true, lowerUnbounded: true}, upper: 4, lowerUnbounded: true},
		{name: "one item 1..1", in: bounds{lower: 1, upper: 1, lowerIncluded: true, upperIncluded: true}, lower: 1, upper: 1},
	}
	for _, tc := range closed {
		t.Run(tc.name, func(t *testing.T) {
			in := tc.in
			m, err := template.NewMultiplicity(in.lower, in.upper, in.lowerIncluded, in.upperIncluded, in.lowerUnbounded, in.upperUnbounded)
			if err != nil {
				t.Fatalf("NewMultiplicity(%+v) = %v; want no error", in, err)
			}
			if m == nil {
				t.Fatalf("NewMultiplicity(%+v) = nil; want a multiplicity", in)
			}
			if m.LowerUnbounded() != tc.lowerUnbounded || m.UpperUnbounded() != tc.upperUnbounded {
				t.Errorf("NewMultiplicity(%+v) unbounded = %t, %t; want %t, %t",
					in, m.LowerUnbounded(), m.UpperUnbounded(), tc.lowerUnbounded, tc.upperUnbounded)
			}
			if !tc.lowerUnbounded && m.Lower() != tc.lower {
				t.Errorf("NewMultiplicity(%+v).Lower() = %d; want %d", in, m.Lower(), tc.lower)
			}
			if !tc.upperUnbounded && m.Upper() != tc.upper {
				t.Errorf("NewMultiplicity(%+v).Upper() = %d; want %d", in, m.Upper(), tc.upper)
			}
		})
	}

	refused := []struct {
		name    string
		in      bounds
		wantMsg string
	}{
		{name: "inverted", in: bounds{lower: 5, upper: 2, lowerIncluded: true, upperIncluded: true}, wantMsg: "inverted interval lower=5 > upper=2 (written 5..2)"},
		{name: "empty once closed", in: bounds{lower: 2, upper: 3}, wantMsg: "inverted interval lower=3 > upper=2 (written >2..<3)"},
		{name: "excluded lower past the largest int", in: bounds{lower: math.MaxInt, upperUnbounded: true}, wantMsg: "closed lower bound overflows int"},
		{name: "excluded upper below the smallest int", in: bounds{upper: math.MinInt, lowerUnbounded: true}, wantMsg: "closed upper bound overflows int"},
	}
	for _, tc := range refused {
		t.Run(tc.name, func(t *testing.T) {
			in := tc.in
			m, err := template.NewMultiplicity(in.lower, in.upper, in.lowerIncluded, in.upperIncluded, in.lowerUnbounded, in.upperUnbounded)
			if err == nil {
				t.Fatalf("NewMultiplicity(%+v) = %+v, nil; want an error containing %q", in, m, tc.wantMsg)
			}
			if m != nil {
				t.Errorf("NewMultiplicity(%+v) returned %+v beside its error; want nil", in, m)
			}
			if !strings.Contains(err.Error(), tc.wantMsg) {
				t.Errorf("NewMultiplicity(%+v) error = %q; want it to contain %q", in, err, tc.wantMsg)
			}
		})
	}
}
