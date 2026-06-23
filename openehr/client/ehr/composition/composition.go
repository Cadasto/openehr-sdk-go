package composition

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	openehrclient "github.com/cadasto/openehr-sdk-go/openehr/client/ehr"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/transport"
)

const routeTemplate = "/ehr/{ehr_id}/composition/{versioned_object_or_version_uid}"

// ErrDeletedAtTime signals that the composition existed but had been
// (logically) deleted at the requested version_at_time — the server
// answered 204 No Content per the composition_get contract
// (resources/its-rest/ehr-validation.openapi.yaml, 204_deleted_at_time).
// It is a typed success signal, not a transport failure: [Get] returns it
// alongside the response metadata and a nil Composition. Check with
// errors.Is(err, composition.ErrDeletedAtTime).
var ErrDeletedAtTime = errors.New("composition: deleted at requested time")

// Get retrieves a Composition under the given EHR. The Ref selects
// the target: [ehr.LatestOf](voID) for the latest version of a
// versioned-object family, [ehr.LatestAtTime](voID, t) for the
// as-of-time variant, or [ehr.VersionOf](uid) for a specific version.
//
// Wire: GET /ehr/{ehr_id}/composition/{ref} [?version_at_time=...]. A 204
// No Content response (the composition was deleted at the requested
// version_at_time) returns a nil Composition with [ErrDeletedAtTime] — a
// typed success signal, distinct from transport.ErrInvalidShape.
func Get(ctx context.Context, c *transport.Client, ehrID openehrclient.EHRID, ref openehrclient.Ref) (*rm.Composition, *openehrclient.VersionMetadata, error) {
	if ehrID == "" {
		return nil, nil, fmt.Errorf("composition.Get: %w: empty EHRID", transport.ErrInvalidConfig)
	}
	if ref == nil {
		return nil, nil, fmt.Errorf("composition.Get: %w: nil Ref", transport.ErrInvalidConfig)
	}
	seg := ref.PathSegment()
	if seg == "" {
		return nil, nil, fmt.Errorf("composition.Get: %w: empty Ref path segment", transport.ErrInvalidConfig)
	}
	req := &transport.Request{
		Method: http.MethodGet,
		Path:   "/ehr/" + url.PathEscape(string(ehrID)) + "/composition/" + url.PathEscape(seg),
		Route:  routeTemplate,
	}
	if qk, qv := ref.Query(); qk != "" {
		req.Query = url.Values{qk: []string{qv}}
	}
	resp, err := c.Do(ctx, req)
	if err != nil {
		var meta *transport.Metadata
		if resp != nil {
			meta = resp.Metadata
		}
		return nil, openehrclient.NewVersionMetadata(meta), err
	}
	if resp.StatusCode == http.StatusNoContent {
		return nil, openehrclient.NewVersionMetadata(resp.Metadata), ErrDeletedAtTime
	}
	if len(resp.Body) == 0 {
		return nil, openehrclient.NewVersionMetadata(resp.Metadata),
			fmt.Errorf("composition.Get: %w: response body is empty", transport.ErrInvalidShape)
	}
	out := new(rm.Composition)
	if err := canjson.Unmarshal(resp.Body, out); err != nil {
		return nil, openehrclient.NewVersionMetadata(resp.Metadata),
			fmt.Errorf("composition.Get: decode: %w", err)
	}
	return out, openehrclient.NewVersionMetadata(resp.Metadata), nil
}

// writeConfig is the resolved option set for Save / Update.
type writeConfig struct {
	prefer          transport.Prefer
	auditDetails    *rm.AuditDetails
	templateID      string
	lifecycleState  string
	objectItemTags  []openehrclient.ItemTag
	versionItemTags []openehrclient.ItemTag
}

// WriteOption mutates the request shape for [Save] and [Update].
type WriteOption func(*writeConfig)

