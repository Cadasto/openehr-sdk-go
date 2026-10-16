package typereg

import (
	jsonv1 "encoding/json"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"io"
	"strings"
	"testing"
)

// TestDecodeOptionsNilWhenDecoderCarriesSDKOptions pins when decodeOptions
// adds nothing. json.UnmarshalDecode and json.Unmarshal apply the options a call
// passes to the decoder for that call's duration, so a nested value's decoder
// already carries the hooks and the v2 error semantics its parent added; for
// such a decoder decodeOptions returns nil, which every json/v2 entry point
// skips. That holds when the decoder does not report errors with legacy
// semantics and its hook set is the SDK aggregate itself or the join the memo
// last produced for that aggregate. Every other decoder gets today's join: the
// SDK hooks and v2 error semantics on top of its own options.
//
// Each non-nil row is the case one condition exists for. Drop the legacy check
// and the two legacy rows come back nil; drop the hooks-equality check on the
// memo entry and the stale-aggregate row comes back nil; drop the joined-pointer
// check and every row whose decoder carries neither the aggregate nor the
// memoised join (no set, a nil set, an unrelated caller, a displaced join)
// comes back nil.
//
// Each row stores its own entry in the package-wide memo, so the test puts the
// entry it found back when it ends: the memo-counting tests in
// streaming_test.go then see the memo as the rest of the binary left it,
// whatever order the tests run in.
func TestDecodeOptionsNilWhenDecoderCarriesSDKOptions(t *testing.T) {
	agg := aggregateUnmarshalers()
	if agg == nil {
		t.Fatal("aggregateUnmarshalers() = nil: this test needs the rm package's interface hooks, which the typereg_test files import")
	}
	saved := lastCallerJoin.Load()
	t.Cleanup(func() { lastCallerJoin.Store(saved) })
	skip := func(dec *jsontext.Decoder, _ *fakeBox) error { return dec.SkipValue() }
	caller := json.UnmarshalFromFunc(skip)
	other := json.UnmarshalFromFunc(skip)
	staleAgg := json.UnmarshalFromFunc(skip)
	joined := json.JoinUnmarshalers(agg, caller)
	otherJoined := json.JoinUnmarshalers(agg, other)

	memoCaller := &joinEntry{hooks: agg, caller: caller, joined: joined}
	memoOther := &joinEntry{hooks: agg, caller: other, joined: otherJoined}
	memoStale := &joinEntry{hooks: staleAgg, caller: caller, joined: joined}

	cases := []struct {
		name    string
		opts    []json.Options
		memo    *joinEntry // stored in lastCallerJoin before the call
		wantNil bool
		// wantAgg: a non-nil result must carry exactly the SDK aggregate
		// (no caller to join); otherwise it carries a join of the aggregate
		// with the decoder's own set.
		wantAgg bool
	}{
		{name: "aggregate", opts: []json.Options{json.WithUnmarshalers(agg)}, memo: memoCaller, wantNil: true},
		{name: "aggregate with v2 error semantics", opts: []json.Options{json.WithUnmarshalers(agg), jsonv1.ReportErrorsWithLegacySemantics(false)}, memo: memoCaller, wantNil: true},
		{name: "memoised join", opts: []json.Options{json.WithUnmarshalers(joined)}, memo: memoCaller, wantNil: true},
		{name: "aggregate with legacy error semantics", opts: []json.Options{json.WithUnmarshalers(agg), jsonv1.ReportErrorsWithLegacySemantics(true)}, memo: memoCaller, wantAgg: true},
		{name: "memoised join with legacy error semantics", opts: []json.Options{json.WithUnmarshalers(joined), jsonv1.ReportErrorsWithLegacySemantics(true)}, memo: memoCaller},
		{name: "no hook set", memo: memoCaller, wantAgg: true},
		{name: "nil hook set", opts: []json.Options{json.WithUnmarshalers(nil)}, memo: memoCaller, wantAgg: true},
		{name: "unrelated caller set", opts: []json.Options{json.WithUnmarshalers(other)}, memo: memoCaller},
		{name: "join displaced from the memo", opts: []json.Options{json.WithUnmarshalers(joined)}, memo: memoOther},
		{name: "join memoised for a stale aggregate", opts: []json.Options{json.WithUnmarshalers(joined)}, memo: memoStale},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lastCallerJoin.Store(tc.memo)
			dec := jsontext.NewDecoder(strings.NewReader(`{}`), tc.opts...)
			got := decodeOptions(dec)
			if tc.wantNil {
				if got != nil {
					t.Fatalf("decodeOptions(decoder with %s) = %v, want nil: the decoder already carries the SDK hooks and v2 error semantics", tc.name, got)
				}
				return
			}
			if got == nil {
				t.Fatalf("decodeOptions(decoder with %s) = nil, want the SDK hooks and v2 error semantics joined on top of the decoder's options", tc.name)
			}
			if legacy, ok := json.GetOption(got, jsonv1.ReportErrorsWithLegacySemantics); legacy || !ok {
				t.Errorf("decodeOptions(decoder with %s): ReportErrorsWithLegacySemantics = %v (present %v), want false (present true)", tc.name, legacy, ok)
			}
			hooks, _ := json.GetOption(got, json.WithUnmarshalers)
			switch {
			case tc.wantAgg && hooks != agg:
				t.Errorf("decodeOptions(decoder with %s): hook set = %p, want the SDK aggregate %p", tc.name, hooks, agg)
			case !tc.wantAgg && (hooks == nil || hooks == agg):
				t.Errorf("decodeOptions(decoder with %s): hook set = %p, want a join of the SDK aggregate %p with the decoder's own set", tc.name, hooks, agg)
			}
		})
	}
}

