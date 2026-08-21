# testkit/cassettes

Vendored **fixture documents** — OPTs, compositions, RM samples, wire bodies, and reference goldens for codec, validation, and probe tests. Checked in so CI does not require a sibling clone.

**These are not REQ-082 Cassette-mode recordings.** Everything here is a request or response *body*: it carries no method, URL, header, or status code, so none of it can be replayed as an HTTP exchange. The Cassette mode that [REQ-082](../../docs/specifications/conformance.md#req-082--runnability) mandates records whole exchanges and lands under `testkit/recordings/`. The directory name predates that distinction; the fixtures are addressed through [`testkit/fixtures`](../fixtures/paths.go), so it is kept rather than renamed.

## Layout

```
cassettes/
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
```

**Pinned subtree.** Everything under `flat-conformance/` is machine-synced from
upstream at a recorded commit — do not hand-edit it. Refresh with
`make flat-conformance-sync`; verify integrity with `make flat-conformance-verify`
(offline `sha256`, the gate `make ci` runs). `make flat-conformance-check` adds an
upstream-drift report (needs network; dev helper, not a gate). Resolve paths via
[`fixtures.FlatConformanceOpt`](../fixtures/paths.go) /
`fixtures.FlatConformanceFlat` / `fixtures.ListFlatConformance`. The rest of
this directory is curated by hand and is not covered by that manifest; the
EHRbase Robot integration-test subset records the upstream commit it was
ingested from in [`ROBOT_SOURCE.txt`](ROBOT_SOURCE.txt) (a provenance pin, not
a per-file `sha256` lock).

Resolve paths via [`testkit/fixtures`](../fixtures/) (`TemplateOpt`, `CompositionJSON`, `CompositionXML`, `RMJSON`, `RMXML`, `SubmissionJSON`, `WebTemplateOpt`, `WebTemplateReference`).

Composition JSON uses template ids **without** `::{uuid}` suffixes.

**Probe vs on-disk.** Vendored `*.json` / `*.xml` under `compositions/` may be omitted from [`ListCompositionJSON`](../fixtures/discover.go) / [`ListRMXML`](../fixtures/discover.go) when canjson/canxml cannot round-trip yet; files remain for template and instance work via `fixtures.CompositionJSON(id)`.

## Index by vendor

### Benchmark (internal)

| Template id | OPT | JSON | XML |
|---|---|:---:|:---:|
| `vital_signs` | yes | yes | — |
| `clinical_notes.v0` | yes | yes | — |

### CODE24 (Cadasto)

**License:** MIT — [`THIRD_PARTY_LICENSES.md`](THIRD_PARTY_LICENSES.md).

| Template id | OPT | JSON | XML | Probes |
|---|---|:---:|:---:|---|
| `body_weight` | yes | yes | yes | round-trip |
| `BMI` | yes | yes | yes | round-trip |
| `alternative_types.en.v1` | yes | yes | yes | round-trip |
| `test_template_rename_node` | yes | yes | yes | round-trip |
| `test_template_rename_node_2` | yes | yes | yes | round-trip |
| `Episode.v2` | yes | yes | yes | round-trip |
| `Address.v2` | yes | yes | yes | JSON/XML on disk; probes skip (codec) |
| `Demonstration.v1` | yes | yes | yes | probes skip |
| `TestPerson.v2` | yes | yes | yes | probes skip |

### ehrbase (openEHR_SDK)

**License:** Apache 2.0 — [`THIRD_PARTY_LICENSES.md`](THIRD_PARTY_LICENSES.md) (commit `4b5a710d3ddc3529a45222fb0398a2440bf83a9b`, 2026-05-17).

**RM-only** (`rm/`, no OPT):

| File | RM root |
|---|---|
| `minimal_evaluation.json` | COMPOSITION |
| `compo_with_nested_party_related.json` | COMPOSITION |
| `ehr_status_other_details_simple.json` | EHR_STATUS |
| `nested_folder.json` | FOLDER |
| `test_all_types.v1.xml` | COMPOSITION |
| `simple_empty_folder.xml` | FOLDER |

**Template triplets** (`templates/` + `compositions/`, from openEHR_SDK test-data):

| Template id | OPT | JSON | XML | Probes |
|---|---|:---:|:---:|---|
| `cluster-slot.ehrbase.org.v0` | yes | yes | — | round-trip |
| `nested.en.v1` | yes | yes | — | round-trip |
| `IDCR Problem List.v1` | yes | — | yes | XML round-trip |
| `IDCR - Laboratory Test Report.v0` | yes | — | yes | XML round-trip |
| `IDCR -  Adverse Reaction List.v1` | yes | — | yes | XML round-trip (upstream double space in id) |

**WebTemplate oracles** (`webtemplate/`, pinned at commit `22b01e0c99b53669394e56da29c2410838b5cf7e` — OPT beside its reference WebTemplate golden, stems match `template_id`):

| Template id | Role | Size (OPT + golden) |
|---|---|---|
| `constrain_test` | PROBE-075 parity oracle (104/104) — pins **no** node name, so its golden carries **0** name predicates | 444 KB + 139 KB |
| `Corona_Anamnese` | REQ-116 oracle — was the loud mode (`Build` → `ErrIDCollision`: four `SECTION.adhoc.v1` siblings; eight reused screening OBSERVATIONs under Symptome); golden carries 350 name-predicate segments over 213 `aqlPath`s. Since REQ-116 Phase 4 it builds and holds **230/230** structural parity | 1.2 MB + 230 KB |
| `GECCO_Diagnose` | REQ-116 oracle — silent mode: always built, but its golden carries 30 name-predicate segments over 24 `aqlPath`s (its three `/content` children have **distinct** archetype ids and are all predicated) it emitted bare. Since REQ-116 Phase 4: **34/34** structural parity; residuals are the golden's own `min=1` outlier (14 nodes) and 1 input delta, both documented | 210 KB + 73 KB |

The Corona pair is the largest cassette in the repo — the size is the cost of guarding the archetype-reuse-under-slot class with the real reference fixture rather than a synthetic cut-down.

### ehrbase (Robot integration-tests)

**License:** Apache 2.0 — [`THIRD_PARTY_LICENSES.md`](THIRD_PARTY_LICENSES.md). Ingest script: [`scripts/ingest-robot-cassettes.sh`](../../scripts/ingest-robot-cassettes.sh).

**Minimal entry** (`valid_templates/minimal/` + `xml_compositions/`):

| Template id | OPT | JSON | XML | Probes |
|---|---|:---:|:---:|---|
| `minimal_evaluation.en.v1` | yes | yes | yes | round-trip |
| `minimal_observation.en.v1` | yes | — | yes | XML only upstream |
| `minimal_admin.en.v1` | yes | — | yes | XML only upstream |
| `minimal_instruction.en.v1` | yes | yes | yes | round-trip |
| `minimal_action_2` | yes | yes | yes | round-trip (`minimal_action.en.v1` OPT does not compile) |

**Persistent:** `persistent_minimal.en.v1` (OPT + JSON + XML, round-trip).

**Constraint templates:** `clinical_content_validation` (OPT + JSON, round-trip); `Test_dv_*` (24 OPT+JSON pairs, round-trip except four `Test_dv_interval_*` — probes skip; see PROBE-038). Not vendored: `cardinality_of_section`, `composition_evaluation_test` (duplicate AQL on compile).

**RM JSON** (`rm/`, flat names): 8 `ehr_status_valid_*` in PROBE-030/033 (excludes ECIS alternate wire); 12 `ehr_status_invalid_*` on disk for client/validation work but excluded from probe discovery (`ehr_status_invalid_*` prefix); 14 `folder_*` including `folder_update_*`.

**Submissions** ([`submissions/`](submissions/README.md)): 47 CONTRIBUTION create payloads from `contributions/` (bulk `create_multiple_compositions` omitted) — use `contribution.Submission`, not `rm.Contribution` decode.

### SDK (`rm/`)

| File | Role |
|---|---|
| `composition_minimal.xml` | Minimal COMPOSITION XML |
| `dv_quantity.xml` | Leaf `DV_QUANTITY` XML |

### ITS-REST

See [`its_rest/README.md`](its_rest/README.md).

## Conventions

- Immutable inputs — fix the codec or refresh from upstream, do not patch cassettes to green tests.
- New template: add `templates/` + `compositions/` files; update this table. If probes should skip, add the id to `compositionJSONExcluded` / `compositionXMLExcluded` / `rmJSONExcluded` in [`discover.go`](../fixtures/discover.go).
