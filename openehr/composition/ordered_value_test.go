package composition_test

import (
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/composition"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// ordinalOPT is a COMPOSITION whose context's other_context holds one
// ELEMENT with a DV_ORDINAL value the template constrains as a node of its
// own: a symbol coded at0003 or at0004, a value of 1 or 2, and a required
// normal_range, a DV_INTERVAL<DV_ORDINAL> each of whose bounds is 1|at0003
// or 2|at0004, listed from 1 on the lower side and from 2 on the upper.
var ordinalOPT = `<?xml version="1.0" encoding="utf-8"?>
<template xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xmlns="http://schemas.openehr.org/v1">
<language><terminology_id><value>ISO_639-1</value></terminology_id><code_string>en</code_string></language>
<template_id><value>ordinal</value></template_id><concept>ordinal</concept>
<definition><rm_type_name>COMPOSITION</rm_type_name><node_id>at0000</node_id>
<attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>context</rm_attribute_name>` + requiredExistence + `
<children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>EVENT_CONTEXT</rm_type_name><node_id></node_id>
<attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>other_context</rm_attribute_name>` + requiredExistence + `
<children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>ITEM_TREE</rm_type_name><node_id>at0001</node_id>
<attributes xsi:type="C_MULTIPLE_ATTRIBUTE"><rm_attribute_name>items</rm_attribute_name>
<children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>ELEMENT</rm_type_name><node_id>at0002</node_id>
<attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>value</rm_attribute_name>` + requiredExistence + `
<children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>DV_ORDINAL</rm_type_name><node_id></node_id>
<attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>symbol</rm_attribute_name>` + requiredExistence + `
<children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>DV_CODED_TEXT</rm_type_name><node_id></node_id>
<attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>defining_code</rm_attribute_name>` + requiredExistence + `
<children xsi:type="C_CODE_PHRASE"><rm_type_name>CODE_PHRASE</rm_type_name><node_id></node_id>
<terminology_id><value>local</value></terminology_id><code_list>at0003</code_list><code_list>at0004</code_list></children>
</attributes></children></attributes>
<attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>value</rm_attribute_name>` + requiredExistence + `
<children xsi:type="C_PRIMITIVE_OBJECT"><rm_type_name>INTEGER</rm_type_name><node_id></node_id>
<item xsi:type="C_INTEGER"><list>1</list><list>2</list></item></children></attributes>
<attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>normal_range</rm_attribute_name>` + requiredExistence + `
<children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>DV_INTERVAL&lt;DV_ORDINAL&gt;</rm_type_name><node_id></node_id>
<attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>lower</rm_attribute_name>` + requiredExistence + ordinalPairs("1|at0003", "2|at0004") + `</attributes>
<attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>upper</rm_attribute_name>` + requiredExistence + ordinalPairs("2|at0004", "1|at0003") + `</attributes>
</children></attributes>
</children></attributes>
</children>
<cardinality><is_ordered>false</is_ordered><is_unique>false</is_unique><interval><lower_included>true</lower_included><lower_unbounded>false</lower_unbounded><upper_unbounded>true</upper_unbounded><lower>1</lower></interval></cardinality>
</attributes></children></attributes></children></attributes>
<archetype_id><value>openEHR-EHR-COMPOSITION.ordinal.v1</value></archetype_id>
<term_definitions code="at0000"><items id="text">Report</items><items id="description">Report</items></term_definitions>
<term_definitions code="at0001"><items id="text">Tree</items><items id="description">Tree</items></term_definitions>
<term_definitions code="at0002"><items id="text">Severity</items><items id="description">Severity</items></term_definitions>
<term_definitions code="at0003"><items id="text">Mild</items><items id="description">Mild</items></term_definitions>
<term_definitions code="at0004"><items id="text">Severe</items><items id="description">Severe</items></term_definitions>
</definition>
</template>`

// ordinalPairs is a C_DV_ORDINAL that lists the given value|local-code
// pairs, in order.
func ordinalPairs(pairs ...string) string {
	var b strings.Builder
	b.WriteString(`<children xsi:type="C_DV_ORDINAL"><rm_type_name>DV_ORDINAL</rm_type_name><node_id></node_id>`)
	for _, p := range pairs {
		value, code, _ := strings.Cut(p, "|")
		b.WriteString(`<list><value>` + value + `</value><symbol><value></value><defining_code><terminology_id><value>local</value></terminology_id>` +
			`<code_string>` + code + `</code_string></defining_code></symbol></list>`)
	}
	b.WriteString(`</children>`)
	return b.String()
}

// localOrdinal is a DV_ORDINAL of value with the local code as its symbol.
func localOrdinal(value rm.Integer, code, text string) rm.DVOrdinal {
	return rm.DVOrdinal{Value: value, Symbol: rm.DVCodedText{
		Value: text, DefiningCode: rm.CodePhrase{CodeString: code, TerminologyID: rm.TerminologyID{Value: "local"}},
	}}
}

// TestREQ101_REQ107_OrdinalSymbolAndNormalRangeBuildAndSet checks that a
// COMPOSITION whose template constrains a DV_ORDINAL's symbol, value and
// normal_range as nodes of their own gets a Builder: the skeleton's
// ordinal takes the first code and the first value the template lists,
// and a normal_range held over DV_ORDERED, as the RM type holds it, whose
// bounds are the template's two pairs, held by pointer (REQ-107 Leaf
// values, Intervals); Set writes a symbol and a normal_range of the
// template's own DV_INTERVAL<DV_ORDINAL> at their own paths, Build keeps
// them, the range converted, and the composition passes
// ValidateComposition.
func TestREQ101_REQ107_OrdinalSymbolAndNormalRangeBuildAndSet(t *testing.T) {
	t.Parallel()
	c := compileInline(t, ordinalOPT)
	b, err := composition.NewBuilder(t.Context(), c,
		composition.WithTerritory("NL"),
		composition.WithComposer(testComposer()),
	)
	if err != nil {
		t.Fatalf("NewBuilder: %v, want a Builder", err)
	}
	ordinal := func(comp *rm.Composition) *rm.DVOrdinal {
		t.Helper()
		if comp.Context == nil {
			t.Fatal("COMPOSITION.context is unset")
		}
		tree, ok := comp.Context.OtherContext.(*rm.ItemTree)
		if !ok || len(tree.Items) != 1 {
			t.Fatalf("EVENT_CONTEXT.other_context = %#v, want an ITEM_TREE with one item", comp.Context.OtherContext)
		}
		el, ok := tree.Items[0].(*rm.Element)
		if !ok {
			t.Fatalf("other_context item is %T, want *rm.Element", tree.Items[0])
		}
		o, ok := el.Value.(*rm.DVOrdinal)
		if !ok {
			t.Fatalf("ELEMENT.value is %T, want *rm.DVOrdinal", el.Value)
		}
		return o
	}
	checkRange := func(t *testing.T, what string, iv *rm.DVInterval[rm.DVOrdered], lower, upper rm.DVOrdinal) {
		t.Helper()
		if iv == nil || iv.LowerUnbounded || iv.UpperUnbounded {
			t.Fatalf("%s normal_range = %+v, want both sides closed", what, iv)
		}
		lo, loOK := iv.Lower.(*rm.DVOrdinal)
		hi, hiOK := iv.Upper.(*rm.DVOrdinal)
		if !loOK || !hiOK {
			t.Fatalf("%s normal_range bounds are %T and %T, want *rm.DVOrdinal", what, iv.Lower, iv.Upper)
		}
		if lo.Value != lower.Value || lo.Symbol.DefiningCode != lower.Symbol.DefiningCode ||
			hi.Value != upper.Value || hi.Symbol.DefiningCode != upper.Symbol.DefiningCode {
			t.Errorf("%s normal_range = %d|%s .. %d|%s, want %d|%s .. %d|%s", what,
				lo.Value, lo.Symbol.DefiningCode.CodeString, hi.Value, hi.Symbol.DefiningCode.CodeString,
				lower.Value, lower.Symbol.DefiningCode.CodeString, upper.Value, upper.Symbol.DefiningCode.CodeString)
		}
	}

	skeleton, err := b.Build()
	if err != nil {
		t.Fatalf("Build of the bare skeleton: %v", err)
	}
	seeded := ordinal(skeleton)
	mild, severe := localOrdinal(1, "at0003", "Mild"), localOrdinal(2, "at0004", "Severe")
	if seeded.Value != 1 || seeded.Symbol.DefiningCode != mild.Symbol.DefiningCode {
		t.Errorf("skeleton DV_ORDINAL = %d|%s, want 1|at0003", seeded.Value, seeded.Symbol.DefiningCode.CodeString)
	}
	checkRange(t, "skeleton", seeded.NormalRange, mild, severe)

	const symbolPath, rangePath = "/context/other_context/items[at0002]/value/symbol", "/context/other_context/items[at0002]/value/normal_range"
	symbol := severe.Symbol
	set := &rm.DVInterval[rm.DVOrdinal]{}
	set.Lower, set.Upper = severe, severe
	set.LowerIncluded, set.UpperIncluded = true, true
	if err := b.Set(symbolPath, &symbol); err != nil {
		t.Fatalf("Set(%s) = %v, want nil", symbolPath, err)
	}
	if err := b.Set(rangePath, set); err != nil {
		t.Fatalf("Set(%s) = %v, want nil", rangePath, err)
	}
	comp, err := b.Build()
	if err != nil {
		t.Fatalf("Build after Set = %v, want nil", err)
	}
	built := ordinal(comp)
	if built.Symbol.DefiningCode != severe.Symbol.DefiningCode {
		t.Errorf("built DV_ORDINAL.symbol = %+v, want the %+v Set was given", built.Symbol.DefiningCode, severe.Symbol.DefiningCode)
	}
	checkRange(t, "built", built.NormalRange, severe, severe)
	if r := validation.ValidateComposition(comp, c); !r.OK {
		t.Errorf("ValidateComposition: %+v, want no issues", r.Issues)
	}
}