// TestMarshalOptionsNilWhenEncoderCarriesEncodeOptions pins when MarshalOptions
// adds nothing. json.MarshalEncode applies the options a call passes to the
// encoder for that call's duration, so once an enclosing value (or the entry
// point) has applied EncodeOptions, a nested value's encoder already has all
// three set; MarshalOptions then returns nil, which MarshalEncode skips.
// Missing or false, any one of the three makes it return EncodeOptions, and the
// encoder's options joined with the result have all three set. Drop any one of
// the three checks and that option's "missing" and "false" rows come back nil.
func TestMarshalOptionsNilWhenEncoderCarriesEncodeOptions(t *testing.T) {
	det, nilSlice, nilMap := json.Deterministic, json.FormatNilSliceAsNull, json.FormatNilMapAsNull
	cases := []struct {
		name    string
		opts    []json.Options
		wantNil bool
	}{
		{name: "all three", opts: []json.Options{det(true), nilSlice(true), nilMap(true)}, wantNil: true},
		{name: "EncodeOptions", opts: []json.Options{EncodeOptions()}, wantNil: true},
		{name: "none"},
		{name: "Deterministic missing", opts: []json.Options{nilSlice(true), nilMap(true)}},
		{name: "FormatNilSliceAsNull missing", opts: []json.Options{det(true), nilMap(true)}},
		{name: "FormatNilMapAsNull missing", opts: []json.Options{det(true), nilSlice(true)}},
		{name: "Deterministic false", opts: []json.Options{det(false), nilSlice(true), nilMap(true)}},
		{name: "FormatNilSliceAsNull false", opts: []json.Options{det(true), nilSlice(false), nilMap(true)}},
		{name: "FormatNilMapAsNull false", opts: []json.Options{det(true), nilSlice(true), nilMap(false)}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			enc := jsontext.NewEncoder(io.Discard, tc.opts...)
			got := MarshalOptions(enc)
			if tc.wantNil {
				if got != nil {
					t.Fatalf("MarshalOptions(encoder with %s) = %v, want nil: the encoder already carries the three encode options", tc.name, got)
				}
				return
			}
			if got == nil {
				t.Fatalf("MarshalOptions(encoder with %s) = nil, want EncodeOptions to join on top of the encoder's options", tc.name)
			}
			effective := json.JoinOptions(enc.Options(), got)
			for _, opt := range []struct {
				name   string
				setter func(bool) json.Options
			}{{"Deterministic", det}, {"FormatNilSliceAsNull", nilSlice}, {"FormatNilMapAsNull", nilMap}} {
				if v, _ := json.GetOption(effective, opt.setter); !v {
					t.Errorf("encoder with %s, joined with MarshalOptions: %s = false, want true", tc.name, opt.name)
				}
			}
		})
	}
}

