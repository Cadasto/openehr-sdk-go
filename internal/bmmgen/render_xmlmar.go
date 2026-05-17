package bmmgen

import (
	"bytes"
	"fmt"
	"go/format"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/bmm"
)

// RenderMarshalXMLFile renders the canonical-XML `MarshalXML`
// companion file for every concrete class in `file`. The output is
// the parallel of [RenderMarshalJSONFile]: each emitted class gains a
// pair of methods — [BMMName] returning the BMM class identifier,
// and [MarshalXML] writing the canonical-XML representation.
//
// Returns (nil, nil) when the file has no concrete classes. The
// caller should skip writing such files entirely.
//
// # Emission strategy
//
// For each concrete class C, the file emits:
//
//  1. `func (c *C) BMMName() string { return "BMM_NAME" }` — the
//     polymorphic discriminator used by both `xsi:type` (canxml) and
//     `_type` (canjson). Centralising it here means consumers can
//     introspect the BMM name through a typed Go method instead of a
//     reverse lookup against the type registry.
//
//  2. `func (c *C) MarshalXML(e *xml.Encoder, start xml.StartElement) error`
//     — writes the canonical-XML representation. Element local name
//     defaults to the snake-cased BMM class name when the parent did
//     not set one. Child elements follow BMM property declaration
//     order (identical to the JSON ordering). Nil-pointer optionals
//     and empty containers are omitted. Polymorphic descendants
//     receive `xsi:type` via [canxml.EncodePoly].
//
// # Hash/map XML emission (v1 limitation)
//
// `Hash<K,V>` properties are NOT emitted to XML in v1 — there is no
// pinned canonical shape for them in the openEHR ITS-XML release
// the SDK targets. Affected fields are skipped at encode time and
// rejected at decode time (the decoder leaves them as the zero
// value). Composition / EHR_STATUS / Directory / Contribution do
// not use Hash on the critical path; the only RM uses are
// extensibility hooks (`other_details`, `author`, …) under
// AUTHORED_RESOURCE descendants. Documented in canxml/doc.go.
func RenderMarshalXMLFile(plan *Plan, file *PlannedFile) ([]byte, error) {
	emitting := concreteClassesIn(file)
	if len(emitting) == 0 {
		return nil, nil
	}
	classFields := make(map[string][]emittedField, len(emitting))
	for _, pc := range emitting {
		fields, err := effectiveFields(plan, pc)
		if err != nil {
			return nil, err
		}
		classFields[pc.BMMName] = fields
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
	body.WriteString("\t\"encoding/xml\"\n\n")
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

	var b strings.Builder

	// BMMName method.
	fmt.Fprintf(&b, "// BMMName returns %q — the BMM class identifier used as the\n", pc.BMMName)
	b.WriteString("// `xsi:type` polymorphic discriminator in canonical XML and the\n")
	b.WriteString("// `_type` discriminator in canonical JSON.\n")
	fmt.Fprintf(&b, "func (%s *%s%s) BMMName() string { return %q }\n\n", recv, pc.GoName, typeArgs, pc.BMMName)

	// MarshalXML method.
	fmt.Fprintf(&b, "// MarshalXML emits canonical openEHR XML for %s. The default\n", pc.GoName)
	b.WriteString("// element local name is the snake-cased BMM class name when the\n")
	b.WriteString("// parent did not set one. Child elements follow BMM property\n")
	b.WriteString("// declaration order; nil-pointer optionals and empty containers are\n")
	b.WriteString("// omitted. Polymorphic descendants are emitted via canxml.EncodePoly.\n")
	fmt.Fprintf(&b, "func (%s *%s%s) MarshalXML(_e *xml.Encoder, _start xml.StartElement) error {\n", recv, pc.GoName, typeArgs)
	b.WriteString("\tif _start.Name.Local == \"\" {\n")
	fmt.Fprintf(&b, "\t\t_start.Name = xml.Name{Local: canxml.ElementName(%q)}\n", pc.BMMName)
	b.WriteString("\t}\n")
	b.WriteString("\tif err := _e.EncodeToken(_start); err != nil {\n\t\treturn err\n\t}\n")

	for _, ef := range fields {
		line, err := renderMarshalXMLField(plan, recv, ef)
		if err != nil {
			return "", fmt.Errorf("render XML field %s.%s: %w", pc.BMMName, ef.Prop.PropertyName(), err)
		}
		b.WriteString(line)
	}

	b.WriteString("\tif err := _e.EncodeToken(_start.End()); err != nil {\n\t\treturn err\n\t}\n")
	b.WriteString("\treturn nil\n")
	b.WriteString("}\n")

	return b.String(), nil
}

// renderMarshalXMLField returns the source-code lines that emit one
// property's XML encoding. Mirrors the kinds handled by renderField
// in render.go: SingleProperty (mandatory/optional, primitive/struct/
// interface), SinglePropertyOpen (generic-param), ContainerProperty
// (list/hash, mono/poly), GenericProperty (concrete generic).
func renderMarshalXMLField(plan *Plan, recv string, ef emittedField) (string, error) {
	propName := ef.Prop.PropertyName()
	goField := FieldName(propName)
	elemName := propName

	switch p := ef.Prop.(type) {
	case *bmm.SingleProperty:
		_, kind := polymorphicProperty(plan, ef.Owner, p)
		if kind == polySingle {
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
		_, kind := polymorphicProperty(plan, ef.Owner, p)
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
		_, kind := polymorphicProperty(plan, ef.Owner, p)
		if kind == polySlice {
			return marshalXMLPolySlice(recv, goField, elemName), nil
		}
		if p.TypeDef != nil && p.TypeDef.ContainerType == "Hash" {
			return marshalXMLHashTODO(goField, elemName), nil
		}
		innerIsClass := containerInnerIsClass(plan, p.TypeDef)
		if innerIsClass {
			return marshalXMLStructSlice(recv, goField, elemName), nil
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

func marshalXMLPrimitiveSlice(recv, field, elem string) string {
	return fmt.Sprintf(
		"\tfor _, _item := range %s.%s {\n\t\tif err := _e.EncodeElement(_item, xml.StartElement{Name: xml.Name{Local: %q}}); err != nil {\n\t\t\treturn err\n\t\t}\n\t}\n",
		recv, field, elem)
}

func marshalXMLGenericMandatory(recv, field, elem string) string {
	return fmt.Sprintf("\tif err := _e.EncodeElement(%s.%s, xml.StartElement{Name: xml.Name{Local: %q}}); err != nil {\n\t\treturn err\n\t}\n", recv, field, elem)
}

func marshalXMLGenericOptional(recv, field, elem string) string {
	// Open generic parameter — declared at the field as `T` (no
	// pointer indirection). Emit unconditionally; encoding/xml skips
	// zero-value when MarshalXML returns nothing meaningful.
	return fmt.Sprintf("\tif err := _e.EncodeElement(%s.%s, xml.StartElement{Name: xml.Name{Local: %q}}); err != nil {\n\t\treturn err\n\t}\n", recv, field, elem)
}

func marshalXMLHashTODO(field, elem string) string {
	return fmt.Sprintf("\t// TODO(canxml): Hash<K,V> XML emission deferred for %s/%s — see canxml/doc.go.\n\t_ = %q\n", field, elem, elem)
}
