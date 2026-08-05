package simplified

// REQ-140 — the **value-decoration** underscore families, decode side: the
// optional RM attributes a *DataValue* carries, addressed at the Web Template
// leaf that holds it.
//
// The Web Template folds ELEMENT.value into its leaf node, so one FLAT path
// addresses two RM objects and the router has to know which one a family
// belongs to (see [rmattrFamily.value]): `<leaf>/_uid` decorates the ELEMENT,
// `<leaf>/_normal_range` decorates the DV_ORDERED value inside it. Which
// families a leaf admits is read off the RM (rminfo) from the **leaf datatype**
// — `normal_range` reaches every DV_ORDERED, `mappings` DV_TEXT and its coded
// subtype — so the vocabulary cannot drift from the BMM.
//
// Nothing here re-implements a datatype. A bound, a `/meaning`, a `/target` and
// a `/purpose` are all decoded by [dvFromSuffixes] — the same function the
// clinical leaf loop uses — which buys the whole suffix allowlist, the
// RM-mandatory checks, the Phase A `|code`-selects-DV_CODED_TEXT substitution
// and the `|raw` bypass for free, and keeps the two directions symmetric
// (rmattr_value_encode.go emits through [emitLeafValue], its inverse).

import (
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
)

// rmattrTails is one family instance's tails split into the three positions the
// recursive grammar has: the family's **own** suffixes (`|match`,
// `|lower_included`, or "" for a bare value), one sub-object per leading path
// segment (`/lower`, `/meaning`, `/target`), and one *indexed* sub-object list
// per list-valued segment (`/_identifier:N` inside a party — REQ-140 § C2) —
// each collected as the suffix map [dvFromSuffixes] takes.
//
// orig maps a normalised tail back to the spelling the payload actually used,
// because the STRUCTURED interconversion re-spells every segment with an
// explicit `:index` (`/lower:0|magnitude`) and an error must name the key its
// author wrote.
type rmattrTails struct {
	own map[string]any
	sub map[string]map[string]any
	// sublist holds the indexed sub-path segments the recursive grammar reaches:
	// `/_identifier:0|id` becomes sublist["_identifier"][0]["id"]. Only a segment
	// the family named in the `lists` set it passed [splitRMAttrTails] lands
	// here; every other segment keeps the single-valued rule, so a stray
	// `/relationship:1` is refused rather than quietly becoming a list.
	sublist map[string]map[int]map[string]any
	orig    map[string]string
}

// ownTail / subTail are the normalised tail spellings, the keys [rmattrTails.orig]
// is indexed by.
func ownTail(suffix string) string {
	if suffix == "" {
		return ""
	}
	return "|" + suffix
}

func subTail(seg, suffix string) string {
	if suffix == "" {
		return "/" + seg
	}
	return "/" + seg + "|" + suffix
}

// subListTail is the normalised spelling of one slot of an indexed sub-path
// list. The `:index` is kept where [subTail] has none to keep, because two
// slots of the same segment are two different keys.
func subListTail(seg string, idx int, suffix string) string {
	return subTail(seg+":"+strconv.Itoa(idx), suffix)
}

// key rebuilds the full FLAT key a tail arrived under, preferring the payload's
// own spelling. Used for every refusal message and for the `decode %q` wrapper
// a nested value's error carries, which is what lets the PROBE-086 census scope
// an exclusion to one key (SKIPPED.md).
func (ts rmattrTails) key(g rmattrGroup, tail string) string {
	if orig, ok := ts.orig[tail]; ok {
		return g.key(orig)
	}
	return g.key(tail)
}

