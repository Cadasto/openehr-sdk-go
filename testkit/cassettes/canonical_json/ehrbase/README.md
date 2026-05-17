# ehrbase canonical JSON cassettes

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

| Local | Upstream path |
|---|---|
| `minimal_evaluation.json` | `composition/canonical_json/minimal_evaluation.json` |
| `compo_with_nested_party_related.json` | `composition/canonical_json/compo_with_nested_party_related.json` |
| `ehr_status_other_details_simple.json` | `ehr/canonical_json/ehr_status_other_details_simple.json` |
| `nested_folder.json` | `folder/canonical_json/nested_folder.json` |

## Coverage rationale

Chosen for breadth/weight balance: COMPOSITION (entry-class variety), COMPOSITION (polymorphic PARTY_RELATED), EHR_STATUS (with `other_details` ITEM_LIST), and FOLDER. Total ~8 KiB.

## Modifications

None. Files are vendored byte-identical from the upstream commit referenced above.

## Deliberately excluded

- `contribution-two_entries-composition.json` — upstream serializes `CONTRIBUTION.versions` as full `ORIGINAL_VERSION` objects, but the openEHR Common IM 1.0.x BMM declares `versions: Set<OBJECT_REF>`. The SDK follows the BMM strictly, so the embedded-version shape decodes as a type mismatch. Re-evaluate when the BMM gets a "VERSIONED_OBJECT inline" variant.
