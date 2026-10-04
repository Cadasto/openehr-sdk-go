package smart

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/cadasto/openehr-sdk-go/auth"
)

const defaultJWKSTTL = 5 * time.Minute

// JWKS holds a cached JSON Web Key Set.
type JWKS struct {
	HTTPClient *http.Client
	URI        string
	// TTL is how long a fetched set is used before a lookup fetches it again.
	// [NewJWKS] sets five minutes. Zero or a negative value keeps the set
	// until a lookup misses: it never goes stale by time.
	TTL time.Duration

	mu   sync.Mutex
	keys map[string]json.RawMessage // keys published with a kid, by kid
	// signing holds every key whose use, if present, is "sig", with or
	// without a kid, in document order. A lookup without a kid reads it.
	signing   []json.RawMessage
	fetchedAt time.Time
	inflight  *jwksRefresh
}

// jwksRefresh carries the result of a single coalesced refresh so that
// waiters observe the leader's outcome rather than assuming success. err is
// written before done is closed, so a receive on done happens-after the write.
type jwksRefresh struct {
	done chan struct{}
	err  error
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
		HTTPClient: httpClient,
		URI:        uri,
		TTL:        defaultJWKSTTL,
		keys:       map[string]json.RawMessage{},
	}, nil
}

// Key returns the JWK document for kid. On cache miss the JWKS document
// is refreshed once before failing.
//
// An empty kid asks for the key that verifies a token whose header carries
// no kid: the set's only signing key, which is a key whose use, if present,
// is "sig", published with or without a kid. When the set holds no signing
// key or more than one, Key fails with [auth.ErrJWKSValidationFailed]; it
// fetches the set when the cache is empty or older than TTL, but does not
// refresh it on that failure.
func (j *JWKS) Key(ctx context.Context, kid string) (json.RawMessage, error) {
	if kid == "" {
		return j.onlySigningKey(ctx)
	}
	var refreshed bool
	for {
		j.mu.Lock()
		stale := j.staleLocked()
		k, ok := j.keys[kid]
		j.mu.Unlock()
		if ok && !stale {
			return k, nil
		}
		if refreshed {
			break
		}
		if err := j.refresh(ctx); err != nil {
			return nil, err
		}
		refreshed = true
	}
	return nil, fmt.Errorf("%w: kid %q not found after refresh", auth.ErrJWKSValidationFailed, kid)
}

// onlySigningKey returns the set's only signing key, for a token without a
// kid (OpenID Connect Core 1.0 §10.1, REQ-062).
func (j *JWKS) onlySigningKey(ctx context.Context) (json.RawMessage, error) {
	j.mu.Lock()
	stale := j.staleLocked()
	j.mu.Unlock()
	if stale {
		if err := j.refresh(ctx); err != nil {
			return nil, err
		}
	}
	return j.cachedSigningKey()
}

// refreshedSigningKey refreshes the set, sharing a refresh already in flight,
// and returns its only signing key. It serves a token without a kid that the
// cached key did not verify (REQ-062).
func (j *JWKS) refreshedSigningKey(ctx context.Context) (json.RawMessage, error) {
	if err := j.refresh(ctx); err != nil {
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

func (j *JWKS) refresh(ctx context.Context) error {
	j.mu.Lock()
	if r := j.inflight; r != nil {
		j.mu.Unlock()
		select {
		case <-r.done:
			// Surface the leader's outcome: if its fetch failed, return that
			// error rather than falling through to a misleading "kid not found
			// after refresh" on the caller's next lookup.
			return r.err
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	r := &jwksRefresh{done: make(chan struct{})}
	j.inflight = r
	j.mu.Unlock()

	err := j.fetch(ctx)

	j.mu.Lock()
	r.err = err
	j.inflight = nil
	j.mu.Unlock()
	close(r.done)
	return err
}

func (j *JWKS) fetch(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, j.URI, nil)
	if err != nil {
		return err
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
	var doc struct {
		Keys []json.RawMessage `json:"keys"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
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
		if err := json.Unmarshal(raw, &meta); err != nil || meta == nil {
			continue
		}
		// A key without a kid is kept for a token without a kid (REQ-062).
		if meta.Use == nil || meta.Use == "sig" {
			signing = append(signing, raw)
		}
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
