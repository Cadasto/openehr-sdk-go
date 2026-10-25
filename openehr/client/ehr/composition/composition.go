package composition

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"

	openehrclient "github.com/cadasto/openehr-sdk-go/openehr/client/ehr"
	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/canjson"
	"github.com/cadasto/openehr-sdk-go/openehr/serialize/simplified"
	"github.com/cadasto/openehr-sdk-go/openehr/template/webtemplate"
	"github.com/cadasto/openehr-sdk-go/transport"
)

const routeTemplate = "/ehr/{ehr_id}/composition/{versioned_object_or_version_uid}"

// ErrDeletedAtTime signals that the composition existed but had been
// (logically) deleted at the requested version_at_time: the server
// answered 204 No Content per the ITS-REST composition_get contract
// (response 204_deleted_at_time).
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
// version_at_time) returns a nil Composition with [ErrDeletedAtTime], a
// typed success signal, distinct from transport.ErrInvalidShape.
//
// With no option, Get asks for canonical JSON and decodes it.
// [WithGetFormat] asks for canonical XML, FLAT or STRUCTURED instead, and
// decodes the body in that format whatever `Content-Type` the server labels
// it with. A FLAT or STRUCTURED body is decoded only when a Web Template is
// given; without one, Get returns a nil Composition, the version metadata
// and a nil error. [WithRawBody] hands back the body as the server sent it.
//
// A body that does not decode returns a [*transport.DecodeError] beside the
// version metadata. An empty or null body returns [transport.ErrInvalidShape]
// in every format.
func Get(ctx context.Context, c *transport.Client, ehrID openehrclient.EHRID, ref openehrclient.Ref, opts ...GetOption) (*rm.Composition, *openehrclient.VersionMetadata, error) {
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
	var cfg getConfig
	for _, o := range opts {
		if o != nil {
			o(&cfg)
		}
	}
	if err := cfg.check(); err != nil {
		return nil, nil, err
	}
	req := &transport.Request{
		Method: http.MethodGet,
		Path:   "/ehr/" + string(ehrID) + "/composition/" + seg,
		Route:  routeTemplate,
		Query:  cfg.query(ref),
		Accept: cfg.accept(),
	}
	resp, err := c.Do(ctx, req)
	if err != nil {
		var meta *transport.Metadata
		if resp != nil {
			meta = resp.Metadata
		}
		return nil, openehrclient.NewVersionMetadata(meta), err
	}
	meta := openehrclient.NewVersionMetadata(resp.Metadata)
	if resp.StatusCode == http.StatusNoContent {
		return nil, meta, ErrDeletedAtTime
	}
	if cfg.raw != nil && resp.StatusCode == http.StatusOK {
		*cfg.raw = Raw{ContentType: resp.Header.Get("Content-Type"), Body: resp.Body}
	}
	if transport.IsNoRepresentationBody(resp.Body) {
		return nil, meta, fmt.Errorf("composition.Get: %w: response body is empty or null", transport.ErrInvalidShape)
	}
	if cfg.format.isSimplified() && cfg.webTemplate == nil {
		return nil, meta, nil
	}
	out, err := cfg.format.unmarshal(resp.Body, cfg.webTemplate, cfg.decodeOptions)
	if err != nil {
		return nil, meta, fmt.Errorf("composition.Get: %w", &transport.DecodeError{
			Method: req.Method, Route: req.Route, Body: resp.Body, Inner: err,
		})
	}
	return out, meta, nil
}

// getConfig is the resolved option set for [Get].
type getConfig struct {
	format        Format
	webTemplate   *webtemplate.WebTemplate
	decodeOptions []simplified.Option
	formatQuery   bool
	raw           *Raw
	rawGiven      bool // WithRawBody was given, so a nil raw is refused
}

// GetOption shapes the request [Get] sends and how it decodes the response.
type GetOption func(*getConfig)

