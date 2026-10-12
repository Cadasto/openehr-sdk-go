package canjson_test

// unmatched_test.go — REQ-170 (wire.md § REQ-170, Unmatched content on
// decode) for canonical JSON, and the REQ-052 rule that keeps the codec's
// sentinels distinct. A member is unmatched when its name is neither `_type`
// nor an attribute of the RM type its object decodes as. The default ignores
// it; WithUnmatched reports every one, in document order, as a JSON Pointer;
// WithStrict refuses the first with canjson.ErrUnknownMember. Both options
// check the members against the RM's attribute lists after the decode has
// succeeded, so a decode error is reported as itself and a value of a class
// outside the RM (AOM 1.4) is not checked.

import (
	"encoding/json/jsontext"
	"errors"
	"os"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aom/aom14"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// plantedValue is the value every extension member carries. No error text may
// contain it: a refusal names the member's path, never its value.
const plantedValue = "planted-secret-value"

// extPlaceholder marks where an extension member goes in an input template.
const extPlaceholder = "<ext>"

// unmatchedCase is one input carrying extension members. tmpl holds one
// extPlaceholder per entry of names, in the same order; the entries of
// pointers are the JSON Pointers those members sit at. Placeholders appear in
// document order, so pointers is also the order a report must follow.
type unmatchedCase struct {
	name     string
	tmpl     string
	names    []string
	pointers []string
	into     func() any
}

// render fills tmpl's placeholders with the case's extension members, each
// carrying plantedValue, or, with withExt false, removes them: the stripped
// input is what the extended one must decode to.
func (c unmatchedCase) render(t *testing.T, withExt bool) []byte {
	t.Helper()
	parts := strings.Split(c.tmpl, extPlaceholder)
	if len(parts) != len(c.names)+1 || len(c.names) != len(c.pointers) {
		t.Fatalf("%s: %d placeholders, %d names, %d pointers; want one name and one pointer per placeholder", c.name, len(parts)-1, len(c.names), len(c.pointers))
	}
	var b strings.Builder
	for i, p := range parts {
		b.WriteString(p)
		if i < len(c.names) && withExt {
			b.WriteString(strconv.Quote(c.names[i]) + ":" + strconv.Quote(plantedValue) + ",")
		}
	}
	return []byte(b.String())
}

// Shared fragments of the composition template.
const (
	ucText     = `{"_type":"DV_TEXT","value":"n"}`
	ucLanguage = `{"_type":"CODE_PHRASE","terminology_id":{"_type":"TERMINOLOGY_ID","value":"ISO_639-1"},"code_string":"en"}`
	ucTime     = `{"_type":"DV_DATE_TIME","value":"2026-01-01T00:00:00Z"}`
)

// unmatchedCases puts extension members everywhere REQ-170's table reaches in
// canonical JSON: the root of a COMPOSITION, inside an ENTRY, an EVENT and a
// data value (each in a polymorphic slot), in a narrow slot that falls back
// to its declared parent, in an interval and its bound (both a by-value bound
// and a bound that is itself a polymorphic slot), ahead of the `_type` that
// decides its object, in a CODE_PHRASE and the TERMINOLOGY_ID it holds by
// value, and in the RM objects a `Hash` holds. One case has names RFC 6901
// must escape.
//
// No RM attribute reaches a class the generated decoders leave without a
// decode method of its own (rm.TerminologyCode, rm.TerminologyTerm,
// rm.Cardinality, rm.MultiplicityInterval): AOM 1.4 alone holds them, and its
// members are not checked (TestREQ170_ClassesOutsideTheRMAreNotChecked). The
// TERMINOLOGY_ID case is the by-value RM class nearest to that shape.
func unmatchedCases() []unmatchedCase {
	element := func(value string) string {
		return `{"_type":"ELEMENT","name":` + ucText + `,"archetype_node_id":"at0004","value":` + value + `}`
	}
	quantity := func(ext, magnitude string) string {
		return `{"_type":"DV_QUANTITY",` + ext + `"magnitude":` + magnitude + `,"units":"kg"}`
	}
	composition := `{` + extPlaceholder + `"_type":"COMPOSITION","name":` + ucText +
		`,"archetype_node_id":"openEHR-EHR-COMPOSITION.encounter.v1","language":` + ucLanguage +
		`,"territory":{"_type":"CODE_PHRASE","terminology_id":{"_type":"TERMINOLOGY_ID","value":"ISO_3166-1"},"code_string":"GB"}` +
		`,"category":{"_type":"DV_CODED_TEXT","value":"event","defining_code":{"_type":"CODE_PHRASE","terminology_id":{"_type":"TERMINOLOGY_ID","value":"openehr"},"code_string":"433"}}` +
		`,"content":[{"_type":"OBSERVATION",` + extPlaceholder + `"name":` + ucText +
		`,"archetype_node_id":"openEHR-EHR-OBSERVATION.body_weight.v2","language":` + ucLanguage +
		`,"encoding":{"_type":"CODE_PHRASE","terminology_id":{"_type":"TERMINOLOGY_ID","value":"IANA_character-sets"},"code_string":"UTF-8"}` +
		`,"subject":{"_type":"PARTY_SELF"}` +
		`,"data":{"_type":"HISTORY","name":` + ucText + `,"archetype_node_id":"at0002","origin":` + ucTime +
		`,"events":[{"_type":"POINT_EVENT",` + extPlaceholder + `"name":` + ucText + `,"archetype_node_id":"at0003","time":` + ucTime +
		`,"data":{"_type":"ITEM_TREE","name":{` + extPlaceholder + `"value":"tree"},"archetype_node_id":"at0001","items":[` +
		element(`{"_type":"DV_QUANTITY",`+extPlaceholder+`"magnitude":80.5,"units":"kg","normal_range":{"_type":"DV_INTERVAL",`+extPlaceholder+
			`"lower":`+quantity(extPlaceholder, "50")+`,"upper":`+quantity("", "120")+`}}`) + `,` +
		element(`{"_type":"DV_INTERVAL","lower":`+quantity(extPlaceholder, "1")+`,"upper":`+quantity("", "2")+`}`) +
		`]}}]}}]` +
		`,` + extPlaceholder + `"composer":{"_type":"PARTY_SELF"}}`
	item := "/content/0/data/events/0/data/items/"

	return []unmatchedCase{
		{
			name: "composition",
			tmpl: composition,
			names: []string{
				"x-root", "x-entry", "x-event", "x-narrow-slot", "x-value",
				"x-interval", "x-bound", "x-slot-bound", "x-root-tail",
			},
			pointers: []string{
				"/x-root",
				"/content/0/x-entry",
				"/content/0/data/events/0/x-event",
				"/content/0/data/events/0/data/name/x-narrow-slot",
				item + "0/value/x-value",
				item + "0/value/normal_range/x-interval",
				item + "0/value/normal_range/lower/x-bound",
				item + "1/value/lower/x-slot-bound",
				"/x-root-tail",
			},
			into: func() any { return new(rm.Composition) },
		},
		{
			// The CODE_PHRASE's members come before its `_type`, and its
			// TERMINOLOGY_ID, held by value, is written once with its
			// `_type` and once with it elided.
			name: "CODE_PHRASE and TERMINOLOGY_ID",
			tmpl: `{"_type":"DV_CODED_TEXT","value":"event","defining_code":{` + extPlaceholder +
				`"terminology_id":{"_type":"TERMINOLOGY_ID",` + extPlaceholder + `"value":"openehr"},"code_string":"433","_type":"CODE_PHRASE"},` +
				`"language":{"terminology_id":{` + extPlaceholder + `"value":"ISO_639-1"},"code_string":"en"}}`,
			names: []string{"x-code-phrase", "x-terminology-id", "x-elided-terminology-id"},
			pointers: []string{
				"/defining_code/x-code-phrase",
				"/defining_code/terminology_id/x-terminology-id",
				"/language/terminology_id/x-elided-terminology-id",
			},
			into: func() any { return new(rm.DVCodedText) },
		},
		{
			// RESOURCE_DESCRIPTION.details is a Hash whose values the decoder
			// reads as RESOURCE_DESCRIPTION_ITEM; original_author is a Hash
			// of strings.
			name: "Hash of RM objects",
			tmpl: `{"_type":"RESOURCE_DESCRIPTION","lifecycle_state":"published","original_author":{"name":"a"},` +
				`"details":{"en":{` + extPlaceholder + `"language":{"terminology_id":{"value":"ISO_639-1"},"code_string":"en"},"purpose":"p"},` +
				`"de":{"language":{` + extPlaceholder + `"terminology_id":{"value":"ISO_639-1"},"code_string":"de"},"purpose":"q"}},` +
				extPlaceholder + `"other_contributors":["c"]}`,
			names:    []string{"x-entry", "x-entry-language", "x-root"},
			pointers: []string{"/details/en/x-entry", "/details/de/language/x-entry-language", "/x-root"},
			into:     func() any { return new(rm.ResourceDescription) },
		},
		{
			// RFC 6901 § 3: `~` is written `~0` and `/` is written `~1`.
			name:     "names RFC 6901 escapes",
			tmpl:     `{"_type":"DV_TEXT",` + extPlaceholder + extPlaceholder + `"value":"x"}`,
			names:    []string{"a~b/c", "~1"},
			pointers: []string{"/a~0b~1c", "/~01"},
			into:     func() any { return new(rm.DVText) },
		},
	}
}

// decodeCollecting decodes in into a fresh value with WithUnmatched (and the
// extra options) and returns the value, the pointers reported and the error.
func decodeCollecting(c unmatchedCase, in []byte, opts ...canjson.DecoderOption) (any, []string, error) {
	var got []string
	v := c.into()
	opts = append(opts, canjson.WithUnmatched(func(p string) { got = append(got, p) }))
	err := canjson.Unmarshal(in, v, opts...)
	return v, got, err
}

// assertStrictRefusal checks every facet REQ-170 gives a strict refusal: it
// wraps canjson.ErrUnknownMember, one step from the sentinel (so its message
// names one pointer), names the first unmatched member's pointer, carries no
// value from the input, and matches neither of the codec's other two
// sentinels (REQ-052) nor the dispatch envelope.
func assertStrictRefusal(t *testing.T, call string, err error, first string) {
	t.Helper()
	if !errors.Is(err, canjson.ErrUnknownMember) {
		t.Fatalf("%s err = %v; want errors.Is(_, canjson.ErrUnknownMember)", call, err)
	}
	if got := errors.Unwrap(err); got != canjson.ErrUnknownMember { //nolint:errorlint // identity by design: the refusal is one step from the sentinel
		t.Errorf("%s: errors.Unwrap(err) = %v; want canjson.ErrUnknownMember: the refusal comes back alone, not inside the decoder's envelopes", call, got)
	}
	if !strings.Contains(err.Error(), strconv.Quote(first)) {
		t.Errorf("%s err = %q; want it to name the first unmatched member %s", call, err, strconv.Quote(first))
	}
	if strings.Contains(err.Error(), plantedValue) {
		t.Errorf("%s err = %q; it carries the member's value %q, want the path alone", call, err, plantedValue)
	}
	if errors.Is(err, canjson.ErrInvalidShape) || errors.Is(err, canjson.ErrInvalidValue) {
		t.Errorf("%s err = %v; an unknown member must match neither canjson.ErrInvalidShape nor canjson.ErrInvalidValue", call, err)
	}
	if de, ok := errors.AsType[*canjson.DecodeError](err); ok && de != nil {
		t.Errorf("%s err = %v; an unknown member is not a dispatch failure, want no *canjson.DecodeError", call, err)
	}
}

// TestREQ170_UnmatchedMembersDefaultReportStrict drives each case through the
// three modes on canjson.Unmarshal.
//
//   - Default: the input decodes, with no error, to the same value as the
//     input without its extension members.
//   - Report: the function receives each extension member once, in document
//     order, by its JSON Pointer from the root, and the decode returns the
//     same value as the default.
//   - Strict: the decode fails at the first extension member (see
//     assertStrictRefusal); the stripped input passes.
//   - Report and strict together: the function sees the first member, then
//     the decode is refused.
func TestREQ170_UnmatchedMembersDefaultReportStrict(t *testing.T) {
	for _, c := range unmatchedCases() {
		t.Run(c.name, func(t *testing.T) {
			in, stripped := c.render(t, true), c.render(t, false)

			want := c.into()
			if err := canjson.Unmarshal(stripped, want); err != nil {
				t.Fatalf("canjson.Unmarshal(stripped input) = %v; the case's base input must decode", err)
			}

			t.Run("default", func(t *testing.T) {
				got := c.into()
				if err := canjson.Unmarshal(in, got); err != nil {
					t.Fatalf("canjson.Unmarshal = %v; want nil: unmatched members are ignored by default", err)
				}
				if !reflect.DeepEqual(got, want) {
					t.Errorf("canjson.Unmarshal decoded\n%#v\nwant the value of the input without its extension members\n%#v", got, want)
				}
			})

			t.Run("report", func(t *testing.T) {
				got, pointers, err := decodeCollecting(c, in)
				if err != nil {
					t.Fatalf("canjson.Unmarshal(WithUnmatched) = %v; want nil: the report option does not refuse", err)
				}
				if !slices.Equal(pointers, c.pointers) {
					t.Errorf("WithUnmatched reported %q; want %q (each member once, in document order)", pointers, c.pointers)
				}
				if !reflect.DeepEqual(got, want) {
					t.Errorf("canjson.Unmarshal(WithUnmatched) decoded\n%#v\nwant the default's value\n%#v", got, want)
				}
			})

			t.Run("strict", func(t *testing.T) {
				err := canjson.Unmarshal(in, c.into(), canjson.WithStrict())
				assertStrictRefusal(t, "canjson.Unmarshal(WithStrict)", err, c.pointers[0])
				if err := canjson.Unmarshal(stripped, c.into(), canjson.WithStrict()); err != nil {
					t.Errorf("canjson.Unmarshal(stripped input, WithStrict) = %v; want nil: nothing in it is unmatched", err)
				}
			})

			t.Run("report and strict", func(t *testing.T) {
				_, pointers, err := decodeCollecting(c, in, canjson.WithStrict())
				assertStrictRefusal(t, "canjson.Unmarshal(WithStrict, WithUnmatched)", err, c.pointers[0])
				if !slices.Equal(pointers, c.pointers[:1]) {
					t.Errorf("WithUnmatched under WithStrict reported %q; want %q, the member the decode stops at", pointers, c.pointers[:1])
				}
			})
		})
	}
}

// TestREQ170_DecoderUnmatchedOptions drives the streaming entry point: a
// Decoder takes the same options, and the pointers it reports are relative to
// the value being decoded, so the second value of a stream reports from its
// own root.
func TestREQ170_DecoderUnmatchedOptions(t *testing.T) {
	stream := `{"_type":"DV_TEXT","x-one":"` + plantedValue + `","value":"a"}` +
		`{"_type":"DV_QUANTITY","magnitude":1,"units":"kg","x-two":{"nested":"` + plantedValue + `"}}`

	var got []string
	dec := canjson.NewDecoder(strings.NewReader(stream), canjson.WithUnmatched(func(p string) { got = append(got, p) }))
	var text rm.DVText
	var qty rm.DVQuantity
	if err := dec.Decode(&text); err != nil {
		t.Fatalf("Decode(first value) = %v; want nil", err)
	}
	if err := dec.Decode(&qty); err != nil {
		t.Fatalf("Decode(second value) = %v; want nil", err)
	}
	if want := []string{"/x-one", "/x-two"}; !slices.Equal(got, want) {
		t.Errorf("Decoder with WithUnmatched reported %q; want %q", got, want)
	}
	if text.Value != "a" || qty.Units != "kg" {
		t.Errorf("Decoder decoded %+v and %+v; want the members it reads filled", text, qty)
	}

	strict := canjson.NewDecoder(strings.NewReader(stream), canjson.WithStrict())
	err := strict.Decode(new(rm.DVText))
	assertStrictRefusal(t, "Decoder(WithStrict).Decode", err, "/x-one")
}

// TestREQ170_DecoderWithOptionsReadsTheValueWhole pins what Decoder.Decode's
// documentation says about a decoder given either option: it reads the whole
// next value before decoding it. So malformed bytes later in a value are
// reported ahead of a shape failure earlier in it, where a decoder without
// options reports the shape failure first (REQ-052's malformed-JSON
// exclusion), and a shape failure's byte offset counts from the start of the
// value. canjson.Unmarshal has the whole input already and decodes as it does
// by default.
func TestREQ170_DecoderWithOptionsReadsTheValueWhole(t *testing.T) {
	const (
		shapeFirst = `{"_type":"DV_QUANTITY","magnitude":"abc","units":"kg" x}`
		first      = `{"_type":"DV_TEXT","value":"a"}   `
		second     = `{"_type":"DV_QUANTITY","magnitude":"abc","units":"kg"}`
	)
	// secondShapeFailure decodes the second value of first+second and returns
	// the shape failure it reports.
	secondShapeFailure := func(t *testing.T, opts ...canjson.DecoderOption) error {
		t.Helper()
		dec := canjson.NewDecoder(strings.NewReader(first+second), opts...)
		if err := dec.Decode(new(rm.DVText)); err != nil {
			t.Fatalf("Decode(first value) = %v; want nil", err)
		}
		err := dec.Decode(new(rm.DVQuantity))
		if !errors.Is(err, canjson.ErrInvalidShape) {
			t.Fatalf("Decode(second value) err = %v; want canjson.ErrInvalidShape", err)
		}
		return err
	}
	// The failing literal starts at this offset in the second value.
	inValue := strings.Index(second, `"abc"`)

	t.Run("no options", func(t *testing.T) {
		if want := "after offset " + strconv.Itoa(len(first)+inValue); !strings.Contains(secondShapeFailure(t).Error(), want) {
			t.Errorf("Decoder().Decode(second value) err names no %q, counted from the start of the stream", want)
		}
	})
	opts := map[string][]canjson.DecoderOption{
		"WithUnmatched": {canjson.WithUnmatched(func(string) {})},
		"WithStrict":    {canjson.WithStrict()},
	}
	for name, o := range opts {
		t.Run(name, func(t *testing.T) {
			err := canjson.NewDecoder(strings.NewReader(shapeFirst), o...).Decode(new(rm.DVQuantity))
			if se, ok := errors.AsType[*jsontext.SyntacticError](err); !ok || se == nil || errors.Is(err, canjson.ErrInvalidShape) {
				t.Errorf("Decoder(%s).Decode(%s) err = %v; want the malformed bytes reported, with no canjson.ErrInvalidShape", name, shapeFirst, err)
			}
			if err := canjson.Unmarshal([]byte(shapeFirst), new(rm.DVQuantity), o...); !errors.Is(err, canjson.ErrInvalidShape) {
				t.Errorf("canjson.Unmarshal(%s, %s) err = %v; want the shape failure, as by default", shapeFirst, name, err)
			}
			if want := "after offset " + strconv.Itoa(inValue); !strings.Contains(secondShapeFailure(t, o...).Error(), want) {
				t.Errorf("Decoder(%s).Decode(second value) err names no %q, counted from the start of the value", name, want)
			}
		})
	}
}

// TestREQ170_ConcurrentDecodesKeepTheirOwnReports runs reporting decodes in
// parallel, each with its own report function and its own extension member,
// and requires each to see exactly its own member, slots included.
func TestREQ170_ConcurrentDecodesKeepTheirOwnReports(t *testing.T) {
	for i := range 8 {
		name := "x-decode-" + strconv.Itoa(i)
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			in := []byte(`{"_type":"ELEMENT","archetype_node_id":"at1",` +
				`"value":{"_type":"DV_QUANTITY","magnitude":1,"units":"kg",` + strconv.Quote(name) + `:1}}`)
			want := []string{"/value/" + name}
			for range 200 {
				var got []string
				err := canjson.Unmarshal(in, new(rm.Element), canjson.WithUnmatched(func(p string) { got = append(got, p) }))
				if err != nil {
					t.Fatalf("canjson.Unmarshal(WithUnmatched) = %v; want nil", err)
				}
				if !slices.Equal(got, want) {
					t.Fatalf("WithUnmatched reported %q; want %q: a concurrent decode's members must not reach this one", got, want)
				}
			}
		})
	}
}

