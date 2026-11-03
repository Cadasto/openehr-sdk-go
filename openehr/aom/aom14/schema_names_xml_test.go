package aom14_test

// REQ-176 § Shapes and § Round trip, REQ-056: canonical XML reads
// `xsi:type="T_ARCHETYPE_ROOT"` as C_ARCHETYPE_ROOT and writes it as
// C_ARCHETYPE_ROOT; names the root element of an OPERATIONAL_TEMPLATE
// `template`, after Template.xsd's global element, and reads a root that
// carries `xsi:type="OPERATIONAL_TEMPLATE"`.

import (
	"bytes"
	"encoding/xml"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aom/aom14"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml"
)

// archetypeRootChild is a C_SINGLE_ATTRIBUTE whose one child carries the
// xsi:type given.
func archetypeRootChild(xsiType string) string {
	return `<c_single_attribute` + nsOpenEHR + `><rm_attribute_name>content</rm_attribute_name>` +
		`<existence><lower>1</lower><upper>1</upper></existence>` +
		`<children xsi:type="` + xsiType + `"><rm_type_name>OBSERVATION</rm_type_name>` +
		`<occurrences><lower_included>true</lower_included><upper_included>true</upper_included>` +
		`<lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded><lower>1</lower><upper>1</upper></occurrences>` +
		`<node_id/><archetype_id><value>openEHR-EHR-OBSERVATION.test.v1</value></archetype_id></children></c_single_attribute>`
}

