package constraints

import (
	"fmt"
	"regexp"
	"time"
)

// CDate constrains an RM ISO_DATE value (C_DATE). Pattern is the
// AOM 1.4 partial-date pattern (e.g. "yyyy-mm-dd", "yyyy-??-??");
// v1 preserves it verbatim for callers that need stricter
// enforcement than [Validate] performs. Validate accepts any
// well-formed ISO 8601 date — full ("2026-05-24"), year-month
// ("2026-05"), or year ("2026").
type CDate struct {
	Pattern string
}

func (CDate) isPrimitive() {}

// Validate accepts a Go string. The value MUST parse as an ISO 8601
// date in one of the three partial shapes (yyyy-mm-dd, yyyy-mm,
// yyyy). Pattern enforcement against AOM partial-date syntax is
// deferred — callers that need it pre-validate before calling.
func (c CDate) Validate(value any) []Violation {
	s, ok := value.(string)
	if !ok {
		return []Violation{{Code: CodeWrongType, Detail: fmt.Sprintf("expected string date, got %T", value)}}
	}
	for _, layout := range []string{"2006-01-02", "2006-01", "2006"} {
		if _, err := time.Parse(layout, s); err == nil {
			return nil
		}
	}
	return []Violation{{Code: CodeInvalidValue, Detail: fmt.Sprintf("%q is not a valid ISO 8601 date", s)}}
}

// CTime constrains an RM ISO_TIME value (C_TIME). Validate accepts
// "hh:mm:ss" and "hh:mm".
type CTime struct {
	Pattern string
}

func (CTime) isPrimitive() {}

// Validate accepts a Go string parsing as ISO 8601 time, optionally
// with trailing fractional seconds. Pattern enforcement deferred.
func (c CTime) Validate(value any) []Violation {
	s, ok := value.(string)
	if !ok {
		return []Violation{{Code: CodeWrongType, Detail: fmt.Sprintf("expected string time, got %T", value)}}
	}
	for _, layout := range []string{"15:04:05", "15:04", "15:04:05.000", "15:04:05Z07:00"} {
		if _, err := time.Parse(layout, s); err == nil {
			return nil
		}
	}
	return []Violation{{Code: CodeInvalidValue, Detail: fmt.Sprintf("%q is not a valid ISO 8601 time", s)}}
}

// CDateTime constrains an RM ISO_DATE_TIME value (C_DATE_TIME).
// Validate accepts RFC 3339 / ISO 8601 timestamps.
type CDateTime struct {
	Pattern string
}

func (CDateTime) isPrimitive() {}

// Validate accepts a Go string that parses under RFC 3339 (the ISO
// 8601 superset openEHR mandates) or its date-only / partial-time
// shortcuts. Pattern enforcement deferred.
func (c CDateTime) Validate(value any) []Violation {
	s, ok := value.(string)
	if !ok {
		return []Violation{{Code: CodeWrongType, Detail: fmt.Sprintf("expected string date-time, got %T", value)}}
	}
	for _, layout := range []string{time.RFC3339, time.RFC3339Nano, "2006-01-02T15:04:05", "2006-01-02T15:04"} {
		if _, err := time.Parse(layout, s); err == nil {
			return nil
		}
	}
	return []Violation{{Code: CodeInvalidValue, Detail: fmt.Sprintf("%q is not a valid ISO 8601 date-time", s)}}
}

// CDuration constrains an RM ISO_DURATION value (C_DURATION).
// Validate accepts the ISO 8601 PnYnMnDTnHnMnS shape. Numeric-range
// bounds on durations (e.g. "between PT1H and PT24H") are deferred
// — converting partial years / months to seconds requires calendar
// reasoning out of scope for a stdlib-only validator. AOM-pattern
// enforcement is similarly deferred.
type CDuration struct {
	Pattern string
}

func (CDuration) isPrimitive() {}

var durationRe = regexp.MustCompile(`^P(?:\d+Y)?(?:\d+M)?(?:\d+W)?(?:\d+D)?(?:T(?:\d+H)?(?:\d+M)?(?:\d+(?:\.\d+)?S)?)?$`)

// Validate accepts a Go string of the ISO 8601 duration shape.
// Returns CodeInvalidValue for shapes outside PnYnMnWnDTnHnMnS or
// for empty / period-only stubs (P, PT).
func (c CDuration) Validate(value any) []Violation {
	s, ok := value.(string)
	if !ok {
		return []Violation{{Code: CodeWrongType, Detail: fmt.Sprintf("expected string duration, got %T", value)}}
	}
	if s == "" || s == "P" || s == "PT" || !durationRe.MatchString(s) {
		return []Violation{{Code: CodeInvalidValue, Detail: fmt.Sprintf("%q is not a valid ISO 8601 duration", s)}}
	}
	return nil
}
