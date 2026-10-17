package jwtbearer_test

import (
	"fmt"
	"net/http"
	"testing"
	"testing/synctest"
	"time"

	"github.com/cadasto/openehr-sdk-go/auth/jwtbearer"
)

// minuteTokenBody is the token response that grants "tok-<n>" for a minute.
func minuteTokenBody(n int) string {
	return fmt.Sprintf(`{"access_token":"tok-%d","token_type":"Bearer","expires_in":60}`, n)
}

// TestREQ063_NegativeRefreshThresholdCountsAsZero pins REQ-063 (auth.md §
// Token refresh): a negative refresh threshold, set by option or in Config,
// counts as 0. A token past its expiry is exchanged for a new one, never
// returned silently, and a token still valid is returned without an
// exchange, as with a threshold of 0 and not the 30-second default.
func TestREQ063_NegativeRefreshThresholdCountsAsZero(t *testing.T) {
	builds := []struct {
		name  string
		build func(t *testing.T, rt http.RoundTripper) *jwtbearer.Source
	}{
		{name: "WithRefreshThreshold", build: func(t *testing.T, rt http.RoundTripper) *jwtbearer.Source {
			src, err := jwtbearer.New("https://idp.test/token", jwtbearer.StaticAssertion("a.b.c"),
				jwtbearer.WithHTTPClient(&http.Client{Transport: rt}), jwtbearer.WithRefreshThreshold(-time.Minute))
			if err != nil {
				t.Fatal(err)
			}
			return src
		}},
		{name: "Config.RefreshThreshold", build: func(t *testing.T, rt http.RoundTripper) *jwtbearer.Source {
			src, err := jwtbearer.FromConfig(jwtbearer.Config{
				HTTPClient:       &http.Client{Transport: rt},
				TokenURL:         "https://idp.test/token",
				Assertion:        jwtbearer.StaticAssertion("a.b.c"),
				RefreshThreshold: -time.Minute,
			})
			if err != nil {
				t.Fatal(err)
			}
			return src
		}},
	}
	tests := []struct {
		name  string
		after time.Duration // how long after the first exchange Token is called again
		want  string
		posts int32
	}{
		{name: "expired ten seconds ago", after: 70 * time.Second, want: "tok-2", posts: 2},
		{name: "valid ten seconds more", after: 50 * time.Second, want: "tok-1", posts: 1},
	}
	for _, b := range builds {
		for _, tc := range tests {
			t.Run(b.name+"/"+tc.name, func(t *testing.T) {
				synctest.Test(t, func(t *testing.T) {
					rt := &scriptedTransport{answer: func(r *http.Request, n int) (*http.Response, error) {
						return tokenAnswer(r, http.StatusOK, minuteTokenBody(n)), nil
					}}
					src := b.build(t, rt)
					if tok, err := src.Token(t.Context()); err != nil || tok.Value != "tok-1" {
						t.Fatalf("first Token() = (%q, %v), want (tok-1, nil)", tok.Value, err)
					}
					time.Sleep(tc.after)

					tok, err := src.Token(t.Context())
					if err != nil || tok.Value != tc.want {
						t.Errorf("Token() %v after a token valid for a minute = (%q, %v), want (%s, nil)", tc.after, tok.Value, err, tc.want)
					}
					if got := rt.posts.Load(); got != tc.posts {
						t.Errorf("token-endpoint requests = %d, want %d", got, tc.posts)
					}
				})
			})
		}
	}
}
