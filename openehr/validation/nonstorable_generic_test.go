package validation_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// REQ-112 — EVENT.offset is computed, never stored, so the template-driven
// walker must not demand it even where the OPT constrains it as mandatory.
// That holds however the OPT spells the event's class: here as the generic
// instantiation POINT_EVENT<ITEM_TREE>, which the non-storable check must
// read as the bare class POINT_EVENT.
func TestValidate_genericEventOffsetIsNotRequired(t *testing.T) {
	const body = `<?xml version="1.0" encoding="UTF-8"?>
<template xmlns="http://schemas.openehr.org/v1" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <template_id><value>generic_event_offset</value></template_id>
  <concept>generic_event_offset</concept>
  <definition xsi:type="C_COMPLEX_OBJECT">
    <rm_type_name>POINT_EVENT&lt;ITEM_TREE&gt;</rm_type_name>
    <node_id>at0002</node_id>
    <attributes xsi:type="C_SINGLE_ATTRIBUTE">
      <rm_attribute_name>offset</rm_attribute_name>
      <existence>
        <lower_included>true</lower_included>
        <upper_included>true</upper_included>
        <lower_unbounded>false</lower_unbounded>
        <upper_unbounded>false</upper_unbounded>
        <lower>1</lower>
        <upper>1</upper>
      </existence>
      <children xsi:type="C_COMPLEX_OBJECT">
        <rm_type_name>DV_DURATION</rm_type_name>
        <node_id></node_id>
      </children>
    </attributes>
  </definition>
</template>`
	opt, err := template.ParseOPT(strings.NewReader(body))
	if err != nil {
		t.Fatalf("ParseOPT: %v", err)
	}
	c, err := templatecompile.Compile(opt)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	event := &rm.PointEvent[rm.ItemStructure]{
		ArchetypeNodeID: "at0002",
		Name:            rm.DVText{Value: "any event"},
		Time:            rm.DVDateTime{Value: "2026-05-24T10:00:00Z"},
		Data:            &rm.ItemTree{ArchetypeNodeID: "at0001", Name: rm.DVText{Value: "tree"}},
	}

	// The whole issue list is asserted, so nothing hides beside the offset
	// check. The one issue at / is the type check's own, known gap: it matches a
	// generic spelling only for DV_INTERVAL, so POINT_EVENT<ITEM_TREE> does not
	// admit a POINT_EVENT value. That gap is tracked separately and is not what
	// this test pins; a required issue at /offset is.
	type issue struct{ path, code, detail string }
	want := []issue{{
		path:   "/",
		code:   "rm_type_mismatch",
		detail: "RM type POINT_EVENT does not satisfy template RM type POINT_EVENT<ITEM_TREE> at /",
	}}
	var got []issue
	for _, is := range validation.Validate(event, c).Issues {
		got = append(got, issue{path: is.Path, code: is.Code, detail: is.Detail})
	}
	if !slices.Equal(got, want) {
		t.Errorf("Validate issues = %+v, want %+v (offset is non-storable and must be skipped)", got, want)
	}
}
