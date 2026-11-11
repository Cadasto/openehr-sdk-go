package simplified

// REQ-053 / REQ-140 — the in-context `ism_transition` node of an ACTION whose
// template models archetyped per-step `ism_transition[atNNNN]` nodes: the
// node merges them, carrying every step's careflow-step code with the current
// states that step admits. Decode reads the reference's spelling there,
// completing a careflow step from that list; encode writes it.

import (
	"bytes"
	"errors"
	"maps"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/template/webtemplate"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// referenceActionKeys are the keys of the PROBE-086 corpus body
// ehrbase_conformance_action.json that spell its ACTION's transition and time,
// as the reference writes them.
var referenceActionKeys = map[string]any{
	conformanceIsm + "/current_state|code":        "532",
	conformanceIsm + "/current_state|value":       "completed",
	conformanceIsm + "/current_state|terminology": "openehr",
	conformanceIsm + "/transition|code":           "548",
	conformanceIsm + "/transition|value":          "finish",
	conformanceIsm + "/transition|terminology":    "openehr",
	conformanceIsm + "/careflow_step|code":        "at0006",
	conformanceIsm + "/careflow_step|value":       "transition",
	conformanceIsm + "/careflow_step|terminology": "local",
	conformanceIsm + "/_reason:0":                 "reason 1",
	rmattrAction + "/time":                        "2022-01-31T10:33:28.72414+01:00",
}

// actionIn returns the first ACTION comp holds, at its top level or in a
// section.
func actionIn(t *testing.T, comp *rm.Composition) *rm.Action {
	t.Helper()
	for _, item := range comp.Content {
		switch x := item.(type) {
		case *rm.Action:
			return x
		case *rm.Section:
			for _, it := range x.Items {
				if a, ok := it.(*rm.Action); ok {
					return a
				}
			}
		}
	}
	t.Fatal("the composition holds no ACTION")
	return nil
}

// bothFormats returns the FLAT body flat and its STRUCTURED form, each with the
// function that decodes it.
func bothFormats(t *testing.T, flat map[string]any) map[string]func(*webtemplate.WebTemplate, ...Option) (*rm.Composition, error) {
	t.Helper()
	b := mustJSON(t, flat)
	structured, err := FlatToStructured(b)
	if err != nil {
		t.Fatalf("FlatToStructured: %v", err)
	}
	return map[string]func(*webtemplate.WebTemplate, ...Option) (*rm.Composition, error){
		"FLAT": func(wt *webtemplate.WebTemplate, opts ...Option) (*rm.Composition, error) {
			return UnmarshalFlat(b, wt, opts...)
		},
		"STRUCTURED": func(wt *webtemplate.WebTemplate, opts ...Option) (*rm.Composition, error) {
			return UnmarshalStructured(structured, wt, opts...)
		},
	}
}

// TestREQ053MergedIsmTransitionDecodesReferenceSpelling — REQ-053, REQ-140,
// PROBE-086. The corpus body spells its ACTION's transition at the in-context
// node, which merges the template's two per-step nodes: its current state,
// transition, careflow step and one reason, and the ACTION's time. They
// decode in every mode and in both formats, the WithTemplate decode
// validates, and encode writes each key back as the corpus spells it.
func TestREQ053MergedIsmTransitionDecodesReferenceSpelling(t *testing.T) {
	wt, compiled := conformanceWT(t)
	for format, decode := range bothFormats(t, rmattrBody(referenceActionKeys)) {
		for mode, opts := range map[string][]Option{
			"default":                 nil,
			"WithStrict":              {WithStrict()},
			"WithTemplate+WithStrict": {WithTemplate(compiled), WithStrict()},
		} {
			t.Run(format+"/"+mode, func(t *testing.T) {
				comp, err := decode(wt, opts...)
				if err != nil {
					t.Fatalf("Unmarshal %s (%s): %v", format, mode, err)
				}
				a := firstAction(t, comp)
				tr := a.IsmTransition
				if got := tr.CurrentState; got.DefiningCode.CodeString != "532" || got.DefiningCode.TerminologyID.Value != "openehr" || got.Value != "completed" {
					t.Errorf("current_state = %+v, want openehr::532|completed", got)
				}
				if got := tr.Transition; got == nil || got.DefiningCode.CodeString != "548" || got.DefiningCode.TerminologyID.Value != "openehr" || got.Value != "finish" {
					t.Errorf("transition = %+v, want openehr::548|finish", got)
				}
				if got := tr.CareflowStep; got == nil || got.DefiningCode.CodeString != "at0006" || got.DefiningCode.TerminologyID.Value != "local" || got.Value != "transition" {
					t.Errorf("careflow_step = %+v, want local::at0006|transition", got)
				}
				if len(tr.Reason) != 1 || tr.Reason[0].GetValue() != "reason 1" {
					t.Errorf("reason = %+v, want the one DV_TEXT %q", tr.Reason, "reason 1")
				}
				if want := referenceActionKeys[rmattrAction+"/time"]; a.Time.Value != want {
					t.Errorf("ACTION time = %q, want %q", a.Time.Value, want)
				}
				if mode == "WithTemplate+WithStrict" {
					for _, is := range validation.ValidateComposition(comp, compiled).Issues {
						t.Errorf("decoded composition: %s at %s: %s", is.Code, is.Path, is.Detail)
					}
				}
				got := reencodeRMAttr(t, wt, comp)
				holdsKeys(t, got, referenceActionKeys)
				if keys := perStepKeys(got); len(keys) > 0 {
					t.Errorf("re-encode wrote %v under a per-step node, want the in-context spelling alone", keys)
				}
			})
		}
	}
}

// TestREQ053MergedCareflowStepCompletion — REQ-053, REQ-121. At the merged
// in-context node, with and without the template, a careflow step that names
// no terminology (no `|terminology`, a null or a blank one) takes `local`, and
// one with no `|value` takes the label the node's careflow_step input lists
// for its code. A careflow step there is never refused for its code: encode no
// longer moves a transition between nodes, so a code the input does not list,
// another step's code, and a terminology other than `local` are kept as
// written, for validation to judge. A code whose value the input does not
// give and the body leaves out is a missing `|value`, refused naming the key.
func TestREQ053MergedCareflowStepCompletion(t *testing.T) {
	wt, compiled := conformanceWT(t)
	const step = conformanceIsm + "/careflow_step"
	edited := func(edit func(map[string]any)) map[string]any {
		keys := inContextIsmKeys("524", "at0005", "a step")
		edit(keys)
		return keys
	}
	modes := map[string][]Option{"default": nil, "WithTemplate": {WithTemplate(compiled)}}
	for _, tc := range []struct {
		name                     string
		keys                     map[string]any
		code, value, terminology string
	}{
		{"as written", edited(func(map[string]any) {}), "at0005", "a step", "local"},
		{"no terminology", edited(func(m map[string]any) { delete(m, step+"|terminology") }), "at0005", "a step", "local"},
		{"a null terminology", edited(func(m map[string]any) { m[step+"|terminology"] = nil }), "at0005", "a step", "local"},
		{"a blank terminology", edited(func(m map[string]any) { m[step+"|terminology"] = "" }), "at0005", "a step", "local"},
		{"no value", edited(func(m map[string]any) { delete(m, step+"|value") }), "at0005", "transition", "local"},
		{"a null value", edited(func(m map[string]any) { m[step+"|value"] = nil }), "at0005", "transition", "local"},
		{"neither", edited(func(m map[string]any) { delete(m, step+"|value"); delete(m, step+"|terminology") }), "at0005", "transition", "local"},
		{"another step's code", edited(func(m map[string]any) { m[step+"|code"] = "at0006" }), "at0006", "a step", "local"},
		{"a code the input does not list", edited(func(m map[string]any) { m[step+"|code"] = "at0099" }), "at0099", "a step", "local"},
		{"another terminology", edited(func(m map[string]any) { m[step+"|terminology"] = "SNOMED-CT" }), "at0005", "a step", "SNOMED-CT"},
	} {
		for mode, opts := range modes {
			t.Run(tc.name+"/"+mode, func(t *testing.T) {
				comp, err := UnmarshalFlat(mustJSON(t, rmattrBody(tc.keys)), wt, opts...)
				if err != nil {
					t.Fatalf("UnmarshalFlat (%s): %v", mode, err)
				}
				got := firstAction(t, comp).IsmTransition.CareflowStep
				if got == nil || got.DefiningCode.CodeString != tc.code || got.Value != tc.value || got.DefiningCode.TerminologyID.Value != tc.terminology {
					t.Errorf("careflow_step = %+v, want %s::%s|%s", got, tc.terminology, tc.code, tc.value)
				}
			})
		}
	}
	unlisted := edited(func(m map[string]any) { m[step+"|code"] = "at0099"; delete(m, step+"|value") })
	for mode, opts := range modes {
		t.Run("an unlisted code with no value/"+mode, func(t *testing.T) {
			_, err := UnmarshalFlat(mustJSON(t, rmattrBody(unlisted)), wt, opts...)
			if !errors.Is(err, ErrUnsupportedDatatype) {
				t.Fatalf("UnmarshalFlat (%s) = %v, want ErrUnsupportedDatatype", mode, err)
			}
			if msg := err.Error(); !strings.Contains(msg, step) || !strings.Contains(msg, "|value") {
				t.Errorf("err = %q, want it to name %s and its missing |value", msg, step)
			}
		})
	}
}

// TestREQ053MergedIsmTransitionInfersCareflowStep — REQ-053, REQ-121. A body
// that spells a transition at the merged node and no careflow step gets the
// one the node's careflow_step input gives its current state: the one item
// whose currentStates hold the state's code, as a `local` code with the
// item's label as its value. Where no item, or more than one, holds the state,
// decode gives no careflow step. The WithTemplate decode of an inferred step
// validates.
func TestREQ053MergedIsmTransitionInfersCareflowStep(t *testing.T) {
	wt, compiled := conformanceWT(t)
	// shared lists 524 among the current states of both careflow steps, so no
	// one step holds it.
	shared := editWTNode(t, wt, conformanceIsm+"/careflow_step", func(n *webtemplate.Node) {
		n.Inputs = slices.Clone(n.Inputs)
		n.Inputs[0].List = slices.Clone(n.Inputs[0].List)
		for i := range n.Inputs[0].List {
			n.Inputs[0].List[i].CurrentStates = []string{"524", "532"}
		}
	})
	stateOnly := func(state string) map[string]any {
		keys := inContextIsmKeys(state, "", "")
		for k := range keys {
			if strings.Contains(k, "/careflow_step|") {
				delete(keys, k)
			}
		}
		return keys
	}
	for _, tc := range []struct {
		name        string
		wt          *webtemplate.WebTemplate
		state, step string
	}{
		{name: "the first step's state", wt: wt, state: "524", step: "at0005"},
		{name: "the second step's state", wt: wt, state: "532", step: "at0006"},
		{name: "a state no step lists", wt: wt, state: "526"},
		{name: "a state two steps list", wt: shared, state: "524"},
	} {
		for mode, opts := range map[string][]Option{"default": nil, "WithTemplate": {WithTemplate(compiled)}} {
			t.Run(tc.name+"/"+mode, func(t *testing.T) {
				comp, err := UnmarshalFlat(mustJSON(t, rmattrBody(stateOnly(tc.state))), tc.wt, opts...)
				if err != nil {
					t.Fatalf("UnmarshalFlat (%s): %v", mode, err)
				}
				got := firstAction(t, comp).IsmTransition.CareflowStep
				if tc.step == "" {
					if got != nil {
						t.Errorf("careflow_step = %+v, want none: no one step holds %s", got, tc.state)
					}
					return
				}
				if got == nil || got.DefiningCode.CodeString != tc.step || got.DefiningCode.TerminologyID.Value != "local" || got.Value != "transition" {
					t.Fatalf("careflow_step = %+v, want local::%s|transition", got, tc.step)
				}
				if mode == "WithTemplate" {
					for _, is := range validation.ValidateComposition(comp, compiled).Issues {
						t.Errorf("decoded composition: %s at %s: %s", is.Code, is.Path, is.Detail)
					}
				}
			})
		}
	}
}

// TestREQ053IsmTransitionImpliesOpenehr — REQ-053. The in-context `transition`
// leaf advertises `|code` and `|value` only, as the reference's does. RM
// Transition_valid admits a code of the openEHR terminology alone, so decode
// reads a transition that names no terminology (no `|terminology`, or a null
// one) as `openehr`, as it reads a current state, at the unconstrained and at
// the merged node, with and without the template. One that names a
// terminology keeps it, for validation to judge.
func TestREQ053IsmTransitionImpliesOpenehr(t *testing.T) {
	minimalWT, minimalCompiled := minimalActionWT(t)
	wt, compiled := conformanceWT(t)
	for _, node := range []struct {
		name     string
		wt       *webtemplate.WebTemplate
		compiled Option
		ism      string
		body     func(map[string]any) map[string]any
		state    map[string]any
	}{
		{"unconstrained", minimalWT, WithTemplate(minimalCompiled), minimalIsm, minimalActionBody, referenceCurrentState},
		{"merged", wt, WithTemplate(compiled), conformanceIsm, rmattrBody, inContextIsmKeys("532", "at0006", "transition")},
	} {
		termKey := node.ism + "/transition|terminology"
		for _, tc := range []struct {
			name  string
			shape func(map[string]any)
			want  string
		}{
			{"absent", func(m map[string]any) { delete(m, termKey) }, "openehr"},
			{"null", func(m map[string]any) { m[termKey] = nil }, "openehr"},
			{"as written", func(m map[string]any) { m[termKey] = "local" }, "local"},
		} {
			for mode, opts := range map[string][]Option{"default": nil, "WithTemplate": {node.compiled}} {
				t.Run(node.name+"/"+tc.name+"/"+mode, func(t *testing.T) {
					keys := maps.Clone(node.state)
					keys[node.ism+"/transition|code"] = "548"
					keys[node.ism+"/transition|value"] = "finish"
					keys[termKey] = "openehr"
					tc.shape(keys)
					comp, err := UnmarshalFlat(mustJSON(t, node.body(keys)), node.wt, opts...)
					if err != nil {
						t.Fatalf("UnmarshalFlat (%s): %v", mode, err)
					}
					got := actionIn(t, comp).IsmTransition.Transition
					if got == nil || got.DefiningCode.CodeString != "548" || got.DefiningCode.TerminologyID.Value != tc.want {
						t.Errorf("transition = %+v, want %s::548", got, tc.want)
					}
				})
			}
		}
	}
}

// TestREQ140IsmTransitionReasonFamily — REQ-140, REQ-053. ISM_TRANSITION
// `reason`, a list of DV_TEXT, is the `_reason:N` family at any ISM transition
// node, the in-context one and an archetyped per-step one alike. Each reason
// takes the DV_TEXT leaf form: a bare value with its `|formatting`, or the
// `|code` substitution of a coded text. Encode writes them back under the
// in-context node in list order, and STRUCTURED reads them as a member list.
// A sparse `:N` is refused as for every list family.
func TestREQ140IsmTransitionReasonFamily(t *testing.T) {
	wt, _ := conformanceWT(t)
	reasons := func(node string) map[string]any {
		return map[string]any{
			node + "/_reason:0":             "reason 1",
			node + "/_reason:0|formatting":  "plain",
			node + "/_reason:1|code":        "282291009",
			node + "/_reason:1|value":       "a coded reason",
			node + "/_reason:1|terminology": "SNOMED-CT",
		}
	}
	wantInContext := reasons(conformanceIsm)
	for _, tc := range []struct {
		name string
		keys map[string]any
	}{
		{"in context", mergeRMAttr(inContextIsmKeys("524", "at0005", "transition"), reasons(conformanceIsm))},
		{"per step", mergeRMAttr(ismTransitionKeys("transition", "524", "at0005"), reasons(rmattrAction+"/transition"))},
	} {
		for format, decode := range bothFormats(t, rmattrBody(tc.keys)) {
			t.Run(tc.name+"/"+format, func(t *testing.T) {
				comp, err := decode(wt, WithStrict())
				if err != nil {
					t.Fatalf("Unmarshal %s: %v", format, err)
				}
				got := firstAction(t, comp).IsmTransition.Reason
				if len(got) != 2 {
					t.Fatalf("reason = %+v, want two reasons", got)
				}
				if plain, ok := got[0].(*rm.DVText); !ok || plain.Value != "reason 1" || plain.Formatting == nil || *plain.Formatting != "plain" {
					t.Errorf("reason[0] = %#v, want the DV_TEXT %q with formatting %q", got[0], "reason 1", "plain")
				}
				if dc, coded := got[1].GetDefiningCode(); !coded || dc.CodeString != "282291009" || dc.TerminologyID.Value != "SNOMED-CT" || got[1].GetValue() != "a coded reason" {
					t.Errorf("reason[1] = %#v, want the DV_CODED_TEXT SNOMED-CT::282291009|a coded reason", got[1])
				}
				re := reencodeRMAttr(t, wt, comp)
				holdsKeys(t, re, wantInContext)
				if keys := perStepKeys(re); len(keys) > 0 {
					t.Errorf("re-encode wrote %v under a per-step node, want the in-context spelling alone", keys)
				}
				if back := decodeRMAttr(t, wt, re); !bytes.Equal(canonical(t, back), canonical(t, comp)) {
					t.Errorf("the re-encode decodes to\n%s\nwant\n%s", canonical(t, back), canonical(t, comp))
				}
			})
		}
	}
	t.Run("a sparse index", func(t *testing.T) {
		keys := mergeRMAttr(inContextIsmKeys("524", "at0005", "transition"), map[string]any{conformanceIsm + "/_reason:1": "a reason"})
		err := refusedInEveryMode(t, wt, rmattrBody(keys))
		if !errors.Is(err, ErrUnknownPath) || !strings.Contains(err.Error(), conformanceIsm+"/_reason:1") {
			t.Errorf("err = %v, want ErrUnknownPath naming %s", err, conformanceIsm+"/_reason:1")
		}
	})
}

// TestREQ053ActionReferenceSpellingDecodesAndValidates — REQ-053, REQ-170. The
// reproduction of issue 421: a body that spells an ACTION's time and current
// state as the specification's examples do, `<action>/time` and
// `<action>/ism_transition/current_state|code` + `|value`, decodes with the
// template and validates, against an ACTION whose template constrains no ISM
// transition and against one whose template models archetyped per-step nodes,
// in both formats. Encode writes the in-context spelling, the merged node's
// inferred careflow step among it, and that output decodes to the same
// composition.
func TestREQ053ActionReferenceSpellingDecodesAndValidates(t *testing.T) {
	const actionTime = "2019-03-22T22:26:01.127+01:00"
	minimalWT, minimalCompiled := minimalActionWT(t)
	wt, compiled := conformanceWT(t)
	spelling := func(action, ism string) map[string]any {
		return map[string]any{
			action + "/time":             actionTime,
			ism + "/current_state|code":  "532",
			ism + "/current_state|value": "completed",
		}
	}
	for _, tc := range []struct {
		name     string
		wt       *webtemplate.WebTemplate
		compiled *templatecompile.Compiled
		body     map[string]any
		want     map[string]any
	}{
		{
			name: "minimal_action_2", wt: minimalWT, compiled: minimalCompiled,
			body: minimalActionBody(spelling(minimalAction, minimalIsm)),
			want: map[string]any{
				minimalAction + "/time":                   actionTime,
				minimalIsm + "/current_state|code":        "532",
				minimalIsm + "/current_state|terminology": "openehr",
			},
		},
		{
			name: "conformance", wt: wt, compiled: compiled,
			body: rmattrBody(spelling(rmattrAction, conformanceIsm)),
			want: map[string]any{
				rmattrAction + "/time":                        actionTime,
				conformanceIsm + "/current_state|code":        "532",
				conformanceIsm + "/current_state|terminology": "openehr",
				conformanceIsm + "/careflow_step|code":        "at0006",
				conformanceIsm + "/careflow_step|value":       "transition",
				conformanceIsm + "/careflow_step|terminology": "local",
			},
		},
	} {
		for format, decode := range bothFormats(t, tc.body) {
			t.Run(tc.name+"/"+format, func(t *testing.T) {
				comp, err := decode(tc.wt, WithTemplate(tc.compiled), WithStrict())
				if err != nil {
					t.Fatalf("Unmarshal %s (WithTemplate): %v", format, err)
				}
				for _, is := range validation.ValidateComposition(comp, tc.compiled).Issues {
					t.Errorf("decoded composition: %s at %s: %s", is.Code, is.Path, is.Detail)
				}
				a := actionIn(t, comp)
				if a.Time.Value != actionTime {
					t.Errorf("ACTION time = %q, want %q", a.Time.Value, actionTime)
				}
				if got := a.IsmTransition.CurrentState; got.DefiningCode.CodeString != "532" || got.DefiningCode.TerminologyID.Value != "openehr" || got.Value != "completed" {
					t.Errorf("current_state = %+v, want openehr::532|completed", got)
				}
				got := reencodeRMAttr(t, tc.wt, comp)
				holdsKeys(t, got, tc.want)
				if keys := perStepKeys(got); len(keys) > 0 {
					t.Errorf("re-encode wrote %v under a per-step node, want the in-context spelling alone", keys)
				}
				back, err := UnmarshalFlat(mustJSON(t, got), tc.wt, WithTemplate(tc.compiled), WithStrict())
				if err != nil {
					t.Fatalf("UnmarshalFlat of the re-encode: %v", err)
				}
				if !bytes.Equal(canonical(t, back), canonical(t, comp)) {
					t.Errorf("the re-encode decodes to\n%s\nwant\n%s", canonical(t, back), canonical(t, comp))
				}
			})
		}
	}
}

// TestREQ140CareflowStepOfStepsConstrainingNoneRoundTrips — REQ-140, REQ-053,
// REQ-106. Where the template's per-step nodes constrain no careflow step, a
// transition spelled under one still decodes with the careflow step the node
// names: the node id as a `local` code, the node's Web Template name as its
// value. The merged in-context node lists each such step by its node id, so
// encode, which writes the transition there alone, writes that careflow step,
// and the output decodes to the same composition: encode silently loses no
// in-scope attribute. No vendored OPT has such steps, so the case strips
// careflow_step from the corpus template's two.
func TestREQ140CareflowStepOfStepsConstrainingNoneRoundTrips(t *testing.T) {
	raw, err := os.ReadFile(fixtures.FlatConformanceOpt())
	if err != nil {
		t.Fatal(err)
	}
	wt := buildEditedWT(t, stripCareflowSteps(t, raw, 2))
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
			if got := firstAction(t, comp).IsmTransition.CareflowStep; got == nil || got.DefiningCode.CodeString != tc.step || got.Value != name {
				t.Fatalf("careflow_step = %+v, want local::%s|%s, the node's id and name", got, tc.step, name)
			}
			got := reencodeRMAttr(t, wt, comp)
			holdsKeys(t, got, inContextIsmKeys(tc.state, tc.step, name))
			if back := decodeRMAttr(t, wt, got); !bytes.Equal(canonical(t, back), canonical(t, comp)) {
				t.Errorf("the re-encode decodes to\n%s\nwant\n%s", canonical(t, back), canonical(t, comp))
			}
		})
	}
}

