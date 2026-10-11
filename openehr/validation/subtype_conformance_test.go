package validation_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// REQ-102 (Validation dimensions, Issue codes, Items that are not a
// LOCATABLE) and REQ-110 (Implementation) — an RM value satisfies an OPT
// child when its type equals the child's RMTypeName or descends from it in
// the pinned BMM, whether that ancestor is abstract or concrete. A child of
// the value's own type binds before a child the type only conforms to, on a
// single-valued attribute and for an item of a multi-valued attribute that is
// not a LOCATABLE. The tests build small synthetic OPTs so each case pins one
// rule.

// elementValueOPT returns an ELEMENT template whose value attribute holds
// children, in that order.
func elementValueOPT(children ...string) string {
	return `<?xml version="1.0"?>
<template xmlns="http://schemas.openehr.org/v1" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <template_id><value>element-value</value></template_id>
  <concept>element-value</concept>
  <language><terminology_id><value>ISO_639-1</value></terminology_id><code_string>en</code_string></language>
  <definition>
    <rm_type_name>ELEMENT</rm_type_name>
    <node_id>at0000</node_id>
    <attributes xsi:type="C_SINGLE_ATTRIBUTE">
      <rm_attribute_name>value</rm_attribute_name>` + strings.Join(children, "") + `
    </attributes>
    <archetype_id><value>openEHR-EHR-ELEMENT.element_value.v1</value></archetype_id>
  </definition>
</template>`
}

// OPT children shared by the element value and ACTOR.languages templates.
const (
	dvTextChild = `
      <children xsi:type="C_COMPLEX_OBJECT">
        <rm_type_name>DV_TEXT</rm_type_name>
        <node_id></node_id>
      </children>`
	// dvCodedTextChild admits a DV_CODED_TEXT coded as local at0001 or at0002.
	dvCodedTextChild = `
      <children xsi:type="C_COMPLEX_OBJECT">
        <rm_type_name>DV_CODED_TEXT</rm_type_name>
        <node_id></node_id>
        <attributes xsi:type="C_SINGLE_ATTRIBUTE">
          <rm_attribute_name>defining_code</rm_attribute_name>
          <children xsi:type="C_CODE_PHRASE">
            <rm_type_name>CODE_PHRASE</rm_type_name>
            <node_id></node_id>
            <terminology_id><value>local</value></terminology_id>
            <code_list>at0001</code_list>
            <code_list>at0002</code_list>
          </children>
        </attributes>
      </children>`
	dvCountChild = `
      <children xsi:type="C_COMPLEX_OBJECT">
        <rm_type_name>DV_COUNT</rm_type_name>
        <node_id></node_id>
      </children>`
	// durationChild is an AOM 1.4 primitive short name in place of the
	// DV_DURATION it stands for; its C_DURATION admits any ISO 8601
	// duration.
	durationChild = `
      <children xsi:type="C_PRIMITIVE_OBJECT">
        <rm_type_name>DURATION</rm_type_name>
        <node_id></node_id>
        <item xsi:type="C_DURATION"></item>
      </children>`
	// integerChild is the INTEGER short name in place of the DV_COUNT it
	// stands for; its C_INTEGER admits 10 to 20.
	integerChild = `
      <children xsi:type="C_PRIMITIVE_OBJECT">
        <rm_type_name>INTEGER</rm_type_name>
        <node_id></node_id>
        <item xsi:type="C_INTEGER">
          <range>
            <lower_included>true</lower_included>
            <upper_included>true</upper_included>
            <lower_unbounded>false</lower_unbounded>
            <upper_unbounded>false</upper_unbounded>
            <lower>10</lower>
            <upper>20</upper>
          </range>
        </item>
      </children>`
)

// valueElement returns an ELEMENT that elementValueOPT's root matches,
// holding v.
func valueElement(v rm.DataValue) *rm.Element {
	return &rm.Element{
		ArchetypeNodeID: "openEHR-EHR-ELEMENT.element_value.v1",
		Name:            rm.DVText{Value: "element"},
		Value:           v,
	}
}

