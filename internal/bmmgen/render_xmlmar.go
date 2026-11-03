package bmmgen

import (
	"bytes"
	"fmt"
	"go/format"
	"slices"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/bmm"
)

// RenderMarshalXMLFile renders the canonical-XML `MarshalXML`
// companion file for every concrete class in `file`. The output is
// the parallel of [RenderMarshalJSONFile]: each emitted class gains a
// pair of methods: [BMMName] returning the BMM class identifier,
// and [MarshalXML] writing the canonical-XML representation.
//
// Returns (nil, nil) when the file has no concrete classes. The
// caller should skip writing such files entirely.
//
// # Emission strategy
//
// For each concrete class C, the file emits:
//
//  1. `func (c *C) BMMName() string { return "BMM_NAME" }` returns the
//     polymorphic discriminator used by both `xsi:type` (canxml) and
//     `_type` (canjson). Centralising it here means consumers can
//     introspect the BMM name through a typed Go method instead of a
//     reverse lookup against the type registry.
//
//  2. `func (c *C) MarshalXML(e *xml.Encoder, start xml.StartElement) error`
//     writes the canonical-XML representation. Element local name
//     defaults to the snake-cased BMM class name when the parent did
//     not set one. Child elements follow the `xs:sequence` of the
//     class's ITS-XML schema type, or of its nearest ancestor's, with
//     the extension bases' elements first; a property the schema does
//     not declare follows, in the generator's property order. A plan
//     built without the schemas, as a test may build one, keeps the
//     generator's property order. Nil-pointer optionals and empty
//     containers are omitted. Polymorphic descendants receive
//     `xsi:type` via [canxml.EncodePoly]. A member whose element the
//     target's root schemas give a choice of shapes writes the shape it
//     holds, with no `xsi:type` ([marshalXMLChoice]).
//
// An Octet list (DV_MULTIMEDIA's `data` and `integrity_check`, Go []byte) is
// one element of standard base64 text, the `xs:base64Binary` the ITS-XML
// schema gives it and the string canonical JSON writes.
//
// # Hash members
//
// A `Hash<K,V>` member is written one element per entry, named after the
// member, in ascending order of the keys, in the shapes the AM Release-1.4
// schemas give them ([xmlHashOf]): a Hash of Strings as StringDictionaryItems,
// the key in `id` and the value as text; a Hash of class values through the
// value class's own encoder, the key in the attribute the class's schema type
// declares that names no member, or, with none, not written, the value's
// language code being its key. A Hash the codec does not model, a Hash of
// Hashes, is handed to encoding/xml, which refuses a map. Documented in
// canxml/doc.go.
func RenderMarshalXMLFile(plan *Plan, file *PlannedFile) ([]byte, error) {
	emitting, err := codecClassesIn(plan, file)
	if err != nil {
		return nil, err
	}
	if len(emitting) == 0 {
		return nil, nil
	}
	classFields := make(map[string][]emittedField, len(emitting))
	for _, pc := range emitting {
		fields, err := effectiveFields(plan, pc)
		if err != nil {
			return nil, err
		}
		ordered, err := xmlChildFields(plan, pc, fields)
		if err != nil {
			return nil, err
		}
		classFields[pc.BMMName] = ordered
	}

	var body bytes.Buffer
	body.WriteString(renderGeneratedHeader(plan))
	body.WriteString("\n")

	// Pre-render to decide whether the cross-target import is needed.
	chunks := make([]string, 0, len(emitting))
	for _, pc := range emitting {
		chunk, err := renderMarshalXML(plan, pc, classFields[pc.BMMName])
		if err != nil {
			return nil, fmt.Errorf("render MarshalXML %s: %w", pc.BMMName, err)
		}
		chunks = append(chunks, chunk)
	}

	body.WriteString("import (\n")
	if slices.ContainsFunc(chunks, func(c string) bool { return strings.Contains(c, "base64.") }) {
		body.WriteString("\t\"encoding/base64\"\n")
	}
	body.WriteString("\t\"encoding/xml\"\n")
	if slices.ContainsFunc(chunks, func(c string) bool { return strings.Contains(c, "fmt.") }) {
		body.WriteString("\t\"fmt\"\n")
	}
	if slices.ContainsFunc(chunks, func(c string) bool { return strings.Contains(c, "slices.Sorted(maps.Keys(") }) {
		body.WriteString("\t\"maps\"\n\t\"slices\"\n")
	}
	body.WriteString("\n")
	if slices.ContainsFunc(chunks, func(c string) bool { return strings.Contains(c, "xmlschema.") }) {
		fmt.Fprintf(&body, "\t%q\n", xmlschemaImportPath)
	}
	body.WriteString("\t\"github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml\"\n")
	if needsExternalImportForJSONMar(plan, chunks) {
		fmt.Fprintf(&body, "\t%q\n", plan.Target.ExternalImport)
	}
	body.WriteString(")\n\n")

	if file.PackagePath != "" {
		fmt.Fprintf(&body, "// BMM package: %s — canonical-XML MarshalXML companions\n\n", file.PackagePath)
	} else {
		body.WriteString("// canonical-XML MarshalXML companions (foundation classes)\n\n")
	}

	for _, c := range chunks {
		body.WriteString(c)
		body.WriteString("\n")
	}

	formatted, err := format.Source(body.Bytes())
	if err != nil {
		return body.Bytes(), fmt.Errorf("gofmt %s_xmlmar_gen.go: %w", file.FileBase, err)
	}
	return formatted, nil
}

