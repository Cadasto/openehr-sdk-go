package xmlunmatched

import (
	"encoding/xml"
	"strings"
)

// SkipNil skips the element start opens when it carries `xsi:nil="true"` (or
// `1`), the element of a member sent without a value, and reports whether it
// did: the member then holds no value, as a `null` member does in canonical
// JSON, whatever the element holds, and the decoder leaves an optional member
// absent and gives a mandatory one its zero value ([Zero]). The element is
// not read, so it needs no `xsi:type` where the member is polymorphic. start
// must be the token the decoder read last.
//
// By default the content of a nilled element is skipped. A tracked decode
// applies its policy for unmatched content to the content: each attribute
// other than the XML Schema instance ones and the namespace declarations, each
// child element, as one item whatever it holds, and text other than XML
// whitespace, as one item, so a strict decode refuses the first. The member
// is one sent without a value ([Tracker.ReportPresence]), and nothing inside
// it is reported missing.
func SkipNil(dec *xml.Decoder, start xml.StartElement) (bool, error) {
	if !isNilled(start.Attr) {
		return false, nil
	}
	t := trackerOf(dec)
	if t == nil {
		return true, skipUntracked(dec, &start)
	}
	if err := t.attrs(start.Attr, nil); err != nil {
		return true, err
	}
	n := len(t.open)
	for {
		tok, err := dec.Token()
		if err != nil {
			return true, err
		}
		switch tk := tok.(type) {
		case xml.CharData:
			if err := t.text(n, tk); err != nil {
				return true, err
			}
		case xml.StartElement:
			if err := t.unmatched(t.elemPath(len(t.open)), isForeign(tk.Name)); err != nil {
				return true, err
			}
			if err := dec.Skip(); err != nil {
				return true, err
			}
		case xml.EndElement:
			return true, nil
		}
	}
}

// DecodeOptional decodes the element start opens, that of an optional member
// of a primitive type, into v as [DecodeElement] does, unless the element
// carries no value, and reports whether it decoded it. An element carries no
// value when it has no text, or, for a string, when its text is XML
// whitespace only: the member is then sent without a value and the decoder
// leaves it absent, where [DecodeElement] would decode the zero value. A
// decode that keeps a string's text ([Tracker.KeepStringText]) decodes such
// a string element too, as the text it carries, and reports it decoded. Text
// that does not convert to v's type fails as in [DecodeElement], so a number
// or boolean element of whitespace only fails. The decoder hands a nilled
// element to [SkipNil] first. start must be the token the decoder read last.
func DecodeOptional(dec *xml.Decoder, start *xml.StartElement, v any) (bool, error) {
	if _, ok := v.(xml.Unmarshaler); ok {
		return true, DecodeElement(dec, start, v)
	}
	if t := trackerOf(dec); t != nil {
		return t.decodeOptional(dec, start, v)
	}
	if err := checkRepeatedAttr(start); err != nil {
		return false, err
	}
	var text string
	if err := dec.DecodeElement(&text, start); err != nil {
		return false, err
	}
	if noValue(text, v) {
		return false, nil
	}
	if s, ok := v.(*string); ok {
		*s = text
		return true, nil
	}
	d := new(textDecoder)
	d.toks = [3]xml.Token{xml.StartElement{Name: start.Name}, xml.CharData(text), xml.EndElement{Name: start.Name}}
	if err := xml.NewTokenDecoder(d).Decode(asText(v)); err != nil {
		return false, untrackedFailure(start, v, err)
	}
	return true, nil
}

// Zero sets *p to the zero value of its type: the value of a mandatory member
// whose element carries `xsi:nil`, as of one the document leaves out, which
// the decoder sets once [SkipNil] has skipped the element.
func Zero[T any](p *T) {
	var zero T
	*p = zero
}

// DecodeMandatory decodes the element start opens, that of a mandatory member
// of a primitive type, into v as [DecodeElement] does, except a string
// element whose text is XML whitespace only: it carries no value, so the
// member gets the empty string, the zero value the decode gives a document
// without the element. A decode that keeps a string's text
// ([Tracker.KeepStringText]) keeps that whitespace instead. The decoder
// hands a nilled element to [SkipNil] first. start must be the token the
// decoder read last.
func DecodeMandatory(dec *xml.Decoder, start *xml.StartElement, v any) error {
	if err := DecodeElement(dec, start, v); err != nil {
		return err
	}
	if s, ok := v.(*string); ok && noValue(*s, s) && !KeepsStringText(dec) {
		*s = ""
	}
	return nil
}

// KeepStringText makes the decodes of t keep the text of a string element
// sent empty or as XML whitespace only, when the element is not nilled, as
// the string's value: [DecodeOptional] then decodes it, where it would leave
// the member absent,
// [DecodeMandatory] keeps its whitespace, and the member carries a value, so
// it is no missing member ([Tracker.ReportMissing]). It is still sent
// without a value for [Tracker.ReportPresence], which reports it as empty.
func (t *Tracker) KeepStringText() {
	t.stringText = true
}

// KeepsStringText reports whether dec belongs to a decode that keeps the
// text of a string element sent empty or as whitespace only
// ([Tracker.KeepStringText]). A hand-written decoder that reads a string
// member's text itself asks it before it drops such text.
func KeepsStringText(dec *xml.Decoder) bool {
	t := trackerOf(dec)
	return t != nil && t.stringText
}

// keepsText reports whether the decodes of t keep the text of v, the target
// of a primitive element, whatever it is: v is a string and t keeps a
// string's text.
func (t *Tracker) keepsText(v any) bool {
	_, isString := v.(*string)
	return isString && t.stringText
}

// noValue reports whether text, the whole text of a primitive element decoded
// into v, carries no value: it is empty, or XML whitespace only and v is a
// string.
func noValue(text string, v any) bool {
	if text == "" {
		return true
	}
	_, isString := v.(*string)
	return isString && strings.TrimLeft(text, xmlSpace) == ""
}

// ReportEmpty reports, when the decode reports the members sent without a
// value ([Tracker.ReportPresence]), the member whose element start opened, a
// child of the innermost open element, as sent empty. A hand-written decoder
// calls it once it has read that element to its end and found it to carry no
// value, for an element that is not a primitive element the tracker sees,
// such as a TERMINOLOGY_CODE `terminology_id` in its TERMINOLOGY_ID shape. A
// nilled element is reported as such as it closes, and nothing inside a
// nilled element is reported, so neither is reported here.
func ReportEmpty(dec *xml.Decoder, start xml.StartElement) {
	t := trackerOf(dec)
	if t == nil || t.presence == nil || isNilled(start.Attr) {
		return
	}
	n := len(t.open)
	if f := &t.open[n-1]; f.class == nil || f.nilled {
		return
	}
	t.sentMember(n, start.Name.Local, false)
}
