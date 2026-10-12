package validation_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// REQ-102 (Validation dimensions, Issue codes, Items that are not a
// LOCATABLE) and REQ-110 (Implementation) — an RM value satisfies an OPT
// child when its type equals the child's RMTypeName or descends from it in
// the pinned BMM, whether that ancestor is abstract or concrete. The value
// binds to the most specific child, in three tiers: a child of its own type,
// then the AOM 1.4 short name it wraps, then a child it conforms to or an
// untyped one. The rule holds on a single-valued attribute and for an item of
// a multi-valued attribute that is not a LOCATABLE. The tests build small
// synthetic OPTs so each case pins one rule.

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
			// The template walk reports the absent namespace, and the RM
			// floor its own OBJECT_REF rule under another code.
			want: []string{"required /items[1]/namespace", "rm_invariant /items[1]/namespace"},
		},
		{
			name:     "OBJECT_REF under PARTY_REF",
			itemType: "PARTY_REF",
			item:     objectRef,
			want:     []string{"rm_type_mismatch /items[1]"},
		},
		{
			name:     "PARTY_REF under LOCATABLE_REF",
			itemType: "LOCATABLE_REF",
			item:     partyRef(),
			want:     []string{"rm_type_mismatch /items[1]"},
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
		{name: "code outside the list", item: localCoded("at9999"), want: []string{"primitive_not_in_list /languages[1]/defining_code"}},
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

// shortNameChild renders an AOM 1.4 primitive short name child whose
// item is a constraint of itemType carrying body.
func shortNameChild(shortName, itemType, body string) string {
	return `
      <children xsi:type="C_PRIMITIVE_OBJECT">
        <rm_type_name>` + shortName + `</rm_type_name>
        <node_id></node_id>
        <item xsi:type="` + itemType + `">` + body + `</item>
      </children>`
}

// REQ-102 — an AOM 1.4 primitive short name is no BMM class, so the
// hierarchy cannot answer for it: each of the six admits the DV wrapper the
// generator builds for it (REQ-107), and refuses any other type, the wrapper
// of another short name included. The generator builds each value, so a
// wrapper the validator and the generator disagree on fails here.
func TestValidate_REQ102_AOMShortNameAdmitsItsWrapper(t *testing.T) {
	t.Parallel()
	tests := []struct {
		short   string
		child   string
		wrapper string
		refused rm.DataValue
	}{
		{short: "DURATION", child: durationChild, wrapper: "DV_DURATION", refused: rm.DVDate{Value: "2026-10-11"}},
		{short: "DATE", child: shortNameChild("DATE", "C_DATE", ""), wrapper: "DV_DATE", refused: rm.DVDateTime{Value: "2026-10-11T09:00:00Z"}},
		{short: "TIME", child: shortNameChild("TIME", "C_TIME", ""), wrapper: "DV_TIME", refused: rm.DVDuration{Value: "PT5M"}},
		{short: "DATE_TIME", child: shortNameChild("DATE_TIME", "C_DATE_TIME", ""), wrapper: "DV_DATE_TIME", refused: rm.DVDate{Value: "2026-10-11"}},
		{
			short:   "BOOLEAN",
			child:   shortNameChild("BOOLEAN", "C_BOOLEAN", "<true_valid>true</true_valid><false_valid>true</false_valid>"),
			wrapper: "DV_BOOLEAN",
			refused: rm.DVCount{Magnitude: 1},
		},
		{short: "INTEGER", child: integerChild, wrapper: "DV_COUNT", refused: &rm.DVQuantity{Magnitude: 15, Units: "1"}},
	}
	for _, tc := range tests {
		t.Run(tc.short, func(t *testing.T) {
			t.Parallel()
			c := mustCompileInline(t, elementValueOPT(tc.child))
			out, err := instance.Generate(t.Context(), c, instance.Options{Policy: instance.Example})
			if err != nil {
				t.Fatalf("Generate(ELEMENT value %s): %v", tc.short, err)
			}
			el, ok := out.(*rm.Element)
			if !ok {
				t.Fatalf("Generate root = %T, want *rm.Element", out)
			}
			if got, _ := rm.RMTypeName(el.Value); got != tc.wrapper {
				t.Fatalf("Generate built ELEMENT.value of type %q under %s, want %s", got, tc.short, tc.wrapper)
			}
			if got := issueKeys(validation.Validate(el, c).Issues); len(got) != 0 {
				t.Errorf("Validate(generated %s under %s) issues = %q, want none", tc.wrapper, tc.short, got)
			}
			refusedType, _ := rm.RMTypeName(tc.refused)
			want := []string{"rm_type_mismatch /value"}
			if got := issueKeys(validation.Validate(valueElement(tc.refused), c).Issues); !slices.Equal(got, want) {
				t.Errorf("Validate(%s under %s) issues = %q, want %q", refusedType, tc.short, got, want)
			}
		})
	}
}

// OPT children for the binding-tier cases.
const (
	dataValueChild = `
      <children xsi:type="C_COMPLEX_OBJECT">
        <rm_type_name>DATA_VALUE</rm_type_name>
        <node_id></node_id>
      </children>`
	untypedChild = `
      <children xsi:type="C_COMPLEX_OBJECT">
        <rm_type_name></rm_type_name>
        <node_id></node_id>
      </children>`
	bareIntervalChild = `
      <children xsi:type="C_COMPLEX_OBJECT">
        <rm_type_name>DV_INTERVAL</rm_type_name>
        <node_id></node_id>
      </children>`
	// countIntervalChild admits a DV_INTERVAL<DV_COUNT> whose upper
	// bound is 0 to 10.
	countIntervalChild = `
      <children xsi:type="C_COMPLEX_OBJECT">
        <rm_type_name>DV_INTERVAL&lt;DV_COUNT&gt;</rm_type_name>
        <node_id></node_id>
        <attributes xsi:type="C_SINGLE_ATTRIBUTE">
          <rm_attribute_name>upper</rm_attribute_name>
          <children xsi:type="C_COMPLEX_OBJECT">
            <rm_type_name>DV_COUNT</rm_type_name>
            <node_id></node_id>
            <attributes xsi:type="C_SINGLE_ATTRIBUTE">
              <rm_attribute_name>magnitude</rm_attribute_name>
              <children xsi:type="C_PRIMITIVE_OBJECT">
                <rm_type_name>INTEGER</rm_type_name>
                <node_id></node_id>
                <item xsi:type="C_INTEGER">
                  <range>
                    <lower_included>true</lower_included><upper_included>true</upper_included>
                    <lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded>
                    <lower>0</lower><upper>10</upper>
                  </range>
                </item>
              </children>
            </attributes>
          </children>
        </attributes>
      </children>`
	// countIntervalUpperRequiredChild admits a DV_INTERVAL<DV_COUNT> whose
	// upper bound must be present.
	countIntervalUpperRequiredChild = `
      <children xsi:type="C_COMPLEX_OBJECT">
        <rm_type_name>DV_INTERVAL&lt;DV_COUNT&gt;</rm_type_name>
        <node_id></node_id>
        <attributes xsi:type="C_SINGLE_ATTRIBUTE">
          <rm_attribute_name>upper</rm_attribute_name>
          <existence>
            <lower_included>true</lower_included><upper_included>true</upper_included>
            <lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded>
            <lower>1</lower><upper>1</upper>
          </existence>
          <children xsi:type="C_COMPLEX_OBJECT">
            <rm_type_name>DV_COUNT</rm_type_name>
            <node_id></node_id>
          </children>
        </attributes>
      </children>`
	// countChild admits a DV_COUNT of 0 to 5.
	countChild = `
      <children xsi:type="C_COMPLEX_OBJECT">
        <rm_type_name>DV_COUNT</rm_type_name>
        <node_id></node_id>
        <attributes xsi:type="C_SINGLE_ATTRIBUTE">
          <rm_attribute_name>magnitude</rm_attribute_name>
          <children xsi:type="C_PRIMITIVE_OBJECT">
            <rm_type_name>INTEGER</rm_type_name>
            <node_id></node_id>
            <item xsi:type="C_INTEGER">
              <range>
                <lower_included>true</lower_included><upper_included>true</upper_included>
                <lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded>
                <lower>0</lower><upper>5</upper>
              </range>
            </item>
          </children>
        </attributes>
      </children>`
)

// REQ-102 — binding prefers the most specific child, in three tiers, and
// the order of the alternatives does not change which child binds. Each
// case's value conforms to both alternatives, and only the tighter one
// reports the violation the value carries, so the issue shows which child
// bound it:
//   - a DV_INTERVAL<DV_COUNT> names the typed child, not the bare
//     DV_INTERVAL it conforms to;
//   - a round-tripped interval that decoded as the bare DV_INTERVAL takes
//     the type its DV_COUNT bounds give, so it names the typed child too;
//   - a DV_COUNT names the DV_COUNT child before the INTEGER short name it
//     wraps;
//   - a DV_DURATION is the wrapper of DURATION, a tighter child than the
//     DATA_VALUE it conforms to;
//   - a DV_CODED_TEXT names the DV_CODED_TEXT child before an untyped one.
func TestValidate_REQ102_AlternativeTiers(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		tight string
		loose string
		value rm.DataValue
		want  []string
	}{
		{
			name:  "typed interval",
			tight: countIntervalChild,
			loose: bareIntervalChild,
			value: &rm.DVInterval[rm.DVCount]{Lower: rm.DVCount{Magnitude: 1}, Upper: rm.DVCount{Magnitude: 50}},
			want:  []string{"primitive_out_of_range /value/upper/magnitude"},
		},
		{
			name:  "round-tripped interval",
			tight: countIntervalChild,
			loose: bareIntervalChild,
			value: &rm.DVInterval[rm.DVOrdered]{Lower: &rm.DVCount{Magnitude: 1}, Upper: &rm.DVCount{Magnitude: 50}},
			want:  []string{"primitive_out_of_range /value/upper/magnitude"},
		},
		{
			name:  "DV_COUNT and INTEGER",
			tight: countChild,
			loose: integerChild,
			value: rm.DVCount{Magnitude: 15},
			want:  []string{"primitive_out_of_range /value/magnitude"},
		},
		{
			name:  "DURATION and DATA_VALUE",
			tight: durationChild,
			loose: dataValueChild,
			value: rm.DVDuration{Value: "five minutes"},
			// The DURATION child's C_DURATION refuses the text, and the RM
			// floor's ISO 8601 rule refuses it under its own code.
			want: []string{"primitive_invalid_value /value", "rm_invariant /value"},
		},
		{
			name:  "DV_CODED_TEXT and untyped",
			tight: dvCodedTextChild,
			loose: untypedChild,
			value: localCoded("at9999"),
			want:  []string{"primitive_not_in_list /value/defining_code"},
		},
	}
	for _, tc := range tests {
		for _, order := range []struct {
			name     string
			children []string
		}{
			{name: "tight first", children: []string{tc.tight, tc.loose}},
			{name: "loose first", children: []string{tc.loose, tc.tight}},
		} {
			t.Run(tc.name+"/"+order.name, func(t *testing.T) {
				t.Parallel()
				c := mustCompileInline(t, elementValueOPT(order.children...))
				r := validation.Validate(valueElement(tc.value), c)
				if got := issueKeys(r.Issues); !slices.Equal(got, tc.want) {
					t.Errorf("Validate(%s, %s) issues = %q, want %q", tc.name, order.name, got, tc.want)
				}
			})
		}
	}
}

