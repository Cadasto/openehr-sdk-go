package instance_test

import (
	"fmt"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
)

// optSlotOnlyTree is an ITEM_TREE with the node id nodeID whose items, at
// cardinality 0..*, hold one optional CLUSTER slot alone.
func optSlotOnlyTree(nodeID string) string {
	return optNode("ITEM_TREE", nodeID, optCardinal("items", 0, -1, optSlot("CLUSTER", nodeID+".1")))
}

// optStructureObservation is an OBSERVATION template whose protocol is the
// attribute protocol, and whose one event, which its HISTORY requires,
// holds an ITEM_TREE data with no items attribute, which the BMM requires
// although the OPT gives it an existence of 0..1, and an optional state
// whose ITEM_TREE holds one optional slot alone.
func optStructureObservation(protocol string) string {
	event := optOccurring("C_COMPLEX_OBJECT", "POINT_EVENT", "at0002", 1, 1,
		optOptionalSingleOver("data", optNode("ITEM_TREE", "at0003")),
		optOptionalSingleOver("state", optSlotOnlyTree("at0004")),
	)
	history := optNode("HISTORY", "at0001", optCardinal("events", 0, -1, event))
	return optTemplate("OBSERVATION", optSingle("data", history), protocol)
}

// jsonObject returns v, a decoded JSON value, as an object, or nil.
func jsonObject(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

// structureItems returns the number of members of the item structure
// object s holds: its rows for an ITEM_TABLE, its items otherwise.
func structureItems(s map[string]any) int {
	key := "items"
	if s["_type"] == "ITEM_TABLE" {
		key = "rows"
	}
	list, _ := s[key].([]any)
	return len(list)
}

// TestREQ107_OptionalItemStructureWithNoItemIsLeftOut is the REQ-107 check
// of the Visit rule that a single-valued attribute the template does not
// require is left unvisited where the item structure a visit would write
// would hold no item, decided by what the walk writes and not by the OPT
// alone. An optional protocol whose ITEM_TREE names no items, or holds an
// optional slot alone, whose ITEM_LIST holds a slot alone, whose ITEM_TABLE
// names no rows, or whose ITEM_TREE holds an optional CLUSTER that needs
// undescribed content, is left out under every policy, and so is an
// event's optional state whose ITEM_TREE holds an optional slot alone. A
// protocol whose ITEM_TREE holds an optional ELEMENT is written with that
// ELEMENT under Large and Medium, and is left out under Required, which
// visits no optional attribute. A protocol the OPT requires, and an event's
// data, which the BMM requires, are written with no items. Every root
// passes the template validator and the RM floor, under every policy, both
// value fills and both compile modes.
func TestREQ107_OptionalItemStructureWithNoItemIsLeftOut(t *testing.T) {
	cases := []struct {
		name     string
		protocol string
		// items is the number of items the written protocol holds, per
		// policy, and -1 where the protocol is left out.
		items map[instance.Policy]int
	}{
		{
			name:     "an ITEM_TREE with no items attribute",
			protocol: optOptionalSingleOver("protocol", optNode("ITEM_TREE", "at0010")),
			items:    map[instance.Policy]int{instance.Large: -1, instance.Medium: -1, instance.Required: -1},
		},
		{
			name:     "an ITEM_TREE with an optional slot alone",
			protocol: optOptionalSingleOver("protocol", optSlotOnlyTree("at0010")),
			items:    map[instance.Policy]int{instance.Large: -1, instance.Medium: -1, instance.Required: -1},
		},
		{
			name: "an ITEM_LIST with an optional slot alone",
			protocol: optOptionalSingleOver("protocol", optNode("ITEM_LIST", "at0010",
				optCardinal("items", 0, -1, optSlot("ELEMENT", "at0010.1")))),
			items: map[instance.Policy]int{instance.Large: -1, instance.Medium: -1, instance.Required: -1},
		},
		{
			name:     "an ITEM_TABLE with no rows attribute",
			protocol: optOptionalSingleOver("protocol", optNode("ITEM_TABLE", "at0010")),
			items:    map[instance.Policy]int{instance.Large: -1, instance.Medium: -1, instance.Required: -1},
		},
		{
			name: "an ITEM_TREE with an optional CLUSTER that needs undescribed content",
			protocol: optOptionalSingleOver("protocol", optNode("ITEM_TREE", "at0010", optCardinal("items", 0, -1,
				optOccurring("C_COMPLEX_OBJECT", "CLUSTER", "at0011", 0, 1, optCardinal("items", 1, -1, optSlot("ELEMENT", "at0012")))))),
			items: map[instance.Policy]int{instance.Large: -1, instance.Medium: -1, instance.Required: -1},
		},
		{
			// The walk visits the items, which their existence requires,
			// but gives them no member: the CLUSTER is optional and needs
			// undescribed content, and only that existence asks for a
			// member, so no top-up builds one (What a visit writes).
			name: "an ITEM_TREE whose items only their existence requires, over an optional CLUSTER that needs undescribed content",
			protocol: optOptionalSingleOver("protocol", optNode("ITEM_TREE", "at0010", optList("items", 1, 0,
				optOccurring("C_COMPLEX_OBJECT", "CLUSTER", "at0011", 0, 1, optCardinal("items", 1, -1, optSlot("ELEMENT", "at0012")))))),
			items: map[instance.Policy]int{instance.Large: -1, instance.Medium: -1, instance.Required: -1},
		},
		{
			name: "an ITEM_TREE with an optional ELEMENT",
			protocol: optOptionalSingleOver("protocol", optNode("ITEM_TREE", "at0010",
				optCardinal("items", 0, -1, optElement("at0011", 0, 1)))),
			items: map[instance.Policy]int{instance.Large: 1, instance.Medium: 1, instance.Required: -1},
		},
		{
			name:     "an ITEM_TREE the OPT requires, with an optional slot alone",
			protocol: optSingle("protocol", optSlotOnlyTree("at0010")),
			items:    map[instance.Policy]int{instance.Large: 0, instance.Medium: 0, instance.Required: 0},
		},
	}
	for _, tc := range cases {
		for _, implicit := range []bool{true, false} {
			c := compileOPTText(t, optStructureObservation(tc.protocol), implicit)
			for _, opts := range guardOptions() {
				t.Run(fmt.Sprintf("%s/implicit=%t/%v/%v", tc.name, implicit, opts.Policy, opts.ValueFill), func(t *testing.T) {
					out, err := instance.Generate(t.Context(), c, opts)
					if err != nil {
						t.Fatalf("Generate: %v", err)
					}
					tree := jsonObject(decodedJSON(t, out))
					protocol, present := tree["protocol"]
					switch want := tc.items[opts.Policy]; {
					case want < 0 && present:
						t.Errorf("protocol = %v, want none: it would hold no item", protocol)
					case want >= 0 && !present:
						t.Errorf("protocol absent, want one with %d items", want)
					case want >= 0 && structureItems(jsonObject(protocol)) != want:
						t.Errorf("protocol = %v, want %d items", protocol, want)
					}
					events := jsonList(jsonObject(tree["data"])["events"])
					if len(events) != 1 {
						t.Fatalf("data.events = %v, want the one event the HISTORY requires", jsonObject(tree["data"])["events"])
					}
					if data := jsonObject(events[0]["data"]); data == nil || structureItems(data) != 0 {
						t.Errorf("event data = %v, want the ITEM_TREE at0003 with no items: the BMM requires it", events[0]["data"])
					}
					if state, ok := events[0]["state"]; ok {
						t.Errorf("event state = %v, want none: it would hold no item", state)
					}
					for _, iss := range templateErrors(out, c) {
						t.Errorf("template validator: %s @ %s: %s", iss.Code, iss.Path, iss.Detail)
					}
					noFloorErrors(t, out)
				})
			}
		}
	}
}

// TestREQ107_OptionalOtherContextWithNoItemIsLeftOut is the REQ-107 check
// of the Visit rule on a COMPOSITION: a context's optional other_context
// whose ITEM_TREE holds an optional slot alone is left out under every
// policy, both value fills and both compile modes, while the context, which
// every policy writes, stays. The root passes the template validator and
// the RM floor.
func TestREQ107_OptionalOtherContextWithNoItemIsLeftOut(t *testing.T) {
	opt := optTemplate("COMPOSITION", optSingle("context", optNode("EVENT_CONTEXT", "",
		optOptionalSingleOver("other_context", optSlotOnlyTree("at0001")))))
	for _, implicit := range []bool{true, false} {
		c := compileOPTText(t, opt, implicit)
		for _, opts := range guardOptions() {
			t.Run(fmt.Sprintf("implicit=%t/%v/%v", implicit, opts.Policy, opts.ValueFill), func(t *testing.T) {
				comp := generateComposition(t, c, opts)
				if comp.Context == nil {
					t.Fatal("composition has no context, want the one every policy writes")
				}
				if comp.Context.OtherContext != nil {
					t.Errorf("other_context = %+v, want none: it would hold no item", comp.Context.OtherContext)
				}
				noValidationErrors(t, comp, c)
				noFloorErrors(t, comp)
			})
		}
	}
}
