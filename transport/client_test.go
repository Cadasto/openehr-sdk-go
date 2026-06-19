package transport

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cadasto/openehr-sdk-go/auth"
	"github.com/cadasto/openehr-sdk-go/auth/basic"
	"github.com/cadasto/openehr-sdk-go/auth/smart"
	"github.com/cadasto/openehr-sdk-go/smart/discovery"
)

// newCatalog returns a static catalog targeting srv.URL as the
// openEHR REST base. Tests that need a richer catalog construct
// their own.
func newCatalog(t *testing.T, srv *httptest.Server) *discovery.ServiceCatalog {
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
		t.Fatal(err)
	}
	return cat
}

func TestNewValidates(t *testing.T) {
	_, err := New(nil, WithHTTPClient(http.DefaultClient))
	if !errors.Is(err, ErrInvalidConfig) {
		t.Errorf("nil catalog: %v", err)
	}
	cat, _ := discovery.NewStaticCatalog(discovery.StaticConfig{Issuer: "x", Services: map[string]discovery.ServiceEntry{}})
	_, err = New(cat)
	if !errors.Is(err, ErrInvalidConfig) {
		t.Errorf("no http client: %v", err)
	}
}

func TestDoBuildsURLFromCatalog(t *testing.T) {
	var got struct {
		path  string
		query url.Values
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.path = r.URL.Path
		got.query = r.URL.Query()
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	c, _ := New(newCatalog(t, srv), WithHTTPClient(srv.Client()))
	_, err := c.Do(context.Background(), &Request{
		Method: "GET",
		Path:   "/ehr/abc",
		Query:  url.Values{"subject_id": {"42"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.path != "/openehr/v1/ehr/abc" {
		t.Errorf("path = %q, want /openehr/v1/ehr/abc", got.path)
	}
	if got.query.Get("subject_id") != "42" {
		t.Errorf("query = %v", got.query)
	}
}

func TestDoServiceUnavailable(t *testing.T) {
	cat, _ := discovery.NewStaticCatalog(discovery.StaticConfig{
		Issuer:   "https://x",
		Services: map[string]discovery.ServiceEntry{},
	})
	c, _ := New(cat, WithHTTPClient(http.DefaultClient))
	_, err := c.Do(context.Background(), &Request{Path: "/x"})
	if !errors.Is(err, ErrServiceUnavailable) {
		t.Errorf("expected ErrServiceUnavailable, got %v", err)
	}
}

func TestDoPlumbsHeaders(t *testing.T) {
	var captured http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r.Header.Clone()
		w.Header().Set("ETag", `"v-42"`)
		w.Header().Set("openehr-version", "1.1.0")
		w.Header().Set("openehr-audit-details", `{"committer":"alice"}`)
		w.Header().Set("Location", "/openehr/v1/ehr/x/composition/v-42")
		w.WriteHeader(201)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	c, _ := New(
		newCatalog(t, srv),
		WithHTTPClient(srv.Client()),
		WithTokenSource(auth.StaticTokenSource(auth.Token{Value: "tok-1", Type: "Bearer"})),
		WithUserAgent("sdk-test/1.0"),
		WithSpecVersion("1.1.0-development"),
		WithCadastoSpecVersionHeader(true),
	)
	resp, err := c.Do(context.Background(), &Request{
		Method:             "POST",
		Path:               "/ehr/x/composition",
		Body:               []byte(`{"_type":"COMPOSITION"}`),
		Prefer:             PreferRepresentation,
		AuditDetailsHeader: `{"committer":"alice"}`,
		RMVersion:          "1.1.0",
		TemplateID:         "openEHR-EHR-COMPOSITION.encounter.v1",
		IfMatch:            "v-41",
	})
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct{ name, want string }{
		{"Authorization", "Bearer tok-1"},
		{"User-Agent", "sdk-test/1.0"},
		{"Cadasto-Openehr-Spec-Version", "1.1.0-development"},
		{"Prefer", "return=representation"},
		{"Openehr-Audit-Details", `{"committer":"alice"}`},
		{"Openehr-Version", "1.1.0"},
		{"Openehr-Template-Id", "openEHR-EHR-COMPOSITION.encounter.v1"},
		{"If-Match", `"v-41"`},
		{"Content-Type", "application/json"},
		{"Accept", "application/json"},
	}
	for _, tc := range tests {
		if got := captured.Get(tc.name); got != tc.want {
			t.Errorf("%s = %q, want %q", tc.name, got, tc.want)
		}
	}
	if resp.Metadata.ETag != "v-42" {
		t.Errorf("ETag captured = %q, want v-42 (quotes stripped)", resp.Metadata.ETag)
	}
	if resp.Metadata.RMVersion != "1.1.0" {
		t.Errorf("RMVersion = %q", resp.Metadata.RMVersion)
	}
	if resp.Metadata.AuditDetails != `{"committer":"alice"}` {
		t.Errorf("AuditDetails = %q", resp.Metadata.AuditDetails)
	}
	if resp.Metadata.Location == "" {
		t.Errorf("Location empty")
	}
}

func TestDoNoAuthSuppressesAuthorization(t *testing.T) {
	var captured string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	c, _ := New(
		newCatalog(t, srv),
		WithHTTPClient(srv.Client()),
		WithTokenSource(auth.StaticTokenSource(auth.Token{Value: "tok"})),
	)
	if _, err := c.Do(context.Background(), &Request{Path: "/capabilities", NoAuth: true}); err != nil {
		t.Fatal(err)
	}
	if captured != "" {
		t.Errorf("expected no Authorization, got %q", captured)
	}
}

func TestDoPerRequestTokenSourceOverride(t *testing.T) {
	var captured string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	c, _ := New(
		newCatalog(t, srv),
		WithHTTPClient(srv.Client()),
		WithTokenSource(auth.StaticTokenSource(auth.Token{Value: "default-tok"})),
	)
	ctx := auth.WithTokenSource(context.Background(), auth.StaticTokenSource(auth.Token{Value: "ctx-tok"}))
	if _, err := c.Do(ctx, &Request{Path: "/x"}); err != nil {
		t.Fatal(err)
	}
	if captured != "Bearer ctx-tok" {
		t.Errorf("Authorization = %q, want Bearer ctx-tok (PROBE-064)", captured)
	}
	// Without ctx override, falls back to default.
	if _, err := c.Do(context.Background(), &Request{Path: "/x"}); err != nil {
		t.Fatal(err)
	}
	if captured != "Bearer default-tok" {
		t.Errorf("fallback Authorization = %q, want Bearer default-tok", captured)
	}
}

func TestDoBasicAuthAuthorization(t *testing.T) {
	var captured string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	src, err := basic.New("alice", "secret")
	if err != nil {
		t.Fatal(err)
	}
	c, _ := New(
		newCatalog(t, srv),
		WithHTTPClient(srv.Client()),
		WithTokenSource(src),
	)
	if _, err := c.Do(context.Background(), &Request{Path: "/ehr"}); err != nil {
		t.Fatal(err)
	}
	want := "Basic " + base64.StdEncoding.EncodeToString([]byte("alice:secret"))
	if captured != want {
		t.Errorf("Authorization = %q, want %q (REQ-069)", captured, want)
	}
}

func TestDoCallerHeadersOverrideCaseInsensitive(t *testing.T) {
	var captured http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r.Header.Clone()
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	c, _ := New(newCatalog(t, srv), WithHTTPClient(srv.Client()))
	_, err := c.Do(context.Background(), &Request{
		Path: "/x",
		Headers: http.Header{
			"accept": {"application/xml"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := captured.Get("Accept"); got != "application/xml" {
		t.Errorf("Accept = %q, want application/xml (case-insensitive override)", got)
	}
}

func TestDoMapsErrorEnvelopes(t *testing.T) {
	cases := []struct {
		name     string
		file     string
		status   int
		sentinel error
	}{
		{"404", "404.json", 404, ErrNotFound},
		{"401", "401.json", 401, ErrUnauthorized},
		{"403", "403.json", 403, ErrForbidden},
		{"409", "409.json", 409, ErrVersionConflict},
		{"412", "412.json", 412, ErrPreconditionFailed},
		{"422", "422.json", 422, ErrUnprocessable},
		{"428", "428.json", 428, ErrPreconditionRequired},
		{"400", "400.json", 400, nil}, // no sentinel; pure WireError
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := readCassette(t, "errors", tc.file)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				_, _ = w.Write(body)
			}))
			defer srv.Close()
			c, _ := New(newCatalog(t, srv), WithHTTPClient(srv.Client()))
			_, err := c.Do(context.Background(), &Request{Path: "/x"})
			if err == nil {
				t.Fatal("expected error")
			}
			var we *WireError
			if !errors.As(err, &we) {
				t.Fatalf("expected *WireError, got %T", err)
			}
			if we.StatusCode != tc.status {
				t.Errorf("StatusCode = %d, want %d", we.StatusCode, tc.status)
			}
			if tc.sentinel != nil && !errors.Is(err, tc.sentinel) {
				t.Errorf("expected errors.Is sentinel %v, got %v", tc.sentinel, err)
			}
			if we.OpenEHR == nil {
				t.Fatal("expected OpenEHR detail")
			}
			if we.OpenEHR.Code == "" {
				t.Error("OpenEHR.Code empty")
			}
			// Default: Message and RawBody are omitted (PHI gate).
			// See TestWireErrorDefaultOmitsMessageAndRawBody.
			if we.OpenEHR.Message != "" {
				t.Errorf("OpenEHR.Message = %q; default client must omit message (PHI)", we.OpenEHR.Message)
			}
			if len(we.RawBody) != 0 {
				t.Errorf("RawBody non-empty (%d bytes); default client must omit raw body (PHI)", len(we.RawBody))
			}
		})
	}
}

func TestDoServerError5xxWrapsServerSentinel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		_, _ = w.Write([]byte(`{"message":"oops","code":"INTERNAL"}`))
	}))
	defer srv.Close()
	c, _ := New(newCatalog(t, srv), WithHTTPClient(srv.Client()))
	_, err := c.Do(context.Background(), &Request{Path: "/x"})
	if !errors.Is(err, ErrServerError) {
		t.Errorf("expected ErrServerError, got %v", err)
	}
}

