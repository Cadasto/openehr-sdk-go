package instance_test

import (
	jsonv2 "encoding/json/v2"
	"fmt"
	mrand "math/rand/v2"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
)

// TestREQ107_NoPlaceholderDateTimeLanguageOrLocalExample walks a generated
// composition and rejects the placeholder the string pass used to stamp:
// a DV_DATE_TIME value of "example", an ENTRY language or encoding code
// "example", or any code phrase local::example. An open DV_TEXT value may
// still be "example" (REQ-103).
func TestREQ107_NoPlaceholderDateTimeLanguageOrLocalExample(t *testing.T) {
	now := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
	for _, name := range []string{"vital_signs", "Test_dv_ordinal_with_constraints.v0"} {
		t.Run(name, func(t *testing.T) {
			out := generateCompiled(t, compileFixture(t, name), instance.Options{
				Policy:    instance.Example,
				Language:  "en",
				Territory: "NL",
				Composer:  testComposer(),
				Now:       now,
			})
			problems := placeholderProblems(t, out)
			if len(problems) > 0 {
				t.Fatalf("REQ-107 placeholder values:\n%s", strings.Join(problems, "\n"))
			}
		})
	}
}

// TestREQ107_OrdinalSymbolFromConstraintPair checks ExampleFill copies the
// first C_DV_ORDINAL pair onto DV_ORDINAL.symbol, and RandomFill copies the
// pair for the value it drew. An empty symbol does not satisfy the pair list.
func TestREQ107_OrdinalSymbolFromConstraintPair(t *testing.T) {
	now := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
	c := compileSyntheticOPT(t, ordinalPairOPT)
	wantExample := rm.CodePhrase{
		CodeString:    "at0010",
		TerminologyID: rm.TerminologyID{Value: "local"},
	}

	t.Run("ExampleFill", func(t *testing.T) {
		out := generateCompiled(t, c, instance.Options{
			Policy:    instance.Example,
			Language:  "en",
			Territory: "NL",
			Composer:  testComposer(),
			Now:       now,
		})
		comp, err := instance.AsComposition(out)
		if err != nil {
			t.Fatalf("AsComposition: %v", err)
		}
		got := ordinalsOf(comp)
		if len(got) != 1 {
			t.Fatalf("ordinals = %d, want 1", len(got))
		}
		if got[0].Value != 0 {
			t.Errorf("ordinal value = %d, want 0 (first pair)", got[0].Value)
		}
		if got[0].Symbol.DefiningCode != wantExample {
			t.Errorf("ordinal symbol = %+v, want %+v", got[0].Symbol.DefiningCode, wantExample)
		}
	})

	t.Run("RandomFill", func(t *testing.T) {
		seen := map[rm.Integer]string{}
		for seed := uint64(1); seed <= 24; seed++ {
			out := generateCompiled(t, c, instance.Options{
				Policy:      instance.Example,
				Language:    "en",
				Territory:   "NL",
				Composer:    testComposer(),
				Now:         now,
				ValueFill:   instance.RandomFill,
				ValueSource: mrand.NewPCG(seed, seed),
			})
			comp, err := instance.AsComposition(out)
			if err != nil {
				t.Fatalf("seed %d AsComposition: %v", seed, err)
			}
			got := ordinalsOf(comp)
			if len(got) != 1 {
				t.Fatalf("seed %d ordinals = %d, want 1", seed, len(got))
			}
			wantCode, ok := ordinalPairCode(got[0].Value)
			if !ok {
				t.Fatalf("seed %d ordinal value %d is not a template pair", seed, got[0].Value)
			}
			if got[0].Symbol.DefiningCode.CodeString != wantCode ||
				got[0].Symbol.DefiningCode.TerminologyID.Value != "local" {
				t.Errorf("seed %d value %d symbol = %+v, want local::%s",
					seed, got[0].Value, got[0].Symbol.DefiningCode, wantCode)
			}
			seen[got[0].Value] = got[0].Symbol.DefiningCode.CodeString
		}
		if _, ok := seen[0]; !ok {
			t.Error("RandomFill never drew ordinal value 0")
		}
		if _, ok := seen[2]; !ok {
			t.Error("RandomFill never drew ordinal value 2")
		}
	})
}

