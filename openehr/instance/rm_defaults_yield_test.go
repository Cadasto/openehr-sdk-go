package instance_test

import (
	"fmt"
	"regexp"
	"testing"

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
			name: "ELEMENT null_flavour, prohibited",
			opt:  optTemplate("ELEMENT", optProhibitedSingle("null_flavour")),
			check: func(t *testing.T, out any) {
				if nf := out.(*rm.Element).NullFlavour; nf != nil {
					t.Errorf("ELEMENT.null_flavour = %+v, want none", nf)
				}
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
