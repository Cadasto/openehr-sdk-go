package jwtbearer_test

import (
	"errors"
	"fmt"
	"net/http"
	"testing"
	"testing/synctest"
	"time"

	"github.com/cadasto/openehr-sdk-go/auth"
	"github.com/cadasto/openehr-sdk-go/auth/jwtbearer"
)

// boundLifetime is the maximum token lifetime the tests set: ten minutes,
// well above the refresh threshold.
const boundLifetime = 10 * time.Minute

// defaultWindow is the source's default refresh threshold: a token is
// renewed once it expires within this time.
const defaultWindow = 30 * time.Second

// boundedBuilds build a source whose maximum token lifetime is d, by option
// and in Config, sending its token requests through rt.
var boundedBuilds = []struct {
	name  string
	build func(t *testing.T, rt http.RoundTripper, d time.Duration) *jwtbearer.Source
}{
	{name: "WithMaxTokenLifetime", build: func(t *testing.T, rt http.RoundTripper, d time.Duration) *jwtbearer.Source {
		t.Helper()
		src, err := jwtbearer.New("https://idp.test/token", jwtbearer.StaticAssertion("a.b.c"),
			jwtbearer.WithHTTPClient(&http.Client{Transport: rt}), jwtbearer.WithMaxTokenLifetime(d))
		if err != nil {
			t.Fatal(err)
		}
		return src
	}},
	{name: "Config.MaxTokenLifetime", build: func(t *testing.T, rt http.RoundTripper, d time.Duration) *jwtbearer.Source {
		t.Helper()
		src, err := jwtbearer.FromConfig(jwtbearer.Config{
			HTTPClient:       &http.Client{Transport: rt},
			TokenURL:         "https://idp.test/token",
			Assertion:        jwtbearer.StaticAssertion("a.b.c"),
			MaxTokenLifetime: d,
		})
		if err != nil {
			t.Fatal(err)
		}
		return src
	}},
}

// numberedGrant is a token response to r, the n-th token request, that
// grants "tok-<n>" with, unless expiresIn is empty, an expires_in member
// whose JSON text is expiresIn.
func numberedGrant(r *http.Request, n int, expiresIn string) *http.Response {
	return tokenAnswer(r, http.StatusOK, expiresInBody(fmt.Sprintf("tok-%d", n), expiresIn))
}

// TestREQ063_MaxTokenLifetimeBoundsTheExpiry pins REQ-063 (auth.md § REQ-063,
// A maximum token lifetime): with a maximum token lifetime d, a token whose
// response has no expires_in, or a null one, expires d after it arrived, and
// one whose expires_in exceeds d is capped there. The first Token call once
// d less the refresh threshold has passed renews it, with no 401 first. A
// token whose expires_in is d or below keeps its own expiry, a lifetime of
// zero included.
func TestREQ063_MaxTokenLifetimeBoundsTheExpiry(t *testing.T) {
	tests := []struct {
		name      string
		expiresIn string // the member's JSON text; empty leaves it out
		want      time.Duration
	}{
		{name: "no expires_in", expiresIn: "", want: boundLifetime},
		{name: "null expires_in", expiresIn: "null", want: boundLifetime},
		{name: "expires_in above the bound", expiresIn: "3600", want: boundLifetime},
		{name: "expires_in at the bound", expiresIn: "600", want: boundLifetime},
		{name: "expires_in below the bound", expiresIn: "300", want: 5 * time.Minute},
		{name: "expires_in zero", expiresIn: "0", want: 0},
	}
	for _, b := range boundedBuilds {
		for _, tc := range tests {
			t.Run(b.name+"/"+tc.name, func(t *testing.T) {
				synctest.Test(t, func(t *testing.T) {
					rt := &scriptedTransport{answer: func(r *http.Request, n int) (*http.Response, error) {
						return numberedGrant(r, n, tc.expiresIn), nil
					}}
					src := b.build(t, rt, boundLifetime)
					start := time.Now()

					tok, err := src.Token(t.Context())
					if err != nil || tok.Value != "tok-1" {
						t.Fatalf("first Token() = (%q, %v), want (tok-1, nil)", tok.Value, err)
					}
					if tok.ExpiresAt.IsZero() {
						t.Fatalf("expires_in %q under a %v bound gives the token no expiry, want a lifetime of %v", tc.expiresIn, boundLifetime, tc.want)
					}
					if got := tok.ExpiresAt.Sub(start); got != tc.want {
						t.Errorf("expires_in %q under a %v bound gives the token a lifetime of %v, want %v", tc.expiresIn, boundLifetime, got, tc.want)
					}
					if tc.want > defaultWindow {
						// One second before the refresh window opens: still held.
						time.Sleep(tc.want - defaultWindow - time.Second)
						if tok, err := src.Token(t.Context()); err != nil || tok.Value != "tok-1" {
							t.Errorf("Token() %v after the grant = (%q, %v), want (tok-1, nil)", time.Since(start), tok.Value, err)
						}
						if got := rt.posts.Load(); got != 1 {
							t.Errorf("token-endpoint requests %v after the grant = %d, want 1", time.Since(start), got)
						}
						time.Sleep(time.Second)
					}
					// The refresh window is open: Token renews the token itself.
					if tok, err := src.Token(t.Context()); err != nil || tok.Value != "tok-2" {
						t.Errorf("Token() %v after the grant = (%q, %v), want (tok-2, nil): the token is due", time.Since(start), tok.Value, err)
					}
					if got := rt.posts.Load(); got != 2 {
						t.Errorf("token-endpoint requests %v after the grant = %d, want 2", time.Since(start), got)
					}
				})
			})
		}
	}
}

