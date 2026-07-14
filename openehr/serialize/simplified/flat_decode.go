package simplified

// REQ-053 — FLAT decode: rebuild a canonical COMPOSITION from a FLAT map.
// The FLAT key grammar (inverse of flat_encode) is parsed here; the canonical
// RM reconstruction (walking each leaf's Web Template aqlPath, materialising
// the elided HISTORY / ITEM_TREE wrappers via rminfo, then decoding through
// canjson) builds on this parser.

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/rminfo"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/template/webtemplate"
)

// UnmarshalFlat decodes FLAT JSON into a canonical COMPOSITION using wt
// (REQ-053). It rebuilds a canonical-JSON tree from the FLAT entries — node
// types and the elided HISTORY/ITEM_TREE wrappers come from the Web Template
// and rminfo, values from the FLAT suffixes — then decodes it through canjson
// (typereg instantiates the polymorphic RM types).
func UnmarshalFlat(data []byte, wt *webtemplate.WebTemplate) (*rm.Composition, error) {
	if wt == nil || wt.Tree == nil {
		return nil, ErrNoTemplate
	}
	var flat map[string]any
	if err := json.Unmarshal(data, &flat); err != nil {
		return nil, err
	}
	compJSON, err := decodeFlat(flat, wt)
	if err != nil {
		return nil, err
	}
	b, err := json.Marshal(compJSON)
	if err != nil {
		return nil, err
	}
	var comp rm.Composition
	if err := canjson.Unmarshal(b, &comp); err != nil {
		return nil, err
	}
	return &comp, nil
}

// decodeFlat builds the canonical-JSON object for the COMPOSITION from the
// FLAT map, driven by the Web Template.
func decodeFlat(flat map[string]any, wt *webtemplate.WebTemplate) (map[string]any, error) {
	root := wt.Tree
	compJSON := map[string]any{
		"_type":             "COMPOSITION",
		"archetype_node_id": root.NodeID,
		"name":              textJSON(orDefault(root.Name, wt.TemplateID)),
	}
	// Group FLAT keys by leaf instance (key minus the |suffix); each group's
	// suffix->value pairs build one DataValue.
	groups := make(map[string]map[string]any)
	for key, val := range flat {
		base := key
		suffix := ""
		if i := strings.LastIndex(key, "|"); i >= 0 {
			base, suffix = key[:i], key[i+1:]
		}
		if groups[base] == nil {
			groups[base] = make(map[string]any)
		}
		groups[base][suffix] = val
	}
	for base, sfx := range groups {
		pk := parseFlatKey(base)
		leaf, predIndex, predType, ok := resolveLeaf(wt, pk.segs)
		if !ok {
			continue // unknown path — tolerant on input
		}
		dv := dvFromSuffixes(leaf.RMType, sfx)
		if dv == nil {
			continue // datatype not yet mapped (|raw fallback is Task 6)
		}
		placeLeaf(compJSON, leaf.AQLPath, predIndex, predType, dv)
	}
	return compJSON, nil
}

// resolveLeaf walks the Web Template by FLAT segment ids to the leaf node,
// collecting, for each ancestor that carries an archetype node id, its flat
// :index (predIndex) and Web Template rmType (predType), both keyed by that
// node id. Returns ok=false when a segment id is not found.
func resolveLeaf(wt *webtemplate.WebTemplate, segs []flatSeg) (*webtemplate.Node, map[string]int, map[string]string, bool) {
	predIndex := make(map[string]int)
	predType := make(map[string]string)
	node := wt.Tree
	if len(segs) == 0 || segs[0].id != node.ID {
		return nil, nil, nil, false
	}
	for _, seg := range segs[1:] {
		var next *webtemplate.Node
		for _, ch := range node.Children {
			if ch.ID == seg.id {
				next = ch
				break
			}
		}
		if next == nil {
			return nil, nil, nil, false
		}
		if next.NodeID != "" {
			predType[next.NodeID] = next.RMType
			if seg.idx >= 0 {
				predIndex[next.NodeID] = seg.idx
			}
		}
		node = next
	}
	return node, predIndex, predType, true
}

// aqlSeg is one canonical-path segment: an attribute name and an optional
// node predicate (archetype id or at-code).
type aqlSeg struct {
	attr string
	pred string
}

// parseAQL splits a canonical aqlPath into attribute+predicate segments.
func parseAQL(p string) []aqlSeg {
	var out []aqlSeg
	for part := range strings.SplitSeq(strings.TrimPrefix(p, "/"), "/") {
		if part == "" {
			continue
		}
		seg := aqlSeg{attr: part}
		if i := strings.IndexByte(part, '['); i >= 0 && strings.HasSuffix(part, "]") {
			seg.attr = part[:i]
			seg.pred = part[i+1 : len(part)-1]
		}
		out = append(out, seg)
	}
	return out
}

// placeLeaf walks aqlPath from compJSON, materialising the intermediate RM
// nodes (concrete type via rminfo + the Web Template, archetype_node_id from
// the predicate, list position from predIndex), and sets the terminal
// attribute to the leaf DataValue.
func placeLeaf(compJSON map[string]any, aqlPath string, predIndex map[string]int, predType map[string]string, dv map[string]any) {
	segs := parseAQL(aqlPath)
	cur := compJSON
	curType := "COMPOSITION"
	for i, seg := range segs {
		if i == len(segs)-1 {
			cur[seg.attr] = dv
			return
		}
		childType := concreteType(curType, seg.attr, seg.pred, predType)
		if childType == "" {
			return // unknown attribute — cannot place
		}
		container, _ := rminfo.Default.IsContainer(curType, seg.attr)
		if container {
			cur = selectElem(cur, seg.attr, childType, seg.pred, predIndex[seg.pred])
		} else {
			obj, ok := cur[seg.attr].(map[string]any)
			if !ok {
				obj = map[string]any{"_type": childType}
				if seg.pred != "" {
					obj["archetype_node_id"] = seg.pred
				}
				cur[seg.attr] = obj
			}
			cur = obj
		}
		curType = childType
	}
}

