package template_test

import (
	"bytes"
	jsonv2 "encoding/json/v2"
	"errors"
	"fmt"
	mrand "math/rand/v2"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/template/constraints"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// identifierOPT is a COMPOSITION, coded as an event, whose content holds an
// OBSERVATION with one ELEMENT whose value is a DV_INTERVAL<DV_DATE_TIME>
// with both bounds constrained to a date-time pattern. The OBSERVATION
// archetype root names a template id of its own. Every identifier element
// is written on one line, and so is the path of its one annotation;
// paddedIdentifierOPT pads them.
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

// identifierText matches the text of each identifier element of
// identifierOPT, the <value> of its template, archetype and terminology
// ids among them, each xsi:type value and the annotation path.
var identifierText = regexp.MustCompile(`(<(?:rm_attribute_name|rm_type_name|node_id|value|code_list|code_string)>)([^<]*)(</)|((?:xsi:type|path)=")([^"]*)(")`)

// unpaddedIdentifierOPT is identifierOPT as written.
func unpaddedIdentifierOPT() string {
	return fmt.Sprintf(identifierOPT, fmt.Sprintf(identifierBound, "lower"), fmt.Sprintf(identifierBound, "upper"))
}

// paddedIdentifierOPT is identifierOPT as a pretty-printer writes it: the
// text of every identifier element, every xsi:type value and the annotation
// path start and end with a line break and indentation, as in
// "defining_code\n   ".
func paddedIdentifierOPT() string {
	return identifierText.ReplaceAllString(unpaddedIdentifierOPT(), "$1$4\n \t\t  $2$5\n\t   $3$6")
}

// dumpNode writes n and its subtree, one line per node or attribute, with
// every name, id and primitive constraint the parser read.
func dumpNode(b *strings.Builder, n template.Node, indent string) {
	switch v := n.(type) {
	case *template.ArchetypeRoot:
		fmt.Fprintf(b, "%sroot %q %q archetype %q template %q name %q terms %v\n",
			indent, v.RMTypeName(), v.NodeID(), v.ArchetypeID(), v.TemplateID(), v.NodeName(), v.Terms())
		for _, a := range v.Attributes() {
			dumpNode(b, a, indent+"  ")
		}
	case *template.ComplexObject:
		fmt.Fprintf(b, "%sobject %q %q name %q constraint %T %+v\n",
			indent, v.RMTypeName(), v.NodeID(), v.NodeName(), v.PrimitiveConstraint(), v.PrimitiveConstraint())
		for _, a := range v.Attributes() {
			dumpNode(b, a, indent+"  ")
		}
	case *template.Attribute:
		fmt.Fprintf(b, "%sattribute %q %v\n", indent, v.Name(), v.Cardinality())
		for _, c := range v.Children() {
			dumpNode(b, c, indent+"  ")
		}
	default:
		fmt.Fprintf(b, "%s%T %q %q\n", indent, n, n.RMTypeName(), n.NodeID())
	}
}

// dumpOPT writes the identity fields of tmpl, its annotations and its
// definition tree.
func dumpOPT(tmpl *template.OperationalTemplate) string {
	var b strings.Builder
	fmt.Fprintf(&b, "template %q concept %q language %q annotations %v\n", tmpl.TemplateID(), tmpl.Concept(), tmpl.Language(), tmpl.Annotations())
	dumpNode(&b, tmpl.Root(), "")
	return b.String()
}

// nodeAt resolves path on tmpl, failing t when it does not resolve.
func nodeAt(t *testing.T, tmpl *template.OperationalTemplate, path string) template.Node {
	t.Helper()
	p, err := tmpl.ParsePath(path)
	if err != nil {
		t.Fatalf("ParsePath(%q): %v", path, err)
	}
	n, err := tmpl.NodeAt(p)
	if err != nil {
		t.Fatalf("NodeAt(%q): %v", path, err)
	}
	return n
}

// TestREQ100_IdentifierElementsReadTrimmed pins § REQ-100 Node taxonomy:
// both parse modes read each identifier element, and each xsi:type value,
// without the white space and line breaks a pretty-printer puts around its
// text. The padded OPT parses to the tree the unpadded one parses to, so
// its attribute names, RM type names, node ids, archetype and template ids,
// its C_CODE_PHRASE terminology and codes and its annotation path are the
// unpadded ones.
func TestREQ100_IdentifierElementsReadTrimmed(t *testing.T) {
	padded := paddedIdentifierOPT()
	for _, want := range []string{"<rm_attribute_name>\n \t\t  defining_code\n\t   </", "<rm_type_name>\n \t\t  DV_DATE_TIME\n\t   </", "xsi:type=\"\n \t\t  C_SINGLE_ATTRIBUTE\n\t   \""} {
		if !strings.Contains(padded, want) {
			t.Fatalf("paddedIdentifierOPT() does not contain %q", want)
		}
	}
	for _, mode := range parseModes {
		t.Run(mode.name, func(t *testing.T) {
			plain, err := mode.parse(strings.NewReader(unpaddedIdentifierOPT()))
			if err != nil {
				t.Fatalf("%s(unpadded): %v", mode.name, err)
			}
			tmpl, err := mode.parse(strings.NewReader(padded))
			if err != nil {
				t.Fatalf("%s(padded): %v", mode.name, err)
			}
			if got, want := dumpOPT(tmpl), dumpOPT(plain); got != want {
				t.Errorf("%s(padded) tree differs from the unpadded tree:\ngot:\n%s\nwant:\n%s", mode.name, got, want)
			}

			if got := tmpl.TemplateID(); got != "padded_identifiers" {
				t.Errorf("TemplateID() = %q, want padded_identifiers", got)
			}
			root, ok := tmpl.Root().(*template.ArchetypeRoot)
			if !ok {
				t.Fatalf("Root() = %T, want *template.ArchetypeRoot", tmpl.Root())
			}
			if got := root.ArchetypeID(); got != "openEHR-EHR-COMPOSITION.encounter.v1" {
				t.Errorf("Root().ArchetypeID() = %q, want openEHR-EHR-COMPOSITION.encounter.v1", got)
			}
			if got := root.RMTypeName(); got != "COMPOSITION" {
				t.Errorf("Root().RMTypeName() = %q, want COMPOSITION", got)
			}
			if got := root.NodeID(); got != "at0000" {
				t.Errorf("Root().NodeID() = %q, want at0000", got)
			}

			obs, ok := nodeAt(t, tmpl, "/content[openEHR-EHR-OBSERVATION.padded.v1]").(*template.ArchetypeRoot)
			if !ok {
				t.Fatal("/content[openEHR-EHR-OBSERVATION.padded.v1] is not an archetype root")
			}
			if got := obs.TemplateID(); got != "padded_observation" {
				t.Errorf("OBSERVATION TemplateID() = %q, want padded_observation", got)
			}

			code, ok := nodeAt(t, tmpl, "/category/defining_code").(*template.ComplexObject)
			if !ok {
				t.Fatal("/category/defining_code is not a complex object")
			}
			want := constraints.CodePhrase{Terminology: "openehr", CodeList: []string{"433"}}
			got, ok := code.PrimitiveConstraint().(constraints.CodePhrase)
			if !ok || got.Terminology != want.Terminology || !slices.Equal(got.CodeList, want.CodeList) {
				t.Errorf("/category/defining_code PrimitiveConstraint() = %#v, want %#v", code.PrimitiveConstraint(), want)
			}

			element := "/content[openEHR-EHR-OBSERVATION.padded.v1]/data[at0001]/events[at0002]/data[at0003]/items[at0004]"
			if got := tmpl.Annotations()[element]; len(got) != 1 || got[0].Value != "A period." {
				t.Errorf("Annotations()[%q] = %+v, want the one comment; annotations %v", element, got, tmpl.Annotations())
			}
			value := element + "/value"
			if got := nodeAt(t, tmpl, value).RMTypeName(); got != "DV_INTERVAL<DV_DATE_TIME>" {
				t.Errorf("%s RMTypeName() = %q, want DV_INTERVAL<DV_DATE_TIME>", value, got)
			}
			for _, bound := range []string{"lower", "upper"} {
				if got := nodeAt(t, tmpl, value+"/"+bound).RMTypeName(); got != "DV_DATE_TIME" {
					t.Errorf("%s/%s RMTypeName() = %q, want DV_DATE_TIME", value, bound, got)
				}
			}
		})
	}
}

// TestREQ100_BlankXSITypeReadsAsEmpty pins § REQ-100 Node taxonomy and
// Strict parse mode together: an xsi:type that holds only white space reads
// as an empty one. Strict mode refuses an <attributes> or a <children> so
// typed, and the default mode reads them as it reads an untyped one.
func TestREQ100_BlankXSITypeReadsAsEmpty(t *testing.T) {
	const blank = "\n \t "
	attrBlank := strings.Replace(unpaddedIdentifierOPT(), `<attributes xsi:type="C_SINGLE_ATTRIBUTE">
      <rm_attribute_name>category</rm_attribute_name>`, `<attributes xsi:type="`+blank+`">
      <rm_attribute_name>category</rm_attribute_name>`, 1)
	childBlank := strings.Replace(unpaddedIdentifierOPT(), `<children xsi:type="C_CODE_PHRASE">`, `<children xsi:type="`+blank+`">`, 1)
	if attrBlank == unpaddedIdentifierOPT() || childBlank == unpaddedIdentifierOPT() {
		t.Fatal("the xsi:type to blank is not found")
	}
	tests := []struct {
		name string
		body string
	}{
		{name: "attributes", body: attrBlank},
		{name: "children", body: childBlank},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := template.ParseOPTStrict(strings.NewReader(tc.body))
			if !errors.Is(err, template.ErrInvalidOPT) || !strings.Contains(err.Error(), "no xsi:type") {
				t.Errorf("ParseOPTStrict(blank xsi:type on %s) error = %v, want ErrInvalidOPT naming no xsi:type", tc.name, err)
			}
			tmpl, err := template.ParseOPT(strings.NewReader(tc.body))
			if err != nil {
				t.Fatalf("ParseOPT(blank xsi:type on %s): %v, want it read as an untyped one", tc.name, err)
			}
			root, ok := tmpl.Root().(*template.ArchetypeRoot)
			if !ok {
				t.Fatalf("Root() = %T, want *template.ArchetypeRoot", tmpl.Root())
			}
			i := slices.IndexFunc(root.Attributes(), func(a *template.Attribute) bool { return a.Name() == "category" })
			if i < 0 || root.Attributes()[i].Cardinality() != template.Single {
				t.Errorf("ParseOPT(blank xsi:type on %s): no single attribute category on the root", tc.name)
			}
			if got := nodeAt(t, tmpl, "/category/defining_code").RMTypeName(); got != "CODE_PHRASE" {
				t.Errorf("/category/defining_code RMTypeName() = %q, want CODE_PHRASE", got)
			}
		})
	}
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

// TestREQ100_PaddedIdentifiersCompileGenerateAndValidate pins the effect of
// § REQ-100 Node taxonomy on the compile, the generator (REQ-107) and the
// validators (REQ-102): the padded OPT compiles to the attributes the
// unpadded one compiles to, none marked NotInRM, and under every policy and
// value fill the generator writes the root it writes for the unpadded OPT,
// which both validators accept. Medium and Large build the optional
// OBSERVATION, so their root holds the interval and its two bounds.
func TestREQ100_PaddedIdentifiersCompileGenerateAndValidate(t *testing.T) {
	compile := func(t *testing.T, body string) *templatecompile.Compiled {
		t.Helper()
		opt, err := template.ParseOPTStrict(strings.NewReader(body))
		if err != nil {
			t.Fatalf("ParseOPTStrict: %v", err)
		}
		c, err := templatecompile.Compile(opt)
		if err != nil {
			t.Fatalf("Compile: %v", err)
		}
		return c
	}
	plain := compile(t, unpaddedIdentifierOPT())
	padded := compile(t, paddedIdentifierOPT())

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
