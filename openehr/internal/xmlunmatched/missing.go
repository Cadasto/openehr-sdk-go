package xmlunmatched

import (
	"encoding/xml"
	"fmt"
	"slices"
	"strings"

	"github.com/cadasto/openehr-sdk-go/internal/reportpath"
)

// Class describes one class to the runtime: what its generated decoder reads
// and what the Reference Model requires of it. The generator emits one per
// class beside the class's UnmarshalXML, which hands it to [Enter]; a
// hand-written decoder declares its own.
type Class struct {
	// Name is the class's name in the Reference Model, such as DV_QUANTITY.
	Name string
	// Attrs names the members the decoder reads as XML attributes of the
	// start element, in no namespace, where the class's ITS-XML schema type
	// carries them so, such as archetype_node_id or an ARCHETYPE_TERM's code.
	// Every other attribute, bar the XML Schema instance ones, the namespace
	// declarations and the key attribute of a Hash entry, which [DecodeEntry]
	// takes off first, is unmatched content.
	Attrs []string
	// Required names the members the Reference Model makes mandatory on the
	// class and a document can carry, in the order the model declares them,
	// an ancestor's before the class's own. It leaves out the members no
	// serialization carries and the four interval boundary flags, which have
	// a value when absent. It holds at most 64 names ([maxRequired]): the
	// generator checks, and [Enter] refuses a description past it.
	Required []string
	// Items names the multi-valued members, whose child elements are the
	// items of one list: the n-th child element of such a name that the
	// decoder reads is the item at position n-1 in the canonical-JSON form of
	// the instance. A child of that local name the decoder skips, one in a
	// foreign namespace, is no item.
	Items []string
}

// maxRequired is the most names a [Class] can list in Required: a frame
// records the members it has seen in one word of that many bits.
const maxRequired = 64

// Enter is the entry of a class's decoder: start opens the element it
// decodes and c describes the class. It applies the decode's policy for
// unmatched content to the attributes of start the decoder does not read,
// as [CheckUnmatchedAttrs] does with c.Attrs as the known ones, and records c
// as the description of the element, so a decode that asks for the mandatory
// members the document leaves out ([Tracker.ReportMissing]) can report them
// as the element closes. start must be the token the decoder read last, and
// no child token may have been read since.
//
// By default it does nothing. A description with more than [maxRequired]
// names in its Required is a programming error, and Enter panics on it
// before any decode, tracked or not: a frame records the members it has
// seen in one word of that many bits, and a name past it would be reported
// missing whatever the document carries.
func Enter(dec *xml.Decoder, start xml.StartElement, c *Class) error {
	if len(c.Required) > maxRequired {
		panic(fmt.Sprintf("xmlunmatched.Enter: class %s lists %d required members, more than the %d a description holds", c.Name, len(c.Required), maxRequired))
	}
	t := trackerOf(dec)
	if t == nil {
		return checkRepeatedAttr(&start)
	}
	return t.enter(start, c)
}

// enter records c on the innermost open element, the one start opened, marks
// the members present as XML attributes of start, and checks the attributes.
func (t *Tracker) enter(start xml.StartElement, c *Class) error {
	f := &t.open[len(t.open)-1]
	f.class = c
	if t.missing != nil {
		for _, a := range start.Attr {
			if a.Name.Space != "" || !slices.Contains(c.Attrs, a.Name.Local) {
				continue
			}
			if i := slices.Index(c.Required, a.Name.Local); i >= 0 {
				f.seen |= 1 << i
			}
		}
	}
	return t.attrs(start.Attr, c.Attrs)
}

