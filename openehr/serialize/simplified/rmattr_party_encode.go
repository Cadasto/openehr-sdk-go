package simplified

// REQ-140 — the party grammar, encode side: the exact inverse of
// rmattr_party.go. One implementation per position, as decode has.
//
// Nothing populated is dropped. Where the grammar has no channel for a value the
// result is a typed error, because no party key has a `|raw` carrier: a
// PARTY_REF typed as anything but [partyRefType], an OBJECT_ID subtype the
// `|id_scheme` discriminator cannot distinguish, a coded PARTICIPATION.function,
// a PARTICIPATION.time, or a `mode` whose code the rubric would not reproduce.

import (
	"fmt"
	"strconv"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// partyRMAttr writes a standalone party under path: its own suffixes plus the
// nested `_identifier:N` list. Used for `context/_health_care_facility`, the
// ENTRY `subject` leaf, and — from Phase C3 — FEEDER_AUDIT_DETAILS' party
// attributes.
func partyRMAttr(out map[string]any, path string, p any) error {
	ids, err := partySuffixesToFlat(out, path, p)
	if err != nil {
		return err
	}
	for i, id := range ids {
		if _, err := emitLeafValue(out, path+"/"+partyIdentifierSeg+":"+strconv.Itoa(i), id, "DV_IDENTIFIER", false, false); err != nil {
			return err
		}
	}
	return nil
}

// partySuffixesToFlat writes a party's own suffixes — `|name`, the `external_ref`
// decomposition, and PARTY_RELATED's `/relationship` — and returns its
// DV_IDENTIFIER list for the caller to spell in its own position's form.
//
// A PARTY_SELF writes nothing: absence of every party key *is* how the grammar
// spells it (see rmattr_party.go), which is what keeps a PARTY_SELF performer and
// the `WithTemplate` PARTY_SELF `subject` default byte-identical on re-encode. A
// PARTY_SELF carrying an `external_ref` is refused instead — the grammar has
// nowhere to put it, and PARTY_IDENTIFIED's `|id` suffixes are not available here
// because decode would read them back as the wrong subtype.
func partySuffixesToFlat(out map[string]any, path string, p any) ([]rm.DVIdentifier, error) {
	if p == nil || rm.IsTypedNil(p) {
		return nil, nil
	}
	if self, ok := as[rm.PartySelf](p); ok {
		if self.ExternalRef != nil {
			return nil, fmt.Errorf("%w: %q is a PARTY_SELF carrying an external_ref, which the party suffix set cannot spell — PARTY_SELF is spelled by the absence of every party key",
				ErrUnsupportedDatatype, path)
		}
		return nil, nil
	}
	if related, ok := as[rm.PartyRelated](p); ok {
		if _, err := emitLeafValue(out, path+"/relationship", related.Relationship, "DV_CODED_TEXT", false, false); err != nil {
			return nil, err
		}
		return partyIdentifiedToFlat(out, path, related.PartyIdentified)
	}
	if identified, ok := as[rm.PartyIdentified](p); ok {
		return partyIdentifiedToFlat(out, path, identified)
	}
	return nil, fmt.Errorf("%w: %q cannot carry a %T; the party grammar spells PARTY_IDENTIFIED, PARTY_RELATED and PARTY_SELF",
		ErrUnsupportedDatatype, path, p)
}

// partyIdentifiedToFlat writes the PARTY_IDENTIFIED half of a party (which
// PARTY_RELATED embeds) and returns its identifier list.
func partyIdentifiedToFlat(out map[string]any, path string, p rm.PartyIdentified) ([]rm.DVIdentifier, error) {
	if p.Name != nil {
		out[path+"|name"] = *p.Name
	}
	if p.ExternalRef != nil {
		if err := partyRefToFlat(out, path, *p.ExternalRef); err != nil {
			return nil, err
		}
	}
	return p.Identifiers, nil
}

// partyRefToFlat decomposes a PARTY_REF into `|id`, the optional `|id_scheme`,
// `|id_namespace` and — only when it is not the reference's own
// [partyRefType] — `|type`.
//
// The `|type` asymmetry is what keeps a corpus body byte-exact while still
// carrying a `PERSON` or `ORGANISATION` reference losslessly: absent means
// PARTY on both sides, so the reference's own spelling is reproduced exactly,
// and the key appears only for the data the reference would drop. An empty type
// is refused rather than defaulted — OBJECT_REF.type is RM-mandatory, and
// writing nothing would let decode read PARTY back over it.
//
// `|id_scheme` is the only OBJECT_ID discriminator, so every scheme-less subtype
// other than HIER_OBJECT_ID would come back as that one: refused, the same policy
// C0 fixed for the OBJECT_REF families.
func partyRefToFlat(out map[string]any, path string, ref rm.PartyRef) error {
	switch ref.Type {
	case "":
		return fmt.Errorf("%w: %q is RM-mandatory on OBJECT_REF but empty; decode would read it back as %q",
			ErrUnsupportedDatatype, path+"|type", partyRefType)
	case partyRefType:
	default:
		out[path+"|type"] = ref.Type
	}
	switch id := ref.ID.(type) {
	case nil:
		return fmt.Errorf("%w: %q is RM-mandatory on OBJECT_REF but absent", ErrUnsupportedDatatype, path+"|id")
	case rm.GenericID:
		out[path+"|id"], out[path+"|id_scheme"] = id.Value, id.Scheme
	case *rm.GenericID:
		out[path+"|id"], out[path+"|id_scheme"] = id.Value, id.Scheme
	case rm.HierObjectID:
		out[path+"|id"] = id.Value
	case *rm.HierObjectID:
		out[path+"|id"] = id.Value
	default:
		return fmt.Errorf("%w: %q cannot carry a %T; only GENERIC_ID (with `|id_scheme`) and HIER_OBJECT_ID (without) are distinguishable in this suffix set",
			ErrUnsupportedDatatype, path+"|id", ref.ID)
	}
	out[path+"|id_namespace"] = ref.Namespace
	return nil
}

// participationsRMAttr writes an indexed PARTICIPATION list — EVENT_CONTEXT's
// `_participation:N` or an ENTRY's `_other_participation:N`, which are the same
// grammar on two owners.
func participationsRMAttr(out map[string]any, base, family string, ps []rm.Participation) error {
	for i, p := range ps {
		if err := participationRMAttr(out, base+"/"+family+":"+strconv.Itoa(i), p); err != nil {
			return err
		}
	}
	return nil
}

// participationRMAttr writes one PARTICIPATION: `|function`, the optional
// `|mode`, the performer's party suffixes inline, and the performer's identifiers
// as the reference's inlined `|identifiers_<field>:N`.
func participationRMAttr(out map[string]any, path string, p rm.Participation) error {
	if p.Time != nil {
		return fmt.Errorf("%w: %q carries a PARTICIPATION.time, which the reference's suffix set has no channel for and the pinned corpus never writes — deferred rather than dropped (REQ-140, see deviations.md)",
			ErrUnsupportedDatatype, path+"|time")
	}
	function, err := plainDVText(path+"|function", p.Function, "PARTICIPATION")
	if err != nil {
		return err
	}
	out[path+"|function"] = function
	if p.Mode != nil {
		rubric, err := participationModeRubric(path+"|mode", *p.Mode)
		if err != nil {
			return err
		}
		out[path+"|mode"] = rubric
	}
	ids, err := partySuffixesToFlat(out, path, p.Performer)
	if err != nil {
		return err
	}
	for i, id := range ids {
		slot := ":" + strconv.Itoa(i)
		out[path+"|identifiers_id"+slot] = id.ID
		for _, opt := range []struct {
			field string
			value *string
		}{{"issuer", id.Issuer}, {"assigner", id.Assigner}, {"type", id.Type}} {
			if opt.value != nil {
				out[path+"|identifiers_"+opt.field+slot] = *opt.value
			}
		}
	}
	return nil
}

// participationModeRubric reduces PARTICIPATION.mode to the bare rubric `|mode`
// carries.
//
// The key has no code and no terminology channel, so decode rebuilds both from
// the vendored group ([participationModes]). Anything that rebuild would not
// reproduce is a typed error rather than a narrowing: a mode coded outside
// `openehr` (silent re-terminologisation), a code the group does not carry, a
// rubric that disagrees with the group's, or a DV_CODED_TEXT decoration the bare
// key cannot hold.
func participationModeRubric(key string, mode rm.DVCodedText) (string, error) {
	if mode.Formatting != nil || mode.Hyperlink != nil || mode.Language != nil ||
		mode.Encoding != nil || len(mode.Mappings) > 0 || mode.DefiningCode.PreferredTerm != nil {
		return "", fmt.Errorf("%w: %q is a bare rubric and cannot carry a decorated DV_CODED_TEXT (formatting, hyperlink, language, encoding, mappings or a preferred term)",
			ErrUnsupportedDatatype, key)
	}
	if term := mode.DefiningCode.TerminologyID.Value; term != participationModeTerminology {
		return "", fmt.Errorf("%w: %q implies the %q terminology, but PARTICIPATION.mode is coded in %q",
			ErrUnsupportedDatatype, key, participationModeTerminology, term)
	}
	rubric, known := participationModes[mode.DefiningCode.CodeString]
	if !known {
		return "", fmt.Errorf("%w: %q carries code %q, which is not in the openEHR `participation mode` group; the key carries only the rubric, so decode could not rebuild the code (REQ-140)",
			ErrUnsupportedDatatype, key, mode.DefiningCode.CodeString)
	}
	if mode.Value != rubric {
		return "", fmt.Errorf("%w: %q carries value %q beside code %q, whose `participation mode` rubric is %q; the key carries the rubric alone, so decode would rewrite the value",
			ErrUnsupportedDatatype, key, mode.Value, mode.DefiningCode.CodeString, rubric)
	}
	return rubric, nil
}
