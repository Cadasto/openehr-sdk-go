package simplified_test

// REQ-053 — the input shorthands the ITS-REST 1.1.0 Simplified Formats let a
// client write: a bare DV_IDENTIFIER is its |id, `|value` is the bare value of
// a DV_TEXT (a reference range's `meaning` included) and of a DV_PARSABLE, and
// a bare ctx/setting names a member of the openEHR setting group by code or by
// rubric. Each decodes to what the full spelling decodes to, with and without
// the template; two spellings of one field that disagree are refused, and
// encode keeps writing the full spelling.

import (
	"bytes"
	"encoding/json"
	"maps"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/simplified"
	"github.com/cadasto/openehr-sdk-go/openehr/template/webtemplate"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// shorthandObs is the test_all_types OBSERVATION the leaf cases write under.
const shorthandObs = "test_all_types/test_all_types:0"

// shorthandBody is the issue's base body, which decodes and validates against
// test_all_types, with extra merged over it; a nil value in extra is written
// as JSON null.
func shorthandBody(extra map[string]any) map[string]any {
	body := map[string]any{
		"ctx/language":      "en",
		"ctx/territory":     "US",
		"ctx/composer_name": "Silvia Blake",
		"ctx/time":          "2019-03-22T22:00:00+01:00",
		"test_all_types/context/context_coded_text|code":        "at0007",
		"test_all_types/context/context_coded_text|value":       "value2",
		"test_all_types/context/context_coded_text|terminology": "local",
	}
	maps.Copy(body, extra)
	return body
}

// shorthandTarget is the test_all_types Web Template and compiled template.
func shorthandTarget(t *testing.T) (*webtemplate.WebTemplate, *templatecompile.Compiled) {
	t.Helper()
	wt, compiled, _, _ := crossFormatComposerSet(t, "test_all_types")
	return wt, compiled
}

// shorthandModes are the two decodes every case runs under.
func shorthandModes(compiled *templatecompile.Compiled) []struct {
	name string
	opts []simplified.Option
} {
	return []struct {
		name string
		opts []simplified.Option
	}{
		{"without template", nil},
		{"with template", []simplified.Option{simplified.WithTemplate(compiled)}},
	}
}

// canonicalComposition renders a decoded composition as canonical JSON for a
// comparison.
func canonicalComposition(t *testing.T, comp *rm.Composition) []byte {
	t.Helper()
	b, err := canjson.Marshal(comp)
	if err != nil {
		t.Fatalf("canjson.Marshal(composition): %v", err)
	}
	return b
}

// shorthandCase is one shorthand spelling and the full spelling it stands for.
type shorthandCase struct {
	name      string
	shorthand map[string]any
	full      map[string]any
}

const (
	identifierID = "35e4059a-e6f4-4ba1-befe-e2493bce3991"
	parsableText = "Formal instructions on carrying out the procedure..."
)

// referenceRange is a DV_QUANTITY with one other reference range whose
// meaning is spelled by meaning, the key below the range that carries it.
func referenceRange(meaning string) map[string]any {
	q := shorthandObs + "/quantity"
	return map[string]any{
		q + "|magnitude": 68.07,
		q + "|unit":      "mg",
		q + "/_other_reference_ranges:0/lower|magnitude": 100,
		q + "/_other_reference_ranges:0/lower|unit":      "mg",
		q + "/_other_reference_ranges:0/" + meaning:      "high",
	}
}

func shorthandCases() []shorthandCase {
	id := shorthandObs + "/identifier"
	text := shorthandObs + "/text"
	parsable := shorthandObs + "/parsable_any"
	feeder := shorthandObs + "/_feeder_audit"
	settingPair := map[string]any{"ctx/setting|code": "238", "ctx/setting|value": "other care"}
	return []shorthandCase{
		{
			name:      "DV_IDENTIFIER bare value alone",
			shorthand: map[string]any{id: identifierID},
			full:      map[string]any{id + "|id": identifierID},
		},
		{
			name: "DV_IDENTIFIER bare value beside issuer, assigner and type",
			shorthand: map[string]any{
				id: identifierID, id + "|issuer": "Hospital", id + "|assigner": "Hospital", id + "|type": "LOCALID",
			},
			full: map[string]any{
				id + "|id": identifierID, id + "|issuer": "Hospital", id + "|assigner": "Hospital", id + "|type": "LOCALID",
			},
		},
		{
			name:      "DV_IDENTIFIER bare value beside an equal |id",
			shorthand: map[string]any{id: identifierID, id + "|id": identifierID},
			full:      map[string]any{id + "|id": identifierID},
		},
		{
			name:      "DV_IDENTIFIER bare value beside a null |id",
			shorthand: map[string]any{id: identifierID, id + "|id": nil},
			full:      map[string]any{id + "|id": identifierID},
		},
		{
			name:      "DV_TEXT |value",
			shorthand: map[string]any{text + "|value": "text 69"},
			full:      map[string]any{text: "text 69"},
		},
		{
			name:      "DV_TEXT |value beside an equal bare value",
			shorthand: map[string]any{text: "text 69", text + "|value": "text 69"},
			full:      map[string]any{text: "text 69"},
		},
		{
			name:      "DV_TEXT |value beside a null |code",
			shorthand: map[string]any{text + "|value": "text 69", text + "|code": nil},
			full:      map[string]any{text: "text 69"},
		},
		{
			name:      "reference range meaning |value",
			shorthand: referenceRange("meaning|value"),
			full:      referenceRange("meaning"),
		},
		{
			name:      "DV_PARSABLE |value",
			shorthand: map[string]any{parsable + "|value": parsableText, parsable + "|formalism": "GLIF 1.0"},
			full:      map[string]any{parsable: parsableText, parsable + "|formalism": "GLIF 1.0"},
		},
		{
			name:      "DV_PARSABLE |value beside an equal bare value",
			shorthand: map[string]any{parsable: parsableText, parsable + "|value": parsableText, parsable + "|formalism": "GLIF 1.0"},
			full:      map[string]any{parsable: parsableText, parsable + "|formalism": "GLIF 1.0"},
		},
		{
			name:      "party _identifier bare value",
			shorthand: map[string]any{"test_all_types/composer/_identifier:0": identifierID, "test_all_types/composer/_identifier:0|issuer": "Hospital"},
			full:      map[string]any{"test_all_types/composer/_identifier:0|id": identifierID, "test_all_types/composer/_identifier:0|issuer": "Hospital"},
		},
		{
			name: "feeder item id bare value and original_content |value",
			shorthand: map[string]any{
				text: "text 69",
				feeder + "/originating_system_audit|system_id": "S",
				feeder + "/originating_system_item_id:0":       identifierID,
				feeder + "/original_content|value":             parsableText,
				feeder + "/original_content|formalism":         "text/plain",
			},
			full: map[string]any{
				text: "text 69",
				feeder + "/originating_system_audit|system_id": "S",
				feeder + "/originating_system_item_id:0|id":    identifierID,
				feeder + "/original_content":                   parsableText,
				feeder + "/original_content|formalism":         "text/plain",
			},
		},
		{
			name:      "ctx/setting by rubric",
			shorthand: map[string]any{"ctx/setting": "other care"},
			full:      settingPair,
		},
		{
			name:      "ctx/setting by code",
			shorthand: map[string]any{"ctx/setting": "238"},
			full:      settingPair,
		},
		{
			name:      "ctx/setting by code, another member",
			shorthand: map[string]any{"ctx/setting": "225"},
			full:      map[string]any{"ctx/setting|code": "225", "ctx/setting|value": "home"},
		},
		{
			name:      "ctx/setting beside the pair it agrees with",
			shorthand: map[string]any{"ctx/setting": "other care", "ctx/setting|code": "238", "ctx/setting|value": "other care"},
			full:      settingPair,
		},
		{
			name:      "ctx/setting beside the code it agrees with",
			shorthand: map[string]any{"ctx/setting": "other care", "ctx/setting|code": "238"},
			full:      settingPair,
		},
		{
			name:      "real-path setting by rubric",
			shorthand: map[string]any{"test_all_types/context/setting": "home"},
			full:      map[string]any{"ctx/setting|code": "225", "ctx/setting|value": "home"},
		},
		{
			name:      "real-path setting with :0 by code",
			shorthand: map[string]any{"test_all_types/context:0/setting:0": "225"},
			full:      map[string]any{"ctx/setting|code": "225", "ctx/setting|value": "home"},
		},
		{
			name:      "null ctx/setting stays absent",
			shorthand: map[string]any{"ctx/setting": nil},
			full:      map[string]any{},
		},
	}
}

// TestREQ053InputShorthandsDecodeAsTheFullSpelling — REQ-053: each shorthand
// decodes to the composition its full spelling decodes to, with and without
// the template, and with the template the composition validates.
func TestREQ053InputShorthandsDecodeAsTheFullSpelling(t *testing.T) {
	wt, compiled := shorthandTarget(t)
	for _, tc := range shorthandCases() {
		for _, mode := range shorthandModes(compiled) {
			t.Run(tc.name+"/"+mode.name, func(t *testing.T) {
				got, err := simplified.UnmarshalFlat(mustJSON(t, shorthandBody(tc.shorthand)), wt, mode.opts...)
				if err != nil {
					t.Fatalf("UnmarshalFlat(shorthand %v) = %v", tc.shorthand, err)
				}
				want, err := simplified.UnmarshalFlat(mustJSON(t, shorthandBody(tc.full)), wt, mode.opts...)
				if err != nil {
					t.Fatalf("UnmarshalFlat(full %v) = %v", tc.full, err)
				}
				if g, w := canonicalComposition(t, got), canonicalComposition(t, want); !bytes.Equal(g, w) {
					t.Errorf("shorthand %v decodes to\n%s\nwant what %v decodes to\n%s", tc.shorthand, g, tc.full, w)
				}
				if mode.opts == nil {
					return
				}
				if res := validation.ValidateComposition(got, compiled); !res.OK {
					t.Errorf("ValidateComposition(shorthand %v) is not OK: %+v", tc.shorthand, res.Issues)
				}
			})
		}
	}
}

// TestREQ053InputShorthandsDecodeInStructured — REQ-053: STRUCTURED reads the
// shorthands as FLAT does, since it decodes through the FLAT keys.
func TestREQ053InputShorthandsDecodeInStructured(t *testing.T) {
	wt, compiled := shorthandTarget(t)
	id := shorthandObs + "/identifier"
	for name, tc := range map[string]shorthandCase{
		"DV_IDENTIFIER bare value beside its issuer": {
			shorthand: map[string]any{id: identifierID, id + "|issuer": "Hospital"},
			full:      map[string]any{id + "|id": identifierID, id + "|issuer": "Hospital"},
		},
		"DV_TEXT |value": {
			shorthand: map[string]any{shorthandObs + "/text|value": "text 69"},
			full:      map[string]any{shorthandObs + "/text": "text 69"},
		},
	} {
		for _, mode := range shorthandModes(compiled) {
			t.Run(name+"/"+mode.name, func(t *testing.T) {
				doc, err := simplified.FlatToStructured(mustJSON(t, shorthandBody(tc.shorthand)))
				if err != nil {
					t.Fatalf("FlatToStructured(%v) = %v", tc.shorthand, err)
				}
				got, err := simplified.UnmarshalStructured(doc, wt, mode.opts...)
				if err != nil {
					t.Fatalf("UnmarshalStructured(%s) = %v", doc, err)
				}
				want, err := simplified.UnmarshalFlat(mustJSON(t, shorthandBody(tc.full)), wt, mode.opts...)
				if err != nil {
					t.Fatalf("UnmarshalFlat(full %v) = %v", tc.full, err)
				}
				if g, w := canonicalComposition(t, got), canonicalComposition(t, want); !bytes.Equal(g, w) {
					t.Errorf("STRUCTURED %s decodes to\n%s\nwant what FLAT %v decodes to\n%s", doc, g, tc.full, w)
				}
			})
		}
	}
}

// TestREQ053DisagreeingShorthandRefused — REQ-053: "Where two keys for one
// field disagree, the codec MUST refuse rather than choose". A shorthand
// beside the full spelling of the same value with a different value is
// refused naming the key and carrying neither value, as is a bare setting
// outside the openEHR setting group, which names no setting at all.
func TestREQ053DisagreeingShorthandRefused(t *testing.T) {
	wt, compiled := shorthandTarget(t)
	const (
		first  = "SECRET-FIRST-0451"
		second = "SECRET-SECOND-0451"
	)
	id := shorthandObs + "/identifier"
	text := shorthandObs + "/text"
	parsable := shorthandObs + "/parsable_any"
	meaning := shorthandObs + "/quantity/_other_reference_ranges:0/meaning"
	withMeaning := func(extra map[string]any) map[string]any {
		body := referenceRange("meaning")
		delete(body, meaning)
		maps.Copy(body, extra)
		return body
	}
	for _, tc := range []struct {
		name  string
		extra map[string]any
		want  error
		named []string
	}{
		{
			name:  "DV_IDENTIFIER bare value and |id",
			extra: map[string]any{id: first, id + "|id": second, id + "|issuer": "Hospital"},
			want:  simplified.ErrUnknownPath,
			named: []string{`"` + id + `"`, `"` + id + `|id"`},
		},
		{
			name:  "DV_TEXT bare value and |value",
			extra: map[string]any{text: first, text + "|value": second},
			want:  simplified.ErrUnknownPath,
			named: []string{`"` + text + `"`, `"` + text + `|value"`},
		},
		{
			name:  "reference range meaning and meaning|value",
			extra: withMeaning(map[string]any{meaning: first, meaning + "|value": second}),
			want:  simplified.ErrUnknownPath,
			named: []string{`"` + meaning + `"`, `"` + meaning + `|value"`},
		},
		{
			name:  "DV_PARSABLE bare value and |value",
			extra: map[string]any{parsable: first, parsable + "|value": second, parsable + "|formalism": "GLIF 1.0"},
			want:  simplified.ErrUnknownPath,
			named: []string{`"` + parsable + `"`, `"` + parsable + `|value"`},
		},
		{
			name:  "ctx/setting and the pair",
			extra: map[string]any{"ctx/setting": "home", "ctx/setting|code": "238", "ctx/setting|value": "other care"},
			want:  simplified.ErrUnknownPath,
			named: []string{`"ctx/setting"`},
		},
		{
			name:  "ctx/setting and the rubric",
			extra: map[string]any{"ctx/setting": "238", "ctx/setting|code": "238", "ctx/setting|value": "Other Care"},
			want:  simplified.ErrUnknownPath,
			named: []string{`"ctx/setting"`, `"ctx/setting|value"`},
		},
		{
			name:  "real-path setting and ctx/setting",
			extra: map[string]any{"test_all_types/context/setting": "home", "ctx/setting": "238"},
			want:  simplified.ErrUnknownPath,
			named: []string{`"ctx/setting"`, `"test_all_types/context/setting"`},
		},
		{
			name:  "ctx/setting outside the setting group",
			extra: map[string]any{"ctx/setting": first},
			want:  simplified.ErrUnsupportedDatatype,
			named: []string{`"ctx/setting"`},
		},
		{
			name:  "ctx/setting outside the setting group beside the pair",
			extra: map[string]any{"ctx/setting": first, "ctx/setting|code": "238", "ctx/setting|value": "other care"},
			want:  simplified.ErrUnsupportedDatatype,
			named: []string{`"ctx/setting"`},
		},
		{
			name:  "real-path setting outside the setting group",
			extra: map[string]any{"test_all_types/context/setting": first},
			want:  simplified.ErrUnsupportedDatatype,
			named: []string{`"test_all_types/context/setting"`},
		},
		{
			name:  "ctx/setting a number",
			extra: map[string]any{"ctx/setting": 238},
			want:  simplified.ErrUnsupportedDatatype,
			named: []string{"ctx/setting"},
		},
	} {
		for _, mode := range shorthandModes(compiled) {
			t.Run(tc.name+"/"+mode.name, func(t *testing.T) {
				comp, err := simplified.UnmarshalFlat(mustJSON(t, shorthandBody(tc.extra)), wt, mode.opts...)
				if err == nil {
					t.Fatalf("UnmarshalFlat(%v) = %s, want %v", tc.extra, canonicalComposition(t, comp), tc.want)
				}
				wantRefusal(t, err, tc.want, tc.named...)
				for _, v := range []string{first, second} {
					if strings.Contains(err.Error(), v) {
						t.Errorf("error %q carries the value %q", err, v)
					}
				}
			})
		}
	}
}

// TestREQ053ShorthandsEncodeInTheFullSpelling — REQ-053: decode reads the
// shorthands, and encode writes what it wrote before: a DV_IDENTIFIER's
// |id, a DV_TEXT's and a DV_PARSABLE's bare value, and the ctx/setting pair.
func TestREQ053ShorthandsEncodeInTheFullSpelling(t *testing.T) {
	wt, compiled := shorthandTarget(t)
	id := shorthandObs + "/identifier"
	text := shorthandObs + "/text"
	parsable := shorthandObs + "/parsable_any"
	body := shorthandBody(map[string]any{
		id:                      identifierID,
		text + "|value":         "text 69",
		parsable + "|value":     parsableText,
		parsable + "|formalism": "GLIF 1.0",
		"ctx/setting":           "home",
		id + "|issuer":          "Hospital",
	})
	comp, err := simplified.UnmarshalFlat(mustJSON(t, body), wt, simplified.WithTemplate(compiled))
	if err != nil {
		t.Fatalf("UnmarshalFlat = %v", err)
	}
	out, err := simplified.MarshalFlat(comp, wt)
	if err != nil {
		t.Fatalf("MarshalFlat = %v", err)
	}
	var flat map[string]any
	if err := json.Unmarshal(out, &flat); err != nil {
		t.Fatalf("unmarshal MarshalFlat output: %v", err)
	}
	for key, want := range map[string]any{
		id + "|id":          identifierID,
		text:                "text 69",
		parsable:            parsableText,
		"ctx/setting|code":  "225",
		"ctx/setting|value": "home",
	} {
		if got, ok := flat[key]; !ok || got != want {
			t.Errorf("MarshalFlat wrote %q = %v (present %v), want %v", key, got, ok, want)
		}
	}
	for _, key := range []string{id, text + "|value", parsable + "|value", "ctx/setting"} {
		if v, ok := flat[key]; ok {
			t.Errorf("MarshalFlat wrote the shorthand %q = %v, want the full spelling only", key, v)
		}
	}
}
