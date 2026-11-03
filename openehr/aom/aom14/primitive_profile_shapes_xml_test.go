package aom14_test

// REQ-176 § Shapes the pinned BMM lacks and § Round trip, and REQ-041 §
// SDK-authored root schemas: the primitives, EXPR_OPERATOR, C_DEFINED_OBJECT,
// C_DV_SCALE and C_DV_STATE take the form of the AM Release-1.4 schemas, so a
// document in that form decodes in every mode with nothing reported and
// encodes back in the schema's order.

import (
	"reflect"
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aom/aom14"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml"
)

// shapeCase is a document in the schema's form, the type it decodes into, and
// the child elements its value encodes back to.
type shapeCase struct {
	name     string
	doc      string
	into     func() any
	children []string
}

// TestREQ176_REQ041_PrimitivesTakeTheSchemaForm pins C_DATE, C_TIME,
// C_DATE_TIME and C_DURATION with a pattern and, but for C_DURATION, a
// timezone_validity in place of the BMM's validity flags, and C_STRING with
// list_open optional.
func TestREQ176_REQ041_PrimitivesTakeTheSchemaForm(t *testing.T) {
	const dateRange = `<range><lower_included>true</lower_included><upper_included>true</upper_included>` +
		`<lower_unbounded>false</lower_unbounded><upper_unbounded>true</upper_unbounded><lower>2000-01-01</lower></range>`
	cases := []shapeCase{
		{
			"C_DATE", `<c_date` + nsOpenEHR + `><pattern>yyyy-mm-??</pattern><timezone_validity>1003</timezone_validity>` + dateRange + `</c_date>`,
			func() any { return new(aom14.CDate) },
			[]string{"pattern", "timezone_validity", "range"},
		},
		{
			"C_TIME", `<c_time` + nsOpenEHR + `><pattern>HH:MM:??</pattern><timezone_validity>1001</timezone_validity><assumed_value>10:00:00</assumed_value></c_time>`,
			func() any { return new(aom14.CTime) },
			[]string{"pattern", "timezone_validity", "assumed_value"},
		},
		{
			"C_DATE_TIME", `<c_date_time` + nsOpenEHR + `><pattern>yyyy-mm-ddTHH:MM:SS</pattern><timezone_validity>1002</timezone_validity></c_date_time>`,
			func() any { return new(aom14.CDateTime) },
			[]string{"pattern", "timezone_validity"},
		},
		{
			"C_DURATION", `<c_duration` + nsOpenEHR + `><pattern>PDTH</pattern></c_duration>`,
			func() any { return new(aom14.CDuration) },
			[]string{"pattern"},
		},
		{
			"C_STRING with no list_open", `<c_string` + nsOpenEHR + `><list>a</list><list>b</list></c_string>`,
			func() any { return new(aom14.CString) },
			[]string{"list", "list"},
		},
		{
			"C_STRING with list_open", `<c_string` + nsOpenEHR + `><list>a</list><list_open>true</list_open></c_string>`,
			func() any { return new(aom14.CString) },
			[]string{"list", "list_open"},
		},
	}
	checkShapes(t, cases)

	var s aom14.CString
	if err := canxml.Unmarshal([]byte(cases[4].doc), &s, canxml.WithStrict()); err != nil || s.ListOpen != nil {
		t.Errorf("canxml.Unmarshal(C_STRING with no list_open) = %v, ListOpen %v; want nil, nil", err, s.ListOpen)
	}
	var d aom14.CDate
	if err := canxml.Unmarshal([]byte(cases[0].doc), &d, canxml.WithStrict()); err != nil ||
		d.Pattern == nil || *d.Pattern != "yyyy-mm-??" || d.TimezoneValidity == nil || *d.TimezoneValidity != aom14.TimezoneValidityKindProhibited {
		t.Errorf("canxml.Unmarshal(C_DATE) = %v, %+v; want its pattern and timezone_validity 1003", err, d)
	}
}

