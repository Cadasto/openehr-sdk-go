package transport

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// recorder captures Observations for assertion in tests. Safe for
// concurrent use; the transport calls OnRequest from the goroutine that
// issued Client.Do but tests may run multiple Do calls in parallel.
type recorder struct {
	mu  sync.Mutex
	all []Observation
}

func (r *recorder) OnRequest(o Observation) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.all = append(r.all, o)
}

func (r *recorder) snapshot() []Observation {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Observation, len(r.all))
	copy(out, r.all)
	return out
}

// TestObserverFiresOncePerLogicalCall is the primary acceptance test
// for REQ-098: the Observer MUST fire exactly once per logical Client.Do
// call, after retries settle, with retry-aware Attempts and Duration.
func TestObserverFiresOncePerLogicalCall(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := hits.Add(1)
		if n < 3 {
			w.WriteHeader(503)
			return
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	rec := &recorder{}
	c, _ := New(newCatalog(t, srv),
		WithHTTPClient(srv.Client()),
		WithRetry(RetryPolicy{MaxAttempts: 3, InitialBackoff: time.Millisecond}),
		WithObserver(rec),
	)
	start := time.Now()
	_, err := c.Do(t.Context(), &Request{Method: "GET", Path: "/x", Route: "/x"})
	if err != nil {
		t.Fatal(err)
	}
	calls := rec.snapshot()
	if len(calls) != 1 {
		t.Fatalf("observer fired %d times, want 1", len(calls))
	}
	obs := calls[0]
	if obs.Attempts != 3 {
		t.Errorf("Attempts = %d, want 3 (retry-aware)", obs.Attempts)
	}
	if obs.StatusCode != 200 {
		t.Errorf("StatusCode = %d, want 200", obs.StatusCode)
	}
	if obs.Err != nil {
		t.Errorf("Err = %v, want nil", obs.Err)
	}
	if obs.Duration <= 0 {
		t.Errorf("Duration = %v, want > 0", obs.Duration)
	}
	if obs.Duration > time.Since(start)+50*time.Millisecond {
		t.Errorf("Duration = %v exceeds wall-clock since start", obs.Duration)
	}
	if obs.Method != "GET" || obs.Route != "/x" {
		t.Errorf("Method=%q Route=%q", obs.Method, obs.Route)
	}
}

// TestObserverFiresOnWireError covers the non-2xx branch — the observer
// MUST receive a non-nil Err and the final StatusCode.
func TestObserverFiresOnWireError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
	}))
	defer srv.Close()
	rec := &recorder{}
	c, _ := New(newCatalog(t, srv),
		WithHTTPClient(srv.Client()),
		WithObserver(rec),
	)
	_, _ = c.Do(t.Context(), &Request{Method: "GET", Path: "/missing"})
	calls := rec.snapshot()
	if len(calls) != 1 {
		t.Fatalf("observer fired %d times, want 1", len(calls))
	}
	obs := calls[0]
	if obs.StatusCode != 404 {
		t.Errorf("StatusCode = %d, want 404", obs.StatusCode)
	}
	if obs.Err == nil {
		t.Errorf("Err = nil, want *WireError")
	}
	var we *WireError
	if !errors.As(obs.Err, &we) {
		t.Errorf("Err = %v, want *WireError", obs.Err)
	}
}

// TestObservationTagsRoundTrip covers REQ-098 tags-via-context: per-call
// tags attached with WithObservationTag MUST surface in Observation.Tags.
func TestObservationTagsRoundTrip(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	rec := &recorder{}
	c, _ := New(newCatalog(t, srv),
		WithHTTPClient(srv.Client()),
		WithObserver(rec),
	)
	ctx := WithObservationTag(t.Context(), "phase", "warmup")
	ctx = WithObservationTag(ctx, "scenario", "ehr-create")
	_, err := c.Do(ctx, &Request{Method: "GET", Path: "/x"})
	if err != nil {
		t.Fatal(err)
	}
	calls := rec.snapshot()
	if len(calls) != 1 {
		t.Fatalf("observer fired %d times, want 1", len(calls))
	}
	obs := calls[0]
	if got, _ := obs.Tags["phase"].(string); got != "warmup" {
		t.Errorf("Tags[phase] = %v, want warmup", obs.Tags["phase"])
	}
	if got, _ := obs.Tags["scenario"].(string); got != "ehr-create" {
		t.Errorf("Tags[scenario] = %v, want ehr-create", obs.Tags["scenario"])
	}
}

// TestObservationTagsDefensiveCopy ensures observers cannot mutate the
// caller's context tag map. Important contract for shared observers.
func TestObservationTagsDefensiveCopy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	mutating := observerFunc(func(o Observation) {
		o.Tags["mutated"] = true
	})
	c, _ := New(newCatalog(t, srv),
		WithHTTPClient(srv.Client()),
		WithObserver(mutating),
	)
	ctx := WithObservationTag(t.Context(), "phase", "warmup")
	_, err := c.Do(ctx, &Request{Method: "GET", Path: "/x"})
	if err != nil {
		t.Fatal(err)
	}
	src := observationTagsFromContext(ctx)
	if _, mut := src["mutated"]; mut {
		t.Errorf("observer mutated caller's context tag map")
	}
}

// TestObserverPanicIsRecovered enforces the safety clause from REQ-098:
// a panicking Observer MUST NOT break the request lifecycle.
func TestObserverPanicIsRecovered(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	c, _ := New(newCatalog(t, srv),
		WithHTTPClient(srv.Client()),
		WithObserver(observerFunc(func(Observation) { panic("boom") })),
	)
	_, err := c.Do(t.Context(), &Request{Method: "GET", Path: "/x"})
	if err != nil {
		t.Errorf("panic in observer broke request: %v", err)
	}
}

// TestNilObserverIsNoop covers the "WithObserver(nil) is safe" clause.
func TestNilObserverIsNoop(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	c, _ := New(newCatalog(t, srv),
		WithHTTPClient(srv.Client()),
		WithObserver(nil),
	)
	if _, err := c.Do(t.Context(), &Request{Method: "GET", Path: "/x"}); err != nil {
		t.Errorf("nil observer broke request: %v", err)
	}
}

// observerFunc adapts a bare function to the Observer interface for
// inline test cases. Kept in this file because it has no production
// callers — observers should normally be types with named state.
type observerFunc func(Observation)

func (f observerFunc) OnRequest(o Observation) { f(o) }
