package validation

// bind_trial.go: binding a value among the children of one binding tier, or
// an item of a list among those of one rank, by walking it under each child
// in turn (REQ-102 Structural — alternatives, Structural — binding items). A
// trial is part of the one walk (engine.go): it counts toward the limits,
// and what a trial under a child the value does not bind to found is
// withdrawn from the walk's records (REQ-171 One walk).

import (
	"slices"

	tcimpl "github.com/cadasto/openehr-sdk-go/internal/templatecompile"
)

// walkMark is how far the walk's records reach at one point: the issues, the
// refusals, the coverage entries, the stops, reported or not, and the classes
// visited. A trial returns the records to its mark when what it found is
// withdrawn.
type walkMark struct {
	issues, refused, external, notInRM, stoppedAt, depthCuts int
	depthStopped                                             bool
	visited                                                  indexSet
}

// mark returns how far the walk's records reach now.
func (e *engine) mark() walkMark {
	return walkMark{
		issues:       len(e.found.issues),
		refused:      len(e.refused),
		external:     len(e.external),
		notInRM:      len(e.notInRM),
		stoppedAt:    len(e.stoppedAt),
		depthCuts:    e.depthCuts,
		depthStopped: e.depthStopped,
		visited:      slices.Clone(e.found.visited),
	}
}

// rollback withdraws everything the walk recorded since m. The visits stay
// counted.
func (e *engine) rollback(m walkMark) {
	e.truncate(m)
	e.stoppedAt = e.stoppedAt[:m.stoppedAt]
	e.depthCuts = m.depthCuts
	e.depthStopped = m.depthStopped
}

// truncate withdraws the issues, the refusals, the coverage entries and the
// classes visited the walk recorded since m, its stops apart.
func (e *engine) truncate(m walkMark) {
	clear(e.found.issues[m.issues:])
	e.found.issues = e.found.issues[:m.issues]
	e.refused = e.refused[:m.refused]
	e.external = e.external[:m.external]
	e.notInRM = e.notInRM[:m.notInRM]
	e.found.visited = append(e.found.visited[:0], m.visited...)
}

// walkSegment is what the walk recorded during one trial, set aside while
// the walk tries the next child.
type walkSegment struct {
	issues                              []Issue
	refused, external, notInRM, stopped []string
	depthCuts                           int
	depthStopped                        bool
	visited                             indexSet
}

// setAside withdraws what the walk recorded since m and returns it, so that
// [engine.restore] can record it again.
func (e *engine) setAside(m walkMark) walkSegment {
	s := walkSegment{
		issues:       slices.Clone(e.found.issues[m.issues:]),
		refused:      slices.Clone(e.refused[m.refused:]),
		external:     slices.Clone(e.external[m.external:]),
		notInRM:      slices.Clone(e.notInRM[m.notInRM:]),
		stopped:      slices.Clone(e.stoppedAt[m.stoppedAt:]),
		depthCuts:    e.depthCuts - m.depthCuts,
		depthStopped: e.depthStopped,
		visited:      slices.Clone(e.found.visited),
	}
	e.rollback(m)
	return s
}

// restore records s again, at the mark it was set aside from, which the walk
// has returned to.
func (e *engine) restore(s walkSegment) {
	e.found.issues = append(e.found.issues, s.issues...)
	e.refused = append(e.refused, s.refused...)
	e.external = append(e.external, s.external...)
	e.notInRM = append(e.notInRM, s.notInRM...)
	e.stoppedAt = append(e.stoppedAt, s.stopped...)
	e.depthCuts += s.depthCuts
	e.depthStopped = s.depthStopped
	e.found.visited = append(e.found.visited[:0], s.visited...)
}

// keepStops withdraws what the walk recorded since m, the `max_depth` and
// `max_nodes` issues and their stops excepted. The visit limit ended the walk
// inside a trial, so no binding was decided there.
func (e *engine) keepStops(m walkMark) {
	kept := e.found.issues[:m.issues]
	for _, iss := range e.found.issues[m.issues:] {
		if iss.Code == "max_depth" || iss.Code == "max_nodes" {
			kept = append(kept, iss)
		}
	}
	clear(e.found.issues[len(kept):])
	e.found.issues = kept
	e.truncate(walkMark{
		issues:   len(kept),
		refused:  m.refused,
		external: m.external,
		notInRM:  m.notInRM,
		visited:  m.visited,
	})
}

// validatesSince reports whether the walk recorded no error finding since m,
// nothing that makes [Result.OK] false, and the depth limit stopped it
// nowhere since m. A stop of the depth limit is reported once per walk, so
// one an earlier part of the walk reported leaves none to report since m;
// the count of stops tells it all the same.
func (e *engine) validatesSince(m walkMark) bool {
	return e.depthCuts == m.depthCuts && !slices.ContainsFunc(e.found.issues[m.issues:], func(iss Issue) bool { return iss.Severity == Error })
}

