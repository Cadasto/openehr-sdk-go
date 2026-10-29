package admin_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cadasto/openehr-sdk-go/auth"
	"github.com/cadasto/openehr-sdk-go/cadasto/admin"
	"github.com/cadasto/openehr-sdk-go/smart/discovery"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// newClient builds a transport.Client whose openEHR REST entry points
// at srv.URL with the conventional /openehr/v1 API prefix — the prefix
// the probe MUST strip when targeting deployment-root /health/*.
func newClient(t *testing.T, srv *httptest.Server) *transport.Client {
	t.Helper()
	return newClientWithHTTP(t, srv, srv.Client())
}

// testToken is the bearer token a client from newTokenClient carries.
const testToken = "probe-must-not-send"

// newTokenClient is newClient with a static token source, so a request
// that attached the client's credential would carry testToken. A check
// that a probe sends no Authorization header needs it: on a client
// without a token source, attaching the credential sends nothing.
func newTokenClient(t *testing.T, srv *httptest.Server) *transport.Client {
	t.Helper()
	return newClientWithHTTP(t, srv, srv.Client(),
		transport.WithTokenSource(auth.StaticTokenSource(auth.Token{Value: testToken})))
}

// newClientWithHTTP is newClient with hc as the HTTP client, so a test
// can wrap the server's own transport. opts follow WithHTTPClient.
func newClientWithHTTP(t *testing.T, srv *httptest.Server, hc *http.Client, opts ...transport.Option) *transport.Client {
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
	c, err := transport.New(cat, append([]transport.Option{transport.WithHTTPClient(hc)}, opts...)...)
	if err != nil {
		t.Fatalf("build client: %v", err)
	}
	return c
}

// TestTokenClientSendsAuthorization is the control for the checks that a
// probe sends no Authorization header: a request through transport.Do on
// the same client does send the token.
func TestTokenClientSendsAuthorization(t *testing.T) { // REQ-083
	gotAuth := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth <- r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	if _, err := newTokenClient(t, srv).Do(t.Context(), &transport.Request{Method: http.MethodGet, Path: "/ehr"}); err != nil {
		t.Fatalf("Do: %v", err)
	}
	if got, want := <-gotAuth, "Bearer "+testToken; got != want {
		t.Errorf("Authorization = %q, want %q", got, want)
	}
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
	// REQ-083: probes target the deployment origin, NOT the openEHR
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
// → hc.Do returns a network error → probe wraps it so callers can
// errors.Is / errors.AsType the underlying error, and its text names the
// probe path.
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
	// The error message must contain the probe path so loop-mode callers
	// can identify which endpoint failed without parsing the wrapped
	// error type.
	if !strings.Contains(err.Error(), "/health/ready") {
		t.Errorf("err = %q, want probe path in message", err)
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

// Header contract: probe sets Accept but never Authorization, though the
// client carries a token source. An injected http.RoundTripper MAY add
// auth — that is outside the probe's contract — but the probe itself
// MUST NOT attach a token.
func TestProbeSetsAcceptNoAuthorization(t *testing.T) { // REQ-083
	var gotAccept, gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAccept = r.Header.Get("Accept")
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	c := newTokenClient(t, srv)
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

// TestNilClientProbesDoNotPanic pins the REQ-025 guard on the probe's
// deliberate transport.Do bypass: Live/Ready read the catalog directly,
// so the central nil-Client guard in Do never runs for this leaf and the
// probe must refuse a nil client itself.
func TestNilClientProbesDoNotPanic(t *testing.T) {
	cases := []struct {
		name string
		call func() error
	}{
		{"Live", func() error { return admin.Live(t.Context(), nil) }},
		{"Ready", func() error { return admin.Ready(t.Context(), nil) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("%s with a nil client panicked: %v", tc.name, r)
				}
			}()
			if err := tc.call(); !errors.Is(err, transport.ErrInvalidConfig) {
				t.Errorf("err = %v; want ErrInvalidConfig", err)
			}
		})
	}
}

// readyDoc is a readiness document that names the service and its
// version.
const readyDoc = `{"status":"UP","version":"1.2.3"}`

// readySecret stands for a value the server sends. It must never reach
// an error's text.
const readySecret = "PATIENT-SECRET-4711"

// secretDoc is a readiness document that carries readySecret.
const secretDoc = `{"status":"UP","note":"` + readySecret + `"}`

// bodyHandler answers every request with status and body.
func bodyHandler(status int, body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}
}

