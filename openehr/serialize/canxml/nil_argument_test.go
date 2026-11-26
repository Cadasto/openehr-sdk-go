package canxml_test

import (
	"encoding/xml"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canxml"
)

// TestREQ056_NilEncoderOrDecoderIsRefused pins the no-panic rule (REQ-056,
// idiom.md § No panics) on the helpers the generated XML codecs call: a nil
// *xml.Encoder handed to EncodeString, EncodeText or EncodePoly, and a nil
// *xml.Decoder handed to DecodeAs or DecodeAsOrDefault, is refused with an
// error wrapping typereg.ErrNilArgument that names the helper and the element,
// a decode helper's inside a DecodeError on the element. EncodePoly refuses a
// nil encoder for an absent value too, and DecodeAs for an element with no
// discriminator. Can-fail: removing a helper's check panics on the nil
// encoder or decoder in its rows, or, in the absent-value and
// no-discriminator rows, returns nil or typereg.ErrMissingType.
func TestREQ056_NilEncoderOrDecoderIsRefused(t *testing.T) {
	t.Parallel()
	typed := func(class string) xml.StartElement {
		return xml.StartElement{Name: xml.Name{Local: "value"}, Attr: []xml.Attr{{Name: xml.Name{Space: canxml.NSXSI, Local: "type"}, Value: class}}}
	}
	bare := xml.StartElement{Name: xml.Name{Local: "value"}, Attr: []xml.Attr{{Name: xml.Name{Local: "type"}, Value: "DV_TEXT"}}}
	untyped := xml.StartElement{Name: xml.Name{Local: "value"}}
	newText := func() any { return new(rm.DVText) }
	for _, tc := range []struct {
		name   string
		helper string
		decode bool
		call   func() error
	}{
		{name: "EncodeString", helper: "EncodeString", call: func() error { return canxml.EncodeString(nil, "value", "x") }},
		{name: "EncodeText", helper: "EncodeText", call: func() error { return canxml.EncodeText(nil, "value", rm.Real(1)) }},
		{name: "EncodePoly", helper: "EncodePoly", call: func() error { return canxml.EncodePoly(nil, "value", &rm.DVText{Value: "x"}) }},
		{name: "EncodePoly of an absent value", helper: "EncodePoly", call: func() error { return canxml.EncodePoly(nil, "value", nil) }},
		{name: "DecodeAs, xsi:type", helper: "DecodeAs", decode: true, call: func() error {
			_, err := canxml.DecodeAs[rm.DataValue](nil, typed("DV_TEXT"))
			return err
		}},
		{name: "DecodeAs, bare type", helper: "DecodeAs", decode: true, call: func() error {
			_, err := canxml.DecodeAs[rm.DataValue](nil, bare)
			return err
		}},
		{name: "DecodeAs, no discriminator", helper: "DecodeAs", decode: true, call: func() error {
			_, err := canxml.DecodeAs[rm.DataValue](nil, untyped)
			return err
		}},
		{name: "DecodeAsOrDefault, xsi:type", helper: "DecodeAsOrDefault", decode: true, call: func() error {
			_, err := canxml.DecodeAsOrDefault[rm.DVTextLike](nil, typed("DV_TEXT"), newText)
			return err
		}},
		{name: "DecodeAsOrDefault, no discriminator", helper: "DecodeAsOrDefault", decode: true, call: func() error {
			_, err := canxml.DecodeAsOrDefault[rm.DVTextLike](nil, untyped, newText)
			return err
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := callRecovering(tc.call)
			if !errors.Is(err, typereg.ErrNilArgument) {
				t.Fatalf("%s with a nil argument = %v; want an error wrapping typereg.ErrNilArgument", tc.name, err)
			}
			if msg := err.Error(); !strings.Contains(msg, "<value>") || !strings.HasSuffix(msg, " for canxml."+tc.helper) {
				t.Errorf("%s error = %q; want it to name canxml.%s and the element", tc.name, msg, tc.helper)
			}
			de, ok := errors.AsType[*canxml.DecodeError](err)
			if tc.decode && (!ok || de == nil || de.Path != "/value") {
				t.Errorf("%s error = %v; want a DecodeError on /value", tc.name, err)
			}
			if !tc.decode && ok {
				t.Errorf("%s error = %v; an encode refusal is no DecodeError", tc.name, err)
			}
		})
	}
}

// callRecovering runs call and returns its error, or a panic it raises as an
// error that wraps nothing.
func callRecovering(call func() error) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panicked: %v", r)
		}
	}()
	return call()
}
