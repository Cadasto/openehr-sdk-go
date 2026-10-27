package jwtbearer_test

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/auth/jwtbearer"
)

// redactedMarker is the text a credential prints and logs as.
const redactedMarker = "[redacted]"

// urlMask is the text url.URL.Redacted writes in place of a URL's password.
const urlMask = "xxxxx"

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

// TestREQ060_PrintsNoCredential pins REQ-060: a jwtbearer.Config, a
// *jwtbearer.ClaimsSigner and a *jwtbearer.Source holding a token, printed
// with any fmt verb or logged through log/slog, show no access token, no
// private key and no password of the token URL. Config and ClaimsSigner show
// their credential as [redacted] and their other fields as they are.
func TestREQ060_PrintsNoCredential(t *testing.T) { // REQ-060
	t.Parallel()
	const (
		access      = "ACCESS-s3cr3t"
		urlPassword = "urlpass-s3cr3t"
	)
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey: %v", err)
	}
	privateD := key.D.String()
	signer, err := jwtbearer.NewClaimsSigner(jwtbearer.ClaimsTemplate{
		Issuer:   "client-id",
		Subject:  "client-id",
		Audience: "https://idp.example/token",
	}, key, jwtbearer.WithKeyID("kid-1"))
	if err != nil {
		t.Fatalf("NewClaimsSigner: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"access_token":%q,"token_type":"Bearer","expires_in":3600}`, access)
	}))
	t.Cleanup(srv.Close)
	tokenURL, err := url.Parse(srv.URL + "/token")
	if err != nil {
		t.Fatalf("url.Parse(%q): %v", srv.URL, err)
	}
	tokenURL.User = url.UserPassword("svc", urlPassword)
	src, err := jwtbearer.New(tokenURL.String(), signer,
		jwtbearer.WithHTTPClient(srv.Client()), jwtbearer.WithClientID("client-id"), jwtbearer.WithIssuer("https://idp.example"))
	if err != nil {
		t.Fatalf("jwtbearer.New: %v", err)
	}
	if tok, err := src.Token(t.Context()); err != nil || tok.Value != access {
		t.Fatalf("Token() = (%q, %v), want (%q, nil)", tok.Value, err, access)
	}

	tests := []struct {
		name    string
		v       any
		secrets []string
		shown   []string
	}{
		{
			name: "Config",
			v: jwtbearer.Config{
				TokenURL:  "https://svc:" + urlPassword + "@idp.example/token",
				Assertion: signer,
				Scope:     "system/*.read",
				ClientID:  "client-id",
				Issuer:    "https://idp.example",
			},
			secrets: []string{privateD, urlPassword},
			shown:   []string{redactedMarker, "idp.example/token", "system/*.read", "client-id", "https://idp.example"},
		},
		{
			name: "Config with a password in its issuer and audience",
			v: jwtbearer.Config{
				TokenURL:  "https://idp.example/token",
				Assertion: signer,
				Audience:  "https://aud:" + urlPassword + "@api.example",
				Issuer:    "https://iss:" + urlPassword + "@idp.example",
				ClientID:  "client-id",
			},
			secrets: []string{privateD, urlPassword},
			shown: []string{
				redactedMarker, "client-id",
				"https://aud:" + urlMask + "@api.example",
				"https://iss:" + urlMask + "@idp.example",
			},
		},
		{
			name:    "*ClaimsSigner",
			v:       signer,
			secrets: []string{privateD},
			shown:   []string{redactedMarker, "RS384", "kid-1", "client-id", "https://idp.example/token"},
		},
		{
			name:    "*Source",
			v:       src,
			secrets: []string{access, privateD, urlPassword},
			shown:   []string{tokenURL.Host, "client-id", "https://idp.example"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			for _, p := range printProblems(tc.v, tc.secrets, tc.shown) {
				t.Error(p)
			}
		})
	}

	t.Run("LogValuer", func(t *testing.T) {
		t.Parallel()
		for _, v := range []any{jwtbearer.Config{}, &jwtbearer.Config{}, signer} {
			if _, ok := v.(slog.LogValuer); !ok {
				t.Errorf("%T does not implement slog.LogValuer", v)
			}
		}
	})

	t.Run("no credential prints as empty", func(t *testing.T) {
		t.Parallel()
		if out := fmt.Sprint(jwtbearer.Config{TokenURL: "https://idp.example/token"}); strings.Contains(out, redactedMarker) || !strings.Contains(out, "Assertion: <nil>") {
			t.Errorf("fmt.Sprint(a Config without an assertion) = %q, want Assertion: <nil> and no %s", out, redactedMarker)
		}
	})

	t.Run("nil", func(t *testing.T) {
		t.Parallel()
		for _, v := range []any{(*jwtbearer.Source)(nil), (*jwtbearer.ClaimsSigner)(nil)} {
			for _, verb := range printVerbs {
				if out := fmt.Sprintf(verb, v); out != "<nil>" {
					t.Errorf("fmt.Sprintf(%q, a nil %T) = %q, want <nil>", verb, v, out)
				}
			}
		}
	})
}

// TestREQ025_FormatWithNilStateDoesNotPanic pins REQ-025 (idiom.md § No
// panics): Format called directly with a nil fmt.State returns without a
// panic, for the zero value of every type of auth/jwtbearer that has a Format
// method, and for a nil pointer where Format takes one.
func TestREQ025_FormatWithNilStateDoesNotPanic(t *testing.T) { // REQ-025
	t.Parallel()
	tests := []struct {
		name string
		v    fmt.Formatter
	}{
		{name: "jwtbearer.Config", v: jwtbearer.Config{}},
		{name: "*jwtbearer.ClaimsSigner", v: &jwtbearer.ClaimsSigner{}},
		{name: "nil *jwtbearer.ClaimsSigner", v: (*jwtbearer.ClaimsSigner)(nil)},
		{name: "*jwtbearer.Source", v: &jwtbearer.Source{}},
		{name: "nil *jwtbearer.Source", v: (*jwtbearer.Source)(nil)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("%s: Format(nil, 'v') panicked: %v", tc.name, r)
				}
			}()
			tc.v.Format(nil, 'v')
		})
	}
}
