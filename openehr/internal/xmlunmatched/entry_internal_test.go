package xmlunmatched

import (
	"encoding/xml"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
)

// The targets below read a Hash member as the generated decoders do
// (REQ-176 § Round trip): one element per entry, the key of a String value
// in the entry's `id` attribute, the key of a class value in its `id`
// attribute or else read from the decoded value, and one mark per member,
// false as the element opens.

// entryItem is a class value of a Hash, a cut-down RESOURCE_DESCRIPTION_ITEM:
// its language and its purpose are mandatory.
type entryItem struct {
	Language string
	Purpose  string
}

var entryItemClass = &Class{Name: "ITEM", Required: []string{"language", "purpose"}}

func (v *entryItem) UnmarshalXML(dec *xml.Decoder, start xml.StartElement) error {
	if err := Enter(dec, start, entryItemClass); err != nil {
		return err
	}
	for {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		switch tk := tok.(type) {
		case xml.StartElement:
			switch LocalName(tk.Name) {
			case "language":
				if err := DecodeElement(dec, &tk, &v.Language); err != nil {
					return err
				}
			case "purpose":
				if err := DecodeElement(dec, &tk, &v.Purpose); err != nil {
					return err
				}
			default:
				if err := SkipUnmatched(dec, tk); err != nil {
					return err
				}
			}
		case xml.CharData:
			if err := CheckUnmatchedText(dec, tk); err != nil {
				return err
			}
		case xml.EndElement:
			return nil
		}
	}
}

// entryHolder holds three Hash members: author, of String values keyed by
// `id`; details, of class values keyed by their language; and parts, of
// class values keyed by `id`. author and details are mandatory.
type entryHolder struct {
	Author  map[string]string
	Details map[string]entryItem
	Parts   map[string]entryItem
}

var entryHolderClass = &Class{Name: "HOLDER", Required: []string{"author", "details"}}

func (h *entryHolder) UnmarshalXML(dec *xml.Decoder, start xml.StartElement) error {
	if err := Enter(dec, start, entryHolderClass); err != nil {
		return err
	}
	var author, details, parts bool
	for {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		switch tk := tok.(type) {
		case xml.StartElement:
			switch LocalName(tk.Name) {
			case "author":
				if err := DecodeEntry(dec, &tk, "id", &h.Author, &author); err != nil {
					return err
				}
			case "details":
				if err := DecodeEntryKeyedBy(dec, &tk, func(v *entryItem) string { return v.Language }, &h.Details, &details); err != nil {
					return err
				}
			case "parts":
				if err := DecodeEntry(dec, &tk, "id", &h.Parts, &parts); err != nil {
					return err
				}
			default:
				if err := SkipUnmatched(dec, tk); err != nil {
					return err
				}
			}
		case xml.CharData:
			if err := CheckUnmatchedText(dec, tk); err != nil {
				return err
			}
		case xml.EndElement:
			return nil
		}
	}
}

// entriesDoc is a document of every Hash shape: two String entries, one of
// them empty, two language-keyed class entries and one `id`-keyed class
// entry.
const entriesDoc = `<holder ` + xmlnsDecl + `>` +
	`<author id="b">B</author><author id="a"></author>` +
	`<details><language>en</language><purpose>p</purpose></details>` +
	`<details><language>de</language><purpose>q</purpose></details>` +
	`<parts id="x"><language>fr</language><purpose>r</purpose></parts>` +
	`</holder>`

// entriesValue is the value entriesDoc decodes to.
var entriesValue = entryHolder{
	Author:  map[string]string{"a": "", "b": "B"},
	Details: map[string]entryItem{"en": {Language: "en", Purpose: "p"}, "de": {Language: "de", Purpose: "q"}},
	Parts:   map[string]entryItem{"x": {Language: "fr", Purpose: "r"}},
}

