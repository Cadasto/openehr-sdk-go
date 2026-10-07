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

// externalFixture is a stand-in for openehr_external_terminologies.xml: code
// sets only, no groups, each with an external issuer and a description
// attribute the generator ignores. It pairs with fixture as the two files of
// the pin.
const externalFixture = `<terminology name="openehr" language="en" version="9.9.9" date="2026-01-02">
	<codeset issuer="ISO" openehr_id="languages" name="languages" external_id="ISO_639-1">
		<code value="en" description="English"/><code value="en-us" description="English (United States)"/>
	</codeset>
	<codeset issuer="IANA" openehr_id="character_sets" name="character sets" external_id="IANA_character-sets">
		<code value="UTF-8"/>
	</codeset>
</terminology>`

// REQ-034: the external file carries code sets only; each one keeps the
// issuer and external id the pin gives it, and its codes in source order.
func TestParseReadsAnExternalShapedDocument(t *testing.T) {
	t.Parallel()
	got, err := Parse(strings.NewReader(externalFixture))
	if err != nil {
		t.Fatalf("Parse(externalFixture) = _, %v; want no error — a document of code sets alone is one half of the pin", err)
	}
	if len(got.Groups) != 0 {
		t.Errorf("Parse(externalFixture) = %d groups, want 0", len(got.Groups))
	}
	want := []CodeSetDef{
		{ID: "languages", Name: "languages", Issuer: "ISO", ExternalID: "ISO_639-1", Codes: []string{"en", "en-us"}},
		{ID: "character_sets", Name: "character sets", Issuer: "IANA", ExternalID: "IANA_character-sets", Codes: []string{"UTF-8"}},
	}
	if len(got.CodeSets) != len(want) {
		t.Fatalf("Parse(externalFixture) = %d code sets, want %d", len(got.CodeSets), len(want))
	}
	for i, w := range want {
		g := got.CodeSets[i]
		if g.ID != w.ID || g.Name != w.Name || g.Issuer != w.Issuer || g.ExternalID != w.ExternalID || !slices.Equal(g.Codes, w.Codes) {
			t.Errorf("code set %d = %+v, want %+v", i, g, w)
		}
	}
}

