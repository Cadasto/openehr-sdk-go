package bmmgen

import (
	"fmt"
	"slices"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/bmm"
)

// xmlChoice is a member whose element an ITS-XML schema gives one of several
// shapes, which the target's roots name in their schema_forms with the form
// [schemaFormChoice]: the decoder chooses the shape by the element's first
// child, and the encoder writes the shape the member holds.
type xmlChoice struct {
	// label names the member as its class and property, such as
	// ORDINAL.symbol.
	label string
	// varName is the package-level list of shapes the decoder hands
	// xmlunmatched.DecodeChoice.
	varName string
	// shapes are the shapes in the entry's order.
	shapes []xmlChoiceShape
}

// xmlChoiceShape is one shape of an [xmlChoice]: the Go type of its class, the
// child elements its schema type declares, and those of them the member's own
// schema type makes optional that the class holds as a mandatory member, which
// the class's encoder always writes: the member's encoder leaves each out when
// it is empty.
type xmlChoiceShape struct {
	class     string
	goType    string
	elements  []string
	omitEmpty []string
}

// xmlChoiceOf returns the choice ef, a member of the class pc, takes, or nil
// when the target's roots give the member no choice form, for pc or for a class
// pc inherits from. A choice is refused when its member is not a single
// property typed Any, which alone holds a value of any shape; when one of its
// shapes is a class with no generated XML decoder or a generic class; and when
// two of its shapes share an element, so the first child could not choose.
// Each shape's elements are its schema type's, or, in a plan built without the
// schemas, its members'. A shape's mandatory member whose element the type the
// schemas give the member's own element makes optional, such as the `value` of
// the DV_CODED_TEXT shape of ORDINAL.symbol, is left out when empty; a plan
// built without the schemas leaves out none.
func xmlChoiceOf(plan *Plan, pc *PlannedClass, ef emittedField) (*xmlChoice, error) {
	prop := ef.Prop.PropertyName()
	var form schemaForm
	found := false
	for _, class := range append([]string{pc.BMMName}, AncestorChain(plan, pc.BMMName)...) {
		if form, found = plan.extensions.schemaForms[class][prop]; found {
			break
		}
	}
	if !found || form.Form != schemaFormChoice {
		return nil, nil
	}
	label := form.Class + "." + prop
	if p, ok := ef.Prop.(*bmm.SingleProperty); !ok || p.TypeName != bmmAnyType {
		return nil, fmt.Errorf("%s.schema_forms: %s is typed %s; a choice needs a member typed %s", sdkExtensionsMember, label, propertyTypeName(ef.Prop), bmmAnyType)
	}
	c := &xmlChoice{label: label, varName: "xmlChoice" + pc.GoName + FieldName(prop)}
	var optional []string
	if plan.xmlOrder != nil {
		optional = plan.xmlOrder.optionalChildrenOf(plan, pc.BMMName, prop)
	}
	owner := map[string]string{} // element → the shape that declares it
	for _, class := range form.Shapes {
		shape, err := xmlChoiceShapeOf(plan, label, class, optional)
		if err != nil {
			return nil, err
		}
		for _, elem := range shape.elements {
			if other, shared := owner[elem]; shared {
				return nil, fmt.Errorf("%s.schema_forms: the shapes %s and %s of %s share the element %s, so the first child cannot choose", sdkExtensionsMember, other, class, label, elem)
			}
			owner[elem] = class
		}
		c.shapes = append(c.shapes, shape)
	}
	return c, nil
}

// xmlChoiceShapeOf returns the shape class takes in the choice label names,
// whose element's schema type makes the elements optional names optional.
func xmlChoiceShapeOf(plan *Plan, label, class string, optional []string) (xmlChoiceShape, error) {
	decoding, err := xmlClassDecoding(plan, class)
	if err != nil {
		return xmlChoiceShape{}, err
	}
	pc := plan.Classes[class]
	sc, simple := pc.Class.(*bmm.SimpleClass)
	if decoding != xmlOwnDecoder || !simple || sc.IsGeneric() {
		return xmlChoiceShape{}, fmt.Errorf("%s.schema_forms: the shape %s of %s has no XML decoder of its own", sdkExtensionsMember, class, label)
	}
	fields, err := effectiveFields(plan, pc)
	if err != nil {
		return xmlChoiceShape{}, err
	}
	ordered, err := xmlChildFields(plan, pc, fields)
	if err != nil {
		return xmlChoiceShape{}, err
	}
	attrs, err := xmlAttributesOf(plan, pc, fields)
	if err != nil {
		return xmlChoiceShape{}, err
	}
	shape := xmlChoiceShape{class: class, goType: qualifyClassRef(plan, pc)}
	for _, f := range ordered {
		name := f.Prop.PropertyName()
		if slices.Contains(attrs.members, name) {
			continue
		}
		shape.elements = append(shape.elements, name)
		if isMandatoryProperty(f.Prop) && slices.Contains(optional, name) {
			shape.omitEmpty = append(shape.omitEmpty, name)
		}
	}
	return shape, nil
}

