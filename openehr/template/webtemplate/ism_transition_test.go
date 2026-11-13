package webtemplate_test

// REQ-106 / REQ-053 — an ACTION's in-context `time` leaf and `ism_transition`
// node.
//
// ACTION.time and ACTION.ism_transition are RM-mandatory, so every ACTION
// carries both. The builder gives every ACTION the reference's in-context
// `time` leaf and one in-context `ism_transition` node with no node id. Where
// the template models the transition as archetyped `ism_transition[atNNNN]`
// nodes, one per careflow step, the in-context node merges them as the
// reference does, and the archetyped nodes stay beside it; where it models
// none, the in-context node is the unconstrained transition.

import (
	"bytes"
	"encoding/json"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/template/webtemplate"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// The vendored OPTs with an ACTION: minimal_action_2's constrains no ISM
// transition, the other two model archetyped ones.
const (
	minimalActionOPT = "../../../testkit/corpus/templates/minimal_action_2.opt"
	conformanceOPT   = "../../../testkit/corpus/flat-conformance/templates/conformance_ehrbase.de.v0.opt"
	// testAllTypesOPT's one ACTION models three archetyped ISM transitions,
	// at0003 to at0005, and the reference's Web Template for it is vendored.
	testAllTypesOPT = "../../../testkit/corpus/crossformat/test_all_types/template.opt"
)

// actionNodes returns every ACTION node of wt, in tree order.
func actionNodes(wt *webtemplate.WebTemplate) []*webtemplate.Node {
	var out []*webtemplate.Node
	walkOurTree(wt.Tree, func(n *webtemplate.Node) {
		if n.RMType == "ACTION" {
			out = append(out, n)
		}
	})
	return out
}

// onlyAction returns the one ACTION node of wt, failing the test otherwise.
func onlyAction(t *testing.T, wt *webtemplate.WebTemplate) *webtemplate.Node {
	t.Helper()
	actions := actionNodes(wt)
	if len(actions) != 1 {
		t.Fatalf("%s has %d ACTION nodes, want 1 — fixture changed?", wt.TemplateID, len(actions))
	}
	return actions[0]
}

// inContextIsm returns the ISM_TRANSITION child of action that has no node id,
// failing the test unless there is exactly one.
func inContextIsm(t *testing.T, action *webtemplate.Node) *webtemplate.Node {
	t.Helper()
	var ism *webtemplate.Node
	for _, ch := range action.Children {
		if ch.RMType != "ISM_TRANSITION" || ch.NodeID != "" {
			continue
		}
		if ism != nil {
			t.Fatalf("ACTION %s has two in-context ISM_TRANSITION children, %s and %s", action.AQLPath, ism.AQLPath, ch.AQLPath)
		}
		ism = ch
	}
	if ism == nil {
		t.Fatalf("ACTION %s has no in-context ISM_TRANSITION child; children = %v", action.AQLPath, childIDs(action))
	}
	return ism
}

// archetypedIsms counts the ISM_TRANSITION children of action that carry a
// node id: the per-careflow-step nodes.
func archetypedIsms(action *webtemplate.Node) int {
	n := 0
	for _, ch := range action.Children {
		if ch.RMType == "ISM_TRANSITION" && ch.NodeID != "" {
			n++
		}
	}
	return n
}

// transitionLeaf is the in-context `transition` leaf under the ISM transition
// node at base, as the reference's test_all_types golden spells it.
func transitionLeaf(base string) *webtemplate.Node {
	return &webtemplate.Node{
		ID: "transition", Name: "transition", RMType: "DV_CODED_TEXT", Max: 1, AQLPath: base + "/transition",
		Inputs: []webtemplate.Input{{Suffix: "code", Type: "TEXT"}, {Suffix: "value", Type: "TEXT"}},
	}
}

// unconstrainedIsm is the in-context ISM transition node an ACTION at
// actionPath gets when the walk emits no archetyped one.
func unconstrainedIsm(actionPath string) *webtemplate.Node {
	base := actionPath + "/ism_transition"
	return &webtemplate.Node{
		ID: "ism_transition", Name: "Ism_transition", RMType: "ISM_TRANSITION", Max: 1, AQLPath: base,
		Children: []*webtemplate.Node{
			{
				ID: "current_state", Name: "Current_state", RMType: "DV_CODED_TEXT", Max: 1, AQLPath: base + "/current_state",
				Inputs: []webtemplate.Input{{Suffix: "code", Type: "TEXT"}, {Suffix: "value", Type: "TEXT"}},
			},
			transitionLeaf(base),
		},
	}
}

// step is one archetyped careflow step as the merged node lists it: its
// careflow-step code and label, and the current states it admits.
type step struct {
	code, label string
	states      []string
}

// mergedIsm is the merged in-context ISM transition node an ACTION at
// actionPath gets from archetyped steps, with min the occurrences of the node
// and of its careflow_step and current_state leaves, and currentStates the
// list of the current_state leaf.
func mergedIsm(actionPath string, minNode, minStep, minState int, steps []step, currentStates []string) *webtemplate.Node {
	base := actionPath + "/ism_transition"
	var careflow, states []webtemplate.InputListItem
	for _, s := range steps {
		careflow = append(careflow, webtemplate.InputListItem{Value: s.code, Label: s.label, CurrentStates: s.states})
	}
	for _, c := range currentStates {
		states = append(states, webtemplate.InputListItem{Value: c})
	}
	return &webtemplate.Node{
		ID: "ism_transition", Name: "ism_transition", RMType: "ISM_TRANSITION", Min: minNode, Max: 1, AQLPath: base,
		Children: []*webtemplate.Node{
			{
				ID: "careflow_step", Name: "Careflow_step", RMType: "DV_CODED_TEXT", Min: minStep, Max: 1, AQLPath: base + "/careflow_step",
				Inputs: []webtemplate.Input{{Suffix: "code", Type: "CODED_TEXT", List: careflow}},
			},
			{
				ID: "current_state", Name: "Current_state", RMType: "DV_CODED_TEXT", Min: minState, Max: 1, AQLPath: base + "/current_state",
				Inputs: []webtemplate.Input{{Suffix: "code", Type: "CODED_TEXT", List: states, Terminology: "openehr"}},
			},
			transitionLeaf(base),
		},
	}
}

// TestREQ106InContextIsmTransitionForUnconstrainedAction — REQ-106: the
// minimal_action_2 ACTION archetype constrains no ISM transition, so its
// ACTION node gets the reference's in-context `ism_transition` node: no node
// id, at most one, holding a `current_state` and a `transition` coded-text
// leaf, and no `careflow_step`, which the reference leaves out there.
func TestREQ106InContextIsmTransitionForUnconstrainedAction(t *testing.T) {
	wt, err := webtemplate.Build(compileFixture(t, minimalActionOPT))
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	action := onlyAction(t, wt)
	if n := archetypedIsms(action); n != 0 {
		t.Fatalf("ACTION %s has %d archetyped ISM transitions, want none — fixture changed?", action.AQLPath, n)
	}
	if got, want := inContextIsm(t, action), unconstrainedIsm(action.AQLPath); !reflect.DeepEqual(got, want) {
		t.Errorf("in-context ISM transition node =\n  %s\nwant\n  %s", jsonOf(t, got), jsonOf(t, want))
	}
}

// TestREQ106MergedIsmTransitionForArchetypedAction — REQ-106: an ACTION whose
// template models archetyped `ism_transition[atNNNN]` nodes keeps them and
// also gets the reference's merged in-context `ism_transition` node beside
// them: no node id, min and max from the first archetyped node, and the
// `careflow_step`, `current_state` and `transition` leaves in that order. The
// careflow step lists each archetyped node's step with the current states that
// node admits; the current state lists them all in node order, a state two
// nodes admit listed twice, as the reference appends each node's list as is.
// test_all_types is the reference's own golden case, pinned whole;
// conformance_ehrbase is the PROBE-086 template, pinned on its shape.
func TestREQ106MergedIsmTransitionForArchetypedAction(t *testing.T) {
	raw, err := os.ReadFile(testAllTypesOPT)
	if err != nil {
		t.Fatal(err)
	}
	const states, moreStates = "<code_list>532</code_list>", "<code_list>532</code_list><code_list>245</code_list>"
	if n := bytes.Count(raw, []byte(states)); n != 1 {
		t.Fatalf("%s holds %q %d times, want once", testAllTypesOPT, states, n)
	}
	planned := step{code: "at0003", label: "Planned", states: []string{"526"}}
	active := step{code: "at0004", label: "Active", states: []string{"245"}}
	completed := step{code: "at0005", label: "Completed", states: []string{"532"}}
	tests := []struct {
		name string
		opt  []byte
		want func(actionPath string) *webtemplate.Node
		// archetyped is the number of per-step nodes kept beside the merged one.
		archetyped int
	}{
		{
			name: "test_all_types",
			opt:  raw,
			want: func(p string) *webtemplate.Node {
				return mergedIsm(p, 1, 1, 1, []step{planned, active, completed}, []string{"526", "245", "532"})
			},
			archetyped: 3,
		},
		{
			name: "a step admitting a state another admits, listed twice",
			opt:  bytes.Replace(raw, []byte(states), []byte(moreStates), 1),
			want: func(p string) *webtemplate.Node {
				both := step{code: "at0005", label: "Completed", states: []string{"532", "245"}}
				return mergedIsm(p, 1, 1, 1, []step{planned, active, both}, []string{"526", "245", "532", "245"})
			},
			archetyped: 3,
		},
		{
			// The merged node's min is the first step's, not the last's.
			name: "the first step optional",
			opt:  optionalIsmTransition(t, raw, "at0003"),
			want: func(p string) *webtemplate.Node {
				return mergedIsm(p, 0, 1, 1, []step{planned, active, completed}, []string{"526", "245", "532"})
			},
			archetyped: 3,
		},
		{
			name: "the last step optional",
			opt:  optionalIsmTransition(t, raw, "at0005"),
			want: func(p string) *webtemplate.Node {
				return mergedIsm(p, 1, 1, 1, []step{planned, active, completed}, []string{"526", "245", "532"})
			},
			archetyped: 3,
		},
		{
			name: "the first step constraining nothing",
			opt:  stripIsmTransition(t, raw, "at0003"),
			want: func(p string) *webtemplate.Node {
				return mergedIsm(p, 1, 1, 1, []step{active, completed}, []string{"245", "532"})
			},
			archetyped: 2,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			action := onlyAction(t, buildOPT(t, tc.opt))
			if n := archetypedIsms(action); n != tc.archetyped {
				t.Errorf("ACTION %s keeps %d archetyped ISM transitions, want %d", action.AQLPath, n, tc.archetyped)
			}
			if got, want := inContextIsm(t, action), tc.want(action.AQLPath); !reflect.DeepEqual(got, want) {
				t.Errorf("merged ISM transition node =\n  %s\nwant\n  %s", jsonOf(t, got), jsonOf(t, want))
			}
		})
	}
	t.Run("conformance_ehrbase.de.v0", func(t *testing.T) {
		action := onlyAction(t, buildOPT(t, readFile(t, conformanceOPT)))
		steps := archetypedIsms(action)
		if steps == 0 {
			t.Fatalf("ACTION %s has no archetyped ISM transition — fixture changed?", action.AQLPath)
		}
		ism := inContextIsm(t, action)
		if ism.AQLPath != action.AQLPath+"/ism_transition" {
			t.Errorf("merged ISM transition aqlPath = %s, want %s/ism_transition", ism.AQLPath, action.AQLPath)
		}
		if got, want := childIDs(ism), []string{"careflow_step", "current_state", "transition"}; !slices.Equal(got, want) {
			t.Fatalf("merged ISM transition children = %v, want %v", got, want)
		}
		if in := ism.Children[0].Inputs; len(in) != 1 || len(in[0].List) != steps {
			t.Errorf("merged careflow_step inputs = %+v, want one code input listing one step per archetyped node (%d)", in, steps)
		}
	})
}

