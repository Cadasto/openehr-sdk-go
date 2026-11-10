package simplified_test

// REQ-053 — a WithTemplate decode takes from the template the suffixes of a
// coded value the formats let a client leave out (issue 423). ITS-REST 1.1.0
// Simplified Formats requires a DV_CODED_TEXT's |value and |terminology "only
// for external terminologies", and lets a DV_ORDINAL's |value and |ordinal
// "be left out if symbol is defined in template". So with the template, a
// coded text takes the terminology its defining_code constraint names and the
// term text of a code that constraint lists, and an ordinal takes the value
// and the term text the C_DV_ORDINAL pairs give its code. A terminology the
// template does not determine is a missing required suffix. Without the
// template decode is unchanged.

import (
	"bytes"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aom/aom14"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/rmpath"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/simplified"
	"github.com/cadasto/openehr-sdk-go/openehr/template/webtemplate"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// The test_all_types leaves the cases write.
const (
	// contextCoded is a DV_CODED_TEXT whose template lists the local codes
	// at0006 "value1", at0007 "value2" and at0008 "value3".
	contextCoded = "test_all_types/context/context_coded_text"
	// obsCoded is a DV_CODED_TEXT whose template names the local terminology
	// and lists no code.
	obsCoded = shorthandObs + "/coded_text"
	// obsReference is a DV_CODED_TEXT whose template names a SNOMED-CT
	// reference set and no terminology.
	obsReference = shorthandObs + "/coded_text_terminology"
	// obsOrdinal is a DV_ORDINAL whose template pairs the local codes at0014,
	// at0015 and at0016 with 0, 1 and 2, the texts "ord1", "ord1" and "ord3".
	obsOrdinal = shorthandObs + "/ordinal"
	// obsText is a DV_TEXT, at which |code selects a DV_CODED_TEXT.
	obsText = shorthandObs + "/text"
	// contextCodedPath is the canonical path of contextCoded's value.
	contextCodedPath = "/context/other_context[at0004]/item[at0005]/value"
)

// completionBody is the issue's body without its leaf: the four ctx/ fields,
// with leaves merged over them.
func completionBody(leaves map[string]any) map[string]any {
	body := map[string]any{
		"ctx/language":      "en",
		"ctx/territory":     "US",
		"ctx/composer_name": "Silvia Blake",
		"ctx/time":          "2019-03-22T22:00:00+01:00",
	}
	maps.Copy(body, leaves)
	return body
}

// allTypesOPT is the text of the test_all_types OPT.
func allTypesOPT(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(fixtures.CrossFormatRoot(), "test_all_types", "template.opt"))
	if err != nil {
		t.Fatalf("read test_all_types template.opt: %v", err)
	}
	return string(raw)
}

// editedAllTypes compiles opt, an edit of the test_all_types OPT, and builds
// its Web Template.
func editedAllTypes(t *testing.T, opt string) (*webtemplate.WebTemplate, *templatecompile.Compiled) {
	t.Helper()
	model, err := aom14.ReadOperationalTemplate(strings.NewReader(opt))
	if err != nil {
		t.Fatalf("ReadOperationalTemplate(edited test_all_types): %v", err)
	}
	return compileTemplate(t, "test_all_types", model)
}

// replaceExactlyOnce returns s with its one occurrence of old replaced by
// repl, failing the test where old does not occur exactly once.
func replaceExactlyOnce(t *testing.T, s, old, repl string) string {
	t.Helper()
	if n := strings.Count(s, old); n != 1 {
		t.Fatalf("%q occurs %d times, want once — fixture changed?", old, n)
	}
	return strings.Replace(s, old, repl, 1)
}

// decodeWith decodes body with opts, failing the test on an error.
func decodeWith(t *testing.T, wt *webtemplate.WebTemplate, body map[string]any, opts ...simplified.Option) *rm.Composition {
	t.Helper()
	comp, err := simplified.UnmarshalFlat(mustJSON(t, body), wt, opts...)
	if err != nil {
		t.Fatalf("UnmarshalFlat(%v) = %v", body, err)
	}
	return comp
}

// completionCase is a body that leaves suffixes out and the full spelling it
// decodes to with the template.
type completionCase struct {
	name        string
	short, full map[string]any
}

