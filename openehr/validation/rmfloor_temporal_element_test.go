package validation_test

// rmfloor_temporal_element_test.go: pins for the two REQ-112 catalogue rows
// the floor evaluates on the node itself: Value_valid on the four ISO 8601
// data values, and ELEMENT's Inv_null_flavour_indicated (exactly one of
// value and null_flavour). Each test fails when its rule is removed.

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// TestREQ112_TemporalValueValid covers the REQ-112 Value_valid rule: a value
// that fails the type's ISO 8601 predicate (the empty string and a
// placeholder included) is reported as rm_invariant on the node, and the
// partial and deviating forms the REQ-123 parse admits stay valid.
func TestREQ112_TemporalValueValid(t *testing.T) {
	cases := []struct {
		name string
		root any
		ok   bool
	}{
		{"DV_DATE_TIME placeholder", &rm.DVDateTime{Value: "example"}, false},
		{"DV_DATE_TIME empty", &rm.DVDateTime{Value: ""}, false},
		{"DV_DATE_TIME full", &rm.DVDateTime{Value: "2024-03-15T10:30:00Z"}, true},
		{"DV_DATE_TIME partial year", &rm.DVDateTime{Value: "2024"}, true},
		{"DV_DATE placeholder", &rm.DVDate{Value: "example"}, false},
		{"DV_DATE empty", &rm.DVDate{Value: ""}, false},
		{"DV_DATE month out of range", &rm.DVDate{Value: "2024-13-01"}, false},
		{"DV_DATE full", &rm.DVDate{Value: "2024-03-15"}, true},
		{"DV_DATE partial month", &rm.DVDate{Value: "2024-03"}, true},
		{"DV_TIME placeholder", &rm.DVTime{Value: "example"}, false},
		{"DV_TIME empty", &rm.DVTime{Value: ""}, false},
		{"DV_TIME full", &rm.DVTime{Value: "10:30:00"}, true},
		{"DV_TIME partial hour-minute", &rm.DVTime{Value: "10:30"}, true},
		{"DV_DURATION placeholder", &rm.DVDuration{Value: "example"}, false},
		{"DV_DURATION empty", &rm.DVDuration{Value: ""}, false},
		{"DV_DURATION bare P", &rm.DVDuration{Value: "P"}, false},
		{"DV_DURATION full", &rm.DVDuration{Value: "P1Y2M3DT4H5M6S"}, true},
		{"DV_DURATION week mixed with day", &rm.DVDuration{Value: "P1W2D"}, true},
		{"DV_DURATION negative", &rm.DVDuration{Value: "-P1D"}, true},
		{"DV_DATE_TIME by value", rm.DVDateTime{Value: "example"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := validation.ValidateRM(tc.root)
			if got := containsIssue(r.Issues, "/", "rm_invariant"); got == tc.ok {
				t.Errorf("ValidateRM(%T %+v): rm_invariant at \"/\" = %v, want %v; issues=%+v", tc.root, tc.root, got, !tc.ok, r.Issues)
			}
			if tc.ok && !r.OK {
				t.Errorf("ValidateRM(%T %+v) want OK; issues=%+v", tc.root, tc.root, r.Issues)
			}
		})
	}
}

// TestREQ112_TemporalValueValidNested pins that the rule fires on the node
// wherever the walk reaches it: an ELEMENT value is reported at /value.
func TestREQ112_TemporalValueValidNested(t *testing.T) {
	el := validElement()
	el.Value = &rm.DVDateTime{Value: "example"}
	r := validation.ValidateRM(el)
	if !containsIssue(r.Issues, "/value", "rm_invariant") {
		t.Errorf("ValidateRM(ELEMENT with placeholder DV_DATE_TIME) want rm_invariant at /value; got %+v", r.Issues)
	}
}

func validElement() *rm.Element {
	return &rm.Element{
		ArchetypeNodeID: "at0001",
		Name:            rm.DVText{Value: "item"},
	}
}

func unknownNullFlavour() *rm.DVCodedText {
	return &rm.DVCodedText{
		Value: "unknown",
		DefiningCode: rm.CodePhrase{
			TerminologyID: rm.TerminologyID{Value: "openehr"},
			CodeString:    "253",
		},
	}
}

// TestREQ112_ElementNullFlavourIndicated covers Inv_null_flavour_indicated
// (is_null() xor null_flavour = Void): exactly one of value and null_flavour.
func TestREQ112_ElementNullFlavourIndicated(t *testing.T) {
	both := func() *rm.Element {
		e := validElement()
		e.Value = &rm.DVText{Value: "x"}
		e.NullFlavour = unknownNullFlavour()
		return e
	}
	cases := []struct {
		name  string
		build func() *rm.Element
		ok    bool
	}{
		{"value only", func() *rm.Element {
			e := validElement()
			e.Value = &rm.DVText{Value: "x"}
			return e
		}, true},
		{"null_flavour only", func() *rm.Element {
			e := validElement()
			e.NullFlavour = unknownNullFlavour()
			return e
		}, true},
		{"both", both, false},
		{"neither", validElement, false},
		{"typed-nil value counts as no value, with null_flavour", func() *rm.Element {
			e := validElement()
			e.Value = (*rm.DVText)(nil)
			e.NullFlavour = unknownNullFlavour()
			return e
		}, true},
		{"typed-nil value counts as no value, alone", func() *rm.Element {
			e := validElement()
			e.Value = (*rm.DVText)(nil)
			return e
		}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := validation.ValidateRM(tc.build())
			if got := containsIssue(r.Issues, "/", "rm_invariant"); got == tc.ok {
				t.Errorf("ValidateRM(ELEMENT %s): rm_invariant at \"/\" = %v, want %v; issues=%+v", tc.name, got, !tc.ok, r.Issues)
			}
			if tc.ok && !r.OK {
				t.Errorf("ValidateRM(ELEMENT %s) want OK; issues=%+v", tc.name, r.Issues)
			}
		})
	}

	// REQ-112: an ELEMENT held by value, as the root or as a member of
	// CLUSTER.items, is checked the same way and reported where it sits.
	inCluster := func(e *rm.Element) *rm.Cluster {
		return &rm.Cluster{
			ArchetypeNodeID: "at0000",
			Name:            rm.DVText{Value: "group"},
			Items:           []rm.Item{*e},
		}
	}
	byValue := []struct {
		name string
		root any
		at   string
	}{
		{"both, root by value", *both(), "/"},
		{"neither, root by value", *validElement(), "/"},
		{"both, by value in CLUSTER.items", inCluster(both()), "/items[0]"},
		{"neither, by value in CLUSTER.items", inCluster(validElement()), "/items[0]"},
	}
	for _, tc := range byValue {
		t.Run(tc.name, func(t *testing.T) {
			r := validation.ValidateRM(tc.root)
			if !containsIssue(r.Issues, tc.at, "rm_invariant") {
				t.Errorf("ValidateRM(ELEMENT %s): want rm_invariant at %q; issues=%+v", tc.name, tc.at, r.Issues)
			}
		})
	}
}
