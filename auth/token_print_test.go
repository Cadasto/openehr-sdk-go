package auth_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/cadasto/openehr-sdk-go/auth"
)

// redactedMarker is the text a credential prints and logs as.
const redactedMarker = "[redacted]"

// urlPassword is the password the tests put in a URL, and urlMask the text
// url.URL.Redacted writes in its place.
const (
	urlPassword = "urlpass-s3cr3t"
	urlMask     = "xxxxx"
)

// printVerbs are the fmt verbs, with flags and widths, that must all print the
// text of a value's String method.
var printVerbs = []string{"%v", "%+v", "%#v", "%s", "%q", "%d", "%x", "%X", "%t", "%20v", "%-20s", "%+q", "%#x"}

// printProblems prints v through String and GoString, with every verb of
// printVerbs, nested in a struct and a slice, and through the text and JSON
// handlers of log/slog. It returns one line for each output that shows one of
// secrets, for a String text that lacks one of shown, and for a verb whose
// output differs from the String text. When v implements slog.LogValuer, the
// slog output must also carry the marker.
func printProblems(v any, secrets, shown []string) []string {
	var problems []string
	leaks := func(where, out string) {
		for _, s := range secrets {
			if strings.Contains(out, s) {
				problems = append(problems, fmt.Sprintf("%s shows the credential %q: %s", where, s, out))
			}
		}
	}
	stringer, ok := v.(fmt.Stringer)
	if !ok {
		problems = append(problems, fmt.Sprintf("%T does not implement fmt.Stringer", v))
	}
	if _, ok := v.(fmt.Formatter); !ok {
		problems = append(problems, fmt.Sprintf("%T does not implement fmt.Formatter", v))
	}
	var want string
	if stringer != nil {
		want = stringer.String()
		leaks("String()", want)
		for _, s := range shown {
			if !strings.Contains(want, s) {
				problems = append(problems, fmt.Sprintf("String() = %q, want it to show %q", want, s))
			}
		}
	}
	if g, ok := v.(fmt.GoStringer); !ok {
		problems = append(problems, fmt.Sprintf("%T does not implement fmt.GoStringer", v))
	} else if got := g.GoString(); got != want {
		problems = append(problems, fmt.Sprintf("GoString() = %q, want %q, the text of String()", got, want))
	}
	for _, verb := range printVerbs {
		out := fmt.Sprintf(verb, v)
		leaks(fmt.Sprintf("fmt.Sprintf(%q)", verb), out)
		if stringer != nil && out != want {
			problems = append(problems, fmt.Sprintf("fmt.Sprintf(%q) = %q, want %q, the text of String()", verb, out, want))
		}
	}
	leaks("a struct field under %+v", fmt.Sprintf("%+v", struct{ V any }{v}))
	leaks("a slice element under %#v", fmt.Sprintf("%#v", []any{v}))

	_, logValuer := v.(slog.LogValuer)
	for name, newHandler := range map[string]func(*bytes.Buffer) slog.Handler{
		"slog JSONHandler": func(b *bytes.Buffer) slog.Handler { return slog.NewJSONHandler(b, nil) },
		"slog TextHandler": func(b *bytes.Buffer) slog.Handler { return slog.NewTextHandler(b, nil) },
	} {
		var buf bytes.Buffer
		slog.New(newHandler(&buf)).Info("printed", "v", v)
		leaks(name, buf.String())
		if logValuer && !strings.Contains(buf.String(), redactedMarker) {
			problems = append(problems, fmt.Sprintf("%s wrote %q, want it to carry %s", name, buf.String(), redactedMarker))
		}
	}
	return problems
}

