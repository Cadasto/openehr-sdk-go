package instance_test

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/terminology"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// localAt0000 is what the walk writes for a code phrase the OPT constrains
// to terminology local with an empty code list.
var localAt0000 = rm.CodePhrase{CodeString: "at0000", TerminologyID: rm.TerminologyID{Value: "local"}}

// yieldAction is an ACTION template that names every attribute the RM
// floor needs, with the given children for language, encoding and
// ISM_TRANSITION.current_state; "" names the attribute with no child.
func yieldAction(language, encoding, currentState string) string {
	return optTemplate("ACTION",
		optSingle("language", language), optSingle("encoding", encoding), optSingle("subject"),
		optSingle("ism_transition", optNode("ISM_TRANSITION", "", optSingle("current_state", currentState))),
		optSingle("description", emptyTree))
}

// yieldMultimedia is an ELEMENT template whose value is a DV_MULTIMEDIA
// with attrs.
func yieldMultimedia(attrs ...string) string {
	return optTemplate("ELEMENT", optSingle("value", optNode("DV_MULTIMEDIA", "", attrs...)))
}

// optStringAttr is a C_SINGLE_ATTRIBUTE called name whose child is a
// C_STRING with body (a <list> or a <pattern>).
func optStringAttr(name, body string) string {
	return optSingle(name, optPrimitive("STRING", "C_STRING", body))
}

// optTerminologyIDValue is a CODE_PHRASE terminology_id attribute whose
// TERMINOLOGY_ID value the OPT constrains with a C_STRING listing id.
func optTerminologyIDValue(id string) string {
	return optSingle("terminology_id", optNode("TERMINOLOGY_ID", "", optStringAttr("value", "<list>"+id+"</list>")))
}

// openehrAt0000 is what the walk writes for a code phrase the OPT
// constrains to terminology openehr with an empty code list.
var openehrAt0000 = rm.CodePhrase{CodeString: "at0000", TerminologyID: rm.TerminologyID{Value: terminology.ID}}

// codedTextWithValue is a DV_CODED_TEXT whose text the OPT constrains with
// a C_STRING listing texts, and whose defining_code it constrains to
// terminology openehr with an empty code list, so the code has no value.
func codedTextWithValue(texts ...string) string {
	var list strings.Builder
	for _, text := range texts {
		list.WriteString("<list>" + text + "</list>")
	}
	return optNode("DV_CODED_TEXT", "", optStringAttr("value", list.String()),
		optSingle("defining_code", optCodePhrase(terminology.ID)))
}

// checkCodedText fails t unless got carries the text and the code the walk
// wrote.
func checkCodedText(t *testing.T, what string, got rm.DVCodedText, text string, code rm.CodePhrase) {
	t.Helper()
	if got.Value != text || got.DefiningCode != code {
		t.Errorf("%s = %q %+v, want the walk's %q %+v", what, got.Value, got.DefiningCode, text, code)
	}
}

func checkCode(t *testing.T, what string, got, want rm.CodePhrase) {
	t.Helper()
	if got != want {
		t.Errorf("%s = %+v, want %+v", what, got, want)
	}
}

func generatedComposition(t *testing.T, out any) *rm.Composition {
	t.Helper()
	comp, err := instance.AsComposition(out)
	if err != nil {
		t.Fatalf("AsComposition: %v", err)
	}
	return comp
}

// templateErrors returns every error the template validator reports on
// out: ValidateComposition for a COMPOSITION root, Validate otherwise.
func templateErrors(out any, c *templatecompile.Compiled) []validation.Issue {
	var res validation.Result
	if comp, ok := out.(*rm.Composition); ok {
		res = validation.ValidateComposition(comp, c)
	} else {
		res = validation.Validate(out, c)
	}
	var errs []validation.Issue
	for _, iss := range res.Issues {
		if iss.Severity == validation.Error {
			errs = append(errs, iss)
		}
	}
	return errs
}