// localCoded returns a DV_CODED_TEXT coded as code in the local
// terminology.
func localCoded(code string) rm.DVCodedText {
	return rm.DVCodedText{
		Value:        "coded",
		DefiningCode: rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "local"}, CodeString: code},
	}
}

// REQ-102 — a DV_CODED_TEXT descends from DV_TEXT, so an ACTOR.languages
// item coded as one binds to a DV_TEXT child (REQ-110: a demographic root).
// It used to be refused as rm_type_mismatch.
func TestValidateDemographic_REQ102_CodedTextLanguageUnderTextChild(t *testing.T) {
	t.Parallel()
	c := mustCompileInline(t, actorOPT(languagesChild("DV_TEXT")))
	p := validActor()
	p.Languages = []rm.DVTextLike{codedText("English", "en")}

	r := validation.ValidateDemographic(p, c)
	if got := issueKeys(r.Issues); len(got) != 0 {
		t.Errorf("ValidateDemographic(DV_CODED_TEXT language under a DV_TEXT child) issues = %q, want none", got)
	}
}

// REQ-102 — a PARTY_REF or a LOCATABLE_REF descends from OBJECT_REF, so a
// FOLDER.items reference of either type binds to an OBJECT_REF child and is
// walked there (REQ-110: an EHR-IM root). A PARTY_REF used to be refused as
// rm_type_mismatch. Going the other way, an OBJECT_REF under a PARTY_REF
// child, or a sibling under a LOCATABLE_REF child, does not conform.
func TestValidateFolder_REQ102_ReferenceSubtypeUnderObjectRefChild(t *testing.T) {
	t.Parallel()
	partyRef := func() rm.PartyRef {
		return rm.PartyRef{
			ID:        &rm.HierObjectID{Value: "a3e8b1c2-0d4f-4e5a-9b6c-7d8e9f0a1b2c"},
			Namespace: "demographic",
			Type:      "PERSON",
		}
	}
	noNamespace := partyRef()
	noNamespace.Namespace = ""
	locatableRef := rm.LocatableRef{
		ID:        &rm.HierObjectID{Value: "6f1d3a52-9c0e-4b7a-8f21-3d5e7a9b1c04"},
		Namespace: "local",
		Type:      "COMPOSITION",
		Path:      new("/content[openEHR-EHR-OBSERVATION.blood_pressure.v1]"),
	}
	objectRef := &rm.ObjectRef{
		ID:        &rm.HierObjectID{Value: "6f1d3a52-9c0e-4b7a-8f21-3d5e7a9b1c04"},
		Namespace: "local",
		Type:      "COMPOSITION",
	}

	tests := []struct {
		name     string
		itemType string
		item     rm.ObjectRefLike
		want     []string
	}{
		{name: "PARTY_REF under OBJECT_REF", itemType: "OBJECT_REF", item: partyRef(), want: []string{}},
		{name: "LOCATABLE_REF under OBJECT_REF", itemType: "OBJECT_REF", item: locatableRef, want: []string{}},
		{
			name:     "bound PARTY_REF is walked",
			itemType: "OBJECT_REF",
			item:     noNamespace,
			want:     []string{"required /items[@1]/namespace"},
		},
		{
			name:     "OBJECT_REF under PARTY_REF",
			itemType: "PARTY_REF",
			item:     objectRef,
			want:     []string{"rm_type_mismatch /items[@1]"},
		},
		{
			name:     "PARTY_REF under LOCATABLE_REF",
			itemType: "LOCATABLE_REF",
			item:     partyRef(),
			want:     []string{"rm_type_mismatch /items[@1]"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := mustCompileInline(t, folderOPT(tc.itemType))
			folder := &rm.Folder{
				ArchetypeNodeID: "openEHR-EHR-FOLDER.generic.v1",
				Name:            rm.DVText{Value: "root"},
				Items:           []rm.ObjectRefLike{tc.item},
			}
			r := validation.ValidateFolder(folder, c)
			if got := issueKeys(r.Issues); !slices.Equal(got, tc.want) {
				t.Errorf("ValidateFolder(%s item under %s child) issues = %q, want %q", describeItem(tc.item), tc.itemType, got, tc.want)
			}
		})
	}
}

