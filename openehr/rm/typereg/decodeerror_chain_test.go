package typereg_test

import (
	"errors"
	"runtime"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
)

// TestREQ056_DecodeErrorChainMessage pins the message of a chain of
// envelopes, each the Inner of the one before: every envelope's own text in
// turn, in each of the four forms Path and Type give it, then the cause of the
// innermost. It is the text formatting each Inner in turn gives, so a chain
// reads the same however it is rendered.
func TestREQ056_DecodeErrorChainMessage(t *testing.T) {
	var typedNil *typereg.DecodeError
	for _, tc := range []struct {
		name string
		err  *typereg.DecodeError
		want string
	}{
		{
			name: "four forms, then the cause",
			err: &typereg.DecodeError{Path: "/a", Type: "A", Inner: &typereg.DecodeError{
				Path: "/b", Inner: &typereg.DecodeError{
					Type: "C", Inner: &typereg.DecodeError{Inner: errors.New("boom")},
				},
			}},
			want: `decode /a (_type="A"): decode /b: decode _type="C": decode: boom`,
		},
		{
			name: "innermost without a cause",
			err:  &typereg.DecodeError{Path: "/a", Inner: &typereg.DecodeError{Path: "/b"}},
			want: `decode /a: decode /b: unspecified error`,
		},
		{
			name: "a typed nil envelope ends the chain",
			err:  &typereg.DecodeError{Path: "/a", Inner: typedNil},
			want: `decode /a: decode: unspecified error`,
		},
		{
			name: "a wrapped envelope is the cause, rendered whole",
			err: &typereg.DecodeError{Path: "/a", Inner: errors.Join(
				&typereg.DecodeError{Path: "/b", Inner: errors.New("boom")},
			)},
			want: `decode /a: decode /b: boom`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.err.Error(); got != tc.want {
				t.Errorf("Error() = %q\nwant      %q", got, tc.want)
			}
		})
	}
}

// TestREQ056_DecodeErrorChainRendersInLinearMemory pins the cost of rendering
// a long chain of envelopes: memory in proportion to the message, not to the
// square of the chain's length. A decode that fails deep inside nested polymorphic
// slots builds a chain as long as the nesting, and rendering each inner
// message in turn, then copying it into the outer one, made a chain 10,000
// envelopes long take seconds.
//
// The bound is eight times the message's length: a buffer that doubles as it
// grows allocates less than twice the text it ends with, and the rest is
// headroom for the cause and the runtime. Formatting each inner message in
// turn allocates about the chain's length over two times the message.
//
// Can-fail control: render the Inner of each envelope with fmt, as one call
// per envelope, and the bytes allocated exceed the bound by two orders of
// magnitude.
func TestREQ056_DecodeErrorChainRendersInLinearMemory(t *testing.T) {
	const length = 4000
	err := &typereg.DecodeError{Path: "/items", Type: "ELEMENT", Inner: errors.New("boom")}
	for range length - 1 {
		err = &typereg.DecodeError{Path: "/items", Type: "CLUSTER", Inner: err}
	}

	var (
		msg  string
		best uint64
	)
	for i := range 3 {
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		msg = err.Error()
		runtime.ReadMemStats(&after)
		if n := after.TotalAlloc - before.TotalAlloc; i == 0 || n < best {
			best = n
		}
	}

	if got := strings.Count(msg, "decode /items"); got != length {
		t.Fatalf("Error() names %d envelopes; want %d", got, length)
	}
	if !strings.HasSuffix(msg, `decode /items (_type="ELEMENT"): boom`) {
		t.Fatalf("Error() ends %q; want the innermost envelope and its cause", msg[max(0, len(msg)-80):])
	}
	if limit := 8 * uint64(len(msg)); best > limit {
		t.Errorf("Error() of a chain of %d envelopes allocated %d bytes for a %d-byte message; want at most %d",
			length, best, len(msg), limit)
	}
}
