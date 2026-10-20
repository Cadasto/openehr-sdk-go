package instance_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// The OPTs below exercise the REQ-107 rule on content the template does not
// describe (§ Slot fills): a slot fill, or a LOCATABLE built from the BMM
// type of an attribute with no allowed child. A node needs such content
// when it is a slot, or when an attribute of its own that is required can
// only be given such members.

// slotOnlyCluster is a CLUSTER whose items, which the RM requires, admit
// only a slot of CLUSTER with no includes, under the given occurrences
// (an empty string gives none, so the CLUSTER is required).
func slotOnlyCluster(nodeID, occurrences string) string {
	return guardChild("C_COMPLEX_OBJECT", "CLUSTER", nodeID, occurrences, "",
		optMultiple("items", optSlot("CLUSTER", nodeID+"1")))
}

// describedHistory is an OBSERVATION's data with a HISTORY the template
// describes. Its events are optional in the RM, so it needs nothing more.
var describedHistory = optSingle("data", optNode("HISTORY", "at0001"))

// undescribedObservation is an OPT child for an OBSERVATION archetype root
// of archetypeID with the given occurrences and attributes.
func undescribedObservation(archetypeID, occurrences string, attrs ...string) string {
	return guardChild("C_ARCHETYPE_ROOT", "OBSERVATION", "at0000", occurrences, archetypeID, attrs...)
}

// undescribedShape renders what the walk built: the archetype node ids of a
// COMPOSITION's content, or of a CLUSTER's or an ITEM_TREE's items, in
// order, each CLUSTER followed by its own items in parentheses; for an
// OBSERVATION, whether it has a protocol.
func undescribedShape(v any) string {
	switch n := v.(type) {
	case *rm.Composition:
		ids := make([]string, len(n.Content))
		for i, c := range n.Content {
			ids[i] = nodeID(c)
		}
		return strings.Join(ids, ",")
	case *rm.ItemTree:
		return itemsShape(n.Items)
	case *rm.Cluster:
		return itemsShape(n.Items)
	case *rm.Observation:
		return fmt.Sprintf("protocol=%t", n.Protocol != nil && !rm.IsTypedNil(n.Protocol))
	}
	return fmt.Sprintf("%T", v)
}

func itemsShape(items []rm.Item) string {
	parts := make([]string, len(items))
	for i, item := range items {
		parts[i] = nodeID(item)
		if c, ok := item.(*rm.Cluster); ok {
			parts[i] += "(" + itemsShape(c.Items) + ")"
		}
	}
	return strings.Join(parts, ",")
}