// renderMarshalXML emits the BMMName + MarshalXML methods for one
// concrete class. Field set is supplied by the caller so it does not
// need to be recomputed.
func renderMarshalXML(plan *Plan, pc *PlannedClass, fields []emittedField) (string, error) {
	sc, ok := pc.Class.(*bmm.SimpleClass)
	if !ok {
		return "", fmt.Errorf("expected SimpleClass for %s, got %T", pc.BMMName, pc.Class)
	}

	recv := jsonmarReceiverName(pc.GoName)
	typeArgs := ""
	typeParams := ""
	if sc.IsGeneric() {
		typeParams = genericClassParamList(plan, sc)
		typeArgs = genericTypeArgList(sc)
	}
	_ = typeParams // reserved — generic type parameter list lives on the receiver via typeArgs

	// Partition fields: the members the class's ITS-XML schema type
	// declares as XML attributes go on the start token, in the type's
	// order; the others are child elements.
	attrs, err := xmlAttributesOf(plan, pc, fields)
	if err != nil {
		return "", err
	}
	var elemFields []emittedField
	for _, ef := range fields {
		if !slices.Contains(attrs.members, ef.Prop.PropertyName()) {
			elemFields = append(elemFields, ef)
		}
	}

	var b strings.Builder

	// BMMName method. An abstract class (BASE Interval) is no polymorphic
	// discriminator value, so it gets none.
	if !sc.IsAbstract() {
		fmt.Fprintf(&b, "// BMMName returns %q — the BMM class identifier used as the\n", pc.BMMName)
		b.WriteString("// `xsi:type` polymorphic discriminator in canonical XML and the\n")
		b.WriteString("// `_type` discriminator in canonical JSON.\n")
		fmt.Fprintf(&b, "func (%s *%s%s) BMMName() string { return %q }\n\n", recv, pc.GoName, typeArgs, pc.BMMName)
	}

	// MarshalXML method.
	fmt.Fprintf(&b, "// MarshalXML emits canonical openEHR XML for %s. The default\n", pc.GoName)
	b.WriteString("// element local name is the snake-cased BMM class name when the\n")
	if plan.xmlOrder != nil {
		b.WriteString("// parent did not set one. Child elements follow the ITS-XML schema\n")
		b.WriteString("// sequence of the class, then any property the schema does not\n")
		b.WriteString("// declare; nil-pointer optionals and empty containers are omitted.\n")
		b.WriteString("// Polymorphic descendants are emitted via canxml.EncodePoly.\n")
	} else {
		b.WriteString("// parent did not set one. Child elements follow BMM property\n")
		b.WriteString("// declaration order; nil-pointer optionals and empty containers are\n")
		b.WriteString("// omitted. Polymorphic descendants are emitted via canxml.EncodePoly.\n")
	}
	b.WriteString("// Members the class's ITS-XML schema type declares as XML attributes\n")
	b.WriteString("// are appended to start.Attr before the start token is written.\n")
	hashes, err := hasXMLHash(plan, fields)
	if err != nil {
		return "", err
	}
	if hashes {
		b.WriteString("// A Hash member writes one element per entry, in ascending key order.\n")
	}
	shaped := hasIntervalShape(fields)
	if shaped {
		b.WriteString("//\n")
		b.WriteString("// An open side (`lower_unbounded` or `upper_unbounded` set) whose bound\n")
		b.WriteString("// is empty emits no `lower` or `upper` element.\n")
	}
	fmt.Fprintf(&b, "func (%s *%s%s) MarshalXML(_e *xml.Encoder, _start xml.StartElement) error {\n", recv, pc.GoName, typeArgs)
	b.WriteString("\tif _start.Name.Local == \"\" {\n")
	fmt.Fprintf(&b, "\t\t_start.Name = xml.Name{Local: canxml.ElementName(%q)}\n", pc.BMMName)
	b.WriteString("\t}\n")
	// Append the attribute members.
	for _, name := range attrs.members {
		b.WriteString(marshalXMLAttribute(recv, FieldName(name), name))
	}
	b.WriteString("\tif err := _e.EncodeToken(_start); err != nil {\n\t\treturn err\n\t}\n")

	for _, ef := range elemFields {
		choice, err := xmlChoiceOf(plan, pc, ef)
		if err != nil {
			return "", err
		}
		var line string
		if choice != nil {
			line = marshalXMLChoice(recv, FieldName(ef.Prop.PropertyName()), ef.Prop.PropertyName(), choice, isMandatoryProperty(ef.Prop))
		} else if line, err = renderMarshalXMLField(plan, recv, sc, ef); err != nil {
			return "", fmt.Errorf("render XML field %s.%s: %w", pc.BMMName, ef.Prop.PropertyName(), err)
		}
		if shaped {
			line = guardOpenIntervalBoundXML(recv, ef.Prop.PropertyName(), line)
		}
		b.WriteString(line)
	}

	b.WriteString("\tif err := _e.EncodeToken(_start.End()); err != nil {\n\t\treturn err\n\t}\n")
	b.WriteString("\treturn nil\n")
	b.WriteString("}\n")

	return b.String(), nil
}

