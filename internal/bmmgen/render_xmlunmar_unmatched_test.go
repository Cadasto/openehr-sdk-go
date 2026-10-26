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
// decoder hands the content it does not read to the xmlunmatched helpers that
// apply the decode's policy (REQ-170): the start element's attributes on
// entry, an element in a foreign namespace through the name it switches on,
// every other element it does not read through its default arm, the text
// between its child elements through its character-data arm, a Hash element
// and the element of an Any member through their own arms, and a primitive
// element through xmlunmatched.DecodeElement.
//
// An arm may hand an element to the decoder's own DecodeElement only when the
// member's class is one of those the two targets render a decoder for, which
// checks its own content; the test collects that set from the targets
// themselves. An arm may read an element as text through
// xmlunmatched.DecodeElement only when the member is a primitive or an
// enumeration. Any other class-typed member, such as one held as a BMM
// primitive class or by a generic parameter, would otherwise read nothing and
// report nothing. A member typed by a generic parameter goes through
// xmlunmatched.DecodeParam. No decoder skips an element itself, so none drops
// content unreported.
func TestREQ170_RenderUnmarshalXMLRoutesUnmatchedContent(t *testing.T) {
	const (
		attrs        = "\tif _err := xmlunmatched.CheckUnmatchedAttrs(_dec, _start"
		switchOnName = "\t\t\tswitch xmlunmatched.LocalName(_t.Name) {\n"
		defaultArm   = "\t\t\tdefault:\n\t\t\t\tif _err := xmlunmatched.SkipUnmatched(_dec, _t); _err != nil {\n"
		textArm      = "\t\tcase xml.CharData:\n\t\t\tif _err := xmlunmatched.CheckUnmatchedText(_dec, _t); _err != nil {\n"
	)
	plans := map[string]*Plan{}
	withDecoder := map[string]bool{}
	for _, target := range []Target{TargetRM, TargetAOM14} {
		plan, err := BuildPlanForTarget(context.Background(), target, bmm.FSResolver{Root: testResources})
		if err != nil {
			t.Fatalf("BuildPlanForTarget: %v", err)
		}
		plans[target.GoPackage] = plan
		for _, f := range plan.Files {
			classes, err := codecClassesIn(plan, f)
			if err != nil {
				t.Fatalf("codecClassesIn(%s): %v", f.FileBase, err)
			}
			for _, pc := range classes {
				withDecoder[pc.BMMName] = true
			}
		}
	}
	for name := range handWrittenXMLCodecs {
		withDecoder[name] = true
	}

	for _, target := range []Target{TargetRM, TargetAOM14} {
		plan := plans[target.GoPackage]
		hashes, anys, params, octets := 0, 0, 0, 0
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
				for _, want := range []string{attrs, switchOnName, defaultArm, textArm} {
					if !strings.Contains(src, want) {
						t.Errorf("%s XML decoder lacks %q:\n%s", pc.BMMName, want, src)
					}
				}
				for _, banned := range []string{"_dec.Skip()", "_t.Name.Local"} {
					if strings.Contains(src, banned) {
						t.Errorf("%s XML decoder still uses %q: content it does not read must go through xmlunmatched:\n%s", pc.BMMName, banned, src)
					}
				}
				for _, ef := range fields {
					prop := ef.Prop.PropertyName()
					arm := xmlCaseArm(src, prop)
					switch {
					case isHashProperty(ef.Prop):
						hashes++
						if !strings.Contains(arm, "xmlunmatched.SkipUnmatched(_dec, _t)") {
							t.Errorf("%s.%s is a Hash member; its arm must hand the element to xmlunmatched.SkipUnmatched:\n%s", pc.BMMName, prop, arm)
						}
					case isAnyProperty(ef.Prop):
						anys++
						if !strings.Contains(arm, "xmlunmatched.SkipUnmatched(_dec, _t)") || strings.Contains(arm, "DecodeElement(") {
							t.Errorf("%s.%s is an Any member; its arm must hand the element to xmlunmatched.SkipUnmatched as one item and decode nothing:\n%s", pc.BMMName, prop, arm)
						}
					case isOpenParamProperty(ef):
						if strings.Contains(arm, "_err := canxml.DecodeAs[") {
							continue // a polymorphic slot by its bound, dispatched by xsi:type
						}
						params++
						if !strings.Contains(arm, "xmlunmatched.DecodeParam(") || !strings.Contains(arm, ", canxml.DecodeAs[") {
							t.Errorf("%s.%s is typed by a generic parameter; its arm must decode through xmlunmatched.DecodeParam, which chooses by the instantiation, with canxml.DecodeAs to dispatch an interface:\n%s", pc.BMMName, prop, arm)
						}
					case isOctetProperty(ef.Prop):
						octets++
						if !strings.Contains(arm, "xmlunmatched.DecodeBase64(") {
							t.Errorf("%s.%s is an Octet list, one xs:base64Binary element; its arm must decode through xmlunmatched.DecodeBase64:\n%s", pc.BMMName, prop, arm)
						}
					case strings.Contains(arm, "xmlunmatched.DecodeElement("):
						if name := decodedClassName(ef.Prop); !isPrimitive(name) && !isEnumerationClass(plan, name) {
							t.Errorf("%s.%s decodes its element as text through xmlunmatched.DecodeElement, but %q is neither a primitive nor an enumeration:\n%s", pc.BMMName, prop, name, arm)
						}
					case strings.Contains(arm, "_dec.DecodeElement("):
						if name := decodedClassName(ef.Prop); !withDecoder[name] {
							t.Errorf("%s.%s hands its element to the decoder's own DecodeElement, but %q has no generated decoder: encoding/xml would read nothing from it and report nothing:\n%s", pc.BMMName, prop, name, arm)
						}
					}
				}
			}
		}
		if target.GoPackage == TargetRM.GoPackage && (hashes == 0 || params == 0 || octets == 0) {
			t.Errorf("the RM target rendered %d Hash arms, %d generic-parameter arms and %d Octet arms; want the resource-metadata Hash members, VERSION<T>.data and DV_MULTIMEDIA.data covered", hashes, params, octets)
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

// isOpenParamProperty reports whether ef is typed by a generic parameter of
// the class that declares it.
func isOpenParamProperty(ef emittedField) bool {
	switch p := ef.Prop.(type) {
	case *bmm.SinglePropertyOpen:
		return true
	case *bmm.SingleProperty:
		return isOpenParam(ef.Owner, p.TypeName)
	case *bmm.ContainerProperty:
		return isOpenParam(ef.Owner, containerInnerName(p.TypeDef))
	}
	return false
}

// isOctetProperty reports whether prop is a container of the BMM primitive
// Octet, held as a Go []byte.
func isOctetProperty(prop bmm.Property) bool {
	p, ok := prop.(*bmm.ContainerProperty)
	return ok && isOctetContainer(p.TypeDef)
}

// TestREQ056_RenderMarshalXMLOctetListAsBase64 pins that every Octet list the
// targets encode (DV_MULTIMEDIA's `data` and `integrity_check`) is written as
// one element of base64 text, the xs:base64Binary of the ITS-XML schema,
// and never one element per byte.
func TestREQ056_RenderMarshalXMLOctetListAsBase64(t *testing.T) {
	octets := 0
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
				src, err := renderMarshalXML(plan, pc, fields)
				if err != nil {
					t.Fatalf("renderMarshalXML(%s): %v", pc.BMMName, err)
				}
				for _, ef := range fields {
					if !isOctetProperty(ef.Prop) {
						continue
					}
					octets++
					field := FieldName(ef.Prop.PropertyName())
					if want := "base64.StdEncoding.EncodeToString(" + jsonmarReceiverName(pc.GoName) + "." + field + ")"; !strings.Contains(src, want) {
						t.Errorf("%s.%s encoder lacks %q: an Octet list is one base64 element:\n%s", pc.BMMName, ef.Prop.PropertyName(), want, src)
					}
					if strings.Contains(src, "range "+jsonmarReceiverName(pc.GoName)+"."+field+" ") {
						t.Errorf("%s.%s encoder writes one element per byte:\n%s", pc.BMMName, ef.Prop.PropertyName(), src)
					}
				}
			}
		}
	}
	if octets == 0 {
		t.Error("no Octet list found; want DV_MULTIMEDIA's data and integrity_check covered")
	}
}

// isEnumerationClass reports whether the BMM class name is an enumeration,
// whose Go type is a named string or integer.
func isEnumerationClass(plan *Plan, name string) bool {
	pc, ok := plan.Classes[name]
	if !ok {
		return false
	}
	_, enum := pc.Class.(*bmm.Enumeration)
	return enum
}

// decodedClassName returns the BMM class a property's element decodes as:
// its type, a container's item type, or a generic type's root.
func decodedClassName(prop bmm.Property) string {
	switch p := prop.(type) {
	case *bmm.SingleProperty:
		return p.TypeName
	case *bmm.ContainerProperty:
		return containerInnerName(p.TypeDef)
	case *bmm.GenericProperty:
		if p.TypeDef != nil {
			return p.TypeDef.RootType
		}
	}
	return ""
}
