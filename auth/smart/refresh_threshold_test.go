package smart_test

import (
	"net/http"
	"testing"
	"testing/synctest"
	"time"

	"github.com/cadasto/openehr-sdk-go/auth"
	"github.com/cadasto/openehr-sdk-go/auth/smart"
	"github.com/cadasto/openehr-sdk-go/smart/discovery"
)

// TestREQ063_NegativeRefreshThresholdCountsAsZero pins REQ-063 (auth.md §
// Token refresh): a negative refresh threshold, set by option or in Config,
// counts as 0. A token past its expiry is refreshed, never returned
// silently, and a token still valid is returned without a refresh, as with
// a threshold of 0 and not the 30-second default.
func TestREQ063_NegativeRefreshThresholdCountsAsZero(t *testing.T) {
	builds := []struct {
		name  string
		build func(t *testing.T, srv *scriptedServer) *smart.Source
	}{
		{name: "WithRefreshThreshold", build: func(t *testing.T, srv *scriptedServer) *smart.Source {
			return scriptedSource(t, srv, smart.WithRefreshThreshold(-time.Minute))
		}},
		{name: "Config.RefreshThreshold", build: func(t *testing.T, srv *scriptedServer) *smart.Source {
			src, err := smart.FromConfig(smart.Config{
				HTTPClient: &http.Client{Transport: srv},
				ClientID:   "client-id",
				Audience:   testAudience,
				Auth: discovery.AuthEndpoints{
					AuthorizationEndpoint: discovery.MustParseURL("https://idp.test/authorize"),
					TokenEndpoint:         discovery.MustParseURL("https://idp.test/token"),
				},
				RefreshThreshold: -time.Minute,
			})
			if err != nil {
				t.Fatal(err)
			}
			return src
		}},
	}
	tests := []struct {
		name      string
		expiresIn time.Duration // from now; negative is in the past
		want      string
		refreshes int
	}{
		{name: "expired ten seconds ago", expiresIn: -10 * time.Second, want: "at-new", refreshes: 1},
		{name: "valid ten seconds more", expiresIn: 10 * time.Second, want: "at-held", refreshes: 0},
	}
	for _, b := range builds {
		for _, tc := range tests {
			t.Run(b.name+"/"+tc.name, func(t *testing.T) {
				synctest.Test(t, func(t *testing.T) {
					srv := &scriptedServer{refresh: func(r *http.Request, _ int) (*http.Response, error) {
						return scriptedAnswer(r, http.StatusOK, freshTokenBody), nil
					}}
					src := b.build(t, srv)
					src.SetTokens(auth.Token{Value: "at-held", Type: auth.TokenTypeBearer, ExpiresAt: time.Now().Add(tc.expiresIn)}, "rt-0")

					tok, err := src.Token(t.Context())
					if err != nil || tok.Value != tc.want {
						t.Errorf("Token() with a token that expires in %v = (%q, %v), want (%s, nil)", tc.expiresIn, tok.Value, err, tc.want)
					}
					if n, _ := srv.counts(); n != tc.refreshes {
						t.Errorf("refresh requests = %d, want %d", n, tc.refreshes)
					}
				})
			})
		}
	}
}
