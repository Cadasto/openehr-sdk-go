package transport_test

// reauth_challenge_test.go — REQ-166 as a consumer sees it: the Bearer
// challenge on the *transport.WireError of a real 401 or 403, the value-free
// Error() text beside it, and the opt-in 401 safety net that reads it
// (REQ-063). External, like decode_error_test.go, so every assertion goes
// through the exported surface; the parser's own table lives in
// challenge_test.go.

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/cadasto/openehr-sdk-go/auth"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// TestWireErrorCarriesBearerChallenge — REQ-166: a 401 or 403 carrying a
// Bearer challenge exposes it as WireError.Challenge, from any header line;
// without a parseable Bearer challenge, or on another status, Challenge stays
// nil, and the status still maps to its sentinel either way.
func TestWireErrorCarriesBearerChallenge(t *testing.T) { // REQ-166
	cases := []struct {
		name     string
		status   int
		lines    []string
		sentinel error
		want     *transport.BearerChallenge
	}{
		{
			name:     "401 insufficient_scope",
			status:   http.StatusUnauthorized,
			lines:    []string{`Bearer realm="cdr", error="insufficient_scope", scope="patient/composition-*.r"`},
			sentinel: transport.ErrUnauthorized,
			want:     &transport.BearerChallenge{Realm: "cdr", Error: "insufficient_scope", Scope: "patient/composition-*.r"},
		},
		{
			name:     "403 insufficient_scope",
			status:   http.StatusForbidden,
			lines:    []string{`Bearer error="insufficient_scope", scope="patient/ehr.r"`},
			sentinel: transport.ErrForbidden,
			want:     &transport.BearerChallenge{Error: "insufficient_scope", Scope: "patient/ehr.r"},
		},
		{
			name:     "401 Bearer on the second header line",
			status:   http.StatusUnauthorized,
			lines:    []string{`Basic realm="b"`, `bearer error="invalid_token", resource_metadata="https://rs.example/meta"`},
			sentinel: transport.ErrUnauthorized,
			want: &transport.BearerChallenge{Error: "invalid_token", Params: map[string]string{
				"resource_metadata": "https://rs.example/meta",
			}},
		},
		{
			name:     "401 without a header",
			status:   http.StatusUnauthorized,
			sentinel: transport.ErrUnauthorized,
		},
		{
			name:     "401 with an unparseable Bearer challenge",
			status:   http.StatusUnauthorized,
			lines:    []string{`Bearer error="insufficient_scope`},
			sentinel: transport.ErrUnauthorized,
		},
		{
			name:     "403 with a non-Bearer challenge only",
			status:   http.StatusForbidden,
			lines:    []string{`DPoP error="invalid_token", algs="ES256"`},
			sentinel: transport.ErrForbidden,
		},
		{
			name:     "404 with a Bearer challenge",
			status:   http.StatusNotFound,
			lines:    []string{`Bearer error="invalid_token"`},
			sentinel: transport.ErrNotFound,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := newDecodeClient(t, tc.status, "", http.Header{"Www-Authenticate": tc.lines})

			_, err := c.Do(t.Context(), &transport.Request{Path: "/x"})
			if !errors.Is(err, tc.sentinel) {
				t.Fatalf("Do() error = %v, want errors.Is %v", err, tc.sentinel)
			}
			we, ok := errors.AsType[*transport.WireError](err)
			if !ok || we == nil {
				t.Fatalf("errors.AsType[*transport.WireError] did not match %T (%v)", err, err)
			}
			if !reflect.DeepEqual(we.Challenge, tc.want) {
				t.Errorf("WireError.Challenge = %+v, want %+v", we.Challenge, tc.want)
			}
		})
	}
}