// REQ-034: an openEHR-issued code set is matched exactly, so two codes that
// differ only in letter case are two members, not a collision. The
// letter-case refusal is for the other issuers alone.
func TestParseAcceptsALetterCasePairInAnOpenEHRCodeSet(t *testing.T) {
	t.Parallel()
	const doc = `<terminology name="openehr" language="en" version="9.9.9">
		<codeset issuer="openehr" openehr_id="normal_statuses" name="normal statuses" external_id="openehr_normal_statuses">
			<code value="H"/><code value="h"/>
		</codeset>
	</terminology>`
	got, err := Parse(strings.NewReader(doc))
	if err != nil {
		t.Fatalf("Parse(openEHR code set with H and h) = _, %v; want no error — openEHR codes are compared exactly", err)
	}
	if codes := got.CodeSets[0].Codes; !slices.Equal(codes, []string{"H", "h"}) {
		t.Errorf("codes = %q, want [H h]", codes)
	}
}

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
	if cs.Issuer != "openehr" || cs.ExternalID != "openehr_normal_statuses" {
		t.Errorf("code set issuer/external id = %q/%q, want openehr/openehr_normal_statuses", cs.Issuer, cs.ExternalID)
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
	const normalStatuses = `issuer="openehr" openehr_id="normal_statuses" name="normal statuses" external_id="openehr_normal_statuses"`
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
			in: doc(`<codeset ` + normalStatuses + `>
				<code value="N"/><code value="N"/></codeset>`),
			wants: []string{`"normal_statuses"`, `"N"`},
		},
		{
			name:  "code set with no codes",
			in:    doc(`<codeset ` + normalStatuses + `></codeset>`),
			wants: []string{`"normal_statuses"`},
		},
		{
			name:  "code with no value",
			in:    doc(`<codeset ` + normalStatuses + `><code/></codeset>`),
			wants: []string{`"normal_statuses"`, "value"},
		},
		{
			name:  "a code set and a group sharing an openehr_id",
			in:    doc(`<codeset issuer="openehr" openehr_id="setting" name="setting" external_id="openehr_setting"><code value="N"/></codeset><group openehr_id="setting" name="setting"><concept id="1" rubric="a"/></group>`),
			wants: []string{`"setting"`, "twice"},
		},
		{
			name:  "code set with no issuer",
			in:    doc(`<codeset openehr_id="languages" name="languages" external_id="ISO_639-1"><code value="en"/></codeset>`),
			wants: []string{`"languages"`, "issuer"},
		},
		{
			name:  "code set with no external_id",
			in:    doc(`<codeset issuer="ISO" openehr_id="languages" name="languages"><code value="en"/></codeset>`),
			wants: []string{`"languages"`, "external_id"},
		},
		{
			// Has ignores letter case for an issuer other than openEHR, so
			// these two codes would be one member: which one All yields
			// would no longer say what Has accepts.
			name: "two codes of an ISO code set differing only in letter case",
			in: doc(`<codeset issuer="ISO" openehr_id="languages" name="languages" external_id="ISO_639-1">
				<code value="en-us"/><code value="fr"/><code value="en-US"/></codeset>`),
			wants: []string{`"languages"`, `"en-us"`, `"en-US"`, "letter case"},
		},
		{
			name: "two codes of an IANA code set differing only in letter case",
			in: doc(`<codeset issuer="IANA" openehr_id="character_sets" name="character sets" external_id="IANA_character-sets">
				<code value="UTF-8"/><code value="utf-8"/></codeset>`),
			wants: []string{`"character_sets"`, `"UTF-8"`, `"utf-8"`, "letter case"},
		},
		{
			// A document whose tables were all renamed upstream would
			// otherwise drop out of the pin without a word.
			name:  "a document with no groups and no code sets",
			in:    doc(``),
			wants: []string{"no groups and no code sets"},
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

// TestParseRefusesTrailingContent is the can-fail control for the single-root
// guard: encoding/xml stops at the first element, so a second top-level element
// after the terminology root must be refused, not silently dropped — otherwise
// it would slip past the sha256 pin at a version bump. The second root is
// appended to the otherwise valid fixture, so the trailing element is the only
// reason Parse can refuse the document: with the EOF guard deleted, the fixture
// parses cleanly and the nil-error arm fails on its own, not by riding an
// unrelated refusal.
func TestParseRefusesTrailingContent(t *testing.T) {
	t.Parallel()
	_, err := Parse(strings.NewReader(fixture + `<codeset/>`))
	if err == nil {
		t.Fatal("Parse(fixture + a second root) = _, nil; want a refusal — the pin holds exactly one terminology element")
	}
	if !strings.Contains(err.Error(), "trailing") {
		t.Errorf("Parse(fixture + a second root) error = %q, want it to name the trailing element", err)
	}
}

// mustParse parses one fixture document or stops the test.
func mustParse(t *testing.T, doc string) *Terminology {
	t.Helper()
	term, err := Parse(strings.NewReader(doc))
	if err != nil {
		t.Fatalf("Parse(fixture) = _, %v", err)
	}
	return term
}

// REQ-034: the two files make one pin. The groups and code sets of
// openehr_terminology.xml come first, then the code sets of
// openehr_external_terminologies.xml, each file in its document order.
func TestMergeJoinsTheTwoFilesInSourceOrder(t *testing.T) {
	t.Parallel()
	got, err := Merge(mustParse(t, fixture), mustParse(t, externalFixture))
	if err != nil {
		t.Fatalf("Merge(fixture, externalFixture) = _, %v; want no error", err)
	}
	if got.Name != "openehr" || got.Language != "en" || got.Version != "9.9.9" || got.Date != "2026-01-01" {
		t.Errorf("release metadata = %q/%q/%q/%q, want the terminology file's openehr/en/9.9.9/2026-01-01",
			got.Name, got.Language, got.Version, got.Date)
	}
	var ids []string
	for _, cs := range got.CodeSets {
		ids = append(ids, cs.ID)
	}
	if want := []string{"normal_statuses", "languages", "character_sets"}; !slices.Equal(ids, want) {
		t.Errorf("merged code sets = %q, want %q", ids, want)
	}
	if len(got.Groups) != 1 || got.Groups[0].ID != "audit_change_type" {
		t.Errorf("merged groups = %+v, want the one group of the terminology file", got.Groups)
	}
}

// REQ-034: both files come from one TERM release, so a pair naming two
// releases is refused rather than generated under one version.
func TestMergeRefusesTwoReleases(t *testing.T) {
	t.Parallel()
	external := strings.Replace(externalFixture, `version="9.9.9"`, `version="9.9.8"`, 1)
	_, err := Merge(mustParse(t, fixture), mustParse(t, external))
	if err == nil {
		t.Fatal("Merge(version 9.9.9, version 9.9.8) = _, nil; want a refusal — both files must come from one release")
	}
	for _, want := range []string{`"9.9.9"`, `"9.9.8"`} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("Merge error = %q, want it to name version %s", err, want)
		}
	}
}

