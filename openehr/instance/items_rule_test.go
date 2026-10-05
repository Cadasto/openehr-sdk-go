package instance_test

import (
	"fmt"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// optRequiredMultiple is a C_MULTIPLE_ATTRIBUTE called name with no
// children whose existence is 1..1 and whose cardinality lower bound is 1:
// the OPT requires a member and describes none.
func optRequiredMultiple(name string) string {
	return `<attributes xsi:type="C_MULTIPLE_ATTRIBUTE"><rm_attribute_name>` + name + `</rm_attribute_name>` +
		`<existence><lower_included>true</lower_included><upper_included>true</upper_included>` +
		`<lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded>` +
		`<lower>1</lower><upper>1</upper></existence>` +
		`<cardinality><is_ordered>false</is_ordered><is_unique>false</is_unique><interval>` +
		`<lower_included>true</lower_included><lower_unbounded>false</lower_unbounded>` +
		`<upper_unbounded>true</upper_unbounded><lower>1</lower></interval></cardinality></attributes>`
}

// itemsVariants are the ways an OPT can leave ITEM_TREE.items or
// ITEM_LIST.items without a member: it does not name the attribute, it
// names it optional with no children, or it names it optional with only
// an optional ELEMENT slot, which the walk leaves to the caller.
var itemsVariants = []struct {
	name  string
	attrs []string
}{
	{name: "no items attribute"},
	{name: "optional childless items", attrs: []string{optOptionalMultiple("items")}},
	{name: "optional items with a slot", attrs: []string{optMultipleLowerZero("items", optSlot("ELEMENT", "at9000"))}},
}

// itemsPlacements put an ITEM_TREE or ITEM_LIST at the template root, or
// as an ACTION's description. structure returns the generated one.
var itemsPlacements = []struct {
	name      string
	opt       func(structure string) string
	structure func(t *testing.T, out any) any
}{
	{
		name: "root",
		opt:  func(structure string) string { return structure },
		structure: func(_ *testing.T, out any) any {
			return out
		},
	},
	{
		name: "ACTION description",
		opt: func(structure string) string {
			return optTemplate("ACTION", append(entryAttrs,
				optSingle("ism_transition", optNode("ISM_TRANSITION", "")),
				optSingle("description", structure))...)
		},
		structure: func(t *testing.T, out any) any {
			a, ok := out.(*rm.Action)
			if !ok {
				t.Fatalf("generated root is %T, want *rm.Action", out)
			}
			return a.Description
		},
	},
}

// itemCount returns the number of members of a generated ITEM_TREE or
// ITEM_LIST.
func itemCount(t *testing.T, s any) int {
	t.Helper()
	switch v := s.(type) {
	case *rm.ItemTree:
		return len(v.Items)
	case *rm.ItemList:
		return len(v.Items)
	}
	t.Fatalf("structure is %T, want *rm.ItemTree or *rm.ItemList", s)
	return 0
}

// itemsOPT is the template for one case: the structure of rmType with
// attrs, placed by place. A root structure is the template's archetype
// root; a nested one is a plain node.
func itemsOPT(place string, opt func(string) string, rmType string, attrs []string) string {
	if place == "root" {
		return optTemplate(rmType, attrs...)
	}
	return opt(optNode(rmType, "at0001", attrs...))
}

// TestREQ107_OptionalItemsGetNoMember is the REQ-107 check that an
// ITEM_TREE or ITEM_LIST whose items attribute the OPT leaves optional, or
// does not name, gets no member: the RM makes both items lists optional
// and no RM rule needs a member, so Minimal MUST NOT materialise one and an
// optional attribute the OPT leaves silent MUST get no child. It holds
// under both policies and both compile modes, at the root and nested, and
// the output passes the RM floor and the template validator.
func TestREQ107_OptionalItemsGetNoMember(t *testing.T) {
	for _, rmType := range []string{"ITEM_TREE", "ITEM_LIST"} {
		for _, variant := range itemsVariants {
			for _, place := range itemsPlacements {
				for _, implicit := range []bool{true, false} {
					c := compileOPTText(t, itemsOPT(place.name, place.opt, rmType, variant.attrs), implicit)
					for _, policy := range []instance.Policy{instance.Minimal, instance.Example} {
						t.Run(fmt.Sprintf("%s/%s/%s/implicit=%t/%v", rmType, variant.name, place.name, implicit, policy), func(t *testing.T) {
							out, err := instance.Generate(t.Context(), c, instance.Options{Policy: policy, Now: defaultsNow})
							if err != nil {
								t.Fatalf("Generate: %v", err)
							}
							if n := itemCount(t, place.structure(t, out)); n != 0 {
								t.Errorf("%s.items has %d members, want none", rmType, n)
							}
							checkValid(t, out, c)
						})
					}
				}
			}
		}
	}
}

// TestREQ107_RequiredItemsGetOneMember is the REQ-107 check that an
// ITEM_TREE or ITEM_LIST whose items attribute the OPT requires, with no
// child to describe a member, gets exactly one member, and that the output
// passes the RM floor and the template validator. It holds under both
// policies and both compile modes, at the root and nested.
func TestREQ107_RequiredItemsGetOneMember(t *testing.T) {
	for _, rmType := range []string{"ITEM_TREE", "ITEM_LIST"} {
		for _, place := range itemsPlacements {
			for _, implicit := range []bool{true, false} {
				c := compileOPTText(t, itemsOPT(place.name, place.opt, rmType, []string{optRequiredMultiple("items")}), implicit)
				for _, policy := range []instance.Policy{instance.Minimal, instance.Example} {
					t.Run(fmt.Sprintf("%s/%s/implicit=%t/%v", rmType, place.name, implicit, policy), func(t *testing.T) {
						out, err := instance.Generate(t.Context(), c, instance.Options{Policy: policy, Now: defaultsNow})
						if err != nil {
							t.Fatalf("Generate: %v", err)
						}
						if n := itemCount(t, place.structure(t, out)); n != 1 {
							t.Errorf("%s.items has %d members, want 1", rmType, n)
						}
						checkValid(t, out, c)
					})
				}
			}
		}
	}
}

// checkValid reports every error the RM floor or the template validator
// finds in out.
func checkValid(t *testing.T, out any, c *templatecompile.Compiled) {
	t.Helper()
	for _, iss := range validation.ValidateRM(out).Issues {
		if iss.Severity == validation.Error {
			t.Errorf("ValidateRM: %s @ %s: %s", iss.Code, iss.Path, iss.Detail)
		}
	}
	for _, iss := range validation.Validate(out, c).Issues {
		if iss.Severity == validation.Error {
			t.Errorf("Validate: %s @ %s: %s", iss.Code, iss.Path, iss.Detail)
		}
	}
}
