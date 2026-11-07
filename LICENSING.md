# Licensing

The SDK is **MIT** ([`LICENSE`](LICENSE)). That grant covers original code, tests, documentation, and plans in this repository. It does **not** re-license third-party artefacts that are pinned in-tree.

This file is the front-door inventory of that in-tree material. Per-tree READMEs and [`testkit/corpus/THIRD_PARTY_LICENSES.md`](testkit/corpus/THIRD_PARTY_LICENSES.md) keep the file lists, commit pins, and upstream copyright notices. Go module dependencies are declared in [`go.mod`](go.mod) and are not repeated here.

A full copy of Apache License 2.0 sits at [`licenses/Apache-2.0.txt`](licenses/Apache-2.0.txt) so redistributors do not have to fetch it.

## Inventory

| Artefact | Path | Upstream | Licence | Role |
|---|---|---|---|---|
| EHRbase openEHR_SDK test data | [`testkit/corpus/`](testkit/corpus/) (`rm/`, template triplets from `test-data` and the validation test resources, `webtemplate/`, `flat-conformance/`, six of the [`crossformat/`](testkit/corpus/crossformat/) sets) | [ehrbase/openEHR_SDK](https://github.com/ehrbase/openEHR_SDK) | Apache-2.0 | Codec, WebTemplate, FLAT, and cross-format parity oracles. Not part of the SDK runtime. |
| EHRbase Robot integration-test data | [`testkit/corpus/`](testkit/corpus/) (minimal / `Test_dv_*` / family-history / terminology-test triplets, the `alternative_types.en.v1` OPT, `rm/ehr_status_*`, `rm/folder_*`, `submissions/`, [`aql/suites/`](testkit/corpus/aql/suites/), four of the [`crossformat/`](testkit/corpus/crossformat/) sets) | [ehrbase/integration-tests](https://github.com/ehrbase/integration-tests) | Apache-2.0 | Validation, contribution-shape, AQL admissibility, and cross-format parity fixtures. Not part of the SDK runtime. |
| EHRbase server test resources | [`testkit/corpus/`](testkit/corpus/) (the `BMI` OPT, [`ehrbase-service/`](testkit/corpus/ehrbase-service/)) | [ehrbase/ehrbase](https://github.com/ehrbase/ehrbase) | Apache-2.0 | Template samples for the tests that read the whole template corpus. Not part of the SDK runtime. |
| CaboLabs openEHR-SDK sample template | [`testkit/corpus/`](testkit/corpus/) (the `Vital signs monitoring` OPT and composition) | [CaboLabs/openEHR-SDK](https://github.com/CaboLabs/openEHR-SDK) | Apache-2.0 | A second, independently authored vital-signs template for the tests that read the whole template corpus. Not part of the SDK runtime. |
| openEHR CKM sample template | [`testkit/corpus/`](testkit/corpus/) (the `Vital signs CKM` OPT) | [openEHR CKM](https://ckm.openehr.org/ckm/) | CC-BY-SA (CKM content) | CKM's *Vital signs* template, renamed; see the fixture licences file. Not part of the SDK runtime. |
| Ripple IDCR sample template | [`testkit/corpus/`](testkit/corpus/) (the `IDCR - Vital Signs Encounter.v1` OPT, and the `IDCR - Vital Signs Encounter - updated.v1` OPT built from it on current CKM archetypes, CC-BY-SA) | [RippleOSI/Ripple-openEHR](https://github.com/RippleOSI/Ripple-openEHR) | Apache-2.0 | A vital-signs template with a SECTION, on the earlier archetype revisions. Not part of the SDK runtime. |
| CODE24 / Cadasto sample templates | [`testkit/corpus/`](testkit/corpus/) (see the fixture README) | CODE24 (`clinical_notes.v0` with Medblocks) | MIT | Parser, validation, and serialization samples. No patient data. |
| openEHR ITS-REST OpenAPI | [`resources/its-rest/`](resources/its-rest/) | [openEHR/specifications-ITS-REST](https://github.com/openEHR/specifications-ITS-REST) | CC-BY-ND 3.0 (declared on each YAML `info.license`) | Normative REST contract (REQ-095). Unmodified. |
| openEHR ITS-XML schemas | [`resources/its-xml/`](resources/its-xml/) | [openEHR/specifications-ITS-XML](https://github.com/openEHR/specifications-ITS-XML) | Apache-2.0 (the upstream repository's `LICENSE`; the files carry no licence header) | Child-element order for the canonical-XML encoders (REQ-056, ADR 0002 D10). Unmodified. |
| openEHR AQL grammar | [`resources/aql/grammar/`](resources/aql/grammar/) | openEHR Foundation | CC-BY-SA 3.0 Unported | Parser input. SDK deltas in `active/` are a documented derivative under the same terms. |
| openEHR BMM schemas | [`resources/bmm/`](resources/bmm/) | [openEHR/specifications-ITS-BMM](https://github.com/openEHR/specifications-ITS-BMM) | openEHR Foundation specification artefact | Pinned RM / AM / BASE inputs for codegen. |
| openEHR Terminology | [`resources/terminology/`](resources/terminology/) (`openehr_terminology.xml`, `openehr_external_terminologies.xml`, `PropertyUnitData.xml`) | [openEHR/specifications-TERM](https://github.com/openEHR/specifications-TERM) | openEHR Foundation specification artefact | Pinned vocabulary for `openehr/terminology`: the openEHR groups and code sets, the Foundation's snapshot of the ISO and IANA code sets, and the property-unit table. |

Copyright notices for the Apache-2.0 corpora:

- openEHR_SDK test data: Copyright 2021–2026 Vitasystems GmbH and Hannover Medical School.
- integration-tests Robot fixtures: Copyright (c) 2019 Vitasystems GmbH and Hannover Medical School.
- ehrbase server test resources: Copyright (c) 2019 Vitasystems GmbH and Hannover Medical School.
- CaboLabs openEHR-SDK sample template: the repository's `LICENSE` names no copyright holder; the project is published by CaboLabs.
- Ripple IDCR sample template: the repository's `LICENSE` names no copyright holder; the repository is published under the RippleOSI organisation.

Some vendored OPTs embed clinical models (CKM / IDCR / GECCO and similar) whose original authors may use Creative Commons terms of their own. This inventory attributes the trees we copied from; it does not replace those authors' notices inside the OPT files.

## What this is not

Runtime Go dependencies (OpenTelemetry, ANTLR, `x/oauth2`, go-oidc, go-jose) stay in `go.mod`. They are not vendored as source in this tree.

EHRbase's own OpenAPI documents are **not** pinned here. The REST contract is [`resources/its-rest/`](resources/its-rest/). EHRbase-specific deployment extensions that the SDK still calls (for example `PurgeTemplates` → `DELETE /admin/template/all`) are documented on the function and in [REQ-099](docs/specifications/module-layout.md#req-099--its-rest-admin-client-surface). There is no second OpenAPI pin for them.