// rawHandler takes over the connection, writes raw as the whole
// response and closes the connection. It lets a test send what
// net/http's server would not write: a body shorter than its
// Content-Length, or a malformed chunked trailer.
func rawHandler(t *testing.T, raw string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Error("the test server's ResponseWriter is not an http.Hijacker; the premise of this test is gone")
			return
		}
		conn, buf, err := hj.Hijack()
		if err != nil {
			t.Errorf("Hijack: %v", err)
			return
		}
		defer func() { _ = conn.Close() }()
		_, _ = buf.WriteString(raw)
		_ = buf.Flush()
	}
}

// assertNoServerValues fails the test when msg carries readySecret or
// any of the other bytes the server sent.
func assertNoServerValues(t *testing.T, msg string, sent ...string) {
	t.Helper()
	for _, v := range append([]string{readySecret}, sent...) {
		if strings.Contains(msg, v) {
			t.Errorf("error text %q carries %q, which the server sent", msg, v)
		}
	}
}

// TestReadyBodyReturnsBody pins the readiness body: a 2xx from the
// default readiness path returns exactly the bytes the server sent.
func TestReadyBodyReturnsBody(t *testing.T) { // REQ-083
	paths := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths <- r.URL.Path
		bodyHandler(http.StatusOK, readyDoc)(w, r)
	}))
	defer srv.Close()

	got, err := admin.ReadyBody(t.Context(), newClient(t, srv))
	if err != nil {
		t.Fatalf("ReadyBody: %v", err)
	}
	if string(got) != readyDoc {
		t.Errorf("ReadyBody = %q, want %q", got, readyDoc)
	}
	if p := <-paths; p != admin.DefaultReadyPath {
		t.Errorf("probe path = %q, want %q", p, admin.DefaultReadyPath)
	}
}

// TestReadyBodySendsReadyRequest pins the request ReadyBody sends: a GET
// to the WithReadyPath override at the origin of the openEHR REST entry,
// with Accept: application/json and no Authorization header, though the
// client carries a token source.
func TestReadyBodySendsReadyRequest(t *testing.T) { // REQ-083
	type seen struct {
		method, host, path, accept string
		auth                       bool
	}
	reqs := make(chan seen, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, auth := r.Header["Authorization"]
		reqs <- seen{method: r.Method, host: r.Host, path: r.URL.Path, accept: r.Header.Get("Accept"), auth: auth}
		bodyHandler(http.StatusOK, readyDoc)(w, r)
	}))
	defer srv.Close()
	srvURL, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse server URL: %v", err)
	}

	got, err := admin.ReadyBody(t.Context(), newTokenClient(t, srv), admin.WithReadyPath("/ehrbase/rest/status"))
	if err != nil {
		t.Fatalf("ReadyBody: %v", err)
	}
	if string(got) != readyDoc {
		t.Errorf("ReadyBody = %q, want %q", got, readyDoc)
	}
	want := seen{method: http.MethodGet, host: srvURL.Host, path: "/ehrbase/rest/status", accept: "application/json"}
	if r := <-reqs; r != want {
		t.Errorf("request = %+v, want %+v (the override path at the origin, no /openehr/v1, no Authorization)", r, want)
	}
}

