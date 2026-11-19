package aom14

import (
	"bytes"
	"testing"
)

// TestReadPoolsNoBufferPastTheLimit pins the bound on inputBuffers: a read
// whose input outgrows inputBufferMax does not return its buffer to the pool,
// so one large template does not keep its size for the life of the process.
// The input is a bare element after a long run of spaces; whatever the read
// makes of it, it keeps every byte of it in its buffer first, which is all the
// test needs.
//
// It reads the pool the reader puts into, so it does not run in parallel. A
// buffer put back is not certain to come out of the pool again, so the test
// cannot fail spuriously: without the bound, the pool gives back the large
// buffer to the next Get on the same processor, and the test fails.
func TestReadPoolsNoBufferPastTheLimit(t *testing.T) {
	input := append(bytes.Repeat([]byte(" "), inputBufferMax+1), "<x/>"...)
	_, _ = ReadOperationalTemplate(bytes.NewReader(input)) // only the buffer it fills matters
	for range 4 {
		b := inputBuffers.Get().(*bytes.Buffer)
		if b.Cap() > inputBufferMax {
			t.Fatalf("inputBuffers kept a buffer of %d bytes after a read of %d; want none larger than %d", b.Cap(), len(input), inputBufferMax)
		}
	}
}
