package instance_test

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/terminology"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// The OPTs below put a PARTICIPATION, a TERM_MAPPING and an ISM_TRANSITION's
// transition and reason where the walk builds them, for the REQ-107 RM
// defaults on those classes: a PARTICIPATION's coded function and its mode,
// a TERM_MAPPING's match and purpose, and the transition that leads into an
// ISM_TRANSITION's current state.

// optParticipation is a PARTICIPATION with attrs.
func optParticipation(attrs ...string) string {
	return optNode("PARTICIPATION", "", attrs...)
}

// contextParticipationsOPT is a COMPOSITION whose EVENT_CONTEXT holds one
// participation with attrs.
func contextParticipationsOPT(attrs ...string) string {
	return optTemplate("COMPOSITION", optSingle("context", optNode("EVENT_CONTEXT", "",
		optMultiple("participations", optParticipation(attrs...)))))
}

// otherParticipationsOPT is an OBSERVATION that holds one other
// participation with attrs.
func otherParticipationsOPT(attrs ...string) string {
	return optTemplate("OBSERVATION", optMultiple("other_participations", optParticipation(attrs...)))
}

// participationTime is a PARTICIPATION time the OPT names as a
// DV_INTERVAL<DV_DATE_TIME> with a lower bound and no upper one.
var participationTime = optSingle("time", optNode("DV_INTERVAL&lt;DV_DATE_TIME&gt;", "",
	optSingle("lower", optNode("DV_DATE_TIME", ""))))

// bareCodedText is a DV_CODED_TEXT the OPT names with no constraint.
var bareCodedText = optNode("DV_CODED_TEXT", "")

// contextParticipations returns the participations of a generated
// COMPOSITION root's context.
func contextParticipations(t *testing.T, out any) []rm.Participation {
	t.Helper()
	comp := generatedComposition(t, out)
	if comp.Context == nil {
		t.Fatal("COMPOSITION.context absent, want the EVENT_CONTEXT the OPT names")
	}
	return comp.Context.Participations
}

// onlyParticipation returns the one participation of a generated
// COMPOSITION root's context.
func onlyParticipation(t *testing.T, out any) rm.Participation {
	t.Helper()
	ps := contextParticipations(t, out)
	if len(ps) != 1 {
		t.Fatalf("%d context participations generated, want 1", len(ps))
	}
	return ps[0]
}

// otherParticipations returns the other participations of a generated
// OBSERVATION root.
func otherParticipations(t *testing.T, out any) []rm.Participation {
	t.Helper()
	obs, ok := out.(*rm.Observation)
	if !ok {
		t.Fatalf("generated root is %T, want *rm.Observation", out)
	}
	return obs.OtherParticipations
}

// codedFunction returns a participation's function as the DV_CODED_TEXT it
// must be.
func codedFunction(t *testing.T, p rm.Participation) rm.DVCodedText {
	t.Helper()
	switch f := p.Function.(type) {
	case *rm.DVCodedText:
		return *f
	case rm.DVCodedText:
		return f
	}
	t.Fatalf("PARTICIPATION.function is %T, want a DV_CODED_TEXT", p.Function)
	return rm.DVCodedText{}
}

// checkPlainFunction fails t unless p's function is a DV_TEXT, not coded,
// with the text want.
func checkPlainFunction(t *testing.T, p rm.Participation, want string) {
	t.Helper()
	switch f := p.Function.(type) {
	case *rm.DVText:
		if f.Value != want {
			t.Errorf("PARTICIPATION.function = %q, want %q", f.Value, want)
		}
	case rm.DVText:
		if f.Value != want {
			t.Errorf("PARTICIPATION.function = %q, want %q", f.Value, want)
		}
	default:
		t.Errorf("PARTICIPATION.function is %T, want a DV_TEXT", p.Function)
	}
}

// checkSelfPerformer fails t unless p's performer is a PARTY_SELF.
func checkSelfPerformer(t *testing.T, p rm.Participation) {
	t.Helper()
	if _, ok := p.Performer.(*rm.PartySelf); !ok {
		t.Errorf("PARTICIPATION.performer is %T, want *rm.PartySelf", p.Performer)
	}
}

