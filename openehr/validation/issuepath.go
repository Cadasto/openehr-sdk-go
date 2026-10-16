package validation

import (
	"strconv"
	"strings"

	tcimpl "github.com/cadasto/openehr-sdk-go/internal/templatecompile"
)

// issuepath.go: the one rule the walk uses to write the predicate of an
// item of a multi-valued attribute in [Issue.Path], with a template and
// without one. The template rules (walk_composition.go) and the RM rules
// (rmfloor.go) report at the paths the walk writes, so one node gets one
// path whichever of them reports it.

// pathItem is what the path rule reads of one item of a multi-valued
// attribute.
type pathItem struct {
	// id is the item's archetype_node_id (an archetype id on an archetype
	// root), or "" when the item carries none.
	id string
	// name is the name the OPT pins on the node the item binds to, or ""
	// when that node pins none or no OPT node binds the item. It never
	// comes from the item itself.
	name string
}

// predicateKind is how the path rule writes the predicate of an item.
type predicateKind uint8

const (
	// predicateNone writes no predicate: the item is written at the path of
	// its attribute. Only an OPT child that declares no id is written so.
	predicateNone predicateKind = iota
	// predicateID writes the item's id: [id].
	predicateID
	// predicateIDName writes the item's id with the pinned name that tells
	// it apart: [id, 'name'].
	predicateIDName
	// predicatePosition writes the item's 1-based position: [n].
	predicatePosition
)

// predicate is the predicate the path rule gives one item of a multi-valued
// attribute, held as the parts it is written from rather than as a string,
// so that it is written only into a path that is kept.
type predicate struct {
	// pathItem is what the rule read of the item.
	pathItem
	// kind is how the predicate is written.
	kind predicateKind
	// pos is the item's 1-based position among the items of its attribute,
	// which a positional predicate writes.
	pos int
}

// itemPaths returns the path of each item of the multi-valued attribute at
// attrPath, whose id and pinned name items holds, in order. It writes the
// predicate of each into items ([resolvePredicates]). An item is written:
//
//   - [id] when no other item carries its id;
//   - [id, 'name'] when another item shares the id and the item's pinned
//     name tells it apart from every item that shares it;
//   - [n], its 1-based position, otherwise: it has no id, or its id is
//     shared and its pinned name does not tell it apart.
func itemPaths(attrPath string, items []predicate) []string {
	resolvePredicates(items, true)
	out := make([]string, len(items))
	for i := range items {
		out[i] = items[i].pathBelow(attrPath)
	}
	return out
}

// attributePath returns the issue path of the attribute name of the node at
// the issue path parent: the root's attribute is "/name", any other
// parent's parent + "/name", built in one step.
func attributePath(parent, name string) string {
	if parent == "/" {
		return "/" + name
	}
	return parent + "/" + name
}

// optChildPaths returns, for each OPT child of the multi-valued attribute at
// attrPath, the path of the node it constrains: the path a node the instance
// lacks is reported at. No item exists to number, so a child is never
// written by a position. A child whose id another child shares, and whose
// pinned name does not tell it apart, keeps its id alone, and a child that
// declares no id has no predicate of its own and is written at attrPath.
func optChildPaths(attrPath string, children []*tcimpl.CompiledNode) []string {
	items := make([]predicate, len(children))
	for i, c := range children {
		items[i].pathItem = pathItem{id: optChildID(c), name: c.NodeName()}
	}
	resolvePredicates(items, false)
	out := make([]string, len(items))
	for i := range items {
		out[i] = items[i].pathBelow(attrPath)
	}
	return out
}

// optChildID is the id an OPT child pins on the node it constrains: the
// archetype id of an archetype root, else its at-code.
func optChildID(c *tcimpl.CompiledNode) string {
	if id := c.ArchetypeID(); id != "" {
		return id
	}
	return c.NodeID()
}

// resolvePredicates applies the path rule to the items of one multi-valued
// attribute, in order, whose id and pinned name items holds, and writes into
// each the predicate the rule gives it. numbered says whether the items
// exist in the instance and can be numbered: an item with no id, or with a
// shared id its pinned name does not tell apart, is then written by its
// position. When numbered is false (OPT children of a node the instance
// lacks), the first has no predicate and the second keeps its id alone.
func resolvePredicates(items []predicate, numbered bool) {
	s := newIDSharing(items)
	for i := range items {
		it := &items[i]
		it.pos = i + 1
		switch {
		case it.id == "" && numbered:
			it.kind = predicatePosition
		case it.id == "":
			it.kind = predicateNone
		case !s.shared(it.id):
			it.kind = predicateID
		case s.tellsApart(it.pathItem):
			it.kind = predicateIDName
		case numbered:
			it.kind = predicatePosition
		default:
			it.kind = predicateID
		}
	}
}

// pathBelow returns the path of the item p is the predicate of, below the
// path attrPath of its attribute, written in one step.
func (p *predicate) pathBelow(attrPath string) string {
	var b strings.Builder
	b.Grow(len(attrPath) + p.size())
	b.WriteString(attrPath)
	p.writeTo(&b)
	return b.String()
}

