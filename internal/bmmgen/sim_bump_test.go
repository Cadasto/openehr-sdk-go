package bmmgen

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/bmmdiff"
	"github.com/cadasto/openehr-sdk-go/openehr/bmm"
)

// TestSimulatedVersionBump exercises Phase 5's "Definition of done":
//
//	A simulated version bump (e.g. fake openehr_rm_1.2.1.bmm.json with
//	one added property) regenerates without manual intervention; the
//	diff in openehr/rm/ is small and reviewable; tests pass; CHANGELOG
//	entry template is auto-suggested.
//
// Procedure:
//  1. Stage a temp resources dir containing the original BMM files
//     plus a synthesised `openehr_rm_1.2.1.bmm.json` whose only
//     semantic delta vs the 1.2.0 baseline is: DV_QUANTITY gains an
//     optional SingleProperty `test_property` of type String.
//  2. Generate into a fresh out dir using -target rm.
//  3. Assert: data_types_quantity_gen.go now contains a
//     TestProperty *string `json:"test_property,omitempty"` field on
//     DV_QUANTITY; typereg_gen.go is unchanged vs the baseline (no
//     new class registered).
//  4. Run bmmdiff.Diff(old, new) and assert the CHANGELOG suggestion
//     is the expected one-liner. Print it via t.Logf so a human
//     running the test sees the suggestion.
func TestSimulatedVersionBump(t *testing.T) {
	// --- 1. Stage the synthetic 1.2.1 resources dir.
	stage := t.TempDir()
	copyResource(t, "openehr_base_1.3.0.bmm.json", stage)

	// Read 1.2.0, mutate JSON to derive 1.2.1.
	srcPath := filepath.Join(testResources, "openehr_rm_1.2.0.bmm.json")
	rawSrc, err := os.ReadFile(srcPath)
	if err != nil {
		t.Fatalf("read base RM: %v", err)
	}
	bumped, err := bumpRMWithTestProperty(rawSrc)
	if err != nil {
		t.Fatalf("bumpRMWithTestProperty: %v", err)
	}
	if err := os.WriteFile(filepath.Join(stage, "openehr_rm_1.2.1.bmm.json"), bumped, 0o644); err != nil {
		t.Fatalf("write bumped: %v", err)
	}

	// --- 2. Generate into a fresh out dir using only the RM target.
	outDir := t.TempDir()
	result, err := Run(Options{
		ResourcesDir: stage,
		OutDir:       outDir,
		Targets:      []Target{TargetRM},
		RootID:       "openehr_rm_1.2.1",
	})
	if err != nil {
		t.Fatalf("Run regen: %v", err)
	}
	if len(result.Files) == 0 {
		t.Fatalf("no files emitted")
	}

	// --- 3a. data_types_quantity_gen.go must contain the new field.
	quantPath := filepath.Join(outDir, "openehr/rm/data_types_quantity_gen.go")
	body, err := os.ReadFile(quantPath)
	if err != nil {
		t.Fatalf("read generated quantity file: %v", err)
	}
	wantSnippets := []string{
		"TestProperty",
		`json:"test_property,omitempty"`,
	}
	for _, want := range wantSnippets {
		if !strings.Contains(string(body), want) {
			t.Errorf("data_types_quantity_gen.go missing snippet %q", want)
		}
	}

	// --- 3b. typereg_gen.go must be unchanged vs the baseline
	//        regeneration (same DV_QUANTITY etc., no new class).
	baselineOutDir := t.TempDir()
	if _, err := Run(Options{
		ResourcesDir: testResources,
		OutDir:       baselineOutDir,
		Targets:      []Target{TargetRM},
		RootID:       "openehr_rm_1.2.0",
	}); err != nil {
		t.Fatalf("Run baseline: %v", err)
	}
	baselineTypereg := mustReadFile(t, filepath.Join(baselineOutDir, "openehr/rm/typereg_gen.go"))
	bumpedTypereg := mustReadFile(t, filepath.Join(outDir, "openehr/rm/typereg_gen.go"))
	// typereg files differ only in the SourceLabel inside the header
	// comment (1.2.0 vs 1.2.1). Strip both file headers before
	// comparing so we observe the *structural* content.
	baselineTrim := stripHeader(string(baselineTypereg))
	bumpedTrim := stripHeader(string(bumpedTypereg))
	if baselineTrim != bumpedTrim {
		t.Errorf("typereg_gen.go (excluding header) differs between baseline and bumped\n=== baseline ===\n%s\n=== bumped ===\n%s",
			baselineTrim, bumpedTrim)
	}

	// --- 3c. The Go-side diff vs the baseline data_types_quantity
	//        file must be small (the only structural change is the
	//        added field).
	baselineQuant := mustReadFile(t, filepath.Join(baselineOutDir, "openehr/rm/data_types_quantity_gen.go"))
	added, removed := lineDelta(string(baselineQuant), string(body))
	if removed != 0 {
		t.Errorf("expected 0 removed lines from data_types_quantity_gen.go, got %d", removed)
	}
	// Add count is small — typically 1 (just the field) plus an
	// optional doc-comment line above it. Allow up to 4 to give the
	// template room.
	if added < 1 || added > 4 {
		t.Errorf("expected 1-4 added lines, got %d", added)
	}

	// --- 4. CHANGELOG suggestion.
	oldSchema, err := bmm.LoadAll("openehr_rm_1.2.0", bmm.FSResolver{Root: testResources})
	if err != nil {
		t.Fatalf("LoadAll old: %v", err)
	}
	newSchema, err := bmm.LoadAll("openehr_rm_1.2.1", bmm.FSResolver{Root: stage})
	if err != nil {
		t.Fatalf("LoadAll new: %v", err)
	}
	report := bmmdiff.Diff(oldSchema, newSchema)
	suggestion := bmmdiff.SuggestChangelogEntry(report)
	if suggestion == "" {
		t.Fatalf("expected non-empty CHANGELOG suggestion")
	}
	wantContains := []string{
		"openEHR RM",
		"1.2.0 -> 1.2.1",
		"DV_QUANTITY",
		"test_property",
		"[bmm-bump]",
	}
	for _, w := range wantContains {
		if !strings.Contains(suggestion, w) {
			t.Errorf("suggestion missing %q\n  got: %s", w, suggestion)
		}
	}
	t.Logf("simulated-bump CHANGELOG entry:\n  %s", suggestion)
}

