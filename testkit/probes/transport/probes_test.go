package transportprobes_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	openehrclient "github.com/cadasto/openehr-sdk-go/openehr/client/ehr"
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
	var (
		mu       sync.Mutex
		captured []string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		captured = append(captured, r.URL.EscapedPath())
		mu.Unlock()
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

	snapshot := func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), captured...)
	}
	res, err := probes.Probe091PathSegmentValidation(t.Context(), newClient(t, srv), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != "pass" {
		t.Fatalf("PROBE-091 = %s: %s", res.Status, res.Detail)
	}
	// Only the two positive legs reach the wire: ten leaves × four hostile
	// ids must all fail closed.
	if got := snapshot(); len(got) != 2 {
		t.Errorf("captured %d requests %v, want 2 (the well-formed template id and the service root)", len(got), got)
	}
}

// TestProbe091RejectsMissingInputs pins the probe's own guard rails.
func TestProbe091RejectsMissingInputs(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer srv.Close()

	if _, err := probes.Probe091PathSegmentValidation(t.Context(), nil, func() []string { return nil }); err == nil {
		t.Error("nil client: want an error")
	}
	if _, err := probes.Probe091PathSegmentValidation(t.Context(), newClient(t, srv), nil); err == nil {
		t.Error("nil recorder: want an error")
	}
}

// PROBE-101 (REQ-151) — the ids the fake below keys its two EHR arms on.
const (
	// probe101UndecodableEHR is answered 200 with a body that cannot decode
	// as an EHR.
	probe101UndecodableEHR openehrclient.EHRID = "11111111-1111-4111-8111-111111111111"
	// probe101MissingEHR is answered 404.
	probe101MissingEHR openehrclient.EHRID = "22222222-2222-4222-8222-222222222222"
)

// probe101Arms is what the PROBE-101 fake serves on each of the three routes
// the probe drives. The passing case fills it with the shapes the catalog
// entry names; each planted case below rewrites exactly one arm, so a failure
// is attributable to the arm under test and to nothing else.
type probe101Arms struct {
	undecodableStatus int
	undecodableBody   string // GET /ehr/{probe101UndecodableEHR}
	missingStatus     int
	missingBody       string // GET /ehr/{probe101MissingEHR}
	listStatus        int
	listBody          string // GET /definition/template/adl1.4
}

// probe101Conformant is the backend PROBE-101 is specified against: an
// undecodable 200, a 404, and a list route answering a JSON object where an
// array is expected.
func probe101Conformant() probe101Arms {
	return probe101Arms{
		undecodableStatus: http.StatusOK,
		undecodableBody:   `[1, 2, 3]`,
		missingStatus:     http.StatusNotFound,
		missingBody:       `{"message":"no such ehr"}`,
		listStatus:        http.StatusOK,
		listBody:          `{"templates":[{"template_id":"body_weight.v1"}]}`,
	}
}

// probe101ValidEHR is a body that decodes cleanly as an EHR — what a planted
// backend serves where the probe expects an undecodable one.
const probe101ValidEHR = `{"_type":"EHR","ehr_id":{"value":"11111111-1111-4111-8111-111111111111"},` +
	`"system_id":{"value":"cdr.example"},"time_created":{"value":"2026-01-01T00:00:00Z"}}`

// probe101Server routes by path suffix and records every request path.
func probe101Server(arms probe101Arms, record func(string)) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		record(r.URL.EscapedPath())
		status, body := http.StatusOK, `{}`
		switch {
		case strings.HasSuffix(r.URL.Path, "/ehr/"+string(probe101UndecodableEHR)):
			status, body = arms.undecodableStatus, arms.undecodableBody
		case strings.HasSuffix(r.URL.Path, "/ehr/"+string(probe101MissingEHR)):
			status, body = arms.missingStatus, arms.missingBody
		case strings.HasSuffix(r.URL.Path, "/definition/template/adl1.4"):
			status, body = arms.listStatus, arms.listBody
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
}

// runProbe101 drives the probe against a backend serving arms.
func runProbe101(t *testing.T, arms probe101Arms) (probes.Result, []string) {
	t.Helper()
	var (
		mu       sync.Mutex
		captured []string
	)
	srv := probe101Server(arms, func(p string) {
		mu.Lock()
		captured = append(captured, p)
		mu.Unlock()
	})
	defer srv.Close()

	snapshot := func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), captured...)
	}
	res, err := probes.Probe101DecodeFailureSurfaced(t.Context(), newClient(t, srv), snapshot, probe101UndecodableEHR, probe101MissingEHR)
	if err != nil {
		t.Fatal(err)
	}
	return res, snapshot()
}

