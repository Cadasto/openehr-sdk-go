package validation

// rmfloor_coded.go: REQ-112 Coded invariants — the RM invariants that tie a
// coded attribute to a group or a code set of the openEHR terminology,
// checked by the RM floor on every node it visits. The rule table below is
// the catalogue's table, one rule per row; codedValues reads the coded
// attributes each class carries, by value or by pointer, straight from the
// typed RM value (rmread reads neither DV_TEXT language/encoding, nor the
// DV_ENCAPSULATED codes, nor any PARTICIPATION). The group, the code set and
// the membership verdict come from openehr/terminology (REQ-034).

import (
	"fmt"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/terminology"
	"github.com/cadasto/openehr-sdk-go/openehr/validation/rmread"
)

// codedRule is one RM invariant over a coded attribute: the attribute's
// CODE_PHRASE must be a member of a group or of a code set of the openEHR
// terminology. Exactly one of group and codeSet is set.
type codedRule struct {
	owner     string // the class that declares the invariant, e.g. "ENTRY"
	invariant string // the invariant's RM name, e.g. "Language_valid"
	attr      string // the coded attribute, e.g. "language"
	// codedText is set when the attribute is a DV_CODED_TEXT: the rule then
	// reads its defining_code, not the attribute itself.
	codedText bool
	group     *terminology.Group
	codeSet   *terminology.CodeSet
}

// The REQ-112 Coded invariants table, one rule per row. A row naming two
// classes (DV_TEXT and DV_CODED_TEXT, DV_MULTIMEDIA and DV_PARSABLE, every
// ENTRY or DV_ORDERED concrete) is one rule, since one class declares it.
var (
	ruleCompositionCategory  = codedRule{owner: "COMPOSITION", invariant: "Category_validity", attr: "category", codedText: true, group: terminology.CompositionCategory}
	ruleCompositionLanguage  = codedRule{owner: "COMPOSITION", invariant: "Language_valid", attr: "language", codeSet: terminology.Languages}
	ruleCompositionTerritory = codedRule{owner: "COMPOSITION", invariant: "Territory_valid", attr: "territory", codeSet: terminology.Countries}
	ruleEventContextSetting  = codedRule{owner: "EVENT_CONTEXT", invariant: "Setting_valid", attr: "setting", codedText: true, group: terminology.Setting}
	ruleEntryLanguage        = codedRule{owner: "ENTRY", invariant: "Language_valid", attr: "language", codeSet: terminology.Languages}
	ruleEntryEncoding        = codedRule{owner: "ENTRY", invariant: "Encoding_valid", attr: "encoding", codeSet: terminology.CharacterSets}
	ruleElementNullFlavour   = codedRule{owner: "ELEMENT", invariant: "Inv_null_flavour_valid", attr: "null_flavour", codedText: true, group: terminology.NullFlavours}
	ruleEventMathFunction    = codedRule{owner: "INTERVAL_EVENT", invariant: "Math_function_validity", attr: "math_function", codedText: true, group: terminology.EventMathFunction}
	ruleISMCurrentState      = codedRule{owner: "ISM_TRANSITION", invariant: "Current_state_valid", attr: "current_state", codedText: true, group: terminology.InstructionStates}
	ruleISMTransition        = codedRule{owner: "ISM_TRANSITION", invariant: "Transition_valid", attr: "transition", codedText: true, group: terminology.InstructionTransitions}
	ruleParticipationFunc    = codedRule{owner: "PARTICIPATION", invariant: "Function_valid", attr: "function", codedText: true, group: terminology.ParticipationFunction}
	ruleParticipationMode    = codedRule{owner: "PARTICIPATION", invariant: "Mode_valid", attr: "mode", codedText: true, group: terminology.ParticipationMode}
	rulePartyRelationship    = codedRule{owner: "PARTY_RELATED", invariant: "Relationship_valid", attr: "relationship", codedText: true, group: terminology.SubjectRelationship}
	ruleTermMappingPurpose   = codedRule{owner: "TERM_MAPPING", invariant: "Purpose_valid", attr: "purpose", codedText: true, group: terminology.TermMappingPurpose}
	ruleTextLanguage         = codedRule{owner: "DV_TEXT", invariant: "Language_valid", attr: "language", codeSet: terminology.Languages}
	ruleTextEncoding         = codedRule{owner: "DV_TEXT", invariant: "Encoding_valid", attr: "encoding", codeSet: terminology.CharacterSets}
	ruleNormalStatus         = codedRule{owner: "DV_ORDERED", invariant: "Normal_status_validity", attr: "normal_status", codeSet: terminology.NormalStatuses}
	ruleEncapsulatedLanguage = codedRule{owner: "DV_ENCAPSULATED", invariant: "Language_valid", attr: "language", codeSet: terminology.Languages}
	ruleEncapsulatedCharset  = codedRule{owner: "DV_ENCAPSULATED", invariant: "Charset_valid", attr: "charset", codeSet: terminology.CharacterSets}
	ruleMediaType            = codedRule{owner: "DV_MULTIMEDIA", invariant: "Media_type_valid", attr: "media_type", codeSet: terminology.MediaTypes}
	ruleCompression          = codedRule{owner: "DV_MULTIMEDIA", invariant: "Compression_algorithm_validity", attr: "compression_algorithm", codeSet: terminology.CompressionAlgorithms}
	ruleIntegrityCheck       = codedRule{owner: "DV_MULTIMEDIA", invariant: "Integrity_check_algorithm_validity", attr: "integrity_check_algorithm", codeSet: terminology.IntegrityCheckAlgorithms}
)

