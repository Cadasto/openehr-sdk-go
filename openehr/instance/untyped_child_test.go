package instance_test

import (
	"fmt"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/composition"
	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/internal/rmnames"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// optUntyped is a C_COMPLEX_OBJECT that names no RM type, with the given
// node id and attributes.
func optUntyped(nodeID string, attrs ...string) string {
	return optNode("", nodeID, attrs...)
}

// optAbstractInterval is a DV_INTERVAL over the abstract parameter param,
// written as an OPT rm_type_name, with the given attributes.
func optAbstractInterval(param string, attrs ...string) string {
	return optNode("DV_INTERVAL&lt;"+param+"&gt;", "", attrs...)
}

// valueTypeName is the RM type name of an ELEMENT's value: a typed
// interval's parameterised name, or the class name of any other value.
func valueTypeName(v rm.DataValue) string {
	if name, ok := rmnames.TypedIntervalName(v); ok {
		return name
	}
	return rmTypeName(v)
}

// TestREQ107_UntypedAndAbstractIntervalChildrenAreBuilt is the REQ-107
// check of an OPT child the generator used to refuse as an unknown RM type,
// although the template validator accepts what it is built as. An untyped
// child, which names no RM type, is built as the type the BMM declares for
// its attribute, or the concrete type the generator builds for it: a
// DV_TEXT for an ELEMENT's value, typed DATA_VALUE, and an ELEMENT for a
// CLUSTER's items, typed ITEM. A DV_INTERVAL over an abstract parameter
// other than DV_ORDERED is built over a concrete descendant of it: the type
// the OPT names for a bound where that is one, and otherwise the first in
// name order, DV_COUNT for DV_QUANTIFIED and DV_AMOUNT. Each root passes
// the template validator and the RM floor, under every policy, both value
// fills and both compile modes.
func TestREQ107_UntypedAndAbstractIntervalChildrenAreBuilt(t *testing.T) {
	cases := []struct {
		name string
		opt  string
		// want is the RM type of the ELEMENT's value, or, for a CLUSTER
		// root, of its one member.
		want string
	}{
		{
			name: "an untyped ELEMENT value",
			opt:  optTemplate("ELEMENT", optSingle("value", optUntyped(""))),
			want: "DV_TEXT",
		},
		{
			name: "an untyped CLUSTER item",
			opt:  optTemplate("CLUSTER", optMultiple("items", optUntyped("at0001"))),
			want: "ELEMENT",
		},
		{
			name: "a DV_INTERVAL over DV_QUANTIFIED with no bound",
			opt:  optTemplate("ELEMENT", optSingle("value", optAbstractInterval("DV_QUANTIFIED"))),
			want: "DV_INTERVAL<DV_COUNT>",
		},
		{
			name: "a DV_INTERVAL over DV_AMOUNT with no bound",
			opt:  optTemplate("ELEMENT", optSingle("value", optAbstractInterval("DV_AMOUNT"))),
			want: "DV_INTERVAL<DV_COUNT>",
		},
		{
			name: "a DV_INTERVAL over DV_TEMPORAL whose lower bound is a DV_DATE_TIME",
			opt: optTemplate("ELEMENT", optSingle("value", optAbstractInterval("DV_TEMPORAL",
				optSingle("lower", optNode("DV_DATE_TIME", ""))))),
			want: "DV_INTERVAL<DV_DATE_TIME>",
		},
		{
			name: "a DV_INTERVAL over DV_QUANTIFIED whose upper bound is a DV_DURATION",
			opt: optTemplate("ELEMENT", optSingle("value", optAbstractInterval("DV_QUANTIFIED",
				optSingle("upper", optNode("DV_DURATION", ""))))),
			want: "DV_INTERVAL<DV_DURATION>",
		},
		{
			name: "a DV_INTERVAL over DV_QUANTIFIED with an untyped lower bound",
			opt: optTemplate("ELEMENT", optSingle("value", optAbstractInterval("DV_QUANTIFIED",
				optSingle("lower", optUntyped(""))))),
			want: "DV_INTERVAL<DV_COUNT>",
		},
	}
	for _, tc := range cases {
		for _, implicit := range []bool{true, false} {
			c := compileOPTText(t, tc.opt, implicit)
			for _, opts := range defaultsOptions() {
				t.Run(fmt.Sprintf("%s/implicit=%t/%v/%v", tc.name, implicit, opts.Policy, opts.ValueFill), func(t *testing.T) {
					out, err := instance.Generate(t.Context(), c, opts)
					if err != nil {
						t.Fatalf("Generate: %v", err)
					}
					got := ""
					switch v := out.(type) {
					case *rm.Element:
						if v.Value != nil && !rm.IsTypedNil(v.Value) {
							got = valueTypeName(v.Value)
						}
					case *rm.Cluster:
						if len(v.Items) == 1 {
							got = rmTypeName(v.Items[0])
						}
					}
					if got != tc.want {
						t.Errorf("built %q, want %q", got, tc.want)
					}
					noFloorErrors(t, out)
					for _, iss := range templateErrors(out, c) {
						t.Errorf("Validate: %s @ %s: %s", iss.Code, iss.Path, iss.Detail)
					}
				})
			}
		}
	}
}

// TestREQ107_SkeletonOfUntypedAndAbstractIntervalChildren is the REQ-101
// side of TestREQ107_UntypedAndAbstractIntervalChildrenAreBuilt:
// NewSkeleton returns a composition whose context's other_context holds an
// ELEMENT with an untyped value and one with a DV_INTERVAL over
// DV_QUANTIFIED, which passes the template validator, under every policy.
func TestREQ107_SkeletonOfUntypedAndAbstractIntervalChildren(t *testing.T) {
	tree := optNode("ITEM_TREE", "at0001", optMultiple("items",
		optElementOccurring("at0002", 1, optSingle("value", optUntyped(""))),
		optElementOccurring("at0003", 1, optSingle("value", optAbstractInterval("DV_QUANTIFIED")))))
	opt := optTemplate("COMPOSITION", optSingle("context", optNode("EVENT_CONTEXT", "", optSingle("other_context", tree))))
	for _, implicit := range []bool{true, false} {
		c := compileOPTText(t, opt, implicit)
		for _, policy := range everyPolicy {
			t.Run(fmt.Sprintf("implicit=%t/%v", implicit, policy), func(t *testing.T) {
				comp, err := composition.NewSkeleton(t.Context(), c, composition.WithPolicy(policy),
					composition.WithTerritory("NL"), composition.WithComposer(testComposer()), composition.WithNow(defaultsNow))
				if err != nil {
					t.Fatalf("NewSkeleton: %v", err)
				}
				noValidationErrors(t, comp, c)
				noFloorErrors(t, comp)
			})
		}
	}
}

// optElementOccurring is an ELEMENT child with the node id nodeID and
// occurrences lower..1, with the given attributes.
func optElementOccurring(nodeID string, lower int, attrs ...string) string {
	return optOccurring("C_COMPLEX_OBJECT", "ELEMENT", nodeID, lower, 1, attrs...)
}
