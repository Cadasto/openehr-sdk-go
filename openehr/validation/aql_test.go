package validation_test

import (
	"errors"
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/aql"
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

func hasCode(r validation.Result, code string) bool {
	for _, i := range r.Issues {
		if i.Code == code {
			return true
		}
	}
	return false
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
	if !hasCode(r, "aql_archetype_not_in_template") {
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
	if !hasCode(r, "aql_from_archetype") {
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
	if !hasCode(r, "aql_unbound_param") {
		t.Fatalf("want aql_unbound_param; issues = %+v", r.Issues)
	}
}