// WithPrefer overrides the response-shape preference (REQ-094). The
// default is [transport.PreferMinimal] per the spec.
func WithPrefer(p transport.Prefer) WriteOption {
	return func(c *writeConfig) { c.prefer = p }
}

// WithAuditDetails attaches the commit-time audit envelope via the
// `openehr-audit-details` header (REQ-059). Nil omits the header.
func WithAuditDetails(a *rm.AuditDetails) WriteOption {
	return func(c *writeConfig) { c.auditDetails = a }
}

// WithTemplateID sets the `openehr-template-id` header (REQ-059) so
// the deployment can validate the payload against the declared
// template. Empty omits the header.
func WithTemplateID(id string) WriteOption {
	return func(c *writeConfig) { c.templateID = id }
}

// WithLifecycleState sets the committed VERSION's lifecycle_state via the
// `openehr-version` header (REQ-059) — an openEHR "version lifecycle
// state" code (e.g. "532"). Empty omits the header (server default).
func WithLifecycleState(code string) WriteOption {
	return func(c *writeConfig) { c.lifecycleState = code }
}

// WithObjectItemTags sets the openehr-item-tag header (REQ-059).
func WithObjectItemTags(tags []openehrclient.ItemTag) WriteOption {
	return func(c *writeConfig) { c.objectItemTags = tags }
}

// WithVersionItemTags sets the openehr-version-item-tag header
// (REQ-059).
func WithVersionItemTags(tags []openehrclient.ItemTag) WriteOption {
	return func(c *writeConfig) { c.versionItemTags = tags }
}

// deleteConfig is the resolved option set for [Delete]. Delete does
// not return a body so it shares only the audit-details option with
// the writes.
type deleteConfig struct {
	auditDetails *rm.AuditDetails
}

// DeleteOption mutates [Delete]'s request shape.
type DeleteOption func(*deleteConfig)

// WithDeleteAudit attaches the commit-time audit envelope as the
// `openehr-audit-details` header on a delete (REQ-059). Nil omits.
func WithDeleteAudit(a *rm.AuditDetails) DeleteOption {
	return func(c *deleteConfig) { c.auditDetails = a }
}

// Save creates a new Composition under ehrID.
//
// Wire: POST /ehr/{ehr_id}/composition.
//
// The response shape follows the Prefer option (REQ-094) per the
// ITS-REST OpenAPI `201_COMPOSITION` response (SDK-GAP-09):
//   - PreferMinimal (default) — server returns the new version's
//     identifier in the `Location` header; the returned
//     `*rm.Composition` is nil and only the metadata is populated
//     (ETag + parsed VersionUID).
//   - PreferRepresentation — server returns the bare `COMPOSITION`
//     body (not `ORIGINAL_VERSION<COMPOSITION>`) which is decoded
//     into the returned value. The audit / lifecycle / preceding-
//     version fields that `ORIGINAL_VERSION` carries are not in the
//     POST/PUT body per spec; they are available via the version
//     metadata (`ETag` → `VersionUID`) or via a follow-up
//     `GET /versioned_composition/{vo_uid}/version/{version_uid}`
//     which is the canonical home for the `ORIGINAL_VERSION` envelope.
//   - PreferIdentifier — server returns the ITS-REST `Identifier` body
//     (`{"uid": …}`); the returned `*rm.Composition` is nil and the
//     identifier is resolved into the metadata `VersionUID` (the
//     `Location` header stays canonical when present).
//
// Audit details and the template id flow via the `openehr-*` header
// family (REQ-059).
func Save(ctx context.Context, c *transport.Client, ehrID openehrclient.EHRID, comp *rm.Composition, opts ...WriteOption) (*rm.Composition, *openehrclient.VersionMetadata, error) {
	if ehrID == "" {
		return nil, nil, fmt.Errorf("composition.Save: %w: empty EHRID", transport.ErrInvalidConfig)
	}
	if comp == nil {
		return nil, nil, fmt.Errorf("composition.Save: %w: nil Composition", transport.ErrInvalidConfig)
	}
	cfg := writeConfig{prefer: transport.PreferMinimal}
	for _, o := range opts {
		o(&cfg)
	}
	body, err := canjson.Marshal(comp)
	if err != nil {
		return nil, nil, fmt.Errorf("composition.Save: marshal body: %w", err)
	}
	auditHeader, err := openehrclient.MarshalAuditDetails(cfg.auditDetails)
	if err != nil {
		return nil, nil, fmt.Errorf("composition.Save: %w", err)
	}
	objectTags, versionTags, err := marshalItemTagHeaders(cfg.objectItemTags, cfg.versionItemTags)
	if err != nil {
		return nil, nil, fmt.Errorf("composition.Save: %w", err)
	}
	verHeader, err := openehrclient.FormatLifecycleStateHeader(cfg.lifecycleState)
	if err != nil {
		return nil, nil, fmt.Errorf("composition.Save: %w", err)
	}
	req := &transport.Request{
		Method:             http.MethodPost,
		Path:               "/ehr/" + url.PathEscape(string(ehrID)) + "/composition",
		Route:              "/ehr/{ehr_id}/composition",
		Body:               body,
		Prefer:             cfg.prefer,
		AuditDetailsHeader: auditHeader,
		RMVersion:          verHeader,
		TemplateID:         cfg.templateID,
		ItemTag:            objectTags,
		VersionItemTag:     versionTags,
	}
	return doWrite(ctx, c, req, cfg.prefer)
}

