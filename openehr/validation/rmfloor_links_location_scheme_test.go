package validation_test

// rmfloor_links_location_scheme_test.go: REQ-112 § Per-RM-type invariant
// catalogue (the LOCATABLE links and EVENT_CONTEXT location rules, and the
// DV_EHR_URI scheme rule), REQ-171 § Where the RM rules come from and
// § Structural access, REQ-168. The floor reports a present-and-empty
// `links` on a LOCATABLE and a present-and-empty `location` on an
// EVENT_CONTEXT as rm_invariant at the attribute, and a DV_EHR_URI whose
// value has no `ehr` scheme as rm_invariant at the value, with the
// invariant as the rule, at the root and nested, under a template as
// without one. Presence is the generated access's reading, so a document
// that leaves the attribute out or sends null draws nothing; only a decoded
// `[]` or `""` is present and empty. Each document is decoded from JSON text
// with canjson, because the encoder drops an empty list.

import (
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// The rules of the three invariants, as the floor names them in Issue.Rule.
const (
	linksValid    = "LOCATABLE.Links_valid"
	locationValid = "EVENT_CONTEXT.location_valid"
	schemeValid   = "DV_EHR_URI.Scheme_valid"
)

// decodeJSON decodes the canonical JSON text into a new T, as a server
// receives it.
func decodeJSON[T any](t *testing.T, text string) *T {
	t.Helper()
	v := new(T)
	if err := canjson.Unmarshal([]byte(text), v); err != nil {
		t.Fatalf("canjson.Unmarshal(%s): %v", text, err)
	}
	return v
}

// Fragments of the documents below, in canonical JSON.
const (
	lsName = `"name":{"_type":"DV_TEXT","value":"x"}`
	// lsLink is a complete LINK to an ehr URI.
	lsLink = `{"_type":"LINK","meaning":{"_type":"DV_TEXT","value":"m"},"type":{"_type":"DV_TEXT","value":"t"},"target":{"_type":"DV_EHR_URI","value":"ehr://example.org/x"}}`
)

// lsComposition returns a COMPOSITION that holds the members at its root and
// the observation members in its one OBSERVATION content item.
func lsComposition(root, observation string) string {
	return `{"_type":"COMPOSITION","archetype_node_id":"openEHR-EHR-COMPOSITION.c.v1",` + lsName + root +
		`,"content":[{"_type":"OBSERVATION","archetype_node_id":"openEHR-EHR-OBSERVATION.o.v1",` + lsName + observation + `}]}`
}

// REQ-112 LOCATABLE.Links_valid — `links /= Void implies not links.is_empty`:
// the floor reports a `links` that is present and holds no item, at the
// node's own `links`, on the root and on a LOCATABLE nested under it, with
// the invariant as the rule. A `links` the document leaves out, sends as
// null or fills draws nothing, and so does a typed-nil one built in Go.
func TestValidateRM_REQ112_LinksValid(t *testing.T) {
	t.Parallel()
	const nested = "/content[openEHR-EHR-OBSERVATION.o.v1]/links"
	tests := []struct {
		name string
		root any
		want []string
	}{
		{
			name: "root links present and empty",
			root: decodeJSON[rm.Composition](t, lsComposition(`,"links":[]`, ``)),
			want: []string{"rm_invariant /links rm " + linksValid},
		},
		{name: "root links absent", root: decodeJSON[rm.Composition](t, lsComposition(``, ``))},
		{name: "root links null", root: decodeJSON[rm.Composition](t, lsComposition(`,"links":null`, ``))},
		{name: "root links holding a link", root: decodeJSON[rm.Composition](t, lsComposition(`,"links":[`+lsLink+`]`, ``))},
		{
			name: "nested links present and empty",
			root: decodeJSON[rm.Composition](t, lsComposition(``, `,"links":[]`)),
			want: []string{"rm_invariant " + nested + " rm " + linksValid},
		},
		{name: "nested links absent", root: decodeJSON[rm.Composition](t, lsComposition(``, ``))},
		{name: "nested links null", root: decodeJSON[rm.Composition](t, lsComposition(``, `,"links":null`))},
		{name: "nested links holding a link", root: decodeJSON[rm.Composition](t, lsComposition(``, `,"links":[`+lsLink+`]`))},
		{
			name: "root and nested links present and empty",
			root: decodeJSON[rm.Composition](t, lsComposition(`,"links":[]`, `,"links":[]`)),
			want: []string{"rm_invariant " + nested + " rm " + linksValid, "rm_invariant /links rm " + linksValid},
		},
		{
			name: "an ELEMENT's links present and empty",
			root: decodeJSON[rm.Element](t, `{"_type":"ELEMENT","archetype_node_id":"at0001",`+lsName+`,"value":{"_type":"DV_TEXT","value":"v"},"links":[]}`),
			want: []string{"rm_invariant /links rm " + linksValid},
		},
		{name: "built in Go, a non-nil empty list", root: &rm.Composition{Links: []rm.Link{}}, want: []string{"rm_invariant /links rm " + linksValid}},
		{name: "built in Go, a nil list", root: &rm.Composition{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := identityKeys(withRule(validation.ValidateRM(tc.root), linksValid)); !slices.Equal(got, tc.want) {
				t.Errorf("ValidateRM(%s) issues under %s = %q, want %q (REQ-112 LOCATABLE.Links_valid)", tc.name, linksValid, got, tc.want)
			}
		})
	}
}

// REQ-112 EVENT_CONTEXT.location_valid — `location /= Void implies not
// location.is_empty`: the floor reports a `location` that is present and
// empty, at the context's own `location`, at the root and under a
// COMPOSITION. A `location` the document leaves out, sends as null or fills
// draws nothing.
func TestValidateRM_REQ112_EventContextLocation(t *testing.T) {
	t.Parallel()
	composition := func(location string) string {
		return `{"_type":"COMPOSITION","archetype_node_id":"openEHR-EHR-COMPOSITION.c.v1",` + lsName +
			`,"context":{"_type":"EVENT_CONTEXT"` + location + `}}`
	}
	tests := []struct {
		name string
		root any
		want []string
	}{
		{
			name: "location present and empty",
			root: decodeJSON[rm.EventContext](t, `{"_type":"EVENT_CONTEXT","location":""}`),
			want: []string{"rm_invariant /location rm " + locationValid},
		},
		{name: "location absent", root: decodeJSON[rm.EventContext](t, `{"_type":"EVENT_CONTEXT"}`)},
		{name: "location null", root: decodeJSON[rm.EventContext](t, `{"_type":"EVENT_CONTEXT","location":null}`)},
		{name: "location holding text", root: decodeJSON[rm.EventContext](t, `{"_type":"EVENT_CONTEXT","location":"ward A3"}`)},
		{name: "location holding a space", root: decodeJSON[rm.EventContext](t, `{"_type":"EVENT_CONTEXT","location":" "}`)},
		{
			name: "location present and empty under a COMPOSITION",
			root: decodeJSON[rm.Composition](t, composition(`,"location":""`)),
			want: []string{"rm_invariant /context/location rm " + locationValid},
		},
		{name: "location absent under a COMPOSITION", root: decodeJSON[rm.Composition](t, composition(``))},
		{name: "location holding text under a COMPOSITION", root: decodeJSON[rm.Composition](t, composition(`,"location":"home"`))},
		{name: "built in Go, a pointer to an empty string", root: &rm.EventContext{Location: new("")}, want: []string{"rm_invariant /location rm " + locationValid}},
		{name: "built in Go, a nil pointer", root: &rm.EventContext{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := identityKeys(withRule(validation.ValidateRM(tc.root), locationValid)); !slices.Equal(got, tc.want) {
				t.Errorf("ValidateRM(%s) issues under %s = %q, want %q (REQ-112 EVENT_CONTEXT.location_valid)", tc.name, locationValid, got, tc.want)
			}
		})
	}
}

// ehrURI returns a DV_EHR_URI whose value is value.
func ehrURI(value string) string {
	return `{"_type":"DV_EHR_URI","value":"` + value + `"}`
}

// REQ-112 DV_EHR_URI.Scheme_valid — `scheme.is_equal (Ehr_scheme)`: the floor
// reports a DV_EHR_URI whose value's RFC 3986 scheme is not `ehr`, compared
// without regard to letter case, at the value itself. A value with no scheme
// has none equal to `ehr`, so it is reported too; an empty one is the
// mandatory-presence rule's, so it draws nothing here.
func TestValidateRM_REQ112_DVEHRURIScheme(t *testing.T) {
	t.Parallel()
	const bad = "rm_invariant / rm " + schemeValid
	tests := []struct {
		name, value string
		want        []string
	}{
		{name: "ehr with an authority", value: "ehr://example.org/x"},
		{name: "ehr with an empty authority", value: "ehr://"},
		{name: "ehr with a path", value: "ehr:/x"},
		{name: "ehr alone", value: "ehr:"},
		{name: "EHR in capitals", value: "EHR:"},
		{name: "mixed case with an authority", value: "Ehr://example.org/x"},
		{name: "http", value: "http://example.org/x", want: []string{bad}},
		{name: "urn", value: "urn:ehr:x", want: []string{bad}},
		{name: "a scheme that starts with ehr", value: "ehr+x://y", want: []string{bad}},
		{name: "a scheme with digits and dots", value: "a1.b-c+d:x", want: []string{bad}},
		{name: "a scheme a letter long", value: "x:y", want: []string{bad}},
		{name: "no colon", value: "ehr", want: []string{bad}},
		{name: "a relative reference", value: "//example.org/x", want: []string{bad}},
		{name: "a colon after a slash", value: "/x:y", want: []string{bad}},
		{name: "ehr after a slash", value: "x/ehr:y", want: []string{bad}},
		{name: "a scheme starting with a digit", value: "1http://x", want: []string{bad}},
		{name: "an empty scheme", value: ":x", want: []string{bad}},
		{name: "a space in the scheme", value: "x y:z", want: []string{bad}},
		{name: "a space before ehr", value: " ehr://x", want: []string{bad}},
		{name: "a space alone", value: " ", want: []string{bad}},
		{name: "empty", value: ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root := decodeJSON[rm.DVEHRURI](t, ehrURI(tc.value))
			if got := identityKeys(withRule(validation.ValidateRM(root), schemeValid)); !slices.Equal(got, tc.want) {
				t.Errorf("ValidateRM(DV_EHR_URI %q) issues under %s = %q, want %q (REQ-112 DV_EHR_URI.Scheme_valid)", tc.value, schemeValid, got, tc.want)
			}
		})
	}
}

