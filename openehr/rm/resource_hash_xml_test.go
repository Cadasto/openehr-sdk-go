package rm_test

// REQ-176 § Round trip, REQ-056 — canonical XML writes the Hash members of the
// RM resource classes in the AM Release-1.4 shapes: a Hash<String,String> as
// one element per entry, the key in its `id` attribute and the value as its
// text; RESOURCE_DESCRIPTION's details one element per value with no `id`,
// keyed by the value's language code; entries in ascending key order.

import (
	"encoding/xml"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml"
)

// language returns the CODE_PHRASE of an ISO 639-1 language code.
func language(code string) rm.CodePhrase {
	return rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "ISO_639-1"}, CodeString: code}
}

// resourceDescription is a description that fills every Hash member of the
// three resource classes, its maps written out of key order.
func resourceDescription() *rm.ResourceDescription {
	return &rm.ResourceDescription{
		OriginalAuthor: map[string]string{"name": "A. Author", "email": "a@example.org", "date": "2026-11-03"},
		LifecycleState: "published",
		OtherDetails:   &map[string]string{"MD5-CAM-1.0.1": "0F1E", "build_uid": "b-1"},
		Details: map[string]rm.ResourceDescriptionItem{
			"nl": {
				Language: language("nl"), Purpose: "Doel", Keywords: []string{"bloeddruk"},
				OriginalResourceURI: &map[string]string{"source": "urn:x"},
				OtherDetails:        &map[string]string{"note": "n"},
			},
			"en": {Language: language("en"), Purpose: "Purpose"},
		},
	}
}

// elementsNamed returns the start elements named local in doc, in document
// order.
func elementsNamed(t *testing.T, doc []byte, local string) []xml.StartElement {
	t.Helper()
	var out []xml.StartElement
	dec := xml.NewDecoder(strings.NewReader(string(doc)))
	for {
		tok, err := dec.Token()
		if err != nil {
			return out
		}
		if se, ok := tok.(xml.StartElement); ok && se.Name.Local == local {
			out = append(out, se.Copy())
		}
	}
}

// attrValues returns the value of the attribute name on each of elems, or "-"
// where an element carries none.
func attrValues(elems []xml.StartElement, name string) []string {
	out := make([]string, len(elems))
	for i, e := range elems {
		out[i] = "-"
		for _, a := range e.Attr {
			if a.Name.Local == name {
				out[i] = a.Value
			}
		}
	}
	return out
}

// TestREQ176_REQ056_ResourceDescriptionHashRoundTrip pins the round trip of
// every Hash shape RESOURCE_DESCRIPTION holds: it encodes, decodes back in
// strict mode with nothing unmatched and nothing missing, and the decoded
// value equals the one encoded.
func TestREQ176_REQ056_ResourceDescriptionHashRoundTrip(t *testing.T) {
	in := resourceDescription()
	doc, err := canxml.Marshal(in)
	if err != nil {
		t.Fatalf("canxml.Marshal = %v; want nil", err)
	}
	var got rm.ResourceDescription
	var unmatched, missing []string
	if err := canxml.Unmarshal(doc, &got, canxml.WithStrict(),
		canxml.WithUnmatched(func(p string) { unmatched = append(unmatched, p) }),
		canxml.WithMissing(func(p string) { missing = append(missing, p) })); err != nil {
		t.Fatalf("canxml.Unmarshal(%s, strict) = %v; want nil", doc, err)
	}
	if len(unmatched) > 0 || len(missing) > 0 {
		t.Errorf("decoding %s reported unmatched %q and missing %q; want neither", doc, unmatched, missing)
	}
	if !reflect.DeepEqual(&got, in) {
		t.Errorf("round trip of %s = %#v; want %#v", doc, got, *in)
	}
}

// TestREQ176_REQ056_StringHashShape pins the shape of a Hash<String,String>:
// one element per entry, named after the member, its key in `id` and its
// value as its text, the entries in ascending order of their keys whatever
// the order of the map.
func TestREQ176_REQ056_StringHashShape(t *testing.T) {
	authors := map[string]string{}
	var keys []string
	for i := range 16 {
		k := fmt.Sprintf("k%02d", (i*7)%16)
		authors[k] = "v" + k
		keys = append(keys, k)
	}
	slices.Sort(keys)
	doc, err := canxml.Marshal(&rm.ResourceDescription{OriginalAuthor: authors, LifecycleState: "x"})
	if err != nil {
		t.Fatalf("canxml.Marshal = %v; want nil", err)
	}
	elems := elementsNamed(t, doc, "original_author")
	if got := attrValues(elems, "id"); !slices.Equal(got, keys) {
		t.Errorf("original_author ids = %q; want each key once, ascending, %q", got, keys)
	}
	for _, k := range keys {
		if want := `<original_author id="` + k + `">v` + k + `</original_author>`; !strings.Contains(string(doc), want) {
			t.Errorf("%s lacks %s", doc, want)
		}
	}
}

