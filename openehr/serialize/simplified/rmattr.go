package simplified

// REQ-140 — underscore-prefixed RM attributes, decode side.
//
// The Simplified Formats' *RM Attributes prefix* rule addresses an optional RM
// attribute the template does not constrain by prefixing its RM attribute name
// with `_` at the node it belongs to. A FLAT path segment starting with `_`
// (after any `:N` index is stripped) therefore **ends** Web Template
// resolution: the segments before it name the owning node, and everything from
// the `_` segment on is grammar this file owns, keyed by the owner's RM class.
//
// The grammar is recursive — a family's value decomposes by its own RM type and
// families nest (`…/_feeder_audit/originating_system_audit/provider/_identifier:0|id`)
// — so a group is collected as the *tail* of each key after the family
// segment, not as a flat suffix set: `""` for a bare value, `"|meaning"` for a
// suffix, `"/originating_system_audit|time"` for a subpath plus suffix. Later
// phases add families that read the subpath tails; the C0 families use `""` and
// `"|suffix"` only.
//
// Two OBJECT_ID subtype policies are load-bearing and both are derived from the
// vendored PROBE-086 corpus (ADR 0014 — the reference spelling wins):
//
//   - `_uid` is one bare string, so the concrete UID_BASED_ID subtype has no
//     channel of its own and is re-derived from the lexical form
//     ([uidBasedIDType]): the corpus writes a three-part OBJECT_VERSION_ID at
//     the composition root (`6e3a…::ehrbase.org::1`) and a bare UUID —
//     HIER_OBJECT_ID — on every ENTRY, SECTION, CLUSTER and collapsed ELEMENT.
//     Encode refuses a value whose lexical form implies the *other* subtype
//     rather than letting the round-trip retype it (see rmattr_encode.go).
//   - OBJECT_REF (`_work_flow_id`, `_guideline_id`) discriminates its
//     OBJECT_ID by the **presence of `|id_scheme`**: with it the id is a
//     GENERIC_ID carrying that scheme (the corpus's `HOSPITAL-NS`), without it
//     a HIER_OBJECT_ID. No other OBJECT_ID subtype is expressible — GENERIC_ID
//     is the only one with a scheme, and every scheme-less subtype would
//     collide with HIER_OBJECT_ID on the wire — so encode refuses the rest.
//     Note the suffix is `|namespace`, not the party grammar's `|id_namespace`;
//     verified against `ehrbase_conformance_{action,evaluation,instruction,
//     observation,admin_entry,feeder_audit_multimedia}`.

import (
	"cmp"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/rminfo"
)

// rmattrGroup is one `_`-family instance's FLAT entries: the tail of each key
// after the family segment, mapped to that key's value. base is the owner's
// FLAT path and index the family's `:N` (-1 when the key carried none), which
// together with family reproduce the original FLAT key for an error message —
// the codec must always name the key the payload author wrote.
type rmattrGroup struct {
	base   string
	family string
	index  int
	tails  map[string]any
}

// prefix is the FLAT spelling of the family instance itself, without any tail:
// `<base>/_link:0`. Every refusal names this or a full key built from it, which
// is also how the PROBE-086 census scopes an exclusion (SKIPPED.md).
func (g rmattrGroup) prefix() string {
	if g.index >= 0 {
		return g.base + "/" + g.family + ":" + strconv.Itoa(g.index)
	}
	return g.base + "/" + g.family
}

// key is the full FLAT key a tail arrived under.
func (g rmattrGroup) key(tail string) string { return g.prefix() + tail }

// slot is the effective list position of this instance. A missing `:N` is
// position 0: the OPT-free FLAT ↔ STRUCTURED interconversion re-spells every
// segment with an explicit index, so `_link` and `_link:0` must mean the same
// thing — the same equivalence [selectElem] already applies to clinical
// segments. Two groups landing on one slot is caught by [checkRMAttrIndexes].
func (g rmattrGroup) slot() int { return max(g.index, 0) }

