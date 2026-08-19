package transportprobes_test

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/cadasto/openehr-sdk-go/smart/discovery"
	probes "github.com/cadasto/openehr-sdk-go/testkit/probes/transport"
	"github.com/cadasto/openehr-sdk-go/transport"
)

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
		t.Fatal(err)
	}
	c, err := transport.New(cat, transport.WithHTTPClient(srv.Client()))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// TestProbe091PathSegmentValidation drives PROBE-091 in Sandbox mode. The
// fake answers anything it is asked with a body the positive legs can
// decode; every hostile leg is expected never to reach it (REQ-150).
func TestProbe091PathSegmentValidation(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		switch r.Method {
		case http.MethodOptions:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"solution":"test","vendor":"test","restapi_specs_version":"1.1.0-development"}`))
		default:
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write([]byte(`<template/>`))
		}
	}))
	defer srv.Close()

	res, err := probes.Probe091PathSegmentValidation(t.Context(), newClient(t, srv), func() int {
		return int(hits.Load())
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != "pass" {
		t.Fatalf("PROBE-091 = %s: %s", res.Status, res.Detail)
	}
	// Only the two positive legs reach the wire: nine leaves × two hostile
	// ids must all fail closed.
	if n := hits.Load(); n != 2 {
		t.Errorf("captured %d requests, want 2 (the well-formed template id and the service root)", n)
	}
}

// TestProbe091RejectsMissingInputs pins the probe's own guard rails.
func TestProbe091RejectsMissingInputs(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer srv.Close()

	if _, err := probes.Probe091PathSegmentValidation(t.Context(), nil, func() int { return 0 }); err == nil {
		t.Error("nil client: want an error")
	}
	if _, err := probes.Probe091PathSegmentValidation(t.Context(), newClient(t, srv), nil); err == nil {
		t.Error("nil counter: want an error")
	}
}
