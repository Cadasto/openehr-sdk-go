package validation_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// compileStrictOPT parses body in strict mode and compiles it.
func compileStrictOPT(t *testing.T, body string) *templatecompile.Compiled {
	t.Helper()
	opt, err := template.ParseOPTStrict(strings.NewReader(body))
	if err != nil {
		t.Fatalf("ParseOPTStrict: %v", err)
	}
	c, err := templatecompile.Compile(opt)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	return c
}

// nullFlavorElementOPT is an OPT rooted at an ELEMENT that constrains the
// attribute %s to a DV_CODED_TEXT whose code is openehr 271.
const nullFlavorElementOPT = `<?xml version="1.0" encoding="UTF-8"?>
<template xmlns="http://schemas.openehr.org/v1" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <template_id><value>null_flavor_element</value></template_id>
  <concept>null_flavor_element</concept>
  <definition xsi:type="C_COMPLEX_OBJECT">
    <rm_type_name>ELEMENT</rm_type_name>
    <node_id>at0001</node_id>
    <attributes xsi:type="C_SINGLE_ATTRIBUTE">
      <rm_attribute_name>%s</rm_attribute_name>
      <existence><lower>0</lower><upper>1</upper></existence>
      <children xsi:type="C_COMPLEX_OBJECT">
        <rm_type_name>DV_CODED_TEXT</rm_type_name>
        <node_id></node_id>
        <attributes xsi:type="C_SINGLE_ATTRIBUTE">
          <rm_attribute_name>defining_code</rm_attribute_name>
          <existence><lower>1</lower><upper>1</upper></existence>
          <children xsi:type="C_CODE_PHRASE">
            <rm_type_name>CODE_PHRASE</rm_type_name>
            <node_id></node_id>
            <terminology_id><value>openehr</value></terminology_id>
            <code_list>271</code_list>
          </children>
        </attributes>
      </children>
    </attributes>
  </definition>
</template>`

// TestREQ100_NullFlavorConstraintApplies pins § REQ-100 Node taxonomy end to
// end: an OPT that spells an ELEMENT's null_flavour null_flavor compiles to an
// attribute named null_flavour, of the RM type the BMM gives it, and the
// validator applies its code list to the ELEMENT's null_flavour, exactly as
// for the RM spelling.
func TestREQ100_NullFlavorConstraintApplies(t *testing.T) {
	element := func(code string) *rm.Element {
		return &rm.Element{
			ArchetypeNodeID: "at0001",
			Name:            rm.DVText{Value: "item"},
			NullFlavour: &rm.DVCodedText{
				Value:        "no information",
				DefiningCode: rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "openehr"}, CodeString: code},
			},
		}
	}
	for _, spelling := range []string{"null_flavor", "null_flavour"} {
		t.Run(spelling, func(t *testing.T) {
			c := compileStrictOPT(t, fmt.Sprintf(nullFlavorElementOPT, spelling))
			attr := c.Root().Attribute("null_flavour")
			if attr == nil {
				var names []string
				for _, a := range c.Root().Attributes() {
					names = append(names, a.Name())
				}
				t.Fatalf("Compile(ELEMENT.%s): no attribute null_flavour; attributes %q", spelling, names)
			}
			if got := attr.RMTypeName(); got != "DV_CODED_TEXT" {
				t.Errorf("Compile(ELEMENT.%s): null_flavour RMTypeName() = %q, want DV_CODED_TEXT", spelling, got)
			}

			if r := validation.Validate(element("271"), c); len(r.Issues) != 0 {
				t.Errorf("Validate(null_flavour 271, listed) issues = %+v, want none", issueKeys(r.Issues))
			}
			// 253 is a valid null flavour the OPT does not list.
			r := validation.Validate(element("253"), c)
			want := []string{"primitive_not_in_list /null_flavour/defining_code"}
			if got := issueKeys(r.Issues); !slices.Equal(got, want) {
				t.Errorf("Validate(null_flavour 253, not listed) issues = %+v, want %+v", got, want)
			}
		})
	}
}

