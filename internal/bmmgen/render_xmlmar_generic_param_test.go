package bmmgen

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/bmm"
)

// TestREQ056_RenderMarshalXMLGenericParamClassValue pins, for every member
// the RM and AOM 1.4 targets encode that is typed by a generic parameter of
// its class, that the canonical-XML encoder writes a class value there
// through canxml.EncodePoly, with its `xsi:type`, whatever the instantiation,
// and anything else, a primitive, through the encoder's own EncodeElement, as
// its text. A member whose parameter is bound to an abstract class, such as
// EVENT.data, is a polymorphic site by its bound and goes to canxml.EncodePoly
// alone. The census must reach ORIGINAL_VERSION.data and the bounds of BASE
// Interval, and finds no list of a generic parameter, which the encoder does
// not cover.
func TestREQ056_RenderMarshalXMLGenericParamClassValue(t *testing.T) {
	var covered []string
	for _, target := range []Target{TargetRM, TargetAOM14} {
		plan, err := BuildPlanForTarget(context.Background(), target, testResolver())
		if err != nil {
			t.Fatalf("BuildPlanForTarget: %v", err)
		}
		for _, f := range plan.Files {
			classes, err := codecClassesIn(plan, f)
			if err != nil {
				t.Fatalf("codecClassesIn(%s): %v", f.FileBase, err)
			}
			for _, pc := range classes {
				sc, ok := pc.Class.(*bmm.SimpleClass)
				if !ok {
					continue
				}
				fields, err := effectiveFields(plan, pc)
				if err != nil {
					t.Fatalf("effectiveFields(%s): %v", pc.BMMName, err)
				}
				recv := jsonmarReceiverName(pc.GoName)
				for _, ef := range fields {
					if !isOpenParamProperty(ef) {
						continue
					}
					prop := ef.Prop.PropertyName()
					if _, list := ef.Prop.(*bmm.ContainerProperty); list {
						t.Errorf("%s.%s is a list of a generic parameter; the canonical-XML encoder does not route its class items through canxml.EncodePoly", pc.BMMName, prop)
						continue
					}
					src, err := renderMarshalXMLField(plan, recv, sc, ef)
					if err != nil {
						t.Fatalf("renderMarshalXMLField(%s.%s): %v", pc.BMMName, prop, err)
					}
					if _, kind := polymorphicProperty(plan, ef.Owner, sc, ef.Prop); kind == polySingle {
						if want := fmt.Sprintf("canxml.EncodePoly(_e, %q, %s.%s)", prop, recv, FieldName(prop)); !strings.Contains(src, want) {
							t.Errorf("%s.%s is bound to an abstract class; its encoder lacks %q:\n%s", pc.BMMName, prop, want, src)
						}
						continue
					}
					covered = append(covered, pc.BMMName+"."+prop)
					for _, want := range []string{
						".(canxml.BMMNamer)",
						fmt.Sprintf("canxml.EncodePoly(_e, %q, _c)", prop),
						fmt.Sprintf("_e.EncodeElement(%s, xml.StartElement{Name: xml.Name{Local: %q}})", openParamPointer(recv, ef), prop),
					} {
						if !strings.Contains(src, want) {
							t.Errorf("%s.%s is typed by a generic parameter; its encoder lacks %q, so a class value would be written with no xsi:type or a primitive not as its text:\n%s", pc.BMMName, prop, want, src)
						}
					}
				}
			}
		}
	}
	for _, want := range []string{"ORIGINAL_VERSION.data", "Interval.lower", "Interval.upper", "Point_interval.lower", "Proper_interval.upper"} {
		if !slices.Contains(covered, want) {
			t.Errorf("the census covered %q; want %s among them", covered, want)
		}
	}
}

// openParamPointer returns the expression of a pointer to the value of ef,
// a member typed by a generic parameter, in recv's encoder: the field when
// an optional member holds it as a pointer, its address otherwise.
func openParamPointer(recv string, ef emittedField) string {
	field := recv + "." + FieldName(ef.Prop.PropertyName())
	if p, ok := ef.Prop.(*bmm.SingleProperty); ok && !p.IsMandatory {
		return field
	}
	return "&" + field
}