// checkCodedAs fails t unless got carries code, with the text text.
func checkCodedAs(t *testing.T, what string, got rm.DVCodedText, code rm.CodePhrase, text string) {
	t.Helper()
	if got.DefiningCode != code || got.Value != text {
		t.Errorf("%s = %q %+v, want %q %+v", what, got.Value, got.DefiningCode, text, code)
	}
}

// expectedIssue is an error a case expects a validator to report: a breach
// of an RM rule the OPT's own constraint makes (REQ-107 Exceptions), or a
// known gap of the template validator.
type expectedIssue struct {
	name  string
	match func(validation.Issue) bool
}

// ruleBroken expects a breach of the RM rule rule, such as
// PARTICIPATION.Mode_valid. The floor reports it, and so does the template
// validator, which runs the floor too.
func ruleBroken(rule string) expectedIssue {
	return expectedIssue{name: "a breach of " + rule, match: func(iss validation.Issue) bool { return iss.Rule == rule }}
}

// characterBindingGap is a known gap of the template validator, outside
// the generator: where the OPT puts a C_STRING on a TERM_MAPPING's match,
// it binds no rm.Character to the node the C_STRING sits on, an AOM 1.4
// STRING, so it reports rm_type_mismatch at the match whatever the match
// holds. The case expects it, so it fails once the validator binds a
// Character there, and the expectation is then dropped.
var characterBindingGap = expectedIssue{
	name: "the template validator's rm_type_mismatch on a C_STRING under TERM_MAPPING.match",
	match: func(iss validation.Issue) bool {
		return iss.Code == "rm_type_mismatch" && iss.Layer == validation.LayerTemplate && strings.HasSuffix(iss.Path, "/match")
	},
}

// checkValidBut reports every error the RM floor or the template validator
// finds in out that none of expected matches, and fails too when neither
// reports one of expected, so the case is seen to break that rule, or to
// meet that gap.
func checkValidBut(t *testing.T, out any, c *templatecompile.Compiled, expected ...expectedIssue) {
	t.Helper()
	seen := make([]bool, len(expected))
	check := func(validator string, issues []validation.Issue) {
		t.Helper()
		for _, iss := range issues {
			if iss.Severity != validation.Error {
				continue
			}
			i := slices.IndexFunc(expected, func(e expectedIssue) bool { return e.match(iss) })
			if i < 0 {
				t.Errorf("%s: %s @ %s: %s", validator, iss.Code, iss.Path, iss.Detail)
				continue
			}
			seen[i] = true
		}
	}
	check("ValidateRM", validation.ValidateRM(out).Issues)
	check("Validate", templateErrors(out, c))
	for i, e := range expected {
		if !seen[i] {
			t.Errorf("neither validator reports %s, which the case expects", e.name)
		}
	}
}

// runEveryWay generates opt under every policy, both value fills and both
// compile modes, and hands each root to check.
func runEveryWay(t *testing.T, name, opt string, check func(t *testing.T, opts instance.Options, out any, c *templatecompile.Compiled)) {
	t.Helper()
	for _, implicit := range []bool{true, false} {
		c := compileOPTText(t, opt, implicit)
		for _, opts := range defaultsOptions() {
			opts.Territory, opts.Composer = "NL", testComposer()
			t.Run(fmt.Sprintf("%s/implicit=%t/%v/%v", name, implicit, opts.Policy, opts.ValueFill), func(t *testing.T) {
				out, err := instance.Generate(t.Context(), c, opts)
				if err != nil {
					t.Fatalf("Generate: %v, want a root", err)
				}
				check(t, opts, out, c)
			})
		}
	}
}

// openehrCode is code in the openEHR terminology.
func openehrCode(code string) rm.CodePhrase {
	return rm.CodePhrase{CodeString: code, TerminologyID: rm.TerminologyID{Value: terminology.ID}}
}

