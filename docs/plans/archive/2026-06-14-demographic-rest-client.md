# Plan — Demographic REST client (`openehr/client/demographic/`)

**Date:** 2026-06-14
**Status:** Archived — landed (Phases 1–3) — `Create / Get / Update / Delete` over the five typed PARTY resources (Phase 1) + the read-only `versioned_party` family (Phase 2), polymorphic decode via `typereg.DecodeAs[rm.Party]`, and PROBE-073 (Phase 3). Deferred to separate tracks: demographic-specific AQL helpers; MPI / identity-federation.
**Owner:** SDK maintainers
**Covers:** the openEHR **Demographic** API (PARTY hierarchy CRUD) over the existing transport stack; REQ-013, REQ-020..026 (idiom + building-block), REQ-040 (`_type` registry / RM polymorphism), REQ-054 (optimistic concurrency). Reserves Demographic conformance probes.
**Depends on:** the landed REST client foundation — `transport/`, `auth/`, `smart/discovery/`, `openehr/rm/`, `openehr/serialize/canjson/` — and the canonical client shape established in [`2026-05-15-rest-api-client.md`](2026-05-15-rest-api-client.md) (Phases 1–6). Split out of that plan (its Phase 7) so the landed client family could be archived.
**Defers:** demographic-specific AQL helpers (covered by the AQL builders plan); MPI / identity-federation policy (separate research track).

## Why a separate plan

The openEHR REST client family ([`2026-05-15-rest-api-client.md`](2026-05-15-rest-api-client.md)) is landed for System, EHR (+ sub-resources), Query, Definition, and Admin. The **Demographic** API was the one open functional area in that plan (Phase 7, `doc.go` only). It is tracked here as its own deliverable; the parent plan is archived.

The ITS-REST Demographic API is `Status: development` upstream — so `openehr/client/demographic/` ships as **Draft**: breaking changes are possible between SDK minor versions until the upstream stabilises (documented in the package `doc.go`).

## Surface

The Demographic API mirrors the EHR versioned-resource pattern over the PARTY hierarchy. Same canonical client shape as the other leaves (modelled on the `composition` leaf) — package-level functions over a `*transport.Client`, with a `Repository` convenience for DI seams (REQ-023); `ctx` first (REQ-020); functional options (REQ-022); typed errors (REQ-025); goroutine-safe (REQ-026).

```go
package demographic

type Type string // person | organisation | group | agent | role

func Create(ctx context.Context, c *transport.Client, party rm.Party, opts ...WriteOption) (rm.Party, *ehr.VersionMetadata, error)
func Get(ctx context.Context, c *transport.Client, t Type, ref ehr.Ref) (rm.Party, *ehr.VersionMetadata, error)
func Update(ctx context.Context, c *transport.Client, t Type, voID ehr.VersionedObjectID, ifMatch string, party rm.Party, opts ...WriteOption) (rm.Party, *ehr.VersionMetadata, error)
func Delete(ctx context.Context, c *transport.Client, t Type, versionUID ehr.VersionUID, ifMatch string) (*ehr.VersionMetadata, error)
```

**Spec-grounded corrections to the original sketch** (canonical OpenAPI, ITS-REST development):

- **No generic `/demographic/party` endpoint.** Each concrete PARTY type is its own resource — `POST /demographic/{person|organisation|group|agent|role}`, `GET|PUT|DELETE /demographic/{type}/{uid_based_id}`. `Create` derives the segment from the value's concrete type; the read/update/delete paths take the `Type` explicitly (the caller addresses an existing resource by id, not by value).
- **Polymorphic decode goes through `typereg.DecodeAs[rm.Party]`, not `transport.Decode`.** `canjson.Unmarshal` (plain `json.Unmarshal`) cannot decode into the abstract `rm.Party` interface; the client reads the bare body via `c.Do` and dispatches on `_type` through the registry (REQ-040). `rm.Party` is satisfied by the concrete pointer types (`*rm.Person`, …).
- **Shared version types are reused from the EHR leaf** (`ehr.Ref`, `ehr.VersionMetadata`, `ehr.VersionedObjectID`, `ehr.VersionUID`, `ehr.MarshalAuditDetails`) — `openehr/client/demographic` imports `openehr/client/ehr`.
- **Writes:** `Prefer` (default `minimal`) + required `If-Match` on PUT → **412** on mismatch; **409 only on DELETE** (referential conflict); **no 428** in this API. Create → 201 + `Location` + `ETag`.
- **`PARTY_RELATIONSHIP` / `PARTY_IDENTITY` have no endpoints** — they are schema components carried inside the PARTY body, so the original Phase 2 ("relationships and identities through the surface") collapses to "already covered by the body round-trip".