// REQ-112 DV_EHR_URI.Scheme_valid — the rule runs wherever the walk visits a
// DV_EHR_URI: the target of a LINK under a LOCATABLE's `links`, an ELEMENT's
// value, an ATTESTATION's items, and a DV_EHR_URI sent where a DV_URI is
// declared, a DV_TEXT's `hyperlink` and a DV_MULTIMEDIA's `uri`. Each is
// reported at the URI's own path.
func TestValidateRM_REQ112_DVEHRURISchemeWhereVisited(t *testing.T) {
	t.Parallel()
	link := func(target string) string {
		return `{"_type":"LINK","meaning":{"_type":"DV_TEXT","value":"m"},"type":{"_type":"DV_TEXT","value":"t"},"target":` + target + `}`
	}
	tests := []struct {
		name string
		root any
		want []string
	}{
		{
			name: "a LINK target on the root",
			root: decodeJSON[rm.Composition](t, lsComposition(`,"links":[`+link(ehrURI("http://x"))+`]`, ``)),
			want: []string{"rm_invariant /links[1]/target rm " + schemeValid},
		},
		{
			name: "a LINK target on a nested OBSERVATION",
			root: decodeJSON[rm.Composition](t, lsComposition(``, `,"links":[`+link(ehrURI("ehr://x"))+`,`+link(ehrURI("http://x"))+`]`)),
			want: []string{"rm_invariant /content[openEHR-EHR-OBSERVATION.o.v1]/links[2]/target rm " + schemeValid},
		},
		{
			name: "an ehr LINK target",
			root: decodeJSON[rm.Composition](t, lsComposition(`,"links":[`+link(ehrURI("ehr://x"))+`]`, ``)),
		},
		{
			name: "a LINK target with no scheme",
			root: decodeJSON[rm.Composition](t, lsComposition(`,"links":[`+link(ehrURI("example.org/ehr/123"))+`]`, ``)),
			want: []string{"rm_invariant /links[1]/target rm " + schemeValid},
		},
		{
			name: "an ELEMENT value",
			root: decodeJSON[rm.Element](t, `{"_type":"ELEMENT","archetype_node_id":"at0001",`+lsName+`,"value":`+ehrURI("http://x")+`}`),
			want: []string{"rm_invariant /value rm " + schemeValid},
		},
		{
			name: "an ehr ELEMENT value",
			root: decodeJSON[rm.Element](t, `{"_type":"ELEMENT","archetype_node_id":"at0001",`+lsName+`,"value":`+ehrURI("ehr://x")+`}`),
		},
		{
			name: "an ATTESTATION item",
			root: decodeJSON[rm.Attestation](t, `{"_type":"ATTESTATION","items":[`+ehrURI("ehr://x")+`,`+ehrURI("urn:x")+`]}`),
			want: []string{"rm_invariant /items[2] rm " + schemeValid},
		},
		{
			name: "a DV_TEXT hyperlink",
			root: decodeJSON[rm.DVText](t, `{"_type":"DV_TEXT","value":"v","hyperlink":`+ehrURI("http://x")+`}`),
			want: []string{"rm_invariant /hyperlink rm " + schemeValid},
		},
		{
			name: "an ehr DV_TEXT hyperlink",
			root: decodeJSON[rm.DVText](t, `{"_type":"DV_TEXT","value":"v","hyperlink":`+ehrURI("ehr://x")+`}`),
		},
		{
			name: "a DV_MULTIMEDIA uri",
			root: decodeJSON[rm.DVMultimedia](t, `{"_type":"DV_MULTIMEDIA","media_type":{"terminology_id":{"value":"IANA_media-types"},"code_string":"text/plain"},"size":1,"uri":`+ehrURI("http://x")+`}`),
			want: []string{"rm_invariant /uri rm " + schemeValid},
		},
		{
			name: "an ehr DV_MULTIMEDIA uri",
			root: decodeJSON[rm.DVMultimedia](t, `{"_type":"DV_MULTIMEDIA","media_type":{"terminology_id":{"value":"IANA_media-types"},"code_string":"text/plain"},"size":1,"uri":`+ehrURI("ehr://x")+`}`),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := identityKeys(withRule(validation.ValidateRM(tc.root), schemeValid)); !slices.Equal(got, tc.want) {
				t.Errorf("ValidateRM(%s) issues under %s = %q, want %q (REQ-112 DV_EHR_URI.Scheme_valid)", tc.name, schemeValid, got, tc.want)
			}
		})
	}
}