// TestREQ107_ParticipationsAreWritten is the REQ-107 check that a
// PARTICIPATION the OPT puts in an EVENT_CONTEXT's participations, or in an
// ENTRY's other_participations, is written, with the RM defaults where the
// OPT gives no value: a function the walk writes as a DV_CODED_TEXT takes
// openehr 253|unknown| of the participation function group, and one it
// writes as a DV_TEXT stays a DV_TEXT; a mode takes openehr 193|not
// specified|; an omitted performer is a PARTY_SELF, and one the OPT names
// keeps what the OPT gives it; a time the OPT names is written. Where the
// OPT codes the function or the mode in local, the code the walk wrote
// stays and breaks that RM rule (Precedence 1). A participations list the
// OPT requires and gives no child gets one PARTICIPATION built from the BMM.
// No list the generator writes is empty (EVENT_CONTEXT
// Participations_validity, ENTRY Other_participations_valid). Each holds
// under every policy, both value fills and both compile modes, and passes
// both validators but for the rule a case breaks.
func TestREQ107_ParticipationsAreWritten(t *testing.T) {
	unknown := openehrCode("253")
	notSpecified := openehrCode("193")
	cases := []struct {
		name   string
		opt    string
		list   func(t *testing.T, out any) []rm.Participation
		check  func(t *testing.T, p rm.Participation)
		expect []expectedIssue
	}{
		{
			name: "context: function DV_TEXT, mode, performer omitted, time",
			opt: contextParticipationsOPT(optSingle("function", optNode("DV_TEXT", "")),
				optSingle("mode", bareCodedText), participationTime),
			list: contextParticipations,
			check: func(t *testing.T, p rm.Participation) {
				checkPlainFunction(t, p, walkText)
				if p.Mode == nil {
					t.Fatal("PARTICIPATION.mode absent, want openehr 193")
				}
				checkCodedAs(t, "PARTICIPATION.mode", *p.Mode, notSpecified, "not specified")
				checkSelfPerformer(t, p)
				if p.Time == nil || p.Time.Lower.Value != defaultsNow.Format(time.RFC3339) || !p.Time.UpperUnbounded {
					t.Errorf("PARTICIPATION.time = %+v, want a lower bound at Options.Now and no upper bound", p.Time)
				}
			},
		},
		{
			name: "context: function DV_CODED_TEXT with no code",
			opt:  contextParticipationsOPT(optSingle("function", bareCodedText)),
			list: contextParticipations,
			check: func(t *testing.T, p rm.Participation) {
				checkCodedAs(t, "PARTICIPATION.function", codedFunction(t, p), unknown, "unknown")
				if p.Mode != nil {
					t.Errorf("PARTICIPATION.mode = %+v, want none: the OPT does not name it", p.Mode)
				}
				checkSelfPerformer(t, p)
			},
		},
		{
			name: "context: function DV_CODED_TEXT a C_CODE_PHRASE names in openehr with no code",
			opt:  contextParticipationsOPT(optSingle("function", optCodedText(terminology.ID))),
			list: contextParticipations,
			check: func(t *testing.T, p rm.Participation) {
				checkCodedAs(t, "PARTICIPATION.function", codedFunction(t, p), unknown, "unknown")
			},
		},
		{
			name:   "context: function coded in local",
			opt:    contextParticipationsOPT(optSingle("function", optCodedText("local"))),
			list:   contextParticipations,
			expect: []expectedIssue{ruleBroken("PARTICIPATION.Function_valid")},
			check: func(t *testing.T, p rm.Participation) {
				checkCodedAs(t, "PARTICIPATION.function", codedFunction(t, p), localAt0000, walkText)
			},
		},
		{
			name:   "context: mode coded in local",
			opt:    contextParticipationsOPT(optSingle("function", optNode("DV_TEXT", "")), optSingle("mode", optCodedText("local"))),
			list:   contextParticipations,
			expect: []expectedIssue{ruleBroken("PARTICIPATION.Mode_valid")},
			check: func(t *testing.T, p rm.Participation) {
				if p.Mode == nil {
					t.Fatal("PARTICIPATION.mode absent, want the local code the walk wrote")
				}
				checkCodedAs(t, "PARTICIPATION.mode", *p.Mode, localAt0000, walkText)
			},
		},
		{
			name: "context: performer a PARTY_IDENTIFIED the OPT names",
			opt: contextParticipationsOPT(optSingle("function", optNode("DV_TEXT", "")),
				optSingle("performer", optNode("PARTY_IDENTIFIED", "", optStringAttr("name", "<list>Dr X</list>")))),
			list: contextParticipations,
			check: func(t *testing.T, p rm.Participation) {
				pi, ok := p.Performer.(*rm.PartyIdentified)
				if !ok || pi.Name == nil || *pi.Name != "Dr X" {
					t.Errorf("PARTICIPATION.performer = %#v, want a PARTY_IDENTIFIED named Dr X", p.Performer)
				}
			},
		},
		{
			name: "context: performer the abstract PARTY_PROXY",
			opt: contextParticipationsOPT(optSingle("function", optNode("DV_TEXT", "")),
				optSingle("performer", optNode("PARTY_PROXY", ""))),
			list:  contextParticipations,
			check: checkSelfPerformer,
		},
		{
			name: "context: participations the OPT requires with no child",
			opt:  optTemplate("COMPOSITION", optSingle("context", optNode("EVENT_CONTEXT", "", optMultiple("participations")))),
			list: contextParticipations,
			check: func(t *testing.T, p rm.Participation) {
				checkPlainFunction(t, p, walkText)
				checkSelfPerformer(t, p)
			},
		},
		{
			name: "OBSERVATION other_participations: function DV_CODED_TEXT and mode with no code",
			opt:  otherParticipationsOPT(optSingle("function", bareCodedText), optSingle("mode", bareCodedText)),
			list: otherParticipations,
			check: func(t *testing.T, p rm.Participation) {
				checkCodedAs(t, "PARTICIPATION.function", codedFunction(t, p), unknown, "unknown")
				if p.Mode == nil {
					t.Fatal("PARTICIPATION.mode absent, want openehr 193")
				}
				checkCodedAs(t, "PARTICIPATION.mode", *p.Mode, notSpecified, "not specified")
				checkSelfPerformer(t, p)
			},
		},
	}
	for _, tc := range cases {
		runEveryWay(t, tc.name, tc.opt, func(t *testing.T, _ instance.Options, out any, c *templatecompile.Compiled) {
			ps := tc.list(t, out)
			if len(ps) != 1 {
				t.Fatalf("%d participations generated, want 1", len(ps))
			}
			tc.check(t, ps[0])
			checkValidBut(t, out, c, tc.expect...)
		})
	}
}

