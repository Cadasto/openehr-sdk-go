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
// [Pointer.Parent] and [Pointer.Token] read a pointer one reference token at
// a time, from the last back to the root, without writing its text.
//
// A Pointer stays valid after the function it was passed to returns: it may
// be kept, and read from any goroutine. The zero Pointer is the root of the
// value, whose text is empty. Two Pointers compare equal with == only when
// they come from one report; compare their text, through [Pointer.String],
// to tell whether they name the same member.
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

// Parent returns the pointer of the object, list or Hash that holds p's
// member or item, and true. For the root it returns the zero Pointer and
// false.
//
// Parents are shared, not made: the members of one object have the object's
// own pointer as their parent, so within one decode the parents of two
// members of one object compare equal with ==. A caller can so gather the
// pointers of a report into one tree and meet each parent once. Parent
// writes none of the pointer's text.
func (p Pointer) Parent() (Pointer, bool) {
	node, ok := p.node.Parent()
	return Pointer{node: node}, ok
}

// Token returns p's last reference token, as RFC 6901 names it, unescaped:
// "~" and "/" stand for themselves, not "~0" and "~1". It is the name of an
// attribute, the key of a Hash entry, or the position of an item in a list,
// in decimal. For the root it returns "", as it does for an empty name or
// key; [Pointer.Parent] tells the two apart.
//
// Token writes none of the pointer's text, and allocates only for an item's
// position.
func (p Pointer) Token() string {
	return p.node.Last()
}
