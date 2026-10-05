package instance_test

import (
	"encoding/json"
	"fmt"
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// uidClasses are the classes whose generated instances REQ-107 gives a
// uid: COMPOSITION, the ENTRY concretes, GENERIC_ENTRY, the PARTY
// concretes and PARTY_RELATIONSHIP. Every other locatable gets none.
var uidClasses = []string{
	"COMPOSITION",
	"OBSERVATION", "EVALUATION", "INSTRUCTION", "ACTION", "ADMIN_ENTRY",
	"GENERIC_ENTRY",
	"PERSON", "ORGANISATION", "GROUP", "AGENT", "ROLE",
	"PARTY_RELATIONSHIP",
}

// recordingUIDs is a counting UIDSource that remembers every uid it
// issued.
type recordingUIDs struct {
	issued []string
}

func (r *recordingUIDs) next() *rm.HierObjectID {
	v := fmt.Sprintf("stamp-%04d", len(r.issued)+1)
	r.issued = append(r.issued, v)
	return &rm.HierObjectID{Value: v}
}

// identityOptions are the settings an identity case runs with: the policy,
// a recording UIDSource, and the options a COMPOSITION root needs.
func identityOptions(policy instance.Policy, uids *recordingUIDs) instance.Options {
	return instance.Options{
		Policy:    policy,
		Language:  "en",
		Territory: "NL",
		Composer:  testComposer(),
		Now:       defaultsNow,
		UIDSource: uids.next,
	}
}

// TestREQ107_UIDClassRootsTakeUIDFromSource is the REQ-107 check that a
// COMPOSITION, an ENTRY, a GENERIC_ENTRY, a PARTY and a PARTY_RELATIONSHIP
// generated as the template root carry a uid that Options.UIDSource issued,
// under both policies (RM PARTY Uid_mandatory for the PARTY classes).
func TestREQ107_UIDClassRootsTakeUIDFromSource(t *testing.T) {
	for _, class := range uidClasses {
		c := compileOPTText(t, optTemplate(class), true)
		for _, policy := range []instance.Policy{instance.Minimal, instance.Example} {
			t.Run(class+"/"+policy.String(), func(t *testing.T) {
				uids := &recordingUIDs{}
				out, err := instance.Generate(t.Context(), c, identityOptions(policy, uids))
				if err != nil {
					t.Fatalf("Generate: %v", err)
				}
				loc, ok := out.(rm.Locatable)
				if !ok {
					t.Fatalf("generated root is %T, want a locatable", out)
				}
				uid, _ := loc.GetUID().(*rm.HierObjectID)
				if uid == nil || !slices.Contains(uids.issued, uid.Value) {
					t.Errorf("%s.uid = %v, want a uid UIDSource issued (issued %v)", class, loc.GetUID(), uids.issued)
				}
			})
		}
	}
}

// identityTree is a compiled template whose generated tree the identity
// checks read. nested reports whether that tree holds archetype_details
// below its root.
type identityTree struct {
	name   string
	c      *templatecompile.Compiled
	nested bool
}

// identityTrees are generated trees that hold many locatable classes:
// vendored COMPOSITIONs with nested entries, HISTORY, events, ITEM_TREE,
// CLUSTER and ELEMENT, one with SECTIONs, a PERSON with nested archetyped
// CLUSTERs, a CLUSTER template with CLUSTER slot fills, and CLUSTER,
// ELEMENT and ITEM_TREE roots.
func identityTrees(t *testing.T) []identityTree {
	t.Helper()
	trees := []identityTree{
		{name: "CLUSTER root", c: compileOPTText(t, optTemplate("CLUSTER", optMultiple("items", optNode("ELEMENT", "at0001"))), true)},
		{name: "ELEMENT root", c: compileOPTText(t, optTemplate("ELEMENT"), true)},
		{name: "ITEM_TREE root", c: compileOPTText(t, optTemplate("ITEM_TREE", optMultiple("items", optNode("CLUSTER", "at0001"))), true)},
		{name: "CLUSTER slot fill", c: compileOPTText(t, optTemplate("CLUSTER", optMultiple("items", optSlot("CLUSTER", "at0001"))), true), nested: true},
	}
	for _, name := range []string{
		"templates/vital_signs", "templates/nested.en.v1", "templates/TestPerson.v2", "templates/cluster-slot.ehrbase.org.v0",
	} {
		trees = append(trees, identityTree{name: name, c: compileCorpusOPT(t, name), nested: true})
	}
	return trees
}

// compileCorpusOPT compiles the vendored OPT that ListAllOPTs names name.
func compileCorpusOPT(t *testing.T, name string) *templatecompile.Compiled {
	t.Helper()
	refs, err := fixtures.ListAllOPTs()
	if err != nil {
		t.Fatalf("ListAllOPTs: %v", err)
	}
	for _, ref := range refs {
		if ref.Name != name {
			continue
		}
		opt, err := template.ParseFile(ref.Path)
		if err != nil {
			t.Fatalf("ParseFile %s: %v", name, err)
		}
		c, err := templatecompile.Compile(opt)
		if err != nil {
			t.Fatalf("Compile %s: %v", name, err)
		}
		return c
	}
	t.Fatalf("no vendored OPT named %s", name)
	return nil
}

// generatedLocatable is one locatable of a generated tree, read from its
// canonical JSON.
type generatedLocatable struct {
	path    string
	rmType  string
	hasUID  bool
	details map[string]any // archetype_details, or nil
}

// generatedLocatables returns every locatable in out: each JSON object
// that carries an archetype_node_id.
func generatedLocatables(t *testing.T, out any) []generatedLocatable {
	t.Helper()
	b, err := canjson.Marshal(out)
	if err != nil {
		t.Fatalf("canjson.Marshal: %v", err)
	}
	var doc any
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	var found []generatedLocatable
	var walk func(v any, path string)
	walk = func(v any, path string) {
		switch n := v.(type) {
		case map[string]any:
			if _, ok := n["archetype_node_id"]; ok {
				rmType, _ := n["_type"].(string)
				details, _ := n["archetype_details"].(map[string]any)
				_, hasUID := n["uid"]
				found = append(found, generatedLocatable{path: path, rmType: rmType, hasUID: hasUID, details: details})
			}
			for k, child := range n {
				walk(child, path+"/"+k)
			}
		case []any:
			for i, child := range n {
				walk(child, fmt.Sprintf("%s[%d]", path, i))
			}
		}
	}
	walk(doc, "")
	return found
}

// TestREQ107_OnlyUIDClassesCarryAUID is the REQ-107 check that the
// generator gives a uid to every COMPOSITION, ENTRY, GENERIC_ENTRY, PARTY
// and PARTY_RELATIONSHIP in a generated tree and to no other locatable,
// under both policies.
func TestREQ107_OnlyUIDClassesCarryAUID(t *testing.T) {
	for _, tree := range identityTrees(t) {
		for _, policy := range []instance.Policy{instance.Minimal, instance.Example} {
			t.Run(tree.name+"/"+policy.String(), func(t *testing.T) {
				out, err := instance.Generate(t.Context(), tree.c, identityOptions(policy, &recordingUIDs{}))
				if err != nil {
					t.Fatalf("Generate: %v", err)
				}
				locs := generatedLocatables(t, out)
				if len(locs) == 0 {
					t.Fatal("generated tree has no locatable")
				}
				for _, loc := range locs {
					if want := slices.Contains(uidClasses, loc.rmType); loc.hasUID != want {
						t.Errorf("%s at %q: uid present = %t, want %t", loc.rmType, loc.path, loc.hasUID, want)
					}
				}
			})
		}
	}
}

// TestREQ107_TemplateIDOnTheRootOnly is the REQ-107 check that the
// template id is written in the template root's archetype_details and in
// no other archetype_details, such as a nested OBSERVATION's, a nested
// archetyped CLUSTER's or a slot fill's, under both policies.
func TestREQ107_TemplateIDOnTheRootOnly(t *testing.T) {
	for _, tree := range identityTrees(t) {
		for _, policy := range []instance.Policy{instance.Minimal, instance.Example} {
			t.Run(tree.name+"/"+policy.String(), func(t *testing.T) {
				out, err := instance.Generate(t.Context(), tree.c, identityOptions(policy, &recordingUIDs{}))
				if err != nil {
					t.Fatalf("Generate: %v", err)
				}
				nested := 0
				for _, loc := range generatedLocatables(t, out) {
					if loc.details == nil {
						continue
					}
					tid, hasTID := loc.details["template_id"].(map[string]any)
					if loc.path == "" {
						if got, _ := tid["value"].(string); got != tree.c.TemplateID() {
							t.Errorf("root archetype_details.template_id = %v, want %q", loc.details["template_id"], tree.c.TemplateID())
						}
						continue
					}
					nested++
					if hasTID {
						t.Errorf("%s at %q: archetype_details.template_id = %v, want none", loc.rmType, loc.path, tid)
					}
				}
				if tree.c.Root().ArchetypeID() == "" {
					t.Fatalf("template root names no archetype, want one so it has archetype_details")
				}
				if tree.nested && nested == 0 {
					t.Errorf("generated tree has no nested archetype_details, want at least one")
				}
			})
		}
	}
}