// optTermMapping is a TERM_MAPPING with attrs.
func optTermMapping(attrs ...string) string {
	return optNode("TERM_MAPPING", "", attrs...)
}

// mappingTarget is a TERM_MAPPING target the OPT pins to ICD10 J45.
var mappingTarget = optSingle("target", optCodePhrase("ICD10", "J45"))

// mappedValueOPT is an ELEMENT whose value is a DV_CODED_TEXT coded
// SNOMED-CT 195967001 that holds one mapping with attrs.
func mappedValueOPT(attrs ...string) string {
	return optTemplate("ELEMENT", optSingle("value", optNode("DV_CODED_TEXT", "",
		optSingle("defining_code", optCodePhrase("SNOMED-CT", "195967001")),
		optMultiple("mappings", optTermMapping(attrs...)))))
}

// valueMappings returns the mappings of a generated ELEMENT root's value, a
// DV_TEXT or a DV_CODED_TEXT.
func valueMappings(t *testing.T, out any) []rm.TermMapping {
	t.Helper()
	switch v := generatedElement(t, out).Value.(type) {
	case *rm.DVCodedText:
		return v.Mappings
	case *rm.DVText:
		return v.Mappings
	}
	t.Fatalf("ELEMENT.value is %T, want a DV_TEXT or a DV_CODED_TEXT", generatedElement(t, out).Value)
	return nil
}

// matchCharacters are the characters TERM_MAPPING Match_valid admits.
var matchCharacters = []rm.Character{">", "=", "<", "?"}

