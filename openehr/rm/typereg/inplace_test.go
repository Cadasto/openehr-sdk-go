package typereg

import (
	jsonv1 "encoding/json"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"strings"
	"testing"
)

// TestDecodeSlotInPlaceGate pins when a polymorphic slot is decoded where it
// stands instead of being read whole first (REQ-052): only on a decoder that
// carries the SDK hooks under their buffered identity, with v2 error
// semantics, at an object whose leading `_type` the registry knows. Each
// refused row is the case one condition exists for: drop the identity check
// and the aggregate row comes back handled; drop the legacy check and the
// legacy row does; drop the registry lookup and the unknown-type row does. The
// leading-type read refuses the no-hooks, caller, null and not-first rows
// itself, and its own rows are in leadingtype_test.go. The handled
// rows put the slot after a name, as the first element of an array and after
// a comma, so a lead that did not step over the separator would refuse them.
//
// A refused slot leaves the decoder where it was, so the buffered read that
// follows gets the whole slot: every refused row reads it back.
func TestDecodeSlotInPlaceGate(t *testing.T) {
	set := unmarshalerAgg.Load()
	if set == nil {
		t.Fatal("unmarshalerAgg.Load() = nil: this test needs the rm package's interface hooks, which the typereg_test files import")
	}
	caller := json.JoinUnmarshalers(set.all, json.UnmarshalFromFunc(func(dec *jsontext.Decoder, _ *fakeBox) error { return dec.SkipValue() }))
	const slot = `{"_type":"DV_TEXT","value":"x"}`
	cases := []struct {
		name    string
		doc     string
		lead    int // tokens to read before the slot
		opts    []json.Options
		handled bool
	}{
		{name: "buffered identity, after a name", doc: `{"value":` + slot + `}`, lead: 2, opts: []json.Options{set.bufferedOpt}, handled: true},
		{name: "buffered identity, first array element", doc: `{"items":[` + slot + `]}`, lead: 3, opts: []json.Options{set.bufferedOpt}, handled: true},
		{name: "buffered identity, after a comma", doc: `{"items":[` + slot + ` , ` + slot + `]}`, lead: 3, opts: []json.Options{set.bufferedOpt}, handled: true},
		{name: "the aggregate itself", doc: `{"value":` + slot + `}`, lead: 2, opts: []json.Options{json.WithUnmarshalers(set.all)}},
		{name: "no hooks", doc: `{"value":` + slot + `}`, lead: 2},
		{name: "a caller's hooks", doc: `{"value":` + slot + `}`, lead: 2, opts: []json.Options{json.WithUnmarshalers(caller)}},
		{name: "legacy error semantics", doc: `{"value":` + slot + `}`, lead: 2, opts: []json.Options{set.bufferedOpt, jsonv1.ReportErrorsWithLegacySemantics(true)}},
		{name: "null", doc: `{"value":null}`, lead: 2, opts: []json.Options{set.bufferedOpt}},
		{name: "_type not first", doc: `{"value":{"value":"x","_type":"DV_TEXT"}}`, lead: 2, opts: []json.Options{set.bufferedOpt}},
		{name: "unknown _type", doc: `{"value":{"_type":"NO_SUCH_TYPE","value":"x"}}`, lead: 2, opts: []json.Options{set.bufferedOpt}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dec := jsontext.NewDecoder(strings.NewReader(tc.doc), tc.opts...)
			for range tc.lead {
				if _, err := dec.ReadToken(); err != nil {
					t.Fatalf("ReadToken before the slot of %s: %v", tc.doc, err)
				}
			}
			if tc.name == "buffered identity, after a comma" {
				if _, err := dec.ReadValue(); err != nil {
					t.Fatalf("ReadValue of the first element of %s: %v", tc.doc, err)
				}
			}
			before := dec.InputOffset()
			var out any
			handled, err := decodeSlotInPlace(dec, &out)
			if handled != tc.handled || err != nil {
				t.Fatalf("decodeSlotInPlace at %s = (%v, %v), want (%v, nil)", tc.doc, handled, err, tc.handled)
			}
			if tc.handled {
				if out == nil {
					t.Errorf("decodeSlotInPlace at %s handled the slot but left out nil", tc.doc)
				}
				return
			}
			if got := dec.InputOffset(); got != before {
				t.Errorf("decodeSlotInPlace at %s refused the slot but moved the decoder from offset %d to %d", tc.doc, before, got)
			}
			if raw, err := dec.ReadValue(); err != nil || !strings.HasPrefix(tc.doc[before:], ":"+string(raw)) {
				t.Errorf("ReadValue after the refusal at %s = (%s, %v), want the whole slot", tc.doc, raw, err)
			}
		})
	}
}