// modes are the three decode modes, each as the options it passes and the
// pointers it collects.
func modes(reported *[]string) map[string][]canjson.DecoderOption {
	report := canjson.WithUnmatched(func(p string) { *reported = append(*reported, p) })
	return map[string][]canjson.DecoderOption{
		"default": nil,
		"report":  {report},
		"strict":  {canjson.WithStrict(), report},
	}
}

// TestREQ170_UnknownTypeFailsInEveryMode pins that a `_type` naming no RM
// class is a dispatch failure in every mode, never an unmatched member: the
// slot cannot be decoded at all, so nothing under it is reported either.
func TestREQ170_UnknownTypeFailsInEveryMode(t *testing.T) {
	const in = `{"_type":"ELEMENT","name":{"_type":"DV_TEXT","value":"n"},"archetype_node_id":"at1",` +
		`"value":{"_type":"NO_SUCH_CLASS","x-ext":"` + plantedValue + `"}}`
	var reported []string
	for mode, opts := range modes(&reported) {
		t.Run(mode, func(t *testing.T) {
			reported = nil
			err := canjson.Unmarshal([]byte(in), new(rm.Element), opts...)
			if !errors.Is(err, typereg.ErrUnknownType) {
				t.Fatalf("canjson.Unmarshal(%s) err = %v; want errors.Is(_, typereg.ErrUnknownType)", mode, err)
			}
			if errors.Is(err, canjson.ErrUnknownMember) {
				t.Errorf("canjson.Unmarshal(%s) err = %v; an unknown `_type` is a dispatch failure, not an unknown member", mode, err)
			}
			if len(reported) != 0 {
				t.Errorf("canjson.Unmarshal(%s) reported %q; want nothing: the slot is never decoded", mode, reported)
			}
		})
	}
}

