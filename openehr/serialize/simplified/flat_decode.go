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

	"github.com/cadasto/openehr-sdk-go/internal/bmmtype"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/rminfo"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
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
	var names map[string]string
	if cfg.template != nil {
		names = buildNameIndex(cfg.template)
	}
	compJSON, unmatched, err := decodeFlatIgnoring(flat, wt, names, cfg.strict)
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
// FLAT map, driven by the Web Template. names (optional; nil unless a template
// was supplied) maps a node's compiled aqlPath to its LOCATABLE.name. Every
// key it does not read goes to sink, and the decode carries on without it.
func decodeFlat(flat map[string]any, wt *webtemplate.WebTemplate, names map[string]string, sink *unmatchedSink) (map[string]any, error) {
	root := wt.Tree
	compJSON := map[string]any{
		"_type":             "COMPOSITION",
		"archetype_node_id": root.NodeID,
		"name":              textJSON(cmp.Or(root.Name, wt.TemplateID)),
	}
	// A key ending in a bare `|` spells the key without it; both at once are two
	// spellings of one field (REQ-170), judged once the decode knows whether it
	// reads that field.
	flat, fold := foldEmptySuffix(flat)
	if sink != nil {
		sink.spellings = fold.spellings
	}
	// Separate composition-level context from clinical content; context is
	// rebuilt from RM attributes, not from a Web Template leaf path.
	ctx, ctxOrigin, content, err := siphonContext(flat, root.ID, sink)
	if err != nil {
		return nil, err
	}
	budget := &allocBudget{limit: maxTotalNodes}
	ambiguous := ambiguousBarePaths(wt)
	// The reference's in-context keys for RM attributes the Web Template this
	// SDK builds does not model name RM data decode would drop: refused in every
	// mode, before anything could set them aside as unmatched or the
	// WithTemplate defaults replace them (REQ-170).
	if err := refuseInContextKeys(content, wt, ambiguous); err != nil {
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
	compositeGroups, err := compositeLeafGroups(content, wt, sink)
	if err != nil {
		return nil, err
	}
	// Siphon off the underscore-prefixed RM attributes (REQ-140): a path
	// segment starting with `_` ends Web Template resolution at the segment
	// before it, so those keys are grouped per (owner path, family, :index) and
	// routed by the owner's RM class instead of resolving to a leaf. What stays
	// in content is ordinary template-constrained leaf data.
	attrGroups, err := rmattrGroups(content, sink)
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
	// Process leaf groups in a stable (sorted) order so the reconstructed tree
	// is deterministic: distinct-node-id siblings with no explicit :index
	// (e.g. multiple content items, or elements under one ITEM_TREE) are
	// appended in this order, which must not depend on Go map iteration.
	for _, base := range slices.Sorted(maps.Keys(groups)) {
		keyOf := func(suffix string) string { return keys[base][suffix] }
		pk, err := parseFlatKey(base)
		if err != nil {
			return nil, err
		}
		leaf, predIndex, predType, err := resolveLeaf(wt, pk.segs, ambiguous)
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
		sfx, err := leafSuffixes(leaf, groups[base], keyOf, sink)
		if err != nil {
			return nil, err
		}
		if len(sfx) == 0 {
			continue
		}
		dv, err := leafFromSuffixes(nodeRMType(leaf), leafListOpen(leaf), sfx)
		if err != nil {
			return nil, fmt.Errorf("simplified: decode %q: %w", base, err)
		}
		if err := placeLeaf(compJSON, leaf.AQLPath, predIndex, predType, dv, budget, names); err != nil {
			return nil, fmt.Errorf("simplified: place %q: %w", base, err)
		}
	}
	// Composite leaves are placed like any other leaf, after the clinical loop so a
	// malformed ordinary key still surfaces first.
	for _, g := range compositeGroups {
		if err := placeCompositeLeaf(compJSON, wt, g, ambiguous, budget, names); err != nil {
			return nil, err
		}
	}
	// Underscore RM attributes come after the clinical leaves, so their owners
	// are decorated onto nodes the content walk has already materialised rather
	// than gap-filling their own (REQ-140).
	if err := decodeRMAttrGroups(compJSON, wt, attrGroups, ambiguous, budget, names); err != nil {
		return nil, err
	}
	// Every key is placed, so each rebuilt ISM transition can now get the careflow
	// step its node names, or have the body's checked against it (REQ-053).
	if err := settleIsmTransitions(compJSON, wt); err != nil {
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
	if err := applyContext(compJSON, ci); err != nil {
		return nil, err
	}
	// Conformant mode (a template was supplied, so names is non-nil): fill the
	// RM-mandatory attributes the formats do not carry, from ctx defaults + RM
	// conventions, so the decoded composition is OPT-validatable.
	if names != nil {
		completeRequired(compJSON, ci)
	}
	// Both modes: archetype_details has no FLAT key, and the Web Template
	// alone holds what it records (REQ-053).
	rebuildArchetypeDetails(compJSON, wt)
	if err := fold.settle(sink); err != nil {
		return nil, err
	}
	return compJSON, nil
}

// leafSuffixes returns the suffixes of one leaf group that the decode reads,
// handing the key of every other one to sink (REQ-170). keyOf gives the key a
// suffix arrived under.
//
//   - A node with children is structure, not a value: the decode reads a |raw
//     fragment there and its bare value, which names the node and so is
//     matched content of the wrong shape, refused where the value is built
//     (REQ-170). Every other suffix is unmatched (an extension suffix such as
//     `|x_note`, say). The RM attributes the reference spells as a suffix on
//     such a node never get here: [refuseInContextKeys] refuses them first.
//   - A leaf whose datatype has a suffix set keeps the suffixes in that set
//     ([leafSuffixMatched]); the rest are unmatched.
//   - A leaf whose datatype has neither a suffix set nor a sub-path grammar
//     resolves but has no carrier other than |raw: in practice the composer's
//     PARTY_PROXY, whose party grammar the ctx/ forms own. Its bare value, and
//     a suffix that names something the datatype has, are refused in every
//     mode with ErrUnsupportedDatatype, as the other carrier-less keys at a
//     leaf are, naming the first such key: an attribute of its RM class, or
//     for a party the party grammar's suffixes (the composer's `external_ref`
//     among them). Any other suffix is unmatched (REQ-170).
//   - An unparameterised `DV_INTERVAL` leaf has a sub-path grammar, the
//     interval grammar, but names no bound datatype to read it with, so it
//     reads its |raw fragment alone; every other key is unmatched.
func leafSuffixes(leaf *webtemplate.Node, sfx map[string]any, keyOf func(string) string, sink *unmatchedSink) (map[string]any, error) {
	if len(leaf.Children) > 0 {
		var out map[string]any
		for _, suffix := range slices.Sorted(maps.Keys(sfx)) {
			if suffix == "raw" || suffix == "" {
				continue
			}
			if out == nil {
				out = maps.Clone(sfx)
			}
			delete(out, suffix)
			sink.add(keyOf(suffix))
		}
		if out == nil {
			return sfx, nil
		}
		return out, nil
	}
	rmType := nodeRMType(leaf)
	if _, judged := leafSuffixMatched(rmType, ""); judged {
		return matchedSuffixes(rmType, sfx, keyOf, sink), nil
	}
	subPathGrammar := bmmtype.Class(rmType) == "DV_INTERVAL"
	var out map[string]any
	for _, suffix := range slices.Sorted(maps.Keys(sfx)) {
		switch {
		case suffix == "raw":
			// |raw is the carrier of an unmapped datatype; dvFromSuffixes refuses
			// it beside any other suffix.
		case !subPathGrammar && (suffix == "" || unmappedReads(rmType, suffix)):
			if _, raw := sfx["raw"]; raw {
				break
			}
			if suffix == "" {
				// Named as the allowlist refusals name a bare value, so a caller
				// reading the refusal removes the bare key alone.
				return nil, fmt.Errorf("simplified: decode %q: %w: unexpected bare value for %s", keyOf(suffix), ErrUnsupportedDatatype, rmType)
			}
			return nil, fmt.Errorf("simplified: decode %q: %w: %s", keyOf(suffix), ErrUnsupportedDatatype, rmType)
		default:
			if out == nil {
				out = maps.Clone(sfx)
			}
			delete(out, suffix)
			sink.add(keyOf(suffix))
		}
	}
	if out == nil {
		return sfx, nil
	}
	return out, nil
}

// unmappedReads reports whether suffix names something a leaf of rmType, a
// datatype with neither a suffix set nor a sub-path grammar, has: for a party,
// a suffix of the party grammar (the composer's `external_ref` suffixes and
// `|name`); otherwise an attribute of its RM class.
func unmappedReads(rmType, suffix string) bool {
	if isPartyLeafType(rmType) {
		return partyProxyOwnTails[suffix]
	}
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
// One composition-level family deliberately stays out, because admitting it
// would be adding a field rather than accepting a spelling: `composer|id` /
// `|id_scheme` / `|id_namespace` — the composer's external_ref, which the ctx/
// short forms structurally cannot carry. Silently dropping it would violate
// REQ-053's semantics-preserving contract, so those suffixes stay refused and
// appear in the PROBE-086 census (see deviations.md and
// docs/specifications/conformance.md § PROBE-086).
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
// `composer` is deliberately absent: the composer external_ref suffixes
// (`composer|id` / `|id_scheme` / `|id_namespace`) must keep flowing through to
// decode so they are refused there and counted in the PROBE-086 census as the
// real PARTY_PROXY data loss they are (ADR 0015). Every base here is held out of
// that census by the harness (`IsCompositionMeta`), so refusing extra suffixes
// early moves no count.
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

// ctxSettingBare is the bare spelling of ctx/setting, which the decode reads as
// a pair of suffixes only, and ctxSettingTerminology the witness of the
// terminology that pair implies.
const (
	ctxSettingBare        = "ctx/setting"
	ctxSettingTerminology = "ctx/setting|terminology"
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
// ctx/-owned base, plus `composer` for its party sub-structure. Each names a
// single-valued RM attribute (COMPOSITION.language, .composer, .context,
// EVENT_CONTEXT.start_time, …). The set is read off those tables, so an alias
// added there brings its segments with it.
var metadataSegPaths = func() map[string]bool {
	paths := map[string]bool{"composer": true}
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
// unchanged, for the leaf loop to refuse.
func metadataRel(rel string) (norm string, overIndexed bool) {
	path, suffix, piped := strings.Cut(rel, "|")
	pk, err := parseFlatKey(path)
	if err != nil {
		return rel, false
	}
	var b strings.Builder
	var seen string
	onMeta := true
	for i, s := range pk.segs {
		if i > 0 {
			b.WriteByte('/')
		}
		if onMeta {
			p := s.id
			if seen != "" {
				p = seen + "/" + s.id
			}
			if metadataSegPaths[p] {
				seen = p
				b.WriteString(s.id)
				overIndexed = overIndexed || s.idx > 0
				continue
			}
			onMeta = false
		}
		writeFlatSeg(&b, s)
	}
	if piped {
		b.WriteByte('|')
		b.WriteString(suffix)
	}
	return b.String(), overIndexed
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

// composerSubStructure reports whether sub, the path below `composer/`, opens
// with a segment of the party grammar's sub-structure: the nested
// `_identifier:N` list or PARTY_RELATED's `relationship`.
func composerSubStructure(sub string) bool {
	seg := sub
	if cut := strings.IndexAny(seg, "/|"); cut >= 0 {
		seg = seg[:cut]
	}
	id, _, _ := strings.Cut(seg, ":")
	return partyListTails[id] || partySubTails[id]
}

// isMetadataSpelling reports whether an unindexed root-relative spelling is one
// [siphonContext] handles itself: an alias, a terminology witness, a suffix of
// a leaf the ctx/ short form owns, or the composer's party sub-structure.
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
	return strings.HasPrefix(rel, "composer/")
}

// cutRoot splits a FLAT key at its template-root segment and returns the path
// below it. The root segment may carry an explicit index (`<root>:0/…`, the
// form REQ-053 reads as `<root>/…`), so it is parsed rather than matched as a
// literal prefix; rootSeg reports the index it carries. rooted is false for a
// key whose first segment is not the root, has nothing below it, or carries a
// malformed index (the leaf loop refuses that one).
func cutRoot(key, rootID string) (rel string, rootSeg flatSeg, rooted bool) {
	first, rel, found := strings.Cut(key, "/")
	if !found {
		return "", flatSeg{}, false
	}
	pk, err := parseFlatKey(first)
	if err != nil || pk.suffix != "" || len(pk.segs) != 1 || pk.segs[0].id != rootID {
		return "", flatSeg{}, false
	}
	return rel, pk.segs[0], true
}

// rootSegmentAlone parses a key that has no `/` as one segment and its
// suffix, and reports the segment when it is the template root's.
func rootSegmentAlone(key, rootID string) (flatSeg, bool) {
	if strings.Contains(key, "/") {
		return flatSeg{}, false
	}
	pk, err := parseFlatKey(key)
	if err != nil || len(pk.segs) != 1 || pk.segs[0].id != rootID {
		return flatSeg{}, false
	}
	return pk.segs[0], true
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
		curType = concreteType(curType, seg.attr, "", rel[i+1].attr)
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
// outside that leaf's datatype, go to sink as unmatched content (REQ-170).
func siphonContext(flat map[string]any, rootID string, sink *unmatchedSink) (ctx map[string]any, ctxOrigin map[string]string, content map[string]any, err error) {
	ctx, ctxOrigin, content = make(map[string]any), make(map[string]string), make(map[string]any)
	// Sorted so a body carrying two conflicting real-path spellings reports the
	// same key first on every run — a map-order-dependent error message is not
	// reproducible for whoever has to fix the payload.
	for _, key := range slices.Sorted(maps.Keys(flat)) {
		val := flat[key]
		if strings.HasPrefix(key, "ctx/") {
			if key == ctxSettingBare {
				// ctx/setting is read as its |code + |value pair; a bare value is
				// the field missing both (REQ-170: a missing required suffix).
				return nil, nil, nil, fmt.Errorf("%w: %q spells the setting without its |code and |value, the pair ctx/setting travels as",
					ErrUnsupportedDatatype, key)
			}
			if key == ctxSettingTerminology {
				// The setting's terminology is implied, never carried: a witness
				// naming it is read and dropped, and one naming any other
				// terminology is a typed error, not a silent rewrite (REQ-053),
				// exactly as the real-path witness below.
				if err := checkTerminologyWitness(key, val, terminology.ID); err != nil {
					return nil, nil, nil, err
				}
				continue
			}
			if !ctxFields[key] {
				sink.add(key)
				continue
			}
			if err := putCtx(ctx, ctxOrigin, key, val, key); err != nil {
				return nil, nil, nil, err
			}
			continue
		}
		// A key that is the root segment alone, with or without a suffix,
		// is judged from the left like any other: an index past :0 on it is
		// past its bound (REQ-170).
		if seg, alone := rootSegmentAlone(key, rootID); alone && seg.idx > 0 {
			return nil, nil, nil, fmt.Errorf("simplified: %q: %w", key, errRootIndex(seg))
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
			return nil, nil, nil, fmt.Errorf("simplified: %q: %w", key, errRootIndex(rootSeg))
		}
		// The tables below are keyed on the unindexed spelling. An indexed
		// metadata path is read exactly as that spelling, and only `:0` is an
		// index a single-valued attribute has (REQ-053), so any other is
		// refused. A key that is no metadata spelling once its `:0` is gone
		// keeps flowing as content under the key it arrived with.
		raw := rel
		rel, overIndexed := metadataRel(rel)
		if overIndexed && isMetadataSpelling(rel) {
			return nil, nil, nil, fmt.Errorf("%w: %q (a composition metadata path runs through single-valued attributes, so :0 is the only index it takes)",
				ErrUnknownPath, key)
		}
		// Any other key that runs through a metadata segment past :0 is past
		// that segment's bound whatever follows it, whether or not the Web
		// Template carries a node for the segment (REQ-170).
		if overIndexed {
			return nil, nil, nil, fmt.Errorf("simplified: %q: %w", key, errSingleValuedIndex(overIndexedMetadataSeg(raw)))
		}
		if want, isWitness := metadataAliasTerminology[rel]; isWitness {
			if err := checkTerminologyWitness(key, val, want); err != nil {
				return nil, nil, nil, err
			}
			continue
		}
		if ctxKey, isAlias := metadataAliases[rel]; isAlias {
			if err := putCtx(ctx, ctxOrigin, ctxKey, val, key); err != nil {
				return nil, nil, nil, err
			}
			continue
		}
		// An EVENT_CONTEXT leaf the ctx/ short form owns outright cannot also
		// travel as an ordinary Web Template leaf: the ctx/ entry is written last
		// and would overwrite whatever the leaf placed, dropping it with no error.
		// Every spelling of such a leaf is therefore either an accepted alias, a
		// checked witness, or refused here naming the key (REQ-053) — the
		// composer external_ref suffixes stay out of this set deliberately, so
		// they keep flowing to decode and stay visible in the PROBE-086 census.
		// A suffix the leaf's datatype does not have at all is not such a
		// spelling: the decode does not read it, so it is unmatched (REQ-170).
		// A bare value is not such a suffix but a spelling of the field missing
		// its required suffixes: refused, as below.
		if base, suffix, _ := strings.Cut(rel, "|"); contextMetaOwnedBases[base] {
			if matched, _ := leafSuffixMatched(metadataOwnedBaseType[base], suffix); !matched && suffix != "" {
				sink.add(key)
				continue
			}
			return nil, nil, nil, fmt.Errorf("%w: %q is not a spelling the ctx/ short form can carry — %s owns this leaf, so only its accepted suffixes travel (see deviations.md)",
				ErrUnsupportedDatatype, key, metadataOwnedBaseCtx[base])
		}
		// The ADR 0015 boundary, REQ-140 edition. REQ-140's party grammar reaches
		// a party's sub-structure — its `_identifier:N` list and, for a
		// PARTY_RELATED, `/relationship` — everywhere except here: no `ctx/` short
		// form can carry it, so a composer decoded from those keys could not be
		// re-emitted (encode is ctx/-only) and the value would vanish on the way
		// out. That is the silent loss REQ-053 forbids, so the keys are refused by
		// name and counted in the PROBE-086 census, exactly as the composer's own
		// `|id` / `|id_scheme` / `|id_namespace` external_ref suffixes are (those
		// reach the leaf loop and are refused there as PARTY_PROXY). Closing this
		// means giving the composer's reference a `ctx/` carrier, which is an
		// ADR-level decision, not a codec one.
		// Only the party grammar's sub-structure is the composer's: any other
		// sub-path names nothing a party has, and is unmatched content where the
		// leaf loop finds no node for it.
		if sub, isComposerSub := strings.CutPrefix(rel, "composer/"); isComposerSub && composerSubStructure(sub) {
			return nil, nil, nil, fmt.Errorf("%w: %q is composer party sub-structure, which no ctx/ short form can carry (ADR 0015 boundary)",
				ErrUnsupportedDatatype, key)
		}
		content[key] = val
	}
	return ctx, ctxOrigin, content, nil
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
		return fmt.Errorf("%w: %s = %q, but the ctx/ form implies %q (a differing terminology cannot be carried)",
			ErrUnsupportedDatatype, key, got, want)
	}
	return nil
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
		return fmt.Errorf("%w: conflicting spellings of %s: %q gives %#v, %q gives %#v; remove one",
			ErrUnknownPath, ctxKey, ctxSpelling(origin, ctxKey), prev, bodyKey, val)
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
	// enforces that the two arrive together, so applyContext keys on
	// haveSettingCode alone.
	settingCode, settingValue         string
	haveSettingCode, haveSettingValue bool
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
	// ctx/setting travels as a pair: the rubric is not derivable from the code
	// without a terminology service, and a code is not derivable from the rubric
	// at all, so half of it cannot be completed — it is refused naming the
	// missing key (REQ-053). The same shape requireSuffix enforces for a leaf's
	// mandatory suffixes.
	if ci.haveSettingCode != ci.haveSettingValue {
		present, missing := "ctx/setting|code", "ctx/setting|value"
		if !ci.haveSettingCode {
			present, missing = "ctx/setting|value", "ctx/setting|code"
		}
		// Both halves are named in the spelling the payload used: telling a
		// real-path author to add a `ctx/`-spelled key they never wrote sends them
		// to the wrong place. ctxSpelling falls back to the ctx/ form when the
		// missing key has no origin, which is exactly right — it was not written.
		return ci, fmt.Errorf("%w: %s without %s — the ctx/setting pair travels together",
			ErrUnsupportedDatatype, ctxSpelling(origin, present), ctxSpelling(origin, missing))
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
	// PARTY_IDENTIFIED is not the EHR subject. [applyContext] can only build one,
	// and its switch prefers PARTY_SELF — so accepting the pair would drop the
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
	if ci.haveTime || ci.haveSettingCode {
		// Merge into any EVENT_CONTEXT already reconstructed from clinical paths
		// (other_context, …) rather than replacing it — otherwise that data would
		// be lost.
		ctxObj, _ := compJSON["context"].(map[string]any)
		if ctxObj == nil {
			ctxObj = map[string]any{"_type": "EVENT_CONTEXT"}
			compJSON["context"] = ctxObj
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
		// rminfo knows each class by its bare BMM name; a caller's |raw fragment
		// may spell a generic instantiation (DV_INTERVAL<DV_QUANTITY>).
		for _, attr := range rminfo.Default.RequiredAttributes(bmmtype.Class(t)) {
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
		return ctxCodedDefault(terminology.Setting, "238")
	case "category":
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
		// Placement reads an index only where the attribute is a list, so on a
		// single-valued one an index past :0 would be dropped and the key merged
		// into the one value (REQ-170).
		if seg.idx > 0 && holdsOneValue(node, next) {
			return nil, nil, nil, errSingleValuedIndex(seg)
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
// attribute to the leaf value: a DataValue, or the RM String of a STRING leaf.
//
// Reconstructed intermediate and leaf nodes carry _type + archetype_node_id;
// when names is non-nil (a template was supplied via WithTemplate) the mandatory
// LOCATABLE.name is set from it, keyed by the node's compiled aqlPath — which
// the walk reconstructs by keeping a predicate only on container attributes
// (matching templatecompile's path convention). Without names, nodes are left
// unnamed (rmpath re-resolves by archetype_node_id, so the round-trip does not
// depend on it — but the result is then format-idempotent, not canonically
// complete; see deviations.md).
func placeLeaf(compJSON map[string]any, aqlPath string, predIndex map[string]int, predType map[string]string, value any, budget *allocBudget, names map[string]string) error {
	cur, attr, err := walkAQL(compJSON, aqlPath, predIndex, predType, budget, names)
	if err != nil {
		return err
	}
	if _, exists := cur[attr]; exists {
		// Two FLAT keys resolved to the same terminal slot (e.g. "a" vs
		// "a:0" on a repeatable) — overwriting would silently drop one.
		return fmt.Errorf("%w: duplicate placement at %q", ErrUnknownPath, aqlPath)
	}
	cur[attr] = value
	return nil
}

// walkAQL materialises the intermediate RM nodes along aqlPath and returns the
// object that holds its **final** segment, together with that segment's
// attribute name — the seam [placeLeaf] and the REQ-140 attribute router share.
// A leaf's terminal attribute is the datatype slot (`…/items[at0008]/value`); an
// underscore family's is the RM attribute it addresses
// (`…/items[at0008]/uid`), which is also the lookahead [concreteType] needs to
// resolve the abstract ITEM_STRUCTURE slot the Web Template collapses.
func walkAQL(compJSON map[string]any, aqlPath string, predIndex map[string]int, predType map[string]string, budget *allocBudget, names map[string]string) (map[string]any, string, error) {
	segs := parseAQL(aqlPath)
	if len(segs) == 0 {
		return nil, "", fmt.Errorf("%w: empty canonical path", ErrUnknownPath)
	}
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
			return cur, seg.attr, nil
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
			return nil, "", fmt.Errorf("%w: cannot resolve RM type for %q on %s (aqlPath %q)", ErrUnknownPath, seg.attr, curType, aqlPath)
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
				return nil, "", err
			}
			cur = next
		} else {
			obj, ok := cur[seg.attr].(map[string]any)
			if held, present := cur[seg.attr]; present && !ok {
				// The slot already holds a scalar (a STRING leaf, a bare
				// attribute): a second key reaching the same single-valued
				// attribute through a deeper node would replace it with an
				// object and drop the first value (REQ-053).
				return nil, "", fmt.Errorf("%w: %s.%s already holds a %T, but keys reach it through a second Web Template node — refused rather than overwritten (aqlPath %q)",
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
				cur[seg.attr] = obj
			} else if held, _ := obj["archetype_node_id"].(string); held != seg.pred {
				// Two Web Template nodes stand for this one single-valued
				// attribute (an ACTION's ism_transition[at0005] and [at0006]).
				// Merging their keys into one object would re-encode them under
				// one node only, moving the other's keys there (REQ-053).
				return nil, "", fmt.Errorf("%w: %s.%s is single-valued, but keys reach it through two Web Template nodes, [%s] and [%s] — refused rather than merged (aqlPath %q)",
					ErrUnknownPath, curType, seg.attr, held, seg.pred, aqlPath)
			}
			cur = obj
		}
		// Only a LOCATABLE has a name. The template's name index also covers an
		// archetyped ISM_TRANSITION node, which must not gain one.
		if nm := names[namePrefix.String()]; nm != "" && nameable(childType) {
			if _, has := cur["name"]; !has {
				cur["name"] = textJSON(nm)
			}
		}
		curType = childType
	}
	// Unreachable: the loop returns at i == len(segs)-1 and segs is non-empty.
	return nil, "", fmt.Errorf("%w: canonical path %q walked past its last segment", ErrUnknownPath, aqlPath)
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

// ismTransitionNodes indexes the Web Template's archetyped ISM_TRANSITION nodes
// by the bare spelling of their canonical path, which is the path
// [settleIsmTransitions] rebuilds from the decoded tree.
func ismTransitionNodes(wt *webtemplate.WebTemplate) map[string]ismNode {
	out := make(map[string]ismNode)
	var walk func(n *webtemplate.Node, flat string)
	walk = func(n *webtemplate.Node, flat string) {
		for _, ch := range n.Children {
			chFlat := flat + "/" + ch.ID
			if ch.NodeID != "" && nodeRMType(ch) == "ISM_TRANSITION" {
				out[bareAQLPath(ch.AQLPath)] = ismNode{id: ch.NodeID, name: ch.Name, flat: chFlat}
			}
			walk(ch, chFlat)
		}
	}
	if wt.Tree != nil {
		walk(wt.Tree, wt.Tree.ID)
	}
	return out
}

// settleIsmTransitions finishes each ISM_TRANSITION the placement walk rebuilt
// at an archetyped `ism_transition[atNNNN]` node, once every key is placed.
//
// Encode matches a transition to its node by careflow step code, because an
// ISM_TRANSITION is not LOCATABLE and has no archetype_node_id (REQ-121). So a
// transition without a careflow step gets the one its node names: the node id as
// a `local` code, the node's Web Template name as its value. A careflow step the
// body carries must be coded with the node id; one coded otherwise belongs to
// another node, and encode would move the transition there (REQ-053). The node id
// [walkAQL] recorded on the object is removed here, so it never reaches canjson.
//
// The walk rebuilds each object's bare canonical path from the archetype_node_id
// values placement wrote, which is how [walkAQL] spells the same path.
func settleIsmTransitions(compJSON map[string]any, wt *webtemplate.WebTemplate) error {
	nodes := ismTransitionNodes(wt)
	if len(nodes) == 0 {
		return nil
	}
	return settleIsmTransitionsUnder(compJSON, "", nodes)
}

// settleIsmTransitionsUnder walks obj, whose bare canonical path is path, in
// sorted attribute order so a body with two bad transitions names the same one
// on every run.
func settleIsmTransitionsUnder(obj map[string]any, path string, nodes map[string]ismNode) error {
	for _, attr := range slices.Sorted(maps.Keys(obj)) {
		switch v := obj[attr].(type) {
		case map[string]any:
			p := path + "/" + attr + nodePredicate(v)
			if n, ok := nodes[p]; ok && v["_type"] == "ISM_TRANSITION" {
				if err := settleIsmTransition(v, n); err != nil {
					return err
				}
				continue
			}
			if err := settleIsmTransitionsUnder(v, p, nodes); err != nil {
				return err
			}
		case []any:
			for _, e := range v {
				m, ok := e.(map[string]any)
				if !ok {
					continue
				}
				if err := settleIsmTransitionsUnder(m, path+"/"+attr+nodePredicate(m), nodes); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// nodePredicate is the `[id]` a rebuilt node adds to its canonical path, or ""
// for a node placed without one.
func nodePredicate(m map[string]any) string {
	if id, ok := m["archetype_node_id"].(string); ok && id != "" {
		return "[" + id + "]"
	}
	return ""
}

// settleIsmTransition gives one rebuilt ISM_TRANSITION the careflow step its node
// names, or checks the one the body carries. The node id is an at-code, which
// belongs to the archetype's `local` terminology: a careflow step that names no
// terminology (no terminology_id, a null one, or a blank value) takes it, and one
// coded in any other terminology, or whose terminology_id has another shape,
// names no node.
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
		return fmt.Errorf("%w: %s/careflow_step is coded %q, but its node is ism_transition[%s]; a careflow step coded otherwise names another node",
			ErrUnsupportedDatatype, n.flat, code, n.id)
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
			return fmt.Errorf("%w: %s/careflow_step is coded in terminology %q, but its node id %s is an at-code of the %s terminology",
				ErrUnsupportedDatatype, n.flat, term, n.id, localTerminology)
		}
	default:
		return fmt.Errorf("%w: %s/careflow_step carries a terminology_id of JSON type %T, not a TERMINOLOGY_ID object",
			ErrUnsupportedDatatype, n.flat, tid)
	}
	return nil
}

// localTerminology is the terminology an archetype's own at-codes belong to.
const localTerminology = "local"

// The reference's in-context spelling of RM attributes the Web Template this SDK
// builds does not model, which REQ-170 refuses in every mode: an ACTION's ISM
// transition (a `current_state`, `transition` or `careflow_step` leaf or a
// `_reason` family under an ISM transition node) and its own `time` leaf, an
// INTERVAL_EVENT's `math_function`, `width` and `sample_count`, and an
// OBSERVATION's `history_origin`. ismReferenceSeg is the FLAT id the reference
// gives the ISM transition node when it writes it in context.
const (
	ismReasonFamily = "_reason"
	ismReferenceSeg = "ism_transition"
)

// ismLeaves are the ISM_TRANSITION leaves the reference writes under its ISM
// transition node, by FLAT id.
var ismLeaves = map[string]bool{"current_state": true, "transition": true, "careflow_step": true}

// intervalEventAttrs are the INTERVAL_EVENT attributes the reference writes in
// context.
var intervalEventAttrs = map[string]bool{"math_function": true, "width": true, "sample_count": true}

// inContextAttrs names, per Web Template node RM type, the RM attributes the
// reference writes in context under such a node and REQ-170 refuses where the
// Web Template does not resolve them. leaf holds those spelled as a leaf under
// the node, suffix those also spelled as a suffix on it, and owner is the RM
// class the message names. An EVENT node is the abstract slot a Web Template
// gives an event; the attributes are INTERVAL_EVENT's, and a body that spells
// one is an interval event.
var inContextAttrs = map[string]struct {
	owner        string
	leaf, suffix map[string]bool
}{
	"ACTION":         {owner: "ACTION", leaf: map[string]bool{"time": true}},
	"EVENT":          {owner: "INTERVAL_EVENT", leaf: intervalEventAttrs, suffix: intervalEventAttrs},
	"INTERVAL_EVENT": {owner: "INTERVAL_EVENT", leaf: intervalEventAttrs, suffix: intervalEventAttrs},
	"OBSERVATION": {
		owner: "OBSERVATION", leaf: map[string]bool{"history_origin": true},
		suffix: map[string]bool{"history_origin": true},
	},
}

// inContextCite names the section a refusal of an in-context attribute cites:
// REQ-140's fourth out-of-scope class lists an ISM transition's `transition`
// and `reason`; REQ-170 alone binds the rest (`current_state`,
// `careflow_step`, the ACTION's `time`, and the INTERVAL_EVENT and OBSERVATION
// attributes).
func inContextCite(attr string) string {
	switch attr {
	case "ISM transition transition", "ISM transition reason":
		return "REQ-140"
	}
	return "REQ-170"
}

// refuseInContextKeys refuses, in every mode, the reference's in-context
// spelling of the RM attributes in [inContextAttrs] and of an ACTION's ISM
// transition, wherever the Web Template does not resolve it (REQ-170; REQ-140's
// fourth out-of-scope class covers `transition` and `reason`):
//
//   - a `current_state`, `transition` or `careflow_step` leaf, with any suffix,
//     under an ISM transition node: the reference's in-context `ism_transition`
//     node, which this SDK's Web Template does not project, or an archetyped
//     `ism_transition[atNNNN]` node that has no child of that id (the Web
//     Template models `current_state` and `careflow_step` there, never
//     `transition`);
//   - a `_reason` family under either node;
//   - the ACTION's own `time` leaf, with any suffix;
//   - an INTERVAL_EVENT's `math_function`, `width` or `sample_count`, and an
//     OBSERVATION's `history_origin`, each as a leaf under the node, with any
//     suffix, or as a suffix on the node.
//
// Each names RM data the decode would otherwise drop, or replace with a
// default, so it is refused rather than ignored. A key the Web Template
// resolves is decoded as usual, and any other unresolved key under those nodes
// is unmatched content. A key that walks through one of several reused siblings
// is left to the refusal that path gets.
//
// Keys are checked in sorted order, so a body with several names the same one on
// every run. The error names the key, or the `_reason` family instance, and
// carries no value.
func refuseInContextKeys(content map[string]any, wt *webtemplate.WebTemplate, ambiguous map[string]bool) error {
	for _, key := range slices.Sorted(maps.Keys(content)) {
		pk, err := parseFlatKey(key)
		if err != nil {
			// A malformed index is refused where the key is decoded.
			continue
		}
		named, owner, attr, ok := inContextKey(wt, key, pk, ambiguous)
		if !ok {
			continue
		}
		return fmt.Errorf("%w: %q is the reference's in-context spelling of an %s's %s, which the Web Template this SDK builds does not model — refused rather than dropped or replaced (%s, see deviations.md)",
			ErrUnknownPath, named, owner, attr, inContextCite(attr))
	}
	return nil
}

// inContextKey reports whether key, parsed as pk, is one of the key shapes
// [refuseInContextKeys] refuses: named is what the refusal names (the key, or
// the family instance for `_reason`), owner the RM class and attr the
// attribute it spells.
func inContextKey(wt *webtemplate.WebTemplate, key string, pk parsedKey, ambiguous map[string]bool) (named, owner, attr string, ok bool) {
	segs := pk.segs
	node := wt.Tree
	if len(segs) < 2 || segs[0].id != node.ID {
		return "", "", "", false
	}
	for i := 1; i < len(segs); i++ {
		child := childByID(node, segs[i].id)
		rmType := nodeRMType(node)
		attrs := inContextAttrs[rmType]
		last := i == len(segs)-1
		if last && child == nil && attrs.leaf[segs[i].id] {
			return key, attrs.owner, segs[i].id, true
		}
		if rmType == "ACTION" && !last && isIsmTransitionSeg(child, segs[i].id) {
			named, attr, ok := ismAttrKey(key, segs, i, child)
			return named, "ACTION", attr, ok
		}
		if child == nil || (child.NodeID != "" && ambiguous[bareAQLPath(child.AQLPath)]) {
			return "", "", "", false
		}
		node = child
	}
	if attrs := inContextAttrs[nodeRMType(node)]; attrs.suffix[pk.suffix] && childByID(node, pk.suffix) == nil {
		return key, attrs.owner, pk.suffix, true
	}
	return "", "", "", false
}

// ismAttrKey judges the key below the ISM transition segment segs[i], whose Web
// Template node is ism (nil for the reference's in-context node): an
// ISM_TRANSITION leaf the node does not model, or a `_reason` family.
func ismAttrKey(key string, segs []flatSeg, i int, ism *webtemplate.Node) (named, attr string, ok bool) {
	rest := segs[i+1:]
	switch {
	case len(rest) == 1 && ismLeaves[rest[0].id] && (ism == nil || childByID(ism, rest[0].id) == nil):
		return key, "ISM transition " + rest[0].id, true
	case rest[0].id == ismReasonFamily:
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
// Keys are consumed from content: what stays behind is ordinary leaf data. Each
// group carries sink, which takes the keys its grammar does not read.
func compositeLeafGroups(content map[string]any, wt *webtemplate.WebTemplate, sink *unmatchedSink) ([]rmattrGroup, error) {
	byLeaf := make(map[string]*rmattrGroup)
	for _, key := range slices.Sorted(maps.Keys(content)) {
		pk, err := parseFlatKey(key)
		if err != nil {
			return nil, err
		}
		base, family, index, tail, isComposite := splitCompositeLeafKey(wt, pk)
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

// splitCompositeLeafKey splits a parsed FLAT key at the composite Web Template
// leaf it addresses, if any: the first segment run that resolves to a childless
// node of a party or DV_INTERVAL RM type. base is the leaf's parent path, family
// and index the leaf segment itself, and tail everything after it (each remaining
// segment with a leading "/", then "|suffix").
//
// Two kinds of key are deliberately not composite leaves. One whose path reaches
// an `_`-prefixed segment first belongs to that family — a `_feeder_audit`'s
// nested `/subject` is inside its tails, not at a Web Template node — and one
// addressing a `ctx/`-owned metadata leaf is the composer, whose party
// sub-structure is the ADR 0015 refusal in [siphonContext] and whose own suffixes
// stay a PARTY_PROXY leaf refusal.
func splitCompositeLeafKey(wt *webtemplate.WebTemplate, pk parsedKey) (base, family string, index int, tail string, ok bool) {
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
		var b, t strings.Builder
		for j, s := range pk.segs[:i] {
			if j > 0 {
				b.WriteByte('/')
			}
			writeFlatSeg(&b, s)
		}
		for _, s := range pk.segs[i+1:] {
			t.WriteByte('/')
			writeFlatSeg(&t, s)
		}
		if pk.suffix != "" {
			t.WriteByte('|')
			t.WriteString(pk.suffix)
		}
		return b.String(), seg.id, seg.idx, t.String(), true
	}
	return "", "", 0, "", false
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
func placeCompositeLeaf(compJSON map[string]any, wt *webtemplate.WebTemplate, g rmattrGroup,
	ambiguous map[string]bool, budget *allocBudget, names map[string]string,
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
	node, predIndex, predType, err := resolveLeaf(wt, pk.segs, ambiguous)
	if err != nil {
		// splitCompositeLeafKey resolved this path already, so the only ways
		// here are an index past the bound of a single-valued attribute and the
		// reused-sibling refusal, each with its own message.
		return fmt.Errorf("simplified: %q: %w", g.prefix(), err)
	}
	value, err := compositeLeafValue(g, nodeRMType(node))
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
	if err := placeLeaf(compJSON, node.AQLPath, predIndex, predType, value, budget, names); err != nil {
		return fmt.Errorf("simplified: place %q: %w", g.prefix(), err)
	}
	return nil
}

// compositeLeafValue decodes a composite leaf's group by its RM type: the party
// grammar (rmattr_party.go) or the interval grammar (rmattr_value.go), each the
// same implementation those families use at every other position.
func compositeLeafValue(g rmattrGroup, rmType string) (map[string]any, error) {
	if anchor, isInterval := intervalLeafAnchor(rmType); isInterval {
		return intervalLeafSuffixes(g, anchor)
	}
	party, populated, err := partyLeafSuffixes(g)
	if err != nil {
		return nil, err
	}
	if !populated {
		return nil, fmt.Errorf("%w: %s carries no party key (PARTY_IDENTIFIED needs at least one of |name, |id or an _identifier)",
			ErrUnsupportedDatatype, g.prefix())
	}
	return party, nil
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
//     it has no underscore owner and the key is unresolvable.
func rmattrOwnerAt(wt *webtemplate.WebTemplate, compJSON map[string]any, base string,
	ambiguous map[string]bool, budget *allocBudget, names map[string]string,
) (rmattrOwner, error) {
	pk, err := parseFlatKey(base)
	if err != nil {
		return rmattrOwner{}, err
	}
	segs := pk.segs
	if len(segs) == 0 || segs[0].id != wt.Tree.ID {
		return rmattrOwner{}, errSegNotFound
	}
	walk := func(ownerAql string, predIndex map[string]int, predType map[string]string) func(string) (map[string]any, error) {
		return func(attr string) (map[string]any, error) {
			node, _, err := walkAQL(compJSON, ownerAql+"/"+attr, predIndex, predType, budget, names)
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
		return rmattrOwner{kind: "EVENT_CONTEXT", resolve: walk("/context", nil, nil)}, nil
	}
	if len(segs) == 1 {
		return rmattrOwner{
			kind:    nodeRMType(wt.Tree),
			resolve: func(string) (map[string]any, error) { return compJSON, nil },
		}, nil
	}
	node, predIndex, predType, err := resolveLeaf(wt, segs, ambiguous)
	if err != nil {
		return rmattrOwner{}, err
	}
	ownerAql, kind, leaf := bareAQLPath(node.AQLPath), nodeRMType(node), ""
	if len(node.Children) == 0 {
		trimmed, isElementValue := strings.CutSuffix(ownerAql, "/value")
		if !isElementValue {
			return rmattrOwner{}, errSegNotFound
		}
		// The leaf's own RM type is the *anchor* the value-decoration families are
		// judged and decoded against (REQ-140 § C1): one FLAT path addresses both
		// the ELEMENT and the DataValue it holds.
		ownerAql, kind, leaf = trimmed, "ELEMENT", nodeRMType(node)
	}
	return rmattrOwner{kind: kind, leaf: leaf, resolve: walk(ownerAql, predIndex, predType)}, nil
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
	// REQ-053 — the one substitution carried in suffix form: a DV_CODED_TEXT
	// stored at a DV_TEXT-typed leaf (the corpus's dv_coded_text_as_dv_text
	// shape). |code is the discriminator: its presence re-selects the
	// DV_CODED_TEXT builder, and the group then follows that type's own rules —
	// |value required, |terminology / |formatting optional, bare value refused.
	// Without |code the leaf stays a plain DV_TEXT and a stray |value /
	// |terminology is refused by the allowlist below, as before. The encoder is
	// the exact inverse: leafToFlat routes a fully-captured DV_CODED_TEXT at a
	// DV_TEXT leaf to the coded suffix set instead of |raw.
	// The discriminator is a non-empty string |code, not the key's mere presence:
	// `|code: ""` and `|code: null` are what a form emits for "free text, no code
	// selected", and promoting those would mint a DV_CODED_TEXT whose
	// CODE_PHRASE.code_string is empty — RM-invalid, yet stable enough on
	// re-encode that nothing downstream flags it, and matched by any AQL predicate
	// testing defining_code/code_string against ''. They stay a plain DV_TEXT, so
	// the allowlist below refuses the stray |code as it did before this carve-out.
	if rmType == "DV_TEXT" {
		if code, coded := sfx["code"].(string); coded && code != "" {
			rmType = "DV_CODED_TEXT"
		}
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

// multimediaFromSuffixes rebuilds a canonical DV_MULTIMEDIA from its FLAT suffix
// set (REQ-140). `media_type` and `size` are RM-mandatory, so they are required
// rather than defaulted; the bare key is the **uri**, and the two Byte[]
// attributes carry the base64 their canonical form uses.
//
// The three CODE_PHRASE-valued attributes reachable here travel as a bare code:
// media_type in the implied [mediaTypeTerminology], the two algorithms with no
// terminology at all (see there). `charset`, `language` and `thumbnail` are not
// suffixes — they ride the REQ-140 underscore members.
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
	for _, attr := range multimediaBareCodeAttrs {
		v, ok := sfx[attr]
		if !ok {
			continue
		}
		algo, err := ctxString("|"+attr, v)
		if err != nil {
			return nil, err
		}
		// No terminology: the wire carries the code alone and this codec has no
		// openEHR code-set identifier to imply for either algorithm group, so
		// inventing one would put a terminology on the RM the author never wrote.
		dv[attr] = map[string]any{"_type": "CODE_PHRASE", "code_string": algo}
	}
	return dv, nil
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
	"DV_SCALE":   {"code": true, "value": true, "ordinal": true},
	"DV_PROPORTION": {
		"numerator": true, "denominator": true, "type": true,
		"magnitude_status": true, "normal_status": true,
		"accuracy": true, "accuracy_is_percent": true, "precision": true,
	},
	"DV_IDENTIFIER": {"id": true, "issuer": true, "assigner": true, "type": true},
	"CODE_PHRASE":   {"code": true, "terminology": true, "preferred_term": true},
	"DV_PARSABLE":   {"": true, "formalism": true},
	"DV_MULTIMEDIA": {
		"": true, "mediatype": true, "size": true, "data": true,
		"alternatetext": true, "integrity_check": true,
		"integrity_check_algorithm": true, "compression_algorithm": true,
	},
}

// suffixKind is the JSON value kind an optional pass-through suffix must carry.
// The value itself is handed to canjson untouched, so this is the last point at
// which a wrong kind can be reported against the FLAT key carrying it.
type suffixKind uint8

const (
	kindString suffixKind = iota
	kindNumber
	kindBool
)

func (k suffixKind) String() string {
	switch k {
	case kindNumber:
		return "number"
	case kindBool:
		return "boolean"
	case kindString:
		return "string"
	default:
		return fmt.Sprintf("suffixKind(%d)", int(k))
	}
}

// holds reports whether v could satisfy this kind. It is a necessary condition,
// not a full type check: canjson remains the authority on the RM type, and this
// deliberately admits everything canjson does so that naming the FLAT key never
// costs a body that decodes today.
//
//   - Numbers arrive as json.Number from a decoded body (unmarshalObject
//     preserves exact magnitudes) but as ordinary Go numerics from in-package
//     callers, so both spellings count.
//   - A *quoted* number satisfies kindNumber: canjson parses a numeric string
//     into Real / Integer, and some producers quote every scalar. Only a string
//     that is not a number at all (`|accuracy: "abc"`) is refused — the case
//     canjson would fail on anyway, but too late to name the key.
func (k suffixKind) holds(v any) bool {
	switch t := v.(type) {
	case string:
		if k == kindString {
			return true
		}
		_, err := strconv.ParseFloat(t, 64)
		return k == kindNumber && err == nil
	case bool:
		return k == kindBool
	case json.Number, float32, float64, int, int32, int64:
		return k == kindNumber
	}
	return false
}

// orderedSuffix is one optional pass-through suffix: the canonical RM attribute
// it rebuilds and the JSON kind its value must carry.
type orderedSuffix struct {
	attr string
	kind suffixKind
}

// orderedSuffixes maps the optional DV_ORDERED / DV_QUANTIFIED / DV_AMOUNT
// suffixes onto the canonical RM attribute each rebuilds, for the datatypes
// whose allowedSuffixes admit them. The value passes through as decoded (a
// json.Number keeps its exact lexical form), so canjson enforces the RM type; the
// one exception is normal_status, a CODE_PHRASE rebuilt from a bare code.
//
// The kind is carried alongside because canjson enforces the RM type too late to
// say *which FLAT key* was malformed — see [applyOrderedSuffixes].
var orderedSuffixes = map[string]orderedSuffix{
	"magnitude_status":    {"magnitude_status", kindString},
	"accuracy":            {"accuracy", kindNumber},
	"accuracy_is_percent": {"accuracy_is_percent", kindBool},
	"precision":           {"precision", kindNumber},
	"units_system":        {"units_system", kindString},
	"units_display_name":  {"units_display_name", kindString},
	"formatting":          {"formatting", kindString},
}

// orderedSuffixNames holds the [orderedSuffixes] keys in sorted order, so a leaf
// carrying two malformed suffixes names the same one on every run.
var orderedSuffixNames = slices.Sorted(maps.Keys(orderedSuffixes))

// applyOrderedSuffixes copies the optional suffixes present in sfx onto the
// canonical object dv. An absent suffix sets nothing — the attributes are all
// optional in the RM, so a missing one must stay absent rather than become a
// zero value (the same contract requireSuffix enforces for mandatory ones).
//
// A value of the wrong JSON kind is refused here, where the caller still has the
// FLAT key in hand to name it (decode %q). Passed through, it would instead
// surface from canjson against the *rebuilt tree* — "decode /content/0:
// typereg.Decode …" — naming a canonical path the payload author never wrote and
// no FLAT key at all. Deliberately no gap sentinel: a malformed value is a defect
// in the payload, not a datatype or path this codec declines to model, and the
// conformance census counts only the latter. The check adds no strictness of its
// own — it admits everything canjson admits, quoted numbers included (see
// [suffixKind.holds]) — and the RM's own value constraints (magnitude_status
// against its code set, …) stay with the validation package.
func applyOrderedSuffixes(dv, sfx map[string]any) error {
	for _, suffix := range orderedSuffixNames {
		v, ok := sfx[suffix]
		if !ok {
			continue
		}
		want := orderedSuffixes[suffix]
		if !want.kind.holds(v) {
			return fmt.Errorf("|%s must be a %s, got %T", suffix, want.kind, v)
		}
		dv[want.attr] = v
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
	return slices.ContainsFunc(node.Inputs, func(in webtemplate.Input) bool { return in.ListOpen })
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
// written. The map returned is flat itself when nothing was folded.
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
		case sameCtxValue(other, flat[key]):
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
		if id, idxStr, found := strings.CutLast(p, ":"); found {
			if n, err := strconv.Atoi(idxStr); err == nil {
				if n < 0 || idxStr != strconv.Itoa(n) {
					return parsedKey{}, fmt.Errorf("%w: invalid :index %q in %q", ErrUnknownPath, idxStr, key)
				}
				seg.id = id
				seg.idx = n
			}
		}
		segs = append(segs, seg)
	}
	return parsedKey{segs: segs, suffix: suffix}, nil
}
