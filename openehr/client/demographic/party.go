package demographic

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"

	openehrclient "github.com/cadasto/openehr-sdk-go/openehr/client"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
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
// [openehrclient.LatestOf](voID) for the latest version of a
// versioned-object family, [openehrclient.LatestAtTime](voID, t) for the
// as-of-time variant, or [openehrclient.VersionOf](uid) for a specific
// version. A Ref that [openehrclient.ValidateRef] refuses, such as a nil
// Ref, an empty id or a time whose UTC year falls outside 0000-9999, fails
// with [transport.ErrInvalidConfig] before any request. The returned value is the
// concrete type ([*rm.Person] etc.) behind the [rm.Party] interface, decoded
// polymorphically by its `_type` discriminator; a `_type` naming a class
// other than t's does not decode.
//
// With no option, Get asks for canonical JSON and decodes it.
// [WithGetFormat] asks for canonical XML instead, and decodes the body as
// the class t names, whatever `Content-Type` the server labels it with; a
// root element that names another class, by its `xsi:type` or its name,
// does not decode.
// [WithRawBody] hands back the body as the server sent it.
//
// Wire: GET /demographic/{type}/{uid_based_id} [?version_at_time=...]. A 204
// (the party was deleted at the requested time) yields a nil Party and nil
// error; any other empty-body 2xx is surfaced as [transport.ErrInvalidShape].
// No version at the requested time is a 404, surfaced as
// [transport.ErrNotFound]. A body that does not decode returns a
// [*transport.DecodeError] beside the version metadata. These outcomes are
// the same in both formats.
func Get(ctx context.Context, c *transport.Client, t Type, ref openehrclient.Ref, opts ...GetOption) (rm.Party, *openehrclient.VersionMetadata, error) {
	if !t.valid() {
		return nil, nil, fmt.Errorf("demographic.Get: %w: invalid PARTY type %q", transport.ErrInvalidConfig, t)
	}
	if err := openehrclient.ValidateRef(ref); err != nil {
		return nil, nil, fmt.Errorf("demographic.Get: %w", err)
	}
	seg := ref.PathSegment()
	cfg, err := newGetConfig("demographic.Get", opts)
	if err != nil {
		return nil, nil, err
	}
	req := &transport.Request{
		Method: http.MethodGet,
		Path:   basePath(t) + "/" + seg,
		Route:  basePath(t) + "/{uid_based_id}",
		Accept: cfg.format.MediaType(),
	}
	if qk, qv := ref.Query(); qk != "" {
		req.Query = url.Values{qk: []string{qv}}
	}
	return getParty(ctx, c, req, t, cfg)
}

// writeConfig is the resolved option set for Create / Update. It embeds
// the shared [openehrclient.WriteConfig] (Prefer / audit details /
// lifecycle state) and adds the representation format.
type writeConfig struct {
	openehrclient.WriteConfig
	format Format
}

// WriteOption mutates the request shape for [Create] and [Update].
type WriteOption func(*writeConfig)

// WithPrefer overrides the response-shape preference. The default
// is [transport.PreferMinimal].
func WithPrefer(p transport.Prefer) WriteOption {
	return func(c *writeConfig) { c.Prefer = p }
}

// WithAuditDetails attaches the commit-time audit envelope via the
// `openehr-audit-details` header. Nil omits the header.
func WithAuditDetails(a *rm.AuditDetails) WriteOption {
	return func(c *writeConfig) { c.AuditDetails = a }
}

// WithLifecycleState sets the committed VERSION's lifecycle_state via the
// `openehr-version` header. Empty omits the header; an
// unrecognised code fails the write with [transport.ErrInvalidConfig].
func WithLifecycleState(s openehrclient.LifecycleState) WriteOption {
	return func(c *writeConfig) { c.LifecycleState = s }
}