// stripCareflowSteps returns opt with every careflow_step attribute removed,
// so that no ISM transition constrains one. It fails the test unless it
// removes want of them, laid out one element per line as the vendored OPTs
// are.
func stripCareflowSteps(t *testing.T, opt []byte, want int) []byte {
	t.Helper()
	lines := strings.SplitAfter(string(opt), "\n")
	indentOf := func(l string) int { return len(l) - len(strings.TrimLeft(l, " \t")) }
	found := 0
	for i := 0; i+1 < len(lines); i++ {
		if !strings.HasPrefix(strings.TrimSpace(lines[i]), "<attributes") ||
			strings.TrimSpace(lines[i+1]) != "<rm_attribute_name>careflow_step</rm_attribute_name>" {
			continue
		}
		indent := indentOf(lines[i])
		end := i + 1
		for end < len(lines) && (indentOf(lines[end]) != indent || strings.TrimSpace(lines[end]) != "</attributes>") {
			end++
		}
		if end == len(lines) {
			t.Fatalf("a careflow_step attribute at line %d is not closed", i+1)
		}
		lines = slices.Delete(lines, i, end+1)
		found++
		i--
	}
	if found != want {
		t.Fatalf("the OPT holds %d careflow_step attributes, want %d", found, want)
	}
	return []byte(strings.Join(lines, ""))
}

