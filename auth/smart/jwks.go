package smart

import (
	"context"
	"encoding/json"
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/cadasto/openehr-sdk-go/auth"
	"github.com/cadasto/openehr-sdk-go/internal/coalesce"
	"github.com/cadasto/openehr-sdk-go/internal/urlredact"
)

const (
	defaultJWKSTTL                = 5 * time.Minute
	defaultJWKSMinRefreshInterval = 30 * time.Second
)

// JWKS holds a cached JSON Web Key Set.
//
// Build one with [NewJWKS]. The exported fields are read without the lock,
// so set them before the first lookup and leave them as they are afterwards.
// A JWKS built as a literal without an HTTPClient does not panic: every
// fetch fails with an error matching [auth.ErrInvalidConfig].
type JWKS struct {
	HTTPClient *http.Client
	URI        string
	// TTL is how long a fetched set is used before a lookup fetches it again.
	// [NewJWKS] sets five minutes. Zero or a negative value keeps the set
	// until a lookup misses: it never goes stale by time.
	TTL time.Duration
	// MinRefreshInterval is how old the fetched set must be before a lookup
	// for a kid the set does not hold fetches it again. A lookup that misses
	// a set fetched more recently fails at once, without a fetch, so tokens
	// that name unknown kids cannot make the set be fetched for each of
	// them. [NewJWKS] sets 30 seconds. Zero or a negative value sets no
	// minimum: every miss fetches the set. A set older than a positive TTL
	// is fetched again whatever this interval.
	MinRefreshInterval time.Duration

	mu sync.Mutex
	// keys holds the signing keys published with a kid, by kid. A signing
	// key is one whose use, if present, is "sig".
	keys map[string]json.RawMessage
	// signing holds every signing key, with or without a kid, in document
	// order. A lookup without a kid reads it.
	signing   []json.RawMessage
	fetchedAt time.Time
	inflight  *jwksRefresh
}

// jwksRefresh carries the result of a single coalesced refresh so that
// waiters observe the leader's outcome rather than assuming success. err and
// retry are written before done is closed, so a receive on done
// happens-after the writes.
type jwksRefresh struct {
	done chan struct{}
	err  error
	// retry reports that the refresh has no result for its waiters: it
	// failed only because the context of the lookup leading it ended, or a
	// panic unwound that lookup before the fetch returned. A waiter whose
	// own context is live then refreshes again.
	retry bool
}

// NewJWKS constructs a JWKS fetcher for uri. HTTPClient is required.
func NewJWKS(httpClient *http.Client, uri string) (*JWKS, error) {
	if httpClient == nil {
		return nil, fmt.Errorf("%w: HTTPClient is required", auth.ErrInvalidConfig)
	}
	if uri == "" {
		return nil, fmt.Errorf("%w: JWKS URI is required", auth.ErrInvalidConfig)
	}
	return &JWKS{
		HTTPClient:         httpClient,
		URI:                uri,
		TTL:                defaultJWKSTTL,
		MinRefreshInterval: defaultJWKSMinRefreshInterval,
		keys:               map[string]json.RawMessage{},
	}, nil
}

