package instance_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// optInterval is an AOM interval body with the given bounds; an upper of
// -1 is unbounded.
func optInterval(lower, upper int) string {
	body := `<lower_included>true</lower_included><lower_unbounded>false</lower_unbounded>` +
		fmt.Sprintf(`<lower>%d</lower>`, lower)
	if upper < 0 {
		return body + `<upper_unbounded>true</upper_unbounded>`
	}
	return body + `<upper_included>true</upper_included><upper_unbounded>false</upper_unbounded>` +
		fmt.Sprintf(`<upper>%d</upper>`, upper)
}

// optCardinal is a C_MULTIPLE_ATTRIBUTE called name over children, with no
// existence and a cardinality of lower..upper (upper -1 is unbounded).
func optCardinal(name string, lower, upper int, children ...string) string {
	return `<attributes xsi:type="C_MULTIPLE_ATTRIBUTE"><rm_attribute_name>` + name + `</rm_attribute_name>` +
		strings.Join(children, "") +
		`<cardinality><is_ordered>false</is_ordered><is_unique>false</is_unique><interval>` +
		optInterval(lower, upper) + `</interval></cardinality></attributes>`
}

// optRequiredCardinal is optCardinal with an existence of 1..1 and no
// children.
func optRequiredCardinal(name string, lower, upper int) string {
	return `<attributes xsi:type="C_MULTIPLE_ATTRIBUTE"><rm_attribute_name>` + name + `</rm_attribute_name>` +
		`<existence>` + optInterval(1, 1) + `</existence>` +
		`<cardinality><is_ordered>false</is_ordered><is_unique>false</is_unique><interval>` +
		optInterval(lower, upper) + `</interval></cardinality></attributes>`
}

// optOccurring is a child of the given xsi:type and rmType with occurrences
// lower..upper (upper -1 is unbounded).
func optOccurring(xsiType, rmType, nodeID string, lower, upper int, attrs ...string) string {
	return `<children xsi:type="` + xsiType + `"><rm_type_name>` + rmType + `</rm_type_name>` +
		`<occurrences>` + optInterval(lower, upper) + `</occurrences>` +
		`<node_id>` + nodeID + `</node_id>` + strings.Join(attrs, "") + `</children>`
}

// itemIDs returns the archetype_node_id of every member of a CLUSTER's or
// an ITEM_TREE's items.
func itemIDs(t *testing.T, out any) []string {
	t.Helper()
	var items []rm.Item
	switch v := out.(type) {
	case *rm.Cluster:
		items = v.Items
	case *rm.ItemTree:
		items = v.Items
	default:
		t.Fatalf("generated root is %T, want a CLUSTER or an ITEM_TREE", out)
	}
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, nodeID(item))
	}
	return ids
}

// TestREQ107_ProhibitedChildGetsNoMember is the REQ-107 check that an OPT
// child whose occurrences upper bound is 0 gets no member of a
// multi-valued attribute: not from the per-child fill, and not as the
// seed of the top-up to the cardinality lower bound. A CLUSTER whose items
// the OPT prohibits still gets its members: the RM requires them. It
// holds under both policies, both value fills and both compile modes. A
// slot is not covered: the template parser keeps no occurrences for an
// ARCHETYPE_SLOT.
func TestREQ107_ProhibitedChildGetsNoMember(t *testing.T) {
	cases := []struct {
		name string
		opt  string
		want []string
		// valid says the template validator must find no error; the
		// CLUSTER whose only child is prohibited contradicts the RM.
		valid bool
	}{
		{
			name: "per-child fill",
			opt: optTemplate("CLUSTER", optMultiple("items",
				optNode("ELEMENT", "at0001"), optOccurring("C_COMPLEX_OBJECT", "ELEMENT", "at0002", 0, 0))),
			want:  []string{"at0001"},
			valid: true,
		},
		{
			name: "top-up seed",
			opt: optTemplate("CLUSTER", optCardinal("items", 2, -1,
				optOccurring("C_COMPLEX_OBJECT", "ELEMENT", "at0001", 0, 0),
				optOccurring("C_COMPLEX_OBJECT", "ELEMENT", "at0002", 0, -1))),
			want:  []string{"at0002", "at0002"},
			valid: true,
		},
		{
			name: "CLUSTER items with only a prohibited child",
			opt: optTemplate("CLUSTER", optCardinal("items", 0, -1,
				optOccurring("C_COMPLEX_OBJECT", "ELEMENT", "at0001", 0, 0))),
			want: []string{"at0000"},
		},
		{
			// The OPT prohibits items, which the RM requires: the RM rule
			// wins, and the walk fills the list as it would an allowed one.
			name:  "CLUSTER items prohibited, an ELEMENT child",
			opt:   optTemplate("CLUSTER", optProhibitedMultiple("items", optNode("ELEMENT", "at0001"))),
			want:  []string{"at0001"},
			valid: true,
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
					if got := itemIDs(t, out); !slices.Equal(got, tc.want) {
						t.Errorf("items = %v, want %v", got, tc.want)
					}
					noFloorErrors(t, out)
					if !tc.valid {
						return
					}
					for _, iss := range templateErrors(out, c) {
						t.Errorf("template validator: %s @ %s: %s", iss.Code, iss.Path, iss.Detail)
					}
				})
			}
		}
	}
}