// TestREQ053TemplateCompletesCodedValues — REQ-053: with the template, a
// coded text's |code alone, or with only one of |value and |terminology,
// decodes to what its full spelling decodes to, and so does an ordinal's
// |code alone, or with only one of |value and |ordinal; the composition
// validates. The first two cases are issue 423's bodies.
func TestREQ053TemplateCompletesCodedValues(t *testing.T) {
	wt, compiled := shorthandTarget(t)
	fullCoded := map[string]any{contextCoded + "|code": "at0007", contextCoded + "|value": "value2", contextCoded + "|terminology": "local"}
	fullOrdinal := map[string]any{obsOrdinal + "|code": "at0015", obsOrdinal + "|value": "ord1", obsOrdinal + "|ordinal": 1}
	for _, tc := range []completionCase{
		{name: "local code alone", short: map[string]any{contextCoded + "|code": "at0007"}, full: fullCoded},
		{
			name:  "local code and value",
			short: map[string]any{contextCoded + "|code": "at0007", contextCoded + "|value": "value2"},
			full:  fullCoded,
		},
		{
			name:  "local code and terminology",
			short: map[string]any{contextCoded + "|code": "at0007", contextCoded + "|terminology": "local"},
			full:  fullCoded,
		},
		{
			name:  "local code and a null terminology",
			short: map[string]any{contextCoded + "|code": "at0007", contextCoded + "|terminology": nil},
			full:  fullCoded,
		},
		{
			name:  "local terminology and no code list",
			short: map[string]any{obsCoded + "|code": "at0006", obsCoded + "|value": "value1"},
			full:  map[string]any{obsCoded + "|code": "at0006", obsCoded + "|value": "value1", obsCoded + "|terminology": "local"},
		},
		{name: "ordinal code alone", short: map[string]any{obsOrdinal + "|code": "at0015"}, full: fullOrdinal},
		{
			name:  "ordinal code and value",
			short: map[string]any{obsOrdinal + "|code": "at0015", obsOrdinal + "|value": "ord1"},
			full:  fullOrdinal,
		},
		{
			name:  "ordinal code and ordinal",
			short: map[string]any{obsOrdinal + "|code": "at0015", obsOrdinal + "|ordinal": 1},
			full:  fullOrdinal,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			withTemplate := simplified.WithTemplate(compiled)
			got := decodeWith(t, wt, completionBody(tc.short), withTemplate)
			want := decodeWith(t, wt, completionBody(tc.full), withTemplate)
			if g, w := canonicalComposition(t, got), canonicalComposition(t, want); !bytes.Equal(g, w) {
				t.Errorf("%v decodes to\n%s\nwant what %v decodes to\n%s", tc.short, g, tc.full, w)
			}
			if res := validation.ValidateComposition(got, compiled); !res.OK {
				t.Errorf("ValidateComposition(%v) is not OK: %+v", tc.short, res.Issues)
			}
		})
	}
}

// TestREQ053TemplateCompletionInStructured — REQ-053: STRUCTURED decodes
// through the FLAT keys, so it completes a coded value as FLAT does.
func TestREQ053TemplateCompletionInStructured(t *testing.T) {
	wt, compiled := shorthandTarget(t)
	short := completionBody(map[string]any{contextCoded + "|code": "at0007", obsOrdinal + "|code": "at0015"})
	full := completionBody(map[string]any{
		contextCoded + "|code": "at0007", contextCoded + "|value": "value2", contextCoded + "|terminology": "local",
		obsOrdinal + "|code": "at0015", obsOrdinal + "|value": "ord1", obsOrdinal + "|ordinal": 1,
	})
	doc, err := simplified.FlatToStructured(mustJSON(t, short))
	if err != nil {
		t.Fatalf("FlatToStructured(%v) = %v", short, err)
	}
	got, err := simplified.UnmarshalStructured(doc, wt, simplified.WithTemplate(compiled))
	if err != nil {
		t.Fatalf("UnmarshalStructured(%s) = %v", doc, err)
	}
	want := decodeWith(t, wt, full, simplified.WithTemplate(compiled))
	if g, w := canonicalComposition(t, got), canonicalComposition(t, want); !bytes.Equal(g, w) {
		t.Errorf("STRUCTURED %s decodes to\n%s\nwant what FLAT %v decodes to\n%s", doc, g, full, w)
	}
}

