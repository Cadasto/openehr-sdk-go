// Example: validate single values against one leaf constraint of an
// operational template (OPT), without compiling the template or building a
// composition. The embedded OPT constrains a DV_QUANTITY to a magnitude of
// 0..300 in mm[Hg]; the program resolves that leaf by path and checks three
// values against it, two of which fail on purpose.
//
// Runs offline and needs no fixture; the template is embedded below:
//
//	go run ./cmd/examples/primitive-validate
package main

import (
	"fmt"
	"log"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/template/constraints"
)

// minimalQuantityOPT is the smallest OPT with one primitive leaf: a
// C_DV_QUANTITY directly under the COMPOSITION's content attribute. Real
// templates nest such leaves many levels deep, but the constraint itself has
// the same shape.
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
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	// Step 1: parse the template. ParseOPT reads from any io.Reader; ParseFile
	// is the convenience for a path on disk.
	opt, err := template.ParseOPT(strings.NewReader(minimalQuantityOPT))
	if err != nil {
		return fmt.Errorf("parse OPT: %w", err)
	}
	fmt.Printf("template_id : %s\n", opt.TemplateID())

	// Step 2: find the leaf and its typed constraint.
	constraint, err := primitiveConstraintAt(opt, "/content")
	if err != nil {
		return err
	}
	fmt.Printf("constraint  : %T at /content\n", constraint)

	// Step 3: check values. QuantityValue is the small value shape the
	// constraint accepts, so a caller can check one number without building
	// an rm.DVQuantity first.
	cases := []struct {
		label string
		value constraints.QuantityValue
	}{
		{"in-range", constraints.QuantityValue{Magnitude: 120, Units: "mm[Hg]"}},
		{"out-of-range magnitude", constraints.QuantityValue{Magnitude: 500, Units: "mm[Hg]"}},
		{"unknown unit", constraints.QuantityValue{Magnitude: 50, Units: "psi"}},
	}
	var failures int
	for _, tc := range cases {
		// Validate returns nil when the value satisfies every clause, and
		// otherwise one Violation per clause that failed.
		violations := constraint.Validate(tc.value)
		if len(violations) == 0 {
			fmt.Printf("  %-22s OK\n", tc.label)
			continue
		}
		failures++
		fmt.Printf("  %-22s %d violation(s)\n", tc.label, len(violations))
		for _, violation := range violations {
			// Code is the stable identifier a program branches on; Detail
			// is the explanation for people.
			fmt.Printf("    [%s] %s\n", violation.Code, violation.Detail)
		}
	}
	if failures > 0 {
		fmt.Printf("summary     : %d/%d cases failed validation (expected for demo)\n", failures, len(cases))
		return nil
	}
	fmt.Println("summary     : all cases passed")
	return nil
}

// primitiveConstraintAt resolves a template path to its node and returns the
// typed value constraint on it. Only leaf nodes such as DV_QUANTITY or
// CODE_PHRASE carry one; on a structural node PrimitiveConstraint is nil.
func primitiveConstraintAt(opt *template.OperationalTemplate, path string) (constraints.PrimitiveConstraint, error) {
	// ParsePath checks the path syntax once; NodeAt then walks the tree.
	parsed, err := opt.ParsePath(path)
	if err != nil {
		return nil, fmt.Errorf("parse path %s: %w", path, err)
	}
	node, err := opt.NodeAt(parsed)
	if err != nil {
		return nil, fmt.Errorf("resolve %s: %w", path, err)
	}

	// NodeAt returns the tree's Node interface. The value constraint lives on
	// *template.ComplexObject, the node kind that stands for an RM object.
	object, ok := node.(*template.ComplexObject)
	if !ok {
		return nil, fmt.Errorf("node at %s is %T, want *template.ComplexObject", path, node)
	}
	constraint := object.PrimitiveConstraint()
	if constraint == nil {
		return nil, fmt.Errorf("node at %s carries no primitive constraint", path)
	}
	return constraint, nil
}
