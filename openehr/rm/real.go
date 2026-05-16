package rm

import (
	"encoding/json"
	"fmt"
	"strconv"
)

// Real is the BMM Real primitive. Upstream CDR canonical JSON sometimes
// emits decimal magnitudes as quoted strings; decode accepts both forms.
type Real float64

// UnmarshalJSON accepts a JSON number or a decimal string.
func (r *Real) UnmarshalJSON(b []byte) error {
	if len(b) == 0 {
		return fmt.Errorf("rm.Real: empty input")
	}
	if b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return fmt.Errorf("rm.Real: %w", err)
		}
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return fmt.Errorf("rm.Real: parse %q: %w", s, err)
		}
		*r = Real(f)
		return nil
	}
	var f float64
	if err := json.Unmarshal(b, &f); err != nil {
		return fmt.Errorf("rm.Real: %w", err)
	}
	*r = Real(f)
	return nil
}

// MarshalJSON emits a JSON number per REQ-052.
func (r Real) MarshalJSON() ([]byte, error) {
	return json.Marshal(float64(r))
}