func ordinalPairCode(v rm.Integer) (string, bool) {
	switch v {
	case 0:
		return "at0010", true
	case 2:
		return "at0011", true
	default:
		return "", false
	}
}

func generateCompiled(t *testing.T, c *templatecompile.Compiled, opts instance.Options) any {
	t.Helper()
	out, err := instance.Generate(t.Context(), c, opts)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	return out
}

func placeholderProblems(t *testing.T, root any) []string {
	t.Helper()
	raw, err := canjson.Marshal(root)
	if err != nil {
		t.Fatalf("canjson.Marshal: %v", err)
	}
	var tree any
	if err := jsonv2.Unmarshal(raw, &tree); err != nil {
		t.Fatalf("json unmarshal: %v", err)
	}
	var problems []string
	walkPlaceholders(tree, "", &problems)
	slices.Sort(problems)
	return problems
}

func walkPlaceholders(v any, path string, problems *[]string) {
	switch n := v.(type) {
	case map[string]any:
		loc := path
		if loc == "" {
			loc = "/"
		}
		if typ, _ := n["_type"].(string); typ == "DV_DATE_TIME" && jsonString(n, "value") == "example" {
			*problems = append(*problems, "DV_DATE_TIME.value example at "+loc)
		}
		if attr := lastJSONAttr(loc); (attr == "language" || attr == "encoding") && jsonString(n, "code_string") == "example" {
			*problems = append(*problems, attr+" code_string example at "+loc)
		}
		if jsonTerminology(n) == "local" && jsonString(n, "code_string") == "example" {
			*problems = append(*problems, "local::example at "+loc)
		}
		for k, child := range n {
			walkPlaceholders(child, joinJSON(loc, k), problems)
		}
	case []any:
		base := path
		if base == "" {
			base = "/"
		}
		for i, child := range n {
			walkPlaceholders(child, fmt.Sprintf("%s[%d]", base, i), problems)
		}
	}
}

func jsonString(n map[string]any, key string) string {
	s, _ := n[key].(string)
	return s
}

func jsonTerminology(n map[string]any) string {
	switch term := n["terminology_id"].(type) {
	case string:
		return term
	case map[string]any:
		return jsonString(term, "value")
	default:
		return ""
	}
}

func joinJSON(parent, key string) string {
	if parent == "" || parent == "/" {
		return "/" + key
	}
	return parent + "/" + key
}

func lastJSONAttr(path string) string {
	path = strings.TrimSuffix(path, "/")
	if i := strings.LastIndex(path, "/"); i >= 0 {
		path = path[i+1:]
	}
	if i := strings.IndexByte(path, '['); i >= 0 {
		path = path[:i]
	}
	return path
}

func ordinalsOf(comp *rm.Composition) []*rm.DVOrdinal {
	var out []*rm.DVOrdinal
	for _, item := range comp.Content {
		walkContent(item, &out)
	}
	return out
}

func walkContent(item rm.ContentItem, out *[]*rm.DVOrdinal) {
	switch n := item.(type) {
	case *rm.Observation:
		walkHistory(n.Data, out)
		walkStructure(n.Protocol, out)
	case *rm.Evaluation:
		walkStructure(n.Data, out)
		walkStructure(n.Protocol, out)
	case *rm.Instruction:
		walkStructure(n.Protocol, out)
	case *rm.AdminEntry:
		walkStructure(n.Data, out)
	case *rm.Section:
		for _, child := range n.Items {
			walkContent(child, out)
		}
	}
}

func walkHistory(h rm.History[rm.ItemStructure], out *[]*rm.DVOrdinal) {
	for _, ev := range h.Events {
		switch e := ev.(type) {
		case *rm.PointEvent[rm.ItemStructure]:
			walkStructure(e.Data, out)
			walkStructure(e.State, out)
		case *rm.IntervalEvent[rm.ItemStructure]:
			walkStructure(e.Data, out)
			walkStructure(e.State, out)
		}
	}
	walkStructure(h.Summary, out)
}

func walkStructure(s rm.ItemStructure, out *[]*rm.DVOrdinal) {
	switch n := s.(type) {
	case *rm.ItemTree:
		for _, item := range n.Items {
			walkItem(item, out)
		}
	case *rm.ItemList:
		for i := range n.Items {
			walkElement(&n.Items[i], out)
		}
	case *rm.ItemSingle:
		walkElement(&n.Item, out)
	case *rm.ItemTable:
		for i := range n.Rows {
			walkItem(&n.Rows[i], out)
		}
	}
}

