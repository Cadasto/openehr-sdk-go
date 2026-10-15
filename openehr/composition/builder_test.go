package composition_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/composition"
	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/rmpath"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

func compileFixture(t *testing.T, name string) *templatecompile.Compiled {
	t.Helper()
	opt, err := template.ParseFile(fixtures.TemplateOptForName(name))
	if err != nil {
		t.Fatalf("ParseFile %s: %v", name, err)
	}
	c, err := templatecompile.Compile(opt)
	if err != nil {
		t.Fatalf("Compile %s: %v", name, err)
	}
	return c
}

// testComposer returns a stable *rm.PartyIdentified fixture. The
// Name field is *string in the generated RM, so callers can't inline
// a struct literal — this helper centralises the conversion.
func testComposer() *rm.PartyIdentified {
	name := "Test Composer"
	return &rm.PartyIdentified{Name: &name}
}

// Systolic DV_QUANTITY path under vital_signs blood_pressure
// observation. Confirmed by compile-time inspection of the fixture.
const systolicPath = "/content[openEHR-EHR-OBSERVATION.blood_pressure.v1]/data/events[at0006]/data/items[at0004]/value"

const diastolicPath = "/content[openEHR-EHR-OBSERVATION.blood_pressure.v1]/data/events[at0006]/data/items[at0005]/value"

// TestNewSkeleton_vitalSigns asserts NewSkeleton produces a valid
// COMPOSITION with category, language, territory, composer, and a
// non-empty content slice.
func TestNewSkeleton_vitalSigns(t *testing.T) {
	c := compileFixture(t, "vital_signs")
	comp, err := composition.NewSkeleton(context.Background(), c,
		composition.WithTerritory("NL"),
		composition.WithComposer(testComposer()),
	)
	if err != nil {
		t.Fatalf("NewSkeleton: %v", err)
	}
	if comp.Category.DefiningCode.CodeString != "433" {
		t.Errorf("Category code = %q, want 433", comp.Category.DefiningCode.CodeString)
	}
	if comp.Territory.CodeString != "NL" {
		t.Errorf("Territory code = %q, want NL", comp.Territory.CodeString)
	}
	if comp.Composer == nil {
		t.Error("Composer is nil")
	}
	if comp.Language.CodeString == "" {
		t.Error("Language code is empty")
	}
	if comp.Context == nil || comp.Context.StartTime.Value == "" {
		t.Error("Context.StartTime is empty")
	}
	if len(comp.Content) == 0 {
		t.Error("Content empty — vital_signs OPT pins multiple OBSERVATIONs")
	}
}

// TestNewSkeleton_requiresComposer asserts that omitting WithComposer
// surfaces instance.ErrComposerRequired (wrapped).
func TestNewSkeleton_requiresComposer(t *testing.T) {
	c := compileFixture(t, "vital_signs")
	_, err := composition.NewSkeleton(context.Background(), c,
		composition.WithTerritory("NL"),
	)
	if err == nil {
		t.Fatal("expected error for missing Composer, got nil")
	}
}

// unnamedEntryOPT is a COMPOSITION template whose one content entry is an
// OBSERVATION the template names no archetype for.
const unnamedEntryOPT = `<?xml version="1.0" encoding="utf-8"?>
<template xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xmlns="http://schemas.openehr.org/v1">
<language><terminology_id><value>ISO_639-1</value></terminology_id><code_string>en</code_string></language>
<template_id><value>unnamed_entry</value></template_id><concept>unnamed_entry</concept>
<definition><rm_type_name>COMPOSITION</rm_type_name><node_id>at0000</node_id>
<attributes xsi:type="C_MULTIPLE_ATTRIBUTE"><rm_attribute_name>content</rm_attribute_name>
<children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>OBSERVATION</rm_type_name><node_id>at0000</node_id></children>
<cardinality><is_ordered>false</is_ordered><is_unique>false</is_unique><interval><lower_included>true</lower_included><lower_unbounded>false</lower_unbounded><upper_unbounded>true</upper_unbounded><lower>1</lower></interval></cardinality>
</attributes>
<archetype_id><value>openEHR-EHR-COMPOSITION.encounter.v1</value></archetype_id></definition>
</template>`

