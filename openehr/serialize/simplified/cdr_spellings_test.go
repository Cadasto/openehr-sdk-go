package simplified_test

// REQ-053 — three spellings the reference CDR writes and reads that decode
// dropped, issue 456: a DV_PROPORTION's bare value, which is its magnitude; a
// DV_ORDINAL's |terminology; and the INTERVAL_EVENT attributes of an abstract
// EVENT the Web Template folds into its OBSERVATION. Each body is built on the
// corpus template the reference wrote it against.

import (
	"bytes"
	"errors"
	"maps"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aom/aom14"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/rmpath"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/simplified"
	"github.com/cadasto/openehr-sdk-go/openehr/template/webtemplate"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// The FLAT paths of the three positions, on the Vital signs CKM,
// minimal_admin.en.v1 and clinical_notes.v0 corpus templates.
const (
	spoKey     = "vital_signs_ckm/pulse_oximetry/any_event:0/spo"
	ordinalKey = "minimal/minimal:0/ordinal"
	storyObs   = "clinical_notes.v0/story_history"
)

// cdrBody is the mandatory context and extra.
func cdrBody(extra map[string]any) map[string]any {
	body := map[string]any{
		"ctx/language":      "en",
		"ctx/territory":     "NL",
		"ctx/composer_name": "Composer",
		"ctx/time":          "2026-10-26T09:00:00Z",
	}
	maps.Copy(body, extra)
	return body
}

// cdrFormats are the two forms every decode runs in.
var cdrFormats = []string{"FLAT", "STRUCTURED"}

// strictModes are [shorthandModes] with WithStrict added to each.
func strictModes(compiled *templatecompile.Compiled) []struct {
	name string
	opts []simplified.Option
} {
	modes := shorthandModes(compiled)
	for i := range modes {
		modes[i].opts = append([]simplified.Option{simplified.WithStrict()}, modes[i].opts...)
	}
	return modes
}

// --- DV_PROPORTION bare value ------------------------------------------------

// spoBody is a body over Vital signs CKM whose SpO2 is 50/100 as a percent,
// with extra.
func spoBody(extra map[string]any) map[string]any {
	body := cdrBody(map[string]any{
		"vital_signs_ckm/pulse_oximetry/any_event:0/time": "2026-10-26T09:00:00Z",
		spoKey + "|numerator":                             50.0,
		spoKey + "|denominator":                           100.0,
		spoKey + "|type":                                  2,
	})
	maps.Copy(body, extra)
	return body
}

// respelledMember is the STRUCTURED form of body with its member `"|value":v`
// renamed member, where v is the JSON text of the value.
func respelledMember(t *testing.T, body map[string]any, v, member string) []byte {
	t.Helper()
	doc, err := simplified.FlatToStructured(mustJSON(t, body))
	if err != nil {
		t.Fatalf("FlatToStructured: %v", err)
	}
	out := bytes.Replace(doc, []byte(`"|value":`+v), []byte(strconv.Quote(member)+`:`+v), 1)
	if bytes.Equal(out, doc) {
		t.Fatalf("fixture: no |value member holding %s in %s", v, doc)
	}
	return out
}

// TestREQ053BareProportionValueIsItsMagnitude — REQ-053, issue 456. The
// reference writes a DV_PROPORTION's magnitude, numerator/denominator, as the
// leaf's bare value beside its suffixes, and reads it. Decode reads it, in
// every spelling of a bare value, as the value the suffixes alone give, where
// it equals numerator/denominator to within one part in a billion; a null one
// is absent. Strict decode, with and without the template, finds nothing
// unmatched.
func TestREQ053BareProportionValueIsItsMagnitude(t *testing.T) {
	wt, compiled := templateOf(t, "Vital signs CKM")
	for _, mode := range strictModes(compiled) {
		t.Run(mode.name, func(t *testing.T) {
			want, err := simplified.UnmarshalFlat(mustJSON(t, spoBody(nil)), wt, mode.opts...)
			if err != nil {
				t.Fatalf("UnmarshalFlat(no bare value) = %v", err)
			}
			for name, extra := range map[string]map[string]any{
				"bare key":                    {spoKey: 0.5},
				"|value":                      {spoKey + "|value": 0.5},
				"bare key and an equal value": {spoKey: 0.5, spoKey + "|value": 0.5},
				"quoted":                      {spoKey: "0.5"},
				"inside the tolerance":        {spoKey: 0.5000000004},
				"null":                        {spoKey: nil},
			} {
				for _, format := range cdrFormats {
					got, err := decodeFormat(t, format, wt, spoBody(extra), mode.opts...)
					if err != nil {
						t.Errorf("%s %s: decode = %v, want the value the suffixes give", format, name, err)
						continue
					}
					if !bytes.Equal(canonicalComposition(t, got), canonicalComposition(t, want)) {
						t.Errorf("%s %s: decoded\n%s\nwant\n%s", format, name, canonicalComposition(t, got), canonicalComposition(t, want))
					}
				}
			}
			for _, member := range []string{"|", ""} {
				got, err := simplified.UnmarshalStructured(respelledMember(t, spoBody(map[string]any{spoKey: 0.5}), "0.5", member), wt, mode.opts...)
				if err != nil {
					t.Errorf("STRUCTURED %q member: decode = %v", member, err)
					continue
				}
				if !bytes.Equal(canonicalComposition(t, got), canonicalComposition(t, want)) {
					t.Errorf("STRUCTURED %q member: decoded\n%s\nwant\n%s", member, canonicalComposition(t, got), canonicalComposition(t, want))
				}
			}
		})
	}
}

// TestREQ053BareProportionValueThatContradictsIsRefused — REQ-053, issue 456.
// A bare DV_PROPORTION value that is not numerator/denominator to within one
// part in a billion contradicts them, and so does any value beside a zero
// denominator. Decode refuses it in every mode, wrapping
// ErrUnsupportedDatatype and naming the key as written, never the value.
func TestREQ053BareProportionValueThatContradictsIsRefused(t *testing.T) {
	wt, compiled := templateOf(t, "Vital signs CKM")
	for name, tc := range map[string]struct {
		extra map[string]any
		key   string
	}{
		"bare key":                {map[string]any{spoKey: 0.6012345}, spoKey},
		"|value":                  {map[string]any{spoKey + "|value": 0.6012345}, spoKey + "|value"},
		"just past the tolerance": {map[string]any{spoKey: 0.5000000006}, spoKey},
		"zero denominator":        {map[string]any{spoKey: 0.6012345, spoKey + "|denominator": 0}, spoKey},
	} {
		for _, mode := range append(shorthandModes(compiled), strictModes(compiled)...) {
			for _, format := range cdrFormats {
				_, err := decodeFormat(t, format, wt, spoBody(tc.extra), mode.opts...)
				if !errors.Is(err, simplified.ErrUnsupportedDatatype) {
					t.Errorf("%s %s %s: decode = %v, want ErrUnsupportedDatatype", name, mode.name, format, err)
					continue
				}
				if format == "FLAT" && !strings.Contains(err.Error(), strconv.Quote(tc.key)) {
					t.Errorf("%s %s: decode = %v, want it to name %q", name, mode.name, err, tc.key)
				}
				if strings.Contains(err.Error(), "6012345") || strings.Contains(err.Error(), "5000000006") {
					t.Errorf("%s %s %s: decode = %v carries the value", name, mode.name, format, err)
				}
			}
		}
	}
}

// TestREQ053BareProportionValueNeedsBothOperands — REQ-053, issue 456. A bare
// DV_PROPORTION value holds nothing the RM holds, so without its numerator
// and its denominator it is refused as the value missing the first of them it
// lacks, in every mode, as before it was read.
func TestREQ053BareProportionValueNeedsBothOperands(t *testing.T) {
	wt, compiled := templateOf(t, "Vital signs CKM")
	alone := spoBody(map[string]any{spoKey: 0.5})
	for _, k := range []string{"|numerator", "|denominator", "|type"} {
		delete(alone, spoKey+k)
	}
	besideNumerator := spoBody(map[string]any{spoKey: 0.5})
	delete(besideNumerator, spoKey+"|denominator")
	for name, tc := range map[string]struct {
		body    map[string]any
		missing string
	}{
		"alone":                {alone, "missing required |numerator"},
		"beside its numerator": {besideNumerator, "missing required |denominator"},
	} {
		for _, mode := range append(shorthandModes(compiled), strictModes(compiled)...) {
			for _, format := range cdrFormats {
				_, err := decodeFormat(t, format, wt, tc.body, mode.opts...)
				if !errors.Is(err, simplified.ErrUnsupportedDatatype) || !strings.Contains(err.Error(), tc.missing) {
					t.Errorf("%s %s %s: decode = %v, want ErrUnsupportedDatatype %s", name, mode.name, format, err, tc.missing)
				}
			}
		}
	}
}

// --- DV_ORDINAL |terminology -------------------------------------------------

// ordinalValuePath is the canonical path of the ordinal minimal_admin.en.v1
// holds.
const ordinalValuePath = "/content[openEHR-EHR-ADMIN_ENTRY.minimal.v1]/data[at0001]/items[at0002]/value"

// ordinalBody is a body over minimal_admin.en.v1 whose ordinal is the
// template's first option, with extra.
func ordinalBody(extra map[string]any) map[string]any {
	body := cdrBody(map[string]any{
		ordinalKey + "|code":    "at0003",
		ordinalKey + "|value":   "option 1",
		ordinalKey + "|ordinal": 1,
	})
	maps.Copy(body, extra)
	return body
}

// ordinalTerminologyOf returns the terminology of the symbol of the ordinal
// comp holds at [ordinalValuePath].
func ordinalTerminologyOf(t *testing.T, comp *rm.Composition) string {
	t.Helper()
	items, err := rmpath.ItemsAtPath(comp, ordinalValuePath)
	if err != nil || len(items) != 1 {
		t.Fatalf("ItemsAtPath(%s) = %d items, %v; want the ordinal", ordinalValuePath, len(items), err)
	}
	ord, ok := items[0].(*rm.DVOrdinal)
	if !ok {
		t.Fatalf("%s holds a %T, want *rm.DVOrdinal", ordinalValuePath, items[0])
	}
	return ord.Symbol.DefiningCode.TerminologyID.Value
}

// TestREQ053OrdinalTerminologyIsItsSymbols — REQ-053, issue 456. Decode reads
// a DV_ORDINAL's |terminology as its symbol's terminology. A local one reads
// as the ordinal without it, under strict, with and without the template,
// and a null one is absent. Without the template any terminology is taken as
// written, and with it so is one where the template's pairs give that code
// and value none.
func TestREQ053OrdinalTerminologyIsItsSymbols(t *testing.T) {
	wt, compiled := templateOf(t, "minimal_admin.en.v1")
	for _, mode := range strictModes(compiled) {
		want, err := simplified.UnmarshalFlat(mustJSON(t, ordinalBody(nil)), wt, mode.opts...)
		if err != nil {
			t.Fatalf("%s: UnmarshalFlat(no |terminology) = %v", mode.name, err)
		}
		for name, v := range map[string]any{"local": "local", "null": nil} {
			for _, format := range cdrFormats {
				got, err := decodeFormat(t, format, wt, ordinalBody(map[string]any{ordinalKey + "|terminology": v}), mode.opts...)
				if err != nil {
					t.Errorf("%s %s %s: decode = %v", mode.name, format, name, err)
					continue
				}
				if !bytes.Equal(canonicalComposition(t, got), canonicalComposition(t, want)) {
					t.Errorf("%s %s %s: decoded\n%s\nwant\n%s", mode.name, format, name, canonicalComposition(t, got), canonicalComposition(t, want))
				}
			}
		}
	}
	for _, format := range cdrFormats {
		got, err := decodeFormat(t, format, wt, ordinalBody(map[string]any{ordinalKey + "|terminology": "SNOMED-CT"}), simplified.WithStrict())
		if err != nil {
			t.Fatalf("%s without the template: decode = %v", format, err)
		}
		if term := ordinalTerminologyOf(t, got); term != "SNOMED-CT" {
			t.Errorf("%s without the template: terminology %q, want SNOMED-CT as written", format, term)
		}
		unlisted := ordinalBody(map[string]any{
			ordinalKey + "|code": "at9999", ordinalKey + "|ordinal": 9, ordinalKey + "|terminology": "SNOMED-CT",
		})
		got, err = decodeFormat(t, format, wt, unlisted, simplified.WithStrict(), simplified.WithTemplate(compiled))
		if err != nil {
			t.Fatalf("%s with the template, a code it does not list: decode = %v", format, err)
		}
		if term := ordinalTerminologyOf(t, got); term != "SNOMED-CT" {
			t.Errorf("%s with the template, a code it does not list: terminology %q, want SNOMED-CT as written", format, term)
		}
	}
}

// TestREQ053OrdinalTerminologyTheTemplateContradictsIsRefused — REQ-053,
// issue 456. Where the template's C_DV_ORDINAL pairs give the ordinal's code
// and value one terminology, a |terminology naming another contradicts it:
// a WithTemplate decode refuses it, wrapping ErrUnsupportedDatatype and naming
// the FLAT key, never the value.
func TestREQ053OrdinalTerminologyTheTemplateContradictsIsRefused(t *testing.T) {
	wt, compiled := templateOf(t, "minimal_admin.en.v1")
	key := ordinalKey + "|terminology"
	body := ordinalBody(map[string]any{key: "SNOMED-CT-0456"})
	for _, opts := range [][]simplified.Option{
		{simplified.WithTemplate(compiled)},
		{simplified.WithTemplate(compiled), simplified.WithStrict()},
	} {
		for _, format := range cdrFormats {
			_, err := decodeFormat(t, format, wt, body, opts...)
			if !errors.Is(err, simplified.ErrUnsupportedDatatype) {
				t.Errorf("%s: decode = %v, want ErrUnsupportedDatatype", format, err)
				continue
			}
			if format == "FLAT" && !strings.Contains(err.Error(), strconv.Quote(key)) {
				t.Errorf("%s: decode = %v, want it to name %q", format, err, key)
			}
			if err != nil && strings.Contains(err.Error(), "0456") {
				t.Errorf("%s: decode = %v carries the value", format, err)
			}
		}
	}
}

// --- an abstract EVENT folded into its OBSERVATION ---------------------------

// storyBody is a body over clinical_notes.v0, whose story_history
// OBSERVATION folds in its one abstract EVENT, with extra.
func storyBody(extra map[string]any) map[string]any {
	body := cdrBody(map[string]any{
		storyObs + "/story:0": "Headache since yesterday",
		storyObs + "/time":    "2026-10-26T09:00:00Z",
	})
	maps.Copy(body, extra)
	return body
}

// storyIntervalKeys are the reference's spellings of the three INTERVAL_EVENT
// attributes at the folded event.
func storyIntervalKeys() map[string]any {
	return map[string]any{
		storyObs + "/width":                     "PT42H",
		storyObs + "/math_function|code":        "145",
		storyObs + "/math_function|value":       "minimum",
		storyObs + "/math_function|terminology": "openehr",
		storyObs + "|sample_count":              3,
	}
}

// TestREQ053CollapsedEventReadsIntervalAttributes — REQ-053, issue 456.
// clinical_notes.v0's story_history holds one abstract EVENT that can occur
// once, which the Web Template folds into the OBSERVATION, its `time` leaf
// with it. The reference writes the event's width, math_function and
// sample_count at the OBSERVATION too. Decode reads them there, with and
// without the template, in FLAT and in STRUCTURED, and builds the event as an
// INTERVAL_EVENT carrying them; under strict nothing is unmatched.
func TestREQ053CollapsedEventReadsIntervalAttributes(t *testing.T) {
	wt, compiled := templateOf(t, "clinical_notes.v0")
	for _, mode := range strictModes(compiled) {
		for _, format := range cdrFormats {
			t.Run(mode.name+"/"+format, func(t *testing.T) {
				comp, err := decodeFormat(t, format, wt, storyBody(storyIntervalKeys()), mode.opts...)
				if err != nil {
					t.Fatalf("decode = %v", err)
				}
				ev := intervalEvent(t, onlyEvent(t, wt, comp, "story_history"))
				if ev.Width.Value != "PT42H" {
					t.Errorf("width = %q, want PT42H", ev.Width.Value)
				}
				mf := ev.MathFunction
				if mf.Value != "minimum" || mf.DefiningCode.CodeString != "145" || mf.DefiningCode.TerminologyID.Value != "openehr" {
					t.Errorf("math_function = %+v, want openehr::145|minimum", mf)
				}
				if ev.SampleCount == nil || *ev.SampleCount != 3 {
					t.Errorf("sample_count = %v, want 3", ev.SampleCount)
				}
				if ev.Time.Value != "2026-10-26T09:00:00Z" {
					t.Errorf("time = %q, want the body's", ev.Time.Value)
				}
			})
		}
	}
}

// TestREQ053CollapsedEventOneIntervalAttributeMakesAnIntervalEvent — REQ-053,
// issue 456. Each of the three attributes alone makes the folded event an
// INTERVAL_EVENT, as at an EVENT node; with the template the math_function and
// width the body leaves out take their defaults. A body that spells none of
// them decodes a POINT_EVENT.
func TestREQ053CollapsedEventOneIntervalAttributeMakesAnIntervalEvent(t *testing.T) {
	wt, compiled := templateOf(t, "clinical_notes.v0")
	for name, keys := range map[string]map[string]any{
		"width":         {storyObs + "/width": "PT1H"},
		"math_function": {storyObs + "/math_function|code": "146", storyObs + "/math_function|value": "mean", storyObs + "/math_function|terminology": "openehr"},
		"sample_count":  {storyObs + "|sample_count": 2},
	} {
		t.Run(name, func(t *testing.T) {
			intervalEvent(t, onlyEvent(t, wt, decodeWith(t, wt, storyBody(keys), simplified.WithStrict()), "story_history"))
			ev := intervalEvent(t, onlyEvent(t, wt, decodeWith(t, wt, storyBody(keys), simplified.WithStrict(), simplified.WithTemplate(compiled)), "story_history"))
			if _, spelled := keys[storyObs+"/math_function|code"]; !spelled && ev.MathFunction.DefiningCode.CodeString != "640" {
				t.Errorf("math_function = %+v, want the 640|actual default", ev.MathFunction)
			}
			if _, spelled := keys[storyObs+"/width"]; !spelled && ev.Width.Value != "PT0S" {
				t.Errorf("width = %q, want the PT0S default", ev.Width.Value)
			}
		})
	}
	ev := onlyEvent(t, wt, decodeWith(t, wt, storyBody(nil), simplified.WithStrict(), simplified.WithTemplate(compiled)), "story_history")
	if _, ok := ev.(*rm.PointEvent[rm.ItemStructure]); !ok {
		t.Errorf("event is a %T, want *rm.PointEvent[rm.ItemStructure]", ev)
	}
}

// TestREQ053CollapsedEventEncodesIntervalAttributes — REQ-053, issue 456.
// Encode writes an INTERVAL_EVENT the Web Template folds into its OBSERVATION
// with the keys decode reads there, and decode reads the FLAT and the
// STRUCTURED it writes back to the same event. A POINT_EVENT writes none of
// them.
func TestREQ053CollapsedEventEncodesIntervalAttributes(t *testing.T) {
	wt, compiled := templateOf(t, "clinical_notes.v0")
	comp := decodeWith(t, wt, storyBody(storyIntervalKeys()), simplified.WithTemplate(compiled))
	want := storyIntervalKeys()
	want[storyObs+"|sample_count"] = 3.0 // a JSON number reads back as float64
	got := flatOf(t, comp, wt)
	for key, v := range want {
		if got[key] != v {
			t.Errorf("MarshalFlat wrote %q = %#v, want %#v", key, got[key], v)
		}
	}
	for format, back := range map[string]*rm.Composition{
		"FLAT":       decodeAs(t, "FLAT", mustJSON(t, got), wt, simplified.WithStrict()),
		"STRUCTURED": structuredRoundTrip(t, comp, wt),
	} {
		ev := intervalEvent(t, onlyEvent(t, wt, back, "story_history"))
		if ev.SampleCount == nil || *ev.SampleCount != 3 || ev.Width.Value != "PT42H" || ev.MathFunction.DefiningCode.CodeString != "145" {
			t.Errorf("decode of the encoded %s: event = %+v, want sample_count 3, width PT42H, math_function 145", format, ev)
		}
	}

	point := decodeWith(t, wt, storyBody(nil), simplified.WithTemplate(compiled))
	for key := range flatOf(t, point, wt) {
		if strings.HasPrefix(key, storyObs+"|sample_count") || strings.HasPrefix(key, storyObs+"/math_function") || strings.HasPrefix(key, storyObs+"/width") {
			t.Errorf("a POINT_EVENT wrote %q", key)
		}
	}
}

// TestREQ053CollapsedEventAttributesAreSingleValued — REQ-053, issue 456. Each
// attribute of the folded event is single-valued, so an index past :0 on it
// is refused with ErrUnknownPath in every mode, as at an EVENT node.
func TestREQ053CollapsedEventAttributesAreSingleValued(t *testing.T) {
	wt, _ := templateOf(t, "clinical_notes.v0")
	for _, key := range []string{storyObs + "/width:1", storyObs + "/math_function:1|code"} {
		for _, opts := range [][]simplified.Option{nil, {simplified.WithStrict()}} {
			_, err := simplified.UnmarshalFlat(mustJSON(t, storyBody(map[string]any{key: "PT1H"})), wt, opts...)
			if !errors.Is(err, simplified.ErrUnknownPath) || !strings.Contains(err.Error(), "single-valued") {
				t.Errorf("UnmarshalFlat(%s) = %v, want ErrUnknownPath for an index past a single-valued attribute", key, err)
			}
		}
	}
}

// TestREQ053ObservationWithEventNodeReadsNoIntervalAttributes — REQ-053. An
// OBSERVATION whose event is a Web Template node of its own folds no event
// in, so the interval attributes spelled at the OBSERVATION are unmatched
// content there: strict decode refuses them with ErrUnknownPath.
func TestREQ053ObservationWithEventNodeReadsNoIntervalAttributes(t *testing.T) {
	wt, _ := templateOf(t, "Vital signs CKM")
	const obs = "vital_signs_ckm/pulse_oximetry"
	for _, key := range []string{obs + "/width", obs + "/math_function|code", obs + "|sample_count"} {
		_, err := simplified.UnmarshalFlat(mustJSON(t, spoBody(map[string]any{key: "PT1H"})), wt, simplified.WithStrict())
		if !errors.Is(err, simplified.ErrUnknownPath) || !strings.Contains(err.Error(), "unmatched content") {
			t.Errorf("UnmarshalFlat(%s, strict) = %v, want it refused as unmatched content", key, err)
		}
	}
}

// TestREQ053IntervalAttributeKeyTakenByAChildIsRefused — REQ-053. Where the
// node an INTERVAL_EVENT's attributes are spelled at, here the OBSERVATION
// that folds the event in, has a child whose id is `width`, the key is the
// child's, and decode reads it as the child. Encode then has no key for the
// event's width: it refuses the event with ErrUnsupportedDatatype naming the
// key, rather than write the width where decode reads the child or drop it.
// A POINT_EVENT, which has no width, encodes, and its child round-trips.
func TestREQ053IntervalAttributeKeyTakenByAChildIsRefused(t *testing.T) {
	raw, err := os.ReadFile(fixtures.TemplateOpt("clinical_notes.v0"))
	if err != nil {
		t.Fatal(err)
	}
	const story = `<items id="text">Story</items>`
	if n := strings.Count(string(raw), story); n != 1 {
		t.Fatalf("clinical_notes.v0 holds %q %d times, want once", story, n)
	}
	model, err := aom14.ReadOperationalTemplate(strings.NewReader(strings.Replace(string(raw), story, `<items id="text">Width</items>`, 1)))
	if err != nil {
		t.Fatalf("ReadOperationalTemplate: %v", err)
	}
	wt, compiled := compileTemplate(t, "clinical_notes.v0", model)
	body := cdrBody(map[string]any{storyObs + "/width:0": "Narrow", storyObs + "/time": "2026-10-26T09:00:00Z"})

	point := decodeWith(t, wt, body, simplified.WithStrict(), simplified.WithTemplate(compiled))
	back := decodeAs(t, "FLAT", mustJSON(t, flatOf(t, point, wt)), wt, simplified.WithStrict(), simplified.WithTemplate(compiled))
	if !bytes.Equal(canonicalComposition(t, back), canonicalComposition(t, point)) {
		t.Errorf("a POINT_EVENT beside the width child did not round-trip:\n%s\nwant\n%s", canonicalComposition(t, back), canonicalComposition(t, point))
	}

	body[storyObs+"|sample_count"] = 2
	interval := decodeWith(t, wt, body, simplified.WithStrict(), simplified.WithTemplate(compiled))
	intervalEvent(t, onlyEvent(t, wt, interval, "story_history"))
	key := strconv.Quote(storyObs + "/width")
	for name, marshal := range map[string]func(*rm.Composition, *webtemplate.WebTemplate) ([]byte, error){
		"FLAT": simplified.MarshalFlat, "STRUCTURED": simplified.MarshalStructured,
	} {
		if _, err := marshal(interval, wt); !errors.Is(err, simplified.ErrUnsupportedDatatype) || !strings.Contains(err.Error(), key) {
			t.Errorf("Marshal%s of an INTERVAL_EVENT whose width key a child takes = %v, want ErrUnsupportedDatatype naming %s", name, err, key)
		}
	}
}

// twoFoldedEventsTemplate is clinical_notes.v0 with a second abstract EVENT,
// at0003, that can occur once beside story_history's at0002: the Web Template
// folds both into the OBSERVATION, whose node then holds a `time` leaf for
// each.
func twoFoldedEventsTemplate(t *testing.T) (*webtemplate.WebTemplate, *templatecompile.Compiled) {
	t.Helper()
	raw, err := os.ReadFile(fixtures.TemplateOpt("clinical_notes.v0"))
	if err != nil {
		t.Fatal(err)
	}
	const anchor = "<children xsi:type=\"C_COMPLEX_OBJECT\">\n                <rm_type_name>EVENT</rm_type_name>"
	if n := strings.Count(string(raw), anchor); n != 1 {
		t.Fatalf("clinical_notes.v0 holds its EVENT anchor %d times, want once", n)
	}
	const second = `<children xsi:type="C_COMPLEX_OBJECT">
                <rm_type_name>EVENT</rm_type_name>
                <occurrences>
                  <lower_included>true</lower_included>
                  <upper_included>true</upper_included>
                  <lower_unbounded>false</lower_unbounded>
                  <upper_unbounded>false</upper_unbounded>
                  <lower>0</lower>
                  <upper>1</upper>
                </occurrences>
                <node_id>at0003</node_id>
              </children>
              `
	model, err := aom14.ReadOperationalTemplate(strings.NewReader(strings.Replace(string(raw), anchor, second+anchor, 1)))
	if err != nil {
		t.Fatalf("ReadOperationalTemplate: %v", err)
	}
	wt, compiled := compileTemplate(t, "clinical_notes.v0", model)
	var times []string
	var walk func(n *webtemplate.Node)
	walk = func(n *webtemplate.Node) {
		for _, ch := range n.Children {
			if n.ID == "story_history" && strings.HasPrefix(ch.ID, "time") {
				times = append(times, ch.ID)
			}
			walk(ch)
		}
	}
	walk(wt.Tree)
	if len(times) != 2 {
		t.Fatalf("story_history's node holds the time leaves %q, want two", times)
	}
	return wt, compiled
}

// TestREQ053TwoFoldedEventsIntervalAttributesAreRefused — REQ-053. Where an
// OBSERVATION folds in two abstract EVENTs, the keys at the OBSERVATION name
// no single event: decode reads `<observation>/width`, `/math_function` and
// `|sample_count` there as unmatched content, and encode refuses an
// INTERVAL_EVENT among them with ErrUnsupportedDatatype naming the key, where
// it wrote none and so turned the event into a POINT_EVENT. A POINT_EVENT
// there encodes and round-trips.
func TestREQ053TwoFoldedEventsIntervalAttributesAreRefused(t *testing.T) {
	wt, compiled := twoFoldedEventsTemplate(t)
	for _, key := range slices.Sorted(maps.Keys(storyIntervalKeys())) {
		for _, opts := range [][]simplified.Option{{simplified.WithStrict()}, {simplified.WithStrict(), simplified.WithTemplate(compiled)}} {
			_, err := simplified.UnmarshalFlat(mustJSON(t, storyBody(map[string]any{key: storyIntervalKeys()[key]})), wt, opts...)
			if !errors.Is(err, simplified.ErrUnknownPath) || !strings.Contains(err.Error(), "unmatched content") {
				t.Errorf("UnmarshalFlat(%s, strict) = %v, want it refused as unmatched content", key, err)
			}
		}
	}

	point := decodeWith(t, wt, storyBody(nil), simplified.WithStrict(), simplified.WithTemplate(compiled))
	back := decodeAs(t, "FLAT", mustJSON(t, flatOf(t, point, wt)), wt, simplified.WithStrict(), simplified.WithTemplate(compiled))
	if !bytes.Equal(canonicalComposition(t, back), canonicalComposition(t, point)) {
		t.Errorf("a POINT_EVENT at an OBSERVATION folding two events did not round-trip:\n%s\nwant\n%s", canonicalComposition(t, back), canonicalComposition(t, point))
	}

	// The event the original template folds alone, at the same path, is an
	// INTERVAL_EVENT carrying the three.
	single, singleCompiled := templateOf(t, "clinical_notes.v0")
	interval := decodeWith(t, single, storyBody(storyIntervalKeys()), simplified.WithStrict(), simplified.WithTemplate(singleCompiled))
	intervalEvent(t, onlyEvent(t, single, interval, "story_history"))
	key := strconv.Quote(storyObs + "/width")
	for name, marshal := range map[string]func(*rm.Composition, *webtemplate.WebTemplate) ([]byte, error){
		"FLAT": simplified.MarshalFlat, "STRUCTURED": simplified.MarshalStructured,
	} {
		if _, err := marshal(interval, wt); !errors.Is(err, simplified.ErrUnsupportedDatatype) || !strings.Contains(err.Error(), key) {
			t.Errorf("Marshal%s of an INTERVAL_EVENT at an OBSERVATION folding two events = %v, want ErrUnsupportedDatatype naming %s", name, err, key)
		}
	}
	ev := intervalEvent(t, onlyEvent(t, single, interval, "story_history"))
	ev.Width = rm.DVDuration{}
	ev.MathFunction = rm.DVCodedText{}
	key = strconv.Quote(storyObs + "|sample_count")
	if _, err := simplified.MarshalFlat(interval, wt); !errors.Is(err, simplified.ErrUnsupportedDatatype) || !strings.Contains(err.Error(), key) {
		t.Errorf("MarshalFlat of an INTERVAL_EVENT holding a sample_count alone = %v, want ErrUnsupportedDatatype naming %s", err, key)
	}
}
