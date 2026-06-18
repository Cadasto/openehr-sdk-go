# Authentication

**Status:** Draft

Normative contract for the `auth/` package family and the application-level `smart/` package. Covers REQ-060 through REQ-064 and REQ-069.

The SDK supports authenticated requests through a layered model:

```
Application
   └─→ smart/                          (application-level launch context)
   └─→ auth/<provider>/                (concrete provider — SMART, ClientCreds, JWTBearer)
                  └─→ auth/            (generic TokenSource + OAuth2 primitives)
```

The boundary between layers is **the generic `TokenSource` abstraction** — providers implement it; transports consume it; everything authenticated flows through it.

## Canonical sources

The SMART-on-openEHR authentication model in this SDK is derived from two primary specifications:

- **openEHR SMART App Launch** — [https://specifications.openehr.org/releases/ITS-REST/development/smart_app_launch.html](https://specifications.openehr.org/releases/ITS-REST/development/smart_app_launch.html) — defines openEHR-specific extensions: the `services` discovery map, `launch/patient`, `launch/episode`, `ehrId`, `episodeId` claims, and the `org.openehr.rest` service identifier.
- **HL7 SMART App Launch v2.2** — [https://hl7.org/fhir/smart-app-launch/](https://hl7.org/fhir/smart-app-launch/) — defines the PKCE flow, scopes and launch context (including `offline_access`, `online_access`, `launch`, `launch/patient`), client-confidential-asymmetric (`private_key_jwt`), Backend Services, and JWKS rotation.

See also [ADR 0009](../adr/0009-smart-auth-library-scope.md) for the dependency and library-scope decisions underpinning this implementation.

## TokenSource contract

### REQ-060

`auth/` **MUST** define a `TokenSource` interface returning a token, an expiry, and a non-nil error or nil:

```go
// auth/tokensource.go (sketch)

package auth

import (
    "context"
    "time"
)

// Token is the credential delivered to the wire.
type Token struct {
    Value     string        // scheme-specific credential (bearer token or Basic payload)
    Type      string        // Authorization scheme: "Bearer" (default), "Basic", …
    ExpiresAt time.Time     // absolute expiry; zero value = "no expiry / unknown"
    Scope     string        // space-separated scope grant (informational; not enforced by SDK)
    Issuer    string        // issuer URL the token was minted by (for audit / disambiguation)
}

// TokenSource returns a token suitable for the next outgoing request.
// Implementations MUST:
//   - Refresh transparently when ExpiresAt is near or past (REQ-063).
//   - Coalesce concurrent refresh attempts (REQ-026).
//   - Honour ctx for cancellation and deadlines (REQ-020).
type TokenSource interface {
    Token(ctx context.Context) (Token, error)
}
```

Rules:

- Every authenticated request path **MUST** acquire its bearer through a `TokenSource`. No package outside `auth/<provider>/` may construct a `Token` directly.
- `Token.Value` is opaque to `transport/`; transports forward it as `Authorization: <Type> <Value>` without inspection. When `Type` is empty, `transport/` treats it as `Bearer`.
- A `TokenSource` **MAY** be stateful (caching, refresh) but **MUST** be safe for concurrent use (REQ-026).

### Provider sub-packages (REQ-012)

`auth/` does not contain provider implementations. Each provider is a sub-package:

| Sub-package | Grant | Audience |
|---|---|---|
| `auth/smart/` | SMART-on-openEHR (Authorization Code + PKCE + launch) | Interactive end-user app on top of a SMART-on-openEHR EHR / CDR |
| `auth/clientcreds/` | OAuth2 Client Credentials | Service-to-service callers (benchmark, seeder, MCP server, federator backend) |
| `auth/jwtbearer/` | OAuth2 JWT Bearer (RFC 7523) | Systems holding a signed assertion (e.g. trusted intermediaries) |
| `auth/basic/` | HTTP Basic (RFC 7617) on openEHR REST | Deployments that accept a static username/password per request (dev, legacy gateways) |

Additional providers (plain OIDC, session-cookie) **MAY** be added as further sub-packages without changing the `TokenSource` contract.

### Per-request TokenSource

Some use cases — most notably an MCP server forwarding an incoming caller's token — need to attach a per-request `TokenSource` rather than configuring one at client-construction time:

```go
// auth/context.go (sketch)
func WithTokenSource(ctx context.Context, ts TokenSource) context.Context
func TokenSourceFromContext(ctx context.Context) (TokenSource, bool)
```

The `transport/` package **MUST** check the context for a per-request `TokenSource` and prefer it over the client-default `TokenSource` when present. This **MUST** be documented in `transport/` and `auth/`.

## SMART flows

### REQ-068 — Flow and launch-mode coverage

The platform supports four SMART grant flows and three launch modes. The SDK **MUST** cover all of them across the `auth/<provider>/` family:

| Flow | Provider | Use |
|---|---|---|
| Authorization Code + **PKCE** (public clients) | `auth/smart` | Interactive end-user app, no client secret stored on device |
| Authorization Code + **client_secret** (confidential web apps) | `auth/smart` (same flow, secret-based client auth) | Server-rendered web app holding a server-side secret |
| **Client Credentials** (backend services) | `auth/clientcreds` | Service-to-service callers (benchmark, seeder, MCP server backend) |
| **JWT Bearer** (confidential clients with asymmetric keys) | `auth/jwtbearer` | Systems holding a signed assertion |

#### JWT Bearer — client_assertion signing algorithms (Phase 3a)

The HL7 SMART `client-confidential-asymmetric` profile states that clients **SHALL** support **RS384** and **ES384** for signing `client_assertion` JWTs at the token endpoint. The SDK's `auth/jwtbearer.ClaimsSigner` implements this baseline:

| Algorithm | Key type | Status |
|---|---|---|
| **RS384** | RSA (`*rsa.PrivateKey`) | **default** — SMART baseline |
| **ES384** | ECDSA P-384 (`*ecdsa.PrivateKey`) | supported — SMART baseline |
| RS256 | RSA (`*rsa.PrivateKey`) | supported — back-compat only |
| ES256 | ECDSA P-256 (`*ecdsa.PrivateKey`) | supported — common in practice |

The default algorithm is **RS384** (changed from RS256 in Phase 3a). Callers that previously relied on the RS256 default must pass `WithAlgorithm("RS256")` explicitly if they require RS256.

All signing is delegated to `github.com/go-jose/go-jose/v4`, which handles JOSE encoding including ECDSA r‖s byte-padding. Hand-rolled PKCS1v15/ECDSA paths have been removed.

Key-type validation is enforced at `NewClaimsSigner` construction time and returns `auth.ErrInvalidConfig` on mismatch (e.g. ES384 with an RSA key). Opaque `crypto.Signer` implementations (e.g. KMS/HSM handles) are supported for both RSA and ECDSA: a non-concrete signer is wrapped at signing time with `go-jose/v4`'s `cryptosigner.Opaque`, which handles the JOSE encoding (including ECDSA r‖s) — no concrete-key requirement.

#### Authorization Code with asymmetric client auth — `private_key_jwt` (Phase 3b, F-C)

The HL7 SMART `client-confidential-asymmetric` profile lets a confidential client authenticate the authorization-code token exchange with a **signed `client_assertion`** (RFC 7523 / RFC 7521 `private_key_jwt`) instead of a shared `client_secret`. This is preferred over `client_secret_basic` because no symmetric secret is transmitted to the token endpoint.

`auth/smart` enables this via `WithClientAssertionKey(signer crypto.Signer, alg, kid string)`. When configured, the code exchange (and refresh) **MUST**:

- Send form fields `client_assertion_type=urn:ietf:params:oauth:client-assertion-type:jwt-bearer` and a freshly signed `client_assertion`.
- **Omit** the HTTP Basic `Authorization` header (no `client_secret_basic`).

The assertion is produced by reusing `auth/jwtbearer.ClaimsSigner` (the same RS384-default signer as the JWT Bearer flow above) with `iss = sub = client_id`, `aud = token_endpoint`, and an auto-generated unique `jti` and short `exp` (default 5 minutes). The signing algorithm and `kid` are caller-supplied; key/alg mismatches are rejected at construction with `auth.ErrInvalidConfig`.

Client-authentication method selection is **deterministic** (no trial-and-error):

| Configuration | Method | Wire effect |
|---|---|---|
| `WithClientAssertionKey` set | `private_key_jwt` | signed `client_assertion` form fields; no Basic header |
| `WithClientSecret` set (only) | `client_secret_basic` | HTTP Basic header |
| neither | public client | no client authentication |

Configuring **both** an assertion key and a client secret is ambiguous and is rejected at construction with `auth.ErrInvalidConfig`.

##### G-3 — discovery-driven method cross-check

When the authorization server advertises `token_endpoint_auth_methods_supported` (RFC 8414), `FromConfig` cross-checks the method implied by the configured credential against that list. If the list is non-empty and does **not** contain the configured method (`private_key_jwt` when a signer is set, `client_secret_basic` when only a secret is set), construction fails fast with `auth.ErrInvalidConfig` rather than deferring the failure to a rejected token request. When the list is empty or absent, the check is skipped (the server has not constrained the method).

#### Backend Services asymmetric client auth — `client_credentials` + `client_assertion` (Phase 3c, F-C)

The HL7 SMART [Backend Services](https://hl7.org/fhir/smart-app-launch/backend-services.html) profile specifies that a backend service authenticates at the token endpoint with:

- `grant_type=client_credentials`
- `client_assertion_type=urn:ietf:params:oauth:client-assertion-type:jwt-bearer`
- A freshly signed `client_assertion` JWT (RFC 7523)
- **No** HTTP Basic `Authorization` header and **no** `client_secret`

`auth/clientcreds` implements this via `WithClientAssertion(src jwtbearer.AssertionSource)`. When configured, `fetch` calls `src.Assertion(ctx)` on every token exchange, adds the two `client_assertion*` form fields, and omits Basic auth and `client_secret`. Signing errors are wrapped as `auth.ErrTokenExchangeFailed` with the message prefix `"client_assertion signing: ..."`.

**Distinction from `auth/jwtbearer`:** `auth/jwtbearer` implements the separate RFC 7523 _JWT Bearer Token Grant_ (`grant_type=urn:ietf:params:oauth:grant-type:jwt-bearer`) — the JWT is the _authorization grant_ itself. `auth/clientcreds` with `WithClientAssertion` uses `grant_type=client_credentials` — the JWT is the _client authentication credential_. Both use `jwtbearer.AssertionSource` / `jwtbearer.ClaimsSigner` for signing.

**Configuration rules** (enforced at `FromConfig`):

| Configuration | Behaviour |
|---|---|
| `WithClientAssertion` set, no `ClientSecret` | `private_key_jwt` — signed `client_assertion` form fields; no Basic header |
| `ClientSecret` set (only) | `client_secret_basic` (default) or `client_secret_post` |
| Both `ClientSecret` and `WithClientAssertion` | Rejected with `auth.ErrInvalidConfig` (ambiguous) |
| Neither | Rejected with `auth.ErrInvalidConfig` (no credentials) |

### Launch modes

Three launch modes the SDK **MUST** support — each is a way the SMART flow starts:

| Mode | Description |
|---|---|
| **Standalone** | The SDK initiates the launch by redirecting the user to the authorization endpoint. No EHR-side launch parameter. |
| **Embedded** (iFrame) | The SDK is launched from inside an EHR or portal that has already authenticated the user; the EHR provides a `launch` parameter that the SDK forwards to the authorization endpoint to obtain launch context. |
| **Backend service** | No user interaction. Uses Client Credentials or JWT Bearer. No launch context. |

The launch mode is determined by configuration at construction time and **MAY** also be derived per call (e.g. an MCP server that accepts both standalone and embedded launches from different transports).

## SMART-on-openEHR

### REQ-061 — PKCE flow

`auth/smart` **MUST** implement the SMART App Launch flow with PKCE (RFC 7636), adapted for openEHR-specific scope syntax and launch context.

The flow (standalone launch, summarised):

1. **Discovery.** Fetch the SMART configuration document from the deployment's well-known URL (see [service-discovery.md](service-discovery.md)). Extract `authorization_endpoint`, `token_endpoint`, `jwks_uri`, `registration_endpoint` (if dynamic registration is used), and `scopes_supported`.
2. **PKCE pair.** Generate a `code_verifier` (cryptographically random, 43–128 chars per RFC 7636) and derive `code_challenge` = `S256(code_verifier)`.
3. **Authorization request.** Redirect the user to `authorization_endpoint` with `response_type=code`, `client_id`, `redirect_uri`, `scope` (openEHR-formatted, e.g. `<compartment>/<resource>.<permission>`), `aud` (the openEHR REST base or an explicit audience identifier), `state`, `code_challenge`, `code_challenge_method=S256`, plus SMART-specific `launch` parameter if EHR-launch.
4. **Authorization response.** Receive the `code` and `state` at the redirect URI. The SDK **MUST** verify the `state` matches the value sent in step 3.
5. **Token exchange.** POST to `token_endpoint` with `grant_type=authorization_code`, `code`, `redirect_uri`, `client_id`, `code_verifier`. Receive `access_token`, `refresh_token` (if granted), `expires_in`, `scope`, plus SMART-specific `patient`, `encounter`, `id_token`, etc.
6. **Launch context capture.** Surface the SMART launch parameters to the application via `smart/` (see § Launch context below).
7. **Use.** Subsequent requests carry the access token as `Authorization: Bearer …`; the SDK's `TokenSource` implementation refreshes transparently (REQ-063).

The PKCE implementation **MUST**:

- Use `S256` as the challenge method; `plain` is prohibited.
- Generate cryptographically random verifiers (`crypto/rand`).
- **Generate or validate OAuth `state`.** When the application calls `BeginAuthorization` with an empty `state`, the SDK **MUST** generate a cryptographically random value (minimum 32 bytes before base64url encoding). When exchanging the authorization code, the SDK **MUST** verify that the callback `state` equals the value sent in step 3 **before** any token-endpoint call; mismatch **MUST** return `ErrLaunchInvalidState`.

### REQ-062 — JWKS rotation

#### Algorithm allowlists (surface-only in v0.8)

The SMART discovery resolver surfaces two algorithm-selection lists onto `AuthEndpoints` (REQ-070):

- **`TokenEndpointAuthSigningAlgValuesSupported`** (`token_endpoint_auth_signing_alg_values_supported`) — the JWS algorithms the authorization server accepts for client-assertion JWTs at the token endpoint (e.g. `["RS384","ES384"]`). Phase 3b client-credential selection logic will read this list to choose a signing algorithm; in v0.8 the field is populated but not yet consumed.
- **`IDTokenSigningAlgValuesSupported`** (`id_token_signing_alg_values_supported`) — the JWS algorithms used to sign ID tokens (e.g. `["RS256","ES384"]`). ID-token verification (REQ-064) consumes this list as the verification allowlist when present (see _ID-token verification algorithm agility_ below). `TokenEndpointAuthSigningAlgValuesSupported` remains surface-only in v0.8 (Phase 3b client-credential alg selection).

The SDK validates ID tokens (and, in some deployments, opaque access tokens via introspection or signature verification) against the deployment's published JWKS. JWKS rotation **MUST** be handled:

- The JWKS document **MUST** be fetched on first use and cached.
- The cache **MUST** honour a documented TTL (default: 5 minutes).
- On a verification miss (`kid` not in cache), the SDK **MUST** refresh the JWKS once before reporting the verification as failed. This handles silent rotation by the authorization server.
- The refresh path **MUST** coalesce concurrent attempts (REQ-026).

#### ID-token verification algorithm agility (REQ-062, REQ-064) — landed in Phase 3e

`smart.ValidateIDToken` verifies the `id_token` signature against the deployment's JWKS and then applies the SDK's claim semantics. Signature verification is delegated to **`github.com/coreos/go-oidc/v3`** (which uses `go-jose/v4`); the SDK does **not** hand-roll signature verification or JWK→key parsing.

- **Supported algorithms:** `RS256`, `RS384`, `ES256`, `ES384`. RS384/ES384 are the HL7 SMART asymmetric baseline; RS256/ES256 cover the widely deployed remainder. Both RSA and ECDSA keys published in the JWKS are honoured.
- **Allowlist:** the caller passes the deployment's `id_token_signing_alg_values_supported` (via `smart.WithIDTokenSigningAlgs` / `ValidateConfig.AllowedIDTokenAlgs`). When non-empty it is intersected with the supported set — the discovery list can narrow but never widen the SDK's support, and an empty intersection falls back to the full supported set rather than go-oidc's RS256-only default. When the caller passes nothing, the full supported set applies.
- **Rejected:** the unsecured `none` algorithm is always rejected (explicitly, and because it is never in the allowlist); any algorithm outside the effective allowlist is rejected; an `alg`/key-type mismatch is rejected by go-jose key matching. All rejections surface as `auth.ErrJWKSValidationFailed` (preserved sentinel — `errors.Is` keeps working).
- **Verify-before-claims:** the signature is verified before any claim is trusted (inherent to go-oidc). The SDK then re-applies its stricter claim semantics via `claimsFromMap`: `iss`/`aud`/`exp`/`nbf`/`iat` with a **30-second** clock skew (`clockSkew`) plus the required `nonce` match. The returned `*IDTokenClaims` shape is unchanged.

#### RFC 7662 token introspection client (F-J) — opt-in, resource-server scope — landed in Phase 5b

The `auth/introspect` package provides a standalone, opt-in RFC 7662 token introspection client. It is a **resource-server / MCP-gateway concern**, not wired into the default `auth/smart` client path — reference SMART client SDKs deliberately omit introspection (it is not a client-side operation). Consumers acting as resource servers that need to validate opaque access tokens at runtime can use it independently.

**Standards:** [RFC 7662 — OAuth 2.0 Token Introspection](https://www.rfc-editor.org/rfc/rfc7662) and the [HL7 SMART App Launch token-introspection profile](https://www.hl7.org/fhir/smart-app-launch/token-introspection.html).

**Construction.** `introspect.New(endpoint string, httpClient *http.Client, opts ...Option) (*Client, error)` — injects the `*http.Client` (REQ-021; nil is rejected with `auth.ErrInvalidConfig`); validates that `endpoint` is a non-empty, parseable absolute URL (also `auth.ErrInvalidConfig` on failure). The `introspection_endpoint` URL is surfaced from the authorization server's discovery document via `smart/discovery` (see REQ-070 / `AuthEndpoints.IntrospectionEndpoint`) and can be passed directly.

**Introspection call.** `(*Client).Introspect(ctx context.Context, token string, bearer string) (Result, error)` — POSTs `token=<value>` form-encoded to the endpoint (RFC 7662 §2.1) with `Authorization: Bearer <bearer>` (the resource server authenticates using its own access credential). `ctx` is threaded (REQ-020). An `{"active":false}` response is a **successful** introspection — returned as `(Result{Active:false}, nil)`; inactive tokens are **not** treated as errors. Non-2xx responses are returned as a wrapped `*auth.ExchangeError` (sentinel `auth.ErrTokenExchangeFailed`; `OAuth2` field populated when the body matches RFC 6749 §5.2).

**`Result` fields (RFC 7662 §2.2).** `Active bool` (required). Optional/conditional: `Scope`, `ClientID`, `Username`, `TokenType`, `Exp`/`Iat`/`Nbf` (RFC 7662 numeric dates parsed to `time.Time` from the float64 JSON number), `Sub`, `Aud` (string or JSON array — array values joined with a space), `Iss`, `Jti`. SMART/openEHR launch-context extras when present: `Patient`, `FHIRUser` (`fhirUser`), `EHRID` (`ehrId`), `EpisodeID` (`episodeId`). `Raw map[string]any` carries the complete decoded body including vendor-extension claims.

### REQ-063 — Token refresh

**Requesting a refresh token.** The authorization server grants a `refresh_token` only when the authorization request includes the appropriate offline-access scope. Per HL7 SMART App Launch v2 "Scopes and Launch Context" ([https://hl7.org/fhir/smart-app-launch/](https://hl7.org/fhir/smart-app-launch/)):

- Include `offline_access` in the scope list to request a refresh token that persists beyond the current browser session.
- Include `online_access` to request a refresh token scoped to the current online session only.

The SDK provides `auth.ScopeOfflineAccess` and `auth.ScopeOnlineAccess` constants for composing these scope strings via `auth.JoinScopes`. These constants are lexical only — whether the server honours the request depends on the deployment's policy.

The `auth/smart` `TokenSource` **MUST** transparently refresh access tokens when:

- `ExpiresAt` is within a configurable threshold (default: 30 seconds) of `time.Now()`.
- A `401 Unauthorized` response is received from the wire with an indication that the token has expired.

Refresh uses the stored `refresh_token` against the deployment's `token_endpoint` (`grant_type=refresh_token`). On refresh failure:

- A re-authentication-required signal **MUST** be surfaced to the consumer (typed error or callback — to be designed).
- The expired token **MUST NOT** be used silently.

If no `refresh_token` is available (the deployment did not grant one), the `TokenSource` **MUST** return a typed error directing the consumer to restart the launch flow.

#### Implementation — Phase 4a + 4b (Source/error side + transport hook)

**Terminal vs. transient refresh classification (`ExchangeError.Terminal()`).**
`auth.ExchangeError` exposes a `Terminal() bool` method. A failure is terminal when the HTTP status is 4xx **and** the OAuth2 error code is `invalid_grant` or `invalid_client`. All other failures (5xx, network, context, unparsed) are transient and return `false`. The distinction drives the F-L refresh-clearing rule below.

**F-L: clear refresh token only on terminal failure.**
When a `refresh_token` grant fails, `Source.Token` classifies the returned `*auth.ExchangeError`:

- **Terminal** (`invalid_grant` / `invalid_client` with 4xx) → clear `s.refresh` and `s.cur`, then return `ErrReauthRequired`. A subsequent `Token()` call will short-circuit to `ErrReauthRequired` without issuing another POST.
- **Transient** (5xx, network, ctx) → retain `s.refresh` and `s.cur`, return `ErrRefreshFailed`. The consumer may retry; the refresh token is still valid.

Both state mutations happen under `s.mu` (the same mutex used by all of `Token`).

**Configurable early-expiry buffer (G-2).**
`WithRefreshThreshold(d time.Duration)` sets the proactive-refresh window (default: 30 seconds). A token is considered stale — and `Token()` will attempt a refresh — when `time.Until(ExpiresAt) <= RefreshThreshold`. This is the sole configurable early-expiry buffer; no duplicate option exists.

**`RefreshIfNeeded(ctx context.Context) error`.**
A non-request-bound refresh trigger: checks `staleLocked()` and whether a refresh token is present; if both are true it calls through to `Token()` to execute the refresh; otherwise it is a no-op returning `nil`. Error contract is identical to `Token()`.

**`Reauther` interface (`auth.Reauther`).**
```go
// auth/reauth.go
type Reauther interface {
    Reauth(ctx context.Context) error
}
```
`*smart.Source` implements `Reauther`. `Reauth(ctx)` forces a refresh regardless of the current token's freshness by zeroing `s.cur` and calling `Token()`. It applies the same F-L terminal/transient classification as a regular refresh failure.

**`ReautherFunc` adapter.**
```go
// auth/reauth.go
type ReautherFunc func(ctx context.Context) error
func (f ReautherFunc) Reauth(ctx context.Context) error { return f(ctx) }
```
`ReautherFunc` lets a closure — for example a discovery-catalog-refresh function (REQ-071 bullet 3) — satisfy `Reauther` without importing `smart/discovery` into `transport/`.

**Transport-layer opt-in 401→reauth safety net (Phase 4b, F-D).**
`transport.WithReauthOn401(r auth.Reauther)` installs an opt-in safety net. When a wire `401` is received:

1. If a `Reauther` is configured **and** this `Do` call has not yet reauthed, `transport/` calls `r.Reauth(ctx)` exactly once.
2. If `Reauth` returns nil, the request is retried once. The retry re-acquires the token via `tokenSourceFor` — now pointing at the refreshed credential.
3. If the retry also returns `401`, `transport.ErrUnauthorized` is surfaced. If `Reauth` itself returns an error, that error (wrapped) is surfaced. In either case the loop does not repeat.

A per-`Do` boolean guards against infinite loops; `Reauth` is called at most once per `Do` invocation regardless of retry policy.

When `WithReauthOn401` is **not** set, the existing contract is unchanged: a wire `401` returns `transport.ErrUnauthorized` immediately after one upstream call.

This hook is a **complementary safety net** — proactive expiry-based refresh in `Source.Token()` before the request is issued remains the primary mechanism. The hook covers the residual window where a token expires between the proactive-refresh check and the wire round-trip.

The retry fires for **all HTTP methods**, including non-idempotent writes (`POST`/`PUT`). This is safe because a `401` means the request was rejected at the authentication layer and therefore **not processed** by the resource — re-driving it once after refreshing the credential cannot double-apply a write. A `401` arising from insufficient *scope* (rather than token expiry) will simply `401` again and surface `transport.ErrUnauthorized` after the single retry (one wasted round-trip, no harm). Deployments that signal authorization failures with `403` (reserving `401` for authentication/expiry) get the cleanest behaviour.

Out of scope (v1 implementation status):

- `auth/clientcreds` and `auth/jwtbearer` do not implement `Reauther`; they have no refresh path. Callers using those providers may wire a custom `ReautherFunc` closure.
- MTLS, FAPI, JAR/PAR — out of v1 scope.

### REQ-064 — Launch context

The application-level `smart/` package **MUST** expose the SMART launch context as typed values:

```go
// smart/context.go (sketch)

package smart

import "context"

type LaunchContext struct {
    // FHIR-compat launch-context claims (SMART App Launch §7.1).
    Patient     string         // SMART "patient" launch parameter — opaque to SDK
    Encounter   string         // SMART "encounter" launch parameter
    User        string         // SMART "fhirUser" / openEHR equivalent
    Scopes      []string       // granted scopes (post-token-exchange)
    IDToken     *IDTokenClaims // parsed ID-token claims (sub, aud, iss, iat, exp, custom)
    Issuer      string         // deployment issuer URL

    // openEHR-native launch-context claims, per the openEHR SMART App Launch spec
    // (https://specifications.openehr.org/releases/ITS-REST/development/smart_app_launch.html).
    EHRID     string // "ehrId" token claim — EHR-level context, requested via "launch/patient"
    EpisodeID string // "episodeId" token claim — Episode context (experimental), via "launch/episode"

    // SMART-compat extras surfaced by reference SMART clients.
    Intent            string // "intent" — suggested workflow for the app
    SMARTStyleURL     string // "smart_style_url" — EHR style sheet URL
    NeedPatientBanner *bool  // "need_patient_banner" — nil: server silent, caller shows banner; non-nil: server's explicit value
    Tenant            string // "tenant" — multi-tenant EHR deployment identifier

    Raw         map[string]any // verbatim token-response payload for custom claims
}

func WithLaunchContext(ctx context.Context, lc *LaunchContext) context.Context
func LaunchContextFromContext(ctx context.Context) (*LaunchContext, bool)
```

Consumers **MUST NOT** be required to parse JWT claims by hand. `IDTokenClaims` carries the standard claims plus a typed map for deployment-extension claims; the SDK validates the signature, exp, iss, aud, nonce as part of the token exchange. Signature verification supports RS256/RS384/ES256/ES384 and is constrained by the deployment's advertised `id_token_signing_alg_values_supported` when supplied — see _ID-token verification algorithm agility_ under REQ-062.

The `EHRID` and `EpisodeID` fields are populated from the `ehrId` and `episodeId` token claims defined in the canonical openEHR SMART App Launch specification. The SMART-compat extras (`Intent`, `SMARTStyleURL`, `NeedPatientBanner`, `Tenant`) are populated when present. All of these fields are also available untyped via `Raw`.

## Platform principal claims

### REQ-067

When the token-endpoint response or ID token carries platform-issued principal claims, the SDK **MUST** surface them on `LaunchContext` (or the equivalent for non-SMART providers) verbatim:

```go
type LaunchContext struct {
    // ... fields from REQ-064 above ...

    // Platform-issued principal claims (when present; nil when absent).
    Principal *PrincipalIdentity
}

type PrincipalIdentity struct {
    UID  string // tenant-scoped internal principal identifier (e.g. "principal_uid" claim)
    Type PrincipalType
    // Raw lets the consumer reach further claims without SDK churn.
    Raw map[string]any
}

type PrincipalType string

const (
    PrincipalTypePerson PrincipalType = "PERSON"
    PrincipalTypeAgent  PrincipalType = "AGENT"
    PrincipalTypeUnknown PrincipalType = "" // claim absent or unrecognised
)
```

Rules:

- The SDK **MUST NOT** coerce the principal type — if the claim is missing or carries an unrecognised value, `Type` is `PrincipalTypeUnknown` and consumers handle it.
- The SDK **MUST NOT** invent a `Principal` value when no claim is present — `LaunchContext.Principal` is `nil` in that case.
- Claim names (`principal_uid`, `principal_type`) are configurable via `smart.WithPrincipalClaimNames(...)` when building a `LaunchContext`. Principal claims are read from the validated ID token when present, otherwise from the token-endpoint JSON body (`TokenResponse.Raw`).

## AI caller attribution

### REQ-066

When the SDK is consumed by an AI-facing surface (MCP server, agent integration), the consumer **MAY** want to record AI-mediated provenance on outgoing requests — which agent identifier acted, which model provider was involved, the upstream trace context.

The SDK **MUST** provide an opt-in carriage path for this metadata. Two equivalent shapes:

```go
// Functional option at client construction.
client, _ := ehr.New(catalog,
    transport.WithCallerAttribution(transport.CallerAttribution{
        AgentID:       "mcp-claude-code/1.2.0",
        ModelProvider: "anthropic",
        // additional opaque attributes
        Attributes: map[string]string{"orchestrator": "ralph-loop"},
    }),
)

// Per-request via context (preferred for MCP servers handling diverse calls).
ctx = transport.WithCallerAttribution(ctx, transport.CallerAttribution{...})
```

Transport carriage:

- The SDK **MUST** emit the attribution metadata as both:
  - A configurable HTTP header (default: `X-Cadasto-Caller-Attribution`, value: JSON-encoded), and
  - OTel span attributes (`caller.agent_id`, `caller.model_provider`, `caller.*`).
- The mechanism **MUST** be **opt-in** — no defaults are sent automatically.
- The SDK **MUST NOT** include personally identifying claims about the *user* in the attribution metadata; PII flows through the existing token / claim path, not through caller attribution.

This is the SDK's contribution to platform-side audit. The platform decides what to do with the metadata.

## Per-client binding

### REQ-065

Each SDK client instance **MUST** bind to exactly one issuer and therefore one tenant context:

- Discovery cache (`smart/discovery`) is keyed by issuer.
- `TokenSource` is per-client (or per-request via ctx, REQ-060).
- Connection pool, retry budget, OTel spans are per-client.

Multi-issuer / multi-tenant fan-out is achieved by constructing **one client per issuer**. The SDK **MUST NOT** internally multiplex issuers behind a single client. This matters most for the federator use case ([use-cases.md § Federative API client](use-cases.md#federative-api-client)).

## HTTP Basic on openEHR REST

### REQ-069

Some openEHR REST deployments (development CDRs, legacy gateways, internal tools) authenticate API calls with **HTTP Basic** — a static username and password on every request, not an OAuth2 access token.

`auth/basic` **MUST** implement `auth.TokenSource` for this case:

```go
// auth/basic/basic.go (sketch)

package basic

// New returns a TokenSource that always yields Type "Basic" and Value set to the
// base64-encoded "username:password" payload per RFC 7617.
func New(username, password string) (*Source, error)
```

Rules:

- `New` **MUST** reject an empty username with `auth.ErrInvalidConfig`. Password **MAY** be empty when the deployment allows it.
- `Token()` **MUST** return `auth.Token{Type: "Basic", Value: <base64(user-pass)>}` with no expiry (`ExpiresAt` zero) — there is no token exchange or refresh.
- The implementation **MUST** be safe for concurrent use (REQ-026) and honour `context.Context` cancellation (REQ-020).
- `transport/` **MUST** emit `Authorization: Basic <Value>` when `Token.Type` is `Basic` (already satisfied by the generic `Authorization: <Type> <Value>` rule in REQ-060).
- `auth/basic` **MUST NOT** perform OAuth2 token-endpoint calls; it is unrelated to `client_secret_basic` on the token endpoint (see `auth/clientcreds`).

Consumers wire Basic auth at client construction:

```go
ts, _ := basic.New("service", os.Getenv("OPENEHR_PASSWORD"))
client, _ := transport.New(catalog, transport.WithTokenSource(ts), ...)
```

Per-request override via `auth.WithTokenSource(ctx, ts)` **MUST** work the same as for Bearer providers (REQ-060).

## Client Credentials and JWT Bearer providers

`auth/clientcreds` and `auth/jwtbearer` are simpler — no interactive flow, no launch context. They:

- Implement `auth.TokenSource`.
- Accept configuration via functional options (REQ-022): client ID, client secret (or signing key), token endpoint, scope, audience.
- Coalesce concurrent refreshes (REQ-026).
- Map wire-level auth errors onto the `transport/` error hierarchy.

These providers **MUST** support the same JWKS rotation behaviour as `auth/smart` when they need to validate issued tokens (typically less common — service-to-service callers often accept opaque tokens).

## Scope handling

The SDK **MUST NOT** enforce, parse, or validate scope strings as application policy — that is the deployment's responsibility. The SDK **MUST**:

- Pass scope strings verbatim from configuration through to the authorization request.
- Round-trip the granted scope from the token response back to the application via `LaunchContext.Scopes`.
- Provide a small helper (`auth.BuildScope(compartment, resource, permission)`) for composing openEHR-formatted scopes (`<compartment>/<resource>.<permission>`) without forcing the application to template strings.

## Error mapping

Auth errors **MUST** surface as typed sentinels. The shared classes live in
package `auth` (`auth/errors.go`); the SMART-launch-specific state-mismatch
sentinel lives in package `smart` (`auth/smart/errors.go`):

```go
// package auth — shared across all providers
var (
    ErrInvalidConfig        = errors.New("auth: invalid configuration")
    ErrTokenExchangeFailed  = errors.New("auth: token exchange failed")
    ErrRefreshFailed        = errors.New("auth: token refresh failed")
    ErrReauthRequired       = errors.New("auth: re-authentication required")
    ErrJWKSValidationFailed = errors.New("auth: JWKS validation failed")
)

// package smart — SMART App Launch specific
var ErrLaunchInvalidState = errors.New("SMART launch: state mismatch")
```

A PKCE `code_verifier` mismatch is **not** a separate client-side sentinel: the
verifier is sent to the token endpoint, and a mismatch is rejected **server-side**,
surfacing as `auth.ErrTokenExchangeFailed`. Token-exchange and refresh failures
are wrapped in `*auth.ExchangeError`, which carries the HTTP `StatusCode`, the
parsed RFC 6749 `OAuth2` envelope, and a `Terminal()` predicate (4xx
`invalid_grant`/`invalid_client`/`invalid_token`) that drives refresh-token
clearing (REQ-063, F-L).

Consumers detect classes via `errors.Is`. The underlying wire error is preserved via `errors.Unwrap`.

## What is NOT in scope here

- **OAuth2 dynamic client registration** — the SDK consumes a pre-registered client. Dynamic registration **MAY** be added as a helper in `smart/` later.
- **App-side credential storage** — token storage (encrypted at rest, OS keychain, browser cookie) is application-side.
- **Refresh-token revocation** — the deployment owns revocation policy; the SDK reacts to the resulting wire errors.
- **MTLS, FAPI, JAR / PAR** — out of v1 scope; **MAY** be addressed by future provider sub-packages.

## Coverage matrix

| Spec | REQ | Lives in |
|---|---|---|
| TokenSource interface | REQ-060 | `auth/` |
| Provider layering | REQ-012 | `auth/<provider>/` sub-packages |
| Per-request TokenSource | (REQ-020 ctx + REQ-060) | `auth/context.go`, consumed by `transport/` |
| SMART PKCE flow | REQ-061 | `auth/smart/` |
| JWKS rotation | REQ-062 | `auth/smart/`, optionally `auth/clientcreds/`, `auth/jwtbearer/` |
| Token refresh | REQ-063 | `auth/smart/` (primary), `auth/<provider>/` (as applicable) |
| Launch context | REQ-064 | `smart/` |
| Per-client / tenant binding | REQ-065 | `auth/<provider>/`, `smart/discovery/` |
| AI caller attribution | REQ-066 | `transport/`, `auth/context.go` |
| Platform principal claims | REQ-067 | `auth/smart/`, `smart/` |
| Flow + launch-mode coverage | REQ-068 | `auth/smart/`, `auth/clientcreds/`, `auth/jwtbearer/` |
| HTTP Basic on openEHR REST | REQ-069 | `auth/basic/`, consumed by `transport/` |