// buildEditedWT reads, compiles and builds the operational template raw, a
// vendored one a test has edited.
func buildEditedWT(t *testing.T, raw []byte) *webtemplate.WebTemplate {
	t.Helper()
	opt, err := fixtures.ReadOperationalTemplate(raw)
	if err != nil {
		t.Fatalf("ReadOperationalTemplate: %v", err)
	}
	c, err := templatecompile.Compile(opt)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	wt, err := webtemplate.Build(c)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	return wt
}

// siblingActionsWT is a minimal Web Template with two reused ACTION siblings,
// `a` and `b`, of one archetype, which pin the names A and B (REQ-111). ism
// builds each one's ISM transition node from the ACTION's aqlPath and pinned
// name.
func siblingActionsWT(ism func(base, pin string) *webtemplate.Node) *webtemplate.WebTemplate {
	action := func(id, pin string) *webtemplate.Node {
		base := "/content[openEHR-EHR-ACTION.test.v1,'" + pin + "']"
		return &webtemplate.Node{
			ID: id, Name: pin, RMType: "ACTION", NodeID: "openEHR-EHR-ACTION.test.v1", Max: 1, AQLPath: base,
			Children: []*webtemplate.Node{
				{ID: "time", Name: "Time", RMType: "DV_DATE_TIME", Max: 1, AQLPath: base + "/time", Inputs: []webtemplate.Input{{Type: "DATETIME"}}},
				ism(base, pin),
			},
		}
	}
	return &webtemplate.WebTemplate{Tree: &webtemplate.Node{
		ID: "root", RMType: "COMPOSITION", NodeID: "openEHR-EHR-COMPOSITION.t.v1",
		Children: []*webtemplate.Node{action("a", "A"), action("b", "B")},
	}}
}

