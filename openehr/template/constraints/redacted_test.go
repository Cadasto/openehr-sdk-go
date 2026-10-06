package constraints_test

// redacted_test.go: REQ-168 § The redacting carrier. A Redacted holds one
// submitted value; no fmt verb, JSON encoder or slog handler writes it, and
// Reveal returns it unchanged.

import (
	"bytes"
	"encoding/hex"
	jsonv1 "encoding/json"
	jsonv2 "encoding/json/v2"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/template/constraints"
)

// printVerbs are the fmt verbs a caller might print a diagnostic with.
var printVerbs = []string{"%v", "%+v", "%#v", "%s", "%d", "%q", "%x", "%X"}

// markedViolation returns the violation CString reports for markerString,
// whose Value holds the marker.
func markedViolation(t *testing.T) constraints.Violation {
	t.Helper()
	c := constraints.CString{List: []string{"alpha", "beta"}}
	got := c.Validate(markerString)
	if len(got) != 1 {
		t.Fatalf("CString.Validate(%q) = %d violation(s), want 1", markerString, len(got))
	}
	if reveal := got[0].Value.Reveal(); reveal != markerString {
		t.Fatalf("CString.Validate(%q) Value.Reveal() = %#v, want %q", markerString, reveal, markerString)
	}
	return got[0]
}

// leaksMarker reports whether out contains markerString as written, quoted, or
// hex-encoded in either case.
func leaksMarker(out string) bool {
	h := hex.EncodeToString([]byte(markerString))
	for _, form := range []string{markerString, h, strings.ToUpper(h)} {
		if strings.Contains(out, form) {
			return true
		}
	}
	return false
}

func TestREQ168_RedactedPrintsNoValue(t *testing.T) {
	t.Parallel()
	v := markedViolation(t)
	r := constraints.Redact(markerString)
	subjects := []struct {
		name  string
		value any
	}{
		{name: "Violation", value: v},
		{name: "*Violation", value: &v},
		{name: "[]Violation", value: []constraints.Violation{v}},
		{name: "Redacted", value: r},
		{name: "*Redacted", value: &r},
	}
	for _, s := range subjects {
		for _, verb := range printVerbs {
			t.Run(s.name+" "+verb, func(t *testing.T) {
				t.Parallel()
				out := fmt.Sprintf(verb, s.value)
				if leaksMarker(out) {
					t.Errorf("fmt.Sprintf(%q, %s) = %q, which contains the submitted value", verb, s.name, out)
				}
				if !strings.Contains(out, "[redacted]") {
					t.Errorf("fmt.Sprintf(%q, %s) = %q, want it to contain [redacted]", verb, s.name, out)
				}
			})
		}
	}
	if got := r.String(); got != "[redacted]" {
		t.Errorf("Redact(%q).String() = %q, want %q", markerString, got, "[redacted]")
	}
}

func TestREQ168_ZeroRedactedHoldsNoValue(t *testing.T) {
	t.Parallel()
	var zero constraints.Redacted
	if reveal := zero.Reveal(); reveal != nil {
		t.Errorf("Redacted{}.Reveal() = %#v, want nil", reveal)
	}
	if s := zero.String(); s != "" {
		t.Errorf("Redacted{}.String() = %q, want \"\"", s)
	}
	for _, verb := range printVerbs {
		if out := fmt.Sprintf(verb, zero); out != "" {
			t.Errorf("fmt.Sprintf(%q, Redacted{}) = %q, want \"\"", verb, out)
		}
	}
	if constraints.Redact(nil) != zero {
		t.Errorf("Redact(nil) = %v, want the zero Redacted", constraints.Redact(nil).Reveal())
	}
}

func TestREQ168_RedactRevealReturnsTheValueUnchanged(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		value any
	}{
		{name: "string", value: markerString},
		{name: "int", value: markerInt},
		{name: "int32", value: int32(markerInt)},
		{name: "float64", value: markerReal},
		{name: "bool", value: false},
		{name: "CodedTermRef", value: markerSymbol},
		{name: "OrdinalSymbol", value: constraints.OrdinalSymbol{Value: markerOrdinal, Symbol: markerSymbol}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := constraints.Redact(tc.value).Reveal(); got != tc.value {
				t.Errorf("Redact(%#v).Reveal() = %#v (%T), want %#v (%T)", tc.value, got, got, tc.value, tc.value)
			}
			if constraints.Redact(tc.value) != constraints.Redact(tc.value) {
				t.Errorf("Redact(%#v) == Redact(%#v) = false, want true", tc.value, tc.value)
			}
		})
	}
}

