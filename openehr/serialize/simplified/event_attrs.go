package simplified

// REQ-053 — the RM attributes the Simplified Formats spell where the Web
// Template gives them no node (ITS-REST 1.1.0 Simplified Formats § OBSERVATION
// and § INTERVAL_EVENT, issue 424): an INTERVAL_EVENT's `sample_count`, a
// suffix on its event node; an abstract EVENT's `math_function` and `width`,
// leaves under its node, which an INTERVAL_EVENT node already has; and an
// OBSERVATION's `history_origin`, a leaf under its node. An event the Web
// Template folds into its OBSERVATION, its HISTORY's only event at max 1 of
// any type (issue 452), has no node, so its three are spelled at the
// OBSERVATION's (issue 456), where a folded INTERVAL_EVENT has its
// `math_function` and `width` as children; where it folds in more than one,
// those keys name none of them. With the template, a folded event it types
// POINT_EVENT reads none of the three, as the reference gives them no node
// there (issue 452); without it, a folded POINT_EVENT reads them as a folded
// abstract EVENT does, since the Web Template does not tell the two apart.
// The reference implementation gives none of them a Web Template node either,
// so the codec reads and writes them itself, and the Web Template stays the
// reference's.

import (
	"fmt"
	"slices"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/template/webtemplate"
)

// The FLAT spellings of the attributes, each the RM attribute's own name.
const (
	sampleCountSuffix = "sample_count"
	mathFunctionSeg   = "math_function"
	widthSeg          = "width"
	historyOriginSeg  = "history_origin"
)

// sampleCountFit is the fit of `|sample_count`: an RM Integer.
var sampleCountFit = valueFit{attr: "INTERVAL_EVENT.sample_count", bmmType: "Integer"}

// codecLeaf returns the leaf the codec reads at id under parent, a Web Template
// node with no child of that id, or nil where it reads none:
//
//   - under an abstract EVENT node, `math_function` (a DV_CODED_TEXT) and
//     `width` (a DV_DURATION), the INTERVAL_EVENT attributes an INTERVAL_EVENT
//     node has as children;
//   - under an OBSERVATION node, `history_origin`, the DV_DATE_TIME at
//     `data/origin`, where the node's events name the HISTORY's node id
//     ([historyNodeID]);
//   - under an OBSERVATION node that folds in an event ([foldedEvent]), that
//     event's `math_function` and `width`, as under an abstract EVENT's node.
//
// The leaf is built per call, and its path extends the parent's, so it
// resolves through [reusedSiblings] as the parent does.
func codecLeaf(parent *webtemplate.Node, id string) *webtemplate.Node {
	switch nodeRMType(parent) {
	case "EVENT":
		return intervalAttrLeaf(parent.AQLPath, id)
	case "OBSERVATION":
		if id != historyOriginSeg {
			if event, folded := foldedEvent(parent); folded {
				return intervalAttrLeaf(event.AQLPath, id)
			}
			return nil
		}
		if history, ok := historyNodeID(parent); ok {
			return &webtemplate.Node{
				ID: id, Name: "History_origin", RMType: "DV_DATE_TIME", Max: 1,
				AQLPath: parent.AQLPath + "/data[" + history + "]/origin",
			}
		}
	}
	return nil
}

// decodeCodecLeaf is [codecLeaf] for a decode with tmpl, the compiled
// template's index, nil without the template: it returns nil for an
// INTERVAL_EVENT attribute of an event the template types POINT_EVENT
// ([templateIndex.typesPointEvent]). Such an event has none, and the
// reference has no node for the key there.
func decodeCodecLeaf(parent *webtemplate.Node, id string, tmpl *templateIndex) *webtemplate.Node {
	leaf := codecLeaf(parent, id)
	if leaf == nil {
		return nil
	}
	if event, ok := intervalEventOf(parent, leaf); ok && tmpl.typesPointEvent(event) {
		return nil
	}
	return leaf
}

