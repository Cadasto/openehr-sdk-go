package constraints_test

// temporal_validate_test.go: REQ-103 and REQ-123. Each temporal validator's
// outcome for a fixed table of ISO 8601 text: nil for a form the RM's
// temporal parse accepts, one CodeInvalidValue violation with the validator's
// Detail text and the input as Value for any other string, and one
// CodeWrongType violation with no Value for a value that is not a string.

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/template/constraints"
)

func TestREQ103TemporalValidateOutcomes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		validate  func(any) []constraints.Violation
		detail    string // Detail of the CodeInvalidValue violation
		wrongType string // Detail of the CodeWrongType violation for the int 42
		accepted  []string
		refused   []string
	}{
		{
			name:      "CDate",
			validate:  constraints.CDate{}.Validate,
			detail:    "value is not a valid ISO 8601 date",
			wrongType: "expected string date, got int",
			accepted:  []string{"2026-05-24", "2026-05", "2026", "20260524", "202605", "2024-02-29"},
			refused: []string{
				"", "2026-13-01", "2026-02-30", "2023-02-29", "2026-5-24", "26-05-24",
				"2026-05-24T10:00", "not-a-date", "２０２６",
			},
		},
		{
			name:      "CTime",
			validate:  constraints.CTime{}.Validate,
			detail:    "value is not a valid ISO 8601 time",
			wrongType: "expected string time, got int",
			accepted: []string{
				"10:30:00", "10:30", "10", "103000", "1030", "10:30:00.5", "10:30:00,5",
				"10:30:00Z", "10:30:00+01:00", "10:30:00+0100", "10:30:00+01", "10:30:00-12:00",
				"10:30:00.123456789Z", "103000,5+0100", "23:59:60",
			},
			refused: []string{
				"", "25:00", "24:00:00", "T10:00", "10:30:00.+01", "10:30:00,+01", "1:30",
				"10:30:00+15", "10:30.5", "10:30:00.", "noon",
			},
		},
		{
			name:      "CDateTime",
			validate:  constraints.CDateTime{}.Validate,
			detail:    "value is not a valid ISO 8601 date-time",
			wrongType: "expected string date-time, got int",
			accepted: []string{
				"2026-05-24T10:30:00Z", "2026-05-24T10:30:00", "2026-05-24T10:30", "2026-05-24T10",
				"2026-05-24", "2026-05", "2026", "20260524", "202605", "20251024T121033",
				"20260524T103000Z", "2026-05-24T10:30:00+0100", "2026-05-24T10:30:00,5+01",
				"20260524T103000+01:00",
			},
			refused: []string{
				"", "2026-13-01", "2026-13-01T10:00", "2026-05-24T25:00", "T10:00", "2026-05-24T",
				"2026-05T10:30", "2026-05-24T103000", "20260524T10:30:00", "2026-05-24 10:30:00",
				"2026-05-24T10:30:00.+01",
			},
		},
		{
			name:      "CDuration",
			validate:  constraints.CDuration{}.Validate,
			detail:    "value is not a valid ISO 8601 duration",
			wrongType: "expected string duration, got int",
			accepted: []string{
				"P1Y2M3DT4H5M6S", "PT5M", "P0D", "P1W", "P1Y2W3D", "-P1D", "PT1,5S", "PT1.5S",
				"-PT0S", "PT36H",
			},
			refused: []string{
				"", "P", "PT", "P1DT", "P1YT", "P1Y2MT", "P1.5D", "PT1.S", "P1D1Y", "1D",
				"P-1D", "P1M1M", "not-iso", "pt1s",
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			for _, s := range tc.accepted {
				if got := tc.validate(s); len(got) != 0 {
					t.Errorf("%s{}.Validate(%q) = %+v, want no violation", tc.name, s, got)
				}
			}
			for _, s := range tc.refused {
				want := []constraints.Violation{{Code: constraints.CodeInvalidValue, Detail: tc.detail, Value: constraints.Redact(s)}}
				if got := tc.validate(s); !slices.Equal(got, want) {
					t.Errorf("%s{}.Validate(%q) = %+v, want %+v", tc.name, s, got, want)
				}
			}
			want := []constraints.Violation{{Code: constraints.CodeWrongType, Detail: tc.wrongType}}
			if got := tc.validate(42); !slices.Equal(got, want) {
				t.Errorf("%s{}.Validate(42) = %+v, want %+v", tc.name, got, want)
			}
		})
	}
}

