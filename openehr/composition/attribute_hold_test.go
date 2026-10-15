package composition

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm/rminfo"
)

// TestREQ101_PlainValueTypesArePrimitives checks, for REQ-101 Type
// enforcement, that every plain value type the attribute check refuses an
// RM value under is a name the pinned RM metadata knows as a primitive
// type, so a misspelt entry, which no attribute type would ever match,
// fails here rather than letting Set accept a value the attribute cannot
// hold.
func TestREQ101_PlainValueTypesArePrimitives(t *testing.T) {
	t.Parallel()
	ar, ok := rminfo.Default.(rminfo.AbsenceReporter)
	if !ok {
		t.Fatal("rminfo.Default does not report absence reasons")
	}
	for name := range plainValueTypes {
		if got := ar.AbsenceReason(name); got != rminfo.AbsencePrimitive {
			t.Errorf("rminfo.Default.AbsenceReason(%q) = %v, want %v", name, got, rminfo.AbsencePrimitive)
		}
	}
}

// TestREQ101_CanHold checks, for REQ-101 Type enforcement, which values an
// RM attribute of a given type can hold: a value of a type that is the
// attribute's or descends from it, by class; no RM value under a plain
// value type, such as the String of DV_DURATION.value or the Integer64 of
// DV_COUNT.magnitude; and no verdict, so no refusal, where the pinned
// class hierarchy does not know the attribute's type: a generic parameter
// left open, Any, and Iso8601_duration, which DV_DURATION descends from in
// the BMM although the hierarchy drops that edge.
func TestREQ101_CanHold(t *testing.T) {
	t.Parallel()
	cases := []struct {
		held, got    string
		holds, known bool
	}{
		{held: "String", got: "DV_DURATION", holds: false, known: true},
		{held: "Integer64", got: "DV_COUNT", holds: false, known: true},
		{held: "DATA_VALUE", got: "DV_QUANTITY", holds: true, known: true},
		{held: "DATA_VALUE", got: "ELEMENT", holds: false, known: true},
		{held: "DATA_VALUE", got: "PARTY_IDENTIFIED", holds: false, known: true},
		{held: "DV_COUNT", got: "DV_COUNT", holds: true, known: true},
		{held: "DV_COUNT", got: "DV_QUANTITY", holds: false, known: true},
		{held: "DV_INTERVAL", got: "DV_INTERVAL<DV_COUNT>", holds: true, known: true},
		{held: "T", got: "DV_QUANTITY", holds: false, known: false},
		{held: "Any", got: "DV_DURATION", holds: false, known: false},
		{held: "Iso8601_duration", got: "DV_DURATION", holds: false, known: false},
	}
	for _, tc := range cases {
		t.Run(tc.held+" holds "+tc.got, func(t *testing.T) {
			t.Parallel()
			holds, known := canHold(tc.held, tc.got)
			if known != tc.known || (known && holds != tc.holds) {
				t.Errorf("canHold(%q, %q) = (%v, %v), want (%v, %v)", tc.held, tc.got, holds, known, tc.holds, tc.known)
			}
		})
	}
}
