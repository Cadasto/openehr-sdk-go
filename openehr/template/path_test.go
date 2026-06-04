package template_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// REQ-100 § Path syntax — accept valid forms.
func TestParsePath_ValidForms(t *testing.T) {
	opt := mustParseVitalSigns(t)
	cases := []struct {
		in   string
		want string // String() round-trip
	}{
		{"/", "/"},
		{"/content", "/content"},
		{"/category/defining_code", "/category/defining_code"},
		{"/content[at0001]", "/content[at0001]"},
		{"/content[openEHR-EHR-OBSERVATION.blood_pressure.v1]", "/content[openEHR-EHR-OBSERVATION.blood_pressure.v1]"},
		{"/content[at0001]/data", "/content[at0001]/data"},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			p, err := opt.ParsePath(tc.in)
			if err != nil {
				t.Fatalf("ParsePath(%q): %v", tc.in, err)
			}
			if got := p.String(); got != tc.want {
				t.Errorf("round-trip = %q, want %q", got, tc.want)
			}
		})
	}
}

// REQ-100 § Path syntax — reject malformed grammar.
func TestParsePath_RejectsMalformed(t *testing.T) {
	opt := mustParseVitalSigns(t)
	cases := []struct {
		in     string
		reason string
	}{
		{"", "empty"},
		{"content", "missing leading slash"},
		{"/content/", "trailing slash"},
		{"//content", "empty segment"},
		{"/content[", "unclosed predicate"},
		{"/content[at0001", "unclosed predicate"},
		{"/content]", "unbalanced bracket"},
		{"/content[]", "empty predicate"},
		{"/[at0001]", "predicate without name"},
		// REQ-100 explicitly rejects AQL-style predicates.
		{"/content[name='Systolic']", "AQL predicate"},
		{"/content[at0001,name='x']", "multi-predicate"},
		{"/content[@id=x]", "@ marker"},
	}
	for _, tc := range cases {
		t.Run(tc.reason, func(t *testing.T) {
			_, err := opt.ParsePath(tc.in)
			if !errors.Is(err, template.ErrPathSyntax) {
				t.Fatalf("ParsePath(%q) = %v, want ErrPathSyntax", tc.in, err)
			}
		})
	}
}

// REQ-100 § Resolution semantics — root path returns the OPT root.
func TestNodeAt_Root(t *testing.T) {
	opt := mustParseVitalSigns(t)
	p, err := opt.ParsePath("/")
	if err != nil {
		t.Fatalf("ParsePath(/): %v", err)
	}
	n, err := opt.NodeAt(p)
	if err != nil {
		t.Fatalf("NodeAt(/): %v", err)
	}
	if n != opt.Root() {
		t.Errorf("NodeAt(/) did not return the template root")
	}
}

// REQ-100 § Resolution semantics — walk into a single attribute.
func TestNodeAt_SingleAttribute(t *testing.T) {
	opt := mustParseVitalSigns(t)
	p, _ := opt.ParsePath("/content")
	n, err := opt.NodeAt(p)
	if err != nil {
		t.Fatalf("NodeAt(/content): %v", err)
	}
	// First content child is an ArchetypeRoot for the first
	// vital_signs observation slot fill.
	ar, ok := n.(*template.ArchetypeRoot)
	if !ok {
		t.Fatalf("NodeAt(/content) type = %T, want *template.ArchetypeRoot", n)
	}
	if ar.RMTypeName() != "OBSERVATION" {
		t.Errorf("RMTypeName = %q, want OBSERVATION", ar.RMTypeName())
	}
	if !strings.HasPrefix(ar.ArchetypeID(), "openEHR-EHR-OBSERVATION.") {
		t.Errorf("ArchetypeID = %q, want openEHR-EHR-OBSERVATION.* prefix", ar.ArchetypeID())
	}
}

