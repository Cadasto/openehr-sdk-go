package rmnames_test

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/internal/rmnames"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// TestTypedIntervalName exhaustively covers the DVOrdered concrete
// closure (9 instantiations × value/pointer) plus the negatives: the
// bare DVOrdered form, non-interval values, and nil.
func TestTypedIntervalName(t *testing.T) {
	tests := []struct {
		v      any
		want   string
		wantOK bool
	}{
		{rm.DVInterval[rm.DVCount]{}, "DV_INTERVAL<DV_COUNT>", true},
		{&rm.DVInterval[rm.DVCount]{}, "DV_INTERVAL<DV_COUNT>", true},
		{rm.DVInterval[rm.DVDate]{}, "DV_INTERVAL<DV_DATE>", true},
		{&rm.DVInterval[rm.DVDate]{}, "DV_INTERVAL<DV_DATE>", true},
		{rm.DVInterval[rm.DVDateTime]{}, "DV_INTERVAL<DV_DATE_TIME>", true},
		{&rm.DVInterval[rm.DVDateTime]{}, "DV_INTERVAL<DV_DATE_TIME>", true},
		{rm.DVInterval[rm.DVDuration]{}, "DV_INTERVAL<DV_DURATION>", true},
		{&rm.DVInterval[rm.DVDuration]{}, "DV_INTERVAL<DV_DURATION>", true},
		{rm.DVInterval[rm.DVOrdinal]{}, "DV_INTERVAL<DV_ORDINAL>", true},
		{&rm.DVInterval[rm.DVOrdinal]{}, "DV_INTERVAL<DV_ORDINAL>", true},
		{rm.DVInterval[rm.DVProportion]{}, "DV_INTERVAL<DV_PROPORTION>", true},
		{&rm.DVInterval[rm.DVProportion]{}, "DV_INTERVAL<DV_PROPORTION>", true},
		{rm.DVInterval[rm.DVQuantity]{}, "DV_INTERVAL<DV_QUANTITY>", true},
		{&rm.DVInterval[rm.DVQuantity]{}, "DV_INTERVAL<DV_QUANTITY>", true},
		{rm.DVInterval[rm.DVScale]{}, "DV_INTERVAL<DV_SCALE>", true},
		{&rm.DVInterval[rm.DVScale]{}, "DV_INTERVAL<DV_SCALE>", true},
		{rm.DVInterval[rm.DVTime]{}, "DV_INTERVAL<DV_TIME>", true},
		{&rm.DVInterval[rm.DVTime]{}, "DV_INTERVAL<DV_TIME>", true},
		// Negatives: bare form stays the registry's concern; non-interval
		// and nil report false.
		{rm.DVInterval[rm.DVOrdered]{}, "", false},
		{rm.DVQuantity{}, "", false},
		{nil, "", false},
	}
	for _, tc := range tests {
		got, ok := rmnames.TypedIntervalName(tc.v)
		if got != tc.want || ok != tc.wantOK {
			t.Errorf("TypedIntervalName(%T) = (%q, %v), want (%q, %v)", tc.v, got, ok, tc.want, tc.wantOK)
		}
	}
}
