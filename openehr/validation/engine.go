package validation

// engine.go: the one walk. One walk over the instance visits each node once
// per place that holds it. At a node it knows the RM class of the value and,
// where a template reaches, the compiled node the value binds to; at each of
// the node's attributes it applies, in order, presence, then for each item
// the RM rules of its class and the descent below it. The RM rules are the
// floor's own rule bodies, dispatched through the inventory; the engine is
// the driver only.

import (
	"fmt"
	"slices"

	"github.com/cadasto/openehr-sdk-go/internal/rmaccess"
	tcimpl "github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/rminfo"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
)

// runEngine validates root in one walk, within lim, each zero or negative
// field of which stands for its default.
//
// With a nil c the walk applies the Reference Model rules alone and returns
// what [validateRM] returns for root: the same guards, the same findings by
// path, code, layer and rule, in the same order, and the same coverage,
// except that a value held from two places is validated at each of them. A
// walk condition (`cyclic_value`, `max_depth`, `max_nodes`) is worded in its
// Detail as the engine's own. The template side is not built yet: runEngine
// panics when c is not nil.
func runEngine(root any, c *templatecompile.Compiled, lim Limits) Result {
	if c != nil {
		panic("validation: runEngine does not walk a compiled template yet")
	}
	rmType, guard, ok := rmRootGuard(root)
	if !ok {
		return guard
	}
	e := newEngine(lim)
	e.walk(engineNode{value: root, rmType: rmType, path: "/"})
	r := resultFromIssues(e.rules.issues)
	r.Coverage.Stopped = e.stoppedAt
	r.Coverage.Unevaluated = unevaluatedIn(e.rules.visited)
	return r
}

// engine is the state of one walk. It keeps no record of the values it has
// finished: a value with an identity is checked against the values the walk
// is inside, and only them, so a value two places hold is validated at each.
type engine struct {
	// rules is what the Reference Model rules report into: the issues, in
	// walk order, and the classes the walk visited. The engine runs the rules
	// through it and uses none of its own walk state.
	rules rmFloorWalker
	// info is the class metadata the walk reads each node's attributes by,
	// and lister the part of it that lists a class's attributes; a nil lister
	// lists none.
	info   rminfo.Lookup
	lister rminfo.AttributeLister
	// maxDepth and maxNodes are the [Limits] of the call, defaults filled in.
	maxDepth, maxNodes int
	// stoppedAt holds the path of the max_depth issue and of the max_nodes
	// issue, in the order the walk reported them; depthStopped is set once
	// the first depth stop is reported, so later ones are silent.
	stoppedAt    []string
	depthStopped bool
	// ancestry holds the values with an identity ([hasIdentity]) the walk is
	// inside, outermost first: the one record a cycle is detected against.
	ancestry []any
	// visits counts the values the walk has visited, up to maxNodes; stopped
	// is set once it reaches that limit, and ends the walk.
	visits  int
	stopped bool
}

// newEngine returns the state of a walk within lim, each zero or negative
// field of which stands for its default.
func newEngine(lim Limits) *engine {
	lim = lim.withDefaults()
	info := rminfo.Default
	lister, _ := info.(rminfo.AttributeLister)
	return &engine{
		rules:    rmFloorWalker{info: info},
		info:     info,
		lister:   lister,
		maxDepth: lim.Depth,
		maxNodes: lim.Nodes,
		ancestry: make([]any, 0, 16),
	}
}

// engineNode is one node of the walk: a value held at one place.
type engineNode struct {
	// value is the node's value.
	value any
	// rmType is the RM type the value is read as: its runtime type where the
	// closed RM set knows it, otherwise the type its attribute declares. It
	// may carry a generic bound (DV_INTERVAL<DV_QUANTITY>).
	rmType string
	// path is the node's issue path.
	path string
	// role is what the node's place tells of it beyond its type.
	role floorRole
	// depth is the node's nesting depth, the root's 0.
	depth int
	// bound is the compiled template node the value binds to, nil where the
	// template does not reach, and so on every node of a walk with no
	// template.
	bound *tcimpl.CompiledNode
}

// declares reports whether the template node n binds to declares attr. A
// node the template does not reach declares nothing.
func (n engineNode) declares(attr string) bool {
	return n.bound != nil && n.bound.Attribute(attr) != nil
}

// engineAttr is one attribute the walk visits at a node, as the class
// metadata declares it.
type engineAttr struct {
	name string
	// rmType is the attribute's declared RM type, or its items' for a list.
	rmType string
	// shape is how the walk reads the attribute from the value.
	shape attrShape
	// mandatory is set when the RM marks the attribute mandatory for the
	// class, no rule of the class reports its absence itself
	// ([classRules.leavesOut]) and, for a single-valued one, the node's role
	// does not let it be blank ([mayBeBlank]): the walk then applies the
	// mandatory-presence rule to it.
	mandatory bool
}