// ismLeaf is the careflow_step or current_state leaf of the ISM transition
// node at base, listing items, coded in terminology ("" for local codes).
func ismLeaf(base, id, name, terminology string, items ...webtemplate.InputListItem) *webtemplate.Node {
	return &webtemplate.Node{
		ID: id, Name: name, RMType: "DV_CODED_TEXT", Min: 1, Max: 1, AQLPath: base + "/" + id,
		Inputs: []webtemplate.Input{{Suffix: "code", Type: "CODED_TEXT", Terminology: terminology, List: items}},
	}
}

// siblingCareflowStep decodes the FLAT body that adds keys to a minimal
// context against wt, and returns the careflow step of the ACTION named
// action.
func siblingCareflowStep(t *testing.T, wt *webtemplate.WebTemplate, action string, keys map[string]any) (*rm.DVCodedText, error) {
	t.Helper()
	body := map[string]any{"ctx/language": "en", "ctx/territory": "US", "ctx/time": "2021-12-21T14:19:31+01:00", "ctx/composer_name": "x"}
	maps.Copy(body, keys)
	comp, err := UnmarshalFlat(mustJSON(t, body), wt)
	if err != nil {
		return nil, err
	}
	for _, c := range comp.Content {
		if a, ok := c.(*rm.Action); ok && a.Name.GetValue() == action {
			return a.IsmTransition.CareflowStep, nil
		}
	}
	t.Fatalf("the composition holds no ACTION named %s", action)
	return nil, nil
}