// TestREQ106MergedCareflowStepListsStepsConstrainingNone — REQ-106, REQ-140:
// the merged node holds `careflow_step` whenever it merges steps. A step whose
// archetype constrains no careflow step still names one, its node id, which a
// transition decoded under it is given; the merged list carries it as one
// item, the node id with the step's Web Template name as its label (and, like
// every other list item, no localized labels), and the current states that
// step admits. Otherwise encode, which writes a transition at the merged node
// alone, would drop that careflow step. The leaf's min is the first step's
// that constrains one, else 0. No vendored OPT has such a step, so the cases
// strip careflow_step from test_all_types steps.
func TestREQ106MergedCareflowStepListsStepsConstrainingNone(t *testing.T) {
	raw := readFile(t, testAllTypesOPT)
	planned := step{code: "at0003", label: "Planned", states: []string{"526"}}
	active := step{code: "at0004", label: "Active", states: []string{"245"}}
	completed := step{code: "at0005", label: "Completed", states: []string{"532"}}
	allStates := []string{"526", "245", "532"}
	tests := []struct {
		name  string
		strip []string
		want  func(actionPath string) *webtemplate.Node
	}{
		{
			name:  "the middle step",
			strip: []string{"at0004"},
			want: func(p string) *webtemplate.Node {
				return mergedIsm(p, 1, 1, 1, []step{planned, active, completed}, allStates)
			},
		},
		{
			name:  "the first step",
			strip: []string{"at0003"},
			want: func(p string) *webtemplate.Node {
				return mergedIsm(p, 1, 1, 1, []step{planned, active, completed}, allStates)
			},
		},
		{
			name:  "every step",
			strip: []string{"at0003", "at0004", "at0005"},
			want: func(p string) *webtemplate.Node {
				return mergedIsm(p, 1, 0, 1, []step{planned, active, completed}, allStates)
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			opt := raw
			for _, id := range tc.strip {
				opt = stripIsmAttribute(t, opt, id, "careflow_step")
			}
			action := onlyAction(t, buildOPT(t, opt))
			if n := archetypedIsms(action); n != 3 {
				t.Errorf("ACTION %s keeps %d archetyped ISM transitions, want 3", action.AQLPath, n)
			}
			if got, want := inContextIsm(t, action), tc.want(action.AQLPath); !reflect.DeepEqual(got, want) {
				t.Errorf("merged ISM transition node =\n  %s\nwant\n  %s", jsonOf(t, got), jsonOf(t, want))
			}
		})
	}
}