// selectElem finds (or creates) the element with archetype_node_id==pred in
// cur[attr]'s list, at the idx-th position among same-pred siblings (idx is
// the flat :index for a repeatable node; 0 otherwise). Distinct sibling node
// ids get distinct elements even without an explicit index.
func selectElem(cur map[string]any, attr, elemType, pred string, idx int) map[string]any {
	arr, _ := cur[attr].([]any)
	want := max(idx, 0)
	var matches []int
	for i, e := range arr {
		if m, ok := e.(map[string]any); ok && m["archetype_node_id"] == pred {
			matches = append(matches, i)
		}
	}
	for len(matches) <= want {
		el := map[string]any{"_type": elemType}
		if pred != "" {
			el["archetype_node_id"] = pred
		}
		arr = append(arr, el)
		matches = append(matches, len(arr)-1)
	}
	cur[attr] = arr
	return arr[matches[want]].(map[string]any)
}

// concreteType resolves the RM type to instantiate for attr on parentType,
// mapping the abstract RM slots to concrete types the way the Web Template /
// canonical form require.
func concreteType(parentType, attr, pred string, predType map[string]string) string {
	t, ok := rminfo.Default.AttributeRMType(parentType, attr)
	if !ok {
		return ""
	}
	switch t {
	case "CONTENT_ITEM":
		if wt := predType[pred]; wt != "" {
			return wt // OBSERVATION / EVALUATION / …
		}
		return "OBSERVATION"
	case "EVENT":
		if predType[pred] == "INTERVAL_EVENT" {
			return "INTERVAL_EVENT"
		}
		return "POINT_EVENT"
	case "T", "ITEM_STRUCTURE":
		return "ITEM_TREE"
	case "ITEM":
		if predType[pred] == "CLUSTER" {
			return "CLUSTER"
		}
		return "ELEMENT"
	default:
		return t // already concrete (HISTORY, …)
	}
}

// dvFromSuffixes builds the canonical-JSON DataValue for a leaf from its FLAT
// suffix->value map (the inverse of leafToFlat). Bare values live under the
// "" suffix. Returns nil for a datatype not yet mapped.
func dvFromSuffixes(rmType string, sfx map[string]any) map[string]any {
	switch rmType {
	case "DV_TEXT":
		return map[string]any{"_type": "DV_TEXT", "value": sfx[""]}
	case "DV_DATE_TIME":
		return map[string]any{"_type": "DV_DATE_TIME", "value": sfx[""]}
	case "DV_DATE":
		return map[string]any{"_type": "DV_DATE", "value": sfx[""]}
	case "DV_TIME":
		return map[string]any{"_type": "DV_TIME", "value": sfx[""]}
	case "DV_QUANTITY":
		return map[string]any{"_type": "DV_QUANTITY", "magnitude": sfx["magnitude"], "units": sfx["unit"]}
	case "DV_CODED_TEXT":
		dc := map[string]any{"_type": "CODE_PHRASE", "code_string": sfx["code"]}
		if t, ok := sfx["terminology"]; ok {
			dc["terminology_id"] = map[string]any{"_type": "TERMINOLOGY_ID", "value": t}
		}
		return map[string]any{"_type": "DV_CODED_TEXT", "value": sfx["value"], "defining_code": dc}
	}
	return nil
}

// textJSON is a canonical DV_TEXT object.
func textJSON(value string) map[string]any {
	return map[string]any{"_type": "DV_TEXT", "value": value}
}

// orDefault returns s if non-empty, else def.
func orDefault(s, def string) string {
	if s != "" {
		return s
	}
	return def
}

// flatSeg is one "/"-separated FLAT path segment: a Web Template id with an
// optional zero-based instance index (idx == -1 when the segment carries no
// :index).
type flatSeg struct {
	id  string
	idx int
}

// parsedKey is a decomposed FLAT key: its path segments and the trailing
// pipe attribute suffix ("" when the key is a bare value).
type parsedKey struct {
	segs   []flatSeg
	suffix string
}

// parseFlatKey splits a FLAT key into path segments and the trailing |suffix.
// Each "/"-separated segment may carry a ":<index>" suffix; a trailing
// "|<attr>" is the leaf attribute suffix.
func parseFlatKey(key string) parsedKey {
	var suffix string
	if i := strings.LastIndex(key, "|"); i >= 0 {
		suffix = key[i+1:]
		key = key[:i]
	}
	parts := strings.Split(key, "/")
	segs := make([]flatSeg, 0, len(parts))
	for _, p := range parts {
		seg := flatSeg{id: p, idx: -1}
		if j := strings.LastIndex(p, ":"); j >= 0 {
			if n, err := strconv.Atoi(p[j+1:]); err == nil {
				seg.id = p[:j]
				seg.idx = n
			}
		}
		segs = append(segs, seg)
	}
	return parsedKey{segs: segs, suffix: suffix}
}