// REQ-100 § Resolution semantics — predicate selects a specific
// archetype-root sibling (not just the first child).
func TestNodeAt_PredicateArchetypeID(t *testing.T) {
	opt := mustParseVitalSigns(t)
	// The vital_signs OPT has multiple OBSERVATION archetype roots
	// under /content. Walk through each first to pick the second
	// archetype id deterministically.
	first, _ := opt.NodeAt(mustParse(t, opt, "/content"))
	firstAR, ok := first.(*template.ArchetypeRoot)
	if !ok {
		t.Fatalf("first /content child is %T, want *template.ArchetypeRoot", first)
	}

	co, ok := opt.Root().(*template.ArchetypeRoot)
	if !ok {
		t.Fatalf("root not an *ArchetypeRoot: %T", opt.Root())
	}
	var contentAttr *template.Attribute
	for _, a := range co.Attributes() {
		if a.Name() == "content" {
			contentAttr = a
			break
		}
	}
	if contentAttr == nil || len(contentAttr.Children()) < 2 {
		t.Skip("fixture changed: need at least 2 children under /content for predicate test")
	}

	// Pick the archetype id of the second content child and look it
	// up via predicate.
	var secondAR *template.ArchetypeRoot
	for i, c := range contentAttr.Children() {
		if i == 0 {
			continue
		}
		if ar, ok := c.(*template.ArchetypeRoot); ok {
			secondAR = ar
			break
		}
	}
	if secondAR == nil {
		t.Skip("fixture changed: need another ArchetypeRoot under /content")
	}
	if secondAR.ArchetypeID() == firstAR.ArchetypeID() {
		t.Skip("fixture changed: second child has same archetype id as first")
	}

	path := "/content[" + secondAR.ArchetypeID() + "]"
	p, err := opt.ParsePath(path)
	if err != nil {
		t.Fatalf("ParsePath(%q): %v", path, err)
	}
	n, err := opt.NodeAt(p)
	if err != nil {
		t.Fatalf("NodeAt(%q): %v", path, err)
	}
	gotAR, ok := n.(*template.ArchetypeRoot)
	if !ok {
		t.Fatalf("NodeAt(%q) = %T, want *template.ArchetypeRoot", path, n)
	}
	if gotAR.ArchetypeID() != secondAR.ArchetypeID() {
		t.Errorf("predicate selected %q, want %q", gotAR.ArchetypeID(), secondAR.ArchetypeID())
	}
}

// REQ-100 § Resolution semantics — unknown attribute → ErrPathNotFound.
func TestNodeAt_UnknownAttribute(t *testing.T) {
	opt := mustParseVitalSigns(t)
	p, _ := opt.ParsePath("/this_attribute_does_not_exist")
	_, err := opt.NodeAt(p)
	if !errors.Is(err, template.ErrPathNotFound) {
		t.Fatalf("got %v, want ErrPathNotFound", err)
	}
}

// REQ-100 § Resolution semantics — unmatched predicate → ErrPathNotFound.
func TestNodeAt_UnmatchedPredicate(t *testing.T) {
	opt := mustParseVitalSigns(t)
	p, _ := opt.ParsePath("/content[at9999]")
	_, err := opt.NodeAt(p)
	if !errors.Is(err, template.ErrPathNotFound) {
		t.Fatalf("got %v, want ErrPathNotFound", err)
	}
}

// REQ-100 § Resolution semantics — descending into a leaf node returns
// ErrPathNotFound when the segment cannot be honoured.
func TestNodeAt_DeepNonexistent(t *testing.T) {
	opt := mustParseVitalSigns(t)
	p, _ := opt.ParsePath("/category/defining_code/no_such_attr")
	_, err := opt.NodeAt(p)
	if !errors.Is(err, template.ErrPathNotFound) {
		t.Fatalf("got %v, want ErrPathNotFound", err)
	}
}

