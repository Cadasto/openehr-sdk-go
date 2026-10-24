package validation_test

import (
	"fmt"
	"log"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// Check a value against the Reference Model alone, with no template. An
// ELEMENT carries exactly one of value and null_flavour. The element below
// has neither, so the check reports it.
func ExampleValidateRM() {
	cluster := &rm.Cluster{
		ArchetypeNodeID: "at0001",
		Name:            rm.DVText{Value: "Blood pressure"},
		Items: []rm.Item{
			&rm.Element{
				ArchetypeNodeID: "at0004",
				Name:            rm.DVText{Value: "Systolic"},
			},
		},
	}

	result := validation.ValidateRM(cluster)
	fmt.Println("OK:", result.OK)
	for _, issue := range result.Issues {
		fmt.Println(issue.Code, "at", issue.Path)
	}
	// Output:
	// OK: false
	// rm_invariant at /items[at0004]
}

// reportOPT is a minimal operational template: a COMPOSITION whose content
// is required, by its existence and by a cardinality of at least one item,
// and holds a blood pressure OBSERVATION.
const reportOPT = `<?xml version="1.0" encoding="UTF-8"?>
<template xmlns="http://schemas.openehr.org/v1" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <template_id><value>example_report</value></template_id>
  <concept>example_report</concept>
  <definition>
    <rm_type_name>COMPOSITION</rm_type_name>
    <node_id>at0000</node_id>
    <archetype_id><value>openEHR-EHR-COMPOSITION.report.v1</value></archetype_id>
    <attributes xsi:type="C_MULTIPLE_ATTRIBUTE">
      <rm_attribute_name>content</rm_attribute_name>
      <existence>
        <lower_included>true</lower_included><upper_included>true</upper_included>
        <lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded>
        <lower>1</lower><upper>1</upper>
      </existence>
      <children xsi:type="C_COMPLEX_OBJECT">
        <rm_type_name>OBSERVATION</rm_type_name>
        <node_id>at0000</node_id>
        <archetype_id><value>openEHR-EHR-OBSERVATION.blood_pressure.v1</value></archetype_id>
      </children>
      <cardinality>
        <is_ordered>true</is_ordered><is_unique>false</is_unique>
        <interval>
          <lower_included>true</lower_included><upper_included>false</upper_included>
          <lower_unbounded>false</lower_unbounded><upper_unbounded>true</upper_unbounded>
          <lower>1</lower>
        </interval>
      </cardinality>
    </attributes>
  </definition>
</template>`

// Check a composition against a template. The result includes the Reference
// Model floor, so one call reports what the template requires and what the
// Reference Model requires; ValidateRM reports the floor alone.
func ExampleValidateComposition() {
	opt, err := template.ParseOPTStrict(strings.NewReader(reportOPT))
	if err != nil {
		log.Fatal(err)
	}
	compiled, err := templatecompile.Compile(opt)
	if err != nil {
		log.Fatal(err)
	}

	// The template requires content and the composition has none. The
	// composition also leaves its rm_version empty, which the Reference
	// Model forbids.
	composition := &rm.Composition{
		ArchetypeNodeID: "openEHR-EHR-COMPOSITION.report.v1",
		ArchetypeDetails: &rm.Archetyped{
			ArchetypeID: rm.ArchetypeID{Value: "openEHR-EHR-COMPOSITION.report.v1"},
		},
		Name: rm.DVText{Value: "Report"},
		Category: rm.DVCodedText{
			DVText: rm.DVText{Value: "event"},
			DefiningCode: rm.CodePhrase{
				TerminologyID: rm.TerminologyID{Value: "openehr"},
				CodeString:    "433",
			},
		},
		Composer: rm.PartySelf{},
		Language: rm.CodePhrase{
			TerminologyID: rm.TerminologyID{Value: "ISO_639-1"},
			CodeString:    "en",
		},
		Territory: rm.CodePhrase{
			TerminologyID: rm.TerminologyID{Value: "ISO_3166-1"},
			CodeString:    "NL",
		},
	}

	byTemplate := validation.ValidateComposition(composition, compiled)
	fmt.Printf("template: OK=%t issues=%d\n", byTemplate.OK, len(byTemplate.Issues))
	for _, issue := range byTemplate.Issues {
		fmt.Printf("  %s at %s\n", issue.Code, issue.Path)
	}

	byRM := validation.ValidateRM(composition)
	fmt.Printf("reference model: OK=%t issues=%d\n", byRM.OK, len(byRM.Issues))
	for _, issue := range byRM.Issues {
		fmt.Printf("  %s at %s\n", issue.Code, issue.Path)
	}
	// Output:
	// template: OK=false issues=2
	//   required at /content
	//   rm_version_valid at /archetype_details/rm_version
	// reference model: OK=false issues=1
	//   rm_version_valid at /archetype_details/rm_version
}

// Build a validator with a lower depth limit and read what the result did
// not check. The SECTIONs below nest three deep; under a depth limit of 2 the
// walk stops at the name of the second, reports it once as max_depth, and
// lists that path in the result's coverage.
func ExampleNew() {
	bottom := &rm.Section{ArchetypeNodeID: "at0003", Name: rm.DVText{Value: "Bottom"}}
	middle := &rm.Section{ArchetypeNodeID: "at0002", Name: rm.DVText{Value: "Middle"}, Items: []rm.ContentItem{bottom}}
	top := &rm.Section{ArchetypeNodeID: "at0001", Name: rm.DVText{Value: "Top"}, Items: []rm.ContentItem{middle}}

	v := validation.New(validation.WithLimits(validation.Limits{Depth: 2}))
	result := v.Validate(top, nil)
	fmt.Println("OK:", result.OK)
	for _, issue := range result.Issues {
		fmt.Println(issue.Code, "at", issue.Path)
	}
	fmt.Println("complete:", result.Coverage.Complete())
	fmt.Println("stopped at:", result.Coverage.Stopped)
	// Output:
	// OK: false
	// max_depth at /items[at0002]/name
	// complete: false
	// stopped at: [/items[at0002]/name]
}
