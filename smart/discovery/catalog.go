package discovery

import (
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/cadasto/openehr-sdk-go/internal/urlredact"
)

// ServiceCatalog is the resolved set of service base URLs for a
// SMART-on-openEHR deployment, plus metadata for caching and refresh.
// Pass by pointer; treat as immutable after Resolver
// produces it.
//
// A catalog carries two URLs that are often, but not always, the same.
// BaseURL is the Platform base URL: the address the SMART configuration
// was resolved from, which an embedded SMART launch passes to the app as
// its "iss" parameter. Issuer is the OpenID Connect issuer that signs ID
// tokens. A Platform whose sign-in is handled by a separate identity
// provider declares that provider's URL as its issuer, so the two differ.
//
// Printed with fmt or logged with log/slog, a *ServiceCatalog shows every
// URL it holds with its password masked: see [ServiceCatalog.String]. Its
// JSON encoding is unchanged and carries BaseURL and Issuer as written, so a
// [Cache] can store a catalog and read it back.
type ServiceCatalog struct {
	// BaseURL is the Platform base URL the catalog was resolved from,
	// exactly as the caller passed it to Resolver.Resolve or
	// Resolver.Refresh. The resolver caches the catalog under this URL.
	// For a catalog built by NewStaticCatalog it is StaticConfig.BaseURL,
	// or StaticConfig.Issuer when that is empty.
	BaseURL string
	// Issuer is the OpenID Connect issuer: the "issuer" member of the
	// SMART configuration, or BaseURL when the document declares none.
	// ID tokens are checked against this value.
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
	// "no TTL declared by source"; callers can apply a default.
	ExpiresAt time.Time
	// ETag is the source's ETag for conditional refresh; empty when
	// the source did not advertise one.
	ETag string

	// smartJWKSURI is the SMART configuration's jwks_uri as written, which
	// the resolver compares with the issuer's OpenID configuration again
	// when a 304 Not Modified renews the catalog. A catalog that went
	// through a cache that keeps exported fields only comes back without it.
	smartJWKSURI string
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
// service. The typed clients look up each request's service with Service;
// OpenEHRRest is for a caller that wants the openEHR REST entry itself.
func (c *ServiceCatalog) OpenEHRRest() (ServiceEntry, bool) {
	return c.Service(ServiceIDOpenEHRRest)
}

// Stale reports whether c is past its declared expiry. Catalogs
// without an ExpiresAt are never stale by this measure. Stale checks the
// TTL only; callers can trigger a refresh on other signals (401/403)
// independently.
func (c *ServiceCatalog) Stale(now time.Time) bool {
	if c == nil {
		return true
	}
	if c.ExpiresAt.IsZero() {
		return false
	}
	return !now.Before(c.ExpiresAt)
}

// String describes c by its exported fields, with the password of every URL
// that carries one masked as [url.URL.Redacted] masks it: BaseURL, Issuer,
// the base URL and links of each service, and the auth endpoints. Each
// service is shown by the text of [ServiceEntry.String], in the order of its
// key, Auth by the text of [AuthEndpoints.String], and ResolvedAt and
// ExpiresAt in RFC 3339 form. A nil c gives "<nil>".
//
// String, GoString, Format and LogValue are methods of *ServiceCatalog,
// because the [Resolver] and [NewStaticCatalog] hand out pointers. A
// ServiceCatalog value copied out of one has none of them, and fmt and
// log/slog then print its fields, BaseURL and Issuer as written, so print or
// log the pointer.
func (c *ServiceCatalog) String() string {
	if c == nil {
		return "<nil>"
	}
	var services strings.Builder
	services.WriteString("map[")
	for i, id := range slices.Sorted(maps.Keys(c.Services)) {
		if i > 0 {
			services.WriteString(", ")
		}
		fmt.Fprintf(&services, "%q: %s", id, c.Services[id].String())
	}
	services.WriteString("]")
	return fmt.Sprintf("discovery.ServiceCatalog{BaseURL: %q, Issuer: %q, Services: %s, Auth: %s, ResolvedAt: %s, ExpiresAt: %s, ETag: %q}",
		urlredact.String(c.BaseURL), urlredact.String(c.Issuer), services.String(), c.Auth.String(),
		c.ResolvedAt.Format(time.RFC3339Nano), c.ExpiresAt.Format(time.RFC3339Nano), c.ETag)
}

// GoString returns the text of [ServiceCatalog.String], so that printing c
// in Go syntax shows no password either.
func (c *ServiceCatalog) GoString() string { return c.String() }

// Format writes the text of [ServiceCatalog.String] for every fmt verb,
// whatever its flags and width. Without it, fmt would print the fields of c,
// BaseURL and Issuer as written. With a nil f it does nothing.
func (c *ServiceCatalog) Format(f fmt.State, _ rune) {
	if f == nil {
		return
	}
	_, _ = io.WriteString(f, c.String())
}

// LogValue lets log/slog log c as a group of its exported fields, keyed by
// field name, with every URL's password masked as [ServiceCatalog.String]
// masks it. Services is a group keyed by service identifier, each entry
// logged as [ServiceEntry.LogValue] logs it, and Auth is logged as
// [AuthEndpoints.LogValue] logs it. A nil c logs as nil.
func (c *ServiceCatalog) LogValue() slog.Value {
	if c == nil {
		return slog.AnyValue(nil)
	}
	services := make([]slog.Attr, 0, len(c.Services))
	for _, id := range slices.Sorted(maps.Keys(c.Services)) {
		services = append(services, slog.Attr{Key: id, Value: c.Services[id].LogValue()})
	}
	return slog.GroupValue(
		slog.String("BaseURL", urlredact.String(c.BaseURL)),
		slog.String("Issuer", urlredact.String(c.Issuer)),
		slog.Attr{Key: "Services", Value: slog.GroupValue(services...)},
		slog.Attr{Key: "Auth", Value: c.Auth.LogValue()},
		slog.Time("ResolvedAt", c.ResolvedAt),
		slog.Time("ExpiresAt", c.ExpiresAt),
		slog.String("ETag", c.ETag),
	)
}

// ServiceEntry is one resolved service in the catalog.
//
// Printed with fmt or logged with log/slog, a ServiceEntry shows its URLs
// with their passwords masked: see [ServiceEntry.String]. Its JSON encoding
// is unchanged.
type ServiceEntry struct {
	// ID is the canonical service identifier (e.g. "org.openehr.rest").
	ID string
	// BaseURL is the parsed, validated base URL for this service.
	// Always absolute; transport/ joins paths onto this URL.
	BaseURL *url.URL
	// Version is the entry's "version" member, verbatim, usually the
	// Platform's own API version; empty when absent. On a required service,
	// the resolver compares it only when the entry has no SpecVersion and the
	// caller set WithAcceptedSpecVersions.
	Version string
	// SpecVersion is the entry's "spec_version" member, verbatim (e.g.
	// "1.1.0"); empty when absent. This member is not part of
	// the SMART on openEHR document, but the resolver reads it when present
	// and, on a required service, checks it against the accepted versions
	// at resolution time.
	SpecVersion string
	// Description is the entry's "description" member, verbatim; empty
	// when absent.
	Description string
	// Documentation is the entry's "documentation" link, verbatim; empty
	// when absent. The SDK neither fetches nor validates it.
	Documentation string
	// OpenAPI is the entry's "openapi" link, verbatim; empty when absent.
	// The SDK neither fetches nor validates it.
	OpenAPI string
	// Capabilities is an optional capability flag list the deployment
	// advertised. Opaque to the SDK; consumers may inspect it.
	Capabilities []string
}

// String describes e by its fields, with the password of BaseURL,
// Documentation and OpenAPI masked when one carries one, as
// [url.URL.Redacted] masks it. A nil BaseURL is shown as "".
//
// String, GoString, Format and LogValue are methods of ServiceEntry, because
// [ServiceCatalog.Service] hands out values, so a *ServiceEntry prints the
// same way. fmt answers %T and %p without calling Format, and %p of a
// ServiceEntry value prints its fields, Documentation and OpenAPI as
// written, so never print a ServiceEntry with %p.
func (e ServiceEntry) String() string {
	return fmt.Sprintf("discovery.ServiceEntry{ID: %q, BaseURL: %q, Version: %q, SpecVersion: %q, Description: %q, "+
		"Documentation: %q, OpenAPI: %q, Capabilities: %q}",
		e.ID, e.BaseURL.Redacted(), e.Version, e.SpecVersion, e.Description,
		urlredact.String(e.Documentation), urlredact.String(e.OpenAPI), e.Capabilities)
}

// GoString returns the text of [ServiceEntry.String], so that printing e in
// Go syntax shows no password either.
func (e ServiceEntry) GoString() string { return e.String() }

// Format writes the text of [ServiceEntry.String] for every fmt verb,
// whatever its flags and width. With a nil f it does nothing.
func (e ServiceEntry) Format(f fmt.State, _ rune) {
	if f == nil {
		return
	}
	_, _ = io.WriteString(f, e.String())
}

// LogValue lets log/slog log e as a group of its fields, keyed by field
// name, with the URLs masked as [ServiceEntry.String] masks them.
func (e ServiceEntry) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("ID", e.ID),
		slog.String("BaseURL", e.BaseURL.Redacted()),
		slog.String("Version", e.Version),
		slog.String("SpecVersion", e.SpecVersion),
		slog.String("Description", e.Description),
		slog.String("Documentation", urlredact.String(e.Documentation)),
		slog.String("OpenAPI", urlredact.String(e.OpenAPI)),
		slog.Any("Capabilities", e.Capabilities),
	)
}

