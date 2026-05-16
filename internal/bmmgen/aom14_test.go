package bmmgen

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/bmm"
)

// TestAOM14PlanFileAssignments asserts that the small load-bearing
// subset of AOM 1.4 classes lands in the expected files.
func TestAOM14PlanFileAssignments(t *testing.T) {
	plan, err := BuildPlanForTarget(context.Background(), TargetAOM14, bmm.FSResolver{Root: testResources})
	if err != nil {
		t.Fatalf("BuildPlanForTarget(AOM14): %v", err)
	}
	cases := map[string]string{
		"ARCHETYPE":          "archetype",
		"ARCHETYPE_ONTOLOGY": "archetype_ontology",
		"ARCHETYPE_SLOT":     "archetype_constraint_model",
		"C_OBJECT":           "archetype_constraint_model",
		"C_ATTRIBUTE":        "archetype_constraint_model",
		"C_COMPLEX_OBJECT":   "archetype_constraint_model",
		"ASSERTION":          "archetype_assertion",
	}
	for cls, wantFile := range cases {
		pc, ok := plan.Classes[cls]
		if !ok {
			t.Errorf("class %s not in AOM plan", cls)
			continue
		}
		if pc.External {
			t.Errorf("class %s marked External; expected owned by AOM target", cls)
		}
		if pc.FileBase != wantFile {
			t.Errorf("class %s in %s_gen.go, want %s_gen.go", cls, pc.FileBase, wantFile)
		}
	}
}

// TestAOM14ExternalBaseClasses asserts that base/RM classes
// referenced by AOM (ARCHETYPE_ID, HIER_OBJECT_ID, CODE_PHRASE,
// AUTHORED_RESOURCE, VALIDITY_KIND) are present in the plan but
// marked External so the renderer qualifies them with `rm.`.
func TestAOM14ExternalBaseClasses(t *testing.T) {
	plan, err := BuildPlanForTarget(context.Background(), TargetAOM14, bmm.FSResolver{Root: testResources})
	if err != nil {
		t.Fatalf("BuildPlanForTarget(AOM14): %v", err)
	}
	externals := []string{
		"ARCHETYPE_ID",
		"HIER_OBJECT_ID",
		"CODE_PHRASE",
		"AUTHORED_RESOURCE",
		"VALIDITY_KIND",
	}
	for _, name := range externals {
		pc, ok := plan.Classes[name]
		if !ok {
			t.Errorf("base class %s missing from AOM plan", name)
			continue
		}
		if !pc.External {
			t.Errorf("base class %s expected External=true; got false", name)
		}
	}
}

// TestAOM14ConcreteRegistry asserts the AOM concrete-class set
// contains ARCHETYPE and key C_OBJECT descendants. Abstract classes
// (C_OBJECT itself, C_PRIMITIVE) must NOT register.
func TestAOM14ConcreteRegistry(t *testing.T) {
	plan, err := BuildPlanForTarget(context.Background(), TargetAOM14, bmm.FSResolver{Root: testResources})
	if err != nil {
		t.Fatalf("BuildPlanForTarget(AOM14): %v", err)
	}
	want := map[string]bool{
		"ARCHETYPE":        false,
		"ARCHETYPE_SLOT":   false,
		"C_COMPLEX_OBJECT": false,
		"C_STRING":         false,
	}
	abstract := map[string]bool{
		"C_OBJECT":             true,
		"C_PRIMITIVE":          true,
		"C_DEFINED_OBJECT":     true,
		"C_DOMAIN_TYPE":        true,
		"C_ATTRIBUTE":          true,
		"C_REFERENCE_OBJECT":   true,
		"ARCHETYPE_CONSTRAINT": true,
	}
	for _, pc := range plan.ConcreteClasses {
		if _, in := want[pc.BMMName]; in {
			want[pc.BMMName] = true
		}
		if abstract[pc.BMMName] {
			t.Errorf("abstract class %s should NOT be in ConcreteClasses", pc.BMMName)
		}
	}
	for name, found := range want {
		if !found {
			t.Errorf("concrete class %s missing from ConcreteClasses", name)
		}
	}
}

// TestAOM14CyclicSinglePropDetection asserts that the mutual
// recursion between ARCHETYPE and ARCHETYPE_ONTOLOGY is broken by a
// pointer on at least one side. Without this the Go compiler reports
// "invalid recursive type".
func TestAOM14CyclicSinglePropDetection(t *testing.T) {
	plan, err := BuildPlanForTarget(context.Background(), TargetAOM14, bmm.FSResolver{Root: testResources})
	if err != nil {
		t.Fatalf("BuildPlanForTarget(AOM14): %v", err)
	}
	aProp := plan.CyclicSingleProps["ARCHETYPE"]["ontology"]
	oProp := plan.CyclicSingleProps["ARCHETYPE_ONTOLOGY"]["parent_archetype"]
	if !aProp && !oProp {
		t.Errorf("expected at least one of ARCHETYPE.ontology / ARCHETYPE_ONTOLOGY.parent_archetype to be marked cyclic; got CyclicSingleProps=%v", plan.CyclicSingleProps)
	}
}

