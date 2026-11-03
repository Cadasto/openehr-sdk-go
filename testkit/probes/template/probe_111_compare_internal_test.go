package templateprobes

import (
	"strings"
	"testing"
)

// probe111Doc wraps body in an operational template root that declares the
// openEHR namespace as the default and binds xsi, plus any extra attributes.
func probe111Doc(rootAttrs, body string) []byte {
	return []byte(`<?xml version="1.0" encoding="UTF-8"?><template xmlns="http://schemas.openehr.org/v1" ` +
		`xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"` + rootAttrs + `>` + body + `</template>`)
}

// TestProbe111CompareDocuments pins PROBE-111's normalised comparison
// (REQ-176 § Round trip and § Shapes the pinned BMM lacks): each
// normalisation and rewrite sets aside the difference it names, and the
// difference next to it, which it must not hide, still fails. Drop or widen
// a rule in probe_111_operational_template_compare.go and a case reddens.
func TestProbe111CompareDocuments(t *testing.T) {
	const term = `<items code="at0000">%s</items>`
	cases := []struct {
		name      string
		input     []byte
		reEncoded []byte
		want      string // a part of the difference; "" when the documents agree
	}{
		{
			name:      "white space between elements and the XML declaration set aside",
			input:     probe111Doc("", "\n  <concept>at0000</concept>\n"),
			reEncoded: []byte(`<template xmlns="http://schemas.openehr.org/v1"><concept>at0000</concept></template>`),
		},
		{
			name:      "white space inside a string kept",
			input:     probe111Doc("", `<concept>at0000 </concept>`),
			reEncoded: probe111Doc("", `<concept>at0000</concept>`),
			want:      `/template[1]/concept[1]: text "at0000 ", re-encoded as "at0000"`,
		},
		{
			name:      "a Hash member's entries compare as a set",
			input:     probe111Doc("", strings.Replace(term, "%s", `<items id="text">T</items><items id="description">D</items>`, 1)),
			reEncoded: probe111Doc("", strings.Replace(term, "%s", `<items id="description">D</items><items id="text">T</items>`, 1)),
		},
		{
			name:      "a Hash entry's value still compares",
			input:     probe111Doc("", strings.Replace(term, "%s", `<items id="text">T</items><items id="description">D</items>`, 1)),
			reEncoded: probe111Doc("", strings.Replace(term, "%s", `<items id="description">D</items><items id="text">X</items>`, 1)),
			want:      `/template[1]/items[1]/items[2]: text "T", re-encoded as "X"`,
		},
		{
			name:      "a Hash entry lost still counts",
			input:     probe111Doc("", strings.Replace(term, "%s", `<items id="text">T</items><items id="description">D</items>`, 1)),
			reEncoded: probe111Doc("", strings.Replace(term, "%s", `<items id="text">T</items>`, 1)),
			want:      `/template[1]/items[1]/items[1]/@id: "description", re-encoded as "text"`,
		},
		{
			name: "RESOURCE_DESCRIPTION details compare as a set keyed by language",
			input: probe111Doc("", `<description><details><language><code_string>nl</code_string></language></details>`+
				`<details><language><code_string>en</code_string></language></details></description>`),
			reEncoded: probe111Doc("", `<description><details><language><code_string>en</code_string></language></details>`+
				`<details><language><code_string>nl</code_string></language></details></description>`),
		},
		{
			name: "a T_VIEW constraint's items are a List, so their order counts",
			input: probe111Doc("", `<view><constraints path="/a"><items id="b"><value>1</value></items>`+
				`<items id="a"><value>2</value></items></constraints></view>`),
			reEncoded: probe111Doc("", `<view><constraints path="/a"><items id="a"><value>2</value></items>`+
				`<items id="b"><value>1</value></items></constraints></view>`),
			want: `/template[1]/view[1]/constraints[1]/items[1]/@id: "b", re-encoded as "a"`,
		},
		{
			name:      "a number compares by its value",
			input:     probe111Doc("", `<magnitude>1.0</magnitude><precision>-1</precision>`),
			reEncoded: probe111Doc("", `<magnitude>1</magnitude><precision>-1.00</precision>`),
		},
		{
			name:      "a number of another value differs",
			input:     probe111Doc("", `<magnitude>1.5</magnitude>`),
			reEncoded: probe111Doc("", `<magnitude>1</magnitude>`),
			want:      `/template[1]/magnitude[1]: text "1.5", re-encoded as "1"`,
		},
		{
			name:      "a boolean's white space collapses",
			input:     probe111Doc("", "<existence><lower_included>true\n   </lower_included><upper_included>true</upper_included><lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded><lower>0</lower><upper>1</upper></existence>"),
			reEncoded: probe111Doc("", `<existence><lower_included>true</lower_included><upper_included>true</upper_included><lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded><lower>0</lower><upper>1</upper></existence>`),
		},
		{
			name:      "a boolean compares by its value, so 1 is true and 0 is false",
			input:     probe111Doc("", "<existence><lower_included>1</lower_included><upper_included> 1\n</upper_included><lower_unbounded>0</lower_unbounded><upper_unbounded>false</upper_unbounded><lower>0</lower><upper>1</upper></existence>"),
			reEncoded: probe111Doc("", `<existence><lower_included>true</lower_included><upper_included>true</upper_included><lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded><lower>0</lower><upper>1</upper></existence>`),
		},
		{
			name:      "a boolean of another value differs",
			input:     probe111Doc("", `<existence><lower_included>1</lower_included><upper_included>true</upper_included><lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded><lower>0</lower><upper>1</upper></existence>`),
			reEncoded: probe111Doc("", `<existence><lower_included>false</lower_included><upper_included>true</upper_included><lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded><lower>0</lower><upper>1</upper></existence>`),
			want:      `/template[1]/existence[1]/lower_included[1]: text "1", re-encoded as "false"`,
		},
		{
			name:      "a number that is no boolean literal differs from true",
			input:     probe111Doc("", `<magnitude>1.0</magnitude>`),
			reEncoded: probe111Doc("", `<magnitude>true</magnitude>`),
			want:      `/template[1]/magnitude[1]: text "1.0", re-encoded as "true"`,
		},
		{
			name:      "an absent included flag compares as its default: closed on a bounded side, open on an unbounded one",
			input:     probe111Doc("", `<occurrences><lower_unbounded>false</lower_unbounded><upper_unbounded>true</upper_unbounded><lower>1</lower></occurrences>`),
			reEncoded: probe111Doc("", `<occurrences><lower_included>true</lower_included><upper_included>false</upper_included><lower_unbounded>false</lower_unbounded><upper_unbounded>true</upper_unbounded><lower>1</lower></occurrences>`),
		},
		{
			name:      "an included flag written against its default differs",
			input:     probe111Doc("", `<occurrences><lower_unbounded>false</lower_unbounded><upper_unbounded>true</upper_unbounded><lower>1</lower></occurrences>`),
			reEncoded: probe111Doc("", `<occurrences><lower_included>false</lower_included><upper_included>false</upper_included><lower_unbounded>false</lower_unbounded><upper_unbounded>true</upper_unbounded><lower>1</lower></occurrences>`),
			want:      `/template[1]/occurrences[1]/lower_included[1]: text "true", re-encoded as "false"`,
		},
		{
			name:      "an absent unbounded flag compares as its default, and an empty flag as absent",
			input:     probe111Doc("", `<occurrences><lower_included></lower_included><upper_included>false</upper_included><lower>1</lower></occurrences>`),
			reEncoded: probe111Doc("", `<occurrences><lower_included>true</lower_included><upper_included>false</upper_included><lower_unbounded>false</lower_unbounded><upper_unbounded>true</upper_unbounded><lower>1</lower></occurrences>`),
		},
		{
			name:      "an xsi:type compares as the name it denotes",
			input:     probe111Doc(` xmlns:xs="http://www.w3.org/2001/XMLSchema"`, `<item xsi:type="xs:string">a</item><children xsi:type="C_STRING"/>`),
			reEncoded: probe111Doc("", `<item xmlns:xsd="http://www.w3.org/2001/XMLSchema" xsi:type="xsd:string">a</item><children xsi:type="C_STRING"/>`),
		},
		{
			name:      "an xsi:type naming another type differs",
			input:     probe111Doc(` xmlns:xs="http://www.w3.org/2001/XMLSchema"`, `<item xsi:type="xs:string">a</item>`),
			reEncoded: probe111Doc("", `<item xmlns:xsd="http://www.w3.org/2001/XMLSchema" xsi:type="xsd:integer">a</item>`),
			want:      `/template[1]/item[1]/@type: "{http://www.w3.org/2001/XMLSchema}string", re-encoded as "{http://www.w3.org/2001/XMLSchema}integer"`,
		},
		{
			name:      "the root's xsi:type is set aside",
			input:     probe111Doc(` xsi:type="OPERATIONAL_TEMPLATE"`, `<concept>at0000</concept>`),
			reEncoded: probe111Doc("", `<concept>at0000</concept>`),
		},
		{
			name:      "an xsi:type the encoder does not write is set aside",
			input:     probe111Doc("", `<definition xsi:type="C_ARCHETYPE_ROOT"/>`),
			reEncoded: probe111Doc("", `<definition/>`),
		},
		{
			name:      "an xsi:type below the root re-encoded as another class differs",
			input:     probe111Doc("", `<definition xsi:type="C_ARCHETYPE_ROOT"/>`),
			reEncoded: probe111Doc("", `<definition xsi:type="C_COMPLEX_OBJECT"/>`),
			want: `/template[1]/definition[1]/@type: "{http://schemas.openehr.org/v1}C_ARCHETYPE_ROOT", ` +
				`re-encoded as "{http://schemas.openehr.org/v1}C_COMPLEX_OBJECT"`,
		},
		{
			name:      "an xsi:type the re-encoded document adds below the root differs",
			input:     probe111Doc("", `<definition/>`),
			reEncoded: probe111Doc("", `<definition xsi:type="C_ARCHETYPE_ROOT"/>`),
			want:      `/template[1]/definition[1]/@type: the re-encoded document adds "{http://schemas.openehr.org/v1}C_ARCHETYPE_ROOT"`,
		},
		{
			name:      "an attribute other than xsi:type absent from the re-encoded document differs",
			input:     probe111Doc("", `<term_definitions code="at0000" language="en"/>`),
			reEncoded: probe111Doc("", `<term_definitions code="at0000"/>`),
			want:      `/template[1]/term_definitions[1]/@language: absent from the re-encoded document`,
		},
		{
			name: "the encoder's rewrites set aside on the input",
			input: probe111Doc("", `<definition xsi:type="T_ARCHETYPE_ROOT"><attributes><match_negated>false</match_negated></attributes>`+
				`<timezone_validity>optional</timezone_validity><operator>matches</operator>`+
				`<symbol><value/><defining_code><code_string>at0001</code_string></defining_code></symbol></definition>`),
			reEncoded: probe111Doc("", `<definition xsi:type="C_ARCHETYPE_ROOT"><attributes/>`+
				`<timezone_validity>1002</timezone_validity><operator>2007</operator>`+
				`<symbol><defining_code><code_string>at0001</code_string></defining_code></symbol></definition>`),
		},
		{
			name:      "a rewrite applies to the input only",
			input:     probe111Doc("", `<attributes/>`),
			reEncoded: probe111Doc("", `<attributes><match_negated>false</match_negated></attributes>`),
			want:      `/template[1]/attributes[1]: the re-encoded document adds match_negated after the input's last child`,
		},
		{
			name:      "a symbol's non-empty value is kept",
			input:     probe111Doc("", `<symbol><value>x</value><defining_code/></symbol>`),
			reEncoded: probe111Doc("", `<symbol><defining_code/></symbol>`),
			want:      `/template[1]/symbol[1]/value[1]: the re-encoded document has defining_code there`,
		},
		{
			name:      "an attribute the re-encoded document adds differs",
			input:     probe111Doc("", `<term_definitions code="at0000"/>`),
			reEncoded: probe111Doc("", `<term_definitions code="at0000" language="en"/>`),
			want:      `/template[1]/term_definitions[1]/@language: the re-encoded document adds "en"`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := probe111CompareDocuments(tc.input, tc.reEncoded)
			if err != nil {
				t.Fatalf("probe111CompareDocuments: %v", err)
			}
			if got != tc.want {
				t.Errorf("probe111CompareDocuments(\n%s,\n%s)\n= %q\nwant %q", tc.input, tc.reEncoded, got, tc.want)
			}
		})
	}
}

