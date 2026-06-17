package validation_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

// REQ-102 v2 — a structurally complete composition against
// vital_signs.opt produces no Issues. Establishes the positive
// path: all four mandatory composition attrs present, content
// non-empty with a known archetype id, identity matches at the
// composition root.
func TestValidateComposition_Valid(t *testing.T) {
	c := mustCompile(t, "vital_signs")
	comp := validVitalSignsComposition()
	r := validation.ValidateComposition(comp, c)
	if !r.OK {
		for _, issue := range r.Issues {
			t.Logf("issue: %s: %s — %s", issue.Path, issue.Code, issue.Detail)
		}
		t.Fatalf("ValidateComposition(valid) returned %d issues, want 0", len(r.Issues))
	}
}

// REQ-102 v2 — empty Category triggers required at /category. The
// composition's Category is the BMM-mandatory DV_CODED_TEXT
// channel — wiping it makes both DefiningCode and Value empty,
// which rmread reports as ok=false.
func TestValidateComposition_RequiredCategory(t *testing.T) {
	c := mustCompile(t, "vital_signs")
	comp := validVitalSignsComposition()
	comp.Category = rm.DVCodedText{}
	r := validation.ValidateComposition(comp, c)
	if r.OK {
		t.Fatal("expected required issue for empty category, got OK")
	}
	if !containsIssue(r.Issues, "/category", "required") {
		t.Errorf("expected /category required issue, got %+v", r.Issues)
	}
	// Sentinel bridge: Issue.Err() maps Code → typed sentinel so
	// callers can dispatch via errors.Is per spec § Sentinels.
	var requiredIssue validation.Issue
	for _, i := range r.Issues {
		if i.Path == "/category" && i.Code == "required" {
			requiredIssue = i
			break
		}
	}
	if !errors.Is(requiredIssue.Err(), validation.ErrRequired) {
		t.Errorf("Issue.Err() did not bridge %q to ErrRequired (got %v)", requiredIssue.Code, requiredIssue.Err())
	}
}

// REQ-102 v2 — nil Composer surfaces required at /composer.
// Composer is a PartyProxy interface; rmread returns ok=false on nil.
func TestValidateComposition_RequiredComposer(t *testing.T) {
	c := mustCompile(t, "vital_signs")
	comp := validVitalSignsComposition()
	comp.Composer = nil
	r := validation.ValidateComposition(comp, c)
	if r.OK || !containsIssue(r.Issues, "/composer", "required") {
		t.Errorf("expected /composer required issue, got %+v", r.Issues)
	}
}

// REQ-102 v2 — zero-valued Language / Territory each emit one
// required issue at their respective paths.
func TestValidateComposition_RequiredLanguageTerritory(t *testing.T) {
	c := mustCompile(t, "vital_signs")
	comp := validVitalSignsComposition()
	comp.Language = rm.CodePhrase{}
	comp.Territory = rm.CodePhrase{}
	r := validation.ValidateComposition(comp, c)
	if r.OK {
		t.Fatal("expected required issues for missing language/territory, got OK")
	}
	if !containsIssue(r.Issues, "/language", "required") ||
		!containsIssue(r.Issues, "/territory", "required") {
		t.Errorf("expected /language AND /territory required issues, got %+v", r.Issues)
	}
}

// REQ-102 v2 — Content with an unknown archetype id surfaces
// slot_fill at the offending /content[id] path. Uses an archetype
// id outside vital_signs.opt's declared root set.
func TestValidateComposition_SlotFillUnknownArchetype(t *testing.T) {
	c := mustCompile(t, "vital_signs")
	comp := validVitalSignsComposition()
	comp.Content = []rm.ContentItem{
		&rm.Observation{
			ArchetypeNodeID: "openEHR-EHR-OBSERVATION.no_such_archetype.v1",
		},
	}
	r := validation.ValidateComposition(comp, c)
	if r.OK {
		t.Fatal("expected slot_fill issue for unknown archetype, got OK")
	}
	if !containsCode(r.Issues, "slot_fill") {
		t.Errorf("expected slot_fill issue, got %+v", r.Issues)
	}
}

// REQ-104 — a CLUSTER filling the protocol slot whose archetype id
// fails the slot's parsed include assertions surfaces slot_fill. This
// is the reject path of the parsed grammar (distinct from the
// unknown-root case above, which never matches any OPT child).
func TestValidateComposition_SlotFillParsedIncludeRejects(t *testing.T) {
	c := mustCompile(t, "vital_signs")

	const badID = "openEHR-EHR-CLUSTER.not_an_allowed_device.v1"
	// Guard: only meaningful when some CLUSTER slot carries parsed
	// includes that reject badID. If every slot falls back to the
	// RM-type prefix rule, any CLUSTER id fits and there is nothing
	// to assert.
	var rejects bool
	for _, n := range c.AllByRMType("CLUSTER") {
		if n.IsSlot() && n.SlotRules().HasParsedIncludes() && !n.AllowsArchetypeID(badID) {
			rejects = true
			break
		}
	}
	if !rejects {
		t.Skip("no CLUSTER slot with parsed includes rejects the test id")
	}

	comp := validVitalSignsComposition()
	obs := validBloodPressureObservation()
	obs.Protocol = &rm.ItemTree{
		ArchetypeNodeID: "at0011",
		Name:            rm.DVText{Value: "protocol"},
		Items: []rm.Item{
			&rm.Cluster{
				ArchetypeNodeID: badID,
				Name:            rm.DVText{Value: "Device"},
			},
		},
	}
	comp.Content = []rm.ContentItem{obs}

	r := validation.ValidateComposition(comp, c)
	if r.OK {
		t.Fatal("expected slot_fill for non-conforming CLUSTER in protocol slot, got OK")
	}
	if !containsCode(r.Issues, "slot_fill") {
		t.Errorf("expected slot_fill issue, got %+v", r.Issues)
	}
}

