package parse

// span_internal_test.go: the spanAt guards no external test can reach —
// REQ-113 § Value-free structured drop records (PROBE-096 arm (c)'s helper).
// Both cases are LATENT: no current drop site produces them, and these tests
// exist so a future call site that does fails loudly here instead of
// panicking in a diagnostics helper (PR #112 review).

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aql/parse/gen"
)

// TestSpanAtTypedNilIsZero pins that an absent node yields the zero span
// whether the nil arrives untyped or typed — a typed nil satisfies the
// interface cases and would otherwise panic inside the accessor.
func TestSpanAtTypedNilIsZero(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		node any
	}{
		{"untyped nil", nil},
		{"typed-nil context", (*gen.NodePredicateContext)(nil)},
		{"typed-nil terminal", (*struct{ gen.NodePredicateContext })(nil)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := spanAt(tc.node); !got.IsZero() {
				t.Errorf("spanAt(%s) = %+v; want the zero span", tc.name, got)
			}
		})
	}
}

// TestBoundedSpanNeverInverts pins the [Start, End) contract when the two
// endpoints are computed independently: a missing endpoint collapses to zero
// width at the known one instead of producing an inverted range.
func TestBoundedSpanNeverInverts(t *testing.T) {
	t.Parallel()
	at := Position{Line: 2, Col: 7}
	cases := []struct {
		name       string
		start, end Position
		want       Span
	}{
		{"both known", at, Position{Line: 2, Col: 9}, Span{Start: at, End: Position{Line: 2, Col: 9}}},
		{"missing end", at, Position{}, Span{Start: at, End: at}},
		{"missing start", Position{}, at, Span{Start: at, End: at}},
		{"both missing", Position{}, Position{}, Span{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := boundedSpan(tc.start, tc.end); got != tc.want {
				t.Errorf("boundedSpan(%+v, %+v) = %+v; want %+v", tc.start, tc.end, got, tc.want)
			}
		})
	}
}
