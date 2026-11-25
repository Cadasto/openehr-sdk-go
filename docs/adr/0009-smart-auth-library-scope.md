---
kind: adr
id: ADR-0009
title: "SMART-on-openEHR auth library scope and dependency model"
status: accepted
date: 2026-06-18
---

# ADR 0009 — SMART-on-openEHR auth library scope and dependency model

- **Status:** Accepted, 2026-06-18.
- **Amended:** 2026-11-25: decision (a) names the packages as they stand and cites the coverage table, and decision (c) states that no `oauth2.TokenSource` adapter ships.
- **Resolves:** [STRAND-05](../specifications/research-strands.md#strand-05--smart-on-openehr-auth-library). **Amends:** REQ-061, REQ-062, REQ-063, REQ-064.

## Context

**STRAND-05** asked two related questions:

1. **What auth library should the SDK build?** REQ-061..064 define the contract — PKCE, JWKS rotation, token refresh, launch context — but the scope of the implementation (which flows, which launch modes, how far along the SMART conformance profile) was open.
2. **Hand-roll or adopt ecosystem libraries?** The SDK previously held an "OpenTelemetry-only runtime dependency" rule to keep the dependency graph minimal. SMART-on-openEHR requires OAuth2 code flows, PKCE, JWKS rotation, JOSE signing (RS384, ES384 baseline), and OIDC ID-token verification. The question was whether to hand-roll these security-sensitive primitives or adopt audited ecosystem libraries.

The auth conformance audit produced concrete evidence: a full SMART-on-openEHR auth library was built across `auth/smart`, `auth/clientcreds`, `auth/jwtbearer`, `auth/basic` and `smart/discovery`, and the dependency question was resolved in favour of adopting ecosystem libraries for client-assertion signing and ID-token verification.

## Decision

### (a) Build the full SMART-on-openEHR auth library

The SDK ships the client side of SMART on openEHR across `auth/` and `smart/`: discovery in `smart/discovery` ([ADR 0023](0023-smart-platform-base-url-and-oidc-issuer.md)); the authorization-code flow with PKCE, public or confidential, ID-token verification, token refresh and revocation in `auth/smart`; SMART Backend Services and client credentials in `auth/clientcreds`; the RFC 7523 JWT Bearer grant in `auth/jwtbearer`; HTTP Basic in `auth/basic`; launch context and Platform principal claims in `smart`; the scope helpers in `auth`; and, with `transport.WithReauthOn401`, one fresh credential and one retry after a 401.
The flows and launch modes it covers are [auth.md § REQ-068](../specifications/auth.md#req-068--flow-and-launch-mode-coverage), and PROBE-001 to PROBE-009 exercise them.
Token introspection (RFC 7662) is a resource-server operation outside the SDK's client scope ([auth.md § What is NOT in scope here](../specifications/auth.md#what-is-not-in-scope-here)).

### (b) Relax the OTel-only dependency rule; adopt ecosystem security libraries

The SDK adopts three new runtime dependencies, scoped to `auth/` and `smart/`:

| Library | Used for |
|---|---|
| `golang.org/x/oauth2` | PKCE `code_verifier`/`code_challenge` generation (RFC 7636 `GenerateVerifier` / `S256ChallengeFromVerifier`) in `auth/smart/pkce.go`, plus an RFC 7636 parity cross-check in PROBE-004. (Token sources, refresh, and token-endpoint error handling are the SDK's own `auth` types — not `x/oauth2`'s.) |
| `github.com/coreos/go-oidc/v3` | ID-token signature verification (RS256/RS384/ES256/ES384 via `go-jose`) |
| `github.com/go-jose/go-jose/v4` _(direct; also required by `go-oidc/v3`)_ | JWS signing for `client_assertion` / JWT Bearer grant; JWK→`crypto.PublicKey` parsing; ECDSA r‖s byte-padding |

> **Note — dependency classification.** `go-jose/v4` is a **direct** dependency in `go.mod`: `auth/jwtbearer` and `auth/smart` import it directly for JWS signing and JWK parsing. It is *also* required by `go-oidc/v3`, so adopting it added no separate supply-chain surface.

**Rationale for adoption.** The pre-audit implementation hand-rolled RSA signature verification and JWK parsing. This was a correctness and coverage risk:

- Hand-rolling JWS signing misses alg-confusion guards (e.g. RSA key used with ES* alg), ECDSA `r‖s` byte-padding edge cases, and JWK type dispatch.
- Hand-rolling ID-token verification misses `alg:none` rejection, key-type/alg mismatch enforcement, and subtle clock-skew / claim-order issues.
- RS256-only signing limited the SDK to a sub-baseline profile; SMART makes RS384/ES384 the mandatory baseline of the `client-confidential-asymmetric` profile.

`go-jose/v4` and `go-oidc/v3` are widely deployed, receive regular security audits, and are the de-facto Go substrate for JOSE/OIDC. The supply-chain cost (three additional direct deps plus their transitive closure) is accepted in exchange for crypto correctness. The previous OTel-only rule was a heuristic, not a hard constraint — it served early development when auth was minimal. At SMART-on-openEHR scope, maintaining that heuristic would require writing and owning security-sensitive crypto code the ecosystem already provides correctly.

**Scope boundary.** The new deps are consumed only within `auth/` and `smart/`. Packages without auth concerns (`openehr/rm`, `openehr/aql`, `openehr/serialize`, `openehr/validation`, `transport`, etc.) do not transitively import them.

### (c) No `oauth2.TokenSource` adapter

The SDK's flows implement its own `auth.TokenSource`, and `golang.org/x/oauth2` serves only PKCE generation, so the SDK ships no adapter from an `oauth2.TokenSource`, whose `Token()` takes no `context.Context`, and no issuer-keyed helper that routes a request to one of several Platforms' token sources.
Either is an additive change when a consumer needs it.

## Alternatives considered

**Continue hand-rolling under the OTel-only rule.**
Rejected. Maintaining hand-rolled JOSE signing and ID-token verification at the RS384/ES384/ES256/RS256 multi-alg level is non-trivial ongoing maintenance and carries a material risk of subtle crypto bugs (alg confusion, ECDSA byte-padding, JWK type dispatch). The OTel-only rule was always a heuristic; the correct application of it is "avoid unnecessary deps", not "avoid deps for security primitives the ecosystem provides correctly".

**Use `golang.org/x/oauth2` as an opaque auth engine (full delegation).**
Considered and partially applied. `x/oauth2`'s PKCE helpers (`oauth2.GenerateVerifier`, `oauth2.S256ChallengeFromVerifier`) generate the library's `code_verifier`/`code_challenge` in `auth/smart/pkce.go` (and back the RFC 7636 parity check in PROBE-004). However, the SDK needs its own `TokenSource` abstraction (with `ExpiresAt`, `Issuer`, `Scope` fields and `context.Context`-first semantics), its own token-endpoint exchange/refresh, and its own status-aware error type, so `x/oauth2` is used as a focused library, not as the top-level auth engine. This keeps the SDK's API shape independent of `x/oauth2`'s API surface.

**Use a different JOSE library (`lestrrat-go/jwx`, `square/go-jose`).**
Not evaluated in depth. `go-jose/v4` is the direct successor of `square/go-jose/v2` and the dependency already required by `go-oidc/v3`. Adding a second JOSE library for signing would duplicate the crypto surface with no benefit.

## Consequences

- The `auth/` and `smart/` packages depend on `golang.org/x/oauth2`, `github.com/coreos/go-oidc/v3`, and `github.com/go-jose/go-jose/v4` (directly imported for JWS signing; also required by `go-oidc/v3`).
- The OTel-only runtime dependency rule is retired. The new guidance (recorded in `docs/architecture.md`) is: prefer the standard library and established Go ecosystem libraries for security-sensitive primitives; avoid unnecessary deps for non-security concerns.
- `auth.ScopeOfflineAccess` / `auth.ScopeOnlineAccess` are the canonical way to request a refresh token in authorization scope strings (REQ-063, `offline_access` / `online_access`).
- STRAND-05 is closed.

## Traceability

- Resolves: STRAND-05
- Amends: REQ-061, REQ-062, REQ-063, REQ-064