// REQ-100 § Resolution semantics — predicate by at-code selects a
// specific archetype-root sibling. Complements TestNodeAt_PredicateArchetypeID
// (which uses an archetype-id predicate) by exercising the at-code
// branch of matchesPredicate.
func TestNodeAt_PredicateAtCode(t *testing.T) {
	opt := mustParseVitalSigns(t)
	// Find an at-code on a content child; vital_signs.opt's OBSERVATION
	// archetype roots each carry at0000 as their own node id, so we
	// instead descend into one and pick a deeper at-code.
	root, ok := opt.Root().(*template.ArchetypeRoot)
	if !ok {
		t.Fatalf("root = %T, want *template.ArchetypeRoot", opt.Root())
	}
	atCode, hostAttr := findAtCode(t, root)
	if atCode == "" {
		t.Skip("fixture changed: no at-code child found under root")
	}

	path := "/" + hostAttr + "[" + atCode + "]"
	p, err := opt.ParsePath(path)
	if err != nil {
		t.Fatalf("ParsePath(%q): %v", path, err)
	}
	n, err := opt.NodeAt(p)
	if err != nil {
		t.Fatalf("NodeAt(%q): %v", path, err)
	}
	if n.NodeID() != atCode {
		t.Errorf("NodeAt(%q) NodeID = %q, want %q", path, n.NodeID(), atCode)
	}
}

// REQ-100 § Resolution semantics — *Slot is a leaf in v1; an OPT
// path that attempts to descend through a slot returns
// ErrPathNotFound (the slot's child shape is opaque until slot-fill
// validation lands — REQ-104).
func TestNodeAt_CannotDescendSlot(t *testing.T) {
	opt := mustParseVitalSigns(t)
	slotAttrName, slotNodeID := findSlotUnderRoot(t, opt)
	if slotAttrName == "" {
		t.Skip("fixture changed: vital_signs.opt no longer carries a top-level *Slot")
	}
	// First, resolve the slot itself — that must succeed.
	slotPath := "/" + slotAttrName
	if slotNodeID != "" {
		slotPath += "[" + slotNodeID + "]"
	}
	p, err := opt.ParsePath(slotPath)
	if err != nil {
		t.Fatalf("ParsePath(%q): %v", slotPath, err)
	}
	slotNode, err := opt.NodeAt(p)
	if err != nil {
		t.Fatalf("NodeAt(%q): %v", slotPath, err)
	}
	if _, ok := slotNode.(*template.Slot); !ok {
		t.Fatalf("NodeAt(%q) = %T, want *template.Slot", slotPath, slotNode)
	}

	// Then attempt to descend through the slot — must fail with
	// ErrPathNotFound (the "cannot descend" branch in walkPath).
	deeper := slotPath + "/anything"
	dp, err := opt.ParsePath(deeper)
	if err != nil {
		t.Fatalf("ParsePath(%q): %v", deeper, err)
	}
	if _, err := opt.NodeAt(dp); !errors.Is(err, template.ErrPathNotFound) {
		t.Fatalf("NodeAt(%q) = %v, want ErrPathNotFound", deeper, err)
	}
}

// REQ-100 § Resolution semantics — at least one *Slot exists under the
// vital_signs fixture, and at least one carries a non-empty includes
// assertion string.
func TestParseFile_VitalSigns_ContainsSlot(t *testing.T) {
	opt := mustParseVitalSigns(t)
	slots := collectSlots(opt.Root())
	if len(slots) == 0 {
		t.Fatal("expected at least one *Slot in vital_signs.opt tree")
	}
	var withIncludes int
	for _, s := range slots {
		if len(s.Includes()) > 0 {
			withIncludes++
		}
	}
	if withIncludes == 0 {
		t.Errorf("expected at least one *Slot with non-empty Includes(); none found in %d slots", len(slots))
	}
}