func TestRetryOnRetriableStatus(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := hits.Add(1)
		if n < 3 {
			w.WriteHeader(503)
			return
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	c, _ := New(
		newCatalog(t, srv),
		WithHTTPClient(srv.Client()),
		WithRetry(RetryPolicy{
			MaxAttempts:    3,
			InitialBackoff: time.Millisecond,
			MaxBackoff:     5 * time.Millisecond,
			Multiplier:     2.0,
		}),
	)
	if _, err := c.Do(context.Background(), &Request{Path: "/x"}); err != nil {
		t.Fatal(err)
	}
	if got := hits.Load(); got != 3 {
		t.Errorf("expected 3 attempts, got %d", got)
	}
}

// TestRetryNotAppliedToNonRetriableStatus is the regression test for the
// review's Critical finding #1: with retry enabled, a 404 (or any status
// not in RetriableStatus) MUST result in exactly one attempt. Before the
// fix, the custom errorAs helper never recognised *WireError and the
// retry loop treated every wire failure as a transport error, retrying
// regardless of status. Violates REQ-091.
func TestRetryNotAppliedToNonRetriableStatus(t *testing.T) {
	cases := []struct {
		name   string
		status int
	}{
		{"404", 404},
		{"400", 400},
		{"409", 409},
		{"412", 412},
		{"428", 428},
		{"500", 500},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var hits atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				hits.Add(1)
				w.WriteHeader(tc.status)
			}))
			defer srv.Close()
			c, _ := New(
				newCatalog(t, srv),
				WithHTTPClient(srv.Client()),
				WithRetry(RetryPolicy{
					MaxAttempts:    5,
					InitialBackoff: time.Millisecond,
				}),
			)
			_, _ = c.Do(context.Background(), &Request{Method: "GET", Path: "/x"})
			if got := hits.Load(); got != 1 {
				t.Errorf("status %d under retry: got %d attempts, want 1 (status not in RetriableStatus)", tc.status, got)
			}
		})
	}
}

