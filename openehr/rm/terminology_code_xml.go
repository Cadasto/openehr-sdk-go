package rm

import (
	"encoding/xml"

	"github.com/cadasto/openehr-sdk-go/openehr/internal/xmlunmatched"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml"
)

// The canonical-XML codec of TERMINOLOGY_CODE is written by hand: the BASE
// schema lists the class among its primitive types, for which the generator
// emits no codec, yet AOM 1.4 holds one by value in ARCHETYPE.original_language.
// The generator's handWrittenXMLCodecs names it, so the decoders that hold a
// TerminologyCode hand its element to these methods.

var (
	_ xml.Marshaler   = (*TerminologyCode)(nil)
	_ xml.Unmarshaler = (*TerminologyCode)(nil)
)

// MarshalXML emits canonical openEHR XML for TerminologyCode in the shape the
// AOM 1.4 ITS-XML schema gives `original_language`, a CODE_PHRASE:
// `terminology_id` holds its identifier in a `value` child, then
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
// `terminology_id` in both spellings: the CODE_PHRASE one of the AOM 1.4
// ITS-XML schema, with the identifier in a `value` child, and the plain text
// one that mirrors the canonical-JSON string. `code_string`,
// `terminology_version` and `uri` are text. Content it does not read, text
// between its child elements among it, is unmatched: skipped, or reported or
// refused under canxml.WithUnmatched and canxml.WithStrict.
func (t *TerminologyCode) UnmarshalXML(dec *xml.Decoder, start xml.StartElement) error {
	if err := xmlunmatched.CheckUnmatchedAttrs(dec, start); err != nil {
		return err
	}
	for {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		switch tk := tok.(type) {
		case xml.StartElement:
			switch xmlunmatched.LocalName(tk.Name) {
			case "terminology_id":
				if err := decodeTerminologyIDXML(dec, tk, &t.TerminologyID); err != nil {
					return err
				}
			case "code_string":
				if err := xmlunmatched.DecodeElement(dec, &tk, &t.CodeString); err != nil {
					return err
				}
			case "terminology_version":
				var v string
				if err := xmlunmatched.DecodeElement(dec, &tk, &v); err != nil {
					return err
				}
				t.TerminologyVersion = &v
			case "uri":
				var v string
				if err := xmlunmatched.DecodeElement(dec, &tk, &v); err != nil {
					return err
				}
				t.URI = &v
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

// decodeTerminologyIDXML reads the `terminology_id` element start opens into
// out: the text of its first `value` child when it has one (the TERMINOLOGY_ID
// shape), and its own text otherwise. Any other child, and a second `value`,
// is unmatched content.
func decodeTerminologyIDXML(dec *xml.Decoder, start xml.StartElement, out *string) error {
	if err := xmlunmatched.CheckUnmatchedAttrs(dec, start); err != nil {
		return err
	}
	var text []byte
	var value *string
	for {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		switch tk := tok.(type) {
		case xml.CharData:
			text = append(text, tk...)
		case xml.StartElement:
			if value == nil && xmlunmatched.LocalName(tk.Name) == "value" {
				var v string
				if err := xmlunmatched.DecodeElement(dec, &tk, &v); err != nil {
					return err
				}
				value = &v
				continue
			}
			if err := xmlunmatched.SkipUnmatched(dec, tk); err != nil {
				return err
			}
		case xml.EndElement:
			if value != nil {
				*out = *value
			} else {
				*out = string(text)
			}
			return nil
		}
	}
}
