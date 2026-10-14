package validation

// engine.go: the one walk. One walk over the instance visits each node once
// per place that holds it. At a node it knows the RM class of the value and,
// where a template reaches, the compiled node the value binds to. At the node
// it checks identity and type against that node, runs the RM rules of the
// class and the template's value constraint at a leaf; then, at each of the
// node's attributes, those the template declares first and in its order, it
// applies in order admission, presence, counts, and for each admitted item
// the same steps one level down. The template rules
// are the template walk's own bodies (walk_composition.go) and the RM rules
// the floor's (rmfloor.go), dispatched through the inventory; the engine is
// the driver only.

import (
	"fmt"
	"slices"
	"strings"

	"github.com/cadasto/openehr-sdk-go/internal/bmmtype"
	"github.com/cadasto/openehr-sdk-go/internal/rmaccess"
	tcimpl "github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/rminfo"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
)

// runEngine validates root in one walk, with the compiled template c when it
// is not nil, within lim, each zero or negative field of which stands for its
// default.
//
// With a nil c the walk applies the Reference Model rules alone and returns
// what [validateRM] returns for root: the same guards, the same findings by
// path, code, layer and rule, in the same order, and the same coverage,
// except that a value held from two places is validated at each of them. A
// walk condition (`cyclic_value`, `max_depth`, `max_nodes`) is worded in its
// Detail as the engine's own.
//
// With a compiled template the root binds to the template's root node, under
// the guards [Validator.Validate] applies (a nil root, a template with no
// root), and the walk applies at every node the template rules and the
// Reference Model rules together. Content the template does not admit, an
// attribute it prohibits or an item that binds to no child, is reported once
// and not entered; content it does not constrain is walked with the
// Reference Model rules alone. A root outside the closed RM set is checked
// against the template's root node and then reported as `rm_type_unknown`
// at "/", as [Validate] reports it.
func runEngine(root any, c *templatecompile.Compiled, lim Limits) Result {
	r, _ := engineWalk(root, c, lim)
	return r
}

// engineWalk runs the walk [runEngine] describes and returns its result with
// the engine's final state, or a nil engine when a guard refused the call.
func engineWalk(root any, c *templatecompile.Compiled, lim Limits) (Result, *engine) {
	if c == nil {
		rmType, guard, ok := rmRootGuard(root)
		if !ok {
			return guard, nil
		}
		e := newEngine(lim)
		e.walk(engineNode{value: root, rmType: rmType, path: "/"})
		return e.result(), e
	}
	if root == nil || rmaccess.IsTypedNilPointer(root) {
		return nilRootResult(), nil
	}
	if c.Root() == nil {
		return resultFromIssues([]Issue{{
			Path:     "",
			Code:     "nil_template",
			Detail:   "compiled template has no root node",
			Severity: Error,
		}}), nil
	}
	e := newEngine(lim)
	rmType, _, known := rmTypeInfo(root)
	e.walk(engineNode{value: root, rmType: rmType, path: "/", bound: c.Root()})
	if !known {
		e.found.emit(Issue{
			Path:     "/",
			Code:     "rm_type_unknown",
			Detail:   fmt.Sprintf("root Go type %T is outside the closed RM set; the RM rules cannot descend", root),
			Severity: Error,
		})
	}
	return e.result(), e
}

// engine is the state of one walk. It keeps no record of the values it has
// finished: a value with an identity is checked against the values the walk
// is inside, and only them, so a value two places hold is validated at each.
type engine struct {
	// found is what the walk found: the issues, in walk order, and the
	// classes whose Reference Model rules ran. The engine runs the rules
	// through it and reports the template's issues into it too, so they
	// keep walk order.
	found findings
	// maxDepth and maxNodes are the [Limits] of the call, defaults filled in.
	maxDepth, maxNodes int
	// stoppedAt holds the path of the max_depth issue and of the max_nodes
	// issue, in the order the walk reported them; depthStopped is set once
	// the first depth stop is reported, so later ones are silent.
	stoppedAt    []string
	depthStopped bool
	// external holds, in walk order, the path of each node bound to a
	// CONSTRAINT_REF, whose value set is outside the SDK
	// ([Coverage.External]). Nil until one is met.
	external []string
	// refused holds, in walk order, the path of each attribute or item the
	// template did not admit, which the walk reported once and did not
	// enter.
	refused []string
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
	return &engine{
		maxDepth: lim.Depth,
		maxNodes: lim.Nodes,
		ancestry: make([]any, 0, 16),
	}
}

