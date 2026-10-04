---
kind: adr
id: ADR-0023
title: "SMART discovery: the Platform base URL and the OIDC issuer are separate values"
status: accepted
date: 2026-10-04
---

# ADR 0023 — SMART discovery: the Platform base URL and the OIDC issuer are separate values

- **Status:** Accepted, 2026-10-04 (maintainer sign-off on the proposal, choosing the cross-check on by default).
- **Supersedes:** —
- **Superseded by:** —
- **Amends:** [REQ-070](../specifications/service-discovery.md#req-070) (the catalog carries both values), [REQ-071](../specifications/service-discovery.md#req-071) (the cache key), [REQ-073](../specifications/service-discovery.md#req-073--discovery-trust-posture) (the issuer rule), [REQ-061](../specifications/auth.md#req-061--pkce-flow) (the default `aud`), [REQ-065](../specifications/auth.md#req-065) (what a client binds to).
- **Related:** [ADR 0008](0008-smart-discovery-services-shape.md) (the `services` map shape, unchanged); [ADR 0009](0009-smart-auth-library-scope.md) (the auth library scope, unchanged).

## Context

A SMART client meets two URLs that the current catalog holds as one value, `ServiceCatalog.Issuer`.

The first is the URL discovery starts from. SMART App Launch 2.2.0 fetches `{base}/.well-known/smart-configuration` from the API base; its embedded launch hands the app that same base as the `iss` launch parameter ("Identifies the EHR's FHIR endpoint"). SMART on openEHR moves the discovery root to the Platform base URL (the gateway) and keeps `iss` naming it. The second is the OpenID Connect issuer: the discovery document's `issuer` member, which SMART defines as "this system's OpenID Connect Issuer URL", required only with `sso-openid-connect`, and which every ID token's `iss` claim must equal (OIDC Core 1.0 §3.1.3.7). Neither SMART App Launch nor SMART on openEHR requires the two URLs to be equal. SMART on openEHR only encourages Platforms to keep a consistent `issuer` and serve FHIR at the same base "when feasible", and it expects user identity to come from an external identity provider, which usually has its own issuer URL.

REQ-073 resolved the two into one by requiring the document's `issuer` to equal the URL it was fetched from, citing OIDC Discovery 1.0 §4.3. That clause governs `openid-configuration`, whose fetch URL is built from the issuer itself, so equality holds by construction there. A `smart-configuration` document is fetched from the API base, which the issuer need not share. The effect is that a conformant Platform whose authorization server lives at another URL, for example an external OIDC provider in front of an openEHR gateway, cannot be resolved at all.

The rule's stated purpose is to stop a hostile or misconfigured server impersonating another identity provider. Requiring equality does not deliver it. The document arrives over TLS from the Platform the caller chose, and that Platform is authoritative for which authorization server it delegates to: a hostile Platform can serve its own authorization server at its own base URL and pass the equality check. What protects a client is checking ID tokens against the declared issuer, the declared keys and the client's own `client_id`, and binding each authorization request to the issuer it started with (RFC 9700 §4.4).

The choice changes the meaning of a public field and the cache key, which callers persist and compare, so reversing it later breaks them twice.

## Decision

The catalog records the Platform base URL and the OIDC issuer as two values. The base URL is what discovery is resolved from, what the cache is keyed by, what a client binds to and the default `aud`. The issuer is the document's `issuer` member, or the base URL when the document declares none, and ID tokens are checked against it. A declared issuer that differs from the base URL is accepted when it is a well-formed `https` URL and the issuer's own OIDC discovery document confirms it, which is where OIDC Discovery 1.0 §4.3 applies; a caller can turn that confirmation off. The mechanics are in [service-discovery.md § REQ-070](../specifications/service-discovery.md#req-070) and [§ REQ-073](../specifications/service-discovery.md#req-073--discovery-trust-posture), and the default `aud` in [auth.md § REQ-061](../specifications/auth.md#req-061--pkce-flow).

## Consequences

- Platforms with an external or separately hosted authorization server resolve. Platforms that align the two URLs, as SMART on openEHR recommends, see no change: their `BaseURL` and `Issuer` are equal.
- `ServiceCatalog.Issuer` keeps its name and now means the OIDC issuer; `ServiceCatalog.BaseURL` and `StaticConfig.BaseURL` are added. A caller that used `Issuer` as "the URL I resolved" moves to `BaseURL`. A hand-built catalog from `NewStaticCatalog` that sets only `Issuer` keeps working, because `BaseURL` defaults to it.
- The resolver no longer refuses a document for naming another issuer, but by default it confirms that issuer against the issuer's own OIDC discovery document, at the cost of one extra fetch per resolution or refresh when the two URLs differ. A Platform whose authorization server publishes no `openid-configuration` (SMART App Launch does not require one) is refused until the caller turns the confirmation off.
- The authorization request always carries `aud`. A source built without a catalog has to name its audience, which is a new construction error for callers that relied on omitting it.
- Binding each authorization request to its issuer, which is what actually defends a multi-Platform client against mix-up, becomes possible with a stable issuer value; this decision does not deliver it.
