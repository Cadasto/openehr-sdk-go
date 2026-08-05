package simplified

// REQ-140 — the `_feeder_audit` family, decode side: FEEDER_AUDIT on any
// LOCATABLE (the composition root, a SECTION, any ENTRY, an EVENT, a CLUSTER, the
// collapsed ELEMENT a leaf hides — all nine positions the pinned corpus writes it
// at, in 14 of its 34 bodies).
//
// It is the deepest family in the grammar, and the only one whose tails nest three
// levels: `_feeder_audit/originating_system_audit/provider/_identifier:0|id`. The
// C1 tail machinery ([splitRMAttrTails]) flattens a multi-segment sub-path into one
// joined key and admits a `:index` only on the leading segment, so it cannot
// express that shape. This file adds the missing step instead of widening it:
// [rmattrChildGroups] partitions one group's tails into the family's own suffixes
// and a **re-rooted [rmattrGroup] per sub-path segment**, so each level is decoded
// by the machinery that already owns it — a FEEDER_AUDIT_DETAILS party by C2's
// [partyLeafSuffixes], its identifier list by that party's own nested
// `_identifier:N`, an item id by the DV_IDENTIFIER leaf builder.
//
// Two shapes are corpus corrections recorded in wire.md § REQ-140:
//
//   - the FEEDER_AUDIT_DETAILS `subject` spells PARTY_SELF **explicitly**, as
//     `subject|_type: "PARTY_SELF"` (`ehrbase_conformance_party_self.json`). It has
//     to: the attribute is RM-optional there, so the absence of every party key
//     already means *absent*, unlike a PARTICIPATION performer or an ENTRY
//     `subject` where absence is how PARTY_SELF is spelled. `|_type` reaches no
//     other party position.
//   - `|time` is optional on both audits (two of the corpus's three
//     `feeder_system_audit`s omit it) while `|system_id` is RM-mandatory and always
//     written.
//
// `other_details` (an ITEM_STRUCTURE) is deferred on both sides: the reference's
// suffix set has no channel for it and no corpus body writes one, so it is a typed
// refusal naming the key rather than a silent drop.

import (
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
)

// feederAuditItemIDLists maps the two DV_IDENTIFIER list segments onto the
// canonical FEEDER_AUDIT attribute each fills. The FLAT segment is **singular**
// where the RM attribute is plural — the reference's spelling.
var feederAuditItemIDLists = map[string]string{
	"originating_system_item_id": "originating_system_item_ids",
	"feeder_system_item_id":      "feeder_system_item_ids",
}

// feederAuditDetailSegs are the two FEEDER_AUDIT_DETAILS sub-objects. The segment
// name is the canonical attribute name, so it doubles as the key to write.
var feederAuditDetailSegs = map[string]bool{
	"originating_system_audit": true,
	"feeder_system_audit":      true,
}

// feederAuditContentSegs maps the two spellings of the one RM attribute
// `original_content` onto the datatype each names. FEEDER_AUDIT.original_content is
// a DV_ENCAPSULATED, and the reference disambiguates the choice **by key name**
// rather than by a `_type` suffix.
var feederAuditContentSegs = map[string]string{
	"original_content":            "DV_PARSABLE",
	"original_content_multimedia": "DV_MULTIMEDIA",
}

// feederAuditDetailsOwnTails is FEEDER_AUDIT_DETAILS' own-suffix grammar.
// `system_id` is RM-mandatory; the other two are optional and carried as written.
var feederAuditDetailsOwnTails = map[string]bool{"system_id": true, "version_id": true, "time": true}

// feederAuditDetailsParties are its three party sub-objects. `location` and
// `provider` are PARTY_IDENTIFIED; `subject` is the abstract PARTY_PROXY, which is
// what gives it the `|_type` spelling the others do not have.
var feederAuditDetailsParties = map[string]bool{"location": true, "subject": true, "provider": true}