// TestREQ107_TermMappingsAreWritten is the REQ-107 check that a TERM_MAPPING
// the OPT puts in a DV_CODED_TEXT's or DV_TEXT's mappings is written. Its
// match is one Match_valid admits: ? where the OPT leaves it silent or open,
// the character a C_STRING pins, and the first admitted one a pattern
// accepts (Precedence 3); a C_STRING that admits none makes Generate return
// an error wrapping ErrConstraintUnsatisfiable, and no root. A purpose the
// OPT names with no code takes openehr 669|public health|, one it pins keeps
// its code with that code's rubric, and one it codes in local keeps the
// walk's code and breaks Purpose_valid (Precedence 1). The target is the
// code the OPT pins. A mappings list the OPT requires and gives no child
// gets one TERM_MAPPING built from the BMM, whose match is ?. Each holds
// under every policy, both value fills and both compile modes, and passes
// both validators but for the rule a case breaks and, where the OPT puts a
// C_STRING on the match, the template validator's known gap there
// (characterBindingGap).
func TestREQ107_TermMappingsAreWritten(t *testing.T) {
	icd10 := rm.CodePhrase{CodeString: "J45", TerminologyID: rm.TerminologyID{Value: "ICD10"}}
	cases := []struct {
		name string
		opt  string
		// match is the match every setting must give, and matchExample the
		// one ExampleFill must give where the settings may differ.
		match, matchExample rm.Character
		check               func(t *testing.T, m rm.TermMapping)
		expect              []expectedIssue
	}{
		{
			name:  "match silent, purpose with no code",
			opt:   mappedValueOPT(optSingle("purpose", bareCodedText), mappingTarget),
			match: "?",
			check: func(t *testing.T, m rm.TermMapping) {
				if m.Purpose == nil {
					t.Fatal("TERM_MAPPING.purpose absent, want openehr 669")
				}
				checkCodedAs(t, "TERM_MAPPING.purpose", *m.Purpose, openehrCode("669"), "public health")
				if m.Target != icd10 {
					t.Errorf("TERM_MAPPING.target = %+v, want %+v", m.Target, icd10)
				}
			},
		},
		{name: "match an open C_STRING", opt: mappedValueOPT(optStringAttr("match", ""), mappingTarget), match: "?", expect: []expectedIssue{characterBindingGap}},
		{name: "match a pattern .*", opt: mappedValueOPT(optStringAttr("match", "<pattern>.*</pattern>"), mappingTarget), match: "?", expect: []expectedIssue{characterBindingGap}},
		{name: "match a C_STRING listing =", opt: mappedValueOPT(optStringAttr("match", "<list>=</list>"), mappingTarget), match: "=", expect: []expectedIssue{characterBindingGap}},
		{
			name:         "match a C_STRING listing = and <",
			opt:          mappedValueOPT(optStringAttr("match", "<list>=</list><list>&lt;</list>"), mappingTarget),
			matchExample: "=",
			expect:       []expectedIssue{characterBindingGap},
		},
		{name: "match a pattern that admits > and <", opt: mappedValueOPT(optStringAttr("match", "<pattern>[&lt;&gt;]</pattern>"), mappingTarget), match: "<", expect: []expectedIssue{characterBindingGap}},
		{name: "match a C_STRING listing a character and a word", opt: mappedValueOPT(optStringAttr("match", "<list>broader</list><list>&gt;</list>"), mappingTarget), match: ">", expect: []expectedIssue{characterBindingGap}},
		{
			name:  "purpose pinned in openehr",
			opt:   mappedValueOPT(optSingle("purpose", optCodedText(terminology.ID, "670")), mappingTarget),
			match: "?",
			check: func(t *testing.T, m rm.TermMapping) {
				if m.Purpose == nil {
					t.Fatal("TERM_MAPPING.purpose absent, want openehr 670")
				}
				checkCodedAs(t, "TERM_MAPPING.purpose", *m.Purpose, openehrCode("670"), "reimbursement")
			},
		},
		{
			name:   "purpose coded in local",
			opt:    mappedValueOPT(optSingle("purpose", optCodedText("local")), mappingTarget),
			match:  "?",
			expect: []expectedIssue{ruleBroken("TERM_MAPPING.Purpose_valid")},
			check: func(t *testing.T, m rm.TermMapping) {
				if m.Purpose == nil {
					t.Fatal("TERM_MAPPING.purpose absent, want the local code the walk wrote")
				}
				checkCodedAs(t, "TERM_MAPPING.purpose", *m.Purpose, localAt0000, walkText)
			},
		},
		{
			name:  "purpose not named",
			opt:   mappedValueOPT(mappingTarget),
			match: "?",
			check: func(t *testing.T, m rm.TermMapping) {
				if m.Purpose != nil {
					t.Errorf("TERM_MAPPING.purpose = %+v, want none: the OPT does not name it", m.Purpose)
				}
			},
		},
		{
			name: "DV_TEXT mappings with a C_STRING listing >",
			opt: optTemplate("ELEMENT", optSingle("value", optNode("DV_TEXT", "",
				optMultiple("mappings", optTermMapping(optStringAttr("match", "<list>&gt;</list>"), mappingTarget))))),
			match:  ">",
			expect: []expectedIssue{characterBindingGap},
		},
		{
			name:  "DV_TEXT mappings the OPT requires with no child",
			opt:   optTemplate("ELEMENT", optSingle("value", optNode("DV_TEXT", "", optMultiple("mappings")))),
			match: "?",
		},
	}
	for _, tc := range cases {
		runEveryWay(t, tc.name, tc.opt, func(t *testing.T, opts instance.Options, out any, c *templatecompile.Compiled) {
			ms := valueMappings(t, out)
			if len(ms) != 1 {
				t.Fatalf("%d mappings generated, want 1", len(ms))
			}
			m := ms[0]
			if !slices.Contains(matchCharacters, m.Match) {
				t.Errorf("TERM_MAPPING.match = %q, want one of %q (Match_valid)", m.Match, matchCharacters)
			}
			if tc.match != "" && m.Match != tc.match {
				t.Errorf("TERM_MAPPING.match = %q, want %q", m.Match, tc.match)
			}
			if tc.matchExample != "" && opts.ValueFill == instance.ExampleFill && m.Match != tc.matchExample {
				t.Errorf("TERM_MAPPING.match = %q, want the list's first member %q", m.Match, tc.matchExample)
			}
			if tc.check != nil {
				tc.check(t, m)
			}
			checkValidBut(t, out, c, tc.expect...)
		})
	}
}