// bindAmong binds a value to one of candidates, the children of one binding
// tier or rank, in OPT order, and returns the child (REQ-102). room reports
// whether the candidate at an index has room for the value, the occurrences
// of a list item's child; nil gives every candidate room, as for the value
// of a single-valued attribute. trial walks the value under a candidate.
//
// The value binds to the first candidate that has room and under which it
// validates, the walk under it recording no error finding; failing that, to
// the first that has room; failing that, to the first. Only where two or
// more candidates have room is a trial needed; otherwise the value binds to
// the one with room, or to the first, and bindAmong walks nothing.
//
// With keep, the walk under the bound candidate stays recorded: the trial
// that validated, or, where none did, the trial under the first candidate
// with room, set aside while the others ran. So a value under k candidates
// costs at most k walks. Without keep, every trial is withdrawn and the
// caller walks the value under the bound candidate itself, which it does
// for a list item, whose path waits on the binding of the items beside it.
// What a trial under a candidate the value does not bind to recorded is
// withdrawn in both cases; its visits stay counted.
//
// Where the visit limit ends the walk inside a trial, no binding is decided:
// bindAmong withdraws what the trials recorded, the `max_depth` and
// `max_nodes` issues of the trial the walk ended in excepted, and returns
// nil.
func (e *engine) bindAmong(candidates []*tcimpl.CompiledNode, room func(int) bool, trial func(*tcimpl.CompiledNode), keep bool) *tcimpl.CompiledNode {
	first, withRoom := -1, 0
	for i := range candidates {
		if room == nil || room(i) {
			if first < 0 {
				first = i
			}
			withRoom++
		}
	}
	switch withRoom {
	case 0:
		return candidates[0]
	case 1:
		return candidates[first]
	}
	var firstFound walkSegment
	for i, c := range candidates {
		if room != nil && !room(i) {
			continue
		}
		m := e.mark()
		trial(c)
		switch {
		case e.stopped:
			e.keepStops(m)
			return nil
		case e.validatesSince(m):
			if !keep {
				e.rollback(m)
			}
			return c
		case keep && i == first:
			firstFound = e.setAside(m)
		default:
			e.rollback(m)
		}
	}
	if keep {
		e.restore(firstFound)
	}
	return candidates[first]
}

// bindItems binds each of items, the items of the multi-valued attribute a
// of node n, to one of children, the attribute's OPT children, and returns
// for each item the child it binds to, nil when none does, and whether it
// binds by its archetype_node_id ([bindsByNodeID]). An item binds among the
// children of its rank ([itemRank]): a rank of one binds it to that child.
// Where an item's rank holds two or more children, [engine.bindRanked]
// rebinds the items of ranks of two or more, in item order, by
// [engine.bindAmong]. The walk at attrPath, in role, is where those trials
// run. Where the visit limit ends the walk inside a trial, bindItems
// returns with the walk stopped and the binding not decided.
func (e *engine) bindItems(n *engineNode, a *engineAttr, children []*tcimpl.CompiledNode, items []any, attrPath issuePath, role floorRole) (bound []*tcimpl.CompiledNode, byNodeID []bool) {
	bound = make([]*tcimpl.CompiledNode, len(items))
	byNodeID = make([]bool, len(items))
	parent := n.bound.RMTypeName()
	var buf [8]*tcimpl.CompiledNode
	ranked := false
	for i, item := range items {
		byNodeID[i] = bindsByNodeID(item, parent, a.name)
		rank := itemRank(buf[:0], children, item, byNodeID[i])
		if len(rank) > 0 {
			bound[i] = rank[0]
		}
		ranked = ranked || len(rank) > 1
	}
	if ranked {
		e.bindRanked(n, a, children, items, bound, byNodeID, attrPath, role)
	}
	return bound, byNodeID
}

// bindRanked rebinds, in item order, each of items whose rank holds two or
// more children by [engine.bindAmong], counting per child the items bound to
// it before, so that a child has room while that count is below its
// occurrences upper bound ([hasRoom]). bound holds each item bound to the
// first child of its rank, and comes back with the binding; byNodeID holds
// how each item binds. Each trial walks the item at the path the shared path
// rule writes for it under that first binding, against the child tried; the
// trials are withdrawn, and the caller walks each item once its binding,
// and so its path, is known.
func (e *engine) bindRanked(n *engineNode, a *engineAttr, children []*tcimpl.CompiledNode, items []any, bound []*tcimpl.CompiledNode, byNodeID []bool, attrPath issuePath, role floorRole) {
	from := len(e.preds)
	e.preds = appendBoundPredicates(e.preds, items, bound, a.shape == shapeHash)
	counts := make([]int, len(children))
	var buf [8]*tcimpl.CompiledNode
	for i, item := range items {
		rank := itemRank(buf[:0], children, item, byNodeID[i])
		if len(rank) > 1 {
			path := e.path.item(attrPath, e.preds[from+i])
			bound[i] = e.bindAmong(rank,
				func(j int) bool { return hasRoom(rank[j], counts[slices.Index(children, rank[j])]) },
				func(c *tcimpl.CompiledNode) {
					e.walk(engineNode{value: item, rmType: runtimeRMType(item, a.rmType), path: path, role: role, depth: n.depth + 1, bound: c})
				},
				false)
			if e.stopped {
				break
			}
		}
		if bound[i] != nil {
			counts[slices.Index(children, bound[i])]++
		}
	}
	e.preds = e.preds[:from]
}
