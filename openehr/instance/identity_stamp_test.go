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
// uid: the ENTRY concretes, GENERIC_ENTRY, the PARTY concretes and
// PARTY_RELATIONSHIP. Every other locatable, a COMPOSITION included, gets
// one only where its OPT requires it.
var uidClasses = []string{
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

// TestREQ107_UIDClassRootsTakeUIDFromSource is the REQ-107 check that an
// ENTRY, a GENERIC_ENTRY, a PARTY and a PARTY_RELATIONSHIP generated as
// the template root carry a uid that Options.UIDSource issued, under every
// policy (RM PARTY Uid_mandatory for the PARTY classes).
func TestREQ107_UIDClassRootsTakeUIDFromSource(t *testing.T) {
	for _, class := range uidClasses {
		c := compileOPTText(t, optTemplate(class), true)
		for _, policy := range everyPolicy {
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
// below its root; requiredBare, that it holds none under Required, since
// the template makes every nested archetype optional and Required builds
// none (REQ-107 Visit rule, What a visit writes).
type identityTree struct {
	name         string
	c            *templatecompile.Compiled
	nested       bool
	requiredBare bool
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
		bare := name == "templates/vital_signs" || name == "templates/nested.en.v1"
		trees = append(trees, identityTree{name: name, c: compileCorpusOPT(t, name), nested: true, requiredBare: bare})
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
// generator gives a uid to every ENTRY, GENERIC_ENTRY, PARTY and
// PARTY_RELATIONSHIP in a generated tree and to no other locatable whose
// OPT does not require one, a COMPOSITION included, under every policy.
func TestREQ107_OnlyUIDClassesCarryAUID(t *testing.T) {
	for _, tree := range identityTrees(t) {
		for _, policy := range everyPolicy {
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
// archetyped CLUSTER's or a slot fill's, under every policy. Under Required
// a template whose nested archetypes are all optional holds none to check.
func TestREQ107_TemplateIDOnTheRootOnly(t *testing.T) {
	for _, tree := range identityTrees(t) {
		for _, policy := range everyPolicy {
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
				bare := policy == instance.Required && tree.requiredBare
				wantNested := tree.nested && !bare
				if wantNested && nested == 0 {
					t.Errorf("generated tree has no nested archetype_details, want at least one")
				}
				if !wantNested && tree.nested && nested > 0 {
					t.Errorf("generated tree has %d nested archetype_details, want none: the template makes each nested archetype optional", nested)
				}
			})
		}
	}
}

// partyClasses are the PARTY concretes, whose uid the RM needs (PARTY
// Uid_mandatory).
var partyClasses = []string{"PERSON", "ORGANISATION", "GROUP", "AGENT", "ROLE"}

// TestREQ107_ProhibitedUIDYieldsExceptOnAParty is the REQ-107 check that
// the generator gives no uid to a COMPOSITION, an ENTRY, a GENERIC_ENTRY or
// a PARTY_RELATIONSHIP whose OPT prohibits its uid, and gives a PARTY one
// all the same, because the RM needs it. Each class is the template root;
// a PARTY_RELATIONSHIP is also nested in a PERSON's relationships. It
// holds under every policy, both value fills and both compile modes, and
// the output passes the RM floor.
func TestREQ107_ProhibitedUIDYieldsExceptOnAParty(t *testing.T) {
	type tree struct {
		name    string
		opt     string
		located func(t *testing.T, out any) rm.Locatable
		wantUID bool
	}
	root := func(_ *testing.T, out any) rm.Locatable { return out.(rm.Locatable) }
	var cases []tree
	for _, class := range append([]string{"COMPOSITION"}, uidClasses...) {
		cases = append(cases, tree{
			name:    class + " root",
			opt:     optTemplate(class, optProhibitedSingle("uid")),
			located: root,
			wantUID: slices.Contains(partyClasses, class),
		})
	}
	cases = append(cases, tree{
		name: "PARTY_RELATIONSHIP in PERSON.relationships",
		opt: optTemplate("PERSON", optMultiple("relationships",
			optNode("PARTY_RELATIONSHIP", "at0001", optProhibitedSingle("uid")))),
		located: func(t *testing.T, out any) rm.Locatable {
			p := out.(*rm.Person)
			if len(p.Relationships) != 1 {
				t.Fatalf("PERSON.relationships has %d members, want 1", len(p.Relationships))
			}
			return &p.Relationships[0]
		},
	})
	for _, tc := range cases {
		for _, implicit := range []bool{true, false} {
			c := compileOPTText(t, tc.opt, implicit)
			for _, opts := range defaultsOptions() {
				uids := &recordingUIDs{}
				opts.UIDSource = uids.next
				opts.Language, opts.Territory, opts.Composer = "en", "NL", testComposer()
				t.Run(fmt.Sprintf("%s/implicit=%t/%v/%v", tc.name, implicit, opts.Policy, opts.ValueFill), func(t *testing.T) {
					out, err := instance.Generate(t.Context(), c, opts)
					if err != nil {
						t.Fatalf("Generate: %v", err)
					}
					uid := tc.located(t, out).GetUID()
					if has := uid != nil && !rm.IsTypedNil(uid); has != tc.wantUID {
						t.Errorf("uid = %v, present %t, want %t", uid, has, tc.wantUID)
					}
					noFloorErrors(t, out)
				})
			}
		}
	}
}

// optRequiredSingle is a C_SINGLE_ATTRIBUTE called name with existence
// 1..1 over children.
func optRequiredSingle(name string, children ...string) string {
	return optSingle(name, children...)
}

// TestREQ107_UIDIsNotVisited is the REQ-107 check that the walk never
// visits a locatable's uid, which the identity rule decides: an ENTRY
// whose OPT names its uid, with no child or with a HIER_OBJECT_ID child,
// and a COMPOSITION or a CLUSTER whose OPT requires it, keep the uid the
// generator stamps from Options.UIDSource, and a COMPOSITION or a CLUSTER
// whose OPT names an optional uid gets none. It holds under every policy,
// both value fills and both compile modes.
func TestREQ107_UIDIsNotVisited(t *testing.T) {
	hier := optNode("HIER_OBJECT_ID", "", optStringAttr("value", "<list>from-the-opt</list>"))
	uidAttrs := map[string]string{
		"optional, no child":         optOptionalSingle("uid"),
		"optional, a HIER_OBJECT_ID": optOptionalSingleOver("uid", hier),
		"required, a HIER_OBJECT_ID": optRequiredSingle("uid", hier),
	}
	for shape, attr := range uidAttrs {
		for _, class := range []string{"COMPOSITION", "OBSERVATION", "CLUSTER"} {
			wantStamp := class == "OBSERVATION" || shape == "required, a HIER_OBJECT_ID"
			for _, implicit := range []bool{true, false} {
				c := compileOPTText(t, optTemplate(class, attr), implicit)
				for _, opts := range defaultsOptions() {
					uids := &recordingUIDs{}
					opts.UIDSource = uids.next
					opts.Territory, opts.Composer = "NL", testComposer()
					t.Run(fmt.Sprintf("%s/%s/implicit=%t/%v/%v", class, shape, implicit, opts.Policy, opts.ValueFill), func(t *testing.T) {
						out, err := instance.Generate(t.Context(), c, opts)
						if err != nil {
							t.Fatalf("Generate: %v", err)
						}
						uid, _ := out.(rm.Locatable).GetUID().(*rm.HierObjectID)
						switch {
						case wantStamp && (uid == nil || !slices.Contains(uids.issued, uid.Value)):
							t.Errorf("%s.uid = %v, want the uid UIDSource issued", class, out.(rm.Locatable).GetUID())
						case !wantStamp && out.(rm.Locatable).GetUID() != nil:
							t.Errorf("%s.uid = %v, want none", class, out.(rm.Locatable).GetUID())
						}
					})
				}
			}
		}
	}
}

// TestREQ107_RequiredUIDOnOtherLocatables is the REQ-107 check that a
// locatable outside the classes the identity rule names gets a uid only
// where its OPT requires one (an existence lower bound of at least 1),
// drawn from Options.UIDSource, or from the random fallback when that is
// nil, and none where its OPT names the uid optional, under Large too.
// It holds for a COMPOSITION root, and a CLUSTER and an ELEMENT at the
// root and nested, under every policy, both value fills and both compile
// modes, and the output passes the RM floor.
func TestREQ107_RequiredUIDOnOtherLocatables(t *testing.T) {
	type place struct {
		name   string
		opt    func(uid string) string
		target func(t *testing.T, out any) rm.Locatable
	}
	places := []place{
		{
			name:   "COMPOSITION root",
			opt:    func(uid string) string { return optTemplate("COMPOSITION", uid) },
			target: func(_ *testing.T, out any) rm.Locatable { return out.(rm.Locatable) },
		},
		{
			name: "CLUSTER root",
			opt: func(uid string) string {
				return optTemplate("CLUSTER", uid, optMultiple("items", optNode("ELEMENT", "at0001")))
			},
			target: func(_ *testing.T, out any) rm.Locatable { return out.(rm.Locatable) },
		},
		{
			name:   "ELEMENT root",
			opt:    func(uid string) string { return optTemplate("ELEMENT", uid) },
			target: func(_ *testing.T, out any) rm.Locatable { return out.(rm.Locatable) },
		},
		{
			name: "ELEMENT in CLUSTER.items",
			opt: func(uid string) string {
				return optTemplate("CLUSTER", optMultiple("items", optNode("ELEMENT", "at0001", uid)))
			},
			target: func(t *testing.T, out any) rm.Locatable {
				items := out.(*rm.Cluster).Items
				if len(items) != 1 {
					t.Fatalf("CLUSTER.items has %d members, want 1", len(items))
				}
				return items[0].(rm.Locatable)
			},
		},
	}
	for _, p := range places {
		for _, required := range []bool{true, false} {
			uidAttr := optOptionalSingle("uid")
			if required {
				uidAttr = optRequiredSingle("uid")
			}
			for _, implicit := range []bool{true, false} {
				c := compileOPTText(t, p.opt(uidAttr), implicit)
				for _, opts := range defaultsOptions() {
					for _, source := range []string{"UIDSource", "random fallback"} {
						uids := &recordingUIDs{}
						if source == "UIDSource" {
							opts.UIDSource = uids.next
						} else {
							opts.UIDSource = nil
						}
						opts.Territory, opts.Composer = "NL", testComposer()
						t.Run(fmt.Sprintf("%s/required=%t/%s/implicit=%t/%v/%v", p.name, required, source, implicit, opts.Policy, opts.ValueFill), func(t *testing.T) {
							out, err := instance.Generate(t.Context(), c, opts)
							if err != nil {
								t.Fatalf("Generate: %v", err)
							}
							uid, _ := p.target(t, out).GetUID().(*rm.HierObjectID)
							switch {
							case !required && uid != nil:
								t.Errorf("uid = %v, want none: the OPT names it optional", uid)
							case required && uid == nil:
								t.Errorf("uid absent, want one: the OPT requires it")
							case required && source == "UIDSource" && !slices.Contains(uids.issued, uid.Value):
								t.Errorf("uid = %q, want one UIDSource issued (%v)", uid.Value, uids.issued)
							}
							noFloorErrors(t, out)
						})
					}
				}
			}
		}
	}
}

// TestREQ107_CorpusCompositionsCarryNoUID is the REQ-107 Identity check,
// over every vendored COMPOSITION template, that the generator writes no
// COMPOSITION uid where the OPT does not require one, which no vendored
// OPT does: the server that stores the composition gives it its version
// id. It holds under every policy, both value fills and several fixed
// seeds.
func TestREQ107_CorpusCompositionsCarryNoUID(t *testing.T) {
	checked := 0
	eachCorpusRun(t, func(run corpusRun) {
		comp, ok := run.out.(*rm.Composition)
		if !ok {
			return
		}
		if attr := run.compiled.Root().Attribute("uid"); attr != nil {
			t.Fatalf("%s: the OPT constrains the COMPOSITION uid; this check assumes no vendored OPT does", run)
		}
		checked++
		if comp.UID != nil {
			t.Errorf("%s: COMPOSITION uid = %v, want none: the OPT does not require it", run, comp.UID)
		}
	})
	if checked == 0 {
		t.Fatal("no vendored COMPOSITION generated, so nothing was checked")
	}
}
