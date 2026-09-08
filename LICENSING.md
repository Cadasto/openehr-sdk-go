# Licensing

The SDK is **MIT**. That grant covers original code, tests, documentation, and plans in this repository. It does **not** re-license third-party artefacts that are pinned in-tree.

This file is the front-door inventory of that in-tree material. Per-tree READMEs and [`testkit/cassettes/THIRD_PARTY_LICENSES.md`](testkit/cassettes/THIRD_PARTY_LICENSES.md) keep the file lists, commit pins, and upstream copyright notices. Go module dependencies are declared in [`go.mod`](go.mod) and are not repeated here.

A full copy of Apache License 2.0 sits at [`licenses/Apache-2.0.txt`](licenses/Apache-2.0.txt) so redistributors do not have to fetch it.

## Inventory

| Artefact | Path | Upstream | Licence | Role |
|---|---|---|---|---|
| EHRbase openEHR_SDK test-data | [`testkit/cassettes/`](testkit/cassettes/) (`rm/`, template triplets, `webtemplate/`, `flat-conformance/`) | [ehrbase/openEHR_SDK](https://github.com/ehrbase/openEHR_SDK) | Apache-2.0 | Codec, WebTemplate, and FLAT parity oracles. Not part of the SDK runtime. |
| EHRbase Robot integration-test data | [`testkit/cassettes/`](testkit/cassettes/) (minimal / `Test_dv_*` triplets, `rm/ehr_status_*`, `rm/folder_*`, `submissions/`, [`aql/conformance/`](testkit/cassettes/aql/conformance/)) | [ehrbase/integration-tests](https://github.com/ehrbase/integration-tests) | Apache-2.0 | Validation, contribution-shape, and AQL admissibility fixtures. Not part of the SDK runtime. |
| CODE24 / Cadasto sample templates | [`testkit/cassettes/`](testkit/cassettes/) (see the cassette README) | CODE24 | MIT | Parser, validation, and serialization samples. No patient data. |
| openEHR ITS-REST OpenAPI | [`resources/its-rest/`](resources/its-rest/) | [openEHR/specifications-ITS-REST](https://github.com/openEHR/specifications-ITS-REST) | CC-BY-ND 3.0 (declared on each YAML `info.license`) | Normative REST contract (REQ-095). Unmodified. |
| openEHR AQL grammar | [`resources/aql/grammar/`](resources/aql/grammar/) | openEHR Foundation | CC-BY-SA 4.0 | Parser input. SDK deltas in `active/` are a documented derivative under the same terms. |
| openEHR BMM schemas | [`resources/bmm/`](resources/bmm/) | [openEHR/BMM-publisher](https://github.com/openEHR/BMM-publisher) | openEHR Foundation specification artefact | Pinned RM / AM / BASE inputs for codegen. |
| openEHR Terminology | [`resources/terminology/`](resources/terminology/) | [openEHR/specifications-TERM](https://github.com/openEHR/specifications-TERM) | openEHR Foundation specification artefact | Pinned vocabulary for `openehr/terminology` (REQ-034). |

Copyright notices for the Apache-2.0 corpora:

- openEHR_SDK test-data: Copyright 2021–2026 Vitasystems GmbH and Hannover Medical School.
- integration-tests Robot fixtures: Copyright (c) 2019 Vitasystems GmbH and Hannover Medical School.

Some vendored OPTs embed clinical models (CKM / IDCR / GECCO and similar) whose original authors may use Creative Commons terms of their own. This inventory attributes the trees we copied from; it does not replace those authors' notices inside the OPT files.

## What this is not

Runtime Go dependencies (OpenTelemetry, ANTLR, `x/oauth2`, go-oidc, go-jose) stay in `go.mod`. They are not vendored as source in this tree.

EHRbase's own OpenAPI documents are **not** pinned here. The REST contract is [`resources/its-rest/`](resources/its-rest/). EHRbase-specific deployment extensions that the SDK still calls (for example `PurgeTemplates` → `DELETE /admin/template/all`) are documented on the function and in [REQ-099](docs/specifications/module-layout.md#req-099--its-rest-admin-client-surface), not by a second OpenAPI pin.
