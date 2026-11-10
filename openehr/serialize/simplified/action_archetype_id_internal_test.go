package simplified

// REQ-053 — the template index gives an ACTIVITY's path the
// action_archetype_id of the ACTIVITY node there only where every node at
// that path gives the same one: nodes there that disagree leave the path with
// none, and decode then leaves the attribute out. Only reused siblings the
// path cannot tell apart share one, and decode refuses a body that reaches
// them, so the rule is pinned on the index itself.

import "testing"

// TestREQ053TemplateIndexActionArchetypeIDDisagreement — REQ-053: nodes that
// share a path but disagree on the action_archetype_id, one of them giving
// none included, leave the path with none; nodes that agree keep it.
func TestREQ053TemplateIndexActionArchetypeIDDisagreement(t *testing.T) {
	t.Parallel()
	const first, second = "openEHR-EHR-ACTION.first.v1", "openEHR-EHR-ACTION.second.v1"
	cases := []struct {
		name string
		adds []string
		want string
	}{
		{name: "one node", adds: []string{first}, want: first},
		{name: "two nodes that agree", adds: []string{first, first}, want: first},
		{name: "two that disagree", adds: []string{first, second}, want: ""},
		{name: "a node with none, then one with an id", adds: []string{"", first}, want: ""},
		{name: "an id, then a node with none", adds: []string{first, ""}, want: ""},
		{name: "a disagreement that a third node repeats", adds: []string{first, second, first}, want: ""},
	}
	for _, tc := range cases {
		x := &templateIndex{actionArchetypeIDs: make(map[string]string)}
		for _, id := range tc.adds {
			x.addActionArchetypeID("/activities[at0001]", id)
		}
		if got := x.actionArchetypeID("/activities[at0001]"); got != tc.want {
			t.Errorf("%s: actionArchetypeID after %q = %q, want %q", tc.name, tc.adds, got, tc.want)
		}
	}
}
