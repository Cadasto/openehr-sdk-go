package instance_test

import (
	"bytes"
	"errors"
	"fmt"
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/templateinstance/rmwrite"
	"github.com/cadasto/openehr-sdk-go/openehr/composition"
	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/internal/rmnames"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
)

// optUntyped is a C_COMPLEX_OBJECT that names no RM type, with the given
// node id and attributes.
func optUntyped(nodeID string, attrs ...string) string {
	return optNode("", nodeID, attrs...)
}

// optAbstractInterval is a DV_INTERVAL over the parameter param, written as
// an OPT rm_type_name, with the given attributes.
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
// CLUSTER's items, typed ITEM. A child of an abstract data-value class,
// such as DV_QUANTIFIED, is built as its first concrete descendant in name
// order. A DV_INTERVAL over an abstract parameter
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
			name: "an ELEMENT value of the abstract DV_QUANTIFIED",
			opt:  optTemplate("ELEMENT", optSingle("value", optNode("DV_QUANTIFIED", ""))),
			want: "DV_COUNT",
		},
		{
			name: "an ELEMENT value of the abstract DV_ORDERED",
			opt:  optTemplate("ELEMENT", optSingle("value", optNode("DV_ORDERED", ""))),
			want: "DV_COUNT",
		},
		{
			name: "an ELEMENT value of the abstract DV_TEMPORAL",
			opt:  optTemplate("ELEMENT", optSingle("value", optNode("DV_TEMPORAL", ""))),
			want: "DV_DATE",
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

// historyOf is a HISTORY template with an origin whose events are events.
func historyOf(events ...string) string {
	return optTemplate("HISTORY", optSingle("origin", optNode("DV_DATE_TIME", "")), optMultiple("events", events...))
}

// observationWithProtocol is an OBSERVATION template with one event over an
// empty tree, whose protocol is the attribute protocol.
func observationWithProtocol(protocol string) string {
	history := optNode("HISTORY", "at0001", optSingle("origin", optNode("DV_DATE_TIME", "")),
		optMultiple("events", optNode("POINT_EVENT", "at0002", optSingle("data", emptyTree))))
	return optTemplate("OBSERVATION", optSingle("data", history), protocol)
}

// TestREQ107_UntypedChildIsReadAsTheTypeItIsBuiltAs is the REQ-107 check
// that the walk reads an untyped child, which names no RM type, as the type
// it builds it as, wherever it reads a node's type: whether its attribute
// is typed by an RM class, so that an AOM primitive short name under it is
// built as its DV wrapper; which of its attributes the RM computes rather
// than stores; the type of an attribute the template names with no child;
// and whether such an attribute can only hold content the template does
// not describe. Each template holds one untyped child, and the root
// Generate returns for it is the root it returns for the same template
// with the child typed as the walk builds it, which passes the template
// validator and the RM floor, under every policy, both value fills and both
// compile modes.
func TestREQ107_UntypedChildIsReadAsTheTypeItIsBuiltAs(t *testing.T) {
	cases := []struct {
		name string
		// typed is the RM type the walk builds the untyped child as.
		typed string
		// opt is the template with the child typed as rmType.
		opt func(rmType string) string
	}{
		{
			name:  "an ELEMENT whose value is a DATE_TIME object",
			typed: "ELEMENT",
			opt: func(rmType string) string {
				return optTemplate("CLUSTER", optMultiple("items", optNode(rmType, "at0001",
					optSingle("value", optNode("DATE_TIME", "")))))
			},
		},
		{
			name:  "an ELEMENT whose value is a C_DATE_TIME",
			typed: "ELEMENT",
			opt: func(rmType string) string {
				return optTemplate("CLUSTER", optMultiple("items", optNode(rmType, "at0001",
					optSingle("value", optPrimitive("DATE_TIME", "C_DATE_TIME", "<pattern>yyyy-mm-ddTHH:MM:SS</pattern>")))))
			},
		},
		{
			name:  "a POINT_EVENT that names its offset",
			typed: "POINT_EVENT",
			opt: func(rmType string) string {
				return historyOf(optNode(rmType, "at0002",
					optOptionalSingleOver("offset", optNode("DV_DURATION", "")), optSingle("data", emptyTree)))
			},
		},
		{
			name:  "an ELEMENT whose value the template names with no child",
			typed: "ELEMENT",
			opt: func(rmType string) string {
				return optTemplate("CLUSTER", optMultiple("items", optNode(rmType, "at0001", optSingle("value"))))
			},
		},
		{
			name:  "a POINT_EVENT whose data the template names with no child",
			typed: "POINT_EVENT",
			opt: func(rmType string) string {
				return historyOf(optNode(rmType, "at0002", optSingle("data")))
			},
		},
		{
			name:  "an optional POINT_EVENT whose data the template names with no child",
			typed: "POINT_EVENT",
			opt: func(rmType string) string {
				return historyOf(
					optOccurring("C_COMPLEX_OBJECT", "POINT_EVENT", "at0002", 1, 1, optSingle("data", emptyTree)),
					optOccurring("C_COMPLEX_OBJECT", rmType, "at0003", 0, 1, optSingle("data")))
			},
		},
		{
			name:  "an ITEM_TREE whose items the template names with no child",
			typed: "ITEM_TREE",
			opt: func(rmType string) string {
				return observationWithProtocol(optSingle("protocol", optNode(rmType, "at0003", optMultiple("items"))))
			},
		},
		{
			name:  "an optional ITEM_TREE whose items the template names with no child",
			typed: "ITEM_TREE",
			opt: func(rmType string) string {
				return observationWithProtocol(optOptionalSingleOver("protocol", optNode(rmType, "at0003", optMultiple("items"))))
			},
		},
	}
	for _, tc := range cases {
		for _, implicit := range []bool{true, false} {
			untyped := compileOPTText(t, tc.opt(""), implicit)
			typed := compileOPTText(t, tc.opt(tc.typed), implicit)
			for _, opts := range defaultsOptions() {
				t.Run(fmt.Sprintf("%s/implicit=%t/%v/%v", tc.name, implicit, opts.Policy, opts.ValueFill), func(t *testing.T) {
					want, err := instance.Generate(t.Context(), typed, seededOptions(opts, 1))
					if err != nil {
						t.Fatalf("Generate with the child typed %s: %v", tc.typed, err)
					}
					got, err := instance.Generate(t.Context(), untyped, seededOptions(opts, 1))
					if err != nil {
						t.Fatalf("Generate with the child untyped: %v", err)
					}
					noFloorErrors(t, got)
					for _, iss := range templateErrors(got, untyped) {
						t.Errorf("Validate: %s @ %s: %s", iss.Code, iss.Path, iss.Detail)
					}
					gotJSON, err := canjson.Marshal(got)
					if err != nil {
						t.Fatalf("canjson.Marshal: %v", err)
					}
					wantJSON, err := canjson.Marshal(want)
					if err != nil {
						t.Fatalf("canjson.Marshal: %v", err)
					}
					if !bytes.Equal(gotJSON, wantJSON) {
						t.Errorf("the untyped child's root differs from the typed one's, first at %s",
							firstJSONDifference(decodeJSON(t, gotJSON), decodeJSON(t, wantJSON), ""))
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

// TestREQ107_IntervalOverAnUnorderedTypeIsRefused is the REQ-107 check of
// the known gap on generic types: a DV_INTERVAL over a type that is no
// DV_ORDERED class, the concrete DV_TEXT or the abstract DATA_VALUE, which
// has DV_ORDERED classes among its descendants, cannot be built, and
// Generate returns an error wrapping rmwrite.ErrUnknownRMType and no root,
// under every policy and both compile modes.
func TestREQ107_IntervalOverAnUnorderedTypeIsRefused(t *testing.T) {
	for _, param := range []string{"DV_TEXT", "DATA_VALUE"} {
		opt := optTemplate("ELEMENT", optSingle("value", optAbstractInterval(param)))
		for _, implicit := range []bool{true, false} {
			c := compileOPTText(t, opt, implicit)
			for _, policy := range everyPolicy {
				t.Run(fmt.Sprintf("%s/implicit=%t/%v", param, implicit, policy), func(t *testing.T) {
					out, err := instance.Generate(t.Context(), c, instance.Options{Policy: policy, Now: defaultsNow})
					if !errors.Is(err, rmwrite.ErrUnknownRMType) {
						t.Errorf("Generate error = %v, want one wrapping rmwrite.ErrUnknownRMType", err)
					}
					if out != nil {
						t.Errorf("Generate returned %T with the error, want no root", out)
					}
				})
			}
		}
	}
}