// TestREQ176_DecodeEntryKeys pins that each Hash entry lands in its map under
// its key, in an untracked decode and in a strict one alike: a String value
// and a class value under the `id` they carry, and a class value with no key
// attribute under the key read from it.
func TestREQ176_DecodeEntryKeys(t *testing.T) {
	var plain entryHolder
	if err := xml.Unmarshal([]byte(entriesDoc), &plain); err != nil {
		t.Fatalf("xml.Unmarshal = %v; want nil", err)
	}
	if !reflect.DeepEqual(plain, entriesValue) {
		t.Errorf("untracked decode = %#v; want %#v", plain, entriesValue)
	}
	var strict entryHolder
	tr := NewTracker(nil, true)
	if err := tr.Decode(tr.NewDecoder(strings.NewReader(entriesDoc)), &strict); err != nil {
		t.Fatalf("strict decode = %v; want nil: a key attribute is read, not unmatched", err)
	}
	if !reflect.DeepEqual(strict, entriesValue) {
		t.Errorf("strict decode = %#v; want %#v", strict, entriesValue)
	}
}

// TestREQ176_EntryKeyAttributeNotReported pins that the decode takes an
// entry's key attribute off before it reads the value, so the report option
// never names it, on a String value or on a class value whose own decoder
// checks the attributes; every other attribute of the entry, and a child
// element inside a String value, stays unmatched.
func TestREQ176_EntryKeyAttributeNotReported(t *testing.T) {
	doc := `<holder ` + xmlnsDecl + `>` +
		`<author id="a" lang="x">A<note/></author>` +
		`<details><language>en</language><purpose>p</purpose></details>` +
		`<parts id="x" extra="y"><language>fr</language><purpose>r</purpose></parts>` +
		`</holder>`
	var got entryHolder
	var paths []string
	tr := NewTracker(func(p string) { paths = append(paths, p) }, false)
	if err := tr.Decode(tr.NewDecoder(strings.NewReader(doc)), &got); err != nil {
		t.Fatalf("Decode = %v; want nil", err)
	}
	want := []string{"/holder[1]/author[1]/@lang", "/holder[1]/author[1]/note[1]", "/holder[1]/parts[1]/@extra"}
	if !slices.Equal(paths, want) {
		t.Errorf("reported %q; want %q, and no key attribute", paths, want)
	}
	if got.Author["a"] != "A" || got.Parts["x"].Purpose != "r" {
		t.Errorf("decoded %#v; want author a = \"A\" and part x's purpose \"r\"", got)
	}
}

// TestREQ176_EntryMissingMemberPointers pins how the missing-member report
// names what lies inside a Hash entry: by the entry's key, escaped as RFC 6901
// asks (`~` as `~0`, `/` as `~1`), whether the key is an attribute or read from
// the value; and that a Hash with an entry is present, even when the entry's
// String value is empty, while a Hash with none is missing.
func TestREQ176_EntryMissingMemberPointers(t *testing.T) {
	for _, tc := range []struct {
		name string
		doc  string
		want []string
	}{
		{name: "every member present", doc: entriesDoc},
		{
			name: "an empty String entry is an entry",
			doc: `<holder ` + xmlnsDecl + `><author id="a"></author>` +
				`<details><language>en</language><purpose>p</purpose></details></holder>`,
		},
		{
			name: "a missing member inside a language-keyed entry",
			doc: `<holder ` + xmlnsDecl + `><author id="a">A</author>` +
				`<details><language>de</language></details></holder>`,
			want: []string{"/details/de/purpose"},
		},
		{
			name: "a missing member inside an id-keyed entry, its key escaped",
			doc: `<holder ` + xmlnsDecl + `><author id="a">A</author>` +
				`<details><language>en</language><purpose>p</purpose></details>` +
				`<parts id="a/b~c"><purpose>r</purpose></parts></holder>`,
			want: []string{"/parts/a~1b~0c/language"},
		},
		{
			name: "a Hash with no entry",
			doc:  `<holder ` + xmlnsDecl + `><author id="a">A</author></holder>`,
			want: []string{"/details"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := decodeMissing(tc.doc, new(entryHolder), false)
			if err != nil {
				t.Fatalf("Decode = %v; want nil", err)
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("missing members %q; want %q", got, tc.want)
			}
		})
	}
}

// TestREQ176_EntryValueFailureNamesNoValue pins that a class value that fails
// to decode fails the entry with its own error, and the entry adds nothing to
// the map.
func TestREQ176_EntryValueFailureNamesNoValue(t *testing.T) {
	doc := `<holder ` + xmlnsDecl + `><parts id="x"><language>fr</purpose></parts></holder>`
	var got entryHolder
	err := xml.Unmarshal([]byte(doc), &got)
	if syntax, ok := errors.AsType[*xml.SyntaxError](err); !ok || syntax == nil {
		t.Fatalf("xml.Unmarshal = %v; want the syntax error of the malformed value", err)
	}
	if len(got.Parts) != 0 {
		t.Errorf("Parts = %#v; want no entry for a value that failed", got.Parts)
	}
}

