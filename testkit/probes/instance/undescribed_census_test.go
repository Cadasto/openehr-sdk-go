package instanceprobes_test

import (
	jsonv2 "encoding/json/v2"
	"fmt"
	mrand "math/rand/v2"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/bmmtype"
	"github.com/cadasto/openehr-sdk-go/internal/rmroots"
	tcimpl "github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/composition"
	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/rminfo"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// undescribedMember is one member of a generated root that no template
// node describes: a slot fill (kind "slot", ident its archetype id) or a
// LOCATABLE built from the BMM type of an attribute the template gives no
// allowed child (kind "bmm", ident its RM type). attr is the template path
// of the node that holds it and the attribute it sits in.
type undescribedMember struct {
	template string
	policy   string
	attr     string
	kind     string
	ident    string
}

func (m undescribedMember) line() string {
	return m.template + "\t" + m.policy + "\t" + m.attr + "\t" + m.kind + "\t" + m.ident
}

// undescribedPin is a row of undescribedBelowRequired: an undescribed
// member the walk builds below an optional node because the node that
// holds it is required, with the reason it is required.
type undescribedPin struct {
	member undescribedMember
	reason string
}

// undescribedBelowRequired pins every undescribed member a root Generate
// returns under Large and Medium holds below an optional node, for the
// compiling vendored templates. Each sits in a node the template or the RM
// requires, so REQ-107 § Slot fills keeps it: the rule reads an optional
// node's own attributes only, and below a required child the walk builds
// what § Contract says, undescribed members included where the BMM or the
// attribute's cardinality asks for a member. A child's occurrences add no
// such member, though the generator's existenceOnly reads them: a required
// child that is not a slot gets a member of its own, so no top-up runs, and
// the parser does not read a slot's occurrences (REQ-107 Known gap: slot
// occurrences). A list only its existence requires gets no undescribed
// member (What a visit writes), so vital_signs' protocols and event states,
// whose items the template leaves without a child or with slots only at an
// existence of 1..1 and a cardinality of 0..*, hold none. Medium builds
// only the first optional archetype root of a content attribute, so it does
// not reach clinical_content_validation's EVALUATION. A new such member, or
// a listed one that is gone, fails the census.
var undescribedBelowRequired = []undescribedPin{
	{
		member: undescribedMember{
			template: "templates/clinical_content_validation", policy: "large",
			attr: "/content[openEHR-EHR-EVALUATION.validation_evaliation_test.v0,'Validation evaluation wth single']/data/item",
			kind: "bmm", ident: "ELEMENT",
		},
		reason: "the RM requires the EVALUATION's data, which takes the ITEM_SINGLE at0001, and its item, which the template leaves without a child",
	},
}

// TestREQ107_UndescribedContentCensus is the corpus check of REQ-107 §
// Slot fills under Large and Medium: no optional node of a generated root
// holds an undescribed member in an attribute of its own, unless a top-up
// had to build that node (topUpLastResort), and the undescribed members
// below an optional node are exactly the ones undescribedBelowRequired
// lists. Required builds no optional node of its own, so it is not read
// here.
func TestREQ107_UndescribedContentCensus(t *testing.T) {
	refs, err := fixtures.ListAllOPTs()
	if err != nil {
		t.Fatalf("ListAllOPTs: %v", err)
	}
	observed := make(map[undescribedMember]struct{})
	var violations []string
	for _, ref := range refs {
		opt, err := template.ParseFile(ref.Path)
		if err != nil {
			continue // the ratchet's compile list covers it
		}
		compiled, err := templatecompile.Compile(opt)
		if err != nil {
			continue
		}
		for _, policy := range []instance.Policy{instance.Large, instance.Medium} {
			opts := instance.Options{Policy: policy, Language: "en", Now: ratchetNow}
			if compiled.Root().RMTypeName() == "COMPOSITION" {
				opts.Territory = "NL"
				opts.Composer = fixedComposer()
			}
			out, err := instance.Generate(t.Context(), compiled, opts)
			if err != nil {
				continue // the ratchet's refusal rows cover it
			}
			members, err := undescribedMembers(out, compiled)
			if err != nil {
				t.Fatalf("%s %s: %v", ref.Name, policy, err)
			}
			for _, m := range members {
				m.member.template, m.member.policy = ref.Name, policy.String()
				switch {
				case m.holderOptional:
					violations = append(violations, m.member.line())
				case m.underOptional:
					observed[m.member] = struct{}{}
				}
			}
		}
	}
	want := make(map[undescribedMember]struct{}, len(undescribedBelowRequired))
	for _, pin := range undescribedBelowRequired {
		if pin.reason == "" {
			t.Errorf("pin %s has no reason", pin.member.line())
		}
		want[pin.member] = struct{}{}
	}
	var unlisted, stale []string
	for m := range observed {
		if _, ok := want[m]; !ok {
			unlisted = append(unlisted, m.line())
		}
	}
	for m := range want {
		if _, ok := observed[m]; !ok {
			stale = append(stale, m.line())
		}
	}
	slices.Sort(violations)
	violations = slices.Compact(violations)
	slices.Sort(unlisted)
	slices.Sort(stale)
	if len(violations) > 0 {
		t.Errorf("an optional node no top-up had to build holds undescribed content in an attribute of its own:\n  %s", strings.Join(violations, "\n  "))
	}
	if len(unlisted) > 0 {
		t.Errorf("undescribed members below an optional node, not in undescribedBelowRequired:\n  %s", strings.Join(unlisted, "\n  "))
	}
	if len(stale) > 0 {
		t.Errorf("undescribedBelowRequired rows no longer generated:\n  %s", strings.Join(stale, "\n  "))
	}
}

// censusOPT is a template whose root is a CLUSTER archetype root with items
// over children, at a cardinality lower bound of 1.
func censusOPT(children ...string) string {
	return `<?xml version="1.0" encoding="utf-8"?>
<template xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xmlns="http://schemas.openehr.org/v1">
<language><terminology_id><value>ISO_639-1</value></terminology_id><code_string>en</code_string></language>
<template_id><value>census</value></template_id><concept>census</concept>
<definition><rm_type_name>CLUSTER</rm_type_name><node_id>at0000</node_id>` + censusItems(children...) +
		`<archetype_id><value>openEHR-EHR-CLUSTER.census.v1</value></archetype_id></definition>
</template>`
}

// censusItems is a C_MULTIPLE_ATTRIBUTE items over children, at a
// cardinality lower bound of 1.
func censusItems(children ...string) string {
	return `<attributes xsi:type="C_MULTIPLE_ATTRIBUTE"><rm_attribute_name>items</rm_attribute_name>` +
		strings.Join(children, "") +
		`<cardinality><is_ordered>false</is_ordered><is_unique>false</is_unique><interval>` +
		`<lower_included>true</lower_included><lower_unbounded>false</lower_unbounded>` +
		`<upper_unbounded>true</upper_unbounded><lower>1</lower></interval></cardinality></attributes>`
}

// censusOptional is a C_COMPLEX_OBJECT of rmType with occurrences 0..1.
func censusOptional(rmType, nodeID string, attrs ...string) string {
	return censusOccurring(rmType, nodeID, 0, attrs...)
}

// censusOccurring is a C_COMPLEX_OBJECT of rmType with occurrences
// lower..1.
func censusOccurring(rmType, nodeID string, lower int, attrs ...string) string {
	return `<children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>` + rmType + `</rm_type_name>` +
		`<occurrences><lower_included>true</lower_included><upper_included>true</upper_included>` +
		`<lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded>` +
		fmt.Sprintf(`<lower>%d</lower><upper>1</upper></occurrences>`, lower) +
		`<node_id>` + nodeID + `</node_id>` + strings.Join(attrs, "") + `</children>`
}

func compileCensusOPT(t *testing.T, xml string) *templatecompile.Compiled {
	t.Helper()
	opt, err := template.ParseOPT(strings.NewReader(xml))
	if err != nil {
		t.Fatalf("ParseOPT: %v", err)
	}
	c, err := templatecompile.Compile(opt)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	return c
}

// TestREQ107_CensusExemptsATopUpsLastResort checks the census's reading of
// a top-up (REQ-107 § Contract What a visit writes) against one root. The
// RM requires a CLUSTER's items, and the template's one child, the
// optional CLUSTER at0002, needs undescribed content, since its own items
// admit only a slot. Under Large and Medium the top-up builds a member of
// that child, so the census lists its slot fill below an optional node and
// reports no violation. Read beside a template that also allows a
// described ELEMENT, which the top-up would have taken, the same root holds
// a violation. Read beside one whose described sibling carries the node id
// at0002, which the top-up may not take, it holds none.
func TestREQ107_CensusExemptsATopUpsLastResort(t *testing.T) {
	slotOnly := censusOptional("CLUSTER", "at0002",
		censusItems(`<children xsi:type="ARCHETYPE_SLOT"><rm_type_name>CLUSTER</rm_type_name><node_id>at0003</node_id></children>`))
	element := `<children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>ELEMENT</rm_type_name>` +
		`<node_id>at0004</node_id></children>`
	cases := []struct {
		name      string
		against   string
		violation bool
	}{
		{name: "the template the root was generated from", against: censusOPT(slotOnly)},
		{name: "a described sibling the top-up would take", against: censusOPT(slotOnly, censusOptional("ELEMENT", "at0004")), violation: true},
		{name: "a described sibling that carries the child's node id", against: censusOPT(slotOnly, censusOptional("CLUSTER", "at0002", censusItems(element)))},
	}
	generated := compileCensusOPT(t, censusOPT(slotOnly))
	for _, policy := range []instance.Policy{instance.Large, instance.Medium} {
		out, err := instance.Generate(t.Context(), generated, instance.Options{Policy: policy, Language: "en", Now: ratchetNow})
		if err != nil {
			t.Fatalf("Generate(%s): %v", policy, err)
		}
		for _, tc := range cases {
			t.Run(fmt.Sprintf("%s/%s", tc.name, policy), func(t *testing.T) {
				findings, err := undescribedMembers(out, compileCensusOPT(t, tc.against))
				if err != nil {
					t.Fatalf("undescribedMembers: %v", err)
				}
				if len(findings) != 1 {
					t.Fatalf("undescribedMembers found %d members, want the one slot fill: %+v", len(findings), findings)
				}
				f := findings[0]
				if f.member.kind != "slot" || !f.underOptional {
					t.Errorf("finding %+v, want a slot fill below an optional node", f)
				}
				if f.holderOptional != tc.violation {
					t.Errorf("finding %s: holderOptional = %t, want %t", f.member.line(), f.holderOptional, tc.violation)
				}
			})
		}
	}
}

// censusTreeOPT is a template whose root is an ITEM_TREE archetype root
// with the items attribute items. The RM does not require an ITEM_TREE's
// items, so only the template's bounds ask for a member.
func censusTreeOPT(items string) string {
	return `<?xml version="1.0" encoding="utf-8"?>
<template xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xmlns="http://schemas.openehr.org/v1">
<language><terminology_id><value>ISO_639-1</value></terminology_id><code_string>en</code_string></language>
<template_id><value>census_tree</value></template_id><concept>census_tree</concept>
<definition><rm_type_name>ITEM_TREE</rm_type_name><node_id>at0000</node_id>` + items +
		`<archetype_id><value>openEHR-EHR-ITEM_TREE.census.v1</value></archetype_id></definition>
</template>`
}

// censusExistenceItems is a C_MULTIPLE_ATTRIBUTE items over children with
// an existence of 1..1 and a cardinality lower bound of 0: only the
// existence asks for a member.
func censusExistenceItems(children ...string) string {
	return `<attributes xsi:type="C_MULTIPLE_ATTRIBUTE"><rm_attribute_name>items</rm_attribute_name>` +
		`<existence><lower_included>true</lower_included><upper_included>true</upper_included>` +
		`<lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded>` +
		`<lower>1</lower><upper>1</upper></existence>` + strings.Join(children, "") +
		`<cardinality><is_ordered>false</is_ordered><is_unique>false</is_unique><interval>` +
		`<lower_included>true</lower_included><lower_unbounded>false</lower_unbounded>` +
		`<upper_unbounded>true</upper_unbounded><lower>0</lower></interval></cardinality></attributes>`
}

// censusRequiredItems is a C_MULTIPLE_ATTRIBUTE items over children with
// an existence of 1..1 and a cardinality lower bound of 1: the existence
// asks for the list, and the cardinality for its member.
func censusRequiredItems(children ...string) string {
	return `<attributes xsi:type="C_MULTIPLE_ATTRIBUTE"><rm_attribute_name>items</rm_attribute_name>` +
		`<existence><lower_included>true</lower_included><upper_included>true</upper_included>` +
		`<lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded>` +
		`<lower>1</lower><upper>1</upper></existence>` + strings.Join(children, "") +
		`<cardinality><is_ordered>false</is_ordered><is_unique>false</is_unique><interval>` +
		`<lower_included>true</lower_included><lower_unbounded>false</lower_unbounded>` +
		`<upper_unbounded>true</upper_unbounded><lower>1</lower></interval></cardinality></attributes>`
}

// TestREQ107_CensusGrantsNoLastResortToAnExistenceOnlyList checks the
// census's reading of a top-up (REQ-107 § Contract What a visit writes)
// where only a list's existence, or only its cardinality, asks for a
// member. The root is generated from an ITEM_TREE whose items have an
// existence of 1..1, a cardinality lower bound of 1 and one child, the
// optional CLUSTER at0002, whose own items admit only a slot: the top-up
// takes that CLUSTER as its last resort, so the census lists its slot fill
// below an optional node and reports no violation. Read beside the same
// tree whose items only their existence requires, where the top-up takes
// no child that needs undescribed content, and beside the same tree whose
// items only their cardinality requires, which Large and Medium do not
// visit since every member they would get needs undescribed content
// (REQ-107 Visit rule), the same root holds a violation.
func TestREQ107_CensusGrantsNoLastResortToAnExistenceOnlyList(t *testing.T) {
	slotOnly := censusOptional("CLUSTER", "at0002",
		censusItems(`<children xsi:type="ARCHETYPE_SLOT"><rm_type_name>CLUSTER</rm_type_name><node_id>at0003</node_id></children>`))
	cases := []struct {
		name      string
		against   string
		violation bool
	}{
		{name: "the template the root was generated from", against: censusTreeOPT(censusRequiredItems(slotOnly))},
		{name: "items only their existence requires", against: censusTreeOPT(censusExistenceItems(slotOnly)), violation: true},
		{name: "items only their cardinality requires", against: censusTreeOPT(censusItems(slotOnly)), violation: true},
	}
	generated := compileCensusOPT(t, censusTreeOPT(censusRequiredItems(slotOnly)))
	for _, policy := range []instance.Policy{instance.Large, instance.Medium} {
		out, err := instance.Generate(t.Context(), generated, instance.Options{Policy: policy, Language: "en", Now: ratchetNow})
		if err != nil {
			t.Fatalf("Generate(%s): %v", policy, err)
		}
		for _, tc := range cases {
			t.Run(fmt.Sprintf("%s/%s", tc.name, policy), func(t *testing.T) {
				findings, err := undescribedMembers(out, compileCensusOPT(t, tc.against))
				if err != nil {
					t.Fatalf("undescribedMembers: %v", err)
				}
				if len(findings) != 1 {
					t.Fatalf("undescribedMembers found %d members, want the one slot fill: %+v", len(findings), findings)
				}
				f := findings[0]
				if f.member.kind != "slot" || !f.underOptional {
					t.Errorf("finding %+v, want a slot fill below an optional node", f)
				}
				if f.holderOptional != tc.violation {
					t.Errorf("finding %s: holderOptional = %t, want %t", f.member.line(), f.holderOptional, tc.violation)
				}
			})
		}
	}
}

// TestREQ107_FamilyHistoryLeavesItsUnfillableSlotOut is the corpus check of
// REQ-107 § Slot fills on family_history.v.1.2.3: its slot at0027, whose
// includes no example can fill, sits in the optional CLUSTER at0024, whose
// required items admit only slots. That CLUSTER needs undescribed content,
// so no policy builds it, and Generate, under every policy and both value
// fills, and the builder return a root with no member of it that passes the
// RM floor and the template validator. Its body holds an ELEMENT except
// under Required, which builds only what the template requires
// (requiredBodies does not list it).
func TestREQ107_FamilyHistoryLeavesItsUnfillableSlotOut(t *testing.T) {
	const holder = "/content[openEHR-EHR-EVALUATION.family_history.v2]/data/items[at0003]/items[at0024]"
	c := compileFixture(t, "family_history.v.1.2.3")
	if n, err := c.NodeAt(holder); err != nil || n.IsSlot() {
		t.Fatalf("NodeAt(%s) = %v, %v; want the optional CLUSTER at0024", holder, n, err)
	}
	roots := make(map[string]any)
	for _, policy := range []instance.Policy{instance.Large, instance.Medium, instance.Required} {
		for _, fill := range []instance.ValueFill{instance.ExampleFill, instance.RandomFill} {
			out, err := instance.Generate(t.Context(), c, instance.Options{
				Policy: policy, ValueFill: fill, ValueSource: mrand.NewPCG(1, 2),
				Language: "en", Territory: "NL", Composer: fixedComposer(), Now: ratchetNow,
			})
			if err != nil {
				t.Fatalf("Generate(%s, %s): %v", policy, fill, err)
			}
			roots[fmt.Sprintf("Generate(%s, %s)", policy, fill)] = out
		}
	}
	b, err := composition.NewBuilder(t.Context(), c, composition.WithLanguage("en"), composition.WithTerritory("NL"),
		composition.WithComposer(fixedComposer()), composition.WithNow(ratchetNow))
	if err != nil {
		t.Fatalf("NewBuilder: %v", err)
	}
	comp, err := b.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	roots["builder"] = comp
	for call, out := range roots {
		reasons := outputReasons(t, out, c, true, instance.ExampleFill)
		if strings.HasPrefix(call, "Generate(required") {
			delete(reasons, reasonHollowBody+":/")
		}
		if len(reasons) > 0 {
			t.Errorf("%s: %d findings, want none: %v", call, len(reasons), reasons)
		}
		raw, err := canjson.Marshal(out)
		if err != nil {
			t.Fatalf("%s: canjson.Marshal: %v", call, err)
		}
		if strings.Contains(string(raw), `"archetype_node_id":"at0024"`) {
			t.Errorf("%s: holds a member of the CLUSTER at0024, want none", call)
		}
	}
}

// censusFinding is an undescribed member with where it sits: holderOptional
// when the node that holds it is optional and is not the child a top-up
// had to take (topUpLastResort), so the walk built that node though
// nothing required it, and underOptional when that node or one above it
// is optional.
type censusFinding struct {
	member         undescribedMember
	holderOptional bool
	underOptional  bool
}

// undescribedMembers decodes root as canonical JSON and walks it beside the
// compiled template, binding each member to the allowed template child that
// describes it, as the template validator binds a LOCATABLE by its archetype
// id or node id. A member it binds to no child that is not a slot is
// undescribed, and the walk does not go below it.
func undescribedMembers(root any, compiled *templatecompile.Compiled) ([]censusFinding, error) {
	raw, err := canjson.Marshal(root)
	if err != nil {
		return nil, fmt.Errorf("canjson.Marshal: %w", err)
	}
	var tree map[string]any
	if err := jsonv2.Unmarshal(raw, &tree); err != nil {
		return nil, fmt.Errorf("json unmarshal: %w", err)
	}
	var out []censusFinding
	censusWalk(compiled.Root(), tree, false, false, &out)
	return out, nil
}

// censusWalk visits the members of every attribute of value, the JSON
// object bound to the template node node. underOptional reports whether
// node or a node above it is optional, and lastResort whether node is the
// child a top-up had to take (topUpLastResort). It binds every member of an
// attribute before it walks below any, so the room each child has left is
// known when it asks whether a member's node was a last resort.
func censusWalk(node *templatecompile.CompiledNode, value map[string]any, underOptional, lastResort bool, out *[]censusFinding) {
	for _, key := range sortedKeys(value) {
		if key == "_type" || key == "archetype_node_id" || key == "archetype_details" {
			continue
		}
		var attr *templatecompile.CompiledAttribute
		if node != nil {
			attr = node.Attribute(key)
		}
		members := objects(value[key])
		bound := make([]*templatecompile.CompiledNode, len(members))
		kinds := make([]string, len(members))
		built := make(map[*templatecompile.CompiledNode]int)
		for i, member := range members {
			bound[i], kinds[i] = censusBind(attr, member, built)
			if bound[i] != nil {
				built[bound[i]]++
			}
		}
		for i, member := range members {
			if kind := kinds[i]; kind != "" {
				ident, _ := member["_type"].(string)
				if kind == "slot" {
					ident, _ = member["archetype_node_id"].(string)
				}
				holderOptional := node != nil && nodeOptional(node) && !lastResort
				*out = append(*out, censusFinding{
					member:         undescribedMember{attr: holderPath(node) + "/" + key, kind: kind, ident: ident},
					holderOptional: holderOptional,
					underOptional:  underOptional,
				})
				continue
			}
			child := bound[i]
			below := underOptional
			if child != nil && nodeOptional(child) {
				below = true
			}
			censusWalk(child, member, below, child != nil && topUpLastResort(child, built, len(members)), out)
		}
	}
}

// censusBind binds member, a JSON object in attr, to an allowed template
// child of attr, given built, the members of attr bound to each child
// before it. It returns the child that describes member, or the kind of
// undescribed member it is: "bmm" for a LOCATABLE in an attribute with no
// allowed child, "slot" for one only a slot admits, and "unbound" for one
// no child admits. A LOCATABLE binds among the children that carry its
// archetype_node_id, ranked by its name as the template validator ranks
// them (REQ-102 Structural — binding items): those that pin that name,
// else those that pin none, else all of them; within the first rank that
// holds one, it binds to the first child with room (censusRoom), else the
// first of the rank, so each of the named uses of one archetype Large
// writes binds to its own child, and a member past a full child binds to
// the next one with room. The validator also tries each child's
// constraints, which the census does not. A member that is not a LOCATABLE
// binds to the first allowed child of its RM type, else the first allowed
// child, else none, and is never undescribed: only a LOCATABLE has a
// template node of its own.
func censusBind(attr *templatecompile.CompiledAttribute, member map[string]any, built map[*templatecompile.CompiledNode]int) (*templatecompile.CompiledNode, string) {
	children := allowedChildren(attr)
	id, locatable := member["archetype_node_id"].(string)
	if !locatable {
		typ, _ := member["_type"].(string)
		for _, c := range children {
			if c.RMTypeName() == typ {
				return c, ""
			}
		}
		if len(children) > 0 {
			return children[0], ""
		}
		return nil, ""
	}
	if len(children) == 0 {
		return nil, "bmm"
	}
	name := ""
	if n, ok := member["name"].(map[string]any); ok {
		name, _ = n["value"].(string)
	}
	slot := false
	var named, unnamed, carriers []*templatecompile.CompiledNode
	for _, c := range children {
		if c.IsSlot() {
			slot = true
			continue
		}
		if c.ArchetypeID() != id && c.NodeID() != id {
			continue
		}
		switch {
		case c.NodeName() != "" && c.NodeName() == name:
			named = append(named, c)
		case c.NodeName() == "":
			unnamed = append(unnamed, c)
		}
		carriers = append(carriers, c)
	}
	for _, rank := range [][]*templatecompile.CompiledNode{named, unnamed, carriers} {
		if len(rank) == 0 {
			continue
		}
		for _, c := range rank {
			if censusRoom(c, built[c]) {
				return c, ""
			}
		}
		return rank[0], ""
	}
	if slot {
		return nil, "slot"
	}
	return nil, "unbound"
}

// allowedChildren returns attr's template children whose occurrences upper
// bound is not 0.
func allowedChildren(attr *templatecompile.CompiledAttribute) []*templatecompile.CompiledNode {
	if attr == nil {
		return nil
	}
	var out []*templatecompile.CompiledNode
	for _, c := range attr.Children() {
		if occ := c.Occurrences(); occ != nil && !occ.UpperUnbounded() && occ.Upper() == 0 {
			continue
		}
		out = append(out, c)
	}
	return out
}

// nodeOptional reports whether the walk builds node though nothing
// requires it: a child of an attribute that is not required, by the BMM or
// by an existence or cardinality lower bound of at least 1, or a child of a
// multi-valued attribute whose occurrences lower bound is 0. The template
// root is required. This reads a required child of an optional multi-valued
// attribute as optional, although § Slot fills builds it where a described
// sibling has the attribute visited; no vendored template has one that
// holds undescribed content, so the census would name it if one came.
func nodeOptional(node *templatecompile.CompiledNode) bool {
	attr := parentAttribute(node)
	if attr == nil {
		return false
	}
	if !attrRequired(node.Parent(), attr) {
		return true
	}
	if attr.Cardinality() == template.Single {
		return false
	}
	occ := node.Occurrences()
	return occ != nil && !occ.LowerUnbounded() && occ.Lower() == 0
}

// parentAttribute returns the attribute of node's parent that holds node,
// or nil for the template root.
func parentAttribute(node *templatecompile.CompiledNode) *templatecompile.CompiledAttribute {
	parent := node.Parent()
	if parent == nil {
		return nil
	}
	for _, attr := range parent.Attributes() {
		if slices.Contains(attr.Children(), node) {
			return attr
		}
	}
	return nil
}

// attrRequired reports whether the walk has to give attr, an attribute of
// node, a member: the BMM marks it mandatory (bmmMandatory), or its
// existence or cardinality lower bound is at least 1.
func attrRequired(node *templatecompile.CompiledNode, attr *templatecompile.CompiledAttribute) bool {
	return bmmMandatory(node, attr) || tcimpl.LowerAtLeastOne(attr.Existence()) || tcimpl.LowerAtLeastOne(attr.ChildMultiplicity())
}

// bmmMandatory reports whether the BMM marks attr, an attribute of node,
// mandatory on the class the walk builds node as (builtClass), as the
// generator reads it: where the compile marks it so (Required), on the
// class node names or, for an untyped node, on the type of the attribute
// that holds it, and where the class the walk builds marks it so, as
// CLUSTER does the items of a LOCATABLE.
func bmmMandatory(node *templatecompile.CompiledNode, attr *templatecompile.CompiledAttribute) bool {
	if attr.Required() {
		return true
	}
	return slices.Contains(rminfo.Default.RequiredAttributes(builtClass(node)), attr.Name())
}

// topUpLastResort reports whether node, an optional child, is a child a
// top-up takes when no other will do (REQ-107 § Contract What a visit
// writes), given built, the members of its list bound to each child, and
// held, the members the list holds. Its list is multi-valued and asks the
// top-up for a member beyond its existence (topUpAsks), so a top-up builds
// members where the children's own fall short; the list holds no more
// members than the top-up fills it up to (topUpTarget), since a top-up adds
// none past that count, so a list that holds more got no top-up; every
// child before node that is not a slot is full (censusRoom) or one the walk
// refuses (censusRefused), which the top-up passes over, so node was the
// first child with room that is not a slot and that the walk builds;
// and no allowed child that needs no undescribed content
// (needsNoUndescribed) still has room and has an identity that no earlier
// sibling other than node carries while it has room, since the top-up would
// have taken that child first. A child full now may have had room when the
// top-up took node, but the room a child has never grows, so one with room
// now had it then. Where only the list's existence asks for a member, the
// top-up takes only a child that needs no undescribed content, so it has no
// last resort. Nor has a list that only its cardinality lower bound
// requires when every allowed child needs undescribed content: Large and
// Medium do not visit it (REQ-107 Visit rule), so it gets no top-up
// (visitedThoughUndescribed). A member built so is not one of node's own,
// which § Slot fills keeps out, so the census lists what it holds rather
// than calling it a violation. Unlike the generator's existenceOnly, this
// does not read a child's occurrences to decide whether the top-up asks
// beyond the existence, on purpose: a required child that is not a slot
// gets a member of its own, so no top-up runs for it, and the parser does
// not read a slot's occurrences (REQ-107 Known gap: slot occurrences).
// Reading them would only exempt a node that no top-up builds.
func topUpLastResort(node *templatecompile.CompiledNode, built map[*templatecompile.CompiledNode]int, held int) bool {
	attr := parentAttribute(node)
	if attr == nil || node.IsSlot() || attr.Cardinality() != template.Multiple || !topUpAsks(node.Parent(), attr) {
		return false
	}
	if held > topUpTarget(node.Parent(), attr) {
		return false
	}
	children := allowedChildren(attr)
	at := slices.Index(children, node)
	if at < 0 {
		return false
	}
	for _, c := range children[:at] {
		if !c.IsSlot() && censusRoom(c, built[c]) && !censusRefused(c) {
			return false
		}
	}
	for i, c := range children {
		if needsNoUndescribed(c) && censusRoom(c, built[c]) && !identityHeldBefore(children, i, node, built) {
			return false
		}
	}
	return visitedThoughUndescribed(node.Parent(), attr, children)
}

// topUpAsks reports whether the top-up of attr, a list of parent, asks for
// a member beyond the list's existence: the BMM marks it mandatory
// (bmmMandatory), its
// cardinality lower bound is at least 1, or it is a list the RM keeps
// non-empty once present on the class the walk builds parent as
// (builtClass; rminfo.NonEmptyLists, with no alternative, as the
// generator's rmNonEmptyList reads it), such as COMPOSITION.content or
// SECTION.items, whose existence lower bound is at least 1.
func topUpAsks(parent *templatecompile.CompiledNode, attr *templatecompile.CompiledAttribute) bool {
	if bmmMandatory(parent, attr) || tcimpl.LowerAtLeastOne(attr.ChildMultiplicity()) {
		return true
	}
	l, ok := rminfo.NonEmptyListOf(builtClass(parent), attr.Name())
	return ok && l.Alternative == "" && tcimpl.LowerAtLeastOne(attr.Existence())
}

// topUpTarget returns the number of members a top-up fills attr, a list of
// parent, up to (REQ-107 § Contract What a visit writes): its cardinality
// lower bound, or 1 where that is 0 and the list is required, since the
// BMM marks it mandatory (bmmMandatory), its existence lower bound is at
// least 1, or an
// allowed child that is not a slot has an occurrences lower bound of at
// least 1.
func topUpTarget(parent *templatecompile.CompiledNode, attr *templatecompile.CompiledAttribute) int {
	if cm := attr.ChildMultiplicity(); cm != nil && !cm.LowerUnbounded() && cm.Lower() > 0 {
		return cm.Lower()
	}
	if bmmMandatory(parent, attr) || tcimpl.LowerAtLeastOne(attr.Existence()) {
		return 1
	}
	for range tcimpl.MandatoryListChildren(attr) {
		return 1
	}
	return 0
}

// censusRefused reports whether the walk refuses a member of node, a child
// that is not a slot, for what node holds itself, so a top-up passes over
// it (REQ-107 § Contract What a visit writes): node is an object of a class
// that is always an archetype root for which the template names no
// archetype id, or an attribute of its own that the template does not
// prohibit is one the generator cannot write and the template requires
// (unwritableRequired).
func censusRefused(node *templatecompile.CompiledNode) bool {
	if node.IsSlot() {
		return false
	}
	if unnamedRoot(node) {
		return true
	}
	for _, attr := range node.Attributes() {
		if !attrProhibited(node, attr) && unwritableRequired(node, attr) {
			return true
		}
	}
	return false
}

// unnamedRoot reports whether node, a child that is not a slot, is built as
// an object of a class that is always an archetype root (builtClass) for
// which the template names no archetype id, which REQ-107 § Slot fills reads
// as content the template does not describe.
func unnamedRoot(node *templatecompile.CompiledNode) bool {
	return node.ArchetypeID() == "" && rmroots.IsArchetypeRoot(builtClass(node))
}

// builtClass returns the bare RM class the generator builds node as, which
// every rule of REQ-107 reads the node as (Generic and untyped types): the
// class node names, or, where that is abstract, the class the generator
// builds for it (censusSubstitutes). A node that names no RM type is built
// as the type the BMM declares for the attribute that holds it, read on the
// class its parent is built as, or as the constraint of that type where it
// is a formal generic parameter of that class, such as the T of an EVENT's
// data, and so substituted. It is "" for an untyped root.
func builtClass(node *templatecompile.CompiledNode) string {
	class := bmmtype.Class(strings.TrimSpace(node.RMTypeName()))
	if class == "" {
		parent, attr := node.Parent(), parentAttribute(node)
		if parent == nil || attr == nil {
			return ""
		}
		holder := builtClass(parent)
		held, ok := rminfo.Default.AttributeRMType(holder, attr.Name())
		if !ok {
			return ""
		}
		if constraint, formal := bmmtype.ParameterConstraint(holder, held); formal {
			held = constraint
		}
		class = bmmtype.Class(held)
	}
	if concrete, abstract := censusSubstitutes[class]; abstract {
		return concrete
	}
	return class
}

// censusSubstitutes names the concrete class the generator builds for each
// abstract LOCATABLE class a template node can be read as, as REQ-107
// What a visit writes builds it: POINT_EVENT for EVENT, ITEM_TREE for
// ITEM_STRUCTURE, ELEMENT for ITEM, OBSERVATION for an abstract entry, and
// CLUSTER for LOCATABLE. The census reads only a LOCATABLE's class.
// TestREQ107_CensusSubstitutesFollowTheGenerator pins each against the
// generator.
var censusSubstitutes = map[string]string{
	"EVENT":          "POINT_EVENT",
	"ITEM_STRUCTURE": "ITEM_TREE",
	"ITEM":           "ELEMENT",
	"CONTENT_ITEM":   "OBSERVATION",
	"CARE_ENTRY":     "OBSERVATION",
	"ENTRY":          "OBSERVATION",
	"LOCATABLE":      "CLUSTER",
}

// censusRoom reports whether child, an allowed child of a list that n
// members are bound to, has room for another: it is a slot, whose
// occurrences the parser does not read, its occurrences state no upper
// bound, or n is below it (REQ-102 Structural — binding items).
func censusRoom(child *templatecompile.CompiledNode, n int) bool {
	if child.IsSlot() {
		return true
	}
	occ := child.Occurrences()
	return occ == nil || occ.UpperUnbounded() || n < occ.Upper()
}

// visitedThoughUndescribed reports whether Large and Medium visit attr, a
// list of parent with the allowed children children, where a top-up of it
// can only take a child that needs undescribed content: the list is
// required by more than its cardinality lower bound, since the BMM marks
// it mandatory (bmmMandatory),
// its existence lower bound is at least 1, or an allowed child that is not
// a slot has an occurrences lower bound of at least 1, or one of its
// children needs no undescribed content, so the rule on undescribed
// content does not keep the walk out (REQ-107 Visit rule).
func visitedThoughUndescribed(parent *templatecompile.CompiledNode, attr *templatecompile.CompiledAttribute, children []*templatecompile.CompiledNode) bool {
	if bmmMandatory(parent, attr) || tcimpl.LowerAtLeastOne(attr.Existence()) {
		return true
	}
	for range tcimpl.MandatoryListChildren(attr) {
		return true
	}
	return slices.ContainsFunc(children, needsNoUndescribed)
}

// needsNoUndescribed reports whether node needs no undescribed content, as
// § Slot fills defines it: node is not a slot or an object of a class that
// is always an archetype root for which the template names no archetype id
// (unnamedRoot), and no attribute of its own
// that is required, and that the template does not prohibit, can be given
// only undescribed members or is one the generator cannot write
// (unwritableRequired). Such an attribute has no allowed child and an
// element type that is a LOCATABLE, or allows only slots. The census
// compiles with the implicit attributes, so node carries every attribute
// the BMM marks mandatory. A single-valued attribute with an allowed child
// that is not a slot counts as one that can be given a described member,
// although the walk builds the alternative the validator binds, which can
// be a slot: where the census cannot tell, it reads a sibling as needing
// none, so it reports a violation rather than grant an exemption.
func needsNoUndescribed(node *templatecompile.CompiledNode) bool {
	if node.IsSlot() || unnamedRoot(node) {
		return false
	}
	for _, attr := range node.Attributes() {
		if !attrRequired(node, attr) || attrProhibited(node, attr) {
			continue
		}
		if unwritableRequired(node, attr) {
			return false
		}
		children := allowedChildren(attr)
		if len(children) == 0 {
			if locatableType(attr.RMTypeName()) {
				return false
			}
			continue
		}
		if !slices.ContainsFunc(children, func(c *templatecompile.CompiledNode) bool { return !c.IsSlot() }) {
			return false
		}
	}
	return true
}

// unwritableRequired reports whether attr of node is an attribute the
// generator has no writer for that the template requires, so the walk would
// refuse it (REQ-107 Known gap: attributes the generator cannot write): a
// LOCATABLE's links or feeder_audit, an EHR_ACCESS's settings, or a list
// whose items the RM types as primitive values other than a DV_MULTIMEDIA's
// data, which the BMM marks mandatory on the class the walk builds node as
// (bmmMandatory), whose existence lower bound is at least 1, or which holds
// a mandatory child (tcimpl.MandatoryListChildren),
// other than a list that only its existence asks a member of, as the
// generator's writerNeeded reads it.
func unwritableRequired(node *templatecompile.CompiledNode, attr *templatecompile.CompiledAttribute) bool {
	class, name := builtClass(node), attr.Name()
	isList, _ := rminfo.Default.IsContainer(class, name)
	noWriter := false
	switch {
	case name == "links" || name == "feeder_audit":
		h, ok := rminfo.Default.(rminfo.Hierarchy)
		if ok {
			site, declared := h.DeclaredOn(class, name)
			noWriter = declared && site == "LOCATABLE"
		}
	case name == "settings":
		noWriter = class == "EHR_ACCESS"
	case isList && (class != "DV_MULTIMEDIA" || name != "data"):
		itemType, ok := rminfo.Default.AttributeRMType(class, name)
		ar, reports := rminfo.Default.(rminfo.AbsenceReporter)
		noWriter = ok && itemType != "" && itemType != "Hash" && reports && ar.AbsenceReason(itemType) == rminfo.AbsencePrimitive
	}
	if !noWriter {
		return false
	}
	existence := tcimpl.LowerAtLeastOne(attr.Existence())
	mandatoryChild := false
	for range tcimpl.MandatoryListChildren(attr) {
		mandatoryChild = true
		break
	}
	mandatory := bmmMandatory(node, attr)
	if !mandatory && !existence && !mandatoryChild {
		return false
	}
	return !isList || mandatory || tcimpl.AsksForMember(attr) || !existence
}

// attrProhibited reports whether the template prohibits attr, an attribute
// of node, which the BMM does not mark mandatory on the class the walk
// builds node as (bmmMandatory): an existence upper bound of 0, or children
// none of which it allows. The walk does not visit such an attribute.
func attrProhibited(node *templatecompile.CompiledNode, attr *templatecompile.CompiledAttribute) bool {
	if bmmMandatory(node, attr) {
		return false
	}
	if e := attr.Existence(); e != nil && !e.UpperUnbounded() && e.Upper() == 0 {
		return true
	}
	return len(attr.Children()) > 0 && len(allowedChildren(attr)) == 0
}

// locatableType reports whether the pinned RM knows rmType as a LOCATABLE.
func locatableType(rmType string) bool {
	h, ok := rminfo.Default.(rminfo.Hierarchy)
	if !ok {
		return false
	}
	conforms, known := h.ConformsTo(rmType, "LOCATABLE")
	return known && conforms
}

// identityHeldBefore reports whether a sibling before children[i], other
// than a slot, carries its identity, its archetype id when it is an
// archetype root and its node id otherwise, and is lastResort, the child a
// top-up took, which had room then, or still has room (censusRoom, given
// built). The validator binds a member to the first child that carries its
// archetype id or node id and has room, so the generator's top-up passes
// over a child whose identity such a sibling carries. A child with no
// identity is never carried.
func identityHeldBefore(children []*templatecompile.CompiledNode, i int, lastResort *templatecompile.CompiledNode, built map[*templatecompile.CompiledNode]int) bool {
	id := children[i].ArchetypeID()
	if id == "" {
		id = children[i].NodeID()
	}
	if id == "" {
		return false
	}
	return slices.ContainsFunc(children[:i], func(sib *templatecompile.CompiledNode) bool {
		return !sib.IsSlot() && (sib.ArchetypeID() == id || sib.NodeID() == id) &&
			(sib == lastResort || censusRoom(sib, built[sib]))
	})
}

// holderPath is node's template path, or "?" for a value built from the
// BMM, which has none.
func holderPath(node *templatecompile.CompiledNode) string {
	if node == nil {
		return "?"
	}
	return strings.TrimSuffix(node.AQLPath(), "/")
}

// objects returns v's JSON objects: v itself, or each object of the array
// v is.
func objects(v any) []map[string]any {
	switch x := v.(type) {
	case map[string]any:
		return []map[string]any{x}
	case []any:
		var out []map[string]any
		for _, e := range x {
			if m, ok := e.(map[string]any); ok {
				out = append(out, m)
			}
		}
		return out
	}
	return nil
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

// censusRootOPT is a template whose root is an archetype root of rmType
// with the attributes attrs.
func censusRootOPT(rmType string, attrs ...string) string {
	return `<?xml version="1.0" encoding="utf-8"?>
<template xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xmlns="http://schemas.openehr.org/v1">
<language><terminology_id><value>ISO_639-1</value></terminology_id><code_string>en</code_string></language>
<template_id><value>census_root</value></template_id><concept>census_root</concept>
<definition><rm_type_name>` + rmType + `</rm_type_name><node_id>at0000</node_id>` + strings.Join(attrs, "") +
		`<archetype_id><value>openEHR-EHR-` + rmType + `.census.v1</value></archetype_id></definition>
</template>`
}

// censusList is a C_MULTIPLE_ATTRIBUTE name over children with an existence
// of existence..1, or none where existence is -1, and a cardinality of
// lower..*.
func censusList(name string, existence, lower int, children ...string) string {
	out := `<attributes xsi:type="C_MULTIPLE_ATTRIBUTE"><rm_attribute_name>` + name + `</rm_attribute_name>`
	if existence >= 0 {
		out += fmt.Sprintf(`<existence><lower_included>true</lower_included><upper_included>true</upper_included>`+
			`<lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded>`+
			`<lower>%d</lower><upper>1</upper></existence>`, existence)
	}
	return out + strings.Join(children, "") +
		`<cardinality><is_ordered>false</is_ordered><is_unique>false</is_unique><interval>` +
		fmt.Sprintf(`<lower_included>true</lower_included><lower_unbounded>false</lower_unbounded>`+
			`<upper_unbounded>true</upper_unbounded><lower>%d</lower></interval></cardinality></attributes>`, lower)
}

// TestREQ107_CensusLastResortFollowsTheGenerator checks the census's
// reading of a top-up's last resort (REQ-107 § Contract What a visit
// writes) against roots generated under Large and Medium from the template
// they are read against. In each, the top-up has to build the optional
// node that holds the one slot fill, so the census lists that fill below an
// optional node and reports no violation:
//
//   - a SECTION's items, a list the RM keeps non-empty once present, at
//     existence 1..1 and cardinality 0..*: their existence asks for a member
//     the top-up takes from every source, the optional inner SECTION
//     included, whose own items admit only a slot;
//   - a CLUSTER's items at cardinality 2..* over an optional ELEMENT and an
//     optional CLUSTER whose items admit only a slot: the ELEMENT is full
//     after its own member, so the top-up takes the CLUSTER, the first child
//     with room;
//   - a CLUSTER's items over an optional CLUSTER whose items admit only a
//     slot and an optional ELEMENT whose feeder_audit the template requires,
//     which the generator cannot write: both need undescribed content, and
//     the top-up takes the first;
//   - the same two children the other way round: the walk refuses the
//     ELEMENT, so the top-up passes over it and takes the CLUSTER;
//   - a SECTION's items at existence 1..1 over an optional ADMIN_ENTRY the
//     template defines inline with no archetype id, which the walk refuses,
//     and the optional inner SECTION: the top-up passes over the entry and
//     takes the SECTION;
//   - the same with a node that names no RM type, or the abstract ENTRY, in
//     place of the ADMIN_ENTRY, which the walk builds as an OBSERVATION and
//     refuses, and an untyped ELEMENT, built as one, in place of the ELEMENT
//     whose feeder_audit the template requires: the census reads each as
//     the class the walk builds;
//   - the items of a required LOCATABLE, which the walk builds as a
//     CLUSTER, whose items the BMM marks mandatory, at existence 1..1, or
//     with no existence, over an optional CLUSTER whose items admit only a
//     slot: the census reads the list as the BMM declares it on CLUSTER,
//     so the top-up asks for a member it takes from that CLUSTER.
//   - a CLUSTER's items over an optional CLUSTER whose items admit only a
//     slot and, after it, an optional LOCATABLE, or CLUSTER, whose items
//     admit only a slot at existence 0..0: the BMM marks the items of the
//     CLUSTER the walk builds mandatory, which wins over the prohibition,
//     so both children need undescribed content, and the top-up takes the
//     first; the census reads the later one as needing it too.
//
// Read beside a template that leaves the SECTION's items optional, where
// no top-up runs, the first root holds a violation. So does a root
// generated from a CLUSTER's items at cardinality 1..* over a required
// ELEMENT and a required CLUSTER whose items admit only a slot, read beside
// the same template with that CLUSTER optional: the ELEMENT's own member
// meets the cardinality lower bound, so no top-up runs, although every
// child before the CLUSTER is full.
func TestREQ107_CensusLastResortFollowsTheGenerator(t *testing.T) {
	slotItems := func(nodeID string) string {
		return censusItems(`<children xsi:type="ARCHETYPE_SLOT"><rm_type_name>CLUSTER</rm_type_name><node_id>` + nodeID + `1</node_id></children>`)
	}
	slotCluster := func(nodeID string) string {
		return censusOptional("CLUSTER", nodeID, slotItems(nodeID))
	}
	feederAuditElement := censusOptional("ELEMENT", "at0003", `<attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>feeder_audit</rm_attribute_name>`+
		`<existence><lower_included>true</lower_included><upper_included>true</upper_included>`+
		`<lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded><lower>1</lower><upper>1</upper></existence>`+
		`<children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>FEEDER_AUDIT</rm_type_name><node_id></node_id></children></attributes>`)
	untypedFeederAuditElement := censusOptional("", "at0003", `<attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>feeder_audit</rm_attribute_name>`+
		`<existence><lower_included>true</lower_included><upper_included>true</upper_included>`+
		`<lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded><lower>1</lower><upper>1</upper></existence>`+
		`<children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>FEEDER_AUDIT</rm_type_name><node_id></node_id></children></attributes>`)
	prohibitedSlotItems := `<attributes xsi:type="C_MULTIPLE_ATTRIBUTE"><rm_attribute_name>items</rm_attribute_name>` +
		`<existence><lower_included>true</lower_included><upper_included>true</upper_included>` +
		`<lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded><lower>0</lower><upper>0</upper></existence>` +
		`<children xsi:type="ARCHETYPE_SLOT"><rm_type_name>CLUSTER</rm_type_name><node_id>at0004</node_id></children>` +
		`<cardinality><is_ordered>false</is_ordered><is_unique>false</is_unique><interval>` +
		`<lower_included>true</lower_included><lower_unbounded>false</lower_unbounded>` +
		`<upper_unbounded>true</upper_unbounded><lower>0</lower></interval></cardinality></attributes>`
	innerSection := censusOptional("SECTION", "at0002", censusList("items", 1, 0,
		`<children xsi:type="ARCHETYPE_SLOT"><rm_type_name>SECTION</rm_type_name><node_id>at0003</node_id></children>`))
	sectionRoot := censusRootOPT("SECTION", censusList("items", 1, 0, innerSection))
	cases := []struct {
		name      string
		generated string
		against   string
		violation bool
	}{
		{name: "a SECTION's items at existence 1..1", generated: sectionRoot, against: sectionRoot},
		{
			name:      "a SECTION's items read as optional",
			generated: sectionRoot,
			against:   censusRootOPT("SECTION", censusList("items", 0, 0, innerSection)),
			violation: true,
		},
		{
			name: "a full ELEMENT before the CLUSTER",
			generated: censusRootOPT("CLUSTER", censusList("items", -1, 2,
				censusOptional("ELEMENT", "at0001"), slotCluster("at0002"))),
		},
		{
			name:      "an ELEMENT whose feeder_audit the template requires after the CLUSTER",
			generated: censusRootOPT("CLUSTER", censusList("items", -1, 1, slotCluster("at0002"), feederAuditElement)),
		},
		{
			name:      "an ELEMENT whose feeder_audit the template requires before the CLUSTER",
			generated: censusRootOPT("CLUSTER", censusList("items", -1, 1, feederAuditElement, slotCluster("at0002"))),
		},
		{
			name: "an inline ADMIN_ENTRY before a SECTION",
			generated: censusRootOPT("SECTION", censusList("items", 1, 0,
				censusOptional("ADMIN_ENTRY", "at0004", `<attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>data</rm_attribute_name>`+
					`<children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>ITEM_TREE</rm_type_name><node_id>at0005</node_id></children></attributes>`),
				innerSection)),
		},
		{
			name:      "an untyped ELEMENT whose feeder_audit the template requires before the CLUSTER",
			generated: censusRootOPT("CLUSTER", censusList("items", -1, 1, untypedFeederAuditElement, slotCluster("at0002"))),
		},
		{
			name:      "an untyped inline item before a SECTION",
			generated: censusRootOPT("SECTION", censusList("items", 1, 0, censusOptional("", "at0004"), innerSection)),
		},
		{
			name: "the items of a LOCATABLE, built as a CLUSTER, at existence 1..1",
			generated: censusRootOPT("CLUSTER", censusList("items", -1, 1,
				censusOccurring("LOCATABLE", "at0001", 1, censusList("items", 1, 0, slotCluster("at0002"))))),
		},
		{
			name: "the items of a LOCATABLE, built as a CLUSTER, with no existence",
			generated: censusRootOPT("CLUSTER", censusList("items", -1, 1,
				censusOccurring("LOCATABLE", "at0001", 1, censusList("items", -1, 0, slotCluster("at0002"))))),
		},
		{
			name: "a LOCATABLE whose items admit only a slot at existence 0..0 after the CLUSTER",
			generated: censusRootOPT("CLUSTER", censusList("items", -1, 1,
				slotCluster("at0002"), censusOptional("LOCATABLE", "at0003", prohibitedSlotItems))),
		},
		{
			name: "a CLUSTER whose items admit only a slot at existence 0..0 after the CLUSTER",
			generated: censusRootOPT("CLUSTER", censusList("items", -1, 1,
				slotCluster("at0002"), censusOptional("CLUSTER", "at0003", prohibitedSlotItems))),
		},
		{
			name:      "an inline ENTRY before a SECTION",
			generated: censusRootOPT("SECTION", censusList("items", 1, 0, censusOptional("ENTRY", "at0004"), innerSection)),
		},
		{
			name: "a required CLUSTER after a required ELEMENT read as optional",
			generated: censusRootOPT("CLUSTER", censusList("items", -1, 1,
				censusOccurring("ELEMENT", "at0001", 1), censusOccurring("CLUSTER", "at0002", 1, slotItems("at0002")))),
			against: censusRootOPT("CLUSTER", censusList("items", -1, 1,
				censusOccurring("ELEMENT", "at0001", 1), slotCluster("at0002"))),
			violation: true,
		},
	}
	for _, tc := range cases {
		against := tc.against
		if against == "" {
			against = tc.generated
		}
		generated := compileCensusOPT(t, tc.generated)
		for _, policy := range []instance.Policy{instance.Large, instance.Medium} {
			t.Run(fmt.Sprintf("%s/%s", tc.name, policy), func(t *testing.T) {
				out, err := instance.Generate(t.Context(), generated, instance.Options{Policy: policy, Language: "en", Now: ratchetNow})
				if err != nil {
					t.Fatalf("Generate: %v", err)
				}
				findings, err := undescribedMembers(out, compileCensusOPT(t, against))
				if err != nil {
					t.Fatalf("undescribedMembers: %v", err)
				}
				if len(findings) != 1 {
					t.Fatalf("undescribedMembers found %d members, want the one slot fill: %+v", len(findings), findings)
				}
				f := findings[0]
				if f.member.kind != "slot" || !f.underOptional {
					t.Errorf("finding %+v, want a slot fill below an optional node", f)
				}
				if f.holderOptional != tc.violation {
					t.Errorf("finding %s: holderOptional = %t, want %t", f.member.line(), f.holderOptional, tc.violation)
				}
			})
		}
	}
}

// TestREQ107_CensusSubstitutesFollowTheGenerator pins the census's table of
// the concrete class the generator builds for an abstract LOCATABLE class
// (censusSubstitutes) against the generator: a root generated under Large
// from a template that names the abstract class holds a member of the class
// the table gives for it, where the walk builds that member.
func TestREQ107_CensusSubstitutesFollowTheGenerator(t *testing.T) {
	archetypeRoot := func(rmType string) string {
		return `<children xsi:type="C_ARCHETYPE_ROOT"><rm_type_name>` + rmType + `</rm_type_name>` +
			`<node_id>at0000</node_id><archetype_id><value>openEHR-EHR-OBSERVATION.census.v1</value></archetype_id></children>`
	}
	event := func(rmType string, attrs ...string) string {
		return censusRootOPT("HISTORY", censusList("events", 1, 1, censusOccurring(rmType, "at0001", 1, attrs...)))
	}
	cases := []struct {
		abstract string
		opt      string
		// path leads from the root to the member built for the node that
		// names the abstract class.
		path []string
	}{
		{abstract: "EVENT", opt: event("EVENT"), path: []string{"events"}},
		{
			abstract: "ITEM_STRUCTURE",
			opt: event("POINT_EVENT", `<attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>data</rm_attribute_name>`+
				censusOccurring("ITEM_STRUCTURE", "at0002", 1)+`</attributes>`),
			path: []string{"events", "data"},
		},
		{abstract: "ITEM", opt: censusRootOPT("CLUSTER", censusList("items", 1, 1, censusOccurring("ITEM", "at0001", 1))), path: []string{"items"}},
		{abstract: "CONTENT_ITEM", opt: censusRootOPT("SECTION", censusList("items", 1, 1, archetypeRoot("CONTENT_ITEM"))), path: []string{"items"}},
		{abstract: "CARE_ENTRY", opt: censusRootOPT("SECTION", censusList("items", 1, 1, archetypeRoot("CARE_ENTRY"))), path: []string{"items"}},
		{abstract: "ENTRY", opt: censusRootOPT("SECTION", censusList("items", 1, 1, archetypeRoot("ENTRY"))), path: []string{"items"}},
		{
			abstract: "LOCATABLE",
			opt: censusRootOPT("CLUSTER", censusList("items", 1, 1,
				censusOccurring("LOCATABLE", "at0001", 1, censusList("items", 1, 1, censusOccurring("ELEMENT", "at0002", 1))))),
			path: []string{"items"},
		},
	}
	if len(cases) != len(censusSubstitutes) {
		t.Fatalf("%d cases for %d substitutes, want one each", len(cases), len(censusSubstitutes))
	}
	for _, tc := range cases {
		t.Run(tc.abstract, func(t *testing.T) {
			want, ok := censusSubstitutes[tc.abstract]
			if !ok {
				t.Fatalf("censusSubstitutes names no substitute for %s", tc.abstract)
			}
			out, err := instance.Generate(t.Context(), compileCensusOPT(t, tc.opt), instance.Options{Policy: instance.Large, Language: "en", Now: ratchetNow})
			if err != nil {
				t.Fatalf("Generate: %v", err)
			}
			raw, err := canjson.Marshal(out)
			if err != nil {
				t.Fatalf("canjson.Marshal: %v", err)
			}
			var member map[string]any
			if err := jsonv2.Unmarshal(raw, &member); err != nil {
				t.Fatalf("json unmarshal: %v", err)
			}
			for _, key := range tc.path {
				members := objects(member[key])
				if len(members) == 0 {
					t.Fatalf("the root holds no member at %s", strings.Join(tc.path, "/"))
				}
				member = members[0]
			}
			if got, _ := member["_type"].(string); got != want {
				t.Errorf("a node of %s is built as %s, the census reads it as %s", tc.abstract, got, want)
			}
		})
	}
}

// TestREQ107_CensusReadsAListAsTheBuiltClassDeclaresIt checks that the
// census reads whether a child is optional (nodeOptional) on the class the
// walk builds its parent as: an ELEMENT at occurrences 1..1 in the items of
// a LOCATABLE, which the walk builds as a CLUSTER, whose items the BMM marks
// mandatory, is required, as it is in the items of a CLUSTER, although the
// template states no existence or cardinality lower bound for them.
func TestREQ107_CensusReadsAListAsTheBuiltClassDeclaresIt(t *testing.T) {
	for _, class := range []string{"LOCATABLE", "CLUSTER"} {
		t.Run(class, func(t *testing.T) {
			c := compileCensusOPT(t, censusRootOPT("CLUSTER", censusList("items", -1, 1,
				censusOccurring(class, "at0001", 1, censusList("items", -1, 0, censusOccurring("ELEMENT", "at0002", 1))))))
			node, err := c.NodeAt("/items[at0001]/items[at0002]")
			if err != nil {
				t.Fatalf("NodeAt: %v", err)
			}
			if nodeOptional(node) {
				t.Errorf("nodeOptional(%s) = true, want false: the items of a %s, built as a CLUSTER, are mandatory", holderPath(node), class)
			}
		})
	}
}