// TestREQ170_TypeAtAConcretePositionFailsAsDispatch pins that an unknown or
// mismatched `_type` where the decode target fixes the class, at the root and
// at a field held by value, fails in all three modes with the dispatch
// failure the decoder reports for it (canjson.DecodeError on `/_type`,
// typereg.ErrTypeMismatch), not with canjson.ErrUnknownMember, and reports
// nothing. Each object also carries an extension member ahead of its
// `_type`, which a check that ran before the decode, or in its place, would
// reach first.
func TestREQ170_TypeAtAConcretePositionFailsAsDispatch(t *testing.T) {
	cases := []struct {
		name, in string
		into     func() any
	}{
		{
			name: "unknown at the root",
			in:   `{"x-ext":"` + plantedValue + `","_type":"NO_SUCH_CLASS","value":"v"}`,
			into: func() any { return new(rm.DVText) },
		},
		{
			name: "mismatched at the root",
			in:   `{"x-ext":"` + plantedValue + `","_type":"DV_QUANTITY","value":"v"}`,
			into: func() any { return new(rm.DVText) },
		},
		{
			name: "unknown at a by-value field",
			in:   `{"_type":"CODE_PHRASE","code_string":"433","terminology_id":{"x-ext":"` + plantedValue + `","_type":"NO_SUCH_CLASS","value":"openehr"}}`,
			into: func() any { return new(rm.CodePhrase) },
		},
		{
			name: "mismatched at a by-value field",
			in:   `{"_type":"CODE_PHRASE","code_string":"433","terminology_id":{"x-ext":"` + plantedValue + `","_type":"DV_TEXT","value":"openehr"}}`,
			into: func() any { return new(rm.CodePhrase) },
		},
	}
	for _, tc := range cases {
		var reported []string
		for mode, opts := range modes(&reported) {
			t.Run(tc.name+"/"+mode, func(t *testing.T) {
				reported = nil
				err := canjson.Unmarshal([]byte(tc.in), tc.into(), opts...)
				de, ok := errors.AsType[*canjson.DecodeError](err)
				if !ok || de == nil || !errors.Is(err, typereg.ErrTypeMismatch) {
					t.Fatalf("canjson.Unmarshal(%s, %s) err = %v; want a *canjson.DecodeError wrapping typereg.ErrTypeMismatch", tc.in, mode, err)
				}
				if errors.Is(err, canjson.ErrUnknownMember) {
					t.Errorf("canjson.Unmarshal(%s, %s) err = %v; the `_type` is a dispatch failure, not an unknown member", tc.in, mode, err)
				}
				if strings.Contains(err.Error(), plantedValue) {
					t.Errorf("canjson.Unmarshal(%s, %s) err = %q; it carries the extension member's value", tc.in, mode, err)
				}
				if len(reported) != 0 {
					t.Errorf("canjson.Unmarshal(%s, %s) reported %q; want nothing: a failed decode is not checked", tc.in, mode, reported)
				}
			})
		}
	}
}

