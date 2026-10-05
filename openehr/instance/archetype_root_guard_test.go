package instance_test

import (
	"encoding/json"
	"errors"
	"fmt"
	mrand "math/rand/v2"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/cadasto/openehr-sdk-go/internal/rmroots"
	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// The OPTs below are small COMPOSITION templates for the REQ-107 rule on
// archetype roots: an object of a class that is always an archetype root
// needs an archetype id from the template, or the generator refuses it.
// Each template differs from the others only in its root's archetype id and
// in its content attribute.

const (
	guardExistence11 = `<existence><lower_included>true</lower_included><upper_included>true</upper_included>` +
		`<lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded>` +
		`<lower>1</lower><upper>1</upper></existence>`
	guardExistence01 = `<existence><lower_included>true</lower_included><upper_included>true</upper_included>` +
		`<lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded>` +
		`<lower>0</lower><upper>1</upper></existence>`
	guardOccurrences11 = `<occurrences><lower_included>true</lower_included><upper_included>true</upper_included>` +
		`<lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded>` +
		`<lower>1</lower><upper>1</upper></occurrences>`
	guardOccurrences01 = `<occurrences><lower_included>true</lower_included><upper_included>true</upper_included>` +
		`<lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded>` +
		`<lower>0</lower><upper>1</upper></occurrences>`
	// guardOpenCardinality has no lower bound, so only the existence of
	// the attribute decides whether it is required.
	guardOpenCardinality = `<cardinality><is_ordered>false</is_ordered><is_unique>false</is_unique><interval>` +
		`<lower_included>true</lower_included><lower_unbounded>false</lower_unbounded>` +
		`<upper_unbounded>true</upper_unbounded><lower>0</lower></interval></cardinality>`

	guardCompositionID = "openEHR-EHR-COMPOSITION.encounter.v1"
	guardObservationID = "openEHR-EHR-OBSERVATION.example.v1"
)

// guardOPT is a COMPOSITION template with one content attribute. An empty
// rootArchetypeID leaves the archetype id off the template root.
func guardOPT(rootArchetypeID, content string) string {
	id := ""
	if rootArchetypeID != "" {
		id = `<archetype_id><value>` + rootArchetypeID + `</value></archetype_id>`
	}
	return `<?xml version="1.0" encoding="utf-8"?>
<template xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xmlns="http://schemas.openehr.org/v1">
<language><terminology_id><value>ISO_639-1</value></terminology_id><code_string>en</code_string></language>
<template_id><value>archetype_root_guard</value></template_id><concept>archetype_root_guard</concept>
<definition><rm_type_name>COMPOSITION</rm_type_name><node_id>at0000</node_id>` + content + id + `</definition>
</template>`
}

// guardMultiple is a C_MULTIPLE_ATTRIBUTE called name with the given
// existence and children, and no lower bound on its cardinality.
func guardMultiple(name, existence string, children ...string) string {
	return `<attributes xsi:type="C_MULTIPLE_ATTRIBUTE"><rm_attribute_name>` + name + `</rm_attribute_name>` +
		existence + guardOpenCardinality + strings.Join(children, "") + `</attributes>`
}

// guardChild is an OPT child of the given xsi:type and RM type. An empty
// archetypeID leaves the archetype id off it.
func guardChild(xsiType, rmType, nodeID, occurrences, archetypeID string, attrs ...string) string {
	id := ""
	if archetypeID != "" {
		id = `<archetype_id><value>` + archetypeID + `</value></archetype_id>`
	}
	return `<children xsi:type="` + xsiType + `"><rm_type_name>` + rmType + `</rm_type_name>` + occurrences +
		`<node_id>` + nodeID + `</node_id>` + strings.Join(attrs, "") + id + `</children>`
}

// guardOptions are the four generator settings the rule holds for: both
// policies, each with both value fills. RandomFill draws from a fixed
// source, so a failure can be replayed.
func guardOptions() []instance.Options {
	var out []instance.Options
	for _, policy := range []instance.Policy{instance.Minimal, instance.Example} {
		for _, fill := range []instance.ValueFill{instance.ExampleFill, instance.RandomFill} {
			o := instance.Options{
				Policy:    policy,
				ValueFill: fill,
				Territory: "NL",
				Composer:  testComposer(),
				Now:       time.Date(2021, 3, 4, 5, 6, 7, 0, time.UTC),
			}
			if fill == instance.RandomFill {
				o.ValueSource = mrand.NewPCG(1, 2)
			}
			out = append(out, o)
		}
	}
	return out
}

// TestREQ107_UnnamedArchetypeRootIsRefused is the REQ-107 check that an
// object of an archetype-root class (an ENTRY, a COMPOSITION, a PARTY,
// EHR_STATUS or EHR_ACCESS) for which the OPT names no archetype id makes
// Generate return an error wrapping ErrArchetypeIDMissing, and no root, at
// either policy and either value fill. That covers an OPT node without an
// archetype id, a required attribute the OPT leaves without children, and
// the template root. The error names the RM type and the OPT path, and it
// wraps neither ErrSlotFillUnsupported nor ErrConstraintUnsatisfiable.
//
// The control rows still generate, and their output passes the RM floor:
// an entry the OPT names, and an optional content attribute the OPT leaves
// silent, which gets no child at all.
func TestREQ107_UnnamedArchetypeRootIsRefused(t *testing.T) {
	cases := []struct {
		name string
		opt  string
		// detail is what the error says after the sentinel's own text; ""
		// marks a control row, which must generate.
		detail string
	}{
		{
			name: "content child of an unknown xsi:type",
			opt: guardOPT(guardCompositionID, guardMultiple("content", guardExistence11,
				guardChild("X_UNKNOWN", "OBSERVATION", "at0000", guardOccurrences11, ""))),
			detail: "OBSERVATION at /content[at0000]",
		},
		{
			name: "C_COMPLEX_OBJECT entry without an archetype id",
			opt: guardOPT(guardCompositionID, guardMultiple("content", guardExistence11,
				guardChild("C_COMPLEX_OBJECT", "OBSERVATION", "at0000", guardOccurrences11, ""))),
			detail: "OBSERVATION at /content[at0000]",
		},
		{
			name: "abstract ENTRY without an archetype id",
			opt: guardOPT(guardCompositionID, guardMultiple("content", guardExistence11,
				guardChild("C_COMPLEX_OBJECT", "ENTRY", "at0000", guardOccurrences11, ""))),
			detail: "ENTRY at /content[at0000]",
		},
		{
			name:   "required content the template leaves without children",
			opt:    guardOPT(guardCompositionID, guardMultiple("content", guardExistence11)),
			detail: "OBSERVATION for COMPOSITION.content at / (required, but the template names no child)",
		},
		{
			name: "required SECTION items the template leaves without children",
			opt: guardOPT(guardCompositionID, guardMultiple("content", guardExistence11,
				guardChild("C_COMPLEX_OBJECT", "SECTION", "at0001", guardOccurrences11, "",
					guardMultiple("items", guardExistence11)))),
			detail: "OBSERVATION for SECTION.items at /content[at0001] (required, but the template names no child)",
		},
		{
			name: "template root without an archetype id",
			opt: guardOPT("", guardMultiple("content", guardExistence11,
				guardChild("C_ARCHETYPE_ROOT", "OBSERVATION", "at0000", guardOccurrences11, guardObservationID))),
			detail: "COMPOSITION at / (the template root)",
		},
		{
			name: "optional content whose only child is an unnamed entry",
			opt: guardOPT(guardCompositionID, guardMultiple("content", guardExistence01,
				guardChild("C_COMPLEX_OBJECT", "OBSERVATION", "at0000", guardOccurrences01, ""))),
			detail: "OBSERVATION at /content[at0000]",
		},
		{
			name: "control: an entry the template names",
			opt: guardOPT(guardCompositionID, guardMultiple("content", guardExistence11,
				guardChild("C_ARCHETYPE_ROOT", "OBSERVATION", "at0000", guardOccurrences11, guardObservationID))),
		},
		{
			name: "control: optional content the template leaves silent",
			opt:  guardOPT(guardCompositionID, guardMultiple("content", guardExistence01)),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := compileOPTText(t, tc.opt, true)
			for _, opts := range guardOptions() {
				call := fmt.Sprintf("Generate(%v, %v)", opts.Policy, opts.ValueFill)
				out, err := instance.Generate(t.Context(), c, opts)
				if tc.detail == "" {
					if err != nil {
						t.Fatalf("%s: %v, want a root", call, err)
					}
					if r := validation.ValidateRM(out); !r.OK {
						t.Errorf("%s: ValidateRM issues %+v, want none", call, r.Issues)
					}
					continue
				}
				checkArchetypeIDMissing(t, call, err, tc.detail)
				if out != nil {
					t.Errorf("%s returned %T with the error, want no root", call, out)
				}
			}
		})
	}
}