// TestREQ053TemplateCompletionKeepsWhatTheBodyGives — REQ-053: a |value, a
// |terminology or an |ordinal the body gives is kept as written, even where
// the template gives another, for validation to judge; only what the body
// leaves out is taken from the template.
func TestREQ053TemplateCompletionKeepsWhatTheBodyGives(t *testing.T) {
	wt, compiled := shorthandTarget(t)
	for _, tc := range []completionCase{
		{
			name:  "coded text value",
			short: map[string]any{contextCoded + "|code": "at0007", contextCoded + "|value": "my words"},
			full:  map[string]any{contextCoded + "|code": "at0007", contextCoded + "|value": "my words", contextCoded + "|terminology": "local"},
		},
		{
			name:  "ordinal value",
			short: map[string]any{obsOrdinal + "|code": "at0015", obsOrdinal + "|value": "my words"},
			full:  map[string]any{obsOrdinal + "|code": "at0015", obsOrdinal + "|value": "my words", obsOrdinal + "|ordinal": 1},
		},
		{
			name:  "ordinal ordinal",
			short: map[string]any{obsOrdinal + "|code": "at0015", obsOrdinal + "|ordinal": 2},
			full:  map[string]any{obsOrdinal + "|code": "at0015", obsOrdinal + "|value": "ord1", obsOrdinal + "|ordinal": 2},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			withTemplate := simplified.WithTemplate(compiled)
			got := decodeWith(t, wt, completionBody(tc.short), withTemplate)
			want := decodeWith(t, wt, completionBody(tc.full), withTemplate)
			if g, w := canonicalComposition(t, got), canonicalComposition(t, want); !bytes.Equal(g, w) {
				t.Errorf("%v decodes to\n%s\nwant what %v decodes to\n%s", tc.short, g, tc.full, w)
			}
		})
	}
	t.Run("coded text terminology", func(t *testing.T) {
		comp := decodeWith(t, wt, completionBody(map[string]any{
			contextCoded + "|code": "at0007", contextCoded + "|value": "value2", contextCoded + "|terminology": "SNOMED-CT",
		}), simplified.WithTemplate(compiled))
		if got := codedTextAt(t, comp, contextCodedPath).DefiningCode.TerminologyID.Value; got != "SNOMED-CT" {
			t.Errorf("terminology = %q, want the body's SNOMED-CT", got)
		}
	})
}

// codedTextAt returns the DV_CODED_TEXT comp holds at path.
func codedTextAt(t *testing.T, comp *rm.Composition, path string) *rm.DVCodedText {
	t.Helper()
	items, err := rmpath.ItemsAtPath(comp, path)
	if err != nil || len(items) != 1 {
		t.Fatalf("ItemsAtPath(%s) = %d items, %v; want one coded text", path, len(items), err)
	}
	coded, ok := items[0].(*rm.DVCodedText)
	if !ok {
		t.Fatalf("ItemsAtPath(%s) = %T, want *rm.DVCodedText", path, items[0])
	}
	return coded
}

// refusalCase is a body the decode refuses with ErrUnsupportedDatatype, the
// message naming the leaf's FLAT key and the missing suffix.
type refusalCase struct {
	name   string
	leaves map[string]any
	key    string
	reason string
}

// wantMissingSuffix decodes body against wt with opts and checks the decode
// refuses it as tc says.
func wantMissingSuffix(t *testing.T, wt *webtemplate.WebTemplate, tc refusalCase, opts ...simplified.Option) {
	t.Helper()
	comp, err := simplified.UnmarshalFlat(mustJSON(t, completionBody(tc.leaves)), wt, opts...)
	if err == nil {
		t.Fatalf("UnmarshalFlat(%v) = %s, want it refused: %s", tc.leaves, canonicalComposition(t, comp), tc.reason)
	}
	wantRefusal(t, err, simplified.ErrUnsupportedDatatype, `"`+tc.key+`"`, tc.reason)
}

