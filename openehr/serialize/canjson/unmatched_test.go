package canjson_test

// unmatched_test.go — REQ-170 (wire.md § REQ-170, Unmatched content on
// decode) for canonical JSON, and the REQ-052 rule that keeps the codec's
// sentinels distinct. A member is unmatched when its name is neither `_type`
// nor an attribute of the RM type its object decodes as. The default ignores
// it; WithUnmatched reports every one, in document order, as a JSON Pointer;
// WithStrict refuses the first with canjson.ErrUnknownMember.

import (
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
// and a bound that is itself a polymorphic slot), and in an AOM 1.4 value and
// its constraint intervals. One case has names RFC 6901 must escape.
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
			// AOM 1.4: a C_COMPLEX_OBJECT, its `occurrences` (the abstract BASE
			// Interval, which declares no `_type`: the `_type` member it carries
			// is never unmatched), a C_SINGLE_ATTRIBUTE in a polymorphic slot,
			// and that attribute's `existence` interval.
			name: "AOM 1.4 value",
			tmpl: `{"_type":"C_COMPLEX_OBJECT",` + extPlaceholder + `"rm_type_name":"OBSERVATION","node_id":"at0000",` +
				`"occurrences":{"_type":"MULTIPLICITY_INTERVAL",` + extPlaceholder + `"lower":1,"upper":1},` +
				`"attributes":[{"_type":"C_SINGLE_ATTRIBUTE",` + extPlaceholder + `"rm_attribute_name":"data",` +
				`"existence":{` + extPlaceholder + `"lower":1,"upper":1}}]}`,
			names:    []string{"x-object", "x-occurrences", "x-attribute", "x-existence"},
			pointers: []string{"/x-object", "/occurrences/x-occurrences", "/attributes/0/x-attribute", "/attributes/0/existence/x-existence"},
			into:     func() any { return new(aom14.CComplexObject) },
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
// wraps canjson.ErrUnknownMember, is returned on its own (so its message names
// one pointer), names the first unmatched member's pointer, carries no value
// from the input, and matches neither of the codec's other two sentinels
// (REQ-052) nor the dispatch envelope.
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

// TestREQ170_ConcurrentDecodesKeepTheirOwnReports runs tracked decodes in
// parallel, each with its own report function and its own extension member,
// and requires each to see exactly its own member. The tracking state is held
// per decoder, so concurrent decodes never see each other's members, slots
// included.
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

// TestREQ170_UnknownTypeFailsInEveryMode pins that a `_type` naming no RM
// class is a dispatch failure in every mode, never an unmatched member: the
// slot cannot be decoded at all, so nothing under it is reported either.
func TestREQ170_UnknownTypeFailsInEveryMode(t *testing.T) {
	const in = `{"_type":"ELEMENT","name":{"_type":"DV_TEXT","value":"n"},"archetype_node_id":"at1",` +
		`"value":{"_type":"NO_SUCH_CLASS","x-ext":"` + plantedValue + `"}}`
	for _, mode := range []string{"default", "report", "strict"} {
		t.Run(mode, func(t *testing.T) {
			var reported []string
			report := canjson.WithUnmatched(func(p string) { reported = append(reported, p) })
			var opts []canjson.DecoderOption
			switch mode {
			case "report":
				opts = []canjson.DecoderOption{report}
			case "strict":
				opts = []canjson.DecoderOption{canjson.WithStrict(), report}
			}
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

// TestREQ170_RegisteredTypeCensus decodes, for every registered library type
// (RM and AOM 1.4), a body carrying its `_type` and one extension member, and
// requires the report to name exactly that member. It is the control that no
// registered type decodes outside the tracked path, where an unmatched member
// would be skipped unreported.
func TestREQ170_RegisteredTypeCensus(t *testing.T) {
	const libraryPrefix = "github.com/cadasto/openehr-sdk-go/openehr/"
	checked := 0
	for _, name := range typereg.Default.Names() {
		ctor, _ := typereg.Default.Lookup(name)
		rt := reflect.TypeOf(ctor())
		for rt.Kind() == reflect.Pointer {
			rt = rt.Elem()
		}
		if !strings.HasPrefix(rt.PkgPath(), libraryPrefix) {
			continue // a test fixture registered elsewhere; not a shipped type
		}
		checked++
		in := `{"_type":"` + name + `","x-census":"` + plantedValue + `"}`
		var got []string
		err := canjson.Unmarshal([]byte(in), ctor(), canjson.WithUnmatched(func(p string) { got = append(got, p) }))
		if err != nil {
			t.Errorf("%s: canjson.Unmarshal(%s, WithUnmatched) = %v; want nil", name, in, err)
			continue
		}
		if want := []string{"/x-census"}; !slices.Equal(got, want) {
			t.Errorf("%s: WithUnmatched reported %q; want %q", name, got, want)
		}
	}
	if checked < 100 {
		t.Errorf("census covered %d library types; want the full RM + AOM 1.4 inventory", checked)
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
