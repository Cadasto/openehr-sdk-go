package smart_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"slices"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/cadasto/openehr-sdk-go/auth"
	"github.com/cadasto/openehr-sdk-go/auth/smart"
	"github.com/cadasto/openehr-sdk-go/smart/discovery"
)

// changeLog records the token changes a source reports.
type changeLog struct {
	mu      sync.Mutex
	changes []smart.TokenChange
	ctxs    []context.Context
}

func (l *changeLog) record(ctx context.Context, c smart.TokenChange) {
	l.mu.Lock()
	l.changes = append(l.changes, c)
	l.ctxs = append(l.ctxs, ctx)
	l.mu.Unlock()
}

func (l *changeLog) all() []smart.TokenChange {
	l.mu.Lock()
	defer l.mu.Unlock()
	return slices.Clone(l.changes)
}

func (l *changeLog) contexts() []context.Context {
	l.mu.Lock()
	defer l.mu.Unlock()
	return slices.Clone(l.ctxs)
}

// accessValues returns the access-token values of changes, in order.
func accessValues(changes []smart.TokenChange) []string {
	out := make([]string, 0, len(changes))
	for _, c := range changes {
		out = append(out, c.Access.Value)
	}
	return out
}

type ctxMarker struct{}

// withinDeadline runs fn and fails the test when it has not returned after
// ten seconds, which here means the source deadlocked.
func withinDeadline(t *testing.T, what string, fn func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		fn()
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatalf("%s did not return within 10s: the token-change hook deadlocked against the source", what)
	}
}

// TestTokenChangeAfterCodeExchange pins REQ-063: after a successful code
// exchange, through ExchangeAuthorizationCode or CompleteAuthorization, the
// source calls the token-change hook once, with the call's context, the new
// access token, the response's refresh token and the token response as
// LastTokenResponse reports it.
func TestTokenChangeAfterCodeExchange(t *testing.T) { // REQ-063
	for _, complete := range []bool{false, true} {
		name := "ExchangeAuthorizationCode"
		if complete {
			name = "CompleteAuthorization"
		}
		t.Run(name, func(t *testing.T) {
			as := newStubServer(t)
			var log changeLog
			src := as.source(t, as.endpoints(), smart.WithTokenChange(log.record))
			as.answerToken(0, launchBody(t, "at-1", map[string]any{"patient": "P1"}, map[string]any{"refresh_token": "rt-1"}))

			req, err := src.BeginAuthorization("")
			if err != nil {
				t.Fatalf("BeginAuthorization: %v", err)
			}
			ctx := context.WithValue(t.Context(), ctxMarker{}, name)
			var tok auth.Token
			if complete {
				tok, _, err = src.CompleteAuthorization(ctx, url.Values{"state": {req.State}, "code": {"code-1"}}, req)
			} else {
				tok, _, err = src.ExchangeAuthorizationCode(ctx, "code-1", req.State, req)
			}
			if err != nil {
				t.Fatalf("%s() error = %v, want success", name, err)
			}

			changes := log.all()
			if len(changes) != 1 {
				t.Fatalf("token-change calls = %d (%q), want 1", len(changes), accessValues(changes))
			}
			c := changes[0]
			if c.Access != tok || c.RefreshToken != "rt-1" {
				t.Errorf("TokenChange = access %+v, refresh %q; want the exchange's %+v, rt-1", c.Access, c.RefreshToken, tok)
			}
			if last := src.LastTokenResponse(); !reflect.DeepEqual(c.Response, last) || c.Response.Patient != "P1" {
				t.Errorf("TokenChange.Response = %+v, want LastTokenResponse() %+v with patient P1", c.Response, last)
			}
			if got := log.contexts()[0].Value(ctxMarker{}); got != name {
				t.Errorf("token-change hook context carries %v, want the %s call's context", got, name)
			}
		})
	}
}

