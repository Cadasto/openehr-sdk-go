package simplified

// REQ-053 / REQ-140 — an ACTION's ISM transition round-trips through the Web
// Template's ISM_TRANSITION nodes. The corpus template models two of them,
// `transition` (ism_transition[at0005]) and `transition2`
// (ism_transition[at0006]). Decode already rebuilt the transition, but encode
// read it through rmpath, which did not resolve ACTION `ism_transition`, and
// so dropped the RM-mandatory attribute without an error.

import (
	"maps"
	"slices"
	"strings"
	"testing"
)

// ismTransitionKeys is one careflow step of the corpus ACTION archetype, as the
// Web Template node for it spells it.
func ismTransitionKeys(node, state, step string) map[string]any {
	base := rmattrAction + "/" + node
	return map[string]any{
		base + "/current_state|code":        state,
		base + "/current_state|value":       "state " + state,
		base + "/current_state|terminology": "openehr",
		base + "/careflow_step|code":        step,
		base + "/careflow_step|value":       "step " + step,
		base + "/careflow_step|terminology": "local",
	}
}

// TestActionIsmTransitionRoundTrips — REQ-053, REQ-140. Each careflow step
// comes back under the node whose ISM_TRANSITION constraint it belongs to, and
// under no other.
func TestActionIsmTransitionRoundTrips(t *testing.T) {
	wt, _ := conformanceWT(t)
	for _, tc := range []struct {
		node, state, step, other string
	}{
		{node: "transition", state: "524", step: "at0005", other: "transition2"},
		{node: "transition2", state: "532", step: "at0006", other: "transition"},
	} {
		t.Run(tc.node, func(t *testing.T) {
			want := ismTransitionKeys(tc.node, tc.state, tc.step)
			comp := decodeRMAttr(t, wt, rmattrBody(want))
			got := reencodeRMAttr(t, wt, comp)
			for _, k := range slices.Sorted(maps.Keys(want)) {
				if got[k] != want[k] {
					t.Errorf("re-encode of %s = %#v, want %#v", k, got[k], want[k])
				}
			}
			for k := range got {
				if strings.HasPrefix(k, rmattrAction+"/"+tc.other+"/") {
					t.Errorf("re-encode wrote %s: the careflow step %s belongs to %s alone", k, tc.step, tc.node)
				}
			}
		})
	}
}
