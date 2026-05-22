package bmmgen

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/bmm"
)

// TestGoldenDataTypesQuantity regenerates the data_types_quantity
// file and diffs it against the checked-in golden.
//
// If the golden drift is intentional, update with:
//
//	cp openehr/rm/data_types_quantity_gen.go \
//	   internal/bmmgen/testdata/data_types_quantity_gen.go.golden
func TestGoldenDataTypesQuantity(t *testing.T) {
	plan, err := BuildPlan(context.Background(), "openehr_rm_1.2.0", bmm.FSResolver{Root: testResources})
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	var file *PlannedFile
	for _, f := range plan.Files {
		if f.FileBase == "data_types_quantity" {
			file = f
			break
		}
	}
	if file == nil {
		t.Fatalf("data_types_quantity file not in plan")
	}
	got, err := RenderFile(plan, file)
	if err != nil {
		t.Fatalf("RenderFile: %v", err)
	}
	goldenPath := filepath.Join("testdata", "data_types_quantity_gen.go.golden")
	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("data_types_quantity_gen.go differs from golden\n=== got ===\n%s\n=== want ===\n%s", got, want)
	}
}

// TestIdempotent regenerates the full RM into a temp dir twice and
// asserts byte-identity across runs. This catches any non-stable
// iteration order (Go map iteration is randomised).
func TestIdempotent(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()
	for _, dir := range []string{dir1, dir2} {
		if _, err := Run(Options{
			ResourcesDir: testResources,
			OutDir:       dir,
			RootID:       "openehr_rm_1.2.0",
		}); err != nil {
			t.Fatalf("Run %s: %v", dir, err)
		}
	}
	compareDirs(t, filepath.Join(dir1, "openehr", "rm"), filepath.Join(dir2, "openehr", "rm"))
}

// TestDriftDetection generates the RM into a temp dir, mutates one
// byte of a generated file, and asserts that -verify reports a
// drift on that file.
func TestDriftDetection(t *testing.T) {
	dir := t.TempDir()
	if _, err := Run(Options{
		ResourcesDir: testResources,
		OutDir:       dir,
		RootID:       "openehr_rm_1.2.0",
	}); err != nil {
		t.Fatalf("initial Run: %v", err)
	}
	target := filepath.Join(dir, "openehr", "rm", "data_types_quantity_gen.go")
	orig, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read %s: %v", target, err)
	}
	// Flip the first byte of the BMM-package marker comment.
	mutated := append([]byte{}, orig...)
	// Find a safe spot to mutate (the "// BMM package:" line).
	idx := bytes.Index(mutated, []byte("// BMM package:"))
	if idx < 0 {
		t.Fatalf("could not find marker comment in %s", target)
	}
	// idx points to "// BMM package:" — change "BMM" to "BBM" by
	// flipping the M at offset 4.
	mutated[idx+4] = 'B'
	if err := os.WriteFile(target, mutated, 0o644); err != nil {
		t.Fatalf("write mutated: %v", err)
	}
	result, err := Run(Options{
		ResourcesDir: testResources,
		OutDir:       dir,
		RootID:       "openehr_rm_1.2.0",
		Verify:       true,
	})
	if err != nil {
		t.Fatalf("verify Run: %v", err)
	}
	if len(result.Drifts) == 0 {
		t.Fatalf("expected at least one drift after mutation, got 0")
	}
	hasMutated := false
	for _, d := range result.Drifts {
		if d.Path == target {
			hasMutated = true
			break
		}
	}
	if !hasMutated {
		t.Errorf("drift not reported for %s; got %v", target, result.Drifts)
	}
}

