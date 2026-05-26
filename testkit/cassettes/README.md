# testkit/cassettes

Vendored test cassettes for codec, validation, and probe tests. Checked in so CI does not require a sibling clone (REQ-082).

## Layout

```
cassettes/
  templates/{template-id}.opt
  compositions/{template-id}.json
  compositions/{template-id}.xml      # when vendored
  rm/{name}.json | {name}.xml         # RM probe samples (ehrbase, leaf XML, …)
  its_rest/                           # ITS-REST / discovery wire
```

Resolve paths via [`testkit/fixtures`](../fixtures/) (`TemplateOpt`, `CompositionJSON`, `CompositionXML`, `RMJSON`, `RMXML`).

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

### SDK (`rm/`)

| File | Role |
|---|---|
| `composition_minimal.xml` | Minimal COMPOSITION XML |
| `dv_quantity.xml` | Leaf `DV_QUANTITY` XML |

### ITS-REST

See [`its_rest/README.md`](its_rest/README.md).

## Conventions

- Immutable inputs — fix the codec or refresh from upstream, do not patch cassettes to green tests.
- New template: add `templates/` + `compositions/` files; update this table. If probes should skip, add the id to `compositionJSONExcluded` / `compositionXMLExcluded` in [`discover.go`](../fixtures/discover.go).
