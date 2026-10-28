package xmlunmatched

import (
	"encoding/xml"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// Class describes one class to the runtime: what its generated decoder reads
// and what the Reference Model requires of it. The generator emits one per
// class beside the class's UnmarshalXML, which hands it to [Enter]; a
// hand-written decoder declares its own.
type Class struct {
	// Name is the class's name in the Reference Model, such as DV_QUANTITY.
	Name string
	// Attrs names the members the decoder reads as XML attributes of the
	// start element, in no namespace, where the ITS-XML schema carries them
	// so: archetype_node_id today. Every other attribute, bar the XML Schema
	// instance ones and the namespace declarations, is unmatched content.
	Attrs []string
	// Required names the members the Reference Model makes mandatory on the
	// class and a document can carry, in the order the model declares them,
	// an ancestor's before the class's own. It leaves out the members no
	// serialization carries and the four interval boundary flags, which have
	// a value when absent. It holds at most 64 names ([maxRequired]): the
	// generator checks, and [Enter] refuses a description past it.
	Required []string
	// Items names the multi-valued members, whose child elements are the
	// items of one list: the n-th child element of such a name is the item
	// at position n-1 in the canonical-JSON form of the instance.
	Items []string
	// Elements names each member the decoder also reads from a child element
	// of another name, the one the ITS-XML schema declares in its place:
	// an element of a class whose own members hold the member's value, such
	// as C_CODED_TEXT's terminology, a string written as the TERMINOLOGY_ID
	// element terminology_id whose value holds it. Such an element gives the
	// member a value when it carries every mandatory member of its own
	// class. When it does not, the member is missing, named by its own
	// pointer, and nothing inside the element is reported: the canonical-JSON
	// form has no member of the element's name. A child element of the
	// member's own name counts as before.
	Elements []Element
	// Inline names each child element whose own members are the class's,
	// read in their place: the `defining_code` of an ORDINAL's symbol
	// written as a DV_CODED_TEXT, a CODE_PHRASE whose terminology_id and
	// code_string are the symbol's own. Such an element is no member of the
	// class. Each mandatory member of its own class it carries a value for
	// gives the class's member of that name a value, it reports none of its
	// own missing, and the pointer of a member inside it leaves its name
	// out: the canonical-JSON form of the instance has no member of that
	// name.
	Inline []string
}

// Element names the child element a member of a [Class] is read from when
// its name is not the member's ([Class.Elements]).
type Element struct {
	// Member is the member's name in the Reference Model, such as terminology.
	Member string
	// Name is the element's local name, such as terminology_id.
	Name string
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
		return nil
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
// whose text is empty or XML whitespace only. Nothing inside a nilled
// element is reported: the member is reported once, by its parent, as a
// null is in canonical JSON. A struct element with no content is a value,
// and so is a Hash or Any element the codec does not model. A member the
// class also reads from an element of another name ([Class.Elements]) has a
// value when that element carries every member its own class requires;
// otherwise the member is reported, by its own name, and nothing inside the
// element is. An element the class reads its own members from
// ([Class.Inline]) gives each of them the value it carries, and the members
// inside it are named as the class's own. The report is the whole policy: no
// missing member refuses the decode, strict or not. A nil fn asks for
// nothing.
func (t *Tracker) ReportMissing(fn func(pointer string)) {
	t.missing = fn
}

// closing reports, when the decode asked, the mandatory members the element
// whose frame is t.open[n-1] carried no value for, and then marks the
// element as a value of its parent's member of that name when it carried
// one. An element the parent's class reads in place of a member of another
// name ([Class.Elements]) reports nothing of its own, and is a value of that
// member when it carries every member its own class requires. An element the
// parent reads its own members from ([Class.Inline]) reports nothing of its
// own either, and gives each member of the parent's class it carries a value
// for that value. It runs as the element closes, before its frame is dropped.
func (t *Tracker) closing(n int) {
	f := &t.open[n-1]
	p := &t.open[n-2]
	if p.inlines(f) {
		if f.class != nil && !f.nilled {
			for i, name := range f.class.Required {
				if f.seen&(1<<i) == 0 {
					continue
				}
				if j := slices.Index(p.class.Required, name); j >= 0 {
					p.seen |= 1 << j
				}
			}
		}
		return
	}
	member, inPlace := f.name, false
	if p.class != nil {
		member, inPlace = p.class.memberOf(f.name)
	}
	if f.class != nil && !f.nilled && !inPlace {
		var b strings.Builder
		for i, name := range f.class.Required {
			if f.seen&(1<<i) != 0 {
				continue
			}
			b.Reset()
			t.writePointer(&b, n)
			b.WriteByte('/')
			b.WriteString(name)
			t.missing(b.String())
		}
	}
	if p.class == nil || f.nilled || (f.primitive && !f.hasText) {
		return
	}
	if inPlace && f.class != nil && f.seen != f.class.allRequired() {
		return
	}
	if i := slices.Index(p.class.Required, member); i >= 0 {
		p.seen |= 1 << i
	}
}

// memberOf returns the member a child element named local is a value of:
// the member c reads in its place ([Class.Elements]) and true, or else the
// name itself and false.
func (c *Class) memberOf(local string) (string, bool) {
	for _, e := range c.Elements {
		if e.Name == local {
			return e.Member, true
		}
	}
	return local, false
}

// allRequired returns the seen word of an element that carries every member
// c requires: one bit per name in c.Required.
func (c *Class) allRequired() uint64 {
	return ^uint64(0) >> (maxRequired - len(c.Required))
}

// inlines reports whether f is the frame of a child element of p's element
// that p's class reads its own members from ([Class.Inline]).
func (p *frame) inlines(f *frame) bool {
	return p.class != nil && len(p.class.Inline) > 0 && slices.Contains(p.class.Inline, f.name)
}

// writePointer writes the JSON Pointer of the element whose frame is
// t.open[n-1] in the canonical-JSON form of the value: nothing for the root
// element, and for each element below it `/` and its local name, followed by
// `/` and its 0-based position among its same-named siblings when the
// parent's class lists the name among its multi-valued members. An element
// its parent's class reads its own members from ([Class.Inline]) adds
// nothing.
func (t *Tracker) writePointer(b *strings.Builder, n int) {
	for k := 2; k < n; k++ {
		f := &t.open[k]
		p := &t.open[k-1]
		if p.inlines(f) {
			continue
		}
		b.WriteByte('/')
		b.WriteString(f.name)
		if p.class != nil && slices.Contains(p.class.Items, f.name) {
			b.WriteByte('/')
			b.WriteString(strconv.Itoa(f.pos - 1))
		}
	}
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
