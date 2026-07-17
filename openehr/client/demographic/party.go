package demographic

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"

	openehrclient "github.com/cadasto/openehr-sdk-go/openehr/client/ehr"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/rm/typereg"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// Type is the concrete PARTY resource segment. The openEHR Demographic API
// has no generic `/demographic/party` endpoint: each concrete PARTY type is
// its own resource (`/demographic/person`, `/demographic/organisation`, …),
// all with identical versioned-CRUD shape. [Create] derives the segment from
// the value's concrete type; the read/update/delete paths take it explicitly
// because the caller addresses an existing resource by id, not by value.
type Type string

// The concrete openEHR PARTY resource types, each addressed at its own
// versioned-CRUD endpoint (/demographic/person, /demographic/organisation, …).
const (
	Person       Type = "person"
	Organisation Type = "organisation"
	Group        Type = "group"
	Agent        Type = "agent"
	Role         Type = "role"
)

func (t Type) valid() bool {
	switch t {
	case Person, Organisation, Group, Agent, Role:
		return true
	}
	return false
}

// resourceType maps a concrete PARTY value to its resource segment.
func resourceType(p rm.Party) (Type, error) {
	switch p.(type) {
	case *rm.Person, rm.Person:
		return Person, nil
	case *rm.Organisation, rm.Organisation:
		return Organisation, nil
	case *rm.Group, rm.Group:
		return Group, nil
	case *rm.Agent, rm.Agent:
		return Agent, nil
	case *rm.Role, rm.Role:
		return Role, nil
	default:
		return "", fmt.Errorf("%w: unsupported PARTY type %T", transport.ErrInvalidConfig, p)
	}
}

func basePath(t Type) string { return "/demographic/" + string(t) }

// Get retrieves a PARTY of the given type. The Ref selects the target:
// [ehr.LatestOf](voID) for the latest version of a versioned-object family,
// [ehr.LatestAtTime](voID, t) for the as-of-time variant, or
// [ehr.VersionOf](uid) for a specific version. The returned value is the
// concrete type ([*rm.Person] etc.) behind the [rm.Party] interface, decoded
// polymorphically by its `_type` discriminator (REQ-040).
//
// Wire: GET /demographic/{type}/{uid_based_id} [?version_at_time=...]. A 204
// (no version at the requested time) yields a nil Party and nil error; any
// other empty-body 2xx is surfaced as [transport.ErrInvalidShape].
func Get(ctx context.Context, c *transport.Client, t Type, ref openehrclient.Ref) (rm.Party, *openehrclient.VersionMetadata, error) {
	if !t.valid() {
		return nil, nil, fmt.Errorf("demographic.Get: %w: invalid PARTY type %q", transport.ErrInvalidConfig, t)
	}
	if ref == nil {
		return nil, nil, fmt.Errorf("demographic.Get: %w: nil Ref", transport.ErrInvalidConfig)
	}
	seg := ref.PathSegment()
	if seg == "" {
		return nil, nil, fmt.Errorf("demographic.Get: %w: empty Ref path segment", transport.ErrInvalidConfig)
	}
	req := &transport.Request{
		Method: http.MethodGet,
		Path:   basePath(t) + "/" + seg,
		Route:  basePath(t) + "/{uid_based_id}",
	}
	if qk, qv := ref.Query(); qk != "" {
		req.Query = url.Values{qk: []string{qv}}
	}
	return getParty(ctx, c, req)
}

// writeConfig is the resolved option set for Create / Update. It embeds
// the shared [openehrclient.WriteConfig] (Prefer / audit details /
// lifecycle state); demographic has no options beyond that.
type writeConfig struct {
	openehrclient.WriteConfig
}

// WriteOption mutates the request shape for [Create] and [Update].
type WriteOption func(*writeConfig)

// WithPrefer overrides the response-shape preference (REQ-094). The default
// is [transport.PreferMinimal] per the spec's write-path rule.
func WithPrefer(p transport.Prefer) WriteOption {
	return func(c *writeConfig) { c.Prefer = p }
}

