package instance_test

import (
	"fmt"
	mrand "math/rand/v2"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// itemStructureList reports, for s, an ITEM_TREE, ITEM_LIST or ITEM_TABLE,
// whether the list that holds its items (its rows for an ITEM_TABLE) is
// present, a non-nil slice, and how many members it holds. ok is false for
// any other value.
func itemStructureList(s rm.ItemStructure) (present bool, members int, ok bool) {
	switch v := s.(type) {
	case *rm.ItemTree:
		return v.Items != nil, len(v.Items), true
	case *rm.ItemList:
		return v.Items != nil, len(v.Items), true
	case *rm.ItemTable:
		return v.Rows != nil, len(v.Rows), true
	}
	return false, 0, false
}

// eventState returns the state of e, a POINT_EVENT or an INTERVAL_EVENT, or
// nil for any other value.
func eventState(e rm.Event) rm.ItemStructure {
	switch v := e.(type) {
	case *rm.PointEvent[rm.ItemStructure]:
		return v.State
	case *rm.IntervalEvent[rm.ItemStructure]:
		return v.State
	}
	return nil
}

// jsonEmptyList reports whether the decoded JSON object s carries key as an
// empty array, the canonical JSON of a present empty list.
func jsonEmptyList(s map[string]any, key string) bool {
	v, present := s[key]
	list, isList := v.([]any)
	return present && isList && len(list) == 0
}

// checkPresentEmpty reports where s, the item structure at where, and its
// canonical JSON object js do not carry a present empty list of items: s
// must be an ITEM_TREE, ITEM_LIST or ITEM_TABLE with wantID as its archetype
// node id whose list is a non-nil slice with no member, and js must write
// that list as [].
func checkPresentEmpty(t *testing.T, call, where string, s rm.ItemStructure, js map[string]any, wantID string) {
	t.Helper()
	present, members, ok := itemStructureList(s)
	if !ok {
		t.Errorf("%s %s = %T, want the item structure %s", call, where, s, wantID)
		return
	}
	if got := nodeID(s); got != wantID {
		t.Errorf("%s %s archetype_node_id = %q, want %q", call, where, got, wantID)
	}
	if !present || members != 0 {
		t.Errorf("%s %s holds a list present=%t with %d members, want present and empty: its existence is 1..1", call, where, present, members)
	}
	key := "items"
	if js["_type"] == "ITEM_TABLE" {
		key = "rows"
	}
	if !jsonEmptyList(js, key) {
		t.Errorf("%s %s JSON %q = %v, want []", call, where, key, js[key])
	}
}

// vitalSignsRequiredLists names, per OBSERVATION archetype of the vital_signs
// template, the node id of the protocol and of each event's state that the
// template requires (existence 1..1) and whose items it requires by their
// existence alone (existence 1..1, cardinality 0..*, no child or slots
// only), so the walk gives them no member. "" names none: respiration has no
// protocol. These are the seven containers issue 341 reports.
var vitalSignsRequiredLists = map[string]struct{ protocol, state string }{
	"openEHR-EHR-OBSERVATION.blood_pressure.v1":   {protocol: "at0011", state: "at0007"},
	"openEHR-EHR-OBSERVATION.body_temperature.v1": {protocol: "at0020", state: "at0029"},
	"openEHR-EHR-OBSERVATION.heart_rate.v1":       {protocol: "at0010", state: "at0012"},
	"openEHR-EHR-OBSERVATION.respiration.v1":      {state: "at0022"},
}

// TestREQ107_VitalSignsRequiredItemListsArePresentAndEmpty is the REQ-107
// check of What a visit writes on vital_signs (issue 341): a list the
// template requires by its existence is there even with no member, so each
// protocol and event state that vitalSignsRequiredLists names holds a
// present empty items, which canonical JSON writes as "items":[]. Large
// builds all four OBSERVATIONs, so all seven containers are checked there;
// Medium builds the blood_pressure one; Required builds none, since the
// template leaves its content optional, and
// TestREQ107_RequiredEmptyItemListIsPresent covers that policy. Every root
// passes the template validator and the RM floor, under every policy and
// both value fills.
func TestREQ107_VitalSignsRequiredItemListsArePresentAndEmpty(t *testing.T) {
	c := compileFixture(t, "vital_signs")
	for _, policy := range everyPolicy {
		for _, fill := range []instance.ValueFill{instance.ExampleFill, instance.RandomFill} {
			call := fmt.Sprintf("Generate(vital_signs, %v, %v)", policy, fill)
			comp := generateComposition(t, c, instance.Options{
				Policy:      policy,
				ValueFill:   fill,
				ValueSource: mrand.NewPCG(1, 2),
				Territory:   "NL",
				Composer:    testComposer(),
				Now:         defaultsNow,
			})
			tree := jsonObject(decodedJSON(t, comp))
			content := jsonList(tree["content"])
			if len(content) != len(comp.Content) {
				t.Fatalf("%s JSON content holds %d members, want the %d of the composition", call, len(content), len(comp.Content))
			}
			seen := map[string]bool{}
			for i, item := range comp.Content {
				obs, ok := item.(*rm.Observation)
				if !ok || obs.ArchetypeDetails == nil {
					t.Fatalf("%s content[%d] = %T, want an archetyped OBSERVATION", call, i, item)
				}
				id := obs.ArchetypeDetails.ArchetypeID.Value
				want, known := vitalSignsRequiredLists[id]
				if !known {
					t.Fatalf("%s content[%d] is %s, which the test does not know", call, i, id)
				}
				seen[id] = true
				where := fmt.Sprintf("/content[%d]", i)
				if want.protocol != "" {
					checkPresentEmpty(t, call, where+"/protocol", obs.Protocol, jsonObject(content[i]["protocol"]), want.protocol)
				}
				events := jsonList(jsonObject(content[i]["data"])["events"])
				if len(obs.Data.Events) == 0 || len(events) != len(obs.Data.Events) {
					t.Fatalf("%s %s/data/events: %d in the root, %d in JSON, want the same, at least one", call, where, len(obs.Data.Events), len(events))
				}
				for j, ev := range obs.Data.Events {
					checkPresentEmpty(t, call, fmt.Sprintf("%s/data/events[%d]/state", where, j), eventState(ev), jsonObject(events[j]["state"]), want.state)
				}
			}
			if policy == instance.Large && len(seen) != len(vitalSignsRequiredLists) {
				t.Errorf("%s builds the OBSERVATIONs %v, want all %d", call, seen, len(vitalSignsRequiredLists))
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

// TestREQ107_RequiredEmptyItemListIsPresent is the REQ-107 check of What a
// visit writes on a list the template requires by its existence alone: an
// OBSERVATION's protocol, which the template requires, is an item structure
// whose items (rows for an ITEM_TABLE) have an existence of 1..1 and get no
// member from the walk. Under every policy, both value fills and both
// compile modes, the list is written present and empty, a non-nil slice
// that canonical JSON writes as [], whichever way the walk leaves it
// without a member: no allowed child, slots only, only an optional child
// that needs undescribed content, a cardinality of 0..0 over an optional
// ELEMENT, or an RM list the template writes as a single attribute with no
// child. A list the template leaves optional by its existence stays absent.
// Every root passes the template validator and the RM floor.
func TestREQ107_RequiredEmptyItemListIsPresent(t *testing.T) {
	cases := []struct {
		name      string
		structure string // the protocol's item structure, node id at0010
		// present is whether the list is written; when it is, it holds no
		// member.
		present bool
	}{
		{
			name:      "ITEM_TREE items with no child",
			structure: optNode("ITEM_TREE", "at0010", optList("items", 1, 0)),
			present:   true,
		},
		{
			name:      "ITEM_LIST items with no child",
			structure: optNode("ITEM_LIST", "at0010", optList("items", 1, 0)),
			present:   true,
		},
		{
			name:      "ITEM_TABLE rows with no child",
			structure: optNode("ITEM_TABLE", "at0010", optList("rows", 1, 0)),
			present:   true,
		},
		{
			name:      "ITEM_LIST items with only a slot",
			structure: optNode("ITEM_LIST", "at0010", optList("items", 1, 0, optSlot("ELEMENT", "at0011"))),
			present:   true,
		},
		{
			name:      "ITEM_TREE items with only a slot",
			structure: optNode("ITEM_TREE", "at0010", optList("items", 1, 0, optSlot("CLUSTER", "at0011"))),
			present:   true,
		},
		{
			name: "ITEM_TREE items with only an optional CLUSTER that needs undescribed content",
			structure: optNode("ITEM_TREE", "at0010", optList("items", 1, 0,
				optOccurring("C_COMPLEX_OBJECT", "CLUSTER", "at0011", 0, 1, optCardinal("items", 1, -1, optSlot("ELEMENT", "at0012"))))),
			present: true,
		},
		{
			name: "ITEM_TREE items at cardinality 0..0 over an optional ELEMENT",
			structure: optNode("ITEM_TREE", "at0010", `<attributes xsi:type="C_MULTIPLE_ATTRIBUTE"><rm_attribute_name>items</rm_attribute_name>`+
				`<existence>`+optInterval(1, 1)+`</existence>`+optElement("at0011", 0, 1)+
				`<cardinality><is_ordered>false</is_ordered><is_unique>false</is_unique><interval>`+
				optInterval(0, 0)+`</interval></cardinality></attributes>`),
			present: true,
		},
		{
			name:      "ITEM_TREE items the template writes as a single attribute with no child",
			structure: optNode("ITEM_TREE", "at0010", optSingle("items")),
			present:   true,
		},
		{
			name:      "control: ITEM_TREE items the template leaves optional by its existence",
			structure: optNode("ITEM_TREE", "at0010", optList("items", 0, 0)),
		},
		{
			name:      "control: ITEM_LIST items the template leaves optional, with only a slot",
			structure: optNode("ITEM_LIST", "at0010", optList("items", 0, 0, optSlot("ELEMENT", "at0011"))),
		},
	}
	for _, tc := range cases {
		for _, implicit := range []bool{true, false} {
			c := compileOPTText(t, optStructureObservation(optSingle("protocol", tc.structure)), implicit)
			for _, opts := range guardOptions() {
				t.Run(fmt.Sprintf("%s/implicit=%t/%v/%v", tc.name, implicit, opts.Policy, opts.ValueFill), func(t *testing.T) {
					out, err := instance.Generate(t.Context(), c, opts)
					if err != nil {
						t.Fatalf("Generate: %v", err)
					}
					obs, ok := out.(*rm.Observation)
					if !ok {
						t.Fatalf("Generate root = %T, want *rm.Observation", out)
					}
					present, members, ok := itemStructureList(obs.Protocol)
					if !ok {
						t.Fatalf("protocol = %T, want an item structure", obs.Protocol)
					}
					protocol := jsonObject(jsonObject(decodedJSON(t, out))["protocol"])
					key := "items"
					if protocol["_type"] == "ITEM_TABLE" {
						key = "rows"
					}
					_, inJSON := protocol[key]
					switch {
					case members != 0:
						t.Errorf("protocol %s holds %d members, want none", key, members)
					case tc.present && (!present || !jsonEmptyList(protocol, key)):
						t.Errorf("protocol %s present=%t, JSON %v, want a present empty list written as []: its existence is 1..1", key, present, protocol[key])
					case !tc.present && (present || inJSON):
						t.Errorf("protocol %s present=%t, JSON %v, want it absent: the template leaves it optional", key, present, protocol[key])
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

// TestREQ107_RequiredListCanonicalJSONCannotCarryEmptyIsLeftOut is the
// REQ-107 Known gap check that a list the template requires by its
// existence alone, which the walk gives no member, is left out where
// canonical JSON cannot carry it empty: an OBSERVATION's
// other_participations at existence 1..1 with no child stays nil under
// every policy, both value fills and both compile modes, while the
// protocol's items beside it, at the same existence, are present and
// empty. The root passes the RM floor; the template validator is not asked,
// since the root falls outside the output rule there.
func TestREQ107_RequiredListCanonicalJSONCannotCarryEmptyIsLeftOut(t *testing.T) {
	opt := optStructureObservation(optSingle("protocol", optNode("ITEM_TREE", "at0010", optList("items", 1, 0))) +
		optList("other_participations", 1, 0))
	for _, implicit := range []bool{true, false} {
		c := compileOPTText(t, opt, implicit)
		for _, opts := range guardOptions() {
			t.Run(fmt.Sprintf("implicit=%t/%v/%v", implicit, opts.Policy, opts.ValueFill), func(t *testing.T) {
				out, err := instance.Generate(t.Context(), c, opts)
				if err != nil {
					t.Fatalf("Generate: %v", err)
				}
				obs, ok := out.(*rm.Observation)
				if !ok {
					t.Fatalf("Generate root = %T, want *rm.Observation", out)
				}
				if obs.OtherParticipations != nil {
					t.Errorf("other_participations = %v, want nil: canonical JSON cannot carry it empty", obs.OtherParticipations)
				}
				tree := jsonObject(decodedJSON(t, out))
				if v, ok := tree["other_participations"]; ok {
					t.Errorf("JSON other_participations = %v, want none", v)
				}
				if present, members, _ := itemStructureList(obs.Protocol); !present || members != 0 {
					t.Errorf("protocol items present=%t with %d members, want present and empty", present, members)
				}
				noFloorErrors(t, out)
			})
		}
	}
}
