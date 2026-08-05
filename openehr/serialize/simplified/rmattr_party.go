package simplified

// REQ-140 — the **party** grammar, decode side: PARTY_IDENTIFIED /
// PARTY_RELATED / PARTY_SELF and the PARTICIPATION that carries one.
//
// One implementation serves every position a party appears at, because the
// underscore grammar is recursive and the reference spells a party identically
// wherever it reaches one (design constraint 5):
//
//   - `context/_health_care_facility` — a party as a family's whole value;
//   - `context/_participation:N` and `<entry>/_other_participation:N` — a party
//     inlined beside the PARTICIPATION's own suffixes on the same key base;
//   - the ENTRY `subject` in-context leaf (REQ-053, flat_decode.go's party-leaf
//     siphon) — a party at a Web Template leaf;
//   - and from Phase C3, FEEDER_AUDIT_DETAILS' `/location`, `/subject` and
//     `/provider`.
//
// The grammar, exactly as the vendored PROBE-086 corpus spells it (ADR 0014):
//
//	|id  |id_scheme  |id_namespace   PARTY_IDENTIFIED.external_ref, decomposed
//	|name                            PARTY_IDENTIFIED.name
//	/_identifier:N|id|issuer|…       PARTY_IDENTIFIED.identifiers (DV_IDENTIFIER)
//	/relationship|code|value|…       PARTY_RELATED.relationship (DV_CODED_TEXT)
//
// Three discriminations are load-bearing and all three are read off the keys
// rather than guessed:
//
//   - `/relationship` selects PARTY_RELATED; any other party key alone selects
//     PARTY_IDENTIFIED; *no* party key at all is PARTY_SELF (a performer the
//     reference writes as a bare `|function` + `|mode` pair —
//     `ehrbase_conformance_party_self.json`) or, at the ENTRY `subject` leaf,
//     absence.
//   - `|id_scheme` selects the `external_ref`'s OBJECT_ID subtype, the same
//     policy C0 fixed for OBJECT_REF (rmattr.go): present ⇒ GENERIC_ID carrying
//     that scheme, absent ⇒ HIER_OBJECT_ID. Note the suffix here is
//     `|id_namespace` where the OBJECT_REF families spell `|namespace`.
//   - a PARTICIPATION spells its performer's identifiers as **inlined indexed
//     suffixes** (`|identifiers_id:N`), not as the nested `_identifier:N` a
//     standalone party uses. Both spellings rebuild the same DV_IDENTIFIER list
//     and each position accepts only its own, so a nested `_identifier:N` on a
//     participation is refused naming the key.

import (
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
)

// partyRefType is the OBJECT_REF `type` a party reference carries when the
// payload does not spell one. The reference implementation writes `|id`,
// `|id_scheme` and `|id_namespace` at a party and no `|type` at all — it
// hardcodes this value — so it is the default both directions agree on, and a
// corpus body round-trips byte-exactly without ever emitting a `|type` key.
//
// It is a *default*, not a fixed value, because real PARTY_REFs are typed
// `PERSON` and `ORGANISATION` (the vendored PROBE-076 fixture
// `clinical_content_validation.json` carries both), and dropping that would be
// the silent loss REQ-053 forbids. Encode therefore spells `|type` when — and
// only when — it differs from this default. That suffix is not invented
// vocabulary: `|type` is the reference's own spelling of **the same RM
// attribute** in the OBJECT_REF families (`_work_flow_id|type`, rmattr.go); this
// codec writes it at one more position, where the reference is lossy. Recorded in
// deviations.md.
const partyRefType = "PARTY"

// partyIdentifierSeg is the sub-path segment carrying a standalone party's
// DV_IDENTIFIER list. It is `_`-prefixed because it *is* an underscore RM
// attribute — one nested inside another family's tails rather than routed, which
// is why it has no entry in [rmattrFamilies]: every position that reaches a party
// reaches it through this grammar, so the party implementation owns it and there
// is no owner for the router to resolve it against. (The composition's
// **composer** is the exception the ADR 0015 boundary refuses — see
// flat_decode.go's siphonContext.)
const partyIdentifierSeg = "_identifier"

// partyOwnTails / partySubTails / partyListTails are the party grammar's three
// tail positions.
var (
	partyOwnTails  = map[string]bool{"id": true, "id_scheme": true, "id_namespace": true, "type": true, "name": true}
	partySubTails  = map[string]bool{"relationship": true}
	partyListTails = map[string]bool{partyIdentifierSeg: true}
)

