package xmlunmatched

import (
	"bytes"
	"unicode/utf8"
)

// doctypeWellFormed reports whether d, a `<!…>` declaration before the root
// element as written, from its `<!` to its `>`, is a document type
// declaration XML 1.0's grammar allows
// (https://www.w3.org/TR/xml/#NT-doctypedecl), which encoding/xml hands on
// unjudged: `<!DOCTYPE`, whitespace and a name; then, after whitespace, an
// optional external id, SYSTEM and a system literal or PUBLIC, a public id
// literal and a system literal, each after whitespace; then optional
// whitespace, an optional internal subset in brackets followed by optional
// whitespace, and `>`.
//
// The internal subset holds only whitespace, parameter-entity references
// (`%name;`), comments, processing instructions whose target is a name other
// than xml in any letter case, and markup declarations: `<!ELEMENT`,
// `<!ATTLIST`, `<!ENTITY` (with `%` and whitespace for a parameter entity) or
// `<!NOTATION`, whitespace and a name, then anything up to the `>` outside
// its quoted literals but `<`. What a markup declaration holds after its name
// is not judged further.
func doctypeWellFormed(d []byte) bool {
	s := dtdScanner{b: d}
	if !s.lit("<!DOCTYPE") || !s.space() || !s.name() {
		return false
	}
	if s.space() {
		switch {
		case s.lit("SYSTEM"):
			if !s.space() || !s.quoted(false) {
				return false
			}
		case s.lit("PUBLIC"):
			if !s.space() || !s.quoted(true) || !s.space() || !s.quoted(false) {
				return false
			}
		}
		s.space()
	}
	if s.lit("[") {
		if !s.intSubset() {
			return false
		}
		s.space()
	}
	return s.lit(">") && s.i == len(s.b)
}

// dtdScanner reads a document type declaration, b, from offset i on. Each
// method reports whether what it reads stands at i and moves i past it when
// it does; what moves i on a false return reads nothing further.
type dtdScanner struct {
	b []byte
	i int
}

// lit reads p.
func (s *dtdScanner) lit(p string) bool {
	if !bytes.HasPrefix(s.b[s.i:], []byte(p)) {
		return false
	}
	s.i += len(p)
	return true
}

// space reads one or more whitespace characters
// (https://www.w3.org/TR/xml/#NT-S).
func (s *dtdScanner) space() bool {
	n := len(s.b[s.i:]) - len(bytes.TrimLeft(s.b[s.i:], " \t\r\n"))
	s.i += n
	return n > 0
}

// name reads an XML name (https://www.w3.org/TR/xml/#NT-Name).
func (s *dtdScanner) name() bool {
	start := s.i
	for s.i < len(s.b) {
		r, n := utf8.DecodeRune(s.b[s.i:])
		if (r == utf8.RuneError && n < 2) || (!isNameStartChar(r) && (s.i == start || !isNameChar(r))) {
			// A byte that is no UTF-8 decodes as U+FFFD, which a name
			// may hold as written but not in its place.
			break
		}
		s.i += n
	}
	return s.i > start
}

// quoted reads a literal in single or double quotes: a public id literal,
// whose characters PubidChar allows (https://www.w3.org/TR/xml/#NT-PubidLiteral),
// when pubid is set, and a system literal of any characters otherwise.
func (s *dtdScanner) quoted(pubid bool) bool {
	if s.i >= len(s.b) || (s.b[s.i] != '"' && s.b[s.i] != '\'') {
		return false
	}
	q := s.b[s.i]
	end := bytes.IndexByte(s.b[s.i+1:], q)
	if end < 0 {
		return false
	}
	if pubid {
		for _, c := range s.b[s.i+1 : s.i+1+end] {
			if !isPubidChar(c) {
				return false
			}
		}
	}
	s.i += end + 2
	return true
}

// intSubset reads an internal subset after its `[`, up to and past its `]`
// (https://www.w3.org/TR/xml/#NT-intSubset).
func (s *dtdScanner) intSubset() bool {
	for {
		s.space()
		switch {
		case s.lit("]"):
			return true
		case s.lit("%"):
			if !s.name() || !s.lit(";") {
				return false
			}
		case s.lit("<!--"):
			if !s.comment() {
				return false
			}
		case s.lit("<?"):
			if !s.procInst() {
				return false
			}
		case s.lit("<!"):
			if !s.markupDecl() {
				return false
			}
		default:
			return false
		}
	}
}

