package validation_test

// walk_alloc_test.go: what the one walk allocates where it reports nothing.

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// REQ-171 — a walk that reports nothing writes no issue path: the walk
// writes a path only into what records it (an issue, a refusal, a stop), so
// it allocates about half the objects it did when it wrote the path of every
// attribute and item as it entered it. The BMI corpus composition reports no
// issue under the RM rules alone; ValidateRM of it allocated 618 objects per
// call before (BenchmarkValidateComposition/BMI/rm at 90355d7c) and 319
// after. The bound sits just above that, so a change that writes paths again
// where nothing records them fails here. The race detector's instrumentation
// changes what the walk allocates, so a race build skips the count.
func TestValidateRM_REQ171_WritesNoPathWhereNothingIsReported(t *testing.T) {
	if raceEnabled {
		t.Skip("the race detector's instrumentation changes what the walk allocates")
	}
	const most = 330
	var comp rm.Composition
	mustDecodeJSON(t, fixtures.CompositionJSON("BMI"), &comp)
	if r := validation.ValidateRM(&comp); len(r.Issues) != 0 {
		t.Fatalf("precondition: ValidateRM(BMI) reports %d issues, want none", len(r.Issues))
	}
	if got := testing.AllocsPerRun(20, func() { _ = validation.ValidateRM(&comp) }); got > most {
		t.Errorf("ValidateRM(BMI) allocates %.0f objects per call, want at most %d: a path is written where nothing records it", got, most)
	}
}
