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
// a value an interface holds by value included, through xmlOpenParamClass; a
// primitive through the encoder's own EncodeElement, as its text; and refuses
// any other value an interface holds, through xmlOpenParamText, with
// canxml.ErrInvalidShape naming the member. A member whose parameter is bound
// to an abstract class, such as EVENT.data, is a polymorphic site by its
// bound and goes to canxml.EncodePoly alone. The census must reach
// ORIGINAL_VERSION.data and the bounds of BASE Interval, and finds no list of
// a generic parameter, which the encoder does not cover.
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
						fmt.Sprintf("_c, _ok = %s(%s); _ok", openParamClassFunc, openParamValue(recv, ef)),
						fmt.Sprintf("_e.EncodeElement(%s, xml.StartElement{Name: xml.Name{Local: %q}})", openParamPointer(recv, ef), prop),
					} {
						if !strings.Contains(src, want) {
							t.Errorf("%s.%s is typed by a generic parameter; its encoder lacks %q, so a class value would be written with no xsi:type or a primitive not as its text:\n%s", pc.BMMName, prop, want, src)
						}
					}
					for _, want := range []string{
						fmt.Sprintf("!%s(%s)", openParamTextFunc, openParamPointer(recv, ef)),
						fmt.Sprintf("canxml.ErrInvalidShape, %s)", openParamValue(recv, ef)),
						fmt.Sprintf(" %s.%s holds %%T, ", sc.ClassName(), prop),
					} {
						if !strings.Contains(src, want) {
							t.Errorf("%s.%s is typed by a generic parameter; its encoder lacks %q, so a value that is no class or primitive would not be refused, naming the member:\n%s", pc.BMMName, prop, want, src)
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

// openParamValue returns the expression of the value of ef, a member typed by
// a generic parameter, in recv's encoder: the value an optional member's
// pointer points to, the field otherwise.
func openParamValue(recv string, ef emittedField) string {
	field := recv + "." + FieldName(ef.Prop.PropertyName())
	if p, ok := ef.Prop.(*bmm.SingleProperty); ok && !p.IsMandatory {
		return "*" + field
	}
	return field
}

// TestREQ056_RenderXMLOpenParamHelpersOnce pins that a target whose
// canonical-XML encoders call xmlOpenParamClass and xmlOpenParamText declares
// each once, in the file xmlOpenParamFile names, and that a target that calls
// neither declares neither, so the package compiles and lint finds no unused
// function. xmlOpenParamClass returns a pointer to a copy of a value, held by
// value, of each class the type registry constructs, and of each generic one
// in any instantiation through xmlClassPointer, which the declaring file
// gives each such class the target declares, and no other file and no other
// target; xmlOpenParamText accepts, for an interface type argument, nil and a
// value of each Go type a BMM primitive maps to. The RM target calls them,
// for ORIGINAL_VERSION.data and the bounds of BASE Interval; the AOM 1.4
// target has no such member.
func TestREQ056_RenderXMLOpenParamHelpersOnce(t *testing.T) {
	for _, tc := range []struct {
		target Target
		calls  bool
	}{
		{target: TargetRM, calls: true},
		{target: TargetAOM14, calls: false},
	} {
		t.Run(tc.target.GoPackage, func(t *testing.T) {
			plan, err := BuildPlanForTarget(context.Background(), tc.target, testResolver())
			if err != nil {
				t.Fatalf("BuildPlanForTarget: %v", err)
			}
			declaring, err := xmlOpenParamFile(plan)
			if err != nil {
				t.Fatalf("xmlOpenParamFile: %v", err)
			}
			classDecl := "func " + openParamClassFunc + "(v any) (canxml.BMMNamer, bool) {"
			textDecl := "func " + openParamTextFunc + "[T any](v *T) bool {"
			methodDecl := ") " + openParamPointerMethod + "() canxml.BMMNamer {"
			calls, classDecls, textDecls, methodDecls := 0, 0, 0, 0
			var helpers string
			for _, f := range plan.Files {
				out, err := RenderMarshalXMLFile(plan, f)
				if err != nil {
					t.Fatalf("RenderMarshalXMLFile(%s): %v", f.FileBase, err)
				}
				src := string(out)
				calls += strings.Count(src, "= "+openParamClassFunc+"(")
				classDecls += strings.Count(src, classDecl)
				textDecls += strings.Count(src, textDecl)
				methodDecls += strings.Count(src, methodDecl)
				if strings.Contains(src, classDecl) {
					if f != declaring {
						t.Errorf("%s declares %s; want it declared in the file xmlOpenParamFile names", f.FileBase, openParamClassFunc)
					}
					helpers = src
				}
			}
			if (calls > 0) != tc.calls {
				t.Fatalf("%d calls to %s; want calls %t", calls, openParamClassFunc, tc.calls)
			}
			want := 0
			if calls > 0 {
				want = 1
			}
			if classDecls != want || textDecls != want {
				t.Fatalf("%d calls to %s; got %d declarations of it and %d of %s, want %d of each", calls, openParamClassFunc, classDecls, textDecls, openParamTextFunc, want)
			}
			if want == 0 {
				if declaring != nil {
					t.Errorf("xmlOpenParamFile = %s; want nil for a target that calls no %s", declaring.FileBase, openParamClassFunc)
				}
				if methodDecls != 0 {
					t.Errorf("%d declarations of %s; want none for a target that calls no %s, which lint would report unused", methodDecls, openParamPointerMethod, openParamClassFunc)
				}
				return
			}
			for _, pc := range anyMemberClasses(plan) {
				if c := fmt.Sprintf("\tcase %s:\n\t\treturn &x, true\n", anyMemberGoType(plan, pc)); !strings.Contains(helpers, c) {
					t.Errorf("%s lacks %q, so a %s held by value would be refused", openParamClassFunc, c, pc.BMMName)
				}
			}
			generic := openParamGenericClasses(plan)
			if !slices.ContainsFunc(generic, func(pc *PlannedClass) bool { return pc.BMMName == "DV_INTERVAL" }) {
				t.Fatalf("openParamGenericClasses lacks DV_INTERVAL; want every generic class the target declares")
			}
			if c := fmt.Sprintf("\tcase %s:\n\t\treturn x.%s(), true\n", openParamPointerType, openParamPointerMethod); !strings.Contains(helpers, c) {
				t.Errorf("%s lacks %q, so a generic class held by value in an instantiation the registry does not construct would be refused", openParamClassFunc, c)
			}
			if d := fmt.Sprintf("type %s interface {\n\t%s() canxml.BMMNamer\n}\n", openParamPointerType, openParamPointerMethod); !strings.Contains(helpers, d) {
				t.Errorf("the file declaring %s lacks %q", openParamClassFunc, d)
			}
			for _, pc := range generic {
				sc := pc.Class.(*bmm.SimpleClass)
				recv := jsonmarReceiverName(pc.GoName)
				d := fmt.Sprintf("func (%s %s%s) %s() canxml.BMMNamer { return &%s }\n", recv, pc.GoName, genericTypeArgList(sc), openParamPointerMethod, recv)
				if !strings.Contains(helpers, d) {
					t.Errorf("the file declaring %s lacks %q, so a %s held by value in any instantiation would be refused", openParamClassFunc, d, pc.BMMName)
				}
			}
			if methodDecls != len(generic) {
				t.Errorf("%d declarations of %s; want one for each of the %d generic classes the target declares", methodDecls, openParamPointerMethod, len(generic))
			}
			if c := "\tcase nil, " + strings.Join(xmlOpenParamPrimitives(), ", ") + ":\n\t\treturn true\n"; !strings.Contains(helpers, c) {
				t.Errorf("%s lacks %q, so a primitive an interface holds would be refused", openParamTextFunc, c)
			}
			if !strings.Contains(helpers, "\tif any(*new(T)) != nil {\n\t\treturn true\n\t}\n") {
				t.Errorf("%s does not accept every value of a type argument that is no interface type", openParamTextFunc)
			}
		})
	}
}
