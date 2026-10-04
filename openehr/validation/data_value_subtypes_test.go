package validation_test

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// dataValueSlotOPT is an ELEMENT archetype whose value the template declares
// only as the abstract DATA_VALUE, so any data value type fits it.
const dataValueSlotOPT = `<?xml version="1.0"?>
<template xmlns="http://schemas.openehr.org/v1" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <template_id><value>data-value-slot</value></template_id>
  <concept>data-value-slot</concept>
  <language><terminology_id><value>ISO_639-1</value></terminology_id><code_string>en</code_string></language>
  <definition>
    <rm_type_name>ELEMENT</rm_type_name>
    <node_id>at0000</node_id>
    <attributes xsi:type="C_SINGLE_ATTRIBUTE">
      <rm_attribute_name>value</rm_attribute_name>
      <existence>
        <lower_included>true</lower_included><upper_included>true</upper_included>
        <lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded>
        <lower>1</lower><upper>1</upper>
      </existence>
      <children xsi:type="C_COMPLEX_OBJECT">
        <rm_type_name>DATA_VALUE</rm_type_name>
        <occurrences>
          <lower_included>true</lower_included><upper_included>true</upper_included>
          <lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded>
          <lower>1</lower><upper>1</upper>
        </occurrences>
        <node_id></node_id>
      </children>
    </attributes>
    <archetype_id><value>openEHR-EHR-ELEMENT.data_value_slot.v1</value></archetype_id>
  </definition>
</template>`

// An OPT node declared as the abstract DATA_VALUE admits every concrete
// DATA_VALUE descendant the pinned BMM (openehr_rm_1.2.0) defines. The rows
// are the descendants the type check used to refuse with a false
// rm_type_mismatch; a parameterised interval is admitted by its class.
func TestDataValueSlotAdmitsEveryConcreteDescendant(t *testing.T) {
	t.Parallel()
	c := mustCompileInline(t, dataValueSlotOPT)
	cases := []struct {
		name  string
		value rm.DataValue
	}{
		{name: "DV_INTERVAL", value: &rm.DVInterval[rm.DVOrdered]{LowerUnbounded: true, UpperUnbounded: true}},
		{name: "DV_INTERVAL<DV_COUNT>", value: &rm.DVInterval[rm.DVCount]{
			Lower: rm.DVCount{Magnitude: 1},
			Upper: rm.DVCount{Magnitude: 5},
		}},
		{name: "DV_PROPORTION", value: &rm.DVProportion{Numerator: 1, Denominator: 2}},
		{name: "DV_MULTIMEDIA", value: &rm.DVMultimedia{}},
		{name: "DV_PARSABLE", value: &rm.DVParsable{Value: "x", Formalism: "text/plain"}},
		{name: "DV_SCALE", value: &rm.DVScale{}},
		{name: "DV_STATE", value: &rm.DVState{}},
		{name: "DV_PARAGRAPH", value: &rm.DVParagraph{}},
		{name: "DV_GENERAL_TIME_SPECIFICATION", value: &rm.DVGeneralTimeSpecification{}},
		{name: "DV_PERIODIC_TIME_SPECIFICATION", value: &rm.DVPeriodicTimeSpecification{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			el := &rm.Element{
				ArchetypeNodeID: "openEHR-EHR-ELEMENT.data_value_slot.v1",
				Name:            rm.DVText{Value: "slot"},
				Value:           tc.value,
			}
			r := validation.Validate(el, c)
			if containsCode(r.Issues, "rm_type_mismatch") {
				t.Errorf("Validate(ELEMENT with %s value) against a DATA_VALUE slot: got rm_type_mismatch, want none; issues=%+v", tc.name, r.Issues)
			}
		})
	}
}
