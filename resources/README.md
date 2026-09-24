# `resources/` — pinned SDK assets

In-tree, version-pinned inputs for code generation, validation, and conformance. Each kind of asset has its own subdirectory, so a new pin set (for example RM XSDs) can land beside the others without mixing formats.

| Subdirectory | Contents |
|---|---|
| [`bmm/`](bmm/README.md) | openEHR BMM schemas (`*.bmm.json`), the source of truth for `openehr/rm/`, `openehr/aom/aom14/`, and related generated types |
| [`aql/`](aql/) | AQL grammar profile assets (ADR 0007) consumed by `openehr/aql/parse` |
| [`its-rest/`](its-rest/README.md) | openEHR REST API OpenAPI specs (`*-validation.openapi.yaml`): the machine-readable contract that `transport/` and `openehr/client/*` target. Synced via `make its-rest-sync` |
| [`terminology/`](terminology/README.md) | The openEHR Terminology (`openehr_terminology.xml`, TERM Release-3.0.0), the source of truth for the generated `openehr/terminology` accessor. Synced via `make terminology-sync` |

See [`bmm/README.md`](bmm/README.md) for the schema inventory, provenance, and the BMM version-bump procedure (ADR 0001). See [`its-rest/README.md`](its-rest/README.md) for the REST API spec inventory and the sync/pin procedure.

Later phases may add more subdirectories here, for example XSD releases beside the BMM pins, as described in the [canonical XML serialization plan](../docs/plans/archive/2026-05-15-canonical-xml-serialization.md).
