package bmmgen

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/bmm"
)

// The classes and members of the two shapes a member's element can take
// ([XMLMemberShape]): the CODE_PHRASE of the BASE schema, and the
// DV_CODED_TEXT shape of the AM 1.4 ORDINAL_SYMBOL.
const (
	codePhraseClass      = "CODE_PHRASE"
	terminologyIDClass   = "TERMINOLOGY_ID"
	codePhraseTermID     = "terminology_id"
	codePhraseCodeString = "code_string"
	identifierValue      = "value"
	definingCodeElement  = "defining_code"
)

// codedTextOtherElements are the elements the DV_CODED_TEXT shape of the AM
// 1.4 ORDINAL_SYMBOL declares beside `defining_code`: the members of DV_TEXT
// and DV_CODED_TEXT that a CODE_PHRASE does not hold, which the decoder
// drops.
var codedTextOtherElements = []string{"value", "hyperlink", "formatting", "mappings", "language", "encoding"}

// checkXMLMemberShapes refuses a target whose [Target.XMLMemberShapes] the
// codecs could not honour: a shape the generator does not know; a key that
// names no mandatory single member, of the shape's type, that a concrete
// class of the target declares, or that [Target.XMLIdentifierElements] also
// names; a CODE_PHRASE whose members the shape needs are missing or have no
// canonical-XML codec; and, when the schemas are loaded, a member the schema
// type of the class does not declare.
func checkXMLMemberShapes(plan *Plan) error {
	shapes := plan.Target.XMLMemberShapes
	for _, key := range slices.Sorted(maps.Keys(shapes)) {
		shape := shapes[key]
		class, member, _ := strings.Cut(key, ".")
		var want string
		switch shape {
		case XMLCodePhraseString:
			want = "String"
		case XMLCodePhraseOrCodedText:
			want = codePhraseClass
		default:
			return fmt.Errorf("bmmgen: XML member shape of %s: unknown shape %d", key, shape)
		}
		if !declaresMandatoryMember(plan, class, member, want) {
			return fmt.Errorf("bmmgen: XML member shape of %s: no mandatory %s member a concrete class of the %s target declares", key, want, plan.Target.GoPackage)
		}
		if _, ok := plan.Target.XMLIdentifierElements[key]; ok {
			return fmt.Errorf("bmmgen: XML member shape of %s: the member is also written as an identifier element", key)
		}
		if err := checkCodePhraseClass(plan, shape); err != nil {
			return fmt.Errorf("bmmgen: XML member shape of %s: %w", key, err)
		}
		if plan.xmlOrder == nil || len(plan.xmlOrder.sets) == 0 {
			continue
		}
		seq, err := plan.xmlOrder.sequenceFor(plan, class)
		if err != nil {
			return fmt.Errorf("bmmgen: XML member shape of %s: %w", key, err)
		}
		if !slices.Contains(seq, member) {
			return fmt.Errorf("bmmgen: XML member shape of %s: the ITS-XML schema type of %s declares no element %s", key, class, member)
		}
	}
	return nil
}

// checkCodePhraseClass reports what of the CODE_PHRASE class the codecs of
// shape need and the plan lacks: a class with a canonical-XML codec, whose
// terminology_id is a TERMINOLOGY_ID with a String value and whose
// code_string is a String, for a CODE_PHRASE string; and no member of the
// name of an element of the DV_CODED_TEXT shape, whose decode cases it
// shares, for a member read in that shape too.
func checkCodePhraseClass(plan *Plan, shape XMLMemberShape) error {
	decoding, err := xmlClassDecoding(plan, codePhraseClass)
	if err != nil {
		return err
	}
	if decoding != xmlOwnDecoder {
		return fmt.Errorf("%s is no class with a canonical-XML codec", codePhraseClass)
	}
	pc := plan.Classes[codePhraseClass]
	sc, ok := pc.Class.(*bmm.SimpleClass)
	if !ok {
		return fmt.Errorf("%s is no simple class", codePhraseClass)
	}
	if shape == XMLCodePhraseString {
		if !singlePropertyOf(sc, codePhraseTermID, terminologyIDClass) || !identifierClass(plan, terminologyIDClass) {
			return fmt.Errorf("%s has no %s that is a %s with a String value", codePhraseClass, codePhraseTermID, terminologyIDClass)
		}
		if !singlePropertyOf(sc, codePhraseCodeString, "String") {
			return fmt.Errorf("%s has no String %s", codePhraseClass, codePhraseCodeString)
		}
		return nil
	}
	fields, err := effectiveFields(plan, pc)
	if err != nil {
		return err
	}
	for _, ef := range fields {
		if name := ef.Prop.PropertyName(); name == definingCodeElement || slices.Contains(codedTextOtherElements, name) {
			return fmt.Errorf("%s has a member %s, an element of the DV_CODED_TEXT shape", codePhraseClass, name)
		}
	}
	return nil
}

