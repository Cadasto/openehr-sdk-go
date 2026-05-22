package admin_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/cadasto/admin"
	"github.com/cadasto/openehr-sdk-go/smart/discovery"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// newClient builds a transport.Client whose openEHR REST entry points
// at srv.URL with the conventional /openehr/v1 API prefix — the prefix
// the probe MUST strip when targeting deployment-root /health/*.
func newClient(t *testing.T, srv *httptest.Server) *transport.Client {
	t.Helper()
	cat, err := discovery.NewStaticCatalog(discovery.StaticConfig{
		Issuer: "https://test.example.com",
		Services: map[string]discovery.ServiceEntry{
			discovery.ServiceIDOpenEHRRest: {
				BaseURL:     discovery.MustParseURL(srv.URL + "/openehr/v1"),
				SpecVersion: discovery.SpecVersionPin,
			},
		},
	})
	if err != nil {
		t.Fatalf("build catalog: %v", err)
	}
	c, err := transport.New(cat, transport.WithHTTPClient(srv.Client()))
	if err != nil {
		t.Fatalf("build client: %v", err)
	}
	return c
}

// captureHandler records the path each request hits and replies with
// the configured status. Body is irrelevant for probes.
func captureHandler(status int, gotPath *string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		*gotPath = r.URL.Path
		w.WriteHeader(status)
	}
}

func TestLiveDefaultPathAtDeploymentRoot(t *testing.T) {
	// SDK-GAP-07: probes target the deployment origin, NOT the openEHR
	// REST base URL. Catalog base is .../openehr/v1; probe MUST hit
	// /health/live at the same host without the /openehr/v1 prefix.
	var got string
	srv := httptest.NewServer(captureHandler(http.StatusOK, &got))
	defer srv.Close()

	c := newClient(t, srv)
	if err := admin.Live(context.Background(), c); err != nil {
		t.Fatalf("Live: %v", err)
	}
	if got != "/health/live" {
		t.Errorf("probe path = %q, want %q (deployment root, not /openehr/v1/health/live)", got, "/health/live")
	}
}

func TestReadyDefaultPathAtDeploymentRoot(t *testing.T) {
	var got string
	srv := httptest.NewServer(captureHandler(http.StatusOK, &got))
	defer srv.Close()

	c := newClient(t, srv)
	if err := admin.Ready(context.Background(), c); err != nil {
		t.Fatalf("Ready: %v", err)
	}
	if got != "/health/ready" {
		t.Errorf("probe path = %q, want %q", got, "/health/ready")
	}
}

func TestLiveWithLivePathOverride(t *testing.T) {
	var got string
	srv := httptest.NewServer(captureHandler(http.StatusOK, &got))
	defer srv.Close()

	c := newClient(t, srv)
	if err := admin.Live(context.Background(), c, admin.WithLivePath("/healthz")); err != nil {
		t.Fatalf("Live: %v", err)
	}
	if got != "/healthz" {
		t.Errorf("override path = %q, want /healthz", got)
	}
}

func TestReadyWithReadyPathOverride(t *testing.T) {
	var got string
	srv := httptest.NewServer(captureHandler(http.StatusOK, &got))
	defer srv.Close()

	c := newClient(t, srv)
	if err := admin.Ready(context.Background(), c, admin.WithReadyPath("/health/started")); err != nil {
		t.Fatalf("Ready: %v", err)
	}
	if got != "/health/started" {
		t.Errorf("override path = %q, want /health/started", got)
	}
}

// Acceptance criterion: 2xx → nil.
func TestProbe2xxReturnsNil(t *testing.T) {
	cases := []int{200, 201, 204, 299}
	for _, status := range cases {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var got string
			srv := httptest.NewServer(captureHandler(status, &got))
			defer srv.Close()
			c := newClient(t, srv)
			if err := admin.Ready(context.Background(), c); err != nil {
				t.Errorf("status=%d: Ready returned %v, want nil", status, err)
			}
		})
	}
}

// Acceptance criterion: 404 → transport.ErrNotFound.
func TestProbe404ReturnsErrNotFound(t *testing.T) {
	var got string
	srv := httptest.NewServer(captureHandler(http.StatusNotFound, &got))
	defer srv.Close()
	c := newClient(t, srv)
	err := admin.Ready(context.Background(), c)
	if err == nil {
		t.Fatal("expected non-nil error for 404")
	}
	if !errors.Is(err, transport.ErrNotFound) {
		t.Errorf("err = %v, want errors.Is(err, transport.ErrNotFound)", err)
	}
}

// Acceptance criterion: other typed errors per REQ-093.
func TestProbeTypedErrorsByStatus(t *testing.T) {
	cases := []struct {
		status   int
		sentinel error
	}{
		{http.StatusUnauthorized, transport.ErrUnauthorized},
		{http.StatusForbidden, transport.ErrForbidden},
		{http.StatusInternalServerError, transport.ErrServerError},
		{http.StatusServiceUnavailable, transport.ErrServerError},
	}
	for _, tc := range cases {
		t.Run(http.StatusText(tc.status), func(t *testing.T) {
			var got string
			srv := httptest.NewServer(captureHandler(tc.status, &got))
			defer srv.Close()
			c := newClient(t, srv)
			err := admin.Live(context.Background(), c)
			if err == nil {
				t.Fatalf("status=%d: expected error", tc.status)
			}
			if !errors.Is(err, tc.sentinel) {
				t.Errorf("status=%d: err = %v, want errors.Is(err, %v)", tc.status, err, tc.sentinel)
			}
		})
	}
}

// Catalog without openEHR REST entry → ErrServiceUnavailable so callers
// get a typed signal rather than a panic.
func TestProbeMissingOpenEHRRestEntry(t *testing.T) {
	cat, err := discovery.NewStaticCatalog(discovery.StaticConfig{
		Issuer:   "https://test.example.com",
		Services: map[string]discovery.ServiceEntry{},
	})
	if err == nil {
		// NewStaticCatalog may itself reject empty services; tolerate
		// either path — if construction fails we already get a typed
		// error, no further probe call needed.
		c, err := transport.New(cat, transport.WithHTTPClient(http.DefaultClient))
		if err != nil {
			t.Skip("transport.New rejected empty catalog before Live could run")
		}
		err = admin.Live(context.Background(), c)
		if err == nil {
			t.Fatal("expected error when openEHR REST entry missing")
		}
		if !errors.Is(err, transport.ErrServiceUnavailable) {
			t.Errorf("err = %v, want errors.Is(err, transport.ErrServiceUnavailable)", err)
		}
	}
}

// Context cancellation should propagate without leaking the request.
func TestProbeContextCanceled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()
	c := newClient(t, srv)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := admin.Live(ctx, c)
	if err == nil {
		t.Fatal("expected error for canceled context")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}

// Probe URL must derive origin (scheme://host) from the openEHR REST
// entry — proving the path-prefix is stripped, not joined.
func TestProbeURLOriginOnly(t *testing.T) {
	var fullURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fullURL = r.Host + r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	c := newClient(t, srv)
	if err := admin.Ready(context.Background(), c); err != nil {
		t.Fatalf("Ready: %v", err)
	}
	srvURL, _ := url.Parse(srv.URL)
	want := srvURL.Host + "/health/ready"
	if fullURL != want {
		t.Errorf("probe URL = %q, want %q (must NOT contain /openehr/v1)", fullURL, want)
	}
	if strings.Contains(fullURL, "/openehr/v1") {
		t.Errorf("probe URL contains REST API prefix: %q", fullURL)
	}
}
