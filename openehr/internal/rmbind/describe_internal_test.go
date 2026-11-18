package rmbind

import (
	"fmt"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
)

// TestDescribeNamesGoTypeAsFormatted holds the names describe writes itself to
// the ones %T gives, and to no allocation.
func TestDescribeNamesGoTypeAsFormatted(t *testing.T) {
	values := []any{"text", true, rm.Integer(3), rm.Integer64(3), rm.Real(1.5), int8(1), struct{}{}}
	for _, v := range values {
		want := fmt.Sprintf("%T", v)
		if got := describe(v); got != want {
			t.Errorf("describe(%T) = %q, want %q", v, got, want)
		}
	}
	for _, v := range values[:5] {
		if n := testing.AllocsPerRun(100, func() { _ = describe(v) }); n != 0 {
			t.Errorf("describe(%T) allocates %v times, want 0", v, n)
		}
	}
}