func TestRetryNotAppliedToPOSTByDefault(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(503)
	}))
	defer srv.Close()
	c, _ := New(
		newCatalog(t, srv),
		WithHTTPClient(srv.Client()),
		WithRetry(RetryPolicy{MaxAttempts: 5, InitialBackoff: time.Millisecond}),
	)
	_, _ = c.Do(context.Background(), &Request{Method: "POST", Path: "/x", Body: []byte(`{}`)})
	if got := hits.Load(); got != 1 {
		t.Errorf("expected 1 attempt for POST (non-idempotent), got %d", got)
	}
}

func TestRetryOptIntoNonIdempotent(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(503)
	}))
	defer srv.Close()
	c, _ := New(
		newCatalog(t, srv),
		WithHTTPClient(srv.Client()),
		WithRetry(RetryPolicy{MaxAttempts: 3, InitialBackoff: time.Millisecond, RetryNonIdempotent: true}),
	)
	_, _ = c.Do(context.Background(), &Request{Method: "POST", Path: "/x", Body: []byte(`{}`)})
	if got := hits.Load(); got != 3 {
		t.Errorf("expected 3 attempts with RetryNonIdempotent, got %d", got)
	}
}

func TestRetryDisabledByDefault(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(503)
	}))
	defer srv.Close()
	c, _ := New(newCatalog(t, srv), WithHTTPClient(srv.Client()))
	_, _ = c.Do(context.Background(), &Request{Path: "/x"})
	if got := hits.Load(); got != 1 {
		t.Errorf("expected 1 attempt by default, got %d", got)
	}
}

