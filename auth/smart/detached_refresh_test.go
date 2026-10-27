package smart_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"slices"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/cadasto/openehr-sdk-go/auth"
	"github.com/cadasto/openehr-sdk-go/auth/smart"
)

// rotatingServer is an in-memory token endpoint that rotates the refresh
// token, as RFC 9700 §4.14.2 describes. It accepts only the refresh token it
// issued last and answers any other with invalid_grant. The n-th refresh it
// accepts spends the presented token at once, then answers after delay with
// "at-<n>" and "rt-<n>". When the request's context ends first, the client
// hung up: the answer is lost, and the rotation stands.
type rotatingServer struct {
	delay   time.Duration
	arrived chan struct{}

	mu        sync.Mutex
	current   string
	accepted  int
	presented []string
	traces    []any
}

type traceKey struct{}

func (s *rotatingServer) RoundTrip(r *http.Request) (*http.Response, error) {
	body, err := io.ReadAll(r.Body)
	_ = r.Body.Close()
	if err != nil {
		return nil, err
	}
	form, err := url.ParseQuery(string(body))
	if err != nil {
		return nil, err
	}
	presented := form.Get("refresh_token")
	s.mu.Lock()
	s.presented = append(s.presented, presented)
	s.traces = append(s.traces, r.Context().Value(traceKey{}))
	if presented != s.current {
		s.mu.Unlock()
		return scriptedAnswer(r, http.StatusBadRequest, `{"error":"invalid_grant"}`), nil
	}
	s.accepted++
	n := s.accepted
	s.current = fmt.Sprintf("rt-%d", n)
	s.mu.Unlock()
	if s.arrived != nil {
		select {
		case s.arrived <- struct{}{}:
		default:
		}
	}
	select {
	case <-time.After(s.delay):
	case <-r.Context().Done():
		return nil, context.Cause(r.Context())
	}
	return scriptedAnswer(r, http.StatusOK,
		fmt.Sprintf(`{"access_token":"at-%d","token_type":"Bearer","expires_in":3600,"refresh_token":"rt-%d"}`, n, n)), nil
}

// seen returns the refresh tokens presented and the trace value each
// request's context carried, in order.
func (s *rotatingServer) seen() ([]string, []any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.presented), slices.Clone(s.traces)
}

// TestREQ063_RefreshOutlivesTheCallThatStartedIt pins REQ-063 (auth.md §
// REQ-063): the refresh request is not ended by the context of the call that
// started it. That call returns its context's error at once; the refresh
// goes on, keeps the call's context values, installs the refresh token the
// server rotated and reports it to the token-change hook, so the next
// refresh presents the new token and is not refused as invalid_grant.
func TestREQ063_RefreshOutlivesTheCallThatStartedIt(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		srv := &rotatingServer{current: "rt-0", delay: 5 * time.Second, arrived: make(chan struct{}, 1)}
		var log changeLog
		src := scriptedSource(t, srv, smart.WithTokenChange(log.record))
		src.SetTokens(staleAccess("at-0"), "rt-0")

		leadCtx, cancelLead := context.WithCancel(context.WithValue(t.Context(), traceKey{}, "trace-1"))
		leadErr := make(chan error, 1)
		go func() {
			_, err := src.Token(leadCtx)
			leadErr <- err
		}()
		<-srv.arrived // the server has spent rt-0 and is answering
		cancelLead()
		synctest.Wait()
		select {
		case err := <-leadErr:
			if !errors.Is(err, context.Canceled) {
				t.Errorf("Token() whose context was cancelled error = %v, want context.Canceled", err)
			}
		default:
			t.Fatal("Token() whose context was cancelled is still waiting on the refresh, want it to return at once")
		}

		time.Sleep(10 * time.Second) // the server's answer has arrived meanwhile

		if access, refresh := src.HeldTokens(); access.Value != "at-1" || refresh != "rt-1" {
			t.Errorf("held tokens after the refresh = (%q, %q), want (at-1, rt-1): the rotated refresh token is lost", access.Value, refresh)
		}
		if changes := log.all(); len(changes) != 1 || changes[0].RefreshToken != "rt-1" {
			t.Errorf("token changes reported = %v, want one with the rotated refresh token rt-1", changes)
		}
		if err := src.Reauth(t.Context()); err != nil {
			t.Errorf("Reauth() after the refresh error = %v, want nil: the next refresh presents the rotated token", err)
		}
		presented, traces := srv.seen()
		if want := []string{"rt-0", "rt-1"}; !slices.Equal(presented, want) {
			t.Errorf("refresh tokens presented = %q, want %q", presented, want)
		}
		if len(traces) == 0 || traces[0] != "trace-1" {
			t.Errorf("context values on the refresh requests = %v, want the first to carry trace-1 from the call that started it", traces)
		}
	})
}