// TestTokenChangeAfterRefresh pins REQ-063 and REQ-064: after a successful
// refresh the source calls the hook once with the new access token, the
// refresh token it now holds, which is the earlier one when the response
// carries none, and LastTokenResponse, launch context kept.
func TestTokenChangeAfterRefresh(t *testing.T) { // REQ-063 REQ-064
	tests := []struct {
		name        string
		extra       map[string]any
		wantRefresh string
	}{
		{name: "rotated refresh token", extra: map[string]any{"refresh_token": "rt-2"}, wantRefresh: "rt-2"},
		{name: "no refresh token in the response", wantRefresh: "rt-1"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			as := newStubServer(t)
			var log changeLog
			src := as.source(t, as.endpoints(), smart.WithTokenChange(log.record))
			as.answerToken(0, launchBody(t, "at-1", map[string]any{"patient": "P1"}, map[string]any{"refresh_token": "rt-1"}))
			req, err := src.BeginAuthorization("")
			if err != nil {
				t.Fatalf("BeginAuthorization: %v", err)
			}
			if _, _, err := src.ExchangeAuthorizationCode(t.Context(), "code-1", req.State, req); err != nil {
				t.Fatalf("ExchangeAuthorizationCode() error = %v", err)
			}
			src.SetTokens(staleAccess("at-1"), "rt-1")

			as.answerToken(0, launchBody(t, "at-2", nil, tc.extra))
			ctx := context.WithValue(t.Context(), ctxMarker{}, "refresh")
			tok, err := src.Token(ctx)
			if err != nil {
				t.Fatalf("Token() error = %v, want a refreshed token", err)
			}

			changes := log.all()
			if got := accessValues(changes); !slices.Equal(got, []string{"at-1", "at-2"}) {
				t.Fatalf("token changes = %q, want the exchange's then the refresh's [at-1 at-2]", got)
			}
			c := changes[1]
			if c.Access != tok || c.RefreshToken != tc.wantRefresh {
				t.Errorf("refresh TokenChange = access %+v, refresh %q; want %+v, %q", c.Access, c.RefreshToken, tok, tc.wantRefresh)
			}
			if _, held := src.HeldTokens(); held != tc.wantRefresh {
				t.Errorf("held refresh token = %q, want %q", held, tc.wantRefresh)
			}
			if last := src.LastTokenResponse(); !reflect.DeepEqual(c.Response, last) || c.Response.Patient != "P1" {
				t.Errorf("refresh TokenChange.Response = %+v, want LastTokenResponse() %+v with the kept patient P1", c.Response, last)
			}
			if got := log.contexts()[1].Value(ctxMarker{}); got != "refresh" {
				t.Errorf("token-change hook context carries %v, want the refreshing Token call's context", got)
			}
		})
	}
}

// TestTokenChangeNotCalledOnFailure pins REQ-063: a failed code exchange or
// refresh, including one whose ID token the source cannot verify, does not
// call the hook, and neither does SetTokens.
func TestTokenChangeNotCalledOnFailure(t *testing.T) { // REQ-063
	const unverifiable = "eyJhbGciOiJSUzI1NiJ9.e30.c2ln"
	exchange := func(t *testing.T, src *smart.Source) error {
		req, err := src.BeginAuthorization("")
		if err != nil {
			t.Fatalf("BeginAuthorization: %v", err)
		}
		_, _, err = src.ExchangeAuthorizationCode(t.Context(), "code-1", req.State, req)
		return err
	}
	refresh := func(t *testing.T, src *smart.Source) error {
		src.SetTokens(staleAccess("at-1"), "rt-1")
		_, err := src.Token(t.Context())
		return err
	}
	tests := []struct {
		name    string
		status  int
		body    string
		call    func(*testing.T, *smart.Source) error
		wantErr error
	}{
		{
			name: "code exchange refused", status: http.StatusBadRequest, body: `{"error":"invalid_grant"}`,
			call: exchange, wantErr: auth.ErrTokenExchangeFailed,
		},
		{
			name: "code exchange with an ID token the source cannot verify", body: tokenBody(t, "at-2", "rt-2", unverifiable),
			call: exchange, wantErr: auth.ErrTokenExchangeFailed,
		},
		{
			name: "refresh fails transiently", status: http.StatusServiceUnavailable, body: `{"error":"temporarily_unavailable"}`,
			call: refresh, wantErr: auth.ErrRefreshFailed,
		},
		{
			name: "refresh refused terminally", status: http.StatusBadRequest, body: `{"error":"invalid_grant"}`,
			call: refresh, wantErr: auth.ErrReauthRequired,
		},
		{
			name: "refresh with an ID token the source cannot verify", body: tokenBody(t, "at-2", "rt-2", unverifiable),
			call: refresh, wantErr: auth.ErrRefreshFailed,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			as := newStubServer(t)
			var log changeLog
			src := as.source(t, as.endpoints(), smart.WithTokenChange(log.record))
			as.answerToken(tc.status, tc.body)

			if err := tc.call(t, src); !errors.Is(err, tc.wantErr) {
				t.Fatalf("call error = %v, want %v", err, tc.wantErr)
			}
			if n := len(as.tokenRequests()); n != 1 {
				t.Fatalf("token requests = %d, want 1", n)
			}
			if changes := log.all(); len(changes) != 0 {
				t.Errorf("token changes = %q, want none for a failed call or SetTokens", accessValues(changes))
			}
		})
	}
}

