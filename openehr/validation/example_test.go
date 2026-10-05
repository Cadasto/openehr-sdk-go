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
	// rm_invariant at /items[0]
}

// reportOPT is a minimal operational template: a COMPOSITION whose content
// is required and holds a blood pressure OBSERVATION.
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
    </attributes>
  </definition>
</template>`

// Check a composition against a template, then against the Reference Model.
// The two checks are separate calls that report their own findings: run both
// when you want both.
func ExampleValidateComposition() {
	opt, err := template.ParseOPTStrict(strings.NewReader(reportOPT))
	if err != nil {
		log.Fatal(err)
	}
	compiled, err := templatecompile.Compile(opt)
	if err != nil {
		log.Fatal(err)
	}

	// The composition has every attribute the Reference Model requires, but
	// the template requires content and the composition has none.
	composition := &rm.Composition{
		ArchetypeNodeID: "openEHR-EHR-COMPOSITION.report.v1",
		ArchetypeDetails: &rm.Archetyped{
			ArchetypeID: rm.ArchetypeID{Value: "openEHR-EHR-COMPOSITION.report.v1"},
			RMVersion:   "1.1.0",
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
	// template: OK=false issues=1
	//   required at /content
	// reference model: OK=true issues=0
}
