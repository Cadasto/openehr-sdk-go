package typereg

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
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
