package care

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"github.com/cadasto/openehr-sdk-go/auth/clientcreds"
	"github.com/cadasto/openehr-sdk-go/openehr/client/ehr"
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

	tokenOpts := []clientcreds.Option{clientcreds.WithHTTPClient(hc)}
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

// CreatePatient creates a fresh EHR (the patient's clinical record) in the CDR
// and returns its ehr_id.
func (c *Client) CreatePatient(ctx context.Context) (string, error) {
	e, _, err := ehr.Create(ctx, c.rest)
	if err != nil {
		return "", fmt.Errorf("care: create EHR: %w", err)
	}
	return e.EHRID.Value, nil
}

// SaveData writes a datamap payload for a patient under the given template.
//
// Slice 4b: resolve the OPT for templateID, run codec.ToComposition, bridge the
// canonical JSON to a typed *rm.Composition, optionally validate against the
// OPT, then POST via openehr/client/ehr/composition.Save.
func (c *Client) SaveData(ctx context.Context, patientID, templateID string, datamap map[string]any) (string, error) {
	if c.codec == nil {
		return "", fmt.Errorf("care: no Codec configured")
	}
	return "", ErrNotImplemented
}

// GetData reads a stored composition back as a datamap payload. Slice 4b.
func (c *Client) GetData(ctx context.Context, patientID, templateID, uid string) (map[string]any, error) {
	if c.codec == nil {
		return nil, fmt.Errorf("care: no Codec configured")
	}
	return nil, ErrNotImplemented
}

// ListData lists stored composition references for a patient + template via
// AQL. Slice 4b.
func (c *Client) ListData(ctx context.Context, patientID, templateID string) ([]string, error) {
	return nil, ErrNotImplemented
}
