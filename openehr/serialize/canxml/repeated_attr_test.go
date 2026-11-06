package canxml_test

// repeated_attr_test.go: REQ-056 (wire.md § REQ-056) for an element that
// carries one attribute twice. That is not well-formed XML, so every decode
// refuses it, whatever its options, wherever the decode reads the element or
// skips it as unmatched content: an error that wraps canxml.ErrInvalidShape,
// names the attribute, and carries none of the values. A decode with any
// option, WithMaxDepth included, also refuses one inside a primitive
// element's text, which a decode with no option may read past.

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aom/aom14"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml"
)

// The two values a repeated attribute carries; neither may appear in an
// error.
const (
	dupA = "first-" + xmlPlanted
	dupB = "second-" + xmlPlanted
)

// repeatedAttrCase is one document whose element carries one attribute
// twice, the attribute the refusal names, and whether a decode with no option
// must refuse it.
type repeatedAttrCase struct {
	name    string
	into    func() any
	in      string
	attr    string // the path segment the refusal names, such as "@archetype_node_id"
	tracked bool   // only a decode given an option must refuse it
	once    string // the document with the attribute once; "" for in without its second value
	// unmatchedFirst marks an element inside unmatched content, which strict
	// refuses as it opens, before it reads what the element holds.
	unmatchedFirst bool
}

func repeatedAttrCases() []repeatedAttrCase {
	twice := func(name string) string { return name + `="` + dupA + `" ` + name + `="` + dupB + `"` }
	composition := func(rootAttrs, inner string) string {
		return `<composition ` + ucNS + ` ` + rootAttrs + `>` +
			`<name><value>encounter</value></name>` + inner +
			`<territory><terminology_id><value>ISO_3166-1</value></terminology_id><code_string>GB</code_string></territory>` +
			`<category><value>event</value><defining_code><terminology_id><value>openehr</value></terminology_id><code_string>433</code_string></defining_code></category>` +
			`<composer xsi:type="PARTY_SELF"/></composition>`
	}
	comp := func() any { return new(rm.Composition) }
	node := `archetype_node_id="openEHR-EHR-COMPOSITION.encounter.v1"`
	return []repeatedAttrCase{
		{name: "the root's member attribute", into: comp, in: composition(twice("archetype_node_id"), xLanguage), attr: "@archetype_node_id"},
		{name: "an unknown attribute", into: comp, in: composition(node+" "+twice("x-flag"), xLanguage), attr: "@x-flag"},
		{name: "a namespace prefix declared twice", into: comp, in: composition(node+" "+twice("xmlns:p"), xLanguage), attr: "@xmlns:p"},
		{name: "two prefixes bound to one namespace", into: comp, in: composition(node+` xmlns:p="urn:a" xmlns:q="urn:a" p:x="`+dupA+`" q:x="`+dupB+`"`, xLanguage), attr: "@x"},
		{
			name: "xsi:type", into: func() any { return new(rm.DVText) }, attr: "@type",
			in:   `<value ` + ucNS + ` xsi:type="DV_TEXT" xsi:type="DV_TEXT"><value>t</value></value>`,
			once: `<value ` + ucNS + ` xsi:type="DV_TEXT"><value>t</value></value>`,
		},
		{name: "a nested class element", into: comp, in: composition(node, `<language `+twice("x")+`><terminology_id><value>ISO_639-1</value></terminology_id><code_string>en</code_string></language>`), attr: "@x"},
		{name: "a primitive element", into: func() any { return new(rm.DVText) }, in: `<value ` + ucNS + `><value ` + twice("x") + `>t</value></value>`, attr: "@x"},
		{name: "an unknown element", into: comp, in: composition(node, xLanguage+`<x-ext `+twice("y")+`/>`), attr: "@y"},
		{name: "inside an unknown element", into: comp, in: composition(node, xLanguage+`<x-ext><z `+twice("y")+`/></x-ext>`), attr: "@y", unmatchedFirst: true},
		{name: "a repeat of a single-valued member", into: comp, in: composition(node, xLanguage+`<language `+twice("x")+`/>`), attr: "@x"},
		{name: "a Hash entry's key", into: func() any { return new(rm.ResourceDescription) }, in: `<description ` + ucNS + `><original_author ` + twice("id") + `>a</original_author><lifecycle_state>published</lifecycle_state></description>`, attr: "@id"},
		{name: "another attribute of a Hash entry", into: func() any { return new(rm.ResourceDescription) }, in: `<description ` + ucNS + `><original_author id="k" ` + twice("x") + `>a</original_author><lifecycle_state>published</lifecycle_state></description>`, attr: "@x"},
		{name: "a boundary flag", into: func() any { return new(aom14.Cardinality) }, in: `<cardinality ` + ucNS + `><is_ordered>true</is_ordered><is_unique>false</is_unique><interval><lower_included ` + twice("x") + `>true</lower_included><upper_included>false</upper_included><lower_unbounded>false</lower_unbounded><upper_unbounded>true</upper_unbounded><lower>0</lower></interval></cardinality>`, attr: "@x"},
		{name: "a TERMINOLOGY_CODE terminology_id", into: func() any { return new(rm.TerminologyCode) }, in: `<terminology_code ` + ucNS + `><terminology_id ` + twice("x") + `>snomed_ct</terminology_id><code_string>1</code_string></terminology_code>`, attr: "@x"},
		{
			name: "a generic slot holding a primitive pointer", attr: "@x",
			into: func() any { s := "initial"; return &rm.Interval[any]{Lower: &s} },
			in:   `<interval ` + ucNS + `><lower ` + twice("x") + `>hello</lower></interval>`,
		},
		{
			name: "inside a generic slot holding a value", attr: "@y",
			into: func() any { return &rm.Interval[any]{Lower: "initial"} },
			in:   `<interval ` + ucNS + `><lower><z ` + twice("y") + `/></lower></interval>`,
		},
		{name: "inside a primitive element", into: func() any { return new(rm.DVText) }, in: `<value ` + ucNS + `><value>t<z ` + twice("y") + `/></value></value>`, attr: "@y", tracked: true},
	}
}

