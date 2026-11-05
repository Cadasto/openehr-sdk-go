package rm

import (
	"encoding/xml"
	"slices"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/internal/xmlunmatched"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml"
)

// The canonical-XML codec of TERMINOLOGY_CODE is written by hand: the BASE
// schema lists the class among its primitive types, for which the generator
// emits no codec, yet TERMINOLOGY_TERM holds one by value in `concept`.
// The generator's handWrittenXMLCodecs names it, so the decoders that hold a
// TerminologyCode hand its element to these methods.

var (
	_ xml.Marshaler   = (*TerminologyCode)(nil)
	_ xml.Unmarshaler = (*TerminologyCode)(nil)
)

// xmlClassTerminologyCode describes TERMINOLOGY_CODE to the canonical-XML
// decode runtime, as the generator describes each class it emits a decoder
// for. The RM tables list no mandatory member for it, so none is required.
var xmlClassTerminologyCode = xmlunmatched.Class{Name: "TERMINOLOGY_CODE"}

// terminologyCodeMembers names the members [TerminologyCode.UnmarshalXML]
// reads as child elements, each single-valued.
var terminologyCodeMembers = [...]string{"terminology_id", "code_string", "terminology_version", "uri"}

// MarshalXML emits canonical openEHR XML for TerminologyCode in the shape of a
// CODE_PHRASE, the form ITS-XML gives a coded term: `terminology_id` holds its
// identifier in a `value` child, then
// `code_string`. `terminology_version` and `uri`, which CODE_PHRASE does not
// have, follow as text elements when set. The default element local name is
// `terminology_code` when the parent did not set one.
func (t *TerminologyCode) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
	if start.Name.Local == "" {
		start.Name = xml.Name{Local: canxml.ElementName("Terminology_code")}
	}
	if err := e.EncodeToken(start); err != nil {
		return err
	}
	id := struct {
		Value string `xml:"value"`
	}{Value: t.TerminologyID}
	if err := e.EncodeElement(id, xml.StartElement{Name: xml.Name{Local: "terminology_id"}}); err != nil {
		return err
	}
	if err := e.EncodeElement(t.CodeString, xml.StartElement{Name: xml.Name{Local: "code_string"}}); err != nil {
		return err
	}
	if t.TerminologyVersion != nil {
		if err := e.EncodeElement(*t.TerminologyVersion, xml.StartElement{Name: xml.Name{Local: "terminology_version"}}); err != nil {
			return err
		}
	}
	if t.URI != nil {
		if err := e.EncodeElement(*t.URI, xml.StartElement{Name: xml.Name{Local: "uri"}}); err != nil {
			return err
		}
	}
	return e.EncodeToken(start.End())
}

// UnmarshalXML decodes canonical openEHR XML into TerminologyCode. It reads
// `terminology_id` in both spellings: the CODE_PHRASE one, with the
// identifier in a `value` child, and the plain text
// one that mirrors the canonical-JSON string. `code_string`,
// `terminology_version` and `uri` are text. Content it does not read, text
// between its child elements and a second child element of one member among
// it, is unmatched: skipped, or reported or refused under
// canxml.WithUnmatched and canxml.WithStrict. The first child element of a
// member gives its value. A member whose element carries `xsi:nil` has none:
// a mandatory one gets the empty string, an optional one stays nil, and what
// the element holds is unmatched content.
func (t *TerminologyCode) UnmarshalXML(dec *xml.Decoder, start xml.StartElement) error {
	if fresh, err := xmlunmatched.Enter(dec, start, &xmlClassTerminologyCode); err != nil {
		return err
	} else if fresh {
		*t = TerminologyCode{}
	}
	// Which members have had a child element.
	var got [len(terminologyCodeMembers)]bool
	for {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		switch tk := tok.(type) {
		case xml.StartElement:
			name := xmlunmatched.LocalName(tk.Name)
			if i := slices.Index(terminologyCodeMembers[:], name); i >= 0 {
				if got[i] {
					if err := xmlunmatched.SkipUnmatched(dec, tk); err != nil {
						return err
					}
					continue
				}
				got[i] = true
			}
			switch name {
			case "terminology_id":
				if nilled, err := xmlunmatched.SkipNil(dec, tk); err != nil {
					return err
				} else if nilled {
					t.TerminologyID = ""
					continue
				}
				if err := decodeTerminologyIDXML(dec, tk, &t.TerminologyID); err != nil {
					return err
				}
			case "code_string":
				if nilled, err := xmlunmatched.SkipNil(dec, tk); err != nil {
					return err
				} else if nilled {
					t.CodeString = ""
					continue
				}
				if err := xmlunmatched.DecodeMandatory(dec, &tk, &t.CodeString); err != nil {
					return err
				}
			case "terminology_version":
				if err := decodeOptionalStringXML(dec, tk, &t.TerminologyVersion); err != nil {
					return err
				}
			case "uri":
				if err := decodeOptionalStringXML(dec, tk, &t.URI); err != nil {
					return err
				}
			default:
				if err := xmlunmatched.SkipUnmatched(dec, tk); err != nil {
					return err
				}
			}
		case xml.CharData:
			if err := xmlunmatched.CheckUnmatchedText(dec, tk); err != nil {
				return err
			}
		case xml.EndElement:
			return nil
		}
	}
}

