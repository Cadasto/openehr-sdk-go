package validation_test

// rmfloor_nonempty_lists_test.go: REQ-112 § Per-RM-type invariant
// catalogue, Non-empty lists; REQ-102 § An existence lower bound alone asks
// for no member; REQ-171 § One walk, step 2. Five RM invariants make a
// list hold a member once it is present (COMPOSITION Content_valid,
// EVENT_CONTEXT Participations_validity, INSTRUCTION Activities_valid,
// SECTION Items_valid, ATTESTATION Items_valid), and HISTORY Events_valid
// asks for an event or a summary. The floor reports a present list with no
// item at the list's path and a HISTORY with neither at its own. Under a
// template, an existence lower bound of 1 or more on one of these lists asks
// for a member, so the template reports the list as `required`, absent or
// empty, and the RM rule gives way where the template reports the list at
// its path or below it, or, for a HISTORY's events, reports its summary:
// one defect, one finding.

import (
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// rmHistory is a HISTORY of the archetype historyOPT carries, with the
// given events and summary, and the attributes the RM requires otherwise.
func rmHistory(events []rm.Event, summary rm.ItemStructure) *rm.History[rm.ItemStructure] {
	return &rm.History[rm.ItemStructure]{
		ArchetypeNodeID: historyArchetype,
		Name:            rm.DVText{Value: "history"},
		Origin:          rm.DVDateTime{Value: "2026-10-25T10:00:00Z"},
		Events:          events,
		Summary:         summary,
	}
}

// historyArchetype is the archetype of historyOPT's root.
const historyArchetype = "openEHR-EHR-HISTORY.nonempty_lists.v1"

// pointEvent is a POINT_EVENT at0002 with the attributes the RM requires.
func pointEvent() *rm.PointEvent[rm.ItemStructure] {
	return &rm.PointEvent[rm.ItemStructure]{
		ArchetypeNodeID: "at0002",
		Name:            rm.DVText{Value: "event"},
		Time:            rm.DVDateTime{Value: "2026-10-25T10:00:00Z"},
		Data:            &rm.ItemTree{ArchetypeNodeID: "at0003", Name: rm.DVText{Value: "data"}},
	}
}

// summaryTree is an ITEM_TREE a HISTORY's summary holds.
func summaryTree() *rm.ItemTree {
	return &rm.ItemTree{ArchetypeNodeID: "at0004", Name: rm.DVText{Value: "summary"}}
}

// withRule returns the issues of r whose rule is rule.
func withRule(r validation.Result, rule string) []validation.Issue {
	var out []validation.Issue
	for _, iss := range r.Issues {
		if iss.Rule == rule {
			out = append(out, iss)
		}
	}
	return out
}

// REQ-112 — the floor reports a list that COMPOSITION Content_valid,
// EVENT_CONTEXT Participations_validity, INSTRUCTION Activities_valid,
// SECTION Items_valid or ATTESTATION Items_valid makes non-empty once
// present, when it is present and holds no item, as rm_invariant at the
// list's own path with the invariant as its rule, at the root and nested.
// It reports none for an absent list or one that holds an item.
func TestValidateRM_REQ112_NonEmptyLists(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		root any
		rule string
		// want is what the floor reports under rule, as identityKeys writes
		// it.
		want []string
	}{
		{
			name: "COMPOSITION content present and empty",
			root: &rm.Composition{Content: []rm.ContentItem{}},
			rule: "COMPOSITION.Content_valid",
			want: []string{"rm_invariant /content rm COMPOSITION.Content_valid"},
		},
		{name: "COMPOSITION content absent", root: &rm.Composition{}, rule: "COMPOSITION.Content_valid"},
		{
			name: "COMPOSITION content holding an item",
			root: &rm.Composition{Content: []rm.ContentItem{&rm.Section{ArchetypeNodeID: "at0001"}}},
			rule: "COMPOSITION.Content_valid",
		},
		{
			name: "EVENT_CONTEXT participations present and empty",
			root: &rm.EventContext{Participations: []rm.Participation{}},
			rule: "EVENT_CONTEXT.Participations_validity",
			want: []string{"rm_invariant /participations rm EVENT_CONTEXT.Participations_validity"},
		},
		{
			name: "EVENT_CONTEXT participations present and empty under a COMPOSITION",
			root: &rm.Composition{Context: &rm.EventContext{Participations: []rm.Participation{}}},
			rule: "EVENT_CONTEXT.Participations_validity",
			want: []string{"rm_invariant /context/participations rm EVENT_CONTEXT.Participations_validity"},
		},
		{name: "EVENT_CONTEXT participations absent", root: &rm.EventContext{}, rule: "EVENT_CONTEXT.Participations_validity"},
		{
			name: "EVENT_CONTEXT participations holding an item",
			root: &rm.EventContext{Participations: []rm.Participation{validParticipation()}},
			rule: "EVENT_CONTEXT.Participations_validity",
		},
		{
			name: "INSTRUCTION activities present and empty",
			root: &rm.Instruction{Activities: []rm.Activity{}},
			rule: "INSTRUCTION.Activities_valid",
			want: []string{"rm_invariant /activities rm INSTRUCTION.Activities_valid"},
		},
		{name: "INSTRUCTION activities absent", root: &rm.Instruction{}, rule: "INSTRUCTION.Activities_valid"},
		{
			name: "INSTRUCTION activities holding an item",
			root: &rm.Instruction{Activities: []rm.Activity{{ArchetypeNodeID: "at0001"}}},
			rule: "INSTRUCTION.Activities_valid",
		},
		{
			name: "SECTION items present and empty",
			root: &rm.Section{Items: []rm.ContentItem{}},
			rule: "SECTION.Items_valid",
			want: []string{"rm_invariant /items rm SECTION.Items_valid"},
		},
		{name: "SECTION items absent", root: &rm.Section{}, rule: "SECTION.Items_valid"},
		{
			name: "SECTION items holding an item",
			root: &rm.Section{Items: []rm.ContentItem{&rm.Section{ArchetypeNodeID: "at0001"}}},
			rule: "SECTION.Items_valid",
		},
		{
			name: "ATTESTATION items present and empty",
			root: &rm.Attestation{Items: []rm.DVEHRURI{}},
			rule: "ATTESTATION.Items_valid",
			want: []string{"rm_invariant /items rm ATTESTATION.Items_valid"},
		},
		{name: "ATTESTATION items absent", root: &rm.Attestation{}, rule: "ATTESTATION.Items_valid"},
		{
			name: "ATTESTATION items holding an item",
			root: &rm.Attestation{Items: []rm.DVEHRURI{{Value: "ehr://example.org/x"}}},
			rule: "ATTESTATION.Items_valid",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := identityKeys(withRule(validation.ValidateRM(tc.root), tc.rule)); !slices.Equal(got, tc.want) {
				t.Errorf("ValidateRM(%s) issues under %s = %q, want %q (REQ-112 Non-empty lists)", tc.name, tc.rule, got, tc.want)
			}
		})
	}
}