// Update modifies the Composition family identified by voID, attaching
// `ifMatch` as the required `If-Match` header (REQ-054).
//
// Wire: PUT /ehr/{ehr_id}/composition/{voID} with If-Match. Errors
// per REQ-093: 409 → [transport.ErrVersionConflict], 412 →
// [transport.ErrPreconditionFailed], 428 →
// [transport.ErrPreconditionRequired]. Forgetting ifMatch returns
// [transport.ErrInvalidConfig] without issuing a request.
//
// Response shape matches [Save]: bare `*rm.Composition` per the
// ITS-REST OpenAPI `200_COMPOSITION_updated` response (SDK-GAP-09).
func Update(ctx context.Context, c *transport.Client, ehrID openehrclient.EHRID, voID openehrclient.VersionedObjectID, ifMatch string, comp *rm.Composition, opts ...WriteOption) (*rm.Composition, *openehrclient.VersionMetadata, error) {
	if ehrID == "" {
		return nil, nil, fmt.Errorf("composition.Update: %w: empty EHRID", transport.ErrInvalidConfig)
	}
	if voID == "" {
		return nil, nil, fmt.Errorf("composition.Update: %w: empty VersionedObjectID", transport.ErrInvalidConfig)
	}
	if ifMatch == "" {
		return nil, nil, fmt.Errorf("composition.Update: %w: empty If-Match (REQ-054)", transport.ErrInvalidConfig)
	}
	if comp == nil {
		return nil, nil, fmt.Errorf("composition.Update: %w: nil Composition", transport.ErrInvalidConfig)
	}
	cfg := writeConfig{prefer: transport.PreferMinimal}
	for _, o := range opts {
		o(&cfg)
	}
	body, err := canjson.Marshal(comp)
	if err != nil {
		return nil, nil, fmt.Errorf("composition.Update: marshal body: %w", err)
	}
	auditHeader, err := openehrclient.MarshalAuditDetails(cfg.auditDetails)
	if err != nil {
		return nil, nil, fmt.Errorf("composition.Update: %w", err)
	}
	objectTags, versionTags, err := marshalItemTagHeaders(cfg.objectItemTags, cfg.versionItemTags)
	if err != nil {
		return nil, nil, fmt.Errorf("composition.Update: %w", err)
	}
	verHeader, err := openehrclient.FormatLifecycleStateHeader(cfg.lifecycleState)
	if err != nil {
		return nil, nil, fmt.Errorf("composition.Update: %w", err)
	}
	req := &transport.Request{
		Method:             http.MethodPut,
		Path:               "/ehr/" + url.PathEscape(string(ehrID)) + "/composition/" + url.PathEscape(string(voID)),
		Route:              "/ehr/{ehr_id}/composition/{versioned_object_id}",
		Body:               body,
		IfMatch:            ifMatch,
		Prefer:             cfg.prefer,
		AuditDetailsHeader: auditHeader,
		RMVersion:          verHeader,
		TemplateID:         cfg.templateID,
		ItemTag:            objectTags,
		VersionItemTag:     versionTags,
	}
	return doWrite(ctx, c, req, cfg.prefer)
}

