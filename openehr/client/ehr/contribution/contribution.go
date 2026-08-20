package contribution

import (
	"bytes"
	"context"
	"fmt"
	"net/http"

	openehrclient "github.com/cadasto/openehr-sdk-go/openehr/client/ehr"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/transport"
)

const (
	routeTemplate = "/ehr/{ehr_id}/contribution"
	routeGet      = "/ehr/{ehr_id}/contribution/{contribution_uid}"
)

// commitConfig is the resolved option set for [Commit].
type commitConfig struct {
	prefer transport.Prefer
}

// CommitOption mutates [Commit]'s request shape.
type CommitOption func(*commitConfig)

// WithPrefer overrides the response-shape preference (REQ-094).
// Default [transport.PreferMinimal] — the spec write-path rule. With
// PreferRepresentation the server returns the persisted Contribution
// body which is decoded into the returned [*rm.Contribution]; an empty
// or undecodable 2xx body is a [*openehrclient.NoRepresentationError],
// not a silent metadata-only success. PreferIdentifier is metadata-only
// today (identifier-slot population is deferred).
func WithPrefer(p transport.Prefer) CommitOption {
	return func(c *commitConfig) { c.prefer = p }
}

// Commit posts a multi-version Contribution to ehrID. The audit
// envelope is carried inside the Submission body (REQ-059); unlike
// per-resource writes there is no separate `openehr-audit-details`
// header.
//
// Wire: POST /ehr/{ehr_id}/contribution. Request body is the
// ITS-REST `Contribution_create` schema — `{audit, versions[]}` with
// each `versions[i]` an inline `ORIGINAL_VERSION<T>` or
// `IMPORTED_VERSION<T>` (REQ-050/095 / PROBE-072), NOT the persisted
// `rm.Contribution` shape whose `versions[]` is `[]OBJECT_REF`. The
// response decodes as `*rm.Contribution` (persisted shape, returned
// under `Prefer: return=representation`). After 2xx +
// PreferRepresentation, an empty or undecodable body is a
// [*openehrclient.NoRepresentationError] that carries the commit
// metadata; a non-2xx stays a [*transport.WireError]. A successful
// minimal or identifier write returns a nil `*rm.Contribution` —
// `== nil` is a correct test for this concrete return;
// [openehrclient.HasResource] is the uniform presence test across
// write leaves, including interface returns.
//
// Concurrency failures within the batch surface as
// [transport.ErrVersionConflict].
func Commit(ctx context.Context, c *transport.Client, ehrID openehrclient.EHRID, batch *Submission, opts ...CommitOption) (*rm.Contribution, *openehrclient.VersionMetadata, error) {
	if ehrID == "" {
		return nil, nil, fmt.Errorf("contribution.Commit: %w: empty EHRID", transport.ErrInvalidConfig)
	}
	if batch == nil {
		return nil, nil, fmt.Errorf("contribution.Commit: %w: nil Submission", transport.ErrInvalidConfig)
	}
	if err := batch.Validate(); err != nil {
		return nil, nil, fmt.Errorf("contribution.Commit: %w: %w", transport.ErrInvalidConfig, err)
	}
	cfg := commitConfig{prefer: transport.PreferMinimal}
	for _, o := range opts {
		if o != nil {
			o(&cfg)
		}
	}
	body, err := canjson.Marshal(batch)
	if err != nil {
		return nil, nil, fmt.Errorf("contribution.Commit: marshal body: %w", err)
	}
	req := &transport.Request{
		Method: http.MethodPost,
		Path:   "/ehr/" + string(ehrID) + "/contribution",
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
	if cfg.prefer != transport.PreferRepresentation {
		return nil, meta, nil
	}
	// JSON null decodes into rm.Contribution as a nil-error no-op —
	// classify it as empty (REQ-094), same as the WriteResult family.
	if body := bytes.TrimSpace(resp.Body); len(body) == 0 || bytes.Equal(body, []byte("null")) {
		return nil, meta, &openehrclient.NoRepresentationError{
			Meta:  meta,
			Cause: fmt.Errorf("contribution.Commit: %w: Prefer=return=representation but response body is empty", transport.ErrInvalidShape),
		}
	}
	var out rm.Contribution
	if err := canjson.Unmarshal(resp.Body, &out); err != nil {
		return nil, meta, &openehrclient.NoRepresentationError{
			Meta:  meta,
			Cause: fmt.Errorf("contribution.Commit: decode response: %w", err),
		}
	}
	return &out, meta, nil
}

// Get reads a persisted contribution by uid (REQ-142 / PROBE-092).
//
// Wire: GET /ehr/{ehr_id}/contribution/{contribution_uid}
// (ITS-REST `contribution_get`). The response decodes as the persisted
// `*rm.Contribution` — `versions[]` of OBJECT_REF, not the
// `Contribution_create` submission shape [Commit] sends.
//
// The returned [*openehrclient.VersionMetadata] exists for shape
// consistency with the other EHR leaves and carries whatever headers
// the server sent: the vendored pin defines only `Content-Type` on
// `200_CONTRIBUTION` (`ETag` / `Location` belong to `201_CONTRIBUTION`),
// so callers MUST NOT require it to be populated on a read.
//
// An empty ehrID or contributionUID fails with
// [transport.ErrInvalidConfig] before any request is issued; a 404 maps
// to [transport.ErrNotFound]. v1 requests canonical JSON only —
// simplified-format Accept values are out of scope.
func Get(ctx context.Context, c *transport.Client, ehrID openehrclient.EHRID, contributionUID string) (*rm.Contribution, *openehrclient.VersionMetadata, error) {
	if ehrID == "" {
		return nil, nil, fmt.Errorf("contribution.Get: %w: empty EHRID", transport.ErrInvalidConfig)
	}
	if contributionUID == "" {
		return nil, nil, fmt.Errorf("contribution.Get: %w: empty contributionUID", transport.ErrInvalidConfig)
	}
	req := &transport.Request{
		Method: http.MethodGet,
		Path:   "/ehr/" + string(ehrID) + "/contribution/" + contributionUID,
		Route:  routeGet,
	}
	out, meta, err := transport.Decode[rm.Contribution](ctx, c, req)
	return out, openehrclient.NewVersionMetadata(meta), err
}

// Repository mirrors the package functions for DI seams.
type Repository interface {
	Commit(ctx context.Context, ehrID openehrclient.EHRID, batch *Submission, opts ...CommitOption) (*rm.Contribution, *openehrclient.VersionMetadata, error)
	Get(ctx context.Context, ehrID openehrclient.EHRID, contributionUID string) (*rm.Contribution, *openehrclient.VersionMetadata, error)
}

// NewRepository binds c to a Repository.
func NewRepository(c *transport.Client) Repository { return &repository{c: c} }

type repository struct{ c *transport.Client }

func (r *repository) Commit(ctx context.Context, ehrID openehrclient.EHRID, batch *Submission, opts ...CommitOption) (*rm.Contribution, *openehrclient.VersionMetadata, error) {
	return Commit(ctx, r.c, ehrID, batch, opts...)
}

func (r *repository) Get(ctx context.Context, ehrID openehrclient.EHRID, contributionUID string) (*rm.Contribution, *openehrclient.VersionMetadata, error) {
	return Get(ctx, r.c, ehrID, contributionUID)
}
