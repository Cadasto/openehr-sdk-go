package simplified

// REQ-053 — FLAT decode: rebuild a canonical COMPOSITION from a FLAT map.
// The FLAT key grammar (inverse of flat_encode) is parsed here; the canonical
// RM reconstruction (walking each leaf's Web Template aqlPath, materialising
// the elided HISTORY / ITEM_TREE wrappers via rminfo, then decoding through
// canjson) builds on this parser.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/rminfo"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/template/webtemplate"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
)

// maxRepeatIndex bounds a single FLAT :index during decode/interconversion, and
// maxTotalNodes bounds the cumulative slots allocated across one decode. A FLAT
// key such as "node:1000000000" would grow a slice to that length; a handful of
// deeply-indexed keys ("a:9999/b:9999/…") would amplify further. Clinical
// repeats are small, so these caps turn a hostile or corrupt payload into an
// error instead of an allocation blow-up.
const (
	maxRepeatIndex = 10_000
	maxTotalNodes  = 1_000_000
)

// allocBudget caps the total array slots materialised across one decode or
// interconversion, bounding allocation amplification from indexed keys.
type allocBudget struct {
	n, limit int
}

func (b *allocBudget) add(k int) error {
	b.n += k
	if b.n > b.limit {
		return fmt.Errorf("%w: decoded-node budget %d exceeded", ErrUnknownPath, b.limit)
	}
	return nil
}

