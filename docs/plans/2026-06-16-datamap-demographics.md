# Plan — Datamap demographics profile (Option B)

**Date:** 2026-06-16  
**Status:** Phase 1 landed — `ToParty` / `FromParty` / party `Schema` in `cadasto/datamap`.  
**Owner:** SDK maintainers  
**Covers:** REQ-058 demographics extension; complements the composition codec.  
**Depends on:** `openehr/template`, existing datamap item-tree encode/decode; defers to [`2026-06-14-demographic-rest-client.md`](2026-06-14-demographic-rest-client.md) for REST transport.

## Goal

Let consumers read and write openEHR **Demographics** (PARTY hierarchy) through the same flat JSON + JSON Schema ergonomics as clinical compositions — without hand-building `Person`, `Organisation`, `Agent`, `Group`, `Role`, identities, contacts, addresses, or relationships.

## Two profiles, one wire convention

| Profile | OPT root | Codec entry points |
|---------|----------|-------------------|
| Composition (existing) | `COMPOSITION` | `ToComposition`, `FromComposition` |
| Party (Option B) | `PERSON`, `ORGANISATION`, `AGENT`, `GROUP`, `ROLE`, … | `ToParty`, `FromParty` |

`Schema(opt)` and `Empty(opt)` auto-select the profile via `IsPartyTemplate(opt)`.

## Party datamap shape

Top-level keys (template-driven):

- `template_id`, `uid`, `vuid`, `name`
- `identities` — object keyed by `archetype-id|Label` (PARTY_IDENTITY details tree)
- `details` — party-level ITEM_TREE items (e.g. geboortedatum, geslacht)
- `contacts` — array of contact objects keyed by `at-code|Label`, each holding address archetypes
- `relationships` — array with `source` / `target` party refs + optional details
- `roles`, `languages`, `capabilities` (ROLE) when constrained by the OPT

Cluster `_code` / `_name` conventions apply to coded ADDRESS and identity purpose names.

## Phases

### Phase 1 — Core party codec (landed)

- `IsPartyTemplate`, `ToParty`, `FromParty`, `FromPartyExpanded`
- Party JSON Schema builder
- Round-trip probe: `TestPerson.v2` cassette (identities + details + contacts)
- `ToComposition` rejects party OPTs with a directed error

### Phase 2 — REST integration

- Wire `ToParty` / `FromParty` through `openehr/client/demographic/` once Phase 1 of the demographic client lands
- `cadasto/care` or `cadasto/mpi` adapters for patient/party resolution

### Phase 3 — Conformance probes

- Reserve REQ-058 probes for party round-trip (PERSON, ORGANISATION, AGENT, GROUP, ROLE)
- Expand cassette coverage beyond `TestPerson.v2`

## Out of scope (unchanged)

- **Option A** — top-level composition `subject` as `PartyRef` (separate small extension on composition codec)
- Patient data embedded in clinical OPT `content` (already works via composition profile)
