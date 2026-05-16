package discovery

import (
	"net/url"
	"time"
)

// ServiceCatalog is the resolved set of service base URLs for a
// SMART-on-openEHR deployment, plus metadata for caching and refresh
// (REQ-070). Pass by pointer; treat as immutable after Resolver
// produces it.
type ServiceCatalog struct {
	// Issuer is the deployment's authoritative issuer URL.
	Issuer string
	// Services maps service identifier (e.g. "org.openehr.rest") to
	// the resolved entry. A SMART-on-openEHR document advertises both
	// "org.openehr.rest" and "org.fhir.rest"; openEHR-side SDKs consume
	// the former and ignore the latter.
	Services map[string]ServiceEntry
	// Auth carries the OAuth2 / OIDC endpoints the deployment exposes.
	Auth AuthEndpoints
	// ResolvedAt records when the catalog was fetched. For hand-built
	// catalogs (NewStaticCatalog) this is the constructor call time.
	ResolvedAt time.Time
	// ExpiresAt is the catalog's TTL deadline. The zero value means
	// "no TTL declared by source" — callers MAY apply a default.
	ExpiresAt time.Time
	// ETag is the source's ETag for conditional refresh; empty when
	// the source did not advertise one.
	ETag string
}

// Service returns the entry for serviceID and ok=true when present.
// Use this rather than direct map access at call sites so a missing
// service surfaces as a typed error rather than a zero value.
func (c *ServiceCatalog) Service(serviceID string) (ServiceEntry, bool) {
	if c == nil {
		return ServiceEntry{}, false
	}
	e, ok := c.Services[serviceID]
	return e, ok
}

// OpenEHRRest is shorthand for c.Service("org.openehr.rest"). Returns
// the entry and ok=true when the catalog advertises the openEHR REST
// service; the typed leaf clients call this on every request.
func (c *ServiceCatalog) OpenEHRRest() (ServiceEntry, bool) {
	return c.Service(ServiceIDOpenEHRRest)
}

// Stale reports whether c is past its declared expiry. Catalogs
// without an ExpiresAt are never stale by this measure — TTL is the
// authoritative trigger; consumers MAY trigger refresh on other signals
// (401/403) independently.
func (c *ServiceCatalog) Stale(now time.Time) bool {
	if c == nil {
		return true
	}
	if c.ExpiresAt.IsZero() {
		return false
	}
	return !now.Before(c.ExpiresAt)
}

// ServiceEntry is one resolved service in the catalog.
type ServiceEntry struct {
	// ID is the canonical service identifier (e.g. "org.openehr.rest").
	ID string
	// BaseURL is the parsed, validated base URL for this service.
	// Always absolute; transport/ joins paths onto this URL.
	BaseURL *url.URL
	// SpecVersion is the declared spec version (e.g. "1.1.0-development").
	// Validated against the SDK's pin at resolution time (REQ-072).
	SpecVersion string
	// Capabilities is an optional capability flag list the deployment
	// advertised. Opaque to the SDK; consumers may inspect it.
	Capabilities []string
}

// AuthEndpoints carries the OAuth2 / OIDC endpoints from the SMART
// configuration document.
type AuthEndpoints struct {
	AuthorizationEndpoint             *url.URL
	TokenEndpoint                     *url.URL
	JWKSURI                           *url.URL
	RegistrationEndpoint              *url.URL
	ScopesSupported                   []string
	ResponseTypesSupported            []string
	CodeChallengeMethodsSupported     []string
	GrantTypesSupported               []string
	TokenEndpointAuthMethodsSupported []string
	Capabilities                      []string
}

// Service identifier constants. The SDK consumes only the openEHR
// service; the FHIR identifier is included so non-Go SDKs sharing
// these constants can avoid string-literal drift.
const (
	ServiceIDOpenEHRRest = "org.openehr.rest"
	ServiceIDFHIRRest    = "org.fhir.rest"
)