// rmattrFamily is one underscore-prefixed RM attribute family: the RM
// attribute name it addresses (which is *not* always the family name with the
// `_` removed — `_work_flow_id` addresses `workflow_id`), whether it is an
// indexed list, whether it decorates the owner or the DataValue a collapsed
// ELEMENT leaf holds, and how one instance's group decodes into canonical JSON.
//
// decode receives the anchor — the leaf datatype of a value-decoration family's
// owner ("" for every other family), which `_normal_range` needs to decode its
// bounds with.
type rmattrFamily struct {
	attr string
	// attrType, when set, is the RM type the owner must declare attr as for this
	// family to be admitted. It exists for the one attribute the reference spells
	// two ways: DV_TEMPORAL redefines `accuracy` as a DV_DURATION object where
	// DV_AMOUNT declares a Real, and the Real has the scalar `|accuracy` suffix —
	// so `_accuracy` must reach the temporal types and only them, or one attribute
	// would have two channels at one owner.
	attrType string
	list     bool
	// value marks a family that decorates the **DataValue** a collapsed ELEMENT
	// leaf holds rather than the LOCATABLE itself: one FLAT path addresses both
	// (the Web Template folds ELEMENT.value into its leaf node), so the RM class
	// the family is judged against and the object it lands on are the value's,
	// not the ELEMENT's. See rmattr_value.go.
	value  bool
	decode func(g rmattrGroup, anchor string) (any, error)
}

// rmattrFamilies is the vocabulary this codec carries. A family absent here is
// ErrUnknownPath — the fail-loud posture REQ-053 sets and REQ-140 inherits, and
// the mechanism by which the families later phases own (`_feeder_audit`,
// `_charset`, `_language`, `_encoding`, `_thumbnail`) and the two the plan defers
// indefinitely (`_instruction_details`, `_wf_definition`) stay visible in the
// PROBE-086 census instead of decoding into nothing.
//
// `_identifier` is deliberately **not** a family here, though the corpus writes
// it: it is only ever reached *inside* a party — nested in
// `_health_care_facility`'s tails, at the ENTRY `subject` leaf, or (Phase C3)
// under a FEEDER_AUDIT_DETAILS party — so the one party implementation owns it
// (rmattr_party.go) and no owner exists for the router to judge it against. The
// composition's **composer** is the one path that would reach it as a family, and
// that is the ADR 0015 boundary flat_decode.go refuses by name.
//
// Which *owners* admit a family is not listed here: it is read off the RM
// itself (rminfo), so `_work_flow_id` is ENTRY-only because only the five ENTRY
// subtypes declare `workflow_id`, and `_uid` reaches every LOCATABLE because
// they all declare `uid`. That is the spec's own rule ("the node it belongs
// to") rather than a table that could drift from the BMM. For a `value` family
// the class consulted is the leaf datatype instead (rmattr_value.go), which is
// what makes `_normal_range` DV_ORDERED-only and `_mapping` DV_TEXT-only.
var rmattrFamilies = map[string]rmattrFamily{
	"_uid":                  {attr: "uid", decode: decodeRMAttrUID},
	"_link":                 {attr: "links", list: true, decode: decodeRMAttrLink},
	"_work_flow_id":         {attr: "workflow_id", decode: decodeRMAttrObjectRef},
	"_guideline_id":         {attr: "guideline_id", decode: decodeRMAttrObjectRef},
	"_end_time":             {attr: "end_time", decode: decodeRMAttrEndTime},
	"_location":             {attr: "location", decode: decodeRMAttrLocation},
	"_health_care_facility": {attr: "health_care_facility", decode: decodeRMAttrHealthCareFacility},
	"_participation":        {attr: "participations", list: true, decode: decodeRMAttrParticipation},
	"_other_participation":  {attr: "other_participations", list: true, decode: decodeRMAttrParticipation},
	"_normal_range":         {attr: "normal_range", value: true, decode: decodeRMAttrNormalRange},
	"_other_reference_ranges": {
		attr: "other_reference_ranges", list: true, value: true,
		decode: decodeRMAttrReferenceRange,
	},
	"_mapping":      {attr: "mappings", list: true, value: true, decode: decodeRMAttrTermMapping},
	"_null_flavour": {attr: "null_flavour", decode: decodeRMAttrNullFlavour},
	"_null_reason":  {attr: "null_reason", decode: decodeRMAttrNullReason},
	"_charset":      {attr: "charset", value: true, decode: decodeRMAttrCodePhraseMember},
	"_language":     {attr: "language", value: true, decode: decodeRMAttrCodePhraseMember},
	"_encoding":     {attr: "encoding", value: true, decode: decodeRMAttrCodePhraseMember},
	"_thumbnail":    {attr: "thumbnail", value: true, decode: decodeRMAttrThumbnail},
	"_accuracy": {
		attr: "accuracy", attrType: "DV_DURATION", value: true,
		decode: decodeRMAttrTemporalAccuracy,
	},
}

