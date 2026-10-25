package validation_test

// binding_trial_test.go: REQ-102 (Structural — alternatives, Structural —
// binding items, Items that are not a LOCATABLE) and REQ-171 (One walk) —
// within the first tier that admits a value, the validator binds it to the
// first child under whose constraints it validates, and to the first child
// of the tier, whose findings it reports, when it validates under none. An
// item of a list binds within its rank to the first child that has room and
// under which it validates, then to the first with room, then to the first.
// The trial is part of the walk: it counts toward the limits, and what a
// trial under a child the value does not bind to found leaves no trace.

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// actionTransitionsOPT returns an OPT rooted at an ACTION whose
// ism_transition, required once, holds the given ISM_TRANSITION
// alternatives, each rendered by transitionChild.
func actionTransitionsOPT(alternatives ...string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<template xmlns="http://schemas.openehr.org/v1" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <language><terminology_id><value>ISO_639-1</value></terminology_id><code_string>en</code_string></language>
  <template_id><value>action-transitions</value></template_id>
  <concept>action-transitions</concept>
  <definition>
    <rm_type_name>ACTION</rm_type_name>
    <node_id>at0000</node_id>
    <attributes xsi:type="C_SINGLE_ATTRIBUTE">
      <rm_attribute_name>ism_transition</rm_attribute_name>
      <existence><lower>1</lower><upper>1</upper></existence>` + strings.Join(alternatives, "") + `
    </attributes>
    <archetype_id><value>openEHR-EHR-ACTION.medication.v1</value></archetype_id>
  </definition>
</template>`
}

// codedChild renders a DV_CODED_TEXT child whose defining_code is coded in
// terminology with code, as an ADL 1.4 OPT constrains an ISM_TRANSITION's
// current_state and careflow_step.
func codedChild(terminology, code string) string {
	return `
          <children xsi:type="C_COMPLEX_OBJECT">
            <rm_type_name>DV_CODED_TEXT</rm_type_name>
            <node_id></node_id>
            <attributes xsi:type="C_SINGLE_ATTRIBUTE">
              <rm_attribute_name>defining_code</rm_attribute_name>
              <existence><lower>1</lower><upper>1</upper></existence>
              <children xsi:type="C_CODE_PHRASE">
                <rm_type_name>CODE_PHRASE</rm_type_name>
                <node_id></node_id>
                <terminology_id><value>` + terminology + `</value></terminology_id>
                <code_list>` + code + `</code_list>
              </children>
            </attributes>
          </children>`
}

// transitionChild renders one ISM_TRANSITION alternative, the minimal_action
// shape: its current_state coded as the openEHR state state, and an optional
// careflow_step coded as the local step, each a closed one-code list. extra
// holds further attribute declarations of the alternative.
func transitionChild(nodeID, state, step string, extra ...string) string {
	return `
      <children xsi:type="C_COMPLEX_OBJECT">
        <rm_type_name>ISM_TRANSITION</rm_type_name>
        <node_id>` + nodeID + `</node_id>
        <attributes xsi:type="C_SINGLE_ATTRIBUTE">
          <rm_attribute_name>current_state</rm_attribute_name>
          <existence><lower>1</lower><upper>1</upper></existence>` + codedChild("openehr", state) + `
        </attributes>
        <attributes xsi:type="C_SINGLE_ATTRIBUTE">
          <rm_attribute_name>careflow_step</rm_attribute_name>
          <existence><lower>0</lower><upper>1</upper></existence>` + codedChild("local", step) + `
        </attributes>` + strings.Join(extra, "") + `
      </children>`
}

// The two alternatives of an ACTION's ISM_TRANSITION the tests use: the
// first codes the initial state 524 and the step at0005, the second the
// completed state 532 and the step at0006.
var (
	initialTransition   = transitionChild("at0005", "524", "at0005")
	completedTransition = transitionChild("at0006", "532", "at0006")
)

// transitionAction returns an otherwise complete ACTION whose
// ISM_TRANSITION is in the openEHR state state at the local careflow step
// step.
func transitionAction(state, step string) *rm.Action {
	act := ismAction(&rm.DVCodedText{
		Value:        "step",
		DefiningCode: rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "local"}, CodeString: step},
	})
	act.IsmTransition.CurrentState.DefiningCode.CodeString = state
	return act
}

// REQ-102 Structural — alternatives — the alternatives of an ACTION's
// ISM_TRANSITION are of one RM type, so they share the first tier. A
// transition that matches the second as a whole binds to it and draws no
// finding, as does one that matches the first.
func TestValidate_REQ102_TransitionBindsToTheAlternativeItMatches(t *testing.T) {
	c := mustCompileInline(t, actionTransitionsOPT(initialTransition, completedTransition))
	for _, tc := range []struct{ name, state, step string }{
		{name: "the first alternative", state: "524", step: "at0005"},
		{name: "the second alternative", state: "532", step: "at0006"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := validation.Validate(transitionAction(tc.state, tc.step), c)
			if !r.OK || len(r.Issues) != 0 {
				t.Errorf("Validate(transition %s at %s) = OK %t, issues %q, want OK with none", tc.state, tc.step, r.OK, issueKeys(r.Issues))
			}
		})
	}
}

// quantityIn renders a C_DV_QUANTITY child that admits a magnitude from
// 0 to 1000 in units.
func quantityIn(units string) string {
	return `
      <children xsi:type="C_DV_QUANTITY">
        <rm_type_name>DV_QUANTITY</rm_type_name>
        <node_id></node_id>
        <list>
          <magnitude>
            <lower_included>true</lower_included><upper_included>true</upper_included>
            <lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded>
            <lower>0.0</lower><upper>1000.0</upper>
          </magnitude>
          <units>` + units + `</units>
        </list>
      </children>`
}

// quantityElement returns an ELEMENT of elementValueOPT whose value is a
// DV_QUANTITY of magnitude in units.
func quantityElement(magnitude float64, units string) *rm.Element {
	return valueElement(rm.DVQuantity{Magnitude: rm.Real(magnitude), Units: units})
}

// REQ-102 Structural — alternatives — of two DV_QUANTITY alternatives that
// differ in their units, a value in the second's units binds to the second
// and draws no finding; one that fits neither binds to the first, so its
// findings are the first alternative's alone, not `alternative_mismatch`,
// and with the alternatives the other way round, the other one's.
func TestValidate_REQ102_QuantityBindsToTheAlternativeItMatches(t *testing.T) {
	both := mustCompileInline(t, elementValueOPT(quantityIn("kg"), quantityIn("[lb_av]")))
	for _, units := range []string{"kg", "[lb_av]"} {
		r := validation.Validate(quantityElement(80, units), both)
		if !r.OK || len(r.Issues) != 0 {
			t.Errorf("Validate(80 %s under kg or [lb_av]) = OK %t, issues %q, want OK with none", units, r.OK, issueKeys(r.Issues))
		}
	}
	for _, order := range [][]string{{"kg", "[lb_av]"}, {"[lb_av]", "kg"}} {
		t.Run(strings.Join(order, " then "), func(t *testing.T) {
			pair := mustCompileInline(t, elementValueOPT(quantityIn(order[0]), quantityIn(order[1])))
			first := mustCompileInline(t, elementValueOPT(quantityIn(order[0])))
			value := quantityElement(2000, "g")
			got, want := issueKeys(validation.Validate(value, pair).Issues), issueKeys(validation.Validate(value, first).Issues)
			if len(want) == 0 {
				t.Fatalf("Validate(2000 g under %s alone) reports nothing, so it shows no binding", order[0])
			}
			if !slices.Equal(got, want) {
				t.Errorf("Validate(2000 g under %s then %s) issues = %q, want the first alternative's %q", order[0], order[1], got, want)
			}
		})
	}
}

// REQ-102 Structural — alternatives — a transition that matches neither
// alternative as a whole is still refused, and binds to the first: it draws
// the findings it draws under the first alone, not `alternative_mismatch`.
// With the alternatives the other way round it draws the other one's.
func TestValidate_REQ102_NoAlternativeMatchedReportsTheFirst(t *testing.T) {
	value := func() *rm.Action { return transitionAction("530", "at0006") }
	for _, tc := range []struct {
		name         string
		first, other string
	}{
		{name: "initial then completed", first: initialTransition, other: completedTransition},
		{name: "completed then initial", first: completedTransition, other: initialTransition},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := validation.Validate(value(), mustCompileInline(t, actionTransitionsOPT(tc.first, tc.other)))
			alone := validation.Validate(value(), mustCompileInline(t, actionTransitionsOPT(tc.first)))
			if r.OK {
				t.Fatalf("Validate(transition 530 at at0006) is OK, want it refused")
			}
			if got, want := issueKeys(r.Issues), issueKeys(alone.Issues); !slices.Equal(got, want) {
				t.Errorf("Validate(transition 530 at at0006) issues = %q, want the first alternative's %q", got, want)
			}
			if containsCode(r.Issues, "alternative_mismatch") {
				t.Errorf("Validate(transition 530 at at0006) issues = %q, want no alternative_mismatch: a tier admits it", issueKeys(r.Issues))
			}
		})
	}
}

// reusedObservationWithData returns reusedObservationChild(name, lower) with
// the node id of the HISTORY it constrains data to replaced by dataID, so
// two children of one archetype differ in what they admit.
func reusedObservationWithData(t *testing.T, name string, lower int, dataID string) string {
	t.Helper()
	child := reusedObservationChild(name, lower)
	const data = `<rm_type_name>HISTORY</rm_type_name><node_id>at0001</node_id>`
	if strings.Count(child, data) != 1 {
		t.Fatal("reusedObservationChild no longer constrains data to HISTORY at0001 once")
	}
	return strings.Replace(child, data, `<rm_type_name>HISTORY</rm_type_name><node_id>`+dataID+`</node_id>`, 1)
}

// REQ-102 Structural — binding items — siblings that carry one id and pin
// no name rank alike, so an item binds to the first that has room and under
// which it validates. An optional then a required sibling each take one
// member of two, so the composition validates; a third member finds no room,
// binds to the first, and is reported by its occurrences. A single member
// binds to the first sibling, which leaves the required one without one.
// Two siblings that pin one name rank alike too: two members of that name
// fill them both. An item that does not validate under the first sibling
// that has room binds to the next one under which it does.
func TestValidateComposition_REQ102_ItemsOfOneRankBindByRoomAndTrial(t *testing.T) {
	const at = "/content[" + reusedID + "]"
	tests := []struct {
		name     string
		children []string
		members  []string
		want     []string
		// detail is a part of the Detail of the one issue want holds.
		detail string
	}{
		{
			name:     "an optional then a required sibling, a member each",
			children: []string{reusedObservationChild("", 0), reusedObservationChild("", 1)},
			members:  []string{"Erste", "Zweite"},
		},
		{
			name:     "an optional then a required sibling, three members",
			children: []string{reusedObservationChild("", 0), reusedObservationChild("", 1)},
			members:  []string{"Erste", "Zweite", "Dritte"},
			want:     []string{"cardinality " + at},
			detail:   "appears 2 times",
		},
		{
			name:     "an optional then a required sibling, one member",
			children: []string{reusedObservationChild("", 0), reusedObservationChild("", 1)},
			members:  []string{"Erste"},
			want:     []string{"cardinality " + at},
			detail:   "appears 0 times",
		},
		{
			name:     "two siblings that pin one name, two members of that name",
			children: []string{reusedObservationChild("Erste", 0), reusedObservationChild("Erste", 0)},
			members:  []string{"Erste", "Erste"},
		},
		{
			name:     "a member that validates under the second sibling alone",
			children: []string{reusedObservationWithData(t, "", 0, "at0099"), reusedObservationChild("", 0)},
			members:  []string{"Erste"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := mustCompileSyntheticOPT(t, reusedArchetypeOPTWith(tc.children...))
			r := validation.ValidateComposition(reusedComposition(tc.members...), c)
			if got := issueKeys(r.Issues); !slices.Equal(got, tc.want) {
				t.Fatalf("ValidateComposition(%q) issues = %q, want %q", tc.members, got, tc.want)
			}
			if tc.detail != "" && !strings.Contains(r.Issues[0].Detail, tc.detail) {
				t.Errorf("ValidateComposition(%q) detail = %q, want it to say %q", tc.members, r.Issues[0].Detail, tc.detail)
			}
		})
	}
}

// participationChild renders a PARTICIPATION child of
// EVENT_CONTEXT.participations, occurring at most once, whose function text
// must be function.
func participationChild(function string) string {
	return `
          <children xsi:type="C_COMPLEX_OBJECT">
            <rm_type_name>PARTICIPATION</rm_type_name>
            <occurrences>
              <lower_included>true</lower_included><upper_included>true</upper_included>
              <lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded>
              <lower>0</lower><upper>1</upper>
            </occurrences>
            <node_id></node_id>
            <attributes xsi:type="C_SINGLE_ATTRIBUTE">
              <rm_attribute_name>function</rm_attribute_name>
              <children xsi:type="C_COMPLEX_OBJECT">
                <rm_type_name>DV_TEXT</rm_type_name>
                <node_id></node_id>
                <attributes xsi:type="C_SINGLE_ATTRIBUTE">
                  <rm_attribute_name>value</rm_attribute_name>
                  <children xsi:type="C_PRIMITIVE_OBJECT">
                    <rm_type_name>STRING</rm_type_name>
                    <node_id></node_id>
                    <item xsi:type="C_STRING"><list>` + function + `</list></item>
                  </children>
                </attributes>
              </children>
            </attributes>
          </children>`
}

// REQ-102 Items that are not a LOCATABLE — PARTICIPATION items carry no id
// and bind by RM type; two PARTICIPATION children share the first tier, so
// an item binds to the first that has room and under which it validates: a
// surgeon binds to the surgeon child although the assistant child comes
// first. A second surgeon finds the surgeon child full and binds to the
// assistant child, whose function constraint reports it.
func TestValidateComposition_REQ102_NonLocatableItemsBindByRoomAndTrial(t *testing.T) {
	const one = `
          <children xsi:type="C_COMPLEX_OBJECT">
            <rm_type_name>PARTICIPATION</rm_type_name>
            <occurrences>
              <lower_included>true</lower_included>
              <upper_included>true</upper_included>
              <lower_unbounded>false</lower_unbounded>
              <upper_unbounded>false</upper_unbounded>
              <lower>0</lower>
              <upper>1</upper>
            </occurrences>
            <node_id></node_id>
          </children>`
	if strings.Count(participationsOPT, one) != 1 {
		t.Fatal("participationsOPT no longer holds its one PARTICIPATION child: update the replacement")
	}
	c := mustCompileSyntheticOPT(t, strings.Replace(participationsOPT, one, participationChild("assistant")+participationChild("surgeon"), 1))
	with := func(function string) rm.Participation {
		p := validParticipation()
		p.Function = rm.DVText{Value: function}
		return p
	}
	tests := []struct {
		name      string
		functions []string
		want      []string
	}{
		{name: "a surgeon", functions: []string{"surgeon"}},
		{name: "a surgeon and an assistant", functions: []string{"surgeon", "assistant"}},
		{
			name: "two surgeons", functions: []string{"surgeon", "surgeon"},
			want: []string{"primitive_not_in_list /context/participations[2]/function/value"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var parts []rm.Participation
			for _, f := range tc.functions {
				parts = append(parts, with(f))
			}
			r := validation.ValidateComposition(participationsComposition(parts...), c)
			if got := issueKeys(r.Issues); !slices.Equal(got, tc.want) {
				t.Errorf("ValidateComposition(participations %q) issues = %q, want %q", tc.functions, got, tc.want)
			}
		})
	}
}

// REQ-171 One walk, Coverage — what a trial under an alternative the value
// does not bind to found reaches neither the issues nor Coverage: a
// CONSTRAINT_REF leaf (External) and an attribute the RM class does not
// have (NotInRM) declared on the first alternative, which the transition
// does not match, are not listed once it binds to the second.
func TestValidate_REQ171_WithdrawnTrialLeavesNoCoverage(t *testing.T) {
	constraintRef := strings.Replace(initialTransition, codedChild("openehr", "524"), `
          <children xsi:type="C_COMPLEX_OBJECT">
            <rm_type_name>DV_CODED_TEXT</rm_type_name>
            <node_id></node_id>
            <attributes xsi:type="C_SINGLE_ATTRIBUTE">
              <rm_attribute_name>defining_code</rm_attribute_name>
              <children xsi:type="CONSTRAINT_REF">
                <rm_type_name>CODE_PHRASE</rm_type_name>
                <node_id></node_id>
                <reference>ac0001</reference>
              </children>
            </attributes>
          </children>`, 1)
	notInRM := transitionChild("at0005", "524", "at0005", `
        <attributes xsi:type="C_SINGLE_ATTRIBUTE">
          <rm_attribute_name>name</rm_attribute_name>
          <existence><lower>1</lower><upper>1</upper></existence>
          <children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>DV_TEXT</rm_type_name><node_id></node_id></children>
        </attributes>`)
	if constraintRef == initialTransition {
		t.Fatal("the first alternative's current_state is not found: update the replacement")
	}
	for _, tc := range []struct {
		name  string
		first string
		got   func(validation.Coverage) []string
	}{
		{name: "External", first: constraintRef, got: func(c validation.Coverage) []string { return c.External }},
		{name: "NotInRM", first: notInRM, got: func(c validation.Coverage) []string { return c.NotInRM }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			value := transitionAction("532", "at0006")
			alone := validation.Validate(value, mustCompileInline(t, actionTransitionsOPT(tc.first)))
			if len(tc.got(alone.Coverage)) == 0 {
				t.Fatalf("Validate under the first alternative alone lists no %s path, so the trial shows nothing", tc.name)
			}
			r := validation.Validate(value, mustCompileInline(t, actionTransitionsOPT(tc.first, completedTransition)))
			if !r.OK || len(r.Issues) != 0 {
				t.Fatalf("Validate(transition 532 at at0006) = OK %t, issues %q, want OK with none", r.OK, issueKeys(r.Issues))
			}
			if got := tc.got(r.Coverage); len(got) != 0 {
				t.Errorf("Validate(transition 532 at at0006) Coverage.%s = %q, want none: the trial under the first alternative was withdrawn", tc.name, got)
			}
			if !r.Coverage.Complete() {
				t.Errorf("Validate(transition 532 at at0006) Coverage.Complete() = false (%+v), want true", r.Coverage)
			}
		})
	}
}

// clusterItemsOPT returns an OPT rooted at a CLUSTER whose items hold one
// ELEMENT at0001 child per value child in valueChildren, each occurring at
// most once and constraining its value to that child.
func clusterItemsOPT(valueChildren ...string) string {
	var children strings.Builder
	for _, value := range valueChildren {
		children.WriteString(`
      <children xsi:type="C_COMPLEX_OBJECT">
        <rm_type_name>ELEMENT</rm_type_name>
        <occurrences>
          <lower_included>true</lower_included><upper_included>true</upper_included>
          <lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded>
          <lower>0</lower><upper>1</upper>
        </occurrences>
        <node_id>at0001</node_id>
        <attributes xsi:type="C_SINGLE_ATTRIBUTE">
          <rm_attribute_name>value</rm_attribute_name>` + value + `
        </attributes>
      </children>`)
	}
	return `<?xml version="1.0"?>
<template xmlns="http://schemas.openehr.org/v1" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <template_id><value>cluster-items</value></template_id>
  <concept>cluster-items</concept>
  <language><terminology_id><value>ISO_639-1</value></terminology_id><code_string>en</code_string></language>
  <definition>
    <rm_type_name>CLUSTER</rm_type_name>
    <node_id>at0000</node_id>
    <attributes xsi:type="C_MULTIPLE_ATTRIBUTE">
      <rm_attribute_name>items</rm_attribute_name>` + children.String() + `
    </attributes>
    <archetype_id><value>openEHR-EHR-CLUSTER.items.v1</value></archetype_id>
  </definition>
</template>`
}

// REQ-171 One walk, Coverage — the classes a withdrawn trial visited are not
// among those Unevaluated reads. Under the first DV_QUANTITY alternative,
// which prohibits other_reference_ranges, the walk refuses them and visits
// no REFERENCE_RANGE; under the second, whose units differ, it walks them
// with the RM rules alone. A value in the first's units with a reference
// range fits neither, binds to the first, and reports what the first alone
// reports: its issues and its Unevaluated, with no REFERENCE_RANGE rule. It
// holds of the value of a single-valued attribute, whose walk under the
// first is kept while the second is tried, and of a list item under two
// ELEMENT children of one node id, every trial of which is withdrawn
// before the walk under the child it binds to.
func TestValidate_REQ171_WithdrawnTrialLeavesNoUnevaluatedClass(t *testing.T) {
	const prohibitsRanges = `
      <children xsi:type="C_COMPLEX_OBJECT">
        <rm_type_name>DV_QUANTITY</rm_type_name>
        <node_id></node_id>
        <attributes xsi:type="C_SINGLE_ATTRIBUTE">
          <rm_attribute_name>units</rm_attribute_name>
          <children xsi:type="C_PRIMITIVE_OBJECT">
            <rm_type_name>STRING</rm_type_name>
            <node_id></node_id>
            <item xsi:type="C_STRING"><list>kg</list></item>
          </children>
        </attributes>
        <attributes xsi:type="C_MULTIPLE_ATTRIBUTE">
          <rm_attribute_name>other_reference_ranges</rm_attribute_name>
          <existence><lower>0</lower><upper>0</upper></existence>
        </attributes>
      </children>`
	quantity := func() rm.DVQuantity {
		return rm.DVQuantity{
			Magnitude: 80,
			Units:     "kg",
			OtherReferenceRanges: []rm.ReferenceRange[rm.DVQuantity]{{
				Meaning: rm.DVText{Value: "normal"},
				Range: rm.DVInterval[rm.DVOrdered]{
					Lower: &rm.DVQuantity{Magnitude: 50, Units: "kg"}, Upper: &rm.DVQuantity{Magnitude: 100, Units: "kg"},
					LowerIncluded: true, UpperIncluded: true,
				},
			}},
		}
	}
	cluster := func() any {
		return &rm.Cluster{
			ArchetypeNodeID:  "openEHR-EHR-CLUSTER.items.v1",
			Name:             rm.DVText{Value: "cluster"},
			ArchetypeDetails: archetypeDetails("openEHR-EHR-CLUSTER.items.v1"),
			Items:            []rm.Item{&rm.Element{ArchetypeNodeID: "at0001", Name: rm.DVText{Value: "element"}, Value: quantity()}},
		}
	}
	const rangeRule = "REFERENCE_RANGE.Range_is_simple"
	for _, tc := range []struct {
		name                string
		root                func() any
		second, first, pair string
	}{
		{
			name:   "a single-valued attribute",
			root:   func() any { return valueElement(quantity()) },
			second: elementValueOPT(quantityIn("[lb_av]")),
			first:  elementValueOPT(prohibitsRanges),
			pair:   elementValueOPT(prohibitsRanges, quantityIn("[lb_av]")),
		},
		{
			name:   "a list item",
			root:   cluster,
			second: clusterItemsOPT(quantityIn("[lb_av]")),
			first:  clusterItemsOPT(prohibitsRanges),
			pair:   clusterItemsOPT(prohibitsRanges, quantityIn("[lb_av]")),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if u := validation.Validate(tc.root(), mustCompileInline(t, tc.second)).Coverage.Unevaluated; !slices.Contains(u, rangeRule) {
				t.Fatalf("Validate under the second alternative alone lists Unevaluated %q, without %s, so the trial shows nothing", u, rangeRule)
			}
			alone := validation.Validate(tc.root(), mustCompileInline(t, tc.first))
			r := validation.Validate(tc.root(), mustCompileInline(t, tc.pair))
			if got, want := issueKeys(r.Issues), issueKeys(alone.Issues); len(want) == 0 || !slices.Equal(got, want) {
				t.Fatalf("Validate(80 kg with a reference range) issues = %q, want the first alternative's %q", got, want)
			}
			if got, want := r.Coverage.Unevaluated, alone.Coverage.Unevaluated; !slices.Equal(got, want) {
				t.Errorf("Validate(80 kg with a reference range) Coverage.Unevaluated = %q, want the first alternative's %q", got, want)
			}
		})
	}
}

// REQ-171 One walk — a refusal inside a withdrawn trial does not withdraw a
// finding of the node above it. Under the first current_state alternative
// the walk refuses the transition's defining_code, typed against a DV_TEXT;
// under the second it admits it. The ISM_TRANSITION's own rule reports the
// code, 999, as outside the openEHR states, at that defining_code: the
// value binds to the second alternative, so the refusal, and the withdrawal
// it would bring, is gone with the first trial.
func TestValidate_REQ171_WithdrawnRefusalKeepsTheNodeFinding(t *testing.T) {
	const textCode = `
          <children xsi:type="C_COMPLEX_OBJECT">
            <rm_type_name>DV_CODED_TEXT</rm_type_name>
            <node_id></node_id>
            <attributes xsi:type="C_SINGLE_ATTRIBUTE">
              <rm_attribute_name>defining_code</rm_attribute_name>
              <children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>DV_TEXT</rm_type_name><node_id></node_id></children>
            </attributes>
          </children>`
	stateAlternatives := func(children ...string) string {
		return `
      <children xsi:type="C_COMPLEX_OBJECT">
        <rm_type_name>ISM_TRANSITION</rm_type_name>
        <node_id>at0005</node_id>
        <attributes xsi:type="C_SINGLE_ATTRIBUTE">
          <rm_attribute_name>current_state</rm_attribute_name>` + strings.Join(children, "") + `
        </attributes>
      </children>`
	}
	value := func() *rm.Action { return transitionAction("999", "at0005") }
	const at = "/ism_transition/current_state/defining_code"
	refused := validation.Validate(value(), mustCompileInline(t, actionTransitionsOPT(stateAlternatives(textCode))))
	if want := []string{"rm_type_mismatch " + at}; !slices.Equal(issueKeys(refused.Issues), want) {
		t.Fatalf("Validate under the refusing alternative alone issues = %q, want %q", issueKeys(refused.Issues), want)
	}
	r := validation.Validate(value(), mustCompileInline(t, actionTransitionsOPT(stateAlternatives(textCode, codedChild("openehr", "999")))))
	if want := []string{"code_not_in_value_set " + at}; !slices.Equal(issueKeys(r.Issues), want) {
		t.Errorf("Validate(state 999) issues = %q, want %q: the node's own finding, which no refusal withdraws", issueKeys(r.Issues), want)
	}
}

// visitsNeeded returns the least visit limit under which validating root
// against c does not stop, searching up to ceiling.
func visitsNeeded(t *testing.T, root any, c *templatecompile.Compiled, ceiling int) int {
	t.Helper()
	for n := 1; n <= ceiling; n++ {
		r := validation.New(validation.WithLimits(validation.Limits{Nodes: n})).Validate(root, c)
		if !containsCode(r.Issues, "max_nodes") {
			return n
		}
	}
	t.Fatalf("validating %T still stops at %d visits", root, ceiling)
	return 0
}

// REQ-171 One walk — where the visit limit ends the walk inside the trial of
// a list item, the stop is reported at the path of the list's attribute,
// whatever the limit: the item's own path waits on the binding the stop left
// undecided. Of two members under two siblings that share an id and pin no
// name, the first is tried under the first sibling; every limit that ends
// the walk inside that trial reports max_nodes at /content alone, and, with
// a depth limit the trial reaches first, max_depth there too.
func TestValidate_REQ171_StopInsideAnItemTrialIsAtItsList(t *testing.T) {
	c := mustCompileSyntheticOPT(t, reusedArchetypeOPTWith(reusedObservationChild("", 0), reusedObservationChild("", 1)))
	root := func() any { return reusedComposition("Erste", "Zweite") }
	if r := validation.Validate(root(), c); !r.OK || len(r.Issues) != 0 {
		t.Fatalf("Validate within the default limits = OK %t, issues %q, want OK with none", r.OK, issueKeys(r.Issues))
	}
	tests := []struct {
		name  string
		depth int
		nodes []int
		want  []string
	}{
		{name: "the visit limit", nodes: []int{1, 2, 10, 40, 64}, want: []string{"max_nodes /content"}},
		{name: "the depth limit, then the visit limit", depth: 3, nodes: []int{4, 10, 40}, want: []string{"max_depth /content", "max_nodes /content"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for _, n := range tc.nodes {
				r := validation.New(validation.WithLimits(validation.Limits{Depth: tc.depth, Nodes: n})).Validate(root(), c)
				if got := issueKeys(r.Issues); !slices.Equal(got, tc.want) {
					t.Errorf("Validate within %d visits: issues %q, want %q", n, got, tc.want)
				}
				if want := slices.Repeat([]string{"/content"}, len(tc.want)); !slices.Equal(r.Coverage.Stopped, want) {
					t.Errorf("Validate within %d visits: Coverage.Stopped %q, want %q", n, r.Coverage.Stopped, want)
				}
			}
		})
	}
}

// REQ-171 One walk — the visits of a trial count toward the visit limit, and
// where the limit ends the walk inside a trial no binding is decided: the
// result holds one max_nodes, one stop in Coverage, and no finding from the
// trials. A transition that matches the second alternative needs more
// visits under both alternatives than under the second alone, since the
// trial under the first counts too; every limit below that ends the walk
// with max_nodes alone. The same holds of a list item tried under two
// siblings of one rank.
func TestValidate_REQ171_TrialVisitsCountTowardTheLimit(t *testing.T) {
	tests := []struct {
		name        string
		root        func() any
		pair, alone *templatecompile.Compiled
	}{
		{
			name:  "an alternative of a single-valued attribute",
			root:  func() any { return transitionAction("532", "at0006") },
			pair:  mustCompileInline(t, actionTransitionsOPT(initialTransition, completedTransition)),
			alone: mustCompileInline(t, actionTransitionsOPT(completedTransition)),
		},
		{
			name:  "a list item of a rank of two",
			root:  func() any { return reusedComposition("Erste") },
			pair:  mustCompileSyntheticOPT(t, reusedArchetypeOPTWith(reusedObservationWithData(t, "", 0, "at0099"), reusedObservationChild("", 0))),
			alone: mustCompileSyntheticOPT(t, reusedArchetypeOPTWith(reusedObservationChild("", 0))),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if r := validation.Validate(tc.root(), tc.pair); !r.OK || len(r.Issues) != 0 {
				t.Fatalf("Validate within the default limits = OK %t, issues %q, want OK with none", r.OK, issueKeys(r.Issues))
			}
			alone := visitsNeeded(t, tc.root(), tc.alone, 10_000)
			pair := visitsNeeded(t, tc.root(), tc.pair, 10_000)
			if pair <= alone {
				t.Errorf("visits needed under both = %d, under the bound child alone = %d, want more: the trial's visits count", pair, alone)
			}
			for n := 1; n < pair; n++ {
				r := validation.New(validation.WithLimits(validation.Limits{Nodes: n})).Validate(tc.root(), tc.pair)
				if got := issueKeys(r.Issues); len(got) != 1 || r.Issues[0].Code != "max_nodes" || len(r.Coverage.Stopped) != 1 {
					t.Errorf("Validate within %d visits: issues %q, Coverage.Stopped %q, want one max_nodes and one stop", n, got, r.Coverage.Stopped)
				}
			}
		})
	}
}

// REQ-171 One walk — the depth limit applies inside a trial, and the walk
// reports max_depth once, at one stop, whichever alternative it binds to. A
// transition that matches neither alternative binds to the first, whose
// findings within the depth limit it draws.
func TestValidate_REQ171_DepthLimitInsideATrial(t *testing.T) {
	c := mustCompileInline(t, actionTransitionsOPT(initialTransition, completedTransition))
	v := validation.New(validation.WithLimits(validation.Limits{Depth: 3}))
	for _, state := range []string{"532", "530"} {
		t.Run("state "+state, func(t *testing.T) {
			r := v.Validate(transitionAction(state, "at0006"), c)
			depth := 0
			for _, iss := range r.Issues {
				if iss.Code == "max_depth" {
					depth++
				}
			}
			if depth != 1 || len(r.Coverage.Stopped) != 1 {
				t.Errorf("Validate(state %s) within depth 3: issues %q, Coverage.Stopped %q, want one max_depth and one stop", state, issueKeys(r.Issues), r.Coverage.Stopped)
			}
		})
	}
}

// REQ-171 One walk — a verdict a trial decides binds a value the same way
// wherever the walk meets it again. An ELEMENT item, tried under the first
// of two ELEMENT children of one rank, binds its value, a DV_QUANTITY held by
// pointer, among two alternatives of other units: the walk under the child
// the item binds to then binds the value by the verdicts its trial decided.
// The value draws what it draws under one ELEMENT child, where nothing is
// tried twice: nothing where it validates under the second alternative, and
// the first alternative's findings where it validates under neither, with
// the same Coverage.
func TestValidate_REQ171_DecidedVerdictBindsAlike(t *testing.T) {
	alternatives := quantityIn("kg") + quantityIn("[lb_av]")
	ranked := mustCompileInline(t, clusterItemsOPT(alternatives, alternatives))
	alone := mustCompileInline(t, clusterItemsOPT(alternatives))
	cluster := func(q *rm.DVQuantity) any {
		return &rm.Cluster{
			ArchetypeNodeID:  "openEHR-EHR-CLUSTER.items.v1",
			Name:             rm.DVText{Value: "cluster"},
			ArchetypeDetails: archetypeDetails("openEHR-EHR-CLUSTER.items.v1"),
			Items:            []rm.Item{&rm.Element{ArchetypeNodeID: "at0001", Name: rm.DVText{Value: "element"}, Value: q}},
		}
	}
	for _, tc := range []struct {
		name  string
		value *rm.DVQuantity
		want  []string
	}{
		{name: "the second alternative", value: &rm.DVQuantity{Magnitude: 80, Units: "[lb_av]"}},
		{name: "neither alternative", value: &rm.DVQuantity{Magnitude: 2000, Units: "g"}, want: []string{"primitive_unit_unknown /items[at0001]/value"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, single := validation.Validate(cluster(tc.value), ranked), validation.Validate(cluster(tc.value), alone)
			if got := issueKeys(single.Issues); !slices.Equal(got, tc.want) {
				t.Fatalf("Validate(%v %s) under one ELEMENT child issues = %q, want %q", tc.value.Magnitude, tc.value.Units, got, tc.want)
			}
			if got := issueKeys(r.Issues); !slices.Equal(got, tc.want) {
				t.Errorf("Validate(%v %s) under two ELEMENT children issues = %q, want %q, as under one", tc.value.Magnitude, tc.value.Units, got, tc.want)
			}
			if !reflect.DeepEqual(r.Coverage, single.Coverage) {
				t.Errorf("Validate(%v %s) under two ELEMENT children Coverage = %+v, want %+v, as under one", tc.value.Magnitude, tc.value.Units, r.Coverage, single.Coverage)
			}
		})
	}
}

// REQ-171 One walk — inside the trial of a list item, a value below it whose
// verdict is that it does not validate is not walked again, and fails the
// trial all the same. A CLUSTER item is tried under the first of two CLUSTER
// children of one node id, whose items offer its ELEMENT two children, of
// kg and of g, and then under the second, which constrains nothing below
// it. Its ELEMENT, in pounds, validates under neither ELEMENT child, so the
// trial under the first CLUSTER child fails, and the item binds to the
// second and validates.
func TestValidate_REQ171_RefutedVerdictFailsTheTrial(t *testing.T) {
	elements := clusterItemsOPT(quantityIn("kg"), quantityIn("g"))
	const items = "<rm_attribute_name>items</rm_attribute_name>"
	start, end := strings.Index(elements, items)+len(items), strings.LastIndex(elements, "\n    </attributes>")
	if start < len(items) || end < start {
		t.Fatal("clusterItemsOPT no longer declares its items as this test reads them: update the test")
	}
	cluster := func(attrs string) string {
		return `
      <children xsi:type="C_COMPLEX_OBJECT">
        <rm_type_name>CLUSTER</rm_type_name>
        <occurrences>
          <lower_included>true</lower_included><upper_included>true</upper_included>
          <lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded>
          <lower>0</lower><upper>1</upper>
        </occurrences>
        <node_id>at0003</node_id>` + attrs + `
      </children>`
	}
	constrained := cluster(`
        <attributes xsi:type="C_MULTIPLE_ATTRIBUTE">
          <rm_attribute_name>items</rm_attribute_name>` + elements[start:end] + `
        </attributes>`)
	withChildren := func(children ...string) *templatecompile.Compiled {
		return mustCompileInline(t, elements[:start]+strings.Join(children, "")+elements[end:])
	}
	root := func() any {
		element := &rm.Element{ArchetypeNodeID: "at0001", Name: rm.DVText{Value: "element"}, Value: &rm.DVQuantity{Magnitude: 80, Units: "[lb_av]"}}
		return &rm.Cluster{
			ArchetypeNodeID:  "openEHR-EHR-CLUSTER.items.v1",
			Name:             rm.DVText{Value: "cluster"},
			ArchetypeDetails: archetypeDetails("openEHR-EHR-CLUSTER.items.v1"),
			Items:            []rm.Item{&rm.Cluster{ArchetypeNodeID: "at0003", Name: rm.DVText{Value: "inner"}, Items: []rm.Item{element}}},
		}
	}
	if r := validation.Validate(root(), withChildren(constrained)); r.OK {
		t.Fatalf("Validate under the first CLUSTER child alone is OK, want its ELEMENT refused, so the trial shows nothing")
	}
	if r := validation.Validate(root(), withChildren(constrained, cluster(""))); !r.OK || len(r.Issues) != 0 {
		t.Errorf("Validate under both CLUSTER children = OK %t, issues %q, want OK with none: the item binds to the second", r.OK, issueKeys(r.Issues))
	}
}

// declaredBefore returns opt with decl inserted before the declaration of
// the attribute name, or, with name empty, after the root's last attribute.
func declaredBefore(t *testing.T, opt, decl, name string) string {
	t.Helper()
	at := strings.Index(opt, "\n    <archetype_id>")
	if name != "" {
		attr := "\n      <rm_attribute_name>" + name + "</rm_attribute_name>"
		if strings.Count(opt, attr) != 1 {
			t.Fatalf("the OPT does not declare %q once at its root: update the insertion", name)
		}
		at = strings.LastIndex(opt[:strings.Index(opt, attr)], "\n    <attributes")
	}
	if at < 0 {
		t.Fatalf("the OPT has no place to declare an attribute before %q: update the insertion", name)
	}
	return opt[:at] + decl + opt[at:]
}

// REQ-171 One walk — a trial the depth limit stops does not validate,
// whether the walk reports that stop inside the trial or reported max_depth
// already, at a place walked before it. So the binding does not turn on the
// order the template declares its attributes in: with an attribute the
// depth limit stops declared before the one bound by trial, and after it,
// the value binds to the first child, under which every trial stops, and
// draws its finding. It holds of a single-valued attribute, an ACTION's
// ISM_TRANSITION whose first alternative requires a transition the value
// lacks, and of a list item, an ELEMENT whose first child admits other
// units than the value's.
func TestValidate_REQ171_DepthStopFailsATrialWhetherReportedThereOrNot(t *testing.T) {
	const (
		requiresTransition = `
        <attributes xsi:type="C_SINGLE_ATTRIBUTE">
          <rm_attribute_name>transition</rm_attribute_name>
          <existence><lower>1</lower><upper>1</upper></existence>
          <children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>DV_CODED_TEXT</rm_type_name><node_id></node_id></children>
        </attributes>`
		description = `
    <attributes xsi:type="C_SINGLE_ATTRIBUTE">
      <rm_attribute_name>description</rm_attribute_name>
      <existence><lower>1</lower><upper>1</upper></existence>
      <children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>ITEM_TREE</rm_type_name><node_id>at0017</node_id></children>
    </attributes>`
		detailsDecl = `
    <attributes xsi:type="C_SINGLE_ATTRIBUTE">
      <rm_attribute_name>archetype_details</rm_attribute_name>
    </attributes>`
	)
	pounds := func() any {
		return &rm.Cluster{
			ArchetypeNodeID:  "openEHR-EHR-CLUSTER.items.v1",
			Name:             rm.DVText{Value: "cluster"},
			ArchetypeDetails: archetypeDetails("openEHR-EHR-CLUSTER.items.v1"),
			Items:            []rm.Item{&rm.Element{ArchetypeNodeID: "at0001", Name: rm.DVText{Value: "element"}, Value: rm.DVQuantity{Magnitude: 80, Units: "[lb_av]"}}},
		}
	}
	tests := []struct {
		name string
		// opt is the template; bound names the attribute bound by trial,
		// and earlier declares the attribute the depth limit stops first.
		opt, bound, earlier string
		root                func() any
		depth               int
		// want is the finding the value draws under the first child.
		want string
	}{
		{
			name:    "an alternative of a single-valued attribute",
			opt:     actionTransitionsOPT(transitionChild("at0005", "532", "at0006", requiresTransition), completedTransition),
			bound:   "ism_transition",
			earlier: description,
			root:    func() any { return transitionAction("532", "at0006") },
			depth:   2,
			want:    "required /ism_transition/transition",
		},
		{
			name:    "a list item of a rank of two",
			opt:     clusterItemsOPT(quantityIn("kg"), quantityIn("[lb_av]")),
			bound:   "items",
			earlier: detailsDecl,
			root:    pounds,
			depth:   2,
			want:    "primitive_unit_unknown /items[at0001]/value",
		},
	}
	for _, tc := range tests {
		for _, order := range []struct {
			name   string
			before string
		}{{name: "declared after", before: ""}, {name: "declared before", before: tc.bound}} {
			t.Run(tc.name+"/"+order.name, func(t *testing.T) {
				c := mustCompileInline(t, declaredBefore(t, tc.opt, tc.earlier, order.before))
				r := validation.New(validation.WithLimits(validation.Limits{Depth: tc.depth})).Validate(tc.root(), c)
				var others []string
				depth := 0
				for _, iss := range r.Issues {
					if iss.Code == "max_depth" {
						depth++
						continue
					}
					others = append(others, iss.Code+" "+iss.Path)
				}
				if depth != 1 || len(r.Coverage.Stopped) != 1 {
					t.Errorf("Validate within depth %d: issues %q, Coverage.Stopped %q, want one max_depth and one stop", tc.depth, issueKeys(r.Issues), r.Coverage.Stopped)
				}
				if want := []string{tc.want}; !slices.Equal(others, want) {
					t.Errorf("Validate within depth %d: issues %q, want max_depth and %q: every trial stops, so the value binds to the first child", tc.depth, issueKeys(r.Issues), want)
				}
			})
		}
	}
}

// REQ-107 Exceptions and known gaps, REQ-102 Structural — binding items —
// the generator gives an optional sibling no member of its own where a
// later, required one carries its identity and neither pins a name. The
// member it builds for the required one validates under the optional one,
// which comes first and has room, so the validator binds it there, and the
// root fails by the required sibling's occurrences, under Large and Medium
// alike. A composition that holds a member of each validates
// (TestValidateComposition_REQ102_ItemsOfOneRankBindByRoomAndTrial).
func TestGenerate_REQ107_REQ102_OptionalSiblingBeforeARequiredOneKeepsItsGap(t *testing.T) {
	c := mustCompileSyntheticOPT(t, reusedArchetypeOPTWith(reusedObservationChild("", 0), reusedObservationChild("", 1)))
	for _, policy := range []instance.Policy{instance.Large, instance.Medium} {
		t.Run(fmt.Sprint(policy), func(t *testing.T) {
			out, err := instance.Generate(t.Context(), c, instance.Options{
				Policy:    policy,
				Territory: "NL",
				Composer:  &rm.PartyIdentified{Name: new("Dr Jones")},
				Now:       time.Date(2026, 10, 25, 9, 0, 0, 0, time.UTC),
			})
			if err != nil {
				t.Fatalf("Generate: %v", err)
			}
			comp, ok := out.(*rm.Composition)
			if !ok {
				t.Fatalf("Generate root = %T, want *rm.Composition", out)
			}
			if len(comp.Content) != 1 {
				t.Fatalf("Generate built %d content items, want the required sibling's member alone", len(comp.Content))
			}
			r := validation.ValidateComposition(comp, c)
			want := []string{"cardinality /content[" + reusedID + "]"}
			if got := issueKeys(r.Issues); !slices.Equal(got, want) || !strings.Contains(r.Issues[0].Detail, "appears 0 times") {
				t.Errorf("ValidateComposition(generated) issues = %q (%v), want %q: the required sibling appears 0 times", got, r.Issues, want)
			}
		})
	}
}

// REQ-102 Structural — alternatives — two slots of one RM type under a
// single-valued attribute share the first tier, and a slot is a child the
// value validates under only where its assertions admit the value's
// archetype id. A protocol the second slot admits binds to it and draws no
// finding; one neither admits binds to the first, which refuses it once, as
// slot_fill.
func TestValidateComposition_REQ102_SlotAlternativesBindByTheirAssertions(t *testing.T) {
	start := strings.Index(singleAttributeSlotOPT, `<children xsi:type="ARCHETYPE_SLOT">`)
	end := strings.Index(singleAttributeSlotOPT, `</includes>`)
	if start < 0 || end < start {
		t.Fatal("singleAttributeSlotOPT holds no protocol slot: update the test")
	}
	end = strings.Index(singleAttributeSlotOPT[end:], `</children>`) + end + len(`</children>`)
	slot := singleAttributeSlotOPT[start:end]
	other := strings.NewReplacer(`at9000`, `at9001`, `allowed`, `other`).Replace(slot)
	c := mustCompileSyntheticOPT(t, singleAttributeSlotOPT[:end]+other+singleAttributeSlotOPT[end:])
	for _, tc := range []struct {
		protocol string
		want     []string
	}{
		{protocol: "openEHR-EHR-ITEM_TREE.allowed.v1"},
		{protocol: "openEHR-EHR-ITEM_TREE.other.v1"},
		{protocol: "openEHR-EHR-ITEM_TREE.rejected.v1", want: []string{"slot_fill /content[openEHR-EHR-OBSERVATION.blood_pressure.v1]/protocol"}},
	} {
		comp := validSingleAttributeSlotComposition(tc.protocol)
		comp.ArchetypeDetails = archetypeDetails(comp.ArchetypeNodeID)
		r := validation.ValidateComposition(comp, c)
		if got := issueKeys(r.Issues); !slices.Equal(got, tc.want) {
			t.Errorf("ValidateComposition(protocol %s) issues = %q, want %q", tc.protocol, got, tc.want)
		}
	}
}

// REQ-102 Structural — binding items — the item's name ranks the children
// that carry its id before any trial: those that pin its name, failing any,
// those that pin no name, failing any, all of them. An item named Erste
// binds to the required child that pins Erste, though the optional child
// before it pins no name and admits it too. An item whose name no child
// pins binds among the children that pin none, so, carrying a defect that
// fails it everywhere, it binds to the child that pins no name and draws
// that defect alone, not the name of the child that pins Erste before it.
func TestValidateComposition_REQ102_ItemNameRanksTheChildren(t *testing.T) {
	const at = "/content[" + reusedID + "]"
	t.Run("a child that pins the name", func(t *testing.T) {
		c := mustCompileSyntheticOPT(t, reusedArchetypeOPTWith(reusedObservationChild("", 0), reusedObservationChild("Erste", 1)))
		if got := issueKeys(validation.ValidateComposition(reusedComposition("Erste"), c).Issues); len(got) != 0 {
			t.Errorf("ValidateComposition(Erste) issues = %q, want none: it binds to the child that pins Erste", got)
		}
	})
	t.Run("a child that pins no name", func(t *testing.T) {
		c := mustCompileSyntheticOPT(t, reusedArchetypeOPTWith(reusedObservationChild("Erste", 0), reusedObservationChild("", 0)))
		r := validation.ValidateComposition(withoutLanguage(t, reusedComposition("Andere"), 0), c)
		if got, want := issueKeys(r.Issues), emptyLanguage(at); !slices.Equal(got, want) {
			t.Errorf("ValidateComposition(Andere, no language) issues = %q, want %q: it binds to the child that pins no name", got, want)
		}
	})
}