// patternCase is one ADL 1.4 pattern with values it admits and values it
// refuses, every one an ISO 8601 form the RM's temporal parse accepts.
type patternCase struct {
	name     string
	validate func(any) []constraints.Violation
	pattern  string
	admitted []string
	refused  []string
}

// TestREQ103TemporalPatternOutcomes pins REQ-103 § Validate contract on the
// ADL 1.4 date, time, date/time and duration patterns: a value its pattern
// admits has no violation, and one it refuses has one CodePatternMismatch
// that holds the value in Value only. The rows hold the patterns the
// vendored OPTs write, in both cases of their letters, and each rule of the
// reading: a mandatory, optional and disallowed field, a literal number, the
// three zone patterns, a fraction on the seconds, and a duration's
// designators either side of T.
func TestREQ103TemporalPatternOutcomes(t *testing.T) {
	t.Parallel()
	date := func(p string) func(any) []constraints.Violation { return constraints.CDate{Pattern: p}.Validate }
	clock := func(p string) func(any) []constraints.Violation { return constraints.CTime{Pattern: p}.Validate }
	stamp := func(p string) func(any) []constraints.Violation { return constraints.CDateTime{Pattern: p}.Validate }
	span := func(p string) func(any) []constraints.Violation { return constraints.CDuration{Pattern: p}.Validate }
	tests := []patternCase{
		{name: "date, day not allowed", validate: date("yyyy-??-XX"), pattern: "yyyy-??-XX",
			admitted: []string{"2019-01", "2019", "201901"}, refused: []string{"2019-01-14", "20190114"}},
		{name: "date, full", validate: date("yyyy-mm-dd"), pattern: "yyyy-mm-dd",
			admitted: []string{"2019-01-14", "20190114"}, refused: []string{"2019-01", "2019"}},
		{name: "date, upper-case letters", validate: date("YYYY-MM-??"), pattern: "YYYY-MM-??",
			admitted: []string{"2019-01-14", "2019-01"}, refused: []string{"2019"}},
		{name: "date, month and no day", validate: date("yyyy-mm-XX"), pattern: "yyyy-mm-XX",
			admitted: []string{"2019-01"}, refused: []string{"2019-01-14", "2019"}},
		{name: "date, lower-case x", validate: date("yyyy-mm-xx"), pattern: "yyyy-mm-xx",
			admitted: []string{"2019-01"}, refused: []string{"2019-01-14"}},
		{name: "date, literal year", validate: date("1995-??-XX"), pattern: "1995-??-XX",
			admitted: []string{"1995-03", "1995"}, refused: []string{"1996-03", "1995-03-01"}},
		{name: "date, literal month", validate: date("yyyy-06-dd"), pattern: "yyyy-06-dd",
			admitted: []string{"2019-06-14"}, refused: []string{"2019-07-14", "2019-06"}},
		{name: "time, full", validate: clock("hh:mm:ss"), pattern: "hh:mm:ss",
			admitted: []string{"10:30:00", "10:30:00.5", "10:30:00,5Z", "103000", "10:30:00+01:00"}, refused: []string{"10:30", "10"}},
		{name: "time, no seconds", validate: clock("hh:mm:XX"), pattern: "hh:mm:XX",
			admitted: []string{"10:30", "1030Z"}, refused: []string{"10:30:00", "10"}},
		{name: "time, optional minutes", validate: clock("HH:??:XX"), pattern: "HH:??:XX",
			admitted: []string{"10:30", "10"}, refused: []string{"10:30:00"}},
		{name: "time, any", validate: clock("hh:??:??"), pattern: "hh:??:??",
			admitted: []string{"10", "10:30", "10:30:00,5"}},
		{name: "time, literal hour", validate: clock("09:mm:ss"), pattern: "09:mm:ss",
			admitted: []string{"09:30:00"}, refused: []string{"10:30:00"}},
		{name: "time, zone hh:mm required", validate: clock("hh:mm:ss±hh:mm"), pattern: "hh:mm:ss±hh:mm",
			admitted: []string{"10:30:00+01:00", "10:30:00-0500", "10:30:00+01", "10:30:00Z"}, refused: []string{"10:30:00"}},
		{name: "time, zone hh required", validate: clock("hh:mm:ss+hh"), pattern: "hh:mm:ss+hh",
			admitted: []string{"10:30:00+01", "10:30:00-05:30", "10:30:00Z"}, refused: []string{"10:30:00"}},
		{name: "time, zone hhmm required", validate: clock("hh:mm:ss-HHMM"), pattern: "hh:mm:ss-HHMM",
			admitted: []string{"10:30:00+0100"}, refused: []string{"10:30:00.25"}},
		{name: "time, zone Z required", validate: clock("hh:mm:ssZ"), pattern: "hh:mm:ssZ",
			admitted: []string{"10:30:00Z"}, refused: []string{"10:30:00+00:00", "10:30:00"}},
		{name: "date-time, full", validate: stamp("yyyy-mm-ddTHH:MM:SS"), pattern: "yyyy-mm-ddTHH:MM:SS",
			admitted: []string{"2019-01-28T21:22:49,426+00:00", "2019-01-28T21:22:49", "20190128T212249Z"},
			refused:  []string{"2019-01-28T21:22", "2019-01-28", "2019"}},
		{name: "date-time, optional minutes and seconds", validate: stamp("yyyy-mm-ddTHH:??:??"), pattern: "yyyy-mm-ddTHH:??:??",
			admitted: []string{"2019-01-28T21", "2019-01-28T21:22", "2019-01-28T21:22:49.5Z"}, refused: []string{"2019-01-28", "2019-01"}},
		{name: "date-time, no seconds", validate: stamp("yyyy-mm-ddThh:mm:XX"), pattern: "yyyy-mm-ddThh:mm:XX",
			admitted: []string{"2019-01-28T21:22Z"}, refused: []string{"2019-01-28T21:22:49", "2019-01-28T21"}},
		{name: "date-time, any", validate: stamp("yyyy-??-??T??:??:??"), pattern: "yyyy-??-??T??:??:??",
			admitted: []string{"2019", "2019-01", "2019-01-28", "2019-01-28T21", "2019-01-28T21:22:49"}},
		{name: "date-time, no time", validate: stamp("yyyy-mm-ddTXX:XX:XX"), pattern: "yyyy-mm-ddTXX:XX:XX",
			admitted: []string{"2019-01-28"}, refused: []string{"2019-01-28T21", "2019-01"}},
		{name: "date-time, space before the time", validate: stamp("yyyy-mm-dd hh:mm:ss"), pattern: "yyyy-mm-dd hh:mm:ss",
			admitted: []string{"2019-01-28T21:22:49"}, refused: []string{"2019-01-28T21:22"}},
		{name: "date-time, zone required", validate: stamp("yyyy-mm-ddThh:mm:ss+hh:mm"), pattern: "yyyy-mm-ddThh:mm:ss+hh:mm",
			admitted: []string{"2019-01-28T21:22:49+01:00", "2019-01-28T21:22:49Z"}, refused: []string{"2019-01-28T21:22:49"}},
		{name: "duration, years", validate: span("PY"), pattern: "PY",
			admitted: []string{"P45Y", "-P2Y", "P0Y"}, refused: []string{"P0D", "P1Y2M", "PT1H"}},
		{name: "duration, years or months", validate: span("PYM"), pattern: "PYM",
			admitted: []string{"P1Y2M", "P3M", "P1Y"}, refused: []string{"P1Y2D", "P1W"}},
		{name: "duration, months weeks or days", validate: span("PMWD"), pattern: "PMWD",
			admitted: []string{"P4W3D", "P2M", "P5D", "-P1M1W"}, refused: []string{"P1Y", "PT1H"}},
		{name: "duration, any date part", validate: span("PYMWD"), pattern: "PYMWD",
			admitted: []string{"P1Y2M3W4D"}, refused: []string{"P1DT1H"}},
		{name: "duration, minutes", validate: span("PTM"), pattern: "PTM",
			admitted: []string{"PT5M", "-PT5M"}, refused: []string{"PT5H", "P5M", "PT5M1S"}},
		{name: "duration, lower-case letters", validate: span("Pwd"), pattern: "Pwd",
			admitted: []string{"P4W", "P3D", "P4W3D"}, refused: []string{"P1M"}},
		{name: "duration, hours or minutes", validate: span("PThm"), pattern: "PThm",
			admitted: []string{"PT2H30M", "PT2H"}, refused: []string{"PT2H30M5S"}},
		{name: "duration, seconds with a fraction", validate: span("PTS"), pattern: "PTS",
			admitted: []string{"PT1.5S", "PT1,5S", "-PT0S"}, refused: []string{"PT1M"}},
		{name: "duration, days and time", validate: span("PDTHMS"), pattern: "PDTHMS",
			admitted: []string{"P1DT2H3M4S", "PT4S"}, refused: []string{"P1W", "P1Y"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			for _, s := range tc.admitted {
				if got := tc.validate(s); len(got) != 0 {
					t.Errorf("pattern %q: Validate(%q) = %+v, want no violation", tc.pattern, s, got)
				}
			}
			for _, s := range tc.refused {
				want := []constraints.Violation{{
					Code:   constraints.CodePatternMismatch,
					Detail: fmt.Sprintf("value does not match pattern %q", tc.pattern),
					Value:  constraints.Redact(s),
				}}
				if got := tc.validate(s); !slices.Equal(got, want) {
					t.Errorf("pattern %q: Validate(%q) = %+v, want %+v", tc.pattern, s, got, want)
				}
			}
		})
	}
}

