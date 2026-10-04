package discovery_test

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"

	"github.com/cadasto/openehr-sdk-go/smart/discovery"
)

// TestResolveStopsOnUnaskedNotModified pins REQ-071: a 304 Not Modified to a
// request that carried no If-None-Match is a failed fetch, so a server that
// always answers 304 ends a resolution with ReasonFetchFailed after at most
// two requests: the conditional one a refresh sends, and one without
// If-None-Match. The server stops answering 304 after 50 requests, so a
// resolver that loops fails the test instead of hanging it.
func TestResolveStopsOnUnaskedNotModified(t *testing.T) { // REQ-071
	const loopCap = 50
	tests := []struct {
		name string
		// warm resolves once against a 200 with an ETag before the server
		// turns to 304, so the second call is a conditional refresh.
		warm       bool
		wantIfNone []string // If-None-Match of the requests after the warm-up
	}{
		{
			name:       "Resolve with nothing cached",
			wantIfNone: []string{""},
		},
		{
			name:       "Refresh of a catalog with an ETag",
			warm:       true,
			wantIfNone: []string{`"v1"`, ""},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var (
				mu     sync.Mutex
				ifNone []string
				warmed = !tc.warm
			)
			srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				defer mu.Unlock()
				if !warmed {
					warmed = true
					w.Header().Set("ETag", `"v1"`)
					_, _ = io.WriteString(w, smartDocument("", ""))
					return
				}
				ifNone = append(ifNone, r.Header.Get("If-None-Match"))
				if len(ifNone) > loopCap {
					w.WriteHeader(http.StatusServiceUnavailable)
					return
				}
				w.WriteHeader(http.StatusNotModified)
			}))
			defer srv.Close()
			res, err := discovery.NewResolver(nil, discovery.WithHTTPClient(srv.Client()))
			if err != nil {
				t.Fatal(err)
			}
			baseURL := srv.URL + platformPath
			if tc.warm {
				if _, err := res.Resolve(t.Context(), baseURL); err != nil {
					t.Fatalf("warm-up Resolve(%q) error = %v", baseURL, err)
				}
				_, err = res.Refresh(t.Context(), baseURL)
			} else {
				_, err = res.Resolve(t.Context(), baseURL)
			}

			derr, ok := errors.AsType[*discovery.DiscoveryError](err)
			if !ok || derr.Reason != discovery.ReasonFetchFailed {
				t.Errorf("error = %v, want a DiscoveryError with Reason %q", err, discovery.ReasonFetchFailed)
			}
			mu.Lock()
			got := slices.Clone(ifNone)
			mu.Unlock()
			if len(got) > 2 {
				t.Fatalf("server received %d requests answered 304, want at most 2", len(got))
			}
			if !slices.Equal(got, tc.wantIfNone) {
				t.Errorf("If-None-Match of the requests = %q, want %q", got, tc.wantIfNone)
			}
		})
	}
}
