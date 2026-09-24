package typereg

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"strings"
	"testing"
)

// TestDecodeOptionsMemoisesCallerJoin pins that decodeOptions joins a
// caller-supplied hook set into the SDK aggregate once for a run of calls under
// one caller pointer, not once per call: after the first, lastCallerJoin holds
// that pair and every repeat reuses its join. This is the property that lets
// sibling values at one nesting level, and a later decode reusing the same
// WithUnmarshalers value, cost no extra join.
//
// Mutation: replace the joinCallerUnmarshalers call in decodeOptions with a
// direct json.JoinUnmarshalers(hooks, caller). callerJoinCount lives inside the
// helper, so bypassing it never increments the counter and the observed count
// drops to 0 (not 1), which trips the assertion.
func TestDecodeOptionsMemoisesCallerJoin(t *testing.T) {
	caller := json.UnmarshalFromFunc(func(dec *jsontext.Decoder, _ *fakeBox) error {
		return dec.SkipValue()
	})
	dec := jsontext.NewDecoder(strings.NewReader(`{}`), json.WithUnmarshalers(caller))

	before := callerJoinCount.Load()
	for range 3 {
		_ = decodeOptions(dec)
	}
	if got := callerJoinCount.Load() - before; got != 1 {
		t.Errorf("decodeOptions joined the caller %d times across three calls under one caller pointer, want 1: the join must be memoised per caller, not repeated per nested level", got)
	}
}

// TestDecodeOptionsNilCallerIsNoCaller pins that json.WithUnmarshalers(nil) is
// treated as no caller: decodeOptions performs no join for it, so a caller that
// explicitly passes a nil hook set costs nothing. Mutation: drop the
// `caller != nil` guard in decodeOptions and the count becomes 1, a pointless
// join of the aggregate with nil.
func TestDecodeOptionsNilCallerIsNoCaller(t *testing.T) {
	dec := jsontext.NewDecoder(strings.NewReader(`{}`), json.WithUnmarshalers(nil))

	before := callerJoinCount.Load()
	_ = decodeOptions(dec)
	if got := callerJoinCount.Load() - before; got != 0 {
		t.Errorf("decodeOptions joined %d times for a nil caller, want 0: WithUnmarshalers(nil) must be treated as no caller", got)
	}
}

// TestDecodeOptionsMemoHoldsOneEntry pins that the caller-hook memo retains only
// the last (aggregate, caller) pair, never a growing set. Two distinct caller
// hook sets alternating miss on every call, so each drives a fresh join; the
// same caller set twice hits on the second call, so it joins once. That is the
// single-entry memo's defining behaviour: it collapses repeats for one caller
// (siblings at a level, a later decode reusing the same WithUnmarshalers value)
// without holding any earlier caller's pointer alive.
//
// Mutation: give joinCallerUnmarshalers a map keyed on (hooks, caller) in place
// of the single-entry lastCallerJoin, and the alternating count drops from 4 to
// 2, because a map answers the second sighting of each caller from its retained
// entry. That drop trips the first assertion below.
func TestDecodeOptionsMemoHoldsOneEntry(t *testing.T) {
	callerA := json.UnmarshalFromFunc(func(dec *jsontext.Decoder, _ *fakeBox) error {
		return dec.SkipValue()
	})
	callerB := json.UnmarshalFromFunc(func(dec *jsontext.Decoder, _ *fakeBox) error {
		return dec.SkipValue()
	})
	decA := jsontext.NewDecoder(strings.NewReader(`{}`), json.WithUnmarshalers(callerA))
	decB := jsontext.NewDecoder(strings.NewReader(`{}`), json.WithUnmarshalers(callerB))

	before := callerJoinCount.Load()
	_ = decodeOptions(decA) // miss: first sight of callerA
	_ = decodeOptions(decB) // miss: the stored pair is callerA
	_ = decodeOptions(decA) // miss: the stored pair is callerB
	_ = decodeOptions(decB) // miss: the stored pair is callerA
	if got := callerJoinCount.Load() - before; got != 4 {
		t.Errorf("two distinct callers alternating joined %d times across four calls, want 4: a single-entry memo cannot serve either after the other displaces it", got)
	}

	before = callerJoinCount.Load()
	_ = decodeOptions(decA) // miss: the stored pair is callerB
	_ = decodeOptions(decA) // hit: the stored pair is now callerA
	if got := callerJoinCount.Load() - before; got != 1 {
		t.Errorf("the same caller twice in a row joined %d times, want 1: the second call must reuse the memo entry", got)
	}
}

