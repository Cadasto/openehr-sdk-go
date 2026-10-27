package bmmgen

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/bmm"
)

// TestREQ175_RenderUnmarshalXMLClassDescriptor pins, for every class whose
// canonical-XML decoder the RM and AOM 1.4 targets render, the description
// of the class the decoder hands xmlunmatched.Enter on entry (REQ-175, ADR
// 0033): a package-level var beside the method, one per class and none per
// generic instantiation, named by the class's BMM name, with Attrs the
// members read as XML attributes, Required the mandatory members a document
// can carry, in the Reference Model's order with an ancestor's before the
// class's own, minus the four interval boundary flags and
// RESOURCE_DESCRIPTION.parent_resource, and Items the multi-valued members.
// No decoder calls CheckUnmatchedAttrs any more: Enter checks the attributes.
func TestREQ175_RenderUnmarshalXMLClassDescriptor(t *testing.T) {
	flagged, parentResource := 0, false
	for _, target := range []Target{TargetRM, TargetAOM14} {
		plan, err := BuildPlanForTarget(context.Background(), target, bmm.FSResolver{Root: testResources})
		if err != nil {
			t.Fatalf("BuildPlanForTarget: %v", err)
		}
		for _, f := range plan.Files {
			classes, err := codecClassesIn(plan, f)
			if err != nil {
				t.Fatalf("codecClassesIn(%s): %v", f.FileBase, err)
			}
			for _, pc := range classes {
				fields, err := effectiveFields(plan, pc)
				if err != nil {
					t.Fatalf("effectiveFields(%s): %v", pc.BMMName, err)
				}
				src, err := renderUnmarshalXML(plan, pc, fields)
				if err != nil {
					t.Fatalf("renderUnmarshalXML(%s): %v", pc.BMMName, err)
				}
				for _, want := range []string{
					fmt.Sprintf("\nvar xmlClass%s = xmlunmatched.Class{\n\tName: %q,\n", pc.GoName, pc.BMMName),
					fmt.Sprintf("\tif _err := xmlunmatched.Enter(_dec, _start, &xmlClass%s); _err != nil {\n", pc.GoName),
				} {
					if !strings.Contains(src, want) {
						t.Errorf("%s XML decoder lacks %q:\n%s", pc.BMMName, want, src)
					}
				}
				if strings.Contains(src, "CheckUnmatchedAttrs") {
					t.Errorf("%s XML decoder still calls CheckUnmatchedAttrs; Enter checks the attributes:\n%s", pc.BMMName, src)
				}
				if strings.Count(src, "var xmlClass") != 1 {
					t.Errorf("%s XML decoder declares %d descriptors; want one per class:\n%s", pc.BMMName, strings.Count(src, "var xmlClass"), src)
				}

				d, err := xmlClassDescriptor(plan, pc, fields)
				if err != nil {
					t.Fatalf("xmlClassDescriptor(%s): %v", pc.BMMName, err)
				}
				var attrs, items, mandatory []string
				for _, ef := range fields {
					prop := ef.Prop.PropertyName()
					if isXMLAttributeProperty(ef.Prop) {
						attrs = append(attrs, prop)
					}
					if _, ok := ef.Prop.(*bmm.ContainerProperty); ok {
						items = append(items, prop)
					}
					if mandatoryProperty(ef.Prop) {
						mandatory = append(mandatory, prop)
					}
				}
				if !slices.Equal(d.attrs, attrs) {
					t.Errorf("%s descriptor Attrs = %q; want the XML-attribute members %q", pc.BMMName, d.attrs, attrs)
				}
				if !slices.Equal(d.items, items) {
					t.Errorf("%s descriptor Items = %q; want the container members %q", pc.BMMName, d.items, items)
				}
				for _, name := range d.required {
					if !slices.Contains(mandatory, name) {
						t.Errorf("%s descriptor requires %q, which is not a mandatory member", pc.BMMName, name)
					}
				}
				for _, name := range mandatory {
					switch {
					case isIntervalFlag(name):
						flagged++
						if slices.Contains(d.required, name) {
							t.Errorf("%s descriptor requires the boundary flag %q; the decoder gives an absent flag a value", pc.BMMName, name)
						}
					case pc.BMMName == "RESOURCE_DESCRIPTION" && name == "parent_resource":
						parentResource = true
						if slices.Contains(d.required, name) {
							t.Errorf("%s descriptor requires %q, which no value can be held in", pc.BMMName, name)
						}
					case !slices.Contains(d.required, name):
						t.Errorf("%s descriptor leaves out the mandatory member %q; Required = %q", pc.BMMName, name, d.required)
					}
				}
				if len(d.required) > maxXMLRequired {
					t.Errorf("%s descriptor requires %d members; the runtime holds at most %d", pc.BMMName, len(d.required), maxXMLRequired)
				}
			}
		}
	}
	if flagged == 0 || !parentResource {
		t.Errorf("saw %d boundary flags and parent_resource %v among the mandatory members; want the interval classes and RESOURCE_DESCRIPTION covered", flagged, parentResource)
	}
}

