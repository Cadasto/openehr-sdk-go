package simplified_test

// REQ-053 — two more defaults a WithTemplate decode gives a body that leaves
// them out. A HISTORY with no origin takes the earliest time its own events
// carry, compared as instants, else the start time, as the reference
// implementation orders the two; an event with no time then takes its
// HISTORY's origin (Simplified Formats § time: "EVENT.time will be set to
// history.origin").

import (
	"fmt"
	"maps"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/simplified"
)

// vitalSignsID is the corpus template whose body_temperature OBSERVATION, and
// the EVENT under its one HISTORY, both repeat.
const vitalSignsID = "vital_signs"

// originBodyStart is the start time the origin cases give in ctx/time.
const originBodyStart = "2026-11-10T12:00:00Z"

// temperatureEvent returns the FLAT keys of event ev of the body_temperature
// OBSERVATION obs: a temperature and, where at is not "", the event's time.
func temperatureEvent(obs, ev int, at string) map[string]any {
	prefix := fmt.Sprintf("encounter/body_temperature:%d/any_event:%d", obs, ev)
	keys := map[string]any{
		prefix + "/temperature|magnitude": 37.5,
		prefix + "/temperature|unit":      "°C",
	}
	if at != "" {
		keys[prefix+"/time"] = at
	}
	return keys
}

// originBody is a vital_signs FLAT body: the ctx/ fields, ctx/time where start
// is not "", and the events given.
func originBody(start string, events ...map[string]any) map[string]any {
	body := map[string]any{
		"ctx/language":      "en",
		"ctx/territory":     "NL",
		"ctx/composer_name": "Silvia Blake",
	}
	if start != "" {
		body["ctx/time"] = start
	}
	for _, ev := range events {
		maps.Copy(body, ev)
	}
	return body
}

// historyOrigin returns the canonical-JSON member path, from the composition, of
// the origin of the HISTORY of content item obs.
func historyOrigin(obs int) []any {
	return []any{"content", obs, "data", "origin", "value"}
}

// eventTime returns the canonical-JSON member path, from the composition, of
// the time of event ev of the HISTORY of content item obs.
func eventTime(obs, ev int) []any {
	return []any{"content", obs, "data", "events", ev, "time", "value"}
}

// memberOf returns what path, a path of canonical-JSON members from the
// composition (a name, or an array index), holds in comp, or nil where it
// holds nothing. A required DV_DATE_TIME decode left out holds "", the value
// canonical JSON writes for it.
func memberOf(t *testing.T, comp *rm.Composition, path []any) any {
	t.Helper()
	var at any = canonicalDoc(t, comp)
	for _, key := range path {
		switch k := key.(type) {
		case string:
			m, _ := at.(map[string]any)
			at = m[k]
		case int:
			list, _ := at.([]any)
			if k >= len(list) {
				return nil
			}
			at = list[k]
		}
	}
	return at
}

