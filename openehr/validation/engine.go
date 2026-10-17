package validation

// engine.go: the one walk, which every entry point runs. The walk over the
// instance visits each node once per place that holds it. At a node it knows
// the RM class of the value and, where a template reaches, the compiled node
// the value binds to. At the node it checks identity and type against that
// node, runs the RM rules of the class and the template's value constraint
// at a leaf; then, at each of the node's attributes, those the template
// declares first and in its order, it applies in order admission, presence,
// counts, and for each admitted item the same steps one level down. The
// template rules are the bodies in walk_composition.go and the RM rules those
// in rmfloor.go and rmfloor_coded.go, dispatched through the inventory; the
// engine is the driver only.

import (
	"fmt"
	"slices"
	"strings"

	"github.com/cadasto/openehr-sdk-go/internal/bmmtype"
	"github.com/cadasto/openehr-sdk-go/internal/rmaccess"
	tcimpl "github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/rminfo"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/template/constraints"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
)

// runEngine validates root in one walk, with the compiled template c when it
// is not nil, within lim, each zero or negative field of which stands for its
// default.
//
// With a nil c the walk applies the Reference Model rules alone, every
// attribute unconstrained, under the guards [ValidateRM] describes: a nil
// root is `nil_root`, and a root outside the closed RM set `rm_type_unknown`
// at "/".
//
// With a compiled template the root binds to the template's root node, under
// the guards [Validator.Validate] applies (a nil root, a template with no
// root), and the walk applies at every node the template rules and the
// Reference Model rules together. Content the template does not admit, an
// attribute it prohibits, a value that binds to no child or an item bound to
// a child it does not allow, is reported once and not entered; content it
// does not constrain is walked with the Reference Model rules alone. A root outside the closed RM set is checked
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
		e.walk(engineNode{value: root, rmType: rmType})
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
	e.walk(engineNode{value: root, rmType: rmType, bound: c.Root()})
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
	// refused holds, in walk order, the path of each attribute, value or
	// item the template did not admit, which the walk reported once and did
	// not enter; an item bound to a child the template does not allow is
	// reported by that child's occurrences.
	refused []string
	// ancestry holds the values with an identity ([hasIdentity]) the walk is
	// inside, outermost first: the one record a cycle is detected against.
	ancestry []any
	// visits counts the values the walk has visited, and the list items
	// holding none it has reported, up to maxNodes; stopped is set once it
	// reaches that limit, and ends the walk.
	visits  int
	stopped bool
	// path holds the steps of the issue path of the node the walk is at,
	// which the handle of each node's path reads ([issuePath]).
	path pathStack
	// preds holds the predicates of the items of each list the walk is
	// inside, outermost first, a run per list: the walk appends a list's run
	// when it enters the list and drops it when it leaves, so a list of any
	// length holds its predicates without a slice of its own.
	preds []predicate
}

// newEngine returns the state of a walk within lim, each zero or negative
// field of which stands for its default.
func newEngine(lim Limits) *engine {
	lim = lim.withDefaults()
	return &engine{
		maxDepth: lim.Depth,
		maxNodes: lim.Nodes,
		ancestry: make([]any, 0, 16),
		path:     pathStack{steps: make([]pathStep, 0, 16)},
		preds:    make([]predicate, 0, 16),
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
	// path is the node's issue path, a handle valid while the node is
	// visited ([issuePath]).
	path issuePath
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
	// mandatory-presence rule to it. In a class's table
	// ([classAttributeTable]) it holds the class's part alone; the walk
	// applies the role's part at each node ([engineAttr.inRole]).
	mandatory bool
	// noRules is set, in a class's table, when no RM rules run on a value
	// read as rmType ([rulesFor]), so that a value of a Go primitive type
	// the attribute holds is a primitive leaf ([primitiveLeaf]). Unset, the
	// walk visits such a value as it visits any other.
	noRules bool
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
	// shapeCounted is a list whose items the class metadata knows as a
	// primitive type ([attributeFacts].PrimitiveItems), such as
	// DV_MULTIMEDIA data, an Array of Octet: nothing is below its items, so
	// it is counted and never read item by item.
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
func readAttribute(value any, a *engineAttr) attrReading {
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
func visitedAt(a *engineAttr, present bool) bool {
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
			Path:   n.path.String(),
			Code:   "cyclic_value",
			Detail: fmt.Sprintf("the walk reached this %s again inside itself; it is not entered again", n.rmType),
		})
		return
	}
	e.ancestry = append(e.ancestry, n.value)
	e.visit(n)
	e.ancestry = e.ancestry[:len(e.ancestry)-1]
}