// TestREQ107_TermMappingMatchNoCharacterIsUnsatisfiable is the REQ-107
// check that a C_STRING on a TERM_MAPPING's match that admits none of the
// characters Match_valid admits makes Generate return an error wrapping
// ErrConstraintUnsatisfiable, and no root, under every policy, both value
// fills and both compile modes.
func TestREQ107_TermMappingMatchNoCharacterIsUnsatisfiable(t *testing.T) {
	for _, pin := range []string{"<list>x</list>", "<pattern>[a-z]</pattern>", "<list>broader</list><list>narrower</list>"} {
		for _, implicit := range []bool{true, false} {
			c := compileOPTText(t, mappedValueOPT(optStringAttr("match", pin), mappingTarget), implicit)
			for _, opts := range defaultsOptions() {
				t.Run(fmt.Sprintf("%s/implicit=%t/%v/%v", pin, implicit, opts.Policy, opts.ValueFill), func(t *testing.T) {
					out, err := instance.Generate(t.Context(), c, opts)
					if !errors.Is(err, instance.ErrConstraintUnsatisfiable) {
						t.Errorf("Generate error = %v, want one wrapping ErrConstraintUnsatisfiable", err)
					}
					if out != nil {
						t.Errorf("Generate returned %T, want no root", out)
					}
				})
			}
		}
	}
}

// instructionTransitions pairs each state of the openEHR instruction states
// group with the transition of the instruction state machine that leads
// into it, and initial with initiate, which no transition leads into.
var instructionTransitions = []struct{ state, transition string }{
	{"524", "535"}, // initial: initiate
	{"526", "535"}, // planned: initiate
	{"527", "537"}, // postponed: postpone
	{"529", "539"}, // scheduled: schedule
	{"245", "540"}, // active: start
	{"530", "544"}, // suspended: suspend
	{"532", "548"}, // completed: finish
	{"531", "547"}, // aborted: abort
	{"528", "166"}, // cancelled: cancel
	{"533", "549"}, // expired: time out
}

// actionTransition returns the ISM_TRANSITION of a generated ACTION root.
func actionTransition(t *testing.T, out any) rm.IsmTransition {
	t.Helper()
	a, ok := out.(*rm.Action)
	if !ok {
		t.Fatalf("generated root is %T, want *rm.Action", out)
	}
	return a.IsmTransition
}

