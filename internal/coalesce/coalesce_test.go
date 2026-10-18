package coalesce_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/cadasto/openehr-sdk-go/internal/coalesce"
)

// TestREQ026_EndedByContext pins the rule REQ-026's coalesced calls share
// (idiom.md § Concurrency): a failure counts as the starting caller's own
// only when that caller's context has ended and the error reports the
// context's error or the cause it ended with.
func TestREQ026_EndedByContext(t *testing.T) {
	errServer := errors.New("token endpoint returned 503")
	errCause := errors.New("the request was abandoned")

	cancelled := func() context.Context {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		return ctx
	}
	cancelledWithCause := func() context.Context {
		ctx, cancel := context.WithCancelCause(t.Context())
		cancel(errCause)
		return ctx
	}
	expired := func() context.Context {
		ctx, cancel := context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
		t.Cleanup(cancel)
		return ctx
	}
	live := func() context.Context { return t.Context() }
	// requestErr wraps err as http.Client.Do does.
	requestErr := func(err error) error {
		return &url.Error{Op: "Post", URL: "https://idp.example/token", Err: err}
	}

	tests := []struct {
		name string
		ctx  func() context.Context
		err  error
		want bool
	}{
		{name: "no error, live context", ctx: live, err: nil, want: false},
		{name: "no error, cancelled context", ctx: cancelled, err: nil, want: false},
		{name: "cancellation error, live context", ctx: live, err: requestErr(context.Canceled), want: false},
		{name: "cancellation error, cancelled context", ctx: cancelled, err: requestErr(context.Canceled), want: true},
		{name: "wrapped twice, cancelled context", ctx: cancelled, err: fmt.Errorf("refresh: %w", requestErr(context.Canceled)), want: true},
		{name: "deadline error, expired context", ctx: expired, err: requestErr(context.DeadlineExceeded), want: true},
		{name: "cause, context cancelled with that cause", ctx: cancelledWithCause, err: requestErr(errCause), want: true},
		{name: "cancellation error, context cancelled with a cause", ctx: cancelledWithCause, err: requestErr(context.Canceled), want: true},
		{name: "server error, cancelled context", ctx: cancelled, err: errServer, want: false},
		{name: "server error, context cancelled with a cause", ctx: cancelledWithCause, err: errServer, want: false},
		{name: "client timeout, live context", ctx: live, err: requestErr(context.DeadlineExceeded), want: false},
		{name: "client timeout, cancelled context", ctx: cancelled, err: requestErr(context.DeadlineExceeded), want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := tc.ctx()
			if got := coalesce.EndedByContext(ctx, tc.err); got != tc.want {
				t.Errorf("EndedByContext(ctx.Err() = %v, cause = %v; err = %v) = %v, want %v",
					ctx.Err(), context.Cause(ctx), tc.err, got, tc.want)
			}
		})
	}
}

// TestREQ026_EndedByContextReadsTheCauseNetHTTPReturns pins why the cause
// counts: net/http answers a request whose context ended with a cause with
// an error that reports the cause and not context.Canceled.
func TestREQ026_EndedByContextReadsTheCauseNetHTTPReturns(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer srv.Close()

	errCause := errors.New("the request was abandoned")
	ctx, cancel := context.WithCancelCause(t.Context())
	cancel(errCause)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := srv.Client().Do(req)
	if err == nil {
		_ = resp.Body.Close()
		t.Fatal("Do with an ended context succeeded, want an error")
	}
	if !errors.Is(err, errCause) {
		t.Fatalf("Do error = %v, want one that reports the cause %v", err, errCause)
	}
	if !coalesce.EndedByContext(ctx, err) {
		t.Errorf("EndedByContext(ctx cancelled with a cause, %v) = false, want true", err)
	}
}
