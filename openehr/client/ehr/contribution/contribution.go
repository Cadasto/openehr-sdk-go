package contribution

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	openehrclient "github.com/cadasto/openehr-sdk-go/openehr/client/ehr"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/transport"
)

const routeTemplate = "/ehr/{ehr_id}/contribution"

// commitConfig is the resolved option set for [Commit].
type commitConfig struct {
	prefer transport.Prefer
}

// CommitOption mutates [Commit]'s request shape.
type CommitOption func(*commitConfig)

// WithPrefer overrides the response-shape preference (REQ-094).
// Default [transport.PreferMinimal] — the spec write-path rule. With
// PreferRepresentation the server returns the persisted Contribution
// body which is decoded into the returned [*rm.Contribution].
func WithPrefer(p transport.Prefer) CommitOption {
	return func(c *commitConfig) { c.prefer = p }
}

// Commit posts a multi-version Contribution to ehrID. The audit
// envelope is carried inside the Contribution body (REQ-059); unlike
// per-resource writes there is no separate `openehr-audit-details`
// header.
//
// Wire: POST /ehr/{ehr_id}/contribution.
//
// The Contribution's UID field MAY be unset — the server assigns it
// on success. Concurrency failures within the batch surface as
// [transport.ErrVersionConflict].
func Commit(ctx context.Context, c *transport.Client, ehrID openehrclient.EHRID, batch *rm.Contribution, opts ...CommitOption) (*rm.Contribution, *openehrclient.VersionMetadata, error) {
	if ehrID == "" {
		return nil, nil, fmt.Errorf("contribution.Commit: %w: empty EHRID", transport.ErrInvalidConfig)
	}
	if batch == nil {
		return nil, nil, fmt.Errorf("contribution.Commit: %w: nil Contribution", transport.ErrInvalidConfig)
	}
	cfg := commitConfig{prefer: transport.PreferMinimal}
	for _, o := range opts {
		o(&cfg)
	}
	body, err := canjson.Marshal(batch)
	if err != nil {
		return nil, nil, fmt.Errorf("contribution.Commit: marshal body: %w", err)
	}
	req := &transport.Request{
		Method: http.MethodPost,
		Path:   "/ehr/" + url.PathEscape(string(ehrID)) + "/contribution",
		Route:  routeTemplate,
		Body:   body,
		Prefer: cfg.prefer,
	}
	resp, err := c.Do(ctx, req)
	if err != nil {
		if resp != nil {
			return nil, openehrclient.NewVersionMetadata(resp.Metadata), err
		}
		return nil, nil, err
	}
	meta := openehrclient.NewVersionMetadata(resp.Metadata)
	if cfg.prefer != transport.PreferRepresentation || len(resp.Body) == 0 {
		return nil, meta, nil
	}
	var out rm.Contribution
	if err := canjson.Unmarshal(resp.Body, &out); err != nil {
		return nil, meta, fmt.Errorf("contribution.Commit: decode response: %w", err)
	}
	return &out, meta, nil
}

// Repository mirrors the package functions for DI seams.
type Repository interface {
	Commit(ctx context.Context, ehrID openehrclient.EHRID, batch *rm.Contribution, opts ...CommitOption) (*rm.Contribution, *openehrclient.VersionMetadata, error)
}

// NewRepository binds c to a Repository.
func NewRepository(c *transport.Client) Repository { return &repository{c: c} }

type repository struct{ c *transport.Client }

func (r *repository) Commit(ctx context.Context, ehrID openehrclient.EHRID, batch *rm.Contribution, opts ...CommitOption) (*rm.Contribution, *openehrclient.VersionMetadata, error) {
	return Commit(ctx, r.c, ehrID, batch, opts...)
}
