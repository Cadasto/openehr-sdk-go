package termgen

import (
	"slices"
	"strings"
	"testing"
)

// fixture is a two-entry stand-in for the pin: one code set, one group, one
// XML comment (the pin carries two of those, on code 532). Shared by the
// parser, renderer and Run tests so they all describe the same document.
const fixture = `<terminology name="openehr" language="en" version="9.9.9" date="2026-01-01">
	<codeset issuer="openehr" openehr_id="normal_statuses" name="normal statuses" external_id="openehr_normal_statuses">
		<code value="N"/><code value="H"/>
	</codeset>
	<group openehr_id="audit_change_type" name="audit change type">
		<concept id="249" rubric="creation"/>
		<concept id="523" rubric="deleted"/><!-- a comment -->
	</group>
</terminology>`

func TestParseReadsTheReleaseAndBothTables(t *testing.T) {
	t.Parallel()
	got, err := Parse(strings.NewReader(fixture))
	if err != nil {
		t.Fatalf("Parse(fixture) = _, %v; want no error", err)
	}
	if got.Name != "openehr" || got.Language != "en" || got.Version != "9.9.9" || got.Date != "2026-01-01" {
		t.Errorf("release metadata = %q/%q/%q/%q, want openehr/en/9.9.9/2026-01-01",
			got.Name, got.Language, got.Version, got.Date)
	}
	if len(got.CodeSets) != 1 || len(got.Groups) != 1 {
		t.Fatalf("Parse(fixture) = %d code sets, %d groups; want 1 and 1", len(got.CodeSets), len(got.Groups))
	}
	cs := got.CodeSets[0]
	if cs.ID != "normal_statuses" || cs.Name != "normal statuses" {
		t.Errorf("code set = %q/%q, want normal_statuses/normal statuses", cs.ID, cs.Name)
	}
	if !slices.Equal(cs.Codes, []string{"N", "H"}) {
		t.Errorf("code set codes = %q, want [N H] in source order", cs.Codes)
	}
	g := got.Groups[0]
	if g.ID != "audit_change_type" || g.Name != "audit change type" {
		t.Errorf("group = %q/%q, want audit_change_type/audit change type", g.ID, g.Name)
	}
	want := []ConceptDef{{Code: "249", Rubric: "creation"}, {Code: "523", Rubric: "deleted"}}
	if !slices.Equal(g.Concepts, want) {
		t.Errorf("group concepts = %v, want %v in source order (the XML comment must be ignored)", g.Concepts, want)
	}
}

// TestParseRefusesABrokenPin walks the invariants the generated tables rely
// on. Every case must come back as an error — never a panic — and the message
// must name the offending id, because that is all the maintainer running
// `make termgen` after a version bump gets to work from.
func TestParseRefusesABrokenPin(t *testing.T) {
	t.Parallel()
	const attrs = `name="openehr" language="en" version="9.9.9" date="2026-01-01"`
	doc := func(body string) string {
		return `<terminology ` + attrs + `>` + body + `</terminology>`
	}
	tests := []struct {
		name  string
		in    string
		wants []string // substrings the message must carry
	}{
		{
			name:  "not the openEHR terminology",
			in:    `<terminology name="local" language="en" version="9.9.9"></terminology>`,
			wants: []string{`"local"`, `"openehr"`},
		},
		{
			name:  "no version",
			in:    `<terminology name="openehr" language="en" version=""></terminology>`,
			wants: []string{"version"},
		},
		{
			name: "duplicate code in a group",
			in: doc(`<group openehr_id="audit_change_type" name="audit change type">
				<concept id="249" rubric="creation"/><concept id="249" rubric="amendment"/></group>`),
			wants: []string{`"audit_change_type"`, `"249"`},
		},
		{
			name: "duplicate rubric in a group",
			in: doc(`<group openehr_id="audit_change_type" name="audit change type">
				<concept id="249" rubric="creation"/><concept id="250" rubric="creation"/></group>`),
			wants: []string{`"audit_change_type"`, `"creation"`},
		},
		{
			name:  "group with no concepts",
			in:    doc(`<group openehr_id="empty_group" name="empty group"></group>`),
			wants: []string{`"empty_group"`},
		},
		{
			name: "concept with no rubric",
			in: doc(`<group openehr_id="audit_change_type" name="audit change type">
				<concept id="249"/></group>`),
			wants: []string{`"audit_change_type"`, `"249"`, "rubric"},
		},
		{
			name: "concept with no code",
			in: doc(`<group openehr_id="audit_change_type" name="audit change type">
				<concept rubric="creation"/></group>`),
			wants: []string{`"audit_change_type"`, "id"},
		},
		{
			name: "two groups sharing an openehr_id",
			in: doc(`<group openehr_id="setting" name="setting"><concept id="1" rubric="a"/></group>
				<group openehr_id="setting" name="setting again"><concept id="2" rubric="b"/></group>`),
			wants: []string{`"setting"`, "twice"},
		},
		{
			name: "openehr_id that is not lower-case snake_case",
			in: doc(`<group openehr_id="Audit_Change_Type" name="audit change type">
				<concept id="249" rubric="creation"/></group>`),
			wants: []string{`"Audit_Change_Type"`, `^[a-z][a-z0-9_]*$`},
		},
		{
			name: "two openehr_ids mangling to the same Go name",
			in: doc(`<group openehr_id="audit_change_type" name="one"><concept id="1" rubric="a"/></group>
				<group openehr_id="audit__change_type" name="two"><concept id="2" rubric="b"/></group>`),
			wants: []string{`"audit_change_type"`, `"audit__change_type"`, "AuditChangeType"},
		},
		{
			name: "duplicate code in a code set",
			in: doc(`<codeset openehr_id="normal_statuses" name="normal statuses">
				<code value="N"/><code value="N"/></codeset>`),
			wants: []string{`"normal_statuses"`, `"N"`},
		},
		{
			name:  "code set with no codes",
			in:    doc(`<codeset openehr_id="normal_statuses" name="normal statuses"></codeset>`),
			wants: []string{`"normal_statuses"`},
		},
		{
			name:  "code with no value",
			in:    doc(`<codeset openehr_id="normal_statuses" name="normal statuses"><code/></codeset>`),
			wants: []string{`"normal_statuses"`, "value"},
		},
		{
			name:  "a code set and a group sharing an openehr_id",
			in:    doc(`<codeset openehr_id="setting" name="setting"><code value="N"/></codeset><group openehr_id="setting" name="setting"><concept id="1" rubric="a"/></group>`),
			wants: []string{`"setting"`, "twice"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := Parse(strings.NewReader(tc.in))
			if err == nil {
				t.Fatalf("Parse(%s) = %+v, nil; want a refusal", tc.name, got)
			}
			for _, want := range tc.wants {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("Parse(%s) error = %q; want it to name %s", tc.name, err, want)
				}
			}
		})
	}
}

// TestParseRefusesAnotherRootElement is the can-fail control for the shape
// guard: a well-formed XML document that is not the terminology pin must not
// decode into empty tables.
func TestParseRefusesAnotherRootElement(t *testing.T) {
	t.Parallel()
	if _, err := Parse(strings.NewReader(`<archetype name="openehr" version="1"/>`)); err == nil {
		t.Error("Parse(<archetype>) = _, nil; want a refusal — only <terminology> is the pin")
	}
}
