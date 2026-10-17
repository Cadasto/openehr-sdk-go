package canxml_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml"
)

// TestUnmarshal_REQ052_XMITypeRefusalNamesItsClass pins the text of a
// DecodeError that carries canxml's own shape refusal of an `xmi:type`
// discriminator: the value-free rule binds a DecodeError wherever it is
// raised, canonical XML's included (REQ-052), and the refusal's text is
// fixed, so it names its class and why, while no value from the document
// reaches it.
//
// Can-fail control: drop the canxml shape refusal from the forms the
// renderer keeps and the text reads "cause withheld" in place of the class.
func TestUnmarshal_REQ052_XMITypeRefusalNamesItsClass(t *testing.T) {
	const head = `<element xmlns="http://schemas.openehr.org/v1" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xmlns:xmi="http://www.omg.org/XMI" archetype_node_id="at0001"><name><value>e</value></name>`
	in := head + `<value xmi:type="DV_TEXT"><value>Jane Doe 7777</value></value></element>`
	for mode, opts := range map[string][]canxml.DecoderOption{
		"default":    nil,
		"WithStrict": {canxml.WithStrict()},
	} {
		t.Run(mode, func(t *testing.T) {
			err := canxml.Unmarshal([]byte(in), new(rm.Element), opts...)
			de, ok := errors.AsType[*canxml.DecodeError](err)
			if !ok || de == nil {
				t.Fatalf("Unmarshal(%s) = %T %v; want a *canxml.DecodeError", in, err, err)
			}
			if !errors.Is(err, canxml.ErrInvalidShape) {
				t.Errorf("Unmarshal(%s) = %v; want errors.Is(_, canxml.ErrInvalidShape)", in, err)
			}
			msg := de.Error()
			for _, want := range []string{
				"decode /value: ",
				"canxml: invalid XML shape: xmi:type on the wire is rejected (use xsi:type)",
			} {
				if !strings.Contains(msg, want) {
					t.Errorf("DecodeError.Error() = %q; want it to keep %q", msg, want)
				}
			}
			if strings.Contains(msg, "cause withheld") {
				t.Errorf("DecodeError.Error() = %q; want the refusal named by its class, not withheld", msg)
			}
			for _, v := range []string{"Jane", "Doe", "7777"} {
				if strings.Contains(msg, v) {
					t.Errorf("DecodeError.Error() = %q; the text carries %q from the document", msg, v)
				}
			}
		})
	}
}
