package instance_test

import (
	"errors"
	"fmt"
	mrand "math/rand/v2"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/cadasto/openehr-sdk-go/internal/rmroots"
	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/rminfo"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// The OPTs below are small templates for the REQ-107 rule on archetype
// roots: an object of a class that is always an archetype root needs an
// archetype id from the template, or the generator refuses it. Most are
// COMPOSITION templates that differ only in their root's archetype id and in
// their content attribute.

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

// guardRootOPT is a template whose root is of rmType and holds attrs. An
// empty rootArchetypeID leaves the archetype id off the template root.
func guardRootOPT(rmType, rootArchetypeID string, attrs ...string) string {
	id := ""
	if rootArchetypeID != "" {
		id = `<archetype_id><value>` + rootArchetypeID + `</value></archetype_id>`
	}
	return `<?xml version="1.0" encoding="utf-8"?>
<template xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xmlns="http://schemas.openehr.org/v1">
<language><terminology_id><value>ISO_639-1</value></terminology_id><code_string>en</code_string></language>
<template_id><value>archetype_root_guard</value></template_id><concept>archetype_root_guard</concept>
<definition><rm_type_name>` + rmType + `</rm_type_name><node_id>at0000</node_id>` + strings.Join(attrs, "") + id + `</definition>
</template>`
}

// guardOPT is a COMPOSITION template with one content attribute. An empty
// rootArchetypeID leaves the archetype id off the template root.
func guardOPT(rootArchetypeID, content string) string {
	return guardRootOPT("COMPOSITION", rootArchetypeID, content)
}

// guardMultiple is a C_MULTIPLE_ATTRIBUTE called name with the given
// existence and children, and no lower bound on its cardinality.
func guardMultiple(name, existence string, children ...string) string {
	return `<attributes xsi:type="C_MULTIPLE_ATTRIBUTE"><rm_attribute_name>` + name + `</rm_attribute_name>` +
		existence + guardOpenCardinality + strings.Join(children, "") + `</attributes>`
}

// guardSingle is a C_SINGLE_ATTRIBUTE called name with the given existence
// and children.
func guardSingle(name, existence string, children ...string) string {
	return `<attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>` + name + `</rm_attribute_name>` +
		existence + strings.Join(children, "") + `</attributes>`
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

// guardRootClasses are the classes whose objects are always archetype
// roots, and guardEntryClasses the concrete ENTRY classes among them. Both
// lists are checked against internal/rmroots and rminfo, so a class the
// shared list gains or loses fails the test until the lists follow.
var (
	guardRootClasses = []string{
		"ACTION", "ADMIN_ENTRY", "AGENT", "COMPOSITION", "EHR_ACCESS", "EHR_STATUS", "EVALUATION",
		"GROUP", "INSTRUCTION", "OBSERVATION", "ORGANISATION", "PERSON", "ROLE",
	}
	guardEntryClasses = []string{"ACTION", "ADMIN_ENTRY", "EVALUATION", "INSTRUCTION", "OBSERVATION"}
)

// checkGuardClasses fails t unless guardRootClasses names exactly the
// classes rminfo knows that rmroots.IsArchetypeRoot accepts, and
// guardEntryClasses exactly the concrete descendants of ENTRY.
func checkGuardClasses(t *testing.T) {
	t.Helper()
	var roots []string
	for _, name := range rminfo.Default.KnownRMTypes() {
		if rmroots.IsArchetypeRoot(name) {
			roots = append(roots, name)
		}
	}
	if !slices.Equal(roots, guardRootClasses) {
		t.Fatalf("archetype-root classes rminfo knows = %v, want %v", roots, guardRootClasses)
	}
	hierarchy, ok := rminfo.Default.(rminfo.Hierarchy)
	if !ok {
		t.Fatal("rminfo.Default does not answer class-hierarchy questions")
	}
	if entries, _ := hierarchy.ConcreteDescendants("ENTRY"); !slices.Equal(entries, guardEntryClasses) {
		t.Fatalf("concrete ENTRY classes = %v, want %v", entries, guardEntryClasses)
	}
}

// guardCase is one row of TestREQ107_UnnamedArchetypeRootIsRefused.
type guardCase struct {
	name string
	opt  string
	// detail is what the error says after the sentinel's own text; ""
	// marks a control row, which must generate.
	detail string
	// check, when set, runs on a control row's output after the RM-floor
	// check.
	check func(t *testing.T, call string, out any)
}

// TestREQ107_UnnamedArchetypeRootIsRefused is the REQ-107 check that an
// object of an archetype-root class (an ENTRY, a COMPOSITION, a PARTY,
// EHR_STATUS or EHR_ACCESS) for which the OPT names no archetype id makes
// Generate return an error wrapping ErrArchetypeIDMissing, and no root, at
// either policy and either value fill. That covers an OPT node without an
// archetype id, a required attribute the OPT leaves without children, and
// the template root. It runs every archetype-root class as the template
// root and every ENTRY class as a content child. The error names the RM
// type and the OPT path, and it wraps neither ErrSlotFillUnsupported nor
// ErrConstraintUnsatisfiable.
//
// The control rows still generate, and their output passes the RM floor:
// an entry the OPT names, and an optional content attribute the OPT leaves
// silent, written as multiple or as single, which gets no child at all. A
// required slot is no such node: the slot-fill rule stamps it with the
// RM-type-prefix archetype id instead, and the fill passes the floor too.
func TestREQ107_UnnamedArchetypeRootIsRefused(t *testing.T) {
	checkGuardClasses(t)
	cases := []guardCase{
		{
			name: "content child of an unknown xsi:type",
			opt: guardOPT(guardCompositionID, guardMultiple("content", guardExistence11,
				guardChild("X_UNKNOWN", "OBSERVATION", "at0000", guardOccurrences11, ""))),
			detail: "OBSERVATION at /content[at0000]",
		},
		{
			name: "archetype id under an xsi:type the parser does not recognise",
			opt: guardOPT(guardCompositionID, guardMultiple("content", guardExistence11,
				guardChild("ARCHETYPE_ROOT", "OBSERVATION", "at0000", guardOccurrences11, guardObservationID))),
			detail: "OBSERVATION at /content[at0000]",
		},
		{
			name: "abstract CONTENT_ITEM without an archetype id",
			opt: guardOPT(guardCompositionID, guardMultiple("content", guardExistence11,
				guardChild("C_COMPLEX_OBJECT", "CONTENT_ITEM", "at0001", guardOccurrences11, ""))),
			detail: "CONTENT_ITEM (built as OBSERVATION) at /content[at0001]",
		},
		{
			name:   "required content the template leaves without children",
			opt:    guardOPT(guardCompositionID, guardMultiple("content", guardExistence11)),
			detail: "OBSERVATION for COMPOSITION.content at / (required, but the template names no child)",
		},
		{
			name:   "required content written as a single attribute without children",
			opt:    guardOPT(guardCompositionID, guardSingle("content", guardExistence11)),
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
		{
			name: "control: optional content written as a single attribute and left silent",
			opt:  guardOPT(guardCompositionID, guardSingle("content", guardExistence01)),
		},
		{
			name: "control: a required slot of an entry class with no includes",
			opt: guardOPT(guardCompositionID, guardMultiple("content", guardExistence11,
				guardChild("ARCHETYPE_SLOT", "OBSERVATION", "at0000", guardOccurrences11, ""))),
			check: func(t *testing.T, call string, out any) {
				t.Helper()
				checkSlotFallbackStamp(t, call, firstContent(t, call, out), "OBSERVATION")
			},
		},
	}
	for _, class := range guardRootClasses {
		cases = append(cases, guardCase{
			name:   "template root " + class + " without an archetype id",
			opt:    guardRootOPT(class, ""),
			detail: class + " at / (the template root)",
		})
	}
	for _, class := range append(slices.Clone(guardEntryClasses), "ENTRY", "CARE_ENTRY") {
		detail := class + " at /content[at0000]"
		if class == "ENTRY" || class == "CARE_ENTRY" {
			detail = class + " (built as OBSERVATION) at /content[at0000]"
		}
		cases = append(cases, guardCase{
			name: "content child " + class + " without an archetype id",
			opt: guardOPT(guardCompositionID, guardMultiple("content", guardExistence11,
				guardChild("C_COMPLEX_OBJECT", class, "at0000", guardOccurrences11, ""))),
			detail: detail,
		})
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
					if tc.check != nil {
						tc.check(t, call, out)
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

// TestREQ107_UnnamedNonRootTemplateRootHasNoArchetypeDetails is the REQ-107
// check that a template root of a class that is not always an archetype
// root, for which the OPT names no archetype id, gets no archetype_details:
// an ARCHETYPED needs an archetype id, the generator invents none, and these
// classes may leave the attribute out. The output passes the RM floor and
// the template validator at either policy and either value fill.
func TestREQ107_UnnamedNonRootTemplateRootHasNoArchetypeDetails(t *testing.T) {
	for _, class := range []string{"CLUSTER", "SECTION", "GENERIC_ENTRY", "ELEMENT", "ITEM_TREE"} {
		t.Run(class, func(t *testing.T) {
			if rmroots.IsArchetypeRoot(class) {
				t.Fatalf("%s is an archetype-root class; want one that may leave archetype_details out", class)
			}
			c := compileOPTText(t, guardRootOPT(class, ""), true)
			for _, opts := range guardOptions() {
				call := fmt.Sprintf("Generate(%v, %v)", opts.Policy, opts.ValueFill)
				out, err := instance.Generate(t.Context(), c, opts)
				if err != nil {
					t.Fatalf("%s: %v, want a root", call, err)
				}
				root, ok := out.(rm.Locatable)
				if !ok {
					t.Fatalf("%s returned %T, want a LOCATABLE", call, out)
				}
				if ad := root.GetArchetypeDetails(); ad != nil {
					t.Errorf("%s: archetype_details = %+v, want none", call, *ad)
				}
				if r := validation.ValidateRM(out); !r.OK {
					t.Errorf("%s: ValidateRM issues %+v, want none", call, r.Issues)
				}
				if r := validation.Validate(out, c); !r.OK {
					t.Errorf("%s: Validate issues %+v, want none", call, r.Issues)
				}
			}
		})
	}
}

// TestREQ107_NamedTemplateRootPassesTheFloor is the REQ-107 check that a
// template root of every archetype-root class, for which the template names
// an archetype id, generates output that passes the RM floor and the
// template validator at either policy and either value fill. The template
// says nothing below the root, so every RM-mandatory attribute of the class
// comes from the generator's own defaults: a party's identities, and a
// ROLE's performer.
func TestREQ107_NamedTemplateRootPassesTheFloor(t *testing.T) {
	checkGuardClasses(t)
	for _, class := range guardRootClasses {
		t.Run(class, func(t *testing.T) {
			c := compileOPTText(t, guardRootOPT(class, "openEHR-EHR-"+class+".example.v1"), true)
			for _, opts := range guardOptions() {
				call := fmt.Sprintf("Generate(%v, %v)", opts.Policy, opts.ValueFill)
				out, err := instance.Generate(t.Context(), c, opts)
				if err != nil {
					t.Fatalf("%s: %v, want a root", call, err)
				}
				if r := validation.ValidateRM(out); !r.OK {
					t.Errorf("%s: ValidateRM issues %+v, want none", call, r.Issues)
				}
				if r := validation.Validate(out, c); !r.OK {
					t.Errorf("%s: Validate issues %+v, want none", call, r.Issues)
				}
			}
		})
	}
}

// TestREQ107_RequiredSlotFillPassesTheFloor is the REQ-107 check that a
// required slot with no includes, which the slot-fill rule stamps with the
// RM-type-prefix archetype id, generates a fill that passes the RM floor at
// either policy and either value fill. The fill's body is not in the
// template, so the RM-mandatory attributes of its class come from the
// generator's defaults: an ENTRY's language, encoding and subject, and
// each class's own, such as an OBSERVATION's data or an ACTION's time,
// ism_transition and description. A slot of an abstract class (ENTRY,
// CARE_ENTRY, CONTENT_ITEM) is stamped with its declared type and built as
// OBSERVATION, so its defaults are OBSERVATION's. A SECTION, which has
// none, and a CLUSTER, whose one is items, are controls. A CLUSTER fill
// gets exactly one item, whether the top-up of a required items list or
// the RM fill of an optional one makes it.
func TestREQ107_RequiredSlotFillPassesTheFloor(t *testing.T) {
	type slotCase struct {
		name   string
		opt    string
		rmType string
		// fill returns the slot fill in the output.
		fill func(t *testing.T, call string, out any) any
	}
	var cases []slotCase
	for _, class := range []string{
		"OBSERVATION", "EVALUATION", "INSTRUCTION", "ACTION", "ADMIN_ENTRY", "GENERIC_ENTRY", "SECTION",
		"ENTRY", "CARE_ENTRY", "CONTENT_ITEM",
	} {
		cases = append(cases, slotCase{
			name: "content slot of " + class,
			opt: guardOPT(guardCompositionID, guardMultiple("content", guardExistence11,
				guardChild("ARCHETYPE_SLOT", class, "at0000", guardOccurrences11, ""))),
			rmType: class,
			fill:   firstContent,
		})
	}
	cases = append(cases,
		slotCase{
			name: "required items slot of CLUSTER in a CLUSTER",
			opt: guardRootOPT("CLUSTER", "", guardMultiple("items", guardExistence11,
				guardChild("ARCHETYPE_SLOT", "CLUSTER", "at0001", guardOccurrences11, ""))),
			rmType: "CLUSTER",
			fill: func(t *testing.T, call string, out any) any {
				t.Helper()
				root, ok := out.(*rm.Cluster)
				if !ok || len(root.Items) != 1 {
					t.Fatalf("%s returned %T %+v, want a CLUSTER with one item", call, out, out)
				}
				return root.Items[0]
			},
		},
		slotCase{
			name: "optional items slot of CLUSTER in an ITEM_TREE",
			opt: guardRootOPT("ITEM_TREE", "", guardMultiple("items", guardExistence01,
				guardChild("ARCHETYPE_SLOT", "CLUSTER", "at0001", guardOccurrences01, ""))),
			rmType: "CLUSTER",
			fill: func(t *testing.T, call string, out any) any {
				t.Helper()
				root, ok := out.(*rm.ItemTree)
				if !ok || len(root.Items) != 1 {
					t.Fatalf("%s returned %T %+v, want an ITEM_TREE with one item", call, out, out)
				}
				return root.Items[0]
			},
		},
	)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := compileOPTText(t, tc.opt, true)
			for _, opts := range guardOptions() {
				call := fmt.Sprintf("Generate(%v, %v)", opts.Policy, opts.ValueFill)
				out, err := instance.Generate(t.Context(), c, opts)
				if err != nil {
					t.Fatalf("%s: %v, want a root", call, err)
				}
				fill := tc.fill(t, call, out)
				checkSlotFallbackStamp(t, call, fill, tc.rmType)
				if cl, ok := fill.(*rm.Cluster); ok {
					checkClusterFillItem(t, call, cl)
				}
				if r := validation.ValidateRM(out); !r.OK {
					t.Errorf("%s: ValidateRM issues %+v, want none", call, r.Issues)
				}
			}
		})
	}
}

// checkClusterFillItem fails t unless the CLUSTER slot fill cl holds exactly
// one item, the placeholder ELEMENT (node id at0000, name "element") the
// generator puts in an items list the OPT does not describe. A second item,
// or an ELEMENT named after its RM type, would be the BMM fill's default
// for CLUSTER.items, which the slot fill must not get as well.
func checkClusterFillItem(t *testing.T, call string, cl *rm.Cluster) {
	t.Helper()
	if len(cl.Items) != 1 {
		t.Errorf("%s: the CLUSTER fill has %d items, want 1", call, len(cl.Items))
		return
	}
	el, ok := cl.Items[0].(*rm.Element)
	if !ok {
		t.Errorf("%s: the CLUSTER fill's item is %T, want *rm.Element", call, cl.Items[0])
		return
	}
	name := ""
	if el.Name != nil {
		name = el.Name.GetValue()
	}
	if el.ArchetypeNodeID != "at0000" || name != "element" {
		t.Errorf("%s: the CLUSTER fill's item is %s named %q, want the placeholder at0000 named \"element\"", call, el.ArchetypeNodeID, name)
	}
}

// firstContent returns the first content item of out, which must be a
// COMPOSITION with content.
func firstContent(t *testing.T, call string, out any) any {
	t.Helper()
	comp, err := instance.AsComposition(out)
	if err != nil {
		t.Fatalf("%s: AsComposition: %v", call, err)
	}
	if len(comp.Content) == 0 {
		t.Fatalf("%s: content is empty, want the stamped slot fill", call)
	}
	return comp.Content[0]
}

// checkSlotFallbackStamp fails t unless fill is a LOCATABLE that carries the
// RM-type-prefix archetype id the slot-fill rule gives a slot of rmType
// without parsed includes, as its node id and in its archetype_details.
func checkSlotFallbackStamp(t *testing.T, call string, fill any, rmType string) {
	t.Helper()
	want := "openEHR-EHR-" + rmType + ".example.v1"
	item, ok := fill.(rm.Locatable)
	if !ok {
		t.Fatalf("%s: the slot fill is %T, want a LOCATABLE", call, fill)
	}
	if got := item.GetArchetypeNodeID(); got != want {
		t.Errorf("%s: the slot fill's archetype_node_id = %q, want %q", call, got, want)
	}
	ad := item.GetArchetypeDetails()
	if ad == nil || ad.ArchetypeID.Value != want {
		t.Errorf("%s: the slot fill's archetype_details = %+v, want archetype_id %q", call, ad, want)
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

// TestREQ107_BMMMandatoryAttributesReachNoArchetypeRoot pins why the
// generator's fill for the mandatory attributes the OPT leaves out, which
// builds each value from rminfo alone and has no error to return, never
// builds an archetype root without archetype_details. It reads the inputs
// that fill reads: for every class rminfo knows, no attribute rminfo marks
// required is typed with a class that admits an archetype root, neither a
// root class itself nor an abstract class with a root among its concrete
// descendants, such as PARTY, ACTOR, ENTRY and CARE_ENTRY, whose concrete
// descendants are all roots. If a BMM bump breaks this, the fill needs the
// same refusal as the generator's other paths.
func TestREQ107_BMMMandatoryAttributesReachNoArchetypeRoot(t *testing.T) {
	hierarchy, ok := rminfo.Default.(rminfo.Hierarchy)
	if !ok {
		t.Fatal("rminfo.Default does not answer class-hierarchy questions")
	}
	// rootsAdmitted returns the archetype-root classes a value typed rmType
	// can be. A type the hierarchy does not know (a primitive, a formal
	// generic parameter) stands for itself.
	rootsAdmitted := func(rmType string) []string {
		admitted, known := hierarchy.ConcreteDescendants(rmType)
		if !known {
			admitted = []string{rmType}
		}
		var roots []string
		for _, name := range admitted {
			if rmroots.IsArchetypeRoot(name) {
				roots = append(roots, name)
			}
		}
		return roots
	}

	// The check must see the abstract classes whose concrete descendants are
	// all archetype roots, or it is vacuous.
	for _, class := range []string{"PARTY", "ACTOR", "ENTRY", "CARE_ENTRY"} {
		abstract, _ := hierarchy.IsAbstract(class)
		all, _ := hierarchy.ConcreteDescendants(class)
		if roots := rootsAdmitted(class); !abstract || len(all) == 0 || !slices.Equal(all, roots) {
			t.Errorf("rminfo %s: abstract %v, admits %v, of which %v are archetype roots; want an abstract class whose concrete descendants are all roots", class, abstract, all, roots)
		}
	}

	checked := 0
	for _, class := range rminfo.Default.KnownRMTypes() {
		for _, attr := range rminfo.Default.RequiredAttributes(class) {
			rmType, ok := rminfo.Default.AttributeRMType(class, attr)
			if !ok {
				t.Errorf("rminfo marks %s.%s required but gives it no RM type", class, attr)
				continue
			}
			checked++
			if roots := rootsAdmitted(rmType); len(roots) > 0 {
				t.Errorf("rminfo marks %s.%s required and types it %s, which admits the archetype roots %v: the generator would build one without archetype_details", class, attr, rmType, roots)
			}
		}
	}
	if checked == 0 {
		t.Fatal("rminfo marks no attribute required; want the RM's, or the check is vacuous")
	}
}