// propertyTypeName returns the BMM type a property is declared with, as a
// message names it.
func propertyTypeName(prop bmm.Property) string {
	switch p := prop.(type) {
	case *bmm.SingleProperty:
		return p.TypeName
	case *bmm.SinglePropertyOpen:
		return p.TypeName
	case *bmm.ContainerProperty:
		if p.TypeDef != nil {
			return p.TypeDef.ContainerType + "<" + containerInnerName(p.TypeDef) + ">"
		}
	case *bmm.GenericProperty:
		if p.TypeDef != nil {
			return p.TypeDef.RootType
		}
	}
	return fmt.Sprintf("%T", prop)
}

// renderXMLChoiceVar renders the package-level list of c's shapes the decoder
// hands xmlunmatched.DecodeChoice, one shape per line, each with the members
// the shape makes optional that its class requires, which the missing-member
// report leaves out.
func renderXMLChoiceVar(c *xmlChoice) string {
	var b strings.Builder
	fmt.Fprintf(&b, "// %s lists the shapes the element of %s takes, which its first child chooses.\n", c.varName, c.label)
	fmt.Fprintf(&b, "var %s = []xmlunmatched.Shape{\n", c.varName)
	for _, s := range c.shapes {
		optional := ""
		if len(s.omitEmpty) > 0 {
			optional = ", Optional: " + stringSliceLiteral(s.omitEmpty)
		}
		fmt.Fprintf(&b, "\t{Elements: %s%s, New: func() any { return new(%s) }},\n", stringSliceLiteral(s.elements), optional, s.goType)
	}
	b.WriteString("}\n")
	return b.String()
}

// unmarshalXMLChoice emits the decode case of a choice member, held as a Go
// any, or as *any when optional: xmlunmatched.DecodeChoice decodes the element
// as the shape its first child chooses and returns the pointer to the value.
func unmarshalXMLChoice(recv, field, elem string, c *xmlChoice, mandatory bool) string {
	assign := fmt.Sprintf("\t\t\t\t%s.%s = _v\n", recv, field)
	if !mandatory {
		assign = fmt.Sprintf("\t\t\t\t%s.%s = &_v\n", recv, field)
	}
	return fmt.Sprintf(
		"\t\t\tcase %q:\n\t\t\t\t_v, _err := xmlunmatched.DecodeChoice(_dec, _t, %s)\n\t\t\t\tif _err != nil {\n\t\t\t\t\treturn _err\n\t\t\t\t}\n%s",
		elem, c.varName, assign)
}

// marshalXMLChoice emits the encoding of a choice member: the value it holds,
// a pointer to one of the shapes or a shape by value, under the member's
// element, through the shape's own MarshalXML, which writes no `xsi:type`; a
// nil member writes nothing, and a value of any other type is refused. A
// shape with members to leave out when empty is written through
// xmlschema.EncodeElementOmittingEmpty.
func marshalXMLChoice(recv, field, elem string, c *xmlChoice, mandatory bool) string {
	start := fmt.Sprintf("xml.StartElement{Name: xml.Name{Local: %q}}", elem)
	var b strings.Builder
	value := recv + "." + field
	indent := "\t"
	if !mandatory {
		fmt.Fprintf(&b, "\tif %s != nil {\n", value)
		value = "*" + value
		indent = "\t\t"
	}
	encode := func(s xmlChoiceShape, v string) {
		call := fmt.Sprintf("_e.EncodeElement(%s, %s)", v, start)
		if len(s.omitEmpty) > 0 {
			args := make([]string, len(s.omitEmpty))
			for i, name := range s.omitEmpty {
				args[i] = fmt.Sprintf("%q", name)
			}
			call = fmt.Sprintf("xmlschema.EncodeElementOmittingEmpty(_e, %s, %s, %s)", v, start, strings.Join(args, ", "))
		}
		fmt.Fprintf(&b, "%s\tif err := %s; err != nil {\n%s\t\treturn err\n%s\t}\n", indent, call, indent, indent)
	}
	fmt.Fprintf(&b, "%sswitch _v := %s.(type) {\n", indent, value)
	fmt.Fprintf(&b, "%scase nil:\n", indent)
	for _, s := range c.shapes {
		fmt.Fprintf(&b, "%scase *%s:\n", indent, s.goType)
		encode(s, "_v")
	}
	for _, s := range c.shapes {
		fmt.Fprintf(&b, "%scase %s:\n", indent, s.goType)
		encode(s, "&_v")
	}
	fmt.Fprintf(&b, "%sdefault:\n", indent)
	fmt.Fprintf(&b, "%s\treturn fmt.Errorf(\"canxml: %s holds %%T, which is none of the shapes its element takes\", _v)\n", indent, c.label)
	fmt.Fprintf(&b, "%s}\n", indent)
	if !mandatory {
		b.WriteString("\t}\n")
	}
	return b.String()
}

// isMandatoryProperty reports whether prop is a mandatory single property.
func isMandatoryProperty(prop bmm.Property) bool {
	p, ok := prop.(*bmm.SingleProperty)
	return ok && p.IsMandatory
}
