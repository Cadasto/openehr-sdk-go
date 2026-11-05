package templatecompile_test

// template_readings_test.go: readings of § REQ-111 Reading the model that
// hold from the OPT XML to the compiled tree, read by
// aom14.ReadOperationalTemplate and compiled by Compile, on templates written
// for each reading: an ELEMENT's null_flavor, the template-level node name,
// a blank xsi:type, the C_STRING list_open and the boolean flags, the
// quantity property units, and the term definitions the compiled tree hands
// out. The corpus goldens show none of these readings on such shapes.

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/aom/aom14"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/template/constraints"
	"github.com/cadasto/openehr-sdk-go/openehr/terminology"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// definitionOPT is an OPT whose definition is a node of RM type %[1]s,
// at0000, holding the attributes %[2]s.
const definitionOPT = `<?xml version="1.0" encoding="UTF-8"?>
<template xmlns="http://schemas.openehr.org/v1" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <template_id><value>template_readings</value></template_id>
  <concept>template_readings</concept>
  <definition>
    <rm_type_name>%[1]s</rm_type_name>
    <node_id>at0000</node_id>
    %[2]s
  </definition>
</template>`

// codedAttribute is a C_SINGLE_ATTRIBUTE named name over a DV_CODED_TEXT
// whose defining_code lists the one openehr code.
func codedAttribute(name, code string) string {
	return fmt.Sprintf(`<attributes xsi:type="C_SINGLE_ATTRIBUTE">
      <rm_attribute_name>%s</rm_attribute_name>
      <children xsi:type="C_COMPLEX_OBJECT">
        <rm_type_name>DV_CODED_TEXT</rm_type_name>
        <node_id></node_id>
        <attributes xsi:type="C_SINGLE_ATTRIBUTE">
          <rm_attribute_name>defining_code</rm_attribute_name>
          <children xsi:type="C_CODE_PHRASE">
            <rm_type_name>CODE_PHRASE</rm_type_name>
            <node_id></node_id>
            <terminology_id><value>openehr</value></terminology_id>
            <code_list>%s</code_list>
          </children>
        </attributes>
      </children>
    </attributes>`, name, code)
}

// declaredAttributes returns the names of the attributes n declares, in
// document order, with those the compile step injects left out.
func declaredAttributes(n *templatecompile.CompiledNode) []string {
	var names []string
	for _, a := range n.Attributes() {
		if !a.Implicit() {
			names = append(names, a.Name())
		}
	}
	return names
}

// codeListAt returns the code list of the C_CODE_PHRASE at path in c.
func codeListAt(t *testing.T, c *templatecompile.Compiled, path string) []string {
	t.Helper()
	n, err := c.NodeAt(path)
	if err != nil {
		t.Fatalf("NodeAt(%s) = %v, want the CODE_PHRASE leaf", path, err)
	}
	cp, ok := n.PrimitiveConstraint().(constraints.CodePhrase)
	if !ok {
		t.Fatalf("NodeAt(%s).PrimitiveConstraint() = %#v, want a CodePhrase", path, n.PrimitiveConstraint())
	}
	return cp.CodeList
}

