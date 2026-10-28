package instance

import (
	tcimpl "github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/template/constraints"
	"github.com/cadasto/openehr-sdk-go/openehr/terminology"
)

// codedRule is an RM invariant that keeps a coded attribute within a group
// or a code set of the openEHR terminology: one row of the coded invariants
// the template-less RM validator checks, which a test holds this table to.
type codedRule struct {
	class     string // the class that declares the invariant, such as ENTRY
	invariant string // the invariant's RM name, such as Language_valid
	attr      string // the coded attribute, such as language
	// group is the group of a DV_CODED_TEXT attribute, whose defining code
	// must be an openehr code of the group; codeSet is the code set of a
	// CODE_PHRASE attribute, whose code string must be one of the set.
	// Exactly one is set.
	group   *terminology.Group
	codeSet *terminology.CodeSet
	// def returns the RM default the generator writes for the attribute of
	// v; ok is false where it has none.
	def func(g *generator, v any) (code rm.CodePhrase, ok bool)
	// needed reports whether an RM rule needs the attribute where the OPT
	// prohibits it, so the attribute's own OPT child constrains it; nil
	// where none does.
	needed func(opt *tcimpl.CompiledNode) bool
}

// The coded invariants, one rule per row. A row naming two classes (DV_TEXT
// and DV_CODED_TEXT, DV_MULTIMEDIA and DV_PARSABLE, every ENTRY or
// DV_ORDERED concrete) is one rule, since one class declares it.
var (
	ruleCompositionCategory  = codedRule{class: "COMPOSITION", invariant: "Category_validity", attr: "category", group: terminology.CompositionCategory, def: openehrDefault("433")}
	ruleCompositionLanguage  = codedRule{class: "COMPOSITION", invariant: "Language_valid", attr: "language", codeSet: terminology.Languages, def: languageDefault}
	ruleCompositionTerritory = codedRule{class: "COMPOSITION", invariant: "Territory_valid", attr: "territory", codeSet: terminology.Countries, def: territoryDefault}
	ruleEventContextSetting  = codedRule{class: "EVENT_CONTEXT", invariant: "Setting_valid", attr: "setting", group: terminology.Setting, def: openehrDefault("238")}
	ruleEntryLanguage        = codedRule{class: "ENTRY", invariant: "Language_valid", attr: "language", codeSet: terminology.Languages, def: languageDefault}
	ruleEntryEncoding        = codedRule{class: "ENTRY", invariant: "Encoding_valid", attr: "encoding", codeSet: terminology.CharacterSets, def: characterSetDefault}
	ruleElementNullFlavour   = codedRule{class: "ELEMENT", invariant: "Inv_null_flavour_valid", attr: "null_flavour", group: terminology.NullFlavours, def: openehrDefault("271"), needed: nullFlavourNeeded}
	ruleEventMathFunction    = codedRule{class: "INTERVAL_EVENT", invariant: "Math_function_validity", attr: "math_function", group: terminology.EventMathFunction, def: openehrDefault("146")}
	ruleISMCurrentState      = codedRule{class: "ISM_TRANSITION", invariant: "Current_state_valid", attr: "current_state", group: terminology.InstructionStates, def: openehrDefault("524")}
	ruleISMTransition        = codedRule{class: "ISM_TRANSITION", invariant: "Transition_valid", attr: "transition", group: terminology.InstructionTransitions, def: transitionDefault}
	ruleParticipationFunc    = codedRule{class: "PARTICIPATION", invariant: "Function_valid", attr: "function", group: terminology.ParticipationFunction, def: openehrDefault("253")}
	ruleParticipationMode    = codedRule{class: "PARTICIPATION", invariant: "Mode_valid", attr: "mode", group: terminology.ParticipationMode, def: openehrDefault("193")}
	rulePartyRelationship    = codedRule{class: "PARTY_RELATED", invariant: "Relationship_valid", attr: "relationship", group: terminology.SubjectRelationship, def: openehrDefault("253")}
	ruleTermMappingPurpose   = codedRule{class: "TERM_MAPPING", invariant: "Purpose_valid", attr: "purpose", group: terminology.TermMappingPurpose, def: openehrDefault("669")}
	ruleTextLanguage         = codedRule{class: "DV_TEXT", invariant: "Language_valid", attr: "language", codeSet: terminology.Languages, def: languageDefault}
	ruleTextEncoding         = codedRule{class: "DV_TEXT", invariant: "Encoding_valid", attr: "encoding", codeSet: terminology.CharacterSets, def: characterSetDefault}
	ruleNormalStatus         = codedRule{class: "DV_ORDERED", invariant: "Normal_status_validity", attr: "normal_status", codeSet: terminology.NormalStatuses}
	ruleEncapsulatedLanguage = codedRule{class: "DV_ENCAPSULATED", invariant: "Language_valid", attr: "language", codeSet: terminology.Languages, def: languageDefault}
	ruleEncapsulatedCharset  = codedRule{class: "DV_ENCAPSULATED", invariant: "Charset_valid", attr: "charset", codeSet: terminology.CharacterSets, def: characterSetDefault}
	ruleMediaType            = codedRule{class: "DV_MULTIMEDIA", invariant: "Media_type_valid", attr: "media_type", codeSet: terminology.MediaTypes, def: mediaTypeDefault}
	ruleCompression          = codedRule{class: "DV_MULTIMEDIA", invariant: "Compression_algorithm_validity", attr: "compression_algorithm", codeSet: terminology.CompressionAlgorithms}
	ruleIntegrityCheck       = codedRule{class: "DV_MULTIMEDIA", invariant: "Integrity_check_algorithm_validity", attr: "integrity_check_algorithm", codeSet: terminology.IntegrityCheckAlgorithms}
)