// TestREQ176_REQ041_OperatorAndDefinedObjectTakeTheSchemaForm pins
// EXPR_BINARY_OPERATOR with precedence_overridden written even when false,
// since the schema makes it mandatory, and C_COMPLEX_OBJECT with no
// assumed_value, which the schema declares only on the classes that type it.
func TestREQ176_REQ041_OperatorAndDefinedObjectTakeTheSchemaForm(t *testing.T) {
	checkShapes(t, []shapeCase{{
		"EXPR_BINARY_OPERATOR", operatorXML("2007"),
		func() any { return new(aom14.ExprBinaryOperator) },
		[]string{"type", "operator", "precedence_overridden"},
	}})
	b, err := canxml.Marshal(&aom14.ExprBinaryOperator{Type: "Boolean", Operator: aom14.OperatorKindMatches})
	if err != nil {
		t.Fatalf("canxml.Marshal(EXPR_BINARY_OPERATOR) = %v", err)
	}
	if got, want := childNames(t, b), []string{"type", "operator", "precedence_overridden"}; !slices.Equal(got, want) {
		t.Errorf("canxml.Marshal(EXPR_BINARY_OPERATOR, precedence not overridden) wrote %q; want %q", got, want)
	}

	for _, typ := range []reflect.Type{reflect.TypeFor[aom14.CComplexObject](), reflect.TypeFor[aom14.CArchetypeRoot](), reflect.TypeFor[aom14.CPrimitiveObject]()} {
		if _, has := typ.FieldByName("AssumedValue"); has {
			t.Errorf("%s has an AssumedValue; the schema declares none on it", typ.Name())
		}
	}
	doc := `<c_complex_object` + nsOpenEHR + `><rm_type_name>ELEMENT</rm_type_name><node_id>at0001</node_id><assumed_value>x</assumed_value></c_complex_object>`
	var reported []string
	if err := canxml.Unmarshal([]byte(doc), new(aom14.CComplexObject), canxml.WithUnmatched(func(p string) { reported = append(reported, p) })); err != nil {
		t.Fatalf("canxml.Unmarshal(C_COMPLEX_OBJECT with assumed_value, report) = %v; want nil", err)
	}
	if want := []string{"/c_complex_object[1]/assumed_value[1]"}; !slices.Equal(reported, want) {
		t.Errorf("canxml.Unmarshal(C_COMPLEX_OBJECT with assumed_value, report) reported %q; want %q, the schema declares none", reported, want)
	}
}

