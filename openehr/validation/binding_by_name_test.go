package validation_test

// binding_by_name_test.go: REQ-102 (Structural — binding items), REQ-116,
// REQ-168 — where several OPT children of a multi-valued attribute carry
// one archetype id, as when a template reuses an archetype under distinct
// template-level names, an item binds to the child whose pinned name equals
// its name, else to the first child that pins no name, else to the first
// child. The item's name decides the binding only: it reaches no path and
// no message.

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/validation"
)

const (
	reusedID = "openEHR-EHR-OBSERVATION.reused.v1"
	// obrienName is the second name reusedArchetypeOPT pins.
	obrienName = `O'Brien\Notes`
	erstePath  = "/content[" + reusedID + ", 'Erste']"
	obrienPath = "/content[" + reusedID + `, 'O\'Brien\\Notes']`
)

// withoutLanguage returns comp with the RM-mandatory language of its content
// item at index i left empty, a defect the RM rules report at that item's
// path.
func withoutLanguage(t *testing.T, comp *rm.Composition, i int) *rm.Composition {
	t.Helper()
	obs, ok := comp.Content[i].(*rm.Observation)
	if !ok {
		t.Fatalf("content[%d] is %T, want *rm.Observation", i, comp.Content[i])
	}
	obs.Language = rm.CodePhrase{}
	return comp
}

// withNames returns comp with the name of each of its content items replaced
// by the one name makes of the text at the same index.
func withNames(t *testing.T, comp *rm.Composition, name func(string) rm.DVTextLike, texts ...string) *rm.Composition {
	t.Helper()
	for i, text := range texts {
		obs, ok := comp.Content[i].(*rm.Observation)
		if !ok {
			t.Fatalf("content[%d] is %T, want *rm.Observation", i, comp.Content[i])
		}
		obs.Name = name(text)
	}
	return comp
}

// codedName returns a DV_CODED_TEXT whose value is text and whose code is a
// well-formed local one.
func codedName(text string) rm.DVCodedText {
	return rm.DVCodedText{
		Value:        text,
		DefiningCode: rm.CodePhrase{TerminologyID: rm.TerminologyID{Value: "local"}, CodeString: "at0001"},
	}
}

// emptyLanguage returns, sorted as issueKeys sorts them, the issues an empty
// language on the item at itemPath gives: `required`, which the template
// walk and the RM floor both report and the result carries once, and the
// floor's `code_not_in_value_set`, since an empty code is in no code set.
func emptyLanguage(itemPath string) []string {
	return []string{"code_not_in_value_set " + itemPath + "/language", "required " + itemPath + "/language"}
}

// REQ-102, REQ-116 — items whose names are the names the template pins bind
// to the children that pin them, whatever their order, so a correct
// composition passes. The name is read from a DV_TEXT or a DV_CODED_TEXT, by
// value or by pointer, as the walk reads the name it checks.
func TestValidateComposition_REQ102_REQ116_SharedIDBindsByPinnedName(t *testing.T) {
	c := mustCompileSyntheticOPT(t, reusedArchetypeOPT)
	forms := []struct {
		name string
		make func(string) rm.DVTextLike
	}{
		{name: "DV_TEXT", make: func(s string) rm.DVTextLike { return rm.DVText{Value: s} }},
		{name: "*DV_TEXT", make: func(s string) rm.DVTextLike { return &rm.DVText{Value: s} }},
		{name: "DV_CODED_TEXT", make: func(s string) rm.DVTextLike { return codedName(s) }},
		{name: "*DV_CODED_TEXT", make: func(s string) rm.DVTextLike { n := codedName(s); return &n }},
	}
	for _, form := range forms {
		t.Run(form.name, func(t *testing.T) {
			for _, names := range [][]string{{"Erste", obrienName}, {obrienName, "Erste"}} {
				comp := withNames(t, reusedComposition(names...), form.make, names...)
				r := validation.ValidateComposition(comp, c)
				if len(r.Issues) != 0 {
					t.Errorf("ValidateComposition(content named %q as %s) issues = %q, want none", names, form.name, issueKeys(r.Issues))
				}
			}
		})
	}
}

