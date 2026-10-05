package instance_test

import (
	"fmt"
	mrand "math/rand/v2"
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/terminology"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// mathFunctionVariants are the ways an OPT can give an INTERVAL_EVENT's
// math_function no value: leave it silent, or name it without a code, as an
// attribute with no child, as a DV_CODED_TEXT with no constraint, or as a
// C_CODE_PHRASE with an empty code list.
var mathFunctionVariants = []struct {
	name  string
	attrs []string
}{
	{name: "silent"},
	{name: "named with no child", attrs: []string{optSingle("math_function")}},
	{name: "bare DV_CODED_TEXT", attrs: []string{optSingle("math_function", optNode("DV_CODED_TEXT", ""))}},
	{name: "empty-list C_CODE_PHRASE", attrs: []string{optSingle("math_function", optCodedText(terminology.ID))}},
}

// intervalEventAttrs are the INTERVAL_EVENT attributes the RM requires
// besides math_function. The OPT names them, so the event passes the RM
// floor with or without the implicit attributes.
var intervalEventAttrs = []string{
	optSingle("time", optNode("DV_DATE_TIME", "")),
	optSingle("width", optNode("DV_DURATION", "")),
	optSingle("data", optNode("ITEM_TREE", "at0003")),
}

// eventPlacements put an INTERVAL_EVENT at the template root or in a
// HISTORY's events. event returns the generated one.
var eventPlacements = []struct {
	name  string
	opt   func(attrs []string) string
	event func(t *testing.T, out any) *rm.IntervalEvent[rm.ItemStructure]
}{
	{
		name: "root",
		opt: func(attrs []string) string {
			return optTemplate("INTERVAL_EVENT", append(attrs, intervalEventAttrs...)...)
		},
		event: func(t *testing.T, out any) *rm.IntervalEvent[rm.ItemStructure] {
			t.Helper()
			ev, ok := out.(*rm.IntervalEvent[rm.ItemStructure])
			if !ok {
				t.Fatalf("generated root is %T, want *rm.IntervalEvent[rm.ItemStructure]", out)
			}
			return ev
		},
	},
	{
		name: "HISTORY event",
		opt: func(attrs []string) string {
			return optTemplate("HISTORY",
				optSingle("origin", optNode("DV_DATE_TIME", "")),
				optMultiple("events", optNode("INTERVAL_EVENT", "at0002", append(attrs, intervalEventAttrs...)...)))
		},
		event: func(t *testing.T, out any) *rm.IntervalEvent[rm.ItemStructure] {
			t.Helper()
			h, ok := out.(*rm.History[rm.ItemStructure])
			if !ok {
				t.Fatalf("generated root is %T, want *rm.History[rm.ItemStructure]", out)
			}
			if len(h.Events) != 1 {
				t.Fatalf("HISTORY.events has %d members, want 1", len(h.Events))
			}
			ev, ok := h.Events[0].(*rm.IntervalEvent[rm.ItemStructure])
			if !ok {
				t.Fatalf("HISTORY.events[0] is %T, want *rm.IntervalEvent[rm.ItemStructure]", h.Events[0])
			}
			return ev
		},
	},
}

// defaultsOptions are the generator settings the default rules hold for:
// both policies, each with both value fills.
func defaultsOptions() []instance.Options {
	var out []instance.Options
	for _, policy := range []instance.Policy{instance.Minimal, instance.Example} {
		for _, fill := range []instance.ValueFill{instance.ExampleFill, instance.RandomFill} {
			out = append(out, instance.Options{
				Policy:      policy,
				ValueFill:   fill,
				ValueSource: mrand.NewPCG(1, 2),
				Now:         defaultsNow,
			})
		}
	}
	return out
}

// noFloorErrors reports every error the RM floor finds in out.
func noFloorErrors(t *testing.T, out any) {
	t.Helper()
	for _, iss := range validation.ValidateRM(out).Issues {
		if iss.Severity == validation.Error {
			t.Errorf("ValidateRM: %s @ %s: %s", iss.Code, iss.Path, iss.Detail)
		}
	}
}

// TestREQ107_IntervalEventTakesMeanMathFunction is the REQ-107 check that
// an INTERVAL_EVENT whose OPT gives math_function no value, by leaving it
// silent or naming it without a code, takes the math function openehr
// 146|mean|, with the rubric of the openEHR event math function group, so
// that RM Math_function_validity holds. It holds at the root and nested,
// under both policies and both value fills, with and without the implicit
// attributes.
func TestREQ107_IntervalEventTakesMeanMathFunction(t *testing.T) {
	mean, _ := terminology.EventMathFunction.Rubric("146")
	want := rm.DVCodedText{
		Value:        mean,
		DefiningCode: rm.CodePhrase{CodeString: "146", TerminologyID: rm.TerminologyID{Value: terminology.ID}},
	}
	for _, variant := range mathFunctionVariants {
		for _, place := range eventPlacements {
			for _, implicit := range []bool{true, false} {
				c := compileOPTText(t, place.opt(variant.attrs), implicit)
				for _, opts := range defaultsOptions() {
					t.Run(fmt.Sprintf("%s/%s/implicit=%t/%v/%v", variant.name, place.name, implicit, opts.Policy, opts.ValueFill), func(t *testing.T) {
						out, err := instance.Generate(t.Context(), c, opts)
						if err != nil {
							t.Fatalf("Generate: %v", err)
						}
						got := place.event(t, out).MathFunction
						if got.Value != want.Value || got.DefiningCode != want.DefiningCode {
							t.Errorf("INTERVAL_EVENT.math_function = %q %+v, want %q %+v", got.Value, got.DefiningCode, want.Value, want.DefiningCode)
						}
						noFloorErrors(t, out)
					})
				}
			}
		}
	}
}

// TestREQ107_IntervalEventKeepsPinnedMathFunction is the REQ-107 check that
// a math function code the OPT gives an INTERVAL_EVENT is kept.
func TestREQ107_IntervalEventKeepsPinnedMathFunction(t *testing.T) {
	pinned := []string{optSingle("math_function", optCodedText(terminology.ID, "145"))}
	want := rm.CodePhrase{CodeString: "145", TerminologyID: rm.TerminologyID{Value: terminology.ID}}
	for _, place := range eventPlacements {
		for _, implicit := range []bool{true, false} {
			c := compileOPTText(t, place.opt(pinned), implicit)
			for _, opts := range defaultsOptions() {
				t.Run(fmt.Sprintf("%s/implicit=%t/%v/%v", place.name, implicit, opts.Policy, opts.ValueFill), func(t *testing.T) {
					out, err := instance.Generate(t.Context(), c, opts)
					if err != nil {
						t.Fatalf("Generate: %v", err)
					}
					if got := place.event(t, out).MathFunction.DefiningCode; got != want {
						t.Errorf("INTERVAL_EVENT.math_function.defining_code = %+v, want the pinned %+v", got, want)
					}
					noFloorErrors(t, out)
				})
			}
		}
	}
}

// mediaTypeVariants are the ways an OPT can give a DV_MULTIMEDIA's
// media_type no value: leave it silent, or name it without a code, as an
// attribute with no child, as a CODE_PHRASE with no constraint, or as a
// C_CODE_PHRASE with an empty code list.
var mediaTypeVariants = []struct {
	name  string
	attrs []string
}{
	{name: "silent"},
	{name: "named with no child", attrs: []string{optSingle("media_type")}},
	{name: "bare CODE_PHRASE", attrs: []string{optSingle("media_type", optNode("CODE_PHRASE", ""))}},
	{name: "empty-list C_CODE_PHRASE", attrs: []string{optSingle("media_type", optCodePhrase("IANA_media-types"))}},
}

// multimediaPlacements put a DV_MULTIMEDIA at the template root or as an
// ELEMENT's value. multimedia returns the generated one.
var multimediaPlacements = []struct {
	name       string
	opt        func(attrs []string) string
	multimedia func(t *testing.T, out any) *rm.DVMultimedia
}{
	{
		name: "root",
		opt:  func(attrs []string) string { return optTemplate("DV_MULTIMEDIA", attrs...) },
		multimedia: func(t *testing.T, out any) *rm.DVMultimedia {
			t.Helper()
			mm, ok := out.(*rm.DVMultimedia)
			if !ok {
				t.Fatalf("generated root is %T, want *rm.DVMultimedia", out)
			}
			return mm
		},
	},
	{
		name: "ELEMENT value",
		opt: func(attrs []string) string {
			return optTemplate("ELEMENT", optSingle("value", optNode("DV_MULTIMEDIA", "", attrs...)))
		},
		multimedia: func(t *testing.T, out any) *rm.DVMultimedia {
			t.Helper()
			return rootElementValue[*rm.DVMultimedia](t, out)
		},
	},
}

// checkExampleURI fails t unless mm has no data and the uri
// http://example.com, the RM default that makes Not_empty hold.
func checkExampleURI(t *testing.T, mm *rm.DVMultimedia) {
	t.Helper()
	if len(mm.Data) != 0 {
		t.Errorf("DV_MULTIMEDIA.data = %v, want none", mm.Data)
	}
	uri, ok := mm.URI.(*rm.DVURI)
	if !ok || uri == nil || uri.Value != "http://example.com" {
		t.Errorf("DV_MULTIMEDIA.uri = %#v, want a DV_URI http://example.com", mm.URI)
	}
}

// TestREQ107_MultimediaTakesTextPlainAndExampleURI is the REQ-107 check
// that a DV_MULTIMEDIA whose OPT gives media_type no value, by leaving it
// silent or naming it without a code, takes the media type text/plain in
// IANA_media-types, so that RM Media_type_valid holds, and, having neither
// uri nor data, the uri http://example.com, so that Not_empty holds. It
// holds at the root and nested, under both policies and both value fills,
// with and without the implicit attributes.
func TestREQ107_MultimediaTakesTextPlainAndExampleURI(t *testing.T) {
	want := rm.CodePhrase{CodeString: "text/plain", TerminologyID: rm.TerminologyID{Value: "IANA_media-types"}}
	for _, variant := range mediaTypeVariants {
		for _, place := range multimediaPlacements {
			for _, implicit := range []bool{true, false} {
				c := compileOPTText(t, place.opt(variant.attrs), implicit)
				for _, opts := range defaultsOptions() {
					t.Run(fmt.Sprintf("%s/%s/implicit=%t/%v/%v", variant.name, place.name, implicit, opts.Policy, opts.ValueFill), func(t *testing.T) {
						out, err := instance.Generate(t.Context(), c, opts)
						if err != nil {
							t.Fatalf("Generate: %v", err)
						}
						mm := place.multimedia(t, out)
						if mm.MediaType != want {
							t.Errorf("DV_MULTIMEDIA.media_type = %+v, want %+v", mm.MediaType, want)
						}
						checkExampleURI(t, mm)
						noFloorErrors(t, out)
					})
				}
			}
		}
	}
}

// TestREQ107_MultimediaKeepsPinnedMediaType is the REQ-107 check that a
// media type code the OPT gives a DV_MULTIMEDIA is kept, while the uri
// default still applies. One pin lists text/plain beside image/png, so the
// OPT admits the default and only the rule that keeps a given code stops
// it: under ExampleFill the walk gives the first code, image/png, and
// under RandomFill one of the two.
func TestREQ107_MultimediaKeepsPinnedMediaType(t *testing.T) {
	for _, codes := range [][]string{{"image/png"}, {"image/png", "text/plain"}} {
		pinned := []string{optSingle("media_type", optCodePhrase("IANA_media-types", codes...))}
		for _, place := range multimediaPlacements {
			for _, implicit := range []bool{true, false} {
				c := compileOPTText(t, place.opt(pinned), implicit)
				for _, opts := range defaultsOptions() {
					t.Run(fmt.Sprintf("%v/%s/implicit=%t/%v/%v", codes, place.name, implicit, opts.Policy, opts.ValueFill), func(t *testing.T) {
						out, err := instance.Generate(t.Context(), c, opts)
						if err != nil {
							t.Fatalf("Generate: %v", err)
						}
						mm := place.multimedia(t, out)
						if got := mm.MediaType.TerminologyID.Value; got != "IANA_media-types" {
							t.Errorf("DV_MULTIMEDIA.media_type = %+v, want the pinned terminology IANA_media-types", mm.MediaType)
						}
						switch got := mm.MediaType.CodeString; {
						case opts.ValueFill == instance.ExampleFill && got != codes[0]:
							t.Errorf("DV_MULTIMEDIA.media_type code = %q, want the walk's %q", got, codes[0])
						case !slices.Contains(codes, got):
							t.Errorf("DV_MULTIMEDIA.media_type code = %q, want one of the pinned %q", got, codes)
						}
						checkExampleURI(t, mm)
						noFloorErrors(t, out)
					})
				}
			}
		}
	}
}

// TestREQ107_MultimediaMediaTypeKeepsTheOPTTerminology is the REQ-107
// check that the media type default does not override the terminology the
// OPT's C_CODE_PHRASE names: the generated value must satisfy the OPT's
// primitive constraints, and text/plain in IANA_media-types does not
// satisfy a C_CODE_PHRASE that names the terminology openEHR. The media
// type keeps that terminology, the uri default still applies, and the
// template validator finds no error.
func TestREQ107_MultimediaMediaTypeKeepsTheOPTTerminology(t *testing.T) {
	named := []string{optSingle("media_type", optCodePhrase("openEHR"))}
	for _, place := range multimediaPlacements {
		for _, implicit := range []bool{true, false} {
			c := compileOPTText(t, place.opt(named), implicit)
			for _, opts := range defaultsOptions() {
				t.Run(fmt.Sprintf("%s/implicit=%t/%v/%v", place.name, implicit, opts.Policy, opts.ValueFill), func(t *testing.T) {
					out, err := instance.Generate(t.Context(), c, opts)
					if err != nil {
						t.Fatalf("Generate: %v", err)
					}
					mm := place.multimedia(t, out)
					if got := mm.MediaType.TerminologyID.Value; got != "openEHR" {
						t.Errorf("DV_MULTIMEDIA.media_type = %+v, want the OPT's terminology openEHR", mm.MediaType)
					}
					checkExampleURI(t, mm)
					for _, iss := range validation.Validate(out, c).Issues {
						if iss.Severity == validation.Error {
							t.Errorf("Validate: %s @ %s: %s", iss.Code, iss.Path, iss.Detail)
						}
					}
				})
			}
		}
	}
}
