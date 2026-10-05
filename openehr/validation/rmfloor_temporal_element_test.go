package validation_test

// rmfloor_temporal_element_test.go: pins for the two REQ-112 catalogue rows
// the floor evaluates on the node itself: Value_valid on the four ISO 8601
// data values, and ELEMENT's Inv_null_flavour_indicated (exactly one of
// value and null_flavour). Each test fails when its rule is removed.

import (
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
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

// TestREQ112_TemporalAndElementDetailsAreValueFree pins that the issues for
// Value_valid and Inv_null_flavour_indicated name the rule and the attribute,
// never the offending value. Each ELEMENT carries a malformed temporal literal
// and a null_flavour, so both rules fire, and the literal must appear in no
// field of any reported issue.
func TestREQ112_TemporalAndElementDetailsAreValueFree(t *testing.T) {
	const literal = "2024-13-45T99:99:99Z"
	cases := []struct {
		name  string
		value rm.DataValue
	}{
		{name: "DV_DATE_TIME", value: &rm.DVDateTime{Value: literal}},
		{name: "DV_DATE", value: &rm.DVDate{Value: literal}},
		{name: "DV_TIME", value: &rm.DVTime{Value: literal}},
		{name: "DV_DURATION", value: &rm.DVDuration{Value: literal}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			el := validElement()
			el.Value = tc.value
			el.NullFlavour = unknownNullFlavour()
			r := validation.ValidateRM(el)
			// Both rules must fire, or the check below would pass on a report
			// that holds neither of them.
			if !containsIssue(r.Issues, "/value", "rm_invariant") {
				t.Errorf("ValidateRM(ELEMENT with %s %q and a null_flavour): want rm_invariant (Value_valid) at /value; issues=%+v", tc.name, literal, r.Issues)
			}
			if !containsIssue(r.Issues, "/", "rm_invariant") {
				t.Errorf("ValidateRM(ELEMENT with %s %q and a null_flavour): want rm_invariant (Inv_null_flavour_indicated) at /; issues=%+v", tc.name, literal, r.Issues)
			}
			for _, issue := range r.Issues {
				fields := []struct{ name, text string }{
					{name: "Path", text: issue.Path},
					{name: "Code", text: issue.Code},
					{name: "Detail", text: issue.Detail},
					{name: "Severity", text: issue.Severity.String()},
				}
				for _, f := range fields {
					if strings.Contains(f.text, literal) {
						t.Errorf("ValidateRM(ELEMENT with %s %q): issue %s at %q has %s %q, which echoes the offending value", tc.name, literal, issue.Code, issue.Path, f.name, f.text)
					}
				}
			}
		})
	}
}

// TestREQ112_TemporalJSONNullValue pins what the floor reports today for a
// temporal data value whose value is JSON null, decoded through canjson. The
// null decodes to the empty string, and the floor reports it twice: as
// required at /value/value and as rm_invariant (Value_valid) at /value.
// REQ-112 does not state that pair; this test records it as it is.
func TestREQ112_TemporalJSONNullValue(t *testing.T) {
	for _, rmType := range []string{"DV_DATE_TIME", "DV_DATE", "DV_TIME", "DV_DURATION"} {
		t.Run(rmType, func(t *testing.T) {
			body := `{"_type":"ELEMENT","archetype_node_id":"at0001",` +
				`"name":{"_type":"DV_TEXT","value":"item"},` +
				`"value":{"_type":"` + rmType + `","value":null}}`
			var el rm.Element
			if err := canjson.Unmarshal([]byte(body), &el); err != nil {
				t.Fatalf("canjson.Unmarshal(%s): %v", body, err)
			}
			r := validation.ValidateRM(&el)
			got := make([]string, 0, len(r.Issues))
			for _, issue := range r.Issues {
				got = append(got, issue.Code+" "+issue.Path)
			}
			slices.Sort(got)
			want := []string{"required /value/value", "rm_invariant /value"}
			if !slices.Equal(got, want) {
				t.Errorf("ValidateRM(%s) issues (code path) = %q, want %q; issues=%+v", body, got, want, r.Issues)
			}
		})
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
