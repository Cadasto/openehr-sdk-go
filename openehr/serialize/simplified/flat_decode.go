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
	"iter"
	"maps"
	"math"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"weak"

	"github.com/cadasto/openehr-sdk-go/internal/bmmtype"
	"github.com/cadasto/openehr-sdk-go/internal/cstringvalue"
	"github.com/cadasto/openehr-sdk-go/internal/iso8601"
	"github.com/cadasto/openehr-sdk-go/internal/rmlists"
	tcimpl "github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/rminfo"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/template/constraints"
	"github.com/cadasto/openehr-sdk-go/openehr/template/webtemplate"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/terminology"
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

// UnmarshalFlat decodes FLAT JSON into a canonical COMPOSITION using wt.
// It rebuilds a canonical-JSON tree from the FLAT entries (node types and
// the elided HISTORY/ITEM_TREE wrappers come from the Web Template and
// rminfo, values from the FLAT suffixes), then decodes it through canjson
// (typereg instantiates the polymorphic RM types).
//
// FLAT carries no archetype_details, so decode rebuilds it on every node
// the Web Template identifies by an archetype id: the archetype id is the
// node's id, rm_version is [rm.Release], and the COMPOSITION alone also
// gets the Web Template's template id. Whatever the original composition
// held there is not recovered.
//
// A key the decode does not read is ignored by default, and the value holds
// the rest of the body. [WithUnmatched] reports each such key and
// [WithStrict] refuses the first, wrapping [ErrUnknownPath].
func UnmarshalFlat(data []byte, wt *webtemplate.WebTemplate, opts ...Option) (*rm.Composition, error) {
	if wt == nil || wt.Tree == nil {
		return nil, ErrNoTemplate
	}
	flat, err := unmarshalObject(data)
	if err != nil {
		return nil, err
	}
	cfg := newDecodeConfig(opts)
	var loc locator
	if cfg.tracked() {
		if loc, err = flatLocations(data); err != nil {
			return nil, err
		}
	}
	return decodeFlatMap(flat, wt, cfg, loc, nil)
}

// decodeFlatMap is the decode both entry points share once the body is a FLAT
// map: it decodes the map without its unmatched keys, applies the caller's
// policy for them (loc places each key in the input), and builds the
// COMPOSITION through canjson. skipped are unmatched items set aside before
// the map was made, as [memberKeys] of STRUCTURED member paths; the policy
// takes them with the keys the decode sets aside.
func decodeFlatMap(flat map[string]any, wt *webtemplate.WebTemplate, cfg decodeConfig, loc locator, skipped []string) (*rm.Composition, error) {
	var tmpl *templateIndex
	if cfg.template != nil {
		tmpl = templateIndexFor(cfg.template)
		if !cfg.defaultTime.IsZero() {
			// Every decode with the template shares the cached index, so the
			// caller's time goes on a copy that this decode alone reads. It
			// takes the form the instance generator writes its clock in.
			withTime := *tmpl
			withTime.startTime = cfg.defaultTime.UTC().Format(time.RFC3339)
			tmpl = &withTime
		}
	}
	compJSON, unmatched, err := decodeFlatIgnoring(flat, wt, tmpl, cfg.strict)
	unmatched = slices.Concat(skipped, unmatched)
	if cfg.strict {
		if refusal := applyUnmatchedPolicy(cfg, loc, unmatched); refusal != nil {
			return nil, refusal
		}
	}
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
	if !cfg.strict {
		// The report waits for the decode to succeed, so a decode that fails
		// passes the caller nothing.
		if err := applyUnmatchedPolicy(cfg, loc, unmatched); err != nil {
			return nil, err
		}
	}
	return &comp, nil
}

// decodeFlat builds the canonical-JSON object for the COMPOSITION from the
// FLAT map, driven by the Web Template. tmpl (nil unless a template was
// supplied) holds what the decode reads from the compiled template at each
// node path ([templateIndex]). Every key it does not read goes to sink, and the
// decode carries on without it.
func decodeFlat(flat map[string]any, wt *webtemplate.WebTemplate, tmpl *templateIndex, sink *unmatchedSink) (map[string]any, error) {
	// A key that is no path in the FLAT grammar is refused before any is read.
	if err := checkFlatKeys(flat); err != nil {
		return nil, err
	}
	root := wt.Tree
	compJSON := map[string]any{
		"_type":             "COMPOSITION",
		"archetype_node_id": root.NodeID,
	}
	// A key ending in a bare `|` spells the key without it; both at once are two
	// spellings of one field (REQ-170), judged once the decode knows whether it
	// reads that field.
	flat, fold := foldEmptySuffix(flat)
	// A duplicate sibling's earlier spelling, `x2` for `x_1`, is read as the Web
	// Template's id, and a key spelled both ways is two spellings of one field
	// too (REQ-053, REQ-170). Every later stage reads the Web Template's id.
	flat, siblings := foldSiblingSpellings(flat, wt, fold.spellings)
	if sink != nil {
		sink.spellings = siblings.spellings
	}
	// Separate composition-level context from clinical content; context is
	// rebuilt from RM attributes, not from a Web Template leaf path. The
	// composer's real-path keys come out as one party group.
	ctx, ctxOrigin, composer, content, err := siphonContext(flat, root.ID, sink)
	if err != nil {
		return nil, err
	}
	budget := &allocBudget{limit: maxTotalNodes}
	sib := reusedSiblingsOf(wt)
	// The three passes below walk content in sorted order and share one parse of
	// each key; each skips the keys an earlier one took out of content.
	parsed := parseContentKeys(content)
	// The reference's in-context keys for RM attributes the Web Template this
	// SDK builds does not model name RM data decode would drop: refused in every
	// mode, before anything could set them aside as unmatched or the
	// WithTemplate defaults replace them (REQ-170).
	if err := refuseInContextKeys(content, parsed, wt, sib); err != nil {
		return nil, err
	}
	// Siphon off the composite leaves first — the ones whose FLAT form has
	// sub-paths of its own rather than a suffix set (REQ-053's ENTRY `subject` and
	// the DV_INTERVAL leaf, REQ-140's party and interval grammars). A party leaf's
	// three key shapes — its own suffixes, the PARTY_RELATED `/relationship`
	// sub-object and the nested `_identifier:N` list — all address one RM value,
	// and which concrete PARTY_PROXY subtype that is is only known once all three
	// are in hand; an interval leaf's `/lower` and `/upper` are the same shape. So
	// they are collected together and decoded by the one implementation each
	// grammar has. It has to run before the `_` router, which would otherwise claim
	// `subject/_identifier:0` as a family whose owner is a leaf the grammar cannot
	// judge.
	compositeGroups, err := compositeLeafGroups(content, parsed, wt, sink)
	if err != nil {
		return nil, err
	}
	// Siphon off the underscore-prefixed RM attributes (REQ-140): a path
	// segment starting with `_` ends Web Template resolution at the segment
	// before it, so those keys are grouped per (owner path, family, :index) and
	// routed by the owner's RM class instead of resolving to a leaf. What stays
	// in content is ordinary template-constrained leaf data.
	attrGroups, err := rmattrGroups(content, parsed, sink)
	if err != nil {
		return nil, err
	}
	// Group the remaining FLAT keys by leaf instance (key minus the |suffix);
	// each group's suffix->value pairs build one DataValue. keys keeps the key
	// each suffix arrived under, which is the one an unmatched suffix reports.
	groups := make(map[string]map[string]any)
	keys := make(map[string]map[string]string)
	for key, val := range content {
		base, suffix := splitSuffix(key)
		if groups[base] == nil {
			groups[base] = make(map[string]any)
			keys[base] = make(map[string]string)
		}
		groups[base][suffix] = val
		keys[base][suffix] = key
	}
	// A value decoration beside a leaf's bare "" keeps the "" a value
	// ([leafSuffixes]).
	decorations := decoratingFamilies(attrGroups)
	// The value type each ELEMENT with value types is spelled under: keys that
	// reach two of them, or the ELEMENT itself and one of them, are refused
	// before any of their values is built (REQ-053).
	claims := make(valueClaims)
	// Process leaf groups in a stable (sorted) order so the reconstructed tree
	// is deterministic: distinct-node-id siblings are appended in this order,
	// which must not depend on Go map iteration. [orderByTemplate] later puts
	// them in template order. The first pass reads each group's keys, the
	// second builds and places its value.
	type leafGroup struct {
		base      string
		leaf      *webtemplate.Node
		predIndex map[string]int
		predType  map[string]string
		sfx       map[string]any
		keyOf     func(suffix string) string
	}
	var leaves []leafGroup
	for _, base := range slices.Sorted(maps.Keys(groups)) {
		keyOf := func(suffix string) string { return keys[base][suffix] }
		pk, err := parseFlatKey(base)
		if err != nil {
			return nil, err
		}
		node, predIndex, predType, err := resolveLeaf(wt, pk.segs, sib)
		if errors.Is(err, errSegNotFound) {
			// A key that does not resolve to a Web Template node is a wrong
			// template, a typo, or an unsupported _-prefixed RM attribute (ctx/
			// is siphoned off above and |raw is a suffix, so neither reaches this
			// branch): unmatched content, which the decode does not read
			// (REQ-170).
			for _, suffix := range slices.Sorted(maps.Keys(groups[base])) {
				sink.add(keyOf(suffix))
			}
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("simplified: %q: %w", base, err)
		}
		// A key at an ELEMENT with value types reads as its first one.
		leaf := valueLeaf(node)
		if v, spelled := groups[base][sampleCountSuffix]; spelled && carriesSampleCount(leaf) {
			// An event's sample count is a suffix on its node, which has no
			// value of its own (REQ-053).
			delete(groups[base], sampleCountSuffix)
			if err := placeSampleCount(compJSON, sib.nodePath(leaf), leaf.AQLPath, predIndex, predType, v, keyOf(sampleCountSuffix), budget, tmpl, sib); err != nil {
				return nil, fmt.Errorf("simplified: decode %q: %w", base, err)
			}
			if len(groups[base]) == 0 {
				continue
			}
		}
		sfx, err := leafSuffixes(leaf, groups[base], keyOf, sink, decoratesValue(leaf, sib, decorations[indexedPath(base)]))
		if err != nil {
			return nil, err
		}
		if len(sfx) == 0 {
			continue
		}
		if reach, ok := valueTypeReached(pk.segs, node, sib); ok {
			if err := claims.claim(reach, base); err != nil {
				return nil, err
			}
		}
		leaves = append(leaves, leafGroup{base: base, leaf: leaf, predIndex: predIndex, predType: predType, sfx: sfx, keyOf: keyOf})
	}
	for _, g := range leaves {
		base, leaf, sfx, keyOf := g.base, g.leaf, g.sfx, g.keyOf
		// The careflow step of a merged in-context ISM transition takes what it
		// leaves out from its Web Template node, in both modes, once every key
		// is placed ([settleMergedIsmTransition]).
		settled := mergedCareflowStep(leaf)
		dv, err := clinicalLeafValue(leaf, sfx, keyOf, tmpl != nil || settled)
		if err != nil {
			return nil, fmt.Errorf("simplified: decode %q: %w", base, err)
		}
		key, err := placeLeaf(compJSON, sib.nodePath(leaf), leaf.AQLPath, g.predIndex, g.predType, dv, budget, tmpl, sib)
		if err != nil {
			return nil, fmt.Errorf("simplified: place %q: %w", base, err)
		}
		if _, raw := sfx["raw"]; !raw && !settled {
			// The key the template is indexed by is known once the value is
			// placed: what the body leaves out of a coded value is taken from
			// the template, or refused, now (REQ-053).
			if err := tmpl.completeCodedValue(dv, key); err != nil {
				return nil, fmt.Errorf("simplified: decode %q: %w", base, err)
			}
			if nodeRMType(leaf) == "DV_ORDINAL" {
				settleOrdinalTerminology(dv, tmpl.ordinalTerminologies(key))
			}
		}
	}
	// Composite leaves are placed like any other leaf, after the clinical loop so a
	// malformed ordinary key still surfaces first.
	for _, g := range compositeGroups {
		if err := placeCompositeLeaf(compJSON, wt, g, sib, budget, tmpl, claims); err != nil {
			return nil, err
		}
	}
	// Underscore RM attributes come after the clinical leaves, so their owners
	// are decorated onto nodes the content walk has already materialised rather
	// than gap-filling their own (REQ-140).
	if err := decodeRMAttrGroups(compJSON, wt, attrGroups, sib, budget, tmpl, claims); err != nil {
		return nil, err
	}
	// Every member is placed, the ones a composite leaf or an underscore family
	// reached first among them: each list now takes the template's node order
	// (REQ-053).
	orderByTemplate(compJSON, wt, sib)
	// The composition is named by its `<root>/_name` where the body spells one,
	// which the router has just placed, and otherwise by the Web Template
	// (REQ-140).
	if _, named := compJSON["name"]; !named {
		compJSON["name"] = textJSON(impliedCompositionName(wt))
	}
	// Every key is placed, so each rebuilt ISM transition can now get the careflow
	// step its node names, or have the body's checked against it, and a current
	// state that names no terminology gets `openehr` (REQ-053).
	if err := settleIsmTransitions(compJSON, wt, sib); err != nil {
		return nil, err
	}
	// A sparse :index (":0" and ":2" with no ":1") would have gap-filled an
	// empty phantom instance in selectElem; reject it before context/completion
	// can decorate fabricated data into something OPT-valid.
	if err := checkNoPhantoms(compJSON); err != nil {
		return nil, err
	}
	// Context: parse once, then apply (with the mandatory-field check) after
	// content, so a refused content key surfaces first.
	ci, err := parseCtx(ctx, ctxOrigin)
	if err != nil {
		return nil, err
	}
	if tmpl != nil {
		ci.category = tmpl.category
		// A body with no start time in any spelling takes the caller's default
		// as if its ctx/time held it, except that the default builds no context
		// on a persistent composition (REQ-053). RM 1.0.4 allows that
		// composition one, so a start time the body gives still builds it.
		if !ci.haveTime && tmpl.startTime != "" {
			ci.time, ci.haveTime = tmpl.startTime, true
			ci.timeBuildsNoContext = persistentComposition(compJSON, ci)
		}
	}
	composerJSON, err := decodeComposer(composer, ci, ctxOrigin)
	if err != nil {
		return nil, err
	}
	if err := applyContext(compJSON, ci, composerJSON); err != nil {
		return nil, err
	}
	// Conformant mode (a template was supplied, so tmpl is non-nil): every key
	// is placed, so each rebuilt node gets the required structures with no
	// member the formats have no key for (REQ-053); then the RM-mandatory
	// attributes the formats do not carry are filled, from ctx defaults + RM
	// conventions, so the decoded composition is OPT-validatable. An ACTIVITY's
	// action_archetype_id, which the body may leave out, is the one the
	// generator writes for the template (REQ-053).
	if tmpl != nil {
		rebuildEmptyStructures(compJSON, tmpl)
		completeActionArchetypeIDs(compJSON, tmpl)
		completeRequired(compJSON, ci)
	}
	// Both modes: archetype_details has no FLAT key, and the Web Template
	// alone holds what it records (REQ-053).
	rebuildArchetypeDetails(compJSON, wt)
	if err := fold.settle(sink); err != nil {
		return nil, err
	}
	if err := siblings.settle(sink); err != nil {
		return nil, err
	}
	return compJSON, nil
}

// leafSuffixes returns the suffixes of one leaf group that the decode reads,
// handing the key of every other one to sink (REQ-170). keyOf gives the key a
// suffix arrived under.
//
//   - A node with children is structure, not a value: the decode reads a |raw
//     fragment there and its bare value, which name the node and so are
//     matched content of the wrong shape (REQ-170). A |raw is refused here
//     with ErrUnsupportedDatatype naming its key, whatever its fragment's
//     _type: it carries a datatype (REQ-053), and a fragment placed whole at
//     the node would be dropped once the node's own keys rebuild it. The bare
//     value is refused where the value is built. Every other suffix is
//     unmatched (an extension suffix such as `|x_note`, say). The RM
//     attributes the reference spells as a suffix on such a node never get
//     here: [refuseInContextKeys] refuses them first.
//   - A leaf whose datatype has a suffix set keeps the suffixes in that set
//     ([leafSuffixMatched]); the rest are unmatched.
//   - A leaf whose datatype has neither a suffix set nor a sub-path grammar
//     (an unmapped datatype such as DV_STATE) resolves but has no carrier
//     other than |raw. Its bare value, and a suffix that names an attribute of
//     its RM class, are refused in every mode with ErrUnsupportedDatatype, as
//     the other carrier-less keys at a leaf are, naming the first such key.
//     Any other suffix is unmatched (REQ-170). A party leaf does not get
//     here: [siphonContext] reads every key at the composer's real path, and
//     [compositeLeafGroups] every other party leaf.
//   - An unparameterised `DV_INTERVAL` leaf has a sub-path grammar, the
//     interval grammar, but names no bound datatype to read it with, so it
//     reads its |raw fragment alone; every other key is unmatched.
//
// A key the decode reads that holds JSON null is absent (REQ-053): it is
// dropped without a word, so a leaf whose keys are all null comes back empty,
// and a null key is never refused for want of a carrier. A key the decode
// does not read goes to sink whatever its value.
//
// A bare "" at a leaf of [unsetStringType] is absent the same way where no
// other key of the leaf the decode reads holds a value and decorated is
// false: encode writes no key for that value, so a decode, encode and decode
// keeps the composition (REQ-053). decorated reports whether an underscore
// family decorates the leaf's value ([decoratesValue]); one keeps the "", as
// a suffix does, since a decoration with no value is refused.
func leafSuffixes(leaf *webtemplate.Node, sfx map[string]any, keyOf func(string) string, sink *unmatchedSink, decorated bool) (map[string]any, error) {
	rmType := nodeRMType(leaf)
	if len(leaf.Children) == 0 {
		if _, judged := leafSuffixMatched(rmType, ""); judged {
			out := matchedSuffixes(rmType, sfx, keyOf, sink)
			if !decorated && unsetBareValue(rmType, out) {
				return nil, nil
			}
			return out, nil
		}
	}
	out := maps.Clone(sfx)
	if len(leaf.Children) > 0 {
		// A null |raw is absent (REQ-053), and falls to withoutNulls below.
		if sfx["raw"] != nil {
			return nil, fmt.Errorf("simplified: decode %q: %w: |raw carries a datatype, and the %s it names is a node with children",
				keyOf("raw"), ErrUnsupportedDatatype, rmType)
		}
		for _, suffix := range slices.Sorted(maps.Keys(sfx)) {
			if suffix != "raw" && suffix != "" {
				delete(out, suffix)
				sink.add(keyOf(suffix))
			}
		}
		return withoutNulls(out), nil
	}
	subPathGrammar := bmmtype.Class(rmType) == "DV_INTERVAL"
	out = withoutNulls(out)
	for _, suffix := range slices.Sorted(maps.Keys(sfx)) {
		read := suffix == "raw" || (!subPathGrammar && (suffix == "" || unmappedReads(rmType, suffix)))
		switch {
		case !read:
			delete(out, suffix)
			sink.add(keyOf(suffix))
		case suffix == "raw", sfx[suffix] == nil:
			// |raw is the carrier of an unmapped datatype; dvFromSuffixes refuses
			// it beside any other suffix. A null key is absent.
		case out["raw"] != nil:
			// Kept beside |raw, which dvFromSuffixes then refuses.
		case suffix == "":
			// Named as the allowlist refusals name a bare value, so a caller
			// reading the refusal removes the bare key alone.
			return nil, fmt.Errorf("simplified: decode %q: %w: unexpected bare value for %s", keyOf(suffix), ErrUnsupportedDatatype, rmType)
		default:
			return nil, fmt.Errorf("simplified: decode %q: %w: %s", keyOf(suffix), ErrUnsupportedDatatype, rmType)
		}
	}
	return out, nil
}

// unsetBareValue reports whether sfx, the suffixes the decode reads at a leaf
// of rmType, is a bare "" alone at one of [unsetStringType].
func unsetBareValue(rmType string, sfx map[string]any) bool {
	v, bare := sfx[""]
	return bare && len(sfx) == 1 && v == "" && unsetStringType(rmType)
}

// decoratingFamilies returns the families of the underscore groups that hold
// a value, by the [indexedPath] of the node each one is at. A group whose
// keys all hold JSON null is absent (REQ-053) and decorates nothing.
func decoratingFamilies(groups []rmattrGroup) map[string][]string {
	out := make(map[string][]string)
	for _, g := range groups {
		if allNull(g.tails) {
			continue
		}
		at := indexedPath(g.base)
		out[at] = append(out[at], g.family)
	}
	return out
}

// decoratesValue reports whether one of families is a value decoration the
// decode reads at leaf: leaf is a collapsed ELEMENT's value, and the family
// is judged against its datatype, as [rmattrOwnerAt] and [rmattrFamilyOf]
// judge it. A family of the ELEMENT itself, such as _null_flavour, decorates
// no value.
func decoratesValue(leaf *webtemplate.Node, sib *reusedSiblings, families []string) bool {
	if len(families) == 0 || len(leaf.Children) > 0 || !strings.HasSuffix(sib.nodePath(leaf), "/value") {
		return false
	}
	for _, family := range families {
		if fam, _, read := rmattrFamilyOf("ELEMENT", nodeRMType(leaf), family); read && fam.value {
			return true
		}
	}
	return false
}

// indexedPath spells a FLAT path with an index on every segment, :0 where
// the segment carries none, which addresses the same node ([resolveLeaf]).
// A path that does not parse comes back as written.
func indexedPath(path string) string {
	pk, err := parseFlatKey(path)
	if err != nil {
		return path
	}
	var b strings.Builder
	for i, seg := range pk.segs {
		if i > 0 {
			b.WriteByte('/')
		}
		writeFlatSeg(&b, flatSeg{id: seg.id, idx: max(seg.idx, 0)})
	}
	return b.String()
}

// unmappedReads reports whether suffix names an attribute of the RM class of
// rmType, a datatype with neither a suffix set nor a sub-path grammar.
func unmappedReads(rmType, suffix string) bool {
	_, declared := rminfo.Default.AttributeRMType(bmmtype.Class(rmType), suffix)
	return declared
}

// metadataAliases maps the reference implementation's real-path spelling of
// composition-level metadata — relative to the template root — onto the ctx/
// short form REQ-053 canonically emits.
//
// The codec is deliberately asymmetric here: both spellings are accepted on
// input, only ctx/ is written on output (ADR 0015). Upstream FLAT carries
// `<root>/language|code` where this SDK reads and writes `ctx/language`, and
// before this table that pure respelling was mishandled twice over:
//
//   - First it was refused, but not as a path gap: the composition-level
//     `language` / `territory` / `composer` Web Template leaves are CODE_PHRASE
//     and PARTY_PROXY, neither of which had a suffix mapping, so an
//     EHRbase-authored body failed with ErrUnsupportedDatatype. Only
//     `composer_self`, which reaches no Web Template node at all, was
//     ErrUnknownPath.
//   - Then, once CODE_PHRASE became a mapped leaf type (the PROBE-086 ratchet),
//     those keys stopped failing and started decoding *silently* through ordinary
//     leaf placement — bypassing ctx normalisation entirely. The value landed on
//     the RM attribute directly, where applyContext (which runs after content and
//     assigns from the ctx/ values) overwrote it; a body carrying only the real
//     path failed the mandatory-context check instead.
//
// Each entry is keyed on the unindexed spelling. [metadataRel] reads a key
// whose metadata segments carry `:0` as that spelling and reports any other
// index, which [siphonContext] refuses (REQ-053); [cutRoot] does the same for
// the template root's own segment.
//
// Entries here are respellings *only* — same information, different surface.
// `context/setting|code` / `|value` joined on 2026-08-05 (the amended REQ-053):
// ADR 0015 had left it out because `ctx/setting` was unimplemented on both
// surfaces — an emission gap, not a spelling one — and closing that gap
// (encode writes the pair, applyContext rebuilds the setting) is what made the
// real path a genuine respelling. Note the alias deliberately shadows any Web
// Template `setting` node: the same RM attribute either way, and `rmpath`
// leaves `…/context/setting` unresolvable so encode cannot double-spell it.
//
// The composer's other real-path keys stay out, because no ctx/ field carries
// them: `composer|id` / `|id_scheme` / `|id_namespace` / `|type` (its
// external_ref), `composer/_identifier:N` and `composer/relationship`. They
// carry the rest of the composer at its real path, read by the party grammar
// beside the name the alias gives (ADR 0029, [decodeComposer]), and the
// PROBE-086 census compares them (docs/specifications/conformance.md
// § PROBE-086).
var metadataAliases = map[string]string{
	"language|code":         "ctx/language",
	"territory|code":        "ctx/territory",
	"composer|name":         "ctx/composer_name",
	"composer_self":         "ctx/composer_self",
	"context/start_time":    "ctx/time",
	"context/setting|code":  "ctx/setting|code",
	"context/setting|value": "ctx/setting|value",
}

