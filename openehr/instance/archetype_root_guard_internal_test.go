package instance

import (
	"errors"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/rmroots"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/rminfo"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
)

// contentOPT is a COMPOSITION template whose content attribute has the
// given existence and no children.
func contentOPT(lower int) string {
	bound := "0"
	if lower > 0 {
		bound = "1"
	}
	return `<?xml version="1.0" encoding="utf-8"?>
<template xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xmlns="http://schemas.openehr.org/v1">
<language><terminology_id><value>ISO_639-1</value></terminology_id><code_string>en</code_string></language>
<template_id><value>implicit_single</value></template_id><concept>implicit_single</concept>
<definition><rm_type_name>COMPOSITION</rm_type_name><node_id>at0000</node_id>
<attributes xsi:type="C_MULTIPLE_ATTRIBUTE"><rm_attribute_name>content</rm_attribute_name>
<existence><lower_included>true</lower_included><upper_included>true</upper_included><lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded><lower>` + bound + `</lower><upper>1</upper></existence>
<cardinality><is_ordered>false</is_ordered><is_unique>false</is_unique><interval><lower_included>true</lower_included><lower_unbounded>false</lower_unbounded><upper_unbounded>true</upper_unbounded><lower>0</lower></interval></cardinality>
</attributes>
<archetype_id><value>openEHR-EHR-COMPOSITION.encounter.v1</value></archetype_id></definition>
</template>`
}

// TestREQ107_ImplicitSingleArchetypeRootRefusedWhenRequired checks the
// single-attribute path of the generator's BMM fill (REQ-107). A required
// attribute the template leaves without children, whose default would be an
// archetype root, is refused with ErrArchetypeIDMissing. An optional one is
// not refused, and the generator builds nothing for it, so no archetype
// root appears without archetype_details. The refusal wraps neither
// ErrSlotFillUnsupported nor ErrConstraintUnsatisfiable.
//
// No attribute the pinned RM declares single-valued has a default that is
// an archetype root (TestREQ107_SingleAttributeDefaultsAreNoArchetypeRoot).
// A template reaches this path when it writes a multi-valued attribute such
// as COMPOSITION.content as a C_SINGLE_ATTRIBUTE, which the external
// TestREQ107_UnnamedArchetypeRootIsRefused covers through Generate. This
// test hands the path COMPOSITION.content directly, whose default is an
// OBSERVATION, so the optional case can count what the path builds.
func TestREQ107_ImplicitSingleArchetypeRootRefusedWhenRequired(t *testing.T) {
	cases := []struct {
		name  string
		lower int
		// detail is what the error says after the sentinel's own text; ""
		// means the call must succeed and build nothing.
		detail string
	}{
		{name: "required", lower: 1, detail: "OBSERVATION for COMPOSITION.content at / (required, but the template names no child)"},
		{name: "optional", lower: 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opt, err := template.ParseOPT(strings.NewReader(contentOPT(tc.lower)))
			if err != nil {
				t.Fatalf("ParseOPT: %v", err)
			}
			c, err := templatecompile.Compile(opt)
			if err != nil {
				t.Fatalf("Compile: %v", err)
			}
			attr := c.Root().Attribute("content")
			if attr == nil {
				t.Fatal("compiled root has no content attribute")
			}
			uids := 0
			g := &generator{compiled: c, opts: Options{
				Policy: Example,
				UIDSource: func() *rm.HierObjectID {
					uids++
					return &rm.HierObjectID{Value: "00000000-0000-4000-8000-000000000001"}
				},
			}}
			err = g.materialiseImplicitSingle(c.Root(), attr, &rm.Composition{})
			if tc.detail == "" {
				if err != nil {
					t.Errorf("materialiseImplicitSingle(optional content) = %v, want nil", err)
				}
				if uids != 0 {
					t.Errorf("materialiseImplicitSingle(optional content) drew %d uids, want 0: it built a default", uids)
				}
				return
			}
			if !errors.Is(err, ErrArchetypeIDMissing) {
				t.Fatalf("materialiseImplicitSingle(required content) = %v, want an error wrapping ErrArchetypeIDMissing", err)
			}
			for _, other := range []error{ErrSlotFillUnsupported, ErrConstraintUnsatisfiable} {
				if errors.Is(err, other) {
					t.Errorf("materialiseImplicitSingle(required content) = %v, want it not to wrap %v", err, other)
				}
			}
			if got, ok := strings.CutPrefix(err.Error(), ErrArchetypeIDMissing.Error()+": "); !ok || got != tc.detail {
				t.Errorf("materialiseImplicitSingle(required content) = %q, want %q followed by %q", err, ErrArchetypeIDMissing, tc.detail)
			}
		})
	}
}

// TestREQ107_SingleAttributeDefaultsAreNoArchetypeRoot pins that only a
// template that writes a multi-valued attribute as single reaches the
// archetype-root refusal on the single-attribute path of the generator's
// BMM fill (REQ-107). For every attribute rminfo knows as single-valued,
// the value the generator builds for the attribute's RM type, by the
// mapping that path uses, is not of an archetype-root class. A BMM bump
// that breaks this makes the refusal reachable from a well-formed template
// and fails here.
func TestREQ107_SingleAttributeDefaultsAreNoArchetypeRoot(t *testing.T) {
	lister, ok := rminfo.Default.(rminfo.AttributeLister)
	if !ok {
		t.Fatal("rminfo.Default does not list attributes")
	}
	checked, built := 0, 0
	for _, class := range rminfo.Default.KnownRMTypes() {
		for _, attr := range lister.AttributeNames(class) {
			if container, _ := rminfo.Default.IsContainer(class, attr); container {
				continue
			}
			rmType, ok := rminfo.Default.AttributeRMType(class, attr)
			if !ok {
				t.Errorf("rminfo lists %s.%s but gives it no RM type", class, attr)
				continue
			}
			checked++
			v, err := newRMForOPTType(rmType)
			if err != nil {
				continue
			}
			built++
			if got := rmTypeOf(v); rmroots.IsArchetypeRoot(got) {
				t.Errorf("%s.%s is typed %s, and the generator builds its default as %s, an archetype root", class, attr, rmType, got)
			}
		}
	}
	if checked == 0 || built == 0 {
		t.Fatalf("checked %d single attributes and built %d defaults; want some of each, or the check is vacuous", checked, built)
	}
	t.Logf("checked %d single attributes, built %d defaults", checked, built)
}
