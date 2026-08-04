# Implementation plans

Active and archived implementation plans for `openehr-sdk-go`. Plans derive from [`../../docs/specifications/`](../../docs/specifications/) — they translate normative REQs into sequenced delivery.

**Landed vs planned checklist:** [`../roadmap.md`](../roadmap.md). **Completed or superseded plans:** [`archive/README.md`](archive/README.md).

## Active plans

### Ecosystem fit-gap delivery (2026-07-16)

Prioritised from a peer-SDK ecosystem fit-gap review. Each plan authors its REQ spec prose (Phase 0, via `sdd-specify`) before implementation starts. The proposed REQ IDs follow the [numbering policy](../specifications/REQ.md#numbering-policy) topic bands — the two authoring validators land in the clinical-modeling headroom (110–119) next to REQ-109/110; the contribution builder opens a new "SDK authoring & client tooling" band (130–139) because the wire band (050–059) is exhausted.

| Plan | Scope | Covers (proposed) | Probe |
|---|---|---|---|
| [2026-07-16-flat-author-linter.md](2026-07-16-flat-author-linter.md) | Pre-submit FLAT path linter + CLI | REQ-115 (builds on REQ-053/106/111) | PROBE-083 |
| [2026-07-16-opt-author-validator.md](2026-07-16-opt-author-validator.md) | OPT author validator + CLI | REQ-114 (builds on REQ-100/104/106/108) | PROBE-085 |
| [2026-07-16-contribution-builder.md](2026-07-16-contribution-builder.md) | Fluent `Contribution_create` builder | REQ-130 (builds on REQ-050/059) | PROBE-084 |
| [2026-08-04-aql-expressivity-completion.md](2026-08-04-aql-expressivity-completion.md) | AQL structured-AST catalogue completion + lint-gate acceptance + builder containment algebra | REQ-117 (extends REQ-113/109/055) | PROBE-087, PROBE-088 |

The fourth plan in this set — the upstream FLAT parity harness — **landed 2026-08-01 and was archived** ([archive/2026-07-16-web-template-tests-conformance.md](archive/2026-07-16-web-template-tests-conformance.md)): PROBE-086 is Implemented (Sandbox), and REQ-080's roadmap row moved `planned → partial`.

### Simplified formats — WebTemplate + FLAT/STRUCTURED (planned umbrella)

| Plan | Scope | Covers REQs / probes |
|---|---|---|
| [2026-06-23-simplified-formats.md](2026-06-23-simplified-formats.md) | Umbrella — FLAT/STRUCTURED codecs (WebTemplate export Phase 2 **landed**; Phase 3 codecs **landed**); now carries the residual REQ-053 edge deferrals + the shared simplified-template model. The datatype-coverage residue moved to the [coverage ratchet](archive/2026-08-03-flat-coverage-ratchet.md) (landed): what remains is DV_TEXT subtype substitution as suffixes, `ctx/setting` emission, and the `_`-prefixed RM attributes (their own REQ) | REQ-053; PROBE-076 |

WebTemplate JSON export (**REQ-106**) landed as a direct slice and was archived ([archive/2026-05-22-webtemplate-export.md](archive/2026-05-22-webtemplate-export.md)); the umbrella's shared simplified-template model is deferred until REQ-053 (FLAT/STRUCTURED) gives it a second consumer.

Phase 3 — FLAT / STRUCTURED composition codecs (`openehr/serialize/simplified`, REQ-053) — **landed and was archived** ([archive/2026-07-14-flat-structured-codecs.md](archive/2026-07-14-flat-structured-codecs.md)): bidirectional codecs, `ctx/`, `|raw`/`|other`, full datatype set, PROBE-076, and `WithTemplate` name + RM-mandatory completion so a decoded composition validates against the OPT. Residual edge deferrals (exotic `ctx/` fields on encode, `.schema` media types) are tracked by the umbrella above + the package [`deviations.md`](../../openehr/serialize/simplified/deviations.md); the upstream byte-conformance probe landed as PROBE-086, and its coverage ratchet is archived at [2026-08-03-flat-coverage-ratchet.md](archive/2026-08-03-flat-coverage-ratchet.md).

The Phase 2 clinical-building-blocks umbrella **landed and was archived** ([archive/2026-05-21-phase-2-clinical-building-blocks.md](archive/2026-05-21-phase-2-clinical-building-blocks.md)); its remaining deferred scope (simplified formats) is now sequenced by the umbrella above. The two AQL plans also landed and moved to `archive/` — AQL builders ([REQ-055](archive/2026-05-21-aql-builders.md)) and AQL parse + lint ([REQ-109](archive/2026-06-15-aql-lint.md)).

**Landed (archived):** OPT parser, REQ-100 follow-ups (Phases 1–8), composition validation (REQ-102), composition builder (REQ-101), template-driven instance generator (REQ-107), REQ-104 slot assertions + REQ-105 terminology bindings (PR #43), C_PRIMITIVE_OBJECT wire parser + REQ-107 UID emission, BMM codegen, canonical JSON/XML, AQL builders (REQ-055), AQL static lint (REQ-109), validation beyond COMPOSITION (REQ-110 — demographic PARTY hierarchy + FOLDER / EHR_STATUS), the public compiled-template bridge (REQ-111, ADR 0010), the SMART-on-openEHR auth conformance audit (REQ-061..064/068, REQ-070..072; ADR 0008/0009; STRAND-05), polymorphic `_type` round-trip stability (REQ-052/040/102/107), seeded synthetic value generation (value-fill + seed; REQ-103/107/101), template-less RM validation floor (REQ-112), stored-query / query REST conformance (REQ-055/057), the execution-oriented parsed AQL AST (REQ-113), the modernize & simplify sweep (client write-plumbing dedup REQ-094, generated LOCATABLE identity surface — ADR 0013, REQ-031/040/043), the path-parameter single-encoding fix + class-predicate `ParsedPath` (REQ-095/113, PR #74), the WebTemplate JSON export (REQ-106, ADR-0014; EHRbase v2.3 structural parity via PROBE-075), template-level node naming with name-predicated paths (REQ-116; archetype-reuse templates now export, PROBE-075 parity extended to both vendored oracles, PROBE-086 unblocked), the upstream FLAT parity harness (REQ-080/PROBE-086 — the pinned EHRbase corpus round-trips through the REQ-053 codec, exact on the modelled subset and counting the rest; it caught a silent encode-side data loss, fixed in `rmpath` under REQ-121), and the PROBE-086 coverage ratchet (REQ-053/121 — CODE_PHRASE leaves, the optional datatype suffixes, and the metadata-spelling decision [ADR 0015](../adr/0015-flat-metadata-spelling.md); 10.5% → 19.5% of the upstream corpus compared exactly) — see [archive/](archive/README.md). The umbrella validation scope first sketched in the archived [umbrella validation plan](archive/2026-05-21-validation.md) is now complete.

## Header convention (load-bearing)

Every plan MUST start with the fields in [`_template.md`](_template.md):

- **`Covers:`** — REQ-NNN / PROBE-NNN / STRAND-NN identifiers
- **`Status:`** / **`Implementation:`** — mirror [`../specifications/README.md`](../specifications/README.md#status-header)
- **`Depends on:`** / **`Defers:`** — explicit scope boundaries

Naming: `YYYY-MM-DD-<short-title>.md`. New plans: copy [`_template.md`](_template.md). When a plan lands, move it to [`archive/`](archive/README.md) and update [`../specifications/traceability.yaml`](../specifications/traceability.yaml).