// TestWireErrorErrorOmitsChallenge — REQ-166, REQ-093: Error() names no
// challenge value, while errors.AsType still reaches every one of them.
func TestWireErrorErrorOmitsChallenge(t *testing.T) { // REQ-166
	values := map[string]string{
		"realm":             "realm-7c1e",
		"error":             "insufficient_scope",
		"error_description": "token-for-patient-4411-lacks-scope",
		"error_uri":         "https://as.example/err-9d2a",
		"scope":             "patient/composition-5b8f.r",
		"resource_metadata": "https://rs.example/meta-0e6c",
	}
	var params []string
	for name, v := range values {
		params = append(params, name+`="`+v+`"`)
	}
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			c := newDecodeClient(t, status, "", http.Header{"Www-Authenticate": {"Bearer " + strings.Join(params, ", ")}})

			_, err := c.Do(t.Context(), &transport.Request{Path: "/x"})
			we, ok := errors.AsType[*transport.WireError](err)
			if !ok || we == nil {
				t.Fatalf("errors.AsType[*transport.WireError] did not match %T (%v)", err, err)
			}
			if we.Challenge == nil {
				t.Fatal("WireError.Challenge = nil; the premise of this test is gone")
			}
			for name, v := range values {
				if strings.Contains(err.Error(), v) {
					t.Errorf("Error() = %q contains the %s value %q; challenge values stay out of the error text", err.Error(), name, v)
				}
			}
			if we.Challenge.ErrorDescription != values["error_description"] || we.Challenge.Params["resource_metadata"] != values["resource_metadata"] {
				t.Errorf("WireError.Challenge = %+v, want the values reachable through errors.AsType", we.Challenge)
			}
		})
	}
}

// TestReauthOn401FollowsBearerChallenge — REQ-166, REQ-063: the opt-in 401
// safety net calls Reauth and retries once only when the 401 has no Bearer
// challenge, or one that names no error or invalid_token. Any other error,
// insufficient_scope included, comes back at once: no Reauth, no retry.
func TestReauthOn401FollowsBearerChallenge(t *testing.T) { // REQ-166
	cases := []struct {
		name      string
		lines     []string
		reauth    bool   // whether Reauth runs and the request is retried
		wantError string // Challenge.Error on the surfaced 401, when refused
	}{
		{name: "no header", reauth: true},
		{name: "Bearer without error", lines: []string{`Bearer realm="x"`}, reauth: true},
		{name: "Bearer invalid_token", lines: []string{`Bearer error="invalid_token"`}, reauth: true},
		{name: "non-Bearer challenge only", lines: []string{`Basic realm="x"`}, reauth: true},
		{name: "unparseable Bearer challenge", lines: []string{`Bearer error="insufficient_scope`}, reauth: true},
		{
			name:      "Bearer insufficient_scope",
			lines:     []string{`Bearer error="insufficient_scope", scope="patient/composition-*.r"`},
			wantError: "insufficient_scope",
		},
		{name: "Bearer invalid_request", lines: []string{`Bearer error="invalid_request"`}, wantError: "invalid_request"},
		{
			name:      "Basic then Bearer insufficient_scope on one line",
			lines:     []string{`Basic realm="x", Bearer error="insufficient_scope"`},
			wantError: "insufficient_scope",
		},
		{
			name:      "Bearer insufficient_scope on the second line",
			lines:     []string{`DPoP algs="ES256"`, `bearer ERROR="insufficient_scope"`},
			wantError: "insufficient_scope",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var upstream, reauths atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if upstream.Add(1) == 1 {
					for _, l := range tc.lines {
						w.Header().Add("WWW-Authenticate", l)
					}
					w.WriteHeader(http.StatusUnauthorized)
					return
				}
				_, _ = w.Write([]byte(`{}`))
			}))
			t.Cleanup(srv.Close)
			c, err := transport.New(newDecodeCatalog(t, srv),
				transport.WithHTTPClient(srv.Client()),
				transport.WithReauthOn401(auth.ReautherFunc(func(context.Context) error {
					reauths.Add(1)
					return nil
				})),
			)
			if err != nil {
				t.Fatal(err)
			}

			_, err = c.Do(t.Context(), &transport.Request{Path: "/x"})

			wantReauths, wantUpstream := int32(0), int32(1)
			if tc.reauth {
				wantReauths, wantUpstream = 1, 2
			}
			if got := reauths.Load(); got != wantReauths {
				t.Errorf("Reauth calls = %d, want %d", got, wantReauths)
			}
			if got := upstream.Load(); got != wantUpstream {
				t.Errorf("upstream requests = %d, want %d", got, wantUpstream)
			}
			if tc.reauth {
				if err != nil {
					t.Errorf("Do() error = %v, want success after the one Reauth and retry", err)
				}
				return
			}
			if !errors.Is(err, transport.ErrUnauthorized) {
				t.Fatalf("Do() error = %v, want errors.Is ErrUnauthorized", err)
			}
			we, ok := errors.AsType[*transport.WireError](err)
			if !ok || we == nil || we.Challenge == nil || we.Challenge.Error != tc.wantError {
				t.Errorf("surfaced error = %v, want a *WireError whose Challenge.Error is %q", err, tc.wantError)
			}
		})
	}
}