// Delete logically deletes the Composition version addressed by
// versionUID, attaching the preceding version's identifier as
// `If-Match` (REQ-054). The server typically responds 204 No Content.
//
// Wire: DELETE /ehr/{ehr_id}/composition/{version_uid} with If-Match.
// REQ-054 enforcement mirrors [Update].
func Delete(ctx context.Context, c *transport.Client, ehrID openehrclient.EHRID, versionUID openehrclient.VersionUID, ifMatch string, opts ...DeleteOption) (*openehrclient.VersionMetadata, error) {
	if ehrID == "" {
		return nil, fmt.Errorf("composition.Delete: %w: empty EHRID", transport.ErrInvalidConfig)
	}
	if versionUID == "" {
		return nil, fmt.Errorf("composition.Delete: %w: empty VersionUID", transport.ErrInvalidConfig)
	}
	if ifMatch == "" {
		return nil, fmt.Errorf("composition.Delete: %w: empty If-Match (REQ-054)", transport.ErrInvalidConfig)
	}
	cfg := deleteConfig{}
	for _, o := range opts {
		o(&cfg)
	}
	auditHeader, err := openehrclient.MarshalAuditDetails(cfg.auditDetails)
	if err != nil {
		return nil, fmt.Errorf("composition.Delete: %w", err)
	}
	req := &transport.Request{
		Method:             http.MethodDelete,
		Path:               "/ehr/" + url.PathEscape(string(ehrID)) + "/composition/" + url.PathEscape(string(versionUID)),
		Route:              "/ehr/{ehr_id}/composition/{version_uid}",
		IfMatch:            ifMatch,
		AuditDetailsHeader: auditHeader,
	}
	resp, err := c.Do(ctx, req)
	if err != nil {
		if resp != nil {
			return openehrclient.NewVersionMetadata(resp.Metadata), err
		}
		return nil, err
	}
	return openehrclient.NewVersionMetadata(resp.Metadata), nil
}