// TestREQ103TemporalPatternMalformed pins REQ-103 § Validate contract on a
// pattern outside the ADL 1.4 grammar, or one that breaks its validity rules
// for ?? and XX: every value, a valid one included, gets one
// CodeInvalidValue that names the pattern and holds no Value, as a malformed
// CString regex does.
func TestREQ103TemporalPatternMalformed(t *testing.T) {
	t.Parallel()
	tests := []struct {
		kind     string
		validate func(pattern string) func(any) []constraints.Violation
		value    string
		patterns []string
	}{
		{
			kind:     "date",
			validate: func(p string) func(any) []constraints.Violation { return constraints.CDate{Pattern: p}.Validate },
			value:    "2019-01-14",
			patterns: []string{
				"yyyy-mm", "yyyy-XX-dd", "yyyy-??-dd", "????-mm-dd", "XXXX-mm-dd", "yyyy-mm-dd+hh:mm",
				"yyyy/mm/dd", "yyyy-mX-dd", "yyyy-mm-ddT", "yyy-mm-dd", "PY", "195-??-XX",
			},
		},
		{
			kind:     "time",
			validate: func(p string) func(any) []constraints.Violation { return constraints.CTime{Pattern: p}.Validate },
			value:    "10:30:00",
			patterns: []string{
				"??:mm:ss", "XX:mm:ss", "hh:mm", "hh:XX:ss", "hh:??:ss", "hh:mm:ss+h", "hh:mm:ssZZ",
				"hh:mm:ss+hh:", "hh:mm:ss*hh", "hh-mm-ss",
			},
		},
		{
			kind:     "date-time",
			validate: func(p string) func(any) []constraints.Violation { return constraints.CDateTime{Pattern: p}.Validate },
			value:    "2019-01-28T21:22:49",
			patterns: []string{
				"yyyy-mm-dd", "yyyy-mm-ddThh:mm", "yyyy-??-ddThh:mm:ss", "yyyy-mm-??Thh:mm:ss",
				"yyyy-mm-ddTXX:XX:XX+hh", "yyyy-mm-ddXhh:mm:ss", "hh:mm:ss",
			},
		},
		{
			kind:     "duration",
			validate: func(p string) func(any) []constraints.Violation { return constraints.CDuration{Pattern: p}.Validate },
			value:    "P1D",
			patterns: []string{"P", "PT", "PYY", "PDY", "PTY", "Y", "PH", "P1Y", "PTT", "PTHT", "yyyy-mm-dd"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.kind, func(t *testing.T) {
			t.Parallel()
			for _, p := range tc.patterns {
				want := []constraints.Violation{{
					Code:   constraints.CodeInvalidValue,
					Detail: fmt.Sprintf("constraint pattern %q is not a valid ADL 1.4 %s pattern", p, tc.kind),
				}}
				if got := tc.validate(p)(tc.value); !slices.Equal(got, want) {
					t.Errorf("pattern %q: Validate(%q) = %+v, want %+v", p, tc.value, got, want)
				}
			}
		})
	}
}

// closedSpan is the TemporalRange from lower to upper, both included.
func closedSpan(lower, upper string) constraints.TemporalRange {
	return constraints.TemporalRange{Lower: lower, Upper: upper, LowerInclusive: true, UpperInclusive: true}
}

// TestREQ103TemporalRangeOutcomes pins REQ-103 § Validate contract on a
// temporal range: a value inside it has no violation, and one outside it has
// one CodeOutOfRange that names the range and holds the value in Value only.
// Durations order by BASE Iso8601_duration.to_seconds, so P1D equals PT24H
// and 48 months (1460.16 days) fall short of 4 years (1460.96 days). A
// partial date or time stands for the start of the period it names, and the
// order is clock-local, so a zone on the value or on a bound is not applied.
func TestREQ103TemporalRangeOutcomes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		validate func(any) []constraints.Violation
		detail   string // Detail of the CodeOutOfRange violation
		admitted []string
		refused  []string
	}{
		{
			name:     "duration, PT24H only",
			validate: constraints.CDuration{Range: closedSpan("PT24H", "PT24H")}.Validate,
			detail:   "value outside [PT24H..PT24H]",
			admitted: []string{"PT24H", "P1D", "PT1440M", "PT86400S"},
			refused:  []string{"P0D", "PT23H", "P2D", "-PT24H"},
		},
		{
			name:     "duration, years with a pattern",
			validate: constraints.CDuration{Pattern: "PY", Range: closedSpan("P0Y", "P150Y")}.Validate,
			detail:   "value outside [P0Y..P150Y]",
			admitted: []string{"P0Y", "P150Y", "P45Y"},
			refused:  []string{"P151Y", "-P1Y"},
		},
		{
			name:     "duration, at least 4 years",
			validate: constraints.CDuration{Range: constraints.TemporalRange{Lower: "P4Y", LowerInclusive: true, UpperUnbounded: true}}.Validate,
			detail:   "value outside [P4Y..*)",
			admitted: []string{"P4Y", "P1461D", "P49M", "P5Y"},
			refused:  []string{"P3Y", "P1460D", "P48M"},
		},
		{
			name:     "duration, both ends excluded",
			validate: constraints.CDuration{Range: constraints.TemporalRange{Lower: "PT0S", Upper: "PT1H"}}.Validate,
			detail:   "value outside (PT0S..PT1H)",
			admitted: []string{"PT1S", "PT59M", "PT0.5S"},
			refused:  []string{"PT0S", "PT1H", "-PT1S", "P0D"},
		},
		{
			name:     "duration, at most zero",
			validate: constraints.CDuration{Range: constraints.TemporalRange{LowerUnbounded: true, Upper: "P0Y", UpperInclusive: true}}.Validate,
			detail:   "value outside (*..P0Y]",
			admitted: []string{"-P5D", "P0D", "-P1Y2M"},
			refused:  []string{"P1D", "PT1S"},
		},
		{
			name:     "date",
			validate: constraints.CDate{Range: closedSpan("2004-05-20", "2004-06-02")}.Validate,
			detail:   "value outside [2004-05-20..2004-06-02]",
			admitted: []string{"2004-05-20", "2004-06-02", "20040525", "2004-06"},
			refused:  []string{"2004-05-19", "2004-06-03", "2004-05", "2004"},
		},
		{
			name:     "time, at or after 09:30",
			validate: constraints.CTime{Range: constraints.TemporalRange{Lower: "09:30:00", LowerInclusive: true, UpperUnbounded: true}}.Validate,
			detail:   "value outside [09:30:00..*)",
			admitted: []string{"09:30:00", "10", "09:30:00+05:00", "0930"},
			refused:  []string{"09:29:59", "09", "09:00:00-05:00", "09:29:59.999Z"},
		},
		{
			name:     "time, after 09:30",
			validate: constraints.CTime{Range: constraints.TemporalRange{Lower: "09:30:00+02:00", UpperUnbounded: true}}.Validate,
			detail:   "value outside (09:30:00+02:00..*)",
			admitted: []string{"09:30:00.5", "09:31", "09:30:01Z"},
			refused:  []string{"09:30:00", "09:30", "09:30:00+02:00", "08:30:00Z"},
		},
		{
			name:     "date-time",
			validate: constraints.CDateTime{Range: closedSpan("2004-05-20T00:00:00", "2005-05-19T23:59:59")}.Validate,
			detail:   "value outside [2004-05-20T00:00:00..2005-05-19T23:59:59]",
			admitted: []string{"2004-05-20T00:00:00", "2004-05-20", "2004-06", "2005-05-19T23:59:59Z", "20041231T120000"},
			refused:  []string{"2004-05-19T23:59:59", "2005-05-20", "2004-05", "2005-05-19T23:59:59.5"},
		},
		{
			name:     "date-time, zone on the bound",
			validate: constraints.CDateTime{Range: constraints.TemporalRange{Lower: "2004-05-20T10:00:00+02:00", LowerInclusive: true, UpperUnbounded: true}}.Validate,
			detail:   "value outside [2004-05-20T10:00:00+02:00..*)",
			admitted: []string{"2004-05-20T10:00:00-05:00", "2004-05-20T10:00:00", "2004-05-20T10:00Z"},
			refused:  []string{"2004-05-20T09:00:00Z", "2004-05-20T09:59:59+02:00", "2004-05-20"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			for _, s := range tc.admitted {
				if got := tc.validate(s); len(got) != 0 {
					t.Errorf("Validate(%q) = %+v, want no violation", s, got)
				}
			}
			for _, s := range tc.refused {
				want := []constraints.Violation{{Code: constraints.CodeOutOfRange, Detail: tc.detail, Value: constraints.Redact(s)}}
				if got := tc.validate(s); !slices.Equal(got, want) {
					t.Errorf("Validate(%q) = %+v, want %+v", s, got, want)
				}
			}
		})
	}
}