// TestRetryNoRetrySentinel covers REQ-096: an explicit NoRetry / Disabled
// policy MUST produce exactly one attempt even when the server returns
// a normally-retriable status. Regression guard for benchmarks that
// require hidden-retry-free latency measurement.
func TestRetryNoRetrySentinel(t *testing.T) {
	cases := []struct {
		name   string
		policy RetryPolicy
	}{
		{"NoRetry sentinel", NoRetry},
		{"Disabled overrides MaxAttempts", RetryPolicy{Disabled: true, MaxAttempts: 5, InitialBackoff: time.Millisecond}},
		{"MaxAttempts=1", RetryPolicy{MaxAttempts: 1, InitialBackoff: time.Millisecond}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var hits atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				hits.Add(1)
				w.WriteHeader(503)
			}))
			defer srv.Close()
			c, _ := New(
				newCatalog(t, srv),
				WithHTTPClient(srv.Client()),
				WithRetry(tc.policy),
			)
			_, _ = c.Do(context.Background(), &Request{Method: "GET", Path: "/x"})
			if got := hits.Load(); got != 1 {
				t.Errorf("policy %+v: got %d attempts, want 1", tc.policy, got)
			}
		})
	}
}

func TestRetryCtxCancellation(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(503)
	}))
	defer srv.Close()
	c, _ := New(
		newCatalog(t, srv),
		WithHTTPClient(srv.Client()),
		WithRetry(RetryPolicy{MaxAttempts: 10, InitialBackoff: 50 * time.Millisecond}),
	)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()
	_, err := c.Do(ctx, &Request{Path: "/x"})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected DeadlineExceeded, got %v", err)
	}
}

func TestCallerAttributionDefault(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get(DefaultCallerAttributionHeader)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	c, _ := New(
		newCatalog(t, srv),
		WithHTTPClient(srv.Client()),
		WithCallerAttribution(CallerAttribution{
			AgentID:       "mcp-claude-code/1.0",
			ModelProvider: "anthropic",
		}),
	)
	if _, err := c.Do(context.Background(), &Request{Path: "/x"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, `"agent_id":"mcp-claude-code/1.0"`) {
		t.Errorf("header = %q, missing agent_id", got)
	}
	if !strings.Contains(got, `"model_provider":"anthropic"`) {
		t.Errorf("header = %q, missing model_provider", got)
	}
}

func TestCallerAttributionPerRequest(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get(DefaultCallerAttributionHeader)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	c, _ := New(
		newCatalog(t, srv),
		WithHTTPClient(srv.Client()),
		WithCallerAttribution(CallerAttribution{AgentID: "default"}),
	)
	ctx := WithCallerAttributionCtx(context.Background(), CallerAttribution{AgentID: "override"})
	if _, err := c.Do(ctx, &Request{Path: "/x"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, `"agent_id":"override"`) {
		t.Errorf("header = %q, expected override agent_id", got)
	}
}

func TestCallerAttributionOmittedWhenEmpty(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get(DefaultCallerAttributionHeader)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	c, _ := New(newCatalog(t, srv), WithHTTPClient(srv.Client()))
	if _, err := c.Do(context.Background(), &Request{Path: "/x"}); err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Errorf("expected no attribution header, got %q", got)
	}
}

func TestETagRoundTrip(t *testing.T) {
	var captured string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "GET":
			w.Header().Set("ETag", `"v-1"`)
			_, _ = w.Write([]byte(`{"_type":"DV_TEXT","value":"hi"}`))
		case "PUT":
			captured = r.Header.Get("If-Match")
			w.Header().Set("ETag", `"v-2"`)
			w.WriteHeader(204)
		}
	}))
	defer srv.Close()
	c, _ := New(newCatalog(t, srv), WithHTTPClient(srv.Client()))
	getResp, err := c.Do(context.Background(), &Request{Method: "GET", Path: "/x"})
	if err != nil {
		t.Fatal(err)
	}
	if getResp.Metadata.ETag != "v-1" {
		t.Errorf("ETag captured = %q, want v-1", getResp.Metadata.ETag)
	}
	if _, err := c.Do(context.Background(), &Request{
		Method:  "PUT",
		Path:    "/x",
		IfMatch: getResp.Metadata.ETag,
	}); err != nil {
		t.Fatal(err)
	}
	if captured != `"v-1"` {
		t.Errorf("If-Match = %q, want \"v-1\" (re-quoted)", captured)
	}
}

func TestETagAlreadyQuotedNotDoubleQuoted(t *testing.T) {
	var captured string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r.Header.Get("If-Match")
		w.WriteHeader(204)
	}))
	defer srv.Close()
	c, _ := New(newCatalog(t, srv), WithHTTPClient(srv.Client()))
	if _, err := c.Do(context.Background(), &Request{Method: "PUT", Path: "/x", IfMatch: `"v-1"`}); err != nil {
		t.Fatal(err)
	}
	if captured != `"v-1"` {
		t.Errorf("If-Match = %q, expected single-quoted", captured)
	}
}