// TestREQ053TemplateCompletionRefusesWhatItDoesNotDetermine — REQ-053,
// REQ-170: with the template, a coded text whose |terminology is absent and
// that the template does not determine is refused as a missing required
// suffix, and so is a |value or an |ordinal the template does not determine;
// each refusal names the FLAT key.
func TestREQ053TemplateCompletionRefusesWhatItDoesNotDetermine(t *testing.T) {
	wt, compiled := shorthandTarget(t)
	const (
		missingTerminology = "DV_CODED_TEXT missing required |terminology"
		missingValue       = "DV_CODED_TEXT missing required |value"
	)
	for _, tc := range []refusalCase{
		{
			name:   "a reference set with no terminology",
			leaves: map[string]any{obsReference + "|code": "1004034", obsReference + "|value": "a term"},
			key:    obsReference, reason: missingTerminology,
		},
		{
			name:   "a coded text at a DV_TEXT leaf",
			leaves: map[string]any{obsText + "|code": "c1", obsText + "|value": "a term"},
			key:    obsText, reason: missingTerminology,
		},
		{
			name:   "a code outside the code list",
			leaves: map[string]any{contextCoded + "|code": "at0099"},
			key:    contextCoded, reason: missingValue,
		},
		{
			name:   "a local code and no code list",
			leaves: map[string]any{obsCoded + "|code": "at0006"},
			key:    obsCoded, reason: missingValue,
		},
		{
			name:   "a listed code in another terminology",
			leaves: map[string]any{contextCoded + "|code": "at0007", contextCoded + "|terminology": "SNOMED-CT"},
			key:    contextCoded, reason: missingValue,
		},
		{
			name:   "an ordinal code outside the pairs",
			leaves: map[string]any{obsOrdinal + "|code": "at0099"},
			key:    obsOrdinal, reason: "DV_ORDINAL missing required |value",
		},
		{
			name:   "an ordinal code outside the pairs, with its value",
			leaves: map[string]any{obsOrdinal + "|code": "at0099", obsOrdinal + "|value": "ord9"},
			key:    obsOrdinal, reason: "DV_ORDINAL missing required |ordinal",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			wantMissingSuffix(t, wt, tc, simplified.WithTemplate(compiled))
		})
	}
}

// TestREQ053DecodeWithoutTemplateCompletesNothing — REQ-053: without the
// template decode is unchanged. A coded text needs |code and |value and takes
// no terminology it does not carry; an ordinal needs all three suffixes.
func TestREQ053DecodeWithoutTemplateCompletesNothing(t *testing.T) {
	wt, _ := shorthandTarget(t)
	for _, tc := range []refusalCase{
		{name: "coded text code alone", leaves: map[string]any{contextCoded + "|code": "at0007"}, key: contextCoded, reason: "DV_CODED_TEXT missing required |value"},
		{name: "ordinal code alone", leaves: map[string]any{obsOrdinal + "|code": "at0015"}, key: obsOrdinal, reason: "DV_ORDINAL missing required |value"},
		{
			name:   "ordinal code and value",
			leaves: map[string]any{obsOrdinal + "|code": "at0015", obsOrdinal + "|value": "ord1"},
			key:    obsOrdinal, reason: "DV_ORDINAL missing required |ordinal",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			wantMissingSuffix(t, wt, tc)
		})
	}
	t.Run("coded text with no terminology", func(t *testing.T) {
		comp := decodeWith(t, wt, completionBody(map[string]any{contextCoded + "|code": "at0007", contextCoded + "|value": "value2"}))
		if got := codedTextAt(t, comp, contextCodedPath).DefiningCode.TerminologyID.Value; got != "" {
			t.Errorf("terminology = %q, want none: decode without the template takes none from it", got)
		}
	})
}

// TestREQ053TemplateCompletionExternalTerminology — REQ-053: a template that
// names an external terminology and lists no code gives a coded text that
// terminology, and no |value: it lists no code whose term text it could give.
// No vendored template constrains a coded text so; this one names SNOMED-CT
// on test_all_types' reference set.
func TestREQ053TemplateCompletionExternalTerminology(t *testing.T) {
	const reference = "<referenceSetUri>terminology:SNOMED-CT</referenceSetUri>"
	opt := replaceExactlyOnce(t, allTypesOPT(t), reference,
		"<terminology_id><value>SNOMED-CT</value></terminology_id>"+reference)
	wt, compiled := editedAllTypes(t, opt)
	withTemplate := simplified.WithTemplate(compiled)
	t.Run("code and value", func(t *testing.T) {
		short := map[string]any{obsReference + "|code": "1004034", obsReference + "|value": "a term"}
		full := map[string]any{obsReference + "|code": "1004034", obsReference + "|value": "a term", obsReference + "|terminology": "SNOMED-CT"}
		got := decodeWith(t, wt, completionBody(short), withTemplate)
		want := decodeWith(t, wt, completionBody(full), withTemplate)
		if g, w := canonicalComposition(t, got), canonicalComposition(t, want); !bytes.Equal(g, w) {
			t.Errorf("%v decodes to\n%s\nwant what %v decodes to\n%s", short, g, full, w)
		}
		if res := validation.ValidateComposition(got, compiled); !res.OK {
			t.Errorf("ValidateComposition(%v) is not OK: %+v", short, res.Issues)
		}
	})
	t.Run("code alone", func(t *testing.T) {
		wantMissingSuffix(t, wt, refusalCase{
			leaves: map[string]any{obsReference + "|code": "1004034"},
			key:    obsReference, reason: "DV_CODED_TEXT missing required |value",
		}, withTemplate)
	})
}