// TestREQ170_StrictNeverRefusesAnAbsentMember pins that strict covers
// unmatched members only: an absent member, mandatory or not, and an interval
// boundary flag REQ-169 defaults, decode under WithStrict, and the flags take
// their defaults.
func TestREQ170_StrictNeverRefusesAnAbsentMember(t *testing.T) {
	t.Run("interval without flags", func(t *testing.T) {
		const in = `{"_type":"DV_INTERVAL","lower":{"_type":"DV_QUANTITY","magnitude":1,"units":"kg"},` +
			`"upper":{"_type":"DV_QUANTITY","magnitude":2,"units":"kg"}}`
		var iv rm.DVInterval[rm.DVQuantity]
		if err := canjson.Unmarshal([]byte(in), &iv, canjson.WithStrict()); err != nil {
			t.Fatalf("canjson.Unmarshal(%s, WithStrict) = %v; want nil: an absent flag is not unmatched", in, err)
		}
		if !iv.LowerIncluded || !iv.UpperIncluded || iv.LowerUnbounded || iv.UpperUnbounded {
			t.Errorf("flags = included %v/%v, unbounded %v/%v; want both included and neither unbounded (REQ-169)",
				iv.LowerIncluded, iv.UpperIncluded, iv.LowerUnbounded, iv.UpperUnbounded)
		}
	})
	t.Run("composition without its mandatory members", func(t *testing.T) {
		const in = `{"_type":"COMPOSITION"}`
		if err := canjson.Unmarshal([]byte(in), new(rm.Composition), canjson.WithStrict()); err != nil {
			t.Errorf("canjson.Unmarshal(%s, WithStrict) = %v; want nil: an absent mandatory member is not unmatched", in, err)
		}
	})
}