// TestReadyBodyLimit pins the bounded read: a body of exactly the limit
// is returned whole, one byte more returns no bytes and an error that
// matches transport.ErrResponseTooLarge and names the limit. 0 selects
// DefaultMaxReadyBody, a negative limit reads without one, and
// math.MaxInt64 reads every body whole.
func TestReadyBodyLimit(t *testing.T) { // REQ-083
	n := int64(len(secretDoc))
	atDefault := strings.Repeat("x", int(admin.DefaultMaxReadyBody))
	cases := []struct {
		name  string
		body  string
		opts  []admin.ReadyOption
		limit int64 // the limit the error names; 0 when the body is returned whole
	}{
		{name: "exactly n", body: secretDoc, opts: []admin.ReadyOption{admin.WithMaxReadyBody(n)}},
		{name: "one byte over n", body: secretDoc + " ", opts: []admin.ReadyOption{admin.WithMaxReadyBody(n)}, limit: n},
		{name: "exactly the default", body: atDefault},
		{name: "one byte over the default", body: atDefault + "x", limit: admin.DefaultMaxReadyBody},
		{name: "zero selects the default", body: atDefault + "x", opts: []admin.ReadyOption{admin.WithMaxReadyBody(0)}, limit: admin.DefaultMaxReadyBody},
		{name: "negative reads without a limit", body: atDefault + "x", opts: []admin.ReadyOption{admin.WithMaxReadyBody(-1)}},
		{name: "MaxInt64 reads the body whole", body: secretDoc, opts: []admin.ReadyOption{admin.WithMaxReadyBody(math.MaxInt64)}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(bodyHandler(http.StatusOK, tc.body))
			defer srv.Close()

			got, err := admin.ReadyBody(t.Context(), newClient(t, srv), tc.opts...)
			if tc.limit == 0 {
				if err != nil {
					t.Fatalf("ReadyBody: %v", err)
				}
				if string(got) != tc.body {
					t.Errorf("ReadyBody returned %d bytes, want the %d-byte body whole", len(got), len(tc.body))
				}
				return
			}
			if got != nil {
				t.Errorf("ReadyBody returned %d bytes beside the error, want none: a partial body is not the document", len(got))
			}
			if !errors.Is(err, transport.ErrResponseTooLarge) {
				t.Fatalf("err = %v, want errors.Is(err, transport.ErrResponseTooLarge)", err)
			}
			if limit := strconv.FormatInt(tc.limit, 10); !strings.Contains(err.Error(), limit) {
				t.Errorf("err = %q, want it to name the limit %s", err, limit)
			}
			assertNoServerValues(t, err.Error())
		})
	}
}

// countingTransport counts the response body bytes its caller reads.
type countingTransport struct {
	base http.RoundTripper
	read int64
}

func (ct *countingTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	resp, err := ct.base.RoundTrip(r)
	if err != nil {
		return nil, err
	}
	resp.Body = &countingBody{ReadCloser: resp.Body, read: &ct.read}
	return resp, nil
}

type countingBody struct {
	io.ReadCloser
	read *int64
}

func (b *countingBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	*b.read += int64(n)
	return n, err
}

// TestReadyBodyStopsReadingPastLimit pins the cap on the bytes read, not
// only the check on their count: ReadyBody stops one byte past the limit
// instead of reading a long body to its end.
func TestReadyBodyStopsReadingPastLimit(t *testing.T) { // REQ-083
	const limit = 1024
	srv := httptest.NewServer(bodyHandler(http.StatusOK, strings.Repeat("x", 64*limit)))
	defer srv.Close()
	ct := &countingTransport{base: srv.Client().Transport}

	got, err := admin.ReadyBody(t.Context(), newClientWithHTTP(t, srv, &http.Client{Transport: ct}), admin.WithMaxReadyBody(limit))
	if !errors.Is(err, transport.ErrResponseTooLarge) {
		t.Fatalf("err = %v, want errors.Is(err, transport.ErrResponseTooLarge)", err)
	}
	if got != nil {
		t.Errorf("ReadyBody returned %d bytes beside the error, want none", len(got))
	}
	if ct.read > limit+1 {
		t.Errorf("ReadyBody read %d body bytes under a limit of %d, want at most %d", ct.read, limit, limit+1)
	}
}

// drainCap is the most bytes a probe reads from a body it does not return,
// as the package doc states: 256 KiB.
const drainCap = 256 << 10

// endlessHandler answers with status and a body that never ends. It writes
// until a write fails or the request's context ends, which both happen once
// the client closes the connection.
func endlessHandler(status int) http.HandlerFunc {
	chunk := []byte(strings.Repeat("x", 32<<10))
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		for r.Context().Err() == nil {
			if _, err := w.Write(chunk); err != nil {
				return
			}
		}
	}
}

