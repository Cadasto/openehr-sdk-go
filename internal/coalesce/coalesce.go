// Package coalesce holds the rule the SDK's coalesced calls share. Such a
// call runs once on behalf of every caller waiting on it, under the context
// of the caller that started it. When that context ending is all that failed
// the call, the failure belongs to that caller alone: a waiter whose own
// context is still live does not take it as its result, and tries again.
// [EndedByContext] decides when a failure is of that kind.
package coalesce

import (
	"context"
	"errors"
)

// EndedByContext reports whether err means the call failed only because ctx
// ended: ctx is done, and err reports ctx's own error or its cause. The cause
// counts because net/http returns context.Cause(ctx) when a request's context
// ends, so a context ended with a cause (context.WithCancelCause,
// context.WithTimeoutCause, errgroup) yields an error that reports the cause
// and not context.Canceled or context.DeadlineExceeded. An error that reports
// some other context error does not count and is a failure of the call: the
// HTTP client's own timeout reports context.DeadlineExceeded whether ctx is
// live or was only cancelled.
func EndedByContext(ctx context.Context, err error) bool {
	ctxErr := ctx.Err()
	return err != nil && ctxErr != nil &&
		(errors.Is(err, ctxErr) || errors.Is(err, context.Cause(ctx)))
}
