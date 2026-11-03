package aom14_test

// REQ-176 § Shapes the pinned BMM lacks, and REQ-041 § SDK-authored root
// schemas, Observable: the AOM 1.4 types carry the classes and members the
// AOM 1.4 root declares in the AM Release-1.4 schemas' form, so a document
// written in that form decodes in every mode, strict included, with nothing
// dropped, and encodes back in that form.

import (
	"encoding/xml"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aom/aom14"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml"
)

const (
	nsOpenEHR = ` xmlns="http://schemas.openehr.org/v1" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"`
	localCode = `<terminology_id><value>local</value></terminology_id><code_string>at0001</code_string>`
)

// decodeModes are the three decode modes: the default, which skips unmatched
// content, one that reports it, and one that refuses it.
func decodeModes(reported *[]string) map[string][]canxml.DecoderOption {
	return map[string][]canxml.DecoderOption{
		"default": nil,
		"report":  {canxml.WithUnmatched(func(p string) { *reported = append(*reported, p) })},
		"strict":  {canxml.WithStrict()},
	}
}

// childNames returns the local names of the child elements of the root of doc.
func childNames(t *testing.T, doc []byte) []string {
	t.Helper()
	var root struct {
		Children []struct {
			XMLName xml.Name
		} `xml:",any"`
	}
	if err := xml.Unmarshal(doc, &root); err != nil {
		t.Fatalf("reading %s: %v", doc, err)
	}
	names := make([]string, len(root.Children))
	for i, c := range root.Children {
		names[i] = c.XMLName.Local
	}
	return names
}

// TestREQ176_REQ041_TypesCarryTheRootsNames pins that the type registry holds
// the classes the AOM 1.4 root declares under the AM Release-1.4 schemas'
// names, and neither the BMM's names for them nor a type alias; and that a
// document naming a class by the BMM's name in `xsi:type` names an unknown
// type and fails in every mode.
func TestREQ176_REQ041_TypesCarryTheRootsNames(t *testing.T) {
	for _, name := range []string{
		"OPERATIONAL_TEMPLATE", "C_ARCHETYPE_ROOT", "FLAT_ARCHETYPE_ONTOLOGY", "ANNOTATION", "T_VIEW",
		"T_VIEW_CONSTRAINT", "T_VIEW_CONSTRAINT_ITEM", "T_CONSTRAINT", "T_ATTRIBUTE", "T_COMPLEX_OBJECT",
		"C_CODE_REFERENCE", "C_CODE_PHRASE", "C_DV_ORDINAL", "C_DV_QUANTITY", "CodeDefinitionSet",
		"TermBindingSet", "TERM_BINDING_ITEM", "ConstraintBindingSet", "CONSTRAINT_BINDING_ITEM",
	} {
		if _, ok := typereg.Default.Lookup(name); !ok {
			t.Errorf("typereg.Default has no %s, which the AOM 1.4 root declares", name)
		}
	}
	for _, name := range []string{"C_CODED_TEXT", "C_ORDINAL", "C_QUANTITY", "T_ARCHETYPE_ROOT"} {
		if _, ok := typereg.Default.Lookup(name); ok {
			t.Errorf("typereg.Default holds %s, which the AOM 1.4 root leaves out or reads as an alias", name)
		}
	}

	for _, pinned := range []string{"C_CODED_TEXT", "C_ORDINAL", "C_QUANTITY"} {
		doc := `<c_single_attribute` + nsOpenEHR + `><rm_attribute_name>value</rm_attribute_name>` +
			`<existence><lower>1</lower><upper>1</upper></existence>` +
			`<children xsi:type="` + pinned + `"><rm_type_name>CODE_PHRASE</rm_type_name></children></c_single_attribute>`
		var reported []string
		for mode, opts := range decodeModes(&reported) {
			err := canxml.Unmarshal([]byte(doc), new(aom14.CSingleAttribute), opts...)
			if !errors.Is(err, typereg.ErrUnknownType) {
				t.Errorf("canxml.Unmarshal(xsi:type=%q, %s) = %v; want typereg.ErrUnknownType", pinned, mode, err)
			}
		}
	}
}

