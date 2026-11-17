package simplified_test

// REQ-053, issue 452 — an event the Web Template lifts, any subtype at max 1
// that is its HISTORY's only event (REQ-106), spells no segment of its own:
// its leaves are keyed under its OBSERVATION, and an INTERVAL_EVENT's
// sample_count is a suffix on the OBSERVATION's node, as for a folded abstract
// EVENT (issue 456). Decode rebuilds the event with the RM type the template
// gives it, its time and, for an INTERVAL_EVENT, its width, math_function and
// sample_count; encode writes them back under the same keys.

import (
	"bytes"
	"errors"
	"maps"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/rmpath"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/simplified"
	"github.com/cadasto/openehr-sdk-go/openehr/template/webtemplate"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

const (
	// labObs is the FLAT path of the laboratory test report's OBSERVATION,
	// whose one event the Web Template lifts.
	labObs = "laboratory_test_report/laboratory_test"
	// labTestName is the path of the test name the event holds.
	labTestName = "/content[openEHR-EHR-OBSERVATION.laboratory_test.v0]/data[at0001]/events[at0002]/data[at0003]/items[at0005]/value"
)

// labHistories are the OBSERVATION attributes the laboratory test report's
// HISTORY is tried under: its own `data`, and `state`, an OBSERVATION's other
// HISTORY, whose only event at max 1 the Web Template lifts as it lifts
// data's.
var labHistories = []string{"data", "state"}

// labReportTarget builds the Web Template and the compiled template of the
// corpus laboratory test report, whose OBSERVATION's one event is a
// POINT_EVENT at 0..1, with that event's RM type replaced by eventType.
func labReportTarget(t *testing.T, eventType string) (*webtemplate.WebTemplate, *templatecompile.Compiled) {
	t.Helper()
	return labReportTargetIn(t, eventType, "data")
}

// labReportTargetIn is [labReportTarget] with the OBSERVATION's HISTORY held
// by its attribute history, `data` as the template has it or `state`.
func labReportTargetIn(t *testing.T, eventType, history string) (*webtemplate.WebTemplate, *templatecompile.Compiled) {
	t.Helper()
	raw, err := os.ReadFile(fixtures.TemplateOpt("IDCR - Laboratory Test Report.v0"))
	if err != nil {
		t.Fatal(err)
	}
	point := []byte("<rm_type_name>POINT_EVENT</rm_type_name>")
	if n := bytes.Count(raw, point); n != 1 {
		t.Fatalf("the laboratory test report names POINT_EVENT %d times, want once — fixture changed?", n)
	}
	raw = bytes.Replace(raw, point, []byte("<rm_type_name>"+eventType+"</rm_type_name>"), 1)
	if history != "data" {
		// The OBSERVATION's own `data` is the first attribute of that name
		// after its type name.
		data := []byte("<rm_attribute_name>data</rm_attribute_name>")
		obs := bytes.Index(raw, []byte("<rm_type_name>OBSERVATION</rm_type_name>"))
		at := bytes.Index(raw[max(obs, 0):], data)
		if obs < 0 || at < 0 {
			t.Fatal("the laboratory test report has no OBSERVATION data attribute — fixture changed?")
		}
		at += obs
		raw = slices.Concat(raw[:at], []byte("<rm_attribute_name>"+history+"</rm_attribute_name>"), raw[at+len(data):])
	}
	opt, err := fixtures.ReadOperationalTemplate(raw)
	if err != nil {
		t.Fatalf("ReadOperationalTemplate: %v", err)
	}
	return compileTemplate(t, "IDCR - Laboratory Test Report.v0", opt)
}

// labBody is a body over the laboratory test report keyed as the reference
// keys it: the context, the test name and the event's time, plus extra.
func labBody(extra map[string]any) map[string]any {
	body := map[string]any{
		"ctx/language":        "en",
		"ctx/territory":       "GB",
		"ctx/composer_name":   "Lab",
		"ctx/time":            "2026-11-16T10:00:00Z",
		labObs + "/test_name": "Full blood count",
		labObs + "/time":      "2026-11-16T09:30:00Z",
	}
	maps.Copy(body, extra)
	return body
}

// labIntervalKeys are the keys of a lifted INTERVAL_EVENT's width,
// math_function and sample_count.
func labIntervalKeys() map[string]any {
	return map[string]any{
		labObs + "/width":                     "PT1H",
		labObs + "/math_function|code":        "146",
		labObs + "/math_function|value":       "mean",
		labObs + "/math_function|terminology": "openehr",
		labObs + "|sample_count":              4.0, // a JSON number reads back as float64
	}
}

// labTestNameOf returns the test name comp holds.
func labTestNameOf(t *testing.T, comp *rm.Composition) string {
	t.Helper()
	v, err := rmpath.ItemAtPath(comp, labTestName)
	if err != nil {
		t.Fatalf("ItemAtPath(test name): %v", err)
	}
	text, ok := v.(*rm.DVText)
	if !ok {
		t.Fatalf("test name is a %T, want *rm.DVText", v)
	}
	return text.Value
}

// labEvent returns the one event comp's laboratory test OBSERVATION holds
// in its HISTORY at history, `data` or `state`.
func labEvent(t *testing.T, wt *webtemplate.WebTemplate, comp *rm.Composition, history string) rm.Event {
	t.Helper()
	if history == "data" {
		return onlyEvent(t, wt, comp, "laboratory_test")
	}
	obs := observationOf(t, wt, comp, "laboratory_test")
	if obs.State == nil || len(obs.State.Events) != 1 {
		t.Fatalf("the OBSERVATION's state holds no single event: %+v", obs.State)
	}
	return obs.State.Events[0]
}

// TestREQ053LiftedEventKeysDecode — REQ-053, issue 452. A strict decode of
// the reference's keys for the laboratory test report, FLAT and STRUCTURED,
// with and without the template, holds every key and rebuilds the event as
// the POINT_EVENT the template gives, with its time and its test name.
func TestREQ053LiftedEventKeysDecode(t *testing.T) {
	wt, compiled := labReportTarget(t, "POINT_EVENT")
	body := mustJSON(t, labBody(nil))
	for _, mode := range shorthandModes(compiled) {
		for _, format := range []string{"FLAT", "STRUCTURED"} {
			t.Run(mode.name+"/"+format, func(t *testing.T) {
				opts := append([]simplified.Option{simplified.WithStrict()}, mode.opts...)
				comp := decodeAs(t, format, body, wt, opts...)
				got := onlyEvent(t, wt, comp, "laboratory_test")
				ev, ok := got.(*rm.PointEvent[rm.ItemStructure])
				if !ok {
					t.Fatalf("event is a %T, want *rm.PointEvent[rm.ItemStructure]", got)
				}
				if ev.Time.Value != "2026-11-16T09:30:00Z" {
					t.Errorf("time = %q, want the body's", ev.Time.Value)
				}
				if got := labTestNameOf(t, comp); got != "Full blood count" {
					t.Errorf("test name = %q, want the body's", got)
				}
			})
		}
	}
}

// TestREQ053LiftedEventRoundTrips — REQ-053, issue 452. Encode writes a lifted
// POINT_EVENT's and a lifted INTERVAL_EVENT's leaves under the OBSERVATION,
// with no segment for the event, the INTERVAL_EVENT's width and math_function
// once, at their Web Template leaves, and a strict decode of the FLAT and the
// STRUCTURED it writes, with and without the template, gives back the event's
// RM type, time, width, math_function and sample_count. The event is lifted
// out of the OBSERVATION's `data` HISTORY or out of its `state` one.
func TestREQ053LiftedEventRoundTrips(t *testing.T) {
	for _, history := range labHistories {
		for _, tc := range []struct {
			eventType string
			extra     map[string]any
		}{
			{eventType: "POINT_EVENT"},
			{eventType: "INTERVAL_EVENT", extra: labIntervalKeys()},
		} {
			t.Run(history+"/"+tc.eventType, func(t *testing.T) {
				testLiftedEventRoundTrips(t, history, tc.eventType, tc.extra)
			})
		}
	}
}

// testLiftedEventRoundTrips is one case of [TestREQ053LiftedEventRoundTrips]:
// the event of eventType lifted out of the HISTORY at history, its body's
// keys labBody(extra).
func testLiftedEventRoundTrips(t *testing.T, history, eventType string, extra map[string]any) {
	t.Helper()
	wt, compiled := labReportTargetIn(t, eventType, history)
	comp := decodeWith(t, wt, labBody(extra), simplified.WithStrict(), simplified.WithTemplate(compiled))
	assertLabEvent(t, "UnmarshalFlat", eventType, labEvent(t, wt, comp, history))
	flat := flatOf(t, comp, wt)
	for key, v := range labBody(extra) {
		if strings.HasPrefix(key, "ctx/") {
			continue
		}
		if flat[key] != v {
			t.Errorf("MarshalFlat wrote %q = %#v, want %#v", key, flat[key], v)
		}
	}
	for key := range flat {
		if strings.Contains(key, "any_event") {
			t.Errorf("MarshalFlat wrote %q, a segment for the lifted event", key)
		}
	}
	for _, mode := range shorthandModes(compiled) {
		opts := append([]simplified.Option{simplified.WithStrict()}, mode.opts...)
		for _, format := range []string{"FLAT", "STRUCTURED"} {
			back := decodeAs(t, format, mustJSON(t, flat), wt, opts...)
			assertLabEvent(t, mode.name+"/"+format, eventType, labEvent(t, wt, back, history))
		}
	}
	assertLabEvent(t, "MarshalStructured", eventType, labEvent(t, wt, structuredRoundTrip(t, comp, wt), history))
}

// assertLabEvent checks that ev, decoded in the way named how, is an event of
// eventType with the time labBody gives it and, as an INTERVAL_EVENT, the
// width, math_function and sample_count labIntervalKeys give it.
func assertLabEvent(t *testing.T, how, eventType string, ev rm.Event) {
	t.Helper()
	if got := eventTimeOf(t, ev); got != "2026-11-16T09:30:00Z" {
		t.Errorf("%s: time = %q, want 2026-11-16T09:30:00Z", how, got)
	}
	if eventType != "INTERVAL_EVENT" {
		if _, ok := ev.(*rm.PointEvent[rm.ItemStructure]); !ok {
			t.Errorf("%s: event is a %T, want *rm.PointEvent[rm.ItemStructure]", how, ev)
		}
		return
	}
	iv, ok := ev.(*rm.IntervalEvent[rm.ItemStructure])
	if !ok {
		t.Errorf("%s: event is a %T, want *rm.IntervalEvent[rm.ItemStructure]", how, ev)
		return
	}
	if iv.Width.Value != "PT1H" {
		t.Errorf("%s: width = %q, want PT1H", how, iv.Width.Value)
	}
	if mf := iv.MathFunction; mf.Value != "mean" || mf.DefiningCode.CodeString != "146" {
		t.Errorf("%s: math_function = %+v, want 146|mean", how, mf)
	}
	if iv.SampleCount == nil || *iv.SampleCount != 4 {
		t.Errorf("%s: sample_count = %v, want 4", how, iv.SampleCount)
	}
}

// TestREQ053LiftedIntervalEventKeepsItsType — REQ-053, issue 452. A lifted
// INTERVAL_EVENT whose body spells neither its width nor its math_function
// still decodes as the INTERVAL_EVENT the template gives, without the
// template and with it; with it, the two take their defaults, the openEHR
// 640|actual and PT0S. The event is lifted out of the OBSERVATION's `data`
// HISTORY or out of its `state` one.
func TestREQ053LiftedIntervalEventKeepsItsType(t *testing.T) {
	for _, history := range labHistories {
		wt, compiled := labReportTargetIn(t, "INTERVAL_EVENT", history)
		for _, mode := range shorthandModes(compiled) {
			for _, format := range []string{"FLAT", "STRUCTURED"} {
				t.Run(history+"/"+mode.name+"/"+format, func(t *testing.T) {
					opts := append([]simplified.Option{simplified.WithStrict()}, mode.opts...)
					comp := decodeAs(t, format, mustJSON(t, labBody(nil)), wt, opts...)
					iv := intervalEvent(t, labEvent(t, wt, comp, history))
					if mode.opts == nil {
						return
					}
					if iv.MathFunction.DefiningCode.CodeString != "640" || iv.Width.Value != "PT0S" {
						t.Errorf("math_function = %+v, width = %q; want the 640|actual and PT0S defaults", iv.MathFunction, iv.Width.Value)
					}
				})
			}
		}
	}
}

// labIntervalAttrs are the keys of each of a lifted event's three
// INTERVAL_EVENT attributes, by attribute.
func labIntervalAttrs() map[string]map[string]any {
	return map[string]map[string]any{
		"width": {labObs + "/width": "PT1H"},
		"math_function": {
			labObs + "/math_function|code":        "146",
			labObs + "/math_function|value":       "mean",
			labObs + "/math_function|terminology": "openehr",
		},
		"sample_count": {labObs + "|sample_count": 4},
	}
}

// TestREQ053LiftedPointEventReadsNoIntervalAttributes — REQ-053, issue 452.
// The laboratory test report's template types its lifted event a POINT_EVENT,
// which has no width, math_function or sample_count, and the reference gives
// those keys no node there. With the template, a strict decode, FLAT and
// STRUCTURED, refuses `<observation>/width`, `/math_function` and
// `|sample_count` as unmatched content; the default decode reports each
// unmatched, drops it and keeps the POINT_EVENT. Without the template a
// folded POINT_EVENT cannot be told from a folded abstract EVENT, so a strict
// decode reads each and builds an INTERVAL_EVENT. The event is lifted out of
// the OBSERVATION's `data` HISTORY or out of its `state` one.
func TestREQ053LiftedPointEventReadsNoIntervalAttributes(t *testing.T) {
	for _, history := range labHistories {
		testLiftedPointEventReadsNoIntervalAttributes(t, history)
	}
}

// testLiftedPointEventReadsNoIntervalAttributes is
// [TestREQ053LiftedPointEventReadsNoIntervalAttributes] for the event lifted
// out of the HISTORY at history.
func testLiftedPointEventReadsNoIntervalAttributes(t *testing.T, history string) {
	t.Helper()
	wt, compiled := labReportTargetIn(t, "POINT_EVENT", history)
	for _, attr := range slices.Sorted(maps.Keys(labIntervalAttrs())) {
		keys := labIntervalAttrs()[attr]
		for _, format := range cdrFormats {
			t.Run(history+"/"+attr+"/"+format, func(t *testing.T) {
				_, err := decodeFormat(t, format, wt, labBody(keys), simplified.WithStrict(), simplified.WithTemplate(compiled))
				if !errors.Is(err, simplified.ErrUnknownPath) || !strings.Contains(err.Error(), "unmatched content") {
					t.Errorf("strict decode with the template = %v, want it refused as unmatched content", err)
				}

				var unmatched []string
				comp, err := decodeFormat(t, format, wt, labBody(keys), simplified.WithTemplate(compiled),
					simplified.WithUnmatched(func(path string) { unmatched = append(unmatched, path) }))
				if err != nil {
					t.Fatalf("decode with the template = %v", err)
				}
				if ev := labEvent(t, wt, comp, history); !isPointEvent(ev) {
					t.Errorf("decode with the template: event is a %T, want *rm.PointEvent[rm.ItemStructure]", ev)
				}
				if len(unmatched) == 0 {
					t.Errorf("decode with the template reported nothing unmatched, want the %s keys", attr)
				}
				for _, path := range unmatched {
					if !strings.Contains(path, attr) {
						t.Errorf("decode with the template reported %q unmatched, want only the %s keys", path, attr)
					}
				}
				if format == "FLAT" && !slices.Equal(slices.Sorted(slices.Values(unmatched)), slices.Sorted(maps.Keys(keys))) {
					t.Errorf("decode with the template reported %q unmatched, want %q", unmatched, slices.Sorted(maps.Keys(keys)))
				}

				comp, err = decodeFormat(t, format, wt, labBody(keys), simplified.WithStrict())
				if err != nil {
					t.Fatalf("strict decode without the template = %v", err)
				}
				intervalEvent(t, labEvent(t, wt, comp, history))
			})
		}
	}
}

// isPointEvent reports whether ev is a POINT_EVENT.
func isPointEvent(ev rm.Event) bool {
	_, ok := ev.(*rm.PointEvent[rm.ItemStructure])
	return ok
}
