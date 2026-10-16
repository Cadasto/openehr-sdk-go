package smart_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/cadasto/openehr-sdk-go/auth"
	"github.com/cadasto/openehr-sdk-go/auth/smart"
	"github.com/cadasto/openehr-sdk-go/smart/discovery"
)

// scriptedServer is an in-memory authorization server whose answers the
// test scripts: refresh answers the n-th token request and keySet the n-th
// key-set fetch, each counted from 1. arrived, when not nil, is sent to as
// each request arrives and dropped when its buffer is full. Nothing goes
// over a network, so a synctest bubble sees every wait inside it.
type scriptedServer struct {
	arrived chan struct{}
	refresh func(r *http.Request, n int) (*http.Response, error)
	keySet  func(r *http.Request, n int) (*http.Response, error)

	mu        sync.Mutex
	refreshes int
	fetches   int
}

func (s *scriptedServer) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.Body != nil {
		_, _ = io.Copy(io.Discard, r.Body)
		_ = r.Body.Close()
	}
	s.mu.Lock()
	answer, n := s.refresh, 0
	if r.URL.Path == "/jwks" {
		s.fetches++
		answer, n = s.keySet, s.fetches
	} else {
		s.refreshes++
		n = s.refreshes
	}
	s.mu.Unlock()
	if s.arrived != nil {
		select {
		case s.arrived <- struct{}{}:
		default:
		}
	}
	return answer(r, n)
}

// counts returns how many token requests and key-set fetches arrived.
func (s *scriptedServer) counts() (refreshes, fetches int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.refreshes, s.fetches
}

// scriptedAnswer is a JSON response to r.
func scriptedAnswer(r *http.Request, status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": {"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    r,
	}
}

const (
	freshTokenBody = `{"access_token":"at-new","token_type":"Bearer","expires_in":3600}`
	outageBody     = `{"error":"temporarily_unavailable"}`
)

// errTransportBug is the value a scripted request panics with.
var errTransportBug = errors.New("transport bug")

// scriptedSource is a public-client source whose requests srv answers.
func scriptedSource(t *testing.T, srv *scriptedServer, opts ...smart.Option) *smart.Source {
	t.Helper()
	src, err := newSource("client-id", discovery.AuthEndpoints{
		AuthorizationEndpoint: discovery.MustParseURL("https://idp.test/authorize"),
		TokenEndpoint:         discovery.MustParseURL("https://idp.test/token"),
	}, append([]smart.Option{
		smart.WithHTTPClient(&http.Client{Transport: srv}),
		smart.WithRedirectURI("https://app.example/callback"),
		smart.WithIssuer("https://idp.test"),
	}, opts...)...)
	if err != nil {
		t.Fatal(err)
	}
	return src
}

// scriptedJWKS is a key set whose fetches srv answers.
func scriptedJWKS(t *testing.T, srv *scriptedServer) *smart.JWKS {
	t.Helper()
	jwks, err := smart.NewJWKS(&http.Client{Transport: srv}, "https://idp.test/jwks")
	if err != nil {
		t.Fatal(err)
	}
	return jwks
}

// TestREQ026_RefreshLeaderCancelDoesNotFailWaiters pins REQ-026 (idiom.md §
// Concurrency) with REQ-063: when the call leading a shared refresh gives up
// and that alone fails the refresh, a call waiting on it with a live context
// does not get the leader's cancellation. It refreshes again and gets the
// new token.
func TestREQ026_RefreshLeaderCancelDoesNotFailWaiters(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		srv := &scriptedServer{arrived: make(chan struct{}, 1)}
		srv.refresh = func(r *http.Request, n int) (*http.Response, error) {
			if n == 1 {
				// The leader's refresh is held until the leader gives up.
				<-r.Context().Done()
				return nil, r.Context().Err()
			}
			return scriptedAnswer(r, http.StatusOK, freshTokenBody), nil
		}
		src := scriptedSource(t, srv)
		src.SetTokens(staleAccess("at-old"), "rt-0")

		leadCtx, cancelLead := context.WithCancel(t.Context())
		var (
			wg               sync.WaitGroup
			leadErr, waitErr error
			waitTok          auth.Token
		)
		wg.Go(func() { _, leadErr = src.Token(leadCtx) })
		<-srv.arrived // the leader's refresh is held
		wg.Go(func() { waitTok, waitErr = src.Token(t.Context()) })
		synctest.Wait() // the second call waits on the leader's refresh
		cancelLead()
		wg.Wait()

		if !errors.Is(leadErr, context.Canceled) {
			t.Errorf("leading Token() error = %v, want context.Canceled", leadErr)
		}
		if waitErr != nil || waitTok.Value != "at-new" {
			t.Errorf("waiting Token() = (%q, %v), want (at-new, nil): the leader's cancellation is not the waiter's", waitTok.Value, waitErr)
		}
		if n, _ := srv.counts(); n != 2 {
			t.Errorf("refresh requests = %d, want 2 (the cancelled one, the waiter's own)", n)
		}
	})
}