// TestREQ111_CompileReadsNullFlavorOnAnElementOnly pins the null_flavor
// reading of § REQ-111 Reading the model, in both of the reader's modes: an
// ELEMENT's null_flavor is read as null_flavour, so its path and its
// constraint are the RM attribute's, and that one spelling on that one class
// is the whole reading: the same name on another class, and another name on
// an ELEMENT, stay as written. An ELEMENT that declares both names keeps
// null_flavor as written, in either order, each attribute with its own
// constraint, and null_flavor marked as an attribute the RM does not have.
func TestREQ111_CompileReadsNullFlavorOnAnElementOnly(t *testing.T) {
	one := []struct {
		name, rmType, attr, want string
	}{
		{name: "an ELEMENT's null_flavor reads as null_flavour", rmType: "ELEMENT", attr: "null_flavor", want: "null_flavour"},
		{name: "an ELEMENT's null_flavour stays", rmType: "ELEMENT", attr: "null_flavour", want: "null_flavour"},
		{name: "null_flavor on another class stays", rmType: "CLUSTER", attr: "null_flavor", want: "null_flavor"},
		{name: "another spelling on an ELEMENT stays", rmType: "ELEMENT", attr: "null_flavours", want: "null_flavours"},
	}
	for _, mode := range readerModes {
		for _, tc := range one {
			t.Run(mode.name+"/"+tc.name, func(t *testing.T) {
				c, err := readAndCompile(fmt.Sprintf(definitionOPT, tc.rmType, codedAttribute(tc.attr, "271")), mode.opts...)
				if err != nil {
					t.Fatalf("read and compile %s.%s = %v, want the template", tc.rmType, tc.attr, err)
				}
				if got := declaredAttributes(c.Root()); !slices.Equal(got, []string{tc.want}) {
					t.Fatalf("%s.%s: declared attributes = %q, want %q", tc.rmType, tc.attr, got, tc.want)
				}
				if got := codeListAt(t, c, "/"+tc.want+"/defining_code"); !slices.Equal(got, []string{"271"}) {
					t.Errorf("%s.%s: code list at /%s/defining_code = %q, want [271]", tc.rmType, tc.attr, tc.want, got)
				}
			})
		}
	}

	both := []struct {
		name  string
		attrs string
		want  []string
	}{
		{name: "null_flavor first", attrs: codedAttribute("null_flavor", "271") + codedAttribute("null_flavour", "253"), want: []string{"null_flavor", "null_flavour"}},
		{name: "null_flavour first", attrs: codedAttribute("null_flavour", "253") + codedAttribute("null_flavor", "271"), want: []string{"null_flavour", "null_flavor"}},
	}
	for _, mode := range readerModes {
		for _, tc := range both {
			t.Run(mode.name+"/both declared, "+tc.name, func(t *testing.T) {
				c, err := readAndCompile(fmt.Sprintf(definitionOPT, "ELEMENT", tc.attrs), mode.opts...)
				if err != nil {
					t.Fatalf("read and compile = %v, want the template", err)
				}
				if got := declaredAttributes(c.Root()); !slices.Equal(got, tc.want) {
					t.Fatalf("declared attributes = %q, want %q", got, tc.want)
				}
				for attr, code := range map[string]string{"null_flavor": "271", "null_flavour": "253"} {
					if got := codeListAt(t, c, "/"+attr+"/defining_code"); !slices.Equal(got, []string{code}) {
						t.Errorf("code list at /%s/defining_code = %q, want [%s]", attr, got, code)
					}
				}
				if !c.Root().Attribute("null_flavor").NotInRM() || c.Root().Attribute("null_flavour").NotInRM() {
					t.Errorf("NotInRM: null_flavor %t, null_flavour %t; want null_flavor alone marked",
						c.Root().Attribute("null_flavor").NotInRM(), c.Root().Attribute("null_flavour").NotInRM())
				}
			})
		}
	}
}

// nameValue is a name attribute over one child of RM type rmType whose value
// is the C_PRIMITIVE_OBJECT holding the C_STRING item body.
func nameValue(rmType, body string) string {
	return fmt.Sprintf(`<children xsi:type="C_COMPLEX_OBJECT">
        <rm_type_name>%s</rm_type_name>
        <node_id/>
        <attributes xsi:type="C_SINGLE_ATTRIBUTE">
          <rm_attribute_name>value</rm_attribute_name>
          <children xsi:type="C_PRIMITIVE_OBJECT">
            <rm_type_name>STRING</rm_type_name>
            <node_id/>
            <item xsi:type="C_STRING">%s</item>
          </children>
        </attributes>
      </children>`, rmType, body)
}

// nameAttribute is the name attribute over the children given.
func nameAttribute(children ...string) string {
	return `<attributes xsi:type="C_SINGLE_ATTRIBUTE">
      <rm_attribute_name>name</rm_attribute_name>
      ` + strings.Join(children, "\n      ") + `
    </attributes>`
}