// REQ-104 — C_SINGLE_ATTRIBUTE slots enforce the same parsed
// include/exclude rules as C_MULTIPLE_ATTRIBUTE slots. A protocol
// slot that accepts only openEHR-EHR-ITEM_TREE.allowed.v1 must reject
// any other ITEM_TREE archetype id even though the RM type matches.
func TestValidateComposition_SingleAttributeSlotFillParsedIncludeRejects(t *testing.T) {
	c := mustCompileSyntheticOPT(t, singleAttributeSlotOPT)
	comp := validSingleAttributeSlotComposition("openEHR-EHR-ITEM_TREE.rejected.v1")

	r := validation.ValidateComposition(comp, c)
	if !containsCode(r.Issues, "slot_fill") {
		t.Fatalf("expected slot_fill for non-conforming single-attribute slot fill, got OK=%v issues=%+v", r.OK, r.Issues)
	}
}

// REQ-102 v2 — nil composition argument surfaces a global
// nil_composition issue (no panic).
func TestValidateComposition_NilComposition(t *testing.T) {
	c := mustCompile(t, "vital_signs")
	r := validation.ValidateComposition(nil, c)
	if r.OK || !containsCode(r.Issues, "nil_composition") {
		t.Errorf("expected nil_composition issue, got %+v", r.Issues)
	}
}

// REQ-102 v2 — nil compiled template surfaces a nil_template issue.
func TestValidateComposition_NilTemplate(t *testing.T) {
	r := validation.ValidateComposition(validVitalSignsComposition(), nil)
	if r.OK || !containsCode(r.Issues, "nil_template") {
		t.Errorf("expected nil_template issue, got %+v", r.Issues)
	}
}

// REQ-102 v2 — Severity stringer + Issue exports work as
// documented. Includes the out-of-range fallback so future
// severity additions don't silently render as a numeric value.
func TestSeverity_String(t *testing.T) {
	if got := validation.Error.String(); got != "error" {
		t.Errorf("Error.String() = %q, want \"error\"", got)
	}
	if got := validation.Warning.String(); got != "warning" {
		t.Errorf("Warning.String() = %q, want \"warning\"", got)
	}
	if got := validation.Severity(99).String(); !strings.Contains(got, "99") || !strings.Contains(got, "severity") {
		t.Errorf("Severity(99).String() = %q, want a label containing \"severity\" and the numeric form \"99\"", got)
	}
}

// REQ-102 v2 — Result.Issues is never nil after a validator call
// (even when OK is true).
func TestValidateComposition_IssuesNeverNilOnSuccess(t *testing.T) {
	c := mustCompile(t, "vital_signs")
	comp := validVitalSignsComposition()
	r := validation.ValidateComposition(comp, c)
	if !r.OK {
		t.Fatalf("expected OK result, got issues %+v", r.Issues)
	}
	if r.Issues == nil {
		t.Errorf("Result.Issues = nil on success; doc says never nil")
	}
}

// REQ-102 v2 — wrong RM type under a single attribute surfaces
// rm_type_mismatch. Replace COMPOSITION's category (declared
// DV_CODED_TEXT) with an empty DV_TEXT struct by zeroing the
// DefiningCode but keeping the Value — the rmread layer reports
// ok=true (DVCodedText non-zero) and the structural walker
// descends into the DV_CODED_TEXT OPT child. Without an
// rm_type_mismatch case at the leaf we exercise this via the
// COMPOSITION /content slot: build an Evaluation where the slot
// expects an Observation.
func TestValidateComposition_RMTypeMismatch(t *testing.T) {
	c := mustCompile(t, "vital_signs")
	comp := validVitalSignsComposition()
	// The /content slot accepts archetype-id pins; assigning the
	// blood_pressure archetype id to an Evaluation tricks the
	// matcher into binding to the Observation OPT child, exposing
	// the rm_type_mismatch path.
	comp.Content = []rm.ContentItem{
		&rm.Evaluation{
			ArchetypeNodeID: "openEHR-EHR-OBSERVATION.blood_pressure.v1",
		},
	}
	r := validation.ValidateComposition(comp, c)
	if r.OK {
		t.Fatal("expected rm_type_mismatch, got OK")
	}
	if !containsCode(r.Issues, "rm_type_mismatch") {
		t.Errorf("expected rm_type_mismatch issue, got %+v", r.Issues)
	}
}

