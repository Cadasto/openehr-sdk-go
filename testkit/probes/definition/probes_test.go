package definitionprobes_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
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

// TestProbe093TemplateListFilters drives PROBE-093 in Sandbox mode: the
// fake records every request's query and answers each list call with a
// template-metadata body (REQ-143).
func TestProbe093TemplateListFilters(t *testing.T) {
	var captured []url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = append(captured, r.URL.Query())
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"template_id":"vital_signs.v1","concept":"Vital Signs"}]`))
	}))
	defer srv.Close()

	res, err := probes.Probe093TemplateListFilters(t.Context(), newClient(t, srv), &captured)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != "pass" {
		t.Fatalf("PROBE-093 = %s: %s", res.Status, res.Detail)
	}
	// Three of the four legs reach the wire; the negative-paging leg must not.
	if len(captured) != 3 {
		t.Errorf("captured %d requests, want 3 (unfiltered, filtered, zero-paging)", len(captured))
	}
}

// TestProbe093EmptyCatalogPasses pins that an empty catalog is a pass, not
// a decode failure: REQ-143 licenses no assertion that a filtered
// deployment holds templates. This test says nothing about the shape of the
// returned slice — the probe discards it entirely and asserts only on the
// error and the captured queries. The non-nil zero-length slice REQ-144
// requires on an empty 2xx body is pinned by TestListTemplatesEmpty and
// TestListStoredQueriesEmpty in openehr/client/definition.
func TestProbe093EmptyCatalogPasses(t *testing.T) {
	var captured []url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = append(captured, r.URL.Query())
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	res, err := probes.Probe093TemplateListFilters(t.Context(), newClient(t, srv), &captured)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != "pass" {
		t.Fatalf("PROBE-093 against an empty catalog = %s: %s", res.Status, res.Detail)
	}
}

// TestProbe093RejectsMissingInputs pins the probe's own guard rails.
func TestProbe093RejectsMissingInputs(t *testing.T) {
	var captured []url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()

	if _, err := probes.Probe093TemplateListFilters(t.Context(), nil, &captured); err == nil {
		t.Error("nil client: want an error")
	}
	if _, err := probes.Probe093TemplateListFilters(t.Context(), newClient(t, srv), nil); err == nil {
		t.Error("nil recorder: want an error")
	}
}
