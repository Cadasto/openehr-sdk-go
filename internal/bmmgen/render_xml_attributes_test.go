package bmmgen

// REQ-056 — the generated canonical-XML codec writes and reads as an XML
// attribute each member the class's ITS-XML schema type declares as one,
// found through the type's extension bases; an attribute that names no
// member is recorded as the key of the Hash that holds the class.

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
)

// buildPlanWithITSXML builds the plan of target over the vendored BMM and
// gives it the vendored ITS-XML schemas, as [Run] does.
func buildPlanWithITSXML(t *testing.T, target Target) *Plan {
	t.Helper()
	return buildPlanWithSchemas(t, target, testITSXML)
}

// buildPlanWithSchemas builds the plan of target over the vendored BMM and
// gives it the target's ITS-XML schema sets, read from dir.
func buildPlanWithSchemas(t *testing.T, target Target, dir string) *Plan {
	t.Helper()
	plan, err := BuildPlanForTarget(context.Background(), target, testResolver())
	if err != nil {
		t.Fatalf("BuildPlanForTarget(%s): %v", target.GoPackage, err)
	}
	if plan.xmlOrder, err = (&itsXMLSchemas{dir: dir}).orderFor(target.ITSXMLSchemaSets); err != nil {
		t.Fatalf("ITS-XML schemas of %s under %s: %v", target.GoPackage, dir, err)
	}
	if err := plan.xmlOrder.checkAnonymousTypes(plan); err != nil {
		t.Fatalf("ITS-XML schemas of %s under %s: %v", target.GoPackage, dir, err)
	}
	return plan
}

// classXMLAttributes returns the XML attributes the plan gives class.
func classXMLAttributes(t *testing.T, plan *Plan, class string) xmlAttributeSet {
	t.Helper()
	pc, ok := plan.Classes[class]
	if !ok {
		t.Fatalf("plan has no class %s", class)
	}
	fields, err := effectiveFields(plan, pc)
	if err != nil {
		t.Fatalf("effectiveFields(%s): %v", class, err)
	}
	set, err := xmlAttributesOf(plan, pc, fields)
	if err != nil {
		t.Fatalf("xmlAttributesOf(%s): %v", class, err)
	}
	return set
}

// TestXMLAttributesFromVendoredSchemas pins the XML attributes the plan
// takes from the vendored schemas: `archetype_node_id` on every LOCATABLE
// descendant, by the base chain of its schema type; `code` on the AOM 1.4
// ARCHETYPE_TERM, which the AM Release-1.4 schema declares as an attribute;
// and the BASE `id` of the resource classes, which names no member and is
// recorded as the key of the Hash that holds the class.
func TestXMLAttributesFromVendoredSchemas(t *testing.T) {
	rmPlan := buildPlanWithITSXML(t, TargetRM)
	aomPlan := buildPlanWithITSXML(t, TargetAOM14)
	for _, tc := range []struct {
		plan          *Plan
		class         string
		members, keys []string
	}{
		{plan: rmPlan, class: "OBSERVATION", members: []string{"archetype_node_id"}},
		{plan: rmPlan, class: "ELEMENT", members: []string{"archetype_node_id"}},
		{plan: rmPlan, class: "COMPOSITION", members: []string{"archetype_node_id"}},
		{plan: rmPlan, class: "DV_TEXT"},
		{plan: rmPlan, class: "RESOURCE_DESCRIPTION_ITEM", keys: []string{"id"}},
		{plan: rmPlan, class: "TRANSLATION_DETAILS", keys: []string{"id"}},
		{plan: aomPlan, class: "ARCHETYPE_TERM", members: []string{"code"}},
		{plan: aomPlan, class: "C_COMPLEX_OBJECT"},
		// REQ-176: the AOM 1.4 root's classes, the two of T_VIEW's anonymous
		// types among them, take the attributes their schema types declare.
		{plan: aomPlan, class: "CodeDefinitionSet", members: []string{"language"}},
		{plan: aomPlan, class: "TermBindingSet", members: []string{"terminology"}},
		{plan: aomPlan, class: "TERM_BINDING_ITEM", members: []string{"code"}},
		{plan: aomPlan, class: "ConstraintBindingSet", members: []string{"terminology"}},
		{plan: aomPlan, class: "CONSTRAINT_BINDING_ITEM", members: []string{"code"}},
		{plan: aomPlan, class: "FLAT_ARCHETYPE_ONTOLOGY", members: []string{"archetype_id"}},
		{plan: aomPlan, class: "ANNOTATION", members: []string{"path"}},
		{plan: aomPlan, class: "T_VIEW_CONSTRAINT", members: []string{"path"}},
		{plan: aomPlan, class: "T_VIEW_CONSTRAINT_ITEM", members: []string{"id"}},
		{plan: aomPlan, class: "C_ARCHETYPE_ROOT"},
	} {
		t.Run(tc.plan.Target.GoPackage+"/"+tc.class, func(t *testing.T) {
			got := classXMLAttributes(t, tc.plan, tc.class)
			if !slices.Equal(got.members, tc.members) || !slices.Equal(got.keys, tc.keys) {
				t.Errorf("XML attributes of %s = members %q, keys %q; want members %q, keys %q",
					tc.class, got.members, got.keys, tc.members, tc.keys)
			}
		})
	}
}

