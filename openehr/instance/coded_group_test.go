package instance_test

import (
	"fmt"
	mrand "math/rand/v2"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/terminology"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// randomSeeds is how many RandomFill sources a Precedence 4 case sweeps. A
// case lists one or two codes outside the set among at most four, so the
// sweep draws an outside code under every policy and compile mode.
const randomSeeds = 32

// heldCode is the code phrase an RM rule reads on a generated root, and the
// text of the coded text that holds it, if the attribute is one.
type heldCode struct {
	code  rm.CodePhrase
	text  string
	coded bool
}

// codedHeld is what a coded text holds.
func codedHeld(ct rm.DVCodedText) heldCode {
	return heldCode{code: ct.DefiningCode, text: ct.Value, coded: true}
}

// phraseHeld is what an optional code phrase attribute holds; it stops t
// when the attribute is unset.
func phraseHeld(t *testing.T, what string, cp *rm.CodePhrase) heldCode {
	t.Helper()
	if cp == nil {
		t.Fatalf("%s is unset, want the code the OPT lists", what)
	}
	return heldCode{code: *cp}
}

// codedPresent is what an optional coded text attribute holds; it stops t
// when the attribute is unset.
func codedPresent(t *testing.T, what string, ct *rm.DVCodedText) heldCode {
	t.Helper()
	if ct == nil {
		t.Fatalf("%s is unset, want the code the OPT lists", what)
	}
	return codedHeld(*ct)
}

// codeIn is code in terminologyID.
func codeIn(terminologyID, code string) rm.CodePhrase {
	return rm.CodePhrase{CodeString: code, TerminologyID: rm.TerminologyID{Value: terminologyID}}
}

// inSetCase is one REQ-107 Precedence 4 case: an OPT whose constraint on a
// coded attribute the RM keeps within a group or a code set lists codes in
// and out of that set.
type inSetCase struct {
	name string
	opt  string
	read func(t *testing.T, out any) heldCode
	// group is the group whose rubric a coded text takes as its text; nil
	// for a code phrase attribute.
	group *terminology.Group
	// example is the code the attribute holds under ExampleFill. random
	// lists the codes it may hold under RandomFill; example alone when
	// empty.
	example rm.CodePhrase
	random  []rm.CodePhrase
	// broken is the rule the root breaks where the OPT admits no member of
	// the set, and brokenPath where the floor reports it; "" for a root
	// both validators pass.
	broken, brokenPath string
}

// Readers of the attributes several Precedence 4 cases share.
var (
	categoryOf = func(t *testing.T, out any) heldCode {
		return codedHeld(generatedComposition(t, out).Category)
	}
	settingOf = func(t *testing.T, out any) heldCode {
		comp := generatedComposition(t, out)
		if comp.Context == nil {
			t.Fatal("COMPOSITION.context is absent, want an EVENT_CONTEXT")
		}
		return codedHeld(comp.Context.Setting)
	}
	actionOf = func(t *testing.T, out any) *rm.Action {
		t.Helper()
		a, ok := out.(*rm.Action)
		if !ok {
			t.Fatalf("generated root is %T, want *rm.Action", out)
		}
		return a
	}
	modeOf = func(t *testing.T, out any) heldCode {
		return codedPresent(t, "PARTICIPATION.mode", onlyParticipation(t, out).Mode)
	}
)

// modeOPT is a COMPOSITION whose one context participation has a DV_TEXT
// function and the mode child.
func modeOPT(mode string) string {
	return contextParticipationsOPT(optSingle("function", optNode("DV_TEXT", "")), optSingle("mode", mode))
}

// elementValueOPT is an ELEMENT whose value is a node of rmType with attrs.
func elementValueOPT(rmType string, attrs ...string) string {
	return optTemplate("ELEMENT", optSingle("value", optNode(rmType, "", attrs...)))
}

// optCodePhraseStrings is a CODE_PHRASE whose terminology id and code
// string the OPT constrains with C_STRING lists, so any listed terminology
// goes with any listed code.
func optCodePhraseStrings(terminologies, codes []string) string {
	list := func(items []string) string {
		var s strings.Builder
		for _, item := range items {
			s.WriteString("<list>" + item + "</list>")
		}
		return s.String()
	}
	return optNode("CODE_PHRASE", "",
		optSingle("terminology_id", optNode("TERMINOLOGY_ID", "", optStringAttr("value", list(terminologies)))),
		optStringAttr("code_string", list(codes)))
}

// TestREQ107_CodedAttributeKeepsToItsGroupOrCodeSet is the REQ-107
// Precedence 4 check on each row of the REQ-112 Coded invariants table: where
// the OPT's constraint on the attribute lists codes in and out of the group
// or code set the RM ties it to, the generator keeps a code the walk wrote
// that the set has, and otherwise writes, without a draw, the RM default
// where the OPT admits it, else the first code the OPT lists that the set
// has, a coded text taking that code's rubric in its group as its text.
// Under ExampleFill the walk writes the first listed code, which is outside
// the set; under RandomFill each case is swept over randomSeeds sources, so
// the walk draws an outside code too. Where the OPT lists no member of the
// set, the code the walk wrote stays, and the root breaks the rule (REQ-107
// § Exceptions and known gaps): both validators report exactly that
// code_not_in_value_set. Every other root passes both validators. Each case
// holds under every policy, both value fills and both compile modes.
func TestREQ107_CodedAttributeKeepsToItsGroupOrCodeSet(t *testing.T) {
	const other = "271" // no information: a null flavour, in no group below but that one
	textRoot := func(t *testing.T, out any) *rm.DVText { return rootElementValue[*rm.DVText](t, out) }
	multimedia := func(t *testing.T, out any) *rm.DVMultimedia { return rootElementValue[*rm.DVMultimedia](t, out) }
	parsable := func(t *testing.T, out any) *rm.DVParsable { return rootElementValue[*rm.DVParsable](t, out) }
	cases := []inSetCase{
		// One row per row of the Coded invariants table: the OPT lists a
		// code outside the set first, then one inside it, and the RM default,
		// where there is one, is not listed.
		{
			name:    "COMPOSITION category",
			opt:     optTemplate("COMPOSITION", optSingle("category", optCodedText(terminology.ID, other, "431"))),
			read:    categoryOf,
			group:   terminology.CompositionCategory,
			example: openehrCode("431"),
		},
		{
			name: "COMPOSITION language",
			opt:  optTemplate("COMPOSITION", optSingle("language", optCodePhrase("ISO_639-1", "xx", "de"))),
			read: func(t *testing.T, out any) heldCode {
				return heldCode{code: generatedComposition(t, out).Language}
			},
			example: codeIn("ISO_639-1", "de"),
		},
		{
			name: "COMPOSITION territory",
			opt:  optTemplate("COMPOSITION", optSingle("territory", optCodePhrase("ISO_3166-1", "XX", "BE"))),
			read: func(t *testing.T, out any) heldCode {
				return heldCode{code: generatedComposition(t, out).Territory}
			},
			example: codeIn("ISO_3166-1", "BE"),
		},
		{
			name: "EVENT_CONTEXT setting",
			opt: optTemplate("COMPOSITION", optSingle("context", optNode("EVENT_CONTEXT", "",
				optSingle("setting", optCodedText(terminology.ID, other, "225"))))),
			read:    settingOf,
			group:   terminology.Setting,
			example: openehrCode("225"),
		},
		{
			name:    "ENTRY language",
			opt:     yieldAction(optCodePhrase("ISO_639-1", "xx", "de"), "", ""),
			read:    func(t *testing.T, out any) heldCode { return heldCode{code: actionOf(t, out).Language} },
			example: codeIn("ISO_639-1", "de"),
		},
		{
			name:    "ENTRY encoding",
			opt:     yieldAction("", optCodePhrase("IANA_character-sets", "no-such-charset", "US-ASCII"), ""),
			read:    func(t *testing.T, out any) heldCode { return heldCode{code: actionOf(t, out).Encoding} },
			example: codeIn("IANA_character-sets", "US-ASCII"),
		},
		{
			name: "ELEMENT null_flavour",
			opt:  optTemplate("ELEMENT", optSingle("null_flavour", optCodedText(terminology.ID, "433", "272"))),
			read: func(t *testing.T, out any) heldCode {
				return codedPresent(t, "ELEMENT.null_flavour", out.(*rm.Element).NullFlavour)
			},
			group:   terminology.NullFlavours,
			example: openehrCode("272"),
		},
		{
			name: "INTERVAL_EVENT math_function",
			opt: optTemplate("INTERVAL_EVENT", append([]string{optSingle("math_function", optCodedText(terminology.ID, other, "145"))},
				intervalEventAttrs...)...),
			read:    func(t *testing.T, out any) heldCode { return codedHeld(mathFunction(t, out)) },
			group:   terminology.EventMathFunction,
			example: openehrCode("145"),
		},
		{
			name: "INTERVAL_EVENT math_function, a C_CODE_PHRASE on the coded text as a whole",
			opt: optTemplate("INTERVAL_EVENT", append([]string{optSingle("math_function", optCodedTextWhole(terminology.ID, other, "145"))},
				intervalEventAttrs...)...),
			read:    func(t *testing.T, out any) heldCode { return codedHeld(mathFunction(t, out)) },
			group:   terminology.EventMathFunction,
			example: openehrCode("145"),
		},
		{
			name:    "ISM_TRANSITION current_state",
			opt:     yieldAction("", "", optCodedText(terminology.ID, other, "526")),
			read:    func(t *testing.T, out any) heldCode { return codedHeld(actionTransition(t, out).CurrentState) },
			group:   terminology.InstructionStates,
			example: openehrCode("526"),
		},
		{
			// The current state is initial, so the default is initiate (535),
			// which the OPT does not list.
			name: "ISM_TRANSITION transition",
			opt:  optISMTransition(optSingle("transition", optCodedText(terminology.ID, other, "540"))),
			read: func(t *testing.T, out any) heldCode {
				return codedPresent(t, "ISM_TRANSITION.transition", actionTransition(t, out).Transition)
			},
			group:   terminology.InstructionTransitions,
			example: openehrCode("540"),
		},
		{
			// The group has one code, the default.
			name: "PARTICIPATION function",
			opt:  contextParticipationsOPT(optSingle("function", optCodedText(terminology.ID, other, "253"))),
			read: func(t *testing.T, out any) heldCode {
				return codedHeld(codedFunction(t, onlyParticipation(t, out)))
			},
			group:   terminology.ParticipationFunction,
			example: openehrCode("253"),
		},
		{
			name:    "PARTICIPATION mode",
			opt:     modeOPT(optCodedText(terminology.ID, other, "216")),
			read:    modeOf,
			group:   terminology.ParticipationMode,
			example: openehrCode("216"),
		},
		{
			name:    "PARTY_RELATED relationship",
			opt:     partyRelatedSubject(optSingle("relationship", optCodedText(terminology.ID, other, "10"))),
			read:    func(t *testing.T, out any) heldCode { return codedHeld(subjectRelationship(t, out)) },
			group:   terminology.SubjectRelationship,
			example: openehrCode("10"),
		},
		{
			name: "TERM_MAPPING purpose",
			opt:  mappedValueOPT(optSingle("purpose", optCodedText(terminology.ID, other, "670")), mappingTarget),
			read: func(t *testing.T, out any) heldCode {
				ms := valueMappings(t, out)
				if len(ms) != 1 {
					t.Fatalf("%d mappings generated, want 1", len(ms))
				}
				return codedPresent(t, "TERM_MAPPING.purpose", ms[0].Purpose)
			},
			group:   terminology.TermMappingPurpose,
			example: openehrCode("670"),
		},
		{
			name: "DV_TEXT language",
			opt:  elementValueOPT("DV_TEXT", optSingle("language", optCodePhrase("ISO_639-1", "xx", "de"))),
			read: func(t *testing.T, out any) heldCode {
				return phraseHeld(t, "DV_TEXT.language", textRoot(t, out).Language)
			},
			example: codeIn("ISO_639-1", "de"),
		},
		{
			name: "DV_TEXT encoding",
			opt:  elementValueOPT("DV_TEXT", optSingle("encoding", optCodePhrase("IANA_character-sets", "no-such-charset", "US-ASCII"))),
			read: func(t *testing.T, out any) heldCode {
				return phraseHeld(t, "DV_TEXT.encoding", textRoot(t, out).Encoding)
			},
			example: codeIn("IANA_character-sets", "US-ASCII"),
		},
		{
			name: "DV_CODED_TEXT language",
			opt: elementValueOPT("DV_CODED_TEXT", optSingle("defining_code", optCodePhrase("SNOMED-CT", "248152002")),
				optSingle("language", optCodePhrase("ISO_639-1", "xx", "de"))),
			read: func(t *testing.T, out any) heldCode {
				return phraseHeld(t, "DV_CODED_TEXT.language", rootElementValue[*rm.DVCodedText](t, out).Language)
			},
			example: codeIn("ISO_639-1", "de"),
		},
		{
			// DV_ORDERED's normal status has no RM default.
			name: "DV_ORDERED normal_status",
			opt:  elementValueOPT("DV_COUNT", optSingle("normal_status", optCodePhrase("openehr_normal_statuses", "X", "N"))),
			read: func(t *testing.T, out any) heldCode {
				return phraseHeld(t, "DV_COUNT.normal_status", rootElementValue[*rm.DVCount](t, out).NormalStatus)
			},
			example: codeIn("openehr_normal_statuses", "N"),
		},
		{
			name:    "DV_MULTIMEDIA media_type",
			opt:     yieldMultimedia(optSingle("media_type", optCodePhrase("IANA_media-types", "no/such-type", "image/png"))),
			read:    func(t *testing.T, out any) heldCode { return heldCode{code: multimedia(t, out).MediaType} },
			example: codeIn("IANA_media-types", "image/png"),
		},
		{
			name: "DV_MULTIMEDIA compression_algorithm",
			opt:  yieldMultimedia(optSingle("compression_algorithm", optCodePhrase("openehr_compression_algorithms", "no-such-algorithm", "gzip"))),
			read: func(t *testing.T, out any) heldCode {
				return phraseHeld(t, "DV_MULTIMEDIA.compression_algorithm", multimedia(t, out).CompressionAlgorithm)
			},
			example: codeIn("openehr_compression_algorithms", "gzip"),
		},
		{
			name: "DV_MULTIMEDIA integrity_check_algorithm",
			opt:  yieldMultimedia(optSingle("integrity_check_algorithm", optCodePhrase("openehr_integrity_check_algorithms", "no-such-algorithm", "SHA-256"))),
			read: func(t *testing.T, out any) heldCode {
				return phraseHeld(t, "DV_MULTIMEDIA.integrity_check_algorithm", multimedia(t, out).IntegrityCheckAlgorithm)
			},
			example: codeIn("openehr_integrity_check_algorithms", "SHA-256"),
		},
		{
			name: "DV_ENCAPSULATED language, on a DV_MULTIMEDIA",
			opt:  yieldMultimedia(optSingle("language", optCodePhrase("ISO_639-1", "xx", "de"))),
			read: func(t *testing.T, out any) heldCode {
				return phraseHeld(t, "DV_MULTIMEDIA.language", multimedia(t, out).Language)
			},
			example: codeIn("ISO_639-1", "de"),
		},
		{
			name: "DV_ENCAPSULATED charset, on a DV_PARSABLE",
			opt:  elementValueOPT("DV_PARSABLE", optSingle("charset", optCodePhrase("IANA_character-sets", "no-such-charset", "US-ASCII"))),
			read: func(t *testing.T, out any) heldCode {
				return phraseHeld(t, "DV_PARSABLE.charset", parsable(t, out).Charset)
			},
			example: codeIn("IANA_character-sets", "US-ASCII"),
		},

		// The RM default wins over a member listed before it, and a member
		// the walk wrote stays.
		{
			name:    "PARTICIPATION mode, the default 193 listed after another member",
			opt:     modeOPT(optCodedText(terminology.ID, other, "216", "193")),
			read:    modeOf,
			group:   terminology.ParticipationMode,
			example: openehrCode("193"),
			random:  []rm.CodePhrase{openehrCode("216"), openehrCode("193")},
		},
		{
			name: "DV_TEXT language, the default en listed after another member",
			opt:  elementValueOPT("DV_TEXT", optSingle("language", optCodePhrase("ISO_639-1", "xx", "de", "en"))),
			read: func(t *testing.T, out any) heldCode {
				return phraseHeld(t, "DV_TEXT.language", textRoot(t, out).Language)
			},
			example: codeIn("ISO_639-1", "en"),
			random:  []rm.CodePhrase{codeIn("ISO_639-1", "de"), codeIn("ISO_639-1", "en")},
		},
		{
			name:    "PARTICIPATION mode, a member the walk wrote before the default",
			opt:     modeOPT(optCodedText(terminology.ID, "216", "193")),
			read:    modeOf,
			group:   terminology.ParticipationMode,
			example: openehrCode("216"),
			random:  []rm.CodePhrase{openehrCode("216"), openehrCode("193")},
		},

		// A terminology id and a code string the OPT lists apart: any
		// listed terminology goes with any listed code.
		{
			name:    "PARTICIPATION mode, terminologies local and openehr, codes at0001 and 193",
			opt:     modeOPT(optNode("DV_CODED_TEXT", "", optSingle("defining_code", optCodePhraseStrings([]string{"local", terminology.ID}, []string{"at0001", "193"})))),
			read:    modeOf,
			group:   terminology.ParticipationMode,
			example: openehrCode("193"),
		},
		{
			name:    "PARTICIPATION mode, terminologies local and openehr, codes at0001 and 216",
			opt:     modeOPT(optNode("DV_CODED_TEXT", "", optSingle("defining_code", optCodePhraseStrings([]string{"local", terminology.ID}, []string{"at0001", "216"})))),
			read:    modeOf,
			group:   terminology.ParticipationMode,
			example: openehrCode("216"),
		},

		// The OPT lists no member: the walk's code stays and breaks the rule.
		{
			name:       "PARTICIPATION mode, no member listed",
			opt:        modeOPT(optCodedText(terminology.ID, other, "272")),
			read:       modeOf,
			example:    openehrCode(other),
			random:     []rm.CodePhrase{openehrCode(other), openehrCode("272")},
			broken:     "PARTICIPATION.Mode_valid",
			brokenPath: "/context/participations[1]/mode/defining_code",
		},
		{
			name: "DV_TEXT language, no member listed",
			opt:  elementValueOPT("DV_TEXT", optSingle("language", optCodePhrase("ISO_639-1", "xx", "yy"))),
			read: func(t *testing.T, out any) heldCode {
				return phraseHeld(t, "DV_TEXT.language", textRoot(t, out).Language)
			},
			example:    codeIn("ISO_639-1", "xx"),
			random:     []rm.CodePhrase{codeIn("ISO_639-1", "xx"), codeIn("ISO_639-1", "yy")},
			broken:     "DV_TEXT.Language_valid",
			brokenPath: "/value/language",
		},
	}
	for _, tc := range cases {
		for _, implicit := range []bool{true, false} {
			c := compileOPTText(t, tc.opt, implicit)
			for _, policy := range everyPolicy {
				t.Run(fmt.Sprintf("%s/implicit=%t/%v/%v", tc.name, implicit, policy, instance.ExampleFill), func(t *testing.T) {
					checkInSet(t, tc, c, inSetOptions(policy, instance.ExampleFill, 0))
				})
				for seed := range uint64(randomSeeds) {
					t.Run(fmt.Sprintf("%s/implicit=%t/%v/%v/seed=%d", tc.name, implicit, policy, instance.RandomFill, seed), func(t *testing.T) {
						checkInSet(t, tc, c, inSetOptions(policy, instance.RandomFill, seed))
					})
				}
			}
		}
	}
}

// TestREQ107_TransitionLeadsIntoTheSettledCurrentState is the REQ-107 RM
// defaults check that an ISM_TRANSITION's transition the OPT names with no
// code is the one that leads into the current state as Precedence 4
// settles it: where the OPT lists a code outside the instruction states
// group before 245|active|, the current state is active and the transition
// 540|start|, not the 535|initiate| beside a state outside the group. It
// holds under every policy, ExampleFill and randomSeeds RandomFill sources,
// and both compile modes, and the root passes both validators.
func TestREQ107_TransitionLeadsIntoTheSettledCurrentState(t *testing.T) {
	opt := optISMTransition(optSingle("current_state", optCodedText(terminology.ID, "271", "245")),
		optSingle("transition", bareCodedText))
	for _, implicit := range []bool{true, false} {
		c := compileOPTText(t, opt, implicit)
		for _, policy := range everyPolicy {
			check := func(t *testing.T, opts instance.Options) {
				t.Helper()
				out, err := instance.Generate(t.Context(), c, opts)
				if err != nil {
					t.Fatalf("Generate: %v, want a root", err)
				}
				iv := actionTransition(t, out)
				checkOpenEHRCode(t, "ISM_TRANSITION.current_state", iv.CurrentState, terminology.InstructionStates, "245")
				checkTransition(t, iv, "540")
				checkValidBut(t, out, c)
			}
			t.Run(fmt.Sprintf("implicit=%t/%v/%v", implicit, policy, instance.ExampleFill), func(t *testing.T) {
				check(t, inSetOptions(policy, instance.ExampleFill, 0))
			})
			for seed := range uint64(randomSeeds) {
				t.Run(fmt.Sprintf("implicit=%t/%v/%v/seed=%d", implicit, policy, instance.RandomFill, seed), func(t *testing.T) {
					check(t, inSetOptions(policy, instance.RandomFill, seed))
				})
			}
		}
	}
}

// inSetOptions are the settings of a Precedence 4 case: policy, fill, a
// RandomFill source seeded by seed, and what a COMPOSITION root needs.
func inSetOptions(policy instance.Policy, fill instance.ValueFill, seed uint64) instance.Options {
	return instance.Options{
		Policy:      policy,
		ValueFill:   fill,
		ValueSource: mrand.NewPCG(seed, 2),
		Now:         defaultsNow,
		Language:    "en",
		Territory:   "NL",
		Composer:    testComposer(),
	}
}

// checkInSet generates tc under opts and checks the code the attribute
// holds, its text, and what the validators report.
func checkInSet(t *testing.T, tc inSetCase, c *templatecompile.Compiled, opts instance.Options) {
	t.Helper()
	out, err := instance.Generate(t.Context(), c, opts)
	if err != nil {
		t.Fatalf("Generate: %v, want a root", err)
	}
	got := tc.read(t, out)
	want := []rm.CodePhrase{tc.example}
	if opts.ValueFill == instance.RandomFill && len(tc.random) > 0 {
		want = tc.random
	}
	if !slices.Contains(want, got.code) {
		t.Errorf("%s holds %+v, want one of %+v", tc.name, got.code, want)
	}
	if tc.group != nil && tc.broken == "" {
		if rubric, _ := tc.group.Rubric(got.code.CodeString); got.text != rubric {
			t.Errorf("%s text = %q, want the rubric %q of %s in the group %q", tc.name, got.text, rubric, got.code.CodeString, tc.group.Name())
		}
	}
	checkOnlyBroken(t, "ValidateRM", validation.ValidateRM(out).Issues, tc.broken, tc.brokenPath)
	checkOnlyBroken(t, "template validator", templateErrors(out, c), tc.broken, tc.brokenPath)
}

// checkOnlyBroken fails t unless the errors among issues are exactly the
// code_not_in_value_set of rule at path, or none when rule is "".
func checkOnlyBroken(t *testing.T, validator string, issues []validation.Issue, rule, path string) {
	t.Helper()
	var seen int
	for _, iss := range issues {
		if iss.Severity != validation.Error {
			continue
		}
		if rule != "" && iss.Code == "code_not_in_value_set" && iss.Rule == rule && iss.Path == path {
			seen++
			continue
		}
		t.Errorf("%s: %s @ %s (%s): %s", validator, iss.Code, iss.Path, iss.Rule, iss.Detail)
	}
	want := 0
	if rule != "" {
		want = 1
	}
	if seen != want {
		t.Errorf("%s: %d code_not_in_value_set of %s @ %s, want %d", validator, seen, rule, path, want)
	}
}

// TestREQ107_PeriodicFormalismTakesHL7EIVLWhereTheOPTRejectsHL7PIVL is the
// REQ-107 Precedence 4 check on a DV_PERIODIC_TIME_SPECIFICATION's formalism,
// which RM Value_valid keeps to HL7:PIVL or HL7:EIVL: where the OPT's
// C_STRING lists foo and HL7:EIVL, the walk's foo gives way to HL7:EIVL,
// without a draw, since the OPT rejects the default HL7:PIVL, and so it does
// where a pattern admits foo or HL7:EIVL; where it lists foo alone, foo
// stays (REQ-107 § Exceptions and known gaps). The RM floor
// does not evaluate Value_valid, so the second root is not validated: a
// silent validator would prove nothing there. Each holds under every
// policy, ExampleFill and randomSeeds RandomFill sources, and both compile
// modes.
func TestREQ107_PeriodicFormalismTakesHL7EIVLWhereTheOPTRejectsHL7PIVL(t *testing.T) {
	cases := []struct {
		name, list, want string
		validate         bool
	}{
		{name: "foo or HL7:EIVL", list: "<list>foo</list><list>HL7:EIVL</list>", want: "HL7:EIVL", validate: true},
		{name: "foo alone", list: "<list>foo</list>", want: "foo"},
		{name: "a pattern admitting foo or HL7:EIVL", list: "<pattern>foo|HL7:EIVL</pattern>", want: "HL7:EIVL", validate: true},
	}
	for _, tc := range cases {
		value := optNode("DV_PERIODIC_TIME_SPECIFICATION", "", optSingle("value", optNode("DV_PARSABLE", "",
			optSingle("formalism", optPrimitive("STRING", "C_STRING", tc.list)))))
		for _, implicit := range []bool{true, false} {
			c := compileOPTText(t, optTemplate("ELEMENT", optSingle("value", value)), implicit)
			for _, policy := range everyPolicy {
				check := func(t *testing.T, opts instance.Options) {
					t.Helper()
					out, err := instance.Generate(t.Context(), c, opts)
					if err != nil {
						t.Fatalf("Generate: %v, want a root", err)
					}
					if got := dvValue[*rm.DVPeriodicTimeSpecification](t, out).Value.Formalism; got != tc.want {
						t.Errorf("DV_PERIODIC_TIME_SPECIFICATION.value.formalism = %q, want %q", got, tc.want)
					}
					if tc.validate {
						checkValid(t, out, c)
					}
				}
				t.Run(fmt.Sprintf("%s/implicit=%t/%v/%v", tc.name, implicit, policy, instance.ExampleFill), func(t *testing.T) {
					check(t, inSetOptions(policy, instance.ExampleFill, 0))
				})
				for seed := range uint64(randomSeeds) {
					t.Run(fmt.Sprintf("%s/implicit=%t/%v/%v/seed=%d", tc.name, implicit, policy, instance.RandomFill, seed), func(t *testing.T) {
						check(t, inSetOptions(policy, instance.RandomFill, seed))
					})
				}
			}
		}
	}
}
