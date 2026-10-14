package validation_test

// coverage_unevaluated_test.go: REQ-171 § Coverage — Unevaluated lists,
// sorted and once each, the invariants of the classes the walk visited, those
// their ancestors declare included, that the invariant inventory does not
// classify as evaluated. Complete says nothing of it, and the zero Coverage,
// which a guard and ValidateAQL return, lists none.

import (
	"slices"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aql"
	"github.com/cadasto/openehr-sdk-go/openehr/aql/contain"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// sortedOnce reports whether keys is sorted with no key twice.
func sortedOnce(keys []string) bool {
	for i := 1; i < len(keys); i++ {
		if keys[i-1] >= keys[i] {
			return false
		}
	}
	return true
}

// REQ-171 Acceptance — a root of a class with invariants the SDK does not
// evaluate lists them in Coverage.Unevaluated: a DV_QUANTITY root, whose walk
// visits no other class, lists exactly the invariants of DV_AMOUNT,
// DV_QUANTIFIED and DV_ORDERED it is subject to and the floor does not
// evaluate, and not DV_ORDERED.Normal_status_validity, which it does. The
// result is complete all the same.
func TestCoverage_REQ171_UnevaluatedListsARootsInvariants(t *testing.T) {
	t.Parallel()
	want := []string{
		"DV_AMOUNT.Accuracy_is_percent_validity",
		"DV_AMOUNT.Accuracy_validity",
		"DV_ORDERED.Is_simple_validity",
		"DV_ORDERED.Normal_range_and_status_consistency",
		"DV_ORDERED.Other_reference_ranges_validity",
		"DV_QUANTIFIED.Magnitude_status_valid",
	}
	for _, tc := range []struct {
		name string
		root any
	}{
		{name: "by pointer", root: &rm.DVQuantity{Magnitude: 120, Units: "mm[Hg]"}},
		{name: "by value", root: rm.DVQuantity{Magnitude: 120, Units: "mm[Hg]"}},
	} {
		r := validation.ValidateRM(tc.root)
		if !r.OK || !slices.Equal(r.Coverage.Unevaluated, want) {
			t.Errorf("ValidateRM(DV_QUANTITY %s) OK = %t, Coverage.Unevaluated = %q, want OK and %q", tc.name, r.OK, r.Coverage.Unevaluated, want)
		}
		if !r.Coverage.Complete() {
			t.Errorf("ValidateRM(DV_QUANTITY %s) Coverage.Complete() = false, want true: Complete does not read Unevaluated", tc.name)
		}
	}
}

// REQ-171 Where the RM rules come from, § Coverage — an invariant another
// evaluated check decides is evaluated, so a root of a class subject to it
// does not list it: DV_TEXT.Valid_value and DV_PARAGRAPH.Items_valid, which
// the mandatory-presence rule decides, and the parts of an ISO 8601 value the
// REQ-123 parse of DV_DATE_TIME.Value_valid or DV_TIME.Value_valid refuses,
// such as Iso8601_date_time.Hour_valid. The parts that parse accepts against
// the BASE rule, a date-time without its day or month, a leap second, are
// still listed.
func TestCoverage_REQ171_UnevaluatedLeavesOutWhatAnotherCheckDecides(t *testing.T) {
	t.Parallel()
	ordered := []string{
		"DV_ORDERED.Is_simple_validity",
		"DV_ORDERED.Normal_range_and_status_consistency",
		"DV_ORDERED.Other_reference_ranges_validity",
		"DV_QUANTIFIED.Magnitude_status_valid",
	}
	for _, tc := range []struct {
		name string
		root any
		want []string
	}{
		{name: "DV_TEXT", root: &rm.DVText{Value: "Blood pressure"}, want: []string{"DV_TEXT.Formatting_valid"}},
		{name: "DV_PARAGRAPH", root: &rm.DVParagraph{Items: []rm.DVTextLike{&rm.DVText{Value: "Blood pressure"}}}, want: []string{"DV_TEXT.Formatting_valid"}},
		{name: "DV_DATE", root: &rm.DVDate{Value: "2024-03-15"}, want: ordered},
		{name: "DV_TIME", root: &rm.DVTime{Value: "10:30:00"}, want: append(slices.Clone(ordered), "Iso8601_time.Second_valid")},
		{name: "DV_DATE_TIME", root: &rm.DVDateTime{Value: "2024-03-15T10:30:00Z"}, want: append(slices.Clone(ordered),
			"Iso8601_date_time.Day_valid",
			"Iso8601_date_time.Month_valid",
			"Iso8601_date_time.Partial_validity_day",
			"Iso8601_date_time.Partial_validity_hour",
			"Iso8601_date_time.Partial_validity_month",
			"Iso8601_date_time.Partial_validity_year",
			"Iso8601_date_time.Second_valid",
		)},
	} {
		r := validation.ValidateRM(tc.root)
		if !r.OK || !slices.Equal(r.Coverage.Unevaluated, tc.want) {
			t.Errorf("ValidateRM(%s) OK = %t, Coverage.Unevaluated = %q, want OK and %q", tc.name, r.OK, r.Coverage.Unevaluated, tc.want)
		}
	}
}

// REQ-171 § Coverage — over every vendored corpus document, Unevaluated is
// sorted with no repeat, lists invariants of LOCATABLE and DV_TEXT the SDK
// does not evaluate and none of the RM rules it does, those the
// mandatory-presence rule decides included, and is the same with
// the template as without: with no limit met, the floor visits every class
// the template walk enters.
func TestCoverage_REQ171_UnevaluatedOverTheCorpus(t *testing.T) {
	v := validation.New()
	templated := 0
	for _, doc := range corpusDocuments(t) {
		floor := validation.ValidateRM(doc.root).Coverage.Unevaluated
		if !sortedOnce(floor) {
			t.Errorf("ValidateRM(%s) Coverage.Unevaluated = %q, want it sorted with no repeat", doc.rel.Rel, floor)
		}
		for _, evaluated := range []string{"DV_TEXT.Mappings_valid", "ELEMENT.Inv_null_flavour_indicated", "CODE_PHRASE.Code_string_valid", "LOCATABLE.Archetype_node_id_valid", "DV_TEXT.Valid_value"} {
			if slices.Contains(floor, evaluated) {
				t.Errorf("ValidateRM(%s) Coverage.Unevaluated lists %s, which the floor evaluates", doc.rel.Rel, evaluated)
			}
		}
		for _, want := range []string{"DV_TEXT.Formatting_valid", "LOCATABLE.Links_valid"} {
			if !slices.Contains(floor, want) {
				t.Errorf("ValidateRM(%s) Coverage.Unevaluated = %q, want it to list %s", doc.rel.Rel, floor, want)
			}
		}
		if doc.c == nil {
			continue
		}
		templated++
		r := v.Validate(doc.root, doc.c)
		if !slices.Equal(r.Coverage.Unevaluated, floor) {
			t.Errorf("Validate(%s) Coverage.Unevaluated = %q, want what ValidateRM lists, %q", doc.rel.Rel, r.Coverage.Unevaluated, floor)
		}
	}
	if templated == 0 {
		t.Fatal("no corpus document was validated against its OPT; the check has gone blind")
	}
}

// REQ-171 § Coverage — with a template, Unevaluated covers the classes either
// walk visited: when the visit limit stops the floor at the root, the classes
// the template walk entered below it are still listed.
func TestCoverage_REQ171_UnevaluatedUnitesTheTemplateWalk(t *testing.T) {
	t.Parallel()
	c := mustCompile(t, "vital_signs")
	v := validation.New(validation.WithLimits(validation.Limits{Nodes: 1}))
	floor := v.Validate(validVitalSignsComposition(), nil)
	if len(floor.Coverage.Stopped) != 1 {
		t.Fatalf("precondition: Validate(vital signs, nil) with a visit limit of 1 Coverage.Stopped = %q, want one stop", floor.Coverage.Stopped)
	}
	const entry = "ENTRY.Other_participations_valid"
	if slices.Contains(floor.Coverage.Unevaluated, entry) {
		t.Fatalf("precondition: Validate(vital signs, nil) with a visit limit of 1 lists %s, want the floor stopped before any ENTRY", entry)
	}
	r := v.Validate(validVitalSignsComposition(), c)
	if !slices.Contains(r.Coverage.Unevaluated, entry) || !sortedOnce(r.Coverage.Unevaluated) {
		t.Errorf("Validate(vital signs, c) with a visit limit of 1 Coverage.Unevaluated = %q, want it sorted and to list %s, from the OBSERVATION the template walk entered", r.Coverage.Unevaluated, entry)
	}
	for _, key := range floor.Coverage.Unevaluated {
		if !slices.Contains(r.Coverage.Unevaluated, key) {
			t.Errorf("Validate(vital signs, c) with a visit limit of 1 Coverage.Unevaluated = %q, want it to keep %s, which the floor lists", r.Coverage.Unevaluated, key)
		}
	}
}

// REQ-171 § Coverage — a guard, ValidateAQL and ValidateAQLWithTypeRelation
// list no unevaluated invariant: they visit no value.
func TestCoverage_REQ171_GuardsAndAQLListNoUnevaluated(t *testing.T) {
	t.Parallel()
	c := mustCompile(t, "vital_signs")
	for _, tc := range []struct {
		name string
		r    validation.Result
	}{
		{name: "Validate(nil, c)", r: validation.Validate(nil, c)},
		{name: "Validate(root, nil)", r: validation.Validate(validVitalSignsComposition(), nil)},
		{name: "New().Validate(root, template without a root)", r: validation.New().Validate(validVitalSignsComposition(), &templatecompile.Compiled{})},
		{name: "ValidateRM(nil)", r: validation.ValidateRM(nil)},
		{name: "ValidateRM(root outside the RM set)", r: validation.ValidateRM(struct{}{})},
		{name: "ValidateRMEHRStatus(nil)", r: validation.ValidateRMEHRStatus(nil)},
		{name: "ValidateRMEHRStatusBytes(array)", r: validation.ValidateRMEHRStatusBytes([]byte(`[]`))},
		{name: "ValidateAQL", r: validation.ValidateAQL(aql.NewQuery("SELECT * FROM EHR e CONTAINS COMPOSITION c[openEHR-EHR-COMPOSITION.encounter.v1]"), c)},
		{name: "ValidateAQLWithTypeRelation", r: validation.ValidateAQLWithTypeRelation(aql.NewQuery("SELEC x"), c, contain.Default())},
	} {
		if len(tc.r.Issues) == 0 {
			t.Errorf("%s reported nothing; want an issue, so the case is not vacuous", tc.name)
		}
		if tc.r.Coverage.Unevaluated != nil {
			t.Errorf("%s Coverage.Unevaluated = %q, want nil", tc.name, tc.r.Coverage.Unevaluated)
		}
	}
}

// REQ-171 § Coverage — Complete says nothing of Unevaluated: a Coverage that
// lists only unevaluated invariants is complete.
func TestCoverage_REQ171_CompleteIgnoresUnevaluated(t *testing.T) {
	t.Parallel()
	c := validation.Coverage{Unevaluated: []string{"DV_AMOUNT.Accuracy_validity"}}
	if !c.Complete() {
		t.Errorf("Coverage{Unevaluated: %q}.Complete() = false, want true", c.Unevaluated)
	}
}

// REQ-171 § Coverage, REQ-112 — the presence-aware EHR_STATUS entry carries
// the Coverage of the value walk it runs: what ValidateRMEHRStatus reports for
// the decoded status. The status omits subject, so the entry replaces the
// walk's findings with its key-presence finding and rebuilds the Result, and
// the Coverage must survive that rebuild.
func TestValidateRMEHRStatusBytes_REQ171_ListsUnevaluated(t *testing.T) {
	t.Parallel()
	data := []byte(`{
		"_type": "EHR_STATUS",
		"name": {"_type": "DV_TEXT", "value": "EHR Status"},
		"archetype_node_id": "openEHR-EHR-EHR_STATUS.generic.v1",
		"archetype_details": {"_type": "ARCHETYPED",
			"archetype_id": {"_type": "ARCHETYPE_ID", "value": "openEHR-EHR-EHR_STATUS.generic.v1"},
			"rm_version": "1.1.0"},
		"is_modifiable": true,
		"is_queryable": true
	}`)
	var status rm.EHRStatus
	if err := canjson.Unmarshal(data, &status); err != nil {
		t.Fatalf("canjson.Unmarshal(EHR_STATUS): %v", err)
	}
	want := validation.ValidateRMEHRStatus(&status).Coverage
	got := validation.ValidateRMEHRStatusBytes(data)
	if !slices.ContainsFunc(got.Issues, func(i validation.Issue) bool { return i.Path == "/subject" && i.Code == "required" }) {
		t.Fatalf("precondition: ValidateRMEHRStatusBytes(EHR_STATUS without subject) issues %+v, want required at /subject, so the key-absence path runs", got.Issues)
	}
	if len(want.Unevaluated) == 0 || !slices.Equal(got.Coverage.Unevaluated, want.Unevaluated) {
		t.Errorf("ValidateRMEHRStatusBytes(EHR_STATUS without subject) Coverage.Unevaluated = %q, want %q, what ValidateRMEHRStatus lists, and not empty", got.Coverage.Unevaluated, want.Unevaluated)
	}
	if !slices.Equal(got.Coverage.Stopped, want.Stopped) || !slices.Equal(got.Coverage.External, want.External) {
		t.Errorf("ValidateRMEHRStatusBytes(EHR_STATUS without subject) Coverage Stopped %q External %q, want %q and %q, what ValidateRMEHRStatus reports", got.Coverage.Stopped, got.Coverage.External, want.Stopped, want.External)
	}
}