// TestREQ106MergedIsmTransitionForStepsConstrainingOnlyTransition — REQ-106:
// an archetyped ISM_TRANSITION that constrains its transition alone is no
// empty container, so the walk keeps it and the ACTION gets the merged node,
// not the unconstrained one. Its `careflow_step` lists each step by its node
// id, the careflow step a transition decoded under that step is given, which
// encode could otherwise not write; with no step constraining one, its
// `current_state` is the unconstrained leaf; its `transition` is the first
// step's own, as the reference takes it. No vendored OPT has such a step, so
// the case strips current_state from the test_all_types steps and turns their
// careflow_step into a transition, at0003's coded with its own node id.
func TestREQ106MergedIsmTransitionForStepsConstrainingOnlyTransition(t *testing.T) {
	const careflowAttr, transitionAttr = "<rm_attribute_name>careflow_step</rm_attribute_name>", "<rm_attribute_name>transition</rm_attribute_name>"
	raw := readFile(t, testAllTypesOPT)
	for _, id := range []string{"at0003", "at0004", "at0005"} {
		raw = stripIsmAttribute(t, raw, id, "current_state")
	}
	if n := bytes.Count(raw, []byte(careflowAttr)); n != 3 {
		t.Fatalf("%s holds %q %d times, want once per step", testAllTypesOPT, careflowAttr, n)
	}
	raw = bytes.ReplaceAll(raw, []byte(careflowAttr), []byte(transitionAttr))
	action := onlyAction(t, buildOPT(t, raw))
	if n := archetypedIsms(action); n != 3 {
		t.Errorf("ACTION %s keeps %d archetyped ISM transitions, want 3", action.AQLPath, n)
	}
	steps := []step{{code: "at0003", label: "Planned"}, {code: "at0004", label: "Active"}, {code: "at0005", label: "Completed"}}
	want := mergedIsm(action.AQLPath, 1, 0, 0, steps, nil)
	want.Children[1] = unconstrainedIsm(action.AQLPath).Children[0]
	want.Children[2] = &webtemplate.Node{
		ID: "transition", Name: "transition", RMType: "DV_CODED_TEXT", Min: 1, Max: 1, AQLPath: want.AQLPath + "/transition",
		Inputs: []webtemplate.Input{{Suffix: "code", Type: "CODED_TEXT", List: []webtemplate.InputListItem{{Value: "at0003", Label: "Planned"}}}},
	}
	if got := inContextIsm(t, action); !reflect.DeepEqual(got, want) {
		t.Errorf("merged ISM transition node =\n  %s\nwant\n  %s", jsonOf(t, got), jsonOf(t, want))
	}
}

