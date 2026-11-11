package instance_test

import (
	"fmt"
	mrand "math/rand/v2"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// optOptionalSingleOver is a C_SINGLE_ATTRIBUTE called name with existence
// 0..1 over children: optional, but the OPT pins children under it, so
// Large and Medium visit it; Required does not visit an optional
// attribute. The template validator reads such an attribute like any
// other, DV_PARSABLE's charset and language and DV_MULTIMEDIA's
// compression_algorithm among them, and reports a code in it that the RM
// ties to a code set as code_not_in_value_set.
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
// must not replace that local. It holds under every policy, both value
// fills and both compile modes, where the policy visits the code phrase:
// the optional ones under Large and Medium only, since Required does
// not visit an optional attribute. Neither the template validator nor the RM
// floor reports anything on the code phrase, except where an RM rule ties
// the attribute to a code set: there local::at0000 is no member, and the
// floor, alone or inside template validation, reports exactly one
// code_not_in_value_set on the code phrase
// (REQ-107 § Exceptions and known gaps, "A placeholder code on an attribute
// an RM rule ties to a code set or terminology group for which § Contract
// gives no default"). A DV_PARSABLE's charset and language are no such
// case: they take their RM defaults instead, which
// TestREQ107_EncapsulatedAttributesAreWritten checks on the same shape.
func TestREQ107_BareCodePhraseKeepsLocal(t *testing.T) {
	bare := optNode("CODE_PHRASE", "")
	cases := []struct {
		name  string
		value string
		path  string
		code  func(v rm.DataValue) *rm.CodePhrase
		// codeSet is set when an RM rule ties the attribute to a code set
		// (REQ-112 Coded invariants), which the placeholder breaks.
		codeSet bool
		// policies are the policies the case runs under; every policy when
		// empty.
		policies []instance.Policy
	}{
		{
			name:     "DV_COUNT normal_status",
			value:    optNode("DV_COUNT", "", optOptionalSingleOver("normal_status", bare)),
			path:     "/value/normal_status",
			code:     func(v rm.DataValue) *rm.CodePhrase { return v.(*rm.DVCount).NormalStatus },
			codeSet:  true,
			policies: largeAndMedium,
		},
		{
			name:     "DV_QUANTITY normal_status",
			value:    optNode("DV_QUANTITY", "", optOptionalSingleOver("normal_status", bare)),
			path:     "/value/normal_status",
			code:     func(v rm.DataValue) *rm.CodePhrase { return v.(*rm.DVQuantity).NormalStatus },
			codeSet:  true,
			policies: largeAndMedium,
		},
		{
			name:     "DV_MULTIMEDIA compression_algorithm",
			value:    optNode("DV_MULTIMEDIA", "", optOptionalSingleOver("compression_algorithm", bare)),
			path:     "/value/compression_algorithm",
			code:     func(v rm.DataValue) *rm.CodePhrase { return v.(*rm.DVMultimedia).CompressionAlgorithm },
			codeSet:  true,
			policies: largeAndMedium,
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
			for _, opts := range optionsFor(tc.policies) {
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
					var outsideCodeSet int
					for _, iss := range validation.ValidateRM(out).Issues {
						if iss.Severity != validation.Error || !strings.HasPrefix(iss.Path, tc.path) {
							continue
						}
						if tc.codeSet && iss.Code == "code_not_in_value_set" && iss.Path == tc.path {
							outsideCodeSet++
							continue
						}
						t.Errorf("ValidateRM: %s @ %s: %s", iss.Code, iss.Path, iss.Detail)
					}
					want := 0
					if tc.codeSet {
						want = 1
					}
					if outsideCodeSet != want {
						t.Errorf("ValidateRM: %d code_not_in_value_set @ %s, want %d", outsideCodeSet, tc.path, want)
					}
					// Template validation includes the floor, so it carries
					// the same one code_not_in_value_set, and nothing else.
					var templateOutside int
					for _, iss := range templateErrors(out, c) {
						if !strings.HasPrefix(iss.Path, tc.path) {
							continue
						}
						if tc.codeSet && iss.Code == "code_not_in_value_set" && iss.Path == tc.path {
							templateOutside++
							continue
						}
						t.Errorf("template validator: %s @ %s: %s", iss.Code, iss.Path, iss.Detail)
					}
					if templateOutside != want {
						t.Errorf("template validator: %d code_not_in_value_set @ %s, want %d", templateOutside, tc.path, want)
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
// replace local. It holds for a code phrase at the root, under every
// policy, and under an optional DV_COUNT normal_status, under Large and
// Medium, which visit it; both value fills and both compile modes.
func TestREQ107_UnconstrainedTerminologyIDKeepsLocal(t *testing.T) {
	termID := optSingle("terminology_id", optNode("TERMINOLOGY_ID", ""))
	cases := []struct {
		name string
		opt  string
		code func(out any) rm.CodePhrase
		// policies are the policies the case runs under; every policy when
		// empty.
		policies []instance.Policy
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
			policies: largeAndMedium,
		},
	}
	for _, tc := range cases {
		for _, implicit := range []bool{true, false} {
			c := compileOPTText(t, tc.opt, implicit)
			for _, opts := range optionsFor(tc.policies) {
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

// TestREQ107_CodeListWithNoTerminologyKeepsLocal is the REQ-107 check that
// a C_CODE_PHRASE that lists codes and names no terminology, by an empty
// TERMINOLOGY_ID value or by no terminology_id at all, gives a listed code
// in terminology local: the first listed code under ExampleFill, and a
// listed code under RandomFill for each of a few fixed seeds, never in
// another terminology or in an empty one. It holds on a DV_CODED_TEXT's
// defining_code and on a TERM_MAPPING's target, a bare CODE_PHRASE, under
// every policy, since each visits both, and both compile modes. Neither the
// RM floor nor the template validator reports anything on the result.
func TestREQ107_CodeListWithNoTerminologyKeepsLocal(t *testing.T) {
	codes := []string{"at0001", "at0002"}
	emptyTermID := optCodePhrase("", codes...)
	noTermID := strings.Replace(emptyTermID, `<terminology_id><value></value></terminology_id>`, "", 1)
	if noTermID == emptyTermID {
		t.Fatal("optCodePhrase no longer writes an empty terminology_id to remove")
	}
	phrases := []struct {
		name string
		opt  string
	}{
		{name: "empty terminology_id", opt: emptyTermID},
		{name: "no terminology_id", opt: noTermID},
	}
	shapes := []struct {
		name string
		opt  func(codePhrase string) string
		code func(t *testing.T, out any) rm.CodePhrase
	}{
		{
			name: "DV_CODED_TEXT defining_code",
			opt: func(codePhrase string) string {
				return optTemplate("ELEMENT", optSingle("value",
					optNode("DV_CODED_TEXT", "", optSingle("defining_code", codePhrase))))
			},
			code: func(t *testing.T, out any) rm.CodePhrase {
				value := generatedElement(t, out).Value
				ct, ok := value.(*rm.DVCodedText)
				if !ok {
					t.Fatalf("ELEMENT.value is %T, want *rm.DVCodedText", value)
				}
				return ct.DefiningCode
			},
		},
		{
			name: "TERM_MAPPING target",
			opt: func(codePhrase string) string {
				return mappedValueOPT(optSingle("target", codePhrase))
			},
			code: func(t *testing.T, out any) rm.CodePhrase {
				ms := valueMappings(t, out)
				if len(ms) != 1 {
					t.Fatalf("%d mappings generated, want 1", len(ms))
				}
				return ms[0].Target
			},
		},
	}
	firstListed := rm.CodePhrase{CodeString: codes[0], TerminologyID: rm.TerminologyID{Value: "local"}}
	for _, shape := range shapes {
		for _, phrase := range phrases {
			for _, implicit := range []bool{true, false} {
				c := compileOPTText(t, shape.opt(phrase.opt), implicit)
				for _, run := range codeListRuns(everyPolicy, codeListSeeds) {
					opts := run.opts
					t.Run(fmt.Sprintf("%s/%s/implicit=%t/%s", shape.name, phrase.name, implicit, run.name), func(t *testing.T) {
						out, err := instance.Generate(t.Context(), c, opts)
						if err != nil {
							t.Fatalf("Generate: %v", err)
						}
						got := shape.code(t, out)
						if opts.ValueFill == instance.ExampleFill {
							checkCode(t, shape.name, got, firstListed)
						} else if got.TerminologyID.Value != "local" || !slices.Contains(codes, got.CodeString) {
							t.Errorf("%s = %+v, want a code of %v in local", shape.name, got, codes)
						}
						noFloorErrors(t, out)
						for _, iss := range templateErrors(out, c) {
							t.Errorf("template validator: %s @ %s: %s", iss.Code, iss.Path, iss.Detail)
						}
					})
				}
			}
		}
	}
}

// codeListSeeds are the PCG seeds a RandomFill case draws a listed code
// with, one generation each.
var codeListSeeds = [][2]uint64{{1, 2}, {3, 4}, {5, 6}, {7, 8}}

// codeListRun is one generator setting a case runs under, with the name of
// its subtest.
type codeListRun struct {
	name string
	opts instance.Options
}

// codeListRuns are the settings of optionsFor under policies, with
// RandomFill run once per seed in seeds instead of once. ExampleFill
// ignores its source, so it runs once per policy.
func codeListRuns(policies []instance.Policy, seeds [][2]uint64) []codeListRun {
	var out []codeListRun
	for _, opts := range optionsFor(policies) {
		if opts.ValueFill == instance.ExampleFill {
			out = append(out, codeListRun{name: fmt.Sprintf("%v/%v", opts.Policy, opts.ValueFill), opts: opts})
			continue
		}
		for _, seed := range seeds {
			opts.ValueSource = mrand.NewPCG(seed[0], seed[1])
			out = append(out, codeListRun{
				name: fmt.Sprintf("%v/%v/seed=%d,%d", opts.Policy, opts.ValueFill, seed[0], seed[1]),
				opts: opts,
			})
		}
	}
	return out
}
