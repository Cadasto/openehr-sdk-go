# Plan — Template-driven validation beyond COMPOSITION (demographic + EHR-IM roots)

**Date:** 2026-06-17
**Status:** Landed — archived. REQ-110 + PROBE-074 shipped; generic `Validate` + `ValidateDemographic` / `ValidateFolder` / `ValidateEHRStatus`.
**Owner:** SDK maintainers
**Covers:** generalising the REQ-102 template-driven validation walker so it validates **any** archetypeable RM root, not only `COMPOSITION`. Adds the demographic PARTY hierarchy (`PERSON`, `ORGANISATION`, `GROUP`, `AGENT`, `ROLE`) plus its archetypeable sub-components (`ADDRESS`, `CONTACT`, `PARTY_IDENTITY`, `PARTY_RELATIONSHIP`, `CAPABILITY`) and the EHR-IM roots `FOLDER` and `EHR_STATUS`. New requirement **REQ-110**; conformance **PROBE-074**.
**Depends on:** REQ-100 (OPT parse) + REQ-102 (composition validation walker) + REQ-103 (primitive constraints) — all landed. Complements the demographic REST client ([`2026-06-14-demographic-rest-client.md`](2026-06-14-demographic-rest-client.md)).
**Defers:** the canxml `DV_MULTIMEDIA` decode bug (base64 `data`/`integrity_check` mis-routed into the `size` Integer → `ParseUint`); orthogonal to validation (the validator takes an in-memory root, never decodes XML). Tracked as a known gap; tests decode via canjson / build in-memory.

## Why

The earlier hedge — "demographics have no OPT/template the way compositions do, so it's a thinner check" — was **wrong**. Demographic archetypes are standard ADL 1.4 OPTs rooted at `PERSON`/`ORGANISATION`/… (`TestPerson.v2.opt` compiles via the existing machinery to 174 nodes, root `PERSON`). The validation walker is already value-source-generic — it reads RM properties through `rmread` and recurses on the compiled OPT. Only the *closed routing sets* are COMPOSITION-scoped:

1. `rmTypeInfo` (RM type name + archetype_node_id) — `openehr/validation/composition.go`
2. `bmmSubtypes` (abstract→concrete admission) — `openehr/validation/walk_composition.go`
3. `rmread.ReadSingle` / `ReadMultiple` + per-type reader funcs — `openehr/validation/rmread/read.go`
4. `rmread.isTypedNilPointer` (typed-nil guard) — same file

These four are kept **in lock-step** (documented at `read.go` `isTypedNilPointer`). Generalising = extending all four for the new RM types + a thin public surface. No walker-logic changes.

## Surface

```go
package validation

// Validate is the generic entry: validate any archetypeable RM root
// (the value recognised by the walker's closed RM set) against a
// compiled OPT. REQ-110.
func Validate(root any, c *templatecompile.Compiled) Result

// Typed convenience wrappers (compile-time discoverable):
func ValidateComposition(comp *rm.Composition, c *templatecompile.Compiled) Result // REQ-102, now delegates
func ValidateDemographic(party rm.Party, c *templatecompile.Compiled) Result       // PERSON/ORGANISATION/GROUP/AGENT/ROLE
func ValidateFolder(folder *rm.Folder, c *templatecompile.Compiled) Result
func ValidateEHRStatus(status *rm.EHRStatus, c *templatecompile.Compiled) Result
```

`ValidateComposition` keeps its `nil_composition` guard for source compatibility, then delegates to `Validate`. `ADDRESS`/`CONTACT`/`PARTY_IDENTITY`/`PARTY_RELATIONSHIP`/`CAPABILITY` are reachable as roots through the generic `Validate` (and as children during a PARTY walk); no dedicated wrapper.

## Phases

### Phase 1 — rmread readers for the new RM types
Add per-type `readXxxSingle`/`readXxxMultiple` and the `ReadSingle`/`ReadMultiple`/`isTypedNilPointer` switch rows. Attributes (canonical snake_case json keys):
- **Person/Group/Agent/Organisation**: single `archetype_node_id`,`name`,`details`(ITEM_STRUCTURE iface); multiple `identities`,`contacts`,`relationships`,`languages`,`roles`.
- **Role**: single `archetype_node_id`,`name`,`details`; multiple `capabilities`,`contacts`,`identities`,`relationships`.
- **Address / PartyIdentity / PartyRelationship**: single `archetype_node_id`,`name`,`details`.
- **Contact**: single `archetype_node_id`,`name`; multiple `addresses`.
- **Capability**: single `archetype_node_id`,`name`,`credentials`(ITEM_STRUCTURE iface).
- **Folder**: single `archetype_node_id`,`name`,`details`; multiple `folders`,`items`.
- **EHRStatus**: single `archetype_node_id`,`name`,`subject`,`other_details`(iface),`is_modifiable`,`is_queryable`.

Value-typed slices append `&slice[k]` (so `rmTypeInfo` sees `*rm.T`), mirroring `readInstructionMultiple`. Tests in `rmread/read_test.go`.

### Phase 2 — validation routing
Extend `rmTypeInfo` (`*rm.T` + `rm.T` cases → name + `ArchetypeNodeID`) and `bmmSubtypes`:
`PARTY → {AGENT,GROUP,ORGANISATION,PERSON,ROLE}`, `ACTOR → {AGENT,GROUP,ORGANISATION,PERSON}`, and the new concretes into `LOCATABLE`. Remove the "Out of scope: FOLDER/EHR_STATUS" comment.

### Phase 3 — public API + tests
Add `Validate` + the three typed wrappers; refactor `ValidateComposition` to delegate. Tests (`composition_test.go` companions / new `noncomposition_test.go`):
- PERSON via `TestPerson.v2.opt` + decoded `TestPerson.v2.json` (canjson; in-memory fallback) → expect clean / known issues; mutate for negative `required`/`rm_type_mismatch`/`archetype_id_mismatch`.
- ADDRESS via `Address.v2.opt`.
- FOLDER / EHR_STATUS via synthetic inline OPTs + `rm/folder_*.json` / `rm/ehr_status_*.json` fixtures.
- nil/typed-nil root guards; generic `Validate` on each root.

### Phase 4 — conformance + specs
PROBE-074 (template-driven validation of non-COMPOSITION roots) in `conformance.md` + `testkit/probes/validation/` (or `…/demographic/`). REQ-110 in `REQ.md`, `clinical-modeling.md`, `traceability.yaml`.

### Phase 5 — docs + archive
Update `roadmap.md` (demographic-validator line → landed; FOLDER/EHR_STATUS notes), `architecture.md`, `validation/doc.go`. Move the demographic plans (`2026-06-14-demographic-rest-client.md`, this plan) to `docs/plans/archive/` once landed.

## Definition of done

- `Validate` + typed wrappers validate each new root against an OPT; `ValidateComposition` behaviour unchanged (existing tests green).
- The four lock-step switches extended consistently; `make ci` (fmt + golangci-lint v2 + tests) green; go-reviewer clean.
- REQ-110 + PROBE-074 wired into the spec registry; `make spec-check` / `make probe-status` pass.

## Mapping to specs

- [`../../specifications/clinical-modeling.md`](../../specifications/clinical-modeling.md) — REQ-102 (extended), REQ-110.
- [`../../specifications/rm-modeling.md`](../../specifications/rm-modeling.md) — PARTY/ACTOR hierarchy, FOLDER, EHR_STATUS.
- [`../../specifications/module-layout.md`](../../specifications/module-layout.md) — `openehr/validation` building-block independence (REQ-013) unchanged.