// TestREQ170_ClassesOutsideTheRMAreNotChecked pins the canonical-JSON scope
// of the options: a value of a class outside the RM, an AOM 1.4 archetype or
// constraint, decodes as by default and none of its members is checked, the
// RM values it holds included (its ARCHETYPE_ID, and its original_language,
// an rm.TerminologyCode). Each input carries extension members at several
// depths; the report sees none, strict refuses none, and the value is the
// default's.
func TestREQ170_ClassesOutsideTheRMAreNotChecked(t *testing.T) {
	cases := []struct {
		name, in string
		into     func() any
	}{
		{
			name: "archetype",
			in: `{"_type":"ARCHETYPE","x-archetype":"` + plantedValue + `",` +
				`"archetype_id":{"value":"openEHR-EHR-OBSERVATION.x.v1","x-id":1},"concept":"at0000",` +
				`"original_language":{"terminology_id":"ISO_639-1","code_string":"en","x-language":1},` +
				`"definition":{"_type":"C_COMPLEX_OBJECT","rm_type_name":"OBSERVATION","node_id":"at0000","x-definition":1}}`,
			into: func() any { return new(aom14.Archetype) },
		},
		{
			// A C_COMPLEX_OBJECT, its `occurrences` (the abstract BASE
			// Interval), a C_SINGLE_ATTRIBUTE in a polymorphic slot, and that
			// attribute's `existence` interval.
			name: "constraint",
			in: `{"_type":"C_COMPLEX_OBJECT","x-object":1,"rm_type_name":"OBSERVATION","node_id":"at0000",` +
				`"occurrences":{"_type":"MULTIPLICITY_INTERVAL","x-occurrences":1,"lower":1,"upper":1},` +
				`"attributes":[{"_type":"C_SINGLE_ATTRIBUTE","x-attribute":1,"rm_attribute_name":"data",` +
				`"existence":{"x-existence":1,"lower":1,"upper":1}}]}`,
			into: func() any { return new(aom14.CComplexObject) },
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			want := tc.into()
			if err := canjson.Unmarshal([]byte(tc.in), want); err != nil {
				t.Fatalf("canjson.Unmarshal(%s) = %v; want nil", tc.in, err)
			}
			var reported []string
			for mode, opts := range modes(&reported) {
				reported = nil
				got := tc.into()
				if err := canjson.Unmarshal([]byte(tc.in), got, opts...); err != nil {
					t.Errorf("canjson.Unmarshal(%s) = %v; want nil: a class outside the RM is not checked", mode, err)
				}
				if len(reported) != 0 {
					t.Errorf("canjson.Unmarshal(%s) reported %q; want nothing", mode, reported)
				}
				if !reflect.DeepEqual(got, want) {
					t.Errorf("canjson.Unmarshal(%s) decoded\n%#v\nwant the default's value\n%#v", mode, got, want)
				}
			}
		})
	}
}

