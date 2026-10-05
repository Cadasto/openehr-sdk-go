package instance_test

import (
	"fmt"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// localPlaceholder is the code REQ-107 makes the generator write on a code
// phrase it has no OPT value for.
var localPlaceholder = rm.CodePhrase{
	CodeString:    "at0000",
	TerminologyID: rm.TerminologyID{Value: "local"},
}

// TestREQ107_RootPlaceholders is the REQ-107 check that a generation root
// gets the same placeholders as a nested value: the URI ehr://example on a
// DV_EHR_URI and the code at0000 in terminology local on a CODE_PHRASE.
// The root is compiled with the implicit attributes, so its String
// attributes are filled by the pass that writes the open-string example.
func TestREQ107_RootPlaceholders(t *testing.T) {
	cases := []struct {
		name  string
		root  string
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
	}
	for _, tc := range cases {
		c := compileOPTText(t, optTemplate(tc.root), true)
		for _, policy := range []instance.Policy{instance.Minimal, instance.Example} {
			t.Run(tc.name+"/"+policy.String(), func(t *testing.T) {
				out, err := instance.Generate(t.Context(), c, instance.Options{Policy: policy, Now: defaultsNow})
				if err != nil {
					t.Fatalf("Generate(%v): %v", policy, err)
				}
				tc.check(t, out)
			})
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
// through the generator.
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
			for _, policy := range []instance.Policy{instance.Minimal, instance.Example} {
				t.Run(fmt.Sprintf("%s/implicit=%t/%v", tc.name, implicit, policy), func(t *testing.T) {
					out, err := instance.Generate(t.Context(), c, instance.Options{Policy: policy, Now: defaultsNow})
					if err != nil {
						t.Fatalf("Generate: %v", err)
					}
					tc.check(t, out)
				})
			}
		}
	}

	// A root DV_EHR_URI without the implicit attributes has no value
	// attribute to walk, so the placeholder is written after the walk.
	c := compileOPTText(t, optTemplate("DV_EHR_URI"), false)
	for _, policy := range []instance.Policy{instance.Minimal, instance.Example} {
		t.Run("DV_EHR_URI root/implicit=false/"+policy.String(), func(t *testing.T) {
			out, err := instance.Generate(t.Context(), c, instance.Options{Policy: policy, Now: defaultsNow})
			if err != nil {
				t.Fatalf("Generate: %v", err)
			}
			if got := out.(*rm.DVEHRURI).Value; got != "ehr://example" {
				t.Errorf("DV_EHR_URI.value = %q, want %q", got, "ehr://example")
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
