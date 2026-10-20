package composition_test

import (
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/composition"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/rmpath"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// Value children for compileAlternatives that the binding reads apart from
// the class hierarchy: the AOM 1.4 primitive short name DURATION, which the
// generator writes as its canonical DV wrapper DV_DURATION; a child with no
// RM type; and typed intervals, one over the abstract DV_ORDERED and one
// over the concrete DV_QUANTITY.
const (
	durationValueChild = `<children xsi:type="C_PRIMITIVE_OBJECT"><rm_type_name>DURATION</rm_type_name><node_id></node_id>` +
		`<item xsi:type="C_DURATION"></item></children>`
	untypedValueChild    = `<children xsi:type="C_COMPLEX_OBJECT"><rm_type_name></rm_type_name><node_id></node_id></children>`
	orderedIntervalType  = "DV_INTERVAL&lt;DV_ORDERED&gt;"
	quantityIntervalType = "DV_INTERVAL&lt;DV_QUANTITY&gt;"
)

// countInterval returns a DV_INTERVAL<DV_COUNT> from 1 to 5.
func countInterval() *rm.DVInterval[rm.DVCount] {
	return &rm.DVInterval[rm.DVCount]{
		Lower: rm.DVCount{Magnitude: 1}, LowerIncluded: true,
		Upper: rm.DVCount{Magnitude: 5}, UpperIncluded: true,
	}
}

// TestREQ101_SetAcceptsWhatTheValidatorBinds checks, for REQ-101 Type
// enforcement, that Set accepts a value the validator's alternatives rule
// (REQ-102) binds at the path beyond the class hierarchy, that Build writes
// it there, and that the validator reports no finding the composition did
// not have before: a DV_DURATION at a node typed with the AOM 1.4 short name
// DURATION, the value NewSkeleton itself writes there; a DV_QUANTITY at an
// untyped alternative after a DV_CODED_TEXT; and a DV_INTERVAL<DV_COUNT> at
// a DV_INTERVAL<DV_ORDERED> node, whose bounds conform to DV_ORDERED.
//
// An untyped node that NodeAt returns, the first or only child at the path,
// is out of reach: NewSkeleton cannot build a value for it, so no Builder
// exists to Set it on.
func TestREQ101_SetAcceptsWhatTheValidatorBinds(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		c    *templatecompile.Compiled
		// nodeAt is the RM type of the node Compiled.NodeAt returns.
		nodeAt string
		// skeleton is the RM type of the value NewSkeleton writes at the path.
		skeleton string
		v        any
	}{
		{
			name: "DV_DURATION at DURATION", c: compileAlternatives(t, durationValueChild),
			nodeAt: "DURATION", skeleton: "DV_DURATION", v: &rm.DVDuration{Value: "PT5M"},
		},
		{
			name:   "DV_QUANTITY at an untyped alternative after DV_CODED_TEXT",
			c:      compileAlternatives(t, valueAlternative("DV_CODED_TEXT", false), untypedValueChild),
			nodeAt: "DV_CODED_TEXT", skeleton: "DV_CODED_TEXT", v: &rm.DVQuantity{Magnitude: 120, Units: "mm[Hg]"},
		},
		{
			name: "DV_INTERVAL<DV_COUNT> at DV_INTERVAL<DV_ORDERED>", c: compileAlternatives(t, valueAlternative(orderedIntervalType, false)),
			nodeAt: "DV_INTERVAL<DV_ORDERED>", skeleton: "DV_INTERVAL", v: countInterval(),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			node, err := tc.c.NodeAt(abstractValuePath)
			if err != nil {
				t.Fatalf("NodeAt(%s): %v", abstractValuePath, err)
			}
			if got := node.RMTypeName(); got != tc.nodeAt {
				t.Fatalf("NodeAt(%s) has RM type %q, want %q (fixture drift)", abstractValuePath, got, tc.nodeAt)
			}
			b, err := composition.NewBuilder(t.Context(), tc.c,
				composition.WithTerritory("NL"),
				composition.WithComposer(testComposer()),
			)
			if err != nil {
				t.Fatalf("NewBuilder: %v", err)
			}
			skeleton, err := b.Build()
			if err != nil {
				t.Fatalf("Build of the bare skeleton: %v", err)
			}
			written, err := rmpath.ItemAtPath(skeleton, abstractValuePath)
			if err != nil {
				t.Fatalf("ItemAtPath(%s) on the skeleton: %v", abstractValuePath, err)
			}
			if name, _ := rm.RMTypeName(written); name != tc.skeleton {
				t.Fatalf("NewSkeleton writes %T (RM %q) at %s, want %s (generator drift)", written, name, abstractValuePath, tc.skeleton)
			}
			before := validation.ValidateComposition(skeleton, tc.c)

			if err := b.Set(abstractValuePath, tc.v); err != nil {
				t.Fatalf("Set(%s, %T) = %v, want nil", abstractValuePath, tc.v, err)
			}
			comp, err := b.Build()
			if err != nil {
				t.Fatalf("Build after Set(%s, %T) = %v, want nil", abstractValuePath, tc.v, err)
			}
			got, err := rmpath.ItemAtPath(comp, abstractValuePath)
			if err != nil {
				t.Fatalf("ItemAtPath(%s) on the built composition: %v", abstractValuePath, err)
			}
			if got != tc.v {
				t.Errorf("built composition holds %T %v at %s, want the value Set was given, %T %v", got, got, abstractValuePath, tc.v, tc.v)
			}
			after := validation.ValidateComposition(comp, tc.c)
			if added := newIssues(before, after); len(added) > 0 {
				t.Errorf("ValidateComposition after Set(%s, %T) reports new findings %q, want none", abstractValuePath, tc.v, added)
			}
		})
	}
}