// codedRules lists every rule of the table.
var codedRules = []*codedRule{
	&ruleCompositionCategory, &ruleCompositionLanguage, &ruleCompositionTerritory,
	&ruleEventContextSetting, &ruleEntryLanguage, &ruleEntryEncoding,
	&ruleElementNullFlavour, &ruleEventMathFunction, &ruleISMCurrentState,
	&ruleISMTransition, &ruleParticipationFunc, &ruleParticipationMode,
	&rulePartyRelationship, &ruleTermMappingPurpose, &ruleTextLanguage,
	&ruleTextEncoding, &ruleNormalStatus, &ruleEncapsulatedLanguage,
	&ruleEncapsulatedCharset, &ruleMediaType, &ruleCompression,
	&ruleIntegrityCheck,
}

// accepts reports whether code satisfies r. A group's rule asks the openEHR
// terminology itself, so code must be an openehr code as well as a member
// of the group. A code set's rule takes the bare code string, as the RM's
// CODE_SET_ACCESS.has_code does, under the code set's own rule on letter
// case. An empty code string is a member of neither.
func (r *codedRule) accepts(code rm.CodePhrase) bool {
	if r.group != nil {
		return code.TerminologyID.Value == terminology.ID && r.group.Has(code.CodeString)
	}
	return r.codeSet.Has(code.CodeString)
}

// terminologyID is the terminology id a code of r's set is written in: the
// openEHR terminology for a group, and the code set's external id, such as
// ISO_639-1, for a code set.
func (r *codedRule) terminologyID() string {
	if r.group != nil {
		return terminology.ID
	}
	return r.codeSet.ExternalID()
}

// openehrDefault is the default code of an openehr group's rule.
func openehrDefault(code string) func(*generator, any) (rm.CodePhrase, bool) {
	return func(*generator, any) (rm.CodePhrase, bool) {
		return rm.CodePhrase{CodeString: code, TerminologyID: rm.TerminologyID{Value: terminology.ID}}, true
	}
}

// languageDefault is Options.Language in ISO_639-1.
func languageDefault(g *generator, _ any) (rm.CodePhrase, bool) {
	return g.languageCode(), true
}

// territoryDefault is Options.Territory in ISO_3166-1.
func territoryDefault(g *generator, _ any) (rm.CodePhrase, bool) {
	return rm.CodePhrase{CodeString: g.opts.Territory, TerminologyID: rm.TerminologyID{Value: "ISO_3166-1"}}, true
}

// characterSetDefault is UTF-8 in IANA_character-sets.
func characterSetDefault(*generator, any) (rm.CodePhrase, bool) {
	return utf8Code(), true
}

// mediaTypeDefault is text/plain in IANA_media-types.
func mediaTypeDefault(*generator, any) (rm.CodePhrase, bool) {
	return rm.CodePhrase{CodeString: "text/plain", TerminologyID: rm.TerminologyID{Value: "IANA_media-types"}}, true
}

