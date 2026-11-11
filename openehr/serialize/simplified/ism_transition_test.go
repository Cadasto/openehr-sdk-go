package simplified

// REQ-053 / REQ-140 — an ACTION's ISM transition round-trips through the Web
// Template's ISM_TRANSITION nodes. Every ACTION has the reference's in-context
// `ism_transition` node. The corpus template also models two archetyped
// per-step nodes, `transition` (ism_transition[at0005]) and `transition_1`
// (ism_transition[at0006]), which its in-context node merges. Encode writes
// the in-context spelling alone; decode reads both, and refuses one body that
// spells one transition both ways.

import (
	"bytes"
	"errors"
	"maps"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/template/webtemplate"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
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

// conformanceIsm is the in-context ISM transition node of the corpus
// template's ACTION, which merges its two archetyped per-step nodes.
const conformanceIsm = rmattrAction + "/ism_transition"

// inContextIsmKeys is one transition of the corpus ACTION in the reference's
// in-context spelling: its current state, as [ismTransitionKeys] spells it,
// and its careflow step, whose value is stepValue.
func inContextIsmKeys(state, step, stepValue string) map[string]any {
	return map[string]any{
		conformanceIsm + "/current_state|code":        state,
		conformanceIsm + "/current_state|value":       "state " + state,
		conformanceIsm + "/current_state|terminology": "openehr",
		conformanceIsm + "/careflow_step|code":        step,
		conformanceIsm + "/careflow_step|value":       stepValue,
		conformanceIsm + "/careflow_step|terminology": "local",
	}
}

// perStepKeys returns, sorted, the keys of a FLAT map under the corpus
// ACTION's archetyped per-step nodes, `transition` and `transition_1`.
func perStepKeys(flat map[string]any) []string {
	var out []string
	for _, k := range slices.Sorted(maps.Keys(flat)) {
		if strings.HasPrefix(k, rmattrAction+"/transition") {
			out = append(out, k)
		}
	}
	return out
}

// holdsKeys reports each key of want that got does not hold with want's value.
func holdsKeys(t *testing.T, got, want map[string]any) {
	t.Helper()
	for _, k := range slices.Sorted(maps.Keys(want)) {
		if got[k] != want[k] {
			t.Errorf("re-encode of %s = %#v, want %#v", k, got[k], want[k])
		}
	}
}

// TestREQ053PerStepIsmTransitionReencodesInContext — REQ-053, REQ-121. A
// transition spelled under an archetyped per-step node decodes as before, and
// encode writes it in the reference's spelling alone: under the in-context
// `ism_transition` node, with its careflow step, and under no per-step node.
// That output decodes to the same composition. The second node's earlier
// spelling, `transition2`, reads as `transition_1`.
func TestREQ053PerStepIsmTransitionReencodesInContext(t *testing.T) {
	wt, _ := conformanceWT(t)
	for _, tc := range []struct{ node, state, step string }{
		{node: "transition", state: "524", step: "at0005"},
		{node: "transition_1", state: "532", step: "at0006"},
		{node: "transition2", state: "532", step: "at0006"},
	} {
		t.Run(tc.node, func(t *testing.T) {
			comp := decodeRMAttr(t, wt, rmattrBody(ismTransitionKeys(tc.node, tc.state, tc.step)))
			got := reencodeRMAttr(t, wt, comp)
			holdsKeys(t, got, inContextIsmKeys(tc.state, tc.step, "step "+tc.step))
			if keys := perStepKeys(got); len(keys) > 0 {
				t.Errorf("re-encode wrote %v under a per-step node, want the in-context spelling alone", keys)
			}
			if back := decodeRMAttr(t, wt, got); !bytes.Equal(canonical(t, back), canonical(t, comp)) {
				t.Errorf("the in-context re-encode decodes to\n%s\nwant the per-step decode\n%s", canonical(t, back), canonical(t, comp))
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
// spells only a transition's current state under a per-step node still names
// that node, and the node id is the careflow step the transition belongs to.
// Decode therefore gives the rebuilt ISM_TRANSITION that careflow step — the node
// id as a `local` code, the node's Web Template name as its value — and encode
// writes it with the state in the in-context spelling. Without it, the
// careflow step the archetype gives the transition was lost with no error.
func TestActionIsmTransitionDecodeGivesCareflowStep(t *testing.T) {
	wt, _ := conformanceWT(t)
	for _, tc := range []struct{ node, state, step string }{
		{node: "transition", state: "524", step: "at0005"},
		{node: "transition_1", state: "532", step: "at0006"},
	} {
		t.Run(tc.node, func(t *testing.T) {
			name := ismNodeName(t, wt, tc.node)
			stateOnly := map[string]any{}
			for k, v := range ismTransitionKeys(tc.node, tc.state, tc.step) {
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
			holdsKeys(t, got, inContextIsmKeys(tc.state, tc.step, name))
			if keys := perStepKeys(got); len(keys) > 0 {
				t.Errorf("re-encode wrote %v under a per-step node, want the in-context spelling alone", keys)
			}
		})
	}
}

// TestActionIsmTransitionCareflowStepTerminologyDefaultsToLocal — REQ-053,
// REQ-121. At a per-step node a careflow step that names no terminology (none,
// or a blank one) takes `local`, the only terminology an archetype's at-code
// belongs to, so the decoded CODE_PHRASE carries its RM-mandatory
// terminology_id and the transition re-encodes with it.
func TestActionIsmTransitionCareflowStepTerminologyDefaultsToLocal(t *testing.T) {
	wt, _ := conformanceWT(t)
	termKey := rmattrAction + "/transition/careflow_step|terminology"
	for _, tc := range []struct {
		name  string
		shape func(map[string]any)
	}{
		{name: "absent", shape: func(m map[string]any) { delete(m, termKey) }},
		{name: "blank", shape: func(m map[string]any) { m[termKey] = "" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			keys := ismTransitionKeys("transition", "524", "at0005")
			tc.shape(keys)
			comp := decodeRMAttr(t, wt, rmattrBody(keys))
			step := firstAction(t, comp).IsmTransition.CareflowStep
			if step == nil {
				t.Fatal("decoded ISM_TRANSITION has no careflow_step")
			}
			if got := step.DefiningCode.TerminologyID.Value; got != "local" {
				t.Errorf("careflow_step terminology = %q, want local", got)
			}
			got := reencodeRMAttr(t, wt, comp)
			holdsKeys(t, got, map[string]any{
				conformanceIsm + "/current_state|code":        "524",
				conformanceIsm + "/careflow_step|code":        "at0005",
				conformanceIsm + "/careflow_step|terminology": "local",
			})
		})
	}
}

// TestActionIsmTransitionBlankTerminologyEncodes — REQ-121, REQ-140. An ACTION
// built in Go with only its careflow step's code set keeps its transition:
// encode writes the careflow step with no `|terminology`, and decode reads that
// as `local`, as it reads one at any careflow step, so the round trip gives the
// step its terminology instead of losing the transition with no error.
func TestActionIsmTransitionBlankTerminologyEncodes(t *testing.T) {
	wt, _ := conformanceWT(t)
	comp := decodeRMAttr(t, wt, rmattrBody(ismTransitionKeys("transition", "524", "at0005")))
	firstAction(t, comp).IsmTransition.CareflowStep.DefiningCode.TerminologyID.Value = ""
	got := reencodeRMAttr(t, wt, comp)
	holdsKeys(t, got, map[string]any{
		conformanceIsm + "/current_state|code": "524",
		conformanceIsm + "/careflow_step|code": "at0005",
	})
	if term, written := got[conformanceIsm+"/careflow_step|terminology"]; written {
		t.Errorf("re-encode wrote careflow_step|terminology = %#v for a blank terminology, want no key", term)
	}
	step := firstAction(t, decodeRMAttr(t, wt, got)).IsmTransition.CareflowStep
	if step == nil || step.DefiningCode.CodeString != "at0005" || step.DefiningCode.TerminologyID.Value != "local" {
		t.Errorf("decoded careflow_step = %+v, want local::at0005", step)
	}
}

// TestActionIsmTransitionCareflowStepCodedOtherwiseRefused — REQ-053, REQ-121. A
// careflow step whose code is not its node's id names another node, so encode
// would move the transition there. One carrying the node's id in a terminology
// other than `local` names no archetype node at all: an at-code belongs to the
// archetype's own terminology. Decode refuses both, naming the key's node and its
// id, never the step's code, terminology or clinical value.
func TestActionIsmTransitionCareflowStepCodedOtherwiseRefused(t *testing.T) {
	wt, _ := conformanceWT(t)
	inTerminology := func(keys map[string]any, node, terminology string) map[string]any {
		keys[rmattrAction+"/"+node+"/careflow_step|terminology"] = terminology
		return keys
	}
	for _, tc := range []struct {
		name string
		keys map[string]any
	}{
		{name: "another node's id", keys: ismTransitionKeys("transition", "524", "at0006")},
		{name: "no node's id", keys: ismTransitionKeys("transition_1", "532", "at0099")},
		{name: "the node's id in another terminology", keys: inTerminology(ismTransitionKeys("transition", "524", "at0005"), "transition", "SNOMED-CT")},
		{name: "a |raw step whose terminology_id is a string", keys: rawCareflowStep("SNOMED-CT")},
		{name: "a |raw step whose terminology_id is an array", keys: rawCareflowStep([]any{"local"})},
		{name: "a |raw step whose terminology_id value is a number", keys: rawCareflowStep(map[string]any{"_type": "TERMINOLOGY_ID", "value": 7})},
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

// rawCareflowStep is a transition at the at0005 node whose careflow step rides
// |raw with terminologyID as its terminology_id, a shape no CODE_PHRASE has.
func rawCareflowStep(terminologyID any) map[string]any {
	keys := ismTransitionKeys("transition", "524", "at0005")
	for k := range keys {
		if strings.Contains(k, "/careflow_step|") {
			delete(keys, k)
		}
	}
	keys[rmattrAction+"/transition/careflow_step|raw"] = map[string]any{
		"_type": "DV_CODED_TEXT", "value": "secret step",
		"defining_code": map[string]any{"_type": "CODE_PHRASE", "code_string": "at0005", "terminology_id": terminologyID},
	}
	return keys
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
	at6 := ismTransitionKeys("transition_1", "532", "at0006")
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

// TestREQ053IsmTransitionSpelledBothWaysRefused — REQ-053. The in-context
// `ism_transition` node and an archetyped per-step node stand for the one
// ISM_TRANSITION of an ACTION. A body that spells its transition both ways, in
// whole or in part, `_reason` family included, is refused with ErrUnknownPath
// in every mode and in both formats, whichever node a key reaches first; the
// two spellings are never merged into one object.
func TestREQ053IsmTransitionSpelledBothWaysRefused(t *testing.T) {
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
	inContext := inContextIsmKeys("524", "at0005", "transition")
	perStep := ismTransitionKeys("transition", "524", "at0005")
	for _, tc := range []struct {
		name string
		keys map[string]any
	}{
		{name: "state in context, step per step", keys: mergeRMAttr(only(inContext, "current_state"), only(perStep, "careflow_step"))},
		{name: "step in context, state per step", keys: mergeRMAttr(only(inContext, "careflow_step"), only(perStep, "current_state"))},
		{name: "a whole transition both ways", keys: mergeRMAttr(inContext, perStep)},
		{name: "a reason per step beside an in-context transition", keys: mergeRMAttr(inContext, map[string]any{rmattrAction + "/transition/_reason:0": "a reason"})},
		{name: "an in-context reason beside a per-step transition", keys: mergeRMAttr(perStep, map[string]any{conformanceIsm + "/_reason:0": "a reason"})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := rmattrBody(tc.keys)
			for format, err := range map[string]error{
				"FLAT":       refusedInEveryMode(t, wt, body),
				"STRUCTURED": structuredRefusedInEveryMode(t, wt, body),
			} {
				if !errors.Is(err, ErrUnknownPath) || errors.Is(err, ErrNotModelled) {
					t.Errorf("%s err = %v, want ErrUnknownPath and not ErrNotModelled", format, err)
				}
				if !strings.Contains(err.Error(), "ism_transition is single-valued") {
					t.Errorf("%s err = %v, want it to say ACTION.ism_transition is single-valued", format, err)
				}
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
	for name, tmpl := range map[string]*templateIndex{
		"without a template": nil,
		"with a template":    buildTemplateIndex(compiled),
	} {
		t.Run(name, func(t *testing.T) {
			compJSON, err := decodeFlat(rmattrBody(ismTransitionKeys("transition", "524", "at0005")), wt, tmpl, nil)
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

// FLAT path fragments of the minimal_action_2 corpus template, whose ACTION
// archetype constrains no ISM transition: the Web Template gives that ACTION
// the reference's in-context `ism_transition` node.
const (
	minimalRoot   = "minimal"
	minimalAction = minimalRoot + "/minimal_2:0"
	minimalIsm    = minimalAction + "/ism_transition"
)

// minimalActionWT builds the minimal_action_2 Web Template and compiled
// template.
func minimalActionWT(t *testing.T) (*webtemplate.WebTemplate, *templatecompile.Compiled) {
	t.Helper()
	opt, err := readOPTFile(fixtures.TemplateOpt("minimal_action_2"))
	if err != nil {
		t.Fatalf("parse minimal_action_2: %v", err)
	}
	c, err := templatecompile.Compile(opt)
	if err != nil {
		t.Fatalf("compile minimal_action_2: %v", err)
	}
	wt, err := webtemplate.Build(c)
	if err != nil {
		t.Fatalf("build minimal_action_2 Web Template: %v", err)
	}
	if wt.Tree.ID != minimalRoot {
		t.Fatalf("minimal_action_2 root is %q, want %q — fixture changed?", wt.Tree.ID, minimalRoot)
	}
	return wt, c
}

// minimalActionBody is a decodable minimal_action_2 FLAT body: the context,
// the category and the ACTION's proportion, plus extra.
func minimalActionBody(extra map[string]any) map[string]any {
	body := map[string]any{
		"ctx/language":                            "en",
		"ctx/territory":                           "NL",
		"ctx/time":                                "2026-10-26T09:00:00Z",
		"ctx/composer_name":                       "Composer",
		minimalRoot + "/category|code":            "433",
		minimalRoot + "/category|value":           "event",
		minimalRoot + "/category|terminology":     "openehr",
		minimalAction + "/proportion|numerator":   0,
		minimalAction + "/proportion|denominator": 1,
		minimalAction + "/proportion|type":        3,
	}
	maps.Copy(body, extra)
	return body
}

// referenceCurrentState is the in-context `current_state` spelling of the
// EHRbase FLAT corpus (ehrbase_conformance_action.json, test_all_types).
var referenceCurrentState = map[string]any{
	minimalIsm + "/current_state|code":        "532",
	minimalIsm + "/current_state|value":       "completed",
	minimalIsm + "/current_state|terminology": "openehr",
}

// TestREQ053InContextIsmTransitionDecodesReferenceSpelling — REQ-053, REQ-106.
// The EHRbase corpus spells an ACTION's current state in context,
// `<action>/ism_transition/current_state|code|value|terminology`. For an ACTION
// whose template constrains no ISM transition the Web Template has that node,
// so the spelling decodes, in every mode and in both formats, into the
// ACTION's current_state, and the WithTemplate decode validates.
func TestREQ053InContextIsmTransitionDecodesReferenceSpelling(t *testing.T) {
	wt, compiled := minimalActionWT(t)
	flat := mustJSON(t, minimalActionBody(referenceCurrentState))
	structured, err := FlatToStructured(flat)
	if err != nil {
		t.Fatalf("FlatToStructured: %v", err)
	}
	for format, decode := range map[string]func([]byte, *webtemplate.WebTemplate, ...Option) (*rm.Composition, error){
		"FLAT": UnmarshalFlat, "STRUCTURED": UnmarshalStructured,
	} {
		body := flat
		if format == "STRUCTURED" {
			body = structured
		}
		for mode, opts := range map[string][]Option{
			"default":                 nil,
			"WithStrict":              {WithStrict()},
			"WithTemplate+WithStrict": {WithTemplate(compiled), WithStrict()},
		} {
			t.Run(format+"/"+mode, func(t *testing.T) {
				comp, err := decode(body, wt, opts...)
				if err != nil {
					t.Fatalf("Unmarshal %s (%s): %v", format, mode, err)
				}
				a, ok := comp.Content[0].(*rm.Action)
				if !ok {
					t.Fatalf("content[0] = %T, want *rm.Action", comp.Content[0])
				}
				got := a.IsmTransition.CurrentState
				if got.Value != "completed" || got.DefiningCode.CodeString != "532" || got.DefiningCode.TerminologyID.Value != "openehr" {
					t.Errorf("Unmarshal %s (%s) current_state = %+v, want openehr::532|completed", format, mode, got)
				}
				if a.IsmTransition.CareflowStep != nil || a.IsmTransition.Transition != nil {
					t.Errorf("Unmarshal %s (%s) ISM transition = %+v, want no careflow_step or transition: the body carries none", format, mode, a.IsmTransition)
				}
				if mode == "WithTemplate+WithStrict" {
					for _, is := range validation.ValidateComposition(comp, compiled).Issues {
						t.Errorf("decoded composition: %s at %s: %s", is.Code, is.Path, is.Detail)
					}
				}
			})
		}
	}
}

// TestREQ053InContextIsmTransitionInventsNoState — REQ-053. The in-context ISM
// transition is not archetyped, so decode gives it no careflow step, and keeps
// no node id or name on it; and a body that spells no current state gets no
// ISM transition at all, even with WithTemplate: the decode invents no state,
// and the validator's `required` is the right answer.
func TestREQ053InContextIsmTransitionInventsNoState(t *testing.T) {
	wt, compiled := minimalActionWT(t)
	tmpl := buildTemplateIndex(compiled)
	compJSON, err := decodeFlat(minimalActionBody(referenceCurrentState), wt, tmpl, nil)
	if err != nil {
		t.Fatalf("decodeFlat: %v", err)
	}
	content, _ := compJSON["content"].([]any)
	action, _ := content[0].(map[string]any)
	ism, _ := action["ism_transition"].(map[string]any)
	if got := slices.Sorted(maps.Keys(ism)); !slices.Equal(got, []string{"_type", "current_state"}) {
		t.Errorf("decoded in-context ISM_TRANSITION members = %v, want only [_type current_state]", got)
	}

	comp, err := UnmarshalFlat(mustJSON(t, minimalActionBody(nil)), wt, WithTemplate(compiled), WithStrict())
	if err != nil {
		t.Fatalf("UnmarshalFlat (WithTemplate): %v", err)
	}
	a, ok := comp.Content[0].(*rm.Action)
	if !ok {
		t.Fatalf("content[0] = %T, want *rm.Action", comp.Content[0])
	}
	if a.IsmTransition.CurrentState.DefiningCode.CodeString != "" {
		t.Errorf("UnmarshalFlat (WithTemplate) invented current_state %+v for a body that spells none", a.IsmTransition.CurrentState)
	}
	const want = "/content[openEHR-EHR-ACTION.minimal_2.v1]/ism_transition"
	issues := validation.ValidateComposition(comp, compiled).Issues
	if !slices.ContainsFunc(issues, func(is validation.Issue) bool { return is.Code == "required" && is.Path == want }) {
		t.Errorf("ValidateComposition issues = %v, want `required` at %s", issues, want)
	}
}

// TestREQ170InContextIsmTransitionNodeRefusesUnmodelledKeys — REQ-170, REQ-140.
// The in-context `ism_transition` node of an ACTION whose template constrains
// no ISM transition models `current_state` and `transition`, and carries the
// `_reason` family, but has no careflow step: the template names none. Under
// it, the reference's `careflow_step` leaf stays refused in every mode and in
// both formats, carrying none of the value, and FLAT names the key; the
// refusal names the attribute it refuses and no requirement id.
func TestREQ170InContextIsmTransitionNodeRefusesUnmodelledKeys(t *testing.T) {
	wt, _ := minimalActionWT(t)
	const secret = "secret-value-0451"
	for _, tc := range []struct {
		name, named, attr string
		keys              []string
	}{
		{
			"careflow_step leaf", minimalIsm + "/careflow_step|code", "an ACTION's ISM transition careflow_step",
			[]string{minimalIsm + "/careflow_step|code", minimalIsm + "/careflow_step|value", minimalIsm + "/careflow_step|terminology"},
		},
		{"bare careflow_step leaf", minimalIsm + "/careflow_step", "an ACTION's ISM transition careflow_step", []string{minimalIsm + "/careflow_step"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			keys := maps.Clone(referenceCurrentState)
			for _, k := range tc.keys {
				keys[k] = secret
			}
			body := minimalActionBody(keys)
			for format, err := range map[string]error{
				"FLAT":       refusedInEveryMode(t, wt, body),
				"STRUCTURED": structuredRefusedInEveryMode(t, wt, body),
			} {
				if !errors.Is(err, ErrNotModelled) || !errors.Is(err, ErrUnknownPath) {
					t.Errorf("%s err = %v, want the ErrNotModelled refusal, which matches ErrUnknownPath too", format, err)
				}
				// STRUCTURED names the key as its FLAT form, with indexes.
				if format == "FLAT" && !strings.Contains(err.Error(), strconv.Quote(tc.named)) {
					t.Errorf("%s err = %v, want it to name %q", format, err, tc.named)
				}
				if !strings.Contains(err.Error(), tc.attr) || strings.Contains(err.Error(), "REQ-") {
					t.Errorf("%s err = %v, want it to name %q and no requirement id", format, err, tc.attr)
				}
				if strings.Contains(err.Error(), secret) {
					t.Errorf("%s err = %v carries the key's value", format, err)
				}
			}
		})
	}
}

// minimalReferenceKeys is the rest of the reference's in-context spelling of
// the minimal_action_2 ACTION beside [referenceCurrentState]: the transition's
// `transition`, two reasons, a plain and a coded one, and the ACTION's own
// `time`, which differs from the body's ctx/time.
var minimalReferenceKeys = map[string]any{
	minimalIsm + "/transition|code":           "548",
	minimalIsm + "/transition|value":          "finish",
	minimalIsm + "/transition|terminology":    "openehr",
	minimalIsm + "/_reason:0":                 "reason 1",
	minimalIsm + "/_reason:1|code":            "282291009",
	minimalIsm + "/_reason:1|value":           "a coded reason",
	minimalIsm + "/_reason:1|terminology":     "SNOMED-CT",
	minimalAction + "/time":                   "2019-03-22T22:26:01.127+01:00",
	minimalIsm + "/current_state|code":        "532",
	minimalIsm + "/current_state|value":       "completed",
	minimalIsm + "/current_state|terminology": "openehr",
}

// TestREQ053InContextIsmTransitionReadsReferenceKeys — REQ-053, REQ-140. Under
// the in-context node of an ACTION whose template constrains no ISM
// transition, the reference's `transition` leaf and `_reason:N` family decode,
// and so does the ACTION's own `time`, in every mode and in both formats: a
// plain reason as a DV_TEXT, a coded one as a DV_CODED_TEXT, and the time as
// written, not replaced by ctx/time. The WithTemplate decode validates, and
// encode writes each key back as the body spells it.
func TestREQ053InContextIsmTransitionReadsReferenceKeys(t *testing.T) {
	wt, compiled := minimalActionWT(t)
	flat := mustJSON(t, minimalActionBody(minimalReferenceKeys))
	structured, err := FlatToStructured(flat)
	if err != nil {
		t.Fatalf("FlatToStructured: %v", err)
	}
	for format, decode := range map[string]func([]byte, *webtemplate.WebTemplate, ...Option) (*rm.Composition, error){
		"FLAT": UnmarshalFlat, "STRUCTURED": UnmarshalStructured,
	} {
		body := flat
		if format == "STRUCTURED" {
			body = structured
		}
		for mode, opts := range map[string][]Option{
			"default":                 nil,
			"WithStrict":              {WithStrict()},
			"WithTemplate+WithStrict": {WithTemplate(compiled), WithStrict()},
		} {
			t.Run(format+"/"+mode, func(t *testing.T) {
				comp, err := decode(body, wt, opts...)
				if err != nil {
					t.Fatalf("Unmarshal %s (%s): %v", format, mode, err)
				}
				a, ok := comp.Content[0].(*rm.Action)
				if !ok {
					t.Fatalf("content[0] = %T, want *rm.Action", comp.Content[0])
				}
				tr := a.IsmTransition
				if tr.Transition == nil || tr.Transition.DefiningCode.CodeString != "548" ||
					tr.Transition.DefiningCode.TerminologyID.Value != "openehr" || tr.Transition.Value != "finish" {
					t.Errorf("transition = %+v, want openehr::548|finish", tr.Transition)
				}
				if len(tr.Reason) != 2 {
					t.Fatalf("reason = %+v, want two reasons", tr.Reason)
				}
				if _, coded := tr.Reason[0].GetDefiningCode(); coded || tr.Reason[0].GetValue() != "reason 1" {
					t.Errorf("reason[0] = %+v, want the DV_TEXT %q", tr.Reason[0], "reason 1")
				}
				if dc, coded := tr.Reason[1].GetDefiningCode(); !coded || dc.CodeString != "282291009" ||
					dc.TerminologyID.Value != "SNOMED-CT" || tr.Reason[1].GetValue() != "a coded reason" {
					t.Errorf("reason[1] = %+v, want the DV_CODED_TEXT SNOMED-CT::282291009|a coded reason", tr.Reason[1])
				}
				if want := minimalReferenceKeys[minimalAction+"/time"]; a.Time.Value != want {
					t.Errorf("ACTION time = %q, want %q as the body spells it", a.Time.Value, want)
				}
				if mode == "WithTemplate+WithStrict" {
					for _, is := range validation.ValidateComposition(comp, compiled).Issues {
						t.Errorf("decoded composition: %s at %s: %s", is.Code, is.Path, is.Detail)
					}
				}
				holdsKeys(t, reencodeRMAttr(t, wt, comp), minimalReferenceKeys)
			})
		}
	}
}

// ismStateTerminologyShapes are the two spellings of a current state that names
// no terminology: no `|terminology` key, and a null one, which REQ-053 reads as
// absent.
var ismStateTerminologyShapes = []struct {
	name  string
	shape func(keys map[string]any, termKey string)
}{
	{name: "absent", shape: func(m map[string]any, k string) { delete(m, k) }},
	{name: "null", shape: func(m map[string]any, k string) { m[k] = nil }},
}

// TestREQ053InContextIsmCurrentStateImpliesOpenehr — REQ-053, REQ-106. The
// in-context `current_state` leaf advertises `|code` and `|value` inputs only,
// as the reference does, so a body that follows them names no terminology.
// ISM_TRANSITION's Current_state_valid admits the openEHR terminology alone, so
// decode reads that current state as `openehr`, in every mode and in both
// formats, and the WithTemplate decode validates. Without the implied
// terminology the decoded CODE_PHRASE had no terminology_id, which validation
// reported as `required`.
func TestREQ053InContextIsmCurrentStateImpliesOpenehr(t *testing.T) {
	wt, compiled := minimalActionWT(t)
	const termKey = minimalIsm + "/current_state|terminology"
	for _, sh := range ismStateTerminologyShapes {
		keys := maps.Clone(referenceCurrentState)
		sh.shape(keys, termKey)
		flat := mustJSON(t, minimalActionBody(keys))
		structured, err := FlatToStructured(flat)
		if err != nil {
			t.Fatalf("FlatToStructured (%s): %v", sh.name, err)
		}
		for format, decode := range map[string]func([]byte, *webtemplate.WebTemplate, ...Option) (*rm.Composition, error){
			"FLAT": UnmarshalFlat, "STRUCTURED": UnmarshalStructured,
		} {
			body := flat
			if format == "STRUCTURED" {
				body = structured
			}
			for mode, opts := range map[string][]Option{
				"default":                 nil,
				"WithStrict":              {WithStrict()},
				"WithTemplate+WithStrict": {WithTemplate(compiled), WithStrict()},
			} {
				t.Run(sh.name+"/"+format+"/"+mode, func(t *testing.T) {
					comp, err := decode(body, wt, opts...)
					if err != nil {
						t.Fatalf("Unmarshal %s (%s): %v", format, mode, err)
					}
					a, ok := comp.Content[0].(*rm.Action)
					if !ok {
						t.Fatalf("content[0] = %T, want *rm.Action", comp.Content[0])
					}
					got := a.IsmTransition.CurrentState
					if got.DefiningCode.TerminologyID.Value != "openehr" || got.DefiningCode.CodeString != "532" || got.Value != "completed" {
						t.Errorf("Unmarshal %s (%s) current_state = %+v, want openehr::532|completed", format, mode, got)
					}
					if mode == "WithTemplate+WithStrict" {
						for _, is := range validation.ValidateComposition(comp, compiled).Issues {
							t.Errorf("decoded composition: %s at %s: %s", is.Code, is.Path, is.Detail)
						}
					}
				})
			}
		}
	}
}

// TestREQ053ArchetypedIsmCurrentStateImpliesOpenehr — REQ-053. The same reading
// holds at the archetyped `ism_transition[atNNNN]` nodes, with and without the
// compiled template: a current state that names no terminology decodes as
// `openehr`, and re-encodes, in the in-context spelling, with
// `|terminology: "openehr"`.
func TestREQ053ArchetypedIsmCurrentStateImpliesOpenehr(t *testing.T) {
	wt, compiled := conformanceWT(t)
	for _, tc := range []struct{ node, state, step string }{
		{node: "transition", state: "524", step: "at0005"},
		{node: "transition_1", state: "532", step: "at0006"},
	} {
		termKey := rmattrAction + "/" + tc.node + "/current_state|terminology"
		for _, sh := range ismStateTerminologyShapes {
			for mode, opts := range map[string][]Option{
				"default":      nil,
				"WithTemplate": {WithTemplate(compiled)},
			} {
				t.Run(tc.node+"/"+sh.name+"/"+mode, func(t *testing.T) {
					keys := ismTransitionKeys(tc.node, tc.state, tc.step)
					sh.shape(keys, termKey)
					comp, err := UnmarshalFlat(mustJSON(t, rmattrBody(keys)), wt, opts...)
					if err != nil {
						t.Fatalf("UnmarshalFlat (%s): %v", mode, err)
					}
					got := firstAction(t, comp).IsmTransition.CurrentState
					if got.DefiningCode.TerminologyID.Value != "openehr" || got.DefiningCode.CodeString != tc.state {
						t.Errorf("UnmarshalFlat (%s) current_state = %+v, want openehr::%s", mode, got, tc.state)
					}
					const reKey = conformanceIsm + "/current_state|terminology"
					if re := reencodeRMAttr(t, wt, comp); re[reKey] != "openehr" {
						t.Errorf("re-encode of %s = %#v, want \"openehr\"", reKey, re[reKey])
					}
				})
			}
		}
	}
}

// TestREQ053IsmCurrentStateTerminologyReadAsWritten — REQ-053. A current state
// that names a terminology keeps it, even one Current_state_valid does not
// admit: decode implies `openehr` only where the body names none, and leaves
// a wrong one for validation to report.
func TestREQ053IsmCurrentStateTerminologyReadAsWritten(t *testing.T) {
	t.Run("in-context", func(t *testing.T) {
		wt, compiled := minimalActionWT(t)
		keys := maps.Clone(referenceCurrentState)
		keys[minimalIsm+"/current_state|terminology"] = "local"
		comp, err := UnmarshalFlat(mustJSON(t, minimalActionBody(keys)), wt, WithTemplate(compiled), WithStrict())
		if err != nil {
			t.Fatalf("UnmarshalFlat: %v", err)
		}
		a, ok := comp.Content[0].(*rm.Action)
		if !ok {
			t.Fatalf("content[0] = %T, want *rm.Action", comp.Content[0])
		}
		if got := a.IsmTransition.CurrentState.DefiningCode.TerminologyID.Value; got != "local" {
			t.Errorf("current_state terminology = %q, want %q as written", got, "local")
		}
		if issues := validation.ValidateComposition(comp, compiled).Issues; len(issues) == 0 {
			t.Error("ValidateComposition reports no issue for a current_state coded in `local`, want one")
		}
	})
	t.Run("archetyped", func(t *testing.T) {
		wt, _ := conformanceWT(t)
		keys := ismTransitionKeys("transition_1", "532", "at0006")
		keys[rmattrAction+"/transition_1/current_state|terminology"] = "local"
		got := firstAction(t, decodeRMAttr(t, wt, rmattrBody(keys))).IsmTransition.CurrentState
		if got.DefiningCode.TerminologyID.Value != "local" {
			t.Errorf("current_state terminology = %q, want %q as written", got.DefiningCode.TerminologyID.Value, "local")
		}
	})
}