// TestREQ053HistoryOriginOrder — REQ-053: with the template, a HISTORY the
// body gives no origin takes the earliest time its own events carry, compared
// as instants (a time with no zone read in UTC, as the reference reads it),
// else the start time, the body's or the WithDefaultTime default, else none.
// An event the body gives no time takes its HISTORY's origin. Each HISTORY
// takes its own.
func TestREQ053HistoryOriginOrder(t *testing.T) {
	const (
		// zonedEarlier is 09:30 UTC: earlier as an instant than tenUTC,
		// though its clock reads later.
		zonedEarlier = "2026-11-10T10:30:00+01:00"
		tenUTC       = "2026-11-10T10:00:00Z"
		// unzoned is 09:15 read in UTC, earlier than zonedEarlier.
		unzoned = "2026-11-10T09:15:00"
		other   = "2026-11-09T08:00:00Z"
	)
	wt, compiled := templateOf(t, vitalSignsID)
	cases := []struct {
		name string
		body map[string]any
		opts []simplified.Option
		// want maps a canonical-JSON member path, by its fmt %v spelling, to
		// the time it holds, or to "" where it holds none.
		want map[string]string
		// validates is whether the composition validates.
		validates bool
	}{
		{
			name: "the earliest event time, as an instant",
			body: originBody(originBodyStart,
				temperatureEvent(0, 0, tenUTC), temperatureEvent(0, 1, zonedEarlier), temperatureEvent(0, 2, "")),
			want: map[string]string{
				fmt.Sprint(historyOrigin(0)): zonedEarlier,
				fmt.Sprint(eventTime(0, 0)):  tenUTC,
				fmt.Sprint(eventTime(0, 1)):  zonedEarlier,
				fmt.Sprint(eventTime(0, 2)):  zonedEarlier,
			},
			validates: true,
		},
		{
			name: "a time with no zone read in UTC",
			body: originBody(originBodyStart,
				temperatureEvent(0, 0, zonedEarlier), temperatureEvent(0, 1, unzoned), temperatureEvent(0, 2, "")),
			want: map[string]string{
				fmt.Sprint(historyOrigin(0)): unzoned,
				fmt.Sprint(eventTime(0, 2)):  unzoned,
			},
			validates: true,
		},
		{
			name: "no event time: the start time",
			body: originBody(originBodyStart, temperatureEvent(0, 0, ""), temperatureEvent(0, 1, "")),
			want: map[string]string{
				fmt.Sprint(historyOrigin(0)): originBodyStart,
				fmt.Sprint(eventTime(0, 0)):  originBodyStart,
				fmt.Sprint(eventTime(0, 1)):  originBodyStart,
			},
			validates: true,
		},
		{
			name: "each HISTORY on its own",
			body: originBody(originBodyStart,
				temperatureEvent(0, 0, ""), temperatureEvent(0, 1, other),
				temperatureEvent(1, 0, "")),
			want: map[string]string{
				fmt.Sprint(historyOrigin(0)): other,
				fmt.Sprint(eventTime(0, 0)):  other,
				fmt.Sprint(historyOrigin(1)): originBodyStart,
				fmt.Sprint(eventTime(1, 0)):  originBodyStart,
			},
			validates: true,
		},
		{
			name: "the default time yields to an event time",
			body: originBody("", temperatureEvent(0, 0, ""), temperatureEvent(0, 1, other)),
			opts: []simplified.Option{simplified.WithDefaultTime(callerClock)},
			want: map[string]string{
				fmt.Sprint(historyOrigin(0)): other,
				fmt.Sprint(eventTime(0, 0)):  other,
				fmt.Sprint(eventTime(0, 1)):  other,
			},
			validates: true,
		},
		{
			name: "no start time: the event time",
			body: originBody("", temperatureEvent(0, 0, other), temperatureEvent(0, 1, "")),
			want: map[string]string{
				fmt.Sprint(historyOrigin(0)): other,
				fmt.Sprint(eventTime(0, 1)):  other,
			},
		},
		{
			name: "a time that does not parse takes no part",
			body: originBody(originBodyStart,
				temperatureEvent(0, 0, "2026-13-40T99:00:00Z"), temperatureEvent(0, 1, tenUTC), temperatureEvent(0, 2, "")),
			want: map[string]string{
				fmt.Sprint(historyOrigin(0)): tenUTC,
				fmt.Sprint(eventTime(0, 0)):  "2026-13-40T99:00:00Z",
				fmt.Sprint(eventTime(0, 2)):  tenUTC,
			},
		},
		{
			name: "no time at all",
			body: originBody("", temperatureEvent(0, 0, "")),
			want: map[string]string{
				fmt.Sprint(historyOrigin(0)): "",
				fmt.Sprint(eventTime(0, 0)):  "",
			},
		},
	}
	paths := [][]any{
		historyOrigin(0), historyOrigin(1),
		eventTime(0, 0), eventTime(0, 1), eventTime(0, 2), eventTime(1, 0),
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opts := append([]simplified.Option{simplified.WithTemplate(compiled)}, tc.opts...)
			for format, comp := range decodeBothFormats(t, wt, tc.body, opts...) {
				for _, path := range paths {
					want, pinned := tc.want[fmt.Sprint(path)]
					if !pinned {
						continue
					}
					got := memberOf(t, comp, path)
					if want == "" && got != nil && got != "" {
						t.Errorf("%s: %v = %#v, want none", format, path, got)
					}
					if want != "" && got != want {
						t.Errorf("%s: %v = %#v, want %q", format, path, got, want)
					}
				}
				if tc.validates {
					wantValidates(t, format, comp, compiled, true)
				}
			}
		})
	}
}