// REQ-102, REQ-116 — an item bound by its pinned name is written with that
// name, so a defect inside it is reported at the name-predicated path of
// the child it binds to, whichever position it holds.
func TestValidateComposition_REQ102_REQ116_BoundNameReachesItemPath(t *testing.T) {
	named := mustCompileSyntheticOPT(t, reusedArchetypeOPT)
	withUnnamed := mustCompileSyntheticOPT(t, reusedArchetypeOPTOf("Erste", obrienName, ""))
	tests := []struct {
		name  string
		opt   string
		names []string
		index int
		want  []string
	}{
		{
			name: "named item second", names: []string{"Erste", obrienName}, index: 1,
			want: emptyLanguage(obrienPath),
		},
		{
			name: "named item first", names: []string{obrienName, "Erste"}, index: 0,
			want: emptyLanguage(obrienPath),
		},
		{
			// The OPT requires each of its three children once, so the
			// missing one is reported too.
			name: "named item beside one bound to the child that pins no name", opt: "unnamed",
			names: []string{"Andere", "Erste"}, index: 1,
			want: append([]string{"cardinality " + obrienPath}, emptyLanguage(erstePath)...),
		},
		{
			// The item bound to the child that pins no name has no name to
			// be written with, so it is written by its position.
			name: "item bound to the child that pins no name", opt: "unnamed",
			names: []string{"Erste", obrienName, "Andere"}, index: 2,
			want: emptyLanguage("/content[3]"),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := named
			if tc.opt == "unnamed" {
				c = withUnnamed
			}
			comp := withoutLanguage(t, reusedComposition(tc.names...), tc.index)
			r := validation.ValidateComposition(comp, c)
			if got := issueKeys(r.Issues); !slices.Equal(got, tc.want) {
				t.Errorf("ValidateComposition issues = %q, want %q", got, tc.want)
			}
		})
	}
}

// REQ-102 — two items that both carry one pinned name bind to the child that
// pins it, which then occurs too often while its sibling occurs too seldom;
// the items share their bound name, so they are written by position.
func TestValidateComposition_REQ102_SharedPinnedNameBindsBothToOneChild(t *testing.T) {
	c := mustCompileSyntheticOPT(t, reusedArchetypeOPT)
	r := validation.ValidateComposition(reusedComposition("Erste", "Erste"), c)
	want := []string{"cardinality " + erstePath, "cardinality " + obrienPath}
	if got := issueKeys(r.Issues); !slices.Equal(got, want) {
		t.Errorf("ValidateComposition issues = %q, want %q", got, want)
	}
}

// REQ-102 — an item whose name no child pins binds to the first child that
// pins no name, whatever the order of the items, so the composition passes.
func TestValidateComposition_REQ102_UnmatchedNameBindsToUnnamedChild(t *testing.T) {
	c := mustCompileSyntheticOPT(t, reusedArchetypeOPTOf("Erste", obrienName, ""))
	for _, names := range [][]string{{"Erste", obrienName, "Andere"}, {"Andere", "Erste", obrienName}} {
		r := validation.ValidateComposition(reusedComposition(names...), c)
		if len(r.Issues) != 0 {
			t.Errorf("ValidateComposition(content named %q) issues = %q, want none", names, issueKeys(r.Issues))
		}
	}
}

// REQ-102 — of two children that carry the id and pin no name, an item whose
// name no child pins binds to the first. The first may occur once and the
// second must, so the second is reported as occurring too seldom; had the item
// bound to the second, neither would be.
func TestValidateComposition_REQ102_UnmatchedNameBindsToFirstUnnamedChild(t *testing.T) {
	c := mustCompileSyntheticOPT(t, reusedArchetypeOPTWith(reusedObservationChild("", 0), reusedObservationChild("", 1)))
	r := validation.ValidateComposition(reusedComposition("Andere"), c)
	want := []string{"cardinality /content[" + reusedID + "]"}
	if got := issueKeys(r.Issues); !slices.Equal(got, want) {
		t.Fatalf("ValidateComposition issues = %q, want %q", got, want)
	}
	if detail := r.Issues[0].Detail; !strings.Contains(detail, "appears 0 times") || !strings.Contains(detail, "[1..1]") {
		t.Errorf("cardinality detail = %q, want the second child, occurring 0 times against [1..1]", detail)
	}
}