// TestREQ063_WithoutMaxTokenLifetimeATokenWithoutExpiryIsKept pins REQ-063
// (auth.md § REQ-063): without a maximum token lifetime, by leaving out the
// option or with a Config.MaxTokenLifetime of zero, a token whose response
// has no expires_in has no expiry. Token keeps returning it, however long
// after, until Reauth drops it.
func TestREQ063_WithoutMaxTokenLifetimeATokenWithoutExpiryIsKept(t *testing.T) {
	builds := []struct {
		name  string
		build func(t *testing.T, rt http.RoundTripper) *jwtbearer.Source
	}{
		{name: "New without WithMaxTokenLifetime", build: func(t *testing.T, rt http.RoundTripper) *jwtbearer.Source {
			return newSource(t, "https://idp.test/token", &http.Client{Transport: rt})
		}},
		{name: "Config.MaxTokenLifetime 0", build: func(t *testing.T, rt http.RoundTripper) *jwtbearer.Source {
			src, err := jwtbearer.FromConfig(jwtbearer.Config{
				HTTPClient: &http.Client{Transport: rt},
				TokenURL:   "https://idp.test/token",
				Assertion:  jwtbearer.StaticAssertion("a.b.c"),
			})
			if err != nil {
				t.Fatal(err)
			}
			return src
		}},
	}
	for _, b := range builds {
		t.Run(b.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				rt := &scriptedTransport{answer: func(r *http.Request, n int) (*http.Response, error) {
					return numberedGrant(r, n, ""), nil
				}}
				src := b.build(t, rt)

				tok, err := src.Token(t.Context())
				if err != nil || tok.Value != "tok-1" {
					t.Fatalf("first Token() = (%q, %v), want (tok-1, nil)", tok.Value, err)
				}
				if !tok.ExpiresAt.IsZero() {
					t.Errorf("a token without expires_in and no bound expires at %v, want no expiry", tok.ExpiresAt)
				}
				time.Sleep(1000 * time.Hour)
				if tok, err := src.Token(t.Context()); err != nil || tok.Value != "tok-1" {
					t.Errorf("Token() 1000h after the grant = (%q, %v), want (tok-1, nil): the token has no expiry", tok.Value, err)
				}
				if got := rt.posts.Load(); got != 1 {
					t.Errorf("token-endpoint requests before Reauth = %d, want 1", got)
				}
				if err := src.Reauth(t.Context()); err != nil {
					t.Fatalf("Reauth() = %v, want nil", err)
				}
				if tok, err := src.Token(t.Context()); err != nil || tok.Value != "tok-2" {
					t.Errorf("Token() after Reauth = (%q, %v), want (tok-2, nil)", tok.Value, err)
				}
			})
		})
	}
}