// TestREQ056_XMLRepeatedAttributeIsRefused drives each case through
// canxml.Unmarshal and a canxml.Decoder, with no option and with each report
// and strict option and with WithMaxDepth: each refuses the document with an
// error that wraps canxml.ErrInvalidShape and not canxml.ErrUnknownElement,
// names the attribute and carries neither value. A case only a decode given
// an option must refuse may decode with no option, and a refusal there is
// the same refusal. Strict refuses unmatched content around the element
// first, as the first problem in document order. The same document with one
// attribute decodes.
func TestREQ056_XMLRepeatedAttributeIsRefused(t *testing.T) {
	modes := []struct {
		name    string
		opts    []canxml.DecoderOption
		tracked bool
	}{
		{name: "default"},
		{name: "WithUnmatched", opts: []canxml.DecoderOption{canxml.WithUnmatched(func(canxml.Path) {})}, tracked: true},
		{name: "WithStrict", opts: []canxml.DecoderOption{canxml.WithStrict()}, tracked: true},
		{name: "WithMissing", opts: []canxml.DecoderOption{canxml.WithMissing(func(canxml.Pointer) {})}, tracked: true},
		{name: "WithPresence", opts: []canxml.DecoderOption{canxml.WithPresence(func(canxml.Pointer, canxml.Presence) {})}, tracked: true},
		{name: "WithMaxDepth", opts: []canxml.DecoderOption{canxml.WithMaxDepth(64)}, tracked: true},
	}
	for _, c := range repeatedAttrCases() {
		t.Run(c.name, func(t *testing.T) {
			once := c.once
			if once == "" {
				once = removeSecond(t, c.in)
			}
			if err := canxml.Unmarshal([]byte(once), c.into()); err != nil {
				t.Fatalf("canxml.Unmarshal(%s) = %v; want nil: the document carries each attribute once", once, err)
			}
			for _, m := range modes {
				refused := m.tracked || !c.tracked
				check := func(call string, err error) {
					t.Helper()
					if c.unmatchedFirst && m.name == "WithStrict" {
						if !errors.Is(err, canxml.ErrUnknownElement) {
							t.Errorf("%s %s = %v; want the refusal of the unmatched element around it, canxml.ErrUnknownElement", call, m.name, err)
						}
						return
					}
					if !refused && err == nil {
						return // a decode with no option may read past the element
					}
					if !errors.Is(err, canxml.ErrInvalidShape) || errors.Is(err, canxml.ErrUnknownElement) {
						t.Fatalf("%s %s = %v; want an error that wraps canxml.ErrInvalidShape and not canxml.ErrUnknownElement", call, m.name, err)
					}
					if !strings.Contains(err.Error(), c.attr) {
						t.Errorf("%s %s = %q; want it to name the attribute %s", call, m.name, err, c.attr)
					}
					if c.once == "" && strings.Contains(err.Error(), xmlPlanted) {
						t.Errorf("%s %s = %q; it carries an attribute's value", call, m.name, err)
					}
				}
				check("canxml.Unmarshal", canxml.Unmarshal([]byte(c.in), c.into(), m.opts...))
				check("Decoder.Decode", canxml.NewDecoder(bytes.NewReader([]byte(c.in)), m.opts...).Decode(c.into()))
			}
		})
	}
}

// removeSecond returns doc with the second of its repeated attributes left
// out: the attribute that follows the first value dupA.
func removeSecond(t *testing.T, doc string) string {
	t.Helper()
	i := strings.Index(doc, dupB)
	if i < 0 {
		t.Fatalf("the case carries no second value %q", dupB)
	}
	start := strings.LastIndex(doc[:i], " ")
	end := i + len(dupB) + 1 // the closing quote
	return doc[:start] + doc[end:]
}
