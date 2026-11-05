package bmmgen

import (
	"errors"
	"fmt"

	"github.com/cadasto/openehr-sdk-go/openehr/bmm"
)

// xmlHash is how the generated canonical-XML codec writes and reads the
// entries of one Hash member, in the shapes the AM Release-1.4 schemas give
// them: one element per entry, named after the member, written in ascending
// order of the keys.
type xmlHash struct {
	// valueType is the Go type of an entry's value: "string" for a Hash of
	// Strings, else the value class's Go type, qualified when another target
	// emits it.
	valueType string
	// keyAttr names the XML attribute of an entry's element that holds its
	// key: `id` for a Hash of Strings, whose entries are StringDictionaryItems,
	// and, for a Hash of class values, the attribute the value class's ITS-XML
	// schema type declares that names no member of the class. It is "" when
	// the entries are keyed by their language.
	keyAttr string
	// languageKey is, for a Hash of class values keyed by their language, the
	// Go selector of the language code from a value: "Language.CodeString".
	languageKey string
	// optional reports a member held through a pointer, *map[string]V.
	optional bool
}

// stringDictionaryKey is the attribute of a StringDictionaryItem, the element
// of one entry of a Hash<String,String>, that holds the entry's key.
const stringDictionaryKey = "id"

// errXMLHashNoKey reports a Hash of class values whose entries canonical XML
// has no key for: the value class's ITS-XML schema type declares more than one
// attribute that names no member of the class, or none while the class has no
// mandatory language code.
var errXMLHashNoKey = errors.New("bmmgen: canonical XML has no key for the entries of a Hash of class values")

// xmlHashOf returns how the canonical-XML codec writes ef when ef is a Hash
// member; isHash is false for any other member. A Hash the codec does not
// model, one whose values are neither Strings nor a class with a decoder of
// its own, such as RESOURCE_ANNOTATIONS.documentation, a Hash of Hashes,
// returns a nil shape with isHash true.
//
// A Hash of class values is keyed by the attribute the value class's ITS-XML
// schema type declares that names no member of the class ([xmlAttributesOf]);
// with none, by the code of the class's language, a mandatory member whose
// class holds a mandatory String code_string, as a CODE_PHRASE does. A value
// class with more than one such attribute, or with none and no such language,
// is refused with [errXMLHashNoKey], and so is a Hash of polymorphic values,
// which would need `xsi:type` dispatch per entry.
func xmlHashOf(plan *Plan, ef emittedField) (shape *xmlHash, isHash bool, err error) {
	var value bmm.Type
	optional := false
	switch p := ef.Prop.(type) {
	case *bmm.GenericProperty:
		if p.TypeDef == nil || p.TypeDef.RootType != "Hash" {
			return nil, false, nil
		}
		if len(p.TypeDef.GenericParameters) == 2 {
			value = p.TypeDef.GenericParameters[1]
		}
		optional = !p.IsMandatory
	case *bmm.ContainerProperty:
		if p.TypeDef == nil || p.TypeDef.ContainerType != "Hash" {
			return nil, false, nil
		}
		value = p.TypeDef.TypeDef
	default:
		return nil, false, nil
	}
	st, ok := value.(*bmm.SimpleType)
	if !ok {
		return nil, true, nil
	}
	if st.TypeName == "String" {
		return &xmlHash{valueType: "string", keyAttr: stringDictionaryKey, optional: optional}, true, nil
	}
	pc, ok := plan.Classes[st.TypeName]
	if !ok {
		return nil, true, nil
	}
	where := fmt.Sprintf("%s.%s, a Hash of %s", ef.OwnerName, ef.Prop.PropertyName(), st.TypeName)
	if isInterfaceTypeRef(plan, st.TypeName) {
		return nil, true, fmt.Errorf("%w: %s, whose values are polymorphic", errXMLHashNoKey, where)
	}
	decoding, err := xmlClassDecoding(plan, st.TypeName)
	if err != nil {
		return nil, true, err
	}
	if decoding != xmlOwnDecoder {
		return nil, true, nil
	}
	valueType, err := typeRef(plan, ef.Owner, value)
	if err != nil {
		return nil, true, err
	}
	fields, err := effectiveFields(plan, pc)
	if err != nil {
		return nil, true, err
	}
	attrs, err := xmlAttributesOf(plan, pc, fields)
	if err != nil {
		return nil, true, err
	}
	shape = &xmlHash{valueType: valueType, optional: optional}
	switch len(attrs.keys) {
	case 0:
		if shape.languageKey, err = languageKeyOf(plan, fields); err != nil {
			return nil, true, fmt.Errorf("%w: %s, whose schema type declares no key attribute: %w", errXMLHashNoKey, where, err)
		}
	case 1:
		shape.keyAttr = attrs.keys[0]
	default:
		return nil, true, fmt.Errorf("%w: %s, whose schema type declares the attributes %q, none of them a member", errXMLHashNoKey, where, attrs.keys)
	}
	return shape, true, nil
}

