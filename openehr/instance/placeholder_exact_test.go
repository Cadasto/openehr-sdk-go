package instance_test

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// localPlaceholder is the code REQ-107 makes the generator write on a code
// phrase it has no OPT value for.
var localPlaceholder = rm.CodePhrase{
	CodeString:    "at0000",
	TerminologyID: rm.TerminologyID{Value: "local"},
}

// nestedTextValue is the text the generator writes on a nested DV_TEXT or
// DV_CODED_TEXT the OPT gives no value: the open-string example.
const nestedTextValue = "example"

// TestREQ107_RootPlaceholders is the REQ-107 check that a generation root
// gets the values a nested one gets: the URI ehr://example on a
// DV_EHR_URI, the code at0000 in terminology local on a CODE_PHRASE, that
// code and the nested text on a DV_CODED_TEXT, the nested text on a
// DV_TEXT, and the clock, in UTC as RFC 3339, on a DV_DATE_TIME. A code
// phrase whose OPT names a terminology keeps it. The root is compiled with
// the implicit attributes, whose String attributes are filled by the pass
// that writes the open-string example, and without them, when the root has
// no attribute to walk. Now is set east of UTC, so a value written in its
// own zone fails.
func TestREQ107_RootPlaceholders(t *testing.T) {
	now := time.Date(2021, 3, 4, 7, 6, 7, 0, time.FixedZone("UTC+2", 2*60*60))
	// The OPT names the terminology of a code phrase and no code.
	snomed := optSingle("terminology_id", optNode("TERMINOLOGY_ID", "",
		optSingle("value", optPrimitive("STRING", "C_STRING", "<list>SNOMED-CT</list>"))))
	cases := []struct {
		name  string
		root  string
		attrs []string
		check func(t *testing.T, out any)
	}{
		{
			name: "DV_EHR_URI value",
			root: "DV_EHR_URI",
			check: func(t *testing.T, out any) {
				if got := out.(*rm.DVEHRURI).Value; got != "ehr://example" {
					t.Errorf("DV_EHR_URI.value = %q, want %q", got, "ehr://example")
				}
			},
		},
		{
			name: "CODE_PHRASE code",
			root: "CODE_PHRASE",
			check: func(t *testing.T, out any) {
				if got := *out.(*rm.CodePhrase); got != localPlaceholder {
					t.Errorf("CODE_PHRASE = %+v, want %+v", got, localPlaceholder)
				}
			},
		},
		{
			name: "DV_CODED_TEXT code and text",
			root: "DV_CODED_TEXT",
			check: func(t *testing.T, out any) {
				ct := out.(*rm.DVCodedText)
				if ct.DefiningCode != localPlaceholder {
					t.Errorf("DV_CODED_TEXT.defining_code = %+v, want %+v", ct.DefiningCode, localPlaceholder)
				}
				if ct.Value != nestedTextValue {
					t.Errorf("DV_CODED_TEXT.value = %q, want %q", ct.Value, nestedTextValue)
				}
			},
		},
		{
			name: "DV_TEXT text",
			root: "DV_TEXT",
			check: func(t *testing.T, out any) {
				if got := out.(*rm.DVText).Value; got != nestedTextValue {
					t.Errorf("DV_TEXT.value = %q, want %q", got, nestedTextValue)
				}
			},
		},
		{
			name: "DV_DATE_TIME value from the clock",
			root: "DV_DATE_TIME",
			check: func(t *testing.T, out any) {
				if got, want := out.(*rm.DVDateTime).Value, now.UTC().Format(time.RFC3339); got != want {
					t.Errorf("DV_DATE_TIME.value = %q, want %q", got, want)
				}
			},
		},
		{
			name:  "CODE_PHRASE code under the terminology the OPT names",
			root:  "CODE_PHRASE",
			attrs: []string{snomed},
			check: func(t *testing.T, out any) {
				want := rm.CodePhrase{CodeString: "at0000", TerminologyID: rm.TerminologyID{Value: "SNOMED-CT"}}
				if got := *out.(*rm.CodePhrase); got != want {
					t.Errorf("CODE_PHRASE = %+v, want %+v", got, want)
				}
			},
		},
	}
	for _, tc := range cases {
		for _, implicit := range []bool{true, false} {
			c := compileOPTText(t, optTemplate(tc.root, tc.attrs...), implicit)
			for _, policy := range everyPolicy {
				t.Run(fmt.Sprintf("%s/implicit=%t/%v", tc.name, implicit, policy), func(t *testing.T) {
					out, err := instance.Generate(t.Context(), c, instance.Options{Policy: policy, Now: now})
					if err != nil {
						t.Fatalf("Generate: %v", err)
					}
					tc.check(t, out)
				})
			}
		}
	}
}

