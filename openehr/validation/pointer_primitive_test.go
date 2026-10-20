package validation_test

import (
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// REQ-102 (Validation dimensions, Primitive constraints) — an optional
// primitive attribute the RM holds by pointer binds the template's primitive
// node as the value it points to, and that node's constraint judges the
// value: a valid one validates, and one outside the constraint is the
// constraint's own finding, never an rm_type_mismatch. The pointers are those
// the generated access hands over for the optional attributes of the
// quantified types and of INTERVAL_EVENT: an *rm.Integer precision and
// sample_count, an *rm.Real accuracy and a *bool accuracy_is_percent. The
// DV_PROPORTION precision rows are the control: the generated access hands
// that pointer over dereferenced.

// primitiveAttrChild returns an OPT child of RM type rmType whose attribute
// attr holds the AOM 1.4 short-name node shortName, constrained by a
// primitive item of type itemType with body.
func primitiveAttrChild(rmType, attr, shortName, itemType, body string) string {
	return `
      <children xsi:type="C_COMPLEX_OBJECT">
        <rm_type_name>` + rmType + `</rm_type_name>
        <node_id></node_id>
        <attributes xsi:type="C_SINGLE_ATTRIBUTE">
          <rm_attribute_name>` + attr + `</rm_attribute_name>` + shortNameChild(shortName, itemType, body) + `
        </attributes>
      </children>`
}

// Primitive item bodies for the pointer rows.
const (
	// integerZeroToFive admits an Integer from 0 to 5.
	integerZeroToFive = `<range><lower_included>true</lower_included><upper_included>true</upper_included>` +
		`<lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded>` +
		`<lower>0</lower><upper>5</upper></range>`
	// realZeroToOne admits a Real from 0 to 1.
	realZeroToOne = `<range><lower_included>true</lower_included><upper_included>true</upper_included>` +
		`<lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded>` +
		`<lower>0.0</lower><upper>1.0</upper></range>`
	// trueOnly admits true and refuses false.
	trueOnly = `<true_valid>true</true_valid><false_valid>false</false_valid>`
)

// intervalEventOPT returns an INTERVAL_EVENT template whose sample_count
// holds an INTEGER node constrained by integerZeroToFive.
func intervalEventOPT() string {
	return `<?xml version="1.0"?>
<template xmlns="http://schemas.openehr.org/v1" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <template_id><value>interval-event</value></template_id>
  <concept>interval-event</concept>
  <language><terminology_id><value>ISO_639-1</value></terminology_id><code_string>en</code_string></language>
  <definition>
    <rm_type_name>INTERVAL_EVENT</rm_type_name>
    <node_id>at0000</node_id>
    <attributes xsi:type="C_SINGLE_ATTRIBUTE">
      <rm_attribute_name>sample_count</rm_attribute_name>` + shortNameChild("INTEGER", "C_INTEGER", integerZeroToFive) + `
    </attributes>
    <archetype_id><value>openEHR-EHR-INTERVAL_EVENT.interval_event.v1</value></archetype_id>
  </definition>
</template>`
}

// REQ-102 — each row validates an ELEMENT whose value holds a pointer-held
// primitive under a template primitive constraint, and pins the template's
// findings: none for a value the constraint admits, its out_of_range or
// not_in_list at the attribute for one it refuses. A pointer that is not set
// is an absent attribute: the constraint judges no value and the rows report
// nothing.
func TestValidate_REQ102_PointerHeldPrimitiveBindsAsItsValue(t *testing.T) {
	t.Parallel()
	const (
		precision = "/value/precision"
		accuracy  = "/value/accuracy"
		percent   = "/value/accuracy_is_percent"
	)
	tests := []struct {
		name  string
		child string
		value rm.DataValue
		want  []pinnedIssue
	}{
		{
			name:  "DV_QUANTITY precision under C_INTEGER, inside",
			child: primitiveAttrChild("DV_QUANTITY", "precision", "INTEGER", "C_INTEGER", integerZeroToFive),
			value: &rm.DVQuantity{Magnitude: 1, Units: "kg", Precision: new(rm.Integer(2))},
		},
		{
			name:  "DV_QUANTITY precision under C_INTEGER, outside",
			child: primitiveAttrChild("DV_QUANTITY", "precision", "INTEGER", "C_INTEGER", integerZeroToFive),
			value: &rm.DVQuantity{Magnitude: 1, Units: "kg", Precision: new(rm.Integer(7))},
			want:  []pinnedIssue{templateFinding("out_of_range", precision)},
		},
		{
			name:  "DV_QUANTITY accuracy under C_REAL, inside",
			child: primitiveAttrChild("DV_QUANTITY", "accuracy", "REAL", "C_REAL", realZeroToOne),
			value: &rm.DVQuantity{Magnitude: 1, Units: "kg", Accuracy: new(rm.Real(0.5))},
		},
		{
			name:  "DV_COUNT accuracy under C_REAL, inside",
			child: primitiveAttrChild("DV_COUNT", "accuracy", "REAL", "C_REAL", realZeroToOne),
			value: &rm.DVCount{Magnitude: 3, Accuracy: new(rm.Real(0.5))},
		},
		{
			name:  "DV_COUNT accuracy under C_REAL, outside",
			child: primitiveAttrChild("DV_COUNT", "accuracy", "REAL", "C_REAL", realZeroToOne),
			value: &rm.DVCount{Magnitude: 3, Accuracy: new(rm.Real(2.5))},
			want:  []pinnedIssue{templateFinding("out_of_range", accuracy)},
		},
		{
			name:  "DV_COUNT accuracy_is_percent under C_BOOLEAN, admitted",
			child: primitiveAttrChild("DV_COUNT", "accuracy_is_percent", "BOOLEAN", "C_BOOLEAN", trueOnly),
			value: &rm.DVCount{Magnitude: 3, Accuracy: new(rm.Real(0.5)), AccuracyIsPercent: new(true)},
		},
		{
			name:  "DV_COUNT accuracy_is_percent under C_BOOLEAN, refused",
			child: primitiveAttrChild("DV_COUNT", "accuracy_is_percent", "BOOLEAN", "C_BOOLEAN", trueOnly),
			value: &rm.DVCount{Magnitude: 3, Accuracy: new(rm.Real(0.5)), AccuracyIsPercent: new(false)},
			want:  []pinnedIssue{templateFinding("not_in_list", percent)},
		},
		{
			name:  "DV_DURATION accuracy under C_REAL, inside",
			child: primitiveAttrChild("DV_DURATION", "accuracy", "REAL", "C_REAL", realZeroToOne),
			value: &rm.DVDuration{Value: "PT1H", Accuracy: new(rm.Real(0.5))},
		},
		{
			name:  "DV_QUANTITY precision not set under C_INTEGER, absent",
			child: primitiveAttrChild("DV_QUANTITY", "precision", "INTEGER", "C_INTEGER", integerZeroToFive),
			value: &rm.DVQuantity{Magnitude: 1, Units: "kg"},
		},
		{
			name:  "DV_COUNT accuracy not set under C_REAL, absent",
			child: primitiveAttrChild("DV_COUNT", "accuracy", "REAL", "C_REAL", realZeroToOne),
			value: &rm.DVCount{Magnitude: 3},
		},
		{
			name:  "DV_COUNT accuracy_is_percent not set under C_BOOLEAN, absent",
			child: primitiveAttrChild("DV_COUNT", "accuracy_is_percent", "BOOLEAN", "C_BOOLEAN", trueOnly),
			value: &rm.DVCount{Magnitude: 3},
		},
		{
			name:  "DV_PROPORTION precision under C_INTEGER, inside",
			child: primitiveAttrChild("DV_PROPORTION", "precision", "INTEGER", "C_INTEGER", integerZeroToFive),
			value: &rm.DVProportion{Numerator: 1, Denominator: 2, Type: 0, Precision: new(rm.Integer(2))},
		},
		{
			name:  "DV_PROPORTION precision under C_INTEGER, outside",
			child: primitiveAttrChild("DV_PROPORTION", "precision", "INTEGER", "C_INTEGER", integerZeroToFive),
			value: &rm.DVProportion{Numerator: 1, Denominator: 2, Type: 0, Precision: new(rm.Integer(7))},
			want:  []pinnedIssue{templateFinding("out_of_range", precision)},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := mustCompileInline(t, elementValueOPT(tc.child))
			got := pinnedIssues(validation.Validate(valueElement(tc.value), c).Issues)
			if !slices.Equal(got, tc.want) {
				t.Errorf("Validate(%s) issues =\n  %+v\nwant\n  %+v", tc.name, got, tc.want)
			}
		})
	}
}

