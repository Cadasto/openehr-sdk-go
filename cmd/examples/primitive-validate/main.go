// Example: validate single values against one leaf constraint of an
// operational template (OPT), without building a composition. The embedded
// OPT constrains a DV_QUANTITY to a magnitude of 0..300 in mm[Hg]; the
// program reads it with aom14.ReadOperationalTemplate, compiles it with
// templatecompile.Compile, resolves that leaf by path in the compiled
// template, and checks three values against its constraint, two of which
// fail on purpose.
//
// Runs offline and needs no fixture; the template is embedded below:
//
//	go run ./cmd/examples/primitive-validate
package main

import (
	"fmt"
	"log"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/aom/aom14"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/template/constraints"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
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
	// Step 1: read the template into the operational template model and
	// compile it. ReadOperationalTemplate reads from any io.Reader and by
	// default reads a node type it does not know as a leaf without its
	// subtree; canxml.WithStrict() and templatecompile.WithStrict() refuse
	// such readings.
	opt, err := aom14.ReadOperationalTemplate(strings.NewReader(minimalQuantityOPT))
	if err != nil {
		return fmt.Errorf("read OPT: %w", err)
	}
	compiled, err := templatecompile.Compile(opt)
	if err != nil {
		return fmt.Errorf("compile template: %w", err)
	}
	fmt.Printf("template_id : %s\n", compiled.TemplateID())

	// Step 2: find the leaf and its typed constraint.
	constraint, err := primitiveConstraintAt(compiled, "/content")
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
			// is the explanation for people. Detail never carries the value
			// itself, so it is safe to log or send back to a client.
			// Value.Reveal() is the explicit step that reads the value: take
			// it only where showing the value is safe, as here, where the
			// value is the program's own demo input.
			fmt.Printf("    [%s] %s (submitted: %v)\n", violation.Code, violation.Detail, violation.Value.Reveal())
		}
	}
	if failures > 0 {
		fmt.Printf("summary     : %d/%d cases failed validation (expected for demo)\n", failures, len(cases))
		return nil
	}
	fmt.Println("summary     : all cases passed")
	return nil
}

// primitiveConstraintAt resolves a template path to its node in the compiled
// template and returns the typed value constraint on it. Only leaf nodes such
// as DV_QUANTITY or CODE_PHRASE carry one; on a structural node
// PrimitiveConstraint is nil.
func primitiveConstraintAt(compiled *templatecompile.Compiled, path string) (constraints.PrimitiveConstraint, error) {
	// ParsePath checks the path syntax once and needs no template; Resolve
	// then walks the compiled tree.
	parsed, err := template.ParsePath(path)
	if err != nil {
		return nil, fmt.Errorf("parse path %s: %w", path, err)
	}
	node, err := compiled.Resolve(parsed)
	if err != nil {
		return nil, fmt.Errorf("resolve %s: %w", path, err)
	}
	constraint := node.PrimitiveConstraint()
	if constraint == nil {
		return nil, fmt.Errorf("node at %s carries no primitive constraint", path)
	}
	return constraint, nil
}