// TestREQ103TemporalPatternAndRangeBothReported pins the Validate contract's
// one violation per failing clause: a duration that both its pattern and its
// range refuse gets the CodePatternMismatch, then the CodeOutOfRange.
func TestREQ103TemporalPatternAndRangeBothReported(t *testing.T) {
	t.Parallel()
	c := constraints.CDuration{Pattern: "PY", Range: constraints.TemporalRange{Lower: "P1Y", LowerInclusive: true, UpperUnbounded: true}}
	want := []constraints.Violation{
		{Code: constraints.CodePatternMismatch, Detail: `value does not match pattern "PY"`, Value: constraints.Redact("PT1H")},
		{Code: constraints.CodeOutOfRange, Detail: "value outside [P1Y..*)", Value: constraints.Redact("PT1H")},
	}
	if got := c.Validate("PT1H"); !slices.Equal(got, want) {
		t.Errorf("Validate(%q) = %+v, want %+v", "PT1H", got, want)
	}
	date := constraints.CDate{Pattern: "yyyy-mm-dd", Range: closedSpan("2004-05-20", "2004-06-02")}
	wantDate := []constraints.Violation{
		{Code: constraints.CodePatternMismatch, Detail: `value does not match pattern "yyyy-mm-dd"`, Value: constraints.Redact("2004-05")},
		{Code: constraints.CodeOutOfRange, Detail: "value outside [2004-05-20..2004-06-02]", Value: constraints.Redact("2004-05")},
	}
	if got := date.Validate("2004-05"); !slices.Equal(got, wantDate) {
		t.Errorf("Validate(%q) = %+v, want %+v", "2004-05", got, wantDate)
	}
}

