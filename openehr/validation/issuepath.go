package validation

import (
	"strconv"
	"strings"

	tcimpl "github.com/cadasto/openehr-sdk-go/internal/templatecompile"
)

// issuepath.go: the one rule the walk uses to write the predicate of an
// item of a multi-valued attribute in [Issue.Path], with a template and
// without one, and the handle the walk carries for the path of each node
// and attribute it visits, which writes the path only when something keeps
// it. The template rules (walk_composition.go) and the RM rules (rmfloor.go)
// report at the paths the walk writes, so one node gets one path whichever
// of them reports it.

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
// so that it is written only into a path that is kept ([issuePath]).
type predicate struct {
	// pathItem is what the rule read of the item.
	pathItem
	// kind is how the predicate is written.
	kind predicateKind
	// pos is the item's 1-based position among the items of its attribute,
	// which a positional predicate writes.
	pos int
}

// issuePath is the issue path of a node or an attribute the walk visits,
// held as the number of steps of the walk's path stack it spans rather than
// as a string: the root's path when n is 0, the first n steps below the root
// otherwise. The walk writes the string only when something keeps the path
// ([issuePath.String]), so that the paths of the nodes nothing reports at
// are never written.
//
// A handle is valid only during the visit that pushed its last step. The
// walk replaces the step at a depth when it moves on to the next attribute
// or item there ([pathStack.attribute], [pathStack.item]), so a handle kept
// past its visit would read the steps of another node. Whatever keeps a path
// keeps the string, written when it is recorded: an issue, a refusal, a stop
// of the walk, the path of a CONSTRAINT_REF.
type issuePath struct {
	// stack is the walk's path stack; nil on a root's path, which spans no
	// step.
	stack *pathStack
	// n is the number of steps of the stack the path spans.
	n int
}

// pathStack holds the steps of the issue path of the node the walk is at,
// outermost first, below the root. Each step is an attribute, or an item of
// the multi-valued attribute the step before it names.
type pathStack struct {
	steps []pathStep
}

// pathStep is one step of an issue path below the root.
type pathStep struct {
	// attr is the name of an attribute, written "/name"; "" on an item.
	attr string
	// item is the predicate of an item, written after the path of its
	// attribute; unused on an attribute.
	item predicate
}

// attribute returns the path of the attribute name of the node at p, which
// spans a step of s more than p: s keeps p's steps and holds the
// attribute's step in place of any it held at that depth.
func (s *pathStack) attribute(p issuePath, name string) issuePath {
	s.steps = append(s.steps[:p.n], pathStep{attr: name})
	return issuePath{stack: s, n: p.n + 1}
}

// item returns the path of the item of the multi-valued attribute at p
// whose predicate is pred, which spans a step of s more than p, as
// [pathStack.attribute] pushes one.
func (s *pathStack) item(p issuePath, pred predicate) issuePath {
	s.steps = append(s.steps[:p.n], pathStep{item: pred})
	return issuePath{stack: s, n: p.n + 1}
}

// String writes the path: "/" for the root, "/name" for an attribute of the
// root, parent + "/name" for an attribute of any other node, and the path of
// its attribute followed by its predicate for an item.
func (p issuePath) String() string {
	return p.join()
}

// join writes the path with segments appended in order, in one step; the
// segments together are a sub-path that begins with "/", such as
// "/archetype_details", or "/", "language" and "/defining_code". Below the
// root, the segments follow the path; at the root they stand alone, so the
// root's sub-path is "/archetype_details", never "//archetype_details".
func (p issuePath) join(segments ...string) string {
	if p.n == 0 {
		switch len(segments) {
		case 0:
			return "/"
		case 1:
			return segments[0]
		}
	}
	var steps []pathStep
	if p.n > 0 {
		steps = p.stack.steps[:p.n]
	}
	size := 0
	for i := range steps {
		size += steps[i].size()
	}
	for _, segment := range segments {
		size += len(segment)
	}
	var b strings.Builder
	b.Grow(size)
	for i := range steps {
		steps[i].writeTo(&b)
	}
	for _, segment := range segments {
		b.WriteString(segment)
	}
	return b.String()
}

// size returns the length of the step as [pathStep.writeTo] writes it.
func (s *pathStep) size() int {
	if s.attr != "" {
		return len("/") + len(s.attr)
	}
	return s.item.size()
}

// writeTo writes the step to b: "/name" for an attribute, the predicate for
// an item.
func (s *pathStep) writeTo(b *strings.Builder) {
	if s.attr != "" {
		b.WriteByte('/')
		b.WriteString(s.attr)
		return
	}
	s.item.writeTo(b)
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
// each the predicate the rule gives it. An item that exists in the instance
// is written:
//
//   - [id] when no other item carries its id;
//   - [id, 'name'] when another item shares the id and the item's pinned
//     name tells it apart from every item that shares it;
//   - [n], its 1-based position, otherwise: it has no id, or its id is
//     shared and its pinned name does not tell it apart.
//
// numbered says whether the items exist in the instance and can be
// numbered. When it is false (OPT children of a node the instance lacks),
// an item with no id has no predicate, and one whose shared id its pinned
// name does not tell apart keeps its id alone.
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

// entryPredicate returns the predicate of the entry at index i, in ascending
// key order, of a Hash attribute: its 1-based position, never its key, which
// is instance data (REQ-102 Issue paths).
func entryPredicate(i int) predicate {
	return predicate{kind: predicatePosition, pos: i + 1}
}

// pathBelow returns the path of the item p is the predicate of, below the
// issue path attrPath of its attribute, already written, in one step.
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
