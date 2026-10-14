package validation

// engine_internal_test.go: REQ-171 § One walk (Admission) — the path rule
// that tells which findings lie inside content the template refused.

import "testing"

// REQ-171 § One walk — a path is at or below another only where it is that
// path, or continues it with an attribute or a predicate: a path that shares
// a prefix with it and goes on inside one segment is a sibling, never
// inside.
func TestIsAtOrBelow_REQ171_SegmentBoundary(t *testing.T) {
	t.Parallel()
	tests := []struct {
		path, under string
		want        bool
	}{
		{path: "/items[1]", under: "/items[1]", want: true},
		{path: "/items[1]/value", under: "/items[1]", want: true},
		{path: "/items[10]", under: "/items[1]"},
		{path: "/items[10]/value", under: "/items[1]"},
		{path: "/x[2]/y", under: "/x", want: true},
		{path: "/x", under: "/x[2]/y"},
		{path: "/integrity_check", under: "/integrity_check", want: true},
		{path: "/integrity_check_algorithm", under: "/integrity_check"},
		{path: "/integrity_check_algorithm/code_string", under: "/integrity_check"},
		{path: "/null_flavour/defining_code", under: "/null_flavour", want: true},
	}
	for _, tc := range tests {
		if got := isAtOrBelow(tc.path, tc.under); got != tc.want {
			t.Errorf("isAtOrBelow(%q, %q) = %t, want %t", tc.path, tc.under, got, tc.want)
		}
	}
}