// intervalAttrLeaf returns the leaf of the INTERVAL_EVENT attribute id of the
// abstract EVENT or folded event at eventPath, `math_function` or `width`, or nil for any
// other id.
func intervalAttrLeaf(eventPath, id string) *webtemplate.Node {
	switch id {
	case mathFunctionSeg:
		return &webtemplate.Node{ID: id, Name: "Math_function", RMType: "DV_CODED_TEXT", Max: 1, AQLPath: eventPath + "/" + id}
	case widthSeg:
		return &webtemplate.Node{ID: id, Name: "Width", RMType: "DV_DURATION", Max: 1, AQLPath: eventPath + "/" + id}
	}
	return nil
}

// intervalEventOf returns the path of the abstract or folded event whose
// INTERVAL_EVENT attribute leaf is, a [codecLeaf] under parent: the attribute
// makes the event an INTERVAL_EVENT. It reports false for any other leaf.
func intervalEventOf(parent, leaf *webtemplate.Node) (string, bool) {
	switch nodeRMType(parent) {
	case "EVENT", "OBSERVATION":
		if leaf.ID == mathFunctionSeg || leaf.ID == widthSeg {
			return strings.TrimSuffix(leaf.AQLPath, "/"+leaf.ID), true
		}
	}
	return "", false
}

// typesPointEvent reports whether the template types the event at eventPath a
// POINT_EVENT, which holds none of the INTERVAL_EVENT attributes the codec
// reads ([codecLeaf], [sampleCountEvent]). eventPath is the event's Web
// Template path, through one of its OBSERVATION's HISTORY attributes
// ([isObservationHistory]) and an `events[…]` segment. The compiled template
// keys the event without the HISTORY's node id, as both attributes are
// single-valued ([buildTemplateIndex]). A nil index, the one of a
// decode without the template, types no event: there the Web Template alone
// cannot tell a folded POINT_EVENT from a folded abstract EVENT, since it
// gives either one leaf of its own, its `time`, at the OBSERVATION.
func (x *templateIndex) typesPointEvent(eventPath string) bool {
	if x == nil {
		return false
	}
	segs := parseAQL(eventPath)
	n := len(segs)
	if n < 2 || !isObservationHistory(segs[n-2].attr) || segs[n-1].attr != "events" {
		return false
	}
	start := 0
	if n > 2 {
		start = segs[n-3].end
	}
	return x.rmType(eventPath[:start]+"/"+segs[n-2].attr+eventPath[segs[n-2].end:]) == "POINT_EVENT"
}

// isObservationHistory reports whether attr is an OBSERVATION attribute that
// holds a HISTORY, `data` or `state`: the Web Template lifts the only event at
// max 1 out of either into the OBSERVATION's node.
func isObservationHistory(attr string) bool {
	return attr == "data" || attr == "state"
}

// foldedEvent returns the event an OBSERVATION node folds in, where
// it folds in exactly one ([foldedEventPaths]). The node returned stands for
// the event and holds its path. It reports false where obs folds in no event,
// or more than one, whose keys at the OBSERVATION name none of them.
func foldedEvent(obs *webtemplate.Node) (*webtemplate.Node, bool) {
	paths := foldedEventPaths(obs)
	if len(paths) != 1 {
		return nil, false
	}
	return foldedEventNode(paths[0]), true
}

// foldedEventNode is the node that stands for the folded event at path.
func foldedEventNode(path string) *webtemplate.Node {
	return &webtemplate.Node{RMType: "EVENT", Max: 1, AQLPath: path}
}

// foldedEventPaths returns the paths of the events an OBSERVATION node folds
// in, in the order of their `time` leaves. The Web Template gives an event
// that is its HISTORY's only event and occurs at most once no node, whatever
// its type (issue 452): it lifts the event's children into the OBSERVATION's
// node, its `time` leaf among them, each at its path through the event
// ([webtemplate.Build]), so each path is read off a `time` leaf.
func foldedEventPaths(obs *webtemplate.Node) []string {
	depth := len(parseAQL(obs.AQLPath))
	var paths []string
	for _, ch := range obs.Children {
		event, folded := foldedEventOf(depth, ch)
		if !folded || ch.AQLPath != event+"/time" {
			continue
		}
		if !slices.Contains(paths, event) {
			paths = append(paths, event)
		}
	}
	return paths
}