// TestREQ063_RefreshRunsAtMostThirtySeconds pins REQ-063: a refresh whose
// starting call gave up still ends, after 30 seconds, when the server never
// answers. A forced call waiting on it then gets that failure, and no
// goroutine of the source is left behind.
func TestREQ063_RefreshRunsAtMostThirtySeconds(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		srv := &scriptedServer{arrived: make(chan struct{}, 1)}
		srv.refresh = func(r *http.Request, _ int) (*http.Response, error) {
			<-r.Context().Done() // the server never answers
			return nil, context.Cause(r.Context())
		}
		src := scriptedSource(t, srv)
		src.SetTokens(staleAccess("at-0"), "rt-0")

		start := time.Now()
		leadCtx, cancelLead := context.WithCancel(t.Context())
		go func() { _, _ = src.Token(leadCtx) }()
		<-srv.arrived
		waitCtx, cancelWait := context.WithTimeout(t.Context(), time.Minute)
		defer cancelWait()
		waitErr := make(chan error, 1)
		go func() { waitErr <- src.RefreshIfNeeded(waitCtx) }()
		synctest.Wait() // RefreshIfNeeded waits on the refresh
		cancelLead()

		err := <-waitErr
		if elapsed := time.Since(start); elapsed != 30*time.Second {
			t.Errorf("RefreshIfNeeded() returned after %v, want 30s, when the refresh request is cut off", elapsed)
		}
		if !errors.Is(err, auth.ErrRefreshFailed) || !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("RefreshIfNeeded() error = %v, want auth.ErrRefreshFailed with context.DeadlineExceeded", err)
		}
		if n, _ := srv.counts(); n != 1 {
			t.Errorf("refresh requests = %d, want 1 (one shared refresh)", n)
		}
	})
}

// crashChildEnv names the variable that makes
// TestREQ026_RefreshPanicAfterTheStarterLeftIsNotSwallowed run its scenario
// in a child process.
const crashChildEnv = "SMART_REFRESH_PANIC_CHILD"

// swallowedLine is what the child prints when the panic did not end it.
const swallowedLine = "the refresh panic was swallowed"

// TestREQ026_RefreshPanicAfterTheStarterLeftIsNotSwallowed pins REQ-026
// (idiom.md § Concurrency): when the refresh panics after the call that
// started it has returned, the panic is not dropped. It surfaces on the
// refresh's goroutine and ends the program, as any unrecovered panic in a
// goroutine does. The scenario runs in a child process, which the panic ends.
func TestREQ026_RefreshPanicAfterTheStarterLeftIsNotSwallowed(t *testing.T) {
	if os.Getenv(crashChildEnv) == "1" {
		refreshPanicAfterTheStarterLeft(t)
		return
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestREQ026_RefreshPanicAfterTheStarterLeftIsNotSwallowed$", "-test.count=1")
	cmd.Env = append(os.Environ(), crashChildEnv+"=1")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	if _, ok := errors.AsType[*exec.ExitError](err); !ok {
		t.Fatalf("child process error = %v, want it to exit with a failure; output:\n%s", err, out.String())
	}
	if !bytes.Contains(out.Bytes(), []byte("panic: "+errTransportBug.Error())) {
		t.Errorf("child process output does not show the panic %q:\n%s", errTransportBug, out.String())
	}
	if bytes.Contains(out.Bytes(), []byte(swallowedLine)) {
		t.Errorf("child process went on after the refresh panicked:\n%s", out.String())
	}
}

// refreshPanicAfterTheStarterLeft is the child's scenario: the call that
// starts the refresh gives up, then the refresh panics.
func refreshPanicAfterTheStarterLeft(t *testing.T) {
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
	src.SetTokens(staleAccess("at-0"), "rt-0")

	leadCtx, cancelLead := context.WithCancel(t.Context())
	leadErr := make(chan error, 1)
	go func() {
		_, err := src.Token(leadCtx)
		leadErr <- err
	}()
	<-srv.arrived
	cancelLead()
	select {
	case err := <-leadErr:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Token() whose context was cancelled error = %v, want context.Canceled", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Token() whose context was cancelled is still waiting on the refresh, want it to return at once")
	}
	close(release)
	time.Sleep(5 * time.Second)
	fmt.Println(swallowedLine)
}

