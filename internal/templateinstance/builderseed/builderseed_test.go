package builderseed_test

import (
	"context"
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/importguard"
	"github.com/cadasto/openehr-sdk-go/internal/templateinstance/builderseed"
)

// TestAskedReadsTheMarkWithSets checks that Asked reports the mark With
// sets, on the context With returns and on one derived from it, and no mark
// on a context that never had it.
func TestAskedReadsTheMarkWithSets(t *testing.T) {
	t.Parallel()
	type otherKey struct{}
	cases := []struct {
		name string
		ctx  func(context.Context) context.Context
		want bool
	}{
		{name: "plain context", ctx: func(ctx context.Context) context.Context { return ctx }, want: false},
		{name: "With", ctx: builderseed.With, want: true},
		{
			name: "derived from With",
			ctx: func(ctx context.Context) context.Context {
				return context.WithValue(builderseed.With(ctx), otherKey{}, "x")
			},
			want: true,
		},
		{
			name: "another key holding true",
			ctx:  func(ctx context.Context) context.Context { return context.WithValue(ctx, otherKey{}, true) },
			want: false,
		},
	}
	for _, tc := range cases {
		if got := builderseed.Asked(tc.ctx(t.Context())); got != tc.want {
			t.Errorf("Asked(%s) = %t, want %t", tc.name, got, tc.want)
		}
	}
}

// TestBuilderseedImportsOnlyTheStandardLibrary guards the edges this package
// sits under: openehr/composition and openehr/instance both import it, and
// composition imports instance, so an import of either, or of any package of
// this module that reaches them, could close a cycle. Every import of its own
// non-test files is therefore a standard-library package.
//
// importguard.TestStandard is the can-fail control for the standard-library
// check.
func TestBuilderseedImportsOnlyTheStandardLibrary(t *testing.T) {
	t.Parallel()
	imports, err := importguard.Imports(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, imp := range imports {
		if !importguard.Standard(imp) {
			t.Errorf("internal/templateinstance/builderseed imports %q; it imports only the standard library, so the composition builder and the instance generator can both import it without a cycle", imp)
		}
	}
}
