package canxml_test

// real_precision_test.go: a Real element in canonical XML (REQ-052
// § Floating-point precision, REQ-056). A lossy literal is refused as any
// primitive element whose text does not decode is refused, and a literal that
// is not lossy decodes to the float64 nearest it. The text rules XML had
// before the lossy test stay: empty text reads as 0, surrounding whitespace is
// trimmed, and whitespace alone or a quoted number is text that does not spell
// a Real.

import (
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml"
)

// xmlQuantity returns a DV_QUANTITY document whose magnitude element holds
// magnitude as its text.
func xmlQuantity(magnitude string) []byte {
	return []byte(`<dv_quantity ` + ucNS + `><magnitude>` + magnitude + `</magnitude><units>kg</units></dv_quantity>`)
}

// TestUnmarshal_REQ056_LossyRealLiteralIsRefused is the canonical-XML half of
// the acceptance in wire.md § Floating-point precision (REQ-052, REQ-056): in
// every decode mode, a lossy DV_QUANTITY magnitude fails with a
// *canxml.DecodeError naming the element's path, wrapping
// canxml.ErrInvalidShape and not canonical JSON's shape sentinel, with a text
// that does not carry the literal.
//
// Can-fail control: drop rm.Real's lossy test from UnmarshalText, or remove
// the method so encoding/xml parses the text itself, and every row decodes.
func TestUnmarshal_REQ056_LossyRealLiteralIsRefused(t *testing.T) {
	const path = "/dv_quantity[1]/magnitude[1]"
	for _, lit := range []string{"9007199254740993", "1.2345678901234567890", "1e-400"} {
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
				if errors.Is(err, typereg.ErrInvalidShape) {
					t.Errorf("canxml.Unmarshal(%s, magnitude %s) err = %v; want no canonical-JSON shape sentinel in an XML refusal", mode, lit, err)
				}
				if strings.Contains(err.Error(), lit) {
					t.Errorf("canxml.Unmarshal(%s, magnitude %s) err = %q; it carries the literal", mode, lit, err)
				}
			}
		})
	}
}

// TestUnmarshal_REQ056_RealLiteralDecodesToNearestFloat64 pins the other side
// of the lossy test in canonical XML (REQ-052, REQ-056): a magnitude that is
// not lossy, however many digits it spells, decodes in every mode to the
// float64 nearest the literal.
//
// Can-fail control: refuse a literal by a fixed digit budget and
// 18446744073709551616 and the 35-digit expansion of 0.1 fail.
func TestUnmarshal_REQ056_RealLiteralDecodesToNearestFloat64(t *testing.T) {
	for _, lit := range []string{"18446744073709551616", "0.10000000000000001", "0.1000000000000000055511151231257827", "1.10", "72.5"} {
		t.Run(lit, func(t *testing.T) {
			want, err := strconv.ParseFloat(lit, 64)
			if err != nil {
				t.Fatalf("strconv.ParseFloat(%s) = %v", lit, err)
			}
			for mode, opts := range xmlDecodeModes() {
				var q rm.DVQuantity
				if err := canxml.Unmarshal(xmlQuantity(lit), &q, opts...); err != nil {
					t.Fatalf("canxml.Unmarshal(%s, magnitude %s) = %v; want it decoded", mode, lit, err)
				}
				if float64(q.Magnitude) != want {
					t.Errorf("canxml.Unmarshal(%s, magnitude %s) magnitude = %v; want the nearest float64, %v", mode, lit, float64(q.Magnitude), want)
				}
			}
		})
	}
}

// TestUnmarshal_REQ056_RealTextRules pins the text rules a Real element had
// before the lossy test (REQ-056): empty text reads as 0 and whitespace around
// the number is trimmed, while whitespace alone and a quoted number are text
// that does not spell a Real, refused with the *strconv.NumError reachable.
//
// Can-fail control: hand UnmarshalText's input to strconv.ParseFloat
// untrimmed and the padded row fails; parse empty text instead of reading it
// as 0 and the empty rows fail.
func TestUnmarshal_REQ056_RealTextRules(t *testing.T) {
	for _, tc := range []struct {
		name, doc string
		want      float64
	}{
		{name: "empty", doc: `<dv_quantity ` + ucNS + `><magnitude></magnitude><units>kg</units></dv_quantity>`, want: 0},
		{name: "self-closing", doc: `<dv_quantity ` + ucNS + `><magnitude/><units>kg</units></dv_quantity>`, want: 0},
		{name: "padded", doc: string(xmlQuantity(" \n\t72.5 \r\n")), want: 72.5},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for mode, opts := range xmlDecodeModes() {
				q := rm.DVQuantity{Magnitude: 1}
				if err := canxml.Unmarshal([]byte(tc.doc), &q, opts...); err != nil {
					t.Fatalf("canxml.Unmarshal(%s, %s) = %v; want it decoded", mode, tc.doc, err)
				}
				if float64(q.Magnitude) != tc.want {
					t.Errorf("canxml.Unmarshal(%s, %s) magnitude = %v; want %v", mode, tc.doc, float64(q.Magnitude), tc.want)
				}
			}
		})
	}
	for _, tc := range []struct{ name, text string }{
		{name: "whitespace only", text: " \n\t "},
		{name: "quoted", text: `"72.5"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for mode, opts := range xmlDecodeModes() {
				err := canxml.Unmarshal(xmlQuantity(tc.text), new(rm.DVQuantity), opts...)
				if ne, ok := errors.AsType[*strconv.NumError](err); !ok || ne == nil {
					t.Errorf("canxml.Unmarshal(%s, magnitude %q) err = %v (%T); want a *strconv.NumError reachable", mode, tc.text, err, err)
				}
				if !errors.Is(err, canxml.ErrInvalidShape) {
					t.Errorf("canxml.Unmarshal(%s, magnitude %q) err = %v; want errors.Is(_, canxml.ErrInvalidShape)", mode, tc.text, err)
				}
			}
		})
	}
}
