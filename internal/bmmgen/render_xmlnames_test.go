package bmmgen

import (
	"context"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/bmm"
)

// aom14PlanWithSchemas builds the AOM 1.4 plan with its ITS-XML schema sets
// loaded, as a generator run has it.
func aom14PlanWithSchemas(t *testing.T) *Plan {
	t.Helper()
	plan, err := BuildPlanForTarget(context.Background(), TargetAOM14, bmm.FSResolver{Root: testResources})
	if err != nil {
		t.Fatalf("BuildPlanForTarget(aom14): %v", err)
	}
	schemas := &itsXMLSchemas{dir: testITSXML}
	if plan.xmlOrder, err = schemas.orderFor(TargetAOM14.ITSXMLSchemaSets); err != nil {
		t.Fatalf("orderFor(aom14): %v", err)
	}
	return plan
}

// TestREQ056_CheckXMLTypeNames pins the schema names the generator accepts
// for a target: the AOM 1.4 target's own pass, and each name the codecs
// could not honour is refused (REQ-056).
func TestREQ056_CheckXMLTypeNames(t *testing.T) {
	if err := checkXMLTypeNames(aom14PlanWithSchemas(t)); err != nil {
		t.Fatalf("checkXMLTypeNames(aom14) = %v; want nil", err)
	}
	for _, tc := range []struct {
		name  string
		names map[string]string
		want  string
	}{
		{"unknown class", map[string]string{"C_NO_SUCH_CLASS": "C_CODE_PHRASE"}, "no concrete class"},
		{"abstract class", map[string]string{"C_OBJECT": "C_CODE_PHRASE"}, "no concrete class"},
		{"class of another target", map[string]string{"CODE_PHRASE": "C_CODE_PHRASE"}, "no concrete class"},
		{"name of a declared class", map[string]string{"C_CODED_TEXT": "C_ORDINAL"}, "name of a declared class"},
		{"one name for two classes", map[string]string{"C_CODED_TEXT": "C_DV_SCALE", "C_ORDINAL": "C_DV_SCALE"}, "given to both"},
		{"name the schemas lack", map[string]string{"C_CODED_TEXT": "C_NO_SUCH_SCHEMA_TYPE"}, "not declared by the target's ITS-XML schemas"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			plan := aom14PlanWithSchemas(t)
			plan.Target.XMLTypeNames = tc.names
			err := checkXMLTypeNames(plan)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("checkXMLTypeNames(%v) = %v; want an error containing %q", tc.names, err, tc.want)
			}
		})
	}
}

// TestREQ056_XMLSiteStart pins which polymorphic decode sites hand canxml the
// start with schema names rewritten: a site that can hold a class with a
// schema name, and no other, so the rest of the generated code is unchanged
// (REQ-056).
func TestREQ056_XMLSiteStart(t *testing.T) {
	aom := aom14PlanWithSchemas(t)
	rmPlan, err := BuildPlanForTarget(context.Background(), TargetRM, bmm.FSResolver{Root: testResources})
	if err != nil {
		t.Fatalf("BuildPlanForTarget(rm): %v", err)
	}
	for _, tc := range []struct {
		plan  *Plan
		iface string
		want  string
	}{
		{aom, "CObject", "xmlBMMTyped(_t)"},
		{aom, "CDomainType", "xmlBMMTyped(_t)"},
		{aom, "CPrimitive", "_t"},
		{aom, "ExprItem", "_t"},
		{rmPlan, "DataValue", "_t"},
	} {
		if got := xmlSiteStart(tc.plan, tc.iface); got != tc.want {
			t.Errorf("xmlSiteStart(%s, %s) = %q; want %q", tc.plan.Target.GoPackage, tc.iface, got, tc.want)
		}
	}
}

// TestRenderXMLNamesFileOnlyForNamedTargets pins that the RM target, whose
// schemas use the BMM names, gets no xmlnames_gen.go, and the AOM 1.4 target
// gets one mapping each schema name to its BMM name.
func TestRenderXMLNamesFileOnlyForNamedTargets(t *testing.T) {
	rmPlan, err := BuildPlanForTarget(context.Background(), TargetRM, bmm.FSResolver{Root: testResources})
	if err != nil {
		t.Fatalf("BuildPlanForTarget(rm): %v", err)
	}
	if body, err := RenderXMLNamesFile(rmPlan); err != nil || body != nil {
		t.Errorf("RenderXMLNamesFile(rm) = %d bytes, %v; want nil, nil", len(body), err)
	}
	body, err := RenderXMLNamesFile(aom14PlanWithSchemas(t))
	if err != nil {
		t.Fatalf("RenderXMLNamesFile(aom14): %v", err)
	}
	for _, want := range []string{`"C_CODE_PHRASE": "C_CODED_TEXT"`, `"C_DV_ORDINAL":  "C_ORDINAL"`, `"C_DV_QUANTITY": "C_QUANTITY"`} {
		if !strings.Contains(string(body), want) {
			t.Errorf("RenderXMLNamesFile(aom14) lacks %s:\n%s", want, body)
		}
	}
}