// TestProbe101DecodeFailureSurfaced drives PROBE-101 in Sandbox mode against a
// backend that serves all three shapes the catalog entry names (REQ-151).
func TestProbe101DecodeFailureSurfaced(t *testing.T) { // PROBE-101, REQ-151
	res, captured := runProbe101(t, probe101Conformant())
	if res.Status != "pass" {
		t.Fatalf("PROBE-101 = %s: %s", res.Status, res.Detail)
	}
	// One request per arm: every leg must fail on the response it was served,
	// never on a pre-flight refusal that never reached the wire.
	if len(captured) != 3 {
		t.Errorf("captured %d requests %v, want 3 (one per arm)", len(captured), captured)
	}
}

// The three planted backends below each break exactly one arm — a probe that
// cannot fail proves nothing.
func TestProbe101FlagsADecodableBody(t *testing.T) { // PROBE-101, REQ-151
	arms := probe101Conformant()
	arms.undecodableBody = probe101ValidEHR
	res, _ := runProbe101(t, arms)
	if res.Status != "fail" {
		t.Fatalf("PROBE-101 = %s on a 200 the read decoded cleanly, want fail (detail=%q)", res.Status, res.Detail)
	}
	// Matched against the detail this plant must produce, so the case cannot
	// rot into failing for an unrelated reason.
	if !strings.Contains(res.Detail, "does not decode") {
		t.Errorf("detail = %q, want the undecodable-200 arm's detail", res.Detail)
	}
}

// TestProbe101FlagsASilentZeroValue is the defect the probe exists to catch,
// planted as far as a real leaf allows: a 200 body that decodes into a
// zero-valued resource. A leaf that swallowed its decode error would look
// exactly like this to a caller, so the probe must refuse it.
func TestProbe101FlagsASilentZeroValue(t *testing.T) { // PROBE-101, REQ-151
	arms := probe101Conformant()
	arms.undecodableBody = `{}`
	res, _ := runProbe101(t, arms)
	if res.Status != "fail" {
		t.Fatalf("PROBE-101 = %s on a 200 that decoded into a zero-valued EHR, want fail (detail=%q)", res.Status, res.Detail)
	}
	if !strings.Contains(res.Detail, "zero") {
		t.Errorf("detail = %q, want it to name the zero-valued success it caught", res.Detail)
	}
}

func TestProbe101FlagsAServedMissingID(t *testing.T) { // PROBE-101, REQ-151
	arms := probe101Conformant()
	arms.missingStatus, arms.missingBody = http.StatusOK, probe101ValidEHR
	res, _ := runProbe101(t, arms)
	if res.Status != "fail" {
		t.Fatalf("PROBE-101 = %s when the absent-resource arm was answered 200, want fail (detail=%q)", res.Status, res.Detail)
	}
	if !strings.Contains(res.Detail, "404") {
		t.Errorf("detail = %q, want the non-2xx arm's detail", res.Detail)
	}
}

func TestProbe101FlagsADecodableList(t *testing.T) { // PROBE-101, REQ-151
	arms := probe101Conformant()
	arms.listBody = `[{"template_id":"body_weight.v1"}]`
	res, _ := runProbe101(t, arms)
	if res.Status != "fail" {
		t.Fatalf("PROBE-101 = %s on a list body that decoded cleanly, want fail (detail=%q)", res.Status, res.Detail)
	}
	if !strings.Contains(res.Detail, "list leaf") {
		t.Errorf("detail = %q, want the list arm's detail", res.Detail)
	}
}

// TestProbe101RejectsMissingInputs pins the probe's own guard rails.
func TestProbe101RejectsMissingInputs(t *testing.T) { // PROBE-101
	srv := probe101Server(probe101Conformant(), func(string) {})
	defer srv.Close()
	c := newClient(t, srv)
	recorder := func() []string { return nil }

	if _, err := probes.Probe101DecodeFailureSurfaced(t.Context(), nil, recorder, probe101UndecodableEHR, probe101MissingEHR); err == nil {
		t.Error("nil client: want an error")
	}
	if _, err := probes.Probe101DecodeFailureSurfaced(t.Context(), c, nil, probe101UndecodableEHR, probe101MissingEHR); err == nil {
		t.Error("nil recorder: want an error")
	}
	if _, err := probes.Probe101DecodeFailureSurfaced(t.Context(), c, recorder, "", probe101MissingEHR); err == nil {
		t.Error("empty undecodable id: want an error")
	}
	if _, err := probes.Probe101DecodeFailureSurfaced(t.Context(), c, recorder, probe101UndecodableEHR, ""); err == nil {
		t.Error("empty missing id: want an error")
	}
}
