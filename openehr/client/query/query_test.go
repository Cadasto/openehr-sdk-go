package query_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
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
	_, _, err := query.RunStored(context.Background(), newClient(t, srv), "org.openehr::compositions", nil,
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