// holds reports whether code satisfies the rule. A group invariant asks the
// openEHR terminology itself, so the code must be coded in it as well as be
// a member of the group. A code-set invariant takes the bare code, as the
// RM's CODE_SET_ACCESS.has_code does, so the terminology id is not compared;
// the code set's own membership rule decides letter case. An empty
// code_string is a member of neither.
func (r *codedRule) holds(code rm.CodePhrase) bool {
	if r.group != nil {
		return code.TerminologyID.Value == terminology.ID && r.group.Has(code.CodeString)
	}
	return r.codeSet.Has(code.CodeString)
}

// detail is the diagnostic for a breach on a node of class: it names the
// class, the attribute, the invariant and the group or code set (with the
// external id of an ISO or IANA set), and never the code the node carries or
// its terminology id (REQ-093).
func (r *codedRule) detail(class string) string {
	invariant := r.owner + "." + r.invariant
	switch {
	case r.group != nil:
		return fmt.Sprintf("%s.%s must be coded in the openEHR terminology with a code of its group %q (RM %s)",
			class, r.attr, r.group.Name(), invariant)
	case r.codeSet.Issuer() == openEHRIssuer:
		return fmt.Sprintf("%s.%s must carry a code of the openEHR code set %q (RM %s)",
			class, r.attr, r.codeSet.Name(), invariant)
	default:
		return fmt.Sprintf("%s.%s must carry a code of the %s code set %q, external id %s (RM %s)",
			class, r.attr, r.codeSet.Issuer(), r.codeSet.Name(), r.codeSet.ExternalID(), invariant)
	}
}

// openEHRIssuer is the issuer the pinned terminology names for the code sets
// openEHR defines itself; see [terminology.CodeSet.Issuer].
const openEHRIssuer = "openehr"

// codedValue is one coded attribute a node carries, checked under rule.
// at is the path from the node to the object holding the attribute ("" for
// the node itself, "/other_participations[0]" for one of an ENTRY's
// participations), and class names that object when it is not the node.
type codedValue struct {
	rule  *codedRule
	code  rm.CodePhrase
	at    string
	class string
}

// path returns where the CODE_PHRASE the rule reads sits, below the node at
// nodePath.
func (v codedValue) path(nodePath string) string {
	p := nodePath
	if v.at != "" {
		p = joinPath(p, v.at)
	}
	p = joinPath(p, "/"+v.rule.attr)
	if v.rule.codedText {
		p += "/defining_code"
	}
	return p
}

// checkCodedInvariants runs the REQ-112 coded invariants on the node value,
// whose runtime class is rmType, at path. It reports each breach as
// `code_not_in_value_set` at the CODE_PHRASE the invariant reads.
func (w *rmFloorWalker) checkCodedInvariants(value any, rmType, path string) {
	for _, v := range codedValues(value) {
		if v.rule.holds(v.code) {
			continue
		}
		class := rmType
		if v.class != "" {
			class = v.class
		}
		w.emit(Issue{
			Path:   v.path(path),
			Code:   "code_not_in_value_set",
			Detail: v.rule.detail(class),
		})
	}
}