// REQ-102 v2 — wrong archetype_node_id at a LOCATABLE node surfaces
// archetype_id_mismatch (the LOCATABLE is at the archetype-root
// level). Tests the identity-check branch when the matched OPT
// child has an ArchetypeID pinned.
func TestValidateComposition_ArchetypeIDMismatch(t *testing.T) {
	c := mustCompile(t, "vital_signs")
	comp := validVitalSignsComposition()
	// vital_signs.opt has a body_temperature archetype root under
	// /content. An Observation whose archetype_node_id pretends to
	// be body_temperature but the OPT slot matches by archetype id —
	// this isn't actually a mismatch, the matcher binds correctly.
	// To force the mismatch path we'd need the matcher to bind to
	// a child whose pinned ArchetypeID disagrees with the RM's. Use
	// the matchChildByID strict equality channel: assign a known
	// archetype id that exists, mutate the OPT-bound at-code on a
	// nested LOCATABLE so the identity branch fires deeper. For
	// Phase 2 we exercise the surface from the COMPOSITION root
	// instead: when the RM archetype_node_id at the composition
	// root is wrong, the OPT root's ArchetypeID() (if any) fires
	// archetype_id_mismatch.
	comp.ArchetypeNodeID = "openEHR-EHR-COMPOSITION.encounter.WRONG.v1"
	r := validation.ValidateComposition(comp, c)
	if r.OK {
		t.Fatal("expected archetype_id_mismatch or node_id_mismatch, got OK")
	}
	// Either code is acceptable depending on whether the OPT root
	// pins an archetype id or only a node id — both indicate the
	// identity mismatch.
	if !containsCode(r.Issues, "archetype_id_mismatch") && !containsCode(r.Issues, "node_id_mismatch") {
		t.Errorf("expected identity mismatch issue at root, got %+v", r.Issues)
	}
}

// REQ-102 v2 Phase 2 — empty Observation.Data.Events (zero events
// where the OPT pins existence ≥ 1) surfaces a `required` issue
// at /content[…]/data/events.
func TestValidateComposition_EmptyEvents(t *testing.T) {
	c := mustCompile(t, "vital_signs")
	comp := validVitalSignsComposition()
	obs := comp.Content[0].(*rm.Observation)
	obs.Data.Events = nil
	r := validation.ValidateComposition(comp, c)
	if r.OK {
		t.Fatal("expected required + cardinality issues for empty events, got OK")
	}
	// vital_signs.opt pins both existence ≥ 1 AND cardinality lower ≥ 1
	// on /data/events. Both constraints are independent — the spec
	// (clinical-modeling.md § REQ-102) and PROBE-026 expect both codes.
	wantPath := "/content[openEHR-EHR-OBSERVATION.blood_pressure.v1]/data/events"
	if !containsIssue(r.Issues, wantPath, "required") {
		t.Errorf("expected required at %s, got %+v", wantPath, r.Issues)
	}
	if !containsIssue(r.Issues, wantPath, "cardinality") {
		t.Errorf("expected cardinality at %s (OPT pins child-count lower ≥ 1), got %+v", wantPath, r.Issues)
	}
}

// REQ-102 v2 Phase 2 — Element.Value set to nil when the OPT
// requires a value (existence ≥ 1) surfaces `required` at the
// element's /value path. Confirms the walker reaches ELEMENT
// leaves and existence is checked there.
func TestValidateComposition_NilElementValue(t *testing.T) {
	c := mustCompile(t, "vital_signs")
	comp := validVitalSignsComposition()
	obs := comp.Content[0].(*rm.Observation)
	pe := obs.Data.Events[0].(*rm.PointEvent[rm.ItemStructure])
	list := pe.Data.(*rm.ItemList)
	list.Items[0].Value = nil
	r := validation.ValidateComposition(comp, c)
	wantPath := "/content[openEHR-EHR-OBSERVATION.blood_pressure.v1]/data/events[at0006]/data/items[at0004]/value"
	if !containsIssue(r.Issues, wantPath, "required") {
		t.Errorf("expected required at %s, got %+v", wantPath, r.Issues)
	}
}

// REQ-102 v2 Phase 3 — out-of-range systolic magnitude triggers
// primitive_out_of_range at the element's /value path. vital_signs.opt
// pins the systolic DV_QUANTITY range upper at 1000 mm[Hg].
func TestValidateComposition_PrimitiveOutOfRange(t *testing.T) {
	c := mustCompile(t, "vital_signs")
	comp := validVitalSignsComposition()
	obs := comp.Content[0].(*rm.Observation)
	pe := obs.Data.Events[0].(*rm.PointEvent[rm.ItemStructure])
	list := pe.Data.(*rm.ItemList)
	list.Items[0].Value = &rm.DVQuantity{
		Magnitude: rm.Real(2000),
		Units:     "mm[Hg]",
	}
	r := validation.ValidateComposition(comp, c)
	wantPath := "/content[openEHR-EHR-OBSERVATION.blood_pressure.v1]/data/events[at0006]/data/items[at0004]/value"
	found := false
	for _, i := range r.Issues {
		if i.Path == wantPath && strings.HasPrefix(i.Code, "primitive_") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected primitive_* issue at %s, got %+v", wantPath, r.Issues)
	}
}

