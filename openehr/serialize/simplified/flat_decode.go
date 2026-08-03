package simplified

// REQ-053 — FLAT decode: rebuild a canonical COMPOSITION from a FLAT map.
// The FLAT key grammar (inverse of flat_encode) is parsed here; the canonical
// RM reconstruction (walking each leaf's Web Template aqlPath, materialising
// the elided HISTORY / ITEM_TREE wrappers via rminfo, then decoding through
// canjson) builds on this parser.

import (
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
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
		"name":              textJSON(cmp.Or(root.Name, wt.TemplateID)),
	}
	// Separate composition-level context from clinical content; context is
	// rebuilt from RM attributes, not from a Web Template leaf path.
	ctx, content, err := siphonContext(flat, root.ID)
	if err != nil {
		return nil, err
	}
	// Group FLAT keys by leaf instance (key minus the |suffix); each group's
	// suffix->value pairs build one DataValue.
	groups := make(map[string]map[string]any)
	for key, val := range content {
		base, suffix := splitSuffix(key)
		if groups[base] == nil {
			groups[base] = make(map[string]any)
		}
		groups[base][suffix] = val
	}
	// Process leaf groups in a stable (sorted) order so the reconstructed tree
	// is deterministic: distinct-node-id siblings with no explicit :index
	// (e.g. multiple content items, or elements under one ITEM_TREE) are
	// appended in this order, which must not depend on Go map iteration.
	budget := &allocBudget{limit: maxTotalNodes}
	ambiguous := ambiguousBarePaths(wt)
	for _, base := range slices.Sorted(maps.Keys(groups)) {
		sfx := groups[base]
		pk, err := parseFlatKey(base)
		if err != nil {
			return nil, err
		}
		leaf, predIndex, predType, err := resolveLeaf(wt, pk.segs, ambiguous)
		if errors.Is(err, errSegNotFound) {
			// A key that does not resolve to a WT node is a wrong template, a
			// typo, or an unsupported _-prefixed RM attribute (see deviations.md
			// — ctx/ is siphoned off above and |raw is a suffix, so neither
			// reaches this branch). Fail loudly, never drop silently (REQ-053).
			return nil, fmt.Errorf("%w: %q", ErrUnknownPath, base)
		}
		if err != nil {
			return nil, fmt.Errorf("simplified: %q: %w", base, err)
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

// metadataAliases maps the reference implementation's real-path spelling of
// composition-level metadata — relative to the template root — onto the ctx/
// short form REQ-053 canonically emits.
//
// The codec is deliberately asymmetric here: both spellings are accepted on
// input, only ctx/ is written on output (ADR 0015). Upstream FLAT carries
// `<root>/language|code` where this SDK reads and writes `ctx/language`; before
// this table such a body failed with ErrUnknownPath, which made an
// EHRbase-authored composition undecodable over a pure respelling.
//
// Entries here are respellings *only* — same information, different surface. Two
// composition-level families deliberately stay out, because admitting them would
// be adding a field rather than accepting a spelling:
//
//   - `context/setting|*` — ctx/setting is unsupported on decode *too*, so this
//     is an unimplemented field on both surfaces, not a spelling gap.
//   - `composer|id` / `|id_scheme` / `|id_namespace` — the composer's
//     external_ref, which the ctx/ short forms structurally cannot carry. Silently
//     dropping it would violate REQ-053's semantics-preserving contract.
//
// Both remain refused, and so remain visible in the PROBE-086 census.
var metadataAliases = map[string]string{
	"language|code":      "ctx/language",
	"territory|code":     "ctx/territory",
	"composer|name":      "ctx/composer_name",
	"composer_self":      "ctx/composer_self",
	"context/start_time": "ctx/time",
}

// metadataAliasTerminology pins the terminology each respelled CODE_PHRASE
// carries implicitly in the ctx/ form, where only the code travels. The suffix
// is a witness, not data: the value is checked and discarded. Accepting a
// mismatch would silently rewrite the terminology, since applyContext hardcodes
// these when rebuilding the CODE_PHRASE.
var metadataAliasTerminology = map[string]string{
	"language|terminology":  "ISO_639-1",
	"territory|terminology": "ISO_3166-1",
}

// siphonContext splits a FLAT map into composition-level context and clinical
// content, normalising both accepted metadata spellings into one ctx/-keyed map.
//
// A real path that contradicts an explicit ctx/ entry is an error rather than a
// precedence rule: preferring either silently would corrupt composition
// metadata, the same stance the codec already takes on an index collision.
func siphonContext(flat map[string]any, rootID string) (ctx, content map[string]any, err error) {
	ctx, content = make(map[string]any), make(map[string]any)
	// Sorted so a body carrying two conflicting real-path spellings reports the
	// same key first on every run — a map-order-dependent error message is not
	// reproducible for whoever has to fix the payload.
	for _, key := range slices.Sorted(maps.Keys(flat)) {
		val := flat[key]
		switch {
		case strings.HasPrefix(key, "ctx/"):
			if err := putCtx(ctx, key, val, key); err != nil {
				return nil, nil, err
			}
			continue
		}
		rel, isRooted := strings.CutPrefix(key, rootID+"/")
		if !isRooted {
			content[key] = val
			continue
		}
		if want, isWitness := metadataAliasTerminology[rel]; isWitness {
			got, err := ctxString(key, val)
			if err != nil {
				return nil, nil, err
			}
			if got != want {
				return nil, nil, fmt.Errorf("%w: %s = %q, but the ctx/ form implies %q (a differing terminology cannot be carried)",
					ErrUnsupportedDatatype, key, got, want)
			}
			continue
		}
		if ctxKey, isAlias := metadataAliases[rel]; isAlias {
			if err := putCtx(ctx, ctxKey, val, key); err != nil {
				return nil, nil, err
			}
			continue
		}
		content[key] = val
	}
	return ctx, content, nil
}

// putCtx records a context value, rejecting a second spelling that disagrees.
// origin names the key as the caller wrote it, so the error points at the
// payload rather than at the normalised ctx/ form they may never have used.
func putCtx(ctx map[string]any, ctxKey string, val any, origin string) error {
	if prev, seen := ctx[ctxKey]; seen && prev != val {
		return fmt.Errorf("%w: conflicting spellings of %s — %s gives %#v, another key already gave %#v; remove one",
			ErrUnknownPath, ctxKey, origin, val, prev)
	}
	ctx[ctxKey] = val
	return nil
}

// ctxInfo is the parsed ctx/ context — the shared source for applyContext and
// the RM-mandatory completion pass.
type ctxInfo struct {
	language, territory string
	composerName        string
	composerSelf        bool
	haveComposerName    bool
	time                string
	haveTime            bool
}

// parseCtx decodes the ctx/ entries. Only the core context fields are supported;
// any other ctx/ field is ErrUnknownPath (see deviations.md). Values of the
// wrong JSON type are rejected — coercing them would silently corrupt
// composition metadata (e.g. a numeric composer_name becoming an empty
// PARTY_IDENTIFIED name).
func parseCtx(ctx map[string]any) (ctxInfo, error) {
	var ci ctxInfo
	var err error
	for key, val := range ctx {
		switch strings.TrimPrefix(key, "ctx/") {
		case "language":
			ci.language, err = ctxString(key, val)
		case "territory":
			ci.territory, err = ctxString(key, val)
		case "composer_name":
			ci.composerName, err = ctxString(key, val)
			ci.haveComposerName = true
		case "composer_self":
			b, ok := val.(bool)
			if !ok {
				err = fmt.Errorf("%w: %s must be a boolean, got %T", ErrUnsupportedDatatype, key, val)
			}
			ci.composerSelf = b
		case "time":
			ci.time, err = ctxString(key, val)
			ci.haveTime = true
		default:
			err = fmt.Errorf("%w: %q (context field not supported — see deviations.md)", ErrUnknownPath, key)
		}
		if err != nil {
			return ci, err
		}
	}
	return ci, nil
}

// ctxString asserts a ctx/ value is a JSON string, failing loudly otherwise.
func ctxString(key string, val any) (string, error) {
	s, ok := val.(string)
	if !ok {
		return "", fmt.Errorf("%w: %s must be a string, got %T", ErrUnsupportedDatatype, key, val)
	}
	return s, nil
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
		return codePhraseJSON(cmp.Or(ci.language, "en"), "ISO_639-1")
	case "encoding":
		return codePhraseJSON("UTF-8", "IANA_character-sets")
	case "subject", "composer":
		return map[string]any{"_type": "PARTY_SELF"}
	case "origin", "time":
		if !ci.haveTime {
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
// **bare** spelling of the ancestor's AQLPath (REQ-116 name predicates
// stripped), because placeLeaf rebuilds its lookup prefix from parseAQL
// segments, which are bare. Keying by the predicated spelling made every
// lookup under a name-pinned container miss, so concreteType fell back to a
// default RM type — wrong _type embedded silently where it resolved, decode
// failure where it did not. Keying by bare node id alone would be worse
// still: the same at-code (or a self-nested archetype id) appearing twice
// along one path would silently apply one segment's index or type to
// another; the bare path prefix stays unique per chain position. Returns
// ok=false when a segment id is not found.
func resolveLeaf(wt *webtemplate.WebTemplate, segs []flatSeg, ambiguous map[string]bool) (*webtemplate.Node, map[string]int, map[string]string, error) {
	predIndex := make(map[string]int)
	predType := make(map[string]string)
	node := wt.Tree
	if len(segs) == 0 || segs[0].id != node.ID {
		return nil, nil, nil, errSegNotFound
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
			return nil, nil, nil, errSegNotFound
		}
		if next.NodeID != "" {
			bare := bareAQLPath(next.AQLPath)
			if ambiguous[bare] {
				// Reused siblings (REQ-116) share this bare spelling, and
				// placement below keys on it: distinct siblings' instances
				// would collapse onto one list slot — a silent merge of one
				// sibling's data into another. Refuse rather than corrupt;
				// see deviations.md § Conformance (reused-sibling residual).
				return nil, nil, nil, fmt.Errorf("%w: FLAT id %q reaches one of several reused siblings sharing the path %q — not yet decodable (see deviations.md)", ErrUnknownPath, seg.id, bare)
			}
			predType[bare] = next.RMType
			if seg.idx >= 0 {
				predIndex[bare] = seg.idx
			}
		}
		node = next
	}
	return node, predIndex, predType, nil
}

// errSegNotFound reports a FLAT segment id with no Web Template child — the
// caller wraps it with the offending key (a wrong template, a typo, or an
// unsupported _-prefixed RM attribute).
var errSegNotFound = errors.New("segment not found")

// ambiguousBarePaths returns the bare spellings claimed by more than one Web
// Template node — the reused-sibling class REQ-116 made buildable (siblings
// separated only by a pinned name predicate, or by an ordinal-deduped id).
// Placement keys on the bare spelling, so these paths cannot be decoded
// unambiguously yet; resolveLeaf refuses them loudly.
func ambiguousBarePaths(wt *webtemplate.WebTemplate) map[string]bool {
	counts := make(map[string]int)
	var walk func(n *webtemplate.Node)
	walk = func(n *webtemplate.Node) {
		if n.NodeID != "" {
			counts[bareAQLPath(n.AQLPath)]++
		}
		for _, ch := range n.Children {
			walk(ch)
		}
	}
	if wt.Tree != nil {
		walk(wt.Tree)
	}
	ambiguous := make(map[string]bool)
	for p, c := range counts {
		if c > 1 {
			ambiguous[p] = true
		}
	}
	return ambiguous
}

// aqlSeg is one canonical-path segment: an attribute name and an optional
// node predicate (archetype id or at-code).
type aqlSeg struct {
	attr string
	pred string
}

// parseAQL splits a canonical aqlPath into attribute+predicate segments. The
// predicate is taken as a bare node id (archetype id or at-code).
//
// REQ-116 name predicates are stripped first ([at0001,'Reaction details'] ->
// [at0001]), which is both a correctness and a parsing requirement:
//
//   - archetype_node_id must carry the bare id — a name predicate constrains
//     the node's name, it does not rename the node — and the WithTemplate
//     name index is keyed on the bare spelling to match.
//   - a pinned name may itself contain '/' or ',' — the corona oracle pins
//     one of each — which the segment split below would otherwise tear
//     apart.
//
// Compound predicates (e.g. [at0001 and name/value='x']) are still not
// split — no supported Web Template emits them (mirror
// rmpath.parsePredicate if that changes).
func parseAQL(p string) []aqlSeg {
	var out []aqlSeg
	for part := range strings.SplitSeq(strings.TrimPrefix(bareAQLPath(p), "/"), "/") {
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
	// Two path keys are rebuilt in lockstep, both in the BARE spelling
	// (parseAQL stripped any REQ-116 name predicate): aqlPrefix carries a
	// predicate on every predicated segment — the positional key
	// predIndex/predType are stored under, bare-keyed by resolveLeaf to
	// match; namePrefix keeps a predicate only on container attributes
	// (templatecompile's convention), the key of the WithTemplate name
	// index, which aliases every entry under its bare spelling.
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
			return fmt.Errorf("%w: cannot resolve RM type for %q on %s (aqlPath %q)", ErrUnknownPath, seg.attr, curType, aqlPath)
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

// bareAQLPath strips REQ-116 name predicates from a compiled AQL path,
// turning `/content[…SECTION.adhoc.v1,'Symptome']` back into
// `/content[…SECTION.adhoc.v1]`. Embedded `\'` escapes are honoured.
//
// Decode builds its lookup key from the *incoming FLAT* segments, which
// key on archetype id / at-code alone, so the index has to answer to the
// bare spelling as well as the compiled one.
func bareAQLPath(p string) string {
	if !strings.Contains(p, ",'") {
		return p
	}
	var b strings.Builder
	b.Grow(len(p))
	for i := 0; i < len(p); {
		if p[i] == ',' && i+1 < len(p) && p[i+1] == '\'' {
			j := i + 2
			for j < len(p) {
				if p[j] == '\\' && j+1 < len(p) {
					j += 2
					continue
				}
				if p[j] == '\'' {
					j++
					break
				}
				j++
			}
			i = j
			continue
		}
		b.WriteByte(p[i])
		i++
	}
	return b.String()
}

// buildNameIndex walks the compiled template and maps each archetype node's
// aqlPath to its LOCATABLE name — the template-level node name where the OPT
// pins one (REQ-116: that pinned value IS the node's modelled name, and it is
// what the reference materialises), else the node-id term rubric in the
// default language. Used by decode to repopulate LOCATABLE.name (see
// WithTemplate).
//
// Every node is indexed under both its compiled path and that path's bare
// spelling (REQ-116 name predicates removed), because decode composes its
// key from FLAT segments that carry no name. Siblings sharing a bare path
// carry the same at-code and therefore the same rubric — but where they pin
// *distinct* names (archetype reuse under a slot), the bare spelling cannot
// say which sibling is meant, so the alias falls back to the shared rubric
// rather than guess: those instances decode with the pre-REQ-116 name. The
// per-sibling pinned name at a bare key needs the FLAT segment identity
// carried into this lookup — recorded in deviations.md § Conformance as part
// of the reused-sibling residual the PROBE-086 adapter work owns.
func buildNameIndex(c *templatecompile.Compiled) map[string]string {
	names := make(map[string]string)
	lang := c.Language()
	var walk func(n *templatecompile.CompiledNode)
	walk = func(n *templatecompile.CompiledNode) {
		if id := n.NodeID(); id != "" {
			var rubric string
			if t, ok := n.Term(id, lang); ok {
				rubric = t.Items["text"]
			}
			name := n.NodeName()
			if name == "" {
				name = rubric
			}
			if name != "" {
				path := n.AQLPath()
				names[path] = name
				if bare := bareAQLPath(path); bare != path {
					if prev, taken := names[bare]; taken && prev != name {
						// Two named siblings share this bare spelling —
						// ambiguous; keep the shared rubric instead.
						names[bare] = rubric
					} else {
						names[bare] = name
					}
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

// bareLeafAttr maps each bare-leaf datatype to the canonical attribute its ""
// suffix value rebuilds (DV_COUNT -> magnitude, DV_BOOLEAN -> value, … — per
// the STABLE RM mappings).
var bareLeafAttr = map[string]string{
	"DV_TEXT":      "value",
	"DV_DATE_TIME": "value",
	"DV_DATE":      "value",
	"DV_TIME":      "value",
	"DV_DURATION":  "value",
	"DV_URI":       "value",
	"DV_EHR_URI":   "value",
	"DV_COUNT":     "magnitude",
	"DV_BOOLEAN":   "value",
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
	// The bare-leaf datatypes all rebuild as {_type, <attr>: value} from the ""
	// suffix; the table replaces nine identical switch cases.
	if attr, ok := bareLeafAttr[rmType]; ok {
		v, err := requireSuffix(rmType, sfx, "")
		if err != nil {
			return nil, err
		}
		dv := map[string]any{"_type": rmType, attr: v}
		if err := applyOrderedSuffixes(dv, sfx); err != nil {
			return nil, err
		}
		return dv, nil
	}
	switch rmType {
	case "DV_QUANTITY":
		mag, err := requireSuffix(rmType, sfx, "magnitude")
		if err != nil {
			return nil, err
		}
		unit, err := requireSuffix(rmType, sfx, "unit")
		if err != nil {
			return nil, err
		}
		dv := map[string]any{"_type": "DV_QUANTITY", "magnitude": mag, "units": unit}
		if err := applyOrderedSuffixes(dv, sfx); err != nil {
			return nil, err
		}
		return dv, nil
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
		dv := map[string]any{"_type": "DV_CODED_TEXT", "value": val, "defining_code": dc}
		if err := applyOrderedSuffixes(dv, sfx); err != nil {
			return nil, err
		}
		return dv, nil
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
		dv := map[string]any{"_type": "DV_PROPORTION", "numerator": num, "denominator": den, "type": typ}
		if err := applyOrderedSuffixes(dv, sfx); err != nil {
			return nil, err
		}
		return dv, nil
	case "CODE_PHRASE":
		// A leaf CODE_PHRASE (ENTRY language / encoding), not the defining_code
		// nested inside DV_CODED_TEXT. |code is required — a CODE_PHRASE without
		// one is not a code; |terminology is optional, matching the encoder, which
		// omits an empty TERMINOLOGY_ID rather than writing a blank suffix.
		code, err := requireSuffix(rmType, sfx, "code")
		if err != nil {
			return nil, err
		}
		cp := map[string]any{"_type": "CODE_PHRASE", "code_string": code}
		if t, ok := sfx["terminology"]; ok {
			cp["terminology_id"] = map[string]any{"_type": "TERMINOLOGY_ID", "value": t}
		}
		return cp, nil
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
	"DV_TEXT":       {"": true, "formatting": true},
	"DV_CODED_TEXT": {"code": true, "value": true, "terminology": true, "formatting": true},
	"DV_DATE_TIME":  {"": true, "magnitude_status": true, "normal_status": true},
	"DV_DATE":       {"": true, "magnitude_status": true, "normal_status": true},
	"DV_TIME":       {"": true, "magnitude_status": true, "normal_status": true},
	"DV_DURATION":   {"": true, "magnitude_status": true, "normal_status": true, "accuracy": true, "accuracy_is_percent": true},
	"DV_URI":        {"": true},
	"DV_EHR_URI":    {"": true},
	"DV_QUANTITY": {
		"magnitude": true, "unit": true,
		"magnitude_status": true, "normal_status": true,
		"accuracy": true, "accuracy_is_percent": true,
		"precision": true, "units_system": true, "units_display_name": true,
	},
	"DV_COUNT":   {"": true, "magnitude_status": true, "normal_status": true, "accuracy": true, "accuracy_is_percent": true},
	"DV_BOOLEAN": {"": true},
	"DV_ORDINAL": {"code": true, "value": true, "ordinal": true},
	"DV_PROPORTION": {
		"numerator": true, "denominator": true, "type": true,
		"magnitude_status": true, "normal_status": true,
		"accuracy": true, "accuracy_is_percent": true, "precision": true,
	},
	"DV_IDENTIFIER": {"id": true, "issuer": true, "assigner": true, "type": true},
	"CODE_PHRASE":   {"code": true, "terminology": true},
}

// orderedSuffixAttr maps the optional DV_ORDERED / DV_QUANTIFIED / DV_AMOUNT
// suffixes onto the canonical RM attribute each rebuilds, for the datatypes
// whose allowedSuffixes admit them. The value passes through as decoded (a
// json.Number keeps its exact lexical form), so canjson enforces the RM type; the
// one exception is normal_status, a CODE_PHRASE rebuilt from a bare code.
var orderedSuffixAttr = map[string]string{
	"magnitude_status":    "magnitude_status",
	"accuracy":            "accuracy",
	"accuracy_is_percent": "accuracy_is_percent",
	"precision":           "precision",
	"units_system":        "units_system",
	"units_display_name":  "units_display_name",
	"formatting":          "formatting",
}

// applyOrderedSuffixes copies the optional suffixes present in sfx onto the
// canonical object dv. An absent suffix sets nothing — the attributes are all
// optional in the RM, so a missing one must stay absent rather than become a
// zero value (the same contract requireSuffix enforces for mandatory ones).
func applyOrderedSuffixes(dv, sfx map[string]any) error {
	for suffix, attr := range orderedSuffixAttr {
		if v, ok := sfx[suffix]; ok {
			dv[attr] = v
		}
	}
	if v, ok := sfx["normal_status"]; ok {
		code, err := ctxString("|normal_status", v)
		if err != nil {
			return err
		}
		dv["normal_status"] = codePhraseJSON(code, normalStatusTerminology)
	}
	return nil
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
		return nil, fmt.Errorf("%w: %s missing required %s", ErrUnsupportedDatatype, rmType, label)
	}
	return v, nil
}

// textJSON is a canonical DV_TEXT object.
func textJSON(value string) map[string]any {
	return map[string]any{"_type": "DV_TEXT", "value": value}
}

// splitSuffix splits a FLAT key at its trailing pipe attribute — the one place
// that owns the |suffix grammar, shared by grouping and full key parsing so the
// two cannot drift.
func splitSuffix(key string) (base, suffix string) {
	if i := strings.LastIndex(key, "|"); i >= 0 {
		return key[:i], key[i+1:]
	}
	return key, ""
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
	key, suffix := splitSuffix(key)
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