// TestREQ170_RootTargetsAreChecked pins that the walk starts from the decode
// target's own RM shape (REQ-170 *Scope*), so every object that decodes as an
// RM class is checked: the elements of a slice and the values of a map of RM
// values, eliding `_type` or not; the bounds of a generic target, which binds
// them to a class; the payload of an ORIGINAL_VERSION or the bounds of a BASE
// interval whose type argument is an RM class or a polymorphic RM interface,
// read by their own `_type`; and a narrow RM interface, which an elided
// `_type` decodes as its parent class.
func TestREQ170_RootTargetsAreChecked(t *testing.T) {
	cases := []struct {
		name, in string
		into     func() any
		want     []string
	}{
		{
			name: "slice",
			in:   `[{"value":"a","x-a":"` + plantedValue + `"},{"_type":"DV_TEXT","value":"b","x-b":1}]`,
			into: func() any { return new([]rm.DVText) },
			want: []string{"/0/x-a", "/1/x-b"},
		},
		{
			name: "map",
			in:   `{"k":{"value":"a","x-a":"` + plantedValue + `"}}`,
			into: func() any { return new(map[string]rm.DVText) },
			want: []string{"/k/x-a"},
		},
		{
			name: "generic bound to a class",
			in:   `{"_type":"DV_INTERVAL","lower":{"magnitude":1,"units":"kg","x-a":"` + plantedValue + `"},"upper":{"magnitude":2,"units":"kg"}}`,
			into: func() any { return new(rm.DVInterval[rm.DVQuantity]) },
			want: []string{"/lower/x-a"},
		},
		{
			name: "payload bound to a class",
			in:   `{"_type":"ORIGINAL_VERSION","data":{"value":"a","x-a":"` + plantedValue + `"}}`,
			into: func() any { return new(rm.OriginalVersion[rm.DVText]) },
			want: []string{"/data/x-a"},
		},
		{
			name: "payload bound to an abstract RM interface",
			in:   `{"_type":"ORIGINAL_VERSION","data":{"_type":"DV_TEXT","value":"a","x-a":"` + plantedValue + `"}}`,
			into: func() any { return new(rm.OriginalVersion[rm.DataValue]) },
			want: []string{"/data/x-a"},
		},
		{
			name: "payload bound to another abstract RM interface",
			in: `{"_type":"ORIGINAL_VERSION","data":{"_type":"ADMIN_ENTRY","name":{"value":"n"},"archetype_node_id":"at1",` +
				`"x-a":"` + plantedValue + `"}}`,
			into: func() any { return new(rm.OriginalVersion[rm.ContentItem]) },
			want: []string{"/data/x-a"},
		},
		{
			name: "BASE interval bound to an abstract RM interface",
			in:   `{"_type":"Point_interval","lower":{"_type":"DV_COUNT","magnitude":1,"x-a":"` + plantedValue + `"}}`,
			into: func() any { return new(rm.PointInterval[rm.DVOrdered]) },
			want: []string{"/lower/x-a"},
		},
		{
			name: "narrow RM interface",
			in:   `{"value":"a","x-a":"` + plantedValue + `"}`,
			into: func() any { return new(rm.DVTextLike) },
			want: []string{"/x-a"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got []string
			if err := canjson.Unmarshal([]byte(tc.in), tc.into(), canjson.WithUnmatched(func(p string) { got = append(got, p) })); err != nil {
				t.Fatalf("canjson.Unmarshal(%s, WithUnmatched) = %v; want nil", tc.in, err)
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("WithUnmatched reported %q; want %q", got, tc.want)
			}
			err := canjson.Unmarshal([]byte(tc.in), tc.into(), canjson.WithStrict())
			assertStrictRefusal(t, "canjson.Unmarshal(WithStrict)", err, tc.want[0])
		})
	}
}

// callerStruct is a caller's own type, with a member named like an extension
// and an RM value inside it.
type callerStruct struct {
	Type  string    `json:"_type"`
	Value string    `json:"value"`
	XA    int       `json:"x-a"`
	Text  rm.DVText `json:"text"`
}