// REQ-100 § Resolution semantics — clinical_note.opt resolves a deep
// /content path. Complements the identity-only check in
// TestParseFile_ClinicalNote_Identity by proving traversal works on a
// structurally distinct OPT.
func TestParseFile_ClinicalNote_Path(t *testing.T) {
	opt, err := template.ParseFile(fixtures.TemplateOptForName("clinical_note"))
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	p, err := opt.ParsePath("/content")
	if err != nil {
		t.Fatalf("ParsePath(/content): %v", err)
	}
	n, err := opt.NodeAt(p)
	if err != nil {
		t.Fatalf("NodeAt(/content): %v", err)
	}
	// First /content child in clinical_note.opt is the OBSERVATION
	// archetype root for openEHR-EHR-OBSERVATION.story.v1.
	ar, ok := n.(*template.ArchetypeRoot)
	if !ok {
		t.Fatalf("NodeAt(/content) = %T, want *template.ArchetypeRoot", n)
	}
	if ar.RMTypeName() != "OBSERVATION" {
		t.Errorf("RMTypeName = %q, want OBSERVATION", ar.RMTypeName())
	}
	if !strings.HasPrefix(ar.ArchetypeID(), "openEHR-EHR-OBSERVATION.") {
		t.Errorf("ArchetypeID = %q, want openEHR-EHR-OBSERVATION.* prefix", ar.ArchetypeID())
	}
}

// REQ-100 § Path syntax — characters after a closing ']' must be the
// segment separator '/' (or end of input). Any other character is a
// grammar error. Guards against accidental AQL-style trailing tags.
func TestParsePath_RejectsCharAfterCloseBracket(t *testing.T) {
	opt := mustParseVitalSigns(t)
	_, err := opt.ParsePath("/content[at0001]extra")
	if !errors.Is(err, template.ErrPathSyntax) {
		t.Fatalf("got %v, want ErrPathSyntax", err)
	}
}

// REQ-100 § Resolution semantics — descending past a leaf
// *ComplexObject (no attributes — e.g. an unknown xsi:type that the
// parser admits as a forward-compatible leaf) returns ErrPathNotFound,
// because the subsequent segment cannot resolve to an attribute on
// the leaf.
func TestNodeAt_LeafMidPath(t *testing.T) {
	// Synthetic OPT: the `category` attribute resolves to a leaf
	// DV_CODED_TEXT *ComplexObject (no attributes admitted under it
	// in v1). A two-segment path "/category/defining_code" must fail
	// with ErrPathNotFound at the second step.
	const body = `<?xml version="1.0"?>
<template xmlns="http://schemas.openehr.org/v1">
  <template_id><value>leaf</value></template_id>
  <concept>leaf</concept>
  <definition>
    <rm_type_name>COMPOSITION</rm_type_name>
    <node_id>at0000</node_id>
    <attributes xsi:type="C_SINGLE_ATTRIBUTE"
      xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
      <rm_attribute_name>category</rm_attribute_name>
      <children xsi:type="C_DV_CODED_TEXT">
        <rm_type_name>DV_CODED_TEXT</rm_type_name>
      </children>
    </attributes>
  </definition>
</template>`
	opt, err := template.ParseOPT(strings.NewReader(body))
	if err != nil {
		t.Fatalf("ParseOPT: %v", err)
	}
	p, err := opt.ParsePath("/category/defining_code")
	if err != nil {
		t.Fatalf("ParsePath: %v", err)
	}
	if _, err := opt.NodeAt(p); !errors.Is(err, template.ErrPathNotFound) {
		t.Fatalf("NodeAt(/category/defining_code) on leaf = %v, want ErrPathNotFound", err)
	}
}

// findAtCode returns the first (attribute-name, at-code) pair found
// among the root's direct attribute children. Returns ("", "") when
// no at-coded child exists.
func findAtCode(t *testing.T, root *template.ArchetypeRoot) (atCode, hostAttr string) {
	t.Helper()
	for _, a := range root.Attributes() {
		for _, c := range a.Children() {
			if id := c.NodeID(); strings.HasPrefix(id, "at") {
				return id, a.Name()
			}
		}
	}
	return "", ""
}