// TestREQ176_TArchetypeRootReadsAsCArchetypeRoot pins the T_ARCHETYPE_ROOT
// alias: a child written `xsi:type="T_ARCHETYPE_ROOT"` decodes in every mode,
// with nothing reported, to the same *aom14.CArchetypeRoot as one written
// C_ARCHETYPE_ROOT, and encodes as C_ARCHETYPE_ROOT; the type registry does
// not hold the alias (TestREQ176_REQ041_TypesCarryTheRootsNames), so without
// the alias the child names an unknown type. social.opt, its seven archetype
// roots rewritten to T_ARCHETYPE_ROOT, decodes to the value of the file as
// written.
func TestREQ176_TArchetypeRootReadsAsCArchetypeRoot(t *testing.T) {
	var want aom14.CSingleAttribute
	if err := canxml.Unmarshal([]byte(archetypeRootChild("C_ARCHETYPE_ROOT")), &want, canxml.WithStrict()); err != nil {
		t.Fatalf("canxml.Unmarshal(C_ARCHETYPE_ROOT) = %v; want nil", err)
	}
	if _, ok := want.Children[0].(*aom14.CArchetypeRoot); !ok {
		t.Fatalf("a C_ARCHETYPE_ROOT child decodes as %T; want *aom14.CArchetypeRoot", want.Children[0])
	}
	var reported []string
	for mode, opts := range decodeModes(&reported) {
		var got aom14.CSingleAttribute
		if err := canxml.Unmarshal([]byte(archetypeRootChild("T_ARCHETYPE_ROOT")), &got, opts...); err != nil {
			t.Fatalf("canxml.Unmarshal(T_ARCHETYPE_ROOT, %s) = %v; want nil", mode, err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("canxml.Unmarshal(T_ARCHETYPE_ROOT, %s) = %#v; want %#v", mode, got, want)
		}
	}
	if len(reported) != 0 {
		t.Errorf("the reporting decode reported %q; want nothing", reported)
	}
	b, err := canxml.Marshal(&want)
	if err != nil {
		t.Fatalf("canxml.Marshal: %v", err)
	}
	if !strings.Contains(string(b), `xsi:type="C_ARCHETYPE_ROOT"`) || strings.Contains(string(b), "T_ARCHETYPE_ROOT") {
		t.Errorf("canxml.Marshal wrote %s; want the child as C_ARCHETYPE_ROOT", b)
	}
	if _, ok := typereg.Default.Lookup("T_ARCHETYPE_ROOT"); ok {
		t.Error("typereg.Default holds T_ARCHETYPE_ROOT; the alias belongs to canonical-XML dispatch alone")
	}
	err = canxml.Unmarshal([]byte(archetypeRootChild("X_ARCHETYPE_ROOT")), new(aom14.CSingleAttribute))
	if !errors.Is(err, typereg.ErrUnknownType) {
		t.Errorf("canxml.Unmarshal(X_ARCHETYPE_ROOT) = %v; want typereg.ErrUnknownType, only the named alias reads", err)
	}

	original, err := os.ReadFile("../../../testkit/corpus/templates/social.opt")
	if err != nil {
		t.Fatalf("read social.opt: %v", err)
	}
	from, to := []byte(`xsi:type="C_ARCHETYPE_ROOT"`), []byte(`xsi:type="T_ARCHETYPE_ROOT"`)
	if got := bytes.Count(original, from); got != 7 {
		t.Fatalf("social.opt holds %d %s; want 7", got, from)
	}
	var asWritten, rewritten aom14.OperationalTemplate
	if err := canxml.Unmarshal(original, &asWritten); err != nil {
		t.Fatalf("canxml.Unmarshal(social.opt) = %v", err)
	}
	if err := canxml.Unmarshal(bytes.ReplaceAll(original, from, to), &rewritten); err != nil {
		t.Fatalf("canxml.Unmarshal(social.opt with T_ARCHETYPE_ROOT) = %v", err)
	}
	if !reflect.DeepEqual(rewritten, asWritten) {
		t.Error("social.opt with T_ARCHETYPE_ROOT decodes to another value than social.opt as written")
	}
}

// minimalTemplate is an OPERATIONAL_TEMPLATE with its mandatory members, under
// a root element of the name and with the attributes given.
func minimalTemplate(root, attrs string) string {
	return `<` + root + nsOpenEHR + attrs + `><language>` + `<terminology_id><value>ISO_639-1</value></terminology_id><code_string>en</code_string></language>` +
		`<template_id><value>t.v1</value></template_id><concept>c</concept>` +
		`<definition><rm_type_name>COMPOSITION</rm_type_name><occurrences><lower_included>true</lower_included><upper_included>true</upper_included>` +
		`<lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded><lower>1</lower><upper>1</upper></occurrences>` +
		`<node_id>at0000</node_id><archetype_id><value>openEHR-EHR-COMPOSITION.test.v1</value></archetype_id></definition></` + root + `>`
}

// rootName returns the local name of the root element of doc.
func rootName(t *testing.T, doc []byte) string {
	t.Helper()
	var root struct{ XMLName xml.Name }
	if err := xml.Unmarshal(doc, &root); err != nil {
		t.Fatalf("reading %s: %v", doc, err)
	}
	return root.XMLName.Local
}

// TestREQ176_OperationalTemplateRootElementIsTemplate pins the root element:
// canxml.Marshal and canxml.MarshalIndent write an OPERATIONAL_TEMPLATE under
// `template`, Template.xsd's global element for the type, with no `xsi:type`,
// and an ARCHETYPE under `archetype`, Archetype.xsd's; and a `template` root
// that carries `xsi:type="OPERATIONAL_TEMPLATE"`, as social.opt does, decodes
// in every mode with nothing reported, to the value of one that does not.
func TestREQ176_OperationalTemplateRootElementIsTemplate(t *testing.T) {
	var want aom14.OperationalTemplate
	if err := canxml.Unmarshal([]byte(minimalTemplate("template", "")), &want, canxml.WithStrict()); err != nil {
		t.Fatalf("canxml.Unmarshal(template) = %v; want nil", err)
	}
	var reported []string
	for mode, opts := range decodeModes(&reported) {
		var got aom14.OperationalTemplate
		if err := canxml.Unmarshal([]byte(minimalTemplate("template", ` xsi:type="OPERATIONAL_TEMPLATE"`)), &got, opts...); err != nil {
			t.Fatalf("canxml.Unmarshal(a root with xsi:type, %s) = %v; want nil", mode, err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("canxml.Unmarshal(a root with xsi:type, %s) = %#v; want %#v", mode, got, want)
		}
	}
	if len(reported) != 0 {
		t.Errorf("the reporting decode reported %q; want nothing", reported)
	}

	compact, err := canxml.Marshal(&want)
	if err != nil {
		t.Fatalf("canxml.Marshal: %v", err)
	}
	indented, err := canxml.MarshalIndent(&want, "", "  ")
	if err != nil {
		t.Fatalf("canxml.MarshalIndent: %v", err)
	}
	for _, doc := range [][]byte{compact, indented} {
		if got := rootName(t, doc); got != "template" {
			t.Errorf("an OPERATIONAL_TEMPLATE is written under %q; want template:\n%s", got, doc)
		}
		if strings.Contains(string(doc), `xsi:type="OPERATIONAL_TEMPLATE"`) {
			t.Errorf("the root carries an xsi:type:\n%s", doc)
		}
	}
	var back aom14.OperationalTemplate
	if err := canxml.Unmarshal(compact, &back, canxml.WithStrict()); err != nil || !reflect.DeepEqual(back, want) {
		t.Errorf("round trip of %s = %v; want the value decoded", compact, err)
	}

	b, err := canxml.Marshal(&aom14.Archetype{})
	if err != nil {
		t.Fatalf("canxml.Marshal(ARCHETYPE): %v", err)
	}
	if got := rootName(t, b); got != "archetype" {
		t.Errorf("an ARCHETYPE is written under %q; want archetype", got)
	}
}
