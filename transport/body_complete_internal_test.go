package transport

// body_complete_internal_test.go — REQ-151 § Whether the body arrived whole,
// for a read failure that no response bytes can raise.

import (
	"compress/flate"
	"fmt"
	"testing"
)

// TestBodyArrivedWholeFlateInternalError pins Complete for
// flate.InternalError, a content-coding failure body_complete_test.go cannot
// raise from bytes on the wire. compress/flate raises it ("unexpected length
// code") when the decoder for a dynamic block's code lengths yields a symbol
// above 18, but that decoder is built from 19 code lengths, so it only ever
// yields the symbols 0 to 18. The read and the declared length are those of a
// decompressed body with nothing read, so the declared-length rule cannot
// mark the failure complete on its own.
func TestBodyArrivedWholeFlateInternalError(t *testing.T) { // REQ-151
	internal := flate.InternalError("unexpected length code")
	cases := []struct {
		name    string
		readErr error
	}{
		{name: "bare", readErr: internal},
		{name: "wrapped", readErr: fmt.Errorf("gzip: %w", internal)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := bodyArrivedWhole(tc.readErr, 0, -1); !got {
				t.Errorf("bodyArrivedWhole(%v, 0, -1) = %v, want true", tc.readErr, got)
			}
		})
	}
}
