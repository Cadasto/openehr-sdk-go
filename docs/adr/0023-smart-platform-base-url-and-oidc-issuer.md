---
kind: adr
id: ADR-0023
title: "SMART discovery: the services map, a soft version gate, and the base URL and issuer as two values"
status: accepted
date: 2026-10-04
---

# ADR 0023 — SMART discovery: the services map, a soft version gate, and the base URL and issuer as two values

- **Status:** Accepted, 2026-10-04 (maintainer sign-off on the proposal, choosing the cross-check on by default).
- **Amended:** 2026-11-25: takes in the shape of `services` and the soft version gate, and points at the trust posture now under REQ-072.
- **Amends:** [REQ-070](../specifications/auth.md#req-070) (the catalog carries both values and reads `services` as a map; with it the one-Platform catalog), [REQ-071](../specifications/auth.md#req-071) (the cache key), [REQ-072](../specifications/auth.md#req-072) (the version gate and the issuer rule of its *Trust posture*), [REQ-061](../specifications/auth.md#req-061--pkce-flow) (the default `aud`). **Applies:** [REQ-063](../specifications/auth.md#req-063--token-refresh-and-revocation) (revocation reads the discovered `revocation_endpoint`).
- **Related:** [ADR 0009](0009-smart-auth-library-scope.md) (the auth library scope).

## Context

**The `services` map.**
The openEHR SMART App Launch specification defines `services` as a JSON object keyed by reverse-domain service identifier, each entry carrying `baseUrl` and optional `description`, `documentation`, `openapi` and `capabilities` members.
It defines no `spec_version` on an entry, so a conformant Platform need not advertise one, while some deployments do.

**Two URLs.**
A SMART client meets two URLs.
The first is where discovery starts: SMART App Launch 2.2.0 fetches `{base}/.well-known/smart-configuration` from the API base, and its embedded launch hands the app that base as `iss`; SMART on openEHR moves the root to the Platform base URL and keeps `iss` naming it.
The second is the OpenID Connect issuer, the document's `issuer` member, which every ID token's `iss` claim must equal (OIDC Core 1.0 §3.1.3.7).
Neither specification requires the two to be equal, and SMART on openEHR expects user identity from an external identity provider, which usually has its own issuer URL.

Requiring the document's `issuer` to equal the URL it was fetched from misreads OIDC Discovery 1.0 §4.3, which governs `openid-configuration`, a document fetched from the issuer itself.
Such a rule refuses a conformant Platform whose authorization server lives elsewhere, and does not stop impersonation: a hostile Platform can serve its own authorization server at its own base and pass it.
What protects a client is checking ID tokens against the declared issuer, keys and `client_id`, and binding each authorization request to the issuer it started with (RFC 9700 §4.4).

The choice fixes the meaning of a public field and the cache key, which callers persist and compare.

## Decision

1. **`services` is the specification's map.**
   The resolver reads `services` as an object keyed by service identifier, sets each entry's `ID` from its key and surfaces its canonical members verbatim; a non-canonical `spec_version` member is tolerated and surfaced, and an array form is not read ([auth.md § REQ-070](../specifications/auth.md#req-070)).
2. **The version gate is soft unless the caller narrows it.**
   An advertised `spec_version` is always compared.
   An entry that advertises none passes, unless the caller has set `WithAcceptedSpecVersions`, in which case its `version` is compared and an entry with neither fails; without that option `version` is never compared, since a Platform advertises its own API version there ([auth.md § REQ-072](../specifications/auth.md#req-072)).
3. **The Platform base URL and the OIDC issuer are two values.**
   The base URL is what discovery is resolved from, what the cache is keyed by, what a client binds to, and the default `aud`.
   The issuer is the document's `issuer` member, or the base URL when the document declares none, and ID tokens are checked against it.
   A declared issuer that differs from the base URL is accepted when it is a well-formed `https` URL and the issuer's own OIDC discovery document confirms it; `WithoutOpenIDConfigurationCheck()` turns that confirmation off.
   The mechanics are in [auth.md § REQ-070](../specifications/auth.md#req-070) and [§ Trust posture](../specifications/auth.md#trust-posture), and the default `aud` in [auth.md § REQ-061](../specifications/auth.md#req-061--pkce-flow).

## Consequences

- Discovery works against a Platform that writes the specification's `services` map, and one that omits `spec_version` passes by default; a caller who wants strict pinning names the accepted versions.
- Platforms with an external or separately hosted authorization server resolve. Platforms that align the two URLs, as SMART on openEHR recommends, have equal `BaseURL` and `Issuer`.
- `ServiceCatalog.Issuer` means the OIDC issuer and `ServiceCatalog.BaseURL` the URL resolved; a catalog from `NewStaticCatalog` that sets only `Issuer` keeps working, because `BaseURL` defaults to it.
- When the two URLs differ, the default cross-check costs one extra fetch whenever the SMART document is fetched or a cached catalog is renewed on a `304`, and none on a fresh cache hit ([auth.md § REQ-071](../specifications/auth.md#req-071)).
  A Platform whose authorization server publishes no `openid-configuration`, which SMART App Launch does not require, is refused until the caller turns the cross-check off.
- The authorization request always carries `aud`, so a source built without a catalog names its audience.
- Discovery surfaces `introspection_endpoint` and `management_endpoint` without using them, since introspection is a resource-server operation outside the SDK's client scope; `revocation_endpoint` is what `(*auth/smart.Source).Revoke` posts to.
- A stable issuer value makes it possible to bind each authorization request to its issuer, the defence against mix-up for a multi-Platform client; this decision does not deliver it.

## Alternatives considered

- **Read the array shape as well as the map.** Rejected: no conformant Platform writes it, and a second decoder hides the regression the SDK's own fixture once hid.
- **Require `spec_version` and enforce it strictly.** Rejected: the specification does not define it, so it would refuse every conformant deployment.
- **Never compare `spec_version`.** Rejected: deployments that write it expect a caller to be able to pin it.
- **Keep one value, requiring the issuer to equal the base URL.** Rejected, as *Context* says.

## Traceability

- Amends: REQ-070, REQ-071, REQ-072, REQ-061
- Applies: REQ-063
