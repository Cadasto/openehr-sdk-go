package instance_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// prohibitedExistence is an existence of 0..0: the OPT prohibits the
// attribute.
const prohibitedExistence = `<existence><lower_included>true</lower_included><upper_included>true</upper_included>` +
	`<lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded>` +
	`<lower>0</lower><upper>0</upper></existence>`

// optProhibitedSingle is a C_SINGLE_ATTRIBUTE called name that the OPT
// prohibits, over children.
func optProhibitedSingle(name string, children ...string) string {
	return `<attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>` + name + `</rm_attribute_name>` +
		prohibitedExistence + strings.Join(children, "") + `</attributes>`
}

// optProhibitedMultiple is a C_MULTIPLE_ATTRIBUTE called name that the OPT
// prohibits, over children, with no lower bound on its cardinality.
func optProhibitedMultiple(name string, children ...string) string {
	return `<attributes xsi:type="C_MULTIPLE_ATTRIBUTE"><rm_attribute_name>` + name + `</rm_attribute_name>` +
		prohibitedExistence + strings.Join(children, "") +
		`<cardinality><is_ordered>false</is_ordered><is_unique>false</is_unique><interval>` +
		`<lower_included>true</lower_included><lower_unbounded>false</lower_unbounded>` +
		`<upper_unbounded>true</upper_unbounded><lower>0</lower></interval></cardinality></attributes>`
}

// TestREQ107_ProhibitedAttributeIsNotVisited is the REQ-107 check that
// Generate does not visit an attribute the OPT prohibits with an existence
// of 0..0, under either policy, either value fill and either compile mode:
// a single-valued or a multi-valued one, with or without children the OPT
// names under it, gets nothing.
func TestREQ107_ProhibitedAttributeIsNotVisited(t *testing.T) {
	cases := []struct {
		name    string
		opt     string
		present func(out any) bool
	}{
		{
			name: "OBSERVATION protocol, no children",
			opt:  optTemplate("OBSERVATION", optProhibitedSingle("protocol")),
			present: func(out any) bool {
				p := out.(*rm.Observation).Protocol
				return p != nil && !rm.IsTypedNil(p)
			},
		},
		{
			name: "OBSERVATION protocol, an ITEM_TREE child",
			opt:  optTemplate("OBSERVATION", optProhibitedSingle("protocol", optNode("ITEM_TREE", "at0001"))),
			present: func(out any) bool {
				p := out.(*rm.Observation).Protocol
				return p != nil && !rm.IsTypedNil(p)
			},
		},
		{
			name: "OBSERVATION provider, no children",
			opt:  optTemplate("OBSERVATION", optProhibitedSingle("provider")),
			present: func(out any) bool {
				p := out.(*rm.Observation).Provider
				return p != nil && !rm.IsTypedNil(p)
			},
		},
		{
			name:    "ITEM_TREE items, no children",
			opt:     optTemplate("ITEM_TREE", optProhibitedMultiple("items")),
			present: func(out any) bool { return len(out.(*rm.ItemTree).Items) > 0 },
		},
		{
			name:    "ITEM_TREE items, an ELEMENT child",
			opt:     optTemplate("ITEM_TREE", optProhibitedMultiple("items", optNode("ELEMENT", "at0001"))),
			present: func(out any) bool { return len(out.(*rm.ItemTree).Items) > 0 },
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
					if tc.present(out) {
						t.Errorf("the prohibited attribute was materialised, want nothing")
					}
				})
			}
		}
	}
}
