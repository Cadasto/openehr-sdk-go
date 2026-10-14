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

// itemPaths returns the path of each item of the multi-valued attribute at
// attrPath. An item is written:
//
//   - [id] when no other item carries its id;
//   - [id, 'name'] when another item shares the id and the item's pinned
//     name tells it apart from every item that shares it;
//   - [n], its 1-based position, otherwise: it has no id, or its id is
//     shared and its pinned name does not tell it apart.
func itemPaths(attrPath string, items []pathItem) []string {
	preds := predicates(items, true)
	for i, p := range preds {
		preds[i] = attrPath + p
	}
	return preds
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
	items := make([]pathItem, len(children))
	for i, c := range children {
		items[i] = pathItem{id: optChildID(c), name: c.NodeName()}
	}
	preds := predicates(items, false)
	for i, p := range preds {
		preds[i] = attrPath + p
	}
	return preds
}

// optChildID is the id an OPT child pins on the node it constrains: the
// archetype id of an archetype root, else its at-code.
func optChildID(c *tcimpl.CompiledNode) string {
	if id := c.ArchetypeID(); id != "" {
		return id
	}
	return c.NodeID()
}

// predicates applies the path rule to the items of one multi-valued
// attribute and returns each item's predicate, brackets included. numbered
// says whether the items exist in the instance and can be numbered: an item
// with no id, or with a shared id its pinned name does not tell apart, is
// then written by its position. When numbered is false (OPT children of a
// node the instance lacks), the first has no predicate and the second keeps
// its id alone.
func predicates(items []pathItem, numbered bool) []string {
	out := make([]string, len(items))
	s := newIDSharing(items)
	for i, it := range items {
		switch {
		case it.id == "" && numbered:
			out[i] = positionalPredicate(i)
		case it.id == "":
			out[i] = ""
		case !s.shared(it.id):
			out[i] = "[" + it.id + "]"
		case s.tellsApart(it):
			out[i] = namePredicate(it.id, it.name)
		case numbered:
			out[i] = positionalPredicate(i)
		default:
			out[i] = "[" + it.id + "]"
		}
	}
	return out
}

// positionalPredicate writes the predicate of the item at 0-based index i.
func positionalPredicate(i int) string {
	return "[" + strconv.Itoa(i+1) + "]"
}

// namePredicate writes an id with the name that tells its item apart. The
// name sits in single quotes; a backslash and a single quote inside it are
// each escaped by a backslash. The backslash goes first, so the backslash
// written for a quote is not escaped again.
func namePredicate(id, name string) string {
	name = strings.ReplaceAll(name, `\`, `\\`)
	name = strings.ReplaceAll(name, "'", `\'`)
	return "[" + id + ", '" + name + "']"
}

// idSharing counts, over the items of one attribute, how many carry each id
// and, for the ids that more than one item carries, how many carry each
// pinned name.
type idSharing struct {
	perID map[string]int
	// perName counts the items of each shared id by their pinned name.
	perName map[pathItem]int
}

// newIDSharing counts the ids of items. An attribute with one item or none
// shares no id, so it allocates nothing.
func newIDSharing(items []pathItem) idSharing {
	var s idSharing
	if len(items) < 2 {
		return s
	}
	s.perID = make(map[string]int, len(items))
	for _, it := range items {
		if it.id != "" {
			s.perID[it.id]++
		}
	}
	for _, it := range items {
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
	return id != "" && s.perID[id] > 1
}

// tellsApart reports whether the pinned name of it tells it apart from every
// other item that carries its id: it has a name and no other such item binds
// to a node pinning the same one. A sibling bound to a node that pins no name
// does not carry that name either, since an item whose name a child pins
// binds to that child (see [matchChildByID]), so it does not stop the name
// from telling the item apart.
func (s idSharing) tellsApart(it pathItem) bool {
	return it.name != "" && s.perName[it] == 1
}
