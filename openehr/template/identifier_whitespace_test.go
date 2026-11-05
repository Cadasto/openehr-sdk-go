package template_test

import (
	"bytes"
	jsonv2 "encoding/json/v2"
	"fmt"
	mrand "math/rand/v2"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/cadasto/openehr-sdk-go/openehr/aom/aom14"
	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// identifierOPT is a COMPOSITION, coded as an event, whose content holds an
// OBSERVATION with one ELEMENT whose value is a DV_INTERVAL<DV_DATE_TIME>
// with both bounds constrained to a date-time pattern. The OBSERVATION
// archetype root names a template id of its own. Every identifier element
// is written on one line, and so is the path of its one annotation;
// paddedElementOPT pads them.
const identifierOPT = `<?xml version="1.0" encoding="UTF-8"?>
<template xmlns="http://schemas.openehr.org/v1" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <language><terminology_id><value>ISO_639-1</value></terminology_id><code_string>en</code_string></language>
  <template_id><value>padded_identifiers</value></template_id>
  <concept>padded_identifiers</concept>
  <definition>
    <rm_type_name>COMPOSITION</rm_type_name>
    <occurrences><lower>1</lower><upper>1</upper></occurrences>
    <node_id>at0000</node_id>
    <attributes xsi:type="C_SINGLE_ATTRIBUTE">
      <rm_attribute_name>category</rm_attribute_name>
      <existence><lower>1</lower><upper>1</upper></existence>
      <children xsi:type="C_COMPLEX_OBJECT">
        <rm_type_name>DV_CODED_TEXT</rm_type_name>
        <occurrences><lower>1</lower><upper>1</upper></occurrences>
        <node_id></node_id>
        <attributes xsi:type="C_SINGLE_ATTRIBUTE">
          <rm_attribute_name>defining_code</rm_attribute_name>
          <existence><lower>1</lower><upper>1</upper></existence>
          <children xsi:type="C_CODE_PHRASE">
            <rm_type_name>CODE_PHRASE</rm_type_name>
            <occurrences><lower>1</lower><upper>1</upper></occurrences>
            <node_id></node_id>
            <terminology_id><value>openehr</value></terminology_id>
            <code_list>433</code_list>
          </children>
        </attributes>
      </children>
    </attributes>
    <attributes xsi:type="C_MULTIPLE_ATTRIBUTE">
      <rm_attribute_name>content</rm_attribute_name>
      <existence><lower>0</lower><upper>1</upper></existence>
      <children xsi:type="C_ARCHETYPE_ROOT">
        <rm_type_name>OBSERVATION</rm_type_name>
        <occurrences><lower>0</lower><upper>1</upper></occurrences>
        <node_id>at0000</node_id>
        <attributes xsi:type="C_SINGLE_ATTRIBUTE">
          <rm_attribute_name>data</rm_attribute_name>
          <existence><lower>1</lower><upper>1</upper></existence>
          <children xsi:type="C_COMPLEX_OBJECT">
            <rm_type_name>HISTORY</rm_type_name>
            <occurrences><lower>1</lower><upper>1</upper></occurrences>
            <node_id>at0001</node_id>
            <attributes xsi:type="C_MULTIPLE_ATTRIBUTE">
              <rm_attribute_name>events</rm_attribute_name>
              <existence><lower>1</lower><upper>1</upper></existence>
              <children xsi:type="C_COMPLEX_OBJECT">
                <rm_type_name>POINT_EVENT</rm_type_name>
                <occurrences><lower>0</lower><upper_unbounded>true</upper_unbounded></occurrences>
                <node_id>at0002</node_id>
                <attributes xsi:type="C_SINGLE_ATTRIBUTE">
                  <rm_attribute_name>data</rm_attribute_name>
                  <existence><lower>1</lower><upper>1</upper></existence>
                  <children xsi:type="C_COMPLEX_OBJECT">
                    <rm_type_name>ITEM_TREE</rm_type_name>
                    <occurrences><lower>1</lower><upper>1</upper></occurrences>
                    <node_id>at0003</node_id>
                    <attributes xsi:type="C_MULTIPLE_ATTRIBUTE">
                      <rm_attribute_name>items</rm_attribute_name>
                      <existence><lower>0</lower><upper>1</upper></existence>
                      <children xsi:type="C_COMPLEX_OBJECT">
                        <rm_type_name>ELEMENT</rm_type_name>
                        <occurrences><lower>0</lower><upper>1</upper></occurrences>
                        <node_id>at0004</node_id>
                        <attributes xsi:type="C_SINGLE_ATTRIBUTE">
                          <rm_attribute_name>value</rm_attribute_name>
                          <existence><lower>0</lower><upper>1</upper></existence>
                          <children xsi:type="C_COMPLEX_OBJECT">
                            <rm_type_name>DV_INTERVAL&lt;DV_DATE_TIME&gt;</rm_type_name>
                            <occurrences><lower>1</lower><upper>1</upper></occurrences>
                            <node_id></node_id>
                            %[1]s
                            %[2]s
                          </children>
                        </attributes>
                      </children>
                      <cardinality><is_ordered>false</is_ordered><is_unique>false</is_unique><interval><lower>0</lower><upper_unbounded>true</upper_unbounded></interval></cardinality>
                    </attributes>
                  </children>
                </attributes>
              </children>
              <cardinality><is_ordered>true</is_ordered><is_unique>false</is_unique><interval><lower>1</lower><upper_unbounded>true</upper_unbounded></interval></cardinality>
            </attributes>
          </children>
        </attributes>
        <archetype_id><value>openEHR-EHR-OBSERVATION.padded.v1</value></archetype_id>
        <template_id><value>padded_observation</value></template_id>
        <term_definitions code="at0000"><items id="text">Padded</items><items id="description">An observation.</items></term_definitions>
        <term_definitions code="at0001"><items id="text">History</items><items id="description">The history.</items></term_definitions>
        <term_definitions code="at0002"><items id="text">Any event</items><items id="description">An event.</items></term_definitions>
        <term_definitions code="at0003"><items id="text">Tree</items><items id="description">The tree.</items></term_definitions>
        <term_definitions code="at0004"><items id="text">Period</items><items id="description">A period.</items></term_definitions>
      </children>
      <cardinality><is_ordered>false</is_ordered><is_unique>false</is_unique><interval><lower>0</lower><upper_unbounded>true</upper_unbounded></interval></cardinality>
    </attributes>
    <archetype_id><value>openEHR-EHR-COMPOSITION.encounter.v1</value></archetype_id>
    <term_definitions code="at0000"><items id="text">Encounter</items><items id="description">An encounter.</items></term_definitions>
  </definition>
  <annotations path="/content[openEHR-EHR-OBSERVATION.padded.v1]/data[at0001]/events[at0002]/data[at0003]/items[at0004]"><items id="comment">A period.</items></annotations>
</template>`

// identifierBound is the %s attribute of a DV_INTERVAL<DV_DATE_TIME> that
// constrains that bound to a date-time pattern.
const identifierBound = `<attributes xsi:type="C_SINGLE_ATTRIBUTE">
                              <rm_attribute_name>%s</rm_attribute_name>
                              <existence><lower>0</lower><upper>1</upper></existence>
                              <children xsi:type="C_COMPLEX_OBJECT">
                                <rm_type_name>DV_DATE_TIME</rm_type_name>
                                <occurrences><lower>1</lower><upper>1</upper></occurrences>
                                <node_id></node_id>
                                <attributes xsi:type="C_SINGLE_ATTRIBUTE">
                                  <rm_attribute_name>value</rm_attribute_name>
                                  <existence><lower>1</lower><upper>1</upper></existence>
                                  <children xsi:type="C_PRIMITIVE_OBJECT">
                                    <rm_type_name>DATE_TIME</rm_type_name>
                                    <occurrences><lower>1</lower><upper>1</upper></occurrences>
                                    <node_id></node_id>
                                    <item xsi:type="C_DATE_TIME"><pattern>yyyy-??-??T??:??:??</pattern></item>
                                  </children>
                                </attributes>
                              </children>
                            </attributes>`

// unpaddedIdentifierOPT is identifierOPT as written.
func unpaddedIdentifierOPT() string {
	return fmt.Sprintf(identifierOPT, fmt.Sprintf(identifierBound, "lower"), fmt.Sprintf(identifierBound, "upper"))
}

// elementText matches the text of each identifier element of identifierOPT,
// the <value> of its template, archetype and terminology ids among them, and
// the annotation path. It leaves out the xsi:type values: the reader's strict
// mode, which the test reads with, refuses an xsi:type padded with white
// space, as it refuses an unknown one.
var elementText = regexp.MustCompile(`(<(?:rm_attribute_name|rm_type_name|node_id|value|code_list|code_string)>)([^<]*)(</)|(path=")([^"]*)(")`)

// paddedElementOPT is identifierOPT as a pretty-printer writes it: the text
// of every identifier element and the annotation path start and end with a
// line break and indentation, as in "defining_code\n   ". Every xsi:type
// value is left as written.
func paddedElementOPT() string {
	return elementText.ReplaceAllString(unpaddedIdentifierOPT(), "$1$4\n \t\t  $2$5\n\t   $3$6")
}

// compiledAttributes lists the path of every attribute under n, with " not
// in RM" after each the compile marks NotInRM.
func compiledAttributes(n *templatecompile.CompiledNode, path string, out *[]string) {
	for _, a := range n.Attributes() {
		p := path + "/" + a.Name()
		if a.NotInRM() {
			*out = append(*out, p+" not in RM")
		} else {
			*out = append(*out, p)
		}
		for _, c := range a.Children() {
			compiledAttributes(c, p, out)
		}
	}
}

// identifierOptions are the generator settings the padded OPT is checked
// under: every policy, each with both value fills, and a fixed clock and
// uid source so two runs over equivalent templates write the same root.
func identifierOptions() []instance.Options {
	name := "Test Composer"
	var out []instance.Options
	for _, policy := range []instance.Policy{instance.Medium, instance.Required, instance.Large} {
		for _, fill := range []instance.ValueFill{instance.ExampleFill, instance.RandomFill} {
			out = append(out, instance.Options{
				Policy:    policy,
				ValueFill: fill,
				Territory: "NL",
				Composer:  &rm.PartyIdentified{Name: &name},
				Now:       time.Date(2021, 3, 4, 5, 6, 7, 0, time.UTC),
			})
		}
	}
	return out
}

// generateJSON generates a root for c under opts, with fresh seeded sources,
// and returns it with its canonical JSON.
func generateJSON(t *testing.T, c *templatecompile.Compiled, opts instance.Options) (any, []byte) {
	t.Helper()
	opts.UIDSource = instance.UIDsFrom(mrand.NewPCG(1, 2))
	if opts.ValueFill == instance.RandomFill {
		opts.ValueSource = mrand.NewPCG(3, 4)
	}
	out, err := instance.Generate(t.Context(), c, opts)
	if err != nil {
		t.Fatalf("Generate(%v, %v): %v, want a root", opts.Policy, opts.ValueFill, err)
	}
	raw, err := canjson.Marshal(out)
	if err != nil {
		t.Fatalf("canjson.Marshal(Generate(%v, %v)): %v", opts.Policy, opts.ValueFill, err)
	}
	return out, raw
}

// holdsType reports whether the canonical JSON raw holds an object whose
// _type is typ.
func holdsType(t *testing.T, raw []byte, typ string) bool {
	t.Helper()
	var tree any
	if err := jsonv2.Unmarshal(raw, &tree); err != nil {
		t.Fatalf("json unmarshal: %v", err)
	}
	var walk func(v any) bool
	walk = func(v any) bool {
		switch node := v.(type) {
		case map[string]any:
			if got, _ := node["_type"].(string); got == typ {
				return true
			}
			for _, child := range node {
				if walk(child) {
					return true
				}
			}
		case []any:
			return slices.ContainsFunc(node, walk)
		}
		return false
	}
	return walk(tree)
}

// TestREQ111_PaddedIdentifiersCompileGenerateAndValidate pins the effect of
// § REQ-111 Reading the model on the compile, the generator (REQ-107) and the
// validators (REQ-102): the padded OPT (paddedElementOPT, which leaves the
// xsi:type values as written) compiles to the attributes the unpadded one
// compiles to, none marked NotInRM, and under every policy and
// value fill the generator writes the root it writes for the unpadded OPT,
// which both validators accept. Medium and Large build the optional
// OBSERVATION, so their root holds the interval and its two bounds.
func TestREQ111_PaddedIdentifiersCompileGenerateAndValidate(t *testing.T) {
	compile := func(t *testing.T, body string) *templatecompile.Compiled {
		t.Helper()
		opt, err := aom14.ReadOperationalTemplate(strings.NewReader(body), canxml.WithStrict())
		if err != nil {
			t.Fatalf("ReadOperationalTemplate(strict): %v", err)
		}
		c, err := templatecompile.Compile(opt)
		if err != nil {
			t.Fatalf("Compile: %v", err)
		}
		return c
	}
	plain := compile(t, unpaddedIdentifierOPT())
	padded := compile(t, paddedElementOPT())

	var got, want []string
	compiledAttributes(padded.Root(), "", &got)
	compiledAttributes(plain.Root(), "", &want)
	if !slices.Equal(got, want) {
		t.Errorf("Compile(padded) attributes = %q, want %q", got, want)
	}
	for _, p := range got {
		if strings.HasSuffix(p, " not in RM") {
			t.Errorf("Compile(padded) marks %s", p)
		}
	}

	for _, opts := range identifierOptions() {
		call := fmt.Sprintf("Generate(%v, %v)", opts.Policy, opts.ValueFill)
		t.Run(call, func(t *testing.T) {
			out, gotJSON := generateJSON(t, padded, opts)
			_, wantJSON := generateJSON(t, plain, opts)
			if !bytes.Equal(gotJSON, wantJSON) {
				t.Errorf("%s over the padded OPT =\n%s\nwant the root written for the unpadded OPT:\n%s", call, gotJSON, wantJSON)
			}
			if opts.Policy != instance.Required && !holdsType(t, gotJSON, "DV_INTERVAL") {
				t.Errorf("%s over the padded OPT holds no DV_INTERVAL:\n%s", call, gotJSON)
			}
			if r := validation.ValidateRM(out); !r.OK {
				t.Errorf("ValidateRM(%s) issues %+v, want none", call, r.Issues)
			}
			r := validation.Validate(out, padded)
			if !r.OK {
				t.Errorf("Validate(%s, padded) issues %+v, want none", call, r.Issues)
			}
			if len(r.Coverage.NotInRM) != 0 {
				t.Errorf("Validate(%s, padded) Coverage.NotInRM = %q, want none", call, r.Coverage.NotInRM)
			}
		})
	}
}
