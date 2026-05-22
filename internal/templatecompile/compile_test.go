package templatecompile_test

import (
	"errors"
	"path/filepath"
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/rminfo"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
)

// Phase 4 — Compile turns a parsed OPT into a walker-friendly tree.
// Spot-check identity propagation and root resolution.
func TestCompile_Identity(t *testing.T) {
	c := mustCompile(t, "vital_signs.opt")
	if got, want := c.TemplateID(), "vital_signs"; got != want {
		t.Errorf("TemplateID = %q, want %q", got, want)
	}
	if got, want := c.Concept(), "vital_signs"; got != want {
		t.Errorf("Concept = %q, want %q", got, want)
	}
	if c.UID() == "" {
		t.Error("UID empty; expected non-empty for vital_signs.opt")
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
	c := mustCompile(t, "vital_signs.opt")

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

// Phase 4 — AllByRMType + AllByNodeID indexes deliver O(1)
// reverse lookups. Sanity: at least one entry for each well-known
// term.
func TestCompile_Indexes(t *testing.T) {
	c := mustCompile(t, "vital_signs.opt")

	obs := c.AllByRMType("OBSERVATION")
	if len(obs) == 0 {
		t.Error("AllByRMType(OBSERVATION) returned 0; vital_signs.opt has multiple OBSERVATION roots")
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
	c := mustCompile(t, "vital_signs.opt")
	root := c.Root()

	// vital_signs.opt declares <attributes> for category and content
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
	opt, err := template.ParseFile(filepath.Join("..", "..", "openehr", "template", "testdata", "vital_signs.opt"))
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
	c := mustCompile(t, "vital_signs.opt")

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
	c := mustCompile(t, "vital_signs.opt")
	bindings := c.TermBindings()
	if len(bindings) == 0 {
		t.Fatal("TermBindings empty; vital_signs.opt has SNOMED-CT bindings")
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
	opt, err := template.ParseFile(filepath.Join("..", "..", "openehr", "template", "testdata", "vital_signs.opt"))
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
	c := mustCompile(t, "vital_signs.opt")
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
	c := mustCompile(t, "vital_signs.opt")
	var slots []*templatecompile.CompiledNode
	for _, n := range c.AllByRMType("CLUSTER") {
		if n.IsSlot() {
			slots = append(slots, n)
		}
	}
	if len(slots) == 0 {
		// CLUSTER slots are common in vital_signs.opt; if none, the
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

func mustCompile(t *testing.T, fixture string) *templatecompile.Compiled {
	t.Helper()
	opt, err := template.ParseFile(filepath.Join("..", "..", "openehr", "template", "testdata", fixture))
	if err != nil {
		t.Fatalf("ParseFile(%s): %v", fixture, err)
	}
	c, err := templatecompile.Compile(opt)
	if err != nil {
		t.Fatalf("Compile(%s): %v", fixture, err)
	}
	return c
}