// foldedEventOf returns the path of the folded event that ch, a child of an
// OBSERVATION node whose path has depth segments, sits below: the child's
// path runs through one of the OBSERVATION's HISTORY attributes
// ([isObservationHistory]) and an `events[…]` segment with more below it. It
// reports false for any other child, an event node of its own among them.
func foldedEventOf(depth int, ch *webtemplate.Node) (string, bool) {
	segs := parseAQL(ch.AQLPath)
	if len(segs) < depth+3 || !isObservationHistory(segs[depth].attr) || segs[depth+1].attr != "events" {
		return "", false
	}
	return ch.AQLPath[:segs[depth+1].end], true
}

// markFoldedIntervalEvent records in predType, under its bare path, that the
// event an OBSERVATION node folds in is an INTERVAL_EVENT, where child, one of
// the node's children, sits below that event and the event is one. The Web
// Template lifts a folded INTERVAL_EVENT's in-context `width` leaf into the
// OBSERVATION with its other children, and no other event has one, so the
// event takes the type its template gives whatever key reaches it. Any other
// folded event decodes as the POINT_EVENT [concreteType] gives, unless a key
// spells one of its interval attributes ([decodeCodecLeaf]), which one the
// template types POINT_EVENT does not read.
func markFoldedIntervalEvent(obs, child *webtemplate.Node, predType map[string]string) {
	if nodeRMType(obs) != "OBSERVATION" || !strings.Contains(strings.TrimPrefix(child.AQLPath, obs.AQLPath), "/events[") {
		return
	}
	event, folded := foldedEventOf(len(parseAQL(obs.AQLPath)), child)
	if folded && slices.ContainsFunc(obs.Children, func(ch *webtemplate.Node) bool { return ch.AQLPath == event+"/"+widthSeg }) {
		predType[bareAQLPath(event)] = "INTERVAL_EVENT"
	}
}

// historyNodeID returns the node id of the HISTORY an OBSERVATION node's
// `data` holds, read off the paths of the node's children below it (its
// events). It reports false where no child runs through `data`, or where two
// children name different ones.
func historyNodeID(obs *webtemplate.Node) (string, bool) {
	depth := len(parseAQL(obs.AQLPath))
	id := ""
	for _, ch := range obs.Children {
		segs := parseAQL(ch.AQLPath)
		if len(segs) <= depth || segs[depth].attr != "data" || segs[depth].pred == "" {
			continue
		}
		if id != "" && id != segs[depth].pred {
			return "", false
		}
		id = segs[depth].pred
	}
	return id, id != ""
}

// sampleCountEvent returns the event whose `|sample_count` a Web Template node
// takes as a suffix in a decode with tmpl, the compiled template's index, nil
// without the template: an event node's own, abstract or an INTERVAL_EVENT,
// and an OBSERVATION node's that folds in an event ([foldedEvent]), unless
// tmpl types that event POINT_EVENT ([templateIndex.typesPointEvent]). It
// reports false for any other node.
func sampleCountEvent(node *webtemplate.Node, tmpl *templateIndex) (*webtemplate.Node, bool) {
	switch nodeRMType(node) {
	case "EVENT", "INTERVAL_EVENT":
		return node, true
	case "OBSERVATION":
		event, folded := foldedEvent(node)
		if !folded || tmpl.typesPointEvent(event.AQLPath) {
			return nil, false
		}
		return event, true
	}
	return nil, false
}

// placeSampleCount places v, the `|sample_count` of the event at eventPath
// (its resolution path; eventNames, its path as the Web Template spells it),
// keyed key in the body: an Integer the event holds, which makes it an
// INTERVAL_EVENT. A null is absent (REQ-053), and a value the RM Integer does
// not admit is refused, naming key.
func placeSampleCount(compJSON map[string]any, eventPath, eventNames string, predIndex map[string]int, predType map[string]string, v any, key string, budget *allocBudget, tmpl *templateIndex, sib *reusedSiblings) error {
	if v == nil {
		return nil
	}
	if err := sampleCountFit.check(func(string) string { return key }, sampleCountSuffix, v); err != nil {
		return err
	}
	predType[bareAQLPath(eventNames)] = "INTERVAL_EVENT"
	_, err := placeLeaf(compJSON, eventPath+"/"+sampleCountSuffix, eventNames+"/"+sampleCountSuffix, predIndex, predType, v, budget, tmpl, sib)
	return err
}

