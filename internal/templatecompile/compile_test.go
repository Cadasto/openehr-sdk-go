package templatecompile_test

import (
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/rminfo"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
	"github.com/cadasto/openehr-sdk-go/openehr/template/constraints"
)

// Phase 4 — Compile turns a parsed OPT into a walker-friendly tree.
// Spot-check identity propagation and root resolution.
func TestCompile_Identity(t *testing.T) {
	c := mustCompile(t, "vital_signs")
	if got, want := c.TemplateID(), "vital_signs"; got != want {
		t.Errorf("TemplateID = %q, want %q", got, want)
	}
	if got, want := c.Concept(), "vital_signs"; got != want {
		t.Errorf("Concept = %q, want %q", got, want)
	}
	if c.UID() == "" {
		t.Error("UID empty; expected non-empty for vital_signs")
	}
	if got, want := c.Language(), "en"; got != want {
		t.Errorf("Language = %q, want %q", got, want)
	}
	if c.Root() == nil {
		t.Fatal("Root() returned nil")
	}
	if got, want := c.Root().RMTypeName(), "COMPOSITION"; got != want {
		t.Errorf("Root RMTypeName = %q, want %q", got, want)
	}
	if c.Root().AQLPath() != "/" {
		t.Errorf("Root AQLPath = %q, want %q", c.Root().AQLPath(), "/")
	}
}

// Phase 4 — AQL path computation: single attributes get no
// predicate; multiple attributes get a predicate from the child
// (archetype id when *ArchetypeRoot, otherwise at-code).
func TestCompile_AQLPaths(t *testing.T) {
	c := mustCompile(t, "vital_signs")

	// /content[openEHR-EHR-OBSERVATION.blood_pressure.v1] — multiple
	// attribute → archetype-id predicate.
	bp, err := c.NodeAt("/content[openEHR-EHR-OBSERVATION.blood_pressure.v1]")
	if err != nil {
		t.Fatalf("NodeAt: %v", err)
	}
	if bp.RMTypeName() != "OBSERVATION" {
		t.Errorf("bp RMTypeName = %q, want OBSERVATION", bp.RMTypeName())
	}
	if bp.ArchetypeID() != "openEHR-EHR-OBSERVATION.blood_pressure.v1" {
		t.Errorf("bp ArchetypeID = %q", bp.ArchetypeID())
	}

	// /category — single attribute → no predicate.
	if _, err := c.NodeAt("/category"); err != nil {
		t.Errorf("NodeAt(/category): %v", err)
	}

	// Unknown path → ErrPathNotFound.
	if _, err := c.NodeAt("/no_such_attr"); !errors.Is(err, templatecompile.ErrPathNotFound) {
		t.Errorf("NodeAt(/no_such_attr) = %v, want ErrPathNotFound", err)
	}
}

// Phase 4 — Compiled.NodeAt is exact-match on precomputed AQL paths,
// not wire tree-walk. Predicate-less "/content" on a multi-child
// attribute resolves on the wire tree but is not indexed here.
func TestCompiled_NodeAt_RejectsPredicatelessMultiChild(t *testing.T) {
	c := mustCompile(t, "vital_signs")
	if _, err := c.NodeAt("/content"); !errors.Is(err, templatecompile.ErrPathNotFound) {
		t.Fatalf("NodeAt(/content) = %v, want ErrPathNotFound (use full AQL path)", err)
	}
}

// Phase 4 — AllByRMType + AllByNodeID indexes deliver O(1)
// reverse lookups. Sanity: at least one entry for each well-known
// term.
func TestCompile_Indexes(t *testing.T) {
	c := mustCompile(t, "vital_signs")

	obs := c.AllByRMType("OBSERVATION")
	if len(obs) == 0 {
		t.Error("AllByRMType(OBSERVATION) returned 0; vital_signs has multiple OBSERVATION roots")
	}
	at0000 := c.AllByNodeID("at0000")
	if len(at0000) == 0 {
		t.Error("AllByNodeID(at0000) returned 0; expected at least the root")
	}
}

