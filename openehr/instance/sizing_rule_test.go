package instance_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
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
// holds under every policy, both value fills and both compile modes. A
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
		// prohibited is the path of the attribute the OPT prohibits and the
		// RM requires, which the generator fills anyway (REQ-107 §
		// Exceptions and known gaps): the template validator's one error
		// is prohibited there, on the template layer with the rule
		// existence (REQ-102 § Structural — prohibited attributes).
		prohibited string
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
			// The list breaks the prohibition (REQ-107), so the template
			// validator reports it as prohibited (REQ-102).
			name:       "CLUSTER items prohibited, an ELEMENT child",
			opt:        optTemplate("CLUSTER", optProhibitedMultiple("items", optNode("ELEMENT", "at0001"))),
			want:       []string{"at0001"},
			valid:      true,
			prohibited: "/items",
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
					prohibited := 0
					for _, iss := range templateErrors(out, c) {
						if isProhibitedAt(iss, tc.prohibited) {
							prohibited++
							continue
						}
						t.Errorf("template validator: %s @ %s: %s", iss.Code, iss.Path, iss.Detail)
					}
					wantProhibited := 0
					if tc.prohibited != "" {
						wantProhibited = 1
					}
					if prohibited != wantProhibited {
						t.Errorf("template validator: %d prohibited (template, existence) @ %q, want %d: REQ-107 fills the list the RM needs, and REQ-102 reports it", prohibited, tc.prohibited, wantProhibited)
					}
				})
			}
		}
	}
}

// TestREQ107_ProhibitedAlternativeIsSkipped is the REQ-107 check that a
// C_SINGLE_ATTRIBUTE skips an alternative whose occurrences upper bound is
// 0: the next allowed one wins, and with none allowed the OPT prohibits
// the attribute, so no policy visits an optional one.
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
			want: map[instance.Policy]string{instance.Large: "DV_COUNT", instance.Medium: "DV_COUNT", instance.Required: "DV_COUNT"},
		},
		{
			// Every alternative is prohibited, so the OPT prohibits the
			// optional attribute: no policy visits it.
			name: "no allowed alternative: prohibited",
			attr: optOptionalSingleOver("value", optOccurring("C_COMPLEX_OBJECT", "DV_COUNT", "", 0, 0)),
			want: map[instance.Policy]string{instance.Large: "", instance.Medium: "", instance.Required: ""},
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

// membersUnderEveryPolicy is n members under each of the three policies.
func membersUnderEveryPolicy(n int) map[instance.Policy]int {
	return map[instance.Policy]int{instance.Large: n, instance.Medium: n, instance.Required: n}
}

// TestREQ107_SilentRequiredMultipleIsToppedUp is the REQ-107 check that a
// multi-valued attribute the OPT names without children, whose cardinality
// lower bound is above 1, is topped up to that bound with members built
// from its BMM type, and to no more than its cardinality upper bound,
// except the member the BMM requires (REQ-107 Precedence 2), under each
// policy that visits it. Where only that cardinality asks for a member, no
// policy visits such a list: Required leaves it out by its own rule, and
// Large and Medium because every member it would get is undescribed
// (REQ-107 Visit rule).
func TestREQ107_SilentRequiredMultipleIsToppedUp(t *testing.T) {
	cases := []struct {
		name string
		opt  string
		// want is the number of members, per policy.
		want map[instance.Policy]int
	}{
		{name: "CLUSTER items, lower 2", opt: optTemplate("CLUSTER", optCardinal("items", 2, -1)), want: membersUnderEveryPolicy(2)},
		// The RM leaves an ITEM_TREE's items optional and the OPT names no
		// child, so only the cardinality asks for a member: a cardinality
		// lower bound counts the members of a list that exists and does not
		// make Required visit the list, nor make Large and Medium visit one
		// whose members, placeholder ELEMENTs built from the BMM type, the
		// template does not describe (REQ-107 Visit rule).
		{name: "ITEM_TREE items, lower 3", opt: optTemplate("ITEM_TREE", optCardinal("items", 3, -1)), want: membersUnderEveryPolicy(0)},
		// The existence requires the same list, so every policy visits it
		// and its cardinality tops it up.
		{name: "ITEM_TREE items, existence 1, lower 3", opt: optTemplate("ITEM_TREE", optRequiredCardinal("items", 3, -1)), want: membersUnderEveryPolicy(3)},
		// Existence 1..1 alone asks for no member, and cardinality 0..0
		// allows none, so the list stays empty.
		{name: "ITEM_TREE items, existence 1, upper 0", opt: optTemplate("ITEM_TREE", optRequiredCardinal("items", 0, 0)), want: membersUnderEveryPolicy(0)},
		// The BMM requires a CLUSTER's items, and cardinality 0..0 allows
		// none: the RM rule wins (REQ-107 Precedence 2), so the items get
		// one member, which breaks the OPT's upper bound.
		{name: "CLUSTER items, upper 0", opt: optTemplate("CLUSTER", optCardinal("items", 0, 0)), want: membersUnderEveryPolicy(1)},
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
					if got, want := len(itemIDs(t, out)), tc.want[opts.Policy]; got != want {
						t.Errorf("items has %d members, want %d", got, want)
					}
					noFloorErrors(t, out)
				})
			}
		}
	}
}

