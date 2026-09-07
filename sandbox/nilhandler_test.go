package sandbox_test

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/cadasto/openehr-sdk-go/sandbox"
)

// TestHandleNilFailsClosed pins REQ-025: a nil handler passed to
// [sandbox.Backend.Handle] MUST NOT be silently dropped — a dropped
// registration would make the route look like it never fired, which
// is indistinguishable from "the SDK under test never called it".
// Instead the route is registered and answers 500, naming the
// registration that was nil.
func TestHandleNilFailsClosed(t *testing.T) {
	t.Parallel()
	b := sandbox.New()
	b.Handle(http.MethodGet, "/widget", nil)

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "https://sandbox.local/openehr/v1/widget", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := b.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 for a nil-handler registration", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "nil handler registered for GET /widget") {
		t.Fatalf("body = %q, want it to name the nil registration", body)
	}
}

// TestHandleFuncNilFailsClosed is the [sandbox.Backend.HandleFunc]
// sibling of TestHandleNilFailsClosed.
func TestHandleFuncNilFailsClosed(t *testing.T) {
	t.Parallel()
	b := sandbox.New()
	b.HandleFunc(http.MethodPost, "/widget", nil)

	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "https://sandbox.local/openehr/v1/widget", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := b.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 for a nil-handler registration", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "nil handler registered for POST /widget") {
		t.Fatalf("body = %q, want it to name the nil registration", body)
	}
}

// TestScriptedNilFailsClosed pins the same fail-closed behaviour for
// [sandbox.Scripted], whose only route is registered via HandleFunc.
func TestScriptedNilFailsClosed(t *testing.T) {
	t.Parallel()
	b := sandbox.Scripted(nil)

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "https://sandbox.local/openehr/v1/anything", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := b.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 for sandbox.Scripted(nil)", resp.StatusCode)
	}
}