// comment reads the rest of a comment after its `<!--`: text up to the first
// `--`, which must be followed by `>` (https://www.w3.org/TR/xml/#NT-Comment).
func (s *dtdScanner) comment() bool {
	end := bytes.Index(s.b[s.i:], []byte("--"))
	if end < 0 || !bytes.HasPrefix(s.b[s.i+end:], []byte("-->")) {
		return false
	}
	s.i += end + len("-->")
	return true
}

// procInst reads the rest of a processing instruction after its `<?`: a
// target, a name other than xml in any letter case, then `?>`, or whitespace
// and text up to the first `?>` (https://www.w3.org/TR/xml/#NT-PI).
func (s *dtdScanner) procInst() bool {
	start := s.i
	if !s.name() || isXMLTarget(string(s.b[start:s.i])) {
		return false
	}
	if s.lit("?>") {
		return true
	}
	if !s.space() {
		return false
	}
	end := bytes.Index(s.b[s.i:], []byte("?>"))
	if end < 0 {
		return false
	}
	s.i += end + len("?>")
	return true
}

// markupDecls are the keywords of the markup declarations an internal subset
// holds after `<!` (https://www.w3.org/TR/xml/#NT-markupdecl).
var markupDecls = []string{"ELEMENT", "ATTLIST", "ENTITY", "NOTATION"}

// markupDecl reads the rest of a markup declaration after its `<!`: its
// keyword, whitespace, for a parameter entity `%` and whitespace, and a
// name, then anything but `<` up to and past the first `>` outside a quoted
// literal.
func (s *dtdScanner) markupDecl() bool {
	keyword := ""
	for _, k := range markupDecls {
		if s.lit(k) {
			keyword = k
			break
		}
	}
	if keyword == "" || !s.space() {
		return false
	}
	if keyword == "ENTITY" && s.lit("%") && !s.space() {
		return false
	}
	if !s.name() {
		return false
	}
	for s.i < len(s.b) {
		switch c := s.b[s.i]; c {
		case '"', '\'':
			end := bytes.IndexByte(s.b[s.i+1:], c)
			if end < 0 {
				return false
			}
			s.i += end + 2
		case '>':
			s.i++
			return true
		case '<':
			return false
		default:
			s.i++
		}
	}
	return false
}

// isNameStartChar reports whether r may start an XML name
// (https://www.w3.org/TR/xml/#NT-NameStartChar).
func isNameStartChar(r rune) bool {
	switch {
	case r == ':' || r == '_' || ('A' <= r && r <= 'Z') || ('a' <= r && r <= 'z'):
		return true
	case r < 0xC0:
		return false
	}
	return (r <= 0xD6) || (0xD8 <= r && r <= 0xF6) || (0xF8 <= r && r <= 0x2FF) ||
		(0x370 <= r && r <= 0x37D) || (0x37F <= r && r <= 0x1FFF) || (0x200C <= r && r <= 0x200D) ||
		(0x2070 <= r && r <= 0x218F) || (0x2C00 <= r && r <= 0x2FEF) || (0x3001 <= r && r <= 0xD7FF) ||
		(0xF900 <= r && r <= 0xFDCF) || (0xFDF0 <= r && r <= 0xFFFD) || (0x10000 <= r && r <= 0xEFFFF)
}

// isNameChar reports whether r may stand in an XML name after its first
// character (https://www.w3.org/TR/xml/#NT-NameChar).
func isNameChar(r rune) bool {
	return isNameStartChar(r) || r == '-' || r == '.' || ('0' <= r && r <= '9') || r == 0xB7 ||
		(0x300 <= r && r <= 0x36F) || (0x203F <= r && r <= 0x2040)
}

// isPubidChar reports whether c may stand in a public id literal
// (https://www.w3.org/TR/xml/#NT-PubidChar).
func isPubidChar(c byte) bool {
	switch {
	case c == ' ' || c == '\r' || c == '\n':
		return true
	case ('a' <= c && c <= 'z') || ('A' <= c && c <= 'Z') || ('0' <= c && c <= '9'):
		return true
	}
	return bytes.IndexByte([]byte("-'()+,./:=?;!*#@$_%"), c) >= 0
}
