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

The SDK validates ID tokens (and, in some deployments, opaque access tokens via introspection or signature verification) against the deployment's published JWKS. JWKS rotation **MUST** be handled:

- The JWKS document **MUST** be fetched on first use and cached.
- The cache **MUST** honour a documented TTL (default: 5 minutes).
- On a verification miss (`kid` not in cache), the SDK **MUST** refresh the JWKS once before reporting the verification as failed. This handles silent rotation by the authorization server.
- The refresh path **MUST** coalesce concurrent attempts (REQ-026).

### REQ-063 — Token refresh

The `auth/smart` `TokenSource` **MUST** transparently refresh access tokens when:

- `ExpiresAt` is within a configurable threshold (default: 30 seconds) of `time.Now()`.
- A `401 Unauthorized` response is received from the wire with an indication that the token has expired.

Refresh uses the stored `refresh_token` against the deployment's `token_endpoint` (`grant_type=refresh_token`). On refresh failure:

- A re-authentication-required signal **MUST** be surfaced to the consumer (typed error or callback — to be designed).
- The expired token **MUST NOT** be used silently.

If no `refresh_token` is available (the deployment did not grant one), the `TokenSource` **MUST** return a typed error directing the consumer to restart the launch flow.

#### Transport-layer 401 → re-auth (Invalidatable)

A `TokenSource` **MAY** implement the optional `auth.Invalidatable` capability (a single `Invalidate()` method that drops any cached token). When the active source implements it, `transport/` **MUST**, on a `401 Unauthorized` for an authenticated request, invalidate the source and retry the request **exactly once** with a freshly acquired token, outside the retry budget (so a disabled retry policy still recovers). A source that does not implement `Invalidatable` (e.g. `StaticTokenSource`) surfaces the `401` to the consumer unchanged.

This recovers the case a source **cannot** self-detect: an access token minted without an `expires_in` hint has a zero `ExpiresAt` and is therefore never treated as stale by proactive refresh, yet may have expired server-side. `auth/clientcreds` implements `Invalidatable`; `Invalidate()` clears the cached token so the next `Token()` performs a fresh client-credentials exchange.

Out of scope (v1 implementation status):

- **`auth/smart` refresh-token surfacing:** the typed re-authentication-required signal on `refresh_token`-grant failure is still to be designed; `auth/smart` does not yet auto-refresh via its stored `refresh_token` on a wire 401. The transport-layer `Invalidatable` re-auth above covers re-acquirable grants (`auth/clientcreds`); `auth/jwtbearer` may adopt `Invalidatable` when its re-assertion path lands.

### REQ-064 — Launch context

The application-level `smart/` package **MUST** expose the SMART launch context as typed values:

```go
// smart/context.go (sketch)

package smart

import "context"

type LaunchContext struct {
    Patient     string         // SMART "patient" launch parameter — opaque to SDK
    Encounter   string         // SMART "encounter" launch parameter
    User        string         // SMART "fhirUser" / openEHR equivalent
    Scopes      []string       // granted scopes (post-token-exchange)
    IDToken     *IDTokenClaims // parsed ID-token claims (sub, aud, iss, iat, exp, custom)
    Issuer      string         // deployment issuer URL
    Raw         map[string]any // verbatim token-response payload for custom claims
}

func WithLaunchContext(ctx context.Context, lc *LaunchContext) context.Context
func LaunchContextFromContext(ctx context.Context) (*LaunchContext, bool)
```

Consumers **MUST NOT** be required to parse JWT claims by hand. `IDTokenClaims` carries the standard claims plus a typed map for deployment-extension claims; the SDK validates the signature, exp, iss, aud, nonce as part of the token exchange.

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

Auth errors **MUST** surface as typed values:

```go
var (
    ErrLaunchInvalidState     = errors.New("SMART launch: state mismatch")
    ErrLaunchPKCEMismatch     = errors.New("SMART launch: PKCE verifier mismatch")
    ErrTokenExchangeFailed    = errors.New("SMART launch: token exchange failed")
    ErrRefreshFailed          = errors.New("SMART launch: token refresh failed")
    ErrReauthRequired         = errors.New("SMART launch: re-authentication required")
    ErrJWKSValidationFailed   = errors.New("auth: JWKS validation failed")
)
```

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
