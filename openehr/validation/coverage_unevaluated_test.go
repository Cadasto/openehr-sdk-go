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
// still listed. DV_PROPORTION.Is_integral_validity, which the DV_PROPORTION
// precision check decides as DV_PROPORTION.Precision_validity, is not listed
// on any DV_PROPORTION root, whatever its precision; nor are the
// proportion-kind rules and Valid_denominator, which the same check
// evaluates (REQ-112).
func TestCoverage_REQ171_UnevaluatedLeavesOutWhatAnotherCheckDecides(t *testing.T) {
	t.Parallel()
	ordered := []string{
		"DV_ORDERED.Is_simple_validity",
		"DV_ORDERED.Normal_range_and_status_consistency",
		"DV_ORDERED.Other_reference_ranges_validity",
		"DV_QUANTIFIED.Magnitude_status_valid",
	}
	proportion := []string{
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
		{name: "DV_PROPORTION, precision 0 over whole operands", root: &rm.DVProportion{Numerator: 1, Denominator: 2, Precision: new(rm.Integer(0))}, want: proportion},
		{name: "DV_PROPORTION, precision -1 over fractional operands", root: &rm.DVProportion{Numerator: 1.5, Denominator: 2.5, Precision: new(rm.Integer(-1))}, want: proportion},
		{name: "DV_PROPORTION, no precision", root: &rm.DVProportion{Numerator: 1.5, Denominator: 2}, want: proportion},
	} {
		r := validation.ValidateRM(tc.root)
		if !r.OK || !slices.Equal(r.Coverage.Unevaluated, tc.want) {
			t.Errorf("ValidateRM(%s) OK = %t, Coverage.Unevaluated = %q, want OK and %q", tc.name, r.OK, r.Coverage.Unevaluated, tc.want)
		}
	}
}

// admissionCodes are the codes of the findings by which a template refuses
// content: the walk reports the content once and does not enter it.
var admissionCodes = []string{"prohibited", "slot_fill", "rm_type_mismatch", "alternative_mismatch"}

// REQ-171 § Coverage, § One walk — over every vendored corpus document,
// Unevaluated is sorted with no repeat, lists invariants of LOCATABLE and
// DV_TEXT the SDK does not evaluate and none of the RM rules it does, those
// the mandatory-presence rule decides included. With the template it lists
// the invariants of the classes the walk visited: what the RM rules alone
// list, where the template refuses nothing, and no more than that where it
// refuses content, whose classes the walk does not enter.
func TestCoverage_REQ171_UnevaluatedOverTheCorpus(t *testing.T) {
	v := validation.New()
	templated, refusing := 0, 0
	for _, doc := range corpusDocuments(t) {
		floor := validation.ValidateRM(doc.root).Coverage.Unevaluated
		if !sortedOnce(floor) {
			t.Errorf("ValidateRM(%s) Coverage.Unevaluated = %q, want it sorted with no repeat", doc.rel.Rel, floor)
		}
		for _, evaluated := range []string{"DV_TEXT.Mappings_valid", "ELEMENT.Inv_null_flavour_indicated", "CODE_PHRASE.Code_string_valid", "LOCATABLE.Archetype_node_id_valid", "DV_TEXT.Valid_value"} {
			if slices.Contains(floor, evaluated) {
				t.Errorf("ValidateRM(%s) Coverage.Unevaluated lists %s, which the RM rules evaluate", doc.rel.Rel, evaluated)
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
		refused := slices.ContainsFunc(r.Issues, func(i validation.Issue) bool { return slices.Contains(admissionCodes, i.Code) })
		if refused {
			refusing++
		}
		if !sortedOnce(r.Coverage.Unevaluated) {
			t.Errorf("Validate(%s) Coverage.Unevaluated = %q, want it sorted with no repeat", doc.rel.Rel, r.Coverage.Unevaluated)
		}
		if extra := slices.DeleteFunc(slices.Clone(r.Coverage.Unevaluated), func(u string) bool { return slices.Contains(floor, u) }); len(extra) > 0 {
			t.Errorf("Validate(%s) Coverage.Unevaluated lists %q, which ValidateRM does not: the walk with the template visited a class the walk without it did not", doc.rel.Rel, extra)
		}
		if !refused && !slices.Equal(r.Coverage.Unevaluated, floor) {
			t.Errorf("Validate(%s) Coverage.Unevaluated = %q, want what ValidateRM lists, %q: the template refuses nothing", doc.rel.Rel, r.Coverage.Unevaluated, floor)
		}
	}
	if templated == 0 || refusing == 0 {
		t.Fatalf("validated %d corpus documents against their OPTs, %d of them with refused content; want some of each, or the check has gone blind", templated, refusing)
	}
}

// REQ-171 § Coverage, § Surface — the limits bound the walk with a template
// as without one, so Unevaluated lists the classes of the values the walk
// visited before a limit stopped it, and no class below: with a visit limit
// of 1 the walk stops at the root of a vital signs composition, with its
// template as with none, and lists no invariant of the OBSERVATION below.
func TestCoverage_REQ171_UnevaluatedListsTheClassesTheWalkVisited(t *testing.T) {
	t.Parallel()
	c := mustCompile(t, "vital_signs")
	v := validation.New(validation.WithLimits(validation.Limits{Nodes: 1}))
	const entry = "ENTRY.Other_participations_valid"
	if full := validation.Validate(validVitalSignsComposition(), c); !slices.Contains(full.Coverage.Unevaluated, entry) {
		t.Fatalf("precondition: Validate(vital signs, c) Coverage.Unevaluated = %q, want it to list %s, from the OBSERVATION the walk enters", full.Coverage.Unevaluated, entry)
	}
	floor := v.Validate(validVitalSignsComposition(), nil)
	r := v.Validate(validVitalSignsComposition(), c)
	for name, res := range map[string]validation.Result{"nil": floor, "c": r} {
		if len(res.Coverage.Stopped) != 1 {
			t.Errorf("Validate(vital signs, %s) with a visit limit of 1 Coverage.Stopped = %q, want one stop", name, res.Coverage.Stopped)
		}
		if slices.Contains(res.Coverage.Unevaluated, entry) || !sortedOnce(res.Coverage.Unevaluated) {
			t.Errorf("Validate(vital signs, %s) with a visit limit of 1 Coverage.Unevaluated = %q, want it sorted and without %s: the walk stopped before any ENTRY", name, res.Coverage.Unevaluated, entry)
		}
	}
	if !slices.Equal(r.Coverage.Unevaluated, floor.Coverage.Unevaluated) {
		t.Errorf("Validate(vital signs, c) with a visit limit of 1 Coverage.Unevaluated = %q, want what the walk with no template lists, %q", r.Coverage.Unevaluated, floor.Coverage.Unevaluated)
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