// REQ-170: a Hash entry whose key an earlier entry of the member holds is
// unmatched content, and decode refuses, in every mode, an entry that has no
// key.

// entryModes are the decodes a Hash entry with no key is refused in alike: a
// whole document read untracked, as canxml.Unmarshal reads one given no
// option, and read through a tracker that reports unmatched content or
// refuses it.
var entryModes = map[string]func(doc string, v any) error{
	"default": func(doc string, v any) error { return Unmarshal([]byte(doc), v, nil) },
	"report":  func(doc string, v any) error { return Unmarshal([]byte(doc), v, NewTracker(func(string) {}, false)) },
	"strict":  func(doc string, v any) error { return Unmarshal([]byte(doc), v, NewTracker(nil, true)) },
}

// TestREQ176_REQ170_EntryRepeatedKeyUnmatched pins that a Hash entry whose key
// an earlier entry of the same member holds is unmatched content, whether its
// value is a String or a class keyed by an attribute or by its language. The
// earlier entry is kept, though the repeat's value differs, so the default
// decode reads the member as if the repeat were absent. The report option
// reports the repeat's element once, by its path, and nothing inside it: not
// the attributes and child elements it holds besides its value, nor, for the
// missing-member report, the mandatory members it leaves out. Strict refuses
// it with ErrUnknownElement, naming that path and none of its key or value.
func TestREQ176_REQ170_EntryRepeatedKeyUnmatched(t *testing.T) {
	const key, repeat = "secret-key", "secret-value"
	item := func(lang, purpose string) string {
		return `<language>` + lang + `</language><purpose>` + purpose + `</purpose>`
	}
	for _, tc := range []struct {
		name, body, path string
		want             entryHolder
	}{
		{
			name: "a String entry repeating an id",
			body: `<author id="` + key + `">A</author><author id="b">B</author>` +
				`<author id="` + key + `" lang="x">` + repeat + `<note/></author>` +
				`<details>` + item("en", "p") + `</details>`,
			path: "/holder[1]/author[3]",
			want: entryHolder{
				Author:  map[string]string{key: "A", "b": "B"},
				Details: map[string]entryItem{"en": {Language: "en", Purpose: "p"}},
			},
		},
		{
			name: "a class entry repeating an id",
			body: `<author id="a">A</author><details>` + item("en", "p") + `</details>` +
				`<parts id="` + key + `">` + item("fr", "r") + `</parts>` +
				`<parts id="` + key + `" extra="y">` + item("de", repeat) + `<note/></parts>`,
			path: "/holder[1]/parts[2]",
			want: entryHolder{
				Author:  map[string]string{"a": "A"},
				Details: map[string]entryItem{"en": {Language: "en", Purpose: "p"}},
				Parts:   map[string]entryItem{key: {Language: "fr", Purpose: "r"}},
			},
		},
		{
			name: "a language-keyed entry repeating a language",
			body: `<author id="a">A</author><details>` + item(key, "p") + `</details>` +
				`<details extra="y">` + item(key, repeat) + `<note/></details>`,
			path: "/holder[1]/details[2]",
			want: entryHolder{
				Author:  map[string]string{"a": "A"},
				Details: map[string]entryItem{key: {Language: key, Purpose: "p"}},
			},
		},
		{
			name: "a language-keyed entry repeating a language, a mandatory member left out",
			body: `<author id="a">A</author><details>` + item(key, "p") + `</details>` +
				`<details><language>` + key + `</language></details>`,
			path: "/holder[1]/details[2]",
			want: entryHolder{
				Author:  map[string]string{"a": "A"},
				Details: map[string]entryItem{key: {Language: key, Purpose: "p"}},
			},
		},
	} {
		doc := `<holder ` + xmlnsDecl + `>` + tc.body + `</holder>`
		t.Run(tc.name, func(t *testing.T) {
			var plain entryHolder
			if err := Unmarshal([]byte(doc), &plain, nil); err != nil || !reflect.DeepEqual(plain, tc.want) {
				t.Errorf("default decode = %#v, %v; want %#v, the earlier entry kept", plain, err, tc.want)
			}
			var reported entryHolder
			var paths []string
			if err := Unmarshal([]byte(doc), &reported, NewTracker(func(p string) { paths = append(paths, p) }, false)); err != nil || !reflect.DeepEqual(reported, tc.want) {
				t.Errorf("report decode = %#v, %v; want %#v, the earlier entry kept", reported, err, tc.want)
			}
			if want := []string{tc.path}; !slices.Equal(paths, want) {
				t.Errorf("reported %q; want %q, the repeat as one item", paths, want)
			}
			err := Unmarshal([]byte(doc), new(entryHolder), NewTracker(nil, true))
			if ue, ok := errors.AsType[*unknownElementError](err); !ok || ue == nil || ue.path != tc.path {
				t.Errorf("strict decode = %v; want the refusal of %s as unmatched content", err, tc.path)
			}
			if !errors.Is(err, ErrUnknownElement) || errors.Is(err, ErrInvalidShape) {
				t.Errorf("strict decode = %v; want an error wrapping ErrUnknownElement and not ErrInvalidShape", err)
			}
			if err != nil && (strings.Contains(err.Error(), key) || strings.Contains(err.Error(), repeat)) {
				t.Errorf("strict decode error %q quotes the repeat's key or value", err)
			}
			missing, err := decodeMissing(doc, new(entryHolder), false)
			if err != nil || len(missing) > 0 {
				t.Errorf("missing-member decode = %q, %v; want nothing missing, the repeat unread", missing, err)
			}
		})
	}
}

