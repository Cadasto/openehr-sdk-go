package definition_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/openehr/client/definition"
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

	meta, _, err := definition.PutStoredQuery(context.Background(), newClient(t, srv),
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
}