// splitRMAttrTails partitions g's tails. lists names the sub-path segments the
// family treats as an indexed list (nil for a family that has none); every other
// segment is single-valued, so a `:0` on it is the index-less spelling (the
// interconversion normalises every segment) and folds onto it, while anything
// higher addresses a list position that attribute does not have. Two spellings
// landing on one slot are refused rather than silently merged — the same rule
// [checkRMAttrIndexes] applies to `_family:N`.
func splitRMAttrTails(g rmattrGroup, lists map[string]bool) (rmattrTails, error) {
	ts := rmattrTails{
		own:     make(map[string]any),
		sub:     make(map[string]map[string]any),
		sublist: make(map[string]map[int]map[string]any),
		orig:    make(map[string]string),
	}
	for _, tail := range slices.Sorted(maps.Keys(g.tails)) {
		val := g.tails[tail]
		path, suffix := splitSuffix(tail)
		if path == "" {
			ts.own[suffix] = val
			ts.orig[ownTail(suffix)] = tail
			continue
		}
		seg, idx, isList, err := rmattrSubSegment(g, tail, path, lists)
		if err != nil {
			return rmattrTails{}, err
		}
		slot := ts.sub[seg]
		norm := subTail(seg, suffix)
		if isList {
			if ts.sublist[seg] == nil {
				ts.sublist[seg] = make(map[int]map[string]any)
			}
			if ts.sublist[seg][idx] == nil {
				ts.sublist[seg][idx] = make(map[string]any)
			}
			slot, norm = ts.sublist[seg][idx], subListTail(seg, idx, suffix)
		} else if slot == nil {
			slot = make(map[string]any)
			ts.sub[seg] = slot
		}
		if _, taken := slot[suffix]; taken {
			return rmattrTails{}, fmt.Errorf("%w: %q collides with another spelling of the same sub-path; remove one",
				ErrUnsupportedDatatype, g.key(tail))
		}
		slot[suffix] = val
		ts.orig[norm] = tail
	}
	return ts, nil
}

// rmattrSubSegment normalises one tail's sub-path into the segment name the
// grammar tables are keyed by, and reports whether it addresses a slot of an
// indexed list. A multi-segment path is returned joined, so the caller's
// allowlist refuses it naming the offending key rather than reporting its first
// segment as if the rest were not there.
//
// Only the leading segment may carry an index, and only for a segment in lists:
// the recursive grammar's lists sit at the position the family names them at
// (`/_identifier:N`), never deeper.
func rmattrSubSegment(g rmattrGroup, tail, path string, lists map[string]bool) (seg string, idx int, isList bool, err error) {
	segs := strings.Split(strings.TrimPrefix(path, "/"), "/")
	head := -1
	for i, s := range segs {
		id, n, err := rmattrSegIndex(g, tail, s)
		if err != nil {
			return "", 0, false, err
		}
		segs[i] = id
		if i == 0 {
			head = n
			continue
		}
		if n > 0 {
			return "", 0, false, rmattrNotAList(g, tail, id)
		}
	}
	joined := strings.Join(segs, "/")
	if lists[joined] {
		return joined, max(head, 0), true, nil
	}
	if head > 0 {
		return "", 0, false, rmattrNotAList(g, tail, segs[0])
	}
	return joined, 0, false, nil
}

// rmattrNotAList is the refusal for a `:index` on a sub-path segment the family
// declares single-valued.
func rmattrNotAList(g rmattrGroup, tail, seg string) error {
	return fmt.Errorf("%w: %q carries a :index, but %s addresses a single-valued RM attribute",
		ErrUnsupportedDatatype, g.key(tail), seg)
}

// rmattrSegIndex splits a `:index` off a sub-path segment under the FLAT
// spelling rules [parseFlatKey] enforces everywhere else (canonical decimal, no
// negatives).
func rmattrSegIndex(g rmattrGroup, tail, seg string) (id string, idx int, err error) {
	j := strings.LastIndex(seg, ":")
	if j < 0 {
		return seg, -1, nil
	}
	n, convErr := strconv.Atoi(seg[j+1:])
	if convErr != nil {
		return seg, -1, nil // not an index at all — part of the segment name
	}
	if n < 0 || seg[j+1:] != strconv.Itoa(n) {
		return "", 0, fmt.Errorf("%w: invalid :index %q in %q", ErrUnknownPath, seg[j+1:], g.key(tail))
	}
	return seg[:j], n, nil
}