// probeCall runs one probe and returns the bytes it returns: none for Live
// and Ready.
type probeCall func(ctx context.Context, c *transport.Client) ([]byte, error)

func callLive(ctx context.Context, c *transport.Client) ([]byte, error) {
	return nil, admin.Live(ctx, c)
}

func callReady(ctx context.Context, c *transport.Client) ([]byte, error) {
	return nil, admin.Ready(ctx, c)
}

func callReadyBody(ctx context.Context, c *transport.Client) ([]byte, error) {
	return admin.ReadyBody(ctx, c)
}

// TestProbesBoundEndlessBodyDrain pins the bounded drain: a 2xx body that
// never ends makes Live and Ready return nil, and a 503 body that never
// ends makes ReadyBody return no bytes and an error matching
// transport.ErrServerError, each after reading at most drainCap bytes of
// it. The counter wraps the body the probe reads, above net/http's own
// buffered reader, so it counts only the bytes handed to the probe and the
// bound needs no allowance. The deadline ends a drain without a cap, so
// such a drain fails on the byte count instead of hanging the suite.
func TestProbesBoundEndlessBodyDrain(t *testing.T) { // REQ-083
	cases := []struct {
		name     string
		status   int
		call     probeCall
		sentinel error // nil when the probe must return nil
	}{
		{name: "Live 200", status: http.StatusOK, call: callLive},
		{name: "Ready 200", status: http.StatusOK, call: callReady},
		{name: "ReadyBody 503", status: http.StatusServiceUnavailable, call: callReadyBody, sentinel: transport.ErrServerError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(endlessHandler(tc.status))
			defer srv.Close()
			ct := &countingTransport{base: srv.Client().Transport}
			c := newClientWithHTTP(t, srv, &http.Client{Transport: ct})
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()

			got, err := tc.call(ctx, c)
			if got != nil {
				t.Errorf("%s returned %d bytes, want none", tc.name, len(got))
			}
			switch {
			case tc.sentinel == nil && err != nil:
				t.Errorf("%s = %v, want nil", tc.name, err)
			case tc.sentinel != nil && !errors.Is(err, tc.sentinel):
				t.Errorf("%s err = %v, want errors.Is(err, %v)", tc.name, err, tc.sentinel)
			}
			if ct.read > drainCap {
				t.Errorf("%s read %d bytes of a body that never ends, want at most %d", tc.name, ct.read, drainCap)
			}
		})
	}
}

// trickleHandler answers with status and a body that never ends, sent one
// byte at a time with a pause after each. It stops once a write fails or
// the request's context ends, which both happen once the client drops the
// connection.
func trickleHandler(status int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		rc := http.NewResponseController(w)
		for {
			if _, err := io.WriteString(w, "x"); err != nil {
				return
			}
			if err := rc.Flush(); err != nil {
				return
			}
			select {
			case <-r.Context().Done():
				return
			case <-time.After(10 * time.Millisecond):
			}
		}
	}
}

// stallHandler answers with status and one byte of body, then sends nothing
// more until the request's context ends, which happens once the client
// drops the connection.
func stallHandler(status int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		_, _ = io.WriteString(w, "x")
		_ = http.NewResponseController(w).Flush()
		<-r.Context().Done()
	}
}

