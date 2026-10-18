package transport

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/cadasto/openehr-sdk-go/smart/discovery"
)

// TestREQ150_BaseURLKeepsItsOwnEncoding pins the request target the server
// receives when the service base URL carries an escaped byte. The base keeps
// its own encoding, so an escaped "/" (%2F) in a tenant segment stays one
// segment; the request path is encoded from its decoded form alone, so a
// literal "%" in it is sent as %25 (REQ-150, REQ-095). A base with no
// special encoding is sent as before.
func TestREQ150_BaseURLKeepsItsOwnEncoding(t *testing.T) {
	t.Parallel()
	var (
		mu  sync.Mutex
		got string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		got = r.RequestURI
		mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)

	tests := []struct {
		name   string
		base   string
		method string
		path   string
		want   string
	}{
		{name: "escaped slash in base", base: "/tenant%2F1/openehr", method: http.MethodGet, path: "/ehr/x", want: "/tenant%2F1/openehr/ehr/x"},
		{name: "escaped slash in base, service root", base: "/tenant%2F1/openehr", method: http.MethodOptions, path: "/", want: "/tenant%2F1/openehr/"},
		{name: "escaped slash in base, request path encoded", base: "/tenant%2F1/openehr", method: http.MethodGet, path: "/ehr/a b/%2e%2e", want: "/tenant%2F1/openehr/ehr/a%20b/%252e%252e"},
		{name: "plain base", base: "/openehr/v1", method: http.MethodGet, path: "/definition/template/adl1.4/Referral Request.v1", want: "/openehr/v1/definition/template/adl1.4/Referral%20Request.v1"},
		{name: "plain base, service root", base: "/openehr/v1", method: http.MethodOptions, path: "/", want: "/openehr/v1/"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cat, err := discovery.NewStaticCatalog(discovery.StaticConfig{
				Issuer: "https://idp.example",
				Services: map[string]discovery.ServiceEntry{
					discovery.ServiceIDOpenEHRRest: {BaseURL: discovery.MustParseURL(srv.URL + tc.base), SpecVersion: discovery.SpecVersionPin},
				},
			})
			if err != nil {
				t.Fatalf("NewStaticCatalog: %v", err)
			}
			c, err := New(cat, WithHTTPClient(srv.Client()))
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			if _, err := c.Do(t.Context(), &Request{Method: tc.method, Path: tc.path}); err != nil {
				t.Fatalf("Do(%s %q) = %v, want nil", tc.method, tc.path, err)
			}
			mu.Lock()
			defer mu.Unlock()
			if got != tc.want {
				t.Errorf("base %q, %s %q: server saw %q, want %q", tc.base, tc.method, tc.path, got, tc.want)
			}
		})
	}
}
