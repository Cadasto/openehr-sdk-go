package webtemplate_test

// REQ-106 — node names. A node with a node id is named by the name its
// template pins on it, where it pins one, else by its archetype term, as the
// reference names it; its id derives from the same name.

import (
	"maps"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/template/webtemplate"
)

// nodeNames is the part of a node the names test compares: its name, its
// localized name and its localized names.
type nodeNames struct {
	name, localized string
	names           map[string]string
}

func (n nodeNames) equal(o nodeNames) bool {
	return n.name == o.name && n.localized == o.localized && maps.Equal(n.names, o.names)
}

// TestREQ106NamesMatchTheReference — REQ-106, PROBE-075: every node with a
// node id carries the reference golden's name, localizedName and
// localizedNames, on the three parity oracles. Corona_Anamnese and
// GECCO_Diagnose pin names on reused archetypes, where the name is the one
// the template pins (Corona's "Husten", not its archetype term); constrain_test
// pins none, so every name there is the term.
func TestREQ106NamesMatchTheReference(t *testing.T) {
	for _, stem := range parityFixtures {
		t.Run(stem, func(t *testing.T) {
			ref := map[string]nodeNames{}
			walkRefTree(refTree(t, loadReferenceStem(t, stem)), func(m map[string]any) {
				if refStr(m, "nodeId") == "" {
					return
				}
				names := map[string]string{}
				if ln, ok := m["localizedNames"].(map[string]any); ok {
					for lang, v := range ln {
						s, _ := v.(string)
						names[lang] = s
					}
				}
				ref[refStr(m, "aqlPath")+" "+refStr(m, "id")] = nodeNames{refStr(m, "name"), refStr(m, "localizedName"), names}
			})
			if len(ref) == 0 {
				t.Fatalf("the %s golden holds no node with a node id", stem)
			}
			ours := map[string]nodeNames{}
			walkOurTree(buildOPT(t, readFile(t, referenceDir+"/"+stem+".opt")).Tree, func(n *webtemplate.Node) {
				if n.NodeID != "" {
					ours[n.AQLPath+" "+n.ID] = nodeNames{n.Name, n.LocalizedName, n.LocalizedNames}
				}
			})
			for key, want := range ref {
				got, ok := ours[key]
				switch {
				case !ok:
					t.Errorf("%s: no node, want one named %q", key, want.name)
				case !got.equal(want):
					t.Errorf("%s: names = %+v, want the golden's %+v", key, got, want)
				}
			}
		})
	}
}

// TestREQ106RootNameIsThePinnedName — REQ-106: the IDCR - Vital Signs
// Encounter.v1 template pins "Vital Signs Observations" on its composition,
// whose archetype term is "Encounter", so the root is named, and its id
// derived, from the pinned name; its description stays the term's.
func TestREQ106RootNameIsThePinnedName(t *testing.T) {
	const pinned = "Vital Signs Observations"
	wt := buildOPT(t, readFile(t, "../../../testkit/corpus/templates/IDCR - Vital Signs Encounter.v1.opt"))
	root := wt.Tree
	got := nodeNames{root.Name, root.LocalizedName, root.LocalizedNames}
	want := nodeNames{pinned, pinned, map[string]string{wt.DefaultLanguage: pinned}}
	if !got.equal(want) {
		t.Errorf("root names = %+v, want %+v", got, want)
	}
	if root.ID != "vital_signs_observations" {
		t.Errorf("root id = %q, want vital_signs_observations", root.ID)
	}
	const description = "Interaction, contact or care event between a subject of care and healthcare provider(s)."
	if d := root.LocalizedDescriptions[wt.DefaultLanguage]; d != description {
		t.Errorf("root description = %q, want the archetype term's %q", d, description)
	}
}

