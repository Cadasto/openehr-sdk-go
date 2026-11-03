package bmmgen

// REQ-176 § Shapes, REQ-056: the encoder of a choice member leaves out, when
// empty, a mandatory member of a shape that the schema type of the member's
// element makes optional, such as the `value` of the DV_CODED_TEXT shape of
// ORDINAL.symbol.

import (
	"slices"
	"strings"
	"testing"
)

// TestXSDOptionalElements pins which elements a type makes optional: those
// whose every declaration has minOccurs="0" on itself or on a particle around
// it, each branch of an xs:choice counted as a particle of its own, through
// the groups the type refers to; and the type of a member's element, found in
// the owner's particle or its base's.
func TestXSDOptionalElements(t *testing.T) {
	root := writeSchemaTree(t, map[string]string{"s/a.xsd": xsdDoc(`
		<xs:group name="G"><xs:sequence><xs:element name="in_group" minOccurs="0"/></xs:sequence></xs:group>
		<xs:complexType name="SYMBOL"><xs:choice>
			<xs:sequence>
				<xs:element name="value" minOccurs="0"/>
				<xs:element name="code"/>
				<xs:sequence minOccurs="0"><xs:element name="in_optional_sequence"/></xs:sequence>
				<xs:group ref="G"/>
			</xs:sequence>
			<xs:sequence><xs:element name="terminology"/><xs:element name="twice" minOccurs="0"/></xs:sequence>
			<xs:sequence><xs:element name="other"/><xs:element name="twice"/></xs:sequence>
		</xs:choice></xs:complexType>
		<xs:complexType name="BASE"><xs:sequence><xs:element name="symbol" type="SYMBOL"/></xs:sequence></xs:complexType>
		<xs:complexType name="ORDINAL"><xs:complexContent><xs:extension base="BASE"><xs:sequence>
			<xs:element name="value" type="xs:int"/></xs:sequence></xs:extension></xs:complexContent></xs:complexType>`)})
	set, err := loadXSDSet(root, []string{"s/a.xsd"})
	if err != nil {
		t.Fatalf("loadXSDSet: %v", err)
	}
	if got, want := set.optionalElements("SYMBOL"), []string{"in_group", "in_optional_sequence", "value"}; !slices.Equal(got, want) {
		t.Errorf("optionalElements(SYMBOL) = %q; want %q", got, want)
	}
	if got, ok := set.elementType("ORDINAL", "symbol"); !ok || got != "SYMBOL" {
		t.Errorf("elementType(ORDINAL, symbol) = %q, %v; want SYMBOL, from the base's particle", got, ok)
	}
	if got, ok := set.elementType("ORDINAL", "nothing"); ok {
		t.Errorf("elementType(ORDINAL, nothing) = %q; want none", got)
	}
}

// TestChoice_REQ176_EmptySymbolValueIsLeftOut pins REQ-176's rule for the
// DV_CODED_TEXT shape of ORDINAL.symbol and SCALE.symbol: OpenehrProfile.xsd's
// ORDINAL_SYMBOL makes `value` optional, which DV_CODED_TEXT holds as a
// mandatory member, so the encoder writes that shape through
// canxml.EncodeElementOmittingEmpty, leaving `value` out when empty, and the
// CODE_PHRASE shape, whose mandatory members the schema type requires, as it
// writes any value. Without the schemas no member is left out.
func TestChoice_REQ176_EmptySymbolValueIsLeftOut(t *testing.T) {
	const (
		omitting = "canxml.EncodeElementOmittingEmpty(_e, %s, xml.StartElement{Name: xml.Name{Local: \"symbol\"}}, \"value\")"
		plain    = "_e.EncodeElement(%s, xml.StartElement{Name: xml.Name{Local: \"symbol\"}})"
	)
	encoder := func(t *testing.T, p *Plan, recv string) string {
		t.Helper()
		for _, f := range p.Files {
			if f.FileBase != "openehr_archetype_profile" {
				continue
			}
			b, err := RenderMarshalXMLFile(p, f)
			if err != nil {
				t.Fatalf("RenderMarshalXMLFile(%s): %v", f.FileBase, err)
			}
			src := string(b)
			i := strings.Index(src, "func ("+recv+") MarshalXML")
			if i < 0 {
				t.Fatalf("no %s MarshalXML in %s", recv, f.FileBase)
			}
			src = src[i:]
			return src[:strings.Index(src, "\n}\n")]
		}
		t.Fatal("the plan has no openehr_archetype_profile file")
		return ""
	}
	withSchemas := buildPlanWithITSXML(t, TargetAOM14)
	for _, recv := range []string{"o *Ordinal", "s *Scale"} {
		src := encoder(t, withSchemas, recv)
		for _, want := range []string{
			"case *rm.DVCodedText:\n\t\tif err := " + strings.Replace(omitting, "%s", "_v", 1),
			"case rm.DVCodedText:\n\t\tif err := " + strings.Replace(omitting, "%s", "&_v", 1),
			"case *rm.CodePhrase:\n\t\tif err := " + strings.Replace(plain, "%s", "_v", 1),
			"case rm.CodePhrase:\n\t\tif err := " + strings.Replace(plain, "%s", "&_v", 1),
		} {
			if !strings.Contains(src, want) {
				t.Errorf("%s MarshalXML lacks\n%s\nin\n%s", recv, want, src)
			}
		}
		if control := encoder(t, planWithoutSchemas(t, TargetAOM14), recv); strings.Contains(control, "EncodeElementOmittingEmpty") {
			t.Errorf("%s MarshalXML built without the schemas leaves a member out:\n%s", recv, control)
		}
	}
}
