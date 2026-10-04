package transport_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/cadasto/openehr-sdk-go/auth/clientcreds"
	"github.com/cadasto/openehr-sdk-go/smart/discovery"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// TestReauthOn401WithClientCredentials — REQ-063: with a client-credentials
// Source as both the token source and the Reauther, a 401 on a still-fresh
// cached token makes the transport call Reauth, which runs a new exchange,
// and the one retry carries the new token.
func TestReauthOn401WithClientCredentials(t *testing.T) { // REQ-063
	var (
		tokenPosts    atomic.Int32
		resourceCalls atomic.Int32
		retryBearer   atomic.Value
	)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /token", func(w http.ResponseWriter, r *http.Request) {
		n := tokenPosts.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"access_token":"tok-%d","token_type":"Bearer","expires_in":3600}`, n)
	})
	mux.HandleFunc("/openehr/v1/x", func(w http.ResponseWriter, r *http.Request) {
		// The server has revoked tok-1 although it has not expired.
		if resourceCalls.Add(1) == 2 {
			retryBearer.Store(r.Header.Get("Authorization"))
		}
		if r.Header.Get("Authorization") != "Bearer tok-2" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	src, err := clientcreds.New("c", "s", srv.URL+"/token", clientcreds.WithHTTPClient(srv.Client()))
	if err != nil {
		t.Fatal(err)
	}
	cat, err := discovery.NewStaticCatalog(discovery.StaticConfig{
		Issuer: srv.URL,
		Services: map[string]discovery.ServiceEntry{
			discovery.ServiceIDOpenEHRRest: {
				BaseURL:     discovery.MustParseURL(srv.URL + "/openehr/v1"),
				SpecVersion: discovery.SpecVersionPin,
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	c, err := transport.New(cat,
		transport.WithHTTPClient(srv.Client()),
		transport.WithTokenSource(src),
		transport.WithReauthOn401(src),
	)
	if err != nil {
		t.Fatal(err)
	}

	resp, err := c.Do(t.Context(), &transport.Request{Path: "/x"})
	if err != nil {
		t.Fatalf("Do() error = %v, want success after Reauth", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("StatusCode = %d, want 200", resp.StatusCode)
	}
	if got := resourceCalls.Load(); got != 2 {
		t.Errorf("resource calls = %d, want 2 (401 then the one retry)", got)
	}
	if got, _ := retryBearer.Load().(string); got != "Bearer tok-2" {
		t.Errorf("retry Authorization = %q, want Bearer tok-2", got)
	}
	if got := tokenPosts.Load(); got != 2 {
		t.Errorf("token-endpoint POSTs = %d, want 2 (first token, then Reauth)", got)
	}
}