// WithGetFormat sets the representation the composition is read in. The
// default is [FormatJSON]. When the option is given more than once, the last
// one wins, decode options included.
//
// Get sends f's media type as `Accept` and decodes a 200 body in f, whatever
// `Content-Type` the server labels it with. A FLAT or STRUCTURED read that
// also has [WithGetFormatQuery] sends `Accept: application/json` instead, and
// still decodes the body in f. Canonical XML needs no Web Template. FLAT
// and STRUCTURED are decoded against wt. When wt is nil, Get does not decode
// a FLAT or STRUCTURED body: it returns a nil composition, the version
// metadata and a nil error, and [WithRawBody] can hand back the body.
// Canonical JSON and XML ignore wt.
//
// opts are the simplified codec's decode options, such as
// [simplified.WithTemplate], [simplified.WithStrict] and
// [simplified.WithUnmatched]. Get hands them, unchanged and in order, to the
// decode of a FLAT or STRUCTURED body, and hands it none when opts is empty.
// Canonical JSON and XML ignore them. Without [simplified.WithTemplate] the
// decoded composition lacks the name of every node below the root and the
// reference model defaults the compiled template completes, so a caller who
// needs a complete composition passes the compiled template. WithGetFormat
// keeps its own copy of opts.
//
// Get fails with [transport.ErrInvalidConfig], before any request, when f is
// not one of the four formats.
func WithGetFormat(f Format, wt *webtemplate.WebTemplate, opts ...simplified.Option) GetOption {
	opts = slices.Clone(opts)
	return func(c *getConfig) {
		c.format = f
		c.webTemplate = wt
		c.decodeOptions = opts
	}
}

// WithGetFormatQuery makes a FLAT or STRUCTURED read name its format with a
// query parameter rather than with a media type. It is a deployment
// extension outside ITS-REST 1.1.0, for servers that select the simplified
// format from a `format` query parameter.
//
// With it, a FLAT or STRUCTURED read sends `format=FLAT` or
// `format=STRUCTURED`, beside the `version_at_time` parameter the Ref may
// carry, and `Accept: application/json`. The body is still decoded in the
// format [WithGetFormat] names. The option is off by default and adds
// nothing to a canonical JSON or XML read.
func WithGetFormatQuery() GetOption {
	return func(c *getConfig) { c.formatQuery = true }
}

// WithRawBody makes Get copy the response's `Content-Type` value and its
// body bytes into dst when the server answers 200 and the body was read,
// before any decode. Get fills dst even when it then fails: with
// [transport.ErrInvalidShape] for an empty or null body, or with a
// [*transport.DecodeError] for a body that does not decode. Every other
// outcome leaves dst as it was: a status other than 200, such as a 204 or an
// error status, a failure before any response arrives, or a body that could
// not be read. It works with every format, the default included.
//
// A nil dst fails Get with [transport.ErrInvalidConfig] before any request.
func WithRawBody(dst *Raw) GetOption {
	return func(c *getConfig) {
		c.raw = dst
		c.rawGiven = true
	}
}

// Raw is what the server sent for a composition read, as received. Get
// fills it when [WithRawBody] is given.
type Raw struct {
	// ContentType is the response's `Content-Type` header value, parameters
	// included. It is empty when the server sent none.
	ContentType string
	// Body is the response body. It is the same slice a
	// [*transport.DecodeError] from the same call holds, not a copy. For a
	// clinical composition it is patient data.
	Body []byte
}

// check refuses, before any request, the option values Get cannot send: a
// format outside the four, and a nil WithRawBody destination.
func (c *getConfig) check() error {
	if !c.format.valid() {
		return fmt.Errorf("composition.Get: %w: unknown composition format %s", transport.ErrInvalidConfig, c.format)
	}
	if c.rawGiven && c.raw == nil {
		return fmt.Errorf("composition.Get: %w: nil WithRawBody destination", transport.ErrInvalidConfig)
	}
	return nil
}

// accept returns the media type a read asks for: the format's own, or
// application/json for FLAT and STRUCTURED under WithGetFormatQuery.
func (c *getConfig) accept() string {
	if c.formatQuery && c.format.isSimplified() {
		return mediaTypeJSON
	}
	return c.format.MediaType()
}

