package definition

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"time"

	"github.com/cadasto/openehr-sdk-go/openehr/rm"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// TemplateFormat selects between the ADL 1.4 Operational-Template
// shape and the ADL 2 source-form shape on the wire. Only
// [FormatADL14] is supported in v1; ADL 2 follows in a later commit
// per docs/plans/2026-05-15-rest-api-client.md.
//
// The string value is the URL-path segment under
// `/definition/template/...` and the value the deployment expects.
type TemplateFormat string

const (
	// FormatADL14 is the openEHR ADL 1.4 / OPT format. Wire payload
	// is XML, Content-Type `application/xml`.
	FormatADL14 TemplateFormat = "adl1.4"
)

// PathSegment returns the URL-path segment for this format.
func (f TemplateFormat) PathSegment() string { return string(f) }

// ContentType returns the HTTP Content-Type for the upload body of
// this format.
func (f TemplateFormat) ContentType() string {
	switch f {
	case FormatADL14:
		return "application/xml"
	default:
		return "application/octet-stream"
	}
}

// IsValid reports whether f is a supported format.
func (f TemplateFormat) IsValid() bool {
	switch f {
	case FormatADL14:
		return true
	default:
		return false
	}
}

// TemplateMetadata is the typed listing/upload response shape per the
// openEHR REST Definition API. Documented fields are typed and
// deployment-specific fields are preserved verbatim in Extras for
// forward-compatibility — mirrors the pattern in
// [github.com/cadasto/openehr-sdk-go/openehr/client/system.ServiceCapabilities].
type TemplateMetadata struct {
	// TemplateID is the deployment-assigned template identifier
	// (typically the OPT `template_id` element).
	TemplateID string `json:"template_id,omitempty"`
	// Concept is the human-readable concept name (e.g. "Body Weight").
	Concept string `json:"concept,omitempty"`
	// ArchetypeID is the root ARCHETYPE_ID the template specialises
	// (e.g. "openEHR-EHR-COMPOSITION.encounter.v1").
	ArchetypeID string `json:"archetype_id,omitempty"`
	// Version is the deployment-side version string (typically a
	// timestamp or a semver).
	Version string `json:"version,omitempty"`
	// CreatedOn is when the deployment first received this template. The
	// wire field is `created_timestamp` per the Definition API
	// TemplateMetadata schema.
	CreatedOn time.Time `json:"created_timestamp,omitzero"`
	// Description is an optional free-text description.
	Description string `json:"description,omitempty"`
	// Extras preserves deployment-specific fields not in the standard
	// metadata shape.
	Extras map[string]json.RawMessage `json:"-"`
}

var knownTemplateMetadataFields = map[string]struct{}{
	"template_id":       {},
	"concept":           {},
	"archetype_id":      {},
	"version":           {},
	"created_timestamp": {},
	"description":       {},
}

// templateTimestampLayouts are the time formats accepted for
// `created_timestamp`. The Definition API documents RFC3339, but some
// deployments emit a space-separated, timezone-less form
// ("2006-01-02 15:04:05"); decode tolerantly rather than failing the whole
// response (a strict time.Time decode would abort ListTemplates entirely).
var templateTimestampLayouts = []string{
	time.RFC3339Nano,
	time.RFC3339,
	"2006-01-02 15:04:05.999999999",
	"2006-01-02 15:04:05",
	"2006-01-02T15:04:05",
}

// parseTemplateTimestamp decodes the created_timestamp JSON value leniently.
// A JSON null or empty string yields the zero time (no error).
func parseTemplateTimestamp(raw json.RawMessage) (time.Time, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return time.Time{}, nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return time.Time{}, fmt.Errorf("created_timestamp: %w", err)
	}
	if s == "" {
		return time.Time{}, nil
	}
	for _, layout := range templateTimestampLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("created_timestamp: cannot parse %q as a known timestamp layout", s)
}

// UnmarshalJSON decodes both documented fields and Extras in one pass.
func (m *TemplateMetadata) UnmarshalJSON(data []byte) error {
	type alias TemplateMetadata
	// Shadow created_timestamp as a RawMessage so the strict time.Time
	// decoder never sees a non-RFC3339 deployment timestamp; parse it
	// leniently afterwards.
	aux := struct {
		*alias
		CreatedOn json.RawMessage `json:"created_timestamp,omitempty"`
	}{alias: (*alias)(m)}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	ts, err := parseTemplateTimestamp(aux.CreatedOn)
	if err != nil {
		return err
	}
	m.CreatedOn = ts

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	for k, v := range raw {
		if _, ok := knownTemplateMetadataFields[k]; ok {
			continue
		}
		if m.Extras == nil {
			m.Extras = map[string]json.RawMessage{}
		}
		m.Extras[k] = v
	}
	return nil
}