// metadataAliasTerminology pins the terminology each respelled coded field
// carries implicitly in the ctx/ form, where the terminology does not travel.
// The suffix is a witness, not data: the value is checked and discarded.
// Accepting a mismatch would silently rewrite the terminology, since
// applyContext hardcodes these when rebuilding the CODE_PHRASE.
var metadataAliasTerminology = map[string]string{
	"language|terminology":        "ISO_639-1",
	"territory|terminology":       "ISO_3166-1",
	"context/setting|terminology": terminology.ID,
}

// contextMetaOwnedBases are the composition-metadata leaves the ctx/ short
// forms own outright: COMPOSITION `language` and `territory`, and EVENT_CONTEXT
// `setting` and `start_time`. The Web Template carries a node for them, so an
// unaccepted spelling would resolve as an ordinary leaf and then be silently
// overwritten by [applyContext], which writes the ctx/ values last, or fail
// there for a reason that is not the real one (`language|preferred_term` alone
// is a CODE_PHRASE with no `|code`). Refusing every other spelling of these
// bases that their datatype has, in [siphonContext], naming the ctx/ form that
// owns the leaf, is what keeps that unreachable; a suffix their datatype does
// not have is unmatched content and is never placed.
//
// `composer` is absent: its real path carries the whole composer in the party
// grammar (ADR 0029), so [siphonContext] reads its keys rather than refusing
// them. Every base here is held out of the PROBE-086 census by the harness
// (`IsCompositionMeta`), so refusing extra suffixes early moves no count.
var contextMetaOwnedBases = map[string]bool{
	"context/setting": true, "context/start_time": true,
	"language": true, "territory": true,
}

// metadataOwnedBaseCtx names the ctx/ short form that owns each base, so the
// refusal points at the spelling the payload should have used.
var metadataOwnedBaseCtx = map[string]string{
	"context/setting": "ctx/setting|code + ctx/setting|value", "context/start_time": "ctx/time",
	"language": "ctx/language", "territory": "ctx/territory",
}

// metadataOwnedBaseType is the RM type of each ctx/-owned base, EVENT_CONTEXT's
// own attribute type, whether or not the template carries a node for it. A
// suffix in that type's suffix set is one the decode reads, and the ctx/ form
// cannot carry it; any other suffix is unmatched content (REQ-170).
var metadataOwnedBaseType = map[string]string{
	"context/setting": "DV_CODED_TEXT", "context/start_time": "DV_DATE_TIME",
	"language": "CODE_PHRASE", "territory": "CODE_PHRASE",
}

// ctxSettingBare is the bare spelling of ctx/setting, which names a member of
// the openEHR setting group by its code or its rubric ([putBareSetting]), and
// ctxSettingTerminology the witness of the terminology the setting implies.
// settingBase is the real path of the same field below the template root,
// whose bare key decode reads as ctxSettingBare.
const (
	ctxSettingBare        = "ctx/setting"
	ctxSettingTerminology = "ctx/setting|terminology"
	settingBase           = "context/setting"
)

// ctxFields are the ctx/ keys the decode reads into context (REQ-053). Any
// other ctx/ key is unmatched content (REQ-170), except [ctxSettingBare] and
// [ctxSettingTerminology], which the decode also reads.
var ctxFields = map[string]bool{
	"ctx/language": true, "ctx/territory": true,
	"ctx/composer_name": true, "ctx/composer_self": true,
	"ctx/time":         true,
	"ctx/setting|code": true, "ctx/setting|value": true,
}

// metadataSegPaths are the root-relative segment paths the metadata spellings
// run through: every leading run of segments of an alias, a witness or a
// ctx/-owned base, plus `composer`, whose real path carries the whole composer
// ([siphonContext]). Each names a single-valued RM attribute
// (COMPOSITION.language, .composer, .context, EVENT_CONTEXT.start_time, …). The
// set is read off those tables, so an alias added there brings its segments
// with it.
var metadataSegPaths = func() map[string]bool {
	paths := map[string]bool{composerSeg: true}
	add := func(spelling string) {
		base, _, _ := strings.Cut(spelling, "|")
		for i := range len(base) {
			if base[i] == '/' {
				paths[base[:i]] = true
			}
		}
		paths[base] = true
	}
	for _, s := range slices.Concat(slices.Collect(maps.Keys(metadataAliases)),
		slices.Collect(maps.Keys(metadataAliasTerminology)),
		slices.Collect(maps.Keys(contextMetaOwnedBases))) {
		add(s)
	}
	return paths
}()

// metadataRel rewrites a root-relative FLAT key into the unindexed spelling the
// metadata tables are keyed on. Every segment on a metadata path is a
// single-valued attribute, and OPT-free STRUCTURED → FLAT conversion writes
// each segment with an explicit index, so REQ-053 makes `language:0|code` the
// same real path as `language|code`, and `context:0/start_time:0` the same as
// `context/start_time`. Only the leading run of metadata segments loses its
// index: what follows keeps it, such as a party's `_identifier:0` or the
// archetyped content under `context:0/other_context`. The suffix is kept as
// written.
//
// overIndexed reports a metadata segment that carries an index other than :0.
// That index addresses a list slot a single-valued attribute does not have.
// The segment is rewritten without it all the same, so the caller can tell
// whether the key is a metadata spelling at all. A malformed index comes back
// unchanged, for the leaf loop to refuse, and so does a key whose metadata run
// carries no index.
//
// What follows the run is sliced from rel rather than re-spelled: a segment
// [parseFlatSeg] accepts has one spelling only ([cutAtSeg]).
func metadataRel(rel string) (norm string, overIndexed bool) {
	path, _, _ := strings.Cut(rel, "|")
	var seen string // the run's unindexed spelling so far
	runEnd := 0     // the offset in rel just past the run's last segment
	off := 0        // the offset in rel of the segment being read
	indexed, onMeta := false, true
	for p := range strings.SplitSeq(path, "/") {
		s, ok := parseFlatSeg(p)
		if !ok {
			return rel, false
		}
		if onMeta {
			at := s.id
			if seen != "" {
				at = seen + "/" + s.id
			}
			if metadataSegPaths[at] {
				seen, runEnd = at, off+len(p)
				indexed = indexed || s.idx >= 0
				overIndexed = overIndexed || s.idx > 0
			} else {
				onMeta = false
			}
		}
		off += len(p) + 1
	}
	if !indexed {
		return rel, false
	}
	return seen + rel[runEnd:], overIndexed
}

// overIndexedMetadataSeg returns the first segment of the leading run of
// metadata segments of rel, a root-relative path, that carries an index past
// :0. [metadataRel] reports that one exists.
func overIndexedMetadataSeg(rel string) flatSeg {
	path, _, _ := strings.Cut(rel, "|")
	pk, err := parseFlatKey(path)
	if err != nil {
		return flatSeg{}
	}
	var seen string
	for _, s := range pk.segs {
		p := strings.TrimPrefix(seen+"/"+s.id, "/")
		if !metadataSegPaths[p] {
			break
		}
		if s.idx > 0 {
			return s
		}
		seen = p
	}
	return flatSeg{}
}

// composerSeg is the FLAT id of the composer's real path below the template
// root, which carries the whole composer in the party grammar (ADR 0029).
const composerSeg = "composer"

// composerTail returns the tail of rel, an unindexed root-relative spelling,
// below the composer's own segment: "" for the bare key, "|id" for a suffix,
// "/_identifier:0|id" for a sub-path. isComposer is false for any other path,
// a longer segment id such as `composer_self` among them.
func composerTail(rel string) (tail string, isComposer bool) {
	rest, ok := strings.CutPrefix(rel, composerSeg)
	if !ok || (rest != "" && rest[0] != '|' && rest[0] != '/') {
		return "", false
	}
	return rest, true
}

// isMetadataSpelling reports whether an unindexed root-relative spelling is one
// [siphonContext] handles itself: an alias, a terminology witness, a suffix of
// a leaf the ctx/ short form owns, or a key at the composer's real path.
func isMetadataSpelling(rel string) bool {
	if _, isAlias := metadataAliases[rel]; isAlias {
		return true
	}
	if _, isWitness := metadataAliasTerminology[rel]; isWitness {
		return true
	}
	if base, _, _ := strings.Cut(rel, "|"); contextMetaOwnedBases[base] {
		return true
	}
	_, isComposer := composerTail(rel)
	return isComposer
}

// cutRoot splits a FLAT key at its template-root segment and returns the path
// below it. The root segment may carry an explicit index (`<root>:0/…`, the
// form REQ-053 reads as `<root>/…`), so it is parsed rather than matched as a
// literal prefix; rootSeg reports the index it carries. rooted is false for a
// key with no `/`, for one whose first segment is not the root, and for one
// whose root segment carries a malformed index (the leaf loop refuses that
// one); `<root>/` is rooted, with rel empty. The first segment's
// |suffix is cut off before it is parsed, as [parseFlatKey] does, so a bare
// `<root>|/…` is rooted and `<root>|x/…` is not.
func cutRoot(key, rootID string) (rel string, rootSeg flatSeg, rooted bool) {
	first, rel, found := strings.Cut(key, "/")
	if !found {
		return "", flatSeg{}, false
	}
	first, suffix := splitSuffix(first)
	seg, ok := parseFlatSeg(first)
	if !ok || suffix != "" || seg.id != rootID {
		return "", flatSeg{}, false
	}
	return rel, seg, true
}

// rootSegmentAlone parses a key that has no `/` as one segment and its
// suffix, and reports the segment when it is the template root's.
func rootSegmentAlone(key, rootID string) (flatSeg, bool) {
	if strings.Contains(key, "/") {
		return flatSeg{}, false
	}
	base, _ := splitSuffix(key)
	seg, ok := parseFlatSeg(base)
	if !ok || seg.id != rootID {
		return flatSeg{}, false
	}
	return seg, true
}

// errRootIndex refuses an index past the bound of the template root's own
// segment. The root is the one COMPOSITION, so `:0` is its explicit-index
// spelling and anything higher names a composition that does not exist; reading
// the key as the root's would merge it in with no error. REQ-170 lists an index
// past its bound as malformed matched content. The message names the segment
// and carries no value; the caller names the key.
func errRootIndex(seg flatSeg) error {
	return fmt.Errorf("%w: FLAT id %q carries :%d, but the template root is the one COMPOSITION, not an indexed list",
		ErrUnknownPath, seg.id, seg.idx)
}

// errSingleValuedIndex refuses an index past the bound of a segment that
// addresses a single-valued RM attribute (COMPOSITION.context,
// ACTION.ism_transition, ENTRY.language, …). `:0` is the one value's
// explicit-index spelling, which OPT-free STRUCTURED → FLAT conversion writes;
// anything higher names a list slot the attribute does not have, and ignoring
// the index would merge the key into the one value with no error (REQ-170: an
// index past its bound). The message names the segment and carries no value;
// the caller names the key.
func errSingleValuedIndex(seg flatSeg) error {
	return fmt.Errorf("%w: FLAT id %q carries :%d, but it addresses a single-valued RM attribute, not an indexed list",
		ErrUnknownPath, seg.id, seg.idx)
}

// holdsOneValue reports whether the Web Template node child, reached from its
// parent node, stands on a single-valued RM attribute: the one its FLAT
// segment's :index addresses. That is judged against the RM, not the Web
// Template's occurrences: the attribute chain from the parent's canonical path
// to the child's is walked with rminfo, so a node the Web Template reaches
// through folded structures (an EVENT under OBSERVATION/data/events) is judged
// on the attribute that actually holds it. For a collapsed ELEMENT leaf the
// index belongs to the ELEMENT the leaf folds away, so the attribute judged is
// the one before `/value`, as [resolveLeaf] stores it. An attribute rminfo does
// not know is not judged.
func holdsOneValue(parent, child *webtemplate.Node) bool {
	parentSegs, childSegs := parseAQL(parent.AQLPath), parseAQL(child.AQLPath)
	if len(childSegs) <= len(parentSegs) {
		return false
	}
	rel := childSegs[len(parentSegs):]
	target := len(rel) - 1
	if child.NodeID != "" && len(child.Children) == 0 && rel[target].attr == "value" && target > 0 {
		target--
	}
	curType := nodeRMType(parent)
	for i, seg := range rel[:target] {
		curType = concreteType(curType, seg.attr, "", "", rel[i+1].attr)
		if curType == "" {
			return false
		}
	}
	container, known := rminfo.Default.IsContainer(curType, rel[target].attr)
	return known && !container
}

// MetadataAliasSpellings returns the root-relative FLAT spellings the decoder
// accepts as aliases of the ctx/ composition-metadata short forms,
// sorted. `"language|code"`, for instance, is accepted at
// `<root>/language|code` and normalised to `ctx/language`.
//
// Each spelling is listed once, unindexed. Every segment on it is a
// single-valued attribute, so the decoder reads the same spelling with `:0`
// on any of its segments (`<root>/language:0|code`,
// `<root>/context:0/start_time:0`, the form OPT-free STRUCTURED → FLAT
// conversion writes) exactly as the unindexed one, and refuses any other
// index there. The template root's own segment is read the same way
// (`<root>:0/composer:0|name`). The same holds for [MetadataWitnessSpellings].
//
// This is informative public surface for conformance tooling, not a decode knob.
// A harness that has to hold composition metadata out of a like-for-like FLAT
// comparison derives that hold-out from here rather than restating
// the table, so an alias the codec accepts cannot silently diverge from the
// spellings the harness excuses. The returned slice is freshly allocated, so a
// caller may sort or filter it without reaching the decoder's own table.
func MetadataAliasSpellings() []string {
	return slices.Sorted(maps.Keys(metadataAliases))
}

// MetadataWitnessSpellings returns the root-relative FLAT spellings the decoder
// accepts as terminology witnesses beside an aliased CODE_PHRASE, sorted: the
// `|terminology` keys whose value is checked against the terminology the ctx/
// short form implies and then discarded.
//
// It is informative public surface for conformance tooling on the same terms
// as [MetadataAliasSpellings], reported separately because a witness is not
// data. Nothing on the ctx/ side carries it, so a comparison has to account
// for it as a checked-and-dropped key rather than as a respelling. The
// returned slice is freshly allocated.
func MetadataWitnessSpellings() []string {
	return slices.Sorted(maps.Keys(metadataAliasTerminology))
}

// siphonContext splits a FLAT map into composition-level context and clinical
// content, normalising both accepted metadata spellings into one ctx/-keyed map.
// ctxOrigin records, per normalised ctx/ key, the body key the value arrived
// under, so an error names the spelling the payload actually used rather than a
// ctx/ form its author may never have written.
//
// A real path that contradicts an explicit ctx/ entry is an error rather than a
// precedence rule: preferring either silently would corrupt composition
// metadata, the same stance the codec already takes on an index collision.
//
// A ctx/ field the decode does not read, and a suffix of a ctx/-owned leaf
// outside that leaf's datatype, go to sink as unmatched content (REQ-170),
// whatever their value. A ctx/ field, an alias, a witness or a ctx/-owned leaf
// suffix the decode reads that holds JSON null is absent (REQ-053) and is
// dropped, and so is a "" at ctx/time or at the start time's real path, the
// two spellings of a DV_DATE_TIME.
//
// Every other key at the composer's real path (`<root>/composer|id`,
// `<root>/composer/_identifier:0|id`, …) goes to composer, one party group
// whose unmatched keys go to sink: the real path carries the whole composer
// (ADR 0029), and [decodeComposer] reads the group beside the ctx/ composer
// fields once they are parsed. Two spellings of one composer key, with and
// without the `:0` of the root or the composer segment, are one key: they must
// agree, and a null one is absent ([putComposerKey]).
func siphonContext(flat map[string]any, rootID string, sink *unmatchedSink) (ctx map[string]any, ctxOrigin map[string]string, composer *rmattrGroup, content map[string]any, err error) {
	ctx, ctxOrigin, content = make(map[string]any), make(map[string]string), make(map[string]any)
	composer = newRMAttrGroup(rootID, composerSeg, -1, sink)
	composer.metadata = "the composer"
	// Sorted so a body carrying two conflicting real-path spellings reports the
	// same key first on every run — a map-order-dependent error message is not
	// reproducible for whoever has to fix the payload.
	for _, key := range slices.Sorted(maps.Keys(flat)) {
		val := flat[key]
		if strings.HasPrefix(key, "ctx/") {
			if key != ctxSettingBare && key != ctxSettingTerminology && !ctxFields[key] {
				sink.add(key)
				continue
			}
			// A ctx/ field the decode reads that holds null is absent
			// (REQ-053): a null ctx/language is a missing one. ctx/time
			// spells the start time, a DV_DATE_TIME, so a "" there is
			// absent as a "" at the start time's real path is (below), and
			// neither spelling's "" disagrees with the other's time. Every
			// other ctx/ field keeps its reading of "".
			if val == nil || (key == "ctx/time" && val == "") {
				continue
			}
			if key == ctxSettingBare {
				// The format accepts a setting's code or its rubric alone
				// (REQ-053): it is read as the pair it names, and compared
				// with any other spelling of that pair.
				if err := putBareSetting(ctx, ctxOrigin, key, val); err != nil {
					return nil, nil, nil, nil, err
				}
				continue
			}
			if key == ctxSettingTerminology {
				// The setting's terminology is implied, never carried: a witness
				// naming it is read and dropped, and one naming any other
				// terminology is a typed error, not a silent rewrite (REQ-053),
				// exactly as the real-path witness below.
				if err := checkTerminologyWitness(key, val, terminology.ID); err != nil {
					return nil, nil, nil, nil, err
				}
				continue
			}
			if err := putCtx(ctx, ctxOrigin, key, val, key); err != nil {
				return nil, nil, nil, nil, err
			}
			continue
		}
		// A key that is the root segment alone, with or without a suffix,
		// is judged from the left like any other: an index past :0 on it is
		// past its bound (REQ-170).
		if seg, alone := rootSegmentAlone(key, rootID); alone && seg.idx > 0 {
			return nil, nil, nil, nil, fmt.Errorf("simplified: %q: %w", key, errRootIndex(seg))
		}
		rel, rootSeg, isRooted := cutRoot(key, rootID)
		if !isRooted {
			content[key] = val
			continue
		}
		// The template root is the one COMPOSITION: `<root>:0/` is `<root>/`
		// (REQ-053), and any other index is past its bound, refused whatever
		// follows it (REQ-170).
		if rootSeg.idx > 0 {
			return nil, nil, nil, nil, fmt.Errorf("simplified: %q: %w", key, errRootIndex(rootSeg))
		}
		// The tables below are keyed on the unindexed spelling. An indexed
		// metadata path is read exactly as that spelling, and only `:0` is an
		// index a single-valued attribute has (REQ-053), so any other is
		// refused. A key that is no metadata spelling once its `:0` is gone
		// keeps flowing as content under the key it arrived with.
		raw := rel
		rel, overIndexed := metadataRel(rel)
		if overIndexed && isMetadataSpelling(rel) {
			return nil, nil, nil, nil, fmt.Errorf("%w: %q (a composition metadata path runs through single-valued attributes, so :0 is the only index it takes)",
				ErrUnknownPath, key)
		}
		// Any other key that runs through a metadata segment past :0 is past
		// that segment's bound whatever follows it, whether or not the Web
		// Template carries a node for the segment (REQ-170).
		if overIndexed {
			return nil, nil, nil, nil, fmt.Errorf("simplified: %q: %w", key, errSingleValuedIndex(overIndexedMetadataSeg(raw)))
		}
		// A witness, an alias or a suffix of a ctx/-owned leaf that holds null
		// is absent (REQ-053), so it neither conflicts with another spelling
		// nor is refused for want of a ctx/ carrier.
		if want, isWitness := metadataAliasTerminology[rel]; isWitness {
			if val == nil {
				continue
			}
			if err := checkTerminologyWitness(key, val, want); err != nil {
				return nil, nil, nil, nil, err
			}
			continue
		}
		if ctxKey, isAlias := metadataAliases[rel]; isAlias {
			// A "" at the start time's real path is absent as a null is,
			// since its leaf is a DV_DATE_TIME ([leafSuffixes], REQ-053). Any
			// other key of that leaf the decode reads is refused below, so the
			// "" is alone wherever the decode goes on.
			if val == nil || (val == "" && unsetStringType(metadataOwnedBaseType[rel])) {
				continue
			}
			if err := putCtx(ctx, ctxOrigin, ctxKey, val, key); err != nil {
				return nil, nil, nil, nil, err
			}
			continue
		}
		// An EVENT_CONTEXT leaf the ctx/ short form owns outright cannot also
		// travel as an ordinary Web Template leaf: the ctx/ entry is written last
		// and would overwrite whatever the leaf placed, dropping it with no error.
		// Every spelling of such a leaf is therefore either an accepted alias, a
		// checked witness, or refused here naming the key (REQ-053). The
		// composer is not such a leaf: its real path is read below.
		// A suffix the leaf's datatype does not have at all is not such a
		// spelling: the decode does not read it, so it is unmatched (REQ-170).
		// A bare value is not such a suffix but a spelling of the field missing
		// its required suffixes: refused, as below, except the setting's, which
		// is read as the bare ctx/setting is (REQ-053).
		if base, suffix, _ := strings.Cut(rel, "|"); contextMetaOwnedBases[base] {
			if matched, _ := leafSuffixMatched(metadataOwnedBaseType[base], suffix); !matched && suffix != "" {
				sink.add(key)
				continue
			}
			if val == nil {
				continue
			}
			if rel == settingBase {
				if err := putBareSetting(ctx, ctxOrigin, key, val); err != nil {
					return nil, nil, nil, nil, err
				}
				continue
			}
			return nil, nil, nil, nil, fmt.Errorf("%w: %q is not a spelling the ctx/ short form can carry — %s owns this leaf, so only its accepted suffixes travel (see deviations.md)",
				ErrUnsupportedDatatype, key, metadataOwnedBaseCtx[base])
		}
		// The composer's real path carries the whole composer in the party
		// grammar REQ-140 gives every other party position (ADR 0029): its
		// `external_ref` suffixes, its `_identifier:N` list and a PARTY_RELATED's
		// `/relationship`. Its `|name` is the alias above. The keys are read
		// whether or not the Web Template carries a composer node, as the
		// aliases are, and the party grammar sets aside what it does not read.
		if tail, isComposer := composerTail(rel); isComposer {
			putComposerKey(composer, tail, val, key)
			continue
		}
		content[key] = val
	}
	return ctx, ctxOrigin, composer, content, nil
}

// checkTerminologyWitness reads a terminology witness of a coded metadata
// field: a key that names the terminology the ctx/ form implies, which matches
// and is dropped, or names another, which the ctx/ form cannot carry and is
// refused (REQ-053).
func checkTerminologyWitness(key string, val any, want string) error {
	got, err := ctxString(key, val)
	if err != nil {
		return err
	}
	if got != want {
		return fmt.Errorf("%w: %s names a terminology other than %q, the one the ctx/ form implies (a differing terminology cannot be carried)",
			ErrUnsupportedDatatype, key, want)
	}
	return nil
}

// putBareSetting reads a bare setting, ctx/setting or the real path's bare
// key, which the format lets carry either the setting's code or its rubric.
// The value is looked up in the openEHR setting group, by code first and then
// by rubric, and recorded as the ctx/setting|code + |value pair that names the
// same member, so it is compared with any other spelling of the pair as two
// spellings of one field are ([putCtx], REQ-053). A value that is neither is
// refused naming the key: no setting can be built from it, since the RM
// requires a code of that group.
func putBareSetting(ctx map[string]any, origin map[string]string, key string, val any) error {
	s, err := ctxString(strconv.Quote(key), val)
	if err != nil {
		return err
	}
	code, rubric, member := settingMember(s)
	if !member {
		return fmt.Errorf("%w: %q is neither a code nor a rubric of the openEHR setting group, so it names no setting a composition can carry",
			ErrUnsupportedDatatype, key)
	}
	if err := putCtx(ctx, origin, "ctx/setting|code", code, key); err != nil {
		return err
	}
	return putCtx(ctx, origin, "ctx/setting|value", rubric, key)
}

// settingMember finds s in the openEHR setting group, as a code first and then
// as a rubric, and returns that member's code and rubric. member is false when
// s is neither.
func settingMember(s string) (code, rubric string, member bool) {
	if r, isCode := terminology.Setting.Rubric(s); isCode {
		return s, r, true
	}
	code, member = terminology.Setting.Code(s)
	return code, s, member
}

