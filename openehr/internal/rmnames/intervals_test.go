package rmnames_test

import (
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/internal/rmnames"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
)

// typedIntervalCases enumerates the DVOrdered concrete closure —
// shared by the behaviour test (each case must resolve) and the
// registry parity test (the set of cases must be complete).
var typedIntervalCases = []struct {
	v    any
	want string
}{
	{rm.DVInterval[rm.DVCount]{}, "DV_INTERVAL<DV_COUNT>"},
	{&rm.DVInterval[rm.DVCount]{}, "DV_INTERVAL<DV_COUNT>"},
	{rm.DVInterval[rm.DVDate]{}, "DV_INTERVAL<DV_DATE>"},
	{&rm.DVInterval[rm.DVDate]{}, "DV_INTERVAL<DV_DATE>"},
	{rm.DVInterval[rm.DVDateTime]{}, "DV_INTERVAL<DV_DATE_TIME>"},
	{&rm.DVInterval[rm.DVDateTime]{}, "DV_INTERVAL<DV_DATE_TIME>"},
	{rm.DVInterval[rm.DVDuration]{}, "DV_INTERVAL<DV_DURATION>"},
	{&rm.DVInterval[rm.DVDuration]{}, "DV_INTERVAL<DV_DURATION>"},
	{rm.DVInterval[rm.DVOrdinal]{}, "DV_INTERVAL<DV_ORDINAL>"},
	{&rm.DVInterval[rm.DVOrdinal]{}, "DV_INTERVAL<DV_ORDINAL>"},
	{rm.DVInterval[rm.DVProportion]{}, "DV_INTERVAL<DV_PROPORTION>"},
	{&rm.DVInterval[rm.DVProportion]{}, "DV_INTERVAL<DV_PROPORTION>"},
	{rm.DVInterval[rm.DVQuantity]{}, "DV_INTERVAL<DV_QUANTITY>"},
	{&rm.DVInterval[rm.DVQuantity]{}, "DV_INTERVAL<DV_QUANTITY>"},
	{rm.DVInterval[rm.DVScale]{}, "DV_INTERVAL<DV_SCALE>"},
	{&rm.DVInterval[rm.DVScale]{}, "DV_INTERVAL<DV_SCALE>"},
	{rm.DVInterval[rm.DVTime]{}, "DV_INTERVAL<DV_TIME>"},
	{&rm.DVInterval[rm.DVTime]{}, "DV_INTERVAL<DV_TIME>"},
}

// TestTypedIntervalName exhaustively covers the DVOrdered concrete
// closure (9 instantiations × value/pointer) plus the negatives: the
// bare DVOrdered form, non-interval values, and nil.
func TestTypedIntervalName(t *testing.T) {
	for _, tc := range typedIntervalCases {
		got, ok := rmnames.TypedIntervalName(tc.v)
		if got != tc.want || !ok {
			t.Errorf("TypedIntervalName(%T) = (%q, %v), want (%q, true)", tc.v, got, ok, tc.want)
		}
	}
	// Negatives: bare form stays the registry's concern; non-interval
	// and nil report false.
	for _, v := range []any{rm.DVInterval[rm.DVOrdered]{}, rm.DVQuantity{}, nil} {
		if got, ok := rmnames.TypedIntervalName(v); got != "" || ok {
			t.Errorf("TypedIntervalName(%T) = (%q, %v), want (\"\", false)", v, got, ok)
		}
	}
}

// TestTypedIntervalClosureParity ties the hand-maintained switch to
// the registry: the parameterised names derivable from the live
// registry's DVOrdered implementers must equal the set the case table
// covers. A new DV_ORDERED descendant in the BMM registers a new
// implementer and fails here, forcing a new switch arm and table row
// — the drift tripwire in lieu of generator emission (one prior
// hand-written copy had drifted three instantiations behind).
func TestTypedIntervalClosureParity(t *testing.T) {
	var fromRegistry []string
	for _, name := range typereg.Default.Names() {
		ctor, _ := typereg.Default.Lookup(name)
		if _, ok := ctor().(rm.DVOrdered); ok {
			fromRegistry = append(fromRegistry, "DV_INTERVAL<"+name+">")
		}
	}
	var fromCases []string
	for _, tc := range typedIntervalCases {
		if !slices.Contains(fromCases, tc.want) {
			fromCases = append(fromCases, tc.want)
		}
	}
	slices.Sort(fromCases)
	if len(fromRegistry) == 0 {
		t.Fatal("registry yields no DVOrdered implementers — registrations missing?")
	}
	if !slices.Equal(fromRegistry, fromCases) {
		t.Errorf("DVOrdered closure drift:\n  registry-derived: %v\n  case table:       %v", fromRegistry, fromCases)
	}
}
