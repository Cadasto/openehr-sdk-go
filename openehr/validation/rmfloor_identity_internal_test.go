package validation

// rmfloor_identity_internal_test.go: REQ-112 § Driver — the floor's visit
// record covers every pointer to an RM value. hasIdentity lists them by hand
// beside the generated rm.IsTypedNil; this test reads both switches and
// fails when a BMM bump adds a type to one and not the other.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// REQ-112 — hasIdentity accepts exactly the pointer types rm.IsTypedNil
// lists, and no value form.
func TestHasIdentity_REQ112_MatchesRMPointerTypes(t *testing.T) {
	want := typeSwitchCases(t, filepath.Join("..", "rm", "typereg_gen.go"), "IsTypedNil", "")
	got := typeSwitchCases(t, "rmfloor_identity.go", "hasIdentity", "rm.")
	if len(want) == 0 {
		t.Fatal("rm.IsTypedNil lists no case; the parse found the wrong function")
	}
	if missing := subtract(want, got); len(missing) > 0 {
		t.Errorf("hasIdentity lacks pointer types rm.IsTypedNil lists: %q", missing)
	}
	if extra := subtract(got, want); len(extra) > 0 {
		t.Errorf("hasIdentity lists types rm.IsTypedNil does not: %q", extra)
	}

	// Spot-check the behaviour behind the lists: the pointer has an identity,
	// the value it points at does not.
	for _, tc := range []struct {
		v    any
		want bool
	}{
		{v: &rm.Section{}, want: true},
		{v: rm.Section{}, want: false},
		{v: &rm.DVInterval[rm.DVQuantity]{}, want: true},
		{v: rm.DVInterval[rm.DVQuantity]{}, want: false},
		{v: &rm.PartyRef{}, want: true},
		{v: new("text"), want: false},
	} {
		if got := hasIdentity(tc.v); got != tc.want {
			t.Errorf("hasIdentity(%T) = %t, want %t", tc.v, got, tc.want)
		}
	}
}

// typeSwitchCases returns, sorted, the case types of the type switch in the
// function name of the Go file at path, each written as source with the
// package qualifier qualifier removed.
func typeSwitchCases(t *testing.T, path, name, qualifier string) []string {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	var cases []string
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv != nil || fn.Name.Name != name {
			continue
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			if cc, ok := n.(*ast.CaseClause); ok {
				for _, e := range cc.List {
					c := types.ExprString(e)
					if qualifier != "" {
						c = strings.ReplaceAll(c, qualifier, "")
					}
					cases = append(cases, c)
				}
			}
			return true
		})
	}
	if cases == nil {
		t.Fatalf("%s: no case in a function %s", path, name)
	}
	slices.Sort(cases)
	return cases
}

// subtract returns the members of a that b lacks; both are sorted.
func subtract(a, b []string) []string {
	var out []string
	for _, s := range a {
		if _, found := slices.BinarySearch(b, s); !found {
			out = append(out, s)
		}
	}
	return out
}
