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
	// CreatedOn is when the deployment first received this template.
	CreatedOn time.Time `json:"created_on,omitzero"`
	// Description is an optional free-text description.
	Description string `json:"description,omitempty"`
	// Extras preserves deployment-specific fields not in the standard
	// metadata shape.
	Extras map[string]json.RawMessage `json:"-"`
}

var knownTemplateMetadataFields = map[string]struct{}{
	"template_id":  {},
	"concept":      {},
	"archetype_id": {},
	"version":      {},
	"created_on":   {},
	"description":  {},
}

// UnmarshalJSON decodes both documented fields and Extras in one pass.
func (m *TemplateMetadata) UnmarshalJSON(data []byte) error {
	type alias TemplateMetadata
	var a alias
	if err := json.Unmarshal(data, &a); err != nil {
		return err
	}
	*m = TemplateMetadata(a)

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
	raw, err := io.ReadAll(body)
	if err != nil {
		return nil, nil, fmt.Errorf("definition.UploadTemplate: read body: %w", err)
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
		Path:   "/definition/template/" + format.PathSegment() + "/" + url.PathEscape(templateID),
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
		Path:   "/definition/template/" + format.PathSegment() + "/" + url.PathEscape(templateID),
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

// exampleConfig is the resolved option set for [ExampleComposition].
type exampleConfig struct {
	format string
}

// ExampleOption mutates [ExampleComposition]'s request shape.
type ExampleOption func(*exampleConfig)

// WithExampleFormat overrides the `format` query parameter the SDK
// requests for the example response. Default is omitted — the
// deployment chooses its canonical-JSON default. Consumers wanting
// FLAT or STRUCTURED variants pass them here (and accept that the
// returned bytes will not decode into [*rm.Composition]).
func WithExampleFormat(f string) ExampleOption {
	return func(c *exampleConfig) { c.format = f }
}

// ExampleComposition asks the deployment to synthesise an example
// COMPOSITION for templateID. The example is typically used by
// validators and UIs to bootstrap a payload against a known template.
//
// Wire: GET /definition/template/{format}/{template_id}/example_composition.
// Decodes the response body via canjson into a [*rm.Composition].
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
	req := &transport.Request{
		Method: http.MethodGet,
		Path:   "/definition/template/" + format.PathSegment() + "/" + url.PathEscape(templateID) + "/example_composition",
		Route:  "/definition/template/{format}/{template_id}/example_composition",
		Accept: "application/json",
	}
	if cfg.format != "" {
		req.Query = url.Values{"format": []string{cfg.format}}
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