// TestREQ053TemplateCompletionNeedsAgreement — REQ-053: the template
// determines nothing where it disagrees with itself. Two coded-text
// alternatives at one path that name different terminologies give no
// terminology, ordinal pairs that give one code two values give it no
// ordinal and no text, and pairs that give it two texts give it no text.
func TestREQ053TemplateCompletionNeedsAgreement(t *testing.T) {
	t.Run("coded text alternatives", func(t *testing.T) {
		// alternative is contextCoded's DV_CODED_TEXT, from its opening tag to
		// its close after the code list; snomed is a copy in SNOMED-CT.
		opt := allTypesOPT(t)
		const lastCode = "<code_list>at0008</code_list>"
		replaceExactlyOnce(t, opt, lastCode, lastCode)
		listAt := strings.Index(opt, lastCode)
		start := strings.LastIndex(opt[:listAt], `<children xsi:type="C_COMPLEX_OBJECT">`)
		closing := regexp.MustCompile(`^` + lastCode + `\s*</children>\s*</attributes>\s*</children>`).FindStringIndex(opt[listAt:])
		if start < 0 || closing == nil {
			t.Fatal("the test_all_types OPT no longer holds contextCoded's DV_CODED_TEXT as expected — fixture changed?")
		}
		alternative := opt[start : listAt+closing[1]]
		snomed := replaceExactlyOnce(t, alternative, "<value>local</value>", "<value>SNOMED-CT</value>")
		wt, compiled := editedAllTypes(t, replaceExactlyOnce(t, opt, alternative, alternative+snomed))
		wantMissingSuffix(t, wt, refusalCase{
			leaves: map[string]any{contextCoded + "|code": "at0007", contextCoded + "|value": "value2"},
			key:    contextCoded, reason: "DV_CODED_TEXT missing required |terminology",
		}, simplified.WithTemplate(compiled))
	})
	t.Run("ordinal pairs", func(t *testing.T) {
		opt := replaceExactlyOnce(t, allTypesOPT(t), "<code_string>at0016</code_string>", "<code_string>at0015</code_string>")
		wt, compiled := editedAllTypes(t, opt)
		for _, tc := range []refusalCase{
			{leaves: map[string]any{obsOrdinal + "|code": "at0015"}, key: obsOrdinal, reason: "DV_ORDINAL missing required |value"},
			{
				leaves: map[string]any{obsOrdinal + "|code": "at0015", obsOrdinal + "|value": "ord1"},
				key:    obsOrdinal, reason: "DV_ORDINAL missing required |ordinal",
			},
		} {
			wantMissingSuffix(t, wt, tc, simplified.WithTemplate(compiled))
		}
	})
	t.Run("ordinal texts", func(t *testing.T) {
		// A SNOMED-CT pair beside the local one, before or after it, gives
		// at0015 the same value, 1, and no term text, where the local pair
		// gives it "ord1".
		pair := regexp.MustCompile(`<list>\s*<value>1</value>\s*<symbol>\s*<value/>\s*<defining_code>\s*<terminology_id>\s*<value>local</value>\s*</terminology_id>\s*<code_string>at0015</code_string>\s*</defining_code>\s*</symbol>\s*</list>`)
		opt := allTypesOPT(t)
		local := matchOnce(t, pair, opt, "the test_all_types OPT")
		snomed := `<list><value>1</value><symbol><value/><defining_code><terminology_id><value>SNOMED-CT</value></terminology_id>` +
			`<code_string>at0015</code_string></defining_code></symbol></list>`
		for order, pairs := range map[string]string{"SNOMED-CT first": snomed + local, "local first": local + snomed} {
			t.Run(order, func(t *testing.T) {
				wt, compiled := editedAllTypes(t, replaceExactlyOnce(t, opt, local, pairs))
				withTemplate := simplified.WithTemplate(compiled)
				wantMissingSuffix(t, wt, refusalCase{
					leaves: map[string]any{obsOrdinal + "|code": "at0015"},
					key:    obsOrdinal, reason: "DV_ORDINAL missing required |value",
				}, withTemplate)
				short := map[string]any{obsOrdinal + "|code": "at0015", obsOrdinal + "|value": "ord1"}
				full := map[string]any{obsOrdinal + "|code": "at0015", obsOrdinal + "|value": "ord1", obsOrdinal + "|ordinal": 1}
				got := decodeWith(t, wt, completionBody(short), withTemplate)
				want := decodeWith(t, wt, completionBody(full), withTemplate)
				if g, w := canonicalComposition(t, got), canonicalComposition(t, want); !bytes.Equal(g, w) {
					t.Errorf("%v decodes to\n%s\nwant what %v decodes to\n%s", short, g, full, w)
				}
			})
		}
	})
}