// MarshalJSON re-emits documented fields plus Extras (insertion order
// for known fields; map iteration order is non-deterministic on the
// Extras side, but the result decodes back to the same key set).
func (m TemplateMetadata) MarshalJSON() ([]byte, error) {
	type alias TemplateMetadata
	known, err := json.Marshal(alias(m))
	if err != nil {
		return nil, err
	}
	if len(m.Extras) == 0 {
		return known, nil
	}
	var merged map[string]json.RawMessage
	if err := json.Unmarshal(known, &merged); err != nil {
		return nil, err
	}
	if merged == nil {
		merged = map[string]json.RawMessage{}
	}
	maps.Copy(merged, m.Extras)
	return json.Marshal(merged)
}

// maxUploadBytes is the maximum number of bytes UploadTemplate will
// read from the body io.Reader before returning an "input too large"
// error. Default is 32 MiB. Unexported so tests in package definition
// can lower it temporarily via t.Cleanup.
var maxUploadBytes int64 = 32 << 20

// uploadConfig is the resolved option set for [UploadTemplate].
type uploadConfig struct {
	versionParam string
}

// UploadOption mutates [UploadTemplate]'s request shape.
type UploadOption func(*uploadConfig)

// WithUploadVersion sets a `version` query parameter on the upload
// request. Some deployments allow client-supplied versioning when the
// OPT itself does not carry one; most use the OPT's embedded version.
func WithUploadVersion(v string) UploadOption {
	return func(c *uploadConfig) { c.versionParam = v }
}

// UploadTemplate uploads the body (a serialised template in the given
// format) to the deployment. For [FormatADL14] the body MUST be an
// OPT XML document; the Content-Type header is set to
// `application/xml` automatically. ADL 2 source-form upload (Content-
// Type `text/plain`) is not yet implemented.
//
// Wire: POST /definition/template/{format}. The decoded
// [*TemplateMetadata] reflects the deployment's view of the freshly
// uploaded template.
func UploadTemplate(ctx context.Context, c *transport.Client, format TemplateFormat, body io.Reader, opts ...UploadOption) (*TemplateMetadata, *transport.Metadata, error) {
	if !format.IsValid() {
		return nil, nil, fmt.Errorf("definition.UploadTemplate: %w: format %q is not supported in v1 (ADL 2 deferred)", transport.ErrInvalidConfig, format)
	}
	if body == nil {
		return nil, nil, fmt.Errorf("definition.UploadTemplate: %w: nil body", transport.ErrInvalidConfig)
	}
	raw, err := io.ReadAll(io.LimitReader(body, maxUploadBytes+1))
	if err != nil {
		return nil, nil, fmt.Errorf("definition.UploadTemplate: read body: %w", err)
	}
	if int64(len(raw)) > maxUploadBytes {
		return nil, nil, fmt.Errorf("definition.UploadTemplate: %w: body exceeds %d bytes", transport.ErrInvalidConfig, maxUploadBytes)
	}
	if len(raw) == 0 {
		return nil, nil, fmt.Errorf("definition.UploadTemplate: %w: empty body", transport.ErrInvalidConfig)
	}
	cfg := uploadConfig{}
	for _, o := range opts {
		o(&cfg)
	}
	req := &transport.Request{
		Method:      http.MethodPost,
		Path:        "/definition/template/" + format.PathSegment(),
		Route:       "/definition/template/{format}",
		Body:        raw,
		ContentType: format.ContentType(),
		Accept:      "application/json",
	}
	if cfg.versionParam != "" {
		req.Query = url.Values{"version": []string{cfg.versionParam}}
	}
	resp, err := c.Do(ctx, req)
	if err != nil {
		if resp != nil {
			return nil, resp.Metadata, err
		}
		return nil, nil, err
	}
	if len(resp.Body) == 0 {
		// Some deployments return 204 with only headers. Surface a
		// minimal metadata constructed from the Location header so
		// the caller can still find the template.
		return &TemplateMetadata{TemplateID: extractLastPathSegment(resp.Metadata.Location)}, resp.Metadata, nil
	}
	var out TemplateMetadata
	if err := json.Unmarshal(resp.Body, &out); err != nil {
		return nil, resp.Metadata, fmt.Errorf("definition.UploadTemplate: decode metadata: %w", err)
	}
	return &out, resp.Metadata, nil
}

