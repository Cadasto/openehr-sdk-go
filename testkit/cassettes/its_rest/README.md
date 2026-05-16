# testkit/cassettes/its_rest

Vendored fixtures for openEHR REST 1.1.0-development (REQ-050, REQ-095) and the SMART discovery contract (REQ-070..072). Cassettes are checked in so CI does not require a live deployment (REQ-082).

## Authoritative source

Endpoint shapes track the upstream OpenAPI YAML (REQ-095):

```
https://github.com/openEHR/specifications-ITS-REST/tree/master/computable/OAS
```

Pinned commit: `8e0a2a5d04ddb91cfa6c0c7ed68b9c89b9e3ad6c` (2026-04, ITS-REST 1.1.0-development WIP). Update this line — and the affected cassettes — when bumping the pin.

## Layout

| Sub-directory | Format | Used by |
|---|---|---|
| `errors/` | openEHR REST error envelopes (REQ-093) | `transport/` error-mapping tests; PROBE-068 |
| `discovery/` | SMART configuration document + JWKS | `smart/discovery/` tests; PROBE-001, PROBE-002, PROBE-040, PROBE-041 |

Resource-level cassettes (Composition POST/GET/PUT/DELETE, EHR_STATUS, Directory, AQL, Templates) are **deferred** until the corresponding leaf clients in `openehr/client/{system,ehr/*,query,definition}/` land in Phases 2–6 of [`docs/plans/2026-05-15-rest-api-client.md`](../../../docs/plans/2026-05-15-rest-api-client.md). The provenance README and the error/discovery cassettes here are the Phase 0 foundation; each later phase adds its cassette directory under `its_rest/` with its own provenance subsection.

## Provenance

### `errors/`

Hand-crafted error envelopes that match the REQ-093 shape (`{message, code, coded_text?}`) plus the documented HTTP status semantics. Each file is named for the status code it represents.

| File | Status | Scenario |
|---|---|---|
| `400.json` | 400 Bad Request | Composition violates template constraints |
| `401.json` | 401 Unauthorized | Bearer token missing or rejected |
| `403.json` | 403 Forbidden | Token valid but lacks required scope |
| `404.json` | 404 Not Found | Versioned-object id does not exist |
| `409.json` | 409 Conflict | Stale `If-Match` (version conflict) |
| `412.json` | 412 Precondition Failed | `If-Match` syntactically rejected by backend |
| `428.json` | 428 Precondition Required | PUT without `If-Match` against versioned resource |

The envelopes are deliberately small and language-agnostic so the same cassettes are reusable across the Go and PHP SDKs (REQ-080, REQ-081). When a real deployment surfaces a richer envelope (e.g. `coded_text` populated against an openEHR terminology), refresh from that deployment and record the source commit here.

### `discovery/`

Hand-crafted SMART configuration document that satisfies the openEHR SMART discovery contract: standard SMART App Launch fields (`authorization_endpoint`, `token_endpoint`, `jwks_uri`, `scopes_supported`, `response_types_supported`, `code_challenge_methods_supported`) plus the openEHR `services` extension (REQ-070) carrying `org.openehr.rest` with a parseable `base_url` and a declared `spec_version`.

| File | Notes |
|---|---|
| `smart-configuration.json` | Reference SMART config advertising `org.openehr.rest` at spec_version `1.1.0-development`. |
| `smart-configuration-mismatch.json` | Variant advertising `1.0.3` — exercises PROBE-003 (spec-version mismatch fails fast at discovery). |
| `jwks.json` | Reference JWKS document with two RS256 keys; used to exercise JWKS rotation (PROBE-006). |

## Conventions

- Cassettes are immutable inputs. Never hand-edit a vendored cassette to make a test pass — fix the codec or open a follow-up to refresh from upstream.
- New cassette directories require a row in the Layout table and a Provenance subsection.
- Cassettes that exercise SDK-emitted bytes (e.g. round-trip outputs) live next to their test as `testdata/`, not here.