// rmattrOwner is the resolved owner of an `_`-family group: the RM class name
// the family is judged against, and a lazy accessor for the canonical-JSON node
// to decorate.
//
// The accessor is deferred on purpose. Materialising the owner walks (and
// gap-fills) the rebuilt tree, so doing it before the family and its suffixes
// are validated would leave a half-built node behind on a refusal. attr is the
// RM attribute about to be written: [walkAQL] needs the following path segment
// to resolve the abstract ITEM_STRUCTURE slot the Web Template collapses.
type rmattrOwner struct {
	kind string
	// leaf is the RM type of the DataValue a collapsed ELEMENT owner holds under
	// `value` ("" when the owner is not a collapsed leaf) — the **anchor** the
	// value-decoration families are judged and decoded against (REQ-140 § C1).
	leaf    string
	resolve func(attr string) (map[string]any, error)
}

// splitRMAttrKey splits a parsed FLAT key at its first `_`-prefixed path
// segment. base is the owner's FLAT path (indices preserved), family and index
// come from the `_` segment, and tail is everything after it — the remaining
// path segments, each with a leading "/", then "|suffix" when the key carried
// one. ok is false for a key with no `_` segment, and for one whose *root*
// segment is `_`-prefixed (no owner could precede it), which then fails as an
// ordinary unresolvable path.
func splitRMAttrKey(pk parsedKey) (base, family string, index int, tail string, ok bool) {
	for i, seg := range pk.segs {
		if !strings.HasPrefix(seg.id, "_") {
			continue
		}
		if i == 0 {
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

// writeFlatSeg re-spells one parsed FLAT segment, so a reconstructed base or
// tail is byte-identical to the key it came from ([parseFlatKey] has already
// rejected any non-canonical index spelling).
func writeFlatSeg(b *strings.Builder, s flatSeg) {
	b.WriteString(s.id)
	if s.idx >= 0 {
		b.WriteByte(':')
		b.WriteString(strconv.Itoa(s.idx))
	}
}

// rmattrGroups collects the `_`-family groups out of a FLAT body's content
// keys, one per (base path, family, :index), and returns them in a stable order
// (base, then family, then index) so a body with two bad groups names the same
// one on every run. The second return maps each (base, family) to the raw
// indexes seen for it, which [checkRMAttrIndexes] needs to judge the sequence.
//
// Keys are consumed from content: what stays behind is ordinary leaf data.
func rmattrGroups(content map[string]any) ([]rmattrGroup, map[string]map[int]bool, error) {
	byInstance := make(map[string]*rmattrGroup)
	indexes := make(map[string]map[int]bool)
	for _, key := range slices.Sorted(maps.Keys(content)) {
		pk, err := parseFlatKey(key)
		if err != nil {
			return nil, nil, err
		}
		base, family, index, tail, isAttr := splitRMAttrKey(pk)
		if !isAttr {
			continue
		}
		val := content[key]
		delete(content, key)
		fam := base + "\x00" + family
		id := fam + "\x00" + strconv.Itoa(index)
		g := byInstance[id]
		if g == nil {
			g = &rmattrGroup{base: base, family: family, index: index, tails: make(map[string]any)}
			byInstance[id] = g
		}
		g.tails[tail] = val
		if indexes[fam] == nil {
			indexes[fam] = make(map[int]bool)
		}
		indexes[fam][index] = true
	}
	groups := make([]rmattrGroup, 0, len(byInstance))
	for _, g := range byInstance {
		groups = append(groups, *g)
	}
	slices.SortFunc(groups, func(a, b rmattrGroup) int {
		return cmp.Or(
			strings.Compare(a.base, b.base),
			strings.Compare(a.family, b.family),
			cmp.Compare(a.index, b.index),
		)
	})
	return groups, indexes, nil
}

// rmattrDecode routes one `_`-family group to its typed decoder and places the
// result on the owner (REQ-140). The order of the checks is the error taxonomy:
//
//  1. an unrecognised family, or one the owner's RM class does not declare, is
//     ErrUnknownPath — the same refusal a path that reaches no Web Template
//     node gets, because that is exactly what it is;
//  2. a recognised family whose `:index` breaks the FLAT indexing rules is
//     ErrUnknownPath (structurally inadmissible, like a sparse clinical index);
//  3. a recognised family carrying a suffix outside its grammar, or missing an
//     RM-mandatory one, is ErrUnsupportedDatatype naming the offending key;
//  4. only then is the owner materialised, so a refusal leaves no half-built
//     node in the rebuilt tree.
func rmattrDecode(owner rmattrOwner, g rmattrGroup, indexes map[int]bool, budget *allocBudget) error {
	fam, known := rmattrFamilies[g.family]
	if !known {
		return fmt.Errorf("%w: %q (no such RM attribute family — see docs/specifications/wire.md § REQ-140 for the vocabulary this codec carries)",
			ErrUnknownPath, g.prefix())
	}
	// A value-decoration family is judged against the leaf datatype, not the
	// ELEMENT that holds it (both answer to the same FLAT path). owner.leaf is ""
	// for every owner that is not a collapsed leaf, which declares nothing — so
	// `_normal_range` on a SECTION is refused by this one check.
	judged := owner.kind
	if fam.value {
		judged = owner.leaf
	}
	declaredType, declared := rminfo.Default.AttributeRMType(judged, fam.attr)
	if !declared {
		return fmt.Errorf("%w: %q (%s declares no %s attribute)", ErrUnknownPath, g.prefix(),
			cmp.Or(judged, owner.kind), fam.attr)
	}
	// The attribute exists on this owner but not in the type the family spells it
	// as — the DV_TEMPORAL / DV_AMOUNT `accuracy` split, where the Real form rides
	// the scalar `|accuracy` suffix instead (see [rmattrFamily.attrType]).
	if fam.attrType != "" && declaredType != fam.attrType {
		return fmt.Errorf("%w: %q addresses %s.%s, which is typed %s here rather than %s — that form has its own suffix (REQ-140)",
			ErrUnknownPath, g.prefix(), judged, fam.attr, declaredType, fam.attrType)
	}
	if err := checkRMAttrIndexes(g, fam, indexes); err != nil {
		return err
	}
	val, err := fam.decode(g, owner.leaf)
	if err != nil {
		return err
	}
	node, err := owner.resolve(fam.host())
	if err != nil {
		return err
	}
	if fam.value {
		if node, err = rmattrValueNode(node, g, judged); err != nil {
			return err
		}
	}
	if fam.list {
		return placeRMAttrList(node, fam.attr, g, val, budget)
	}
	if _, taken := node[fam.attr]; taken {
		return fmt.Errorf("%w: %q duplicates a value already placed on %s.%s", ErrUnknownPath, g.prefix(), judged, fam.attr)
	}
	node[fam.attr] = val
	return nil
}

// host names the owner attribute the family's value is reached through: `value`
// for a value decoration (the collapsed ELEMENT's DataValue slot), the family's
// own attribute otherwise. It is what [rmattrOwner.resolve] walks to, and — for
// a collapsed ELEMENT — the lookahead [concreteType] resolves the elided
// ITEM_STRUCTURE with.
func (f rmattrFamily) host() string {
	if f.value {
		return "value"
	}
	return f.attr
}

// rmattrValueNode narrows a resolved collapsed-ELEMENT node to the DataValue a
// value-decoration family decorates.
//
// The decoration has no meaning without a value to carry it, and the clinical
// leaf loop has already run by the time the router does (decodeFlat), so an
// absent `value` here means the payload spelled a decoration for a leaf it never
// spelled: refuse rather than fabricate a bare DataValue out of the anchor type,
// which would invent a magnitude or a rubric the author never wrote. (An ELEMENT
// legitimately *without* a value carries `_null_flavour` instead, which is an
// owner family and never reaches here.)
func rmattrValueNode(node map[string]any, g rmattrGroup, anchor string) (map[string]any, error) {
	dv, ok := node["value"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%w: %q decorates a %s value the body does not carry; spell the leaf itself beside it",
			ErrUnsupportedDatatype, g.prefix(), anchor)
	}
	return dv, nil
}

// checkRMAttrIndexes applies the FLAT `:index` rules to a `_family:N` sequence.
// They are the clinical-segment rules, not a second set: canonical spelling and
// the negative-index rejection live in [parseFlatKey], the repeat bound is
// [maxRepeatIndex], and a gap is refused for the same reason [checkNoPhantoms]
// refuses one on a clinical container — a missing occurrence would gap-fill a
// phantom instance out of a malformed payload.
//
// A scalar family admits `:0` beside the index-less spelling because the
// OPT-free FLAT ↔ STRUCTURED interconversion re-spells every segment with an
// explicit index; anything higher is a list where the RM has a single value.
func checkRMAttrIndexes(g rmattrGroup, fam rmattrFamily, indexes map[int]bool) error {
	if !fam.list {
		if g.index > 0 {
			return fmt.Errorf("%w: %q (%s addresses a single-valued RM attribute, not an indexed list)", ErrUnknownPath, g.prefix(), g.family)
		}
		if len(indexes) > 1 {
			// Either `_uid` beside `_uid:0`, or `_uid:0` beside `_uid:1`: both are
			// two spellings competing for one RM slot, and picking either would be
			// the silent overwrite [placeLeaf] refuses for a clinical leaf.
			return fmt.Errorf("%w: %q is spelled more than once, but %s addresses a single-valued RM attribute; remove all but one",
				ErrUnknownPath, g.prefix(), g.family)
		}
		return nil
	}
	if g.slot() > maxRepeatIndex {
		return fmt.Errorf("%w: %q :index %d exceeds bound %d", ErrUnknownPath, g.prefix(), g.slot(), maxRepeatIndex)
	}
	slots := make(map[int]bool, len(indexes))
	for idx := range indexes {
		slot := max(idx, 0)
		if slots[slot] {
			return fmt.Errorf("%w: %q collides with another spelling of :index %d; remove one", ErrUnknownPath, g.prefix(), slot)
		}
		slots[slot] = true
	}
	for i := range len(slots) {
		if !slots[i] {
			return fmt.Errorf("%w: %q sits in a sparse :index sequence (no :%d); the occurrences must run 0..%d",
				ErrUnknownPath, g.prefix(), i, len(slots)-1)
		}
	}
	return nil
}

// placeRMAttrList writes one indexed family instance into the owner's
// canonical array attribute. [checkRMAttrIndexes] has already proved the
// sequence is dense, so every slot the growth creates is filled by its own
// group; a collision is still refused rather than overwritten.
func placeRMAttrList(node map[string]any, attr string, g rmattrGroup, val any, budget *allocBudget) error {
	arr, _ := node[attr].([]any)
	slot := g.slot()
	if need := slot + 1 - len(arr); need > 0 {
		if err := budget.add(need); err != nil {
			return err
		}
		for len(arr) <= slot {
			arr = append(arr, nil)
		}
	}
	if arr[slot] != nil {
		return fmt.Errorf("%w: %q duplicates the value already at %s:%d", ErrUnknownPath, g.prefix(), attr, slot)
	}
	arr[slot] = val
	node[attr] = arr
	return nil
}

// --- suffix helpers -----------------------------------------------------

// checkRMAttrTails rejects any tail outside a family's grammar, naming the
// offending FLAT key. The message deliberately avoids the "unexpected |x for
// TYPE" phrasing the datatype allowlist uses: that shape is parsed by the
// PROBE-086 harness to scope a refusal to one suffix, and a family refusal
// scopes to the whole family instance.
func checkRMAttrTails(g rmattrGroup, allowed map[string]bool) error {
	for _, tail := range slices.Sorted(maps.Keys(g.tails)) {
		if !allowed[tail] {
			return fmt.Errorf("%w: %q is not part of the %s grammar (REQ-140)", ErrUnsupportedDatatype, g.key(tail), g.family)
		}
	}
	return nil
}

// rmattrString returns a required tail's value as a string, failing loudly when
// it is absent (an RM-mandatory attribute must not become a coerced empty
// string) or of the wrong JSON kind.
func rmattrString(g rmattrGroup, tail string) (string, error) {
	v, present := g.tails[tail]
	if !present {
		return "", fmt.Errorf("%w: %s is missing the required %s", ErrUnsupportedDatatype, g.prefix(), rmattrTailLabel(tail))
	}
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("%w: %q must be a string, got %T", ErrUnsupportedDatatype, g.key(tail), v)
	}
	return s, nil
}

// rmattrTailLabel names a tail for a "missing" message, where quoting a key
// that is not in the body would be misleading.
func rmattrTailLabel(tail string) string {
	if tail == "" {
		return "bare value"
	}
	return tail
}

// rmattrBareOnly is the grammar of a bare-value family: the key itself and
// nothing beside it, so a stray suffix or subpath is refused rather than
// dropped.
var rmattrBareOnly = map[string]bool{"": true}

// --- families -----------------------------------------------------------

// decodeRMAttrUID decodes `_uid` on any LOCATABLE. See the file comment for the
// subtype policy.
func decodeRMAttrUID(g rmattrGroup, _ string) (any, error) {
	if err := checkRMAttrTails(g, rmattrBareOnly); err != nil {
		return nil, err
	}
	value, err := rmattrString(g, "")
	if err != nil {
		return nil, err
	}
	return map[string]any{"_type": uidBasedIDType(value), "value": value}, nil
}

// uidBasedIDType names the concrete UID_BASED_ID subtype a `_uid` string
// spells. OBJECT_VERSION_ID's lexical form is
// `object_id '::' creating_system_id '::' version_tree_id` (REQ-120, validated
// by rm.ParseObjectVersionID); everything else — a bare UUID, an OID, a
// two-part `root::extension` — is a HIER_OBJECT_ID, whose form admits them all.
//
// Shared with encode, which refuses a value whose form implies the other
// subtype, so the two directions cannot disagree about what a string means.
func uidBasedIDType(value string) string {
	if _, err := rm.ParseObjectVersionID(value); err == nil {
		return "OBJECT_VERSION_ID"
	}
	return "HIER_OBJECT_ID"
}

// linkTails is LINK's suffix set. All three are RM-mandatory.
var linkTails = map[string]bool{"|meaning": true, "|type": true, "|target": true}

// decodeRMAttrLink decodes one `_link:N` instance into a canonical LINK.
// `|meaning` and `|type` rebuild as plain DV_TEXT — the suffixes carry a string
// and nothing else, which is why encode refuses a coded or decorated value
// rather than narrowing it (rmattr_encode.go).
func decodeRMAttrLink(g rmattrGroup, _ string) (any, error) {
	if err := checkRMAttrTails(g, linkTails); err != nil {
		return nil, err
	}
	meaning, err := rmattrString(g, "|meaning")
	if err != nil {
		return nil, err
	}
	typ, err := rmattrString(g, "|type")
	if err != nil {
		return nil, err
	}
	target, err := rmattrString(g, "|target")
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"_type":   "LINK",
		"meaning": textJSON(meaning),
		"type":    textJSON(typ),
		"target":  map[string]any{"_type": "DV_EHR_URI", "value": target},
	}, nil
}

