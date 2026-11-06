package simplified_test

// REQ-053 / REQ-116 — reused siblings that pinned names tell apart encode to
// FLAT and STRUCTURED, each under its own id, and decode back into the RM
// instance that carries the sibling's name.

import (
	"maps"
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/rmpath"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/simplified"
	"github.com/cadasto/openehr-sdk-go/openehr/template/webtemplate"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// namedEventsTemplate builds the template in testdata whose OBSERVATION holds
// two EVENTs of one node id, at most one each, told apart only by the names
// they pin, 'First event' and 'Second event'. The Web Template lifts both
// events, so their leaves sit side by side under the OBSERVATION: text and
// time for the first, text2, count and time2 for the second.
func namedEventsTemplate(t *testing.T) (*webtemplate.WebTemplate, *templatecompile.Compiled) {
	t.Helper()
	opt, err := readOPTFile("testdata/named_sibling_events.opt")
	if err != nil {
		t.Fatalf("readOPTFile: %v", err)
	}
	return compileTemplate(t, "sdk_named_sibling_events.v1", opt)
}

const (
	namedEventsObs    = "sdk_named_sibling_events/sdk_named_events"
	namedEventsEvents = "/content[openEHR-EHR-OBSERVATION.sdk_named_events.v1]/data[at0001]/events"
)

// namedEvent returns the EVENT of comp named name.
func namedEvent(t *testing.T, comp *rm.Composition, name string) rm.Locatable {
	t.Helper()
	v, err := rmpath.ItemAtPath(comp, namedEventsEvents+"[at0002,'"+name+"']")
	if err != nil {
		t.Fatalf("the event named %q: %v", name, err)
	}
	ev, ok := v.(rm.Locatable)
	if !ok {
		t.Fatalf("the event named %q is a %T", name, v)
	}
	return ev
}

// namedEventItem returns the value of the ELEMENT nodeID in the EVENT of comp
// named name.
func namedEventItem(t *testing.T, comp *rm.Composition, name, nodeID string) any {
	t.Helper()
	v, err := rmpath.ItemAtPath(comp, namedEventsEvents+"[at0002,'"+name+"']/data[at0003]/items["+nodeID+"]/value")
	if err != nil {
		t.Fatalf("the %s value of the event named %q: %v", nodeID, name, err)
	}
	return v
}

// namedEventsComposition is the skeleton the generator writes for the template
// under Large, which fills both events, with distinct texts in the two events
// so a value moved to the other event shows.
func namedEventsComposition(t *testing.T, compiled *templatecompile.Compiled) *rm.Composition {
	t.Helper()
	comp := skeletonOf(t, compiled, instance.Large)
	for name, text := range map[string]string{"First event": "first text", "Second event": "second text"} {
		dv, ok := namedEventItem(t, comp, name, "at0004").(*rm.DVText)
		if !ok {
			t.Fatalf("the text of the event named %q is not a *rm.DVText", name)
		}
		dv.Value = text
	}
	if r := validation.ValidateComposition(comp, compiled); !r.OK {
		t.Fatalf("the edited skeleton is invalid: %v", r.Issues)
	}
	return comp
}

// eventNames returns the names of the OBSERVATION's events, in list order.
func eventNames(t *testing.T, comp *rm.Composition) []string {
	t.Helper()
	items, err := rmpath.ItemsAtPath(comp, namedEventsEvents)
	if err != nil {
		t.Fatalf("events: %v", err)
	}
	names := make([]string, 0, len(items))
	for _, it := range items {
		ev, ok := it.(rm.Locatable)
		if !ok {
			t.Fatalf("event is a %T", it)
		}
		names = append(names, rm.DVTextValueOf(ev.GetName()))
	}
	return names
}

// TestREQ053ReusedSiblingEventsRoundTrip — REQ-053, REQ-116: a composition
// holding both named EVENT siblings encodes, in FLAT and in STRUCTURED, each
// event's values under its own sibling's ids, and decodes, with and without
// the template, back into an event carrying that sibling's name, in template
// order, re-encoding to the same FLAT.
func TestREQ053ReusedSiblingEventsRoundTrip(t *testing.T) {
	wt, compiled := namedEventsTemplate(t)
	comp := namedEventsComposition(t, compiled)

	keys := flatKeys(t, comp, wt)
	for key, want := range map[string]string{
		namedEventsObs + "/text":  "first text",
		namedEventsObs + "/text2": "second text",
	} {
		if got := keys[key]; got != want {
			t.Errorf("MarshalFlat %q = %v, want %q", key, got, want)
		}
	}
	count, ok := namedEventItem(t, comp, "Second event", "at0005").(*rm.DVCount)
	if !ok {
		t.Fatal("the second event's count is not a *rm.DVCount")
	}
	if got, ok := keys[namedEventsObs+"/count"].(float64); !ok || rm.Integer64(got) != count.Magnitude {
		t.Errorf("MarshalFlat %q = %v, want %d", namedEventsObs+"/count", keys[namedEventsObs+"/count"], count.Magnitude)
	}
	for _, key := range []string{namedEventsObs + "/time", namedEventsObs + "/time2"} {
		if _, ok := keys[key]; !ok {
			t.Errorf("MarshalFlat wrote no %q", key)
		}
	}

	for _, format := range []string{"FLAT", "STRUCTURED"} {
		for _, mode := range []struct {
			name string
			opts []simplified.Option
		}{
			{"with the template", []simplified.Option{simplified.WithTemplate(compiled), simplified.WithStrict()}},
			{"without the template", []simplified.Option{simplified.WithStrict()}},
		} {
			t.Run(format+" "+mode.name, func(t *testing.T) {
				back := bmiRoundTrip(t, format, comp, wt, mode.opts...)
				if got, want := eventNames(t, back), []string{"First event", "Second event"}; !slices.Equal(got, want) {
					t.Fatalf("decoded events = %q, want %q", got, want)
				}
				for name, want := range map[string]string{"First event": "first text", "Second event": "second text"} {
					if dv, ok := namedEventItem(t, back, name, "at0004").(*rm.DVText); !ok || dv.Value != want {
						t.Errorf("decoded text of the event named %q = %v, want %q", name, namedEventItem(t, back, name, "at0004"), want)
					}
				}
				if _, err := rmpath.ItemAtPath(back, namedEventsEvents+"[at0002,'First event']/data[at0003]/items[at0005]"); err == nil {
					t.Error("the second event's count was placed in the first event too")
				}
				if len(mode.opts) == 2 {
					for _, is := range validation.ValidateComposition(back, compiled).Issues {
						t.Errorf("decoded composition: %s at %s: %s", is.Code, is.Path, is.Detail)
					}
				}
				if got := flatKeys(t, back, wt); !maps.EqualFunc(got, keys, func(a, b any) bool { return jsonEqual(t, a, b) }) {
					t.Errorf("re-encoded FLAT differs:\n got %v\nwant %v", got, keys)
				}
			})
		}
	}
}

// TestREQ053ReusedSiblingEventsDecodeIntoTheirOwnEvent — REQ-053, REQ-116: a
// FLAT key under one named sibling lands in the event carrying that sibling's
// name, created with it when absent, even where a key of the other sibling
// sorts first; the events then stand in template order.
func TestREQ053ReusedSiblingEventsDecodeIntoTheirOwnEvent(t *testing.T) {
	wt, compiled := namedEventsTemplate(t)
	// count (second event) sorts before text (first event).
	body := mustJSON(t, map[string]any{
		"ctx/language":                                  "en",
		"ctx/territory":                                 "NL",
		"ctx/composer_name":                             "Composer",
		namedEventsObs + "/count":                       7,
		namedEventsObs + "/text":                        "first text",
		namedEventsObs + "/time":                        "2026-10-26T09:00:00Z",
		namedEventsObs + "/time2":                       "2026-10-26T10:00:00Z",
		"sdk_named_sibling_events/category|code":        "433",
		"sdk_named_sibling_events/category|value":       "event",
		"sdk_named_sibling_events/category|terminology": "openehr",
	})
	for _, mode := range []struct {
		name string
		opts []simplified.Option
	}{
		{"with the template", []simplified.Option{simplified.WithTemplate(compiled), simplified.WithStrict()}},
		{"without the template", []simplified.Option{simplified.WithStrict()}},
	} {
		t.Run(mode.name, func(t *testing.T) {
			back, err := simplified.UnmarshalFlat(body, wt, mode.opts...)
			if err != nil {
				t.Fatalf("UnmarshalFlat: %v", err)
			}
			if got, want := eventNames(t, back), []string{"First event", "Second event"}; !slices.Equal(got, want) {
				t.Fatalf("decoded events = %q, want %q", got, want)
			}
			if dv, ok := namedEventItem(t, back, "First event", "at0004").(*rm.DVText); !ok || dv.Value != "first text" {
				t.Errorf("first event's text = %v, want %q", namedEventItem(t, back, "First event", "at0004"), "first text")
			}
			if c, ok := namedEventItem(t, back, "Second event", "at0005").(*rm.DVCount); !ok || c.Magnitude != 7 {
				t.Errorf("second event's count = %v, want 7", namedEventItem(t, back, "Second event", "at0005"))
			}
			if len(mode.opts) == 2 {
				for _, is := range validation.ValidateComposition(back, compiled).Issues {
					t.Errorf("decoded composition: %s at %s: %s", is.Code, is.Path, is.Detail)
				}
			}
		})
	}
}

// jsonEqual reports whether a and b, two values decoded from JSON, are equal.
func jsonEqual(t *testing.T, a, b any) bool {
	t.Helper()
	return string(mustJSON(t, a)) == string(mustJSON(t, b))
}
