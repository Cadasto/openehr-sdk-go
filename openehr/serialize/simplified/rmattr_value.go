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

// splitRMAttrTails partitions g's tails. sub names the single-valued sub-path
// segments the family's grammar reads and lists the ones it treats as an
// indexed list (nil for a family that has none). A tail whose sub-path names
// neither is unmatched content (REQ-170): it goes to the group's sink before
// its indexes are judged. A `:0` on a single-valued segment is the index-less
// spelling (the interconversion normalises every segment) and folds onto it,
// while anything higher addresses a list position that attribute does not
// have. Two spellings landing on one slot are refused rather than silently
// merged — the same rule [checkRMAttrIndexes] applies to `_family:N`.
func splitRMAttrTails(g rmattrGroup, sub, lists map[string]bool) (rmattrTails, error) {
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
		seg, idx, isList, known, err := rmattrSubSegment(g, tail, path, sub, lists)
		if err != nil {
			return rmattrTails{}, err
		}
		if !known {
			g.sink.add(g.key(tail))
			continue
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
		if held, taken := slot[suffix]; taken {
			if g.dups == nil {
				return rmattrTails{}, fmt.Errorf("%w: %q collides with another spelling of the same sub-path; remove one",
					ErrUnsupportedDatatype, g.key(tail))
			}
			// Judged once the decode knows whether it reads the sub-path
			// ([rmattrGroup.settleDups]).
			*g.dups = append(*g.dups, tailDup{first: ts.key(g, norm), second: g.key(tail), firstVal: held, secondVal: val})
			continue
		}
		slot[suffix] = val
		ts.orig[norm] = tail
	}
	return ts, nil
}

// rmattrSubSegment normalises one tail's sub-path into the segment name the
// grammar tables are keyed by, and reports whether it addresses a slot of an
// indexed list. A multi-segment path is joined, so it matches no single
// segment of the grammar and is unmatched as a whole. known is false for a
// path that names neither a segment in sub nor one in lists.
//
// Only the leading segment may carry an index, and only for a segment in lists:
// the recursive grammar's lists sit at the position the family names them at
// (`/_identifier:N`), never deeper. A malformed index spelling is refused
// whatever segment it is on. The key is judged from the left (REQ-170): a
// leading segment the grammar reads, past its bound (above `:0` on a
// single-valued one, above [maxRepeatIndex] on a list), makes the key
// malformed whatever follows it, so it is refused even where a later segment
// would leave the key unmatched.
func rmattrSubSegment(g rmattrGroup, tail, path string, sub, lists map[string]bool) (seg string, idx int, isList, known bool, err error) {
	segs := strings.Split(strings.TrimPrefix(path, "/"), "/")
	indexes := make([]int, len(segs))
	for i, s := range segs {
		id, n, err := rmattrSegIndex(g, tail, s)
		if err != nil {
			return "", 0, false, false, err
		}
		segs[i], indexes[i] = id, n
	}
	switch first := segs[0]; {
	case lists[first] && indexes[0] > maxRepeatIndex:
		return "", 0, false, true, fmt.Errorf("%w: %q :index %d exceeds bound %d",
			ErrUnknownPath, g.key(tail), indexes[0], maxRepeatIndex)
	case sub[first] && !lists[first] && indexes[0] > 0:
		return "", 0, false, true, rmattrNotAList(g, tail, first)
	}
	joined := strings.Join(segs, "/")
	if !sub[joined] && !lists[joined] {
		return joined, 0, false, false, nil
	}
	for i := 1; i < len(segs); i++ {
		if indexes[i] > 0 {
			return "", 0, false, true, rmattrNotAList(g, tail, segs[i])
		}
	}
	if lists[joined] {
		return joined, max(indexes[0], 0), true, true, nil
	}
	if indexes[0] > 0 {
		return "", 0, false, true, rmattrNotAList(g, tail, segs[0])
	}
	return joined, 0, false, true, nil
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
	id, idxStr, found := strings.CutLast(seg, ":")
	if !found {
		return seg, -1, nil
	}
	n, convErr := strconv.Atoi(idxStr)
	if convErr != nil {
		return seg, -1, nil // not an index at all — part of the segment name
	}
	if n < 0 || idxStr != strconv.Itoa(n) {
		return "", 0, fmt.Errorf("%w: invalid :index %q in %q", ErrUnknownPath, idxStr, g.key(tail))
	}
	return id, n, nil
}