// REQ-102 — an INTERVAL_EVENT sample_count, an *rm.Integer, binds the
// template's INTEGER node as its value: a count the C_INTEGER admits
// validates, and one it refuses is its out_of_range, not an rm_type_mismatch.
func TestValidate_REQ102_PointerHeldSampleCountBindsAsItsValue(t *testing.T) {
	t.Parallel()
	c := mustCompileInline(t, intervalEventOPT())
	event := func(count rm.Integer) *rm.IntervalEvent[rm.ItemStructure] {
		return &rm.IntervalEvent[rm.ItemStructure]{
			ArchetypeNodeID: "openEHR-EHR-INTERVAL_EVENT.interval_event.v1",
			Name:            &rm.DVText{Value: "event"},
			Data:            &rm.ItemTree{ArchetypeNodeID: "at0001", Name: rm.DVText{Value: "data"}},
			MathFunction:    codedTextOf(openEHRCode("146")),
			Time:            rm.DVDateTime{Value: "2026-01-01T10:00:00Z"},
			Width:           rm.DVDuration{Value: "PT1H"},
			SampleCount:     new(count),
		}
	}
	tests := []struct {
		name  string
		count rm.Integer
		want  []pinnedIssue
	}{
		{name: "inside", count: 2},
		{name: "outside", count: 7, want: []pinnedIssue{templateFinding("out_of_range", "/sample_count")}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := pinnedIssues(validation.Validate(event(tc.count), c).Issues)
			if !slices.Equal(got, tc.want) {
				t.Errorf("Validate(INTERVAL_EVENT sample_count %d) issues =\n  %+v\nwant\n  %+v", tc.count, got, tc.want)
			}
		})
	}
}