// checkTransition fails t unless iv's transition is openehr code with its
// rubric in the instruction transitions group.
func checkTransition(t *testing.T, iv rm.IsmTransition, code string) {
	t.Helper()
	if iv.Transition == nil {
		t.Fatalf("ISM_TRANSITION.transition absent, want openehr %s", code)
	}
	rubric, _ := terminology.InstructionTransitions.Rubric(code)
	checkCodedAs(t, "ISM_TRANSITION.transition", *iv.Transition, openehrCode(code), rubric)
}

// TestREQ107_ISMTransitionIsWritten is the REQ-107 check that an
// ISM_TRANSITION's transition the OPT names is written, and where the OPT
// gives it no code, that it takes the transition of the openEHR instruction
// state machine that leads into the current state written, initiate beside
// initial and beside a current state outside the instruction states group,
// with that code's rubric, so Transition_valid holds. A transition the OPT
// pins keeps its code, with its rubric; one it codes in local keeps the code
// the walk wrote and breaks Transition_valid (Precedence 1). A current state
// outside the group is kept as the OPT pins it, and breaks
// Current_state_valid. A reason the OPT names is written. Each holds under
// every policy, both value fills and both compile modes, and passes both
// validators but for the rule a case breaks.
func TestREQ107_ISMTransitionIsWritten(t *testing.T) {
	type ismCase struct {
		name   string
		opt    string
		check  func(t *testing.T, iv rm.IsmTransition)
		expect []expectedIssue
	}
	cases := []ismCase{
		{
			name:  "transition silent",
			opt:   optISMTransition(optSingle("transition")),
			check: func(t *testing.T, iv rm.IsmTransition) { checkTransition(t, iv, "535") },
		},
		{
			name:  "transition named with no code",
			opt:   optISMTransition(optSingle("transition", bareCodedText)),
			check: func(t *testing.T, iv rm.IsmTransition) { checkTransition(t, iv, "535") },
		},
		{
			name: "transition pinned",
			opt: optISMTransition(optSingle("current_state", optCodedText(terminology.ID, "532")),
				optSingle("transition", optCodedText(terminology.ID, "540"))),
			check: func(t *testing.T, iv rm.IsmTransition) { checkTransition(t, iv, "540") },
		},
		{
			name:   "transition coded in local",
			opt:    optISMTransition(optSingle("transition", optCodedText("local"))),
			expect: []expectedIssue{ruleBroken("ISM_TRANSITION.Transition_valid")},
			check: func(t *testing.T, iv rm.IsmTransition) {
				if iv.Transition == nil {
					t.Fatal("ISM_TRANSITION.transition absent, want the local code the walk wrote")
				}
				checkCodedAs(t, "ISM_TRANSITION.transition", *iv.Transition, localAt0000, walkText)
			},
		},
		{
			name: "current state outside the instruction states group",
			opt: optISMTransition(optSingle("current_state", optCodedText(terminology.ID, "433")),
				optSingle("transition", bareCodedText)),
			expect: []expectedIssue{ruleBroken("ISM_TRANSITION.Current_state_valid")},
			check: func(t *testing.T, iv rm.IsmTransition) {
				if iv.CurrentState.DefiningCode != openehrCode("433") {
					t.Errorf("ISM_TRANSITION.current_state = %+v, want the openehr 433 the OPT pins", iv.CurrentState.DefiningCode)
				}
				checkTransition(t, iv, "535")
			},
		},
		{
			// A local code that shares its code string with completed is not
			// the openEHR state, so it is outside the group too.
			name: "current state a local code with the code string of completed",
			opt: optISMTransition(optSingle("current_state", optCodedText("local", "532")),
				optSingle("transition", bareCodedText)),
			expect: []expectedIssue{ruleBroken("ISM_TRANSITION.Current_state_valid")},
			check:  func(t *testing.T, iv rm.IsmTransition) { checkTransition(t, iv, "535") },
		},
		{
			name: "reason",
			opt:  optISMTransition(optMultiple("reason", optNode("DV_TEXT", ""))),
			check: func(t *testing.T, iv rm.IsmTransition) {
				if len(iv.Reason) != 1 || iv.Reason[0] == nil || iv.Reason[0].GetValue() != walkText {
					t.Errorf("ISM_TRANSITION.reason = %+v, want one DV_TEXT %q", iv.Reason, walkText)
				}
				if iv.Transition != nil {
					t.Errorf("ISM_TRANSITION.transition = %+v, want none: the OPT does not name it", iv.Transition)
				}
			},
		},
	}
	for _, pair := range instructionTransitions {
		rubric, _ := terminology.InstructionStates.Rubric(pair.state)
		cases = append(cases, ismCase{
			name: "current state " + rubric,
			opt: optISMTransition(optSingle("current_state", optCodedText(terminology.ID, pair.state)),
				optSingle("transition", bareCodedText)),
			check: func(t *testing.T, iv rm.IsmTransition) {
				checkCodedAs(t, "ISM_TRANSITION.current_state", iv.CurrentState, openehrCode(pair.state), rubric)
				checkTransition(t, iv, pair.transition)
			},
		})
	}
	for _, tc := range cases {
		runEveryWay(t, tc.name, tc.opt, func(t *testing.T, _ instance.Options, out any, c *templatecompile.Compiled) {
			tc.check(t, actionTransition(t, out))
			checkValidBut(t, out, c, tc.expect...)
		})
	}
}