// TestREQ103TemporalRangeMalformedBound pins REQ-103 § Validate contract on
// a TemporalRange built in code with a bound that the ISO 8601 parse of the
// constrained type refuses, on a side that is not unbounded: every value, a
// valid one included, gets one CodeInvalidValue that names the range and
// holds no Value.
func TestREQ103TemporalRangeMalformedBound(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		validate func(any) []constraints.Violation
		value    string
		detail   string
	}{
		{
			name:     "duration lower",
			validate: constraints.CDuration{Range: constraints.TemporalRange{Lower: "P1X", LowerInclusive: true, UpperUnbounded: true}}.Validate,
			value:    "P1D",
			detail:   "constraint range [P1X..*) has a bound that is not a valid ISO 8601 duration",
		},
		{
			name:     "date upper",
			validate: constraints.CDate{Range: closedSpan("2004-01-01", "2004-02-30")}.Validate,
			value:    "2004-01-10",
			detail:   "constraint range [2004-01-01..2004-02-30] has a bound that is not a valid ISO 8601 date",
		},
		{
			name:     "time upper left empty",
			validate: constraints.CTime{Range: constraints.TemporalRange{LowerUnbounded: true, UpperInclusive: true}}.Validate,
			value:    "10:00",
			detail:   "constraint range (*..] has a bound that is not a valid ISO 8601 time",
		},
		{
			name:     "date-time lower of another type",
			validate: constraints.CDateTime{Range: constraints.TemporalRange{Lower: "P1D", LowerInclusive: true, UpperUnbounded: true}}.Validate,
			value:    "2004-01-10",
			detail:   "constraint range [P1D..*) has a bound that is not a valid ISO 8601 date-time",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			want := []constraints.Violation{{Code: constraints.CodeInvalidValue, Detail: tc.detail}}
			if got := tc.validate(tc.value); !slices.Equal(got, want) {
				t.Errorf("Validate(%q) = %+v, want %+v", tc.value, got, want)
			}
		})
	}
}