// TestREQ026_RefreshFailureReachesWaitersAfterLeaderCancel pins REQ-026 with
// REQ-063: a refresh that fails for a reason of its own reaches the calls
// waiting on it, even when the leader's context ended meanwhile. Only a
// failure the leader's ending caused stays the leader's.
func TestREQ026_RefreshFailureReachesWaitersAfterLeaderCancel(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		release := make(chan struct{})
		srv := &scriptedServer{arrived: make(chan struct{}, 1)}
		srv.refresh = func(r *http.Request, n int) (*http.Response, error) {
			<-release // answers without watching the request's context
			return scriptedAnswer(r, http.StatusServiceUnavailable, outageBody), nil
		}
		src := scriptedSource(t, srv)
		src.SetTokens(staleAccess("at-old"), "rt-0")

		leadCtx, cancelLead := context.WithCancel(t.Context())
		var (
			wg               sync.WaitGroup
			leadErr, waitErr error
		)
		wg.Go(func() { _, leadErr = src.Token(leadCtx) })
		<-srv.arrived
		wg.Go(func() { _, waitErr = src.Token(t.Context()) })
		synctest.Wait()
		cancelLead()
		synctest.Wait() // the leader's context has ended; its request is still out
		close(release)
		wg.Wait()

		for _, c := range []struct {
			call string
			err  error
		}{{"leading Token()", leadErr}, {"waiting Token()", waitErr}} {
			ex, ok := errors.AsType[*auth.ExchangeError](c.err)
			if !ok || ex.StatusCode != http.StatusServiceUnavailable || !errors.Is(c.err, auth.ErrRefreshFailed) {
				t.Errorf("%s error = %v, want the 503 refresh failure", c.call, c.err)
			}
		}
		if n, _ := srv.counts(); n != 1 {
			t.Errorf("refresh requests = %d, want 1 (one shared refresh)", n)
		}
	})
}

// TestREQ026_RefreshPanicReleasesWaiters pins REQ-026 with REQ-025: a panic
// in a shared refresh goes up through the call that led it, unchanged, and
// does not leave the refresh in flight. The call waiting on it refreshes
// again instead of waiting out its deadline, and later calls are served.
func TestREQ026_RefreshPanicReleasesWaiters(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		release := make(chan struct{})
		srv := &scriptedServer{arrived: make(chan struct{}, 1)}
		srv.refresh = func(r *http.Request, n int) (*http.Response, error) {
			if n == 1 {
				<-release
				panic(errTransportBug)
			}
			return scriptedAnswer(r, http.StatusOK, freshTokenBody), nil
		}
		src := scriptedSource(t, srv)
		src.SetTokens(staleAccess("at-old"), "rt-0")

		waitCtx, cancelWait := context.WithTimeout(t.Context(), time.Minute)
		defer cancelWait()
		var (
			wg        sync.WaitGroup
			leadPanic any
			waitErr   error
			waitTok   auth.Token
		)
		wg.Go(func() {
			defer func() { leadPanic = recover() }()
			_, _ = src.Token(t.Context())
		})
		<-srv.arrived
		wg.Go(func() { waitTok, waitErr = src.Token(waitCtx) })
		synctest.Wait()
		close(release)
		wg.Wait()

		if leadPanic != errTransportBug { //nolint:errorlint // the panic value itself, as it was raised
			t.Errorf("leading Token() panicked with %v, want %v as it was raised", leadPanic, errTransportBug)
		}
		if waitErr != nil || waitTok.Value != "at-new" {
			t.Errorf("waiting Token() = (%q, %v), want (at-new, nil) from a new refresh", waitTok.Value, waitErr)
		}
		if n, _ := srv.counts(); n != 2 {
			t.Errorf("refresh requests = %d, want 2 (the one that panicked, the waiter's own)", n)
		}
		if tok, err := src.Token(waitCtx); err != nil || tok.Value != "at-new" {
			t.Errorf("Token() after the panic = (%q, %v), want (at-new, nil)", tok.Value, err)
		}
	})
}