// codedValues lists the coded attributes of the table that value carries, by
// value or by pointer. A mandatory attribute is listed whatever it holds, so
// one that decoded empty breaks its rule; an optional one only when present.
// A class with no coded invariant in the table yields nothing: that includes
// the change-control classes (AUDIT_DETAILS, ATTESTATION) the table leaves
// out. Every arm is pinned by tests in both forms, since the walk hands over
// either: a DV_INTERVAL holds its bounds by value, for one. The ENTRY and
// DV_ORDERED arms are pinned against the registry, by pointer and by value,
// so a concrete added there that is missing here, in either form, fails
// loudly.
func codedValues(value any) []codedValue {
	if value == nil || rmread.IsTypedNilPointer(value) {
		return nil
	}
	switch v := value.(type) {
	case *rm.Composition:
		return compositionCoded(v)
	case rm.Composition:
		return compositionCoded(&v)
	case *rm.EventContext:
		return eventContextCoded(v)
	case rm.EventContext:
		return eventContextCoded(&v)

	case *rm.Observation:
		return entryCoded(v.Language, v.Encoding, v.OtherParticipations)
	case rm.Observation:
		return entryCoded(v.Language, v.Encoding, v.OtherParticipations)
	case *rm.Evaluation:
		return entryCoded(v.Language, v.Encoding, v.OtherParticipations)
	case rm.Evaluation:
		return entryCoded(v.Language, v.Encoding, v.OtherParticipations)
	case *rm.Instruction:
		return entryCoded(v.Language, v.Encoding, v.OtherParticipations)
	case rm.Instruction:
		return entryCoded(v.Language, v.Encoding, v.OtherParticipations)
	case *rm.Action:
		return entryCoded(v.Language, v.Encoding, v.OtherParticipations)
	case rm.Action:
		return entryCoded(v.Language, v.Encoding, v.OtherParticipations)
	case *rm.AdminEntry:
		return entryCoded(v.Language, v.Encoding, v.OtherParticipations)
	case rm.AdminEntry:
		return entryCoded(v.Language, v.Encoding, v.OtherParticipations)

	case *rm.Element:
		return elementCoded(v)
	case rm.Element:
		return elementCoded(&v)

	case *rm.IntervalEvent[rm.ItemStructure]:
		return mathFunctionCoded(v.MathFunction)
	case rm.IntervalEvent[rm.ItemStructure]:
		return mathFunctionCoded(v.MathFunction)
	case *rm.IntervalEvent[rm.ItemList]:
		return mathFunctionCoded(v.MathFunction)
	case rm.IntervalEvent[rm.ItemList]:
		return mathFunctionCoded(v.MathFunction)
	case *rm.IntervalEvent[rm.ItemSingle]:
		return mathFunctionCoded(v.MathFunction)
	case rm.IntervalEvent[rm.ItemSingle]:
		return mathFunctionCoded(v.MathFunction)
	case *rm.IntervalEvent[rm.ItemTable]:
		return mathFunctionCoded(v.MathFunction)
	case rm.IntervalEvent[rm.ItemTable]:
		return mathFunctionCoded(v.MathFunction)
	case *rm.IntervalEvent[rm.ItemTree]:
		return mathFunctionCoded(v.MathFunction)
	case rm.IntervalEvent[rm.ItemTree]:
		return mathFunctionCoded(v.MathFunction)

	case *rm.IsmTransition:
		return ismTransitionCoded(v)
	case rm.IsmTransition:
		return ismTransitionCoded(&v)
	case *rm.Participation:
		return participationCoded(nil, "", *v)
	case rm.Participation:
		return participationCoded(nil, "", v)
	case *rm.PartyRelated:
		return []codedValue{relationshipCoded("", v)}
	case rm.PartyRelated:
		return []codedValue{relationshipCoded("", &v)}
	case *rm.TermMapping:
		return appendCodedText(nil, &ruleTermMappingPurpose, v.Purpose)
	case rm.TermMapping:
		return appendCodedText(nil, &ruleTermMappingPurpose, v.Purpose)

	case *rm.DVText:
		return textCoded(v.Language, v.Encoding)
	case rm.DVText:
		return textCoded(v.Language, v.Encoding)
	case *rm.DVCodedText:
		return textCoded(v.Language, v.Encoding)
	case rm.DVCodedText:
		return textCoded(v.Language, v.Encoding)

	case *rm.DVQuantity:
		return appendCode(nil, &ruleNormalStatus, v.NormalStatus)
	case rm.DVQuantity:
		return appendCode(nil, &ruleNormalStatus, v.NormalStatus)
	case *rm.DVCount:
		return appendCode(nil, &ruleNormalStatus, v.NormalStatus)
	case rm.DVCount:
		return appendCode(nil, &ruleNormalStatus, v.NormalStatus)
	case *rm.DVProportion:
		return appendCode(nil, &ruleNormalStatus, v.NormalStatus)
	case rm.DVProportion:
		return appendCode(nil, &ruleNormalStatus, v.NormalStatus)
	case *rm.DVOrdinal:
		return appendCode(nil, &ruleNormalStatus, v.NormalStatus)
	case rm.DVOrdinal:
		return appendCode(nil, &ruleNormalStatus, v.NormalStatus)
	case *rm.DVScale:
		return appendCode(nil, &ruleNormalStatus, v.NormalStatus)
	case rm.DVScale:
		return appendCode(nil, &ruleNormalStatus, v.NormalStatus)
	case *rm.DVDateTime:
		return appendCode(nil, &ruleNormalStatus, v.NormalStatus)
	case rm.DVDateTime:
		return appendCode(nil, &ruleNormalStatus, v.NormalStatus)
	case *rm.DVDate:
		return appendCode(nil, &ruleNormalStatus, v.NormalStatus)
	case rm.DVDate:
		return appendCode(nil, &ruleNormalStatus, v.NormalStatus)
	case *rm.DVTime:
		return appendCode(nil, &ruleNormalStatus, v.NormalStatus)
	case rm.DVTime:
		return appendCode(nil, &ruleNormalStatus, v.NormalStatus)
	case *rm.DVDuration:
		return appendCode(nil, &ruleNormalStatus, v.NormalStatus)
	case rm.DVDuration:
		return appendCode(nil, &ruleNormalStatus, v.NormalStatus)

	case *rm.DVMultimedia:
		return multimediaCoded(v)
	case rm.DVMultimedia:
		return multimediaCoded(&v)
	case *rm.DVParsable:
		return encapsulatedCoded(v.Language, v.Charset)
	case rm.DVParsable:
		return encapsulatedCoded(v.Language, v.Charset)
	}
	return nil
}