// TestMethodStubsForDVQuantity asserts that the DV_QUANTITY class
// receives a method stub with the correct shape:
//   - PascalCase method name (e.g. IsStrictlyComparableTo)
//   - first doc line begins with the Go method name
//   - panic message uses the BMM names verbatim
//
// Phase 3 contract: every BMM function maps to one Go method stub.
func TestMethodStubsForDVQuantity(t *testing.T) {
	plan, err := BuildPlan(context.Background(), "openehr_rm_1.2.0", bmm.FSResolver{Root: testResources})
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	var file *PlannedFile
	for _, f := range plan.Files {
		if f.FileBase == "data_types_quantity" {
			file = f
			break
		}
	}
	if file == nil {
		t.Fatalf("data_types_quantity file not in plan")
	}
	got, err := RenderFile(plan, file)
	if err != nil {
		t.Fatalf("RenderFile: %v", err)
	}
	src := string(got)
	wantSnippets := []string{
		"// IsStrictlyComparableTo True if this quantity and `_other_` have the same `_units_` and also `_units_system_` if it exists.",
		"func (d *DVQuantity) IsStrictlyComparableTo(other DVOrdered) bool {",
		`panic("not implemented: DV_QUANTITY.is_strictly_comparable_to — implement in a non-generated file")`,
		// Pre/Post propagation
		"// Pre: is_strictly_comparable_to (other)",
		"// Post: Result = magnitude < other.magnitude",
		// Operator alias
		"// Aliases: + (Go does not support operator overloading)",
		"func (d *DVQuantity) Add(other DVQuantity) DVQuantity {",
	}
	for _, snip := range wantSnippets {
		if !bytes.Contains(got, []byte(snip)) {
			t.Errorf("expected snippet not found in generated output:\n  want: %s", snip)
		}
	}
	// Phase 3: emission count is non-trivial.
	if plan.MethodStubsEmitted == 0 {
		t.Errorf("expected Plan.MethodStubsEmitted > 0, got 0")
	}
	_ = src
}

// TestVerifyOnFreshTreeIsClean asserts that immediately after a
// generation the working tree passes -verify with no drifts.
func TestVerifyOnFreshTreeIsClean(t *testing.T) {
	dir := t.TempDir()
	if _, err := Run(Options{
		ResourcesDir: testResources,
		OutDir:       dir,
		RootID:       "openehr_rm_1.2.0",
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	result, err := Run(Options{
		ResourcesDir: testResources,
		OutDir:       dir,
		RootID:       "openehr_rm_1.2.0",
		Verify:       true,
	})
	if err != nil {
		t.Fatalf("verify Run: %v", err)
	}
	if len(result.Drifts) != 0 {
		t.Errorf("expected no drift, got %d", len(result.Drifts))
		for _, d := range result.Drifts {
			t.Errorf("  drift: %s (existing=%v)", d.Path, d.Existing)
		}
	}
}

// compareDirs asserts that every file in a exists in b with the
// same content, and vice versa. Recurses into sub-directories so
// targets that emit into sub-packages (e.g. openehr/rm/rminfo/) are
// covered.
func compareDirs(t *testing.T, a, b string) {
	t.Helper()
	aEntries, err := os.ReadDir(a)
	if err != nil {
		t.Fatalf("read %s: %v", a, err)
	}
	bEntries, err := os.ReadDir(b)
	if err != nil {
		t.Fatalf("read %s: %v", b, err)
	}
	if len(aEntries) != len(bEntries) {
		t.Fatalf("dir entry counts differ: %s=%d vs %s=%d", a, len(aEntries), b, len(bEntries))
	}
	for _, ae := range aEntries {
		ap := filepath.Join(a, ae.Name())
		bp := filepath.Join(b, ae.Name())
		if ae.IsDir() {
			compareDirs(t, ap, bp)
			continue
		}
		ab, err := os.ReadFile(ap)
		if err != nil {
			t.Fatalf("read %s: %v", ap, err)
		}
		bb, err := os.ReadFile(bp)
		if err != nil {
			t.Fatalf("read %s: %v", bp, err)
		}
		if !bytes.Equal(ab, bb) {
			t.Errorf("file %s differs across runs (len %d vs %d)", ae.Name(), len(ab), len(bb))
		}
	}
}
