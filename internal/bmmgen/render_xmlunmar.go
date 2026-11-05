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
// which mandatory members the document left out.
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
//     A single-valued member's case first checks one bit of `_got`, which
//     the method keeps for its single-valued members ([xmlSingleValued]):
//     a second child element of the member goes to
//     `xmlunmatched.SkipUnmatched`, so the method keeps the first child's
//     value and the repeat is unmatched content ([withRepeatGuard]).
//     An optional member whose field can hold nil then hands a nilled
//     element to `xmlunmatched.SkipNil` and leaves the member absent
//     ([withNilGuard]); an optional primitive decodes through
//     `xmlunmatched.DecodeOptional`, which leaves it absent when its element
//     has no text.
//
//  4. A member whose element the target's root schemas give a choice of
//     shapes, in their schema_forms, goes to `xmlunmatched.DecodeChoice`
//     with the list of its shapes, a package-level `xmlChoice<GoName><Field>`
//     emitted beside the class's description: the element's first child
//     chooses the shape ([xmlChoiceOf]).
//
//  5. An element the class's schema accepts that no member holds, which the
//     target's root schemas name in their dropped_elements for the class or
//     a class it inherits from, goes to `xmlunmatched.DropElement`, which
//     drops it when its text is one of the texts the entry gives and refuses any
//     other text. Every other element, a foreign one included, reaches the
//     default arm, `xmlunmatched.SkipUnmatched`, which skips it, reports it or
//     refuses it as the decode's options say.
//
//  6. Text between the child elements goes to
//     `xmlunmatched.CheckUnmatchedText`: the class's content is elements, so
//     text other than XML whitespace is unmatched, one item per element.
//
//  7. `xmi:type` is rejected via [canxml.XSITypeOf] (centralised in the
//     helper).
//
// # Hash and Any decoding
//
// A Hash<K,V> member reads each element of its name as one entry, through
// `xmlunmatched.DecodeEntry`, which takes the key attribute off before the
// value is read, or `xmlunmatched.DecodeEntryKeyedBy`, which reads the key
// from the value, its language code ([xmlHashOf]). The method keeps one mark
// per Hash member, false as the element opens, which it hands the runtime
// with each entry, so the member's map starts empty at the element's first
// entry of it in every decode ([xmlHashMarks]). A Hash the codec does not
// model, a Hash of Hashes, is not decoded: its element goes to
// `xmlunmatched.SkipUnmatched` as one item of unmatched content, so it is
// skipped by default, reported or refused on request, and the receiver field
// is left at its zero value. Documented in canxml/doc.go. A property typed by
// the BMM root type Any decodes through `xmlschema.DecodeAny`, as the class
// its `xsi:type` names, as the text of an XML Schema built-in datatype kept
// with its `xsi:type`, or as its text.
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
	if slices.ContainsFunc(chunks, func(c string) bool { return strings.Contains(c, "xmlschema.") }) {
		fmt.Fprintf(&body, "\t%q\n", xmlschemaImportPath)
	}
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

	// The members the class's ITS-XML schema type declares as XML
	// attributes, which the description names, are read from _start.Attr
	// before the token loop; every member, those included, has its case in
	// the switch. The child-element case of an attribute member keeps the
	// decoder tolerant of producers that write it as a child element (legacy
	// fixtures, the SDK's own pre-fix output): the attribute is read first,
	// and a child element that appears overwrites it.
	elemFields := fields

	// The description of the class the method hands the runtime on entry.
	desc, err := xmlClassDescriptor(plan, pc, fields)
	if err != nil {
		return "", err
	}
	// The elements the class's schema accepts that no member holds, which the
	// target's roots name for the class or a class it inherits from.
	dropped, err := droppedElementsOf(plan, pc, fields)
	if err != nil {
		return "", err
	}
	b.WriteString(renderXMLClassVar(desc))
	b.WriteString("\n")
	// The members whose element the target's roots give a choice of shapes,
	// each with the list of its shapes beside the description.
	choices := map[string]*xmlChoice{}
	for _, ef := range fields {
		c, err := xmlChoiceOf(plan, pc, ef)
		if err != nil {
			return "", err
		}
		if c != nil {
			choices[ef.Prop.PropertyName()] = c
			b.WriteString(renderXMLChoiceVar(c))
			b.WriteString("\n")
		}
	}

	fmt.Fprintf(&b, "// UnmarshalXML decodes canonical openEHR XML into %s.\n", pc.GoName)
	b.WriteString("// Polymorphic fields are routed through canxml.DecodeAs so the\n")
	b.WriteString("// concrete type is selected by `xsi:type` at each polymorphic site.\n")
	b.WriteString("// Missing/unknown/type-mismatch dispatch failures wrap typereg\n")
	b.WriteString("// sentinels inside *canxml.DecodeError for errors.Is / errors.As.\n")
	b.WriteString("// Members the class's ITS-XML schema type declares as XML attributes\n")
	b.WriteString("// are read from _start.Attr.\n")
	legacy := sc.IsAbstract() && hasIntervalShape(fields)
	// A class that carries the four boundary flags records which of them and
	// of its two bounds the document carried, and gives the absent flags their
	// defaults at the closing tag, once every child has been read (REQ-169).
	flagged, err := intervalFlagged(fields)
	if err != nil {
		return "", fmt.Errorf("%s: %w", pc.BMMName, err)
	}
	marks, err := xmlHashMarks(plan, fields)
	if err != nil {
		return "", err
	}
	once, err := xmlSingleValuedMembers(plan, pc, fields)
	if err != nil {
		return "", err
	}
	// The cases come before the method's head, which declares the marks of
	// the list members they append to.
	cases, lists, err := renderUnmarshalXMLCases(plan, pc, sc, recv, elemFields, xmlCaseContext{choices: choices, once: once, flagged: flagged, legacy: legacy})
	if err != nil {
		return "", err
	}
	if len(marks) > 0 {
		b.WriteString("// A Hash member reads each element of its name as one entry, into a\n")
		b.WriteString("// map that starts empty at the first entry.\n")
	}
	if len(lists) > 0 {
		b.WriteString("// A list member reads each element of its name as one item, into a\n")
		b.WriteString("// list that starts empty at the first item.\n")
	}
	if len(once) > 0 {
		b.WriteString("// A single-valued member keeps its first child element; a second one is\n")
		b.WriteString("// unmatched content.\n")
	}
	b.WriteString("// Content the method does not read is unmatched: it is skipped, or\n")
	b.WriteString("// reported or refused under canxml.WithUnmatched and canxml.WithStrict.\n")
	if legacy {
		b.WriteString("//\n")
		b.WriteString(legacyIntervalXMLDoc)
	}
	if flagged {
		b.WriteString("//\n")
		b.WriteString(intervalFlagDecodeDoc())
		b.WriteString(intervalFlagXMLDoc)
	}
	fmt.Fprintf(&b, "func (%s *%s%s) UnmarshalXML(_dec *xml.Decoder, _start xml.StartElement) error {\n", recv, pc.GoName, typeArgs)
	// The runtime learns the class, and every attribute the class does not
	// read is unmatched content.
	b.WriteString(enterXMLClass(recv, pc.GoName, typeArgs))
	// Read the attribute members from _start.Attr.
	for _, name := range desc.attrs {
		b.WriteString(unmarshalXMLAttribute(recv, FieldName(name), name))
	}
	if len(once) > 0 {
		b.WriteString("\t// A bit per single-valued member that has had a child element.\n")
		b.WriteString("\tvar _got uint64\n")
	}
	if len(lists) > 0 {
		b.WriteString("\t// Whether the element has had an item of each list member.\n")
		fmt.Fprintf(&b, "\tvar %s bool\n", strings.Join(lists, ", "))
	}
	if len(marks) > 0 {
		b.WriteString("\t// Whether the element has had an entry of each Hash member.\n")
		fmt.Fprintf(&b, "\tvar %s bool\n", strings.Join(marks, ", "))
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

	b.WriteString(cases)
	for _, d := range dropped {
		b.WriteString(unmarshalXMLDroppedElement(d.element, d.texts))
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

// xmlCaseContext is what the cases of a class's UnmarshalXML read beside the
// member itself ([renderUnmarshalXMLCases]).
type xmlCaseContext struct {
	choices map[string]*xmlChoice // the members whose element takes a choice of shapes
	once    map[string]int        // the bit of each single-valued member
	flagged bool                  // the class carries the four interval boundary flags
	legacy  bool                  // the class is the abstract legacy interval
}

// renderUnmarshalXMLCases returns the case of each member in fields, in
// order, for the switch on a child element's local name in the UnmarshalXML
// of pc, and the names of the marks the cases of the list members read
// ([withListReset]), which the method declares.
func renderUnmarshalXMLCases(plan *Plan, pc *PlannedClass, sc *bmm.SimpleClass, recv string, fields []emittedField, c xmlCaseContext) (string, []string, error) {
	var (
		b     strings.Builder
		lists []string
		err   error
	)
	for _, ef := range fields {
		var caseBody string
		if prop := ef.Prop.PropertyName(); c.flagged && isIntervalFlag(prop) {
			caseBody = unmarshalXMLIntervalFlag(recv, prop)
		} else if c := c.choices[prop]; c != nil {
			caseBody = unmarshalXMLChoice(recv, FieldName(prop), prop, c, isMandatoryProperty(ef.Prop))
		} else {
			caseBody, err = renderUnmarshalXMLField(plan, recv, sc, ef)
			if err != nil {
				return "", nil, fmt.Errorf("render UnmarshalXML field %s.%s: %w", pc.BMMName, prop, err)
			}
		}
		if prop := ef.Prop.PropertyName(); c.flagged && isIntervalBound(prop) {
			// The bound's presence decides its side's absent `*_unbounded`
			// (REQ-169).
			caseBody += unmarshalXMLIntervalBoundSeen(prop)
		}
		nilable, err := xmlNilable(plan, ef)
		if err != nil {
			return "", nil, fmt.Errorf("%s.%s: %w", pc.BMMName, ef.Prop.PropertyName(), err)
		}
		if nilable {
			caseBody, err = withNilGuard(caseBody, recv, FieldName(ef.Prop.PropertyName()), isMandatoryMember(ef.Prop))
			if err != nil {
				return "", nil, fmt.Errorf("render UnmarshalXML field %s.%s: %w", pc.BMMName, ef.Prop.PropertyName(), err)
			}
		}
		if bit, ok := c.once[ef.Prop.PropertyName()]; ok {
			caseBody, err = withRepeatGuard(caseBody, bit)
			if err != nil {
				return "", nil, fmt.Errorf("render UnmarshalXML field %s.%s: %w", pc.BMMName, ef.Prop.PropertyName(), err)
			}
		}
		var listed bool
		if caseBody, listed = withListReset(caseBody, recv, FieldName(ef.Prop.PropertyName())); listed {
			lists = append(lists, xmlListMark(FieldName(ef.Prop.PropertyName())))
		}
		if c.legacy {
			caseBody = withLegacyElementName(caseBody, ef.Prop.PropertyName())
		}
		b.WriteString(caseBody)
	}
	return b.String(), lists, nil
}

// withListReset inserts into caseBody, the case of a member held in recv's
// Go field field, the lines that start the member's list empty at the
// element's first item of it, before the item is appended, and reports
// whether caseBody appends an item: the list then holds the items of this
// element alone, whatever the value held before, as canonical JSON replaces
// an array. A case that appends no item, a single-valued member's or one
// that skips the member's elements, comes back unchanged. A list held as
// *[]T is set to nil, and the case allocates it again.
func withListReset(caseBody, recv, field string) (string, bool) {
	at := strings.Index(caseBody, fmt.Sprintf("\t\t\t\t%s.%s = append(%s.%s, _v)\n", recv, field, recv, field))
	if at < 0 {
		at = strings.Index(caseBody, fmt.Sprintf("\t\t\t\tif %s.%s == nil {\n\t\t\t\t\t%s.%s = new([]", recv, field, recv, field))
	}
	if at < 0 {
		return caseBody, false
	}
	mark := xmlListMark(field)
	reset := fmt.Sprintf("\t\t\t\tif !%s {\n\t\t\t\t\t%s = true\n\t\t\t\t\t%s.%s = nil\n\t\t\t\t}\n", mark, mark, recv, field)
	return caseBody[:at] + reset + caseBody[at:], true
}

// xmlListMark returns the name of the local variable that marks the list
// member held in the Go field field ([withListReset]). A member is a list or
// a Hash, never both, so it shares the prefix of [xmlHashMark].
func xmlListMark(field string) string {
	return "_begun" + field
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

	// A Hash member reads one entry per element; one the codec does not model
	// is unmatched content.
	hash, isHash, err := xmlHashOf(plan, ef)
	if err != nil {
		return "", err
	}
	if hash != nil {
		return unmarshalXMLHash(recv, goField, elemName, hash), nil
	}
	if isHash {
		return unmarshalXMLHashUnmodelled(elemName), nil
	}

	switch p := ef.Prop.(type) {
	case *bmm.SingleProperty:
		if p.TypeName == bmmAnyType {
			// An Any member decodes as the type its discriminator names, or
			// as its text: xmlschema.DecodeAny.
			return unmarshalXMLAny(recv, goField, elemName, p.IsMandatory), nil
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
		if !isPrimitive(p.TypeName) {
			// An enumeration is a class: an empty element is no value of it,
			// and its decode refuses it.
			return unmarshalXMLEnumerationOptionalLit(recv, goField, elemName, goType), nil
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
		// List<T>, Set<T> or Array<T> at the property type level — Go
		// holds it as []T, or *[]T when optional: one element per item.
		if p.TypeDef != nil && containerKinds[p.TypeDef.RootType] {
			return unmarshalXMLGenericList(plan, recv, goField, elemName, ef.Owner, p)
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

// unmarshalXMLPrimitiveMandatory emits the case of a mandatory member of a
// primitive type, through xmlunmatched.DecodeMandatory, which gives a string
// element of whitespace only the empty string, as an absent one has
// (REQ-175).
func unmarshalXMLPrimitiveMandatory(recv, field, elem string) string {
	return fmt.Sprintf(
		"\t\t\tcase %q:\n\t\t\t\tif _err := xmlunmatched.DecodeMandatory(_dec, &_t, &%s.%s); _err != nil {\n\t\t\t\t\treturn _err\n\t\t\t\t}\n",
		elem, recv, field)
}

// unmarshalXMLEnumerationOptionalLit emits the case of an optional member
// typed by an enumeration, which decodes from the element's text like a
// primitive, through xmlunmatched.DecodeElement.
func unmarshalXMLEnumerationOptionalLit(recv, field, elem, goType string) string {
	return fmt.Sprintf(
		"\t\t\tcase %q:\n\t\t\t\tvar _v %s\n\t\t\t\tif _err := xmlunmatched.DecodeElement(_dec, &_t, &_v); _err != nil {\n\t\t\t\t\treturn _err\n\t\t\t\t}\n\t\t\t\t%s.%s = &_v\n",
		elem, goType, recv, field)
}

// unmarshalXMLPrimitiveOptionalLit emits the case of an optional member of a
// primitive type, through xmlunmatched.DecodeOptional, which leaves the member
// absent when its element has no text, or whitespace only for a string
// (REQ-056).
func unmarshalXMLPrimitiveOptionalLit(recv, field, elem, goType string) string {
	return fmt.Sprintf(
		"\t\t\tcase %q:\n\t\t\t\tvar _v %s\n\t\t\t\tif _ok, _err := xmlunmatched.DecodeOptional(_dec, &_t, &_v); _err != nil {\n\t\t\t\t\treturn _err\n\t\t\t\t} else if _ok {\n\t\t\t\t\t%s.%s = &_v\n\t\t\t\t} else {\n\t\t\t\t\t%s.%s = nil\n\t\t\t\t}\n",
		elem, goType, recv, field, recv, field)
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
// primitive class, which the pinned AOM 1.4 schema holds by value in
// ARCHETYPE.original_language; its methods live in
// openehr/rm/terminology_code_xml.go. The AOM 1.4 root gives
// AUTHORED_RESOURCE its RM form, whose original_language is a CODE_PHRASE.
var handWrittenXMLCodecs = map[string]bool{
	"Terminology_code": true,
}

// unmarshalXMLNoDecoder emits the case of a property whose class has no
// canonical-XML decoder of its own, such as a BMM primitive class like
// TERMINOLOGY_CODE: encoding/xml would read nothing from it and report
// nothing either. The element is handed to xmlunmatched.SkipUnmodelled as one
// item: skipped by default, reported or refused on request, and the field is
// left at its zero value, while the member counts as present.
func unmarshalXMLNoDecoder(elem, bmmType string) string {
	return fmt.Sprintf(
		"\t\t\tcase %q:\n\t\t\t\t// Canonical XML has no decoder for %s — see canxml/doc.go.\n\t\t\t\tif _err := xmlunmatched.SkipUnmodelled(_dec, _t); _err != nil {\n\t\t\t\t\treturn _err\n\t\t\t\t}\n",
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

// unmarshalXMLGenericList emits the decode case of a member typed by a
// generic List, Set or Array of one item type, held as []T, or as *[]T when
// optional, which the case allocates on the first item. Each element is one
// item, decoded as a container property's item of that type is and
// appended. An item the case cannot decode alone, an interface, a generic
// parameter or an Octet, is refused at generation: no such member exists,
// and a container property's item of those kinds needs `xsi:type`
// dispatch or a base64 element.
func unmarshalXMLGenericList(plan *Plan, recv, field, elem string, owner *bmm.SimpleClass, p *bmm.GenericProperty) (string, error) {
	params := p.TypeDef.GenericParameters
	if len(params) != 1 {
		return "", fmt.Errorf("generic %s with %d parameters; want one item type", p.TypeDef.RootType, len(params))
	}
	itemName := containerInnerName(&bmm.ContainerType{ContainerType: p.TypeDef.RootType, TypeDef: params[0]})
	if itemName == bmmOctetType || isOpenParam(owner, itemName) || isInterfaceTypeRef(plan, itemName) {
		return "", fmt.Errorf("generic %s<%s>: canonical XML decodes no such item alone", p.TypeDef.RootType, itemName)
	}
	itemType, err := typeRef(plan, owner, params[0])
	if err != nil {
		return "", err
	}
	decode := "xmlunmatched.DecodeElement(_dec, &_t, &_v)"
	if !isPrimitive(itemName) {
		decoding, err := xmlClassDecoding(plan, itemName)
		if err != nil {
			return "", err
		}
		switch decoding {
		case xmlNoDecoder:
			return unmarshalXMLNoDecoder(elem, itemName), nil
		case xmlOwnDecoder:
			decode = "_dec.DecodeElement(&_v, &_t)"
		case xmlTextDecoder:
		}
	}
	appendItem := fmt.Sprintf("\t\t\t\t%s.%s = append(%s.%s, _v)\n", recv, field, recv, field)
	if !p.IsMandatory {
		appendItem = fmt.Sprintf("\t\t\t\tif %s.%s == nil {\n\t\t\t\t\t%s.%s = new([]%s)\n\t\t\t\t}\n\t\t\t\t*%s.%s = append(*%s.%s, _v)\n",
			recv, field, recv, field, itemType, recv, field, recv, field)
	}
	return fmt.Sprintf(
		"\t\t\tcase %q:\n\t\t\t\tvar _v %s\n\t\t\t\tif _err := %s; _err != nil {\n\t\t\t\t\treturn _err\n\t\t\t\t}\n%s",
		elem, itemType, decode, appendItem), nil
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
// omit it, so absence leaves the zero value rather than failing; the
// description's Required list lets a decode that asks report it.
func unmarshalXMLAttribute(recv, goField, attrName string) string {
	return fmt.Sprintf(
		"\tfor _, _a := range _start.Attr {\n\t\tif _a.Name.Local == %q && _a.Name.Space == \"\" {\n\t\t\t%s.%s = _a.Value\n\t\t\tbreak\n\t\t}\n\t}\n",
		attrName, recv, goField)
}

// droppedXMLElement is an element a class's decoder reads and drops, with the
// texts it may hold.
type droppedXMLElement struct {
	element string
	texts   []string
}

// droppedElementsOf returns, in element order, the elements the decoder of pc
// reads and drops: those the target's roots name, in their dropped_elements,
// for pc or for a class pc inherits from. An element that names one of fields,
// pc's members, is an error, since the member reads it, and so is an element
// two of those classes drop with different texts.
func droppedElementsOf(plan *Plan, pc *PlannedClass, fields []emittedField) ([]droppedXMLElement, error) {
	if len(plan.extensions.droppedElements) == 0 {
		return nil, nil
	}
	texts := map[string][]string{}
	from := map[string]string{}
	for _, class := range append([]string{pc.BMMName}, AncestorChain(plan, pc.BMMName)...) {
		for _, elem := range sortedStringKeys(plan.extensions.droppedElements[class]) {
			want := plan.extensions.droppedElements[class][elem]
			if prev, ok := texts[elem]; ok && !slices.Equal(prev, want) {
				return nil, fmt.Errorf("%s inherits the dropped element %s from %s and from %s, with different texts", pc.BMMName, elem, from[elem], class)
			}
			texts[elem], from[elem] = want, class
		}
	}
	for _, ef := range fields {
		if prop := ef.Prop.PropertyName(); texts[prop] != nil {
			return nil, fmt.Errorf("%s drops %s, which is one of its members: %s.dropped_elements names it for %s", pc.BMMName, prop, sdkExtensionsMember, from[prop])
		}
	}
	out := make([]droppedXMLElement, 0, len(texts))
	for _, elem := range sortedStringKeys(texts) {
		out = append(out, droppedXMLElement{element: elem, texts: texts[elem]})
	}
	return out, nil
}

// unmarshalXMLDroppedElement emits the case of an element the class's schema
// accepts that no member holds: xmlunmatched.DropElement reads it, drops it
// when its text is one of texts, and refuses any other text.
func unmarshalXMLDroppedElement(elem string, texts []string) string {
	quoted := make([]string, len(texts))
	for i, text := range texts {
		quoted[i] = fmt.Sprintf("%q", text)
	}
	return fmt.Sprintf(
		"\t\t\tcase %q:\n\t\t\t\t// The schema accepts the element, which no member holds: it is read, and dropped when its text is %s.\n\t\t\t\tif _err := xmlunmatched.DropElement(_dec, &_t, %s); _err != nil {\n\t\t\t\t\treturn _err\n\t\t\t\t}\n",
		elem, strings.Join(quoted, " or "), strings.Join(quoted, ", "))
}

// enterXMLClass emits the call that hands the start element and the class's
// description to xmlunmatched.Enter at the head of UnmarshalXML, and clears
// the receiver when Enter says the decode starts the document over: the root
// of a second decode into a value the first decode filled in part.
func enterXMLClass(recv, goName, typeArgs string) string {
	return fmt.Sprintf(
		"\tif _fresh, _err := xmlunmatched.Enter(_dec, _start, &xmlClass%s); _err != nil {\n\t\treturn _err\n\t} else if _fresh {\n\t\t*%s = %s%s{}\n\t}\n",
		goName, recv, goName, typeArgs)
}

// maxXMLRequired is the most presence-checked members a class may have: the
// runtime keeps which of them an element has a value for in one 64-bit word.
const maxXMLRequired = 64

// xmlClassDesc is the description of a class its generated decoder hands
// xmlunmatched.Enter, rendered by [renderXMLClassVar] as a package-level var
// named `xmlClass<GoName>`: the class's BMM name, the members the decoder
// reads as XML attributes, which its ITS-XML schema type declares as
// attributes, in the type's order ([xmlAttributesOf]), the presence-checked
// mandatory members in the Reference Model's declaration order, and the
// multi-valued members.
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
	attrs, err := xmlAttributesOf(plan, pc, fields)
	if err != nil {
		return xmlClassDesc{}, err
	}
	d := xmlClassDesc{goName: pc.GoName, name: pc.BMMName, attrs: attrs.members}
	for _, ef := range fields {
		if xmlMultiValued(ef.Prop) {
			d.items = append(d.items, ef.Prop.PropertyName())
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

// maxXMLSingleValued is the most single-valued members a generated
// canonical-XML decoder guards against a repeat: it keeps a bit per member
// in one uint64.
const maxXMLSingleValued = 64

// xmlSingleValuedMembers returns the single-valued members among fields, each
// with its bit in the decoder's `_got`, numbered in the order of fields. A
// class with more of them than [maxXMLSingleValued] is refused.
func xmlSingleValuedMembers(plan *Plan, pc *PlannedClass, fields []emittedField) (map[string]int, error) {
	once := map[string]int{}
	for _, ef := range fields {
		single, err := xmlSingleValued(plan, ef)
		if err != nil {
			return nil, err
		}
		if single {
			once[ef.Prop.PropertyName()] = len(once)
		}
	}
	if len(once) > maxXMLSingleValued {
		return nil, fmt.Errorf("%s has %d single-valued members; a generated canonical-XML decoder guards at most %d", pc.BMMName, len(once), maxXMLSingleValued)
	}
	return once, nil
}

// xmlSingleValued reports whether ef is a single-valued member, whose value
// canonical XML writes as one child element: neither a multi-valued member
// ([xmlMultiValued]), whose child elements are its items, nor a Hash member,
// whose child elements are its entries. A second child element of a
// single-valued member is a repeat (REQ-170).
func xmlSingleValued(plan *Plan, ef emittedField) (bool, error) {
	shape, isHash, err := xmlHashOf(plan, ef)
	if err != nil || shape != nil || isHash {
		return false, err
	}
	return !xmlMultiValued(ef.Prop), nil
}

// withRepeatGuard puts at the head of caseBody, the case branch of a
// single-valued member whose bit in `_got` is bit, the check that hands a
// second child element of the member to xmlunmatched.SkipUnmatched and
// leaves the branch, so the decoder keeps the first child's value and the
// repeat is skipped, reported or refused as unmatched content (REQ-170).
// caseBody's first line is its case label.
func withRepeatGuard(caseBody string, bit int) (string, error) {
	label, body, ok := strings.Cut(caseBody, "\n")
	if !ok || !strings.HasPrefix(label, "\t\t\tcase ") {
		return "", fmt.Errorf("the case branch does not open with its case label: %q", label)
	}
	var b strings.Builder
	b.WriteString(label + "\n")
	fmt.Fprintf(&b, "\t\t\t\tif _got&(1<<%d) != 0 {\n", bit)
	b.WriteString("\t\t\t\t\tif _err := xmlunmatched.SkipUnmatched(_dec, _t); _err != nil {\n")
	b.WriteString("\t\t\t\t\t\treturn _err\n")
	b.WriteString("\t\t\t\t\t}\n")
	b.WriteString("\t\t\t\t\tbreak\n")
	b.WriteString("\t\t\t\t}\n")
	fmt.Fprintf(&b, "\t\t\t\t_got |= 1 << %d\n", bit)
	b.WriteString(body)
	return b.String(), nil
}

// xmlNilable reports whether ef is a single-valued member that a nilled
// element gives no value: an optional one, whose Go field holds a pointer, an
// interface or an Octet list, is left absent (REQ-056), and a mandatory one
// gets its zero value, as a missing member does (REQ-175). Not a multi-valued
// member or a Hash, not a boundary flag, not a member typed by the owner's
// generic parameter, which the field holds by value, and not one whose class
// has no decoder, which the decoder skips.
func xmlNilable(plan *Plan, ef emittedField) (bool, error) {
	if isIntervalFlag(ef.Prop.PropertyName()) {
		return false, nil
	}
	shape, isHash, err := xmlHashOf(plan, ef)
	if err != nil || shape != nil || isHash {
		return false, err
	}
	switch p := ef.Prop.(type) {
	case *bmm.SingleProperty:
		if isOpenParam(ef.Owner, p.TypeName) {
			return false, nil
		}
		if isPrimitive(p.TypeName) || p.TypeName == bmmAnyType || isInterfaceTypeRef(plan, p.TypeName) {
			return true, nil
		}
		decoding, err := xmlClassDecoding(plan, p.TypeName)
		return decoding != xmlNoDecoder, err
	case *bmm.GenericProperty:
		if p.TypeDef == nil || containerKinds[p.TypeDef.RootType] {
			return false, nil
		}
		decoding, err := xmlClassDecoding(plan, p.TypeDef.RootType)
		return decoding == xmlOwnDecoder, err
	case *bmm.ContainerProperty:
		return isOctetContainer(p.TypeDef), nil
	}
	return false, nil
}

// isMandatoryMember reports whether prop is a mandatory member, whatever its
// kind.
func isMandatoryMember(prop bmm.Property) bool {
	switch p := prop.(type) {
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

// withNilGuard puts at the head of caseBody, the case branch of a member held
// in the field field of recv, the check that hands a nilled element to
// xmlunmatched.SkipNil, gives the member no value, nil when it is optional
// and its zero value when it is mandatory, and leaves the branch (REQ-056,
// REQ-175). caseBody's first line is its case label.
func withNilGuard(caseBody, recv, field string, mandatory bool) (string, error) {
	label, body, ok := strings.Cut(caseBody, "\n")
	if !ok || !strings.HasPrefix(label, "\t\t\tcase ") {
		return "", fmt.Errorf("the case branch does not open with its case label: %q", label)
	}
	var b strings.Builder
	b.WriteString(label + "\n")
	b.WriteString("\t\t\t\tif _nil, _err := xmlunmatched.SkipNil(_dec, _t); _err != nil {\n")
	b.WriteString("\t\t\t\t\treturn _err\n")
	b.WriteString("\t\t\t\t} else if _nil {\n")
	if mandatory {
		fmt.Fprintf(&b, "\t\t\t\t\txmlunmatched.Zero(&%s.%s)\n", recv, field)
	} else {
		fmt.Fprintf(&b, "\t\t\t\t\t%s.%s = nil\n", recv, field)
	}
	b.WriteString("\t\t\t\t\tbreak\n")
	b.WriteString("\t\t\t\t}\n")
	b.WriteString(body)
	return b.String(), nil
}

// xmlMultiValued reports whether prop is a multi-valued member, whose items
// canonical XML writes as one element each: a container property, or a
// property typed by a generic List, Set or Array. An Octet list, such as
// DV_MULTIMEDIA `data`, is one element whose text is the base64 of its bytes,
// and canonical JSON one string, so it is single-valued: its JSON Pointer
// takes no index (REQ-175).
func xmlMultiValued(prop bmm.Property) bool {
	switch p := prop.(type) {
	case *bmm.ContainerProperty:
		return !isOctetContainer(p.TypeDef)
	case *bmm.GenericProperty:
		return p.TypeDef != nil && containerKinds[p.TypeDef.RootType] && p.TypeDef.RootType != "Hash"
	}
	return false
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

// unmarshalXMLAny emits the case of a property typed by the BMM root type
// Any, such as the AOM 1.4 `EXPR_LEAF.item`, held as a Go any, or as *any when
// optional: xmlschema.DecodeAny decodes the element as the class its
// discriminator names, as the text of an XML Schema built-in datatype, kept
// with its `xsi:type`, or, with no discriminator, as its text.
func unmarshalXMLAny(recv, field, elem string, mandatory bool) string {
	assign := fmt.Sprintf("\t\t\t\t%s.%s = _v\n", recv, field)
	if !mandatory {
		assign = fmt.Sprintf("\t\t\t\t%s.%s = &_v\n", recv, field)
	}
	return fmt.Sprintf(
		"\t\t\tcase %q:\n\t\t\t\t_v, _err := xmlschema.DecodeAny(_dec, _t)\n\t\t\t\tif _err != nil {\n\t\t\t\t\treturn _err\n\t\t\t\t}\n%s",
		elem, assign)
}