// Phase 4 — implicit attribute injection: COMPOSITION's RM-mandatory
// fields the OPT did NOT declare (composer, language, territory) are
// injected via rminfo. OPT-declared fields (category, content) appear
// as non-implicit but still flagged Required when the BMM mandates
// them.
func TestCompile_InjectsImplicitRMAttributes(t *testing.T) {
	c := mustCompile(t, "vital_signs")
	root := c.Root()

	// vital_signs declares <attributes> for category and content
	// only — see grep on rm_attribute_name in the fixture. composer,
	// language, territory are RM-mandatory but OPT-silent, so they
	// must be injected as implicit.
	wantImplicit := []string{"composer", "language", "territory"}
	for _, name := range wantImplicit {
		a := root.Attribute(name)
		if a == nil {
			t.Errorf("Root missing RM-required attribute %q (rminfo should inject it)", name)
			continue
		}
		if !a.Required() {
			t.Errorf("Root attribute %q Required=false; want true", name)
		}
		if !a.Implicit() {
			t.Errorf("Root attribute %q Implicit=false; want true (OPT does not declare it)", name)
		}
		if a.RMTypeName() == "" {
			t.Errorf("Root attribute %q RMTypeName empty; rminfo should resolve it", name)
		}
	}

	// OPT-declared attributes are NOT flagged implicit, even when
	// RM-mandatory.
	if a := root.Attribute("category"); a == nil || a.Implicit() {
		t.Errorf("OPT-declared category should not be Implicit; got %+v", a)
	} else if !a.Required() {
		t.Errorf("OPT-declared category Required=false; want true (RM mandates it)")
	}
}

// Phase 4 — when SkipImplicitAttributes is set, the compile step
// emits only OPT-declared attributes. Useful for serialisation
// round-trip and pure-OPT introspection.
func TestCompile_SkipImplicitAttributes(t *testing.T) {
	opt, err := template.ParseFile(fixtures.TemplateOptForName("vital_signs"))
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	c, err := templatecompile.Compile(opt, templatecompile.Options{SkipImplicitAttributes: true})
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	for _, a := range c.Root().Attributes() {
		if a.Implicit() {
			t.Errorf("SkipImplicitAttributes: found implicit attribute %q on Root", a.Name())
		}
	}
}

// Phase 4 — Compile returns ErrInvalidInput for nil / rootless
// templates.
func TestCompile_InvalidInput(t *testing.T) {
	if _, err := templatecompile.Compile(nil); !errors.Is(err, templatecompile.ErrInvalidInput) {
		t.Errorf("Compile(nil) = %v, want ErrInvalidInput", err)
	}
}

// Phase 4 — terms are scoped to their enclosing archetype root.
// Looking up at0004 via the blood_pressure root must return
// "Systolic"; via heart_rate it returns "Rate". This proves the
// parent-walk lookup respects scope rather than colliding into a
// single global map.
func TestCompile_PerArchetypeRootTermScope(t *testing.T) {
	c := mustCompile(t, "vital_signs")

	bp, err := c.NodeAt("/content[openEHR-EHR-OBSERVATION.blood_pressure.v1]")
	if err != nil {
		t.Fatalf("NodeAt blood_pressure: %v", err)
	}
	bpTerm, ok := bp.Term("at0004")
	if !ok {
		t.Fatal("blood_pressure Term(at0004) missing")
	}
	if bpTerm.Items["text"] != "Systolic" {
		t.Errorf("blood_pressure at0004 text = %q, want %q", bpTerm.Items["text"], "Systolic")
	}

	hr, err := c.NodeAt("/content[openEHR-EHR-OBSERVATION.heart_rate.v1]")
	if err != nil {
		t.Fatalf("NodeAt heart_rate: %v", err)
	}
	hrTerm, ok := hr.Term("at0004")
	if !ok {
		t.Fatal("heart_rate Term(at0004) missing")
	}
	if hrTerm.Items["text"] == "Systolic" {
		t.Errorf("heart_rate at0004 leaked from blood_pressure terminology: %q", hrTerm.Items["text"])
	}
	if hrTerm.Items["text"] == bpTerm.Items["text"] {
		t.Errorf("blood_pressure and heart_rate at0004 returned same text %q (scope not respected)", hrTerm.Items["text"])
	}
}

// Phase 4 — term-binding flattening across all archetype roots.
func TestCompile_FlattenTermBindings(t *testing.T) {
	c := mustCompile(t, "vital_signs")
	bindings := c.TermBindings()
	if len(bindings) == 0 {
		t.Fatal("TermBindings empty; vital_signs has SNOMED-CT bindings")
	}
	var hasSnomed bool
	for _, b := range bindings {
		if b.Terminology == "SNOMED-CT" {
			hasSnomed = true
			break
		}
	}
	if !hasSnomed {
		t.Errorf("no SNOMED-CT binding flattened; have %d bindings", len(bindings))
	}
}