// AuthEndpoints carries the OAuth2 / OIDC endpoints from the SMART
// configuration document.
//
// Printed with fmt or logged with log/slog, an AuthEndpoints shows each
// endpoint with its password masked: see [AuthEndpoints.String]. Its JSON
// encoding is unchanged.
type AuthEndpoints struct {
	AuthorizationEndpoint *url.URL
	TokenEndpoint         *url.URL
	JWKSURI               *url.URL
	RegistrationEndpoint  *url.URL
	// IntrospectionEndpoint is the RFC 7662 token-introspection endpoint
	// advertised by the authorization server. Nil when absent. Discovery
	// surfaces it, but the SDK does not consume it: token introspection is
	// a resource-server operation.
	IntrospectionEndpoint *url.URL
	// RevocationEndpoint is the RFC 7009 token-revocation endpoint. Nil when
	// absent. The revoke method in auth/smart posts the token there.
	RevocationEndpoint *url.URL
	// ManagementEndpoint is the SMART management endpoint (deployment-specific).
	// Nil when absent.
	ManagementEndpoint *url.URL

	ScopesSupported               []string
	ResponseTypesSupported        []string
	CodeChallengeMethodsSupported []string
	// GrantTypesSupported lists the OAuth2 grant types the authorization
	// server accepts (e.g. "authorization_code", "client_credentials").
	// clientcreds.NewFromCatalog refuses to build a source when the list is
	// non-empty and leaves out "client_credentials".
	GrantTypesSupported []string
	// TokenEndpointAuthMethodsSupported lists the client-authentication methods
	// the authorization server accepts (e.g. "private_key_jwt",
	// "client_secret_basic"). auth/smart and clientcreds.NewFromCatalog check
	// the configured client credential against this list when it is non-empty.
	TokenEndpointAuthMethodsSupported []string
	// TokenEndpointAuthSigningAlgValuesSupported lists the JWS algorithms
	// accepted for client-assertion JWTs at the token endpoint
	// (e.g. "RS384", "ES384"). When the list is non-empty, auth/smart and
	// clientcreds.NewFromCatalog refuse a client assertion whose algorithm
	// is not in it.
	// The SDK does not use the list to select an algorithm.
	TokenEndpointAuthSigningAlgValuesSupported []string
	// IDTokenSigningAlgValuesSupported lists the JWS algorithms used to sign
	// ID tokens (e.g. "RS256", "ES384"). When the list is not empty,
	// auth/smart accepts only the listed algorithms the SDK supports as it
	// verifies the ID token at the code exchange and on a refresh. To verify
	// an ID token yourself with the same limit, pass the list to
	// smart.WithIDTokenSigningAlgs.
	IDTokenSigningAlgValuesSupported []string
	// AuthorizationResponseIssParameterSupported reports whether the
	// authorization server adds an "iss" parameter, naming itself, to every
	// authorization response it sends to the redirect URI. It comes from the
	// authorization_response_iss_parameter_supported member (RFC 9207 §3)
	// and is false when the document leaves it out. When it is true,
	// auth/smart refuses a redirect that arrives without "iss".
	AuthorizationResponseIssParameterSupported bool
	Capabilities                               []string
}

