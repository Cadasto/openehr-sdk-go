package typereg

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"strings"
	"testing"
)

// TestDecodeOptionsMemoisesCallerJoin pins that decodeOptions joins a
// caller-supplied hook set into the SDK aggregate once per caller pointer, not
// once per nested decode level: callerJoinCache serves every call after the
// first for the same caller. Without the memo the join would run on every
// nested value, so a deep tree would re-join once per level (the deferred-minor
// finding). Mutation: replace the joinCallerUnmarshalers call in decodeOptions
// with a direct json.JoinUnmarshalers(hooks, caller) and the count becomes 3.
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