// TestREQ107_ExactPlaceholders is the REQ-107 check of the exact
// placeholders the generator writes where the OPT gives no value: the code
// at0000 in terminology local on a coded text, a code phrase, and an
// ordinal or scale symbol, and the URI ehr://example on a DV_EHR_URI. A
// C_CODE_PHRASE with an empty code list gives the code at0000 under the
// terminology it names, or under local. Each OPT is compiled with and
// without the implicit attributes, because the two take different paths
// through the generator, and generated under both value fills: a
// RandomFill draw from an empty code list keeps the named terminology, and
// a C_DV_ORDINAL that lists no ordinal gets the placeholder symbol.
func TestREQ107_ExactPlaceholders(t *testing.T) {
	cases := []struct {
		name  string
		opt   string
		check func(t *testing.T, out any)
	}{
		{
			name: "DV_CODED_TEXT under ELEMENT",
			opt:  optTemplate("ELEMENT", optSingle("value", optNode("DV_CODED_TEXT", ""))),
			check: func(t *testing.T, out any) {
				ct := rootElementValue[*rm.DVCodedText](t, out)
				if ct.DefiningCode != localPlaceholder {
					t.Errorf("DV_CODED_TEXT.defining_code = %+v, want %+v", ct.DefiningCode, localPlaceholder)
				}
			},
		},
		{
			name: "CODE_PHRASE under DV_CODED_TEXT",
			opt: optTemplate("ELEMENT", optSingle("value",
				optNode("DV_CODED_TEXT", "", optSingle("defining_code", optNode("CODE_PHRASE", ""))))),
			check: func(t *testing.T, out any) {
				ct := rootElementValue[*rm.DVCodedText](t, out)
				if ct.DefiningCode != localPlaceholder {
					t.Errorf("DV_CODED_TEXT.defining_code = %+v, want %+v", ct.DefiningCode, localPlaceholder)
				}
			},
		},
		{
			name: "C_CODE_PHRASE with an empty code list, terminology named",
			opt:  optTemplate("ELEMENT", optSingle("value", optCodedText("SNOMED-CT"))),
			check: func(t *testing.T, out any) {
				want := rm.CodePhrase{CodeString: "at0000", TerminologyID: rm.TerminologyID{Value: "SNOMED-CT"}}
				if got := rootElementValue[*rm.DVCodedText](t, out).DefiningCode; got != want {
					t.Errorf("DV_CODED_TEXT.defining_code = %+v, want %+v", got, want)
				}
			},
		},
		{
			name: "C_CODE_PHRASE with an empty code list, no terminology",
			opt:  optTemplate("ELEMENT", optSingle("value", optCodedText(""))),
			check: func(t *testing.T, out any) {
				if got := rootElementValue[*rm.DVCodedText](t, out).DefiningCode; got != localPlaceholder {
					t.Errorf("DV_CODED_TEXT.defining_code = %+v, want %+v", got, localPlaceholder)
				}
			},
		},
		{
			name: "DV_ORDINAL under ELEMENT",
			opt:  optTemplate("ELEMENT", optSingle("value", optNode("DV_ORDINAL", ""))),
			check: func(t *testing.T, out any) {
				ord := rootElementValue[*rm.DVOrdinal](t, out)
				if ord.Symbol.DefiningCode != localPlaceholder {
					t.Errorf("DV_ORDINAL.symbol.defining_code = %+v, want %+v", ord.Symbol.DefiningCode, localPlaceholder)
				}
			},
		},
		{
			// A C_DV_ORDINAL that lists no ordinal leaves nothing to draw
			// from, under RandomFill too, so the placeholder decides.
			name: "C_DV_ORDINAL with no ordinal listed",
			opt: optTemplate("ELEMENT", optSingle("value",
				`<children xsi:type="C_DV_ORDINAL"><rm_type_name>DV_ORDINAL</rm_type_name><node_id></node_id></children>`)),
			check: func(t *testing.T, out any) {
				ord := rootElementValue[*rm.DVOrdinal](t, out)
				if ord.Symbol.DefiningCode != localPlaceholder {
					t.Errorf("DV_ORDINAL.symbol.defining_code = %+v, want %+v", ord.Symbol.DefiningCode, localPlaceholder)
				}
			},
		},
		{
			name: "DV_SCALE under ELEMENT",
			opt:  optTemplate("ELEMENT", optSingle("value", optNode("DV_SCALE", ""))),
			check: func(t *testing.T, out any) {
				sc := rootElementValue[*rm.DVScale](t, out)
				if sc.Symbol.DefiningCode != localPlaceholder {
					t.Errorf("DV_SCALE.symbol.defining_code = %+v, want %+v", sc.Symbol.DefiningCode, localPlaceholder)
				}
			},
		},
		{
			name: "DV_ORDINAL root",
			opt:  optTemplate("DV_ORDINAL"),
			check: func(t *testing.T, out any) {
				if got := out.(*rm.DVOrdinal).Symbol.DefiningCode; got != localPlaceholder {
					t.Errorf("DV_ORDINAL.symbol.defining_code = %+v, want %+v", got, localPlaceholder)
				}
			},
		},
		{
			name: "DV_SCALE root",
			opt:  optTemplate("DV_SCALE"),
			check: func(t *testing.T, out any) {
				if got := out.(*rm.DVScale).Symbol.DefiningCode; got != localPlaceholder {
					t.Errorf("DV_SCALE.symbol.defining_code = %+v, want %+v", got, localPlaceholder)
				}
			},
		},
		{
			name: "DV_EHR_URI under ELEMENT",
			opt:  optTemplate("ELEMENT", optSingle("value", optNode("DV_EHR_URI", ""))),
			check: func(t *testing.T, out any) {
				if got := rootElementValue[*rm.DVEHRURI](t, out).Value; got != "ehr://example" {
					t.Errorf("DV_EHR_URI.value = %q, want %q", got, "ehr://example")
				}
			},
		},
	}
	for _, tc := range cases {
		for _, implicit := range []bool{false, true} {
			c := compileOPTText(t, tc.opt, implicit)
			for _, opts := range defaultsOptions() {
				t.Run(fmt.Sprintf("%s/implicit=%t/%v/%v", tc.name, implicit, opts.Policy, opts.ValueFill), func(t *testing.T) {
					out, err := instance.Generate(t.Context(), c, opts)
					if err != nil {
						t.Fatalf("Generate: %v", err)
					}
					tc.check(t, out)
				})
			}
		}
	}
}