// GetTemplate fetches the raw OPT bytes for templateID under the
// given format. Returns the bytes verbatim — consumers are expected
// to parse the XML themselves (or pass it to a template parser
// downstream).
//
// Wire: GET /definition/template/{format}/{template_id}. Accept
// header is set to `application/xml` to request the OPT body (some
// deployments also serve `application/openehr.wt+json` Web Template
// shape under the same path; consumers requiring that should call
// [transport.Client.Do] directly with a custom Accept).
func GetTemplate(ctx context.Context, c *transport.Client, templateID string, format TemplateFormat) ([]byte, *transport.Metadata, error) {
	if !format.IsValid() {
		return nil, nil, fmt.Errorf("definition.GetTemplate: %w: format %q is not supported in v1", transport.ErrInvalidConfig, format)
	}
	if templateID == "" {
		return nil, nil, fmt.Errorf("definition.GetTemplate: %w: empty templateID", transport.ErrInvalidConfig)
	}
	req := &transport.Request{
		Method: http.MethodGet,
		Path:   "/definition/template/" + format.PathSegment() + "/" + templateID,
		Route:  "/definition/template/{format}/{template_id}",
		Accept: "application/xml",
	}
	resp, err := c.Do(ctx, req)
	if err != nil {
		if resp != nil {
			return nil, resp.Metadata, err
		}
		return nil, nil, err
	}
	return resp.Body, resp.Metadata, nil
}

// ListTemplates returns the deployment's catalog of templates for
// the given format.
//
// Wire: GET /definition/template/{format}.
func ListTemplates(ctx context.Context, c *transport.Client, format TemplateFormat) ([]TemplateMetadata, *transport.Metadata, error) {
	if !format.IsValid() {
		return nil, nil, fmt.Errorf("definition.ListTemplates: %w: format %q is not supported in v1", transport.ErrInvalidConfig, format)
	}
	req := &transport.Request{
		Method: http.MethodGet,
		Path:   "/definition/template/" + format.PathSegment(),
		Route:  "/definition/template/{format}",
		Accept: "application/json",
	}
	resp, err := c.Do(ctx, req)
	if err != nil {
		if resp != nil {
			return nil, resp.Metadata, err
		}
		return nil, nil, err
	}
	if len(resp.Body) == 0 {
		return nil, resp.Metadata, nil
	}
	var out []TemplateMetadata
	if err := json.Unmarshal(resp.Body, &out); err != nil {
		return nil, resp.Metadata, fmt.Errorf("definition.ListTemplates: decode: %w", err)
	}
	return out, resp.Metadata, nil
}

// DeleteTemplate removes a template by id where the deployment
// supports it (many production deployments disable template delete
// to preserve referential integrity for compositions already stored
// against the template). A `405 Method Not Allowed` or `403
// Forbidden` from the wire surfaces as a typed
// [transport.WireError]; consumers SHOULD treat delete as a
// best-effort operation guarded by deployment policy.
//
// Wire: DELETE /definition/template/{format}/{template_id}.
func DeleteTemplate(ctx context.Context, c *transport.Client, templateID string, format TemplateFormat) (*transport.Metadata, error) {
	if !format.IsValid() {
		return nil, fmt.Errorf("definition.DeleteTemplate: %w: format %q is not supported in v1", transport.ErrInvalidConfig, format)
	}
	if templateID == "" {
		return nil, fmt.Errorf("definition.DeleteTemplate: %w: empty templateID", transport.ErrInvalidConfig)
	}
	req := &transport.Request{
		Method: http.MethodDelete,
		Path:   "/definition/template/" + format.PathSegment() + "/" + templateID,
		Route:  "/definition/template/{format}/{template_id}",
	}
	resp, err := c.Do(ctx, req)
	if err != nil {
		if resp != nil {
			return resp.Metadata, err
		}
		return nil, err
	}
	return resp.Metadata, nil
}

// ExampleType selects the kind of example the deployment synthesises:
// ExampleTypeInput (ready to submit to the repository) or
// ExampleTypeOutput (as it would appear when retrieved). Maps to the
// `type` query parameter; when unset the spec default ("input") applies.
type ExampleType string

