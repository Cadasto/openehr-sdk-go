package transport

import (
	"bytes"
	"io"
	"math"
	"net/http"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/cadasto/openehr-sdk-go/smart/discovery"
)

// TestREQ091_BackoffSaturatesInsteadOfOverflowing pins the wait for a large
// attempt number or a large Multiplier. Past 2^63 ns the float product no
// longer fits a time.Duration; converting it anyway gives an
// implementation-defined value (on amd64 the most negative Duration), the
// MaxBackoff cap then never applies, and the retry loop, which skips a wait
// of zero or less, sends the next attempts back to back. The wait instead
// stops at MaxBackoff, or at the largest Duration when MaxBackoff is unset,
// and every value that fits keeps exactly the value it had.
func TestREQ091_BackoffSaturatesInsteadOfOverflowing(t *testing.T) {
	t.Parallel()
	capped := RetryPolicy{InitialBackoff: 100 * time.Millisecond, MaxBackoff: 30 * time.Second, Multiplier: 2}
	uncapped := RetryPolicy{InitialBackoff: 100 * time.Millisecond, Multiplier: 2}
	// 2^53+4 ns is exactly representable as a float64 and 2^53+3 ns is not:
	// it rounds up to 2^53+4. Comparing the cap as a float would let this
	// wait through 1 ns over its cap.
	const big = time.Duration(1<<53 + 4)
	tests := []struct {
		name    string
		policy  RetryPolicy
		attempt int
		want    time.Duration
	}{
		{name: "attempt 0 waits nothing", policy: capped, attempt: 0, want: 0},
		{name: "first retry waits InitialBackoff", policy: capped, attempt: 1, want: 100 * time.Millisecond},
		{name: "second retry doubles", policy: capped, attempt: 2, want: 200 * time.Millisecond},
		{name: "ninth retry under the cap", policy: capped, attempt: 9, want: 25600 * time.Millisecond},
		{name: "tenth retry capped", policy: capped, attempt: 10, want: 30 * time.Second},
		{name: "attempt 37 capped", policy: capped, attempt: 37, want: 30 * time.Second},
		{name: "attempt 38 past 2^63 ns stays capped", policy: capped, attempt: 38, want: 30 * time.Second},
		{name: "attempt 39 stays capped", policy: capped, attempt: 39, want: 30 * time.Second},
		{name: "attempt 1000 overflows the float to +Inf, stays capped", policy: capped, attempt: 1000, want: 30 * time.Second},
		{name: "uncapped attempt 37 fits", policy: uncapped, attempt: 37, want: time.Duration(100 * float64(time.Millisecond) * math.Pow(2, 36))},
		{name: "uncapped attempt 38 saturates", policy: uncapped, attempt: 38, want: math.MaxInt64},
		{name: "uncapped attempt 1000 saturates", policy: uncapped, attempt: 1000, want: math.MaxInt64},
		{
			name:    "large Multiplier capped",
			policy:  RetryPolicy{InitialBackoff: time.Millisecond, MaxBackoff: time.Second, Multiplier: 1e6},
			attempt: 4, want: time.Second,
		},
		{
			name:    "large Multiplier uncapped saturates",
			policy:  RetryPolicy{InitialBackoff: time.Millisecond, Multiplier: 1e300},
			attempt: 3, want: math.MaxInt64,
		},
		{
			name:    "NaN Multiplier waits nothing",
			policy:  RetryPolicy{InitialBackoff: time.Millisecond, MaxBackoff: time.Second, Multiplier: math.NaN()},
			attempt: 2, want: 0,
		},
		{
			name:    "cap compared as a Duration, not a rounded float",
			policy:  RetryPolicy{InitialBackoff: big, MaxBackoff: big - 1, Multiplier: 1},
			attempt: 1, want: big - 1,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.policy.backoff(tc.attempt); got != tc.want {
				t.Errorf("%+v.backoff(%d) = %v, want %v", tc.policy, tc.attempt, got, tc.want)
			}
		})
	}
}

// always503 answers every request with a 503 and counts the requests.
type always503 struct{ hits *atomic.Int32 }

func (a always503) RoundTrip(r *http.Request) (*http.Response, error) {
	a.hits.Add(1)
	return &http.Response{
		StatusCode: http.StatusServiceUnavailable,
		Header:     http.Header{},
		Body:       io.NopCloser(bytes.NewReader(nil)),
		Request:    r,
	}, nil
}

// TestREQ091_BackoffWaitSurvivesOverflowThroughDo drives the retry loop on
// a fake clock. With InitialBackoff 1ms, Multiplier 1e6 and MaxBackoff 1s the
// first wait is 1ms and every later one is capped at 1s, so five waits take
// 4.001s. From the fourth wait on the float product is past 2^63 ns; a wait
// lost to the overflow would end the call early, sending its attempts back to
// back at a server that is already answering 503.
func TestREQ091_BackoffWaitSurvivesOverflowThroughDo(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cat, err := discovery.NewStaticCatalog(discovery.StaticConfig{
			Issuer: "https://idp.example",
			Services: map[string]discovery.ServiceEntry{
				discovery.ServiceIDOpenEHRRest: {BaseURL: discovery.MustParseURL("https://cdr.example/openehr/v1"), SpecVersion: discovery.SpecVersionPin},
			},
		})
		if err != nil {
			t.Fatalf("NewStaticCatalog: %v", err)
		}
		var hits atomic.Int32
		c, err := New(cat,
			WithHTTPClient(&http.Client{Transport: always503{hits: &hits}}),
			WithRetry(RetryPolicy{MaxAttempts: 6, InitialBackoff: time.Millisecond, Multiplier: 1e6, MaxBackoff: time.Second}),
		)
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		start := time.Now()
		_, err = c.Do(t.Context(), &Request{Method: http.MethodGet, Path: "/x", Route: "/x"})
		elapsed := time.Since(start)
		if err == nil {
			t.Fatal("Do against a server answering 503 returned no error")
		}
		if n := hits.Load(); n != 6 {
			t.Errorf("attempts = %d, want 6", n)
		}
		if want := time.Millisecond + 4*time.Second; elapsed != want {
			t.Errorf("total wait = %v, want %v: a wait after the overflow was skipped", elapsed, want)
		}
	})
}
