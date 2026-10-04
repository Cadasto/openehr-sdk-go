package discovery_test

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/cadasto/openehr-sdk-go/smart/discovery"
)

// TestResolveRefusesPlaintextBaseURLInAnyCase pins REQ-073: a resolver built
// without WithAllowInsecure refuses a plaintext base URL with
// ReasonInsecureURL before any request leaves the process. URL schemes are
// case-insensitive (RFC 3986 §3.1), so "HTTP://" and "Http://" are as
// plaintext as "http://".
func TestResolveRefusesPlaintextBaseURLInAnyCase(t *testing.T) { // REQ-073
	const doc = `{"services":{"org.openehr.rest":{"baseUrl":"https://api.example.com/openehr/v1"}}}`
	for _, scheme := range []string{"http", "HTTP", "Http"} {
		t.Run(scheme, func(t *testing.T) {
			var hits atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				hits.Add(1)
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, doc)
			}))
			defer srv.Close()

			res, err := discovery.NewResolver(nil, discovery.WithHTTPClient(srv.Client()))
			if err != nil {
				t.Fatal(err)
			}
			baseURL := scheme + strings.TrimPrefix(srv.URL, "http")
			_, err = res.Resolve(t.Context(), baseURL)

			derr, ok := errors.AsType[*discovery.DiscoveryError](err)
			if !ok || derr.Reason != discovery.ReasonInsecureURL {
				t.Errorf("Resolve(%q) error = %v, want a DiscoveryError with Reason %q", baseURL, err, discovery.ReasonInsecureURL)
			}
			if got := hits.Load(); got != 0 {
				t.Errorf("Resolve(%q) sent %d request(s) to the server, want none", baseURL, got)
			}
		})
	}
}
