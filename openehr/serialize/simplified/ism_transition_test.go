package simplified

// REQ-053 / REQ-140 — an ACTION's ISM transition round-trips through the Web
// Template's ISM_TRANSITION nodes. The corpus template models two of them,
// `transition` (ism_transition[at0005]) and `transition2`
// (ism_transition[at0006]). Decode already rebuilt the transition, but encode
// read it through rmpath, which did not resolve ACTION `ism_transition`, and
// so dropped the RM-mandatory attribute without an error.

import (
	"errors"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/template/webtemplate"
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

// ismNodeName is the Web Template name of the ACTION transition node with the
// given FLAT id — what decode gives a careflow step the body does not carry.
func ismNodeName(t *testing.T, wt *webtemplate.WebTemplate, node string) string {
	t.Helper()
	n := wt.Tree
	for id := range strings.SplitSeq(strings.TrimPrefix(rmattrAction+"/"+node, rmattrRoot+"/"), "/") {
		if n = childByID(n, id); n == nil {
			t.Fatalf("corpus template has no %q under %s — fixture changed?", id, rmattrAction)
		}
	}
	if n.Name == "" {
		t.Fatalf("corpus template node %q has no name — fixture changed?", node)
	}
	return n.Name
}

// firstAction digs out the corpus template's conformance_action.
func firstAction(t *testing.T, comp *rm.Composition) *rm.Action {
	t.Helper()
	sec, ok := comp.Content[0].(*rm.Section)
	if !ok {
		t.Fatalf("content[0] = %T, want *rm.Section", comp.Content[0])
	}
	for _, item := range sec.Items {
		if a, ok := item.(*rm.Action); ok {
			return a
		}
	}
	t.Fatal("no ACTION found under the section")
	return nil
}

// TestActionIsmTransitionDecodeGivesCareflowStep — REQ-053, REQ-121. A body that
// spells only a transition's current state still names the node it sits under,
// and that node id is the careflow step encode matches the transition by. Decode
// therefore gives the rebuilt ISM_TRANSITION that careflow step — the node id as a
// `local` code, the node's Web Template name as its value — and the state comes
// back under the same node. Without it, encode matched no node and the state was
// lost with no error.
func TestActionIsmTransitionDecodeGivesCareflowStep(t *testing.T) {
	wt, _ := conformanceWT(t)
	for _, tc := range []struct {
		node, state, step, other string
	}{
		{node: "transition", state: "524", step: "at0005", other: "transition2"},
		{node: "transition2", state: "532", step: "at0006", other: "transition"},
	} {
		t.Run(tc.node, func(t *testing.T) {
			name := ismNodeName(t, wt, tc.node)
			full := ismTransitionKeys(tc.node, tc.state, tc.step)
			full[rmattrAction+"/"+tc.node+"/careflow_step|value"] = name
			stateOnly := map[string]any{}
			for k, v := range full {
				if strings.Contains(k, "/current_state|") {
					stateOnly[k] = v
				}
			}

			comp := decodeRMAttr(t, wt, rmattrBody(stateOnly))
			step := firstAction(t, comp).IsmTransition.CareflowStep
			if step == nil {
				t.Fatalf("decoded ISM_TRANSITION has no careflow_step, want local::%s", tc.step)
			}
			if got := step.DefiningCode.CodeString; got != tc.step {
				t.Errorf("careflow_step code = %q, want %q (the node id)", got, tc.step)
			}
			if got := step.DefiningCode.TerminologyID.Value; got != "local" {
				t.Errorf("careflow_step terminology = %q, want local", got)
			}
			if step.Value != name {
				t.Errorf("careflow_step value = %q, want %q (the node's Web Template name)", step.Value, name)
			}

			got := reencodeRMAttr(t, wt, comp)
			for _, k := range slices.Sorted(maps.Keys(full)) {
				if got[k] != full[k] {
					t.Errorf("re-encode of %s = %#v, want %#v", k, got[k], full[k])
				}
			}
			for k := range got {
				if strings.HasPrefix(k, rmattrAction+"/"+tc.other+"/") {
					t.Errorf("re-encode wrote %s: the transition belongs to %s alone", k, tc.node)
				}
			}
		})
	}
}

// TestActionIsmTransitionCareflowStepCodedOtherwiseRefused — REQ-053, REQ-121. A
// careflow step whose code is not its node's id names another node, so encode
// would move the transition there. Decode refuses it instead, naming the key's
// node and the code, never the clinical value.
func TestActionIsmTransitionCareflowStepCodedOtherwiseRefused(t *testing.T) {
	wt, _ := conformanceWT(t)
	for _, tc := range []struct {
		name string
		keys map[string]any
	}{
		{name: "another node's id", keys: ismTransitionKeys("transition", "524", "at0006")},
		{name: "no node's id", keys: ismTransitionKeys("transition2", "532", "at0099")},
		{name: "a careflow step with no current state", keys: map[string]any{
			rmattrAction + "/transition/careflow_step|code":        "at0006",
			rmattrAction + "/transition/careflow_step|value":       "secret step",
			rmattrAction + "/transition/careflow_step|terminology": "local",
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := decodeRMAttrErr(t, wt, rmattrBody(tc.keys))
			if !errors.Is(err, ErrUnsupportedDatatype) {
				t.Fatalf("err = %v, want ErrUnsupportedDatatype", err)
			}
			if !strings.Contains(err.Error(), "careflow_step") {
				t.Errorf("err = %v, want it to name the careflow_step", err)
			}
			for k, v := range tc.keys {
				if s, ok := v.(string); ok && strings.HasSuffix(k, "|value") && strings.Contains(err.Error(), s) {
					t.Errorf("err = %v carries the clinical value %q", err, s)
				}
			}
		})
	}
}

// TestActionIsmTransitionTwoNodesRefused — REQ-053. An ACTION has one
// ISM_TRANSITION, and each of the template's transition nodes stands for it. Keys
// spelled under two of those nodes cannot be merged into one object: the merged
// transition would re-encode under one node only, moving the other node's keys.
// Decode refuses the body with ErrUnknownPath, in either key order.
func TestActionIsmTransitionTwoNodesRefused(t *testing.T) {
	wt, _ := conformanceWT(t)
	only := func(keys map[string]any, member string) map[string]any {
		out := map[string]any{}
		for k, v := range keys {
			if strings.Contains(k, "/"+member+"|") {
				out[k] = v
			}
		}
		return out
	}
	at5 := ismTransitionKeys("transition", "524", "at0005")
	at6 := ismTransitionKeys("transition2", "532", "at0006")
	for _, tc := range []struct {
		name string
		keys map[string]any
	}{
		{name: "state at one node, step at the other", keys: mergeRMAttr(only(at5, "current_state"), only(at6, "careflow_step"))},
		{name: "step at one node, state at the other", keys: mergeRMAttr(only(at5, "careflow_step"), only(at6, "current_state"))},
		{name: "a whole transition at both nodes", keys: mergeRMAttr(at5, at6)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := decodeRMAttrErr(t, wt, rmattrBody(tc.keys))
			if !errors.Is(err, ErrUnknownPath) {
				t.Fatalf("err = %v, want ErrUnknownPath", err)
			}
			if !strings.Contains(err.Error(), "ism_transition") {
				t.Errorf("err = %v, want it to name the ism_transition attribute", err)
			}
		})
	}
}

