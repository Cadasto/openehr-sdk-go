package canjson

import "github.com/cadasto/openehr-sdk-go/internal/reportpath"

// Pointer is the JSON Pointer (RFC 6901) of a member from the root of the
// value, as [WithUnmatched] and [WithMissing] report it, such as
// "/content/0/data/x-note". It carries the member's name, never its value.
//
// A Pointer writes its text only when asked, through [Pointer.String] or
// [Pointer.AppendText]. The pointers of the members of one object share the
// pointer of the object, so reporting many members of a deep document costs
// the decode memory in proportion to the document, and a caller that only
// counts or samples the members pays nothing for the text of the others.
//
// A Pointer stays valid after the function it was passed to returns: it may
// be kept, and read from any goroutine. The zero Pointer is the root of the
// value, whose text is empty.
type Pointer struct {
	node *reportpath.Node
}

// String returns the pointer's text, such as "/content/0/data/x-note".
func (p Pointer) String() string {
	return p.node.String()
}

// AppendText appends the pointer's text to b and returns the extended
// buffer, as [encoding.TextAppender] asks. It never fails: the error is
// always nil.
func (p Pointer) AppendText(b []byte) ([]byte, error) {
	return p.node.AppendText(b), nil
}
