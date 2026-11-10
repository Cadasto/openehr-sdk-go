package simplified_test

// REQ-053 — the defaults a WithTemplate decode gives a body that leaves them
// out. The composition category is the one the template fixes, where it fixes
// one: a persistent template's composition is persistent, not the 433|event
// default. WithDefaultTime gives a body with no start time the caller's time,
// as if the body carried it in ctx/time (issue 420), except that on a
// persistent composition it builds no EVENT_CONTEXT.

import (
	"maps"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/cadasto/openehr-sdk-go/openehr/aom/aom14"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/simplified"
	"github.com/cadasto/openehr-sdk-go/openehr/template/webtemplate"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// The corpus templates the cases decode against. Both hold one OBSERVATION
// with one HISTORY and one EVENT; the first fixes the category 433|event, the
// second 431|persistent.
const (
	minimalObservationID = "minimal_observation.en.v1"
	persistentMinimalID  = "persistent_minimal.en.v1"
	// persistentCategory is the persistent template's category constraint, the
	// one code its C_CODE_PHRASE lists.
	persistentCategory = "<code_list>431</code_list>"
)

// The canonical-JSON members, from the composition, of the times both
// templates' OBSERVATION holds: its HISTORY origin and its one EVENT's time.
var (
	originPath    = []any{"content", 0, "data", "origin", "value"}
	eventTimePath = []any{"content", 0, "data", "events", 0, "time", "value"}
)

// callerClock is the time the cases pass WithDefaultTime, off UTC and with a
// fraction of a second, and callerClockText the value decode writes for it:
// in UTC, to the second.
var callerClock = time.Date(2026, 11, 10, 9, 30, 15, 500_000_000, time.FixedZone("CET", 3600))

const callerClockText = "2026-11-10T08:30:15Z"

// defaultsBody is issue 420's FLAT body for the template whose Web Template
// root is root: the three ctx/ fields and the one text leaf, with no ctx/time
// and no category. extra is merged over it.
func defaultsBody(root string, extra map[string]any) map[string]any {
	body := map[string]any{
		"ctx/language":           "en",
		"ctx/territory":          "US",
		"ctx/composer_name":      "Silvia Blake",
		root + "/minimal:0/text": "first value",
	}
	maps.Copy(body, extra)
	return body
}

// editedCorpusTemplate compiles the corpus OPT id with its one occurrence of
// old replaced by repl, and builds its Web Template.
func editedCorpusTemplate(t *testing.T, id, old, repl string) (*webtemplate.WebTemplate, *templatecompile.Compiled) {
	t.Helper()
	raw, err := os.ReadFile(fixtures.TemplateOpt(id))
	if err != nil {
		t.Fatalf("read %s: %v", id, err)
	}
	model, err := aom14.ReadOperationalTemplate(strings.NewReader(replaceExactlyOnce(t, string(raw), old, repl)))
	if err != nil {
		t.Fatalf("ReadOperationalTemplate(edited %s): %v", id, err)
	}
	return compileTemplate(t, id, model)
}

// decodeBothFormats decodes body, a FLAT body, as FLAT and, restructured, as
// STRUCTURED, with opts, failing the test on an error. It returns the two
// compositions by format name.
func decodeBothFormats(t *testing.T, wt *webtemplate.WebTemplate, body map[string]any, opts ...simplified.Option) map[string]*rm.Composition {
	t.Helper()
	flat := mustJSON(t, body)
	structured, err := simplified.FlatToStructured(flat)
	if err != nil {
		t.Fatalf("FlatToStructured(%s): %v", flat, err)
	}
	fromFlat, err := simplified.UnmarshalFlat(flat, wt, opts...)
	if err != nil {
		t.Fatalf("UnmarshalFlat(%s): %v", flat, err)
	}
	fromStructured, err := simplified.UnmarshalStructured(structured, wt, opts...)
	if err != nil {
		t.Fatalf("UnmarshalStructured(%s): %v", structured, err)
	}
	return map[string]*rm.Composition{"FLAT": fromFlat, "STRUCTURED": fromStructured}
}

// wantCategory fails the test unless comp's category is code in the openEHR
// terminology, with text as its value.
func wantCategory(t *testing.T, format string, comp *rm.Composition, code, text string) {
	t.Helper()
	got := comp.Category
	if got.DefiningCode.CodeString != code || got.DefiningCode.TerminologyID.Value != "openehr" || got.Value != text {
		t.Errorf("%s: category = %s::%s|%s|, want openehr::%s|%s|", format,
			got.DefiningCode.TerminologyID.Value, got.DefiningCode.CodeString, got.Value, code, text)
	}
}

// wantValidates fails the test unless comp's validation against compiled
// reports OK as want.
func wantValidates(t *testing.T, format string, comp *rm.Composition, compiled *templatecompile.Compiled, want bool) {
	t.Helper()
	if r := validation.ValidateComposition(comp, compiled); r.OK != want {
		t.Errorf("%s: ValidateComposition OK = %v, want %v; issues=%v", format, r.OK, want, r.Issues)
	}
}

// wantTimes fails the test unless each of paths, a path of canonical-JSON
// members from the composition (a name, or an array index), holds want in
// comp.
func wantTimes(t *testing.T, format string, comp *rm.Composition, want string, paths ...[]any) {
	t.Helper()
	doc := canonicalDoc(t, comp)
	for _, path := range paths {
		var at any = doc
		for _, key := range path {
			switch k := key.(type) {
			case string:
				m, _ := at.(map[string]any)
				at = m[k]
			case int:
				if list, _ := at.([]any); k < len(list) {
					at = list[k]
				} else {
					at = nil
				}
			}
		}
		if at != want {
			t.Errorf("%s: %v = %#v, want %q", format, path, at, want)
		}
	}
}

// wantContext fails the test unless comp has an EVENT_CONTEXT that starts at
// start, in the 238|other care setting.
func wantContext(t *testing.T, format string, comp *rm.Composition, start string) {
	t.Helper()
	ctx := comp.Context
	if ctx == nil {
		t.Errorf("%s: no EVENT_CONTEXT, want one starting at %s", format, start)
		return
	}
	if ctx.StartTime.Value != start {
		t.Errorf("%s: context start_time = %q, want %q", format, ctx.StartTime.Value, start)
	}
	if s := ctx.Setting; s.DefiningCode.CodeString != "238" || s.DefiningCode.TerminologyID.Value != "openehr" || s.Value != "other care" {
		t.Errorf("%s: context setting = %s::%s|%s|, want openehr::238|other care|", format,
			s.DefiningCode.TerminologyID.Value, s.DefiningCode.CodeString, s.Value)
	}
}

// TestREQ053TemplateFixesTheCategory — REQ-053: with the template, a body
// that spells no category takes the one the template fixes, a C_CODE_PHRASE
// listing one openehr code, with that code's openEHR rubric as its text. Where
// the template fixes none, the category stays 433|event.
func TestREQ053TemplateFixesTheCategory(t *testing.T) {
	const startTime = "2026-01-01T00:00:00Z"
	persistentWT, persistent := templateOf(t, persistentMinimalID)
	eventWT, event := templateOf(t, minimalObservationID)
	twoCodesWT, twoCodes := editedCorpusTemplate(t, persistentMinimalID, persistentCategory,
		persistentCategory+"<code_list>433</code_list>")
	localWT, local := editedCorpusTemplate(t, persistentMinimalID,
		"<value>openehr</value>\n            </terminology_id>\n            "+persistentCategory,
		"<value>local</value>\n            </terminology_id>\n            "+persistentCategory)
	cases := []struct {
		name      string
		wt        *webtemplate.WebTemplate
		compiled  *templatecompile.Compiled
		root      string
		code      string
		text      string
		validates bool
	}{
		{name: "a persistent template", wt: persistentWT, compiled: persistent, root: "persistent_minimal", code: "431", text: "persistent", validates: true},
		{name: "an event template", wt: eventWT, compiled: event, root: "minimal", code: "433", text: "event", validates: true},
		{name: "a template listing two codes", wt: twoCodesWT, compiled: twoCodes, root: "persistent_minimal", code: "433", text: "event", validates: true},
		{name: "a template fixing a local code", wt: localWT, compiled: local, root: "persistent_minimal", code: "433", text: "event", validates: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := defaultsBody(tc.root, map[string]any{"ctx/time": startTime})
			for format, comp := range decodeBothFormats(t, tc.wt, body, simplified.WithTemplate(tc.compiled)) {
				wantCategory(t, format, comp, tc.code, tc.text)
				wantValidates(t, format, comp, tc.compiled, tc.validates)
			}
		})
	}
}

