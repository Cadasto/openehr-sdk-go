package importguard

import (
	"go/build/constraint"
	"testing"
)

// TestAnyBuild pins the build-tag rule that decides whether a file this
// machine leaves out still counts for a REQ-013 guard: every tag but "ignore"
// may be set or unset, whichever the expression needs at that point. Each case
// fails if one branch of the rule is dropped or inverted.
func TestAnyBuild(t *testing.T) {
	t.Parallel()
	tests := []struct {
		line string
		want bool
	}{
		{line: "//go:build ignore", want: false},
		{line: "//go:build !ignore", want: true},
		{line: "//go:build linux", want: true},
		{line: "//go:build !linux", want: true},
		{line: "//go:build ignore && linux", want: false},
		{line: "//go:build ignore || linux", want: true},
		{line: "//go:build !(linux || ignore)", want: true},
		{line: "//go:build !ignore && ignore", want: false},
	}
	for _, tc := range tests {
		expr, err := constraint.Parse(tc.line)
		if err != nil {
			t.Fatalf("constraint.Parse(%q) error: %v", tc.line, err)
		}
		if got := anyBuild(expr, true); got != tc.want {
			t.Errorf("anyBuild(%q) = %t, want %t", tc.line, got, tc.want)
		}
	}
}