// TestREQ111_CompileReadsTheTemplateLevelNodeName pins the node name the
// compile step carries (§ REQ-111, the reading § REQ-116 gives): the one
// non-blank entry, trimmed, of a closed C_STRING list on the name's value,
// judged on the list as the OPT writes it, the first alternative that pins
// one winning. A choice, a blank entry, an open list, a pattern and an absent
// name pin none, and the template concept is never substituted. A <children>
// typed C_STRING is a shape the reader's default mode alone reads.
func TestREQ111_CompileReadsTheTemplateLevelNodeName(t *testing.T) {
	cases := []struct {
		name, attrs, want string
		defaultOnly       bool
	}{
		{name: "a one-entry list", attrs: nameAttribute(nameValue("DV_TEXT", "<list>Husten</list>")), want: "Husten"},
		{name: "a one-entry list on a DV_CODED_TEXT", attrs: nameAttribute(nameValue("DV_CODED_TEXT", "<list>Symptome</list>")), want: "Symptome"},
		{name: "a one-entry list padded", attrs: nameAttribute(nameValue("DV_TEXT", "<list>  Husten \n</list>")), want: "Husten"},
		{name: "a C_STRING child in place of the C_PRIMITIVE_OBJECT", attrs: nameAttribute(`<children xsi:type="C_COMPLEX_OBJECT">
        <rm_type_name>DV_TEXT</rm_type_name>
        <node_id/>
        <attributes xsi:type="C_SINGLE_ATTRIBUTE">
          <rm_attribute_name>value</rm_attribute_name>
          <children xsi:type="C_STRING">
            <rm_type_name>STRING</rm_type_name>
            <node_id/>
            <list>Kontakt</list>
          </children>
        </attributes>
      </children>`), want: "Kontakt", defaultOnly: true},
		{name: "a two-entry list is a choice", attrs: nameAttribute(nameValue("DV_TEXT", "<list>A</list><list>B</list>")), want: ""},
		{name: "a two-entry list with a blank entry is a choice", attrs: nameAttribute(nameValue("DV_TEXT", "<list>A</list><list> </list>")), want: ""},
		{name: "the first alternative that pins a name wins", attrs: nameAttribute(nameValue("DV_TEXT", "<list>First</list>"), nameValue("DV_CODED_TEXT", "<list>Second</list>")), want: "First"},
		{name: "an alternative that pins nothing falls through", attrs: nameAttribute(`<children xsi:type="C_COMPLEX_OBJECT">
        <rm_type_name>DV_CODED_TEXT</rm_type_name>
        <node_id/>
      </children>`, nameValue("DV_TEXT", "<list>Second</list>")), want: "Second"},
		{name: "an alternative that pins a blank falls through", attrs: nameAttribute(nameValue("DV_TEXT", "<list>  </list>"), nameValue("DV_CODED_TEXT", "<list>Second</list>")), want: "Second"},
		{name: "an open one-entry list", attrs: nameAttribute(nameValue("DV_TEXT", "<list>Husten</list><list_open>true</list_open>")), want: ""},
		{name: "a one-entry list written closed", attrs: nameAttribute(nameValue("DV_TEXT", "<list>Husten</list><list_open>false</list_open>")), want: "Husten"},
		{name: "a one-entry list with an empty list_open", attrs: nameAttribute(nameValue("DV_TEXT", "<list>Husten</list><list_open/>")), want: "Husten"},
		{name: "a lone blank entry", attrs: nameAttribute(nameValue("DV_TEXT", "<list>   </list>")), want: ""},
		{name: "a pattern", attrs: nameAttribute(nameValue("DV_TEXT", "<pattern>.*</pattern>")), want: ""},
		{name: "no name attribute", attrs: "", want: ""},
	}
	for _, mode := range readerModes {
		for _, tc := range cases {
			if tc.defaultOnly && mode.name != "default" {
				continue
			}
			t.Run(mode.name+"/"+tc.name, func(t *testing.T) {
				c, err := readAndCompile(fmt.Sprintf(definitionOPT, "COMPOSITION", tc.attrs), mode.opts...)
				if err != nil {
					t.Fatalf("read and compile = %v, want the template", err)
				}
				if got := c.Root().NodeName(); got != tc.want {
					t.Errorf("Root().NodeName() = %q, want %q", got, tc.want)
				}
			})
		}
	}
}

