package simplified

// REQ-053 — Format completeness. A member added to the Format iota block
// without an arm in String and MediaType must fail a test rather than ship
// silently: the exhaustive linter is not enabled in this repo, and the
// exported-API tests loop over a hard-coded pair of formats, so neither would
// notice. formatSentinel is unexported, which is why this test sits inside the
// package rather than in simplified_test.

import (
	"fmt"
	"testing"
)

// TestFormatMembersAreComplete walks every format between the FormatUnknown
// zero value and the formatSentinel upper bound — both excluded — and asserts
// each one carries a canonical media type, a spelled-out String, and a media
// type that classifies back to it.
func TestFormatMembersAreComplete(t *testing.T) { // REQ-053
	t.Parallel()
	members := 0
	for f := FormatUnknown + 1; f < formatSentinel; f++ {
		members++
		t.Run(f.String(), func(t *testing.T) {
			t.Parallel()
			mt := f.MediaType()
			if mt == "" {
				t.Fatalf("Format(%d).MediaType() = %q, want a canonical media type; a new format needs an arm in MediaType", int(f), mt)
			}
			if fallback := fmt.Sprintf("Format(%d)", int(f)); f.String() == fallback {
				t.Errorf("Format(%d).String() = %q, the numeric fallback; a new format needs an arm in String", int(f), fallback)
			}
			if back, err := ParseMediaType(mt); back != f || err != nil {
				t.Errorf("ParseMediaType(Format(%d).MediaType() = %q) = %v, %v; want %v, nil", int(f), mt, back, err, f)
			}
		})
	}
	// Keeps the loop honest: a sentinel accidentally moved next to
	// FormatUnknown would make every assertion above vacuous.
	if members < 2 {
		t.Errorf("walked %d formats between FormatUnknown and formatSentinel, want at least 2 (FLAT and STRUCTURED)", members)
	}
}
