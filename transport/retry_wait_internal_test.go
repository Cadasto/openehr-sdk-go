package transport

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestREQ091_RetryWaitReadsAnEndedContextFirst pins that retryWait returns
// the context's error, never nil, once the context has ended, whatever the
// wait. With a tiny positive wait the timer has fired by the time a select
// runs, and a select picks between two ready cases at random, so the check
// has to come before the wait; one nil would start an attempt after the
// caller gave up and drop the Response Do already holds.
func TestREQ091_RetryWaitReadsAnEndedContextFirst(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, d := range []time.Duration{-time.Second, 0, time.Nanosecond, time.Microsecond} {
		for i := range 500 {
			if err := retryWait(ctx, d); !errors.Is(err, context.Canceled) {
				t.Fatalf("retryWait(ended ctx, %v) = %v on call %d, want context.Canceled", d, err, i+1)
			}
		}
	}
}