// WithAuditDetails attaches the commit-time audit envelope via the
// `openehr-audit-details` header (REQ-059). Nil omits the header.
func WithAuditDetails(a *rm.AuditDetails) WriteOption {
	return func(c *writeConfig) { c.AuditDetails = a }
}

// WithLifecycleState sets the committed VERSION's lifecycle_state via the
// `openehr-version` header (REQ-059). Empty omits the header; an
// unrecognised code fails the write with [transport.ErrInvalidConfig].
func WithLifecycleState(s openehrclient.LifecycleState) WriteOption {
	return func(c *writeConfig) { c.LifecycleState = s }
}

// Create commits a new PARTY. The resource path is derived from party's
// concrete type ([*rm.Person] → /demographic/person, etc.).
//
// Wire: POST /demographic/{type}. The response shape follows the Prefer
// option (REQ-094): minimal (default) returns no body — the new version's id
// is in the Location / ETag headers and the returned Party is nil;
// representation returns the created PARTY body; identifier returns the
// ITS-REST Identifier body, resolved into the metadata VersionUID.
func Create(ctx context.Context, c *transport.Client, party rm.Party, opts ...WriteOption) (rm.Party, *openehrclient.VersionMetadata, error) {
	if party == nil {
		return nil, nil, fmt.Errorf("demographic.Create: %w: nil Party", transport.ErrInvalidConfig)
	}
	t, err := resourceType(party)
	if err != nil {
		return nil, nil, fmt.Errorf("demographic.Create: %w", err)
	}
	cfg := writeConfig{WriteConfig: openehrclient.WriteConfig{Prefer: transport.PreferMinimal}}
	for _, o := range opts {
		o(&cfg)
	}
	body, err := canjson.Marshal(party)
	if err != nil {
		return nil, nil, fmt.Errorf("demographic.Create: marshal body: %w", err)
	}
	auditHeader, err := cfg.ResolveAuditHeader("demographic.Create")
	if err != nil {
		return nil, nil, err
	}
	verHeader, err := cfg.ResolveLifecycleHeader("demographic.Create")
	if err != nil {
		return nil, nil, err
	}
	req := &transport.Request{
		Method:             http.MethodPost,
		Path:               basePath(t),
		Route:              basePath(t),
		Body:               body,
		Prefer:             cfg.Prefer,
		AuditDetailsHeader: auditHeader,
		RMVersion:          verHeader,
	}
	return openehrclient.WriteResult(ctx, c, req, "demographic", decodeParty)
}

// Update modifies the PARTY family identified by voID, attaching `ifMatch` as
// the required If-Match header (REQ-054) — the preceding version's id,
// typically the prior [openehrclient.VersionMetadata.VersionUID] / ETag.
//
// Wire: PUT /demographic/{type}/{voID} with If-Match. Errors per REQ-093:
// 412 → [transport.ErrPreconditionFailed]. Status→sentinel mapping is
// endpoint-agnostic, so a 409 (should the deployment emit one) still maps to
// [transport.ErrVersionConflict]. Forgetting ifMatch returns
// [transport.ErrInvalidConfig] without issuing a request. Response shape
// matches [Create].
func Update(ctx context.Context, c *transport.Client, t Type, voID openehrclient.VersionedObjectID, ifMatch string, party rm.Party, opts ...WriteOption) (rm.Party, *openehrclient.VersionMetadata, error) {
	if !t.valid() {
		return nil, nil, fmt.Errorf("demographic.Update: %w: invalid PARTY type %q", transport.ErrInvalidConfig, t)
	}
	if voID == "" {
		return nil, nil, fmt.Errorf("demographic.Update: %w: empty VersionedObjectID", transport.ErrInvalidConfig)
	}
	if ifMatch == "" {
		return nil, nil, fmt.Errorf("demographic.Update: %w: empty If-Match (REQ-054)", transport.ErrInvalidConfig)
	}
	if party == nil {
		return nil, nil, fmt.Errorf("demographic.Update: %w: nil Party", transport.ErrInvalidConfig)
	}
	cfg := writeConfig{WriteConfig: openehrclient.WriteConfig{Prefer: transport.PreferMinimal}}
	for _, o := range opts {
		o(&cfg)
	}
	body, err := canjson.Marshal(party)
	if err != nil {
		return nil, nil, fmt.Errorf("demographic.Update: marshal body: %w", err)
	}
	auditHeader, err := cfg.ResolveAuditHeader("demographic.Update")
	if err != nil {
		return nil, nil, err
	}
	verHeader, err := cfg.ResolveLifecycleHeader("demographic.Update")
	if err != nil {
		return nil, nil, err
	}
	req := &transport.Request{
		Method:             http.MethodPut,
		Path:               basePath(t) + "/" + string(voID),
		Route:              basePath(t) + "/{uid_based_id}",
		Body:               body,
		IfMatch:            ifMatch,
		Prefer:             cfg.Prefer,
		AuditDetailsHeader: auditHeader,
		RMVersion:          verHeader,
	}
	return openehrclient.WriteResult(ctx, c, req, "demographic", decodeParty)
}