// rmattrChildGroups partitions g's tails into the family's own suffixes (keyed by
// suffix name, "" for a bare value — the map shape [dvFromSuffixes] takes) and one
// re-rooted [rmattrGroup] per sub-path segment instance, keyed by segment id and
// ordered by list slot.
//
// A child group's base is g's own FLAT prefix and its family the segment id, so
// [rmattrGroup.prefix] and [rmattrGroup.key] keep reproducing the key the payload
// author wrote at every depth — which is what lets a refusal three levels down name
// a real FLAT key, and what lets the party and datatype decoders be reused verbatim.
//
// The `:index` rules are the ones [checkRMAttrIndexes] applies to a `_family:N`
// sequence, for the same reasons: bounded by [maxRepeatIndex], no two spellings of
// one slot, no gaps (a gap would fabricate an empty instance out of a malformed
// payload). Whether a segment may be a list at all is the caller's judgement — a
// single-valued segment simply has exactly one instance.
func rmattrChildGroups(g rmattrGroup) (map[string]any, map[string][]rmattrGroup, error) {
	own := make(map[string]any)
	bySeg := make(map[string]map[int]*rmattrGroup)
	for _, tail := range slices.Sorted(maps.Keys(g.tails)) {
		val := g.tails[tail]
		rest, isSub := strings.CutPrefix(tail, "/")
		if !isSub {
			// "" is the bare value, "|suffix" one of the family's own suffixes.
			own[strings.TrimPrefix(tail, "|")] = val
			continue
		}
		seg, remainder := rest, ""
		if cut := strings.IndexAny(rest, "/|"); cut >= 0 {
			seg, remainder = rest[:cut], rest[cut:]
		}
		id, idx, err := rmattrSegIndex(g, tail, seg)
		if err != nil {
			return nil, nil, err
		}
		slot := max(idx, 0)
		if slot > maxRepeatIndex {
			return nil, nil, fmt.Errorf("%w: %q :index %d exceeds bound %d", ErrUnknownPath, g.key(tail), slot, maxRepeatIndex)
		}
		if bySeg[id] == nil {
			bySeg[id] = make(map[int]*rmattrGroup)
		}
		child := bySeg[id][slot]
		if child == nil {
			child = &rmattrGroup{base: g.prefix(), family: id, index: idx, tails: make(map[string]any)}
			bySeg[id][slot] = child
		} else if child.index != idx {
			return nil, nil, fmt.Errorf("%w: %q collides with another spelling of :index %d; remove one",
				ErrUnknownPath, g.key(tail), slot)
		}
		if _, taken := child.tails[remainder]; taken {
			return nil, nil, fmt.Errorf("%w: %q collides with another spelling of the same sub-path; remove one",
				ErrUnsupportedDatatype, g.key(tail))
		}
		child.tails[remainder] = val
	}
	children := make(map[string][]rmattrGroup, len(bySeg))
	for id, slots := range bySeg {
		dense := make([]rmattrGroup, 0, len(slots))
		for i := range len(slots) {
			child, ok := slots[i]
			if !ok {
				return nil, nil, fmt.Errorf("%w: %s/%s sits in a sparse :index sequence (no :%d); the occurrences must run 0..%d",
					ErrUnknownPath, g.prefix(), id, i, len(slots)-1)
			}
			dense = append(dense, *child)
		}
		children[id] = dense
	}
	return own, children, nil
}

// rmattrChildSingle returns the one instance of a single-valued sub-path segment.
// A `:index` above 0 addresses a list slot the RM attribute does not have; the
// index-less spelling and `:0` are one instance, since the OPT-free FLAT ↔
// STRUCTURED interconversion re-spells every segment with an explicit index.
func rmattrChildSingle(g rmattrGroup, seg string, insts []rmattrGroup) (rmattrGroup, error) {
	notSingle := fmt.Errorf("%w: %q addresses a single-valued RM attribute, not an indexed list",
		ErrUnknownPath, g.prefix()+"/"+seg)
	if len(insts) != 1 {
		return rmattrGroup{}, notSingle
	}
	if insts[0].index > 0 {
		return rmattrGroup{}, notSingle
	}
	return insts[0], nil
}

