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
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

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

// challengeValues is a Bearer challenge whose every value is distinctive,
// so a leak of any of them into Error() is unmistakable.
var challengeValues = map[string]string{
	"realm":             "realm-7c1e",
	"error":             "insufficient_scope",
	"error_description": "token-for-patient-4411-lacks-scope",
	"error_uri":         "https://as.example/err-9d2a",
	"scope":             "patient/composition-5b8f.r",
	"resource_metadata": "https://rs.example/meta-0e6c",
}

// challengeHeader is challengeValues as one WWW-Authenticate line.
func challengeHeader() http.Header {
	var params []string
	for name, v := range challengeValues {
		params = append(params, name+`="`+v+`"`)
	}
	return http.Header{"Www-Authenticate": {"Bearer " + strings.Join(params, ", ")}}
}

// TestWireErrorErrorOmitsChallenge — REQ-166, REQ-093: Error() names no
// challenge value, whether or not the client keeps raw error bodies.
func TestWireErrorErrorOmitsChallenge(t *testing.T) { // REQ-166
	for _, raw := range []bool{false, true} {
		for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden} {
			t.Run(fmt.Sprintf("%d raw=%t", status, raw), func(t *testing.T) {
				c := newDecodeClient(t, status, "", challengeHeader(), transport.WithRawErrorBodies(raw))

				_, err := c.Do(t.Context(), &transport.Request{Path: "/x"})
				we, ok := errors.AsType[*transport.WireError](err)
				if !ok || we == nil || we.Challenge == nil {
					t.Fatalf("Do() error = %v, want a *WireError carrying a Challenge; the premise of this test is gone", err)
				}
				for name, v := range challengeValues {
					if strings.Contains(err.Error(), v) {
						t.Errorf("Error() = %q contains the %s value %q; challenge values stay out of the error text", err.Error(), name, v)
					}
				}
			})
		}
	}
}

// TestWireErrorChallengeDescriptionNeedsRawErrorBodies — REQ-166: the
// challenge's coded fields always reach the caller, while error_description,
// free text from the server, is kept only when the client is built with
// WithRawErrorBodies(true).
func TestWireErrorChallengeDescriptionNeedsRawErrorBodies(t *testing.T) { // REQ-166
	coded := transport.BearerChallenge{
		Realm:    challengeValues["realm"],
		Error:    challengeValues["error"],
		ErrorURI: challengeValues["error_uri"],
		Scope:    challengeValues["scope"],
		Params:   map[string]string{"resource_metadata": challengeValues["resource_metadata"]},
	}
	withDescription := coded
	withDescription.ErrorDescription = challengeValues["error_description"]
	cases := []struct {
		name string
		opts []transport.Option
		want transport.BearerChallenge
	}{
		{name: "default", want: coded},
		{name: "raw error bodies off", opts: []transport.Option{transport.WithRawErrorBodies(false)}, want: coded},
		{name: "raw error bodies on", opts: []transport.Option{transport.WithRawErrorBodies(true)}, want: withDescription},
	}
	for _, tc := range cases {
		for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden} {
			t.Run(fmt.Sprintf("%s %d", tc.name, status), func(t *testing.T) {
				c := newDecodeClient(t, status, "", challengeHeader(), tc.opts...)

				_, err := c.Do(t.Context(), &transport.Request{Path: "/x"})
				we, ok := errors.AsType[*transport.WireError](err)
				if !ok || we == nil || we.Challenge == nil {
					t.Fatalf("Do() error = %v, want a *WireError carrying a Challenge", err)
				}
				if !reflect.DeepEqual(*we.Challenge, tc.want) {
					t.Errorf("WireError.Challenge = %+v, want %+v", *we.Challenge, tc.want)
				}
			})
		}
	}
}

