package webtemplate_test

// REQ-106 — node names. A node with a node id is named by the name its
// template pins on it, where it pins one, else by its archetype term, as the
// reference names it; its id derives from the same name.

import (
	"maps"
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
