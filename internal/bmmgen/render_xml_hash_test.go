package bmmgen

// REQ-176 § Round trip, REQ-056 — the generated canonical-XML codec writes a
// Hash member one element per entry, in the AM Release-1.4 schemas' shapes: a
// Hash<String,String> as StringDictionaryItems, the key in `id` and the value
// as text; a Hash of class values keyed by the value's key attribute where its
// schema type declares one, else by its language code, with no `id` for the
// resource classes in either target; entries in ascending key order.

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/bmm"
)

// hashShapeOf returns the shape the plan gives the member prop of class, and
// whether it is a Hash.
func hashShapeOf(t *testing.T, plan *Plan, class, prop string) (*xmlHash, bool) {
	t.Helper()
	pc, ok := plan.Classes[class]
	if !ok {
		t.Fatalf("plan has no class %s", class)
	}
	fields, err := effectiveFields(plan, pc)
	if err != nil {
		t.Fatalf("effectiveFields(%s): %v", class, err)
	}
	i := slices.IndexFunc(fields, func(ef emittedField) bool { return ef.Prop.PropertyName() == prop })
	if i < 0 {
		t.Fatalf("%s has no member %s", class, prop)
	}
	shape, isHash, err := xmlHashOf(plan, fields[i])
	if err != nil {
		t.Fatalf("xmlHashOf(%s.%s): %v", class, prop, err)
	}
	return shape, isHash
}

// TestREQ176_XMLHashShapes pins the shape of every Hash member the two
// targets emit a codec for: the Hash<String,String> members of the resource
// classes, ARCHETYPE_TERM and ANNOTATION keyed by `id`; RESOURCE_DESCRIPTION's
// details and ARCHETYPE's translations keyed by the value's language code,
// in the RM target too, whose BASE Release-1.2.0 schema gives the two classes
// an `id`; and RESOURCE_ANNOTATIONS' Hash of Hashes left unmodelled.
func TestREQ176_XMLHashShapes(t *testing.T) {
	rmPlan := buildPlanWithITSXML(t, TargetRM)
	aomPlan := buildPlanWithITSXML(t, TargetAOM14)
	strs := func(optional bool) *xmlHash {
		return &xmlHash{valueType: "string", keyAttr: "id", optional: optional}
	}
	byLanguage := func(valueType string, optional bool) *xmlHash {
		return &xmlHash{valueType: valueType, languageKey: "Language.CodeString", optional: optional}
	}
	for _, tc := range []struct {
		plan        *Plan
		class, prop string
		want        *xmlHash
	}{
		{rmPlan, "RESOURCE_DESCRIPTION", "original_author", strs(false)},
		{rmPlan, "RESOURCE_DESCRIPTION", "other_details", strs(true)},
		{rmPlan, "RESOURCE_DESCRIPTION", "details", byLanguage("ResourceDescriptionItem", false)},
		{rmPlan, "RESOURCE_DESCRIPTION_ITEM", "original_resource_uri", strs(true)},
		{rmPlan, "RESOURCE_DESCRIPTION_ITEM", "other_details", strs(true)},
		{rmPlan, "TRANSLATION_DETAILS", "author", strs(false)},
		{rmPlan, "TRANSLATION_DETAILS", "other_details", strs(true)},
		{rmPlan, "RESOURCE_ANNOTATIONS", "documentation", nil},
		{aomPlan, "ARCHETYPE_TERM", "items", strs(false)},
		{aomPlan, "ANNOTATION", "items", strs(false)},
		{aomPlan, "ARCHETYPE", "translations", byLanguage("rm.TranslationDetails", true)},
		{aomPlan, "RESOURCE_DESCRIPTION", "details", byLanguage("rm.ResourceDescriptionItem", false)},
	} {
		t.Run(tc.plan.Target.GoPackage+"/"+tc.class+"."+tc.prop, func(t *testing.T) {
			got, isHash := hashShapeOf(t, tc.plan, tc.class, tc.prop)
			if !isHash {
				t.Fatalf("xmlHashOf(%s.%s) says it is no Hash", tc.class, tc.prop)
			}
			if fmt.Sprint(got) != fmt.Sprint(tc.want) {
				t.Errorf("xmlHashOf(%s.%s) = %+v; want %+v", tc.class, tc.prop, got, tc.want)
			}
		})
	}
}

