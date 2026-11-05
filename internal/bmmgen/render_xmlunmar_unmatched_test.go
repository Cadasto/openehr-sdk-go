package bmmgen

import (
	"context"
	"fmt"
	"regexp"
	"slices"
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
// between its child elements through its character-data arm, the element of
// a Hash canonical XML does not model through its own arm, and a primitive
// element through xmlunmatched.DecodeElement. A member typed by a generic
// List, Set or Array decodes each element as one item and appends it, a Hash
// member each element as one entry, and an Any member its element through
// xmlschema.DecodeAny, as the type its discriminator names (REQ-176).
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
// content unreported. Each single-valued member's arm, and no other, opens
// with the guard that hands a second child element of the member to
// xmlunmatched.SkipUnmatched, each member of a class with a bit of its own
// (REQ-170). Each optional member whose field can hold nil, and no other,
// then hands a nilled element to xmlunmatched.SkipNil and sets the field to
// nil, and an optional primitive decodes through xmlunmatched.DecodeOptional
// (REQ-056). The other checks read the arm without its guards.
func TestREQ170_RenderUnmarshalXMLRoutesUnmatchedContent(t *testing.T) {
	const (
		attrs        = "\tif _fresh, _err := xmlunmatched.Enter(_dec, _start, &xmlClass"
		switchOnName = "\t\t\tswitch xmlunmatched.LocalName(_t.Name) {\n"
		defaultArm   = "\t\t\tdefault:\n\t\t\t\tif _err := xmlunmatched.SkipUnmatched(_dec, _t); _err != nil {\n"
		textArm      = "\t\tcase xml.CharData:\n\t\t\tif _err := xmlunmatched.CheckUnmatchedText(_dec, _t); _err != nil {\n"
	)
	plans := map[string]*Plan{}
	withDecoder := map[string]bool{}
	for _, target := range []Target{TargetRM, TargetAOM14} {
		plan, err := BuildPlanForTarget(context.Background(), target, testResolver())
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
		hashes, anys, params, octets, lists, choices, singles, nils, zeroed, optionals, resets := 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0
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
				bits := map[string]string{}
				for _, ef := range fields {
					prop := ef.Prop.PropertyName()
					arm := xmlCaseArm(src, prop)
					guard := repeatGuard.FindStringSubmatch(arm)
					single, err := xmlSingleValued(plan, ef)
					if err != nil {
						t.Fatalf("xmlSingleValued(%s.%s): %v", pc.BMMName, prop, err)
					}
					switch {
					case single && guard == nil:
						t.Errorf("%s.%s is single-valued; its arm must open with the repeat guard:\n%s", pc.BMMName, prop, arm)
					case !single && guard != nil:
						t.Errorf("%s.%s is multi-valued or a Hash member, whose child elements are its items or entries; its arm must not guard against a repeat:\n%s", pc.BMMName, prop, arm)
					case guard != nil:
						singles++
						if guard[1] != guard[2] {
							t.Errorf("%s.%s tests bit %s of _got and sets bit %s; want one bit:\n%s", pc.BMMName, prop, guard[1], guard[2], arm)
						}
						if other, ok := bits[guard[1]]; ok {
							t.Errorf("%s.%s and %s share bit %s of _got; want a bit per member", pc.BMMName, prop, other, guard[1])
						}
						bits[guard[1]] = prop
						arm = strings.Replace(arm, guard[0], "", 1)
					}
					nilable, err := xmlNilable(plan, ef)
					if err != nil {
						t.Fatalf("xmlNilable(%s.%s): %v", pc.BMMName, prop, err)
					}
					nilGuard := nilGuardOf(FieldName(prop)).FindStringSubmatch(arm)
					switch {
					case nilable && nilGuard == nil:
						t.Errorf("%s.%s is single-valued; its arm must hand a nilled element to xmlunmatched.SkipNil and give the member no value:\n%s", pc.BMMName, prop, arm)
					case !nilable && nilGuard != nil:
						t.Errorf("%s.%s is multi-valued, a Hash, a boundary flag or typed by a generic parameter; its arm must not skip a nilled element:\n%s", pc.BMMName, prop, arm)
					case nilGuard != nil:
						nils++
						// A mandatory member gets its zero value, as a missing one
						// does (REQ-175); an optional one is left nil (REQ-056).
						if zeroes := nilGuard[1] == ""; zeroes != mandatoryProperty(ef.Prop) {
							t.Errorf("%s.%s: mandatory is %t, yet its nil guard zeroes it is %t; want a mandatory member zeroed and an optional one set to nil:\n%s", pc.BMMName, prop, mandatoryProperty(ef.Prop), zeroes, arm)
						}
						if nilGuard[1] == "" {
							zeroed++
						}
						arm = strings.Replace(arm, nilGuard[0], "", 1)
					}
					// An arm that appends each element as one item starts the
					// list empty at the element's first item, as canonical JSON
					// replaces an array (REQ-056).
					if at := strings.Index(arm, "= append("); at >= 0 {
						reset := listResetOf(FieldName(prop)).FindStringIndex(arm)
						switch {
						case reset == nil:
							t.Errorf("%s.%s appends each element as one item; its arm must start the list empty at the element's first item:\n%s", pc.BMMName, prop, arm)
						case reset[0] > at:
							t.Errorf("%s.%s starts its list empty after appending an item; want the reset first:\n%s", pc.BMMName, prop, arm)
						default:
							resets++
						}
					}
					if p, ok := ef.Prop.(*bmm.SingleProperty); ok && !p.IsMandatory && isPrimitive(p.TypeName) && !isOpenParam(ef.Owner, p.TypeName) {
						optionals++
						if !strings.Contains(arm, "xmlunmatched.DecodeOptional(_dec, &_t, &_v)") {
							t.Errorf("%s.%s is an optional primitive; its arm must decode through xmlunmatched.DecodeOptional, which leaves it absent when its element has no text:\n%s", pc.BMMName, prop, arm)
						}
						continue
					}
					if isGenericListProperty(ef.Prop) {
						lists++
						if !strings.Contains(arm, "append(") {
							t.Errorf("%s.%s is typed by a generic list; its arm must decode each element as one item and append it:\n%s", pc.BMMName, prop, arm)
						}
					}
					choice, err := xmlChoiceOf(plan, pc, ef)
					if err != nil {
						t.Fatalf("xmlChoiceOf(%s.%s): %v", pc.BMMName, prop, err)
					}
					switch {
					case choice != nil:
						choices++
						if !strings.Contains(arm, "xmlunmatched.DecodeChoice(_dec, _t, "+choice.varName+")") {
							t.Errorf("%s.%s takes a choice of shapes; its arm must decode through xmlunmatched.DecodeChoice with its shapes:\n%s", pc.BMMName, prop, arm)
						}
					case isHashProperty(ef.Prop):
						hashes++
						shape, _, err := xmlHashOf(plan, ef)
						if err != nil {
							t.Fatalf("xmlHashOf(%s.%s): %v", pc.BMMName, prop, err)
						}
						if shape == nil {
							if !strings.Contains(arm, "xmlunmatched.SkipUnmodelled(_dec, _t)") {
								t.Errorf("%s.%s is a Hash canonical XML does not model; its arm must hand the element to xmlunmatched.SkipUnmodelled, which keeps the member present:\n%s", pc.BMMName, prop, arm)
							}
							continue
						}
						if !strings.Contains(arm, "xmlunmatched.DecodeEntry(") && !strings.Contains(arm, "xmlunmatched.DecodeEntryKeyedBy(") {
							t.Errorf("%s.%s is a Hash member; its arm must decode each element as one entry through xmlunmatched.DecodeEntry or DecodeEntryKeyedBy:\n%s", pc.BMMName, prop, arm)
						}
					case isAnyProperty(ef.Prop):
						anys++
						if !strings.Contains(arm, "xmlschema.DecodeAny(_dec, _t)") || strings.Contains(arm, "SkipUnmatched(") {
							t.Errorf("%s.%s is an Any member; its arm must decode the element through xmlschema.DecodeAny, as the type its discriminator names:\n%s", pc.BMMName, prop, arm)
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
		if singles == 0 || nils == 0 || zeroed == 0 || nils == zeroed || optionals == 0 || resets == 0 {
			t.Errorf("the %s target rendered %d repeat guards, %d nil guards (%d zeroing), %d optional primitives and %d list resets; want each covered", target.GoPackage, singles, nils, zeroed, optionals, resets)
		}
		if target.GoPackage == TargetRM.GoPackage && (hashes == 0 || params == 0 || octets == 0) {
			t.Errorf("the RM target rendered %d Hash arms, %d generic-parameter arms and %d Octet arms; want the resource-metadata Hash members, VERSION<T>.data and DV_MULTIMEDIA.data covered", hashes, params, octets)
		}
		if target.GoPackage == TargetAOM14.GoPackage && (anys == 0 || lists == 0 || choices == 0) {
			t.Errorf("the AOM 1.4 target rendered %d Any arms, %d generic-list arms and %d choice arms; want `EXPR_LEAF.item`, C_CODE_PHRASE's `code_list` and ORDINAL's `symbol` covered", anys, lists, choices)
		}
	}
}

// repeatGuard matches the repeat guard at the head of a single-valued
// member's arm ([withRepeatGuard]): the bit it tests and the bit it sets.
var repeatGuard = regexp.MustCompile(`\t\t\t\tif _got&\(1<<(\d+)\) != 0 \{\n\t\t\t\t\tif _err := xmlunmatched\.SkipUnmatched\(_dec, _t\); _err != nil \{\n\t\t\t\t\t\treturn _err\n\t\t\t\t\t\}\n\t\t\t\t\tbreak\n\t\t\t\t\}\n\t\t\t\t_got \|= 1 << (\d+)\n`)

// nilGuardOf matches the nil guard of the member held in the Go field field
// ([withNilGuard]); its first group holds " = nil" when the guard sets the
// field to nil, and is empty when it zeroes the field.
func nilGuardOf(field string) *regexp.Regexp {
	f := regexp.QuoteMeta(field)
	return regexp.MustCompile(`\t\t\t\tif _nil, _err := xmlunmatched\.SkipNil\(_dec, _t\); _err != nil \{\n\t\t\t\t\treturn _err\n\t\t\t\t\} else if _nil \{\n\t\t\t\t\t(?:[a-z]+\.` + f + `( = nil)|xmlunmatched\.Zero\(&[a-z]+\.` + f + `\))\n\t\t\t\t\tbreak\n\t\t\t\t\}\n`)
}

// listResetOf matches the lines that start the list held in the Go field
// field empty at the element's first item ([withListReset]).
func listResetOf(field string) *regexp.Regexp {
	f := regexp.QuoteMeta(field)
	return regexp.MustCompile(`\t\t\t\tif !_begun` + f + ` \{\n\t\t\t\t\t_begun` + f + ` = true\n\t\t\t\t\t[a-z]+\.` + f + ` = nil\n\t\t\t\t\}\n`)
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

// isHashProperty reports whether prop is a BMM Hash member.
func isHashProperty(prop bmm.Property) bool {
	switch p := prop.(type) {
	case *bmm.ContainerProperty:
		return p.TypeDef != nil && p.TypeDef.ContainerType == "Hash"
	case *bmm.GenericProperty:
		return p.TypeDef != nil && p.TypeDef.RootType == "Hash"
	}
	return false
}

// isGenericListProperty reports whether prop is typed by a generic List, Set
// or Array, which canonical XML writes as one element per item.
func isGenericListProperty(prop bmm.Property) bool {
	p, ok := prop.(*bmm.GenericProperty)
	return ok && p.TypeDef != nil && slices.Contains([]string{"List", "Set", "Array"}, p.TypeDef.RootType)
}

// isAnyProperty reports whether prop is a single property typed by the BMM
// root type Any, which canonical XML decodes as its discriminator says.
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
// its type, a container's item type, a generic list's item type, or another
// generic type's root.
func decodedClassName(prop bmm.Property) string {
	switch p := prop.(type) {
	case *bmm.SingleProperty:
		return p.TypeName
	case *bmm.ContainerProperty:
		return containerInnerName(p.TypeDef)
	case *bmm.GenericProperty:
		if isGenericListProperty(p) && len(p.TypeDef.GenericParameters) == 1 {
			return containerInnerName(&bmm.ContainerType{TypeDef: p.TypeDef.GenericParameters[0]})
		}
		if p.TypeDef != nil {
			return p.TypeDef.RootType
		}
	}
	return ""
}

// TestREQ056_GenericListDecode pins the decode case of a member typed by a
// generic List, Set or Array (REQ-056): a primitive item reads as text and
// a class item through the class's own decoder, each appended, and an item
// the case cannot decode alone, an interface or an Octet, or a list with no
// item type, is refused at generation rather than skipped.
func TestREQ056_GenericListDecode(t *testing.T) {
	plan, err := BuildPlanForTarget(context.Background(), TargetAOM14, testResolver())
	if err != nil {
		t.Fatalf("BuildPlanForTarget(aom14): %v", err)
	}
	owner, ok := plan.Classes["C_CODE_PHRASE"].Class.(*bmm.SimpleClass)
	if !ok {
		t.Fatalf("plan has no simple class C_CODE_PHRASE")
	}
	field := func(mandatory bool, root string, items ...string) emittedField {
		p := &bmm.GenericProperty{TypeDef: &bmm.GenericType{RootType: root}}
		for _, item := range items {
			p.TypeDef.GenericParameters = append(p.TypeDef.GenericParameters, &bmm.SimpleType{TypeName: item})
		}
		p.Name = "probe"
		p.IsMandatory = mandatory
		return emittedField{Prop: p, Owner: owner, OwnerName: "C_CODE_PHRASE"}
	}
	for _, tc := range []struct {
		name string
		ef   emittedField
		want []string // fragments of the case; nil means a refusal
	}{
		{"optional list of String", field(false, "List", "String"), []string{"var _v string", "xmlunmatched.DecodeElement(_dec, &_t, &_v)", "c.Probe = new([]string)", "*c.Probe = append(*c.Probe, _v)"}},
		{"mandatory set of a class", field(true, "Set", "C_QUANTITY_ITEM"), []string{"var _v CQuantityItem", "_dec.DecodeElement(&_v, &_t)", "c.Probe = append(c.Probe, _v)"}},
		{"list of an interface", field(false, "List", "C_OBJECT"), nil},
		{"list of Octet", field(false, "Array", "Octet"), nil},
		{"list with no item type", field(false, "List"), nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := renderUnmarshalXMLField(plan, "c", owner, tc.ef)
			if tc.want == nil {
				if err == nil {
					t.Errorf("renderUnmarshalXMLField(%s) = %q, nil; want a refusal", tc.name, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("renderUnmarshalXMLField(%s) = %v; want a case", tc.name, err)
			}
			for _, w := range tc.want {
				if !strings.Contains(got, w) {
					t.Errorf("renderUnmarshalXMLField(%s) lacks %q:\n%s", tc.name, w, got)
				}
			}
		})
	}
}