// heldRefreshSource builds a source on h with an openid scope and the hook
// set, holding a stale access token and refresh token rt-A.
func heldRefreshSource(t *testing.T, h *heldRefreshTransport, hook func(context.Context, smart.TokenChange)) *smart.Source {
	t.Helper()
	src, err := newSource("client-id", discovery.AuthEndpoints{
		AuthorizationEndpoint: discovery.MustParseURL("https://idp.test/authorize"),
		TokenEndpoint:         discovery.MustParseURL("https://idp.test/token"),
		JWKSURI:               discovery.MustParseURL("https://idp.test/jwks"),
	},
		smart.WithHTTPClient(&http.Client{Transport: h}),
		smart.WithRedirectURI("https://app.example/callback"),
		smart.WithIssuer(oidcIssuer),
		smart.WithScopes("openid"),
		smart.WithTokenChange(hook),
	)
	if err != nil {
		t.Fatalf("newSource: %v", err)
	}
	src.SetTokens(staleAccess("A-1"), "rt-A")
	return src
}

// TestTokenChangeOnceForCoalescedRefresh pins REQ-063: Token callers that
// share one refresh cause one token change.
func TestTokenChangeOnceForCoalescedRefresh(t *testing.T) { // REQ-063
	key := newRSAKey(t)
	synctest.Test(t, func(t *testing.T) {
		const callers = 5
		h := newHeldRefreshTransport(t, key)
		var log changeLog
		src := heldRefreshSource(t, h, log.record)

		results := make(chan tokenResult, callers)
		call := func() {
			tok, err := src.Token(t.Context())
			results <- tokenResult{tok, err}
		}
		go call()
		<-h.arrived // the leader's refresh is held at the server
		for range callers - 1 {
			go call()
		}
		synctest.Wait() // every other caller waits on the held refresh
		h.release <- heldResponse{http.StatusOK, `{"access_token":"A-2","token_type":"Bearer","expires_in":3600,"refresh_token":"rt-A2"}`}

		for range callers {
			if r := <-results; r.err != nil || r.tok.Value != "A-2" {
				t.Errorf("Token() = %q, %v; want A-2", r.tok.Value, r.err)
			}
		}
		changes := log.all()
		if len(changes) != 1 || changes[0].Access.Value != "A-2" || changes[0].RefreshToken != "rt-A2" {
			t.Errorf("token changes = %+v, want one with A-2 and rt-A2", changes)
		}
		if n := len(h.refreshes()); n != 1 {
			t.Errorf("refresh grants = %d, want 1", n)
		}
	})
}