// TestREQ106InContextNamesMatchTestAllTypes — REQ-106: every node without a
// node id that the builder names carries the name of the test_all_types
// golden, the one vendored golden the reference's own code regenerated: the
// in-context leaves in lower case, as their RM attribute names, and the merged
// ACTION node's `Careflow_step` and `Current_state`, which the reference
// capitalises. The older goldens capitalise every in-context leaf, so they are
// not compared here.
func TestREQ106InContextNamesMatchTestAllTypes(t *testing.T) {
	ref := map[string]string{}
	walkRefTree(refTree(t, loadReferenceStem(t, actionParityStem)), func(m map[string]any) {
		if refStr(m, "nodeId") == "" {
			ref[refStr(m, "aqlPath")+" "+refStr(m, "id")] = refStr(m, "name")
		}
	})
	compared := map[string]bool{}
	walkOurTree(buildOPT(t, readFile(t, testAllTypesOPT)).Tree, func(n *webtemplate.Node) {
		if n.NodeID != "" || n.Name == "" {
			return
		}
		key := n.AQLPath + " " + n.ID
		want, ok := ref[key]
		if !ok {
			t.Errorf("%s: named %q, but the golden has no node there", key, n.Name)
			return
		}
		if n.Name != want {
			t.Errorf("%s: name = %q, want the golden's %q", key, n.Name, want)
		}
		compared[n.ID] = true
	})
	for _, id := range []string{
		"language", "territory", "composer", "start_time", "setting", "encoding", "subject", "time", "time_1",
		"narrative", "expiry_time", "timing", "action_archetype_id",
		"ism_transition", "careflow_step", "current_state", "transition",
	} {
		if !compared[id] {
			t.Errorf("no named %s node compared — fixture or builder changed?", id)
		}
	}
}

// TestREQ106InContextLeavesAreNamedByTheirAttribute — REQ-106: an in-context
// node is named by the RM attribute it stands for, in lower case, as the
// reference's code names it (`start_time`, not `Start_time`), wherever the
// builder adds it: the INTERVAL_EVENT `math_function` and `width` of
// Corona_Anamnese, which test_all_types lacks, and the unconstrained
// `ism_transition` node and its `current_state` of minimal_action_2. Only the
// merged ACTION node's `Careflow_step` and `Current_state` are capitalised, as
// the reference names them.
func TestREQ106InContextLeavesAreNamedByTheirAttribute(t *testing.T) {
	tests := []struct {
		opt string
		ids []string // in-context ids the template must give, to keep the test honest
	}{
		{opt: referenceDir + "/Corona_Anamnese.opt", ids: []string{"language", "territory", "composer", "math_function", "width", "time"}},
		{opt: minimalActionOPT, ids: []string{"ism_transition", "current_state", "transition", "time", "subject"}},
		{opt: testAllTypesOPT, ids: []string{"narrative", "expiry_time", "timing", "action_archetype_id", "start_time", "setting"}},
	}
	for _, tc := range tests {
		t.Run(tc.opt[strings.LastIndexByte(tc.opt, '/')+1:], func(t *testing.T) {
			seen := map[string]bool{}
			var walk func(n *webtemplate.Node, merged bool)
			walk = func(n *webtemplate.Node, merged bool) {
				for _, ch := range n.Children {
					attr := ch.AQLPath[strings.LastIndexByte(ch.AQLPath, '/')+1:]
					inContext := ch.NodeID == "" && ch.Name != "" && !strings.Contains(attr, "[")
					want := attr
					if merged && (attr == "careflow_step" || attr == "current_state") {
						want = strings.ToUpper(attr[:1]) + attr[1:]
					}
					if inContext {
						seen[ch.ID] = true
						if ch.Name != want {
							t.Errorf("%s: name = %q, want %q", ch.AQLPath, ch.Name, want)
						}
					}
					walk(ch, ch.RMType == "ISM_TRANSITION" && ch.NodeID == "" && childByID(ch, "careflow_step") != nil)
				}
			}
			walk(buildOPT(t, readFile(t, tc.opt)).Tree, false)
			for _, id := range tc.ids {
				if !seen[id] {
					t.Errorf("no in-context %s node — fixture or builder changed?", id)
				}
			}
		})
	}
}

// childByID returns the child of n with id, or nil.
func childByID(n *webtemplate.Node, id string) *webtemplate.Node {
	for _, ch := range n.Children {
		if ch.ID == id {
			return ch
		}
	}
	return nil
}

