package aom14

import (
	"bytes"
	"encoding/xml"
	"errors"

	"github.com/cadasto/openehr-sdk-go/openehr/internal/xmlunmatched"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml"
)

// read_detect.go: when [ReadOperationalTemplate] reads a document a second
// time, through [tolerate]. A document the parser and canonical XML read
// alike, which most are, pays for the checks below and for no second reading.
// Each reading tolerate makes applies to a document only where the first
// decode failed in a way this file names, or where the checks below find
// what the reading changes, so the result never depends on whether the
// second reading ran.

// retryable reports whether err, the failure of the first decode, is one a
// reading of [tolerate] can lift: the text of a value, a type a slot cannot
// dispatch, which only the default mode reads but inside a C_DV_QUANTITY,
// whose assumed value every mode reads as absent where the model cannot hold
// it, or an element in the namespace the AM Release-1.4 schemas declare,
// which a strict decode refuses. A type a slot cannot dispatch covers a
// <children> of a primitive or an attribute class and an <item> of a class
// the parser reads a constraint of; the text of a value covers a real number
// canonical XML does not read and an integer bound written as a decimal.
func retryable(err error, strict bool) bool {
	switch {
	case xmlunmatched.IsPrimitiveFailure(err), errors.Is(err, canxml.ErrNamespace), failsInsideQuantity(err):
		return true
	case strict:
		return false
	}
	return isTypeFailure(err)
}

// isTypeFailure reports whether err is the failure of a type a slot cannot
// dispatch.
func isTypeFailure(err error) bool {
	return errors.Is(err, typereg.ErrMissingType) || errors.Is(err, typereg.ErrUnknownType) || errors.Is(err, typereg.ErrTypeMismatch)
}

// failsInsideQuantity reports whether err is the failure of a type a slot
// inside a C_DV_QUANTITY cannot dispatch, such as a bound of the normal range
// of its assumed value.
func failsInsideQuantity(err error) bool {
	if !isTypeFailure(err) {
		return false
	}
	for {
		de, ok := errors.AsType[*canxml.DecodeError](err)
		if !ok || de == nil {
			return false
		}
		if de.Type == "C_DV_QUANTITY" {
			inner, ok := errors.AsType[*canxml.DecodeError](de.Inner)
			return ok && inner != nil
		}
		err = de.Inner
	}
}

// needsTolerance reports whether data, which the first decode read into ot
// without a failure, may hold what [tolerate] reads otherwise than that
// decode: a C_BOOLEAN item without its true_valid or false_valid, or with an
// empty one, which the decode reads as false, or with an assumed value, which
// may be one the parser reads as none; the namespace the AM Release-1.4
// schemas declare; a bare type attribute, which the decode reads as an
// xsi:type; or an empty element a number is read from, which the decode reads
// as 0. Unless strict, it also looks for a root element in a namespace the
// decode does not read, whose content the decode skips, and for an archetype
// id on a constraint object the decode read as a C_COMPLEX_OBJECT, which it
// skips: more archetype_id elements than archetype roots that hold one. A
// check may find what tolerate then leaves as it is, which costs a second
// reading that changes nothing; none misses what it looks for.
func needsTolerance(data []byte, ot *OperationalTemplate, strict bool) bool {
	var m modelFacts
	m.walk(ot.Definition.Attributes)
	if m.booleanAssumes ||
		bytes.Contains(data, []byte(nsOpenEHRv2)) ||
		hasBareType(data) ||
		hasEmptyValue(data) ||
		!booleansHoldFlags(data) {
		return true
	}
	if strict {
		return false
	}
	if ot.Definition.ArchetypeID.Value != "" {
		m.roots++
	}
	return foreignRoot(data) || archetypeIDs(data) > m.roots
}

// modelFacts is what [needsTolerance] reads off the model the first decode
// gave.
type modelFacts struct {
	// booleanAssumes is true where a C_BOOLEAN item has an assumed value.
	booleanAssumes bool
	// roots counts the archetype roots that hold an archetype id.
	roots int
}

// walk reads the constraint objects under attrs into m.
func (m *modelFacts) walk(attrs []CAttribute) {
	for _, a := range attrs {
		var children []CObject
		switch v := a.(type) {
		case *CSingleAttribute:
			if v != nil {
				children = v.Children
			}
		case *CMultipleAttribute:
			if v != nil {
				children = v.Children
			}
		}
		for _, c := range children {
			switch v := c.(type) {
			case *CComplexObject:
				if v != nil {
					m.walk(v.Attributes)
				}
			case *CArchetypeRoot:
				if v != nil {
					if v.ArchetypeID.Value != "" {
						m.roots++
					}
					m.walk(v.Attributes)
				}
			case *CPrimitiveObject:
				if v == nil {
					continue
				}
				if b, ok := v.Item.(*CBoolean); ok && b != nil && b.AssumedValue != nil {
					m.booleanAssumes = true
				}
			}
		}
	}
}

// foreignRoot reports whether the root element of data is in a namespace
// other than none, the one canonical XML reads, the one the AM Release-1.4
// schemas declare and the XSI one: a namespace the default mode reads as
// canonical XML's. It reads data up to the root's start tag.
func foreignRoot(data []byte) bool {
	d := xml.NewDecoder(bytes.NewReader(bytes.TrimPrefix(data, byteOrderMark)))
	for {
		tok, err := d.RawToken()
		if err != nil {
			return false
		}
		if start, ok := tok.(xml.StartElement); ok {
			return isForeignNamespace(ownNamespace(start.Name, start.Attr))
		}
	}
}