// TestREQ170_NonRMTargetsAreNotChecked pins that the walk starts only from a
// target whose own shape is an RM class (REQ-170 *Scope*): an any, a
// map[string]any and a caller's own struct, which reads the member and holds
// an RM value whose extension member the walk does not reach, decode an
// RM-looking input in every mode, report nothing, and get the default's value.
//
// A BASE interval is not an RM class either: the BMM lists it among its
// primitive types. An rm.Interval[rm.Integer] is not checked as the target,
// nor as the value of a member, here an ORIGINAL_VERSION's data bound to it.
// The controls show that its descendants are checked: the same members under
// an rm.PointInterval[rm.Integer] and an rm.DVInterval[rm.DVCount] report the
// extension member and fail strict.
func TestREQ170_NonRMTargetsAreNotChecked(t *testing.T) {
	const (
		rmLooking = `{"_type":"DV_TEXT","value":"v","x-a":1,"text":{"value":"t","x-b":1}}`
		interval  = `{"lower":1,"upper":2,"x-ext":"` + plantedValue + `"}`
	)
	targets := map[string]struct {
		in   string
		into func() any
	}{
		"any":                    {in: rmLooking, into: func() any { return new(any) }},
		"map[string]any":         {in: rmLooking, into: func() any { return new(map[string]any) }},
		"caller struct":          {in: rmLooking, into: func() any { return new(callerStruct) }},
		"BASE interval":          {in: interval, into: func() any { return new(rm.Interval[rm.Integer]) }},
		"BASE interval as value": {in: `{"_type":"ORIGINAL_VERSION","data":` + interval + `}`, into: func() any { return new(rm.OriginalVersion[rm.Interval[rm.Integer]]) }},
	}
	for name, target := range targets {
		t.Run(name, func(t *testing.T) {
			want := target.into()
			if err := canjson.Unmarshal([]byte(target.in), want); err != nil {
				t.Fatalf("canjson.Unmarshal(%s) = %v; want nil", target.in, err)
			}
			var reported []string
			for mode, opts := range modes(&reported) {
				reported = nil
				got := target.into()
				if err := canjson.Unmarshal([]byte(target.in), got, opts...); err != nil {
					t.Errorf("canjson.Unmarshal(%s, %s) = %v; want nil: the target decodes as no RM type", target.in, mode, err)
				}
				if len(reported) != 0 {
					t.Errorf("canjson.Unmarshal(%s, %s) reported %q; want nothing", target.in, mode, reported)
				}
				if !reflect.DeepEqual(got, want) {
					t.Errorf("canjson.Unmarshal(%s, %s) decoded %#v; want the default's %#v", target.in, mode, got, want)
				}
			}
		})
	}

	controls := map[string]struct {
		in   string
		into func() any
	}{
		"Point_interval": {in: interval, into: func() any { return new(rm.PointInterval[rm.Integer]) }},
		"DV_INTERVAL":    {in: `{"lower":{"magnitude":1},"upper":{"magnitude":2},"x-ext":"` + plantedValue + `"}`, into: func() any { return new(rm.DVInterval[rm.DVCount]) }},
	}
	for name, control := range controls {
		t.Run("control "+name, func(t *testing.T) {
			var reported []string
			if err := canjson.Unmarshal([]byte(control.in), control.into(), canjson.WithUnmatched(func(p string) { reported = append(reported, p) })); err != nil {
				t.Fatalf("canjson.Unmarshal(%s, WithUnmatched) = %v; want nil", control.in, err)
			}
			if want := []string{"/x-ext"}; !slices.Equal(reported, want) {
				t.Errorf("WithUnmatched reported %q; want %q: %s is a class and is checked", reported, want, name)
			}
			err := canjson.Unmarshal([]byte(control.in), control.into(), canjson.WithStrict())
			assertStrictRefusal(t, "canjson.Unmarshal(WithStrict)", err, "/x-ext")
		})
	}
}