// check refuses any tail outside a family's grammar, naming the offending FLAT
// key. Own suffixes and sub-paths are checked against separate sets because
// they are separate positions: `|lower_included` is not `/lower_included`.
func (ts rmattrTails) check(g rmattrGroup, own, sub map[string]bool) error {
	for _, suffix := range slices.Sorted(maps.Keys(ts.own)) {
		if !own[suffix] {
			return fmt.Errorf("%w: %q is not part of the %s grammar (REQ-140)",
				ErrUnsupportedDatatype, ts.key(g, ownTail(suffix)), g.family)
		}
	}
	for _, seg := range slices.Sorted(maps.Keys(ts.sub)) {
		if sub[seg] {
			continue
		}
		suffix := slices.Sorted(maps.Keys(ts.sub[seg]))[0]
		return fmt.Errorf("%w: %q is not part of the %s grammar (REQ-140)",
			ErrUnsupportedDatatype, ts.key(g, subTail(seg, suffix)), g.family)
	}
	return nil
}

// value decodes one sub-object with the datatype machinery a clinical leaf of
// the same type uses, wrapping the error the way [decodeFlat] wraps a leaf's —
// `decode "<key>"` with the *base* key — so the census reads the refusal at the
// same precision it reads a leaf's.
func (ts rmattrTails) value(g rmattrGroup, seg, rmType string) (map[string]any, error) {
	return ts.valueAt(g, subTail(seg, ""), rmType, ts.sub[seg])
}

// listValue is [rmattrTails.value] for one slot of an indexed sub-path list.
func (ts rmattrTails) listValue(g rmattrGroup, seg string, idx int, rmType string) (map[string]any, error) {
	return ts.valueAt(g, subListTail(seg, idx, ""), rmType, ts.sublist[seg][idx])
}

// valueAt is the shared body: decode sfx as rmType, naming tail's own FLAT key on
// failure.
func (ts rmattrTails) valueAt(g rmattrGroup, tail, rmType string, sfx map[string]any) (map[string]any, error) {
	dv, err := dvFromSuffixes(rmType, false, sfx)
	if err != nil {
		return nil, fmt.Errorf("simplified: decode %q: %w", ts.key(g, tail), err)
	}
	return dv, nil
}

// listSlots returns the dense slot sequence of an indexed sub-path list, or nil
// when the family instance carries none. The `:index` rules are the ones
// [checkRMAttrIndexes] applies to a `_family:N` sequence — bounded by
// [maxRepeatIndex], no gaps — because a gap here would fabricate an empty
// instance out of a malformed payload exactly as it would there.
func (ts rmattrTails) listSlots(g rmattrGroup, seg string) ([]int, error) {
	slots := ts.sublist[seg]
	if len(slots) == 0 {
		return nil, nil
	}
	for _, idx := range slices.Sorted(maps.Keys(slots)) {
		if idx > maxRepeatIndex {
			return nil, fmt.Errorf("%w: %q :index %d exceeds bound %d",
				ErrUnknownPath, ts.key(g, subListTail(seg, idx, "")), idx, maxRepeatIndex)
		}
	}
	out := make([]int, 0, len(slots))
	for i := range len(slots) {
		if slots[i] == nil {
			return nil, fmt.Errorf("%w: %s/%s sits in a sparse :index sequence (no :%d); the occurrences must run 0..%d",
				ErrUnknownPath, g.prefix(), seg, i, len(slots)-1)
		}
		out = append(out, i)
	}
	return out, nil
}