// check hands every tail outside a family's grammar to the group's sink as
// unmatched content and removes it (REQ-170). Own suffixes and sub-paths are
// checked against separate sets because they are separate positions:
// `|lower_included` is not `/lower_included`. [splitRMAttrTails] has already
// set aside a sub-path the grammar does not name, so the sub-path pass is a
// backstop.
func (ts rmattrTails) check(g rmattrGroup, own, sub map[string]bool) {
	for _, suffix := range slices.Sorted(maps.Keys(ts.own)) {
		if !own[suffix] {
			g.sink.add(ts.key(g, ownTail(suffix)))
			delete(ts.own, suffix)
		}
	}
	for _, seg := range slices.Sorted(maps.Keys(ts.sub)) {
		if !sub[seg] {
			ts.dropSub(g, seg)
		}
	}
}

// dropSub hands every key of the sub-object seg to the group's sink as
// unmatched content and removes the sub-object.
func (ts rmattrTails) dropSub(g rmattrGroup, seg string) {
	for _, suffix := range slices.Sorted(maps.Keys(ts.sub[seg])) {
		g.sink.add(ts.key(g, subTail(seg, suffix)))
	}
	delete(ts.sub, seg)
}

// filterSub keeps, of the sub-object seg, the suffixes a value of rmType
// reads, handing the key of every other one to the group's sink; a sub-object
// left with nothing is removed, as if the body did not spell it.
func (ts rmattrTails) filterSub(g rmattrGroup, seg, rmType string) {
	sfx, ok := ts.sub[seg]
	if !ok {
		return
	}
	sfx = matchedSuffixes(rmType, sfx, func(suffix string) string { return ts.key(g, subTail(seg, suffix)) }, g.sink)
	if len(sfx) == 0 {
		delete(ts.sub, seg)
		return
	}
	ts.sub[seg] = sfx
}

// filterSublist is [rmattrTails.filterSub] for every slot of the indexed
// sub-path list seg. A slot left with nothing is removed, so the :index rules
// then judge only the slots the body spells with matched keys.
func (ts rmattrTails) filterSublist(g rmattrGroup, seg, rmType string) {
	for _, idx := range slices.Sorted(maps.Keys(ts.sublist[seg])) {
		sfx := matchedSuffixes(rmType, ts.sublist[seg][idx], func(suffix string) string {
			return ts.key(g, subListTail(seg, idx, suffix))
		}, g.sink)
		if len(sfx) == 0 {
			delete(ts.sublist[seg], idx)
			continue
		}
		ts.sublist[seg][idx] = sfx
	}
	if len(ts.sublist[seg]) == 0 {
		delete(ts.sublist, seg)
	}
}