// TestREQ063_MaxTokenLifetimeHoldsTheBackoffToTheCappedExpiry pins REQ-063
// (auth.md § REQ-063, A maximum token lifetime): the capped ExpiresAt is the
// one the transient-failure backoff reads. When the renewal of a token
// without expires_in fails transiently inside the refresh window, Token
// serves the held token; once the capped expiry has passed it returns the
// failure.
func TestREQ063_MaxTokenLifetimeHoldsTheBackoffToTheCappedExpiry(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		rt := &scriptedTransport{answer: func(r *http.Request, n int) (*http.Response, error) {
			if n == 1 {
				return numberedGrant(r, n, ""), nil
			}
			return outage(r, n)
		}}
		src := boundedBuilds[0].build(t, rt, boundLifetime)
		if tok, err := src.Token(t.Context()); err != nil || tok.Value != "tok-1" {
			t.Fatalf("first Token() = (%q, %v), want (tok-1, nil)", tok.Value, err)
		}

		time.Sleep(boundLifetime - defaultWindow)
		if tok, err := src.Token(t.Context()); err != nil || tok.Value != "tok-1" {
			t.Errorf("Token() inside the refresh window during an outage = (%q, %v), want (tok-1, nil): the held token is still valid", tok.Value, err)
		}
		if got := rt.posts.Load(); got != 2 {
			t.Errorf("token-endpoint requests inside the refresh window = %d, want 2 (the grant, one failed renewal)", got)
		}

		time.Sleep(defaultWindow)
		if _, err := src.Token(t.Context()); !errors.Is(err, auth.ErrTokenExchangeFailed) {
			t.Errorf("Token() at the capped expiry during an outage error = %v, want auth.ErrTokenExchangeFailed", err)
		}
	})
}

// TestREQ063_MaxTokenLifetimeAtOrBelowZeroIsRefused pins REQ-063 (auth.md §
// REQ-063, A maximum token lifetime): WithMaxTokenLifetime with a d of zero
// or below, and a negative Config.MaxTokenLifetime, fail construction with
// auth.ErrInvalidConfig, through New and FromConfig alike. A
// Config.MaxTokenLifetime of zero sets no bound and is accepted, as is any
// positive bound.
func TestREQ063_MaxTokenLifetimeAtOrBelowZeroIsRefused(t *testing.T) {
	t.Parallel()
	client := jwtbearer.WithHTTPClient(&http.Client{})
	assertion := jwtbearer.StaticAssertion("a.b.c")
	fromConfig := func(d time.Duration) (*jwtbearer.Source, error) {
		return jwtbearer.FromConfig(jwtbearer.Config{
			HTTPClient:       &http.Client{},
			TokenURL:         "https://idp.test/token",
			Assertion:        assertion,
			MaxTokenLifetime: d,
		})
	}
	tests := []struct {
		name    string
		build   func() (*jwtbearer.Source, error)
		refused bool
	}{
		{name: "WithMaxTokenLifetime(0)", refused: true, build: func() (*jwtbearer.Source, error) {
			return jwtbearer.New("https://idp.test/token", assertion, client, jwtbearer.WithMaxTokenLifetime(0))
		}},
		{name: "WithMaxTokenLifetime(-1s)", refused: true, build: func() (*jwtbearer.Source, error) {
			return jwtbearer.New("https://idp.test/token", assertion, client, jwtbearer.WithMaxTokenLifetime(-time.Second))
		}},
		{name: "Config.MaxTokenLifetime -1ns", refused: true, build: func() (*jwtbearer.Source, error) {
			return fromConfig(-1)
		}},
		{name: "Config.MaxTokenLifetime 0", build: func() (*jwtbearer.Source, error) {
			return fromConfig(0)
		}},
		{name: "WithMaxTokenLifetime(1ns)", build: func() (*jwtbearer.Source, error) {
			return jwtbearer.New("https://idp.test/token", assertion, client, jwtbearer.WithMaxTokenLifetime(time.Nanosecond))
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			src, err := tc.build()
			if !tc.refused {
				if err != nil || src == nil {
					t.Errorf("%s = (%v, %v), want a source and no error", tc.name, src, err)
				}
				return
			}
			if !errors.Is(err, auth.ErrInvalidConfig) || src != nil {
				t.Errorf("%s = (%v, %v), want (nil, an error matching auth.ErrInvalidConfig)", tc.name, src, err)
			}
		})
	}
}
