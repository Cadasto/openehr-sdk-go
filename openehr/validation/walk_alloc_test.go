package validation_test

// walk_alloc_test.go: what the one walk allocates where it reports nothing.

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// REQ-171 — a walk that reports nothing writes no issue path: the walk
// writes a path only into what records it (an issue, a refusal, a stop), and
// writing the path of every attribute and item as it entered it cost about
// 300 objects. The BMI corpus composition reports no issue under the RM
// rules alone; ValidateRM of it allocated 618 objects per call before the
// walk wrote paths only where they are recorded
// (BenchmarkValidateComposition/BMI/rm on the commit "perf(validation):
// count named and pointer primitive leaves without entering them", 2d59ff58)
// and 319 after (PR 282). PR 343 took it to 41, reading a primitive leaf the
// walk only counts without boxing it, PR 483 to 35, dropping three
// per-value allocations, and PR 500 to 19, reading a list's items onto a
// reused stack. The bound sits just above that, so a change that writes
// paths again where nothing records them fails here. The race detector's
// instrumentation changes what the walk allocates, so a race build skips
// the count.
func TestValidateRM_REQ171_WritesNoPathWhereNothingIsReported(t *testing.T) {
	if raceEnabled {
		t.Skip("the race detector's instrumentation changes what the walk allocates")
	}
	const most = 21
	var comp rm.Composition
	mustDecodeJSON(t, fixtures.CompositionJSON("BMI"), &comp)
	if r := validation.ValidateRM(&comp); len(r.Issues) != 0 {
		t.Fatalf("precondition: ValidateRM(BMI) reports %d issues, want none", len(r.Issues))
	}
	if got := testing.AllocsPerRun(20, func() { _ = validation.ValidateRM(&comp) }); got > most {
		t.Errorf("ValidateRM(BMI) allocates %.0f objects per call, want at most %d: a path is written where nothing records it", got, most)
	}
}

// REQ-103, REQ-102 and REQ-171 — a template walk that reports nothing checks
// each code, quantity and ordinal its template constrains with the typed
// check, without boxing the value into an any, and binds the items of each
// list on the engine's stacks, without slices of the list's own. The BMI
// corpus composition reports no issue under its template;
// ValidateComposition of it allocated 64 objects per call before either
// change (BenchmarkValidateComposition/BMI/template on 826b4b5c, PR 500), 53
// after the commit "perf(validation): REQ-103 check a code, a quantity and
// an ordinal typed" and 24 after "perf(validation): REQ-102 bind a list's
// items on stacks, not per-list slices". The bound sits just above that, so
// undoing either change fails here. The race detector's instrumentation
// changes what the walk allocates, so a race build skips the count.
func TestValidateComposition_REQ171_ChecksLeavesTypedAndBindsListsOnStacks(t *testing.T) {
	if raceEnabled {
		t.Skip("the race detector's instrumentation changes what the walk allocates")
	}
	const most = 26
	var comp rm.Composition
	mustDecodeJSON(t, fixtures.CompositionJSON("BMI"), &comp)
	c := mustCompile(t, "BMI")
	if r := validation.ValidateComposition(&comp, c); len(r.Issues) != 0 {
		t.Fatalf("precondition: ValidateComposition(BMI) reports %d issues, want none", len(r.Issues))
	}
	if got := testing.AllocsPerRun(20, func() { _ = validation.ValidateComposition(&comp, c) }); got > most {
		t.Errorf("ValidateComposition(BMI) allocates %.0f objects per call, want at most %d: a leaf is boxed for its check, or a list is bound with slices of its own", got, most)
	}
}
