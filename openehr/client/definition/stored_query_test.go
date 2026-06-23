package definition_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/client/definition"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// TestGetStoredQueryEmptyBody verifies that a 200 response with an
// empty body does not return a decode error. CDRs may legally return
// 200/204 with no body on a GET; without the guard, json.Unmarshal
// yields "unexpected end of JSON input".
func TestGetStoredQueryEmptyBody(t *testing.T) {
	const wantName = "org.openehr::vitals"
	const wantVersion = "1.0.0"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		// intentionally empty body
	}))
	defer srv.Close()

	meta, _, err := definition.GetStoredQuery(context.Background(), newClient(t, srv), wantName, wantVersion)
	if err != nil {
		t.Fatalf("GetStoredQuery: unexpected error on empty body: %v", err)
	}
	if meta == nil {
		t.Fatal("GetStoredQuery: returned nil metadata")
	}
	if meta.Name != wantName {
		t.Errorf("Name = %q, want %q", meta.Name, wantName)
	}
	if meta.Version != wantVersion {
		t.Errorf("Version = %q, want %q", meta.Version, wantVersion)
	}
}

func TestPutStoredQuery(t *testing.T) {
	var captured *http.Request
	var body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r.Clone(r.Context())
		b, _ := io.ReadAll(r.Body)
		body = string(b)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"name": "org.openehr::vitals",
			"type": "aql",
			"version": "1.0.0",
			"q": "SELECT c FROM EHR e CONTAINS COMPOSITION c"
		}`))
	}))
	defer srv.Close()

	meta, _, err := definition.PutStoredQuery(
		context.Background(), newClient(t, srv),
		"org.openehr::vitals",
		"SELECT c FROM EHR e CONTAINS COMPOSITION c",
	)
	if err != nil {
		t.Fatal(err)
	}
	if captured.Method != http.MethodPut {
		t.Errorf("method = %q", captured.Method)
	}
	if !strings.Contains(captured.URL.Path, "org.openehr") {
		t.Errorf("path = %q", captured.URL.Path)
	}
	if captured.Header.Get("Content-Type") != "text/plain" {
		t.Errorf("content-type = %q", captured.Header.Get("Content-Type"))
	}
	if body != "SELECT c FROM EHR e CONTAINS COMPOSITION c" {
		t.Errorf("body = %q", body)
	}
	if meta.Name != "org.openehr::vitals" {
		t.Errorf("name = %q", meta.Name)
	}
	if got := captured.URL.Query().Get("query_type"); got != "AQL" {
		t.Errorf("query_type = %q, want AQL", got)
	}
}

func TestPutStoredQueryVersion(t *testing.T) {
	var captured *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r.Clone(r.Context())
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	meta, _, err := definition.PutStoredQueryVersion(
		context.Background(), newClient(t, srv),
		"org.openehr::vitals", "1.2.0",
		"SELECT c FROM EHR e CONTAINS COMPOSITION c",
	)
	if err != nil {
		t.Fatal(err)
	}
	if captured.Method != http.MethodPut {
		t.Errorf("method = %q", captured.Method)
	}
	if captured.URL.Path != "/openehr/v1/definition/query/org.openehr::vitals/1.2.0" {
		t.Errorf("path = %q, want …/org.openehr::vitals/1.2.0", captured.URL.Path)
	}
	if meta.Version != "1.2.0" {
		t.Errorf("version = %q, want 1.2.0", meta.Version)
	}
}

func TestPutStoredQueryVersionRejectsEmpty(t *testing.T) {
	_, _, err := definition.PutStoredQueryVersion(context.Background(), nil, "org.openehr::vitals", "", "SELECT 1")
	if !errors.Is(err, transport.ErrInvalidConfig) {
		t.Errorf("expected ErrInvalidConfig, got %v", err)
	}
}

func TestPutStoredQueryVersionConflict(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"message":"version already exists","code":"CONFLICT"}`))
	}))
	defer srv.Close()
	_, _, err := definition.PutStoredQueryVersion(context.Background(), newClient(t, srv),
		"org.openehr::vitals", "1.2.0", "SELECT c FROM EHR e CONTAINS COMPOSITION c")
	if !errors.Is(err, transport.ErrVersionConflict) {
		t.Errorf("409 should map to ErrVersionConflict, got %v", err)
	}
}
