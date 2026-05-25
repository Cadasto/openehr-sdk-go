// Example: parse a minimal OPT with a DV_QUANTITY primitive constraint,
// resolve a path, and call PrimitiveConstraint.Validate (REQ-103).
// Demonstrates the smallest clinical-modeling constraint path — no
// compiled template, no validation walker, no RM composition fixture.
//
// Run:
//
//	go run ./cmd/examples/primitive-validate
//
// The embedded OPT is the same shape as PROBE-024's synthetic fixture
// (magnitude 0..300, units mm[Hg] only).
package main

import (
	"fmt"
	"log"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/template/constraints"
)

// minimalQuantityOPT is a tiny OPT with one C_DV_QUANTITY leaf at /content.
const minimalQuantityOPT = `<?xml version="1.0" encoding="UTF-8"?>
<template xmlns="http://schemas.openehr.org/v1" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <template_id><value>example_primitive</value></template_id>
  <concept>example_primitive</concept>
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

func main() {
	tmpl, err := template.ParseOPT(strings.NewReader(minimalQuantityOPT))
	if err != nil {
		log.Fatalf("ParseOPT: %v", err)
	}
	fmt.Printf("template_id : %s\n", tmpl.TemplateID())

	p, err := tmpl.ParsePath("/content")
	if err != nil {
		log.Fatalf("ParsePath(/content): %v", err)
	}
	node, err := tmpl.NodeAt(p)
	if err != nil {
		log.Fatalf("NodeAt(/content): %v", err)
	}
	co, ok := node.(*template.ComplexObject)
	if !ok {
		log.Fatalf("node at /content: want *ComplexObject, got %T", node)
	}
	primitive := co.PrimitiveConstraint()
	if primitive == nil {
		log.Fatal("PrimitiveConstraint is nil at /content")
	}
	fmt.Printf("constraint  : %T at /content\n", primitive)

	cases := []struct {
		label string
		value constraints.QuantityValue
	}{
		{"in-range", constraints.QuantityValue{Magnitude: 120, Units: "mm[Hg]"}},
		{"out-of-range magnitude", constraints.QuantityValue{Magnitude: 500, Units: "mm[Hg]"}},
		{"unknown unit", constraints.QuantityValue{Magnitude: 50, Units: "psi"}},
	}

	var failures int
	for _, c := range cases {
		vs := primitive.Validate(c.value)
		if len(vs) == 0 {
			fmt.Printf("  %-22s OK\n", c.label)
			continue
		}
		failures++
		fmt.Printf("  %-22s %d violation(s)\n", c.label, len(vs))
		for _, v := range vs {
			fmt.Printf("    [%s] %s\n", v.Code, v.Detail)
		}
	}
	if failures > 0 {
		fmt.Printf("summary     : %d/%d cases failed validation (expected for demo)\n", failures, len(cases))
		return
	}
	fmt.Println("summary     : all cases passed")
}