// TestREQ101_SetRefusesWhatTheValidatorDoesNotBind checks, for REQ-101 Type
// enforcement, that Set returns ErrTypeMismatch, and Build carries it, for a
// value the alternatives rule (REQ-102) binds to no node at the path, at the
// nodes the short-name and interval tiers read: a DV_TEXT at DURATION; a
// DV_DATE, the wrapper of another short name, at DURATION; a DV_QUANTITY, an
// ordered value but no interval, at DV_INTERVAL<DV_ORDERED>; and a
// DV_INTERVAL<DV_COUNT> at DV_INTERVAL<DV_QUANTITY>, since a DV_COUNT bound
// does not conform to DV_QUANTITY. A Go string at DURATION is refused
// although the validator binds a string at a DURATION leaf: it is no RM
// value. A typed-nil *rm.DVDuration at DURATION is refused too.
func TestREQ101_SetRefusesWhatTheValidatorDoesNotBind(t *testing.T) {
	t.Parallel()
	duration := compileAlternatives(t, durationValueChild)
	orderedInterval := compileAlternatives(t, valueAlternative(orderedIntervalType, false))
	quantityInterval := compileAlternatives(t, valueAlternative(quantityIntervalType, false))

	cases := []struct {
		name string
		c    *templatecompile.Compiled
		v    any
	}{
		{name: "DV_TEXT at DURATION", c: duration, v: &rm.DVText{Value: "x"}},
		{name: "DV_DATE at DURATION", c: duration, v: &rm.DVDate{Value: "2026-10-15"}},
		{name: "Go string at DURATION", c: duration, v: "PT5M"},
		{name: "typed-nil DV_DURATION at DURATION", c: duration, v: (*rm.DVDuration)(nil)},
		{name: "DV_QUANTITY at DV_INTERVAL<DV_ORDERED>", c: orderedInterval, v: &rm.DVQuantity{Magnitude: 1, Units: "mm"}},
		{name: "DV_INTERVAL<DV_COUNT> at DV_INTERVAL<DV_QUANTITY>", c: quantityInterval, v: countInterval()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			b, err := composition.NewBuilder(t.Context(), tc.c,
				composition.WithTerritory("NL"),
				composition.WithComposer(testComposer()),
			)
			if err != nil {
				t.Fatalf("NewBuilder: %v", err)
			}
			if err := b.Set(abstractValuePath, tc.v); !errors.Is(err, composition.ErrTypeMismatch) {
				t.Errorf("Set(%s, %T) = %v, want ErrTypeMismatch", abstractValuePath, tc.v, err)
			}
			if _, err := b.Build(); !errors.Is(err, composition.ErrTypeMismatch) {
				t.Errorf("Build after Set(%s, %T) = %v, want ErrTypeMismatch", abstractValuePath, tc.v, err)
			}
		})
	}
}