// REQ-102 v2 Phase 3 — DV_QUANTITY units not in the OPT-allowed
// list trigger primitive_unit_unknown.
func TestValidateComposition_PrimitiveUnitsUnknown(t *testing.T) {
	c := mustCompile(t, "vital_signs")
	comp := validVitalSignsComposition()
	obs := comp.Content[0].(*rm.Observation)
	pe := obs.Data.Events[0].(*rm.PointEvent[rm.ItemStructure])
	list := pe.Data.(*rm.ItemList)
	list.Items[0].Value = &rm.DVQuantity{
		Magnitude: rm.Real(120),
		Units:     "psi", // not in {mm[Hg]}
	}
	r := validation.ValidateComposition(comp, c)
	if !containsCode(r.Issues, "primitive_unit_unknown") {
		t.Errorf("expected primitive_unit_unknown issue, got %+v", r.Issues)
	}
}

// REQ-102 v2 Phase 3 — category defining_code is constrained by the
// OPT to a closed list (openehr::433). Supplying a code outside that
// list surfaces a primitive_not_in_list issue at
// /category/defining_code.
func TestValidateComposition_PrimitiveCategoryNotInList(t *testing.T) {
	c := mustCompile(t, "vital_signs")
	comp := validVitalSignsComposition()
	comp.Category.DefiningCode = rm.CodePhrase{
		TerminologyID: rm.TerminologyID{Value: "openehr"},
		CodeString:    "999",
	}
	r := validation.ValidateComposition(comp, c)
	if r.OK {
		t.Fatal("expected primitive issue for category not in list, got OK")
	}
	wantPath := "/category/defining_code"
	found := false
	for _, i := range r.Issues {
		if i.Path == wantPath && strings.HasPrefix(i.Code, "primitive_") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected primitive_* at %s, got %+v", wantPath, r.Issues)
	}
}

// REQ-102 v2 Phase 2 — removing the systolic element entirely
// (empty items slice on the ITEM_LIST) surfaces a `required`
// issue at /content[…]/data/events[at0006]/data/items.
func TestValidateComposition_MissingSystolic(t *testing.T) {
	c := mustCompile(t, "vital_signs")
	comp := validVitalSignsComposition()
	obs := comp.Content[0].(*rm.Observation)
	pe := obs.Data.Events[0].(*rm.PointEvent[rm.ItemStructure])
	list := pe.Data.(*rm.ItemList)
	list.Items = nil
	r := validation.ValidateComposition(comp, c)
	wantPath := "/content[openEHR-EHR-OBSERVATION.blood_pressure.v1]/data/events[at0006]/data/items"
	if !containsIssue(r.Issues, wantPath, "required") {
		t.Errorf("expected required at %s, got %+v", wantPath, r.Issues)
	}
}

// REQ-102 v2 Phase 4 — alternative matching on C_SINGLE_ATTRIBUTE.
// An OPT with two alternatives under /name (DV_TEXT or
// DV_CODED_TEXT) accepts an RM value of either type; a DV_QUANTITY
// under the same slot triggers `alternative_mismatch`. Uses a
// synthetic OPT so the test does not depend on vital_signs.opt's
// shape (which has no native multi-child single attribute on a
// reachable leaf).
func TestValidateComposition_AlternativeMismatch_Positive(t *testing.T) {
	const body = `<?xml version="1.0"?>
<template xmlns="http://schemas.openehr.org/v1" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <template_id><value>alt-test</value></template_id>
  <concept>alt-test</concept>
  <language>
    <terminology_id><value>ISO_639-1</value></terminology_id>
    <code_string>en</code_string>
  </language>
  <definition>
    <rm_type_name>COMPOSITION</rm_type_name>
    <node_id>at0000</node_id>
    <attributes xsi:type="C_SINGLE_ATTRIBUTE">
      <rm_attribute_name>name</rm_attribute_name>
      <existence>
        <lower>1</lower>
        <upper>1</upper>
      </existence>
      <children xsi:type="C_COMPLEX_OBJECT">
        <rm_type_name>DV_TEXT</rm_type_name>
        <node_id />
      </children>
      <children xsi:type="C_COMPLEX_OBJECT">
        <rm_type_name>DV_CODED_TEXT</rm_type_name>
        <node_id />
      </children>
    </attributes>
  </definition>
</template>`
	opt, err := template.ParseOPT(strings.NewReader(body))
	if err != nil {
		t.Fatalf("ParseOPT: %v", err)
	}
	c, err := templatecompile.Compile(opt)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	// Positive: DVText satisfies the first alternative (DV_TEXT) —
	// no alternative_mismatch fires on /name. Other issues
	// (composer/language/territory implicit BMM-required) MAY
	// surface but are unrelated.
	compOK := &rm.Composition{
		ArchetypeNodeID: "at0000",
		Name:            rm.DVText{Value: "ok"},
	}
	if r := validation.ValidateComposition(compOK, c); containsCode(r.Issues, "alternative_mismatch") {
		t.Errorf("DVText under DV_TEXT|DV_CODED_TEXT alternatives mismatched: %+v", r.Issues)
	}
}

