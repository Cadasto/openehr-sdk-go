package instance_test

import (
	"fmt"
	"maps"
	mrand "math/rand/v2"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// existenceOnlyShape renders what the walk built at the template root: the
// archetype node ids of an ITEM_LIST's items, or what undescribedShape
// renders for any other root.
func existenceOnlyShape(out any) string {
	if l, ok := out.(*rm.ItemList); ok {
		ids := make([]string, len(l.Items))
		for i, e := range l.Items {
			ids[i] = e.ArchetypeNodeID
		}
		return strings.Join(ids, ",")
	}
	return undescribedShape(out)
}

// TestREQ107_ExistenceOnlyItemsGetNoUndescribedMember is the REQ-107 check
// of What a visit writes and § Slot fills for a list only its existence
// asks a member of: an existence of 1..1 on items the BMM does not mark
// mandatory, at a cardinality of 0..*. The top-up takes only a member of
// the first allowed child that needs no undescribed content and whose
// identity no earlier sibling carries, and builds nothing where there is
// none, so an ITEM_TREE or ITEM_LIST whose items have no child, only
// slots, or only a child that needs undescribed content gets no items.
// The controls keep the top-up they had: an optional described child
// still gives the one member, and a list whose cardinality lower bound is
// 1, or a CLUSTER's items, which the BMM marks mandatory, is still topped
// up with a slot fill or a member built from its BMM type. Every root
// passes the RM floor and the template validator, under every policy, both
// value fills and both compile modes.
func TestREQ107_ExistenceOnlyItemsGetNoUndescribedMember(t *testing.T) {
	const stub = "openEHR-EHR-CLUSTER.example.v1"
	element := optOccurring("C_COMPLEX_OBJECT", "ELEMENT", "at0002", 0, 1)
	cases := []struct {
		name string
		opt  string
		want string // the shape (existenceOnlyShape) under every policy
	}{
		{
			name: "ITEM_TREE items with no child",
			opt:  optTemplate("ITEM_TREE", guardMultiple("items", guardExistence11)),
		},
		{
			name: "ITEM_LIST items with no child",
			opt:  optTemplate("ITEM_LIST", guardMultiple("items", guardExistence11)),
		},
		{
			name: "ITEM_TREE items with only a slot",
			opt:  optTemplate("ITEM_TREE", guardMultiple("items", guardExistence11, optSlot("CLUSTER", "at0001"))),
		},
		{
			name: "ITEM_TREE items with only a slot that includes a device cluster",
			opt: optTemplate("ITEM_TREE", guardMultiple("items", guardExistence11,
				optSlotIncluding("CLUSTER", "at0001", `openEHR-EHR-CLUSTER\.device\.v1`))),
		},
		{
			name: "ITEM_TREE items whose one child needs undescribed content",
			opt: optTemplate("ITEM_TREE", guardMultiple("items", guardExistence11,
				slotOnlyCluster("at0003", guardOccurrences01))),
		},
		{
			name: "control: ITEM_TREE items with an optional described ELEMENT",
			opt:  optTemplate("ITEM_TREE", guardMultiple("items", guardExistence11, element)),
			want: "at0002",
		},
		{
			name: "control: ITEM_TREE items with a slot before an optional described ELEMENT",
			opt:  optTemplate("ITEM_TREE", guardMultiple("items", guardExistence11, optSlot("CLUSTER", "at0001"), element)),
			want: "at0002",
		},
		{
			name: "control: ITEM_TREE items at cardinality 1..* with no child",
			opt:  optTemplate("ITEM_TREE", optRequiredMultiple("items")),
			want: "at0000",
		},
		{
			name: "control: ITEM_TREE items at cardinality 1..* with only a slot",
			opt:  optTemplate("ITEM_TREE", guardCountedMultiple("items", guardExistence11, optSlot("CLUSTER", "at0001"))),
			want: stub + "(at0000)",
		},
		{
			name: "control: CLUSTER items, which the BMM requires, with only a slot",
			opt:  optTemplate("CLUSTER", guardMultiple("items", guardExistence11, optSlot("CLUSTER", "at0001"))),
			want: stub + "(at0000)",
		},
		{
			name: "control: CLUSTER items, which the BMM requires, with no child",
			opt:  optTemplate("CLUSTER", guardMultiple("items", guardExistence11)),
			want: "at0000",
		},
	}
	for _, tc := range cases {
		for _, implicit := range []bool{true, false} {
			c := compileOPTText(t, tc.opt, implicit)
			for _, opts := range guardOptions() {
				t.Run(fmt.Sprintf("%s/implicit=%t/%v/%v", tc.name, implicit, opts.Policy, opts.ValueFill), func(t *testing.T) {
					out, err := instance.Generate(t.Context(), c, opts)
					if err != nil {
						t.Fatalf("Generate: %v", err)
					}
					if got := existenceOnlyShape(out); got != tc.want {
						t.Errorf("Generate built %q, want %q", got, tc.want)
					}
					noFloorErrors(t, out)
					for _, iss := range templateErrors(out, c) {
						t.Errorf("template validator: %s @ %s: %s", iss.Code, iss.Path, iss.Detail)
					}
				})
			}
		}
	}
}

// TestREQ107_VitalSignsStateAndProtocolHoldNoUndescribedItems is the
// REQ-107 check of What a visit writes on vital_signs (issue 295). Each
// OBSERVATION requires its protocol, and each event its state (existence
// 1..1), and the template gives their items an existence of 1..1 at a
// cardinality of 0..* and no child, or only slots. Only that existence
// asks for a member, so under Large and Medium, both value fills and
// several seeds, the root holds no LOCATABLE with the archetype node id
// at0000 and no slot fill below a protocol or a state; the blood_pressure
// event's state is the ITEM_LIST at0007 and its protocol the ITEM_TREE
// at0011, both with no items. Every root passes the template validator and
// the RM floor.
func TestREQ107_VitalSignsStateAndProtocolHoldNoUndescribedItems(t *testing.T) {
	const bloodPressure = "openEHR-EHR-OBSERVATION.blood_pressure.v1"
	c := compileFixture(t, "vital_signs")
	for _, policy := range largeAndMedium {
		for _, fill := range []instance.ValueFill{instance.ExampleFill, instance.RandomFill} {
			seeds := []uint64{1, 2, 3}
			if fill == instance.ExampleFill {
				seeds = seeds[:1]
			}
			for _, seed := range seeds {
				call := fmt.Sprintf("Generate(vital_signs, %v, %v, seed %d)", policy, fill, seed)
				comp := generateComposition(t, c, instance.Options{
					Policy:      policy,
					ValueFill:   fill,
					ValueSource: mrand.NewPCG(seed, 20261021),
					Territory:   "NL",
					Composer:    testComposer(),
					Now:         defaultsNow,
				})
				tree, _ := decodedJSON(t, comp).(map[string]any)
				for _, p := range nodeIDPaths(tree, "", "at0000") {
					t.Errorf("%s holds a LOCATABLE with archetype node id at0000 at %s, want none", call, p)
				}
				content := jsonList(tree["content"])
				if !slices.ContainsFunc(content, func(obs map[string]any) bool { return obs["archetype_node_id"] == bloodPressure }) {
					t.Errorf("%s holds no %s, want one", call, bloodPressure)
				}
				for i, obs := range content {
					where := fmt.Sprintf("/content[%d]", i)
					for _, p := range archetypedPaths(obs["protocol"], where+"/protocol") {
						t.Errorf("%s holds a slot fill at %s, want none below a protocol", call, p)
					}
					data, _ := obs["data"].(map[string]any)
					events := jsonList(data["events"])
					for j, ev := range events {
						for _, p := range archetypedPaths(ev["state"], fmt.Sprintf("%s/data/events[%d]/state", where, j)) {
							t.Errorf("%s holds a slot fill at %s, want none below a state", call, p)
						}
					}
					if obs["archetype_node_id"] != bloodPressure {
						continue
					}
					if !emptyStructure(obs["protocol"], "ITEM_TREE", "at0011") {
						t.Errorf("%s %s/protocol = %v, want the ITEM_TREE at0011 with no items", call, where, obs["protocol"])
					}
					if len(events) == 0 {
						t.Errorf("%s %s has no event, want one whose state is the ITEM_LIST at0007", call, where)
					}
					for j, ev := range events {
						if !emptyStructure(ev["state"], "ITEM_LIST", "at0007") {
							t.Errorf("%s %s/data/events[%d]/state = %v, want the ITEM_LIST at0007 with no items", call, where, j, ev["state"])
						}
					}
				}
				if res := validation.ValidateComposition(comp, c); !res.OK {
					t.Errorf("ValidateComposition(%s) OK = false: %v", call, res.Issues)
				}
				if res := validation.ValidateRM(comp); !res.OK {
					t.Errorf("ValidateRM(%s) OK = false: %v", call, res.Issues)
				}
			}
		}
	}
}

// jsonList returns the objects of v, a decoded JSON array, or nil when v
// is not one.
func jsonList(v any) []map[string]any {
	arr, _ := v.([]any)
	var out []map[string]any
	for _, e := range arr {
		if m, ok := e.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

// emptyStructure reports whether v, a decoded JSON value, is an object of
// rmType with the archetype node id nodeID and no items.
func emptyStructure(v any, rmType, nodeID string) bool {
	m, ok := v.(map[string]any)
	if !ok || m["_type"] != rmType || m["archetype_node_id"] != nodeID {
		return false
	}
	_, hasItems := m["items"]
	return !hasItems
}

// nodeIDPaths returns the JSON path of every object in v, a decoded JSON
// tree at path, whose archetype_node_id is id.
func nodeIDPaths(v any, path, id string) []string {
	var out []string
	switch node := v.(type) {
	case map[string]any:
		if node["archetype_node_id"] == id {
			out = append(out, path)
		}
		for _, k := range slices.Sorted(maps.Keys(node)) {
			out = append(out, nodeIDPaths(node[k], path+"/"+k, id)...)
		}
	case []any:
		for i, child := range node {
			out = append(out, nodeIDPaths(child, fmt.Sprintf("%s[%d]", path, i), id)...)
		}
	}
	return out
}

// archetypedPaths returns the JSON path of every object in v, a decoded
// JSON tree at path, that carries archetype_details.
func archetypedPaths(v any, path string) []string {
	var out []string
	switch node := v.(type) {
	case map[string]any:
		if _, ok := node["archetype_details"]; ok {
			out = append(out, path)
		}
		for _, k := range slices.Sorted(maps.Keys(node)) {
			out = append(out, archetypedPaths(node[k], path+"/"+k)...)
		}
	case []any:
		for i, child := range node {
			out = append(out, archetypedPaths(child, fmt.Sprintf("%s[%d]", path, i))...)
		}
	}
	return out
}
