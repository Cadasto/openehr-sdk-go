package simplified_test

// REQ-053 — an OBSERVATION's explicit `<observation>/history_origin`, which
// ITS-REST 1.1.0 Simplified Formats § OBSERVATION maps to `history.origin`, a
// DV_DATE_TIME "set to the time of the earliest event if not explicitly set".
// The Web Template gives it no node. Decode reads it into the HISTORY's origin,
// ahead of the earliest event time and the start time; encode writes it where
// decode would not derive the same origin from the events.

import (
	"errors"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/simplified"
)

// explicitOrigin is the corpus `observation` fixture's explicit origin, a day
// before its event time and before its start time.
const (
	historyOriginKey = eventObs + "/history_origin"
	explicitOrigin   = "2021-12-20T15:02:58.0094262+01:00"
)

// TestREQ053HistoryOriginDecodes — REQ-053. The explicit history_origin is the
// HISTORY's origin, with and without the template, in FLAT and in STRUCTURED,
// though the event's time is later and the body has a start time; nothing in
// the body is unmatched.
func TestREQ053HistoryOriginDecodes(t *testing.T) {
	wt, compiled := conformanceTarget(t)
	body := mustJSON(t, eventBody(map[string]any{historyOriginKey: explicitOrigin}))
	for _, mode := range shorthandModes(compiled) {
		for _, format := range []string{"FLAT", "STRUCTURED"} {
			t.Run(mode.name+"/"+format, func(t *testing.T) {
				opts := append([]simplified.Option{simplified.WithStrict()}, mode.opts...)
				obs := observationOf(t, wt, decodeAs(t, format, body, wt, opts...), "conformance_observation")
				if obs.Data.Origin.Value != explicitOrigin {
					t.Errorf("origin = %q, want the explicit %q", obs.Data.Origin.Value, explicitOrigin)
				}
			})
		}
	}
}

// TestREQ053HistoryOriginComesFirst — REQ-053. With the template the origin is
// the explicit history_origin, else the earliest event time, else the start
// time; an event the body gives no time takes the explicit origin. A null or
// empty history_origin is absent, and the origin falls back as without it.
func TestREQ053HistoryOriginComesFirst(t *testing.T) {
	wt, compiled := conformanceTarget(t)
	const eventTime = "2021-12-21T16:02:58.0094262+01:00"
	timeless := eventBody(map[string]any{historyOriginKey: explicitOrigin})
	delete(timeless, eventAt+"/time")
	for _, tc := range []struct {
		name             string
		body             map[string]any
		origin, evAtTime string
	}{
		{"explicit before the event time", eventBody(map[string]any{historyOriginKey: explicitOrigin}), explicitOrigin, eventTime},
		{"explicit gives a timeless event its time", timeless, explicitOrigin, explicitOrigin},
		{"null falls back to the event time", eventBody(map[string]any{historyOriginKey: nil}), eventTime, eventTime},
		{"empty falls back to the event time", eventBody(map[string]any{historyOriginKey: ""}), eventTime, eventTime},
	} {
		t.Run(tc.name, func(t *testing.T) {
			comp := decodeWith(t, wt, tc.body, simplified.WithTemplate(compiled))
			obs := observationOf(t, wt, comp, "conformance_observation")
			if obs.Data.Origin.Value != tc.origin {
				t.Errorf("origin = %q, want %q", obs.Data.Origin.Value, tc.origin)
			}
			if got := eventTimeOf(t, onlyEvent(t, wt, comp, "conformance_observation")); got != tc.evAtTime {
				t.Errorf("event time = %q, want %q", got, tc.evAtTime)
			}
		})
	}
}

// TestREQ053HistoryOriginMustBeADateTime — REQ-053 "A value must fit its
// attribute". history_origin is a DV_DATE_TIME: a number is refused with
// ErrUnsupportedDatatype naming the key, and an index past :0 on the
// single-valued attribute with ErrUnknownPath.
func TestREQ053HistoryOriginMustBeADateTime(t *testing.T) {
	wt, _ := conformanceTarget(t)
	_, err := simplified.UnmarshalFlat(mustJSON(t, eventBody(map[string]any{historyOriginKey: 20211220})), wt)
	if !errors.Is(err, simplified.ErrUnsupportedDatatype) || !strings.Contains(err.Error(), historyOriginKey) {
		t.Errorf("a numeric history_origin: err = %v, want ErrUnsupportedDatatype naming %q", err, historyOriginKey)
	}
	_, err = simplified.UnmarshalFlat(mustJSON(t, eventBody(map[string]any{historyOriginKey + ":1": explicitOrigin})), wt)
	if !errors.Is(err, simplified.ErrUnknownPath) {
		t.Errorf("history_origin:1: err = %v, want ErrUnknownPath", err)
	}
}

// TestREQ053HistoryOriginEncodes — REQ-053. Encode writes the origin at
// `<observation>/history_origin` where it differs from the earliest event time
// decode would give it, and decode reads the FLAT and the STRUCTURED it writes
// back to that origin, without the template, which derives none; an origin
// that is the earliest event time writes no key.
func TestREQ053HistoryOriginEncodes(t *testing.T) {
	wt, compiled := conformanceTarget(t)
	explicit := decodeWith(t, wt, eventBody(map[string]any{historyOriginKey: explicitOrigin}), simplified.WithTemplate(compiled))
	got := flatOf(t, explicit, wt)
	if got[historyOriginKey] != explicitOrigin {
		t.Errorf("MarshalFlat wrote %q = %#v, want %q", historyOriginKey, got[historyOriginKey], explicitOrigin)
	}
	for format, back := range map[string]*rm.Composition{
		"FLAT":       decodeAs(t, "FLAT", mustJSON(t, got), wt, simplified.WithStrict()),
		"STRUCTURED": structuredRoundTrip(t, explicit, wt),
	} {
		if origin := observationOf(t, wt, back, "conformance_observation").Data.Origin.Value; origin != explicitOrigin {
			t.Errorf("decode of the encoded %s: origin = %q, want %q", format, origin, explicitOrigin)
		}
	}

	derived := decodeWith(t, wt, eventBody(nil), simplified.WithTemplate(compiled))
	if v, written := flatOf(t, derived, wt)[historyOriginKey]; written {
		t.Errorf("an origin at the earliest event time wrote %q = %#v", historyOriginKey, v)
	}
}
