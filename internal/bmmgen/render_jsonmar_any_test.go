package bmmgen

// REQ-176, REQ-052, REQ-056 — the generated canonical-JSON encoder of a member
// typed Any refuses a value its decoder would not read back, as the
// canonical-XML encoder refuses it: a choice member anything but one of its
// shapes, and any other Any member anything but a value of a class the type
// registry constructs, a typed string or a string, the closed set one
// generated function holds for both encoders.

import (
	"go/format"
	"slices"
	"strings"
	"testing"
)

// TestREQ176_REQ052_AnyMemberJSONEncoderChecks pins the check the AOM 1.4
// encoders run before they write ORDINAL's and SCALE's symbol, which takes the
// DV_CODED_TEXT or the CODE_PHRASE shape, and EXPR_LEAF's item: for a symbol a
// type switch on the member that lets each shape through by pointer or by
// value, and nil; for the item the closed set of anyMemberValue, with no
// interface that any type with a BMMName method satisfies. Each refuses any
// other value with the member's name and the Go type it holds.
func TestREQ176_REQ052_AnyMemberJSONEncoderChecks(t *testing.T) {
	plan := buildPlanWithITSXML(t, TargetAOM14)
	for class, want := range map[string][]string{
		"ORDINAL": {
			"switch _v := o.Symbol.(type) {",
			"case nil, *rm.DVCodedText, *rm.CodePhrase, rm.DVCodedText, rm.CodePhrase:",
			`return fmt.Errorf("canjson: ORDINAL.symbol holds %T, which is none of the shapes its canonical XML takes", _v)`,
		},
		"SCALE": {
			"switch _v := s.Symbol.(type) {",
			`return fmt.Errorf("canjson: SCALE.symbol holds %T, which is none of the shapes its canonical XML takes", _v)`,
		},
		"EXPR_LEAF": {
			"if _, ok := anyMemberValue(e.Item); !ok {",
			`return fmt.Errorf("canjson: EXPR_LEAF.item holds %T, which is no registered class, typed string or string", e.Item)`,
		},
	} {
		pc, ok := plan.Classes[class]
		if !ok {
			t.Fatalf("plan has no class %s", class)
		}
		got, err := renderMarshalJSON(plan, pc)
		if err != nil {
			t.Fatalf("renderMarshalJSON(%s): %v", class, err)
		}
		body := got[strings.Index(got, "MarshalJSONTo(enc"):]
		check, _, _ := strings.Cut(body, "json.MarshalEncode(")
		for _, w := range want {
			if !strings.Contains(check, w) {
				t.Errorf("%s's MarshalJSONTo lacks %q before it writes the value:\n%s", class, w, got)
			}
		}
		if strings.Contains(check, "BMMName()") {
			t.Errorf("%s's MarshalJSONTo accepts any type with a BMMName method:\n%s", class, got)
		}
	}
	pc := plan.Classes["C_STRING"]
	got, err := renderMarshalJSON(plan, pc)
	if err != nil {
		t.Fatalf("renderMarshalJSON(C_STRING): %v", err)
	}
	if strings.Contains(got, "switch _v") {
		t.Errorf("C_STRING, which has no Any member, checks one:\n%s", got)
	}
}

