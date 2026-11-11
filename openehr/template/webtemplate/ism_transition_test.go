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
// node admits; the current state lists them all in node order, a repeat
// dropped. test_all_types is the reference's own golden case, pinned whole;
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
			name: "a step admitting a state another admits",
			opt:  bytes.Replace(raw, []byte(states), []byte(moreStates), 1),
			want: func(p string) *webtemplate.Node {
				both := step{code: "at0005", label: "Completed", states: []string{"532", "245"}}
				return mergedIsm(p, 1, 1, 1, []step{planned, active, both}, []string{"526", "245", "532"})
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

// TestREQ106InContextIsmTransitionWhenEveryArchetypedOneIsPruned — REQ-106:
// an archetyped ISM_TRANSITION that constrains neither its current_state nor
// its careflow_step contributes no node, as any empty container does. An
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
// ISM_TRANSITION nodeID removed, so that it constrains neither its
// current_state nor its careflow_step. It fails the test unless opt holds
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
