package care

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/cadasto/openehr-sdk-go/auth/clientcreds"
	"github.com/cadasto/openehr-sdk-go/openehr/client/definition"
	"github.com/cadasto/openehr-sdk-go/openehr/client/ehr"
	"github.com/cadasto/openehr-sdk-go/openehr/client/ehr/composition"
	"github.com/cadasto/openehr-sdk-go/openehr/client/query"
	"github.com/cadasto/openehr-sdk-go/openehr/template"
	"github.com/cadasto/openehr-sdk-go/smart/discovery"
	"github.com/cadasto/openehr-sdk-go/transport"
)

// Codec is the datamap <-> RM COMPOSITION conversion seam. Per the AGENTS.md
// boundary rule, cadasto/care does NOT import cadasto/datamap directly; a thin
// adapter wired by the consumer satisfies this interface by delegating to the
// datamap package functions. Keeping it an interface preserves the cadasto/
// extraction cut line.
type Codec interface {
	ToComposition(opt *template.OperationalTemplate, datamap map[string]any) (map[string]any, error)
	FromComposition(opt *template.OperationalTemplate, comp map[string]any) (map[string]any, error)
}

// Config configures a Cadasto care Client. The Lab24 platform-side per-tenant
// configuration (PROP-0032: cadasto_tenant_config) maps onto these fields — the
// platform resolves endpoint/credentials and constructs this Config per tenant.
type Config struct {
	// Tenant + Environment are informational (subdomain, dev/acc/prod).
	Tenant      string
	Environment string
	// BaseURL is the openEHR REST base, e.g.
	// "https://<tenant>.api.acc.cadasto.io/openehr/v1". Required.
	BaseURL string
	// TokenURL is the OAuth2 client-credentials token endpoint. Required.
	TokenURL string
	// ClientID / ClientSecret are the OAuth2 client-credentials. Required.
	ClientID     string
	ClientSecret string
	// Audience optionally binds the token to an audience.
	Audience string
	// HTTPClient is injected (the SDK never allocates one). Defaults to
	// http.DefaultClient when nil.
	HTTPClient *http.Client
	// Codec converts between datamap payloads and RM compositions.
	Codec Codec
}

// Client is a thin Cadasto domain client over the openEHR REST surface.
// Goroutine-safe once constructed.
type Client struct {
	rest  *transport.Client
	codec Codec
}

// ErrNotImplemented marks domain operations whose composition bridge is not yet
// wired (Slice 4b: datamap -> *rm.Composition + OPT resolution).
var ErrNotImplemented = errors.New("cadasto/care: operation not implemented yet")

