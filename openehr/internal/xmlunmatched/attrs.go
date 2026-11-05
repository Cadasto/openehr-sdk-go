package xmlunmatched

import (
	"encoding/xml"
	"errors"
)

// errRepeatedAttr is the cause of the refusal of an element that carries one
// attribute twice, which is not well-formed XML. It names no value.
var errRepeatedAttr = errors.New("the element carries one attribute twice")

// repeatedAttr returns the name of the first attribute of attrs that an
// earlier one carries too, by its name as the namespaces in scope resolve it,
// and whether there is one. Two spellings of one name, such as two prefixes
// bound to one namespace, are one name; a namespace declaration is an
// attribute like any other.
func repeatedAttr(attrs []xml.Attr) (xml.Name, bool) {
	for i := 1; i < len(attrs); i++ {
		for j := range i {
			if attrs[j].Name == attrs[i].Name {
				return attrs[i].Name, true
			}
		}
	}
	return xml.Name{}, false
}

// attrSegment returns the last path segment that names the attribute n: `@`
// and its local name, or `@xmlns:` and the prefix a namespace declaration
// declares.
func attrSegment(n xml.Name) string {
	if n.Space == "xmlns" {
		return "@xmlns:" + n.Local
	}
	return "@" + n.Local
}

// checkRepeatedAttr refuses the element start opens, in a decode no tracker
// reads, when it carries one attribute twice: a primitive failure that wraps
// [ErrInvalidShape] and names the element by its name alone and the
// attribute, as an untracked primitive failure names its element. A tracked
// or scoped decode refuses such an element as its reader reads it
// ([trackingReader.Token]), at its path from the root of the value.
func checkRepeatedAttr(start *xml.StartElement) error {
	if n, ok := repeatedAttr(start.Attr); ok {
		return primitiveFailure("/"+start.Name.Local+"/"+attrSegment(n), errRepeatedAttr)
	}
	return nil
}

// skipUntracked skips the element start opens in a decode no tracker reads,
// as dec.Skip does, refusing start or any element inside it that carries one
// attribute twice ([checkRepeatedAttr]). start must be the token the decoder
// read last.
func skipUntracked(dec *xml.Decoder, start *xml.StartElement) error {
	if err := checkRepeatedAttr(start); err != nil {
		return err
	}
	for depth := 1; depth > 0; {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		switch tk := tok.(type) {
		case xml.StartElement:
			if err := checkRepeatedAttr(&tk); err != nil {
				return err
			}
			depth++
		case xml.EndElement:
			depth--
		}
	}
	return nil
}