// emitCodecAttrs writes the attributes [codecLeaf] and `|sample_count` name, of
// v, the instance at node, under flatPath: an INTERVAL_EVENT's sample_count
// where it is set, an abstract or folded event's math_function and width where
// the instance is an INTERVAL_EVENT, at its node or at the OBSERVATION node that
// folds it in ([foldedEvent]), and an OBSERVATION's history origin
// ([emitHistoryOrigin]). root and rootAql are the resolution root v's children
// resolve against.
func emitCodecAttrs(out map[string]any, node *webtemplate.Node, flatPath string, v any, root rm.Locatable, rootAql string, sib *reusedSiblings) error {
	switch nodeRMType(node) {
	case "EVENT", "INTERVAL_EVENT":
		ev, ok := as[rm.IntervalEvent[rm.ItemStructure]](v)
		if !ok {
			return nil
		}
		if nodeRMType(node) != "EVENT" {
			emitSampleCount(out, flatPath, ev)
			return nil
		}
		return emitIntervalAttrs(out, node, flatPath, ev, root, rootAql, sib)
	case "OBSERVATION":
		obs, ok := as[rm.Observation](v)
		if !ok {
			return nil
		}
		if err := emitFoldedEvent(out, node, flatPath, root, rootAql, sib); err != nil {
			return err
		}
		if codecLeaf(node, historyOriginSeg) == nil {
			return nil
		}
		return emitHistoryOrigin(out, flatPath+"/"+historyOriginSeg, obs.Data)
	}
	return nil
}

// emitSampleCount writes ev's sample_count at flatPath, where it is set.
func emitSampleCount(out map[string]any, flatPath string, ev rm.IntervalEvent[rm.ItemStructure]) {
	if ev.SampleCount != nil {
		out[flatPath+"|"+sampleCountSuffix] = int64(*ev.SampleCount)
	}
}

// emitIntervalAttrs writes ev, an INTERVAL_EVENT at the abstract EVENT node or
// the OBSERVATION node that folds it in, as node's [codecLeaf] spellings and
// `|sample_count` under flatPath. Where a child of node is the attribute's own
// leaf, at its path, as a folded INTERVAL_EVENT's in-context leaves are, the
// node walk writes it, and so does not this. Where another child of node has
// the id of one of those leaves, the key is the child's, and decode reads it
// as the child, so the attribute, which an INTERVAL_EVENT always holds, has no
// key: it is refused, wrapping [ErrUnsupportedDatatype], rather than written
// where it would be read as the child or dropped.
func emitIntervalAttrs(out map[string]any, node *webtemplate.Node, flatPath string, ev rm.IntervalEvent[rm.ItemStructure], root rm.Locatable, rootAql string, sib *reusedSiblings) error {
	emitSampleCount(out, flatPath, ev)
	for _, id := range []string{mathFunctionSeg, widthSeg} {
		leaf := codecLeaf(node, id)
		if slices.ContainsFunc(node.Children, func(ch *webtemplate.Node) bool { return ch.AQLPath == leaf.AQLPath }) {
			continue
		}
		if childByID(node, id) != nil {
			return fmt.Errorf("%w: %q is the key of a child of its node, so the INTERVAL_EVENT's %s has no key there",
				ErrUnsupportedDatatype, flatPath+"/"+id, id)
		}
		if err := emitNode(out, leaf, flatPath, root, rootAql, sib); err != nil {
			return err
		}
	}
	return nil
}