// TestREQ176_CodePhraseTakesTheSchemaForm verifies and closes the backlog lead
// on OpenehrProfile.xsd:10: C_CODE_PHRASE writes the elements the schema type
// declares, in its order, a TERMINOLOGY_ID and a typed assumed value among
// them, and no `reference`, which the schema does not declare; and a
// `reference` element in a document is unmatched content.
func TestREQ176_CodePhraseTakesTheSchemaForm(t *testing.T) {
	in := &aom14.CCodePhrase{
		RMTypeName:    "CODE_PHRASE",
		NodeID:        "at0002",
		Occurrences:   rm.Interval[aom14.Integer]{Lower: 1, Upper: 1, LowerIncluded: true, UpperIncluded: true},
		AssumedValue:  &rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "local"}, CodeString: "at0001"},
		TerminologyID: &rm.TerminologyID{Value: "local"},
		CodeList:      &[]string{"at0001", "at0002"},
	}
	b, err := canxml.Marshal(in)
	if err != nil {
		t.Fatalf("canxml.Marshal: %v", err)
	}
	want := []string{"rm_type_name", "occurrences", "node_id", "assumed_value", "terminology_id", "code_list", "code_list"}
	if got := childNames(t, b); !slices.Equal(got, want) {
		t.Errorf("canxml.Marshal wrote child elements %q in %s; want the schema's %q", got, b, want)
	}
	var back aom14.CCodePhrase
	if err := canxml.Unmarshal(b, &back, canxml.WithStrict()); err != nil {
		t.Fatalf("canxml.Unmarshal(%s, strict) = %v; want nil", b, err)
	}
	if !reflect.DeepEqual(&back, in) {
		t.Errorf("round trip = %+v; want %+v", back, *in)
	}

	doc := `<c_code_phrase` + nsOpenEHR + `><rm_type_name>CODE_PHRASE</rm_type_name><terminology_id><value>local</value></terminology_id><reference>ac0001</reference></c_code_phrase>`
	var reported []string
	if err := canxml.Unmarshal([]byte(doc), new(aom14.CCodePhrase), canxml.WithUnmatched(func(p string) { reported = append(reported, p) })); err != nil {
		t.Fatalf("canxml.Unmarshal(reference, report) = %v; want nil", err)
	}
	if want := []string{"/c_code_phrase[1]/reference[1]"}; !slices.Equal(reported, want) {
		t.Errorf("canxml.Unmarshal(reference, report) reported %q; want %q, the schema declares no reference", reported, want)
	}
}

// TestREQ176_QuantityItemAndOrdinalKeepTheSchemasMembers verifies and closes
// the backlog lead on OpenehrProfile.xsd:120: C_QUANTITY_ITEM.precision and
// ORDINAL's normal_range, other_reference_ranges and normal_status, the
// DvOrderedElements group, decode in every mode with nothing reported and
// encode back in the schema's order.
func TestREQ176_QuantityItemAndOrdinalKeepTheSchemasMembers(t *testing.T) {
	const interval = `<lower_included>true</lower_included><upper_included>true</upper_included>` +
		`<lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded>`
	quantity := `<c_quantity_item` + nsOpenEHR + `><magnitude>` + interval + `<lower>0.0</lower><upper>250.0</upper></magnitude>` +
		`<precision>` + interval + `<lower>1</lower><upper>1</upper></precision><units>mm</units></c_quantity_item>`
	ordinalValue := func(v string) string {
		return `<symbol><defining_code>` + localCode + `</defining_code></symbol><value>` + v + `</value>`
	}
	ordinal := `<ordinal` + nsOpenEHR + `>` +
		`<normal_range><lower xsi:type="DV_ORDINAL">` + ordinalValue("1") + `</lower><upper xsi:type="DV_ORDINAL">` + ordinalValue("2") + `</upper>` + interval + `</normal_range>` +
		`<other_reference_ranges><meaning><value>critical</value></meaning><range><lower xsi:type="DV_ORDINAL">` + ordinalValue("3") + `</lower>` +
		`<lower_included>true</lower_included><upper_included>false</upper_included><lower_unbounded>false</lower_unbounded><upper_unbounded>true</upper_unbounded></range></other_reference_ranges>` +
		`<normal_status><terminology_id><value>openehr</value></terminology_id><code_string>N</code_string></normal_status>` +
		`<value>1</value><symbol><defining_code>` + localCode + `</defining_code></symbol></ordinal>`

	var reported []string
	for mode, opts := range decodeModes(&reported) {
		var q aom14.CQuantityItem
		if err := canxml.Unmarshal([]byte(quantity), &q, opts...); err != nil {
			t.Fatalf("canxml.Unmarshal(C_QUANTITY_ITEM, %s) = %v; want nil", mode, err)
		}
		if q.Precision == nil || q.Precision.Lower != 1 || q.Precision.Upper != 1 || q.Magnitude == nil || q.Units != "mm" {
			t.Errorf("canxml.Unmarshal(C_QUANTITY_ITEM, %s) = %+v; want its magnitude, precision 1..1 and units mm", mode, q)
		}
		var o aom14.Ordinal
		if err := canxml.Unmarshal([]byte(ordinal), &o, opts...); err != nil {
			t.Fatalf("canxml.Unmarshal(ORDINAL, %s) = %v; want nil", mode, err)
		}
		if o.NormalRange == nil || o.NormalStatus == nil || o.NormalStatus.CodeString != "N" || len(o.OtherReferenceRanges) != 1 || o.Value != 1 {
			t.Errorf("canxml.Unmarshal(ORDINAL, %s) = %+v; want its normal range, its reference range, normal status N and value 1", mode, o)
		}
	}
	if len(reported) != 0 {
		t.Errorf("the reporting decodes reported %q; want nothing", reported)
	}

	for _, tc := range []struct {
		name string
		v    any
		want []string
	}{
		{"C_QUANTITY_ITEM", &aom14.CQuantityItem{Precision: &rm.Interval[aom14.Integer]{Lower: 0, LowerIncluded: true, UpperUnbounded: true}, Units: "mm"}, []string{"precision", "units"}},
		{"ORDINAL", &aom14.Ordinal{NormalStatus: &rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "openehr"}, CodeString: "N"}, Value: 1, Symbol: &rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "local"}, CodeString: "at0001"}}, []string{"normal_status", "value", "symbol"}},
	} {
		b, err := canxml.Marshal(tc.v)
		if err != nil {
			t.Fatalf("canxml.Marshal(%s): %v", tc.name, err)
		}
		if got := childNames(t, b); !slices.Equal(got, tc.want) {
			t.Errorf("canxml.Marshal(%s) wrote child elements %q in %s; want %q", tc.name, got, b, tc.want)
		}
	}
}