// TestIsmTransitionCanonicalJSONCarriesOnlyRMMembers — REQ-053, REQ-121.
// ISM_TRANSITION is not LOCATABLE, so it has neither an archetype_node_id nor a
// name. Decode keys the rebuilt transition by its node while it places leaves, and
// the template's name index covers the node, but neither may reach the canonical
// JSON handed to canjson — in either decode mode.
func TestIsmTransitionCanonicalJSONCarriesOnlyRMMembers(t *testing.T) {
	wt, compiled := conformanceWT(t)
	for name, names := range map[string]map[string]string{
		"without a template": nil,
		"with a template":    buildNameIndex(compiled),
	} {
		t.Run(name, func(t *testing.T) {
			compJSON, err := decodeFlat(rmattrBody(ismTransitionKeys("transition", "524", "at0005")), wt, names)
			if err != nil {
				t.Fatalf("decodeFlat: %v", err)
			}
			var found []map[string]any
			var walk func(v any)
			walk = func(v any) {
				switch x := v.(type) {
				case map[string]any:
					if x["_type"] == "ISM_TRANSITION" {
						found = append(found, x)
					}
					for _, c := range x {
						walk(c)
					}
				case []any:
					for _, c := range x {
						walk(c)
					}
				}
			}
			walk(compJSON)
			if len(found) != 1 {
				t.Fatalf("canonical JSON holds %d ISM_TRANSITION objects, want 1", len(found))
			}
			for _, member := range slices.Sorted(maps.Keys(found[0])) {
				switch member {
				case "_type", "current_state", "careflow_step", "transition", "reason":
				default:
					t.Errorf("canonical ISM_TRANSITION carries %q, which the RM class does not declare", member)
				}
			}
		})
	}
}