// attrShape is how the walk reads an attribute from a value.
type attrShape uint8

const (
	// shapeSingle is a single-valued attribute: one value or none.
	shapeSingle attrShape = iota
	// shapeList is a list of RM values, read item by item.
	shapeList
	// shapeCounted is a list of a primitive type ([isPrimitiveType]), such
	// as DV_MULTIMEDIA data, an Array of Octet: nothing is below its items,
	// so it is counted and never read item by item.
	shapeCounted
	// shapeHash is a Hash attribute, read entry by entry in ascending key
	// order.
	shapeHash
)

// multiValued reports whether an attribute of the shape holds items: a list,
// counted or not. A Hash is no list: present and empty, it breaks no
// mandatory rule.
func (s attrShape) multiValued() bool {
	return s == shapeList || s == shapeCounted
}

// attrReading is an attribute as the walk reads it from a value, once.
type attrReading struct {
	// present is the attribute's presence, as the generated access decides
	// it; a single value that is nil or a typed-nil pointer is absent.
	present bool
	// count is the number of items of a list, counted or not, or of entries
	// of a Hash.
	count int
	// items holds the items of a list or the entries of a Hash, in key
	// order; nil for a counted list and a single-valued attribute.
	items []any
	// value is the value of a present single-valued attribute.
	value any
}

// readAttribute reads the attribute a of value, by its shape.
func readAttribute(value any, a engineAttr) attrReading {
	switch a.shape {
	case shapeList:
		items, present := rmaccess.Multiple(value, a.name)
		return attrReading{present: present, count: len(items), items: items}
	case shapeCounted:
		n, present := rmaccess.Len(value, a.name)
		return attrReading{present: present, count: n}
	case shapeHash:
		_, entries, present := rmaccess.Hash(value, a.name)
		return attrReading{present: present, count: len(entries), items: entries}
	case shapeSingle:
		v, present := rmaccess.Single(value, a.name)
		if present && v != nil && !rmaccess.IsTypedNilPointer(v) {
			return attrReading{present: true, value: v}
		}
	}
	return attrReading{}
}

// visitedAt reports whether the walk visits a at node n, which it reads as
// present or not: the attributes the walk visits at a node are the union of
// those the bound template node declares, those present on the value, and
// those the RM marks mandatory for the class. Where the template does not
// reach, an attribute in that union is visited with the RM rules alone; one
// outside it has nothing to check. An absent part of a reference that the
// template does not declare is outside it too: the reference's own rule
// reports it, when it checks the node.
func (n engineNode) visitedAt(a engineAttr, present bool) bool {
	return present || a.mandatory || n.declares(a.name)
}

// walk visits n unless the walk has stopped or n holds a nil or typed-nil
// value. A value with an identity that the walk is inside is a cycle: walk
// reports `cyclic_value` at n's path and does not enter it again. Every
// other value is visited, however many places hold it, so a value two places
// share is validated at each, against each place.
func (e *engine) walk(n engineNode) {
	if e.stopped || n.value == nil || rmaccess.IsTypedNilPointer(n.value) {
		return
	}
	if !hasIdentity(n.value) {
		e.visit(n)
		return
	}
	if slices.Contains(e.ancestry, n.value) {
		e.rules.emit(Issue{
			Path:   n.path,
			Code:   "cyclic_value",
			Detail: fmt.Sprintf("the walk reached this %s again inside itself; it is not entered again", n.rmType),
		})
		return
	}
	e.ancestry = append(e.ancestry, n.value)
	e.visit(n)
	e.ancestry = e.ancestry[:len(e.ancestry)-1]
}

// visit checks node n. A visit past the visit limit checks nothing: it
// reports `max_nodes` at n's path and ends the walk. Otherwise visit runs
// the RM rules of the node's class and records the class as visited; then,
// for a value of a registered RM class ([rmaccess.Covers]) above the depth
// limit, it visits the node's attributes. A value of no registered class is
// a leaf, checked by the rules of its declared type alone. At the depth limit
// visit goes no further, and the first node to reach it reports `max_depth`.
func (e *engine) visit(n engineNode) {
	if e.visits == e.maxNodes {
		e.rules.emit(Issue{
			Path:   n.path,
			Code:   "max_nodes",
			Detail: fmt.Sprintf("the walk stopped at %s after visiting %d values, its limit per call", n.rmType, e.maxNodes),
		})
		e.stoppedAt = append(e.stoppedAt, n.path)
		e.stopped = true
		return
	}
	e.visits++
	rules, class := e.rules.checkInvariants(n.value, n.rmType, n.path, n.role)
	if !rmaccess.Covers(n.value) {
		return
	}
	if n.depth >= e.maxDepth {
		if !e.depthStopped {
			e.depthStopped = true
			e.rules.emit(Issue{
				Path:   n.path,
				Code:   "max_depth",
				Detail: fmt.Sprintf("the walk reached its depth limit %d at %s; nothing below that depth is walked, and later stops there are not reported again", e.maxDepth, n.rmType),
			})
			e.stoppedAt = append(e.stoppedAt, n.path)
		}
		return
	}
	e.visitAttributes(n, class, rules)
}