// Phase 4 — Compile honours a caller-supplied Lookup. Substituting
// a synthetic lookup with no mandatory attributes should skip
// implicit injection entirely.
func TestCompile_HonoursCustomLookup(t *testing.T) {
	opt, err := template.ParseFile(fixtures.TemplateOptForName("vital_signs"))
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	// Empty lookup → no RM info, no implicit injection.
	c, err := templatecompile.Compile(opt, templatecompile.Options{Lookup: rminfo.New(nil)})
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	for _, a := range c.Root().Attributes() {
		if a.Implicit() {
			t.Errorf("synthetic empty lookup must not inject implicit attribute %q", a.Name())
		}
	}
}

// Phase 4 — parent back-pointers wire up correctly: every non-root
// CompiledNode has a non-nil Parent reachable via Attributes.
func TestCompile_ParentBackPointers(t *testing.T) {
	c := mustCompile(t, "vital_signs")
	root := c.Root()
	if root.Parent() != nil {
		t.Errorf("Root().Parent() = %p, want nil", root.Parent())
	}
	// Walk down two levels and verify the back-pointer.
	for _, a := range root.Attributes() {
		for _, child := range a.Children() {
			if child.Parent() != root {
				t.Errorf("child %s has Parent=%p, want %p (root)", child.RMTypeName(), child.Parent(), root)
			}
		}
	}
}

// Phase 4 — *Slot nodes survive compile as IsSlot=true leaves with
// SlotIncludes preserved.
func TestCompile_SlotsPreserved(t *testing.T) {
	c := mustCompile(t, "vital_signs")
	var slots []*templatecompile.CompiledNode
	for _, n := range c.AllByRMType("CLUSTER") {
		if n.IsSlot() {
			slots = append(slots, n)
		}
	}
	if len(slots) == 0 {
		// CLUSTER slots are common in vital_signs; if none, the
		// compile collapsed something it shouldn't have.
		t.Fatal("no CLUSTER *Slot nodes found in compiled tree")
	}
	// At least one slot should carry an Includes string.
	if !slices.ContainsFunc(slots, func(n *templatecompile.CompiledNode) bool {
		return len(n.SlotIncludes()) > 0
	}) {
		t.Errorf("no slot carries Includes; expected at least one slot with includes")
	}
}

// Phase 4 / REQ-103 — Compile threads the typed
// [constraints.PrimitiveConstraint] from each wire-side ComplexObject
// onto the matching CompiledNode unchanged. Catches regressions where
// the compile step drops, mutates, or rebuilds the primitive (e.g. a
// "DeepCopy"-style refactor that loses pointer identity on
// *float64 / *int64 default fields).
//
// Uses a synthetic OPT with a C_INTEGER child so the path is
// predictable and the constraint value is small enough to compare
// via reflect.DeepEqual.
func TestCompile_ThreadsPrimitiveConstraint(t *testing.T) {
	const body = `<?xml version="1.0" encoding="UTF-8"?>
<template xmlns="http://schemas.openehr.org/v1" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <template_id><value>primitive_thread</value></template_id>
  <concept>primitive_thread</concept>
  <definition xsi:type="C_COMPLEX_OBJECT">
    <rm_type_name>COMPOSITION</rm_type_name>
    <node_id>at0000</node_id>
    <attributes xsi:type="C_MULTIPLE_ATTRIBUTE">
      <rm_attribute_name>content</rm_attribute_name>
      <children xsi:type="C_INTEGER">
        <rm_type_name>INTEGER</rm_type_name>
        <node_id>at0001</node_id>
        <range>
          <lower_included>true</lower_included>
          <upper_included>true</upper_included>
          <lower_unbounded>false</lower_unbounded>
          <upper_unbounded>false</upper_unbounded>
          <lower>0</lower>
          <upper>100</upper>
        </range>
        <list>1</list>
        <list>5</list>
        <list>10</list>
        <assumed_value>5</assumed_value>
      </children>
    </attributes>
  </definition>
</template>`
	opt, err := template.ParseOPT(strings.NewReader(body))
	if err != nil {
		t.Fatalf("ParseOPT: %v", err)
	}

	// Wire-side: locate the C_INTEGER child via the root → attribute → children chain.
	rootCO, ok := opt.Root().(*template.ComplexObject)
	if !ok {
		t.Fatalf("Root not *ComplexObject: %T", opt.Root())
	}
	wireChild, ok := rootCO.Attributes()[0].Children()[0].(*template.ComplexObject)
	if !ok {
		t.Fatalf("expected wire leaf *ComplexObject, got %T", rootCO.Attributes()[0].Children()[0])
	}
	wirePrim := wireChild.PrimitiveConstraint()
	if _, isInt := wirePrim.(constraints.CInteger); !isInt {
		t.Fatalf("wire primitive = %T, want constraints.CInteger", wirePrim)
	}

	// Compile-side: the C_INTEGER node appears under /content with its
	// at-code predicate (multi-cardinality attribute).
	c, err := templatecompile.Compile(opt)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	cn, err := c.NodeAt("/content[at0001]")
	if err != nil {
		t.Fatalf("NodeAt(/content[at0001]): %v", err)
	}
	compiledPrim := cn.PrimitiveConstraint()
	if compiledPrim == nil {
		t.Fatal("CompiledNode.PrimitiveConstraint() returned nil — Compile dropped the wire primitive")
	}
	if !reflect.DeepEqual(wirePrim, compiledPrim) {
		t.Errorf("primitive constraint mutated by Compile:\n  wire     = %#v\n  compiled = %#v", wirePrim, compiledPrim)
	}
}

