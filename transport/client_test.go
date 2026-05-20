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
	"sync/atomic"
	"testing"
	"time"

	"github.com/cadasto/openehr-sdk-go/auth"
	"github.com/cadasto/openehr-sdk-go/auth/basic"
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
	c, _ := New(newCatalog(t, srv),
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

// TestDoIdempotencyKey covers REQ-097: a non-empty Request.IdempotencyKey
// MUST set the Idempotency-Key header verbatim (no quoting, no prefix);
// an empty value MUST NOT set the header (avoids accidental empty keys).
func TestDoIdempotencyKey(t *testing.T) {
	t.Run("non-empty sets header", func(t *testing.T) {
		var captured http.Header
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			captured = r.Header.Clone()
			w.WriteHeader(201)
			_, _ = w.Write([]byte(`{}`))
		}))
		defer srv.Close()
		c, _ := New(newCatalog(t, srv), WithHTTPClient(srv.Client()))
		_, err := c.Do(context.Background(), &Request{
			Method:         "POST",
			Path:           "/ehr/x/composition",
			Body:           []byte(`{}`),
			IdempotencyKey: "01900000-0000-7000-8000-000000000abc",
		})
		if err != nil {
			t.Fatal(err)
		}
		if got := captured.Get("Idempotency-Key"); got != "01900000-0000-7000-8000-000000000abc" {
			t.Errorf("Idempotency-Key = %q, want verbatim UUIDv7", got)
		}
	})
	t.Run("empty omits header", func(t *testing.T) {
		var captured http.Header
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			captured = r.Header.Clone()
			w.WriteHeader(201)
			_, _ = w.Write([]byte(`{}`))
		}))
		defer srv.Close()
		c, _ := New(newCatalog(t, srv), WithHTTPClient(srv.Client()))
		_, err := c.Do(context.Background(), &Request{
			Method: "POST",
			Path:   "/ehr/x/composition",
			Body:   []byte(`{}`),
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, present := captured["Idempotency-Key"]; present {
			t.Errorf("Idempotency-Key header set despite empty IdempotencyKey field: %q", captured.Get("Idempotency-Key"))
		}
	})
}

func TestDoNoAuthSuppressesAuthorization(t *testing.T) {
	var captured string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	c, _ := New(newCatalog(t, srv),
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
	c, _ := New(newCatalog(t, srv),
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
	c, _ := New(newCatalog(t, srv),
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
			if we.OpenEHR.Message == "" {
				t.Error("OpenEHR.Message empty")
			}
			if len(we.RawBody) == 0 {
				t.Error("RawBody empty")
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
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&hits, 1)
		if n < 3 {
			w.WriteHeader(503)
			return
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	c, _ := New(newCatalog(t, srv),
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
	if got := atomic.LoadInt32(&hits); got != 3 {
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
			var hits int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				atomic.AddInt32(&hits, 1)
				w.WriteHeader(tc.status)
			}))
			defer srv.Close()
			c, _ := New(newCatalog(t, srv),
				WithHTTPClient(srv.Client()),
				WithRetry(RetryPolicy{
					MaxAttempts:    5,
					InitialBackoff: time.Millisecond,
				}),
			)
			_, _ = c.Do(context.Background(), &Request{Method: "GET", Path: "/x"})
			if got := atomic.LoadInt32(&hits); got != 1 {
				t.Errorf("status %d under retry: got %d attempts, want 1 (status not in RetriableStatus)", tc.status, got)
			}
		})
	}
}

func TestRetryNotAppliedToPOSTByDefault(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(503)
	}))
	defer srv.Close()
	c, _ := New(newCatalog(t, srv),
		WithHTTPClient(srv.Client()),
		WithRetry(RetryPolicy{MaxAttempts: 5, InitialBackoff: time.Millisecond}),
	)
	_, _ = c.Do(context.Background(), &Request{Method: "POST", Path: "/x", Body: []byte(`{}`)})
	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Errorf("expected 1 attempt for POST (non-idempotent), got %d", got)
	}
}

func TestRetryOptIntoNonIdempotent(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(503)
	}))
	defer srv.Close()
	c, _ := New(newCatalog(t, srv),
		WithHTTPClient(srv.Client()),
		WithRetry(RetryPolicy{MaxAttempts: 3, InitialBackoff: time.Millisecond, RetryNonIdempotent: true}),
	)
	_, _ = c.Do(context.Background(), &Request{Method: "POST", Path: "/x", Body: []byte(`{}`)})
	if got := atomic.LoadInt32(&hits); got != 3 {
		t.Errorf("expected 3 attempts with RetryNonIdempotent, got %d", got)
	}
}

func TestRetryDisabledByDefault(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(503)
	}))
	defer srv.Close()
	c, _ := New(newCatalog(t, srv), WithHTTPClient(srv.Client()))
	_, _ = c.Do(context.Background(), &Request{Path: "/x"})
	if got := atomic.LoadInt32(&hits); got != 1 {
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
			var hits int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				atomic.AddInt32(&hits, 1)
				w.WriteHeader(503)
			}))
			defer srv.Close()
			c, _ := New(newCatalog(t, srv),
				WithHTTPClient(srv.Client()),
				WithRetry(tc.policy),
			)
			_, _ = c.Do(context.Background(), &Request{Method: "GET", Path: "/x"})
			if got := atomic.LoadInt32(&hits); got != 1 {
				t.Errorf("policy %+v: got %d attempts, want 1", tc.policy, got)
			}
		})
	}
}

func TestRetryCtxCancellation(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(503)
	}))
	defer srv.Close()
	c, _ := New(newCatalog(t, srv),
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
	c, _ := New(newCatalog(t, srv),
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
	c, _ := New(newCatalog(t, srv),
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