// checkArchetypeIDMissing fails t unless err wraps ErrArchetypeIDMissing,
// wraps neither ErrSlotFillUnsupported nor ErrConstraintUnsatisfiable, and
// reads as the sentinel followed by detail.
func checkArchetypeIDMissing(t *testing.T, call string, err error, detail string) {
	t.Helper()
	if !errors.Is(err, instance.ErrArchetypeIDMissing) {
		t.Errorf("%s error = %v, want one wrapping ErrArchetypeIDMissing", call, err)
		return
	}
	for _, other := range []error{instance.ErrSlotFillUnsupported, instance.ErrConstraintUnsatisfiable} {
		if errors.Is(err, other) {
			t.Errorf("%s error = %v, want it not to wrap %v", call, err, other)
		}
	}
	got, ok := strings.CutPrefix(err.Error(), instance.ErrArchetypeIDMissing.Error()+": ")
	if !ok || got != detail {
		t.Errorf("%s error = %q, want %q followed by %q", call, err, instance.ErrArchetypeIDMissing, detail)
	}
}

// TestREQ107_UnsatisfiableConstraintIsNotArchetypeIDMissing is the REQ-107
// check that the error for a primitive constraint no value satisfies does
// not wrap ErrArchetypeIDMissing, which is kept for archetype roots.
func TestREQ107_UnsatisfiableConstraintIsNotArchetypeIDMissing(t *testing.T) {
	c := compileSyntheticOPT(t, editOPT(t, textPatternOPT, textValueListXYZ, "<pattern>XYZ(</pattern>"))
	for _, opts := range stringLeafFills() {
		_, err := instance.Generate(t.Context(), c, opts)
		if !errors.Is(err, instance.ErrConstraintUnsatisfiable) {
			t.Fatalf("Generate(%v) error = %v, want one wrapping ErrConstraintUnsatisfiable", opts.ValueFill, err)
		}
		if errors.Is(err, instance.ErrArchetypeIDMissing) {
			t.Errorf("Generate(%v) error = %v, want it not to wrap ErrArchetypeIDMissing", opts.ValueFill, err)
		}
	}
}