// Value children for compileAlternatives whose own attributes hold a value
// the template constrains: a DV_DURATION whose value, a String in the RM,
// the template types with the AOM 1.4 short name DURATION; a DV_COUNT whose
// magnitude, an Integer64 in the RM, it types INTEGER; and a
// DV_INTERVAL<DV_COUNT> whose lower bound, a T in the RM and so a DV_COUNT
// here, it allows as a DV_COUNT or an untyped node; and a bare DV_INTERVAL,
// whose name binds no T, with the same lower bound.
const (
	durationValueLeafChild = `<children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>DV_DURATION</rm_type_name><node_id></node_id>` +
		`<attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>value</rm_attribute_name>` +
		`<children xsi:type="C_PRIMITIVE_OBJECT"><rm_type_name>DURATION</rm_type_name><node_id></node_id><item xsi:type="C_DURATION"></item></children>` +
		`</attributes></children>`
	countMagnitudeLeafChild = `<children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>DV_COUNT</rm_type_name><node_id></node_id>` +
		`<attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>magnitude</rm_attribute_name>` +
		`<children xsi:type="C_PRIMITIVE_OBJECT"><rm_type_name>INTEGER</rm_type_name><node_id></node_id><item xsi:type="C_INTEGER"></item></children>` +
		`</attributes></children>`
	countIntervalLowerChild = `<children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>DV_INTERVAL&lt;DV_COUNT&gt;</rm_type_name><node_id></node_id>` +
		`<attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>lower</rm_attribute_name>` +
		`<children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>DV_COUNT</rm_type_name><node_id></node_id></children>` +
		untypedValueChild +
		`</attributes></children>`
	bareIntervalLowerChild = `<children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>DV_INTERVAL</rm_type_name><node_id></node_id>` +
		`<attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>lower</rm_attribute_name>` +
		`<children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>DV_COUNT</rm_type_name><node_id></node_id></children>` +
		untypedValueChild +
		`</attributes></children>`
)

// Paths below abstractValuePath into the value children above.
const (
	durationValueLeafPath  = abstractValuePath + "/value"
	countMagnitudeLeafPath = abstractValuePath + "/magnitude"
	countIntervalLowerPath = abstractValuePath + "/lower"
)

// eventDataOPT is a COMPOSITION template with one OBSERVATION whose
// history holds one event of the RM type that eventTypeMarker stands for.
// The event's data, which the RM types with the event's generic parameter
// T, is allowed as an ITEM_TREE or as an untyped node, at the one path
// eventDataPath. The event type names no T.
const (
	eventTypeMarker = "EVENT_TYPE"
	eventDataOPT    = `<?xml version="1.0" encoding="utf-8"?>
<template xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xmlns="http://schemas.openehr.org/v1">
<language><terminology_id><value>ISO_639-1</value></terminology_id><code_string>en</code_string></language>
<template_id><value>event_data</value></template_id><concept>event_data</concept>
<definition><rm_type_name>COMPOSITION</rm_type_name><node_id>at0000</node_id>
<attributes xsi:type="C_MULTIPLE_ATTRIBUTE"><rm_attribute_name>content</rm_attribute_name>
<children xsi:type="C_ARCHETYPE_ROOT"><rm_type_name>OBSERVATION</rm_type_name><node_id>at0000</node_id>
<attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>data</rm_attribute_name>
<children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>HISTORY</rm_type_name><node_id>at0001</node_id>
<attributes xsi:type="C_MULTIPLE_ATTRIBUTE"><rm_attribute_name>events</rm_attribute_name>
<children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>` + eventTypeMarker + `</rm_type_name><node_id>at0002</node_id>
<attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>data</rm_attribute_name>
<children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>ITEM_TREE</rm_type_name><node_id>at0003</node_id></children>
<children xsi:type="C_COMPLEX_OBJECT"><rm_type_name></rm_type_name><node_id>at0003</node_id></children>
</attributes>
</children>
<cardinality><is_ordered>false</is_ordered><is_unique>false</is_unique><interval><lower_included>true</lower_included><lower_unbounded>false</lower_unbounded><upper_unbounded>true</upper_unbounded><lower>0</lower></interval></cardinality>
</attributes>
</children>
</attributes>
<archetype_id><value>openEHR-EHR-OBSERVATION.event_data.v1</value></archetype_id>
</children>
<cardinality><is_ordered>false</is_ordered><is_unique>false</is_unique><interval><lower_included>true</lower_included><lower_unbounded>false</lower_unbounded><upper_unbounded>true</upper_unbounded><lower>0</lower></interval></cardinality>
</attributes>
<archetype_id><value>openEHR-EHR-COMPOSITION.report.v1</value></archetype_id>
</definition>
</template>`
	eventDataPath = "/content[openEHR-EHR-OBSERVATION.event_data.v1]/data/events[at0002]/data"
)

