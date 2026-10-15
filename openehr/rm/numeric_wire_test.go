package rm_test

import (
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
)

// TestRealQuotedNumberDecodes asserts the permissive-decode half of
// ADR 0004: a JSON quoted decimal string is accepted for a `Real`
// field. The vendored fixtures (BMI.json) carry quoted magnitudes and
// the SDK MUST decode them without error.
func TestRealQuotedNumberDecodes(t *testing.T) {
	in := []byte(`{"_type":"DV_QUANTITY","magnitude":"80.5","units":"kg"}`)
	var q rm.DVQuantity
	if err := canjson.Unmarshal(in, &q); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if float64(q.Magnitude) != 80.5 {
		t.Errorf("Magnitude = %v; want 80.5 (from quoted input)", q.Magnitude)
	}
}

// TestRealEncodesAsNumber asserts the strict-encode half of ADR 0004:
// regardless of how the value was decoded, encode emits a JSON number.
// This is the load-bearing guarantee for downstream consumers and
// openEHR wire conformance.
func TestRealEncodesAsNumber(t *testing.T) {
	in := []byte(`{"_type":"DV_QUANTITY","magnitude":"80.5","units":"kg"}`)
	var q rm.DVQuantity
	if err := canjson.Unmarshal(in, &q); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	out, err := canjson.Marshal(&q)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if !strings.Contains(string(out), `"magnitude":80.5`) {
		t.Errorf("encode must emit unquoted number; got: %s", out)
	}
	if strings.Contains(string(out), `"magnitude":"80.5"`) {
		t.Errorf("encode MUST NOT emit quoted magnitude; got: %s", out)
	}
}

// TestIntegerQuotedNumberDecodes — same asymmetric tolerance for
// `Integer`. `DV_COUNT.magnitude` is `int64` (Integer64) so we use
// `DV_PROPORTION.type` which is BMM `Integer` (int32) for this
// assertion.
func TestIntegerQuotedNumberDecodes(t *testing.T) {
	in := []byte(`{"_type":"DV_PROPORTION","numerator":1,"denominator":2,"type":"1"}`)
	var p rm.DVProportion
	if err := canjson.Unmarshal(in, &p); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if int32(p.Type) != 1 {
		t.Errorf("Type = %v; want 1 (from quoted input)", p.Type)
	}
}

// TestIntegerWholeNumberWithFractionDecodes pins wire.md § Floating-point
// precision (REQ-052, ADR 0004 as amended): an `Integer` field accepts a
// number whose value is a whole number however the literal writes it, with
// a zero fraction or an exponent, bare or quoted, and reads it as that
// integer. The reference's test_all_types FLAT writes a DV_PROPORTION
// `type` as 1.0, which PROBE-105's flat-canonical leg reaches through
// canjson, so the case runs through canjson on that very field.
func TestIntegerWholeNumberWithFractionDecodes(t *testing.T) { // REQ-052, PROBE-105
	t.Parallel()
	for _, tc := range []struct {
		lit  string
		want int32
	}{
		{lit: `1.0`, want: 1},
		{lit: `1e0`, want: 1},
		{lit: `10.00`, want: 10},
		{lit: `1E1`, want: 10},
		{lit: `"1.0"`, want: 1},
	} {
		t.Run(tc.lit, func(t *testing.T) {
			t.Parallel()
			in := []byte(`{"_type":"DV_PROPORTION","numerator":1.5,"denominator":1.0,"type":` + tc.lit + `}`)
			var p rm.DVProportion
			if err := canjson.Unmarshal(in, &p); err != nil {
				t.Fatalf("canjson.Unmarshal(type %s) = %v, want nil", tc.lit, err)
			}
			if int32(p.Type) != tc.want {
				t.Errorf("canjson.Unmarshal(type %s): Type = %d, want %d", tc.lit, p.Type, tc.want)
			}
		})
	}
}

// TestIntegerRefusalThroughCanjsonKeepsShapeSentinel pins that accepting a
// whole number written with a fraction leaves the refusals through a
// generated RM type classified as they were (REQ-052, wire.md § Decode-side
// shape sentinel and § Floating-point precision). A non-zero fraction or an
// out-of-range value on DV_PROPORTION.type is refused, the generated funnel
// wraps the refusal in canjson.ErrInvalidShape, never in the encode-only
// canjson.ErrInvalidValue, and a quoted literal keeps its *strconv.NumError
// cause reachable.
func TestIntegerRefusalThroughCanjsonKeepsShapeSentinel(t *testing.T) { // REQ-052
	t.Parallel()
	for _, lit := range []string{`1.5`, `1e10`, `"1.5"`, `"1e10"`} {
		t.Run(lit, func(t *testing.T) {
			t.Parallel()
			in := []byte(`{"_type":"DV_PROPORTION","numerator":1.5,"denominator":1.0,"type":` + lit + `}`)
			var p rm.DVProportion
			err := canjson.Unmarshal(in, &p)
			if err == nil {
				t.Fatalf("canjson.Unmarshal(type %s) = nil error, Type = %d; want a refusal", lit, p.Type)
			}
			if !errors.Is(err, canjson.ErrInvalidShape) {
				t.Errorf("canjson.Unmarshal(type %s) err = %v; want it to wrap canjson.ErrInvalidShape", lit, err)
			}
			if errors.Is(err, canjson.ErrInvalidValue) {
				t.Errorf("canjson.Unmarshal(type %s) err = %v; canjson.ErrInvalidValue must not appear on a decode path", lit, err)
			}
			if strings.HasPrefix(lit, `"`) {
				if _, ok := errors.AsType[*strconv.NumError](err); !ok {
					t.Errorf("canjson.Unmarshal(type %s) err = %v; want the *strconv.NumError cause reachable", lit, err)
				}
			}
		})
	}
}

// TestIntegerEncodesAsNumber — strict-encode for `Integer`.
func TestIntegerEncodesAsNumber(t *testing.T) {
	p := &rm.DVProportion{Numerator: 1, Denominator: 2, Type: rm.Integer(1)}
	out, err := canjson.Marshal(p)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if !strings.Contains(string(out), `"type":1`) {
		t.Errorf("encode must emit unquoted integer; got: %s", out)
	}
	if strings.Contains(string(out), `"type":"1"`) {
		t.Errorf("encode MUST NOT emit quoted Integer; got: %s", out)
	}
}

// TestRealMalformedStringFails asserts that a quoted but
// un-parseable string surfaces as a typed error, not a silent zero.
func TestRealMalformedStringFails(t *testing.T) {
	in := []byte(`{"_type":"DV_QUANTITY","magnitude":"not-a-number","units":"kg"}`)
	var q rm.DVQuantity
	err := canjson.Unmarshal(in, &q)
	if err == nil {
		t.Fatal("expected error for malformed quoted magnitude")
	}
	if !strings.Contains(err.Error(), "rm.Real") {
		t.Errorf("err = %v; want it to mention rm.Real", err)
	}
}
