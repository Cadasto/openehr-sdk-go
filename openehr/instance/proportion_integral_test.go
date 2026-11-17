package instance_test

import (
	"math"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// optProportionOver is an ELEMENT template whose value is a DV_PROPORTION
// with the given attributes.
func optProportionOver(attrs ...string) string {
	return optTemplate("ELEMENT", optSingle("value", optNode("DV_PROPORTION", "", attrs...)))
}

// optIntegralPinned is the is_integral attribute an OPT pins to true with a
// C_BOOLEAN that admits true alone, as Demonstration.v1 does.
func optIntegralPinned() string {
	return optSingle("is_integral", optPrimitive("BOOLEAN", "C_BOOLEAN", "<true_valid>true</true_valid><false_valid>false</false_valid>"))
}

// optProportionPart is the attribute name of a DV_PROPORTION carrying a
// C_REAL (numerator, denominator) or a C_INTEGER (type, precision) item
// with body.
func optProportionPart(name, body string) string {
	if name == "type" || name == "precision" {
		return optSingle(name, optPrimitive("INTEGER", "C_INTEGER", body))
	}
	return optSingle(name, optPrimitive("REAL", "C_REAL", body))
}

// wholeNumber reports whether v is a finite whole number.
func wholeNumber(v rm.Real) bool {
	f := float64(v)
	return !math.IsInf(f, 0) && !math.IsNaN(f) && math.Trunc(f) == f
}

// TestREQ107_IntegralProportionTakesPrecisionZero is the REQ-107 Leaf
// values check on a DV_PROPORTION's is_integral: a value whose template
// pins is_integral to true, and a fraction or an integer fraction, gets a
// whole-number numerator and denominator and the precision 0 wherever the
// template admits 0, under both value fills, a fraction the template
// requires a precision of with no child included; where the template
// admits no precision of 0 the precision it admits stays, and a ratio
// with no pin keeps no precision. The RM floor and the template validator
// pass.
func TestREQ107_IntegralProportionTakesPrecisionZero(t *testing.T) {
	r, closed := instance.RealRange, instance.Closed
	// Draws from this numerator range are rarely whole numbers.
	fractional := optProportionPart("numerator", r(closed(0.5), closed(9.5)))
	cases := []struct {
		name  string
		attrs []string
		// precision is the precision the value takes, or nil for none.
		precision *rm.Integer
		// whole says the numerator and the denominator are whole numbers.
		whole bool
	}{
		{
			name:      "fraction whose template requires a precision with no child",
			attrs:     []string{fractional, optProportionPart("type", instance.IntList(3)), optSingle("precision")},
			precision: new(rm.Integer(0)),
			whole:     true,
		},
		{
			name:      "fraction whose template names no precision",
			attrs:     []string{fractional, optProportionPart("type", instance.IntList(3))},
			precision: new(rm.Integer(0)),
			whole:     true,
		},
		{
			name:      "integer fraction over a precision list of 2 and 0",
			attrs:     []string{fractional, optProportionPart("type", instance.IntList(4)), optProportionPart("precision", instance.IntList(2, 0))},
			precision: new(rm.Integer(0)),
			whole:     true,
		},
		{
			name:      "fraction whose template prohibits the precision",
			attrs:     []string{fractional, optProportionPart("type", instance.IntList(3)), optProhibitedSingle("precision")},
			precision: nil,
			whole:     true,
		},
		{
			name:      "ratio whose template pins is_integral",
			attrs:     []string{optIntegralPinned(), fractional, optProportionPart("type", instance.IntList(0))},
			precision: new(rm.Integer(0)),
			whole:     true,
		},
		{
			name:      "ratio whose template pins is_integral and admits precision 2 alone",
			attrs:     []string{optIntegralPinned(), fractional, optProportionPart("type", instance.IntList(0)), optProportionPart("precision", instance.IntList(2))},
			precision: new(rm.Integer(2)),
			whole:     true,
		},
		{
			name:      "ratio with no pin",
			attrs:     []string{fractional, optProportionPart("type", instance.IntList(0))},
			precision: nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := compileOPTText(t, optProportionOver(tc.attrs...), true)
			eachProportionRun(t, c, func(t *testing.T, what string, out any, p rm.DVProportion) {
				switch {
				case tc.precision == nil && p.Precision != nil:
					t.Errorf("%s: DV_PROPORTION %s, want no precision", what, proportionText(p))
				case tc.precision != nil && (p.Precision == nil || *p.Precision != *tc.precision):
					t.Errorf("%s: DV_PROPORTION %s, want precision %d", what, proportionText(p), *tc.precision)
				}
				if tc.whole && !(wholeNumber(p.Numerator) && wholeNumber(p.Denominator)) {
					t.Errorf("%s: DV_PROPORTION %s, want a whole-number numerator and denominator", what, proportionText(p))
				}
				if breaks := proportionKindBreaks(p); len(breaks) > 0 {
					t.Errorf("%s: DV_PROPORTION %s breaks %v", what, proportionText(p), breaks)
				}
				checkValid(t, out, c)
			})
		})
	}
}
