package sandbox_test

import (
	"net/http"
	"testing"

	"github.com/cadasto/openehr-sdk-go/sandbox"
)

// TestZeroValueBackend pins that a zero-value Backend is ready to use,
// like [bytes.Buffer] — no [sandbox.New] call required. Before ehrs
// allocated lazily under the lock, this panicked with "assignment to
// entry in nil map" on the first EHR create.
func TestZeroValueBackend(t *testing.T) {
	t.Parallel()
	var b sandbox.Backend
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "https://sandbox.local/openehr/v1/ehr", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := b.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip on a zero-value Backend: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201 from a zero-value Backend", resp.StatusCode)
	}
}