// describeItem names a test reference's RM type for a failure message.
func describeItem(v rm.ObjectRefLike) string {
	if name, ok := rm.RMTypeName(v); ok {
		return name
	}
	return "unknown"
}

// REQ-102 — with alternatives [DV_TEXT, DV_CODED_TEXT], a DV_CODED_TEXT
// conforms to both, and binds to the DV_CODED_TEXT child in either order:
// a code outside its list is reported there, which only that child can
// report. A DV_TEXT binds to the DV_TEXT child, the only one it conforms to.
func TestValidate_REQ102_ExactTypeAlternativeBindsFirst(t *testing.T) {
	t.Parallel()
	orders := []struct {
		name     string
		children []string
	}{
		{name: "DV_TEXT first", children: []string{dvTextChild, dvCodedTextChild}},
		{name: "DV_CODED_TEXT first", children: []string{dvCodedTextChild, dvTextChild}},
	}
	values := []struct {
		name  string
		value rm.DataValue
		want  []string
	}{
		{name: "code outside the list", value: localCoded("at9999"), want: []string{"primitive_not_in_list /value/defining_code"}},
		{name: "code in the list", value: localCoded("at0001"), want: []string{}},
		{name: "plain text", value: rm.DVText{Value: "text"}, want: []string{}},
	}
	for _, order := range orders {
		c := mustCompileInline(t, elementValueOPT(order.children...))
		for _, v := range values {
			t.Run(order.name+"/"+v.name, func(t *testing.T) {
				t.Parallel()
				r := validation.Validate(valueElement(v.value), c)
				if got := issueKeys(r.Issues); !slices.Equal(got, v.want) {
					t.Errorf("Validate(ELEMENT with %s value) issues = %q, want %q", v.name, got, v.want)
				}
			})
		}
	}
}

// REQ-102 — an ACTOR.languages item is not a LOCATABLE, and binds by the
// same rule as a single-valued attribute: with children [DV_TEXT,
// DV_CODED_TEXT] in either order, a DV_CODED_TEXT item binds to the
// DV_CODED_TEXT child, whose code list then reports a code outside it.
func TestValidateDemographic_REQ102_ExactTypeItemChildBindsFirst(t *testing.T) {
	t.Parallel()
	orders := []struct {
		name      string
		languages string
	}{
		{name: "DV_TEXT first", languages: dvTextChild + dvCodedTextChild},
		{name: "DV_CODED_TEXT first", languages: dvCodedTextChild + dvTextChild},
	}
	items := []struct {
		name string
		item rm.DVTextLike
		want []string
	}{
		{name: "code outside the list", item: localCoded("at9999"), want: []string{"primitive_not_in_list /languages[@1]/defining_code"}},
		{name: "code in the list", item: localCoded("at0002"), want: []string{}},
		{name: "plain text", item: rm.DVText{Value: "en"}, want: []string{}},
	}
	for _, order := range orders {
		c := mustCompileInline(t, actorOPT(order.languages))
		for _, it := range items {
			t.Run(order.name+"/"+it.name, func(t *testing.T) {
				t.Parallel()
				p := validActor()
				p.Languages = []rm.DVTextLike{it.item}
				r := validation.ValidateDemographic(p, c)
				if got := issueKeys(r.Issues); !slices.Equal(got, it.want) {
					t.Errorf("ValidateDemographic(%s language) issues = %q, want %q", it.name, got, it.want)
				}
			})
		}
	}
}

