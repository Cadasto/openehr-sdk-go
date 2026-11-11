package simplified

// REQ-053 — the RM attributes the Simplified Formats spell where the Web
// Template gives them no node (ITS-REST 1.1.0 Simplified Formats § OBSERVATION
// and § INTERVAL_EVENT, issue 424): an INTERVAL_EVENT's `sample_count`, a
// suffix on its event node; an abstract EVENT's `math_function` and `width`,
// leaves under its node, which an INTERVAL_EVENT node already has; and an
// OBSERVATION's `history_origin`, a leaf under its node. The reference
// implementation gives none of them a Web Template node either, so the codec
// reads and writes them itself, and the Web Template stays the reference's.

import (
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
//     ([historyNodeID]).
//
// The leaf is built per call, and its path extends the parent's, so it
// resolves through [reusedSiblings] as the parent does.
func codecLeaf(parent *webtemplate.Node, id string) *webtemplate.Node {
	switch nodeRMType(parent) {
	case "EVENT":
		switch id {
		case mathFunctionSeg:
			return &webtemplate.Node{ID: id, Name: "Math_function", RMType: "DV_CODED_TEXT", Max: 1, AQLPath: parent.AQLPath + "/" + id}
		case widthSeg:
			return &webtemplate.Node{ID: id, Name: "Width", RMType: "DV_DURATION", Max: 1, AQLPath: parent.AQLPath + "/" + id}
		}
	case "OBSERVATION":
		if id != historyOriginSeg {
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

// intervalEventLeaf reports whether leaf, under an abstract EVENT node, is an
// INTERVAL_EVENT attribute, which makes the event it is spelled on an
// INTERVAL_EVENT.
func intervalEventLeaf(parent, leaf *webtemplate.Node) bool {
	return nodeRMType(parent) == "EVENT" && (leaf.ID == mathFunctionSeg || leaf.ID == widthSeg)
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

// carriesSampleCount reports whether a Web Template node takes the
// `|sample_count` suffix: an event node, abstract or an INTERVAL_EVENT.
func carriesSampleCount(node *webtemplate.Node) bool {
	switch nodeRMType(node) {
	case "EVENT", "INTERVAL_EVENT":
		return true
	}
	return false
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
// where it is set, an abstract EVENT node's math_function and width where the
// instance is an INTERVAL_EVENT, and an OBSERVATION's history origin
// ([emitHistoryOrigin]). root and rootAql are the resolution root v's children
// resolve against.
func emitCodecAttrs(out map[string]any, node *webtemplate.Node, flatPath string, v any, root rm.Locatable, rootAql string, sib *reusedSiblings) error {
	switch nodeRMType(node) {
	case "EVENT", "INTERVAL_EVENT":
		ev, ok := as[rm.IntervalEvent[rm.ItemStructure]](v)
		if !ok {
			return nil
		}
		if ev.SampleCount != nil {
			out[flatPath+"|"+sampleCountSuffix] = int64(*ev.SampleCount)
		}
		if nodeRMType(node) != "EVENT" {
			return nil
		}
		for _, id := range []string{mathFunctionSeg, widthSeg} {
			if err := emitNode(out, codecLeaf(node, id), flatPath, root, rootAql, sib); err != nil {
				return err
			}
		}
	case "OBSERVATION":
		obs, ok := as[rm.Observation](v)
		if !ok || codecLeaf(node, historyOriginSeg) == nil {
			return nil
		}
		return emitHistoryOrigin(out, flatPath+"/"+historyOriginSeg, obs.Data)
	}
	return nil
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