// TestGoldenAOM14Archetype regenerates the archetype file from the
// AOM target and diffs it against the checked-in golden. The golden
// covers a representative subset: ARCHETYPE struct + method stubs
// with rm-qualified base type references.
//
// If the golden drift is intentional, update with:
//
//	cp openehr/aom/aom14/archetype_gen.go \
//	   internal/bmmgen/testdata/aom14_archetype_gen.go.golden
func TestGoldenAOM14Archetype(t *testing.T) {
	plan, err := BuildPlanForTarget(context.Background(), TargetAOM14, bmm.FSResolver{Root: testResources})
	if err != nil {
		t.Fatalf("BuildPlanForTarget(AOM14): %v", err)
	}
	var file *PlannedFile
	for _, f := range plan.Files {
		if f.FileBase == "archetype" {
			file = f
			break
		}
	}
	if file == nil {
		t.Fatalf("archetype file not in AOM plan")
	}
	got, err := RenderFile(plan, file)
	if err != nil {
		t.Fatalf("RenderFile: %v", err)
	}
	goldenPath := filepath.Join("testdata", "aom14_archetype_gen.go.golden")
	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("aom14 archetype_gen.go differs from golden\n=== got ===\n%s\n=== want ===\n%s", got, want)
	}
}

// TestAOM14IdempotentAndVerifyClean runs the full multi-target
// generator into a temp dir twice and asserts byte-identical output
// across runs and a clean -verify on the second run. Catches
// non-determinism specific to AOM emission (e.g. cross-target import
// emission triggered by map-iteration order).
func TestAOM14IdempotentAndVerifyClean(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()
	for _, dir := range []string{dir1, dir2} {
		if _, err := Run(Options{
			ResourcesDir: testResources,
			OutDir:       dir,
		}); err != nil {
			t.Fatalf("Run %s: %v", dir, err)
		}
	}
	compareDirs(t, filepath.Join(dir1, "openehr", "aom", "aom14"), filepath.Join(dir2, "openehr", "aom", "aom14"))

	// Verify cleanly after the second run.
	result, err := Run(Options{
		ResourcesDir: testResources,
		OutDir:       dir2,
		Verify:       true,
	})
	if err != nil {
		t.Fatalf("verify Run: %v", err)
	}
	if len(result.Drifts) != 0 {
		t.Errorf("expected no drift across both targets, got %d", len(result.Drifts))
		for _, d := range result.Drifts {
			t.Errorf("  drift: %s (existing=%v)", d.Path, d.Existing)
		}
	}
}

// TestAOM14CrossTargetReferences asserts that the generated AOM
// archetype file qualifies base-class references with the `rm.`
// package prefix and emits the corresponding import. This is the
// concrete check that Option C (one-way aom14 → rm dep) is wired.
func TestAOM14CrossTargetReferences(t *testing.T) {
	plan, err := BuildPlanForTarget(context.Background(), TargetAOM14, bmm.FSResolver{Root: testResources})
	if err != nil {
		t.Fatalf("BuildPlanForTarget(AOM14): %v", err)
	}
	var file *PlannedFile
	for _, f := range plan.Files {
		if f.FileBase == "archetype" {
			file = f
			break
		}
	}
	if file == nil {
		t.Fatalf("archetype file not in AOM plan")
	}
	got, err := RenderFile(plan, file)
	if err != nil {
		t.Fatalf("RenderFile: %v", err)
	}
	wantSnippets := []string{
		`import "github.com/cadasto/openehr-sdk-go/openehr/rm"`,
		"ArchetypeID rm.ArchetypeID",
		"UID *rm.HierObjectID",
		`panic("not implemented: ARCHETYPE.concept_name`,
	}
	for _, snip := range wantSnippets {
		if !bytes.Contains(got, []byte(snip)) {
			t.Errorf("expected snippet not found in AOM archetype output:\n  want: %s", snip)
		}
	}
}

// TestAOM14TypeRegistrySharing asserts that the AOM typereg_gen.go
// imports the SAME rm/typereg package as RM. Phase 4 design
// (Option C) reuses the single Default registry across both models.
func TestAOM14TypeRegistrySharing(t *testing.T) {
	plan, err := BuildPlanForTarget(context.Background(), TargetAOM14, bmm.FSResolver{Root: testResources})
	if err != nil {
		t.Fatalf("BuildPlanForTarget(AOM14): %v", err)
	}
	got, err := RenderTypeRegFile(plan)
	if err != nil {
		t.Fatalf("RenderTypeRegFile: %v", err)
	}
	wantSnippets := []string{
		`import "github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"`,
		`typereg.Default.Register("ARCHETYPE", func() any { return &Archetype{} })`,
		`typereg.Default.Register("ARCHETYPE_SLOT", func() any { return &ArchetypeSlot{} })`,
		`typereg.Default.Register("C_COMPLEX_OBJECT", func() any { return &CComplexObject{} })`,
	}
	for _, snip := range wantSnippets {
		if !bytes.Contains(got, []byte(snip)) {
			t.Errorf("expected snippet not found in AOM typereg output:\n  want: %s", snip)
		}
	}
}
