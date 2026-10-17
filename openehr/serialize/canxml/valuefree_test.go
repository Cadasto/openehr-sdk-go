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

// slotElement is an ELEMENT whose value slot holds body as the RM type
// xsiType, so a failure inside the slot comes back in a DecodeError.
func slotElement(xsiType, body string) string {
	return `<element xmlns="http://schemas.openehr.org/v1" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" archetype_node_id="at0001"><name><value>e</value></name>` +
		`<value xsi:type="` + xsiType + `">` + body + `</value></element>`
}

// TestUnmarshal_REQ052_SlotRefusalsKeepTheirTextOnOneLine pins the text of a
// DecodeError over a canonical-XML refusal the SDK writes around a cause: an
// interval side written bounded with no bound, which names the RM type
// between canxml's shape sentinel and the refusal. The text keeps each part,
// stays on one line (REQ-052) and carries no value from the document, and
// Value stays empty: canonical XML holds no literal for it.
//
// Can-fail control: write an error that wraps several causes as errors.Join
// does, a line break between the causes and none of its own text, and the
// interval rows lose "DV_INTERVAL" and break the line.
func TestUnmarshal_REQ052_SlotRefusalsKeepTheirTextOnOneLine(t *testing.T) {
	cases := []struct {
		name   string
		in     string
		keep   []string
		absent []string
	}{
		{
			name: "an interval side written bounded with no bound",
			in: slotElement("DV_QUANTITY", `<normal_range><upper xsi:type="DV_QUANTITY"><magnitude>7777</magnitude><units>mm</units></upper>`+
				`<lower_unbounded>false</lower_unbounded></normal_range><magnitude>3</magnitude><units>mm</units>`),
			keep: []string{
				`decode /value (_type="DV_QUANTITY"): `,
				"canxml: invalid XML shape: DV_INTERVAL: lower_unbounded is false but the interval gives no lower bound",
			},
			absent: []string{"7777"},
		},
	}
	for _, tc := range cases {
		for mode, opts := range xmlDecodeModes() {
			t.Run(tc.name+"/"+mode, func(t *testing.T) {
				err := canxml.Unmarshal([]byte(tc.in), new(rm.Element), opts...)
				de, ok := errors.AsType[*canxml.DecodeError](err)
				if !ok || de == nil {
					t.Fatalf("Unmarshal(%s) = %T %v; want a *canxml.DecodeError", tc.in, err, err)
				}
				msg := de.Error()
				for _, want := range tc.keep {
					if !strings.Contains(msg, want) {
						t.Errorf("DecodeError.Error() = %q; want it to keep %q", msg, want)
					}
				}
				if strings.ContainsAny(msg, "\n\r") {
					t.Errorf("DecodeError.Error() = %q; want the text on one line", msg)
				}
				for _, v := range tc.absent {
					if strings.Contains(msg, v) {
						t.Errorf("DecodeError.Error() = %q; the text carries %q from the document", msg, v)
					}
				}
				if got := de.Value.Reveal(); got != nil {
					t.Errorf("DecodeError.Value.Reveal() = %#v; want nothing: canonical XML holds no literal here", got)
				}
			})
		}
	}
}