// emitFoldedEvent writes the INTERVAL_EVENT attributes of the event an
// OBSERVATION node folds in ([foldedEvent]) under flatPath, the OBSERVATION's,
// where the instance there is an INTERVAL_EVENT. root, the OBSERVATION, and
// rootAql are the resolution root the event resolves against.
//
// Where the node folds in more than one event, its keys name none of them,
// and decode reads them as unmatched content: an INTERVAL_EVENT among them
// that holds an attribute the keys would carry is refused, wrapping
// [ErrUnsupportedDatatype] and naming the first such key, rather than written
// where no event takes it or dropped.
func emitFoldedEvent(out map[string]any, node *webtemplate.Node, flatPath string, root rm.Locatable, rootAql string, sib *reusedSiblings) error {
	paths := foldedEventPaths(node)
	for _, path := range paths {
		ev, ok, err := foldedIntervalEvent(path, root, rootAql, sib)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		if len(paths) == 1 {
			return emitIntervalAttrs(out, node, flatPath, ev, root, rootAql, sib)
		}
		attr, err := heldIntervalAttr(path, flatPath, ev, root, rootAql, sib)
		if err != nil {
			return err
		}
		if attr != "" {
			key := flatPath + "|" + sampleCountSuffix
			if attr != sampleCountSuffix {
				key = flatPath + "/" + attr
			}
			return fmt.Errorf("%w: %q names no single event at an OBSERVATION that folds in %d events, so an INTERVAL_EVENT's %s has no key there",
				ErrUnsupportedDatatype, key, len(paths), attr)
		}
	}
	return nil
}

// foldedIntervalEvent returns the instance of the folded event at path, under
// root, where it is an INTERVAL_EVENT. It reports false where the instance is
// absent or of another type.
func foldedIntervalEvent(path string, root rm.Locatable, rootAql string, sib *reusedSiblings) (rm.IntervalEvent[rm.ItemStructure], bool, error) {
	relPath := strings.TrimPrefix(sib.nodePath(foldedEventNode(path)), rootAql)
	v, err := scope{root: root, path: rootAql, sib: sib}.itemAt(relPath)
	if err != nil {
		return rm.IntervalEvent[rm.ItemStructure]{}, false, skipNotFound(err, relPath)
	}
	ev, ok := as[rm.IntervalEvent[rm.ItemStructure]](v)
	return ev, ok, nil
}

// heldIntervalAttr returns the first of width, math_function and sample_count
// that encode would write under flatPath for ev, the INTERVAL_EVENT of the
// folded event at eventPath, or "" where it writes none of them.
func heldIntervalAttr(eventPath, flatPath string, ev rm.IntervalEvent[rm.ItemStructure], root rm.Locatable, rootAql string, sib *reusedSiblings) (string, error) {
	for _, id := range []string{widthSeg, mathFunctionSeg} {
		probe := make(map[string]any)
		if err := emitNode(probe, intervalAttrLeaf(eventPath, id), flatPath, root, rootAql, sib); err != nil {
			return "", err
		}
		if len(probe) > 0 {
			return id, nil
		}
	}
	if ev.SampleCount != nil {
		return sampleCountSuffix, nil
	}
	return "", nil
}

// emitHistoryOrigin writes h's origin at flatPath, unless it is the origin
// decode gives a body with no history_origin: the earliest time h's events
// carry, as written, and nothing else. An origin no event time stands for, or
// one with decorations, is written, so the round trip keeps it.
func emitHistoryOrigin(out map[string]any, flatPath string, h rm.History[rm.ItemStructure]) error {
	if earliest, ok := earliestTimeOf(h.Events); ok && h.Origin.Value == earliest {
		m, err := canonicalMap(h.Origin)
		if err != nil {
			return err
		}
		if len(m) == 2 { // _type and value alone
			return nil
		}
	}
	return leafToFlat(out, flatPath, h.Origin, "DV_DATE_TIME", false)
}

// earliestTimeOf returns, as written, the earliest time events carry, compared
// as instants ([eventInstant]), and whether any carries one: what
// [earliestEventTime] reads off a decoded HISTORY.
func earliestTimeOf(events []rm.Event) (string, bool) {
	var earliest string
	var least float64
	found := false
	for _, e := range events {
		value := eventTime(e)
		if instant, ok := eventInstant(value); ok && (!found || instant < least) {
			earliest, least, found = value, instant, true
		}
	}
	return earliest, found
}

// eventTime returns the time of e as written, "" for an event of a type the
// codec does not walk.
func eventTime(e rm.Event) string {
	if p, ok := as[rm.PointEvent[rm.ItemStructure]](e); ok {
		return p.Time.Value
	}
	if i, ok := as[rm.IntervalEvent[rm.ItemStructure]](e); ok {
		return i.Time.Value
	}
	return ""
}
