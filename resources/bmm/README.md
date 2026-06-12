# `resources/bmm/` — pinned openEHR BMM schemas

Machine-readable openEHR meta-model schemas (BMM) in their canonical `P_BMM` JSON form. The SDK's domain types in `openehr/rm/`, `openehr/aom/aom14/`, and related packages are **derived** from these files (see [`../../docs/specifications/bmm-conformance.md`](../../docs/specifications/bmm-conformance.md)).

These files are **the SDK's source of truth** for the openEHR Reference Model, Archetype Object Model, base types, language types, and terminology service interface. The SDK pins them as in-tree assets so that:

1. Builds are reproducible — no network fetch at build time.
2. Generated code can be re-emitted byte-for-byte (drift detection in CI).
3. Version bumps to any BMM are explicit, reviewable commits.

## Files

| File | Schema ID | bmm_version | Classes | Includes | v1 status | Role |
|---|---|---|---|---|---|---|
| `openehr_base_1.3.0.bmm.json` | `openehr_base_1.3.0` | 2.4 | 43 + 29 primitive types | — | **primary** | Foundation: primitives, base types, change-control |
| `openehr_rm_1.2.0.bmm.json` | `openehr_rm_1.2.0` | 2.4 | 146 | base 1.3.0 | **primary** (excluding `ehr_extract` package) | Reference Model: clinical + demographic |
| `openehr_am_1.4.0.bmm.json` | `openehr_am_1.4.0` | 2.4 | 39 | base 1.3.0 | **primary** | Archetype Object Model 1.4 (ADL 1.4) |
| `openehr_am_2.4.0.bmm.json` | `openehr_am_2.4.0` | 2.4 | 75 | base 1.3.0 + lang 1.1.0 | **deferred** | Archetype Object Model 2 (ADL 2 / AOM 2) |
| `openehr_lang_1.1.0.bmm.json` | `openehr_lang_1.1.0` | 2.4 | 86 | base 1.3.0 | **deferred** (reference only) | LANG types — BMM meta-model + `P_BMM` persistence classes. Used as documentation while `openehr/bmm/` is hand-written. |
| `openehr_term_3.1.0.bmm.json` | `openehr_term_3.1.0` | — | (terminology service interface) | — | **deferred** | Terminology service interface |

**Primary** files drive v1 code generation. **Deferred** files stay here (no removal) — they are kept for future generation phases or as cross-references. See [`../../docs/specifications/scope.md`](../../docs/specifications/scope.md#out-of-scope-v1) and [`../../docs/specifications/bmm-conformance.md § v1 scope summary`](../../docs/specifications/bmm-conformance.md#v1-scope-summary).

Schema dependencies (transitive `includes`):

```
base 1.3.0  (foundation)
├── rm 1.2.0
├── am 1.4.0
└── lang 1.1.0
    └── am 2.4.0
```

## Provenance

These BMM files are published by the openEHR Foundation as the computable form of the corresponding openEHR specification documents. The on-disk format is **`P_BMM`** — the persistence binding of the abstract BMM meta-model, defined in the openEHR LANG specification (`bmm` and `bmm_persistence`).

The files in this directory are **byte-identical copies** of the upstream releases. They are stored here (rather than fetched at build time) for the reasons listed above. When upstream publishes a newer version, see § Updating below.

All six pins were verified byte-identical to [`openEHR/BMM-publisher`](https://github.com/openEHR/BMM-publisher) `resources/` on **2026-06-12**. At that time `openehr_lang_1.1.0` was re-synced to the canonical modular publisher form (it had previously been a flattened variant); the other five matched. To re-audit, fetch each `resources/<file>` from the publisher and `sha256sum` against the pin.

## Updating

A BMM version bump is **never accidental**. The canonical procedure is **[ADR 0001 — BMM version-bump runbook](../../docs/adr/0001-bmm-version-bump-runbook.md)**. The short form:

1. Drop the new file alongside the old (e.g. `openehr_rm_1.2.1.bmm.json` next to `openehr_rm_1.2.0.bmm.json`). Do **not** overwrite the old file.
2. Run `make codegen` then `make codegen-verify`. Optionally inspect the semantic diff with `go run ./cmd/bmmdiff <old> <new>`.
3. Update version pins in [`../../docs/specifications/bmm-conformance.md`](../../docs/specifications/bmm-conformance.md) and the schema ID table above.
4. Add a short CHANGELOG bullet under `## [Unreleased]` (Added / Changed / Removed per [`../../docs/specifications/module-layout.md § Versioning`](../../docs/specifications/module-layout.md#versioning)).
5. Remove the old file **in the same commit** once the regen and tests pass.

See ADR 0001 for the full procedure, roles, and tooling notes. The weekly drift bot ([`.github/workflows/codegen-drift.yml`](../../.github/workflows/codegen-drift.yml)) catches accidental hand-edits or generator-template changes between bumps.

Mid-version evolution (`openehr_rm_1.2.0` → `openehr_rm_1.2.0.2`, i.e. a `schema_revision` change without a `rm_release` change) is recorded by replacing the file and noting it in the CHANGELOG; no code regeneration is required if no semantic content changed.

## Integrity

Each BMM file SHOULD be accompanied by a checksum in this README's git history (CI computes and asserts at PR time once the codegen pipeline lands — see [`../../docs/plans/`](../../docs/plans/)). The checksums are not stored separately to avoid two sources of truth.

## Why these files and not the upstream URL

- **Network determinism.** Generated code in `openehr/rm/` must be a deterministic function of the inputs; fetching at build time introduces flake and TOFU risk.
- **Auditability.** A change to any BMM is a real commit, reviewable in the PR diff.
- **Air-gapped builds.** Consumers building the SDK behind a corporate proxy or in an air-gapped CI need the inputs in-tree.

## References

- BMM (abstract meta-model): openEHR LANG specification — *Basic Meta-Model*.
- P_BMM persistence: openEHR LANG specification — *BMM Persistence Format*.
- SDK conformance contract: [`../../docs/specifications/bmm-conformance.md`](../../docs/specifications/bmm-conformance.md).
- Generator design: [`../../docs/plans/`](../../docs/plans/) — `bmm-codegen` plan.