// REQ-102, REQ-110 — a round-tripped bare DV_INTERVAL with no bound present
// has no parameter, so only a bare DV_INTERVAL child names its type. Under
// [DV_INTERVAL<DV_COUNT>, DV_INTERVAL] it binds the bare child in either
// order, and the typed child's mandatory upper bound is not reported. Under a
// DV_INTERVAL<DV_COUNT> child alone it is still admitted, in the third tier
// (REQ-110 Known limitations): it is walked under that child, so a mandatory
// upper bound is reported there, and no type mismatch is.
//
// The third-tier admission holds only when no bound is present. A bare
// interval with a DV_QUANTITY bound has the parameter DV_QUANTITY, so the
// DV_INTERVAL<DV_COUNT> child does not admit it, in either order: it binds the
// bare child, which it conforms to, and nothing is reported.
func TestValidate_REQ102_UnboundedIntervalNamesBareChild(t *testing.T) {
	t.Parallel()
	unbounded := &rm.DVInterval[rm.DVOrdered]{LowerUnbounded: true, UpperUnbounded: true}
	quantityBounded := &rm.DVInterval[rm.DVOrdered]{
		Lower: &rm.DVQuantity{Magnitude: 1, Units: "mm"}, LowerIncluded: true,
		UpperUnbounded: true,
	}
	tests := []struct {
		name     string
		value    rm.DataValue
		children []string
		want     []string
	}{
		{name: "unbounded, typed first", value: unbounded, children: []string{countIntervalUpperRequiredChild, bareIntervalChild}, want: []string{}},
		{name: "unbounded, bare first", value: unbounded, children: []string{bareIntervalChild, countIntervalUpperRequiredChild}, want: []string{}},
		{name: "unbounded, typed alone", value: unbounded, children: []string{countIntervalChild}, want: []string{}},
		{name: "unbounded, typed alone, upper mandatory", value: unbounded, children: []string{countIntervalUpperRequiredChild}, want: []string{"required /value/upper"}},
		{name: "DV_QUANTITY bound, typed first", value: quantityBounded, children: []string{countIntervalUpperRequiredChild, bareIntervalChild}, want: []string{}},
		{name: "DV_QUANTITY bound, bare first", value: quantityBounded, children: []string{bareIntervalChild, countIntervalUpperRequiredChild}, want: []string{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := mustCompileInline(t, elementValueOPT(tc.children...))
			r := validation.Validate(valueElement(tc.value), c)
			if got := issueKeys(r.Issues); !slices.Equal(got, tc.want) {
				t.Errorf("Validate(bare DV_INTERVAL, %s) issues = %q, want %q", tc.name, got, tc.want)
			}
		})
	}
}