// TestREQ106MergedTransitionIsTheFirstStepsOwn — REQ-106: the merged node's
// `transition` leaf is the first step's own transition, as the reference takes
// it, where that step constrains one: its min, max and inputs, at the merged
// node's path, named `transition`. Where the first step constrains none, the
// leaf is the open one, whatever a later step constrains. No vendored OPT
// constrains a transition, so the cases splice one, coded in the openehr
// terminology, into a test_all_types step.
func TestREQ106MergedTransitionIsTheFirstStepsOwn(t *testing.T) {
	raw := readFile(t, testAllTypesOPT)
	planned := step{code: "at0003", label: "Planned", states: []string{"526"}}
	active := step{code: "at0004", label: "Active", states: []string{"245"}}
	completed := step{code: "at0005", label: "Completed", states: []string{"532"}}
	tests := []struct {
		name   string
		nodeID string
		// own reports whether the merged leaf is the step's own transition.
		own bool
	}{
		{name: "the first step", nodeID: "at0003", own: true},
		{name: "a later step only", nodeID: "at0004", own: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			action := onlyAction(t, buildOPT(t, constrainIsmTransition(t, raw, tc.nodeID, "535", "536")))
			want := mergedIsm(action.AQLPath, 1, 1, 1, []step{planned, active, completed}, []string{"526", "245", "532"})
			if tc.own {
				want.Children[2] = &webtemplate.Node{
					ID: "transition", Name: "transition", RMType: "DV_CODED_TEXT", Min: 1, Max: 1, AQLPath: want.AQLPath + "/transition",
					Inputs: []webtemplate.Input{{
						Suffix: "code", Type: "CODED_TEXT", Terminology: "openehr",
						List: []webtemplate.InputListItem{{Value: "535"}, {Value: "536"}},
					}},
				}
			}
			if got := inContextIsm(t, action); !reflect.DeepEqual(got, want) {
				t.Errorf("merged ISM transition node =\n  %s\nwant\n  %s", jsonOf(t, got), jsonOf(t, want))
			}
			stepPath := action.AQLPath + "/ism_transition[" + tc.nodeID + "]"
			i := slices.IndexFunc(action.Children, func(n *webtemplate.Node) bool { return n.AQLPath == stepPath })
			if i < 0 {
				t.Fatalf("ACTION %s has no step at %s; children = %v", action.AQLPath, stepPath, childIDs(action))
			}
			if !slices.Contains(childIDs(action.Children[i]), "transition") {
				t.Errorf("step %s children = %v, want its own transition kept beside the merged node", stepPath, childIDs(action.Children[i]))
			}
		})
	}
}