// TestREQ026_RefreshPanicDoesNotWedgeLaterCalls pins REQ-026 with REQ-025: a
// call that comes after a panicked refresh, with no call waiting on it,
// refreshes again instead of waiting on the abandoned one.
func TestREQ026_RefreshPanicDoesNotWedgeLaterCalls(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		srv := &scriptedServer{}
		srv.refresh = func(r *http.Request, n int) (*http.Response, error) {
			if n == 1 {
				panic(errTransportBug)
			}
			return scriptedAnswer(r, http.StatusOK, freshTokenBody), nil
		}
		src := scriptedSource(t, srv)
		src.SetTokens(staleAccess("at-old"), "rt-0")
		func() {
			defer func() { _ = recover() }()
			_, _ = src.Token(t.Context())
		}()

		ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
		defer cancel()
		if tok, err := src.Token(ctx); err != nil || tok.Value != "at-new" {
			t.Errorf("Token() after a panicked refresh = (%q, %v), want (at-new, nil)", tok.Value, err)
		}
		if n, _ := srv.counts(); n != 2 {
			t.Errorf("refresh requests = %d, want 2", n)
		}
	})
}

// TestREQ026_JWKSLeaderCancelDoesNotFailWaiters pins REQ-026 with REQ-062:
// when the lookup leading a shared key-set fetch gives up and that alone
// fails the fetch, a lookup waiting on it with a live context fetches again
// and finds the key.
func TestREQ026_JWKSLeaderCancelDoesNotFailWaiters(t *testing.T) {
	body := string(keySetBody(t, publishedKey{key: newRSAKey(t), kid: "k1", use: "sig"}))
	synctest.Test(t, func(t *testing.T) {
		srv := &scriptedServer{arrived: make(chan struct{}, 1)}
		srv.keySet = func(r *http.Request, n int) (*http.Response, error) {
			if n == 1 {
				<-r.Context().Done()
				return nil, r.Context().Err()
			}
			return scriptedAnswer(r, http.StatusOK, body), nil
		}
		jwks := scriptedJWKS(t, srv)

		leadCtx, cancelLead := context.WithCancel(t.Context())
		var (
			wg               sync.WaitGroup
			leadErr, waitErr error
		)
		wg.Go(func() { _, leadErr = jwks.Key(leadCtx, "k1") })
		<-srv.arrived
		wg.Go(func() { _, waitErr = jwks.Key(t.Context(), "k1") })
		synctest.Wait()
		cancelLead()
		wg.Wait()

		if !errors.Is(leadErr, context.Canceled) {
			t.Errorf("leading Key() error = %v, want context.Canceled", leadErr)
		}
		if waitErr != nil {
			t.Errorf("waiting Key() error = %v, want nil: the leader's cancellation is not the waiter's", waitErr)
		}
		if _, n := srv.counts(); n != 2 {
			t.Errorf("key-set fetches = %d, want 2 (the cancelled one, the waiter's own)", n)
		}
	})
}

