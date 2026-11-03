package aom14_test

// REQ-176 § Round trip, REQ-056 — the AOM 1.4 Hash members round-trip in the
// shapes the AM Release-1.4 schemas give them: ARCHETYPE_TERM's and
// ANNOTATION's items as StringDictionaryItems, the key in `id` and the value
// as text; ARCHETYPE's translations one TRANSLATION_DETAILS per value, with no
// `id`, keyed by the value's language code; entries in ascending key order.
// REQ-170: an OPT repeating a Hash key is refused in every mode.

import (
	"encoding/xml"
	"errors"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aom/aom14"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml"
)

// childAttrs returns, for each child element of the root of doc named local,
// its attributes as name=value pairs joined by spaces.
func childAttrs(t *testing.T, doc []byte, local string) []string {
	t.Helper()
	var out []string
	dec := xml.NewDecoder(strings.NewReader(string(doc)))
	depth := 0
	for {
		tok, err := dec.Token()
		if err != nil {
			return out
		}
		switch tk := tok.(type) {
		case xml.StartElement:
			depth++
			if depth == 2 && tk.Name.Local == local {
				pairs := make([]string, 0, len(tk.Attr))
				for _, a := range tk.Attr {
					pairs = append(pairs, a.Name.Local+"="+a.Value)
				}
				out = append(out, strings.Join(pairs, " "))
			}
		case xml.EndElement:
			depth--
		}
	}
}

// TestREQ176_REQ056_ArchetypeTermItems pins ARCHETYPE_TERM.items: a term the
// schema's way decodes in every mode with nothing unmatched, its items keyed by
// `id`, and encodes back with `code` on the term and one `items` element per
// entry, `id` holding the key, in ascending key order.
func TestREQ176_REQ056_ArchetypeTermItems(t *testing.T) {
	doc := `<archetype_term` + nsOpenEHR + ` code="at0000">` +
		`<items id="text">Blood pressure</items><items id="description">The local measurement</items><items id="comment"></items>` +
		`</archetype_term>`
	want := aom14.ArchetypeTerm{Code: "at0000", Items: map[string]string{
		"text": "Blood pressure", "description": "The local measurement", "comment": "",
	}}
	var reported []string
	for mode, opts := range decodeModes(&reported) {
		var got aom14.ArchetypeTerm
		if err := canxml.Unmarshal([]byte(doc), &got, opts...); err != nil {
			t.Fatalf("canxml.Unmarshal(%s) = %v; want nil", mode, err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("canxml.Unmarshal(%s) = %#v; want %#v", mode, got, want)
		}
	}
	if len(reported) > 0 {
		t.Errorf("the report option reported %q; want nothing, the key attributes included", reported)
	}
	out, err := canxml.Marshal(&want)
	if err != nil {
		t.Fatalf("canxml.Marshal = %v; want nil", err)
	}
	if got, w := childAttrs(t, out, "items"), []string{"id=comment", "id=description", "id=text"}; !slices.Equal(got, w) {
		t.Errorf("%s writes items with %q; want %q", out, got, w)
	}
	if !strings.Contains(string(out), ` code="at0000"`) || !strings.Contains(string(out), `<items id="text">Blood pressure</items>`) {
		t.Errorf("%s lacks the term's code attribute or an item as `id` and text", out)
	}
	var again aom14.ArchetypeTerm
	if err := canxml.Unmarshal(out, &again, canxml.WithStrict()); err != nil || !reflect.DeepEqual(again, want) {
		t.Errorf("decoding %s again = %#v, %v; want %#v", out, again, err, want)
	}
}

// TestREQ176_REQ056_AnnotationItems pins ANNOTATION.items, the other
// Hash<String,String> of the operational template: it round-trips under its
// `path`, its entries keyed by `id`.
func TestREQ176_REQ056_AnnotationItems(t *testing.T) {
	in := &aom14.Annotation{Path: "/content[at0001]", Items: map[string]string{"design note": "d", "comment": "c"}}
	out, err := canxml.Marshal(in)
	if err != nil {
		t.Fatalf("canxml.Marshal = %v; want nil", err)
	}
	if got, w := childAttrs(t, out, "items"), []string{"id=comment", "id=design note"}; !slices.Equal(got, w) {
		t.Errorf("%s writes items with %q; want %q", out, got, w)
	}
	var got aom14.Annotation
	if err := canxml.Unmarshal(out, &got, canxml.WithStrict()); err != nil {
		t.Fatalf("canxml.Unmarshal(%s, strict) = %v; want nil", out, err)
	}
	if !reflect.DeepEqual(&got, in) {
		t.Errorf("round trip of %s = %#v; want %#v", out, got, *in)
	}
}

// TestREQ176_REQ056_ArchetypeTranslations pins ARCHETYPE's translations, a
// Hash of the RM TRANSLATION_DETAILS: one `translations` element per value,
// with no `id`, in ascending order of the keys, each decoded under its
// language code.
func TestREQ176_REQ056_ArchetypeTranslations(t *testing.T) {
	lang := func(code string) rm.CodePhrase {
		return rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "ISO_639-1"}, CodeString: code}
	}
	in := &aom14.Archetype{
		ArchetypeID:      rm.ArchetypeID{Value: "openEHR-EHR-OBSERVATION.blood_pressure.v2"},
		Concept:          "at0000",
		OriginalLanguage: lang("en"),
		Translations: &map[string]rm.TranslationDetails{
			"nl": {Language: lang("nl"), Author: map[string]string{"name": "V"}},
			"de": {Language: lang("de"), Author: map[string]string{"name": "W", "organisation": "O"}, OtherDetails: &map[string]string{"x": "y"}},
		},
	}
	out, err := canxml.Marshal(in)
	if err != nil {
		t.Fatalf("canxml.Marshal = %v; want nil", err)
	}
	if got := childAttrs(t, out, "translations"); !slices.Equal(got, []string{"", ""}) {
		t.Errorf("%s writes translations with the attributes %q; want two, with none", out, got)
	}
	if i, j := strings.Index(string(out), "<code_string>de<"), strings.Index(string(out), "<code_string>nl<"); i < 0 || j < i {
		t.Errorf("%s does not write the de translation before the nl one", out)
	}
	var got aom14.Archetype
	var reported []string
	if err := canxml.Unmarshal(out, &got, canxml.WithUnmatched(func(p string) { reported = append(reported, p) })); err != nil {
		t.Fatalf("canxml.Unmarshal(%s) = %v; want nil", out, err)
	}
	if len(reported) > 0 {
		t.Errorf("decoding %s reported %q; want nothing", out, reported)
	}
	if !reflect.DeepEqual(got.Translations, in.Translations) {
		t.Errorf("round trip of %s gives the translations %#v; want %#v", out, got.Translations, *in.Translations)
	}
}

