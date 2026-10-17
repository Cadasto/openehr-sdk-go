package clientcreds_test

import (
	"net/http"
	"testing"
	"testing/synctest"
	"time"
)

// TestREQ063_LargeExpiresInGivesAFutureExpiry pins REQ-063 (auth.md § Token
// refresh): an expires_in too large to count in nanoseconds as a
// time.Duration still gives the access token an expiry in the future, so the
// source keeps using the token and does not run an exchange on every call.
func TestREQ063_LargeExpiresInGivesAFutureExpiry(t *testing.T) {
	tests := []struct {
		name      string
		expiresIn string
		// atLeast is the shortest lifetime the token may get; exact asks for
		// exactly that lifetime.
		atLeast time.Duration
		exact   bool
	}{
		{name: "an hour", expiresIn: "3600", atLeast: time.Hour, exact: true},
		{name: "just past the time.Duration range", expiresIn: "9223372037", atLeast: 50 * 365 * 24 * time.Hour},
		{name: "ten billion seconds", expiresIn: "10000000000", atLeast: 50 * 365 * 24 * time.Hour},
		{name: "the largest int64", expiresIn: "9223372036854775807", atLeast: 50 * 365 * 24 * time.Hour},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				rt := &scriptedTransport{answer: func(r *http.Request, _ int) (*http.Response, error) {
					return tokenAnswer(r, http.StatusOK, `{"access_token":"tok","token_type":"Bearer","expires_in":`+tc.expiresIn+`}`), nil
				}}
				src := newSource(t, "https://idp.test/token", &http.Client{Transport: rt})
				start := time.Now()

				tok, err := src.Token(t.Context())
				if err != nil || tok.Value != "tok" {
					t.Fatalf("Token() = (%q, %v), want (tok, nil)", tok.Value, err)
				}
				lifetime := tok.ExpiresAt.Sub(start)
				if lifetime < tc.atLeast || (tc.exact && lifetime != tc.atLeast) {
					t.Errorf("expires_in %s gives the token a lifetime of %v, want %v at least (exactly: %t)", tc.expiresIn, lifetime, tc.atLeast, tc.exact)
				}
				if tok, err := src.Token(t.Context()); err != nil || tok.Value != "tok" {
					t.Errorf("second Token() = (%q, %v), want (tok, nil)", tok.Value, err)
				}
				if got := rt.posts.Load(); got != 1 {
					t.Errorf("token-endpoint requests = %d, want 1: the token is still valid", got)
				}
			})
		})
	}
}