// TestREQ111ReusedSiblingsSettleTheirOwnIsmTransition — REQ-111, REQ-053.
// Reused ACTION siblings that pinned names tell apart each have their own ISM
// transition nodes, at paths that differ only by those names. Decode settles a
// rebuilt transition against the node it was placed under, never against a
// sibling's: at the merged in-context node, a careflow step is inferred from
// and completed with that node's own list; at an archetyped per-step node, a
// transition gets that node's careflow step, and a careflow step coded
// otherwise is refused naming that node.
func TestREQ111ReusedSiblingsSettleTheirOwnIsmTransition(t *testing.T) {
	stateKeys := func(ism, state string) map[string]any {
		return map[string]any{ism + "/current_state|code": state, ism + "/current_state|value": "s", ism + "/current_state|terminology": "openehr"}
	}
	t.Run("merged", func(t *testing.T) {
		steps := map[string]struct{ code, state string }{"A": {"at0005", "524"}, "B": {"at0006", "532"}}
		wt := siblingActionsWT(func(base, pin string) *webtemplate.Node {
			ism := base + "/ism_transition"
			s := steps[pin]
			return &webtemplate.Node{
				ID: "ism_transition", Name: "ism_transition", RMType: "ISM_TRANSITION", Min: 1, Max: 1, AQLPath: ism,
				Children: []*webtemplate.Node{
					ismLeaf(ism, "careflow_step", "Careflow_step", "", webtemplate.InputListItem{Value: s.code, Label: "Step " + pin, CurrentStates: []string{s.state}}),
					ismLeaf(ism, "current_state", "Current_state", "openehr", webtemplate.InputListItem{Value: s.state}),
				},
			}
		})
		for _, pin := range []string{"A", "B"} {
			ism := "root/" + strings.ToLower(pin) + "/ism_transition"
			s := steps[pin]
			for name, keys := range map[string]map[string]any{
				"its state alone":            stateKeys(ism, s.state),
				"its step's code, no |value": mergeRMAttr(stateKeys(ism, s.state), map[string]any{ism + "/careflow_step|code": s.code}),
			} {
				t.Run(pin+"/"+name, func(t *testing.T) {
					got, err := siblingCareflowStep(t, wt, pin, keys)
					if err != nil {
						t.Fatalf("UnmarshalFlat: %v", err)
					}
					if got == nil || got.DefiningCode.CodeString != s.code || got.DefiningCode.TerminologyID.Value != "local" || got.Value != "Step "+pin {
						t.Errorf("ACTION %s careflow_step = %+v, want local::%s|Step %s", pin, got, s.code, pin)
					}
				})
			}
		}
	})
	t.Run("per step", func(t *testing.T) {
		wt := siblingActionsWT(func(base, pin string) *webtemplate.Node {
			ism := base + "/ism_transition[at0005]"
			return &webtemplate.Node{
				ID: "transition", Name: "Step " + pin, RMType: "ISM_TRANSITION", NodeID: "at0005", Min: 1, Max: 1, AQLPath: ism,
				Children: []*webtemplate.Node{
					ismLeaf(ism, "current_state", "Current_state", "openehr", webtemplate.InputListItem{Value: "524"}),
					ismLeaf(ism, "careflow_step", "Careflow_step", "", webtemplate.InputListItem{Value: "at0005", Label: "Step " + pin}),
				},
			}
		})
		for _, pin := range []string{"A", "B"} {
			ism := "root/" + strings.ToLower(pin) + "/transition"
			t.Run(pin+"/its state alone", func(t *testing.T) {
				got, err := siblingCareflowStep(t, wt, pin, stateKeys(ism, "524"))
				if err != nil {
					t.Fatalf("UnmarshalFlat: %v", err)
				}
				if got == nil || got.DefiningCode.CodeString != "at0005" || got.Value != "Step "+pin {
					t.Errorf("ACTION %s careflow_step = %+v, want local::at0005|Step %s", pin, got, pin)
				}
			})
			t.Run(pin+"/another step's code", func(t *testing.T) {
				keys := mergeRMAttr(stateKeys(ism, "524"), map[string]any{ism + "/careflow_step|code": "at0006", ism + "/careflow_step|value": "x"})
				_, err := siblingCareflowStep(t, wt, pin, keys)
				if !errors.Is(err, ErrUnsupportedDatatype) {
					t.Fatalf("UnmarshalFlat = %v, want ErrUnsupportedDatatype", err)
				}
				if !strings.Contains(err.Error(), ism+"/careflow_step") {
					t.Errorf("err = %q, want it to name %s/careflow_step", err, ism)
				}
			})
		}
	})
}

