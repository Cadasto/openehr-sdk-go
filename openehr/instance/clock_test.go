package instance_test

import (
	"strings"
	"testing"
	"time"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
)

// TestREQ107_ZeroNowIsCurrentTime is the REQ-107 check that a zero
// Options.Now is read as the current time: a date-time the generator takes
// from the clock lies between the time before and after the call, and is
// written in UTC as RFC 3339, which has whole seconds.
//
// The UTC check cannot fail on a host whose local zone is UTC, because the
// current time is then already in UTC. TestREQ107_RootPlaceholders and
// TestREQ107_GeneratedDateTimesInUTC catch a date-time written in the
// zone of Now on any host: they set Now two hours east of UTC.
func TestREQ107_ZeroNowIsCurrentTime(t *testing.T) {
	cases := []struct {
		name     string
		compiled func(t *testing.T) *templatecompile.Compiled
		opts     instance.Options
		clock    func(t *testing.T, out any) string
	}{
		{
			name: "HISTORY.origin",
			compiled: func(t *testing.T) *templatecompile.Compiled {
				return compileOPTText(t, optTemplate("OBSERVATION"), true)
			},
			clock: func(t *testing.T, out any) string {
				obs, err := instance.AsObservation(out)
				if err != nil {
					t.Fatalf("AsObservation: %v", err)
				}
				return obs.Data.Origin.Value
			},
		},
		{
			name: "EVENT_CONTEXT.start_time",
			compiled: func(t *testing.T) *templatecompile.Compiled {
				return compileFixture(t, "vital_signs")
			},
			opts: instance.Options{Territory: "NL", Composer: testComposer()},
			clock: func(t *testing.T, out any) string {
				comp, err := instance.AsComposition(out)
				if err != nil {
					t.Fatalf("AsComposition: %v", err)
				}
				if comp.Context == nil {
					t.Fatal("COMPOSITION.context is nil")
				}
				return comp.Context.StartTime.Value
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := tc.compiled(t)
			opts := tc.opts // Now is left zero.
			// RFC 3339 as the generator writes it drops the fraction of a
			// second, so the lower bound is truncated the same way.
			before := time.Now().UTC().Truncate(time.Second)
			out, err := instance.Generate(t.Context(), c, opts)
			after := time.Now().UTC()
			if err != nil {
				t.Fatalf("Generate: %v", err)
			}
			got := tc.clock(t, out)
			at, err := time.Parse(time.RFC3339, got)
			if err != nil {
				t.Fatalf("%s = %q, want an RFC 3339 date-time: %v", tc.name, got, err)
			}
			if !strings.HasSuffix(got, "Z") || got != at.UTC().Format(time.RFC3339) {
				t.Errorf("%s = %q, want it written in UTC as RFC 3339 (%q)", tc.name, got, at.UTC().Format(time.RFC3339))
			}
			if at.Before(before) || at.After(after) {
				t.Errorf("%s = %q, want the current time, between %s and %s",
					tc.name, got, before.Format(time.RFC3339), after.Format(time.RFC3339Nano))
			}
		})
	}
}