// REQ-168 — each of the three findings names the class and the attribute in
// its Detail and no value, and its Value is empty: the scheme is part of the
// URI's value, so neither the Detail nor the Value carries it.
func TestValidateRM_REQ168_LinksLocationSchemeAreValueFree(t *testing.T) {
	t.Parallel()
	const marker = "zzmarkerscheme"
	tests := []struct {
		name  string
		root  any
		rule  string
		names []string // the words the Detail must hold
	}{
		{
			name: "links", rule: linksValid, names: []string{"COMPOSITION", "links"},
			root: decodeJSON[rm.Composition](t, lsComposition(`,"links":[]`, ``)),
		},
		{
			name: "location", rule: locationValid, names: []string{"EVENT_CONTEXT", "location"},
			root: decodeJSON[rm.EventContext](t, `{"_type":"EVENT_CONTEXT","location":""}`),
		},
		{
			name: "scheme", rule: schemeValid, names: []string{"DV_EHR_URI", "value"},
			root: decodeJSON[rm.DVEHRURI](t, ehrURI(marker+"://payload")),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			issues := withRule(validation.ValidateRM(tc.root), tc.rule)
			if len(issues) != 1 {
				t.Fatalf("ValidateRM(%s) issues under %s = %d, want 1", tc.name, tc.rule, len(issues))
			}
			iss := issues[0]
			if iss.Layer != validation.LayerRM || iss.Code != "rm_invariant" {
				t.Errorf("issue under %s Layer = %v, Code = %q, want the RM layer and rm_invariant", tc.rule, iss.Layer, iss.Code)
			}
			for _, word := range tc.names {
				if !strings.Contains(iss.Detail, word) {
					t.Errorf("issue under %s Detail = %q, want it to name %q", tc.rule, iss.Detail, word)
				}
			}
			if strings.Contains(iss.Detail, marker) || strings.Contains(iss.Detail, "payload") {
				t.Errorf("issue under %s Detail = %q, want no part of the value (REQ-168)", tc.rule, iss.Detail)
			}
			if got := iss.Value.Reveal(); got != nil {
				t.Errorf("issue under %s Value.Reveal() = %v, want an empty Value (REQ-168)", tc.rule, got)
			}
		})
	}
}