// TestREQ168TemporalViolationsValueFree pins REQ-168 on the temporal pattern
// and range clauses: neither Code nor Detail repeats any part of the value,
// which sits in Value alone. The values carry digits no pattern or bound in
// this test does.
func TestREQ168TemporalViolationsValueFree(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		c      constraints.PrimitiveConstraint
		value  string
		needle string
	}{
		{name: "date pattern", c: constraints.CDate{Pattern: "yyyy-??-XX"}, value: "1987-06-05", needle: "1987"},
		{name: "date range", c: constraints.CDate{Range: closedSpan("2004-05-20", "2004-06-02")}, value: "1987-06-05", needle: "1987"},
		{name: "time pattern", c: constraints.CTime{Pattern: "hh:mm:XX"}, value: "13:47:39", needle: "47:39"},
		{name: "time range", c: constraints.CTime{Range: closedSpan("09:00:00", "10:00:00")}, value: "13:47:39", needle: "47:39"},
		{name: "date-time pattern", c: constraints.CDateTime{Pattern: "yyyy-mm-ddThh:mm:XX"}, value: "1987-06-05T13:47:39", needle: "1987"},
		{name: "date-time range", c: constraints.CDateTime{Range: closedSpan("2004-05-20", "2004-06-02")}, value: "1987-06-05T13:47:39", needle: "1987"},
		{name: "duration pattern", c: constraints.CDuration{Pattern: "PY"}, value: "P987654321D", needle: "987654321"},
		{name: "duration range", c: constraints.CDuration{Range: closedSpan("P0D", "P1D")}, value: "P987654321D", needle: "987654321"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := tc.c.Validate(tc.value)
			if len(got) != 1 {
				t.Fatalf("Validate(%q) = %+v, want one violation", tc.value, got)
			}
			if v := got[0]; strings.Contains(string(v.Code), tc.needle) || strings.Contains(v.Detail, tc.needle) {
				t.Errorf("Validate(%q) = %+v: Code or Detail holds %q from the value", tc.value, v, tc.needle)
			}
			if got[0].Value != constraints.Redact(tc.value) {
				t.Errorf("Validate(%q).Value = %v, want the value", tc.value, got[0].Value)
			}
		})
	}
}