// REQ-112 — HISTORY Events_valid, (events /= Void and then not
// events.is_empty) or summary /= Void: the floor reports a HISTORY whose
// events is absent or holds no item and whose summary is absent, as
// rm_invariant at the HISTORY's own path, at the root and as an
// OBSERVATION's data, and reports none for a HISTORY that carries an event
// or a summary.
func TestValidateRM_REQ112_HistoryEventsOrSummary(t *testing.T) {
	t.Parallel()
	const rule = "HISTORY.Events_valid"
	tests := []struct {
		name string
		root any
		want []string
	}{
		{name: "no events and no summary", root: rmHistory(nil, nil), want: []string{"rm_invariant / rm " + rule}},
		{name: "events present and empty, no summary", root: rmHistory([]rm.Event{}, nil), want: []string{"rm_invariant / rm " + rule}},
		{name: "an event", root: rmHistory([]rm.Event{pointEvent()}, nil)},
		{name: "a summary", root: rmHistory(nil, summaryTree())},
		{name: "events present and empty, a summary", root: rmHistory([]rm.Event{}, summaryTree())},
		{
			name: "an OBSERVATION's data with neither",
			root: &rm.Observation{Data: *rmHistory(nil, nil)},
			want: []string{"rm_invariant /data rm " + rule},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := identityKeys(withRule(validation.ValidateRM(tc.root), rule)); !slices.Equal(got, tc.want) {
				t.Errorf("ValidateRM(HISTORY with %s) issues under %s = %q, want %q (REQ-112 Non-empty lists)", tc.name, rule, got, tc.want)
			}
		})
	}
}

