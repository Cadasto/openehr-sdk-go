package simplified_test

// REQ-053 — the INTERVAL_EVENT attributes the Simplified Formats spell where
// the Web Template gives them no node (issue 424): `<event>|sample_count` on
// any event node, and `<event>/math_function` and `<event>/width` on an
// abstract EVENT node, which then decodes as an INTERVAL_EVENT. ITS-REST 1.1.0
// Simplified Formats § INTERVAL_EVENT lists all three; the reference gives
// none of them a Web Template node.

import (
	"encoding/json"
	"errors"
	"maps"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/rmpath"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/simplified"
	"github.com/cadasto/openehr-sdk-go/openehr/template/webtemplate"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// The PROBE-086 corpus template's OBSERVATION and its event, an abstract EVENT
// node in the Web Template.
const (
	eventObs = "conformance-ehrbase.de.v0/conformance_section/conformance_observation"
	eventAt  = eventObs + "/any_event:0"
)

// eventBody is a decodable body over the corpus template: the mandatory
// context, one quantity under the event and the event's time, plus extra.
func eventBody(extra map[string]any) map[string]any {
	body := map[string]any{
		"ctx/language":                     "en",
		"ctx/territory":                    "US",
		"ctx/composer_self":                true,
		"ctx/time":                         "2021-12-21T14:19:31.649613+01:00",
		eventAt + "/dv_quantity|magnitude": 65.9,
		eventAt + "/dv_quantity|unit":      "unit",
		eventAt + "/time":                  "2021-12-21T16:02:58.0094262+01:00",
	}
	maps.Copy(body, extra)
	return body
}

// intervalKeys are the corpus `interval_event` fixture's keys for the three
// attributes, with the values the specification's example gives.
func intervalKeys() map[string]any {
	return map[string]any{
		eventAt + "|sample_count":              5,
		eventAt + "/width":                     "P30D",
		eventAt + "/math_function|code":        "146",
		eventAt + "/math_function|value":       "mean",
		eventAt + "/math_function|terminology": "openehr",
	}
}

// observationOf returns the one OBSERVATION comp holds at the Web Template
// node of id obsID.
func observationOf(t *testing.T, wt *webtemplate.WebTemplate, comp *rm.Composition, obsID string) *rm.Observation {
	t.Helper()
	var path string
	var walk func(n *webtemplate.Node)
	walk = func(n *webtemplate.Node) {
		if n.ID == obsID && n.RMType == "OBSERVATION" {
			path = n.AQLPath
		}
		for _, ch := range n.Children {
			walk(ch)
		}
	}
	walk(wt.Tree)
	if path == "" {
		t.Fatalf("the Web Template has no OBSERVATION %q — fixture changed?", obsID)
	}
	items, err := rmpath.ItemsAtPath(comp, path)
	if err != nil || len(items) != 1 {
		t.Fatalf("ItemsAtPath(%s) = %d items, %v; want the OBSERVATION", path, len(items), err)
	}
	obs, ok := items[0].(*rm.Observation)
	if !ok {
		t.Fatalf("%s holds a %T, want *rm.Observation", path, items[0])
	}
	return obs
}

// onlyEvent returns the one event the OBSERVATION obsID holds.
func onlyEvent(t *testing.T, wt *webtemplate.WebTemplate, comp *rm.Composition, obsID string) rm.Event {
	t.Helper()
	events := observationOf(t, wt, comp, obsID).Data.Events
	if len(events) != 1 {
		t.Fatalf("OBSERVATION %s holds %d events, want 1", obsID, len(events))
	}
	return events[0]
}

// intervalEvent returns ev as an INTERVAL_EVENT, failing the test when it is
// another type.
func intervalEvent(t *testing.T, ev rm.Event) *rm.IntervalEvent[rm.ItemStructure] {
	t.Helper()
	iv, ok := ev.(*rm.IntervalEvent[rm.ItemStructure])
	if !ok {
		t.Fatalf("event is a %T, want *rm.IntervalEvent[rm.ItemStructure]", ev)
	}
	return iv
}

// eventTimeOf returns the time of ev, a POINT_EVENT or an INTERVAL_EVENT.
func eventTimeOf(t *testing.T, ev rm.Event) string {
	t.Helper()
	switch e := ev.(type) {
	case *rm.PointEvent[rm.ItemStructure]:
		return e.Time.Value
	case *rm.IntervalEvent[rm.ItemStructure]:
		return e.Time.Value
	}
	t.Fatalf("event is a %T, want a POINT_EVENT or an INTERVAL_EVENT", ev)
	return ""
}

// flatOf encodes comp as FLAT and returns the map it writes.
func flatOf(t *testing.T, comp *rm.Composition, wt *webtemplate.WebTemplate) map[string]any {
	t.Helper()
	b, err := simplified.MarshalFlat(comp, wt)
	if err != nil {
		t.Fatalf("MarshalFlat: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

// structuredAsFlat encodes comp as STRUCTURED and returns the FLAT map the
// OPT-free conversion gives that document.
func structuredAsFlat(t *testing.T, comp *rm.Composition, wt *webtemplate.WebTemplate) map[string]any {
	t.Helper()
	b, err := simplified.MarshalStructured(comp, wt)
	if err != nil {
		t.Fatalf("MarshalStructured: %v", err)
	}
	fb, err := simplified.StructuredToFlat(b)
	if err != nil {
		t.Fatalf("StructuredToFlat: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(fb, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

// TestREQ053AbstractEventDecodesIntervalAttributes — REQ-053, issue 424. The
// corpus `interval_event` body's three attributes decode under the abstract
// EVENT node, with and without the template, in FLAT and in STRUCTURED, and
// the event they are spelled on is an INTERVAL_EVENT carrying them. Under
// strict, nothing in the body is unmatched.
func TestREQ053AbstractEventDecodesIntervalAttributes(t *testing.T) {
	wt, compiled := conformanceTarget(t)
	body := mustJSON(t, eventBody(intervalKeys()))
	for _, mode := range shorthandModes(compiled) {
		for _, format := range []string{"FLAT", "STRUCTURED"} {
			t.Run(mode.name+"/"+format, func(t *testing.T) {
				opts := append([]simplified.Option{simplified.WithStrict()}, mode.opts...)
				comp := decodeAs(t, format, body, wt, opts...)
				ev := intervalEvent(t, onlyEvent(t, wt, comp, "conformance_observation"))
				if ev.SampleCount == nil || *ev.SampleCount != 5 {
					t.Errorf("sample_count = %v, want 5", ev.SampleCount)
				}
				if ev.Width.Value != "P30D" {
					t.Errorf("width = %q, want P30D", ev.Width.Value)
				}
				mf := ev.MathFunction
				if mf.Value != "mean" || mf.DefiningCode.CodeString != "146" || mf.DefiningCode.TerminologyID.Value != "openehr" {
					t.Errorf("math_function = %+v, want openehr::146|mean", mf)
				}
				if ev.Time.Value != "2021-12-21T16:02:58.0094262+01:00" {
					t.Errorf("time = %q, want the body's", ev.Time.Value)
				}
			})
		}
	}
}

// TestREQ053OneIntervalAttributeMakesAnIntervalEvent — REQ-053. Each of the
// three attributes alone makes the abstract EVENT it is spelled on an
// INTERVAL_EVENT. With the template, the RM-mandatory math_function and width
// the body leaves out take their defaults, as on an INTERVAL_EVENT node: the
// openEHR 640|actual and PT0S. A body that spells none of them, or holds null
// at each, decodes a POINT_EVENT.
func TestREQ053OneIntervalAttributeMakesAnIntervalEvent(t *testing.T) {
	wt, compiled := conformanceTarget(t)
	for _, tc := range []struct {
		name string
		keys map[string]any
	}{
		{"sample_count", map[string]any{eventAt + "|sample_count": 3}},
		{"width", map[string]any{eventAt + "/width": "PT1H"}},
		{"math_function", map[string]any{
			eventAt + "/math_function|code": "146", eventAt + "/math_function|value": "mean",
			eventAt + "/math_function|terminology": "openehr",
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bare := decodeWith(t, wt, eventBody(tc.keys))
			intervalEvent(t, onlyEvent(t, wt, bare, "conformance_observation"))

			ev := intervalEvent(t, onlyEvent(t, wt, decodeWith(t, wt, eventBody(tc.keys), simplified.WithTemplate(compiled)), "conformance_observation"))
			if _, spelled := tc.keys[eventAt+"/math_function|code"]; !spelled && ev.MathFunction.DefiningCode.CodeString != "640" {
				t.Errorf("math_function = %+v, want the 640|actual default", ev.MathFunction)
			}
			if _, spelled := tc.keys[eventAt+"/width"]; !spelled && ev.Width.Value != "PT0S" {
				t.Errorf("width = %q, want the PT0S default", ev.Width.Value)
			}
		})
	}
	nulls := map[string]any{
		eventAt + "|sample_count": nil, eventAt + "/width": nil,
		eventAt + "/math_function|code": nil, eventAt + "/math_function|value": nil,
	}
	for name, keys := range map[string]map[string]any{"none": nil, "all null": nulls} {
		t.Run(name, func(t *testing.T) {
			ev := onlyEvent(t, wt, decodeWith(t, wt, eventBody(keys), simplified.WithTemplate(compiled)), "conformance_observation")
			if _, ok := ev.(*rm.PointEvent[rm.ItemStructure]); !ok {
				t.Errorf("event is a %T, want *rm.PointEvent[rm.ItemStructure]", ev)
			}
		})
	}
}

// TestREQ053SampleCountMustFitAnInteger — REQ-053 "A value must fit its
// attribute". `|sample_count` holds an RM Integer: a whole number, bare or
// quoted, decodes; a fraction, a word or a Boolean is refused with
// ErrUnsupportedDatatype naming the key and not its value.
func TestREQ053SampleCountMustFitAnInteger(t *testing.T) {
	wt, _ := conformanceTarget(t)
	key := eventAt + "|sample_count"
	for _, v := range []any{"7", 7.0} {
		ev := intervalEvent(t, onlyEvent(t, wt, decodeWith(t, wt, eventBody(map[string]any{key: v})), "conformance_observation"))
		if ev.SampleCount == nil || *ev.SampleCount != 7 {
			t.Errorf("|sample_count %#v: sample_count = %v, want 7", v, ev.SampleCount)
		}
	}
	for _, v := range []any{7.5, "seven-0451", true, 3000000000} {
		_, err := simplified.UnmarshalFlat(mustJSON(t, eventBody(map[string]any{key: v})), wt)
		if !errors.Is(err, simplified.ErrUnsupportedDatatype) || !strings.Contains(err.Error(), key) {
			t.Errorf("|sample_count %#v: err = %v, want ErrUnsupportedDatatype naming %q", v, err, key)
		}
		if err != nil && strings.Contains(err.Error(), "0451") {
			t.Errorf("|sample_count %#v: err = %v carries the value", v, err)
		}
	}
}

// TestREQ053AbstractEventEncodesIntervalAttributes — REQ-053, issue 424.
// Encode writes an INTERVAL_EVENT under an abstract EVENT node with the keys
// decode reads, in FLAT and in STRUCTURED, `|sample_count` only where it is
// set, and decode reads the result back to the same event. A POINT_EVENT
// writes none of them.
func TestREQ053AbstractEventEncodesIntervalAttributes(t *testing.T) {
	wt, compiled := conformanceTarget(t)
	comp := decodeWith(t, wt, eventBody(intervalKeys()), simplified.WithTemplate(compiled))
	want := intervalKeys()
	want[eventAt+"|sample_count"] = 5.0 // a JSON number reads back as float64
	for name, got := range map[string]map[string]any{
		"FLAT":       flatOf(t, comp, wt),
		"STRUCTURED": structuredAsFlat(t, comp, wt),
	} {
		for key, v := range want {
			if got[key] != v {
				t.Errorf("%s wrote %q = %#v, want %#v", name, key, got[key], v)
			}
		}
	}
	back := intervalEvent(t, onlyEvent(t, wt, decodeAs(t, "FLAT", mustJSON(t, flatOf(t, comp, wt)), wt), "conformance_observation"))
	if back.SampleCount == nil || *back.SampleCount != 5 || back.Width.Value != "P30D" || back.MathFunction.DefiningCode.CodeString != "146" {
		t.Errorf("decode of the encoded body: event = %+v, want sample_count 5, width P30D, math_function 146", back)
	}

	widthOnly := decodeWith(t, wt, eventBody(map[string]any{eventAt + "/width": "PT1H"}))
	if v, written := flatOf(t, widthOnly, wt)[eventAt+"|sample_count"]; written {
		t.Errorf("an INTERVAL_EVENT with no sample_count wrote |sample_count = %#v", v)
	}

	point := decodeWith(t, wt, eventBody(nil), simplified.WithTemplate(compiled))
	for key := range flatOf(t, point, wt) {
		if strings.HasPrefix(key, eventAt+"|sample_count") || strings.HasPrefix(key, eventAt+"/math_function") || strings.HasPrefix(key, eventAt+"/width") {
			t.Errorf("a POINT_EVENT wrote %q", key)
		}
	}
}

// TestREQ053IntervalEventNodeCarriesSampleCount — REQ-053, issue 424. Under an
// INTERVAL_EVENT node, whose Web Template has math_function and width but no
// sample_count, `|sample_count` decodes and encode writes it back.
func TestREQ053IntervalEventNodeCarriesSampleCount(t *testing.T) {
	opt, err := readOPTFile(fixtures.WebTemplateOpt("constrain_test"))
	if err != nil {
		t.Fatalf("readOPTFile(constrain_test): %v", err)
	}
	wt, compiled := compileTemplate(t, "constrain_test", opt)
	const event = "constrain/blood_pressure:0/a24_hour_average:0"
	body := map[string]any{
		"ctx/language":                 "en",
		"ctx/territory":                "US",
		"ctx/composer_self":            true,
		"ctx/time":                     "2021-12-21T14:19:31+01:00",
		event + "/systolic|magnitude":  120,
		event + "/systolic|unit":       "mm[Hg]",
		event + "/time":                "2021-12-21T16:00:00+01:00",
		event + "/width":               "PT24H",
		event + "/math_function|code":  "146",
		event + "/math_function|value": "mean",
		event + "|sample_count":        24,
	}
	for _, mode := range shorthandModes(compiled) {
		for _, format := range []string{"FLAT", "STRUCTURED"} {
			t.Run(mode.name+"/"+format, func(t *testing.T) {
				opts := append([]simplified.Option{simplified.WithStrict()}, mode.opts...)
				comp := decodeAs(t, format, mustJSON(t, body), wt, opts...)
				ev := intervalEvent(t, onlyEvent(t, wt, comp, "blood_pressure"))
				if ev.SampleCount == nil || *ev.SampleCount != 24 {
					t.Errorf("sample_count = %v, want 24", ev.SampleCount)
				}
				if got := flatOf(t, comp, wt)[event+"|sample_count"]; got != 24.0 {
					t.Errorf("MarshalFlat wrote |sample_count = %#v, want 24", got)
				}
			})
		}
	}
}
