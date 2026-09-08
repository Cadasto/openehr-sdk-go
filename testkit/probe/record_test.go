package probe_test

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/testkit/probe"
)

func TestRecorder_RedactsAuthorization(t *testing.T) {
	t.Parallel()
	rec := probe.NewRecorder(roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Header.Get("Authorization") == "" {
			t.Fatal("recorder must forward Authorization to the next hop; redaction is on the recording")
		}
		return &http.Response{
			StatusCode: http.StatusCreated,
			Header:     http.Header{"Set-Cookie": []string{"sid=abc"}, "ETag": []string{`"x"`}},
			Body:       io.NopCloser(strings.NewReader(`{"_type":"EHR"}`)),
			Request:    req,
		}, nil
	}), probe.HARProvenance{
		Deployment: "ehrbase-local",
		BaseURL:    "http://127.0.0.1:8080/ehrbase/rest/openehr/v1",
		CapturedAt: "2026-09-07T18:53:48Z",
		SDKCommit:  "deadbeef",
	})

	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "http://127.0.0.1:8080/ehrbase/rest/openehr/v1/ehr?access_token=secret", strings.NewReader(""))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer secret-token")
	req.Header.Set("Cookie", "sid=abc")
	resp, err := rec.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201", resp.StatusCode)
	}

	har := rec.HAR()
	if har.Log.Req082 == nil || !har.Log.Req082.Redaction.Ran {
		t.Fatal("HAR log._req082.redaction.ran is not true")
	}
	if len(har.Log.Entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(har.Log.Entries))
	}
	e := har.Log.Entries[0]
	for _, h := range e.Request.Headers {
		switch strings.ToLower(h.Name) {
		case "authorization", "cookie", "proxy-authorization":
			t.Fatalf("request header %q survived redaction", h.Name)
		}
	}
	for _, h := range e.Response.Headers {
		if strings.EqualFold(h.Name, "set-cookie") {
			t.Fatalf("response header %q survived redaction", h.Name)
		}
	}
	if strings.Contains(strings.ToLower(e.Request.URL), "access_token") {
		t.Fatalf("request URL still carries access_token: %s", e.Request.URL)
	}

	path := filepath.Join(t.TempDir(), "capture.har")
	data, err := json.Marshal(har)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := probe.ValidateHAR(path); err != nil {
		t.Fatalf("ValidateHAR(recorded) = %v, want a recording the runner would accept", err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
