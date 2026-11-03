package bmmgen

// REQ-176, REQ-052 — the generated canonical-JSON encoder of a member typed
// Any refuses a value its decoder would not read back, as the canonical-XML
// encoder refuses it: a choice member anything but one of its shapes, and any
// other Any member anything but a class value, a typed string or a string.

import (
	"strings"
	"testing"
)

// TestREQ176_REQ052_AnyMemberJSONEncoderChecks pins the check the AOM 1.4
// encoders run before they write ORDINAL's and SCALE's symbol, which takes the
// DV_CODED_TEXT or the CODE_PHRASE shape, and EXPR_LEAF's item: a type switch
// on the member that lets each shape through by pointer or by value, or a
// string, a typed string or a class value, and nil, and refuses any other with
// the member's name and the Go type it holds.
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
			"switch _v := e.Item.(type) {",
			"case nil, string, xmlschema.TypedString, *xmlschema.TypedString, interface{ BMMName() string }:",
			`return fmt.Errorf("canjson: EXPR_LEAF.item holds %T, which is no class value, typed string or string", _v)`,
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
