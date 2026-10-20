package instanceprobes_test

import (
	jsonv2 "encoding/json/v2"
	"fmt"
	mrand "math/rand/v2"
	"slices"
	"strings"
	"testing"

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
// what § Contract says, undescribed members included. Medium builds only
// the first optional archetype root of a content attribute, so it reaches
// vital_signs' blood_pressure and not clinical_content_validation's
// EVALUATION. A new such member, or a listed one that is gone, fails the
// census.
var undescribedBelowRequired = []undescribedPin{
	{
		member: undescribedMember{
			template: "templates/clinical_content_validation", policy: "large",
			attr: "/content[openEHR-EHR-EVALUATION.validation_evaliation_test.v0,'Validation evaluation wth single']/data/item",
			kind: "bmm", ident: "ELEMENT",
		},
		reason: "the RM requires the EVALUATION's data, which takes the ITEM_SINGLE at0001, and its item, which the template leaves without a child",
	},
	{
		member: undescribedMember{
			template: "templates/vital_signs", policy: "large",
			attr: "/content[openEHR-EHR-OBSERVATION.blood_pressure.v1]/data/events[at0006]/state/items",
			kind: "bmm", ident: "ELEMENT",
		},
		reason: vitalSignsState("ITEM_LIST at0007"),
	},
	{
		member: undescribedMember{
			template: "templates/vital_signs", policy: "medium",
			attr: "/content[openEHR-EHR-OBSERVATION.blood_pressure.v1]/data/events[at0006]/state/items",
			kind: "bmm", ident: "ELEMENT",
		},
		reason: vitalSignsState("ITEM_LIST at0007"),
	},
	{
		member: undescribedMember{
			template: "templates/vital_signs", policy: "large",
			attr: "/content[openEHR-EHR-OBSERVATION.blood_pressure.v1]/protocol/items",
			kind: "slot", ident: "openEHR-EHR-CLUSTER.device.v1",
		},
		reason: vitalSignsProtocolSlot("ITEM_TREE at0011"),
	},
	{
		member: undescribedMember{
			template: "templates/vital_signs", policy: "medium",
			attr: "/content[openEHR-EHR-OBSERVATION.blood_pressure.v1]/protocol/items",
			kind: "slot", ident: "openEHR-EHR-CLUSTER.device.v1",
		},
		reason: vitalSignsProtocolSlot("ITEM_TREE at0011"),
	},
	{
		member: undescribedMember{
			template: "templates/vital_signs", policy: "large",
			attr: "/content[openEHR-EHR-OBSERVATION.body_temperature.v1]/data/events[at0003]/state/items",
			kind: "slot", ident: "openEHR-EHR-CLUSTER.environmental_conditions.v1",
		},
		reason: "the template requires the event's state and the items of its ITEM_TREE at0029 (existence 1..1), which admit only slots",
	},
	{
		member: undescribedMember{
			template: "templates/vital_signs", policy: "large",
			attr: "/content[openEHR-EHR-OBSERVATION.body_temperature.v1]/protocol/items",
			kind: "slot", ident: "openEHR-EHR-CLUSTER.device.v1",
		},
		reason: vitalSignsProtocolSlot("ITEM_TREE at0020"),
	},
	{
		member: undescribedMember{
			template: "templates/vital_signs", policy: "large",
			attr: "/content[openEHR-EHR-OBSERVATION.heart_rate.v1]/data/events[at0003]/state/items",
			kind: "bmm", ident: "ELEMENT",
		},
		reason: vitalSignsState("ITEM_LIST at0012"),
	},
	{
		member: undescribedMember{
			template: "templates/vital_signs", policy: "large",
			attr: "/content[openEHR-EHR-OBSERVATION.heart_rate.v1]/protocol/items",
			kind: "bmm", ident: "ELEMENT",
		},
		reason: "the template requires the protocol (existence 1..1) and the items of its ITEM_LIST at0010 (existence 1..1), and names no child of them",
	},
	{
		member: undescribedMember{
			template: "templates/vital_signs", policy: "large",
			attr: "/content[openEHR-EHR-OBSERVATION.respiration.v1]/data/events[at0002]/state/items",
			kind: "bmm", ident: "ELEMENT",
		},
		reason: vitalSignsState("ITEM_LIST at0022"),
	},
}

// vitalSignsState is the reason for a placeholder in a vital_signs event's
// state, which the template requires with list.
func vitalSignsState(list string) string {
	return "the template requires the event's state (existence 1..1) and the items of its " + list +
		" (existence 1..1), and names no child of them"
}

// vitalSignsProtocolSlot is the reason for a slot fill in a vital_signs
// protocol, which the template requires with tree.
func vitalSignsProtocolSlot(tree string) string {
	return "the template requires the protocol (existence 1..1) and the items of its " + tree +
		" (existence 1..1), which admit only a slot"
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
	return `<children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>` + rmType + `</rm_type_name>` +
		`<occurrences><lower_included>true</lower_included><upper_included>true</upper_included>` +
		`<lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded>` +
		`<lower>0</lower><upper>1</upper></occurrences>` +
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
	censusWalk(compiled.Root(), tree, false, &out)
	return out, nil
}

// censusWalk visits the members of every attribute of value, the JSON
// object bound to the template node node. underOptional reports whether
// node or a node above it is optional.
func censusWalk(node *templatecompile.CompiledNode, value map[string]any, underOptional bool, out *[]censusFinding) {
	for _, key := range sortedKeys(value) {
		if key == "_type" || key == "archetype_node_id" || key == "archetype_details" {
			continue
		}
		var attr *templatecompile.CompiledAttribute
		if node != nil {
			attr = node.Attribute(key)
		}
		for _, member := range objects(value[key]) {
			child, kind := censusBind(attr, member)
			if kind != "" {
				ident, _ := member["_type"].(string)
				if kind == "slot" {
					ident, _ = member["archetype_node_id"].(string)
				}
				holderOptional := node != nil && nodeOptional(node) && !topUpLastResort(node)
				*out = append(*out, censusFinding{
					member:         undescribedMember{attr: holderPath(node) + "/" + key, kind: kind, ident: ident},
					holderOptional: holderOptional,
					underOptional:  underOptional,
				})
				continue
			}
			below := underOptional
			if child != nil && nodeOptional(child) {
				below = true
			}
			censusWalk(child, member, below, out)
		}
	}
}

// censusBind binds member, a JSON object in attr, to an allowed template
// child of attr. It returns the child that describes member, or the kind
// of undescribed member it is: "bmm" for a LOCATABLE in an attribute with
// no allowed child, "slot" for one only a slot admits, and "unbound" for
// one no child admits. A member that is not a LOCATABLE binds to the first
// allowed child of its RM type, else the first allowed child, else none,
// and is never undescribed: only a LOCATABLE has a template node of its
// own.
func censusBind(attr *templatecompile.CompiledAttribute, member map[string]any) (*templatecompile.CompiledNode, string) {
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
	slot := false
	for _, c := range children {
		if c.IsSlot() {
			slot = true
			continue
		}
		if c.ArchetypeID() == id || c.NodeID() == id {
			return c, ""
		}
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
	if !attrRequired(attr) {
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

// attrRequired reports whether the walk has to give attr a member: the BMM
// marks it mandatory, or its existence or cardinality lower bound is at
// least 1.
func attrRequired(attr *templatecompile.CompiledAttribute) bool {
	return attr.Required() || lowerAtLeastOne(attr.Existence()) || lowerAtLeastOne(attr.ChildMultiplicity())
}

// topUpLastResort reports whether node, an optional child, is the child a
// top-up takes when no other will do (REQ-107 § Contract What a visit
// writes): its attribute is multi-valued and required, so a top-up builds
// a member where no child got one of its own, node is the first allowed
// child that is not a slot, and no allowed child that is not a slot needs
// no undescribed content (needsNoUndescribed) and has an identity no
// earlier sibling carries. A member built so is not one of node's own,
// which § Slot fills keeps out, so the census lists what it holds rather
// than calling it a violation.
func topUpLastResort(node *templatecompile.CompiledNode) bool {
	attr := parentAttribute(node)
	if attr == nil || attr.Cardinality() != template.Multiple || !attrRequired(attr) {
		return false
	}
	children := allowedChildren(attr)
	first := slices.IndexFunc(children, func(c *templatecompile.CompiledNode) bool { return !c.IsSlot() })
	if first < 0 || children[first] != node {
		return false
	}
	for i, c := range children {
		if needsNoUndescribed(c) && !identityCarriedBefore(children, i) {
			return false
		}
	}
	return true
}

// needsNoUndescribed reports whether node needs no undescribed content, as
// § Slot fills defines it: node is not a slot, and no attribute of its own
// that is required, and that the template does not prohibit, can be given
// only undescribed members. Such an attribute has no allowed child and an
// element type that is a LOCATABLE, or allows only slots. The census
// compiles with the implicit attributes, so node carries every attribute
// the BMM marks mandatory. A single-valued attribute with an allowed child
// that is not a slot counts as one that can be given a described member,
// although the walk builds the alternative the validator binds, which can
// be a slot: where the census cannot tell, it reads a sibling as needing
// none, so it reports a violation rather than grant an exemption.
func needsNoUndescribed(node *templatecompile.CompiledNode) bool {
	if node.IsSlot() {
		return false
	}
	for _, attr := range node.Attributes() {
		if !attrRequired(attr) || attrProhibited(attr) {
			continue
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

// attrProhibited reports whether the template prohibits attr, which the
// BMM does not mark mandatory: an existence upper bound of 0, or children
// none of which it allows. The walk does not visit such an attribute.
func attrProhibited(attr *templatecompile.CompiledAttribute) bool {
	if attr.Required() {
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

// identityCarriedBefore reports whether a sibling before children[i], other
// than a slot, carries its identity: its archetype id when it is an
// archetype root, its node id otherwise. The validator binds a member to
// the first child whose archetype id or node id it carries. A child with
// no identity is never carried.
func identityCarriedBefore(children []*templatecompile.CompiledNode, i int) bool {
	id := children[i].ArchetypeID()
	if id == "" {
		id = children[i].NodeID()
	}
	if id == "" {
		return false
	}
	return slices.ContainsFunc(children[:i], func(sib *templatecompile.CompiledNode) bool {
		return !sib.IsSlot() && (sib.ArchetypeID() == id || sib.NodeID() == id)
	})
}

func lowerAtLeastOne(m *template.Multiplicity) bool {
	return m != nil && !m.LowerUnbounded() && m.Lower() >= 1
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
