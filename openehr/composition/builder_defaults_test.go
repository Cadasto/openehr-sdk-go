package composition_test

import (
	"testing"
	"time"

	"github.com/cadasto/openehr-sdk-go/openehr/composition"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// TestREQ101_REQ107_NewBuilderLanguageEncodingTimeAndOrdinal checks that
// NewBuilder, which calls Generate, honours WithLanguage and WithNow and
// fills an ordinal symbol from the template pair. REQ-101 pins the builder
// options; REQ-107 is the generator contract those options reach.
func TestREQ101_REQ107_NewBuilderLanguageEncodingTimeAndOrdinal(t *testing.T) {
	fixed := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
	c := compileFixture(t, "Test_dv_ordinal_with_constraints.v0")
	b, err := composition.NewBuilder(t.Context(), c,
		composition.WithLanguage("en"),
		composition.WithTerritory("NL"),
		composition.WithComposer(testComposer()),
		composition.WithNow(fixed),
	)
	if err != nil {
		t.Fatalf("NewBuilder: %v", err)
	}
	comp, err := b.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	wantTime := fixed.Format(time.RFC3339)
	if comp.Context == nil {
		t.Fatal("Context is nil")
	}
	if comp.Context.StartTime.Value != wantTime {
		t.Errorf("EventContext.start_time = %q, want %q", comp.Context.StartTime.Value, wantTime)
	}

	obs := firstObservation(t, comp)
	if obs.Language.CodeString != "en" || obs.Language.TerminologyID.Value != "ISO_639-1" {
		t.Errorf("ENTRY.language = %s::%s, want ISO_639-1::en",
			obs.Language.TerminologyID.Value, obs.Language.CodeString)
	}
	if obs.Encoding.CodeString != "UTF-8" || obs.Encoding.TerminologyID.Value != "IANA_character-sets" {
		t.Errorf("ENTRY.encoding = %s::%s, want IANA_character-sets::UTF-8",
			obs.Encoding.TerminologyID.Value, obs.Encoding.CodeString)
	}
	if obs.Data.Origin.Value != wantTime {
		t.Errorf("HISTORY.origin = %q, want %q", obs.Data.Origin.Value, wantTime)
	}
	eventTime := firstEventTime(t, obs)
	if eventTime != wantTime {
		t.Errorf("EVENT.time = %q, want %q", eventTime, wantTime)
	}

	ordinals := ordinalsInObservation(obs)
	if len(ordinals) == 0 {
		t.Fatal("no DV_ORDINAL in built composition")
	}
	got := ordinals[0]
	if got.Value != 1 {
		t.Errorf("ordinal value = %d, want 1", got.Value)
	}
	if got.Symbol.DefiningCode.CodeString != "at0039" || got.Symbol.DefiningCode.TerminologyID.Value != "local" {
		t.Errorf("ordinal symbol = %s::%s, want local::at0039",
			got.Symbol.DefiningCode.TerminologyID.Value, got.Symbol.DefiningCode.CodeString)
	}
}

func firstObservation(t *testing.T, comp *rm.Composition) *rm.Observation {
	t.Helper()
	for _, item := range comp.Content {
		if obs, ok := item.(*rm.Observation); ok {
			return obs
		}
	}
	t.Fatal("no OBSERVATION in composition content")
	return nil
}

func firstEventTime(t *testing.T, obs *rm.Observation) string {
	t.Helper()
	if len(obs.Data.Events) == 0 {
		t.Fatal("observation history has no events")
	}
	switch ev := obs.Data.Events[0].(type) {
	case *rm.PointEvent[rm.ItemStructure]:
		return ev.Time.Value
	case *rm.IntervalEvent[rm.ItemStructure]:
		return ev.Time.Value
	default:
		t.Fatalf("event type %T", obs.Data.Events[0])
		return ""
	}
}

func ordinalsInObservation(obs *rm.Observation) []*rm.DVOrdinal {
	var out []*rm.DVOrdinal
	for _, ev := range obs.Data.Events {
		switch e := ev.(type) {
		case *rm.PointEvent[rm.ItemStructure]:
			collectOrdinals(e.Data, &out)
		case *rm.IntervalEvent[rm.ItemStructure]:
			collectOrdinals(e.Data, &out)
		}
	}
	return out
}

func collectOrdinals(s rm.ItemStructure, out *[]*rm.DVOrdinal) {
	tree, ok := s.(*rm.ItemTree)
	if !ok {
		return
	}
	for _, item := range tree.Items {
		el, ok := item.(*rm.Element)
		if !ok {
			continue
		}
		if o, ok := el.Value.(*rm.DVOrdinal); ok {
			*out = append(*out, o)
		}
	}
}
