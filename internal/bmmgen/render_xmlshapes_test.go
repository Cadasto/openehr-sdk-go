package bmmgen

import (
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/bmm"
)

// TestREQ056_CheckXMLMemberShapes pins the member shapes the generator
// accepts for a target: the AOM 1.4 target's own pass, and each one the
// codecs could not honour is refused (REQ-056).
func TestREQ056_CheckXMLMemberShapes(t *testing.T) {
	if err := checkXMLMemberShapes(aom14PlanWithSchemas(t)); err != nil {
		t.Fatalf("checkXMLMemberShapes(aom14) = %v; want nil", err)
	}
	const (
		noString = "no mandatory String member a concrete class of the aom14 target declares"
		noPhrase = "no mandatory CODE_PHRASE member a concrete class of the aom14 target declares"
	)
	for _, tc := range []struct {
		name   string
		shapes map[string]XMLMemberShape
		want   string
	}{
		{"unknown shape", map[string]XMLMemberShape{"C_QUANTITY.property": 0}, "unknown shape 0"},
		{"unknown class", map[string]XMLMemberShape{"C_NO_SUCH_CLASS.property": XMLCodePhraseString}, noString},
		{"abstract class", map[string]XMLMemberShape{"C_OBJECT.node_id": XMLCodePhraseString}, noString},
		{"member an ancestor declares", map[string]XMLMemberShape{"C_QUANTITY.node_id": XMLCodePhraseString}, noString},
		{"optional member", map[string]XMLMemberShape{"C_CODED_TEXT.reference": XMLCodePhraseString}, noString},
		{"CODE_PHRASE string that is no String", map[string]XMLMemberShape{"ORDINAL.symbol": XMLCodePhraseString}, noString},
		{"coded text that is no CODE_PHRASE", map[string]XMLMemberShape{"C_QUANTITY.property": XMLCodePhraseOrCodedText}, noPhrase},
		{"member also an identifier element", map[string]XMLMemberShape{"C_CODED_TEXT.terminology": XMLCodePhraseString}, "also written as an identifier element"},
		{"element the schema type lacks", map[string]XMLMemberShape{"ARCHETYPE_TERM.code": XMLCodePhraseString}, "the ITS-XML schema type of ARCHETYPE_TERM declares no element code"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			plan := aom14PlanWithSchemas(t)
			plan.Target.XMLMemberShapes = tc.shapes
			err := checkXMLMemberShapes(plan)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("checkXMLMemberShapes(%v) = %v; want an error containing %q", tc.shapes, err, tc.want)
			}
		})
	}
}

// TestREQ056_CheckXMLMemberShapesCodePhrase pins that a member shape is
// refused when the CODE_PHRASE class its codecs build on cannot serve it
// (REQ-056): a plan without the class, a CODE_PHRASE without the
// terminology_id or code_string the CODE_PHRASE string is written into, and
// one with a member named as an element of the DV_CODED_TEXT shape, whose
// decode cases it would share.
func TestREQ056_CheckXMLMemberShapesCodePhrase(t *testing.T) {
	for _, tc := range []struct {
		name  string
		shape XMLMemberShape
		edit  func(sc *bmm.SimpleClass) // nil drops the class
		want  string
	}{
		{"no CODE_PHRASE", XMLCodePhraseString, nil, "CODE_PHRASE is no class with a canonical-XML codec"},
		{"no CODE_PHRASE for a coded text", XMLCodePhraseOrCodedText, nil, "CODE_PHRASE is no class with a canonical-XML codec"},
		{"no terminology_id", XMLCodePhraseString, func(sc *bmm.SimpleClass) { delete(sc.Properties, "terminology_id") }, "CODE_PHRASE has no terminology_id that is a TERMINOLOGY_ID"},
		{"no code_string", XMLCodePhraseString, func(sc *bmm.SimpleClass) { delete(sc.Properties, "code_string") }, "CODE_PHRASE has no String code_string"},
		{"a member named value", XMLCodePhraseOrCodedText, func(sc *bmm.SimpleClass) { addStringProperty(sc, "value") }, "CODE_PHRASE has a member value, an element of the DV_CODED_TEXT shape"},
		{"a member named defining_code", XMLCodePhraseOrCodedText, func(sc *bmm.SimpleClass) { addStringProperty(sc, "defining_code") }, "CODE_PHRASE has a member defining_code, an element of the DV_CODED_TEXT shape"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			plan := aom14PlanWithSchemas(t)
			key := "C_QUANTITY.property"
			if tc.shape == XMLCodePhraseOrCodedText {
				key = "ORDINAL.symbol"
			}
			plan.Target.XMLMemberShapes = map[string]XMLMemberShape{key: tc.shape}
			if tc.edit == nil {
				delete(plan.Classes, codePhraseClass)
			} else {
				pc := *plan.Classes[codePhraseClass]
				sc := *pc.Class.(*bmm.SimpleClass)
				sc.Properties = maps.Clone(sc.Properties)
				sc.PropertyOrder = slices.Clone(sc.PropertyOrder)
				tc.edit(&sc)
				pc.Class = &sc
				plan.Classes[codePhraseClass] = &pc
			}
			err := checkXMLMemberShapes(plan)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("checkXMLMemberShapes(%s) = %v; want an error containing %q", tc.name, err, tc.want)
			}
		})
	}
}

// addStringProperty declares a single String property named name on sc, last
// in its declaration order.
func addStringProperty(sc *bmm.SimpleClass, name string) {
	p := &bmm.SingleProperty{TypeName: "String"}
	p.Name = name
	sc.Properties[name] = p
	sc.PropertyOrder = append(sc.PropertyOrder, name)
}