// query returns a read's query parameters: the one ref may carry and, under
// WithGetFormatQuery, the format parameter of FLAT and STRUCTURED. It is nil
// when there are none, as it always was without options.
func (c *getConfig) query(ref openehrclient.Ref) url.Values {
	var q url.Values
	if k, v := ref.Query(); k != "" {
		q = url.Values{k: {v}}
	}
	if c.formatQuery && c.format.isSimplified() {
		if q == nil {
			q = url.Values{}
		}
		q.Set("format", c.format.String())
	}
	return q
}

// writeConfig is the resolved option set for Save / Update. It embeds
// the shared [openehrclient.WriteConfig] (Prefer / audit details /
// lifecycle state) and adds composition's own extra options — template
// id, item tags and the representation format — that the other
// versioned-write leaves lack.
type writeConfig struct {
	openehrclient.WriteConfig
	templateID      string
	objectItemTags  []openehrclient.ItemTag
	versionItemTags []openehrclient.ItemTag
	format          Format
	webTemplate     *webtemplate.WebTemplate
	decodeOptions   []simplified.Option
	formatQuery     bool
}

// WriteOption mutates the request shape for [Save] and [Update].
type WriteOption func(*writeConfig)

// WithPrefer overrides the response-shape preference. The
// default is [transport.PreferMinimal], the ITS-REST default.
func WithPrefer(p transport.Prefer) WriteOption {
	return func(c *writeConfig) { c.Prefer = p }
}

// WithAuditDetails attaches the commit-time audit envelope via the
// `openehr-audit-details` header. Nil omits the header.
func WithAuditDetails(a *rm.AuditDetails) WriteOption {
	return func(c *writeConfig) { c.AuditDetails = a }
}

// WithTemplateID sets the `openehr-template-id` header so
// the deployment can validate the payload against the declared
// template. On a canonical JSON or XML write, empty omits the header. A
// FLAT or STRUCTURED write always sends the header, and takes the Web
// Template's id when this one is empty; see [WithFormat].
func WithTemplateID(id string) WriteOption {
	return func(c *writeConfig) { c.templateID = id }
}

// WithFormat sets the representation the composition is written in, and the
// one a returned composition is read in. The default is [FormatJSON]. When
// the option is given more than once, the last one wins, decode options
// included.
//
// The request body is serialised in f and sent with f's media type as
// `Content-Type`. Under [transport.PreferRepresentation] the write asks for
// f's media type in `Accept` and decodes the response body in f; under any
// other preference it asks for application/json, the media type of the
// identifier body the server may return. A FLAT or STRUCTURED write that
// also has [WithFormatQuery] sends application/json as both `Content-Type`
// and `Accept` instead, and still writes and reads the body in f.
//
// FLAT and STRUCTURED are written and read against wt, which must then be
// non-nil. They also always send the `openehr-template-id` header, holding
// the id [WithTemplateID] names or, when that is absent or empty, wt's
// template id. Canonical JSON and XML ignore wt.
//
// opts are the simplified codec's decode options, such as
// [simplified.WithTemplate], [simplified.WithStrict] and
// [simplified.WithUnmatched]. The write hands them, unchanged and in order,
// to the decode of a FLAT or STRUCTURED representation, and hands it none
// when opts is empty; they play no part in writing the body. Canonical JSON
// and XML ignore them. Without [simplified.WithTemplate] the returned
// composition lacks the name of every node below the root and the reference
// model defaults the compiled template completes, so a caller who needs a
// complete composition passes the compiled template. WithFormat keeps its
// own copy of opts.
//
// The write fails with [transport.ErrInvalidConfig], before any request,
// when f is not one of the four formats, when FLAT or STRUCTURED gets a nil
// wt, and when the [WithTemplateID] id and wt's template id are both empty
// or name different templates. A composition the codec cannot serialise
// also fails before any request.
func WithFormat(f Format, wt *webtemplate.WebTemplate, opts ...simplified.Option) WriteOption {
	opts = slices.Clone(opts)
	return func(c *writeConfig) {
		c.format = f
		c.webTemplate = wt
		c.decodeOptions = opts
	}
}