// TestProbesBoundSlowBodyDrain pins the time bound on the drain: a 2xx body
// that trickles without end, or sends a byte and stalls, makes Live and
// Ready return nil, and such a 503 body makes ReadyBody return no bytes and
// an error matching transport.ErrServerError, each before its context
// ends. The deadline is far above the time bound, so a drain bounded only
// in bytes holds the probe until the deadline and fails on the context
// check.
func TestProbesBoundSlowBodyDrain(t *testing.T) { // REQ-083
	bodies := []struct {
		name    string
		handler func(status int) http.HandlerFunc
	}{
		{name: "trickle", handler: trickleHandler},
		{name: "stall", handler: stallHandler},
	}
	probes := []struct {
		name     string
		status   int
		call     probeCall
		sentinel error // nil when the probe must return nil
	}{
		{name: "Live 200", status: http.StatusOK, call: callLive},
		{name: "Ready 200", status: http.StatusOK, call: callReady},
		{name: "ReadyBody 503", status: http.StatusServiceUnavailable, call: callReadyBody, sentinel: transport.ErrServerError},
	}
	for _, body := range bodies {
		for _, tc := range probes {
			t.Run(body.name+"/"+tc.name, func(t *testing.T) {
				srv := httptest.NewServer(body.handler(tc.status))
				defer srv.Close()
				ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
				defer cancel()

				got, err := tc.call(ctx, newClient(t, srv))
				if ctxErr := ctx.Err(); ctxErr != nil {
					t.Errorf("%s on a %s body returned after its context ended (%v), want it to return before", tc.name, body.name, ctxErr)
				}
				if got != nil {
					t.Errorf("%s on a %s body returned %d bytes, want none", tc.name, body.name, len(got))
				}
				switch {
				case tc.sentinel == nil && err != nil:
					t.Errorf("%s on a %s body = %v, want nil", tc.name, body.name, err)
				case tc.sentinel != nil && !errors.Is(err, tc.sentinel):
					t.Errorf("%s on a %s body: err = %v, want errors.Is(err, %v)", tc.name, body.name, err, tc.sentinel)
				}
			})
		}
	}
}

// TestReadyBodySlowBodyReturnedWhole pins that the drain's time bound stays
// off the body ReadyBody returns: a 2xx body that arrives one byte at a
// time, over several times the drain's 50 ms, still comes back whole.
func TestReadyBodySlowBodyReturnedWhole(t *testing.T) { // REQ-083
	const body = `{"status":"UP"}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rc := http.NewResponseController(w)
		for i := range len(body) {
			if _, err := io.WriteString(w, body[i:i+1]); err != nil {
				return
			}
			if err := rc.Flush(); err != nil {
				return
			}
			select {
			case <-r.Context().Done():
				return
			case <-time.After(20 * time.Millisecond):
			}
		}
	}))
	defer srv.Close()

	got, err := admin.ReadyBody(t.Context(), newClient(t, srv))
	if err != nil {
		t.Fatalf("ReadyBody on a slow body: %v, want nil", err)
	}
	if string(got) != body {
		t.Errorf("ReadyBody on a slow body = %q, want %q whole", got, body)
	}
}

// TestProbesDrainSmallBodyAndReuseConnection pins the drain and what it is
// for: a probe reads a small body to its end before it closes it, and the
// next probe reuses the connection. The byte count is what fails when the
// drain is skipped. net/http's client itself drains a body closed before
// its end, for a short time and below the counter, so a skipped drain can
// still leave the connection reused.
func TestProbesDrainSmallBodyAndReuseConnection(t *testing.T) { // REQ-083
	cases := []struct {
		name   string
		status int
		call   probeCall
	}{
		{name: "Live 200", status: http.StatusOK, call: callLive},
		{name: "Ready 200", status: http.StatusOK, call: callReady},
		{name: "ReadyBody 200", status: http.StatusOK, call: callReadyBody},
		{name: "Live 503", status: http.StatusServiceUnavailable, call: callLive},
		{name: "Ready 503", status: http.StatusServiceUnavailable, call: callReady},
		{name: "ReadyBody 503", status: http.StatusServiceUnavailable, call: callReadyBody},
	}
	const calls = 3
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var conns atomic.Int64
			srv := httptest.NewUnstartedServer(bodyHandler(tc.status, secretDoc))
			srv.Config.ConnState = func(_ net.Conn, s http.ConnState) {
				if s == http.StateNew {
					conns.Add(1)
				}
			}
			srv.Start()
			defer srv.Close()
			ct := &countingTransport{base: srv.Client().Transport}
			c := newClientWithHTTP(t, srv, &http.Client{Transport: ct})

			for range calls {
				_, err := tc.call(t.Context(), c)
				if wantErr := tc.status != http.StatusOK; (err != nil) != wantErr {
					t.Fatalf("%s: err = %v, want an error = %t", tc.name, err, wantErr)
				}
			}
			if want := int64(calls * len(secretDoc)); ct.read != want {
				t.Errorf("%d calls of %s on a %d-byte body read %d bytes, want %d: each body read to its end", calls, tc.name, len(secretDoc), ct.read, want)
			}
			if n := conns.Load(); n != 1 {
				t.Errorf("%d calls of %s on a %d-byte body opened %d connections, want 1", calls, tc.name, len(secretDoc), n)
			}
		})
	}
}

// TestReadyIgnoresMaxReadyBody pins that WithMaxReadyBody does not change
// Ready, which reads no body it returns.
func TestReadyIgnoresMaxReadyBody(t *testing.T) { // REQ-083
	srv := httptest.NewServer(bodyHandler(http.StatusOK, secretDoc))
	defer srv.Close()
	if err := admin.Ready(t.Context(), newClient(t, srv), admin.WithMaxReadyBody(1)); err != nil {
		t.Errorf("Ready with WithMaxReadyBody(1) = %v, want nil", err)
	}
}

// TestReadyBodyCutMidBody pins the read failure of a 2xx body cut
// mid-read: no bytes, an error that matches io.ErrUnexpectedEOF, and none
// of the bytes read before the cut in the error text.
func TestReadyBodyCutMidBody(t *testing.T) { // REQ-083
	partial := `{"status":"UP","note":"` + readySecret
	srv := httptest.NewServer(rawHandler(t, "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: 100\r\n\r\n"+partial))
	defer srv.Close()

	got, err := admin.ReadyBody(t.Context(), newClient(t, srv))
	if got != nil {
		t.Errorf("ReadyBody returned %q beside the error, want no bytes: a partial body is not the document", got)
	}
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("err = %v, want errors.Is(err, io.ErrUnexpectedEOF)", err)
	}
	assertNoServerValues(t, err.Error(), partial, `"status":"UP"`)
}

