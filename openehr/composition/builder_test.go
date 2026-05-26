package composition_test

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/composition"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
)

// optPath resolves a vendored OPT fixture path relative to this test
// file so `go test` works from any cwd.
func optPath(t *testing.T, name string) string {
	t.Helper()
	_, here, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve test source path")
	}
	root := filepath.Join(filepath.Dir(here), "..", "..", "openehr", "template", "testdata")
	return filepath.Join(root, name)
}

func compileFixture(t *testing.T, name string) *templatecompile.Compiled {
	t.Helper()
	opt, err := template.ParseFile(optPath(t, name))
	if err != nil {
		t.Fatalf("ParseFile %s: %v", name, err)
	}
	c, err := templatecompile.Compile(opt)
	if err != nil {
		t.Fatalf("Compile %s: %v", name, err)
	}
	return c
}

// testComposer returns a stable *rm.PartyIdentified fixture. The
// Name field is *string in the generated RM, so callers can't inline
// a struct literal — this helper centralises the conversion.
func testComposer() *rm.PartyIdentified {
	name := "Test Composer"
	return &rm.PartyIdentified{Name: &name}
}

// Systolic DV_QUANTITY path under vital_signs blood_pressure
// observation. Confirmed by compile-time inspection of the fixture.
const systolicPath = "/content[openEHR-EHR-OBSERVATION.blood_pressure.v1]/data/events[at0006]/data/items[at0004]/value"

const diastolicPath = "/content[openEHR-EHR-OBSERVATION.blood_pressure.v1]/data/events[at0006]/data/items[at0005]/value"

// TestNewSkeleton_vitalSigns asserts NewSkeleton produces a valid
// COMPOSITION with category, language, territory, composer, and a
// non-empty content slice.
func TestNewSkeleton_vitalSigns(t *testing.T) {
	c := compileFixture(t, "vital_signs.opt")
	comp, err := composition.NewSkeleton(context.Background(), c,
		composition.WithTerritory("NL"),
		composition.WithComposer(testComposer()),
	)
	if err != nil {
		t.Fatalf("NewSkeleton: %v", err)
	}
	if comp.Category.DefiningCode.CodeString != "433" {
		t.Errorf("Category code = %q, want 433", comp.Category.DefiningCode.CodeString)
	}
	if comp.Territory.CodeString != "NL" {
		t.Errorf("Territory code = %q, want NL", comp.Territory.CodeString)
	}
	if comp.Composer == nil {
		t.Error("Composer is nil")
	}
	if comp.Language.CodeString == "" {
		t.Error("Language code is empty")
	}
	if comp.Context == nil || comp.Context.StartTime.Value == "" {
		t.Error("Context.StartTime is empty")
	}
	if len(comp.Content) == 0 {
		t.Error("Content empty — vital_signs OPT pins multiple OBSERVATIONs")
	}
}

// TestNewSkeleton_requiresComposer asserts that omitting WithComposer
// surfaces instance.ErrComposerRequired (wrapped).
func TestNewSkeleton_requiresComposer(t *testing.T) {
	c := compileFixture(t, "vital_signs.opt")
	_, err := composition.NewSkeleton(context.Background(), c,
		composition.WithTerritory("NL"),
	)
	if err == nil {
		t.Fatal("expected error for missing Composer, got nil")
	}
}

// TestBuilder_SetQuantity_systolic exercises the canonical
// path-first authoring case: load OPT, NewBuilder, SetQuantity at
// systolic and diastolic DV_QUANTITY paths, Build, marshal via
// canjson, and confirm the magnitude / units survive the round-trip.
func TestBuilder_SetQuantity_systolic(t *testing.T) {
	c := compileFixture(t, "vital_signs.opt")
	b, err := composition.NewBuilder(context.Background(), c,
		composition.WithTerritory("NL"),
		composition.WithComposer(testComposer()),
	)
	if err != nil {
		t.Fatalf("NewBuilder: %v", err)
	}
	if err := b.SetQuantity(systolicPath, 120, "mm[Hg]"); err != nil {
		t.Fatalf("SetQuantity systolic: %v", err)
	}
	if err := b.SetQuantity(diastolicPath, 80, "mm[Hg]"); err != nil {
		t.Fatalf("SetQuantity diastolic: %v", err)
	}
	comp, err := b.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	// Probe the in-memory graph first — cheaper than a round-trip
	// to confirm the assignment landed.
	sys, dia := findSystolicDiastolic(comp)
	if sys == nil {
		t.Fatal("systolic DV_QUANTITY not found in built composition")
	}
	if float64(sys.Magnitude) != 120 || sys.Units != "mm[Hg]" {
		t.Errorf("systolic = %v %q, want 120 mm[Hg]", sys.Magnitude, sys.Units)
	}
	if dia == nil {
		t.Fatal("diastolic DV_QUANTITY not found in built composition")
	}
	if float64(dia.Magnitude) != 80 {
		t.Errorf("diastolic magnitude = %v, want 80", dia.Magnitude)
	}

	// Confirm canjson Marshal preserves the values in the output
	// bytes (the leaf path / value reaches the wire). Full Marshal
	// → Unmarshal round-trip is exercised in PROBE-023; some root-
	// level identity fields (UID) currently marshal without the
	// canjson `_type` discriminator (see instance.newHierObjectID),
	// so the round-trip Unmarshal there clears UID and Composer
	// before re-decoding.
	payload, err := canjson.Marshal(comp)
	if err != nil {
		t.Fatalf("canjson.Marshal: %v", err)
	}
	if !bytes.Contains(payload, []byte(`"magnitude":120`)) {
		t.Errorf("canjson output missing systolic magnitude; payload (first 400 bytes): %s", trim(payload))
	}
	if !bytes.Contains(payload, []byte(`"units":"mm[Hg]"`)) {
		t.Errorf("canjson output missing units")
	}
}

