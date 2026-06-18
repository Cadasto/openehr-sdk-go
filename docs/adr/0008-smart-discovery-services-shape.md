# ADR 0008 — SMART discovery: canonical `services` map shape

- **Status:** Accepted, 2026-06-17.
- **Supersedes:** —
- **Superseded by:** —
- **Tracks:** [`docs/plans/2026-06-16-auth-smart-conformance-audit.md`](../plans/2026-06-16-auth-smart-conformance-audit.md) Phase 1 / Task 1 (F-A). Fixes: REQ-070 (wire decoder), REQ-072 (version-gate softening).

## Context

The openEHR SMART App Launch specification
([ITS-REST/development § SMART App Launch](https://specifications.openehr.org/releases/ITS-REST/development/smart_app_launch.html))
defines `services` as a JSON **object/hash map** keyed by reverse-domain service
identifier, each value carrying **`baseUrl`** (camelCase) plus optional
`description`, `documentation`, `openapi`, and `capabilities` fields:

```json
"services": {
  "org.openehr.rest": {
    "baseUrl": "https://platform.example.com/openehr/rest/v1",
    "description": "openEHR REST API"
  }
}
```

The SDK's wire decoder (`smartConfigWire` / `serviceEntryWire` in
`smart/discovery/resolver.go`) was decoding `services` as a **JSON array**,
with each element carrying an `"id"` field and snake_case `"base_url"` (plus
an invented `"spec_version"` field per entry). This is non-canonical: no
conformant platform emits that shape. Against a real deployment, `services`
decoded to an empty map, `validate` reported `org.openehr.rest` missing, and
discovery failed entirely. The bug was masked only because the SDK's own test
fixture used the same non-canonical array shape.

The domain model (`ServiceCatalog.Services map[string]ServiceEntry`) was
already correct — only the wire decoder and the version-gate enforcement
were wrong.

A second issue: `validate` enforced `spec_version` unconditionally against the
accepted set. The canonical spec does not require services to advertise a
`spec_version`; gating hard on its absence causes interop failures with
compliant platforms that omit the field.

## Decision

### (a) Wire shape: adopt canonical JSON object/map with camelCase `baseUrl`

`smartConfigWire.Services` is now `map[string]serviceEntryWire` (keyed by
service ID). `serviceEntryWire` uses `json:"baseUrl"` (camelCase) and drops
the invented `"id"` field (the key is the ID). `parse` ranges over the map;
`ServiceEntry.ID` is set from the map key.

`spec_version` is retained as a tolerated non-canonical extension on the wire
struct (`json:"spec_version"`) — some internal deployments may emit it.

New top-level wire fields (`introspection_endpoint`, `revocation_endpoint`,
`management_endpoint`) are surfaced onto `AuthEndpoints` (finding F-K, landed —
see `smart/discovery/resolver.go`, tested in `resolver_test.go`).
`introspection_endpoint` is consumed by the opt-in RFC 7662 client
`auth/introspect` (REQ-062); `revocation_endpoint` and `management_endpoint`
remain surface-only (no client wired yet).

### (b) Version gate: soften — only enforce when version is advertised or caller is strict

`validate` now skips the `spec_version` check when:
- the service entry does **not** advertise a `spec_version` (`""` on wire), AND
- the caller has **not** explicitly called `WithAcceptedSpecVersions(...)`.

Strict enforcement remains the default when either condition is met: the
`resolverConfig` tracks `acceptedVersionsLocked bool` which `WithAcceptedSpecVersions`
sets to `true`. This preserves backward compatibility for callers who rely on
strict version pinning.

### (c) Legacy array back-compat: dropped

The non-canonical array+`base_url`+`spec_version` shape is not supported. The
SDK is pre-1.0; the array shape was never canonical (it appeared in the SDK's
own invented fixture, not in the spec). Carrying a tolerant dual-decoder would
preserve a shape no conformant platform emits and would mask future regressions
the same way the original bug was masked.

## Alternatives considered

**Keep legacy array support for one release with a deprecation warning.**
Rejected. The array shape was never advertised as a public interface; it was an
internal implementation detail exposed only in tests. Pre-1.0 means no
compatibility obligations apply. A dual-decoder adds non-trivial complexity and
a subtle test-coverage gap (you must test both paths in perpetuity).

**Add `spec_version` as a first-class required field and enforce strictly.**
Rejected as the starting point for this fix. The canonical spec does not define
it; real-world openEHR platforms are unlikely to emit it. Enforcing a
non-canonical field strictly would break every compliant deployment.

**Silently ignore spec_version entirely (never enforce).**
Rejected. Some Cadasto deployments do emit `spec_version`; callers who want
strict pinning should be able to get it via `WithAcceptedSpecVersions`. The
softened gate satisfies both camps: permissive by default, strict on demand.

## Consequences

- Discovery works correctly against any conformant openEHR SMART platform.
- `TestResolveCanonicalServicesMap` (new) verifies the canonical map shape
  end-to-end (REQ-070).
- All existing tests updated to use the canonical fixture shape (REQ-072).
- `TestResolveSpecVersionMismatch` continues to enforce the mismatch path via
  `WithAcceptedSpecVersions` — the mismatch fixture still advertises
  `spec_version: "1.0.3"`, which triggers the strict path.
- The `acceptedVersionsLocked` sentinel is an implementation detail of
  `resolverConfig`; it is not exposed in the public API.