// empty reports whether no tail is left: every key of the family instance was
// unmatched.
func (ts rmattrTails) empty() bool {
	return len(ts.own) == 0 && len(ts.sub) == 0 && len(ts.sublist) == 0
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
// The defaults are the whole reason this is one function rather than four
// suffix reads. BASE `Interval` declares all four Booleans mandatory (1..1),
// and the reference omits each one's default value. `lower_unbounded` /
// `upper_unbounded` are omitted when false, so absent is false.
// `lower_included` / `upper_included` default by side. On a bounded side they
// are omitted when true, the closed endpoint, so absent is true. On an open
// side absent is false, the only value BASE `Lower_included_valid` /
// `Upper_included_valid` permit there, and an explicit true is read as
// written, although it breaks that invariant (encode refuses it).
// Every open end in the corpus spells `|*_included: false`, so those
// invariants fix the open-side default, not the corpus. The corpus pins
// the bounded side: `dv_count`'s `_normal_range` omits the flags where
// `dv_quantity`'s spells them `false`, and encode's inverse rule (emit
// only what contradicts the bounded-side default) reproduces each. Two
// consequences are deliberate and recorded in
// deviations.md: a redundant `|lower_included: true` on a bounded side is
// normalised away, since it decodes to the same RM value as its absence, and an
// absent `|*_included` on an open side re-encodes as an explicit false.
func intervalSuffixes(g rmattrGroup, ts rmattrTails, anchor string) (map[string]any, error) {
	iv := map[string]any{"_type": "DV_INTERVAL"}
	for _, end := range []string{"lower", "upper"} {
		_, bounded := ts.sub[end]
		unbounded, err := ts.boolTail(g, end+"_unbounded", false)
		if err != nil {
			return nil, err
		}
		// BASE `Interval` defines `lower_unbounded` as the lower boundary being
		// open (-infinity), and `upper_unbounded` the upper one (+infinity), so
		// an open side carries no bound, and the grammar spells an open side by
		// its flag alone: each end must spell exactly one of the two. Both, or
		// neither, is a contradiction the codec must not resolve by guessing:
		// accepting a flagless boundless end would build the very interval
		// [intervalBoundToFlat] refuses on the way out, so the same payload
		// would decode and then fail to re-encode.
		switch {
		case bounded && unbounded:
			return nil, fmt.Errorf("%w: %s spells a /%s bound beside `|%s_unbounded: true`; BASE Interval marks that boundary open, so it carries no bound and the pair contradicts itself",
				ErrUnsupportedDatatype, g.prefix(), end, end)
		case !bounded && !unbounded:
			return nil, fmt.Errorf("%w: %s carries no /%s bound and no `|%s_unbounded: true`; a side not marked `|%s_unbounded` must carry a bound, and an open side is spelled by that flag alone",
				ErrUnsupportedDatatype, g.prefix(), end, end, end)
		}
		if bounded {
			dv, err := ts.value(g, end, anchor)
			if err != nil {
				return nil, err
			}
			iv[end] = dv
		}
		// The closed endpoint is the default only where there is an endpoint:
		// an open side defaults to not included, the one value BASE allows it.
		// An explicit true there is read as written. The RM value then breaks
		// BASE `Lower_included_valid` or `Upper_included_valid`, but it can
		// still be built: RM validation reports it, and encode refuses to
		// write it back ([intervalToFlat]).
		included, err := ts.boolTail(g, end+"_included", !unbounded)
		if err != nil {
			return nil, err
		}
		iv[end+"_unbounded"] = unbounded
		iv[end+"_included"] = included
	}
	return iv, nil
}

// includedOpenSideError refuses to encode an interval side that is both open
// and included, which BASE `Interval` forbids by name (`Lower_included_valid`,
// `Upper_included_valid`). Decode reads that pair as written
// ([intervalSuffixes]); only encode refuses it. key is the side's
// `|*_included` key.
func includedOpenSideError(key, end string) error {
	invariant := "Lower_included_valid"
	if end == "upper" {
		invariant = "Upper_included_valid"
	}
	return fmt.Errorf("%w: %q is true on a side marked `|%s_unbounded: true`; BASE Interval's `%s` forbids an included open boundary, so the pair contradicts itself",
		ErrUnsupportedDatatype, key, end, invariant)
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
	ts, err := intervalTails(g, anchor, intervalSubTails)
	if err != nil {
		return nil, err
	}
	return intervalSuffixes(g, ts, anchor)
}

// intervalTails splits g's tails for the interval grammar, sub being the
// family's sub-object set, and sets aside every key the grammar does not read:
// a tail outside it, and a bound suffix outside the anchor's suffix set. It
// returns [errNothingMatched] when nothing is left.
func intervalTails(g rmattrGroup, anchor string, sub map[string]bool) (rmattrTails, error) {
	ts, err := splitRMAttrTails(g, sub, nil)
	if err != nil {
		return rmattrTails{}, err
	}
	ts.check(g, intervalOwnTails, sub)
	ts.filterSub(g, "lower", anchor)
	ts.filterSub(g, "upper", anchor)
	if ts.empty() {
		return rmattrTails{}, errNothingMatched
	}
	return ts, nil
}

// decodeRMAttrNormalRange decodes `_normal_range` on a DV_ORDERED leaf — a
// DV_INTERVAL of the leaf's own anchor type.
func decodeRMAttrNormalRange(g rmattrGroup, anchor string) (any, error) {
	ts, err := intervalTails(g, anchor, intervalSubTails)
	if err != nil {
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
	ts, err := splitRMAttrTails(g, refRangeSubTails, nil)
	if err != nil {
		return nil, err
	}
	ts.check(g, intervalOwnTails, refRangeSubTails)
	ts.filterSub(g, "lower", anchor)
	ts.filterSub(g, "upper", anchor)
	ts.filterSub(g, "meaning", "DV_TEXT")
	if ts.empty() {
		return nil, errNothingMatched
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
// `match` lands in the intermediate canonical-JSON tree as the one-character
// string [rmattrTails.matchCode] returns (`"match": "="`) — the same canonical
// spelling the generated TERM_MAPPING's own JSON contract uses since
// rm.Character (REQ-046 / REQ-052), so this package's own FLAT decoder never
// depends on rm.Character.UnmarshalJSON's backward-compatibility numeric
// branch to round-trip a mapping. `/target` is decoded by the CODE_PHRASE leaf
// builder — `|code` required, `|terminology` optional exactly as at an ENTRY
// `language` leaf, which is what keeps the two spellings of a CODE_PHRASE from
// diverging — and `/purpose` by the DV_CODED_TEXT one, which requires
// `|code`+`|value` and takes `|terminology` when present.
func decodeRMAttrTermMapping(g rmattrGroup, _ string) (any, error) {
	ts, err := splitRMAttrTails(g, mappingSubTails, nil)
	if err != nil {
		return nil, err
	}
	ts.check(g, mappingOwnTails, mappingSubTails)
	ts.filterSub(g, "target", "CODE_PHRASE")
	ts.filterSub(g, "purpose", "DV_CODED_TEXT")
	if ts.empty() {
		return nil, errNothingMatched
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
// *are* the suffix map. A sub-path, and a suffix outside the datatype's set, is
// unmatched content (REQ-170). Errors are wrapped naming the family prefix, the
// base key the census scopes a datatype refusal by.
func decodeRMAttrLeafValue(g rmattrGroup, rmType string) (any, error) {
	ts, err := splitRMAttrTails(g, nil, nil)
	if err != nil {
		return nil, err
	}
	own := matchedSuffixes(rmType, ts.own, func(suffix string) string { return ts.key(g, ownTail(suffix)) }, g.sink)
	if len(own) == 0 {
		return nil, errNothingMatched
	}
	dv, err := dvFromSuffixes(rmType, false, own)
	if err != nil {
		return nil, fmt.Errorf("simplified: decode %q: %w", g.prefix(), err)
	}
	return dv, nil
}

// matchCode reads `|match` as the single character TERM_MAPPING admits, returning
// it as the one-character canonical string (never the legacy code point — the
// FLAT decoder's own tree carries the same spelling encode produces). Anything
// else — absent, empty, longer than one rune, or a character outside the set —
// is a typed error rather than a truncated or empty string, which would be a
// mapping whose relation to its target is a fabrication.
func (ts rmattrTails) matchCode(g rmattrGroup) (string, error) {
	key := ts.key(g, ownTail("match"))
	v, present := ts.own["match"]
	if !present {
		return "", fmt.Errorf("%w: %s is missing the required |match (RM-mandatory on TERM_MAPPING)",
			ErrUnsupportedDatatype, g.prefix())
	}
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("%w: %q must be a string, got %T", ErrUnsupportedDatatype, key, v)
	}
	runes := []rune(s)
	if len(runes) != 1 || !strings.ContainsRune(matchCodes, runes[0]) {
		return "", fmt.Errorf("%w: %q is %q, but TERM_MAPPING.match is one of %s (REQ-140)",
			ErrUnsupportedDatatype, key, s, matchCodes)
	}
	return string(runes[0]), nil
}
