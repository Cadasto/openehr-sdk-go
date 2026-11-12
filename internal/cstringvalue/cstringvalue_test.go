package cstringvalue_test

import (
	"errors"
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/cstringvalue"
	"github.com/cadasto/openehr-sdk-go/internal/importguard"
	"github.com/cadasto/openehr-sdk-go/openehr/template/constraints"
)

// TestREQ107ActionArchetypeIDFollowsTheConstraint pins the value the
// generator writes on an ACTIVITY's action_archetype_id (REQ-107), which the FLAT
// decode's template completion (REQ-053) takes from the same function: the
// RM default where the C_STRING lists no string and admits it, else the
// example the constraint accepts, a list member, or the shortest match of a
// pattern.
func TestREQ107ActionArchetypeIDFollowsTheConstraint(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		cs   constraints.CString
		want string
	}{
		{
			name: "neither a pattern nor a list",
			cs:   constraints.NewCString("", nil, ""),
			want: cstringvalue.AnyActionArchetype,
		},
		{
			name: "a pattern",
			cs:   constraints.NewCString(`openEHR-EHR-ACTION\.minimal\.v1`, nil, ""),
			want: "openEHR-EHR-ACTION.minimal.v1",
		},
		{
			// The example matches too, but names no archetype.
			name: "a pattern that admits the RM default",
			cs:   constraints.NewCString(`.*`, nil, ""),
			want: cstringvalue.AnyActionArchetype,
		},
		{
			// A list gives values, so the RM default, though listed, does
			// not come before Pick's member.
			name: "a pattern and a list that both admit the RM default",
			cs:   constraints.NewCString(`.*`, []string{"openEHR-EHR-ACTION.one.v1", cstringvalue.AnyActionArchetype}, ""),
			want: "openEHR-EHR-ACTION.one.v1",
		},
		{
			name: "a pattern that admits the RM default, and a list that does not",
			cs:   constraints.NewCString(`.*`, []string{"openEHR-EHR-ACTION.one.v1"}, ""),
			want: "openEHR-EHR-ACTION.one.v1",
		},
		{
			name: "a list",
			cs:   constraints.NewCString("", []string{"openEHR-EHR-ACTION.one.v1", "openEHR-EHR-ACTION.two.v1"}, ""),
			want: "openEHR-EHR-ACTION.one.v1",
		},
		{
			name: "a list and a pattern",
			cs:   constraints.NewCString(`.*\.two\.v1`, []string{"openEHR-EHR-ACTION.one.v1", "openEHR-EHR-ACTION.two.v1"}, ""),
			want: "openEHR-EHR-ACTION.two.v1",
		},
	}
	for _, tc := range cases {
		got, err := cstringvalue.ActionArchetypeID(tc.cs, tc.cs.ExampleValue())
		if err != nil || got != tc.want {
			t.Errorf("%s: ActionArchetypeID(%+v) = %q, %v; want %q, nil", tc.name, tc.cs, got, err, tc.want)
		}
	}
	none := constraints.NewCString(`[0-9]+`, []string{"abc"}, "")
	if got, err := cstringvalue.ActionArchetypeID(none, none.ExampleValue()); !errors.Is(err, cstringvalue.ErrNoValue) {
		t.Errorf("ActionArchetypeID(no member the pattern accepts) = %q, %v; want ErrNoValue", got, err)
	}
}

// TestREQ107PickOrder pins the order of choice of Pick: the example when the
// constraint accepts it, then the first list member it accepts, then, with no
// list, the shortest string the pattern builds, a non-empty one where that is
// empty; and an example that is not a string is an error.
func TestREQ107PickOrder(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		cs      constraints.CString
		example string
		want    string
	}{
		{name: "open string keeps the example", cs: constraints.NewCString("", nil, ""), example: "example", want: "example"},
		{name: "first member the pattern accepts", cs: constraints.NewCString("[0-9]+", []string{"abc", "123"}, ""), example: "abc", want: "123"},
		{name: "shortest match of a pattern", cs: constraints.NewCString(`[A-Z]{3}-\d{2}`, nil, ""), example: "example", want: "AAA-00"},
	}
	for _, tc := range cases {
		got, err := cstringvalue.Pick(tc.cs, tc.example)
		if err != nil || got != tc.want {
			t.Errorf("%s: Pick(%+v, %q) = %q, %v; want %q, nil", tc.name, tc.cs, tc.example, got, err, tc.want)
		}
	}
	if got, err := cstringvalue.Pick(constraints.NewCString("", nil, ""), 7); err == nil {
		t.Errorf("Pick(example 7) = %q, nil; want an error for an example that is not a string", got)
	}
}

