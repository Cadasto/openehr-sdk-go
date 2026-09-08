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

// TestRecorder_StripsURLUserinfo pins that user:pass@ authority credentials are
// stripped at capture time, not merely refused on load: the recorder writes
// redaction.ran=true, so a userinfo leftover would be an attested-but-unredacted
// recording. Deleting the u.User strip makes the recorded URL keep the userinfo
// and ValidateHAR then refuses the file — this test catches both.
func TestRecorder_StripsURLUserinfo(t *testing.T) {
	t.Parallel()
	rec := probe.NewRecorder(roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{}`)),
			Request:    req,
		}, nil
	}), probe.HARProvenance{
		Deployment: "ehrbase-local",
		BaseURL:    "http://127.0.0.1:8080/ehrbase/rest/openehr/v1",
		CapturedAt: "2026-09-07T18:53:48Z",
		SDKCommit:  "deadbeef",
	})

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://user:pass@127.0.0.1:8080/ehrbase/rest/openehr/v1/ehr/x", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := rec.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()

	har := rec.HAR()
	if got := har.Log.Entries[0].Request.URL; strings.Contains(got, "pass@") || strings.Contains(got, "user:") {
		t.Fatalf("recorded URL still carries userinfo: %s", got)
	}
	path := filepath.Join(t.TempDir(), "userinfo.har")
	data, err := json.Marshal(har)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := probe.ValidateHAR(path); err != nil {
		t.Fatalf("ValidateHAR(recorded) = %v, want the userinfo-stripped recording accepted", err)
	}
}

// TestRecorder_BodyCredentialIsRefusedByValidateHAR pins the division of labour:
// bodies are not a capture-time redaction channel, so a credential in a request
// body survives into the recording — and must then be caught when the file is
// loaded (REQ-082 / REQ-093).
func TestRecorder_BodyCredentialIsRefusedByValidateHAR(t *testing.T) {
	t.Parallel()
	rec := probe.NewRecorder(roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{}`)),
			Request:    req,
		}, nil
	}), probe.HARProvenance{
		Deployment: "ehrbase-local",
		BaseURL:    "http://127.0.0.1:8080/ehrbase/rest/openehr/v1",
		CapturedAt: "2026-09-07T18:53:48Z",
		SDKCommit:  "deadbeef",
	})

	body := `{"token":"Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.payloadsegment"}`
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "http://127.0.0.1:8080/ehrbase/rest/openehr/v1/ehr", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp, err := rec.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()

	path := filepath.Join(t.TempDir(), "bodycred.har")
	data, err := json.Marshal(rec.HAR())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := probe.ValidateHAR(path); err == nil {
		t.Fatal("ValidateHAR accepted a recording whose request body carries a Bearer credential")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
