package instance_test

import (
	"fmt"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
)

// optOptionalSingle is a C_SINGLE_ATTRIBUTE called name that the OPT names
// with no children and an existence of 0..1: the attribute is optional.
func optOptionalSingle(name string) string {
	return `<attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>` + name + `</rm_attribute_name>` +
		`<existence><lower_included>true</lower_included><upper_included>true</upper_included>` +
		`<lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded>` +
		`<lower>0</lower><upper>1</upper></existence></attributes>`
}

// visitCases are single-valued attributes the BMM does not mark mandatory,
// structures and data values alike. The OPT names each with no children,
// so only the visit rule decides whether the generator writes it, and
// present reports whether it did.
var visitCases = []struct {
	name    string
	root    string
	attr    string
	present func(out any) bool
}{
	{
		name: "OBSERVATION protocol",
		root: "OBSERVATION",
		attr: "protocol",
		present: func(out any) bool {
			p := out.(*rm.Observation).Protocol
			return p != nil && !rm.IsTypedNil(p)
		},
	},
	{
		name: "OBSERVATION provider",
		root: "OBSERVATION",
		attr: "provider",
		present: func(out any) bool {
			p := out.(*rm.Observation).Provider
			return p != nil && !rm.IsTypedNil(p)
		},
	},
	{
		// A data value: a DV_DATE_TIME.
		name:    "INSTRUCTION expiry_time",
		root:    "INSTRUCTION",
		attr:    "expiry_time",
		present: func(out any) bool { return out.(*rm.Instruction).ExpiryTime != nil },
	},
	{
		// A data value: a DV_TEXT, which an ELEMENT with no value keeps.
		name:    "ELEMENT null_reason",
		root:    "ELEMENT",
		attr:    "null_reason",
		present: func(out any) bool { return out.(*rm.Element).NullReason != nil },
	},
}

// TestREQ107_ExampleVisitsOptionalAttributeMinimalDoesNot is the REQ-107
// check of the visit rule for an attribute outside Minimal's cases: not
// BMM-mandatory, no OPT children, existence lower 0, and single-valued, so
// it has no cardinality. Example MUST visit it, which writes a value, and
// Minimal MUST NOT materialise it.
func TestREQ107_ExampleVisitsOptionalAttributeMinimalDoesNot(t *testing.T) {
	for _, tc := range visitCases {
		t.Run(tc.name, func(t *testing.T) {
			c := compileOPTText(t, optTemplate(tc.root, optOptionalSingle(tc.attr)), true)
			requireAttribute(t, c, tc.attr, 0)
			for _, policy := range []instance.Policy{instance.Minimal, instance.Example} {
				out, err := instance.Generate(t.Context(), c, instance.Options{Policy: policy, Now: defaultsNow})
				if err != nil {
					t.Fatalf("Generate(%v): %v", policy, err)
				}
				want := policy == instance.Example
				if got := tc.present(out); got != want {
					t.Errorf("Generate(%v): %s.%s present = %t, want %t", policy, tc.root, tc.attr, got, want)
				}
			}
		})
	}
}

// TestREQ107_MinimalMaterialisesExistenceLowerOne is the REQ-107 check that
// Minimal materialises an attribute whose existence lower bound is 1 when
// nothing else would make it: the attribute is not BMM-mandatory, has no
// OPT children, and is single-valued, so it has no cardinality.
func TestREQ107_MinimalMaterialisesExistenceLowerOne(t *testing.T) {
	for _, tc := range visitCases {
		t.Run(tc.name, func(t *testing.T) {
			c := compileOPTText(t, optTemplate(tc.root, optSingle(tc.attr)), true)
			requireAttribute(t, c, tc.attr, 1)
			for _, policy := range []instance.Policy{instance.Minimal, instance.Example} {
				out, err := instance.Generate(t.Context(), c, instance.Options{Policy: policy, Now: defaultsNow})
				if err != nil {
					t.Fatalf("Generate(%v): %v", policy, err)
				}
				if !tc.present(out) {
					t.Errorf("Generate(%v): %s.%s absent, want it materialised", policy, tc.root, tc.attr)
				}
			}
		})
	}
}