// WithFormatQuery makes a FLAT or STRUCTURED write name its format with
// query parameters rather than with a media type. It is a deployment
// extension outside ITS-REST 1.1.0, for servers that select the simplified
// format from a `format` query parameter and read the template id from a
// `templateId` query parameter.
//
// With it, a FLAT or STRUCTURED write sends `format=FLAT` or
// `format=STRUCTURED` and `templateId` holding the template id, with
// `Content-Type` and `Accept` both application/json, whatever the preference.
// The body is still written and read in the format [WithFormat] names, and
// the `openehr-template-id` header is still sent. The option is off by
// default and adds nothing to a canonical JSON or XML write.
func WithFormatQuery() WriteOption {
	return func(c *writeConfig) { c.formatQuery = true }
}

// WithLifecycleState sets the committed VERSION's lifecycle_state via the
// `openehr-version` header, as an openEHR version-lifecycle-state
// code (e.g. [openehrclient.LifecycleStateComplete]). The empty value omits
// the header (server default); an unrecognised code fails the write with
// [transport.ErrInvalidConfig].
func WithLifecycleState(s openehrclient.LifecycleState) WriteOption {
	return func(c *writeConfig) { c.LifecycleState = s }
}

// WithObjectItemTags sets the openehr-item-tag header.
func WithObjectItemTags(tags []openehrclient.ItemTag) WriteOption {
	return func(c *writeConfig) { c.objectItemTags = tags }
}

// WithVersionItemTags sets the openehr-version-item-tag header.
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
// `openehr-audit-details` header on a delete. Nil omits.
func WithDeleteAudit(a *rm.AuditDetails) DeleteOption {
	return func(c *deleteConfig) { c.auditDetails = a }
}

// Save creates a new Composition under ehrID.
//
// Wire: POST /ehr/{ehr_id}/composition.
//
// The response shape follows the Prefer option per the
// ITS-REST OpenAPI `201_COMPOSITION` response:
//   - PreferMinimal (default): server returns the new version's
//     identifier in the `Location` header; the returned
//     `*rm.Composition` is nil and only the metadata is populated
//     (ETag + parsed VersionUID).
//   - PreferRepresentation: server returns the bare `COMPOSITION`
//     body (not `ORIGINAL_VERSION<COMPOSITION>`) which is decoded
//     into the returned value. The audit / lifecycle / preceding-
//     version fields that `ORIGINAL_VERSION` carries are not in the
//     POST/PUT body; they are available via the version
//     metadata (`ETag` → `VersionUID`) or via a follow-up
//     `GET /versioned_composition/{vo_uid}/version/{version_uid}`
//     which is the canonical home for the `ORIGINAL_VERSION` envelope.
//   - PreferIdentifier: server returns the ITS-REST `Identifier` body
//     (`{"uid": …}`); the returned `*rm.Composition` is nil and the
//     identifier is resolved into the metadata `VersionUID` (the
//     `Location` header stays canonical when present).
//
// The zero resource on a successful minimal/identifier write is a nil
// pointer, so `== nil` is a correct test for this concrete return;
// [openehrclient.HasResource] is the uniform presence test across write
// leaves, including interface returns. After 2xx +
// PreferRepresentation, an empty or undecodable body is a
// [*openehrclient.NoRepresentationError] carrying commit metadata.
//
// The body is canonical JSON unless [WithFormat] names canonical XML,
// FLAT or STRUCTURED. The format sets how the body is written and how a
// returned composition is decoded. It also sets `Content-Type` and, under
// PreferRepresentation, `Accept`. A FLAT or STRUCTURED write with
// [WithFormatQuery] is the exception: it sends application/json in both
// headers, whatever the preference, and names the format and the template id
// in query parameters. The `If-Match` header of [Update], `Prefer`, the
// audit, lifecycle and item-tag headers, and the version metadata returned
// stay what they are for canonical JSON.
//
// Audit details and the template id flow via the `openehr-*` header
// family. A FLAT or STRUCTURED write always sends the template id, taken
// from [WithTemplateID] or from the Web Template given to [WithFormat].
func Save(ctx context.Context, c *transport.Client, ehrID openehrclient.EHRID, comp *rm.Composition, opts ...WriteOption) (*rm.Composition, *openehrclient.VersionMetadata, error) {
	if ehrID == "" {
		return nil, nil, fmt.Errorf("composition.Save: %w: empty EHRID", transport.ErrInvalidConfig)
	}
	if comp == nil {
		return nil, nil, fmt.Errorf("composition.Save: %w: nil Composition", transport.ErrInvalidConfig)
	}
	cfg := writeConfig{Prefer: transport.PreferMinimal}
	for _, o := range opts {
		if o != nil {
			o(&cfg)
		}
	}
	fw, err := cfg.resolveFormat("composition.Save", comp)
	if err != nil {
		return nil, nil, err
	}
	auditHeader, err := cfg.ResolveAuditHeader("composition.Save")
	if err != nil {
		return nil, nil, err
	}
	objectTags, versionTags, err := marshalItemTagHeaders(cfg.objectItemTags, cfg.versionItemTags)
	if err != nil {
		return nil, nil, fmt.Errorf("composition.Save: %w", err)
	}
	verHeader, err := cfg.ResolveLifecycleHeader("composition.Save")
	if err != nil {
		return nil, nil, err
	}
	req := &transport.Request{
		Method:             http.MethodPost,
		Path:               "/ehr/" + string(ehrID) + "/composition",
		Route:              "/ehr/{ehr_id}/composition",
		Query:              fw.query,
		Body:               fw.body,
		ContentType:        fw.contentType,
		Accept:             fw.accept,
		Prefer:             cfg.Prefer,
		AuditDetailsHeader: auditHeader,
		RMVersion:          verHeader,
		TemplateID:         fw.templateID,
		ItemTag:            objectTags,
		VersionItemTag:     versionTags,
	}
	return openehrclient.WriteResult(ctx, c, req, "composition", fw.decode)
}

