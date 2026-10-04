---
kind: specification
---

# Service discovery

**Status:** Draft

How the SDK resolves service base URLs from a SMART-on-openEHR deployment, and how non-discovering backends are supported. Covers REQ-070 through REQ-073.

A SMART-on-openEHR deployment advertises **service base URLs** via a discovery document (canonical spec: [ITS-REST/development § SMART App Launch](https://specifications.openehr.org/releases/ITS-REST/development/smart_app_launch.html)). The relevant service identifiers for this SDK include `org.openehr.rest` (the openEHR REST API) plus deployment-specific identifiers for Cadasto Extra, Datamap, Admin, and other extras. On a Cadasto deployment, the same discovery document advertises **`org.openehr.rest`** *and* **`org.fhir.rest`** — the openEHR-side SDK consumes the former and ignores the latter, while a FHIR-side SDK does the reverse.

The SDK treats discovery as a **first-class step**, not an implementation detail of constructor convenience.

## Canonical sources

- **openEHR SMART App Launch** — [https://specifications.openehr.org/releases/ITS-REST/development/smart_app_launch.html](https://specifications.openehr.org/releases/ITS-REST/development/smart_app_launch.html) — normative definition of the SMART discovery document shape for openEHR deployments, the `services` map, and openEHR-specific service identifiers.
- **HL7 FHIR SMART App Launch v2.2** — [https://hl7.org/fhir/smart-app-launch/](https://hl7.org/fhir/smart-app-launch/) — defines the `/.well-known/smart-configuration` endpoint, required and optional metadata fields (including `authorization_endpoint`, `token_endpoint`, `jwks_uri`, `scopes_supported`, and algorithm-support lists).

See also [ADR 0009](../adr/0009-smart-auth-library-scope.md) for the dependency decisions that underpin the discovery implementation.

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
    BaseURL    string                  // Platform base URL: the discovery root, and the "iss" of an embedded launch
    Issuer     string                  // OIDC issuer: the document's "issuer", or BaseURL when the document declares none
    Services   map[string]ServiceEntry // keyed by service identifier (e.g. "org.openehr.rest")
    Auth       AuthEndpoints           // authorization_endpoint, token_endpoint, jwks_uri, registration_endpoint
    ResolvedAt time.Time               // when the catalog was resolved
    ExpiresAt  time.Time               // TTL deadline; zero = no TTL declared by source
    ETag       string                  // for conditional refresh
}

type ServiceEntry struct {
    ID            string   // canonical identifier (e.g. "org.openehr.rest")
    BaseURL       *url.URL // resolved base URL
    Version       string   // the entry's "version" member, verbatim
    SpecVersion   string   // the non-canonical "spec_version" member, verbatim (tolerated, ADR 0008)
    Description   string   // the entry's "description" member, verbatim
    Documentation string   // the entry's "documentation" member, verbatim
    OpenAPI       string   // the entry's "openapi" member, verbatim
    Capabilities  []string // optional capability flags
}

type AuthEndpoints struct {
    AuthorizationEndpoint *url.URL
    TokenEndpoint         *url.URL
    JWKSURI               *url.URL
    RegistrationEndpoint  *url.URL // optional
    // Optional endpoints — nil when absent; no error.
    IntrospectionEndpoint *url.URL // RFC 7662; surfaced, not consumed
    RevocationEndpoint    *url.URL // RFC 7009
    ManagementEndpoint    *url.URL // SMART management endpoint
    ScopesSupported       []string
    ResponseTypesSupported []string
    CodeChallengeMethodsSupported []string
    GrantTypesSupported   []string
    TokenEndpointAuthMethodsSupported          []string // G-3 cross-check; NewFromCatalog method check (REQ-068)
    TokenEndpointAuthSigningAlgValuesSupported []string // NewFromCatalog client-assertion alg check (REQ-068)
    IDTokenSigningAlgValuesSupported           []string // ID-token verify allowlist, consumed by ValidateIDToken (REQ-062, REQ-064)
    Capabilities []string
}
```

Every typed client (`openehr/client/ehr`, `openehr/client/query`, `cadasto/extra`, etc.) **MUST** resolve its base URL from the catalog by service ID, not from a top-level "base URL" config field.

The catalog **MUST** carry the Platform base URL and the OIDC issuer as two values ([ADR 0023](../adr/0023-smart-platform-base-url-and-oidc-issuer.md)). `BaseURL` is the URL the document was resolved from: in SMART App Launch the embedded-launch `iss` parameter names this API base, not the OIDC issuer. `Issuer` is the document's `issuer` member, the value ID tokens are checked against; when the document declares no `issuer`, `Issuer` **MUST** equal `BaseURL`.

Each `ServiceEntry` **MUST** surface the canonical `version`, `description`, `documentation` and `openapi` members of its `services` entry verbatim, empty when absent. The SDK **MUST NOT** fetch or validate the `documentation` and `openapi` links.

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

The `NewStaticCatalog` constructor **MUST NOT** require a network round trip; the resulting catalog has `ResolvedAt = time.Now()` and `ExpiresAt = time.Time{}` (no TTL). `StaticConfig.BaseURL` is optional: when empty, the catalog's `BaseURL` **MUST** equal `StaticConfig.Issuer`.

## Resolution flow

The full flow when discovery is in play:

1. **Resolve.** On client construction (or first I/O if construction is lazy), fetch the SMART configuration document at the Platform base URL's well-known URL, **`<base URL>/.well-known/smart-configuration`**, the well-known path appended to the base URL's own path. Parse it; validate the members REQ-072 requires and the openEHR-REST service catalog.
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
- Honour `ETag` / `If-None-Match` for conditional refresh: a `304 Not Modified` to a conditional request extends the cached entry's TTL without replacing the body, and a `304 Not Modified` to a request without `If-None-Match` is a failed fetch ([§ Refresh API](#refresh-api)).
- Be invalidated on `401` / `403` against a previously-working endpoint, after at most one refresh attempt.
- Coalesce concurrent resolution attempts (REQ-026) — one goroutine fetches; the others wait.
- Key every entry by the Platform base URL the caller resolved, never by the document's `issuer` ([ADR 0023](../adr/0023-smart-platform-base-url-and-oidc-issuer.md)).

The resolver defers closing the response body before it branches on the status, so the `304` path closes it too.

**Bullet 3 — catalog refresh on 401 via transport hook (Phase 4b).** A consumer that wants the transport layer to drive a catalog refresh on a wire `401` can supply a `ReautherFunc` closure to `transport.WithReauthOn401`:

```go
transport.WithReauthOn401(auth.ReautherFunc(func(ctx context.Context) error {
    _, err := resolver.Refresh(ctx, baseURL)
    return err
}))
```

`auth.ReautherFunc` satisfies `auth.Reauther` without importing `smart/discovery` into `transport/`. The transport calls the closure at most once per `Do` invocation; on a second `401` after the retry it surfaces `transport.ErrUnauthorized`. This wires REQ-071 bullet 3 (invalidate on 401 + retry once) through the opt-in transport safety net described in [auth.md § REQ-063](auth.md#req-063--token-refresh).

Cache implementation:

- The default cache is in-process.
- A `Cache` interface **MAY** be injected for file-backed or distributed caching:

```go
type Cache interface {
    Get(ctx context.Context, baseURL string) (*ServiceCatalog, bool)
    Put(ctx context.Context, baseURL string, c *ServiceCatalog) error
    Invalidate(ctx context.Context, baseURL string) error
}
```

## Validation

### REQ-072

On every resolution and every refresh, the SDK **MUST**:

- Verify required services are present. Missing required services **MUST** produce a typed `DiscoveryError` with the missing service IDs enumerated.
- Verify spec-version compatibility. When a service entry advertises a `spec_version`, it **MUST** match the SDK's pinned target (REQ-050) or, when the caller sets `WithAcceptedSpecVersions`, one of the versions it names; that list replaces the pinned target, so a caller who still accepts the pin names it too. A mismatch **MUST** produce a typed `DiscoveryError`. When a service entry does **not** advertise `spec_version` (field absent or empty) and the caller has not explicitly narrowed the accepted set via `WithAcceptedSpecVersions`, the check is **skipped** — absence is treated as acceptable (ADR 0008). This preserves strict behaviour for callers that pin versions explicitly. Without `WithAcceptedSpecVersions` the canonical `version` member **MUST NOT** be compared, so a Platform that advertises its own API version is not refused by default. With `WithAcceptedSpecVersions`, the compared value **MUST** be `spec_version` when the entry advertises it and `version` otherwise.
- Verify the authorization-server members SMART App Launch 2.2.0 makes conditional, whenever the document declares any of `authorization_endpoint`, `token_endpoint` or `jwks_uri`, or advertises any of the capabilities `launch-ehr`, `launch-standalone` or `sso-openid-connect` (a document with neither is an anonymous-only deployment and passes): `token_endpoint` **MUST** be present; `authorization_endpoint` **MUST** be present when `capabilities` contains `launch-ehr` or `launch-standalone`; `jwks_uri` **MUST** be present when `capabilities` contains `sso-openid-connect`. A document that omits a member it needs **MUST** produce `DiscoveryError{Reason: ReasonAuthEndpointsMissing}`; a backend-only document without `authorization_endpoint` **MUST** be accepted.
- Validate URL well-formedness. Malformed `BaseURL` / `AuthorizationEndpoint` / etc. **MUST** produce a typed `DiscoveryError`.

Soft compatibility (forward-compatible spec micro-versions) **MAY** be allowed via a functional option:

```go
discovery.WithAcceptedSpecVersions("1.1.0-development", "1.1.0", "1.1.1")
```

The default is **strict** — only the pinned version is accepted.

---

## REQ-073 — Discovery trust posture

SMART configuration documents and their auth endpoints are untrusted input until validated. On every resolution and refresh the SDK **MUST**:

- **Issuer ([ADR 0023](../adr/0023-smart-platform-base-url-and-oidc-issuer.md)).** When the fetched document declares an `"issuer"` member, the SDK **MUST** accept it as the catalog's `Issuer` whether or not it equals the Platform base URL, provided it is an absolute URL with the `https` scheme and no query or fragment (OIDC Core 1.0 §2). A malformed issuer **MUST** produce `DiscoveryError{Reason: ReasonMalformedURL}`; an `http` issuer **MUST** produce `DiscoveryError{Reason: ReasonInsecureURL}` unless the resolver is constructed with `WithAllowInsecure()`. The document's issuer **MUST NOT** replace the base URL: the base URL the caller resolved stays the catalog's `BaseURL` and its cache key.
- **OIDC cross-check.** When the document declares an `issuer` that differs from the Platform base URL, the resolver **MUST**, on every resolution and refresh, fetch `<issuer>/.well-known/openid-configuration` (OIDC Discovery 1.0 §4, the path appended to the issuer's own path) and require its `issuer` member to equal the catalog's `Issuer` exactly (OIDC Discovery 1.0 §4.3) and, when both documents declare `jwks_uri`, the two values to be equal. A mismatch **MUST** produce `DiscoveryError{Reason: ReasonIssuerMismatch}`, and a failed fetch `DiscoveryError{Reason: ReasonFetchFailed}`; an OIDC document that does not parse, or names no `issuer`, is a failed fetch, not a mismatch. The resolver **MUST NOT** fetch the OIDC document when it is constructed with `WithoutOpenIDConfigurationCheck()`, or when the document declares no `issuer` or an `issuer` equal to the base URL.
- **HTTPS on auth endpoints.** `authorization_endpoint`, `token_endpoint`, `jwks_uri`, and `registration_endpoint` (when present) **MUST** use the `https` scheme unless the resolver is constructed with `WithAllowInsecure()`. Plaintext URLs **MUST** produce `DiscoveryError{Reason: ReasonInsecureURL}`. The `allowInsecure` path **MAY** log a warning instead of failing for development deployments.
- **No downgrade on redirect.** While fetching the SMART configuration document or the OIDC document of the cross-check, the resolver **MUST NOT** follow a redirect to a URL whose scheme is not `https`, unless it is constructed with `WithAllowInsecure()`; such a redirect **MUST** produce `DiscoveryError{Reason: ReasonInsecureURL}`. Otherwise the injected client's own redirect policy applies.
- **Service `base_url` entries.** Plaintext `services[].base_url` values **SHOULD** emit the REQ-092 warning when not explicitly marked insecure; hard rejection remains a product decision beyond the auth-endpoint floor ([PR 31](https://github.com/Cadasto/openehr-sdk-go/pull/31)).

Same-origin JWKS enforcement (rejecting `jwks_uri` hosts that differ from the issuer host) is **deferred** — HTTPS-only is the v1 floor.

- **Lives in:** [`smart/discovery/`](../../smart/discovery)

## Refresh API

Consumers **MUST** be able to trigger a refresh explicitly:

```go
catalog, err := sdk.RefreshDiscovery(ctx)
```

A refresh **MUST**:

- Send a conditional request (`If-None-Match`) when the cached entry carries an `ETag`, and a plain request when it carries none, keeping that entry in place meanwhile.
- On a `304 Not Modified` to a conditional request, re-run the REQ-072 checks and the REQ-073 trust checks, the OIDC cross-check included, on the cached document, and renew the cached entry's TTL without replacing its document (REQ-071); on a `200`, re-run the resolve / validate / cache pipeline and replace the entry; on a failure, invalidate the entry. A `304 Not Modified` to a plain request is a failed fetch: no document came back.
- Return the current catalog (or an error if resolution fails).

The refresh API **MUST NOT** block other in-flight requests beyond the coalescing window — they continue with the stale catalog until the refresh completes (typical) or fails (in which case the next request after refresh fails with the discovery error).

## Errors

```go
type DiscoveryError struct {
    Issuer  string // the Platform base URL the resolution was for
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
- **Cross-Platform aggregation.** The federator use case constructs one client per Platform base URL (REQ-065); the SDK does not aggregate catalogs across Platforms.
- **FHIR-side service consumption.** Even when the discovery document advertises `org.fhir.rest`, the SDK ignores it. A sibling FHIR SDK consumes that service.

## Surfaced authorization-server metadata (REQ-070, REQ-062)

The resolver parses and surfaces the following SMART authorization-server metadata fields onto `AuthEndpoints`. All fields are optional — absent fields resolve to nil/empty with no error.

| Wire field | `AuthEndpoints` field | Notes |
|---|---|---|
| `introspection_endpoint` | `IntrospectionEndpoint *url.URL` | RFC 7662 token introspection, a resource-server operation; surfaced only |
| `revocation_endpoint` | `RevocationEndpoint *url.URL` | RFC 7009 token revocation |
| `management_endpoint` | `ManagementEndpoint *url.URL` | SMART management endpoint |
| `token_endpoint_auth_methods_supported` | `TokenEndpointAuthMethodsSupported []string` | Client-auth method list; feeds Phase 3b G-3 selection |
| `token_endpoint_auth_signing_alg_values_supported` | `TokenEndpointAuthSigningAlgValuesSupported []string` | Client-assertion (client-auth) JWS alg list; `auth/clientcreds.NewFromCatalog` refuses an SDK-signed assertion whose algorithm it leaves out (REQ-068) |
| `id_token_signing_alg_values_supported` | `IDTokenSigningAlgValuesSupported []string` | Selects the **ID-token verify allowlist** — pass it to `smart.WithIDTokenSigningAlgs` so `ValidateIDToken` constrains accepted signature algorithms (RS256/RS384/ES256/ES384). Consumed as of Phase 3e (REQ-062, REQ-064; see [auth.md](auth.md#req-062--jwks-rotation)) |

These fields are **consumed**, not merely surfaced:

- `id_token_signing_alg_values_supported` → the ID-token verifier's accepted-algorithm allowlist (Phase 3e; pass via `smart.WithIDTokenSigningAlgs`).
- `token_endpoint_auth_methods_supported` → `auth/smart.FromConfig` cross-checks it against the configured credential's implied method (G-3; a mismatch is rejected with `auth.ErrInvalidConfig`), and `auth/clientcreds.NewFromCatalog` against the configured client-auth method ([auth.md § Backend Services from a resolved catalog](auth.md#backend-services-from-a-resolved-catalog), REQ-068).
- `token_endpoint_auth_signing_alg_values_supported` → `auth/clientcreds.NewFromCatalog` refuses a client assertion from the SDK's own `jwtbearer.ClaimsSigner` whose algorithm a non-empty list leaves out (REQ-068); the SDK does not choose an algorithm from it.

The rest remain **surface-only** (populated but with no consuming logic wired): `revocation_endpoint` / `management_endpoint` (no revocation/management client yet) and `introspection_endpoint` (introspection is a resource-server operation outside the SDK's client scope).

The `smart/discovery` package also exports openEHR SMART capability string constants (`CapabilityContextOpenEHREHR`, `CapabilityContextOpenEHREpisode`, `CapabilityOpenEHRPermissionV1`, `CapabilityLaunchBase64JSON`) for consumers that need to branch on the `capabilities` array.

## Coverage matrix

| Topic | REQ | Lives in |
|---|---|---|
| First-class catalog | REQ-070 | `smart/discovery/`, every typed client constructor |
| Auth-server metadata surface | REQ-070, REQ-062 | `smart/discovery/` |
| Cache + refresh | REQ-071 | `smart/discovery/` |
| Validation | REQ-072 | `smart/discovery/` |
| Trust posture | REQ-073 | `smart/discovery/` |
