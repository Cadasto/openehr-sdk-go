package canxml

import "github.com/cadasto/openehr-sdk-go/internal/reportpath"

// Path is the path of an unmatched item from the root of the value, as
// [WithUnmatched] reports it: one `/name[n]` segment per element, then
// `/@name` for an attribute or `/text()` for text, such as
// "/composition[1]/content[2]/x-note[1]". It carries names only, never a
// value from the input.
//
// A Path writes its text only when asked, through [Path.String] or
// [Path.AppendText]. The paths of the items inside one element share the
// path of the element, so reporting many items of a deep document costs the
// decode memory in proportion to the document, and a caller that only counts
// or samples the items pays nothing for the text of the others.
//
// A Path stays valid after the function it was passed to returns: it may be
// kept, and read from any goroutine. The zero Path is the root of the value,
// whose text is empty. Two Paths compare equal with == only when they come
// from one report; compare their text, through [Path.String], to tell
// whether they name the same item.
type Path struct {
	node *reportpath.Node
}

// String returns the path's text, such as "/composition[1]/@x-flag".
func (p Path) String() string {
	return p.node.String()
}

// AppendText appends the path's text to b and returns the extended buffer,
// as [encoding.TextAppender] asks. It never fails: the error is always nil.
func (p Path) AppendText(b []byte) ([]byte, error) {
	return p.node.AppendText(b), nil
}

// Pointer is the JSON Pointer (RFC 6901) of a member in the canonical-JSON
// form of the instance, from the root of the value, as [WithMissing] and
// [WithPresence] report it, such as
// "/content/0/data/events/0/data/items/0/value/magnitude". It carries the
// member's name, never a value from the input.
//
// A Pointer writes its text only when asked, through [Pointer.String] or
// [Pointer.AppendText]. The pointers of the members of one element share
// the pointer of the element, so reporting many members of a deep document
// costs the decode memory in proportion to the document, and a caller that
// only counts or samples the members pays nothing for the text of the
// others. [Pointer.Parent] and [Pointer.Token] read a pointer one reference
// token at a time, from the last back to the root, without writing its text.
//
// A Pointer stays valid after the function it was passed to returns: it may
// be kept, and read from any goroutine. The zero Pointer is the root of the
// value, whose text is empty.
//
// Two Pointers compare equal with == only when they come from one report:
// a member reported by two options gives two values with the same text.
// Compare their text, through [Pointer.String], to tell whether they name
// the same member.
type Pointer struct {
	node *reportpath.Node
}

// String returns the pointer's text, such as "/description/details/en/purpose".
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
// member or item, in the canonical-JSON form of the instance, and true. For
// the root it returns the zero Pointer and false.
//
// Parents are shared, not made: the members of one element have the
// element's own pointer as their parent, so within one decode the parents of
// two members of one element compare equal with ==, whichever option
// reported them. A caller can so gather the pointers of a report into one
// tree and meet each parent once. Parent writes none of the pointer's text.
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

// reportUnmatched returns the tracker's hook for fn, a [WithUnmatched]
// function, or nil for a nil fn.
func reportUnmatched(fn func(p Path)) func(*reportpath.Node) {
	if fn == nil {
		return nil
	}
	return func(node *reportpath.Node) { fn(Path{node: node}) }
}

// reportMissing returns the tracker's hook for fn, a [WithMissing]
// function, or nil for a nil fn.
func reportMissing(fn func(p Pointer)) func(*reportpath.Node) {
	if fn == nil {
		return nil
	}
	return func(node *reportpath.Node) { fn(Pointer{node: node}) }
}
