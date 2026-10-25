package validation

import (
	"encoding/json"
	"maps"
	"os"
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm/rminfo"
)

// TestNonEmptyLists_REQ112_MatchTheClassification pins the lists the floor's
// REQ-112 Non-empty lists rules read, which the generator reads too
// (rminfo.NonEmptyLists), against invariants.json: the table names exactly
// the invariants the classification calls evaluated by checkListMembers, so
// a change to one fails here until the other follows. The rules the floor
// dispatches (listMemberRules) are that table, keyed CLASS.Invariant, each
// with the list it reads and its alternative.
func TestNonEmptyLists_REQ112_MatchTheClassification(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("invariants.json")
	if err != nil {
		t.Fatalf("read invariants.json: %v", err)
	}
	var classification map[string]struct {
		Kind     string `json:"kind"`
		Function string `json:"function"`
	}
	if err := json.Unmarshal(raw, &classification); err != nil {
		t.Fatalf("decode invariants.json: %v", err)
	}
	var evaluated []string
	for key, e := range classification {
		if e.Kind == "evaluated" && e.Function == listMembersCheck {
			evaluated = append(evaluated, key)
		}
	}
	slices.Sort(evaluated)
	lists := rminfo.NonEmptyLists()
	want := make(map[string]listMembers, len(lists))
	var named []string
	for _, l := range lists {
		key := l.Class + "." + l.Invariant
		named = append(named, key)
		want[key] = listMembers{list: l.List, alternative: l.Alternative}
	}
	slices.Sort(named)
	if !slices.Equal(named, evaluated) {
		t.Errorf("rminfo.NonEmptyLists names %q, want %q, the invariants invariants.json calls evaluated by %s", named, evaluated, listMembersCheck)
	}
	if len(evaluated) == 0 {
		t.Fatalf("invariants.json calls no invariant evaluated by %s, so the check compares nothing", listMembersCheck)
	}
	if !maps.Equal(listMemberRules, want) {
		t.Errorf("listMemberRules = %v, want %v, the table rminfo.NonEmptyLists gives", listMemberRules, want)
	}
}
