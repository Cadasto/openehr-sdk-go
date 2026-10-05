package instance_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/composition"
	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
)

// optSlotIncluding is an ARCHETYPE_SLOT of rmType whose includes admit the
// archetype ids that pattern matches.
func optSlotIncluding(rmType, nodeID, pattern string) string {
	return `<children xsi:type="ARCHETYPE_SLOT"><rm_type_name>` + rmType + `</rm_type_name>` +
		`<node_id>` + nodeID + `</node_id><includes><expression xsi:type="EXPR_BINARY_OPERATOR">` +
		`<type>Boolean</type><operator>2007</operator><precedence_overridden>false</precedence_overridden>` +
		`<left_operand xsi:type="EXPR_LEAF"><type>String</type><item xsi:type="xsd:string">archetype_id/value</item>` +
		`<reference_type>attribute</reference_type></left_operand>` +
		`<right_operand xsi:type="EXPR_LEAF"><type>C_STRING</type><item xsi:type="C_STRING"><pattern>` + pattern +
		`</pattern></item><reference_type>constraint</reference_type></right_operand></expression></includes></children>`
}

// slotProtocolOPT is a COMPOSITION whose one OBSERVATION has an optional
// protocol with the given OPT children.
func slotProtocolOPT(children ...string) string {
	return guardOPT(guardCompositionID, guardMultiple("content", guardExistence11,
		guardChild("C_ARCHETYPE_ROOT", "OBSERVATION", "at0000", guardOccurrences11, guardObservationID,
			optOptionalSingleOver("protocol", children...))))
}

// TestREQ107_SingleValuedSlotFillIsStamped is the REQ-107 check that a slot
// that is the first allowed child of a visited single-valued attribute, a
// required slot, is stamped with an archetype id its includes admit, or
// with the RM-type-prefix fallback where it has no includes; and that one
// whose includes give no archetype id the generator can derive makes
// Generate return an error wrapping ErrSlotFillUnsupported. Both hold at
// either policy, either value fill and through the builder; an OBSERVATION's
// protocol is the attribute, which either policy visits since it has an
// allowed OPT child.
func TestREQ107_SingleValuedSlotFillIsStamped(t *testing.T) {
	const medication = "openEHR-EHR-ITEM_TREE.medication.v1"
	cases := []struct {
		name string
		opt  string
		want string // the archetype id; "" wants ErrSlotFillUnsupported
	}{
		{
			name: "includes a medication tree",
			opt:  slotProtocolOPT(optSlotIncluding("ITEM_TREE", "at0001", `openEHR-EHR-ITEM_TREE\.medication\.v1`)),
			want: medication,
		},
		{
			name: "no includes",
			opt:  slotProtocolOPT(optSlot("ITEM_TREE", "at0001")),
			want: "openEHR-EHR-ITEM_TREE.example.v1",
		},
		{
			// The prohibited alternative is of another RM type: the
			// template validator binds a single-valued attribute's value to
			// the first alternative of its RM type, prohibited or not.
			name: "after a prohibited alternative",
			opt: slotProtocolOPT(optOccurring("C_COMPLEX_OBJECT", "ITEM_LIST", "at0002", 0, 0),
				optSlotIncluding("ITEM_TREE", "at0001", `openEHR-EHR-ITEM_TREE\.medication\.v1`)),
			want: medication,
		},
		{
			name: "includes no archetype id the generator can derive",
			opt:  slotProtocolOPT(optSlotIncluding("ITEM_TREE", "at0001", `openEHR-EHR-ITEM_TREE\..*`)),
		},
	}
	for _, tc := range cases {
		c := compileOPTText(t, tc.opt, true)
		for _, opts := range guardOptions() {
			t.Run(fmt.Sprintf("%s/%v/%v", tc.name, opts.Policy, opts.ValueFill), func(t *testing.T) {
				out, err := instance.Generate(t.Context(), c, opts)
				checkSlotProtocol(t, c, out, err, tc.want)
			})
		}
		t.Run(tc.name+"/builder", func(t *testing.T) {
			b, err := composition.NewBuilder(t.Context(), c, composition.WithTerritory("NL"),
				composition.WithComposer(testComposer()), composition.WithNow(defaultsNow))
			if err != nil {
				checkSlotProtocol(t, c, nil, err, tc.want)
				return
			}
			comp, err := b.Build()
			checkSlotProtocol(t, c, comp, err, tc.want)
		})
	}
}

// checkSlotProtocol fails t unless the protocol of the one OBSERVATION in
// out carries the archetype id want, as its node id and in its
// archetype_details, or, when want is "", err wraps ErrSlotFillUnsupported.
func checkSlotProtocol(t *testing.T, c *templatecompile.Compiled, out any, err error, want string) {
	t.Helper()
	if want == "" {
		if !errors.Is(err, instance.ErrSlotFillUnsupported) {
			t.Fatalf("error = %v, want one wrapping ErrSlotFillUnsupported", err)
		}
		return
	}
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	obs, ok := firstContent(t, "Generate", out).(*rm.Observation)
	if !ok {
		t.Fatalf("content[0] is %T, want *rm.Observation", firstContent(t, "Generate", out))
	}
	tree, ok := obs.Protocol.(*rm.ItemTree)
	if !ok || tree == nil {
		t.Fatalf("OBSERVATION.protocol is %T, want the stamped ITEM_TREE", obs.Protocol)
	}
	if tree.ArchetypeNodeID != want {
		t.Errorf("protocol archetype_node_id = %q, want %q", tree.ArchetypeNodeID, want)
	}
	if tree.ArchetypeDetails == nil || tree.ArchetypeDetails.ArchetypeID.Value != want {
		t.Errorf("protocol archetype_details = %+v, want archetype id %q", tree.ArchetypeDetails, want)
	}
	noFloorErrors(t, out)
	for _, iss := range templateErrors(out, c) {
		t.Errorf("template validator: %s @ %s: %s", iss.Code, iss.Path, iss.Detail)
	}
}