// TestProbe111SchemaRatchetParse pins the ratchet's line format (PROBE-111,
// REQ-176): a name and a reason separated by a tab, comments and blank lines
// ignored; a line without a reason, or a name listed twice, is refused rather
// than read as an entry that holds nothing back.
func TestProbe111SchemaRatchetParse(t *testing.T) {
	got, err := probe111ParseRatchet("# header\n\ntemplates/a b.opt\tthe schema refuses x\r\n")
	if err != nil {
		t.Fatalf("probe111ParseRatchet: %v", err)
	}
	if len(got) != 1 || got[0].Name != "templates/a b.opt" || got[0].Reason != "the schema refuses x" {
		t.Errorf("probe111ParseRatchet = %+v, want one entry for templates/a b.opt with its reason", got)
	}
	for _, bad := range []string{
		"templates/a.opt\n",
		"templates/a.opt\t \n",
		"templates/a.opt\tone\ntemplates/a.opt\ttwo\n",
	} {
		if entries, err := probe111ParseRatchet(bad); err == nil {
			t.Errorf("probe111ParseRatchet(%q) = %+v, nil; want an error", bad, entries)
		}
	}
	if entries, err := Probe111SchemaRatchet(); err != nil {
		t.Errorf("Probe111SchemaRatchet: %v", err)
	} else {
		for _, e := range entries {
			if e.Reason == "" {
				t.Errorf("schema ratchet entry %s has no reason", e.Name)
			}
		}
	}
}