// putCtx records a context value under its normalised ctx/ key, tracking in
// origin the body key it arrived under. The admissibility check is total — every
// value shape is either stored, refused as a shape no ctx/ field can hold, or
// compared against what an earlier spelling already gave:
//
//   - A composite (JSON object or array) is refused outright. No supported ctx/
//     field holds one, and leaving it to the comparison below is what let a
//     malformed payload through before: a composite could not be compared, so the
//     scalar spelling beside it silently overwrote it whenever the sorted key
//     order put the composite first (PR #86 review round 3).
//   - Anything else is compared as canonical JSON bytes, which stays total across
//     kinds — a string against a number, a bool against null — where a type switch
//     on one side alone has to give up on the pairs it does not enumerate. Numbers
//     keep their exact lexical form (json.Number marshals verbatim), so "1" and
//     "1.0" are a conflict rather than being rounded into agreement.
//
// A comparison, never a precedence rule: silently preferring either spelling
// would corrupt composition metadata (ADR 0015, decision 4).
func putCtx(ctx map[string]any, origin map[string]string, ctxKey string, val any, bodyKey string) error {
	switch val.(type) {
	case map[string]any, []any:
		return fmt.Errorf("%w: %s must be a scalar value, got %T (from %s)",
			ErrUnsupportedDatatype, ctxKey, val, bodyKey)
	}
	if prev, seen := ctx[ctxKey]; seen && !sameCtxValue(prev, val) {
		return fmt.Errorf("%w: conflicting spellings of %s: %q and %q give different values; remove one",
			ErrUnknownPath, ctxKey, ctxSpelling(origin, ctxKey), bodyKey)
	}
	ctx[ctxKey] = val
	origin[ctxKey] = bodyKey
	return nil
}

// sameCtxValue reports whether two candidate values for one ctx/ key are the same
// value, comparing their canonical JSON encodings byte for byte. `==` on `any` is
// not an option: it panics for the JSON object or array a malformed payload can
// carry, and this codec must not panic on untrusted input (REQ-025 — no panics).
// Marshalling keeps the comparison independent of the caller's own shape checks.
//
// A value that will not marshal counts as different: an incomparable pair is not
// a proven agreement, and refusing it is the safe outcome.
func sameCtxValue(a, b any) bool {
	ab, err := json.Marshal(a)
	if err != nil {
		return false
	}
	bb, err := json.Marshal(b)
	if err != nil {
		return false
	}
	return bytes.Equal(ab, bb)
}

// ctxSpelling names the body key a ctx/ value arrived under. [siphonContext] always
// records one; the fallback keeps an error legible for a ctx map assembled some
// other way (in-package callers, tests).
func ctxSpelling(origin map[string]string, ctxKey string) string {
	return cmp.Or(origin[ctxKey], ctxKey)
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
	// setting carries the ctx/setting|code + |value pair (REQ-053). parseCtx
	// completes half of it from the openEHR setting group or refuses it, so
	// applyContext keys on haveSettingCode alone.
	settingCode, settingValue         string
	haveSettingCode, haveSettingValue bool
	// category is the composition category the template fixes
	// ([templateIndex.category]), which [defaultAttr] gives a body that spells
	// none. It is the zero codedName where the template fixes none, and in a
	// decode without the template.
	category codedName
	// timeBuildsNoContext is set where time is the default start time
	// ([templateIndex.startTime]) of a persistent composition
	// ([persistentComposition]): it fills the times the body leaves out, and
	// is the start time of a context the body builds, but builds none itself.
	timeBuildsNoContext bool
}