// participationsExistenceOPT is the shape of the Referral template the CDR
// cross-check met: a COMPOSITION whose context's participations carry the
// existence interval body existence, the cardinality interval body
// cardinality, and one PARTICIPATION child at occurrences 0..*.
func participationsExistenceOPT(existence, cardinality string) string {
	return `<?xml version="1.0" encoding="utf-8"?>
<template xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xmlns="http://schemas.openehr.org/v1">
<language><terminology_id><value>ISO_639-1</value></terminology_id><code_string>en</code_string></language>
<template_id><value>participations</value></template_id><concept>participations</concept>
<definition><rm_type_name>COMPOSITION</rm_type_name><node_id>at0000</node_id>
<attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>context</rm_attribute_name>
<children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>EVENT_CONTEXT</rm_type_name><node_id></node_id>
<attributes xsi:type="C_MULTIPLE_ATTRIBUTE"><rm_attribute_name>participations</rm_attribute_name>
<existence>` + existence + `</existence>
<children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>PARTICIPATION</rm_type_name>
<occurrences>` + fromLowerUnbounded("0") + `</occurrences><node_id></node_id></children>
<cardinality><is_ordered>false</is_ordered><is_unique>false</is_unique><interval>` + cardinality + `</interval></cardinality>
</attributes></children></attributes>
<archetype_id><value>openEHR-EHR-COMPOSITION.participations.v1</value></archetype_id></definition>
</template>`
}

// REQ-102, REQ-171 — the Referral shape: EVENT_CONTEXT participations at
// existence 1..1, cardinality 0..*, over a PARTICIPATION child at
// occurrences 0..*. A composition that leaves participations out, absent or
// present and empty, draws the template's `required` there once, and no
// Participations_validity beside it; one participation keeps both. At a
// cardinality of 1..* the empty list draws the template's cardinality
// beside its `required`, and still no RM rule. Where the template leaves
// participations optional (existence 0..1), an absent list draws nothing
// and an empty one the RM rule alone, as the floor reports it.
func TestValidateComposition_REQ102_REQ171_ReferralParticipationsExistence(t *testing.T) {
	t.Parallel()
	required := fiInterval("1", "1")
	optional := fiInterval("0", "1")
	zeroOrMore := fromLowerUnbounded("0")
	oneOrMore := fromLowerUnbounded("1")
	const at = "/context/participations"
	tests := []struct {
		name                   string
		existence, cardinality string
		// parts is the context's participations: nil for an absent list.
		parts []rm.Participation
		want  []string
	}{
		{name: "required, absent", existence: required, cardinality: zeroOrMore, want: []string{"required " + at + " template existence"}},
		{name: "required, present and empty", existence: required, cardinality: zeroOrMore, parts: []rm.Participation{}, want: []string{"required " + at + " template existence"}},
		{name: "required, one participation", existence: required, cardinality: zeroOrMore, parts: []rm.Participation{validParticipation()}},
		{
			name: "required at cardinality 1..*, present and empty", existence: required, cardinality: oneOrMore, parts: []rm.Participation{},
			want: []string{"cardinality " + at + " template cardinality", "required " + at + " template existence"},
		},
		{name: "optional, absent", existence: optional, cardinality: zeroOrMore},
		{
			name: "optional, present and empty", existence: optional, cardinality: zeroOrMore, parts: []rm.Participation{},
			want: []string{"rm_invariant " + at + " rm EVENT_CONTEXT.Participations_validity"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := mustCompileInline(t, participationsExistenceOPT(tc.existence, tc.cardinality))
			comp := participationsComposition()
			comp.Context.Participations = tc.parts
			r := validation.ValidateComposition(comp, c)
			if got := identityKeys(r.Issues); !slices.Equal(got, tc.want) {
				t.Errorf("ValidateComposition(Referral shape, %s) issues = %q, want %q (REQ-102 An existence lower bound alone asks for no member, REQ-171 One walk)", tc.name, got, tc.want)
			}
		})
	}
}

// historyOPT returns a template whose HISTORY root carries events with the
// existence interval body existence and the cardinality interval body
// cardinality, over one POINT_EVENT at0002 at occurrences 0..*. The
// template says nothing of the summary.
func historyOPT(existence, cardinality string) string {
	return `<?xml version="1.0" encoding="utf-8"?>
<template xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xmlns="http://schemas.openehr.org/v1">
<language><terminology_id><value>ISO_639-1</value></terminology_id><code_string>en</code_string></language>
<template_id><value>nonempty_lists</value></template_id><concept>nonempty_lists</concept>
<definition><rm_type_name>HISTORY</rm_type_name><node_id>at0000</node_id>
<attributes xsi:type="C_MULTIPLE_ATTRIBUTE"><rm_attribute_name>events</rm_attribute_name>
<existence>` + existence + `</existence>
<children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>POINT_EVENT</rm_type_name>
<occurrences>` + fromLowerUnbounded("0") + `</occurrences><node_id>at0002</node_id></children>
<cardinality><is_ordered>true</is_ordered><is_unique>false</is_unique><interval>` + cardinality + `</interval></cardinality>
</attributes>
<archetype_id><value>` + historyArchetype + `</value></archetype_id></definition>
</template>`
}

