package validation_test

// validator_test.go: REQ-171 § Surface, REQ-112 § Surface — one validator
// value. Validate on a Validator with a nil template applies the RM rules
// alone and returns what ValidateRM returns; with a template it returns what
// the package-level Validate returns. The package-level entry points behave
// as a Validator built with no options and keep their nil guards, whose path
// is empty. A Validator holds no per-call state, so one serves many
// goroutines.

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/templatecompile"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

// coverageText renders r's coverage for a failure message.
func coverageText(r validation.Result) string {
	return fmt.Sprintf("OK %t, issues %q, stopped %q, external %q", r.OK, issueKeys(r.Issues), r.Coverage.Stopped, r.Coverage.External)
}

// assertSameResult fails the test unless got and want are equal in every
// field: OK, each issue (its value included) in order, and the coverage.
func assertSameResult(t *testing.T, call string, got, want validation.Result) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s = %s\nwant %s", call, coverageText(got), coverageText(want))
	}
}

// REQ-171 Acceptance, REQ-112 § Surface — for every root of the vendored
// corpus, Validate(root, nil) on a Validator built with no options returns
// what ValidateRM returns, issues and coverage; with the root's OPT it
// returns what the package-level Validate returns. The zero Validator and a
// nil *Validator return the same as New().
func TestValidator_REQ171_REQ112_SameAsThePackageEntries(t *testing.T) {
	v := validation.New()
	var zero validation.Validator
	var none *validation.Validator
	for _, doc := range corpusDocuments(t) {
		floor := validation.ValidateRM(doc.root)
		assertSameResult(t, "New().Validate("+doc.rel.Rel+", nil)", v.Validate(doc.root, nil), floor)
		assertSameResult(t, "Validator{}.Validate("+doc.rel.Rel+", nil)", zero.Validate(doc.root, nil), floor)
		assertSameResult(t, "(*Validator)(nil).Validate("+doc.rel.Rel+", nil)", none.Validate(doc.root, nil), floor)
		if doc.c == nil {
			continue
		}
		templated := validation.Validate(doc.root, doc.c)
		assertSameResult(t, "New().Validate("+doc.rel.Rel+", c)", v.Validate(doc.root, doc.c), templated)
		assertSameResult(t, "Validator{}.Validate("+doc.rel.Rel+", c)", zero.Validate(doc.root, doc.c), templated)
		assertSameResult(t, "(*Validator)(nil).Validate("+doc.rel.Rel+", c)", none.Validate(doc.root, doc.c), templated)
	}
}

// REQ-171 § Surface — a Validator holds no per-call state after New, so one
// value shared by parallel calls over several corpus documents, with and
// without their templates, returns what sequential calls return. Run under
// the race detector, a write to shared state also fails here.
func TestValidator_REQ171_SharedBetweenGoroutines(t *testing.T) {
	v := validation.New(validation.WithLimits(validation.Limits{Depth: 64, Nodes: 50_000}))
	docs := corpusDocuments(t)
	type want struct{ floor, templated validation.Result }
	wants := make([]want, len(docs))
	for i, doc := range docs {
		wants[i].floor = v.Validate(doc.root, nil)
		if doc.c != nil {
			wants[i].templated = v.Validate(doc.root, doc.c)
		}
	}
	t.Run("parallel", func(t *testing.T) {
		for round := range 4 {
			for i, doc := range docs {
				t.Run(fmt.Sprintf("%d/%s", round, doc.rel.Rel), func(t *testing.T) {
					t.Parallel()
					assertSameResult(t, "Validate("+doc.rel.Rel+", nil)", v.Validate(doc.root, nil), wants[i].floor)
					if doc.c != nil {
						assertSameResult(t, "Validate("+doc.rel.Rel+", c)", v.Validate(doc.root, doc.c), wants[i].templated)
					}
				})
			}
		}
	})
}

// REQ-112 § Surface, REQ-171 § Surface — a nil guard is a global issue, so
// its path is empty on the floor's entries as on the template-driven ones;
// rm_type_unknown stays at "/", the root the floor could not descend. The
// package-level Validate still reports a nil template as nil_template, where
// a Validator applies the RM rules alone, and a Validator keeps the guard for
// a compiled template with no root.
func TestREQ112_REQ171_GuardPaths(t *testing.T) {
	t.Parallel()
	c := mustCompile(t, "vital_signs")
	comp := validVitalSignsComposition()
	tests := []struct {
		name string
		r    validation.Result
		code string
		path string
	}{
		{name: "ValidateRM(nil)", r: validation.ValidateRM(nil), code: "nil_root", path: ""},
		{name: "ValidateRM(typed-nil *rm.Folder)", r: validation.ValidateRM((*rm.Folder)(nil)), code: "nil_root", path: ""},
		{name: "New().Validate(nil, nil)", r: validation.New().Validate(nil, nil), code: "nil_root", path: ""},
		{name: "ValidateRMFolder(nil)", r: validation.ValidateRMFolder(nil), code: "nil_folder", path: ""},
		{name: "ValidateRMEHRStatus(nil)", r: validation.ValidateRMEHRStatus(nil), code: "nil_ehr_status", path: ""},
		{name: "ValidateRMEHRAccess(nil)", r: validation.ValidateRMEHRAccess(nil), code: "nil_ehr_access", path: ""},
		{name: "ValidateRMDemographic(nil)", r: validation.ValidateRMDemographic(nil), code: "nil_party", path: ""},
		{name: "ValidateRMDemographic(typed-nil *rm.Person)", r: validation.ValidateRMDemographic((*rm.Person)(nil)), code: "nil_party", path: ""},
		{name: "ValidateRM(root outside the RM set)", r: validation.ValidateRM(struct{}{}), code: "rm_type_unknown", path: "/"},
		{name: "New().Validate(root outside the RM set, nil)", r: validation.New().Validate(struct{}{}, nil), code: "rm_type_unknown", path: "/"},
		{name: "Validate(root, nil)", r: validation.Validate(comp, nil), code: "nil_template", path: ""},
		{name: "Validate(nil, c)", r: validation.Validate(nil, c), code: "nil_root", path: ""},
		{name: "New().Validate(nil, c)", r: validation.New().Validate(nil, c), code: "nil_root", path: ""},
		{name: "New().Validate(root, template without a root)", r: validation.New().Validate(comp, &templatecompile.Compiled{}), code: "nil_template", path: ""},
		{name: "Validate(root, template without a root)", r: validation.Validate(comp, &templatecompile.Compiled{}), code: "nil_template", path: ""},
	}
	for _, tc := range tests {
		if len(tc.r.Issues) != 1 || tc.r.Issues[0].Code != tc.code || tc.r.Issues[0].Path != tc.path || tc.r.OK {
			t.Errorf("%s = %s, want only %s at %q, not OK", tc.name, coverageText(tc.r), tc.code, tc.path)
		}
	}
	if r := validation.New().Validate(comp, nil); containsCode(r.Issues, "nil_template") || !reflect.DeepEqual(r, validation.ValidateRM(comp)) {
		t.Errorf("New().Validate(root, nil) = %s, want what ValidateRM returns, with no nil_template: a Validator applies the RM rules alone", coverageText(r))
	}
}