// declaresMandatoryMember reports whether class is a concrete class the
// target emits that declares member as a mandatory single property of the
// type typeName.
func declaresMandatoryMember(plan *Plan, class, member, typeName string) bool {
	pc, ok := plan.Classes[class]
	if !ok || pc.External || !slices.Contains(plan.ConcreteClasses, pc) {
		return false
	}
	sc, ok := pc.Class.(*bmm.SimpleClass)
	if !ok {
		return false
	}
	p, ok := sc.Properties[member].(*bmm.SingleProperty)
	return ok && p.IsMandatory && p.TypeName == typeName
}

// singlePropertyOf reports whether sc declares name as a single property of
// the type typeName.
func singlePropertyOf(sc *bmm.SimpleClass, name, typeName string) bool {
	p, ok := sc.Properties[name].(*bmm.SingleProperty)
	return ok && p.TypeName == typeName
}

// xmlMemberShape returns the shape the ITS-XML schema type of the member
// ef's class gives its element ([Target.XMLMemberShapes]), and whether it
// gives one.
func xmlMemberShape(plan *Plan, ef emittedField) (XMLMemberShape, bool) {
	shape, ok := plan.Target.XMLMemberShapes[ef.OwnerName+"."+ef.Prop.PropertyName()]
	return shape, ok
}

// marshalXMLCodePhraseString emits a String member written as a CODE_PHRASE
// ([XMLCodePhraseString]): a string holding `::` as a value of the Go type
// phraseType whose identifier, of the Go type idType, holds the part before
// the first `::` and whose code string holds the rest; any other string as
// the element's text.
func marshalXMLCodePhraseString(recv, field, elem, phraseType, idType string) string {
	return fmt.Sprintf(
		"\tif _id, _code, _ok := strings.Cut(%[1]s.%[2]s, \"::\"); _ok {\n"+
			"\t\tif err := _e.EncodeElement(&%[4]s{%[6]s: %[5]s{%[7]s: _id}, %[8]s: _code}, xml.StartElement{Name: xml.Name{Local: %[3]q}}); err != nil {\n"+
			"\t\t\treturn err\n\t\t}\n"+
			"\t} else if err := _e.EncodeElement(%[1]s.%[2]s, xml.StartElement{Name: xml.Name{Local: %[3]q}}); err != nil {\n"+
			"\t\treturn err\n\t}\n",
		recv, field, elem, phraseType, idType,
		FieldName(codePhraseTermID), FieldName(identifierValue), FieldName(codePhraseCodeString))
}

// unmarshalXMLCodePhraseString emits the decode case of a String member
// written as a CODE_PHRASE ([XMLCodePhraseString]), which
// xmlunmatched.DecodeCodePhraseString reads in either shape.
func unmarshalXMLCodePhraseString(recv, field, elem string) string {
	return fmt.Sprintf(
		"\t\t\tcase %q:\n\t\t\t\tif _err := xmlunmatched.DecodeCodePhraseString(_dec, &_t, &%s.%s); _err != nil {\n\t\t\t\t\treturn _err\n\t\t\t\t}\n",
		elem, recv, field)
}