// TestReadyBodyTrailerTextStaysOut pins the value-free text on the read
// failure's own text: net/http quotes a malformed chunked trailer line in
// its read error, so a server can put any value there. That value
// appears nowhere in the error text, and the read failure is still
// reached by unwrapping.
func TestReadyBodyTrailerTextStaysOut(t *testing.T) { // REQ-083
	srv := httptest.NewServer(rawHandler(t, "HTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\n\r\n5\r\nhello\r\n0\r\n"+readySecret+"\r\n\r\n"))
	defer srv.Close()

	got, err := admin.ReadyBody(t.Context(), newClient(t, srv))
	cause, ok := errors.AsType[textproto.ProtocolError](err)
	if !ok || !strings.Contains(cause.Error(), readySecret) {
		t.Fatalf("errors.AsType[textproto.ProtocolError](%v) = (%v, %v), want net/http's read error quoting %q; the premise of this test is gone", err, cause, ok, readySecret)
	}
	if got != nil {
		t.Errorf("ReadyBody returned %q beside the error, want no bytes", got)
	}
	assertNoServerValues(t, err.Error(), "hello")
}

// cancelAfterHeaders cancels a context once the response headers have
// arrived, so the cancellation lands while the body is being read.
type cancelAfterHeaders struct {
	base   http.RoundTripper
	cancel context.CancelFunc
}

func (ct cancelAfterHeaders) RoundTrip(r *http.Request) (*http.Response, error) {
	resp, err := ct.base.RoundTrip(r)
	ct.cancel()
	return resp, err
}

// TestReadyBodyCanceledMidBody pins that a read failure other than a cut
// connection is reached by unwrapping too: a context canceled while the
// body is read gives no bytes and an error matching context.Canceled.
func TestReadyBodyCanceledMidBody(t *testing.T) { // REQ-083
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "100")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"status":"UP","note":"`+readySecret)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		<-r.Context().Done()
	}))
	defer srv.Close()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	hc := &http.Client{Transport: cancelAfterHeaders{base: srv.Client().Transport, cancel: cancel}}

	got, err := admin.ReadyBody(ctx, newClientWithHTTP(t, srv, hc))
	if got != nil {
		t.Errorf("ReadyBody returned %q beside the error, want no bytes", got)
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want errors.Is(err, context.Canceled)", err)
	}
	assertNoServerValues(t, err.Error())
}