// intervalOrdinalTemplate is Test_dv_interval_dv_count_lower_upper_constraint
// with its DV_INTERVAL<DV_COUNT> leaf retargeted to DV_INTERVAL<DV_ORDINAL>,
// each bound a C_DV_ORDINAL that pairs the local codes at0047 "mild" and
// at0048 "severe" with 1 and 2. It returns the Web Template, the compiled
// template, the leaf's FLAT key and its canonical path.
func intervalOrdinalTemplate(t *testing.T) (wt *webtemplate.WebTemplate, compiled *templatecompile.Compiled, key, path string) {
	t.Helper()
	const (
		id         = "Test_dv_interval_dv_count_lower_upper_constraint.v0"
		countType  = "DV_INTERVAL&lt;DV_COUNT&gt;"
		boundStart = "<children xsi:type=\"C_COMPLEX_OBJECT\">\n<rm_type_name>DV_COUNT</rm_type_name>"
		boundEnd   = "</attributes>\n                            </children>"
		exactlyOne = "<lower_included>true</lower_included><upper_included>true</upper_included>" +
			"<lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded><lower>1</lower><upper>1</upper>"
		pairs = `<list><value>1</value><symbol><defining_code><terminology_id><value>local</value></terminology_id>` +
			`<code_string>at0047</code_string></defining_code></symbol></list>` +
			`<list><value>2</value><symbol><defining_code><terminology_id><value>local</value></terminology_id>` +
			`<code_string>at0048</code_string></defining_code></symbol></list>`
		terms = `<term_definitions code="at0047"><items id="text">mild</items><items id="description">mild</items></term_definitions>` +
			`<term_definitions code="at0048"><items id="text">severe</items><items id="description">severe</items></term_definitions>`
	)
	ordinalBound := `<children xsi:type="C_DV_ORDINAL"><rm_type_name>DV_ORDINAL</rm_type_name><occurrences>` + exactlyOne +
		`</occurrences><node_id></node_id>` + pairs + `</children>`
	raw, err := os.ReadFile(fixtures.TemplateOpt(id))
	if err != nil {
		t.Fatal(err)
	}
	opt := replaceExactlyOnce(t, string(raw), countType, "DV_INTERVAL&lt;DV_ORDINAL&gt;")
	if strings.Count(opt, boundStart) != 2 {
		t.Fatalf("%s no longer holds two DV_COUNT bounds", id)
	}
	for range 2 {
		start := strings.Index(opt, boundStart)
		end := strings.Index(opt[start:], boundEnd)
		if end < 0 {
			t.Fatalf("%s: a DV_COUNT bound is not closed as expected", id)
		}
		opt = opt[:start] + ordinalBound + opt[start+end+len(boundEnd):]
	}
	opt = replaceExactlyOnce(t, opt, `<term_definitions code="at0046">`, terms+`<term_definitions code="at0046">`)
	model, err := aom14.ReadOperationalTemplate(strings.NewReader(opt))
	if err != nil {
		t.Fatalf("ReadOperationalTemplate: %v", err)
	}
	wt, compiled = compileTemplate(t, id, model)
	var leaf *webtemplate.Node
	var find func(n *webtemplate.Node)
	find = func(n *webtemplate.Node) {
		if strings.HasPrefix(n.RMType, "DV_INTERVAL") {
			leaf = n
		}
		for _, ch := range n.Children {
			find(ch)
		}
	}
	find(wt.Tree)
	if leaf == nil {
		t.Fatal("the Web Template has no DV_INTERVAL leaf")
	}
	key, ok := flatKeyOf(wt.Tree, leaf.AQLPath, nil)
	if !ok {
		t.Fatalf("no FLAT key for %s", leaf.AQLPath)
	}
	return wt, compiled, key, leaf.AQLPath
}

