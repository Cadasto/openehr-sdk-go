package transport_test

// typed_nil_test.go — REQ-025 § No panics, on the typed-nil axis of the
// token source, reauther and tracer provider a caller hands the transport.
// An interface that holds a nil pointer, func, map, slice or chan is not
// == nil, so it passes a plain nil check and then panics, or blocks, on its
// first method call. New refuses one given to WithTokenSource,
// WithReauthOn401 or WithTracerProvider, and Do
// refuses one attached with auth.WithTokenSource before any request leaves
// the process, without falling back to the client-default token source.

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.opentelemetry.io/otel/trace/noop"

	"github.com/cadasto/openehr-sdk-go/auth"
	"github.com/cadasto/openehr-sdk-go/auth/smart"
	"github.com/cadasto/openehr-sdk-go/smart/discovery"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// ptrTokenSource dereferences its receiver, as most token sources do.
type ptrTokenSource struct{ tok auth.Token }

func (s *ptrTokenSource) Token(context.Context) (auth.Token, error) { return s.tok, nil }

// funcTokenSource calls itself, so a nil one panics.
type funcTokenSource func(context.Context) (auth.Token, error)

func (f funcTokenSource) Token(ctx context.Context) (auth.Token, error) { return f(ctx) }

// mapTokenSource writes to itself, so a nil one panics.
type mapTokenSource map[string]int

func (m mapTokenSource) Token(context.Context) (auth.Token, error) {
	m["calls"]++
	return auth.Token{}, nil
}

// sliceTokenSource indexes itself, so a nil one panics.
type sliceTokenSource []auth.Token

func (s sliceTokenSource) Token(context.Context) (auth.Token, error) { return s[0], nil }

// chanTokenSource receives from itself, so a nil one blocks until ctx ends.
type chanTokenSource chan auth.Token

func (c chanTokenSource) Token(ctx context.Context) (auth.Token, error) {
	select {
	case tok := <-c:
		return tok, nil
	case <-ctx.Done():
		return auth.Token{}, ctx.Err()
	}
}

// ptrReauther dereferences its receiver.
type ptrReauther struct{ calls int }

func (r *ptrReauther) Reauth(context.Context) error {
	r.calls++
	return nil
}

// ptrTracerProvider reaches its embedded provider through its receiver, as
// the OpenTelemetry SDK's *TracerProvider reads its own fields, so a nil one
// panics in Tracer.
type ptrTracerProvider struct{ noop.TracerProvider }

// attemptsObserver records the Attempts of the last observation.
type attemptsObserver struct {
	mu       sync.Mutex
	attempts int
}

func (o *attemptsObserver) OnRequest(obs transport.Observation) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.attempts = obs.Attempts
}

func (o *attemptsObserver) last() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.attempts
}

// typedNilCatalog returns a static catalog whose openEHR REST base is srv.
func typedNilCatalog(t *testing.T, srv *httptest.Server) *discovery.ServiceCatalog {
	t.Helper()
	cat, err := discovery.NewStaticCatalog(discovery.StaticConfig{
		Issuer: srv.URL,
		Services: map[string]discovery.ServiceEntry{
			discovery.ServiceIDOpenEHRRest: {
				BaseURL:     discovery.MustParseURL(srv.URL + "/openehr/v1"),
				SpecVersion: discovery.SpecVersionPin,
			},
		},
	})
	if err != nil {
		t.Fatalf("discovery.NewStaticCatalog() error = %v, want nil", err)
	}
	return cat
}

// countingServer answers every request with status and counts the requests
// and the Authorization headers it saw.
func countingServer(t *testing.T, status int) (*httptest.Server, *atomic.Int32, *atomic.Value) {
	t.Helper()
	var hits atomic.Int32
	var authz atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		authz.Store(r.Header.Get("Authorization"))
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(srv.Close)
	return srv, &hits, &authz
}