// TestREQ063_HookPanicAfterTheStarterLeftIsDropped pins REQ-063 (the
// WithTokenChange contract): when the call that started a refresh has
// returned, the refresh's own goroutine reports the change, and a hook that
// panics there does not end the program. The change stays installed.
func TestREQ063_HookPanicAfterTheStarterLeftIsDropped(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		srv := &rotatingServer{current: "rt-0", delay: 5 * time.Second, arrived: make(chan struct{}, 1)}
		var log changeLog
		hook := func(ctx context.Context, c smart.TokenChange) {
			log.record(ctx, c)
			panic(errTransportBug)
		}
		src := scriptedSource(t, srv, smart.WithTokenChange(hook))
		src.SetTokens(staleAccess("at-0"), "rt-0")

		leadCtx, cancelLead := context.WithCancel(t.Context())
		go func() { _, _ = src.Token(leadCtx) }()
		<-srv.arrived
		cancelLead()
		time.Sleep(10 * time.Second) // the refresh ends and reports its change

		if changes := log.all(); len(changes) != 1 || changes[0].RefreshToken != "rt-1" {
			t.Errorf("token changes reported = %v, want one with rt-1", changes)
		}
		if access, refresh := src.HeldTokens(); access.Value != "at-1" || refresh != "rt-1" {
			t.Errorf("held tokens = (%q, %q), want (at-1, rt-1)", access.Value, refresh)
		}
	})
}

// TestREQ063_RefreshGoroutineExitIsATransientFailure pins REQ-063 with
// REQ-026 (idiom.md § Concurrency): a refresh whose goroutine exits through
// runtime.Goexit, as one whose RoundTripper calls t.FailNow does, ends as a
// refresh failure that is not terminal. Token sends that one request and
// returns: past ExpiresAt with an error matching auth.ErrRefreshFailed, and
// before it with the held token, which it then serves without a request
// while the backoff holds. Releasing the callers to try again would send a
// new refresh that exits the same way, without end.
func TestREQ063_RefreshGoroutineExitIsATransientFailure(t *testing.T) {
	tests := []struct {
		name string
		held func(value string) auth.Token
		// wantHeld reports that Token serves the held token in place of
		// the failure.
		wantHeld bool
	}{
		{name: "past ExpiresAt", held: expiredAccess},
		{name: "before ExpiresAt", held: staleAccess, wantHeld: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				srv := &scriptedServer{}
				srv.refresh = func(_ *http.Request, n int) (*http.Response, error) {
					if n > 1 {
						// Token sent the refresh again: end the call
						// rather than let it go round without end.
						cancel()
					}
					runtime.Goexit()
					panic("unreachable")
				}
				src := scriptedSource(t, srv)
				src.SetTokens(tc.held("at-old"), "rt-0")

				tok, err := src.Token(ctx)
				if tc.wantHeld {
					if err != nil || tok.Value != "at-old" {
						t.Errorf("Token() = (%q, %v), want (at-old, nil): the held token in place of the failure", tok.Value, err)
					}
					if tok, err := src.Token(ctx); err != nil || tok.Value != "at-old" {
						t.Errorf("second Token() = (%q, %v), want (at-old, nil) while the backoff holds", tok.Value, err)
					}
				} else if !errors.Is(err, auth.ErrRefreshFailed) {
					t.Errorf("Token() error = %v, want one matching auth.ErrRefreshFailed", err)
				}
				if n, _ := srv.counts(); n != 1 {
					t.Errorf("refresh requests = %d, want 1: a refresh that exits its goroutine is not sent again", n)
				}
			})
		})
	}
}
