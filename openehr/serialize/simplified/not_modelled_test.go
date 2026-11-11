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

// TestREQ170NotModelledKeys — REQ-170. The reference's in-context spelling of
// an ACTION's ISM transition and time, and the deferred family
// `_instruction_details`, are refused in FLAT and in STRUCTURED, by default
// and under strict, with ErrNotModelled and ErrUnknownPath, the key named and
// its value not.
func TestREQ170NotModelledKeys(t *testing.T) {
	wt, _ := conformanceTarget(t)
	const secret = "secret-value-0451"
	for _, key := range []string{
		notModelledAction + "/ism_transition/current_state|code",
		notModelledAction + "/ism_transition/transition|code",
		notModelledAction + "/ism_transition/careflow_step|code",
		notModelledAction + "/transition/transition|code",
		notModelledAction + "/time",
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