func walkItem(item rm.Item, out *[]*rm.DVOrdinal) {
	switch n := item.(type) {
	case *rm.Element:
		walkElement(n, out)
	case *rm.Cluster:
		for _, child := range n.Items {
			walkItem(child, out)
		}
	case rm.Element:
		el := n
		walkElement(&el, out)
	case rm.Cluster:
		cl := n
		for _, child := range cl.Items {
			walkItem(child, out)
		}
	}
}

func walkElement(el *rm.Element, out *[]*rm.DVOrdinal) {
	if el == nil {
		return
	}
	if o, ok := el.Value.(*rm.DVOrdinal); ok {
		*out = append(*out, o)
	}
}

const ordinalPairOPT = `<?xml version="1.0" encoding="utf-8"?>
<template xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xmlns:xsd="http://www.w3.org/2001/XMLSchema" xmlns="http://schemas.openehr.org/v1">
  <language>
    <terminology_id><value>ISO_639-1</value></terminology_id>
    <code_string>en</code_string>
  </language>
  <template_id><value>ordinal_pair</value></template_id>
  <concept>ordinal_pair</concept>
  <definition>
    <rm_type_name>COMPOSITION</rm_type_name>
    <node_id>at0000</node_id>
    <attributes xsi:type="C_MULTIPLE_ATTRIBUTE">
      <rm_attribute_name>content</rm_attribute_name>
      <children xsi:type="C_ARCHETYPE_ROOT">
        <rm_type_name>OBSERVATION</rm_type_name>
        <node_id>at0000</node_id>
        <attributes xsi:type="C_SINGLE_ATTRIBUTE">
          <rm_attribute_name>data</rm_attribute_name>
          <children xsi:type="C_COMPLEX_OBJECT">
            <rm_type_name>HISTORY</rm_type_name>
            <node_id>at0001</node_id>
            <attributes xsi:type="C_MULTIPLE_ATTRIBUTE">
              <rm_attribute_name>events</rm_attribute_name>
              <children xsi:type="C_COMPLEX_OBJECT">
                <rm_type_name>POINT_EVENT</rm_type_name>
                <node_id>at0002</node_id>
                <attributes xsi:type="C_SINGLE_ATTRIBUTE">
                  <rm_attribute_name>data</rm_attribute_name>
                  <children xsi:type="C_COMPLEX_OBJECT">
                    <rm_type_name>ITEM_TREE</rm_type_name>
                    <node_id>at0003</node_id>
                    <attributes xsi:type="C_MULTIPLE_ATTRIBUTE">
                      <rm_attribute_name>items</rm_attribute_name>
                      <children xsi:type="C_COMPLEX_OBJECT">
                        <rm_type_name>ELEMENT</rm_type_name>
                        <node_id>at0004</node_id>
                        <attributes xsi:type="C_SINGLE_ATTRIBUTE">
                          <rm_attribute_name>value</rm_attribute_name>
                          <children xsi:type="C_DV_ORDINAL">
                            <rm_type_name>DV_ORDINAL</rm_type_name>
                            <node_id />
                            <list>
                              <value>0</value>
                              <symbol>
                                <defining_code>
                                  <terminology_id><value>local</value></terminology_id>
                                  <code_string>at0010</code_string>
                                </defining_code>
                              </symbol>
                            </list>
                            <list>
                              <value>2</value>
                              <symbol>
                                <defining_code>
                                  <terminology_id><value>local</value></terminology_id>
                                  <code_string>at0011</code_string>
                                </defining_code>
                              </symbol>
                            </list>
                          </children>
                        </attributes>
                      </children>
                    </attributes>
                  </children>
                </attributes>
              </children>
            </attributes>
          </children>
        </attributes>
        <archetype_id><value>openEHR-EHR-OBSERVATION.ordinal_pair.v1</value></archetype_id>
      </children>
    </attributes>
    <archetype_id><value>openEHR-EHR-COMPOSITION.ordinal_pair.v1</value></archetype_id>
  </definition>
</template>`
