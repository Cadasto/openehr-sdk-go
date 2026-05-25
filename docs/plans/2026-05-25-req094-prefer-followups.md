# Plan — REQ-094 `Prefer` follow-ups (identifier + representation guard)

**Date:** 2026-05-25
**Status:** Draft
**Owner:** SDK maintainers
**Covers:** [REQ-094](../specifications/transport.md#req-094--prefer-response-shape-negotiation)
**Probes:** PROBE-065 (`minimal` round-trip — separate plan scope); no new probe IDs yet
**Implementation:** **not landed** — do not treat REQ-094 as fully satisfied for write-path `identifier` or strict `representation` body rules until this plan closes
**Depends on:** [2026-05-15-rest-api-client.md](2026-05-15-rest-api-client.md) Phase 2 (`transport.WithPrefer`, leaf `doWrite`); SDK-GAP-09 landed in PR #17 (`return=representation` bare-body decode for composition/directory)
**Defers:** PROBE-065 minimal→GET round-trip (keep in REST client plan Phase 3+)

## Goal

Close the remaining REQ-094 gaps on versioned **write** paths after SDK-GAP-09: populate the **identifier** return slot when the server honours `Prefer: return=identifier`, and surface **`ErrInvalidShape`** (or equivalent typed error) when `Prefer: return=representation` is requested but the response body is empty — per [transport.md § REQ-094](../specifications/transport.md#req-094--prefer-response-shape-negotiation) ("MUST NOT silently downgrade").

**Out of scope here:** changing SDK-GAP-09 bare-body decode for `representation` (landed); PROBE-071 / PROBE-061 representation probes.

## Not landed (search keywords)

Use these when grepping the repo later:

| Item | Flag | Current behaviour (pre-fix) |
|---|---|---|
| `Prefer=identifier` | **`NOT LANDED`** | `composition.Save`/`Update` and `directory.Save`/`Update` return `nil` typed resource; identifier body is discarded (`doWrite` only decodes on `representation`) |
| `representation` + empty body | **`NOT LANDED`** | `doWrite` returns `(nil, meta, nil)` without error when `len(resp.Body)==0` |
| `VersionMetadata` identifier field | **`NOT LANDED`** | No populated slot for ITS-REST `Identifier` oneOf arm on write responses |

## Implementation checklist

| Step | Status |
|---|---|
| Spec note in [transport.md](../specifications/transport.md#req-094--prefer-response-shape-negotiation) pointing here | pending |
| `transport` / leaf: decode `Identifier` body → metadata (and/or typed wrapper) | pending |
| `doWrite`: `representation` + empty body → typed error | pending |
| Unit tests: identifier POST/PUT; empty-body representation error | pending |
| PROBE-065 or dedicated probe wiring | pending |
| `traceability.yaml` REQ-094 notes updated when landed | pending |
| `make spec-check` / `make ci` | pending |

## Phases

### Phase 1 — Representation guard

**Tasks:**

1. In `composition.doWrite` / `directory.doWrite` (shared pattern): when `prefer == PreferRepresentation` and `len(resp.Body) == 0`, return a typed error (align with existing `transport.ErrInvalidShape` usage elsewhere).
2. Unit tests mirroring existing representation pins.

**Definition of done:** Empty body + `representation` never returns success with `nil` resource.

### Phase 2 — `Prefer=identifier`

**Tasks:**

1. Define how `*VersionMetadata` (or a small result struct) carries the ITS-REST `Identifier` payload without breaking the `(T, *VersionMetadata, error)` signature used by Save/Update.
2. Decode identifier JSON on write responses when `prefer == PreferIdentifier`.
3. Extend tests + optional PROBE-065 alignment.

**Definition of done:** Callers using `WithPrefer(PreferIdentifier)` receive a populated identifier slot; no silent discard.

## Mapping to specs

- [transport.md § REQ-094](../specifications/transport.md#req-094--prefer-response-shape-negotiation) — normative table (`minimal` / `identifier` / `representation`)
- [conformance.md § PROBE-065](../specifications/conformance.md#probe-065--prefer-returnminimal-on-post-returns-identifier-only) — related minimal→GET probe (deferred)
- PR #17 / SDK-GAP-09 — `representation` bare `COMPOSITION`/`FOLDER` only; explicitly **does not** close this plan

## References

- Deferred in commit `c19fddc` on branch `feat/sdk-gap-09-composition-save-update-decode`
- `openehr/client/ehr/composition/composition.go` — `doWrite` guard at representation branch