// decodeRMAttrHealthCareFacility decodes `context/_health_care_facility` — a
// party as one family's whole value.
func decodeRMAttrHealthCareFacility(g rmattrGroup, _ string) (any, error) {
	ts, err := splitRMAttrTails(g, partyListTails)
	if err != nil {
		return nil, err
	}
	if err := ts.check(g, partyOwnTails, partySubTails); err != nil {
		return nil, err
	}
	ids, err := partyIdentifiers(g, ts)
	if err != nil {
		return nil, err
	}
	party, populated, err := partySuffixes(g, ts, ids)
	if err != nil {
		return nil, err
	}
	if !populated {
		// Unreachable while the allowlist above admits only party keys, but a
		// fabricated PARTY_IDENTIFIED with no name, identifier or reference would
		// breach the RM's own invariant, so say so rather than emit one.
		return nil, fmt.Errorf("%w: %s carries no party key (PARTY_IDENTIFIED needs at least one of |name, |id or an _identifier)",
			ErrUnsupportedDatatype, g.prefix())
	}
	return party, nil
}

// partySuffixes assembles the canonical party carried by one grammar position's
// tails: the four own suffixes plus the PARTY_RELATED `/relationship`, with the
// DV_IDENTIFIER list handed in by the caller because the two positions spell it
// differently (nested for a standalone party, inlined for a PARTICIPATION).
//
// populated is false when the position carries no party key at all. What that
// means is the caller's to decide — a PARTICIPATION performer becomes PARTY_SELF,
// an ENTRY `subject` stays absent — so this reports the fact instead of picking.
func partySuffixes(g rmattrGroup, ts rmattrTails, identifiers []any) (party map[string]any, populated bool, err error) {
	party = map[string]any{"_type": "PARTY_IDENTIFIED"}
	if len(identifiers) > 0 {
		party["identifiers"] = identifiers
		populated = true
	}
	name, haveName, err := ts.ownString(g, "name")
	if err != nil {
		return nil, false, err
	}
	if haveName {
		party["name"] = name
		populated = true
	}
	ref, haveRef, err := partyExternalRef(g, ts)
	if err != nil {
		return nil, false, err
	}
	if haveRef {
		party["external_ref"] = ref
		populated = true
	}
	if _, related := ts.sub["relationship"]; related {
		rel, err := ts.value(g, "relationship", "DV_CODED_TEXT")
		if err != nil {
			return nil, false, err
		}
		party["_type"] = "PARTY_RELATED"
		party["relationship"] = rel
		populated = true
	}
	if !populated {
		return nil, false, nil
	}
	return party, true, nil
}

// partyExternalRef rebuilds PARTY_IDENTIFIED.external_ref from `|id`,
// `|id_scheme`, `|id_namespace` and the optional `|type`.
//
// `|id` is what makes a reference exist: a scheme, a namespace or a type on its
// own qualifies nothing, so any of them alone is refused rather than dropped.
// `namespace` is RM-mandatory on OBJECT_REF and has no default that would not be
// a fabrication, so `|id` without `|id_namespace` is refused too; `type` is
// RM-mandatory as well, but it *has* a default the reference itself writes by
// (see [partyRefType]).
func partyExternalRef(g rmattrGroup, ts rmattrTails) (map[string]any, bool, error) {
	id, haveID, err := ts.ownString(g, "id")
	if err != nil {
		return nil, false, err
	}
	scheme, haveScheme, err := ts.ownString(g, "id_scheme")
	if err != nil {
		return nil, false, err
	}
	namespace, haveNamespace, err := ts.ownString(g, "id_namespace")
	if err != nil {
		return nil, false, err
	}
	refType, haveType, err := ts.ownString(g, "type")
	if err != nil {
		return nil, false, err
	}
	if !haveID {
		for _, spelled := range []struct {
			suffix  string
			present bool
		}{{"id_scheme", haveScheme}, {"id_namespace", haveNamespace}, {"type", haveType}} {
			if spelled.present {
				return nil, false, fmt.Errorf("%w: %s carries |%s without |id, which qualifies no reference",
					ErrUnsupportedDatatype, g.prefix(), spelled.suffix)
			}
		}
		return nil, false, nil
	}
	if !haveNamespace {
		return nil, false, fmt.Errorf("%w: %s is missing |id_namespace (RM-mandatory on OBJECT_REF)",
			ErrUnsupportedDatatype, g.prefix())
	}
	objectID := map[string]any{"_type": "HIER_OBJECT_ID", "value": id}
	if haveScheme {
		objectID = map[string]any{"_type": "GENERIC_ID", "value": id, "scheme": scheme}
	}
	if !haveType {
		refType = partyRefType
	}
	return map[string]any{
		"_type": "PARTY_REF", "id": objectID,
		"namespace": namespace, "type": refType,
	}, true, nil
}