// TestREQ107_ProhibitedAlternativeIsSkipped is the REQ-107 check that a
// C_SINGLE_ATTRIBUTE skips an alternative whose occurrences upper bound is
// 0: the next allowed one wins, and with none allowed the OPT prohibits
// the attribute, so neither policy visits an optional one.
func TestREQ107_ProhibitedAlternativeIsSkipped(t *testing.T) {
	cases := []struct {
		name string
		attr string
		// want is the RM type of the ELEMENT's value under each policy,
		// "" for none.
		want map[instance.Policy]string
	}{
		{
			name: "next allowed alternative wins",
			attr: optSingle("value",
				optOccurring("C_COMPLEX_OBJECT", "DV_TEXT", "", 0, 0), optNode("DV_COUNT", "")),
			want: map[instance.Policy]string{instance.Minimal: "DV_COUNT", instance.Example: "DV_COUNT"},
		},
		{
			// Every alternative is prohibited, so the OPT prohibits the
			// optional attribute: neither policy visits it.
			name: "no allowed alternative: prohibited",
			attr: optOptionalSingleOver("value", optOccurring("C_COMPLEX_OBJECT", "DV_COUNT", "", 0, 0)),
			want: map[instance.Policy]string{instance.Minimal: "", instance.Example: ""},
		},
	}
	for _, tc := range cases {
		for _, implicit := range []bool{true, false} {
			c := compileOPTText(t, optTemplate("ELEMENT", tc.attr), implicit)
			for _, opts := range defaultsOptions() {
				t.Run(fmt.Sprintf("%s/implicit=%t/%v/%v", tc.name, implicit, opts.Policy, opts.ValueFill), func(t *testing.T) {
					out, err := instance.Generate(t.Context(), c, opts)
					if err != nil {
						t.Fatalf("Generate: %v", err)
					}
					got := ""
					if v := out.(*rm.Element).Value; v != nil && !rm.IsTypedNil(v) {
						got = rmTypeName(v)
					}
					if want := tc.want[opts.Policy]; got != want {
						t.Errorf("ELEMENT.value is %q, want %q", got, want)
					}
					noFloorErrors(t, out)
				})
			}
		}
	}
}

// TestREQ107_SilentRequiredMultipleIsToppedUp is the REQ-107 check that a
// multi-valued attribute the OPT names without children, whose cardinality
// lower bound is above 1, is topped up to that bound with members built
// from its BMM type, and to no more than its cardinality upper bound,
// which wins where the existence requires a member the cardinality does
// not allow.
func TestREQ107_SilentRequiredMultipleIsToppedUp(t *testing.T) {
	cases := []struct {
		name string
		opt  string
		want int
	}{
		{name: "CLUSTER items, lower 2", opt: optTemplate("CLUSTER", optCardinal("items", 2, -1)), want: 2},
		{name: "ITEM_TREE items, lower 3", opt: optTemplate("ITEM_TREE", optCardinal("items", 3, -1)), want: 3},
		// Existence 1..1 requires a member, and cardinality 0..0 allows
		// none; the upper bound wins.
		{name: "ITEM_TREE items, existence 1, upper 0", opt: optTemplate("ITEM_TREE", optRequiredCardinal("items", 0, 0)), want: 0},
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
					if got := len(itemIDs(t, out)); got != tc.want {
						t.Errorf("items has %d members, want %d", got, tc.want)
					}
					noFloorErrors(t, out)
				})
			}
		}
	}
}

// rmTypeName is the RM type name of a generated value.
func rmTypeName(v any) string {
	if n, ok := v.(interface{ BMMName() string }); ok {
		return n.BMMName()
	}
	return fmt.Sprintf("%T", v)
}

