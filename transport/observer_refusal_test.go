package transport

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cadasto/openehr-sdk-go/smart/discovery"
)

// TestREQ098_ObserverFiresOnceForEveryRefusal pins that a Client.Do call
// refused before any attempt fires the observer exactly once, as every
// other call does: Attempts 0, StatusCode 0, Err the error Do returned,
// Duration the time Do took, Method and Route as on any other call, URL
// empty because none was built, and Tags a copy the observer cannot use to
// change the caller's context (REQ-098).
func TestREQ098_ObserverFiresOnceForEveryRefusal(t *testing.T) {
	t.Parallel()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hits.Add(1) }))
	t.Cleanup(srv.Close)

	// A catalog literal can hold an entry with no base URL, which
	// NewStaticCatalog would refuse.
	noBase := &discovery.ServiceCatalog{Services: map[string]discovery.ServiceEntry{discovery.ServiceIDOpenEHRRest: {}}}

	tests := []struct {
		name      string
		catalog   *discovery.ServiceCatalog
		ctxEnded  bool
		req       *Request
		wantErr   error
		wantMeth  string
		wantRoute string
	}{
		{name: "nil request", wantErr: ErrInvalidConfig},
		{
			name: "context already ended", ctxEnded: true, wantErr: context.Canceled,
			req:      &Request{Method: http.MethodGet, Path: "/ehr/abc", Route: "/ehr/{ehr_id}"},
			wantMeth: http.MethodGet, wantRoute: "/ehr/{ehr_id}",
		},
		{
			name: "service missing from the catalog", wantErr: ErrServiceUnavailable,
			req:      &Request{Method: http.MethodGet, ServiceID: "org.example.none", Path: "/ehr/abc", Route: "/ehr/{ehr_id}"},
			wantMeth: http.MethodGet, wantRoute: "/ehr/{ehr_id}",
		},
		{
			name: "invalid path segment", wantErr: ErrInvalidPathSegment,
			req:      &Request{Method: http.MethodGet, Path: "/ehr/..", Route: "/ehr/{ehr_id}"},
			wantMeth: http.MethodGet, wantRoute: "/ehr/{ehr_id}",
		},
		{
			name: "path arity differs from the route", wantErr: ErrInvalidPathSegment,
			req:      &Request{Method: http.MethodPut, Path: "/ehr/a/b", Route: "/ehr/{ehr_id}"},
			wantMeth: http.MethodPut, wantRoute: "/ehr/{ehr_id}",
		},
		{
			name: "unrouted request keeps the Path as Route", wantErr: ErrInvalidPathSegment,
			req:      &Request{Path: "/ehr/abc/"},
			wantMeth: http.MethodGet, wantRoute: "/ehr/abc/",
		},
		{
			name: "service with no base URL", catalog: noBase, wantErr: ErrInvalidConfig,
			req:      &Request{Method: http.MethodGet, Path: "/ehr/abc", Route: "/ehr/{ehr_id}"},
			wantMeth: http.MethodGet, wantRoute: "/ehr/{ehr_id}",
		},
	}
	// The subtests run in parallel, so they start only after this function
	// returns. A check after the loop would always see 0 requests. A cleanup
	// runs once every subtest has finished.
	t.Cleanup(func() {
		if n := hits.Load(); n != 0 {
			t.Errorf("issued %d requests, want 0", n)
		}
	})
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var calls atomic.Int32
			var got Observation
			obs := observerFunc(func(o Observation) {
				calls.Add(1)
				got = o
				o.Tags["mutated"] = true
			})
			cat := tc.catalog
			if cat == nil {
				cat = newCatalog(t, srv)
			}
			c, err := New(cat, WithHTTPClient(srv.Client()), WithObserver(obs))
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			ctx := WithObservationTag(t.Context(), "phase", "refusal")
			if tc.ctxEnded {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}

			start := time.Now()
			resp, err := c.Do(ctx, tc.req)
			elapsed := time.Since(start)

			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("Do error = %v, want errors.Is(err, %v)", err, tc.wantErr)
			}
			if resp != nil {
				t.Errorf("Do Response = %+v, want nil", resp)
			}
			if n := calls.Load(); n != 1 {
				t.Fatalf("observer fired %d times, want 1", n)
			}
			if !errors.Is(got.Err, err) {
				t.Errorf("Observation.Err = %v, want the error Do returned, %v", got.Err, err)
			}
			if got.Attempts != 0 || got.StatusCode != 0 || got.URL != "" {
				t.Errorf("Observation {Attempts: %d, StatusCode: %d, URL: %q}, want {0, 0, \"\"}", got.Attempts, got.StatusCode, got.URL)
			}
			if got.Method != tc.wantMeth || got.Route != tc.wantRoute {
				t.Errorf("Observation {Method: %q, Route: %q}, want {%q, %q}", got.Method, got.Route, tc.wantMeth, tc.wantRoute)
			}
			if got.Duration < 0 || got.Duration > elapsed {
				t.Errorf("Observation.Duration = %v, want between 0 and the %v Do took", got.Duration, elapsed)
			}
			if v, _ := got.Tags["phase"].(string); v != "refusal" {
				t.Errorf("Observation.Tags[phase] = %v, want refusal", got.Tags["phase"])
			}
			if _, mutated := observationTagsFromContext(ctx)["mutated"]; mutated {
				t.Error("the observer changed the caller's context tags")
			}
		})
	}
}

// TestREQ098_PanickingObserverOnARefusalIsRecovered pins that the panic
// recovery covers the refusal path too: Do still returns its refusal.
func TestREQ098_PanickingObserverOnARefusalIsRecovered(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	t.Cleanup(srv.Close)
	c, err := New(newCatalog(t, srv),
		WithHTTPClient(srv.Client()),
		WithObserver(observerFunc(func(Observation) { panic("boom") })),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	_, err = c.Do(t.Context(), &Request{Method: http.MethodGet, Path: "/ehr/..", Route: "/ehr/{ehr_id}"})
	if !errors.Is(err, ErrInvalidPathSegment) {
		t.Errorf("Do error = %v, want the path refusal", err)
	}
}
