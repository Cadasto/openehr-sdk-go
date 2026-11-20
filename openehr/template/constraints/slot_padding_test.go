package constraints_test

import (
	"runtime"
	"strings"
	"testing"
	"unsafe"
	"weak"

	"github.com/cadasto/openehr-sdk-go/openehr/template/constraints"
)

// TestNewSlotAssertionKeepsNoPadding pins that an assertion keeps none of the
// white space around the pattern it is given: it keeps a copy of the trimmed
// pattern, not a slice of the caller's string, which would keep the whole
// padded string alive for as long as the assertion is kept.
func TestNewSlotAssertionKeepsNoPadding(t *testing.T) {
	const want = `openEHR-EHR-CLUSTER\.device\.v1`
	pad := strings.Repeat(" ", 1024)
	// The padded pattern is reachable only inside this function, so once it
	// returns the assertion is the only thing that could keep it.
	a, source := func() (constraints.SlotAssertion, weak.Pointer[byte]) {
		pattern := pad + want + pad
		a, err := constraints.NewSlotAssertion(pattern)
		if err != nil {
			t.Fatalf("NewSlotAssertion(%q padded): %v", want, err)
		}
		return a, weak.Make(unsafe.StringData(pattern))
	}()
	runtime.GC()
	if source.Value() != nil {
		t.Errorf("NewSlotAssertion(%q padded with %d spaces on each side) keeps the padded pattern alive: want a copy of the trimmed pattern", want, len(pad))
	}
	if got := a.Pattern(); got != want {
		t.Errorf("NewSlotAssertion(%q padded).Pattern() = %q; want %q", want, got, want)
	}
	runtime.KeepAlive(a)
}
