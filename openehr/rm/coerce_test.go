package rm_test

import (
	"math"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

func TestAsInt64(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want int64
		ok   bool
	}{
		{"int", 7, 7, true},
		{"int64", int64(-3), -3, true},
		{"rm.Integer", rm.Integer(42), 42, true},
		{"rm.Integer64", rm.Integer64(42), 42, true},
		{"rm.Integer64 max", rm.Integer64(math.MaxInt64), math.MaxInt64, true},
		{"rm.Integer64 min", rm.Integer64(math.MinInt64), math.MinInt64, true},
		{"uint64 max", uint64(math.MaxInt64), math.MaxInt64, true},
		{"uint64 overflow", uint64(math.MaxInt64) + 1, 0, false},
		{"float rejected", 1.5, 0, false},
		{"string rejected", "7", 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := rm.AsInt64(tc.in)
			if ok != tc.ok {
				t.Fatalf("ok = %v, want %v", ok, tc.ok)
			}
			if ok && got != tc.want {
				t.Fatalf("got = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestAsReal(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want rm.Real
		ok   bool
	}{
		{"rm.Real", rm.Real(1.5), rm.Real(1.5), true},
		{"float64", 2.25, rm.Real(2.25), true},
		{"int widens", 3, rm.Real(3), true},
		{"rm.Integer widens", rm.Integer(-4), rm.Real(-4), true},
		{"rm.Integer64 widens", rm.Integer64(5), rm.Real(5), true},
		{"rm.Integer64 past int32 widens", rm.Integer64(1 << 40), rm.Real(1 << 40), true},
		{"string rejected", "1.5", 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := rm.AsReal(tc.in)
			if ok != tc.ok {
				t.Fatalf("ok = %v, want %v", ok, tc.ok)
			}
			if ok && got != tc.want {
				t.Fatalf("got = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestIsInt64IsReal(t *testing.T) {
	if !rm.IsInt64(rm.Integer(1)) {
		t.Error("IsInt64(rm.Integer) = false, want true")
	}
	if !rm.IsInt64(rm.Integer64(1)) {
		t.Error("IsInt64(rm.Integer64) = false, want true")
	}
	if !rm.IsReal(rm.Integer64(1)) {
		t.Error("IsReal(rm.Integer64) = false, want true")
	}
	if rm.IsInt64(1.0) {
		t.Error("IsInt64(float) = true, want false")
	}
	if !rm.IsReal(rm.Real(1)) {
		t.Error("IsReal(rm.Real) = false, want true")
	}
}