// marshalXMLAttribute emits the source line that appends one XML
// attribute to _start.Attr before the start token is written, for a member
// the class's ITS-XML schema type declares as an attribute. The member is a
// mandatory String, held as a Go string: [xmlAttributesOf] refuses any other.
func marshalXMLAttribute(recv, goField, attrName string) string {
	return fmt.Sprintf(
		"\t_start.Attr = append(_start.Attr, xml.Attr{Name: xml.Name{Local: %q}, Value: %s.%s})\n",
		attrName, recv, goField)
}

// renderMarshalXMLField returns the source-code lines that emit one
// property's XML encoding. Mirrors the kinds handled by renderField
// in render.go: SingleProperty (mandatory/optional, primitive/struct/
// interface), SinglePropertyOpen (generic-param), ContainerProperty
// (list/hash, mono/poly), GenericProperty (concrete generic).
//
// emitting is the concrete class whose codec is being rendered;
// distinct from ef.Owner when the property is inherited.
func renderMarshalXMLField(plan *Plan, recv string, emitting *bmm.SimpleClass, ef emittedField) (string, error) {
	propName := ef.Prop.PropertyName()
	goField := FieldName(propName)
	elemName := propName

	// A Hash member writes one element per entry; one the codec does not model
	// goes to encoding/xml below, which refuses a map.
	hash, _, err := xmlHashOf(plan, ef)
	if err != nil {
		return "", err
	}
	if hash != nil {
		return marshalXMLHash(recv, goField, elemName, hash), nil
	}

	switch p := ef.Prop.(type) {
	case *bmm.SingleProperty:
		if p.TypeName == bmmAnyType {
			return marshalXMLAny(recv, goField, elemName, p.IsMandatory), nil
		}
		_, kind := polymorphicProperty(plan, ef.Owner, emitting, p)
		if kind == polySingle || kind == polySingleNarrow {
			return marshalXMLPolySingle(recv, goField, elemName), nil
		}
		isClass := !isPrimitive(p.TypeName)
		isInterface := isInterfaceTypeRef(plan, p.TypeName)
		if isInterface {
			// Should already be caught by polymorphicProperty; defensive.
			return marshalXMLPolySingle(recv, goField, elemName), nil
		}
		if p.IsMandatory {
			if isClass {
				return marshalXMLStructMandatory(recv, goField, elemName), nil
			}
			return marshalXMLPrimitiveMandatory(recv, goField, elemName), nil
		}
		if isClass {
			return marshalXMLStructOptional(recv, goField, elemName), nil
		}
		return marshalXMLPrimitiveOptional(recv, goField, elemName), nil

	case *bmm.SinglePropertyOpen:
		_, kind := polymorphicProperty(plan, ef.Owner, emitting, p)
		if kind == polySingle {
			// Open generic parameter with abstract bound (e.g. EVENT.data:
			// T where T ItemStructure). T may be a value or pointer type
			// at instantiation, so a Go-level `!= nil` check would not
			// compile. Emit unconditionally; canxml.EncodePoly handles the
			// nil-interface case at runtime.
			return marshalXMLPolyOpen(recv, goField, elemName), nil
		}
		// Open generic param bound to a concrete primitive or struct.
		// We don't know the concrete type at codegen time; emit a
		// generic call that defers to encoding/xml at runtime.
		if p.IsMandatory {
			return marshalXMLGenericMandatory(recv, goField, elemName), nil
		}
		return marshalXMLGenericOptional(recv, goField, elemName), nil

	case *bmm.ContainerProperty:
		_, kind := polymorphicProperty(plan, ef.Owner, emitting, p)
		if kind == polySlice || kind == polySliceNarrow {
			return marshalXMLPolySlice(recv, goField, elemName), nil
		}
		if p.TypeDef != nil && p.TypeDef.ContainerType == "Hash" {
			return marshalXMLStructMandatory(recv, goField, elemName), nil
		}
		innerIsClass := containerInnerIsClass(plan, p.TypeDef)
		if innerIsClass {
			return marshalXMLStructSlice(recv, goField, elemName), nil
		}
		if isOctetContainer(p.TypeDef) {
			// One xs:base64Binary element, as the ITS-XML schema types
			// DV_MULTIMEDIA.data, matching the base64 string canonical
			// JSON writes.
			return marshalXMLBase64(recv, goField, elemName), nil
		}
		return marshalXMLPrimitiveSlice(recv, goField, elemName), nil

	case *bmm.GenericProperty:
		// Concrete generic (e.g. DVInterval[DVCount]) — emit as a
		// struct field.
		if p.IsMandatory {
			return marshalXMLStructMandatory(recv, goField, elemName), nil
		}
		return marshalXMLStructOptional(recv, goField, elemName), nil

	default:
		return "", fmt.Errorf("unhandled property kind %T", p)
	}
}

