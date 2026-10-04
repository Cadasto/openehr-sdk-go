# testkit/corpus

Vendored fixture documents for codec, validation, and probe tests: OPTs, compositions, RM samples, wire bodies, and reference goldens. They are checked in so CI does not need a sibling clone. Licences and provenance: [`THIRD_PARTY_LICENSES.md`](THIRD_PARTY_LICENSES.md); repository-wide inventory: [`LICENSING.md`](../../LICENSING.md).

**These are not REQ-082 Cassette-mode recordings.** Everything here is a request or response *body*: it carries no method, URL, header, or status code, so none of it can be replayed as an HTTP exchange. The Cassette mode that [REQ-082](../../docs/specifications/conformance.md#req-082--runnability) mandates records whole exchanges and lands under `testkit/recordings/`. Code reaches the fixtures through [`testkit/fixtures`](../fixtures/paths.go).

## Layout

```
corpus/
  templates/{template-id}.opt
  compositions/{template-id}.json
  compositions/{template-id}.xml      # when vendored
  rm/{name}.json | {name}.xml         # RM probe samples (ehrbase, leaf XML, …)
  submissions/{name}.json             # CONTRIBUTION POST wire (inline ORIGINAL_VERSION)
  its_rest/                           # ITS-REST / discovery wire
  webtemplate/{template-id}.opt       # OPT + EHRbase reference WebTemplate
    | {template-id}.webtemplate.json  #   golden (PROBE-075 / REQ-116 oracles)
  flat-conformance/                   # pinned upstream FLAT corpus (PROBE-086)
    MANIFEST.txt                      #   commit pin + per-file sha256
    templates/{name}.opt
    compositions/{name}.json          #   upstream-authored FLAT bodies
  crossformat/                        # pinned upstream cross-format sets (PROBE-105)
    MANIFEST.txt                      #   commit pins, per-file sha256, OPT pointers
    {set}/template.opt                #   absent when the OPT is vendored elsewhere
    {set}/canonical.json | canonical.xml | flat.json | structured.json
  aql/lint/{name}.aql                 # hand-written AQL lint inputs
  aql/conformance/                    # pinned upstream AQL FROM corpus (PROBE-100)
    AQL_SOURCE.txt                    #   commit pin (authoritative — byte copies)
    EXCLUDED.txt                      #   generated: what upstream is not carried
    {FAMILY}/{name}.csv               #   per consuming Robot suite family
```

Everything under `flat-conformance/` is a pinned subtree, machine-synced from
upstream at a recorded commit, so do not hand-edit it. Refresh with
`make flat-conformance-sync` and verify integrity with `make flat-conformance-verify`
(an offline `sha256` check that `make ci` runs as a gate). `make flat-conformance-check`
adds an upstream-drift report; it needs network and is a dev helper, not a gate. Resolve paths via
[`fixtures.FlatConformanceOpt`](../fixtures/paths.go) /
`fixtures.FlatConformanceFlat` / `fixtures.ListFlatConformance`. The rest of
this directory is curated by hand and is not covered by that manifest. The
EHRbase Robot integration-test subset records the upstream commit it was
ingested from in [`ROBOT_SOURCE.txt`](ROBOT_SOURCE.txt). That file is a
provenance pin, not a per-file `sha256` lock.

`aql/conformance/` is a second pinned subtree; do not hand-edit it either. Its
CSVs are byte copies of the upstream files, so its pin
([`AQL_SOURCE.txt`](aql/conformance/AQL_SOURCE.txt)) fully determines their
content. Refresh with [`scripts/ingest-robot-aql.sh`](../../scripts/ingest-robot-aql.sh),
which also regenerates `EXCLUDED.txt`.

`crossformat/` is a third pinned subtree; do not hand-edit it either. See
[Cross-format sets](#cross-format-sets) below.

Resolve paths via [`testkit/fixtures`](../fixtures/) (`TemplateOpt`, `CompositionJSON`, `CompositionXML`, `RMJSON`, `RMXML`, `SubmissionJSON`, `WebTemplateOpt`, `WebTemplateReference`, `ListCrossFormatSets`).

Composition JSON uses template ids **without** `::{uuid}` suffixes.

**Probe vs on-disk.** Every vendored `*.json` under `compositions/` is in [`ListCompositionJSON`](../fixtures/discover.go) and PROBE-030. A file whose content carries a genuine RM-floor finding is held out of the `validation.ValidateRM` leg only (see Conventions). Vendored `*.xml` may be omitted from [`ListRMXML`](../fixtures/discover.go) via `compositionXMLExcluded` when canxml cannot round-trip it yet; the file stays for template and instance work.

## Cross-format sets

Each directory under [`crossformat/`](crossformat/) is one upstream composition given in two or more of canonical JSON, canonical XML, FLAT and STRUCTURED, with the OPT of its template. They are the input to PROBE-105. Both upstreams are Apache-2.0 (see [`THIRD_PARTY_LICENSES.md`](THIRD_PARTY_LICENSES.md)).

| Set | Template id | Formats | Source | Pin |
|---|---|---|---|---|
| `alternative_events` | `AlternativeEvents` | JSON, FLAT | openEHR_SDK | `8a5dae6f` |
| `consult_record` | `EHRN-ABDM-OPConsultRecord.v2.0` | JSON, XML, FLAT, STRUCTURED | integration-tests | `fcb3ac4b` |
| `corona` | `Corona_Anamnese` | JSON, FLAT, STRUCTURED | openEHR_SDK | `8a5dae6f` |
| `ehrn_abdm` | `EHRN-ABDM-OPConsultRecord.v2.0` | JSON, FLAT | openEHR_SDK | `8a5dae6f` |
| `family_history` | `family_history` | JSON, XML | integration-tests | `fcb3ac4b` |
| `multi_list` | `Multi_list` | FLAT, STRUCTURED | openEHR_SDK | `8a5dae6f` |
| `multi_occurrence` | `ehrbase_multi_occurrence.de.v1` | JSON, FLAT | openEHR_SDK | `8a5dae6f` |
| `nested` | `nested.en.v1` | XML, FLAT | integration-tests | `fcb3ac4b` |
| `persistent_minimal` | `persistent_minimal.en.v1` | XML, FLAT | integration-tests | `fcb3ac4b` |
| `test_all_types` | `test_all_types.en.v1` | JSON, FLAT | openEHR_SDK | `8a5dae6f` |

JSON and XML here are the canonical formats. Inside a set the files have fixed names: `template.opt`, `canonical.json`, `canonical.xml`, `flat.json` and `structured.json`. Three sets have no `template.opt`, because their OPT is already vendored in this tree byte for byte. `corona` uses `webtemplate/Corona_Anamnese.opt`, `nested` uses `templates/nested.en.v1.opt` and `persistent_minimal` uses `templates/persistent_minimal.en.v1.opt`; the manifest records each pointer and its `sha256`. `consult_record` and `ehrn_abdm` share a template id but not an OPT: the two upstreams carry different copies of it, and each set keeps its own.

Only files that describe the same instance form a set. The Robot canonical JSON for `nested` and `persistent_minimal`, and the Robot FLAT and STRUCTURED for `family_history`, are other instances of those templates, so they are not vendored.

[`crossformat/MANIFEST.txt`](crossformat/MANIFEST.txt) records both pins, and for every file its source, upstream path and `sha256`. To refresh, check out the pinned commits in local clones of both upstreams, point `CROSSFORMAT_SDK_CLONE` and `CROSSFORMAT_ROBOT_CLONE` at them, and run `bash scripts/ingest-crossformat.sh ingest`. It refuses a clone that is not at its pin. `bash scripts/ingest-crossformat.sh verify` checks the vendored bytes offline. The tests in [`crossformat_test.go`](../fixtures/crossformat_test.go) check the same integrity under `make ci`. Resolve the sets with [`fixtures.ListCrossFormatSets`](../fixtures/crossformat.go).

## Index by vendor

### Benchmark (internal)

| Template id | OPT | JSON | XML |
|---|---|:---:|:---:|
| `vital_signs` | yes | yes | — |
| `clinical_notes.v0` | yes | yes | — |

`clinical_notes.v0` was made by Medblocks together with CODE24; its OPT names its Medblocks author, and it is credited under CODE24 here.

### CODE24 (Cadasto)

**License:** MIT — [`THIRD_PARTY_LICENSES.md`](THIRD_PARTY_LICENSES.md).

| Template id | OPT | JSON | XML | Probes |
|---|---|:---:|:---:|---|
| `body_weight` | yes | yes | yes | round-trip |
| `BMI` | EHRbase (see below) | yes | yes | round-trip |
| `alternative_types.en.v1` | EHRbase (see below) | yes | yes | round-trip |
| `test_template_rename_node` | yes | yes | yes | round-trip |
| `test_template_rename_node_2` | yes | yes | yes | round-trip |
| `Episode.v2` | yes | yes | yes | round-trip |
| `Address.v2` | yes | yes | yes | round-trip |
| `Demonstration.v1` | yes | yes | yes | JSON round-trip, RM floor held out (inverted `DV_INTERVAL` bounds); XML not exercised |
| `TestPerson.v2` | yes | yes | yes | JSON round-trip, RM floor held out (null `CODE_PHRASE.code_string`); XML not exercised |
| `SocialeAnamnese.v1` | yes (`social.opt`) | — | — | no patient data |
| `Referral Request.v1` | yes | — | — | OPT only: instance generation (PROBE-027) and the polymorphic round trip in `openehr/instance` |

Two OPTs in this table are not CODE24's. `alternative_types.en.v1` is byte-identical to the EHRbase Robot `valid_templates/alternative_types/alternative_types.opt`, and `BMI` is the EHRbase server's `service/src/test/resources/knowledge/operational_templates/BMI.opt` with its byte-order mark and CRLF line endings removed. Both are Apache 2.0 and in EHRbase since 2019, before the CODE24 copies. Their compositions are CODE24's.

`body_weight.opt` has its authoring tool's account id replaced by `user=redacted` in the `Generated By` entry; nothing else in it is edited.

`social.opt` is an SDK-normalised Code24 export, not a byte-identical upstream pin. The normalisation rewrites the root element to `<template>` and leaves `xsi:type="OPERATIONAL_TEMPLATE"`, and rewrites the document language element, `T_ARCHETYPE_ROOT`, the wrapper template-id suffix, and the top-level archetype id.

### ehrbase (openEHR_SDK)

**License:** Apache 2.0 — [`THIRD_PARTY_LICENSES.md`](THIRD_PARTY_LICENSES.md) (RM / template-triplet pin `8a5dae6fd82a5f0aa23b25681a13df79bfc21d2b`, 2026-09-28; WebTemplate + FLAT pin `e57511c6aca27ed501d31d663762c37c3491e74e`).

**RM-only** (`rm/`, no OPT):

| File | RM root | Probes |
|---|---|---|
| `minimal_evaluation.json` | COMPOSITION | JSON round-trip, RM floor held out (`EVALUATION` without `archetype_details`) |
| `compo_with_nested_party_related.json` | COMPOSITION | JSON round-trip, RM floor held out (nested `EVALUATION` without `archetype_details`) |
| `ehr_status_other_details_simple.json` | EHR_STATUS | JSON round-trip, RM floor held out (`EHR_STATUS` without `archetype_details`) |
| `nested_folder.json` | FOLDER | JSON round-trip |
| `test_all_types.v1.xml` | COMPOSITION | XML round-trip |
| `simple_empty_folder.xml` | FOLDER | XML round-trip |

**Template triplets** (`templates/` + `compositions/`; `nested.en.v1` from openEHR_SDK test-data, the other four from its validation test resources, three of them under different upstream file names, listed in [`THIRD_PARTY_LICENSES.md`](THIRD_PARTY_LICENSES.md)):

| Template id | OPT | JSON | XML | Probes |
|---|---|:---:|:---:|---|
| `cluster-slot.ehrbase.org.v0` | yes | yes | — | round-trip |
| `nested.en.v1` | yes | yes | — | round-trip |
| `IDCR Problem List.v1` | yes | — | yes | XML round-trip |
| `IDCR - Laboratory Test Report.v0` | yes | — | yes | XML round-trip |
| `IDCR -  Adverse Reaction List.v1` | yes | — | yes | XML round-trip (upstream double space in id) |

**WebTemplate oracles** (`webtemplate/`, pinned at commit `e57511c6aca27ed501d31d663762c37c3491e74e`; each OPT sits beside its reference WebTemplate golden, and file stems match `template_id`):

| Template id | Role | Size (OPT + golden) |
|---|---|---|
| `constrain_test` | PROBE-075 parity oracle (104/104). Pins **no** node name, so its golden carries **0** name predicates | 444 KB + 139 KB |
| `Corona_Anamnese` | REQ-116 oracle. It was the loud mode (`Build` → `ErrIDCollision`: four `SECTION.adhoc.v1` siblings; eight reused screening OBSERVATIONs under Symptome). Its golden carries 350 name-predicate segments over 213 `aqlPath`s. Since REQ-116 Phase 4 it builds and holds **230/230** structural parity | 1.2 MB + 230 KB |
| `GECCO_Diagnose` | REQ-116 oracle, silent mode. It always built, but it emitted bare paths where its golden carries 30 name-predicate segments over 24 `aqlPath`s (its three `/content` children have **distinct** archetype ids and are all predicated). Since REQ-116 Phase 4: **34/34** structural parity. The residuals are the golden's own `min=1` outlier (14 nodes) and 1 input delta, both documented | 210 KB + 73 KB |

The Corona pair is the largest fixture in the repo. That size is the cost of guarding the archetype-reuse-under-slot class with the real reference fixture instead of a synthetic cut-down.

### ehrbase (Robot integration-tests)

**License:** Apache 2.0 — [`THIRD_PARTY_LICENSES.md`](THIRD_PARTY_LICENSES.md). Ingest script: [`scripts/ingest-robot-fixtures.sh`](../../scripts/ingest-robot-fixtures.sh).

**Minimal entry** (`valid_templates/minimal/` + `xml_compositions/`):

| Template id | OPT | JSON | XML | Probes |
|---|---|:---:|:---:|---|
| `minimal_evaluation.en.v1` | yes | yes | yes | round-trip |
| `minimal_observation.en.v1` | yes | — | yes | XML only upstream |
| `minimal_admin.en.v1` | yes | — | yes | XML only upstream |
| `minimal_instruction.en.v1` | yes | yes | yes | round-trip |
| `minimal_action_2` | yes | yes | yes | round-trip (`minimal_action.en.v1` OPT does not compile) |

**Persistent:** `persistent_minimal.en.v1` (OPT + JSON + XML, round-trip).

**Constraint templates:** `clinical_content_validation` (OPT + JSON, round-trip); `Test_dv_*` (28 OPT+JSON pairs, all round-trip in PROBE-030; the two `Test_dv_interval_*_open_constraint` samples have the RM floor held out for inverted bounds, and all four `Test_dv_interval_*` stay out of the constraint-fixture axis). Not vendored: `cardinality_of_section`, `composition_evaluation_test` (duplicate AQL on compile).

**Added at the `b4625fc` pin** (valid OPT + canonical JSON only): `family_history.v.1.2.3`, `my_spanish_template_v0`, `terminology_test.ehrbase.org.v1`, `terminology_test2.ehrbase.org.v1`.

**RM JSON** (`rm/`, flat names): 8 `ehr_status_valid_*` in PROBE-030/033 (excludes ECIS alternate wire), all with the RM floor held out (`EHR_STATUS` without `archetype_details`); 11 `ehr_status_invalid_*` on disk for client/validation work but excluded from probe discovery (`ehr_status_invalid_*` prefix); 17 `folder_*` including `folder_update_*`.

**Submissions** ([`submissions/`](submissions/README.md)): 47 CONTRIBUTION create payloads from `contributions/` (bulk `create_multiple_compositions` omitted). Decode them with `contribution.Submission`, not `rm.Contribution`.

**AQL conformance corpus** ([`aql/conformance/`](aql/conformance/)): 12 FROM-family combination CSVs from `aql/fields_and_results/from/combinations/`, copied byte-exact and filed under the Robot suite family that consumes each one (`AND_OR`, `CONTAINS_A_D`, `EHR_STATUS`, `PREDICATE_A_D`, `USABLE_RM_TYPES_A_D`). Each row is a FROM/CONTAINS shape, and the family names the suite holding the query template it goes into (PROBE-100). Its own ingest, [`scripts/ingest-robot-aql.sh`](../../scripts/ingest-robot-aql.sh), vendors it on its own cadence, so it carries its own pin: [`AQL_SOURCE.txt`](aql/conformance/AQL_SOURCE.txt). That pin is authoritative for the bytes, where `ROBOT_SOURCE.txt` above is best-effort. [`EXCLUDED.txt`](aql/conformance/EXCLUDED.txt) is generated beside it and names all 1124 upstream files the corpus does not carry, each with a reason tag (`execution-semantics`, `non-from-family`, `unconsumed-by-suite`). Both files are generated; do not hand-edit them.

### SDK (`rm/`)

| File | Role |
|---|---|
| `composition_minimal.xml` | Minimal COMPOSITION XML |
| `dv_quantity.xml` | Leaf `DV_QUANTITY` XML |

### ITS-REST

See [`its_rest/README.md`](its_rest/README.md).

## Conventions

- Fixtures are immutable inputs. Fix the codec or refresh from upstream; do not patch fixtures to make tests pass.
- New template: add `templates/` + `compositions/` files; update this table. A composition is never skipped wholesale to keep probes green. If its vendored content carries a genuine RM-floor finding independent of the round trip, it still joins the corpus. It is held out of PROBE-030's `validation.ValidateRM` leg only, and named with its finding in `probe030SkipFloor` in [`probe_030_canjson_round_trip.go`](../probes/serialize/probe_030_canjson_round_trip.go). Composition XML the canxml round trip does not exercise goes in `compositionXMLExcluded`, and an alternate-wire or deliberately invalid `rm/` sample in `rmJSONExcluded` / `rmJSONExcludedPrefixes`, both in [`discover.go`](../fixtures/discover.go).
