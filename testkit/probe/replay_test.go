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
	_, err = r.RoundTrip(req)
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
	defer resp.Body.Close()
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
	_, err = r.RoundTrip(req)
	if !errors.Is(err, probe.ErrUnmatchedRecording) {
		t.Fatalf("second RoundTrip() error = %v, want consumed-match fail-closed", err)
	}
}