// TestREQ107_MultipleSizedByOccurrencesAndCardinality is the REQ-107 check
// that a multi-valued attribute gets max(occurrences.lower, 1) members of
// each OPT child, in OPT order, and no more in all than its cardinality
// upper bound, and is topped up to its cardinality lower bound with
// members of its first child. It holds under both policies, both value
// fills and both compile modes, and the template validator finds no error.
func TestREQ107_MultipleSizedByOccurrencesAndCardinality(t *testing.T) {
	cases := []struct {
		name string
		opt  string
		want []string
	}{
		{
			name: "cardinality upper 1 across two children",
			opt: optTemplate("CLUSTER", optCardinal("items", 1, 1,
				optOccurring("C_COMPLEX_OBJECT", "ELEMENT", "at0001", 0, 1),
				optOccurring("C_COMPLEX_OBJECT", "ELEMENT", "at0002", 0, 1))),
			want: []string{"at0001"},
		},
		{
			name: "occurrences lower 2",
			opt: optTemplate("CLUSTER", optCardinal("items", 1, -1,
				optOccurring("C_COMPLEX_OBJECT", "ELEMENT", "at0001", 2, -1))),
			want: []string{"at0001", "at0001"},
		},
		{
			// The first allowed child is a slot: the top-up takes the
			// first allowed child that is not a slot.
			name: "top-up from the first non-slot child after a slot",
			opt: optTemplate("CLUSTER", optCardinal("items", 2, -1,
				optOccurring("ARCHETYPE_SLOT", "ELEMENT", "at0001", 0, 1),
				optOccurring("C_COMPLEX_OBJECT", "ELEMENT", "at0002", 0, -1))),
			want: []string{"at0002", "at0002"},
		},
		{
			name: "top-up to cardinality lower 2 from one optional child",
			opt: optTemplate("CLUSTER", optCardinal("items", 2, -1,
				optOccurring("C_COMPLEX_OBJECT", "ELEMENT", "at0001", 0, -1))),
			want: []string{"at0001", "at0001"},
		},
		{
			// The top-up adds two members, not one.
			name: "top-up to cardinality lower 3 from one optional child",
			opt: optTemplate("CLUSTER", optCardinal("items", 3, -1,
				optOccurring("C_COMPLEX_OBJECT", "ELEMENT", "at0001", 0, -1))),
			want: []string{"at0001", "at0001", "at0001"},
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
					if got := itemIDs(t, out); !slices.Equal(got, tc.want) {
						t.Errorf("items = %v, want %v", got, tc.want)
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

// TestREQ107_FirstAllowedAlternativeWins is the REQ-107 check that a
// C_SINGLE_ATTRIBUTE with two allowed alternatives of different RM types is
// built from the first, in OPT order, under both policies, both value
// fills and both compile modes.
func TestREQ107_FirstAllowedAlternativeWins(t *testing.T) {
	for _, order := range [][2]string{{"DV_TEXT", "DV_COUNT"}, {"DV_COUNT", "DV_TEXT"}} {
		opt := optTemplate("ELEMENT", optSingle("value", optNode(order[0], ""), optNode(order[1], "")))
		for _, implicit := range []bool{true, false} {
			c := compileOPTText(t, opt, implicit)
			for _, opts := range defaultsOptions() {
				t.Run(fmt.Sprintf("%s,%s/implicit=%t/%v/%v", order[0], order[1], implicit, opts.Policy, opts.ValueFill), func(t *testing.T) {
					out, err := instance.Generate(t.Context(), c, opts)
					if err != nil {
						t.Fatalf("Generate: %v", err)
					}
					v := out.(*rm.Element).Value
					if v == nil || rm.IsTypedNil(v) {
						t.Fatalf("ELEMENT.value absent, want a %s", order[0])
					}
					if got := rmTypeName(v); got != order[0] {
						t.Errorf("ELEMENT.value is %s, want the first alternative %s", got, order[0])
					}
				})
			}
		}
	}
}

// TestREQ107_MinimalSkipsCollidingOptionalSiblings is the REQ-107 check
// that, under Minimal, an optional child whose node_id an earlier optional
// sibling shares gets no member, so validator node-id binding stays
// unambiguous, while Example gives each its member. The two siblings are
// ELEMENTs, not archetype roots, told apart by their value's RM type. It
// holds under both value fills and both compile modes.
func TestREQ107_MinimalSkipsCollidingOptionalSiblings(t *testing.T) {
	opt := optTemplate("CLUSTER", optCardinal("items", 1, -1,
		optOccurring("C_COMPLEX_OBJECT", "ELEMENT", "at0001", 0, 1, optSingle("value", optNode("DV_TEXT", ""))),
		optOccurring("C_COMPLEX_OBJECT", "ELEMENT", "at0001", 0, 1, optSingle("value", optNode("DV_COUNT", "")))))
	want := map[instance.Policy][]string{
		instance.Minimal: {"DV_TEXT"},
		instance.Example: {"DV_TEXT", "DV_COUNT"},
	}
	for _, implicit := range []bool{true, false} {
		c := compileOPTText(t, opt, implicit)
		for _, opts := range defaultsOptions() {
			t.Run(fmt.Sprintf("implicit=%t/%v/%v", implicit, opts.Policy, opts.ValueFill), func(t *testing.T) {
				out, err := instance.Generate(t.Context(), c, opts)
				if err != nil {
					t.Fatalf("Generate: %v", err)
				}
				var got []string
				for _, item := range out.(*rm.Cluster).Items {
					el, ok := item.(*rm.Element)
					if !ok || el.Value == nil {
						t.Fatalf("CLUSTER.items member is %#v, want an ELEMENT with a value", item)
					}
					got = append(got, rmTypeName(el.Value))
				}
				if !slices.Equal(got, want[opts.Policy]) {
					t.Errorf("CLUSTER.items values = %v, want %v", got, want[opts.Policy])
				}
				noFloorErrors(t, out)
			})
		}
	}
}