// decodeRMAttrFeederAudit decodes one `_feeder_audit` instance into a canonical
// FEEDER_AUDIT.
//
// FEEDER_AUDIT has no scalar attribute at all, so the family carries no own suffix
// and a bare value or a `|suffix` on it is refused. `originating_system_audit` is
// RM-mandatory and required rather than defaulted: fabricating an empty
// FEEDER_AUDIT_DETAILS would put a system id of "" on the wire.
func decodeRMAttrFeederAudit(g rmattrGroup, _ string) (any, error) {
	own, children, err := rmattrChildGroups(g)
	if err != nil {
		return nil, err
	}
	if len(own) > 0 {
		suffix := slices.Sorted(maps.Keys(own))[0]
		return nil, fmt.Errorf("%w: %q is not part of the %s grammar (REQ-140); every FEEDER_AUDIT attribute is a sub-path",
			ErrUnsupportedDatatype, g.key(ownTail(suffix)), g.family)
	}
	fa := map[string]any{"_type": "FEEDER_AUDIT"}
	var contentSeg string
	for _, seg := range slices.Sorted(maps.Keys(children)) {
		insts := children[seg]
		switch {
		case feederAuditItemIDLists[seg] != "":
			ids := make([]any, 0, len(insts))
			for _, in := range insts {
				dv, err := feederLeafValue(in, "DV_IDENTIFIER")
				if err != nil {
					return nil, err
				}
				ids = append(ids, dv)
			}
			fa[feederAuditItemIDLists[seg]] = ids
		case feederAuditDetailSegs[seg]:
			in, err := rmattrChildSingle(g, seg, insts)
			if err != nil {
				return nil, err
			}
			details, err := decodeFeederAuditDetails(in)
			if err != nil {
				return nil, err
			}
			fa[seg] = details
		case feederAuditContentSegs[seg] != "":
			if contentSeg != "" {
				return nil, fmt.Errorf("%w: %q and %q both spell FEEDER_AUDIT.original_content; the DV_ENCAPSULATED choice is by key name, so only one may appear",
					ErrUnsupportedDatatype, g.prefix()+"/"+contentSeg, g.prefix()+"/"+seg)
			}
			contentSeg = seg
			in, err := rmattrChildSingle(g, seg, insts)
			if err != nil {
				return nil, err
			}
			content, err := feederLeafValue(in, feederAuditContentSegs[seg])
			if err != nil {
				return nil, err
			}
			fa["original_content"] = content
		default:
			return nil, feederUnknownSegment(g, insts[0], seg, "FEEDER_AUDIT")
		}
	}
	if _, ok := fa["originating_system_audit"]; !ok {
		return nil, fmt.Errorf("%w: %s is missing the required /originating_system_audit (RM-mandatory on FEEDER_AUDIT)",
			ErrUnsupportedDatatype, g.prefix())
	}
	return fa, nil
}

// decodeFeederAuditDetails decodes one `originating_system_audit` /
// `feeder_system_audit` sub-object: its three own suffixes and its three party
// positions, each through C2's party grammar.
func decodeFeederAuditDetails(g rmattrGroup) (any, error) {
	own, children, err := rmattrChildGroups(g)
	if err != nil {
		return nil, err
	}
	for _, suffix := range slices.Sorted(maps.Keys(own)) {
		if !feederAuditDetailsOwnTails[suffix] {
			return nil, fmt.Errorf("%w: %q is not part of the %s grammar (REQ-140)",
				ErrUnsupportedDatatype, g.key(ownTail(suffix)), g.family)
		}
	}
	systemID, present, err := feederOwnString(g, own, "system_id")
	if err != nil {
		return nil, err
	}
	if !present {
		return nil, fmt.Errorf("%w: %s is missing the required |system_id (RM-mandatory on FEEDER_AUDIT_DETAILS)",
			ErrUnsupportedDatatype, g.prefix())
	}
	details := map[string]any{"_type": "FEEDER_AUDIT_DETAILS", "system_id": systemID}
	if versionID, present, err := feederOwnString(g, own, "version_id"); err != nil {
		return nil, err
	} else if present {
		details["version_id"] = versionID
	}
	if when, present, err := feederOwnString(g, own, "time"); err != nil {
		return nil, err
	} else if present {
		details["time"] = map[string]any{"_type": "DV_DATE_TIME", "value": when}
	}
	for _, seg := range slices.Sorted(maps.Keys(children)) {
		if !feederAuditDetailsParties[seg] {
			return nil, feederUnknownSegment(g, children[seg][0], seg, "FEEDER_AUDIT_DETAILS")
		}
		in, err := rmattrChildSingle(g, seg, children[seg])
		if err != nil {
			return nil, err
		}
		party, err := feederParty(in, seg == "subject")
		if err != nil {
			return nil, err
		}
		details[seg] = party
	}
	return details, nil
}