// containerInnerIsClass reports whether the element type of a
// container is a Go class (struct/interface) rather than a primitive.
func containerInnerIsClass(plan *Plan, td *bmm.ContainerType) bool {
	if td == nil || td.TypeDef == nil {
		return false
	}
	switch inner := td.TypeDef.(type) {
	case *bmm.SimpleType:
		if isPrimitive(inner.TypeName) || isSkippedPrimitive(inner.TypeName) {
			return false
		}
		_, ok := plan.Classes[inner.TypeName]
		return ok
	case *bmm.GenericType:
		if isPrimitive(inner.RootType) || isSkippedPrimitive(inner.RootType) {
			return false
		}
		_, ok := plan.Classes[inner.RootType]
		return ok
	}
	return false
}

// --- Per-shape emission helpers ----------------------------------------

// marshalXMLAny emits the encoding of a property typed by the BMM root type
// Any, held as a Go any, or as *any when optional: a string, the text of an
// item with no `xsi:type`, and an xmlschema.TypedString, which writes its own
// `xsi:type`, go to encoding/xml; any other value is a class value, written
// through canxml.EncodePoly with its `xsi:type`; a nil member writes nothing.
func marshalXMLAny(recv, field, elem string, mandatory bool) string {
	start := fmt.Sprintf("xml.StartElement{Name: xml.Name{Local: %q}}", elem)
	value := recv + "." + field
	indent := "\t"
	var b strings.Builder
	if !mandatory {
		fmt.Fprintf(&b, "\tif %s != nil {\n", value)
		value = "*" + value
		indent = "\t\t"
	}
	fmt.Fprintf(&b, "%sswitch _v := %s.(type) {\n", indent, value)
	fmt.Fprintf(&b, "%scase nil:\n", indent)
	fmt.Fprintf(&b, "%scase string, xmlschema.TypedString, *xmlschema.TypedString:\n", indent)
	fmt.Fprintf(&b, "%s\tif err := _e.EncodeElement(_v, %s); err != nil {\n%s\t\treturn err\n%s\t}\n", indent, start, indent, indent)
	fmt.Fprintf(&b, "%sdefault:\n", indent)
	fmt.Fprintf(&b, "%s\tif err := canxml.EncodePoly(_e, %q, _v); err != nil {\n%s\t\treturn err\n%s\t}\n", indent, elem, indent, indent)
	fmt.Fprintf(&b, "%s}\n", indent)
	if !mandatory {
		b.WriteString("\t}\n")
	}
	return b.String()
}

func marshalXMLPrimitiveMandatory(recv, field, elem string) string {
	return fmt.Sprintf("\tif err := _e.EncodeElement(%s.%s, xml.StartElement{Name: xml.Name{Local: %q}}); err != nil {\n\t\treturn err\n\t}\n", recv, field, elem)
}