// REQ-102, REQ-171 — a HISTORY's events at existence 1..1, cardinality
// 0..*, over a POINT_EVENT at occurrences 0..*: while the HISTORY has no
// summary, the template asks events for a member, so events absent or
// present and empty draws the template's `required` at /events once, and
// no Events_valid at the HISTORY's path; a summary keeps both, and an
// event keeps both. At a cardinality of 1..* the empty list draws the
// template's cardinality beside it, and still no RM rule. Where the template
// leaves events optional (existence 0..1), a HISTORY with neither an event
// nor a summary draws Events_valid alone, absent or empty.
func TestValidate_REQ102_REQ171_HistoryEventsExistence(t *testing.T) {
	t.Parallel()
	required := fiInterval("1", "1")
	optional := fiInterval("0", "1")
	zeroOrMore := fromLowerUnbounded("0")
	oneOrMore := fromLowerUnbounded("1")
	tests := []struct {
		name                   string
		existence, cardinality string
		root                   *rm.History[rm.ItemStructure]
		want                   []string
	}{
		{name: "required, absent, no summary", existence: required, cardinality: zeroOrMore, root: rmHistory(nil, nil), want: []string{"required /events template existence"}},
		{name: "required, present and empty, no summary", existence: required, cardinality: zeroOrMore, root: rmHistory([]rm.Event{}, nil), want: []string{"required /events template existence"}},
		{name: "required, absent, a summary", existence: required, cardinality: zeroOrMore, root: rmHistory(nil, summaryTree())},
		{name: "required, an event", existence: required, cardinality: zeroOrMore, root: rmHistory([]rm.Event{pointEvent()}, nil)},
		{
			name: "required at cardinality 1..*, present and empty, no summary", existence: required, cardinality: oneOrMore, root: rmHistory([]rm.Event{}, nil),
			want: []string{"cardinality /events template cardinality", "required /events template existence"},
		},
		{name: "optional, absent, no summary", existence: optional, cardinality: zeroOrMore, root: rmHistory(nil, nil), want: []string{"rm_invariant / rm HISTORY.Events_valid"}},
		{name: "optional, present and empty, no summary", existence: optional, cardinality: zeroOrMore, root: rmHistory([]rm.Event{}, nil), want: []string{"rm_invariant / rm HISTORY.Events_valid"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := mustCompileInline(t, historyOPT(tc.existence, tc.cardinality))
			r := validation.Validate(tc.root, c)
			if got := identityKeys(r.Issues); !slices.Equal(got, tc.want) {
				t.Errorf("Validate(HISTORY, %s) issues = %q, want %q (REQ-102 An existence lower bound alone asks for no member, REQ-171 One walk)", tc.name, got, tc.want)
			}
		})
	}
}

// REQ-171 — with no template the walk is the floor, so Validate(root, nil)
// on a Validator reports the non-empty list rules as ValidateRM does.
func TestValidator_REQ171_NonEmptyListsMatchTheFloor(t *testing.T) {
	t.Parallel()
	root := &rm.Composition{Content: []rm.ContentItem{}, Context: &rm.EventContext{Participations: []rm.Participation{}}}
	want := identityKeys(validation.ValidateRM(root).Issues)
	got := identityKeys(validation.New().Validate(root, nil).Issues)
	if !slices.Equal(got, want) {
		t.Errorf("Validate(root, nil) issues = %q, want ValidateRM's %q", got, want)
	}
	if !slices.ContainsFunc(got, func(k string) bool { return strings.HasSuffix(k, " rm COMPOSITION.Content_valid") }) {
		t.Errorf("Validate(root, nil) issues = %q, want one under COMPOSITION.Content_valid", got)
	}
}