const (
	// ExampleTypeInput requests an example ready to be submitted.
	ExampleTypeInput ExampleType = "input"
	// ExampleTypeOutput requests an example as it appears on retrieval.
	ExampleTypeOutput ExampleType = "output"
)

// ExampleDetailLevel selects how complete the generated example is. Maps
// to the `detail_level` query parameter; when unset the spec default
// ("required") applies.
type ExampleDetailLevel string

const (
	// ExampleDetailRequired populates only required data points.
	ExampleDetailRequired ExampleDetailLevel = "required"
	// ExampleDetailMedium populates a medium level of detail.
	ExampleDetailMedium ExampleDetailLevel = "medium"
	// ExampleDetailComplete populates the most complete example.
	ExampleDetailComplete ExampleDetailLevel = "complete"
)

// IsValid reports whether t is one of the spec's `type` enum values.
func (t ExampleType) IsValid() bool {
	switch t {
	case ExampleTypeInput, ExampleTypeOutput:
		return true
	default:
		return false
	}
}

// IsValid reports whether l is one of the spec's `detail_level` enum values.
func (l ExampleDetailLevel) IsValid() bool {
	switch l {
	case ExampleDetailRequired, ExampleDetailMedium, ExampleDetailComplete:
		return true
	default:
		return false
	}
}

// exampleConfig is the resolved option set for [ExampleComposition].
type exampleConfig struct {
	exampleType ExampleType
	detailLevel ExampleDetailLevel
}

// ExampleOption mutates [ExampleComposition]'s request shape.
type ExampleOption func(*exampleConfig)

// WithExampleType sets the `type` query parameter (input or output).
// Omitted by default — the deployment applies the spec default "input".
func WithExampleType(t ExampleType) ExampleOption {
	return func(c *exampleConfig) { c.exampleType = t }
}

// WithExampleDetailLevel sets the `detail_level` query parameter
// (required, medium, or complete). Omitted by default — the deployment
// applies the spec default "required".
func WithExampleDetailLevel(l ExampleDetailLevel) ExampleOption {
	return func(c *exampleConfig) { c.detailLevel = l }
}

// ExampleComposition asks the deployment to synthesise an example
// COMPOSITION for templateID. The example is typically used by
// validators and UIs to bootstrap a payload against a known template.
//
// Wire: GET /definition/template/{format}/{template_id}/example with the
// optional `type` and `detail_level` query parameters (operationId
// definition_template_adl1.4_example_get —
// resources/its-rest/definition-validation.openapi.yaml line 225). Decodes
// the canonical-JSON response body into a [*rm.Composition]; flat /
// structured / XML negotiation is not reachable through this typed entry
// point (drop to transport.Client.Do for those).
func ExampleComposition(ctx context.Context, c *transport.Client, templateID string, format TemplateFormat, opts ...ExampleOption) (*rm.Composition, *transport.Metadata, error) {
	if !format.IsValid() {
		return nil, nil, fmt.Errorf("definition.ExampleComposition: %w: format %q is not supported in v1", transport.ErrInvalidConfig, format)
	}
	if templateID == "" {
		return nil, nil, fmt.Errorf("definition.ExampleComposition: %w: empty templateID", transport.ErrInvalidConfig)
	}
	cfg := exampleConfig{}
	for _, o := range opts {
		o(&cfg)
	}
	if cfg.exampleType != "" && !cfg.exampleType.IsValid() {
		return nil, nil, fmt.Errorf("definition.ExampleComposition: %w: invalid example type %q", transport.ErrInvalidConfig, cfg.exampleType)
	}
	if cfg.detailLevel != "" && !cfg.detailLevel.IsValid() {
		return nil, nil, fmt.Errorf("definition.ExampleComposition: %w: invalid detail_level %q", transport.ErrInvalidConfig, cfg.detailLevel)
	}
	req := &transport.Request{
		Method: http.MethodGet,
		Path:   "/definition/template/" + format.PathSegment() + "/" + templateID + "/example",
		Route:  "/definition/template/{format}/{template_id}/example",
		Accept: "application/json",
	}
	if cfg.exampleType != "" || cfg.detailLevel != "" {
		q := url.Values{}
		if cfg.exampleType != "" {
			q.Set("type", string(cfg.exampleType))
		}
		if cfg.detailLevel != "" {
			q.Set("detail_level", string(cfg.detailLevel))
		}
		req.Query = q
	}
	out, meta, err := transport.Decode[rm.Composition](ctx, c, req)
	return out, meta, err
}

