package transport

import (
	"context"
	"math"
	"slices"
	"time"
)

// RetryPolicy configures retry-on-status behaviour. Disabled by default
// per REQ-091; enable via transport.WithRetry. Retries respect ctx
// cancellation immediately. See RetryPolicy.MaxAttempts / Disabled for
// the precise "no-retry" semantics (REQ-096).
type RetryPolicy struct {
	// Disabled, when true, forces exactly one attempt regardless of
	// MaxAttempts. Use NoRetry as the canonical zero-config form for
	// "no retries"; Disabled is useful when composing on top of a
	// caller-supplied base policy that already sets MaxAttempts.
	Disabled bool
	// MaxAttempts is the total attempt count: 0 means "use package
	// default" (currently disabled — one attempt); 1 means exactly one
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
	// Default {502, 503, 504} per the spec example.
	RetriableStatus []int
	// RetryNonIdempotent enables retrying POST/PATCH/DELETE etc. when
	// the status is retriable. Defaults to false — the SDK does not
	// retry non-idempotent methods unless the consumer opts in.
	RetryNonIdempotent bool
}

// NoRetry is the canonical "exactly one attempt, no retries" policy.
// Equivalent to RetryPolicy{Disabled: true}. Use it when constructing
// a transport.Client to make the intent explicit at the call site
// (REQ-096).
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
	if d <= 0 {
		return 0
	}
	if p.MaxBackoff > 0 && time.Duration(d) > p.MaxBackoff {
		return p.MaxBackoff
	}
	return time.Duration(d)
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

// retryWait sleeps for d honouring ctx cancellation.
func retryWait(ctx context.Context, d time.Duration) error {
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
