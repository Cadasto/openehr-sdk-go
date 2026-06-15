package query_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/aql"
	"github.com/cadasto/openehr-sdk-go/openehr/client/query"
	"github.com/cadasto/openehr-sdk-go/smart/discovery"
	"github.com/cadasto/openehr-sdk-go/transport"
)

func newClient(t *testing.T, srv *httptest.Server) *transport.Client {
	t.Helper()
	cat, _ := discovery.NewStaticCatalog(discovery.StaticConfig{
		Issuer: "https://test.example.com",
		Services: map[string]discovery.ServiceEntry{
			discovery.ServiceIDOpenEHRRest: {
				BaseURL:     discovery.MustParseURL(srv.URL + "/openehr/v1"),
				SpecVersion: discovery.SpecVersionPin,
			},
		},
	})
	c, err := transport.New(cat, transport.WithHTTPClient(srv.Client()))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func readCassette(t *testing.T, name string) []byte {
	t.Helper()
	_, src, _, _ := runtime.Caller(0)
	path := filepath.Join(filepath.Dir(src), "..", "..", "..", "testkit", "cassettes", "its_rest", "query", name)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read cassette %q: %v", path, err)
	}
	return b
}

func TestExecuteAdhoc(t *testing.T) {
	var captured *http.Request
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r.Clone(r.Context())
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(readCassette(t, "result_set.json"))
	}))
	defer srv.Close()

	rs, _, err := query.Execute(context.Background(), newClient(t, srv), aql.Query{
		Q: "SELECT e/ehr_id/value FROM EHR e",
		Parameters: map[string]any{
			"ehr_id": "7d44b88c-4199-4bad-97dc-d78268e01398",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if captured.Method != http.MethodPost {
		t.Errorf("method = %q", captured.Method)
	}
	if captured.URL.Path != "/openehr/v1/query/aql" {
		t.Errorf("path = %q", captured.URL.Path)
	}
	if body["q"] != "SELECT e/ehr_id/value FROM EHR e" {
		t.Errorf("q = %v", body["q"])
	}
	if len(rs.Rows) != 1 {
		t.Fatalf("rows = %d", len(rs.Rows))
	}
}

func TestExecuteWithEHRID(t *testing.T) {
	var captured *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r.Clone(r.Context())
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(readCassette(t, "result_set.json"))
	}))
	defer srv.Close()

	ehrID := "7d44b88c-4199-4bad-97dc-d78268e01398"
	_, _, err := query.Execute(context.Background(), newClient(t, srv), aql.Query{
		Q:     "SELECT e/ehr_id/value FROM EHR e",
		EHRID: ehrID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if captured.URL.Query().Get("ehr_id") != ehrID {
		t.Errorf("ehr_id query = %q", captured.URL.Query().Get("ehr_id"))
	}
}

func TestRunStoredWithEHRID(t *testing.T) {
	var captured *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r.Clone(r.Context())
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(readCassette(t, "result_set.json"))
	}))
	defer srv.Close()

	ehrID := "7d44b88c-4199-4bad-97dc-d78268e01398"
	_, _, err := query.RunStored(
		context.Background(), newClient(t, srv), "org.openehr::compositions", nil,
		query.WithEHRID(ehrID),
	)
	if err != nil {
		t.Fatal(err)
	}
	if captured.URL.Query().Get("ehr_id") != ehrID {
		t.Errorf("ehr_id query = %q", captured.URL.Query().Get("ehr_id"))
	}
}