// TestBuilder_SetText_typeMismatch confirms SetText's wrap into
// &rm.DVText surfaces a clean ErrTypeMismatch when targeted at a
// non-DV_TEXT path (the vital_signs OPT pins no DV_TEXT primitive
// leaves, so the positive case is exercised end-to-end by PROBE-023
// on an OPT that does — vital_signs systolic is the canonical
// DV_QUANTITY case for SetQuantity).
func TestBuilder_SetText_typeMismatch(t *testing.T) {
	c := compileFixture(t, "vital_signs.opt")
	b, err := composition.NewBuilder(context.Background(), c,
		composition.WithTerritory("NL"),
		composition.WithComposer(testComposer()),
	)
	if err != nil {
		t.Fatalf("NewBuilder: %v", err)
	}
	if err := b.SetText(systolicPath, "wrong"); !errors.Is(err, composition.ErrTypeMismatch) {
		t.Errorf("SetText at DV_QUANTITY path: want ErrTypeMismatch, got %v", err)
	}
}

// TestBuilder_SetCodedText_typeMismatch confirms SetCodedText's
// wrap surfaces ErrTypeMismatch on a non-DV_CODED_TEXT path.
func TestBuilder_SetCodedText_typeMismatch(t *testing.T) {
	c := compileFixture(t, "vital_signs.opt")
	b, err := composition.NewBuilder(context.Background(), c,
		composition.WithTerritory("NL"),
		composition.WithComposer(testComposer()),
	)
	if err != nil {
		t.Fatalf("NewBuilder: %v", err)
	}
	if err := b.SetCodedText(systolicPath, "openehr", "at0000", "x"); !errors.Is(err, composition.ErrTypeMismatch) {
		t.Errorf("SetCodedText at DV_QUANTITY path: want ErrTypeMismatch, got %v", err)
	}
}

// TestBuilder_Set_typeMismatch asserts ErrTypeMismatch when a
// *rm.DVText is supplied at a DV_QUANTITY path.
func TestBuilder_Set_typeMismatch(t *testing.T) {
	c := compileFixture(t, "vital_signs.opt")
	b, err := composition.NewBuilder(context.Background(), c,
		composition.WithTerritory("NL"),
		composition.WithComposer(testComposer()),
	)
	if err != nil {
		t.Fatalf("NewBuilder: %v", err)
	}
	err = b.Set(systolicPath, &rm.DVText{Value: "wrong"})
	if !errors.Is(err, composition.ErrTypeMismatch) {
		t.Errorf("expected ErrTypeMismatch, got %v", err)
	}
}

// TestBuilder_Set_unknownPath asserts ErrUnknownPath for a path the
// OPT does not contain.
func TestBuilder_Set_unknownPath(t *testing.T) {
	c := compileFixture(t, "vital_signs.opt")
	b, err := composition.NewBuilder(context.Background(), c,
		composition.WithTerritory("NL"),
		composition.WithComposer(testComposer()),
	)
	if err != nil {
		t.Fatalf("NewBuilder: %v", err)
	}
	err = b.Set("/no/such/path", &rm.DVText{Value: "x"})
	if !errors.Is(err, composition.ErrUnknownPath) {
		t.Errorf("expected ErrUnknownPath, got %v", err)
	}
	// PR #19 deferred suggestion: the error message should carry the
	// path string AND the template-side cause for callers diagnosing
	// from logs. Wrapping NodeAt's typed cause was added in the
	// follow-up; pin its diagnostic content here.
	if msg := err.Error(); !contains(msg, "/no/such/path") {
		t.Errorf("error missing path string: %v", err)
	}
	// PR #20 re-review (Important 1): the multi-%w wrap means
	// errors.Is reaches the templatecompile sentinel too — the
	// compiled-path API uses its own ErrPathNotFound distinct from
	// template.ErrPathNotFound by design (see
	// internal/templatecompile/compiled.go). Pin it so a future
	// refactor that drops the inner %w surfaces here.
	if !errors.Is(err, templatecompile.ErrPathNotFound) {
		t.Errorf("expected errors.Is to reach templatecompile.ErrPathNotFound, got %v", err)
	}
}