// TestReadyBodyNon2xxMatchesReady pins the status mapping: a status that
// is not 2xx returns no bytes, even when the server sends a body, and the
// same error Ready returns for that status.
func TestReadyBodyNon2xxMatchesReady(t *testing.T) { // REQ-083
	cases := []struct {
		status   int
		sentinel error // nil for a status outside the mapped set
	}{
		{http.StatusServiceUnavailable, transport.ErrServerError},
		{http.StatusInternalServerError, transport.ErrServerError},
		{http.StatusNotFound, transport.ErrNotFound},
		{http.StatusUnauthorized, transport.ErrUnauthorized},
		{http.StatusForbidden, transport.ErrForbidden},
		{http.StatusBadRequest, nil},
		{http.StatusTooManyRequests, nil},
	}
	for _, tc := range cases {
		t.Run(strconv.Itoa(tc.status), func(t *testing.T) {
			srv := httptest.NewServer(bodyHandler(tc.status, secretDoc))
			defer srv.Close()
			c := newClient(t, srv)

			got, err := admin.ReadyBody(t.Context(), c)
			if got != nil {
				t.Errorf("status=%d: ReadyBody returned %q, want no bytes", tc.status, got)
			}
			if err == nil {
				t.Fatalf("status=%d: ReadyBody returned no error", tc.status)
			}
			if tc.sentinel != nil && !errors.Is(err, tc.sentinel) {
				t.Errorf("status=%d: err = %v, want errors.Is(err, %v)", tc.status, err, tc.sentinel)
			}
			readyErr := admin.Ready(t.Context(), c)
			if readyErr == nil || err.Error() != readyErr.Error() {
				t.Errorf("status=%d: ReadyBody err = %v, Ready err = %v, want the same error", tc.status, err, readyErr)
			}
			assertNoServerValues(t, err.Error())
		})
	}
}

// TestReadyBodyPathOptionValidation pins Ready's path guard on ReadyBody:
// an empty or relative override returns ErrInvalidPath and no bytes
// before any request is sent.
func TestReadyBodyPathOptionValidation(t *testing.T) { // REQ-083
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("path validation should fail before hitting the server, but got request to %s", r.URL.Path)
	}))
	defer srv.Close()
	c := newClient(t, srv)

	for _, path := range []string{"", "health/ready", "./health/ready"} {
		t.Run(strconv.Quote(path), func(t *testing.T) {
			got, err := admin.ReadyBody(t.Context(), c, admin.WithReadyPath(path))
			if got != nil {
				t.Errorf("path=%q: ReadyBody returned %q, want no bytes", path, got)
			}
			if !errors.Is(err, admin.ErrInvalidPath) {
				t.Errorf("path=%q: err = %v, want errors.Is(err, ErrInvalidPath)", path, err)
			}
		})
	}
}

// TestReadyBodyNilClient pins the nil-Client guard on ReadyBody, which
// bypasses transport.Client.Do like Ready does: no panic, no bytes and an
// error matching transport.ErrInvalidConfig.
func TestReadyBodyNilClient(t *testing.T) { // REQ-083 REQ-025
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("ReadyBody with a nil client panicked: %v", r)
		}
	}()
	got, err := admin.ReadyBody(t.Context(), nil)
	if got != nil {
		t.Errorf("ReadyBody returned %q, want no bytes", got)
	}
	if !errors.Is(err, transport.ErrInvalidConfig) {
		t.Errorf("err = %v, want errors.Is(err, transport.ErrInvalidConfig)", err)
	}
}

