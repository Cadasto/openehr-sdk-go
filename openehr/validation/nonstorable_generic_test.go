package validation_test

import (
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/aom/aom14"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
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
	opt, err := aom14.ReadOperationalTemplate(strings.NewReader(body))
	if err != nil {
		t.Fatalf("ReadOperationalTemplate: %v", err)
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

// untypedEventOffsetOPT is a HISTORY template whose one event names the RM
// type eventType, or none where eventType is empty, and constrains the
// event's offset with an existence of 1..1.
func untypedEventOffsetOPT(eventType string) string {
	required := `<existence><lower_included>true</lower_included><upper_included>true</upper_included>` +
		`<lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded>` +
		`<lower>1</lower><upper>1</upper></existence>`
	return `<?xml version="1.0" encoding="UTF-8"?>
<template xmlns="http://schemas.openehr.org/v1" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <template_id><value>untyped_event_offset</value></template_id>
  <concept>untyped_event_offset</concept>
  <definition xsi:type="C_COMPLEX_OBJECT">
    <rm_type_name>HISTORY</rm_type_name>
    <node_id>at0001</node_id>
    <attributes xsi:type="C_MULTIPLE_ATTRIBUTE">
      <rm_attribute_name>events</rm_attribute_name>
      <children xsi:type="C_COMPLEX_OBJECT">
        <rm_type_name>` + eventType + `</rm_type_name>
        <node_id>at0002</node_id>
        <attributes xsi:type="C_SINGLE_ATTRIBUTE">
          <rm_attribute_name>offset</rm_attribute_name>` + required + `
          <children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>DV_DURATION</rm_type_name><node_id></node_id></children>
        </attributes>
      </children>
      <cardinality><is_ordered>true</is_ordered><is_unique>false</is_unique><interval>
        <lower_included>true</lower_included><lower_unbounded>false</lower_unbounded>
        <upper_unbounded>true</upper_unbounded><lower>0</lower></interval></cardinality>
    </attributes>
  </definition>
</template>`
}

// TestREQ111_UntypedEventOffsetIsNotRequired is the REQ-111 check that the
// template validator reads a node that names no RM type as the compile
// reads it, the type of the attribute that holds it (A node that names no
// RM type): an untyped child of a HISTORY's events is an EVENT, whose
// offset the RM computes and no serialization carries, so an existence of
// 1..1 on it draws no required issue on a POINT_EVENT, as it draws none
// where the template names POINT_EVENT. The typed spelling is the control.
func TestREQ111_UntypedEventOffsetIsNotRequired(t *testing.T) {
	history := &rm.History[rm.ItemStructure]{
		ArchetypeNodeID: "at0001",
		Name:            &rm.DVText{Value: "history"},
		Origin:          rm.DVDateTime{Value: "2026-05-24T10:00:00Z"},
		Events: []rm.Event{&rm.PointEvent[rm.ItemStructure]{
			ArchetypeNodeID: "at0002",
			Name:            rm.DVText{Value: "any event"},
			Time:            rm.DVDateTime{Value: "2026-05-24T10:00:00Z"},
			Data:            &rm.ItemTree{ArchetypeNodeID: "at0003", Name: rm.DVText{Value: "tree"}},
		}},
	}
	for _, eventType := range []string{"POINT_EVENT", ""} {
		t.Run("event type "+strconv.Quote(eventType), func(t *testing.T) {
			opt, err := aom14.ReadOperationalTemplate(strings.NewReader(untypedEventOffsetOPT(eventType)))
			if err != nil {
				t.Fatalf("ReadOperationalTemplate: %v", err)
			}
			c, err := templatecompile.Compile(opt)
			if err != nil {
				t.Fatalf("Compile: %v", err)
			}
			for _, is := range validation.Validate(history, c).Issues {
				t.Errorf("Validate: %s @ %s: %s", is.Code, is.Path, is.Detail)
			}
		})
	}
}