// Create commits a new PARTY. The resource path is derived from party's
// concrete type ([*rm.Person] → /demographic/person, etc.). A nil party, or
// one holding a nil pointer, fails with [transport.ErrInvalidConfig] without
// issuing a request.
//
// Wire: POST /demographic/{type}. The response shape follows the Prefer
// option: minimal (default) returns no body, the new version's id
// is in the Location / ETag headers and the returned Party is nil;
// representation returns the created PARTY body; identifier returns the
// ITS-REST Identifier body, resolved into the metadata VersionUID.
// The returned Party is an interface: a boxed typed-nil pointer is not
// `== nil`. Use [openehrclient.HasResource] as the presence test
// ([rm.IsTypedNil] is the typed-nil absence check for callers already
// holding a registered RM pointer; it is false for a bare-nil
// interface). After 2xx + PreferRepresentation, an empty or
// undecodable body is a [*openehrclient.NoRepresentationError]
// carrying commit metadata.
//
// The body is canonical JSON unless [WithFormat] names canonical XML. The
// format sets how the body is written and how a returned party is decoded,
// and it sets `Content-Type` and, under PreferRepresentation, `Accept`. The
// `If-Match` header of [Update], `Prefer`, the audit and lifecycle headers,
// and the version metadata returned stay what they are for canonical JSON.
// A party the format's codec cannot serialise fails before any request with
// an [*EncodeError], which wraps the codec's error.
func Create(ctx context.Context, c *transport.Client, party rm.Party, opts ...WriteOption) (rm.Party, *openehrclient.VersionMetadata, error) {
	if party == nil || rm.IsTypedNil(party) {
		return nil, nil, fmt.Errorf("demographic.Create: %w: nil Party", transport.ErrInvalidConfig)
	}
	t, err := resourceType(party)
	if err != nil {
		return nil, nil, fmt.Errorf("demographic.Create: %w", err)
	}
	cfg := writeConfig{Prefer: transport.PreferMinimal}
	for _, o := range opts {
		if o != nil {
			o(&cfg)
		}
	}
	fw, err := cfg.resolveFormat("demographic.Create", party, t)
	if err != nil {
		return nil, nil, err
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
		Body:               fw.body,
		ContentType:        fw.contentType,
		Accept:             fw.accept,
		Prefer:             cfg.Prefer,
		AuditDetailsHeader: auditHeader,
		RMVersion:          verHeader,
	}
	return openehrclient.WriteResult(ctx, c, req, "demographic", fw.decode)
}