// Update modifies the Composition family identified by voID, attaching
// `ifMatch` as the required `If-Match` header.
//
// Wire: PUT /ehr/{ehr_id}/composition/{voID} with If-Match. Errors:
// 412 (the If-Match is not the latest version) →
// [transport.ErrPreconditionFailed], with the server's latest version
// uid in the metadata returned beside the error; 404 (unknown EHR or
// Composition) → [transport.ErrNotFound]; 422 (template / semantic
// validation failure) → [transport.ErrUnprocessable]. A 400 has no
// sentinel and surfaces as a bare [transport.WireError]. (409 →
// [transport.ErrVersionConflict] and 428 →
// [transport.ErrPreconditionRequired] are defensive mappings only: the
// openEHR contract signals a stale If-Match as 412 and a missing one as
// 400.) Forgetting ifMatch returns [transport.ErrInvalidConfig] without
// issuing a request.
//
// Response shape matches [Save]: bare `*rm.Composition` per the
// ITS-REST OpenAPI `200_COMPOSITION_updated` response. The format
// options apply as they do for [Save].
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
	cfg := writeConfig{Prefer: transport.PreferMinimal}
	for _, o := range opts {
		if o != nil {
			o(&cfg)
		}
	}
	fw, err := cfg.resolveFormat("composition.Update", comp)
	if err != nil {
		return nil, nil, err
	}
	auditHeader, err := cfg.ResolveAuditHeader("composition.Update")
	if err != nil {
		return nil, nil, err
	}
	objectTags, versionTags, err := marshalItemTagHeaders(cfg.objectItemTags, cfg.versionItemTags)
	if err != nil {
		return nil, nil, fmt.Errorf("composition.Update: %w", err)
	}
	verHeader, err := cfg.ResolveLifecycleHeader("composition.Update")
	if err != nil {
		return nil, nil, err
	}
	req := &transport.Request{
		Method:             http.MethodPut,
		Path:               "/ehr/" + string(ehrID) + "/composition/" + string(voID),
		Route:              "/ehr/{ehr_id}/composition/{versioned_object_id}",
		Query:              fw.query,
		Body:               fw.body,
		ContentType:        fw.contentType,
		Accept:             fw.accept,
		IfMatch:            ifMatch,
		Prefer:             cfg.Prefer,
		AuditDetailsHeader: auditHeader,
		RMVersion:          verHeader,
		TemplateID:         fw.templateID,
		ItemTag:            objectTags,
		VersionItemTag:     versionTags,
	}
	return openehrclient.WriteResult(ctx, c, req, "composition", fw.decode)
}