// TestREQ176_REQ041_ScaleAndStateTakeTheSchemaForm pins the two classes only
// the schema has: C_DV_SCALE, whose SCALE takes its value and its symbol in
// either order and writes them in the schema's, and C_DV_STATE with its state
// machine, whose states decode as the type their xsi:type names.
func TestREQ176_REQ041_ScaleAndStateTakeTheSchemaForm(t *testing.T) {
	const object = `<occurrences><lower>1</lower><upper>1</upper></occurrences><node_id/>`
	symbol := `<symbol><defining_code>` + localCode + `</defining_code></symbol>`
	transition := `<transitions><event>start</event><guard>ready</guard><next_state xsi:type="TERMINAL_STATE"><name>done</name></next_state></transitions>`
	cases := []shapeCase{
		{
			"C_DV_SCALE, value first", `<c_dv_scale` + nsOpenEHR + `><rm_type_name>DV_SCALE</rm_type_name>` + object +
				`<assumed_value><value>0.5</value>` + symbol + `</assumed_value><list><value>0.5</value>` + symbol + `</list></c_dv_scale>`,
			func() any { return new(aom14.CDVScale) },
			[]string{"rm_type_name", "occurrences", "node_id", "assumed_value", "list"},
		},
		{
			"C_DV_SCALE, symbol first", `<c_dv_scale` + nsOpenEHR + `><rm_type_name>DV_SCALE</rm_type_name>` + object +
				`<list>` + symbol + `<value>-1.5</value></list></c_dv_scale>`,
			func() any { return new(aom14.CDVScale) },
			[]string{"rm_type_name", "occurrences", "node_id", "list"},
		},
		{
			"C_DV_STATE", `<c_dv_state` + nsOpenEHR + `><rm_type_name>DV_STATE</rm_type_name>` + object +
				`<value><states xsi:type="NON_TERMINAL_STATE"><name>planned</name>` + transition + `</states>` +
				`<states xsi:type="TERMINAL_STATE"><name>done</name></states></value></c_dv_state>`,
			func() any { return new(aom14.CDVState) },
			[]string{"rm_type_name", "occurrences", "node_id", "value"},
		},
	}
	checkShapes(t, cases)

	var scale aom14.CDVScale
	if err := canxml.Unmarshal([]byte(cases[1].doc), &scale, canxml.WithStrict()); err != nil {
		t.Fatalf("canxml.Unmarshal(C_DV_SCALE, symbol first) = %v; want nil", err)
	}
	b, err := canxml.Marshal(&scale.List[0])
	if err != nil {
		t.Fatalf("canxml.Marshal(SCALE) = %v", err)
	}
	if got, want := childNames(t, b), []string{"value", "symbol"}; !slices.Equal(got, want) {
		t.Errorf("canxml.Marshal(SCALE read symbol first) wrote %q; want the schema's %q", got, want)
	}

	var state aom14.CDVState
	if err := canxml.Unmarshal([]byte(cases[2].doc), &state, canxml.WithStrict()); err != nil {
		t.Fatalf("canxml.Unmarshal(C_DV_STATE) = %v; want nil", err)
	}
	guard := "ready"
	want := aom14.StateMachine{States: []aom14.State{
		&aom14.NonTerminalState{Name: "planned", Transitions: []aom14.Transition{{Event: "start", Guard: &guard, NextState: &aom14.TerminalState{Name: "done"}}}},
		&aom14.TerminalState{Name: "done"},
	}}
	if !reflect.DeepEqual(state.Value, want) {
		t.Errorf("canxml.Unmarshal(C_DV_STATE).Value = %#v; want %#v", state.Value, want)
	}
	for _, name := range []string{"C_DV_SCALE", "SCALE", "C_DV_STATE", "STATE_MACHINE", "NON_TERMINAL_STATE", "TERMINAL_STATE", "TRANSITION"} {
		if _, ok := typereg.Default.Lookup(name); !ok {
			t.Errorf("typereg.Default has no %s, which the AOM 1.4 root declares", name)
		}
	}
}

// checkShapes decodes each case in every mode, with nothing reported, then
// encodes the value and decodes that back strictly: the encode writes the
// case's child elements in its order, and the round trip gives the value back.
func checkShapes(t *testing.T, cases []shapeCase) {
	t.Helper()
	for _, tc := range cases {
		var reported []string
		var decoded any
		for mode, opts := range decodeModes(&reported) {
			v := tc.into()
			if err := canxml.Unmarshal([]byte(tc.doc), v, opts...); err != nil {
				t.Errorf("canxml.Unmarshal(%s, %s) = %v; want nil", tc.name, mode, err)
				continue
			}
			decoded = v
		}
		if len(reported) != 0 {
			t.Errorf("canxml.Unmarshal(%s, report) reported %q; want nothing", tc.name, reported)
		}
		if decoded == nil {
			continue
		}
		b, err := canxml.Marshal(decoded)
		if err != nil {
			t.Errorf("canxml.Marshal(%s) = %v; want nil", tc.name, err)
			continue
		}
		if got := childNames(t, b); !slices.Equal(got, tc.children) {
			t.Errorf("canxml.Marshal(%s) wrote child elements %q in %s; want %q", tc.name, got, b, tc.children)
		}
		back := tc.into()
		if err := canxml.Unmarshal(b, back, canxml.WithStrict()); err != nil || !reflect.DeepEqual(back, decoded) {
			t.Errorf("round trip of %s through %s = %v; want the decoded value back", tc.name, b, err)
		}
	}
}