// blankTypeOPT is a COMPOSITION whose category, typed %[1]s, holds a
// DV_CODED_TEXT whose defining_code holds a CODE_PHRASE typed %[2]s.
const blankTypeOPT = `<?xml version="1.0" encoding="UTF-8"?>
<template xmlns="http://schemas.openehr.org/v1" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <template_id><value>blank_type</value></template_id>
  <concept>blank_type</concept>
  <definition>
    <rm_type_name>COMPOSITION</rm_type_name>
    <node_id>at0000</node_id>
    <attributes xsi:type="%[1]s">
      <rm_attribute_name>category</rm_attribute_name>
      <children xsi:type="C_COMPLEX_OBJECT">
        <rm_type_name>DV_CODED_TEXT</rm_type_name>
        <node_id></node_id>
        <attributes xsi:type="C_SINGLE_ATTRIBUTE">
          <rm_attribute_name>defining_code</rm_attribute_name>
          <children xsi:type="%[2]s">
            <rm_type_name>CODE_PHRASE</rm_type_name>
            <node_id></node_id>
            <terminology_id><value>openehr</value></terminology_id>
            <code_list>433</code_list>
          </children>
        </attributes>
      </children>
    </attributes>
  </definition>
</template>`

// TestREQ111_ReaderReadsABlankXSITypeAsNone pins § REQ-111 Reading the model
// on an xsi:type that holds only white space: the default mode reads it
// without the white space, as no type, so an <attributes> so typed reads as a
// C_SINGLE_ATTRIBUTE and a <children> as a C_COMPLEX_OBJECT of its RM type,
// and strict mode refuses it.
func TestREQ111_ReaderReadsABlankXSITypeAsNone(t *testing.T) {
	const blank = "\n \t "
	cases := []struct {
		name, attrType, childType string
	}{
		{name: "an <attributes>", attrType: blank, childType: "C_CODE_PHRASE"},
		{name: "a <children>", attrType: "C_SINGLE_ATTRIBUTE", childType: blank},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := fmt.Sprintf(blankTypeOPT, tc.attrType, tc.childType)
			c, err := readAndCompile(body)
			if err != nil {
				t.Fatalf("read and compile, default mode = %v, want the template", err)
			}
			category := c.Root().Attribute("category")
			if category == nil || category.Implicit() || category.Cardinality() != template.Single {
				t.Fatalf("category = %+v, want the declared single attribute", category)
			}
			n, err := c.NodeAt("/category/defining_code")
			if err != nil {
				t.Fatalf("NodeAt(/category/defining_code) = %v, want the CODE_PHRASE node", err)
			}
			if n.RMTypeName() != "CODE_PHRASE" {
				t.Errorf("NodeAt(/category/defining_code).RMTypeName() = %q, want CODE_PHRASE", n.RMTypeName())
			}

			_, err = aom14.ReadOperationalTemplate(strings.NewReader(body), canxml.WithStrict())
			if !errors.Is(err, typereg.ErrMissingType) && !errors.Is(err, typereg.ErrUnknownType) {
				t.Errorf("ReadOperationalTemplate(WithStrict) = %v, want an error wrapping typereg.ErrMissingType or typereg.ErrUnknownType", err)
			}
		})
	}
}

// stringChildren are the two shapes an OPT writes a C_STRING in: the item of
// a C_PRIMITIVE_OBJECT, and a <children> of that type, which the reader's
// default mode alone reads. %s is the C_STRING's content.
var stringChildren = []struct {
	name, child string
	defaultOnly bool
}{
	{name: "C_STRING", child: `<children xsi:type="C_STRING"><rm_type_name>STRING</rm_type_name><node_id/>%s</children>`, defaultOnly: true},
	{name: "C_PRIMITIVE_OBJECT", child: `<children xsi:type="C_PRIMITIVE_OBJECT"><rm_type_name>STRING</rm_type_name><node_id/><item xsi:type="C_STRING">%s</item></children>`},
}

