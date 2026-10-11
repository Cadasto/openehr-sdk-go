package bmmgen

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/bmm"
)

// TestREQ170_RenderUnmarshalXMLRoutesUnmatchedContent pins, for every class
// whose canonical-XML decoder the RM and AOM 1.4 targets render, that the
// decoder hands the content it does not read to the canxml helpers that apply
// the decode's policy (REQ-170): the start element's attributes on entry, an
// element in a foreign namespace through the name it switches on, every other
// element it does not read through its default arm, a Hash element and the
// element of an Any member through their own arms, and a primitive element
// through canxml.DecodeElement. No decoder skips an element itself, so none
// drops content unreported.
func TestREQ170_RenderUnmarshalXMLRoutesUnmatchedContent(t *testing.T) {
	const (
		attrs        = "\tif _err := canxml.CheckUnmatchedAttrs(_dec, _start"
		switchOnName = "\t\t\tswitch canxml.LocalName(_t.Name) {\n"
		defaultArm   = "\t\t\tdefault:\n\t\t\t\tif _err := canxml.SkipUnmatched(_dec, _t); _err != nil {\n"
	)
	for _, target := range []Target{TargetRM, TargetAOM14} {
		plan, err := BuildPlanForTarget(context.Background(), target, bmm.FSResolver{Root: testResources})
		if err != nil {
			t.Fatalf("BuildPlanForTarget: %v", err)
		}
		hashes, anys := 0, 0
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
				for _, want := range []string{attrs, switchOnName, defaultArm} {
					if !strings.Contains(src, want) {
						t.Errorf("%s XML decoder lacks %q:\n%s", pc.BMMName, want, src)
					}
				}
				for _, banned := range []string{"_dec.Skip()", "_t.Name.Local"} {
					if strings.Contains(src, banned) {
						t.Errorf("%s XML decoder still uses %q: content it does not read must go through canxml:\n%s", pc.BMMName, banned, src)
					}
				}
				for _, ef := range fields {
					arm := xmlCaseArm(src, ef.Prop.PropertyName())
					switch {
					case isHashProperty(ef.Prop):
						hashes++
						if !strings.Contains(arm, "canxml.SkipUnmatched(_dec, _t)") {
							t.Errorf("%s.%s is a Hash member; its arm must hand the element to canxml.SkipUnmatched:\n%s", pc.BMMName, ef.Prop.PropertyName(), arm)
						}
					case isAnyProperty(ef.Prop):
						anys++
						if !strings.Contains(arm, "canxml.SkipUnmatched(_dec, _t)") || strings.Contains(arm, "DecodeElement(") {
							t.Errorf("%s.%s is an Any member; its arm must hand the element to canxml.SkipUnmatched as one item and decode nothing:\n%s", pc.BMMName, ef.Prop.PropertyName(), arm)
						}
					case strings.Contains(arm, "DecodeElement(") && !strings.Contains(arm, "canxml.DecodeElement(") && !isClassTypedProperty(plan, ef.Prop):
						t.Errorf("%s.%s decodes as text through the decoder's own DecodeElement, which drops its attributes and child elements unreported:\n%s", pc.BMMName, ef.Prop.PropertyName(), arm)
					}
				}
			}
		}
		if target.GoPackage == TargetRM.GoPackage && hashes == 0 {
			t.Errorf("the RM target rendered no Hash arm; want the resource-metadata Hash members covered")
		}
		if target.GoPackage == TargetAOM14.GoPackage && anys == 0 {
			t.Errorf("the AOM 1.4 target rendered no Any arm; want `assumed_value` and `EXPR_LEAF.item` covered")
		}
	}
}

// xmlCaseArm returns the case arm of a rendered UnmarshalXML that matches the
// element named prop, up to the next arm, or "" when there is none.
func xmlCaseArm(src, prop string) string {
	start := strings.Index(src, fmt.Sprintf("\t\t\tcase %q", prop))
	if start < 0 {
		return ""
	}
	rest := src[start+1:]
	end := len(rest)
	for _, next := range []string{"\t\t\tcase ", "\t\t\tdefault:"} {
		if i := strings.Index(rest, next); i >= 0 && i < end {
			end = i
		}
	}
	return rest[:end]
}

// isHashProperty reports whether prop is a BMM Hash member, which canonical
// XML does not model.
func isHashProperty(prop bmm.Property) bool {
	switch p := prop.(type) {
	case *bmm.ContainerProperty:
		return p.TypeDef != nil && p.TypeDef.ContainerType == "Hash"
	case *bmm.GenericProperty:
		return p.TypeDef != nil && containerKinds[p.TypeDef.RootType]
	}
	return false
}

// isAnyProperty reports whether prop is a single property typed by the BMM
// root type Any, whose content canonical XML does not model.
func isAnyProperty(prop bmm.Property) bool {
	p, ok := prop.(*bmm.SingleProperty)
	return ok && p.TypeName == bmmAnyType
}

// isClassTypedProperty reports whether prop is a single property typed by a
// class, whose decoder checks its own content, rather than by a primitive or
// an open generic parameter.
func isClassTypedProperty(plan *Plan, prop bmm.Property) bool {
	switch p := prop.(type) {
	case *bmm.SingleProperty:
		return !isPrimitive(p.TypeName)
	case *bmm.GenericProperty:
		return true
	case *bmm.ContainerProperty:
		return containerInnerIsClass(plan, p.TypeDef)
	}
	return false
}