// TestREQ053BodyCategoryKeptAsWritten — REQ-053: a category the body spells is
// kept as written, even where the template fixes another; validation judges
// it.
func TestREQ053BodyCategoryKeptAsWritten(t *testing.T) {
	wt, compiled := templateOf(t, persistentMinimalID)
	body := defaultsBody("persistent_minimal", map[string]any{
		"ctx/time":                                "2026-01-01T00:00:00Z",
		"persistent_minimal/category|code":        "433",
		"persistent_minimal/category|value":       "event",
		"persistent_minimal/category|terminology": "openehr",
	})
	comp := decodeWith(t, wt, body, simplified.WithTemplate(compiled))
	wantCategory(t, "FLAT", comp, "433", "event")
}

// TestREQ053DefaultTimeCompletesTheTimes — REQ-053, issue 420: a body with no
// start time decodes, with the template, to a composition with no
// EVENT_CONTEXT whose HISTORY origin and EVENT time are missing, which does
// not validate. With WithDefaultTime it decodes as if it carried ctx/time with
// the caller's time: the context starts then, in the 238|other care setting,
// the origin and the event time take it, and the composition validates.
func TestREQ053DefaultTimeCompletesTheTimes(t *testing.T) {
	wt, compiled := templateOf(t, minimalObservationID)
	body := defaultsBody("minimal", nil)
	for format, comp := range decodeBothFormats(t, wt, body, simplified.WithTemplate(compiled)) {
		if comp.Context != nil {
			t.Errorf("%s: without WithDefaultTime, context = %+v, want none", format, comp.Context)
		}
		wantValidates(t, format, comp, compiled, false)
	}
	for format, comp := range decodeBothFormats(t, wt, body, simplified.WithTemplate(compiled), simplified.WithDefaultTime(callerClock)) {
		wantContext(t, format, comp, callerClockText)
		wantTimes(t, format, comp, callerClockText, originPath, eventTimePath)
		wantValidates(t, format, comp, compiled, true)
	}
}

