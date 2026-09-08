package probe_test

import (
	"errors"
	"io"
	"net/http"
	"testing"

	"github.com/cadasto/openehr-sdk-go/testkit/probe"
)

func TestReplayer_UnmatchedFailsClosed(t *testing.T) {
	t.Parallel()
	r := probe.NewReplayer(probe.HAR{Log: probe.HARLog{
		Version: "1.2",
		Entries: []probe.HAREntry{{
			Request:  probe.HARRequest{Method: http.MethodGet, URL: "http://cdr.example/openehr/v1/ehr/a"},
			Response: probe.HARResponse{Status: http.StatusOK, Content: probe.HARContent{Text: `{}`}},
		}},
	}})

	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "https://sandbox.local/openehr/v1/ehr", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := r.RoundTrip(req)
	if resp != nil {
		_ = resp.Body.Close()
	}
	if !errors.Is(err, probe.ErrUnmatchedRecording) {
		t.Fatalf("RoundTrip() error = %v, want %v", err, probe.ErrUnmatchedRecording)
	}
}

func TestReplayer_ServesMatchingExchange(t *testing.T) {
	t.Parallel()
	r := probe.NewReplayer(probe.HAR{Log: probe.HARLog{
		Version: "1.2",
		Entries: []probe.HAREntry{{
			Request: probe.HARRequest{
				Method: http.MethodPost,
				URL:    "http://127.0.0.1:8080/ehrbase/rest/openehr/v1/ehr",
			},
			Response: probe.HARResponse{
				Status: http.StatusCreated,
				Headers: []probe.HARHeader{
					{Name: "Content-Type", Value: "application/json"},
					{Name: "ETag", Value: `"abc"`},
				},
				Content: probe.HARContent{Text: `{"_type":"EHR"}`},
			},
		}},
	}})

	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "https://sandbox.local/openehr/v1/ehr", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := r.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != `{"_type":"EHR"}` {
		t.Fatalf("body = %q", body)
	}
	if resp.Header.Get("ETag") != `"abc"` {
		t.Fatalf("ETag = %q", resp.Header.Get("ETag"))
	}

	// Consumed in order: a second identical request must fail closed.
	resp2, err := r.RoundTrip(req)
	if resp2 != nil {
		_ = resp2.Body.Close()
	}
	if !errors.Is(err, probe.ErrUnmatchedRecording) {
		t.Fatalf("second RoundTrip() error = %v, want consumed-match fail-closed", err)
	}
}

// TestReplayer_ConsumesIdenticalMatchesInCaptureOrder pins the order-preserving
// MUST (REQ-082 Cassette mode): two exchanges sharing one method+path must be
// served in capture order. The statuses and bodies differ, so LIFO or any
// reorder is caught — the single-entry ServesMatchingExchange test cannot see
// this, since consume-once holds under any ordering.
func TestReplayer_ConsumesIdenticalMatchesInCaptureOrder(t *testing.T) {
	t.Parallel()
	r := probe.NewReplayer(probe.HAR{Log: probe.HARLog{
		Version: "1.2",
		Entries: []probe.HAREntry{
			{
				Request:  probe.HARRequest{Method: http.MethodPost, URL: "http://cdr.example/openehr/v1/ehr"},
				Response: probe.HARResponse{Status: http.StatusCreated, Content: probe.HARContent{Text: `{"n":1}`}},
			},
			{
				Request:  probe.HARRequest{Method: http.MethodPost, URL: "http://cdr.example/openehr/v1/ehr"},
				Response: probe.HARResponse{Status: http.StatusOK, Content: probe.HARContent{Text: `{"n":2}`}},
			},
		},
	}})
	want := []struct {
		status int
		body   string
	}{{http.StatusCreated, `{"n":1}`}, {http.StatusOK, `{"n":2}`}}
	for i, w := range want {
		req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "https://sandbox.local/openehr/v1/ehr", nil)
		if err != nil {
			t.Fatal(err)
		}
		resp, err := r.RoundTrip(req)
		if err != nil {
			t.Fatalf("match %d: RoundTrip() = %v", i, err)
		}
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode != w.status || string(body) != w.body {
			t.Errorf("match %d: got %d %q, want %d %q (capture order)", i, resp.StatusCode, body, w.status, w.body)
		}
	}
	// Both entries consumed: a third identical request fails closed.
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "https://sandbox.local/openehr/v1/ehr", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := r.RoundTrip(req)
	if resp != nil {
		_ = resp.Body.Close()
	}
	if !errors.Is(err, probe.ErrUnmatchedRecording) {
		t.Fatalf("third RoundTrip() error = %v, want both entries consumed then fail closed", err)
	}
}

// TestReplayer_MethodIsPartOfTheKey pins that the method is part of the replay
// key: a GET recording must not answer a POST of the same stripped path. The
// existing unmatched tests differ in path as well as method, so dropping method
// from replayKey would still pass them; this one would not.
func TestReplayer_MethodIsPartOfTheKey(t *testing.T) {
	t.Parallel()
	r := probe.NewReplayer(probe.HAR{Log: probe.HARLog{
		Version: "1.2",
		Entries: []probe.HAREntry{{
			Request:  probe.HARRequest{Method: http.MethodGet, URL: "http://cdr.example/openehr/v1/ehr"},
			Response: probe.HARResponse{Status: http.StatusOK, Content: probe.HARContent{Text: `{}`}},
		}},
	}})
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "https://sandbox.local/openehr/v1/ehr", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := r.RoundTrip(req)
	if resp != nil {
		_ = resp.Body.Close()
	}
	if !errors.Is(err, probe.ErrUnmatchedRecording) {
		t.Fatalf("POST against a GET-only recording of the same path: error = %v, want %v", err, probe.ErrUnmatchedRecording)
	}
}
