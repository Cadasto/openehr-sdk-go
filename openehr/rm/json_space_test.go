package rm

import (
	"errors"
	"testing"
)

// TestUnmarshalJSONTrimsJSONWhitespace pins that the hand-written primitive
// decoders read a value a direct caller passes with the whitespace RFC 8259
// allows around it as they read the bare value (REQ-052): a padded quoted
// literal takes the quoted arm, a padded lossy Real is still refused, and a
// padded null is still a no-op. encoding/json and canjson pass the value
// trimmed, so only a direct caller reaches this.
func TestUnmarshalJSONTrimsJSONWhitespace(t *testing.T) {
	t.Run("Real lossy", func(t *testing.T) {
		for _, in := range []string{" 9007199254740993", "9007199254740993\n", "\t\"9007199254740993\" ", " 0.1234567890123456789 "} {
			r := Real(80.5)
			if err := r.UnmarshalJSON([]byte(in)); !errors.Is(err, errLossyLiteral) {
				t.Errorf("Real.UnmarshalJSON(%q) err = %v, r = %v; want the lossy-literal refusal", in, err, float64(r))
			}
			if r != 80.5 {
				t.Errorf("Real.UnmarshalJSON(%q) wrote %v; want the receiver unchanged", in, float64(r))
			}
		}
	})
	t.Run("Real decodes", func(t *testing.T) {
		for _, in := range []string{" 72.5", "\r\n\"72.5\"\t"} {
			var r Real
			if err := r.UnmarshalJSON([]byte(in)); err != nil || r != 72.5 {
				t.Errorf("Real.UnmarshalJSON(%q) = %v, r = %v; want 72.5", in, err, float64(r))
			}
		}
	})
	t.Run("null", func(t *testing.T) {
		r, c := Real(80.5), Character("a")
		if err := r.UnmarshalJSON([]byte(" null\n")); err != nil || r != 80.5 {
			t.Errorf("Real.UnmarshalJSON(padded null) = %v, r = %v; want a no-op", err, float64(r))
		}
		if err := c.UnmarshalJSON([]byte("\tnull ")); err != nil || c != "a" {
			t.Errorf("Character.UnmarshalJSON(padded null) = %v, c = %q; want a no-op", err, string(c))
		}
	})
	t.Run("Integer", func(t *testing.T) {
		for _, in := range []string{"\t\"2\"", " 2.0 "} {
			var i Integer
			if err := i.UnmarshalJSON([]byte(in)); err != nil || i != 2 {
				t.Errorf("Integer.UnmarshalJSON(%q) = %v, i = %d; want 2", in, err, i)
			}
		}
	})
	t.Run("Integer64", func(t *testing.T) {
		for _, in := range []string{" \"2\" ", "\n2e0"} {
			var i Integer64
			if err := i.UnmarshalJSON([]byte(in)); err != nil || i != 2 {
				t.Errorf("Integer64.UnmarshalJSON(%q) = %v, i = %d; want 2", in, err, i)
			}
		}
	})
	t.Run("Character", func(t *testing.T) {
		for _, in := range []string{" \"a\"", "\n\"a\"\t"} {
			var c Character
			if err := c.UnmarshalJSON([]byte(in)); err != nil || c != "a" {
				t.Errorf("Character.UnmarshalJSON(%q) = %v, c = %q; want \"a\"", in, err, string(c))
			}
		}
	})
	t.Run("whitespace only", func(t *testing.T) {
		var r Real
		if err := r.UnmarshalJSON([]byte(" \t")); err == nil {
			t.Errorf("Real.UnmarshalJSON(whitespace only) = nil; want the empty-input refusal")
		}
	})
}