// TestREQ053DefaultTimeFillsTheActionTime — REQ-053: the default time is the
// ACTION's time too, as a body's ctx/time is.
func TestREQ053DefaultTimeFillsTheActionTime(t *testing.T) {
	opt, err := readOPTFile(fixtures.TemplateOpt("minimal_action_2"))
	if err != nil {
		t.Fatalf("readOPTFile(minimal_action_2): %v", err)
	}
	wt, compiled := compileTemplate(t, "minimal_action_2", opt)
	const action = "minimal/minimal_2:0"
	body := map[string]any{
		"ctx/language":                                 "en",
		"ctx/territory":                                "NL",
		"ctx/composer_name":                            "Composer",
		action + "/proportion|numerator":               0,
		action + "/proportion|denominator":             1,
		action + "/proportion|type":                    3,
		action + "/ism_transition/current_state|code":  "532",
		action + "/ism_transition/current_state|value": "completed",
	}
	for format, comp := range decodeBothFormats(t, wt, body, simplified.WithTemplate(compiled), simplified.WithDefaultTime(callerClock)) {
		wantContext(t, format, comp, callerClockText)
		wantTimes(t, format, comp, callerClockText, []any{"content", 0, "time", "value"})
		wantValidates(t, format, comp, compiled, true)
	}
}

// TestREQ053DefaultTimeYieldsToTheBody — REQ-053: a start time the body gives,
// in ctx/time or at its real path, is kept, and the default fills nothing;
// an empty one is absent, and the default fills it.
func TestREQ053DefaultTimeYieldsToTheBody(t *testing.T) {
	const bodyTime = "2019-03-22T22:00:00+01:00"
	wt, compiled := templateOf(t, minimalObservationID)
	cases := []struct {
		name  string
		extra map[string]any
		want  string
	}{
		{"ctx/time", map[string]any{"ctx/time": bodyTime}, bodyTime},
		{"the real path", map[string]any{"minimal/context/start_time": bodyTime}, bodyTime},
		{"an empty ctx/time", map[string]any{"ctx/time": ""}, callerClockText},
		{"an empty real path", map[string]any{"minimal/context/start_time": ""}, callerClockText},
		{"a null ctx/time", map[string]any{"ctx/time": nil}, callerClockText},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := defaultsBody("minimal", tc.extra)
			for format, comp := range decodeBothFormats(t, wt, body, simplified.WithTemplate(compiled), simplified.WithDefaultTime(callerClock)) {
				wantContext(t, format, comp, tc.want)
				wantTimes(t, format, comp, tc.want, originPath, eventTimePath)
				wantValidates(t, format, comp, compiled, true)
			}
		})
	}
}