// TestREQ176_REQ052_REQ056_AnyMemberClosedSet pins the closed set of values a
// member typed Any that takes no choice of shapes holds, which the AOM 1.4
// target's package declares once, as anyMemberValue in the canonical-JSON
// companion of the file that holds EXPR_LEAF: the classes the type registry
// constructs where the package is linked in, the AOM 1.4 target's and the RM
// target's, each by pointer and by value, the by-value one returned as a
// pointer to it, plus a string and a typed string; and no interface. The RM
// target, whose classes have no such member, declares none.
func TestREQ176_REQ052_REQ056_AnyMemberClosedSet(t *testing.T) {
	aom := buildPlanWithITSXML(t, TargetAOM14)
	rmPlan := buildPlanWithITSXML(t, TargetRM)
	var want []string
	for _, pc := range append(slices.Clone(aom.ConcreteClasses), rmPlan.ConcreteClasses...) {
		want = append(want, pc.BMMName)
	}
	slices.Sort(want)
	var got []string
	for _, pc := range anyMemberClasses(aom) {
		got = append(got, pc.BMMName)
	}
	if !slices.Equal(got, want) {
		t.Errorf("anyMemberClasses(aom14) = %q\nwant the classes both targets register, %q", got, want)
	}

	file, err := anyMemberFile(aom)
	if err != nil || file == nil || file.FileBase != "archetype_assertion" {
		t.Fatalf("anyMemberFile(aom14) = %v, %v; want archetype_assertion", file, err)
	}
	for _, f := range aom.Files {
		src, err := RenderMarshalJSONFile(aom, f)
		if err != nil {
			t.Fatalf("RenderMarshalJSONFile(%s): %v", f.FileBase, err)
		}
		declares := strings.Contains(string(src), "func anyMemberValue(v any) (any, bool) {")
		if declares != (f == file) {
			t.Errorf("%s_jsonmar_gen.go declares anyMemberValue: %v; want %v", f.FileBase, declares, f == file)
		}
	}
	helper := renderAnyMemberValue(aom)
	for _, w := range []string{
		"case nil, string, xmlschema.TypedString, *xmlschema.TypedString,\n",
		"\t\t*CString,\n",
		"\t\t*rm.DVText,\n",
		"\t\t*rm.DVInterval[rm.DVOrdered],\n",
		"\t\treturn v, true\n",
		"\tcase CString:\n\t\treturn &x, true\n",
		"\tcase rm.DVText:\n\t\treturn &x, true\n",
		"\tcase rm.DVInterval[rm.DVOrdered]:\n\t\treturn &x, true\n",
		"\treturn nil, false\n",
	} {
		if !strings.Contains(helper, w) {
			t.Errorf("anyMemberValue lacks %q:\n%s", w, helper)
		}
	}
	if strings.Contains(helper, "interface") {
		t.Errorf("anyMemberValue accepts an interface:\n%s", helper)
	}

	if file, err := anyMemberFile(rmPlan); err != nil || file != nil {
		t.Errorf("anyMemberFile(rm) = %v, %v; want none", file, err)
	}
}

// TestREQ176_REQ056_AnyMemberXMLEncoder pins the canonical-XML encoder of a
// member typed Any that takes no choice of shapes, mandatory or optional: a
// string or a typed string is written as it is, nil writes nothing, and any
// other value is written as the class value anyMemberValue returns for it, a
// pointer where the member holds it by value, or refused with
// canxml.ErrInvalidShape, naming the member and the Go type it holds.
func TestREQ176_REQ056_AnyMemberXMLEncoder(t *testing.T) {
	for _, mandatory := range []bool{true, false} {
		got := marshalXMLAny("e", "Item", "item", "EXPR_LEAF.item", mandatory)
		for _, w := range []string{
			"case nil:\n",
			"case string, xmlschema.TypedString, *xmlschema.TypedString:\n",
			"default:\n",
			"_c, _ok := anyMemberValue(_v)\n",
			`return fmt.Errorf("%w: EXPR_LEAF.item holds %T, which is no registered class, typed string or string", canxml.ErrInvalidShape, _v)`,
			`canxml.EncodePoly(_e, "item", _c)`,
		} {
			if !strings.Contains(got, w) {
				t.Errorf("marshalXMLAny(mandatory %v) lacks %q:\n%s", mandatory, w, got)
			}
		}
		if strings.Contains(got, `canxml.EncodePoly(_e, "item", _v)`) {
			t.Errorf("marshalXMLAny(mandatory %v) writes the value it holds, not the one anyMemberValue returns:\n%s", mandatory, got)
		}
	}
}

// TestREQ176_REQ056_OptionalAnyMemberXMLEncoderParses pins that the
// canonical-XML encoder of a member typed Any is Go source for an optional
// member, held as a *any, as for a mandatory one: the type switch reads the
// value the pointer holds, which a type switch guard takes only in
// parentheses.
func TestREQ176_REQ056_OptionalAnyMemberXMLEncoderParses(t *testing.T) {
	for _, mandatory := range []bool{true, false} {
		body := marshalXMLAny("e", "Item", "item", "EXPR_LEAF.item", mandatory)
		src := "package p\n\nfunc (e *T) MarshalXML(_e *xml.Encoder, _start xml.StartElement) error {\n" + body + "\treturn nil\n}\n"
		if _, err := format.Source([]byte(src)); err != nil {
			t.Errorf("marshalXMLAny(mandatory %v) is no Go source: %v\n%s", mandatory, err, src)
		}
	}
}
