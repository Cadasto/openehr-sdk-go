package bmmgen

import (
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/bmm"
)

// TestREQ052DirectEncodeRefusesWhatItCannotWrite pins that the generator
// refuses a member whose omission the direct writer has no rule for, rather
// than write it one way where json/v2 writes it another: an any member left
// out when empty, which json/v2 judges by the value it writes; a class held by
// value, or a value of another type, left out when zero; a member typed by a
// type parameter other than omitempty; a tag option other than omitempty and
// omitzero; and two fields of one name at one depth, of which json/v2 writes
// neither.
func TestREQ052DirectEncodeRefusesWhatItCannotWrite(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		member directMember
	}{
		{"an any member with omitempty", directMember{name: "a", field: "A", kind: directEncode, encodeKind: encodeAny, omit: "omitempty"}},
		{"a class held by value with omitzero", directMember{name: "c", field: "C", kind: directClass, omit: "omitzero"}},
		{"another type with omitzero", directMember{name: "o", field: "O", kind: directEncode, encodeKind: encodeOther, omit: "omitzero"}},
		{"another type with omitempty", directMember{name: "o", field: "O", kind: directEncode, encodeKind: encodeOther, omit: "omitempty"}},
		{"a type parameter with omitzero", directMember{name: "p", field: "P", kind: directParam, omit: "omitzero"}},
		{"a type parameter with no option", directMember{name: "p", field: "P", kind: directParam}},
		{"a list of numbers", directMember{name: "l", field: "L", kind: directNumber, number: "Real", slice: true}},
	} {
		var b strings.Builder
		if err := renderDirectMember(&b, "x", tc.member); err == nil {
			t.Errorf("renderDirectMember(%s) rendered\n%s\nwant a refusal", tc.name, b.String())
		}
	}

	if _, err := parseDeclaredField("\tN int `json:\"n,string\"`\n"); err == nil {
		t.Error("parseDeclaredField accepted the string tag option, which the direct writer does not apply")
	}

	prop := func(name string) bmm.Property {
		p := &bmm.SingleProperty{TypeName: "String"}
		p.Name = name
		return p
	}
	field := func(name string, depth int) wireField {
		return wireField{ef: emittedField{Prop: prop(name)}, depth: depth}
	}
	if _, err := dominantFields("X", []wireField{field("a", 1), field("a", 1)}); err == nil {
		t.Error("dominantFields kept a name two fields share at one depth, which json/v2 writes for neither")
	}
	kept, err := dominantFields("X", []wireField{field("a", 2), field("b", 1), field("a", 1), field("c", 0)})
	if err != nil {
		t.Fatalf("dominantFields: %v", err)
	}
	var names []string
	for _, f := range kept {
		names = append(names, f.ef.Prop.PropertyName())
	}
	if got := strings.Join(names, ","); got != "b,a,c" {
		t.Errorf("dominantFields kept %s, want b,a,c: the shallowest of each name, in declaration order", got)
	}
}

// TestREQ052DirectSlotCoversTheRegistry pins that the jsonSlot each target
// declares classifies a value of every class the type registry holds for it
// as a class, through a pointer or by value, and a nil pointer to one as
// absent. A class it missed would send every value holding it to the wrapper,
// which writes the same bytes, so only this census sees the loss. The AOM 1.4
// target asks the RM target's registry about the RM classes it holds.
func TestREQ052DirectSlotCoversTheRegistry(t *testing.T) {
	t.Parallel()
	for _, target := range []Target{TargetRM, TargetAOM14} {
		plan := buildPlanWithITSXML(t, target)
		helpers, err := renderDirectHelpers(plan)
		if err != nil {
			t.Fatalf("renderDirectHelpers(%s): %v", target.GoPackage, err)
		}
		_, slot, ok := strings.Cut(helpers, "func jsonSlot(v any) uint8 {\n")
		if !ok {
			t.Fatalf("%s declares no jsonSlot:\n%s", target.GoPackage, helpers)
		}
		slot, _, _ = strings.Cut(slot, "\n}\n")
		at := strings.Index(slot, "\tcase "+firstArm(t, plan)+",")
		if at < 0 {
			t.Fatalf("%s jsonSlot has no list of class values:\n%s", target.GoPackage, slot)
		}
		values := slot[at+len("\tcase "):]
		var arms int
		for _, pc := range plan.ConcreteClasses {
			names, err := reverseRegistryArms(plan, pc)
			if err != nil {
				t.Fatal(err)
			}
			for _, arm := range names {
				arms++
				if !strings.Contains(slot, "\tcase *"+arm+":\n\t\tif x == nil {\n\t\t\treturn jsonSlotAbsent\n\t\t}\n\t\treturn jsonSlotClass\n") {
					t.Errorf("%s jsonSlot does not classify *%s", target.GoPackage, arm)
				}
				if !strings.Contains(values, arm+",") && !strings.Contains(values, arm+":") {
					t.Errorf("%s jsonSlot does not classify a %s value", target.GoPackage, arm)
				}
			}
		}
		if arms < 50 {
			t.Errorf("%s: %d registered classes, want the target's inventory", target.GoPackage, arms)
		}
		external := strings.Contains(slot, "rm.RMTypeName(v)") && strings.Contains(slot, "rm.IsTypedNil(v)")
		if want := target.ExternalQualifier != ""; external != want {
			t.Errorf("%s jsonSlot asks the RM registry: %v, want %v", target.GoPackage, external, want)
		}
	}
}

// firstArm returns the first Go type jsonSlot's list of class values names.
func firstArm(t *testing.T, plan *Plan) string {
	t.Helper()
	names, err := reverseRegistryArms(plan, plan.ConcreteClasses[0])
	if err != nil {
		t.Fatal(err)
	}
	return names[0]
}