// xmlCodedTextDecoder returns the name of the function that decodes the
// member ef read in the DV_CODED_TEXT shape too ([XMLCodePhraseOrCodedText]),
// rendered beside the decoder of the class that declares it, and the name
// of the description that function hands the runtime.
func xmlCodedTextDecoder(plan *Plan, ef emittedField) (fn, desc string) {
	name := ef.OwnerName
	if pc, ok := plan.Classes[ef.OwnerName]; ok {
		name = pc.GoName
	}
	name += FieldName(ef.Prop.PropertyName())
	return "unmarshalXML" + name, "xmlShape" + name
}

// unmarshalXMLCodedTextCall emits the decode case of a member read in the
// DV_CODED_TEXT shape too ([XMLCodePhraseOrCodedText]): the case hands the
// element to the member's own decode function.
func unmarshalXMLCodedTextCall(plan *Plan, recv string, ef emittedField) string {
	fn, _ := xmlCodedTextDecoder(plan, ef)
	return fmt.Sprintf(
		"\t\t\tcase %q:\n\t\t\t\tif _err := %s(_dec, _t, &%s.%s); _err != nil {\n\t\t\t\t\treturn _err\n\t\t\t\t}\n",
		ef.Prop.PropertyName(), fn, recv, FieldName(ef.Prop.PropertyName()))
}

// renderXMLCodedTextShapes renders, beside the decoder of pc, whose
// effective fields are fields, the decode of each member pc declares that
// the target reads in the DV_CODED_TEXT shape too
// ([XMLCodePhraseOrCodedText]), in the order of the table's keys. A member
// a subclass inherits is decoded by the function rendered beside the
// declaring class alone.
func renderXMLCodedTextShapes(plan *Plan, pc *PlannedClass, fields []emittedField) (string, error) {
	var b strings.Builder
	shapes := plan.Target.XMLMemberShapes
	for _, key := range slices.Sorted(maps.Keys(shapes)) {
		class, member, _ := strings.Cut(key, ".")
		if class != pc.BMMName || shapes[key] != XMLCodePhraseOrCodedText {
			continue
		}
		i := slices.IndexFunc(fields, func(ef emittedField) bool {
			return ef.OwnerName == class && ef.Prop.PropertyName() == member
		})
		if i < 0 {
			return "", fmt.Errorf("%s: no member %s to read in the DV_CODED_TEXT shape", pc.BMMName, member)
		}
		src, err := renderXMLCodedTextShape(plan, fields[i])
		if err != nil {
			return "", fmt.Errorf("%s: %w", pc.BMMName, err)
		}
		b.WriteString("\n")
		b.WriteString(src)
	}
	return b.String(), nil
}