// TestREQ176_REQ170_LanguageKeyedEntryContentKept pins that what a
// language-keyed Hash entry that repeats no key holds besides what it reads
// is unmatched as in any element, though the decode learns the entry's key
// only once the entry has closed: an attribute and a child element are
// reported in document order, with the mandatory members it leaves out
// between them as the missing-member report finds them, and before what
// follows the entry; strict refuses the first, with ErrNamespace too for a
// child in a foreign namespace, even when the entry fails to decode after it;
// and an entry with no language reports what it holds before it is refused.
func TestREQ176_REQ170_LanguageKeyedEntryContentKept(t *testing.T) {
	doc := `<holder ` + xmlnsDecl + `><author id="a">A</author>` +
		`<details><language>en</language><purpose>p</purpose></details>` +
		`<details extra="y"><note/><language>de</language></details>` +
		`<other/></holder>`
	var got []string
	tr := NewTracker(func(p string) { got = append(got, "unmatched "+p) }, false)
	tr.ReportMissing(func(p string) { got = append(got, "missing "+p) })
	var v entryHolder
	if err := tr.Decode(tr.NewDecoder(strings.NewReader(doc)), &v); err != nil {
		t.Fatalf("Decode = %v; want nil", err)
	}
	want := []string{
		"unmatched /holder[1]/details[2]/@extra",
		"unmatched /holder[1]/details[2]/note[1]",
		"missing /details/de/purpose",
		"unmatched /holder[1]/other[1]",
	}
	if !slices.Equal(got, want) {
		t.Errorf("reported %q; want %q", got, want)
	}
	if d, ok := v.Details["de"]; !ok || d.Language != "de" {
		t.Errorf("Details = %#v; want the de entry", v.Details)
	}
	for _, tc := range []struct {
		name, body, path string
		foreign          bool
	}{
		{name: "an entry that decodes", body: `<details><language>en</language><purpose>p</purpose></details><details><note/><language>de</language><purpose>q</purpose></details>`, path: "/holder[1]/details[2]/note[1]"},
		{name: "an entry with a foreign child", body: `<details><x:note xmlns:x="urn:other"/><language>de</language><purpose>q</purpose></details>`, path: "/holder[1]/details[1]/note[1]", foreign: true},
		{name: "an entry that fails after its unmatched child", body: `<details><note/><language>de</purpose></details>`, path: "/holder[1]/details[1]/note[1]"},
		{name: "an entry with no language", body: `<details><note/><purpose>q</purpose></details>`, path: "/holder[1]/details[1]/note[1]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc := `<holder ` + xmlnsDecl + `><author id="a">A</author>` + tc.body + `</holder>`
			err := Unmarshal([]byte(doc), new(entryHolder), NewTracker(nil, true))
			if ue, ok := errors.AsType[*unknownElementError](err); !ok || ue == nil || ue.path != tc.path {
				t.Errorf("strict decode = %v; want the refusal of %s", err, tc.path)
			}
			if got := errors.Is(err, ErrNamespace); got != tc.foreign {
				t.Errorf("strict decode = %v; errors.Is(_, ErrNamespace) = %v, want %v", err, got, tc.foreign)
			}
		})
	}
	var paths []string
	doc = `<holder ` + xmlnsDecl + `><author id="a">A</author><details><note/><purpose>q</purpose></details></holder>`
	err := Unmarshal([]byte(doc), new(entryHolder), NewTracker(func(p string) { paths = append(paths, p) }, false))
	if de, ok := errors.AsType[*typereg.DecodeError](err); !ok || de == nil || de.Path != "/holder[1]/details[1]" || !errors.Is(err, ErrInvalidShape) {
		t.Errorf("report decode of an entry with no language = %v; want ErrInvalidShape naming /holder[1]/details[1]", err)
	}
	if want := []string{"/holder[1]/details[1]/note[1]"}; !slices.Equal(paths, want) {
		t.Errorf("report decode of an entry with no language reported %q; want %q", paths, want)
	}
}