// doWrite executes a Save / Update request and decodes the response
// body per the Prefer mode (REQ-094): Prefer=representation decodes the
// bare Composition (and returns [transport.ErrInvalidShape] on an empty
// body); Prefer=identifier resolves the ITS-REST Identifier body into
// the version metadata; for minimal / default the body is empty and the
// returned Composition pointer is nil.
//
// Per ITS-REST OpenAPI `201_COMPOSITION` / `200_COMPOSITION_updated`
// (SDK-GAP-09), the response body is a bare `Composition` — not an
// `ORIGINAL_VERSION<COMPOSITION>` envelope. The full audit / lifecycle
// version envelope lives at `GET /versioned_composition/{vo_uid}/
// version/{version_uid}` (`UVersionOfComposition`) which the
// consumer can fetch when needed.
func doWrite(ctx context.Context, c *transport.Client, req *transport.Request, prefer transport.Prefer) (*rm.Composition, *openehrclient.VersionMetadata, error) {
	resp, err := c.Do(ctx, req)
	if err != nil {
		if resp != nil {
			return nil, openehrclient.NewVersionMetadata(resp.Metadata), err
		}
		return nil, nil, err
	}
	meta := openehrclient.NewVersionMetadata(resp.Metadata)
	switch prefer {
	case transport.PreferRepresentation:
		if len(resp.Body) == 0 {
			// REQ-094: representation MUST NOT silently downgrade to an
			// empty body — surface it rather than returning a nil resource.
			return nil, meta, fmt.Errorf("composition: %w: Prefer=return=representation but response body is empty", transport.ErrInvalidShape)
		}
		var comp rm.Composition
		if err := canjson.Unmarshal(resp.Body, &comp); err != nil {
			return nil, meta, fmt.Errorf("composition: decode Composition: %w", err)
		}
		return &comp, meta, nil
	case transport.PreferIdentifier:
		// REQ-094: populate the identifier slot (meta.VersionUID) from the
		// ITS-REST Identifier body when present; never silently discard it.
		if err := meta.ResolveIdentifierBody(resp.Body); err != nil {
			return nil, meta, fmt.Errorf("composition: %w", err)
		}
		return nil, meta, nil
	default:
		// minimal / default: empty body expected; id is in Location/ETag.
		return nil, meta, nil
	}
}

func marshalItemTagHeaders(object, version []openehrclient.ItemTag) (objectHdr, versionHdr string, err error) {
	if len(object) > 0 {
		objectHdr, err = openehrclient.FormatItemTagHeader(object)
		if err != nil {
			return "", "", err
		}
	}
	if len(version) > 0 {
		versionHdr, err = openehrclient.FormatItemTagHeader(version)
		if err != nil {
			return "", "", err
		}
	}
	return objectHdr, versionHdr, nil
}

// Repository mirrors the package-level Composition functions.
type Repository interface {
	Get(ctx context.Context, ehrID openehrclient.EHRID, ref openehrclient.Ref) (*rm.Composition, *openehrclient.VersionMetadata, error)
	Save(ctx context.Context, ehrID openehrclient.EHRID, comp *rm.Composition, opts ...WriteOption) (*rm.Composition, *openehrclient.VersionMetadata, error)
	Update(ctx context.Context, ehrID openehrclient.EHRID, voID openehrclient.VersionedObjectID, ifMatch string, comp *rm.Composition, opts ...WriteOption) (*rm.Composition, *openehrclient.VersionMetadata, error)
	Delete(ctx context.Context, ehrID openehrclient.EHRID, versionUID openehrclient.VersionUID, ifMatch string, opts ...DeleteOption) (*openehrclient.VersionMetadata, error)
}

// NewRepository binds c to a Repository.
func NewRepository(c *transport.Client) Repository { return &repository{c: c} }

type repository struct{ c *transport.Client }

func (r *repository) Get(ctx context.Context, ehrID openehrclient.EHRID, ref openehrclient.Ref) (*rm.Composition, *openehrclient.VersionMetadata, error) {
	return Get(ctx, r.c, ehrID, ref)
}

func (r *repository) Save(ctx context.Context, ehrID openehrclient.EHRID, comp *rm.Composition, opts ...WriteOption) (*rm.Composition, *openehrclient.VersionMetadata, error) {
	return Save(ctx, r.c, ehrID, comp, opts...)
}

func (r *repository) Update(ctx context.Context, ehrID openehrclient.EHRID, voID openehrclient.VersionedObjectID, ifMatch string, comp *rm.Composition, opts ...WriteOption) (*rm.Composition, *openehrclient.VersionMetadata, error) {
	return Update(ctx, r.c, ehrID, voID, ifMatch, comp, opts...)
}

func (r *repository) Delete(ctx context.Context, ehrID openehrclient.EHRID, versionUID openehrclient.VersionUID, ifMatch string, opts ...DeleteOption) (*openehrclient.VersionMetadata, error) {
	return Delete(ctx, r.c, ehrID, versionUID, ifMatch, opts...)
}