// ismNameOPT is the minimal_action shape: a COMPOSITION whose content holds
// an ACTION whose ISM_TRANSITION the OPT constrains with a name, at
// existence 1..1, which ISM_TRANSITION, a PATHABLE and not a LOCATABLE, does
// not have.
const ismNameOPT = `<?xml version="1.0" encoding="UTF-8"?>
<template xmlns="http://schemas.openehr.org/v1" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <template_id><value>ism_transition_name</value></template_id>
  <concept>ism_transition_name</concept>
  <definition>
    <rm_type_name>COMPOSITION</rm_type_name>
    <node_id>at0000</node_id>
    <attributes xsi:type="C_MULTIPLE_ATTRIBUTE">
      <rm_attribute_name>content</rm_attribute_name>
      <existence><lower>0</lower><upper>1</upper></existence>
      <children xsi:type="C_ARCHETYPE_ROOT">
        <rm_type_name>ACTION</rm_type_name>
        <occurrences><lower>0</lower><upper_unbounded>true</upper_unbounded></occurrences>
        <node_id>at0000</node_id>
        <attributes xsi:type="C_SINGLE_ATTRIBUTE">
          <rm_attribute_name>ism_transition</rm_attribute_name>
          <existence><lower>1</lower><upper>1</upper></existence>
          <children xsi:type="C_COMPLEX_OBJECT">
            <rm_type_name>ISM_TRANSITION</rm_type_name>
            <node_id></node_id>
            <attributes xsi:type="C_SINGLE_ATTRIBUTE">
              <rm_attribute_name>name</rm_attribute_name>
              <existence><lower>1</lower><upper>1</upper></existence>
              <children xsi:type="C_COMPLEX_OBJECT">
                <rm_type_name>DV_TEXT</rm_type_name>
                <node_id></node_id>
              </children>
            </attributes>
          </children>
        </attributes>
        <archetype_id><value>openEHR-EHR-ACTION.medication.v1</value></archetype_id>
      </children>
      <cardinality><interval><lower>0</lower><upper_unbounded>true</upper_unbounded></interval></cardinality>
    </attributes>
    <archetype_id><value>openEHR-EHR-COMPOSITION.encounter.v1</value></archetype_id>
  </definition>
</template>`

// ismNameComposition returns a COMPOSITION that holds two ACTIONs, each with
// an ISM_TRANSITION, which carries no name since no ISM_TRANSITION can.
func ismNameComposition() *rm.Composition {
	comp := codedComposition()
	comp.Context = codedContext()
	comp.Content = []rm.ContentItem{ismAction(nil), ismAction(nil)}
	return comp
}

// TestREQ171_AttributeNotInRMIsLeftOut pins § REQ-171 One walk and
// Coverage: an attribute the OPT declares that its node's RM class does not
// have, here ISM_TRANSITION.name at existence 1..1, is left out of the walk,
// so an instance, which cannot carry it, draws no issue for it. Coverage
// lists its path in walk order, once per node however many declarations name
// it, and Complete does not read the list.
func TestREQ171_AttributeNotInRMIsLeftOut(t *testing.T) {
	// A second declaration of the name, with no child, so the compile admits
	// it beside the first.
	const firstName = `<rm_type_name>DV_TEXT</rm_type_name>
                <node_id></node_id>
              </children>
            </attributes>`
	const nameAgain = `
            <attributes xsi:type="C_SINGLE_ATTRIBUTE">
              <rm_attribute_name>name</rm_attribute_name>
              <existence><lower>1</lower><upper>1</upper></existence>
            </attributes>`
	if strings.Count(ismNameOPT, firstName) != 1 {
		t.Fatal("the first name declaration is not found exactly once")
	}
	twice := strings.Replace(ismNameOPT, firstName, firstName+nameAgain, 1)
	tests := []struct {
		name string
		body string
	}{
		{name: "declared once", body: ismNameOPT},
		{name: "declared twice", body: twice},
	}
	want := []string{"/content[1]/ism_transition/name", "/content[2]/ism_transition/name"}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := compileStrictOPT(t, tc.body)
			r := validation.Validate(ismNameComposition(), c)
			if len(r.Issues) != 0 || !r.OK {
				t.Errorf("Validate(ACTIONs whose ISM_TRANSITION has no name) = OK %t, issues %q, want OK with none", r.OK, issueKeys(r.Issues))
			}
			if !slices.Equal(r.Coverage.NotInRM, want) {
				t.Errorf("Coverage.NotInRM = %q, want %q", r.Coverage.NotInRM, want)
			}
			if !r.Coverage.Complete() {
				t.Errorf("Coverage.Complete() = false (Stopped %q, External %q), want true: NotInRM is not part of it", r.Coverage.Stopped, r.Coverage.External)
			}
		})
	}
}