// transitionDefault is the transition of the openEHR instruction state
// machine that leads into the current state of v, an ISM_TRANSITION:
// transitionInto's for a current state it names, defaultTransition for any
// other.
func transitionDefault(_ *generator, v any) (rm.CodePhrase, bool) {
	iv, ok := v.(*rm.IsmTransition)
	if !ok {
		return rm.CodePhrase{}, false
	}
	code := defaultTransition
	if state := iv.CurrentState.DefiningCode; state.TerminologyID.Value == terminology.ID {
		if into, ok := transitionInto[state.CodeString]; ok {
			code = into
		}
	}
	return rm.CodePhrase{CodeString: code, TerminologyID: rm.TerminologyID{Value: terminology.ID}}, true
}

// languageCode is Options.Language in ISO_639-1.
func (g *generator) languageCode() rm.CodePhrase {
	return rm.CodePhrase{CodeString: g.opts.Language, TerminologyID: rm.TerminologyID{Value: "ISO_639-1"}}
}

// utf8Code is UTF-8 in IANA_character-sets.
func utf8Code() rm.CodePhrase {
	return rm.CodePhrase{CodeString: "UTF-8", TerminologyID: rm.TerminologyID{Value: "IANA_character-sets"}}
}

// codedSlot is a coded attribute a value holds, under the rule that keeps
// it within its set.
type codedSlot struct {
	rule *codedRule
	// code is the code phrase the rule reads: the attribute itself, or the
	// defining code of text.
	code *rm.CodePhrase
	// text is the coded text that holds code, for a group's rule; nil for
	// a code set's.
	text *rm.DVCodedText
}

// codedSlots lists the coded attributes of the table that v holds, in the
// order they are settled: an ISM_TRANSITION's current state before its
// transition, whose default is derived from it. A mandatory attribute is
// listed whatever it holds, an optional one only when present, an
// ELEMENT's null flavour only when it has no value, and a PARTICIPATION's
// function only when it is a DV_CODED_TEXT, as the rules themselves read
// them. Any other value holds none.
func codedSlots(v any) []codedSlot {
	switch x := v.(type) {
	case *rm.Composition:
		return []codedSlot{
			textSlot(&ruleCompositionCategory, &x.Category),
			phraseSlot(&ruleCompositionLanguage, &x.Language),
			phraseSlot(&ruleCompositionTerritory, &x.Territory),
		}
	case *rm.EventContext:
		return []codedSlot{textSlot(&ruleEventContextSetting, &x.Setting)}
	case *rm.Observation:
		return entrySlots(&x.Language, &x.Encoding)
	case *rm.Evaluation:
		return entrySlots(&x.Language, &x.Encoding)
	case *rm.Instruction:
		return entrySlots(&x.Language, &x.Encoding)
	case *rm.Action:
		return entrySlots(&x.Language, &x.Encoding)
	case *rm.AdminEntry:
		return entrySlots(&x.Language, &x.Encoding)
	case *rm.Element:
		if x.Value != nil && !rm.IsTypedNil(x.Value) {
			return nil
		}
		return appendText(nil, &ruleElementNullFlavour, x.NullFlavour)
	case *rm.IntervalEvent[rm.ItemStructure]:
		// typereg builds every INTERVAL_EVENT with this instantiation.
		return []codedSlot{textSlot(&ruleEventMathFunction, &x.MathFunction)}
	case *rm.IsmTransition:
		return appendText([]codedSlot{textSlot(&ruleISMCurrentState, &x.CurrentState)}, &ruleISMTransition, x.Transition)
	case *rm.Participation:
		var out []codedSlot
		if f, ok := x.Function.(*rm.DVCodedText); ok {
			out = appendText(out, &ruleParticipationFunc, f)
		}
		return appendText(out, &ruleParticipationMode, x.Mode)
	case *rm.PartyRelated:
		return []codedSlot{textSlot(&rulePartyRelationship, &x.Relationship)}
	case *rm.TermMapping:
		return appendText(nil, &ruleTermMappingPurpose, x.Purpose)
	case *rm.DVText:
		return appendPhrase(appendPhrase(nil, &ruleTextLanguage, x.Language), &ruleTextEncoding, x.Encoding)
	case *rm.DVCodedText:
		return appendPhrase(appendPhrase(nil, &ruleTextLanguage, x.Language), &ruleTextEncoding, x.Encoding)
	case *rm.DVQuantity:
		return appendPhrase(nil, &ruleNormalStatus, x.NormalStatus)
	case *rm.DVCount:
		return appendPhrase(nil, &ruleNormalStatus, x.NormalStatus)
	case *rm.DVProportion:
		return appendPhrase(nil, &ruleNormalStatus, x.NormalStatus)
	case *rm.DVOrdinal:
		return appendPhrase(nil, &ruleNormalStatus, x.NormalStatus)
	case *rm.DVScale:
		return appendPhrase(nil, &ruleNormalStatus, x.NormalStatus)
	case *rm.DVDateTime:
		return appendPhrase(nil, &ruleNormalStatus, x.NormalStatus)
	case *rm.DVDate:
		return appendPhrase(nil, &ruleNormalStatus, x.NormalStatus)
	case *rm.DVTime:
		return appendPhrase(nil, &ruleNormalStatus, x.NormalStatus)
	case *rm.DVDuration:
		return appendPhrase(nil, &ruleNormalStatus, x.NormalStatus)
	case *rm.DVMultimedia:
		out := encapsulatedSlots(x.Language, x.Charset)
		out = append(out, phraseSlot(&ruleMediaType, &x.MediaType))
		out = appendPhrase(out, &ruleCompression, x.CompressionAlgorithm)
		return appendPhrase(out, &ruleIntegrityCheck, x.IntegrityCheckAlgorithm)
	case *rm.DVParsable:
		return encapsulatedSlots(x.Language, x.Charset)
	}
	return nil
}