// TestREQ053TemplateCompletesIntervalOrdinalBounds — REQ-053: with the
// template, each bound of a DV_INTERVAL<DV_ORDINAL> leaf is completed as a
// leaf ordinal is: a bound that carries |code alone, or |code with one of
// |value and |ordinal, takes the rest from the C_DV_ORDINAL pairs of its
// bound, in FLAT and in STRUCTURED. What the bound gives is kept. A code the
// pairs do not hold is refused as a missing required suffix naming the
// bound's key, and without the template a bound needs all three suffixes.
func TestREQ053TemplateCompletesIntervalOrdinalBounds(t *testing.T) {
	wt, compiled, key, path := intervalOrdinalTemplate(t)
	body := func(bounds map[string]any) []byte { return mustJSON(t, completionBody(bounds)) }
	bounds := map[string]any{
		key + "/lower|code":    "at0047",
		key + "/upper|code":    "at0048",
		key + "/upper|value":   "severe, as written",
		key + "/upper|ordinal": nil,
	}
	ordinalAt := func(t *testing.T, comp *rm.Composition, end string) *rm.DVOrdinal {
		t.Helper()
		items, err := rmpath.ItemsAtPath(comp, path)
		if err != nil || len(items) != 1 {
			t.Fatalf("ItemsAtPath(%s) = %d items, %v; want the interval", path, len(items), err)
		}
		iv, ok := items[0].(*rm.DVInterval[rm.DVOrdered])
		if !ok {
			t.Fatalf("%s holds a %T, want *rm.DVInterval[rm.DVOrdered]", path, items[0])
		}
		bound := iv.Lower
		if end == "upper" {
			bound = iv.Upper
		}
		ord, ok := bound.(*rm.DVOrdinal)
		if !ok {
			t.Fatalf("%s %s bound is a %T, want *rm.DVOrdinal", path, end, bound)
		}
		return ord
	}
	for _, format := range []string{"FLAT", "STRUCTURED"} {
		t.Run(format, func(t *testing.T) {
			got := decodeAs(t, format, body(bounds), wt, simplified.WithTemplate(compiled))
			for _, want := range []struct {
				end, code, text string
				value           rm.Integer
			}{
				{"lower", "at0047", "mild", 1},
				{"upper", "at0048", "severe, as written", 2},
			} {
				ord := ordinalAt(t, got, want.end)
				sym := ord.Symbol
				if ord.Value != want.value || sym.Value != want.text || sym.DefiningCode.CodeString != want.code ||
					sym.DefiningCode.TerminologyID.Value != "local" {
					t.Errorf("%s bound = value %d, symbol %q %s::%s; want value %d, symbol %q local::%s",
						want.end, ord.Value, sym.Value, sym.DefiningCode.TerminologyID.Value, sym.DefiningCode.CodeString,
						want.value, want.text, want.code)
				}
			}
		})
	}
	withTemplate := simplified.WithTemplate(compiled)
	for _, tc := range []struct {
		name   string
		bounds map[string]any
		reason string
		opts   []simplified.Option
	}{
		{"without the template", map[string]any{key + "/lower|code": "at0047"}, "DV_ORDINAL missing required |value", nil},
		{"a code outside the pairs", map[string]any{key + "/lower|code": "at0099"}, "DV_ORDINAL missing required |value", []simplified.Option{withTemplate}},
		{
			"a code outside the pairs, with its value",
			map[string]any{key + "/lower|code": "at0099", key + "/lower|value": "x"},
			"DV_ORDINAL missing required |ordinal",
			[]simplified.Option{withTemplate},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			comp, err := simplified.UnmarshalFlat(body(tc.bounds), wt, tc.opts...)
			if err == nil {
				t.Fatalf("UnmarshalFlat(%v) = %s, want it refused: %s", tc.bounds, canonicalComposition(t, comp), tc.reason)
			}
			wantRefusal(t, err, simplified.ErrUnsupportedDatatype, `"`+key+`/lower"`, tc.reason)
		})
	}
}
