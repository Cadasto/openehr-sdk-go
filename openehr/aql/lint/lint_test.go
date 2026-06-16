package lint_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/aql"
	"github.com/cadasto/openehr-sdk-go/openehr/aql/lint"
	"github.com/cadasto/openehr-sdk-go/openehr/aql/parse"
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
	if len(r.Issues) == 0 {
		t.Fatal("expected issues")
	}
	// REQ-109: Detail carries line:column before the ANTLR message.
	if !strings.Contains(r.Issues[0].Detail, "1:") {
		t.Fatalf("Detail missing position: %q", r.Issues[0].Detail)
	}
}

func TestLintUnparsedDocument(t *testing.T) {
	r := lint.Lint(&parse.Document{}, nil)
	if r.OK() || !has(r, "aql_syntax") {
		t.Fatalf("want aql_syntax for unparsed document, got %v", codes(r))
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

// A path with a wrong structural attribute (eventz) must warn, localised to
// the offending segment and path (GAP-5: payload assertion).
func TestLintBadPathWarns(t *testing.T) {
	const rawPath = "o/data[at0001]/eventz/value"
	c := mustCompile(t, "vital_signs")
	r := lint.LintString(
		"SELECT "+rawPath+" FROM OBSERVATION o[openEHR-EHR-OBSERVATION.blood_pressure.v1]",
		&lint.Options{Compiled: c},
	)
	if !has(r, "aql_path_not_in_template") {
		t.Fatalf("want aql_path_not_in_template warning, got %v", codes(r))
	}
	if !r.OK() {
		t.Fatalf("path warning must not make result not-OK: %v", codes(r))
	}
	var found bool
	for _, i := range r.Issues {
		if i.Code != "aql_path_not_in_template" {
			continue
		}
		found = true
		if i.Path != rawPath {
			t.Errorf("issue Path = %q, want %q", i.Path, rawPath)
		}
		if !strings.Contains(i.Detail, "eventz") {
			t.Errorf("Detail should name the diverging segment, got %q", i.Detail)
		}
	}
	if !found {
		t.Fatal("no aql_path_not_in_template issue to inspect")
	}
}

// TestLintWrongAtCodeLenientFallback documents Layer-3 predicate resolution:
// an unknown at-code on a multi-child segment falls back to the first child
// (mirroring template.NodeAt), so no path warning is emitted.
func TestLintWrongAtCodeLenientFallback(t *testing.T) {
	c := mustCompile(t, "vital_signs")
	r := lint.LintString(
		"SELECT o/data[at0001]/events[at9999]/data/items[at0004]/value/magnitude "+
			"FROM OBSERVATION o[openEHR-EHR-OBSERVATION.blood_pressure.v1]",
		&lint.Options{Compiled: c},
	)
	if has(r, "aql_path_not_in_template") {
		t.Fatalf("wrong at-code with first-child fallback must not warn, got %v", codes(r))
	}
}

// --- Review-driven coverage additions ---------------------------------------

// GAP-2: a referenced $param that IS bound produces no aql_unbound_param (the
// negative of TestLintUnboundParam — guards against an inverted condition).
func TestLintBoundParamClean(t *testing.T) {
	q := aql.NewQuery(
		"SELECT o FROM EHR e CONTAINS OBSERVATION o[openEHR-EHR-OBSERVATION.blood_pressure.v1] " +
			"WHERE e/ehr_id/value = $ehr_id",
	)
	q.Parameters = map[string]any{"ehr_id": "x"}
	r := lint.Lint(mustParse(t, q.Q), &lint.Options{Query: &q})
	if has(r, "aql_unbound_param") || has(r, "aql_unused_param") {
		t.Fatalf("bound+used param must be clean, got %v", codes(r))
	}
	if !r.OK() {
		t.Fatalf("expected OK, got %v", codes(r))
	}
}

// GAP-3: each hasIdentifiableScope disjunct (param-archetype, VERSION) on its
// own suppresses aql_from_archetype — guards the OR-chain against losing a
// disjunct. (Literal archetype and EHR are already covered elsewhere.)
func TestLintIdentifiableScopeSuppressesWarning(t *testing.T) {
	for _, q := range []string{
		"SELECT c FROM COMPOSITION c[$arch]",    // ParamArchetype
		"SELECT v FROM VERSION v[all_versions]", // Version (no EHR)
	} {
		r := lint.LintString(q, nil)
		if has(r, "aql_from_archetype") {
			t.Errorf("%q: must not warn aql_from_archetype, got %v", q, codes(r))
		}
	}
}

// GAP-4: with multiple unused params, the aql_unused_param issues appear in
// deterministic sorted-key order (the sort is a no-op with a single param).
func TestLintUnusedParamsSorted(t *testing.T) {
	q := aql.NewQuery("SELECT o FROM OBSERVATION o[openEHR-EHR-OBSERVATION.blood_pressure.v1]")
	q.Parameters = map[string]any{"zeta": 1, "alpha": 2, "mike": 3}
	r := lint.Lint(mustParse(t, q.Q), &lint.Options{Query: &q})
	var details []string
	for _, i := range r.Issues {
		if i.Code == "aql_unused_param" {
			details = append(details, i.Detail)
		}
	}
	if len(details) != 3 {
		t.Fatalf("want 3 aql_unused_param issues, got %d: %v", len(details), codes(r))
	}
	// Detail embeds the key after a shared prefix, so lexicographic order of
	// Details == sorted-key order.
	if !slices.IsSorted(details) {
		t.Fatalf("aql_unused_param not in sorted order: %v", details)
	}
}

// GAP-6: Lint is collect-all — one query tripping Layer-2 alias, Layer-2
// param, and Layer-3 archetype checks returns all three in a single pass.
func TestLintCollectAll(t *testing.T) {
	c := mustCompile(t, "vital_signs")
	q := aql.NewQuery(
		"SELECT x/foo FROM OBSERVATION o[openEHR-EHR-OBSERVATION.lab_result.v1] " +
			"WHERE o/data = $p",
	)
	r := lint.Lint(mustParse(t, q.Q), &lint.Options{Compiled: c, Query: &q})
	for _, want := range []string{
		"aql_unknown_alias", "aql_unbound_param", "aql_archetype_not_in_template",
	} {
		if !has(r, want) {
			t.Errorf("collect-all missing %s; got %v", want, codes(r))
		}
	}
}

// GAP-7: a nil *Document is guarded (no panic) and yields aql_syntax.
func TestLintNilDocument(t *testing.T) {
	r := lint.Lint(nil, nil)
	if r.OK() || !has(r, "aql_syntax") {
		t.Fatalf("Lint(nil) want aql_syntax, got %v", codes(r))
	}
}