// Key returns the signing key the set publishes under kid, as a JWK
// document. A signing key is one whose use, if present, is "sig", so a key
// published for encryption is never returned. On a cache miss the set is
// fetched once more before Key fails, unless it was fetched less than
// MinRefreshInterval ago: Key then fails at once with
// [auth.ErrJWKSValidationFailed].
//
// An empty kid asks for the key that verifies a token whose header carries
// no kid: the set's only signing key, published with or without a kid. When
// the set holds no signing key or more than one, Key fails with
// [auth.ErrJWKSValidationFailed]. It fetches the set when the cache is empty
// or older than TTL, but does not refresh it on that failure.
func (j *JWKS) Key(ctx context.Context, kid string) (json.RawMessage, error) {
	if kid == "" {
		return j.onlySigningKey(ctx)
	}
	j.mu.Lock()
	stale := j.staleLocked()
	recent := j.fetchedWithinLocked(j.MinRefreshInterval)
	seen := j.fetchedAt
	k, ok := j.keys[kid]
	j.mu.Unlock()
	if ok && !stale {
		return k, nil
	}
	if !stale && recent {
		// A miss on a set fetched within the minimum interval fails without
		// a fetch, so unknown kids cannot make a fetch each (REQ-062).
		return nil, fmt.Errorf("%w: kid %q not found in a key set fetched less than %v ago", auth.ErrJWKSValidationFailed, kid, j.MinRefreshInterval)
	}
	if err := j.refresh(ctx, seen); err != nil {
		return nil, err
	}
	// This lookup fetched the set or waited on a fetch: look kid up in what
	// was fetched without asking again whether it is stale, so a TTL shorter
	// than one fetch does not hide a key the set publishes.
	j.mu.Lock()
	k, ok = j.keys[kid]
	j.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("%w: kid %q not found after refresh", auth.ErrJWKSValidationFailed, kid)
	}
	return k, nil
}

// onlySigningKey returns the set's only signing key, for a token without a
// kid (OpenID Connect Core 1.0 §10.1, REQ-062).
func (j *JWKS) onlySigningKey(ctx context.Context) (json.RawMessage, error) {
	j.mu.Lock()
	stale := j.staleLocked()
	seen := j.fetchedAt
	j.mu.Unlock()
	if stale {
		if err := j.refresh(ctx, seen); err != nil {
			return nil, err
		}
	}
	return j.cachedSigningKey()
}

// refreshedSigningKey refreshes the set, sharing a refresh already in flight,
// and returns its only signing key. It serves a token without a kid that the
// cached key did not verify (REQ-062). As for a kid miss, a set fetched less
// than MinRefreshInterval ago is not fetched again: the set's signing key is
// returned as it is, which another lookup's fetch may have replaced since
// the caller read it.
func (j *JWKS) refreshedSigningKey(ctx context.Context) (json.RawMessage, error) {
	j.mu.Lock()
	recent := !j.staleLocked() && j.fetchedWithinLocked(j.MinRefreshInterval)
	seen := j.fetchedAt
	j.mu.Unlock()
	if recent {
		return j.cachedSigningKey()
	}
	if err := j.refresh(ctx, seen); err != nil {
		return nil, err
	}
	return j.cachedSigningKey()
}

// cachedSigningKey returns the cached set's only signing key without fetching.
func (j *JWKS) cachedSigningKey() (json.RawMessage, error) {
	j.mu.Lock()
	signing := j.signing
	j.mu.Unlock()
	if len(signing) != 1 {
		return nil, fmt.Errorf("%w: no kid given and the JWKS holds %d signing keys, want exactly one", auth.ErrJWKSValidationFailed, len(signing))
	}
	return signing[0], nil
}

func (j *JWKS) staleLocked() bool {
	if j.fetchedAt.IsZero() {
		return true
	}
	if j.TTL <= 0 {
		return false
	}
	return time.Since(j.fetchedAt) >= j.TTL
}

// fetchedWithinLocked reports whether the cached set was fetched less than d
// ago. It reports false for a d of zero or less, and when no set has been
// fetched. The caller holds j.mu.
func (j *JWKS) fetchedWithinLocked(d time.Duration) bool {
	return d > 0 && !j.fetchedAt.IsZero() && time.Since(j.fetchedAt) < d
}

