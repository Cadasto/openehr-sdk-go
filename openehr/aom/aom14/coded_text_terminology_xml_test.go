package aom14_test

import (
	"encoding/xml"
	"fmt"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aom/aom14"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml"
)

// codePhraseChild returns a C_SINGLE_ATTRIBUTE document whose one child is
// written `xsi:type="C_CODE_PHRASE"` and carries terminology, the element
// that names the child's terminology.
func codePhraseChild(terminology string) string {
	return `<c_single_attribute xmlns="http://schemas.openehr.org/v1" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">` +
		`<rm_attribute_name>defining_code</rm_attribute_name>` +
		`<children xsi:type="C_CODE_PHRASE"><rm_type_name>CODE_PHRASE</rm_type_name><node_id/>` + terminology + `</children>` +
		`</c_single_attribute>`
}

// TestREQ056_CodedTextTerminologyDecodesBothShapes pins that canonical XML
// reads C_CODED_TEXT.terminology from the `terminology_id` element the AM 1.4
// schema type C_CODE_PHRASE declares, a TERMINOLOGY_ID whose `value` child
// holds the string, and from the `terminology` element an older SDK wrote,
// in every decode mode (REQ-056).
func TestREQ056_CodedTextTerminologyDecodesBothShapes(t *testing.T) {
	shapes := []struct {
		name, element string
	}{
		{"schema terminology_id", `<terminology_id><value>local</value></terminology_id>`},
		{"older terminology", `<terminology>local</terminology>`},
	}
	for _, shape := range shapes {
		for _, strict := range []bool{false, true} {
			var opts []canxml.DecoderOption
			if strict {
				opts = append(opts, canxml.WithStrict())
			}
			t.Run(fmt.Sprintf("%s/strict=%v", shape.name, strict), func(t *testing.T) {
				doc := codePhraseChild(shape.element)
				var got aom14.CSingleAttribute
				if err := canxml.Unmarshal([]byte(doc), &got, opts...); err != nil {
					t.Fatalf("canxml.Unmarshal(%s) (strict %v) = %v; want nil", doc, strict, err)
				}
				if len(got.Children) != 1 {
					t.Fatalf("canxml.Unmarshal(%s): Children = %#v; want one", doc, got.Children)
				}
				c, ok := got.Children[0].(*aom14.CCodedText)
				if !ok {
					t.Fatalf("canxml.Unmarshal(%s): child is %T; want *aom14.CCodedText", doc, got.Children[0])
				}
				if c.Terminology != "local" {
					t.Errorf("canxml.Unmarshal(%s) (strict %v): Terminology = %q; want %q", doc, strict, c.Terminology, "local")
				}
			})
		}
	}
}

// TestREQ175_CodedTextTerminologyIDIsNotMissing pins that the canonical XML
// the SDK writes for a C_CODED_TEXT, whose terminology is the AM 1.4
// `terminology_id` element, reports no missing member when decoded with
// canxml.WithMissing (REQ-175, REQ-056).
func TestREQ175_CodedTextTerminologyIDIsNotMissing(t *testing.T) {
	in := &aom14.CSingleAttribute{
		RMAttributeName: "defining_code",
		Existence:       rm.NewClosedInterval[aom14.Integer](1, 1),
		Children: []aom14.CObject{&aom14.CCodedText{
			RMTypeName:  "CODE_PHRASE",
			Occurrences: rm.NewClosedInterval[aom14.Integer](1, 1),
			NodeID:      "at0001",
			Terminology: "local",
		}},
	}
	b, err := canxml.Marshal(in)
	if err != nil {
		t.Fatalf("canxml.Marshal: %v", err)
	}
	var missing []string
	var got aom14.CSingleAttribute
	if err := canxml.Unmarshal(b, &got, canxml.WithMissing(func(p string) { missing = append(missing, p) })); err != nil {
		t.Fatalf("canxml.Unmarshal(%s) = %v; want nil", b, err)
	}
	if len(missing) != 0 {
		t.Errorf("canxml.Unmarshal(%s) with WithMissing reported %q; want nothing", b, missing)
	}
}

// TestREQ056_CodedTextTerminologyEncodesSchemaElement pins that canonical XML
// writes C_CODED_TEXT.terminology as the `terminology_id` element of the AM
// 1.4 schema type C_CODE_PHRASE, the string in its `value` child, and writes
// no `terminology` element (REQ-056).
func TestREQ056_CodedTextTerminologyEncodesSchemaElement(t *testing.T) {
	in := &aom14.CSingleAttribute{
		RMAttributeName: "defining_code",
		Children:        []aom14.CObject{&aom14.CCodedText{RMTypeName: "CODE_PHRASE", Terminology: "local"}},
	}
	b, err := canxml.Marshal(in)
	if err != nil {
		t.Fatalf("canxml.Marshal: %v", err)
	}
	var doc struct {
		Children struct {
			Terminology   *string `xml:"terminology"`
			TerminologyID *struct {
				Value string `xml:"value"`
			} `xml:"terminology_id"`
		} `xml:"children"`
	}
	if err := xml.Unmarshal(b, &doc); err != nil {
		t.Fatalf("reading %s: %v", b, err)
	}
	if doc.Children.Terminology != nil {
		t.Errorf("canxml.Marshal wrote a terminology element in %s; want terminology_id only", b)
	}
	if id := doc.Children.TerminologyID; id == nil || id.Value != "local" {
		t.Errorf("canxml.Marshal wrote terminology_id %+v in %s; want one whose value is %q", id, b, "local")
	}
}
