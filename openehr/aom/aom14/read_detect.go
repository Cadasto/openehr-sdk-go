package aom14

import (
	"bytes"
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
// dispatch, which only the default mode reads, or an element in the namespace
// the AM Release-1.4 schemas declare, which a strict decode refuses.
func retryable(err error, strict bool) bool {
	switch {
	case xmlunmatched.IsPrimitiveFailure(err), errors.Is(err, canxml.ErrNamespace):
		return true
	case strict:
		return false
	}
	return errors.Is(err, typereg.ErrMissingType) || errors.Is(err, typereg.ErrUnknownType) || errors.Is(err, typereg.ErrTypeMismatch)
}

// needsTolerance reports whether data, which the first decode read into ot
// without a failure, may hold what [tolerate] reads otherwise than that
// decode: a C_BOOLEAN item without its true_valid or false_valid, or with an
// empty one, which the decode reads as false, or with an assumed value, which
// may be one the parser reads as none; the namespace the AM Release-1.4
// schemas declare; a bare type attribute, which the decode reads as an
// xsi:type; or an empty element a number is read from, which the decode reads
// as 0. A check may find what tolerate then leaves as it is, which costs a
// second reading that changes nothing; none misses what it looks for.
func needsTolerance(data []byte, ot *OperationalTemplate) bool {
	return booleanAssumes(ot.Definition.Attributes) ||
		bytes.Contains(data, []byte(nsOpenEHRv2)) ||
		hasBareType(data) ||
		hasEmptyValue(data) ||
		!booleansHoldFlags(data)
}

// booleanAssumes reports whether a C_BOOLEAN item of a C_PRIMITIVE_OBJECT
// under attrs has an assumed value.
func booleanAssumes(attrs []CAttribute) bool {
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
				if v != nil && booleanAssumes(v.Attributes) {
					return true
				}
			case *CArchetypeRoot:
				if v != nil && booleanAssumes(v.Attributes) {
					return true
				}
			case *CPrimitiveObject:
				if v == nil {
					continue
				}
				if b, ok := v.Item.(*CBoolean); ok && b != nil && b.AssumedValue != nil {
					return true
				}
			}
		}
	}
	return false
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