// feederParty decodes one FEEDER_AUDIT_DETAILS party position. proxy says the
// attribute is typed PARTY_PROXY (`subject`), which is what admits the explicit
// `|_type: PARTY_SELF` spelling; `location` and `provider` are PARTY_IDENTIFIED and
// have no such channel.
func feederParty(g rmattrGroup, proxy bool) (map[string]any, error) {
	if proxy {
		return partyProxyLeafSuffixes(g)
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

// feederLeafValue decodes a re-rooted child group whose whole value is one datatype
// in its own suffix form — an item id, an `original_content`. The child's tails
// *are* the suffix map, so a sub-path of its own is refused naming the key: the
// grammar stops here, and a value the suffix set cannot capture rides that key's
// `|raw` instead (which is how a charset-carrying `original_content` stays
// lossless).
func feederLeafValue(g rmattrGroup, rmType string) (map[string]any, error) {
	sfx := make(map[string]any, len(g.tails))
	for _, tail := range slices.Sorted(maps.Keys(g.tails)) {
		if strings.HasPrefix(tail, "/") {
			return nil, fmt.Errorf("%w: %q is not part of the %s grammar (REQ-140)",
				ErrUnsupportedDatatype, g.key(tail), g.family)
		}
		sfx[strings.TrimPrefix(tail, "|")] = g.tails[tail]
	}
	dv, err := dvFromSuffixes(rmType, false, sfx)
	if err != nil {
		return nil, fmt.Errorf("simplified: decode %q: %w", g.prefix(), err)
	}
	return dv, nil
}

// feederOwnString reads one of a group's own suffixes as a string. present is false
// when the payload did not spell it, so an RM-optional attribute stays absent rather
// than becoming a coerced empty string; a value of another JSON kind is refused
// here, where the FLAT key is still in hand to name it.
func feederOwnString(g rmattrGroup, own map[string]any, suffix string) (string, bool, error) {
	v, present := own[suffix]
	if !present {
		return "", false, nil
	}
	s, ok := v.(string)
	if !ok {
		return "", true, fmt.Errorf("%w: %q must be a string, got %T",
			ErrUnsupportedDatatype, g.key(ownTail(suffix)), v)
	}
	return s, true, nil
}

// feederUnknownSegment is the refusal for a sub-path segment outside a level's
// grammar. `other_details` gets its own message: it is a **named RM attribute this
// codec defers** (an ITEM_STRUCTURE with no channel in the reference's suffix set
// and no corpus fixture), not a typo, and the census reads the distinction.
func feederUnknownSegment(g rmattrGroup, child rmattrGroup, seg, owner string) error {
	tail := slices.Sorted(maps.Keys(child.tails))[0]
	key := child.key(tail)
	if seg == "other_details" {
		return fmt.Errorf("%w: %q addresses %s.other_details, an ITEM_STRUCTURE the reference's suffix set has no channel for and the pinned corpus never writes — deferred rather than dropped (REQ-140, see deviations.md)",
			ErrUnsupportedDatatype, key, owner)
	}
	return fmt.Errorf("%w: %q is not part of the %s grammar (REQ-140)", ErrUnsupportedDatatype, key, g.family)
}

// feederItemIDPath is the FLAT prefix of one DV_IDENTIFIER list slot, shared by the
// two list segments on the encode side.
func feederItemIDPath(base, seg string, i int) string {
	return base + "/" + seg + ":" + strconv.Itoa(i)
}
