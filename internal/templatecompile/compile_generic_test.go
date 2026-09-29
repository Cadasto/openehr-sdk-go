package templatecompile_test

import (
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
)

// REQ-100 — an OPT node declared as a generic instantiation
// (DV_INTERVAL<DV_QUANTITY>) gets the same BMM-derived attribute facts as a
// node declared by bare class name. Its OPT-declared bounds carry the actual
// parameter as their RM type, not DV_INTERVAL's formal "T" and not "", and
// the four boundary flags the RM mandates but the OPT leaves silent are
// injected as implicit, required attributes.
func TestCompile_GenericIntervalNodeAttributes(t *testing.T) {
	cases := []struct {
		fixture, declared, bound string
	}{
		{fixture: "Test_dv_interval_dv_quantity_lower_upper_constraint.v0", declared: "DV_INTERVAL<DV_QUANTITY>", bound: "DV_QUANTITY"},
		{fixture: "Test_dv_interval_dv_count_lower_upper_constraint.v0", declared: "DV_INTERVAL<DV_COUNT>", bound: "DV_COUNT"},
	}
	for _, tc := range cases {
		t.Run(tc.declared, func(t *testing.T) {
			nodes := mustCompile(t, tc.fixture).AllByRMType(tc.declared)
			if len(nodes) != 1 {
				t.Fatalf("AllByRMType(%q) = %d nodes, want 1", tc.declared, len(nodes))
			}
			n := nodes[0]

			for _, name := range []string{"lower", "upper"} {
				a := n.Attribute(name)
				if a == nil {
					t.Errorf("%s: OPT-declared attribute %q missing", tc.declared, name)
					continue
				}
				if a.Implicit() || a.Required() {
					t.Errorf("%s.%s: Implicit=%v Required=%v, want false, false (OPT-declared, optional in the RM)", tc.declared, name, a.Implicit(), a.Required())
				}
				if got := a.RMTypeName(); got != tc.bound {
					t.Errorf("%s.%s: RMTypeName = %q, want %q (the actual parameter)", tc.declared, name, got, tc.bound)
				}
			}

			for _, name := range []string{"lower_unbounded", "upper_unbounded", "lower_included", "upper_included"} {
				a := n.Attribute(name)
				if a == nil {
					t.Errorf("%s: implicit RM-mandatory attribute %q not injected", tc.declared, name)
					continue
				}
				if !a.Implicit() || !a.Required() {
					t.Errorf("%s.%s: Implicit=%v Required=%v, want true, true", tc.declared, name, a.Implicit(), a.Required())
				}
				if got := a.RMTypeName(); got != "Boolean" {
					t.Errorf("%s.%s: RMTypeName = %q, want %q", tc.declared, name, got, "Boolean")
				}
				if a.Cardinality() != template.Single {
					t.Errorf("%s.%s: Cardinality = %v, want Single", tc.declared, name, a.Cardinality())
				}
			}
		})
	}
}

// REQ-100 — the same holds for every BMM generic class and for the
// required flag of an OPT-declared attribute. POINT_EVENT<ITEM_TREE> declares
// data, which the RM mandates and types with EVENT's formal parameter: the
// compiled attribute is required and typed ITEM_TREE. The event's other
// RM-mandatory attributes are injected as implicit.
func TestCompile_GenericEventNodeAttributes(t *testing.T) {
	const body = `<?xml version="1.0" encoding="UTF-8"?>
<template xmlns="http://schemas.openehr.org/v1" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <template_id><value>generic_event</value></template_id>
  <concept>generic_event</concept>
  <definition xsi:type="C_COMPLEX_OBJECT">
    <rm_type_name>POINT_EVENT&lt;ITEM_TREE&gt;</rm_type_name>
    <node_id>at0002</node_id>
    <attributes xsi:type="C_SINGLE_ATTRIBUTE">
      <rm_attribute_name>data</rm_attribute_name>
      <children xsi:type="C_COMPLEX_OBJECT">
        <rm_type_name>ITEM_TREE</rm_type_name>
        <node_id>at0001</node_id>
      </children>
    </attributes>
  </definition>
</template>`
	opt, err := template.ParseOPT(strings.NewReader(body))
	if err != nil {
		t.Fatalf("ParseOPT: %v", err)
	}
	c, err := templatecompile.Compile(opt)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	root := c.Root()

	data := root.Attribute("data")
	switch {
	case data == nil:
		t.Fatal("OPT-declared attribute data missing")
	case data.Implicit() || !data.Required():
		t.Errorf("data: Implicit=%v Required=%v, want false, true (OPT-declared, mandatory in the RM)", data.Implicit(), data.Required())
	case data.RMTypeName() != "ITEM_TREE":
		t.Errorf("data: RMTypeName = %q, want %q (the actual parameter)", data.RMTypeName(), "ITEM_TREE")
	}

	for name, rm := range map[string]string{"name": "DV_TEXT", "archetype_node_id": "String", "time": "DV_DATE_TIME"} {
		a := root.Attribute(name)
		if a == nil {
			t.Errorf("implicit RM-mandatory attribute %q not injected", name)
			continue
		}
		if !a.Implicit() || !a.Required() || a.RMTypeName() != rm {
			t.Errorf("%s: Implicit=%v Required=%v RMTypeName=%q, want true, true, %q", name, a.Implicit(), a.Required(), a.RMTypeName(), rm)
		}
	}
}