// REQ-112, REQ-171 § One walk — the template validator applies the three
// rules in its own walk, so a vital signs document that carries a present
// and empty `links` and `location`, or a link to a URI of another scheme,
// draws each finding from Validate with the template, at the same path as
// ValidateRM gives, and the document without them draws none.
func TestValidate_REQ112_LinksLocationSchemeUnderATemplate(t *testing.T) {
	t.Parallel()
	c := mustCompile(t, "vital_signs")
	raw, err := os.ReadFile(fixtures.CompositionJSON("vital_signs"))
	if err != nil {
		t.Fatalf("read vital_signs.json: %v", err)
	}
	const head = `"_type": "COMPOSITION",`
	if strings.Count(string(raw), head) != 1 {
		t.Fatalf("vital_signs.json holds %d COMPOSITION heads, want 1 (fixture drift)", strings.Count(string(raw), head))
	}
	inject := func(t *testing.T, members string) *rm.Composition {
		return decodeJSON[rm.Composition](t, strings.Replace(string(raw), head, head+members, 1))
	}
	const context = `"context":{"start_time":{"value":"2026-01-01T00:00:00Z"},"setting":{"value":"other care","defining_code":{"terminology_id":{"value":"openehr"},"code_string":"238"}},"location":""},`
	tests := []struct {
		name    string
		members string
		rule    string
		want    []string
	}{
		{name: "links present and empty", members: `"links":[],`, rule: linksValid, want: []string{"rm_invariant /links rm " + linksValid}},
		{name: "location present and empty", members: context, rule: locationValid, want: []string{"rm_invariant /context/location rm " + locationValid}},
		{
			name:    "a link to another scheme",
			members: `"links":[` + strings.Replace(lsLink, "ehr://example.org/x", "http://example.org/x", 1) + `],`,
			rule:    schemeValid,
			want:    []string{"rm_invariant /links[1]/target rm " + schemeValid},
		},
		{name: "a link to an ehr URI", members: `"links":[` + lsLink + `],`, rule: schemeValid},
		{name: "none of them", members: ``, rule: linksValid},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			comp := inject(t, tc.members)
			if got := identityKeys(withRule(validation.Validate(comp, c), tc.rule)); !slices.Equal(got, tc.want) {
				t.Errorf("Validate(vital signs with %s, c) issues under %s = %q, want %q", tc.name, tc.rule, got, tc.want)
			}
			if got := identityKeys(withRule(validation.ValidateRM(comp), tc.rule)); !slices.Equal(got, tc.want) {
				t.Errorf("ValidateRM(vital signs with %s) issues under %s = %q, want %q", tc.name, tc.rule, got, tc.want)
			}
		})
	}
}