// TestREQ106CodedPinnedNameNamesTheNode — REQ-106, issue 455: the IDCR - Vital
// Signs Encounter.v1 template names the pulse rate ELEMENT, whose archetype
// term is "Rate", with the local code at1027, whose term text is "Heart Rate".
// The node takes that text as its name and localized names, derives its id
// from it, and carries it as the name predicate of its aqlPath, as the
// reference names it; its description stays the archetype term's.
func TestREQ106CodedPinnedNameNamesTheNode(t *testing.T) {
	const (
		pinned = "Heart Rate"
		path   = "/content[openEHR-EHR-SECTION.vital_signs.v1]/items[openEHR-EHR-OBSERVATION.pulse.v1]/data[at0002]/events[at0003]/data[at0001]/items[at0004,'Heart Rate']/value"
	)
	wt := buildOPT(t, readFile(t, "../../../testkit/corpus/templates/IDCR - Vital Signs Encounter.v1.opt"))
	var rate *webtemplate.Node
	walkOurTree(wt.Tree, func(n *webtemplate.Node) {
		if n.RMType == "DV_QUANTITY" && n.NodeID == "at0004" && strings.Contains(n.AQLPath, "OBSERVATION.pulse.v1") {
			rate = n
		}
	})
	if rate == nil {
		t.Fatal("no DV_QUANTITY leaf at0004 under the pulse OBSERVATION — fixture changed?")
	}
	if rate.ID != "heart_rate" || rate.AQLPath != path {
		t.Errorf("pulse rate id = %q, aqlPath = %q, want heart_rate at %s", rate.ID, rate.AQLPath, path)
	}
	got := nodeNames{rate.Name, rate.LocalizedName, rate.LocalizedNames}
	want := nodeNames{pinned, pinned, map[string]string{wt.DefaultLanguage: pinned}}
	if !got.equal(want) {
		t.Errorf("pulse rate names = %+v, want %+v", got, want)
	}
	const description = "The rate, measured in beats per minute."
	if d := rate.LocalizedDescriptions[wt.DefaultLanguage]; d != description {
		t.Errorf("pulse rate description = %q, want the archetype term's %q", d, description)
	}
}

// TestREQ106NodesWithoutNodeIDAreNamedByTheirAttribute — REQ-106, issue 455:
// a node with no node id, which no archetype term names, is named by the RM
// attribute that holds it, as the reference names it (OPTParser.buildNode):
// its name and localizedName are the attribute's name, it carries no
// localized names, and its id stays the one the attribute gives it. On
// IDCR - Procedures List.v1 that is the COMPOSITION's category and context,
// and each archetyped transition step's current_state and careflow_step.
// Across the vendored OPTs Build accepts, no node without a node id is left
// without a name.
func TestREQ106NodesWithoutNodeIDAreNamedByTheirAttribute(t *testing.T) {
	const step = "/content[openEHR-EHR-SECTION.procedures_rcp.v1]/items[openEHR-EHR-ACTION.procedure.v1]/ism_transition[at0034]"
	wt := buildOPT(t, readFile(t, "../../../testkit/corpus/ehrbase-service/templates/IDCR - Procedures List.v1.opt"))
	byPath := map[string]*webtemplate.Node{}
	walkOurTree(wt.Tree, func(n *webtemplate.Node) { byPath[n.AQLPath] = n })
	for _, path := range []string{"/category", "/context", step + "/current_state", step + "/careflow_step"} {
		attr := path[strings.LastIndexByte(path, '/')+1:]
		n := byPath[path]
		switch {
		case n == nil:
			t.Errorf("no node at %s — fixture or builder changed?", path)
		case n.NodeID != "":
			t.Errorf("%s: nodeId = %q, want none — fixture changed?", path, n.NodeID)
		case n.ID != attr || n.Name != attr || n.LocalizedName != attr || len(n.LocalizedNames) != 0:
			t.Errorf("%s: id %q, name %q, localizedName %q, localizedNames %v; want %s, %s, %s and none",
				path, n.ID, n.Name, n.LocalizedName, n.LocalizedNames, attr, attr, attr)
		}
	}

	built := 0
	for _, path := range vendoredOPTs(t) {
		_, w, refused := buildVendored(path)
		if refused != nil {
			continue
		}
		built++
		walkOurTree(w.Tree, func(n *webtemplate.Node) {
			if n.NodeID == "" && n.Name == "" {
				t.Errorf("%s: %s (%s, id %s) has no name", filepath.Base(path), n.AQLPath, n.RMType, n.ID)
			}
		})
	}
	if built == 0 {
		t.Fatal("Build accepted no vendored OPT — corpus layout changed?")
	}
}
