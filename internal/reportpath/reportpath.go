// Package reportpath holds the paths the canonical decoders report: the JSON
// Pointer of a member and the path of an XML element. A path is a chain of
// nodes, each one segment and a pointer to the node of the segments before
// it, so the paths of the members of one object share the nodes of the
// object's own path. A decode makes a node only when it reports a path that
// needs it, and writes the path's text only when the caller asks for it, so a
// report costs the decode the segments not yet made, not the length of the
// path.
//
// A node never changes once it is shared, so a path stays valid, and may be
// read from any goroutine, for as long as the caller holds it. The one
// exception is [Node.SetToken], which a decode calls on a node it has not yet
// shared.
package reportpath

import "strconv"

// Node is the last segment of a path and the path before it. The nil *Node is
// the empty path: the root of the value.
type Node struct {
	parent *Node
	text   string
	n      int
	kind   kind
}

// kind is the form of a node's segment.
type kind uint8

const (
	// kindToken writes "/" and the text as one reference token of a JSON
	// Pointer (RFC 6901): "~" as "~0" and "/" as "~1".
	kindToken kind = iota
	// kindName writes "/" and the text as it is: an XML name, which holds
	// neither "~" nor "/".
	kindName
	// kindIndex writes "/" and the number in decimal.
	kindIndex
	// kindElement writes "/", the text, then the number in square brackets.
	kindElement
	// kindAttr writes "/@" and the text.
	kindAttr
	// kindText writes "/text()".
	kindText
)

// NodeBytes is what one node costs in memory on a 64-bit platform, beside
// the bytes of the text it holds: its own size, rounded up to the allocator's
// size class.
const NodeBytes = 48

// Token returns the path p followed by the JSON Pointer reference token s,
// escaped as RFC 6901 asks: "~" as "~0" and "/" as "~1".
func (p *Node) Token(s string) *Node {
	return &Node{parent: p, text: s, kind: kindToken}
}

// Name returns the path p followed by the XML name s, written as it is.
func (p *Node) Name(s string) *Node {
	return &Node{parent: p, text: s, kind: kindName}
}

// Index returns the path p followed by the position i, written in decimal.
func (p *Node) Index(i int) *Node {
	return &Node{parent: p, n: i, kind: kindIndex}
}

// Element returns the path p followed by the segment `/name[pos]` of an XML
// element at position pos among its siblings of that name.
func (p *Node) Element(name string, pos int) *Node {
	return &Node{parent: p, text: name, n: pos, kind: kindElement}
}

// Attr returns the path p followed by the segment `/@name` of an XML
// attribute.
func (p *Node) Attr(name string) *Node {
	return &Node{parent: p, text: name, kind: kindAttr}
}

// Text returns the path p followed by the segment `/text()`.
func (p *Node) Text() *Node {
	return &Node{parent: p, kind: kindText}
}

// SetToken sets the reference token of p, a node [Node.Token] made, to s. A
// decode calls it on a node it made before the token was known, and before
// it shares the node with anyone.
func (p *Node) SetToken(s string) {
	p.text = s
}

// Len returns the length of the path's text, in bytes.
func (p *Node) Len() int {
	n := 0
	for ; p != nil; p = p.parent {
		n += p.segmentLen()
	}
	return n
}

// String returns the path's text: "" for the empty path.
func (p *Node) String() string {
	if p == nil {
		return ""
	}
	return string(p.AppendText(make([]byte, 0, p.Len())))
}

// AppendText appends the path's text to b and returns the extended buffer. It
// grows b once, by the length of the text, and writes the segments into it
// from the last back to the first.
func (p *Node) AppendText(b []byte) []byte {
	b = append(b, make([]byte, p.Len())...)
	end := len(b)
	for ; p != nil; p = p.parent {
		i := end - p.segmentLen()
		p.appendSegment(b[i:i:end])
		end = i
	}
	return b
}

// segmentLen returns the length of p's own segment, in bytes.
func (p *Node) segmentLen() int {
	switch p.kind {
	case kindToken:
		n := 1 + len(p.text)
		for i := range len(p.text) {
			if c := p.text[i]; c == '~' || c == '/' {
				n++
			}
		}
		return n
	case kindName:
		return 1 + len(p.text)
	case kindIndex:
		return 1 + decimalLen(p.n)
	case kindElement:
		return 1 + len(p.text) + 2 + decimalLen(p.n)
	case kindAttr:
		return 2 + len(p.text)
	case kindText:
		return len(textSegment)
	}
	panic(errKind)
}

// textSegment is the segment [Node.Text] writes.
const textSegment = "/text()"

// appendSegment appends p's own segment to b and returns the extended buffer.
func (p *Node) appendSegment(b []byte) []byte {
	switch p.kind {
	case kindToken:
		b = append(b, '/')
		for i := range len(p.text) {
			switch c := p.text[i]; c {
			case '~':
				b = append(b, '~', '0')
			case '/':
				b = append(b, '~', '1')
			default:
				b = append(b, c)
			}
		}
		return b
	case kindName:
		return append(append(b, '/'), p.text...)
	case kindIndex:
		return strconv.AppendInt(append(b, '/'), int64(p.n), 10)
	case kindElement:
		b = append(append(b, '/'), p.text...)
		return append(strconv.AppendInt(append(b, '['), int64(p.n), 10), ']')
	case kindAttr:
		return append(append(b, '/', '@'), p.text...)
	case kindText:
		return append(b, textSegment...)
	}
	panic(errKind)
}

// errKind is the panic of a node whose kind is none of the kinds above: only
// the methods of this package make nodes, so no such node exists.
const errKind = "reportpath: a node of an unknown kind"

// decimalLen returns the length of n written in decimal.
func decimalLen(n int) int {
	var digits [20]byte
	return len(strconv.AppendInt(digits[:0], int64(n), 10))
}