// TestProbeRequestFailureTextStaysOut pins the value-free text on a
// probe's request failure: net/http quotes a malformed response header
// line in its own error, so a server can put any value there, and the
// *url.Error around it prints the URL a second time. The probe's error
// names the probe path once and carries neither the value nor the
// server's address, while the cause is still reached by unwrapping.
func TestProbeRequestFailureTextStaysOut(t *testing.T) { // REQ-083
	srv := httptest.NewServer(rawHandler(t, "HTTP/1.1 200 OK\r\nX-Bad "+readySecret+"\r\n\r\n"))
	defer srv.Close()
	srvURL, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse test server URL: %v", err)
	}

	cases := []struct {
		name string
		path string
		call probeCall
	}{
		{"Live", admin.DefaultLivePath, callLive},
		{"Ready", admin.DefaultReadyPath, callReady},
		{"ReadyBody", admin.DefaultReadyPath, callReadyBody},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.call(t.Context(), newClient(t, srv))
			if got != nil {
				t.Errorf("%s returned %q beside the error, want no bytes", tc.name, got)
			}
			cause, ok := errors.AsType[*url.Error](err)
			if !ok || cause == nil {
				t.Fatalf("errors.AsType[*url.Error](%v) found nothing, want the request failure reached by unwrapping", err)
			}
			if !strings.Contains(cause.Error(), readySecret) {
				t.Fatalf("*url.Error %q does not quote %q; the premise of this test is gone", cause, readySecret)
			}
			msg := err.Error()
			assertNoServerValues(t, msg)
			if strings.Contains(msg, srvURL.Host) {
				t.Errorf("error text %q carries the server's address %q, want the probe path alone", msg, srvURL.Host)
			}
			if n := strings.Count(msg, tc.path); n != 1 {
				t.Errorf("error text %q names the probe path %q %d time(s), want once", msg, tc.path, n)
			}
			if want := tc.path + ": request failed"; !strings.HasSuffix(msg, want) {
				t.Errorf("error text %q, want it to end in %q", msg, want)
			}
		})
	}
}

// failingTransport fails every request with err before sending a byte.
type failingTransport struct{ err error }

func (ft failingTransport) RoundTrip(*http.Request) (*http.Response, error) { return nil, ft.err }

// transportTimeout is a failure that reports itself as a timeout, as a
// dial or TLS handshake timeout does, without matching
// context.DeadlineExceeded. Its text carries readySecret.
type transportTimeout struct{}

func (transportTimeout) Error() string { return "i/o timeout " + readySecret }
func (transportTimeout) Timeout() bool { return true }

// TestProbeRequestFailureWord pins the fixed word a probe's request
// failure ends in: its text says whether the request timed out or was
// canceled, without quoting the failure, whose text here carries
// readySecret. Any other failure reads "request failed", which
// TestProbeRequestFailureTextStaysOut pins.
func TestProbeRequestFailureWord(t *testing.T) { // REQ-083
	stall := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer stall.Close()
	canceled, cancel := context.WithCancel(t.Context())
	cancel()

	cases := []struct {
		name string
		ctx  context.Context
		hc   *http.Client
		want string
	}{
		{
			name: "context already canceled", ctx: canceled, hc: stall.Client(),
			want: "request canceled",
		},
		{
			name: "server stalls past the client timeout", ctx: t.Context(),
			hc:   &http.Client{Transport: stall.Client().Transport, Timeout: 50 * time.Millisecond},
			want: "request timed out",
		},
		{
			name: "transport reports a timeout", ctx: t.Context(),
			hc:   &http.Client{Transport: failingTransport{err: transportTimeout{}}},
			want: "request timed out",
		},
		{
			name: "transport fails past a deadline", ctx: t.Context(),
			hc:   &http.Client{Transport: failingTransport{err: fmt.Errorf("dial %s: %w", readySecret, context.DeadlineExceeded)}},
			want: "request timed out",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := admin.Live(tc.ctx, newClientWithHTTP(t, stall, tc.hc))
			if err == nil {
				t.Fatal("Live = nil, want a request failure")
			}
			msg := err.Error()
			if want := admin.DefaultLivePath + ": " + tc.want; !strings.HasSuffix(msg, want) {
				t.Errorf("Live error text %q, want it to end in %q", msg, want)
			}
			assertNoServerValues(t, msg)
		})
	}
}
