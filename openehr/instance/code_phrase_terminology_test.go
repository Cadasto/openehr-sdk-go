package instance_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// optOptionalSingleOver is a C_SINGLE_ATTRIBUTE called name with existence
// 0..1 over children: optional, but the OPT pins children under it, so
// both policies visit it. The template validator reads no value for some
// such attributes (DV_PARSABLE charset and language, DV_MULTIMEDIA
// compression_algorithm), and would report a required one absent.
func optOptionalSingleOver(name string, children ...string) string {
	return `<attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>` + name + `</rm_attribute_name>` +
		`<existence><lower_included>true</lower_included><upper_included>true</upper_included>` +
		`<lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded>` +
		`<lower>0</lower><upper>1</upper></existence>` + strings.Join(children, "") + `</attributes>`
}

// TestREQ107_BareCodePhraseKeepsLocal is the REQ-107 check that a code
// phrase the OPT names with no constraint gets the code at0000 in
// terminology local, wherever it sits: the empty TERMINOLOGY_ID the
// generator builds from the BMM for the code phrase's own terminology_id
// must not replace that local. It holds under both policies, both value
// fills and both compile modes, and neither the RM floor nor the template
// validator reports anything on the code phrase.
func TestREQ107_BareCodePhraseKeepsLocal(t *testing.T) {
	bare := optNode("CODE_PHRASE", "")
	cases := []struct {
		name  string
		value string
		path  string
		code  func(v rm.DataValue) *rm.CodePhrase
	}{
		{
			name:  "DV_COUNT normal_status",
			value: optNode("DV_COUNT", "", optOptionalSingleOver("normal_status", bare)),
			path:  "/value/normal_status",
			code:  func(v rm.DataValue) *rm.CodePhrase { return v.(*rm.DVCount).NormalStatus },
		},
		{
			name:  "DV_QUANTITY normal_status",
			value: optNode("DV_QUANTITY", "", optOptionalSingleOver("normal_status", bare)),
			path:  "/value/normal_status",
			code:  func(v rm.DataValue) *rm.CodePhrase { return v.(*rm.DVQuantity).NormalStatus },
		},
		{
			name:  "DV_PARSABLE charset",
			value: optNode("DV_PARSABLE", "", optOptionalSingleOver("charset", bare)),
			path:  "/value/charset",
			code:  func(v rm.DataValue) *rm.CodePhrase { return v.(*rm.DVParsable).Charset },
		},
		{
			name:  "DV_PARSABLE language",
			value: optNode("DV_PARSABLE", "", optOptionalSingleOver("language", bare)),
			path:  "/value/language",
			code:  func(v rm.DataValue) *rm.CodePhrase { return v.(*rm.DVParsable).Language },
		},
		{
			name:  "DV_MULTIMEDIA compression_algorithm",
			value: optNode("DV_MULTIMEDIA", "", optOptionalSingleOver("compression_algorithm", bare)),
			path:  "/value/compression_algorithm",
			code:  func(v rm.DataValue) *rm.CodePhrase { return v.(*rm.DVMultimedia).CompressionAlgorithm },
		},
		{
			name:  "DV_CODED_TEXT defining_code",
			value: optNode("DV_CODED_TEXT", "", optOptionalSingleOver("defining_code", bare)),
			path:  "/value/defining_code",
			code:  func(v rm.DataValue) *rm.CodePhrase { return &v.(*rm.DVCodedText).DefiningCode },
		},
	}
	for _, tc := range cases {
		for _, implicit := range []bool{true, false} {
			c := compileOPTText(t, optTemplate("ELEMENT", optSingle("value", tc.value)), implicit)
			for _, opts := range defaultsOptions() {
				t.Run(fmt.Sprintf("%s/implicit=%t/%v/%v", tc.name, implicit, opts.Policy, opts.ValueFill), func(t *testing.T) {
					out, err := instance.Generate(t.Context(), c, opts)
					if err != nil {
						t.Fatalf("Generate: %v", err)
					}
					cp := tc.code(out.(*rm.Element).Value)
					if cp == nil {
						t.Fatalf("%s absent, want local::at0000", tc.name)
					}
					checkCode(t, tc.name, *cp, localAt0000)
					for _, iss := range validation.ValidateRM(out).Issues {
						if iss.Severity == validation.Error && strings.HasPrefix(iss.Path, tc.path) {
							t.Errorf("ValidateRM: %s @ %s: %s", iss.Code, iss.Path, iss.Detail)
						}
					}
					for _, iss := range templateErrors(out, c) {
						if strings.HasPrefix(iss.Path, tc.path) {
							t.Errorf("template validator: %s @ %s: %s", iss.Code, iss.Path, iss.Detail)
						}
					}
				})
			}
		}
	}
}

// TestREQ107_UnconstrainedTerminologyIDKeepsLocal is the REQ-107 check
// that a code phrase whose terminology_id the OPT names as a TERMINOLOGY_ID
// with no constraint on its value keeps the terminology local: the walk
// cannot write that value, and the empty TERMINOLOGY_ID it builds must not
// replace local. It holds for a code phrase at the root and under DV_COUNT
// normal_status, under both policies, both value fills and both compile
// modes.
func TestREQ107_UnconstrainedTerminologyIDKeepsLocal(t *testing.T) {
	termID := optSingle("terminology_id", optNode("TERMINOLOGY_ID", ""))
	cases := []struct {
		name string
		opt  string
		code func(out any) rm.CodePhrase
	}{
		{
			name: "root CODE_PHRASE",
			opt:  optTemplate("CODE_PHRASE", termID),
			code: func(out any) rm.CodePhrase { return *out.(*rm.CodePhrase) },
		},
		{
			name: "DV_COUNT normal_status",
			opt: optTemplate("ELEMENT", optSingle("value",
				optNode("DV_COUNT", "", optOptionalSingleOver("normal_status", optNode("CODE_PHRASE", "", termID))))),
			code: func(out any) rm.CodePhrase {
				ns := out.(*rm.Element).Value.(*rm.DVCount).NormalStatus
				if ns == nil {
					return rm.CodePhrase{}
				}
				return *ns
			},
		},
	}
	for _, tc := range cases {
		for _, implicit := range []bool{true, false} {
			c := compileOPTText(t, tc.opt, implicit)
			for _, opts := range defaultsOptions() {
				t.Run(fmt.Sprintf("%s/implicit=%t/%v/%v", tc.name, implicit, opts.Policy, opts.ValueFill), func(t *testing.T) {
					out, err := instance.Generate(t.Context(), c, opts)
					if err != nil {
						t.Fatalf("Generate: %v", err)
					}
					checkCode(t, tc.name, tc.code(out), localAt0000)
				})
			}
		}
	}
}