// TestREQ101_REQ107_BuilderRefusesUnnamedArchetypeRoot checks that
// NewSkeleton and NewBuilder, which build through instance.Generate, pass
// on its refusal of an archetype-root object the template names no
// archetype for, and return nothing with it.
func TestREQ101_REQ107_BuilderRefusesUnnamedArchetypeRoot(t *testing.T) {
	opt, err := template.ParseOPT(strings.NewReader(unnamedEntryOPT))
	if err != nil {
		t.Fatalf("ParseOPT: %v", err)
	}
	c, err := templatecompile.Compile(opt)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	opts := []composition.Option{composition.WithTerritory("NL"), composition.WithComposer(testComposer())}

	comp, err := composition.NewSkeleton(t.Context(), c, opts...)
	if !errors.Is(err, instance.ErrArchetypeIDMissing) {
		t.Errorf("NewSkeleton error = %v, want one wrapping instance.ErrArchetypeIDMissing", err)
	}
	if comp != nil {
		t.Error("NewSkeleton returned a composition with the error, want nil")
	}
	b, err := composition.NewBuilder(t.Context(), c, opts...)
	if !errors.Is(err, instance.ErrArchetypeIDMissing) {
		t.Errorf("NewBuilder error = %v, want one wrapping instance.ErrArchetypeIDMissing", err)
	}
	if b != nil {
		t.Error("NewBuilder returned a builder with the error, want nil")
	}
}

// TestBuilder_SetQuantity_systolic exercises the canonical
// path-first authoring case: load OPT, NewBuilder, SetQuantity at
// systolic and diastolic DV_QUANTITY paths, Build, marshal via
// canjson, and confirm the magnitude / units survive the round-trip.
func TestBuilder_SetQuantity_systolic(t *testing.T) {
	c := compileFixture(t, "vital_signs")
	b, err := composition.NewBuilder(context.Background(), c,
		composition.WithTerritory("NL"),
		composition.WithComposer(testComposer()),
	)
	if err != nil {
		t.Fatalf("NewBuilder: %v", err)
	}
	if err := b.SetQuantity(systolicPath, 120, "mm[Hg]"); err != nil {
		t.Fatalf("SetQuantity systolic: %v", err)
	}
	if err := b.SetQuantity(diastolicPath, 80, "mm[Hg]"); err != nil {
		t.Fatalf("SetQuantity diastolic: %v", err)
	}
	comp, err := b.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	// Probe the in-memory graph first — cheaper than a round-trip
	// to confirm the assignment landed.
	sys, dia := findSystolicDiastolic(comp)
	if sys == nil {
		t.Fatal("systolic DV_QUANTITY not found in built composition")
	}
	if float64(sys.Magnitude) != 120 || sys.Units != "mm[Hg]" {
		t.Errorf("systolic = %v %q, want 120 mm[Hg]", sys.Magnitude, sys.Units)
	}
	if dia == nil {
		t.Fatal("diastolic DV_QUANTITY not found in built composition")
	}
	if float64(dia.Magnitude) != 80 {
		t.Errorf("diastolic magnitude = %v, want 80", dia.Magnitude)
	}

	// Confirm canjson Marshal preserves the values in the output
	// bytes (the leaf path / value reaches the wire). Full Marshal
	// → Unmarshal round-trip is exercised in PROBE-023; some root-
	// level identity fields (UID) currently marshal without the
	// canjson `_type` discriminator (see instance.newHierObjectID),
	// so the round-trip Unmarshal there clears UID and Composer
	// before re-decoding.
	payload, err := canjson.Marshal(comp)
	if err != nil {
		t.Fatalf("canjson.Marshal: %v", err)
	}
	if !bytes.Contains(payload, []byte(`"magnitude":120`)) {
		t.Errorf("canjson output missing systolic magnitude; payload (first 400 bytes): %s", trim(payload))
	}
	if !bytes.Contains(payload, []byte(`"units":"mm[Hg]"`)) {
		t.Errorf("canjson output missing units")
	}
}

// TestBuilder_SetText_typeMismatch confirms SetText's wrap into
// &rm.DVText surfaces a clean ErrTypeMismatch when targeted at a
// non-DV_TEXT path (the vital_signs OPT pins no DV_TEXT primitive
// leaves, so the positive case is exercised end-to-end by PROBE-023
// on an OPT that does — vital_signs systolic is the canonical
// DV_QUANTITY case for SetQuantity).
func TestBuilder_SetText_typeMismatch(t *testing.T) {
	c := compileFixture(t, "vital_signs")
	b, err := composition.NewBuilder(context.Background(), c,
		composition.WithTerritory("NL"),
		composition.WithComposer(testComposer()),
	)
	if err != nil {
		t.Fatalf("NewBuilder: %v", err)
	}
	if err := b.SetText(systolicPath, "wrong"); !errors.Is(err, composition.ErrTypeMismatch) {
		t.Errorf("SetText at DV_QUANTITY path: want ErrTypeMismatch, got %v", err)
	}
}

