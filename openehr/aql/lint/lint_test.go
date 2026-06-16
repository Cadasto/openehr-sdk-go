package lint_test

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/aql"
	"github.com/cadasto/openehr-sdk-go/openehr/aql/lint"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/testkit/fixtures"
)

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

func codes(r lint.Result) []string {
	out := make([]string, len(r.Issues))
	for i, is := range r.Issues {
		out[i] = is.Code
	}
	return out
}

func has(r lint.Result, code string) bool {
	for _, i := range r.Issues {
		if i.Code == code {
			return true
		}
	}
	return false
}

// --- Layer 1: syntax / empty -------------------------------------------------

func TestLintStringEmpty(t *testing.T) {
	r := lint.LintString("   ", nil)
	if r.OK() || !has(r, "aql_empty") {
		t.Fatalf("want aql_empty error, got %v", codes(r))
	}
}

func TestLintStringSyntax(t *testing.T) {
	r := lint.LintString("SELECT FROM EHR e", nil)
	if r.OK() || !has(r, "aql_syntax") {
		t.Fatalf("want aql_syntax error, got %v", codes(r))
	}
}

// --- Layer 2: shape + params -------------------------------------------------

func TestLintCleanNoTemplate(t *testing.T) {
	r := lint.LintString(
		"SELECT o FROM EHR e CONTAINS OBSERVATION o[openEHR-EHR-OBSERVATION.blood_pressure.v1]",
		nil,
	)
	if !r.OK() {
		t.Fatalf("expected clean, got %v", codes(r))
	}
}

func TestLintUnknownAlias(t *testing.T) {
	// Path rooted at alias "x", but FROM binds "o".
	r := lint.LintString(
		"SELECT x/data FROM OBSERVATION o[openEHR-EHR-OBSERVATION.blood_pressure.v1]",
		nil,
	)
	if r.OK() || !has(r, "aql_unknown_alias") {
		t.Fatalf("want aql_unknown_alias, got %v", codes(r))
	}
}

func TestLintFromArchetypeWarning(t *testing.T) {
	r := lint.LintString("SELECT c FROM COMPOSITION c", nil)
	if !has(r, "aql_from_archetype") {
		t.Fatalf("want aql_from_archetype warning, got %v", codes(r))
	}
	if !r.OK() {
		t.Fatalf("warning must not make result not-OK: %v", codes(r))
	}
}

func TestLintUnboundParam(t *testing.T) {
	q := aql.NewQuery(
		"SELECT o FROM EHR e CONTAINS OBSERVATION o[openEHR-EHR-OBSERVATION.blood_pressure.v1] " +
			"WHERE e/ehr_id/value = $ehr_id",
	)
	// No Parameters bound at all.
	doc := mustParse(t, q.Q)
	r := lint.Lint(doc, &lint.Options{Query: &q})
	if r.OK() || !has(r, "aql_unbound_param") {
		t.Fatalf("want aql_unbound_param, got %v", codes(r))
	}
}

func TestLintUnusedParamWarning(t *testing.T) {
	q := aql.NewQuery(
		"SELECT o FROM EHR e CONTAINS OBSERVATION o[openEHR-EHR-OBSERVATION.blood_pressure.v1] " +
			"WHERE e/ehr_id/value = $ehr_id",
	)
	q.Parameters = map[string]any{"ehr_id": "x", "spurious": 1}
	doc := mustParse(t, q.Q)
	r := lint.Lint(doc, &lint.Options{Query: &q})
	if !has(r, "aql_unused_param") {
		t.Fatalf("want aql_unused_param warning, got %v", codes(r))
	}
	if !r.OK() {
		t.Fatalf("unused-param warning must not make result not-OK: %v", codes(r))
	}
}

// --- Layer 3: template -------------------------------------------------------

func TestLintArchetypeNotInTemplate(t *testing.T) {
	c := mustCompile(t, "vital_signs")
	r := lint.LintString(
		"SELECT o FROM OBSERVATION o[openEHR-EHR-OBSERVATION.lab_result.v1]",
		&lint.Options{Compiled: c},
	)
	if r.OK() || !has(r, "aql_archetype_not_in_template") {
		t.Fatalf("want aql_archetype_not_in_template, got %v", codes(r))
	}
}

func TestLintArchetypeInTemplateClean(t *testing.T) {
	c := mustCompile(t, "vital_signs")
	r := lint.LintString(
		"SELECT o FROM OBSERVATION o[openEHR-EHR-OBSERVATION.blood_pressure.v1]",
		&lint.Options{Compiled: c},
	)
	if !r.OK() || len(r.Issues) != 0 {
		t.Fatalf("expected clean, got %v", codes(r))
	}
}

// A real, structurally valid blood-pressure path (ending in the RM leaf
// /value/magnitude) must NOT warn.
func TestLintValidPathNoWarning(t *testing.T) {
	c := mustCompile(t, "vital_signs")
	r := lint.LintString(
		"SELECT o/data[at0001]/events[at0006]/data[at0003]/items[at0004]/value/magnitude "+
			"FROM OBSERVATION o[openEHR-EHR-OBSERVATION.blood_pressure.v1]",
		&lint.Options{Compiled: c},
	)
	if has(r, "aql_path_not_in_template") {
		t.Fatalf("valid path must not warn, got %v", codes(r))
	}
}

// A path with a wrong structural attribute (eventz) must warn.
func TestLintBadPathWarns(t *testing.T) {
	c := mustCompile(t, "vital_signs")
	r := lint.LintString(
		"SELECT o/data[at0001]/eventz/value "+
			"FROM OBSERVATION o[openEHR-EHR-OBSERVATION.blood_pressure.v1]",
		&lint.Options{Compiled: c},
	)
	if !has(r, "aql_path_not_in_template") {
		t.Fatalf("want aql_path_not_in_template warning, got %v", codes(r))
	}
	if !r.OK() {
		t.Fatalf("path warning must not make result not-OK: %v", codes(r))
	}
}
