package webtemplate_test

// REQ-106 / REQ-053 — the in-context `ism_transition` node of an ACTION whose
// template constrains no ISM transition.
//
// ACTION.ism_transition is RM-mandatory, so every ACTION carries one. Where
// the template models it as archetyped `ism_transition[atNNNN]` nodes, those
// nodes carry it on the FLAT and STRUCTURED paths; where the template
// constrains none, the builder adds the reference's in-context node, so the
// formats can still carry the transition's current state.

import (
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

// TestREQ106InContextIsmTransitionForUnconstrainedAction — REQ-106: the
// minimal_action_2 ACTION archetype constrains no ISM transition, so its
// ACTION node gets the reference's in-context `ism_transition` node: no node
// id, at most one, holding a `current_state` coded-text leaf.
func TestREQ106InContextIsmTransitionForUnconstrainedAction(t *testing.T) {
	wt, err := webtemplate.Build(compileFixture(t, "../../../testkit/corpus/templates/minimal_action_2.opt"))
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	actions := actionNodes(wt)
	if len(actions) != 1 {
		t.Fatalf("minimal_action_2 has %d ACTION nodes, want 1 — fixture changed?", len(actions))
	}
	action := actions[0]
	base := action.AQLPath + "/ism_transition"
	var ism *webtemplate.Node
	for _, ch := range action.Children {
		if ch.RMType == "ISM_TRANSITION" {
			if ism != nil {
				t.Fatalf("ACTION %s has two ISM_TRANSITION children, %s and %s", action.AQLPath, ism.AQLPath, ch.AQLPath)
			}
			ism = ch
		}
	}
	if ism == nil {
		t.Fatalf("ACTION %s has no ISM_TRANSITION child; children = %v", action.AQLPath, childIDs(action))
	}
	want := &webtemplate.Node{
		ID: "ism_transition", Name: "Ism_transition", RMType: "ISM_TRANSITION", Max: 1, AQLPath: base,
		Children: []*webtemplate.Node{{
			ID: "current_state", Name: "Current_state", RMType: "DV_CODED_TEXT", Max: 1, AQLPath: base + "/current_state",
			Inputs: []webtemplate.Input{{Suffix: "code", Type: "TEXT"}, {Suffix: "value", Type: "TEXT"}},
		}},
	}
	if !reflect.DeepEqual(ism, want) {
		t.Errorf("in-context ISM transition node =\n  %s\nwant\n  %s", jsonOf(t, ism), jsonOf(t, want))
	}
}

// TestREQ106NoInContextIsmTransitionForArchetypedAction — REQ-106: an ACTION
// whose template models archetyped `ism_transition[atNNNN]` nodes gets no
// in-context `ism_transition` node beside them, so no FLAT key changes for it.
func TestREQ106NoInContextIsmTransitionForArchetypedAction(t *testing.T) {
	for _, opt := range []string{
		"../../../testkit/corpus/flat-conformance/templates/conformance_ehrbase.de.v0.opt",
		"../../../testkit/corpus/crossformat/test_all_types/template.opt",
	} {
		t.Run(opt[strings.LastIndexByte(opt, '/')+1:], func(t *testing.T) {
			wt, err := webtemplate.Build(compileFixture(t, opt))
			if err != nil {
				t.Fatalf("Build: %v", err)
			}
			actions := actionNodes(wt)
			if len(actions) == 0 {
				t.Fatal("no ACTION node — fixture changed?")
			}
			for _, action := range actions {
				archetyped := 0
				for _, ch := range action.Children {
					if ch.RMType != "ISM_TRANSITION" {
						continue
					}
					if ch.NodeID == "" {
						t.Errorf("ACTION %s has an in-context ISM transition node %s beside its archetyped ones", action.AQLPath, ch.AQLPath)
						continue
					}
					archetyped++
				}
				if archetyped == 0 {
					t.Errorf("ACTION %s has no archetyped ISM transition node — fixture changed?", action.AQLPath)
				}
			}
		})
	}
}

// testAllTypesOPT is the vendored test_all_types operational template, whose
// one ACTION models three archetyped ISM transitions, at0003 to at0005.
const testAllTypesOPT = "../../../testkit/corpus/crossformat/test_all_types/template.opt"

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

// TestREQ106InContextIsmTransitionWhenEveryArchetypedOneIsPruned — REQ-106:
// an archetyped ISM_TRANSITION that constrains neither its current_state nor
// its careflow_step contributes no node, as any empty container does. An
// ACTION all of whose archetyped transitions are dropped so still gets the
// in-context `ism_transition` node, or the RM-mandatory transition would have
// no node to be written at. No vendored OPT has such a transition, so the case
// strips the three of test_all_types.
func TestREQ106InContextIsmTransitionWhenEveryArchetypedOneIsPruned(t *testing.T) {
	raw, err := os.ReadFile(testAllTypesOPT)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"at0003", "at0004", "at0005"} {
		raw = stripIsmTransition(t, raw, id)
	}
	actions := actionNodes(buildOPT(t, raw))
	if len(actions) != 1 {
		t.Fatalf("test_all_types has %d ACTION nodes, want 1 — fixture changed?", len(actions))
	}
	action := actions[0]
	var isms []*webtemplate.Node
	for _, ch := range action.Children {
		if ch.RMType == "ISM_TRANSITION" {
			isms = append(isms, ch)
		}
	}
	if len(isms) != 1 {
		t.Fatalf("ACTION %s has %d ISM_TRANSITION children, want the in-context one alone; children = %v", action.AQLPath, len(isms), childIDs(action))
	}
	if ism := isms[0]; ism.NodeID != "" || ism.AQLPath != action.AQLPath+"/ism_transition" || !slices.Contains(childIDs(ism), "current_state") {
		t.Errorf("ISM transition node = %s, want the in-context node at %s/ism_transition holding current_state", jsonOf(t, ism), action.AQLPath)
	}
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
