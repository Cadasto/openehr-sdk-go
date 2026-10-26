package transport

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/cadasto/openehr-sdk-go/auth"
	"github.com/cadasto/openehr-sdk-go/smart/discovery"
)

// TestNewErrorTextsCarryNoRequirementIDs pins the whole text of New's two
// refusals: the sentinel and what is missing, with no internal requirement
// id, which means nothing to the caller who reads it (REQ-021, REQ-025).
func TestNewErrorTextsCarryNoRequirementIDs(t *testing.T) {
	t.Parallel()
	cat, err := discovery.NewStaticCatalog(discovery.StaticConfig{Issuer: "https://idp.example", Services: map[string]discovery.ServiceEntry{}})
	if err != nil {
		t.Fatalf("NewStaticCatalog: %v", err)
	}
	tests := []struct {
		name    string
		catalog *discovery.ServiceCatalog
		opts    []Option
		want    string
	}{
		{name: "nil catalog", opts: []Option{WithHTTPClient(http.DefaultClient)}, want: "transport: invalid configuration: ServiceCatalog is required"},
		{name: "no HTTP client", catalog: cat, want: "transport: invalid configuration: WithHTTPClient is required"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := New(tc.catalog, tc.opts...)
			if !errors.Is(err, ErrInvalidConfig) {
				t.Fatalf("New error = %v, want ErrInvalidConfig", err)
			}
			if err.Error() != tc.want {
				t.Errorf("New error = %q, want %q", err.Error(), tc.want)
			}
		})
	}
}

// logRecord is one record a recordingHandler kept, with the logCtxKey value
// of the context it was logged with.
type logRecord struct {
	marker any
	msg    string
	attrs  map[string]string
}

// recordingHandler keeps every record and the marker of its context.
type recordingHandler struct {
	mu      sync.Mutex
	records []logRecord
}

func (h *recordingHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *recordingHandler) Handle(ctx context.Context, r slog.Record) error {
	rec := logRecord{marker: ctx.Value(logCtxKey{}), msg: r.Message, attrs: map[string]string{}}
	r.Attrs(func(a slog.Attr) bool {
		rec.attrs[a.Key] = a.Value.String()
		return true
	})
	h.mu.Lock()
	h.records = append(h.records, rec)
	h.mu.Unlock()
	return nil
}

func (h *recordingHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *recordingHandler) WithGroup(string) slog.Handler      { return h }

func (h *recordingHandler) snapshot() []logRecord {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]logRecord(nil), h.records...)
}

type logCtxKey struct{}

// TestREQ098_ObserverPanicLogNamesTheRouteTemplate pins the log of a
// recovered observer panic: it is logged with the call's context, and names
// the request by its route template, or "(unrouted)" when it has none, never
// by the Path, which can carry a caller-supplied identifier. A nil Request,
// which Do refuses, is logged as unrouted (REQ-098, REQ-093).
func TestREQ098_ObserverPanicLogNamesTheRouteTemplate(t *testing.T) {
	t.Parallel()
	const ehrID = "7d44b88c-4199-4bad-97dc-d78268e01398"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)

	tests := []struct {
		name      string
		req       *Request
		wantRoute string
	}{
		{name: "routed", req: &Request{Method: http.MethodGet, Path: "/ehr/" + ehrID, Route: "/ehr/{ehr_id}"}, wantRoute: "/ehr/{ehr_id}"},
		{name: "unrouted", req: &Request{Method: http.MethodGet, Path: "/ehr/" + ehrID}, wantRoute: unroutedRoute},
		{name: "nil request", wantRoute: unroutedRoute},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := &recordingHandler{}
			c, err := New(newCatalog(t, srv),
				WithHTTPClient(srv.Client()),
				WithLogger(slog.New(h)),
				WithObserver(observerFunc(func(Observation) { panic("boom") })),
			)
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			ctx := context.WithValue(t.Context(), logCtxKey{}, "marker")
			_, _ = c.Do(ctx, tc.req)

			recs := h.snapshot()
			if len(recs) != 1 {
				t.Fatalf("logged %d records, want 1", len(recs))
			}
			rec := recs[0]
			if rec.msg != "transport: observer panicked" {
				t.Errorf("log message = %q, want %q", rec.msg, "transport: observer panicked")
			}
			if got := rec.attrs["route"]; got != tc.wantRoute {
				t.Errorf("log route = %q, want %q", got, tc.wantRoute)
			}
			if rec.marker != "marker" {
				t.Error("the panic was not logged with the call's context")
			}
		})
	}
}