// ownString reads one of a family's own suffixes as a string. present is false
// when the payload did not spell it, so an RM-optional attribute stays absent
// rather than becoming a coerced empty string; a value of another JSON kind is
// refused here, where the FLAT key is still in hand to name it.
func (ts rmattrTails) ownString(g rmattrGroup, suffix string) (value string, present bool, err error) {
	v, present := ts.own[suffix]
	if !present {
		return "", false, nil
	}
	s, ok := v.(string)
	if !ok {
		return "", true, fmt.Errorf("%w: %q must be a string, got %T",
			ErrUnsupportedDatatype, ts.key(g, ownTail(suffix)), v)
	}
	return s, true, nil
}

// boolTail reads one of the interval's boundary Booleans, defaulting to def when
// the key is absent. A value of another JSON kind is refused here, where the
// FLAT key is still in hand to name it (the reason [applyOrderedSuffixes] checks
// kinds too).
func (ts rmattrTails) boolTail(g rmattrGroup, suffix string, def bool) (bool, error) {
	v, present := ts.own[suffix]
	if !present {
		return def, nil
	}
	b, ok := v.(bool)
	if !ok {
		return false, fmt.Errorf("%w: %q must be a boolean, got %T",
			ErrUnsupportedDatatype, ts.key(g, ownTail(suffix)), v)
	}
	return b, nil
}

// --- DV_INTERVAL --------------------------------------------------------

// intervalOwnTails / intervalSubTails are the DV_INTERVAL grammar: the two
// bounds as sub-objects, the four boundary Booleans as own suffixes.
var (
	intervalOwnTails = map[string]bool{
		"lower_included": true, "upper_included": true,
		"lower_unbounded": true, "upper_unbounded": true,
	}
	intervalSubTails = map[string]bool{"lower": true, "upper": true}
)

// intervalSuffixes decodes the DV_INTERVAL grammar out of a family instance's
// tails into a canonical DV_INTERVAL: `/lower` and `/upper` through the anchor
// datatype's own captured-key machinery, plus the four boundary Booleans. It is
// the shared implementation for `_normal_range`, `_other_reference_ranges:N`
// (whose REFERENCE_RANGE.range level the reference elides) and — from Phase C3
// — the DV_INTERVAL leaf, so the interval spelling has one definition.
//
// The two defaults are the whole reason this is one function rather than four
// suffix reads. `lower_unbounded` / `upper_unbounded` are RM-mandatory Booleans
// whose false value the reference omits, so absent is false. `lower_included` /
// `upper_included` are RM-**optional** (`Interval` declares them 0..1) while the
// SDK's generated `Interval` carries a mandatory Boolean, so the codec has to
// fix a mapping for "absent": it is the closed endpoint, `true`. That is the
// reading the RM's own invariant implies for a bounded end, and it is the only
// mapping under which the corpus round-trips byte-exactly in both directions —
// `dv_count`'s `_normal_range` omits the flags where `dv_quantity`'s spells
// them `false`, and encode's inverse rule (emit only what contradicts the
// default) reproduces each. One consequence is deliberate and recorded in
// deviations.md: a redundant `|lower_included: true` is normalised away, since
// it decodes to the same RM value as its absence.
func intervalSuffixes(g rmattrGroup, ts rmattrTails, anchor string) (map[string]any, error) {
	iv := map[string]any{"_type": "DV_INTERVAL"}
	for _, end := range []string{"lower", "upper"} {
		if _, bounded := ts.sub[end]; bounded {
			dv, err := ts.value(g, end, anchor)
			if err != nil {
				return nil, err
			}
			iv[end] = dv
		}
		unbounded, err := ts.boolTail(g, end+"_unbounded", false)
		if err != nil {
			return nil, err
		}
		included, err := ts.boolTail(g, end+"_included", true)
		if err != nil {
			return nil, err
		}
		iv[end+"_unbounded"] = unbounded
		iv[end+"_included"] = included
	}
	return iv, nil
}

