package instance_test

import (
	"errors"
	"strconv"
	"strings"
	"testing"
	"unicode"

	"github.com/cadasto/openehr-sdk-go/openehr/instance"
)

// mixedCase returns s with its letters upper and lower case in turn,
// starting with upper: "medium" becomes "MeDiUm".
func mixedCase(s string) string {
	var b strings.Builder
	for i, r := range s {
		if i%2 == 0 {
			r = unicode.ToUpper(r)
		}
		b.WriteRune(r)
	}
	return b.String()
}

// TestREQ107_ParsePolicy is the REQ-107 check of Levels and names for
// ParsePolicy: each of the names required, minimal, medium, large and
// complete gives its policy whatever its case, minimal giving Required and
// complete Large, and any other text, the empty string and a name with a
// space before it included, returns an error wrapping ErrUnsupportedPolicy
// that quotes the text.
func TestREQ107_ParsePolicy(t *testing.T) {
	names := []struct {
		name string
		want instance.Policy
	}{
		{name: "required", want: instance.Required},
		{name: "minimal", want: instance.Required},
		{name: "medium", want: instance.Medium},
		{name: "large", want: instance.Large},
		{name: "complete", want: instance.Large},
	}
	for _, n := range names {
		for _, text := range []string{n.name, strings.ToUpper(n.name), mixedCase(n.name)} {
			t.Run(strconv.Quote(text), func(t *testing.T) {
				got, err := instance.ParsePolicy(text)
				if err != nil || got != n.want {
					t.Errorf("ParsePolicy(%q) = %v, %v; want %v, nil", text, got, err, n.want)
				}
			})
		}
	}
	for _, text := range []string{"example", "", " medium", "big"} {
		t.Run(strconv.Quote(text), func(t *testing.T) {
			got, err := instance.ParsePolicy(text)
			if !errors.Is(err, instance.ErrUnsupportedPolicy) {
				t.Fatalf("ParsePolicy(%q) = %v, %v; want an error wrapping ErrUnsupportedPolicy", text, got, err)
			}
			if quoted := strconv.Quote(text); !strings.Contains(err.Error(), quoted) {
				t.Errorf("ParsePolicy(%q) error = %q, want it to quote the text as %s", text, err, quoted)
			}
		})
	}
}

// TestREQ107_PolicyAliases is the REQ-107 check of Levels and names for
// the aliases: Minimal is Required and Complete is Large, the same values,
// so each prints the name of the level it stands for.
func TestREQ107_PolicyAliases(t *testing.T) {
	cases := []struct {
		alias, level instance.Policy
		name         string
	}{
		{alias: instance.Minimal, level: instance.Required, name: "required"},
		{alias: instance.Complete, level: instance.Large, name: "large"},
	}
	for _, tc := range cases {
		if tc.alias != tc.level {
			t.Errorf("alias of %s = Policy(%d), want Policy(%d)", tc.name, int(tc.alias), int(tc.level))
		}
		if got := tc.alias.String(); got != tc.name {
			t.Errorf("alias of %s: String() = %q, want %q", tc.name, got, tc.name)
		}
	}
}
