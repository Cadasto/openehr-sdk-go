package rmwrite

import (
	"errors"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// openehrText is a DV_CODED_TEXT showing text, coded code in the openEHR
// terminology.
func openehrText(text, code string) rm.DVCodedText {
	return rm.DVCodedText{Value: text, DefiningCode: rm.CodePhrase{CodeString: code, TerminologyID: rm.TerminologyID{Value: "openehr"}}}
}

// TestREQ107_ParticipationAndTermMappingWriters checks that EnsureSingle
// writes a PARTICIPATION's function, mode, performer and time, a
// TERM_MAPPING's match, purpose and target, and an ISM_TRANSITION's
// transition, and reads each back. A function is stored as given, a
// DV_TEXT or a DV_CODED_TEXT, and so is a performer, a PARTY_SELF or a
// PARTY_IDENTIFIED. A match takes a one-character string or an
// rm.Character. A coded text, a code phrase and an interval are taken as a
// value or as a pointer.
func TestREQ107_ParticipationAndTermMappingWriters(t *testing.T) {
	t.Parallel()
	mode := openehrText("face-to-face communication", "216")
	purpose := openehrText("reimbursement", "670")
	transition := openehrText("start", "540")
	target := rm.CodePhrase{CodeString: "12345", TerminologyID: rm.TerminologyID{Value: "SNOMED-CT"}}
	text := &rm.DVText{Value: "requester"}
	coded := &rm.DVCodedText{Value: "unknown", DefiningCode: rm.CodePhrase{CodeString: "253", TerminologyID: rm.TerminologyID{Value: "openehr"}}}
	self := &rm.PartySelf{}
	named := "Dr X"
	identified := &rm.PartyIdentified{Name: &named}
	when := rm.DVInterval[rm.DVDateTime]{Lower: rm.DVDateTime{Value: "2021-03-04T05:06:07Z"}, LowerIncluded: true, UpperUnbounded: true}

	byText, byCoded, bySelf, byIdentified := &rm.Participation{}, &rm.Participation{}, &rm.Participation{}, &rm.Participation{}
	modeValue, timeValue := &rm.Participation{}, &rm.Participation{}
	mapping, mappingByValue := &rm.TermMapping{}, &rm.TermMapping{}
	ism, ismByValue := &rm.IsmTransition{}, &rm.IsmTransition{}
	cases := []valueWrite{
		{"PARTICIPATION function DV_TEXT", byText, "function", text, func() any { return byText.Function }, rm.DVTextLike(text)},
		{"PARTICIPATION function DV_CODED_TEXT", byCoded, "function", coded, func() any { return byCoded.Function }, rm.DVTextLike(coded)},
		{"PARTICIPATION mode", byText, "mode", &mode, func() any { return byText.Mode.DefiningCode }, mode.DefiningCode},
		{"PARTICIPATION mode by value", modeValue, "mode", mode, func() any { return modeValue.Mode.DefiningCode }, mode.DefiningCode},
		{"PARTICIPATION performer PARTY_SELF", bySelf, "performer", self, func() any { return bySelf.Performer }, rm.PartyProxy(self)},
		{"PARTICIPATION performer PARTY_IDENTIFIED", byIdentified, "performer", identified, func() any { return byIdentified.Performer }, rm.PartyProxy(identified)},
		{"PARTICIPATION time", byText, "time", &when, func() any { return byText.Time.Lower.Value }, when.Lower.Value},
		{"PARTICIPATION time by value", timeValue, "time", when, func() any { return timeValue.Time.Lower.Value }, when.Lower.Value},
		{"TERM_MAPPING match string", mapping, "match", "=", func() any { return mapping.Match }, rm.Character("=")},
		{"TERM_MAPPING match Character", mappingByValue, "match", rm.Character("<"), func() any { return mappingByValue.Match }, rm.Character("<")},
		{"TERM_MAPPING purpose", mapping, "purpose", &purpose, func() any { return mapping.Purpose.DefiningCode }, purpose.DefiningCode},
		{"TERM_MAPPING purpose by value", mappingByValue, "purpose", purpose, func() any { return mappingByValue.Purpose.DefiningCode }, purpose.DefiningCode},
		{"TERM_MAPPING target", mapping, "target", &target, func() any { return mapping.Target }, target},
		{"TERM_MAPPING target by value", mappingByValue, "target", target, func() any { return mappingByValue.Target }, target},
		{"ISM_TRANSITION transition", ism, "transition", &transition, func() any { return ism.Transition.DefiningCode }, transition.DefiningCode},
		{"ISM_TRANSITION transition by value", ismByValue, "transition", transition, func() any { return ismByValue.Transition.DefiningCode }, transition.DefiningCode},
	}
	for _, tc := range cases {
		if err := EnsureSingle(tc.parent, tc.attr, tc.child); err != nil {
			t.Errorf("%s: EnsureSingle(%T, %q, %T) = %v, want nil", tc.name, tc.parent, tc.attr, tc.child, err)
			continue
		}
		if got := tc.got(); got != tc.want {
			t.Errorf("%s: %T.%s = %#v, want %#v", tc.name, tc.parent, tc.attr, got, tc.want)
		}
	}
}

// participationList is a list of participations on a parent, and how to
// read it back.
type participationList struct {
	name   string
	parent any
	attr   string
	list   func() []rm.Participation
}

// TestREQ107_ParticipationsMappingsAndReasonsAppend checks that
// AppendMultiple appends a PARTICIPATION to an EVENT_CONTEXT's
// participations and to the other_participations of each of the five
// ENTRY classes, a TERM_MAPPING to a DV_TEXT's or DV_CODED_TEXT's
// mappings, and a DV_TEXT or DV_CODED_TEXT to an ISM_TRANSITION's reason,
// each taken as a value or as a pointer, in order.
func TestREQ107_ParticipationsMappingsAndReasonsAppend(t *testing.T) {
	t.Parallel()
	first := rm.Participation{Performer: &rm.PartySelf{}}
	second := rm.Participation{Function: &rm.DVText{Value: "second"}}
	context, observation, evaluation := &rm.EventContext{}, &rm.Observation{}, &rm.Evaluation{}
	instruction, action, admin := &rm.Instruction{}, &rm.Action{}, &rm.AdminEntry{}
	participationLists := []participationList{
		{"EVENT_CONTEXT participations", context, "participations", func() []rm.Participation { return context.Participations }},
		{"OBSERVATION other_participations", observation, "other_participations", func() []rm.Participation { return observation.OtherParticipations }},
		{"EVALUATION other_participations", evaluation, "other_participations", func() []rm.Participation { return evaluation.OtherParticipations }},
		{"INSTRUCTION other_participations", instruction, "other_participations", func() []rm.Participation { return instruction.OtherParticipations }},
		{"ACTION other_participations", action, "other_participations", func() []rm.Participation { return action.OtherParticipations }},
		{"ADMIN_ENTRY other_participations", admin, "other_participations", func() []rm.Participation { return admin.OtherParticipations }},
	}
	for _, tc := range participationLists {
		for _, child := range []any{&first, second} {
			if err := AppendMultiple(tc.parent, tc.attr, child); err != nil {
				t.Fatalf("%s: AppendMultiple(%T) = %v, want nil", tc.name, child, err)
			}
		}
		got := tc.list()
		if len(got) != 2 || got[0].Performer == nil || got[1].Function == nil || got[1].Function.GetValue() != "second" {
			t.Errorf("%s = %+v, want the two participations in order", tc.name, got)
		}
	}

	mapping := rm.TermMapping{Match: "=", Target: rm.CodePhrase{CodeString: "1", TerminologyID: rm.TerminologyID{Value: "SNOMED-CT"}}}
	other := rm.TermMapping{Match: ">", Target: rm.CodePhrase{CodeString: "2", TerminologyID: rm.TerminologyID{Value: "SNOMED-CT"}}}
	text, coded := &rm.DVText{}, &rm.DVCodedText{}
	for _, tc := range []struct {
		name   string
		parent any
		list   func() []rm.TermMapping
	}{
		{"DV_TEXT mappings", text, func() []rm.TermMapping { return text.Mappings }},
		{"DV_CODED_TEXT mappings", coded, func() []rm.TermMapping { return coded.Mappings }},
	} {
		for _, child := range []any{&mapping, other} {
			if err := AppendMultiple(tc.parent, "mappings", child); err != nil {
				t.Fatalf("%s: AppendMultiple(%T) = %v, want nil", tc.name, child, err)
			}
		}
		if got := tc.list(); len(got) != 2 || got[0] != mapping || got[1] != other {
			t.Errorf("%s = %+v, want %+v then %+v", tc.name, got, mapping, other)
		}
	}

	ism := &rm.IsmTransition{}
	reason := &rm.DVText{Value: "patient request"}
	codedReason := &rm.DVCodedText{Value: "adverse reaction", DefiningCode: rm.CodePhrase{CodeString: "at0001", TerminologyID: rm.TerminologyID{Value: "local"}}}
	for _, child := range []any{reason, codedReason} {
		if err := AppendMultiple(ism, "reason", child); err != nil {
			t.Fatalf("ISM_TRANSITION reason: AppendMultiple(%T) = %v, want nil", child, err)
		}
	}
	if len(ism.Reason) != 2 || ism.Reason[0] != rm.DVTextLike(reason) || ism.Reason[1] != rm.DVTextLike(codedReason) {
		t.Errorf("ISM_TRANSITION.reason = %+v, want the two reasons as given, in order", ism.Reason)
	}
}

// TestREQ107_ParticipationAndTermMappingWritersRefuse checks that each new
// case refuses a child of the wrong Go type with ErrTypeMismatch: a nil or
// typed-nil function, performer or reason among them, and a match that is
// not exactly one character, which is no RM Character. An attribute the
// class does not declare stays ErrUnknownAttribute.
func TestREQ107_ParticipationAndTermMappingWritersRefuse(t *testing.T) {
	t.Parallel()
	single := []struct {
		parent any
		attr   string
		child  any
	}{
		{&rm.Participation{}, "function", "requester"},
		{&rm.Participation{}, "function", (*rm.DVText)(nil)},
		{&rm.Participation{}, "function", (*rm.DVCodedText)(nil)},
		{&rm.Participation{}, "mode", &rm.DVText{}},
		{&rm.Participation{}, "performer", &rm.PartyRef{}},
		{&rm.Participation{}, "performer", (*rm.PartySelf)(nil)},
		{&rm.Participation{}, "time", &rm.DVInterval[rm.DVDate]{}},
		{&rm.TermMapping{}, "match", "=="},
		{&rm.TermMapping{}, "match", ""},
		{&rm.TermMapping{}, "match", "\xff"}, // one byte, so one rune, but no valid UTF-8
		{&rm.TermMapping{}, "match", rm.Character("<>")},
		{&rm.TermMapping{}, "match", '='},
		{&rm.TermMapping{}, "purpose", &rm.DVText{}},
		{&rm.TermMapping{}, "target", "SNOMED-CT::12345"},
		{&rm.IsmTransition{}, "transition", &rm.DVText{}},
	}
	for _, tc := range single {
		if err := EnsureSingle(tc.parent, tc.attr, tc.child); !errors.Is(err, ErrTypeMismatch) {
			t.Errorf("EnsureSingle(%T, %q, %#v) = %v, want ErrTypeMismatch", tc.parent, tc.attr, tc.child, err)
		}
	}
	multiple := []struct {
		parent any
		attr   string
		child  any
	}{
		{&rm.EventContext{}, "participations", &rm.PartySelf{}},
		{&rm.Observation{}, "other_participations", &rm.DVText{}},
		{&rm.Evaluation{}, "other_participations", (*rm.Participation)(nil)},
		{&rm.Instruction{}, "other_participations", &rm.Activity{}},
		{&rm.Action{}, "other_participations", "requester"},
		{&rm.AdminEntry{}, "other_participations", &rm.TermMapping{}},
		{&rm.DVText{}, "mappings", &rm.CodePhrase{}},
		{&rm.DVCodedText{}, "mappings", (*rm.TermMapping)(nil)},
		{&rm.IsmTransition{}, "reason", "patient request"},
		{&rm.IsmTransition{}, "reason", (*rm.DVText)(nil)},
	}
	for _, tc := range multiple {
		if err := AppendMultiple(tc.parent, tc.attr, tc.child); !errors.Is(err, ErrTypeMismatch) {
			t.Errorf("AppendMultiple(%T, %q, %#v) = %v, want ErrTypeMismatch", tc.parent, tc.attr, tc.child, err)
		}
	}
	for _, parent := range []any{&rm.Participation{}, &rm.TermMapping{}, &rm.IsmTransition{}} {
		if err := EnsureSingle(parent, "no_such_attr", "x"); !errors.Is(err, ErrUnknownAttribute) {
			t.Errorf("EnsureSingle(%T, no_such_attr) = %v, want ErrUnknownAttribute", parent, err)
		}
	}
	for _, parent := range []any{
		&rm.EventContext{}, &rm.Observation{}, &rm.Evaluation{}, &rm.Instruction{}, &rm.Action{},
		&rm.AdminEntry{}, &rm.DVText{}, &rm.DVCodedText{}, &rm.IsmTransition{},
	} {
		if err := AppendMultiple(parent, "no_such_attr", &rm.Participation{}); !errors.Is(err, ErrUnknownAttribute) {
			t.Errorf("AppendMultiple(%T, no_such_attr) = %v, want ErrUnknownAttribute", parent, err)
		}
	}
}
