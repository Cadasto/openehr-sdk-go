package smart_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/cadasto/openehr-sdk-go/auth"
	"github.com/cadasto/openehr-sdk-go/auth/smart"
	"github.com/cadasto/openehr-sdk-go/smart/discovery"
)

// redactedMarker is the text a credential prints and logs as.
const redactedMarker = "[redacted]"

// urlMask is the text url.URL.Redacted writes in place of a URL's password.
const urlMask = "xxxxx"

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

// The credentials of credentialValues, each a value the tests look for in
// printed text.
const (
	credSecret   = "CLIENT-SECRET-s3cr3t"
	credAccess   = "ACCESS-s3cr3t"
	credRefresh  = "REFRESH-s3cr3t"
	credIDToken  = "ID-TOKEN-s3cr3t"
	credURLPass  = "urlpass-s3cr3t"
	credVerifier = "VERIFIER-s3cr3t"
)

// credentialResponse returns a token response that carries credAccess,
// credRefresh and credIDToken, in its fields and in Raw.
func credentialResponse() smart.TokenResponse {
	return smart.TokenResponse{
		AccessToken:       credAccess,
		TokenType:         "Bearer",
		ExpiresIn:         3600,
		RefreshToken:      credRefresh,
		Scope:             "openid launch/patient",
		IDToken:           credIDToken,
		Patient:           "patient-1",
		NeedPatientBanner: new(true),
		Raw: map[string]any{
			"access_token":  credAccess,
			"refresh_token": credRefresh,
			"id_token":      credIDToken,
			"token_type":    "Bearer",
			"patient":       "patient-1",
		},
	}
}

// The URLs of urlResponse, each with credURLPass as its password, and the
// text each prints as.
const (
	credIssuer         = "https://iss:" + credURLPass + "@idp.example"
	credIssuerMasked   = "https://iss:" + urlMask + "@idp.example"
	credFHIRUser       = "https://user:" + credURLPass + "@fhir.example/Practitioner/1"
	credFHIRUserMasked = "https://user:" + urlMask + "@fhir.example/Practitioner/1"
	credStyle          = "https://style:" + credURLPass + "@app.example/smart-style.json"
	credStyleMasked    = "https://style:" + urlMask + "@app.example/smart-style.json"
)

// urlResponse returns credentialResponse with credURLPass as the password of
// every URL it holds: FHIRUser and SMARTStyleURL, the issuer and fhirUser
// claims of IDTokenClaims, and the iss, fhirUser and smart_style_url
// members of Raw.
func urlResponse() smart.TokenResponse {
	tr := credentialResponse()
	tr.FHIRUser = credFHIRUser
	tr.SMARTStyleURL = credStyle
	tr.IDTokenClaims = &smart.IDTokenClaims{Subject: "sub-1", Audience: []string{"client-id"}, Issuer: credIssuer, FHIRUser: credFHIRUser}
	tr.Raw["iss"] = credIssuer
	tr.Raw["fhirUser"] = credFHIRUser
	tr.Raw["smart_style_url"] = credStyle
	return tr
}

// credentialRequest returns an authorization request whose PKCE verifier is
// credVerifier.
func credentialRequest() smart.AuthorizationRequest {
	return smart.AuthorizationRequest{
		State:  "state-1",
		Launch: "launch-1",
		PKCE:   smart.PKCEPair{Verifier: credVerifier, Challenge: "challenge-1"},
		Issuer: "https://idp.example",
		Nonce:  "nonce-1",
	}
}

