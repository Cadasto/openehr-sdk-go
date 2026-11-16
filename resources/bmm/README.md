# `resources/bmm/` — pinned openEHR BMM schemas

Machine-readable openEHR meta-model schemas (BMM) in their canonical `P_BMM` JSON form. The SDK's domain types in `openehr/rm/`, `openehr/aom/aom14/`, and related packages are derived from these files (see [`../../docs/specifications/reference-model.md`](../../docs/specifications/reference-model.md)).

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

**Primary** files drive v1 code generation, each target through its root schema under [`sdk/`](#sdk-authored-root-schemas-sdk). **Deferred** files stay here and are not removed. They are kept for future generation phases or as cross-references. See [`../../docs/specifications/scope.md`](../../docs/specifications/scope.md#out-of-scope-v1) and [`../../docs/specifications/reference-model.md § The BMM substrate`](../../docs/specifications/reference-model.md#the-bmm-substrate).

Schema dependencies (transitive `includes`):

```
base 1.3.0  (foundation)
├── rm 1.2.0
├── am 1.4.0
└── lang 1.1.0
    └── am 2.4.0
```

## SDK-authored root schemas (`sdk/`)

Each root here is the schema a generation target loads in place of a pinned file. A root includes the pinned schema and declares only what the pinned schemas lack or get wrong; a class it declares again replaces the included class of that name, whole. Each class and property a root declares ends its `documentation` with a note: its reason (`Reason: …`), then the upstream reference it rests on (`Upstream: …`). The reference is the specification pull request or ticket that carries the same change, as an `https://` URL; the id of the pinned schema it follows; or a vendored ITS-XML schema by its path in the tree, with or without a line, such as `resources/its-xml/components/AM/Release-1.4/Template.xsd:115`. The generator leaves the note out of the Go doc comment it emits. The contract is [`reference-model.md` § SDK-authored root schemas](../../docs/specifications/reference-model.md#sdk-authored-root-schemas); the decision is [ADR 0034](../../docs/adr/0034-sdk-authored-root-schemas.md).

### SDK extension keys

What P_BMM has no construct for, a root says in the top-level `sdk_extensions` object of its JSON, one key per kind. The public loader (`openehr/bmm`) leaves the object aside; the generator reads it from every schema of the target's include chain, and a member it does not read fails generation, so a misspelt key never goes unnoticed. Every entry carries a `reason` and an `upstream` member, the same two parts as a declaration's note, with the same reference forms.

| Key | Entry members | What the generator does |
|---|---|---|
| `classes_left_out` | `class` | Emits no Go type, registry entry or decode hook for the class; rminfo reports it as an excluded class. Generation fails when no schema declares the class, or a class the target emits still names it. |
| `type_aliases` | `alias`, `class` | Carries the alias, a type name a decoder reads as `class` (an `xsi:type` older tools write), in its plan for the canonical-XML codec; the type registry never holds it. No generated code reads it yet. Generation fails on an alias a schema declares. |
| `dropped_elements` | `class`, `element`, `texts` | The XML decoder of `class`, and of every class that inherits from it, reads the element and drops it when its text, less the white space around it, is one of `texts`, the one value it may hold in each way the schema writes it (`false` and `0`), reporting nothing; any other text fails the decode with `canxml.ErrInvalidShape`. The encoder never writes it. Generation fails when the element names a member of the class. |
| `anonymous_type_names` | `schema_type`, `element_path`, `class` | Gives `class` the anonymous type of the element at `element_path` (element names separated by `/`) inside the named schema type, as the schema type the XML codec takes the class's child order and attributes from. Generation fails when no ITS-XML schema of the target declares an anonymous type at that path. |
| `schema_forms` | `class`, `property`, `form`, `shapes` | Gives the property's element the form the schema gives it. The one form is `choice`: the element takes the shape of one of the classes `shapes` lists, and the property is typed `Any`. The XML decoder reads the element as the first shape whose schema type declares its first child element, or as the first shape when none does; the encoder writes the shape the property holds, with no `xsi:type`. Generation fails when the property is not typed `Any`, a shape has no XML decoder, or two shapes share an element. |

Every class an entry of the last four keys names must be one the target holds, so no entry names a class left out. An example entry:

```json
"sdk_extensions": {
    "dropped_elements": [
        {"class": "C_ATTRIBUTE", "element": "match_negated", "texts": ["false", "0"],
         "reason": "a legacy element no AOM class declares, accepted fixed to false.",
         "upstream": "resources/its-xml/components/AM/Release-1.4/Archetype.xsd:52"}
    ]
}
```

| File | Schema ID | Includes | Declares | Loaded by |
|---|---|---|---|---|
| `sdk/openehr_reference_model_1.2.0.bmm.json` | `openehr_reference_model_1.2.0` | rm 1.2.0 | `TRANSLATION_DETAILS`, with `accreditation` where the RM schema misspells it ([specifications-RM PR 43](https://github.com/openEHR/specifications-RM/pull/43)) | the RM target |
| `sdk/openehr_operational_template_1.4.0.bmm.json` | `openehr_operational_template_1.4.0` | am 1.4.0, the RM root | the classes of the AM Release-1.4 `Template.xsd` (`OPERATIONAL_TEMPLATE` and the eight beside it, and the two anonymous types of `T_VIEW`); the ontology in the form of `Archetype.xsd`; `C_CODE_PHRASE`, `C_DV_ORDINAL`, `C_DV_QUANTITY`, `C_QUANTITY_ITEM` and `ORDINAL` in the form of `OpenehrProfile.xsd`, leaving out the BMM's `C_CODED_TEXT`, `C_ORDINAL` and `C_QUANTITY` | the AOM 1.4 target |

The roots are hand-written sources under review, not pins: the sync script never fetches or removes them, `MANIFEST.txt` does not list them, and `make bmm-check` does not hash them. A root's id is its own (`<rm_publisher>_<schema_name>_<rm_release>`, with the words spelled out); the generator refuses an id that both `sdk/` and this directory hold. The root check, a Go test in `internal/bmmgen` that `make test` runs, holds every root to its rules: each type name it uses resolves after the merge, each declaration and each extension entry carries its reason and upstream reference, each class it declares again still differs from the class it shadows, and no root takes a pinned id.

## Provenance

These BMM files are published by the openEHR Foundation as the computable form of the corresponding openEHR specification documents. The on-disk format is `P_BMM`, the persistence binding of the abstract BMM meta-model defined in the openEHR LANG specification (`bmm` and `bmm_persistence`).

The files in this directory are byte-identical copies of [`openEHR/specifications-ITS-BMM`](https://github.com/openEHR/specifications-ITS-BMM) `components/<COMPONENT>/json/` at the commit [`MANIFEST.txt`](MANIFEST.txt) records, fetched by [`scripts/sync-bmm.sh`](../../scripts/sync-bmm.sh) (`make bmm-sync`). They are stored here, not fetched at build time, for the reasons listed above. The pin is currently the ITS-BMM branch `integration/am14-opt-validation`, which adds [specifications-AM PR 48](https://github.com/openEHR/specifications-AM/pull/48) to `master`: `units_system` and `units_display_name` on the AOM 1.4 `C_QUANTITY_ITEM` (SPECAM-109, `schema_revision` 1.4.0.3). Re-pin to `master` once ITS-BMM carries the merged change. When upstream publishes a newer version, follow § Updating below.

`make bmm-drift` compares each pinned file with ITS-BMM `master` and lists any newer version of a pinned component. It needs the network and is not a gate.

Until 2026-10 the pins were audited by hand against [`openEHR/BMM-publisher`](https://github.com/openEHR/BMM-publisher) `resources/` (last on 2026-06-12, when `openehr_lang_1.1.0` was re-synced to the modular form). On 2026-10-26 all six were byte-identical at ITS-BMM `1c678406`, which became the first manifest pin; the publisher's copies of base, rm and term had changed since.

## Updating

A BMM version bump is deliberate, and [ADR 0001](../../docs/adr/0001-bmm-version-bump-runbook.md) records why it takes this shape: one pull request per bump, the new file added and the old one removed in the same commit, and one version per schema id.
The impact on the SDK's version is decided under [`module-layout.md` § Versioning](../../docs/specifications/module-layout.md#versioning), not here.

1. **Stage the new file beside the old.**
   Add its upstream path to `SRC_PATHS` in [`scripts/sync-bmm.sh`](../../scripts/sync-bmm.sh), beside the old one, and run `BMM_REF=master make bmm-sync`: it fetches, say, `openehr_rm_1.2.1.bmm.json` next to `openehr_rm_1.2.0.bmm.json` and records both in `MANIFEST.txt`.
   Do **not** overwrite the old file: the diff shows the rename as a paired add and remove.

   **Then move the roots.**
   Each root under [`sdk/`](#sdk-authored-root-schemas-sdk) includes a pinned schema by id, and the sync never touches it.
   A root that includes the bumped schema names its new id in `includes`; the RM root also takes the new `rm_release`, which changes its id and file name (`openehr_reference_model_1.2.1`) and `TargetRM.RootID` in `internal/bmmgen/target.go` with them.
   Then run the root check, `go test ./internal/bmmgen -run RootSchemas -count=1`.
   A class a root declares again that the new schema now carries in the same form fails it, as a correction the upstream fix has closed: remove that declaration from the root in this pull request.
2. **Regenerate.**
   Run `make codegen`. It rewrites every `_gen.go` file under `openehr/rm/`, `openehr/aom/aom14/` and the other targets, and never touches a `*_ext.go` companion.
3. **Check that the regeneration is reproducible.**
   Run `make codegen-check`; on a freshly regenerated tree it exits 0.
   If it does not, a non-deterministic generator is a bug, not a consequence of the bump.
4. **Inspect the semantic diff** (optional, recommended).
   `go run ./cmd/bmmdiff resources/bmm/openehr_rm_1.2.0.bmm.json resources/bmm/openehr_rm_1.2.1.bmm.json` summarises added and removed classes, per-class property changes, and cardinality and function changes; `-suggest-changelog` adds a one-line CHANGELOG suggestion.
5. **Audit the hand-written companions.**
   A removed class or renamed property breaks every `*_ext.go` companion that names it.
   Run `go build ./...`, and fix or remove the affected companions in this pull request.
6. **Update the version pins** in [`reference-model.md` § Schema → Go package set](../../docs/specifications/reference-model.md#schema--go-package-set): the schema id and `bmm_version`.
7. **Update this file**: the [Files](#files) table (schema id, `bmm_version`, class count) and any prose here that names a version.
8. **Propose the CHANGELOG bullet in the pull request body.**
   `CHANGELOG.md` follows the rule in [`AGENTS.md` § Code style and conventions](../../AGENTS.md#code-style-and-conventions) and is not edited in the bump, so the body carries one short, high-level line, one artefact class, for the maintainer to fold in.
   The `bmmdiff -suggest-changelog` output is a starting point a person reviews.
9. **Remove the old file in the same commit.**
   Drop its path from `SRC_PATHS` and re-run `make bmm-sync`, which deletes the file and rewrites the manifest.
   Never leave both versions here.
10. **Audit the narrow-interface accessors** (REQ-052, PROBE-038).
    If `bmmdiff -suggest-changelog` reports an added class whose `ancestors` include `DV_TEXT`, `DV_URI`, `AUDIT_DETAILS`, `PARTY_IDENTIFIED` or `OBJECT_REF`, the generator's field-lift picks the new subtype up, but two hand-written files need it too:
    - [`openehr/rm/like_interfaces.go`](../../openehr/rm/like_interfaces.go): the marker line `func (NewSubtype) is<Parent>Like() {}` and a method body for every accessor on the parent interface (`GetValue`, `GetDefiningCode`, …); without them the new subtype does not satisfy `<Parent>Like`, and the codec does not compile.
    - [`openehr/rm/like_accessors.go`](../../openehr/rm/like_accessors.go): a `case *NewSubtype:` arm in the matching `*Base` closed type switch, so the helpers recover the parent struct from the new subtype.

    Then add a round-trip case for the new subtype to [`openehr/serialize/canjson/polymorphic_decode_test.go`](../../openehr/serialize/canjson/polymorphic_decode_test.go), so PROBE-038 covers it.
11. **Open the pull request.**
    Its CI runs `make test`, which runs `make codegen-check`.

Mid-version evolution (`openehr_rm_1.2.0` → `openehr_rm_1.2.0.2`, i.e. a `schema_revision` change without a `rm_release` change) is recorded by moving its path in `SRC_PATHS`, re-syncing, and noting it in the CHANGELOG. If no semantic content changed, no code regeneration is required. A re-pin of this kind can still land an upstream fix a root carries: run the root check, and remove each shadow it reports in the same commit.

### Roles

- The **author** runs the steps and opens the pull request.
- The **reviewer** checks that the BMM diff (step 4's output, ideally pasted into the pull request body) matches the scope of the Go diff, and that the proposed CHANGELOG bullet describes the change.
- The **drift bot**, the weekly [`.github/workflows/codegen-drift.yml`](../../.github/workflows/codegen-drift.yml), re-runs `make codegen-check` on a clean checkout after the merge: a green run shows the bump landed cleanly, and between bumps it catches hand-edits to `_gen.go` files and generator-template changes.

### Tooling

- `make test` runs `make codegen-check`, so a passing `make test` shows the regeneration is reproducible.
- `cmd/bmmdiff` is an inspection tool: it always exits 0 and writes to standard output, for pull request bodies and CHANGELOG drafts.
- The drift bot keeps one open issue labelled `bmm-drift` and comments on it on a later failure rather than opening another.
- [`internal/bmmgen/sim_bump_test.go`](../../internal/bmmgen/sim_bump_test.go) simulates a bump on every CI run, so the regeneration path is exercised between bumps.

## Integrity

[`MANIFEST.txt`](MANIFEST.txt) records the upstream commit and each file's `sha256`; the sync script writes it, so never edit it by hand. `make bmm-check`, part of `make ci`, recomputes the hashes offline and fails on a file whose bytes differ, a pinned file with no hash line, or a `*.bmm.json` in this directory the manifest does not list. `make bmm-sync` restores the pinned bytes. The roots under `sdk/` are not pins and have no hash line; the root check holds them instead.

## Why these files and not the upstream URL

- **Network determinism.** Generated code in `openehr/rm/` must be a deterministic function of the inputs. Fetching at build time adds flaky builds and trust-on-first-use (TOFU) risk.
- **Auditability.** A change to any BMM is a real commit, reviewable in the PR diff.
- **Air-gapped builds.** Consumers building the SDK behind a corporate proxy or in an air-gapped CI need the inputs in-tree.

## References

- BMM (abstract meta-model): openEHR LANG specification, *Basic Meta-Model*.
- P_BMM persistence: openEHR LANG specification, *BMM Persistence Format*.
- SDK conformance contract: [`../../docs/specifications/reference-model.md`](../../docs/specifications/reference-model.md).
- Generator design: [ADR 0002](../../docs/adr/0002-bmm-codegen-decisions.md); the generator landed on `main` in commit [b89aa589](https://github.com/Cadasto/openehr-sdk-go/commit/b89aa589).
