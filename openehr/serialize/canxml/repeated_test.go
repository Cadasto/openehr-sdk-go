package canxml_test

// repeated_test.go: REQ-170 (wire.md § REQ-170, Unmatched content on
// decode) for a child element that repeats an earlier child of the same name
// under a single-valued attribute. The repeat is unmatched content: every
// decode keeps the first child's value, so the default reads the member as
// if the repeat were absent; WithUnmatched reports the repeat's element as
// one item, by its path, and nothing inside it; WithStrict refuses it with
// canxml.ErrUnknownElement. The children of a multi-valued attribute are its
// items and never a repeat.

import (
	"bytes"
	"reflect"
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aom/aom14"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml"
)

// repeatCase is one document carrying a repeated child, the same document
// without the repeat, and the path WithUnmatched reports for the repeat.
type repeatCase struct {
	name    string
	into    func() any
	in      string // the document with the repeat
	without string // the document with the repeat left out
	path    string // the repeat's path; "" when the document holds no repeat
}

// repeatedLanguage is a second root `language` whose code differs from the
// first, carrying an extension element and the planted value: neither may be
// reported, since the repeat is one item whatever it holds.
const repeatedLanguage = `<language><terminology_id><value>ISO_639-1</value></terminology_id><code_string>de</code_string><x-ext>` + xmlPlanted + `</x-ext></language>`

// repeatComposition returns a COMPOSITION holding extra after its first
// `language`.
func repeatComposition(extra string) string {
	return `<composition ` + ucNS + ` archetype_node_id="openEHR-EHR-COMPOSITION.encounter.v1">` +
		`<name><value>encounter</value></name>` + xLanguage + extra +
		`<territory><terminology_id><value>ISO_3166-1</value></terminology_id><code_string>GB</code_string></territory>` +
		`<category><value>event</value><defining_code><terminology_id><value>openehr</value></terminology_id><code_string>433</code_string></defining_code></category>` +
		`<composer xsi:type="PARTY_SELF"/>` +
		`</composition>`
}

// repeatContent is one OBSERVATION as a COMPOSITION's content item.
const repeatContent = `<content xsi:type="OBSERVATION" archetype_node_id="openEHR-EHR-OBSERVATION.vitals.v1"><name><value>vitals</value></name>` + xLanguage +
	`<encoding><terminology_id><value>IANA_character-sets</value></terminology_id><code_string>UTF-8</code_string></encoding>` +
	`<subject xsi:type="PARTY_SELF"/><data xsi:type="HISTORY" archetype_node_id="at0001"><name><value>history</value></name><origin>` + xTime + `</origin></data></content>`

// repeatCardinality returns an AOM 1.4 Cardinality holding flags after its
// is_ordered, which is true.
func repeatCardinality(flags string) string {
	return `<cardinality ` + ucNS + `><is_ordered>true</is_ordered>` + flags +
		`<is_unique>false</is_unique>` +
		`<interval><lower_included>true</lower_included><upper_included>false</upper_included><lower_unbounded>false</lower_unbounded><upper_unbounded>true</upper_unbounded><lower>0</lower></interval>` +
		`</cardinality>`
}

func repeatCases() []repeatCase {
	quantity := func(extra string) string {
		return `<value ` + ucNS + ` xsi:type="DV_QUANTITY"><magnitude>80.5</magnitude><units>kg</units>` + extra + `</value>`
	}
	interval := func(extra string) string {
		return `<value ` + ucNS + ` xsi:type="DV_INTERVAL"><lower xsi:type="DV_COUNT"><magnitude>1</magnitude></lower><lower_included>true</lower_included>` + extra +
			`<upper xsi:type="DV_COUNT"><magnitude>2</magnitude></upper></value>`
	}
	code := func(extra string) string {
		return `<terminology_code ` + ucNS + `><terminology_id>snomed_ct</terminology_id><code_string>123</code_string>` + extra + `</terminology_code>`
	}
	return []repeatCase{
		{
			// A struct member held by value, at the root of a COMPOSITION.
			name:    "root language",
			into:    func() any { return new(rm.Composition) },
			in:      repeatComposition(repeatedLanguage),
			without: repeatComposition(""),
			path:    "/composition[1]/language[2]",
		},
		{
			// A primitive member, the second holding another value.
			name:    "DV_QUANTITY units",
			into:    func() any { return new(rm.DVQuantity) },
			in:      quantity(`<units>` + xmlPlanted + `</units>`),
			without: quantity(""),
			path:    "/value[1]/units[2]",
		},
		{
			// A boundary flag, which the decoder also records as present
			// (REQ-169): the first, true, is kept.
			name:    "DV_INTERVAL lower_included",
			into:    func() any { return new(rm.DVInterval[rm.DVOrdered]) },
			in:      interval(`<lower_included>false</lower_included>`),
			without: interval(""),
			path:    "/value[1]/lower_included[2]",
		},
		{
			// The hand-written TERMINOLOGY_CODE decoder.
			name:    "TERMINOLOGY_CODE code_string",
			into:    func() any { return new(rm.TerminologyCode) },
			in:      code(`<code_string>` + xmlPlanted + `</code_string>`),
			without: code(""),
			path:    "/terminology_code[1]/code_string[2]",
		},
		{
			// An AOM 1.4 class: the generator writes the same guard there.
			name:    "AOM 1.4 Cardinality is_ordered",
			into:    func() any { return new(aom14.Cardinality) },
			in:      repeatCardinality(`<is_ordered>false</is_ordered>`),
			without: repeatCardinality(""),
			path:    "/cardinality[1]/is_ordered[2]",
		},
		{
			// The children of a multi-valued attribute are its items: two
			// `content` children are two items, and nothing is reported.
			name: "control: two content items",
			into: func() any { return new(rm.Composition) },
			in:   repeatComposition(repeatContent + repeatContent),
		},
		{
			// archetype_node_id written as an XML attribute and once as a
			// child element is no repeat: the child is the first child of
			// that name.
			name: "control: an attribute member also written as a child element",
			into: func() any { return new(rm.Composition) },
			in:   repeatComposition(`<archetype_node_id>openEHR-EHR-COMPOSITION.encounter.v1</archetype_node_id>`),
		},
	}
}