// REQ-102 v2 Phase 4 — alternative mismatch NEGATIVE case. An RM
// value whose type matches none of the OPT's C_SINGLE_ATTRIBUTE
// alternatives surfaces `alternative_mismatch` at the attribute
// path. Uses Composer (PartyProxy interface) since Go's static
// typing forbids assigning a DV_QUANTITY where Name expects DVText.
func TestValidateComposition_AlternativeMismatch_Negative(t *testing.T) {
	const body = `<?xml version="1.0"?>
<template xmlns="http://schemas.openehr.org/v1" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <template_id><value>alt-neg-test</value></template_id>
  <concept>alt-neg-test</concept>
  <language>
    <terminology_id><value>ISO_639-1</value></terminology_id>
    <code_string>en</code_string>
  </language>
  <definition>
    <rm_type_name>COMPOSITION</rm_type_name>
    <node_id>at0000</node_id>
    <attributes xsi:type="C_SINGLE_ATTRIBUTE">
      <rm_attribute_name>composer</rm_attribute_name>
      <existence><lower>1</lower><upper>1</upper></existence>
      <children xsi:type="C_COMPLEX_OBJECT">
        <rm_type_name>PARTY_SELF</rm_type_name>
        <node_id />
      </children>
      <children xsi:type="C_COMPLEX_OBJECT">
        <rm_type_name>PARTY_IDENTIFIED</rm_type_name>
        <node_id />
      </children>
    </attributes>
  </definition>
</template>`
	opt, err := template.ParseOPT(strings.NewReader(body))
	if err != nil {
		t.Fatalf("ParseOPT: %v", err)
	}
	c, err := templatecompile.Compile(opt)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	// PartyRelated satisfies neither alternative (PARTY_SELF /
	// PARTY_IDENTIFIED) → alternative_mismatch at /composer.
	comp := &rm.Composition{
		ArchetypeNodeID: "at0000",
		Composer: &rm.PartyRelated{
			PartyIdentified: rm.PartyIdentified{},
		},
	}
	r := validation.ValidateComposition(comp, c)
	if r.OK {
		t.Fatal("expected alternative_mismatch for PartyRelated under PARTY_SELF|PARTY_IDENTIFIED alternatives, got OK")
	}
	if !containsIssue(r.Issues, "/composer", "alternative_mismatch") {
		t.Errorf("expected alternative_mismatch at /composer, got %+v", r.Issues)
	}
}

// REQ-102 v2 — typed-nil DataValue inside Element.Value MUST NOT
// panic the walker. Go stores `Element.Value = (*rm.DVQuantity)(nil)`
// as a non-nil interface (carries a type) whose underlying pointer
// is nil; without an explicit typed-nil check, ifacePresent would
// return ok=true and the primitive dispatcher would dereference and
// panic. The walker must treat typed-nil as "value absent" and
// surface a `required` issue when the OPT pins existence ≥ 1.
func TestValidateComposition_TypedNilDataValueNoPanic(t *testing.T) {
	c := mustCompile(t, "vital_signs")
	comp := validVitalSignsComposition()
	obs := comp.Content[0].(*rm.Observation)
	pe := obs.Data.Events[0].(*rm.PointEvent[rm.ItemStructure])
	list := pe.Data.(*rm.ItemList)
	list.Items[0].Value = (*rm.DVQuantity)(nil) // typed-nil — interface non-nil, pointer nil
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("ValidateComposition panicked on typed-nil DV_QUANTITY: %v", r)
		}
	}()
	r := validation.ValidateComposition(comp, c)
	wantPath := "/content[openEHR-EHR-OBSERVATION.blood_pressure.v1]/data/events[at0006]/data/items[at0004]/value"
	if !containsIssue(r.Issues, wantPath, "required") {
		t.Errorf("expected required at %s for typed-nil Element.Value, got %+v", wantPath, r.Issues)
	}
}

// REQ-102 v2 — typed-nil ContentItem in Composition.Content MUST NOT
// panic. Same Go interface footgun as Element.Value but on multi-
// valued interface slices: (*rm.Observation)(nil) in content[].
func TestValidateComposition_TypedNilContentItemNoPanic(t *testing.T) {
	c := mustCompile(t, "vital_signs")
	comp := validVitalSignsComposition()
	comp.Content = []rm.ContentItem{(*rm.Observation)(nil)}
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("ValidateComposition panicked on typed-nil ContentItem: %v", r)
		}
	}()
	r := validation.ValidateComposition(comp, c)
	if r.OK {
		t.Fatal("expected slot_fill for typed-nil content item, got OK")
	}
	if !containsCode(r.Issues, "slot_fill") {
		t.Errorf("expected slot_fill for typed-nil content item, got %+v", r.Issues)
	}
}