// intervalLeafSuffixes decodes a **DV_INTERVAL Web Template leaf** — the same
// grammar `_normal_range` spells at a decoration position, at a template-modelled
// node instead (REQ-053's DV_INTERVAL<T> leaf closure). anchor is the bound
// datatype the Web Template names inside the angle brackets.
//
// The group arrives from [compositeLeafGroups] rather than the `_` router, so its
// family is the leaf's own FLAT segment id and every refusal names the key the
// payload author wrote.
func intervalLeafSuffixes(g rmattrGroup, anchor string) (map[string]any, error) {
	ts, err := splitRMAttrTails(g, nil)
	if err != nil {
		return nil, err
	}
	if err := ts.check(g, intervalOwnTails, intervalSubTails); err != nil {
		return nil, err
	}
	return intervalSuffixes(g, ts, anchor)
}

// decodeRMAttrNormalRange decodes `_normal_range` on a DV_ORDERED leaf — a
// DV_INTERVAL of the leaf's own anchor type.
func decodeRMAttrNormalRange(g rmattrGroup, anchor string) (any, error) {
	ts, err := splitRMAttrTails(g, nil)
	if err != nil {
		return nil, err
	}
	if err := ts.check(g, intervalOwnTails, intervalSubTails); err != nil {
		return nil, err
	}
	return intervalSuffixes(g, ts, anchor)
}

// refRangeSubTails is REFERENCE_RANGE's sub-object set: the interval's two
// bounds plus `/meaning`.
var refRangeSubTails = map[string]bool{"lower": true, "upper": true, "meaning": true}

// decodeRMAttrReferenceRange decodes one `_other_reference_ranges:N` instance.
//
// The reference **elides** REFERENCE_RANGE.range: the interval's bounds and
// boundary Booleans sit directly under the family instance, not under a `range`
// segment (verified against `ehrbase_conformance_data_types_dv_*`; wire.md
// § REQ-140 records the elision). `meaning` is RM-mandatory, so a range spelled
// without one is refused rather than decoded to an empty text; it is a DV_TEXT
// or a DV_CODED_TEXT under the Phase A rule — `|code` present selects the coded
// builder — which is exactly what the corpus writes at both spellings.
func decodeRMAttrReferenceRange(g rmattrGroup, anchor string) (any, error) {
	ts, err := splitRMAttrTails(g, nil)
	if err != nil {
		return nil, err
	}
	if err := ts.check(g, intervalOwnTails, refRangeSubTails); err != nil {
		return nil, err
	}
	iv, err := intervalSuffixes(g, ts, anchor)
	if err != nil {
		return nil, err
	}
	if _, ok := ts.sub["meaning"]; !ok {
		return nil, fmt.Errorf("%w: %s is missing the required /meaning (RM-mandatory on REFERENCE_RANGE)",
			ErrUnsupportedDatatype, g.prefix())
	}
	meaning, err := ts.value(g, "meaning", "DV_TEXT")
	if err != nil {
		return nil, err
	}
	return map[string]any{"_type": "REFERENCE_RANGE", "meaning": meaning, "range": iv}, nil
}

// --- TERM_MAPPING -------------------------------------------------------

// mappingOwnTails / mappingSubTails are TERM_MAPPING's grammar: `|match` on the
// instance, the mandatory `/target` CODE_PHRASE and the optional `/purpose`
// DV_CODED_TEXT as sub-objects.
var (
	mappingOwnTails = map[string]bool{"match": true}
	mappingSubTails = map[string]bool{"target": true, "purpose": true}
)

// matchCodes is the set TERM_MAPPING.match admits — `>` broader, `=` equivalent,
// `<` narrower, `?` unknown (the first three from ISO 2788 / 5964). The RM types
// the attribute as a bare Character, so this lexical check is the only thing
// between a defective value and the wire; the codes' *meaning* is the validation
// package's business, as everywhere else.
const matchCodes = "=<>?"