// TestXMLAttributeCodecOfArchetypeTerm pins the codec the generator renders
// for the attribute the AM Release-1.4 schema gives ARCHETYPE_TERM: the
// encoder writes `code` as an attribute of the start element and no `code`
// element, and the decoder reads it from the start element and names it in
// the class's description, so it is not unmatched.
func TestXMLAttributeCodecOfArchetypeTerm(t *testing.T) {
	plan := buildPlanWithITSXML(t, TargetAOM14)
	pc := plan.Classes["ARCHETYPE_TERM"]
	fields, err := effectiveFields(plan, pc)
	if err != nil {
		t.Fatalf("effectiveFields(ARCHETYPE_TERM): %v", err)
	}
	ordered, err := xmlChildFields(plan, pc, fields)
	if err != nil {
		t.Fatalf("xmlChildFields(ARCHETYPE_TERM): %v", err)
	}
	enc, err := renderMarshalXML(plan, pc, ordered)
	if err != nil {
		t.Fatalf("renderMarshalXML(ARCHETYPE_TERM): %v", err)
	}
	if want := `_start.Attr = append(_start.Attr, xml.Attr{Name: xml.Name{Local: "code"}, Value: a.Code})`; !strings.Contains(enc, want) {
		t.Errorf("ARCHETYPE_TERM encoder lacks %q:\n%s", want, enc)
	}
	if bad := `xml.Name{Local: "code"}}`; strings.Contains(enc, bad) {
		t.Errorf("ARCHETYPE_TERM encoder writes a code element (%q):\n%s", bad, enc)
	}
	dec, err := renderUnmarshalXML(plan, pc, fields)
	if err != nil {
		t.Fatalf("renderUnmarshalXML(ARCHETYPE_TERM): %v", err)
	}
	for _, want := range []string{
		`if _a.Name.Local == "code" && _a.Name.Space == "" {`,
		`Attrs: []string{"code"},`,
	} {
		if !strings.Contains(dec, want) {
			t.Errorf("ARCHETYPE_TERM decoder lacks %q:\n%s", want, dec)
		}
	}
}

// fixtureSchemaTarget writes body as the one schema file of a fresh ITS-XML
// tree and returns that tree and a copy of target that reads only it.
func fixtureSchemaTarget(t *testing.T, target Target, body string) (string, Target) {
	t.Helper()
	dir := t.TempDir()
	writeSchemaFiles(t, dir, map[string]string{"fixture/fixture.xsd": xsdDoc(body)})
	target.ITSXMLSchemaSets = [][]string{{"fixture/fixture.xsd"}}
	return dir, target
}

