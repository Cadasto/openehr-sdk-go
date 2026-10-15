package validation_test

// rmfloor_temporal_element_test.go: pins for the two REQ-112 catalogue rows
// the floor evaluates on the node itself: Value_valid on the four ISO 8601
// data values, and ELEMENT's Inv_null_flavour_indicated (exactly one of
// value and null_flavour). Each test fails when its rule is removed.

import (
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// TestREQ112_TemporalValueValid covers the REQ-112 Value_valid rule: a
// non-empty value that fails the type's ISO 8601 predicate (a placeholder
// included) is reported as rm_invariant on the node, and the partial and
// deviating forms the REQ-123 parse admits stay valid. An empty value is an
// absent RM-mandatory attribute, reported once, as required at /value, and
// not by Value_valid. Each row lists the whole result, by code and path.
func TestREQ112_TemporalValueValid(t *testing.T) {
	invalid := []string{"rm_invariant /"}
	empty := []string{"required /value"}
	cases := []struct {
		name string
		root any
		want []string
	}{
		{"DV_DATE_TIME placeholder", &rm.DVDateTime{Value: "example"}, invalid},
		{"DV_DATE_TIME empty", &rm.DVDateTime{Value: ""}, empty},
		{"DV_DATE_TIME full", &rm.DVDateTime{Value: "2024-03-15T10:30:00Z"}, nil},
		{"DV_DATE_TIME partial year", &rm.DVDateTime{Value: "2024"}, nil},
		{"DV_DATE placeholder", &rm.DVDate{Value: "example"}, invalid},
		{"DV_DATE empty", &rm.DVDate{Value: ""}, empty},
		{"DV_DATE month out of range", &rm.DVDate{Value: "2024-13-01"}, invalid},
		{"DV_DATE full", &rm.DVDate{Value: "2024-03-15"}, nil},
		{"DV_DATE partial month", &rm.DVDate{Value: "2024-03"}, nil},
		{"DV_TIME placeholder", &rm.DVTime{Value: "example"}, invalid},
		{"DV_TIME empty", &rm.DVTime{Value: ""}, empty},
		{"DV_TIME full", &rm.DVTime{Value: "10:30:00"}, nil},
		{"DV_TIME partial hour-minute", &rm.DVTime{Value: "10:30"}, nil},
		{"DV_DURATION placeholder", &rm.DVDuration{Value: "example"}, invalid},
		{"DV_DURATION empty", &rm.DVDuration{Value: ""}, empty},
		{"DV_DURATION bare P", &rm.DVDuration{Value: "P"}, invalid},
		{"DV_DURATION full", &rm.DVDuration{Value: "P1Y2M3DT4H5M6S"}, nil},
		{"DV_DURATION week mixed with day", &rm.DVDuration{Value: "P1W2D"}, nil},
		{"DV_DURATION negative", &rm.DVDuration{Value: "-P1D"}, nil},
		{"DV_DATE_TIME by value", rm.DVDateTime{Value: "example"}, invalid},
		{"DV_DATE_TIME empty, by value", rm.DVDateTime{}, empty},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := validation.ValidateRM(tc.root)
			if got := findingsOf(r); !slices.Equal(got, tc.want) {
				t.Errorf("ValidateRM(%T %+v) findings (code path) = %q, want %q; issues=%+v", tc.root, tc.root, got, tc.want, r.Issues)
			}
			if wantOK := len(tc.want) == 0; r.OK != wantOK {
				t.Errorf("ValidateRM(%T %+v).OK = %v, want %v; issues=%+v", tc.root, tc.root, r.OK, wantOK, r.Issues)
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

// TestREQ112_TemporalJSONNullValue pins what the floor reports for a
// temporal data value whose value is JSON null, decoded through canjson. The
// null decodes to the empty string, an absent RM-mandatory attribute, which
// REQ-112 reports once, as required at /value/value; Value_valid does not
// report it again at /value.
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
			want := []string{"required /value/value"}
			if !slices.Equal(got, want) {
				t.Errorf("ValidateRM(%s) issues (code path) = %q, want %q; issues=%+v", body, got, want, r.Issues)
			}
		})
	}
}

// dateTimeValidityTemplate is the corpus template whose ELEMENT at0023
// holds a DV_DATE_TIME in every event.
const dateTimeValidityTemplate = "Test_dv_date_time_validity_kind_constraint_v0"

// blankDateTimeValues sets to the empty string the value of every
// DV_DATE_TIME an at0023 ELEMENT of raw holds. It reports how many it
// edited.
func blankDateTimeValues(t *testing.T, raw []byte) ([]byte, int) {
	t.Helper()
	var doc any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	n := 0
	var walk func(v any)
	walk = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			if x["_type"] == "ELEMENT" && x["archetype_node_id"] == "at0023" {
				if dv, ok := x["value"].(map[string]any); ok && dv["_type"] == "DV_DATE_TIME" {
					dv["value"] = ""
					n++
				}
			}
			for _, child := range x {
				walk(child)
			}
		case []any:
			for _, child := range x {
				walk(child)
			}
		}
	}
	walk(doc)
	out, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("re-encode fixture: %v", err)
	}
	return out, n
}

// TestValidateComposition_REQ102_REQ112_EmptyTemporalValueOnce pins the
// REQ-112 Value_valid entry under a template: a DV_DATE_TIME whose value is
// blank is an absent mandatory attribute, reported once, as required at
// …/value/value, and Value_valid reports nothing at the DV_DATE_TIME. The
// corpus composition is valid as it stands, so every issue comes from a
// blanked value.
func TestValidateComposition_REQ102_REQ112_EmptyTemporalValueOnce(t *testing.T) {
	c := mustCompile(t, dateTimeValidityTemplate)
	raw, err := os.ReadFile(fixtures.CompositionJSON(dateTimeValidityTemplate))
	if err != nil {
		t.Fatal(err)
	}
	var orig rm.Composition
	if err := canjson.Unmarshal(raw, &orig); err != nil {
		t.Fatalf("decode composition: %v", err)
	}
	if r := validation.ValidateComposition(&orig, c); !r.OK {
		t.Fatalf("ValidateComposition(%s) as vendored: issues = %+v, want none", dateTimeValidityTemplate, r.Issues)
	}
	edited, n := blankDateTimeValues(t, raw)
	if n == 0 {
		t.Fatal("the fixture holds no at0023 DV_DATE_TIME to blank")
	}
	var comp rm.Composition
	if err := canjson.Unmarshal(edited, &comp); err != nil {
		t.Fatalf("decode composition: %v", err)
	}
	r := validation.ValidateComposition(&comp, c)
	required := 0
	for _, iss := range r.Issues {
		switch {
		case iss.Code == "required" && strings.HasSuffix(iss.Path, "/items[at0023]/value/value"):
			required++
		default:
			t.Errorf("ValidateComposition(%s, %d DV_DATE_TIME values blanked) carries %s at %q (layer %s, rule %s), want only required at …/items[at0023]/value/value", dateTimeValidityTemplate, n, iss.Code, iss.Path, iss.Layer, iss.Rule)
		}
	}
	if required != n {
		t.Errorf("ValidateComposition(%s, %d DV_DATE_TIME values blanked) reports %d required at …/items[at0023]/value/value, want %d; issues = %q", dateTimeValidityTemplate, n, required, n, identityKeys(r.Issues))
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
		{"both, by value in CLUSTER.items", inCluster(both()), "/items[at0001]"},
		{"neither, by value in CLUSTER.items", inCluster(validElement()), "/items[at0001]"},
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