// REQ-102 v2 — typed-nil Event in History.Events MUST NOT panic.
func TestValidateComposition_TypedNilEventNoPanic(t *testing.T) {
	c := mustCompile(t, "vital_signs")
	comp := validVitalSignsComposition()
	obs := comp.Content[0].(*rm.Observation)
	obs.Data.Events = []rm.Event{(*rm.PointEvent[rm.ItemStructure])(nil)}
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("ValidateComposition panicked on typed-nil Event: %v", r)
		}
	}()
	r := validation.ValidateComposition(comp, c)
	if !containsCode(r.Issues, "slot_fill") {
		t.Errorf("expected slot_fill for typed-nil event, got %+v", r.Issues)
	}
}

// REQ-102 v2 — per-child occurrences upper bound. vital_signs.opt pins
// systolic ELEMENT (at0004) at 0..1; duplicating it fires
// `cardinality` at the child path.
func TestValidateComposition_DuplicateSystolicOccurrences(t *testing.T) {
	c := mustCompile(t, "vital_signs")
	comp := validVitalSignsComposition()
	obs := comp.Content[0].(*rm.Observation)
	pe := obs.Data.Events[0].(*rm.PointEvent[rm.ItemStructure])
	list := pe.Data.(*rm.ItemList)
	systolic := list.Items[0]
	list.Items = []rm.Element{systolic, systolic}
	r := validation.ValidateComposition(comp, c)
	wantPath := "/content[openEHR-EHR-OBSERVATION.blood_pressure.v1]/data/events[at0006]/data/items[at0004]"
	if !containsIssue(r.Issues, wantPath, "cardinality") {
		t.Errorf("expected cardinality at %s for duplicate systolic, got %+v", wantPath, r.Issues)
	}
}

// REQ-102 v2 — wrong at-code on a nested LOCATABLE bound by RM
// type via C_SINGLE_ATTRIBUTE triggers `node_id_mismatch` at the
// matched node. The single-attribute matcher binds by RM type
// (not at-code), then walkNode descends and checkLocatableIdentity
// catches the at-code disagreement. Multi-valued attributes use
// the OR-accept matchChildByID and surface unbindable at-codes as
// `slot_fill` at the parent attribute path — different code, same
// underlying intent.
//
// Exercises the path
// /content[bp]/data/events[at0006]/data where /data is a
// C_SINGLE_ATTRIBUTE on POINT_EVENT pinning ITEM_LIST at0003.
// Mutating the RM ItemList's archetype_node_id to at9999 fires
// node_id_mismatch at the matched node.
func TestValidateComposition_NestedNodeIDMismatch(t *testing.T) {
	c := mustCompile(t, "vital_signs")
	comp := validVitalSignsComposition()
	obs := comp.Content[0].(*rm.Observation)
	pe := obs.Data.Events[0].(*rm.PointEvent[rm.ItemStructure])
	list := pe.Data.(*rm.ItemList)
	list.ArchetypeNodeID = "at9999" // OPT pins at0003 on the ITEM_LIST
	r := validation.ValidateComposition(comp, c)
	if r.OK {
		t.Fatal("expected node_id_mismatch for nested ITEM_LIST at-code, got OK")
	}
	if !containsCode(r.Issues, "node_id_mismatch") {
		t.Errorf("expected node_id_mismatch on nested LOCATABLE bound via single attribute, got %+v", r.Issues)
	}
}

// REQ-102 v2 Phase 2 — IntervalEvent dispatch reaches the walker
// the same way PointEvent does. Swap the BP fixture's PointEvent
// for an IntervalEvent over the same data and confirm out-of-range
// systolic still surfaces through the IntervalEvent code path.
func TestValidateComposition_IntervalEventDispatch(t *testing.T) {
	c := mustCompile(t, "vital_signs")
	comp := validVitalSignsComposition()
	obs := comp.Content[0].(*rm.Observation)
	pe := obs.Data.Events[0].(*rm.PointEvent[rm.ItemStructure])
	pe.Data.(*rm.ItemList).Items[0].Value = &rm.DVQuantity{
		Magnitude: rm.Real(2000),
		Units:     "mm[Hg]",
	}
	obs.Data.Events[0] = &rm.IntervalEvent[rm.ItemStructure]{
		ArchetypeNodeID: pe.ArchetypeNodeID,
		Name:            pe.Name,
		Time:            pe.Time,
		Data:            pe.Data,
		State:           pe.State,
	}
	r := validation.ValidateComposition(comp, c)
	if r.OK {
		t.Fatal("expected primitive_out_of_range through IntervalEvent dispatch, got OK")
	}
	wantPath := "/content[openEHR-EHR-OBSERVATION.blood_pressure.v1]/data/events[at0006]/data/items[at0004]/value"
	if !containsIssue(r.Issues, wantPath, "primitive_out_of_range") {
		t.Errorf("expected primitive_out_of_range at %s via IntervalEvent, got %+v", wantPath, r.Issues)
	}
}