// TestREQ060_PrintsNoCredential pins REQ-060: a smart.Config, a
// smart.TokenResponse, a smart.TokenChange, a smart.PKCEPair and a
// smart.AuthorizationRequest, printed with any fmt verb or logged through
// log/slog, show each credential they hold as [redacted], the password of
// each URL they hold masked and their other fields as they are; so does a
// *smart.Source, for its issuer. The token members of a TokenResponse's Raw
// keep their keys, and printing does not change the Raw or the ID token
// claims it was given.
func TestREQ060_PrintsNoCredential(t *testing.T) { // REQ-060
	t.Parallel()
	key := newRSAKey(t)
	privateD := key.D.String()
	tokenEndpoint := &url.URL{Scheme: "https", User: url.UserPassword("svc", credURLPass), Host: "idp.example", Path: "/token"}
	secretConfig := smart.Config{
		ClientID:     "client-id",
		ClientSecret: credSecret,
		RedirectURI:  "https://app.example/callback",
		Scopes:       []string{"openid", "launch/patient"},
		Audience:     testAudience,
		Issuer:       "https://idp.example",
		Auth: discovery.AuthEndpoints{
			AuthorizationEndpoint: &url.URL{Scheme: "https", Host: "idp.example", Path: "/authorize"},
			TokenEndpoint:         tokenEndpoint,
		},
	}
	keyConfig := smart.Config{ClientID: "client-id"}
	smart.WithClientAssertionKey(key, "RS384", "kid-1")(&keyConfig)
	response := credentialResponse()
	change := smart.TokenChange{
		Access:       auth.Token{Value: credAccess, Type: auth.TokenTypeBearer, Scope: "openid"},
		RefreshToken: credRefresh,
		Response:     response,
	}
	request := credentialRequest()
	responseSecrets := []string{credAccess, credRefresh, credIDToken}

	tests := []struct {
		name    string
		v       any
		secrets []string
		shown   []string
	}{
		{
			name:    "Config with a client secret",
			v:       secretConfig,
			secrets: []string{credSecret, credURLPass},
			shown:   []string{redactedMarker, "client-id", "https://app.example/callback", "launch/patient", testAudience, "https://idp.example/authorize", "idp.example/token"},
		},
		{
			name: "Config with a password in its issuer, audience and redirect URI",
			v: smart.Config{
				ClientID:     "client-id",
				ClientSecret: credSecret,
				RedirectURI:  "https://app:" + credURLPass + "@app.example/callback",
				Audience:     "https://aud:" + credURLPass + "@cdr.example/openehr",
				Issuer:       "https://iss:" + credURLPass + "@idp.example",
			},
			secrets: []string{credSecret, credURLPass},
			shown: []string{
				redactedMarker, "client-id",
				"https://app:" + urlMask + "@app.example/callback",
				"https://aud:" + urlMask + "@cdr.example/openehr",
				"https://iss:" + urlMask + "@idp.example",
			},
		},
		{
			name:    "*Config with a client assertion key",
			v:       &keyConfig,
			secrets: []string{privateD},
			shown:   []string{redactedMarker, "client-id", "RS384", "kid-1"},
		},
		{
			name:    "TokenResponse",
			v:       response,
			secrets: responseSecrets,
			shown:   []string{redactedMarker, "Bearer", "3600", "openid launch/patient", "patient-1", "access_token", "refresh_token", "id_token"},
		},
		{
			name:    "*TokenResponse",
			v:       &response,
			secrets: responseSecrets,
			shown:   []string{redactedMarker, "patient-1"},
		},
		{
			name:    "TokenChange",
			v:       change,
			secrets: responseSecrets,
			shown:   []string{redactedMarker, "Bearer", "patient-1"},
		},
		{
			name:    "PKCEPair",
			v:       request.PKCE,
			secrets: []string{credVerifier},
			shown:   []string{redactedMarker, "challenge-1"},
		},
		{
			name:    "AuthorizationRequest",
			v:       request,
			secrets: []string{credVerifier},
			shown:   []string{redactedMarker, "state-1", "launch-1", "challenge-1", "https://idp.example", "nonce-1"},
		},
		{
			name:    "TokenResponse with a password in every URL",
			v:       urlResponse(),
			secrets: append([]string{credURLPass}, responseSecrets...),
			shown:   []string{redactedMarker, "patient-1", "sub-1", credIssuerMasked, credFHIRUserMasked, credStyleMasked},
		},
		{
			name: "TokenChange with a password in every URL",
			v: smart.TokenChange{
				Access:       auth.Token{Value: credAccess, Type: auth.TokenTypeBearer, Issuer: credIssuer},
				RefreshToken: credRefresh,
				Response:     urlResponse(),
			},
			secrets: append([]string{credURLPass}, responseSecrets...),
			shown:   []string{redactedMarker, credIssuerMasked, credFHIRUserMasked, credStyleMasked},
		},
		{
			name: "AuthorizationRequest with a password in its issuer",
			v: smart.AuthorizationRequest{
				State:  "state-1",
				PKCE:   smart.PKCEPair{Verifier: credVerifier, Challenge: "challenge-1"},
				Issuer: credIssuer,
			},
			secrets: []string{credVerifier, credURLPass},
			shown:   []string{redactedMarker, "state-1", credIssuerMasked},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if _, ok := tc.v.(slog.LogValuer); !ok {
				t.Errorf("%T does not implement slog.LogValuer", tc.v)
			}
			for _, p := range printProblems(tc.v, tc.secrets, tc.shown) {
				t.Error(p)
			}
		})
	}

	t.Run("*Source with a password in its issuer", func(t *testing.T) {
		t.Parallel()
		src, err := smart.New("client-id", discovery.AuthEndpoints{
			AuthorizationEndpoint: &url.URL{Scheme: "https", Host: "idp.example", Path: "/authorize"},
			TokenEndpoint:         &url.URL{Scheme: "https", Host: "idp.example", Path: "/token"},
		}, smart.WithHTTPClient(&http.Client{}), smart.WithRedirectURI("https://app.example/callback"),
			smart.WithAudience(testAudience), smart.WithIssuer(credIssuer))
		if err != nil {
			t.Fatalf("smart.New: %v", err)
		}
		for _, p := range printProblems(src, []string{credURLPass}, []string{"client-id", credIssuerMasked}) {
			t.Error(p)
		}
	})

	t.Run("Raw is not changed", func(t *testing.T) {
		t.Parallel()
		for _, fixture := range []func() smart.TokenResponse{credentialResponse, urlResponse} {
			tr := fixture()
			_ = fmt.Sprint(tr)
			var buf bytes.Buffer
			slog.New(slog.NewJSONHandler(&buf, nil)).Info("printed", "tr", tr)
			if !reflect.DeepEqual(tr.Raw, fixture().Raw) {
				t.Errorf("Raw after printing and logging = %v, want it unchanged", tr.Raw)
			}
			if !reflect.DeepEqual(tr.IDTokenClaims, fixture().IDTokenClaims) {
				t.Errorf("IDTokenClaims after printing and logging = %+v, want them unchanged", tr.IDTokenClaims)
			}
		}
	})

	t.Run("no credential prints as empty", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name  string
			v     any
			empty []string
		}{
			{name: "Config", v: smart.Config{ClientID: "client-id"}, empty: []string{`ClientSecret: ""`, "ClientAssertion: <nil>"}},
			{name: "TokenResponse", v: smart.TokenResponse{TokenType: "Bearer", Raw: map[string]any{"access_token": ""}}, empty: []string{`AccessToken: ""`, "access_token:]"}},
			{name: "PKCEPair", v: smart.PKCEPair{Challenge: "challenge-1"}, empty: []string{`Verifier: ""`}},
		}
		for _, tc := range tests {
			out := fmt.Sprint(tc.v)
			if strings.Contains(out, redactedMarker) {
				t.Errorf("fmt.Sprint(a %s without credentials) = %q, want no %s", tc.name, out, redactedMarker)
			}
			for _, e := range tc.empty {
				if !strings.Contains(out, e) {
					t.Errorf("fmt.Sprint(a %s without credentials) = %q, want it to show %s", tc.name, out, e)
				}
			}
		}
	})
}

