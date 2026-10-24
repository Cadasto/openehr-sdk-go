package validation_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// REQ-102 (Structural — binding items), REQ-116 — a one-entry name list
// with list_open true pins no name, so an item whose name is that entry
// binds as if the child pinned none: to the first child that pins no name.
// Here that is the first child, which may occur once, so the second, which
// must, is reported as occurring too seldom. Had the open list pinned its
// entry, the item would bind to the second child and nothing would be
// reported.
func TestValidateComposition_REQ102_REQ116_OpenNameListPinsNoName(t *testing.T) {
	const entry = "<list>Erste</list>"
	open := reusedObservationChild("Erste", 1)
	if strings.Count(open, entry) != 1 {
		t.Fatalf("reusedObservationChild does not hold %s exactly once", entry)
	}
	open = strings.Replace(open, entry, entry+"<list_open>true</list_open>", 1)
	c := mustCompileSyntheticOPT(t, reusedArchetypeOPTWith(reusedObservationChild("", 0), open))
	r := validation.ValidateComposition(reusedComposition("Erste"), c)
	want := []string{"cardinality /content[" + reusedID + "]"}
	if got := issueKeys(r.Issues); !slices.Equal(got, want) {
		t.Fatalf("ValidateComposition(content named %q) issues = %q, want %q", "Erste", got, want)
	}
	if detail := r.Issues[0].Detail; !strings.Contains(detail, "appears 0 times") || !strings.Contains(detail, "[1..1]") {
		t.Errorf("cardinality detail = %q, want the second child, occurring 0 times against [1..1]", detail)
	}
}
