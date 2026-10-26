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
	"strconv"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
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

// TestActionIsmTransitionCareflowStepTerminologyDefaultsToLocal — REQ-053,
// REQ-121. A careflow step that names no terminology (none, or a blank one) takes
// `local`, the only terminology an archetype's at-code belongs to, so the decoded
// CODE_PHRASE carries its RM-mandatory terminology_id and the transition
// re-encodes under its node.
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
			if got[rmattrAction+"/transition/current_state|code"] != "524" {
				t.Errorf("re-encode lost the transition: %v", got)
			}
		})
	}
}

// TestActionIsmTransitionBlankTerminologyEncodes — REQ-121, REQ-140. Encode
// reads a careflow step the way decode does: a blank terminology counts as
// `local`, so an ACTION built in Go with only the careflow step's code set keeps
// its transition instead of losing it with no error.
func TestActionIsmTransitionBlankTerminologyEncodes(t *testing.T) {
	wt, _ := conformanceWT(t)
	comp := decodeRMAttr(t, wt, rmattrBody(ismTransitionKeys("transition", "524", "at0005")))
	firstAction(t, comp).IsmTransition.CareflowStep.DefiningCode.TerminologyID.Value = ""
	got := reencodeRMAttr(t, wt, comp)
	if got[rmattrAction+"/transition/current_state|code"] != "524" {
		t.Errorf("re-encode lost the transition whose careflow step has a blank terminology: %v", got)
	}
}

// TestActionIsmTransitionCareflowStepCodedOtherwiseRefused — REQ-053, REQ-121. A
// careflow step whose code is not its node's id names another node, so encode
// would move the transition there. One carrying the node's id in a terminology
// other than `local` names no archetype node at all: an at-code belongs to the
// archetype's own terminology. Decode refuses both, naming the key's node and the
// code, never the clinical value.
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
		{name: "no node's id", keys: ismTransitionKeys("transition2", "532", "at0099")},
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
	opt, err := template.ParseFile(fixtures.TemplateOpt("minimal_action_2"))
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
// The in-context `ism_transition` node models `current_state` only. Under it,
// the reference's `transition` and `careflow_step` leaves and its `_reason`
// family stay refused in every mode and in both formats, carrying none of the
// value, and FLAT names the key; the refusal of `transition` and `reason`
// cites REQ-140, the other REQ-170. The ACTION's own `time` leaf stays refused
// too.
func TestREQ170InContextIsmTransitionNodeRefusesUnmodelledKeys(t *testing.T) {
	wt, _ := minimalActionWT(t)
	const secret = "secret-value-0451"
	for _, tc := range []struct {
		name, named, cite string
		keys              []string
	}{
		{
			"transition leaf", minimalIsm + "/transition|code", "REQ-140",
			[]string{minimalIsm + "/transition|code", minimalIsm + "/transition|value", minimalIsm + "/transition|terminology"},
		},
		{
			"careflow_step leaf", minimalIsm + "/careflow_step|code", "REQ-170",
			[]string{minimalIsm + "/careflow_step|code", minimalIsm + "/careflow_step|value", minimalIsm + "/careflow_step|terminology"},
		},
		{"reason family", minimalIsm + "/_reason:0", "REQ-140", []string{minimalIsm + "/_reason:0"}},
		{"ACTION time leaf", minimalAction + "/time", "REQ-170", []string{minimalAction + "/time"}},
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
				if !errors.Is(err, ErrUnknownPath) || !strings.Contains(err.Error(), "in-context spelling") {
					t.Errorf("%s err = %v, want the in-context ErrUnknownPath refusal", format, err)
				}
				// STRUCTURED names the key as its FLAT form, with indexes.
				if format == "FLAT" && !strings.Contains(err.Error(), strconv.Quote(tc.named)) {
					t.Errorf("%s err = %v, want it to name %q", format, err, tc.named)
				}
				if !strings.Contains(err.Error(), "("+tc.cite+",") {
					t.Errorf("%s err = %v, want it to cite %s", format, err, tc.cite)
				}
				if strings.Contains(err.Error(), secret) {
					t.Errorf("%s err = %v carries the key's value", format, err)
				}
			}
		})
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
// `openehr`, and re-encodes with `|terminology: "openehr"`.
func TestREQ053ArchetypedIsmCurrentStateImpliesOpenehr(t *testing.T) {
	wt, compiled := conformanceWT(t)
	for _, tc := range []struct{ node, state, step string }{
		{node: "transition", state: "524", step: "at0005"},
		{node: "transition2", state: "532", step: "at0006"},
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
					if re := reencodeRMAttr(t, wt, comp); re[termKey] != "openehr" {
						t.Errorf("re-encode of %s = %#v, want \"openehr\"", termKey, re[termKey])
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
		keys := ismTransitionKeys("transition2", "532", "at0006")
		keys[rmattrAction+"/transition2/current_state|terminology"] = "local"
		got := firstAction(t, decodeRMAttr(t, wt, rmattrBody(keys))).IsmTransition.CurrentState
		if got.DefiningCode.TerminologyID.Value != "local" {
			t.Errorf("current_state terminology = %q, want %q as written", got.DefiningCode.TerminologyID.Value, "local")
		}
	})
}