// TestBuilder_SetCodedText_typeMismatch confirms SetCodedText's
// wrap surfaces ErrTypeMismatch on a DV_QUANTITY path, a type
// DV_CODED_TEXT does not descend from.
func TestBuilder_SetCodedText_typeMismatch(t *testing.T) {
	c := compileFixture(t, "vital_signs")
	b, err := composition.NewBuilder(context.Background(), c,
		composition.WithTerritory("NL"),
		composition.WithComposer(testComposer()),
	)
	if err != nil {
		t.Fatalf("NewBuilder: %v", err)
	}
	if err := b.SetCodedText(systolicPath, "openehr", "at0000", "x"); !errors.Is(err, composition.ErrTypeMismatch) {
		t.Errorf("SetCodedText at DV_QUANTITY path: want ErrTypeMismatch, got %v", err)
	}
}

// TestBuilder_Set_typeMismatch asserts ErrTypeMismatch when a
// *rm.DVText is supplied at a DV_QUANTITY path.
//
// REQ-101: a Go value that does not fit the compiled node's RM type is
// refused with ErrTypeMismatch.
func TestBuilder_Set_typeMismatch(t *testing.T) {
	c := compileFixture(t, "vital_signs")
	b, err := composition.NewBuilder(context.Background(), c,
		composition.WithTerritory("NL"),
		composition.WithComposer(testComposer()),
	)
	if err != nil {
		t.Fatalf("NewBuilder: %v", err)
	}
	err = b.Set(systolicPath, &rm.DVText{Value: "wrong"})
	if !errors.Is(err, composition.ErrTypeMismatch) {
		t.Errorf("expected ErrTypeMismatch, got %v", err)
	}
}

// TestBuilder_Set_unknownPath asserts ErrUnknownPath for a path the
// OPT does not contain.
func TestBuilder_Set_unknownPath(t *testing.T) {
	c := compileFixture(t, "vital_signs")
	b, err := composition.NewBuilder(context.Background(), c,
		composition.WithTerritory("NL"),
		composition.WithComposer(testComposer()),
	)
	if err != nil {
		t.Fatalf("NewBuilder: %v", err)
	}
	err = b.Set("/no/such/path", &rm.DVText{Value: "x"})
	if !errors.Is(err, composition.ErrUnknownPath) {
		t.Errorf("expected ErrUnknownPath, got %v", err)
	}
	// PR #19 deferred suggestion: the error message should carry the
	// path string AND the template-side cause for callers diagnosing
	// from logs. Wrapping NodeAt's typed cause was added in the
	// follow-up; pin its diagnostic content here.
	if msg := err.Error(); !strings.Contains(msg, "/no/such/path") {
		t.Errorf("error missing path string: %v", err)
	}
	// PR #20 re-review (Important 1): the multi-%w wrap means
	// errors.Is reaches the templatecompile sentinel too — the
	// compiled-path API uses its own ErrPathNotFound distinct from
	// template.ErrPathNotFound by design (see
	// internal/templatecompile/compiled.go). Pin it so a future
	// refactor that drops the inner %w surfaces here.
	if !errors.Is(err, templatecompile.ErrPathNotFound) {
		t.Errorf("expected errors.Is to reach templatecompile.ErrPathNotFound, got %v", err)
	}
}

// TestBuilder_Build_AggregatesErrors confirms that two bad Set calls
// + one good Set surface as a single joined error from Build, with
// each per-path failure recoverable via errors.Is.
//
// REQ-101: Set does not short-circuit; Build returns every faulty path joined
// in one error.
func TestBuilder_Build_AggregatesErrors(t *testing.T) {
	c := compileFixture(t, "vital_signs")
	b, err := composition.NewBuilder(context.Background(), c,
		composition.WithTerritory("NL"),
		composition.WithComposer(testComposer()),
	)
	if err != nil {
		t.Fatalf("NewBuilder: %v", err)
	}
	// Two failures + one success — Set returns the per-call error
	// (typed), Build joins all into one returned error.
	_ = b.Set("/no/such/path", &rm.DVText{Value: "x"})
	_ = b.SetQuantity(systolicPath, 120, "mm[Hg]") // valid
	_ = b.Set("/also/missing", &rm.DVText{Value: "y"})

	_, err = b.Build()
	if err == nil {
		t.Fatal("expected aggregated error from Build, got nil")
	}
	if !errors.Is(err, composition.ErrUnknownPath) {
		t.Errorf("expected joined error to surface ErrUnknownPath, got %v", err)
	}
	// Both bad-path strings should appear in the joined diagnostic.
	msg := err.Error()
	if !strings.Contains(msg, "/no/such/path") || !strings.Contains(msg, "/also/missing") {
		t.Errorf("joined error missing one of the two bad paths: %v", err)
	}
}