// spendVisit counts one visit, to a value of RM type rmType or to a list
// item holding none whose declared type is rmType, at path, and reports
// whether the walk may go on with it. Past the visit limit it counts
// nothing: it reports `max_nodes` at path, records path in the stops of
// [Coverage.Stopped], ends the walk and returns false.
func (e *engine) spendVisit(path issuePath, rmType string) bool {
	if e.visits == e.maxNodes {
		at := path.String()
		e.found.emit(Issue{
			Path:   at,
			Code:   "max_nodes",
			Detail: fmt.Sprintf("the walk stopped at %s after visiting %d values, its limit per call", rmType, e.maxNodes),
		})
		e.stoppedAt = append(e.stoppedAt, at)
		e.stopped = true
		return false
	}
	e.visits++
	return true
}

// spendLeafVisit counts one visit to a primitive leaf ([primitiveLeaf]) of
// declared RM type rmType at path, as [engine.spendVisit] counts it, unless
// the walk has stopped: once the visit limit has ended the walk, a leaf is
// neither counted nor reported, as no other value is.
func (e *engine) spendLeafVisit(path issuePath, rmType string) {
	if !e.stopped {
		e.spendVisit(path, rmType)
	}
}

// primitiveLeaf reports whether value, the value of the single-valued
// attribute a, is a primitive leaf: a value read under a declared type that
// has no RM rules ([engineAttr].noRules), of a type no RM class is. The
// types are the Go primitive types; the primitive types the RM package
// names, rm.Real, rm.Integer and rm.Character; and the pointers the
// generated access hands over for an optional primitive attribute, *string,
// *bool, *rm.Real and *rm.Integer, each of which it reads as present only
// when it is not nil. A test keeps every one of them outside the RM classes.
func primitiveLeaf(value any, a *engineAttr) bool {
	if !a.noRules {
		return false
	}
	switch value.(type) {
	case string, bool, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64,
		rm.Real, rm.Integer, rm.Character,
		*string, *bool, *rm.Real, *rm.Integer:
		return true
	}
	return false
}