// xmlHashMarks returns the names of the local variables a generated decoder
// keeps for the Hash members of fields the canonical-XML codec reads one
// element per entry, one per member in the order of fields: each marks that
// the element has had an entry of the member, so the runtime starts the
// member's map empty at the first.
func xmlHashMarks(plan *Plan, fields []emittedField) ([]string, error) {
	var marks []string
	for _, ef := range fields {
		shape, _, err := xmlHashOf(plan, ef)
		if err != nil {
			return nil, err
		}
		if shape != nil {
			marks = append(marks, xmlHashMark(FieldName(ef.Prop.PropertyName())))
		}
	}
	return marks, nil
}

// xmlHashMark returns the name of the local variable that marks the Hash
// member held in the Go field field ([xmlHashMarks]).
func xmlHashMark(field string) string {
	return "_begun" + field
}

// hasXMLHash reports whether one of fields is a Hash member the
// canonical-XML codec writes one element per entry.
func hasXMLHash(plan *Plan, fields []emittedField) (bool, error) {
	for _, ef := range fields {
		shape, _, err := xmlHashOf(plan, ef)
		if err != nil {
			return false, err
		}
		if shape != nil {
			return true, nil
		}
	}
	return false, nil
}

// languageKeyOf returns the Go selector of the language code of a value whose
// effective fields are fields: its mandatory `language` member, held by value,
// and that member's class's mandatory String `code_string`.
func languageKeyOf(plan *Plan, fields []emittedField) (string, error) {
	lang, ok := fieldNamed(fields, "language")
	if !ok {
		return "", errors.New("the class has no language member")
	}
	p, ok := lang.Prop.(*bmm.SingleProperty)
	if !ok || !p.IsMandatory {
		return "", errors.New("its language member is not a mandatory single member")
	}
	pc, ok := plan.Classes[p.TypeName]
	if !ok {
		return "", fmt.Errorf("its language member is a %s, which is no class", p.TypeName)
	}
	codeFields, err := effectiveFields(plan, pc)
	if err != nil {
		return "", err
	}
	code, ok := fieldNamed(codeFields, "code_string")
	if !ok || !isMandatoryString(code.Prop) {
		return "", fmt.Errorf("its language member's class %s has no mandatory String code_string", p.TypeName)
	}
	return FieldName("language") + "." + FieldName("code_string"), nil
}

// fieldNamed returns the field of fields whose property is named name.
func fieldNamed(fields []emittedField, name string) (emittedField, bool) {
	for _, ef := range fields {
		if ef.Prop.PropertyName() == name {
			return ef, true
		}
	}
	return emittedField{}, false
}

