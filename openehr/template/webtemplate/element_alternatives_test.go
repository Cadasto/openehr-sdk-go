package webtemplate_test

// REQ-106 — what an ELEMENT gives by the value types its template admits.
//
// One type gives a single value leaf. One DV_CODED_TEXT and one DV_TEXT, in
// either order, give that leaf as a coded text with a free-text `other` input.
// Any other set of two or more types gives the ELEMENT as a node with one child
// per type, and an ELEMENT that constrains no value type gives a node with one
// child per value type the reference offers such an ELEMENT.

import (
	"bytes"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/template/webtemplate"
)

const (
	// alternativeTypesOPT's one ELEMENT, at0004, admits DV_QUANTITY, DV_COUNT
	// or DV_ORDINAL; withValueAlternatives replaces those.
	alternativeTypesOPT = "../../../testkit/corpus/templates/alternative_types.en.v1.opt"
	// alternativeTypesElement is the aqlPath of that ELEMENT.
	alternativeTypesElement = "/content[openEHR-EHR-OBSERVATION.alternative_types.v1]/data[at0001]/events[at0002]/data[at0003]/items[at0004]"
)

// withValueAlternatives returns the alternative_types OPT with the value
// alternatives of its ELEMENT replaced by one unconstrained alternative of each
// of rmTypes, in order, each occurring exactly once where the ELEMENT occurs
// 0..1. With no rmTypes the ELEMENT constrains no value at all.
func withValueAlternatives(t *testing.T, rmTypes ...string) []byte {
	t.Helper()
	raw := readFile(t, alternativeTypesOPT)
	const (
		attrOpen  = `<attributes xsi:type="C_SINGLE_ATTRIBUTE">`
		valueName = "<rm_attribute_name>value</rm_attribute_name>"
		existEnd  = "</existence>"
		attrEnd   = "</attributes>"
	)
	if n := bytes.Count(raw, []byte(valueName)); n != 1 {
		t.Fatalf("%s names a value attribute %d times, want once", alternativeTypesOPT, n)
	}
	name := bytes.Index(raw, []byte(valueName))
	start := bytes.LastIndex(raw[:name], []byte(attrOpen))
	body := name + bytes.Index(raw[name:], []byte(existEnd)) + len(existEnd)
	end := name + bytes.Index(raw[name:], []byte(attrEnd))
	if start < 0 || body < name || end < body {
		t.Fatalf("%s: the value attribute is not laid out as expected", alternativeTypesOPT)
	}
	if len(rmTypes) == 0 {
		return slices.Concat(raw[:start], raw[end+len(attrEnd):])
	}
	escape := strings.NewReplacer("<", "&lt;", ">", "&gt;")
	var alts strings.Builder
	for _, rmType := range rmTypes {
		fmt.Fprintf(&alts, `<children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>%s</rm_type_name>`+
			`<occurrences><lower_included>true</lower_included><upper_included>true</upper_included>`+
			`<lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded>`+
			`<lower>1</lower><upper>1</upper></occurrences><node_id/></children>`, escape.Replace(rmType))
	}
	return slices.Concat(raw[:body], []byte(alts.String()), raw[end:])
}

// elementNodes returns the nodes the ELEMENT at path gives: a node at path, or
// a leaf at path/value that is no child of a node at path.
func elementNodes(wt *webtemplate.WebTemplate, path string) []*webtemplate.Node {
	var out []*webtemplate.Node
	var walk func(parent, n *webtemplate.Node)
	walk = func(parent, n *webtemplate.Node) {
		switch {
		case n.AQLPath == path:
			out = append(out, n)
		case n.AQLPath == path+"/value" && (parent == nil || parent.AQLPath != path):
			out = append(out, n)
		}
		for _, ch := range n.Children {
			walk(n, ch)
		}
	}
	walk(nil, wt.Tree)
	return out
}

