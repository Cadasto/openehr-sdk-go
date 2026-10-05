package instance_test

import (
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

// visitCases are single-valued attributes the BMM does not mark mandatory.
// The OPT names each with no children, so only the visit rule decides
// whether the generator writes it, and present reports whether it did.
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