// TestREQ053DefaultTimeOff — REQ-053: the zero time leaves WithDefaultTime
// off, and without WithTemplate the option does nothing: each decode is the
// one without the option.
func TestREQ053DefaultTimeOff(t *testing.T) {
	wt, compiled := templateOf(t, minimalObservationID)
	body := defaultsBody("minimal", nil)
	cases := []struct {
		name    string
		with    []simplified.Option
		without []simplified.Option
	}{
		{
			name:    "the zero time",
			with:    []simplified.Option{simplified.WithTemplate(compiled), simplified.WithDefaultTime(time.Time{})},
			without: []simplified.Option{simplified.WithTemplate(compiled)},
		},
		{
			name:    "no template",
			with:    []simplified.Option{simplified.WithDefaultTime(callerClock)},
			without: nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			want := decodeBothFormats(t, wt, body, tc.without...)
			for format, comp := range decodeBothFormats(t, wt, body, tc.with...) {
				if comp.Context != nil {
					t.Errorf("%s: context = %+v, want none", format, comp.Context)
				}
				if got, want := canonicalDoc(t, comp), canonicalDoc(t, want[format]); !jsonEqual(t, got, want) {
					t.Errorf("%s: decode with the option = %s, want the decode without it, %s", format, mustJSON(t, got), mustJSON(t, want))
				}
			}
		})
	}
}

// TestREQ053DefaultTimeOnPersistent — REQ-053: on a composition whose
// category is 431|persistent, the body's or the template's, the default time
// builds no EVENT_CONTEXT but still fills the origin and the event time. An
// EVENT_CONTEXT the body builds (with a setting, say) takes the default as its
// start time, a body's own start time builds one as before, and a body
// category other than persistent, code 431 in another terminology included,
// lets the default build one.
func TestREQ053DefaultTimeOnPersistent(t *testing.T) {
	const bodyTime = "2019-03-22T22:00:00+01:00"
	persistentWT, persistent := templateOf(t, persistentMinimalID)
	eventWT, event := templateOf(t, minimalObservationID)
	bodyCategory := func(root, terminology, code, text string) map[string]any {
		return map[string]any{
			root + "/category|code":        code,
			root + "/category|value":       text,
			root + "/category|terminology": terminology,
		}
	}
	cases := []struct {
		name     string
		wt       *webtemplate.WebTemplate
		compiled *templatecompile.Compiled
		body     map[string]any
		// start is the context's start time, or "" for no context.
		start string
		// times is the origin and event time.
		times string
		// validates is whether the composition validates.
		validates bool
	}{
		{
			name: "the template's category", wt: persistentWT, compiled: persistent,
			body:  defaultsBody("persistent_minimal", nil),
			start: "", times: callerClockText, validates: true,
		},
		{
			name: "a body start time", wt: persistentWT, compiled: persistent,
			body:  defaultsBody("persistent_minimal", map[string]any{"ctx/time": bodyTime}),
			start: bodyTime, times: bodyTime, validates: true,
		},
		{
			name: "a body setting", wt: persistentWT, compiled: persistent,
			body:  defaultsBody("persistent_minimal", map[string]any{"ctx/setting|code": "238"}),
			start: callerClockText, times: callerClockText, validates: true,
		},
		{
			name: "the body's category", wt: eventWT, compiled: event,
			body:  defaultsBody("minimal", bodyCategory("minimal", "openehr", "431", "persistent")),
			start: "", times: callerClockText, validates: false,
		},
		{
			name: "a body category of event", wt: persistentWT, compiled: persistent,
			body:  defaultsBody("persistent_minimal", bodyCategory("persistent_minimal", "openehr", "433", "event")),
			start: callerClockText, times: callerClockText, validates: false,
		},
		{
			name: "a body category coded 431 in another terminology", wt: eventWT, compiled: event,
			body:  defaultsBody("minimal", bodyCategory("minimal", "local", "431", "persistent")),
			start: callerClockText, times: callerClockText, validates: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for format, comp := range decodeBothFormats(t, tc.wt, tc.body, simplified.WithTemplate(tc.compiled), simplified.WithDefaultTime(callerClock)) {
				if tc.start == "" {
					if comp.Context != nil {
						t.Errorf("%s: context = %+v, want none", format, comp.Context)
					}
				} else {
					wantContext(t, format, comp, tc.start)
				}
				wantTimes(t, format, comp, tc.times, originPath, eventTimePath)
				wantValidates(t, format, comp, tc.compiled, tc.validates)
			}
		})
	}
}