// REQ-102 — subtype admission runs one way only. A type that neither equals
// a child's RMTypeName nor descends from it is refused: as rm_type_mismatch
// under one child, as alternative_mismatch under two or more. A supertype
// under a subtype child (DV_TEXT under DV_CODED_TEXT) is such a type.
func TestValidate_REQ102_NonConformingTypeStillFails(t *testing.T) {
	t.Parallel()
	quantity := &rm.DVQuantity{Magnitude: 1, Units: "kg"}
	tests := []struct {
		name     string
		children []string
		value    rm.DataValue
		want     []string
	}{
		{
			name:     "DV_QUANTITY under DV_TEXT",
			children: []string{dvTextChild},
			value:    quantity,
			want:     []string{"rm_type_mismatch /value"},
		},
		{
			name:     "DV_QUANTITY under DV_TEXT or DV_CODED_TEXT",
			children: []string{dvTextChild, dvCodedTextChild},
			value:    quantity,
			want:     []string{"alternative_mismatch /value"},
		},
		{
			name:     "DV_TEXT under DV_CODED_TEXT",
			children: []string{dvCodedTextChild},
			value:    rm.DVText{Value: "text"},
			want:     []string{"rm_type_mismatch /value"},
		},
		{
			name:     "DV_TEXT under DV_CODED_TEXT or DV_COUNT",
			children: []string{dvCodedTextChild, dvCountChild},
			value:    rm.DVText{Value: "text"},
			want:     []string{"alternative_mismatch /value"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := mustCompileInline(t, elementValueOPT(tc.children...))
			r := validation.Validate(valueElement(tc.value), c)
			if got := issueKeys(r.Issues); !slices.Equal(got, tc.want) {
				t.Errorf("Validate(%s) issues = %q, want %q", tc.name, got, tc.want)
			}
		})
	}
}

// REQ-102 — an AOM 1.4 primitive short name is no BMM class, so the
// hierarchy cannot answer for it. DURATION still admits its canonical
// DV_DURATION wrapper and INTEGER its DV_COUNT, the types the generator
// builds for them, and the short name's constraint then checks the
// wrapper's value. Each still refuses any other type, the wrapper of
// another short name included.
func TestValidate_REQ102_AOMShortNameAdmitsItsWrapper(t *testing.T) {
	t.Parallel()
	duration := mustCompileInline(t, elementValueOPT(durationChild))
	integer := mustCompileInline(t, elementValueOPT(integerChild))
	tests := []struct {
		name  string
		child string
		value rm.DataValue
		want  []string
	}{
		{name: "DV_DURATION", child: "DURATION", value: rm.DVDuration{Value: "PT5M"}, want: []string{}},
		{name: "DV_DURATION with no duration", child: "DURATION", value: rm.DVDuration{Value: "five minutes"}, want: []string{"primitive_invalid_value /value"}},
		{name: "DV_TEXT", child: "DURATION", value: rm.DVText{Value: "PT5M"}, want: []string{"rm_type_mismatch /value"}},
		{name: "DV_DATE", child: "DURATION", value: rm.DVDate{Value: "2026-10-11"}, want: []string{"rm_type_mismatch /value"}},
		{name: "DV_COUNT", child: "INTEGER", value: rm.DVCount{Magnitude: 15}, want: []string{}},
		{name: "DV_COUNT out of range", child: "INTEGER", value: rm.DVCount{Magnitude: 25}, want: []string{"primitive_out_of_range /value"}},
		{name: "DV_QUANTITY", child: "INTEGER", value: &rm.DVQuantity{Magnitude: 15, Units: "1"}, want: []string{"rm_type_mismatch /value"}},
		{name: "DV_BOOLEAN", child: "INTEGER", value: rm.DVBoolean{Value: true}, want: []string{"rm_type_mismatch /value"}},
	}
	for _, tc := range tests {
		t.Run(tc.child+"/"+tc.name, func(t *testing.T) {
			t.Parallel()
			c := duration
			if tc.child == "INTEGER" {
				c = integer
			}
			r := validation.Validate(valueElement(tc.value), c)
			if got := issueKeys(r.Issues); !slices.Equal(got, tc.want) {
				t.Errorf("Validate(%s under %s) issues = %q, want %q", tc.name, tc.child, got, tc.want)
			}
		})
	}
}
