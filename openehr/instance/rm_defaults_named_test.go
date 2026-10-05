package instance_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/terminology"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// optCodePhrase is a C_CODE_PHRASE under terminologyID, listing codes.
// With no codes the list is empty: the OPT names the terminology and no
// code.
func optCodePhrase(terminologyID string, codes ...string) string {
	var list strings.Builder
	for _, code := range codes {
		list.WriteString(`<code_list>` + code + `</code_list>`)
	}
	return `<children xsi:type="C_CODE_PHRASE"><rm_type_name>CODE_PHRASE</rm_type_name><node_id></node_id>` +
		`<terminology_id><value>` + terminologyID + `</value></terminology_id>` + list.String() + `</children>`
}

// Options the cases below expect the RM defaults to take their codes from.
const (
	namedLanguage  = "nl"
	namedTerritory = "BE"
)

// TestREQ107_RMDefaultsWhenOPTNamesNoCode is the REQ-107 check that an RM
// default applies where the OPT names an attribute without giving it a
// code: as a CODE_PHRASE or DV_CODED_TEXT node with no constraint, or as a
// C_CODE_PHRASE with an empty code list, whose placeholder code the RM
// default wins over. Each OPT is compiled with and without the implicit
// attributes, and generated under both policies and both value fills.
func TestREQ107_RMDefaultsWhenOPTNamesNoCode(t *testing.T) {
	// An ACTION names every attribute the RM floor needs, so the
	// generated value passes it with or without the implicit attributes.
	actionWith := func(language, encoding, currentState string) string {
		return optTemplate("ACTION",
			optSingle("language", language), optSingle("encoding", encoding), optSingle("subject"),
			optSingle("ism_transition", optNode("ISM_TRANSITION", "", optSingle("current_state", currentState))),
			optSingle("description", emptyTree))
	}
	barePhrase := optNode("CODE_PHRASE", "")
	bareText := optNode("DV_CODED_TEXT", "")
	cases := []struct {
		name  string
		opt   string
		check func(t *testing.T, out any)
	}{
		{
			name:  "ENTRY language and encoding, bare CODE_PHRASE",
			opt:   actionWith(barePhrase, barePhrase, bareText),
			check: checkEntryCodes,
		},
		{
			name:  "ENTRY language and encoding, empty-list C_CODE_PHRASE",
			opt:   actionWith(optCodePhrase("ISO_639-1"), optCodePhrase("IANA_character-sets"), bareText),
			check: checkEntryCodes,
		},
		{
			name:  "COMPOSITION language and territory, bare CODE_PHRASE",
			opt:   optTemplate("COMPOSITION", optSingle("language", barePhrase), optSingle("territory", barePhrase)),
			check: checkCompositionCodes,
		},
		{
			name: "COMPOSITION language and territory, empty-list C_CODE_PHRASE",
			opt: optTemplate("COMPOSITION",
				optSingle("language", optCodePhrase("ISO_639-1")),
				optSingle("territory", optCodePhrase("ISO_3166-1"))),
			check: checkCompositionCodes,
		},
		{
			name:  "COMPOSITION category, bare DV_CODED_TEXT",
			opt:   optTemplate("COMPOSITION", optSingle("category", bareText)),
			check: checkCategory,
		},
		{
			name:  "COMPOSITION category, empty-list C_CODE_PHRASE",
			opt:   optTemplate("COMPOSITION", optSingle("category", optCodedText(terminology.ID))),
			check: checkCategory,
		},
		{
			name:  "ISM_TRANSITION current_state, bare DV_CODED_TEXT",
			opt:   actionWith("", "", bareText),
			check: checkCurrentState,
		},
		{
			name:  "ISM_TRANSITION current_state, empty-list C_CODE_PHRASE",
			opt:   actionWith("", "", optCodedText(terminology.ID)),
			check: checkCurrentState,
		},
	}
	for _, tc := range cases {
		for _, implicit := range []bool{true, false} {
			c := compileOPTText(t, tc.opt, implicit)
			for _, policy := range []instance.Policy{instance.Minimal, instance.Example} {
				for _, fill := range []instance.ValueFill{instance.ExampleFill, instance.RandomFill} {
					t.Run(fmt.Sprintf("%s/implicit=%t/%v/%v", tc.name, implicit, policy, fill), func(t *testing.T) {
						out, err := instance.Generate(t.Context(), c, instance.Options{
							Policy:    policy,
							ValueFill: fill,
							Language:  namedLanguage,
							Territory: namedTerritory,
							Composer:  testComposer(),
							Now:       defaultsNow,
						})
						if err != nil {
							t.Fatalf("Generate: %v", err)
						}
						tc.check(t, out)
						for _, iss := range validation.ValidateRM(out).Issues {
							if iss.Severity == validation.Error {
								t.Errorf("ValidateRM: %s @ %s: %s", iss.Code, iss.Path, iss.Detail)
							}
						}
					})
				}
			}
		}
	}
}

