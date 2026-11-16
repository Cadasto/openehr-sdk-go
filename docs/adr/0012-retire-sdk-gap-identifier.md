---
kind: adr
id: ADR-0012
title: "Retire SDK-GAP as a durable identifier; REQ/PROBE is the feature register"
status: accepted
date: 2026-07-02
---

# ADR 0012 — Retire SDK-GAP as a durable identifier; REQ/PROBE is the feature register

- **Status:** Accepted, 2026-07-02.
- **Introduces:** — (no new `REQ-NNN`). **Promotes content into:** REQ-055, REQ-057, REQ-107, REQ-083. **Applies:** the SDD conventions in [`.sdd.yaml`](../.sdd.yaml) and [`development-process.md`](../development-process.md).

## Context

`SDK-GAP-NN` grew as an ad-hoc label for implementation increments between the normative contract and the shipped code.
It is not a normative identifier: the feature register is `REQ-NNN` (registry in [`REQ.md`](../specifications/REQ.md)) plus `PROBE-NNN` conformance probes ([`conformance.md`](../specifications/conformance.md)), and open research questions are `STRAND-NN`.

Over twelve increments the label leaked into the durable surface: the trace map, spec prose, the roadmap, test file names and identifiers.
One token did two jobs.
As a delivery lens it describes a gap that no longer exists once closed; as a traceability anchor it duplicates what `REQ` and `PROBE` already do.
Nine of the twelve gaps were already fully carried by an existing `REQ` and its spec; three carried a durable fact not yet in the normative layer.

## Decision

**`SDK-GAP-NN` is retired as a durable identifier. `REQ-NNN` / `PROBE-NNN` is the sole feature register.**

1. **Going forward.** A newly found gap is worked under a `REQ`, extending one or adding one, with a `PROBE` for wire-level conformance. A GAP-style label may name an in-flight plan file only; it never enters `traceability.yaml`, test names, doc comments or normative prose.
2. **Full purge of the mutable tree.** GAP tokens leave every mutable document, `CHANGELOG.md` included, and all code. A stripped `(SDK-GAP-NN)` in a code comment is replaced by its governing `REQ`/`PROBE` token, never merely deleted. Git history is never rewritten: commit messages keep the tokens, decoded by the crosswalk below.
3. **Content is promoted before tokens are stripped.** GAP-16 → REQ-055 / REQ-057 (verb-aware EHR scoping and the `PutStoredQuery` `Location` recovery); GAP-14 → REQ-107 (the seeded value-fill seam); GAP-07 → REQ-083 (the health-probe contract).
4. **Dangling references get a home.** The deferred PROBE-077 / 078 / 079 stay as `Deferred` or `Planned` catalogue entries, and GAP-14's undelivered `medium` detail level is a `Planned` roadmap row.

## Crosswalk — `SDK-GAP-NN` → `REQ` / `PROBE` (permanent decoder)

| GAP | Capability delivered | → REQ | → PROBE | Disposition |
|---|---|---|---|---|
| **07** | Cadasto `admin` Live/Ready health probes | REQ-083 | — | **Promote** into REQ-083, then strip |
| **09** | `Prefer: return=representation` decodes the bare resource | REQ-094 (+052) | 061, 071 | Strip (already normative) |
| **10** | `contribution.Commit` takes `Contribution_create`, not the persisted shape | REQ-050, 095 | 072 | Strip (already normative) |
| **11** | `<Parent>Like` interfaces + generic-bound polymorphic decode | REQ-052, 040 | 038 | Strip (already normative) |
| **12** | Real-world OPT synthesis/validation (name-fill, generic `DV_INTERVAL<T>`, node_id dedup) | REQ-102, 107, 110 | 027 | Strip + rename tests |
| **13** | `_type` emission on value-in-interface slots; interval re-validation from bounds | REQ-052, 040, 102, 107 | 038 | Strip + rename test |
| **14** | Seeded synthetic value fill (`ValueFill`/`RandomFill`/`ValueSource`) | REQ-107 | — | **Promote** seam into REQ-107; `medium`/`detail_level` → Planned |
| **15** | `ValidateRM` + typed sugars walk any RM root via `rminfo` | REQ-112 | 077 *(deferred)* | Strip (already normative) |
| **16** | Verb-aware `openehr-ehr-id` POST scoping; `PutStoredQuery` `Location` recovery | REQ-055, 057 | 078/079 *(deferred)* | **Promote** wire facts into wire.md, then strip |
| **17** | Tier-2 `parse.Query` AST + `Emit` round-trip; `ErrIncompleteAST` | REQ-113 | 080 | Strip (already normative) |
| **18** | `ValidateRMEHRStatusBytes` — value-typed mandatory `subject` via JSON-key presence | REQ-112 | 081 | Strip (already normative) |
| **19** | `ClassExpr.PredicateComparison` + `Comparison.ParsedPath`; `IdentifiedPath` relocation | REQ-113 | 082 | Strip + rename const |

`SDK-GAP-08` was never assigned in this repository. GAP numbering does not resume; new work takes REQ identifiers.

## Consequences

- The living, normative and CHANGELOG surface is GAP-free, and contributors learn one identifier scheme (`REQ`/`PROBE`/`STRAND`).
- Git history keeps the tokens; this crosswalk decodes them, so old references stay resolvable without keeping the identifier alive.
- No content is lost: the three durable facts live in REQ-055/057/107/083, and the undelivered level and the deferred probes are tracked.

## Alternatives considered

- **Move work-tracking to GitHub issues.** Rejected: issues live outside the versioned tree, are invisible to `make spec-check`, and split the traceability chain across two systems.
- **Keep GAP as a living identifier.** Rejected: a growing list with no product meaning, and a second identifier scheme to learn.
- **Strip the living surface only, keeping archived-plan file names and released CHANGELOG lines.** Rejected in favour of a full purge, so the token survives only in git history with this ADR as its decoder.