// TestXMLAttributesFollowTheSchemaNotTheName pins that the schema, not a
// member's name, makes a member an XML attribute: under a fixture schema
// whose OBSERVATION declares no attribute and whose DV_TEXT declares `value`
// as one, OBSERVATION's `archetype_node_id` is an element and DV_TEXT's
// `value` an attribute.
func TestXMLAttributesFollowTheSchemaNotTheName(t *testing.T) {
	dir, target := fixtureSchemaTarget(t, TargetRM,
		`<xs:complexType name="OBSERVATION"><xs:sequence><xs:element name="name"/></xs:sequence></xs:complexType>`+
			`<xs:complexType name="DV_TEXT"><xs:sequence/><xs:attribute name="value" type="xs:string" use="required"/></xs:complexType>`)
	plan := buildPlanWithSchemas(t, target, dir)
	if got := classXMLAttributes(t, plan, "OBSERVATION"); len(got.members) > 0 || len(got.keys) > 0 {
		t.Errorf("OBSERVATION XML attributes = members %q, keys %q; want none, as the fixture schema declares none", got.members, got.keys)
	}
	if got := classXMLAttributes(t, plan, "DV_TEXT").members; !slices.Equal(got, []string{"value"}) {
		t.Errorf("DV_TEXT XML attribute members = %q; want [value], as the fixture schema declares", got)
	}
}

// TestXMLAttributeMemberNotStringRefused pins the refusal at generation of a
// member the schema declares as an XML attribute whose Go field is not a
// plain string, for which the generator has no rule: an Integer64, an
// optional String and a class-typed member each fail the run with
// errXMLAttributeNotString, naming the class and the member.
func TestXMLAttributeMemberNotStringRefused(t *testing.T) {
	for _, tc := range []struct {
		name, schema, wantMember string
	}{
		{
			name:       "an Integer64",
			schema:     `<xs:complexType name="DV_COUNT"><xs:sequence/><xs:attribute name="magnitude" type="xs:long" use="required"/></xs:complexType>`,
			wantMember: "DV_COUNT.magnitude",
		},
		{
			name:       "an optional String",
			schema:     `<xs:complexType name="FEEDER_AUDIT_DETAILS"><xs:sequence/><xs:attribute name="version_id" type="xs:string"/></xs:complexType>`,
			wantMember: "FEEDER_AUDIT_DETAILS.version_id",
		},
		{
			name:       "a class",
			schema:     `<xs:complexType name="ELEMENT"><xs:sequence/><xs:attribute name="value" type="xs:string"/></xs:complexType>`,
			wantMember: "ELEMENT.value",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir, target := fixtureSchemaTarget(t, TargetRM, tc.schema)
			_, err := Run(Options{
				ResourcesDir:  testResources,
				ITSXMLDir:     dir,
				OutDir:        t.TempDir(),
				Targets:       []Target{target},
				SkipInventory: true,
			})
			if !errors.Is(err, errXMLAttributeNotString) || !strings.Contains(err.Error(), tc.wantMember) {
				t.Errorf("Run with %s as an XML attribute = %v; want errXMLAttributeNotString naming %s", tc.wantMember, err, tc.wantMember)
			}
		})
	}
}

// TestXMLAttributesWithoutSchemas pins that a plan built without the ITS-XML
// schemas, as a test may build one, makes no member an XML attribute: the
// attribute set comes from the schemas alone.
func TestXMLAttributesWithoutSchemas(t *testing.T) {
	plan, err := BuildPlanForTarget(context.Background(), TargetRM, testResolver())
	if err != nil {
		t.Fatalf("BuildPlanForTarget: %v", err)
	}
	pc := plan.Classes["OBSERVATION"]
	fields, err := effectiveFields(plan, pc)
	if err != nil {
		t.Fatalf("effectiveFields(OBSERVATION): %v", err)
	}
	enc, err := renderMarshalXML(plan, pc, fields)
	if err != nil {
		t.Fatalf("renderMarshalXML(OBSERVATION): %v", err)
	}
	if strings.Contains(enc, "_start.Attr = append(") {
		t.Errorf("OBSERVATION encoder, from a plan without schemas, writes an XML attribute:\n%s", enc)
	}
}