// extractLastPathSegment returns the trailing segment of a URL path —
// used as a fallback to surface a template id when the deployment
// returns 204 from upload with only a Location header.
func extractLastPathSegment(p string) string {
	if p == "" {
		return ""
	}
	if u, err := url.Parse(p); err == nil && u.Path != "" {
		p = u.Path
	}
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '/' {
			return p[i+1:]
		}
	}
	return p
}

// Repository mirrors the package-level Definition functions for DI
// seams (REQ-023).
type Repository interface {
	UploadTemplate(ctx context.Context, format TemplateFormat, body io.Reader, opts ...UploadOption) (*TemplateMetadata, *transport.Metadata, error)
	GetTemplate(ctx context.Context, templateID string, format TemplateFormat) ([]byte, *transport.Metadata, error)
	ListTemplates(ctx context.Context, format TemplateFormat) ([]TemplateMetadata, *transport.Metadata, error)
	DeleteTemplate(ctx context.Context, templateID string, format TemplateFormat) (*transport.Metadata, error)
	ExampleComposition(ctx context.Context, templateID string, format TemplateFormat, opts ...ExampleOption) (*rm.Composition, *transport.Metadata, error)
	PutStoredQuery(ctx context.Context, qualifiedName, aqlText string, opts ...StoreOption) (*StoredQueryMetadata, *transport.Metadata, error)
	PutStoredQueryVersion(ctx context.Context, qualifiedName, version, aqlText string, opts ...StoreOption) (*StoredQueryMetadata, *transport.Metadata, error)
	GetStoredQuery(ctx context.Context, qualifiedName, version string) (*StoredQueryMetadata, *transport.Metadata, error)
	ListStoredQueries(ctx context.Context, namePattern string) ([]StoredQueryMetadata, *transport.Metadata, error)
	DeleteStoredQuery(ctx context.Context, qualifiedName, version string) (*transport.Metadata, error)
}

// NewRepository binds c to a Repository.
func NewRepository(c *transport.Client) Repository { return &repository{c: c} }

type repository struct{ c *transport.Client }

func (r *repository) UploadTemplate(ctx context.Context, format TemplateFormat, body io.Reader, opts ...UploadOption) (*TemplateMetadata, *transport.Metadata, error) {
	return UploadTemplate(ctx, r.c, format, body, opts...)
}

func (r *repository) GetTemplate(ctx context.Context, templateID string, format TemplateFormat) ([]byte, *transport.Metadata, error) {
	return GetTemplate(ctx, r.c, templateID, format)
}

func (r *repository) ListTemplates(ctx context.Context, format TemplateFormat) ([]TemplateMetadata, *transport.Metadata, error) {
	return ListTemplates(ctx, r.c, format)
}

func (r *repository) DeleteTemplate(ctx context.Context, templateID string, format TemplateFormat) (*transport.Metadata, error) {
	return DeleteTemplate(ctx, r.c, templateID, format)
}

func (r *repository) ExampleComposition(ctx context.Context, templateID string, format TemplateFormat, opts ...ExampleOption) (*rm.Composition, *transport.Metadata, error) {
	return ExampleComposition(ctx, r.c, templateID, format, opts...)
}

func (r *repository) PutStoredQuery(ctx context.Context, qualifiedName, aqlText string, opts ...StoreOption) (*StoredQueryMetadata, *transport.Metadata, error) {
	return PutStoredQuery(ctx, r.c, qualifiedName, aqlText, opts...)
}

func (r *repository) PutStoredQueryVersion(ctx context.Context, qualifiedName, version, aqlText string, opts ...StoreOption) (*StoredQueryMetadata, *transport.Metadata, error) {
	return PutStoredQueryVersion(ctx, r.c, qualifiedName, version, aqlText, opts...)
}

func (r *repository) GetStoredQuery(ctx context.Context, qualifiedName, version string) (*StoredQueryMetadata, *transport.Metadata, error) {
	return GetStoredQuery(ctx, r.c, qualifiedName, version)
}

func (r *repository) ListStoredQueries(ctx context.Context, namePattern string) ([]StoredQueryMetadata, *transport.Metadata, error) {
	return ListStoredQueries(ctx, r.c, namePattern)
}

func (r *repository) DeleteStoredQuery(ctx context.Context, qualifiedName, version string) (*transport.Metadata, error) {
	return DeleteStoredQuery(ctx, r.c, qualifiedName, version)
}