// renderXMLCodedTextShape renders the decode of a CODE_PHRASE member ef that
// the target reads in the DV_CODED_TEXT shape too ([XMLCodePhraseOrCodedText]):
// a description of the member's element and the function its case calls
// ([xmlCodedTextDecoder]). The function reads the children of a CODE_PHRASE
// as its own decoder does. It reads the DV_CODED_TEXT's `defining_code`
// into the member, through the CODE_PHRASE decoder, and the description
// names that element as one whose members are the member's own
// (xmlunmatched.Class.Inline). It drops the DV_CODED_TEXT's other elements
// through xmlunmatched.Drop. Anything else is unmatched, as in any decoder.
func renderXMLCodedTextShape(plan *Plan, ef emittedField) (string, error) {
	p, ok := ef.Prop.(*bmm.SingleProperty)
	if !ok {
		return "", fmt.Errorf("%s.%s: a member read in the DV_CODED_TEXT shape is no single property", ef.OwnerName, ef.Prop.PropertyName())
	}
	pc, ok := plan.Classes[p.TypeName]
	if !ok {
		return "", fmt.Errorf("%s.%s: no class %s", ef.OwnerName, p.TypeName, p.TypeName)
	}
	sc, ok := pc.Class.(*bmm.SimpleClass)
	if !ok {
		return "", fmt.Errorf("%s.%s: %s is no simple class", ef.OwnerName, p.Name, p.TypeName)
	}
	fields, err := effectiveFields(plan, pc)
	if err != nil {
		return "", err
	}
	desc, err := xmlClassDescriptor(plan, pc, fields)
	if err != nil {
		return "", err
	}
	desc.inline = []string{definingCodeElement}
	fn, descName := xmlCodedTextDecoder(plan, ef)
	member := ef.OwnerName + "." + p.Name
	goType := goNameForRef(plan, p.TypeName)

	var b strings.Builder
	fmt.Fprintf(&b, "// %s describes %s to the\n", descName, member)
	fmt.Fprintf(&b, "// canonical-XML decode runtime: a %s the ITS-XML schema\n", p.TypeName)
	b.WriteString("// also lets a document write as a DV_CODED_TEXT, the members of\n")
	fmt.Fprintf(&b, "// whose %s are its own.\n", definingCodeElement)
	b.WriteString(renderXMLDescriptor(descName, desc))
	b.WriteString("\n")
	fmt.Fprintf(&b, "// %s decodes the element start opens into c,\n", fn)
	fmt.Fprintf(&b, "// %s, in either shape its ITS-XML schema type gives it:\n", member)
	fmt.Fprintf(&b, "// a %s, or a DV_CODED_TEXT whose %s is the\n", p.TypeName, definingCodeElement)
	fmt.Fprintf(&b, "// %s. It drops the DV_CODED_TEXT's other elements. Content\n", p.TypeName)
	b.WriteString("// it does not read is unmatched, as in UnmarshalXML.\n")
	fmt.Fprintf(&b, "func %s(_dec *xml.Decoder, _start xml.StartElement, c *%s) error {\n", fn, goType)
	fmt.Fprintf(&b, "\tif _err := xmlunmatched.Enter(_dec, _start, &%s); _err != nil {\n\t\treturn _err\n\t}\n", descName)
	b.WriteString("\tfor {\n")
	b.WriteString("\t\t_tok, _err := _dec.Token()\n")
	b.WriteString("\t\tif _err != nil {\n\t\t\treturn _err\n\t\t}\n")
	b.WriteString("\t\tswitch _t := _tok.(type) {\n")
	b.WriteString("\t\tcase xml.StartElement:\n")
	b.WriteString("\t\t\tswitch xmlunmatched.LocalName(_t.Name) {\n")
	for _, f := range fields {
		caseBody, err := renderUnmarshalXMLField(plan, "c", sc, f)
		if err != nil {
			return "", fmt.Errorf("render %s field %s.%s: %w", fn, p.TypeName, f.Prop.PropertyName(), err)
		}
		b.WriteString(caseBody)
	}
	fmt.Fprintf(&b, "\t\t\tcase %q:\n\t\t\t\tif _err := _dec.DecodeElement(c, &_t); _err != nil {\n\t\t\t\t\treturn _err\n\t\t\t\t}\n", definingCodeElement)
	b.WriteString("\t\t\tcase ")
	for i, name := range codedTextOtherElements {
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "%q", name)
	}
	b.WriteString(":\n\t\t\t\tif _err := xmlunmatched.Drop(_dec); _err != nil {\n\t\t\t\t\treturn _err\n\t\t\t\t}\n")
	b.WriteString("\t\t\tdefault:\n")
	b.WriteString("\t\t\t\tif _err := xmlunmatched.SkipUnmatched(_dec, _t); _err != nil {\n\t\t\t\t\treturn _err\n\t\t\t\t}\n")
	b.WriteString("\t\t\t}\n")
	b.WriteString(checkUnmatchedXMLText)
	b.WriteString("\t\tcase xml.EndElement:\n\t\t\treturn nil\n")
	b.WriteString("\t\t}\n")
	b.WriteString("\t}\n")
	b.WriteString("}\n")
	return b.String(), nil
}
