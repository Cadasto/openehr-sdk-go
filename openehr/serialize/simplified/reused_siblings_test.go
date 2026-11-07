package simplified_test

// REQ-053 / REQ-116 — reused siblings that pinned names tell apart encode to
// FLAT and STRUCTURED, each under its own id, and decode back into the RM
// instance that carries the sibling's name.

import (
	"maps"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aom/aom14"
	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/rmpath"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/simplified"
	"github.com/cadasto/openehr-sdk-go/openehr/template/webtemplate"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
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
		"ctx/time":                                      "2026-10-26T09:00:00Z",
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

// TestREQ053ReusedSiblingItemNamesDecodeWithTemplate — REQ-053, REQ-116: below
// each of two named EVENT siblings, the ITEM_TREE at0003 and the ELEMENT at0004
// pin names of their own: 'Tree A' and 'Text A' in the first event, 'Tree B'
// and 'Text B' in the second. Written as FLAT or STRUCTURED and decoded with
// the template, each takes the name its own event's template node pins, not
// the rubric the two events share, and the composition validates against the
// template. So does an ELEMENT that holds only a null flavour, which decode
// builds from its underscore attribute (REQ-140), and a tree that holds no
// item, which no key spells and decode rebuilds from the template.
func TestREQ053ReusedSiblingItemNamesDecodeWithTemplate(t *testing.T) {
	opt, err := readOPTFile("testdata/named_sibling_items.opt")
	if err != nil {
		t.Fatalf("readOPTFile: %v", err)
	}
	wt, compiled := compileTemplate(t, "sdk_named_sibling_items.v1", opt)
	const tree, element = "/data[at0003]", "/data[at0003]/items[at0004]"
	edited := func(edit func(t *testing.T, event string, tree *rm.ItemTree)) *rm.Composition {
		comp := skeletonOf(t, compiled, instance.Large)
		for _, event := range []string{"First event", "Second event"} {
			item, err := rmpath.ItemAtPath(comp, namedEventsEvents+"[at0002,'"+event+"']"+tree)
			if err != nil {
				t.Fatalf("the ITEM_TREE of the event named %q: %v", event, err)
			}
			it, ok := item.(*rm.ItemTree)
			if !ok {
				t.Fatalf("the ITEM_TREE of the event named %q is a %T", event, item)
			}
			edit(t, event, it)
		}
		if r := validation.ValidateComposition(comp, compiled); !r.OK {
			t.Fatalf("the edited skeleton is invalid: %v", r.Issues)
		}
		return comp
	}
	for _, tc := range []struct {
		name string
		comp *rm.Composition
		// want maps each event to the names its nodes take, by path below it.
		want map[string]map[string]string
	}{
		{
			name: "value",
			comp: edited(func(*testing.T, string, *rm.ItemTree) {}),
			want: map[string]map[string]string{
				"First event":  {tree: "Tree A", element: "Text A"},
				"Second event": {tree: "Tree B", element: "Text B"},
			},
		},
		{
			name: "null flavour",
			comp: edited(func(t *testing.T, event string, it *rm.ItemTree) {
				for _, item := range it.Items {
					el, ok := item.(*rm.Element)
					if !ok || el.ArchetypeNodeID != "at0004" {
						continue
					}
					el.Value = nil
					el.NullFlavour = &rm.DVCodedText{
						Value:        "unknown",
						DefiningCode: rm.CodePhrase{CodeString: "253", TerminologyID: rm.TerminologyID{Value: "openehr"}},
					}
					return
				}
				t.Fatalf("the event named %q holds no ELEMENT at0004", event)
			}),
			want: map[string]map[string]string{
				"First event":  {tree: "Tree A", element: "Text A"},
				"Second event": {tree: "Tree B", element: "Text B"},
			},
		},
		{
			name: "empty tree",
			comp: edited(func(_ *testing.T, _ string, it *rm.ItemTree) { it.Items = nil }),
			want: map[string]map[string]string{
				"First event":  {tree: "Tree A"},
				"Second event": {tree: "Tree B"},
			},
		},
	} {
		for _, format := range []string{"FLAT", "STRUCTURED"} {
			t.Run(tc.name+" "+format, func(t *testing.T) {
				back := bmiRoundTrip(t, format, tc.comp, wt, simplified.WithTemplate(compiled), simplified.WithStrict())
				for event, names := range tc.want {
					for rel, want := range names {
						item, err := rmpath.ItemAtPath(back, namedEventsEvents+"[at0002,'"+event+"']"+rel)
						if err != nil {
							t.Fatalf("%s in the event named %q: %v", rel, event, err)
						}
						l, ok := item.(rm.Locatable)
						if !ok {
							t.Fatalf("%s in the event named %q is a %T", rel, event, item)
						}
						if got := rm.DVTextValueOf(l.GetName()); got != want {
							t.Errorf("%s in the event named %q is named %q, want %q", rel, event, got, want)
						}
					}
				}
				for _, is := range validation.ValidateComposition(back, compiled).Issues {
					t.Errorf("decoded composition: %s at %s: %s", is.Code, is.Path, is.Detail)
				}
			})
		}
	}
}

// TestREQ053WithTemplateNameFallsBackToTheBareAlias — REQ-053, REQ-116: where
// the Web Template spells a node with a pinned name the compiled template
// holds at no such path, here 'Text A2' for the first event's ELEMENT, which
// the compiled template pins 'Text A', a WithTemplate decode names the node
// from the bare alias, the rubric the two events' ELEMENTs share, rather than
// leaving it with no name.
func TestREQ053WithTemplateNameFallsBackToTheBareAlias(t *testing.T) {
	opt, err := readOPTFile("testdata/named_sibling_items.opt")
	if err != nil {
		t.Fatalf("readOPTFile: %v", err)
	}
	wt, compiled := compileTemplate(t, "sdk_named_sibling_items.v1", opt)
	body, err := simplified.MarshalFlat(skeletonOf(t, compiled, instance.Large), wt)
	if err != nil {
		t.Fatalf("MarshalFlat: %v", err)
	}
	renamed := 0
	var rename func(n *webtemplate.Node)
	rename = func(n *webtemplate.Node) {
		if p := strings.ReplaceAll(n.AQLPath, ",'Text A']", ",'Text A2']"); p != n.AQLPath {
			n.AQLPath = p
			renamed++
		}
		for _, ch := range n.Children {
			rename(ch)
		}
	}
	rename(wt.Tree)
	if renamed == 0 {
		t.Fatal("no Web Template node pins 'Text A'")
	}
	back, err := simplified.UnmarshalFlat(body, wt, simplified.WithTemplate(compiled))
	if err != nil {
		t.Fatalf("UnmarshalFlat: %v", err)
	}
	const path = namedEventsEvents + "[at0002,'First event']/data[at0003]/items[at0004]"
	item, err := rmpath.ItemAtPath(back, path)
	if err != nil {
		t.Fatalf("ItemAtPath(%s): %v", path, err)
	}
	l, ok := item.(rm.Locatable)
	if !ok {
		t.Fatalf("%s is a %T", path, item)
	}
	if got := rm.DVTextValueOf(l.GetName()); got != "Text" {
		t.Errorf("%s is named %q, want the shared rubric %q", path, got, "Text")
	}
}

// TestREQ053ReusedSiblingOrdinalTerminology — REQ-053, REQ-116: where the
// ELEMENT at0004 of each named EVENT sibling is a C_DV_ORDINAL whose one pair
// codes X1 at 1, in local in 'First event' and in SNOMED-CT in 'Second
// event', a WithTemplate decode of a reader-shaped body, |code, |value and
// |ordinal with no terminology, gives each event's symbol the terminology its
// own sibling's pair gives, and the composition validates against the
// template.
func TestREQ053ReusedSiblingOrdinalTerminology(t *testing.T) {
	const id = "sdk_named_sibling_events.v1"
	raw, err := os.ReadFile("testdata/named_sibling_events.opt")
	if err != nil {
		t.Fatal(err)
	}
	// textValue opens the DV_TEXT that each event's at0004 holds, the only
	// DV_TEXT at that depth; its <node_id /> closes it.
	const textValue = "<children xsi:type=\"C_COMPLEX_OBJECT\">\n                              <rm_type_name>DV_TEXT</rm_type_name>"
	opt := string(raw)
	if n := strings.Count(opt, textValue); n != 2 {
		t.Fatalf("%s holds %d DV_TEXT values at at0004's depth, want 2", id, n)
	}
	for _, terminology := range []string{"local", "SNOMED-CT"} {
		at := strings.Index(opt, textValue)
		end := at + strings.Index(opt[at:], "<node_id />") + len("<node_id />")
		opt = opt[:at] + "<children xsi:type=\"C_DV_ORDINAL\">\n<rm_type_name>DV_ORDINAL</rm_type_name>" +
			opt[at+len(textValue):end] +
			"<list><value>1</value><symbol><defining_code><terminology_id><value>" + terminology +
			"</value></terminology_id><code_string>X1</code_string></defining_code></symbol></list>" + opt[end:]
	}
	model, err := aom14.ReadOperationalTemplate(strings.NewReader(opt))
	if err != nil {
		t.Fatalf("ReadOperationalTemplate: %v", err)
	}
	wt, compiled := compileTemplate(t, id, model)
	body := mustJSON(t, map[string]any{
		"ctx/language":                                  "en",
		"ctx/territory":                                 "NL",
		"ctx/composer_name":                             "Composer",
		"ctx/time":                                      "2026-10-26T09:00:00Z",
		namedEventsObs + "/text|code":                   "X1",
		namedEventsObs + "/text|value":                  "Mild",
		namedEventsObs + "/text|ordinal":                1,
		namedEventsObs + "/time":                        "2026-10-26T09:00:00Z",
		namedEventsObs + "/text2|code":                  "X1",
		namedEventsObs + "/text2|value":                 "Mild",
		namedEventsObs + "/text2|ordinal":               1,
		namedEventsObs + "/time2":                       "2026-10-26T10:00:00Z",
		"sdk_named_sibling_events/category|code":        "433",
		"sdk_named_sibling_events/category|value":       "event",
		"sdk_named_sibling_events/category|terminology": "openehr",
	})
	for _, format := range []string{"FLAT", "STRUCTURED"} {
		t.Run(format, func(t *testing.T) {
			back := decodeAs(t, format, body, wt, simplified.WithTemplate(compiled), simplified.WithStrict())
			for event, want := range map[string]string{"First event": "local", "Second event": "SNOMED-CT"} {
				ord, ok := namedEventItem(t, back, event, "at0004").(*rm.DVOrdinal)
				if !ok {
					t.Fatalf("the at0004 value of the event named %q is a %T, want *rm.DVOrdinal", event, namedEventItem(t, back, event, "at0004"))
				}
				if got := ord.Symbol.DefiningCode.TerminologyID.Value; got != want {
					t.Errorf("the ordinal of the event named %q: symbol terminology %q, want %s, its sibling's", event, got, want)
				}
			}
			for _, is := range validation.ValidateComposition(back, compiled).Issues {
				t.Errorf("decoded composition: %s at %s: %s", is.Code, is.Path, is.Detail)
			}
		})
	}
}

// jsonEqual reports whether a and b, two values decoded from JSON, are equal.
func jsonEqual(t *testing.T, a, b any) bool {
	t.Helper()
	return string(mustJSON(t, a)) == string(mustJSON(t, b))
}

// TestREQ053CoronaLargeRoundTripValidates — REQ-053, REQ-116: corona's
// skeleton under Large holds a member of every one of its reused siblings,
// four SECTIONs of one archetype and the OBSERVATIONs each of them reuses.
// Written as FLAT or STRUCTURED and decoded with the template, it validates
// against the template and re-encodes to the same FLAT.
func TestREQ053CoronaLargeRoundTripValidates(t *testing.T) {
	opt, err := readOPTFile(fixtures.WebTemplateOpt("Corona_Anamnese"))
	if err != nil {
		t.Fatalf("readOPTFile: %v", err)
	}
	wt, compiled := compileTemplate(t, "Corona_Anamnese", opt)
	comp := skeletonOf(t, compiled, instance.Large)
	keys := flatKeys(t, comp, wt)
	for _, format := range []string{"FLAT", "STRUCTURED"} {
		t.Run(format, func(t *testing.T) {
			back := bmiRoundTrip(t, format, comp, wt, simplified.WithTemplate(compiled), simplified.WithStrict())
			for _, is := range validation.ValidateComposition(back, compiled).Issues {
				t.Errorf("decoded composition: %s at %s: %s", is.Code, is.Path, is.Detail)
			}
			if got := flatKeys(t, back, wt); !maps.EqualFunc(got, keys, func(a, b any) bool { return jsonEqual(t, a, b) }) {
				t.Errorf("re-encoded FLAT differs:\n got %v\nwant %v", got, keys)
			}
		})
	}
}
