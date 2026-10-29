package admin_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/cadasto/openehr-sdk-go/cadasto/admin"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// statusHeaderSecret stands for a header value the server sends beside a
// status that is not 2xx. It must never reach an error's text.
const statusHeaderSecret = "HEADER-SECRET-0815"

// statusSentinels are the sentinels a probe status error can unwrap to.
// Each error matches exactly one of them.
var statusSentinels = []error{
	transport.ErrUnauthorized,
	transport.ErrForbidden,
	transport.ErrNotFound,
	transport.ErrServerError,
	admin.ErrUnexpectedStatus,
}

// secretStatusHandler answers every request with status, a header that
// carries statusHeaderSecret and a body that carries readySecret.
func secretStatusHandler(status int) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Probe-Note", statusHeaderSecret)
		w.WriteHeader(status)
		_, _ = io.WriteString(w, secretDoc)
	}
}

// TestProbeStatusError pins the status mapping as a value: every status
// that is not 2xx gives a *StatusError that names the probe path and the
// status, unwraps to the mapped transport sentinel or to
// ErrUnexpectedStatus, and whose text carries no body byte or header value.
func TestProbeStatusError(t *testing.T) { // REQ-083
	const (
		livePath  = "/probe/live"
		readyPath = "/probe/ready"
	)
	probes := []struct {
		name string
		path string
		call probeCall
	}{
		{"Live", livePath, func(ctx context.Context, c *transport.Client) ([]byte, error) {
			return nil, admin.Live(ctx, c, admin.WithLivePath(livePath))
		}},
		{"Ready", readyPath, func(ctx context.Context, c *transport.Client) ([]byte, error) {
			return nil, admin.Ready(ctx, c, admin.WithReadyPath(readyPath))
		}},
		{"ReadyBody", readyPath, func(ctx context.Context, c *transport.Client) ([]byte, error) {
			return admin.ReadyBody(ctx, c, admin.WithReadyPath(readyPath))
		}},
	}
	cases := []struct {
		status   int
		sentinel error
	}{
		{http.StatusBadRequest, admin.ErrUnexpectedStatus},
		{http.StatusUnauthorized, transport.ErrUnauthorized},
		{http.StatusForbidden, transport.ErrForbidden},
		{http.StatusNotFound, transport.ErrNotFound},
		{http.StatusMethodNotAllowed, admin.ErrUnexpectedStatus},
		{http.StatusRequestTimeout, admin.ErrUnexpectedStatus},
		{http.StatusTooManyRequests, admin.ErrUnexpectedStatus},
		{http.StatusInternalServerError, transport.ErrServerError},
		{http.StatusServiceUnavailable, transport.ErrServerError},
	}
	for _, p := range probes {
		for _, tc := range cases {
			t.Run(p.name+"/"+strconv.Itoa(tc.status), func(t *testing.T) {
				srv := httptest.NewServer(secretStatusHandler(tc.status))
				defer srv.Close()
				c := newClient(t, srv)

				got, err := p.call(t.Context(), c)
				if got != nil {
					t.Errorf("%s: status=%d returned %q, want no bytes", p.name, tc.status, got)
				}
				if err == nil {
					t.Fatalf("%s: status=%d returned no error", p.name, tc.status)
				}
				serr, ok := errors.AsType[*admin.StatusError](err)
				if !ok || serr == nil {
					t.Fatalf("%s: status=%d: errors.AsType[*admin.StatusError](%v) = %v, %t, want a non-nil *StatusError", p.name, tc.status, err, serr, ok)
				}
				if serr.StatusCode != tc.status {
					t.Errorf("%s: StatusCode = %d, want %d", p.name, serr.StatusCode, tc.status)
				}
				if serr.Path != p.path {
					t.Errorf("%s: Path = %q, want %q", p.name, serr.Path, p.path)
				}
				for _, s := range statusSentinels {
					if got, want := errors.Is(err, s), errors.Is(s, tc.sentinel); got != want {
						t.Errorf("%s: status=%d: errors.Is(err, %v) = %t, want %t", p.name, tc.status, s, got, want)
					}
				}
				want := fmt.Sprintf("admin: probe %s status=%d: %s", p.path, tc.status, tc.sentinel)
				if errors.Is(tc.sentinel, admin.ErrUnexpectedStatus) {
					want = fmt.Sprintf("admin: probe %s: unexpected status %d", p.path, tc.status)
				}
				if msg := err.Error(); msg != want {
					t.Errorf("%s: err.Error() = %q, want %q", p.name, msg, want)
				}
				assertNoServerValues(t, err.Error(), statusHeaderSecret)
			})
		}
	}
}

// TestStatusErrorNilReceiver pins that a nil *StatusError, such as the one
// a failed errors.AsType leaves behind, prints and unwraps without a panic.
func TestStatusErrorNilReceiver(t *testing.T) { // REQ-083
	var e *admin.StatusError
	if msg := e.Error(); msg == "" {
		t.Error("(*StatusError)(nil).Error() = \"\", want a fixed text")
	}
	if got := e.Unwrap(); got != nil {
		t.Errorf("(*StatusError)(nil).Unwrap() = %v, want nil", got)
	}
}