// TestTokenChangeDoesNotHoldUpOtherCallers pins REQ-063: while the hook runs
// for a refresh, a caller that waited on that refresh already has its
// token, and a code exchange made meanwhile returns without running the
// hook beside it; its change reaches the hook after the refresh's, from the
// goroutine already running the hook.
func TestTokenChangeDoesNotHoldUpOtherCallers(t *testing.T) { // REQ-063
	key := newRSAKey(t)
	synctest.Test(t, func(t *testing.T) {
		h := newHeldRefreshTransport(t, key)
		var log changeLog
		entered := make(chan string, 4)
		unblock := make(chan struct{})
		src := heldRefreshSource(t, h, func(ctx context.Context, c smart.TokenChange) {
			entered <- c.Access.Value
			if c.Access.Value == "A-2" {
				<-unblock
			}
			log.record(ctx, c)
		})

		leader := make(chan tokenResult, 1)
		go func() {
			tok, err := src.Token(t.Context())
			leader <- tokenResult{tok, err}
		}()
		<-h.arrived
		waiter := make(chan tokenResult, 1)
		go func() {
			tok, err := src.Token(t.Context())
			waiter <- tokenResult{tok, err}
		}()
		synctest.Wait() // the waiter is parked on the held refresh
		h.release <- heldResponse{http.StatusOK, `{"access_token":"A-2","token_type":"Bearer","expires_in":3600,"refresh_token":"rt-A2"}`}

		if got := <-entered; got != "A-2" {
			t.Fatalf("hook entered for %q, want the refresh's A-2", got)
		}
		synctest.Wait() // the hook is now blocked
		select {
		case r := <-waiter:
			if r.err != nil || r.tok.Value != "A-2" {
				t.Errorf("waiting caller: Token() = %q, %v; want A-2", r.tok.Value, r.err)
			}
		default:
			t.Fatal("the caller waiting on the refresh is held up by the token-change hook")
		}

		exchangeUserB(t, h, src) // returns although the hook is still running
		select {
		case got := <-entered:
			t.Fatalf("hook entered for %q while it was still running for A-2", got)
		default:
		}

		close(unblock)
		if r := <-leader; r.err != nil || r.tok.Value != "A-2" {
			t.Errorf("leading caller: Token() = %q, %v; want A-2", r.tok.Value, r.err)
		}
		if got := accessValues(log.all()); !slices.Equal(got, []string{"A-2", "B-1"}) {
			t.Errorf("token changes = %q, want the refresh's then the code exchange's [A-2 B-1]", got)
		}
	})
}

// TestTokenChangeNotCalledForDiscardedRefresh pins REQ-063: a refresh whose
// result the source discarded because a code exchange or SetTokens replaced
// the session meanwhile causes no token change; the code exchange causes
// its own.
func TestTokenChangeNotCalledForDiscardedRefresh(t *testing.T) { // REQ-063
	key := newRSAKey(t)
	tests := []struct {
		name        string
		replace     func(t *testing.T, h *heldRefreshTransport, src *smart.Source)
		wantChanges []string
	}{
		{name: "code exchange", replace: exchangeUserB, wantChanges: []string{"B-1"}},
		{
			name: "SetTokens",
			replace: func(_ *testing.T, _ *heldRefreshTransport, src *smart.Source) {
				src.SetTokens(freshAccess("C-1"), "rt-C")
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				h := newHeldRefreshTransport(t, key)
				var log changeLog
				src := heldRefreshSource(t, h, log.record)

				leader := make(chan tokenResult, 1)
				go func() {
					tok, err := src.Token(t.Context())
					leader <- tokenResult{tok, err}
				}()
				<-h.arrived
				tc.replace(t, h, src)
				h.release <- heldResponse{http.StatusOK, `{"access_token":"A-2","token_type":"Bearer","expires_in":3600,"refresh_token":"rt-A2"}`}
				if r := <-leader; r.err != nil || r.tok.Value == "A-2" {
					t.Errorf("Token() = %q, %v; want the new session's token", r.tok.Value, r.err)
				}
				if got := accessValues(log.all()); !slices.Equal(got, tc.wantChanges) {
					t.Errorf("token changes = %q, want %q: none for the discarded refresh", got, tc.wantChanges)
				}
			})
		})
	}
}