// partyIdentifiers decodes a standalone party's nested `_identifier:N` list,
// each instance through the DV_IDENTIFIER leaf builder so the nested and the
// clinical-leaf spellings of one datatype cannot diverge.
func partyIdentifiers(g rmattrGroup, ts rmattrTails) ([]any, error) {
	slots, err := ts.listSlots(g, partyIdentifierSeg)
	if err != nil {
		return nil, err
	}
	out := make([]any, 0, len(slots))
	for _, i := range slots {
		dv, err := ts.listValue(g, partyIdentifierSeg, i, "DV_IDENTIFIER")
		if err != nil {
			return nil, err
		}
		out = append(out, dv)
	}
	return out, nil
}

// --- PARTICIPATION ------------------------------------------------------

// participationOwnTails is PARTICIPATION's own-suffix grammar: its two suffixes
// plus the performer's party suffixes, which the reference inlines on the same
// key base. The performer's `|identifiers_*:N` are removed from the tails before
// the check ([takeInlineIdentifiers]), because their `:index` makes each one a
// distinct suffix that no fixed allowlist could enumerate. Its sub-path set is
// the party's ([partySubTails]) — a performer's `/relationship` and nothing else.
var participationOwnTails = func() map[string]bool {
	out := map[string]bool{"function": true, "mode": true}
	maps.Copy(out, partyOwnTails)
	return out
}()

// decodeRMAttrParticipation decodes one `context/_participation:N` or
// `<entry>/_other_participation:N` instance. Both families are the same
// PARTICIPATION grammar on two owners, so they share this decoder.
//
// PARTICIPATION.time is deliberately unmodelled: the reference's suffix set has
// no channel for it and the corpus never writes one, so decode has nothing to
// read and encode refuses a populated one loudly rather than dropping it
// (rmattr_party_encode.go; recorded in deviations.md).
func decodeRMAttrParticipation(g rmattrGroup, _ string) (any, error) {
	// lists is nil: a participation's identifiers ride the inlined suffix
	// spelling, so the nested `/_identifier:N` falls through to the allowlist and
	// is refused naming the key the payload wrote.
	ts, err := splitRMAttrTails(g, nil)
	if err != nil {
		return nil, err
	}
	ids, err := takeInlineIdentifiers(g, ts)
	if err != nil {
		return nil, err
	}
	if err := ts.check(g, participationOwnTails, partySubTails); err != nil {
		return nil, err
	}
	function, haveFunction, err := ts.ownString(g, "function")
	if err != nil {
		return nil, err
	}
	if !haveFunction {
		return nil, fmt.Errorf("%w: %s is missing the required |function (RM-mandatory on PARTICIPATION)",
			ErrUnsupportedDatatype, g.prefix())
	}
	p := map[string]any{"_type": "PARTICIPATION", "function": textJSON(function)}
	rubric, haveMode, err := ts.ownString(g, "mode")
	if err != nil {
		return nil, err
	}
	if haveMode {
		mode, err := participationModeJSON(ts.key(g, ownTail("mode")), rubric)
		if err != nil {
			return nil, err
		}
		p["mode"] = mode
	}
	performer, populated, err := partySuffixes(g, ts, ids)
	if err != nil {
		return nil, err
	}
	if !populated {
		// No party key: the performer is the record subject. PARTY_SELF is the one
		// PARTY_PROXY subtype the grammar spells by *absence*, which is why the
		// corpus's `party_self` fixture writes a participation as `|function` +
		// `|mode` alone.
		performer = map[string]any{"_type": "PARTY_SELF"}
	}
	p["performer"] = performer
	return p, nil
}

// inlineIdentifierFields maps a PARTICIPATION's inlined identifier suffix stem
// onto the DV_IDENTIFIER suffix the nested `_identifier:N` spelling uses.
var inlineIdentifierFields = map[string]string{
	"identifiers_id":       "id",
	"identifiers_issuer":   "issuer",
	"identifiers_assigner": "assigner",
	"identifiers_type":     "type",
}

