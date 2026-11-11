package simplified_test

// REQ-170 — a FLAT key that is not well formed is refused in every mode, never
// set aside as unmatched content: a key is a path of "/"-separated segments,
// each a non-empty id with an optional ":<index>", then at most one
// "|<suffix>". A last bare "|" spells the empty suffix, so `x|` is `x` and is
// well formed.

import (
	"errors"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/serialize/simplified"
)

// malformedKeys are keys that break the FLAT key grammar, the plan's `a//b|`
// first.
var malformedKeys = []string{
	"a//b|",
	"",
	"|code",
	"/" + eventAt + "/dv_quantity|magnitude",
	eventAt + "//dv_quantity|magnitude",
	eventAt + "/dv_quantity/",
	eventAt + "/:0|magnitude",
	eventAt + "/dv_quantity|magnitude|unit",
	eventAt + "/dv_quantity|magnitude/x",
	eventAt + "/dv_quantity||",
	"ctx//language",
	"ctx/",
}

// TestREQ170MalformedKeyRefused — REQ-170. Each malformed key is refused by
// default, under the report option and under strict, with ErrUnknownPath,
// naming the key and none of its value; the report receives nothing.
func TestREQ170MalformedKeyRefused(t *testing.T) {
	wt, _ := conformanceTarget(t)
	const secret = "secret-value-0451"
	for _, key := range malformedKeys {
		t.Run(key, func(t *testing.T) {
			body := mustJSON(t, eventBody(map[string]any{key: secret}))
			var reported []string
			for name, opts := range map[string][]simplified.Option{
				"default":       nil,
				"WithUnmatched": {simplified.WithUnmatched(func(p string) { reported = append(reported, p) })},
				"WithStrict":    {simplified.WithStrict()},
			} {
				_, err := simplified.UnmarshalFlat(body, wt, opts...)
				if !errors.Is(err, simplified.ErrUnknownPath) || errors.Is(err, simplified.ErrNotModelled) {
					t.Fatalf("UnmarshalFlat (%s) = %v, want ErrUnknownPath", name, err)
				}
				if !strings.Contains(err.Error(), `"`+key+`"`) || !strings.Contains(err.Error(), "not a well-formed FLAT key") {
					t.Errorf("UnmarshalFlat (%s) = %q, want it to name %q as not a well-formed FLAT key", name, err, key)
				}
				if strings.Contains(err.Error(), secret) {
					t.Errorf("UnmarshalFlat (%s) = %q carries the value", name, err)
				}
			}
			if len(reported) != 0 {
				t.Errorf("WithUnmatched reported %q, want nothing on a refused decode", reported)
			}
		})
	}
}

// TestREQ170MalformedKeyRefusedByConversion — REQ-170. OPT-free FLAT →
// STRUCTURED conversion refuses the same keys with ErrUnknownPath rather than
// nest them as members a decode would set aside.
func TestREQ170MalformedKeyRefusedByConversion(t *testing.T) {
	for _, key := range malformedKeys {
		_, err := simplified.FlatToStructured(mustJSON(t, map[string]any{key: "v"}))
		if !errors.Is(err, simplified.ErrUnknownPath) || !strings.Contains(err.Error(), `"`+key+`"`) {
			t.Errorf("FlatToStructured(%q) = %v, want ErrUnknownPath naming the key", key, err)
		}
	}
}

// TestREQ170WellFormedKeysStayRead — REQ-170. The check refuses nothing the
// grammar allows: a key ending in a bare `|`, an explicit `:0` on every
// segment, and a ctx/ field with a suffix decode as before.
func TestREQ170WellFormedKeysStayRead(t *testing.T) {
	wt, _ := conformanceTarget(t)
	body := eventBody(map[string]any{
		"conformance-ehrbase.de.v0:0/conformance_section:0/conformance_observation:0/any_event:0/dv_text|": "text",
		"ctx/setting|code":  "238",
		"ctx/setting|value": "other care",
	})
	if _, err := simplified.UnmarshalFlat(mustJSON(t, body), wt, simplified.WithStrict()); err != nil {
		t.Errorf("UnmarshalFlat of well-formed keys: %v", err)
	}
}