// decodeRMAttrTermMapping decodes one `_mapping:N` instance on a DV_TEXT or
// DV_CODED_TEXT leaf.
//
// `match` is a single character and lands in the canonical form as its code
// point, because that is what the generated TERM_MAPPING marshals a Go rune to
// (`"match": 61`); encode spells it back as the one-character string the wire
// carries. `/target` is decoded by the CODE_PHRASE leaf builder — `|code`
// required, `|terminology` optional exactly as at an ENTRY `language` leaf, which
// is what keeps the two spellings of a CODE_PHRASE from diverging — and
// `/purpose` by the DV_CODED_TEXT one, which requires `|code`+`|value` and takes
// `|terminology` when present.
func decodeRMAttrTermMapping(g rmattrGroup, _ string) (any, error) {
	ts, err := splitRMAttrTails(g, nil)
	if err != nil {
		return nil, err
	}
	if err := ts.check(g, mappingOwnTails, mappingSubTails); err != nil {
		return nil, err
	}
	match, err := ts.matchCode(g)
	if err != nil {
		return nil, err
	}
	if _, ok := ts.sub["target"]; !ok {
		return nil, fmt.Errorf("%w: %s is missing the required /target (RM-mandatory on TERM_MAPPING)",
			ErrUnsupportedDatatype, g.prefix())
	}
	target, err := ts.value(g, "target", "CODE_PHRASE")
	if err != nil {
		return nil, err
	}
	tm := map[string]any{"_type": "TERM_MAPPING", "match": match, "target": target}
	if _, ok := ts.sub["purpose"]; ok {
		purpose, err := ts.value(g, "purpose", "DV_CODED_TEXT")
		if err != nil {
			return nil, err
		}
		tm["purpose"] = purpose
	}
	return tm, nil
}

// --- ELEMENT null flavour / null reason ---------------------------------

// decodeRMAttrNullFlavour decodes `_null_flavour` on a collapsed ELEMENT leaf.
//
// It is the ordinary DV_CODED_TEXT suffix grammar — `|code` and `|value`
// required, `|terminology` optional and preserved — not an `openehr`-implied
// code+value pair: `ehrbase_conformance_Element_null_flavor.json` writes
// `|terminology: "openehr"` explicitly, and inventing the terminology for a body
// that omits it would put a key on the wire the author never wrote (constraint 6;
// wire.md § REQ-140 corrected 2026-08-05). Which openEHR code is a legal null
// flavour stays with the validation package, as every other code set does.
//
// This is the one family that is legal beside an **absent** value — that is what
// it is for — so it decorates the ELEMENT rather than the value, and the router
// never asks for a `value` node on its behalf.
func decodeRMAttrNullFlavour(g rmattrGroup, _ string) (any, error) {
	return decodeRMAttrLeafValue(g, "DV_CODED_TEXT")
}

// decodeRMAttrNullReason decodes `_null_reason` — a bare DV_TEXT, through the
// DV_TEXT leaf builder, so the Phase A carve-out lets a DV_CODED_TEXT ride the
// `|code` spelling here too (`null_reason` is typed DV_TEXT, which admits its
// coded subtype).
func decodeRMAttrNullReason(g rmattrGroup, _ string) (any, error) {
	return decodeRMAttrLeafValue(g, "DV_TEXT")
}

// --- the DataValue members that are themselves datatypes ----------------

// decodeRMAttrCodePhraseMember decodes `_charset`, `_language` and `_encoding` —
// the CODE_PHRASE-valued members a DV_ENCAPSULATED subtype or a DV_TEXT carries.
// One decoder for all three families on all four owners: which owner admits which
// member is read off the RM (rminfo, via [rmattrDecode]), and the value is the
// CODE_PHRASE leaf builder's, so the standalone and the ENTRY `language` spellings
// of one datatype cannot diverge. The corpus writes `|code`, `|terminology` and —
// on DV_TEXT's language — `|preferred_term`.
func decodeRMAttrCodePhraseMember(g rmattrGroup, _ string) (any, error) {
	return decodeRMAttrLeafValue(g, "CODE_PHRASE")
}

