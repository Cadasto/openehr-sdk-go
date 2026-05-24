package template_test

import (
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/template/constraints"
)

// REQ-103 — buildPrimitive maps each closed-set xsi:type onto the
// matching constraint Go type during ParseOPT. Uses synthetic OPT
// XML so coverage stays tight regardless of which primitives appear
// in vendored fixtures.

const primitiveOPTTemplate = `<?xml version="1.0" encoding="UTF-8"?>
<template xmlns="http://schemas.openehr.org/v1" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <template_id><value>primitive_test</value></template_id>
  <concept>primitive_test</concept>
  <definition xsi:type="C_COMPLEX_OBJECT">
    <rm_type_name>COMPOSITION</rm_type_name>
    <node_id>at0000</node_id>
    <attributes xsi:type="C_MULTIPLE_ATTRIBUTE">
      <rm_attribute_name>content</rm_attribute_name>
      %s
    </attributes>
  </definition>
</template>`

func parseOPTWithChild(t *testing.T, childXML string) *template.OperationalTemplate {
	t.Helper()
	body := strings.Replace(primitiveOPTTemplate, "%s", childXML, 1)
	tmpl, err := template.ParseOPT(strings.NewReader(body))
	if err != nil {
		t.Fatalf("ParseOPT: %v", err)
	}
	return tmpl
}

func firstChildPrimitive(t *testing.T, tmpl *template.OperationalTemplate) constraints.PrimitiveConstraint {
	t.Helper()
	root, ok := tmpl.Root().(*template.ComplexObject)
	if !ok {
		t.Fatalf("Root not *ComplexObject: %T", tmpl.Root())
	}
	attrs := root.Attributes()
	if len(attrs) != 1 {
		t.Fatalf("want 1 attribute, got %d", len(attrs))
	}
	children := attrs[0].Children()
	if len(children) != 1 {
		t.Fatalf("want 1 child, got %d", len(children))
	}
	co, ok := children[0].(*template.ComplexObject)
	if !ok {
		t.Fatalf("child not *ComplexObject: %T", children[0])
	}
	return co.PrimitiveConstraint()
}

func TestParse_CBoolean(t *testing.T) {
	tmpl := parseOPTWithChild(t, `<children xsi:type="C_BOOLEAN">
		<rm_type_name>BOOLEAN</rm_type_name>
		<node_id />
		<true_valid>true</true_valid>
		<false_valid>false</false_valid>
		<assumed_value>true</assumed_value>
	</children>`)
	p := firstChildPrimitive(t, tmpl)
	c, ok := p.(constraints.CBoolean)
	if !ok {
		t.Fatalf("want CBoolean, got %T", p)
	}
	if !c.TrueValid || c.FalseValid {
		t.Errorf("flags = (%v, %v), want (true, false)", c.TrueValid, c.FalseValid)
	}
	if c.Default == nil || *c.Default != true {
		t.Errorf("Default = %v, want &true", c.Default)
	}
}

func TestParse_CInteger(t *testing.T) {
	tmpl := parseOPTWithChild(t, `<children xsi:type="C_INTEGER">
		<rm_type_name>INTEGER</rm_type_name>
		<node_id />
		<range>
			<lower_included>true</lower_included>
			<upper_included>true</upper_included>
			<lower_unbounded>false</lower_unbounded>
			<upper_unbounded>false</upper_unbounded>
			<lower>0</lower>
			<upper>100</upper>
		</range>
		<list>1</list>
		<list>5</list>
		<list>10</list>
	</children>`)
	p := firstChildPrimitive(t, tmpl)
	c, ok := p.(constraints.CInteger)
	if !ok {
		t.Fatalf("want CInteger, got %T", p)
	}
	if len(c.List) != 3 || c.List[2] != 10 {
		t.Errorf("List = %v, want [1 5 10]", c.List)
	}
	if !c.Range.IsBounded() || c.Range.Upper != 100 {
		t.Errorf("Range = %s, want [0..100]", c.Range)
	}
}

func TestParse_CString(t *testing.T) {
	tmpl := parseOPTWithChild(t, `<children xsi:type="C_STRING">
		<rm_type_name>STRING</rm_type_name>
		<node_id />
		<pattern>^[A-Z]+$</pattern>
		<list>YES</list>
		<list>NO</list>
	</children>`)
	p := firstChildPrimitive(t, tmpl)
	c, ok := p.(constraints.CString)
	if !ok {
		t.Fatalf("want CString, got %T", p)
	}
	if c.Pattern != `^[A-Z]+$` {
		t.Errorf("Pattern = %q, want ^[A-Z]+$", c.Pattern)
	}
	if len(c.List) != 2 || c.List[0] != "YES" {
		t.Errorf("List = %v, want [YES NO]", c.List)
	}
}

func TestParse_CDate(t *testing.T) {
	tmpl := parseOPTWithChild(t, `<children xsi:type="C_DATE">
		<rm_type_name>DATE</rm_type_name>
		<node_id />
		<pattern>yyyy-mm-dd</pattern>
	</children>`)
	p := firstChildPrimitive(t, tmpl)
	c, ok := p.(constraints.CDate)
	if !ok {
		t.Fatalf("want CDate, got %T", p)
	}
	if c.Pattern != "yyyy-mm-dd" {
		t.Errorf("Pattern = %q, want yyyy-mm-dd", c.Pattern)
	}
}