// TestREQ106InContextIsmTransitionWhenEveryArchetypedOneIsPruned — REQ-106:
// an archetyped ISM_TRANSITION that constrains none of its attributes
// contributes no node, as any empty container does. An
// ACTION all of whose archetyped transitions are dropped so still gets the
// in-context `ism_transition` node, the unconstrained one, or the RM-mandatory
// transition would have no node to be written at. No vendored OPT has such a
// transition, so the case strips the three of test_all_types.
func TestREQ106InContextIsmTransitionWhenEveryArchetypedOneIsPruned(t *testing.T) {
	raw := readFile(t, testAllTypesOPT)
	for _, id := range []string{"at0003", "at0004", "at0005"} {
		raw = stripIsmTransition(t, raw, id)
	}
	action := onlyAction(t, buildOPT(t, raw))
	if n := archetypedIsms(action); n != 0 {
		t.Fatalf("ACTION %s keeps %d archetyped ISM transitions, want every one pruned", action.AQLPath, n)
	}
	if got, want := inContextIsm(t, action), unconstrainedIsm(action.AQLPath); !reflect.DeepEqual(got, want) {
		t.Errorf("in-context ISM transition node =\n  %s\nwant\n  %s", jsonOf(t, got), jsonOf(t, want))
	}
}

// TestREQ106ActionInContextTime — REQ-106: every ACTION gets the reference's
// in-context `time` leaf, a DV_DATE_TIME shaped as an EVENT's, with min 0 as
// the other in-context leaves have it, whether or not its template models an
// ISM transition. TestInContextLeavesResolveViaRmpath holds rmpath to
// resolving it.
func TestREQ106ActionInContextTime(t *testing.T) {
	for _, opt := range []string{minimalActionOPT, conformanceOPT, testAllTypesOPT} {
		t.Run(opt[strings.LastIndexByte(opt, '/')+1:], func(t *testing.T) {
			action := onlyAction(t, buildOPT(t, readFile(t, opt)))
			want := &webtemplate.Node{
				ID: "time", Name: "Time", RMType: "DV_DATE_TIME", Max: 1, AQLPath: action.AQLPath + "/time",
				Inputs: []webtemplate.Input{{Type: "DATETIME"}},
			}
			i := slices.IndexFunc(action.Children, func(n *webtemplate.Node) bool { return n.AQLPath == want.AQLPath })
			if i < 0 {
				t.Fatalf("ACTION %s has no node at %s; children = %v", action.AQLPath, want.AQLPath, childIDs(action))
			}
			if got := action.Children[i]; !reflect.DeepEqual(got, want) {
				t.Errorf("ACTION time node =\n  %s\nwant\n  %s", jsonOf(t, got), jsonOf(t, want))
			}
		})
	}
}

