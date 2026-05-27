package bmmgen

import (
	"bytes"
	"fmt"
	"go/format"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/bmm"
)

// RenderUnmarshalXMLFile renders the canonical-XML `UnmarshalXML`
// companion file for every concrete class in `file` — the parallel
// of [RenderUnmarshalJSONFile] for XML. Each emitted class gains
// one method:
//
//	func (c *C) UnmarshalXML(dec *xml.Decoder, start xml.StartElement) error
//
// The method walks the child tokens of `start`, dispatching each on
// the snake-cased element local name back to the matching struct
// field. Polymorphic fields are routed through
// [canxml.DecodeAs] which consults [typereg.Default] for the
// `xsi:type` discriminator.
//
// Returns (nil, nil) when the file has no concrete classes.
//
// # Strategy
//
// For each concrete class C:
//
//  1. Initialise a flat switch on the child element's local name
//     covering every effective property (in any order — XML decode
//     is name-keyed, unlike encode which is order-pinned).
//
//  2. Per property:
//     - Mandatory / optional primitive → `dec.DecodeElement(&v.Field, &t)`
//     - Mandatory struct → `dec.DecodeElement(&v.Field, &t)` (or `v.Field, ...` for pointer)
//     - Optional struct (pointer) → allocate, decode, assign
//     - Polymorphic single → `canxml.DecodeAs[Iface](dec, t)` + assign
//     - Slice of primitive/struct → append per occurrence
//     - Slice of polymorphic → append result of `canxml.DecodeAs[Iface]`
//
//  3. Unknown elements are skipped via `dec.Skip()` so foreign-namespace
//     additions don't trip strict decode.
//
//  4. Unknown attributes are tolerated; `xmi:type` is rejected via
//     [canxml.XSITypeOf] (centralised in the helper).
//
// # Hash/map decoding
//
// Same v1 limitation as the encoder — Hash<K,V> properties are
// skipped at decode time. The receiver field is left at its zero
// value. Documented in canxml/doc.go.
func RenderUnmarshalXMLFile(plan *Plan, file *PlannedFile) ([]byte, error) {
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

	chunks := make([]string, 0, len(emitting))
	for _, pc := range emitting {
		chunk, err := renderUnmarshalXML(plan, pc, classFields[pc.BMMName])
		if err != nil {
			return nil, fmt.Errorf("render UnmarshalXML %s: %w", pc.BMMName, err)
		}
		chunks = append(chunks, chunk)
	}

	body.WriteString("import (\n")
	body.WriteString("\t\"encoding/xml\"\n")
	body.WriteString("\t\"fmt\"\n\n")
	body.WriteString("\t\"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg\"\n")
	body.WriteString("\t\"github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml\"\n")
	if needsExternalImportForJSONMar(plan, chunks) {
		fmt.Fprintf(&body, "\t%q\n", plan.Target.ExternalImport)
	}
	body.WriteString(")\n\n")

	if file.PackagePath != "" {
		fmt.Fprintf(&body, "// BMM package: %s — canonical-XML UnmarshalXML companions\n\n", file.PackagePath)
	} else {
		body.WriteString("// canonical-XML UnmarshalXML companions (foundation classes)\n\n")
	}

	for _, c := range chunks {
		body.WriteString(c)
		body.WriteString("\n")
	}

	// Imports are stable across the file regardless of whether any
	// single class body references them — tombstone each so go-vet does
	// not complain about unused imports. The blank LHS makes each
	// declaration a no-op at link time. canxml is referenced in the doc
	// comment of every chunk but the comment alone doesn't satisfy the
	// import; the tombstone does.
	body.WriteString("\nvar (\n")
	body.WriteString("\t_ = typereg.ErrMissingType\n")
	body.WriteString("\t_ = fmt.Sprintf\n")
	body.WriteString("\t_ = canxml.NSDefault\n")
	body.WriteString(")\n")

	formatted, err := format.Source(body.Bytes())
	if err != nil {
		return body.Bytes(), fmt.Errorf("gofmt %s_xmlunmar_gen.go: %w", file.FileBase, err)
	}
	return formatted, nil
}

