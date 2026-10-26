# `resources/bmm/` — pinned openEHR BMM schemas

Machine-readable openEHR meta-model schemas (BMM) in their canonical `P_BMM` JSON form. The SDK's domain types in `openehr/rm/`, `openehr/aom/aom14/`, and related packages are derived from these files (see [`../../docs/specifications/bmm-conformance.md`](../../docs/specifications/bmm-conformance.md)).

These files are the SDK's source of truth for the openEHR Reference Model, Archetype Object Model, base types, language types, and terminology service interface. The SDK pins them as in-tree assets so that:

1. Builds are reproducible: nothing is fetched over the network at build time.
2. Generated code can be re-emitted byte-for-byte, which CI uses to detect drift.
3. Version bumps to any BMM are explicit, reviewable commits.

## Files

| File | Schema ID | bmm_version | Classes | Includes | v1 status | Role |
|---|---|---|---|---|---|---|
| `openehr_base_1.3.0.bmm.json` | `openehr_base_1.3.0` | 2.4 | 43 + 29 primitive types | — | **primary** | Foundation: primitives, base types, change-control |
| `openehr_rm_1.2.0.bmm.json` | `openehr_rm_1.2.0` | 2.4 | 146 | base 1.3.0 | **primary** (excluding `ehr_extract` package) | Reference Model: clinical + demographic |
| `openehr_am_1.4.0.bmm.json` | `openehr_am_1.4.0` | 2.4 | 39 | base 1.3.0 | **primary** | Archetype Object Model 1.4 (ADL 1.4) |
| `openehr_am_2.4.0.bmm.json` | `openehr_am_2.4.0` | 2.4 | 75 | base 1.3.0 + lang 1.1.0 | **deferred** | Archetype Object Model 2 (ADL 2 / AOM 2) |
| `openehr_lang_1.1.0.bmm.json` | `openehr_lang_1.1.0` | 2.4 | 86 | base 1.3.0 | **deferred** (reference only) | LANG types: BMM meta-model + `P_BMM` persistence classes. Used as documentation while `openehr/bmm/` is hand-written. |
| `openehr_term_3.1.0.bmm.json` | `openehr_term_3.1.0` | — | (terminology service interface) | — | **deferred** | Terminology service interface |

**Primary** files drive v1 code generation. **Deferred** files stay here and are not removed. They are kept for future generation phases or as cross-references. See [`../../docs/specifications/scope.md`](../../docs/specifications/scope.md#out-of-scope-v1) and [`../../docs/specifications/bmm-conformance.md § v1 scope summary`](../../docs/specifications/bmm-conformance.md#v1-scope-summary).

Schema dependencies (transitive `includes`):

```
base 1.3.0  (foundation)
├── rm 1.2.0
├── am 1.4.0
└── lang 1.1.0
    └── am 2.4.0
```

## Provenance

These BMM files are published by the openEHR Foundation as the computable form of the corresponding openEHR specification documents. The on-disk format is `P_BMM`, the persistence binding of the abstract BMM meta-model defined in the openEHR LANG specification (`bmm` and `bmm_persistence`).

The files in this directory are byte-identical copies of [`openEHR/specifications-ITS-BMM`](https://github.com/openEHR/specifications-ITS-BMM) `components/<COMPONENT>/json/` at the commit [`MANIFEST.txt`](MANIFEST.txt) records, fetched by [`scripts/sync-bmm.sh`](../../scripts/sync-bmm.sh) (`make bmm-sync`). They are stored here, not fetched at build time, for the reasons listed above. When upstream publishes a newer version, follow § Updating below.

`make bmm-drift` compares each pinned file with ITS-BMM `master` and lists any newer version of a pinned component. It needs the network and is not a gate.

Until 2026-10 the pins were audited by hand against [`openEHR/BMM-publisher`](https://github.com/openEHR/BMM-publisher) `resources/` (last on 2026-06-12, when `openehr_lang_1.1.0` was re-synced to the modular form). On 2026-10-26 all six were byte-identical at ITS-BMM `1c678406`, which became the first manifest pin; the publisher's copies of base, rm and term had changed since.

## Updating

A BMM version bump is always deliberate and follows [ADR 0001: BMM version-bump runbook](../../docs/adr/0001-bmm-version-bump-runbook.md), the canonical procedure. In outline:

1. Add the new file's upstream path to `SRC_PATHS` in [`scripts/sync-bmm.sh`](../../scripts/sync-bmm.sh), beside the old one, and run `BMM_REF=master make bmm-sync` (e.g. `openehr_rm_1.2.1.bmm.json` lands next to `openehr_rm_1.2.0.bmm.json`). Do **not** overwrite the old file.
2. Run `make codegen` then `make codegen-check`. Optionally inspect the semantic diff with `go run ./cmd/bmmdiff <old> <new>`.
3. Update version pins in [`../../docs/specifications/bmm-conformance.md`](../../docs/specifications/bmm-conformance.md) and the schema ID table above.
4. Propose the one-line CHANGELOG bullet in the PR body; the CHANGELOG follows the rule in [`AGENTS.md § Code style and conventions`](../../AGENTS.md#code-style-and-conventions) (ADR 0001 step 8).
5. Remove the old path from `SRC_PATHS` and re-run `make bmm-sync`, which deletes the old file, **in the same commit** once the regen and tests pass.

See ADR 0001 for the full procedure, roles, and tooling notes. The weekly drift bot ([`.github/workflows/codegen-drift.yml`](../../.github/workflows/codegen-drift.yml)) catches accidental hand-edits or generator-template changes between bumps.

Mid-version evolution (`openehr_rm_1.2.0` → `openehr_rm_1.2.0.2`, i.e. a `schema_revision` change without a `rm_release` change) is recorded by moving its path in `SRC_PATHS`, re-syncing, and noting it in the CHANGELOG. If no semantic content changed, no code regeneration is required.

## Integrity

[`MANIFEST.txt`](MANIFEST.txt) records the upstream commit and each file's `sha256`; the sync script writes it, so never edit it by hand. `make bmm-check`, part of `make ci`, recomputes the hashes offline and fails on a file whose bytes differ, a pinned file with no hash line, or a `*.bmm.json` the manifest does not list. `make bmm-sync` restores the pinned bytes.

## Why these files and not the upstream URL

- **Network determinism.** Generated code in `openehr/rm/` must be a deterministic function of the inputs. Fetching at build time adds flaky builds and trust-on-first-use (TOFU) risk.
- **Auditability.** A change to any BMM is a real commit, reviewable in the PR diff.
- **Air-gapped builds.** Consumers building the SDK behind a corporate proxy or in an air-gapped CI need the inputs in-tree.

## References

- BMM (abstract meta-model): openEHR LANG specification, *Basic Meta-Model*.
- P_BMM persistence: openEHR LANG specification, *BMM Persistence Format*.
- SDK conformance contract: [`../../docs/specifications/bmm-conformance.md`](../../docs/specifications/bmm-conformance.md).
- Generator design: [ADR 0002](../../docs/adr/0002-bmm-codegen-decisions.md); the generator landed on `main` in commit [b89aa589](https://github.com/Cadasto/openehr-sdk-go/commit/b89aa589).