// TestREQ107_NestedMediaTypeTerminology is the REQ-107 check that a
// DV_MULTIMEDIA media_type the OPT names as an unconstrained CODE_PHRASE
// gets the RM default text/plain under IANA_media-types, the terminology
// the template-instance writer gives a media type with none (wire.md § the
// DV_MULTIMEDIA coded attributes). The local terminology of the code
// phrase's placeholder must not survive. The OPT is compiled with the
// implicit attributes, the default.
func TestREQ107_NestedMediaTypeTerminology(t *testing.T) {
	c := compileOPTText(t, optTemplate("ELEMENT", optSingle("value",
		optNode("DV_MULTIMEDIA", "", optSingle("media_type", optNode("CODE_PHRASE", ""))))), true)
	want := rm.CodePhrase{CodeString: "text/plain", TerminologyID: rm.TerminologyID{Value: "IANA_media-types"}}
	for _, policy := range everyPolicy {
		t.Run(policy.String(), func(t *testing.T) {
			out, err := instance.Generate(t.Context(), c, instance.Options{Policy: policy, Now: defaultsNow})
			if err != nil {
				t.Fatalf("Generate: %v", err)
			}
			if got := rootElementValue[*rm.DVMultimedia](t, out).MediaType; got != want {
				t.Errorf("DV_MULTIMEDIA.media_type = %+v, want %+v", got, want)
			}
		})
	}
}

