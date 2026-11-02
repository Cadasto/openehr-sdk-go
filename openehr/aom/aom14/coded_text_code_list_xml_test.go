package aom14_test

import (
	"encoding/xml"
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aom/aom14"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml"
)

// TestREQ056_CodedTextCodeListRoundTrips pins that canonical XML writes
// C_CODED_TEXT.code_list as one `code_list` element per code, the repeated
// xs:string the AM 1.4 schema type C_CODE_PHRASE declares, and reads every
// one of them back, in every decode mode (REQ-056).
func TestREQ056_CodedTextCodeListRoundTrips(t *testing.T) {
	want := []string{"at0001", "at0002"}
	in := &aom14.CSingleAttribute{
		RMAttributeName: "defining_code",
		Children: []aom14.CObject{&aom14.CCodedText{
			RMTypeName:  "CODE_PHRASE",
			Terminology: "local",
			CodeList:    new(slices.Clone(want)),
		}},
	}
	b, err := canxml.Marshal(in)
	if err != nil {
		t.Fatalf("canxml.Marshal: %v", err)
	}
	var doc struct {
		Children struct {
			CodeList []string `xml:"code_list"`
		} `xml:"children"`
	}
	if err := xml.Unmarshal(b, &doc); err != nil {
		t.Fatalf("reading %s: %v", b, err)
	}
	if !slices.Equal(doc.Children.CodeList, want) {
		t.Errorf("canxml.Marshal wrote code_list elements %q in %s; want one per code, %q", doc.Children.CodeList, b, want)
	}
	for _, strict := range []bool{false, true} {
		var opts []canxml.DecoderOption
		if strict {
			opts = append(opts, canxml.WithStrict())
		}
		var back aom14.CSingleAttribute
		if err := canxml.Unmarshal(b, &back, opts...); err != nil {
			t.Fatalf("canxml.Unmarshal(%s) (strict %v) = %v; want nil", b, strict, err)
		}
		if len(back.Children) != 1 {
			t.Fatalf("canxml.Unmarshal(%s): Children = %#v; want one", b, back.Children)
		}
		c, ok := back.Children[0].(*aom14.CCodedText)
		if !ok {
			t.Fatalf("canxml.Unmarshal(%s): child is %T; want *aom14.CCodedText", b, back.Children[0])
		}
		if c.CodeList == nil {
			t.Errorf("canxml.Unmarshal(%s) (strict %v): CodeList = nil; want %q", b, strict, want)
		} else if !slices.Equal(*c.CodeList, want) {
			t.Errorf("canxml.Unmarshal(%s) (strict %v): CodeList = %q; want %q", b, strict, *c.CodeList, want)
		}
	}
}