// TestBuilder_Build_Idempotent confirms a second Build with no new
// Set calls returns a nil error — accumulated errors from the first
// Build are not replayed (PR #19 review suggestion: drain state
// between Build passes for chained authoring).
func TestBuilder_Build_Idempotent(t *testing.T) {
	c := compileFixture(t, "vital_signs")
	b, err := composition.NewBuilder(context.Background(), c,
		composition.WithTerritory("NL"),
		composition.WithComposer(testComposer()),
	)
	if err != nil {
		t.Fatalf("NewBuilder: %v", err)
	}
	// First pass fails on a bogus path.
	_ = b.Set("/no/such/path", &rm.DVText{Value: "x"})
	if _, err := b.Build(); err == nil {
		t.Fatal("first Build: expected error, got nil")
	}
	// Second pass with no new Set must NOT replay the first pass's
	// errors. Skeleton is returned unchanged with nil error.
	out, err := b.Build()
	if err != nil {
		t.Errorf("second Build replayed prior errors: %v", err)
	}
	if out == nil {
		t.Error("second Build returned nil skeleton")
	}
}

// TestBuilder_TemplateID asserts Builder.TemplateID matches the
// compiled template's id.
//
// REQ-101: TemplateID returns the compiled OPT's template id.
func TestBuilder_TemplateID(t *testing.T) {
	c := compileFixture(t, "vital_signs")
	b, err := composition.NewBuilder(context.Background(), c,
		composition.WithTerritory("NL"),
		composition.WithComposer(testComposer()),
	)
	if err != nil {
		t.Fatalf("NewBuilder: %v", err)
	}
	if got, want := b.TemplateID(), c.TemplateID(); got != want {
		t.Errorf("TemplateID = %q, want %q", got, want)
	}
}

// findSystolicDiastolic walks a built blood-pressure composition
// and returns the systolic + diastolic DV_QUANTITY values when both
// are present.
func findSystolicDiastolic(c *rm.Composition) (*rm.DVQuantity, *rm.DVQuantity) {
	var sys, dia *rm.DVQuantity
	for _, item := range c.Content {
		obs, ok := item.(*rm.Observation)
		if !ok {
			continue
		}
		if obs.ArchetypeNodeID != "openEHR-EHR-OBSERVATION.blood_pressure.v1" {
			continue
		}
		for _, ev := range obs.Data.Events {
			pe, ok := ev.(*rm.PointEvent[rm.ItemStructure])
			if !ok {
				continue
			}
			itl, ok := pe.Data.(*rm.ItemList)
			if !ok {
				continue
			}
			for i := range itl.Items {
				el := &itl.Items[i]
				dv, ok := el.Value.(*rm.DVQuantity)
				if !ok || dv == nil {
					continue
				}
				switch el.ArchetypeNodeID {
				case "at0004":
					sys = dv
				case "at0005":
					dia = dv
				}
			}
		}
	}
	return sys, dia
}

func trim(b []byte) string {
	if len(b) > 400 {
		return string(b[:400]) + "...(truncated)"
	}
	return string(b)
}

// Paths in the corpus template Test_dv_text_open_constraint.v0: an
// ELEMENT's DV_TEXT value, an ELEMENT's DV_TEXT name (the template fixes
// its value to "null") and the DV_CODED_TEXT category.
const (
	textValuePath = "/context/other_context/items[at0002]/value"
	textNamePath  = "/content[openEHR-EHR-OBSERVATION.test123.v0]/data/events[at0002]/data/items[at0031,'null']/name"
	categoryPath  = "/category"
)