// TestREQ176_REQ170_EntryMissingKeyRefused pins that a Hash entry that has no
// key is refused in every mode, whether its value is a String or a class keyed
// by an attribute or by its language: the error wraps ErrInvalidShape and is
// a *typereg.DecodeError naming the entry by its path from the root. An empty
// `id` is a key.
func TestREQ176_REQ170_EntryMissingKeyRefused(t *testing.T) {
	item := func(lang string) string {
		return `<language>` + lang + `</language><purpose>p</purpose>`
	}
	for _, tc := range []struct {
		name, body, path string
	}{
		{
			name: "a String entry with no id",
			body: `<author id="a">A</author><author>B</author><details>` + item("en") + `</details>`,
			path: "/holder[1]/author[2]",
		},
		{
			name: "a class entry with no id",
			body: `<author id="a">A</author><details>` + item("en") + `</details><parts>` + item("fr") + `</parts>`,
			path: "/holder[1]/parts[1]",
		},
		{
			name: "a language-keyed entry with no language",
			body: `<author id="a">A</author><details>` + item("en") + `</details><details><purpose>q</purpose></details>`,
			path: "/holder[1]/details[2]",
		},
	} {
		doc := `<holder ` + xmlnsDecl + `>` + tc.body + `</holder>`
		for mode, decode := range entryModes {
			t.Run(tc.name+"/"+mode, func(t *testing.T) {
				err := decode(doc, new(entryHolder))
				if !errors.Is(err, ErrInvalidShape) {
					t.Fatalf("decode = %v; want an error wrapping ErrInvalidShape", err)
				}
				de, ok := errors.AsType[*typereg.DecodeError](err)
				if !ok || de == nil || de.Path != tc.path {
					t.Errorf("decode = %v; want a *typereg.DecodeError naming %s", err, tc.path)
				}
			})
		}
	}
	for mode, decode := range entryModes {
		var got entryHolder
		doc := `<holder ` + xmlnsDecl + `><author id="">A</author><details>` + item("en") + `</details></holder>`
		if err := decode(doc, &got); err != nil || got.Author[""] != "A" {
			t.Errorf("%s decode of an entry with an empty id = %#v, %v; want it under the empty key", mode, got.Author, err)
		}
	}
}

// countHolder holds a Hash member and a whole number after it, whose text may
// fail to decode.
type countHolder struct {
	Author map[string]string
	Count  int
}

var countHolderClass = &Class{Name: "COUNTED"}

func (h *countHolder) UnmarshalXML(dec *xml.Decoder, start xml.StartElement) error {
	if err := Enter(dec, start, countHolderClass); err != nil {
		return err
	}
	var author bool
	for {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		switch tk := tok.(type) {
		case xml.StartElement:
			switch LocalName(tk.Name) {
			case "author":
				if err := DecodeEntry(dec, &tk, "id", &h.Author, &author); err != nil {
					return err
				}
			case "count":
				if err := DecodeElement(dec, &tk, &h.Count); err != nil {
					return err
				}
			default:
				if err := SkipUnmatched(dec, tk); err != nil {
					return err
				}
			}
		case xml.EndElement:
			return nil
		}
	}
}