// TestREQ170_NilOptionsTakeTheDefaultPath pins that canjson.Unmarshal given
// only nil options decodes exactly as with none: no member is checked, the
// value is the default's, and the call allocates no more than one with no
// options.
func TestREQ170_NilOptionsTakeTheDefaultPath(t *testing.T) {
	in := []byte(`{"_type":"DV_QUANTITY","magnitude":80.5,"units":"kg","x-ext":"` + plantedValue + `"}`)
	var want, got rm.DVQuantity
	if err := canjson.Unmarshal(in, &want); err != nil {
		t.Fatalf("canjson.Unmarshal = %v; want nil", err)
	}
	if err := canjson.Unmarshal(in, &got, nil, nil); err != nil {
		t.Fatalf("canjson.Unmarshal(nil, nil) = %v; want nil: a nil option asks for nothing", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("canjson.Unmarshal(nil, nil) decoded %+v; want the default's %+v", got, want)
	}
	if raceEnabled {
		t.Skip("allocation counts vary under the race detector")
	}
	none := testing.AllocsPerRun(50, func() { _ = canjson.Unmarshal(in, new(rm.DVQuantity)) })
	nils := testing.AllocsPerRun(50, func() { _ = canjson.Unmarshal(in, new(rm.DVQuantity), nil) })
	if nils > none {
		t.Errorf("canjson.Unmarshal(nil) allocates %v times per call; want at most the %v of a call with no options", nils, none)
	}
}

// maxDefaultDecodeAllocs is what a default canjson.Unmarshal of a small
// DV_QUANTITY into a fresh value allocated before the unmatched-member options
// existed, the target included (the figure BenchmarkDecodeDVQuantity reports).
// The options run after the decode, so the default decode pays nothing for
// them.
const maxDefaultDecodeAllocs = 4

// TestREQ170_DefaultDecodeAllocatesAsBefore pins the cost of the options on a
// decode that passes none: the default path allocates no more than it did
// before they existed. A check carried inside the decoder, such as a
// per-object wrapper or a tracking hook, adds to it: the decoder-side tracking
// these options first shipped with cost one more allocation here.
func TestREQ170_DefaultDecodeAllocatesAsBefore(t *testing.T) {
	if raceEnabled {
		t.Skip("allocation counts vary under the race detector")
	}
	in := []byte(`{"_type":"DV_QUANTITY","magnitude":80.5,"units":"kg"}`)
	if got := testing.AllocsPerRun(100, func() { _ = canjson.Unmarshal(in, new(rm.DVQuantity)) }); got > maxDefaultDecodeAllocs {
		t.Errorf("canjson.Unmarshal(DV_QUANTITY) allocates %v times per call; want at most %d, the default decode's cost before the options", got, maxDefaultDecodeAllocs)
	}
}

// TestREQ170_RegisteredTypeCensus decodes, for every registered library type,
// a body carrying its `_type` and one extension member. A type of the rm
// package is an RM class, and the report names exactly that member; a type of
// the aom14 package is outside the RM, and it names nothing. The side is
// decided by the Go package, not by the RM metadata the check itself reads,
// so an RM type that metadata misses fails here. It is the control that the
// check reaches every RM class the registry decodes, and no class outside
// the RM.
func TestREQ170_RegisteredTypeCensus(t *testing.T) {
	const (
		rmPackage  = "github.com/cadasto/openehr-sdk-go/openehr/rm"
		aomPackage = "github.com/cadasto/openehr-sdk-go/openehr/aom/aom14"
	)
	rmChecked, outside := 0, 0
	for _, name := range typereg.Default.Names() {
		ctor, _ := typereg.Default.Lookup(name)
		rt := reflect.TypeOf(ctor())
		for rt.Kind() == reflect.Pointer {
			rt = rt.Elem()
		}
		var want []string
		switch rt.PkgPath() {
		case rmPackage:
			want = []string{"/x-census"}
			rmChecked++
		case aomPackage:
			outside++
		default:
			continue // a test fixture registered elsewhere; not a shipped type
		}
		in := `{"_type":"` + name + `","x-census":"` + plantedValue + `"}`
		var got []string
		err := canjson.Unmarshal([]byte(in), ctor(), canjson.WithUnmatched(func(p string) { got = append(got, p) }))
		if err != nil {
			t.Errorf("%s: canjson.Unmarshal(%s, WithUnmatched) = %v; want nil", name, in, err)
			continue
		}
		if !slices.Equal(got, want) {
			t.Errorf("%s (Go package %s): WithUnmatched reported %q; want %q", name, rt.PkgPath(), got, want)
		}
	}
	if rmChecked < 100 {
		t.Errorf("census covered %d RM types; want the full RM inventory", rmChecked)
	}
	if outside < 20 {
		t.Errorf("census covered %d types outside the RM; want the AOM 1.4 inventory", outside)
	}
}

// corpusUnmatched is every unmatched member the vendored canonical-JSON corpus
// carries, by fixture. Each one names no attribute of its object's RM type in
// the pinned RM 1.2.0 schema: DV_PARSABLE declares `size` as a function, not
// an attribute; EVALUATION (with CARE_ENTRY and ENTRY) declares no
// `time_asserted`; and `type` is not the `_type` discriminator. The corpus is
// vendored as-is, so the test pins the set rather than editing the fixtures.
var corpusUnmatched = map[string][]string{
	"compositions/Test_dv_ordinal_open_constraint.v0.json": {
		"/content/0/data/events/0/data/items/0/value/symbol/type",
		"/content/0/data/events/1/data/items/0/value/symbol/type",
		"/content/0/data/events/2/data/items/0/value/symbol/type",
	},
	"compositions/clinical_notes.v0.json":      {"/content/2/activities/0/timing/size"},
	"compositions/family_history.v.1.2.3.json": {"/content/0/time_asserted"},
}

// TestREQ170_CorpusUnderReportAndStrict decodes the whole canonical-JSON
// corpus with the report option and pins the exact set of unmatched members
// (corpusUnmatched). Every other fixture decodes under strict; each pinned one
// is refused at its first member.
func TestREQ170_CorpusUnderReportAndStrict(t *testing.T) {
	rels := listFixtures(t)
	if len(rels) == 0 {
		t.Fatal("the corpus listed no canonical-JSON fixture")
	}
	seen := map[string]bool{}
	for _, rel := range rels {
		t.Run(rel.Rel, func(t *testing.T) {
			raw, err := os.ReadFile(fixtures.ResolveCompositionJSON(rel))
			if err != nil {
				t.Fatal(err)
			}
			factory := fixtureFactory(t, rel)
			var got []string
			if err := canjson.Unmarshal(raw, factory(), canjson.WithUnmatched(func(p string) { got = append(got, p) })); err != nil {
				t.Fatalf("canjson.Unmarshal(WithUnmatched) = %v; want nil", err)
			}
			want := corpusUnmatched[rel.Rel]
			seen[rel.Rel] = want != nil
			if !slices.Equal(got, want) {
				t.Errorf("WithUnmatched reported %q; want %q", got, want)
			}
			err = canjson.Unmarshal(raw, factory(), canjson.WithStrict())
			if want == nil {
				if err != nil {
					t.Errorf("canjson.Unmarshal(WithStrict) = %v; want nil: the fixture carries no unmatched member", err)
				}
				return
			}
			assertStrictRefusal(t, "canjson.Unmarshal(WithStrict)", err, want[0])
		})
	}
	for rel := range corpusUnmatched {
		if !seen[rel] {
			t.Errorf("corpusUnmatched pins %s, which the corpus no longer lists: remove the entry", rel)
		}
	}
}

// TestREQ170_SentinelsStayDistinct pins the REQ-052 rule REQ-170 extends:
// canjson.ErrUnknownMember, canjson.ErrInvalidShape and canjson.ErrInvalidValue
// are distinct values, and an error raised for one matches neither of the
// others under errors.Is. The unknown-member side is asserted on every strict
// refusal above; here a shape failure under strict and an encode failure must
// not match canjson.ErrUnknownMember.
func TestREQ170_SentinelsStayDistinct(t *testing.T) {
	sentinels := map[string]error{
		"ErrUnknownMember": canjson.ErrUnknownMember,
		"ErrInvalidShape":  canjson.ErrInvalidShape,
		"ErrInvalidValue":  canjson.ErrInvalidValue,
	}
	for a, ea := range sentinels {
		for b, eb := range sentinels {
			if a != b && errors.Is(ea, eb) {
				t.Errorf("errors.Is(canjson.%s, canjson.%s) = true; want the sentinels distinct", a, b)
			}
		}
	}

	const shape = `{"_type":"DV_QUANTITY","magnitude":"abc","units":"kg"}`
	err := canjson.Unmarshal([]byte(shape), new(rm.DVQuantity), canjson.WithStrict())
	if !errors.Is(err, canjson.ErrInvalidShape) || errors.Is(err, canjson.ErrUnknownMember) {
		t.Errorf("canjson.Unmarshal(%s, WithStrict) err = %v; want canjson.ErrInvalidShape and not canjson.ErrUnknownMember", shape, err)
	}

	_, err = canjson.Marshal(make(chan int))
	if !errors.Is(err, canjson.ErrInvalidValue) || errors.Is(err, canjson.ErrUnknownMember) {
		t.Errorf("canjson.Marshal(chan) err = %v; want canjson.ErrInvalidValue and not canjson.ErrUnknownMember", err)
	}
}