// String describes a by its fields, with the password of each endpoint URL
// that carries one masked as [url.URL.Redacted] masks it. A nil endpoint is
// shown as "".
//
// String, GoString, Format and LogValue are methods of AuthEndpoints,
// because a [ServiceCatalog] holds its endpoints as a value, so an
// *AuthEndpoints prints the same way. fmt answers %T and %p without calling
// Format, and %p of an AuthEndpoints value prints its fields, each endpoint
// as an address.
func (a AuthEndpoints) String() string {
	return fmt.Sprintf("discovery.AuthEndpoints{AuthorizationEndpoint: %q, TokenEndpoint: %q, JWKSURI: %q, "+
		"RegistrationEndpoint: %q, IntrospectionEndpoint: %q, RevocationEndpoint: %q, ManagementEndpoint: %q, "+
		"ScopesSupported: %q, ResponseTypesSupported: %q, CodeChallengeMethodsSupported: %q, GrantTypesSupported: %q, "+
		"TokenEndpointAuthMethodsSupported: %q, TokenEndpointAuthSigningAlgValuesSupported: %q, "+
		"IDTokenSigningAlgValuesSupported: %q, AuthorizationResponseIssParameterSupported: %t, Capabilities: %q}",
		a.AuthorizationEndpoint.Redacted(), a.TokenEndpoint.Redacted(), a.JWKSURI.Redacted(),
		a.RegistrationEndpoint.Redacted(), a.IntrospectionEndpoint.Redacted(), a.RevocationEndpoint.Redacted(),
		a.ManagementEndpoint.Redacted(),
		a.ScopesSupported, a.ResponseTypesSupported, a.CodeChallengeMethodsSupported, a.GrantTypesSupported,
		a.TokenEndpointAuthMethodsSupported, a.TokenEndpointAuthSigningAlgValuesSupported,
		a.IDTokenSigningAlgValuesSupported, a.AuthorizationResponseIssParameterSupported, a.Capabilities)
}