// objectRefTails is the OBJECT_REF suffix set as the corpus spells it. Note
// `|namespace`, not the party grammar's `|id_namespace`.
var objectRefTails = map[string]bool{"|id": true, "|id_scheme": true, "|namespace": true, "|type": true}

// decodeRMAttrObjectRef decodes `_work_flow_id` / `_guideline_id` into a
// canonical OBJECT_REF. `|namespace` and `|type` are RM-mandatory on
// OBJECT_REF, so they are required rather than defaulted; `|id_scheme` selects
// the OBJECT_ID subtype (see the file comment).
func decodeRMAttrObjectRef(g rmattrGroup, _ string) (any, error) {
	if err := checkRMAttrTails(g, objectRefTails); err != nil {
		return nil, err
	}
	id, err := rmattrString(g, "|id")
	if err != nil {
		return nil, err
	}
	namespace, err := rmattrString(g, "|namespace")
	if err != nil {
		return nil, err
	}
	typ, err := rmattrString(g, "|type")
	if err != nil {
		return nil, err
	}
	objectID := map[string]any{"_type": "HIER_OBJECT_ID", "value": id}
	if _, scheme := g.tails["|id_scheme"]; scheme {
		s, err := rmattrString(g, "|id_scheme")
		if err != nil {
			return nil, err
		}
		objectID = map[string]any{"_type": "GENERIC_ID", "value": id, "scheme": s}
	}
	return map[string]any{"_type": "OBJECT_REF", "id": objectID, "namespace": namespace, "type": typ}, nil
}

// decodeRMAttrEndTime decodes EVENT_CONTEXT's `_end_time` — a bare DV_DATE_TIME
// value (ADR 0016: the EVENT_CONTEXT optionals ride the underscore grammar
// under the real `context` segment, not the ITS `ctx/` sketches).
func decodeRMAttrEndTime(g rmattrGroup, _ string) (any, error) {
	if err := checkRMAttrTails(g, rmattrBareOnly); err != nil {
		return nil, err
	}
	value, err := rmattrString(g, "")
	if err != nil {
		return nil, err
	}
	return map[string]any{"_type": "DV_DATE_TIME", "value": value}, nil
}

// decodeRMAttrLocation decodes EVENT_CONTEXT's `_location` — a bare String, so
// the canonical form is the JSON string itself rather than an RM object.
func decodeRMAttrLocation(g rmattrGroup, _ string) (any, error) {
	if err := checkRMAttrTails(g, rmattrBareOnly); err != nil {
		return nil, err
	}
	return rmattrString(g, "")
}