// itemStructureItemsOPT is an OPT rooted at an ITEM_STRUCTURE that declares
// items, which ITEM_STRUCTURE does not have and its descendants ITEM_TREE
// and ITEM_LIST do, asking for at least one member.
const itemStructureItemsOPT = `<?xml version="1.0" encoding="UTF-8"?>
<template xmlns="http://schemas.openehr.org/v1" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <template_id><value>item_structure_items</value></template_id>
  <concept>item_structure_items</concept>
  <definition xsi:type="C_COMPLEX_OBJECT">
    <rm_type_name>ITEM_STRUCTURE</rm_type_name>
    <node_id>at0001</node_id>
    <attributes xsi:type="C_MULTIPLE_ATTRIBUTE">
      <rm_attribute_name>items</rm_attribute_name>
      <existence><lower>1</lower><upper>1</upper></existence>
      <cardinality><interval><lower>1</lower><upper_unbounded>true</upper_unbounded></interval></cardinality>
    </attributes>
  </definition>
</template>`

// TestREQ171_AttributeTheValueClassHasIsVisited pins the limit of § REQ-171
// One walk's exception: an attribute an instance can carry is visited. Here
// the OPT declares items on an ITEM_STRUCTURE, which has no property of that
// name while its descendants ITEM_TREE and ITEM_LIST do, so the compile does
// not mark it (§ REQ-100 Node taxonomy); the value is an ITEM_TREE, so the
// walk visits items and reports a missing member.
func TestREQ171_AttributeTheValueClassHasIsVisited(t *testing.T) {
	c := compileStrictOPT(t, itemStructureItemsOPT)
	if a := c.Root().Attribute("items"); a == nil || a.NotInRM() {
		t.Fatalf("Compile(ITEM_STRUCTURE.items): attribute %+v, want one not marked NotInRM: its descendants have items", a)
	}
	empty := &rm.ItemTree{ArchetypeNodeID: "at0001", Name: rm.DVText{Value: "tree"}}
	r := validation.Validate(empty, c)
	if got, want := issueKeys(r.Issues), []string{"required /items"}; !slices.Equal(got, want) {
		t.Errorf("Validate(ITEM_TREE with no items) issues = %q, want %q", got, want)
	}
	if len(r.Coverage.NotInRM) != 0 {
		t.Errorf("Coverage.NotInRM = %q, want none: the walk visited items", r.Coverage.NotInRM)
	}
	full := &rm.ItemTree{
		ArchetypeNodeID: "at0001",
		Name:            rm.DVText{Value: "tree"},
		Items:           []rm.Item{&rm.Element{ArchetypeNodeID: "at0002", Name: rm.DVText{Value: "item"}, Value: &rm.DVText{Value: "text"}}},
	}
	if r := validation.Validate(full, c); len(r.Issues) != 0 {
		t.Errorf("Validate(ITEM_TREE with one item) issues = %q, want none", issueKeys(r.Issues))
	}
}

// elementIsNullOPT is an OPT rooted at an ELEMENT that constrains is_null, a
// function ELEMENT declares in the BMM, at existence 1..1.
const elementIsNullOPT = `<?xml version="1.0" encoding="UTF-8"?>
<template xmlns="http://schemas.openehr.org/v1" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <template_id><value>element_is_null</value></template_id>
  <concept>element_is_null</concept>
  <definition xsi:type="C_COMPLEX_OBJECT">
    <rm_type_name>ELEMENT</rm_type_name>
    <node_id>at0001</node_id>
    <attributes xsi:type="C_SINGLE_ATTRIBUTE">
      <rm_attribute_name>is_null</rm_attribute_name>
      <existence><lower>1</lower><upper>1</upper></existence>
    </attributes>
  </definition>
</template>`