// TestREQ176_REQ056_ResourceClassesCarryNoID pins the shape of the resource
// classes as a Hash holds them: RESOURCE_DESCRIPTION's details are written
// with no `id`, in ascending order of their language codes, and decode keyed
// by the language code; an `id` a BASE Release-1.2.0 producer writes on them
// is unmatched content.
func TestREQ176_REQ056_ResourceClassesCarryNoID(t *testing.T) {
	doc, err := canxml.Marshal(resourceDescription())
	if err != nil {
		t.Fatalf("canxml.Marshal = %v; want nil", err)
	}
	details := elementsNamed(t, doc, "details")
	if len(details) != 2 {
		t.Fatalf("%s has %d details elements; want 2", doc, len(details))
	}
	for _, d := range details {
		if len(d.Attr) > 0 {
			t.Errorf("a details element carries the attributes %v; want none", d.Attr)
		}
	}
	if i, j := strings.Index(string(doc), "<code_string>en<"), strings.Index(string(doc), "<code_string>nl<"); i < 0 || j < i {
		t.Errorf("%s does not write the en details before the nl ones", doc)
	}

	base := []byte(`<description xmlns="http://schemas.openehr.org/v1"><original_author id="name">A</original_author>` +
		`<lifecycle_state>x</lifecycle_state><details id="en"><language><terminology_id><value>ISO_639-1</value></terminology_id>` +
		`<code_string>en</code_string></language><purpose>p</purpose></details></description>`)
	var got rm.ResourceDescription
	var unmatched []string
	if err := canxml.Unmarshal(base, &got, canxml.WithUnmatched(func(p string) { unmatched = append(unmatched, p) })); err != nil {
		t.Fatalf("canxml.Unmarshal = %v; want nil", err)
	}
	if want := []string{"/description[1]/details[1]/@id"}; !slices.Equal(unmatched, want) {
		t.Errorf("unmatched %q; want %q", unmatched, want)
	}
	if d, ok := got.Details["en"]; !ok || d.Purpose != "p" {
		t.Errorf("Details = %#v; want the entry keyed by its language code en", got.Details)
	}
}

// TestREQ176_REQ056_TranslationDetailsHashRoundTrip pins the round trip of
// TRANSLATION_DETAILS' two Hash<String,String> members, the mandatory author
// and the optional other_details.
func TestREQ176_REQ056_TranslationDetailsHashRoundTrip(t *testing.T) {
	in := &rm.TranslationDetails{
		Language:     language("de"),
		Author:       map[string]string{"organisation": "O", "name": "N"},
		OtherDetails: &map[string]string{"review": "r"},
	}
	doc, err := canxml.Marshal(in)
	if err != nil {
		t.Fatalf("canxml.Marshal = %v; want nil", err)
	}
	if got := attrValues(elementsNamed(t, doc, "author"), "id"); !slices.Equal(got, []string{"name", "organisation"}) {
		t.Errorf("author ids = %q; want [name organisation]", got)
	}
	var got rm.TranslationDetails
	if err := canxml.Unmarshal(doc, &got, canxml.WithStrict()); err != nil {
		t.Fatalf("canxml.Unmarshal(%s, strict) = %v; want nil", doc, err)
	}
	if !reflect.DeepEqual(&got, in) {
		t.Errorf("round trip of %s = %#v; want %#v", doc, got, *in)
	}
}

// TestREQ176_REQ056_HashMissingMembers pins the missing-member report around
// Hash members: a mandatory Hash with no element is missing, one whose only
// entry is empty is not, and a member a details entry leaves out is named
// under the entry's key, its language code.
func TestREQ176_REQ056_HashMissingMembers(t *testing.T) {
	const lang = `<language><terminology_id><value>ISO_639-1</value></terminology_id><code_string>en</code_string></language>`
	for _, tc := range []struct {
		name, body string
		want       []string
	}{
		{name: "no original_author", body: `<lifecycle_state>x</lifecycle_state><details>` + lang + `<purpose>p</purpose></details>`, want: []string{"/original_author"}},
		{name: "an empty original_author entry", body: `<original_author id="name"/><lifecycle_state>x</lifecycle_state><details>` + lang + `<purpose>p</purpose></details>`},
		{name: "a details entry without purpose", body: `<original_author id="name">A</original_author><lifecycle_state>x</lifecycle_state><details>` + lang + `</details>`, want: []string{"/details/en/purpose"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc := []byte(`<description xmlns="http://schemas.openehr.org/v1">` + tc.body + `</description>`)
			var missing []string
			if err := canxml.Unmarshal(doc, new(rm.ResourceDescription), canxml.WithMissing(func(p string) { missing = append(missing, p) })); err != nil {
				t.Fatalf("canxml.Unmarshal = %v; want nil", err)
			}
			if !slices.Equal(missing, tc.want) {
				t.Errorf("missing %q; want %q", missing, tc.want)
			}
		})
	}
}

// TestREQ176_REQ056_EmptyHashWritesNothing pins that a nil or empty Hash
// member, mandatory or optional, writes no element and does not fail the
// encode, as an empty list does in canonical XML.
func TestREQ176_REQ056_EmptyHashWritesNothing(t *testing.T) {
	for _, v := range []*rm.ResourceDescription{
		{LifecycleState: "x"},
		{LifecycleState: "x", OriginalAuthor: map[string]string{}, Details: map[string]rm.ResourceDescriptionItem{}, OtherDetails: &map[string]string{}},
	} {
		doc, err := canxml.Marshal(v)
		if err != nil {
			t.Fatalf("canxml.Marshal(%#v) = %v; want nil", *v, err)
		}
		for _, name := range []string{"original_author", "other_details", "details"} {
			if n := len(elementsNamed(t, doc, name)); n > 0 {
				t.Errorf("%s writes %d %s elements; want none", doc, n, name)
			}
		}
	}
}
