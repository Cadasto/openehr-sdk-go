package templatecompile_test

// coded_name_test.go: REQ-111 § Node names and name predicates on a name the
// template pins as a DV_CODED_TEXT of one code, whose term text is the
// template-level node name (issue 455).

import (
	"fmt"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// codedNameOPT is a COMPOSITION archetype root whose name attribute is %[1]s,
// holding one SECTION archetype root whose name attribute is %[2]s. Each root
// defines its own terms: the COMPOSITION a padded text for at0001, an empty
// one for at0002, and terms under an external code and under a bare one; the
// SECTION a text for at0010 alone.
const codedNameOPT = `<?xml version="1.0" encoding="UTF-8"?>
<template xmlns="http://schemas.openehr.org/v1" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <template_id><value>coded_name</value></template_id>
  <concept>coded_name</concept>
  <definition>
    <rm_type_name>COMPOSITION</rm_type_name>
    <node_id>at0000</node_id>
    %[1]s
    <attributes xsi:type="C_MULTIPLE_ATTRIBUTE">
      <rm_attribute_name>content</rm_attribute_name>
      <children xsi:type="C_ARCHETYPE_ROOT">
        <rm_type_name>SECTION</rm_type_name>
        <node_id>at0000</node_id>
        %[2]s
        <archetype_id><value>openEHR-EHR-SECTION.adhoc.v1</value></archetype_id>
        <term_definitions code="at0000"><items id="text">Ad hoc heading</items></term_definitions>
        <term_definitions code="at0010"><items id="text">Own heading</items></term_definitions>
      </children>
      <cardinality><is_ordered>false</is_ordered><is_unique>false</is_unique><interval><lower_included>true</lower_included><lower_unbounded>false</lower_unbounded><upper_unbounded>true</upper_unbounded><lower>0</lower></interval></cardinality>
    </attributes>
    <archetype_id><value>openEHR-EHR-COMPOSITION.report.v1</value></archetype_id>
    <term_definitions code="at0000"><items id="text">Report</items></term_definitions>
    <term_definitions code="at0001"><items id="text">  Heart Rate
    </items></term_definitions>
    <term_definitions code="at0002"><items id="text"></items></term_definitions>
    <term_definitions code="SNOMED-CT::248152002"><items id="text">Female</items></term_definitions>
    <term_definitions code="12345"><items id="text">Bare key</items></term_definitions>
  </definition>
</template>`

// codePhraseChild is a C_CODE_PHRASE child, or another kind of code phrase
// constraint xsiType names, of the terminology terminologyID (none where it
// is "") that lists codes.
func codePhraseChild(xsiType, terminologyID string, codes ...string) string {
	var b strings.Builder
	fmt.Fprintf(&b, `<children xsi:type="%s">
            <rm_type_name>CODE_PHRASE</rm_type_name>
            <node_id/>`, xsiType)
	if terminologyID != "" {
		b.WriteString(`<terminology_id><value>` + terminologyID + `</value></terminology_id>`)
	}
	for _, c := range codes {
		b.WriteString(`<code_list>` + c + `</code_list>`)
	}
	b.WriteString(`</children>`)
	return b.String()
}

// codedNameChild is a DV_CODED_TEXT child of a name attribute whose
// defining_code holds the code phrase children phrases, and whose value, where
// item is not "", is the C_PRIMITIVE_OBJECT holding the C_STRING item body.
func codedNameChild(item string, phrases ...string) string {
	value := ""
	if item != "" {
		value = `<attributes xsi:type="C_SINGLE_ATTRIBUTE">
          <rm_attribute_name>value</rm_attribute_name>
          <children xsi:type="C_PRIMITIVE_OBJECT">
            <rm_type_name>STRING</rm_type_name>
            <node_id/>
            <item xsi:type="C_STRING">` + item + `</item>
          </children>
        </attributes>`
	}
	return `<children xsi:type="C_COMPLEX_OBJECT">
        <rm_type_name>DV_CODED_TEXT</rm_type_name>
        <node_id/>
        <attributes xsi:type="C_SINGLE_ATTRIBUTE">
          <rm_attribute_name>defining_code</rm_attribute_name>
          ` + strings.Join(phrases, "\n          ") + `
        </attributes>
        ` + value + `
      </children>`
}

// localCode is a DV_CODED_TEXT name child whose defining_code lists the one
// local code.
func localCode(code string) string {
	return codedNameChild("", codePhraseChild("C_CODE_PHRASE", "local", code))
}

// TestREQ111_CompileReadsACodedTemplateLevelNodeName pins the coded reading
// of § REQ-111 Node names and name predicates (issue 455): where a node's
// name attribute holds one child, a DV_CODED_TEXT whose defining_code holds
// one code phrase, of a named terminology, that lists exactly one code, the
// term text of that code, trimmed, is the node's name. A local code takes
// the text its own archetype defines for it, an openehr code its rubric, an
// external one the text defined under "<terminology>::<code>", else under
// the code alone where that is no at-code. A C_STRING the name's value pins
// comes first, and one that rejects the code's text pins none. Several codes,
// a code with no term text, alternatives and no terminology pin none.
func TestREQ111_CompileReadsACodedTemplateLevelNodeName(t *testing.T) {
	cases := []struct {
		name, attrs, want string
	}{
		{name: "a local code", attrs: nameAttribute(localCode("at0001")), want: "Heart Rate"},
		{name: "an openehr code", attrs: nameAttribute(codedNameChild("", codePhraseChild("C_CODE_PHRASE", "openehr", "433"))), want: "event"},
		{name: "an external code, its term under the terminology and the code", attrs: nameAttribute(codedNameChild("", codePhraseChild("C_CODE_PHRASE", "SNOMED-CT", "248152002"))), want: "Female"},
		{name: "an external code, its term under the code alone", attrs: nameAttribute(codedNameChild("", codePhraseChild("C_CODE_PHRASE", "LOINC", "12345"))), want: "Bare key"},
		{name: "a code reference that lists one code", attrs: nameAttribute(codedNameChild("", codePhraseChild("C_CODE_REFERENCE", "local", "at0001"))), want: "Heart Rate"},
		{name: "one code beside a blank entry", attrs: nameAttribute(codedNameChild("", codePhraseChild("C_CODE_PHRASE", "local", "at0001", " "))), want: "Heart Rate"},
		{name: "a value pattern the code's text matches", attrs: nameAttribute(codedNameChild("<pattern>Heart.*</pattern>", codePhraseChild("C_CODE_PHRASE", "local", "at0001"))), want: "Heart Rate"},
		{name: "a value list pinned beside the code", attrs: nameAttribute(codedNameChild("<list>Pulse</list>", codePhraseChild("C_CODE_PHRASE", "local", "at0001"))), want: "Pulse"},

		{name: "a value pattern that rejects the code's text", attrs: nameAttribute(codedNameChild("<pattern>Other.*</pattern>", codePhraseChild("C_CODE_PHRASE", "local", "at0001"))), want: ""},
		{name: "several codes", attrs: nameAttribute(codedNameChild("", codePhraseChild("C_CODE_PHRASE", "local", "at0001", "at0010"))), want: ""},
		{name: "a local code with no term", attrs: nameAttribute(localCode("at0099")), want: ""},
		{name: "a local code whose term has no text", attrs: nameAttribute(localCode("at0002")), want: ""},
		{name: "an openehr code that two groups give two rubrics", attrs: nameAttribute(codedNameChild("", codePhraseChild("C_CODE_PHRASE", "openehr", "532"))), want: ""},
		{name: "an external at-code, whose bare key names an archetype term", attrs: nameAttribute(codedNameChild("", codePhraseChild("C_CODE_PHRASE", "SNOMED-CT", "at0001"))), want: ""},
		{name: "a code phrase that names no terminology", attrs: nameAttribute(codedNameChild("", codePhraseChild("C_CODE_PHRASE", "", "12345"))), want: ""},
		{name: "a code phrase that lists no code", attrs: nameAttribute(codedNameChild("", codePhraseChild("C_CODE_PHRASE", "local"))), want: ""},
		{name: "defining_code alternatives", attrs: nameAttribute(codedNameChild("", codePhraseChild("C_CODE_PHRASE", "local", "at0001"), codePhraseChild("C_CODE_PHRASE", "local", "at0001"))), want: ""},
		{name: "name alternatives", attrs: nameAttribute(localCode("at0001"), localCode("at0001")), want: ""},
		{name: "a DV_TEXT name alternative", attrs: nameAttribute(nameValue("DV_TEXT", "<pattern>.*</pattern>"), localCode("at0001")), want: ""},
	}
	for _, mode := range readerModes {
		for _, tc := range cases {
			t.Run(mode.name+"/"+tc.name, func(t *testing.T) {
				c, err := readAndCompile(fmt.Sprintf(codedNameOPT, tc.attrs, ""), mode.opts...)
				if err != nil {
					t.Fatalf("read and compile = %v, want the template", err)
				}
				if got := c.Root().NodeName(); got != tc.want {
					t.Errorf("Root().NodeName() = %q, want %q", got, tc.want)
				}
			})
		}
	}
}

// TestREQ111_CodedNodeNameReadsItsOwnArchetypesTerms pins that a local code
// takes the text the node's own archetype defines for it (§ REQ-111 Node
// names and name predicates, issue 455): a SECTION root coded with a code its
// own terms define is named by that text, and its compiled path carries the
// name predicate, while one coded with a code only the enclosing COMPOSITION
// defines pins no name and its path no predicate.
func TestREQ111_CodedNodeNameReadsItsOwnArchetypesTerms(t *testing.T) {
	cases := []struct {
		name, code, wantName, wantPath string
	}{
		{name: "a code its own archetype defines", code: "at0010", wantName: "Own heading", wantPath: "/content[openEHR-EHR-SECTION.adhoc.v1,'Own heading']"},
		{name: "a code only the enclosing archetype defines", code: "at0001", wantName: "", wantPath: "/content[openEHR-EHR-SECTION.adhoc.v1]"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, err := readAndCompile(fmt.Sprintf(codedNameOPT, "", nameAttribute(localCode(tc.code))))
			if err != nil {
				t.Fatalf("read and compile = %v, want the template", err)
			}
			sections := c.AllByArchetypeID("openEHR-EHR-SECTION.adhoc.v1")
			if len(sections) != 1 {
				t.Fatalf("AllByArchetypeID(SECTION.adhoc.v1) = %d nodes, want 1", len(sections))
			}
			if got := sections[0].NodeName(); got != tc.wantName {
				t.Errorf("SECTION NodeName() = %q, want %q", got, tc.wantName)
			}
			if got := sections[0].AQLPath(); got != tc.wantPath {
				t.Errorf("SECTION AQLPath() = %q, want %q", got, tc.wantPath)
			}
		})
	}
}

