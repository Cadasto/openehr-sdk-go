package constraints

import (
	"fmt"

	"github.com/cadasto/openehr-sdk-go/internal/iso8601"
)

// CDate constrains an RM ISO_DATE value (C_DATE). Pattern is the
// AOM 1.4 partial-date pattern (e.g. "yyyy-mm-dd", "yyyy-??-??");
// it is preserved verbatim for callers that need stricter
// enforcement than [CDate.Validate] performs. Validate accepts
// exactly the strings rm.DVDate.ValidISO8601 accepts: a full date, a
// year and month, or a year alone, in the extended layout
// ("2026-05-24", "2026-05", "2026") or the basic one ("20260524",
// "202605"), with the month and day checked against the calendar.
type CDate struct {
	Pattern string
	Range   TemporalRange
}

func (CDate) isPrimitive() {}

// ExampleValue returns "2020-01-01", the documented sentinel
// satisfying the ISO 8601 full-date shape Validate accepts.
// The AOM partial-date pattern is not enforced.
func (CDate) ExampleValue() any { return "2020-01-01" }

// Validate accepts a Go string that rm.DVDate.ValidISO8601 accepts as
// a date value, partial dates included, and reports any other string
// as not a valid ISO 8601 date. The AOM partial-date pattern is not
// enforced; callers that need it pre-validate before calling.
func (c CDate) Validate(value any) []Violation {
	s, ok := value.(string)
	if !ok {
		return []Violation{{Code: CodeWrongType, Detail: fmt.Sprintf("expected string date, got %T", value)}}
	}
	if iso8601.ValidDate(s) {
		return nil
	}
	return []Violation{{Code: CodeInvalidValue, Detail: "value is not a valid ISO 8601 date", Value: Redact(value)}}
}

// CTime constrains an RM ISO_TIME value (C_TIME). Validate accepts
// exactly the strings rm.DVTime.ValidISO8601 accepts: the extended
// layouts hh:mm:ss and hh:mm, the basic layouts hhmmss and hhmm, or
// hh alone, with a comma or dot fraction on the seconds and a zone of
// Z, ±hh, ±hhmm or ±hh:mm. The hour runs from 00 to 23, and the
// second may be 60 for a leap second.
type CTime struct {
	Pattern string
	Range   TemporalRange
}

func (CTime) isPrimitive() {}

// ExampleValue returns "12:00:00", the documented sentinel
// satisfying the ISO 8601 hh:mm:ss shape Validate accepts.
func (CTime) ExampleValue() any { return "12:00:00" }

// Validate accepts a Go string that rm.DVTime.ValidISO8601 accepts as
// a time value, partial times included, and reports any other string
// as not a valid ISO 8601 time. The AOM pattern is not enforced.
func (c CTime) Validate(value any) []Violation {
	s, ok := value.(string)
	if !ok {
		return []Violation{{Code: CodeWrongType, Detail: fmt.Sprintf("expected string time, got %T", value)}}
	}
	if iso8601.ValidTime(s) {
		return nil
	}
	return []Violation{{Code: CodeInvalidValue, Detail: "value is not a valid ISO 8601 time", Value: Redact(value)}}
}

// CDateTime constrains an RM ISO_DATE_TIME value (C_DATE_TIME).
// Validate accepts exactly the strings rm.DVDateTime.ValidISO8601
// accepts: a date as [CDate] accepts it, optionally followed by "T"
// and a time as [CTime] accepts it. A time needs a full date before
// it, and the date and the time use the same layout, both extended
// ("2026-05-24T10:30:00Z") or both basic ("20260524T103000Z"); the
// zone may be written in either layout.
type CDateTime struct {
	Pattern string
	Range   TemporalRange
}

func (CDateTime) isPrimitive() {}

// ExampleValue returns "2020-01-01T12:00:00Z", the documented
// sentinel satisfying the RFC 3339 shape Validate accepts.
func (CDateTime) ExampleValue() any { return "2020-01-01T12:00:00Z" }

// Validate accepts a Go string that rm.DVDateTime.ValidISO8601 accepts
// as a date-time value, a date alone and other partial forms included,
// and reports any other string as not a valid ISO 8601 date-time. The
// AOM pattern is not enforced.
func (c CDateTime) Validate(value any) []Violation {
	s, ok := value.(string)
	if !ok {
		return []Violation{{Code: CodeWrongType, Detail: fmt.Sprintf("expected string date-time, got %T", value)}}
	}
	if iso8601.ValidDateTime(s) {
		return nil
	}
	return []Violation{{Code: CodeInvalidValue, Detail: "value is not a valid ISO 8601 date-time", Value: Redact(value)}}
}

// CDuration constrains an RM ISO_DURATION value (C_DURATION).
// Validate accepts exactly the strings rm.DVDuration.ValidISO8601
// accepts: P[nY][nM][nW][nD][T[nH][nM][nS]], with each designator at
// most once and in that order, at least one component in all and at
// least one after a T, and a comma or dot fraction on the seconds
// only. Two openEHR additions to ISO 8601 are accepted too: a leading
// minus sign ("-P1D") and W mixed with the other designators
// ("P1Y2W"). It does not enforce numeric-range bounds on durations
// (e.g. "between PT1H and PT24H"): converting partial years / months
// to seconds needs calendar reasoning that this validator does not
// do. The AOM pattern is not enforced either.
type CDuration struct {
	Pattern string
	Range   TemporalRange
}

func (CDuration) isPrimitive() {}

// ExampleValue returns "P0D", the documented sentinel satisfying
// the ISO 8601 duration shape Validate accepts.
func (CDuration) ExampleValue() any { return "P0D" }

// Validate accepts a Go string that rm.DVDuration.ValidISO8601 accepts
// as a duration value, and reports any other string, such as "", "P",
// "PT" or "P1DT", as not a valid ISO 8601 duration.
func (c CDuration) Validate(value any) []Violation {
	s, ok := value.(string)
	if !ok {
		return []Violation{{Code: CodeWrongType, Detail: fmt.Sprintf("expected string duration, got %T", value)}}
	}
	if iso8601.ValidDuration(s) {
		return nil
	}
	return []Violation{{Code: CodeInvalidValue, Detail: "value is not a valid ISO 8601 duration", Value: Redact(value)}}
}

// TemporalRange is the AOM 1.4 range of a C_DATE, C_TIME, C_DATE_TIME or
// C_DURATION, its bounds ISO 8601 text of the constrained type.
type TemporalRange struct {
	Lower, Upper                   string
	LowerInclusive, UpperInclusive bool
	LowerUnbounded, UpperUnbounded bool
}