// TestEncodeOptionsIsBuiltOnce pins that EncodeOptions hands every caller the
// same prebuilt value, carrying the three canonical encode options, rather than
// building a new set (an allocation) per call.
func TestEncodeOptionsIsBuiltOnce(t *testing.T) {
	first, second := EncodeOptions(), EncodeOptions()
	if first != second {
		t.Errorf("EncodeOptions() returned %p, then %p: want one value built once and shared", first, second)
	}
	for _, opt := range []struct {
		name   string
		setter func(bool) json.Options
	}{{"Deterministic", json.Deterministic}, {"FormatNilSliceAsNull", json.FormatNilSliceAsNull}, {"FormatNilMapAsNull", json.FormatNilMapAsNull}} {
		if v, ok := json.GetOption(first, opt.setter); !v || !ok {
			t.Errorf("EncodeOptions(): %s = %v (present %v), want true (present true)", opt.name, v, ok)
		}
	}
}

// slotValue is the interface the depth-bound test fills through
// DecodePolymorphic. No RM type implements it: every row expects an error
// before a decoded value would be stored in it.
type slotValue interface{ isSlotValue() }

// TestDecodePolymorphicDepthScanBound pins the bound that lets DecodePolymorphic
// skip the bracket scan of a buffered slot (REQ-108). A value nested d levels
// deep holds d opening and d closing brackets, so d is at most half the slot's
// length, and the scan runs only when the enclosing stack depth plus that bound
// passes the cap. A slot of bare brackets is the tightest case: its depth is
// exactly half its length. So 511 brackets under an object (stack depth 1)
// reach the cap and pass the depth check, failing later at the _type peek as a
// shape error, while 512 are refused with "(513 > 512)". A slot nested past the
// cap only through a member no type declares is refused too: no concrete
// decode ever sees that member, so the slot's bracket scan is its only bound.
//
// Can-fail: with the skip condition loosened by one (half the length minus
// one), the 512-bracket slot skips the scan and fails at the peek instead; with
// the scan always skipped, both refusal rows fail.
func TestDecodePolymorphicDepthScanBound(t *testing.T) {
	brackets := func(n int) string { return strings.Repeat("[", n) + strings.Repeat("]", n) }
	cases := []struct {
		name    string
		slot    string
		refused bool   // refused with ErrMaxDepthExceeded
		wantMsg string // when refused: text the refusal must carry
	}{
		{name: "brackets reaching the cap", slot: brackets(maxDecodeDepth - 1)},
		{name: "brackets one past the cap", slot: brackets(maxDecodeDepth), refused: true, wantMsg: "(513 > 512)"},
		{name: "undeclared member past the cap", slot: `{"_type":"DV_TEXT","value":"x","zz":` + brackets(600) + `}`, refused: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dec := jsontext.NewDecoder(strings.NewReader(`{"slot":` + tc.slot + `}`))
			// Step into the enclosing object, so the slot opens at stack depth 1.
			for range 2 {
				if _, err := dec.ReadToken(); err != nil {
					t.Fatalf("ReadToken before the slot: %v", err)
				}
			}
			var out slotValue
			err := DecodePolymorphic(dec, &out, nil)
			if !tc.refused {
				if errors.Is(err, ErrMaxDepthExceeded) {
					t.Fatalf("DecodePolymorphic(%s at stack depth 1) err = %v, want no depth refusal: the slot reaches the cap without passing it", tc.name, err)
				}
				if !errors.Is(err, ErrInvalidShape) {
					t.Fatalf("DecodePolymorphic(%s at stack depth 1) err = %v, want the _type peek's shape failure (proof the decode got past the depth check)", tc.name, err)
				}
				return
			}
			if !errors.Is(err, ErrMaxDepthExceeded) {
				t.Fatalf("DecodePolymorphic(%s at stack depth 1) err = %v, want errors.Is(_, ErrMaxDepthExceeded)", tc.name, err)
			}
			if tc.wantMsg != "" && !strings.Contains(err.Error(), tc.wantMsg) {
				t.Errorf("DecodePolymorphic(%s at stack depth 1) err = %v, want the text to contain %q", tc.name, err, tc.wantMsg)
			}
		})
	}
}
