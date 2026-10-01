package instance_test

import (
	"math/rand/v2"
	"strings"
	"testing"
	"time"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
)

// openDateTimeBound is a DV_DATE_TIME bound with no constraint, so the
// generator fills it from Options.Now.
const openDateTimeBound = `<children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>DV_DATE_TIME</rm_type_name>` +
	`<occurrences><lower_included>true</lower_included><upper_included>true</upper_included>` +
	`<lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded>` +
	`<lower>1</lower><upper>1</upper></occurrences><node_id></node_id></children>`

// dateTimeBounds returns the two bound values of the one bounded
// DV_INTERVAL<DV_DATE_TIME> in doc.
func dateTimeBounds(t *testing.T, doc any) (lower, upper string) {
	t.Helper()
	found := false
	walkIntervals(doc, "", func(_ string, iv map[string]any) {
		lo, hi, ok := boundedPair(iv)
		if !ok || found {
			return
		}
		lower, _ = lo["value"].(string)
		upper, _ = hi["value"].(string)
		found = true
	})
	if !found {
		t.Fatal("no bounded DV_INTERVAL<DV_DATE_TIME> generated")
	}
	return lower, upper
}

func mustInstant(t *testing.T, s string) time.Time {
	t.Helper()
	at, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("generated date-time %q does not parse as RFC 3339: %v", s, err)
	}
	return at
}

// TestREQ107_DateTimeIntervalOrderedAsInstants is the REQ-107 check that a
// DV_INTERVAL<DV_DATE_TIME> stays in chronological order when one bound
// comes from a clock outside UTC. The lower bound has no constraint, so it
// takes Options.Now; the upper bound is a RandomFill draw in UTC. Now is
// set one hour before the draw, written two hours east of UTC, so its text
// sorts after the draw although its instant comes first.
func TestREQ107_DateTimeIntervalOrderedAsInstants(t *testing.T) {
	count := instance.ReadVendoredOPT(t, instance.CountIntervalOPT)
	stamp := instance.TemporalBound("DV_DATE_TIME", "DATE_TIME", "C_DATE_TIME", "<pattern>yyyy-mm-ddThh:mm:ss</pattern>")
	c := compileSyntheticOPT(t, instance.RetargetInterval(t, count, "DV_DATE_TIME", openDateTimeBound, stamp))
	east := time.FixedZone("UTC+2", 2*60*60)
	for seed := range uint64(5) {
		opts := intervalOptions(instance.Example)
		opts.ValueFill = instance.RandomFill
		opts.Now = time.Date(1990, 1, 1, 0, 0, 0, 0, time.UTC)
		opts.ValueSource = rand.NewPCG(seed, 1)
		_, drawn := dateTimeBounds(t, generateCanonical(t, c, opts))

		opts.Now = mustInstant(t, drawn).Add(-time.Hour).In(east)
		opts.ValueSource = rand.NewPCG(seed, 1)
		doc := generateCanonical(t, c, opts)
		lower, upper := dateTimeBounds(t, doc)
		if mustInstant(t, lower).After(mustInstant(t, upper)) {
			t.Errorf("seed %d: bounds %s and %s are in reverse chronological order (Now %s)", seed, lower, upper, opts.Now.Format(time.RFC3339))
		}
		if !strings.HasSuffix(lower, "Z") {
			t.Errorf("seed %d: lower bound %s from Now %s, want it written in UTC", seed, lower, opts.Now.Format(time.RFC3339))
		}
	}
}

// TestREQ107_GeneratedDateTimesInUTC is the REQ-107 check that every
// date-time the generator takes from Options.Now is written in one layout,
// UTC, whatever zone Now carries.
func TestREQ107_GeneratedDateTimesInUTC(t *testing.T) {
	c := compileFixture(t, "vital_signs")
	opts := intervalOptions(instance.Example)
	opts.Now = time.Date(2026, 10, 1, 23, 0, 0, 0, time.FixedZone("UTC+2", 2*60*60))
	want := opts.Now.UTC().Format(time.RFC3339)
	out, err := instance.Generate(t.Context(), c, opts)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	comp, err := instance.AsComposition(out)
	if err != nil {
		t.Fatalf("AsComposition: %v", err)
	}
	if got := comp.Context.StartTime.Value; got != want {
		t.Errorf("EVENT_CONTEXT.start_time = %q, want %q", got, want)
	}
	_, doc := generateWithJSON(t, c, opts)
	var walk func(v any, path string)
	walk = func(v any, path string) {
		switch x := v.(type) {
		case map[string]any:
			if x["_type"] == "DV_DATE_TIME" {
				if s, _ := x["value"].(string); strings.Contains(s, "+02:00") {
					t.Errorf("%s: DV_DATE_TIME value %q keeps the zone of Now, want UTC", path, s)
				}
			}
			for k, child := range x {
				walk(child, path+"/"+k)
			}
		case []any:
			for _, child := range x {
				walk(child, path+"/*")
			}
		}
	}
	walk(doc, "")
}
