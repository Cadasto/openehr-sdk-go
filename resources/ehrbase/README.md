# `resources/ehrbase/` — pinned EHRbase OpenAPI specs

Vendored OpenAPI documents describing the **EHRbase server implementation** — what a deployed EHRbase instance exposes, including EHRbase-specific admin, experimental, and enterprise endpoints. These are **not** the normative openEHR ITS-REST contract; for that see [`resources/its-rest/`](../its-rest/README.md).

**Source:** [docs.ehrbase.org/redocusaurus/](https://docs.ehrbase.org/redocusaurus/) (generated from the EHRbase Spring Boot app via Springdoc).

**Fetched:** 2026-06-19T14:35:42Z

| File | Upstream | Scope | License |
|---|---|---|---|
| `ehr.openapi.yaml` | `hip-ehrbase-ehr.yaml` | Core openEHR REST (EHR, query, definition) | Apache-2.0 |
| `admin.openapi.yaml` | `hip-ehrbase-admin.yaml` | Admin API (templates, EHR merge, …) | Apache-2.0 |
| `tags.openapi.yaml` | `hip-ehrbase-tags.yaml` | Experimental item-tag API | Apache-2.0 |
| `enterprise.openapi.yaml` | `hip-ehrbase-enterprise.yaml` | HIP enterprise plugins | **not OSS** (see below) |

Reference assets only — the SDK does not generate code from these files.

## License & attribution

These OpenAPI documents are produced by the **[EHRbase](https://github.com/ehrbase/ehrbase) project** (© vitasystems GmbH and the EHRbase contributors). They are vendored here unmodified for reference.

- `ehr.openapi.yaml`, `admin.openapi.yaml`, and `tags.openapi.yaml` declare themselves **Apache License 2.0** (`info.license` in each file) — see the upstream [`LICENSE.md`](https://github.com/ehrbase/ehrbase/blob/develop/LICENSE.md). They are redistributed here under those terms, with attribution to the EHRbase project; the SDK itself remains MIT-licensed.
- `enterprise.openapi.yaml` describes **HIP EHRbase Enterprise** operations that are **not part of the open-source distribution** and carry **no Apache-2.0 grant**. It is included solely as a reference to the enterprise API surface; its use is governed by EHRbase/vitasystems enterprise terms, not by this repository's MIT license or by Apache-2.0.

This attribution is intentionally scoped to these vendored OpenAPI files. The normative contract the SDK targets is the openEHR ITS-REST spec in [`resources/its-rest/`](../its-rest/README.md) (openEHR Foundation), not these EHRbase documents.