// The types below have the fields of the smart types an application stores,
// and none of their methods.
type (
	plainTokenResponse        smart.TokenResponse
	plainTokenChange          smart.TokenChange
	plainAuthorizationRequest smart.AuthorizationRequest
)

// TestREQ060_JSONUnchanged pins REQ-060: the printing methods leave the JSON
// encoding of a TokenChange, a TokenResponse and an AuthorizationRequest as
// it is, credentials included, so an application can store one and read it
// back.
func TestREQ060_JSONUnchanged(t *testing.T) { // REQ-060
	t.Parallel()
	response := credentialResponse()
	change := smart.TokenChange{
		Access:       auth.Token{Value: credAccess, Type: auth.TokenTypeBearer, ExpiresAt: time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)},
		RefreshToken: credRefresh,
		Response:     response,
	}
	request := credentialRequest()
	tests := []struct {
		name   string
		v      any
		plain  any
		decode func([]byte) (any, error)
	}{
		{
			name:  "TokenResponse",
			v:     response,
			plain: plainTokenResponse(response),
			decode: func(b []byte) (any, error) {
				var back smart.TokenResponse
				err := json.Unmarshal(b, &back)
				return back, err
			},
		},
		{
			name:  "TokenChange",
			v:     change,
			plain: plainTokenChange(change),
			decode: func(b []byte) (any, error) {
				var back smart.TokenChange
				err := json.Unmarshal(b, &back)
				return back, err
			},
		},
		{
			name:  "AuthorizationRequest",
			v:     request,
			plain: plainAuthorizationRequest(request),
			decode: func(b []byte) (any, error) {
				var back smart.AuthorizationRequest
				err := json.Unmarshal(b, &back)
				return back, err
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := json.Marshal(tc.v)
			if err != nil {
				t.Fatalf("json.Marshal: %v", err)
			}
			want, err := json.Marshal(tc.plain)
			if err != nil {
				t.Fatalf("json.Marshal of the plain fields: %v", err)
			}
			if !bytes.Equal(got, want) {
				t.Errorf("json.Marshal = %s, want %s, the encoding of its fields", got, want)
			}
			back, err := tc.decode(got)
			if err != nil {
				t.Fatalf("json.Unmarshal(%s): %v", got, err)
			}
			if !reflect.DeepEqual(back, tc.v) {
				t.Errorf("json round trip of %s changed the value: got %s", tc.name, got)
			}
		})
	}
}