// TestREQ060_TokenReplacesCallerAuthorization pins what the Request.Headers
// doc says: a caller's own Authorization is sent only when no token is
// attached; a token that is not zero replaces it, and NoAuth or the zero
// Token leave it in place (REQ-060).
func TestREQ060_TokenReplacesCallerAuthorization(t *testing.T) {
	t.Parallel()
	var (
		mu  sync.Mutex
		got string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		got = r.Header.Get("Authorization")
		mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)
	const callerAuth = "Basic Y2FsbGVyOnB3"
	tests := []struct {
		name   string
		ts     auth.TokenSource
		noAuth bool
		want   string
	}{
		{name: "token replaces the caller's header", ts: auth.StaticTokenSource(auth.Token{Value: "tok"}), want: "Bearer tok"},
		{name: "NoAuth keeps the caller's header", ts: auth.StaticTokenSource(auth.Token{Value: "tok"}), noAuth: true, want: callerAuth},
		{name: "zero token keeps the caller's header", ts: auth.AnonymousTokenSource(), want: callerAuth},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c, err := New(newCatalog(t, srv), WithHTTPClient(srv.Client()), WithTokenSource(tc.ts))
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			req := &Request{Method: http.MethodGet, Path: "/x", Route: "/x", NoAuth: tc.noAuth, Headers: http.Header{"Authorization": {callerAuth}}}
			if _, err := c.Do(t.Context(), req); err != nil {
				t.Fatalf("Do: %v", err)
			}
			mu.Lock()
			defer mu.Unlock()
			if got != tc.want {
				t.Errorf("Authorization = %q, want %q", got, tc.want)
			}
		})
	}
}

// runeSegmentFault is segmentFault as it was written over runes; the
// equivalence test holds the byte loop to it.
func runeSegmentFault(s string) string {
	switch s {
	case "":
		return "empty segment"
	case ".", "..":
		return `dot segment ("." or "..")`
	}
	for i, r := range s {
		switch {
		case r == '/':
			return fmt.Sprintf(`separator "/" at index %d`, i)
		case r == '\\':
			return fmt.Sprintf("backslash at index %d", i)
		case r < 0x20 || r == 0x7F:
			return fmt.Sprintf("control byte at index %d", i)
		}
	}
	return ""
}

// TestSegmentFaultByteLoopMatchesRuneLoop pins that ranging the bytes, as
// idiom.md § API conventions asks of a byte check (String iteration), answers
// exactly as the rune loop did: every byte it looks for is ASCII, so
// multi-byte characters and invalid UTF-8 change nothing (REQ-150).
func TestSegmentFaultByteLoopMatchesRuneLoop(t *testing.T) {
	t.Parallel()
	for _, s := range []string{
		"", ".", "..", "...", "a", "Blood Pressure.v1", "a/b", `a\b`, "a\x00", "a\x1f", "a\x7f", "a\x20",
		"Ä/", "\xff/", "\xc3", "\xc3/", "\xe2\x82", "\xe2\x82\x00", "€\\", "\xed\xa0\x80\x01",
	} {
		if got, want := segmentFault(s), runeSegmentFault(s); got != want {
			t.Errorf("segmentFault(%q) = %q, the rune loop gives %q", s, got, want)
		}
	}
	r := rand.New(rand.NewPCG(1, 2))
	alphabet := []byte{0x00, 0x1F, 0x20, '/', '\\', '.', 'a', 0x7F, 0x80, 0xBF, 0xC2, 0xC3, 0xE0, 0xE2, 0xED, 0xF0, 0xF4, 0xFF}
	for range 100_000 {
		b := make([]byte, r.IntN(8))
		for i := range b {
			if r.IntN(2) == 0 {
				b[i] = alphabet[r.IntN(len(alphabet))]
			} else {
				b[i] = byte(r.IntN(256))
			}
		}
		s := string(b)
		if got, want := segmentFault(s), runeSegmentFault(s); got != want {
			t.Fatalf("segmentFault(%q) = %q, the rune loop gives %q", s, got, want)
		}
	}
}