// readFile reads path, failing the test when it cannot.
func readFile(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// stripIsmTransition returns opt with every attribute of the archetyped
// ISM_TRANSITION nodeID removed, so that it constrains none of them. It fails the test unless opt holds
// that node once, laid out one element per line as the vendored OPTs are.
func stripIsmTransition(t *testing.T, opt []byte, nodeID string) []byte {
	t.Helper()
	lines := strings.SplitAfter(string(opt), "\n")
	indentOf := func(l string) int { return len(l) - len(strings.TrimLeft(l, " \t")) }
	found := 0
	for i := 0; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) != "<rm_type_name>ISM_TRANSITION</rm_type_name>" {
			continue
		}
		indent := indentOf(lines[i])
		j := i + 1
		for j < len(lines) && (indentOf(lines[j]) != indent || !strings.HasPrefix(strings.TrimSpace(lines[j]), "<node_id>")) {
			j++
		}
		if j == len(lines) || strings.TrimSpace(lines[j]) != "<node_id>"+nodeID+"</node_id>" {
			continue
		}
		k := j + 1
		for k < len(lines) && indentOf(lines[k]) >= indent {
			k++
		}
		if k == j+1 || !strings.HasPrefix(strings.TrimSpace(lines[j+1]), "<attributes") {
			t.Fatalf("ISM_TRANSITION %s has no attributes to strip", nodeID)
		}
		lines = slices.Delete(lines, j+1, k)
		found++
	}
	if found != 1 {
		t.Fatalf("the OPT holds ISM_TRANSITION %s %d times, want once", nodeID, found)
	}
	return []byte(strings.Join(lines, ""))
}

