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

// JWKS holds a cached JSON Web Key Set (REQ-062).
type JWKS struct {
	HTTPClient *http.Client
	URI        string
	TTL        time.Duration

	mu        sync.Mutex
	keys      map[string]json.RawMessage
	fetchedAt time.Time
	inflight  chan struct{}
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
// is refreshed once before failing (REQ-062).
func (j *JWKS) Key(ctx context.Context, kid string) (json.RawMessage, error) {
	if kid == "" {
		return nil, fmt.Errorf("%w: empty kid", auth.ErrJWKSValidationFailed)
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
	if j.inflight != nil {
		ch := j.inflight
		j.mu.Unlock()
		select {
		case <-ch:
		case <-ctx.Done():
			return ctx.Err()
		}
		return nil
	}
	j.inflight = make(chan struct{})
	j.mu.Unlock()

	err := j.fetch(ctx)

	j.mu.Lock()
	close(j.inflight)
	j.inflight = nil
	j.mu.Unlock()
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
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("jwks fetch: status %d", resp.StatusCode)
	}
	var doc struct {
		Keys []json.RawMessage `json:"keys"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return fmt.Errorf("jwks decode: %w", err)
	}
	keys := make(map[string]json.RawMessage, len(doc.Keys))
	for _, raw := range doc.Keys {
		var meta struct {
			Kid string `json:"kid"`
		}
		if err := json.Unmarshal(raw, &meta); err != nil || meta.Kid == "" {
			continue
		}
		keys[meta.Kid] = raw
	}
	j.mu.Lock()
	j.keys = keys
	j.fetchedAt = time.Now()
	j.mu.Unlock()
	return nil
}
