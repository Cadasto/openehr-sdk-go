package instance

import (
	"errors"
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/templateinstance/rmwrite"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

func TestNewRMForOPTType_generics(t *testing.T) {
	cases := []struct {
		declared string
		check    func(any) bool
	}{
		{"DV_INTERVAL<DV_QUANTITY>", func(v any) bool { _, ok := v.(*rm.DVInterval[rm.DVQuantity]); return ok }},
		{"DV_INTERVAL<DV_COUNT>", func(v any) bool { _, ok := v.(*rm.DVInterval[rm.DVCount]); return ok }},
		{"DV_INTERVAL<DV_DATE_TIME>", func(v any) bool { _, ok := v.(*rm.DVInterval[rm.DVDateTime]); return ok }},
		{"DV_TEXT", func(v any) bool { _, ok := v.(*rm.DVText); return ok }},
		{"DV_INTERVAL", func(v any) bool { _, ok := v.(*rm.DVInterval[rm.DVOrdered]); return ok }},
	}
	for _, tc := range cases {
		t.Run(tc.declared, func(t *testing.T) {
			v, err := newRMForOPTType(tc.declared)
			if err != nil {
				t.Fatalf("newRMForOPTType: %v", err)
			}
			if !tc.check(v) {
				t.Fatalf("unexpected concrete type %T", v)
			}
		})
	}
}

func TestNewRMForOPTType_unknownGeneric(t *testing.T) {
	_, err := newRMForOPTType("DV_INTERVAL<NOT_A_TYPE>")
	if !errors.Is(err, rmwrite.ErrUnknownRMType) {
		t.Fatalf("want ErrUnknownRMType, got %v", err)
	}
}

func TestParseBMMGeneric(t *testing.T) {
	base, param, ok := parseBMMGeneric("DV_INTERVAL<DV_QUANTITY>")
	if !ok || base != "DV_INTERVAL" || param != "DV_QUANTITY" {
		t.Fatalf("parseBMMGeneric = %q %q %v, want DV_INTERVAL DV_QUANTITY true", base, param, ok)
	}
	if _, _, ok := parseBMMGeneric("DV_TEXT"); ok {
		t.Fatal("expected non-generic DV_TEXT to return ok=false")
	}
}