// bumpRMWithTestProperty parses the 1.2.0 BMM JSON, switches its
// rm_release to "1.2.1", and adds an optional SingleProperty
// `test_property` (type String, is_mandatory absent) to DV_QUANTITY.
// Returns the re-marshalled bytes.
//
// We operate on the raw JSON (rather than the loader's
// in-memory model) to keep the test independent of any further
// internal refactors of openehr/bmm.
func bumpRMWithTestProperty(in []byte) ([]byte, error) {
	var doc map[string]any
	if err := json.Unmarshal(in, &doc); err != nil {
		return nil, err
	}
	doc["rm_release"] = "1.2.1"
	classDefs, ok := doc["class_definitions"].(map[string]any)
	if !ok {
		return nil, errMissing("class_definitions")
	}
	dvq, ok := classDefs["DV_QUANTITY"].(map[string]any)
	if !ok {
		return nil, errMissing("DV_QUANTITY")
	}
	props, ok := dvq["properties"].(map[string]any)
	if !ok {
		props = map[string]any{}
		dvq["properties"] = props
	}
	props["test_property"] = map[string]any{
		"_type":         "P_BMM_SINGLE_PROPERTY",
		"name":          "test_property",
		"type":          "String",
		"documentation": "Synthetic property introduced by the Phase 5 simulated-bump test.",
	}
	return json.MarshalIndent(doc, "", "  ")
}

type errString string

func (e errString) Error() string { return string(e) }
func errMissing(field string) error {
	return errString("simulated bump: missing " + field)
}

// copyResource copies <name> from testResources into stage.
func copyResource(t *testing.T, name, stage string) {
	t.Helper()
	src := filepath.Join(testResources, name)
	dst := filepath.Join(stage, name)
	b, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("copyResource read %s: %v", src, err)
	}
	if err := os.WriteFile(dst, b, 0o644); err != nil {
		t.Fatalf("copyResource write %s: %v", dst, err)
	}
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return b
}

// stripHeader drops the leading two-line generated-code header so two
// files generated against different schema_release labels can be
// compared structurally.
func stripHeader(s string) string {
	// Drop everything up to the first blank line — the header is
	// two // comment lines plus a blank line plus the package clause.
	lines := strings.SplitN(s, "\n", 4)
	if len(lines) < 4 {
		return s
	}
	return lines[3]
}

// lineDelta returns (added, removed) line counts comparing a and b.
// A line present in b but not in a counts as added; vice versa for
// removed. Simple multiset semantics — sufficient for the
// small-diff sanity check in this test.
func lineDelta(a, b string) (added, removed int) {
	aLines := strings.Split(a, "\n")
	bLines := strings.Split(b, "\n")
	aCount := map[string]int{}
	bCount := map[string]int{}
	for _, l := range aLines {
		aCount[l]++
	}
	for _, l := range bLines {
		bCount[l]++
	}
	for l, c := range bCount {
		if d := c - aCount[l]; d > 0 {
			added += d
		}
	}
	for l, c := range aCount {
		if d := c - bCount[l]; d > 0 {
			removed += d
		}
	}
	return added, removed
}
