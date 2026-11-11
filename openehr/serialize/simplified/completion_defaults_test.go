package simplified_test

// REQ-053 — two more defaults a WithTemplate decode gives a body that leaves
// them out. A HISTORY with no origin takes the earliest time its own events
// carry, compared as instants, else the start time, as the reference
// implementation orders the two; an event with no time then takes its
// HISTORY's origin (Simplified Formats § time: "EVENT.time will be set to
// history.origin"). An ACTIVITY with no action_archetype_id takes the one
// the generator writes for the C_STRING its template puts there.

import (
	"fmt"
	"maps"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/cadasto/openehr-sdk-go/openehr/aom/aom14"
	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/simplified"
	"github.com/cadasto/openehr-sdk-go/openehr/template/webtemplate"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
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

// The corpus templates the action_archetype_id cases decode against. The
// first's one ACTIVITY constrains the id by a pattern; the second's two by a
// C_STRING with neither a pattern nor a list (open_activity) and by a list
// (listed_activity).
const (
	minimalInstructionID = "minimal_instruction.en.v1"
	structureID          = "sdk_crosscheck_structure.v1"
)

// actionArchetypeAttribute matches the one action_archetype_id attribute of
// the minimal_instruction OPT, its C_STRING included.
var actionArchetypeAttribute = regexp.MustCompile(`(?s)<attributes xsi:type="C_SINGLE_ATTRIBUTE">\s*<rm_attribute_name>action_archetype_id</rm_attribute_name>.*?</attributes>`)

// firstAlternative is a C_STRING alternative on action_archetype_id, with a
// pattern of its own, that alternativesInstructionTemplate puts before the
// OPT's.
const firstAlternative = `<children xsi:type="C_PRIMITIVE_OBJECT"><rm_type_name>STRING</rm_type_name><node_id /><item xsi:type="C_STRING"><pattern>openEHR-EHR-ACTION\.first\.v1</pattern></item></children>`

// silentInstructionTemplate compiles the minimal_instruction OPT with its
// action_archetype_id attribute removed, so the template says nothing of it,
// under opts, and builds its Web Template.
func silentInstructionTemplate(t *testing.T, opts ...templatecompile.Option) (*webtemplate.WebTemplate, *templatecompile.Compiled) {
	t.Helper()
	return editedInstructionTemplate(t, func(string) string { return "" }, opts...)
}

// alternativesInstructionTemplate compiles the minimal_instruction OPT with
// firstAlternative put before the C_STRING its action_archetype_id attribute
// holds, and builds its Web Template.
func alternativesInstructionTemplate(t *testing.T) (*webtemplate.WebTemplate, *templatecompile.Compiled) {
	t.Helper()
	return editedInstructionTemplate(t, func(attribute string) string {
		return strings.Replace(attribute, "<children", firstAlternative+"<children", 1)
	})
}

// editedInstructionTemplate compiles the minimal_instruction OPT with its one
// action_archetype_id attribute replaced by what edit returns for it, under
// opts, and builds its Web Template.
func editedInstructionTemplate(t *testing.T, edit func(attribute string) string, opts ...templatecompile.Option) (*webtemplate.WebTemplate, *templatecompile.Compiled) {
	t.Helper()
	return rewrittenInstructionTemplate(t, func(opt string) string {
		if n := len(actionArchetypeAttribute.FindAllStringIndex(opt, -1)); n != 1 {
			t.Fatalf("%s holds %d action_archetype_id attributes, want one — fixture changed?", minimalInstructionID, n)
		}
		return actionArchetypeAttribute.ReplaceAllStringFunc(opt, edit)
	}, opts...)
}

// activityStart matches the opening of the one ACTIVITY node of the
// minimal_instruction OPT.
var activityStart = regexp.MustCompile(`<children xsi:type="C_COMPLEX_OBJECT">\s*<rm_type_name>ACTIVITY</rm_type_name>`)

// twinName is the name the twin ACTIVITY of twinActivityTemplate pins, as a
// name attribute.
const twinName = `<attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>name</rm_attribute_name><children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>DV_TEXT</rm_type_name><node_id /><attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>value</rm_attribute_name><children xsi:type="C_PRIMITIVE_OBJECT"><rm_type_name>STRING</rm_type_name><node_id /><item xsi:type="C_STRING"><list>Twin activity</list></item></children></attributes></children></attributes>`

// twinActivityTemplate compiles the minimal_instruction OPT with a twin of
// its one ACTIVITY node put after it: the same node id, but the name 'Twin
// activity' pinned and an action_archetype_id pattern naming another ACTION
// archetype. The twin has a path of its own,
// /activities[at0001,'Twin activity'], and the unnamed one keeps
// /activities[at0001], which is also the twin's path with its name left out.
// It builds its Web Template too.
func twinActivityTemplate(t *testing.T) (*webtemplate.WebTemplate, *templatecompile.Compiled) {
	t.Helper()
	return rewrittenInstructionTemplate(t, func(opt string) string {
		found := activityStart.FindAllStringIndex(opt, -1)
		if len(found) != 1 {
			t.Fatalf("%s holds %d ACTIVITY nodes, want one — fixture changed?", minimalInstructionID, len(found))
		}
		start := found[0][0]
		// The node ends at the </children> that closes its <children>.
		depth := 0
		for i := start; i < len(opt); i++ {
			switch {
			case strings.HasPrefix(opt[i:], "<children"):
				depth++
			case strings.HasPrefix(opt[i:], "</children>"):
				depth--
				if depth == 0 {
					end := i + len("</children>")
					activity := opt[start:end]
					twin := strings.Replace(activity, `openEHR-EHR-ACTION\.minimal\.v1`, `openEHR-EHR-ACTION\.twin\.v1`, 1)
					if twin == activity {
						t.Fatalf("%s: the ACTIVITY's action_archetype_id pattern changed — fixture changed?", minimalInstructionID)
					}
					twin = strings.Replace(twin, `<attributes xsi:type="C_SINGLE_ATTRIBUTE">`, twinName+`<attributes xsi:type="C_SINGLE_ATTRIBUTE">`, 1)
					return opt[:end] + twin + opt[end:]
				}
			}
		}
		t.Fatalf("%s: the ACTIVITY node is never closed", minimalInstructionID)
		return ""
	})
}

// rewrittenInstructionTemplate compiles the minimal_instruction OPT as
// rewrite returns it, under opts, and builds its Web Template.
func rewrittenInstructionTemplate(t *testing.T, rewrite func(opt string) string, opts ...templatecompile.Option) (*webtemplate.WebTemplate, *templatecompile.Compiled) {
	t.Helper()
	raw, err := os.ReadFile(fixtures.TemplateOpt(minimalInstructionID))
	if err != nil {
		t.Fatalf("read %s: %v", minimalInstructionID, err)
	}
	edited := rewrite(string(raw))
	model, err := aom14.ReadOperationalTemplate(strings.NewReader(edited))
	if err != nil {
		t.Fatalf("ReadOperationalTemplate(edited %s): %v", minimalInstructionID, err)
	}
	compiled, err := templatecompile.Compile(model, opts...)
	if err != nil {
		t.Fatalf("Compile(edited %s): %v", minimalInstructionID, err)
	}
	wt, err := webtemplate.Build(compiled)
	if err != nil {
		t.Fatalf("Build(edited %s): %v", minimalInstructionID, err)
	}
	return wt, compiled
}

// activityIDs returns the action_archetype_id of every ACTIVITY in comp, by
// the ACTIVITY's archetype_node_id; an ACTIVITY with none maps to "".
func activityIDs(t *testing.T, comp *rm.Composition) map[string]string {
	t.Helper()
	return activityIDsBy(t, comp, func(activity map[string]any) string {
		id, _ := activity["archetype_node_id"].(string)
		return id
	})
}

// activityNamedIDs returns the action_archetype_id of every ACTIVITY in comp,
// by the ACTIVITY's name; an ACTIVITY with none maps to "".
func activityNamedIDs(t *testing.T, comp *rm.Composition) map[string]string {
	t.Helper()
	return activityIDsBy(t, comp, func(activity map[string]any) string {
		name, _ := activity["name"].(map[string]any)
		value, _ := name["value"].(string)
		return value
	})
}

// activityIDsBy returns the action_archetype_id of every ACTIVITY in comp, by
// what key returns for the ACTIVITY's canonical object; an ACTIVITY with none
// maps to "".
func activityIDsBy(t *testing.T, comp *rm.Composition, key func(activity map[string]any) string) map[string]string {
	t.Helper()
	ids := make(map[string]string)
	var walk func(v any)
	walk = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			if x["_type"] == "ACTIVITY" {
				ids[key(x)], _ = x["action_archetype_id"].(string)
			}
			for _, child := range x {
				walk(child)
			}
		case []any:
			for _, child := range x {
				walk(child)
			}
		}
	}
	walk(canonicalDoc(t, comp))
	return ids
}