// REQ-102 — when every child that carries the id pins a name and none is the
// item's, the item binds to the first of them, whose name constraint then
// reports the mismatch.
func TestValidateComposition_REQ102_UnmatchedNameBindsToFirstNamedChild(t *testing.T) {
	c := mustCompileSyntheticOPT(t, reusedArchetypeOPT)
	r := validation.ValidateComposition(reusedComposition("Dritte"), c)
	want := []string{"cardinality " + obrienPath, "primitive_not_in_list /content[" + reusedID + "]/name/value"}
	if got := issueKeys(r.Issues); !slices.Equal(got, want) {
		t.Errorf("ValidateComposition issues = %q, want %q", got, want)
	}
}

// REQ-107, REQ-102, REQ-116 — the generator's output for a template that
// reuses one archetype under several names, or beside a reuse that pins no
// name, validates against that template with no issue, under both policies.
// The corpus cannot show this: no corpus composition has two items with one
// archetype_node_id.
func TestGenerate_REQ107_REQ116_ReusedArchetypeOutputValidates(t *testing.T) {
	for _, names := range [][]string{{"Erste", "Zweite"}, {"Erste", "Zweite", ""}, {"", "Erste"}} {
		c := mustCompileSyntheticOPT(t, reusedArchetypeOPTOf(names...))
		for _, policy := range []instance.Policy{instance.Minimal, instance.Example} {
			t.Run(fmt.Sprintf("%q/%s", names, policy), func(t *testing.T) {
				out, err := instance.Generate(t.Context(), c, instance.Options{
					Policy:    policy,
					Territory: "NL",
					Composer:  &rm.PartyIdentified{Name: new("Dr Jones")},
					Now:       time.Date(2026, 10, 11, 9, 0, 0, 0, time.UTC),
				})
				if err != nil {
					t.Fatalf("Generate: %v", err)
				}
				comp, ok := out.(*rm.Composition)
				if !ok {
					t.Fatalf("Generate root = %T, want *rm.Composition", out)
				}
				if len(comp.Content) != len(names) {
					t.Fatalf("Generate built %d content items, want %d", len(comp.Content), len(names))
				}
				if got := issueKeys(validation.ValidateComposition(comp, c).Issues); len(got) != 0 {
					t.Errorf("ValidateComposition(generated) issues = %q, want none", got)
				}
			})
		}
	}
}

// REQ-102, REQ-168 — the name an item carries decides its binding only: a
// distinctive name appears in no path and no message, whether it matches no
// pinned name or sits beside items bound by theirs.
func TestValidateComposition_REQ102_REQ168_ItemNameStaysOutOfIssues(t *testing.T) {
	const planted = "PLANTED-ITEM-NAME"
	named := mustCompileSyntheticOPT(t, reusedArchetypeOPT)
	withUnnamed := mustCompileSyntheticOPT(t, reusedArchetypeOPTOf("Erste", obrienName, ""))
	results := map[string]validation.Result{
		"no pinned name matches": validation.ValidateComposition(reusedComposition(planted), named),
		"bound to the unnamed child, with a defect": validation.ValidateComposition(
			withoutLanguage(t, reusedComposition("Erste", planted, obrienName), 1), withUnnamed),
		"beside items bound by name, with defects": validation.ValidateComposition(
			withoutLanguage(t, withoutLanguage(t, reusedComposition(planted, "Erste", obrienName), 1), 2), withUnnamed),
	}
	for name, r := range results {
		if len(r.Issues) == 0 {
			t.Fatalf("%s: no issues, so nothing to inspect", name)
		}
		for _, iss := range r.Issues {
			if strings.Contains(iss.Path, planted) || strings.Contains(iss.Detail, planted) {
				t.Errorf("%s: issue %s at %s (%s) carries the item's name", name, iss.Code, iss.Path, iss.Detail)
			}
		}
	}
}