// TestREQ171_FunctionConstraintIsLeftOut pins § REQ-171 One walk with §
// REQ-112 Driver's set: a template that constrains a BMM function, here
// ELEMENT.is_null at existence 1..1, constrains nothing an instance holds.
// The walk leaves it out as an attribute no serialization carries, so it
// draws no issue, and lists it nowhere in Coverage, since the compile does
// not mark a function (§ REQ-100 Node taxonomy).
func TestREQ171_FunctionConstraintIsLeftOut(t *testing.T) {
	c := compileStrictOPT(t, elementIsNullOPT)
	if a := c.Root().Attribute("is_null"); a == nil || a.NotInRM() {
		t.Fatalf("Compile(ELEMENT.is_null): attribute %+v, want one not marked NotInRM: is_null is a function of ELEMENT", a)
	}
	el := &rm.Element{ArchetypeNodeID: "at0001", Name: rm.DVText{Value: "item"}, Value: &rm.DVText{Value: "text"}}
	r := validation.Validate(el, c)
	if len(r.Issues) != 0 || !r.OK {
		t.Errorf("Validate(ELEMENT, is_null required by the OPT) = OK %t, issues %q, want OK with none", r.OK, issueKeys(r.Issues))
	}
	if len(r.Coverage.NotInRM) != 0 {
		t.Errorf("Coverage.NotInRM = %q, want none: is_null is not marked", r.Coverage.NotInRM)
	}
}

// TestREQ100_NullFlavorBesideNullFlavour pins § REQ-100 Node taxonomy end to
// end for an ELEMENT that declares both spellings: the template compiles,
// the null_flavour constraint applies to the ELEMENT's null_flavour, and
// null_flavor, kept as written and marked by the compile, is left out of the
// walk and listed in Coverage.NotInRM (§ REQ-171 Coverage).
func TestREQ100_NullFlavorBesideNullFlavour(t *testing.T) {
	// The null_flavour declaration lists 253, a valid null flavour the
	// null_flavor declaration, which lists 271, does not.
	const flavour = `<rm_attribute_name>null_flavor</rm_attribute_name>`
	single := fmt.Sprintf(nullFlavorElementOPT, "null_flavor")
	start := strings.Index(single, `<attributes xsi:type="C_SINGLE_ATTRIBUTE">`)
	end := strings.LastIndex(single, `</attributes>`) + len(`</attributes>`)
	if start < 0 || end < start || strings.Count(single, flavour) != 1 {
		t.Fatal("the null_flavor declaration is not found exactly once")
	}
	decl := single[start:end]
	second := strings.Replace(strings.Replace(decl, flavour, `<rm_attribute_name>null_flavour</rm_attribute_name>`, 1), "<code_list>271</code_list>", "<code_list>253</code_list>", 1)
	both := single[:end] + "\n    " + second + single[end:]

	c := compileStrictOPT(t, both)
	if a := c.Root().Attribute("null_flavor"); a == nil || !a.NotInRM() {
		t.Fatalf("Compile(ELEMENT null_flavor beside null_flavour): null_flavor %+v, want one marked NotInRM", a)
	}
	if a := c.Root().Attribute("null_flavour"); a == nil || a.NotInRM() || a.RMTypeName() != "DV_CODED_TEXT" {
		t.Fatalf("Compile(ELEMENT null_flavor beside null_flavour): null_flavour %+v, want one of RM type DV_CODED_TEXT, not marked", a)
	}
	element := func(code string) *rm.Element {
		return &rm.Element{
			ArchetypeNodeID: "at0001",
			Name:            rm.DVText{Value: "item"},
			NullFlavour: &rm.DVCodedText{
				Value:        "null flavour",
				DefiningCode: rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "openehr"}, CodeString: code},
			},
		}
	}
	wantNotInRM := []string{"/null_flavor"}
	r := validation.Validate(element("253"), c)
	if len(r.Issues) != 0 {
		t.Errorf("Validate(null_flavour 253, listed by null_flavour) issues = %q, want none", issueKeys(r.Issues))
	}
	if !slices.Equal(r.Coverage.NotInRM, wantNotInRM) {
		t.Errorf("Coverage.NotInRM = %q, want %q", r.Coverage.NotInRM, wantNotInRM)
	}
	r = validation.Validate(element("271"), c)
	if got, want := issueKeys(r.Issues), []string{"primitive_not_in_list /null_flavour/defining_code"}; !slices.Equal(got, want) {
		t.Errorf("Validate(null_flavour 271, listed by null_flavor only) issues = %q, want %q", got, want)
	}
	if !slices.Equal(r.Coverage.NotInRM, wantNotInRM) {
		t.Errorf("Coverage.NotInRM = %q, want %q", r.Coverage.NotInRM, wantNotInRM)
	}
}
