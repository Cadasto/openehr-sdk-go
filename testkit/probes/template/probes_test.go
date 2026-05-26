package templateprobes_test

import (
	"os"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/template/constraints"
	probes "github.com/cadasto/openehr-sdk-go/testkit/probes/template"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// PROBE-022 — fixture-driven assertion that the OPT parser resolves
// known paths to the expected RM types, node ids, and (for archetype
// roots) archetype ids. Uses the vital_signs.opt fixture vendored
// under openehr/template/testdata/.
func TestProbe022OPTPathResolution_VitalSigns(t *testing.T) {
	body := loadFixture(t, "vital_signs")
	assertions := []probes.PathAssertion{
		{Path: "/", WantRMType: "COMPOSITION", WantNodeID: "at0000"},
		{Path: "/category", WantRMType: "DV_CODED_TEXT"},
		{Path: "/content", WantRMType: "OBSERVATION"},
		{
			Path:            "/content[openEHR-EHR-OBSERVATION.blood_pressure.v1]",
			WantRMType:      "OBSERVATION",
			WantArchetypeID: "openEHR-EHR-OBSERVATION.blood_pressure.v1",
		},
		// At-code predicate — every OBSERVATION archetype root in
		// vital_signs.opt carries at0000 as its own node id. Exercises
		// the at-code branch of matchesPredicate (REQ-100 § Resolution
		// semantics).
		{Path: "/content[at0000]", WantRMType: "OBSERVATION", WantNodeID: "at0000"},
		{Path: "/no_such_attribute", ExpectNotFound: true},
		{Path: "/content[at9999]", ExpectNotFound: true},
	}
	r, err := probes.Probe022OPTPathResolution(body, assertions)
	if err != nil {
		t.Fatalf("Probe022: %v", err)
	}
	if r.Status != "pass" {
		t.Fatalf("Probe022 status=%q detail=%q", r.Status, r.Detail)
	}
	if r.Probe != "PROBE-022" {
		t.Errorf("Probe id = %q, want PROBE-022", r.Probe)
	}
}

// PROBE-022 — second fixture body (clinical_notes.v0). Confirms the
// probe runs against structurally distinct OPTs, not just one.
func TestProbe022OPTPathResolution_ClinicalNote(t *testing.T) {
	body := loadFixture(t, "clinical_note")
	assertions := []probes.PathAssertion{
		{Path: "/", WantRMType: "COMPOSITION"},
		{
			Path:            "/content[openEHR-EHR-OBSERVATION.story.v1]",
			WantRMType:      "OBSERVATION",
			WantArchetypeID: "openEHR-EHR-OBSERVATION.story.v1",
		},
	}
	r, err := probes.Probe022OPTPathResolution(body, assertions)
	if err != nil {
		t.Fatalf("Probe022: %v", err)
	}
	if r.Status != "pass" {
		t.Fatalf("Probe022 status=%q detail=%q", r.Status, r.Detail)
	}
}

// PROBE-022 — contradiction precedence. An assertion that sets both
// ExpectNotFound and a positive want (WantRMType / WantNodeID /
// WantArchetypeID) is a caller bug, not a fixture mismatch. The
// probe MUST satisfy ExpectNotFound first (the negative branch
// short-circuits before positive-want checks run), so a path that
// genuinely does not exist passes regardless of the positive wants.
// Documents the precedence rule for harness authors.
func TestPathAssertion_PrecedenceContradiction(t *testing.T) {
	body := loadFixture(t, "vital_signs")
	r, err := probes.Probe022OPTPathResolution(body, []probes.PathAssertion{
		{Path: "/no_such_attribute", ExpectNotFound: true, WantRMType: "DV_TEXT"},
	})
	if err != nil {
		t.Fatalf("Probe022: %v", err)
	}
	// ExpectNotFound short-circuits; the positive WantRMType is
	// ignored on the negative branch. Validates the documented
	// precedence: negative-first.
	if r.Status != "pass" {
		t.Fatalf("ExpectNotFound must short-circuit before WantRMType; status=%q detail=%q", r.Status, r.Detail)
	}
}

// PROBE-022 — malformed OPT MUST surface as a failed probe Result
// (not a Go error), so cross-SDK harnesses can aggregate failures.
func TestProbe022OPTPathResolution_InvalidOPT(t *testing.T) {
	r, err := probes.Probe022OPTPathResolution([]byte("<bad/>"), []probes.PathAssertion{{Path: "/"}})
	if err != nil {
		t.Fatalf("expected probe Result, got error: %v", err)
	}
	if r.Status != "fail" {
		t.Errorf("status = %q, want fail for invalid OPT", r.Status)
	}
}

// PROBE-022 — caller misuse (empty assertions) is a Go error, not a
// probe failure; harnesses MUST not silently pass an empty list.
func TestProbe022OPTPathResolution_RejectsEmptyAssertions(t *testing.T) {
	body := loadFixture(t, "vital_signs")
	_, err := probes.Probe022OPTPathResolution(body, nil)
	if err == nil {
		t.Fatal("expected Go error for nil assertions")
	}
}

// PROBE-022 — ExpectNotFound MUST be satisfied by ErrPathNotFound
// specifically, not by any error type. Self-review finding #1 from
// PR #10 multi-agent review.
func TestProbe022OPTPathResolution_ExpectNotFoundRequiresSentinel(t *testing.T) {
	body := loadFixture(t, "vital_signs")
	// A syntactically invalid path triggers ParsePath (ErrPathSyntax)
	// before NodeAt; the probe MUST report this as a parse failure,
	// not silently accept it as "not found".
	r, err := probes.Probe022OPTPathResolution(body, []probes.PathAssertion{
		{Path: "no_leading_slash", ExpectNotFound: true},
	})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if r.Status != "fail" {
		t.Errorf("status = %q, want fail (ParsePath error must not satisfy ExpectNotFound)", r.Status)
	}
}

// PROBE-024 — primitive constraint Validate against fixture cases.
// Uses a small synthetic OPT carrying a C_DV_QUANTITY child so the
// probe surface is exercised end-to-end (parse → resolve → validate)
// without depending on the much larger vital_signs.opt's actual
// path predicates.
func TestProbe024PrimitiveValidate_Synthetic(t *testing.T) {
	body := []byte(syntheticDvQuantityOPT)
	cases := []probes.ValidateCase{
		{
			Path:      "/content",
			Value:     constraints.QuantityValue{Magnitude: 120, Units: "mm[Hg]"},
			WantCodes: nil, // in-range, allowed units
		},
		{
			Path:      "/content",
			Value:     constraints.QuantityValue{Magnitude: 500, Units: "mm[Hg]"},
			WantCodes: []constraints.ViolationCode{constraints.CodeOutOfRange},
		},
		{
			Path:      "/content",
			Value:     constraints.QuantityValue{Magnitude: 50, Units: "psi"},
			WantCodes: []constraints.ViolationCode{constraints.CodeUnitUnknown},
		},
	}
	r, err := probes.Probe024PrimitiveValidate(body, cases)
	if err != nil {
		t.Fatalf("Probe024: %v", err)
	}
	if r.Status != "pass" {
		t.Fatalf("Probe024 status=%q detail=%q", r.Status, r.Detail)
	}
	if r.Probe != "PROBE-024" {
		t.Errorf("Probe id = %q, want PROBE-024", r.Probe)
	}
}

// PROBE-024 — caller misuse (empty cases) surfaces as a Go error,
// matching PROBE-022's empty-assertions contract.
func TestProbe024PrimitiveValidate_RejectsEmptyCases(t *testing.T) {
	if _, err := probes.Probe024PrimitiveValidate([]byte(syntheticDvQuantityOPT), nil); err == nil {
		t.Fatal("expected Go error for nil cases")
	}
}

// PROBE-024 — malformed OPT surfaces as a failed Result, not an error
// (cross-SDK aggregators bucket Results, not panics).
func TestProbe024PrimitiveValidate_InvalidOPT(t *testing.T) {
	r, err := probes.Probe024PrimitiveValidate([]byte("<bad/>"), []probes.ValidateCase{{Path: "/"}})
	if err != nil {
		t.Fatalf("expected Result, got error: %v", err)
	}
	if r.Status != "fail" {
		t.Errorf("status = %q, want fail for invalid OPT", r.Status)
	}
}

// syntheticDvQuantityOPT is a minimal OPT whose only child node is a
// DV_QUANTITY constraint at /content. Lives here (not testdata/)
// because the probe is the only consumer.
const syntheticDvQuantityOPT = `<?xml version="1.0" encoding="UTF-8"?>
<template xmlns="http://schemas.openehr.org/v1" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <template_id><value>probe024_test</value></template_id>
  <concept>probe024</concept>
  <definition xsi:type="C_COMPLEX_OBJECT">
    <rm_type_name>COMPOSITION</rm_type_name>
    <node_id>at0000</node_id>
    <attributes xsi:type="C_SINGLE_ATTRIBUTE">
      <rm_attribute_name>content</rm_attribute_name>
      <children xsi:type="C_DV_QUANTITY">
        <rm_type_name>DV_QUANTITY</rm_type_name>
        <node_id />
        <list>
          <magnitude>
            <lower_included>true</lower_included>
            <upper_included>true</upper_included>
            <lower_unbounded>false</lower_unbounded>
            <upper_unbounded>false</upper_unbounded>
            <lower>0</lower>
            <upper>300</upper>
          </magnitude>
          <units>mm[Hg]</units>
        </list>
      </children>
    </attributes>
  </definition>
</template>`

func loadFixture(t *testing.T, name string) []byte {
	t.Helper()
	body, err := os.ReadFile(fixtures.TemplateOptForName(name)) //nolint:gosec // fixture path is test-controlled
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return body
}