// deleteConfig is the resolved option set for [Delete]. A logical delete is a
// versioned commit but returns no body, so it shares only the audit-details
// option with the writes.
type deleteConfig struct {
	auditDetails *rm.AuditDetails
}

// DeleteOption mutates [Delete]'s request shape.
type DeleteOption func(*deleteConfig)

// WithDeleteAudit attaches the commit-time audit envelope as the
// `openehr-audit-details` header on a logical delete (REQ-059). Nil omits it.
func WithDeleteAudit(a *rm.AuditDetails) DeleteOption {
	return func(c *deleteConfig) { c.auditDetails = a }
}

// Delete logically deletes the PARTY version addressed by versionUID,
// attaching the preceding version's id as If-Match (REQ-054). The server
// responds 204 No Content on success. Forgetting ifMatch returns
// [transport.ErrInvalidConfig] without issuing a request.
//
// Wire: DELETE /demographic/{type}/{version_uid} with If-Match. A
// referential-integrity conflict maps to [transport.ErrVersionConflict] (409).
func Delete(ctx context.Context, c *transport.Client, t Type, versionUID openehrclient.VersionUID, ifMatch string, opts ...DeleteOption) (*openehrclient.VersionMetadata, error) {
	if !t.valid() {
		return nil, fmt.Errorf("demographic.Delete: %w: invalid PARTY type %q", transport.ErrInvalidConfig, t)
	}
	if versionUID == "" {
		return nil, fmt.Errorf("demographic.Delete: %w: empty VersionUID", transport.ErrInvalidConfig)
	}
	if ifMatch == "" {
		return nil, fmt.Errorf("demographic.Delete: %w: empty If-Match (REQ-054)", transport.ErrInvalidConfig)
	}
	cfg := deleteConfig{}
	for _, o := range opts {
		o(&cfg)
	}
	auditHeader, err := openehrclient.MarshalAuditDetails(cfg.auditDetails)
	if err != nil {
		return nil, fmt.Errorf("demographic.Delete: %w", err)
	}
	req := &transport.Request{
		Method:             http.MethodDelete,
		Path:               basePath(t) + "/" + string(versionUID),
		Route:              basePath(t) + "/{version_uid}",
		IfMatch:            ifMatch,
		AuditDetailsHeader: auditHeader,
	}
	return openehrclient.DoDelete(ctx, c, req)
}

