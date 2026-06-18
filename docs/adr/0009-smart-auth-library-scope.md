# ADR 0009 — SMART-on-openEHR auth library scope and dependency model

- **Status:** Accepted, 2026-06-18.
- **Supersedes:** —
- **Superseded by:** —
- **Tracks:** [`docs/plans/archive/2026-06-16-auth-smart-conformance-audit.md`](../plans/archive/2026-06-16-auth-smart-conformance-audit.md). Resolves: [STRAND-05](../specifications/research-strands.md#strand-05--smart-on-openehr-auth-library). Amends: REQ-061, REQ-062, REQ-063, REQ-064.

## Context

**STRAND-05** asked two related questions:

1. **What auth library should the SDK build?** REQ-061..064 define the contract — PKCE, JWKS rotation, token refresh, launch context — but the scope of the implementation (which flows, which launch modes, how far along the SMART conformance profile) was open.
2. **Hand-roll or adopt ecosystem libraries?** The SDK previously held an "OpenTelemetry-only runtime dependency" rule to keep the dependency graph minimal. SMART-on-openEHR requires OAuth2 code flows, PKCE, JWKS rotation, JOSE signing (RS384, ES384 baseline), and OIDC ID-token verification. The question was whether to hand-roll these security-sensitive primitives or adopt audited ecosystem libraries.

The auth conformance audit (Phase 1–6) produced concrete evidence: a full SMART-on-openEHR auth library was built across `auth/smart`, `auth/clientcreds`, `auth/jwtbearer`, `auth/basic`, `auth/introspect`, and `smart/discovery`. The dependency question was resolved in favour of adopting ecosystem libraries during Phase 3a (client-assertion signing) and Phase 3e (ID-token verification).

## Decision

### (a) Build the full SMART-on-openEHR auth library

The audit delivered the following scope across `auth/` and `smart/`:

| Capability | Package | Phase |
|---|---|---|
| SMART discovery (`/.well-known/smart-configuration`, canonical `services` map) | `smart/discovery` | 1 (F-A, ADR 0008) |
| PKCE public-client authorization-code flow | `auth/smart` | 2 (REQ-061) |
| Confidential symmetric (`client_secret_basic`) | `auth/smart` | 3b (F-C) |
| Confidential asymmetric `private_key_jwt` (authorization-code) | `auth/smart` | 3b (F-C) |
| SMART Backend Services (`client_credentials` + `private_key_jwt`) | `auth/clientcreds` | 3c (F-C) |
| JWT Bearer Token Grant (RFC 7523) | `auth/jwtbearer` | 3a (F-I) |
| ID-token verification (RS256/RS384/ES256/ES384) | `smart/idtoken` | 3e (F-M) |
| Token refresh + F-L terminal/transient classification | `auth/smart` | 4a (REQ-063) |
| Transport 401→reauth safety net | `transport` | 4b (F-D) |
| Launch context (`ehrId`, `episodeId`, `fhirUser`, …) | `smart` | REQ-064 |
| Platform principal claims | `smart` | REQ-067 |
| RFC 7662 token introspection client (opt-in, resource-server scope) | `auth/introspect` | 5b (F-J) |
| Launch-context scope helpers (`ScopeLaunch*`, `ScopeOfflineAccess`, …) | `auth` | 6a (F-F) |

**Four flows** (PKCE public, confidential symmetric, confidential asymmetric, Backend Services / JWT Bearer) and **three launch modes** (standalone, embedded/EHR-launch, backend) are covered and exercised by PROBE-001..009 in the auth conformance probe suite (see `docs/specifications/conformance.md`).

### (b) Relax the OTel-only dependency rule; adopt ecosystem security libraries

The SDK adopts three new runtime dependencies, scoped to `auth/` and `smart/`:

| Library | Used for |
|---|---|
| `golang.org/x/oauth2` | PKCE `code_verifier`/`code_challenge` generation (RFC 7636 `GenerateVerifier` / `S256ChallengeFromVerifier`) in `auth/smart/pkce.go`, plus an RFC 7636 parity cross-check in PROBE-004. (Token sources, refresh, and token-endpoint error handling are the SDK's own `auth` types — not `x/oauth2`'s.) |
| `github.com/coreos/go-oidc/v3` | ID-token signature verification (RS256/RS384/ES256/ES384 via `go-jose`) |
| `github.com/go-jose/go-jose/v4` _(transitive)_ | JWS signing for `client_assertion` / JWT Bearer grant; JWK→`crypto.PublicKey` parsing; ECDSA r‖s byte-padding |

**Rationale for adoption.** The pre-audit implementation hand-rolled RSA signature verification and JWK parsing. This was a correctness and coverage risk:

- Hand-rolling JWS signing misses alg-confusion guards (e.g. RSA key used with ES* alg), ECDSA `r‖s` byte-padding edge cases, and JWK type dispatch.
- Hand-rolling ID-token verification misses `alg:none` rejection, key-type/alg mismatch enforcement, and subtle clock-skew / claim-order issues.
- RS256-only signing limited the SDK to a sub-baseline profile; SMART mandates RS384/ES384 as the `client-confidential-asymmetric` SHALL baseline.

`go-jose/v4` and `go-oidc/v3` are widely deployed, receive regular security audits, and are the de-facto Go substrate for JOSE/OIDC. The supply-chain cost (three additional transitive deps) is accepted in exchange for crypto correctness. The previous OTel-only rule was a heuristic, not a hard constraint — it served early development when auth was minimal. At SMART-on-openEHR scope, maintaining that heuristic would require writing and owning security-sensitive crypto code the ecosystem already provides correctly.

**Scope boundary.** The new deps are consumed only within `auth/` and `smart/`. Packages without auth concerns (`openehr/rm`, `openehr/aql`, `openehr/serialize`, `openehr/validation`, `transport`, etc.) do not transitively import them.

**Evidence.** Auth conformance probes PROBE-001..009 pass in Sandbox. RS384/ES384 baseline is met (`auth/jwtbearer.ClaimsSigner` default = RS384; ES384 supported). ID-token alg agility covers RS256/RS384/ES256/ES384 (`smart.ValidateIDToken` via `go-oidc/v3`). The `alg:none` and alg/key-type-mismatch paths are both rejected (unit-tested in `smart/idtoken_test.go`). Hand-rolled RSA verify and `rsaPublicKeyFromJWK` have been removed.

### (c) `auth.FromOAuth2TokenSource` adapter — feasible in-core, deferred

Since `golang.org/x/oauth2` is now a core dependency, a thin adapter bridging an `oauth2.TokenSource` into the SDK's `auth.TokenSource` is feasible without a separate submodule:

```go
// Possible future API (not implemented in this audit):
func FromOAuth2TokenSource(src oauth2.TokenSource) TokenSource
```

The primary bridging caveat: `oauth2.TokenSource.Token()` has no `context.Context` parameter, so the adapter must manage context propagation via closure (capturing the context at bridge construction, or accepting a background context for the inner call and relying on `oauth2`'s own context handling). This is a known `x/oauth2` API wart.

**Decision:** this adapter is **not built in this audit** — no current SDK consumer needs it, and the SDK's own `auth/smart`, `auth/clientcreds`, and `auth/jwtbearer` already use their own `auth.TokenSource` implementations directly. The adapter is recorded here as an available, low-cost follow-up once a consumer need arises.

**Issuer-matching helper.** An issuer-keyed multi-EHR helper (analogous to the `issMatch` function in SMART client.js, routing per-request contexts to the correct `auth.TokenSource` via an issuer-keyed discovery cache + `auth.WithTokenSource`) is a possible follow-up built on the per-request `auth.WithTokenSource` pattern and `smart/discovery`'s issuer-keyed resolver. Not implemented in this audit.

## Alternatives considered

**Continue hand-rolling under the OTel-only rule.**
Rejected. Maintaining hand-rolled JOSE signing and ID-token verification at the RS384/ES384/ES256/RS256 multi-alg level is non-trivial ongoing maintenance and carries a material risk of subtle crypto bugs (alg confusion, ECDSA byte-padding, JWK type dispatch). The OTel-only rule was always a heuristic; the correct application of it is "avoid unnecessary deps", not "avoid deps for security primitives the ecosystem provides correctly".

**Use `golang.org/x/oauth2` as an opaque auth engine (full delegation).**
Considered and partially applied. `x/oauth2`'s PKCE helpers (`oauth2.GenerateVerifier`, `oauth2.S256ChallengeFromVerifier`) generate the library's `code_verifier`/`code_challenge` in `auth/smart/pkce.go` (and back the RFC 7636 parity check in PROBE-004). However, the SDK needs its own `TokenSource` abstraction (with `ExpiresAt`, `Issuer`, `Scope` fields and `context.Context`-first semantics), its own token-endpoint exchange/refresh, and its own status-aware error type, so `x/oauth2` is used as a focused library, not as the top-level auth engine. This keeps the SDK's API shape independent of `x/oauth2`'s API surface.

**Use a different JOSE library (`lestrrat-go/jwx`, `square/go-jose`).**
Not evaluated in depth. `go-jose/v4` is the direct successor of `square/go-jose/v2` and the dependency already required by `go-oidc/v3`. Adding a second JOSE library for signing would duplicate the crypto surface with no benefit.

## Consequences

- The `auth/` and `smart/` packages depend on `golang.org/x/oauth2`, `github.com/coreos/go-oidc/v3`, and (transitively) `github.com/go-jose/go-jose/v4`.
- The OTel-only runtime dependency rule is retired. The new guidance (recorded in `docs/architecture.md`) is: prefer the standard library and established Go ecosystem libraries for security-sensitive primitives; avoid unnecessary deps for non-security concerns.
- `auth.ScopeOfflineAccess` / `auth.ScopeOnlineAccess` are the canonical way to request a refresh token in authorization scope strings (REQ-063, `offline_access` / `online_access`).
- The `FromOAuth2TokenSource` adapter and the issuer-matching multi-EHR helper remain open follow-ups with no blocking dependency.
- STRAND-05 is closed.