// TestREQ106ElementValueAlternatives — REQ-106. The node an ELEMENT gives
// depends on the value types its template admits; the cases replace the value
// alternatives of one ELEMENT, which occurs 0..1, with alternatives that each
// occur exactly once.
func TestREQ106ElementValueAlternatives(t *testing.T) {
	t.Parallel()
	en := func(s string) map[string]string { return map[string]string{"en": s} }
	const name = "Alternative types"
	element := func(children ...*webtemplate.Node) *webtemplate.Node {
		return &webtemplate.Node{
			ID: "alternative_types", Name: name, LocalizedName: name, RMType: "ELEMENT", NodeID: "at0004",
			Min: 0, Max: 1, LocalizedNames: en(name), LocalizedDescriptions: en("*"),
			AQLPath: alternativeTypesElement, Children: children,
		}
	}
	// value is a node at the ELEMENT's value, named as the ELEMENT is.
	value := func(id, rmType, nodeID string, minOcc int, inputs ...webtemplate.Input) *webtemplate.Node {
		return &webtemplate.Node{
			ID: id, Name: name, LocalizedName: name, RMType: rmType, NodeID: nodeID,
			Min: minOcc, Max: 1, LocalizedNames: en(name), LocalizedDescriptions: en("*"),
			AQLPath: alternativeTypesElement + "/value", Inputs: inputs,
		}
	}
	// choice is the child an alternative gives: its own occurrences, 1..1 here.
	choice := func(id, rmType string, inputs ...webtemplate.Input) *webtemplate.Node {
		return value(id, rmType, "at0004", 1, inputs...)
	}
	// anyValue is the child an ELEMENT that constrains no value type gives each
	// value type: no node id, and 0..1.
	anyValue := func(id, rmType string, inputs ...webtemplate.Input) *webtemplate.Node {
		return value(id, rmType, "", 0, inputs...)
	}
	text := webtemplate.Input{Type: "TEXT"}
	code := webtemplate.Input{Suffix: "code", Type: "CODED_TEXT", ListOpen: true}
	other := webtemplate.Input{Suffix: "other", Type: "TEXT"}
	quantity := []webtemplate.Input{{Suffix: "magnitude", Type: "DECIMAL"}, {Suffix: "unit", Type: "CODED_TEXT"}}
	count := webtemplate.Input{Type: "INTEGER"}
	var duration []webtemplate.Input
	for _, f := range []string{"year", "month", "day", "week", "hour", "minute", "second"} {
		duration = append(duration, webtemplate.Input{Suffix: f, Type: "INTEGER"})
	}
	proportion := []webtemplate.Input{{Suffix: "numerator", Type: "DECIMAL"}, {Suffix: "denominator", Type: "DECIMAL"}}

	tests := []struct {
		name    string
		rmTypes []string
		want    *webtemplate.Node
	}{
		{
			name:    "one type gives a leaf",
			rmTypes: []string{"DV_COUNT"},
			want:    value("alternative_types", "DV_COUNT", "at0004", 0, count),
		},
		{
			name:    "coded text then text give the coded leaf",
			rmTypes: []string{"DV_CODED_TEXT", "DV_TEXT"},
			want:    value("alternative_types", "DV_CODED_TEXT", "at0004", 0, code, other),
		},
		{
			name:    "text then coded text give the coded leaf",
			rmTypes: []string{"DV_TEXT", "DV_CODED_TEXT"},
			want:    value("alternative_types", "DV_CODED_TEXT", "at0004", 0, code, other),
		},
		{
			name:    "several types give a child each",
			rmTypes: []string{"DV_QUANTITY", "DV_COUNT"},
			want:    element(choice("quantity_value", "DV_QUANTITY", quantity...), choice("count_value", "DV_COUNT", count)),
		},
		{
			name:    "the pair and a third type give a child each",
			rmTypes: []string{"DV_CODED_TEXT", "DV_TEXT", "DV_BOOLEAN"},
			want: element(choice("coded_text_value", "DV_CODED_TEXT", code), choice("text_value", "DV_TEXT", text),
				choice("boolean_value", "DV_BOOLEAN", webtemplate.Input{Type: "BOOLEAN"})),
		},
		{
			name:    "two texts give a child each",
			rmTypes: []string{"DV_TEXT", "DV_TEXT"},
			want:    element(choice("text_value", "DV_TEXT", text), choice("text_value2", "DV_TEXT", text)),
		},
		{
			name:    "a type given twice takes the next free id",
			rmTypes: []string{"DV_COUNT", "DV_QUANTITY", "DV_COUNT"},
			want: element(choice("count_value", "DV_COUNT", count), choice("quantity_value", "DV_QUANTITY", quantity...),
				choice("count_value2", "DV_COUNT", count)),
		},
		{
			name:    "an interval names its value type",
			rmTypes: []string{"DV_DATE_TIME", "DV_INTERVAL<DV_DATE_TIME>"},
			want: element(choice("date_time_value", "DV_DATE_TIME", webtemplate.Input{Type: "DATETIME"}),
				choice("interval_of_date_time_value", "DV_INTERVAL<DV_DATE_TIME>")),
		},
		{
			name: "no value type gives every type the reference offers",
			want: element(
				anyValue("text_value", "DV_TEXT", text),
				anyValue("coded_text_value", "DV_CODED_TEXT", code),
				anyValue("multimedia_value", "DV_MULTIMEDIA"),
				anyValue("parsable_value", "DV_PARSABLE"),
				anyValue("state_value", "DV_STATE"),
				anyValue("boolean_value", "DV_BOOLEAN", webtemplate.Input{Type: "BOOLEAN"}),
				anyValue("identifier_value", "DV_IDENTIFIER"),
				anyValue("uri_value", "DV_URI"),
				anyValue("ehr_uri_value", "DV_EHR_URI"),
				anyValue("duration_value", "DV_DURATION", duration...),
				anyValue("quantity_value", "DV_QUANTITY", quantity...),
				anyValue("count_value", "DV_COUNT", count),
				anyValue("proportion_value", "DV_PROPORTION", proportion...),
				anyValue("date_time_value", "DV_DATE_TIME", webtemplate.Input{Type: "DATETIME"}),
				anyValue("time_value", "DV_TIME", webtemplate.Input{Type: "TIME"}),
				anyValue("ordinal_value", "DV_ORDINAL", webtemplate.Input{Type: "CODED_TEXT"}),
				anyValue("date_value", "DV_DATE", webtemplate.Input{Type: "DATE"}),
			),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := elementNodes(buildOPT(t, withValueAlternatives(t, tc.rmTypes...)), alternativeTypesElement)
			if len(got) != 1 {
				t.Fatalf("value types %v: the ELEMENT gives %d nodes, want 1", tc.rmTypes, len(got))
			}
			if !reflect.DeepEqual(got[0], tc.want) {
				t.Errorf("value types %v: the ELEMENT gives\n    %s\nwant\n    %s", tc.rmTypes, jsonOf(t, got[0]), jsonOf(t, tc.want))
			}
		})
	}
}