// TestREQ107_OptionalNodeHoldingOnlyUndescribedContentGetsNoMember is the
// REQ-107 check of § Slot fills and the Visit rule under Large and Medium:
// an optional child that needs undescribed content gets no member of its
// own, an attribute that is not required and whose members would all be
// undescribed or need it is not visited, a top-up takes a child that needs
// none, and Medium's first optional archetype root is the first that needs
// none. The rule reads a node's own attributes only, so a required node
// below an optional one keeps its undescribed members and the optional
// node is kept. Every root passes the RM floor and the template validator,
// under every policy, both value fills and both compile modes.
func TestREQ107_OptionalNodeHoldingOnlyUndescribedContentGetsNoMember(t *testing.T) {
	const (
		slotted   = "openEHR-EHR-OBSERVATION.slotted.v1"
		bare      = "openEHR-EHR-OBSERVATION.bare.v1"
		described = "openEHR-EHR-OBSERVATION.described.v1"
		third     = "openEHR-EHR-OBSERVATION.third.v1"
		stub      = "openEHR-EHR-CLUSTER.example.v1"
	)
	element := optNode("ELEMENT", "at0009")
	slottedProtocol := optSingle("protocol", optSlot("ITEM_TREE", "at0002"))
	cases := []struct {
		name string
		opt  string
		// want is the shape (undescribedShape) per policy.
		large, medium, required string
	}{
		{
			name:  "optional child whose required items admit only slots",
			opt:   optTemplate("CLUSTER", optMultiple("items", element, slotOnlyCluster("at0002", guardOccurrences01))),
			large: "at0009", medium: "at0009", required: "at0009",
		},
		{
			name: "optional child whose required items the template leaves without a child",
			opt: optTemplate("CLUSTER", optMultiple("items", element,
				guardChild("C_COMPLEX_OBJECT", "CLUSTER", "at0002", guardOccurrences01, "", optRequiredMultiple("items")))),
			large: "at0009", medium: "at0009", required: "at0009",
		},
		{
			// CLUSTER.items is mandatory in the RM: the compile step adds it
			// with no child, or, without implicit attributes, the node does
			// not carry it.
			name: "optional child that does not name the items the RM requires",
			opt: optTemplate("CLUSTER", optMultiple("items", element,
				guardChild("C_COMPLEX_OBJECT", "CLUSTER", "at0002", guardOccurrences01, ""))),
			large: "at0009", medium: "at0009", required: "at0009",
		},
		{
			name: "optional entry whose required protocol admits only a slot",
			opt: guardOPT(guardCompositionID, guardMultiple("content", guardExistence11,
				undescribedObservation(slotted, guardOccurrences01, describedHistory, slottedProtocol),
				undescribedObservation(guardObservationID, guardOccurrences11, describedHistory))),
			large: "openEHR-EHR-OBSERVATION.example.v1", medium: "openEHR-EHR-OBSERVATION.example.v1",
			required: "openEHR-EHR-OBSERVATION.example.v1",
		},
		{
			name: "optional entry whose data, which the RM requires, the template leaves without a child",
			opt: guardOPT(guardCompositionID, guardMultiple("content", guardExistence11,
				undescribedObservation(bare, guardOccurrences01),
				undescribedObservation(guardObservationID, guardOccurrences11, describedHistory))),
			large: "openEHR-EHR-OBSERVATION.example.v1", medium: "openEHR-EHR-OBSERVATION.example.v1",
			required: "openEHR-EHR-OBSERVATION.example.v1",
		},
		{
			name: "optional attribute whose one child needs undescribed content",
			opt: optTemplate("ITEM_TREE", optMultipleLowerZero("items",
				slotOnlyCluster("at0002", ""))),
			large: "", medium: "", required: "",
		},
		{
			name: "optional single attribute whose child needs undescribed content",
			opt: optTemplate("OBSERVATION", describedHistory, optOptionalSingleOver("protocol",
				optNode("ITEM_TREE", "at0002", optMultiple("items", optSlot("CLUSTER", "at0003"))))),
			large: "protocol=false", medium: "protocol=false", required: "protocol=false",
		},
		{
			name: "optional single attribute whose one child is a slot",
			opt: optTemplate("OBSERVATION", describedHistory,
				optOptionalSingleOver("protocol", optSlot("ITEM_TREE", "at0002"))),
			large: "protocol=false", medium: "protocol=false", required: "protocol=false",
		},
		{
			// One level: the required CLUSTER needs undescribed content, and
			// keeps it, since nothing optional holds it; the optional
			// attribute is visited for its described element.
			name: "required child beside a described one in an optional attribute",
			opt: optTemplate("ITEM_TREE", optMultipleLowerZero("items",
				slotOnlyCluster("at0002", ""), optOccurring("C_COMPLEX_OBJECT", "ELEMENT", "at0003", 0, 1))),
			large: "at0002(" + stub + "(at0000)),at0003", medium: "at0002(" + stub + "(at0000)),at0003", required: "",
		},
		{
			// Two levels: the optional CLUSTER's items take a CLUSTER the
			// template describes, so it needs none; the inner CLUSTER is
			// required and keeps its slot fill.
			name: "undescribed content two levels below an optional child",
			opt: optTemplate("CLUSTER", optMultiple("items", element,
				guardChild("C_COMPLEX_OBJECT", "CLUSTER", "at0002", guardOccurrences01, "",
					optMultiple("items", slotOnlyCluster("at0003", ""))))),
			large:    "at0009,at0002(at0003(" + stub + "(at0000)))",
			medium:   "at0009,at0002(at0003(" + stub + "(at0000)))",
			required: "at0009",
		},
		{
			// The RM requires the items: the top-up takes the first child
			// that needs no undescribed content.
			name: "top-up past an optional child that needs undescribed content",
			opt: optTemplate("CLUSTER", optMultiple("items",
				slotOnlyCluster("at0002", guardOccurrences01), optOccurring("C_COMPLEX_OBJECT", "ELEMENT", "at0003", 0, 1))),
			large: "at0003", medium: "at0003", required: "at0003",
		},
		{
			// Every child needs it: the top-up takes the first that is not
			// a slot, and the RM requirement keeps its fill.
			name: "top-up where every child needs undescribed content",
			opt: optTemplate("CLUSTER", optMultiple("items",
				slotOnlyCluster("at0002", guardOccurrences01))),
			large: "at0002(" + stub + "(at0000))", medium: "at0002(" + stub + "(at0000))",
			required: "at0002(" + stub + "(at0000))",
		},
		{
			name: "first optional archetype root under Medium is the first that needs none",
			opt: guardOPT(guardCompositionID, guardMultiple("content", guardExistence11,
				undescribedObservation(slotted, guardOccurrences01, describedHistory, slottedProtocol),
				undescribedObservation(described, guardOccurrences01, describedHistory),
				undescribedObservation(third, guardOccurrences01, describedHistory))),
			large: described + "," + third, medium: described, required: described,
		},
	}
	for _, tc := range cases {
		for _, implicit := range []bool{true, false} {
			c := compileOPTText(t, tc.opt, implicit)
			for _, opts := range guardOptions() {
				t.Run(fmt.Sprintf("%s/implicit=%t/%v/%v", tc.name, implicit, opts.Policy, opts.ValueFill), func(t *testing.T) {
					want := map[instance.Policy]string{
						instance.Large: tc.large, instance.Medium: tc.medium, instance.Required: tc.required,
					}[opts.Policy]
					out, err := instance.Generate(t.Context(), c, opts)
					if err != nil {
						t.Fatalf("Generate: %v", err)
					}
					if got := undescribedShape(out); got != want {
						t.Errorf("Generate built %q, want %q", got, want)
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

// TestREQ107_RequiredSlotOnlyClusterStillFills is the REQ-107 check that
// the rule on undescribed content leaves a required slot alone (§ Slot
// fills): a CLUSTER the template requires, whose items admit only a slot,
// gets that slot stamped with the archetype id its includes give, or makes
// Generate return ErrSlotFillUnsupported where its includes give none, at
// every policy, both value fills and both compile modes. The same CLUSTER
// left optional beside a described element gets no member under any policy,
// so the slot no example can fill does not stop Generate.
func TestREQ107_RequiredSlotOnlyClusterStillFills(t *testing.T) {
	const device = "openEHR-EHR-CLUSTER.device.v1"
	element := optNode("ELEMENT", "at0009")
	cluster := func(occurrences, pattern string) string {
		return guardChild("C_COMPLEX_OBJECT", "CLUSTER", "at0002", occurrences, "",
			optMultiple("items", optSlotIncluding("CLUSTER", "at0003", pattern)))
	}
	cases := []struct {
		name string
		opt  string
		want string // the shape (undescribedShape); "" wants ErrSlotFillUnsupported
	}{
		{
			name: "required, includes a device cluster",
			opt:  optTemplate("CLUSTER", optMultiple("items", element, cluster("", `openEHR-EHR-CLUSTER\.device\.v1`))),
			want: "at0009,at0002(" + device + "(at0000))",
		},
		{
			name: "required, includes no archetype id the generator can derive",
			opt:  optTemplate("CLUSTER", optMultiple("items", element, cluster("", `openEHR-EHR-CLUSTER\..*`))),
		},
		{
			name: "optional, includes no archetype id the generator can derive",
			opt:  optTemplate("CLUSTER", optMultiple("items", element, cluster(guardOccurrences01, `openEHR-EHR-CLUSTER\..*`))),
			want: "at0009",
		},
	}
	for _, tc := range cases {
		for _, implicit := range []bool{true, false} {
			c := compileOPTText(t, tc.opt, implicit)
			for _, opts := range guardOptions() {
				t.Run(fmt.Sprintf("%s/implicit=%t/%v/%v", tc.name, implicit, opts.Policy, opts.ValueFill), func(t *testing.T) {
					out, err := instance.Generate(t.Context(), c, opts)
					if tc.want == "" {
						if !errors.Is(err, instance.ErrSlotFillUnsupported) {
							t.Fatalf("Generate error = %v, want one wrapping ErrSlotFillUnsupported", err)
						}
						return
					}
					if err != nil {
						t.Fatalf("Generate: %v", err)
					}
					if got := undescribedShape(out); got != tc.want {
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
