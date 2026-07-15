package simplified

// REQ-053 — FLAT decode: rebuild a canonical COMPOSITION from a FLAT map.
// The FLAT key grammar (inverse of flat_encode) is parsed here; the canonical
// RM reconstruction (walking each leaf's Web Template aqlPath, materialising
// the elided HISTORY / ITEM_TREE wrappers via rminfo, then decoding through
// canjson) builds on this parser.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/rminfo"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/template/webtemplate"
)

// maxRepeatIndex bounds a FLAT :index during decode/interconversion. A FLAT key
// such as "node:1000000000" would otherwise grow a slice to that length before
// any real data is placed; clinical repeats are small, so a generous cap turns
// a hostile or corrupt key into an error instead of an allocation blow-up.
const maxRepeatIndex = 100_000

// UnmarshalFlat decodes FLAT JSON into a canonical COMPOSITION using wt
// (REQ-053). It rebuilds a canonical-JSON tree from the FLAT entries — node
// types and the elided HISTORY/ITEM_TREE wrappers come from the Web Template
// and rminfo, values from the FLAT suffixes — then decodes it through canjson
// (typereg instantiates the polymorphic RM types).
func UnmarshalFlat(data []byte, wt *webtemplate.WebTemplate) (*rm.Composition, error) {
	if wt == nil || wt.Tree == nil {
		return nil, ErrNoTemplate
	}
	flat, err := unmarshalObject(data)
	if err != nil {
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
	// Process leaf groups in a stable (sorted) order so the reconstructed tree
	// is deterministic: distinct-node-id siblings with no explicit :index
	// (e.g. multiple content items, or elements under one ITEM_TREE) are
	// appended in this order, which must not depend on Go map iteration.
	bases := make([]string, 0, len(groups))
	for base := range groups {
		bases = append(bases, base)
	}
	sort.Strings(bases)
	for _, base := range bases {
		sfx := groups[base]
		pk := parseFlatKey(base)
		leaf, predIndex, predType, ok := resolveLeaf(wt, pk.segs)
		if !ok {
			// A key that does not resolve to a WT node is a wrong template, a
			// typo, or an unsupported feature (ctx/, _-attrs, |raw — Phase 6).
			// Fail loudly rather than drop it silently (REQ-053).
			return nil, fmt.Errorf("%w: %q", ErrUnknownPath, base)
		}
		dv, err := dvFromSuffixes(leaf.RMType, sfx)
		if err != nil {
			return nil, fmt.Errorf("simplified: decode %q: %w", base, err)
		}
		if err := placeLeaf(compJSON, leaf.AQLPath, predIndex, predType, dv); err != nil {
			return nil, fmt.Errorf("simplified: place %q: %w", base, err)
		}
	}
	return compJSON, nil
}

// unmarshalObject decodes a JSON object into a map, preserving integer
// magnitudes exactly (json.Number) rather than routing every number through
// float64 — a DV_COUNT above 2^53 would otherwise be silently rounded before it
// reaches the canonical RM (or the other simplified variant, in interconversion).
func unmarshalObject(data []byte) (map[string]any, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var m map[string]any
	if err := dec.Decode(&m); err != nil {
		return nil, err
	}
	return m, nil
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

// parseAQL splits a canonical aqlPath into attribute+predicate segments. The
// predicate is taken as a bare node id (archetype id or at-code); compound
// predicates (e.g. [at0001 and name/value='x']) are not split — no supported
// Web Template emits them in aqlPath (mirror rmpath.parsePredicate if that
// changes).
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
//
// Reconstructed intermediate and leaf nodes carry _type + archetype_node_id
// only; populating the mandatory LOCATABLE.name from the Web Template node is
// deferred (rmpath re-resolves by archetype_node_id, so round-trip does not
// depend on it — full name population lands with the ctx/name completion).
func placeLeaf(compJSON map[string]any, aqlPath string, predIndex map[string]int, predType map[string]string, dv map[string]any) error {
	segs := parseAQL(aqlPath)
	cur := compJSON
	curType := "COMPOSITION"
	for i, seg := range segs {
		if i == len(segs)-1 {
			cur[seg.attr] = dv
			return nil
		}
		nextAttr := segs[i+1].attr
		childType := concreteType(curType, seg.attr, seg.pred, predType, nextAttr)
		if childType == "" {
			return fmt.Errorf("cannot resolve RM type for %q on %s (aqlPath %q)", seg.attr, curType, aqlPath)
		}
		container, _ := rminfo.Default.IsContainer(curType, seg.attr)
		if container {
			next, err := selectElem(cur, seg.attr, childType, seg.pred, predIndex[seg.pred])
			if err != nil {
				return err
			}
			cur = next
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
	return nil
}

// selectElem finds (or creates) the element with archetype_node_id==pred in
// cur[attr]'s list, at the idx-th position among same-pred siblings (idx is
// the flat :index for a repeatable node; 0 otherwise). Distinct sibling node
// ids get distinct elements even without an explicit index.
func selectElem(cur map[string]any, attr, elemType, pred string, idx int) (map[string]any, error) {
	want := max(idx, 0)
	if want > maxRepeatIndex {
		return nil, fmt.Errorf("%w: :index %d exceeds bound %d", ErrUnknownPath, want, maxRepeatIndex)
	}
	arr, _ := cur[attr].([]any)
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
	// arr[matches[want]] is an element this function appended or matched as a
	// map[string]any above, so the assertion cannot fail.
	return arr[matches[want]].(map[string]any), nil
}

// concreteType resolves the RM type to instantiate for attr on parentType,
// mapping the abstract RM slots to concrete types the way the Web Template /
// canonical form require. nextAttr is the following aqlPath attribute, used to
// disambiguate the abstract ITEM_STRUCTURE slot whose concrete subtype the Web
// Template does not carry (it collapses those nodes).
func concreteType(parentType, attr, pred string, predType map[string]string, nextAttr string) string {
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
		// The Web Template collapses ITEM_STRUCTURE nodes, so their concrete
		// subtype is absent from predType; infer it from the child attribute:
		// `item` -> ITEM_SINGLE, `rows` -> ITEM_TABLE, `items` -> ITEM_TREE /
		// ITEM_LIST. ITEM_TREE and ITEM_LIST both use `items` and are not
		// distinguishable from the path alone; default to ITEM_TREE, which is
		// round-trip-preserving (rmpath re-resolves by attribute + node id).
		// See deviations.md.
		switch nextAttr {
		case "item":
			return "ITEM_SINGLE"
		case "rows":
			return "ITEM_TABLE"
		default:
			return "ITEM_TREE"
		}
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
// suffix->value map (the inverse of leafToFlat). Bare values live under the ""
// suffix (DV_COUNT -> magnitude, DV_BOOLEAN -> value, per the STABLE RM
// mappings). A required suffix that is absent is an error rather than a coerced
// zero value; an unmapped datatype is ErrUnsupportedDatatype.
func dvFromSuffixes(rmType string, sfx map[string]any) (map[string]any, error) {
	switch rmType {
	case "DV_TEXT":
		v, err := requireSuffix(rmType, sfx, "")
		if err != nil {
			return nil, err
		}
		return map[string]any{"_type": "DV_TEXT", "value": v}, nil
	case "DV_DATE_TIME":
		v, err := requireSuffix(rmType, sfx, "")
		if err != nil {
			return nil, err
		}
		return map[string]any{"_type": "DV_DATE_TIME", "value": v}, nil
	case "DV_DATE":
		v, err := requireSuffix(rmType, sfx, "")
		if err != nil {
			return nil, err
		}
		return map[string]any{"_type": "DV_DATE", "value": v}, nil
	case "DV_TIME":
		v, err := requireSuffix(rmType, sfx, "")
		if err != nil {
			return nil, err
		}
		return map[string]any{"_type": "DV_TIME", "value": v}, nil
	case "DV_QUANTITY":
		mag, err := requireSuffix(rmType, sfx, "magnitude")
		if err != nil {
			return nil, err
		}
		unit, err := requireSuffix(rmType, sfx, "unit")
		if err != nil {
			return nil, err
		}
		return map[string]any{"_type": "DV_QUANTITY", "magnitude": mag, "units": unit}, nil
	case "DV_COUNT":
		v, err := requireSuffix(rmType, sfx, "")
		if err != nil {
			return nil, err
		}
		return map[string]any{"_type": "DV_COUNT", "magnitude": v}, nil
	case "DV_BOOLEAN":
		v, err := requireSuffix(rmType, sfx, "")
		if err != nil {
			return nil, err
		}
		return map[string]any{"_type": "DV_BOOLEAN", "value": v}, nil
	case "DV_CODED_TEXT":
		code, err := requireSuffix(rmType, sfx, "code")
		if err != nil {
			return nil, err
		}
		val, err := requireSuffix(rmType, sfx, "value")
		if err != nil {
			return nil, err
		}
		dc := map[string]any{"_type": "CODE_PHRASE", "code_string": code}
		if t, ok := sfx["terminology"]; ok {
			dc["terminology_id"] = map[string]any{"_type": "TERMINOLOGY_ID", "value": t}
		}
		return map[string]any{"_type": "DV_CODED_TEXT", "value": val, "defining_code": dc}, nil
	}
	return nil, fmt.Errorf("%w: %s", ErrUnsupportedDatatype, rmType)
}

// requireSuffix returns sfx[name], or an error if it is absent — a missing
// required suffix must not become a coerced zero value in the canonical RM.
func requireSuffix(rmType string, sfx map[string]any, name string) (any, error) {
	v, ok := sfx[name]
	if !ok {
		label := "|" + name
		if name == "" {
			label = "bare value"
		}
		return nil, fmt.Errorf("%s: missing required %s", rmType, label)
	}
	return v, nil
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