// TestREQ176_REQ170_RepeatedHashKeyRefused pins that an OPT repeating the key
// of a Hash entry, which the pinned schemas do not forbid, is refused in every
// mode rather than decoded with the earlier entry replaced: a corpus OPT given
// a second `original_author` with the `id` of an earlier one fails with
// canxml.ErrInvalidShape, naming the entry by its path and not its key.
func TestREQ176_REQ170_RepeatedHashKeyRefused(t *testing.T) {
	raw, err := os.ReadFile("../../../testkit/corpus/crossformat/consult_record/template.opt")
	if err != nil {
		t.Fatalf("read the corpus OPT: %v", err)
	}
	const last = `<original_author id="date">2023-03-30</original_author>`
	if !strings.Contains(string(raw), last) {
		t.Fatalf("the corpus OPT holds no %s", last)
	}
	doc := strings.Replace(string(raw), last, last+`<original_author id="email">second@example.org</original_author>`, 1)
	var reported []string
	for mode, opts := range decodeModes(&reported) {
		var ot aom14.OperationalTemplate
		err := canxml.Unmarshal([]byte(doc), &ot, opts...)
		if !errors.Is(err, canxml.ErrInvalidShape) {
			t.Fatalf("canxml.Unmarshal(%s) = %v; want canxml.ErrInvalidShape", mode, err)
		}
		const path = "/template[1]/description[1]/original_author[5]"
		if de, ok := errors.AsType[*canxml.DecodeError](err); !ok || de == nil || de.Path != path {
			t.Errorf("canxml.Unmarshal(%s) = %v; want a DecodeError naming %s", mode, err, path)
		}
		if msg := err.Error(); strings.Contains(msg, "email") || strings.Contains(msg, "example.org") {
			t.Errorf("canxml.Unmarshal(%s) error %q quotes the entry's key or value", mode, msg)
		}
	}
}

// TestREQ176_REQ056_LanguageKeyedEntryUnderAnotherKeyRefused pins that the
// encoder of a Hash keyed by its values' language, which writes no key, refuses
// an entry held under another key than its value's language code, or under the
// empty key, which would read back under the language or not at all: with
// canxml.ErrInvalidShape and none of the key.
func TestREQ176_REQ056_LanguageKeyedEntryUnderAnotherKeyRefused(t *testing.T) {
	lang := func(code string) rm.CodePhrase {
		return rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "ISO_639-1"}, CodeString: code}
	}
	const key = "secret-key"
	for name, v := range map[string]any{
		"RESOURCE_DESCRIPTION.details under another key": &rm.ResourceDescription{
			OriginalAuthor: map[string]string{"name": "A"},
			Details: map[string]rm.ResourceDescriptionItem{
				key:  {Language: lang("de"), Purpose: "p"},
				"de": {Language: lang("de"), Purpose: "q"},
			},
		},
		"RESOURCE_DESCRIPTION.details under the empty key": &rm.ResourceDescription{
			OriginalAuthor: map[string]string{"name": "A"},
			Details:        map[string]rm.ResourceDescriptionItem{"": {Language: lang(""), Purpose: "p"}},
		},
		"ARCHETYPE.translations under another key": &aom14.Archetype{
			ArchetypeID:      rm.ArchetypeID{Value: "openEHR-EHR-OBSERVATION.blood_pressure.v2"},
			Concept:          "at0000",
			OriginalLanguage: lang("en"),
			Translations:     &map[string]rm.TranslationDetails{key: {Language: lang("nl"), Author: map[string]string{"name": "V"}}},
		},
	} {
		t.Run(name, func(t *testing.T) {
			out, err := canxml.Marshal(v)
			if !errors.Is(err, canxml.ErrInvalidShape) {
				t.Fatalf("canxml.Marshal = %s, %v; want canxml.ErrInvalidShape", out, err)
			}
			if strings.Contains(err.Error(), key) {
				t.Errorf("canxml.Marshal error %q quotes the key", err)
			}
		})
	}
}