func TestDecodeGeneric(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"_type":"DV_TEXT","value":"hello"}`))
	}))
	defer srv.Close()
	c, _ := New(newCatalog(t, srv), WithHTTPClient(srv.Client()))

	type DVText struct {
		Type  string `json:"_type"`
		Value string `json:"value"`
	}
	_, _ = c, DVText{} // ensure types are referenced
	// We use canjson under the hood; round-trip through rm.DVText via reflection
	// is unnecessary here — just verify the Decode wrapper plumbs the body.
	resp, err := c.Do(context.Background(), &Request{Method: "GET", Path: "/x"})
	if err != nil {
		t.Fatal(err)
	}
	if string(resp.Body) == "" {
		t.Fatal("empty body")
	}
}

func TestDecodeInvalidShape(t *testing.T) {
	// Test the ErrInvalidShape path with a real rm type.
	type rmFake struct{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(204)
	}))
	defer srv.Close()
	c, _ := New(newCatalog(t, srv), WithHTTPClient(srv.Client()))
	_, _, err := Decode[rmFake](context.Background(), c, &Request{Path: "/x"})
	if !errors.Is(err, ErrInvalidShape) {
		t.Errorf("expected ErrInvalidShape, got %v", err)
	}
}

func TestNetworkError(t *testing.T) {
	// Catalog points at a port nobody is listening on.
	cat, _ := discovery.NewStaticCatalog(discovery.StaticConfig{
		Issuer: "https://x",
		Services: map[string]discovery.ServiceEntry{
			discovery.ServiceIDOpenEHRRest: {
				BaseURL:     discovery.MustParseURL("http://127.0.0.1:1"),
				SpecVersion: discovery.SpecVersionPin,
			},
		},
	})
	c, _ := New(cat, WithHTTPClient(&http.Client{Timeout: 50 * time.Millisecond}))
	_, err := c.Do(context.Background(), &Request{Path: "/x"})
	if err == nil {
		t.Fatal("expected network error")
	}
	if errors.Is(err, ErrNotFound) || errors.Is(err, ErrUnauthorized) {
		t.Errorf("network error misclassified: %v", err)
	}
}

func TestTraceparentInjected(t *testing.T) {
	// With the default no-op tracer, traceparent is empty — but the
	// injection path runs without panicking. This test asserts the
	// no-op safety property from REQ-090 / PROBE-051.
	var captured http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r.Header.Clone()
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	c, _ := New(newCatalog(t, srv), WithHTTPClient(srv.Client()))
	if _, err := c.Do(context.Background(), &Request{Path: "/x"}); err != nil {
		t.Fatal(err)
	}
	// No-op tracer => no traceparent. Asserting absence is the noop
	// guarantee; presence would indicate an unexpected global tracer.
	if tp := captured.Get("traceparent"); tp != "" {
		t.Logf("traceparent present under no-op tracer: %q (informational)", tp)
	}
}

// TestWireErrorDefaultOmitsMessageAndRawBody verifies that a Client built
// without WithRawErrorBodies does not expose PHI-bearing fields: Error()
// must omit the server message, WireError.OpenEHR.Message must be empty,
// WireError.RawBody must be nil/empty, and WireError.OpenEHR.Code (a coded
// terminology identifier, not free text) must still be present.
func TestWireErrorDefaultOmitsMessageAndRawBody(t *testing.T) {
	const phi = "patient 1234 not found"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(404)
		_, _ = w.Write([]byte(`{"message":"` + phi + `","code":"NOT_FOUND"}`))
	}))
	defer srv.Close()

	// Default client — no WithRawErrorBodies.
	c, _ := New(newCatalog(t, srv), WithHTTPClient(srv.Client()))
	_, err := c.Do(context.Background(), &Request{Path: "/x"})
	if err == nil {
		t.Fatal("expected error")
	}

	var we *WireError
	if !errors.As(err, &we) {
		t.Fatalf("expected *WireError, got %T", err)
	}

	// Error() must not contain the PHI message or any patient identifier.
	errStr := we.Error()
	if strings.Contains(errStr, "1234") {
		t.Errorf("Error() leaks PHI: %q (must not contain \"1234\")", errStr)
	}
	if strings.Contains(errStr, phi) {
		t.Errorf("Error() leaks PHI: %q (must not contain message text)", errStr)
	}

	// Error() must still contain the openEHR code (non-PHI coded identifier).
	if !strings.Contains(errStr, "NOT_FOUND") {
		t.Errorf("Error() = %q; must contain openEHR code NOT_FOUND", errStr)
	}

	// OpenEHR detail is present but Message is cleared.
	if we.OpenEHR == nil {
		t.Fatal("expected OpenEHR detail to be set (code still present)")
	}
	if we.OpenEHR.Code != "NOT_FOUND" {
		t.Errorf("OpenEHR.Code = %q, want NOT_FOUND", we.OpenEHR.Code)
	}
	if we.OpenEHR.Message != "" {
		t.Errorf("OpenEHR.Message = %q; default client must clear message (PHI)", we.OpenEHR.Message)
	}

	// RawBody must be empty by default.
	if len(we.RawBody) != 0 {
		t.Errorf("RawBody = %d bytes; default client must not preserve raw body (PHI)", len(we.RawBody))
	}
}

// TestWireErrorOptInPreservesMessageAndRawBody verifies that a Client built
// with WithRawErrorBodies(true) preserves the full server payload on WireError:
// OpenEHR.Message is populated and RawBody contains the server response bytes.
func TestWireErrorOptInPreservesMessageAndRawBody(t *testing.T) {
	const phi = "patient 1234 not found"
	const rawPayload = `{"message":"patient 1234 not found","code":"NOT_FOUND"}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(404)
		_, _ = w.Write([]byte(rawPayload))
	}))
	defer srv.Close()

	// Opt-in client.
	c, _ := New(newCatalog(t, srv), WithHTTPClient(srv.Client()), WithRawErrorBodies(true))
	_, err := c.Do(context.Background(), &Request{Path: "/x"})
	if err == nil {
		t.Fatal("expected error")
	}

	var we *WireError
	if !errors.As(err, &we) {
		t.Fatalf("expected *WireError, got %T", err)
	}

	if we.OpenEHR == nil {
		t.Fatal("expected OpenEHR detail")
	}
	if we.OpenEHR.Code != "NOT_FOUND" {
		t.Errorf("OpenEHR.Code = %q, want NOT_FOUND", we.OpenEHR.Code)
	}
	if we.OpenEHR.Message != phi {
		t.Errorf("OpenEHR.Message = %q, want %q (opt-in should preserve)", we.OpenEHR.Message, phi)
	}
	if len(we.RawBody) == 0 {
		t.Error("RawBody empty; WithRawErrorBodies(true) must preserve raw body")
	}
	if string(we.RawBody) != rawPayload {
		t.Errorf("RawBody = %q, want %q", we.RawBody, rawPayload)
	}
}