// decodeOptionalStringXML reads the element start opens, that of an optional
// String member, into *field: nil when the element is nilled, or has no text
// or whitespace only, which canonical XML reads as no value.
func decodeOptionalStringXML(dec *xml.Decoder, start xml.StartElement, field **string) error {
	if isNil, err := xmlunmatched.SkipNil(dec, start); err != nil || isNil {
		if isNil {
			*field = nil
		}
		return err
	}
	var v string
	ok, err := xmlunmatched.DecodeOptional(dec, &start, &v)
	if err != nil {
		return err
	}
	*field = nil
	if ok {
		*field = &v
	}
	return nil
}

// decodeTerminologyIDXML reads the `terminology_id` element start opens into
// out: the text of its first `value` child when it has one (the TERMINOLOGY_ID
// shape), and its own text otherwise. Any other child, and a second `value`,
// is unmatched content. So is text other than whitespace beside a `value`
// child, which then holds the identifier: text before the child is checked
// as the child opens, and text after it as it is read. An unmatched child
// after such text and before the `value` child waits for the text, so the
// items come in document order: it is checked once the `value` child opens
// and the text has been checked, or once the element closes with no `value`
// child, the text then being the identifier. An identifier that is empty or
// XML whitespace only is a member sent without a value
// ([xmlunmatched.ReportEmpty]), and out gets the empty string.
func decodeTerminologyIDXML(dec *xml.Decoder, start xml.StartElement, out *string) error {
	if err := xmlunmatched.CheckUnmatchedAttrs(dec, start); err != nil {
		return err
	}
	var text []byte
	var value *string
	var held xmlunmatched.Held
	for {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		switch tk := tok.(type) {
		case xml.CharData:
			if value != nil {
				if err := xmlunmatched.CheckUnmatchedText(dec, tk); err != nil {
					return err
				}
				continue
			}
			text = append(text, tk...)
		case xml.StartElement:
			if value == nil && xmlunmatched.LocalName(tk.Name) == "value" {
				if err := xmlunmatched.CheckUnmatchedTextBefore(dec, text); err != nil {
					return err
				}
				if err := xmlunmatched.ApplyHeld(dec, &held); err != nil {
					return err
				}
				var v string
				if err := xmlunmatched.DecodeElement(dec, &tk, &v); err != nil {
					return err
				}
				value = &v
				continue
			}
			if value == nil {
				if err := xmlunmatched.SkipUnmatchedAfter(dec, tk, text, &held); err != nil {
					return err
				}
				continue
			}
			if err := xmlunmatched.SkipUnmatched(dec, tk); err != nil {
				return err
			}
		case xml.EndElement:
			if err := xmlunmatched.ApplyHeld(dec, &held); err != nil {
				return err
			}
			if value != nil {
				*out = *value
			} else {
				*out = string(text)
			}
			if strings.TrimLeft(*out, " \t\r\n") == "" {
				// No value: the empty identifier of a document without it.
				*out = ""
				xmlunmatched.ReportEmpty(dec, start)
			}
			return nil
		}
	}
}
