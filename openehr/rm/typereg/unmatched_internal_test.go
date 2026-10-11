package typereg

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"strings"
	"testing"
)

// TestDecodeOptionsThreadsTheTrackedSet pins how decodeOptions treats the
// tracked hook set (REQ-170): it reports the decode as tracked and threads the
// set on as it is, with no caller join, so every nesting level sees the same
// pointer and stays tracked.
//
// Can-fail: treating the tracked set as an ordinary caller set joins it into
// the aggregate, which costs a join here and hands the next level a new
// pointer that no longer reads as tracked.
func TestDecodeOptionsThreadsTheTrackedSet(t *testing.T) {
	dec := jsontext.NewDecoder(strings.NewReader(`{}`), json.WithUnmarshalers(trackedUnmarshalers()))

	before := callerJoinCount.Load()
	opts, tracked := decodeOptions(dec)
	if !tracked {
		t.Fatal("decodeOptions(decoder carrying the tracked set) tracked = false; want true")
	}
	if got := callerJoinCount.Load() - before; got != 0 {
		t.Errorf("decodeOptions joined the tracked set %d times; want 0: it already holds the aggregate", got)
	}
	if got, _ := json.GetOption(opts, json.WithUnmarshalers); got != trackedUnmarshalers() {
		t.Error("decodeOptions did not thread the tracked set on as it is; the next level would not read as tracked")
	}
}

// TestUntrackedDecodeIsNotTracked is the other side: a decoder carrying the
// plain aggregate, or no hooks, is not tracked.
func TestUntrackedDecodeIsNotTracked(t *testing.T) { // REQ-170
	for name, dec := range map[string]*jsontext.Decoder{
		"aggregate": jsontext.NewDecoder(strings.NewReader(`{}`), Unmarshalers()),
		"no hooks":  jsontext.NewDecoder(strings.NewReader(`{}`)),
	} {
		if _, tracked := decodeOptions(dec); tracked {
			t.Errorf("decodeOptions(%s) tracked = true; want false", name)
		}
	}
}

// TestTrackerOfAnUnregisteredDecoderIgnores pins the fallback of trackerOf: a
// decoder no tracked decode registered gets the empty policy, which neither
// reports nor refuses, rather than a nil policy dereferenced in the hook.
// Can-fail: asserting the registry entry without its ok check panics on the
// nil interface.
func TestTrackerOfAnUnregisteredDecoderIgnores(t *testing.T) { // REQ-170
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("trackerOf(unregistered decoder) panicked: %v", r)
		}
	}()
	dec := jsontext.NewDecoder(strings.NewReader(`{}`))
	if err := trackerOf(dec).member("/x"); err != nil {
		t.Errorf("trackerOf(unregistered decoder).member(/x) = %v; want nil, the member ignored", err)
	}
}
