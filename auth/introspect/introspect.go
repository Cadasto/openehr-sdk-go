package introspect

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/cadasto/openehr-sdk-go/auth"
)

// Client posts token introspection requests to a RFC 7662 endpoint.
// Construct one with [New]; safe for concurrent use.
type Client struct {
	endpoint   string
	httpClient *http.Client
}

// New constructs an introspection Client for the given endpoint URL.
// httpClient is required (REQ-021); a nil value returns [auth.ErrInvalidConfig].
// endpoint must be a non-empty, parseable absolute URL; an empty or
// unparseable value returns [auth.ErrInvalidConfig].
func New(endpoint string, httpClient *http.Client, _ ...Option) (*Client, error) {
	if httpClient == nil {
		return nil, fmt.Errorf("%w: HTTPClient is required (REQ-021)", auth.ErrInvalidConfig)
	}
	if endpoint == "" {
		return nil, fmt.Errorf("%w: endpoint is required", auth.ErrInvalidConfig)
	}
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("%w: endpoint %q is not a valid absolute URL", auth.ErrInvalidConfig, endpoint)
	}
	return &Client{endpoint: endpoint, httpClient: httpClient}, nil
}

// Option is a placeholder for future functional options. No options are
// currently defined; the variadic parameter exists for forward compatibility.
type Option func()

// Result holds the RFC 7662 §2.2 introspection response fields.
type Result struct {
	// Active indicates whether the presented token is currently active.
	// Required per RFC 7662 §2.2.
	Active bool

	// Scope is the space-separated list of OAuth2 scopes associated with
	// the token. Optional per RFC 7662 §2.2.
	Scope string

	// ClientID is the client identifier for the OAuth2 client that
	// requested the token. JSON field: "client_id". Optional per RFC 7662 §2.2.
	ClientID string

	// Username is the human-readable identifier for the resource owner
	// who authorized the token. Optional per RFC 7662 §2.2.
	Username string

	// TokenType is the type of the token (e.g. "Bearer"). JSON field:
	// "token_type". Optional per RFC 7662 §2.2.
	TokenType string

	// Exp is the token expiry time, parsed from the RFC 7662 numeric date
	// (seconds since Unix epoch). Zero if absent. Optional per RFC 7662 §2.2.
	Exp time.Time

	// Iat is the time at which the token was issued, parsed from the RFC
	// 7662 numeric date. Zero if absent. Optional per RFC 7662 §2.2.
	Iat time.Time

	// Nbf is the time before which the token must not be accepted, parsed
	// from the RFC 7662 numeric date. Zero if absent. Optional per RFC 7662 §2.2.
	Nbf time.Time

	// Sub is the subject of the token, usually a machine-readable
	// identifier of the resource owner. Optional per RFC 7662 §2.2.
	Sub string

	// Aud is the audience for the token. RFC 7662 §2.2 allows this to be
	// a single string or a JSON array; when an array is received the
	// values are joined with a space (consistent with the scope convention
	// and OAuth2 audience handling elsewhere in this SDK).
	// Optional per RFC 7662 §2.2.
	Aud string

	// Iss is the issuer of the token. Optional per RFC 7662 §2.2.
	Iss string

	// Jti is the unique identifier for the token. Optional per RFC 7662 §2.2.
	Jti string

	// Patient is the SMART / FHIR launch-context patient identifier,
	// conveyed via the "patient" introspection claim. Absent when the
	// deployment does not surface SMART launch context in introspection
	// responses.
	Patient string

	// FHIRUser is the SMART / FHIR launch-context user claim ("fhirUser").
	// Absent when not surfaced by the deployment.
	FHIRUser string

	// EHRID is the openEHR-native EHR identifier, conveyed via the
	// "ehrId" token claim (REQ-064). Absent when not surfaced by the
	// deployment.
	EHRID string

	// EpisodeID is the openEHR-native episode identifier, conveyed via
	// the "episodeId" token claim (REQ-064, experimental). Absent when
	// not surfaced by the deployment.
	EpisodeID string

	// Raw carries the complete decoded introspection response body,
	// including vendor-extension claims not mapped to typed fields.
	Raw map[string]any
}

