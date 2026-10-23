package instance_test

import (
	"fmt"
	mrand "math/rand/v2"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/terminology"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// groupTiedText is a coded text a generated root carries on an attribute
// the RM ties to a group of the openEHR terminology, with that group.
type groupTiedText struct {
	path, class, attr string
	group             *terminology.Group
	text              rm.DVCodedText
}

// groupTiedTexts lists the coded texts out carries on the attributes the
// REQ-112 Coded invariants table pairs with a group of the openEHR
// terminology, spelled out here: a COMPOSITION's category, an
// EVENT_CONTEXT's setting, an ELEMENT's null_flavour, an INTERVAL_EVENT's
// math_function, an ISM_TRANSITION's current_state and transition, a
// PARTICIPATION's function, when it is coded, and mode, a PARTY_RELATED's
// relationship and a TERM_MAPPING's purpose.
func groupTiedTexts(out any) []groupTiedText {
	var found []groupTiedText
	add := func(path, class, attr string, group *terminology.Group, ct *rm.DVCodedText) {
		if ct != nil {
			found = append(found, groupTiedText{path: path + "/" + attr, class: class, attr: attr, group: group, text: *ct})
		}
	}
	visitRM(out, func(path string, v rm.Composition) {
		add(path, "COMPOSITION", "category", terminology.CompositionCategory, &v.Category)
	})
	visitRM(out, func(path string, v rm.EventContext) {
		add(path, "EVENT_CONTEXT", "setting", terminology.Setting, &v.Setting)
	})
	visitRM(out, func(path string, v rm.Element) {
		add(path, "ELEMENT", "null_flavour", terminology.NullFlavours, v.NullFlavour)
	})
	visitRM(out, func(path string, v rm.IntervalEvent[rm.ItemStructure]) {
		add(path, "INTERVAL_EVENT", "math_function", terminology.EventMathFunction, &v.MathFunction)
	})
	visitRM(out, func(path string, v rm.IsmTransition) {
		add(path, "ISM_TRANSITION", "current_state", terminology.InstructionStates, &v.CurrentState)
		add(path, "ISM_TRANSITION", "transition", terminology.InstructionTransitions, v.Transition)
	})
	visitRM(out, func(path string, v rm.Participation) {
		switch f := v.Function.(type) {
		case rm.DVCodedText:
			add(path, "PARTICIPATION", "function", terminology.ParticipationFunction, &f)
		case *rm.DVCodedText:
			add(path, "PARTICIPATION", "function", terminology.ParticipationFunction, f)
		}
		add(path, "PARTICIPATION", "mode", terminology.ParticipationMode, v.Mode)
	})
	visitRM(out, func(path string, v rm.PartyRelated) {
		add(path, "PARTY_RELATED", "relationship", terminology.SubjectRelationship, &v.Relationship)
	})
	visitRM(out, func(path string, v rm.TermMapping) {
		add(path, "TERM_MAPPING", "purpose", terminology.TermMappingPurpose, v.Purpose)
	})
	return found
}

// TestREQ107_CorpusGroupTiedCodesTakeTheirRubric is the REQ-107 RM
// defaults check, over every vendored template, that each openEHR code the
// generator writes on an attribute the RM ties to a group of the openEHR
// terminology carries as its text that code's rubric in that group, under
// every policy, both value fills and several fixed seeds, whether the OPT
// pins the code or an RM default writes it. No vendored OPT limits such a
// text with a C_STRING that rejects the rubric. Demonstration.v1 pins an
// INTERVAL_EVENT's math_function to openehr::147, whose rubric in the event
// math function group is "change".
func TestREQ107_CorpusGroupTiedCodesTakeTheirRubric(t *testing.T) {
	checked := make(map[string]int)
	eachCorpusRun(t, func(run corpusRun) {
		for _, tied := range groupTiedTexts(run.out) {
			code := tied.text.DefiningCode
			if code.TerminologyID.Value != terminology.ID {
				continue
			}
			rubric, ok := tied.group.Rubric(code.CodeString)
			if !ok {
				continue
			}
			checked[tied.class+"."+tied.attr]++
			if tied.text.Value != rubric {
				t.Errorf("%s: %s.%s openehr::%s at %s has text %q, want its rubric %q in the group %q",
					run, tied.class, tied.attr, code.CodeString, tied.path, tied.text.Value, rubric, tied.group.Name())
			}
		}
	})
	if checked["INTERVAL_EVENT.math_function"] == 0 {
		t.Fatal("no INTERVAL_EVENT math_function with an openEHR code generated, so the pinned code was not checked")
	}
	t.Logf("openEHR codes checked against their group rubric, by attribute: %v", checked)
}

// externalCode reports whether a coded text's defining code is in a
// terminology other than openehr and local and carries a code.
func externalCode(c rm.DVCodedText) bool {
	tid := c.DefiningCode.TerminologyID.Value
	return tid != "" && tid != terminology.ID && tid != "local" && c.DefiningCode.CodeString != ""
}

// externalTermKey is the key an OPT's term definitions give a code of a
// terminology other than local: <terminology id>::<code string>.
func externalTermKey(c rm.DVCodedText) string {
	return c.DefiningCode.TerminologyID.Value + "::" + c.DefiningCode.CodeString
}

// TestREQ107_CorpusExternalCodedTextsTakeTheTermText is the REQ-107 RM
// defaults check, over every vendored template, that each DV_CODED_TEXT
// the generator writes in a terminology other than openehr and local,
// whose code its own archetype, the nearest enclosing archetype root,
// defines under <terminology id>::<code string>, takes that term's text
// rather than the open-string example, and that a code its own archetype
// does not define takes no other archetype's text for it, under every
// policy, both value fills and several fixed seeds. family_history.v.1.2.3
// codes its EVALUATION's sex and relationship elements in SNOMED-CT and
// defines their texts (SNOMED-CT::248152002 Female). No vendored OPT limits
// such a text with a C_STRING that rejects its term.
func TestREQ107_CorpusExternalCodedTextsTakeTheTermText(t *testing.T) {
	checked, foreign := 0, 0
	terms := make(map[string]archetypeTerms)
	eachCorpusRun(t, func(run corpusRun) {
		byArchetype, ok := terms[run.name]
		if !ok {
			byArchetype = templateTerms(t, run.raw)
			terms[run.name] = byArchetype
		}
		visitRMIn(run.out, func(path, archetype string, c rm.DVCodedText) {
			if !externalCode(c) || archetype == "" {
				return
			}
			key := externalTermKey(c)
			if texts, ok := byArchetype[archetype][key]; ok {
				checked++
				if !slices.Contains(texts, c.Value) {
					t.Errorf("%s: DV_CODED_TEXT %s at %s has text %q, want the term text %s defines, one of %q",
						run, key, path, c.Value, archetype, texts)
				}
				return
			}
			for other, byCode := range byArchetype {
				if texts := byCode[key]; other != archetype && slices.Contains(texts, c.Value) {
					foreign++
					t.Errorf("%s: DV_CODED_TEXT %s at %s in %s, which defines no such term, has the text %q that %s defines",
						run, key, path, archetype, c.Value, other)
				}
			}
		})
	})
	if checked == 0 {
		t.Fatal("no DV_CODED_TEXT of an external terminology with a term of its own archetype generated, so nothing was checked")
	}
	t.Logf("%d external coded texts checked against their own archetype's term, %d took another archetype's", checked, foreign)
}

// TestREQ107_DemonstrationMathFunctionTakesItsRubric is the REQ-107 RM
// defaults check on the coded text the live run names: Demonstration.v1
// pins the math_function of its INTERVAL_EVENT at0034 to openehr::147, and
// the generator writes that code's rubric in the event math function
// group, "change", not the open-string example, under Large and Medium,
// both value fills and several seeds.
func TestREQ107_DemonstrationMathFunctionTakesItsRubric(t *testing.T) {
	c := compileFixture(t, "Demonstration.v1")
	for _, policy := range largeAndMedium {
		for _, fill := range []instance.ValueFill{instance.ExampleFill, instance.RandomFill} {
			for seed := range uint64(4) {
				opts := instance.Options{
					Policy: policy, ValueFill: fill, ValueSource: mrand.NewPCG(seed, corpusStream),
					Territory: "NL", Composer: testComposer(), Now: defaultsNow,
				}
				out, err := instance.Generate(t.Context(), c, opts)
				if err != nil {
					t.Fatalf("%v/%v/seed=%d: Generate: %v", policy, fill, seed, err)
				}
				found := 0
				visitRM(out, func(path string, ev rm.IntervalEvent[rm.ItemStructure]) {
					if ev.ArchetypeNodeID != "at0034" {
						return
					}
					found++
					want := rm.CodePhrase{CodeString: "147", TerminologyID: rm.TerminologyID{Value: terminology.ID}}
					if mf := ev.MathFunction; mf.DefiningCode != want || mf.Value != "change" {
						t.Errorf("%v/%v/seed=%d: math_function at %s = %q %+v, want \"change\" openehr::147",
							policy, fill, seed, path, mf.Value, mf.DefiningCode)
					}
				})
				if found == 0 {
					t.Fatalf("%v/%v/seed=%d: Demonstration.v1 generated no INTERVAL_EVENT at0034", policy, fill, seed)
				}
			}
		}
	}
}

// TestREQ107_FamilyHistoryExternalCodesTakeTheTermText is the REQ-107 RM
// defaults check on the coded texts the live run names in
// family_history.v.1.2.3: the value of the EVALUATION's sex element at0060,
// coded SNOMED-CT (248152002 Female), and of its relationship element
// at0016 (444301002 Mother, …), each take the text the OPT's term
// definitions give their code under SNOMED-CT::<code>, under Large and
// Medium, both value fills and several seeds.
func TestREQ107_FamilyHistoryExternalCodesTakeTheTermText(t *testing.T) {
	const archetype = "openEHR-EHR-EVALUATION.family_history.v2"
	c := compileFixture(t, "family_history.v.1.2.3")
	raw := readCorpusOPT(t, "family_history.v.1.2.3")
	defined := templateTerms(t, raw)[archetype]
	if texts := defined["SNOMED-CT::248152002"]; !slices.Equal(texts, []string{"Female"}) {
		t.Fatalf("%s defines SNOMED-CT::248152002 as %q, want [Female]", archetype, texts)
	}
	for _, policy := range largeAndMedium {
		for _, fill := range []instance.ValueFill{instance.ExampleFill, instance.RandomFill} {
			for seed := range uint64(4) {
				opts := instance.Options{
					Policy: policy, ValueFill: fill, ValueSource: mrand.NewPCG(seed, corpusStream),
					Territory: "NL", Composer: testComposer(), Now: defaultsNow,
				}
				out, err := instance.Generate(t.Context(), c, opts)
				if err != nil {
					t.Fatalf("%v/%v/seed=%d: Generate: %v", policy, fill, seed, err)
				}
				found := make(map[string]int)
				visitRMIn(out, func(path, in string, e rm.Element) {
					node := e.ArchetypeNodeID
					if in != archetype || (node != "at0060" && node != "at0016") {
						return
					}
					ct, ok := e.Value.(*rm.DVCodedText)
					if !ok || ct.DefiningCode.TerminologyID.Value != "SNOMED-CT" {
						return
					}
					found[node]++
					key := externalTermKey(*ct)
					if texts := defined[key]; len(texts) == 0 || !slices.Contains(texts, ct.Value) {
						t.Errorf("%v/%v/seed=%d: ELEMENT %s value at %s = %q %s, want the term text, one of %q",
							policy, fill, seed, node, path, ct.Value, key, texts)
					}
				})
				for _, node := range []string{"at0060", "at0016"} {
					if found[node] == 0 {
						t.Errorf("%v/%v/seed=%d: no ELEMENT %s coded in SNOMED-CT generated", policy, fill, seed, node)
					}
				}
			}
		}
	}
}

// readCorpusOPT reads the vendored OPT name.
func readCorpusOPT(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(fixtures.TemplateOptForName(name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return raw
}

// optISMTransition is an ACTION whose ISM_TRANSITION has attrs.
func optISMTransition(attrs ...string) string {
	return optTemplate("ACTION", optSingle("language"), optSingle("encoding"), optSingle("subject"),
		optSingle("ism_transition", optNode("ISM_TRANSITION", "", attrs...)),
		optSingle("description", emptyTree))
}

// optCodedTextWhole is a DV_CODED_TEXT that a C_CODE_PHRASE constrains as a
// whole, listing codes of terminologyID.
func optCodedTextWhole(terminologyID string, codes ...string) string {
	var list strings.Builder
	for _, code := range codes {
		list.WriteString(`<code_list>` + code + `</code_list>`)
	}
	return `<children xsi:type="C_CODE_PHRASE"><rm_type_name>DV_CODED_TEXT</rm_type_name><node_id></node_id>` +
		`<terminology_id><value>` + terminologyID + `</value></terminology_id>` + list.String() + `</children>`
}

// mathFunction is the math_function of a generated INTERVAL_EVENT root.
func mathFunction(t *testing.T, out any) rm.DVCodedText {
	t.Helper()
	ev, ok := out.(*rm.IntervalEvent[rm.ItemStructure])
	if !ok {
		t.Fatalf("generated root is %T, want *rm.IntervalEvent[rm.ItemStructure]", out)
	}
	return ev.MathFunction
}

// TestREQ107_PinnedOpenEHRCodeTakesItsGroupRubric is the REQ-107 RM
// defaults check that an openEHR code the OPT pins on an attribute the RM
// ties to a group of the openEHR terminology takes as its text that code's
// rubric in that group: an INTERVAL_EVENT's math_function, as a
// DV_CODED_TEXT with a code constraint and as one a C_CODE_PHRASE
// constrains as a whole; an EVENT_CONTEXT's setting; an ISM_TRANSITION's
// current_state and transition; a PARTICIPATION's function and mode, in an
// EVENT_CONTEXT's participations; and a TERM_MAPPING's purpose, in a
// DV_CODED_TEXT's mappings. Code 532 is "complete" in the version lifecycle
// state group and "completed" in the instruction states group, so an
// ISM_TRANSITION's current_state takes "completed". A code the attribute's
// group has no rubric for, such as 433, the event of the composition
// category group, on a math_function, keeps the text the walk wrote, and so
// does a code whose text the OPT limits with a C_STRING that rejects the
// rubric (Precedence 1). It holds under every policy and both value fills,
// with and without the implicit attributes.
func TestREQ107_PinnedOpenEHRCodeTakesItsGroupRubric(t *testing.T) {
	cases := []struct {
		name, code, text string
		opt              string
		coded            func(t *testing.T, out any) rm.DVCodedText
		valid            bool
	}{
		{
			name: "INTERVAL_EVENT math_function", code: "147", text: "change", valid: true,
			opt:   optTemplate("INTERVAL_EVENT", append([]string{optSingle("math_function", optCodedText(terminology.ID, "147"))}, intervalEventAttrs...)...),
			coded: mathFunction,
		},
		{
			name: "INTERVAL_EVENT math_function a C_CODE_PHRASE constrains as a whole", code: "147", text: "change", valid: true,
			opt:   optTemplate("INTERVAL_EVENT", append([]string{optSingle("math_function", optCodedTextWhole(terminology.ID, "147"))}, intervalEventAttrs...)...),
			coded: mathFunction,
		},
		{
			name: "INTERVAL_EVENT math_function whose code its group has no rubric for", code: "433", text: walkText,
			opt:   optTemplate("INTERVAL_EVENT", append([]string{optSingle("math_function", optCodedText(terminology.ID, "433"))}, intervalEventAttrs...)...),
			coded: mathFunction,
		},
		{
			name: "INTERVAL_EVENT math_function whose C_STRING rejects the rubric", code: "147", text: "Delta", valid: true,
			opt:   optTemplate("INTERVAL_EVENT", append([]string{optSingle("math_function", optCodedTextListing("Delta", terminology.ID, "147"))}, intervalEventAttrs...)...),
			coded: mathFunction,
		},
		{
			name: "EVENT_CONTEXT setting", code: "228", text: "primary medical care", valid: true,
			opt: optTemplate("COMPOSITION", optSingle("context", optNode("EVENT_CONTEXT", "",
				optSingle("setting", optCodedText(terminology.ID, "228"))))),
			coded: func(t *testing.T, out any) rm.DVCodedText { return generatedComposition(t, out).Context.Setting },
		},
		{
			name: "EVENT_CONTEXT setting whose C_STRING rejects the rubric", code: "228", text: "GP", valid: true,
			opt: optTemplate("COMPOSITION", optSingle("context", optNode("EVENT_CONTEXT", "",
				optSingle("setting", optCodedTextListing("GP", terminology.ID, "228"))))),
			coded: func(t *testing.T, out any) rm.DVCodedText { return generatedComposition(t, out).Context.Setting },
		},
		{
			name: "ISM_TRANSITION current_state", code: "532", text: "completed", valid: true,
			opt:   optISMTransition(optSingle("current_state", optCodedText(terminology.ID, "532"))),
			coded: func(t *testing.T, out any) rm.DVCodedText { return out.(*rm.Action).IsmTransition.CurrentState },
		},
		{
			name: "ISM_TRANSITION transition", code: "540", text: "start", valid: true,
			opt: optISMTransition(optSingle("current_state", optCodedText(terminology.ID, "245")),
				optSingle("transition", optCodedText(terminology.ID, "540"))),
			coded: func(t *testing.T, out any) rm.DVCodedText {
				t.Helper()
				iv := actionTransition(t, out)
				if iv.Transition == nil {
					t.Fatal("ISM_TRANSITION.transition absent, want the code the OPT pins")
				}
				return *iv.Transition
			},
		},
		{
			name: "PARTICIPATION function", code: "253", text: "unknown", valid: true,
			opt:   contextParticipationsOPT(optSingle("function", optCodedText(terminology.ID, "253"))),
			coded: func(t *testing.T, out any) rm.DVCodedText { return codedFunction(t, onlyParticipation(t, out)) },
		},
		{
			name: "PARTICIPATION mode", code: "216", text: "face-to-face communication", valid: true,
			opt: contextParticipationsOPT(optSingle("function", optNode("DV_TEXT", "")),
				optSingle("mode", optCodedText(terminology.ID, "216"))),
			coded: func(t *testing.T, out any) rm.DVCodedText {
				t.Helper()
				p := onlyParticipation(t, out)
				if p.Mode == nil {
					t.Fatal("PARTICIPATION.mode absent, want the code the OPT pins")
				}
				return *p.Mode
			},
		},
		{
			name: "TERM_MAPPING purpose", code: "671", text: "research study", valid: true,
			opt: mappedValueOPT(optSingle("purpose", optCodedText(terminology.ID, "671")), mappingTarget),
			coded: func(t *testing.T, out any) rm.DVCodedText {
				t.Helper()
				ms := valueMappings(t, out)
				if len(ms) != 1 || ms[0].Purpose == nil {
					t.Fatalf("ELEMENT value mappings = %+v, want one with the purpose the OPT pins", ms)
				}
				return *ms[0].Purpose
			},
		},
	}
	for _, tc := range cases {
		want := rm.CodePhrase{CodeString: tc.code, TerminologyID: rm.TerminologyID{Value: terminology.ID}}
		for _, implicit := range []bool{true, false} {
			c := compileOPTText(t, tc.opt, implicit)
			for _, opts := range defaultsOptions() {
				opts.Territory, opts.Composer = "NL", testComposer()
				t.Run(fmt.Sprintf("%s/implicit=%t/%v/%v", tc.name, implicit, opts.Policy, opts.ValueFill), func(t *testing.T) {
					out, err := instance.Generate(t.Context(), c, opts)
					if err != nil {
						t.Fatalf("Generate: %v", err)
					}
					got := tc.coded(t, out)
					if got.DefiningCode != want || got.Value != tc.text {
						t.Errorf("%s = %q %+v, want %q openehr::%s", tc.name, got.Value, got.DefiningCode, tc.text, tc.code)
					}
					if tc.valid {
						checkValid(t, out, c)
					}
				})
			}
		}
	}
}

// Texts the synthetic OPTs below give SNOMED-CT::248152002.
const (
	femaleTerm  = "Female"
	innerFemale = "Female (inner)"
)

// snomedFemale is the term definition of SNOMED-CT::248152002 with text,
// as an OPT keys it.
func snomedFemale(text string) string {
	return `<term_definitions code="SNOMED-CT::248152002"><items id="text">` + text + `</items></term_definitions>`
}

// nestedExternalTermOPT is a CLUSTER template whose archetype defines
// SNOMED-CT::248152002 as femaleTerm, holding an inner CLUSTER archetype
// that defines at0001 and, where innerText is not "",
// SNOMED-CT::248152002 as innerText. The inner archetype holds an ELEMENT
// at0001 whose value is coded SNOMED-CT::248152002.
func nestedExternalTermOPT(innerText string) string {
	innerTerms := `<term_definitions code="at0001"><items id="text">Inner element</items></term_definitions>`
	if innerText != "" {
		innerTerms += snomedFemale(innerText)
	}
	element := optNode("ELEMENT", "at0001", optSingle("value", optCodedText("SNOMED-CT", "248152002")))
	inner := optArchetypeRoot("CLUSTER", "openEHR-EHR-CLUSTER.inner.v1", innerTerms, optMultiple("items", element))
	return optTemplate("CLUSTER", snomedFemale(femaleTerm), optMultiple("items", inner))
}

// snomedCodedText returns the one DV_CODED_TEXT coded SNOMED-CT::code in a
// generated root.
func snomedCodedText(code string) func(t *testing.T, out any) rm.DVCodedText {
	return func(t *testing.T, out any) rm.DVCodedText {
		t.Helper()
		var found []rm.DVCodedText
		visitRM(out, func(_ string, c rm.DVCodedText) {
			if c.DefiningCode.TerminologyID.Value == "SNOMED-CT" && c.DefiningCode.CodeString == code {
				found = append(found, c)
			}
		})
		if len(found) != 1 {
			t.Fatalf("%d DV_CODED_TEXT coded SNOMED-CT::%s generated, want 1", len(found), code)
		}
		return found[0]
	}
}

// TestREQ107_ExternalCodedTextTakesTheTermText is the REQ-107 RM defaults
// check that a DV_CODED_TEXT whose code the OPT gives in a terminology
// other than openehr and local takes the text of the term its own
// archetype, the nearest enclosing archetype root, defines under
// <terminology id>::<code string>, as a DV_CODED_TEXT with a code
// constraint and as one a C_CODE_PHRASE constrains as a whole. Where the
// OPT limits the text with a C_STRING that rejects the term text, the text
// the walk wrote stays (Precedence 1); a code its own archetype does not
// define keeps the walk's text, even where an enclosing archetype defines
// it, and so does a code the archetype defines only under its bare code
// string, the key of a local code. It holds under every policy and both
// value fills, in every language asked for.
func TestREQ107_ExternalCodedTextTakesTheTermText(t *testing.T) {
	cases := []struct {
		name  string
		opt   string
		coded func(t *testing.T, out any) rm.DVCodedText
		want  string
	}{
		{
			name:  "ELEMENT value",
			opt:   optTemplate("ELEMENT", snomedFemale(femaleTerm), optSingle("value", optCodedText("SNOMED-CT", "248152002"))),
			coded: elementCodedText,
			want:  femaleTerm,
		},
		{
			name:  "ELEMENT value a C_CODE_PHRASE constrains as a whole",
			opt:   optTemplate("ELEMENT", snomedFemale(femaleTerm), optSingle("value", optCodedTextWhole("SNOMED-CT", "248152002"))),
			coded: elementCodedText,
			want:  femaleTerm,
		},
		{
			name: "ELEMENT value whose C_STRING rejects the term text",
			opt: optTemplate("ELEMENT", snomedFemale(femaleTerm), optSingle("value",
				optCodedTextListing("Woman", "SNOMED-CT", "248152002"))),
			coded: elementCodedText,
			want:  "Woman",
		},
		{
			name:  "ELEMENT value whose code has no term",
			opt:   optTemplate("ELEMENT", snomedFemale(femaleTerm), optSingle("value", optCodedText("SNOMED-CT", "248153007"))),
			coded: elementCodedText,
			want:  walkText,
		},
		{
			name: "ELEMENT value whose code the archetype defines under its bare code string",
			opt: optTemplate("ELEMENT", `<term_definitions code="248152002"><items id="text">Bare</items></term_definitions>`,
				optSingle("value", optCodedText("SNOMED-CT", "248152002"))),
			coded: elementCodedText,
			want:  walkText,
		},
		{
			name:  "ELEMENT value whose code only an enclosing archetype defines",
			opt:   nestedExternalTermOPT(""),
			coded: snomedCodedText("248152002"),
			want:  walkText,
		},
		{
			name:  "ELEMENT value whose code its own and an enclosing archetype define",
			opt:   nestedExternalTermOPT(innerFemale),
			coded: snomedCodedText("248152002"),
			want:  innerFemale,
		},
	}
	for _, tc := range cases {
		c := compileOPTText(t, tc.opt, true)
		for _, language := range []string{"", "en", "nl"} {
			for _, opts := range defaultsOptions() {
				opts.Language = language
				t.Run(fmt.Sprintf("%s/lang=%q/%v/%v", tc.name, language, opts.Policy, opts.ValueFill), func(t *testing.T) {
					out, err := instance.Generate(t.Context(), c, opts)
					if err != nil {
						t.Fatalf("Generate: %v", err)
					}
					if got := tc.coded(t, out); got.Value != tc.want {
						t.Errorf("%s = %q %+v, want text %q", tc.name, got.Value, got.DefiningCode, tc.want)
					}
					checkValid(t, out, c)
				})
			}
		}
	}
}