// TestREQ176_XMLHashCodecOfResourceDescription pins the codec rendered for
// RESOURCE_DESCRIPTION's Hash members: entries written in ascending key order,
// a String value under an `id` attribute and the details with none; and each
// entry decoded through the runtime's entry helper, under its `id` or under
// the value's language code.
func TestREQ176_XMLHashCodecOfResourceDescription(t *testing.T) {
	plan := buildPlanWithITSXML(t, TargetRM)
	enc, dec := renderedXMLCodec(t, plan, "RESOURCE_DESCRIPTION")
	for _, want := range []string{
		`for _, _k := range slices.Sorted(maps.Keys(r.OriginalAuthor)) {`,
		`Attr: []xml.Attr{{Name: xml.Name{Local: "id"}, Value: _k}}`,
		`for _, _k := range slices.Sorted(maps.Keys(*r.OtherDetails)) {`,
		`for _, _k := range slices.Sorted(maps.Keys(r.Details)) {`,
		`if err := _e.EncodeElement(&_v, xml.StartElement{Name: xml.Name{Local: "details"}}); err != nil {`,
	} {
		if !strings.Contains(enc, want) {
			t.Errorf("RESOURCE_DESCRIPTION encoder lacks %q:\n%s", want, enc)
		}
	}
	for prop, want := range map[string]string{
		"original_author": `xmlunmatched.DecodeEntry(_dec, &_t, "id", &r.OriginalAuthor)`,
		"other_details":   `xmlunmatched.DecodeEntry(_dec, &_t, "id", r.OtherDetails)`,
		"details":         `xmlunmatched.DecodeEntryKeyedBy(_dec, &_t, func(_v *ResourceDescriptionItem) string { return _v.Language.CodeString }, &r.Details)`,
	} {
		if arm := xmlCaseArm(dec, prop); !strings.Contains(arm, want) {
			t.Errorf("RESOURCE_DESCRIPTION decoder arm for %s lacks %q:\n%s", prop, want, arm)
		}
	}
	if arm := xmlCaseArm(dec, "other_details"); !strings.Contains(arm, "r.OtherDetails = new(map[string]string)") {
		t.Errorf("RESOURCE_DESCRIPTION decoder arm for the optional other_details allocates no map:\n%s", arm)
	}
}

// renderedXMLCodec renders the encoder and the decoder of class in plan.
func renderedXMLCodec(t *testing.T, plan *Plan, class string) (enc, dec string) {
	t.Helper()
	pc := plan.Classes[class]
	fields, err := effectiveFields(plan, pc)
	if err != nil {
		t.Fatalf("effectiveFields(%s): %v", class, err)
	}
	ordered, err := xmlChildFields(plan, pc, fields)
	if err != nil {
		t.Fatalf("xmlChildFields(%s): %v", class, err)
	}
	if enc, err = renderMarshalXML(plan, pc, ordered); err != nil {
		t.Fatalf("renderMarshalXML(%s): %v", class, err)
	}
	if dec, err = renderUnmarshalXML(plan, pc, fields); err != nil {
		t.Fatalf("renderUnmarshalXML(%s): %v", class, err)
	}
	return enc, dec
}