// TestREQ107_CodeTheOPTGivesIsKeptOverTheDefault is the REQ-107 check that
// an RM default goes only where the OPT gives no code: where the OPT lists
// several codes the default is among, the code the walk wrote from that
// list stays, the first listed under ExampleFill and a listed one under
// RandomFill, with its rubric, on a PARTICIPATION's mode, a TERM_MAPPING's
// purpose and an ISM_TRANSITION's transition. Each holds under every
// policy and both compile modes, and passes both validators.
func TestREQ107_CodeTheOPTGivesIsKeptOverTheDefault(t *testing.T) {
	cases := []struct {
		name  string
		opt   string
		group *terminology.Group
		codes []string // the codes the OPT lists, the first one ExampleFill writes
		coded func(t *testing.T, out any) *rm.DVCodedText
	}{
		{
			name: "PARTICIPATION mode", group: terminology.ParticipationMode, codes: []string{"216", "193"},
			opt: contextParticipationsOPT(optSingle("function", optNode("DV_TEXT", "")),
				optSingle("mode", optCodedText(terminology.ID, "216", "193"))),
			coded: func(t *testing.T, out any) *rm.DVCodedText { return onlyParticipation(t, out).Mode },
		},
		{
			name: "TERM_MAPPING purpose", group: terminology.TermMappingPurpose, codes: []string{"670", "669"},
			opt: mappedValueOPT(optSingle("purpose", optCodedText(terminology.ID, "670", "669")), mappingTarget),
			coded: func(t *testing.T, out any) *rm.DVCodedText {
				t.Helper()
				ms := valueMappings(t, out)
				if len(ms) != 1 {
					t.Fatalf("%d mappings generated, want 1", len(ms))
				}
				return ms[0].Purpose
			},
		},
		{
			// The current state completed derives finish, which the OPT lists
			// after start.
			name: "ISM_TRANSITION transition", group: terminology.InstructionTransitions, codes: []string{"540", "548"},
			opt: optISMTransition(optSingle("current_state", optCodedText(terminology.ID, "532")),
				optSingle("transition", optCodedText(terminology.ID, "540", "548"))),
			coded: func(t *testing.T, out any) *rm.DVCodedText {
				t.Helper()
				return actionTransition(t, out).Transition
			},
		},
	}
	for _, tc := range cases {
		runEveryWay(t, tc.name, tc.opt, func(t *testing.T, opts instance.Options, out any, c *templatecompile.Compiled) {
			got := tc.coded(t, out)
			if got == nil {
				t.Fatalf("%s absent, want one of the codes the OPT lists", tc.name)
			}
			code := got.DefiningCode.CodeString
			if !slices.Contains(tc.codes, code) || got.DefiningCode.TerminologyID.Value != terminology.ID {
				t.Fatalf("%s = %+v, want one of openehr %v", tc.name, got.DefiningCode, tc.codes)
			}
			if opts.ValueFill == instance.ExampleFill && code != tc.codes[0] {
				t.Errorf("%s = openehr %s, want %s, the first code the OPT lists, kept over the default", tc.name, code, tc.codes[0])
			}
			rubric, _ := tc.group.Rubric(code)
			checkCodedAs(t, tc.name, *got, openehrCode(code), rubric)
			checkValidBut(t, out, c)
		})
	}
}
