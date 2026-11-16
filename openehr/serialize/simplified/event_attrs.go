package simplified

// REQ-053 — the RM attributes the Simplified Formats spell where the Web
// Template gives them no node (ITS-REST 1.1.0 Simplified Formats § OBSERVATION
// and § INTERVAL_EVENT, issue 424): an INTERVAL_EVENT's `sample_count`, a
// suffix on its event node; an abstract EVENT's `math_function` and `width`,
// leaves under its node, which an INTERVAL_EVENT node already has; and an
// OBSERVATION's `history_origin`, a leaf under its node. An abstract EVENT the
// Web Template folds into its OBSERVATION has no node, so its three are
// spelled at the OBSERVATION's (issue 456). The reference implementation gives
// none of them a Web Template node either, so the codec reads and writes them
// itself, and the Web Template stays the reference's.

import (
	"fmt"
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
//   - under an OBSERVATION node that folds in an abstract EVENT
//     ([foldedEvent]), that event's `math_function` and `width`, as under the
//     event's node.
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

// intervalAttrLeaf returns the leaf of the INTERVAL_EVENT attribute id of the
// abstract EVENT at eventPath, `math_function` or `width`, or nil for any
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

// intervalEventOf returns the path of the abstract EVENT whose INTERVAL_EVENT
// attribute leaf is, a [codecLeaf] under parent: the attribute makes the event
// an INTERVAL_EVENT. It reports false for any other leaf.
func intervalEventOf(parent, leaf *webtemplate.Node) (string, bool) {
	switch nodeRMType(parent) {
	case "EVENT", "OBSERVATION":
		if leaf.ID == mathFunctionSeg || leaf.ID == widthSeg {
			return strings.TrimSuffix(leaf.AQLPath, "/"+leaf.ID), true
		}
	}
	return "", false
}

// foldedEvent returns the abstract EVENT an OBSERVATION node folds in. The Web
// Template gives an abstract EVENT that occurs at most once no node: it lifts
// the event's children into the OBSERVATION's node, its `time` leaf among
// them, each at its path through the event ([webtemplate.Build]). The node
// returned stands for the event and holds its path, read off that `time`
// leaf. It reports false where obs folds in no event, or more than one.
func foldedEvent(obs *webtemplate.Node) (*webtemplate.Node, bool) {
	depth := len(parseAQL(obs.AQLPath))
	path := ""
	for _, ch := range obs.Children {
		segs := parseAQL(ch.AQLPath)
		if len(segs) != depth+3 || segs[depth].attr != "data" || segs[depth+1].attr != "events" || segs[depth+2].attr != "time" {
			continue
		}
		event := ch.AQLPath[:segs[depth+1].end]
		if path != "" && path != event {
			return nil, false
		}
		path = event
	}
	if path == "" {
		return nil, false
	}
	return &webtemplate.Node{RMType: "EVENT", Max: 1, AQLPath: path}, true
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
// takes as a suffix: an event node's own, abstract or an INTERVAL_EVENT, and
// an OBSERVATION node's that folds in an abstract EVENT ([foldedEvent]). It
// reports false for any other node.
func sampleCountEvent(node *webtemplate.Node) (*webtemplate.Node, bool) {
	switch nodeRMType(node) {
	case "EVENT", "INTERVAL_EVENT":
		return node, true
	case "OBSERVATION":
		return foldedEvent(node)
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
// where it is set, an abstract EVENT's math_function and width where the
// instance is an INTERVAL_EVENT, at its node or at the OBSERVATION node that
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
// `|sample_count` under flatPath. Where a child of node has the id of one of
// those leaves, the key is the child's, and decode reads it as the child, so
// the attribute, which an INTERVAL_EVENT always holds, has no key: it is
// refused, wrapping [ErrUnsupportedDatatype], rather than written where it
// would be read as the child or dropped.
func emitIntervalAttrs(out map[string]any, node *webtemplate.Node, flatPath string, ev rm.IntervalEvent[rm.ItemStructure], root rm.Locatable, rootAql string, sib *reusedSiblings) error {
	emitSampleCount(out, flatPath, ev)
	for _, id := range []string{mathFunctionSeg, widthSeg} {
		if childByID(node, id) != nil {
			return fmt.Errorf("%w: %q is the key of a child of its node, so the INTERVAL_EVENT's %s has no key there",
				ErrUnsupportedDatatype, flatPath+"/"+id, id)
		}
		if err := emitNode(out, codecLeaf(node, id), flatPath, root, rootAql, sib); err != nil {
			return err
		}
	}
	return nil
}

// emitFoldedEvent writes the INTERVAL_EVENT attributes of the event an
// OBSERVATION node folds in ([foldedEvent]) under flatPath, the OBSERVATION's,
// where the instance there is an INTERVAL_EVENT. root, the OBSERVATION, and
// rootAql are the resolution root the event resolves against.
func emitFoldedEvent(out map[string]any, node *webtemplate.Node, flatPath string, root rm.Locatable, rootAql string, sib *reusedSiblings) error {
	event, folded := foldedEvent(node)
	if !folded {
		return nil
	}
	relPath := strings.TrimPrefix(sib.nodePath(event), rootAql)
	v, err := scope{root: root, path: rootAql, sib: sib}.itemAt(relPath)
	if err != nil {
		return skipNotFound(err, relPath)
	}
	ev, ok := as[rm.IntervalEvent[rm.ItemStructure]](v)
	if !ok {
		return nil
	}
	return emitIntervalAttrs(out, node, flatPath, ev, root, rootAql, sib)
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