// TestREQ111_CompileReadsListOpenAndRefusesAFlagThatIsNoBoolean pins, from
// the XML to the compiled tree in both of the reader's modes, the reading of
// C_STRING.list_open § REQ-103 gives and § REQ-111 Reading the model keeps:
// an absent or empty element leaves the list closed, and a written one is
// read as written. Text that is no boolean in list_open, or in a C_BOOLEAN's
// true_valid or false_valid, is refused with an error wrapping
// canxml.ErrInvalidShape.
func TestREQ111_CompileReadsListOpenAndRefusesAFlagThatIsNoBoolean(t *testing.T) {
	cases := []struct {
		name, listOpen string
		want           bool
	}{
		{name: "absent", listOpen: "", want: false},
		{name: "self-closing", listOpen: "<list_open/>", want: false},
		{name: "empty", listOpen: "<list_open></list_open>", want: false},
		{name: "false", listOpen: "<list_open>false</list_open>", want: false},
		{name: "padded false", listOpen: "<list_open> false\n</list_open>", want: false},
		{name: "true", listOpen: "<list_open>true</list_open>", want: true},
	}
	for _, mode := range readerModes {
		for _, sc := range stringChildren {
			if sc.defaultOnly && mode.name != "default" {
				continue
			}
			for _, tc := range cases {
				t.Run(mode.name+"/"+sc.name+"/"+tc.name, func(t *testing.T) {
					child := fmt.Sprintf(sc.child, "<list>a</list>"+tc.listOpen)
					c, err := readAndCompile(childBody(child, ""), mode.opts...)
					if err != nil {
						t.Fatalf("read and compile = %v, want the template", err)
					}
					got, ok := c.Root().Attribute("content").Children()[0].PrimitiveConstraint().(constraints.CString)
					if !ok || got.ListOpen != tc.want || !slices.Equal(got.List, []string{"a"}) {
						t.Errorf("PrimitiveConstraint() = %#v, want a CString listing [a] with ListOpen %t", got, tc.want)
					}
				})
			}
		}
	}

	refused := map[string]string{
		"list_open":   fmt.Sprintf(stringChildren[1].child, "<list>a</list><list_open>maybe</list_open>"),
		"true_valid":  `<children xsi:type="C_PRIMITIVE_OBJECT"><rm_type_name>BOOLEAN</rm_type_name><node_id/><item xsi:type="C_BOOLEAN"><true_valid>yes</true_valid><false_valid>true</false_valid></item></children>`,
		"false_valid": `<children xsi:type="C_PRIMITIVE_OBJECT"><rm_type_name>BOOLEAN</rm_type_name><node_id/><item xsi:type="C_BOOLEAN"><true_valid>true</true_valid><false_valid>no</false_valid></item></children>`,
	}
	for _, mode := range readerModes {
		for flag, child := range refused {
			t.Run(mode.name+"/a "+flag+" that is no boolean", func(t *testing.T) {
				_, err := readAndCompile(childBody(child, ""), mode.opts...)
				if !errors.Is(err, canxml.ErrInvalidShape) {
					t.Errorf("read and compile = %v, want an error wrapping canxml.ErrInvalidShape", err)
				}
			})
		}
	}
}

// quantityChild is a C_DV_QUANTITY child whose property is terminology::code
// and that lists the units given.
func quantityChild(terminologyID, code string, units ...string) string {
	var list strings.Builder
	for _, u := range units {
		list.WriteString("<list><units>" + u + "</units></list>")
	}
	return fmt.Sprintf(`<children xsi:type="C_DV_QUANTITY"><rm_type_name>DV_QUANTITY</rm_type_name><node_id/>`+
		`<property><terminology_id><value>%s</value></terminology_id><code_string>%s</code_string></property>%s</children>`,
		terminologyID, code, list.String())
}

// unitsOfProperty returns the units the pinned property-unit table lists for
// the openehr property code, its primary units first and each part in the
// table's order.
func unitsOfProperty(t *testing.T, code string) []string {
	t.Helper()
	p, ok := terminology.QuantityPropertyByCode(code)
	if !ok {
		t.Fatalf("the pinned property-unit table does not list openehr::%s", code)
	}
	var primary, other []string
	for u := range p.All() {
		if u.Primary {
			primary = append(primary, u.UCUM)
		} else {
			other = append(other, u.UCUM)
		}
	}
	return slices.Concat(primary, other)
}