func TestREQ168_JSONLeavesTheValueOut(t *testing.T) {
	t.Parallel()
	v := markedViolation(t)
	encoders := []struct {
		name    string
		marshal func(any) ([]byte, error)
	}{
		{name: "encoding/json", marshal: jsonv1.Marshal},
		{name: "encoding/json/v2", marshal: func(in any) ([]byte, error) { return jsonv2.Marshal(in) }},
	}
	for _, enc := range encoders {
		t.Run(enc.name+" Violation", func(t *testing.T) {
			t.Parallel()
			for _, in := range []any{v, &v} {
				out, err := enc.marshal(in)
				if err != nil {
					t.Fatalf("Marshal(%T) error = %v", in, err)
				}
				if leaksMarker(string(out)) {
					t.Errorf("Marshal(%T) = %s, which contains the submitted value", in, out)
				}
				var members map[string]any
				if err := jsonv1.Unmarshal(out, &members); err != nil {
					t.Fatalf("Marshal(%T) = %s, which does not decode as an object: %v", in, out, err)
				}
				for name := range members {
					if strings.EqualFold(name, "value") {
						t.Errorf("Marshal(%T) = %s, which has a %q member, want none", in, out, name)
					}
				}
				if got := members["Code"]; got != string(constraints.CodeNotInList) {
					t.Errorf("Marshal(%T) member Code = %#v, want %q", in, got, constraints.CodeNotInList)
				}
			}
		})
		t.Run(enc.name+" Redacted", func(t *testing.T) {
			t.Parallel()
			for _, r := range []constraints.Redacted{constraints.Redact(markerString), {}} {
				out, err := enc.marshal(r)
				if err != nil {
					t.Fatalf("Marshal(Redacted) error = %v", err)
				}
				if string(out) != "null" {
					t.Errorf("Marshal(Redacted) = %s, want null", out)
				}
			}
		})
	}
}

func TestREQ168_SlogWritesNoValue(t *testing.T) {
	t.Parallel()
	v := markedViolation(t)
	handlers := []struct {
		name    string
		handler func(*bytes.Buffer) slog.Handler
	}{
		{name: "text", handler: func(b *bytes.Buffer) slog.Handler { return slog.NewTextHandler(b, nil) }},
		{name: "JSON", handler: func(b *bytes.Buffer) slog.Handler { return slog.NewJSONHandler(b, nil) }},
	}
	attrs := []struct {
		name string
		attr slog.Attr
		// want is a substring each handler's output must contain, keyed by
		// handler name, so a run that logged nothing does not pass.
		want map[string]string
	}{
		{
			name: "Violation",
			attr: slog.Any("violation", v),
			want: map[string]string{"text": "Value:[redacted]", "JSON": `"Code":"not_in_list"`},
		},
		{
			name: "*Violation",
			attr: slog.Any("violation", &v),
			want: map[string]string{"text": "Value:[redacted]", "JSON": `"Code":"not_in_list"`},
		},
		{
			name: "[]Violation",
			attr: slog.Any("violations", []constraints.Violation{v}),
			want: map[string]string{"text": "Value:[redacted]", "JSON": `"Code":"not_in_list"`},
		},
		{
			name: "Redacted",
			attr: slog.Any("value", v.Value),
			want: map[string]string{"text": "value=[redacted]", "JSON": `"value":null`},
		},
	}
	for _, h := range handlers {
		for _, a := range attrs {
			t.Run(h.name+" "+a.name, func(t *testing.T) {
				t.Parallel()
				var buf bytes.Buffer
				slog.New(h.handler(&buf)).LogAttrs(t.Context(), slog.LevelInfo, "validation failed", a.attr)
				out := buf.String()
				if leaksMarker(out) {
					t.Errorf("slog %s handler wrote %q, which contains the submitted value", h.name, out)
				}
				if want := a.want[h.name]; !strings.Contains(out, want) {
					t.Errorf("slog %s handler wrote %q, want it to contain %q", h.name, out, want)
				}
			})
		}
	}
}