// Phase 0 (REQ-102 v2) — C_MULTIPLE_ATTRIBUTE carries an AOM 1.4
// CARDINALITY block in addition to existence. The parser must lift
// its <interval> to a Multiplicity and the compile step must
// surface it via CompiledAttribute.ChildMultiplicity. C_SINGLE_ATTRIBUTE
// has no cardinality block — accessor stays nil.
func TestCompile_ChildMultiplicity(t *testing.T) {
	const body = `<?xml version="1.0"?>
<template xmlns="http://schemas.openehr.org/v1" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <template_id><value>card-test</value></template_id>
  <concept>card-test</concept>
  <definition>
    <rm_type_name>COMPOSITION</rm_type_name>
    <node_id>at0000</node_id>
    <attributes xsi:type="C_MULTIPLE_ATTRIBUTE">
      <rm_attribute_name>content</rm_attribute_name>
      <existence>
        <lower>0</lower>
        <upper>1</upper>
        <lower_unbounded>false</lower_unbounded>
        <upper_unbounded>false</upper_unbounded>
      </existence>
      <cardinality>
        <is_ordered>false</is_ordered>
        <is_unique>false</is_unique>
        <interval>
          <lower>1</lower>
          <upper_unbounded>true</upper_unbounded>
          <lower_unbounded>false</lower_unbounded>
        </interval>
      </cardinality>
      <children xsi:type="C_COMPLEX_OBJECT">
        <rm_type_name>OBSERVATION</rm_type_name>
        <node_id>at0001</node_id>
      </children>
    </attributes>
    <attributes xsi:type="C_SINGLE_ATTRIBUTE">
      <rm_attribute_name>category</rm_attribute_name>
      <existence>
        <lower>1</lower>
        <upper>1</upper>
      </existence>
    </attributes>
  </definition>
</template>`
	opt, err := template.ParseOPT(strings.NewReader(body))
	if err != nil {
		t.Fatalf("ParseOPT: %v", err)
	}
	c, err := templatecompile.Compile(opt, templatecompile.Options{SkipImplicitAttributes: true})
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	root := c.Root()

	var contentAttr, categoryAttr *templatecompile.CompiledAttribute
	for _, a := range root.Attributes() {
		switch a.Name() {
		case "content":
			contentAttr = a
		case "category":
			categoryAttr = a
		}
	}
	if contentAttr == nil || categoryAttr == nil {
		t.Fatalf("missing attrs: content=%v category=%v", contentAttr, categoryAttr)
	}

	cm := contentAttr.ChildMultiplicity()
	if cm == nil {
		t.Fatal("content.ChildMultiplicity() = nil; want lower=1 upper-unbounded")
	}
	if cm.Lower() != 1 {
		t.Errorf("content cardinality.Lower = %d, want 1", cm.Lower())
	}
	if !cm.UpperUnbounded() {
		t.Errorf("content cardinality.UpperUnbounded = false, want true")
	}

	if categoryAttr.ChildMultiplicity() != nil {
		t.Errorf("category.ChildMultiplicity() = %+v, want nil (single attr has no cardinality)", categoryAttr.ChildMultiplicity())
	}

	// Existence is independent: content's existence is 0..1.
	if exi := contentAttr.Existence(); exi == nil || exi.Lower() != 0 || exi.Upper() != 1 {
		t.Errorf("content.Existence = %+v, want [0..1]", exi)
	}
}

