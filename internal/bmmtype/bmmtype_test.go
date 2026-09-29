package bmmtype_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/bmmtype"
)

// REQ-100 — the compiler reads OPT-declared generic names through this
// helper. Split reads the class and the top-level actual parameters, keeps a
// nested parameter whole, and refuses every malformed spelling.
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
		{name: "Unicode space around the parts", in: "\u00a0DV_INTERVAL<\u0085DV_COUNT\v>\u00a0", class: "DV_INTERVAL", params: []string{"DV_COUNT"}, ok: true},
		{name: "white space before the opening bracket", in: "A <B>", class: "A", params: []string{"B"}, ok: true},

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
		{name: "vertical tab as the only parameter", in: "A<\v>"},
		{name: "no-break space as the only parameter", in: "A<\u00a0>"},
		{name: "next-line character as the only parameter", in: "A<\u0085>"},
		{name: "blank second parameter", in: "Hash<String,\u00a0>"},
		{name: "white space inside the class name", in: "DV INTERVAL"},
		{name: "white space inside a generic class name", in: "DV INTERVAL<DV_COUNT>"},
		{name: "white space inside a parameter", in: "DV_INTERVAL<DV QUANTITY>"},
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
		{in: "DV_TEXT", want: "DV_TEXT"},
		{in: "DV_INTERVAL<DV_QUANTITY>", want: "DV_INTERVAL"},
		{in: "A<B<C>>", want: "A"},
		{in: "Hash<String, String>", want: "Hash"},
		{in: " DV_INTERVAL <DV_COUNT> ", want: "DV_INTERVAL"},
		{in: "", want: ""},
		{in: "DV_INTERVAL<DV_QUANTITY", want: "DV_INTERVAL<DV_QUANTITY"},
		{in: "DV_INTERVAL<>", want: "DV_INTERVAL<>"},
		{in: "DV_INTERVAL<DV_QUANTITY>junk", want: "DV_INTERVAL<DV_QUANTITY>junk"},
		{in: "DV INTERVAL", want: "DV INTERVAL"},
		{in: "A<\v>", want: "A<\v>"},
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
		{name: "interval bound of a quantity interval", owner: "DV_INTERVAL<DV_QUANTITY>", declared: "T", want: "DV_QUANTITY"},
		{name: "interval bound of a count interval", owner: "DV_INTERVAL<DV_COUNT>", declared: "T", want: "DV_COUNT"},
		{name: "event data", owner: "POINT_EVENT<ITEM_TREE>", declared: "T", want: "ITEM_TREE"},
		{name: "version data", owner: "ORIGINAL_VERSION<COMPOSITION>", declared: "T", want: "COMPOSITION"},
		// Syntax, not conformance: the actual parameter is returned whole.
		{name: "nested actual parameter", owner: "DV_INTERVAL<DV_INTERVAL<DV_COUNT>>", declared: "T", want: "DV_INTERVAL<DV_COUNT>"},
		{name: "declared type is not a formal parameter", owner: "DV_INTERVAL<DV_QUANTITY>", declared: "Boolean", want: "Boolean"},
		{name: "owner without parameters", owner: "DV_INTERVAL", declared: "T", want: "T"},
		{name: "too many actual parameters", owner: "DV_INTERVAL<DV_QUANTITY, DV_COUNT>", declared: "T", want: "T"},
		{name: "class that is not generic", owner: "DV_TEXT<DV_QUANTITY>", declared: "T", want: "T"},
		{name: "malformed owner", owner: "DV_INTERVAL<DV_QUANTITY", declared: "T", want: "T"},
		{name: "empty declared type", owner: "DV_INTERVAL<DV_QUANTITY>", declared: "", want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := bmmtype.Substitute(tc.owner, tc.declared); got != tc.want {
				t.Errorf("Substitute(%q, %q) = %q, want %q", tc.owner, tc.declared, got, tc.want)
			}
		})
	}
}