// REQ-102 — a slot child never binds an item that is not a LOCATABLE, in
// any tier. A DV_CODED_TEXT names neither child, and conforms to both the
// DV_TEXT slot listed first and the DV_TEXT child after it; it binds to the
// DV_TEXT child, which is walked and reports the item's empty value.
func TestValidateDemographic_REQ102_SlotSkippedInConformanceTier(t *testing.T) {
	t.Parallel()
	const dvTextSlot = `
      <children xsi:type="ARCHETYPE_SLOT">
        <rm_type_name>DV_TEXT</rm_type_name>
        <node_id>at9000</node_id>
      </children>`
	c := mustCompileInline(t, actorOPT(dvTextSlot+dvTextChild))
	p := validActor()
	p.Languages = []rm.DVTextLike{rm.DVCodedText{
		DefiningCode: rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "local"}, CodeString: "at0001"},
	}}
	want := []string{"required /languages[1]/value"}
	if got := issueKeys(validation.ValidateDemographic(p, c).Issues); !slices.Equal(got, want) {
		t.Errorf("ValidateDemographic(DV_CODED_TEXT language under [DV_TEXT slot, DV_TEXT]) issues = %q, want %q", got, want)
	}
}

// orderedIntervalChild admits a DV_INTERVAL<DV_ORDERED> whose bounds are
// DV_COUNTs of 0 to 100: the type parameter is abstract, and the bounds
// conform to it.
var orderedIntervalChild = `
      <children xsi:type="C_COMPLEX_OBJECT">
        <rm_type_name>DV_INTERVAL&lt;DV_ORDERED&gt;</rm_type_name>
        <node_id></node_id>` + orderedIntervalBound("lower") + orderedIntervalBound("upper") + `
      </children>`