// findSlotUnderRoot returns the (attribute-name, node-id) of the first
// *Slot directly under the root's attributes. Empty strings indicate
// none found.
func findSlotUnderRoot(t *testing.T, opt *template.OperationalTemplate) (attr, nodeID string) {
	t.Helper()
	root, ok := opt.Root().(*template.ArchetypeRoot)
	if !ok {
		return "", ""
	}
	for _, a := range root.Attributes() {
		for _, c := range a.Children() {
			if s, ok := c.(*template.Slot); ok {
				return a.Name(), s.NodeID()
			}
		}
	}
	return "", ""
}

// Phase 3 — strict-mode resolution: a predicate-less segment over an
// attribute with multiple candidate children returns ErrAmbiguousPath
// rather than silently picking the first child.
func TestNodeAt_StrictPathsRejectsAmbiguousSegment(t *testing.T) {
	opt := mustParseVitalSigns(t)
	p, err := opt.ParsePath("/content")
	if err != nil {
		t.Fatalf("ParsePath: %v", err)
	}
	// Lenient (default) — picks first child without error.
	if _, err := opt.NodeAt(p); err != nil {
		t.Fatalf("lenient NodeAt(/content): %v", err)
	}
	// Strict — vital_signs.opt has multiple OBSERVATION roots under
	// /content, so a predicate-less /content must trip ErrAmbiguousPath.
	if _, err := opt.NodeAt(p, template.WithStrictPaths()); !errors.Is(err, template.ErrAmbiguousPath) {
		t.Fatalf("strict NodeAt(/content) = %v, want ErrAmbiguousPath", err)
	}
}

// Phase 3 — strict-mode resolution still works with an explicit
// predicate; ambiguity is only about predicate-less segments.
func TestNodeAt_StrictPathsAcceptsPredicate(t *testing.T) {
	opt := mustParseVitalSigns(t)
	p, err := opt.ParsePath("/content[openEHR-EHR-OBSERVATION.blood_pressure.v1]")
	if err != nil {
		t.Fatalf("ParsePath: %v", err)
	}
	if _, err := opt.NodeAt(p, template.WithStrictPaths()); err != nil {
		t.Errorf("strict NodeAt with predicate: %v", err)
	}
}

// Phase 3 — ValidatePath composes NodeAt and discards the resolved
// node. Same sentinel taxonomy.
func TestValidatePath(t *testing.T) {
	opt := mustParseVitalSigns(t)
	good, _ := opt.ParsePath("/")
	if err := opt.ValidatePath(good); err != nil {
		t.Errorf("ValidatePath(/): %v", err)
	}
	bad, _ := opt.ParsePath("/nope")
	if err := opt.ValidatePath(bad); !errors.Is(err, template.ErrPathNotFound) {
		t.Errorf("ValidatePath(/nope) = %v, want ErrPathNotFound", err)
	}
	ambiguous, _ := opt.ParsePath("/content")
	if err := opt.ValidatePath(ambiguous, template.WithStrictPaths()); !errors.Is(err, template.ErrAmbiguousPath) {
		t.Errorf("ValidatePath(/content, strict) = %v, want ErrAmbiguousPath", err)
	}
}

// Phase 3 — Multiplicity validation: parser rejects intervals with
// concrete lower > upper at parse time.
func TestParseOPT_RejectsInvertedMultiplicity(t *testing.T) {
	const body = `<?xml version="1.0"?>
<template xmlns="http://schemas.openehr.org/v1">
  <template_id><value>t</value></template_id>
  <concept>t</concept>
  <definition>
    <rm_type_name>COMPOSITION</rm_type_name>
    <node_id>at0000</node_id>
    <occurrences>
      <lower_unbounded>false</lower_unbounded>
      <upper_unbounded>false</upper_unbounded>
      <lower>5</lower>
      <upper>2</upper>
    </occurrences>
  </definition>
</template>`
	_, err := template.ParseOPT(strings.NewReader(body))
	if !errors.Is(err, template.ErrInvalidOPT) {
		t.Fatalf("ParseOPT got %v, want ErrInvalidOPT for inverted multiplicity", err)
	}
}