// refresh fetches the set, or waits for the fetch another lookup leads,
// unless a fetch has landed since the caller read the set at seen: the set
// is then already newer than the one the caller found wanting. The
// fetch runs under the context of the lookup that leads it. When that
// context ending is all that failed the fetch, or a panic unwound the
// leading lookup, a waiter whose own context is live goes round again: it
// leads a new fetch or waits on a newer one. A waiter whose context has
// ended returns its own context's error. Every other outcome reaches every
// waiter.
func (j *JWKS) refresh(ctx context.Context, seen time.Time) error {
	for {
		j.mu.Lock()
		if j.fetchedAt.After(seen) {
			// A fetch landed after the caller read the set at seen, so the
			// set is already newer than the one it found wanting.
			j.mu.Unlock()
			return nil
		}
		r := j.inflight
		if r == nil {
			r = &jwksRefresh{done: make(chan struct{})}
			j.inflight = r
			j.mu.Unlock()
			return j.leadRefresh(ctx, r)
		}
		j.mu.Unlock()
		select {
		case <-r.done:
		case <-ctx.Done():
			return ctx.Err()
		}
		if !r.retry {
			// Surface the leader's outcome: if its fetch failed, return that
			// error rather than falling through to a misleading "kid not found
			// after refresh" on the caller's next lookup.
			return r.err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
	}
}

// leadRefresh runs the fetch r stands for on behalf of every lookup waiting
// on it and publishes its outcome to them.
//
// A panic in the fetch unwinds leadRefresh before the outcome is published.
// The deferred release still takes r out of flight and wakes the waiters,
// with r marked for retry so that they fetch again, and the panic goes on to
// the caller as it was raised.
func (j *JWKS) leadRefresh(ctx context.Context, r *jwksRefresh) error {
	// r stays marked for retry unless the fetch returns.
	r.retry = true
	defer j.release(r)

	err := j.fetch(ctx)
	r.err = err
	r.retry = coalesce.EndedByContext(ctx, err)
	return err
}

// release takes r out of flight, unless a newer refresh has replaced it
// there, then wakes the lookups waiting on it. leadRefresh defers it, so it
// runs exactly once per refresh, also when a panic unwinds leadRefresh.
func (j *JWKS) release(r *jwksRefresh) {
	j.mu.Lock()
	if j.inflight == r {
		j.inflight = nil
	}
	j.mu.Unlock()
	close(r.done)
}

func (j *JWKS) fetch(ctx context.Context) error {
	if j.HTTPClient == nil {
		// A JWKS built as a literal rather than by NewJWKS may lack one.
		return fmt.Errorf("%w: the JWKS has no HTTPClient: build it with NewJWKS", auth.ErrInvalidConfig)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, j.URI, nil)
	if err != nil {
		// url.Parse's error quotes the URI, which may carry a password.
		return urlredact.Error(err, j.URI)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := j.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Drain a bounded prefix so the connection can be reused, then
		// fail without parsing the (irrelevant) error body.
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
		return fmt.Errorf("jwks fetch: status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	// JWK member names are case-sensitive (RFC 7517 §4), so the set and each
	// key match names exactly. A repeated name is allowed in the set, so that
	// one key with a repeated member does not take the other keys down, and
	// refused in a key, which is then skipped as RFC 7517 §4 allows.
	var doc struct {
		Keys []json.RawMessage `json:"keys"`
	}
	if err := jsonv2.Unmarshal(body, &doc, jsontext.AllowDuplicateNames(true)); err != nil {
		return fmt.Errorf("jwks decode: %w", err)
	}
	keys := make(map[string]json.RawMessage, len(doc.Keys))
	var signing []json.RawMessage
	for _, raw := range doc.Keys {
		// A pointer stays nil for a null entry, which is not a key.
		var meta *struct {
			Kid string `json:"kid"`
			Use any    `json:"use"`
		}
		if err := jsonv2.Unmarshal(raw, &meta); err != nil || meta == nil {
			continue
		}
		// Only a signing key verifies a token, whether the token's kid names
		// it or the token has no kid. A key without a kid is kept for a token
		// without a kid (REQ-062).
		if meta.Use != nil && meta.Use != "sig" {
			continue
		}
		signing = append(signing, raw)
		if meta.Kid != "" {
			keys[meta.Kid] = raw
		}
	}
	j.mu.Lock()
	j.keys = keys
	j.signing = signing
	j.fetchedAt = time.Now()
	j.mu.Unlock()
	return nil
}
