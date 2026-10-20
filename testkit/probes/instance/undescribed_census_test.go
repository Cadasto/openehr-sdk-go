package instanceprobes_test

import (
	jsonv2 "encoding/json/v2"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
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
// holds an undescribed member in an attribute of its own, and the
// undescribed members below an optional node are exactly the ones
// undescribedBelowRequired lists. Required builds no optional node of its
// own, so it is not read here.
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
		t.Errorf("an optional node holds only undescribed content in an attribute of its own:\n  %s", strings.Join(violations, "\n  "))
	}
	if len(unlisted) > 0 {
		t.Errorf("undescribed members below an optional node, not in undescribedBelowRequired:\n  %s", strings.Join(unlisted, "\n  "))
	}
	if len(stale) > 0 {
		t.Errorf("undescribedBelowRequired rows no longer generated:\n  %s", strings.Join(stale, "\n  "))
	}
}

// censusFinding is an undescribed member with where it sits: holderOptional
// when the node that holds it is optional, so the walk built that node
// though nothing required it, and underOptional when that node or one
// above it is.
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
				holderOptional := node != nil && nodeOptional(node)
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
// requires it: a child of a multi-valued attribute whose occurrences lower
// bound is 0, or the child of a single-valued attribute that is not
// required, by the BMM or by an existence lower bound of at least 1. The
// template root is required.
func nodeOptional(node *templatecompile.CompiledNode) bool {
	parent := node.Parent()
	if parent == nil {
		return false
	}
	for _, attr := range parent.Attributes() {
		if !slices.Contains(attr.Children(), node) {
			continue
		}
		if !attr.Required() && !lowerAtLeastOne(attr.Existence()) && !lowerAtLeastOne(attr.ChildMultiplicity()) {
			return true
		}
		if attr.Cardinality() == template.Single {
			return false
		}
		occ := node.Occurrences()
		return occ != nil && !occ.LowerUnbounded() && occ.Lower() == 0
	}
	return false
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
