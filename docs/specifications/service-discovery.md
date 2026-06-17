# Service discovery

**Status:** Draft

How the SDK resolves service base URLs from a SMART-on-openEHR deployment, and how non-discovering backends are supported. Covers REQ-070 through REQ-073.

A SMART-on-openEHR deployment advertises **service base URLs** via a discovery document (canonical spec: [ITS-REST/development § SMART App Launch](https://specifications.openehr.org/releases/ITS-REST/development/smart_app_launch.html)). The relevant service identifiers for this SDK include `org.openehr.rest` (the openEHR REST API) plus deployment-specific identifiers for Cadasto Extra, Datamap, Admin, and other extras. On a Cadasto deployment, the same discovery document advertises **`org.openehr.rest`** *and* **`org.fhir.rest`** — the openEHR-side SDK consumes the former and ignores the latter, while a FHIR-side SDK does the reverse.

The SDK treats discovery as a **first-class step**, not an implementation detail of constructor convenience.

## ServiceCatalog

### REQ-070

SDK constructors **MUST** accept a `smart/discovery.ServiceCatalog`, not a single base URL:

```go
// smart/discovery/catalog.go (sketch)

package discovery

import (
    "context"
    "net/url"
    "time"
)

// ServiceCatalog is the resolved set of service base URLs for a SMART-on-openEHR
// deployment, plus metadata for caching and refresh.
type ServiceCatalog struct {
    Issuer     string                  // deployment issuer URL
    Services   map[string]ServiceEntry // keyed by service identifier (e.g. "org.openehr.rest")
    Auth       AuthEndpoints           // authorization_endpoint, token_endpoint, jwks_uri, registration_endpoint
    ResolvedAt time.Time               // when the catalog was resolved
    ExpiresAt  time.Time               // TTL deadline; zero = no TTL declared by source
    ETag       string                  // for conditional refresh
}

type ServiceEntry struct {
    ID            string   // canonical identifier (e.g. "org.openehr.rest")
    BaseURL       *url.URL // resolved base URL
    SpecVersion   string   // declared spec version, e.g. "1.1.0-development"
    Capabilities  []string // optional capability flags
}

type AuthEndpoints struct {
    AuthorizationEndpoint *url.URL
    TokenEndpoint         *url.URL
    JWKSURI               *url.URL
    RegistrationEndpoint  *url.URL // optional
    // Optional endpoints — nil when absent; no error.
    IntrospectionEndpoint *url.URL // RFC 7662; feeds Phase 5b
    RevocationEndpoint    *url.URL // RFC 7009
    ManagementEndpoint    *url.URL // SMART management endpoint
    ScopesSupported       []string
    ResponseTypesSupported []string
    CodeChallengeMethodsSupported []string
    GrantTypesSupported   []string
    TokenEndpointAuthMethodsSupported          []string // feeds Phase 3b G-3
    TokenEndpointAuthSigningAlgValuesSupported []string // feeds Phase 3b alg selection (REQ-062)
    IDTokenSigningAlgValuesSupported           []string // ID-token verify allowlist, consumed by ValidateIDToken (REQ-062, REQ-064)
    Capabilities []string
}
```

Every typed client (`openehr/client/ehr`, `openehr/client/query`, `cadasto/extra`, etc.) **MUST** resolve its base URL from the catalog by service ID, not from a top-level "base URL" config field.

### Hand-built catalogs

For non-discovering openEHR backends (a static EHRbase deployment, a Cadasto deployment with a pinned configuration, a local CDR for testing), consumers **MUST** be able to construct a `ServiceCatalog` directly without going through a discovery transport:

```go
catalog := discovery.NewStaticCatalog(discovery.StaticConfig{
    Issuer: "https://ehrbase.example/",
    Services: map[string]discovery.ServiceEntry{
        "org.openehr.rest": {
            ID:          "org.openehr.rest",
            BaseURL:     mustParse("https://ehrbase.example/rest/openehr/v1"),
            SpecVersion: "1.1.0-development",
        },
    },
    Auth: discovery.AuthEndpoints{
        // ... or zero value if backend doesn't use OAuth2
    },
})
```

The `NewStaticCatalog` constructor **MUST NOT** require a network round trip; the resulting catalog has `ResolvedAt = time.Now()` and `ExpiresAt = time.Time{}` (no TTL).

## Resolution flow

The full flow when discovery is in play:

1. **Resolve.** On client construction (or first I/O if construction is lazy), fetch the SMART configuration document at the issuer's well-known URL — typically **`<issuer>/.well-known/smart-configuration`**. Parse it; validate required fields (`authorization_endpoint`, `token_endpoint`, `jwks_uri`, the openEHR-REST service catalog).
2. **Cache.** Store the resolved `ServiceCatalog`. The cache **MAY** be in-process (default), file-backed, or an injected `Cache` interface (for distributed deployments).
3. **Validate.** Confirm every service the client intends to use is advertised. `org.openehr.rest` **MUST** be present for openEHR-REST consumers; Cadasto-extra services **MUST** be present for Cadasto clients. Spec-version compatibility is checked **here**, not after the first request.
4. **Route.** Each typed client resolves its base URL from the catalog by service ID at request time.
5. **Refresh.** Triggered by:
   - TTL expiry (`ExpiresAt` reached).
   - `401` / `403` on a previously-working endpoint (might indicate the deployment rotated keys; refresh and retry once).
   - Explicit consumer call: `sdk.RefreshDiscovery(ctx)`.

## Caching

### REQ-071

The discovery cache **MUST**:

- Honour the TTL declared in the discovery response. If no TTL is declared, a default TTL (default: 15 minutes) **MUST** apply.
- Honour `ETag` / `If-None-Match` for conditional refresh — a `304 Not Modified` on refresh extends the cached entry's TTL without replacing the body.
- Be invalidated on `401` / `403` against a previously-working endpoint, after at most one refresh attempt.
- Coalesce concurrent resolution attempts (REQ-026) — one goroutine fetches; the others wait.

Cache implementation:

- The default cache is in-process (a `sync.Map` keyed by issuer URL).
- A `Cache` interface **MAY** be injected for file-backed or distributed caching:

```go
type Cache interface {
    Get(ctx context.Context, issuer string) (*ServiceCatalog, bool)
    Put(ctx context.Context, issuer string, c *ServiceCatalog) error
    Invalidate(ctx context.Context, issuer string) error
}
```

## Validation

### REQ-072

On every resolution and every refresh, the SDK **MUST**:

- Verify required services are present. Missing required services **MUST** produce a typed `DiscoveryError` with the missing service IDs enumerated.
- Verify spec-version compatibility. When a service entry advertises a `spec_version`, it **MUST** match the SDK's pinned target (REQ-050) or the caller-widened set; a mismatch **MUST** produce a typed `DiscoveryError`. When a service entry does **not** advertise `spec_version` (field absent or empty) and the caller has not explicitly narrowed the accepted set via `WithAcceptedSpecVersions`, the check is **skipped** — absence is treated as acceptable (ADR 0008). This preserves strict behaviour for callers that pin versions explicitly.
- Validate URL well-formedness. Malformed `BaseURL` / `AuthorizationEndpoint` / etc. **MUST** produce a typed `DiscoveryError`.

Soft compatibility (forward-compatible spec micro-versions) **MAY** be allowed via a functional option:

```go
discovery.WithAcceptedSpecVersions("1.1.0-development", "1.1.0", "1.1.1")
```

The default is **strict** — only the pinned version is accepted.

---

## REQ-073 — Discovery trust posture

SMART configuration documents and their auth endpoints are untrusted input until validated. On every resolution and refresh the SDK **MUST**:

- **Issuer match (OIDC Discovery §4.3).** When the fetched document declares an `"issuer"` field, it **MUST** equal the issuer URL used to fetch the document. A mismatch **MUST** reject the catalog with `DiscoveryError{Reason: ReasonIssuerMismatch}` — the document's issuer **MUST NOT** silently override the caller's requested issuer (that would let a hostile or misconfigured server impersonate another identity provider downstream).
- **HTTPS on auth endpoints.** `authorization_endpoint`, `token_endpoint`, `jwks_uri`, and `registration_endpoint` (when present) **MUST** use the `https` scheme unless the resolver is constructed with `WithAllowInsecure()`. Plaintext URLs **MUST** produce `DiscoveryError{Reason: ReasonInsecureURL}`. The `allowInsecure` path **MAY** log a warning instead of failing for development deployments.
- **Service `base_url` entries.** Plaintext `services[].base_url` values **SHOULD** emit the REQ-092 warning when not explicitly marked insecure; hard rejection remains a product decision beyond the auth-endpoint floor (see archived [security-hardening plan](../plans/archive/2026-06-11-security-hardening-and-simplification.md)).

Same-origin JWKS enforcement (rejecting `jwks_uri` hosts that differ from the issuer host) is **deferred** — HTTPS-only is the v1 floor.

- **Lives in:** [`smart/discovery/`](../../smart/discovery)
- **Tests:** `smart/discovery/resolver_test.go`

## Refresh API

Consumers **MUST** be able to trigger a refresh explicitly:

```go
catalog, err := sdk.RefreshDiscovery(ctx)
```

This:

- Invalidates the cached catalog for the configured issuer.
- Re-runs the resolve / validate / cache pipeline.
- Returns the new catalog (or an error if resolution fails).

The refresh API **MUST NOT** block other in-flight requests beyond the coalescing window — they continue with the stale catalog until the refresh completes (typical) or fails (in which case the next request after refresh fails with the discovery error).

## Errors

```go
type DiscoveryError struct {
    Issuer  string
    Reason  DiscoveryErrorReason
    Inner   error
}

type DiscoveryErrorReason string

const (
    ReasonFetchFailed          DiscoveryErrorReason = "fetch_failed"
    ReasonParseError           DiscoveryErrorReason = "parse_error"
    ReasonMissingService       DiscoveryErrorReason = "missing_service"
    ReasonSpecVersionMismatch  DiscoveryErrorReason = "spec_version_mismatch"
    ReasonMalformedURL         DiscoveryErrorReason = "malformed_url"
    ReasonAuthEndpointsMissing DiscoveryErrorReason = "auth_endpoints_missing"
    ReasonInsecureURL          DiscoveryErrorReason = "insecure_url"
    ReasonIssuerMismatch       DiscoveryErrorReason = "issuer_mismatch"
)

func (e *DiscoveryError) Error() string
func (e *DiscoveryError) Unwrap() error
```

Discovery errors **MUST** be distinguishable from wire errors via `errors.As(err, &transport.WireError{})` vs `errors.As(err, &DiscoveryError{})`.

## What is NOT in scope here

- **Service registration.** The SDK consumes discovery output; it does not publish or maintain the discovery document.
- **DNS resolution caching.** That belongs to the injected `*http.Client`'s transport configuration.
- **Health probing.** Discovery validates the catalog *structure*; whether the advertised endpoints are reachable is checked on first use, not at resolution time.
- **Cross-issuer aggregation.** The federator use case constructs one client per issuer (REQ-065); the SDK does not aggregate catalogs across issuers.
- **FHIR-side service consumption.** Even when the discovery document advertises `org.fhir.rest`, the SDK ignores it. A sibling FHIR SDK consumes that service.

## Surfaced authorization-server metadata (REQ-070, REQ-062)

The resolver parses and surfaces the following SMART authorization-server metadata fields onto `AuthEndpoints`. All fields are optional — absent fields resolve to nil/empty with no error.

| Wire field | `AuthEndpoints` field | Notes |
|---|---|---|
| `introspection_endpoint` | `IntrospectionEndpoint *url.URL` | RFC 7662; feeds Phase 5b introspection client (REQ-062) |
| `revocation_endpoint` | `RevocationEndpoint *url.URL` | RFC 7009 token revocation |
| `management_endpoint` | `ManagementEndpoint *url.URL` | SMART management endpoint |
| `token_endpoint_auth_methods_supported` | `TokenEndpointAuthMethodsSupported []string` | Client-auth method list; feeds Phase 3b G-3 selection |
| `token_endpoint_auth_signing_alg_values_supported` | `TokenEndpointAuthSigningAlgValuesSupported []string` | Client-assertion (client-auth) JWS alg list; feeds Phase 3b alg selection (REQ-062) — surface-only in v0.8 |
| `id_token_signing_alg_values_supported` | `IDTokenSigningAlgValuesSupported []string` | Selects the **ID-token verify allowlist** — pass it to `smart.WithIDTokenSigningAlgs` so `ValidateIDToken` constrains accepted signature algorithms (RS256/RS384/ES256/ES384). Consumed as of Phase 3e (REQ-062, REQ-064; see [auth.md](auth.md#req-062--jwks-rotation)) |

Most of these fields remain **surface-only** in v0.8: the resolver populates `token_endpoint_auth_signing_alg_values_supported` (client-auth alg selection), `token_endpoint_auth_methods_supported` (method selection), and the introspection/revocation/management endpoints, but no consuming logic for those is wired in this release — that lands in later phases. The exception is `id_token_signing_alg_values_supported`, which the ID-token verifier consumes as of Phase 3e (see the table note above).

The `smart/discovery` package also exports openEHR SMART capability string constants (`CapabilityContextOpenEHREHR`, `CapabilityContextOpenEHREpisode`, `CapabilityOpenEHRPermissionV1`, `CapabilityLaunchBase64JSON`) for consumers that need to branch on the `capabilities` array.

## Coverage matrix

| Topic | REQ | Lives in |
|---|---|---|
| First-class catalog | REQ-070 | `smart/discovery/`, every typed client constructor |
| Auth-server metadata surface | REQ-070, REQ-062 | `smart/discovery/` |
| Cache + refresh | REQ-071 | `smart/discovery/` |
| Validation | REQ-072 | `smart/discovery/` |
| Trust posture | REQ-073 | `smart/discovery/` |
