package bmmgen

// REQ-176 § Shapes and § Round trip, REQ-056: the generated package registers
// with the canonical-XML codec the type aliases its roots name for the classes
// it owns, and the root element of each class whose ITS-XML schema declares a
// global element for its type under another name than the class's.

import (
	"maps"
	"strings"
	"testing"
)

// TestXMLNames_REQ176_REQ056_AOM14RegistersTheAliasAndTheRootElement pins the
// AOM 1.4 target's xml_names_gen.go: it registers T_ARCHETYPE_ROOT as an alias
// of C_ARCHETYPE_ROOT and `template` as the root element of
// OPERATIONAL_TEMPLATE, from Template.xsd's global element, and no root
// element for ARCHETYPE, whose global element `archetype` is the encoder's
// default. Without the schemas the root element goes, which proves it comes
// from them; the RM target, which owns no alias and whose schemas declare no
// global element, writes no file.
func TestXMLNames_REQ176_REQ056_AOM14RegistersTheAliasAndTheRootElement(t *testing.T) {
	const (
		alias = `xmlschema.RegisterTypeAlias("T_ARCHETYPE_ROOT", "C_ARCHETYPE_ROOT")`
		root  = `xmlschema.RegisterRootElement("OPERATIONAL_TEMPLATE", "template")`
	)
	b, err := RenderXMLNamesFile(buildPlanWithITSXML(t, TargetAOM14))
	if err != nil {
		t.Fatalf("RenderXMLNamesFile(aom14): %v", err)
	}
	src := string(b)
	for _, want := range []string{alias, root} {
		if !strings.Contains(src, want) {
			t.Errorf("aom14 %s lacks %s:\n%s", xmlNamesFile, want, src)
		}
	}
	if strings.Contains(src, `RegisterRootElement("ARCHETYPE"`) {
		t.Errorf("aom14 %s registers ARCHETYPE's root element, which is the default:\n%s", xmlNamesFile, src)
	}

	b, err = RenderXMLNamesFile(planWithoutSchemas(t, TargetAOM14))
	if err != nil {
		t.Fatalf("RenderXMLNamesFile(aom14, no schemas): %v", err)
	}
	if src := string(b); !strings.Contains(src, alias) || strings.Contains(src, "RegisterRootElement") {
		t.Errorf("aom14 %s built without the schemas = \n%s\nwant the alias and no root element", xmlNamesFile, src)
	}

	b, err = RenderXMLNamesFile(buildPlanWithITSXML(t, TargetRM))
	if err != nil || b != nil {
		t.Errorf("RenderXMLNamesFile(rm) = %q, %v; want no file", b, err)
	}
}

// TestXMLNames_REQ176_AnAliasIsTheOwningTargets pins that a type alias is
// applied where the target owns the class it names: the fixture root's alias
// of ROOT_OBJECT is kept and registered by a target that owns ROOT_OBJECT, and
// neither by one that holds ROOT_OBJECT as another target's class, although
// the keys merge across the include chain, so the alias is registered once.
func TestXMLNames_REQ176_AnAliasIsTheOwningTargets(t *testing.T) {
	t.Parallel()
	const reg = `xmlschema.RegisterTypeAlias("LEGACY_ROOT_OBJECT", "ROOT_OBJECT")`
	owner := extensionPlan(t, extensionRootID)
	if want := map[string]string{"LEGACY_ROOT_OBJECT": "ROOT_OBJECT"}; !maps.Equal(owner.extensions.typeAliases, want) {
		t.Fatalf("the owning target's type aliases = %v; want %v", owner.extensions.typeAliases, want)
	}
	b, err := RenderXMLNamesFile(owner)
	if err != nil || !strings.Contains(string(b), reg) {
		t.Errorf("the owning target's %s = %s, %v; want it to hold %s", xmlNamesFile, b, err, reg)
	}

	other := extensionTarget(extensionRootID)
	other.OwnPackagePrefixes = []string{"org.test.views"}
	other.ExternalQualifier, other.ExternalImport = "base", "example.org/base"
	p, err := BuildPlanForTarget(t.Context(), other, NewResolver(extensionFixtures))
	if err != nil {
		t.Fatalf("BuildPlanForTarget(a target owning org.test.views only) = %v", err)
	}
	if !p.Classes["ROOT_OBJECT"].External {
		t.Fatal("the target still owns ROOT_OBJECT; the case proves nothing")
	}
	if len(p.extensions.typeAliases) != 0 {
		t.Errorf("a target that does not own ROOT_OBJECT keeps the aliases %v; want none", p.extensions.typeAliases)
	}
	if b, err := RenderXMLNamesFile(p); err != nil || b != nil {
		t.Errorf("a target that does not own ROOT_OBJECT writes %s = %s, %v; want no file", xmlNamesFile, b, err)
	}
}

// planWithoutSchemas builds the plan of target over the vendored BMM with no
// ITS-XML schemas, the control of a rule the schemas drive.
func planWithoutSchemas(t *testing.T, target Target) *Plan {
	t.Helper()
	p, err := BuildPlanForTarget(t.Context(), target, testResolver())
	if err != nil {
		t.Fatalf("BuildPlanForTarget(%s): %v", target.GoPackage, err)
	}
	return p
}

// TestXSDGlobalElements pins the reading of global elements: an element at
// the top of a schema file names the root element of a document of its type,
// for that type only, and a second global element of another name for one
// type, or a global element with no named type, is refused.
func TestXSDGlobalElements(t *testing.T) {
	root := writeSchemaTree(t, map[string]string{"s/a.xsd": xsdDoc(
		`<xs:element name="template" type="OPERATIONAL_TEMPLATE"/>` +
			`<xs:complexType name="OPERATIONAL_TEMPLATE"><xs:sequence/></xs:complexType>` +
			`<xs:complexType name="SUB"><xs:complexContent><xs:extension base="OPERATIONAL_TEMPLATE"/></xs:complexContent></xs:complexType>`)})
	set, err := loadXSDSet(root, []string{"s/a.xsd"})
	if err != nil {
		t.Fatalf("loadXSDSet: %v", err)
	}
	order := &xmlChildOrder{sets: []*xsdSet{set}}
	if got, ok := order.rootElementFor("OPERATIONAL_TEMPLATE"); !ok || got != "template" {
		t.Errorf("rootElementFor(OPERATIONAL_TEMPLATE) = %q, %v; want template", got, ok)
	}
	if got, ok := order.rootElementFor("SUB"); ok {
		t.Errorf("rootElementFor(SUB) = %q; want none, a type that extends OPERATIONAL_TEMPLATE takes no global element from it", got)
	}

	for name, body := range map[string]string{
		"two global elements of one type": `<xs:element name="template" type="T"/><xs:element name="other" type="T"/>`,
		"an anonymous global element":     `<xs:element name="template"><xs:complexType/></xs:element>`,
	} {
		root := writeSchemaTree(t, map[string]string{"s/a.xsd": xsdDoc(body + `<xs:complexType name="T"/>`)})
		if _, err := loadXSDSet(root, []string{"s/a.xsd"}); err == nil || !strings.Contains(err.Error(), "global element") {
			t.Errorf("loadXSDSet with %s = %v; want a refusal naming the global element", name, err)
		}
	}
}