// abstractValueOPT is a COMPOSITION template whose one ELEMENT, in the
// context's other_context, types its value as the abstract DATA_VALUE.
const abstractValueOPT = `<?xml version="1.0" encoding="utf-8"?>
<template xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xmlns="http://schemas.openehr.org/v1">
<language><terminology_id><value>ISO_639-1</value></terminology_id><code_string>en</code_string></language>
<template_id><value>abstract_value</value></template_id><concept>abstract_value</concept>
<definition><rm_type_name>COMPOSITION</rm_type_name><node_id>at0000</node_id>
<attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>context</rm_attribute_name>
<children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>EVENT_CONTEXT</rm_type_name><node_id></node_id>
<attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>other_context</rm_attribute_name>
<children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>ITEM_TREE</rm_type_name><node_id>at0001</node_id>
<attributes xsi:type="C_MULTIPLE_ATTRIBUTE"><rm_attribute_name>items</rm_attribute_name>
<children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>ELEMENT</rm_type_name><node_id>at0002</node_id>
<attributes xsi:type="C_SINGLE_ATTRIBUTE"><rm_attribute_name>value</rm_attribute_name>
<children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>DATA_VALUE</rm_type_name><node_id></node_id></children>
</attributes>
</children>
<cardinality><is_ordered>false</is_ordered><is_unique>false</is_unique><interval><lower_included>true</lower_included><lower_unbounded>false</lower_unbounded><upper_unbounded>true</upper_unbounded><lower>0</lower></interval></cardinality>
</attributes>
</children>
</attributes>
</children>
</attributes>
<archetype_id><value>openEHR-EHR-COMPOSITION.report.v1</value></archetype_id>
<term_definitions code="at0000"><items id="text">Report</items><items id="description">Report</items></term_definitions>
<term_definitions code="at0001"><items id="text">Tree</items><items id="description">Tree</items></term_definitions>
<term_definitions code="at0002"><items id="text">Reading</items><items id="description">Reading</items></term_definitions>
</definition>
</template>`

// abstractValuePath is the DATA_VALUE node of abstractValueOPT.
const abstractValuePath = "/context/other_context/items[at0002]/value"

// compileInline parses and compiles an inline operational template.
func compileInline(t *testing.T, opt string) *templatecompile.Compiled {
	t.Helper()
	parsed, err := template.ParseOPT(strings.NewReader(opt))
	if err != nil {
		t.Fatalf("ParseOPT: %v", err)
	}
	c, err := templatecompile.Compile(parsed)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	return c
}

// codedText returns a DV_CODED_TEXT showing value, coded in SNOMED-CT.
func codedText(value string) *rm.DVCodedText {
	return &rm.DVCodedText{
		Value:        value,
		DefiningCode: rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "SNOMED-CT"}, CodeString: "12345"},
	}
}

// newIssues returns, as "code at path", each issue of after that before
// does not report as often.
func newIssues(before, after validation.Result) []string {
	seen := make(map[string]int, len(before.Issues))
	for _, is := range before.Issues {
		seen[is.Code+" at "+is.Path]++
	}
	var added []string
	for _, is := range after.Issues {
		key := is.Code + " at " + is.Path
		if seen[key] > 0 {
			seen[key]--
			continue
		}
		added = append(added, key)
	}
	return added
}