func marshalXMLPrimitiveOptional(recv, field, elem string) string {
	return fmt.Sprintf(
		"\tif %s.%s != nil {\n\t\tif err := _e.EncodeElement(*%s.%s, xml.StartElement{Name: xml.Name{Local: %q}}); err != nil {\n\t\t\treturn err\n\t\t}\n\t}\n",
		recv, field, recv, field, elem)
}

func marshalXMLStructMandatory(recv, field, elem string) string {
	return fmt.Sprintf("\tif err := _e.EncodeElement(&%s.%s, xml.StartElement{Name: xml.Name{Local: %q}}); err != nil {\n\t\treturn err\n\t}\n", recv, field, elem)
}

func marshalXMLStructOptional(recv, field, elem string) string {
	return fmt.Sprintf(
		"\tif %s.%s != nil {\n\t\tif err := _e.EncodeElement(%s.%s, xml.StartElement{Name: xml.Name{Local: %q}}); err != nil {\n\t\t\treturn err\n\t\t}\n\t}\n",
		recv, field, recv, field, elem)
}

func marshalXMLPolySingle(recv, field, elem string) string {
	return fmt.Sprintf(
		"\tif %s.%s != nil {\n\t\tif err := canxml.EncodePoly(_e, %q, %s.%s); err != nil {\n\t\t\treturn err\n\t\t}\n\t}\n",
		recv, field, elem, recv, field)
}

func marshalXMLPolyOpen(recv, field, elem string) string {
	return fmt.Sprintf(
		"\tif err := canxml.EncodePoly(_e, %q, %s.%s); err != nil {\n\t\treturn err\n\t}\n",
		elem, recv, field)
}

func marshalXMLPolySlice(recv, field, elem string) string {
	return fmt.Sprintf(
		"\tfor _, _item := range %s.%s {\n\t\tif _item == nil {\n\t\t\tcontinue\n\t\t}\n\t\tif err := canxml.EncodePoly(_e, %q, _item); err != nil {\n\t\t\treturn err\n\t\t}\n\t}\n",
		recv, field, elem)
}

func marshalXMLStructSlice(recv, field, elem string) string {
	return fmt.Sprintf(
		"\tfor _idx := range %s.%s {\n\t\tif err := _e.EncodeElement(&%s.%s[_idx], xml.StartElement{Name: xml.Name{Local: %q}}); err != nil {\n\t\t\treturn err\n\t\t}\n\t}\n",
		recv, field, recv, field, elem)
}

// marshalXMLBase64 emits an Octet list, held as a Go []byte, as one element
// whose text is its standard, padded base64 encoding; an empty list emits no
// element.
func marshalXMLBase64(recv, field, elem string) string {
	return fmt.Sprintf(
		"\tif len(%s.%s) > 0 {\n\t\tif err := _e.EncodeElement(base64.StdEncoding.EncodeToString(%s.%s), xml.StartElement{Name: xml.Name{Local: %q}}); err != nil {\n\t\t\treturn err\n\t\t}\n\t}\n",
		recv, field, recv, field, elem)
}

func marshalXMLPrimitiveSlice(recv, field, elem string) string {
	return fmt.Sprintf(
		"\tfor _, _item := range %s.%s {\n\t\tif err := _e.EncodeElement(_item, xml.StartElement{Name: xml.Name{Local: %q}}); err != nil {\n\t\t\treturn err\n\t\t}\n\t}\n",
		recv, field, elem)
}

func marshalXMLGenericMandatory(recv, field, elem string) string {
	// Pass the field address so pointer-receiver MarshalXML on the
	// concrete T (e.g. *DVQuantity for DVInterval[T]) dispatches
	// correctly. encoding/xml only calls pointer-receiver methods on
	// addressable values; a by-value field copy in EncodeElement is
	// NOT addressable and falls back to reflection, which emits the
	// Go field names (PascalCase) instead of our snake_case wire form.
	return fmt.Sprintf("\tif err := _e.EncodeElement(&%s.%s, xml.StartElement{Name: xml.Name{Local: %q}}); err != nil {\n\t\treturn err\n\t}\n", recv, field, elem)
}

func marshalXMLGenericOptional(recv, field, elem string) string {
	// Open generic parameter — declared at the field as `T` (no
	// pointer indirection). Take the address so pointer-receiver
	// MarshalXML on T dispatches; see [marshalXMLGenericMandatory].
	return fmt.Sprintf("\tif err := _e.EncodeElement(&%s.%s, xml.StartElement{Name: xml.Name{Local: %q}}); err != nil {\n\t\treturn err\n\t}\n", recv, field, elem)
}