// TestREQ176_OrdinalSymbolTakesEitherShape pins the schema form of
// ORDINAL.symbol: a symbol in the DV_CODED_TEXT shape decodes as an
// *rm.DVCodedText and one in the CODE_PHRASE shape as an *rm.CodePhrase, in
// every mode with nothing reported; each encodes back in its own shape, with
// no `xsi:type`, which the schema refuses on the element.
func TestREQ176_OrdinalSymbolTakesEitherShape(t *testing.T) {
	for _, tc := range []struct {
		name, symbol string
		want         any
	}{
		{
			"DV_CODED_TEXT", `<value/><defining_code>` + localCode + `</defining_code>`,
			&rm.DVCodedText{DefiningCode: rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "local"}, CodeString: "at0001"}},
		},
		{
			"DV_CODED_TEXT without its value", `<defining_code>` + localCode + `</defining_code>`,
			&rm.DVCodedText{DefiningCode: rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "local"}, CodeString: "at0001"}},
		},
		{
			"CODE_PHRASE", localCode,
			&rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "local"}, CodeString: "at0001"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc := `<ordinal` + nsOpenEHR + `><value>0</value><symbol>` + tc.symbol + `</symbol></ordinal>`
			var reported []string
			for mode, opts := range decodeModes(&reported) {
				var o aom14.Ordinal
				if err := canxml.Unmarshal([]byte(doc), &o, opts...); err != nil {
					t.Fatalf("canxml.Unmarshal(%s) = %v; want nil", mode, err)
				}
				if !reflect.DeepEqual(o.Symbol, tc.want) {
					t.Errorf("canxml.Unmarshal(%s): Symbol = %#v; want %#v", mode, o.Symbol, tc.want)
				}
			}
			if len(reported) != 0 {
				t.Errorf("the reporting decode reported %q; want nothing", reported)
			}
			b, err := canxml.Marshal(&aom14.Ordinal{Value: 0, Symbol: tc.want})
			if err != nil {
				t.Fatalf("canxml.Marshal: %v", err)
			}
			if strings.Contains(string(b), "xsi:type") || !strings.Contains(string(b), "<symbol>") {
				t.Errorf("canxml.Marshal wrote %s; want a symbol element with no xsi:type", b)
			}
			var back aom14.Ordinal
			if err := canxml.Unmarshal(b, &back, canxml.WithStrict()); err != nil || !reflect.DeepEqual(back.Symbol, tc.want) {
				t.Errorf("round trip of %s = %#v, %v; want %#v", b, back.Symbol, err, tc.want)
			}
		})
	}
	if _, err := canxml.Marshal(&aom14.Ordinal{Symbol: "at0001"}); err == nil {
		t.Error("canxml.Marshal(ORDINAL with a string symbol) = nil error; want a refusal, a string is none of the symbol's shapes")
	}
}

