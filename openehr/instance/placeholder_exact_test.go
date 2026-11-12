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

// TestREQ107_EHRURIValueNarrowedToTheEHRScheme is the REQ-107 check that
// DV_EHR_URI Scheme_valid narrows the value of a DV_EHR_URI whose OPT
// constrains it by a C_STRING (Precedence 3). The value the C_STRING path
// picks stays where its scheme is ehr: a list keeps its first member under
// ExampleFill, even beside the placeholder, and a listed member under
// RandomFill, and a pattern of the ehr scheme keeps its own pick or draw.
// The scheme is compared without regard to case, so a list holding only
// EHR://a keeps it. Where the pick has no ehr scheme, the first list
// member with the ehr scheme that the C_STRING accepts is written, so a
// list that names a value without it first gives its ehr member under
// either fill, and with no list the shortest string with the ehr scheme
// that the pattern builds and accepts, so a pattern naming another scheme
// first gives its ehr alternative. Else the placeholder ehr://example is
// written where the C_STRING admits it, as one with neither a list nor a
// pattern, one with an empty list, and the pattern .* do, whose pick
// example has no scheme. A pattern whose first non-empty build it refuses
// itself gets a longer one, not the placeholder it refuses. The root
// passes the RM floor and the template validator under every policy and
// both value fills.
func TestREQ107_EHRURIValueNarrowedToTheEHRScheme(t *testing.T) {
	cases := []struct {
		name string
		body string
		// example is the value under ExampleFill.
		example string
		// random holds the values the draw can give under RandomFill, or is
		// nil where any value with the ehr scheme that the C_STRING
		// accepts will do.
		random []string
	}{
		{name: "neither list nor pattern", body: "", example: "ehr://example", random: []string{"ehr://example"}},
		{name: "empty list", body: "<list></list>", example: "ehr://example", random: []string{"ehr://example"}},
		{name: "pattern whose pick has no scheme", body: "<pattern>.*</pattern>", example: "ehr://example", random: []string{"ehr://example"}},
		{name: "pattern of the ehr scheme", body: "<pattern>ehr://.+</pattern>", example: "ehr://a"},
		{
			name:    "list holding the placeholder second",
			body:    "<list>ehr://a</list><list>ehr://example</list>",
			example: "ehr://a",
			random:  []string{"ehr://a", "ehr://example"},
		},
		{name: "list refusing the placeholder", body: "<list>ehr://abc</list>", example: "ehr://abc", random: []string{"ehr://abc"}},
		{
			name:    "list naming a value without the ehr scheme first",
			body:    "<list>http://x</list><list>ehr://a</list>",
			example: "ehr://a",
			random:  []string{"ehr://a"},
		},
		{name: "list in the ehr scheme spelled in capitals", body: "<list>EHR://a</list>", example: "EHR://a", random: []string{"EHR://a"}},
		{name: "pattern naming another scheme first", body: "<pattern>http://x|ehr://a</pattern>", example: "ehr://a", random: []string{"ehr://a"}},
		{
			name:    "pattern refusing its first non-empty build",
			body:    `<pattern>(?:x?\B|ehr://a)?</pattern>`,
			example: "ehr://a",
			random:  []string{"ehr://a"},
		},
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
					value := rootElementValue[*rm.DVEHRURI](t, out)
					switch {
					case opts.ValueFill == instance.ExampleFill:
						if value.Value != tc.example {
							t.Errorf("DV_EHR_URI.value = %q, want %q", value.Value, tc.example)
						}
					case tc.random != nil:
						if !slices.Contains(tc.random, value.Value) {
							t.Errorf("DV_EHR_URI.value = %q, want one of %q", value.Value, tc.random)
						}
					default:
						if !strings.EqualFold(value.Scheme(), "ehr") {
							t.Errorf("DV_EHR_URI.value = %q, want the ehr scheme", value.Value)
						}
					}
					checkValid(t, out, c)
				})
			}
		}
	}
}

// TestREQ107_EHRURIValueWithoutTheEHRSchemeIsUnsatisfiable is the REQ-107
// check that a C_STRING on a DV_EHR_URI's value that admits no value with
// the ehr scheme Scheme_valid requires (Precedence 3), neither a list
// member nor the placeholder ehr://example, makes Generate return an error
// wrapping ErrConstraintUnsatisfiable, and no root, under every policy,
// both value fills and both compile modes. The error names the RM type,
// the attribute and the OPT path, wraps neither ErrSlotFillUnsupported nor
// ErrArchetypeIDMissing, and does not echo the values the constraint pins.
func TestREQ107_EHRURIValueWithoutTheEHRSchemeIsUnsatisfiable(t *testing.T) {
	cases := []struct {
		name string
		body string
		// rejected are the values of body the error must not echo.
		rejected []string
	}{
		{name: "list of one value", body: "<list>http://x</list>", rejected: []string{"http://x"}},
		{
			name:     "list of two values",
			body:     "<list>http://x</list><list>https://y</list>",
			rejected: []string{"http://x", "https://y"},
		},
		{name: "pattern of another scheme", body: "<pattern>http://.+</pattern>"},
	}
	for _, tc := range cases {
		for _, implicit := range []bool{false, true} {
			c := compileOPTText(t, optEHRURIElement(tc.body), implicit)
			path := stringLeafPath(t, c)
			for _, opts := range defaultsOptions() {
				t.Run(fmt.Sprintf("%s/implicit=%t/%v/%v", tc.name, implicit, opts.Policy, opts.ValueFill), func(t *testing.T) {
					out, err := instance.Generate(t.Context(), c, opts)
					if out != nil {
						t.Errorf("Generate returned %T, want no root", out)
					}
					if !errors.Is(err, instance.ErrConstraintUnsatisfiable) {
						t.Fatalf("Generate error = %v, want one wrapping ErrConstraintUnsatisfiable", err)
					}
					for _, other := range []error{instance.ErrSlotFillUnsupported, instance.ErrArchetypeIDMissing} {
						if errors.Is(err, other) {
							t.Errorf("Generate error = %v, want it not to wrap %v", err, other)
						}
					}
					for _, part := range []string{"DV_EHR_URI.value", path} {
						if !strings.Contains(err.Error(), part) {
							t.Errorf("Generate error = %q, want it to name %q", err, part)
						}
					}
					for _, value := range tc.rejected {
						if strings.Contains(err.Error(), value) {
							t.Errorf("Generate error = %q, want it not to echo the rejected value %q", err, value)
						}
					}
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
// each such leaf, since the OPT's own pattern refuses the placeholder: the
// RM needs the value present (REQ-107 Precedence 2), and the root breaks
// the OPT's pattern instead (REQ-107 Exceptions, the bullet on a pattern
// that admits only the empty string). A DV_TEXT value has no placeholder
// and keeps the empty string, which the RM floor reports.
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