// TestREQ060_TokenPrintsNoCredential pins REQ-060: an auth.Token printed with
// any fmt verb, or logged through log/slog, shows its Value as [redacted] and
// the password of its issuer URL masked, keeps its other fields visible, and
// shows an empty Value as empty and an issuer that is not a URL as written.
func TestREQ060_TokenPrintsNoCredential(t *testing.T) { // REQ-060
	t.Parallel()
	const secret = "TOKEN-VALUE-s3cr3t"
	tok := auth.Token{
		Value:     secret,
		Type:      auth.TokenTypeBearer,
		ExpiresAt: time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC),
		Scope:     "openid launch/patient",
		Issuer:    "https://idp.example",
	}
	shown := []string{redactedMarker, "Bearer", "2026-10-27T12:00:00Z", "openid launch/patient", "https://idp.example"}
	tests := []struct {
		name string
		v    any
	}{
		{name: "auth.Token", v: tok},
		{name: "*auth.Token", v: &tok},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if _, ok := tc.v.(slog.LogValuer); !ok {
				t.Errorf("%T does not implement slog.LogValuer", tc.v)
			}
			for _, p := range printProblems(tc.v, []string{secret}, shown) {
				t.Error(p)
			}
		})
	}

	t.Run("an exported field of another struct", func(t *testing.T) {
		t.Parallel()
		holder := struct{ Token auth.Token }{tok}
		for _, verb := range printVerbs {
			out := fmt.Sprintf(verb, holder)
			if strings.Contains(out, secret) {
				t.Errorf("fmt.Sprintf(%q, holder) shows the token: %s", verb, out)
			}
		}
		if out := fmt.Sprintf("%v", holder); !strings.Contains(out, redactedMarker) {
			t.Errorf("fmt.Sprintf(%%v, holder) = %q, want it to carry %s", out, redactedMarker)
		}
	})

	t.Run("an empty Value prints as empty", func(t *testing.T) {
		t.Parallel()
		empty := auth.Token{Type: auth.TokenTypeBasic}
		if out := fmt.Sprint(empty); strings.Contains(out, redactedMarker) || !strings.Contains(out, `Value: ""`) {
			t.Errorf("fmt.Sprint(%s) = %q, want an empty Value and no %s", "auth.Token{Type: Basic}", out, redactedMarker)
		}
	})

	t.Run("a password in the issuer URL is masked", func(t *testing.T) {
		t.Parallel()
		withPassword := tok
		withPassword.Issuer = "https://iss:" + urlPassword + "@idp.example"
		for _, v := range []any{withPassword, &withPassword} {
			for _, p := range printProblems(v, []string{secret, urlPassword}, []string{redactedMarker, "https://iss:" + urlMask + "@idp.example"}) {
				t.Errorf("%T: %s", v, p)
			}
		}
	})

	t.Run("an issuer that is not a URL prints as written", func(t *testing.T) {
		t.Parallel()
		plain := tok
		plain.Issuer = "issuer name"
		for _, p := range printProblems(plain, []string{secret}, []string{`Issuer: "issuer name"`}) {
			t.Error(p)
		}
	})
}

// plainToken has the fields of auth.Token and none of its methods.
type plainToken auth.Token

// TestREQ060_TokenJSONUnchanged pins REQ-060: the printing methods of
// auth.Token leave its JSON encoding as it is, Value included, so an
// application can persist a token and read it back.
func TestREQ060_TokenJSONUnchanged(t *testing.T) { // REQ-060
	t.Parallel()
	tok := auth.Token{
		Value:     "TOKEN-VALUE-s3cr3t",
		Type:      auth.TokenTypeBearer,
		ExpiresAt: time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC),
		Scope:     "openid",
		Issuer:    "https://idp.example",
	}
	got, err := json.Marshal(tok)
	if err != nil {
		t.Fatalf("json.Marshal(tok): %v", err)
	}
	want, err := json.Marshal(plainToken(tok))
	if err != nil {
		t.Fatalf("json.Marshal(plainToken(tok)): %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("json.Marshal(tok) = %s, want %s, the encoding of its fields", got, want)
	}
	var back auth.Token
	if err := json.Unmarshal(got, &back); err != nil {
		t.Fatalf("json.Unmarshal(%s): %v", got, err)
	}
	if !reflect.DeepEqual(back, tok) {
		t.Errorf("json round trip = %#v, want %#v", plainToken(back), plainToken(tok))
	}
}

// TestREQ060_StaticTokenSourcePrintsNoCredential pins REQ-060: the token
// source auth.StaticTokenSource returns holds its token in an unexported
// field, which fmt prints without calling the token's methods, so the source
// prints the token's String text itself, with any fmt verb, the password of
// the token's issuer URL masked.
func TestREQ060_StaticTokenSourcePrintsNoCredential(t *testing.T) { // REQ-060
	t.Parallel()
	const secret = "STATIC-TOKEN-s3cr3t"
	ts := auth.StaticTokenSource(auth.Token{Value: secret, Type: auth.TokenTypeBearer})
	for _, p := range printProblems(ts, []string{secret}, []string{redactedMarker, "Bearer"}) {
		t.Error(p)
	}

	t.Run("a password in the issuer URL is masked", func(t *testing.T) {
		withPassword := auth.StaticTokenSource(auth.Token{Value: secret, Type: auth.TokenTypeBearer, Issuer: "https://iss:" + urlPassword + "@idp.example"})
		for _, p := range printProblems(withPassword, []string{secret, urlPassword}, []string{redactedMarker, "https://iss:" + urlMask + "@idp.example"}) {
			t.Error(p)
		}
	})
}