// noPanic runs f and turns a panic into an error, so a missing guard fails
// the test that reaches it instead of the whole test binary.
func noPanic(f func() error) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panicked: %v", r)
		}
	}()
	return f()
}

// TestREQ025_NewRefusesATypedNilProvider — REQ-025: New returns an error
// matching ErrInvalidConfig for a token source, reauther or tracer provider
// that holds a nil pointer, func, map, slice or chan, instead of building a
// Client whose first Do panics on it.
func TestREQ025_NewRefusesATypedNilProvider(t *testing.T) {
	t.Parallel()
	srv, _, _ := countingServer(t, http.StatusOK)
	tests := []struct {
		name string
		opt  transport.Option
	}{
		{"token source nil *smart.Source", transport.WithTokenSource((*smart.Source)(nil))},
		{"token source nil pointer", transport.WithTokenSource((*ptrTokenSource)(nil))},
		{"token source nil func", transport.WithTokenSource(funcTokenSource(nil))},
		{"token source nil map", transport.WithTokenSource(mapTokenSource(nil))},
		{"token source nil slice", transport.WithTokenSource(sliceTokenSource(nil))},
		{"token source nil chan", transport.WithTokenSource(chanTokenSource(nil))},
		{"reauther nil *smart.Source", transport.WithReauthOn401((*smart.Source)(nil))},
		{"reauther nil pointer", transport.WithReauthOn401((*ptrReauther)(nil))},
		{"reauther nil auth.ReautherFunc", transport.WithReauthOn401(auth.ReautherFunc(nil))},
		{"tracer provider nil pointer", transport.WithTracerProvider((*ptrTracerProvider)(nil))},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c, err := transport.New(typedNilCatalog(t, srv), transport.WithHTTPClient(srv.Client()), tc.opt)
			if !errors.Is(err, transport.ErrInvalidConfig) {
				t.Fatalf("New(%s) error = %v, want one matching ErrInvalidConfig", tc.name, err)
			}
			if c != nil {
				t.Errorf("New(%s) Client = %v, want nil with the error", tc.name, c)
			}
		})
	}
}

// TestREQ025_NewKeepsAnUntypedNilAsUnset — REQ-025: an untyped nil token
// source or reauther, and a non-nil value of a kind that can be nil, are
// accepted: a nil token source sends no Authorization header, a nil reauther
// leaves a 401 to the caller, and a non-nil map or func is used as given.
func TestREQ025_NewKeepsAnUntypedNilAsUnset(t *testing.T) {
	t.Parallel()

	t.Run("nil token source is anonymous", func(t *testing.T) {
		t.Parallel()
		srv, hits, authz := countingServer(t, http.StatusOK)
		c, err := transport.New(typedNilCatalog(t, srv), transport.WithHTTPClient(srv.Client()), transport.WithTokenSource(nil))
		if err != nil {
			t.Fatalf("New(WithTokenSource(nil)) error = %v, want nil", err)
		}
		if _, err := c.Do(t.Context(), &transport.Request{Path: "/x"}); err != nil {
			t.Fatalf("Do() error = %v, want nil", err)
		}
		if n := hits.Load(); n != 1 {
			t.Errorf("server requests = %d, want 1", n)
		}
		if got, _ := authz.Load().(string); got != "" {
			t.Errorf("Authorization = %q, want none from the anonymous default", got)
		}
	})

	t.Run("nil reauther leaves the 401", func(t *testing.T) {
		t.Parallel()
		srv, hits, _ := countingServer(t, http.StatusUnauthorized)
		c, err := transport.New(typedNilCatalog(t, srv), transport.WithHTTPClient(srv.Client()), transport.WithReauthOn401(nil))
		if err != nil {
			t.Fatalf("New(WithReauthOn401(nil)) error = %v, want nil", err)
		}
		_, err = c.Do(t.Context(), &transport.Request{Path: "/x"})
		if !errors.Is(err, transport.ErrUnauthorized) {
			t.Errorf("Do() error = %v, want one matching ErrUnauthorized", err)
		}
		if n := hits.Load(); n != 1 {
			t.Errorf("server requests = %d, want 1 (no reauth retry)", n)
		}
	})

	t.Run("non-nil map and func are used", func(t *testing.T) {
		t.Parallel()
		srv, _, authz := countingServer(t, http.StatusOK)
		m := mapTokenSource{}
		c, err := transport.New(typedNilCatalog(t, srv), transport.WithHTTPClient(srv.Client()),
			transport.WithTokenSource(m),
			transport.WithReauthOn401(auth.ReautherFunc(func(context.Context) error { return nil })),
		)
		if err != nil {
			t.Fatalf("New(non-nil map token source, non-nil ReautherFunc) error = %v, want nil", err)
		}
		if _, err := c.Do(t.Context(), &transport.Request{Path: "/x"}); err != nil {
			t.Fatalf("Do() error = %v, want nil", err)
		}
		if m["calls"] != 1 {
			t.Errorf("map token source calls = %d, want 1", m["calls"])
		}
		fn := funcTokenSource(func(context.Context) (auth.Token, error) { return auth.Token{Value: "fn-tok"}, nil })
		ctx := auth.WithTokenSource(t.Context(), fn)
		if _, err := c.Do(ctx, &transport.Request{Path: "/x"}); err != nil {
			t.Fatalf("Do(non-nil func override) error = %v, want nil", err)
		}
		if got, _ := authz.Load().(string); got != "Bearer fn-tok" {
			t.Errorf("Authorization = %q, want %q from the func override", got, "Bearer fn-tok")
		}
	})
}