// requireAttribute stops the test unless the compiled root's attribute
// name has the shape the visit-rule tests rely on: not BMM-mandatory, no
// OPT children, no cardinality, and the given existence lower bound.
func requireAttribute(t *testing.T, c *templatecompile.Compiled, name string, existenceLower int) {
	t.Helper()
	attr := c.Root().Attribute(name)
	if attr == nil {
		t.Fatalf("compiled root %s has no attribute %q", c.Root().RMTypeName(), name)
	}
	if attr.Required() {
		t.Fatalf("%s is BMM-mandatory, want an optional attribute", name)
	}
	if n := len(attr.Children()); n != 0 {
		t.Fatalf("%s has %d OPT children, want none", name, n)
	}
	if attr.ChildMultiplicity() != nil {
		t.Fatalf("%s has a cardinality, want none", name)
	}
	e := attr.Existence()
	if e == nil || e.LowerUnbounded() || e.Lower() != existenceLower {
		t.Fatalf("%s existence = %+v, want lower %d", name, e, existenceLower)
	}
}

// TestREQ107_OptionalSilentEventsGetNoMember is the REQ-107 check that a
// multi-valued attribute that is not archetype-rooted, optional, and named
// by the OPT without children gets no member, also under Example, which
// visits it: an optional attribute the OPT leaves silent MUST get no child.
// HISTORY.events is optional in the RM, and its member would be an event,
// not an archetype root. It holds in both compile modes.
func TestREQ107_OptionalSilentEventsGetNoMember(t *testing.T) {
	opt := optTemplate("OBSERVATION", optSingle("data", optNode("HISTORY", "at0001", optOptionalMultiple("events"))))
	for _, implicit := range []bool{true, false} {
		c := compileOPTText(t, opt, implicit)
		for _, opts := range defaultsOptions() {
			t.Run(fmt.Sprintf("implicit=%t/%v/%v", implicit, opts.Policy, opts.ValueFill), func(t *testing.T) {
				out, err := instance.Generate(t.Context(), c, opts)
				if err != nil {
					t.Fatalf("Generate: %v", err)
				}
				if events := out.(*rm.Observation).Data.Events; len(events) != 0 {
					t.Errorf("HISTORY.events has %d members, want none", len(events))
				}
			})
		}
	}
}

// TestREQ107_UnboundedLowerBoundIsNotRequired is the REQ-107 check that an
// existence or cardinality lower bound the OPT marks unbounded does not
// make an attribute required, whatever number the OPT puts beside it: the
// visit rule counts a lower bound only when it is bounded and at least 1.
// Minimal does not visit an optional OBSERVATION protocol whose existence
// lower bound is unbounded with a lower of 1, and Example, which visits
// it, writes it from its BMM type; neither policy gives a silent optional
// ITEM_TREE items list whose cardinality lower bound is unbounded with a
// lower of 2 any member. It holds under both value fills and both compile
// modes.
func TestREQ107_UnboundedLowerBoundIsNotRequired(t *testing.T) {
	const openLower = `<lower_included>true</lower_included><upper_included>true</upper_included>` +
		`<lower_unbounded>true</lower_unbounded>`
	protocol := `<attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>protocol</rm_attribute_name>` +
		`<existence>` + openLower + `<upper_unbounded>false</upper_unbounded><lower>1</lower><upper>1</upper></existence></attributes>`
	items := `<attributes xsi:type="C_MULTIPLE_ATTRIBUTE"><rm_attribute_name>items</rm_attribute_name>` +
		`<existence><lower_included>true</lower_included><upper_included>true</upper_included>` +
		`<lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded><lower>0</lower><upper>1</upper></existence>` +
		`<cardinality><is_ordered>false</is_ordered><is_unique>false</is_unique><interval>` +
		`<lower_included>true</lower_included><lower_unbounded>true</lower_unbounded><upper_unbounded>true</upper_unbounded>` +
		`<lower>2</lower></interval></cardinality></attributes>`
	cases := []struct {
		name  string
		opt   string
		check func(t *testing.T, policy instance.Policy, out any)
	}{
		{
			name: "existence: OBSERVATION protocol",
			opt:  optTemplate("OBSERVATION", protocol),
			check: func(t *testing.T, policy instance.Policy, out any) {
				p := out.(*rm.Observation).Protocol
				present := p != nil && !rm.IsTypedNil(p)
				if want := policy == instance.Example; present != want {
					t.Errorf("OBSERVATION.protocol present = %t, want %t", present, want)
				}
			},
		},
		{
			name: "cardinality: ITEM_TREE items",
			opt:  optTemplate("ITEM_TREE", items),
			check: func(t *testing.T, _ instance.Policy, out any) {
				if n := len(out.(*rm.ItemTree).Items); n != 0 {
					t.Errorf("ITEM_TREE.items has %d members, want none", n)
				}
			},
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
					tc.check(t, opts.Policy, out)
				})
			}
		}
	}
}
