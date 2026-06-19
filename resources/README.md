# `resources/` — pinned SDK assets

In-tree, version-pinned inputs for code generation, validation, and conformance. Assets are grouped by kind so new pin sets (e.g. RM XSDs) can land in sibling subdirectories without mixing formats.

| Subdirectory | Contents |
|---|---|
| [`bmm/`](bmm/README.md) | openEHR BMM schemas (`*.bmm.json`) — source of truth for `openehr/rm/`, `openehr/aom/aom14/`, and related generated types |
| [`aql/`](aql/) | AQL grammar profile assets (ADR 0007) consumed by `openehr/aql/parse` |
| [`its-rest/`](its-rest/README.md) | openEHR REST API OpenAPI specs (`*-validation.openapi.yaml`) — the machine-readable contract `transport/` + `openehr/client/*` target; synced via `make its-rest-sync` |
| [`ehrbase/`](ehrbase/README.md) | EHRbase implementation OpenAPI specs — reference for EHRbase-specific endpoints and deployment extensions |

See [`bmm/README.md`](bmm/README.md) for the schema inventory, provenance, and the BMM version-bump procedure (ADR 0001). See [`its-rest/README.md`](its-rest/README.md) for the REST API spec inventory and the sync/pin procedure.

Future phases may add further subdirectories here (for example XSD releases alongside BMM pins per the [canonical XML serialization plan](../docs/plans/archive/2026-05-15-canonical-xml-serialization.md)).