// TestTokenChangeHookMayCallTheSource pins REQ-063: the source calls the
// hook outside its lock and after it holds the new tokens, so a hook that
// reads LastTokenResponse and calls Token sees the change's own values and
// does not deadlock.
func TestTokenChangeHookMayCallTheSource(t *testing.T) { // REQ-063
	as := newStubServer(t)
	var (
		src  *smart.Source
		mu   sync.Mutex
		seen []string
	)
	src = as.source(t, as.endpoints(), smart.WithTokenChange(func(ctx context.Context, c smart.TokenChange) {
		last := src.LastTokenResponse()
		tok, err := src.Token(ctx)
		if err != nil || tok.Value != c.Access.Value || last.AccessToken != c.Access.Value {
			t.Errorf("inside the hook for %q: Token() = %q, %v; LastTokenResponse().AccessToken = %q; want the change's own token",
				c.Access.Value, tok.Value, err, last.AccessToken)
		}
		mu.Lock()
		seen = append(seen, c.Access.Value)
		mu.Unlock()
	}))

	withinDeadline(t, "a code exchange and a refresh", func() {
		as.answerToken(0, tokenBody(t, "at-1", "rt-1", ""))
		req, err := src.BeginAuthorization("")
		if err != nil {
			t.Errorf("BeginAuthorization: %v", err)
			return
		}
		if _, _, err := src.ExchangeAuthorizationCode(t.Context(), "code-1", req.State, req); err != nil {
			t.Errorf("ExchangeAuthorizationCode() error = %v", err)
			return
		}
		src.SetTokens(staleAccess("at-1"), "rt-1")
		as.answerToken(0, tokenBody(t, "at-2", "rt-2", ""))
		if _, err := src.Token(t.Context()); err != nil {
			t.Errorf("Token() error = %v", err)
		}
	})
	mu.Lock()
	defer mu.Unlock()
	if !slices.Equal(seen, []string{"at-1", "at-2"}) {
		t.Errorf("hook saw %q, want [at-1 at-2]", seen)
	}
}

// TestTokenChangesArriveInInstallOrder pins REQ-063: changes reach the hook
// one at a time, in the order the source installed them. A refresh the hook
// itself causes is reported after the hook returns, not inside it.
func TestTokenChangesArriveInInstallOrder(t *testing.T) { // REQ-063
	as := newStubServer(t)
	var (
		src    *smart.Source
		events []string
	)
	src = as.source(t, as.endpoints(), smart.WithTokenChange(func(ctx context.Context, c smart.TokenChange) {
		events = append(events, "start "+c.Access.Value)
		if c.Access.Value == "at-1" {
			as.answerToken(0, tokenBody(t, "at-2", "rt-2", ""))
			if err := src.Reauth(ctx); err != nil {
				t.Errorf("Reauth() inside the hook error = %v, want a refresh", err)
			}
		}
		events = append(events, "end "+c.Access.Value)
	}))
	as.answerToken(0, tokenBody(t, "at-1", "rt-1", ""))

	withinDeadline(t, "a code exchange whose hook forces a refresh", func() {
		req, err := src.BeginAuthorization("")
		if err != nil {
			t.Errorf("BeginAuthorization: %v", err)
			return
		}
		if _, _, err := src.ExchangeAuthorizationCode(t.Context(), "code-1", req.State, req); err != nil {
			t.Errorf("ExchangeAuthorizationCode() error = %v", err)
		}
	})
	want := []string{"start at-1", "end at-1", "start at-2", "end at-2"}
	if !slices.Equal(events, want) {
		t.Errorf("hook events = %q, want %q", events, want)
	}
}

// TestTokenChangeAfterHookPanic pins REQ-063: a hook that panics does not
// stop later changes from reaching it.
func TestTokenChangeAfterHookPanic(t *testing.T) { // REQ-063
	as := newStubServer(t)
	var log changeLog
	src := as.source(t, as.endpoints(), smart.WithTokenChange(func(ctx context.Context, c smart.TokenChange) {
		log.record(ctx, c)
		if c.Access.Value == "at-1" {
			panic("hook failed")
		}
	}))
	as.answerToken(0, tokenBody(t, "at-1", "rt-1", ""))
	func() {
		defer func() {
			if r := recover(); r == nil {
				t.Fatal("ExchangeAuthorizationCode() returned, want the hook's panic")
			}
		}()
		req, err := src.BeginAuthorization("")
		if err != nil {
			t.Fatalf("BeginAuthorization: %v", err)
		}
		_, _, _ = src.ExchangeAuthorizationCode(t.Context(), "code-1", req.State, req)
	}()

	src.SetTokens(staleAccess("at-1"), "rt-1")
	as.answerToken(0, tokenBody(t, "at-2", "rt-2", ""))
	if _, err := src.Token(t.Context()); err != nil {
		t.Fatalf("Token() error = %v, want a refreshed token", err)
	}
	if got := accessValues(log.all()); !slices.Equal(got, []string{"at-1", "at-2"}) {
		t.Errorf("token changes = %q, want [at-1 at-2]: the refresh after the panic still reaches the hook", got)
	}
}