func TestRunStored(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/openehr/v1/query/org.openehr::compositions" {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(readCassette(t, "result_set.json"))
	}))
	defer srv.Close()

	_, _, err := query.RunStored(context.Background(), newClient(t, srv), "org.openehr::compositions", map[string]any{
		"ehr_id": "7d44b88c-4199-4bad-97dc-d78268e01398",
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestExecuteEmptyQuery(t *testing.T) {
	_, _, err := query.Execute(context.Background(), newClient(t, httptest.NewServer(nil)), aql.Query{})
	if err == nil {
		t.Fatal("expected error")
	}
}

// TestExecuteAQLError verifies that a server-side AQL error is surfaced as an
// *AQLError with the PHI-free error code present in its .Error() string even
// when the default (PHI-suppressed) client is used.
func TestExecuteAQLError(t *testing.T) {
	// Server returns a 400 with an openEHR error envelope containing PHI in
	// the message but a coded, non-PHI error code.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"message":"patient 1234 not found","code":"VALIDATION_FAILED"}`))
	}))
	defer srv.Close()

	t.Run("default_client_suppresses_phi", func(t *testing.T) {
		// Default client: WithRawErrorBodies is false (PHI suppressed).
		c := newClient(t, srv)
		_, _, err := query.Execute(context.Background(), c, aql.Query{
			Q: "SELECT e/ehr_id/value FROM EHR e",
		})
		if err == nil {
			t.Fatal("expected error, got nil")
		}

		var aqlErr *query.AQLError
		if !errors.As(err, &aqlErr) {
			t.Fatalf("expected *query.AQLError, got %T: %v", err, err)
		}

		// Code must be preserved — it is non-PHI.
		if aqlErr.Code != "VALIDATION_FAILED" {
			t.Errorf("Code = %q, want %q", aqlErr.Code, "VALIDATION_FAILED")
		}
		// Message must be suppressed.
		if aqlErr.Message != "" {
			t.Errorf("Message = %q, want empty (PHI suppressed)", aqlErr.Message)
		}
		// Error() must include the code, not just the generic fallback.
		errStr := aqlErr.Error()
		if !strings.Contains(errStr, "VALIDATION_FAILED") {
			t.Errorf("Error() = %q, want it to contain %q", errStr, "VALIDATION_FAILED")
		}
		// Error() must not contain PHI.
		if strings.Contains(errStr, "1234") {
			t.Errorf("Error() = %q leaks PHI (contains %q)", errStr, "1234")
		}
	})

	t.Run("raw_error_bodies_preserves_message", func(t *testing.T) {
		// Build a client with WithRawErrorBodies(true): message must be visible.
		cat, _ := discovery.NewStaticCatalog(discovery.StaticConfig{
			Issuer: "https://test.example.com",
			Services: map[string]discovery.ServiceEntry{
				discovery.ServiceIDOpenEHRRest: {
					BaseURL:     discovery.MustParseURL(srv.URL + "/openehr/v1"),
					SpecVersion: discovery.SpecVersionPin,
				},
			},
		})
		c, err := transport.New(
			cat,
			transport.WithHTTPClient(srv.Client()),
			transport.WithRawErrorBodies(true),
		)
		if err != nil {
			t.Fatal(err)
		}

		_, _, err = query.Execute(context.Background(), c, aql.Query{
			Q: "SELECT e/ehr_id/value FROM EHR e",
		})
		if err == nil {
			t.Fatal("expected error, got nil")
		}

		var aqlErr *query.AQLError
		if !errors.As(err, &aqlErr) {
			t.Fatalf("expected *query.AQLError, got %T: %v", err, err)
		}

		if aqlErr.Message != "patient 1234 not found" {
			t.Errorf("Message = %q, want %q", aqlErr.Message, "patient 1234 not found")
		}
		if aqlErr.Code != "VALIDATION_FAILED" {
			t.Errorf("Code = %q, want %q", aqlErr.Code, "VALIDATION_FAILED")
		}
		if !strings.Contains(aqlErr.Error(), "patient 1234 not found") {
			t.Errorf("Error() = %q, want it to contain the message", aqlErr.Error())
		}
	})
}

// TestExecutePathResolutionError verifies the PROBE-021 mapping: a backend AQL
// error whose envelope denotes path resolution is surfaced as an *AQLError that
// also satisfies errors.Is(err, aql.ErrPathResolution), so callers can branch
// without inspecting CDR-specific codes. A generic validation error must NOT.
func TestExecutePathResolutionError(t *testing.T) {
	cases := map[string]struct {
		body      string
		wantIsErr bool
	}{
		"path code":        {`{"code":"AQL_PATH_RESOLUTION","message":"x"}`, true},
		"path message":     {`{"code":"BAD_REQUEST","message":"could not resolve path /foo/bar"}`, true},
		"generic non-path": {`{"code":"VALIDATION_FAILED","message":"bad request"}`, false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()

			// WithRawErrorBodies so the message-based classifier sees the text.
			cat, _ := discovery.NewStaticCatalog(discovery.StaticConfig{
				Issuer: "https://test.example.com",
				Services: map[string]discovery.ServiceEntry{
					discovery.ServiceIDOpenEHRRest: {
						BaseURL:     discovery.MustParseURL(srv.URL + "/openehr/v1"),
						SpecVersion: discovery.SpecVersionPin,
					},
				},
			})
			c, err := transport.New(
				cat,
				transport.WithHTTPClient(srv.Client()),
				transport.WithRawErrorBodies(true),
			)
			if err != nil {
				t.Fatal(err)
			}

			_, _, err = query.Execute(context.Background(), c, aql.NewQuery("SELECT e FROM EHR e"))
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			var aqlErr *query.AQLError
			if !errors.As(err, &aqlErr) {
				t.Fatalf("expected *query.AQLError, got %T", err)
			}
			if got := errors.Is(err, aql.ErrPathResolution); got != tc.wantIsErr {
				t.Errorf("errors.Is(err, aql.ErrPathResolution) = %v, want %v", got, tc.wantIsErr)
			}
		})
	}
}