// optionalIsmTransition returns opt with the occurrences of the archetyped
// ISM_TRANSITION nodeID made 0..1. It fails the test unless opt holds that
// node once, required, laid out one element per line as the vendored OPTs are.
func optionalIsmTransition(t *testing.T, opt []byte, nodeID string) []byte {
	t.Helper()
	lines := strings.SplitAfter(string(opt), "\n")
	indentOf := func(l string) int { return len(l) - len(strings.TrimLeft(l, " \t")) }
	found := 0
	for i := range lines {
		if strings.TrimSpace(lines[i]) != "<rm_type_name>ISM_TRANSITION</rm_type_name>" {
			continue
		}
		indent := indentOf(lines[i])
		j := i + 1
		for j < len(lines) && (indentOf(lines[j]) != indent || !strings.HasPrefix(strings.TrimSpace(lines[j]), "<node_id>")) {
			j++
		}
		if j == len(lines) || strings.TrimSpace(lines[j]) != "<node_id>"+nodeID+"</node_id>" {
			continue
		}
		for k := i + 1; k < j; k++ {
			if strings.TrimSpace(lines[k]) == "<lower>1</lower>" {
				lines[k] = strings.Replace(lines[k], "<lower>1</lower>", "<lower>0</lower>", 1)
				found++
			}
		}
	}
	if found != 1 {
		t.Fatalf("the OPT holds a required ISM_TRANSITION %s %d times, want once", nodeID, found)
	}
	return []byte(strings.Join(lines, ""))
}

// stripIsmAttribute returns opt with the attr attribute of the archetyped
// ISM_TRANSITION nodeID removed, so that the node no longer constrains it. It
// fails the test unless opt holds that node once, with that attribute, laid
// out one element per line as the vendored OPTs are.
func stripIsmAttribute(t *testing.T, opt []byte, nodeID, attr string) []byte {
	t.Helper()
	lines := strings.SplitAfter(string(opt), "\n")
	indentOf := func(l string) int { return len(l) - len(strings.TrimLeft(l, " \t")) }
	found := 0
	for i := 0; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) != "<rm_type_name>ISM_TRANSITION</rm_type_name>" {
			continue
		}
		indent := indentOf(lines[i])
		j := i + 1
		for j < len(lines) && (indentOf(lines[j]) != indent || !strings.HasPrefix(strings.TrimSpace(lines[j]), "<node_id>")) {
			j++
		}
		if j == len(lines) || strings.TrimSpace(lines[j]) != "<node_id>"+nodeID+"</node_id>" {
			continue
		}
		for a := j + 1; a+1 < len(lines) && indentOf(lines[a]) >= indent; a++ {
			if indentOf(lines[a]) != indent || strings.TrimSpace(lines[a+1]) != "<rm_attribute_name>"+attr+"</rm_attribute_name>" {
				continue
			}
			end := a + 1
			for end < len(lines) && (indentOf(lines[end]) != indent || strings.TrimSpace(lines[end]) != "</attributes>") {
				end++
			}
			if end == len(lines) {
				t.Fatalf("ISM_TRANSITION %s's %s attribute is not closed", nodeID, attr)
			}
			lines = slices.Delete(lines, a, end+1)
			found++
			break
		}
	}
	if found != 1 {
		t.Fatalf("the OPT holds ISM_TRANSITION %s with a %s attribute %d times, want once", nodeID, attr, found)
	}
	return []byte(strings.Join(lines, ""))
}