// TestREQ111_CodedNodeNameOnTheVitalSignsPulseRate pins issue 455 on the
// corpus: the IDCR - Vital Signs Encounter.v1 template names the pulse
// archetype's ELEMENT at0004, whose archetype term is "Rate", with the local
// code at1027, whose term text is "Heart Rate". That text is the node's
// template-level name, and its compiled path carries it as a name predicate.
func TestREQ111_CodedNodeNameOnTheVitalSignsPulseRate(t *testing.T) {
	const path = "/content[openEHR-EHR-SECTION.vital_signs.v1]/items[openEHR-EHR-OBSERVATION.pulse.v1]/data[at0002]/events[at0003]/data[at0001]/items[at0004,'Heart Rate']"
	c, err := templatecompile.Compile(mustReadModelFile(t, fixtures.TemplateOpt("IDCR - Vital Signs Encounter.v1")))
	if err != nil {
		t.Fatalf("Compile(IDCR - Vital Signs Encounter.v1) = %v", err)
	}
	n, err := c.NodeAt(path)
	if err != nil {
		t.Fatalf("NodeAt(%s) = %v, want the pulse rate ELEMENT", path, err)
	}
	if got := n.NodeName(); got != "Heart Rate" {
		t.Errorf("NodeName() = %q, want %q", got, "Heart Rate")
	}
	if term, ok := n.Term("at0004", ""); !ok || term.Items["text"] != "Rate" {
		t.Errorf(`Term("at0004") = %v, %v, want the archetype term "Rate" — fixture changed?`, term, ok)
	}
}