// ReportMissing makes the decodes of t pass every mandatory member a document
// leaves out to fn, once each, as the element that should hold the member
// closes: its class's members in the class's order, so the members of an
// inner element come before those of the elements around it. Each is named
// by its JSON Pointer in the canonical-JSON form of the value being decoded,
// such as `/content/1/data/events/0/time`; a member of the root is `/name`.
//
// A member is left out when the element has no child element of that name
// and no XML attribute of it among the class's attribute members; when its
// child carries `xsi:nil="true"`; or when its child is a primitive element
// with no text, or a string element whose text is XML whitespace only,
// unless the decode keeps the string's text ([Tracker.KeepStringText]). A
// Character of one space is a value. Nothing inside a nilled
// element is reported: the member is reported once, by its parent, as a
// null is in canonical JSON. A struct element with no content is a value,
// and so is an entry of a Hash member, whatever its text, and an element the
// codec does not model, a Hash of Hashes. A member inside a
// Hash entry is named under the entry's key, such as
// `/description/details/en/purpose`. The report is the whole policy: no
// missing member refuses the decode, strict or not. A nil fn asks for
// nothing.
func (t *Tracker) ReportMissing(fn func(pointer *reportpath.Node)) {
	t.missing = fn
}

// ReportPresence makes the decodes of t pass every member a document sends
// without a value to fn, once each, as the member's element closes, so in
// document order: a member of a class whose element carries `xsi:nil="true"`
// (or `1`), nilled set, or is a primitive element with no text, or a string
// element whose text is XML whitespace only, mandatory or optional alike; a
// string whose text the decode keeps ([Tracker.KeepStringText]) included. Each is named by its JSON
// Pointer, as [Tracker.ReportMissing] names a member, such as
// `/content/0/data/events/0/data/items/0/value/precision`. Nothing inside a
// nilled element is reported. An element the decoder skips or drops, such as
// unmatched content, a repeat of a single-valued member or an element in a
// foreign namespace, is no member, and neither is the root. A struct element
// with no content is a value, and so is an entry of a Hash member, whatever
// its text, unless it is nilled. The report is the whole policy: nothing
// refuses the decode, strict or not. A nil fn asks for nothing.
func (t *Tracker) ReportPresence(fn func(pointer *reportpath.Node, nilled bool)) {
	t.presence = fn
}

// checksPresence reports whether the decode follows what each element
// carries: it reports the mandatory members a document leaves out, or the
// members it sends without a value.
func (t *Tracker) checksPresence() bool {
	return t.missing != nil || t.presence != nil
}

// skipping marks the innermost open element, which the decoder is about to
// skip or drop, as no member of its parent: it gives no member a value
// ([Tracker.closing]) and is not a member sent without one.
func (t *Tracker) skipping() {
	t.open[len(t.open)-1].unread = true
}

// closing reports, when the decode asked, the mandatory members the element
// whose frame is t.open[n-1] carried no value for, and then, when the
// element is a member of its parent's class, reports it when it carries no
// value and the decode asked, or marks it as a value of its parent's member
// of that name. It runs as the element closes, before its frame is dropped.
func (t *Tracker) closing(n int) {
	f := &t.open[n-1]
	if t.missing != nil && f.class != nil && !f.nilled {
		for i, name := range f.class.Required {
			if f.seen&(1<<i) != 0 || slices.Contains(f.optional, name) {
				continue
			}
			t.missingMember(n, name)
		}
	}
	p := &t.open[n-2]
	if p.class == nil {
		return
	}
	if f.unread {
		// A skipped child named like a list member is no item of it, so it
		// moves the positions of the items after it ([Tracker.pointerNode]).
		if i := slices.Index(p.class.Items, f.name); i >= 0 {
			p.skip(i, len(p.class.Items))
		}
		return
	}
	if f.nilled || (f.primitive && !f.hasText) {
		// Only the outermost nilled element is sent without a value. An
		// entry of a Hash member is a value whatever its text: its decoder
		// does not read it as a primitive ([decodeEntryValue]).
		if t.presence != nil && !p.nilled {
			t.sentMember(n, "", f.nilled)
		}
		// A string whose text the decode keeps is sent empty and still
		// carries a value, as the empty string does in canonical JSON.
		if !f.kept {
			return
		}
	}
	if i := slices.Index(p.class.Required, f.name); i >= 0 {
		p.seen |= 1 << i
	}
}