// Delete logically deletes the Composition version addressed by
// versionUID, which must be the latest version (the preceding version of
// the deletion). The server typically responds 204 No Content.
//
// Wire: DELETE /ehr/{ehr_id}/composition/{version_uid}. The openEHR
// operation takes no If-Match header: the version uid in the path is the
// precondition. Errors: 409 (the uid is no longer the latest version) →
// [transport.ErrVersionConflict], with the server's latest version uid in
// the metadata returned beside the error; 404 (unknown EHR or version) →
// [transport.ErrNotFound]. A 400 (for example, already deleted) has no
// sentinel and surfaces as a bare [transport.WireError].
func Delete(ctx context.Context, c *transport.Client, ehrID openehrclient.EHRID, versionUID openehrclient.VersionUID, opts ...DeleteOption) (*openehrclient.VersionMetadata, error) {
	if ehrID == "" {
		return nil, fmt.Errorf("composition.Delete: %w: empty EHRID", transport.ErrInvalidConfig)
	}
	if versionUID == "" {
		return nil, fmt.Errorf("composition.Delete: %w: empty VersionUID", transport.ErrInvalidConfig)
	}
	cfg := deleteConfig{}
	for _, o := range opts {
		if o != nil {
			o(&cfg)
		}
	}
	auditHeader, err := openehrclient.MarshalAuditDetails(cfg.auditDetails)
	if err != nil {
		return nil, fmt.Errorf("composition.Delete: %w", err)
	}
	req := &transport.Request{
		Method:             http.MethodDelete,
		Path:               "/ehr/" + string(ehrID) + "/composition/" + string(versionUID),
		Route:              "/ehr/{ehr_id}/composition/{version_uid}",
		AuditDetailsHeader: auditHeader,
	}
	return openehrclient.DoDelete(ctx, c, req)
}

// decodeComposition decodes a Prefer=representation write response.
// Per ITS-REST OpenAPI `201_COMPOSITION` / `200_COMPOSITION_updated`
// (REQ-094), the response body is a bare `Composition` — not an
// `ORIGINAL_VERSION<COMPOSITION>` envelope. The full audit / lifecycle
// version envelope lives at `GET /versioned_composition/{vo_uid}/
// version/{version_uid}` (`UVersionOfComposition`) which the
// consumer can fetch when needed.
func decodeComposition(body []byte) (*rm.Composition, error) {
	var comp rm.Composition
	if err := canjson.Unmarshal(body, &comp); err != nil {
		return nil, fmt.Errorf("composition: decode Composition: %w", err)
	}
	return &comp, nil
}

// formatWrite is the part of a Save or Update request that the
// representation format decides (REQ-173).
type formatWrite struct {
	body        []byte
	contentType string
	accept      string
	templateID  string
	query       url.Values
	decode      func([]byte) (*rm.Composition, error)
}

