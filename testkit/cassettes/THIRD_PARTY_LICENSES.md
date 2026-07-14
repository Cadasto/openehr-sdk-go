# Third-party licenses — vendored cassettes

Some cassette fixtures under this directory are vendored from upstream projects. Per the upstream license terms, copyright and license notices are retained here.

## ehrbase/openEHR_SDK

**Source:** https://github.com/ehrbase/openEHR_SDK  
**Commit:** `4b5a710d3ddc3529a45222fb0398a2440bf83a9b` (2026-05-17)  
**Path within source:** `test-data/src/main/resources/`

```
Copyright 2021–2026 vitasystems GmbH and Hannover Medical School.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
```

**RM-only (`rm/`):** `minimal_evaluation.json`, `compo_with_nested_party_related.json`, `ehr_status_other_details_simple.json`, `nested_folder.json`, `test_all_types.v1.xml`, `simple_empty_folder.xml` — from `composition/canonical_json/`, `ehr/canonical_json/`, `folder/canonical_json/`, and `composition/canonical_xml/` in the commit above.

**Template triplets (`templates/` + `compositions/`):** `cluster-slot.ehrbase.org.v0`, `nested.en.v1`, `IDCR Problem List.v1`, `IDCR - Laboratory Test Report.v0`, `IDCR -  Adverse Reaction List.v1` — OPT from `operationaltemplate/` (or equivalent) and matching canonical JSON/XML from `composition/` in the same upstream tree.

**WebTemplate parity reference (`webtemplate/`):** `constrain_test.opt` (from `operationaltemplate/constrain_test.opt`) + `constrain_test.webtemplate.json` (from `webtemplate/constrain_test.json`), pinned at commit `22b01e0c99b53669394e56da29c2410838b5cf7e`. Vendored unmodified as the REQ-106 / PROBE-075 WebTemplate structural-parity oracle (ADR-0014); chosen because it compiles under the SDK's `templatecompile` (unique AQL paths) and exercises the core datatype set (DV_TEXT / CODED_TEXT / QUANTITY / COUNT / ORDINAL / DATE_TIME / DURATION / PROPORTION). Not part of the SDK runtime.

**Modifications:** Filename stems match operational `template_id` values; the WebTemplate JSON stem is suffixed `.webtemplate` to disambiguate it from canonical-composition JSON; no clinical content edits.

## ehrbase (integration-tests Robot)

**Source:** https://github.com/ehrbase/ehrbase  
**Path within source:** `integration-tests/tests/robot/_resources/test_data_sets/` (sibling clone under `/src/ehrbase/`)

```
Copyright vitasystems GmbH and Hannover Medical School (ehrbase project).

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
```

**Vendored:** Minimal-entry and `Test_dv_*` template triplets under `templates/` + `compositions/`; `persistent_minimal.en.v1`; flat `rm/ehr_status_*` and `rm/folder_*` JSON; `submissions/*.json` CONTRIBUTION create wire from `contributions/`.

**Modifications:** Flat `rm/` and `submissions/` filenames; composition JSON stems match operational `template_id`; no clinical content edits. Re-ingest via `scripts/ingest-robot-cassettes.sh`.

## CODE24 (Cadasto)

**Files:** CODE24-sourced templates under `templates/` paired with `compositions/` (see [README.md](README.md)); benchmark `vital_signs` and `clinical_notes.v0`.

**License:** MIT

Sample clinical and template-definition artefacts contributed by CODE24 for SDK parser, validation, and serialization testing. No patient data.
