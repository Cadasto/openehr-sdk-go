package bmmgen

import (
	"context"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/bmm"
)

// TestTargetRM_REQ041_CarriesTheNamesItsRootDeclares — REQ-041 § SDK-authored
// root schemas, Observable: the RM target loads its root, which corrects
// TRANSLATION_DETAILS.accreditation, so the generated type carries that name
// in Go, in canonical JSON and in canonical XML, and the RM schema's spelling
// `accreditaton` reaches no generated file.
func TestTargetRM_REQ041_CarriesTheNamesItsRootDeclares(t *testing.T) {
	t.Parallel()
	plan, err := BuildPlanForTarget(context.Background(), TargetRM, testResolver())
	if err != nil {
		t.Fatalf("BuildPlanForTarget(RM): %v", err)
	}
	cls, ok := plan.Schema.ClassDefinitions["TRANSLATION_DETAILS"].(*bmm.SimpleClass)
	if !ok {
		t.Fatalf("the RM target has no simple class TRANSLATION_DETAILS")
	}
	if _, ok := cls.Properties["accreditation"]; !ok {
		t.Errorf("TRANSLATION_DETAILS properties = %v, want accreditation among them", cls.PropertyOrder)
	}

	var file *PlannedFile
	for _, f := range plan.Files {
		if f.FileBase == "common_resource" {
			file = f
		}
	}
	if file == nil {
		t.Fatal("the RM target plans no common_resource file")
	}
	var out strings.Builder
	for _, render := range []func() ([]byte, error){
		func() ([]byte, error) { return RenderFile(plan, file) },
		func() ([]byte, error) { return RenderMarshalXMLFile(plan, file) },
		func() ([]byte, error) { return RenderUnmarshalXMLFile(plan, file) },
		func() ([]byte, error) { return RenderRMInfoFile(plan) },
		func() ([]byte, error) { return RenderRMAccessFile(plan) },
	} {
		body, err := render()
		if err != nil {
			t.Fatalf("render: %v", err)
		}
		out.Write(body)
	}
	src := out.String()
	for _, want := range []string{
		"Accreditation *string `json:\"accreditation,omitzero\"`",
		`EncodeElement(*t.Accreditation, xml.StartElement{Name: xml.Name{Local: "accreditation"}})`,
		`case "accreditation":`,
	} {
		if !strings.Contains(src, want) {
			t.Errorf("the generated RM files lack %q", want)
		}
	}
	if strings.Contains(src, "ccreditaton") {
		t.Error("the generated RM files still carry the RM schema's spelling accreditaton")
	}
}