// REQ-034: every table of the pin becomes one package-level variable, so an
// openehr_id one file claims cannot be claimed again by the other file,
// whether verbatim or through a name that mangles to the same Go identifier.
func TestMergeRefusesAGoNameClaimedInBothFiles(t *testing.T) {
	t.Parallel()
	const attrs = `name="openehr" language="en" version="9.9.9"`
	tests := []struct {
		name     string
		external string
		wants    []string
	}{
		{
			name: "an openehr_id in both files",
			external: `<terminology ` + attrs + `><codeset issuer="ISO" openehr_id="normal_statuses" name="n" external_id="ISO_x">
				<code value="a"/></codeset></terminology>`,
			wants: []string{`"normal_statuses"`, "twice"},
		},
		{
			name: "two openehr_ids, one per file, mangling to the same Go name",
			external: `<terminology ` + attrs + `><codeset issuer="ISO" openehr_id="audit__change_type" name="n" external_id="ISO_x">
				<code value="a"/></codeset></terminology>`,
			wants: []string{`"audit_change_type"`, `"audit__change_type"`, "AuditChangeType"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := Merge(mustParse(t, fixture), mustParse(t, tc.external))
			if err == nil {
				t.Fatalf("Merge(%s) = %+v, nil; want a refusal", tc.name, got)
			}
			for _, want := range tc.wants {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("Merge(%s) error = %q; want it to name %s", tc.name, err, want)
				}
			}
		})
	}
}

// TestMergeRefusesAnEmptyVocabulary is the can-fail control for the
// at-least-one-of-each guard: a pin that ends up with zero groups or zero code
// sets (an upstream rename that emptied a table) must be refused, not
// generated into a silently incomplete vocabulary. The two one-sided rows pin
// the *each*: a guard weakened to at-least-one-of-either passes them and fails
// here, and every refusal must name the table that is empty. The guard holds
// over the two files together, since the external file carries no groups.
//
// REQ-034: the accessor must expose every group and every code set of the
// pin, so a pin that parses to an empty table is refused rather than
// generated.
func TestMergeRefusesAnEmptyVocabulary(t *testing.T) {
	t.Parallel()
	release := func(groups []GroupDef, codeSets []CodeSetDef) *Terminology {
		return &Terminology{Name: "openehr", Version: "9.9.9", Groups: groups, CodeSets: codeSets}
	}
	oneGroup := []GroupDef{{ID: "audit_change_type", Name: "audit change type", Concepts: []ConceptDef{{Code: "249", Rubric: "creation"}}}}
	oneCodeSet := []CodeSetDef{{ID: "languages", Name: "languages", Issuer: "ISO", ExternalID: "ISO_639-1", Codes: []string{"en"}}}
	tests := []struct {
		name      string
		core, ext *Terminology
		want      string
	}{
		// Each want is the full count pair, never a one-sided substring:
		// "0 code set(s)" alone also matches the both-empty message, so a
		// row could otherwise pass on the wrong refusal.
		{name: "no groups and no code sets", core: release(nil, nil), ext: release(nil, nil), want: "0 group(s) and 0 code set(s)"},
		{name: "groups but no code sets", core: release(oneGroup, nil), ext: release(nil, nil), want: "1 group(s) and 0 code set(s)"},
		{name: "code sets but no groups", core: release(nil, nil), ext: release(nil, oneCodeSet), want: "0 group(s) and 1 code set(s)"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := Merge(tc.core, tc.ext)
			if err == nil {
				t.Fatalf("Merge(%s) = _, nil; want a refusal — the pin must carry at least one group and one code set", tc.name)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("Merge(%s) error = %q, want it to name the empty table (%q)", tc.name, err, tc.want)
			}
		})
	}
}