// UnmarshalFlat decodes FLAT JSON into a canonical COMPOSITION using wt
// (REQ-053). It rebuilds a canonical-JSON tree from the FLAT entries — node
// types and the elided HISTORY/ITEM_TREE wrappers come from the Web Template
// and rminfo, values from the FLAT suffixes — then decodes it through canjson
// (typereg instantiates the polymorphic RM types).
func UnmarshalFlat(data []byte, wt *webtemplate.WebTemplate, opts ...Option) (*rm.Composition, error) {
	if wt == nil || wt.Tree == nil {
		return nil, ErrNoTemplate
	}
	flat, err := unmarshalObject(data)
	if err != nil {
		return nil, err
	}
	cfg := newDecodeConfig(opts)
	var names map[string]string
	if cfg.template != nil {
		names = buildNameIndex(cfg.template)
	}
	compJSON, err := decodeFlat(flat, wt, names)
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
// FLAT map, driven by the Web Template. names (optional; nil unless a template
// was supplied) maps a node's compiled aqlPath to its LOCATABLE.name.
func decodeFlat(flat map[string]any, wt *webtemplate.WebTemplate, names map[string]string) (map[string]any, error) {
	root := wt.Tree
	compJSON := map[string]any{
		"_type":             "COMPOSITION",
		"archetype_node_id": root.NodeID,
		"name":              textJSON(orDefault(root.Name, wt.TemplateID)),
	}
	// Separate composition-level context (ctx/) from clinical content; context
	// is rebuilt from RM attributes, not from a Web Template leaf path.
	ctx := make(map[string]any)
	// Group FLAT keys by leaf instance (key minus the |suffix); each group's
	// suffix->value pairs build one DataValue.
	groups := make(map[string]map[string]any)
	for key, val := range flat {
		if strings.HasPrefix(key, "ctx/") {
			ctx[key] = val
			continue
		}
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
	budget := &allocBudget{limit: maxTotalNodes}
	for _, base := range bases {
		sfx := groups[base]
		pk, err := parseFlatKey(base)
		if err != nil {
			return nil, err
		}
		leaf, predIndex, predType, ok := resolveLeaf(wt, pk.segs)
		if !ok {
			// A key that does not resolve to a WT node is a wrong template, a
			// typo, or an unsupported feature (ctx/, _-attrs, |raw — Phase 6).
			// Fail loudly rather than drop it silently (REQ-053).
			return nil, fmt.Errorf("%w: %q", ErrUnknownPath, base)
		}
		dv, err := dvFromSuffixes(leaf.RMType, leafListOpen(leaf), sfx)
		if err != nil {
			return nil, fmt.Errorf("simplified: decode %q: %w", base, err)
		}
		if err := placeLeaf(compJSON, leaf.AQLPath, predIndex, predType, dv, budget, names); err != nil {
			return nil, fmt.Errorf("simplified: place %q: %w", base, err)
		}
	}
	// A sparse :index (":0" and ":2" with no ":1") would have gap-filled an
	// empty phantom instance in selectElem; reject it before context/completion
	// can decorate fabricated data into something OPT-valid.
	if err := checkNoPhantoms(compJSON); err != nil {
		return nil, err
	}
	// Context: parse once, then apply (with the mandatory-field check) after
	// content, so an unresolvable content key surfaces as ErrUnknownPath first.
	ci, err := parseCtx(ctx)
	if err != nil {
		return nil, err
	}
	if err := applyContext(compJSON, ci); err != nil {
		return nil, err
	}
	// Conformant mode (a template was supplied, so names is non-nil): fill the
	// RM-mandatory attributes the formats do not carry, from ctx defaults + RM
	// conventions, so the decoded composition is OPT-validatable.
	if names != nil {
		completeRequired(compJSON, ci)
	}
	return compJSON, nil
}

// ctxInfo is the parsed ctx/ context — the shared source for applyContext and
// the RM-mandatory completion pass.
type ctxInfo struct {
	language, territory string
	composerName        string
	composerSelf        bool
	haveComposerName    bool
	time                any
	haveTime            bool
}

// parseCtx decodes the ctx/ entries. Only the core context fields are supported;
// any other ctx/ field is ErrUnknownPath (see deviations.md).
func parseCtx(ctx map[string]any) (ctxInfo, error) {
	var ci ctxInfo
	for key, val := range ctx {
		switch strings.TrimPrefix(key, "ctx/") {
		case "language":
			ci.language, _ = val.(string)
		case "territory":
			ci.territory, _ = val.(string)
		case "composer_name":
			ci.composerName, _ = val.(string)
			ci.haveComposerName = true
		case "composer_self":
			b, _ := val.(bool)
			ci.composerSelf = b
		case "time":
			ci.time = val
			ci.haveTime = true
		default:
			return ci, fmt.Errorf("%w: %q (context field not supported — see deviations.md)", ErrUnknownPath, key)
		}
	}
	return ci, nil
}

// applyContext sets the composition-level metadata from the parsed context and
// enforces that language and territory (mandatory per the Simplified Formats
// spec) are present.
func applyContext(compJSON map[string]any, ci ctxInfo) error {
	if ci.language == "" || ci.territory == "" {
		return fmt.Errorf("%w: ctx/language and ctx/territory are required", ErrMissingContext)
	}
	compJSON["language"] = codePhraseJSON(ci.language, "ISO_639-1")
	compJSON["territory"] = codePhraseJSON(ci.territory, "ISO_3166-1")
	switch {
	case ci.composerSelf:
		compJSON["composer"] = map[string]any{"_type": "PARTY_SELF"}
	case ci.haveComposerName:
		compJSON["composer"] = map[string]any{"_type": "PARTY_IDENTIFIED", "name": ci.composerName}
	}
	if ci.haveTime {
		// Merge into any EVENT_CONTEXT already reconstructed from clinical paths
		// (setting, other_context, health_care_facility, …) rather than replacing
		// it — otherwise that data would be lost.
		ctxObj, _ := compJSON["context"].(map[string]any)
		if ctxObj == nil {
			ctxObj = map[string]any{"_type": "EVENT_CONTEXT"}
			compJSON["context"] = ctxObj
		}
		ctxObj["start_time"] = map[string]any{"_type": "DV_DATE_TIME", "value": ci.time}
	}
	return nil
}

// completeRequired fills the RM-mandatory attributes the FLAT/STRUCTURED formats
// do not carry — ENTRY language/encoding/subject, HISTORY.origin, EVENT.time,
// EVENT_CONTEXT.setting, and any others rminfo reports — with ctx defaults + RM
// conventions, so a WithTemplate decode yields an OPT-validatable composition.
// It only runs in conformant (WithTemplate) mode. The values it synthesises
// (event times, subject, setting) are defaults, not recovered data — the formats
// never carried them; see deviations.md.
func completeRequired(node map[string]any, ci ctxInfo) {
	if t, _ := node["_type"].(string); t != "" {
		for _, attr := range rminfo.Default.RequiredAttributes(t) {
			if _, has := node[attr]; has {
				continue
			}
			if dv := defaultAttr(attr, ci); dv != nil {
				node[attr] = dv
			}
		}
	}
	for _, v := range node {
		switch val := v.(type) {
		case map[string]any:
			completeRequired(val, ci)
		case []any:
			for _, e := range val {
				if m, ok := e.(map[string]any); ok {
					completeRequired(m, ci)
				}
			}
		}
	}
}

// defaultAttr synthesises a default value for an RM-mandatory attribute the
// formats omit. LOCATABLE.name and archetype_node_id are handled elsewhere
// (WithTemplate names / the aqlPath predicate); container attributes (data,
// items, item) are reconstructed from content. Returns nil for attributes with
// no sensible default.
func defaultAttr(attr string, ci ctxInfo) map[string]any {
	switch attr {
	case "language":
		return codePhraseJSON(orDefault(ci.language, "en"), "ISO_639-1")
	case "encoding":
		return codePhraseJSON("UTF-8", "IANA_character-sets")
	case "subject", "composer":
		return map[string]any{"_type": "PARTY_SELF"}
	case "origin", "time":
		if ci.time == nil {
			return nil
		}
		return map[string]any{"_type": "DV_DATE_TIME", "value": ci.time}
	case "setting":
		return map[string]any{"_type": "DV_CODED_TEXT", "value": "other care", "defining_code": codePhraseJSON("238", "openehr")}
	case "category":
		return map[string]any{"_type": "DV_CODED_TEXT", "value": "event", "defining_code": codePhraseJSON("433", "openehr")}
	case "math_function":
		return map[string]any{"_type": "DV_CODED_TEXT", "value": "actual", "defining_code": codePhraseJSON("146", "openehr")}
	case "width":
		return map[string]any{"_type": "DV_DURATION", "value": "PT0S"}
	}
	return nil
}

// codePhraseJSON is a canonical CODE_PHRASE object.
func codePhraseJSON(code, terminology string) map[string]any {
	return map[string]any{
		"_type":          "CODE_PHRASE",
		"code_string":    code,
		"terminology_id": map[string]any{"_type": "TERMINOLOGY_ID", "value": terminology},
	}
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
	// Reject trailing content after the first JSON value — a second document (or
	// garbage) must not be silently ignored.
	if dec.More() {
		return nil, errors.New("simplified: unexpected trailing content after JSON object")
	}
	return m, nil
}

// resolveLeaf walks the Web Template by FLAT segment ids to the leaf node,
// collecting, for each ancestor that carries an archetype node id, its flat
// :index (predIndex) and Web Template rmType (predType) — both keyed by the
// ancestor's canonical AQLPath, which is unique per chain position. Keying by
// bare node id would collide when the same at-code (or a self-nested archetype
// id) appears twice along one path, silently applying one segment's index or
// type to another. Returns ok=false when a segment id is not found.
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
			predType[next.AQLPath] = next.RMType
			if seg.idx >= 0 {
				predIndex[next.AQLPath] = seg.idx
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
// Reconstructed intermediate and leaf nodes carry _type + archetype_node_id;
// when names is non-nil (a template was supplied via WithTemplate) the mandatory
// LOCATABLE.name is set from it, keyed by the node's compiled aqlPath — which
// the walk reconstructs by keeping a predicate only on container attributes
// (matching templatecompile's path convention). Without names, nodes are left
// unnamed (rmpath re-resolves by archetype_node_id, so the round-trip does not
// depend on it — but the result is then format-idempotent, not canonically
// complete; see deviations.md).
func placeLeaf(compJSON map[string]any, aqlPath string, predIndex map[string]int, predType map[string]string, dv map[string]any, budget *allocBudget, names map[string]string) error {
	segs := parseAQL(aqlPath)
	cur := compJSON
	curType := "COMPOSITION"
	// Two path keys are rebuilt in lockstep: aqlPrefix reproduces the WT
	// aqlPath prefix exactly (predicate on every predicated segment) — the
	// positional key predIndex/predType are stored under; namePrefix keeps a
	// predicate only on container attributes (templatecompile's convention),
	// the key of the WithTemplate name index.
	var aqlPrefix, namePrefix strings.Builder
	for i, seg := range segs {
		if i == len(segs)-1 {
			if _, exists := cur[seg.attr]; exists {
				// Two FLAT keys resolved to the same terminal slot (e.g. "a" vs
				// "a:0" on a repeatable) — overwriting would silently drop one.
				return fmt.Errorf("%w: duplicate placement at %q", ErrUnknownPath, aqlPath)
			}
			cur[seg.attr] = dv
			return nil
		}
		aqlPrefix.WriteString("/")
		aqlPrefix.WriteString(seg.attr)
		if seg.pred != "" {
			aqlPrefix.WriteString("[")
			aqlPrefix.WriteString(seg.pred)
			aqlPrefix.WriteString("]")
		}
		wtType := predType[aqlPrefix.String()]
		nextAttr := segs[i+1].attr
		childType := concreteType(curType, seg.attr, wtType, nextAttr)
		if childType == "" {
			return fmt.Errorf("cannot resolve RM type for %q on %s (aqlPath %q)", seg.attr, curType, aqlPath)
		}
		container, _ := rminfo.Default.IsContainer(curType, seg.attr)
		namePrefix.WriteString("/")
		namePrefix.WriteString(seg.attr)
		if container && seg.pred != "" {
			namePrefix.WriteString("[")
			namePrefix.WriteString(seg.pred)
			namePrefix.WriteString("]")
		}
		if container {
			next, err := selectElem(cur, seg.attr, childType, seg.pred, predIndex[aqlPrefix.String()], budget)
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
		if nm := names[namePrefix.String()]; nm != "" {
			if _, has := cur["name"]; !has {
				cur["name"] = textJSON(nm)
			}
		}
		curType = childType
	}
	return nil
}

// buildNameIndex walks the compiled template and maps each archetype node's
// canonical aqlPath to its LOCATABLE name (the node-id term rubric, default
// language). Used by decode to repopulate LOCATABLE.name (see WithTemplate).
func buildNameIndex(c *templatecompile.Compiled) map[string]string {
	names := make(map[string]string)
	lang := c.Language()
	var walk func(n *templatecompile.CompiledNode)
	walk = func(n *templatecompile.CompiledNode) {
		if id := n.NodeID(); id != "" {
			if t, ok := n.Term(id, lang); ok {
				if txt := t.Items["text"]; txt != "" {
					names[n.AQLPath()] = txt
				}
			}
		}
		for _, a := range n.Attributes() {
			for _, ch := range a.Children() {
				walk(ch)
			}
		}
	}
	if root := c.Root(); root != nil {
		walk(root)
	}
	return names
}

// checkNoPhantoms walks the rebuilt tree and rejects any container element that
// selectElem gap-filled but no leaf ever reached: an instance carrying nothing
// beyond _type and archetype_node_id. Such phantoms arise only from a sparse
// :index sequence (":0" and ":2" with no ":1"); accepting them would fabricate
// empty — and, after RM-mandatory completion, seemingly valid — clinical
// instances out of a malformed payload.
func checkNoPhantoms(node map[string]any) error {
	for _, v := range node {
		arr, ok := v.([]any)
		if !ok {
			if m, ok := v.(map[string]any); ok {
				if err := checkNoPhantoms(m); err != nil {
					return err
				}
			}
			continue
		}
		for _, e := range arr {
			m, ok := e.(map[string]any)
			if !ok {
				continue
			}
			if phantomKeysOnly(m) {
				return fmt.Errorf("%w: sparse :index left an empty %v instance (missing occurrence in sequence)", ErrUnknownPath, m["_type"])
			}
			if err := checkNoPhantoms(m); err != nil {
				return err
			}
		}
	}
	return nil
}

// phantomKeysOnly reports whether m carries nothing beyond the identity keys a
// gap-filled element is created with.
func phantomKeysOnly(m map[string]any) bool {
	for k := range m {
		if k != "_type" && k != "archetype_node_id" {
			return false
		}
	}
	return true
}

// selectElem finds (or creates) the element with archetype_node_id==pred in
// cur[attr]'s list, at the idx-th position among same-pred siblings (idx is
// the flat :index for a repeatable node; 0 otherwise). Distinct sibling node
// ids get distinct elements even without an explicit index.
func selectElem(cur map[string]any, attr, elemType, pred string, idx int, budget *allocBudget) (map[string]any, error) {
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
	if need := want + 1 - len(matches); need > 0 {
		if err := budget.add(need); err != nil {
			return nil, err
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
// canonical form require. wtType is the Web Template rmType positionally
// resolved for this segment ("" when the segment has no WT node — e.g. the
// collapsed wrappers); nextAttr is the following aqlPath attribute, used to
// disambiguate the abstract ITEM_STRUCTURE slot whose concrete subtype the Web
// Template does not carry (it collapses those nodes).
func concreteType(parentType, attr, wtType, nextAttr string) string {
	t, ok := rminfo.Default.AttributeRMType(parentType, attr)
	if !ok {
		return ""
	}
	switch t {
	case "CONTENT_ITEM":
		if wtType != "" {
			return wtType // OBSERVATION / EVALUATION / …
		}
		return "OBSERVATION"
	case "EVENT":
		if wtType == "INTERVAL_EVENT" {
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
		if wtType == "CLUSTER" {
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
func dvFromSuffixes(rmType string, listOpen bool, sfx map[string]any) (map[string]any, error) {
	// |raw bypass: a pre-serialised canonical fragment (carrying its own string
	// _type); used directly, regardless of the leaf rmType. Mutually exclusive
	// with every other suffix.
	if raw, ok := sfx["raw"]; ok {
		if len(sfx) > 1 {
			return nil, fmt.Errorf("%w: |raw is mutually exclusive with other suffixes", ErrUnsupportedDatatype)
		}
		frag, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%w: |raw value is not a canonical object", ErrUnsupportedDatatype)
		}
		if t, ok := frag["_type"].(string); !ok || t == "" {
			return nil, fmt.Errorf("%w: |raw fragment missing string _type", ErrUnsupportedDatatype)
		}
		return frag, nil
	}
	if err := checkSuffixAllowlist(rmType, sfx); err != nil {
		return nil, err
	}
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
		// |other is the open-value-set free-text fallback: the leaf is persisted
		// as a DV_TEXT, not a DV_CODED_TEXT (spec §Open Value-Sets and |other).
		if other, ok := sfx["other"]; ok {
			if !listOpen {
				return nil, fmt.Errorf("%w: |other requires an open value-set (listOpen)", ErrUnsupportedDatatype)
			}
			if _, hasCode := sfx["code"]; hasCode {
				return nil, fmt.Errorf("%w: |other is mutually exclusive with |code", ErrUnsupportedDatatype)
			}
			return map[string]any{"_type": "DV_TEXT", "value": other}, nil
		}
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
	case "DV_DURATION":
		v, err := requireSuffix(rmType, sfx, "")
		if err != nil {
			return nil, err
		}
		return map[string]any{"_type": "DV_DURATION", "value": v}, nil
	case "DV_URI":
		v, err := requireSuffix(rmType, sfx, "")
		if err != nil {
			return nil, err
		}
		return map[string]any{"_type": "DV_URI", "value": v}, nil
	case "DV_EHR_URI":
		v, err := requireSuffix(rmType, sfx, "")
		if err != nil {
			return nil, err
		}
		return map[string]any{"_type": "DV_EHR_URI", "value": v}, nil
	case "DV_ORDINAL":
		code, err := requireSuffix(rmType, sfx, "code")
		if err != nil {
			return nil, err
		}
		val, err := requireSuffix(rmType, sfx, "value")
		if err != nil {
			return nil, err
		}
		ordinal, err := requireSuffix(rmType, sfx, "ordinal")
		if err != nil {
			return nil, err
		}
		// Ordinal symbols are archetype-local (at-codes) -> "local" terminology.
		symbol := map[string]any{
			"_type": "DV_CODED_TEXT", "value": val,
			"defining_code": map[string]any{
				"_type": "CODE_PHRASE", "code_string": code,
				"terminology_id": map[string]any{"_type": "TERMINOLOGY_ID", "value": "local"},
			},
		}
		return map[string]any{"_type": "DV_ORDINAL", "value": ordinal, "symbol": symbol}, nil
	case "DV_PROPORTION":
		num, err := requireSuffix(rmType, sfx, "numerator")
		if err != nil {
			return nil, err
		}
		den, err := requireSuffix(rmType, sfx, "denominator")
		if err != nil {
			return nil, err
		}
		typ, err := requireSuffix(rmType, sfx, "type")
		if err != nil {
			return nil, err
		}
		return map[string]any{"_type": "DV_PROPORTION", "numerator": num, "denominator": den, "type": typ}, nil
	case "DV_IDENTIFIER":
		id, err := requireSuffix(rmType, sfx, "id")
		if err != nil {
			return nil, err
		}
		out := map[string]any{"_type": "DV_IDENTIFIER", "id": id}
		for _, s := range []string{"issuer", "assigner", "type"} {
			if v, ok := sfx[s]; ok {
				out[s] = v
			}
		}
		return out, nil
	}
	return nil, fmt.Errorf("%w: %s", ErrUnsupportedDatatype, rmType)
}

// allowedSuffixes lists, per datatype, the pipe suffixes (and "" for a bare
// value) the decoder maps. A key outside this set (a typo like |unitt, or a
// decorated attribute like |accuracy that only rides |raw) is rejected rather
// than silently dropped. |other and |raw are handled before this check.
var allowedSuffixes = map[string]map[string]bool{
	"DV_TEXT":       {"": true},
	"DV_CODED_TEXT": {"code": true, "value": true, "terminology": true},
	"DV_DATE_TIME":  {"": true},
	"DV_DATE":       {"": true},
	"DV_TIME":       {"": true},
	"DV_DURATION":   {"": true},
	"DV_URI":        {"": true},
	"DV_EHR_URI":    {"": true},
	"DV_QUANTITY":   {"magnitude": true, "unit": true},
	"DV_COUNT":      {"": true},
	"DV_BOOLEAN":    {"": true},
	"DV_ORDINAL":    {"code": true, "value": true, "ordinal": true},
	"DV_PROPORTION": {"numerator": true, "denominator": true, "type": true},
	"DV_IDENTIFIER": {"id": true, "issuer": true, "assigner": true, "type": true},
}

// checkSuffixAllowlist rejects any suffix a datatype does not map. An unmapped
// rmType is left to the switch (ErrUnsupportedDatatype). |other is allowed only
// for DV_CODED_TEXT (the case then enforces listOpen).
func checkSuffixAllowlist(rmType string, sfx map[string]any) error {
	allowed, known := allowedSuffixes[rmType]
	if !known {
		return nil
	}
	for k := range sfx {
		if k == "other" && rmType == "DV_CODED_TEXT" {
			continue
		}
		if allowed[k] {
			continue
		}
		label := "|" + k
		if k == "" {
			label = "bare value"
		}
		return fmt.Errorf("%w: unexpected %s for %s", ErrUnsupportedDatatype, label, rmType)
	}
	return nil
}

// leafListOpen reports whether a Web Template leaf constrains an open value-set
// (any input with listOpen) — the precondition for the |other free-text form.
func leafListOpen(node *webtemplate.Node) bool {
	for _, in := range node.Inputs {
		if in.ListOpen {
			return true
		}
	}
	return false
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
// "|<attr>" is the leaf attribute suffix. A numeric index must be spelled
// canonically ("0", "1", …): a negative index would collide with the internal
// "no index" sentinel, and non-canonical spellings ("-1", "+0", "00") would
// make distinct JSON keys resolve to the same slot, silently overwriting one
// value with another — both are rejected.
func parseFlatKey(key string) (parsedKey, error) {
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
				if n < 0 || p[j+1:] != strconv.Itoa(n) {
					return parsedKey{}, fmt.Errorf("%w: invalid :index %q in %q", ErrUnknownPath, p[j+1:], key)
				}
				seg.id = p[:j]
				seg.idx = n
			}
		}
		segs = append(segs, seg)
	}
	return parsedKey{segs: segs, suffix: suffix}, nil
}