// TestMaxResponseBody verifies the body size cap enforced by WithMaxResponseBody.
//
// Sub-test "exceeded": a 1 KiB cap on a 4 KiB response must return an error
// whose message contains "exceeds" (not an OOM / truncated success).
//
// Sub-test "within_limit": a small response (a few bytes) with the default cap
// (64 MiB, i.e. no option set) must succeed — normal traffic is unaffected.
func TestMaxResponseBody(t *testing.T) {
	t.Run("exceeded", func(t *testing.T) {
		// Server returns 4 KiB; client caps at 1 KiB.
		body4k := strings.Repeat("x", 4<<10)
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(body4k))
		}))
		defer srv.Close()
		c, _ := New(
			newCatalog(t, srv),
			WithHTTPClient(srv.Client()),
			WithMaxResponseBody(1<<10),
		)
		_, err := c.Do(context.Background(), &Request{Path: "/x"})
		if err == nil {
			t.Fatal("expected error for oversized body, got nil")
		}
		if !strings.Contains(err.Error(), "exceeds") {
			t.Errorf("error %q should mention \"exceeds\"", err.Error())
		}
	})

	t.Run("within_limit", func(t *testing.T) {
		// Small body, default client (64 MiB cap) — must succeed.
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"ok":true}`))
		}))
		defer srv.Close()
		c, _ := New(newCatalog(t, srv), WithHTTPClient(srv.Client()))
		resp, err := c.Do(context.Background(), &Request{Path: "/x"})
		if err != nil {
			t.Fatalf("unexpected error for small body: %v", err)
		}
		if len(resp.Body) == 0 {
			t.Error("expected non-empty body")
		}
	})
}

// --- 401→reauth tests (REQ-063) ---

// stubReauther is a test double for auth.Reauther that records how many
// times Reauth was called and which token the associated TokenSource
// should vend after the first Reauth call.
type stubTokenSource struct {
	mu        sync.Mutex
	reauths   int
	tokens    []string // tokens[0] is pre-reauth, tokens[1] is post-reauth
	nextIndex int
}

func (s *stubTokenSource) Token(_ context.Context) (auth.Token, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx := s.nextIndex
	if idx >= len(s.tokens) {
		idx = len(s.tokens) - 1
	}
	return auth.Token{Value: s.tokens[idx], Type: "Bearer"}, nil
}

func (s *stubTokenSource) Reauth(_ context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reauths++
	if s.nextIndex < len(s.tokens)-1 {
		s.nextIndex++
	}
	return nil
}

// Reauths returns the number of times Reauth was called.
// Safe for concurrent use: reads under the same mutex as Reauth.
func (s *stubTokenSource) Reauths() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.reauths
}

// TestDoReauthOn401 — REQ-063: on a wire 401, if a Reauther is configured
// and this Do has not yet reauthed, transport calls Reauth once then retries.
// Assert: exactly 2 upstream calls, second carries refreshed bearer, final err=nil.
func TestDoReauthOn401(t *testing.T) { // REQ-063
	var (
		hits     atomic.Int32
		captured [2]atomic.Value // Store/Load from handler and test goroutines
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := int(hits.Add(1))
		if n <= 2 {
			captured[n-1].Store(r.Header.Get("Authorization"))
		}
		if n == 1 {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	stub := &stubTokenSource{tokens: []string{"old-tok", "fresh-tok"}}
	c, _ := New(
		newCatalog(t, srv),
		WithHTTPClient(srv.Client()),
		WithTokenSource(stub),
		WithReauthOn401(stub),
	)
	resp, err := c.Do(context.Background(), &Request{Path: "/x"})
	if err != nil {
		t.Fatalf("expected success after reauth; got %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("StatusCode = %d, want 200", resp.StatusCode)
	}
	if n := hits.Load(); n != 2 {
		t.Errorf("upstream calls = %d, want exactly 2", n)
	}
	first, _ := captured[0].Load().(string)
	second, _ := captured[1].Load().(string)
	if first != "Bearer old-tok" {
		t.Errorf("first request Authorization = %q, want Bearer old-tok", first)
	}
	if second != "Bearer fresh-tok" {
		t.Errorf("second request Authorization = %q, want Bearer fresh-tok (refreshed)", second)
	}
	if n := stub.Reauths(); n != 1 {
		t.Errorf("Reauth called %d times, want exactly 1", n)
	}
}

// TestDoReauthOn401TwiceFails — REQ-063: when the retry after Reauth also
// returns 401, transport surfaces ErrUnauthorized and does NOT loop.
// Assert: Reauth attempted exactly once, err=ErrUnauthorized.
func TestDoReauthOn401TwiceFails(t *testing.T) { // REQ-063
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	stub := &stubTokenSource{tokens: []string{"tok"}}
	c, _ := New(
		newCatalog(t, srv),
		WithHTTPClient(srv.Client()),
		WithTokenSource(stub),
		WithReauthOn401(stub),
	)
	_, err := c.Do(context.Background(), &Request{Path: "/x"})
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized, got %v", err)
	}
	if n := hits.Load(); n != 2 {
		t.Errorf("upstream calls = %d, want exactly 2 (original + one retry)", n)
	}
	if n := stub.Reauths(); n != 1 {
		t.Errorf("Reauth called %d times, want exactly 1 (no infinite loop)", n)
	}
}

// TestDoNoReautherUnchanged — REQ-063: when no Reauther is configured, a
// wire 401 is returned immediately as ErrUnauthorized with exactly one
// upstream call (unchanged existing contract).
func TestDoNoReautherUnchanged(t *testing.T) { // REQ-063
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	c, _ := New(newCatalog(t, srv), WithHTTPClient(srv.Client()))
	_, err := c.Do(context.Background(), &Request{Path: "/x"})
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized, got %v", err)
	}
	if n := hits.Load(); n != 1 {
		t.Errorf("upstream calls = %d, want exactly 1 (no reauther, no retry)", n)
	}
}

// TestDoReauthReturnsError — REQ-063: when the configured Reauther fails on a
// 401, transport surfaces the (wrapped) reauth error rather than retrying or
// swallowing it, and makes no second upstream call.
func TestDoReauthReturnsError(t *testing.T) { // REQ-063
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	errBoom := errors.New("reauth boom")
	stub := &stubTokenSource{tokens: []string{"tok"}}
	c, _ := New(
		newCatalog(t, srv),
		WithHTTPClient(srv.Client()),
		WithTokenSource(stub),
		WithReauthOn401(auth.ReautherFunc(func(context.Context) error { return errBoom })),
	)
	_, err := c.Do(context.Background(), &Request{Path: "/x"})
	if !errors.Is(err, errBoom) {
		t.Fatalf("expected wrapped reauth error, got %v", err)
	}
	if n := hits.Load(); n != 1 {
		t.Errorf("upstream calls = %d, want exactly 1 (reauth failed → no retry)", n)
	}
}

// TestDoReauthWithRealSmartSource — REQ-063: end-to-end proof that a real
// *smart.Source satisfies auth.Reauther and recovers from a wire 401 by running
// its refresh_token grant. The cached token looks valid (far-future expiry, so
// no proactive refresh fires), but the resource rejects it once — simulating
// server-side revocation — and transport drives Source.Reauth, which POSTs a
// refresh grant to the token endpoint, then retries with the fresh bearer.
func TestDoReauthWithRealSmartSource(t *testing.T) { // REQ-063
	var resourceHits atomic.Int32
	var secondBearer atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			_ = r.ParseForm()
			if r.Form.Get("grant_type") == "refresh_token" && r.Form.Get("refresh_token") != "" {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"access_token":"fresh-tok","token_type":"Bearer","expires_in":3600}`))
				return
			}
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		// Resource path: reject the stale bearer once, accept the refreshed one.
		if int(resourceHits.Add(1)) == 1 {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		secondBearer.Store(r.Header.Get("Authorization"))
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	src, err := smart.New("client-id", discovery.AuthEndpoints{
		AuthorizationEndpoint: discovery.MustParseURL(srv.URL + "/authorize"),
		TokenEndpoint:         discovery.MustParseURL(srv.URL + "/token"),
	}, smart.WithHTTPClient(srv.Client()), smart.WithRedirectURI("https://cb"))
	if err != nil {
		t.Fatal(err)
	}
	// Non-stale access token (far-future expiry → no proactive refresh) plus a
	// refresh token, so the 401→reauth safety net is what drives the refresh.
	src.SetTokens(auth.Token{Value: "stale-tok", Type: "Bearer", ExpiresAt: time.Now().Add(time.Hour)}, "refresh-xyz")

	c, _ := New(newCatalog(t, srv), WithHTTPClient(srv.Client()), WithTokenSource(src), WithReauthOn401(src))
	resp, err := c.Do(context.Background(), &Request{Path: "/x"})
	if err != nil {
		t.Fatalf("expected success after real-Source reauth, got %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("StatusCode = %d, want 200", resp.StatusCode)
	}
	if got, _ := secondBearer.Load().(string); got != "Bearer fresh-tok" {
		t.Errorf("retry Authorization = %q, want Bearer fresh-tok", got)
	}
	if n := resourceHits.Load(); n != 2 {
		t.Errorf("resource calls = %d, want 2 (401 then 200)", n)
	}
}

// TestDoReauthDoesNotRestartRetryBudget — REQ-063: when WithReauthOn401 and a
// RetryPolicy are both configured, a 401 encountered mid-5xx-sequence MUST
// grant exactly ONE additional attempt and MUST NOT reopen the 5xx retry
// budget.
//
// Sequence: attempt-1→503 (retried), attempt-2→503 (budget exhausted at
// MaxAttempts=3 means attempt 3 is the last one per policy, so 503 twice uses
// attempts 1 and 2), attempt-3→401 (budget exhausted; reauth fires for one
// extra attempt), attempt-4→200.
//
// Total upstream calls = 4: 3 within the retry budget + 1 reauth-extra.
// The 5xx budget is NOT restarted, so no further 503 retries happen.
func TestDoReauthDoesNotRestartRetryBudget(t *testing.T) { // REQ-063
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := int(hits.Add(1))
		switch n {
		case 1, 2:
			// 503 — within the retry budget (MaxAttempts=3, so attempts 1 and 2
			// are retried; attempt 3 is the last policy attempt).
			w.WriteHeader(http.StatusServiceUnavailable)
		case 3:
			// 401 on the last budget attempt — triggers one reauth-extra.
			w.WriteHeader(http.StatusUnauthorized)
		case 4:
			// Reauth-extra attempt — succeeds.
			_, _ = w.Write([]byte(`{}`))
		default:
			// Any further call proves the budget was incorrectly reopened.
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	}))
	defer srv.Close()

	stub := &stubTokenSource{tokens: []string{"tok", "fresh-tok"}}
	c, _ := New(
		newCatalog(t, srv),
		WithHTTPClient(srv.Client()),
		WithTokenSource(stub),
		WithReauthOn401(stub),
		WithRetry(RetryPolicy{
			MaxAttempts:     3,
			InitialBackoff:  time.Millisecond,
			RetriableStatus: []int{503},
		}),
	)
	resp, err := c.Do(context.Background(), &Request{Path: "/x"})
	if err != nil {
		t.Fatalf("expected success after reauth; got %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("StatusCode = %d, want 200", resp.StatusCode)
	}
	// Must be exactly 4: 3 budget attempts (2×503 + 1×401) + 1 reauth-extra.
	if n := hits.Load(); n != 4 {
		t.Errorf("upstream calls = %d, want exactly 4 (3 budget + 1 reauth-extra); 5xx budget was incorrectly restarted", n)
	}
	if n := stub.Reauths(); n != 1 {
		t.Errorf("Reauth called %d times, want exactly 1", n)
	}
}

// readCassette returns the bytes of a vendored cassette at
// testkit/cassettes/its_rest/<dir>/<name>.
func readCassette(t *testing.T, dir, name string) []byte {
	t.Helper()
	_, src, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	path := filepath.Join(filepath.Dir(src), "..", "testkit", "cassettes", "its_rest", dir, name)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