// TestREQ176_XMLHashKeyedByAttribute pins the other key of a Hash of class
// values: under a fixture schema whose RESOURCE_DESCRIPTION_ITEM declares an
// `id` attribute that names no member, as BASE Release-1.2.0's does, the
// details are keyed by it, written on each entry and taken from it on decode.
func TestREQ176_XMLHashKeyedByAttribute(t *testing.T) {
	dir, target := fixtureSchemaTarget(t, TargetRM,
		`<xs:complexType name="RESOURCE_DESCRIPTION_ITEM"><xs:sequence><xs:element name="language"/></xs:sequence>`+
			`<xs:attribute name="id" type="xs:string" use="required"/></xs:complexType>`)
	plan := buildPlanWithSchemas(t, target, dir)
	if got, _ := hashShapeOf(t, plan, "RESOURCE_DESCRIPTION", "details"); got == nil || got.keyAttr != "id" || got.languageKey != "" {
		t.Fatalf("xmlHashOf(RESOURCE_DESCRIPTION.details) = %+v; want the key in `id`", got)
	}
	enc, dec := renderedXMLCodec(t, plan, "RESOURCE_DESCRIPTION")
	if want := `_e.EncodeElement(&_v, xml.StartElement{Name: xml.Name{Local: "details"}, Attr: []xml.Attr{{Name: xml.Name{Local: "id"}, Value: _k}}})`; !strings.Contains(enc, want) {
		t.Errorf("RESOURCE_DESCRIPTION encoder lacks %q:\n%s", want, enc)
	}
	if want := `xmlunmatched.DecodeEntry(_dec, &_t, "id", &r.Details)`; !strings.Contains(xmlCaseArm(dec, "details"), want) {
		t.Errorf("RESOURCE_DESCRIPTION decoder arm for details lacks %q:\n%s", want, xmlCaseArm(dec, "details"))
	}
}

// TestREQ176_XMLHashWithoutKeyRefused pins the refusal at generation of a
// Hash of class values canonical XML has no key for: a value class whose
// schema type declares two attributes that name no member, and one that
// declares none and has no mandatory language code.
func TestREQ176_XMLHashWithoutKeyRefused(t *testing.T) {
	t.Run("two key attributes", func(t *testing.T) {
		dir, target := fixtureSchemaTarget(t, TargetRM,
			`<xs:complexType name="RESOURCE_DESCRIPTION_ITEM"><xs:sequence/>`+
				`<xs:attribute name="id" type="xs:string"/><xs:attribute name="lang" type="xs:string"/></xs:complexType>`)
		plan := buildPlanWithSchemas(t, target, dir)
		ef := memberOf(t, plan, "RESOURCE_DESCRIPTION", "details")
		if _, _, err := xmlHashOf(plan, ef); !errors.Is(err, errXMLHashNoKey) {
			t.Errorf("xmlHashOf(RESOURCE_DESCRIPTION.details) = %v; want errXMLHashNoKey", err)
		}
	})
	t.Run("no language", func(t *testing.T) {
		plan := buildPlanWithITSXML(t, TargetRM)
		owner := plan.Classes["RESOURCE_DESCRIPTION"].Class.(*bmm.SimpleClass)
		prop := &bmm.GenericProperty{TypeDef: &bmm.GenericType{
			RootType:          "Hash",
			GenericParameters: []bmm.Type{&bmm.SimpleType{TypeName: "String"}, &bmm.SimpleType{TypeName: "DV_BOOLEAN"}},
		}}
		prop.Name = "flags"
		ef := emittedField{Prop: prop, Owner: owner, OwnerName: "RESOURCE_DESCRIPTION"}
		if _, _, err := xmlHashOf(plan, ef); !errors.Is(err, errXMLHashNoKey) {
			t.Errorf("xmlHashOf(a Hash of DV_BOOLEAN) = %v; want errXMLHashNoKey", err)
		}
	})
}

// memberOf returns the effective field prop of class in plan.
func memberOf(t *testing.T, plan *Plan, class, prop string) emittedField {
	t.Helper()
	fields, err := effectiveFields(plan, plan.Classes[class])
	if err != nil {
		t.Fatalf("effectiveFields(%s): %v", class, err)
	}
	i := slices.IndexFunc(fields, func(ef emittedField) bool { return ef.Prop.PropertyName() == prop })
	if i < 0 {
		t.Fatalf("%s has no member %s", class, prop)
	}
	return fields[i]
}