// entrySlots lists an ENTRY's mandatory language and encoding.
func entrySlots(language, encoding *rm.CodePhrase) []codedSlot {
	return []codedSlot{phraseSlot(&ruleEntryLanguage, language), phraseSlot(&ruleEntryEncoding, encoding)}
}

// encapsulatedSlots lists a DV_ENCAPSULATED's language and charset, each
// when present.
func encapsulatedSlots(language, charset *rm.CodePhrase) []codedSlot {
	return appendPhrase(appendPhrase(nil, &ruleEncapsulatedLanguage, language), &ruleEncapsulatedCharset, charset)
}

// textSlot is a DV_CODED_TEXT attribute under rule.
func textSlot(rule *codedRule, ct *rm.DVCodedText) codedSlot {
	return codedSlot{rule: rule, code: &ct.DefiningCode, text: ct}
}

// phraseSlot is a CODE_PHRASE attribute under rule.
func phraseSlot(rule *codedRule, cp *rm.CodePhrase) codedSlot {
	return codedSlot{rule: rule, code: cp}
}

// appendText appends an optional DV_CODED_TEXT attribute under rule when
// it is present.
func appendText(out []codedSlot, rule *codedRule, ct *rm.DVCodedText) []codedSlot {
	if ct == nil {
		return out
	}
	return append(out, textSlot(rule, ct))
}

// appendPhrase appends an optional CODE_PHRASE attribute under rule when
// it is present.
func appendPhrase(out []codedSlot, rule *codedRule, cp *rm.CodePhrase) []codedSlot {
	if cp == nil {
		return out
	}
	return append(out, phraseSlot(rule, cp))
}

// settleCoded keeps each coded attribute of v that codedSlots lists within
// the group or code set its rule names, where the OPT's own constraint on
// the attribute admits a member; opt is the OPT node of v, or nil for a
// value built from the BMM alone, which no OPT constrains. A code the walk
// wrote that the rule accepts stays. Otherwise the attribute takes, without
// a draw, the RM default where the constraint admits it, a coded text with
// the default's rubric in its group as its text; else the first code the
// constraint lists that the rule accepts and admits, a coded text taking
// that code's rubric as its text where the constraint on the text admits
// it (takeText); else the code the walk wrote stays, and the value breaks
// the rule. The default replaces the placeholder the walk writes where the
// OPT names the attribute with no code to choose. An attribute the OPT
// prohibits that no RM rule needs admits no value, so its code stays.
func (g *generator) settleCoded(opt *tcimpl.CompiledNode, v any) {
	for _, s := range codedSlots(v) {
		g.settleSlot(opt, v, s)
	}
}

// settleSlot settles s, a coded attribute of v, as settleCoded says.
func (g *generator) settleSlot(opt *tcimpl.CompiledNode, v any, s codedSlot) {
	r := s.rule
	if r.accepts(*s.code) {
		return
	}
	node, admitsNone := r.constraint(opt)
	if admitsNone {
		return
	}
	if r.def != nil {
		if def, ok := r.def(g, v); ok && s.write(node, def, true) {
			return
		}
	}
	for _, code := range listedCodes(node, r.terminologyID()) {
		if r.accepts(code) && s.write(node, code, false) {
			return
		}
	}
}

