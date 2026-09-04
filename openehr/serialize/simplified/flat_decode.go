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
	ctx, ctxOrigin, content, err := siphonContext(flat, root.ID)
	if err != nil {
		return nil, err
	}
	budget := &allocBudget{limit: maxTotalNodes}
	ambiguous := ambiguousBarePaths(wt)
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
	compositeGroups, err := compositeLeafGroups(content, wt)
	if err != nil {
		return nil, err
	}
	// Siphon off the underscore-prefixed RM attributes (REQ-140): a path
	// segment starting with `_` ends Web Template resolution at the segment
	// before it, so those keys are grouped per (owner path, family, :index) and
	// routed by the owner's RM class instead of resolving to a leaf. What stays
	// in content is ordinary template-constrained leaf data.
	attrGroups, attrIndexes, err := rmattrGroups(content)
	if err != nil {
		return nil, err
	}
	// Group the remaining FLAT keys by leaf instance (key minus the |suffix);
	// each group's suffix->value pairs build one DataValue.
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
	for _, g := range attrGroups {
		owner, err := rmattrOwnerAt(wt, compJSON, g.base, ambiguous, budget, names)
		if errors.Is(err, errSegNotFound) {
			return nil, fmt.Errorf("%w: %q", ErrUnknownPath, g.prefix())
		}
		if err != nil {
			return nil, fmt.Errorf("simplified: %q: %w", g.prefix(), err)
		}
		if err := rmattrDecode(owner, g, attrIndexes[g.base+"\x00"+g.family], budget); err != nil {
			return nil, err
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
	return compJSON, nil
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
	"context/setting|terminology": "openehr",
}

// contextMetaOwnedBases are the EVENT_CONTEXT leaves the ctx/ short forms own
// outright: the Web Template carries a node for them, so an unaccepted spelling
// would resolve as an ordinary leaf and then be silently overwritten by
// [applyContext], which writes the ctx/ values last. Refusing every other
// spelling of these two bases in [siphonContext] is what keeps that unreachable.
//
// Only these two. `language`, `territory` and `composer` are deliberately absent:
// the composer external_ref suffixes (`composer|id` / `|id_scheme` /
// `|id_namespace`) must keep flowing through to decode so they are refused there
// and counted in the PROBE-086 census as the real PARTY_PROXY data loss they are
// (ADR 0015). Both bases here are held out of that census by the harness
// (`IsCompositionMeta`), so refusing extra suffixes early moves no count.
var contextMetaOwnedBases = map[string]bool{
	"context/setting": true, "context/start_time": true,
}

// metadataOwnedBaseCtx names the ctx/ short form that owns each base, so the
// refusal points at the spelling the payload should have used.
var metadataOwnedBaseCtx = map[string]string{
	"context/setting": "ctx/setting|code + ctx/setting|value", "context/start_time": "ctx/time",
}

// MetadataAliasSpellings returns the root-relative FLAT spellings the decoder
// accepts as aliases of the ctx/ composition-metadata short forms (ADR 0015),
// sorted. `"language|code"`, for instance, is accepted at
// `<root>/language|code` and normalised to `ctx/language`.
//
// This is informative public surface for conformance tooling, not a decode knob.
// A harness that has to hold composition metadata out of a like-for-like FLAT
// comparison (PROBE-086) derives that hold-out from here rather than restating
// the table, so an alias the codec accepts cannot silently diverge from the
// spellings a census excuses. The returned slice is freshly allocated, so a
// caller may sort or filter it without reaching the decoder's own table.
func MetadataAliasSpellings() []string {
	return slices.Sorted(maps.Keys(metadataAliases))
}

// MetadataWitnessSpellings returns the root-relative FLAT spellings the decoder
// accepts as terminology witnesses beside an aliased CODE_PHRASE, sorted: the
// `|terminology` keys whose value is checked against the terminology the ctx/
// short form implies and then discarded.
//
// Informative public surface for conformance tooling on the same terms as
// [MetadataAliasSpellings], reported separately because a witness is not data —
// nothing on the ctx/ side carries it, so a comparison has to account for it as
// a checked-and-dropped key rather than as a respelling. The returned slice is
// freshly allocated.
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
func siphonContext(flat map[string]any, rootID string) (ctx map[string]any, ctxOrigin map[string]string, content map[string]any, err error) {
	ctx, ctxOrigin, content = make(map[string]any), make(map[string]string), make(map[string]any)
	// Sorted so a body carrying two conflicting real-path spellings reports the
	// same key first on every run — a map-order-dependent error message is not
	// reproducible for whoever has to fix the payload.
	for _, key := range slices.Sorted(maps.Keys(flat)) {
		val := flat[key]
		if strings.HasPrefix(key, "ctx/") {
			if err := putCtx(ctx, ctxOrigin, key, val, key); err != nil {
				return nil, nil, nil, err
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
				return nil, nil, nil, err
			}
			if got != want {
				return nil, nil, nil, fmt.Errorf("%w: %s = %q, but the ctx/ form implies %q (a differing terminology cannot be carried)",
					ErrUnsupportedDatatype, key, got, want)
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
		if base, _, _ := strings.Cut(rel, "|"); contextMetaOwnedBases[base] {
			return nil, nil, nil, fmt.Errorf("%w: %s is not a spelling the ctx/ short form can carry — %q owns this leaf, so only its accepted suffixes travel (see deviations.md)",
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
		if _, isComposerSub := strings.CutPrefix(rel, "composer/"); isComposerSub {
			return nil, nil, nil, fmt.Errorf("%w: %q is composer party sub-structure, which no ctx/ short form can carry (ADR 0015 boundary)",
				ErrUnsupportedDatatype, key)
		}
		content[key] = val
	}
	return ctx, ctxOrigin, content, nil
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

// parseCtx decodes the ctx/ entries. Only the core context fields are supported;
// any other ctx/ field is ErrUnknownPath (see deviations.md). Values of the
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
			err = fmt.Errorf("%w: %q (context field not supported — see deviations.md)", ErrUnknownPath, key)
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
				"defining_code": codePhraseJSON(ci.settingCode, "openehr"),
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
	cur, attr, err := walkAQL(compJSON, aqlPath, predIndex, predType, budget, names)
	if err != nil {
		return err
	}
	if _, exists := cur[attr]; exists {
		// Two FLAT keys resolved to the same terminal slot (e.g. "a" vs
		// "a:0" on a repeatable) — overwriting would silently drop one.
		return fmt.Errorf("%w: duplicate placement at %q", ErrUnknownPath, aqlPath)
	}
	cur[attr] = dv
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
	// Unreachable: the loop returns at i == len(segs)-1 and segs is non-empty.
	return nil, "", fmt.Errorf("%w: canonical path %q walked past its last segment", ErrUnknownPath, aqlPath)
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
// Keys are consumed from content: what stays behind is ordinary leaf data.
func compositeLeafGroups(content map[string]any, wt *webtemplate.WebTemplate) ([]rmattrGroup, error) {
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
			g = &rmattrGroup{base: base, family: family, index: index, tails: make(map[string]any)}
			byLeaf[id] = g
		}
		g.tails[tail] = val
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
		if len(node.Children) > 0 || !isCompositeLeafType(node.RMType) {
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
			!compositeLeafOwnsSub(node.RMType, pk.segs[i+1].id) {
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
	// A composite leaf is single-valued at every position the reference spells one,
	// so `:0` is the interconversion's explicit-index spelling and anything higher
	// addresses a list slot the RM attribute does not have.
	if g.index > 0 {
		return fmt.Errorf("%w: %q (%s addresses a single-valued RM attribute, not an indexed list)",
			ErrUnknownPath, g.prefix(), g.family)
	}
	pk, err := parseFlatKey(g.prefix())
	if err != nil {
		return err
	}
	node, predIndex, predType, err := resolveLeaf(wt, pk.segs, ambiguous)
	if err != nil {
		// splitCompositeLeafKey resolved this path already, so the only way here is
		// the reused-sibling refusal, which carries its own message.
		return fmt.Errorf("simplified: %q: %w", g.prefix(), err)
	}
	value, err := compositeLeafValue(g, node.RMType)
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
	// segs[1].idx <= 0: the OPT-free STRUCTURED interconversion re-spells every
	// segment with an explicit index, so `context` and `context:0` are one node.
	if len(segs) == 2 && segs[1].id == "context" && segs[1].idx <= 0 {
		return rmattrOwner{kind: "EVENT_CONTEXT", resolve: walk("/context", nil, nil)}, nil
	}
	if len(segs) == 1 {
		return rmattrOwner{
			kind:    wt.Tree.RMType,
			resolve: func(string) (map[string]any, error) { return compJSON, nil },
		}, nil
	}
	node, predIndex, predType, err := resolveLeaf(wt, segs, ambiguous)
	if err != nil {
		return rmattrOwner{}, err
	}
	ownerAql, kind, leaf := bareAQLPath(node.AQLPath), node.RMType, ""
	if len(node.Children) == 0 {
		trimmed, isElementValue := strings.CutSuffix(ownerAql, "/value")
		if !isElementValue {
			return rmattrOwner{}, errSegNotFound
		}
		// The leaf's own RM type is the *anchor* the value-decoration families are
		// judged and decoded against (REQ-140 § C1): one FLAT path addresses both
		// the ELEMENT and the DataValue it holds.
		ownerAql, kind, leaf = trimmed, "ELEMENT", node.RMType
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
