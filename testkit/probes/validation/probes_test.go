package validationprobes_test

import (
	"os"
	"testing"

	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
	probes "github.com/cadasto/openehr-sdk-go/testkit/probes/validation"
)

// PROBE-025 — positive case. A structurally-complete blood-pressure
// composition against vital_signs.opt produces zero issues.
func TestProbe025CompositionValidate_Positive(t *testing.T) {
	body := loadFixture(t, "vital_signs")
	cases := []probes.ValidateCase{
		{
			Name:        "valid_blood_pressure",
			OPT:         body,
			Composition: probes.ValidBloodPressureComposition(),
			WantCodes:   nil,
		},
	}
	r, err := probes.Probe025CompositionValidate(cases)
	if err != nil {
		t.Fatalf("Probe025: %v", err)
	}
	if r.Status != "pass" {
		t.Fatalf("Probe025 status=%q detail=%q", r.Status, r.Detail)
	}
	if r.Probe != "PROBE-025" {
		t.Errorf("Probe id = %q, want PROBE-025", r.Probe)
	}
}

// PROBE-025 — primitive-violation case. Out-of-range systolic
// magnitude triggers a single primitive_out_of_range issue. Stable
// across implementations of REQ-103 + REQ-102 v2.
func TestProbe025CompositionValidate_PrimitiveOutOfRange(t *testing.T) {
	body := loadFixture(t, "vital_signs")
	cases := []probes.ValidateCase{
		{
			Name:        "systolic_2000_mmHg",
			OPT:         body,
			Composition: probes.SystolicOutOfRange(),
			WantCodes:   []string{"primitive_out_of_range"},
		},
	}
	r, err := probes.Probe025CompositionValidate(cases)
	if err != nil {
		t.Fatalf("Probe025: %v", err)
	}
	if r.Status != "pass" {
		t.Fatalf("Probe025 status=%q detail=%q", r.Status, r.Detail)
	}
}

// PROBE-026 — missing-required-node case. Removing the systolic
// element from the ITEM_LIST surfaces a `required` issue at the
// /data/items path. Negative-case stability test.
func TestProbe026MissingNodes_MissingSystolic(t *testing.T) {
	body := loadFixture(t, "vital_signs")
	cases := []probes.ValidateCase{
		{
			Name:        "missing_systolic_items",
			OPT:         body,
			Composition: probes.SystolicItemsAbsent(),
			WantCodes:   []string{"required"},
		},
	}
	r, err := probes.Probe026MissingNodes(cases)
	if err != nil {
		t.Fatalf("Probe026: %v", err)
	}
	if r.Status != "pass" {
		t.Fatalf("Probe026 status=%q detail=%q", r.Status, r.Detail)
	}
	if r.Probe != "PROBE-026" {
		t.Errorf("Probe id = %q, want PROBE-026", r.Probe)
	}
}

// PROBE-026 — unknown archetype id under /content fires `slot_fill`.
// A Content[i] whose archetype_node_id matches no OPT root and no
// slot include fails the slot-fit fallback.
func TestProbe026MissingNodes_SlotFillUnknownArchetype(t *testing.T) {
	body := loadFixture(t, "vital_signs")
	// The OBSERVATION is RM-valid apart from its id, so only the slot fit
	// fails: template validation includes the RM floor, which walks it.
	cases := []probes.ValidateCase{
		{
			Name:        "unknown_archetype_id_under_content",
			OPT:         body,
			Composition: probes.UnknownArchetypeUnderContent(),
			WantCodes:   []string{"slot_fill"},
		},
	}
	r, err := probes.Probe026MissingNodes(cases)
	if err != nil {
		t.Fatalf("Probe026: %v", err)
	}
	if r.Status != "pass" {
		t.Fatalf("Probe026 status=%q detail=%q", r.Status, r.Detail)
	}
}

// PROBE-026 — DV_QUANTITY units outside the OPT-allowed list fires
// `primitive_unit_unknown` at the element's /value path.
func TestProbe026MissingNodes_PrimitiveUnitUnknown(t *testing.T) {
	body := loadFixture(t, "vital_signs")
	cases := []probes.ValidateCase{
		{
			Name:        "systolic_units_psi",
			OPT:         body,
			Composition: probes.SystolicUnitsPSI(),
			WantCodes:   []string{"primitive_unit_unknown"},
		},
	}
	r, err := probes.Probe026MissingNodes(cases)
	if err != nil {
		t.Fatalf("Probe026: %v", err)
	}
	if r.Status != "pass" {
		t.Fatalf("Probe026 status=%q detail=%q", r.Status, r.Detail)
	}
}

