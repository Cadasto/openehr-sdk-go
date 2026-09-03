package rm_test

import (
	"errors"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
)

// TestIntegerNilReceiverIsRefusedNotPanicked pins the nil-receiver axis
// (idiom.md § No panics, REQ-025): UnmarshalJSON assigns through the
// pointer, so a nil *rm.Integer would be dereferenced. A nil pointer is
// caller-constructible input reachable through the documented API, not a
// programmer error, so the method must fail closed with a plain error —
// caller misuse is not a wire-shape problem and must stay outside
// typereg.ErrInvalidShape. A panic fails the test run outright.
//
// `null` is the discriminating second input: encoding/json treats a null
// literal as a no-op for a non-pointer target, so it reaches the
// assignment with nothing to report, and a nil check placed after the
// decode would let it through and panic on a value that is not even
// there.
func TestIntegerNilReceiverIsRefusedNotPanicked(t *testing.T) {
	for _, in := range []string{"42", `"42"`, "null"} {
		t.Run(in, func(t *testing.T) {
			var i *rm.Integer
			err := i.UnmarshalJSON([]byte(in))
			if err == nil {
				t.Fatalf("(*rm.Integer)(nil).UnmarshalJSON(%s) = nil error, want a refusal", in)
			}
			if errors.Is(err, typereg.ErrInvalidShape) {
				t.Errorf("err = %v; caller misuse must not classify as a wire-shape failure", err)
			}
		})
	}
}