// mandatoryProperty reports whether the RM makes prop mandatory, by the rule
// rminfo's table applies: a single or generic property by its flag, a
// container by its flag or a lower bound of one.
func mandatoryProperty(prop bmm.Property) bool {
	switch p := prop.(type) {
	case *bmm.SingleProperty:
		return p.IsMandatory
	case *bmm.SinglePropertyOpen:
		return p.IsMandatory
	case *bmm.GenericProperty:
		return p.IsMandatory
	case *bmm.ContainerProperty:
		return containerRequired(p.Cardinality, p.IsMandatory)
	}
	return false
}

// TestREQ175_XMLClassDescriptorOrderAndShape pins the order of Required
// against two classes: OBSERVATION lists LOCATABLE's members before ENTRY's
// and ENTRY's before its own, the Reference Model's declaration order with
// an ancestor's first, which is not the order the decoder's switch uses; and
// DV_COUNT renders to the literal its one mandatory member and one
// multi-valued member give, before gofmt aligns it.
func TestREQ175_XMLClassDescriptorOrderAndShape(t *testing.T) {
	plan, err := BuildPlanForTarget(context.Background(), TargetRM, bmm.FSResolver{Root: testResources})
	if err != nil {
		t.Fatalf("BuildPlanForTarget: %v", err)
	}
	describe := func(name string) (xmlClassDesc, []emittedField) {
		t.Helper()
		pc, ok := plan.Classes[name]
		if !ok {
			t.Fatalf("plan has no class %s", name)
		}
		fields, err := effectiveFields(plan, pc)
		if err != nil {
			t.Fatalf("effectiveFields(%s): %v", name, err)
		}
		d, err := xmlClassDescriptor(plan, pc, fields)
		if err != nil {
			t.Fatalf("xmlClassDescriptor(%s): %v", name, err)
		}
		return d, fields
	}

	obs, _ := describe("OBSERVATION")
	want := []string{"name", "archetype_node_id", "language", "encoding", "subject", "data"}
	if !slices.Equal(obs.required, want) {
		t.Errorf("OBSERVATION Required = %q; want %q, an ancestor's members first", obs.required, want)
	}
	if obs.name != "OBSERVATION" || !slices.Equal(obs.attrs, []string{"archetype_node_id"}) {
		t.Errorf("OBSERVATION descriptor = %+v; want Name OBSERVATION and Attrs [archetype_node_id]", obs)
	}
	count, _ := describe("DV_COUNT")
	if got, want := renderXMLClassVar(count), "// xmlClassDVCount describes DV_COUNT to the canonical-XML decode runtime.\nvar xmlClassDVCount = xmlunmatched.Class{\n\tName: \"DV_COUNT\",\n\tRequired: []string{\"magnitude\"},\n\tItems: []string{\"other_reference_ranges\"},\n}\n"; got != want {
		t.Errorf("DV_COUNT descriptor renders as:\n%s\nwant:\n%s", got, want)
	}
}

// TestREQ175_XMLClassDescriptorCap pins the cap the runtime's bitset sets: a
// class with more presence-checked members than the runtime can hold is
// refused by the generator, at the boundary, rather than rendered.
func TestREQ175_XMLClassDescriptorCap(t *testing.T) {
	members := func(n int) ([]emittedField, []string) {
		fields := make([]emittedField, 0, n)
		order := make([]string, 0, n)
		for i := range n {
			p := &bmm.SingleProperty{TypeName: "String"}
			p.Name = fmt.Sprintf("m%d", i)
			p.IsMandatory = true
			fields = append(fields, emittedField{Prop: p, OwnerName: "WIDE"})
			order = append(order, p.Name)
		}
		return fields, order
	}
	fields, order := members(maxXMLRequired)
	if got, err := xmlRequiredMembers("WIDE", fields, order); err != nil || len(got) != maxXMLRequired {
		t.Errorf("xmlRequiredMembers(%d members) = %d, %v; want all %d and no error", maxXMLRequired, len(got), err, maxXMLRequired)
	}
	fields, order = members(maxXMLRequired + 1)
	if _, err := xmlRequiredMembers("WIDE", fields, order); err == nil {
		t.Errorf("xmlRequiredMembers(%d members) = nil error; want a refusal: the runtime holds at most %d", maxXMLRequired+1, maxXMLRequired)
	}
}
