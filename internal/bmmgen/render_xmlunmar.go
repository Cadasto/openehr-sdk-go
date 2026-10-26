package bmmgen

import (
	"bytes"
	"fmt"
	"go/format"
	"maps"
	"slices"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/bmm"
)

// RenderUnmarshalXMLFile renders the canonical-XML `UnmarshalXML`
// companion file for every concrete class in `file`, the parallel
// of [RenderUnmarshalJSONFile] for XML. Each emitted class gains
// one method and, beside it, one description of the class:
//
//	var xmlClassC = xmlunmatched.Class{Name: "C", …}
//
//	func (c *C) UnmarshalXML(dec *xml.Decoder, start xml.StartElement) error
//
// The method walks the child tokens of `start`, dispatching each on
// the snake-cased element local name back to the matching struct
// field. Polymorphic fields are routed through
// [canxml.DecodeAs] which consults [typereg.Default] for the
// `xsi:type` discriminator. Content the method does not read is handed
// to the helpers of openehr/internal/xmlunmatched, which apply the
// decode's policy for unmatched content: ignore it (the default), report
// it, or refuse it. The description tells the same runtime what the
// Reference Model requires of the class, so a decode that asks can learn
// which mandatory members the document left out (ADR 0033).
//
// Returns (nil, nil) when the file has no concrete classes.
//
// # Strategy
//
// For each concrete class C:
//
//  1. On entry, hand the start element and the class's description to
//     `xmlunmatched.Enter`. The description, a package-level
//     `xmlClassC` emitted once per class (never per generic
//     instantiation), names the members the class reads as XML
//     attributes, its presence-checked mandatory members in the Reference
//     Model's declaration order (an ancestor's before the class's own,
//     minus the four interval boundary flags and
//     RESOURCE_DESCRIPTION.parent_resource) and its multi-valued members.
//     Every attribute but the ones named, the XML Schema instance
//     attributes (`xsi:type` among them) and the namespace declarations
//     is unmatched; the runtime reports a mandatory member the element
//     leaves out, by its JSON Pointer, when the decode asks.
//
//  2. Initialise a flat switch on `xmlunmatched.LocalName` of the child
//     element covering every effective property (in any order: XML
//     decode is name-keyed, unlike encode which is order-pinned).
//     LocalName answers "" for an element in a foreign namespace, so
//     such an element matches no property.
//
//  3. Per property:
//     - Mandatory / optional primitive → `xmlunmatched.DecodeElement(dec, &t, &v.Field)`
//     - Mandatory struct → `dec.DecodeElement(&v.Field, &t)` (or `v.Field, ...` for pointer)
//     - Optional struct (pointer) → allocate, decode, assign
//     - Open generic parameter → `xmlunmatched.DecodeParam`, which chooses
//     by the instantiation: a primitive, a struct or an interface
//     - Class with no generated decoder (a BMM primitive class such as
//     TERMINOLOGY_CODE) → `xmlunmatched.SkipUnmatched`, one item
//     - Polymorphic single → `canxml.DecodeAs[Iface](dec, t)` + assign
//     - Slice of primitive/struct → append per occurrence
//     - Slice of polymorphic → append result of `canxml.DecodeAs[Iface]`
//
//  4. Every other element, a foreign one included, reaches the default
//     arm, `xmlunmatched.SkipUnmatched`, which skips it, reports it or refuses
//     it as the decode's options say.
//
//  5. Text between the child elements goes to
//     `xmlunmatched.CheckUnmatchedText`: the class's content is elements, so
//     text other than XML whitespace is unmatched, one item per element.
//
//  6. `xmi:type` is rejected via [canxml.XSITypeOf] (centralised in the
//     helper).
//
// # Hash/map and Any decoding
//
// Same limitation as the encoder: Hash<K,V> properties are not
// decoded, and neither is the content of a property typed by the BMM
// root type Any. Their element goes to `xmlunmatched.SkipUnmatched` as one
// item of unmatched content, so it is skipped by default, reported or
// refused on request, and the receiver field is left at its zero value.
// Documented in canxml/doc.go.
func RenderUnmarshalXMLFile(plan *Plan, file *PlannedFile) ([]byte, error) {
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
	body.WriteString("\t\"github.com/cadasto/openehr-sdk-go/openehr/internal/xmlunmatched\"\n")
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

	// The description of the class the method hands the runtime on entry.
	desc, err := xmlClassDescriptor(plan, pc, fields)
	if err != nil {
		return "", err
	}
	b.WriteString(renderXMLClassVar(desc))
	b.WriteString("\n")

	fmt.Fprintf(&b, "// UnmarshalXML decodes canonical openEHR XML into %s.\n", pc.GoName)
	b.WriteString("// Polymorphic fields are routed through canxml.DecodeAs so the\n")
	b.WriteString("// concrete type is selected by `xsi:type` at each polymorphic site.\n")
	b.WriteString("// Missing/unknown/type-mismatch dispatch failures wrap typereg\n")
	b.WriteString("// sentinels inside *canxml.DecodeError for errors.Is / errors.As.\n")
	b.WriteString("// Properties typed as XML attributes per the openEHR ITS-XML XSDs\n")
	b.WriteString("// (currently `archetype_node_id`) are read from _start.Attr.\n")
	b.WriteString("// Content the method does not read is unmatched: it is skipped, or\n")
	b.WriteString("// reported or refused under canxml.WithUnmatched and canxml.WithStrict.\n")
	legacy := sc.IsAbstract() && hasIntervalShape(fields)
	if legacy {
		b.WriteString("//\n")
		b.WriteString(legacyIntervalXMLDoc)
	}
	// A class that carries the four boundary flags records which of them and
	// of its two bounds the document carried, and gives the absent flags their
	// defaults at the closing tag, once every child has been read (REQ-169).
	flagged, err := intervalFlagged(fields)
	if err != nil {
		return "", fmt.Errorf("%s: %w", pc.BMMName, err)
	}
	if flagged {
		b.WriteString("//\n")
		b.WriteString(intervalFlagDecodeDoc())
		b.WriteString(intervalFlagXMLDoc)
	}
	fmt.Fprintf(&b, "func (%s *%s%s) UnmarshalXML(_dec *xml.Decoder, _start xml.StartElement) error {\n", recv, pc.GoName, typeArgs)
	// The runtime learns the class, and every attribute the class does not
	// read is unmatched content.
	b.WriteString(enterXMLClass(pc.GoName))
	// Read attribute-typed properties from _start.Attr.
	for _, ef := range attrFields {
		line := unmarshalXMLAttribute(recv, FieldName(ef.Prop.PropertyName()), ef.Prop.PropertyName())
		b.WriteString(line)
	}
	if flagged {
		b.WriteString("\tvar _seen intervalFlagsSeen\n")
	}
	b.WriteString("\tfor {\n")
	b.WriteString("\t\t_tok, _err := _dec.Token()\n")
	b.WriteString("\t\tif _err != nil {\n")
	b.WriteString("\t\t\treturn _err\n")
	b.WriteString("\t\t}\n")
	b.WriteString("\t\tswitch _t := _tok.(type) {\n")
	b.WriteString("\t\tcase xml.StartElement:\n")
	b.WriteString("\t\t\tswitch xmlunmatched.LocalName(_t.Name) {\n")

	for _, ef := range elemFields {
		var caseBody string
		if prop := ef.Prop.PropertyName(); flagged && isIntervalFlag(prop) {
			caseBody = unmarshalXMLIntervalFlag(recv, prop)
		} else {
			caseBody, err = renderUnmarshalXMLField(plan, recv, sc, ef)
			if err != nil {
				return "", fmt.Errorf("render UnmarshalXML field %s.%s: %w", pc.BMMName, prop, err)
			}
		}
		if prop := ef.Prop.PropertyName(); flagged && isIntervalBound(prop) {
			// The bound's presence decides its side's absent `*_unbounded`
			// (REQ-169).
			caseBody += unmarshalXMLIntervalBoundSeen(prop)
		}
		if legacy {
			caseBody = withLegacyElementName(caseBody, ef.Prop.PropertyName())
		}
		b.WriteString(caseBody)
	}

	b.WriteString("\t\t\tdefault:\n")
	b.WriteString("\t\t\t\tif _err := xmlunmatched.SkipUnmatched(_dec, _t); _err != nil {\n")
	b.WriteString("\t\t\t\t\treturn _err\n")
	b.WriteString("\t\t\t\t}\n")
	b.WriteString("\t\t\t}\n") // close inner switch (Name.Local)
	// The class's content is elements: text between them is unmatched.
	b.WriteString(checkUnmatchedXMLText)
	b.WriteString("\t\tcase xml.EndElement:\n")
	if flagged {
		fmt.Fprintf(&b, "\t\t\tif _err := %s; _err != nil {\n", intervalFlagSettle("_seen", recv))
		fmt.Fprintf(&b, "\t\t\t\treturn fmt.Errorf(\"%%w: %s: %%w\", canxml.ErrInvalidShape, _err)\n", pc.BMMName)
		b.WriteString("\t\t\t}\n")
	}
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
		if p.TypeName == bmmAnyType {
			// The codec cannot decode the content of an Any member: the
			// element goes to xmlunmatched.SkipUnmatched, like a Hash member's.
			return unmarshalXMLAnyContent(elemName), nil
		}
		ifaceName, kind := polymorphicProperty(plan, ef.Owner, emitting, p)
		if kind == polySingle {
			return unmarshalXMLPolySingle(recv, goField, elemName, ifaceName), nil
		}
		if kind == polySingleNarrow {
			// REQ-052: narrow-interface slot. canxml.DecodeAsOrDefault
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
		if isOpenParam(ef.Owner, p.TypeName) {
			// Typed by the owner's generic parameter (VERSION<T>.data):
			// what the element decodes into depends on the instantiation,
			// so xmlunmatched.DecodeParam chooses at run time, dispatching
			// on `xsi:type` when T is an interface.
			if p.IsMandatory {
				return unmarshalXMLOpenParam(recv, goField, elemName, p.TypeName), nil
			}
			return unmarshalXMLOpenParamOptional(recv, goField, elemName, p.TypeName), nil
		}
		if isClass {
			decoding, err := xmlClassDecoding(plan, p.TypeName)
			if err != nil {
				return "", err
			}
			switch decoding {
			case xmlNoDecoder:
				return unmarshalXMLNoDecoder(elemName, p.TypeName), nil
			case xmlTextDecoder:
				// An enumeration's Go type is a named string or integer,
				// which decodes from the element's text like a primitive.
				isClass = false
			case xmlOwnDecoder:
			}
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
		// emit a generic decode that chooses at run time, through
		// xmlunmatched.DecodeParam, so a primitive's attributes and child
		// elements count as unmatched content and an interface-typed
		// instantiation dispatches on `xsi:type`. The field is declared
		// as `T` (open param), not `*T`, mandatory or not: we cannot
		// allocate via new(T) for an unknown layout, so decode in place.
		return unmarshalXMLOpenParam(recv, goField, elemName, p.TypeName), nil

	case *bmm.ContainerProperty:
		ifaceName, kind := polymorphicProperty(plan, ef.Owner, emitting, p)
		if kind == polySlice || kind == polySliceNarrow {
			// REQ-052: narrow-element slices share the same RawMessage
			// + canxml.DecodeAs shape on the XML side. XML fixtures
			// uniformly carry xsi:type on slice items today; a
			// dedicated polySliceNarrow XML path can be added if a
			// fixture appears that omits the discriminator.
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
			decoding, err := xmlClassDecoding(plan, containerInnerName(p.TypeDef))
			if err != nil {
				return "", err
			}
			switch decoding {
			case xmlNoDecoder:
				return unmarshalXMLNoDecoder(elemName, containerInnerName(p.TypeDef)), nil
			case xmlTextDecoder:
				return unmarshalXMLPrimitiveSlice(recv, goField, elemName, innerType), nil
			case xmlOwnDecoder:
			}
			return unmarshalXMLStructSlice(recv, goField, elemName, innerType), nil
		}
		if isOpenParam(ef.Owner, containerInnerName(p.TypeDef)) {
			return unmarshalXMLOpenParamSlice(recv, goField, elemName, innerType), nil
		}
		if isOctetContainer(p.TypeDef) {
			// An Octet list is one xs:base64Binary element in the
			// ITS-XML schema (DV_MULTIMEDIA.data), as it is one base64
			// string in canonical JSON, not one element per byte.
			return unmarshalXMLBase64(recv, goField, elemName), nil
		}
		return unmarshalXMLPrimitiveSlice(recv, goField, elemName, innerType), nil

	case *bmm.GenericProperty:
		// Hash<K,V> at the property type level — Go maps it to
		// map[K]V. v1 defers XML emission/decoding (see canxml/doc.go).
		if p.TypeDef != nil && containerKinds[p.TypeDef.RootType] {
			return unmarshalXMLHashTODO(elemName), nil
		}
		if p.TypeDef != nil {
			decoding, err := xmlClassDecoding(plan, p.TypeDef.RootType)
			if err != nil {
				return "", err
			}
			if decoding != xmlOwnDecoder {
				return unmarshalXMLNoDecoder(elemName, p.TypeDef.RootType), nil
			}
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

// The primitive helpers decode through xmlunmatched.DecodeElement rather
// than the decoder's own DecodeElement: encoding/xml reads a primitive from
// its text and ignores any attribute or child element on it, and the helper
// hands those to the decode's policy for unmatched content.

func unmarshalXMLPrimitiveMandatory(recv, field, elem string) string {
	return fmt.Sprintf(
		"\t\t\tcase %q:\n\t\t\t\tif _err := xmlunmatched.DecodeElement(_dec, &_t, &%s.%s); _err != nil {\n\t\t\t\t\treturn _err\n\t\t\t\t}\n",
		elem, recv, field)
}

func unmarshalXMLPrimitiveOptionalLit(recv, field, elem, goType string) string {
	return fmt.Sprintf(
		"\t\t\tcase %q:\n\t\t\t\tvar _v %s\n\t\t\t\tif _err := xmlunmatched.DecodeElement(_dec, &_t, &_v); _err != nil {\n\t\t\t\t\treturn _err\n\t\t\t\t}\n\t\t\t\t%s.%s = &_v\n",
		elem, goType, recv, field)
}

// unmarshalXMLOpenParam emits the decode case of a field typed by the open
// generic parameter param that is not polymorphic by its bound. The
// parameter may be bound to a primitive (BASE Interval<Integer>), to a
// class, or to an interface (ORIGINAL_VERSION<any>), so the case decodes in
// place through xmlunmatched.DecodeParam, which chooses by the
// instantiation and dispatches an interface through canxml.DecodeAs.
func unmarshalXMLOpenParam(recv, field, elem, param string) string {
	return fmt.Sprintf(
		"\t\t\tcase %q:\n\t\t\t\tif _err := xmlunmatched.DecodeParam(_dec, &_t, &%s.%s, canxml.DecodeAs[%s]); _err != nil {\n\t\t\t\t\treturn _err\n\t\t\t\t}\n",
		elem, recv, field, param)
}

// unmarshalXMLOpenParamOptional emits the decode case of an optional field
// typed by an open generic parameter, held as a pointer: it allocates the
// value, decodes it through xmlunmatched.DecodeParam and assigns it.
func unmarshalXMLOpenParamOptional(recv, field, elem, param string) string {
	return fmt.Sprintf(
		"\t\t\tcase %q:\n\t\t\t\t_v := new(%s)\n\t\t\t\tif _err := xmlunmatched.DecodeParam(_dec, &_t, _v, canxml.DecodeAs[%s]); _err != nil {\n\t\t\t\t\treturn _err\n\t\t\t\t}\n\t\t\t\t%s.%s = _v\n",
		elem, param, param, recv, field)
}

// unmarshalXMLOpenParamSlice emits the decode case of a container whose
// items are typed by an open generic parameter: each item decodes through
// xmlunmatched.DecodeParam and is appended.
func unmarshalXMLOpenParamSlice(recv, field, elem, param string) string {
	return fmt.Sprintf(
		"\t\t\tcase %q:\n\t\t\t\tvar _v %s\n\t\t\t\tif _err := xmlunmatched.DecodeParam(_dec, &_t, &_v, canxml.DecodeAs[%s]); _err != nil {\n\t\t\t\t\treturn _err\n\t\t\t\t}\n\t\t\t\t%s.%s = append(%s.%s, _v)\n",
		elem, param, param, recv, field, recv, field)
}

// unmarshalXMLBase64 emits the decode case of an Octet list, held as a Go
// []byte: one element whose text is xs:base64Binary, decoded through
// xmlunmatched.DecodeBase64.
func unmarshalXMLBase64(recv, field, elem string) string {
	return fmt.Sprintf(
		"\t\t\tcase %q:\n\t\t\t\tif _err := xmlunmatched.DecodeBase64(_dec, &_t, &%s.%s); _err != nil {\n\t\t\t\t\treturn _err\n\t\t\t\t}\n",
		elem, recv, field)
}

// isOctetContainer reports whether td is a container of the BMM primitive
// Octet, which the generator holds as a Go []byte.
func isOctetContainer(td *bmm.ContainerType) bool {
	return containerInnerName(td) == bmmOctetType
}

// bmmOctetType is the BMM name of the byte primitive.
const bmmOctetType = "Octet"

// handWrittenXMLCodecs names the BMM classes whose canonical-XML codec is
// written by hand in the target package rather than generated, because the
// generator emits no codec for their class. TERMINOLOGY_CODE is a BASE
// primitive class, held by value in AOM 1.4 ARCHETYPE.original_language; its
// methods live in openehr/rm/terminology_code_xml.go.
var handWrittenXMLCodecs = map[string]bool{
	"Terminology_code": true,
}

// unmarshalXMLNoDecoder emits the case of a property whose class has no
// canonical-XML decoder of its own, such as a BMM primitive class like
// TERMINOLOGY_CODE: encoding/xml would read nothing from it and report
// nothing either. The element is handed to xmlunmatched.SkipUnmatched as one
// item, like a Hash member's: skipped by default, reported or refused on
// request, and the field is left at its zero value.
func unmarshalXMLNoDecoder(elem, bmmType string) string {
	return fmt.Sprintf(
		"\t\t\tcase %q:\n\t\t\t\t// Canonical XML has no decoder for %s — see canxml/doc.go.\n\t\t\t\tif _err := xmlunmatched.SkipUnmatched(_dec, _t); _err != nil {\n\t\t\t\t\treturn _err\n\t\t\t\t}\n",
		elem, bmmType)
}

// xmlDecoding says how the Go type of a BMM class decodes from canonical XML.
type xmlDecoding int

const (
	// xmlNoDecoder: nothing reads the element, such as a BMM primitive
	// class like TERMINOLOGY_CODE, held as a struct with no XML decoder.
	xmlNoDecoder xmlDecoding = iota
	// xmlOwnDecoder: the type has a generated UnmarshalXML, which reads its
	// own content and hands what it does not read to the unmatched-content
	// helpers.
	xmlOwnDecoder
	// xmlTextDecoder: the type is a named string or integer (an
	// enumeration) and decodes from the element's text like a primitive.
	xmlTextDecoder
)

// xmlClassDecoding reports how the Go type of the BMM class name decodes.
// A class has a generated UnmarshalXML by the rule [codecClassesIn] applies:
// a concrete class that is not a BMM primitive class, or the abstract BASE
// Interval, which the base schema lists among its primitive types. A class
// another target owns counts by the same rule, since that target renders its
// decoder. A class whose codec is written by hand ([handWrittenXMLCodecs])
// has one too. A class the plan does not hold is rendered as `any`, which
// nothing decodes.
func xmlClassDecoding(plan *Plan, name string) (xmlDecoding, error) {
	if handWrittenXMLCodecs[name] {
		return xmlOwnDecoder, nil
	}
	pc, ok := plan.Classes[name]
	if !ok {
		return xmlNoDecoder, nil
	}
	switch cls := pc.Class.(type) {
	case *bmm.Enumeration:
		return xmlTextDecoder, nil
	case *bmm.SimpleClass:
		if cls.IsAbstract() {
			if !cls.IsGeneric() || codecPolymorphicAbstractGeneric(plan, pc) {
				return xmlNoDecoder, nil
			}
			shaped, err := intervalShaped(plan, pc)
			if err != nil || !shaped {
				return xmlNoDecoder, err
			}
			return xmlOwnDecoder, nil
		}
		if pc.IsPrimitive {
			return xmlNoDecoder, nil
		}
		return xmlOwnDecoder, nil
	}
	return xmlNoDecoder, nil
}

// isOpenParam reports whether name is one of owner's generic parameters.
func isOpenParam(owner *bmm.SimpleClass, name string) bool {
	if owner == nil || owner.GenericParameterDefs == nil {
		return false
	}
	_, ok := owner.GenericParameterDefs[name]
	return ok
}

// containerInnerName returns the BMM name of a container's item type: the
// class name, or the root type of a generic class.
func containerInnerName(td *bmm.ContainerType) string {
	if td == nil || td.TypeDef == nil {
		return ""
	}
	switch inner := td.TypeDef.(type) {
	case *bmm.SimpleType:
		return inner.TypeName
	case *bmm.GenericType:
		return inner.RootType
	}
	return ""
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

// unmarshalXMLPolySingleNarrow emits the REQ-052 narrow-interface
// decode case: canxml.DecodeAsOrDefault dispatches via xsi:type when
// present and otherwise instantiates the parent concrete type
// (`parentGo`) — preserving openEHR canonical XML fixtures that omit
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
		"\t\t\tcase %q:\n\t\t\t\tvar _v %s\n\t\t\t\tif _err := xmlunmatched.DecodeElement(_dec, &_t, &_v); _err != nil {\n\t\t\t\t\treturn _err\n\t\t\t\t}\n\t\t\t\t%s.%s = append(%s.%s, _v)\n",
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

// enterXMLClass emits the call that hands the start element and the class's
// description to xmlunmatched.Enter at the head of UnmarshalXML.
func enterXMLClass(goName string) string {
	return fmt.Sprintf(
		"\tif _err := xmlunmatched.Enter(_dec, _start, &xmlClass%s); _err != nil {\n\t\treturn _err\n\t}\n",
		goName)
}

// maxXMLRequired is the most presence-checked members a class may have: the
// runtime keeps which of them an element has a value for in one 64-bit word.
const maxXMLRequired = 64

// xmlClassDesc is the description of a class its generated decoder hands
// xmlunmatched.Enter, rendered by [renderXMLClassVar] as a package-level var
// named `xmlClass<GoName>`: the class's BMM name, the members the decoder
// reads as XML attributes, the presence-checked mandatory members in the
// Reference Model's declaration order, and the multi-valued members.
type xmlClassDesc struct {
	goName   string
	name     string
	attrs    []string
	required []string
	items    []string
}

// xmlClassDescriptor builds the description of pc from its effective fields.
// Required is the mandatory fields ([xmlPresenceChecked]) in the order
// [effectiveProperties] gives, the Reference Model's declaration order with
// an ancestor's members before the class's own, which rminfo's table shares;
// the decoder's own field order puts the nearest ancestor first and is not
// used. A class with more presence-checked members than the runtime holds
// ([maxXMLRequired]) is refused.
func xmlClassDescriptor(plan *Plan, pc *PlannedClass, fields []emittedField) (xmlClassDesc, error) {
	d := xmlClassDesc{goName: pc.GoName, name: pc.BMMName}
	for _, ef := range fields {
		prop := ef.Prop.PropertyName()
		if isXMLAttributeProperty(ef.Prop) {
			d.attrs = append(d.attrs, prop)
		}
		if _, ok := ef.Prop.(*bmm.ContainerProperty); ok {
			d.items = append(d.items, prop)
		}
	}
	_, order := effectiveProperties(plan, pc.BMMName)
	required, err := xmlRequiredMembers(pc.BMMName, fields, order)
	if err != nil {
		return xmlClassDesc{}, err
	}
	d.required = required
	return d, nil
}

// xmlRequiredMembers returns the presence-checked members among fields, the
// effective fields of the class named name, in the order of order, the
// Reference Model's declaration order of the class's attributes. A field
// order does not place, and more members than [maxXMLRequired], are errors.
func xmlRequiredMembers(name string, fields []emittedField, order []string) ([]string, error) {
	checked := make(map[string]bool, len(fields))
	for _, ef := range fields {
		if xmlPresenceChecked(ef) {
			checked[ef.Prop.PropertyName()] = true
		}
	}
	required := make([]string, 0, len(checked))
	for _, prop := range order {
		if checked[prop] {
			required = append(required, prop)
			delete(checked, prop)
		}
	}
	if len(checked) > 0 {
		return nil, fmt.Errorf("%s: mandatory members %v are not in the class's declaration order", name, slices.Sorted(maps.Keys(checked)))
	}
	if len(required) > maxXMLRequired {
		return nil, fmt.Errorf("%s has %d presence-checked members; the canonical-XML runtime holds at most %d", name, len(required), maxXMLRequired)
	}
	return required, nil
}

// xmlPresenceChecked reports whether the runtime checks that a document
// carries ef: the Reference Model makes it mandatory, by the rule rminfo's
// table applies (a single or generic property by its flag, a container by
// its flag or a lower bound of one), and it is neither one of the four
// interval boundary flags, which the decoder gives a value when absent
// (REQ-169), nor RESOURCE_DESCRIPTION.parent_resource, which no value can
// be held in (rminfo.IsNonStorableAttr).
func xmlPresenceChecked(ef emittedField) bool {
	prop := ef.Prop.PropertyName()
	if isIntervalFlag(prop) || (ef.OwnerName == "RESOURCE_DESCRIPTION" && prop == "parent_resource") {
		return false
	}
	switch p := ef.Prop.(type) {
	case *bmm.SingleProperty:
		return p.IsMandatory
	case *bmm.SinglePropertyOpen:
		return p.IsMandatory
	case *bmm.GenericProperty:
		return p.IsMandatory
	case *bmm.ContainerProperty:
		return containerRequired(p.Cardinality, p.IsMandatory)
	}
	return false
}

// renderXMLClassVar renders d as the package-level var the decoder hands
// xmlunmatched.Enter, one field per line, leaving out an empty list.
func renderXMLClassVar(d xmlClassDesc) string {
	var b strings.Builder
	fmt.Fprintf(&b, "// xmlClass%s describes %s to the canonical-XML decode runtime.\n", d.goName, d.name)
	fmt.Fprintf(&b, "var xmlClass%s = xmlunmatched.Class{\n", d.goName)
	fmt.Fprintf(&b, "\tName: %q,\n", d.name)
	for _, f := range []struct {
		name string
		vals []string
	}{{"Attrs", d.attrs}, {"Required", d.required}, {"Items", d.items}} {
		if len(f.vals) == 0 {
			continue
		}
		fmt.Fprintf(&b, "\t%s: %s,\n", f.name, stringSliceLiteral(f.vals))
	}
	b.WriteString("}\n")
	return b.String()
}

// stringSliceLiteral renders vals as a Go `[]string{…}` literal.
func stringSliceLiteral(vals []string) string {
	var b strings.Builder
	b.WriteString("[]string{")
	for i, v := range vals {
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "%q", v)
	}
	b.WriteByte('}')
	return b.String()
}

// checkUnmatchedXMLText is the token-loop arm that hands the text read
// directly inside the element to xmlunmatched.CheckUnmatchedText.
const checkUnmatchedXMLText = "\t\tcase xml.CharData:\n" +
	"\t\t\tif _err := xmlunmatched.CheckUnmatchedText(_dec, _t); _err != nil {\n" +
	"\t\t\t\treturn _err\n" +
	"\t\t\t}\n"

// bmmAnyType is the BMM name of the root type, which the generator maps to
// the Go empty interface.
const bmmAnyType = "Any"

// unmarshalXMLAnyContent emits the case of a property typed by the BMM root
// type Any, such as the AOM 1.4 `assumed_value` and `EXPR_LEAF.item`. The
// codec cannot decode its content, so the element is handed to
// xmlunmatched.SkipUnmatched as one item, like a Hash member's: skipped by
// default, reported or refused on request, and the field is left at its
// zero value.
func unmarshalXMLAnyContent(elem string) string {
	return fmt.Sprintf(
		"\t\t\tcase %q:\n\t\t\t\t// Canonical XML does not model the content of an Any member — see canxml/doc.go.\n\t\t\t\tif _err := xmlunmatched.SkipUnmatched(_dec, _t); _err != nil {\n\t\t\t\t\treturn _err\n\t\t\t\t}\n",
		elem)
}

// unmarshalXMLHashTODO emits the case of a Hash<K,V> property. Canonical XML
// does not model Hash, so the element is handed to xmlunmatched.SkipUnmatched
// like unmatched content: skipped by default, reported or refused on request.
func unmarshalXMLHashTODO(elem string) string {
	return fmt.Sprintf(
		"\t\t\tcase %q:\n\t\t\t\t// TODO(canxml): Hash<K,V> XML decode deferred — see canxml/doc.go.\n\t\t\t\tif _err := xmlunmatched.SkipUnmatched(_dec, _t); _err != nil {\n\t\t\t\t\treturn _err\n\t\t\t\t}\n",
		elem)
}
