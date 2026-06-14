# Plan — Demographic REST client (`openehr/client/demographic/`)

**Date:** 2026-06-14
**Status:** Planned — `openehr/client/demographic/` is a `doc.go` stub.
**Owner:** SDK maintainers
**Covers:** the openEHR **Demographic** API (PARTY hierarchy CRUD) over the existing transport stack; REQ-013, REQ-020..026 (idiom + building-block), REQ-040 (`_type` registry / RM polymorphism), REQ-054 (optimistic concurrency). Reserves Demographic conformance probes.
**Depends on:** the landed REST client foundation — `transport/`, `auth/`, `smart/discovery/`, `openehr/rm/`, `openehr/serialize/canjson/` — and the canonical client shape established in [`archive/2026-05-15-rest-api-client.md`](archive/2026-05-15-rest-api-client.md) (Phases 1–6). Split out of that plan (its Phase 7) so the landed client family could be archived.
**Defers:** demographic-specific AQL helpers (covered by the AQL builders plan); MPI / identity-federation policy (separate research track).

## Why a separate plan

The openEHR REST client family ([`archive/2026-05-15-rest-api-client.md`](archive/2026-05-15-rest-api-client.md)) is landed for System, EHR (+ sub-resources), Query, Definition, and Admin. The **Demographic** API was the one open functional area in that plan (Phase 7, `doc.go` only). It is tracked here as its own deliverable; the parent plan is archived.

The ITS-REST Demographic API is `Status: development` upstream — so `openehr/client/demographic/` ships as **Draft**: breaking changes are possible between SDK minor versions until the upstream stabilises (documented in the package `doc.go`).

## Surface

The Demographic API mirrors the EHR versioned-resource pattern over the PARTY hierarchy. Same canonical client shape as the other leaves — package-level functions over a `*transport.Client`, with a `Repository` convenience for DI seams (REQ-023); `ctx` first (REQ-020); functional options (REQ-022); typed errors (REQ-025); goroutine-safe (REQ-026).

```go
package demographic

func Create(ctx context.Context, c *transport.Client, party rm.Party, opts ...CreateOption) (*VersionMetadata, error)
func Get(ctx context.Context, c *transport.Client, ref Ref) (rm.Party, *VersionMetadata, error)
func Update(ctx context.Context, c *transport.Client, partyID rm.ObjectVersionID, ifMatch string, party rm.Party, opts ...UpdateOption) (*VersionMetadata, error)
func Delete(ctx context.Context, c *transport.Client, partyID rm.ObjectVersionID, ifMatch string) error
```

- `rm.Party` is the abstract category; concrete `Person`, `Organisation`, `Group`, `Agent` are discriminated via `_type` through the registry (REQ-040 in action) — no inheritance emulation.
- `Ref` is a sealed union (latest vs specific version), same pattern as `composition.Ref`.
- Versioned writes carry `If-Match` / `ETag` (REQ-054), identical to the EHR write paths.
- Relationships and identities (`PARTY_RELATIONSHIP`, `PARTY_IDENTITY`) are reached through the same generic surface.

## Phases

### Phase 1 — Party CRUD

1. `demographic.Create / Get / Update / Delete` for the four concrete PARTY types, decoded polymorphically via `typereg`.
2. `VersionMetadata` reuse (ETag, Location, version UID) — the shared type from the EHR client.
3. Tests against vendored cassettes (`testkit/cassettes/its_rest/demographic/`).

### Phase 2 — Relationships and identities

1. `PARTY_RELATIONSHIP` read/write through the demographic surface.
2. `PARTY_IDENTITY` handling on the owning party.

### Phase 3 — Conformance probes

1. Reserve + implement Demographic probes in [`../../docs/specifications/conformance.md`](../../docs/specifications/conformance.md) (round-trip create→get; polymorphic decode of each PARTY subtype).

## Definition of done

- `openehr/client/demographic/` compiles and passes cassette tests for the four PARTY types.
- Each concrete type round-trips through `typereg` (REQ-040).
- Package `doc.go` states the Draft maturity caveat (upstream `Status: development`).

## Mapping to specs

- [`../../docs/specifications/idiom.md`](../../docs/specifications/idiom.md) — REQ-020..026; the client shape.
- [`../../docs/specifications/wire.md#req-054`](../../docs/specifications/wire.md#req-054) — REQ-054; versioned-write semantics reused.
- [`../../docs/specifications/module-layout.md`](../../docs/specifications/module-layout.md) — `openehr/client/demographic/` placement and dependency direction.
- [`../../docs/specifications/rm-modeling.md`](../../docs/specifications/rm-modeling.md) — PARTY polymorphism via the type registry (REQ-040).