// TestREQ140IsmTransitionNilReasonTakesNoIndex — REQ-140. A nil entry of an
// ISM_TRANSITION's reason list, an untyped or a typed nil, carries nothing, so
// encode writes no key for it and gives it no `_reason:N` index: a real reason
// after it is `_reason:0`, never a `_reason:1` with no `_reason:0` before it,
// which decode refuses as a sparse index.
func TestREQ140IsmTransitionNilReasonTakesNoIndex(t *testing.T) {
	wt, _ := conformanceWT(t)
	for name, empty := range map[string]rm.DVTextLike{
		"an untyped nil": nil,
		"a typed nil":    (*rm.DVText)(nil),
	} {
		t.Run(name, func(t *testing.T) {
			comp := decodeRMAttr(t, wt, rmattrBody(inContextIsmKeys("524", "at0005", "transition")))
			firstAction(t, comp).IsmTransition.Reason = []rm.DVTextLike{empty, &rm.DVText{Value: "a reason"}}
			got := reencodeRMAttr(t, wt, comp)
			if v := got[conformanceIsm+"/_reason:0"]; v != "a reason" {
				t.Errorf("re-encode of %s/_reason:0 = %#v, want %q", conformanceIsm, v, "a reason")
			}
			for k := range got {
				if strings.HasPrefix(k, conformanceIsm+"/_reason:") && !strings.HasPrefix(k, conformanceIsm+"/_reason:0") {
					t.Errorf("re-encode wrote %s = %#v, want the one reason at :0", k, got[k])
				}
			}
		})
	}
}

// TestREQ053MergedIsmTransitionInfersNoUnlabelledCareflowStep — REQ-053. A
// careflow step decode infers at the merged node takes the label of the one
// step whose current states hold the transition's state as its value. Where
// that step's item has no label, decode infers none, rather than a careflow
// step with an empty value.
func TestREQ053MergedIsmTransitionInfersNoUnlabelledCareflowStep(t *testing.T) {
	wt, _ := conformanceWT(t)
	unlabelled := editWTNode(t, wt, conformanceIsm+"/careflow_step", func(n *webtemplate.Node) {
		n.Inputs = slices.Clone(n.Inputs)
		n.Inputs[0].List = slices.Clone(n.Inputs[0].List)
		for i := range n.Inputs[0].List {
			n.Inputs[0].List[i].Label = ""
		}
	})
	keys := inContextIsmKeys("524", "", "")
	for k := range keys {
		if strings.Contains(k, "/careflow_step|") {
			delete(keys, k)
		}
	}
	comp := decodeRMAttr(t, unlabelled, rmattrBody(keys))
	if got := firstAction(t, comp).IsmTransition.CareflowStep; got != nil {
		t.Errorf("careflow_step = %+v, want none: the one step holding 524 has no label", got)
	}
}