// linksExistenceOPT returns a template rooted at an OBSERVATION that
// declares its links at the existence interval body existence, cardinality
// 0..*, and nothing else.
func linksExistenceOPT(existence string) string {
	return `<?xml version="1.0"?>
<template xmlns="http://schemas.openehr.org/v1" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
<template_id><value>observation-links</value></template_id><concept>observation-links</concept>
<language><terminology_id><value>ISO_639-1</value></terminology_id><code_string>en</code_string></language>
<definition><rm_type_name>OBSERVATION</rm_type_name><node_id>at0000</node_id>
<attributes xsi:type="C_MULTIPLE_ATTRIBUTE"><rm_attribute_name>links</rm_attribute_name>
<existence>` + existence + `</existence>
<cardinality><is_ordered>false</is_ordered><is_unique>false</is_unique><interval>` + fromLowerUnbounded("0") + `</interval></cardinality>
</attributes>
<archetype_id><value>openEHR-EHR-OBSERVATION.links.v1</value></archetype_id></definition>
</template>`
}

// REQ-102 An existence lower bound alone asks for no member, REQ-112
// LOCATABLE.Links_valid — the floor reports a `links` that is present and
// empty, but `links` is not one of the lists the floor holds non-empty for
// the template's sake, so a template's existence of 1..1 on it asks for no
// member: an absent list and an empty one draw no `required` from the
// template, and the empty one draws the RM's Links_valid alone. A list that
// holds a link draws nothing. The template's existence does not change what
// the RM rule reports.
func TestValidate_REQ102_REQ112_LinksExistenceAsksForNoMember(t *testing.T) {
	t.Parallel()
	const at = "/links"
	tests := []struct {
		name      string
		existence string
		links     string // the members after the OBSERVATION's name
		want      []string
	}{
		{name: "required, absent", existence: fiInterval("1", "1")},
		{name: "required, null", existence: fiInterval("1", "1"), links: `,"links":null`},
		{name: "required, present and empty", existence: fiInterval("1", "1"), links: `,"links":[]`, want: []string{"rm_invariant " + at + " rm " + linksValid}},
		{name: "required, one link", existence: fiInterval("1", "1"), links: `,"links":[` + lsLink + `]`},
		{name: "optional, absent", existence: fiInterval("0", "1")},
		{name: "optional, present and empty", existence: fiInterval("0", "1"), links: `,"links":[]`, want: []string{"rm_invariant " + at + " rm " + linksValid}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := mustCompileInline(t, linksExistenceOPT(tc.existence))
			obs := decodeJSON[rm.Observation](t, `{"_type":"OBSERVATION","archetype_node_id":"openEHR-EHR-OBSERVATION.links.v1",`+lsName+tc.links+`}`)
			var got []validation.Issue
			for _, iss := range validation.Validate(obs, c).Issues {
				if iss.Path == at {
					got = append(got, iss)
				}
			}
			if keys := identityKeys(got); !slices.Equal(keys, tc.want) {
				t.Errorf("Validate(OBSERVATION with links existence %s, %s) issues at %s = %q, want %q (REQ-102 An existence lower bound alone asks for no member)", tc.existence, tc.name, at, keys, tc.want)
			}
		})
	}
}