// wireText is a stand-in wire struct for the nil-argument tests: it declares
// the _type field DecodeInto's gotType points into.
type wireText struct {
	Type  string `json:"_type"`
	Value string `json:"value"`
}

// callNoPanic runs f and fails the test if it panics, returning f's error.
func callNoPanic(t *testing.T, call string, f func() error) (err error) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("%s panicked: %v; want an error wrapping ErrNilArgument", call, r)
		}
	}()
	return f()
}

// assertNilArgument checks the operation-specific facets of a nil-argument
// refusal: a *DecodeError, ErrNilArgument in the chain, the argument named in
// the text, and neither the nil-receiver nor the shape sentinel.
func assertNilArgument(t *testing.T, call, arg string, err error) {
	t.Helper()
	if !errors.Is(err, ErrNilArgument) {
		t.Fatalf("%s err = %v; want errors.Is(_, ErrNilArgument)", call, err)
	}
	if de, ok := errors.AsType[*DecodeError](err); !ok || de == nil {
		t.Errorf("%s err = %v (%T); want a *DecodeError", call, err, err)
	}
	if !strings.Contains(err.Error(), ": "+arg+" for ") {
		t.Errorf("%s err = %q; want the text to name the nil argument %q", call, err, arg)
	}
	if errors.Is(err, ErrNilReceiver) || errors.Is(err, ErrInvalidShape) {
		t.Errorf("%s err = %v; a nil argument must match neither ErrNilReceiver nor ErrInvalidShape", call, err)
	}
}

// TestDecodeIntoNilArguments pins the REQ-025 no-panic rule on DecodeInto's
// pointer arguments (ruling P20): a nil dec, a nil out and a nil gotType are
// caller misuse (no generated body passes one), so each is refused with a
// *DecodeError wrapping ErrNilArgument before any decode rather than
// dereferenced. Can-fail: removing a nil check panics in its case (dec on
// StackDepth, gotType on the `*gotType` read once the decode succeeds) or, for
// out, returns the v2 decoder's own non-pointer error without ErrNilArgument.
func TestDecodeIntoNilArguments(t *testing.T) {
	const body = `{"_type":"DV_TEXT","value":"x"}`
	cases := []struct {
		arg  string
		call func(out *wireText) error
	}{
		{"dec", func(out *wireText) error { return DecodeInto(nil, "DV_TEXT", out, &out.Type) }},
		{"out", func(out *wireText) error {
			return DecodeInto(jsontext.NewDecoder(strings.NewReader(body)), "DV_TEXT", nil, &out.Type)
		}},
		{"gotType", func(out *wireText) error {
			return DecodeInto(jsontext.NewDecoder(strings.NewReader(body)), "DV_TEXT", out, nil)
		}},
	}
	for _, tc := range cases {
		t.Run("nil "+tc.arg, func(t *testing.T) {
			var out wireText
			call := "DecodeInto(nil " + tc.arg + ")"
			err := callNoPanic(t, call, func() error { return tc.call(&out) })
			assertNilArgument(t, call, tc.arg, err)
			if out.Value != "" {
				t.Errorf("%s decoded Value = %q; want the refusal before any decode", call, out.Value)
			}
		})
	}
}

// TestDecodePolymorphicNilArguments is the DecodePolymorphic twin (REQ-025,
// ruling P20): a nil dec and a nil out are refused with a *DecodeError
// wrapping ErrNilArgument before any read. Can-fail: removing the dec check
// panics on ReadValue; removing the out check panics on the final `*out`
// store once the slot decodes.
func TestDecodePolymorphicNilArguments(t *testing.T) {
	// No _type, so the slot takes the fallback and needs no registry entry:
	// with the out check removed the decode succeeds and the `*out` store
	// panics.
	const body = `{"value":"x"}`
	fallback := func() any { return new(wireText) }
	t.Run("nil dec", func(t *testing.T) {
		var out any
		call := "DecodePolymorphic(nil dec)"
		err := callNoPanic(t, call, func() error { return DecodePolymorphic[any](nil, &out, fallback) })
		assertNilArgument(t, call, "dec", err)
	})
	t.Run("nil out", func(t *testing.T) {
		dec := jsontext.NewDecoder(strings.NewReader(body))
		call := "DecodePolymorphic(nil out)"
		err := callNoPanic(t, call, func() error { return DecodePolymorphic[any](dec, nil, fallback) })
		assertNilArgument(t, call, "out", err)
		if dec.InputOffset() != 0 {
			t.Errorf("%s read %d bytes; want the refusal before any read", call, dec.InputOffset())
		}
	})
}