// TestREQ176_MatchNegatedIsDropped pins REQ-176's match_negated: on a
// C_SINGLE_ATTRIBUTE and a C_MULTIPLE_ATTRIBUTE, before rm_attribute_name or
// after existence, the value false, written `false` or `0`, is read and
// dropped in every mode and never written; any other text is refused in every
// mode with canxml.ErrInvalidShape.
func TestREQ176_MatchNegatedIsDropped(t *testing.T) {
	const existence = `<existence><lower>1</lower><upper>1</upper></existence>`
	for _, tc := range []struct {
		root, extra string
		into        func() any
	}{
		{"c_single_attribute", "", func() any { return new(aom14.CSingleAttribute) }},
		{"c_multiple_attribute", `<cardinality><is_ordered>false</is_ordered><is_unique>false</is_unique><interval><lower>0</lower></interval></cardinality>`, func() any { return new(aom14.CMultipleAttribute) }},
	} {
		docs := func(text string) []string {
			before := `<` + tc.root + nsOpenEHR + `><match_negated>` + text + `</match_negated><rm_attribute_name>items</rm_attribute_name>` + existence + tc.extra + `</` + tc.root + `>`
			after := `<` + tc.root + nsOpenEHR + `><rm_attribute_name>items</rm_attribute_name>` + existence + `<match_negated>` + text + `</match_negated>` + tc.extra + `</` + tc.root + `>`
			return []string{before, after}
		}
		for _, text := range []string{"false", "0", " false "} {
			for _, doc := range docs(text) {
				var reported []string
				for mode, opts := range decodeModes(&reported) {
					v := tc.into()
					if err := canxml.Unmarshal([]byte(doc), v, opts...); err != nil {
						t.Errorf("canxml.Unmarshal(%s, %s) = %v; want nil, match_negated dropped", doc, mode, err)
						continue
					}
					b, err := canxml.Marshal(v)
					if err != nil || strings.Contains(string(b), "match_negated") {
						t.Errorf("canxml.Marshal(decoded %s) = %s, %v; want no match_negated", tc.root, b, err)
					}
				}
				if len(reported) != 0 {
					t.Errorf("canxml.Unmarshal(%s, report) reported %q; want nothing", doc, reported)
				}
			}
		}
		for _, text := range []string{"true", "1", ""} {
			for _, doc := range docs(text) {
				var reported []string
				for mode, opts := range decodeModes(&reported) {
					if err := canxml.Unmarshal([]byte(doc), tc.into(), opts...); !errors.Is(err, canxml.ErrInvalidShape) {
						t.Errorf("canxml.Unmarshal(%s, %s) = %v; want canxml.ErrInvalidShape", doc, mode, err)
					}
				}
			}
		}
	}
}

// TestREQ176_SchemaAttributesRoundTrip pins the XML attributes the AM
// Release-1.4 schemas declare on the operational template and ontology
// classes: each decodes from its attribute in strict mode and encodes back as
// one, T_VIEW's anonymous constraint and item types included. The ontology
// holds no ARCHETYPE_TERM: its items are a Hash, whose canonical XML is not
// this test's.
func TestREQ176_SchemaAttributesRoundTrip(t *testing.T) {
	for _, tc := range []struct {
		name string
		doc  string
		into func() any
	}{
		{
			"T_VIEW", `<t_view` + nsOpenEHR + `><constraints path="/content[at0001]"><items id="pass_through"><value>true</value></items></constraints></t_view>`,
			func() any { return new(aom14.TView) },
		},
		{
			"FLAT_ARCHETYPE_ONTOLOGY", `<flat_archetype_ontology` + nsOpenEHR + ` archetype_id="openEHR-EHR-OBSERVATION.x.v1">` +
				`<term_definitions language="en"></term_definitions>` +
				`<term_bindings terminology="SNOMED-CT"><items code="at0000"><value><terminology_id><value>SNOMED-CT</value></terminology_id><code_string>1</code_string></value></items></term_bindings>` +
				`<constraint_bindings terminology="SNOMED-CT"><items code="ac0001"><value>terminology:SNOMED-CT?subset=x</value></items></constraint_bindings>` +
				`</flat_archetype_ontology>`,
			func() any { return new(aom14.FlatArchetypeOntology) },
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := tc.into()
			if err := canxml.Unmarshal([]byte(tc.doc), v, canxml.WithStrict()); err != nil {
				t.Fatalf("canxml.Unmarshal(%s, strict) = %v; want nil", tc.doc, err)
			}
			b, err := canxml.Marshal(v)
			if err != nil {
				t.Fatalf("canxml.Marshal: %v", err)
			}
			for _, attr := range []string{`path="/content[at0001]"`, `id="pass_through"`, `archetype_id="openEHR-EHR-OBSERVATION.x.v1"`, `language="en"`, `code="at0000"`, `terminology="SNOMED-CT"`, `code="ac0001"`} {
				if strings.Contains(tc.doc, attr) && !strings.Contains(string(b), attr) {
					t.Errorf("canxml.Marshal wrote %s; want the attribute %s", b, attr)
				}
			}
			back := tc.into()
			if err := canxml.Unmarshal(b, back, canxml.WithStrict()); err != nil || !reflect.DeepEqual(back, v) {
				t.Errorf("round trip of %s = %+v, %v; want %+v", b, back, err, v)
			}
		})
	}
}