// resolveFormat applies the format options to one write and serialises
// comp. Every refusal comes before any request (REQ-173): a format outside
// the four, FLAT or STRUCTURED with a nil Web Template, a template id the
// rule in simplifiedTemplateID cannot resolve, and a composition the codec
// cannot serialise. Canonical JSON with no format option yields the request
// every write sent before the formats existed.
func (c *writeConfig) resolveFormat(op string, comp *rm.Composition) (formatWrite, error) {
	f := c.format
	if !f.valid() {
		return formatWrite{}, fmt.Errorf("%s: %w: unknown composition format %s", op, transport.ErrInvalidConfig, f)
	}
	templateID := c.templateID
	if f.isSimplified() {
		if c.webTemplate == nil {
			return formatWrite{}, fmt.Errorf("%s: %w: %s needs a non-nil Web Template", op, transport.ErrInvalidConfig, f)
		}
		id, err := simplifiedTemplateID(c.templateID, c.webTemplate.TemplateID)
		if err != nil {
			return formatWrite{}, fmt.Errorf("%s: %w", op, err)
		}
		templateID = id
	}
	body, err := f.marshal(comp, c.webTemplate)
	if err != nil {
		return formatWrite{}, fmt.Errorf("%s: marshal body: %w", op, err)
	}
	fw := formatWrite{
		body:        body,
		contentType: f.MediaType(),
		accept:      mediaTypeJSON, // the Identifier body is JSON
		templateID:  templateID,
		decode:      f.decoder(c.webTemplate, c.decodeOptions),
	}
	if c.Prefer == transport.PreferRepresentation {
		fw.accept = f.MediaType()
	}
	if c.formatQuery && f.isSimplified() {
		fw.contentType = mediaTypeJSON
		fw.accept = mediaTypeJSON
		fw.query = url.Values{"format": {f.String()}, "templateId": {templateID}}
	}
	return fw, nil
}

// simplifiedTemplateID resolves the template id a FLAT or STRUCTURED write
// sends (REQ-173): the WithTemplateID id, or the Web Template's when that
// is empty. Two non-empty ids that differ, and two empty ids, are refused.
// The error names neither id.
func simplifiedTemplateID(option, webTemplate string) (string, error) {
	switch {
	case option != "" && webTemplate != "" && option != webTemplate:
		return "", fmt.Errorf("%w: the WithTemplateID id differs from the Web Template's template id", transport.ErrInvalidConfig)
	case option == "" && webTemplate == "":
		return "", fmt.Errorf("%w: no template id: WithTemplateID and the Web Template's template id are both empty", transport.ErrInvalidConfig)
	}
	return cmp.Or(option, webTemplate), nil
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
	Get(ctx context.Context, ehrID openehrclient.EHRID, ref openehrclient.Ref, opts ...GetOption) (*rm.Composition, *openehrclient.VersionMetadata, error)
	Save(ctx context.Context, ehrID openehrclient.EHRID, comp *rm.Composition, opts ...WriteOption) (*rm.Composition, *openehrclient.VersionMetadata, error)
	Update(ctx context.Context, ehrID openehrclient.EHRID, voID openehrclient.VersionedObjectID, ifMatch string, comp *rm.Composition, opts ...WriteOption) (*rm.Composition, *openehrclient.VersionMetadata, error)
	Delete(ctx context.Context, ehrID openehrclient.EHRID, versionUID openehrclient.VersionUID, opts ...DeleteOption) (*openehrclient.VersionMetadata, error)
}

// NewRepository binds c to a Repository.
func NewRepository(c *transport.Client) Repository { return &repository{c: c} }

type repository struct{ c *transport.Client }

func (r *repository) Get(ctx context.Context, ehrID openehrclient.EHRID, ref openehrclient.Ref, opts ...GetOption) (*rm.Composition, *openehrclient.VersionMetadata, error) {
	return Get(ctx, r.c, ehrID, ref, opts...)
}

func (r *repository) Save(ctx context.Context, ehrID openehrclient.EHRID, comp *rm.Composition, opts ...WriteOption) (*rm.Composition, *openehrclient.VersionMetadata, error) {
	return Save(ctx, r.c, ehrID, comp, opts...)
}

func (r *repository) Update(ctx context.Context, ehrID openehrclient.EHRID, voID openehrclient.VersionedObjectID, ifMatch string, comp *rm.Composition, opts ...WriteOption) (*rm.Composition, *openehrclient.VersionMetadata, error) {
	return Update(ctx, r.c, ehrID, voID, ifMatch, comp, opts...)
}

func (r *repository) Delete(ctx context.Context, ehrID openehrclient.EHRID, versionUID openehrclient.VersionUID, opts ...DeleteOption) (*openehrclient.VersionMetadata, error) {
	return Delete(ctx, r.c, ehrID, versionUID, opts...)
}