// TestREQ107_BMMMemberWinsOverCardinalityUpper is the REQ-107 Precedence 2
// check that a list the BMM requires a member in gets that member although
// the OPT's cardinality upper bound is 0, which breaks the OPT's bound
// (Exceptions and known gaps): a PERSON's identities at cardinality 0..0
// get one member, built from the BMM where the OPT names no child and from
// the first allowed child where it names one, and the root passes the RM
// floor. CLUSTER items at 0..0 are the other such list
// (TestREQ107_SilentRequiredMultipleIsToppedUp).
func TestREQ107_BMMMemberWinsOverCardinalityUpper(t *testing.T) {
	cases := []struct {
		name string
		opt  string
		// want is the archetype_node_id of the one identity; "" takes
		// any, for one built from the BMM.
		want string
	}{
		{name: "no child", opt: optTemplate("PERSON", optCardinal("identities", 0, 0))},
		{
			name: "optional child",
			opt: optTemplate("PERSON", optCardinal("identities", 0, 0,
				optOccurring("C_COMPLEX_OBJECT", "PARTY_IDENTITY", "at0001", 0, 1))),
			want: "at0001",
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
					p, ok := out.(*rm.Person)
					if !ok {
						t.Fatalf("generated root is %T, want a PERSON", out)
					}
					if got := len(p.Identities); got != 1 {
						t.Fatalf("identities has %d members, want 1", got)
					}
					if got := p.Identities[0].ArchetypeNodeID; tc.want != "" && got != tc.want {
						t.Errorf("identity archetype_node_id is %q, want %q", got, tc.want)
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
// each OPT child under Large and Medium, and occurrences.lower under
// Required, in OPT order, and no more in all than its cardinality upper
// bound, and is topped up to its cardinality lower bound with members of
// its first child. The cases build the same members under every policy,
// both value fills and both compile modes, and the template validator
// finds no error.
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
// built from the first, in OPT order, under every policy, both value
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

// TestREQ107_MediumSkipsACollidingOptionalSibling is the REQ-107 check
// that, under Medium, an optional child whose identity an earlier optional
// sibling shares gets no member, while under Large each gets its own
// (REQ-107 What a visit writes). The two siblings are ELEMENTs, whose
// identity is their node_id, told apart by their value's RM type, so the
// members show which were built. The root passes the template validator
// under both policies, which binds each member to the sibling that built
// it (REQ-102 Structural — binding items). It holds under both value fills
// and both compile modes.
func TestREQ107_MediumSkipsACollidingOptionalSibling(t *testing.T) {
	opt := optTemplate("CLUSTER", optCardinal("items", 1, -1,
		optOccurring("C_COMPLEX_OBJECT", "ELEMENT", "at0001", 0, 1, optSingle("value", optNode("DV_TEXT", ""))),
		optOccurring("C_COMPLEX_OBJECT", "ELEMENT", "at0001", 0, 1, optSingle("value", optNode("DV_COUNT", "")))))
	wants := map[instance.Policy][]string{instance.Large: {"DV_TEXT", "DV_COUNT"}, instance.Medium: {"DV_TEXT"}}
	for _, implicit := range []bool{true, false} {
		c := compileOPTText(t, opt, implicit)
		// Required gives an optional child no member of its own, which
		// TestREQ107_RequiredSizesMembersByOccurrencesLower pins.
		for _, opts := range optionsFor(largeAndMedium) {
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
				if want := wants[opts.Policy]; !slices.Equal(got, want) {
					t.Errorf("CLUSTER.items values = %v, want %v", got, want)
				}
				noFloorErrors(t, out)
				checkTemplateValidator(t, out, c)
			})
		}
	}
}

// TestREQ107_KeepsTheFirstOptionalSiblingOfEachIdentity is the REQ-107
// check that, under Medium, an optional sibling yields to a required one
// with its identity and to an earlier optional one with it; the identity is
// the one the validator binds a member by: the node_id for an ELEMENT, the
// archetype id for an archetype root. So the optional at0001 ELEMENTs get
// no member beside the required one, while at0002, which no required
// sibling carries, keeps its first optional sibling, and the optional root
// of the second archetype gets none, since it follows the first optional
// archetype root. Under Large every sibling gets its member, in OPT order:
// the optional ELEMENTs beside the required one of their node_id, and both
// roots of one archetype. The root passes the template validator under both
// policies, which binds each member to the first sibling with room under
// which it validates (REQ-102 Structural — binding items). It holds under
// both value fills and both compile modes.
func TestREQ107_KeepsTheFirstOptionalSiblingOfEachIdentity(t *testing.T) {
	element := func(nodeID string, lower int, valueType string) string {
		return optOccurring("C_COMPLEX_OBJECT", "ELEMENT", nodeID, lower, 1, optSingle("value", optNode(valueType, "")))
	}
	const clusterA, clusterB = "openEHR-EHR-CLUSTER.sibling_a.v1", "openEHR-EHR-CLUSTER.sibling_b.v1"
	root := func(archetypeID string) string {
		return `<children xsi:type="C_ARCHETYPE_ROOT"><rm_type_name>CLUSTER</rm_type_name>` +
			`<occurrences>` + optInterval(0, 1) + `</occurrences><node_id>at0000</node_id>` +
			optCardinal("items", 1, -1, element("at0001", 1, "DV_TEXT")) +
			`<archetype_id><value>` + archetypeID + `</value></archetype_id></children>`
	}
	opt := optTemplate("CLUSTER", optCardinal("items", 1, -1,
		element("at0001", 1, "DV_BOOLEAN"),
		element("at0001", 0, "DV_TEXT"),
		root(clusterA),
		element("at0002", 0, "DV_DATE"),
		root(clusterB),
		element("at0001", 0, "DV_COUNT"),
		root(clusterA),
		element("at0002", 0, "DV_DURATION")))
	want := map[instance.Policy][]string{
		instance.Large:  {"DV_BOOLEAN", "DV_TEXT", clusterA, "DV_DATE", clusterB, "DV_COUNT", clusterA, "DV_DURATION"},
		instance.Medium: {"DV_BOOLEAN", clusterA, "DV_DATE"},
	}
	for _, implicit := range []bool{true, false} {
		c := compileOPTText(t, opt, implicit)
		// Required gives an optional child no member of its own, which
		// TestREQ107_RequiredSizesMembersByOccurrencesLower pins.
		for _, opts := range optionsFor(largeAndMedium) {
			t.Run(fmt.Sprintf("implicit=%t/%v/%v", implicit, opts.Policy, opts.ValueFill), func(t *testing.T) {
				out, err := instance.Generate(t.Context(), c, opts)
				if err != nil {
					t.Fatalf("Generate: %v", err)
				}
				var got []string
				for _, item := range out.(*rm.Cluster).Items {
					switch v := item.(type) {
					case *rm.Element:
						if v.Value == nil {
							t.Fatalf("CLUSTER.items member is %#v, want an ELEMENT with a value", item)
						}
						got = append(got, rmTypeName(v.Value))
					case *rm.Cluster:
						got = append(got, v.ArchetypeNodeID)
					default:
						t.Fatalf("CLUSTER.items member is %#v, want an ELEMENT or a CLUSTER", item)
					}
				}
				if !slices.Equal(got, want[opts.Policy]) {
					t.Errorf("CLUSTER.items members = %v, want %v", got, want[opts.Policy])
				}
				noFloorErrors(t, out)
				checkTemplateValidator(t, out, c)
			})
		}
	}
}

// TestREQ107_OptionalSiblingBesideARequiredOne is the REQ-107 check that,
// under Medium, an optional child whose identity a required sibling carries
// gets no member of its own, whether that sibling comes before or after it,
// while under Large it gets one, in OPT order. A slot with an ELEMENT's
// node_id does not count as its sibling, and optional roots of different
// archetypes each get theirs under Large and only the first gets one under
// Medium. The cases are archetype roots of one archetype under
// CLUSTER.items in both orders, ELEMENTs of one node_id, roots of two
// archetypes, and a slot before an optional ELEMENT of its node_id, which a
// required ELEMENT of another node_id precedes so that no top-up builds it;
// the siblings that share an identity differ in content, so the members
// show which of them was built. It holds under both value fills and both
// compile modes.
//
// Medium's skip leaves the top-up free. Where CLUSTER.items needs two
// members and an optional root or ELEMENT yields to a required one after
// it, the required child's member comes first, and the top-up then builds
// one of the optional child, the first allowed child with room that needs
// no undescribed content. Where the optional ELEMENT yields to a required
// one before it, the required one is full after its own member, so the
// top-up builds one of the optional ELEMENT, whose identity no earlier
// sibling with room carries. Under Large the two members are each
// sibling's own, so no top-up is needed.
//
// Where siblings share an identity and pin no name, the template validator
// binds each member to the first of them that has room and under which it
// validates (REQ-102 Structural — binding items), so the members built for
// siblings that differ in content bind to the children that built them and
// the root passes it. Under Medium, where the optional sibling comes first
// and its content is the required one's, the member built for the required
// one validates under the optional one too and binds there, so the root
// shows the known gap (checkTemplateValidator); under Large the optional
// sibling's own member fills its one occurrence, and the root passes.
func TestREQ107_OptionalSiblingBesideARequiredOne(t *testing.T) {
	const clusterA, clusterB = "openEHR-EHR-CLUSTER.sibling_a.v1", "openEHR-EHR-CLUSTER.sibling_b.v1"
	element := func(nodeID string, lower int, valueType string) string {
		return optOccurring("C_COMPLEX_OBJECT", "ELEMENT", nodeID, lower, 1, optSingle("value", optNode(valueType, "")))
	}
	root := func(archetypeID string, lower int, valueType string) string {
		return `<children xsi:type="C_ARCHETYPE_ROOT"><rm_type_name>CLUSTER</rm_type_name>` +
			`<occurrences>` + optInterval(lower, 1) + `</occurrences><node_id>at0000</node_id>` +
			optCardinal("items", 1, -1, element("at0001", 1, valueType)) +
			`<archetype_id><value>` + archetypeID + `</value></archetype_id></children>`
	}
	rootGap, elementGap := []string{"/items[" + clusterA + "]"}, []string{"/items[at0001]"}
	cases := []struct {
		name string
		// lower is the cardinality lower bound of CLUSTER.items.
		lower    int
		children []string
		// large and medium are the members built under each policy.
		large, medium []string
		// mediumGap holds the paths at which the root shows the known gap
		// under Medium; none where it passes the template validator, as
		// every root does under Large.
		mediumGap []string
	}{
		{
			name:     "required root then optional root",
			lower:    1,
			children: []string{root(clusterA, 1, "DV_TEXT"), root(clusterA, 0, "DV_COUNT")},
			large:    []string{clusterA + "[DV_TEXT]", clusterA + "[DV_COUNT]"},
			medium:   []string{clusterA + "[DV_TEXT]"},
		},
		{
			name:     "optional root then required root",
			lower:    1,
			children: []string{root(clusterA, 0, "DV_COUNT"), root(clusterA, 1, "DV_TEXT")},
			large:    []string{clusterA + "[DV_COUNT]", clusterA + "[DV_TEXT]"},
			medium:   []string{clusterA + "[DV_TEXT]"},
		},
		{
			// Under Medium the optional root admits the member built for
			// the required one, so the validator binds it there.
			name:      "optional root then required root of one content",
			lower:     1,
			children:  []string{root(clusterA, 0, "DV_TEXT"), root(clusterA, 1, "DV_TEXT")},
			large:     []string{clusterA + "[DV_TEXT]", clusterA + "[DV_TEXT]"},
			medium:    []string{clusterA + "[DV_TEXT]"},
			mediumGap: rootGap,
		},
		{
			name:     "required element then optional element",
			lower:    1,
			children: []string{element("at0001", 1, "DV_TEXT"), element("at0001", 0, "DV_COUNT")},
			large:    []string{"DV_TEXT", "DV_COUNT"},
			medium:   []string{"DV_TEXT"},
		},
		{
			name:     "optional roots of different archetypes",
			lower:    1,
			children: []string{root(clusterA, 0, "DV_TEXT"), root(clusterB, 0, "DV_COUNT")},
			large:    []string{clusterA + "[DV_TEXT]", clusterB + "[DV_COUNT]"},
			medium:   []string{clusterA + "[DV_TEXT]"},
		},
		{
			name:     "slot then optional element of its node_id",
			lower:    1,
			children: []string{element("at0002", 1, "DV_COUNT"), optSlot("ELEMENT", "at0001"), element("at0001", 0, "DV_TEXT")},
			large:    []string{"DV_COUNT", "DV_TEXT"},
			medium:   []string{"DV_COUNT", "DV_TEXT"},
		},
		{
			// Under Medium the top-up builds a member of the optional root
			// that yields to the required one.
			name:     "optional root then required root, cardinality lower 2",
			lower:    2,
			children: []string{root(clusterA, 0, "DV_COUNT"), root(clusterA, 1, "DV_TEXT")},
			large:    []string{clusterA + "[DV_COUNT]", clusterA + "[DV_TEXT]"},
			medium:   []string{clusterA + "[DV_TEXT]", clusterA + "[DV_COUNT]"},
		},
		{
			name:     "optional element then required element, cardinality lower 2",
			lower:    2,
			children: []string{element("at0001", 0, "DV_COUNT"), element("at0001", 1, "DV_TEXT")},
			large:    []string{"DV_COUNT", "DV_TEXT"},
			medium:   []string{"DV_TEXT", "DV_COUNT"},
		},
		{
			// Under Medium the optional ELEMENT admits the member built for
			// the required one, so the validator binds it there.
			name:      "optional element then required element of one content",
			lower:     1,
			children:  []string{element("at0001", 0, "DV_TEXT"), element("at0001", 1, "DV_TEXT")},
			large:     []string{"DV_TEXT", "DV_TEXT"},
			medium:    []string{"DV_TEXT"},
			mediumGap: elementGap,
		},
		{
			// Under Medium the optional ELEMENT yields to the required one
			// before it. The required one is full after its own member, so
			// the top-up builds a member of the optional one, which the
			// validator binds there, the first of the two with room.
			name:     "required element then optional element, cardinality lower 2",
			lower:    2,
			children: []string{element("at0001", 1, "DV_TEXT"), element("at0001", 0, "DV_COUNT")},
			large:    []string{"DV_TEXT", "DV_COUNT"},
			medium:   []string{"DV_TEXT", "DV_COUNT"},
		},
	}
	for _, tc := range cases {
		opt := optTemplate("CLUSTER", optCardinal("items", tc.lower, -1, tc.children...))
		for _, implicit := range []bool{true, false} {
			c := compileOPTText(t, opt, implicit)
			// Required gives an optional child no member of its own, which
			// TestREQ107_RequiredSizesMembersByOccurrencesLower pins.
			for _, opts := range optionsFor(largeAndMedium) {
				t.Run(fmt.Sprintf("%s/implicit=%t/%v/%v", tc.name, implicit, opts.Policy, opts.ValueFill), func(t *testing.T) {
					out, err := instance.Generate(t.Context(), c, opts)
					if err != nil {
						t.Fatalf("Generate: %v", err)
					}
					want, gap := tc.large, []string(nil)
					if opts.Policy == instance.Medium {
						want, gap = tc.medium, tc.mediumGap
					}
					if got := memberLabels(t, out); !slices.Equal(got, want) {
						t.Errorf("CLUSTER.items members = %v, want %v", got, want)
					}
					noFloorErrors(t, out)
					checkTemplateValidator(t, out, c, gap...)
				})
			}
		}
	}
}

// TestREQ107_ArchetypeRootCarriesItsNodeID is the REQ-107 check that an
// archetype root carries its node_id as well as its archetype id, as the
// template validator binds a LOCATABLE member (REQ-102 Structural — binding
// items). Under Medium, an optional ELEMENT whose node_id is the at0000 of
// an archetype root before it, or of a required root after it, gets no
// member, while an optional ELEMENT before an optional root keeps its
// member: the ELEMENT does not carry the root's archetype id. Under Large
// the optional ELEMENT gets its member in each case. Where both get their
// members, the ELEMENT's member, which carries the root's node_id, binds to
// the ELEMENT, since the root has no room left or does not admit it, so the
// root passes the template validator (REQ-102 Structural — binding items).
// It holds under both value fills and both compile modes.
func TestREQ107_ArchetypeRootCarriesItsNodeID(t *testing.T) {
	const clusterA = "openEHR-EHR-CLUSTER.sibling_a.v1"
	element := func(lower int) string {
		return optOccurring("C_COMPLEX_OBJECT", "ELEMENT", "at0000", lower, 1, optSingle("value", optNode("DV_COUNT", "")))
	}
	root := func(lower int) string {
		return `<children xsi:type="C_ARCHETYPE_ROOT"><rm_type_name>CLUSTER</rm_type_name>` +
			`<occurrences>` + optInterval(lower, 1) + `</occurrences><node_id>at0000</node_id>` +
			optCardinal("items", 1, -1, optOccurring("C_COMPLEX_OBJECT", "ELEMENT", "at0001", 1, 1,
				optSingle("value", optNode("DV_TEXT", "")))) +
			`<archetype_id><value>` + clusterA + `</value></archetype_id></children>`
	}
	rootMember := clusterA + "[DV_TEXT]"
	cases := []struct {
		name          string
		children      []string
		large, medium []string
	}{
		{
			name:     "optional root then optional element",
			children: []string{root(0), element(0)},
			large:    []string{rootMember, "DV_COUNT"},
			medium:   []string{rootMember},
		},
		{
			name:     "required root then optional element",
			children: []string{root(1), element(0)},
			large:    []string{rootMember, "DV_COUNT"},
			medium:   []string{rootMember},
		},
		{
			name:     "optional element then required root",
			children: []string{element(0), root(1)},
			large:    []string{"DV_COUNT", rootMember},
			medium:   []string{rootMember},
		},
		{
			name:     "optional element then optional root",
			children: []string{element(0), root(0)},
			large:    []string{"DV_COUNT", rootMember},
			medium:   []string{"DV_COUNT", rootMember},
		},
		{
			name:     "optional root then required element",
			children: []string{root(0), element(1)},
			large:    []string{rootMember, "DV_COUNT"},
			medium:   []string{rootMember, "DV_COUNT"},
		},
	}
	for _, tc := range cases {
		opt := optTemplate("CLUSTER", optCardinal("items", 1, -1, tc.children...))
		for _, implicit := range []bool{true, false} {
			c := compileOPTText(t, opt, implicit)
			// Required gives an optional child no member of its own, which
			// TestREQ107_RequiredSizesMembersByOccurrencesLower pins.
			for _, opts := range optionsFor(largeAndMedium) {
				t.Run(fmt.Sprintf("%s/implicit=%t/%v/%v", tc.name, implicit, opts.Policy, opts.ValueFill), func(t *testing.T) {
					out, err := instance.Generate(t.Context(), c, opts)
					if err != nil {
						t.Fatalf("Generate: %v", err)
					}
					want := tc.large
					if opts.Policy == instance.Medium {
						want = tc.medium
					}
					if got := memberLabels(t, out); !slices.Equal(got, want) {
						t.Errorf("CLUSTER.items members = %v, want %v", got, want)
					}
					noFloorErrors(t, out)
					checkTemplateValidator(t, out, c)
				})
			}
		}
	}
}

// TestREQ107_NeverSkipsAChildWithNoIdentity is the REQ-107 check that a
// child with neither an archetype id nor a node_id has no identity, so no
// sibling holds it and neither Large nor Medium skips it: two optional
// ELEMENTs with no node_id under CLUSTER.items each get a member, under
// both policies. Their values differ in RM type, so the members show that
// both were built. It holds under both value fills and both compile modes.
// The template validator is not consulted: it binds a LOCATABLE member by
// an archetype id or a node_id its OPT child carries (REQ-102), so it binds
// neither member here, whatever the generator builds.
func TestREQ107_NeverSkipsAChildWithNoIdentity(t *testing.T) {
	element := func(valueType string) string {
		return optOccurring("C_COMPLEX_OBJECT", "ELEMENT", "", 0, 1, optSingle("value", optNode(valueType, "")))
	}
	opt := optTemplate("CLUSTER", optCardinal("items", 1, -1, element("DV_TEXT"), element("DV_COUNT")))
	want := []string{"DV_TEXT", "DV_COUNT"}
	for _, implicit := range []bool{true, false} {
		c := compileOPTText(t, opt, implicit)
		// Required gives an optional child no member of its own, which
		// TestREQ107_RequiredSizesMembersByOccurrencesLower pins.
		for _, opts := range optionsFor(largeAndMedium) {
			t.Run(fmt.Sprintf("implicit=%t/%v/%v", implicit, opts.Policy, opts.ValueFill), func(t *testing.T) {
				out, err := instance.Generate(t.Context(), c, opts)
				if err != nil {
					t.Fatalf("Generate: %v", err)
				}
				if got := memberLabels(t, out); !slices.Equal(got, want) {
					t.Errorf("CLUSTER.items members = %v, want %v: a child with no identity is never skipped", got, want)
				}
				noFloorErrors(t, out)
			})
		}
	}
}

// TestREQ107_MediumBuildsOneOptionalArchetypePerAttribute is the REQ-107
// check that, under Medium, an optional archetype root after the first
// optional archetype root of a multi-valued attribute, in OPT order, gets
// no member of its own, whatever its archetype, while Large builds a member
// of each optional archetype and Required builds none. The rule binds
// archetype roots only: an optional ELEMENT between the two roots keeps
// its member under Medium. Where the first optional root yields to a
// required root of its archetype, it still counts as the first, so Medium
// builds no optional archetype at all, while Large builds both optional
// roots beside the required one. Each root passes the template
// validator and the RM floor. It holds under both value fills and both
// compile modes.
func TestREQ107_MediumBuildsOneOptionalArchetypePerAttribute(t *testing.T) {
	const clusterA, clusterB = "openEHR-EHR-CLUSTER.sibling_a.v1", "openEHR-EHR-CLUSTER.sibling_b.v1"
	element := func(nodeID string, lower int, valueType string) string {
		return optOccurring("C_COMPLEX_OBJECT", "ELEMENT", nodeID, lower, 1, optSingle("value", optNode(valueType, "")))
	}
	root := func(archetypeID string, lower int, valueType string) string {
		return `<children xsi:type="C_ARCHETYPE_ROOT"><rm_type_name>CLUSTER</rm_type_name>` +
			`<occurrences>` + optInterval(lower, 1) + `</occurrences><node_id>at0000</node_id>` +
			optCardinal("items", 1, -1, element("at0001", 1, valueType)) +
			`<archetype_id><value>` + archetypeID + `</value></archetype_id></children>`
	}
	cases := []struct {
		name     string
		children []string
		want     map[instance.Policy][]string
	}{
		{
			name: "two optional roots of different archetypes around an optional element",
			children: []string{
				element("at0001", 1, "DV_TEXT"),
				root(clusterA, 0, "DV_BOOLEAN"),
				element("at0002", 0, "DV_COUNT"),
				root(clusterB, 0, "DV_DATE"),
			},
			want: map[instance.Policy][]string{
				instance.Large:    {"DV_TEXT", clusterA + "[DV_BOOLEAN]", "DV_COUNT", clusterB + "[DV_DATE]"},
				instance.Medium:   {"DV_TEXT", clusterA + "[DV_BOOLEAN]", "DV_COUNT"},
				instance.Required: {"DV_TEXT"},
			},
		},
		{
			name: "the first optional root yields to a required root of its archetype",
			children: []string{
				root(clusterA, 1, "DV_TEXT"),
				root(clusterA, 0, "DV_BOOLEAN"),
				root(clusterB, 0, "DV_DATE"),
			},
			want: map[instance.Policy][]string{
				instance.Large:    {clusterA + "[DV_TEXT]", clusterA + "[DV_BOOLEAN]", clusterB + "[DV_DATE]"},
				instance.Medium:   {clusterA + "[DV_TEXT]"},
				instance.Required: {clusterA + "[DV_TEXT]"},
			},
		},
	}
	for _, tc := range cases {
		opt := optTemplate("CLUSTER", optCardinal("items", 1, -1, tc.children...))
		for _, implicit := range []bool{true, false} {
			c := compileOPTText(t, opt, implicit)
			for _, opts := range defaultsOptions() {
				t.Run(fmt.Sprintf("%s/implicit=%t/%v/%v", tc.name, implicit, opts.Policy, opts.ValueFill), func(t *testing.T) {
					out, err := instance.Generate(t.Context(), c, opts)
					if err != nil {
						t.Fatalf("Generate: %v", err)
					}
					if got, want := memberLabels(t, out), tc.want[opts.Policy]; !slices.Equal(got, want) {
						t.Errorf("CLUSTER.items members = %v, want %v", got, want)
					}
					noFloorErrors(t, out)
					checkTemplateValidator(t, out, c)
				})
			}
		}
	}
}

// TestREQ107_ArchetypeRootWithNoOccurrencesIsNotOptional is the REQ-107
// check that an archetype root the OPT gives no occurrences is not an
// optional child, since it has no occurrences lower bound of 0. Every
// policy sizes it as one member, and Medium's one-optional-archetype rule
// does not drop it: two such roots of different archetypes under
// CLUSTER.items each get one member under Large, Medium and Required. The
// root passes the template validator and the RM floor. It holds under both
// value fills and both compile modes.
func TestREQ107_ArchetypeRootWithNoOccurrencesIsNotOptional(t *testing.T) {
	const clusterA, clusterB = "openEHR-EHR-CLUSTER.sibling_a.v1", "openEHR-EHR-CLUSTER.sibling_b.v1"
	root := func(archetypeID, valueType string) string {
		return optArchetypeRoot("CLUSTER", archetypeID, optCardinal("items", 1, -1,
			optOccurring("C_COMPLEX_OBJECT", "ELEMENT", "at0001", 1, 1, optSingle("value", optNode(valueType, "")))))
	}
	opt := optTemplate("CLUSTER", optCardinal("items", 1, -1, root(clusterA, "DV_TEXT"), root(clusterB, "DV_COUNT")))
	want := []string{clusterA + "[DV_TEXT]", clusterB + "[DV_COUNT]"}
	for _, implicit := range []bool{true, false} {
		c := compileOPTText(t, opt, implicit)
		for _, opts := range defaultsOptions() {
			t.Run(fmt.Sprintf("implicit=%t/%v/%v", implicit, opts.Policy, opts.ValueFill), func(t *testing.T) {
				out, err := instance.Generate(t.Context(), c, opts)
				if err != nil {
					t.Fatalf("Generate: %v", err)
				}
				if got := memberLabels(t, out); !slices.Equal(got, want) {
					t.Errorf("CLUSTER.items members = %v, want %v", got, want)
				}
				noFloorErrors(t, out)
				checkTemplateValidator(t, out, c)
			})
		}
	}
}

// sharedIdentityGap names the REQ-107 known gap for an optional sibling
// that carries a later, required one's identity, where neither pins a name
// (§ Exceptions and known gaps).
const sharedIdentityGap = "REQ-107 known gap, an optional sibling that carries a later, required one's identity, where neither pins a name"

// checkTemplateValidator checks the template validator's verdict on out.
// With no gaps, out must pass it: every error is reported. Otherwise out
// holds an optional sibling that carries a later, required one's identity,
// where neither pins a name; under Medium the generator built no member of
// the optional one, and the validator binds the member built for the
// required one to the optional one, under which it validates too
// (sharedIdentityGap). Each path in gaps must carry the cardinality issue
// that shows it, and the other errors that binding causes are expected.
// The day the generator builds a member of each, the check fails, and the
// case should then expect out to pass.
func checkTemplateValidator(t *testing.T, out any, c *templatecompile.Compiled, gaps ...string) {
	t.Helper()
	errs := templateErrors(out, c)
	if len(gaps) == 0 {
		for _, iss := range errs {
			t.Errorf("Validate: %s @ %s: %s", iss.Code, iss.Path, iss.Detail)
		}
		return
	}
	for _, path := range gaps {
		if !hasCardinalityAt(errs, path) {
			t.Errorf("%s: Validate reports %s, want a cardinality issue at %s; if the generator now builds a member of each sibling, expect the root to pass",
				sharedIdentityGap, issueSummary(errs), path)
		}
	}
}

// hasCardinalityAt reports whether errs holds the template validator's
// report of a child's occurrences broken at path: code cardinality, on the
// template layer, rule occurrences (REQ-102).
func hasCardinalityAt(errs []validation.Issue, path string) bool {
	return slices.ContainsFunc(errs, func(iss validation.Issue) bool {
		return iss.Path == path && iss.Code == "cardinality" &&
			iss.Layer == validation.LayerTemplate && iss.Rule == "occurrences"
	})
}

// issueSummary lists each issue in errs by code and path, or says there is
// none.
func issueSummary(errs []validation.Issue) string {
	if len(errs) == 0 {
		return "no error"
	}
	parts := make([]string, 0, len(errs))
	for _, iss := range errs {
		parts = append(parts, iss.Code+" @ "+iss.Path)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

// memberLabels names each member of a generated CLUSTER's items: an
// ELEMENT by its value's RM type, a CLUSTER by its archetype_node_id
// followed by the labels of its own members in brackets.
func memberLabels(t *testing.T, out any) []string {
	t.Helper()
	cluster, ok := out.(*rm.Cluster)
	if !ok {
		t.Fatalf("generated value is %T, want a CLUSTER", out)
	}
	var got []string
	for _, item := range cluster.Items {
		switch v := item.(type) {
		case *rm.Element:
			if v.Value == nil {
				t.Fatalf("CLUSTER.items member is %#v, want an ELEMENT with a value", item)
			}
			got = append(got, rmTypeName(v.Value))
		case *rm.Cluster:
			got = append(got, v.ArchetypeNodeID+"["+strings.Join(memberLabels(t, v), ",")+"]")
		default:
			t.Fatalf("CLUSTER.items member is %#v, want an ELEMENT or a CLUSTER", item)
		}
	}
	return got
}
