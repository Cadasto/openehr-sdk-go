package sandbox_test

import (
	"net/http"
	"testing"

	"github.com/cadasto/openehr-sdk-go/sandbox"
)

// TestRecorderSnapshotsHeadersAtWriteHeader pins that the sandbox
// freezes a scripted route's headers where a real server does: at the
// first WriteHeader (or first Write). Handing the live header map to
// the response let a header set *after* WriteHeader appear in Sandbox
// mode and vanish against a real server, whose header block is already
// on the wire — the cross-mode disagreement REQ-082 forbids.
func TestRecorderSnapshotsHeadersAtWriteHeader(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		handle func(http.ResponseWriter)
		header string
		want   string
	}{
		{
			name: "set before WriteHeader is kept",
			handle: func(w http.ResponseWriter) {
				w.Header().Set("X-Early", "1")
				w.WriteHeader(http.StatusOK)
			},
			header: "X-Early",
			want:   "1",
		},
		{
			name: "set after WriteHeader is dropped",
			handle: func(w http.ResponseWriter) {
				w.WriteHeader(http.StatusOK)
				w.Header().Set("X-Late", "1")
			},
			header: "X-Late",
			want:   "",
		},
		{
			name: "set after the first Write is dropped",
			handle: func(w http.ResponseWriter) {
				_, _ = w.Write([]byte("body"))
				w.Header().Set("X-Late", "1")
			},
			header: "X-Late",
			want:   "",
		},
		{
			name: "a handler that never writes still reports its headers",
			handle: func(w http.ResponseWriter) {
				w.Header().Set("X-Silent", "1")
			},
			header: "X-Silent",
			want:   "1",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			b := sandbox.Scripted(func(w http.ResponseWriter, _ *http.Request) { tc.handle(w) })
			resp := roundTrip(t, b, http.MethodGet, "https://sandbox.local/openehr/v1/ehr")
			if got := resp.header.Get(tc.header); got != tc.want {
				t.Errorf("scripted response header %s = %q, want %q", tc.header, got, tc.want)
			}
		})
	}
}

// TestScriptedResponseShape pins that a scripted response carries the
// documented status line and the request it served, matching the
// built-in responses field for field (see TestBuiltinResponseShape).
func TestScriptedResponseShape(t *testing.T) {
	t.Parallel()
	b := sandbox.Scripted(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "https://sandbox.local/openehr/v1/ehr", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := b.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip(GET /ehr): %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if want := "418 I'm a teapot"; resp.Status != want {
		t.Errorf("scripted response Status = %q, want %q", resp.Status, want)
	}
	if resp.Request != req {
		t.Errorf("scripted response Request = %p, want the request that was served (%p)", resp.Request, req)
	}
}

// TestFirstRegisteredRouteWins pins the registration-order rule
// [sandbox.Backend.Handle] documents: when two routes both match one
// request, the one registered first answers.
func TestFirstRegisteredRouteWins(t *testing.T) {
	t.Parallel()
	b := sandbox.New()
	b.HandleFunc(http.MethodGet, "/ehr/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	b.HandleFunc(http.MethodGet, "/ehr/known", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusPreconditionFailed)
	})

	resp := roundTrip(t, b, http.MethodGet, "https://sandbox.local/openehr/v1/ehr/known")
	if resp.status != http.StatusTeapot {
		t.Errorf("RoundTrip(GET /ehr/known) status = %d, want %d from the first-registered matching route (the second would answer %d)",
			resp.status, http.StatusTeapot, http.StatusPreconditionFailed)
	}
}

// TestSubtreeRouteIsAnchored pins that a trailing-slash route matches
// a subtree from the start of the path, not anywhere inside it: "/ehr/"
// must not answer a request for a composition that merely happens to
// carry "/ehr/" further along.
func TestSubtreeRouteIsAnchored(t *testing.T) {
	t.Parallel()
	b := sandbox.New()
	b.HandleFunc(http.MethodGet, "/ehr/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})

	inside := roundTrip(t, b, http.MethodGet, "https://sandbox.local/openehr/v1/ehr/x")
	if inside.status != http.StatusTeapot {
		t.Errorf("RoundTrip(GET /ehr/x) status = %d, want %d — the subtree route owns it", inside.status, http.StatusTeapot)
	}

	outside := roundTrip(t, b, http.MethodGet, "https://sandbox.local/openehr/v1/definition/ehr/x")
	if outside.status == http.StatusTeapot {
		t.Errorf("RoundTrip(GET /definition/ehr/x) status = %d, want the built-in 404 — %q is not under the %q subtree",
			outside.status, "/definition/ehr/x", "/ehr/")
	}
	if outside.status != http.StatusNotFound {
		t.Errorf("RoundTrip(GET /definition/ehr/x) status = %d, want %d", outside.status, http.StatusNotFound)
	}
}
