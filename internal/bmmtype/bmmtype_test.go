package bmmtype_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/bmmtype"
)

// Split reads the class and the top-level actual parameters, keeps a nested
// parameter whole, and refuses every malformed spelling.
func TestSplit(t *testing.T) {
	cases := []struct {
		name   string
		in     string
		class  string
		params []string
		ok     bool
	}{
		{name: "bare class", in: "DV_TEXT", class: "DV_TEXT", ok: true},
		{name: "one parameter", in: "DV_INTERVAL<DV_QUANTITY>", class: "DV_INTERVAL", params: []string{"DV_QUANTITY"}, ok: true},
		{name: "two parameters", in: "Hash<String, String>", class: "Hash", params: []string{"String", "String"}, ok: true},
		{name: "nested parameter stays whole", in: "A<B<C>>", class: "A", params: []string{"B<C>"}, ok: true},
		{
			name: "nested and sibling parameters", in: "FUNCTION<TUPLE2<Integer, Real>, Boolean>",
			class: "FUNCTION", params: []string{"TUPLE2<Integer, Real>", "Boolean"}, ok: true,
		},
		{name: "white space around the parts", in: "  DV_INTERVAL < DV_COUNT >  ", class: "DV_INTERVAL", params: []string{"DV_COUNT"}, ok: true},

		{name: "empty", in: ""},
		{name: "blank", in: "   "},
		{name: "missing class", in: "<DV_QUANTITY>"},
		{name: "unclosed", in: "DV_INTERVAL<DV_QUANTITY"},
		{name: "empty parameter list", in: "DV_INTERVAL<>"},
		{name: "blank parameter list", in: "DV_INTERVAL< >"},
		{name: "empty trailing parameter", in: "Hash<String,>"},
		{name: "empty leading parameter", in: "Hash<,String>"},
		{name: "extra closing bracket", in: "DV_INTERVAL<DV_QUANTITY>>"},
		{name: "unclosed nested parameter", in: "A<B<C>"},
		{name: "text after the closing bracket", in: "DV_INTERVAL<DV_QUANTITY>junk"},
		{name: "second parameter list", in: "A<B><C>"},
		{name: "closing bracket without an opening one", in: "DV_INTERVAL>"},
		{name: "top-level comma", in: "DV_TEXT,DV_CODED_TEXT"},
		{name: "text after a nested closing bracket", in: "A<B<C> D>"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			class, params, ok := bmmtype.Split(tc.in)
			if class != tc.class || !slices.Equal(params, tc.params) || ok != tc.ok {
				t.Errorf("Split(%q) = (%q, %q, %v), want (%q, %q, %v)", tc.in, class, params, ok, tc.class, tc.params, tc.ok)
			}
		})
	}
}

// A deeply nested name splits in one pass: the class, and the whole nested
// remainder as its single parameter.
func TestSplit_deepNesting(t *testing.T) {
	const depth = 10_000
	name := strings.Repeat("A<", depth) + "B" + strings.Repeat(">", depth)
	class, params, ok := bmmtype.Split(name)
	wantParam := strings.Repeat("A<", depth-1) + "B" + strings.Repeat(">", depth-1)
	if class != "A" || len(params) != 1 || params[0] != wantParam || !ok {
		t.Errorf("Split(%d-deep name) = (%q, %d params, %v), want (\"A\", the %d-deep remainder, true)", depth, class, len(params), ok, depth-1)
	}
}

// Class strips the parameters of a well-formed name and returns a malformed
// one unchanged, so a class-keyed lookup misses it instead of guessing.
func TestClass(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"DV_TEXT", "DV_TEXT"},
		{"DV_INTERVAL<DV_QUANTITY>", "DV_INTERVAL"},
		{"A<B<C>>", "A"},
		{"Hash<String, String>", "Hash"},
		{" DV_INTERVAL <DV_COUNT> ", "DV_INTERVAL"},
		{"", ""},
		{"DV_INTERVAL<DV_QUANTITY", "DV_INTERVAL<DV_QUANTITY"},
		{"DV_INTERVAL<>", "DV_INTERVAL<>"},
		{"DV_INTERVAL<DV_QUANTITY>junk", "DV_INTERVAL<DV_QUANTITY>junk"},
	}
	for _, tc := range cases {
		if got := bmmtype.Class(tc.in); got != tc.want {
			t.Errorf("Class(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// Substitute replaces a formal parameter with the actual one the owner name
// supplies, and leaves every other declared type alone.
func TestSubstitute(t *testing.T) {
	cases := []struct {
		name, owner, declared, want string
	}{
		{"interval bound of a quantity interval", "DV_INTERVAL<DV_QUANTITY>", "T", "DV_QUANTITY"},
		{"interval bound of a count interval", "DV_INTERVAL<DV_COUNT>", "T", "DV_COUNT"},
		{"event data", "POINT_EVENT<ITEM_TREE>", "T", "ITEM_TREE"},
		{"version data", "ORIGINAL_VERSION<COMPOSITION>", "T", "COMPOSITION"},
		// Syntax, not conformance: the actual parameter is returned whole.
		{"nested actual parameter", "DV_INTERVAL<DV_INTERVAL<DV_COUNT>>", "T", "DV_INTERVAL<DV_COUNT>"},
		{"declared type is not a formal parameter", "DV_INTERVAL<DV_QUANTITY>", "Boolean", "Boolean"},
		{"owner without parameters", "DV_INTERVAL", "T", "T"},
		{"too many actual parameters", "DV_INTERVAL<DV_QUANTITY, DV_COUNT>", "T", "T"},
		{"class that is not generic", "DV_TEXT<DV_QUANTITY>", "T", "T"},
		{"malformed owner", "DV_INTERVAL<DV_QUANTITY", "T", "T"},
		{"empty declared type", "DV_INTERVAL<DV_QUANTITY>", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := bmmtype.Substitute(tc.owner, tc.declared); got != tc.want {
				t.Errorf("Substitute(%q, %q) = %q, want %q", tc.owner, tc.declared, got, tc.want)
			}
		})
	}
}
