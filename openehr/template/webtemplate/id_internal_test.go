package webtemplate

import (
	"slices"
	"strings"
	"testing"
	"unicode"
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

// sanitizeIDReference is sanitizeID as it stood before it lowercased rune by
// rune: it lowercases the whole string with strings.ToLower first and reads
// the first rune of the result through a []rune conversion. Web Template ids
// are locked to the reference implementation's rules, so sanitizeID must
// give the same id as this copy for every input.
func sanitizeIDReference(s string) string {
	var b strings.Builder
	prevUnderscore := false
	for _, r := range strings.ToLower(s) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r) || r == '.' || r == '-':
			b.WriteRune(r)
			prevUnderscore = false
		case r == '_':
			if !prevUnderscore {
				b.WriteByte('_')
				prevUnderscore = true
			}
		default:
			if b.Len() > 0 && !prevUnderscore {
				b.WriteByte('_')
				prevUnderscore = true
			}
		}
	}
	out := strings.Trim(b.String(), "_")
	if out != "" {
		if r := []rune(out)[0]; unicode.IsDigit(r) {
			out = "a" + out
		}
	}
	return out
}

// sanitizeIDInputs are the inputs on which sanitizeID must agree with
// sanitizeIDReference, and the seed corpus of FuzzSanitizeID. They reach
// every kind of input the lowercasing meets: ASCII, runes with no case,
// runes whose lowercase form has a different UTF-8 length or lies in
// another Unicode block, and bytes that are not valid UTF-8.
var sanitizeIDInputs = []string{
	"",
	"Blood pressure",
	"UPPER lower MiXeD",
	"abc123",
	"123abc",
	"24 hour average",
	"_24 hour",       // the digit is first only after the trim
	"  __Leading__ ", // underscores and spaces around
	"a.b-c_d",
	"a__b  c..d--e",
	"?!*()[]{}<>/\\|@#$%^&+=~`'\"",
	"Anzeichens.",
	"Körpertemperatur",
	"ÉCOLE Ñandú Ærø",
	"ΟΔΥΣΣΕΥΣ",                // Greek capitals ending in sigma: no final-sigma form
	"Σίσυφος ΚΑΙ ΣΑΣ",         // final sigma inside and at the end of words
	"\u0130stanbul",           // Turkish dotted capital I (two bytes, lowercases to one)
	"D\u0130YARBAKIR \u0131i", // dotted and dotless i together
	"I\u0307",                 // I followed by a combining dot above
	"STRAßE",                  // German sharp s has no single-rune uppercase
	"\u1e9e",                  // capital sharp s lowercases to ß
	"\u023a \u023e \u2c7e",    // capitals whose lowercase forms take one byte more, and one less
	"\u212a \u212b",           // Kelvin and Angstrom signs lowercase to ASCII k and to å
	"\u01c4\u01c5\u01c6",      // a Latin digraph in its upper, title and lower forms
	"血压测量",                    // CJK, no case
	"日本語テキスト",                 // Japanese, no case
	"١٢٣ arabic-indic",        // Arabic-Indic digits are digits, so the id gets an "a"
	"１２３",                     // fullwidth digits
	"①②③",                     // circled numbers are not digits
	"😀 smile",
	"a😀b👍🏽c",
	"\u00ad soft hyphen \u200b zero width",
	"\xff\xfeinvalid",   // bytes that are not valid UTF-8
	"ok\xc3",            // a truncated two-byte sequence at the end
	"\xc3\x28 \xe2\x82", // invalid continuation bytes
	"\ufffd literal replacement",
}

// sanitizeID lowercases rune by rune; it must give the id the earlier
// whole-string version gave on every input, because Web Template ids are
// locked to the reference implementation's rules.
func TestSanitizeIDMatchesReference(t *testing.T) {
	for _, in := range sanitizeIDInputs {
		if got, want := sanitizeID(in), sanitizeIDReference(in); got != want {
			t.Errorf("sanitizeID(%q) = %q, earlier version gave %q", in, got, want)
		}
	}
}

// FuzzSanitizeID checks on arbitrary input that sanitizeID gives the id the
// earlier whole-string version gave.
func FuzzSanitizeID(f *testing.F) {
	for _, in := range sanitizeIDInputs {
		f.Add(in)
	}
	f.Fuzz(func(t *testing.T, in string) {
		if got, want := sanitizeID(in), sanitizeIDReference(in); got != want {
			t.Errorf("sanitizeID(%q) = %q, earlier version gave %q", in, got, want)
		}
	})
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
