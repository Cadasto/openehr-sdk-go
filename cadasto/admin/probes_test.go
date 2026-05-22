package admin_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
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
	// strconv.Itoa instead of http.StatusText: the latter returns ""
	// for non-standard codes (e.g. 299), producing useless subtest
	// names like "#00".
	cases := []int{200, 201, 204, 299}
	for _, status := range cases {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
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

// Acceptance criterion: unmapped non-2xx codes (400, 405, 408, 429)
// surface as a plain formatted error WITHOUT a transport sentinel.
// Callers MUST NOT depend on errors.Is matching outside the documented
// mapping (404/401/403/5xx).
func TestProbeUnmapped4xxHasNoSentinel(t *testing.T) {
	cases := []int{http.StatusBadRequest, http.StatusMethodNotAllowed, http.StatusRequestTimeout, http.StatusTooManyRequests}
	for _, status := range cases {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			var got string
			srv := httptest.NewServer(captureHandler(status, &got))
			defer srv.Close()
			c := newClient(t, srv)
			err := admin.Live(context.Background(), c)
			if err == nil {
				t.Fatalf("status=%d: expected error", status)
			}
			// None of the mapped sentinels should match.
			for _, sentinel := range []error{
				transport.ErrNotFound,
				transport.ErrUnauthorized,
				transport.ErrForbidden,
				transport.ErrServerError,
			} {
				if errors.Is(err, sentinel) {
					t.Errorf("status=%d: err matched %v but unmapped codes must not carry a sentinel", status, sentinel)
				}
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
//
// Fails the test (rather than t.Skip) on any setup error: the prior
// shape wrapped the assertion in `if err == nil` after NewStaticCatalog,
// which made the entire test vacuous if catalog construction ever
// changed shape. Now construction failures are real test failures.
func TestProbeMissingOpenEHRRestEntry(t *testing.T) {
	cat, err := discovery.NewStaticCatalog(discovery.StaticConfig{
		Issuer:   "https://test.example.com",
		Services: map[string]discovery.ServiceEntry{},
	})
	if err != nil {
		t.Fatalf("NewStaticCatalog rejected empty services map (setup change?): %v", err)
	}
	c, err := transport.New(cat, transport.WithHTTPClient(http.DefaultClient))
	if err != nil {
		t.Fatalf("transport.New rejected empty-services catalog (setup change?): %v", err)
	}
	err = admin.Live(context.Background(), c)
	if err == nil {
		t.Fatal("expected error when openEHR REST entry missing")
	}
	if !errors.Is(err, transport.ErrServiceUnavailable) {
		t.Errorf("err = %v, want errors.Is(err, transport.ErrServiceUnavailable)", err)
	}
}

// Connection-failure path: server closed before probe issues request
// → hc.Do returns a network error → probe wraps it with the full URL
// via %w so callers can errors.Is / errors.As the underlying error.
func TestProbeConnectionRefused(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	target := srv.URL
	srv.Close() // close before probe — connection will refuse.

	cat, err := discovery.NewStaticCatalog(discovery.StaticConfig{
		Issuer: "https://test.example.com",
		Services: map[string]discovery.ServiceEntry{
			discovery.ServiceIDOpenEHRRest: {
				BaseURL:     discovery.MustParseURL(target + "/openehr/v1"),
				SpecVersion: discovery.SpecVersionPin,
			},
		},
	})
	if err != nil {
		t.Fatalf("NewStaticCatalog: %v", err)
	}
	c, err := transport.New(cat, transport.WithHTTPClient(http.DefaultClient))
	if err != nil {
		t.Fatalf("transport.New: %v", err)
	}
	err = admin.Ready(context.Background(), c)
	if err == nil {
		t.Fatal("expected error for closed server")
	}
	// Error message MUST contain the probe URL so loop-mode callers
	// can identify which endpoint failed without parsing the wrapped
	// error type.
	if !strings.Contains(err.Error(), "/health/ready") {
		t.Errorf("err = %q, want probe URL in message", err)
	}
}

// Path-option validation: empty / non-absolute overrides surface as
// ErrInvalidPath before any network I/O.
func TestProbePathOptionValidation(t *testing.T) {
	cases := []struct {
		name string
		path string
	}{
		{"empty", ""},
		{"no_leading_slash", "health/live"},
		{"relative_dot", "./health/live"},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("path validation should fail before hitting the server, but got request to %s", r.URL.Path)
	}))
	defer srv.Close()
	c := newClient(t, srv)

	for _, tc := range cases {
		t.Run("Live/"+tc.name, func(t *testing.T) {
			err := admin.Live(context.Background(), c, admin.WithLivePath(tc.path))
			if !errors.Is(err, admin.ErrInvalidPath) {
				t.Errorf("path=%q: err = %v, want errors.Is(err, ErrInvalidPath)", tc.path, err)
			}
		})
		t.Run("Ready/"+tc.name, func(t *testing.T) {
			err := admin.Ready(context.Background(), c, admin.WithReadyPath(tc.path))
			if !errors.Is(err, admin.ErrInvalidPath) {
				t.Errorf("path=%q: err = %v, want errors.Is(err, ErrInvalidPath)", tc.path, err)
			}
		})
	}
}

// Header contract: probe sets Accept but never Authorization. An
// injected http.RoundTripper MAY add auth — that is outside the
// probe's contract — but the probe itself MUST NOT attach a token.
func TestProbeSetsAcceptNoAuthorization(t *testing.T) {
	var gotAccept, gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAccept = r.Header.Get("Accept")
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	c := newClient(t, srv)
	if err := admin.Live(context.Background(), c); err != nil {
		t.Fatalf("Live: %v", err)
	}
	if gotAccept != "application/json" {
		t.Errorf("Accept = %q, want application/json", gotAccept)
	}
	if gotAuth != "" {
		t.Errorf("Authorization = %q, want empty (probes do not attach auth)", gotAuth)
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
