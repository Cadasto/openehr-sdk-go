package aom14_test

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aom/aom14"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml"
)

// schemaTypeNameCase is one AOM 1.4 constraint class whose AM 1.4 ITS-XML
// schema name differs from its BMM name.
type schemaTypeNameCase struct {
	bmm, schema string
	// value is an instance of the class with its rm_type_name set.
	value aom14.CObject
	// is reports whether a decoded child is of the class.
	is func(aom14.CObject) bool
}

func schemaTypeNameCases() []schemaTypeNameCase {
	return []schemaTypeNameCase{
		{
			bmm: "C_CODED_TEXT", schema: "C_CODE_PHRASE",
			value: &aom14.CCodedText{RMTypeName: "CODE_PHRASE", NodeID: "at0001"},
			is:    func(c aom14.CObject) bool { _, ok := c.(*aom14.CCodedText); return ok },
		},
		{
			bmm: "C_ORDINAL", schema: "C_DV_ORDINAL",
			value: &aom14.COrdinal{RMTypeName: "DV_ORDINAL", NodeID: "at0001"},
			is:    func(c aom14.CObject) bool { _, ok := c.(*aom14.COrdinal); return ok },
		},
		{
			bmm: "C_QUANTITY", schema: "C_DV_QUANTITY",
			value: &aom14.CQuantity{RMTypeName: "DV_QUANTITY", NodeID: "at0001"},
			is:    func(c aom14.CObject) bool { _, ok := c.(*aom14.CQuantity); return ok },
		},
	}
}

// TestREQ056_AOM14SchemaTypeNamesDecode pins that a C_ATTRIBUTE child whose
// `xsi:type` spells one of the three classes by its AM 1.4 ITS-XML schema
// name, or by its BMM name, decodes to that class, in every decode mode.
func TestREQ056_AOM14SchemaTypeNamesDecode(t *testing.T) {
	for _, tc := range schemaTypeNameCases() {
		for _, name := range []string{tc.schema, tc.bmm} {
			for _, strict := range []bool{false, true} {
				doc := `<c_single_attribute xmlns="http://schemas.openehr.org/v1" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">` +
					`<rm_attribute_name>value</rm_attribute_name>` +
					`<children xsi:type="` + name + `"><rm_type_name>X</rm_type_name><node_id>at0001</node_id></children>` +
					`</c_single_attribute>`
				var opts []canxml.DecoderOption
				if strict {
					opts = append(opts, canxml.WithStrict())
				}
				t.Run(fmt.Sprintf("%s/strict=%v", name, strict), func(t *testing.T) {
					var got aom14.CSingleAttribute
					if err := canxml.Unmarshal([]byte(doc), &got, opts...); err != nil {
						t.Fatalf("canxml.Unmarshal (strict %v) of a child typed %s = %v; want nil", strict, name, err)
					}
					if len(got.Children) != 1 || !tc.is(got.Children[0]) {
						t.Fatalf("canxml.Unmarshal (strict %v) of a child typed %s: Children = %#v; want one %s", strict, name, got.Children, tc.bmm)
					}
				})
			}
		}
	}
}

// TestREQ056_AOM14SchemaTypeNamesEncode pins that canonical XML writes the
// AM 1.4 ITS-XML schema name as the `xsi:type` of each of the three
// classes, and that the output decodes back to the class.
func TestREQ056_AOM14SchemaTypeNamesEncode(t *testing.T) {
	for _, tc := range schemaTypeNameCases() {
		t.Run(tc.bmm, func(t *testing.T) {
			in := &aom14.CSingleAttribute{RMAttributeName: "value", Children: []aom14.CObject{tc.value}}
			b, err := canxml.Marshal(in)
			if err != nil {
				t.Fatalf("canxml.Marshal: %v", err)
			}
			types := childTypes(t, b)
			if !slices.Equal(types, []string{tc.schema}) {
				t.Errorf("canxml.Marshal wrote children xsi:type %q in %s; want [%s]", types, b, tc.schema)
			}
			var back aom14.CSingleAttribute
			if err := canxml.Unmarshal(b, &back); err != nil {
				t.Fatalf("canxml.Unmarshal of %s: %v", b, err)
			}
			if len(back.Children) != 1 || !tc.is(back.Children[0]) {
				t.Errorf("canxml.Unmarshal of %s: Children = %#v; want one %s", b, back.Children, tc.bmm)
			}
		})
	}
}

// childTypes returns the `xsi:type` of every `children` element of the
// document in b, in document order.
func childTypes(t *testing.T, b []byte) []string {
	t.Helper()
	const nsXSI = "http://www.w3.org/2001/XMLSchema-instance"
	var out []string
	dec := xml.NewDecoder(bytes.NewReader(b))
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			return out
		}
		if err != nil {
			t.Fatalf("reading %s: %v", b, err)
		}
		start, ok := tok.(xml.StartElement)
		if !ok || start.Name.Local != "children" {
			continue
		}
		for _, a := range start.Attr {
			if a.Name.Space == nsXSI && a.Name.Local == "type" {
				out = append(out, a.Value)
			}
		}
	}
}