// sentMember passes to the decode's hook the pointer of a member the document
// sends without a value: the element whose frame is t.open[n-1], or its
// member name when name is not empty. While a Hash entry's key is unknown it
// holds the pointer instead ([Tracker.holdEntry]).
func (t *Tracker) sentMember(n int, name string, nilled bool) {
	p := t.memberPointer(n, name)
	if t.held != nil {
		*t.held = append(*t.held, heldItem{path: p, sent: true, nilled: nilled})
		return
	}
	t.presence(p, nilled)
}

// missingMember passes to the decode's hook the pointer of the mandatory
// member name that the element whose frame is t.open[n-1] leaves out. While a
// Hash entry's key is unknown it holds the pointer instead
// ([Tracker.holdEntry]).
func (t *Tracker) missingMember(n int, name string) {
	p := t.memberPointer(n, name)
	if t.held != nil {
		*t.held = append(*t.held, heldItem{path: p, missing: true})
		return
	}
	t.missing(p)
}

// memberPointer returns the JSON Pointer, in the canonical-JSON form of the
// value, of the element whose frame is t.open[n-1], followed by `/` and name
// when name is not empty.
func (t *Tracker) memberPointer(n int, name string) *reportpath.Node {
	p := t.pointerPath(n)
	if name != "" {
		p = p.Name(name)
	}
	return p
}

// pointerPath returns the JSON Pointer, in the canonical-JSON form of the
// value, of the element whose frame is t.open[n-1]: nil, the empty pointer,
// for the root element, and the nodes of one token per element below it
// ([Tracker.pointerNode]). It makes the nodes of the frames that have none
// yet, those after the innermost frame below the root that has one, and keeps
// each in its frame until the element closes.
func (t *Tracker) pointerPath(n int) *reportpath.Node {
	j := n - 1
	for j >= 2 && t.open[j].ptr == nil {
		j--
	}
	var p *reportpath.Node
	if j >= 2 {
		p = t.open[j].ptr
	}
	for j = max(j+1, 2); j < n; j++ {
		p = t.pointerNode(j, p)
		t.open[j].ptr = p
	}
	return p
}

// pointerNode returns the pointer of the element whose frame is t.open[k],
// k >= 2, given parent, the pointer of the element around it: `/` and its
// local name, then `/` and its key, escaped as RFC 6901 asks, when it is one
// entry of a Hash member, or else `/` and its 0-based position among the
// same-named siblings the decoder read when it is one item of a multi-valued
// member. An item's position is its position in the document less the earlier
// siblings of its name the decoder skipped, every one of which has closed.
//
// While a Hash entry is held, an entry's key may lie in its value, still to
// be read: the node is then made without it and given it when the hold ends
// ([Tracker.holdEntry]).
func (t *Tracker) pointerNode(k int, parent *reportpath.Node) *reportpath.Node {
	f := &t.open[k]
	p := parent.Name(f.name)
	if f.key != nil {
		if t.held == nil {
			return p.Token(f.key())
		}
		p = p.Token("")
		t.pending = append(t.pending, pendingKey{node: p, key: f.key})
		return p
	}
	if up := &t.open[k-1]; up.class != nil {
		if i := slices.Index(up.class.Items, f.name); i >= 0 {
			return p.Index(f.pos - 1 - up.skippedAt(i))
		}
	}
	return p
}

// isNilled reports whether attrs carries `xsi:nil="true"`: an attribute
// named nil in the resolved XML Schema instance namespace or written
// literally as `xsi:nil`, whose value is an XML Schema boolean true, `true`
// or `1`.
func isNilled(attrs []xml.Attr) bool {
	for _, a := range attrs {
		if (a.Name.Space == NSXSI && a.Name.Local == "nil") || a.Name.Local == "xsi:nil" {
			v := strings.TrimSpace(a.Value)
			return v == "true" || v == "1"
		}
	}
	return false
}