// generatedActivityIDs returns what activityIDs returns for the composition
// the instance generator writes for compiled under its Large policy.
func generatedActivityIDs(t *testing.T, compiled *templatecompile.Compiled) map[string]string {
	t.Helper()
	return activityIDs(t, generatedComposition(t, compiled))
}

// generatedComposition returns the composition the instance generator writes
// for compiled under its Large policy.
func generatedComposition(t *testing.T, compiled *templatecompile.Compiled) *rm.Composition {
	t.Helper()
	composer := "Test Composer"
	out, err := instance.Generate(t.Context(), compiled, instance.Options{
		Policy:    instance.Large,
		Territory: "NL",
		Composer:  &rm.PartyIdentified{Name: &composer},
		Now:       time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("instance.Generate: %v", err)
	}
	comp, err := instance.AsComposition(out)
	if err != nil {
		t.Fatalf("instance.AsComposition: %v", err)
	}
	return comp
}

// instructionBody is a minimal_instruction FLAT body whose one ACTIVITY holds
// a duration, with extra merged over it.
func instructionBody(extra map[string]any) map[string]any {
	body := map[string]any{
		"ctx/language":      "en",
		"ctx/territory":     "NL",
		"ctx/composer_name": "Silvia Blake",
		"ctx/time":          originBodyStart,
		"minimal/minimal:0/current_activity/duration": "PT10M",
		"minimal/minimal:0/narrative":                 "narrative",
	}
	maps.Copy(body, extra)
	return body
}

// structureBody is an sdk_crosscheck_structure FLAT body: the participation
// its context requires, its mandatory OBSERVATION, and its INSTRUCTION's two
// ACTIVITY nodes, each holding a text.
func structureBody() map[string]any {
	const root = "sdk_cross-check_structure"
	return map[string]any{
		"ctx/language":      "en",
		"ctx/territory":     "NL",
		"ctx/composer_name": "Silvia Blake",
		"ctx/time":          originBodyStart,
		root + "/context/_participation:0|function":    "requester",
		root + "/context/_participation:0|name":        "Dr. Lee",
		root + "/sdk_structure/any_event:0/text":       "observed",
		root + "/sdk_instruction/open_activity/text":   "open",
		root + "/sdk_instruction/listed_activity/text": "listed",
		root + "/sdk_instruction/narrative":            "narrative",
	}
}

// TestREQ053ActivityTakesTheTemplatesActionArchetypeID — REQ-053: with the
// template, an ACTIVITY the body gives no action_archetype_id takes the value
// the generator writes for the C_STRING its template puts on that attribute:
// the RM default /.*/ where the C_STRING admits it, as one with neither a
// pattern nor a list or with the pattern .* does, or where the template says
// nothing of the attribute, whether or not the compiled template holds the
// attribute the RM requires there; else a list member or a pattern match the
// C_STRING accepts. Of several alternatives, the first decides, as it does
// for the generator. The composition then validates, and the value is the
// generator's.
func TestREQ053ActivityTakesTheTemplatesActionArchetypeID(t *testing.T) {
	instructionWT, instruction := templateOf(t, minimalInstructionID)
	silentWT, silent := silentInstructionTemplate(t)
	explicitWT, explicit := silentInstructionTemplate(t, templatecompile.WithoutImplicitAttributes())
	alternativesWT, alternatives := alternativesInstructionTemplate(t)
	anyPatternWT, anyPattern := editedInstructionTemplate(t, func(attribute string) string {
		edited := strings.Replace(attribute, `<pattern>openEHR-EHR-ACTION\.minimal\.v1</pattern>`, `<pattern>.*</pattern>`, 1)
		if edited == attribute {
			t.Fatalf("%s: the ACTIVITY's action_archetype_id pattern changed — fixture changed?", minimalInstructionID)
		}
		return edited
	})
	structureWT, structure := templateOf(t, structureID)
	cases := []struct {
		name     string
		wt       *webtemplate.WebTemplate
		compiled *templatecompile.Compiled
		body     map[string]any
		// want maps each ACTIVITY's archetype_node_id to its
		// action_archetype_id.
		want map[string]string
	}{
		{
			name: "a pattern", wt: instructionWT, compiled: instruction, body: instructionBody(nil),
			want: map[string]string{"at0001": "openEHR-EHR-ACTION.minimal.v1"},
		},
		{
			name: "a pattern that admits the RM default", wt: anyPatternWT, compiled: anyPattern, body: instructionBody(nil),
			want: map[string]string{"at0001": "/.*/"},
		},
		{
			name: "a template silent on it", wt: silentWT, compiled: silent, body: instructionBody(nil),
			want: map[string]string{"at0001": "/.*/"},
		},
		{
			// Compiled with only the attributes the OPT declares, the
			// ACTIVITY node holds no action_archetype_id attribute at all.
			name: "a template silent on it, compiled without implicit attributes", wt: explicitWT, compiled: explicit, body: instructionBody(nil),
			want: map[string]string{"at0001": "/.*/"},
		},
		{
			name: "two alternatives: the first", wt: alternativesWT, compiled: alternatives, body: instructionBody(nil),
			want: map[string]string{"at0001": "openEHR-EHR-ACTION.first.v1"},
		},
		{
			name: "neither a pattern nor a list, and a list", wt: structureWT, compiled: structure, body: structureBody(),
			want: map[string]string{"at0001": "/.*/", "at0004": "openEHR-EHR-ACTION.sdk_unwritten.v1"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			generated := generatedActivityIDs(t, tc.compiled)
			for format, comp := range decodeBothFormats(t, tc.wt, tc.body, simplified.WithTemplate(tc.compiled)) {
				got := activityIDs(t, comp)
				if !maps.Equal(got, tc.want) {
					t.Errorf("%s: action_archetype_id by ACTIVITY = %v, want %v", format, got, tc.want)
				}
				for id, want := range got {
					if generated[id] != want {
						t.Errorf("%s: ACTIVITY %s: action_archetype_id = %q, the generator writes %q", format, id, want, generated[id])
					}
				}
				wantValidates(t, format, comp, tc.compiled, true)
			}
		})
	}
}

// unbuildableAlternative is a C_STRING on action_archetype_id that accepts no
// value the generator builds: its one list member fails its pattern.
const unbuildableAlternative = `<children xsi:type="C_PRIMITIVE_OBJECT"><rm_type_name>STRING</rm_type_name><node_id /><item xsi:type="C_STRING"><pattern>[0-9]+</pattern><list>abc</list></item></children>`

// TestREQ053ActivityTakesItsOwnNodesActionArchetypeID — REQ-053: an ACTIVITY
// that pins no name, beside a twin of its node id that pins one and names
// another ACTION archetype, takes the action_archetype_id its own node gives,
// and the twin takes its own, as the generator writes them. Both ACTIVITY
// nodes are reachable by the path with no name, and only the unnamed one's
// node is there. The composition then validates.
func TestREQ053ActivityTakesItsOwnNodesActionArchetypeID(t *testing.T) {
	wt, compiled := twinActivityTemplate(t)
	body := instructionBody(map[string]any{"minimal/minimal:0/twin_activity/duration": "PT20M"})
	want := map[string]string{"Current Activity": "openEHR-EHR-ACTION.minimal.v1", "Twin activity": "openEHR-EHR-ACTION.twin.v1"}
	if got := activityNamedIDs(t, generatedComposition(t, compiled)); !maps.Equal(got, want) {
		t.Fatalf("generator: action_archetype_id by ACTIVITY name = %v, want %v", got, want)
	}
	for format, comp := range decodeBothFormats(t, wt, body, simplified.WithTemplate(compiled)) {
		if got := activityNamedIDs(t, comp); !maps.Equal(got, want) {
			t.Errorf("%s: action_archetype_id by ACTIVITY name = %v, want %v", format, got, want)
		}
		wantValidates(t, format, comp, compiled, true)
	}
}

// TestREQ053ActivityTakesNoActionArchetypeIDTheTemplateDoesNotSettle —
// REQ-053: with the template, decode leaves out the action_archetype_id of an
// ACTIVITY the body gives none where the first C_STRING on that attribute
// accepts no value the generator builds, and the generator refuses the
// template too.
func TestREQ053ActivityTakesNoActionArchetypeIDTheTemplateDoesNotSettle(t *testing.T) {
	unbuildableWT, unbuildable := editedInstructionTemplate(t, func(attribute string) string {
		head, _, _ := strings.Cut(attribute, "<children")
		return head + unbuildableAlternative + "</attributes>"
	})
	cases := []struct {
		name     string
		wt       *webtemplate.WebTemplate
		compiled *templatecompile.Compiled
		body     map[string]any
		// want maps each ACTIVITY's name to its action_archetype_id, ""
		// where decode leaves it out.
		want map[string]string
		// generatorRefuses is whether the instance generator refuses the
		// template.
		generatorRefuses bool
	}{
		{
			name: "a C_STRING no built value satisfies", wt: unbuildableWT, compiled: unbuildable,
			body: instructionBody(nil), want: map[string]string{"Current Activity": ""}, generatorRefuses: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.generatorRefuses {
				composer := "Test Composer"
				if _, err := instance.Generate(t.Context(), tc.compiled, instance.Options{
					Policy:    instance.Large,
					Territory: "NL",
					Composer:  &rm.PartyIdentified{Name: &composer},
					Now:       time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
				}); err == nil {
					t.Errorf("instance.Generate = nil error, want the template refused")
				}
			}
			for format, comp := range decodeBothFormats(t, tc.wt, tc.body, simplified.WithTemplate(tc.compiled)) {
				if got := activityNamedIDs(t, comp); !maps.Equal(got, tc.want) {
					t.Errorf("%s: action_archetype_id by ACTIVITY name = %v, want %v", format, got, tc.want)
				}
			}
		})
	}
}

// TestREQ053ActivityKeepsTheBodysActionArchetypeID — REQ-053: an
// action_archetype_id the body gives is kept as written, even one the
// template's C_STRING refuses, and an empty one stays refused.
func TestREQ053ActivityKeepsTheBodysActionArchetypeID(t *testing.T) {
	const key = "minimal/minimal:0/current_activity/action_archetype_id"
	wt, compiled := templateOf(t, minimalInstructionID)
	for _, given := range []string{"openEHR-EHR-ACTION.minimal.v1", "/openEHR-EHR-ACTION\\.other\\.v1/"} {
		body := instructionBody(map[string]any{key: given})
		for format, comp := range decodeBothFormats(t, wt, body, simplified.WithTemplate(compiled)) {
			if got := activityIDs(t, comp)["at0001"]; got != given {
				t.Errorf("%s: action_archetype_id = %q, want the body's %q", format, got, given)
			}
		}
	}
	body := instructionBody(map[string]any{key: ""})
	if _, err := simplified.UnmarshalFlat(mustJSON(t, body), wt, simplified.WithTemplate(compiled)); err == nil {
		t.Errorf("UnmarshalFlat(empty %s) = nil error, want the empty id refused", key)
	}
}