// compileEventData compiles eventDataOPT with an event of RM type
// eventType, and checks that the event compiled with that type.
func compileEventData(t *testing.T, eventType string) *templatecompile.Compiled {
	t.Helper()
	if n := strings.Count(eventDataOPT, eventTypeMarker); n != 1 {
		t.Fatalf("eventDataOPT carries %d event type markers, want 1 (fixture drift)", n)
	}
	c := compileInline(t, strings.Replace(eventDataOPT, eventTypeMarker, eventType, 1))
	const eventPath = "/content[openEHR-EHR-OBSERVATION.event_data.v1]/data/events[at0002]"
	node, err := c.NodeAt(eventPath)
	if err != nil {
		t.Fatalf("NodeAt(%s): %v (fixture drift)", eventPath, err)
	}
	if got := node.RMTypeName(); got != eventType {
		t.Fatalf("NodeAt(%s) has RM type %q, want %q (fixture drift)", eventPath, got, eventType)
	}
	return c
}

// TestREQ101_SetRefusesWhatTheAttributeCannotHold checks, for REQ-101 Type
// enforcement, that Set returns ErrTypeMismatch, and Build carries it, for a
// value that binds a node at the path but that the RM attribute the path
// writes cannot hold: a DV_DURATION at the DURATION leaf under
// DV_DURATION.value, which holds a String; a DV_COUNT at the INTEGER leaf
// under DV_COUNT.magnitude, which holds an Integer64; a PARTY_IDENTIFIED and
// an ELEMENT at an untyped alternative of ELEMENT.value, which holds a
// DATA_VALUE; and a DV_QUANTITY at an untyped alternative of the lower bound
// of a DV_INTERVAL<DV_COUNT>, which holds a DV_COUNT.
//
// Where the attribute's type is a generic parameter the template leaves
// open, the value must conform to the parameter's constraint: a
// PARTY_IDENTIFIED and a DV_TEXT at an untyped alternative of the lower
// bound of a bare DV_INTERVAL, whose T must be a DV_ORDERED; and a DV_TEXT
// at an untyped alternative of the data of a bare EVENT, whose T must be an
// ITEM_STRUCTURE, or of a bare POINT_EVENT, whose T is passed to EVENT's.
func TestREQ101_SetRefusesWhatTheAttributeCannotHold(t *testing.T) {
	t.Parallel()
	durationLeaf := compileAlternatives(t, durationValueLeafChild)
	countLeaf := compileAlternatives(t, countMagnitudeLeafChild)
	codedThenUntyped := compileAlternatives(t, valueAlternative("DV_CODED_TEXT", false), untypedValueChild)
	countLower := compileAlternatives(t, countIntervalLowerChild)
	bareLower := compileAlternatives(t, bareIntervalLowerChild)
	eventData := compileEventData(t, "EVENT")
	pointEventData := compileEventData(t, "POINT_EVENT")

	cases := []struct {
		name string
		c    *templatecompile.Compiled
		path string
		// nodeAt is the RM type of the node Compiled.NodeAt returns.
		nodeAt string
		v      any
	}{
		{
			name: "DV_DURATION at DURATION under DV_DURATION.value", c: durationLeaf,
			path: durationValueLeafPath, nodeAt: "DURATION", v: &rm.DVDuration{Value: "PT5M"},
		},
		{
			name: "DV_COUNT at INTEGER under DV_COUNT.magnitude", c: countLeaf,
			path: countMagnitudeLeafPath, nodeAt: "INTEGER", v: &rm.DVCount{Magnitude: 3},
		},
		{
			name: "PARTY_IDENTIFIED at an untyped alternative after DV_CODED_TEXT", c: codedThenUntyped,
			path: abstractValuePath, nodeAt: "DV_CODED_TEXT", v: &rm.PartyIdentified{},
		},
		{
			name: "ELEMENT at an untyped alternative after DV_CODED_TEXT", c: codedThenUntyped,
			path: abstractValuePath, nodeAt: "DV_CODED_TEXT", v: &rm.Element{},
		},
		{
			name: "DV_QUANTITY at an untyped lower alternative of DV_INTERVAL<DV_COUNT>", c: countLower,
			path: countIntervalLowerPath, nodeAt: "DV_COUNT", v: &rm.DVQuantity{Magnitude: 1, Units: "mm"},
		},
		{
			name: "PARTY_IDENTIFIED at an untyped lower alternative of a bare DV_INTERVAL", c: bareLower,
			path: countIntervalLowerPath, nodeAt: "DV_COUNT", v: &rm.PartyIdentified{},
		},
		{
			name: "DV_TEXT at an untyped lower alternative of a bare DV_INTERVAL", c: bareLower,
			path: countIntervalLowerPath, nodeAt: "DV_COUNT", v: &rm.DVText{Value: "x"},
		},
		{
			name: "DV_TEXT at an untyped data alternative of a bare EVENT", c: eventData,
			path: eventDataPath, nodeAt: "ITEM_TREE", v: &rm.DVText{Value: "x"},
		},
		{
			name: "DV_TEXT at an untyped data alternative of a bare POINT_EVENT", c: pointEventData,
			path: eventDataPath, nodeAt: "ITEM_TREE", v: &rm.DVText{Value: "x"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			node, err := tc.c.NodeAt(tc.path)
			if err != nil {
				t.Fatalf("NodeAt(%s): %v", tc.path, err)
			}
			if got := node.RMTypeName(); got != tc.nodeAt {
				t.Fatalf("NodeAt(%s) has RM type %q, want %q (fixture drift)", tc.path, got, tc.nodeAt)
			}
			b, err := composition.NewBuilder(t.Context(), tc.c,
				composition.WithTerritory("NL"),
				composition.WithComposer(testComposer()),
			)
			if err != nil {
				t.Fatalf("NewBuilder: %v", err)
			}
			if err := b.Set(tc.path, tc.v); !errors.Is(err, composition.ErrTypeMismatch) {
				t.Errorf("Set(%s, %T) = %v, want ErrTypeMismatch", tc.path, tc.v, err)
			}
			if _, err := b.Build(); !errors.Is(err, composition.ErrTypeMismatch) {
				t.Errorf("Build after Set(%s, %T) = %v, want ErrTypeMismatch", tc.path, tc.v, err)
			}
		})
	}
}

