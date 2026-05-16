package definitionprobes_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/cadasto/openehr-sdk-go/smart/discovery"
	probes "github.com/cadasto/openehr-sdk-go/testkit/probes/definition"
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

func readOPT(t *testing.T) []byte {
	t.Helper()
	_, src, _, _ := runtime.Caller(0)
	path := filepath.Join(filepath.Dir(src), "..", "..", "cassettes", "its_rest", "definition", "body_weight.opt")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read cassette %q: %v", path, err)
	}
	return b
}

func TestProbe067TemplateUploadRoundTrip(t *testing.T) {
	opt := readOPT(t)
	// Server stores POSTed body in memory and returns it on GET.
	var stored []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			stored, _ = io.ReadAll(r.Body)
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Location", "/openehr/v1/definition/template/adl1.4/body_weight.v1")
			_, _ = w.Write([]byte(`{"template_id":"body_weight.v1"}`))
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write(stored)
		default:
			t.Errorf("unexpected method %q", r.Method)
		}
	}))
	defer srv.Close()

	r, err := probes.Probe067TemplateUploadRoundTrip(context.Background(), newClient(t, srv), opt, "body_weight.v1")
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "pass" {
		t.Errorf("PROBE-067 status = %q (detail: %s)", r.Status, r.Detail)
	}
	if !bytes.Equal(stored, opt) {
		t.Error("upload body did not round-trip server-side")
	}
}

func TestProbe067RejectsServerIDMismatch(t *testing.T) {
	opt := readOPT(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Server returns a different template_id than expected.
		_, _ = w.Write([]byte(`{"template_id":"other_template.v1"}`))
	}))
	defer srv.Close()
	r, err := probes.Probe067TemplateUploadRoundTrip(context.Background(), newClient(t, srv), opt, "body_weight.v1")
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "fail" {
		t.Errorf("expected fail on id mismatch, got %q (detail: %s)", r.Status, r.Detail)
	}
}

func TestProbe067RejectsByteDrift(t *testing.T) {
	opt := readOPT(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"template_id":"body_weight.v1"}`))
		case http.MethodGet:
			// Server reformatted on storage — returns different bytes.
			_, _ = w.Write([]byte(`<?xml version="1.0"?><template/>`))
		}
	}))
	defer srv.Close()
	r, err := probes.Probe067TemplateUploadRoundTrip(context.Background(), newClient(t, srv), opt, "body_weight.v1")
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "fail" {
		t.Errorf("expected fail on byte drift, got %q (detail: %s)", r.Status, r.Detail)
	}
}
