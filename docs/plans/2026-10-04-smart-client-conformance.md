---
kind: plan
---

# Plan — SMART client conformance

**Date:** 2026-10-04
**Status:** Draft — phases 0, 1 and 5 delivered (PRs 209, 212 and 211); phases 2 to 4 open
**Covers:** [REQ-060](../specifications/auth.md#req-060) through [REQ-068](../specifications/auth.md#req-068--flow-and-launch-mode-coverage), [REQ-070](../specifications/service-discovery.md#req-070) through [REQ-073](../specifications/service-discovery.md#req-073--discovery-trust-posture), the unnumbered [Scope handling](../specifications/auth.md#scope-handling) section, and new requirements at the next free numbers for token revocation, scope syntax and the bearer challenge
**Depends on:** nothing outside this plan
**Defers:** discovery and JWKS HTTP caching semantics, relative endpoint URLs, the `launch-base64-json` decoder, the future of `auth/introspect`, and the watch list at the end

This header is for the reader. No tool reads it, and nothing fails when it is missing or out of date. The work itself meets the [Definition of Ready](../development-process.md#definition-of-ready) before it starts and the [Definition of Done](../development-process.md#definition-of-done) in the implementing PR.

## Goal

Bring the SDK's SMART, OAuth 2.0 and OpenID Connect **client** surface in line with the standards it builds on, so it works against every conformant SMART on openEHR Platform and fails closed where those standards say a client refuses. The SDK is a client only: it talks to authorization servers and calls openEHR REST with the token it gets. Platform-side rules (authorization server, resource-server token validation) are used only to describe what a client can expect.

## How conflicts are settled

Each gap is filed under the standard that owns the rule: the OAuth 2.0 family (RFC 6749, 6750, 7009, 7636, 8414, 9207, 9700), OpenID Connect (Core 1.0, Discovery 1.0), JOSE (RFC 7515, 7517, 8725), HL7 SMART App Launch (2.1.0 floor, 2.2.0 target) and SMART on openEHR. SMART on openEHR is the least mature of these. Where it disagrees with one of the others, or is silent, the upstream standard decides and the openEHR wording is noted as the conflicting source.

## Phases

Each phase is one pull request in its own worktree under `.worktrees/`. Phases 1 to 4 stack, because they edit the same discovery and authorization-code code paths; phase 5 starts from `main`.

### Phase 0 — wire fixes that restore spec'd behaviour (maintenance lane)

**Branch:** `fix/smart-client-wire-conformance`, from `main`.

**Tasks:**
- D1: the discovery document URL keeps the issuer's path (`{base}/.well-known/smart-configuration`).
- O1: the authorize URL keeps the query the authorization endpoint already carries (RFC 6749 §3.1).
- O2: `client_secret_basic` form-encodes the client id and secret in `auth/smart` as `auth/clientcreds` already does (RFC 6749 §2.3.1).
- O7: a `token_type` of `bearer` in any letter case becomes the `Bearer` scheme in all three token-endpoint providers (RFC 6749 §5.1).

**Definition of done:** one failing test per bug committed before its fix; `make ci` green; PR body carries the maintenance-lane line.

### Phase 1 — discovery model

**Branch:** `feat/smart-discovery-model`, stacked on phase 0.

**Tasks:**
- S1: the catalog records the Platform base URL (the discovery root, the embedded-launch `iss`) and the OIDC issuer (the document's `issuer`, which ID tokens are checked against) as two values. A document whose `issuer` differs from the base URL is accepted when it is an absolute `https` URL; an opt-in check compares it with `{issuer}/.well-known/openid-configuration`, where OIDC Discovery §4.3 applies. Recorded in a new ADR, because it reverses the current issuer-match rule.
- S2: `aud` is always sent, defaulting to the Platform base URL (HL7 SMART: `aud` equals `iss`); the REQ-061 text that names the openEHR REST base changes accordingly.
- S3: `authorization_endpoint` is required only when the document advertises `launch-ehr` or `launch-standalone` (SMART 2.2.0); `jwks_uri` only with `sso-openid-connect`; `token_endpoint` whenever any auth member is present.
- S8: an authorization-code source refuses a server whose `code_challenge_methods_supported` is present and lacks `S256`.
- E1: service entries surface the canonical `version`, `description`, `documentation` and `openapi`; the version gate keeps its current default and reads `version` only in strict mode.
- REQ-065 binds a client to one Platform base URL rather than to "one issuer".

**Definition of done:** spec amendments and the ADR in the same PR; tests citing the amended REQs; `make spec-gen` output committed; `make ci` green.

### Phase 2 — completing the authorization

**Branch:** `feat/smart-authorization-completion`, stacked on phase 1.

**Tasks:**
- O3: one call completes the redirect: it checks `state`, turns an `error` response (RFC 6749 §4.1.2.1) into a typed error, checks the RFC 9207 `iss` parameter against the issuer the request was started with, and records that issuer on `AuthorizationRequest` (RFC 9700 §4.4 mix-up defence).
- S4: an embedded-launch helper reads `iss` and `launch` from the launch URL and checks `iss` against a caller allowlist; a request carrying `launch` always includes the `launch` scope (HL7 SMART).
- I1: the ID token is validated as part of completing the authorization, not only when the application builds a launch context (OIDC Core §3.1.3.7). This likely moves ID-token validation next to the token exchange in `auth/smart`, keeping the `smart` names as aliases; recorded in an ADR if the package boundary moves.
- I2: a nonce is generated and sent when `openid` is requested, and checked on the ID token; the token's own `nonce` claim is surfaced.
- I3: extra ID-token audiences are refused unless the caller trusts them, and `azp`, when present, equals the client id.
- I4: an ID token without `kid` is verified when the JWKS holds exactly one suitable key (OIDC Core §10.1).
- I5: an ID token returned by a refresh is checked against the original (OIDC Core §12.2).
- S7: `LaunchContext.User` comes from the verified ID token only.

**Definition of done:** as phase 1.

### Phase 3 — token lifecycle

**Branch:** `feat/smart-token-lifecycle`, stacked on phase 2.

**Tasks:**
- S5: confidential clients leave `client_id` out of token and refresh requests (HL7 SMART token request table).
- S6: a SMART client assertion carries a `kid` and expires at most five minutes after issue (HL7 SMART asymmetric profile).
- O6: a token-change callback lets an application persist a rotated refresh token (RFC 6749 §6, RFC 9700 §4.14).
- O8: sign-out revokes the refresh token at the advertised `revocation_endpoint` (RFC 7009); new requirement.
- A refresh response that omits the launch context (`ehrId`, `patient`, `episodeId` and the rest, which SMART lets a server leave out) keeps the earlier context on the source's last token response, so a launch context rebuilt after a refresh does not lose it.

**Definition of done:** as phase 1.

### Phase 4 — backend services and the transport

**Branch:** `feat/smart-backend-transport`, stacked on phase 1.

**Tasks:**
- O5: `auth/clientcreds` and `auth/jwtbearer` drop a cached token on request, so the transport's 401 safety net works for them.
- O4: the transport reads the RFC 6750 `WWW-Authenticate` challenge on 401 and 403, exposes its error code and required scope, and retries after a 401 only for `invalid_token` or a challenge without an error code; new requirement.
- Backend services build from a resolved catalog: token endpoint, client-authentication method cross-check, grant type and signing-algorithm cross-checks.

**Definition of done:** as phase 1.

### Phase 5 — openEHR scope syntax and the backend-flow wording

**Branch:** `feat/openehr-scope-syntax`, from `main`.

**Tasks:**
- E2: a typed openEHR scope builder and parser (compartment, `template` / `composition` / `aql`, pattern, permissions in `cruds` order, the RFC 6749 §3.3 scope-token character set, the last-dot rule). Syntax only; the SDK still applies no scope policy. The Scope handling section gets a requirement id.
- E3: REQ-068 presents SMART Backend Services as `client_credentials` with a signed client assertion (`auth/clientcreds`), and the RFC 7523 §2.1 grant (`auth/jwtbearer`) as a separate, non-SMART flow.

**Definition of done:** as phase 1.

## Deferred and watch list

- HTTP caching semantics for the discovery document and the JWKS (a 304 extending the cached entry, `Cache-Control` on the JWKS, stale-if-error).
- Relative endpoint URLs in the discovery document.
- A decoder for the experimental `launch-base64-json` launch value.
- Whether `auth/introspect`, a resource-server helper, stays in a client-only SDK: a maintainer decision.
- Watch: the RFC 7523 update (client-assertion `aud` as the issuer, `typ: client-authentication+jwt`) once SMART adopts it; URI forms of the openEHR capability names; DPoP; SMART 2.2.0 `associated_endpoints` and `authorization_details`.
