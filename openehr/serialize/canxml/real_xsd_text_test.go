package canxml_test

import (
	"bytes"
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml"
)

// TestUnmarshal_REQ056_RealTextOutsideXSDoubleIsRefused pins, in every
// decode mode, that a DV_QUANTITY magnitude strconv.ParseFloat reads but the
// xs:double lexical space lacks fails as a primitive element whose text does
// not decode: a *canxml.DecodeError on the element's path that wraps
// canxml.ErrInvalidShape, with a text that does not carry the literal
// (REQ-056).
func TestUnmarshal_REQ056_RealTextOutsideXSDoubleIsRefused(t *testing.T) {
	const path = "/dv_quantity[1]/magnitude[1]"
	for _, lit := range []string{"0x1p-2", "1_000", "Inf", "Infinity", "nan"} {
		t.Run(lit, func(t *testing.T) {
			for mode, opts := range xmlDecodeModes() {
				var q rm.DVQuantity
				err := canxml.Unmarshal(xmlQuantity(lit), &q, opts...)
				de, ok := errors.AsType[*canxml.DecodeError](err)
				if !ok || de == nil {
					t.Fatalf("canxml.Unmarshal(%s, magnitude %s) err = %v, magnitude %v; want a *canxml.DecodeError", mode, lit, err, float64(q.Magnitude))
				}
				if de.Path != path {
					t.Errorf("canxml.Unmarshal(%s, magnitude %s) DecodeError.Path = %q; want %q", mode, lit, de.Path, path)
				}
				if !errors.Is(err, canxml.ErrInvalidShape) {
					t.Errorf("canxml.Unmarshal(%s, magnitude %s) err = %v; want errors.Is(_, canxml.ErrInvalidShape)", mode, lit, err)
				}
				if strings.Contains(err.Error(), lit) {
					t.Errorf("canxml.Unmarshal(%s, magnitude %s) err = %q; it carries the literal", mode, lit, err)
				}
			}
		})
	}
}

// TestUnmarshal_REQ056_RealTextInXSDoubleDecodes pins, in every decode mode,
// that the special values and the decimal forms of the xs:double lexical
// space strconv.ParseFloat also reads decode as a DV_QUANTITY magnitude
// (REQ-056).
func TestUnmarshal_REQ056_RealTextInXSDoubleDecodes(t *testing.T) {
	for _, tc := range []struct {
		lit  string
		want float64
	}{
		{"INF", math.Inf(1)},
		{"+INF", math.Inf(1)},
		{"-INF", math.Inf(-1)},
		{"NaN", math.NaN()},
		{"-1.", -1},
		{".5", 0.5},
		{"1E-3", 0.001},
	} {
		t.Run(tc.lit, func(t *testing.T) {
			for mode, opts := range xmlDecodeModes() {
				var q rm.DVQuantity
				if err := canxml.Unmarshal(xmlQuantity(tc.lit), &q, opts...); err != nil {
					t.Fatalf("canxml.Unmarshal(%s, magnitude %s) = %v; want it decoded", mode, tc.lit, err)
				}
				got := float64(q.Magnitude)
				if got != tc.want && !(math.IsNaN(tc.want) && math.IsNaN(got)) {
					t.Errorf("canxml.Unmarshal(%s, magnitude %s) magnitude = %v; want %v", mode, tc.lit, got, tc.want)
				}
			}
		})
	}
}

// TestMarshal_REQ056_RealSpecialValuesAreXSDouble pins that canonical XML
// writes a DV_QUANTITY magnitude that is an infinity as INF or -INF and one
// that is not a number as NaN, the xs:double spellings, and that the
// document decodes back to the same value (REQ-056).
func TestMarshal_REQ056_RealSpecialValuesAreXSDouble(t *testing.T) {
	for _, tc := range []struct {
		in   float64
		want string
	}{
		{math.Inf(1), "INF"},
		{math.Inf(-1), "-INF"},
		{math.NaN(), "NaN"},
	} {
		t.Run(tc.want, func(t *testing.T) {
			b, err := canxml.Marshal(&rm.DVQuantity{Magnitude: rm.Real(tc.in), Units: "kg"})
			if err != nil {
				t.Fatalf("canxml.Marshal(magnitude %v): %v", tc.in, err)
			}
			if want := "<magnitude>" + tc.want + "</magnitude>"; !bytes.Contains(b, []byte(want)) {
				t.Errorf("canxml.Marshal(magnitude %v) = %s; want it to carry %s", tc.in, b, want)
			}
			var back rm.DVQuantity
			if err := canxml.Unmarshal(b, &back); err != nil {
				t.Fatalf("canxml.Unmarshal(%s) = %v; want it decoded", b, err)
			}
			got := float64(back.Magnitude)
			if got != tc.in && !(math.IsNaN(tc.in) && math.IsNaN(got)) {
				t.Errorf("canxml.Unmarshal(%s) magnitude = %v; want %v", b, got, tc.in)
			}
		})
	}
}