// TestREQ056_RenderXMLMemberShapes pins the codecs the AOM 1.4 target renders
// for its two member shapes (REQ-056, REQ-170, REQ-175). C_QUANTITY's
// encoder writes a property holding `::` as a CODE_PHRASE and any other as
// text, and its decoder reads the property through
// xmlunmatched.DecodeCodePhraseString. ORDINAL's decoder hands its symbol to
// the one function rendered beside it, which describes the element with
// defining_code inline, reads the CODE_PHRASE's own members, hands
// defining_code to the CODE_PHRASE decoder, drops the DV_CODED_TEXT's other
// elements through xmlunmatched.Drop and skips nothing itself.
func TestREQ056_RenderXMLMemberShapes(t *testing.T) {
	plan := aom14PlanWithSchemas(t)
	render := func(class string) (enc, dec string) {
		t.Helper()
		pc := plan.Classes[class]
		fields, err := effectiveFields(plan, pc)
		if err != nil {
			t.Fatalf("effectiveFields(%s): %v", class, err)
		}
		ordered, err := xmlChildFields(plan, pc, fields)
		if err != nil {
			t.Fatalf("xmlChildFields(%s): %v", class, err)
		}
		if enc, err = renderMarshalXML(plan, pc, ordered); err != nil {
			t.Fatalf("renderMarshalXML(%s): %v", class, err)
		}
		if dec, err = renderUnmarshalXML(plan, pc, fields); err != nil {
			t.Fatalf("renderUnmarshalXML(%s): %v", class, err)
		}
		return enc, dec
	}

	enc, dec := render("C_QUANTITY")
	for _, want := range []string{
		`if _id, _code, _ok := strings.Cut(c.Property, "::"); _ok {`,
		`_e.EncodeElement(&rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: _id}, CodeString: _code}, xml.StartElement{Name: xml.Name{Local: "property"}})`,
		`} else if err := _e.EncodeElement(c.Property, xml.StartElement{Name: xml.Name{Local: "property"}}); err != nil {`,
	} {
		if !strings.Contains(enc, want) {
			t.Errorf("C_QUANTITY XML encoder lacks %q:\n%s", want, enc)
		}
	}
	if arm := xmlCaseArm(dec, "property"); !strings.Contains(arm, "xmlunmatched.DecodeCodePhraseString(_dec, &_t, &c.Property)") {
		t.Errorf("C_QUANTITY property arm = %q; want it to decode through xmlunmatched.DecodeCodePhraseString", arm)
	}

	_, dec = render("ORDINAL")
	if arm := xmlCaseArm(dec, "symbol"); !strings.Contains(arm, "unmarshalXMLOrdinalSymbol(_dec, _t, &o.Symbol)") {
		t.Errorf("ORDINAL symbol arm = %q; want it to hand the element to unmarshalXMLOrdinalSymbol", arm)
	}
	fn := dec[strings.Index(dec, "\nfunc unmarshalXMLOrdinalSymbol("):]
	for _, want := range []string{
		"var xmlShapeOrdinalSymbol = xmlunmatched.Class{\n\tName: \"CODE_PHRASE\",\n\tRequired: []string{\"terminology_id\", \"code_string\"},\n\tInline: []string{\"defining_code\"},\n}\n",
		"func unmarshalXMLOrdinalSymbol(_dec *xml.Decoder, _start xml.StartElement, c *rm.CodePhrase) error {",
		"xmlunmatched.Enter(_dec, _start, &xmlShapeOrdinalSymbol)",
	} {
		if !strings.Contains(dec, want) {
			t.Errorf("ORDINAL XML decoder lacks %q:\n%s", want, dec)
		}
	}
	for _, tc := range []struct{ element, want string }{
		{"terminology_id", "_dec.DecodeElement(&c.TerminologyID, &_t)"},
		{"code_string", "xmlunmatched.DecodeElement(_dec, &_t, &c.CodeString)"},
		{"preferred_term", "c.PreferredTerm = &_v"},
		{"defining_code", "_dec.DecodeElement(c, &_t)"},
		{"value", "xmlunmatched.Drop(_dec)"},
	} {
		if arm := xmlCaseArm(fn, tc.element); !strings.Contains(arm, tc.want) {
			t.Errorf("unmarshalXMLOrdinalSymbol %s arm = %q; want it to contain %q", tc.element, arm, tc.want)
		}
	}
	if want := "\t\t\tcase \"value\", \"hyperlink\", \"formatting\", \"mappings\", \"language\", \"encoding\":\n"; !strings.Contains(fn, want) {
		t.Errorf("unmarshalXMLOrdinalSymbol lacks the arm %q that drops the DV_CODED_TEXT's other elements:\n%s", want, fn)
	}
	if strings.Contains(fn, "_dec.Skip()") {
		t.Errorf("unmarshalXMLOrdinalSymbol skips an element itself; content it does not read must go through xmlunmatched:\n%s", fn)
	}

	// The function is rendered once, beside the decoder of the class that
	// declares the member.
	count := 0
	for _, f := range plan.Files {
		body, err := RenderUnmarshalXMLFile(plan, f)
		if err != nil {
			t.Fatalf("RenderUnmarshalXMLFile(%s): %v", f.FileBase, err)
		}
		count += strings.Count(string(body), "\nfunc unmarshalXMLOrdinalSymbol(")
	}
	if count != 1 {
		t.Errorf("the AOM 1.4 target renders unmarshalXMLOrdinalSymbol %d times; want once", count)
	}
}