func checkEntryCodes(t *testing.T, out any) {
	t.Helper()
	a := out.(*rm.Action)
	if want := (rm.CodePhrase{CodeString: namedLanguage, TerminologyID: rm.TerminologyID{Value: "ISO_639-1"}}); a.Language != want {
		t.Errorf("ACTION.language = %+v, want %+v", a.Language, want)
	}
	if want := (rm.CodePhrase{CodeString: "UTF-8", TerminologyID: rm.TerminologyID{Value: "IANA_character-sets"}}); a.Encoding != want {
		t.Errorf("ACTION.encoding = %+v, want %+v", a.Encoding, want)
	}
}

func checkCompositionCodes(t *testing.T, out any) {
	t.Helper()
	comp := out.(*rm.Composition)
	if want := (rm.CodePhrase{CodeString: namedLanguage, TerminologyID: rm.TerminologyID{Value: "ISO_639-1"}}); comp.Language != want {
		t.Errorf("COMPOSITION.language = %+v, want %+v", comp.Language, want)
	}
	if want := (rm.CodePhrase{CodeString: namedTerritory, TerminologyID: rm.TerminologyID{Value: "ISO_3166-1"}}); comp.Territory != want {
		t.Errorf("COMPOSITION.territory = %+v, want %+v", comp.Territory, want)
	}
}

func checkCategory(t *testing.T, out any) {
	t.Helper()
	checkOpenEHRCode(t, "COMPOSITION.category", out.(*rm.Composition).Category, terminology.CompositionCategory, "433")
}

func checkCurrentState(t *testing.T, out any) {
	t.Helper()
	checkOpenEHRCode(t, "ISM_TRANSITION.current_state", out.(*rm.Action).IsmTransition.CurrentState, terminology.InstructionStates, "524")
}

// checkOpenEHRCode checks that got is code of the openehr terminology with
// that code's rubric in group as its text.
func checkOpenEHRCode(t *testing.T, what string, got rm.DVCodedText, group *terminology.Group, code string) {
	t.Helper()
	if want := (rm.CodePhrase{CodeString: code, TerminologyID: rm.TerminologyID{Value: terminology.ID}}); got.DefiningCode != want {
		t.Errorf("%s.defining_code = %+v, want %+v", what, got.DefiningCode, want)
	}
	if rubric, _ := group.Rubric(code); got.Value != rubric {
		t.Errorf("%s.value = %q, want %q", what, got.Value, rubric)
	}
}

// TestREQ107_RMDefaultsKeepOPTCode is the REQ-107 check that an RM default
// does not replace a code the OPT gives: an ENTRY's language and encoding,
// a COMPOSITION's language and territory, and an ISM_TRANSITION's current
// state, each pinned by a C_CODE_PHRASE listing one code.
func TestREQ107_RMDefaultsKeepOPTCode(t *testing.T) {
	phrase := func(terminologyID, code string) rm.CodePhrase {
		return rm.CodePhrase{CodeString: code, TerminologyID: rm.TerminologyID{Value: terminologyID}}
	}
	action := optTemplate("ACTION",
		optSingle("language", optCodePhrase("ISO_639-1", "de")),
		optSingle("encoding", optCodePhrase("IANA_character-sets", "ISO-8859-1")),
		optSingle("subject"),
		optSingle("ism_transition", optNode("ISM_TRANSITION", "",
			optSingle("current_state", optCodedText(terminology.ID, "526")))),
		optSingle("description", emptyTree))
	composition := optTemplate("COMPOSITION",
		optSingle("language", optCodePhrase("ISO_639-1", "de")),
		optSingle("territory", optCodePhrase("ISO_3166-1", "DE")))
	cases := []struct {
		name string
		opt  string
		got  func(out any) rm.CodePhrase
		want rm.CodePhrase
	}{
		{"ENTRY language", action, func(out any) rm.CodePhrase { return out.(*rm.Action).Language }, phrase("ISO_639-1", "de")},
		{"ENTRY encoding", action, func(out any) rm.CodePhrase { return out.(*rm.Action).Encoding }, phrase("IANA_character-sets", "ISO-8859-1")},
		{
			"ISM_TRANSITION current_state", action,
			func(out any) rm.CodePhrase { return out.(*rm.Action).IsmTransition.CurrentState.DefiningCode },
			phrase(terminology.ID, "526"),
		},
		{"COMPOSITION language", composition, func(out any) rm.CodePhrase { return out.(*rm.Composition).Language }, phrase("ISO_639-1", "de")},
		{"COMPOSITION territory", composition, func(out any) rm.CodePhrase { return out.(*rm.Composition).Territory }, phrase("ISO_3166-1", "DE")},
	}
	for _, tc := range cases {
		c := compileOPTText(t, tc.opt, true)
		for _, policy := range []instance.Policy{instance.Minimal, instance.Example} {
			t.Run(tc.name+"/"+policy.String(), func(t *testing.T) {
				out, err := instance.Generate(t.Context(), c, instance.Options{
					Policy:    policy,
					Language:  namedLanguage,
					Territory: namedTerritory,
					Composer:  testComposer(),
					Now:       defaultsNow,
				})
				if err != nil {
					t.Fatalf("Generate: %v", err)
				}
				if got := tc.got(out); got != tc.want {
					t.Errorf("%s = %+v, want the OPT's %+v", tc.name, got, tc.want)
				}
			})
		}
	}
}