// marshalXMLHash emits the source lines that write the entries of the Hash
// member field, held as h says, as elements named elem, in ascending order of
// their keys: each with its key in the attribute h names, if any, and its
// value as text or through the value class's own encoder. A nil or empty Hash
// writes no element.
//
// An entry keyed by its value's language is written without its key, so the
// decoder reads it back under the language code: an entry held under any
// other key, the empty one included, which the decoder refuses, is refused
// with canxml.ErrInvalidShape, as a value the encoder cannot write is, and
// the error names neither the key nor the code.
func marshalXMLHash(recv, field, elem string, h *xmlHash) string {
	keys, m := recv+"."+field, recv+"."+field
	if h.optional {
		keys, m = "*"+keys, "(*"+m+")"
	}
	start := fmt.Sprintf("xml.StartElement{Name: xml.Name{Local: %q}}", elem)
	if h.keyAttr != "" {
		start = fmt.Sprintf("xml.StartElement{Name: xml.Name{Local: %q}, Attr: []xml.Attr{{Name: xml.Name{Local: %q}, Value: _k}}}", elem, h.keyAttr)
	}
	value := "&_v"
	if h.valueType == "string" {
		value = "_v"
	}
	check := ""
	if h.languageKey != "" {
		check = fmt.Sprintf(
			"\t\tif _k == \"\" || _v.%s != _k {\n\t\t\treturn fmt.Errorf(\"%%w: a %s entry is held under a key other than its language code\", canxml.ErrInvalidShape)\n\t\t}\n",
			h.languageKey, elem)
	}
	loop := fmt.Sprintf(
		"\tfor _, _k := range slices.Sorted(maps.Keys(%s)) {\n\t\t_v := %s[_k]\n%s\t\tif err := _e.EncodeElement(%s, %s); err != nil {\n\t\t\treturn err\n\t\t}\n\t}\n",
		keys, m, check, value, start)
	if !h.optional {
		return loop
	}
	// gofmt indents the loop inside the guard when the file is formatted.
	return fmt.Sprintf("\tif %s.%s != nil {\n%s\t}\n", recv, field, loop)
}

// unmarshalXMLHash emits the decode case of the Hash member field, held as h
// says: each element named elem is one entry, which the runtime decodes and
// adds to the map under its key, taking the key attribute off first or
// reading the key from the value. The member's mark ([xmlHashMarks]) goes with
// each entry, so the runtime starts the map empty at the element's first. An
// optional member's map is allocated on the first entry.
func unmarshalXMLHash(recv, field, elem string, h *xmlHash) string {
	m := "&" + recv + "." + field
	alloc := ""
	if h.optional {
		m = recv + "." + field
		alloc = fmt.Sprintf("\t\t\t\tif %s == nil {\n\t\t\t\t\t%s = new(map[string]%s)\n\t\t\t\t}\n", m, m, h.valueType)
	}
	mark := "&" + xmlHashMark(field)
	call := fmt.Sprintf("xmlunmatched.DecodeEntry(_dec, &_t, %q, %s, %s)", h.keyAttr, m, mark)
	if h.keyAttr == "" {
		call = fmt.Sprintf("xmlunmatched.DecodeEntryKeyedBy(_dec, &_t, func(_v *%s) string { return _v.%s }, %s, %s)", h.valueType, h.languageKey, m, mark)
	}
	return fmt.Sprintf("\t\t\tcase %q:\n%s\t\t\t\tif _err := %s; _err != nil {\n\t\t\t\t\treturn _err\n\t\t\t\t}\n", elem, alloc, call)
}

// unmarshalXMLHashUnmodelled emits the case of a Hash member canonical XML
// does not model ([xmlHashOf]): the element is handed to
// xmlunmatched.SkipUnmatched as one item, skipped by default, reported or
// refused on request, and the field is left at its zero value.
func unmarshalXMLHashUnmodelled(elem string) string {
	return fmt.Sprintf(
		"\t\t\tcase %q:\n\t\t\t\t// Canonical XML does not model a Hash of these values — see canxml/doc.go.\n\t\t\t\tif _err := xmlunmatched.SkipUnmodelled(_dec, _t); _err != nil {\n\t\t\t\t\treturn _err\n\t\t\t\t}\n",
		elem)
}