// TestREQ170_XMLRepeatedChildIsUnmatched drives each case through the three
// modes on canxml.Unmarshal and on a canxml.Decoder:
//
//   - Default: the document decodes, with no error, to the value of the same
//     document without the repeat, so the first child's value is kept.
//   - Report: WithUnmatched receives the repeat's path once, and nothing
//     inside the repeat, and the decode returns the default's value.
//   - Strict: the decode is refused at the repeat (assertXMLStrictRefusal);
//     the document without it passes.
//
// A control case holds no repeat: every mode decodes it and nothing is
// reported.
func TestREQ170_XMLRepeatedChildIsUnmatched(t *testing.T) {
	for _, c := range repeatCases() {
		t.Run(c.name, func(t *testing.T) {
			def := c.into()
			if err := canxml.Unmarshal([]byte(c.in), def); err != nil {
				t.Fatalf("canxml.Unmarshal(%s) = %v; want nil: a repeat is skipped by default", c.in, err)
			}
			if c.without != "" {
				want := c.into()
				if err := canxml.Unmarshal([]byte(c.without), want); err != nil {
					t.Fatalf("canxml.Unmarshal(%s) = %v; want nil", c.without, err)
				}
				if !reflect.DeepEqual(def, want) {
					t.Errorf("canxml.Unmarshal decoded the repeat into\n%#v\nwant the first child's value, as without the repeat\n%#v", def, want)
				}
			}
			var wantPaths []string
			if c.path != "" {
				wantPaths = []string{c.path}
			}
			got, paths, err := unmarshalCollecting(c.into, []byte(c.in))
			if err != nil {
				t.Fatalf("canxml.Unmarshal(WithUnmatched) = %v; want nil", err)
			}
			if !slices.Equal(paths, wantPaths) {
				t.Errorf("WithUnmatched reported %q; want %q: the repeat once, nothing inside it", paths, wantPaths)
			}
			assertNoPlantedValue(t, paths)
			if !reflect.DeepEqual(got, def) {
				t.Errorf("canxml.Unmarshal(WithUnmatched) decoded\n%#v\nwant the default's value\n%#v", got, def)
			}

			err = canxml.Unmarshal([]byte(c.in), c.into(), canxml.WithStrict())
			if c.path == "" {
				if err != nil {
					t.Errorf("canxml.Unmarshal(%s, WithStrict) = %v; want nil: the document holds no repeat", c.in, err)
				}
			} else {
				assertXMLStrictRefusal(t, "canxml.Unmarshal(WithStrict)", err, c.path, false)
				if err := canxml.Unmarshal([]byte(c.without), c.into(), canxml.WithStrict()); err != nil {
					t.Errorf("canxml.Unmarshal(without the repeat, WithStrict) = %v; want nil", err)
				}
			}

			// A Decoder reads the value as Unmarshal does.
			var streamed []string
			dv := c.into()
			dec := canxml.NewDecoder(bytes.NewReader([]byte(c.in)), canxml.WithUnmatched(func(p canxml.Path) { streamed = append(streamed, p.String()) }))
			if err := dec.Decode(dv); err != nil {
				t.Fatalf("Decoder.Decode(WithUnmatched) = %v; want nil", err)
			}
			if !slices.Equal(streamed, wantPaths) {
				t.Errorf("Decoder.Decode(WithUnmatched) reported %q; want %q", streamed, wantPaths)
			}
			if !reflect.DeepEqual(dv, def) {
				t.Errorf("Decoder.Decode decoded\n%#v\nwant Unmarshal's value\n%#v", dv, def)
			}
		})
	}
}

// TestREQ170_XMLRepeatedChildKeepsTheFirstValue pins the value each decode
// keeps by reading it, not only by comparison with the document without the
// repeat: the first root language, "en", where the repeat says "de".
func TestREQ170_XMLRepeatedChildKeepsTheFirstValue(t *testing.T) {
	in := []byte(repeatComposition(repeatedLanguage))
	for _, mode := range []struct {
		name string
		opts []canxml.DecoderOption
	}{
		{name: "default"},
		{name: "WithUnmatched", opts: []canxml.DecoderOption{canxml.WithUnmatched(func(canxml.Path) {})}},
		{name: "WithMissing", opts: []canxml.DecoderOption{canxml.WithMissing(func(canxml.Pointer) {})}},
	} {
		t.Run(mode.name, func(t *testing.T) {
			var c rm.Composition
			if err := canxml.Unmarshal(in, &c, mode.opts...); err != nil {
				t.Fatalf("canxml.Unmarshal = %v; want nil", err)
			}
			if c.Language.CodeString != "en" {
				t.Errorf("Language.CodeString = %q; want %q, the first language's", c.Language.CodeString, "en")
			}
		})
	}
}