// rootElementValue returns the value of the generated root ELEMENT as a T, or
// stops the test when the root is not an ELEMENT holding a T.
func rootElementValue[T rm.DataValue](t *testing.T, out any) T {
	t.Helper()
	el, ok := out.(*rm.Element)
	if !ok {
		t.Fatalf("generated root is %T, want *rm.Element", out)
	}
	v, ok := el.Value.(T)
	if !ok {
		t.Fatalf("ELEMENT.value is %T, want %T", el.Value, *new(T))
	}
	return v
}

// optEHRURIElement is an OPT whose root ELEMENT holds a DV_EHR_URI whose
// value carries a C_STRING with body, such as a list or a pattern.
func optEHRURIElement(body string) string {
	return optTemplate("ELEMENT", optSingle("value",
		optNode("DV_EHR_URI", "", optSingle("value", optPrimitive("STRING", "C_STRING", body)))))
}

// TestREQ107_EHRURIPlaceholderUnderOpenCString is the REQ-107 check that a
// DV_EHR_URI whose value the OPT constrains by a C_STRING that admits the
// placeholder ehr://example gets that placeholder, not the open-string
// example, which DV_EHR_URI Scheme_valid refuses: a C_STRING with neither a
// list nor a pattern, one with an empty list, and one whose pattern admits
// the placeholder. A C_STRING that refuses the placeholder keeps the value
// it admits. The root passes the RM floor and the template validator under
// every policy and both value fills.
func TestREQ107_EHRURIPlaceholderUnderOpenCString(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{name: "neither list nor pattern", body: "", want: "ehr://example"},
		{name: "empty list", body: "<list></list>", want: "ehr://example"},
		{name: "pattern admitting the placeholder", body: "<pattern>ehr://.+</pattern>", want: "ehr://example"},
		{name: "list refusing the placeholder", body: "<list>ehr://abc</list>", want: "ehr://abc"},
	}
	for _, tc := range cases {
		for _, implicit := range []bool{false, true} {
			c := compileOPTText(t, optEHRURIElement(tc.body), implicit)
			for _, opts := range defaultsOptions() {
				t.Run(fmt.Sprintf("%s/implicit=%t/%v/%v", tc.name, implicit, opts.Policy, opts.ValueFill), func(t *testing.T) {
					out, err := instance.Generate(t.Context(), c, opts)
					if err != nil {
						t.Fatalf("Generate: %v", err)
					}
					if got := rootElementValue[*rm.DVEHRURI](t, out).Value; got != tc.want {
						t.Errorf("DV_EHR_URI.value = %q, want %q", got, tc.want)
					}
					checkValid(t, out, c)
				})
			}
		}
	}
}

// emptyOnlyPattern is a C_STRING whose pattern admits only the empty
// string, so the walk writes "" under it, under either value fill.
var emptyOnlyPattern = optPrimitive("STRING", "C_STRING", "<pattern>x{0}</pattern>")

// optEmptyOnlySymbol is an OPT whose root ELEMENT holds a DV_ORDINAL or a
// DV_SCALE, as rmType names, whose symbol carries emptyOnlyPattern on its
// text and on its defining_code's code_string.
func optEmptyOnlySymbol(rmType string) string {
	return optTemplate("ELEMENT", optSingle("value", optNode(rmType, "",
		optSingle("symbol", optNode("DV_CODED_TEXT", "",
			optSingle("value", emptyOnlyPattern),
			optSingle("defining_code", optNode("CODE_PHRASE", "", optSingle("code_string", emptyOnlyPattern))))))))
}