// TestREQ101_SetAcceptsConformingValue checks that Set accepts a value
// whose RM type descends from the compiled node's, that Build writes it
// into the composition at that path, and that the validator reports no
// finding the composition did not have before: a DV_CODED_TEXT at an
// ELEMENT's DV_TEXT value and at an ELEMENT's DV_TEXT name, and a
// DV_QUANTITY at a value the template types as the abstract DATA_VALUE.
func TestREQ101_SetAcceptsConformingValue(t *testing.T) {
	t.Parallel()
	text := compileFixture(t, "Test_dv_text_open_constraint.v0")
	abstract := compileInline(t, abstractValueOPT)

	cases := []struct {
		name     string
		c        *templatecompile.Compiled
		path     string
		v        any
		wantType string
		// same reports whether Build stores v itself, not a copy.
		same bool
	}{
		{
			name: "DV_CODED_TEXT at an ELEMENT's DV_TEXT value", c: text, path: textValuePath,
			v: codedText("Lorem"), wantType: "DV_CODED_TEXT", same: true,
		},
		{
			name: "DV_CODED_TEXT at an ELEMENT's DV_TEXT name", c: text, path: textNamePath,
			v: codedText("null"), wantType: "DV_CODED_TEXT",
		},
		{
			name: "DV_QUANTITY at a DATA_VALUE value", c: abstract, path: abstractValuePath,
			v: &rm.DVQuantity{Magnitude: 120, Units: "mm[Hg]"}, wantType: "DV_QUANTITY", same: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			b, err := composition.NewBuilder(t.Context(), tc.c,
				composition.WithTerritory("NL"),
				composition.WithComposer(testComposer()),
			)
			if err != nil {
				t.Fatalf("NewBuilder: %v", err)
			}
			skeleton, err := b.Build()
			if err != nil {
				t.Fatalf("Build of the bare skeleton: %v", err)
			}
			before := validation.ValidateComposition(skeleton, tc.c)

			if err := b.Set(tc.path, tc.v); err != nil {
				t.Fatalf("Set(%s, %T) = %v, want nil", tc.path, tc.v, err)
			}
			comp, err := b.Build()
			if err != nil {
				t.Fatalf("Build after Set(%s, %T) = %v, want nil", tc.path, tc.v, err)
			}

			got, err := rmpath.ItemAtPath(comp, tc.path)
			if err != nil {
				t.Fatalf("ItemAtPath(%s) on the built composition: %v", tc.path, err)
			}
			if name, _ := rm.RMTypeName(got); name != tc.wantType {
				t.Errorf("built composition holds %T (RM %q) at %s, want %s", got, name, tc.path, tc.wantType)
			}
			if tc.same && got != tc.v {
				t.Errorf("built composition holds %v at %s, want the value Set was given, %v", got, tc.path, tc.v)
			}
			if text, ok := got.(rm.DVTextLike); ok {
				if code, coded := text.GetDefiningCode(); !coded || code.CodeString != "12345" {
					t.Errorf("built %s defining_code = %+v (coded %v), want SNOMED-CT::12345", tc.path, code, coded)
				}
			}

			after := validation.ValidateComposition(comp, tc.c)
			if added := newIssues(before, after); len(added) > 0 {
				t.Errorf("ValidateComposition after Set(%s, %T) reports new findings %q, want none", tc.path, tc.v, added)
			}
		})
	}
}

// TestREQ101_SetRefusesNonConformingValue checks that Set returns
// ErrTypeMismatch, and Build carries it, for a value whose RM type does
// not conform to the compiled node's: a DV_TEXT at a DV_QUANTITY node, a
// DV_TEXT at a DV_CODED_TEXT node (DV_TEXT does not descend from
// DV_CODED_TEXT), a typed-nil DV_CODED_TEXT, a nil and a value that is not
// an RM type.
func TestREQ101_SetRefusesNonConformingValue(t *testing.T) {
	t.Parallel()
	text := compileFixture(t, "Test_dv_text_open_constraint.v0")
	vitals := compileFixture(t, "vital_signs")

	cases := []struct {
		name string
		c    *templatecompile.Compiled
		path string
		v    any
	}{
		{name: "DV_TEXT at DV_QUANTITY", c: vitals, path: systolicPath, v: &rm.DVText{Value: "x"}},
		{name: "DV_TEXT at DV_CODED_TEXT", c: text, path: categoryPath, v: &rm.DVText{Value: "x"}},
		{name: "DV_QUANTITY at DV_TEXT", c: text, path: textValuePath, v: &rm.DVQuantity{Magnitude: 1, Units: "mm"}},
		{name: "typed-nil DV_CODED_TEXT at DV_TEXT", c: text, path: textValuePath, v: (*rm.DVCodedText)(nil)},
		{name: "nil at DV_TEXT", c: text, path: textValuePath, v: nil},
		{name: "non-RM value at DV_TEXT", c: text, path: textValuePath, v: "Lorem"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			b, err := composition.NewBuilder(t.Context(), tc.c,
				composition.WithTerritory("NL"),
				composition.WithComposer(testComposer()),
			)
			if err != nil {
				t.Fatalf("NewBuilder: %v", err)
			}
			if err := b.Set(tc.path, tc.v); !errors.Is(err, composition.ErrTypeMismatch) {
				t.Errorf("Set(%s, %T) = %v, want ErrTypeMismatch", tc.path, tc.v, err)
			}
			if _, err := b.Build(); !errors.Is(err, composition.ErrTypeMismatch) {
				t.Errorf("Build after Set(%s, %T) = %v, want ErrTypeMismatch", tc.path, tc.v, err)
			}
		})
	}
}