// constraint returns the OPT node that constrains r's attribute of the
// value whose OPT node is opt: the alternative the walk builds it under
// ([bindingChild]), nil where the OPT names no allowed child, which admits
// any code. That alternative is the OPT's value for the attribute, so a
// code only a later alternative names is never read. admitsNone is set
// where the OPT prohibits the attribute and no RM rule needs it. opt is
// nil for a value built from the BMM alone.
func (r *codedRule) constraint(opt *tcimpl.CompiledNode) (node *tcimpl.CompiledNode, admitsNone bool) {
	if opt == nil {
		return nil, false
	}
	attr := opt.Attribute(r.attr)
	if attr == nil {
		return nil, false
	}
	if attrProhibited(opt, attr) && (r.needed == nil || !r.needed(opt)) {
		return nil, true
	}
	return bindingChild(opt, r.attr), false
}

// write writes code over s where node, the OPT node that constrains the
// attribute, admits it, and reports whether it did. On a coded text, a
// default goes with its rubric in the rule's group as its text, where node
// admits both; any other code takes its rubric where node admits the text,
// and keeps the text the walk wrote where it does not.
func (s codedSlot) write(node *tcimpl.CompiledNode, code rm.CodePhrase, isDefault bool) bool {
	if node != nil && !phraseAdmitted(node, code) {
		return false
	}
	if s.text == nil {
		*s.code = code
		return true
	}
	rubric, hasRubric := s.rule.group.Rubric(code.CodeString)
	if isDefault {
		withRubric := *s.text
		withRubric.DefiningCode, withRubric.Value = code, rubric
		if !codedTextAdmits(node, withRubric) {
			return false
		}
		*s.text = withRubric
		return true
	}
	s.text.DefiningCode = code
	if hasRubric {
		takeText(node, s.text, rubric)
	}
	return true
}

// listedCodes returns, in the order the OPT lists them, the codes node
// lists for a code phrase, or for a coded text's defining code: a
// C_CODE_PHRASE's codes, in its terminology, or in terminologyID where it
// names none; or, for a CODE_PHRASE node whose code string carries a
// C_STRING list, each listed code string in each terminology the C_STRING
// on its terminology id lists, or in terminologyID where it lists none.
// Below node it reads, like the walk, the alternative the walk builds
// under ([bindingChild]). A nil node, a code list that is empty and a
// pattern list nothing.
func listedCodes(node *tcimpl.CompiledNode, terminologyID string) []rm.CodePhrase {
	if node == nil {
		return nil
	}
	if cp, ok := node.PrimitiveConstraint().(constraints.CodePhrase); ok {
		term := cp.Terminology
		if term == "" {
			term = terminologyID
		}
		out := make([]rm.CodePhrase, 0, len(cp.CodeList))
		for _, code := range cp.CodeList {
			out = append(out, rm.CodePhrase{CodeString: code, TerminologyID: rm.TerminologyID{Value: term}})
		}
		return out
	}
	if dc := bindingChild(node, "defining_code"); dc != nil {
		return listedCodes(dc, terminologyID)
	}
	terms := []string{terminologyID}
	if tid := bindingChild(node, "terminology_id"); tid != nil {
		if listed := listedStrings(tid.Attribute("value")); len(listed) > 0 {
			terms = listed
		}
	}
	var out []rm.CodePhrase
	for _, code := range listedStrings(node.Attribute("code_string")) {
		for _, term := range terms {
			out = append(out, rm.CodePhrase{CodeString: code, TerminologyID: rm.TerminologyID{Value: term}})
		}
	}
	return out
}

// listedStrings returns the list of the C_STRING the OPT puts on attr, its
// first child the OPT does not prohibit, as stringAdmitted reads it. An
// attribute the OPT does not name, or constrains with no C_STRING, lists
// none.
func listedStrings(attr *tcimpl.CompiledAttribute) []string {
	if attr == nil {
		return nil
	}
	children := tcimpl.AllowedChildNodes(attr)
	if len(children) == 0 {
		return nil
	}
	cs, ok := children[0].PrimitiveConstraint().(constraints.CString)
	if !ok {
		return nil
	}
	return cs.List
}
