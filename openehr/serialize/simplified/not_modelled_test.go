package simplified_test

// REQ-170 — issue 428. A key the Simplified Formats define and this SDK reads
// but does not model is refused in every mode with an error that matches
// simplified.ErrNotModelled, and simplified.ErrUnknownPath as before, so a
// caller can tell it from a key no template has. Its text names the key, says
// this SDK does not model it, and points to no file a client cannot see.

import (
	"errors"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/serialize/simplified"
	"github.com/cadasto/openehr-sdk-go/openehr/template/webtemplate"
)

// notModelledAction is the PROBE-086 corpus template's ACTION.
const notModelledAction = "conformance-ehrbase.de.v0/conformance_section/conformance_action"

// TestREQ170NotModelledKeys — REQ-170. The reference's spelling of a
// `transition` under an archetyped per-step ISM transition node, which the Web
// Template does not model there, and the deferred family
// `_instruction_details`, are refused in FLAT and in STRUCTURED, by default
// and under strict, with ErrNotModelled and ErrUnknownPath, the key named and
// its value not.
func TestREQ170NotModelledKeys(t *testing.T) {
	wt, _ := conformanceTarget(t)
	const secret = "secret-value-0451"
	for _, key := range []string{
		notModelledAction + "/transition/transition|code",
		notModelledAction + "/_instruction_details|activity_id",
	} {
		body := mustJSON(t, eventBody(map[string]any{key: secret}))
		for _, format := range []string{"FLAT", "STRUCTURED"} {
			for name, opts := range map[string][]simplified.Option{"default": nil, "strict": {simplified.WithStrict()}} {
				t.Run(format+"/"+name+"/"+key, func(t *testing.T) {
					err := decodeErr(t, format, body, wt, opts...)
					if !errors.Is(err, simplified.ErrNotModelled) || !errors.Is(err, simplified.ErrUnknownPath) {
						t.Fatalf("err = %v, want ErrNotModelled and ErrUnknownPath", err)
					}
					msg := err.Error()
					// FLAT names the key as written, or the family instance;
					// STRUCTURED names its FLAT form, an index on every segment.
					named := strings.Contains(msg, `"`+key+`"`) || strings.Contains(msg, `"`+strings.Split(key, "|")[0])
					if format == "STRUCTURED" {
						named = strings.Contains(msg, `"conformance-ehrbase.de.v0/conformance_section:0/conformance_action:0/`)
					}
					if !named || !strings.Contains(msg, "does not model") {
						t.Errorf("err = %q, want it to name the key and say this SDK does not model it", msg)
					}
					for _, banned := range []string{secret, "path not in web template", "deviations.md", "REQ-"} {
						if strings.Contains(msg, banned) {
							t.Errorf("err = %q carries %q", msg, banned)
						}
					}
				})
			}
		}
	}
}

// TestREQ170InContextActionKeysAreRead — REQ-170, REQ-053. The reference's
// in-context spelling of an ACTION's ISM transition, `_reason` family
// included, and its own `time` resolve in the Web Template, so they decode in
// FLAT and in STRUCTURED, by default and under strict: none is refused as a
// key this SDK does not model.
func TestREQ170InContextActionKeysAreRead(t *testing.T) {
	wt, _ := conformanceTarget(t)
	ism := notModelledAction + "/ism_transition"
	body := mustJSON(t, eventBody(map[string]any{
		ism + "/current_state|code":        "532",
		ism + "/current_state|value":       "completed",
		ism + "/current_state|terminology": "openehr",
		ism + "/transition|code":           "548",
		ism + "/transition|value":          "finish",
		ism + "/transition|terminology":    "openehr",
		ism + "/careflow_step|code":        "at0006",
		ism + "/careflow_step|value":       "transition",
		ism + "/careflow_step|terminology": "local",
		ism + "/_reason:0":                 "reason 1",
		notModelledAction + "/time":        "2022-01-31T10:33:28.72414+01:00",
	}))
	structured, err := simplified.FlatToStructured(body)
	if err != nil {
		t.Fatalf("FlatToStructured: %v", err)
	}
	for format, decode := range map[string]func() error{
		"FLAT": func() error { _, err := simplified.UnmarshalFlat(body, wt, simplified.WithStrict()); return err },
		"STRUCTURED": func() error {
			_, err := simplified.UnmarshalStructured(structured, wt, simplified.WithStrict())
			return err
		},
	} {
		if err := decode(); err != nil {
			t.Errorf("Unmarshal %s (strict) = %v, want the in-context keys read", format, err)
		}
	}
}

// TestREQ170UnmatchedIsNotNotModelled — REQ-170. A mistyped key is unmatched
// content: strict refuses it with ErrUnknownPath, not ErrNotModelled, so the
// two refusals are told apart by sentinel; the sentinels are distinct.
func TestREQ170UnmatchedIsNotNotModelled(t *testing.T) {
	wt, _ := conformanceTarget(t)
	body := mustJSON(t, eventBody(map[string]any{eventAt + "|sample_cnt": 5}))
	_, err := simplified.UnmarshalFlat(body, wt, simplified.WithStrict())
	if !errors.Is(err, simplified.ErrUnknownPath) || errors.Is(err, simplified.ErrNotModelled) {
		t.Errorf("strict refusal of a mistyped key = %v, want ErrUnknownPath and not ErrNotModelled", err)
	}
	if errors.Is(simplified.ErrUnknownPath, simplified.ErrNotModelled) {
		t.Error("ErrUnknownPath matches ErrNotModelled; want the sentinels distinct")
	}
	if simplified.ErrNotModelled.Error() == simplified.ErrUnknownPath.Error() {
		t.Errorf("ErrNotModelled reads %q, the text of ErrUnknownPath", simplified.ErrNotModelled)
	}
}

// decodeErr decodes the FLAT body flat in format and returns the error,
// failing the test when the decode succeeds.
func decodeErr(t *testing.T, format string, flat []byte, wt *webtemplate.WebTemplate, opts ...simplified.Option) error {
	t.Helper()
	body, decode := flat, simplified.UnmarshalFlat
	if format == "STRUCTURED" {
		var err error
		if body, err = simplified.FlatToStructured(flat); err != nil {
			t.Fatalf("FlatToStructured: %v", err)
		}
		decode = simplified.UnmarshalStructured
	}
	_, err := decode(body, wt, opts...)
	if err == nil {
		t.Fatalf("Unmarshal %s succeeded, want a refusal", format)
	}
	return err
}