// visitAttributes visits the attributes of node n, of the bare RM class
// class whose rules are rules, in the order the class metadata lists them.
// It leaves out an attribute no serialization carries
// ([rminfo.IsNonStorableAttr]) and one the metadata gives no type.
func (e *engine) visitAttributes(n engineNode, class string, rules *classRules) {
	if e.lister == nil {
		return
	}
	names := e.lister.AttributeNames(class)
	if names == nil {
		return
	}
	required := e.info.RequiredAttributes(class)
	for _, name := range names {
		if e.stopped {
			return // the visit limit ended the walk below this node
		}
		if rminfo.IsNonStorableAttr(class, name) {
			continue
		}
		attrType, ok := e.info.AttributeRMType(class, name)
		if !ok {
			continue
		}
		a := engineAttr{name: name, rmType: attrType, shape: e.shapeOf(class, name, attrType)}
		a.mandatory = slices.Contains(required, name) && !rules.leavesOut(name) &&
			(a.shape != shapeSingle || !mayBeBlank(n.role, name))
		e.visitAttribute(n, class, a)
	}
}

// shapeOf returns how the walk reads the attribute name of the class class,
// whose declared RM type, or its items' for a list, is attrType.
func (e *engine) shapeOf(class, name, attrType string) attrShape {
	if isContainer, _ := e.info.IsContainer(class, name); isContainer {
		if isPrimitiveType(e.info, attrType) {
			return shapeCounted
		}
		return shapeList
	}
	if attrType == hashType {
		return shapeHash
	}
	return shapeSingle
}

// visitAttribute visits the attribute a of node n, of class class: it reads
// the attribute once, and, when the attribute is one the walk visits
// ([engineNode.visitedAt]), applies the steps in order. Presence: absent, a
// mandatory attribute is `required`; a mandatory list that is present and
// empty is `cardinality`. Then each item: a single value is walked at the
// attribute's path, each item of a list of RM values at the path the shared
// path rule writes for it ([itemPaths]), and each entry of a Hash at its
// position in ascending key order ([hashEntryPath]), never at its key. Only a
// value of a registered RM class ([rmaccess.Covers]) is walked as an item of
// a list or a Hash; a counted list is never read item by item.
func (e *engine) visitAttribute(n engineNode, class string, a engineAttr) {
	r := readAttribute(n.value, a)
	if !n.visitedAt(a, r.present) {
		return
	}
	attrPath := joinPath(n.path, "/"+a.name)
	if a.mandatory {
		e.rules.checkMandatory(n.rmType, a.name, attrPath, a.shape.multiValued(), r.present, r.count)
	}
	if !r.present {
		return
	}
	role := childRole(class, n.role, a.name)
	switch a.shape {
	case shapeSingle:
		e.walk(engineNode{value: r.value, rmType: runtimeRMType(r.value, a.rmType), path: attrPath, role: role, depth: n.depth + 1})
	case shapeList:
		if !slices.ContainsFunc(r.items, rmaccess.Covers) {
			return
		}
		for i, path := range e.itemPaths(attrPath, r.items) {
			e.walkItem(n, r.items[i], a.rmType, path, role)
		}
	case shapeHash:
		for i, entry := range r.items {
			e.walkItem(n, entry, a.rmType, hashEntryPath(attrPath, i), role)
		}
	case shapeCounted:
		// Nothing is below the items of a list of a primitive type.
	}
}

// walkItem walks item, an item of a list or an entry of a Hash of node n
// whose declared RM type is rmType, at path, in role, when it is a value of a
// registered RM class; any other item has nothing below it to walk.
func (e *engine) walkItem(n engineNode, item any, rmType, path string, role floorRole) {
	if !rmaccess.Covers(item) {
		return
	}
	e.walk(engineNode{value: item, rmType: runtimeRMType(item, rmType), path: path, role: role, depth: n.depth + 1})
}

// itemPaths returns the issue path of each of the items of the multi-valued
// attribute at attrPath, by the shared path rule ([itemPaths]), fed each
// item's archetype_node_id. Where no template binds an item, the rule is fed
// no pinned name.
func (e *engine) itemPaths(attrPath string, items []any) []string {
	pathItems := make([]pathItem, len(items))
	for i, item := range items {
		pathItems[i] = pathItem{id: locatableArchetypeNodeID(item)}
	}
	return itemPaths(attrPath, pathItems)
}