// listAttribute returns a multi-valued attribute name with the existence
// interval body existence, the cardinality interval body cardinality and
// children, each a <children> element.
func listAttribute(name, existence, cardinality string, children ...string) string {
	return `<attributes xsi:type="C_MULTIPLE_ATTRIBUTE"><rm_attribute_name>` + name + `</rm_attribute_name>` +
		`<existence>` + existence + `</existence>` + strings.Join(children, "") +
		`<cardinality><is_ordered>false</is_ordered><is_unique>false</is_unique><interval>` + cardinality + `</interval></cardinality></attributes>`
}

// occurringChild returns a C_COMPLEX_OBJECT child of RM type rmType at
// nodeID with the occurrences interval body occurrences.
func occurringChild(rmType, nodeID, occurrences string) string {
	return `<children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>` + rmType + `</rm_type_name>` +
		`<occurrences>` + occurrences + `</occurrences><node_id>` + nodeID + `</node_id></children>`
}

// REQ-171 — one absence, one finding: a list that holds no item draws the
// template's finding alone where the template reports it at the list's path
// or below it, as the occurrences of a child the list allows, and, for a
// HISTORY's events, where it reports that HISTORY's summary. A template
// report on another attribute withdraws nothing: the RM rule stays beside
// it.
func TestValidate_REQ171_EmptyListGivesWayToATemplateReportBelowIt(t *testing.T) {
	t.Parallel()
	optional := fiInterval("0", "1")
	required := fiInterval("1", "1")
	zeroOrMore := fromLowerUnbounded("0")
	historyWith := func(attrs ...string) string { return rootOPT("HISTORY", historyArchetype, attrs...) }
	optionalEvents := listAttribute("events", optional, zeroOrMore, occurringChild("POINT_EVENT", "at0002", zeroOrMore))
	contentEmpty := participationsComposition()
	contentEmpty.Content = []rm.ContentItem{}
	tests := []struct {
		name string
		opt  string
		root any
		want []string
	}{
		{
			name: "HISTORY events optional over a POINT_EVENT at 1..1, events absent, no summary",
			opt:  historyWith(listAttribute("events", optional, zeroOrMore, occurringChild("POINT_EVENT", "at0002", required))),
			root: rmHistory(nil, nil),
			want: []string{"cardinality /events[at0002] template occurrences"},
		},
		{
			name: "HISTORY events optional over a POINT_EVENT at 1..1, events present and empty, no summary",
			opt:  historyWith(listAttribute("events", optional, zeroOrMore, occurringChild("POINT_EVENT", "at0002", required))),
			root: rmHistory([]rm.Event{}, nil),
			want: []string{"cardinality /events[at0002] template occurrences"},
		},
		{
			name: "COMPOSITION content optional over a SECTION at 1..1, content present and empty",
			opt: rootOPT("COMPOSITION", "openEHR-EHR-COMPOSITION.participations.v1",
				listAttribute("content", optional, zeroOrMore, occurringChild("SECTION", "at0001", required))),
			root: contentEmpty,
			want: []string{"cardinality /content[at0001] template occurrences"},
		},
		{
			name: "HISTORY events optional, summary required, both absent",
			opt:  historyWith(optionalEvents, singleAttribute("summary", "1", "1", occurringChild("ITEM_TREE", "at0004", required))),
			root: rmHistory(nil, nil),
			want: []string{"required /summary template existence"},
		},
		{
			name: "HISTORY events optional, summary required, events present and empty, summary absent",
			opt:  historyWith(optionalEvents, singleAttribute("summary", "1", "1", occurringChild("ITEM_TREE", "at0004", required))),
			root: rmHistory([]rm.Event{}, nil),
			want: []string{"required /summary template existence"},
		},
		{
			name: "HISTORY events optional, period required, no events and no summary",
			opt:  historyWith(optionalEvents, singleAttribute("period", "1", "1", occurringChild("DV_DURATION", "", required))),
			root: rmHistory(nil, nil),
			want: []string{"required /period template existence", "rm_invariant / rm HISTORY.Events_valid"},
		},
		{
			name: "COMPOSITION content present and empty, context participations required and absent",
			opt:  participationsExistenceOPT(required, zeroOrMore),
			root: contentEmpty,
			want: []string{"required /context/participations template existence", "rm_invariant /content rm COMPOSITION.Content_valid"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := mustCompileInline(t, tc.opt)
			if got := identityKeys(validation.Validate(tc.root, c).Issues); !slices.Equal(got, tc.want) {
				t.Errorf("Validate(%s) issues = %q, want %q (REQ-171 One walk, step 2)", tc.name, got, tc.want)
			}
		})
	}
}