// PROBE-026 — category defining_code outside the OPT closed list
// fires `primitive_not_in_list` at /category/defining_code.
func TestProbe026MissingNodes_PrimitiveNotInList(t *testing.T) {
	body := loadFixture(t, "vital_signs")
	cases := []probes.ValidateCase{
		{
			Name:        "category_code_999",
			OPT:         body,
			Composition: probes.CategoryCode999(),
			// The OPT's code list refuses 999, and the RM floor, which
			// template validation includes, refuses it under its own code:
			// 999 is in no composition category of the openEHR terminology.
			WantCodes: []string{"primitive_not_in_list", "code_not_in_value_set"},
		},
	}
	r, err := probes.Probe026MissingNodes(cases)
	if err != nil {
		t.Fatalf("Probe026: %v", err)
	}
	if r.Status != "pass" {
		t.Fatalf("Probe026 status=%q detail=%q", r.Status, r.Detail)
	}
}

// PROBE-026 — empty events. An OBSERVATION whose Data.Events slice
// is present and empty fires BOTH `required` (existence lower ≥ 1) AND
// `cardinality` (the OPT pins child-count lower ≥ 1) at
// /data/events. Both constraints are independent — multiset
// reflects both for openEHR conformance. A nil Data.Events is absent
// (REQ-171 § Structural access): it breaks existence and has no item
// count to check, so it fires `required` alone.
func TestProbe026MissingNodes_EmptyEvents(t *testing.T) {
	body := loadFixture(t, "vital_signs")
	cases := []probes.ValidateCase{
		{
			Name:        "empty_events",
			OPT:         body,
			Composition: probes.EventsEmpty(),
			WantCodes:   []string{"required", "cardinality"},
		},
		{
			Name:        "absent_events",
			OPT:         body,
			Composition: probes.EventsAbsent(),
			WantCodes:   []string{"required"},
		},
	}
	r, err := probes.Probe026MissingNodes(cases)
	if err != nil {
		t.Fatalf("Probe026: %v", err)
	}
	if r.Status != "pass" {
		t.Fatalf("Probe026 status=%q detail=%q", r.Status, r.Detail)
	}
}

// PROBE-026 — per-child occurrences upper bound. vital_signs.opt
// pins the systolic ELEMENT (at0004) under /data/events[at0006]/data/items
// at occurrences 0..1; supplying two copies of the systolic
// element fires `cardinality` at the child path. Mirrors the
// unit-level TestValidateComposition_DuplicateSystolicOccurrences
// so other conformant implementations are bound to the same code
// multiset for the occurrences-upper-bound clause.
func TestProbe026MissingNodes_DuplicateSystolicOccurrences(t *testing.T) {
	body := loadFixture(t, "vital_signs")
	cases := []probes.ValidateCase{
		{
			Name:        "duplicate_systolic",
			OPT:         body,
			Composition: probes.SystolicTwice(),
			WantCodes:   []string{"cardinality"},
		},
	}
	r, err := probes.Probe026MissingNodes(cases)
	if err != nil {
		t.Fatalf("Probe026: %v", err)
	}
	if r.Status != "pass" {
		t.Fatalf("Probe026 status=%q detail=%q", r.Status, r.Detail)
	}
}

// PROBE-026 — C_SINGLE_ATTRIBUTE with two alternatives: PartyIdentified
// conforms to neither PARTY_SELF nor PARTY_RELATED (it is the supertype
// of PARTY_RELATED, not a descendant) → alternative_mismatch.
func TestProbe026MissingNodes_AlternativeMismatch(t *testing.T) {
	cases := []probes.ValidateCase{
		{
			Name:        "composer_party_identified",
			OPT:         []byte(probes.AlternativeMismatchOPT),
			Composition: probes.ComposerPartyIdentified(),
			WantCodes:   []string{"alternative_mismatch"},
		},
	}
	r, err := probes.Probe026MissingNodes(cases)
	if err != nil {
		t.Fatalf("Probe026: %v", err)
	}
	if r.Status != "pass" {
		t.Fatalf("Probe026 status=%q detail=%q", r.Status, r.Detail)
	}
}

// PROBE-026 — single-child C_SINGLE_ATTRIBUTE type constraint:
// PartyRelated under PARTY_SELF-only composer → rm_type_mismatch.
func TestProbe026MissingNodes_RMTypeMismatch(t *testing.T) {
	cases := []probes.ValidateCase{
		{
			Name:        "composer_party_related_single_alt",
			OPT:         []byte(probes.RMTypeMismatchOPT),
			Composition: probes.ComposerPartyRelated(),
			WantCodes:   []string{"rm_type_mismatch"},
		},
	}
	r, err := probes.Probe026MissingNodes(cases)
	if err != nil {
		t.Fatalf("Probe026: %v", err)
	}
	if r.Status != "pass" {
		t.Fatalf("Probe026 status=%q detail=%q", r.Status, r.Detail)
	}
}

// loadFixture reads a vendored template.opt from testkit/corpus/templates/.
func loadFixture(t *testing.T, name string) []byte {
	t.Helper()
	body, err := os.ReadFile(fixtures.TemplateOptForName(name))
	if err != nil {
		t.Fatalf("read fixture %q: %v", name, err)
	}
	return body
}
