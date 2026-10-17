package transport

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/cadasto/openehr-sdk-go/smart/discovery"
)

// failingRoundTripper fails every request as a network failure would, and
// counts them.
type failingRoundTripper struct{ hits *atomic.Int32 }

func (f failingRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	f.hits.Add(1)
	return nil, errors.New("simulated dial failure")
}

// TestREQ091_RequestThatCannotBeBuiltIsNotRetried pins that a request
// http.NewRequestWithContext refuses is tried once: its method and URL are
// the same on every attempt, so a retry would fail the same way after a
// wasted backoff. Its error text and its *url.Error are unchanged. A network
// failure under the same policy is still retried, as before (REQ-091).
func TestREQ091_RequestThatCannotBeBuiltIsNotRetried(t *testing.T) {
	policy := RetryPolicy{MaxAttempts: 3, InitialBackoff: 50 * time.Millisecond}

	t.Run("request that cannot be built", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			// The port is not a number, so the text url.URL.String makes of
			// the request URL does not parse.
			base := &url.URL{Scheme: "https", Host: "x.example:bad", Path: "/openehr/v1"}
			rec := &recorder{}
			c := clientForBase(t, base, http.DefaultClient, WithObserver(rec), WithRetry(policy))
			start := time.Now()
			_, err := c.Do(t.Context(), &Request{Method: http.MethodGet, Path: "/ehr", Route: "/ehr"})
			elapsed := time.Since(start)
			if err == nil {
				t.Fatal("Do error = nil, want the request-build error")
			}
			if want := `transport: build request GET /ehr: parse "/ehr": `; !strings.HasPrefix(err.Error(), want) {
				t.Errorf("Do error = %q, want it to start with %q", err.Error(), want)
			}
			if ue, ok := errors.AsType[*url.Error](err); !ok || ue == nil || ue.Op != "parse" {
				t.Errorf("Do error = %v, want the *url.Error url.Parse returned in its chain", err)
			}
			obs := rec.snapshot()
			if len(obs) != 1 {
				t.Fatalf("observer fired %d times, want 1", len(obs))
			}
			if obs[0].Attempts != 1 {
				t.Errorf("Observation.Attempts = %d, want 1", obs[0].Attempts)
			}
			if elapsed != 0 {
				t.Errorf("Do waited %v, want no backoff", elapsed)
			}
		})
	})

	t.Run("network failure", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			var hits atomic.Int32
			base := discovery.MustParseURL("https://cdr.example/openehr/v1")
			rec := &recorder{}
			c := clientForBase(t, base, &http.Client{Transport: failingRoundTripper{hits: &hits}}, WithObserver(rec), WithRetry(policy))
			if _, err := c.Do(t.Context(), &Request{Method: http.MethodGet, Path: "/ehr", Route: "/ehr"}); err == nil {
				t.Fatal("Do error = nil, want the network failure")
			}
			if n := hits.Load(); n != 3 {
				t.Errorf("requests sent = %d, want 3", n)
			}
			if obs := rec.snapshot(); len(obs) != 1 || obs[0].Attempts != 3 {
				t.Errorf("observations = %+v, want one with Attempts 3", obs)
			}
		})
	})
}

// clientForBase builds a Client whose openEHR REST service is at base.
func clientForBase(t *testing.T, base *url.URL, hc *http.Client, opts ...Option) *Client {
	t.Helper()
	cat, err := discovery.NewStaticCatalog(discovery.StaticConfig{
		Issuer: "https://idp.example",
		Services: map[string]discovery.ServiceEntry{
			discovery.ServiceIDOpenEHRRest: {BaseURL: base, SpecVersion: discovery.SpecVersionPin},
		},
	})
	if err != nil {
		t.Fatalf("NewStaticCatalog: %v", err)
	}
	c, err := New(cat, append([]Option{WithHTTPClient(hc)}, opts...)...)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}
