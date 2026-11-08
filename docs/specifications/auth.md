---
kind: specification
---

# Authentication and service discovery

**Status:** Draft

Normative contract for the `auth/` package family, the application-level `smart/` package and service discovery in `smart/discovery`. Covers REQ-060 to REQ-064, REQ-068, REQ-069, REQ-165 and REQ-167, and discovery's REQ-070 to REQ-073 ([§ Service discovery](#service-discovery)). REQ-065 and REQ-067 are retired, and caller attribution (REQ-066) is in [transport.md](transport.md#req-066--caller-attribution).

The SDK supports authenticated requests through a layered model:

```
Application
   └─→ smart/                          (application-level launch context)
   └─→ auth/<provider>/                (concrete provider — SMART, ClientCreds, JWTBearer, Basic)
                  └─→ auth/            (generic TokenSource + OAuth2 primitives)
```

The boundary between layers is **the generic `TokenSource` abstraction** — providers implement it; transports consume it; everything authenticated flows through it.

## Canonical sources

The SMART-on-openEHR authentication and discovery model in this SDK is derived from two primary specifications:

- **openEHR SMART App Launch** — [https://specifications.openehr.org/releases/ITS-REST/development/smart_app_launch.html](https://specifications.openehr.org/releases/ITS-REST/development/smart_app_launch.html) — defines openEHR-specific extensions: the SMART discovery document's shape for openEHR deployments with its `services` map, `launch/patient`, `launch/episode`, the `ehrId` and `episodeId` claims, and the `org.openehr.rest` service identifier.
- **HL7 FHIR SMART App Launch v2.2** — [https://hl7.org/fhir/smart-app-launch/](https://hl7.org/fhir/smart-app-launch/) — defines the `/.well-known/smart-configuration` endpoint and its metadata fields, the PKCE flow, scopes and launch context (including `offline_access`, `online_access`, `launch`, `launch/patient`), client-confidential-asymmetric (`private_key_jwt`), Backend Services, and JWKS rotation.

See also [ADR 0009](../adr/0009-smart-auth-library-scope.md) for the dependency and library-scope decisions underpinning this implementation.

## TokenSource contract

### REQ-060

`auth/` **MUST** define a `TokenSource` interface returning a token, an expiry, and a non-nil error or nil:

```go
// sketch
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
type TokenSource interface {
    Token(ctx context.Context) (Token, error)
}
```

Rules:

- Every authenticated request path acquires its bearer through a `TokenSource`, and within the SDK only the `auth/` packages build a `Token`.
- `Token.Value` is opaque to `transport/`: transports **MUST** forward it as `Authorization: <Type> <Value>` without inspecting either half — there is no scheme allowlist, so a scheme this SDK has no provider for (`DPoP`, …) reaches the wire verbatim. When `Type` is empty, `transport/` **MUST** treat it as `Bearer`. A **zero** `Token` (no value and no type — `auth.Token.IsZero`) is the anonymous case and **MUST** suppress the `Authorization` header entirely rather than emit an empty scheme or an empty credential; `auth.AnonymousTokenSource` is the sanctioned way to ask for it, and it is `transport/`'s default token source.
- A `TokenSource` **MAY** be stateful (caching, refresh) but **MUST** be safe for concurrent use (REQ-026).

Printing or logging a value of the `auth/` packages, of `smart`, of `smart/discovery` or of `transport` that holds a credential **MUST NOT** show the credential, with any `fmt` verb (`%v`, `%+v`, `%#v`, `%s` and `%d` included) or any `log/slog` handler. This covers `auth.Token` and the source `auth.StaticTokenSource` returns; `Config`, `TokenResponse` (with the token members of its `Raw`), `TokenChange`, `PKCEPair`, `AuthorizationRequest` and `*Source` in `auth/smart`; `Config` and `*Source` in `auth/clientcreds`; `Config`, `*ClaimsSigner` and `*Source` in `auth/jwtbearer`; `*Source` in `auth/basic`; `*LaunchContext` and `*PrincipalIdentity` in `smart` (with the token members of their `Raw`); `*Client` in `transport`, which shows only the base URL of its catalog, never its token source or reauther; and `*ServiceCatalog`, `ServiceEntry`, `AuthEndpoints` and `StaticConfig` in `smart/discovery`, whose credential is the password a URL they hold can carry. A credential that is set prints as `[redacted]`, an empty one as empty, and the password in any URL the type holds, in a field or in the `iss`, `fhirUser` or `smart_style_url` member of a `Raw`, is masked; a token member of a `smart` `Raw` prints as `[redacted]` whenever it is present. The other fields, such as the token type, expiry, scope, issuer, client ID and URLs, stay visible. `fmt` answers `%p` without calling a method, so `%p` of a value that is not a pointer prints its fields and is outside this rule. A `smart.LaunchContext` or `smart.PrincipalIdentity` value copied out of its pointer prints as the pointer does, but the `log/slog` JSON handler encodes its fields, tokens included, so logging such a value through that handler is outside this rule too: log the pointer. The JSON encoding of these types is unchanged and carries the credential, so an application can store a token, a `TokenChange` or an `AuthorizationRequest` and read it back.

The coalesced refresh in `auth/smart`, `auth/clientcreds` and `auth/jwtbearer` is race-free: the goroutine that refreshes stores the token before it closes the channel the other callers wait on, so under the Go memory model every waiter reads the stored result. When a waiter tries again is [idiom.md § Concurrency (REQ-026)](idiom.md#concurrency-req-026)'s rule.

#### Credential-bearing requests

The token, refresh and revocation requests of `auth/smart`, `auth/clientcreds` and `auth/jwtbearer` carry a credential, and they **MUST NOT** be redirected, whatever `CheckRedirect` the injected `HTTPClient` has, so a 307 or 308 cannot carry a refresh token, a client secret or a client assertion to another URL. A 3xx response **MUST** fail like any other non-2xx answer, with an `*auth.ExchangeError` that carries the status code and matches `auth.ErrTokenExchangeFailed`, `auth.ErrRefreshFailed` or `auth.ErrRevocationFailed`, as the request's own class says.

The SDK sends each of these requests with a copy of the injected client whose redirect policy returns a 3xx response instead of following it, and leaves the injected client as it is. The copy keeps the injected client's `Transport`, `Jar` and `Timeout`, so it stays within [REQ-021](idiom.md#http-client-injection-req-021).

#### Provider sub-packages

`auth/` does not contain provider implementations. Each provider is a sub-package, as [module-layout.md § Boundary rules](module-layout.md#boundary-rules) lays out:

| Sub-package | Grant | Audience |
|---|---|---|
| `auth/smart/` | SMART-on-openEHR (Authorization Code + PKCE + launch) | Interactive end-user app on top of a SMART-on-openEHR EHR / CDR |
| `auth/clientcreds/` | OAuth2 Client Credentials | Service-to-service callers (benchmark, seeder, MCP server, federator backend) |
| `auth/jwtbearer/` | OAuth2 JWT Bearer (RFC 7523) | Systems holding a signed assertion (e.g. trusted intermediaries) |
| `auth/basic/` | HTTP Basic (RFC 7617) on openEHR REST | Deployments that accept a static username/password per request (dev, legacy gateways) |

Further providers, such as plain OIDC or a session cookie, can be added as sub-packages without changing the `TokenSource` contract.

#### Per-request TokenSource

Some use cases — most notably an MCP server forwarding an incoming caller's token — need to attach a per-request `TokenSource` rather than configuring one at client-construction time:

```go
// package auth (sketch)
func WithTokenSource(ctx context.Context, ts TokenSource) context.Context
func TokenSourceFromContext(ctx context.Context) (TokenSource, bool)
```

The `transport/` package **MUST** check the context for a per-request `TokenSource` and prefer it over the client-default `TokenSource` when present.

#### Error mapping

Auth errors **MUST** surface as typed sentinels. The shared classes live in
package `auth`, and the SMART-launch sentinels in package `auth/smart`:

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
surfacing as `auth.ErrTokenExchangeFailed`. Token-exchange, refresh and revocation failures
are wrapped in `*auth.ExchangeError`, which carries the HTTP `StatusCode`, the
parsed RFC 6749 `OAuth2` envelope, and a `Terminal()` predicate (4xx
`invalid_grant`/`invalid_client`/`invalid_token`) that drives refresh-token
clearing (REQ-063).

Consumers detect classes with `errors.Is`, and read the status and the OAuth2 error with `errors.AsType[*auth.ExchangeError]`.

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

#### Client-assertion signing algorithms

The HL7 SMART `client-confidential-asymmetric` profile states that clients **SHALL** support **RS384** and **ES384** for signing `client_assertion` JWTs at the token endpoint. The SDK's `auth/jwtbearer.ClaimsSigner` implements this baseline:

| Algorithm | Key type | Status |
|---|---|---|
| **RS384** | RSA (`*rsa.PrivateKey`) | **default** — SMART baseline |
| **ES384** | ECDSA P-384 (`*ecdsa.PrivateKey`) | supported — SMART baseline |
| RS256 | RSA (`*rsa.PrivateKey`) | supported |
| ES256 | ECDSA P-256 (`*ecdsa.PrivateKey`) | supported |

`NewClaimsSigner` checks the key when it builds the signer and refuses with `auth.ErrInvalidConfig` a key whose type does not fit the algorithm, such as ES384 with an RSA key, and an RSA key under 1024 bits, which `crypto/rsa` refuses to sign with. An opaque `crypto.Signer`, such as a KMS or HSM handle, works for RSA and ECDSA alike. The SDK delegates the signing and its JOSE encoding, the ECDSA `r‖s` form included, to `github.com/go-jose/go-jose/v4`.

#### Authorization Code with asymmetric client auth — `private_key_jwt`

The HL7 SMART `client-confidential-asymmetric` profile lets a confidential client authenticate the authorization-code token exchange with a **signed `client_assertion`** (RFC 7523 / RFC 7521 `private_key_jwt`) instead of a shared `client_secret`. This is preferred over `client_secret_basic` because no symmetric secret is transmitted to the token endpoint.

`auth/smart` enables this via `WithClientAssertionKey(signer crypto.Signer, alg, kid string)`. When configured, the code exchange (and refresh) **MUST**:

- Send form fields `client_assertion_type=urn:ietf:params:oauth:client-assertion-type:jwt-bearer` and a freshly signed `client_assertion`.
- **Omit** the HTTP Basic `Authorization` header (no `client_secret_basic`).

The assertion is produced by reusing `auth/jwtbearer.ClaimsSigner`, the RS384-default signer of § Client-assertion signing algorithms, with `iss = sub = client_id`, `aud = token_endpoint`, and an auto-generated unique `jti` and short `exp` (default 5 minutes). The signing algorithm and `kid` are caller-supplied; key/alg mismatches are rejected at construction with `auth.ErrInvalidConfig`. The HL7 SMART asymmetric profile requires a `kid` header and an `exp` at most five minutes after issue, so `WithClientAssertionKey` with an empty `kid` **MUST** fail construction with `auth.ErrInvalidConfig`, and the assertion's lifetime **MUST NOT** exceed five minutes. When the authorization server advertises `token_endpoint_auth_signing_alg_values_supported`, the configured algorithm **MUST** be in it, or construction fails with `auth.ErrInvalidConfig` ([§ Advertised metadata lists](#advertised-metadata-lists)).

Client-authentication method selection is **deterministic** (no trial-and-error):

| Configuration | Method | Wire effect |
|---|---|---|
| `WithClientAssertionKey` set | `private_key_jwt` | signed `client_assertion` form fields; no Basic header |
| `WithClientSecret` set (only) | `client_secret_basic` (default) or `client_secret_post` | HTTP Basic header, or `client_id`/`client_secret` form fields |
| neither | public client | no client authentication |

For a symmetric secret the SDK defaults to `client_secret_basic`. When the server advertises `token_endpoint_auth_methods_supported` but **not** `client_secret_basic`, and **does** advertise `client_secret_post`, the SDK falls back to `client_secret_post` (credentials in the form body) so deployments that accept only the post-style method still work.

Configuring **both** an assertion key and a client secret is ambiguous and is rejected at construction with `auth.ErrInvalidConfig`.

A confidential client **MUST NOT** send `client_id` as a form field in the code exchange or the refresh, except as part of the `client_secret_post` credential; a public client **MUST** send it (HL7 SMART App Launch token request: `client_id` is "required for public apps" and confidential apps omit it, because they authenticate).

##### Method cross-check against discovery

When the authorization server advertises `token_endpoint_auth_methods_supported` (RFC 8414), `FromConfig` cross-checks the method implied by the configured credential against that list. If the list is non-empty and does **not** contain the resolved method (`private_key_jwt` when a signer is set; for a symmetric secret, `client_secret_basic` by default or `client_secret_post` when only the post method is advertised), construction fails fast with `auth.ErrInvalidConfig` rather than deferring the failure to a rejected token request. When the list is empty or absent, the check is skipped (the server has not constrained the method).

#### Backend Services asymmetric client auth — `client_credentials` + `client_assertion`

The HL7 SMART [Backend Services](https://hl7.org/fhir/smart-app-launch/backend-services.html) profile specifies that a backend service authenticates at the token endpoint with:

- `grant_type=client_credentials`
- `client_assertion_type=urn:ietf:params:oauth:client-assertion-type:jwt-bearer`
- A freshly signed `client_assertion` JWT (RFC 7523)
- **No** HTTP Basic `Authorization` header and **no** `client_secret`

`auth/clientcreds` implements this with `WithClientAssertion(src jwtbearer.AssertionSource)`: every token exchange asks `src` for a fresh assertion, adds the two `client_assertion*` form fields, and omits Basic auth and `client_secret`. A signing error fails the exchange with an error matching `auth.ErrTokenExchangeFailed`.

`auth/jwtbearer` **MUST** provide `NewClientAssertion(clientID, tokenURL string, signer crypto.Signer, alg, kid string) (*ClaimsSigner, error)`, which builds the assertion source of the HL7 SMART asymmetric profile: `iss = sub = clientID`, `aud = tokenURL`, `typ: JWT`, a unique `jti`, and an `exp` five minutes after issue. It **MUST** fail with `auth.ErrInvalidConfig`, never with a panic, when an argument is empty or the signer's key does not fit `alg`, and succeed for an `alg` of [§ Client-assertion signing algorithms](#client-assertion-signing-algorithms) whose key fits, for a signer of the caller's own type as for a standard-library key. For the signer, empty means nil, a nil RSA or ECDSA private key, or a signer whose `Public` method panics or reports no usable public key (nil, or an RSA or ECDSA public key without its modulus or curve). An RSA key under 1024 bits, the minimum `crypto/rsa` signs with, does not fit `RS256` or `RS384`; its size is read from the signer's public key, so an opaque signer is checked as a standard-library key is. `auth/smart` builds its own client assertion with it.

##### Backend Services from a resolved catalog

`auth/clientcreds` **MUST** provide `NewFromCatalog(catalog, clientID, clientSecret, opts...)` for SMART Backend Services against a resolved catalog, configured as the **Configuration rules** table below says, which rejects with `auth.ErrInvalidConfig` a client secret given together with a client assertion, and a configuration with neither. It **MUST** fail with `auth.ErrInvalidConfig` when the catalog is nil or names no token endpoint, and otherwise post to the token endpoint in `catalog.Auth.TokenEndpoint` and record `catalog.Issuer` on the tokens it produces, unless the caller passes `WithIssuer`, whose issuer then wins. When the catalog advertises the corresponding list, construction **MUST** fail with `auth.ErrInvalidConfig` when:

- `grant_types_supported` does not contain `client_credentials`;
- `token_endpoint_auth_methods_supported` does not contain the configured method (`private_key_jwt` with a client assertion, `client_secret_basic` or `client_secret_post` with a secret);
- `token_endpoint_auth_signing_alg_values_supported` does not contain the algorithm of a client assertion produced by the SDK's own `jwtbearer.ClaimsSigner`.

[§ Advertised metadata lists](#advertised-metadata-lists) says when a list counts as advertised and which client assertions the signing-algorithm list checks.

**Distinction from `auth/jwtbearer`:** `auth/jwtbearer` implements the separate RFC 7523 _JWT Bearer Token Grant_ (`grant_type=urn:ietf:params:oauth:grant-type:jwt-bearer`) — the JWT is the _authorization grant_ itself. `auth/clientcreds` with `WithClientAssertion` uses `grant_type=client_credentials` — the JWT is the _client authentication credential_. Both use `jwtbearer.AssertionSource` / `jwtbearer.ClaimsSigner` for signing.

**Configuration rules** (enforced at `FromConfig`):

| Configuration | Behaviour |
|---|---|
| `WithClientAssertion` set, no `ClientSecret` | `private_key_jwt` — signed `client_assertion` form fields; no Basic header |
| `ClientSecret` set (only) | `client_secret_basic` (default) or `client_secret_post` |
| Both `ClientSecret` and `WithClientAssertion` | Rejected with `auth.ErrInvalidConfig` (ambiguous) |
| Neither | Rejected with `auth.ErrInvalidConfig` (no credentials) |

#### Advertised metadata lists

Three checks compare the configuration with a list the authorization server advertises: the PKCE check of [§ REQ-061](#req-061--pkce-flow) (`code_challenge_methods_supported`), the signing-algorithm check of `WithClientAssertionKey` and the list checks of `NewFromCatalog` above. A list constrains construction only when it is present and not empty, since a hand-built catalog commonly omits these lists. The signing-algorithm list constrains only a client assertion that the SDK's own `jwtbearer.ClaimsSigner` produces, since the SDK cannot inspect the algorithm of any other `jwtbearer.AssertionSource`. For these three checks, an absent or empty list, and the signing-algorithm list for any other assertion source, **MUST NOT** fail construction. The `auth/smart` method cross-check ([§ Method cross-check against discovery](#method-cross-check-against-discovery)) reads its list the same way: it fails construction only on a non-empty list that lacks the resolved method. The SDK **MUST NOT** choose a signing algorithm from a list: a `jwtbearer.ClaimsSigner` signs with the algorithm it was built with.

### Launch modes

The SDK supports three launch modes ([§ REQ-068](#req-068--flow-and-launch-mode-coverage)), each a way the SMART flow starts:

| Mode | Description |
|---|---|
| **Standalone** | The SDK initiates the launch by redirecting the user to the authorization endpoint. No EHR-side launch parameter. |
| **Embedded** (iFrame) | The SDK is launched from inside an EHR or portal that has already authenticated the user; the EHR provides a `launch` parameter that the SDK forwards to the authorization endpoint to obtain launch context. |
| **Backend service** | No user interaction. Three confidential backend flows: `auth/clientcreds` with a client assertion (`client_credentials` + signed `client_assertion`, no Basic, no `client_secret` — SMART Backend Services), and, outside the SMART asymmetric profile, `auth/clientcreds` with a symmetric `client_secret` (HTTP Basic) and `auth/jwtbearer` (the RFC 7523 §2.1 JWT Bearer authorization grant, not a SMART flow). No launch context. |

The launch mode follows from the configuration at construction time. An application can also choose it per call, as an MCP server that accepts standalone and embedded launches from different transports does.

## SMART-on-openEHR

### REQ-061 — PKCE flow

`auth/smart` **MUST** implement the SMART App Launch flow with PKCE (RFC 7636), adapted for openEHR-specific scope syntax and launch context.

The flow (standalone launch, summarised):

1. **Discovery.** Fetch the SMART configuration document from the Platform base URL's well-known URL (see [§ Service discovery](#service-discovery)). Extract `authorization_endpoint`, `token_endpoint`, `jwks_uri`, `registration_endpoint` (if dynamic registration is used), and `scopes_supported`.
2. **PKCE pair.** Generate a `code_verifier` (cryptographically random, 43–128 chars per RFC 7636) and derive `code_challenge` = `S256(code_verifier)`.
3. **Authorization request.** Redirect the user to `authorization_endpoint` with `response_type=code`, `client_id`, `redirect_uri`, `scope` (for openEHR resources, the [REQ-165](#req-165--openehr-scope-syntax) shape `<compartment>/<resource>-<pattern>.<permissions>`, e.g. `patient/composition-*.rs`), `aud` (the Platform base URL, which is the `iss` of an embedded launch, or an explicit audience identifier), `state`, `code_challenge`, `code_challenge_method=S256`, plus SMART-specific `launch` parameter if EHR-launch.
4. **Authorization response.** Receive the redirect at the redirect URI and complete it with `CompleteAuthorization` (§ Completing the authorization below), which checks `state`, the RFC 9207 `iss` and an error response before any token-endpoint call.
5. **Token exchange.** POST to `token_endpoint` with `grant_type=authorization_code`, `code`, `redirect_uri`, `code_verifier`, and `client_id` for a public client (a confidential client authenticates instead; § REQ-068). Receive `access_token`, `refresh_token` (if granted), `expires_in`, `scope`, plus SMART-specific `patient`, `encounter`, `id_token`, etc.
6. **Launch context capture.** Surface the SMART launch parameters to the application via `smart/` (see § Launch context below).
7. **Use.** Subsequent requests carry the access token as `Authorization: Bearer …`; the SDK's `TokenSource` implementation refreshes transparently (REQ-063).

The PKCE implementation **MUST**:

- Use `S256` as the challenge method; `plain` is prohibited.
- Generate cryptographically random verifiers.
- **Generate or validate OAuth `state`.** When the application calls `BeginAuthorization` with an empty `state`, the SDK **MUST** generate a cryptographically random value (minimum 32 bytes before base64url encoding). When exchanging the authorization code, the SDK **MUST** refuse a callback whose `state` differs from the value sent in step 3 with `ErrLaunchInvalidState`, **before** any token-endpoint call.
- **Refuse a server that cannot verify `S256`.** When the authorization server advertises `code_challenge_methods_supported` and the list does not contain `S256`, constructing the `auth/smart` source **MUST** fail with `auth.ErrInvalidConfig` (HL7 SMART App Launch requires servers to support `S256`; RFC 9700 §2.1.1 makes PKCE support detectable from this metadata). [§ Advertised metadata lists](#advertised-metadata-lists) covers an absent or empty list.

The verifier comes from the RFC 7636 helper of `golang.org/x/oauth2`, and the state and the nonce from `crypto/rand`.

The authorization request **MUST** carry `aud` (HL7 SMART App Launch lists it as required; the openEHR SMART specification does not define its value). A source built from a resolved catalog **MUST** default `aud` to the catalog's `BaseURL` when the caller sets no audience; constructing a source that has no audience at all **MUST** fail with `auth.ErrInvalidConfig`.

When the configured scopes contain `openid`, the SDK **MUST** generate a nonce from at least 32 random bytes (base64url-encoded) in `BeginAuthorization`, record it on the returned `AuthorizationRequest`, and send it as `nonce` from `AuthorizeURL` (OpenID Connect Core 1.0 §3.1.2.1); without `openid`, `AuthorizeURL` **MUST NOT** send a `nonce`. `BeginAuthorization` **MUST** also record on the request the issuer the source is bound to (its `Issuer`, the OIDC issuer from discovery), so each authorization response is checked against the authorization server the user was sent to (RFC 9700 §4.4, the mix-up defence a client of several Platforms owes).

When `AuthorizeURL` is given a `launch` value, the request's `scope` **MUST** contain `launch` (HL7 SMART App Launch: an app launched from an EHR requests the `launch` scope), which the SDK adds when the configured scopes lack it.

#### Completing the authorization

`auth/smart` **MUST** provide `(*Source).CompleteAuthorization(ctx, callback url.Values, req AuthorizationRequest)`, which takes the query the redirect URI received and the request the launch started with, and which makes no token-endpoint call until every check below has passed, the `state` check first. A request without `State` or a PKCE verifier **MUST** fail with `auth.ErrInvalidConfig` before any check, so an empty state never matches an empty state. A callback that repeats `state`, `iss`, `code` or `error` **MUST** fail with `ErrAuthorizationRejected` (RFC 6749 §3.1 forbids a repeated response parameter). The checks **MUST** run in this order:

1. The `state` check of the PKCE rules above: a mismatch fails with `ErrLaunchInvalidState`.
2. **The issuer (RFC 9207).** The issuer check applies when the callback carries a non-empty `iss` or the authorization server advertises `authorization_response_iss_parameter_supported: true`. An empty `iss` counts as absent, and without either the check **MUST NOT** run. When the check applies and the request's `Issuer` is empty, the call **MUST** fail with `auth.ErrInvalidConfig`, not `ErrLaunchIssuerMismatch`, because there is nothing to compare with. Otherwise the callback's `iss` **MUST** equal `req.Issuer` exactly, and a callback without `iss` **MUST** be refused, because that callback is missing `iss` where the server advertises the parameter (RFC 9207 §2.4); either failure is `ErrLaunchIssuerMismatch`.
3. When the callback carries `error` (RFC 6749 §4.1.2.1), or carries neither `error` nor `code`, the call **MUST** fail with an error that `errors.Is` matches to `ErrAuthorizationRejected` and, for `error`, from which `errors.As` extracts an `*auth.OAuth2Error` holding `error`, `error_description` and `error_uri`.
4. The code is then exchanged exactly as `ExchangeAuthorizationCode` exchanges it, including the ID-token rules of § REQ-064.

#### Embedded launch

`auth/smart` **MUST** provide `ParseEHRLaunch(query url.Values, allow func(iss string) bool) (EHRLaunch, error)`, which reads the `iss` and `launch` parameters a Launcher appends to the app's launch URL. It **MUST** fail with `ErrLaunchInvalidRequest` when `iss` is missing or not an absolute URL with a host, or `launch` is missing, without consulting `allow`. Otherwise it **MUST** fail with `ErrLaunchIssuerNotAllowed` when `allow` is nil or returns false for `iss`, so a client never resolves discovery for a Platform it has not chosen to trust. `EHRLaunch.Issuer` is the Platform base URL to resolve ([§ REQ-070](#req-070)), and the caller passes `EHRLaunch.Launch` unchanged to `AuthorizeURL`.

### REQ-062 — JWKS rotation

#### Algorithm allowlists

The SMART discovery resolver surfaces two algorithm-selection lists onto `AuthEndpoints` (REQ-070):

- **`TokenEndpointAuthSigningAlgValuesSupported`** (`token_endpoint_auth_signing_alg_values_supported`): the JWS algorithms the authorization server accepts for client-assertion JWTs at the token endpoint (e.g. `["RS384","ES384"]`). `auth/smart` and `auth/clientcreds.NewFromCatalog` check a client assertion's algorithm against it, as [§ REQ-068](#req-068--flow-and-launch-mode-coverage) and its [§ Advertised metadata lists](#advertised-metadata-lists) set out.
- **`IDTokenSigningAlgValuesSupported`** (`id_token_signing_alg_values_supported`): the JWS algorithms used to sign ID tokens (e.g. `["RS256","ES384"]`). ID-token verification (REQ-064) uses this list as its allowlist when present ([§ ID-token verification](#id-token-verification)).

The SDK verifies ID tokens against the deployment's published JWKS, which it fetches on first use and caches. A lookup **MUST** fetch the set again once the cached one is older than a positive `JWKS.TTL`, and that TTL **SHOULD** default to five minutes, so a key the authorization server withdraws stops verifying tokens within that time. On a verification miss (a `kid` not in the cache, or a token without a `kid` whose signature the cached signing key does not verify), the SDK **MUST** refresh the JWKS once before reporting the verification as failed, which handles a silent rotation by the authorization server, unless it fetched the cached set less than `JWKS.MinRefreshInterval` ago. Within that interval the SDK **MUST NOT** fetch the set: it reports a `kid` miss as a failure, and verifies a token without a `kid` once more against the signing key the cache then holds, which another lookup's fetch may have replaced, so tokens that name unknown `kid`s, or carry signatures the cached key does not verify, cannot make it fetch the set for each of them. The interval **SHOULD** default to 30 seconds; zero or a negative value sets no minimum. A set older than a positive `JWKS.TTL` is fetched again whatever the interval. Concurrent refreshes share one fetch ([idiom.md § Concurrency](idiom.md#concurrency-req-026)).

When an ID token's header carries no `kid`, the SDK **MUST** verify it with the set's only signing key, a key published without a `kid` included, when the set holds exactly one key whose `use`, if present, is `sig`, and **MUST** reject the token otherwise (OpenID Connect Core 1.0 §10.1 lets an issuer omit `kid` only when its set holds one signing key).

A key whose `use` is present and is not `sig` **MUST NOT** verify an ID token, also when the token's `kid` names it: the SDK looks a `kid` up only among the set's signing keys, so such a token is refused as a `kid` the set does not hold. A key that publishes an `alg` **MUST NOT** verify a token whose header names another `alg` (RFC 8725 §3.1). Both refusals are decided on the token, so they match `auth.ErrJWKSValidationFailed` ([§ ID-token verification](#id-token-verification)).

#### ID-token verification

`auth/smart.ValidateIDToken` verifies the `id_token` signature against the deployment's JWKS and then applies the SDK's claim rules. The code exchange and the refresh verify the ID token they receive with it (§ REQ-064), and the top-level `smart` package re-exports it together with `IDTokenClaims`. The SDK delegates signature verification and JWK parsing to `github.com/coreos/go-oidc/v3`, which uses `go-jose/v4`.

- **Supported algorithms:** the SDK **MUST** support `RS256`, `RS384`, `ES256` and `ES384`, and **MUST NOT** treat any other algorithm as supported. RS384/ES384 are the HL7 SMART asymmetric baseline; RS256/ES256 cover the widely deployed remainder. Both RSA and ECDSA keys published in the JWKS are honoured.
- **Allowlist:** the caller passes the deployment's `id_token_signing_alg_values_supported` (via `smart.WithIDTokenSigningAlgs` / `ValidateConfig.AllowedIDTokenAlgs`). The effective allowlist **MUST** be the supported set narrowed to a non-empty caller allowlist, never widened by it, and the full supported set when the caller passes none. An empty intersection (the deployment advertises only algorithms the SDK does not support) **MUST** fail closed with `auth.ErrJWKSValidationFailed` before the JWKS is fetched, with no fallback to the full supported set.
- **Rejected:** the SDK **MUST** reject the unsecured `none` algorithm in any letter case, even when the allowlist names it, and any algorithm outside the effective allowlist. A key whose type does not fit the `alg` is rejected by the key matching of the verification library.
- **Error classes:** every rejection decided on the token itself (its segments, header, algorithm, key, signature or claims), here and under the claim rules below, **MUST** surface as an error that `errors.Is` matches to `auth.ErrJWKSValidationFailed`. A missing issuer, client ID or JWKS **MUST** surface as `auth.ErrInvalidConfig`, and a failed JWKS fetch as the fetch error, neither of them matching `auth.ErrJWKSValidationFailed`, so an outage never reads as a bad token.
- **Verify before claims:** the SDK **MUST** verify the signature before it trusts any claim; the claim checks read only the verified payload.
- **Claims:** `iss` **MUST** equal the configured issuer exactly, with no URL normalisation (OIDC Core §3.1.3.7), and `aud` **MUST** contain the client ID. When `aud` lists any other audience, the token **MUST** be rejected unless each of them is in the caller's trusted set, empty by default (OIDC Core 1.0 §3.1.3.7 step 3); when the token carries `azp`, it **MUST** equal the client ID. A token without `exp`, or without a non-empty string `sub`, **MUST** be rejected, since OIDC Core 1.0 §2 requires both claims. A token **MUST** be rejected when its `exp` lies the clock-skew allowance or more before the validation time, or when its `nbf` or `iat`, if present, lies more than the allowance after it. The allowance **SHOULD** be 30 seconds, so a small clock difference between the SDK and the authorization server does not refuse a valid token. When the caller supplies a nonce, the `nonce` claim **MUST** equal it.

`IDTokenClaims.Nonce` **MUST** hold the token's own `nonce` claim, empty when the token has none, whether or not the caller expected one.

### REQ-063 — Token refresh

**Requesting a refresh token.** The authorization server grants a `refresh_token` only when the authorization request includes the appropriate offline-access scope. Per HL7 FHIR SMART App Launch v2 "Scopes and Launch Context" ([https://hl7.org/fhir/smart-app-launch/](https://hl7.org/fhir/smart-app-launch/)):

- Include `offline_access` in the scope list to request a refresh token that persists beyond the current browser session.
- Include `online_access` to request a refresh token scoped to the current online session only.

The SDK provides `auth.ScopeOfflineAccess` and `auth.ScopeOnlineAccess` constants for composing these scope strings via `auth.JoinScopes`. These constants are lexical only — whether the server honours the request depends on the deployment's policy.

The `auth/smart`, `auth/clientcreds` and `auth/jwtbearer` token sources **MUST** proactively renew an access token when its `ExpiresAt` is within the refresh threshold of `time.Now()`: `auth/smart` with its refresh token, the other two with a new exchange, as their grants issue no refresh token. `WithRefreshThreshold(d time.Duration)` configures it on each of them: a `d` of 0 **MUST** select the default, and a negative `d` **MUST** count as 0, so a token past its `ExpiresAt` is never served. The default **SHOULD** be 30 seconds, which leaves a request time to reach the resource server before its token expires.

A token response whose `expires_in` is zero or below **MUST** give the access token a lifetime of zero seconds (RFC 6749 §5.1): its `ExpiresAt` is the time the response arrived, so the source hands that token to the call that obtained it and the next `Token` call renews it, or, for an `auth/smart` source that holds no refresh token, returns `auth.ErrReauthRequired` without a request. A response without `expires_in`, or with `null`, **MUST** leave `ExpiresAt` zero, so the token has no declared expiry, unless the source has a maximum token lifetime (below). `auth/smart.TokenResponse.HasExpiresIn` tells a response without `expires_in` from one whose `expires_in` is 0. A zero-lifetime token is past its `ExpiresAt` at once, so the transient-failure backoff below never serves it in place of a failure: a `Token` call that finds it renews it, sharing a renewal already in flight, where the source can renew it, as for any token past its `ExpiresAt`, and no renewal is sent without a call.

**A maximum token lifetime.** RFC 6749 §5.1 only recommends `expires_in`, so a token without one can still lapse at the server. `auth/clientcreds` and `auth/jwtbearer` **MUST** provide `WithMaxTokenLifetime(d time.Duration)`, which bounds how long the source trusts a token from its token endpoint:

- A token without a declared expiry **MUST** get an `ExpiresAt` of the time its response arrived plus `d`, so the refresh threshold renews it as it renews a token with `expires_in`.
- A token whose declared expiry is later than that time **MUST** be capped at it; an earlier declared expiry is kept.
- `WithMaxTokenLifetime` with a `d` of zero or below **MUST** fail construction with `auth.ErrInvalidConfig`, and so **MUST** a negative `Config.MaxTokenLifetime`, whose zero sets no bound.

The capped `ExpiresAt` is the token's `ExpiresAt` for everything below, the transient-failure backoff included.

**Acceptance (maximum token lifetime).** With `WithMaxTokenLifetime(d)`, a response without `expires_in` is renewed by the first `Token` call once `d` less the refresh threshold has passed, with no `401` first. A response whose `expires_in` exceeds `d` is renewed then too, and one below `d` keeps its own expiry. Without the option, a response without `expires_in` is cached until `Reauth`.

Wire-driven refresh — recovering from a `401 Unauthorized` that indicates an expired/invalid token — is **opt-in** rather than automatic inside the `TokenSource`: the transport layer exposes `transport.WithReauthOn401`, which on a 401 invokes the configured `auth.Reauther` (implemented by `*smart.Source.Reauth`) once and replays the request. Keeping it opt-in avoids surprising retries of non-idempotent writes ([§ Transport 401 hook](#transport-401-hook)).

Refresh uses the stored `refresh_token` against the deployment's `token_endpoint` (`grant_type=refresh_token`). A failed refresh is **terminal** when the token endpoint answered with a 4xx status and the OAuth2 error `invalid_grant`, `invalid_client` or `invalid_token`, which `auth.ExchangeError.Terminal()` reports, and **transient** otherwise (5xx, network, context, an unparsed response, an ID token the source refuses). On refresh failure:

- A terminal failure **MUST** surface to the consumer as the typed sentinel `auth.ErrReauthRequired` (wrapped in `*auth.ExchangeError`). The source then drops its access and refresh tokens, so later calls return `auth.ErrReauthRequired` without another request.
- A transient failure keeps both tokens. When a renewal that `Token` sends fails transiently while the held access token is not yet past its `ExpiresAt`, the `auth/smart`, `auth/clientcreds` and `auth/jwtbearer` token sources **MUST** return that token from `Token` without an error, and **MUST NOT** send another renewal request until a backoff ends: one second after the first such failure, twice as long after each further failure in a row, and never later than that `ExpiresAt`. Until then `Token` returns the held token without a request. A successful renewal, or new tokens (a code exchange, `SetTokens`, `Revoke`, a new exchange), ends the backoff. Past `ExpiresAt`, a transient failure surfaces as `auth.ErrRefreshFailed` (`auth.ErrTokenExchangeFailed` in `auth/clientcreds` and `auth/jwtbearer`), so the consumer can retry. In `auth/clientcreds` and `auth/jwtbearer`, an exchange that fails only because the context of the `Token` call that sent it ended returns that context's error and starts no backoff, as the call that started a refresh does in `auth/smart` (below). `Reauth` and `RefreshIfNeeded` do not wait for the backoff: they send their request at once and return its failure, and a token that `Reauth` marked for renewal is not served this way.
- The expired token **MUST NOT** be used silently.
- Concurrent `Token` calls on one source that find its token due for renewal **MUST** share one renewal request: with a rotating refresh token, a second request would present the refresh token the first already spent. The calls that share it get the same outcome, the held token included when a transient failure leaves it to be served.
- In `auth/smart` the refresh request **MUST NOT** be ended by the context of the call that started it, so a refresh token the server has already rotated is installed and reported to the `WithTokenChange` hook even when that call gives up. The request runs for at most 30 seconds. The call whose context ends first returns that context's error at once, and the calls waiting on the refresh get its own result. A refresh whose goroutine exits without an answer (`runtime.Goexit`) ends as a transient failure.

If no `refresh_token` is available (the deployment did not grant one), the `TokenSource` **MUST** return a typed error directing the consumer to restart the launch flow.

**Token changes.** `auth/smart` **MUST** provide the option `WithTokenChange(func(ctx context.Context, change TokenChange))` and call it after every successful code exchange and refresh, and never after a failed one, with the new access token, the refresh token the source now holds (after a refresh whose response carried none, the previous one; after a code exchange whose response carried none, empty) and the token response. The source calls it once it holds the new tokens, and **MUST** call it without holding its lock, so the hook can call the source. An application that keeps a session across restarts stores a rotated refresh token from it: RFC 6749 §6 has the client discard the old refresh token when a new one is issued, and RFC 9700 §4.14 makes rotation one of the two ways a public client's refresh token is protected. When `Revoke` (§ REQ-167) signs out a source that holds a token, the source **MUST** report one empty change, and never from `Revoke`'s own goroutine before the revocation request has been sent or has failed (when there is one to send), so a hook that panics or blocks cannot stop the request. The source **MUST** report changes to the hook one at a time, in the order it installed them, `Revoke`'s empty change included. Another call that reports changes, one already reporting or one that installs new tokens while `Revoke`'s request is out, can therefore report `Revoke`'s change before the request ends, and `Revoke` can return before the hook has seen it; that hook runs apart from `Revoke` and cannot hold up its request.

```go
type TokenChange struct {
    Access       auth.Token
    RefreshToken string
    Response     TokenResponse
}
```

#### Forced refresh

`RefreshIfNeeded(ctx context.Context) error` refreshes only when the access token is stale and a refresh token is present, and otherwise returns `nil`. It sends its request even while a backoff holds `Token` back, and returns the refresh's failure where `Token` would return the held token.

`auth.Reauther` forces a fresh credential:

```go
type Reauther interface {
    Reauth(ctx context.Context) error
}
```

`*smart.Source` implements `Reauther`. With a refresh token, `Reauth(ctx)` forces a refresh whatever the current token's freshness, with the same terminal and transient outcomes as any refresh. Without one there is nothing to exchange, so `Reauth` does not discard a cached token that is still within its `ExpiresAt` (a wire 401 may be scope-related, and a public client has no other credential); it returns `auth.ErrReauthRequired`, clearing the cached token only when it is already past `ExpiresAt`.

**Backend providers.** `auth/clientcreds` and `auth/jwtbearer` **MUST** implement `Reauther`: `Reauth` drops the cached access token and obtains a new one with a fresh exchange, coalesced with any exchange already in flight (REQ-026), and returns that exchange's error, if any. They have no refresh token; a new exchange is their only way to a new token.

`auth.ReautherFunc` adapts a function to `Reauther`:

```go
type ReautherFunc func(ctx context.Context) error
func (f ReautherFunc) Reauth(ctx context.Context) error { return f(ctx) }
```

It lets a closure, such as a discovery-catalog refresh ([§ REQ-071](#req-071)), serve as a `Reauther` without `transport/` importing `smart/discovery`.

#### Transport 401 hook

`transport.WithReauthOn401(r auth.Reauther)` installs an opt-in safety net. When a wire `401` is received:

1. If a `Reauther` is configured, this `Do` call has not yet reauthed, **and** the response's Bearer challenge permits it under [transport.md § REQ-166](transport.md#req-166--bearer-challenge-on-401-and-403), `transport/` **MUST** call `r.Reauth(ctx)` exactly once.
2. If `Reauth` returns nil, the request **MUST** be retried once. The retry acquires its token again, so it carries the refreshed credential.
3. If the retry also returns `401`, or `Reauth` itself returns an error, the call **MUST** fail without another attempt: with `transport.ErrUnauthorized` in the first case, and with the `Reauth` error, wrapped, in the second.

`Reauth` runs at most once per `Do` call, whatever the retry policy. Without `WithReauthOn401`, a wire `401` returns `transport.ErrUnauthorized` after one upstream call.

This hook is a **complementary safety net** — proactive expiry-based refresh in `Source.Token()` before the request is issued remains the primary mechanism. The hook covers the residual window where a token expires between the proactive-refresh check and the wire round-trip.

The retry **MUST** fire for **all HTTP methods**, including non-idempotent writes (`POST`/`PUT`), and repeat the request's method and body. This is safe because a `401` means the request was rejected at the authentication layer and therefore **not processed** by the resource — re-driving it once after refreshing the credential cannot double-apply a write; [REQ-166](transport.md#req-166--bearer-challenge-on-401-and-403) says which `401`s are re-driven. Deployments that signal authorization failures with `403` (reserving `401` for authentication/expiry) get the cleanest behaviour.

### REQ-167 — Token revocation

`auth/smart` **MUST** provide `(*Source).Revoke(ctx)` for signing out (RFC 7009). When the source's catalog advertises `revocation_endpoint`, `Revoke` **MUST** POST the form-encoded `token` with its `token_type_hint` (RFC 7009 §2.1): the refresh token when the source holds one (`refresh_token`), otherwise the access token (`access_token`). It **MUST** authenticate exactly as the token endpoint does (§ REQ-068), so a public client sends `client_id`.

Whatever the outcome, and before any revocation request, `Revoke` **MUST** clear the source's access and refresh tokens, drop its last token response and forget the identity of the last ID token it verified (§ REQ-064), so a failed call never leaves a signed-out session usable, no refresh starts with the token being revoked, and `LastTokenResponse` returns the zero value afterwards. It **MUST** return:

- nil on a 200 response, which RFC 7009 §2.2 sends for an unknown or already invalid token too;
- an `*auth.ExchangeError` matching `auth.ErrRevocationFailed` on any other response, or when the request gets none;
- an error matching `auth.ErrInvalidConfig` when the source holds a token but its catalog advertises no revocation endpoint;
- nil, without sending a request or calling the token-change hook, when the source holds no token, that is no refresh token and no access-token value, whether or not a revocation endpoint is advertised.

### REQ-064 — Launch context

The application-level `smart/` package **MUST** expose the SMART launch context as typed values:

```go
// sketch
package smart

import "context"

type LaunchContext struct {
    // FHIR-compat launch-context claims (SMART App Launch §7.1).
    Patient     string         // SMART "patient" launch parameter — opaque to SDK
    Encounter   string         // SMART "encounter" launch parameter
    User        string         // verified ID token's fhirUser claim, else its sub
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

Consumers never parse JWT claims by hand: `IDTokenClaims` carries the standard claims plus a typed map for deployment-extension claims.

When a token response carries an `id_token`, `ExchangeAuthorizationCode`, and so `CompleteAuthorization`, **MUST** validate it before returning: against the source's JWKS, its `Issuer`, its client ID, the request's nonce and the discovery `id_token_signing_alg_values_supported` (OpenID Connect Core 1.0 §3.1.3.7). A source without a JWKS **MUST** fail such an exchange with `auth.ErrInvalidConfig` instead of returning an unverified ID token. Any ID-token failure at the exchange **MUST** surface as an `*auth.ExchangeError` whose sentinel is `auth.ErrTokenExchangeFailed`, wrapping the cause so its § REQ-062 class (`auth.ErrJWKSValidationFailed`, `auth.ErrInvalidConfig` or the key-set fetch error) stays reachable with `errors.Is`. A code exchange whose response carries no `refresh_token` **MUST NOT** keep a refresh token from an earlier session. The verified claims **MUST** be returned on `TokenResponse.IDTokenClaims`, and `LaunchContextFromTokenResponse` **MUST** use them without validating again. Given an `id_token` without verified claims, it **MUST** validate that token itself as § REQ-062 describes, with the nonce, signing algorithms and trusted audiences its caller passes.

A refresh response that carries an `id_token` **MUST** be validated the same way except for the nonce; when the source has verified an earlier ID token and has not been signed out with `Revoke` since (§ REQ-167), the new one **MUST** carry that token's `iss` and `sub` and list the same audiences in `aud`, in any order (OpenID Connect Core 1.0 §12.2; RFC 7519 gives `aud` no order). A failure to verify that ID token **MUST** fail the refresh with an error that matches `auth.ErrRefreshFailed` and the failure's own § REQ-062 class: `auth.ErrJWKSValidationFailed` for a token refused on its own content, otherwise `auth.ErrInvalidConfig` or the key-set fetch error. The source **MUST** then keep its previous access and refresh tokens, including when the refresh was forced by `Reauth`. Such a failure is transient (§ REQ-063): it reaches `Token` only once the held access token is past its `ExpiresAt`, and reaches `Reauth` and `RefreshIfNeeded` at once.

A refresh response can leave members out: OpenID Connect Core 1.0 §12.2 lets a server omit the `id_token`, SMART App Launch lets it omit launch context, and RFC 6749 §6 reads an omitted `scope` as the original grant. The source's last token response **MUST** keep the earlier value of each such member that a refresh response omits and take the new value of each it carries: the verified `IDTokenClaims`, kept when the response carries no `id_token`, so a `LaunchContext` rebuilt from `LastTokenResponse` keeps its `User` and `IDToken`; the launch-context parameters `patient`, `encounter`, `ehrId`, `episodeId`, `fhirContext`, `intent`, `need_patient_banner`, `smart_style_url` and `tenant`; and `scope`, whose earlier value, when the last token response has none, as after `SetTokens`, is the scope of the access token the source holds. A launch-context parameter or `scope` that the response carries counts as carried even as an empty string or null. A refresh response that omits `scope` therefore gives an access token with the kept scope.

`LaunchContext.User` **MUST** come from a verified ID token only: its `fhirUser` claim, else its `sub`, and empty without a verified ID token. A `fhirUser` member in the token-endpoint body is not an identity claim and never sets it, but `TokenResponse.FHIRUser` and `Raw` **MUST** still carry the member. Signature verification supports RS256/RS384/ES256/ES384 and is constrained by the deployment's advertised `id_token_signing_alg_values_supported` when supplied ([§ ID-token verification](#id-token-verification)).

The `EHRID` and `EpisodeID` fields are populated from the `ehrId` and `episodeId` token claims defined in the canonical openEHR SMART App Launch specification. The SMART-compat extras (`Intent`, `SMARTStyleURL`, `NeedPatientBanner`, `Tenant`) are populated when present. All of these fields are also available untyped via `Raw`.

#### Platform principal claims

When the token-endpoint response or ID token carries platform-issued principal claims, the SDK **MUST** surface them on `LaunchContext` (or the equivalent for non-SMART providers) verbatim:

```go
type LaunchContext struct {
    // ... the fields above ...

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

Its rules:

- The SDK **MUST NOT** coerce the principal type — if the claim is missing or carries an unrecognised value, `Type` is `PrincipalTypeUnknown` and consumers handle it.
- The SDK **MUST NOT** invent a `Principal` value when no claim is present — `LaunchContext.Principal` is `nil` in that case.
- Claim names (`principal_uid`, `principal_type`) are configurable via `smart.WithPrincipalClaimNames(...)` when building a `LaunchContext`. Principal claims are read from the validated ID token when present, otherwise from the token-endpoint JSON body (`TokenResponse.Raw`).

## HTTP Basic on openEHR REST

### REQ-069

Some openEHR REST deployments (development CDRs, legacy gateways, internal tools) authenticate API calls with **HTTP Basic** — a static username and password on every request, not an OAuth2 access token.

`auth/basic` **MUST** implement `auth.TokenSource` for this case:

```go
// sketch
package basic

// New returns a TokenSource that always yields Type "Basic" and Value set to the
// base64-encoded "username:password" payload per RFC 7617.
func New(username, password string) (*Source, error)
```

Rules:

- `New` **MUST** reject an empty username with `auth.ErrInvalidConfig`. Password **MAY** be empty when the deployment allows it.
- `Token()` **MUST** return `auth.Token{Type: "Basic", Value: <base64(user-pass)>}` with no expiry (`ExpiresAt` zero) — there is no token exchange or refresh.

`auth/basic` makes no OAuth2 token-endpoint calls; it is unrelated to `client_secret_basic` on the token endpoint (see `auth/clientcreds`). The rules of [§ REQ-060](#req-060) apply to it as to every token source: `transport/` sends its token as `Authorization: Basic <Value>`, and a per-request override with `auth.WithTokenSource(ctx, ts)` works as it does for Bearer providers.

Consumers wire Basic auth at client construction:

```go
ts, _ := basic.New("service", os.Getenv("OPENEHR_PASSWORD"))
client, _ := transport.New(catalog, transport.WithTokenSource(ts), ...)
```

## Client Credentials and JWT Bearer providers

`auth/clientcreds` and `auth/jwtbearer` are simpler — no interactive flow, no launch context. They:

- Implement `auth.TokenSource`.
- Take the token endpoint and the client's credential as arguments of `New` (`clientcreds.New(clientID, clientSecret, tokenURL, …)`, `jwtbearer.New(tokenURL, assertion, …)`), and further configuration, such as scope, audience or a client assertion source, as [functional options](idiom.md#functional-options).
- Coalesce concurrent refreshes (REQ-026).
- Report a failed token request as an `*auth.ExchangeError`, matched by the `auth` sentinels of [§ Error mapping](#error-mapping).

## Scope handling

### REQ-165 — openEHR scope syntax

The SDK does not apply scope strings as application policy: which scopes to grant, and whether a granted scope covers a request, is decided by the authorization server and the resource server. Building or reading the syntax of one scope token is not policy. The SDK **MUST**:

- Pass scope strings verbatim from configuration through to the authorization request.
- Round-trip the granted scope from the token response back to the application via `LaunchContext.Scopes`.
- Provide a small helper (`auth.BuildScope(compartment, resource, permission)`) for composing scopes of the shape `<compartment>/<resource>.<permission>` without templating strings. `BuildScope` is lexical: it **MUST** trim surrounding white space on each part and not validate the parts, so it serves SMART on FHIR scopes as well. It **MUST** leave out an empty compartment or permission together with its separator, and keep an empty resource, separator included: `BuildScope("patient", "", "rs")` is `patient/.rs`, `BuildScope("patient", "", "")` is `patient/`, and `BuildScope("", "launch", "")` is `launch`.
- Provide a typed builder and reader for the openEHR resource scopes of SMART on openEHR § Resource Scopes, `<compartment>/<resource>-<pattern>.<permissions>`:

```go
// package auth (sketch)

type OpenEHRScope struct {
    Compartment string // "patient", "user" or "system"
    Resource    string // "template", "composition" or "aql"
    Pattern     string // a template id or stored-query name, "*" wildcards allowed
    Permissions string // a non-empty subset of "cruds", in that order
}

func (s OpenEHRScope) Token() (string, error)
func ParseOpenEHRScope(token string) (OpenEHRScope, bool)
```

`ErrInvalidScope` is declared once, in [Error mapping](#error-mapping).

`Token` **MUST** return the single scope token, or an error that `errors.Is` matches to `auth.ErrInvalidScope` when any of these holds:

- the compartment is not `patient`, `user` or `system`, or the resource is not `template`, `composition` or `aql`;
- the permissions are empty, repeat a letter, use a letter outside `cruds`, or are out of the order `c`, `r`, `u`, `d`, `s` (HL7 SMART App Launch v2 permission syntax, which the openEHR scopes follow);
- the pattern is empty, or contains a character outside the RFC 6749 §3.3 scope-token set (`%x21 / %x23-5B / %x5D-7E`: printable ASCII other than space, `"` and `\`).

`Token` **MUST NOT** trim, escape or otherwise rewrite a part, so a template id that contains a space or a non-ASCII character cannot be written as a scope.

`ParseOpenEHRScope` **MUST** read one token with the same grammar: the compartment ends at the first `/`, the permissions start after the last `.`, and the resource ends at the first `-`, so dotted template ids and query names (`org.openehr::bloodpressure.v1`) survive. It **MUST** return `false`, never an error, for a token that is not an openEHR resource scope, such as `openid`, `launch/patient` or the SMART on FHIR scope `patient/Observation.rs`. `Token` and `ParseOpenEHRScope` **MUST NOT** interpret the wildcards in a pattern; their matching rules belong to the authorization server.

## Service discovery

How the SDK resolves service base URLs from a SMART-on-openEHR deployment, and how non-discovering backends are supported.

A SMART-on-openEHR deployment advertises **service base URLs** via a discovery document (canonical spec: [ITS-REST/development § SMART App Launch](https://specifications.openehr.org/releases/ITS-REST/development/smart_app_launch.html)). The relevant service identifiers for this SDK include `org.openehr.rest` (the openEHR REST API) plus deployment-specific identifiers for Cadasto Extra, Datamap, Admin, and other extras. On a Cadasto deployment, the same discovery document advertises **`org.openehr.rest`** *and* **`org.fhir.rest`** — the openEHR-side SDK consumes the former and ignores the latter, while a FHIR-side SDK does the reverse.

The SDK treats discovery as a **first-class step**, not an implementation detail of constructor convenience.

### ServiceCatalog

#### REQ-070

SDK constructors **MUST** accept a `smart/discovery.ServiceCatalog`, not a single base URL:

```go
// sketch
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
    TokenEndpointAuthMethodsSupported          []string // auth/smart cross-check; NewFromCatalog method check (REQ-068)
    TokenEndpointAuthSigningAlgValuesSupported []string // auth/smart and NewFromCatalog client-assertion alg check (REQ-068)
    IDTokenSigningAlgValuesSupported           []string // ID-token verify allowlist, consumed by ValidateIDToken (REQ-062, REQ-064)
    AuthorizationResponseIssParameterSupported bool     // RFC 9207 authorization_response_iss_parameter_supported (REQ-061)
    Capabilities []string
}
```

`AuthEndpoints.Clone()` **MUST** return a copy that shares nothing with its receiver: each URL is a new value, its user information included, and each list a new slice, so an edit to one cannot reach the other. `NewStaticCatalog` **MUST** copy `StaticConfig.Auth` this way.

Every typed client (`openehr/client/ehr`, `openehr/client/query`, `cadasto/extra`, etc.) resolves its base URL from the catalog by service ID, not from a top-level "base URL" config field.

The catalog **MUST** carry the Platform base URL and the OIDC issuer as two values ([ADR 0023](../adr/0023-smart-platform-base-url-and-oidc-issuer.md)). `BaseURL` is the URL the document was resolved from: in SMART App Launch the embedded-launch `iss` parameter names this API base, not the OIDC issuer. `Issuer` is the document's `issuer` member, the value ID tokens are checked against; when the document declares no `issuer`, `Issuer` **MUST** equal `BaseURL`.

Each `ServiceEntry` **MUST** surface the canonical `version`, `description`, `documentation` and `openapi` members of its `services` entry verbatim, empty when absent. The SDK **MUST NOT** fetch or validate the `documentation` and `openapi` links.

##### Hand-built catalogs

For non-discovering openEHR backends (a static EHRbase deployment, a Cadasto deployment with a pinned configuration, a local CDR for testing), consumers **MUST** be able to construct a `ServiceCatalog` directly without going through a discovery transport:

```go
catalog, err := discovery.NewStaticCatalog(discovery.StaticConfig{
    Issuer: "https://ehrbase.example/",
    Services: map[string]discovery.ServiceEntry{
        "org.openehr.rest": {
            ID:          "org.openehr.rest",
            BaseURL:     discovery.MustParseURL("https://ehrbase.example/rest/openehr/v1"),
            SpecVersion: "1.1.0",
        },
    },
    Auth: discovery.AuthEndpoints{
        // ... or zero value if backend doesn't use OAuth2
    },
})
if err != nil {
    return err // a missing Issuer, or a nil or relative service BaseURL
}
```

The `NewStaticCatalog` constructor **MUST NOT** require a network round trip; the resulting catalog has `ResolvedAt = time.Now()` and takes `ExpiresAt` from `StaticConfig.ExpiresAt`, so it has no TTL unless the caller sets one. `StaticConfig.BaseURL` is optional: when empty, the catalog's `BaseURL` **MUST** equal `StaticConfig.Issuer`.

### Resolution flow

The full flow when discovery is in play:

1. **Resolve.** On an explicit `(*discovery.Resolver).Resolve(ctx, baseURL)` (the transport only stores the catalog it is given), fetch the SMART configuration document at the Platform base URL's well-known URL, **`<base URL>/.well-known/smart-configuration`**, the well-known path appended to the base URL's own path. Parse it; validate the members REQ-072 requires and the openEHR-REST service catalog.
2. **Cache.** Store the resolved `ServiceCatalog`. The cache **MAY** be in-process (default), file-backed, or an injected `Cache` interface (for distributed deployments).
3. **Validate.** Confirm every service the client intends to use is advertised: `org.openehr.rest` for openEHR-REST consumers, the Cadasto-extra services for Cadasto clients ([§ REQ-072](#req-072)). Spec-version compatibility is checked **here**, not after the first request.
4. **Route.** Each typed client resolves its base URL from the catalog by service ID at request time.
5. **Refresh.** Triggered by:
   - TTL expiry (`ExpiresAt` reached).
   - A `401` on a previously working endpoint, when the consumer wires a catalog refresh into the transport ([§ REQ-071](#req-071)); the request is then retried once.
   - An explicit call to `(*discovery.Resolver).Refresh(ctx, baseURL)` ([§ Refresh API](#refresh-api)).

### Caching

#### REQ-071

The discovery cache **MUST**:

- Honour the TTL declared in the discovery response. The declared TTL follows [RFC 9111](https://www.rfc-editor.org/rfc/rfc9111.html): it is 0 when `Cache-Control` carries `no-store` or `no-cache`, else the `Cache-Control` `max-age` in seconds, else the time from `Date`, or from receipt without one, to `Expires`, where a past or invalid `Expires` declares 0; a TTL of 0 leaves the entry stale at once. When the response declares none, a default TTL applies, which **SHOULD** be 15 minutes, so a change a Platform makes to its document reaches its clients within that time.
- Honour `ETag` / `If-None-Match` for conditional refresh: a `304 Not Modified` to a conditional request renews the cached entry's TTL from the TTL the `304` declares, or the default when it declares none, without replacing the body, and a `304 Not Modified` to a request without `If-None-Match` is a failed fetch ([§ Refresh API](#refresh-api)).
- Coalesce concurrent resolution attempts (REQ-026): one caller fetches and the others wait. A caller **MUST** get an error that `errors.Is` matches to its own context's error when that context ends while it waits on a fetch another caller started, or when its context's end alone fails the fetch it started. Such a failure of the starting caller's context, or a panic in the fetch or in the `Cache` call that records its result, **MUST NOT** fail the callers that joined the fetch, nor make the resolver write or drop the cached entry, since it says nothing about the Platform: each joined caller whose own context is still live fetches again. The panic **MUST** reach the caller that started the fetch unchanged and leave no caller for that base URL waiting.
- Key every entry by the Platform base URL the caller resolved, never by the document's `issuer` ([ADR 0023](../adr/0023-smart-platform-base-url-and-oidc-issuer.md)). A cached catalog whose `BaseURL` is not the base URL it is found under **MUST** be treated as absent: the resolver fetches with a plain request, as on a miss, and the result replaces the entry.
- Return a fresh cached catalog only once it passes the calling resolver's REQ-072 checks and those REQ-073 checks that send no request (the issuer, the Platform base URL and the auth endpoints' `https` scheme), because one `Cache` may serve resolvers built with different options. A catalog that fails them **MUST** produce the `DiscoveryError` a new document would and stay cached: the failure says nothing new about the Platform. The OIDC cross-check is not run on a fresh hit; it runs when the document is fetched or renewed (REQ-073).

**Known gap.** Through a shared `Cache`, a catalog that a resolver built with `WithoutOpenIDConfigurationCheck()` cached reaches a resolver that requires the OIDC cross-check unchecked, until the entry expires or is refreshed. Entries are keyed by the Platform base URL alone, so a catalog fetched from one resolver's `WithWellKnownPath` document likewise reaches a resolver configured with another path.

The resolver closes the response body whatever the status, the `304` path included.

The cache is refreshed on a wire `401` only when the consumer wires it, by passing an `auth.ReautherFunc` closure to `transport.WithReauthOn401`:

```go
transport.WithReauthOn401(auth.ReautherFunc(func(ctx context.Context) error {
    _, err := resolver.Refresh(ctx, baseURL)
    return err
}))
```

The transport then calls the closure at most once per `Do` call and retries the request once; on a second `401` it surfaces `transport.ErrUnauthorized` ([§ REQ-063](#req-063--token-refresh)).

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

##### Refresh API

Consumers **MUST** be able to trigger a refresh explicitly, with `(*discovery.Resolver).Refresh`:

```go
catalog, err := resolver.Refresh(ctx, baseURL) // baseURL is the Platform base URL
```

A refresh **MUST**:

- Send a conditional request (`If-None-Match`) when the cached entry carries an `ETag`, and a plain request when it carries none, keeping that entry in place meanwhile.
- On a `304 Not Modified` to a conditional request, re-run the REQ-072 checks and the REQ-073 trust checks, the OIDC cross-check included, on the cached document, and renew the cached entry's TTL without replacing its document (REQ-071); on a `200`, re-run the resolve / validate / cache pipeline and replace the entry; on a failure, invalidate the entry, except a failure that only the caller's own cancelled or expired context caused, and a panic (REQ-071). A `304 Not Modified` to a plain request is a failed fetch: no document came back.
- Return the current catalog (or an error if resolution fails).

A refresh does not hold up other requests beyond the coalescing window: they continue with the cached catalog until the refresh completes or fails. After a failed refresh the next request fetches again and reports the discovery error if that fetch fails too, unless only the refreshing caller's own context ended the refresh, or it panicked; neither of those makes the resolver write or drop the cached entry.

### Validation

#### REQ-072

On every resolution and every refresh, the SDK **MUST**:

- Verify required services are present. Missing required services **MUST** produce a typed `DiscoveryError` with the missing service IDs enumerated.
- Verify spec-version compatibility of each required service; a service the caller does not require is not checked. The compared value is the service entry's `spec_version`. When the entry advertises none (absent or empty), the compared value is the entry's `version` if the caller has narrowed the accepted set with `WithAcceptedSpecVersions`, and otherwise there is none: the entry **MUST** pass, and its `version` **MUST NOT** be compared: absence is acceptable (ADR 0008), and a Platform advertises its own API version in `version`. The compared value **MUST** match the SDK's pinned target (REQ-050) or, when the caller sets `WithAcceptedSpecVersions`, one of the versions it names; that list replaces the pinned target, so a caller who still accepts the pin names it too. A mismatch **MUST** produce a typed `DiscoveryError`.
- SMART App Launch 2.2.0 lists `token_endpoint` as required, and requires `issuer` when `capabilities` contains `sso-openid-connect`. The anonymous-only relaxation, and passing a document without `issuer`, are the SDK's own choice. A document that declares none of `authorization_endpoint`, `token_endpoint` or `jwks_uri`, and advertises none of `launch-ehr`, `launch-standalone` or `sso-openid-connect`, is an anonymous-only deployment and **MUST** pass. Otherwise `token_endpoint` **MUST** be present; `authorization_endpoint` **MUST** be present when `capabilities` contains `launch-ehr` or `launch-standalone`; `jwks_uri` **MUST** be present when `capabilities` contains `sso-openid-connect`. A document that omits a member it needs **MUST** produce `DiscoveryError{Reason: ReasonAuthEndpointsMissing}`. No other member is needed, and this rule **MUST NOT** refuse a document for any other missing member: neither a missing `issuer`, even with `sso-openid-connect`, nor a missing `authorization_endpoint` on a backend-only document.
- Validate URL well-formedness. Malformed `BaseURL` / `AuthorizationEndpoint` / etc. **MUST** produce a typed `DiscoveryError`.

Soft compatibility (forward-compatible spec micro-versions) **MAY** be allowed via a functional option:

```go
discovery.WithAcceptedSpecVersions("1.1.0", "1.1.1")
```

The default is **strict** — only the pinned version is accepted.

---

##### Errors

```go
type DiscoveryError struct {
    Issuer          string // the Platform base URL the resolution was for
    Reason          DiscoveryErrorReason
    MissingServices []string // with ReasonMissingService
    SpecVersionGot, SpecVersionWant string // with ReasonSpecVersionMismatch
    Inner           error
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

Discovery errors **MUST** be distinguishable from wire errors: `errors.AsType[*discovery.DiscoveryError]` matches one, and `errors.AsType[*transport.WireError]` does not.

### REQ-073 — Discovery trust posture

SMART configuration documents and their auth endpoints are untrusted input until validated. On every resolution and refresh the SDK **MUST** apply the rules below, except on a fresh cache hit, which runs only the checks [§ REQ-071](#req-071) names. The plaintext warnings are not repeated on a hit either: they are logged when the document is fetched or a cached catalog is renewed on a `304`.

- **Issuer ([ADR 0023](../adr/0023-smart-platform-base-url-and-oidc-issuer.md)).** When the fetched document declares an `"issuer"` member, the SDK **MUST** accept it as the catalog's `Issuer` whether or not it equals the Platform base URL, provided it is an absolute URL with the `https` scheme and no query or fragment (OIDC Core 1.0 §2). A malformed issuer **MUST** produce `DiscoveryError{Reason: ReasonMalformedURL}`; an `http` issuer **MUST** produce `DiscoveryError{Reason: ReasonInsecureURL}` unless the resolver is constructed with `WithAllowInsecure()`. The document's issuer **MUST NOT** replace the base URL: the base URL the caller resolved stays the catalog's `BaseURL` and its cache key.
- **OIDC cross-check.** When the document declares an `issuer` that differs from the Platform base URL, the resolver **MUST**, whenever it fetches the document or renews a cached one on a `304`, fetch `<issuer>/.well-known/openid-configuration` (OIDC Discovery 1.0 §4, the path appended to the issuer's own path) and require its `issuer` member to equal the catalog's `Issuer` exactly (OIDC Discovery 1.0 §4.3) and, when both documents declare `jwks_uri`, the two values to be equal. A mismatch **MUST** produce `DiscoveryError{Reason: ReasonIssuerMismatch}`, and a failed fetch `DiscoveryError{Reason: ReasonFetchFailed}`; an OIDC document that does not parse, or names no `issuer`, is a failed fetch, not a mismatch. The resolver **MUST NOT** fetch the OIDC document when it is constructed with `WithoutOpenIDConfigurationCheck()`, or when the document declares no `issuer` or an `issuer` equal to the base URL.
- **Platform base URL.** A plaintext Platform base URL **MUST** produce `DiscoveryError{Reason: ReasonInsecureURL}` unless the resolver is constructed with `WithAllowInsecure()`.
- **HTTPS on auth endpoints.** `authorization_endpoint`, `token_endpoint`, `jwks_uri`, and `registration_endpoint` (when present) **MUST** use the `https` scheme unless the resolver is constructed with `WithAllowInsecure()`. Plaintext URLs **MUST** produce `DiscoveryError{Reason: ReasonInsecureURL}`. The `allowInsecure` path **MAY** log a warning instead of failing for development deployments.
- **No downgrade on redirect.** While fetching the SMART configuration document or the OIDC document of the cross-check, the resolver **MUST NOT** follow a redirect to a URL whose scheme is not `https`, unless it is constructed with `WithAllowInsecure()`; such a redirect **MUST** produce `DiscoveryError{Reason: ReasonInsecureURL}`. Otherwise the injected client's own redirect policy applies.
- **Plaintext warnings.** The resolver **SHOULD** log a warning when a catalog it resolved holds a plaintext `http` URL, a service's `services.<id>.baseUrl` or an auth endpoint accepted under `WithAllowInsecure()`, and when it fetched the SMART configuration document over `http`, a redirect to `http` included. A plaintext service URL is warned about, not refused: which deployments to talk to is the consumer's decision. A catalog built with `NewStaticCatalog` is not checked.

The resolver does not require `jwks_uri` to share the issuer's host; the `https` scheme is the floor.

### Surfaced authorization-server metadata

The resolver parses and surfaces the following SMART authorization-server metadata fields onto `AuthEndpoints` (REQ-070). All fields are optional — absent fields resolve to nil/empty with no error.

| Wire field | `AuthEndpoints` field | Notes |
|---|---|---|
| `introspection_endpoint` | `IntrospectionEndpoint *url.URL` | RFC 7662 token introspection, a resource-server operation; surfaced only |
| `revocation_endpoint` | `RevocationEndpoint *url.URL` | RFC 7009 token revocation; `auth/smart.Source.Revoke` posts to it (REQ-167) |
| `management_endpoint` | `ManagementEndpoint *url.URL` | SMART management endpoint |
| `token_endpoint_auth_methods_supported` | `TokenEndpointAuthMethodsSupported []string` | Client-auth method list; feeds the `auth/smart` method cross-check and the `auth/clientcreds.NewFromCatalog` check (REQ-068) |
| `token_endpoint_auth_signing_alg_values_supported` | `TokenEndpointAuthSigningAlgValuesSupported []string` | Client-assertion (client-auth) JWS alg list; checked by `auth/smart` and `auth/clientcreds.NewFromCatalog` (REQ-068) |
| `authorization_response_iss_parameter_supported` | `AuthorizationResponseIssParameterSupported bool` | RFC 9207 §3; when true, `CompleteAuthorization` refuses a callback without `iss` (REQ-061) |
| `id_token_signing_alg_values_supported` | `IDTokenSigningAlgValuesSupported []string` | Selects the **ID-token verify allowlist**: `auth/smart` applies it at the code exchange and on refresh, and a caller of `smart.ValidateIDToken` passes it through `smart.WithIDTokenSigningAlgs`, so verification constrains accepted signature algorithms (RS256/RS384/ES256/ES384); see [§ REQ-062](#req-062--jwks-rotation) and REQ-064 |

These fields are **consumed**, not merely surfaced:

- `id_token_signing_alg_values_supported` → the ID-token verifier's accepted-algorithm allowlist, which `auth/smart` applies itself.
- `code_challenge_methods_supported` → `auth/smart.FromConfig` refuses a server whose non-empty list leaves out `S256`, the only PKCE method the SDK sends.
- `grant_types_supported` → `auth/clientcreds.NewFromCatalog` refuses a server whose non-empty list leaves out `client_credentials`.
- `token_endpoint_auth_methods_supported` → `auth/smart.FromConfig` cross-checks it against the configured credential's implied method (a mismatch is rejected with `auth.ErrInvalidConfig`), and `auth/clientcreds.NewFromCatalog` against the configured client-auth method ([§ Backend Services from a resolved catalog](#backend-services-from-a-resolved-catalog), REQ-068).
- `authorization_response_iss_parameter_supported` → when true, `auth/smart.CompleteAuthorization` refuses a redirect without `iss` ([§ REQ-061](#req-061--pkce-flow), RFC 9207 §2.4).
- `token_endpoint_auth_signing_alg_values_supported` → `auth/smart.FromConfig` checks the configured client-assertion algorithm against it ([§ REQ-068](#req-068--flow-and-launch-mode-coverage)), and `auth/clientcreds.NewFromCatalog` checks client assertions against it ([§ Backend Services from a resolved catalog](#backend-services-from-a-resolved-catalog), REQ-068).
- `revocation_endpoint` → `auth/smart.Source.Revoke` signs out there (RFC 7009; REQ-167).

The rest remain **surface-only** (populated but with no consuming logic wired): `management_endpoint` (no management client yet) and `introspection_endpoint` (introspection is a resource-server operation outside the SDK's client scope).

The `smart/discovery` package also exports openEHR SMART capability string constants (`CapabilityContextOpenEHREHR`, `CapabilityContextOpenEHREpisode`, `CapabilityOpenEHRPermissionV1`, `CapabilityLaunchBase64JSON`) for consumers that need to branch on the `capabilities` array.

## What is NOT in scope here

- **OAuth2 dynamic client registration** — the SDK consumes a pre-registered client. Dynamic registration could be added as a helper in `smart/` later.
- **App-side credential storage** — token storage (encrypted at rest, OS keychain, browser cookie) is application-side.
- **Revocation policy** — when tokens expire or are revoked on the server is the deployment's decision; the SDK reacts to the resulting wire errors and offers only the client's own sign-out revocation (REQ-167).
- **MTLS, FAPI, JAR / PAR** — out of v1 scope; future provider sub-packages could address them.
- **Token introspection (RFC 7662)** — a resource-server operation, and the SDK is a client of authorization servers. Discovery still surfaces `introspection_endpoint` (REQ-070), but the SDK ships no introspection client.
- **Service registration.** The SDK consumes discovery output; it does not publish or maintain the discovery document.
- **DNS resolution caching.** That belongs to the injected `*http.Client`'s transport configuration.
- **Health probing.** Discovery validates the catalog *structure*; whether the advertised endpoints are reachable is checked on first use, not at resolution time.
- **Cross-Platform aggregation.** The federator use case constructs one client per Platform base URL ([scope.md § Federative API client](scope.md#federative-api-client)); the SDK does not aggregate catalogs across Platforms.
- **FHIR-side service consumption.** Even when the discovery document advertises `org.fhir.rest`, the SDK ignores it. A sibling FHIR SDK consumes that service.