// TestREQ026_JWKSFetchFailureReachesWaitersAfterLeaderCancel pins REQ-026
// with REQ-062: a fetch that fails for a reason of its own reaches the
// lookups waiting on it, even when the leader's context ended meanwhile.
func TestREQ026_JWKSFetchFailureReachesWaitersAfterLeaderCancel(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		release := make(chan struct{})
		srv := &scriptedServer{arrived: make(chan struct{}, 1)}
		srv.keySet = func(r *http.Request, n int) (*http.Response, error) {
			<-release
			return scriptedAnswer(r, http.StatusServiceUnavailable, `{}`), nil
		}
		jwks := scriptedJWKS(t, srv)

		leadCtx, cancelLead := context.WithCancel(t.Context())
		var (
			wg               sync.WaitGroup
			leadErr, waitErr error
		)
		wg.Go(func() { _, leadErr = jwks.Key(leadCtx, "k1") })
		<-srv.arrived
		wg.Go(func() { _, waitErr = jwks.Key(t.Context(), "k1") })
		synctest.Wait()
		cancelLead()
		synctest.Wait()
		close(release)
		wg.Wait()

		for _, c := range []struct {
			call string
			err  error
		}{{"leading Key()", leadErr}, {"waiting Key()", waitErr}} {
			if c.err == nil || !strings.Contains(c.err.Error(), "status 503") {
				t.Errorf("%s error = %v, want the 503 fetch failure", c.call, c.err)
			}
		}
		if _, n := srv.counts(); n != 1 {
			t.Errorf("key-set fetches = %d, want 1 (one shared fetch)", n)
		}
	})
}

// TestREQ026_JWKSFetchPanicReleasesWaiters pins REQ-026 with REQ-025 and
// REQ-062: a panic in a shared key-set fetch goes up through the lookup that
// led it, unchanged, and does not leave the fetch in flight. The lookup
// waiting on it fetches again and finds the key, and so does a later one.
func TestREQ026_JWKSFetchPanicReleasesWaiters(t *testing.T) {
	body := string(keySetBody(t, publishedKey{key: newRSAKey(t), kid: "k1", use: "sig"}))
	synctest.Test(t, func(t *testing.T) {
		release := make(chan struct{})
		srv := &scriptedServer{arrived: make(chan struct{}, 1)}
		srv.keySet = func(r *http.Request, n int) (*http.Response, error) {
			if n == 1 {
				<-release
				panic(errTransportBug)
			}
			return scriptedAnswer(r, http.StatusOK, body), nil
		}
		jwks := scriptedJWKS(t, srv)

		waitCtx, cancelWait := context.WithTimeout(t.Context(), time.Minute)
		defer cancelWait()
		var (
			wg        sync.WaitGroup
			leadPanic any
			waitErr   error
		)
		wg.Go(func() {
			defer func() { leadPanic = recover() }()
			_, _ = jwks.Key(t.Context(), "k1")
		})
		<-srv.arrived
		wg.Go(func() { _, waitErr = jwks.Key(waitCtx, "k1") })
		synctest.Wait()
		close(release)
		wg.Wait()

		if leadPanic != errTransportBug { //nolint:errorlint // the panic value itself, as it was raised
			t.Errorf("leading Key() panicked with %v, want %v as it was raised", leadPanic, errTransportBug)
		}
		if waitErr != nil {
			t.Errorf("waiting Key() error = %v, want nil from a new fetch", waitErr)
		}
		ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
		defer cancel()
		if _, err := jwks.Key(ctx, "k1"); err != nil {
			t.Errorf("Key() after the panic error = %v, want nil", err)
		}
		if _, n := srv.counts(); n != 2 {
			t.Errorf("key-set fetches = %d, want 2 (the one that panicked, the waiter's own)", n)
		}
	})
}

// TestREQ025_JWKSLiteralWithoutHTTPClientRefuses pins REQ-025 (idiom.md § No
// panics): a JWKS built as a literal without an HTTPClient answers a lookup,
// with or without a kid, with an error matching auth.ErrInvalidConfig
// instead of panicking.
func TestREQ025_JWKSLiteralWithoutHTTPClientRefuses(t *testing.T) {
	for _, kid := range []string{"k1", ""} {
		jwks := &smart.JWKS{URI: "https://idp.test/jwks"}
		var err error
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("Key(%q) on a JWKS literal panicked: %v", kid, r)
				}
			}()
			_, err = jwks.Key(t.Context(), kid)
		}()
		if !errors.Is(err, auth.ErrInvalidConfig) {
			t.Errorf("Key(%q) on a JWKS literal without an HTTPClient error = %v, want auth.ErrInvalidConfig", kid, err)
		}
	}
}