// decodeRMAttrThumbnail decodes `_thumbnail` — a nested DV_MULTIMEDIA in the same
// suffix form its owner rides, so the grammar is recursive by construction.
//
// The nested value's *own* underscore members have no sub-path spelling here: a
// thumbnail carrying a charset, a language or a thumbnail of its own rides
// `_thumbnail|raw` instead (the DV_MULTIMEDIA leaf builder's `|raw` carrier —
// lossless, and a shape the pinned corpus never writes). Recorded in
// deviations.md.
func decodeRMAttrThumbnail(g rmattrGroup, _ string) (any, error) {
	return decodeRMAttrLeafValue(g, "DV_MULTIMEDIA")
}

// decodeRMAttrTemporalAccuracy decodes `_accuracy` on a DV_DATE / DV_DATE_TIME /
// DV_TIME leaf — a bare DV_DURATION value.
//
// DV_TEMPORAL redefines `accuracy` as a DV_DURATION **object** where DV_AMOUNT
// (and so DV_QUANTITY, DV_COUNT, DV_DURATION, DV_PROPORTION) declares a Real, so
// the attribute has two spellings in the reference's grammar: the scalar
// `|accuracy` suffix and this family. The family is admitted only where the RM
// declares the DV_DURATION form ([rmattrFamily.attrType]), so one attribute never
// has two channels at one owner.
func decodeRMAttrTemporalAccuracy(g rmattrGroup, _ string) (any, error) {
	return decodeRMAttrLeafValue(g, "DV_DURATION")
}

// decodeRMAttrLeafValue decodes a family whose whole value is one datatype
// spelled in its own suffix form, with no sub-paths of its own: the group's tails
// *are* the suffix map. Errors are wrapped naming the family prefix, the base key
// the census scopes a datatype refusal by.
func decodeRMAttrLeafValue(g rmattrGroup, rmType string) (any, error) {
	ts, err := splitRMAttrTails(g, nil)
	if err != nil {
		return nil, err
	}
	if len(ts.sub) > 0 {
		seg := slices.Sorted(maps.Keys(ts.sub))[0]
		suffix := slices.Sorted(maps.Keys(ts.sub[seg]))[0]
		return nil, fmt.Errorf("%w: %q is not part of the %s grammar (REQ-140)",
			ErrUnsupportedDatatype, ts.key(g, subTail(seg, suffix)), g.family)
	}
	dv, err := dvFromSuffixes(rmType, false, ts.own)
	if err != nil {
		return nil, fmt.Errorf("simplified: decode %q: %w", g.prefix(), err)
	}
	return dv, nil
}

// matchCode reads `|match` as the single character TERM_MAPPING admits, returning
// its code point. Anything else — absent, empty, longer than one rune, or a
// character outside the set — is a typed error rather than a truncated or zero
// rune, which would be a mapping whose relation to its target is a fabrication.
func (ts rmattrTails) matchCode(g rmattrGroup) (int32, error) {
	key := ts.key(g, ownTail("match"))
	v, present := ts.own["match"]
	if !present {
		return 0, fmt.Errorf("%w: %s is missing the required |match (RM-mandatory on TERM_MAPPING)",
			ErrUnsupportedDatatype, g.prefix())
	}
	s, ok := v.(string)
	if !ok {
		return 0, fmt.Errorf("%w: %q must be a string, got %T", ErrUnsupportedDatatype, key, v)
	}
	runes := []rune(s)
	if len(runes) != 1 || !strings.ContainsRune(matchCodes, runes[0]) {
		return 0, fmt.Errorf("%w: %q is %q, but TERM_MAPPING.match is one of %s (REQ-140)",
			ErrUnsupportedDatatype, key, s, matchCodes)
	}
	return runes[0], nil
}
