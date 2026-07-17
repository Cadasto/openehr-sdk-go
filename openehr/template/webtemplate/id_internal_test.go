package webtemplate

import (
	"slices"
	"testing"
)

// REQ-106, ADR-0014 — lock the reverse-engineered EHRbase id sanitisation
// rules (diacritics kept, non-alphanumerics collapse to '_', trailing '.'
// kept, leading-digit 'a' guard).
func TestSanitizeID(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Blood pressure", "blood_pressure"},
		{"Start_time", "start_time"},
		{"Geschichte/Historie", "geschichte_historie"},
		{"Körpertemperatur", "körpertemperatur"}, // diacritics kept
		{"Vorhanden?", "vorhanden"},              // trailing punct trimmed
		{"Bezeichnung des Symptoms oder Anzeichens.", "bezeichnung_des_symptoms_oder_anzeichens."}, // trailing dot kept
		{"*Agent (en)", "agent_en"},
		{"Menschen, die dort", "menschen_die_dort"},
		{"24 hour average", "a24_hour_average"},            // leading-digit guard
		{"Medikations-Screening", "medikations-screening"}, // hyphen kept
		{"", ""},
	}
	for _, tc := range cases {
		if got := sanitizeID(tc.in); got != tc.want {
			t.Errorf("sanitizeID(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// durationFields maps an ISO-8601-style C_DURATION pattern to ordered field
// names (REQ-106).
func TestDurationFields(t *testing.T) {
	cases := []struct {
		pattern string
		want    []string
	}{
		{"PYMWD", []string{"year", "month", "week", "day"}},
		{"PY", []string{"year"}},
		{"PMWD", []string{"month", "week", "day"}},
		{"PYM", []string{"year", "month"}},
		{"PTHMS", []string{"hour", "minute", "second"}},
		{"", []string{"year", "month", "day", "week", "hour", "minute", "second"}},
	}
	for _, tc := range cases {
		if got := durationFields(tc.pattern); !slices.Equal(got, tc.want) {
			t.Errorf("durationFields(%q) = %v, want %v", tc.pattern, got, tc.want)
		}
	}
}