// TestReauthOn401FollowsBearerChallenge — REQ-166, REQ-063: the opt-in 401
// safety net calls Reauth and retries once only when the 401 has no Bearer
// challenge, or one that names no error or invalid_token. Any other error,
// insufficient_scope included, comes back at once: no Reauth, no retry. A
// 403 never reaches Reauth, even with a challenge a 401 would act on.
func TestReauthOn401FollowsBearerChallenge(t *testing.T) { // REQ-166
	cases := []struct {
		name      string
		status    int // the first response's status; zero means 401
		lines     []string
		reauth    bool   // whether Reauth runs and the request is retried
		wantError string // Challenge.Error on the surfaced response, when refused
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
			// The error code is compared exactly as RFC 6750 §3.1 spells it,
			// so a different spelling counts as another error.
			name:      "Bearer Invalid_Token in another case",
			lines:     []string{`Bearer error="Invalid_Token"`},
			wantError: "Invalid_Token",
		},
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
		{
			name:      "403 Bearer invalid_token",
			status:    http.StatusForbidden,
			lines:     []string{`Bearer error="invalid_token"`},
			wantError: "invalid_token",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, sentinel := http.StatusUnauthorized, transport.ErrUnauthorized
			if tc.status == http.StatusForbidden {
				status, sentinel = http.StatusForbidden, transport.ErrForbidden
			}
			var upstream, reauths atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if upstream.Add(1) == 1 {
					for _, l := range tc.lines {
						w.Header().Add("WWW-Authenticate", l)
					}
					w.WriteHeader(status)
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
			if !errors.Is(err, sentinel) {
				t.Fatalf("Do() error = %v, want errors.Is %v", err, sentinel)
			}
			we, ok := errors.AsType[*transport.WireError](err)
			if !ok || we == nil || we.Challenge == nil || we.Challenge.Error != tc.wantError {
				t.Errorf("surfaced error = %v, want a *WireError whose Challenge.Error is %q", err, tc.wantError)
			}
		})
	}
}

// TestReauthOn401RetryPolicyComesFirst — REQ-166, REQ-063, REQ-091: a
// RetryPolicy that lists 401 retries it under that policy whatever the
// challenge says; the challenge gate is consulted only once the policy has
// given up.
func TestReauthOn401RetryPolicyComesFirst(t *testing.T) { // REQ-166
	cases := []struct {
		name         string
		line         string
		wantReauths  int32
		wantUpstream int32
	}{
		// Three policy attempts, then the gate refuses.
		{name: "insufficient_scope", line: `Bearer error="insufficient_scope"`, wantReauths: 0, wantUpstream: 3},
		// Three policy attempts, then one Reauth and its one retry.
		{name: "invalid_token", line: `Bearer error="invalid_token"`, wantReauths: 1, wantUpstream: 4},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var upstream, reauths atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				upstream.Add(1)
				w.Header().Set("WWW-Authenticate", tc.line)
				w.WriteHeader(http.StatusUnauthorized)
			}))
			t.Cleanup(srv.Close)
			c, err := transport.New(newDecodeCatalog(t, srv),
				transport.WithHTTPClient(srv.Client()),
				transport.WithRetry(transport.RetryPolicy{
					MaxAttempts:     3,
					InitialBackoff:  time.Millisecond,
					RetriableStatus: []int{http.StatusUnauthorized},
				}),
				transport.WithReauthOn401(auth.ReautherFunc(func(context.Context) error {
					reauths.Add(1)
					return nil
				})),
			)
			if err != nil {
				t.Fatal(err)
			}

			_, err = c.Do(t.Context(), &transport.Request{Path: "/x"})
			if !errors.Is(err, transport.ErrUnauthorized) {
				t.Fatalf("Do() error = %v, want errors.Is ErrUnauthorized", err)
			}
			if got := reauths.Load(); got != tc.wantReauths {
				t.Errorf("Reauth calls = %d, want %d", got, tc.wantReauths)
			}
			if got := upstream.Load(); got != tc.wantUpstream {
				t.Errorf("upstream requests = %d, want %d", got, tc.wantUpstream)
			}
		})
	}
}