// parseCtx decodes the ctx/ entries. Only the core context fields reach it:
// [siphonContext] sets every other ctx/ field aside as unmatched content
// ([ctxFields]), so the default arm is a broken invariant. Values of the
// wrong JSON type are rejected — coercing them would silently corrupt
// composition metadata (e.g. a numeric composer_name becoming an empty
// PARTY_IDENTIFIED name). origin (from [siphonContext]) names the body key each
// value arrived under, so an error points at the payload's own spelling.
//
// Keys are walked in sorted order: a body with two bad ctx/ fields must name the
// same one on every run.
func parseCtx(ctx map[string]any, origin map[string]string) (ctxInfo, error) {
	var ci ctxInfo
	var err error
	for _, key := range slices.Sorted(maps.Keys(ctx)) {
		val, label := ctx[key], ctxSpelling(origin, key)
		switch strings.TrimPrefix(key, "ctx/") {
		case "language":
			ci.language, err = ctxString(label, val)
		case "territory":
			ci.territory, err = ctxString(label, val)
		case "composer_name":
			ci.composerName, err = ctxString(label, val)
			ci.haveComposerName = true
			// `Name_valid` — an empty name, in either spelling, is not a composer
			// without one: alone it would decode to a PARTY_IDENTIFIED the RM
			// forbids and encode refuses. A null one never gets here; it is
			// absent (REQ-053).
			if err == nil && ci.composerName == "" {
				err = fmt.Errorf("%w: %q is present but empty (RM invariant `Name_valid`); a composer carrying no name is spelled by the key's absence",
					ErrUnsupportedDatatype, label)
			}
		case "composer_self":
			b, ok := val.(bool)
			if !ok {
				err = fmt.Errorf("%w: %s must be a boolean, got %T", ErrUnsupportedDatatype, label, val)
			}
			ci.composerSelf = b
		case "time":
			ci.time, err = ctxString(label, val)
			ci.haveTime = true
		case "setting|code":
			ci.settingCode, err = ctxString(label, val)
			ci.haveSettingCode = true
		case "setting|value":
			ci.settingValue, err = ctxString(label, val)
			ci.haveSettingValue = true
		default:
			err = fmt.Errorf("simplified: %q reached the ctx/ parser, which reads only the fields in ctxFields", key)
		}
		if err != nil {
			return ci, err
		}
	}
	// Half of the ctx/setting pair, in either spelling, is completed from the
	// openEHR setting group, as a bare ctx/setting is ([putBareSetting]) —
	// REQ-053: |code alone takes its member's rubric, and |value alone its
	// member's code. A |code is read as a code only and a |value as a rubric
	// only. Completion runs here, once every spelling has met in [putCtx], so a
	// half beside another spelling naming another member was already refused
	// there as a disagreement. A half the group does not complete (a code outside
	// it, a value that is no rubric of it, an empty half) is refused naming the
	// missing key, the shape requireSuffix enforces for a leaf's mandatory
	// suffixes.
	if ci.haveSettingCode != ci.haveSettingValue {
		present, missing := "ctx/setting|code", "ctx/setting|value"
		if ci.haveSettingCode {
			ci.settingValue, ci.haveSettingValue = terminology.Setting.Rubric(ci.settingCode)
		} else {
			present, missing = "ctx/setting|value", "ctx/setting|code"
			ci.settingCode, ci.haveSettingCode = terminology.Setting.Code(ci.settingValue)
		}
		// Both halves are named in the spelling the payload used: telling a
		// real-path author to add a `ctx/`-spelled key they never wrote sends them
		// to the wrong place. ctxSpelling falls back to the ctx/ form when the
		// missing key has no origin, which is exactly right — it was not written.
		if !ci.haveSettingCode || !ci.haveSettingValue {
			return ci, fmt.Errorf("%w: %s without %s, and the openEHR setting group has no member to complete it from",
				ErrUnsupportedDatatype, ctxSpelling(origin, present), ctxSpelling(origin, missing))
		}
	}
	// Neither half may be empty. An empty code would rebuild a CODE_PHRASE with no
	// code_string — RM-invalid, and refused by emitContextSetting on the way back
	// out, so accepting it here would mint a composition this codec cannot
	// re-encode (REQ-053 fail-loud). An all-empty pair is refused for the same
	// reason rather than read as "absent": absent is the key not being there, and
	// silently treating an explicit empty pair as absent would hand it the
	// WithTemplate `238|other care` default the payload plainly did not ask for.
	if ci.haveSettingCode {
		if ci.settingCode == "" {
			return ci, fmt.Errorf("%w: %s is empty — ctx/setting requires a defining code (omit the pair for an absent setting)",
				ErrUnsupportedDatatype, ctxSpelling(origin, "ctx/setting|code"))
		}
		if ci.settingValue == "" {
			return ci, fmt.Errorf("%w: %s is empty — ctx/setting requires the code's rubric (omit the pair for an absent setting)",
				ErrUnsupportedDatatype, ctxSpelling(origin, "ctx/setting|value"))
		}
	}
	// composer_self and a composer name are two mutually exclusive
	// representations of one RM attribute: PARTY_SELF carries no name, and
	// PARTY_IDENTIFIED is not the EHR subject. [decodeComposer] can only build
	// one, and it prefers PARTY_SELF — so accepting the pair would drop the
	// name silently, exactly what refusing two conflicting spellings of one field
	// prevents. `composer_self: false` beside a name is not a conflict: it denies
	// nothing the name asserts, and the encoder's own PARTY_IDENTIFIED output
	// omits composer_self entirely.
	if ci.composerSelf && ci.haveComposerName {
		return ci, fmt.Errorf("%w: conflicting composer spellings: %q makes the composer the EHR subject (PARTY_SELF), %q names one (PARTY_IDENTIFIED); remove one",
			ErrUnknownPath, ctxSpelling(origin, "ctx/composer_self"), ctxSpelling(origin, "ctx/composer_name"))
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
// the composer [decodeComposer] built (nil for none), and enforces that
// language and territory (mandatory per the Simplified Formats spec) are
// present.
func applyContext(compJSON map[string]any, ci ctxInfo, composer map[string]any) error {
	if ci.language == "" || ci.territory == "" {
		return fmt.Errorf("%w: ctx/language and ctx/territory are required", ErrMissingContext)
	}
	compJSON["language"] = codePhraseJSON(ci.language, "ISO_639-1")
	compJSON["territory"] = codePhraseJSON(ci.territory, "ISO_3166-1")
	if composer != nil {
		compJSON["composer"] = composer
	}
	// Merge into any EVENT_CONTEXT already reconstructed from clinical paths
	// (other_context, …) rather than replacing it — otherwise that data would
	// be lost. A default start time on a persistent composition builds none,
	// but is the start time of one the body builds (REQ-053).
	ctxObj, _ := compJSON["context"].(map[string]any)
	if ctxObj == nil && ((ci.haveTime && !ci.timeBuildsNoContext) || ci.haveSettingCode) {
		ctxObj = map[string]any{"_type": "EVENT_CONTEXT"}
		compJSON["context"] = ctxObj
	}
	if ctxObj == nil {
		return nil
	}
	if ci.haveTime {
		ctxObj["start_time"] = map[string]any{"_type": "DV_DATE_TIME", "value": ci.time}
	}
	if ci.haveSettingCode {
		// parseCtx enforced the pair, and the terminology is the implied
		// openehr (a real-path |terminology witness naming anything else was
		// already refused in siphonContext) — REQ-053.
		ctxObj["setting"] = map[string]any{
			"_type": "DV_CODED_TEXT", "value": ci.settingValue,
			"defining_code": codePhraseJSON(ci.settingCode, terminology.ID),
		}
	}
	return nil
}

// persistentComposition reports whether compJSON, with ci, is a persistent
// composition: whether its category, the one the body spells or else the one
// [defaultAttr] gives it, is 431 `persistent` in the openehr terminology.
func persistentComposition(compJSON map[string]any, ci ctxInfo) bool {
	category, spelled := compJSON["category"].(map[string]any)
	if !spelled {
		category = defaultAttr("category", ci)
	}
	dc, _ := category["defining_code"].(map[string]any)
	tid, _ := dc["terminology_id"].(map[string]any)
	return dc["code_string"] == "431" && tid["value"] == terminology.ID
}

// completeRequired fills the RM-mandatory attributes the FLAT/STRUCTURED formats
// do not carry — ENTRY language/encoding/subject, HISTORY.origin, EVENT.time,
// EVENT_CONTEXT.setting, and any others rminfo reports — with ctx defaults + RM
// conventions, so a WithTemplate decode yields an OPT-validatable composition.
// It only runs in conformant (WithTemplate) mode. The values it synthesises
// (event times, subject, setting) are defaults, not recovered data — the formats
// never carried them; see deviations.md. A HISTORY's origin and its events'
// times follow [historyTimes].
func completeRequired(node map[string]any, ci ctxInfo) {
	if t, _ := node["_type"].(string); t != "" {
		// rminfo knows each class by its bare BMM name; a caller's |raw fragment
		// may spell a generic instantiation (DV_INTERVAL<DV_QUANTITY>).
		class := bmmtype.Class(t)
		if class == "HISTORY" {
			ci = historyTimes(node, ci)
		}
		for _, attr := range rminfo.Default.RequiredAttributes(class) {
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

// historyTimes gives history, a HISTORY the body gives no origin, the origin
// the reference implementation gives it (REQ-053): the earliest time its own
// events carry ([earliestEventTime]), else the start time in ci, else none.
// It returns ci with the HISTORY's origin as its time, where it has one, so
// each event below it that the body gives no time takes that origin, as the
// Simplified Formats say (§ time: "EVENT.time will be set to history.origin").
// An origin the HISTORY already holds is kept, and its events take it too.
func historyTimes(history map[string]any, ci ctxInfo) ctxInfo {
	if _, has := history["origin"]; !has {
		if earliest, ok := earliestEventTime(history); ok {
			history["origin"] = map[string]any{"_type": "DV_DATE_TIME", "value": earliest}
		} else if ci.haveTime {
			history["origin"] = map[string]any{"_type": "DV_DATE_TIME", "value": ci.time}
		}
	}
	origin, _ := history["origin"].(map[string]any)
	if value, _ := origin["value"].(string); value != "" {
		ci.time, ci.haveTime = value, true
	}
	return ci
}

// earliestEventTime returns, as written, the earliest time the events of
// history carry, and whether any carries one. Times are compared as instants
// ([eventInstant]); a time that does not parse takes no part. Of two times
// that name one instant, the first event's is returned.
func earliestEventTime(history map[string]any) (string, bool) {
	events, _ := history["events"].([]any)
	var earliest string
	var least float64
	found := false
	for _, e := range events {
		event, _ := e.(map[string]any)
		at, _ := event["time"].(map[string]any)
		value, _ := at["value"].(string)
		instant, ok := eventInstant(value)
		if ok && (!found || instant < least) {
			earliest, least, found = value, instant, true
		}
	}
	return earliest, found
}

// eventInstant returns the instant the ISO 8601 date-time s names, in seconds
// from 0001-01-01T00:00:00 UTC, and whether s parses as one ([iso8601]). A
// time with a zone is its clock less the zone's offset from UTC, and one with
// no zone is read in UTC, as the reference implementation reads it
// (DV_DATE_TIME magnitude). A partial value stands for the start of the period
// it names.
func eventInstant(s string) (float64, bool) {
	d, t, err := iso8601.ParseDateTime(s)
	if err != nil {
		return 0, false
	}
	magnitude := iso8601.DateTimeMagnitude(d, t)
	if t.Zone == "" {
		return magnitude, true
	}
	return iso8601.UTCMagnitude(magnitude, t.Zone)
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
		return ctxCodedDefault(terminology.Setting, "238")
	case "category":
		// The category the template fixes, else 433 `event`.
		if ci.category != (codedName{}) {
			return ci.category.json()
		}
		return ctxCodedDefault(terminology.CompositionCategory, "433")
	case "math_function":
		// 640 is `actual` — the earlier 146|actual pair was inconsistent (146 is
		// `mean`), so the code is corrected to match the evidently intended rubric.
		return ctxCodedDefault(terminology.EventMathFunction, "640")
	case "width":
		return map[string]any{"_type": "DV_DURATION", "value": "PT0S"}
	}
	return nil
}

// ctxCodedDefault builds the DV_CODED_TEXT for a ctx/ default from the pinned
// group, so the value is the code's own rubric and never a string typed beside
// the code (REQ-034). The codes are string literals at the call sites above and
// members of their groups — TestCtxDefaultsAreGroupMembersWithPinnedRubrics
// pins that.
func ctxCodedDefault(g *terminology.Group, code string) map[string]any {
	rubric, _ := g.Rubric(code)
	return map[string]any{
		"_type": "DV_CODED_TEXT", "value": rubric,
		"defining_code": codePhraseJSON(code, terminology.ID),
	}
}

// codePhraseJSON is a canonical CODE_PHRASE object. The second argument is the
// TERMINOLOGY_ID value — [terminology.ID] for every openEHR-coded site.
func codePhraseJSON(code, terminologyID string) map[string]any {
	return map[string]any{
		"_type":          "CODE_PHRASE",
		"code_string":    code,
		"terminology_id": map[string]any{"_type": "TERMINOLOGY_ID", "value": terminologyID},
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
// **bare** spelling of the ancestor's AQLPath (REQ-111 name predicates
// stripped), because placeLeaf rebuilds those lookup prefixes bare. Keying
// by the predicated spelling made every lookup under a name-pinned container
// miss, so concreteType fell back to a default RM type — wrong _type
// embedded silently where it resolved, decode failure where it did not.
// Keying by bare node id alone would be worse still: the same at-code (or a
// self-nested archetype id) appearing twice along one path would silently
// apply one segment's index or type to another; the bare path prefix stays
// unique per chain position. Returns errSegNotFound when a segment id is not
// found, and refuses a key through a reused sibling that no pinned name tells
// apart from the others (sib).
func resolveLeaf(wt *webtemplate.WebTemplate, segs []flatSeg, sib *reusedSiblings) (*webtemplate.Node, map[string]int, map[string]string, error) {
	predIndex := make(map[string]int)
	predType := make(map[string]string)
	node := wt.Tree
	if len(segs) == 0 || segs[0].id != node.ID {
		return nil, nil, nil, errSegNotFound
	}
	for i, seg := range segs[1:] {
		var next *webtemplate.Node
		for _, ch := range node.Children {
			if ch.ID == seg.id {
				next = ch
				break
			}
		}
		if next == nil {
			// The segment may name an attribute the codec reads where the Web
			// Template has no node ([codecLeaf]). Each is single-valued, so an
			// index past :0 on it is past its bound whatever follows it, and it
			// is a leaf, so a segment below it resolves nothing. An
			// INTERVAL_EVENT attribute under an abstract EVENT makes the event
			// an INTERVAL_EVENT (REQ-053).
			leaf := codecLeaf(node, seg.id)
			switch {
			case leaf == nil:
				return nil, nil, nil, errSegNotFound
			case seg.idx > 0:
				return nil, nil, nil, errSingleValuedIndex(seg)
			case i != len(segs)-2:
				return nil, nil, nil, errSegNotFound
			}
			if intervalEventLeaf(node, leaf) {
				predType[bareAQLPath(node.AQLPath)] = "INTERVAL_EVENT"
			}
			return leaf, predIndex, predType, nil
		}
		// Placement reads an index only where the attribute is a list, so on a
		// single-valued one an index past :0 would be dropped and the key merged
		// into the one value (REQ-170).
		if seg.idx > 0 && holdsOneValue(node, next) {
			return nil, nil, nil, errSingleValuedIndex(seg)
		}
		if sib.isShared(next) {
			// Reused siblings (REQ-111) that no pinned name tells apart share
			// this resolution path, and placement walks it: distinct
			// siblings' instances would collapse onto one list slot — a
			// silent merge of one sibling's data into another. Refuse rather
			// than corrupt; see deviations.md § Conformance.
			return nil, nil, nil, fmt.Errorf("%w: FLAT id %q reaches one of several reused siblings sharing the path %q — not yet decodable (see deviations.md)", ErrUnknownPath, seg.id, sib.nodePath(next))
		}
		if next.NodeID != "" {
			bare := bareAQLPath(next.AQLPath)
			predType[bare] = nodeRMType(next)
			if seg.idx >= 0 {
				predIndex[bare] = seg.idx
				// A collapsed leaf's index belongs to the ELEMENT the Web
				// Template folded away, one attribute up: placement keys on
				// the *container* segment's path, so an index stored only
				// under the leaf's own `…/value` spelling is never read and
				// every instance lands on list slot 0 — a duplicate placement
				// that refuses the second one (REQ-140). A template that does
				// carry a node for the owner itself has already recorded its
				// own index there, and that one wins.
				if owner, collapsed := strings.CutSuffix(bare, "/value"); collapsed && owner != "" {
					if _, taken := predIndex[owner]; !taken {
						predIndex[owner] = seg.idx
					}
				}
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

// aqlSeg is one canonical-path segment: an attribute name, an optional node
// predicate (archetype id or at-code) and an optional REQ-111 name.
type aqlSeg struct {
	attr string
	pred string
	// name is the name the segment's predicate carries, its escapes undone,
	// and named reports whether it carries one.
	name  string
	named bool
	// end is the offset in the parsed path just past the segment.
	end int
}

// parseAQL splits a canonical aqlPath into attribute+predicate segments. The
// predicate is taken as a bare node id (archetype id or at-code), and a
// REQ-111 name predicate ([at0001,'Reaction details']) goes to the name
// apart from it, which is both a correctness and a parsing requirement:
//
//   - archetype_node_id must carry the bare id — a name predicate constrains
//     the node's name, it does not rename the node.
//   - a pinned name may itself contain '/' or ',' — the corona oracle pins
//     one of each — which a plain split on '/' would tear apart.
//
// Placement reads a name only where the path it walks keeps one: the
// resolution path of [reusedSiblings], which keeps a name only where it tells
// reused siblings apart. Compound predicates (e.g. [at0001 and
// name/value='x']) are still not split — no supported Web Template emits them
// (mirror rmpath.parsePredicate if that changes).
func parseAQL(p string) []aqlSeg {
	return slices.Collect(aqlSegments(p))
}

// aqlSegments yields the segments [parseAQL] returns, one at a time, for a
// caller that reads each once.
func aqlSegments(p string) iter.Seq[aqlSeg] {
	return func(yield func(aqlSeg) bool) {
		for raw := range rawSegments(p) {
			seg := aqlSeg{attr: p[raw.start+1 : raw.end], end: raw.end}
			if raw.bracketed {
				open := raw.start + strings.IndexByte(p[raw.start:raw.idEnd], '[')
				seg.attr = p[raw.start+1 : open]
				seg.pred = p[open+1 : raw.idEnd]
				seg.name, seg.named = raw.name(p), raw.named
			}
			if seg.attr == "" {
				continue
			}
			if !yield(seg) {
				return
			}
		}
	}
}

// placeLeaf walks aqlPath from compJSON, materialising the intermediate RM
// nodes (concrete type via rminfo + the Web Template, archetype_node_id from
// the predicate, list position from predIndex), and sets the terminal
// attribute to the leaf value: a DataValue, or the RM String of a STRING leaf.
//
// Reconstructed intermediate and leaf nodes carry _type + archetype_node_id;
// when tmpl is non-nil (a template was supplied via WithTemplate) the mandatory
// LOCATABLE.name is set from it, and a folded ITEM_STRUCTURE takes the concrete
// subtype it gives, both keyed by the node's compiled aqlPath — which the walk
// reconstructs from namePath by keeping a predicate only on container
// attributes (matching templatecompile's path convention). Without tmpl, nodes
// are left unnamed (rmpath re-resolves by archetype_node_id, so the round-trip
// does not depend on it — but the result is then format-idempotent, not
// canonically complete; see deviations.md).
//
// It returns the placed value's path in the spelling the [templateIndex] is
// keyed by. aqlPath is the leaf's resolution path ([reusedSiblings], sib), and
// namePath its aqlPath as the Web Template spells it, every pinned name kept.
func placeLeaf(compJSON map[string]any, aqlPath, namePath string, predIndex map[string]int, predType map[string]string, value any, budget *allocBudget, tmpl *templateIndex, sib *reusedSiblings) (string, error) {
	cur, attr, key, err := walkAQLKeyed(compJSON, aqlPath, namePath, predIndex, predType, budget, tmpl, sib)
	if err != nil {
		return "", err
	}
	if _, exists := cur[attr]; exists {
		// Two FLAT keys resolved to the same terminal slot (e.g. "a" vs
		// "a:0" on a repeatable) — overwriting would silently drop one.
		return "", fmt.Errorf("%w: duplicate placement at %q", ErrUnknownPath, aqlPath)
	}
	cur[attr] = value
	return key + "/" + attr, nil
}

// walkAQL materialises the intermediate RM nodes along aqlPath and returns the
// object that holds its **final** segment, together with that segment's
// attribute name — the seam [placeLeaf] and the REQ-140 attribute router share.
// A leaf's terminal attribute is the datatype slot (`…/items[at0008]/value`); an
// underscore family's is the RM attribute it addresses
// (`…/items[at0008]/uid`), which is also the lookahead [concreteType] needs to
// resolve the abstract ITEM_STRUCTURE slot the Web Template collapses.
//
// aqlPath is a resolution path ([reusedSiblings], sib), and namePath the same
// path as the Web Template spells it, every pinned name kept: the template
// index is read under namePath's names ([walkAQLKeyed]).
func walkAQL(compJSON map[string]any, aqlPath, namePath string, predIndex map[string]int, predType map[string]string, budget *allocBudget, tmpl *templateIndex, sib *reusedSiblings) (map[string]any, string, error) {
	holder, attr, _, err := walkAQLKeyed(compJSON, aqlPath, namePath, predIndex, predType, budget, tmpl, sib)
	return holder, attr, err
}

// walkAQLKeyed is [walkAQL] that also returns the holder's path in the
// spelling the [templateIndex] is keyed by, so a caller can look up what the
// template gives the value it places there.
func walkAQLKeyed(compJSON map[string]any, aqlPath, namePath string, predIndex map[string]int, predType map[string]string, budget *allocBudget, tmpl *templateIndex, sib *reusedSiblings) (map[string]any, string, string, error) {
	segs := parseAQL(aqlPath)
	if len(segs) == 0 {
		return nil, "", "", fmt.Errorf("%w: empty canonical path", ErrUnknownPath)
	}
	// named holds the segments whose names key the template index: those of
	// namePath, which keeps the names aqlPath drops. Below a reused sibling,
	// a node can pin a name its namesake below the other sibling does not
	// pin, so the bare alias the index answers a dropped name with would give
	// it the rubric the two share. Only a lookup with the template reads them.
	named := segs
	if tmpl != nil && namePath != aqlPath {
		if s := parseAQL(namePath); len(s) == len(segs) {
			named = s
		}
	}
	cur := compJSON
	curType := "COMPOSITION"
	// Two path keys are rebuilt in lockstep: aqlPrefix carries a bare
	// predicate on every predicated segment — the positional key
	// predIndex/predType are stored under, bare-keyed by resolveLeaf to
	// match; namePrefix keeps a predicate only on container attributes
	// (templatecompile's convention), with the name namePath pins there, the
	// key of the WithTemplate [templateIndex], which also answers every entry
	// under its bare spelling. aqlPath keeps a name only where it tells reused
	// siblings apart (REQ-111), and there the member is chosen by that name
	// too, and created with it; where a reused sibling pins no name among
	// siblings that pin one, the member is chosen among those carrying none of
	// their names (sib).
	var aqlPrefix, namePrefix strings.Builder
	for i, seg := range segs {
		if i == len(segs)-1 {
			return cur, seg.attr, namePrefix.String(), nil
		}
		aqlPrefix.WriteString("/")
		aqlPrefix.WriteString(seg.attr)
		if seg.pred != "" {
			aqlPrefix.WriteString("[")
			aqlPrefix.WriteString(seg.pred)
			aqlPrefix.WriteString("]")
		}
		container, _ := rminfo.Default.IsContainer(curType, seg.attr)
		namePrefix.WriteString("/")
		namePrefix.WriteString(seg.attr)
		if container && seg.pred != "" {
			namePrefix.WriteString("[")
			namePrefix.WriteString(tcimpl.NamePredicate(seg.pred, named[i].name))
			namePrefix.WriteString("]")
		}
		// A reused sibling's member carries the sibling's name: the coded name
		// the template determines there, else the pinned text. A sibling that
		// pins none among siblings that pin one owns no member that carries
		// one of their names.
		var pinned map[string]any
		if seg.named {
			pinned = tmpl.nameJSON(namePrefix.String())
			if v, _ := pinned["value"].(string); v != seg.name {
				pinned = textJSON(seg.name)
			}
		}
		excluded := sib.excludedAt(aqlPath[:seg.end])
		wtType := predType[aqlPrefix.String()]
		nextAttr := segs[i+1].attr
		childType := concreteType(curType, seg.attr, wtType, tmpl.rmType(namePrefix.String()), nextAttr)
		if childType == "" {
			return nil, "", "", fmt.Errorf("%w: cannot resolve RM type for %q on %s (aqlPath %q)", ErrUnknownPath, seg.attr, curType, aqlPath)
		}
		if container {
			next, err := selectElem(cur, seg.attr, childType, seg.pred, pinned, excluded, predIndex[aqlPrefix.String()], budget)
			if err != nil {
				return nil, "", "", err
			}
			if childType == "INTERVAL_EVENT" && next["_type"] == "POINT_EVENT" {
				// An event an earlier key built under an abstract EVENT node is
				// an INTERVAL_EVENT once a key spells one of its attributes
				// ([codecLeaf]); every POINT_EVENT attribute is one of its too.
				next["_type"] = childType
			}
			cur = next
		} else {
			obj, ok := cur[seg.attr].(map[string]any)
			if held, present := cur[seg.attr]; present && !ok {
				// The slot already holds a scalar (a STRING leaf, a bare
				// attribute): a second key reaching the same single-valued
				// attribute through a deeper node would replace it with an
				// object and drop the first value (REQ-053).
				return nil, "", "", fmt.Errorf("%w: %s.%s already holds a %T, but keys reach it through a second Web Template node — refused rather than overwritten (aqlPath %q)",
					ErrUnknownPath, curType, seg.attr, held, aqlPath)
			}
			if !ok {
				obj = map[string]any{"_type": childType}
				// The node id is recorded on a non-LOCATABLE ISM_TRANSITION too,
				// so the check below can tell its nodes apart while leaves are
				// placed; settleIsmTransitions removes it before canjson.
				if seg.pred != "" {
					obj["archetype_node_id"] = seg.pred
				}
				if childType == "ISM_TRANSITION" {
					// So is the resolution path of the Web Template node the
					// transition is placed under, which settleIsmTransitions
					// settles it against and removes: reused ACTION siblings
					// (REQ-111) have ISM transition nodes whose paths differ by
					// the siblings' names alone, which no node id carries.
					obj[ismNodeKey] = aqlPath[:seg.end]
				}
				if pinned != nil {
					obj["name"] = pinned
				}
				cur[seg.attr] = obj
			} else if held, _ := obj["archetype_node_id"].(string); held != seg.pred || (pinned != nil && nameValue(obj) != seg.name) ||
				(pinned == nil && slices.Contains(excluded, nameValue(obj))) {
				// Two Web Template nodes stand for this one single-valued
				// attribute (two of an ACTION's ism_transition nodes, such as
				// [at0005] and [at0006], or the in-context one and a per-step
				// one; or two reused siblings, REQ-111: each pinning its name,
				// or one pinning none beside one whose name the object carries).
				// Merging their keys into one object would re-encode them under
				// one node only, or decode one body's two spellings of the
				// value as one (REQ-053).
				want := seg.pred
				if pinned != nil || len(excluded) > 0 {
					held, want = tcimpl.NamePredicate(held, nameValue(obj)), tcimpl.NamePredicate(seg.pred, seg.name)
				}
				return nil, "", "", fmt.Errorf("%w: %s.%s is single-valued, but keys reach it through two Web Template nodes, [%s] and [%s] — refused rather than merged (aqlPath %q)",
					ErrUnknownPath, curType, seg.attr, held, want, aqlPath)
			}
			cur = obj
		}
		// Only a LOCATABLE has a name. The template's name index also covers an
		// archetyped ISM_TRANSITION node, which must not gain one. Nor does a
		// member of a reused sibling that pins no name gain one of the names
		// its siblings pin: encode would read it back as theirs.
		if _, has := cur["name"]; !has {
			if name := tmpl.nameJSON(namePrefix.String()); name != nil && nameable(childType) && !namedOneOf(name, excluded) {
				cur["name"] = name
			}
		}
		curType = childType
	}
	// Unreachable: the loop returns at i == len(segs)-1 and segs is non-empty.
	return nil, "", "", fmt.Errorf("%w: canonical path %q walked past its last segment", ErrUnknownPath, aqlPath)
}

// nameable reports whether a rebuilt node of this RM type declares a `name`,
// so decode never adds a member the RM class does not have.
func nameable(rmType string) bool {
	_, declared := rminfo.Default.AttributeRMType(bmmtype.Class(rmType), "name")
	return declared
}

// ismNode is an archetyped `ism_transition[atNNNN]` Web Template node: its node
// id, its name, and its FLAT path without indexes (for messages).
type ismNode struct {
	id, name, flat string
}

// mergedIsmNode is the in-context `ism_transition` node the Web Template gives
// an ACTION whose template models archetyped per-step nodes, which it merges:
// the careflow steps its careflow_step input lists, each with its label and
// the current states that step admits, and its FLAT path without indexes (for
// messages).
type mergedIsmNode struct {
	steps []webtemplate.InputListItem
	flat  string
}

// ismNodes are the Web Template's ISM_TRANSITION nodes [settleIsmTransitions]
// settles a rebuilt transition at, each indexed by its resolution path
// ([reusedSiblings]), which [walkAQL] records on the transition it places
// under the node ([ismNodeKey]).
type ismNodes struct {
	// archetyped are the per-step `ism_transition[atNNNN]` nodes.
	archetyped map[string]ismNode
	// merged are the in-context `ism_transition` nodes that merge per-step ones.
	merged map[string]mergedIsmNode
}

// ismTransitionNodes indexes the Web Template's ISM_TRANSITION nodes
// ([ismNodes]). found reports whether it has an ISM_TRANSITION node of any
// kind, an in-context `ism_transition` node that merges no archetyped one
// included.
func ismTransitionNodes(wt *webtemplate.WebTemplate, sib *reusedSiblings) (nodes ismNodes, found bool) {
	nodes = ismNodes{archetyped: make(map[string]ismNode), merged: make(map[string]mergedIsmNode)}
	var walk func(n *webtemplate.Node, flat string)
	walk = func(n *webtemplate.Node, flat string) {
		for _, ch := range n.Children {
			chFlat := flat + "/" + ch.ID
			if nodeRMType(ch) == "ISM_TRANSITION" {
				found = true
				switch step := childByID(ch, careflowStepLeaf); {
				case ch.NodeID != "":
					nodes.archetyped[sib.nodePath(ch)] = ismNode{id: ch.NodeID, name: ch.Name, flat: chFlat}
				case step != nil:
					nodes.merged[sib.nodePath(ch)] = mergedIsmNode{steps: codeInputList(step), flat: chFlat}
				}
			}
			walk(ch, chFlat)
		}
	}
	if wt.Tree != nil {
		walk(wt.Tree, wt.Tree.ID)
	}
	return nodes, found
}

// codeInputList returns the list of a leaf's `code` input, nil where it has
// none.
func codeInputList(leaf *webtemplate.Node) []webtemplate.InputListItem {
	for _, in := range leaf.Inputs {
		if in.Suffix == "code" {
			return in.List
		}
	}
	return nil
}

// settleIsmTransitions finishes each ISM_TRANSITION the placement walk rebuilt,
// once every key is placed.
//
// At every kind of node, a current state and a transition that name no
// terminology take `openehr` ([impliedOpenehrTerminology]). The rest depends
// on the node the transition was spelled under:
//
//   - at an archetyped per-step node, the careflow step is the node's
//     ([settleIsmTransition]);
//   - at the in-context node that merges per-step ones, the careflow step is
//     completed from the steps the node lists ([settleMergedIsmTransition]);
//   - at an in-context node that merges no archetyped one, nothing more: the
//     body cannot spell a careflow step there.
//
// A transition is settled against the node [walkAQL] placed it under, by the
// resolution path it recorded there ([ismNodeKey]), so reused ACTION siblings
// (REQ-111) each settle against their own node. That record and the node id
// [walkAQL] recorded on a per-step transition are removed, so neither reaches
// canjson.
func settleIsmTransitions(compJSON map[string]any, wt *webtemplate.WebTemplate, sib *reusedSiblings) error {
	nodes, found := ismTransitionNodes(wt, sib)
	if !found {
		return nil
	}
	return settleIsmTransitionsUnder(compJSON, nodes)
}

// ismNodeKey is the member [walkAQL] records on an ISM_TRANSITION it places,
// holding the resolution path of the Web Template node it places it under.
// It is no RM member: [settleIsmTransitions] removes it.
const ismNodeKey = "_ism_transition_node"

// settleIsmTransitionsUnder walks obj in sorted attribute order, so a body with
// two bad transitions names the same one on every run.
func settleIsmTransitionsUnder(obj map[string]any, nodes ismNodes) error {
	for _, attr := range slices.Sorted(maps.Keys(obj)) {
		switch v := obj[attr].(type) {
		case map[string]any:
			if v["_type"] != "ISM_TRANSITION" {
				if err := settleIsmTransitionsUnder(v, nodes); err != nil {
					return err
				}
				continue
			}
			at, _ := v[ismNodeKey].(string)
			delete(v, ismNodeKey)
			impliedOpenehrTerminology(v)
			if n, ok := nodes.archetyped[at]; ok {
				if err := settleIsmTransition(v, n); err != nil {
					return err
				}
			} else if m, ok := nodes.merged[at]; ok {
				if err := settleMergedIsmTransition(v, m); err != nil {
					return err
				}
			}
		case []any:
			for _, e := range v {
				if m, ok := e.(map[string]any); ok {
					if err := settleIsmTransitionsUnder(m, nodes); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

// impliedOpenehrTerminology gives a rebuilt ISM_TRANSITION's current state and
// transition the `openehr` terminology where they name none (no
// terminology_id, or a null one). RM `Current_state_valid` and
// `Transition_valid` admit a code of the openEHR `instruction states` and
// `instruction transitions` groups alone, so no other terminology can be
// meant; it is the footing `ctx/setting` has in EVENT_CONTEXT `Setting_valid`
// (REQ-053). A terminology the body names is kept as written, for validation
// to check.
func impliedOpenehrTerminology(tr map[string]any) {
	for _, attr := range []string{"current_state", "transition"} {
		if dc := definingCodeOf(tr[attr]); dc != nil && dc["terminology_id"] == nil {
			dc["terminology_id"] = map[string]any{"_type": "TERMINOLOGY_ID", "value": terminology.ID}
		}
	}
}

// settleIsmTransition gives one rebuilt ISM_TRANSITION spelled at the per-step
// node n the careflow step n names, or checks the one the body carries. The
// node id is an at-code, which belongs to the archetype's `local` terminology:
// a careflow step that names no terminology (no terminology_id, a null one, or
// a blank value) takes it. One coded with another code, in any other
// terminology, or whose terminology_id has another shape, names another node
// than the one it is spelled under, and is refused.
func settleIsmTransition(tr map[string]any, n ismNode) error {
	delete(tr, "archetype_node_id")
	step, carried := tr["careflow_step"]
	if !carried {
		tr["careflow_step"] = map[string]any{
			"_type": "DV_CODED_TEXT", "value": n.name,
			"defining_code": codePhraseJSON(n.id, localTerminology),
		}
		return nil
	}
	dc := definingCodeOf(step)
	code, _ := dc["code_string"].(string)
	if code != n.id {
		return fmt.Errorf("%w: %s/careflow_step is not coded with its node's id %s; a careflow step coded otherwise names another node",
			ErrUnsupportedDatatype, n.flat, n.id)
	}
	switch tid := dc["terminology_id"].(type) {
	case nil:
		dc["terminology_id"] = map[string]any{"_type": "TERMINOLOGY_ID", "value": localTerminology}
	case map[string]any:
		raw := tid["value"]
		term, isString := raw.(string)
		switch {
		case raw == nil || (isString && term == ""):
			tid["value"] = localTerminology
		case !isString:
			return fmt.Errorf("%w: %s/careflow_step carries a terminology_id value of JSON type %T, not a string",
				ErrUnsupportedDatatype, n.flat, raw)
		case term != localTerminology:
			return fmt.Errorf("%w: %s/careflow_step is coded in a terminology other than %s, but its node id %s is an at-code of the %s terminology",
				ErrUnsupportedDatatype, n.flat, localTerminology, n.id, localTerminology)
		}
	default:
		return fmt.Errorf("%w: %s/careflow_step carries a terminology_id of JSON type %T, not a TERMINOLOGY_ID object",
			ErrUnsupportedDatatype, n.flat, tid)
	}
	return nil
}

// localTerminology is the terminology an archetype's own at-codes belong to.
const localTerminology = "local"

// settleMergedIsmTransition completes one rebuilt ISM_TRANSITION spelled at the
// in-context node m, which merges an ACTION's per-step nodes.
//
// A careflow step the body carries is kept whatever its code, for validation to
// judge: encode writes every transition at this node, so a transition never
// moves to another node (REQ-053). A careflow step that names no terminology
// (no terminology_id, a null one, or a blank value) takes `local`, the
// terminology of the at-codes m lists. One with no value takes the label m
// lists for its code where it is coded in `local`; any other is refused as a
// missing |value, as a coded value the template does not complete is.
//
// A transition the body gives no careflow step gets the one step of m whose
// current states hold its current state's openehr code, as a `local` code with
// the step's label as its value. Where no step, or more than one, holds it,
// the transition keeps no careflow step: decode invents none.
func settleMergedIsmTransition(tr map[string]any, m mergedIsmNode) error {
	raw, carried := tr["careflow_step"]
	if !carried {
		if s, ok := m.stepFor(tr["current_state"]); ok {
			tr["careflow_step"] = map[string]any{
				"_type": "DV_CODED_TEXT", "value": s.Label,
				"defining_code": codePhraseJSON(s.Value, localTerminology),
			}
		}
		return nil
	}
	step, _ := raw.(map[string]any)
	dc := definingCodeOf(step)
	if dc == nil {
		// A |raw value with no defining code: canonical JSON judges its shape.
		return nil
	}
	switch tid := dc["terminology_id"].(type) {
	case nil:
		dc["terminology_id"] = map[string]any{"_type": "TERMINOLOGY_ID", "value": localTerminology}
	case map[string]any:
		if v, isString := tid["value"].(string); tid["value"] == nil || (isString && v == "") {
			tid["value"] = localTerminology
		}
	}
	if _, valued := step["value"]; valued {
		return nil
	}
	tid, _ := dc["terminology_id"].(map[string]any)
	code, _ := dc["code_string"].(string)
	label := ""
	if tid["value"] == localTerminology {
		label = m.label(code)
	}
	if label == "" {
		return fmt.Errorf("simplified: decode %q: %w", m.flat+"/"+careflowStepLeaf, errMissingSuffix("DV_CODED_TEXT", "value"))
	}
	step["value"] = label
	return nil
}

// label returns the label m lists for the careflow-step code, or "".
func (m mergedIsmNode) label(code string) string {
	for _, s := range m.steps {
		if s.Value == code {
			return s.Label
		}
	}
	return ""
}

// stepFor returns the one careflow step of m whose current states hold the
// code of state, a rebuilt current state coded in openehr, and whether there
// is exactly one, with a label.
func (m mergedIsmNode) stepFor(state any) (webtemplate.InputListItem, bool) {
	dc := definingCodeOf(state)
	tid, _ := dc["terminology_id"].(map[string]any)
	code, _ := dc["code_string"].(string)
	if code == "" || tid["value"] != terminology.ID {
		return webtemplate.InputListItem{}, false
	}
	var found []webtemplate.InputListItem
	for _, s := range m.steps {
		if slices.Contains(s.CurrentStates, code) {
			found = append(found, s)
		}
	}
	if len(found) != 1 || found[0].Label == "" {
		return webtemplate.InputListItem{}, false
	}
	return found[0], true
}

// The reference's spelling of an ACTION's ISM transition and time, which the
// Web Template this SDK builds models as an ACTION's in-context `ism_transition`
// node, holding `current_state`, `transition` and, where it merges per-step
// nodes, `careflow_step`, and its `time` leaf. REQ-170 refuses, in every mode,
// a key of this spelling the Web Template does not resolve
// ([refuseInContextKeys]). ismReferenceSeg is the FLAT id the reference gives
// the ISM transition node when it writes it in context, ismReasonFamily the
// underscore family of its `reason`, careflowStepLeaf the FLAT id of its
// careflow step, and actionTimeLeaf the ACTION's time.
const (
	ismReasonFamily  = "_reason"
	ismReferenceSeg  = "ism_transition"
	careflowStepLeaf = "careflow_step"
	actionTimeLeaf   = "time"
)

// ismLeaves are the ISM_TRANSITION leaves the reference writes under its ISM
// transition node, by FLAT id.
var ismLeaves = map[string]bool{"current_state": true, "transition": true, careflowStepLeaf: true}

// refuseInContextKeys refuses, in every mode, the reference's spelling of an
// ACTION's ISM transition and time wherever the Web Template does not resolve
// it (REQ-170; REQ-140's third out-of-scope class covers the careflow step of
// an ACTION whose `ism_transition` node merges no archetyped one):
//
//   - a `current_state`, `transition` or `careflow_step` leaf, with any suffix,
//     under an ISM transition node that has no child of that id: an
//     in-context `ism_transition` node that merges no archetyped one has no
//     `careflow_step`, and an archetyped `ism_transition[atNNNN]` node no
//     `transition`, unless its archetype constrains one;
//   - any such leaf, and a `_reason` family, under an `ism_transition` segment
//     where the Web Template has no node of that id;
//   - the ACTION's own `time` leaf, with any suffix, where the Web Template has
//     no `time` node under the ACTION.
//
// Each names RM data the decode would otherwise drop, or replace with a
// default, so it is refused rather than ignored. A key the Web Template
// resolves is decoded as usual, a `_reason` family under a node the Web
// Template has is the underscore family [rmattrFamilies] carries, and any
// other unresolved key under those nodes is unmatched content. A key that
// walks through one of several reused siblings that no pinned name tells
// apart is left to the refusal that path gets.
//
// keys holds content's keys in sorted order, each parsed once
// ([parseContentKeys]), so a body with several names the same one on every run.
// The error wraps [ErrNotModelled], names the key, or the `_reason` family
// instance, and carries no value. Such a key that holds JSON null carries no
// data to drop: it is absent (REQ-053), and is removed from content.
func refuseInContextKeys(content map[string]any, keys []contentKey, wt *webtemplate.WebTemplate, sib *reusedSiblings) error {
	for _, ck := range keys {
		key := ck.key
		if ck.err != nil {
			// A malformed index is refused where the key is decoded.
			continue
		}
		named, owner, attr, ok := inContextKey(wt, key, ck.pk, sib)
		if !ok {
			continue
		}
		if content[key] == nil {
			// A null names no data to drop: the key is absent (REQ-053).
			delete(content, key)
			continue
		}
		return fmt.Errorf("%w: %q spells an %s's %s, a Simplified Formats path this SDK does not model; refused rather than ignored",
			ErrNotModelled, named, owner, attr)
	}
	return nil
}

// inContextKey reports whether key, parsed as pk, is one of the key shapes
// [refuseInContextKeys] refuses: named is what the refusal names (the key, or
// the family instance for `_reason`), owner the RM class and attr the
// attribute it spells.
func inContextKey(wt *webtemplate.WebTemplate, key string, pk parsedKey, sib *reusedSiblings) (named, owner, attr string, ok bool) {
	segs := pk.segs
	node := wt.Tree
	if len(segs) < 2 || segs[0].id != node.ID {
		return "", "", "", false
	}
	for i := 1; i < len(segs); i++ {
		child := childByID(node, segs[i].id)
		rmType := nodeRMType(node)
		last := i == len(segs)-1
		if last && child == nil && rmType == "ACTION" && segs[i].id == actionTimeLeaf {
			return key, "ACTION", actionTimeLeaf, true
		}
		if rmType == "ACTION" && !last && isIsmTransitionSeg(child, segs[i].id) {
			named, attr, ok := ismAttrKey(key, segs, i, child)
			return named, "ACTION", attr, ok
		}
		if child == nil || sib.isShared(child) {
			return "", "", "", false
		}
		node = child
	}
	return "", "", "", false
}

// ismAttrKey judges the key below the ISM transition segment segs[i], whose Web
// Template node is ism (nil for the reference's in-context node where the Web
// Template has none): an ISM_TRANSITION leaf the node does not model, or a
// `_reason` family where there is no node to carry it.
func ismAttrKey(key string, segs []flatSeg, i int, ism *webtemplate.Node) (named, attr string, ok bool) {
	rest := segs[i+1:]
	switch {
	case len(rest) == 1 && ismLeaves[rest[0].id] && (ism == nil || childByID(ism, rest[0].id) == nil):
		return key, "ISM transition " + rest[0].id, true
	case ism == nil && rest[0].id == ismReasonFamily:
		var b strings.Builder
		for j, s := range segs[:i+2] {
			if j > 0 {
				b.WriteByte('/')
			}
			writeFlatSeg(&b, s)
		}
		return b.String(), "ISM transition reason", true
	}
	return "", "", false
}

// isIsmTransitionSeg reports whether a FLAT segment under an ACTION addresses its
// ISM transition: a Web Template child of RM type ISM_TRANSITION, or, where the
// Web Template has no child of that id, the reference's in-context
// `ism_transition` node.
func isIsmTransitionSeg(child *webtemplate.Node, id string) bool {
	if child != nil {
		return nodeRMType(child) == "ISM_TRANSITION"
	}
	return id == ismReferenceSeg
}

// definingCodeOf returns the defining_code of a rebuilt DV_CODED_TEXT, or nil
// when the value carries none (a DV_TEXT, a |raw fragment of another shape);
// reading a nil map yields no code, so such a step is refused before anything
// is written into it.
func definingCodeOf(v any) map[string]any {
	dv, _ := v.(map[string]any)
	dc, _ := dv["defining_code"].(map[string]any)
	return dc
}

// compositeLeafGroups siphons the FLAT keys addressed at a **composite** Web
// Template leaf out of content, one [rmattrGroup] per leaf instance, in a stable
// order. A composite leaf is one whose FLAT form is not a suffix set but a small
// grammar with sub-paths of its own — a party (`subject/_identifier:0|id`,
// `subject/relationship|code`) or a DV_INTERVAL (`…/lower|magnitude`) — so its
// keys have to be collected before the leaf loop, which would try to resolve
// `lower` and `relationship` as Web Template children that do not exist.
//
// The group is the same tail-carrier the `_` router uses — base is the leaf's
// *parent* path, family its own FLAT segment id, and the tails everything after
// it — so [rmattrGroup.prefix] reproduces the leaf's own FLAT spelling and the
// party / interval grammars decode it with no second mechanism (REQ-140 design
// constraint 5).
//
// Keys are consumed from content: what stays behind is ordinary leaf data. keys
// holds content's keys in sorted order, each parsed once ([parseContentKeys]); a
// key no longer in content is skipped, and the first one left that does not
// parse is refused. Each group carries sink, which takes the keys its grammar
// does not read.
func compositeLeafGroups(content map[string]any, keys []contentKey, wt *webtemplate.WebTemplate, sink *unmatchedSink) ([]rmattrGroup, error) {
	byLeaf := make(map[string]*rmattrGroup)
	for _, ck := range keys {
		key := ck.key
		if _, ok := content[key]; !ok {
			continue
		}
		if ck.err != nil {
			return nil, ck.err
		}
		base, family, index, tail, isComposite := splitCompositeLeafKey(wt, key, ck.pk)
		if !isComposite {
			continue
		}
		val := content[key]
		delete(content, key)
		id := base + "\x00" + family + "\x00" + strconv.Itoa(index)
		g := byLeaf[id]
		if g == nil {
			g = newRMAttrGroup(base, family, index, sink)
			byLeaf[id] = g
		}
		g.put(tail, val, key)
	}
	groups := make([]rmattrGroup, 0, len(byLeaf))
	for _, g := range byLeaf {
		groups = append(groups, *g)
	}
	slices.SortFunc(groups, func(a, b rmattrGroup) int {
		return strings.Compare(a.prefix(), b.prefix())
	})
	return groups, nil
}

// splitCompositeLeafKey splits a FLAT key, parsed as pk, at the composite Web
// Template leaf it addresses, if any: the first segment run that resolves to a
// childless node of a party or DV_INTERVAL RM type. base is the leaf's parent
// path, family and index the leaf segment itself, and tail everything after it
// (each remaining segment with a leading "/", then "|suffix"); base and tail
// are slices of key ([cutAtSeg]).
//
// Two kinds of key are deliberately not composite leaves. One whose path reaches
// an `_`-prefixed segment first belongs to that family — a `_feeder_audit`'s
// nested `/subject` is inside its tails, not at a Web Template node — and one
// addressing a composition-metadata leaf [emitContext] writes is the composer,
// whose keys [siphonContext] has already taken, whether or not the Web
// Template carries its node (ADR 0029).
func splitCompositeLeafKey(wt *webtemplate.WebTemplate, key string, pk parsedKey) (base, family string, index int, tail string, ok bool) {
	node := wt.Tree
	if len(pk.segs) == 0 || pk.segs[0].id != node.ID {
		return "", "", 0, "", false
	}
	for i := 1; i < len(pk.segs); i++ {
		seg := pk.segs[i]
		if strings.HasPrefix(seg.id, "_") {
			return "", "", 0, "", false
		}
		next := childByID(node, seg.id)
		if next == nil {
			return "", "", 0, "", false
		}
		node = next
		if hasValueTypes(node) && atValueLeaf(node, pk.segs[i+1:]) && isCompositeLeafType(nodeRMType(valueLeaf(node))) {
			// A key at an ELEMENT with value types reads as its first one, here
			// a composite leaf: the key is that leaf's, spelled at the ELEMENT.
			base, tail := cutAtSeg(key, i)
			return base, seg.id, seg.idx, tail, true
		}
		if len(node.Children) > 0 || !isCompositeLeafType(nodeRMType(node)) {
			continue
		}
		if ctxOnlyLeafPaths[bareAQLPath(node.AQLPath)] {
			return "", "", 0, "", false
		}
		// A `_`-prefixed segment after the leaf belongs to the leaf's own
		// grammar only where that grammar declares it: a party leaf carries a
		// nested `_identifier:N`. Every other one addresses an underscore RM
		// attribute of the LOCATABLE the Web Template folded the leaf into —
		// `<leaf>/_uid`, `<leaf>/_link:N`, `<leaf>/_feeder_audit` — which the
		// `_` router owns and resolves to that ELEMENT. Folding those into the
		// leaf's tails instead deletes them from the router's view, and the
		// leaf grammar then refuses them: a body MarshalFlat itself writes
		// (REQ-140), and the `<leaf>/_uid` the upstream corpus writes too.
		if i+1 < len(pk.segs) && strings.HasPrefix(pk.segs[i+1].id, "_") &&
			!compositeLeafOwnsSub(nodeRMType(node), pk.segs[i+1].id) {
			return "", "", 0, "", false
		}
		base, tail := cutAtSeg(key, i)
		return base, seg.id, seg.idx, tail, true
	}
	return "", "", 0, "", false
}

// atValueLeaf reports whether a key whose segments after the ELEMENT node el
// are rest is spelled at el itself, where it reads as el's first value type
// ([valueLeaf]): rest is empty, or starts with a sub-path of that type's own
// grammar, a segment that is no child of el and no underscore family, which
// the `_` router owns.
func atValueLeaf(el *webtemplate.Node, rest []flatSeg) bool {
	return len(rest) == 0 || (childByID(el, rest[0].id) == nil && !strings.HasPrefix(rest[0].id, "_"))
}

// childByID returns node's child with the given FLAT segment id, or nil.
func childByID(node *webtemplate.Node, id string) *webtemplate.Node {
	for _, ch := range node.Children {
		if ch.ID == id {
			return ch
		}
	}
	return nil
}

// isCompositeLeafType reports whether a childless Web Template leaf of this RM
// type carries a grammar with sub-paths of its own rather than a suffix set, and
// therefore has to be siphoned by [compositeLeafGroups] before the leaf loop.
func isCompositeLeafType(rmType string) bool {
	return isPartyLeafType(rmType) || isIntervalLeafType(rmType)
}

// compositeLeafOwnsSub reports whether an `_`-prefixed sub-path segment under a
// composite leaf belongs to that leaf's own grammar rather than to the LOCATABLE
// the Web Template collapsed the leaf into.
//
// Only the party grammar declares one — the nested `_identifier:N` every party
// position carries ([partyListTails]). A DV_INTERVAL leaf declares none, so
// every `_` segment beneath one is an owner attribute.
func compositeLeafOwnsSub(rmType, seg string) bool {
	id, _, _ := strings.Cut(seg, ":")
	return isPartyLeafType(rmType) && partyListTails[id]
}

// placeCompositeLeaf decodes one composite-leaf group and places the value at the
// leaf's canonical path, exactly as the clinical leaf loop places a DataValue.
// A group under, or at, an ELEMENT with value types claims the value type it
// reaches first, refused where the body's keys reach another (claims).
func placeCompositeLeaf(compJSON map[string]any, wt *webtemplate.WebTemplate, g rmattrGroup,
	sib *reusedSiblings, budget *allocBudget, tmpl *templateIndex, claims valueClaims,
) error {
	pk, err := parseFlatKey(g.prefix())
	if err != nil {
		return err
	}
	// resolveLeaf judges the leaf's own index as it judges any other leaf's:
	// a party leaf stands on a single-valued RM attribute, so `:0` is the
	// interconversion's explicit-index spelling and anything higher is refused;
	// a DV_INTERVAL leaf's index belongs to the ELEMENT the Web Template folded
	// away, which may repeat, and the placement reads it.
	node, predIndex, predType, err := resolveLeaf(wt, pk.segs, sib)
	if err != nil {
		// splitCompositeLeafKey resolved this path already, so the only ways
		// here are an index past the bound of a single-valued attribute and the
		// reused-sibling refusal, each with its own message.
		return fmt.Errorf("simplified: %q: %w", g.prefix(), err)
	}
	reach, reached := valueTypeReached(pk.segs, node, sib)
	// A key at an ELEMENT with value types reads as its first one.
	node = valueLeaf(node)
	value, ordinalBounds, err := compositeLeafValue(g, node, tmpl != nil)
	if err == nil || errors.Is(err, errNothingMatched) {
		if dupErr := g.settleDups(); dupErr != nil {
			return dupErr
		}
	}
	if errors.Is(err, errNothingMatched) {
		return nil
	}
	if err != nil {
		return err
	}
	if reached {
		if err := claims.claim(reach, g.prefix()); err != nil {
			return err
		}
	}
	key, err := placeLeaf(compJSON, sib.nodePath(node), node.AQLPath, predIndex, predType, value, budget, tmpl, sib)
	if err != nil {
		return fmt.Errorf("simplified: place %q: %w", g.prefix(), err)
	}
	for _, end := range ordinalBounds {
		// A bound's path in the template is known once the leaf is placed:
		// what the body leaves out of its ordinal is taken from the template's
		// pairs for that bound, or refused, now, as at a leaf (REQ-053).
		if err := tmpl.completeCodedValue(value[end], key+"/"+end); err != nil {
			return fmt.Errorf("simplified: decode %q: %w", g.prefix()+"/"+end, err)
		}
		settleOrdinalTerminology(value[end], tmpl.ordinalTerminologies(key+"/"+end))
	}
	return nil
}

// compositeLeafValue decodes a composite leaf's group by the leaf's RM type:
// the party grammar (rmattr_party.go) or the interval grammar
// (rmattr_value.go), each the same implementation those families use at every
// other position. It also returns the bounds of a DV_INTERVAL<DV_ORDINAL> leaf
// that were built from their suffixes, which a WithTemplate decode (deferred)
// completes and whose symbols it settles once the leaf is placed.
func compositeLeafValue(g rmattrGroup, leaf *webtemplate.Node, deferred bool) (map[string]any, []string, error) {
	if anchor, isInterval := intervalLeafAnchor(nodeRMType(leaf)); isInterval {
		return intervalLeafSuffixes(g, leafHolds(leaf), anchor, deferred)
	}
	party, populated, err := partyLeafSuffixes(g)
	if err != nil {
		return nil, nil, err
	}
	if !populated {
		return nil, nil, fmt.Errorf("%w: %s carries no party key (PARTY_IDENTIFIED needs at least one of |name, |id or an _identifier)",
			ErrUnsupportedDatatype, g.prefix())
	}
	return party, nil, nil
}

// rmattrOwnerAt resolves the owner of an underscore-family group whose base
// FLAT path is base (REQ-140): the RM class the family is judged against, plus a
// deferred accessor that materialises the canonical-JSON node to decorate.
//
// Three shapes of owner, in the order they are tested:
//
//   - `<root>/context` is the composition's EVENT_CONTEXT, resolved *without*
//     consulting the Web Template. ADR 0016 puts the EVENT_CONTEXT optionals
//     under the real `context` segment, and they are RM-optional attributes a
//     template need not constrain at all — so a template carrying no `context`
//     node must behave exactly like one that does (the PROBE-086 corpus
//     template carries one, with `start_time` and `setting` under it).
//   - the template root is the COMPOSITION itself.
//   - anything else resolves through the Web Template. A node with children owns
//     the family directly; a **childless leaf** is a collapsed ELEMENT — the Web
//     Template folds ELEMENT.value into the leaf node, so the LOCATABLE that
//     owns `<leaf>/_uid` is the ELEMENT one attribute up (the corpus spells it
//     that way on `…/any_event:0/dv_quantity`). A childless leaf whose canonical
//     path does not end in `/value` hides no LOCATABLE (an in-context
//     `context/start_time`, an ENTRY `language`, an ISM_TRANSITION member), so
//     it has no underscore owner and the key is unresolvable. An ELEMENT with
//     value types ([hasValueTypes]) owns its own families at its node, and
//     reads a value decoration there as its first value type's; under one of
//     its value types the ELEMENT owns that value's decorations alone.
func rmattrOwnerAt(wt *webtemplate.WebTemplate, compJSON map[string]any, base string,
	sib *reusedSiblings, budget *allocBudget, tmpl *templateIndex,
) (rmattrOwner, error) {
	pk, err := parseFlatKey(base)
	if err != nil {
		return rmattrOwner{}, err
	}
	segs := pk.segs
	if len(segs) == 0 || segs[0].id != wt.Tree.ID {
		return rmattrOwner{}, errSegNotFound
	}
	// ownerAql is the owner's resolution path, and ownerNames the same path as
	// the Web Template spells it ([walkAQL]).
	walk := func(ownerAql, ownerNames string, predIndex map[string]int, predType map[string]string) func(string) (map[string]any, error) {
		return func(attr string) (map[string]any, error) {
			node, _, err := walkAQL(compJSON, ownerAql+"/"+attr, ownerNames+"/"+attr, predIndex, predType, budget, tmpl, sib)
			return node, err
		}
	}
	// The OPT-free STRUCTURED interconversion re-spells every segment with an
	// explicit index, so `context` and `context:0` are one node. COMPOSITION.context
	// is single-valued, so any other index is past its bound (REQ-170), whether or
	// not the template carries a `context` node.
	if len(segs) == 2 && segs[1].id == "context" {
		if segs[1].idx > 0 {
			return rmattrOwner{}, errSingleValuedIndex(segs[1])
		}
		return rmattrOwner{kind: "EVENT_CONTEXT", resolve: walk("/context", "/context", nil, nil)}, nil
	}
	if len(segs) == 1 {
		return rmattrOwner{
			kind:    nodeRMType(wt.Tree),
			resolve: func(string) (map[string]any, error) { return compJSON, nil },
		}, nil
	}
	node, predIndex, predType, err := resolveLeaf(wt, segs, sib)
	if err != nil {
		return rmattrOwner{}, err
	}
	owner := rmattrOwner{kind: nodeRMType(node)}
	owner.reach, _ = valueTypeReached(segs, node, sib)
	ownerAql, ownerNames := sib.nodePath(node), node.AQLPath
	switch {
	case hasValueTypes(node):
		// An ELEMENT with value types owns its own families, and a value
		// decoration spelled at it reads as its first value type's, as a key at
		// it does ([valueLeaf]).
		owner.leaf = nodeRMType(valueLeaf(node))
	case len(node.Children) == 0:
		trimmed, isElementValue := strings.CutSuffix(ownerAql, "/value")
		if !isElementValue {
			return rmattrOwner{}, errSegNotFound
		}
		// The leaf's own RM type is the *anchor* the value-decoration families are
		// judged and decoded against (REQ-140 § C1): one FLAT path addresses both
		// the ELEMENT and the DataValue it holds. Under a value type of an ELEMENT
		// with value types it addresses the value alone: the ELEMENT's own
		// families are spelled at the ELEMENT.
		ownerAql, ownerNames = trimmed, strings.TrimSuffix(ownerNames, "/value")
		owner.kind, owner.leaf, owner.valueOnly = "ELEMENT", nodeRMType(node), sib.elementOf(node) != nil
	}
	owner.resolve = walk(ownerAql, ownerNames, predIndex, predType)
	return owner, nil
}

// bareAQLPath strips REQ-111 name predicates from a compiled AQL path,
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

// templateIndex is what a WithTemplate decode reads from the compiled template:
// the facts the formats do not carry and the template determines, per archetype
// node. Each is keyed by the node's compiled aqlPath, which keeps a predicate
// only on container attributes and which [walkAQL] rebuilds as it walks, and
// again by that path's bare spelling (REQ-111 name predicates removed), which
// answers where decode's key names no node the template indexes
// ([templateIndex.indexed]).
//
// A nil index is the OPT-free decode: every lookup then answers "".
type templateIndex struct {
	// names maps a path to the LOCATABLE name decode gives the node there.
	names map[string]string
	// codedNames maps a path to the DV_CODED_TEXT name decode gives the node
	// there in place of the DV_TEXT name of names, where the template
	// determines one ([codedNameOf]). It maps to the zero codedName where the
	// nodes at one path disagree on it, one of them determining none
	// included: the template then gives no single coded name.
	codedNames map[string]codedName
	// ordinals maps the compiled path of a C_DV_ORDINAL node, and its bare
	// path, to the terminology its pairs give each code at each ordinal
	// value. A pair maps to "" where the nodes at that path, or the pairs of
	// one of them, give it more than one: the template then gives no single
	// answer.
	ordinals map[string]map[ordinalPair]string
	// ordinalCodes maps the compiled path of a C_DV_ORDINAL node, and its
	// bare path, to what its pairs give each code ([ordinalCode]).
	ordinalCodes map[string]map[string]ordinalCode
	// codedValues maps the compiled path of a DV_CODED_TEXT node, and its
	// bare path, to what its defining_code constraint gives a coded value
	// there ([codedTerms]).
	codedValues map[string]codedTerms
	// rmTypes maps a path to the RM type the template gives the node there. It
	// maps to "" where the nodes at one path disagree, the alternatives of one
	// single-valued attribute, say, which share a path: the template then gives
	// no single answer.
	rmTypes map[string]string
	// structures maps the bare path of a node to the empty structures decode
	// may rebuild under it ([emptyStructure]). A path whose nodes disagree on
	// them has no entry: the template then gives no single answer.
	structures map[string][]emptyStructure
	// structureRoute holds the bare path of every node on the way to a key of
	// structures, that key included, so the rebuild walk enters no other node.
	structureRoute map[string]bool
	// category is the composition category the template fixes ([categoryOf]),
	// or the zero codedName where it fixes none.
	category codedName
	// startTime is the start time [WithDefaultTime] gives a body that carries
	// none, as decode writes it, or "" for none. It is the caller's, not the
	// template's: the cached index ([templateIndexFor]) never holds one, and
	// [decodeFlatMap] sets it on a copy that one decode reads.
	startTime string
	// actionArchetypeIDs maps the compiled path of an ACTIVITY node to the
	// action_archetype_id decode gives an ACTIVITY there that the body gives
	// none ([actionArchetypeIDOf]). It maps to "" where the nodes at one
	// path disagree on it, or where the generator writes none: the template
	// then gives no single answer, and decode leaves the attribute out.
	actionArchetypeIDs map[string]string
}

// categoryOf returns the category the template fixes on root, its
// COMPOSITION: where the one child the template allows on root's category is
// a DV_CODED_TEXT whose defining_code is constrained by one C_CODE_PHRASE, or
// one C_CODE_REFERENCE read as the code phrase it carries, of the openehr
// terminology listing exactly one code, it is that code with its openEHR
// rubric as its text ([codeTermText]). A template that constrains the
// category in any other way, or not at all, and a code with no single rubric
// fix none: categoryOf then returns the zero codedName.
func categoryOf(root *templatecompile.CompiledNode) codedName {
	a := root.Attribute("category")
	if a == nil {
		return codedName{}
	}
	children := tcimpl.AllowedChildNodes(a)
	if len(children) != 1 || children[0].RMTypeName() != "DV_CODED_TEXT" {
		return codedName{}
	}
	phrase, ok := constraints.CodePhraseOf(onePrimitive(children[0], "defining_code"))
	if !ok || phrase.Terminology != terminology.ID || len(phrase.CodeList) != 1 {
		return codedName{}
	}
	code := phrase.CodeList[0]
	text, ok := codeTermText(children[0], terminology.ID, code)
	if !ok {
		return codedName{}
	}
	return codedName{value: text, code: code, terminology: terminology.ID}
}

// emptyStructure is a structure decode rebuilds with no member, under a
// rebuilt node that lacks it, as the generator writes it: an ITEM_STRUCTURE
// object that holds its type, its node id and a name, and, where the
// template requires the list that holds its items by its existence, that
// list present and empty ([requiredEmptyList]), and nothing else. The
// formats have no key for a structure with no member, so its node is never
// rebuilt from the body.
//
// The template gives one where a single-valued attribute of a node allows
// only children that agree, on the RM type and the node id, and that are
// each a concrete ITEM_STRUCTURE the template lets hold no member
// ([holdsNoMember]). A slot and an archetype root give none: the node they
// stand for is not the template's alone to name.
type emptyStructure struct {
	// attr is the single-valued attribute that holds the structure, and path
	// the structure's bare path, the one its name in the index falls back to
	// ([rebuildEmptyStructures]).
	attr, path string
	// rmType is the structure's concrete ITEM_STRUCTURE subtype, and nodeID its
	// archetype_node_id.
	rmType, nodeID string
	// required is set where the template's existence requires attr. Where it
	// does not, decode rebuilds the structure only where the RM requires attr
	// of the rebuilt node's class, as the validator reads it.
	required bool
	// list names the structure's list that the rebuilt structure carries
	// present and empty ([requiredEmptyList]), or is "" for none.
	list string
}

// name returns the LOCATABLE name the template gives the node at path, or "".
func (x *templateIndex) name(path string) string {
	if x == nil {
		return ""
	}
	return x.names[x.indexed(path)]
}

// indexed returns path, or its bare spelling where the index holds no node at
// path. Decode composes its key with every name the Web Template pins on the
// way ([walkAQLKeyed], [rebuildEmptyStructures]), which is the node's compiled
// path; where a key names no node the template indexes, the bare alias, under
// which every node is also indexed, answers.
func (x *templateIndex) indexed(path string) string {
	if _, ok := x.rmTypes[path]; ok || !strings.Contains(path, ",'") {
		return path
	}
	return bareAQLPath(path)
}

// nameJSON returns, as a canonical object, the name decode gives the node at
// path: the coded name the template determines there, else a DV_TEXT of the
// node's name, or nil where the index gives the node no name.
func (x *templateIndex) nameJSON(path string) map[string]any {
	if x == nil {
		return nil
	}
	path = x.indexed(path)
	if c := x.codedNames[path]; c != (codedName{}) {
		return c.json()
	}
	if name := x.name(path); name != "" {
		return textJSON(name)
	}
	return nil
}

// ordinalPair is a C_DV_ORDINAL pair without its terminology: the symbol's
// code and the ordinal value, which FLAT carries as |code and |ordinal.
type ordinalPair struct {
	code  string
	value int
}

// addOrdinals records the terminology each pair of ord, the C_DV_ORDINAL of
// the node n at path, gives its code at its value. A code and value that
// another pair or another node at path gives another terminology are left
// with none. It also records what the pairs give each code ([ordinalCode]).
func (x *templateIndex) addOrdinals(path string, n *templatecompile.CompiledNode, ord constraints.CDvOrdinal) {
	terms := x.ordinals[path]
	if terms == nil {
		terms = make(map[ordinalPair]string, len(ord.Values))
		x.ordinals[path] = terms
	}
	codes := x.ordinalCodes[path]
	if codes == nil {
		codes = make(map[string]ordinalCode, len(ord.Values))
		x.ordinalCodes[path] = codes
	}
	for _, v := range ord.Values {
		pair := ordinalPair{code: v.Symbol.CodeString, value: v.Value}
		term := v.Symbol.Terminology
		if prev, seen := terms[pair]; seen && prev != term {
			term = ""
		}
		terms[pair] = term
		text, _ := codeTermText(n, v.Symbol.Terminology, v.Symbol.CodeString)
		c, seen := codes[pair.code]
		switch {
		case !seen:
			c = ordinalCode{value: v.Value, text: text}
		case c.value != v.Value:
			c.ambiguous = true
		}
		c.text = agreedText(c.text, text)
		codes[pair.code] = c
	}
}

// agreedText is the term text two constraints that give one code the texts
// held and next give it together: the text where they agree, none ("") where
// they do not.
func agreedText(held, next string) string {
	if held != next {
		return ""
	}
	return held
}

// ordinalCode is what the C_DV_ORDINAL pairs at one path give one code: its
// ordinal value, and its term text in the pair's terminology
// ([codeTermText]), "" for none. A code that pairs there give two values is
// ambiguous: the template gives it neither. Pairs that give it two texts
// leave it with none.
type ordinalCode struct {
	value     int
	text      string
	ambiguous bool
}

// ordinalCodeAt returns what the template's C_DV_ORDINAL at path gives code
// ([ordinalCode]), looked up as [templateIndex.ordinalTerminologies] looks up
// the pairs. ok is false where no pair there holds code, where the code is
// ambiguous, and where there is no C_DV_ORDINAL at path.
func (x *templateIndex) ordinalCodeAt(path, code string) (c ordinalCode, ok bool) {
	codes, held := x.ordinalCodes[path]
	if !held {
		codes = x.ordinalCodes[bareAQLPath(path)]
	}
	c, ok = codes[code]
	return c, ok && !c.ambiguous
}

// codedTerms is what the template gives a coded value at one path: the
// terminology that its defining_code constraint names, and the term text of
// each code it lists that has one ([codeTermText]). The zero value gives no
// terminology and no text.
type codedTerms struct {
	terminology string
	texts       map[string]string
}

// codedTermsOf returns what n, a DV_CODED_TEXT node, gives a coded value: the
// terminology of the one C_CODE_PHRASE or C_CODE_REFERENCE that constrains its
// defining_code, a reference set whose code phrase names none taking the one
// its terminology: URI names ([constraints.CodePhraseWithTerminologyOf]), the
// SDK's reading (REQ-053), and the term text of each code that phrase lists,
// the text a coded node name takes. A defining_code the template constrains in
// any other way, or not at all, and a code phrase that names no terminology
// and no such URI give nothing.
func codedTermsOf(n *templatecompile.CompiledNode) codedTerms {
	phrase, ok := constraints.CodePhraseWithTerminologyOf(onePrimitive(n, "defining_code"))
	if !ok || phrase.Terminology == "" {
		return codedTerms{}
	}
	t := codedTerms{terminology: phrase.Terminology, texts: make(map[string]string, len(phrase.CodeList))}
	for _, code := range phrase.CodeList {
		if text, ok := codeTermText(n, phrase.Terminology, code); ok {
			t.texts[code] = text
		}
	}
	return t
}

// addCodedTerms records t, what a DV_CODED_TEXT node at path gives a coded
// value there. A second node at that path, an alternative, that names
// another terminology, or none, leaves the path with nothing; one that names
// the same adds the texts of its codes, and a code the two give different
// texts is left with none.
func (x *templateIndex) addCodedTerms(path string, t codedTerms) {
	prev, seen := x.codedValues[path]
	switch {
	case !seen:
		x.codedValues[path] = t
	case prev.terminology != t.terminology:
		x.codedValues[path] = codedTerms{}
	default:
		// A node's texts are recorded under its compiled path and its bare
		// alias alike, so they are merged into a copy. A reference set read
		// by its URI records no texts, a nil map.
		texts := make(map[string]string, len(prev.texts)+len(t.texts))
		maps.Copy(texts, prev.texts)
		for code, text := range t.texts {
			if held, both := texts[code]; both {
				text = agreedText(held, text)
			}
			texts[code] = text
		}
		x.codedValues[path] = codedTerms{terminology: prev.terminology, texts: texts}
	}
}

// codedTermsAt returns what the template gives a coded value at path
// ([codedTerms]), looked up as [templateIndex.ordinalTerminologies] looks up
// an ordinal's pairs.
func (x *templateIndex) codedTermsAt(path string) codedTerms {
	if t, ok := x.codedValues[path]; ok {
		return t
	}
	return x.codedValues[bareAQLPath(path)]
}

// completeCodedValue gives dv, a value decode built from a leaf's suffixes
// and placed at path, the suffixes the body leaves out and the template
// determines (REQ-053), and refuses one it leaves out and the template does
// not determine, as a missing required suffix. Only a coded text and an
// ordinal leave any out, and only in a decode with the template
// ([clinicalLeafValue]); every other value is left as it is, and so is every
// value in a decode without the template.
//
// A coded text that names no terminology takes the one the template's
// defining_code constraint names at path ([codedTerms]), and one with no
// value takes the term text that constraint lists for its code, where the
// coded text's terminology is that constraint's: the text a coded node name
// takes. A terminology the template does not determine is refused, except at
// an ISM transition, which [settleIsmTransitions] gives one
// ([ismSettlesTerminology]). An ordinal with no ordinal value, or no symbol
// text, takes the value and the term text the template's pairs at path give
// its code, where they give it one value ([ordinalCode]); its terminology is
// settled after ([settleOrdinalTerminology]). What the body gives is kept as
// written, for validation to judge.
func (x *templateIndex) completeCodedValue(dv any, path string) error {
	m, _ := dv.(map[string]any)
	if x == nil || m == nil {
		return nil
	}
	switch m["_type"] {
	case "DV_CODED_TEXT":
		return x.completeCodedText(m, path)
	case "DV_ORDINAL":
		return x.completeOrdinal(m, path)
	}
	return nil
}

// completeCodedText is [templateIndex.completeCodedValue] for a coded text.
func (x *templateIndex) completeCodedText(dv map[string]any, path string) error {
	dc, _ := dv["defining_code"].(map[string]any)
	terms := x.codedTermsAt(path)
	if _, named := dc["terminology_id"]; !named && terms.terminology != "" {
		dc["terminology_id"] = map[string]any{"_type": "TERMINOLOGY_ID", "value": terms.terminology}
	}
	if _, valued := dv["value"]; !valued {
		text := ""
		if tid, _ := dc["terminology_id"].(map[string]any); terms.terminology != "" && tid["value"] == terms.terminology {
			code, _ := dc["code_string"].(string)
			text = terms.texts[code]
		}
		if text == "" {
			return errMissingSuffix("DV_CODED_TEXT", "value")
		}
		dv["value"] = text
	}
	if _, named := dc["terminology_id"]; !named && !ismSettlesTerminology(path) {
		return errMissingSuffix("DV_CODED_TEXT", "terminology")
	}
	return nil
}

// completeOrdinal is [templateIndex.completeCodedValue] for an ordinal.
func (x *templateIndex) completeOrdinal(dv map[string]any, path string) error {
	symbol, _ := dv["symbol"].(map[string]any)
	dc, _ := symbol["defining_code"].(map[string]any)
	code, _ := dc["code_string"].(string)
	c, ok := x.ordinalCodeAt(path, code)
	if _, valued := symbol["value"]; !valued {
		if !ok || c.text == "" {
			return errMissingSuffix("DV_ORDINAL", "value")
		}
		symbol["value"] = c.text
	}
	if _, valued := dv["value"]; !valued {
		if !ok {
			return errMissingSuffix("DV_ORDINAL", "ordinal")
		}
		dv["value"] = c.value
	}
	return nil
}

// ismSettlesTerminology reports whether path names an ISM transition's current
// state, transition or careflow step, whose terminology [settleIsmTransitions]
// gives where the body names none: RM Current_state_valid and
// Transition_valid admit `openehr` alone, and a careflow step is coded with an
// at-code. Only ISM_TRANSITION has an attribute of any of these names.
func ismSettlesTerminology(path string) bool {
	_, attr, _ := strings.CutLast(path, "/")
	return attr == "current_state" || attr == "transition" || attr == careflowStepLeaf
}

// mergedCareflowStep reports whether leaf is the careflow step of the
// in-context `ism_transition` node that merges an ACTION's per-step nodes, the
// only in-context ISM transition node with one.
func mergedCareflowStep(leaf *webtemplate.Node) bool {
	return strings.HasSuffix(bareAQLPath(leaf.AQLPath), "/"+ismReferenceSeg+"/"+careflowStepLeaf)
}

// ordinalTerminologies returns the terminology the template's C_DV_ORDINAL at
// path gives each code and value, "" for one it gives no single one; nil
// where there is none, and in a decode without the template. Below reused
// siblings whose C_DV_ORDINALs differ, path, which keeps every name the
// template pins, finds the sibling's own pairs; the bare alias answers where
// it names no C_DV_ORDINAL the template holds.
func (x *templateIndex) ordinalTerminologies(path string) map[ordinalPair]string {
	if x == nil {
		return nil
	}
	if terms, ok := x.ordinals[path]; ok {
		return terms
	}
	return x.ordinals[bareAQLPath(path)]
}

// addCodedName records name, the coded name the node at path gets, the zero
// codedName where it gets none. A second node at that path with another
// leaves the path with no coded name.
func (x *templateIndex) addCodedName(path string, name codedName) {
	if prev, seen := x.codedNames[path]; seen && prev != name {
		name = codedName{}
	}
	x.codedNames[path] = name
}

// codedName is a DV_CODED_TEXT name the template determines for a node, or
// the composition category it fixes ([categoryOf]): its text, and the code and
// terminology of its defining code. The zero value is no coded value.
type codedName struct {
	value, code, terminology string
}

// json is the canonical object of the coded name c.
func (c codedName) json() map[string]any {
	return map[string]any{
		"_type":         "DV_CODED_TEXT",
		"value":         c.value,
		"defining_code": codePhraseJSON(c.code, c.terminology),
	}
}

// codedNameOf returns the coded name the template determines for the node n,
// or the zero codedName where it determines none. The template determines
// one where it constrains n's name as a DV_CODED_TEXT, through one child or
// through alternatives that agree on it, each of which determines one coded
// value ([codedValueOf]). A name the template constrains in any other way,
// or not at all, gets the DV_TEXT name of the index.
func codedNameOf(n *templatecompile.CompiledNode) codedName {
	attr := n.Attribute("name")
	if attr == nil {
		return codedName{}
	}
	children := tcimpl.AllowedChildNodes(attr)
	if len(children) == 0 {
		return codedName{}
	}
	first := codedValueOf(children[0])
	for _, c := range children[1:] {
		if codedValueOf(c) != first {
			return codedName{}
		}
	}
	return first
}

// codedValueOf returns the coded value c, a child of a node's name,
// determines, or the zero codedName where it determines none. c must be a
// DV_CODED_TEXT whose defining_code the template constrains through one
// C_CODE_PHRASE, or one C_CODE_REFERENCE read as the code phrase it carries,
// of a named terminology, that lists either exactly one code, or several of
// which exactly one has a term text ([codeTermText]) equal to the text the
// template pins on c's value: a closed C_STRING list of one entry. The
// value's text is the pinned text, else the code's term text, and the
// C_STRING on c's value, if any, must admit it. This is the name the
// generator writes there. A code phrase that names no terminology or lists
// no code, such as a reference set that names only its URI, and several
// codes with no pinned text leave the code open, and a code with no term
// text and no pinned text leaves the text open: decode invents neither.
func codedValueOf(c *templatecompile.CompiledNode) codedName {
	if c.RMTypeName() != "DV_CODED_TEXT" {
		return codedName{}
	}
	phrase, ok := constraints.CodePhraseOf(onePrimitive(c, "defining_code"))
	if !ok || phrase.Terminology == "" {
		return codedName{}
	}
	text, hasText := onePrimitive(c, "value").(constraints.CString)
	pinned := ""
	if hasText && len(text.List) == 1 && !text.ListOpen {
		pinned = text.List[0]
	}
	code := ""
	switch {
	case len(phrase.CodeList) == 1:
		code = phrase.CodeList[0]
	case pinned != "":
		for _, listed := range phrase.CodeList {
			if term, ok := codeTermText(c, phrase.Terminology, listed); ok && term == pinned {
				if code != "" && code != listed {
					return codedName{}
				}
				code = listed
			}
		}
	}
	if code == "" {
		return codedName{}
	}
	value := pinned
	if value == "" {
		if value, ok = codeTermText(c, phrase.Terminology, code); !ok {
			return codedName{}
		}
	}
	if hasText && len(text.Validate(value)) > 0 {
		return codedName{}
	}
	return codedName{value: value, code: code, terminology: phrase.Terminology}
}

// onePrimitive returns the primitive constraint of the one child the template
// allows on the attribute attr of n, or nil where it allows none or more than
// one.
func onePrimitive(n *templatecompile.CompiledNode, attr string) constraints.PrimitiveConstraint {
	a := n.Attribute(attr)
	if a == nil {
		return nil
	}
	children := tcimpl.AllowedChildNodes(a)
	if len(children) != 1 {
		return nil
	}
	return children[0].PrimitiveConstraint()
}

// codeTermText returns the term text of code in terminologyID, as the
// generator reads it for a coded name under the node n. A code of the openEHR
// terminology takes its rubric ([openehrRubric]). Any other takes the text of
// the term that n's own archetype, its nearest enclosing archetype root,
// defines for it: under the code for a local code, else under
// "<terminology>::<code>", or under the code alone where that is no at-code,
// the key some OPTs give such a term. ok is false where none gives a text.
func codeTermText(n *templatecompile.CompiledNode, terminologyID, code string) (string, bool) {
	switch terminologyID {
	case terminology.ID:
		return openehrRubric(code)
	case localTerminology:
		text, ok := tcimpl.RootTermItem(n, code, "text")
		return text, ok && text != ""
	}
	if text, ok := tcimpl.RootTermItem(n, terminologyID+"::"+code, "text"); ok && text != "" {
		return text, true
	}
	if isAtCode(code) {
		return "", false
	}
	text, ok := tcimpl.RootTermItem(n, code, "text")
	return text, ok && text != ""
}

// openehrRubric returns the rubric the openEHR terminology gives code. A
// rubric belongs to a code within a group, and the RM ties a node's name to
// no group, so code has a rubric only where every group that holds it gives
// it the same one: 532 is "complete" in one group and "completed" in another,
// and has none.
func openehrRubric(code string) (string, bool) {
	rubric := ""
	for g := range terminology.Groups() {
		r, ok := g.Rubric(code)
		if !ok {
			continue
		}
		if rubric != "" && r != rubric {
			return "", false
		}
		rubric = r
	}
	return rubric, rubric != ""
}

// isAtCode reports whether code is an archetype term code: "at" followed by
// digits, with any dot extensions of digits for a specialisation, such as
// at0010 or at0010.2 (ADL 1.4, Node Identification).
func isAtCode(code string) bool {
	rest, ok := strings.CutPrefix(code, "at")
	if !ok {
		return false
	}
	for part := range strings.SplitSeq(rest, ".") {
		if part == "" || strings.Trim(part, "0123456789") != "" {
			return false
		}
	}
	return true
}

// rmType returns the one RM type the template gives the node at path, or ""
// where it gives none or the nodes there disagree.
func (x *templateIndex) rmType(path string) string {
	if x == nil {
		return ""
	}
	return x.rmTypes[x.indexed(path)]
}

// addActionArchetypeID records id for the ACTIVITY node at path. A second
// node at that path with another id leaves the path with no single answer.
func (x *templateIndex) addActionArchetypeID(path, id string) {
	if prev, seen := x.actionArchetypeIDs[path]; seen && prev != id {
		id = ""
	}
	x.actionArchetypeIDs[path] = id
}

// actionArchetypeID returns the action_archetype_id decode gives an ACTIVITY
// at path that the body gives none, or "" where the template gives no single
// one. Every ACTIVITY decode rebuilds stands for a node of the template, so
// its path is indexed.
func (x *templateIndex) actionArchetypeID(path string) string {
	return x.actionArchetypeIDs[x.indexed(path)]
}

// actionArchetypeIDOf returns the action_archetype_id the generator writes on
// the ACTIVITY n under its example fill, or "" where it writes none. Like the
// generator, it reads the first child the template allows on that attribute:
// the value [cstringvalue.ActionArchetypeID] picks from that child's
// C_STRING, or "" where the C_STRING admits no value the pick builds. Where
// n holds no such attribute (a template compiled without implicit
// attributes) or the template allows no child there, the template names no
// archetype, and the RM default [cstringvalue.AnyActionArchetype] stands.
func actionArchetypeIDOf(n *templatecompile.CompiledNode) string {
	a := n.Attribute("action_archetype_id")
	if a == nil {
		return cstringvalue.AnyActionArchetype
	}
	children := tcimpl.AllowedChildNodes(a)
	if len(children) == 0 {
		return cstringvalue.AnyActionArchetype
	}
	// A child with no C_STRING, a C_PRIMITIVE_OBJECT with no item say, gives
	// the zero constraint, which names no archetype either.
	cs, _ := children[0].PrimitiveConstraint().(constraints.CString)
	id, err := cstringvalue.ActionArchetypeID(cs, cs.ExampleValue())
	if err != nil {
		return ""
	}
	return id
}

// addRMType records rmType for the node at path. A second node at that path
// with another type leaves the path with no single answer.
func (x *templateIndex) addRMType(path, rmType string) {
	if prev, seen := x.rmTypes[path]; seen && prev != rmType {
		rmType = ""
	}
	x.rmTypes[path] = rmType
}

// templateIndexes holds the index [buildTemplateIndex] built for each compiled
// template, for as long as the template lives, so the decodes that share a
// template build its index once. It is keyed by a weak pointer to the template,
// so the cache does not keep a template alive, and the entry is dropped once
// the template is collected; the index holds no pointer back into it. A
// compiled template does not change once compiled, and no decode writes to an
// index, so every decode with one template can read the same index.
var templateIndexes sync.Map // weak.Pointer[templatecompile.Compiled] → *templateIndex

// templateIndexFor returns the index of the compiled template c, building it
// on the first decode with c ([templateIndexes]). Two first decodes at once may
// both build it; the one stored first wins, and the other is discarded.
func templateIndexFor(c *templatecompile.Compiled) *templateIndex {
	key := weak.Make(c)
	if x, ok := templateIndexes.Load(key); ok {
		return x.(*templateIndex)
	}
	x, loaded := templateIndexes.LoadOrStore(key, buildTemplateIndex(c))
	if !loaded {
		runtime.AddCleanup(c, func(key weak.Pointer[templatecompile.Compiled]) { templateIndexes.Delete(key) }, key)
	}
	return x.(*templateIndex)
}

// buildTemplateIndex walks the compiled template and indexes each archetype
// node's RM type and its LOCATABLE name — the template-level node name where
// the OPT pins one (REQ-111: that pinned value IS the node's modelled name, and
// it is what the reference materialises), else the node-id term rubric in the
// default language, read from the node's nearest enclosing archetype root
// alone, as the Web Template builder and the generator read it: a node whose
// own archetype does not define its node id gets no name from the index.
// Decode repopulates LOCATABLE.name from the names, and takes
// a folded ITEM_STRUCTURE's concrete subtype from the types (see WithTemplate).
// Where the template constrains a node's name as a DV_CODED_TEXT and
// determines its code and text, the node's coded name ([codedNameOf]) takes
// the place of its DV_TEXT name.
//
// Every node is indexed under both its compiled path and that path's bare
// spelling. Siblings sharing a bare path carry the same at-code and therefore
// the same rubric — but where they pin *distinct* names (archetype reuse under
// a slot), the bare spelling cannot say which sibling is meant, so the name
// alias falls back to the shared rubric rather than guess. Decode looks every
// node up under its compiled path, every name the template pins on the way
// kept, so a node below a reused sibling takes the name its own template node
// pins, and turns to the bare alias only where that key misses
// ([templateIndex.indexed]). Siblings sharing a bare path with
// distinct RM types leave the type alias with no answer, and siblings there
// that disagree on a coded name, one of them giving none included, leave the
// alias with no coded name, so instances read under the alias take the
// DV_TEXT name.
//
// The empty structures each node may get ([emptyStructure]) are indexed under
// the bare path alone, the one the rebuild walk composes; their names are read
// as any other node's. Nodes at one bare path that disagree on them,
// alternatives or siblings that narrow one archetype differently, leave the
// path with none, since decode cannot tell which of them a rebuilt node stands
// for ([dropDisagreeingStructures]).
//
// The walk reads the compiled template in place, through the module-internal
// readers that do not copy a node's attributes, an attribute's children or a
// term's items. A compiled template carries one set of terms, in its default
// language, so the rubric is in that language.
func buildTemplateIndex(c *templatecompile.Compiled) *templateIndex {
	x := &templateIndex{
		names:              make(map[string]string),
		codedNames:         make(map[string]codedName),
		rmTypes:            make(map[string]string),
		structures:         make(map[string][]emptyStructure),
		ordinals:           make(map[string]map[ordinalPair]string),
		ordinalCodes:       make(map[string]map[string]ordinalCode),
		codedValues:        make(map[string]codedTerms),
		actionArchetypeIDs: make(map[string]string),
	}
	var walk func(n *templatecompile.CompiledNode)
	walk = func(n *templatecompile.CompiledNode) {
		// Under the compiled path, the key [walkAQLKeyed] composes, and its
		// bare alias ([templateIndex.ordinalTerminologies]).
		if ord, ok := n.PrimitiveConstraint().(constraints.CDvOrdinal); ok {
			path := n.AQLPath()
			x.addOrdinals(path, n, ord)
			if bare := bareAQLPath(path); bare != path {
				x.addOrdinals(bare, n, ord)
			}
		}
		if n.RMTypeName() == "ACTIVITY" {
			// Under the compiled path alone: the completion walk keeps the
			// names the template pins, so an ACTIVITY's path is its own
			// node's, and a bare alias would let a named twin's id meet the
			// id of the unnamed node whose path that alias is.
			x.addActionArchetypeID(n.AQLPath(), actionArchetypeIDOf(n))
		}
		if n.RMTypeName() == "DV_CODED_TEXT" {
			path, terms := n.AQLPath(), codedTermsOf(n)
			x.addCodedTerms(path, terms)
			if bare := bareAQLPath(path); bare != path {
				x.addCodedTerms(bare, terms)
			}
		}
		if s := emptyStructuresOf(n); len(s) > 0 {
			if bare := bareAQLPath(n.AQLPath()); x.structures[bare] == nil {
				x.structures[bare] = s
			}
		}
		if id := n.NodeID(); id != "" {
			path := n.AQLPath()
			bare := bareAQLPath(path)
			x.addRMType(path, n.RMTypeName())
			coded := codedNameOf(n)
			x.addCodedName(path, coded)
			if bare != path {
				x.addRMType(bare, n.RMTypeName())
				x.addCodedName(bare, coded)
			}
			rubric, _ := tcimpl.RootTermItem(n, id, "text")
			name := n.NodeName()
			if name == "" {
				name = rubric
			}
			if name != "" {
				x.names[path] = name
				if bare != path {
					if prev, taken := x.names[bare]; taken && prev != name {
						// Two named siblings share this bare spelling —
						// ambiguous; keep the shared rubric instead.
						x.names[bare] = rubric
					} else {
						x.names[bare] = name
					}
				}
			}
		}
		for _, a := range tcimpl.NodeAttributes(n) {
			for _, ch := range tcimpl.ChildNodes(a) {
				walk(ch)
			}
		}
	}
	if root := c.Root(); root != nil {
		walk(root)
		dropDisagreeingStructures(root, x.structures)
		x.category = categoryOf(root)
	}
	x.structureRoute = make(map[string]bool)
	for p := range x.structures {
		for i := 1; i <= len(p); i++ {
			if i == len(p) || p[i] == '/' {
				x.structureRoute[p[:i]] = true
			}
		}
	}
	return x
}

// dropDisagreeingStructures removes from structures each bare path at which
// two nodes of the template below root, the root included, disagree on the
// empty structures they give ([emptyStructuresOf]): a node that gives none
// there disagrees with one that gives some.
func dropDisagreeingStructures(root *templatecompile.CompiledNode, structures map[string][]emptyStructure) {
	if len(structures) == 0 {
		return
	}
	var walk func(n *templatecompile.CompiledNode)
	walk = func(n *templatecompile.CompiledNode) {
		bare := bareAQLPath(n.AQLPath())
		if s, ok := structures[bare]; ok && !slices.Equal(s, emptyStructuresOf(n)) {
			delete(structures, bare)
		}
		for _, a := range tcimpl.NodeAttributes(n) {
			for _, ch := range tcimpl.ChildNodes(a) {
				walk(ch)
			}
		}
	}
	walk(root)
}

// emptyStructuresOf returns, in attribute order, the empty structures the
// template gives the node n ([emptyStructure]), one per single-valued
// attribute at most.
func emptyStructuresOf(n *templatecompile.CompiledNode) []emptyStructure {
	var out []emptyStructure
	for _, a := range tcimpl.NodeAttributes(n) {
		if a.Cardinality() == template.Multiple {
			continue
		}
		children := tcimpl.AllowedChildNodes(a)
		if len(children) == 0 {
			continue
		}
		first := children[0]
		if slices.ContainsFunc(children, func(c *templatecompile.CompiledNode) bool {
			return c.RMTypeName() != first.RMTypeName() || c.NodeID() != first.NodeID() || !isEmptyStructure(c)
		}) {
			continue
		}
		out = append(out, emptyStructure{
			attr:     a.Name(),
			path:     bareAQLPath(first.AQLPath()),
			rmType:   first.RMTypeName(),
			nodeID:   first.NodeID(),
			required: tcimpl.LowerAtLeastOne(a.Existence()),
			list:     requiredEmptyList(first),
		})
	}
	return out
}

// requiredEmptyList returns the name of the list of c, a template node of a
// concrete ITEM_STRUCTURE subtype, that the generator writes present and
// empty when it gives the list no member: the list the template requires by
// its existence (a bounded existence lower bound of 1 or more), one whose
// every child it prohibits included, of those canonical JSON carries as []
// when present and empty ([rmlists.KeepsEmpty]): an ITEM_LIST's or
// ITEM_TREE's items, or an ITEM_TABLE's rows. It returns "" where c carries
// no such list, where the template leaves it optional, as the generator then
// leaves it out.
func requiredEmptyList(c *templatecompile.CompiledNode) string {
	class := bmmtype.Class(c.RMTypeName())
	for _, a := range tcimpl.NodeAttributes(c) {
		if rmlists.KeepsEmpty(class, a.Name()) && tcimpl.LowerAtLeastOne(a.Existence()) && !a.NotInRM() {
			return a.Name()
		}
	}
	return ""
}

// isEmptyStructure reports whether decode can rebuild the template node c as
// a structure with no member: c is no slot and no archetype root, is of a
// concrete ITEM_STRUCTURE subtype, and holds no member the template or the RM
// requires ([holdsNoMember]). A node with no node id has no name in the index,
// so [rebuildEmptyStructures] leaves it out.
func isEmptyStructure(c *templatecompile.CompiledNode) bool {
	if c.IsSlot() || c.ArchetypeID() != "" {
		return false
	}
	if conforms, _ := tcimpl.ClassConformsTo(c.RMTypeName(), "ITEM_STRUCTURE"); !conforms {
		return false
	}
	h, ok := rminfo.Default.(rminfo.Hierarchy)
	if !ok {
		return false
	}
	if abstract, _ := h.IsAbstract(c.RMTypeName()); abstract {
		return false
	}
	return holdsNoMember(c)
}

// holdsNoMember reports whether the template node c, an ITEM_STRUCTURE,
// validates with no member, as the validator reads it (clinical-modeling.md,
// An existence lower bound alone asks for no member). The RM requires none of
// its attributes but its name and node id, which the rebuilt structure
// carries. The template requires none of the others: no single-valued one by
// its existence, no list by its existence together with a member it asks for
// ([tcimpl.AsksForMember]), and no list it leaves optional by its existence
// by a child that asks a list with no item for a member
// ([tcimpl.MandatoryListChildren]). The validator also lets an RM rule that
// keeps a list non-empty once present ask for a member, but the floor keeps
// no list of an ITEM_STRUCTURE so ([rminfo.NonEmptyLists]).
func holdsNoMember(c *templatecompile.CompiledNode) bool {
	for _, attr := range rminfo.Default.RequiredAttributes(bmmtype.Class(c.RMTypeName())) {
		if !carriedByEmptyStructure(attr) {
			return false
		}
	}
	for _, a := range tcimpl.NodeAttributes(c) {
		if carriedByEmptyStructure(a.Name()) || a.NotInRM() {
			continue
		}
		existence := tcimpl.LowerAtLeastOne(a.Existence())
		switch {
		case a.Cardinality() != template.Multiple:
			if existence {
				return false
			}
		case existence:
			if tcimpl.AsksForMember(a) {
				return false
			}
		default:
			for range tcimpl.MandatoryListChildren(a) {
				return false
			}
		}
	}
	return true
}

// carriedByEmptyStructure reports whether attr is one a rebuilt empty
// structure carries: its archetype_node_id and its name.
func carriedByEmptyStructure(attr string) bool {
	return attr == "archetype_node_id" || attr == "name"
}

// rebuildEmptyStructures gives each node decode rebuilt the empty structures
// the template gives it ([emptyStructure]) that it lacks, where the template's
// existence or the RM requires the attribute that holds one: the formats have
// no key for a structure with no member, so a body whose template requires
// one never spells it. Each structure gets its type, its node id and the name
// the template index gives its path, and its list present and empty where the
// template requires that list by its existence; one whose path has no name, a
// node with no node id say, is not rebuilt. The walk composes each node's bare
// path as walkAQL does, a predicate only on a list member, and enters only the
// nodes on the way to one the template gives structures (structureRoute). The
// root is entered with an empty path, which no key has: a COMPOSITION holds no
// ITEM_STRUCTURE. It never builds a parent node, a member or a value: only a
// structure under a node the body rebuilt.
//
// The structure's name is read under its path with the names the template
// pins on the way kept (named): a member's name stays in its predicate where
// the index holds a node at that spelling. Below reused siblings (REQ-111),
// one structure can pin a name its namesake below the other sibling does not
// pin, and the bare path would give it the rubric the two share.
func rebuildEmptyStructures(compJSON map[string]any, tmpl *templateIndex) {
	if len(tmpl.structures) == 0 {
		return
	}
	var walk func(node map[string]any, path, named []byte)
	walk = func(node map[string]any, path, named []byte) {
		base, namedBase := len(path), len(named)
		for _, s := range tmpl.structures[string(path)] {
			if _, has := node[s.attr]; has || (!s.required && !rmRequires(node, s.attr)) {
				continue
			}
			if name := tmpl.nameJSON(string(named) + "/" + s.attr); name != nil {
				structure := map[string]any{
					"_type":             s.rmType,
					"archetype_node_id": s.nodeID,
					"name":              name,
				}
				if s.list != "" {
					structure[s.list] = []any{}
				}
				node[s.attr] = structure
			}
		}
		for attr, v := range node {
			switch x := v.(type) {
			case map[string]any:
				path = append(append(path[:base], '/'), attr...)
				if tmpl.structureRoute[string(path)] {
					named = append(append(named[:namedBase], '/'), attr...)
					walk(x, path, named)
				}
			case []any:
				for _, e := range x {
					m, ok := e.(map[string]any)
					if !ok {
						continue
					}
					path = appendMemberPath(path[:base], attr, m)
					if tmpl.structureRoute[string(path)] {
						named = appendMemberName(appendMemberPath(named[:namedBase], attr, m), m, tmpl.rmTypes)
						walk(m, path, named)
					}
				}
			}
		}
	}
	walk(compJSON, make([]byte, 0, 256), make([]byte, 0, 256))
}

// completeActionArchetypeIDs gives each ACTIVITY in compJSON that the body
// gives no action_archetype_id the one the template index gives its path
// ([templateIndex.actionArchetypeID]): the value the generator writes there,
// so the RM-mandatory attribute the formats let a client leave out is
// filled as the generator fills it (REQ-053). An id the body gives is kept;
// an empty one was refused where the leaf was read. The walk composes each
// node's path as [rebuildEmptyStructures] does, a predicate only on a list
// member, with the names the template pins on the way kept where the index
// holds that spelling. A template with no ACTIVITY skips the walk.
func completeActionArchetypeIDs(compJSON map[string]any, tmpl *templateIndex) {
	if len(tmpl.actionArchetypeIDs) == 0 {
		return
	}
	var walk func(node map[string]any, named []byte)
	walk = func(node map[string]any, named []byte) {
		if t, _ := node["_type"].(string); t == "ACTIVITY" {
			if _, has := node["action_archetype_id"]; !has {
				if id := tmpl.actionArchetypeID(string(named)); id != "" {
					node["action_archetype_id"] = id
				}
			}
		}
		base := len(named)
		for attr, v := range node {
			switch x := v.(type) {
			case map[string]any:
				named = append(append(named[:base], '/'), attr...)
				walk(x, named)
			case []any:
				for _, e := range x {
					if m, ok := e.(map[string]any); ok {
						named = appendMemberName(appendMemberPath(named[:base], attr, m), m, tmpl.rmTypes)
						walk(m, named)
					}
				}
			}
		}
	}
	walk(compJSON, make([]byte, 0, 256))
}

// rmRequires reports whether the RM requires attr of node's class, as the
// validator reads it, from the value's class.
func rmRequires(node map[string]any, attr string) bool {
	t, _ := node["_type"].(string)
	return slices.Contains(rminfo.Default.RequiredAttributes(bmmtype.Class(t)), attr)
}

// checkNoPhantoms walks the rebuilt tree and rejects any container element that
// selectElem gap-filled but no leaf ever reached: an instance carrying nothing
// beyond _type and archetype_node_id, or one still marked [gapFilled]. Such
// phantoms arise only from a sparse :index sequence (":0" and ":2" with no
// ":1"); accepting them would fabricate empty — and, after RM-mandatory
// completion, seemingly valid — clinical instances out of a malformed payload.
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
			if phantomKeysOnly(m) || m[gapFilled] != nil {
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

// orderByTemplate puts the members of every list in the rebuilt tree in the
// Web Template's node order (REQ-053). FLAT orders only the instances of one
// node, by their :index; nothing orders distinct nodes, and the leaf loop, the
// composite leaves and the underscore families each append the members they
// reach first, in the order of their keys. A member's place is where its node
// first appears in a pre-order walk of the Web Template ([templateRanks]),
// which follows the template. The instances of one node share a place, and
// the sort is stable, so they keep their :index order. A member no Web Template
// node accounts for, one without an archetype_node_id say, goes after the rest
// in the order it was placed, so a list of RM attribute values keeps its order.
//
// The walk descends only into the nodes whose path the Web Template accounts
// for: no node below any other one is a member it places. Reused siblings that
// their pinned names tell apart (sib, REQ-111) each take their own place, and a
// member stands for one of them by its name.
func orderByTemplate(compJSON map[string]any, wt *webtemplate.WebTemplate, sib *reusedSiblings) {
	rank := templateRanks(wt, sib)
	named := len(sib.kept) > 0
	memberPath := func(path []byte, attr string, m map[string]any) []byte {
		path = appendMemberPath(path, attr, m)
		if named {
			path = appendMemberName(path, m, rank)
		}
		return path
	}
	type member struct {
		rank int
		v    any
	}
	// walk visits the members of node, whose path is in path, and gives the
	// buffer back cut to that path; it grows the buffer for each child's path.
	var walk func(node map[string]any, path []byte) []byte
	walk = func(node map[string]any, path []byte) []byte {
		base := len(path)
		for attr, v := range node {
			switch x := v.(type) {
			case map[string]any:
				path = memberPath(path[:base], attr, x)
				if _, known := rank[string(path)]; known {
					path = walk(x, path)
				}
			case []any:
				if len(x) > 1 {
					ranks := make([]int, len(x))
					sorted := true
					for i, e := range x {
						m, _ := e.(map[string]any)
						path = memberPath(path[:base], attr, m)
						r, known := rank[string(path)]
						if !known {
							r = math.MaxInt
						}
						ranks[i] = r
						sorted = sorted && (i == 0 || ranks[i-1] <= r)
					}
					if !sorted {
						members := make([]member, len(x))
						for i, e := range x {
							members[i] = member{rank: ranks[i], v: e}
						}
						slices.SortStableFunc(members, func(a, b member) int { return cmp.Compare(a.rank, b.rank) })
						for i, mb := range members {
							x[i] = mb.v
						}
					}
				}
				for _, e := range x {
					m, ok := e.(map[string]any)
					if !ok {
						continue
					}
					path = memberPath(path[:base], attr, m)
					if _, known := rank[string(path)]; known {
						path = walk(m, path)
					}
				}
			}
		}
		return path[:base]
	}
	walk(compJSON, make([]byte, 0, 256))
}

// appendMemberPath appends to the path of a rebuilt node the bare canonical
// path segment of the node m it holds under attr: the attribute, and m's
// archetype_node_id as its predicate where it has one — the spelling [walkAQL]
// keys predIndex by, and the bare spelling of a Web Template node's aqlPath.
func appendMemberPath(path []byte, attr string, m map[string]any) []byte {
	path = append(path, '/')
	path = append(path, attr...)
	if id, _ := m["archetype_node_id"].(string); id != "" {
		path = append(path, '[')
		path = append(path, id...)
		path = append(path, ']')
	}
	return path
}

// appendMemberName gives path, which ends at the segment [appendMemberPath]
// appended for the rebuilt node m, m's name in that segment's predicate where
// known holds that spelling. In the ranks of [orderByTemplate], m is then one
// of several reused siblings that their names tell apart; in the
// [templateIndex], it stands for the node the template pins that name on.
func appendMemberName[V any](path []byte, m map[string]any, known map[string]V) []byte {
	name := nameValue(m)
	if name == "" || len(path) == 0 || path[len(path)-1] != ']' {
		return path
	}
	end := len(path) - 1
	path = append(path[:end], tcimpl.NamePredicate("", name)...)
	path = append(path, ']')
	if _, ok := known[string(path)]; ok {
		return path
	}
	return append(path[:end], ']')
}

// templateRanks maps the resolution path ([reusedSiblings]) of every Web
// Template node, the bare spelling of its aqlPath with only the names that tell
// reused siblings apart, and every prefix of it that ends at a segment, to the
// position at which a pre-order walk of the Web Template first reaches it. A
// prefix stands for a node the Web Template folds away, such as the ELEMENT a
// collapsed leaf at `…/items[at0004]/value` stands for at `…/items[at0004]`:
// that ELEMENT takes its leaf's place.
func templateRanks(wt *webtemplate.WebTemplate, sib *reusedSiblings) map[string]int {
	rank := make(map[string]int)
	next := 0
	var walk func(n *webtemplate.Node)
	walk = func(n *webtemplate.Node) {
		p := sib.nodePath(n)
		for seg := range rawSegments(p) {
			if _, seen := rank[p[:seg.end]]; !seen {
				rank[p[:seg.end]] = next
			}
		}
		next++
		for _, ch := range n.Children {
			walk(ch)
		}
	}
	if wt.Tree != nil {
		walk(wt.Tree)
	}
	return rank
}

// selectElem finds (or creates) the element with archetype_node_id==pred in
// cur[attr]'s list, at the idx-th position among same-pred siblings (idx is
// the flat :index for a repeatable node; 0 otherwise). Distinct sibling node
// ids get distinct elements even without an explicit index.
//
// name, where not nil, is the name of a reused sibling (REQ-111): only the
// elements whose name has its value count, and an element created here gets
// it. An element created only to fill the slots before idx carries that name
// too, so a later key of the same sibling finds it, and it is marked
// [gapFilled] until a key selects it, so [checkNoPhantoms] still sees it.
// excluded, where not empty, are the names of the reused siblings beside one
// that pins none: an element carrying one of them does not count.
func selectElem(cur map[string]any, attr, elemType, pred string, name map[string]any, excluded []string, idx int, budget *allocBudget) (map[string]any, error) {
	want := max(idx, 0)
	if want > maxRepeatIndex {
		return nil, fmt.Errorf("%w: :index %d exceeds bound %d", ErrUnknownPath, want, maxRepeatIndex)
	}
	arr, _ := cur[attr].([]any)
	pinned, _ := name["value"].(string)
	var matches []int
	for i, e := range arr {
		if m, ok := e.(map[string]any); ok && m["archetype_node_id"] == pred && (name == nil || nameValue(m) == pinned) &&
			(len(excluded) == 0 || !slices.Contains(excluded, nameValue(m))) {
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
		if name != nil {
			el["name"] = deepCopyJSON(name)
			el[gapFilled] = true
		}
		arr = append(arr, el)
		matches = append(matches, len(arr)-1)
	}
	cur[attr] = arr
	// arr[matches[want]] is an element this function appended or matched as a
	// map[string]any above, so the assertion cannot fail.
	el := arr[matches[want]].(map[string]any)
	delete(el, gapFilled)
	return el, nil
}

// gapFilled marks an element [selectElem] created with a reused sibling's name
// to fill a slot before the one a key selects, until a key selects it. It
// cannot clash with an RM attribute, and [checkNoPhantoms] refuses a decode
// that leaves one, so it never reaches canjson.
const gapFilled = "\x00gap-filled"

// nameValue returns the value of the name the rebuilt node m carries, or "".
func nameValue(m map[string]any) string {
	name, _ := m["name"].(map[string]any)
	v, _ := name["value"].(string)
	return v
}

// namedOneOf reports whether name, a rebuilt name object, has one of names as
// its value.
func namedOneOf(name map[string]any, names []string) bool {
	if len(names) == 0 {
		return false
	}
	v, _ := name["value"].(string)
	return slices.Contains(names, v)
}

// concreteType resolves the RM type to instantiate for attr on parentType,
// mapping the abstract RM slots to concrete types the way the Web Template /
// canonical form require. wtType is the Web Template rmType positionally
// resolved for this segment ("" when the segment has no WT node — e.g. the
// collapsed wrappers); tmplType is the RM type the compiled template gives the
// node ("" in an OPT-free decode, or where the template gives no single
// answer); nextAttr is the following aqlPath attribute. Both resolve the
// abstract ITEM_STRUCTURE slot whose concrete subtype the Web Template does not
// carry (it collapses those nodes).
func concreteType(parentType, attr, wtType, tmplType, nextAttr string) string {
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
		// subtype is absent from predType. Under WithTemplate the compiled
		// template names it (REQ-053), and the walk takes that type where it can
		// build it there.
		if foldedStructureType(tmplType, nextAttr) {
			return tmplType
		}
		// Otherwise infer it from the child attribute: `item` -> ITEM_SINGLE,
		// `rows` -> ITEM_TABLE, `items` -> ITEM_TREE / ITEM_LIST. ITEM_TREE and
		// ITEM_LIST both use `items` and are not distinguishable from the path
		// alone; default to ITEM_TREE, which is round-trip-preserving (rmpath
		// re-resolves by attribute + node id). See deviations.md.
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

// foldedStructureType reports whether rmType, the type the compiled template
// gives a folded ITEM_STRUCTURE, is one the walk can build there: an
// ITEM_STRUCTURE that declares nextAttr, the attribute the path goes on
// through. A template naming anything else gives the walk nothing to take.
func foldedStructureType(rmType, nextAttr string) bool {
	if rmType == "" {
		return false
	}
	h, ok := rminfo.Default.(rminfo.Hierarchy)
	if !ok {
		return false
	}
	if conforms, _ := h.ConformsTo(rmType, "ITEM_STRUCTURE"); !conforms {
		return false
	}
	_, declared := rminfo.Default.AttributeRMType(rmType, nextAttr)
	return declared
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
//
// A suffix holding JSON null is absent (REQ-053), so a null required suffix is
// a missing one, and no null reaches the canonical tree as the zero of its
// type. Every other value must fit the RM attribute it fills
// ([checkSuffixFits]); one that does not is refused naming the FLAT key keyOf
// gives for its suffix (nil names the suffix alone). A |raw fragment fits when
// its _type is rmType or an RM subtype of it ([checkRawFits]) and canonical
// JSON decodes it as that _type ([checkRawDecodes]).
func dvFromSuffixes(rmType string, listOpen bool, sfx map[string]any, keyOf func(suffix string) string) (map[string]any, error) {
	return dvFromSuffixesAt(rmType, rmType, listOpen, sfx, keyOf)
}

// dvFromSuffixesAt is [dvFromSuffixes] at a position whose RM attribute is
// declared as holds, which can be wider than rmType, the datatype the
// suffixes are read as: a collapsed ELEMENT's value is a DATA_VALUE, an
// interval bound the RM holds as an interface is a DV_ORDERED, and the feeder
// audit's content is a DV_ENCAPSULATED. A |raw fragment there fits when its
// _type is holds or an RM subtype of it and its members decode as that _type,
// which is what canonical JSON admits.
func dvFromSuffixesAt(holds, rmType string, listOpen bool, sfx map[string]any, keyOf func(suffix string) string) (map[string]any, error) {
	return dvFromSuffixesWith(holds, rmType, listOpen, sfx, keyOf, false)
}

// clinicalLeafValue is [leafFromSuffixes] for a leaf of the clinical leaf
// loop. Where deferred, a coded text and an ordinal are built without the
// suffixes the body leaves out and may be supplied once the value is placed:
// in a decode with the template, [templateIndex.completeCodedValue] gives or
// refuses them, and at the careflow step of a merged in-context ISM
// transition, in either mode, [settleMergedIsmTransition] does.
func clinicalLeafValue(leaf *webtemplate.Node, sfx map[string]any, keyOf func(suffix string) string, deferred bool) (any, error) {
	rmType := nodeRMType(leaf)
	if !deferred || rmType == stringLeafType {
		return leafFromSuffixes(leafHolds(leaf), rmType, leafListOpen(leaf), sfx, keyOf)
	}
	return dvFromSuffixesWith(leafHolds(leaf), rmType, leafListOpen(leaf), sfx, keyOf, true)
}

// dvFromSuffixesWith is [dvFromSuffixesAt], with deferred set where a decode
// with the template may supply what the body leaves out: a coded text's
// |value and an ordinal's |value and |ordinal are then left out of the value
// rather than refused ([deferrableSuffix]).
func dvFromSuffixesWith(holds, rmType string, listOpen bool, sfx map[string]any, keyOf func(suffix string) string, deferred bool) (map[string]any, error) {
	sfx = withoutNulls(sfx)
	// |raw bypass: a pre-serialised canonical fragment (carrying its own string
	// _type); used directly, whatever the leaf rmType, once the attribute it
	// fills admits its _type. Mutually exclusive with every other suffix.
	if raw, ok := sfx["raw"]; ok {
		if len(sfx) > 1 {
			return nil, fmt.Errorf("%w: |raw is mutually exclusive with other suffixes", ErrUnsupportedDatatype)
		}
		frag, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%w: |raw value is not a canonical object", ErrUnsupportedDatatype)
		}
		t, ok := frag["_type"].(string)
		if !ok || t == "" {
			return nil, fmt.Errorf("%w: |raw fragment missing string _type", ErrUnsupportedDatatype)
		}
		if err := checkRawFits(t, holds, keyOf); err != nil {
			return nil, err
		}
		if err := checkRawDecodes(frag, t, keyOf); err != nil {
			return nil, err
		}
		return frag, nil
	}
	// REQ-053 — the one substitution carried in suffix form: a DV_CODED_TEXT
	// stored at a DV_TEXT-typed leaf (the corpus's dv_coded_text_as_dv_text
	// shape). |code is the discriminator: its presence re-selects the
	// DV_CODED_TEXT builder, and the group then follows that type's own rules —
	// |value required, |terminology / |formatting optional, bare value refused
	// (a decode with the template completes the coded text as it completes any
	// other, [templateIndex.completeCodedValue]).
	// Without |code the leaf stays a plain DV_TEXT, whose |value is its bare
	// value ([shorthandSpellings]), and a stray |terminology is refused by the
	// allowlist below, as before. The encoder is the exact inverse:
	// leafToFlat routes a fully-captured DV_CODED_TEXT at a DV_TEXT leaf to the
	// coded suffix set instead of |raw.
	// The discriminator is a |code that is not the empty string, not the key's
	// mere presence: `|code: ""` and `|code: null` are what a form emits for
	// "free text, no code selected", and promoting those would mint a
	// DV_CODED_TEXT whose CODE_PHRASE.code_string is empty — RM-invalid, yet
	// stable enough on re-encode that nothing downstream flags it, and matched by
	// any AQL predicate testing defining_code/code_string against ''. They stay a
	// plain DV_TEXT: a null |code is absent (REQ-053, dropped above), so a
	// |value beside one is the text's value, and the allowlist below refuses
	// the stray `|code: ""`, as it did before this carve-out. A |code of
	// another JSON kind does select the coded builder, so the fit check refuses
	// it naming the |code key (REQ-053), rather than the allowlist refusing the
	// leaf.
	if rmType == "DV_TEXT" {
		if code, coded := sfx["code"]; coded {
			if s, isString := code.(string); !isString || s != "" {
				rmType = "DV_CODED_TEXT"
			}
		}
	}
	if err := checkSuffixAllowlist(rmType, sfx); err != nil {
		return nil, err
	}
	if err := checkSuffixFits(rmType, sfx, keyOf); err != nil {
		return nil, err
	}
	sfx, err := foldShorthand(rmType, sfx, keyOf)
	if err != nil {
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
			// |other carries the value alone, so it is mutually exclusive with
			// *every* other suffix — not just |code. The rebuild below returns a
			// bare DV_TEXT, so admitting a companion (|formatting, |terminology, …)
			// would accept it and then discard it silently; the encoder routes such
			// a value to |raw, and decode has to refuse the shape it never writes.
			if len(sfx) > 1 {
				return nil, fmt.Errorf("%w: |other is mutually exclusive with every other suffix, got %s",
					ErrUnsupportedDatatype, strings.Join(slices.Sorted(maps.Keys(sfx)), "+"))
			}
			return map[string]any{"_type": "DV_TEXT", "value": other}, nil
		}
		code, err := requireSuffix(rmType, sfx, "code")
		if err != nil {
			return nil, err
		}
		val, valued, err := deferrableSuffix(rmType, sfx, "value", deferred)
		if err != nil {
			return nil, err
		}
		dc := map[string]any{"_type": "CODE_PHRASE", "code_string": code}
		if t, ok := sfx["terminology"]; ok {
			dc["terminology_id"] = map[string]any{"_type": "TERMINOLOGY_ID", "value": t}
		}
		dv := map[string]any{"_type": "DV_CODED_TEXT", "defining_code": dc}
		if valued {
			dv["value"] = val
		}
		if err := applyOrderedSuffixes(dv, sfx); err != nil {
			return nil, err
		}
		return dv, nil
	case "DV_ORDINAL":
		code, err := requireSuffix(rmType, sfx, "code")
		if err != nil {
			return nil, err
		}
		val, valued, err := deferrableSuffix(rmType, sfx, "value", deferred)
		if err != nil {
			return nil, err
		}
		ordinal, ordered, err := deferrableSuffix(rmType, sfx, "ordinal", deferred)
		if err != nil {
			return nil, err
		}
		// The suffixes carry no terminology for the symbol: it is rebuilt in
		// "local", as the reference does, and a WithTemplate decode then gives
		// it the template's ([settleOrdinalTerminology]).
		symbol := map[string]any{
			"_type": "DV_CODED_TEXT",
			"defining_code": map[string]any{
				"_type": "CODE_PHRASE", "code_string": code,
				"terminology_id": map[string]any{"_type": "TERMINOLOGY_ID", "value": "local"},
			},
		}
		if valued {
			symbol["value"] = val
		}
		dv := map[string]any{"_type": "DV_ORDINAL", "symbol": symbol}
		if ordered {
			dv["value"] = ordinal
		}
		return dv, nil
	case "DV_SCALE":
		// The DV_ORDINAL grammar one class over: same symbol, same three
		// suffixes, `|ordinal` carrying DV_SCALE's Real `value` (scaleToFlat).
		code, err := requireSuffix(rmType, sfx, "code")
		if err != nil {
			return nil, err
		}
		val, err := requireSuffix(rmType, sfx, "value")
		if err != nil {
			return nil, err
		}
		scale, err := requireSuffix(rmType, sfx, "ordinal")
		if err != nil {
			return nil, err
		}
		symbol := map[string]any{
			"_type": "DV_CODED_TEXT", "value": val,
			"defining_code": map[string]any{
				"_type": "CODE_PHRASE", "code_string": code,
				"terminology_id": map[string]any{"_type": "TERMINOLOGY_ID", "value": "local"},
			},
		}
		return map[string]any{"_type": "DV_SCALE", "value": scale, "symbol": symbol}, nil
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
		// |preferred_term is the one attribute the standalone leaf spelling carries
		// and the nested defining_code triple does not (corpus:
		// `dv_text/_language|preferred_term`).
		if pt, ok := sfx["preferred_term"]; ok {
			cp["preferred_term"] = pt
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
	case "DV_PARSABLE":
		// Both attributes are RM-mandatory, so half a leaf is refused rather than
		// decoded to a coerced empty string. `charset` and `language` ride the
		// REQ-140 `_charset` / `_language` members, not suffixes.
		value, err := requireSuffix(rmType, sfx, "")
		if err != nil {
			return nil, err
		}
		formalism, err := requireSuffix(rmType, sfx, "formalism")
		if err != nil {
			return nil, err
		}
		return map[string]any{"_type": "DV_PARSABLE", "value": value, "formalism": formalism}, nil
	case "DV_MULTIMEDIA":
		return multimediaFromSuffixes(sfx)
	}
	return nil, fmt.Errorf("%w: %s", ErrUnsupportedDatatype, rmType)
}

// withoutNulls returns m without the entries that hold JSON null, which
// REQ-053 reads as absent: m itself when it holds none.
func withoutNulls(m map[string]any) map[string]any {
	var out map[string]any
	for k, v := range m {
		if v != nil {
			continue
		}
		if out == nil {
			out = maps.Clone(m)
		}
		delete(out, k)
	}
	if out == nil {
		return m
	}
	return out
}

// multimediaFromSuffixes rebuilds a canonical DV_MULTIMEDIA from its FLAT suffix
// set (REQ-140). `media_type` and `size` are RM-mandatory, so they are required
// rather than defaulted; the bare key is the **uri**, and the two Byte[]
// attributes carry the base64 their canonical form uses.
//
// The three CODE_PHRASE-valued attributes reachable here travel as a bare code,
// each in an implied terminology: media_type in [mediaTypeTerminology], the two
// algorithms in the code sets [multimediaAlgorithmCodes] names. `charset`,
// `language` and `thumbnail` are not suffixes — they ride the REQ-140 underscore
// members.
func multimediaFromSuffixes(sfx map[string]any) (map[string]any, error) {
	mediaType, err := requireSuffix("DV_MULTIMEDIA", sfx, "mediatype")
	if err != nil {
		return nil, err
	}
	size, err := requireSuffix("DV_MULTIMEDIA", sfx, "size")
	if err != nil {
		return nil, err
	}
	code, err := ctxString("|mediatype", mediaType)
	if err != nil {
		return nil, err
	}
	dv := map[string]any{
		"_type":      "DV_MULTIMEDIA",
		"media_type": codePhraseJSON(code, mediaTypeTerminology),
		"size":       size,
	}
	if uri, ok := sfx[""]; ok {
		dv["uri"] = map[string]any{"_type": "DV_URI", "value": uri}
	}
	for suffix, attr := range map[string]string{
		"data": "data", "integrity_check": "integrity_check", "alternatetext": "alternate_text",
	} {
		if v, ok := sfx[suffix]; ok {
			dv[attr] = v
		}
	}
	for _, a := range multimediaAlgorithmCodes {
		v, ok := sfx[a.attr]
		if !ok {
			continue
		}
		algo, err := ctxString("|"+a.attr, v)
		if err != nil {
			return nil, err
		}
		// The bare code in the code set the RM names for the attribute (REQ-140),
		// by the identifier the pinned terminology gives it (REQ-034). Membership
		// is not checked here: the validation floor reports a code outside the
		// set, as it does for |normal_status.
		dv[a.attr] = codePhraseJSON(algo, a.codeSet.ExternalID())
	}
	return dv, nil
}

// allowedSuffixes lists, per datatype, the pipe suffixes (and "" for a bare
// value) the decoder maps. A key outside this set (a typo like |unitt, or a
// decorated attribute like |accuracy that only rides |raw) is rejected rather
// than silently dropped. |other and |raw are handled before this check.
var allowedSuffixes = map[string]map[string]bool{
	"DV_TEXT":       {"": true, "value": true, "formatting": true},
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
	"DV_SCALE":   {"code": true, "value": true, "ordinal": true},
	"DV_PROPORTION": {
		"numerator": true, "denominator": true, "type": true,
		"magnitude_status": true, "normal_status": true,
		"accuracy": true, "accuracy_is_percent": true, "precision": true,
	},
	"DV_IDENTIFIER": {"": true, "id": true, "issuer": true, "assigner": true, "type": true},
	"CODE_PHRASE":   {"code": true, "terminology": true, "preferred_term": true},
	"DV_PARSABLE":   {"": true, "value": true, "formalism": true},
	"DV_MULTIMEDIA": {
		"": true, "mediatype": true, "size": true, "data": true,
		"alternatetext": true, "integrity_check": true,
		"integrity_check_algorithm": true, "compression_algorithm": true,
	},
}

// shorthandSpelling is a second spelling decode reads for one attribute of a
// datatype: short is the suffix ("" for the bare value) a client may write in
// place of full, the suffix encode writes.
type shorthandSpelling struct {
	short, full, attr string
}

// shorthandSpellings are the input shorthands of the ITS-REST 1.1.0
// Simplified Formats: a DV_IDENTIFIER's id may be written as the bare value
// ("For the input |id might be left out"), and a DV_TEXT's or a DV_PARSABLE's
// value as `|value`. Decode reads either spelling; encode writes full only.
var shorthandSpellings = map[string]shorthandSpelling{
	"DV_IDENTIFIER": {short: "", full: "id", attr: "id"},
	"DV_TEXT":       {short: "value", full: "", attr: "value"},
	"DV_PARSABLE":   {short: "value", full: "", attr: "value"},
}

// foldShorthand rewrites the shorthand spelling in sfx, a suffix map of rmType
// with no nulls left, onto the suffix its datatype is built from
// ([shorthandSpellings]). Both spellings may be present when they hold the
// same value. Two that differ are refused naming both keys and neither value,
// rather than one being chosen (REQ-053). The map returned is sfx itself when
// there is nothing to rewrite.
func foldShorthand(rmType string, sfx map[string]any, keyOf func(string) string) (map[string]any, error) {
	sp, has := shorthandSpellings[rmType]
	if !has {
		return sfx, nil
	}
	v, short := sfx[sp.short]
	if !short {
		return sfx, nil
	}
	if w, both := sfx[sp.full]; both && !sameCtxValue(v, w) {
		return nil, fmt.Errorf("%w: %s and %s spell the %s %s twice and disagree; remove one",
			ErrUnknownPath, suffixLabel(keyOf, sp.full), suffixLabel(keyOf, sp.short), rmType, sp.attr)
	}
	out := maps.Clone(sfx)
	delete(out, sp.short)
	out[sp.full] = v
	return out, nil
}

// orderedPassThrough are the optional DV_ORDERED / DV_QUANTIFIED / DV_AMOUNT
// suffixes, and DV_TEXT's `|formatting`, whose value passes through as decoded
// onto the canonical attribute of the same name (a json.Number keeps its exact
// lexical form), in sorted order. `|normal_status`, a CODE_PHRASE rebuilt from
// a bare code, is the one optional suffix that does not pass through.
var orderedPassThrough = func() []string {
	var out []string
	for suffix, path := range orderedSuffixAttrPaths {
		if path == suffix {
			out = append(out, suffix)
		}
	}
	slices.Sort(out)
	return out
}()

// applyOrderedSuffixes copies the optional suffixes present in sfx onto the
// canonical object dv. An absent suffix sets nothing — the attributes are all
// optional in the RM, so a missing one must stay absent rather than become a
// zero value (the same contract requireSuffix enforces for mandatory ones).
//
// [dvFromSuffixes] has already checked every value against its RM attribute
// ([checkSuffixFits]) while the FLAT key was in hand to name it, so nothing
// reaches canjson that would fail there naming a canonical path the payload
// author never wrote. The RM's own value constraints (magnitude_status against
// its code set, …) stay with the validation package.
func applyOrderedSuffixes(dv, sfx map[string]any) error {
	for _, suffix := range orderedPassThrough {
		if v, ok := sfx[suffix]; ok {
			dv[suffix] = v
		}
	}
	if v, ok := sfx["normal_status"]; ok {
		code, err := ctxString("|normal_status", v)
		if err != nil {
			return err
		}
		// The bare code alone: the terminology is the implied openehr, and this
		// decode stays lenient about membership (see [normalStatusCaptured]).
		dv["normal_status"] = codePhraseJSON(code, terminology.ID)
	}
	return nil
}

// checkSuffixAllowlist rejects any suffix a datatype does not map, naming the
// first such suffix in sorted order, so a leaf with two names the same one on
// every run. An unmapped rmType is left to the switch (ErrUnsupportedDatatype).
// |other is allowed only for DV_CODED_TEXT (the case then enforces listOpen).
func checkSuffixAllowlist(rmType string, sfx map[string]any) error {
	allowed, known := allowedSuffixes[rmType]
	if !known {
		return nil
	}
	unexpected := func(k string) bool {
		return !allowed[k] && (k != "other" || rmType != "DV_CODED_TEXT")
	}
	first, refused := "", false
	for k := range sfx {
		if unexpected(k) && (!refused || k < first) {
			first, refused = k, true
		}
	}
	if !refused {
		return nil
	}
	label := "|" + first
	if first == "" {
		label = "bare value"
	}
	return fmt.Errorf("%w: unexpected %s for %s", ErrUnsupportedDatatype, label, rmType)
}

// settleOrdinalTerminology gives the symbol of dv, a DV_ORDINAL decode built
// from its suffixes in local, the terminology terms gives its code at its
// ordinal value, where it gives one (REQ-053): the formats carry an ordinal's
// code and value but not its terminology, and a WithTemplate decode takes it
// from the template's C_DV_ORDINAL. dv has been placed already; the symbol is
// changed in place.
//
// The value is read as canonical JSON reads an Integer, from a JSON number or
// a numeric string. One that is no Integer is refused when the tree is
// decoded, so the terminology it would take does not matter.
func settleOrdinalTerminology(dv any, terms map[ordinalPair]string) {
	m, _ := dv.(map[string]any)
	symbol, _ := m["symbol"].(map[string]any)
	code, _ := symbol["defining_code"].(map[string]any)
	codeString, _ := code["code_string"].(string)
	value, err := strconv.ParseFloat(fmt.Sprint(m["value"]), 64)
	if err != nil {
		return
	}
	if t := terms[ordinalPair{code: codeString, value: int(value)}]; t != "" {
		code["terminology_id"] = map[string]any{"_type": "TERMINOLOGY_ID", "value": t}
	}
}

// leafListOpen reports whether a Web Template leaf constrains an open value-set
// (any input with listOpen) — the precondition for the |other free-text form.
func leafListOpen(node *webtemplate.Node) bool {
	return slices.ContainsFunc(node.Inputs, func(in webtemplate.Input) bool { return in.ListOpen })
}

// leafHolds names the RM type of the attribute a Web Template leaf's value
// fills, the type a `|raw` fragment there must name or descend from (REQ-053).
// A collapsed ELEMENT's leaf fills ELEMENT.value, a DATA_VALUE, whatever
// datatype the template names: the encoder writes a substituted value there
// as `|raw` stamped with its own type. Every other leaf types the attribute
// it fills itself: an in-context leaf (an EVENT's `time`, an ENTRY's
// `language`, COMPOSITION `category`), say. A node with children takes no
// `|raw` at all: [leafSuffixes] refuses one there first.
func leafHolds(leaf *webtemplate.Node) string {
	if strings.HasSuffix(bareAQLPath(leaf.AQLPath), "/value") {
		return "DATA_VALUE"
	}
	return nodeRMType(leaf)
}

// requireSuffix returns sfx[name], or an error if it is absent — a missing
// required suffix must not become a coerced zero value in the canonical RM.
func requireSuffix(rmType string, sfx map[string]any, name string) (any, error) {
	v, ok := sfx[name]
	if !ok {
		return nil, errMissingSuffix(rmType, name)
	}
	return v, nil
}

// deferrableSuffix is [requireSuffix] for a suffix a decode with the template
// may supply once the value is placed ([templateIndex.completeCodedValue]):
// where deferred, an absent one is no error, and ok reports whether sfx holds
// it.
func deferrableSuffix(rmType string, sfx map[string]any, name string, deferred bool) (v any, ok bool, err error) {
	if v, ok = sfx[name]; ok || deferred {
		return v, ok, nil
	}
	return nil, false, errMissingSuffix(rmType, name)
}

// errMissingSuffix refuses a value of rmType whose required suffix name the
// body leaves out, "" naming the bare value.
func errMissingSuffix(rmType, name string) error {
	label := "|" + name
	if name == "" {
		label = "bare value"
	}
	return fmt.Errorf("%w: %s missing required %s", ErrUnsupportedDatatype, rmType, label)
}

// impliedCompositionName is the name decode gives a composition whose body
// spells no `<root>/_name`: the Web Template root's name, or the template id
// where the root has none. Encode writes no `_name` for a plain DV_TEXT name
// of this value, so a round trip names the composition the same way (REQ-140).
func impliedCompositionName(wt *webtemplate.WebTemplate) string {
	return cmp.Or(wt.Tree.Name, wt.TemplateID)
}

// textJSON is a canonical DV_TEXT object.
func textJSON(value string) map[string]any {
	return map[string]any{"_type": "DV_TEXT", "value": value}
}

// emptySuffixFold is what [foldEmptySuffix] made of a body's keys ending in a
// bare `|`.
type emptySuffixFold struct {
	// spellings maps a key of the folded body to every key as written that it
	// stands for, where that is not just itself.
	spellings map[string][]string
	// conflicts maps a key x to the `x|` beside it whose value differs; `x|` is
	// out of the folded body until [emptySuffixFold.settle] judges the pair.
	conflicts map[string]string
}

// foldEmptySuffix reads a key that ends in a bare `|` as the spelling of the key
// without it: the empty suffix is the bare value, so `x|` is `x`. A lone `x|`
// is renamed `x`; beside an `x` of the same value it is read once. Beside an
// `x` of another value it is held out, and the pair is judged once the decode
// knows whether it reads `x` ([emptySuffixFold.settle]): two spellings of one
// field the decode reads disagree, and a pair it does not read is two
// unmatched keys (REQ-170). Either way the sink reports every spelling as
// written. A spelling that holds null is absent (REQ-053), so beside the
// other spelling it is no disagreement: the value is the other one's. The map
// returned is flat itself when nothing was folded.
func foldEmptySuffix(flat map[string]any) (map[string]any, emptySuffixFold) {
	var out map[string]any
	var fold emptySuffixFold
	for _, key := range slices.Sorted(maps.Keys(flat)) {
		bare, piped := strings.CutSuffix(key, "|")
		if !piped {
			continue
		}
		if out == nil {
			out = maps.Clone(flat)
			fold.spellings = make(map[string][]string)
		}
		delete(out, key)
		other, both := flat[bare]
		switch {
		case !both:
			out[bare] = flat[key]
			fold.spellings[bare] = []string{key}
		case sameCtxValue(other, flat[key]), flat[key] == nil:
			// A null `x|` is absent beside `x` (REQ-053): no second value.
			fold.spellings[bare] = []string{bare, key}
		case other == nil:
			// A null `x` is absent: `x|` carries the one value.
			out[bare] = flat[key]
			fold.spellings[bare] = []string{bare, key}
		default:
			fold.spellings[bare] = []string{bare, key}
			if fold.conflicts == nil {
				fold.conflicts = make(map[string]string)
			}
			fold.conflicts[bare] = key
		}
	}
	if out == nil {
		return flat, fold
	}
	return out, fold
}

// settle refuses a held-out `x|` whose `x` the decode read: sink reports the
// spellings of every key it did not read, so a pair it did not report is two
// spellings of one read field that disagree. The refusal names both keys and
// neither value.
func (f emptySuffixFold) settle(sink *unmatchedSink) error {
	for _, bare := range slices.Sorted(maps.Keys(f.conflicts)) {
		if piped := f.conflicts[bare]; !sink.reported(piped) {
			return fmt.Errorf("%w: %q and %q spell one bare value twice and disagree; remove one",
				ErrUnknownPath, bare, piped)
		}
	}
	return nil
}

// splitSuffix splits a FLAT key at its trailing pipe attribute — the one place
// that owns the |suffix grammar, shared by grouping and full key parsing so the
// two cannot drift.
func splitSuffix(key string) (base, suffix string) {
	base, suffix, found := strings.CutLast(key, "|")
	if !found {
		return key, ""
	}
	return base, suffix
}

// checkFlatKeys refuses the first key of flat, in sorted order, that is not a
// well-formed FLAT key ([flatKeyMalformation]), with [ErrUnknownPath], naming
// the key and none of its value (REQ-170). Such a key is no path at all, so
// it is refused in every mode rather than set aside as unmatched content.
func checkFlatKeys(flat map[string]any) error {
	for _, key := range slices.Sorted(maps.Keys(flat)) {
		if why := flatKeyMalformation(key); why != "" {
			return fmt.Errorf("%w: %q is not a well-formed FLAT key: %s", ErrUnknownPath, key, why)
		}
	}
	return nil
}

// flatKeyMalformation says how key breaks the FLAT key grammar, or returns ""
// where it keeps it. A key is a path of "/"-separated segments, each a
// non-empty id with an optional ":<index>", then at most one "|<suffix>" that
// holds no "/". A last bare "|" spells the empty suffix ([foldEmptySuffix]),
// so it is set aside before the check.
func flatKeyMalformation(key string) string {
	path, suffix, piped := strings.Cut(strings.TrimSuffix(key, "|"), "|")
	for seg := range strings.SplitSeq(path, "/") {
		switch {
		case seg == "":
			return "it has an empty path segment"
		case strings.HasPrefix(seg, ":"):
			return "a path segment has no id before its :index"
		}
	}
	switch {
	case !piped:
		return ""
	case suffix == "":
		return "it has an empty |suffix before its end"
	case strings.Contains(suffix, "|"):
		return "it has more than one |suffix"
	case strings.Contains(suffix, "/"):
		return "a path segment follows its |suffix"
	}
	return ""
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

// parseFlatKey splits a FLAT key into path segments and the trailing |suffix:
// a trailing "|<attr>" is the leaf attribute suffix, and each "/"-separated
// segment before it is read by [parseFlatSeg]. A segment whose index is not
// spelled canonically is refused, naming the index and the path.
func parseFlatKey(key string) (parsedKey, error) {
	key, suffix := splitSuffix(key)
	segs := make([]flatSeg, 0, strings.Count(key, "/")+1)
	for p := range strings.SplitSeq(key, "/") {
		seg, ok := parseFlatSeg(p)
		if !ok {
			_, idxStr, _ := strings.CutLast(p, ":")
			return parsedKey{}, fmt.Errorf("%w: invalid :index %q in %q", ErrUnknownPath, idxStr, key)
		}
		segs = append(segs, seg)
	}
	return parsedKey{segs: segs, suffix: suffix}, nil
}

// cutAtSeg splits key, a FLAT key [parseFlatKey] accepts, around its path
// segment i, which the key must have: base is the path before that segment,
// and tail everything after it, each later segment with its leading "/", then
// "|suffix" when the suffix is not empty. Both are slices of key, and they are
// the bytes re-spelling the parsed segments with [writeFlatSeg] would build: a
// segment [parseFlatSeg] accepts is its id alone, or its id, ":" and the
// canonical index, so it has one spelling only. A trailing bare "|" is no
// suffix to parseFlatKey, so tail leaves it out.
func cutAtSeg(key string, i int) (base, tail string) {
	path, suffix := splitSuffix(key)
	start := 0
	for range i {
		start += strings.IndexByte(path[start:], '/') + 1
	}
	end := len(path)
	if j := strings.IndexByte(path[start:], '/'); j >= 0 {
		end = start + j
	}
	if start > 0 {
		base = path[:start-1]
	}
	if suffix == "" {
		return base, path[end:]
	}
	return base, key[end:]
}

// parseFlatSeg reads one "/"-separated FLAT path segment, which may carry a
// ":<index>" after its id. A numeric index must be spelled canonically ("0",
// "1", …): a negative index would collide with the internal "no index"
// sentinel, and non-canonical spellings ("-1", "+0", "00") would make distinct
// JSON keys resolve to the same slot, silently overwriting one value with
// another, so ok is false for both. Text after the last ":" that is not a
// number is no index, and stays part of the id.
func parseFlatSeg(p string) (seg flatSeg, ok bool) {
	id, idxStr, found := strings.CutLast(p, ":")
	if !found {
		return flatSeg{id: p, idx: -1}, true
	}
	n, err := strconv.Atoi(idxStr)
	if err != nil {
		return flatSeg{id: p, idx: -1}, true
	}
	if n < 0 || idxStr != strconv.Itoa(n) {
		return flatSeg{}, false
	}
	return flatSeg{id: id, idx: n}, true
}

// contentKey is one FLAT content key with its [parseFlatKey] result: pk when
// the key parses, err when it does not.
type contentKey struct {
	key string
	pk  parsedKey
	err error
}

// parseContentKeys parses each key of content once, in sorted order, for the
// passes that walk content before the leaf loop. A key that does not parse
// keeps its error, and each pass decides what that means for it.
func parseContentKeys(content map[string]any) []contentKey {
	keys := slices.Sorted(maps.Keys(content))
	out := make([]contentKey, len(keys))
	for i, key := range keys {
		pk, err := parseFlatKey(key)
		out[i] = contentKey{key: key, pk: pk, err: err}
	}
	return out
}