// orderedIntervalBound constrains the side attribute of orderedIntervalChild
// to a DV_COUNT of 0 to 100.
func orderedIntervalBound(side string) string {
	return `
        <attributes xsi:type="C_SINGLE_ATTRIBUTE">
          <rm_attribute_name>` + side + `</rm_attribute_name>
          <children xsi:type="C_COMPLEX_OBJECT">
            <rm_type_name>DV_COUNT</rm_type_name>
            <node_id></node_id>
            <attributes xsi:type="C_SINGLE_ATTRIBUTE">
              <rm_attribute_name>magnitude</rm_attribute_name>
              <children xsi:type="C_PRIMITIVE_OBJECT">
                <rm_type_name>INTEGER</rm_type_name>
                <node_id></node_id>
                <item xsi:type="C_INTEGER">
                  <range>
                    <lower_included>true</lower_included><upper_included>true</upper_included>
                    <lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded>
                    <lower>0</lower><upper>100</upper>
                  </range>
                </item>
              </children>
            </attributes>
          </children>
        </attributes>`
}

// REQ-110, REQ-102 (RM type match) — under a DV_INTERVAL<T> whose T is
// abstract, each present bound has to be a T: a bound of a type that
// conforms to T, such as a DV_COUNT under DV_ORDERED, is one. That holds for
// the bare interval a round trip decodes and for a typed one whose parameter
// conforms to T; a bound that does not conform is still refused.
func TestValidate_REQ110_IntervalBoundsConformToAbstractParameter(t *testing.T) {
	t.Parallel()
	c := mustCompileInline(t, elementValueOPT(orderedIntervalChild))
	count := func(m int64) *rm.DVCount { return &rm.DVCount{Magnitude: m} }
	tests := []struct {
		name  string
		value rm.DataValue
		want  []string
	}{
		{
			name:  "bare interval with DV_COUNT bounds",
			value: &rm.DVInterval[rm.DVOrdered]{Lower: count(1), Upper: count(50), LowerIncluded: true, UpperIncluded: true},
			want:  []string{},
		},
		{
			name:  "typed DV_INTERVAL<DV_COUNT>",
			value: &rm.DVInterval[rm.DVCount]{Lower: rm.DVCount{Magnitude: 1}, Upper: rm.DVCount{Magnitude: 50}, LowerIncluded: true, UpperIncluded: true},
			want:  []string{},
		},
		{
			name: "bare half-open interval with a DV_COUNT lower bound",
			value: &rm.DVInterval[rm.DVOrdered]{
				Lower: count(1), LowerIncluded: true, UpperUnbounded: true,
			},
			want: []string{},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := issueKeys(validation.Validate(valueElement(tc.value), c).Issues); !slices.Equal(got, tc.want) {
				t.Errorf("Validate(%s under DV_INTERVAL<DV_ORDERED>) issues = %q, want %q", tc.name, got, tc.want)
			}
		})
	}

	// A bound that does not conform to the parameter is refused. Every bound
	// a Go interval can hold is a DV_ORDERED, so the refusal is shown under a
	// DV_INTERVAL<DV_COUNT>, whose parameter a DV_QUANTITY does not conform to.
	countOnly := mustCompileInline(t, elementValueOPT(countIntervalChild))
	quantity := &rm.DVQuantity{Magnitude: 1, Units: "1"}
	for name, v := range map[string]rm.DataValue{
		"bare interval with DV_QUANTITY bounds": &rm.DVInterval[rm.DVOrdered]{Lower: quantity, Upper: quantity, LowerIncluded: true, UpperIncluded: true},
		"typed DV_INTERVAL<DV_QUANTITY>":        &rm.DVInterval[rm.DVQuantity]{Lower: *quantity, Upper: *quantity, LowerIncluded: true, UpperIncluded: true},
	} {
		want := []string{"rm_type_mismatch /value"}
		if got := issueKeys(validation.Validate(valueElement(v), countOnly).Issues); !slices.Equal(got, want) {
			t.Errorf("Validate(%s under DV_INTERVAL<DV_COUNT>) issues = %q, want %q", name, got, want)
		}
	}
}

// REQ-107, REQ-110 — the generator's output for a DV_INTERVAL<DV_ORDERED>
// with DV_COUNT bounds, under both policies, validates against that OPT.
func TestGenerate_REQ107_REQ110_AbstractIntervalParameterValidates(t *testing.T) {
	t.Parallel()
	c := mustCompileInline(t, elementValueOPT(orderedIntervalChild))
	for _, policy := range []instance.Policy{instance.Minimal, instance.Example} {
		t.Run(policy.String(), func(t *testing.T) {
			t.Parallel()
			out, err := instance.Generate(t.Context(), c, instance.Options{Policy: policy})
			if err != nil {
				t.Fatalf("Generate: %v", err)
			}
			el, ok := out.(*rm.Element)
			if !ok {
				t.Fatalf("Generate root = %T, want *rm.Element", out)
			}
			if got := issueKeys(validation.Validate(el, c).Issues); len(got) != 0 {
				t.Errorf("Validate(generated %T) issues = %q, want none", el.Value, got)
			}
		})
	}
}