// TestREQ025_DoRefusesATypedNilPerRequestTokenSource — REQ-025: a typed-nil
// token source attached with auth.WithTokenSource makes Do fail with an error
// matching ErrInvalidConfig before any request leaves the process. It never
// falls back to the client-default token source, which would send the
// Client's own credential on a request meant to carry the caller's, and it is
// not retried, since every attempt would fail the same way.
func TestREQ025_DoRefusesATypedNilPerRequestTokenSource(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		ts   auth.TokenSource
	}{
		{"nil *smart.Source", (*smart.Source)(nil)},
		{"nil pointer", (*ptrTokenSource)(nil)},
		{"nil func", funcTokenSource(nil)},
		{"nil map", mapTokenSource(nil)},
		{"nil slice", sliceTokenSource(nil)},
		{"nil chan", chanTokenSource(nil)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv, hits, _ := countingServer(t, http.StatusOK)
			obs := &attemptsObserver{}
			c, err := transport.New(typedNilCatalog(t, srv),
				transport.WithHTTPClient(srv.Client()),
				transport.WithTokenSource(auth.StaticTokenSource(auth.Token{Value: "client-default"})),
				transport.WithRetry(transport.RetryPolicy{MaxAttempts: 3, InitialBackoff: time.Millisecond, MaxBackoff: time.Millisecond}),
				transport.WithObserver(obs),
			)
			if err != nil {
				t.Fatalf("New() error = %v, want nil", err)
			}
			// A nil chan blocks until ctx ends, so a bounded ctx turns a
			// missing guard into a failure instead of a hung test.
			ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
			defer cancel()
			ctx = auth.WithTokenSource(ctx, tc.ts)
			err = noPanic(func() error {
				_, err := c.Do(ctx, &transport.Request{Path: "/x"})
				return err
			})
			if !errors.Is(err, transport.ErrInvalidConfig) {
				t.Errorf("Do(override %s) error = %v, want one matching ErrInvalidConfig", tc.name, err)
			}
			if n := hits.Load(); n != 0 {
				t.Errorf("server requests = %d, want 0: the request must not leave the process, nor fall back to the client-default token", n)
			}
			if got := obs.last(); got != 1 {
				t.Errorf("Observation.Attempts = %d, want 1: a typed-nil override fails the same way on every attempt", got)
			}
		})
	}
}