// Phase 3 — Cardinality.String / IsValid sanity.
func TestCardinality_StringAndIsValid(t *testing.T) {
	if got := template.Single.String(); got != "single" {
		t.Errorf("Single.String() = %q, want %q", got, "single")
	}
	if got := template.Multiple.String(); got != "multiple" {
		t.Errorf("Multiple.String() = %q, want %q", got, "multiple")
	}
	if !template.Single.IsValid() || !template.Multiple.IsValid() {
		t.Errorf("Single and Multiple must report IsValid()=true")
	}
	if template.Cardinality(99).IsValid() {
		t.Errorf("out-of-range Cardinality(99) must report IsValid()=false")
	}
	if got := template.Cardinality(99).String(); !strings.Contains(got, "99") {
		t.Errorf("Cardinality(99).String() = %q, want token containing 99", got)
	}
}

// Phase 3 — ObjectNode supertype: both *ComplexObject and
// *ArchetypeRoot satisfy ObjectNode; *Slot and *Attribute do not.
// Walker code can use a single `case template.ObjectNode:` arm
// instead of listing the two concrete types.
func TestObjectNode_SatisfiedByObjectNodes(t *testing.T) {
	opt := mustParseVitalSigns(t)
	root := opt.Root()
	if _, ok := root.(template.ObjectNode); !ok {
		t.Errorf("root %T does not satisfy ObjectNode", root)
	}
	// Find a *Slot in the tree and assert it does NOT satisfy
	// ObjectNode (it is a leaf).
	slots := collectSlots(opt.Root())
	if len(slots) == 0 {
		t.Skip("fixture has no *Slot to negative-check")
	}
	var slot template.Node = slots[0]
	if _, ok := slot.(template.ObjectNode); ok {
		t.Errorf("*Slot must NOT satisfy ObjectNode")
	}
}

// Phase 4 prep — Terms() captures the <term_definitions code="..."> blocks
// nested under a C_ARCHETYPE_ROOT. Spot-check a known at-code from
// the blood-pressure archetype root in vital_signs.opt.
func TestArchetypeRoot_TermsCaptured(t *testing.T) {
	opt := mustParseVitalSigns(t)
	root, ok := opt.Root().(*template.ArchetypeRoot)
	if !ok {
		t.Fatalf("root = %T, want *template.ArchetypeRoot", opt.Root())
	}
	bp := findArchetypeRoot(t, root, "openEHR-EHR-OBSERVATION.blood_pressure.v1")
	if bp == nil {
		t.Fatal("blood_pressure archetype root not found under /content")
	}
	terms := bp.Terms()
	if len(terms) == 0 {
		t.Fatal("Terms() empty; expected at least one term definition on blood_pressure root")
	}
	// at0004 = "Systolic" per the vendored fixture.
	t4, ok := bp.Term("at0004")
	if !ok {
		t.Fatal("Term(at0004) missing on blood_pressure root")
	}
	if t4.Items["text"] != "Systolic" {
		t.Errorf("at0004.text = %q, want %q", t4.Items["text"], "Systolic")
	}
	if t4.Items["description"] == "" {
		t.Errorf("at0004.description empty; expected a description string")
	}
}

