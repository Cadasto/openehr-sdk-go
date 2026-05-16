package rm_test

import (
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
)

// TestRealQuotedNumberDecodes asserts the permissive-decode half of
// ADR 0004: a JSON quoted decimal string is accepted for a `Real`
// field. The fixture cassettes (BMI.json) carry quoted magnitudes and
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
// This is the load-bearing guarantee for downstream consumers / PHP
// SDK parity.
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