// TestREQ101_SetAcceptsWhatTheAttributeHolds checks, for REQ-101 Type
// enforcement, that the attribute check reads the attribute's type with the
// generic parameter its owner names: under a DV_INTERVAL<DV_COUNT>, whose
// lower bound the RM types T, Set accepts a DV_COUNT at the lower bound and
// Build writes it there. rmpath does not step into an interval's bounds, so
// the test reads the interval at abstractValuePath.
func TestREQ101_SetAcceptsWhatTheAttributeHolds(t *testing.T) {
	t.Parallel()
	c := compileAlternatives(t, countIntervalLowerChild)
	b, err := composition.NewBuilder(t.Context(), c,
		composition.WithTerritory("NL"),
		composition.WithComposer(testComposer()),
	)
	if err != nil {
		t.Fatalf("NewBuilder: %v", err)
	}
	v := &rm.DVCount{Magnitude: 3}
	if err := b.Set(countIntervalLowerPath, v); err != nil {
		t.Fatalf("Set(%s, %T) = %v, want nil", countIntervalLowerPath, v, err)
	}
	comp, err := b.Build()
	if err != nil {
		t.Fatalf("Build after Set(%s, %T) = %v, want nil", countIntervalLowerPath, v, err)
	}
	got, err := rmpath.ItemAtPath(comp, abstractValuePath)
	if err != nil {
		t.Fatalf("ItemAtPath(%s) on the built composition: %v", abstractValuePath, err)
	}
	interval, ok := got.(*rm.DVInterval[rm.DVCount])
	if !ok {
		t.Fatalf("built composition holds %T at %s, want a *rm.DVInterval[rm.DVCount]", got, abstractValuePath)
	}
	if interval.Lower.Magnitude != v.Magnitude {
		t.Errorf("built interval at %s has lower magnitude %d, want %d, the DV_COUNT Set was given", abstractValuePath, interval.Lower.Magnitude, v.Magnitude)
	}
}