func TestParse_CCodePhrase(t *testing.T) {
	tmpl := parseOPTWithChild(t, `<children xsi:type="C_CODE_PHRASE">
		<rm_type_name>CODE_PHRASE</rm_type_name>
		<node_id />
		<terminology_id><value>openehr</value></terminology_id>
		<code_list>433</code_list>
		<code_list>434</code_list>
	</children>`)
	p := firstChildPrimitive(t, tmpl)
	c, ok := p.(constraints.CodePhrase)
	if !ok {
		t.Fatalf("want CodePhrase, got %T", p)
	}
	if c.Terminology != "openehr" {
		t.Errorf("Terminology = %q, want openehr", c.Terminology)
	}
	if len(c.CodeList) != 2 {
		t.Errorf("CodeList = %v, want [433 434]", c.CodeList)
	}
	if c.External() {
		t.Errorf("External() = true, want false (closed list)")
	}
}

func TestParse_CDvQuantity(t *testing.T) {
	tmpl := parseOPTWithChild(t, `<children xsi:type="C_DV_QUANTITY">
		<rm_type_name>DV_QUANTITY</rm_type_name>
		<node_id />
		<property>
			<terminology_id><value>openehr</value></terminology_id>
			<code_string>125</code_string>
		</property>
		<list>
			<magnitude>
				<lower_included>true</lower_included>
				<upper_included>true</upper_included>
				<lower_unbounded>false</lower_unbounded>
				<upper_unbounded>false</upper_unbounded>
				<lower>0</lower>
				<upper>300</upper>
			</magnitude>
			<precision>
				<lower_included>true</lower_included>
				<upper_included>true</upper_included>
				<lower_unbounded>false</lower_unbounded>
				<upper_unbounded>false</upper_unbounded>
				<lower>0</lower>
				<upper>0</upper>
			</precision>
			<units>mm[Hg]</units>
		</list>
	</children>`)
	p := firstChildPrimitive(t, tmpl)
	c, ok := p.(constraints.DvQuantity)
	if !ok {
		t.Fatalf("want DvQuantity, got %T", p)
	}
	if len(c.Units) != 1 || c.Units[0].Units != "mm[Hg]" {
		t.Errorf("Units = %+v, want one mm[Hg] entry", c.Units)
	}
	if c.Property == nil || c.Property.CodeString != "125" {
		t.Errorf("Property = %+v, want openehr::125", c.Property)
	}
	if !c.Units[0].Magnitude.IsBounded() || c.Units[0].Magnitude.Upper != 300 {
		t.Errorf("Magnitude range = %s, want [0..300]", c.Units[0].Magnitude)
	}
	// Validate against the parsed constraint — end-to-end smoke.
	if v := c.Validate(constraints.QuantityValue{Magnitude: 120, Units: "mm[Hg]"}); len(v) != 0 {
		t.Errorf("Validate(120) = %v, want nil", v)
	}
	if v := c.Validate(constraints.QuantityValue{Magnitude: 500, Units: "mm[Hg]"}); len(v) != 1 || v[0].Code != constraints.CodeOutOfRange {
		t.Errorf("Validate(500) = %v, want CodeOutOfRange", v)
	}
}

func TestParse_CDvOrdinal(t *testing.T) {
	tmpl := parseOPTWithChild(t, `<children xsi:type="C_DV_ORDINAL">
		<rm_type_name>DV_ORDINAL</rm_type_name>
		<node_id />
		<list>
			<value>0</value>
			<symbol>
				<terminology_id><value>local</value></terminology_id>
				<code_string>at0001</code_string>
			</symbol>
		</list>
		<list>
			<value>1</value>
			<symbol>
				<terminology_id><value>local</value></terminology_id>
				<code_string>at0002</code_string>
			</symbol>
		</list>
	</children>`)
	p := firstChildPrimitive(t, tmpl)
	c, ok := p.(constraints.CDvOrdinal)
	if !ok {
		t.Fatalf("want CDvOrdinal, got %T", p)
	}
	if len(c.Values) != 2 || c.Values[1].Value != 1 || c.Values[1].Symbol.CodeString != "at0002" {
		t.Errorf("Values = %+v, want two ordered entries", c.Values)
	}
}

// REQ-103 — non-primitive xsi:type values (here ARCHETYPE_SLOT) do
// NOT carry a primitive constraint. Guards against the dispatch
// over-attaching.
func TestParse_NonPrimitive_NoConstraint(t *testing.T) {
	tmpl := parseOPTWithChild(t, `<children xsi:type="ARCHETYPE_SLOT">
		<rm_type_name>CLUSTER</rm_type_name>
		<node_id>at0001</node_id>
	</children>`)
	root, _ := tmpl.Root().(*template.ComplexObject)
	attrs := root.Attributes()
	if len(attrs) != 1 || len(attrs[0].Children()) != 1 {
		t.Fatal("unexpected tree shape")
	}
	if _, ok := attrs[0].Children()[0].(*template.Slot); !ok {
		t.Errorf("child should be *Slot, got %T", attrs[0].Children()[0])
	}
}

// REQ-103 — a leaf COMPLEX_OBJECT (i.e. one of the non-primitive
// RM types like ELEMENT) returns nil from PrimitiveConstraint().
func TestParse_LeafComplexObject_NoConstraint(t *testing.T) {
	tmpl := parseOPTWithChild(t, `<children xsi:type="C_COMPLEX_OBJECT">
		<rm_type_name>ELEMENT</rm_type_name>
		<node_id>at0001</node_id>
	</children>`)
	p := firstChildPrimitive(t, tmpl)
	if p != nil {
		t.Errorf("PrimitiveConstraint() = %T, want nil for C_COMPLEX_OBJECT", p)
	}
}
