package aql

import (
	"testing"
)

// triviaSamples are predicate texts on both sides of the trivia-free check:
// each byte that can start trivia, a lone dash, and the regions the walk
// copies verbatim.
var triviaSamples = []string{
	"", "at0001", "at0001,'x'", "at0001, 'x'", " at0001", "at0001 ", "at0001\t", "a\r\nb",
	"openEHR-EHR-OBSERVATION.bp.v1", "a--b", "a-- note\nb", "a--", "'a--b'", "'a b'",
	"\uFEFFat0001", "at\uEFBB" + "BF0001", "at\x00" + "FEFF0001", "{/a b/}", "{/a/}", "{", "x::SNOMED|a b|",
	"at0001,\"--\"", "-", "--\n", "at0001-", "\xef", "\xee", "\x00",
}

// TestStripPredicateTriviaFastPathAgreesWithTheWalk pins that the
// trivia-free check only skips texts the walk would return unchanged, and
// that a trivia-free text is handed back without building a new string.
func TestStripPredicateTriviaFastPathAgreesWithTheWalk(t *testing.T) {
	for _, s := range triviaSamples {
		if got, want := StripPredicateTrivia(s), stripPredicateTrivia(s); got != want {
			t.Errorf("StripPredicateTrivia(%q) = %q, the walk gives %q", s, got, want)
		}
		if triviaFree(s) && stripPredicateTrivia(s) != s {
			t.Errorf("triviaFree(%q) but the walk changes it to %q", s, stripPredicateTrivia(s))
		}
	}
	if n := testing.AllocsPerRun(100, func() { _ = StripPredicateTrivia("openEHR-EHR-OBSERVATION.bp.v1") }); n != 0 {
		t.Errorf("StripPredicateTrivia of a trivia-free text allocates %v times, want 0", n)
	}
}

// FuzzStripPredicateTriviaFastPath holds the same agreement over generated
// texts.
func FuzzStripPredicateTriviaFastPath(f *testing.F) {
	for _, s := range triviaSamples {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if got, want := StripPredicateTrivia(s), stripPredicateTrivia(s); got != want {
			t.Errorf("StripPredicateTrivia(%q) = %q, the walk gives %q", s, got, want)
		}
	})
}