// Phase 4 prep — TermBindings() captures the <term_bindings
// terminology="..."> blocks. The blood-pressure fixture binds at-codes
// to SNOMED-CT codes; at least one binding must surface.
func TestArchetypeRoot_TermBindingsCaptured(t *testing.T) {
	opt := mustParseVitalSigns(t)
	root, ok := opt.Root().(*template.ArchetypeRoot)
	if !ok {
		t.Fatalf("root = %T, want *template.ArchetypeRoot", opt.Root())
	}
	bp := findArchetypeRoot(t, root, "openEHR-EHR-OBSERVATION.blood_pressure.v1")
	if bp == nil {
		t.Fatal("blood_pressure archetype root not found under /content")
	}
	bindings := bp.TermBindings()
	if len(bindings) == 0 {
		t.Fatal("TermBindings() empty; expected at least one binding on blood_pressure root")
	}
	// The fixture pins SNOMED-CT bindings on at0013 and similar; assert
	// shape rather than exact at-code so the test survives fixture
	// re-flow.
	var snomed *template.TermBinding
	for i := range bindings {
		if bindings[i].Terminology == "SNOMED-CT" {
			snomed = &bindings[i]
			break
		}
	}
	if snomed == nil {
		t.Fatalf("no SNOMED-CT binding found; bindings = %+v", bindings)
	}
	if snomed.NodeOrPath == "" {
		t.Errorf("SNOMED-CT binding missing NodeOrPath: %+v", snomed)
	}
	if snomed.Target.CodeString == "" {
		t.Errorf("SNOMED-CT binding missing Target.CodeString: %+v", snomed)
	}
}

// Phase 4 prep — Terms() returns a defensive copy: caller mutation
// must not leak into the underlying ArchetypeRoot.
func TestArchetypeRoot_TermsImmutable(t *testing.T) {
	opt := mustParseVitalSigns(t)
	root, ok := opt.Root().(*template.ArchetypeRoot)
	if !ok {
		t.Fatalf("root = %T, want *template.ArchetypeRoot", opt.Root())
	}
	bp := findArchetypeRoot(t, root, "openEHR-EHR-OBSERVATION.blood_pressure.v1")
	if bp == nil {
		t.Skip("fixture changed: blood_pressure root missing")
	}
	first := bp.Terms()
	original := len(first)
	delete(first, "at0004")
	if got := len(bp.Terms()); got != original {
		t.Errorf("Terms() shared underlying map: %d after delete, %d originally", got, original)
	}
}

// findArchetypeRoot returns the first *ArchetypeRoot descendant of
// root whose ArchetypeID matches archetypeID. Returns nil when no
// match exists.
func findArchetypeRoot(t *testing.T, root *template.ArchetypeRoot, archetypeID string) *template.ArchetypeRoot {
	t.Helper()
	var found *template.ArchetypeRoot
	var visit func(n template.Node)
	visit = func(n template.Node) {
		if found != nil {
			return
		}
		switch v := n.(type) {
		case *template.ArchetypeRoot:
			if v.ArchetypeID() == archetypeID {
				found = v
				return
			}
			for _, a := range v.Attributes() {
				for _, c := range a.Children() {
					visit(c)
				}
			}
		case *template.ComplexObject:
			for _, a := range v.Attributes() {
				for _, c := range a.Children() {
					visit(c)
				}
			}
		}
	}
	for _, a := range root.Attributes() {
		for _, c := range a.Children() {
			visit(c)
		}
	}
	return found
}

// collectSlots returns every *Slot reachable from n via attribute
// children, depth-first.
func collectSlots(n template.Node) []*template.Slot {
	var out []*template.Slot
	var visit func(template.Node)
	visit = func(n template.Node) {
		switch v := n.(type) {
		case *template.Slot:
			out = append(out, v)
		case *template.ArchetypeRoot:
			for _, a := range v.Attributes() {
				for _, c := range a.Children() {
					visit(c)
				}
			}
		case *template.ComplexObject:
			for _, a := range v.Attributes() {
				for _, c := range a.Children() {
					visit(c)
				}
			}
		}
	}
	visit(n)
	return out
}

// --- helpers ------------------------------------------------------------

func mustParseVitalSigns(t *testing.T) *template.OperationalTemplate {
	t.Helper()
	opt, err := template.ParseFile(fixtures.TemplateOptForName("vital_signs"))
	if err != nil {
		t.Fatalf("load vital_signs.opt: %v", err)
	}
	return opt
}

func mustParse(t *testing.T, opt *template.OperationalTemplate, path string) template.Path {
	t.Helper()
	p, err := opt.ParsePath(path)
	if err != nil {
		t.Fatalf("ParsePath(%q): %v", path, err)
	}
	return p
}