// compositionCoded lists COMPOSITION's category, language and territory, all
// mandatory.
func compositionCoded(c *rm.Composition) []codedValue {
	return []codedValue{
		{rule: &ruleCompositionCategory, code: c.Category.DefiningCode},
		{rule: &ruleCompositionLanguage, code: c.Language},
		{rule: &ruleCompositionTerritory, code: c.Territory},
	}
}

// eventContextCoded lists EVENT_CONTEXT's mandatory setting and the coded
// attributes of each of its participations, which the walk does not reach.
func eventContextCoded(ec *rm.EventContext) []codedValue {
	out := []codedValue{{rule: &ruleEventContextSetting, code: ec.Setting.DefiningCode}}
	return appendParticipations(out, "participations", ec.Participations)
}

// entryCoded lists an ENTRY's mandatory language and encoding and the coded
// attributes of each of its other_participations, which the walk does not
// reach.
func entryCoded(language, encoding rm.CodePhrase, participations []rm.Participation) []codedValue {
	out := []codedValue{
		{rule: &ruleEntryLanguage, code: language},
		{rule: &ruleEntryEncoding, code: encoding},
	}
	return appendParticipations(out, "other_participations", participations)
}

// elementCoded lists ELEMENT's null_flavour when the ELEMENT has no value:
// the RM states Inv_null_flavour_valid as `is_null implies …`. A typed-nil
// value counts as absent, as it does in [rmFloorWalker.checkElementNullFlavour].
func elementCoded(e *rm.Element) []codedValue {
	if e.Value != nil && !rmread.IsTypedNilPointer(e.Value) {
		return nil
	}
	return appendCodedText(nil, &ruleElementNullFlavour, e.NullFlavour)
}

