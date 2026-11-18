package typereg

import (
	"bytes"
	jsonv1 "encoding/json"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// TestDecodeSlotInPlaceGate pins when a polymorphic slot is decoded where it
// stands instead of being read whole first (REQ-052): only on a decoder that
// carries the SDK hooks under their buffered identity, with v2 error
// semantics, at an object whose leading `_type` the registry knows, or, at a
// slot with a fallback, at an object with no `_type` member at all. Each
// refused row is the case one condition exists for: drop the identity check
// and the aggregate row comes back handled; drop the legacy check and the
// legacy row does; drop the registry lookup and the unknown-type row does. The
// leading-type read refuses the no-hooks, caller, null and not-first rows
// itself, and its own rows are in leadingtype_test.go. The handled
// rows put the slot after a name, as the first element of an array and after
// a comma, so a lead that did not step over the separator would refuse them.
//
// The rows with a fallback pin the absent `_type`: the slot is handled only
// when no member of its own, however late, is named `_type` (a `_type` nested
// deeper does not count), its names hold no escape that could spell `_type`,
// and the options read `_type` from that one member, so duplicate names
// allowed refuse it. Without a fallback, an absent `_type` is a refusal the
// buffered decode reports.
//
// A refused slot leaves the decoder where it was, so the buffered read that
// follows gets the whole slot: every refused row reads it back.
func TestDecodeSlotInPlaceGate(t *testing.T) {
	set := unmarshalerAgg.Load()
	if set == nil {
		t.Fatal("unmarshalerAgg.Load() = nil: this test needs the rm package's interface hooks, which the typereg_test files import")
	}
	dvText, ok := Default.Lookup("DV_TEXT")
	if !ok {
		t.Fatal(`Default.Lookup("DV_TEXT") found nothing: this test needs the rm package's registrations, which the typereg_test files import`)
	}
	caller := json.JoinUnmarshalers(set.all, json.UnmarshalFromFunc(func(dec *jsontext.Decoder, _ *fakeBox) error { return dec.SkipValue() }))
	const slot = `{"_type":"DV_TEXT","value":"x"}`
	cases := []struct {
		name       string
		doc        string
		lead       int  // tokens to read before the slot
		afterComma bool // read the first element whole, so the slot is the second
		opts       []json.Options
		fallback   func() any
		handled    bool
	}{
		{name: "no _type, with a fallback", doc: `{"value":{"value":"x"}}`, lead: 2, opts: []json.Options{set.bufferedOpt}, fallback: dvText, handled: true},
		{name: "no _type, a _type nested deeper", doc: `{"value":{"value":"x","zz":{"_type":"DV_TEXT"},"zy":[{"_type":"DV_TEXT"}]}}`, lead: 2, opts: []json.Options{set.bufferedOpt}, fallback: dvText, handled: true},
		{name: "no _type, without a fallback", doc: `{"value":{"value":"x"}}`, lead: 2, opts: []json.Options{set.bufferedOpt}},
		{name: "_type after a nested value, with a fallback", doc: `{"value":{"zz":{"a":[1,"]"]},"_type":"DV_TEXT","value":"x"}}`, lead: 2, opts: []json.Options{set.bufferedOpt}, fallback: dvText},
		{name: "escaped member name, with a fallback", doc: `{"value":{"value":"x","\u005ftype":"DV_TEXT"}}`, lead: 2, opts: []json.Options{set.bufferedOpt}, fallback: dvText},
		{name: "no _type, duplicate names allowed", doc: `{"value":{"value":"x"}}`, lead: 2, opts: []json.Options{set.bufferedOpt, jsontext.AllowDuplicateNames(true)}, fallback: dvText},
		{name: "no _type, case-insensitive names", doc: `{"value":{"value":"x"}}`, lead: 2, opts: []json.Options{set.bufferedOpt, json.MatchCaseInsensitiveNames(true)}, fallback: dvText},
		{name: "buffered identity, after a name", doc: `{"value":` + slot + `}`, lead: 2, opts: []json.Options{set.bufferedOpt}, handled: true},
		{name: "buffered identity, first array element", doc: `{"items":[` + slot + `]}`, lead: 3, opts: []json.Options{set.bufferedOpt}, handled: true},
		{name: "buffered identity, after a comma", doc: `{"items":[` + slot + ` , ` + slot + `]}`, lead: 3, afterComma: true, opts: []json.Options{set.bufferedOpt}, handled: true},
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
			// The SDK decodes a slot in place only within a value read whole into
			// memory; a bytes.Buffer gives the decoder all of the document in its
			// buffer, as that does.
			dec := jsontext.NewDecoder(bytes.NewBufferString(tc.doc), tc.opts...)
			for range tc.lead {
				if _, err := dec.ReadToken(); err != nil {
					t.Fatalf("ReadToken before the slot of %s: %v", tc.doc, err)
				}
			}
			if tc.afterComma {
				if _, err := dec.ReadValue(); err != nil {
					t.Fatalf("ReadValue of the first element of %s: %v", tc.doc, err)
				}
			}
			before := dec.InputOffset()
			var out any
			handled, err := decodeSlotInPlace(dec, &out, tc.fallback)
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

// TestLacksTypeMember pins the scan that lets a slot with a fallback decode in
// place: true only for an object it reads to its closing brace without
// meeting a member of its own named `_type`. A `_type` nested in a member's
// value is not the slot's; quotes, brackets and an escaped quote inside a
// string are text, not structure; a member name with an escape might decode to
// `_type`; and a value cut short, as a decoder's partly filled buffer would
// cut it, is never read as complete. Each false row with a `_type` sits after
// something the scan must step over correctly to reach it.
func TestLacksTypeMember(t *testing.T) {
	cases := []struct {
		raw  string
		want bool
	}{
		{raw: `{"value":"x"}`, want: true},
		{raw: ` {}`, want: true},
		{raw: `{"value":"x","zz":{"_type":"X"},"zy":[{"_type":"X"},"_type"]}`, want: true},
		{raw: `{"value":"x"},"_type":"X"}`, want: true},
		{raw: `{"a":"_type","b":["_type"]}`, want: true},
		{raw: `{"_type":"X"}`},
		{raw: `{"value":"x","_type":"X"}`},
		{raw: `{"zz":{"a":[1,"]}"]},"_type":"X"}`},
		{raw: `{"a":"\"}","_type":"X"}`},
		{raw: `{"a":"\\","_type":"X"}`},
		{raw: `{"zz":[[{}],[]],"_type":"X"}`},
		{raw: `{"\u005ftype":"X"}`},
		{raw: `{"value":"x"`},
		{raw: `{"value":"x","zz":{}`},
		{raw: `["_type"]`},
		{raw: `null`},
		{raw: ``},
	}
	for _, tc := range cases {
		if got := lacksTypeMember([]byte(tc.raw)); got != tc.want {
			t.Errorf("lacksTypeMember(%s) = %v, want %v", tc.raw, got, tc.want)
		}
	}
}

// TestSettleSlotErrorSettlesASharedFailureOnce pins that a failure above a
// chain that splits into several causes takes the slot's own positions once,
// as it does above a single cause (REQ-052): the causes share it, and a second
// settling would take the slot's pointer and offset off it again.
func TestSettleSlotErrorSettlesASharedFailureOnce(t *testing.T) {
	cases := []struct {
		name  string
		inner error
	}{
		{name: "one cause", inner: errors.New("a")},
		{name: "two causes", inner: errors.Join(errors.New("a"), errors.New("b"))},
		{name: "two causes by %w", inner: fmt.Errorf("%w: %w", errors.New("a"), errors.New("b"))},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := &json.SemanticError{JSONPointer: "/s/s/x", ByteOffset: 300, Err: tc.inner}
			settleSlotError(e, "/s", 100)
			if e.JSONPointer != "/s/x" || e.ByteOffset != 200 {
				t.Errorf("settleSlotError(%s at (/s/s/x, 300), /s, 100) placed it at (%s, %d), want (/s/x, 200)", tc.name, e.JSONPointer, e.ByteOffset)
			}
		})
	}
}