// TestBuilder_Build_AggregatesErrors confirms that two bad Set calls
// + one good Set surface as a single joined error from Build, with
// each per-path failure recoverable via errors.Is.
func TestBuilder_Build_AggregatesErrors(t *testing.T) {
	c := compileFixture(t, "vital_signs.opt")
	b, err := composition.NewBuilder(context.Background(), c,
		composition.WithTerritory("NL"),
		composition.WithComposer(testComposer()),
	)
	if err != nil {
		t.Fatalf("NewBuilder: %v", err)
	}
	// Two failures + one success — Set returns the per-call error
	// (typed), Build joins all into one returned error.
	_ = b.Set("/no/such/path", &rm.DVText{Value: "x"})
	_ = b.SetQuantity(systolicPath, 120, "mm[Hg]") // valid
	_ = b.Set("/also/missing", &rm.DVText{Value: "y"})

	_, err = b.Build()
	if err == nil {
		t.Fatal("expected aggregated error from Build, got nil")
	}
	if !errors.Is(err, composition.ErrUnknownPath) {
		t.Errorf("expected joined error to surface ErrUnknownPath, got %v", err)
	}
	// Both bad-path strings should appear in the joined diagnostic.
	msg := err.Error()
	if !contains(msg, "/no/such/path") || !contains(msg, "/also/missing") {
		t.Errorf("joined error missing one of the two bad paths: %v", err)
	}
}

// TestBuilder_Build_Idempotent confirms a second Build with no new
// Set calls returns a nil error — accumulated errors from the first
// Build are not replayed (PR #19 review suggestion: drain state
// between Build passes for chained authoring).
func TestBuilder_Build_Idempotent(t *testing.T) {
	c := compileFixture(t, "vital_signs.opt")
	b, err := composition.NewBuilder(context.Background(), c,
		composition.WithTerritory("NL"),
		composition.WithComposer(testComposer()),
	)
	if err != nil {
		t.Fatalf("NewBuilder: %v", err)
	}
	// First pass fails on a bogus path.
	_ = b.Set("/no/such/path", &rm.DVText{Value: "x"})
	if _, err := b.Build(); err == nil {
		t.Fatal("first Build: expected error, got nil")
	}
	// Second pass with no new Set must NOT replay the first pass's
	// errors. Skeleton is returned unchanged with nil error.
	out, err := b.Build()
	if err != nil {
		t.Errorf("second Build replayed prior errors: %v", err)
	}
	if out == nil {
		t.Error("second Build returned nil skeleton")
	}
}

// contains is a local micro-helper to keep the aggregated-error test
// self-contained without pulling strings.Contains everywhere.
func contains(haystack, needle string) bool {
	return bytes.Contains([]byte(haystack), []byte(needle))
}

// TestBuilder_TemplateID asserts Builder.TemplateID matches the
// compiled template's id.
func TestBuilder_TemplateID(t *testing.T) {
	c := compileFixture(t, "vital_signs.opt")
	b, err := composition.NewBuilder(context.Background(), c,
		composition.WithTerritory("NL"),
		composition.WithComposer(testComposer()),
	)
	if err != nil {
		t.Fatalf("NewBuilder: %v", err)
	}
	if got, want := b.TemplateID(), c.TemplateID(); got != want {
		t.Errorf("TemplateID = %q, want %q", got, want)
	}
}

// findSystolicDiastolic walks a built blood-pressure composition
// and returns the systolic + diastolic DV_QUANTITY values when both
// are present.
func findSystolicDiastolic(c *rm.Composition) (*rm.DVQuantity, *rm.DVQuantity) {
	var sys, dia *rm.DVQuantity
	for _, item := range c.Content {
		obs, ok := item.(*rm.Observation)
		if !ok {
			continue
		}
		if obs.ArchetypeNodeID != "openEHR-EHR-OBSERVATION.blood_pressure.v1" {
			continue
		}
		for _, ev := range obs.Data.Events {
			pe, ok := ev.(*rm.PointEvent[rm.ItemStructure])
			if !ok {
				continue
			}
			itl, ok := pe.Data.(*rm.ItemList)
			if !ok {
				continue
			}
			for i := range itl.Items {
				el := &itl.Items[i]
				dv, ok := el.Value.(*rm.DVQuantity)
				if !ok || dv == nil {
					continue
				}
				switch el.ArchetypeNodeID {
				case "at0004":
					sys = dv
				case "at0005":
					dia = dv
				}
			}
		}
	}
	return sys, dia
}

func trim(b []byte) string {
	if len(b) > 400 {
		return string(b[:400]) + "...(truncated)"
	}
	return string(b)
}