// TestREQ111_CompileFillsPropertyUnitsForAListedOpenEHRCodeOnly pins, from
// the XML to the compiled tree, the property units § REQ-103 has the compile
// step fill: for an openehr code the pinned table lists, its units with the
// primary ones first, whether or not the constraint lists units, and none for
// another terminology, matched exactly, or a code the table does not list.
func TestREQ111_CompileFillsPropertyUnitsForAListedOpenEHRCodeOnly(t *testing.T) {
	quantity := func(t *testing.T, child string) constraints.DvQuantity {
		t.Helper()
		c, err := readAndCompile(childBody(child, ""))
		if err != nil {
			t.Fatalf("read and compile = %v, want the template", err)
		}
		q, ok := c.Root().Attribute("content").Children()[0].PrimitiveConstraint().(constraints.DvQuantity)
		if !ok {
			t.Fatalf("PrimitiveConstraint() = %T, want a DvQuantity", c.Root().Attribute("content").Children()[0].PrimitiveConstraint())
		}
		return q
	}

	// The pinned table lists Length's primary unit, m, seventh.
	length := quantity(t, quantityChild("openehr", "122"))
	if len(length.PropertyUnits) < 3 || !slices.Equal(length.PropertyUnits[:3], []string{"m", "[in_i]/1000", "cm"}) {
		t.Errorf("openehr::122 PropertyUnits = %q, want m, then [in_i]/1000, cm, … in the table's order", length.PropertyUnits)
	}
	if want := unitsOfProperty(t, "122"); !slices.Equal(length.PropertyUnits, want) {
		t.Errorf("openehr::122 PropertyUnits = %q, want %q", length.PropertyUnits, want)
	}
	if listed := quantity(t, quantityChild("openehr", "125", "cm", "mg")); !slices.Equal(listed.PropertyUnits, unitsOfProperty(t, "125")) {
		t.Errorf("openehr::125 with a units list: PropertyUnits = %q, want %q", listed.PropertyUnits, unitsOfProperty(t, "125"))
	}

	for _, tc := range []struct{ terminology, code string }{
		{"local", "125"},
		{"SNOMED-CT", "122"},
		{"OPENEHR", "122"},
		{"openEHR", "122"},
		{"openehr", "433"},
	} {
		if q := quantity(t, quantityChild(tc.terminology, tc.code)); len(q.PropertyUnits) != 0 {
			t.Errorf("%s::%s: PropertyUnits = %q, want none", tc.terminology, tc.code, q.PropertyUnits)
		}
	}
}

// TestREQ111_CompiledTermIsACopy pins that a term the compiled tree hands out
// holds items of its own: changing them changes no later answer.
func TestREQ111_CompiledTermIsACopy(t *testing.T) {
	c, err := templatecompile.Compile(mustReadModelFile(t, fixtures.TemplateOptForName("vital_signs")))
	if err != nil {
		t.Fatalf("Compile(vital_signs): %v", err)
	}
	want, ok := c.Root().Term("at0000", "")
	if !ok || want.Items["text"] == "" {
		t.Fatalf(`Root().Term("at0000") = %v, %t; want a term with a text`, want, ok)
	}
	want.Items = maps.Clone(want.Items)

	got, _ := c.Root().Term("at0000", "")
	got.Items["text"] = "changed"
	byTemplate, _ := c.TermLang("at0000", "")
	byTemplate.Items["text"] = "changed"

	if again, _ := c.Root().Term("at0000", ""); !maps.Equal(again.Items, want.Items) {
		t.Errorf(`Root().Term("at0000") after a change to an earlier answer = %v, want %v`, again.Items, want.Items)
	}
	if again, _ := c.TermLang("at0000", ""); !maps.Equal(again.Items, want.Items) {
		t.Errorf(`TermLang("at0000") after a change to an earlier answer = %v, want %v`, again.Items, want.Items)
	}
}
