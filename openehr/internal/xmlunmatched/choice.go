package xmlunmatched

import (
	"encoding/xml"
	"slices"
)

// Shape is one of the shapes the element of a choice member takes: the child
// elements the shape's schema type declares, which of the members the class
// that reads it requires the shape makes optional, and how to allocate a value
// of it. The generator emits one list of shapes per choice member.
type Shape struct {
	// Elements names the child elements of the shape, by local name.
	Elements []string
	// Optional names the members the class of the shape requires that the
	// shape's schema type makes optional, such as the `value` of the
	// DV_CODED_TEXT shape of an AOM 1.4 ORDINAL symbol: the missing-member
	// report ([Tracker.ReportMissing]) does not report them.
	Optional []string
	// New returns a pointer to a new value of the shape, whose UnmarshalXML
	// reads the element.
	New func() any
}

// DecodeChoice decodes the element start opens, whose content takes one of
// shapes, and returns the pointer to the value of the shape it took. The
// element's first child element chooses: the shape is the first of shapes
// whose Elements names that child, in no namespace or the openEHR one. An
// element with no child element, or whose first child no shape names,
// decodes as the first shape, so what it holds is that shape's unmatched
// content. start must be the token the decoder read last.
//
// The shape's decoder reads the element as if called on it directly: in a
// tracked decode its unmatched content and its missing members are reported
// at the element's path, and its failures are its own. A member the shape
// makes optional ([Shape.Optional]) is not reported missing.
func DecodeChoice(dec *xml.Decoder, start xml.StartElement, shapes []Shape) (any, error) {
	if len(shapes) == 0 {
		return nil, SkipUnmatched(dec, start)
	}
	if t := trackerOf(dec); t != nil && t.reader != nil {
		// The tracked decoder reads its tokens through t.reader, which can
		// read ahead without recording: the first child is then still to
		// come, so the shape's decoder enters the element itself.
		first, err := t.reader.peekChild()
		if err != nil {
			return nil, err
		}
		shape := chooseShape(shapes, first)
		t.open[len(t.open)-1].optional = shape.Optional
		v := shape.New()
		return v, dec.DecodeElement(v, &start)
	}
	// An untracked decoder cannot read ahead: the tokens up to the first
	// child are read, then handed to the shape's decoder again, before the
	// rest, through a decoder of their own.
	start = start.Copy()
	toks := []xml.Token{start}
	var first xml.Name
	for {
		tok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		tok = xml.CopyToken(tok)
		toks = append(toks, tok)
		if tk, ok := tok.(xml.StartElement); ok {
			first = tk.Name
			break
		}
		if _, ok := tok.(xml.EndElement); ok {
			break
		}
	}
	replay := xml.NewTokenDecoder(&replayReader{toks: toks, src: dec})
	if _, err := replay.Token(); err != nil { // start, so the replay decoder holds it open
		return nil, err
	}
	v := chooseShape(shapes, first).New()
	return v, replay.DecodeElement(v, &start)
}

// chooseShape returns the first of shapes whose elements name first, the
// first child of a choice member's element, or the first shape when none does.
func chooseShape(shapes []Shape, first xml.Name) Shape {
	if local := LocalName(first); local != "" {
		for _, s := range shapes {
			if slices.Contains(s.Elements, local) {
				return s
			}
		}
	}
	return shapes[0]
}

// peekChild reads the source ahead, without recording what it reads, to the
// first child element of the element the decoder opened last, or to that
// element's end, and keeps every token it reads for [trackingReader.Token]
// to return and record in order. It returns the name of that child, or the
// zero name when the element has none. Tokens read ahead earlier and not yet
// returned come first.
func (r *trackingReader) peekChild() (xml.Name, error) {
	for _, tok := range r.pending {
		switch tk := tok.(type) {
		case xml.StartElement:
			return tk.Name, nil
		case xml.EndElement:
			return xml.Name{}, nil
		}
	}
	for {
		tok, err := r.src.Token()
		if err != nil {
			return xml.Name{}, err
		}
		tok = xml.CopyToken(tok)
		r.pending = append(r.pending, tok)
		switch tk := tok.(type) {
		case xml.StartElement:
			return tk.Name, nil
		case xml.EndElement:
			return xml.Name{}, nil
		}
	}
}

// replayReader is a token source that returns a fixed list of tokens, then
// the tokens of src.
type replayReader struct {
	toks []xml.Token
	src  *xml.Decoder
}

// Token returns the next token of the list, or of src once the list is done.
func (r *replayReader) Token() (xml.Token, error) {
	if len(r.toks) > 0 {
		tok := r.toks[0]
		r.toks = r.toks[1:]
		return tok, nil
	}
	return r.src.Token()
}