// TestREQ106ElementAlternativesInTheCorpus — REQ-106. In the FLAT conformance
// template's CLUSTER, the labresult ELEMENT, which admits DV_TEXT or
// DV_QUANTITY, gives a child for each, and the ELEMENT that constrains no value
// type gives a child for each value type the reference offers, in the
// reference's order.
func TestREQ106ElementAlternativesInTheCorpus(t *testing.T) {
	t.Parallel()
	wt := buildOPT(t, readFile(t, conformanceOPT))
	const cluster = "/items[openEHR-EHR-CLUSTER.conformance_cluster.v0]"
	tests := []struct {
		element string
		want    []string
	}{
		{element: cluster + "/items[at0003]", want: []string{"text_value:DV_TEXT", "quantity_value:DV_QUANTITY"}},
		{element: cluster + "/items[at0005]", want: []string{
			"text_value:DV_TEXT", "coded_text_value:DV_CODED_TEXT", "multimedia_value:DV_MULTIMEDIA",
			"parsable_value:DV_PARSABLE", "state_value:DV_STATE", "boolean_value:DV_BOOLEAN",
			"identifier_value:DV_IDENTIFIER", "uri_value:DV_URI", "ehr_uri_value:DV_EHR_URI",
			"duration_value:DV_DURATION", "quantity_value:DV_QUANTITY", "count_value:DV_COUNT",
			"proportion_value:DV_PROPORTION", "date_time_value:DV_DATE_TIME", "time_value:DV_TIME",
			"ordinal_value:DV_ORDINAL", "date_value:DV_DATE",
		}},
	}
	for _, tc := range tests {
		var found []*webtemplate.Node
		walkOurTree(wt.Tree, func(n *webtemplate.Node) {
			if n.RMType == "ELEMENT" && strings.HasSuffix(n.AQLPath, tc.element) {
				found = append(found, n)
			}
		})
		if len(found) != 1 {
			t.Errorf("%s: %d ELEMENT nodes end in %s, want 1", wt.TemplateID, len(found), tc.element)
			continue
		}
		var got []string
		for _, ch := range found[0].Children {
			got = append(got, ch.ID+":"+ch.RMType)
		}
		if !slices.Equal(got, tc.want) {
			t.Errorf("ELEMENT %s children = %v, want %v", found[0].AQLPath, got, tc.want)
		}
	}
}

// TestREQ106ElementSlotGivesNoNode — REQ-106. An unfilled slot for an ELEMENT
// archetype is no ELEMENT that constrains no value: it gives no node, as any
// unfilled slot gives none, and as the reference skips it. Demonstration.v1
// has four such slots, all at0020, and 13 ELEMENTs that constrain no value.
func TestREQ106ElementSlotGivesNoNode(t *testing.T) {
	t.Parallel()
	wt := buildOPT(t, readFile(t, "../../../testkit/corpus/templates/Demonstration.v1.opt"))
	var slots []string
	unconstrained := 0
	walkOurTree(wt.Tree, func(n *webtemplate.Node) {
		if n.NodeID == "at0020" {
			slots = append(slots, n.AQLPath+" ("+n.RMType+")")
		}
		if n.RMType == "ELEMENT" && len(n.Children) == 17 {
			unconstrained++
		}
	})
	if len(slots) != 0 {
		t.Errorf("%s gives nodes for its unfilled ELEMENT slots: %v", wt.TemplateID, slots)
	}
	if unconstrained != 13 {
		t.Errorf("%s gives %d ELEMENT nodes with a child per value type, want 13", wt.TemplateID, unconstrained)
	}
}