// NewClient builds a care Client from Config: a static openEHR-REST service
// catalog (BaseURL) plus an OAuth2 client-credentials token source (TokenURL).
// Construction only — no network call until the first request.
func NewClient(cfg Config) (*Client, error) {
	if cfg.BaseURL == "" {
		return nil, fmt.Errorf("care: BaseURL is required")
	}
	if cfg.TokenURL == "" {
		return nil, fmt.Errorf("care: TokenURL is required")
	}
	if cfg.ClientID == "" || cfg.ClientSecret == "" {
		return nil, fmt.Errorf("care: ClientID and ClientSecret are required")
	}

	base, err := url.Parse(cfg.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("care: invalid BaseURL: %w", err)
	}

	hc := cfg.HTTPClient
	if hc == nil {
		hc = http.DefaultClient
	}

	catalog, err := discovery.NewStaticCatalog(discovery.StaticConfig{
		Issuer: cfg.TokenURL,
		Services: map[string]discovery.ServiceEntry{
			discovery.ServiceIDOpenEHRRest: {
				ID:          discovery.ServiceIDOpenEHRRest,
				BaseURL:     base,
				SpecVersion: discovery.SpecVersionPin,
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("care: service catalog: %w", err)
	}

	// Cadasto's authorization server advertises only client_secret_post
	// (token_endpoint_auth_methods_supported), so credentials go in the form
	// body — not HTTP Basic (the clientcreds default).
	tokenOpts := []clientcreds.Option{
		clientcreds.WithHTTPClient(hc),
		clientcreds.WithAuthMethod(clientcreds.AuthPost),
	}
	if cfg.Audience != "" {
		tokenOpts = append(tokenOpts, clientcreds.WithAudience(cfg.Audience))
	}
	src, err := clientcreds.New(cfg.ClientID, cfg.ClientSecret, cfg.TokenURL, tokenOpts...)
	if err != nil {
		return nil, fmt.Errorf("care: token source: %w", err)
	}

	rest, err := transport.New(catalog, transport.WithHTTPClient(hc), transport.WithTokenSource(src))
	if err != nil {
		return nil, fmt.Errorf("care: transport: %w", err)
	}

	return &Client{rest: rest, codec: cfg.Codec}, nil
}

// Verify checks connectivity to the configured Cadasto CDR. It forces an
// OAuth2 token acquisition and performs a single read-only capability call
// (list operational templates), exercising the token endpoint, base URL and
// audience without any write side effect. On success it returns the number of
// templates visible; on failure the error pinpoints the failing leg (token vs.
// REST). Intended for a platform "test connection" action.
func (c *Client) Verify(ctx context.Context) (int, error) {
	tpls, _, err := definition.ListTemplates(ctx, c.rest, definition.FormatADL14)
	if err != nil {
		return 0, fmt.Errorf("care: verify connection: %w", err)
	}
	return len(tpls), nil
}

// FetchOPT retrieves and parses the operational template for templateID. It
// lets a consumer run the datamap codec (Schema/ToComposition) against a live
// CDR template without care having to import cadasto/datamap (boundary rule).
func (c *Client) FetchOPT(ctx context.Context, templateID string) (*template.OperationalTemplate, error) {
	optBytes, err := c.FetchOPTRaw(ctx, templateID)
	if err != nil {
		return nil, err
	}
	opt, err := template.ParseOPT(bytes.NewReader(optBytes))
	if err != nil {
		return nil, fmt.Errorf("care: parse template %s: %w", templateID, err)
	}
	return opt, nil
}

// FetchOPTRaw returns the raw OPT bytes for templateID without parsing — useful
// to inspect the deployment's exact serialization when ParseOPT rejects it.
func (c *Client) FetchOPTRaw(ctx context.Context, templateID string) ([]byte, error) {
	optBytes, _, err := definition.GetTemplate(ctx, c.rest, templateID, definition.FormatADL14)
	if err != nil {
		return nil, fmt.Errorf("care: fetch template %s: %w", templateID, err)
	}
	return optBytes, nil
}

// TemplateInfo is a lightweight view of an operational template in the CDR.
type TemplateInfo struct {
	TemplateID  string `json:"template_id"`
	Concept     string `json:"concept,omitempty"`
	ArchetypeID string `json:"archetype_id,omitempty"`
	Version     string `json:"version,omitempty"`
}

// Templates lists the operational templates available in the CDR (read-only).
func (c *Client) Templates(ctx context.Context) ([]TemplateInfo, error) {
	metas, _, err := definition.ListTemplates(ctx, c.rest, definition.FormatADL14)
	if err != nil {
		return nil, fmt.Errorf("care: list templates: %w", err)
	}
	out := make([]TemplateInfo, len(metas))
	for i, m := range metas {
		out[i] = TemplateInfo{
			TemplateID:  m.TemplateID,
			Concept:     m.Concept,
			ArchetypeID: m.ArchetypeID,
			Version:     m.Version,
		}
	}
	return out, nil
}

// QueryResult is a tabular AQL result: column names plus rows of raw cells.
type QueryResult struct {
	Columns []string `json:"columns"`
	Rows    [][]any  `json:"rows"`
}

// Query runs an ad-hoc read-only AQL query against the CDR and returns the
// result as columns + rows. Intended for diagnostics; no EHR scope is applied.
func (c *Client) Query(ctx context.Context, aqlText string, params map[string]any) (*QueryResult, error) {
	rs, _, err := query.ExecuteString(ctx, c.rest, aqlText, params)
	if err != nil {
		return nil, fmt.Errorf("care: query: %w", err)
	}
	cols := make([]string, len(rs.Columns))
	for i, col := range rs.Columns {
		cols[i] = col.Name
	}
	return &QueryResult{Columns: cols, Rows: rs.Rows}, nil
}

// CreatePatient creates a fresh EHR (the patient's clinical record) in the CDR
// and returns its ehr_id.
func (c *Client) CreatePatient(ctx context.Context) (string, error) {
	e, _, err := ehr.Create(ctx, c.rest)
	if err != nil {
		return "", fmt.Errorf("care: create EHR: %w", err)
	}
	return e.EHRID.Value, nil
}

// SaveData writes a datamap payload for a patient under the given template:
// fetch the OPT from the CDR, encode via the codec, bridge canonical JSON to a
// typed *rm.Composition, and POST it. Returns the new composition version uid.
//
// Validation-gate (validate against the OPT before POST) is deferred — the
// SDK's template-driven validator requires the internal compiled-template type
// (REQ-102), not yet on the public surface.
func (c *Client) SaveData(ctx context.Context, patientID, templateID string, datamap map[string]any) (string, error) {
	if c.codec == nil {
		return "", fmt.Errorf("care: no Codec configured")
	}
	optBytes, _, err := definition.GetTemplate(ctx, c.rest, templateID, definition.FormatADL14)
	if err != nil {
		return "", fmt.Errorf("care: fetch template %s: %w", templateID, err)
	}
	opt, err := template.ParseOPT(bytes.NewReader(optBytes))
	if err != nil {
		return "", fmt.Errorf("care: parse template %s: %w", templateID, err)
	}
	compMap, err := c.codec.ToComposition(opt, datamap)
	if err != nil {
		return "", fmt.Errorf("care: encode composition: %w", err)
	}
	comp, err := compositionFromMap(compMap)
	if err != nil {
		return "", err
	}
	_, meta, err := composition.Save(ctx, c.rest, ehr.EHRID(patientID), comp, composition.WithTemplateID(templateID))
	if err != nil {
		return "", fmt.Errorf("care: save composition: %w", err)
	}
	if meta != nil {
		return string(meta.VersionUID), nil
	}
	return "", nil
}

// ListData returns the composition version uids stored for a patient under the
// given template, via an ad-hoc AQL query scoped to the patient's EHR.
func (c *Client) ListData(ctx context.Context, patientID, templateID string) ([]string, error) {
	const q = "SELECT c/uid/value AS uid FROM EHR e " +
		"CONTAINS COMPOSITION c " +
		"WHERE c/archetype_details/template_id/value = $tpl"
	rs, _, err := query.ExecuteString(ctx, c.rest, q, map[string]any{"tpl": templateID}, query.WithEHRID(patientID))
	if err != nil {
		return nil, fmt.Errorf("care: list data: %w", err)
	}
	var out []string
	for _, row := range rs.Rows {
		if len(row) > 0 {
			if s, ok := row[0].(string); ok {
				out = append(out, s)
			}
		}
	}
	return out, nil
}

// GetData reads a stored composition back as a datamap payload. Deferred
// (slice 4b tail): needs the ehr composition Ref construction for a specific
// version uid; ListData + SaveData cover the write+enumerate flow.
func (c *Client) GetData(ctx context.Context, patientID, templateID, uid string) (map[string]any, error) {
	if c.codec == nil {
		return nil, fmt.Errorf("care: no Codec configured")
	}
	return nil, ErrNotImplemented
}

// SaveComposition stores a NEW composition (POST) from an already-encoded
// canonical-JSON map and returns the new version uid. Codec-free: the caller
// has already produced the composition (e.g. via datamap.ToComposition).
func (c *Client) SaveComposition(ctx context.Context, patientID, templateID string, compMap map[string]any) (string, error) {
	comp, err := compositionFromMap(compMap)
	if err != nil {
		return "", err
	}
	_, meta, err := composition.Save(ctx, c.rest, ehr.EHRID(patientID), comp, composition.WithTemplateID(templateID))
	if err != nil {
		return "", fmt.Errorf("care: save composition: %w", err)
	}
	if meta != nil {
		return string(meta.VersionUID), nil
	}
	return "", nil
}

// UpdateComposition creates a NEW VERSION (PUT) of an existing composition and
// returns the new version uid (…::N+1). voID is the versioned-object uuid (the
// segment before the first "::"); ifMatch is the current full version uid.
func (c *Client) UpdateComposition(ctx context.Context, patientID, voID, ifMatch, templateID string, compMap map[string]any) (string, error) {
	comp, err := compositionFromMap(compMap)
	if err != nil {
		return "", err
	}
	_, meta, err := composition.Update(ctx, c.rest, ehr.EHRID(patientID), ehr.VersionedObjectID(voID), ifMatch, comp, composition.WithTemplateID(templateID))
	if err != nil {
		return "", fmt.Errorf("care: update composition: %w", err)
	}
	if meta != nil {
		return string(meta.VersionUID), nil
	}
	return "", nil
}

// CompositionETag GETs a composition by ref (a versioned-object id returns the
// latest version, a full version uid returns that version) and returns the
// response ETag — i.e. the current version uid, suitable as an If-Match.
func (c *Client) CompositionETag(ctx context.Context, patientID, ref string) (string, error) {
	req := &transport.Request{
		Method: http.MethodGet,
		Path:   "/ehr/" + url.PathEscape(patientID) + "/composition/" + url.PathEscape(ref),
		Route:  "/ehr/{ehr_id}/composition/{versioned_object_or_version_uid}",
		Accept: "application/json",
	}
	resp, err := c.rest.Do(ctx, req)
	if err != nil {
		return "", fmt.Errorf("care: head composition %s: %w", ref, err)
	}
	if resp != nil && resp.Metadata != nil {
		return resp.Metadata.ETag, nil
	}
	return "", nil
}

// UpdateCompositionRaw PUTs a composition map as canonical JSON directly,
// bypassing the typed *rm.Composition bridge (which rejects RM subtype
// polymorphism and can re-serialize lossily). Returns the new version uid.
func (c *Client) UpdateCompositionRaw(ctx context.Context, patientID, voID, ifMatch, templateID string, compMap map[string]any) (string, error) {
	// Cadasto quirks for composition update (verified against acc):
	//   1. the body MUST carry a uid (OBJECT_VERSION_ID = the preceding
	//      version), else the server 500s in its extractUid helper;
	//   2. the If-Match must be sent UNQUOTED — Cadasto keeps the surrounding
	//      double quotes in the parsed value and then rejects it as "not a
	//      valid Version UID". The transport quotes If-Match per RFC 7232, so
	//      we set it as a raw header (Headers overrides the standard one).
	compMap["uid"] = map[string]any{"_type": "OBJECT_VERSION_ID", "value": ifMatch}
	body, err := json.Marshal(compMap)
	if err != nil {
		return "", fmt.Errorf("care: marshal composition: %w", err)
	}
	req := &transport.Request{
		Method:     http.MethodPut,
		Path:       "/ehr/" + url.PathEscape(patientID) + "/composition/" + url.PathEscape(voID),
		Route:      "/ehr/{ehr_id}/composition/{versioned_object_id}",
		Body:       body,
		TemplateID: templateID,
		Accept:     "application/json",
		Headers:    http.Header{"If-Match": []string{ifMatch}},
	}
	resp, err := c.rest.Do(ctx, req)
	if err != nil {
		return "", fmt.Errorf("care: update composition (raw): %w", err)
	}
	if resp != nil && resp.Metadata != nil {
		return cleanVersionUID(resp.Metadata.ETag), nil
	}
	return "", nil
}

// cleanVersionUID strips surrounding quotes and the gzip-proxy "-gzip" ETag
// suffix that Cadasto's Caddy front-end appends to compression-negotiated
// responses, leaving a bare version uid.
func cleanVersionUID(s string) string {
	s = strings.TrimSuffix(strings.TrimPrefix(s, `"`), `"`)
	if i := strings.Index(s, "-gzip"); i >= 0 {
		s = s[:i]
	}
	return strings.Trim(s, `"`)
}

// SaveCompositionRaw POSTs a composition map as canonical JSON directly,
// bypassing the typed bridge. Returns the new version uid.
func (c *Client) SaveCompositionRaw(ctx context.Context, patientID, templateID string, compMap map[string]any) (string, error) {
	body, err := json.Marshal(compMap)
	if err != nil {
		return "", fmt.Errorf("care: marshal composition: %w", err)
	}
	req := &transport.Request{
		Method:     http.MethodPost,
		Path:       "/ehr/" + url.PathEscape(patientID) + "/composition",
		Route:      "/ehr/{ehr_id}/composition",
		Body:       body,
		TemplateID: templateID,
		Accept:     "application/json",
	}
	resp, err := c.rest.Do(ctx, req)
	if err != nil {
		return "", fmt.Errorf("care: save composition (raw): %w", err)
	}
	if resp != nil && resp.Metadata != nil {
		if resp.Metadata.ETag != "" {
			return resp.Metadata.ETag, nil
		}
		return resp.Metadata.Location, nil
	}
	return "", nil
}

// GetComposition retrieves a stored composition by EHR id and version uid and
// returns it as a canonical-JSON map (read-only). The caller can run the
// datamap decoder (FromComposition) on the result with the matching OPT.
func (c *Client) GetComposition(ctx context.Context, patientID, versionUID string) (map[string]any, error) {
	// Fetch the raw canonical JSON instead of decoding into a typed
	// *rm.Composition: the typed path (canjson/typereg) rejects RM subtype
	// polymorphism (e.g. a DV_CODED_TEXT value where the model types DV_TEXT),
	// and the datamap codec only needs the map form anyway.
	b, err := c.GetCompositionRaw(ctx, patientID, versionUID, "application/json")
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("care: decode composition %s: %w", versionUID, err)
	}
	return m, nil
}

// GetCompositionRaw fetches a stored composition's raw bytes in the requested
// representation (e.g. "application/json" or "application/xml"), bypassing the
// typed RM decode. Empty accept defaults to JSON.
func (c *Client) GetCompositionRaw(ctx context.Context, patientID, versionUID, accept string) ([]byte, error) {
	if accept == "" {
		accept = "application/json"
	}
	req := &transport.Request{
		Method: http.MethodGet,
		Path:   "/ehr/" + url.PathEscape(patientID) + "/composition/" + url.PathEscape(versionUID),
		Route:  "/ehr/{ehr_id}/composition/{versioned_object_or_version_uid}",
		Accept: accept,
	}
	resp, err := c.rest.Do(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("care: get composition %s: %w", versionUID, err)
	}
	return resp.Body, nil
}
