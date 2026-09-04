package rm

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
)

// Integer is the BMM Integer primitive. Some upstream canonical JSON
// fixtures quote integral values as strings; decode accepts both forms.
type Integer int32

// UnmarshalJSON accepts a JSON number or a decimal integer string.
//
// A nil receiver is refused rather than dereferenced (REQ-025, idiom.md
// § No panics): the method assigns through the pointer, and a nil
// pointer is caller-constructible input reachable through the documented
// API. That refusal is a plain error, outside typereg.ErrInvalidShape —
// caller misuse is not a wire-shape problem.
func (i *Integer) UnmarshalJSON(b []byte) error {
	if i == nil {
		return fmt.Errorf("rm.Integer: %w", typereg.ErrNilReceiver)
	}
	if len(b) == 0 {
		return errors.New("rm.Integer: empty input")
	}
	if b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return fmt.Errorf("rm.Integer: %w", err)
		}
		n, err := strconv.ParseInt(s, 10, 32)
		if err != nil {
			// The *strconv.NumError already quotes the literal once; a second
			// echo here would double it (wire.md § REQ-052).
			return fmt.Errorf("rm.Integer: parse quoted literal: %w", err)
		}
		*i = Integer(n)
		return nil
	}
	var n int32
	if err := json.Unmarshal(b, &n); err != nil {
		return fmt.Errorf("rm.Integer: %w", err)
	}
	*i = Integer(n)
	return nil
}

// MarshalJSON emits a JSON number.
func (i Integer) MarshalJSON() ([]byte, error) {
	return json.Marshal(int32(i))
}