// TestREQ107PickPrefersANonEmptyMatch pins the pick for a pattern-only
// C_STRING whose shortest match is the empty string, which an RM attribute
// such as DV_TEXT.value refuses: Pick builds one repetition of the first
// optional or starred part that yields a character, or the first
// alternative that yields one; where the pattern refuses that string, as a
// word boundary can, or refuses its shortest build, Pick takes the shortest
// string of a wider search the pattern accepts, and keeps the empty string
// only where the pattern accepts no other string it builds. The example and
// the list members keep their precedence.
func TestREQ107PickPrefersANonEmptyMatch(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		pattern string
		list    []string
		example string
		want    string
	}{
		{name: "an optional literal", pattern: `x?`, example: "example", want: "x"},
		{name: "a starred class", pattern: `[0-9]*`, example: "example", want: "0"},
		{name: "the first optional part that yields a character", pattern: `(ab)?c?`, example: "example", want: "ab"},
		{name: "the first alternative's shortest match", pattern: `a|b?`, example: "example", want: "a"},
		{name: "the first alternative that yields a character", pattern: `b?|a`, example: "example", want: "b"},
		{name: "an anchored optional literal", pattern: `^x?$`, example: "example", want: "x"},
		{name: "a pattern that admits only the empty string", pattern: `x{0}`, example: "example", want: ""},
		{name: "a non-empty build the constraint refuses", pattern: `x?\B`, example: "example", want: ""},
		{name: "a longer build where the first non-empty one is refused", pattern: `x?\By?`, example: "example", want: "xy"},
		{name: "a later alternative where the first one's build is refused", pattern: `(?:x?\B|ehr://a)?`, example: "example", want: "ehr://a"},
		{name: "another character where the shortest build is refused", pattern: `.\b.`, example: "example", want: "a-"},
		{name: "the example keeps its precedence", pattern: `[0-9]*`, example: "42", want: "42"},
		{name: "a list member keeps its precedence", pattern: `[0-9]*`, list: []string{"abc", "12"}, example: "example", want: "12"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cs := constraints.NewCString(tc.pattern, tc.list, "")
			got, err := cstringvalue.Pick(cs, tc.example)
			if err != nil || got != tc.want {
				t.Errorf("Pick(pattern %q, list %q, example %q) = %q, %v; want %q, nil", tc.pattern, tc.list, tc.example, got, err, tc.want)
			}
		})
	}
}

// TestREQ013CStringValueImports guards the package's place under both the
// generator and the FLAT codec: its own files import only the standard
// library and openehr/template/constraints, and nothing it pulls in reaches
// transport, auth or openehr/client (REQ-013).
func TestREQ013CStringValueImports(t *testing.T) {
	t.Parallel()
	const allowed = "github.com/cadasto/openehr-sdk-go/openehr/template/constraints"
	imports, err := importguard.Imports(".")
	if err != nil {
		t.Fatal(err)
	}
	if len(imports) == 0 {
		t.Fatal("importguard.Imports(.) found no import; the scan has gone blind")
	}
	for _, imp := range imports {
		if !importguard.Standard(imp) && imp != allowed {
			t.Errorf("internal/cstringvalue imports %q in its own files; it may import only the standard library and %s", imp, allowed)
		}
	}
	violations, err := importguard.Scan(".", importguard.WireLayers())
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range violations {
		t.Errorf("internal/cstringvalue pulls in %q: %s imports it (forbidden entry %q; REQ-013)", v.Import, v.Importer, v.Prefix)
	}
}