// size returns the length of the predicate as [predicate.writeTo] writes it.
func (p *predicate) size() int {
	switch p.kind {
	case predicateNone:
		return 0
	case predicateID:
		return len(p.id) + len("[]")
	case predicateIDName:
		return len(p.id) + escapedSize(p.name) + len("[, '']")
	case predicatePosition:
		return digits(p.pos) + len("[]")
	}
	return 0
}

// writeTo writes the predicate, brackets included, to b. An id stands as it
// is. A name sits in single quotes after the id; a backslash and a single
// quote inside it are each escaped by a backslash, so a backslash written
// for a quote is not escaped again. A position is written in decimal.
func (p *predicate) writeTo(b *strings.Builder) {
	switch p.kind {
	case predicateNone:
		return
	case predicateID:
		b.WriteByte('[')
		b.WriteString(p.id)
	case predicateIDName:
		b.WriteByte('[')
		b.WriteString(p.id)
		b.WriteString(", '")
		writeEscaped(b, p.name)
		b.WriteByte('\'')
	case predicatePosition:
		var buf [20]byte
		b.WriteByte('[')
		b.Write(strconv.AppendInt(buf[:0], int64(p.pos), 10))
	}
	b.WriteByte(']')
}

// escapedSize returns the length of name with each backslash and single
// quote escaped by a backslash, as [writeEscaped] writes it.
func escapedSize(name string) int {
	return len(name) + strings.Count(name, `\`) + strings.Count(name, "'")
}

// writeEscaped writes name to b with each backslash and single quote
// escaped by a backslash. Both are ASCII, so no byte of a multi-byte UTF-8
// sequence is one of them.
func writeEscaped(b *strings.Builder, name string) {
	for i := range len(name) {
		if c := name[i]; c == '\\' || c == '\'' {
			b.WriteByte('\\')
		}
		b.WriteByte(name[i])
	}
}

// digits returns the number of decimal digits of the positive n.
func digits(n int) int {
	d := 1
	for ; n >= 10; n /= 10 {
		d++
	}
	return d
}

// sharingScanMax is the longest list whose ids [idSharing] counts by
// scanning the list at each question rather than by counting them into maps
// once. The scan allocates nothing, but its cost grows with the square of
// the length, while the maps cost four allocations and grow with the
// length. Measured on items that each carry their own id, the case where
// the scan reads the whole list for every item, the two take about the same
// time at 12 items (BenchmarkIDSharing); at 16 the maps take a fifth less.
const sharingScanMax = 12

// idSharing counts, over the items of one attribute, how many carry each id
// and, for the ids that more than one item carries, how many carry each
// pinned name. A list of up to [sharingScanMax] items is counted by a scan
// of items at each question; a longer one is counted once, into perID and
// perName.
type idSharing struct {
	// items are the items, scanned when perID is nil.
	items []predicate
	// perID counts the items of each id, for a long list.
	perID map[string]int
	// perName counts the items of each shared id by their pinned name, for a
	// long list.
	perName map[pathItem]int
}

// newIDSharing counts the ids of items: by a scan at each question for a
// short list, which allocates nothing, and into maps for a long one.
func newIDSharing(items []predicate) idSharing {
	if len(items) <= sharingScanMax {
		return idSharing{items: items}
	}
	return countedIDSharing(items)
}

// countedIDSharing counts the ids of items into maps, once.
func countedIDSharing(items []predicate) idSharing {
	s := idSharing{perID: make(map[string]int, len(items))}
	for i := range items {
		if id := items[i].id; id != "" {
			s.perID[id]++
		}
	}
	for i := range items {
		it := items[i].pathItem
		if !s.shared(it.id) || it.name == "" {
			continue
		}
		if s.perName == nil {
			s.perName = map[pathItem]int{}
		}
		s.perName[it]++
	}
	return s
}

// shared reports whether more than one item carries id.
func (s idSharing) shared(id string) bool {
	if id == "" {
		return false
	}
	if s.perID != nil {
		return s.perID[id] > 1
	}
	n := 0
	for i := range s.items {
		if s.items[i].id == id {
			if n++; n > 1 {
				return true
			}
		}
	}
	return false
}

// tellsApart reports whether the pinned name of it, an item whose id is
// shared, tells it apart from every other item that carries its id: it has a
// name and no other such item binds to a node pinning the same one. A
// sibling bound to a node that pins no name does not carry that name either,
// since an item whose name a child pins binds to that child (see
// [matchChildByID]), so it does not stop the name from telling the item
// apart.
func (s idSharing) tellsApart(it pathItem) bool {
	if it.name == "" {
		return false
	}
	if s.perID != nil {
		return s.perName[it] == 1
	}
	n := 0
	for i := range s.items {
		if s.items[i].pathItem == it {
			if n++; n > 1 {
				return false
			}
		}
	}
	return n == 1
}
