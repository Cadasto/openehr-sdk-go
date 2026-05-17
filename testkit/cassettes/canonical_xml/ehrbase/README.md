# ehrbase canonical XML cassettes

Vendored from [ehrbase/openEHR_SDK](https://github.com/ehrbase/openEHR_SDK) under the Apache License, Version 2.0.

```
Copyright (c) 2021 Vitasystems GmbH and Hannover Medical School.

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

The full third-party license inventory lives in [`../../THIRD_PARTY_LICENSES.md`](../../THIRD_PARTY_LICENSES.md).

## Source

- **Upstream:** [`ehrbase/openEHR_SDK`](https://github.com/ehrbase/openEHR_SDK)
- **Commit:** `4b5a710d3ddc3529a45222fb0398a2440bf83a9b`
- **Snapshot date:** 2026-05-17 (`develop` branch)
- **Source path:** `test-data/src/main/resources/`

## Files

| Local | Upstream path | Coverage |
|---|---|---|
| `IDCR-LabReportRAW1.xml` | `composition/canonical_xml/IDCR-LabReportRAW1.xml` | Lab report COMPOSITION with `OBJECT_VERSION_ID` uid and nested observations |
| `test_all_types.v1.xml` | `composition/canonical_xml/test_all_types.v1.xml` | COMPOSITION exercising every `DV_*` type, including `DV_INTERVAL<DV_QUANTITY>` |
| `simple_empty_folder.xml` | `folder/canonical_xml/simple_empty_folder.xml` | Minimal FOLDER without `archetype_node_id` (tolerance test) |

## Upstream wire profile

The upstream files follow the openEHR ITS-XML XSD profile, notably:

- `archetype_node_id` as an XML attribute on every LOCATABLE descendant (not a child element).
- `xmlns:xsi` declared on the root with no default `xmlns="http://schemas.openehr.org/v1"` declaration.
- `xsi:type` on every concrete value at a polymorphic site, including generic concrete types like `DV_INTERVAL`.

The SDK's canxml decoder accepts both the upstream profile and the SDK-deterministic profile; the encoder emits a canonical form that round-trips through itself but differs in byte shape from the upstream files (we declare `xmlns="…"` on the root and emit element bodies where upstream uses self-closing form on empty elements).

## Modifications

None. Files are vendored byte-identical from the upstream commit referenced above.