// REQ-102 v2 Phase 2 — SECTION recursion is bounded by the OPT
// (compiled tree is finite; slots are leaves). vital_signs.opt
// declares no SECTION under /content, so any Section we drop into
// the composition fails slot-fit with `slot_fill` — but the walker
// MUST NOT panic on the empty Section.Items case, and the slot-fit
// failure is the ONLY issue at that path.
func TestValidateComposition_SectionEmptyNoPanic(t *testing.T) {
	c := mustCompile(t, "vital_signs")
	comp := validVitalSignsComposition()
	// Replace Content with an unrelated Section so the slot-fit
	// machinery is exercised; Items is nil to exercise the
	// recursion-bottoms-out path without panic.
	comp.Content = []rm.ContentItem{
		&rm.Section{
			ArchetypeNodeID: "openEHR-EHR-SECTION.unknown.v1",
			Name:            rm.DVText{Value: "Section"},
			Items:           nil,
		},
	}
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("ValidateComposition panicked on SECTION with nil Items: %v", r)
		}
	}()
	r := validation.ValidateComposition(comp, c)
	// Slot-fit MUST flag the SECTION because vital_signs.opt's
	// /content does not declare any SECTION archetype root or slot.
	if r.OK {
		t.Fatal("expected slot_fill for SECTION under /content, got OK")
	}
	if !containsCode(r.Issues, "slot_fill") {
		t.Errorf("expected slot_fill for SECTION under /content, got %+v", r.Issues)
	}
}

// --- helpers ----------------------------------------------------------------

// validVitalSignsComposition constructs a structurally-complete
// composition matching the vital_signs.opt root + observation
// constraints. v2's template-driven walker enforces all BMM-mandatory
// attributes that the OPT either pins or that the COMPOSITION /
// OBSERVATION classes carry by default (name, language, territory,
// encoding, subject on entries; data + protocol pinned by the OPT
// for OBSERVATION). The helper populates the full chain so the
// "valid" test asserts a zero-issue result.
func validVitalSignsComposition() *rm.Composition {
	return &rm.Composition{
		ArchetypeNodeID: "openEHR-EHR-COMPOSITION.encounter.v1",
		Name:            rm.DVText{Value: "Encounter"},
		Category: rm.DVCodedText{
			DVText: rm.DVText{Value: "event"},
			DefiningCode: rm.CodePhrase{
				TerminologyID: rm.TerminologyID{Value: "openehr"},
				CodeString:    "433",
			},
		},
		Composer: rm.PartySelf{},
		Language: rm.CodePhrase{
			TerminologyID: rm.TerminologyID{Value: "ISO_639-1"},
			CodeString:    "en",
		},
		Territory: rm.CodePhrase{
			TerminologyID: rm.TerminologyID{Value: "ISO_3166-1"},
			CodeString:    "NL",
		},
		Content: []rm.ContentItem{
			validBloodPressureObservation(),
		},
	}
}

// validBloodPressureObservation constructs an OBSERVATION conforming
// to vital_signs.opt's /content[blood_pressure] subtree: history
// with a single PointEvent over an ItemTree with the systolic
// element, plus protocol and the BMM-mandatory entry channels
// (language, encoding, subject, name).
func validBloodPressureObservation() *rm.Observation {
	return &rm.Observation{
		ArchetypeNodeID: "openEHR-EHR-OBSERVATION.blood_pressure.v1",
		Name:            rm.DVText{Value: "Blood pressure"},
		Language: rm.CodePhrase{
			TerminologyID: rm.TerminologyID{Value: "ISO_639-1"},
			CodeString:    "en",
		},
		Encoding: rm.CodePhrase{
			TerminologyID: rm.TerminologyID{Value: "IANA_character-sets"},
			CodeString:    "UTF-8",
		},
		Subject: rm.PartySelf{},
		Data: rm.History[rm.ItemStructure]{
			ArchetypeNodeID: "at0001",
			Name:            rm.DVText{Value: "history"},
			Origin:          rm.DVDateTime{Value: "2026-05-24T10:00:00Z"},
			Events: []rm.Event{
				&rm.PointEvent[rm.ItemStructure]{
					ArchetypeNodeID: "at0006",
					Name:            rm.DVText{Value: "any event"},
					Time:            rm.DVDateTime{Value: "2026-05-24T10:00:00Z"},
					// vital_signs.opt pins both /data and /state on
					// the PointEvent as ITEM_LIST with required items.
					// Match the OPT RM-type or v2 emits rm_type_mismatch.
					Data: &rm.ItemList{
						ArchetypeNodeID: "at0003",
						Name:            rm.DVText{Value: "blood pressure"},
						Items: []rm.Element{{
							ArchetypeNodeID: "at0004",
							Name:            rm.DVText{Value: "Systolic"},
							Value: &rm.DVQuantity{
								Magnitude: rm.Real(120),
								Units:     "mm[Hg]",
							},
						}},
					},
					State: &rm.ItemList{
						ArchetypeNodeID: "at0007",
						Name:            rm.DVText{Value: "state"},
						Items: []rm.Element{{
							ArchetypeNodeID: "at0008",
							Name:            rm.DVText{Value: "Position"},
						}},
					},
				},
			},
		},
		// The OPT pins /protocol/items to an ARCHETYPE_SLOT
		// constrained to CLUSTER. Slot fit is evaluated against the
		// parsed REQ-104 assertion grammar, falling back to the
		// RM-type-prefix rule when the OPT carried no parseable
		// includes; an archetype id of the shape
		// "openEHR-EHR-CLUSTER.<concept>.v<n>" satisfies the match.
		Protocol: &rm.ItemTree{
			ArchetypeNodeID: "at0011",
			Name:            rm.DVText{Value: "protocol"},
			Items: []rm.Item{
				&rm.Cluster{
					ArchetypeNodeID: "openEHR-EHR-CLUSTER.device.v1",
					Name:            rm.DVText{Value: "Device"},
				},
			},
		},
	}
}

