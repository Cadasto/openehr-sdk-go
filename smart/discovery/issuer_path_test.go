package discovery_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"

	"github.com/cadasto/openehr-sdk-go/smart/discovery"
)

// TestResolveFetchesWellKnownUnderIssuerPath verifies that the resolver
// appends the well-known path to the issuer's own path instead of replacing
// it (REQ-070). It pins the SMART on openEHR sentence that a base URL with a
// path segment, such as https://platform.example.com/gateway/v1, serves its
// configuration at
// https://platform.example.com/gateway/v1/.well-known/smart-configuration.
func TestResolveFetchesWellKnownUnderIssuerPath(t *testing.T) { // REQ-070
	const doc = `{"services":{"org.openehr.rest":{"baseUrl":"https://api.example.com/openehr/v1"}}}`
	tests := []struct {
		name       string
		issuerPath string // appended to the test server's origin
		opts       []discovery.Option
		wantPath   string
	}{
		{
			name:       "issuer with a path",
			issuerPath: "/gateway/v1",
			wantPath:   "/gateway/v1/.well-known/smart-configuration",
		},
		{
			name:       "issuer path with a trailing slash",
			issuerPath: "/gateway/v1/",
			wantPath:   "/gateway/v1/.well-known/smart-configuration",
		},
		{
			name:       "host-only issuer",
			issuerPath: "",
			wantPath:   "/.well-known/smart-configuration",
		},
		{
			name:       "host-only issuer with a slash",
			issuerPath: "/",
			wantPath:   "/.well-known/smart-configuration",
		},
		{
			name:       "custom well-known path under an issuer path",
			issuerPath: "/gateway/v1",
			opts:       []discovery.Option{discovery.WithWellKnownPath("/custom/smart-config")},
			wantPath:   "/gateway/v1/custom/smart-config",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var (
				mu    sync.Mutex
				paths []string
			)
			srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				paths = append(paths, r.URL.Path)
				mu.Unlock()
				if r.URL.Path != tc.wantPath {
					http.NotFound(w, r)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, doc)
			}))
			defer srv.Close()

			opts := append([]discovery.Option{discovery.WithHTTPClient(srv.Client())}, tc.opts...)
			res, err := discovery.NewResolver(nil, opts...)
			if err != nil {
				t.Fatal(err)
			}
			issuer := srv.URL + tc.issuerPath
			_, err = res.Resolve(t.Context(), issuer)

			mu.Lock()
			got := slices.Clone(paths)
			mu.Unlock()
			if err != nil {
				t.Fatalf("Resolve(%q) error = %v (requested paths %q), want the document at %q", issuer, err, got, tc.wantPath)
			}
			if want := []string{tc.wantPath}; !slices.Equal(got, want) {
				t.Errorf("Resolve(%q) requested paths %q, want %q", issuer, got, want)
			}
		})
	}
}