// takeInlineIdentifiers removes a participation's `|identifiers_<field>:N`
// suffixes from ts and decodes them into the DV_IDENTIFIER list they spell.
//
// They are taken out rather than allowlisted because the `:index` is part of the
// suffix, so the set is unbounded; what remains in ts is what a fixed allowlist
// can judge. An `|identifiers_id` with **no** index is left behind on purpose —
// the reference always indexes, so the allowlist refuses it naming the key
// instead of this function guessing slot 0.
func takeInlineIdentifiers(g rmattrGroup, ts rmattrTails) ([]any, error) {
	slots := make(map[int]map[string]any)
	for _, suffix := range slices.Sorted(maps.Keys(ts.own)) {
		stem, index, indexed := strings.Cut(suffix, ":")
		field, known := inlineIdentifierFields[stem]
		if !indexed || !known {
			continue
		}
		key := ts.key(g, ownTail(suffix))
		n, err := strconv.Atoi(index)
		if err != nil || n < 0 || strconv.Itoa(n) != index {
			return nil, fmt.Errorf("%w: invalid :index %q in %q", ErrUnknownPath, index, key)
		}
		if n > maxRepeatIndex {
			return nil, fmt.Errorf("%w: %q :index %d exceeds bound %d", ErrUnknownPath, key, n, maxRepeatIndex)
		}
		if slots[n] == nil {
			slots[n] = make(map[string]any)
		}
		slots[n][field] = ts.own[suffix]
		delete(ts.own, suffix)
	}
	out := make([]any, 0, len(slots))
	for i := range len(slots) {
		sfx := slots[i]
		if sfx == nil {
			return nil, fmt.Errorf("%w: %s's inlined identifiers sit in a sparse :index sequence (no :%d); the occurrences must run 0..%d",
				ErrUnknownPath, g.prefix(), i, len(slots)-1)
		}
		dv, err := dvFromSuffixes("DV_IDENTIFIER", false, sfx)
		if err != nil {
			return nil, fmt.Errorf("simplified: decode %q: %w", g.key("|identifiers_*:"+strconv.Itoa(i)), err)
		}
		out = append(out, dv)
	}
	return out, nil
}

// --- the vendored `participation mode` vocabulary -----------------------

// participationModes is the openEHR Terminology **`participation mode`** group
// (codes 193–224), vendored code → rubric.
//
// It is here because the reference gives `|mode` **no code channel**: the corpus
// writes the bare rubric (`"face-to-face communication"`, `"not specified"`)
// where PARTICIPATION.mode is a DV_CODED_TEXT whose `defining_code` is
// RM-mandatory. Rebuilding that code is the only way to decode the key without
// fabricating an empty CODE_PHRASE, and the group is small, closed and stable —
// it is part of the openEHR Terminology, versioned with the specifications, not
// a runtime terminology lookup. A rubric outside the table is refused loudly
// rather than guessed; encode is the exact inverse and refuses a mode whose code
// or terminology the rubric would not reproduce. Recorded in deviations.md
// § vendored vocabularies.
//
// Source: openEHR Terminology, group `participation mode`
// (https://specifications.openehr.org/releases/TERM/latest — the
// `openehr_terminology.xml` group of that name).
var participationModes = map[string]string{
	"193": "not specified",
	"194": "asynchronous audiovisual; recorded video",
	"195": "live audiovisual; videoconference; videophone",
	"196": "recorded video",
	"197": "videophone",
	"198": "videoconferencing",
	"199": "asynchronous audio-only; dictated; voice mail",
	"200": "dictated",
	"201": "voice-mail",
	"202": "live audio-only; telephone; internet phone; teleconference",
	"203": "teleconference",
	"204": "telephone",
	"205": "internet telephone",
	"206": "asynchronous text; email; fax; letter; handwritten note; SMS message",
	"207": "email",
	"208": "facsimile/telefax",
	"209": "SMS message",
	"210": "printed/typed letter",
	"211": "handwritten note",
	"212": "live text-only; internet chat; SMS chat; interactive written note",
	"213": "internet chat",
	"214": "SMS chat",
	"215": "interactive written note",
	"216": "face-to-face communication",
	"217": "signing (face-to-face)",
	"218": "signing over video",
	"219": "physically present",
	"220": "physically remote",
	"221": "translated text",
	"222": "interpreted audio-only",
	"223": "interpreted face-to-face communication",
	"224": "interpreted video communication",
}

// participationModeTerminology is the terminology the rubric implies. The wire
// carries neither it nor the code, so both are rebuilt — and encode refuses a
// mode coded anywhere else, since emitting its rubric alone would silently
// re-terminologise it (the `|normal_status` precedent).
const participationModeTerminology = "openehr"

// participationModeCodes inverts [participationModes]. Built once; the group's
// rubrics are distinct, and [TestParticipationModeVocabularyIsInvertible] pins
// that, because a collision would make the round-trip pick a code by map order.
var participationModeCodes = func() map[string]string {
	out := make(map[string]string, len(participationModes))
	for code, rubric := range participationModes {
		out[rubric] = code
	}
	return out
}()

// participationModeJSON rebuilds PARTICIPATION.mode from the rubric `|mode`
// carries, naming key on an unknown one.
func participationModeJSON(key, rubric string) (map[string]any, error) {
	code, known := participationModeCodes[rubric]
	if !known {
		return nil, fmt.Errorf("%w: %q is %q, which is not a rubric of the openEHR `participation mode` group; the key carries no code, so there is nothing else to rebuild PARTICIPATION.mode from (REQ-140)",
			ErrUnsupportedDatatype, key, rubric)
	}
	return map[string]any{
		"_type": "DV_CODED_TEXT", "value": rubric,
		"defining_code": codePhraseJSON(code, participationModeTerminology),
	}, nil
}
