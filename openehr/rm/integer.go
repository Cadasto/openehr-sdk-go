package rm

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
)

// Integer is the BMM Integer primitive. Some upstream canonical JSON
// fixtures quote integral values as strings; decode accepts both forms.
type Integer int32

// UnmarshalJSON accepts a JSON number or a decimal integer string.
func (i *Integer) UnmarshalJSON(b []byte) error {
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
			return fmt.Errorf("rm.Integer: parse %q: %w", s, err)
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