// renderUnmarshalXML emits the UnmarshalXML method body for one
// concrete class.
func renderUnmarshalXML(plan *Plan, pc *PlannedClass, fields []emittedField) (string, error) {
	sc, ok := pc.Class.(*bmm.SimpleClass)
	if !ok {
		return "", fmt.Errorf("expected SimpleClass for %s, got %T", pc.BMMName, pc.Class)
	}

	recv := jsonmarReceiverName(pc.GoName)
	typeArgs := ""
	if sc.IsGeneric() {
		typeArgs = genericTypeArgList(sc)
	}

	var b strings.Builder

	// Partition fields: attribute-typed properties are read from
	// _start.Attr before the token loop; element-typed properties are
	// handled inside the switch. For attribute-typed properties we
	// ALSO keep the child-element case so the decoder remains
	// tolerant of producers that emit the property as a child element
	// (legacy fixtures, the SDK's own pre-fix output). The attribute
	// path runs first; the child-element path overwrites only if the
	// attribute was absent and a child appears.
	var attrFields []emittedField
	elemFields := fields
	for _, ef := range fields {
		if isXMLAttributeProperty(ef.Prop) {
			attrFields = append(attrFields, ef)
		}
	}

	fmt.Fprintf(&b, "// UnmarshalXML decodes canonical openEHR XML into %s.\n", pc.GoName)
	b.WriteString("// Polymorphic fields are routed through canxml.DecodeAs so the\n")
	b.WriteString("// concrete type is selected by `xsi:type` at each polymorphic site.\n")
	b.WriteString("// Missing/unknown/type-mismatch dispatch failures wrap typereg\n")
	b.WriteString("// sentinels inside *canxml.DecodeError for errors.Is / errors.As.\n")
	b.WriteString("// Properties typed as XML attributes per the openEHR ITS-XML XSDs\n")
	b.WriteString("// (currently `archetype_node_id`) are read from _start.Attr.\n")
	fmt.Fprintf(&b, "func (%s *%s%s) UnmarshalXML(_dec *xml.Decoder, _start xml.StartElement) error {\n", recv, pc.GoName, typeArgs)
	// Read attribute-typed properties from _start.Attr.
	for _, ef := range attrFields {
		line := unmarshalXMLAttribute(recv, FieldName(ef.Prop.PropertyName()), ef.Prop.PropertyName())
		b.WriteString(line)
	}
	b.WriteString("\tfor {\n")
	b.WriteString("\t\t_tok, _err := _dec.Token()\n")
	b.WriteString("\t\tif _err != nil {\n")
	b.WriteString("\t\t\treturn _err\n")
	b.WriteString("\t\t}\n")
	b.WriteString("\t\tswitch _t := _tok.(type) {\n")
	b.WriteString("\t\tcase xml.StartElement:\n")
	b.WriteString("\t\t\tswitch _t.Name.Local {\n")

	for _, ef := range elemFields {
		caseBody, err := renderUnmarshalXMLField(plan, recv, sc, ef)
		if err != nil {
			return "", fmt.Errorf("render UnmarshalXML field %s.%s: %w", pc.BMMName, ef.Prop.PropertyName(), err)
		}
		b.WriteString(caseBody)
	}

	b.WriteString("\t\t\tdefault:\n")
	b.WriteString("\t\t\t\tif _err := _dec.Skip(); _err != nil {\n")
	b.WriteString("\t\t\t\t\treturn _err\n")
	b.WriteString("\t\t\t\t}\n")
	b.WriteString("\t\t\t}\n") // close inner switch (Name.Local)
	b.WriteString("\t\tcase xml.EndElement:\n")
	b.WriteString("\t\t\treturn nil\n")
	b.WriteString("\t\t}\n") // close outer switch (token kind)
	b.WriteString("\t}\n")   // close for
	b.WriteString("}\n")
	return b.String(), nil
}