// visit checks node n. A visit past the visit limit checks nothing: it
// reports `max_nodes` at n's path and ends the walk ([engine.spendVisit]).
// Otherwise visit checks
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
// attribute it prohibits, a value that binds to no child or an item bound to
// a child it does not allow, a finding of the node's rules at or below the
// refused content's path is withdrawn once the attributes are visited: the
// refusal is the whole report for that content ([engine.withdrawInside]). A
// finding of theirs at the node's own path stays, though the rule read the
// refused content to reach it.
//
// A value constraint that leaves an empty part of the value to the RM rules
// is settled once the attributes are visited too ([engine.settle]); its
// findings are at the node's own path, never at or below refused content.
func (e *engine) visit(n engineNode) {
	if !e.spendVisit(n.path, n.rmType) {
		return
	}
	scalar := false
	if n.bound != nil {
		scalar = e.checkIdentityAndType(n)
	}
	rulesFrom := len(e.found.issues)
	entry, class := classOf(n.rmType)
	e.found.checkInvariants(n.value, n.rmType, n.path, entry.rules)
	rulesTo := len(e.found.issues)
	var held *heldJudgement
	if n.bound != nil {
		n.bound, held = e.checkLeaf(n, scalar, class)
	}
	if n.bound == nil && !rmaccess.Covers(n.value) {
		return
	}
	if n.depth >= e.maxDepth {
		if !e.depthStopped {
			e.depthStopped = true
			at := n.path.String()
			e.found.emit(Issue{
				Path:   at,
				Code:   "max_depth",
				Detail: fmt.Sprintf("the walk reached its depth limit %d at %s; nothing below that depth is walked, and later stops there are not reported again", e.maxDepth, n.rmType),
			})
			e.stoppedAt = append(e.stoppedAt, at)
		}
		return
	}
	refusedFrom := len(e.refused)
	e.visitAttributes(n, class, entry)
	if held != nil {
		e.settle(held)
	}
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

// checkIdentityAndType checks node n against the template node it binds to:
// its archetype_node_id against the node's pin, then its RM type against the
// node's. It reports whether the node is a
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

// checkLeaf applies the template's value constraint to node n, of the bare
// RM class class, at a leaf of the template, and returns the template node
// n's attributes are visited against: n.bound itself, or nil where the
// template constrains nothing below the node. A slot leaf carries no
// structure below it, and a leaf with a primitive constraint checks the
// value by that constraint ([engine.judgeLeaf]); below either, the node's
// attributes are walked with the RM rules alone. Where the part of the value
// the constraint reads is empty, checkLeaf also returns the judgement the
// walk holds until it has visited the node's attributes, and nil otherwise.
// A CONSTRAINT_REF names a value set outside the SDK and checks nothing
// locally: its path is listed in [Coverage.External], and the bound value's
// attributes are visited against the node as at any other.
func (e *engine) checkLeaf(n engineNode, scalar bool, class string) (*tcimpl.CompiledNode, *heldJudgement) {
	if n.bound.IsSlot() {
		return nil, nil
	}
	pc := n.bound.PrimitiveConstraint()
	if pc == nil {
		return n.bound, nil
	}
	held := e.judgeLeaf(n, class, pc)
	if !isConstraintRef(pc) {
		return nil, held
	}
	e.external = append(e.external, n.path.String())
	if scalar {
		return nil, held // a scalar has no attributes to visit
	}
	return n.bound, held
}

// constrainedPart names, for each RM class whose value a primitive
// constraint reads by its parts, the attributes that lead from the value to
// the part the constraint reads that the RM rules may report as absent or
// empty: a CODE_PHRASE's code_string, a DV_QUANTITY's units, the value of
// the DV_DATE, DV_TIME, DV_DATE_TIME and DV_DURATION an AOM 1.4 short name
// binds, and the code string of a DV_ORDINAL's symbol, two levels down.
var constrainedPart = map[string][]string{
	"CODE_PHRASE":  {"code_string"},
	"DV_QUANTITY":  {"units"},
	"DV_DATE":      {"value"},
	"DV_TIME":      {"value"},
	"DV_DATE_TIME": {"value"},
	"DV_DURATION":  {"value"},
	"DV_ORDINAL":   {"symbol", "defining_code", "code_string"},
}

// heldJudgement is the judgement of a primitive constraint whose part the RM
// rules may still report as absent or empty, held by the walk until it has
// visited the node's attributes ([engine.judgeLeaf], [engine.settle]).
type heldJudgement struct {
	// part is the issue path of the empty part.
	part string
	// from and to bound, in the issues the walk reported, those the
	// constraint drew on what the part leaves.
	from, to int
	// whole holds the issues the constraint draws on the whole value.
	whole []Issue
}

// judgeLeaf applies the primitive constraint pc to node n, of the bare RM
// class class, and reports its issues. Where the walk visits the node's
// attributes and the part of the value pc reads is absent or empty
// ([engine.emptyPart]), the RM rules may report that part, and their
// `required` there then stands for pc's finding: judgeLeaf reports what pc
// judges of what the part leaves ([appendLeftIssues]) and returns what pc
// judges of the whole value, which the walk holds until it has visited the
// attributes and seen whether the part was reported ([engine.settle]).
// Otherwise pc judges the whole value at once, as at the depth limit, where
// the attributes are not visited, and judgeLeaf returns nil.
func (e *engine) judgeLeaf(n engineNode, class string, pc constraints.PrimitiveConstraint) *heldJudgement {
	part, empty := e.emptyPart(n, class)
	if !empty {
		e.found.issues = appendPrimitiveIssues(e.found.issues, n.value, n.path, pc)
		return nil
	}
	from := len(e.found.issues)
	e.found.issues = appendLeftIssues(e.found.issues, n, class, pc)
	return &heldJudgement{
		part:  part,
		from:  from,
		to:    len(e.found.issues),
		whole: appendPrimitiveIssues(nil, n.value, n.path, pc),
	}
}

// appendLeftIssues appends to issues what the primitive constraint pc,
// bound to node n of the bare RM class class, judges of what the empty part
// of n's value leaves: a C_CODE_PHRASE judges the terminology by its own
// rule, its code list set aside, and the constraint of a DV_ORDINAL judges
// the ordinal's value alone, which [primitiveInput] passes on as it is. Any
// other constraint is left nothing to judge: a C_DV_QUANTITY's ranges are
// per unit, and a temporal short name reads the value alone.
func appendLeftIssues(issues []Issue, n engineNode, class string, pc constraints.PrimitiveConstraint) []Issue {
	switch class {
	case "CODE_PHRASE":
		if cp, ok := pc.(constraints.CodePhrase); ok {
			return appendPrimitiveIssues(issues, n.value, n.path, constraints.CodePhrase{Terminology: cp.Terminology})
		}
	case "DV_ORDINAL":
		if in, ok := primitiveInput(n.value).(constraints.OrdinalSymbol); ok {
			return appendPrimitiveIssues(issues, in.Value, n.path, pc)
		}
	}
	return issues
}

// emptyPart returns the issue path of the part of node n's value, of the
// bare RM class class, that a primitive constraint reads ([constrainedPart])
// and reports whether the RM rules may report it as absent or empty. They
// may when the walk visits the node's attributes, which it does on a value
// of a registered RM class short of the depth limit, and the part is absent
// through the generated access, which reads an empty string as absent.
// Whether they do depends on the role of the node that holds the part, a
// DV_SCALE symbol's code_string being one they let be blank, and on whether
// the walk reaches the part before a limit stops it: the walk sees that
// once it has visited the attributes ([engine.settle]). The part's path is
// written only when the part is empty.
func (e *engine) emptyPart(n engineNode, class string) (string, bool) {
	steps, ok := constrainedPart[class]
	if !ok || n.depth >= e.maxDepth || !rmaccess.Covers(n.value) {
		return "", false
	}
	value, path := n.value, n.path
	for i, name := range steps {
		path = e.path.attribute(path, name)
		v, present := rmaccess.Single(value, name)
		if !present || v == nil || rmaccess.IsTypedNilPointer(v) {
			if i < len(steps)-1 {
				return "", false
			}
			return path.String(), true
		}
		value = v
	}
	return "", false
}

// settle settles the judgement h held at a node once the walk has visited
// the node's attributes. Where the walk reported the empty part as
// `required`, that finding stands for the constraint's, and the issues the
// constraint drew on what the part leaves stay as they are. Where it did
// not, because the part is one the RM rules let be blank or a limit stopped
// the walk before it reached the part, the issues the constraint draws on
// the whole value take their place, so the part is reported either way, in
// walk order. No issue at or before h's was removed while the attributes
// were visited: a node below withdraws only issues reported after its own
// visit began ([engine.withdrawInside]).
func (e *engine) settle(h *heldJudgement) {
	reported := slices.ContainsFunc(e.found.issues[h.to:], func(iss Issue) bool {
		return iss.Code == "required" && iss.Path == h.part
	})
	if !reported {
		e.found.issues = slices.Replace(e.found.issues, h.from, h.to, h.whole...)
	}
}

// visitAttributes visits the attributes of node n, of the bare RM class
// class, whose rules and attribute table are entry's ([classOf]). At a node
// the template binds it visits first the attributes the template declares,
// in the template's order. Then it visits the other attributes the class
// metadata lists for class, in the class's order, with the RM rules alone:
// no template clause constrains them. Among them are those the compile
// injected into the bound node because the RM marks them mandatory for the
// node's class ([tcimpl.CompiledAttribute.Implicit]); an injected attribute
// the value's class does not list is not visited, so it draws no finding.
// It leaves out an attribute no serialization carries
// ([rminfo.IsNonStorableAttr]), and one the metadata gives no type that the
// template does not declare. A value of no registered RM class
// ([rmaccess.Covers]) has no attributes the access reads and no class the
// metadata lists attributes for: the attributes the template declares on
// the node it binds to are each read as absent, and only the template's
// existence reports one.
//
// What the class metadata says of each attribute, and how the walk reads
// each one it visits, is read from the table built once per class when the
// package is initialised ([classAttributes]), never from the template, so
// the RM rules read the pinned Reference Model whatever lookup compiled the
// template. Only the part the node's role decides is applied at the node
// ([engineAttr.inRole]).
//
// An attribute the class lists is visited in the template's order, and not
// again in the class's, when the bound node declares it as the template
// wrote it rather than as the compile injected it. The walk marks each such
// attribute by its position in the class's table while it visits the
// template's declarations (declared), once however many of them name it;
// a declaration of a name the class does not list marks none. The compile
// injects only attributes the template does not declare, so a name the
// template declares is never also an injected one, and no class's table
// walks more attributes than the mark holds ([attrMarks]).
func (e *engine) visitAttributes(n engineNode, class string, entry classEntry) {
	var table *classAttributeTable
	if rmaccess.Covers(n.value) {
		table = entry.attrs
	}
	var declared attrMarks
	if n.bound != nil {
		for _, decl := range tcimpl.NodeAttributes(n.bound) {
			if e.stopped {
				return // the visit limit ended the walk below this node
			}
			if decl.Implicit() {
				continue // no template clause: visited below, if the class lists it
			}
			var walked *engineAttr
			if i, ok := table.position(decl.Name()); ok {
				walked = &table.walked[i]
				declared |= 1 << i
			}
			if a, visited := declaredAttr(n, decl, walked); visited {
				e.visitAttribute(&n, class, &a)
			}
		}
	}
	if table == nil {
		return
	}
	for i := range table.walked {
		if e.stopped {
			return
		}
		if declared&(1<<i) != 0 {
			continue // visited above in the template's order
		}
		a := &table.walked[i]
		if a.blankIn(n.role) {
			inRole := a.inRole(n.role)
			a = &inRole
		}
		e.visitAttribute(&n, class, a)
	}
}

// attrMarks marks attributes of a class's table by their position, bit i
// for the attribute at position i ([engine.visitAttributes]). No class's
// table walks more attributes than it has bits; a test keeps it so.
type attrMarks uint64

// declaredAttr returns the attribute decl the bound template node of n
// declares, as the walk visits it, with visited set, given walked, the
// attribute as the value's class table walks it
// ([classAttributeTable.position]), nil when the table walks none by decl's
// name. An attribute the class lists, a serialization carries and the
// metadata types is read as the class metadata gives it, in n's role. Any other is read as the template
// declares it, single-valued or a list; it is not visited when no
// serialization of the bound node's class carries it.
func declaredAttr(n engineNode, decl *tcimpl.CompiledAttribute, walked *engineAttr) (a engineAttr, visited bool) {
	if walked != nil {
		a = walked.inRole(n.role)
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

// walkedAttr returns the attribute f describes, of a class whose rules are
// rules, nil for a class the type registry does not decode, as the class
// metadata gives it, with no template declaration, whatever the role of the
// node it is read on. The class's table holds it, built once per class
// ([classAttributes]). It is mandatory when the metadata marks it mandatory
// for the class and no rule of the class reports its absence itself; the
// role's part is left to [engineAttr.inRole]. It records whether its
// declared type has RM rules ([engineAttr].noRules).
func walkedAttr(f attributeFacts, rules *classRules) engineAttr {
	declared, _ := rulesFor(f.RMType)
	return engineAttr{
		name:      f.Name,
		rmType:    f.RMType,
		shape:     shapeFrom(f.Container, f.PrimitiveItems, f.Hash),
		mandatory: f.Mandatory && !rules.leavesOut(f.Name),
		noRules:   declared == nil,
	}
}

// inRole returns the attribute a of a class's table as the walk reads it on
// a node in role: no longer mandatory where it is single-valued and role
// lets it be blank ([mayBeBlank]).
func (a *engineAttr) inRole(role floorRole) engineAttr {
	out := *a
	if a.blankIn(role) {
		out.mandatory = false
	}
	return out
}

// blankIn reports whether the attribute a of a class's table, mandatory for
// the class, is not mandatory on a node in role: it is single-valued and
// role lets it be blank ([mayBeBlank]).
func (a *engineAttr) blankIn(role floorRole) bool {
	return a.mandatory && a.shape == shapeSingle && mayBeBlank(role, a.name)
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
// RM values at the path the shared path rule writes for it
// ([resolvePredicates]), and each entry of a Hash at its position in
// ascending key order ([entryPredicate]), never at its key. Only a value of
// a registered RM class ([rmaccess.Covers]) is walked as an item of a list
// or a Hash; an item of a list that holds no RM value is reported at its
// path instead ([engine.walkUnconstrained]), and a counted list is never
// read item by item. A primitive leaf ([primitiveLeaf]) the template does
// not constrain is counted as one visit and not walked
// ([engine.spendLeafVisit]): walking it would do nothing more. The
// attribute's path is pushed onto the walk's path stack and written only
// where something records it ([issuePath]).
func (e *engine) visitAttribute(n *engineNode, class string, a *engineAttr) {
	r := readAttribute(n.value, a)
	if !visitedAt(a, r.present) {
		return
	}
	attrPath := e.path.attribute(n.path, a.name)
	if a.decl == nil && primitiveLeaf(r.value, a) {
		// Walking a primitive leaf would only spend its visit (REQ-171
		// visit limit). Only a present single value is read into r.value,
		// so the mandatory-presence rule reports nothing. No RM class is of
		// a primitive leaf's type, so the leaf is read as its declared type,
		// has no identity, and has no attributes to visit, so the depth limit
		// does not reach it. That type has no RM rules to run or record.
		// So the visit is counted here, and the leaf's path is written only
		// when the visit limit is reported there.
		e.spendLeafVisit(attrPath, a.rmType)
		return
	}
	role := childRole(class, n.role, a.name)
	if a.decl != nil {
		if a.shape == shapeSingle {
			e.visitDeclaredSingle(n, a, &r, attrPath, role)
		} else {
			e.visitDeclaredItems(n, a, &r, attrPath, role)
		}
		return
	}
	if a.mandatory {
		e.found.checkMandatory(n.rmType, a.name, attrPath, a.shape.multiValued(), r.present, r.count)
	}
	if r.present {
		e.walkUnconstrained(n, a, &r, attrPath, role)
	}
}

// walkUnconstrained walks what the present attribute a of node n, read as r,
// holds, at attrPath, in role, with the RM rules alone: no template node
// binds it. An item of a list that holds no RM value, a nil element or a
// typed-nil pointer ([isNilItem]), is not walked. It counts as one visit
// ([engine.spendVisit]): within the visit limit it is reported at its own
// path by the mandatory-presence rule ([findings.checkNilItem]); past the
// limit it is reported as `max_nodes` instead, which ends the walk. Once the
// visit limit has ended the walk, no later item is reported or walked. This is
// the one place the RM rules meet a nil item: the walk with no template and
// an attribute the template leaves open both reach it, while a nil item
// under an attribute that declares children is the template's to refuse
// ([engine.visitDeclaredItems]) and never gets here.
func (e *engine) walkUnconstrained(n *engineNode, a *engineAttr, r *attrReading, attrPath issuePath, role floorRole) {
	switch a.shape {
	case shapeSingle:
		e.walk(engineNode{value: r.value, rmType: runtimeRMType(r.value, a.rmType), path: attrPath, role: role, depth: n.depth + 1})
	case shapeList:
		if !slices.ContainsFunc(r.items, walkedOrNil) {
			return
		}
		from := e.unboundPredicates(r.items)
		for i, item := range r.items {
			if e.stopped {
				break // the visit limit ended the walk at or inside an earlier item
			}
			path := e.path.item(attrPath, e.preds[from+i])
			if isNilItem(item) {
				if e.spendVisit(path, a.rmType) {
					e.found.checkNilItem(n.rmType, a.name, path)
				}
				continue
			}
			e.walkItem(n, item, a.rmType, path, role)
		}
		e.preds = e.preds[:from]
	case shapeHash:
		for i, entry := range r.items {
			e.walkItem(n, entry, a.rmType, e.path.item(attrPath, entryPredicate(i)), role)
		}
	case shapeCounted:
		// Nothing is below the items of a list of a primitive type.
	}
}

// walkItem walks item, an item of a list or an entry of a Hash of node n
// whose declared RM type is rmType, at path, in role, with the RM rules
// alone, when it is a value of a registered RM class; any other item has
// nothing below it to walk.
func (e *engine) walkItem(n *engineNode, item any, rmType string, path issuePath, role floorRole) {
	if !rmaccess.Covers(item) {
		return
	}
	e.walk(engineNode{value: item, rmType: runtimeRMType(item, rmType), path: path, role: role, depth: n.depth + 1})
}

// walkedOrNil reports whether item, an item of a list no template node
// binds, is one the walk has something to do with: a value of a registered
// RM class ([rmaccess.Covers]), which it walks, or an item that holds no RM
// value ([isNilItem]), which the RM rules report.
func walkedOrNil(item any) bool {
	return isNilItem(item) || rmaccess.Covers(item)
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
func (e *engine) visitDeclaredSingle(n *engineNode, a *engineAttr, r *attrReading, attrPath issuePath, role floorRole) {
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
// ([unboundItemIssue]) and not entered; each item bound to a child the
// template does not allow ([allowedChildren]), one whose occurrences upper
// bound is 0, is reported by that child's occurrences, among the counts, and
// not entered. Presence: an absent attribute is
// reported as [engine.checkAbsent] says, and one that is present and holds
// no item as [engine.checkEmpty] says. Counts: the template's child count and
// each child's occurrences. Then each admitted item is walked against the
// child it binds to, at the path the shared path rule writes for it, with
// the name the child pins where a sibling shares its id
// ([appendBoundPredicates]); where the template declares no child, each item
// is walked with the RM rules alone, which report an item that holds no RM
// value ([engine.walkUnconstrained]). A nil item under an attribute that
// declares children binds to none, so it is refused above and the RM rules
// never see it.
func (e *engine) visitDeclaredItems(n *engineNode, a *engineAttr, r *attrReading, attrPath issuePath, role floorRole) {
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
	var admitted []bool
	from := len(e.preds)
	if len(children) > 0 && a.shape != shapeCounted {
		var byNodeID []bool
		bound, byNodeID = bindItems(n.bound.RMTypeName(), a.name, children, r.items)
		e.preds = appendBoundPredicates(e.preds, r.items, bound, a.shape == shapeHash)
		allowed := allowedChildren(a.decl)
		admitted = make([]bool, len(r.items))
		for i, item := range r.items {
			switch {
			case bound[i] == nil:
				e.refuse(unboundItemIssue(a.decl, children, item, byNodeID[i], e.path.item(attrPath, e.preds[from+i])))
			case !slices.Contains(allowed, bound[i]):
				// The child's occurrences, reported with the counts below,
				// are the one report for the item.
				e.refused = append(e.refused, e.path.item(attrPath, e.preds[from+i]).String())
			default:
				admitted[i] = true
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
		e.checkEmpty(n, a, attrPath, countOut || reportsAt(occurrences, attrPath))
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
			if !admitted[i] {
				continue
			}
			e.walk(engineNode{value: item, rmType: runtimeRMType(item, a.rmType), path: e.path.item(attrPath, e.preds[from+i]), role: role, depth: n.depth + 1, bound: bound[i]})
		}
	}
	e.preds = e.preds[:from]
}

// reportsAt reports whether one of issues is at path. The path is written
// only when there is an issue to compare it with.
func reportsAt(issues []Issue, path issuePath) bool {
	if len(issues) == 0 {
		return false
	}
	at := path.String()
	return slices.ContainsFunc(issues, func(iss Issue) bool { return iss.Path == at })
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
// blank. The RM's requirement is read from the value's class alone: an
// attribute that class does not list draws no RM finding, whatever
// mandatory bit the compiled template carries for it.
func (e *engine) checkAbsent(n *engineNode, a *engineAttr, attrPath issuePath) {
	multiValued := a.shape != shapeSingle
	if iss, required := absentIssue(a.decl, n.value, attrPath, multiValued); required {
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
// it, read from the value's class alone as [engine.checkAbsent] reads it, it
// is the RM's `cardinality`, rule cardinality, unless the template reports
// the same container itself (reported), and only for a list: a Hash that is
// present and empty breaks no mandatory rule, as the RM rules alone read
// it, however the template declares it.
func (e *engine) checkEmpty(n *engineNode, a *engineAttr, attrPath issuePath, reported bool) {
	if iss, required := emptyIssue(a.decl, n.value, attrPath); required {
		e.found.emit(iss)
		return
	}
	if a.mandatory && a.shape.multiValued() && !reported {
		e.found.checkMandatory(n.rmType, a.name, attrPath, true, true, 0)
	}
}

// unboundPredicates appends to the walk's predicates ([engine].preds) the
// predicate of each of items, the items of a list no template node binds,
// by the shared path rule ([resolvePredicates]), fed each item's
// archetype_node_id, and returns where they start. No template node binds
// these items, so the rule is fed no pinned name. The caller drops them
// once it has walked the items.
func (e *engine) unboundPredicates(items []any) (from int) {
	from = len(e.preds)
	for _, item := range items {
		e.preds = append(e.preds, predicate{id: locatableArchetypeNodeID(item)})
	}
	resolvePredicates(e.preds[from:], true)
	return from
}
