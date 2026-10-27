package bmmgen

import (
	"context"
	"io/fs"
	"path/filepath"
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

// TestREQ056_XMLSite pins which polymorphic decode sites hand canxml the
// start with schema names rewritten, and return a failure with the type
// named as the document wrote it: a site that can hold a class with a
// schema name, and no other, so the rest of the generated code is unchanged
// (REQ-056).
func TestREQ056_XMLSite(t *testing.T) {
	aom := aom14PlanWithSchemas(t)
	rmPlan, err := BuildPlanForTarget(context.Background(), TargetRM, bmm.FSResolver{Root: testResources})
	if err != nil {
		t.Fatalf("BuildPlanForTarget(rm): %v", err)
	}
	renamed := xmlDispatchSite{start: "xmlBMMTyped(_t)", fail: "xmlWrittenType(_err, _t)"}
	plain := xmlDispatchSite{start: "_t", fail: "_err"}
	for _, tc := range []struct {
		plan  *Plan
		iface string
		want  xmlDispatchSite
	}{
		{aom, "CObject", renamed},
		{aom, "CDomainType", renamed},
		{aom, "CPrimitive", plain},
		{aom, "ExprItem", plain},
		{rmPlan, "DataValue", plain},
	} {
		if got := xmlSite(tc.plan, tc.iface); got != tc.want {
			t.Errorf("xmlSite(%s, %s) = %+v; want %+v", tc.plan.Target.GoPackage, tc.iface, got, tc.want)
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

// TestREQ056_CheckXMLIdentifierElements pins the identifier elements the
// generator accepts for a target: the AOM 1.4 target's own pass, and each
// one the codecs could not honour is refused (REQ-056).
func TestREQ056_CheckXMLIdentifierElements(t *testing.T) {
	if err := checkXMLIdentifierElements(aom14PlanWithSchemas(t)); err != nil {
		t.Fatalf("checkXMLIdentifierElements(aom14) = %v; want nil", err)
	}
	const (
		noMember = "no String member a concrete class of the aom14 target declares"
		noClass  = "no class with a canonical-XML decoder and a String value"
		noSchema = "not declared by the ITS-XML schema type of C_CODED_TEXT"
	)
	tid := XMLIdentifierElement{Name: "terminology_id", Type: "TERMINOLOGY_ID"}
	for _, tc := range []struct {
		name  string
		elems map[string]XMLIdentifierElement
		want  string
	}{
		{"unknown class", map[string]XMLIdentifierElement{"C_NO_SUCH_CLASS.terminology": tid}, noMember},
		{"abstract class", map[string]XMLIdentifierElement{"C_OBJECT.node_id": tid}, noMember},
		{"member an ancestor declares", map[string]XMLIdentifierElement{"C_CODED_TEXT.node_id": tid}, noMember},
		{"member that is no String", map[string]XMLIdentifierElement{"C_CODED_TEXT.code_list": tid}, noMember},
		{"element class without a value", map[string]XMLIdentifierElement{"C_CODED_TEXT.terminology": {Name: "terminology_id", Type: "CODE_PHRASE"}}, noClass},
		{"element class the plan lacks", map[string]XMLIdentifierElement{"C_CODED_TEXT.terminology": {Name: "terminology_id", Type: "NO_SUCH_ID"}}, noClass},
		{"element the schema type lacks", map[string]XMLIdentifierElement{"C_CODED_TEXT.terminology": {Name: "terminology", Type: "TERMINOLOGY_ID"}}, noSchema},
	} {
		t.Run(tc.name, func(t *testing.T) {
			plan := aom14PlanWithSchemas(t)
			plan.Target.XMLIdentifierElements = tc.elems
			err := checkXMLIdentifierElements(plan)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("checkXMLIdentifierElements(%v) = %v; want an error containing %q", tc.elems, err, tc.want)
			}
		})
	}
}

// TestREQ056_RunChecksXMLNameTablesBeforeWriting pins that a run refuses a
// target whose ITS-XML name tables the codecs could not honour before it
// writes any file, so a refused run leaves the output tree as it was
// (REQ-056).
func TestREQ056_RunChecksXMLNameTablesBeforeWriting(t *testing.T) {
	for _, tc := range []struct {
		name   string
		target func() Target
		want   string
	}{
		{
			name: "identifier element the schema type lacks",
			target: func() Target {
				tg := TargetAOM14
				tg.XMLIdentifierElements = map[string]XMLIdentifierElement{"C_CODED_TEXT.terminology": {Name: "terminology", Type: "TERMINOLOGY_ID"}}
				return tg
			},
			want: "not declared by the ITS-XML schema type",
		},
		{
			name: "type name the schemas lack",
			target: func() Target {
				tg := TargetAOM14
				tg.XMLTypeNames = map[string]string{"C_CODED_TEXT": "C_CODE_PHRASE", "C_ORDINAL": "C_NO_SUCH_SCHEMA_TYPE"}
				return tg
			},
			want: "not declared by the target's ITS-XML schemas",
		},
		{
			// The identifier element's class takes its schema type from
			// the type-name table, so the table is checked first.
			name: "type name of the class an identifier element belongs to",
			target: func() Target {
				tg := TargetAOM14
				tg.XMLTypeNames = map[string]string{"C_CODED_TEXT": "C_NO_SUCH_SCHEMA_TYPE"}
				return tg
			},
			want: "XML type name C_NO_SUCH_SCHEMA_TYPE of C_CODED_TEXT is not declared by the target's ITS-XML schemas",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := t.TempDir()
			_, err := Run(Options{
				ResourcesDir:  testResources,
				ITSXMLDir:     testITSXML,
				OutDir:        out,
				Targets:       []Target{tc.target()},
				SkipInventory: true,
			})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Run = %v; want an error containing %q", err, tc.want)
			}
			var written []string
			if err := filepath.WalkDir(out, func(path string, d fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if !d.IsDir() {
					written = append(written, path)
				}
				return nil
			}); err != nil {
				t.Fatalf("walking %s: %v", out, err)
			}
			if len(written) > 0 {
				t.Errorf("a refused Run wrote %d files, among them %s; want none", len(written), written[0])
			}
		})
	}
}