// renderUnmarshalXMLField returns the case branch source for one
// property. The element local name is the property snake_case.
// emitting is the concrete class whose codec is being rendered; it is
// passed through to polymorphicProperty so inherited open generic
// parameters can be classified via the emitting class's narrowed bound.
func renderUnmarshalXMLField(plan *Plan, recv string, emitting *bmm.SimpleClass, ef emittedField) (string, error) {
	propName := ef.Prop.PropertyName()
	goField := FieldName(propName)
	elemName := propName

	switch p := ef.Prop.(type) {
	case *bmm.SingleProperty:
		ifaceName, kind := polymorphicProperty(plan, ef.Owner, emitting, p)
		if kind == polySingle {
			return unmarshalXMLPolySingle(recv, goField, elemName, ifaceName), nil
		}
		if kind == polySingleNarrow {
			// SDK-GAP-11: narrow-interface slot. canxml.DecodeAsOrDefault
			// dispatches via xsi:type when present and falls back to the
			// declared parent's concrete type when the wire omits the
			// discriminator (the openEHR canonical XML tolerance).
			parentGo := strings.TrimSuffix(ifaceName, "Like")
			return unmarshalXMLPolySingleNarrow(recv, goField, elemName, ifaceName, parentGo), nil
		}
		isClass := !isPrimitive(p.TypeName)
		isInterface := isInterfaceTypeRef(plan, p.TypeName)
		if isInterface {
			// True abstract / Like-narrow handled by the polySingle
			// branch above; this defensive fall-through covers any
			// other interface-typed shape.
			return unmarshalXMLPolySingle(recv, goField, elemName, p.TypeName), nil
		}
		if p.IsMandatory {
			if isClass {
				return unmarshalXMLStructMandatory(recv, goField, elemName), nil
			}
			return unmarshalXMLPrimitiveMandatory(recv, goField, elemName), nil
		}
		// Optional — compute the Go type expression that matches the
		// field's declared shape (renderField parity).
		goType, _, err := singleTypeRef(plan, ef.Owner, p.TypeName)
		if err != nil {
			return "", err
		}
		if isClass {
			return unmarshalXMLStructOptionalLit(recv, goField, elemName, goType), nil
		}
		return unmarshalXMLPrimitiveOptionalLit(recv, goField, elemName, goType), nil

	case *bmm.SinglePropertyOpen:
		ifaceName, kind := polymorphicProperty(plan, ef.Owner, emitting, p)
		if kind == polySingle {
			return unmarshalXMLPolySingle(recv, goField, elemName, ifaceName), nil
		}
		// Open generic param bound to a concrete primitive or struct —
		// emit a generic decode that defers to encoding/xml at runtime.
		if p.IsMandatory {
			return unmarshalXMLStructMandatory(recv, goField, elemName), nil
		}
		// Field is declared as `T` (open param), not `*T`. We cannot
		// allocate via new(T) for an unknown layout — decode in place.
		return unmarshalXMLStructMandatory(recv, goField, elemName), nil

	case *bmm.ContainerProperty:
		ifaceName, kind := polymorphicProperty(plan, ef.Owner, emitting, p)
		if kind == polySlice || kind == polySliceNarrow {
			// SDK-GAP-11: narrow-element slices share the same RawMessage
			// + canxml.DecodeAs shape on the XML side. XML cassettes
			// uniformly carry xsi:type on slice items today; a
			// dedicated polySliceNarrow XML path can be added if a
			// cassette appears that omits the discriminator.
			return unmarshalXMLPolySlice(recv, goField, elemName, ifaceName), nil
		}
		if p.TypeDef != nil && p.TypeDef.ContainerType == "Hash" {
			return unmarshalXMLHashTODO(elemName), nil
		}
		innerIsClass := containerInnerIsClass(plan, p.TypeDef)
		innerType, err := containerInner(plan, ef.Owner, p.TypeDef)
		if err != nil {
			return "", err
		}
		if innerIsClass {
			return unmarshalXMLStructSlice(recv, goField, elemName, innerType), nil
		}
		return unmarshalXMLPrimitiveSlice(recv, goField, elemName, innerType), nil

	case *bmm.GenericProperty:
		// Hash<K,V> at the property type level — Go maps it to
		// map[K]V. v1 defers XML emission/decoding (see canxml/doc.go).
		if p.TypeDef != nil && containerKinds[p.TypeDef.RootType] {
			return unmarshalXMLHashTODO(elemName), nil
		}
		if p.IsMandatory {
			return unmarshalXMLStructMandatory(recv, goField, elemName), nil
		}
		goType, err := genericTypeRef(plan, ef.Owner, p.TypeDef)
		if err != nil {
			return "", err
		}
		return unmarshalXMLStructOptionalLit(recv, goField, elemName, goType), nil

	default:
		return "", fmt.Errorf("unhandled property kind %T", p)
	}
}

// --- Per-shape decode helpers ------------------------------------------

func unmarshalXMLPrimitiveMandatory(recv, field, elem string) string {
	return fmt.Sprintf(
		"\t\t\tcase %q:\n\t\t\t\tif _err := _dec.DecodeElement(&%s.%s, &_t); _err != nil {\n\t\t\t\t\treturn _err\n\t\t\t\t}\n",
		elem, recv, field)
}

func unmarshalXMLPrimitiveOptionalLit(recv, field, elem, goType string) string {
	return fmt.Sprintf(
		"\t\t\tcase %q:\n\t\t\t\tvar _v %s\n\t\t\t\tif _err := _dec.DecodeElement(&_v, &_t); _err != nil {\n\t\t\t\t\treturn _err\n\t\t\t\t}\n\t\t\t\t%s.%s = &_v\n",
		elem, goType, recv, field)
}