// mathFunctionCoded lists INTERVAL_EVENT's mandatory math_function.
func mathFunctionCoded(mathFunction rm.DVCodedText) []codedValue {
	return []codedValue{{rule: &ruleEventMathFunction, code: mathFunction.DefiningCode}}
}

// ismTransitionCoded lists ISM_TRANSITION's mandatory current_state and its
// transition when present.
func ismTransitionCoded(t *rm.IsmTransition) []codedValue {
	out := []codedValue{{rule: &ruleISMCurrentState, code: t.CurrentState.DefiningCode}}
	return appendCodedText(out, &ruleISMTransition, t.Transition)
}

// relationshipCoded is PARTY_RELATED's relationship, at at below the node.
// The attribute is mandatory and Relationship_valid unconditional, so an
// empty relationship breaks it too. That finding is the only one the floor
// gives a PARTY_RELATED with no relationship: the walk does not read the
// leaf's attributes, so it reports no `required` there (REQ-112, Known gap —
// classes rmread does not model).
func relationshipCoded(at string, p *rm.PartyRelated) codedValue {
	return codedValue{rule: &rulePartyRelationship, code: p.Relationship.DefiningCode, at: at, class: "PARTY_RELATED"}
}

// textCoded lists a DV_TEXT's (or DV_CODED_TEXT's) language and encoding,
// each when present.
func textCoded(language, encoding *rm.CodePhrase) []codedValue {
	out := appendCode(nil, &ruleTextLanguage, language)
	return appendCode(out, &ruleTextEncoding, encoding)
}

// encapsulatedCoded lists a DV_ENCAPSULATED's language and charset, each when
// present.
func encapsulatedCoded(language, charset *rm.CodePhrase) []codedValue {
	out := appendCode(nil, &ruleEncapsulatedLanguage, language)
	return appendCode(out, &ruleEncapsulatedCharset, charset)
}

// multimediaCoded lists DV_MULTIMEDIA's DV_ENCAPSULATED attributes, its
// mandatory media_type, and its compression and integrity check algorithms
// when present.
func multimediaCoded(m *rm.DVMultimedia) []codedValue {
	out := encapsulatedCoded(m.Language, m.Charset)
	out = append(out, codedValue{rule: &ruleMediaType, code: m.MediaType})
	out = appendCode(out, &ruleCompression, m.CompressionAlgorithm)
	return appendCode(out, &ruleIntegrityCheck, m.IntegrityCheckAlgorithm)
}

// appendParticipations appends the coded attributes of each participation,
// held by the node under its container attribute attr.
func appendParticipations(out []codedValue, attr string, participations []rm.Participation) []codedValue {
	for i, p := range participations {
		out = participationCoded(out, fmt.Sprintf("/%s[%d]", attr, i), p)
	}
	return out
}

// participationCoded appends PARTICIPATION's function when it is a
// DV_CODED_TEXT (Function_valid's own guard), its mode when present, and the
// relationship of its performer when that is a PARTY_RELATED: the walk
// reaches no PARTICIPATION, so nothing else reads the performer. at is the
// path from the node to the participation ("" when the participation is the
// node).
func participationCoded(out []codedValue, at string, p rm.Participation) []codedValue {
	if fn, ok := asDVCodedText(p.Function); ok {
		out = append(out, codedValue{rule: &ruleParticipationFunc, code: fn.DefiningCode, at: at, class: "PARTICIPATION"})
	}
	if p.Mode != nil {
		out = append(out, codedValue{rule: &ruleParticipationMode, code: p.Mode.DefiningCode, at: at, class: "PARTICIPATION"})
	}
	if pr, ok := asPartyRelated(p.Performer); ok {
		out = append(out, relationshipCoded(at+"/performer", &pr))
	}
	return out
}

// appendCode appends the rule's check of an optional CODE_PHRASE attribute
// when it is present.
func appendCode(out []codedValue, rule *codedRule, code *rm.CodePhrase) []codedValue {
	if code == nil {
		return out
	}
	return append(out, codedValue{rule: rule, code: *code})
}

// appendCodedText appends the rule's check of an optional DV_CODED_TEXT
// attribute's defining_code when the attribute is present.
func appendCodedText(out []codedValue, rule *codedRule, text *rm.DVCodedText) []codedValue {
	if text == nil {
		return out
	}
	return append(out, codedValue{rule: rule, code: text.DefiningCode})
}