// Update modifies the PARTY family identified by voID, attaching `ifMatch` as
// the required If-Match header: the preceding version's id, typically the
// prior [openehrclient.VersionMetadata.VersionUID] / ETag.
//
// Wire: PUT /demographic/{type}/{voID} with If-Match. Errors:
// 412 → [transport.ErrPreconditionFailed]. Status→sentinel mapping is
// endpoint-agnostic, so a 409 (should the deployment emit one) still maps to
// [transport.ErrVersionConflict]. Forgetting ifMatch, or passing a nil
// party or one holding a nil pointer, returns [transport.ErrInvalidConfig]
// without issuing a request. Response shape
// matches [Create], and the format options apply as they do for [Create].
func Update(ctx context.Context, c *transport.Client, t Type, voID openehrclient.VersionedObjectID, ifMatch string, party rm.Party, opts ...WriteOption) (rm.Party, *openehrclient.VersionMetadata, error) {
	if !t.valid() {
		return nil, nil, fmt.Errorf("demographic.Update: %w: invalid PARTY type %q", transport.ErrInvalidConfig, t)
	}
	if voID == "" {
		return nil, nil, fmt.Errorf("demographic.Update: %w: empty VersionedObjectID", transport.ErrInvalidConfig)
	}
	if ifMatch == "" {
		return nil, nil, fmt.Errorf("demographic.Update: %w: empty If-Match", transport.ErrInvalidConfig)
	}
	if party == nil || rm.IsTypedNil(party) {
		return nil, nil, fmt.Errorf("demographic.Update: %w: nil Party", transport.ErrInvalidConfig)
	}
	// A type outside the five, such as a struct embedding a nil *rm.Person,
	// passes the nil check above and would panic in the encoder (idiom.md
	// § No panics); Create refuses it the same way.
	if _, err := resourceType(party); err != nil {
		return nil, nil, fmt.Errorf("demographic.Update: %w", err)
	}
	cfg := writeConfig{Prefer: transport.PreferMinimal}
	for _, o := range opts {
		if o != nil {
			o(&cfg)
		}
	}
	fw, err := cfg.resolveFormat("demographic.Update", party, t)
	if err != nil {
		return nil, nil, err
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
		Body:               fw.body,
		ContentType:        fw.contentType,
		Accept:             fw.accept,
		IfMatch:            ifMatch,
		Prefer:             cfg.Prefer,
		AuditDetailsHeader: auditHeader,
		RMVersion:          verHeader,
	}
	return openehrclient.WriteResult(ctx, c, req, "demographic", fw.decode)
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
// `openehr-audit-details` header on a logical delete. Nil omits it.
func WithDeleteAudit(a *rm.AuditDetails) DeleteOption {
	return func(c *deleteConfig) { c.auditDetails = a }
}

// Delete logically deletes the PARTY version addressed by versionUID,
// which must be the latest version (the preceding version of the
// deletion). The server responds 204 No Content on success.
//
// Wire: DELETE /demographic/{type}/{version_uid}. The openEHR operation
// takes no If-Match header: the version uid in the path is the
// precondition. Errors: 409 (the uid is no longer the latest version) →
// [transport.ErrVersionConflict], with the server's latest version uid in
// the metadata returned beside the error; 404 (unknown version) →
// [transport.ErrNotFound]. A 400 (for example, already deleted) has no
// sentinel and surfaces as a bare [transport.WireError].
func Delete(ctx context.Context, c *transport.Client, t Type, versionUID openehrclient.VersionUID, opts ...DeleteOption) (*openehrclient.VersionMetadata, error) {
	if !t.valid() {
		return nil, fmt.Errorf("demographic.Delete: %w: invalid PARTY type %q", transport.ErrInvalidConfig, t)
	}
	if versionUID == "" {
		return nil, fmt.Errorf("demographic.Delete: %w: empty VersionUID", transport.ErrInvalidConfig)
	}
	cfg := deleteConfig{}
	for _, o := range opts {
		if o != nil {
			o(&cfg)
		}
	}
	auditHeader, err := openehrclient.MarshalAuditDetails(cfg.auditDetails)
	if err != nil {
		return nil, fmt.Errorf("demographic.Delete: %w", err)
	}
	req := &transport.Request{
		Method:             http.MethodDelete,
		Path:               basePath(t) + "/" + string(versionUID),
		Route:              basePath(t) + "/{version_uid}",
		AuditDetailsHeader: auditHeader,
	}
	return openehrclient.DoDelete(ctx, c, req)
}

// getParty issues a read request and decodes the bare PARTY body in the
// format cfg names: canonical JSON polymorphically via the type registry
// (REQ-040), canonical XML as the class t names. A 204 (the party was
// deleted at the requested time) yields a nil Party; any other empty-body
// 2xx is a wire anomaly surfaced as [transport.ErrInvalidShape].
func getParty(ctx context.Context, c *transport.Client, req *transport.Request, t Type, cfg getConfig) (rm.Party, *openehrclient.VersionMetadata, error) {
	resp, meta, err := cfg.do(ctx, c, req)
	if err != nil {
		return nil, meta, err
	}
	if transport.IsNoRepresentationBody(resp.Body) {
		// Only a 204 legitimately carries no version body (the party was
		// deleted at the requested time; no version at that time is a 404).
		// Any other empty-body 2xx is a wire anomaly — surface it as
		// ErrInvalidShape rather than masquerading as "deleted". (The
		// concrete read leaves route through transport.Decode, which rejects
		// every empty body; this leaf decodes rm.Party polymorphically so it
		// can't, and instead applies the same strictness while carving out
		// the legitimate 204.)
		if resp.StatusCode == http.StatusNoContent {
			return nil, meta, nil
		}
		return nil, meta, fmt.Errorf("demographic: %w: %d response with empty or null body", transport.ErrInvalidShape, resp.StatusCode)
	}
	party, err := cfg.format.decodeParty(resp.Body, t)
	if err != nil {
		return nil, meta, fmt.Errorf("demographic: PARTY body: %w", &transport.DecodeError{
			Method: req.Method, Route: req.Route, Body: resp.Body, Inner: err,
		})
	}
	return party, meta, nil
}

// Repository mirrors the package-level functions as a dependency-injection
// seam.
type Repository interface {
	Get(ctx context.Context, t Type, ref openehrclient.Ref, opts ...GetOption) (rm.Party, *openehrclient.VersionMetadata, error)
	Create(ctx context.Context, party rm.Party, opts ...WriteOption) (rm.Party, *openehrclient.VersionMetadata, error)
	Update(ctx context.Context, t Type, voID openehrclient.VersionedObjectID, ifMatch string, party rm.Party, opts ...WriteOption) (rm.Party, *openehrclient.VersionMetadata, error)
	Delete(ctx context.Context, t Type, versionUID openehrclient.VersionUID, opts ...DeleteOption) (*openehrclient.VersionMetadata, error)

	// Versioned-resource reads.
	GetVersionedParty(ctx context.Context, voUID openehrclient.VersionedObjectID, opts ...GetOption) (*rm.VersionedParty, *openehrclient.VersionMetadata, error)
	GetRevisionHistory(ctx context.Context, voUID openehrclient.VersionedObjectID, opts ...GetOption) (*rm.RevisionHistory, *openehrclient.VersionMetadata, error)
	GetVersion(ctx context.Context, voUID openehrclient.VersionedObjectID, opts ...GetOption) (*PartyVersion, *openehrclient.VersionMetadata, error)
	GetVersionAtTime(ctx context.Context, voUID openehrclient.VersionedObjectID, t time.Time, opts ...GetOption) (*PartyVersion, *openehrclient.VersionMetadata, error)
	GetVersionByID(ctx context.Context, voUID openehrclient.VersionedObjectID, versionUID openehrclient.VersionUID, opts ...GetOption) (*PartyVersion, *openehrclient.VersionMetadata, error)

	// ITEM_TAG routes.
	GetTags(ctx context.Context, t Type, target openehrclient.Ref) ([]openehrclient.StoredItemTag, error)
	SetTags(ctx context.Context, t Type, target openehrclient.Ref, tags []openehrclient.ItemTag, opts ...SetTagsOption) ([]openehrclient.StoredItemTag, error)
	DeleteTag(ctx context.Context, t Type, target openehrclient.Ref, key string) error
	ListTags(ctx context.Context, opts ...ListTagsOption) ([]openehrclient.StoredItemTag, error)
}

// NewRepository binds c to a Repository.
func NewRepository(c *transport.Client) Repository { return &repository{c: c} }

type repository struct{ c *transport.Client }

func (r *repository) Get(ctx context.Context, t Type, ref openehrclient.Ref, opts ...GetOption) (rm.Party, *openehrclient.VersionMetadata, error) {
	return Get(ctx, r.c, t, ref, opts...)
}

func (r *repository) Create(ctx context.Context, party rm.Party, opts ...WriteOption) (rm.Party, *openehrclient.VersionMetadata, error) {
	return Create(ctx, r.c, party, opts...)
}

func (r *repository) Update(ctx context.Context, t Type, voID openehrclient.VersionedObjectID, ifMatch string, party rm.Party, opts ...WriteOption) (rm.Party, *openehrclient.VersionMetadata, error) {
	return Update(ctx, r.c, t, voID, ifMatch, party, opts...)
}

func (r *repository) Delete(ctx context.Context, t Type, versionUID openehrclient.VersionUID, opts ...DeleteOption) (*openehrclient.VersionMetadata, error) {
	return Delete(ctx, r.c, t, versionUID, opts...)
}
