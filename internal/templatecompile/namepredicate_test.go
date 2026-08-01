package templatecompile_test

import (
	"testing"

	"github.com/cadasto/openehr-sdk-go/internal/templatecompile"
)

// REQ-116 — NamePredicate is the single quoting rule both path builders
// share, so its escape behaviour is pinned here once. The backslash cases
// matter even though no vendored name carries one: unescaped, a name ending
// in `\` would emit `,'…\'` whose closing quote every `\'`-honouring
// scanner consumes as escaped, swallowing the rest of the path.
func TestNamePredicate(t *testing.T) {
	tests := []struct {
		name string
		id   string
		in   string
		want string
	}{
		{"empty name keeps bare id", "at0001", "", "at0001"},
		{"plain name", "at0001", "Symptome", "at0001,'Symptome'"},
		{"comma is literal", "at0001", "Kontakt, dort", "at0001,'Kontakt, dort'"},
		// The corona oracle really pins a slash-carrying name (its
		// "Problem/…" node, upstream typo included) — corpus-real.
		{"slash is literal", "at0001", "Problem/Diganose", "at0001,'Problem/Diganose'"}, //nolint:misspell // upstream fixture's own spelling
		{"quote escaped", "at0001", "O'Brien", `at0001,'O\'Brien'`},
		{"backslash escaped", "at0001", `a\b`, `at0001,'a\\b'`},
		{"trailing backslash does not eat the closing quote", "at0001", `a\`, `at0001,'a\\'`},
		{"literal backslash-quote pair", "at0001", `a\'b`, `at0001,'a\\\'b'`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := templatecompile.NamePredicate(tc.id, tc.in); got != tc.want {
				t.Errorf("NamePredicate(%q, %q) = %q, want %q", tc.id, tc.in, got, tc.want)
			}
		})
	}
}