// getParty issues a read request and decodes the bare PARTY body
// polymorphically via the type registry (REQ-040). A 204 (e.g. a
// version_at_time with no matching version) yields a nil Party; any other
// empty-body 2xx is a wire anomaly surfaced as [transport.ErrInvalidShape].
func getParty(ctx context.Context, c *transport.Client, req *transport.Request) (rm.Party, *openehrclient.VersionMetadata, error) {
	resp, err := c.Do(ctx, req)
	if err != nil {
		if resp != nil {
			return nil, openehrclient.NewVersionMetadata(resp.Metadata), err
		}
		return nil, nil, err
	}
	meta := openehrclient.NewVersionMetadata(resp.Metadata)
	if len(resp.Body) == 0 {
		// Only a 204 legitimately carries no version body (e.g. a
		// version_at_time with no matching version). Any other empty-body 2xx
		// is a wire anomaly — surface it as ErrInvalidShape rather than
		// masquerading as "no version". (The concrete read leaves route through
		// transport.Decode, which rejects every empty body; this leaf decodes
		// rm.Party polymorphically so it can't, and instead applies the same
		// strictness while carving out the legitimate 204.)
		if resp.StatusCode == http.StatusNoContent {
			return nil, meta, nil
		}
		return nil, meta, fmt.Errorf("demographic: %w: %d response with empty body", transport.ErrInvalidShape, resp.StatusCode)
	}
	party, err := typereg.DecodeAs[rm.Party](resp.Body)
	if err != nil {
		return nil, meta, fmt.Errorf("demographic: decode PARTY body: %w", err)
	}
	return party, meta, nil
}

// decodeParty decodes a Prefer=representation write response
// polymorphically via the type registry (REQ-040).
func decodeParty(body []byte) (rm.Party, error) {
	party, err := typereg.DecodeAs[rm.Party](body)
	if err != nil {
		return nil, fmt.Errorf("demographic: decode PARTY body: %w", err)
	}
	return party, nil
}

// Repository mirrors the package-level functions for DI seams (REQ-023).
type Repository interface {
	Get(ctx context.Context, t Type, ref openehrclient.Ref) (rm.Party, *openehrclient.VersionMetadata, error)
	Create(ctx context.Context, party rm.Party, opts ...WriteOption) (rm.Party, *openehrclient.VersionMetadata, error)
	Update(ctx context.Context, t Type, voID openehrclient.VersionedObjectID, ifMatch string, party rm.Party, opts ...WriteOption) (rm.Party, *openehrclient.VersionMetadata, error)
	Delete(ctx context.Context, t Type, versionUID openehrclient.VersionUID, ifMatch string, opts ...DeleteOption) (*openehrclient.VersionMetadata, error)

	// Versioned-resource reads (Phase 2).
	GetVersionedParty(ctx context.Context, voUID openehrclient.VersionedObjectID) (*rm.VersionedParty, *openehrclient.VersionMetadata, error)
	GetRevisionHistory(ctx context.Context, voUID openehrclient.VersionedObjectID) (*rm.RevisionHistory, *openehrclient.VersionMetadata, error)
	GetVersion(ctx context.Context, voUID openehrclient.VersionedObjectID) (*PartyVersion, *openehrclient.VersionMetadata, error)
	GetVersionAtTime(ctx context.Context, voUID openehrclient.VersionedObjectID, t time.Time) (*PartyVersion, *openehrclient.VersionMetadata, error)
	GetVersionByID(ctx context.Context, voUID openehrclient.VersionedObjectID, versionUID openehrclient.VersionUID) (*PartyVersion, *openehrclient.VersionMetadata, error)
}

// NewRepository binds c to a Repository.
func NewRepository(c *transport.Client) Repository { return &repository{c: c} }

type repository struct{ c *transport.Client }

func (r *repository) Get(ctx context.Context, t Type, ref openehrclient.Ref) (rm.Party, *openehrclient.VersionMetadata, error) {
	return Get(ctx, r.c, t, ref)
}

func (r *repository) Create(ctx context.Context, party rm.Party, opts ...WriteOption) (rm.Party, *openehrclient.VersionMetadata, error) {
	return Create(ctx, r.c, party, opts...)
}

func (r *repository) Update(ctx context.Context, t Type, voID openehrclient.VersionedObjectID, ifMatch string, party rm.Party, opts ...WriteOption) (rm.Party, *openehrclient.VersionMetadata, error) {
	return Update(ctx, r.c, t, voID, ifMatch, party, opts...)
}

func (r *repository) Delete(ctx context.Context, t Type, versionUID openehrclient.VersionUID, ifMatch string, opts ...DeleteOption) (*openehrclient.VersionMetadata, error) {
	return Delete(ctx, r.c, t, versionUID, ifMatch, opts...)
}