// TestREQ176_EntryKeysOfASecondDecode pins that the entries a first decode put
// in a map are no earlier entries of the document: a document with a primitive
// failure after a Hash member, which Unmarshal given no tracker decodes a
// second time into the same value to name the failure's path, fails at that
// element, not at an entry of the member.
func TestREQ176_EntryKeysOfASecondDecode(t *testing.T) {
	doc := `<counted ` + xmlnsDecl + `><author id="a">A</author><author id="b">B</author><count>many</count></counted>`
	err := Unmarshal([]byte(doc), new(countHolder), nil)
	if de, ok := errors.AsType[*typereg.DecodeError](err); !ok || de == nil || de.Path != "/counted[1]/count[1]" || !IsPrimitiveFailure(err) {
		t.Errorf("Unmarshal = %v; want the primitive failure of /counted[1]/count[1]", err)
	}
}

// TestREQ176_REQ170_EntriesStartEmptyAtTheFirstEntry pins that a Hash
// member's map starts empty at the element's first entry of the member, in
// every mode: a value whose members already hold entries, decoded into from a
// document that holds one of their keys once, decodes, and each member holds
// the document's entries alone, as a value decoded into twice holds the
// second document's.
func TestREQ176_REQ170_EntriesStartEmptyAtTheFirstEntry(t *testing.T) {
	doc := `<holder ` + xmlnsDecl + `><author id="a">A2</author>` +
		`<details><language>en</language><purpose>p2</purpose></details>` +
		`<parts id="x"><language>fr</language><purpose>r2</purpose></parts></holder>`
	want := entryHolder{
		Author:  map[string]string{"a": "A2"},
		Details: map[string]entryItem{"en": {Language: "en", Purpose: "p2"}},
		Parts:   map[string]entryItem{"x": {Language: "fr", Purpose: "r2"}},
	}
	for mode, decode := range entryModes {
		got := entryHolder{
			Author:  map[string]string{"a": "A1", "b": "B1"},
			Details: map[string]entryItem{"en": {Language: "en", Purpose: "p1"}, "de": {Language: "de", Purpose: "q1"}},
			Parts:   map[string]entryItem{"x": {Language: "fr", Purpose: "r1"}, "y": {Language: "it", Purpose: "s1"}},
		}
		if err := decode(doc, &got); err != nil {
			t.Errorf("%s decode into a value holding entries = %v; want nil", mode, err)
			continue
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s decode into a value holding entries = %#v; want %#v", mode, got, want)
		}
	}
}

// TestREQ176_REQ052_EntryRefusalsKeepTheirText pins that the text of the
// refusal of a Hash entry with no key keeps the refusal's own text, which
// holds no value, rather than naming it by its Go type alone, in every mode.
// A repeated key is unmatched content, whose strict refusal names its path
// (TestREQ176_REQ170_EntryRepeatedKeyUnmatched).
//
// Can-fail control: drop the text from typereg's whole-message forms and
// each case reads "cause withheld (*errors.errorString)".
func TestREQ176_REQ052_EntryRefusalsKeepTheirText(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		cause      error
	}{
		{name: "no key", body: `<author>A</author>`, cause: errEntryNoKey},
		{name: "no language", body: `<author id="a">A</author><details><purpose>p</purpose></details>`, cause: errEntryNoKey},
	} {
		doc := `<holder ` + xmlnsDecl + `>` + tc.body + `</holder>`
		for mode, decode := range entryModes {
			t.Run(tc.name+"/"+mode, func(t *testing.T) {
				err := decode(doc, new(entryHolder))
				if err == nil {
					t.Fatalf("decode(%s) = nil; want a refusal", doc)
				}
				got, keep := err.Error(), "canxml: invalid XML shape: "+tc.cause.Error()
				if !strings.Contains(got, keep) || strings.Contains(got, "cause withheld") {
					t.Errorf("decode(%s) = %q; want it to keep %q", doc, got, keep)
				}
			})
		}
	}
}