// TestTokenChangeNilHook pins REQ-063: WithTokenChange(nil) sets no hook.
func TestTokenChangeNilHook(t *testing.T) { // REQ-063
	as := newStubServer(t)
	src := as.source(t, as.endpoints(), smart.WithTokenChange(nil))
	as.answerToken(0, tokenBody(t, "at-1", "rt-1", ""))
	req, err := src.BeginAuthorization("")
	if err != nil {
		t.Fatalf("BeginAuthorization: %v", err)
	}
	if _, _, err := src.ExchangeAuthorizationCode(t.Context(), "code-1", req.State, req); err != nil {
		t.Fatalf("ExchangeAuthorizationCode() error = %v", err)
	}
	src.SetTokens(staleAccess("at-1"), "rt-1")
	as.answerToken(0, tokenBody(t, "at-2", "rt-2", ""))
	if tok, err := src.Token(t.Context()); err != nil || tok.Value != "at-2" {
		t.Fatalf("Token() = %q, %v; want at-2", tok.Value, err)
	}
}

// TestTokenChangeQueuedBehindAPanicIsReported pins REQ-063: when the hook
// panics while a change from another call waits behind it, that change is
// still reported, without any later install of tokens.
func TestTokenChangeQueuedBehindAPanicIsReported(t *testing.T) { // REQ-063
	as := newStubServer(t)
	entered := make(chan string, 4)
	release := make(chan struct{})
	src := as.source(t, as.endpoints(), smart.WithTokenChange(func(_ context.Context, c smart.TokenChange) {
		entered <- c.Access.Value
		if c.Access.Value == "at-1" {
			<-release
			panic("hook failed")
		}
	}))
	exchange := func(code string) error {
		req, err := src.BeginAuthorization("")
		if err != nil {
			return err
		}
		_, _, err = src.ExchangeAuthorizationCode(t.Context(), code, req.State, req)
		return err
	}

	as.answerToken(0, tokenBody(t, "at-1", "rt-1", ""))
	panicked := make(chan any, 1)
	go func() {
		defer func() { panicked <- recover() }()
		_ = exchange("code-1")
	}()
	select {
	case got := <-entered:
		if got != "at-1" {
			t.Fatalf("hook entered for %q, want at-1", got)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the hook was not called for the first code exchange")
	}

	// A second exchange while the hook runs: its change waits behind.
	as.answerToken(0, tokenBody(t, "at-2", "rt-2", ""))
	withinDeadline(t, "a code exchange while the hook runs", func() {
		if err := exchange("code-2"); err != nil {
			t.Errorf("second ExchangeAuthorizationCode() error = %v", err)
		}
	})
	select {
	case got := <-entered:
		t.Fatalf("hook entered for %q while it was still running for at-1", got)
	default:
	}

	close(release)
	if r := <-panicked; r == nil {
		t.Fatal("the first ExchangeAuthorizationCode returned, want the hook's panic")
	}
	select {
	case got := <-entered:
		if got != "at-2" {
			t.Errorf("hook entered for %q, want the waiting at-2", got)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the change queued behind the panicking hook was not reported")
	}
}

// TestTokenChangeResponseIsTheHooksOwn pins REQ-063: the hook gets its own
// copy of the token response's Raw map, so changing it does not change the
// source's last token response.
func TestTokenChangeResponseIsTheHooksOwn(t *testing.T) { // REQ-063
	as := newStubServer(t)
	src := as.source(t, as.endpoints(), smart.WithTokenChange(func(_ context.Context, c smart.TokenChange) {
		delete(c.Response.Raw, "patient")
		c.Response.Raw["added_by_hook"] = true
	}))
	as.answerToken(0, launchBody(t, "at-1", map[string]any{"patient": "P1"}, map[string]any{"refresh_token": "rt-1"}))
	req, err := src.BeginAuthorization("")
	if err != nil {
		t.Fatalf("BeginAuthorization: %v", err)
	}
	if _, _, err := src.ExchangeAuthorizationCode(t.Context(), "code-1", req.State, req); err != nil {
		t.Fatalf("ExchangeAuthorizationCode() error = %v", err)
	}
	checkRaw := func(when string) {
		t.Helper()
		raw := src.LastTokenResponse().Raw
		if raw["patient"] != "P1" {
			t.Errorf("after the hook changed its copy (%s), LastTokenResponse().Raw[patient] = %v, want P1", when, raw["patient"])
		}
		if _, ok := raw["added_by_hook"]; ok {
			t.Errorf("after the hook changed its copy (%s), LastTokenResponse().Raw has the hook's added_by_hook", when)
		}
	}
	checkRaw("code exchange")

	src.SetTokens(staleAccess("at-1"), "rt-1")
	as.answerToken(0, launchBody(t, "at-2", nil, nil))
	if _, err := src.Token(t.Context()); err != nil {
		t.Fatalf("Token() error = %v", err)
	}
	checkRaw("refresh")
}

// TestTokenChangeHandedOnRecoversItsPanics pins REQ-063: the changes handed
// on to a new goroutine after the hook panicked on a caller's goroutine are
// all reported. There no caller could recover a panic, so a hook that
// panics on that goroutine does not end the program: the source drops that
// change and goes on with the rest.
func TestTokenChangeHandedOnRecoversItsPanics(t *testing.T) { // REQ-063
	as := newStubServer(t)
	attempts := make(chan string, 8)
	release := make(chan struct{})
	src := as.source(t, as.endpoints(), smart.WithTokenChange(func(_ context.Context, c smart.TokenChange) {
		attempts <- c.Access.Value
		if c.Access.Value == "at-1" {
			<-release
		}
		panic("hook failed on " + c.Access.Value)
	}))
	exchange := func(code, access string) error {
		as.answerToken(0, tokenBody(t, access, "", ""))
		req, err := src.BeginAuthorization("")
		if err != nil {
			return err
		}
		_, _, err = src.ExchangeAuthorizationCode(t.Context(), code, req.State, req)
		return err
	}
	next := func(want string) {
		t.Helper()
		select {
		case got := <-attempts:
			if got != want {
				t.Fatalf("hook called for %q, want %q", got, want)
			}
		case <-time.After(10 * time.Second):
			t.Fatalf("hook not called for %q", want)
		}
	}

	panicked := make(chan any, 1)
	go func() {
		defer func() { panicked <- recover() }()
		_ = exchange("code-1", "at-1")
	}()
	next("at-1")
	// Two more changes wait behind the blocked hook.
	withinDeadline(t, "two code exchanges while the hook runs", func() {
		for i, access := range []string{"at-2", "at-3"} {
			if err := exchange(fmt.Sprintf("code-%d", i+2), access); err != nil {
				t.Errorf("ExchangeAuthorizationCode(%s) error = %v", access, err)
			}
		}
	})
	close(release)
	if r := <-panicked; r == nil {
		t.Fatal("the first ExchangeAuthorizationCode returned, want the hook's panic")
	}
	next("at-2") // handed on; the hook panics here too
	next("at-3") // still reported after that panic
}

// TestTokenChangeAfterCodeExchangeWithoutRefreshToken pins REQ-063 and
// REQ-064: a code exchange whose response carries no refresh token reports
// an empty refresh token, even when the source held one before, since a new
// session never keeps an earlier session's refresh token.
func TestTokenChangeAfterCodeExchangeWithoutRefreshToken(t *testing.T) { // REQ-063 REQ-064
	as := newStubServer(t)
	var log changeLog
	src := as.source(t, as.endpoints(), smart.WithTokenChange(log.record))
	src.SetTokens(freshAccess("at-0"), "rt-earlier")
	as.answerToken(0, tokenBody(t, "at-1", "", ""))
	req, err := src.BeginAuthorization("")
	if err != nil {
		t.Fatalf("BeginAuthorization: %v", err)
	}
	if _, _, err := src.ExchangeAuthorizationCode(t.Context(), "code-1", req.State, req); err != nil {
		t.Fatalf("ExchangeAuthorizationCode() error = %v", err)
	}
	changes := log.all()
	if len(changes) != 1 || changes[0].Access.Value != "at-1" || changes[0].RefreshToken != "" {
		t.Errorf("token changes = %+v, want one with at-1 and an empty refresh token", changes)
	}
}

// TestTokenChangesKeepInstallOrderAcrossRevoke pins REQ-063 and REQ-167:
// changes reach the hook in the order the source installed them, Revoke's
// empty change included. While the hook is busy on another goroutine with
// an earlier change, Revoke clears the tokens and sends its request, and a
// code exchange installs new tokens before that request ends; the hook then
// sees the earlier change, Revoke's empty one and the new session's, in
// that order.
func TestTokenChangesKeepInstallOrderAcrossRevoke(t *testing.T) { // REQ-063 REQ-167
	as := newStubServer(t)
	as.answerRevoke(0, "")
	var (
		mu   sync.Mutex
		seen []string
	)
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	revokeSent := make(chan struct{}, 1)
	revokeEnd := make(chan struct{})
	client := &http.Client{Transport: observingTransport{
		next:    as.srv.Client().Transport,
		observe: func() { revokeSent <- struct{}{}; <-revokeEnd }, // holds the revocation request
	}}
	src := as.source(t, as.endpoints(), smart.WithHTTPClient(client), smart.WithTokenChange(func(_ context.Context, c smart.TokenChange) {
		mu.Lock()
		seen = append(seen, c.Access.Value) // "" for Revoke's empty change
		mu.Unlock()
		if c.Access.Value == "at-1" {
			entered <- struct{}{}
			<-release
		}
	}))
	exchange := func(code, access string) error {
		as.answerToken(0, tokenBody(t, access, "rt-"+access, ""))
		req, err := src.BeginAuthorization("")
		if err != nil {
			return err
		}
		_, _, err = src.ExchangeAuthorizationCode(t.Context(), code, req.State, req)
		return err
	}
	wait := func(what string, ch <-chan struct{}) {
		t.Helper()
		select {
		case <-ch:
		case <-time.After(10 * time.Second):
			t.Fatalf("timed out waiting for %s", what)
		}
	}

	// The hook is busy with the first session's change on its own goroutine.
	first := make(chan struct{})
	go func() {
		defer close(first)
		if err := exchange("code-1", "at-1"); err != nil {
			t.Errorf("first ExchangeAuthorizationCode() error = %v", err)
		}
	}()
	wait("the hook to start on at-1", entered)

	// Revoke clears the tokens and its request is held on its way.
	revoked := make(chan error, 1)
	go func() { revoked <- src.Revoke(t.Context()) }()
	wait("the revocation request", revokeSent)

	// A new session is installed before the revocation request ends.
	withinDeadline(t, "a code exchange while Revoke's request is on its way", func() {
		if err := exchange("code-2", "at-2"); err != nil {
			t.Errorf("second ExchangeAuthorizationCode() error = %v", err)
		}
	})
	close(revokeEnd)
	select {
	case err := <-revoked:
		if err != nil {
			t.Errorf("Revoke() error = %v, want nil", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Revoke did not return although another goroutine is reporting changes")
	}

	close(release)
	wait("the first code exchange to return", first)
	mu.Lock()
	got := slices.Clone(seen)
	mu.Unlock()
	if want := []string{"at-1", "", "at-2"}; !slices.Equal(got, want) {
		t.Errorf("hook saw access tokens %q, want %q: the earlier change, Revoke's empty one, then the new session's", got, want)
	}
	if access, _ := src.HeldTokens(); access.Value != "at-2" {
		t.Errorf("held access token = %q, want the new session's at-2", access.Value)
	}
}