// introspectionResponse is the internal decode target for the RFC 7662 body.
// Numeric dates and the aud polymorphism are handled in parseResult.
type introspectionResponse struct {
	Active    bool            `json:"active"`
	Scope     string          `json:"scope"`
	ClientID  string          `json:"client_id"`
	Username  string          `json:"username"`
	TokenType string          `json:"token_type"`
	Exp       json.Number     `json:"exp"`
	Iat       json.Number     `json:"iat"`
	Nbf       json.Number     `json:"nbf"`
	Sub       string          `json:"sub"`
	Aud       json.RawMessage `json:"aud"`
	Iss       string          `json:"iss"`
	Jti       string          `json:"jti"`
	// SMART / FHIR launch-context extras.
	Patient   string `json:"patient"`
	FHIRUser  string `json:"fhirUser"`
	EHRID     string `json:"ehrId"`
	EpisodeID string `json:"episodeId"`
}

// Introspect posts a RFC 7662 §2.1 introspection request for token to
// the configured endpoint. bearer is the resource server's own access
// token used to authenticate to the introspection endpoint (carried as
// "Authorization: Bearer <bearer>").
//
// ctx is threaded to the HTTP request (REQ-020).
//
// A response with active:false is a SUCCESSFUL introspection and is
// returned as (Result{Active:false}, nil). Only non-2xx HTTP responses
// or transport / parse failures are returned as errors.
func (c *Client) Introspect(ctx context.Context, token string, bearer string) (Result, error) {
	form := url.Values{"token": {token}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return Result{}, fmt.Errorf("introspect: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+bearer)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return Result{}, &auth.ExchangeError{
			Sentinel: auth.ErrTokenExchangeFailed,
			Inner:    fmt.Errorf("introspect: %w", err),
		}
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return Result{}, &auth.ExchangeError{
			Sentinel:   auth.ErrTokenExchangeFailed,
			StatusCode: resp.StatusCode,
			Inner:      fmt.Errorf("introspect: read body: %w", err),
		}
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Result{}, &auth.ExchangeError{
			Sentinel:   auth.ErrTokenExchangeFailed,
			StatusCode: resp.StatusCode,
			OAuth2:     auth.ParseOAuth2Error(body),
			Inner:      fmt.Errorf("introspect: endpoint returned %d", resp.StatusCode),
		}
	}

	return parseResult(body)
}

// parseResult decodes the RFC 7662 introspection response body.
func parseResult(body []byte) (Result, error) {
	var ir introspectionResponse
	if err := json.Unmarshal(body, &ir); err != nil {
		return Result{}, fmt.Errorf("introspect: decode response: %w", err)
	}

	// Decode raw map for the Raw field.
	var rawMap map[string]any
	if err := json.Unmarshal(body, &rawMap); err != nil {
		return Result{}, fmt.Errorf("introspect: decode raw response: %w", err)
	}

	r := Result{
		Active:    ir.Active,
		Scope:     ir.Scope,
		ClientID:  ir.ClientID,
		Username:  ir.Username,
		TokenType: ir.TokenType,
		Sub:       ir.Sub,
		Iss:       ir.Iss,
		Jti:       ir.Jti,
		Patient:   ir.Patient,
		FHIRUser:  ir.FHIRUser,
		EHRID:     ir.EHRID,
		EpisodeID: ir.EpisodeID,
		Raw:       rawMap,
	}

	// Parse numeric dates from RFC 7662 §2.2 (seconds since Unix epoch).
	r.Exp = parseNumericDate(ir.Exp)
	r.Iat = parseNumericDate(ir.Iat)
	r.Nbf = parseNumericDate(ir.Nbf)

	// Parse aud: RFC 7662 allows a single string or a JSON array.
	r.Aud = parseAud(ir.Aud)

	return r, nil
}

// parseNumericDate converts a RFC 7662 numeric date (float64 JSON number,
// seconds since Unix epoch) to time.Time. Returns the zero value when n
// is empty or zero.
func parseNumericDate(n json.Number) time.Time {
	if n == "" {
		return time.Time{}
	}
	f, err := n.Float64()
	if err != nil || f == 0 {
		return time.Time{}
	}
	sec := int64(f)
	nsec := int64((f - float64(sec)) * 1e9)
	return time.Unix(sec, nsec).UTC()
}

// parseAud handles the RFC 7662 / JWT aud claim polymorphism: the value
// may be a JSON string or a JSON array of strings. When an array is
// received the values are joined with a single space (matching the scope
// convention). Returns an empty string when the field is absent or null.
func parseAud(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	// Try scalar string first.
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	// Try array of strings.
	var arr []string
	if err := json.Unmarshal(raw, &arr); err == nil {
		return strings.Join(arr, " ")
	}
	return ""
}