// GoString returns the text of [AuthEndpoints.String], so that printing a in
// Go syntax shows no password either.
func (a AuthEndpoints) GoString() string { return a.String() }

// Format writes the text of [AuthEndpoints.String] for every fmt verb,
// whatever its flags and width. With a nil f it does nothing.
func (a AuthEndpoints) Format(f fmt.State, _ rune) {
	if f == nil {
		return
	}
	_, _ = io.WriteString(f, a.String())
}

// LogValue lets log/slog log a as a group of its fields, keyed by field
// name, with the endpoints masked as [AuthEndpoints.String] masks them.
func (a AuthEndpoints) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("AuthorizationEndpoint", a.AuthorizationEndpoint.Redacted()),
		slog.String("TokenEndpoint", a.TokenEndpoint.Redacted()),
		slog.String("JWKSURI", a.JWKSURI.Redacted()),
		slog.String("RegistrationEndpoint", a.RegistrationEndpoint.Redacted()),
		slog.String("IntrospectionEndpoint", a.IntrospectionEndpoint.Redacted()),
		slog.String("RevocationEndpoint", a.RevocationEndpoint.Redacted()),
		slog.String("ManagementEndpoint", a.ManagementEndpoint.Redacted()),
		slog.Any("ScopesSupported", a.ScopesSupported),
		slog.Any("ResponseTypesSupported", a.ResponseTypesSupported),
		slog.Any("CodeChallengeMethodsSupported", a.CodeChallengeMethodsSupported),
		slog.Any("GrantTypesSupported", a.GrantTypesSupported),
		slog.Any("TokenEndpointAuthMethodsSupported", a.TokenEndpointAuthMethodsSupported),
		slog.Any("TokenEndpointAuthSigningAlgValuesSupported", a.TokenEndpointAuthSigningAlgValuesSupported),
		slog.Any("IDTokenSigningAlgValuesSupported", a.IDTokenSigningAlgValuesSupported),
		slog.Bool("AuthorizationResponseIssParameterSupported", a.AuthorizationResponseIssParameterSupported),
		slog.Any("Capabilities", a.Capabilities),
	)
}

// Service identifier constants. The SDK consumes only the openEHR
// service; the FHIR identifier is included so a caller that reads the
// FHIR entry of a SMART-on-openEHR document can name it with a constant
// rather than a string literal.
const (
	ServiceIDOpenEHRRest = "org.openehr.rest"
	ServiceIDFHIRRest    = "org.fhir.rest"
)

// openEHR SMART capability string constants.
//
// These values appear in the "capabilities" array of a SMART configuration
// document advertised by an openEHR-capable authorization server, as defined
// by the canonical openEHR SMART App Launch specification
// (https://specifications.openehr.org/releases/ITS-REST/development/smart_app_launch.html).
//
// Callers can inspect ServiceCatalog.Auth.Capabilities or
// ServiceEntry.Capabilities to branch on these strings. The SDK itself
// does not enforce or select behaviour based on them.
const (
	// CapabilityContextOpenEHREHR indicates the server can return an openEHR
	// EHR context parameter on launch.
	CapabilityContextOpenEHREHR = "context-openehr-ehr"

	// CapabilityContextOpenEHREpisode indicates the server can return an
	// openEHR episode context parameter on launch.
	CapabilityContextOpenEHREpisode = "context-openehr-episode"

	// CapabilityOpenEHRPermissionV1 indicates the server supports the openEHR
	// permission model v1 scope vocabulary.
	CapabilityOpenEHRPermissionV1 = "openehr-permission-v1"

	// CapabilityLaunchBase64JSON indicates launch context parameters are
	// delivered as base64-encoded JSON rather than as plain query parameters.
	CapabilityLaunchBase64JSON = "launch-base64-json"
)
