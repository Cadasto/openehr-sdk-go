# Plan — REQ-094 `Prefer` follow-ups (identifier + representation guard)

**Date:** 2026-05-25
**Status:** Landed
**Owner:** SDK maintainers
**Covers:** [REQ-094](../../specifications/transport.md#req-094--prefer-response-shape-negotiation)
**Probes:** PROBE-065 (`minimal` round-trip — separate plan scope, still deferred); no new probe IDs added
**Implementation:** **landed** — write-path `identifier` slot population and strict `representation` empty-body handling shipped across `composition` / `directory` / `ehr_status` (shared `doWrite`); see [transport.md § REQ-094](../../specifications/transport.md#req-094--prefer-response-shape-negotiation)
**Depends on:** [2026-05-15-rest-api-client.md](2026-05-15-rest-api-client.md) Phase 2 (`transport.WithPrefer`, leaf `doWrite`); SDK-GAP-09 landed in PR #17 (`return=representation` bare-body decode for composition/directory)
**Defers:** PROBE-065 minimal→GET round-trip (keep in REST client plan Phase 3+)

## Goal

Close the remaining REQ-094 gaps on versioned **write** paths after SDK-GAP-09: populate the **identifier** return slot when the server honours `Prefer: return=identifier`, and surface **`ErrInvalidShape`** (or equivalent typed error) when `Prefer: return=representation` is requested but the response body is empty — per [transport.md § REQ-094](../../specifications/transport.md#req-094--prefer-response-shape-negotiation) ("MUST NOT silently downgrade").

**Out of scope here:** changing SDK-GAP-09 bare-body decode for `representation` (landed); PROBE-071 / PROBE-061 representation probes.

## Landed behaviour (was: "not landed" search keywords)

| Item | Flag | Behaviour |
|---|---|---|
| `Prefer=identifier` | **`LANDED`** | `doWrite` decodes the ITS-REST `Identifier` body (`{"uid": …}`) and populates `VersionMetadata.VersionUID` when `Location` did not already supply it |
| `representation` + empty body | **`LANDED`** | `doWrite` returns `transport.ErrInvalidShape` instead of `(nil, meta, nil)` when `len(resp.Body)==0` |
| `VersionMetadata` identifier slot | **`LANDED`** | The existing `VersionUID` field is the slot; `ehr.Identifier` + `(*VersionMetadata).ResolveIdentifierBody` decode the oneOf identifier arm |

## Implementation checklist

| Step | Status |
|---|---|
| Spec note in [transport.md](../../specifications/transport.md#req-094--prefer-response-shape-negotiation) pointing here | done |
| `transport` / leaf: decode `Identifier` body → metadata (and/or typed wrapper) | done (`ehr.Identifier` + `ResolveIdentifierBody`) |
| `doWrite`: `representation` + empty body → typed error | done (composition / directory / ehr_status) |
| Unit tests: identifier POST/PUT; empty-body representation error | done (3 leaves) |
| PROBE-065 or dedicated probe wiring | deferred (PROBE-065 minimal→GET round-trip stays in REST client plan) |
| `traceability.yaml` REQ-094 notes updated when landed | done |
| `make spec-check` / `make ci` | done |

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

- [transport.md § REQ-094](../../specifications/transport.md#req-094--prefer-response-shape-negotiation) — normative table (`minimal` / `identifier` / `representation`)
- [conformance.md § PROBE-065](../../specifications/conformance.md#probe-065--prefer-returnminimal-on-post-returns-identifier-only) — related minimal→GET probe (deferred)
- PR #17 / SDK-GAP-09 — `representation` bare `COMPOSITION`/`FOLDER` only; explicitly **does not** close this plan

## References

- Deferred in commit `c19fddc` on branch `feat/sdk-gap-09-composition-save-update-decode`
- `openehr/client/ehr/composition/composition.go` — `doWrite` guard at representation branch