// abstractValueChild is the one child abstractValueOPT gives the
// ELEMENT's value: the abstract DATA_VALUE.
const abstractValueChild = `<children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>DATA_VALUE</rm_type_name><node_id></node_id></children>`

// valueAlternative returns a child that constrains an ELEMENT's value to
// rmType, with no node id. When excluded is true the child's occurrences
// are 0..0, so the template does not allow it.
func valueAlternative(rmType string, excluded bool) string {
	occurrences := ""
	if excluded {
		occurrences = `<occurrences><lower_included>true</lower_included><upper_included>true</upper_included>` +
			`<lower_unbounded>false</lower_unbounded><upper_unbounded>false</upper_unbounded><lower>0</lower><upper>0</upper></occurrences>`
	}
	return `<children xsi:type="C_COMPLEX_OBJECT"><rm_type_name>` + rmType + `</rm_type_name>` + occurrences + `<node_id></node_id></children>`
}

// compileAlternatives compiles abstractValueOPT with the ELEMENT's value
// offering alternatives, in their order, in place of DATA_VALUE. The
// alternatives share one path, abstractValuePath.
func compileAlternatives(t *testing.T, alternatives ...string) *templatecompile.Compiled {
	t.Helper()
	if n := strings.Count(abstractValueOPT, abstractValueChild); n != 1 {
		t.Fatalf("abstractValueOPT carries %d DATA_VALUE value children, want 1 (fixture drift)", n)
	}
	return compileInline(t, strings.Replace(abstractValueOPT, abstractValueChild, strings.Join(alternatives, ""), 1))
}

// alternativeTypesPath is the ELEMENT value of the corpus template
// alternative_types.en.v1, which the template allows as DV_QUANTITY,
// DV_COUNT or DV_ORDINAL, in that order.
const alternativeTypesPath = "/content[openEHR-EHR-OBSERVATION.alternative_types.v1]/data/events[at0002]/data/items[at0004]/value"

// TestREQ101_SetAcceptsAnyAlternativeAtPath checks that Set accepts a
// value whose RM type conforms to any alternative the template allows at
// the path, not only to the first one, which Compiled.NodeAt returns;
// that Build writes it there; and that the validator reports no finding
// the composition did not have before. An alternative whose occurrences
// are 0..0 does not stand in the way: under a DV_QUANTITY at 0..0 then a
// DV_CODED_TEXT, a DV_CODED_TEXT is accepted.
func TestREQ101_SetAcceptsAnyAlternativeAtPath(t *testing.T) {
	t.Parallel()
	codedThenQuantity := compileAlternatives(t, valueAlternative("DV_CODED_TEXT", false), valueAlternative("DV_QUANTITY", false))
	excludedQuantityThenCoded := compileAlternatives(t, valueAlternative("DV_QUANTITY", true), valueAlternative("DV_CODED_TEXT", false))
	alternativeTypes := compileFixture(t, "alternative_types.en.v1")

	cases := []struct {
		name string
		c    *templatecompile.Compiled
		path string
		// nodeAt is the RM type of the node Compiled.NodeAt returns at path.
		nodeAt string
		v      any
	}{
		{
			name: "DV_QUANTITY, the second of DV_CODED_TEXT and DV_QUANTITY", c: codedThenQuantity,
			path: abstractValuePath, nodeAt: "DV_CODED_TEXT", v: &rm.DVQuantity{Magnitude: 120, Units: "mm[Hg]"},
		},
		{
			name: "DV_CODED_TEXT, the first of DV_CODED_TEXT and DV_QUANTITY", c: codedThenQuantity,
			path: abstractValuePath, nodeAt: "DV_CODED_TEXT", v: codedText("Lorem"),
		},
		{
			name: "DV_CODED_TEXT after a DV_QUANTITY at 0..0", c: excludedQuantityThenCoded,
			path: abstractValuePath, nodeAt: "DV_QUANTITY", v: codedText("Lorem"),
		},
		{
			name: "DV_COUNT, the second of the corpus DV_QUANTITY, DV_COUNT and DV_ORDINAL", c: alternativeTypes,
			path: alternativeTypesPath, nodeAt: "DV_QUANTITY", v: &rm.DVCount{Magnitude: 3},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			node, err := tc.c.NodeAt(tc.path)
			if err != nil {
				t.Fatalf("NodeAt(%s): %v", tc.path, err)
			}
			if got := node.RMTypeName(); got != tc.nodeAt {
				t.Fatalf("NodeAt(%s) has RM type %q, want %q (fixture drift)", tc.path, got, tc.nodeAt)
			}
			b, err := composition.NewBuilder(t.Context(), tc.c,
				composition.WithTerritory("NL"),
				composition.WithComposer(testComposer()),
			)
			if err != nil {
				t.Fatalf("NewBuilder: %v", err)
			}
			skeleton, err := b.Build()
			if err != nil {
				t.Fatalf("Build of the bare skeleton: %v", err)
			}
			before := validation.ValidateComposition(skeleton, tc.c)

			if err := b.Set(tc.path, tc.v); err != nil {
				t.Fatalf("Set(%s, %T) = %v, want nil", tc.path, tc.v, err)
			}
			comp, err := b.Build()
			if err != nil {
				t.Fatalf("Build after Set(%s, %T) = %v, want nil", tc.path, tc.v, err)
			}
			got, err := rmpath.ItemAtPath(comp, tc.path)
			if err != nil {
				t.Fatalf("ItemAtPath(%s) on the built composition: %v", tc.path, err)
			}
			if got != tc.v {
				t.Errorf("built composition holds %T %v at %s, want the value Set was given, %T %v", got, got, tc.path, tc.v, tc.v)
			}
			after := validation.ValidateComposition(comp, tc.c)
			if added := newIssues(before, after); len(added) > 0 {
				t.Errorf("ValidateComposition after Set(%s, %T) reports new findings %q, want none", tc.path, tc.v, added)
			}
		})
	}
}