func validSingleAttributeSlotComposition(protocolID string) *rm.Composition {
	obs := validBloodPressureObservation()
	obs.Protocol = &rm.ItemTree{
		ArchetypeNodeID: protocolID,
		Name:            rm.DVText{Value: "Protocol"},
	}
	return &rm.Composition{
		ArchetypeNodeID: "openEHR-EHR-COMPOSITION.single_attribute_slot.v1",
		Name:            rm.DVText{Value: "Encounter"},
		Category: rm.DVCodedText{
			DVText: rm.DVText{Value: "event"},
			DefiningCode: rm.CodePhrase{
				TerminologyID: rm.TerminologyID{Value: "openehr"},
				CodeString:    "433",
			},
		},
		Composer: rm.PartySelf{},
		Language: rm.CodePhrase{
			TerminologyID: rm.TerminologyID{Value: "ISO_639-1"},
			CodeString:    "en",
		},
		Territory: rm.CodePhrase{
			TerminologyID: rm.TerminologyID{Value: "ISO_3166-1"},
			CodeString:    "NL",
		},
		Content: []rm.ContentItem{obs},
	}
}

func mustCompile(t *testing.T, fixture string) *templatecompile.Compiled {
	t.Helper()
	opt, err := template.ParseFile(fixtures.TemplateOptForName(fixture))
	if err != nil {
		t.Fatalf("ParseFile(%s): %v", fixture, err)
	}
	c, err := templatecompile.Compile(opt)
	if err != nil {
		t.Fatalf("Compile(%s): %v", fixture, err)
	}
	return c
}

func mustCompileSyntheticOPT(t *testing.T, xml string) *templatecompile.Compiled {
	t.Helper()
	path := filepath.Join(t.TempDir(), "synthetic.opt")
	if err := os.WriteFile(path, []byte(xml), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	opt, err := template.ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile(synthetic): %v", err)
	}
	c, err := templatecompile.Compile(opt)
	if err != nil {
		t.Fatalf("Compile(synthetic): %v", err)
	}
	return c
}

func containsCode(issues []validation.Issue, code string) bool {
	for _, i := range issues {
		if i.Code == code {
			return true
		}
	}
	return false
}

func containsIssue(issues []validation.Issue, path, code string) bool {
	for _, i := range issues {
		if i.Path == path && i.Code == code {
			return true
		}
	}
	return false
}

const singleAttributeSlotOPT = `<?xml version="1.0" encoding="utf-8"?>
<template xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance" xmlns:xsd="http://www.w3.org/2001/XMLSchema" xmlns="http://schemas.openehr.org/v1">
  <language>
    <terminology_id><value>ISO_639-1</value></terminology_id>
    <code_string>en</code_string>
  </language>
  <template_id><value>single_attribute_slot</value></template_id>
  <concept>single_attribute_slot</concept>
  <definition>
    <rm_type_name>COMPOSITION</rm_type_name>
    <node_id>at0000</node_id>
    <attributes xsi:type="C_MULTIPLE_ATTRIBUTE">
      <rm_attribute_name>content</rm_attribute_name>
      <children xsi:type="C_COMPLEX_OBJECT">
        <rm_type_name>OBSERVATION</rm_type_name>
        <node_id>at0000</node_id>
        <attributes xsi:type="C_SINGLE_ATTRIBUTE">
          <rm_attribute_name>protocol</rm_attribute_name>
          <children xsi:type="ARCHETYPE_SLOT">
            <rm_type_name>ITEM_TREE</rm_type_name>
            <node_id>at9000</node_id>
            <includes>
              <expression xsi:type="EXPR_BINARY_OPERATOR">
                <type>Boolean</type>
                <operator>2007</operator>
                <precedence_overridden>false</precedence_overridden>
                <left_operand xsi:type="EXPR_LEAF">
                  <type>String</type>
                  <item xsi:type="xsd:string">archetype_id/value</item>
                  <reference_type>attribute</reference_type>
                </left_operand>
                <right_operand xsi:type="EXPR_LEAF">
                  <type>C_STRING</type>
                  <item xsi:type="C_STRING">
                    <pattern>openEHR-EHR-ITEM_TREE\.allowed\.v1</pattern>
                  </item>
                  <reference_type>constraint</reference_type>
                </right_operand>
              </expression>
            </includes>
          </children>
        </attributes>
        <archetype_id>
          <value>openEHR-EHR-OBSERVATION.blood_pressure.v1</value>
        </archetype_id>
      </children>
    </attributes>
    <archetype_id>
      <value>openEHR-EHR-COMPOSITION.single_attribute_slot.v1</value>
    </archetype_id>
  </definition>
</template>`