// result returns what the walk found: its issues, in walk order, and its
// coverage.
func (e *engine) result() Result {
	r := resultFromIssues(e.found.issues)
	r.Coverage = Coverage{Stopped: e.stoppedAt, External: e.external, Unevaluated: unevaluatedIn(e.found.visited)}
	return r
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

// engineAttr is one attribute the walk visits at a node.
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
	// classRead is set when the class metadata lists the attribute for the
	// value's class, so the walk reads its RM presence, mandatory, from the
	// class. An attribute the template declares that the value's class does
	// not list has no such reading.
	classRead bool
	// decl is the bound template node's declaration of the attribute, nil
	// where the template does not constrain it: the attribute is then
	// visited with the RM rules alone.
	decl *tcimpl.CompiledAttribute
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
// constrain it, an attribute in that union is visited with the RM rules
// alone; one outside it has nothing to check. An absent part of a reference
// that the template does not declare is outside it too: the reference's own
// rule reports it, when it checks the node.
func visitedAt(a engineAttr, present bool) bool {
	return present || a.mandatory || a.decl != nil
}

// walk visits n unless the walk has stopped or n holds a nil or typed-nil
// value. A value with an identity that the walk is inside is a cycle: walk
// reports `cyclic_value` at n's path and does not enter it again. Every
// other value is visited, however many places hold it, so a value two places
// share is validated at each, against each place's template node.
func (e *engine) walk(n engineNode) {
	if e.stopped || n.value == nil || rmaccess.IsTypedNilPointer(n.value) {
		return
	}
	if !hasIdentity(n.value) {
		e.visit(n)
		return
	}
	if slices.Contains(e.ancestry, n.value) {
		e.found.emit(Issue{
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
// reports `max_nodes` at n's path and ends the walk. Otherwise visit checks
// the node against the template node it binds to, if any: its identity and
// its RM type ([identityIssue], [rmTypeIssue]); then it runs the RM rules of
// the node's class, recording the class as visited; then, at a template
// leaf, the template's value constraint ([checkLeaf]). Last it visits the
// node's attributes: those of a value of a registered RM class
// ([rmaccess.Covers]), and those the template declares on any value it
// binds. A value of no registered class that no template node binds is a
// leaf, checked by the rules of its declared type alone. At the depth limit
// visit goes no further, and the first node to reach it reports
// `max_depth`.
//
// A rule of the node's class may read below the node, into one of its
// attributes. Where the template refused content below the node, an
// attribute it prohibits or a value that binds to no child, a finding of the
// node's rules at or below the refused content's path is withdrawn once the
// attributes are visited: the refusal is the whole report for that content
// ([engine.withdrawInside]).
func (e *engine) visit(n engineNode) {
	if e.visits == e.maxNodes {
		e.found.emit(Issue{
			Path:   n.path,
			Code:   "max_nodes",
			Detail: fmt.Sprintf("the walk stopped at %s after visiting %d values, its limit per call", n.rmType, e.maxNodes),
		})
		e.stoppedAt = append(e.stoppedAt, n.path)
		e.stopped = true
		return
	}
	e.visits++
	scalar := false
	if n.bound != nil {
		scalar = e.checkIdentityAndType(n)
	}
	rulesFrom := len(e.found.issues)
	rules, class := e.found.checkInvariants(n.value, n.rmType, n.path, n.role)
	rulesTo := len(e.found.issues)
	if n.bound != nil {
		n.bound = e.checkLeaf(n, scalar)
	}
	if n.bound == nil && !rmaccess.Covers(n.value) {
		return
	}
	if n.depth >= e.maxDepth {
		if !e.depthStopped {
			e.depthStopped = true
			e.found.emit(Issue{
				Path:   n.path,
				Code:   "max_depth",
				Detail: fmt.Sprintf("the walk reached its depth limit %d at %s; nothing below that depth is walked, and later stops there are not reported again", e.maxDepth, n.rmType),
			})
			e.stoppedAt = append(e.stoppedAt, n.path)
		}
		return
	}
	refusedFrom := len(e.refused)
	e.visitAttributes(n, class, rules)
	if refusals := e.refused[refusedFrom:]; len(refusals) > 0 && rulesTo > rulesFrom {
		e.withdrawInside(rulesFrom, rulesTo, refusals)
	}
}

// withdrawInside removes, from the issues the walk reported at positions
// from up to to, each one whose path is at or below one of refusals, the
// paths of content the template refused ([isAtOrBelow]); the issues after
// to keep their order.
func (e *engine) withdrawInside(from, to int, refusals []string) {
	issues := e.found.issues
	kept := issues[:from]
	for _, iss := range issues[from:to] {
		if !slices.ContainsFunc(refusals, func(r string) bool { return isAtOrBelow(iss.Path, r) }) {
			kept = append(kept, iss)
		}
	}
	if len(kept) == to {
		return
	}
	kept = append(kept, issues[to:]...)
	clear(issues[len(kept):])
	e.found.issues = kept
}

// isAtOrBelow reports whether the issue path path is under, or lies below
// it: under followed by an attribute or a predicate. A path that only shares
// a prefix with under is not below it: /items[10] is not below /items[1].
func isAtOrBelow(path, under string) bool {
	rest, ok := strings.CutPrefix(path, under)
	return ok && (rest == "" || rest[0] == '/' || rest[0] == '[')
}

// checkIdentityAndType checks node n against the template node it binds to,
// as the template walk does: its archetype_node_id against the node's pin,
// then its RM type against the node's. It reports whether the node is a
// scalar leaf ([scalarLeaf]), a Go primitive under an AOM 1.4 short name,
// which has no RM type to match.
func (e *engine) checkIdentityAndType(n engineNode) (scalar bool) {
	if iss, ok := identityIssue(n.bound, n.value, n.path); ok {
		e.found.emit(iss)
	}
	if _, scalar = scalarLeaf(n.bound, n.value); scalar {
		return true
	}
	if iss, ok := rmTypeIssue(n.bound, n.value, n.path); ok {
		e.found.emit(iss)
	}
	return false
}

// checkLeaf applies the template's value constraint to node n, at a leaf of
// the template, and returns the template node n's attributes are visited
// against: n.bound itself, or nil where the template constrains nothing below
// the node. A slot leaf carries no structure below it, and a leaf with a
// primitive constraint checks the value by that constraint
// ([appendPrimitiveIssues]); below either, the node's attributes are walked
// with the RM rules alone. A CONSTRAINT_REF names a value set outside the
// SDK and checks nothing locally: its path is listed in
// [Coverage.External], and the bound value's attributes are visited against
// the node as at any other.
func (e *engine) checkLeaf(n engineNode, scalar bool) *tcimpl.CompiledNode {
	if n.bound.IsSlot() {
		return nil
	}
	pc := n.bound.PrimitiveConstraint()
	if pc == nil {
		return n.bound
	}
	e.found.issues = appendPrimitiveIssues(e.found.issues, n.value, n.path, pc)
	if !isConstraintRef(pc) {
		return nil
	}
	e.external = append(e.external, n.path)
	if scalar {
		return nil // a scalar has no attributes to visit
	}
	return n.bound
}

// visitAttributes visits the attributes of node n, of the bare RM class
// class whose rules are rules. At a node the template binds it visits first
// the attributes the template declares, in the template's order. Then it
// visits the other attributes the class metadata lists for class, in the
// class's order, among them those the compile injected into the bound node
// because the RM marks them mandatory ([tcimpl.CompiledAttribute.Implicit]):
// no template clause constrains those, so they are visited with the RM
// rules alone. Last come the injected attributes the value's class does not
// list, as a value of no registered RM class has none, read as the template
// declares them. It leaves out an attribute no serialization carries
// ([rminfo.IsNonStorableAttr]), and one the metadata gives no type that the
// template does not declare. A value of no registered RM class
// ([rmaccess.Covers]) has no attributes the access reads, so the attributes
// of the template node it binds to are each read as absent.
//
// What the class metadata says of each attribute is read from the table
// built once per class when the package is initialised ([classAttributes]),
// never from the template, so the RM rules read the pinned Reference Model
// whatever lookup compiled the template.
func (e *engine) visitAttributes(n engineNode, class string, rules *classRules) {
	var table *classAttributeTable
	if rmaccess.Covers(n.value) {
		table = classAttributes[class]
	}
	if n.bound != nil {
		for _, decl := range tcimpl.NodeAttributes(n.bound) {
			if e.stopped {
				return // the visit limit ended the walk below this node
			}
			if decl.Implicit() {
				continue // no template clause: visited below
			}
			f, listed := table.lookup(decl.Name())
			if a, visited := declaredAttr(n, decl, f, listed, rules); visited {
				e.visitAttribute(n, class, a)
			}
		}
	}
	if table != nil {
		for _, f := range table.listed {
			if e.stopped {
				return
			}
			if f.NonStorable || !f.Typed || templateDeclares(n.bound, f.Name) {
				continue // not visited, or visited above in the template's order
			}
			e.visitAttribute(n, class, attrFromFacts(n, f, rules))
		}
	}
	if n.bound == nil {
		return
	}
	for _, decl := range tcimpl.NodeAttributes(n.bound) {
		if e.stopped {
			return
		}
		if _, listed := table.lookup(decl.Name()); !decl.Implicit() || listed {
			continue // visited above
		}
		if a, visited := declaredAttr(n, decl, tcimpl.AttributeFacts{}, false, rules); visited {
			e.visitAttribute(n, class, a)
		}
	}
}

// templateDeclares reports whether the template node bound declares the
// attribute name, as the template wrote it rather than as the compile
// injected it; a nil bound declares none.
func templateDeclares(bound *tcimpl.CompiledNode, name string) bool {
	if bound == nil {
		return false
	}
	a := bound.Attribute(name)
	return a != nil && !a.Implicit()
}

// declaredAttr returns the attribute decl the bound template node of n
// declares, as the walk visits it, with visited set, given f, what the class
// metadata says of the attribute for the value's class, which lists it when
// listed is set. An attribute the class lists, a serialization carries and
// the metadata types is read as the class metadata gives it. Any other is
// read as the template declares it, single-valued or a list; it is not
// visited when no serialization of the bound node's class carries it.
func declaredAttr(n engineNode, decl *tcimpl.CompiledAttribute, f tcimpl.AttributeFacts, listed bool, rules *classRules) (a engineAttr, visited bool) {
	if listed && !f.NonStorable && f.Typed {
		a = attrFromFacts(n, f, rules)
		a.decl = decl
		return a, true
	}
	if rminfo.IsNonStorableAttr(bmmtype.Class(n.bound.RMTypeName()), decl.Name()) {
		return a, false
	}
	a = engineAttr{name: decl.Name(), rmType: decl.RMTypeName(), shape: shapeSingle, decl: decl}
	if decl.Cardinality() == template.Multiple {
		a.shape = shapeList
	}
	return a, true
}

// attrFromFacts returns the attribute of node n, whose rules are rules,
// that f describes, as the class metadata gives it, with no template
// declaration. It is mandatory when the metadata marks it mandatory for the
// class, no rule of the class reports its absence itself and, for a
// single-valued one, the node's role does not let it be blank.
func attrFromFacts(n engineNode, f tcimpl.AttributeFacts, rules *classRules) engineAttr {
	a := engineAttr{name: f.Name, rmType: f.RMType, shape: shapeFrom(f.Container, f.PrimitiveItems, f.Hash), classRead: true}
	a.mandatory = f.Mandatory && !rules.leavesOut(f.Name) && (a.shape != shapeSingle || !mayBeBlank(n.role, f.Name))
	return a
}

// shapeFrom returns how the walk reads an attribute that is a container or
// not, whose items, for a container, are of a primitive type or not, and
// that is a Hash or not.
func shapeFrom(container, primitiveItems, hash bool) attrShape {
	switch {
	case container && primitiveItems:
		return shapeCounted
	case container:
		return shapeList
	case hash:
		return shapeHash
	}
	return shapeSingle
}

// visitAttribute visits the attribute a of node n, of class class: it reads
// the attribute once, and, when the attribute is one the walk visits
// ([visitedAt]), applies the steps in order. An attribute the template
// declares goes through the template's steps ([engine.visitDeclaredSingle],
// [engine.visitDeclaredItems]); one it does not constrain is visited with the
// RM rules alone. Presence: absent, a mandatory attribute is `required`; a
// mandatory list that is present and empty is `cardinality`. Then each item:
// a single value is walked at the attribute's path, each item of a list of
// RM values at the path the shared path rule writes for it ([itemPaths]),
// and each entry of a Hash at its position in ascending key order
// ([hashEntryPath]), never at its key. Only a value of a registered RM class
// ([rmaccess.Covers]) is walked as an item of a list or a Hash; a counted
// list is never read item by item.
func (e *engine) visitAttribute(n engineNode, class string, a engineAttr) {
	r := readAttribute(n.value, a)
	if !visitedAt(a, r.present) {
		return
	}
	attrPath := attributePath(n.path, a.name)
	role := childRole(class, n.role, a.name)
	if a.decl != nil {
		if a.shape == shapeSingle {
			e.visitDeclaredSingle(n, a, r, attrPath, role)
		} else {
			e.visitDeclaredItems(n, a, r, attrPath, role)
		}
		return
	}
	if a.mandatory {
		e.found.checkMandatory(n.rmType, a.name, attrPath, a.shape.multiValued(), r.present, r.count)
	}
	if r.present {
		e.walkUnconstrained(n, a, r, attrPath, role)
	}
}

// walkUnconstrained walks what the present attribute a of node n, read as r,
// holds, at attrPath, in role, with the RM rules alone: no template node
// binds it.
func (e *engine) walkUnconstrained(n engineNode, a engineAttr, r attrReading, attrPath string, role floorRole) {
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
// whose declared RM type is rmType, at path, in role, with the RM rules
// alone, when it is a value of a registered RM class; any other item has
// nothing below it to walk.
func (e *engine) walkItem(n engineNode, item any, rmType, path string, role floorRole) {
	if !rmaccess.Covers(item) {
		return
	}
	e.walk(engineNode{value: item, rmType: runtimeRMType(item, rmType), path: path, role: role, depth: n.depth + 1})
}

// visitDeclaredSingle visits the single-valued attribute a of node n, which
// the template declares, read as r, at attrPath, in role, by the steps of
// the one walk.
//
// Admission: a present value of an attribute the template prohibits
// ([isProhibited]) is reported once, as `prohibited`, and a value that binds
// to none of the children the template allows is reported once by
// [admitSingle]; either way the walk does not enter it. Presence: an absent
// attribute is reported as [engine.checkAbsent] says. Then the admitted value
// is walked against the child it binds to, or, where the template declares
// no child, with the RM rules alone.
func (e *engine) visitDeclaredSingle(n engineNode, a engineAttr, r attrReading, attrPath string, role floorRole) {
	if !r.present {
		e.checkAbsent(n, a, attrPath)
		return
	}
	if isProhibited(a.decl) {
		e.refuse(prohibitedIssue(a.decl, n.value, attrPath))
		return
	}
	var child *tcimpl.CompiledNode
	if children := allowedChildren(a.decl); len(children) > 0 {
		bound, refusal, admitted := admitSingle(a.decl, children, r.value, attrPath)
		if !admitted {
			e.refuse(refusal)
			return
		}
		child = bound
	}
	e.walk(engineNode{value: r.value, rmType: runtimeRMType(r.value, a.rmType), path: attrPath, role: role, depth: n.depth + 1, bound: child})
}

// visitDeclaredItems visits the multi-valued attribute a of node n, a list or
// a Hash the template declares, read as r, at attrPath, in role, by the steps
// of the one walk.
//
// Admission: a present attribute the template prohibits that holds an item
// is reported once, as `prohibited`, and not entered; each item that binds to
// none of the template's children ([bindItems]) is reported once
// ([unboundItemIssue]) and not entered. Presence: an absent attribute is
// reported as [engine.checkAbsent] says, and one that is present and holds
// no item as [engine.checkEmpty] says. Counts: the template's child count and
// each child's occurrences. Then each admitted item is walked against the
// child it binds to, at the path the template walk writes for it; where the
// template declares no child, each item is walked with the RM rules alone.
func (e *engine) visitDeclaredItems(n engineNode, a engineAttr, r attrReading, attrPath string, role floorRole) {
	if !r.present {
		e.checkAbsent(n, a, attrPath)
		return
	}
	if r.count > 0 && isProhibited(a.decl) {
		e.refuse(prohibitedIssue(a.decl, n.value, attrPath))
		return
	}
	children := tcimpl.ChildNodes(a.decl)
	var bound []*tcimpl.CompiledNode
	var paths []string
	if len(children) > 0 && a.shape != shapeCounted {
		var byNodeID []bool
		bound, byNodeID = bindItems(n.bound.RMTypeName(), a.name, children, r.items)
		paths = boundItemPaths(attrPath, r.items, bound, a.shape == shapeHash)
		for i, item := range r.items {
			if bound[i] == nil {
				e.refuse(unboundItemIssue(a.decl, children, item, byNodeID[i], paths[i]))
			}
		}
	}
	countIssue, countOut := childCountIssue(a.decl, r.count, attrPath)
	var occurrences []Issue
	if bound != nil {
		occurrences = appendOccurrenceIssues(nil, a.decl, children, bound, attrPath)
	}
	if r.count == 0 && (a.shape.multiValued() || a.decl.Cardinality() == template.Multiple) {
		// The template reports the container itself by its child count, or
		// by the occurrences of a child that has no id of its own, which are
		// written at the container's path ([optChildPaths]).
		reported := countOut || slices.ContainsFunc(occurrences, func(iss Issue) bool { return iss.Path == attrPath })
		e.checkEmpty(n, a, attrPath, reported)
	}
	if countOut {
		e.found.emit(countIssue)
	}
	e.found.issues = append(e.found.issues, occurrences...)
	switch {
	case len(children) == 0:
		e.walkUnconstrained(n, a, r, attrPath, role)
	case bound != nil:
		for i, item := range r.items {
			if bound[i] == nil {
				continue
			}
			e.walk(engineNode{value: item, rmType: runtimeRMType(item, a.rmType), path: paths[i], role: role, depth: n.depth + 1, bound: bound[i]})
		}
	}
}

// refuse reports iss, the one issue for content the template does not admit,
// at the path of that content, which the walk does not enter.
func (e *engine) refuse(iss Issue) {
	e.found.emit(iss)
	e.refused = append(e.refused, iss.Path)
}

// checkAbsent reports the attribute a of node n, which the template declares
// and the instance lacks, at attrPath, once. Where the template's existence
// requires it, it is the template's `required`. Otherwise it is the RM's
// `required` where the RM marks it mandatory for the value's class, as the
// RM rules alone read it (a.mandatory), which leave out the part of a
// reference, whose own rule reports it, and a value the node's role lets be
// blank. Only where the value's class does not list the attribute, so the
// walk has no RM reading of it, is the RM's mandatory bit the template
// carries read instead ([requiredBy]).
func (e *engine) checkAbsent(n engineNode, a engineAttr, attrPath string) {
	multiValued := a.shape != shapeSingle
	if iss, required := absentIssue(a.decl, n.value, attrPath, multiValued); required && (iss.Layer == LayerTemplate || !a.classRead) {
		e.found.emit(iss)
		return
	}
	if a.mandatory {
		e.found.checkMandatory(n.rmType, a.name, attrPath, a.shape.multiValued(), false, 0)
	}
}

// checkEmpty reports the multi-valued attribute a of node n, at attrPath,
// which is present and holds no item, once. Where the template's existence
// requires it, it is the template's `required`. Where only the RM requires
// it, it is the RM's `cardinality`, rule cardinality, unless the template
// reports the same container itself (reported), and only for a list: a Hash
// that is present and empty breaks no mandatory rule, as the RM rules alone
// read it, however the template declares it. Whether the RM requires it is
// read as [engine.checkAbsent] reads it: from the value's class, and from
// the mandatory bit the template carries only where that class does not
// list the attribute.
func (e *engine) checkEmpty(n engineNode, a engineAttr, attrPath string, reported bool) {
	iss, required := emptyIssue(a.decl, n.value, attrPath)
	switch {
	case required && iss.Layer == LayerTemplate:
		e.found.emit(iss)
	case (a.mandatory || required && !a.classRead) && a.shape.multiValued() && !reported:
		e.found.checkMandatory(n.rmType, a.name, attrPath, true, true, 0)
	}
}

// itemPaths returns the issue path of each of the items of the multi-valued
// attribute at attrPath, by the shared path rule ([itemPaths]), fed each
// item's archetype_node_id. No template node binds these items, so the rule
// is fed no pinned name.
func (e *engine) itemPaths(attrPath string, items []any) []string {
	pathItems := make([]pathItem, len(items))
	for i, item := range items {
		pathItems[i] = pathItem{id: locatableArchetypeNodeID(item)}
	}
	return itemPaths(attrPath, pathItems)
}