// TestREQ101_SetRefusesValueNoAlternativeAllows checks that Set returns
// ErrTypeMismatch, and Build carries it, for a value whose RM type
// conforms to none of the alternatives the template allows at the path:
// a DV_BOOLEAN under DV_CODED_TEXT or DV_QUANTITY, and a DV_QUANTITY
// where the template lists DV_QUANTITY with occurrences 0..0, whether
// that alternative comes after the allowed DV_CODED_TEXT or before it,
// where it is the node Compiled.NodeAt returns.
func TestREQ101_SetRefusesValueNoAlternativeAllows(t *testing.T) {
	t.Parallel()
	codedThenQuantity := compileAlternatives(t, valueAlternative("DV_CODED_TEXT", false), valueAlternative("DV_QUANTITY", false))
	codedThenExcludedQuantity := compileAlternatives(t, valueAlternative("DV_CODED_TEXT", false), valueAlternative("DV_QUANTITY", true))
	excludedQuantityThenCoded := compileAlternatives(t, valueAlternative("DV_QUANTITY", true), valueAlternative("DV_CODED_TEXT", false))
	alternativeTypes := compileFixture(t, "alternative_types.en.v1")

	cases := []struct {
		name string
		c    *templatecompile.Compiled
		path string
		v    any
	}{
		{name: "DV_BOOLEAN under DV_CODED_TEXT or DV_QUANTITY", c: codedThenQuantity, path: abstractValuePath, v: &rm.DVBoolean{Value: true}},
		{name: "DV_QUANTITY at 0..0 after DV_CODED_TEXT", c: codedThenExcludedQuantity, path: abstractValuePath, v: &rm.DVQuantity{Magnitude: 1, Units: "mm"}},
		{name: "DV_QUANTITY at 0..0 before DV_CODED_TEXT", c: excludedQuantityThenCoded, path: abstractValuePath, v: &rm.DVQuantity{Magnitude: 1, Units: "mm"}},
		{name: "DV_TEXT under the corpus DV_QUANTITY, DV_COUNT or DV_ORDINAL", c: alternativeTypes, path: alternativeTypesPath, v: &rm.DVText{Value: "x"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			b, err := composition.NewBuilder(t.Context(), tc.c,
				composition.WithTerritory("NL"),
				composition.WithComposer(testComposer()),
			)
			if err != nil {
				t.Fatalf("NewBuilder: %v", err)
			}
			if err := b.Set(tc.path, tc.v); !errors.Is(err, composition.ErrTypeMismatch) {
				t.Errorf("Set(%s, %T) = %v, want ErrTypeMismatch", tc.path, tc.v, err)
			}
			if _, err := b.Build(); !errors.Is(err, composition.ErrTypeMismatch) {
				t.Errorf("Build after Set(%s, %T) = %v, want ErrTypeMismatch", tc.path, tc.v, err)
			}
		})
	}
}