// TestREQ107_PlaceholderUnderEmptyOnlyPattern is the REQ-107 check of the
// placeholders the generator writes where the walk leaves a value empty,
// because the OPT puts a C_STRING whose pattern admits only the empty
// string, x{0}, on it: ehr://example on a DV_EHR_URI's value, and the code
// at0000 in local with the text example on a DV_ORDINAL's or DV_SCALE's
// symbol whose text and code_string carry it. The RM floor reports
// nothing. The template validator reports exactly the pattern mismatch at
// each such leaf, since the OPT's own pattern refuses the placeholder
// (REQ-107 Exceptions: an OPT constraint that admits no value an RM rule
// accepts).
func TestREQ107_PlaceholderUnderEmptyOnlyPattern(t *testing.T) {
	symbolMismatches := []string{
		"template primitive_pattern_mismatch pattern_mismatch @ /value/symbol/value",
		"template primitive_pattern_mismatch pattern_mismatch @ /value/symbol/defining_code/code_string",
	}
	checkSymbol := func(t *testing.T, name string, symbol rm.DVCodedText) {
		t.Helper()
		if symbol.DefiningCode != localPlaceholder || symbol.Value != nestedTextValue {
			t.Errorf("%s.symbol = %q %+v, want %q %+v", name, symbol.Value, symbol.DefiningCode, nestedTextValue, localPlaceholder)
		}
	}
	cases := []struct {
		name     string
		opt      string
		check    func(t *testing.T, out any)
		template []string
	}{
		{
			name: "DV_EHR_URI value",
			opt:  optEHRURIElement("<pattern>x{0}</pattern>"),
			check: func(t *testing.T, out any) {
				if got := rootElementValue[*rm.DVEHRURI](t, out).Value; got != "ehr://example" {
					t.Errorf("DV_EHR_URI.value = %q, want %q", got, "ehr://example")
				}
			},
			template: []string{"template primitive_pattern_mismatch pattern_mismatch @ /value/value"},
		},
		{
			name: "DV_ORDINAL symbol",
			opt:  optEmptyOnlySymbol("DV_ORDINAL"),
			check: func(t *testing.T, out any) {
				checkSymbol(t, "DV_ORDINAL", rootElementValue[*rm.DVOrdinal](t, out).Symbol)
			},
			template: symbolMismatches,
		},
		{
			name: "DV_SCALE symbol",
			opt:  optEmptyOnlySymbol("DV_SCALE"),
			check: func(t *testing.T, out any) {
				checkSymbol(t, "DV_SCALE", rootElementValue[*rm.DVScale](t, out).Symbol)
			},
			template: symbolMismatches,
		},
	}
	for _, tc := range cases {
		for _, implicit := range []bool{false, true} {
			c := compileOPTText(t, tc.opt, implicit)
			for _, opts := range defaultsOptions() {
				t.Run(fmt.Sprintf("%s/implicit=%t/%v/%v", tc.name, implicit, opts.Policy, opts.ValueFill), func(t *testing.T) {
					out, err := instance.Generate(t.Context(), c, opts)
					if err != nil {
						t.Fatalf("Generate: %v", err)
					}
					tc.check(t, out)
					for _, iss := range validation.ValidateRM(out).Issues {
						if iss.Severity == validation.Error {
							t.Errorf("ValidateRM: %s @ %s: %s", iss.Code, iss.Path, iss.Detail)
						}
					}
					var got []string
					for _, iss := range templateErrors(out, c) {
						got = append(got, fmt.Sprintf("%v %s %s @ %s", iss.Layer, iss.Code, iss.Rule, iss.Path))
					}
					if !slices.Equal(got, tc.template) {
						t.Errorf("Validate errors = %q, want %q", got, tc.template)
					}
				})
			}
		}
	}
}