// Phase 0 (REQ-102 v2) — Cardinality survives compilation through
// a real fixture (vital_signs /content is lower=0,
// upper-unbounded per the OPT wire). The accessor must report the
// same interval on the multi-valued attribute, and report nil on a
// C_SINGLE_ATTRIBUTE sibling at the same root depth (/category).
func TestCompile_ChildMultiplicity_FixtureRoundTrip(t *testing.T) {
	c := mustCompile(t, "vital_signs")
	root := c.Root()
	var content, category *templatecompile.CompiledAttribute
	for _, a := range root.Attributes() {
		switch a.Name() {
		case "content":
			content = a
		case "category":
			category = a
		}
	}
	if content == nil || category == nil {
		t.Fatalf("missing attrs: content=%v category=%v", content, category)
	}
	cm := content.ChildMultiplicity()
	if cm == nil {
		t.Fatal("/content ChildMultiplicity = nil; fixture declares lower=0 upper-unbounded")
	}
	if cm.Lower() != 0 || !cm.UpperUnbounded() {
		t.Errorf("/content cardinality = {lower=%d upperUnbounded=%v}, want {0, true}", cm.Lower(), cm.UpperUnbounded())
	}
	if got := category.ChildMultiplicity(); got != nil {
		t.Errorf("/category (C_SINGLE_ATTRIBUTE) ChildMultiplicity = %+v, want nil", got)
	}
}

// Phase 4 (REQ-102 v2) — AOM 1.4 admits C_SINGLE_ATTRIBUTE with
// multiple `<children>` (alternatives) that share an AQL path.
// Compile MUST accept the collision when both candidates were
// registered under the same wire attribute. (Genuine cross-
// attribute duplicates are still rejected — see
// registerpath_test.go TestRegisterPath_DuplicateFromDifferentAttribute.)
func TestCompile_SingleAttributeAlternativesShareAQLPath(t *testing.T) {
	const body = `<?xml version="1.0"?>
<template xmlns="http://schemas.openehr.org/v1" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <template_id><value>alt-test</value></template_id>
  <concept>alt-test</concept>
  <definition>
    <rm_type_name>COMPOSITION</rm_type_name>
    <node_id>at0000</node_id>
    <attributes xsi:type="C_SINGLE_ATTRIBUTE">
      <rm_attribute_name>name</rm_attribute_name>
      <existence><lower>1</lower><upper>1</upper></existence>
      <children xsi:type="C_COMPLEX_OBJECT">
        <rm_type_name>DV_TEXT</rm_type_name>
        <node_id />
      </children>
      <children xsi:type="C_COMPLEX_OBJECT">
        <rm_type_name>DV_CODED_TEXT</rm_type_name>
        <node_id />
      </children>
    </attributes>
  </definition>
</template>`
	opt, err := template.ParseOPT(strings.NewReader(body))
	if err != nil {
		t.Fatalf("ParseOPT: %v", err)
	}
	c, err := templatecompile.Compile(opt, templatecompile.Options{SkipImplicitAttributes: true})
	if err != nil {
		t.Fatalf("Compile rejected legitimate C_SINGLE_ATTRIBUTE alternatives: %v", err)
	}
	// /name resolves to the FIRST alternative (DV_TEXT); the second
	// (DV_CODED_TEXT) is reachable only via the parent attribute's
	// Children() — the structural validator iterates that directly.
	n, err := c.NodeAt("/name")
	if err != nil {
		t.Fatalf("NodeAt(/name): %v", err)
	}
	if n.RMTypeName() != "DV_TEXT" {
		t.Errorf("first alternative at /name = %q, want DV_TEXT", n.RMTypeName())
	}
	// Parent attribute exposes both children; verify the second is
	// reachable for the walker's alternative-matching pass.
	var nameAttr *templatecompile.CompiledAttribute
	for _, a := range c.Root().Attributes() {
		if a.Name() == "name" {
			nameAttr = a
			break
		}
	}
	if nameAttr == nil {
		t.Fatal("compile dropped /name attribute")
	}
	if got := len(nameAttr.Children()); got != 2 {
		t.Errorf("nameAttr.Children() len = %d, want 2", got)
	}
}

func mustCompile(t *testing.T, slug string) *templatecompile.Compiled {
	t.Helper()
	opt, err := template.ParseFile(fixtures.TemplateOptForName(slug))
	if err != nil {
		t.Fatalf("ParseFile(%s): %v", slug, err)
	}
	c, err := templatecompile.Compile(opt)
	if err != nil {
		t.Fatalf("Compile(%s): %v", slug, err)
	}
	return c
}