// bmmTypeDef is the part of a BMM property's type_def the test below reads.
type bmmTypeDef struct {
	Type              string      `json:"type"`
	RootType          string      `json:"root_type"`
	GenericParameters []string    `json:"generic_parameters"`
	TypeDef           *bmmTypeDef `json:"type_def"`
}

// names returns every type name td mentions: a container's element type, a
// generic type's root type and its parameters, and those of a nested
// type_def.
func (td *bmmTypeDef) names() []string {
	if td == nil {
		return nil
	}
	var out []string
	if td.Type != "" {
		out = append(out, td.Type)
	}
	if td.RootType != "" {
		out = append(out, td.RootType)
	}
	out = append(out, td.GenericParameters...)
	return append(out, td.TypeDef.names()...)
}

// TestREQ107_BMMMandatoryAttributesReachNoArchetypeRoot pins why the
// generator's fill for the mandatory attributes the OPT leaves out, which
// builds each value from the BMM alone and has no error to return, never
// builds an archetype root without archetype_details. No mandatory property
// anywhere in the vendored RM BMM is typed with a class that admits an
// archetype root: neither a root class itself, nor an abstract class with a
// root among its concrete descendants, such as PARTY, ACTOR, ENTRY and
// CARE_ENTRY, whose concrete descendants are all roots. A property is
// mandatory when the BMM marks it is_mandatory, or when it is a container
// whose cardinality has a lower bound of 1 or more, as rminfo reads it. If a
// BMM bump breaks this, the fill needs the same refusal as the generator's
// other paths.
func TestREQ107_BMMMandatoryAttributesReachNoArchetypeRoot(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "resources", "bmm", "openehr_rm_1.2.0.bmm.json"))
	if err != nil {
		t.Fatalf("read vendored RM BMM: %v", err)
	}
	var schema struct {
		ClassDefinitions map[string]struct {
			Ancestors  []string `json:"ancestors"`
			IsAbstract bool     `json:"is_abstract"`
			Properties map[string]struct {
				Kind        string      `json:"_type"`
				Type        string      `json:"type"`
				IsMandatory bool        `json:"is_mandatory"`
				TypeDef     *bmmTypeDef `json:"type_def"`
				Cardinality *struct {
					Lower int `json:"lower"`
				} `json:"cardinality"`
			} `json:"properties"`
		} `json:"class_definitions"`
	}
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatalf("decode vendored RM BMM: %v", err)
	}
	defs := schema.ClassDefinitions
	descendants := map[string][]string{}
	for name, c := range defs {
		for _, a := range c.Ancestors {
			descendants[a] = append(descendants[a], name)
		}
	}
	// concretes returns the concrete classes a value typed name can be: name
	// itself when the BMM defines it as concrete, and every concrete
	// descendant. A name the RM BMM does not define (a base type, a formal
	// generic parameter) has no RM descendants other than those it lists.
	concretes := func(name string) []string {
		seen := map[string]bool{}
		var out []string
		var walk func(string)
		walk = func(n string) {
			if seen[n] {
				return
			}
			seen[n] = true
			if c, ok := defs[n]; ok && !c.IsAbstract {
				out = append(out, n)
			}
			for _, d := range descendants[n] {
				walk(d)
			}
		}
		walk(name)
		slices.Sort(out)
		return out
	}
	rootsAdmitted := func(name string) []string {
		var out []string
		for _, n := range concretes(name) {
			if rmroots.IsArchetypeRoot(n) {
				out = append(out, n)
			}
		}
		return out
	}

	// The walk must see the abstract classes whose concrete descendants are
	// all archetype roots, or the check below is vacuous.
	for _, abstract := range []string{"PARTY", "ACTOR", "ENTRY", "CARE_ENTRY"} {
		all, roots := concretes(abstract), rootsAdmitted(abstract)
		if len(all) == 0 || !slices.Equal(all, roots) {
			t.Errorf("BMM %s admits %v, of which %v are archetype roots; want a non-empty set, all roots", abstract, all, roots)
		}
	}

	checked := 0
	for class, c := range defs {
		for prop, p := range c.Properties {
			container := p.Kind == "P_BMM_CONTAINER_PROPERTY"
			if !p.IsMandatory && (!container || p.Cardinality == nil || p.Cardinality.Lower < 1) {
				continue
			}
			checked++
			for _, name := range append([]string{p.Type}, p.TypeDef.names()...) {
				if name == "" {
					continue
				}
				if roots := rootsAdmitted(name); len(roots) > 0 {
					t.Errorf("BMM %s.%s is mandatory and typed %s, which admits the archetype roots %v: the generator would build one without archetype_details", class, prop, name, roots)
				}
			}
		}
	}
	if checked == 0 {
		t.Fatal("found no mandatory BMM property; want the RM's, or the check is vacuous")
	}
}