// TestREQ101_SetAcceptsWhereTheAttributeTypeIsOpen checks, for REQ-101 Type
// enforcement, that the attribute check accepts a value that conforms to the
// constraint of a generic parameter the template leaves open: under a bare
// DV_INTERVAL, whose name binds no T, the lower bound's RM type stays the
// generic T, which must be a DV_ORDERED, and Set accepts a DV_COUNT there,
// which is one, and Build writes it.
func TestREQ101_SetAcceptsWhereTheAttributeTypeIsOpen(t *testing.T) {
	t.Parallel()
	c := compileAlternatives(t, bareIntervalLowerChild)
	b, err := composition.NewBuilder(t.Context(), c,
		composition.WithTerritory("NL"),
		composition.WithComposer(testComposer()),
	)
	if err != nil {
		t.Fatalf("NewBuilder: %v", err)
	}
	v := &rm.DVCount{Magnitude: 3}
	if err := b.Set(countIntervalLowerPath, v); err != nil {
		t.Fatalf("Set(%s, %T) = %v, want nil", countIntervalLowerPath, v, err)
	}
	comp, err := b.Build()
	if err != nil {
		t.Fatalf("Build after Set(%s, %T) = %v, want nil", countIntervalLowerPath, v, err)
	}
	got, err := rmpath.ItemAtPath(comp, abstractValuePath)
	if err != nil {
		t.Fatalf("ItemAtPath(%s) on the built composition: %v", abstractValuePath, err)
	}
	interval, ok := got.(*rm.DVInterval[rm.DVOrdered])
	if !ok {
		t.Fatalf("built composition holds %T at %s, want a *rm.DVInterval[rm.DVOrdered]", got, abstractValuePath)
	}
	if interval.Lower != v {
		t.Errorf("built interval at %s has lower bound %T %v, want the DV_COUNT Set was given, %T %v", abstractValuePath, interval.Lower, interval.Lower, v, v)
	}
}

// TestREQ101_SetAcceptsWhatAnOpenParameterAllows checks, for REQ-101 Type
// enforcement, that the attribute check reads the constraint a generic
// parameter takes from the ancestor it is passed to: under a bare
// POINT_EVENT, whose data the RM types with its T, and whose T EVENT
// constrains to ITEM_STRUCTURE, Set accepts an ITEM_LIST at an untyped
// alternative of the data, and Build writes it.
func TestREQ101_SetAcceptsWhatAnOpenParameterAllows(t *testing.T) {
	t.Parallel()
	c := compileEventData(t, "POINT_EVENT")
	b, err := composition.NewBuilder(t.Context(), c,
		composition.WithTerritory("NL"),
		composition.WithComposer(testComposer()),
	)
	if err != nil {
		t.Fatalf("NewBuilder: %v", err)
	}
	v := &rm.ItemList{ArchetypeNodeID: "at0003"}
	if err := b.Set(eventDataPath, v); err != nil {
		t.Fatalf("Set(%s, %T) = %v, want nil", eventDataPath, v, err)
	}
	comp, err := b.Build()
	if err != nil {
		t.Fatalf("Build after Set(%s, %T) = %v, want nil", eventDataPath, v, err)
	}
	got, err := rmpath.ItemAtPath(comp, eventDataPath)
	if err != nil {
		t.Fatalf("ItemAtPath(%s) on the built composition: %v", eventDataPath, err)
	}
	if got != v {
		t.Errorf("built composition holds %T %v at %s, want the ITEM_LIST Set was given", got, got, eventDataPath)
	}
}