// constrainIsmTransition returns opt with the archetyped ISM_TRANSITION nodeID
// constraining its transition, once, to a DV_CODED_TEXT of the openehr codes,
// placed before its careflow_step as the RM orders the two. It fails the test
// unless opt holds that node once, with a careflow_step, laid out one element
// per line as the vendored OPTs are.
func constrainIsmTransition(t *testing.T, opt []byte, nodeID string, codes ...string) []byte {
	t.Helper()
	lines := strings.SplitAfter(string(opt), "\n")
	indentOf := func(l string) int { return len(l) - len(strings.TrimLeft(l, " \t")) }
	once := []string{
		"<lower_included>true</lower_included>", "<upper_included>true</upper_included>",
		"<lower_unbounded>false</lower_unbounded>", "<upper_unbounded>false</upper_unbounded>",
		"<lower>1</lower>", "<upper>1</upper>",
	}
	// block lists the attribute's elements, each with its depth below the
	// ISM_TRANSITION's attributes.
	type element struct {
		depth int
		text  string
	}
	block := []element{{0, `<attributes xsi:type="C_SINGLE_ATTRIBUTE">`}, {1, "<rm_attribute_name>transition</rm_attribute_name>"}}
	interval := func(depth int, tag string) {
		block = append(block, element{depth, "<" + tag + ">"})
		for _, l := range once {
			block = append(block, element{depth + 1, l})
		}
		block = append(block, element{depth, "</" + tag + ">"})
	}
	interval(1, "existence")
	block = append(block, element{1, `<children xsi:type="C_COMPLEX_OBJECT">`}, element{2, "<rm_type_name>DV_CODED_TEXT</rm_type_name>"})
	interval(2, "occurrences")
	block = append(block, element{2, "<node_id/>"},
		element{2, `<attributes xsi:type="C_SINGLE_ATTRIBUTE">`}, element{3, "<rm_attribute_name>defining_code</rm_attribute_name>"})
	interval(3, "existence")
	block = append(block, element{3, `<children xsi:type="C_CODE_PHRASE">`}, element{4, "<rm_type_name>CODE_PHRASE</rm_type_name>"})
	interval(4, "occurrences")
	block = append(block, element{4, "<node_id/>"}, element{4, "<terminology_id>"}, element{5, "<value>openehr</value>"}, element{4, "</terminology_id>"})
	for _, c := range codes {
		block = append(block, element{4, "<code_list>" + c + "</code_list>"})
	}
	block = append(block, element{3, "</children>"}, element{2, "</attributes>"}, element{1, "</children>"}, element{0, "</attributes>"})

	found := 0
	for i := 0; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) != "<rm_type_name>ISM_TRANSITION</rm_type_name>" {
			continue
		}
		indent := indentOf(lines[i])
		j := i + 1
		for j < len(lines) && (indentOf(lines[j]) != indent || !strings.HasPrefix(strings.TrimSpace(lines[j]), "<node_id>")) {
			j++
		}
		if j == len(lines) || strings.TrimSpace(lines[j]) != "<node_id>"+nodeID+"</node_id>" {
			continue
		}
		for a := j + 1; a+1 < len(lines) && indentOf(lines[a]) >= indent; a++ {
			if indentOf(lines[a]) != indent || strings.TrimSpace(lines[a+1]) != "<rm_attribute_name>careflow_step</rm_attribute_name>" {
				continue
			}
			prefix := lines[a][:indent]
			spliced := make([]string, 0, len(block))
			for _, e := range block {
				spliced = append(spliced, prefix+strings.Repeat("    ", e.depth)+e.text+"\n")
			}
			lines = slices.Insert(lines, a, spliced...)
			found++
			break
		}
	}
	if found != 1 {
		t.Fatalf("the OPT holds ISM_TRANSITION %s with a careflow_step %d times, want once", nodeID, found)
	}
	return []byte(strings.Join(lines, ""))
}

// buildOPT reads, compiles and builds the operational template raw.
func buildOPT(t *testing.T, raw []byte) *webtemplate.WebTemplate {
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

// jsonOf renders n as JSON, for a failure message.
func jsonOf(t *testing.T, n *webtemplate.Node) string {
	t.Helper()
	b, err := json.Marshal(n)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	return string(b)
}

// childIDs lists the ids of n's children, for a failure message.
func childIDs(n *webtemplate.Node) []string {
	ids := make([]string, 0, len(n.Children))
	for _, ch := range n.Children {
		ids = append(ids, ch.ID)
	}
	return ids
}
