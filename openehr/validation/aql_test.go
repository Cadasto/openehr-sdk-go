package validation_test

import (
	"errors"
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/aql"
	"github.com/cadasto/openehr-sdk-go/openehr/aql/contain"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

func compileVitalSigns(t *testing.T) *templatecompile.Compiled {
	t.Helper()
	opt, err := template.ParseFile(fixtures.TemplateOptForName("vital_signs"))
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	c, err := templatecompile.Compile(opt)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	return c
}

// ValidateAQL surfaces a syntax failure as code "aql_syntax", which Issue.Err
// maps to the ErrAQLSyntax sentinel (REQ-109 → REQ-102 bridge).
func TestValidateAQL_Syntax(t *testing.T) {
	r := validation.ValidateAQL(aql.NewQuery("SELECT FROM EHR e"), nil)
	if r.OK {
		t.Fatal("expected not-OK for a syntax error")
	}
	var matched bool
	for _, i := range r.Issues {
		if i.Code == "aql_syntax" && errors.Is(i.Err(), validation.ErrAQLSyntax) {
			matched = true
		}
	}
	if !matched {
		t.Fatalf("want aql_syntax → ErrAQLSyntax; issues = %+v", r.Issues)
	}
}

func TestValidateAQL_Empty(t *testing.T) {
	r := validation.ValidateAQL(aql.NewQuery("   "), nil)
	if r.OK {
		t.Fatal("expected not-OK for an empty query")
	}
	var matched bool
	for _, i := range r.Issues {
		if i.Code == "aql_empty" && errors.Is(i.Err(), validation.ErrAQLSyntax) {
			matched = true
		}
	}
	if !matched {
		t.Fatalf("want aql_empty → ErrAQLSyntax; issues = %+v", r.Issues)
	}
}

// A query naming an archetype absent from the compiled template fails with
// aql_archetype_not_in_template (Layer 3).
func TestValidateAQL_ArchetypeNotInTemplate(t *testing.T) {
	c := compileVitalSigns(t)
	q := aql.NewQuery("SELECT o FROM OBSERVATION o[openEHR-EHR-OBSERVATION.lab_result.v1]")
	r := validation.ValidateAQL(q, c)
	if !containsCode(r.Issues, "aql_archetype_not_in_template") {
		t.Fatalf("want aql_archetype_not_in_template; issues = %+v", r.Issues)
	}
}

// A well-formed query against an archetype present in the template is clean.
func TestValidateAQL_Clean(t *testing.T) {
	c := compileVitalSigns(t)
	q := aql.NewQuery(
		"SELECT o/data[at0001]/events[at0006]/data[at0003]/items[at0004]/value/magnitude " +
			"FROM OBSERVATION o[openEHR-EHR-OBSERVATION.blood_pressure.v1]",
	)
	r := validation.ValidateAQL(q, c)
	if !r.OK {
		t.Fatalf("expected clean result, got %+v", r.Issues)
	}
}

// A warning-only lint result (no Error-severity issue) bridges to Result.OK
// == true: aql_from_archetype is advisory and must not flip the gate.
func TestValidateAQL_WarningOnlyIsOK(t *testing.T) {
	c := compileVitalSigns(t)
	r := validation.ValidateAQL(aql.NewQuery("SELECT c FROM COMPOSITION c"), c)
	if !r.OK {
		t.Fatalf("warning-only result must be OK, got %+v", r.Issues)
	}
	if !containsCode(r.Issues, "aql_from_archetype") {
		t.Fatalf("want aql_from_archetype warning present; issues = %+v", r.Issues)
	}
	if r.Issues[0].Severity != validation.Warning {
		t.Errorf("issue severity = %v, want warning", r.Issues[0].Severity)
	}
}

// Parameter binding bridges through: an unbound $param is an Error-severity
// issue carried into the validation Result.
func TestValidateAQL_UnboundParam(t *testing.T) {
	c := compileVitalSigns(t)
	q := aql.NewQuery(
		"SELECT o FROM EHR e CONTAINS OBSERVATION o[openEHR-EHR-OBSERVATION.blood_pressure.v1] " +
			"WHERE e/ehr_id/value = $ehr_id",
	)
	r := validation.ValidateAQL(q, c)
	if !containsCode(r.Issues, "aql_unbound_param") {
		t.Fatalf("want aql_unbound_param; issues = %+v", r.Issues)
	}
}

// ValidateAQL delegates to ValidateAQLWithTypeRelation with a nil relation,
// and a nil relation SELECTS the REQ-160 default rather than switching the
// containment group off (REQ-161 § Relation supply). The two entry points must
// therefore be indistinguishable, or the escape hatch has changed the default
// behaviour of every existing caller.
func TestValidateAQLWithTypeRelation_NilIsValidateAQL(t *testing.T) {
	// Asserted directly, not only through the equality below: comparing the two
	// entry points cannot fail while one delegates to the other, so the content
	// of this test is that nil still JUDGES containment.
	const impossible = "SELECT c FROM OBSERVATION o CONTAINS COMPOSITION c"
	if r := validation.ValidateAQLWithTypeRelation(aql.NewQuery(impossible), nil, nil); !containsCode(r.Issues, "aql_impossible_containment") {
		t.Fatalf("a nil relation must select the REQ-160 default, not switch the group off; issues = %+v", r.Issues)
	}

	for _, q := range []string{
		impossible,                    // Error-severity containment
		"SELECT c FROM COMPOSITION c", // warning only
		"SELECT FROM EHR e",           // Layer-1 failure
	} {
		base := validation.ValidateAQL(aql.NewQuery(q), nil)
		with := validation.ValidateAQLWithTypeRelation(aql.NewQuery(q), nil, nil)
		if base.OK != with.OK || len(base.Issues) != len(with.Issues) {
			t.Fatalf("%q: ValidateAQL = %+v, WithTypeRelation(nil) = %+v", q, base, with)
		}
		for i := range base.Issues {
			if base.Issues[i] != with.Issues[i] {
				t.Errorf("%q: issue %d = %+v, want %+v", q, i, with.Issues[i], base.Issues[i])
			}
		}
	}
}

// A deployment overlay retires the containment Error the pinned RM raises,
// through the validation seam rather than by abandoning it for lint.LintString
// (REQ-161 § Relation supply). The EHR IM versions no parties, so
// `EHR CONTAINS VERSIONED_PARTY` is Never by default — the case REQ-160
// § Extensibility names.
func TestValidateAQLWithTypeRelation_OverlayRetiresContainmentError(t *testing.T) {
	q := aql.NewQuery("SELECT p FROM EHR e CONTAINS VERSIONED_PARTY vp[uid/value=$vp] CONTAINS PERSON p")
	q.Parameters = map[string]any{"vp": "8849182c-82ad-4088-a07f-48ead4180515::local.ehrbase.org::1"}

	// Premise: the default relation must actually raise the Error, or this
	// test would pass for the wrong reason.
	if base := validation.ValidateAQL(q, nil); !containsCode(base.Issues, "aql_impossible_containment") {
		t.Fatalf("premise broken: default relation raises no aql_impossible_containment; issues = %+v", base.Issues)
	}

	rel := contain.Default().WithOverlay(contain.Edge{From: "EHR", To: "VERSIONED_PARTY"})
	got := validation.ValidateAQLWithTypeRelation(q, nil, rel)
	if containsCode(got.Issues, "aql_impossible_containment") {
		t.Fatalf("overlay did not retire the containment Error; issues = %+v", got.Issues)
	}
	if !got.OK {
		t.Errorf("result must be OK once the deployment's own route is declared; issues = %+v", got.Issues)
	}
}

// A relation governs the containment codes and nothing else: it cannot retire
// a Layer-1 syntax failure, a Layer-2 shape or parameter finding, or a Layer-3
// template finding. An overlay wide enough to admit every pair is the strongest
// form of the question — if anything outside the containment group survives it,
// the seam is passing the relation somewhere it does not belong.
func TestValidateAQLWithTypeRelation_DoesNotReachOtherLayers(t *testing.T) {
	wide := contain.Default().WithOverlay(
		contain.Edge{From: "EHR", To: "VERSIONED_PARTY"},
		contain.Edge{From: "OBSERVATION", To: "COMPOSITION"},
	)
	c := compileVitalSigns(t)

	for _, tc := range []struct {
		name     string
		query    string
		compiled *templatecompile.Compiled
		want     string
	}{
		{"layer 1 syntax", "SELECT FROM EHR e", nil, "aql_syntax"},
		{
			"layer 2 parameter binding",
			"SELECT o FROM EHR e CONTAINS OBSERVATION o[openEHR-EHR-OBSERVATION.blood_pressure.v1] WHERE e/ehr_id/value = $ehr_id",
			c, "aql_unbound_param",
		},
		{
			"layer 3 archetype not in template",
			"SELECT o FROM EHR e CONTAINS OBSERVATION o[openEHR-EHR-OBSERVATION.body_mass_index.v2]",
			c, "aql_archetype_not_in_template",
		},
	} {
		got := validation.ValidateAQLWithTypeRelation(aql.NewQuery(tc.query), tc.compiled, wide)
		if !containsCode(got.Issues, tc.want) {
			t.Errorf("%s: want %s to survive a wide overlay; issues = %+v", tc.name, tc.want, got.Issues)
		}
	}
}

// The three REQ-161 portability advisories consult no relation, so an overlay
// cannot retire them either — they ask a class question of the pinned RM, or
// read the query's own shape.
func TestValidateAQLWithTypeRelation_DoesNotReachPortabilityCodes(t *testing.T) {
	wide := contain.Default().WithOverlay(contain.Edge{From: "EHR", To: "VERSIONED_PARTY"})

	for _, tc := range []struct{ name, query, want string }{
		{"version predicate", "SELECT v FROM EHR e CONTAINS VERSION v", "aql_version_no_predicate"},
		{"unreferenced versioned object", "SELECT e FROM EHR e CONTAINS VERSIONED_COMPOSITION vc", "aql_versioned_object_unreferenced"},
		{
			"fan-out row grain",
			"SELECT o, ev FROM COMPOSITION c CONTAINS (OBSERVATION o AND EVALUATION ev)",
			"aql_fanout_row_grain",
		},
	} {
		got := validation.ValidateAQLWithTypeRelation(aql.NewQuery(tc.query), nil, wide)
		if !containsCode(got.Issues, tc.want) {
			t.Errorf("%s: want %s to survive a wide overlay; issues = %+v", tc.name, tc.want, got.Issues)
		}
	}
}
