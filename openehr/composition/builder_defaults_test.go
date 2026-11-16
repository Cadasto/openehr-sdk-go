package composition_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/cadasto/openehr-sdk-go/openehr/composition"
	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/rmpath"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
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

// TestREQ107_SkeletonAndBuilderTakeTheTemplateDefaults checks that
// NewSkeleton and NewBuilder, which seed from Generate, hold the default
// value a template's constraints set on a data value, here the DV_TEXT
// default value and the DV_COUNT 11 of a corpus template, in place of the
// value the fill gives, under both value fills, as the REQ-107 Trust model
// says of the generator they seed from.
func TestREQ107_SkeletonAndBuilderTakeTheTemplateDefaults(t *testing.T) {
	opt, err := readOPTFile(filepath.Join(fixtures.CorpusRoot(), "webtemplate", "constrain_test.opt"))
	if err != nil {
		t.Fatalf("readOPTFile: %v", err)
	}
	c, err := templatecompile.Compile(opt)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	const (
		textPath  = "/content[openEHR-EHR-OBSERVATION.blood_pressure.v2]/data/events[at1042]/data/items[at1059]/value"
		countPath = "/content[openEHR-EHR-OBSERVATION.abbey_pain_scale.v0]/data/events[at0002]/data/items[at0014]/value"
	)
	builders := map[string]func(t *testing.T, opts ...composition.Option) *rm.Composition{
		"NewSkeleton": func(t *testing.T, opts ...composition.Option) *rm.Composition {
			comp, err := composition.NewSkeleton(t.Context(), c, append(opts, composition.WithPolicy(instance.Large))...)
			if err != nil {
				t.Fatalf("NewSkeleton: %v", err)
			}
			return comp
		},
		"NewBuilder": func(t *testing.T, opts ...composition.Option) *rm.Composition {
			b, err := composition.NewBuilder(t.Context(), c, opts...)
			if err != nil {
				t.Fatalf("NewBuilder: %v", err)
			}
			comp, err := b.Build()
			if err != nil {
				t.Fatalf("Build: %v", err)
			}
			return comp
		},
	}
	for name, build := range builders {
		for _, fill := range []instance.ValueFill{instance.ExampleFill, instance.RandomFill} {
			t.Run(name+"/"+fill.String(), func(t *testing.T) {
				comp := build(t, composition.WithTerritory("NL"), composition.WithComposer(testComposer()), composition.WithValueFill(fill))
				texts, err := rmpath.ItemsAtPath(comp, textPath)
				if err != nil || len(texts) == 0 {
					t.Errorf("%s: %d items, %v; want the DV_TEXT default value", textPath, len(texts), err)
				}
				for _, text := range texts {
					if v, ok := text.(*rm.DVText); !ok || v.Value != "default value" {
						t.Errorf("%s = %#v; want the DV_TEXT default value", textPath, text)
					}
				}
				counts, err := rmpath.ItemsAtPath(comp, countPath)
				if err != nil || len(counts) == 0 {
					t.Errorf("%s: %d items, %v; want the DV_COUNT default 11", countPath, len(counts), err)
				}
				for _, count := range counts {
					if v, ok := count.(*rm.DVCount); !ok || v.Magnitude != 11 {
						t.Errorf("%s = %#v; want the DV_COUNT default 11", countPath, count)
					}
				}
			})
		}
	}
}
