---
kind: specification
---

# Authentication

**Status:** Draft

Normative contract for the `auth/` package family and the application-level `smart/` package. Covers REQ-060 through REQ-069 and REQ-165.

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
- **HL7 FHIR SMART App Launch v2.2** — [https://hl7.org/fhir/smart-app-launch/](https://hl7.org/fhir/smart-app-launch/) — defines the PKCE flow, scopes and launch context (including `offline_access`, `online_access`, `launch`, `launch/patient`), client-confidential-asymmetric (`private_key_jwt`), Backend Services, and JWKS rotation.

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
- `Token.Value` is opaque to `transport/`: transports **MUST** forward it as `Authorization: <Type> <Value>` without inspecting either half — there is no scheme allowlist, so a scheme this SDK has no provider for (`DPoP`, …) reaches the wire verbatim. When `Type` is empty, `transport/` **MUST** treat it as `Bearer`. A **zero** `Token` (no value and no type — `auth.Token.IsZero`) is the anonymous case and **MUST** suppress the `Authorization` header entirely rather than emit an empty scheme or an empty credential; `auth.AnonymousTokenSource` is the sanctioned way to ask for it, and it is `transport/`'s default token source.
- A `TokenSource` **MAY** be stateful (caching, refresh) but **MUST** be safe for concurrent use (REQ-026).

The coalesced refresh in `auth/smart` is race-free: the goroutine that refreshes stores the token before it closes the channel the other callers wait on, so under the Go memory model every waiter reads the stored result. Discovery resolution (`smart/discovery`) coalesces the same way.

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

The SDK **MUST** cover every flow in this table, and the three launch modes below, across the `auth/<provider>/` family. The first three rows are SMART App Launch flows; the last two serve authorization servers outside the SMART asymmetric profile:

| Flow | Provider | Use |
|---|---|---|
| Authorization Code + **PKCE** (public clients) | `auth/smart` | Interactive end-user app, no client secret stored on device |
| Authorization Code + **PKCE** + client authentication (confidential web apps) | `auth/smart` (same flow, with `client_secret` or a signed client assertion) | Server-rendered web app holding a server-side secret or key |
| **SMART Backend Services**: Client Credentials + signed client assertion | `auth/clientcreds` (`WithClientAssertion`) | Service-to-service callers (benchmark, seeder, MCP server backend) |
| **Client Credentials** + `client_secret` | `auth/clientcreds` | Service-to-service callers of an authorization server that accepts a shared secret, outside the SMART asymmetric profile |
| **JWT Bearer authorization grant** (RFC 7523 §2.1) | `auth/jwtbearer` | Systems holding an assertion issued by a trusted party; not a SMART flow |

For a confidential client, `auth/smart` **MUST** send the PKCE `code_challenge` and `code_verifier` as well as its client authentication: HL7 SMART App Launch requires PKCE from every app, and PKCE does not replace client authentication. The openEHR SMART specification's Flow Recommendations present the two as alternatives; this SDK follows HL7 SMART.

The openEHR SMART specification names a "JWT Bearer Token Grant" as the preferred flow for backend services. HL7 SMART App Launch Backend Services, which that specification builds on, uses the `client_credentials` grant with an RFC 7523 §2.2 client assertion, and that is the flow `auth/clientcreds` with `WithClientAssertion` implements. The RFC 7523 §2.1 authorization grant in `auth/jwtbearer` is a separate flow for deployments that issue authorization assertions; it is not SMART Backend Services.

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

The assertion is produced by reusing `auth/jwtbearer.ClaimsSigner` (the same RS384-default signer as the JWT Bearer flow above) with `iss = sub = client_id`, `aud = token_endpoint`, and an auto-generated unique `jti` and short `exp` (default 5 minutes). The signing algorithm and `kid` are caller-supplied; key/alg mismatches are rejected at construction with `auth.ErrInvalidConfig`. The HL7 SMART asymmetric profile requires a `kid` header and an `exp` at most five minutes after issue, so `WithClientAssertionKey` with an empty `kid` **MUST** fail construction with `auth.ErrInvalidConfig`, and the assertion's lifetime **MUST NOT** exceed five minutes. When the authorization server advertises `token_endpoint_auth_signing_alg_values_supported`, the configured algorithm **MUST** be in it, or construction fails with `auth.ErrInvalidConfig`; an absent or empty list is not constraining.

Client-authentication method selection is **deterministic** (no trial-and-error):

| Configuration | Method | Wire effect |
|---|---|---|
| `WithClientAssertionKey` set | `private_key_jwt` | signed `client_assertion` form fields; no Basic header |
| `WithClientSecret` set (only) | `client_secret_basic` (default) or `client_secret_post` | HTTP Basic header, or `client_id`/`client_secret` form fields |
| neither | public client | no client authentication |

For a symmetric secret the SDK defaults to `client_secret_basic`. When the server advertises `token_endpoint_auth_methods_supported` but **not** `client_secret_basic`, and **does** advertise `client_secret_post`, the SDK falls back to `client_secret_post` (credentials in the form body) so deployments that accept only the post-style method still work.

Configuring **both** an assertion key and a client secret is ambiguous and is rejected at construction with `auth.ErrInvalidConfig`.

A confidential client **MUST NOT** send `client_id` as a form field in the code exchange or the refresh, except as part of the `client_secret_post` credential; a public client **MUST** send it (HL7 SMART App Launch token request: `client_id` is "required for public apps" and confidential apps omit it, because they authenticate).

##### G-3 — discovery-driven method cross-check

When the authorization server advertises `token_endpoint_auth_methods_supported` (RFC 8414), `FromConfig` cross-checks the method implied by the configured credential against that list. If the list is non-empty and does **not** contain the resolved method (`private_key_jwt` when a signer is set; for a symmetric secret, `client_secret_basic` by default or `client_secret_post` when only the post method is advertised), construction fails fast with `auth.ErrInvalidConfig` rather than deferring the failure to a rejected token request. When the list is empty or absent, the check is skipped (the server has not constrained the method).

#### Backend Services asymmetric client auth — `client_credentials` + `client_assertion` (Phase 3c, F-C)

The HL7 SMART [Backend Services](https://hl7.org/fhir/smart-app-launch/backend-services.html) profile specifies that a backend service authenticates at the token endpoint with:

- `grant_type=client_credentials`
- `client_assertion_type=urn:ietf:params:oauth:client-assertion-type:jwt-bearer`
- A freshly signed `client_assertion` JWT (RFC 7523)
- **No** HTTP Basic `Authorization` header and **no** `client_secret`

`auth/clientcreds` implements this via `WithClientAssertion(src jwtbearer.AssertionSource)`. When configured, `fetch` calls `src.Assertion(ctx)` on every token exchange, adds the two `client_assertion*` form fields, and omits Basic auth and `client_secret`. Signing errors are wrapped as `auth.ErrTokenExchangeFailed` with the message prefix `"client_assertion signing: ..."`.

`auth/jwtbearer` **MUST** provide `NewClientAssertion(clientID, tokenURL string, signer crypto.Signer, alg, kid string) (*ClaimsSigner, error)`, which builds the assertion source of the HL7 SMART asymmetric profile: `iss = sub = clientID`, `aud = tokenURL`, `typ: JWT`, a unique `jti`, and an `exp` five minutes after issue. It **MUST** fail with `auth.ErrInvalidConfig` when any argument is empty or the key does not fit `alg`. `auth/smart` builds its own client assertion with it.

##### Backend Services from a resolved catalog

`auth/clientcreds` **MUST** provide `NewFromCatalog(catalog, clientID, clientSecret, opts...)` (the secret empty when a client assertion is configured) for SMART Backend Services against a resolved catalog. It **MUST** post to the token endpoint in `catalog.Auth.TokenEndpoint`, and **MUST** fail with `auth.ErrInvalidConfig` when the catalog is nil or names no token endpoint. It **MUST** record `catalog.Issuer` on the tokens it produces, unless the caller passes `WithIssuer`, whose issuer then wins. When the catalog advertises the corresponding list, construction **MUST** fail with `auth.ErrInvalidConfig` when:

- `grant_types_supported` does not contain `client_credentials`;
- `token_endpoint_auth_methods_supported` does not contain the configured method (`private_key_jwt` with a client assertion, `client_secret_basic` or `client_secret_post` with a secret);
- `token_endpoint_auth_signing_alg_values_supported` does not contain the algorithm of a client assertion produced by the SDK's own `jwtbearer.ClaimsSigner` (an assertion source the SDK cannot inspect is not checked).

An absent or empty list **MUST NOT** fail construction, as in § G-3. `NewFromCatalog` **MUST NOT** choose the client-assertion signing algorithm from `token_endpoint_auth_signing_alg_values_supported`: a `jwtbearer.ClaimsSigner` signs with the algorithm it was built with.

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
| **Backend service** | No user interaction. Uses SMART Backend Services (`auth/clientcreds` with a client assertion); the RFC 7523 §2.1 grant in `auth/jwtbearer` serves deployments outside SMART. No launch context. |

The launch mode is determined by configuration at construction time and **MAY** also be derived per call (e.g. an MCP server that accepts both standalone and embedded launches from different transports).

## SMART-on-openEHR

### REQ-061 — PKCE flow

`auth/smart` **MUST** implement the SMART App Launch flow with PKCE (RFC 7636), adapted for openEHR-specific scope syntax and launch context.

The flow (standalone launch, summarised):

1. **Discovery.** Fetch the SMART configuration document from the Platform base URL's well-known URL (see [service-discovery.md](service-discovery.md)). Extract `authorization_endpoint`, `token_endpoint`, `jwks_uri`, `registration_endpoint` (if dynamic registration is used), and `scopes_supported`.
2. **PKCE pair.** Generate a `code_verifier` (cryptographically random, 43–128 chars per RFC 7636) and derive `code_challenge` = `S256(code_verifier)`.
3. **Authorization request.** Redirect the user to `authorization_endpoint` with `response_type=code`, `client_id`, `redirect_uri`, `scope` (for openEHR resources, the [REQ-165](#req-165--openehr-scope-syntax) shape `<compartment>/<resource>-<pattern>.<permissions>`, e.g. `patient/composition-*.rs`), `aud` (the Platform base URL, which is the `iss` of an embedded launch, or an explicit audience identifier), `state`, `code_challenge`, `code_challenge_method=S256`, plus SMART-specific `launch` parameter if EHR-launch.
4. **Authorization response.** Receive the redirect at the redirect URI and complete it with `CompleteAuthorization` (§ Completing the authorization below), which checks `state`, the RFC 9207 `iss` and an error response before any token-endpoint call.
5. **Token exchange.** POST to `token_endpoint` with `grant_type=authorization_code`, `code`, `redirect_uri`, `code_verifier`, and `client_id` for a public client (a confidential client authenticates instead; § REQ-068). Receive `access_token`, `refresh_token` (if granted), `expires_in`, `scope`, plus SMART-specific `patient`, `encounter`, `id_token`, etc.
6. **Launch context capture.** Surface the SMART launch parameters to the application via `smart/` (see § Launch context below).
7. **Use.** Subsequent requests carry the access token as `Authorization: Bearer …`; the SDK's `TokenSource` implementation refreshes transparently (REQ-063).

The PKCE implementation **MUST**:

- Use `S256` as the challenge method; `plain` is prohibited.
- Generate cryptographically random verifiers (`crypto/rand`).
- **Generate or validate OAuth `state`.** When the application calls `BeginAuthorization` with an empty `state`, the SDK **MUST** generate a cryptographically random value (minimum 32 bytes before base64url encoding). When exchanging the authorization code, the SDK **MUST** verify that the callback `state` equals the value sent in step 3 **before** any token-endpoint call; mismatch **MUST** return `ErrLaunchInvalidState`.
- **Refuse a server that cannot verify `S256`.** When the authorization server advertises `code_challenge_methods_supported` and the list does not contain `S256`, constructing the `auth/smart` source **MUST** fail with `auth.ErrInvalidConfig` (HL7 SMART App Launch requires servers to support `S256`; RFC 9700 §2.1.1 makes PKCE support detectable from this metadata). An absent or empty list **MUST NOT** fail construction, since a hand-built catalog commonly omits it.

The authorization request **MUST** carry `aud` (HL7 SMART App Launch lists it as required; the openEHR SMART specification does not define its value). A source built from a resolved catalog **MUST** default `aud` to the catalog's `BaseURL` when the caller sets no audience; constructing a source that has no audience at all **MUST** fail with `auth.ErrInvalidConfig`.

When the configured scopes contain `openid`, `BeginAuthorization` **MUST** generate a nonce from at least 32 random bytes (base64url-encoded), record it on the returned `AuthorizationRequest`, and `AuthorizeURL` **MUST** send it as `nonce` (OpenID Connect Core 1.0 §3.1.2.1); without `openid`, `AuthorizeURL` **MUST NOT** send a `nonce`. `BeginAuthorization` **MUST** also record on the request the issuer the source is bound to (its `Issuer`, the OIDC issuer from discovery), so each authorization response is checked against the authorization server the user was sent to (RFC 9700 §4.4, the mix-up defence a client of several Platforms owes).

When `AuthorizeURL` is given a `launch` value, the request's `scope` **MUST** contain `launch` (HL7 SMART App Launch: an app launched from an EHR requests the `launch` scope); the SDK **MUST** add it when the configured scopes lack it.

#### Completing the authorization

`auth/smart` **MUST** provide `(*Source).CompleteAuthorization(ctx, callback url.Values, req AuthorizationRequest)`, taking the query the redirect URI received and the request the launch started with. A request without `State` or a PKCE verifier **MUST** fail with `auth.ErrInvalidConfig` before any check, so an empty state never matches an empty state. A callback that repeats `state`, `iss`, `code` or `error` **MUST** fail with `ErrAuthorizationRejected` (RFC 6749 §3.1 forbids a repeated response parameter). Otherwise it **MUST** apply these checks in this order and make no token-endpoint call until all of them pass:

1. `state` **MUST** equal `req.State`; otherwise the call fails with `ErrLaunchInvalidState`.
2. An empty `iss` value **MUST** be treated as absent. When the callback carries an `iss` (RFC 9207) or the authorization server advertises `authorization_response_iss_parameter_supported: true`, a request whose `Issuer` is empty **MUST** fail with `auth.ErrInvalidConfig`, not `ErrLaunchIssuerMismatch`, because there is nothing to compare with. Otherwise the callback's `iss` **MUST** equal `req.Issuer` exactly, and a callback without `iss` where the server advertises the parameter **MUST** be refused (RFC 9207 §2.4); either failure is `ErrLaunchIssuerMismatch`.
3. When the callback carries `error` (RFC 6749 §4.1.2.1), the call **MUST** fail with an error that `errors.Is` matches to `ErrAuthorizationRejected` and from which `errors.As` extracts an `*auth.OAuth2Error` holding `error`, `error_description` and `error_uri`. A callback with neither `error` nor `code` **MUST** fail with `ErrAuthorizationRejected` as well.
4. The code is then exchanged exactly as `ExchangeAuthorizationCode` exchanges it, including the ID-token rules of § REQ-064.

#### Embedded launch

`auth/smart` **MUST** provide `ParseEHRLaunch(query url.Values, allow func(iss string) bool) (EHRLaunch, error)`, which reads the `iss` and `launch` parameters a Launcher appends to the app's launch URL. It **MUST** fail with `ErrLaunchInvalidRequest` when `iss` is missing or not an absolute URL with a host, or `launch` is missing, without consulting `allow`. Otherwise it **MUST** fail with `ErrLaunchIssuerNotAllowed` when `allow` is nil or returns false for `iss`, so a client never resolves discovery for a Platform it has not chosen to trust. `EHRLaunch.Issuer` is the Platform base URL to resolve ([service-discovery.md § REQ-070](service-discovery.md#req-070)); `EHRLaunch.Launch` is passed unchanged to `AuthorizeURL`.

### REQ-062 — JWKS rotation

#### Algorithm allowlists

The SMART discovery resolver surfaces two algorithm-selection lists onto `AuthEndpoints` (REQ-070):

- **`TokenEndpointAuthSigningAlgValuesSupported`** (`token_endpoint_auth_signing_alg_values_supported`) — the JWS algorithms the authorization server accepts for client-assertion JWTs at the token endpoint (e.g. `["RS384","ES384"]`). `auth/clientcreds.NewFromCatalog` checks client assertions against this list under [§ Backend Services from a resolved catalog](#backend-services-from-a-resolved-catalog).
- **`IDTokenSigningAlgValuesSupported`** (`id_token_signing_alg_values_supported`) — the JWS algorithms used to sign ID tokens (e.g. `["RS256","ES384"]`). ID-token verification (REQ-064) consumes this list as the verification allowlist when present (see _ID-token verification algorithm agility_ below).

The SDK validates ID tokens against the deployment's published JWKS. JWKS rotation **MUST** be handled:

- The JWKS document **MUST** be fetched on first use and cached.
- The cache **MUST** honour a documented TTL (default: 5 minutes).
- On a verification miss (`kid` not in cache), the SDK **MUST** refresh the JWKS once before reporting the verification as failed. This handles silent rotation by the authorization server.
- The refresh path **MUST** coalesce concurrent attempts (REQ-026).
- A key published without a `kid` **MUST** be kept. When an ID token's header carries no `kid`, the SDK **MUST** verify it with the set's only signing key when the set holds exactly one key whose `use`, if present, is `sig`, and **MUST** reject the token otherwise (OpenID Connect Core 1.0 §10.1 lets an issuer omit `kid` only when its set holds one key).

#### ID-token verification algorithm agility (REQ-062, REQ-064) — landed in Phase 3e

`auth/smart.ValidateIDToken` verifies the `id_token` signature against the deployment's JWKS and then applies the SDK's claim semantics. It lives beside the token exchange so the exchange and the refresh can verify the ID token they receive (§ REQ-064); `smart.ValidateIDToken` and `smart.IDTokenClaims` **MUST** remain as the same function and type for existing callers. Signature verification is delegated to **`github.com/coreos/go-oidc/v3`** (which uses `go-jose/v4`); the SDK does **not** hand-roll signature verification or JWK→key parsing.

- **Supported algorithms:** the SDK **MUST** support `RS256`, `RS384`, `ES256` and `ES384`, and **MUST NOT** treat any other algorithm as supported. RS384/ES384 are the HL7 SMART asymmetric baseline; RS256/ES256 cover the widely deployed remainder. Both RSA and ECDSA keys published in the JWKS are honoured.
- **Allowlist:** the caller passes the deployment's `id_token_signing_alg_values_supported` (via `smart.WithIDTokenSigningAlgs` / `ValidateConfig.AllowedIDTokenAlgs`). A non-empty allowlist **MUST** be intersected with the supported set: it can narrow the SDK's support and **MUST NOT** widen it. An empty intersection (the deployment advertises only algorithms the SDK does not support) **MUST** fail closed with `auth.ErrJWKSValidationFailed` before the JWKS is fetched, with no fallback to the full supported set. With no allowlist, the full supported set **MUST** apply.
- **Rejected:** the SDK **MUST** reject the unsecured `none` algorithm in any letter case, even when the allowlist names it, and **MUST** reject any algorithm outside the effective allowlist. An `alg`/key-type mismatch is rejected by go-jose key matching. Every rejection decided on the token itself (its segments, header, algorithm, key, signature or claims), here and under the claim rules below, **MUST** surface as an error that `errors.Is` matches to `auth.ErrJWKSValidationFailed`. A missing issuer, client ID or JWKS **MUST** surface as `auth.ErrInvalidConfig`, and a failed JWKS fetch **MUST** surface as the fetch error. Such a failure **MUST NOT** match `auth.ErrJWKSValidationFailed`, so an outage never reads as a bad token.
- **Verify-before-claims:** the SDK **MUST** verify the signature before it trusts any claim; the claim checks read only the verified payload. `claimsFromMap` then applies the SDK's claim rules. `iss` **MUST** equal the configured issuer exactly, with no URL normalisation (OIDC Core §3.1.3.7), and `aud` **MUST** contain the client ID. When `aud` lists any other audience, the token **MUST** be rejected unless each of them is in the caller's trusted set, empty by default (OIDC Core 1.0 §3.1.3.7 step 3); when the token carries `azp`, it **MUST** equal the client ID. A token without `exp`, or without a non-empty string `sub`, **MUST** be rejected, since OIDC Core 1.0 §2 requires both claims. A token **MUST** be rejected when its `exp` is 30 seconds (`clockSkew`) or more before the validation time, or when its `nbf` or `iat`, if present, is more than 30 seconds after it. When the caller supplies a nonce, the `nonce` claim **MUST** equal it.

`IDTokenClaims.Nonce` **MUST** hold the token's own `nonce` claim, empty when the token has none, whether or not the caller expected one.

### REQ-063 — Token refresh

**Requesting a refresh token.** The authorization server grants a `refresh_token` only when the authorization request includes the appropriate offline-access scope. Per HL7 FHIR SMART App Launch v2 "Scopes and Launch Context" ([https://hl7.org/fhir/smart-app-launch/](https://hl7.org/fhir/smart-app-launch/)):

- Include `offline_access` in the scope list to request a refresh token that persists beyond the current browser session.
- Include `online_access` to request a refresh token scoped to the current online session only.

The SDK provides `auth.ScopeOfflineAccess` and `auth.ScopeOnlineAccess` constants for composing these scope strings via `auth.JoinScopes`. These constants are lexical only — whether the server honours the request depends on the deployment's policy.

The `auth/smart` `TokenSource` **MUST** proactively refresh access tokens when `ExpiresAt` is within a configurable threshold (default: 30 seconds) of `time.Now()`.

Wire-driven refresh — recovering from a `401 Unauthorized` that indicates an expired/invalid token — is **opt-in** rather than automatic inside the `TokenSource`: the transport layer exposes `transport.WithReauthOn401`, which on a 401 invokes the configured `auth.Reauther` (implemented by `*smart.Source.Reauth`) once and replays the request. Keeping it opt-in avoids surprising retries of non-idempotent writes (see _transport reauth hook_ below).

Refresh uses the stored `refresh_token` against the deployment's `token_endpoint` (`grant_type=refresh_token`). On refresh failure:

- A re-authentication-required signal **MUST** be surfaced to the consumer as the typed sentinel `auth.ErrReauthRequired` (wrapped in `*auth.ExchangeError`).
- The expired token **MUST NOT** be used silently.

If no `refresh_token` is available (the deployment did not grant one), the `TokenSource` **MUST** return a typed error directing the consumer to restart the launch flow.

**Token changes.** `auth/smart` **MUST** provide the option `WithTokenChange(func(ctx context.Context, change TokenChange))`. The source **MUST** call it after every successful code exchange and refresh, once it holds the new tokens and outside its lock, with the new access token, the refresh token it now holds (the previous one when the response carried none) and the token response; it **MUST NOT** call it for a failed exchange. An application that keeps a session across restarts stores a rotated refresh token from it: RFC 6749 §6 has the client discard the old refresh token when a new one is issued, and RFC 9700 §4.14 makes rotation one of the two ways a public client's refresh token is protected. `Revoke` (§ REQ-167) **MUST** call it once with an empty change after clearing the tokens.

```go
type TokenChange struct {
    Access       auth.Token
    RefreshToken string
    Response     TokenResponse
}
```

#### Implementation — Phase 4a + 4b (Source/error side + transport hook)

**Terminal vs. transient refresh classification (`ExchangeError.Terminal()`).**
`auth.ExchangeError` exposes a `Terminal() bool` method. A failure is terminal when the HTTP status is 4xx **and** the OAuth2 error code is `invalid_grant`, `invalid_client`, or `invalid_token`. All other failures (5xx, network, context, unparsed) are transient and return `false`. The distinction drives the F-L refresh-clearing rule below.

**F-L: clear refresh token only on terminal failure.**
When a `refresh_token` grant fails, `Source.Token` classifies the returned `*auth.ExchangeError`:

- **Terminal** (`invalid_grant` / `invalid_client` / `invalid_token` with 4xx) → clear `s.refresh` and `s.cur`, then return `ErrReauthRequired`. A subsequent `Token()` call will short-circuit to `ErrReauthRequired` without issuing another POST.
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
`*smart.Source` implements `Reauther`. When a `refresh_token` is present, `Reauth(ctx)` forces a refresh regardless of the current token's freshness by marking `s.cur` stale and calling `Token()`, applying the same F-L terminal/transient classification as a regular refresh failure. When **no** `refresh_token` is available there is nothing to exchange, so `Reauth` does not discard a cached token that is still within its `ExpiresAt` (a wire 401 may be scope-related, and a public client has no other credential); it returns `auth.ErrReauthRequired`, clearing the cached token only when it is already past `ExpiresAt`.

**`ReautherFunc` adapter.**
```go
// auth/reauth.go
type ReautherFunc func(ctx context.Context) error
func (f ReautherFunc) Reauth(ctx context.Context) error { return f(ctx) }
```
`ReautherFunc` lets a closure — for example a discovery-catalog-refresh function (REQ-071 bullet 3) — satisfy `Reauther` without importing `smart/discovery` into `transport/`.

**Transport-layer opt-in 401→reauth safety net (Phase 4b, F-D).**
`transport.WithReauthOn401(r auth.Reauther)` installs an opt-in safety net. When a wire `401` is received:

1. If a `Reauther` is configured, this `Do` call has not yet reauthed, **and** the response's Bearer challenge permits it under [transport.md § REQ-166](transport.md#req-166--bearer-challenge-on-401-and-403), `transport/` **MUST** call `r.Reauth(ctx)` exactly once.
2. If `Reauth` returns nil, the request **MUST** be retried once. The retry re-acquires the token via `tokenSourceFor` — now pointing at the refreshed credential.
3. If the retry also returns `401`, `transport.ErrUnauthorized` **MUST** be surfaced; if `Reauth` itself returns an error, that error, wrapped, **MUST** be surfaced and the request not retried. In either case the loop does not repeat.

A per-`Do` boolean guards against infinite loops; `Reauth` is called at most once per `Do` invocation regardless of retry policy.

When `WithReauthOn401` is **not** set, the existing contract is unchanged: a wire `401` returns `transport.ErrUnauthorized` immediately after one upstream call.

This hook is a **complementary safety net** — proactive expiry-based refresh in `Source.Token()` before the request is issued remains the primary mechanism. The hook covers the residual window where a token expires between the proactive-refresh check and the wire round-trip.

The retry **MUST** fire for **all HTTP methods**, including non-idempotent writes (`POST`/`PUT`), and repeat the request's method and body. This is safe because a `401` means the request was rejected at the authentication layer and therefore **not processed** by the resource — re-driving it once after refreshing the credential cannot double-apply a write; [REQ-166](transport.md#req-166--bearer-challenge-on-401-and-403) says which `401`s are re-driven. Deployments that signal authorization failures with `403` (reserving `401` for authentication/expiry) get the cleanest behaviour.

**Backend providers.** `auth/clientcreds` and `auth/jwtbearer` **MUST** implement `Reauther`: `Reauth` drops the cached access token and obtains a new one with a fresh exchange, coalesced with any exchange already in flight (REQ-026), and returns that exchange's error, if any. They have no refresh token; a new exchange is their only way to a new token.

Out of scope (v1 implementation status): MTLS, FAPI, JAR/PAR.

### REQ-167 — Token revocation

`auth/smart` **MUST** provide `(*Source).Revoke(ctx)` for signing out (RFC 7009). When the source's catalog advertises `revocation_endpoint`, `Revoke` **MUST** POST the form-encoded `token` with its `token_type_hint` (RFC 7009 §2.1): the refresh token when the source holds one (`refresh_token`), otherwise the access token (`access_token`). It **MUST** authenticate exactly as the token endpoint does (§ REQ-068), so a public client sends `client_id`.

Whatever the outcome, `Revoke` **MUST** then clear the source's access and refresh tokens, so a failed call never leaves a signed-out session usable, and **MUST** report the outcome: nil on a 200 response (RFC 7009 §2.2 answers 200 for an unknown or already invalid token too), otherwise an `*auth.ExchangeError` matching `auth.ErrRevocationFailed`. A source without a revocation endpoint **MUST** clear its tokens and return an error matching `auth.ErrInvalidConfig`. A source that holds no token **MUST** return nil without sending a request.

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

Consumers **MUST NOT** be required to parse JWT claims by hand. `IDTokenClaims` carries the standard claims plus a typed map for deployment-extension claims.

When a token response carries an `id_token`, `ExchangeAuthorizationCode`, and so `CompleteAuthorization`, **MUST** validate it before returning: against the source's JWKS, its `Issuer`, its client ID, the request's nonce and the discovery `id_token_signing_alg_values_supported` (OpenID Connect Core 1.0 §3.1.3.7). A source without a JWKS **MUST** fail such an exchange with `auth.ErrInvalidConfig` instead of returning an unverified ID token. Any ID-token failure at the exchange **MUST** surface as an `*auth.ExchangeError` whose sentinel is `auth.ErrTokenExchangeFailed`, wrapping the cause so its § REQ-062 class (`auth.ErrJWKSValidationFailed`, `auth.ErrInvalidConfig` or the key-set fetch error) stays reachable with `errors.Is`. A code exchange whose response carries no `refresh_token` **MUST NOT** keep a refresh token from an earlier session. The verified claims **MUST** be returned on `TokenResponse.IDTokenClaims`, and `LaunchContextFromTokenResponse` **MUST** use them without validating again. Given an `id_token` without verified claims, it **MUST** validate that token itself as § REQ-062 describes, with the nonce, signing algorithms and trusted audiences its caller passes.

A refresh response that carries an `id_token` **MUST** be validated the same way except for the nonce; when the source has verified an earlier ID token, the new one's `iss` and `sub` **MUST** equal that token's and its `aud` **MUST** list the same audiences, in any order (OpenID Connect Core 1.0 §12.2; RFC 7519 gives `aud` no order). A token refused on its own content **MUST** fail the refresh with an error that matches both `auth.ErrRefreshFailed` and `auth.ErrJWKSValidationFailed`; a configuration error or a failed key-set fetch **MUST** fail it with `auth.ErrRefreshFailed` and its own § REQ-062 class. Either way the source **MUST** keep its previous access and refresh tokens, including when the refresh was forced by `Reauth`. A refresh response that carries no `id_token` keeps the identity the earlier one established (OpenID Connect Core 1.0 §12.2 lets a server omit it): the source **MUST** carry the earlier verified claims onto its last token response's `IDTokenClaims`, so a `LaunchContext` rebuilt from `LastTokenResponse` keeps its `User` and `IDToken`.

`LaunchContext.User` **MUST** come from a verified ID token only: its `fhirUser` claim, else its `sub`. Without a verified ID token `User` **MUST** be empty. A `fhirUser` member in the token-endpoint body is not an identity claim and **MUST NOT** set it; `TokenResponse.FHIRUser` and `Raw` **MUST** still carry the member. Signature verification supports RS256/RS384/ES256/ES384 and is constrained by the deployment's advertised `id_token_signing_alg_values_supported` when supplied — see _ID-token verification algorithm agility_ under REQ-062.

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

Each SDK client instance **MUST** bind to exactly one Platform base URL and therefore one tenant context:

- Discovery cache (`smart/discovery`) entries are keyed as [service-discovery.md § REQ-071](service-discovery.md#req-071) requires; the OIDC issuer the document names is a property of the entry.
- `TokenSource` is per-client (or per-request via ctx, REQ-060).
- Connection pool, retry budget, OTel spans are per-client.

Multi-Platform / multi-tenant fan-out is achieved by constructing **one client per Platform base URL**. The SDK **MUST NOT** internally multiplex Platforms behind a single client. This matters most for the federator use case ([use-cases.md § Federative API client](use-cases.md#federative-api-client)).

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

### REQ-165 — openEHR scope syntax

The SDK **MUST NOT** apply scope strings as application policy: which scopes to grant, and whether a granted scope covers a request, is decided by the authorization server and the resource server. Building or reading the syntax of one scope token is not policy. The SDK **MUST**:

- Pass scope strings verbatim from configuration through to the authorization request.
- Round-trip the granted scope from the token response back to the application via `LaunchContext.Scopes`.
- Provide a small helper (`auth.BuildScope(compartment, resource, permission)`) for composing scopes of the shape `<compartment>/<resource>.<permission>` without templating strings. `BuildScope` is lexical: it **MUST NOT** validate its parts, so it serves SMART on FHIR scopes as well.
- Provide a typed builder and reader for the openEHR resource scopes of SMART on openEHR § Resource Scopes, `<compartment>/<resource>-<pattern>.<permissions>`:

```go
// auth/scope.go (sketch)

type OpenEHRScope struct {
    Compartment string // "patient", "user" or "system"
    Resource    string // "template", "composition" or "aql"
    Pattern     string // a template id or stored-query name, "*" wildcards allowed
    Permissions string // a non-empty subset of "cruds", in that order
}

var ErrInvalidScope = errors.New("auth: invalid scope")

func (s OpenEHRScope) Token() (string, error)
func ParseOpenEHRScope(token string) (OpenEHRScope, bool)
```

`Token` **MUST** return the single scope token, or an error that `errors.Is` matches to `auth.ErrInvalidScope` when any of these holds:

- the compartment is not `patient`, `user` or `system`, or the resource is not `template`, `composition` or `aql`;
- the permissions are empty, repeat a letter, use a letter outside `cruds`, or are out of the order `c`, `r`, `u`, `d`, `s` (HL7 SMART App Launch v2 permission syntax, which the openEHR scopes follow);
- the pattern is empty, or contains a character outside the RFC 6749 §3.3 scope-token set (`%x21 / %x23-5B / %x5D-7E`: printable ASCII other than space, `"` and `\`). A template id that contains a space or a non-ASCII character therefore cannot be written as a scope; the SDK **MUST NOT** escape or rewrite it.

`ParseOpenEHRScope` **MUST** read one token with the same grammar: the compartment ends at the first `/`, the permissions start after the last `.`, and the resource ends at the first `-`, so dotted template ids and query names (`org.openehr::bloodpressure.v1`) survive. It **MUST** return `false`, never an error, for a token that is not an openEHR resource scope, such as `openid`, `launch/patient` or the SMART on FHIR scope `patient/Observation.rs`. Neither function interprets the wildcards in a pattern; their matching rules belong to the authorization server.

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
    ErrInvalidScope         = errors.New("auth: invalid scope") // REQ-165
    ErrRevocationFailed     = errors.New("auth: token revocation failed")
)

// package smart — SMART App Launch specific
var (
    ErrLaunchInvalidState     = errors.New("SMART launch: state mismatch")
    ErrLaunchIssuerMismatch   = errors.New("SMART launch: authorization response issuer mismatch")
    ErrAuthorizationRejected  = errors.New("SMART launch: authorization rejected")
    ErrLaunchIssuerNotAllowed = errors.New("SMART launch: issuer not allowed")
    ErrLaunchInvalidRequest   = errors.New("SMART launch: invalid launch request")
)
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
- **Revocation policy** — when tokens expire or are revoked on the server is the deployment's decision; the SDK reacts to the resulting wire errors and offers only the client's own sign-out revocation (REQ-167).
- **MTLS, FAPI, JAR / PAR** — out of v1 scope; **MAY** be addressed by future provider sub-packages.
- **Token introspection (RFC 7662)** — a resource-server operation, and the SDK is a client of authorization servers. Discovery still surfaces `introspection_endpoint` (REQ-070), but the SDK ships no introspection client.

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
| openEHR scope syntax | REQ-165 | `auth/` |