// TestREQ101_SetRefusesAPathThatWritesNoAttribute checks, for REQ-101 Type
// enforcement, that Set returns ErrTypeMismatch, and Build carries it, at a
// template path that writes no RM attribute. Modelling tools constrain BMM
// functions as if they were attributes, and Demonstration.v1 carries two:
// DV_PROPORTION.is_integral and the offset of a POINT_EVENT. The error
// names the attribute and the class.
func TestREQ101_SetRefusesAPathThatWritesNoAttribute(t *testing.T) {
	t.Parallel()
	demonstration := compileFixture(t, "Demonstration.v1")

	cases := []struct {
		name string
		c    *templatecompile.Compiled
		path string
		// nodeAt is the RM type of the node Compiled.NodeAt returns;
		// owner and attr are the class and the name the path writes.
		nodeAt, owner, attr string
		v                   any
	}{
		{
			name: "DV_BOOLEAN at DV_PROPORTION.is_integral", c: demonstration,
			path:   "/content[openEHR-EHR-OBSERVATION.demo.v1]/data/events[at0002]/data/items[at0004]/items[at0028]/value/is_integral",
			nodeAt: "BOOLEAN", owner: "DV_PROPORTION", attr: "is_integral", v: &rm.DVBoolean{Value: true},
		},
		{
			name: "DV_DURATION at POINT_EVENT.offset", c: demonstration,
			path:   "/content[openEHR-EHR-OBSERVATION.demo.v1]/data/events[at0035]/offset",
			nodeAt: "DV_DURATION", owner: "POINT_EVENT", attr: "offset", v: &rm.DVDuration{Value: "PT1M"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			node, err := tc.c.NodeAt(tc.path)
			if err != nil {
				t.Fatalf("NodeAt(%s): %v", tc.path, err)
			}
			if got := node.RMTypeName(); got != tc.nodeAt {
				t.Fatalf("NodeAt(%s) has RM type %q, want %q (fixture drift)", tc.path, got, tc.nodeAt)
			}
			if got := node.Parent().RMTypeName(); got != tc.owner {
				t.Fatalf("node at %s sits under RM type %q, want %q (fixture drift)", tc.path, got, tc.owner)
			}
			b, err := composition.NewBuilder(t.Context(), tc.c,
				composition.WithTerritory("NL"),
				composition.WithComposer(testComposer()),
			)
			if err != nil {
				t.Fatalf("NewBuilder: %v", err)
			}
			err = b.Set(tc.path, tc.v)
			switch {
			case !errors.Is(err, composition.ErrTypeMismatch):
				t.Errorf("Set(%s, %T) = %v, want ErrTypeMismatch", tc.path, tc.v, err)
			case !strings.Contains(err.Error(), strconv.Quote(tc.attr)) || !strings.Contains(err.Error(), tc.owner):
				t.Errorf("Set(%s, %T) = %v, want an error naming attribute %q and class %s", tc.path, tc.v, err, tc.attr, tc.owner)
			}
			if _, err := b.Build(); !errors.Is(err, composition.ErrTypeMismatch) {
				t.Errorf("Build after Set(%s, %T) = %v, want ErrTypeMismatch", tc.path, tc.v, err)
			}
		})
	}
}

// TestREQ101_SetAtTheRootReturns checks, for REQ-101, that Set at the
// composition root returns rather than panics. The root sits under no RM
// attribute, so the attribute check has no attribute to read there. Build
// then reports ErrInvalidPath, since the builder does not replace the root.
func TestREQ101_SetAtTheRootReturns(t *testing.T) {
	t.Parallel()
	c := compileAlternatives(t, valueAlternative("DV_TEXT", false))
	b, err := composition.NewBuilder(t.Context(), c,
		composition.WithTerritory("NL"),
		composition.WithComposer(testComposer()),
	)
	if err != nil {
		t.Fatalf("NewBuilder: %v", err)
	}
	const root = "/"
	v := &rm.Composition{}
	setErr := func() (err error) {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("Set(%s, %T) panicked: %v", root, v, r)
			}
		}()
		return b.Set(root, v)
	}()
	if _, err := b.Build(); !errors.Is(err, composition.ErrInvalidPath) {
		t.Errorf("Build after Set(%s, %T) = %v (Set returned %v), want ErrInvalidPath", root, v, err, setErr)
	}
}
