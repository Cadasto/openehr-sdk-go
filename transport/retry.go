package transport

import (
	"context"
	"math"
	"slices"
	"time"
)

// RetryPolicy configures retry-on-status behaviour. Retries are
// disabled by default; enable them with transport.WithRetry. Retries
// respect ctx cancellation immediately: once ctx has ended no further
// attempt starts, even with no backoff, and Client.Do returns the last
// Response it received. See RetryPolicy.MaxAttempts and Disabled for the
// precise "no-retry" semantics.
type RetryPolicy struct {
	// Disabled, when true, forces exactly one attempt regardless of
	// MaxAttempts. Use NoRetry as the canonical zero-config form for
	// "no retries"; Disabled is useful when composing on top of a
	// caller-supplied base policy that already sets MaxAttempts.
	Disabled bool
	// MaxAttempts is the total attempt count: 0 means "use package
	// default" (currently disabled, so one attempt); 1 means exactly one
	// attempt (no retries); N ≥ 2 means up to N total attempts.
	MaxAttempts int
	// InitialBackoff is the wait before the first retry. Subsequent
	// waits grow by Multiplier, capped at MaxBackoff.
	InitialBackoff time.Duration
	// MaxBackoff caps individual wait durations.
	MaxBackoff time.Duration
	// Multiplier is the exponential backoff multiplier; 1.0 = constant
	// backoff. Default 2.0.
	Multiplier float64
	// RetriableStatus enumerates HTTP statuses that trigger a retry.
	// Default {502, 503, 504}.
	RetriableStatus []int
	// RetryNonIdempotent also retries a method that is not idempotent
	// (RFC 9110 §9.2.2), such as POST or PATCH, when the status is
	// retriable. GET, HEAD, OPTIONS, PUT and DELETE are retried without
	// it. Defaults to false: the SDK does not retry non-idempotent
	// methods unless the consumer opts in.
	RetryNonIdempotent bool
}

// NoRetry is the canonical "exactly one attempt, no retries" policy.
// Equivalent to RetryPolicy{Disabled: true}. Use it when constructing
// a transport.Client to make the intent explicit at the call site.
var NoRetry = RetryPolicy{Disabled: true}

func (p RetryPolicy) enabled() bool {
	if p.Disabled {
		return false
	}
	return p.MaxAttempts > 1
}

func (p RetryPolicy) backoff(attempt int) time.Duration {
	if attempt <= 0 {
		return 0
	}
	mul := p.Multiplier
	if mul <= 0 {
		mul = 2.0
	}
	d := float64(p.InitialBackoff) * math.Pow(mul, float64(attempt-1))
	// Written so that NaN fails it too: a NaN wait is no wait.
	if !(d > 0) {
		return 0
	}
	// REQ-091: a float at or past 2^63 ns does not fit a Duration, and
	// converting it is implementation-defined (on amd64 the most negative
	// Duration), so the cap below would never apply and the retry loop would
	// skip the wait. Such a wait stops at MaxBackoff, or at the largest
	// Duration when there is no cap. float64(math.MaxInt64) is exactly 2^63.
	if d >= float64(math.MaxInt64) {
		if p.MaxBackoff > 0 {
			return p.MaxBackoff
		}
		return math.MaxInt64
	}
	// Every wait that fits is converted first and capped as a Duration, as
	// before: a float compare would round a cap above 2^53 ns.
	if w := time.Duration(d); p.MaxBackoff <= 0 || w <= p.MaxBackoff {
		return w
	}
	return p.MaxBackoff
}

func (p RetryPolicy) retriable(method string, status int) bool {
	if !p.retriableMethod(method) {
		return false
	}
	return slices.Contains(p.retriableStatuses(), status)
}

func (p RetryPolicy) retriableMethod(m string) bool {
	if p.RetryNonIdempotent {
		return true
	}
	switch m {
	case "GET", "HEAD", "OPTIONS", "PUT", "DELETE":
		return true
	default:
		return false
	}
}

func (p RetryPolicy) retriableStatuses() []int {
	if len(p.RetriableStatus) > 0 {
		return p.RetriableStatus
	}
	return []int{502, 503, 504}
}

// retryWait waits d before the next attempt. It returns ctx's error instead
// when ctx has already ended or ends during the wait, so no attempt starts
// after the caller gave up (REQ-091): that attempt could only fail before any
// response and drop the one Do already has. The ended context is read before
// the wait, because once a short timer has fired a select would pick between
// it and ctx.Done() at random. A zero or negative d does not wait.
func retryWait(ctx context.Context, d time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