// archetypeIDs counts the start tags in data whose local name is
// archetype_id, with a prefix or without. Text that reads so, in a comment
// or a CDATA section, counts as well.
func archetypeIDs(data []byte) int {
	word := []byte("archetype_id")
	n := 0
	for i := 0; ; {
		j := bytes.Index(data[i:], word)
		if j < 0 {
			return n
		}
		j += i
		i = j + len(word)
		if j == 0 || i >= len(data) || !isTagEnd(data[i]) {
			continue
		}
		switch data[j-1] {
		case '<':
			n++
		case ':':
			if lt := bytes.LastIndexByte(data[:j], '<'); lt >= 0 && data[lt+1] != '/' && !bytes.ContainsAny(data[lt:j], " \t\r\n>") {
				n++
			}
		}
	}
}

// booleansHoldFlags reports whether each C_BOOLEAN in data is the type of an
// element that holds a true_valid and a false_valid. A C_BOOLEAN anywhere
// else, such as in text, reads as an element without them.
func booleansHoldFlags(data []byte) bool {
	word := []byte("C_BOOLEAN")
	for i := 0; ; {
		j := bytes.Index(data[i:], word)
		if j < 0 {
			return true
		}
		j += i
		i = j + len(word)
		lt := bytes.LastIndexByte(data[:j], '<')
		if lt < 0 || bytes.IndexByte(data[lt:j], '>') >= 0 {
			return false
		}
		name := data[lt+1:]
		if end := bytes.IndexAny(name, " \t\r\n/>"); end > 0 {
			name = name[:end]
		} else {
			return false
		}
		gt := bytes.IndexByte(data[i:], '>')
		if gt < 0 || data[i+gt-1] == '/' {
			return false
		}
		content := data[i+gt+1:]
		if end := bytes.Index(content, append([]byte("</"), name...)); end >= 0 {
			content = content[:end]
		}
		if !opens(content, "true_valid") || !opens(content, "false_valid") {
			return false
		}
	}
}

// opens reports whether content holds a start tag named local, with or
// without a prefix.
func opens(content []byte, local string) bool {
	name := []byte(local)
	for i := 0; ; {
		j := bytes.Index(content[i:], name)
		if j < 0 {
			return false
		}
		j += i
		i = j + len(name)
		if j == 0 || (content[j-1] != '<' && content[j-1] != ':') || (i < len(content) && !isTagEnd(content[i])) {
			continue // a longer name, or no name of an element
		}
		if lt := bytes.LastIndexByte(content[:j], '<'); lt >= 0 && content[lt+1] != '/' && !bytes.ContainsAny(content[lt:j], " \t\r\n>") {
			return true
		}
	}
}

// isTagEnd reports whether c may follow the name of an element in its tag.
func isTagEnd(c byte) bool {
	return c == '>' || c == '/' || isXMLSpaceByte(c)
}

// hasBareType reports whether data may hold a bare type attribute: the word
// type after XML white space and before an equals sign. Text that reads so
// gives a false positive.
func hasBareType(data []byte) bool {
	word := []byte("type")
	for i := 0; ; {
		j := bytes.Index(data[i:], word)
		if j < 0 {
			return false
		}
		j += i
		i = j + len(word)
		if j == 0 || !isXMLSpaceByte(data[j-1]) {
			continue
		}
		k := i
		for k < len(data) && isXMLSpaceByte(data[k]) {
			k++
		}
		if k < len(data) && data[k] == '=' {
			return true
		}
	}
}

// hasEmptyValue reports whether data holds an element that holds nothing,
// `<name/>` or `<name></name>` with attributes or none, whose local name is
// that of an element a number or a boolean is read from: list, lower, upper,
// assumed_value, true_valid or false_valid. It looks at each slash: one
// before `>` ends a start tag that closes its element, and one after `><`
// opens the end tag of an element that holds nothing.
func hasEmptyValue(data []byte) bool {
	for i := 0; ; {
		j := bytes.IndexByte(data[i:], '/')
		if j < 0 {
			return false
		}
		j += i
		i = j + 1
		switch {
		case j+1 < len(data) && data[j+1] == '>':
			if isValueName(startTagName(data[:j])) {
				return true
			}
		case j >= 2 && data[j-1] == '<' && data[j-2] == '>':
			if isValueName(startTagName(data[:j-2])) {
				return true
			}
		}
	}
}

// startTagName returns the local name of the start tag that head ends in, or
// nil when head ends in no start tag: the name after the last `<`, without
// its prefix.
func startTagName(head []byte) []byte {
	lt := bytes.LastIndexByte(head, '<')
	if lt < 0 || lt+1 >= len(head) {
		return nil
	}
	tag := head[lt+1:]
	if c := tag[0]; c == '/' || c == '!' || c == '?' {
		return nil
	}
	if end := bytes.IndexAny(tag, " \t\r\n/>"); end >= 0 {
		tag = tag[:end]
	}
	if _, local, ok := bytes.Cut(tag, []byte(":")); ok {
		tag = local
	}
	return tag
}

// isValueName reports whether local names an element a number or a boolean
// is read from, which canonical XML reads as 0 or false when it holds nothing.
func isValueName(local []byte) bool {
	switch string(local) {
	case "list", "lower", "upper", "assumed_value", "true_valid", "false_valid":
		return true
	}
	return false
}

// isXMLSpaceByte reports whether c is XML white space.
func isXMLSpaceByte(c byte) bool {
	return c == ' ' || c == '\t' || c == '\r' || c == '\n'
}