// TestREQ107_RMDefaultsYieldToTheOPT is the REQ-107 check that every RM
// default yields to the OPT. Where the OPT's own constraint on the
// attribute rejects the default, through a C_CODE_PHRASE that names
// another terminology with an empty code list, a C_STRING on a code
// phrase's code_string or terminology id, or an existence of 0..0, the
// generator keeps what the walk wrote, which that constraint admits: the
// at0000 placeholder under the OPT's terminology, or nothing. The template
// validator then finds no error. Where the OPT's constraint admits the
// default, the default is written; the rows marked admit check that for
// the cases other tests do not cover. Each row runs under both policies,
// both value fills and both compile modes.
func TestREQ107_RMDefaultsYieldToTheOPT(t *testing.T) {
	setting := func(out any) rm.DVCodedText {
		comp := generatedComposition(t, out)
		if comp.Context == nil {
			t.Fatal("COMPOSITION.context is absent, want an EVENT_CONTEXT")
		}
		return comp.Context.Setting
	}
	atPattern := regexp.MustCompile(`^at[0-9]{4}$`)
	cases := []struct {
		name string
		opt  string
		// validatorBlind marks a row the template validator cannot check:
		// it reports rm_type_mismatch on a code phrase's terminology_id
		// whatever the value, because it reads that attribute as a string.
		validatorBlind bool
		check          func(t *testing.T, out any)
	}{
		{
			name: "ENTRY language, C_CODE_PHRASE local",
			opt:  yieldAction(optCodePhrase("local"), "", ""),
			check: func(t *testing.T, out any) {
				checkCode(t, "ACTION.language", out.(*rm.Action).Language, localAt0000)
			},
		},
		{
			name: "ENTRY encoding, C_CODE_PHRASE local",
			opt:  yieldAction("", optCodePhrase("local"), ""),
			check: func(t *testing.T, out any) {
				checkCode(t, "ACTION.encoding", out.(*rm.Action).Encoding, localAt0000)
			},
		},
		{
			name: "ISM_TRANSITION current_state, C_CODE_PHRASE local",
			opt:  yieldAction("", "", optCodedText("local")),
			check: func(t *testing.T, out any) {
				checkCode(t, "ISM_TRANSITION.current_state", out.(*rm.Action).IsmTransition.CurrentState.DefiningCode, localAt0000)
			},
		},
		{
			name: "COMPOSITION category, C_CODE_PHRASE local",
			opt:  optTemplate("COMPOSITION", optSingle("category", optCodedText("local"))),
			check: func(t *testing.T, out any) {
				checkCode(t, "COMPOSITION.category", generatedComposition(t, out).Category.DefiningCode, localAt0000)
			},
		},
		{
			name: "COMPOSITION language, C_CODE_PHRASE local",
			opt:  optTemplate("COMPOSITION", optSingle("language", optCodePhrase("local"))),
			check: func(t *testing.T, out any) {
				checkCode(t, "COMPOSITION.language", generatedComposition(t, out).Language, localAt0000)
			},
		},
		{
			name: "COMPOSITION territory, C_CODE_PHRASE local",
			opt:  optTemplate("COMPOSITION", optSingle("territory", optCodePhrase("local"))),
			check: func(t *testing.T, out any) {
				checkCode(t, "COMPOSITION.territory", generatedComposition(t, out).Territory, localAt0000)
			},
		},
		{
			name: "COMPOSITION context, prohibited",
			opt:  optTemplate("COMPOSITION", optProhibitedSingle("context")),
			check: func(t *testing.T, out any) {
				if ctx := generatedComposition(t, out).Context; ctx != nil {
					t.Errorf("COMPOSITION.context = %+v, want none", ctx)
				}
			},
		},
		{
			name: "EVENT_CONTEXT setting, C_CODE_PHRASE local",
			opt:  optTemplate("COMPOSITION", optSingle("context", optNode("EVENT_CONTEXT", "", optSingle("setting", optCodedText("local"))))),
			check: func(t *testing.T, out any) {
				checkCode(t, "EVENT_CONTEXT.setting", setting(out).DefiningCode, localAt0000)
			},
		},
		{
			name: "admit: EVENT_CONTEXT setting, C_CODE_PHRASE openehr",
			opt:  optTemplate("COMPOSITION", optSingle("context", optNode("EVENT_CONTEXT", "", optSingle("setting", optCodedText(terminology.ID))))),
			check: func(t *testing.T, out any) {
				checkOpenEHRCode(t, "EVENT_CONTEXT.setting", setting(out), terminology.Setting, "238")
			},
		},
		{
			// The walk writes the non-member 999 under ExampleFill, and
			// either code under RandomFill; the OPT admits 238, so the
			// default replaces a non-member.
			name: "admit: EVENT_CONTEXT setting, C_CODE_PHRASE openehr 999 or 238",
			opt: optTemplate("COMPOSITION", optSingle("context", optNode("EVENT_CONTEXT", "",
				optSingle("setting", optCodedText(terminology.ID, "999", "238"))))),
			check: func(t *testing.T, out any) {
				checkOpenEHRCode(t, "EVENT_CONTEXT.setting", setting(out), terminology.Setting, "238")
			},
		},
		{
			// With its null flavour prohibited, the ELEMENT takes a value
			// instead (TestREQ107_ProhibitedNullFlavourTakesAValue).
			name: "ELEMENT null_flavour, prohibited",
			opt:  optTemplate("ELEMENT", optProhibitedSingle("null_flavour")),
			check: func(t *testing.T, out any) {
				el := out.(*rm.Element)
				if el.NullFlavour != nil {
					t.Errorf("ELEMENT.null_flavour = %+v, want none", el.NullFlavour)
				}
				if el.Value == nil || rm.IsTypedNil(el.Value) {
					t.Errorf("ELEMENT.value absent, want the value an ELEMENT takes in place of a prohibited null flavour")
				}
			},
		},
		{
			name: "ELEMENT null_flavour, C_CODE_PHRASE local",
			opt:  optTemplate("ELEMENT", optSingle("null_flavour", optCodedText("local"))),
			check: func(t *testing.T, out any) {
				el := out.(*rm.Element)
				if el.NullFlavour == nil {
					t.Fatal("ELEMENT.null_flavour absent, want the walk's local::at0000")
				}
				checkCode(t, "ELEMENT.null_flavour", el.NullFlavour.DefiningCode, localAt0000)
			},
		},
		{
			name: "INTERVAL_EVENT math_function, C_CODE_PHRASE local",
			opt: optTemplate("INTERVAL_EVENT", append([]string{optSingle("math_function", optCodedText("local"))},
				intervalEventAttrs...)...),
			check: func(t *testing.T, out any) {
				checkCode(t, "INTERVAL_EVENT.math_function", out.(*rm.IntervalEvent[rm.ItemStructure]).MathFunction.DefiningCode, localAt0000)
			},
		},
		// The rows marked RM wins prohibit an attribute the BMM marks
		// mandatory: the RM rule wins over the prohibition, and the
		// generator writes the attribute as it writes a silent one.
		{
			name: "RM wins: EVENT_CONTEXT start_time, prohibited",
			opt:  optTemplate("COMPOSITION", optSingle("context", optNode("EVENT_CONTEXT", "", optProhibitedSingle("start_time")))),
			check: func(t *testing.T, out any) {
				if got, want := generatedComposition(t, out).Context.StartTime.Value, defaultsNow.Format(time.RFC3339); got != want {
					t.Errorf("EVENT_CONTEXT.start_time = %q, want the clock %q", got, want)
				}
			},
		},
		{
			name: "RM wins: ACTION time, prohibited",
			opt: optTemplate("ACTION", optSingle("language"), optSingle("encoding"), optSingle("subject"),
				optSingle("ism_transition", optNode("ISM_TRANSITION", "")), optSingle("description", emptyTree),
				optProhibitedSingle("time")),
			check: func(t *testing.T, out any) {
				if got, want := out.(*rm.Action).Time.Value, defaultsNow.Format(time.RFC3339); got != want {
					t.Errorf("ACTION.time = %q, want the clock %q", got, want)
				}
			},
		},
		{
			name: "RM wins: ENTRY language, prohibited",
			opt: optTemplate("ACTION", optProhibitedSingle("language"), optSingle("encoding"), optSingle("subject"),
				optSingle("ism_transition", optNode("ISM_TRANSITION", "")), optSingle("description", emptyTree)),
			check: func(t *testing.T, out any) {
				checkCode(t, "ACTION.language", out.(*rm.Action).Language, rm.CodePhrase{CodeString: "en", TerminologyID: rm.TerminologyID{Value: "ISO_639-1"}})
			},
		},
		{
			name: "RM wins: ENTRY encoding, prohibited",
			opt: optTemplate("ACTION", optSingle("language"), optProhibitedSingle("encoding"), optSingle("subject"),
				optSingle("ism_transition", optNode("ISM_TRANSITION", "")), optSingle("description", emptyTree)),
			check: func(t *testing.T, out any) {
				checkCode(t, "ACTION.encoding", out.(*rm.Action).Encoding, rm.CodePhrase{CodeString: "UTF-8", TerminologyID: rm.TerminologyID{Value: "IANA_character-sets"}})
			},
		},
		{
			name: "RM wins: ISM_TRANSITION current_state, prohibited",
			opt: optTemplate("ACTION", optSingle("language"), optSingle("encoding"), optSingle("subject"),
				optSingle("ism_transition", optNode("ISM_TRANSITION", "", optProhibitedSingle("current_state"))),
				optSingle("description", emptyTree)),
			check: func(t *testing.T, out any) {
				checkOpenEHRCode(t, "ISM_TRANSITION.current_state", out.(*rm.Action).IsmTransition.CurrentState, terminology.InstructionStates, "524")
			},
		},
		{
			name: "RM wins: COMPOSITION category, prohibited",
			opt:  optTemplate("COMPOSITION", optProhibitedSingle("category")),
			check: func(t *testing.T, out any) {
				checkOpenEHRCode(t, "COMPOSITION.category", generatedComposition(t, out).Category, terminology.CompositionCategory, "433")
			},
		},
		{
			name: "RM wins: COMPOSITION language, prohibited",
			opt:  optTemplate("COMPOSITION", optProhibitedSingle("language")),
			check: func(t *testing.T, out any) {
				checkCode(t, "COMPOSITION.language", generatedComposition(t, out).Language, rm.CodePhrase{CodeString: "en", TerminologyID: rm.TerminologyID{Value: "ISO_639-1"}})
			},
		},
		{
			name: "RM wins: COMPOSITION territory, prohibited",
			opt:  optTemplate("COMPOSITION", optProhibitedSingle("territory")),
			check: func(t *testing.T, out any) {
				checkCode(t, "COMPOSITION.territory", generatedComposition(t, out).Territory, rm.CodePhrase{CodeString: "NL", TerminologyID: rm.TerminologyID{Value: "ISO_3166-1"}})
			},
		},
		{
			name: "RM wins: COMPOSITION composer, prohibited",
			opt:  optTemplate("COMPOSITION", optProhibitedSingle("composer")),
			check: func(t *testing.T, out any) {
				if p, ok := generatedComposition(t, out).Composer.(*rm.PartyIdentified); !ok || p.Name == nil || *p.Name != *testComposer().Name {
					t.Errorf("COMPOSITION.composer = %#v, want the Options composer", generatedComposition(t, out).Composer)
				}
			},
		},
		{
			name: "RM wins: EVENT_CONTEXT setting, prohibited",
			opt:  optTemplate("COMPOSITION", optSingle("context", optNode("EVENT_CONTEXT", "", optProhibitedSingle("setting")))),
			check: func(t *testing.T, out any) {
				checkOpenEHRCode(t, "EVENT_CONTEXT.setting", setting(out), terminology.Setting, "238")
			},
		},
		{
			name: "RM wins: INTERVAL_EVENT math_function, prohibited",
			opt: optTemplate("INTERVAL_EVENT", append([]string{optProhibitedSingle("math_function")},
				intervalEventAttrs...)...),
			check: func(t *testing.T, out any) {
				checkOpenEHRCode(t, "INTERVAL_EVENT.math_function", out.(*rm.IntervalEvent[rm.ItemStructure]).MathFunction, terminology.EventMathFunction, "146")
			},
		},
		{
			name: "RM wins: DV_MULTIMEDIA media_type, prohibited",
			opt:  yieldMultimedia(optProhibitedSingle("media_type")),
			check: func(t *testing.T, out any) {
				want := rm.CodePhrase{CodeString: "text/plain", TerminologyID: rm.TerminologyID{Value: "IANA_media-types"}}
				checkCode(t, "DV_MULTIMEDIA.media_type", rootElementValue[*rm.DVMultimedia](t, out).MediaType, want)
			},
		},
		{
			name: "RM wins: HISTORY origin, prohibited",
			opt:  optTemplate("OBSERVATION", optSingle("data", optNode("HISTORY", "at0001", optProhibitedSingle("origin")))),
			check: func(t *testing.T, out any) {
				if got, want := out.(*rm.Observation).Data.Origin.Value, defaultsNow.Format(time.RFC3339); got != want {
					t.Errorf("HISTORY.origin = %q, want the clock %q", got, want)
				}
			},
		},
		{
			name: "RM wins: DV_TEXT value, prohibited",
			opt:  optTemplate("ELEMENT", optSingle("value", optNode("DV_TEXT", "", optProhibitedSingle("value")))),
			check: func(t *testing.T, out any) {
				if got := rootElementValue[*rm.DVText](t, out).Value; got == "" {
					t.Errorf("DV_TEXT.value is empty, want the silent default")
				}
			},
		},
		{
			// value and null_flavour are both prohibited: the RM rule that
			// an ELEMENT carry one of them wins, with the null flavour.
			name: "RM wins: ELEMENT value and null_flavour, prohibited",
			opt:  optTemplate("ELEMENT", optProhibitedSingle("null_flavour"), optProhibitedSingle("value")),
			check: func(t *testing.T, out any) {
				el := out.(*rm.Element)
				if el.Value != nil {
					t.Errorf("ELEMENT.value = %#v, want none", el.Value)
				}
				if el.NullFlavour == nil {
					t.Fatal("ELEMENT.null_flavour absent, want openehr::271")
				}
				checkOpenEHRCode(t, "ELEMENT.null_flavour", *el.NullFlavour, terminology.NullFlavours, "271")
			},
		},
		{
			name: "COMPOSITION category, value C_STRING visit",
			opt:  optTemplate("COMPOSITION", optSingle("category", codedTextWithValue("visit"))),
			check: func(t *testing.T, out any) {
				checkCodedText(t, "COMPOSITION.category", generatedComposition(t, out).Category, "visit", openehrAt0000)
			},
		},
		{
			name: "EVENT_CONTEXT setting, value C_STRING home",
			opt:  optTemplate("COMPOSITION", optSingle("context", optNode("EVENT_CONTEXT", "", optSingle("setting", codedTextWithValue("home"))))),
			check: func(t *testing.T, out any) {
				checkCodedText(t, "EVENT_CONTEXT.setting", setting(out), "home", openehrAt0000)
			},
		},
		{
			name: "admit: EVENT_CONTEXT setting, value C_STRING home or other care",
			opt: optTemplate("COMPOSITION", optSingle("context", optNode("EVENT_CONTEXT", "",
				optSingle("setting", codedTextWithValue("home", "other care"))))),
			check: func(t *testing.T, out any) {
				checkOpenEHRCode(t, "EVENT_CONTEXT.setting", setting(out), terminology.Setting, "238")
			},
		},
		{
			name: "ISM_TRANSITION current_state, value C_STRING planned",
			opt:  yieldAction("", "", codedTextWithValue("planned")),
			check: func(t *testing.T, out any) {
				checkCodedText(t, "ISM_TRANSITION.current_state", out.(*rm.Action).IsmTransition.CurrentState, "planned", openehrAt0000)
			},
		},
		{
			name: "INTERVAL_EVENT math_function, value C_STRING maximum",
			opt: optTemplate("INTERVAL_EVENT", append([]string{optSingle("math_function", codedTextWithValue("maximum"))},
				intervalEventAttrs...)...),
			check: func(t *testing.T, out any) {
				checkCodedText(t, "INTERVAL_EVENT.math_function", out.(*rm.IntervalEvent[rm.ItemStructure]).MathFunction, "maximum", openehrAt0000)
			},
		},
		{
			name: "ELEMENT null_flavour, value C_STRING unknown",
			opt:  optTemplate("ELEMENT", optSingle("null_flavour", codedTextWithValue("unknown"))),
			check: func(t *testing.T, out any) {
				el := out.(*rm.Element)
				if el.NullFlavour == nil {
					t.Fatal("ELEMENT.null_flavour absent, want the walk's value")
				}
				checkCodedText(t, "ELEMENT.null_flavour", *el.NullFlavour, "unknown", openehrAt0000)
			},
		},
		{
			name: "DV_MULTIMEDIA media_type, C_CODE_PHRASE openEHR",
			opt:  yieldMultimedia(optSingle("media_type", optCodePhrase("openEHR"))),
			check: func(t *testing.T, out any) {
				want := rm.CodePhrase{CodeString: "at0000", TerminologyID: rm.TerminologyID{Value: "openEHR"}}
				checkCode(t, "DV_MULTIMEDIA.media_type", rootElementValue[*rm.DVMultimedia](t, out).MediaType, want)
			},
		},
		{
			name:           "DV_MULTIMEDIA media_type, terminology id C_STRING openEHR",
			opt:            yieldMultimedia(optSingle("media_type", optNode("CODE_PHRASE", "", optTerminologyIDValue("openEHR")))),
			validatorBlind: true,
			check: func(t *testing.T, out any) {
				want := rm.CodePhrase{CodeString: "at0000", TerminologyID: rm.TerminologyID{Value: "openEHR"}}
				checkCode(t, "DV_MULTIMEDIA.media_type", rootElementValue[*rm.DVMultimedia](t, out).MediaType, want)
			},
		},
		{
			name:           "admit: DV_MULTIMEDIA media_type, terminology id C_STRING IANA_media-types",
			opt:            yieldMultimedia(optSingle("media_type", optNode("CODE_PHRASE", "", optTerminologyIDValue("IANA_media-types")))),
			validatorBlind: true,
			check: func(t *testing.T, out any) {
				want := rm.CodePhrase{CodeString: "text/plain", TerminologyID: rm.TerminologyID{Value: "IANA_media-types"}}
				checkCode(t, "DV_MULTIMEDIA.media_type", rootElementValue[*rm.DVMultimedia](t, out).MediaType, want)
			},
		},
		{
			name: "DV_MULTIMEDIA media_type, code_string C_STRING at-code pattern",
			opt:  yieldMultimedia(optSingle("media_type", optNode("CODE_PHRASE", "", optStringAttr("code_string", "<pattern>at[0-9]{4}</pattern>")))),
			check: func(t *testing.T, out any) {
				if got := rootElementValue[*rm.DVMultimedia](t, out).MediaType.CodeString; !atPattern.MatchString(got) {
					t.Errorf("DV_MULTIMEDIA.media_type code = %q, want a code the at-code pattern accepts", got)
				}
			},
		},
		{
			name: "admit: DV_MULTIMEDIA media_type, code_string C_STRING at0000 or text/plain",
			opt: yieldMultimedia(optSingle("media_type", optNode("CODE_PHRASE", "",
				optStringAttr("code_string", "<list>at0000</list><list>text/plain</list>")))),
			check: func(t *testing.T, out any) {
				if got := rootElementValue[*rm.DVMultimedia](t, out).MediaType.CodeString; got != "text/plain" {
					t.Errorf("DV_MULTIMEDIA.media_type code = %q, want the default text/plain", got)
				}
			},
		},
		{
			name: "DV_MULTIMEDIA uri, prohibited",
			opt:  yieldMultimedia(optProhibitedSingle("uri")),
			check: func(t *testing.T, out any) {
				if uri := rootElementValue[*rm.DVMultimedia](t, out).URI; uri != nil {
					t.Errorf("DV_MULTIMEDIA.uri = %#v, want none", uri)
				}
			},
		},
	}
	for _, tc := range cases {
		for _, implicit := range []bool{true, false} {
			c := compileOPTText(t, tc.opt, implicit)
			for _, opts := range defaultsOptions() {
				opts.Language, opts.Territory, opts.Composer = "en", "NL", testComposer()
				t.Run(fmt.Sprintf("%s/implicit=%t/%v/%v", tc.name, implicit, opts.Policy, opts.ValueFill), func(t *testing.T) {
					out, err := instance.Generate(t.Context(), c, opts)
					if err != nil {
						t.Fatalf("Generate: %v", err)
					}
					tc.check(t, out)
					if tc.validatorBlind {
						return
					}
					for _, iss := range templateErrors(out, c) {
						t.Errorf("template validator: %s @ %s: %s", iss.Code, iss.Path, iss.Detail)
					}
				})
			}
		}
	}
}