func unmarshalXMLStructMandatory(recv, field, elem string) string {
	return fmt.Sprintf(
		"\t\t\tcase %q:\n\t\t\t\tif _err := _dec.DecodeElement(&%s.%s, &_t); _err != nil {\n\t\t\t\t\treturn _err\n\t\t\t\t}\n",
		elem, recv, field)
}

func unmarshalXMLStructOptionalLit(recv, field, elem, goType string) string {
	return fmt.Sprintf(
		"\t\t\tcase %q:\n\t\t\t\t_v := new(%s)\n\t\t\t\tif _err := _dec.DecodeElement(_v, &_t); _err != nil {\n\t\t\t\t\treturn _err\n\t\t\t\t}\n\t\t\t\t%s.%s = _v\n",
		elem, goType, recv, field)
}

func unmarshalXMLPolySingle(recv, field, elem, ifaceName string) string {
	return fmt.Sprintf(
		"\t\t\tcase %q:\n\t\t\t\t_v, _err := canxml.DecodeAs[%s](_dec, _t)\n\t\t\t\tif _err != nil {\n\t\t\t\t\treturn _err\n\t\t\t\t}\n\t\t\t\t%s.%s = _v\n",
		elem, ifaceName, recv, field)
}

// unmarshalXMLPolySingleNarrow emits the SDK-GAP-11 narrow-interface
// decode case: canxml.DecodeAsOrDefault dispatches via xsi:type when
// present and otherwise instantiates the parent concrete type
// (`parentGo`) — preserving openEHR canonical XML cassettes that omit
// xsi:type on concrete-typed slots.
func unmarshalXMLPolySingleNarrow(recv, field, elem, ifaceName, parentGo string) string {
	return fmt.Sprintf(
		"\t\t\tcase %q:\n\t\t\t\t_v, _err := canxml.DecodeAsOrDefault[%s](_dec, _t, func() any { return new(%s) })\n\t\t\t\tif _err != nil {\n\t\t\t\t\treturn _err\n\t\t\t\t}\n\t\t\t\t%s.%s = _v\n",
		elem, ifaceName, parentGo, recv, field)
}

func unmarshalXMLPolySlice(recv, field, elem, ifaceName string) string {
	return fmt.Sprintf(
		"\t\t\tcase %q:\n\t\t\t\t_v, _err := canxml.DecodeAs[%s](_dec, _t)\n\t\t\t\tif _err != nil {\n\t\t\t\t\treturn _err\n\t\t\t\t}\n\t\t\t\t%s.%s = append(%s.%s, _v)\n",
		elem, ifaceName, recv, field, recv, field)
}

func unmarshalXMLStructSlice(recv, field, elem, innerType string) string {
	return fmt.Sprintf(
		"\t\t\tcase %q:\n\t\t\t\tvar _v %s\n\t\t\t\tif _err := _dec.DecodeElement(&_v, &_t); _err != nil {\n\t\t\t\t\treturn _err\n\t\t\t\t}\n\t\t\t\t%s.%s = append(%s.%s, _v)\n",
		elem, innerType, recv, field, recv, field)
}

func unmarshalXMLPrimitiveSlice(recv, field, elem, innerType string) string {
	return fmt.Sprintf(
		"\t\t\tcase %q:\n\t\t\t\tvar _v %s\n\t\t\t\tif _err := _dec.DecodeElement(&_v, &_t); _err != nil {\n\t\t\t\t\treturn _err\n\t\t\t\t}\n\t\t\t\t%s.%s = append(%s.%s, _v)\n",
		elem, innerType, recv, field, recv, field)
}

// unmarshalXMLAttribute emits the source lines that consume one
// XML attribute from _start.Attr at the head of UnmarshalXML.
// Mirror of [marshalXMLAttribute]. Tolerates an absent attribute —
// the openEHR ITS-XML XSDs declare `archetype_node_id` mandatory,
// but some upstream fixtures (e.g. ehrbase's simple_empty_folder)
// omit it, so we treat absence as the zero value rather than an
// error.
func unmarshalXMLAttribute(recv, goField, attrName string) string {
	return fmt.Sprintf(
		"\tfor _, _a := range _start.Attr {\n\t\tif _a.Name.Local == %q && _a.Name.Space == \"\" {\n\t\t\t%s.%s = _a.Value\n\t\t\tbreak\n\t\t}\n\t}\n",
		attrName, recv, goField)
}

func unmarshalXMLHashTODO(elem string) string {
	return fmt.Sprintf(
		"\t\t\tcase %q:\n\t\t\t\t// TODO(canxml): Hash<K,V> XML decode deferred — see canxml/doc.go.\n\t\t\t\tif _err := _dec.Skip(); _err != nil {\n\t\t\t\t\treturn _err\n\t\t\t\t}\n",
		elem)
}