## Phases

### Phase 1 — Party CRUD ✅ landed

1. `demographic.Create / Get / Update / Delete` over the five typed PARTY resources, decoded polymorphically via `typereg.DecodeAs[rm.Party]`. ✅
2. `ehr.VersionMetadata` reuse (ETag, Location, version UID). ✅
3. `Repository` DI seam; `WithPrefer` / `WithAuditDetails` options; `Type` constants + validation. ✅
4. httptest cassette test (`testkit/cassettes/its_rest/demographic/person.json`) covering routing-by-type, polymorphic decode, `Prefer=representation`, and If-Match enforcement. ✅ (Per-type cassettes for organisation/group/agent/role to follow.)

### Phase 2 — Versioned-party reads ✅ landed

The read-only `versioned_party` family (no client precedent — net-new):
1. `GetVersionedParty` (VERSIONED_PARTY container), `GetRevisionHistory` (REVISION_HISTORY), `GetVersion` / `GetVersionAtTime` / `GetVersionByID` (the VERSION envelope). ✅
2. **VERSION decode:** `ORIGINAL_VERSION<PARTY>` is decoded as `OriginalVersion[json.RawMessage]` (the generated `OriginalVersion[T]` unmarshaller routes its *known* polymorphic fields through `typereg.DecodeAs` but decodes the generic `Data *T` by plain `json.Unmarshal`, which cannot target the abstract `rm.Party` interface); the raw `data` is then re-decoded via `typereg.DecodeAs[rm.Party]` and surfaced on a clean `PartyVersion` (envelope fields + decoded `Party`). ✅
3. Repository extended; httptest cassette tests (versioned_party / revision_history / original_version). ✅

(`PARTY_RELATIONSHIP` / `PARTY_IDENTITY` need no work — they round-trip inside the PARTY body, already covered by Phase 1.)

### Phase 3 — Conformance probes ✅ landed

1. PROBE-073 (Demographic PARTY polymorphic round-trip) in [`../../../docs/specifications/conformance.md`](../../../docs/specifications/conformance.md) + [`testkit/probes/demographic/`](../../../testkit/probes/demographic/): create → get → get-version for each PARTY subtype decodes the `_type` discriminator back to the same concrete type (REQ-040), across the typed body (Phase 1) and the `ORIGINAL_VERSION<PARTY>` envelope (Phase 2). Wired into `traceability.yaml` (REQ-040 / REQ-050). ✅

## Definition of done

- `openehr/client/demographic/` compiles and passes cassette tests for the four PARTY types.
- Each concrete type round-trips through `typereg` (REQ-040).
- Package `doc.go` states the Draft maturity caveat (upstream `Status: development`).

## Mapping to specs

- [`../../../docs/specifications/idiom.md`](../../../docs/specifications/idiom.md) — REQ-020..026; the client shape.
- [`../../../docs/specifications/wire.md#req-054`](../../../docs/specifications/wire.md#req-054) — REQ-054; versioned-write semantics reused.
- [`../../../docs/specifications/module-layout.md`](../../../docs/specifications/module-layout.md) — `openehr/client/demographic/` placement and dependency direction.
- [`../../../docs/specifications/rm-modeling.md`](../../../docs/specifications/rm-modeling.md) — PARTY polymorphism via the type registry (REQ-040).
