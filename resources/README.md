# `resources/` — pinned SDK assets

In-tree, version-pinned inputs for code generation, validation, and conformance. Assets are grouped by kind so new pin sets (e.g. RM XSDs) can land in sibling subdirectories without mixing formats.

| Subdirectory | Contents |
|---|---|
| [`bmm/`](bmm/README.md) | openEHR BMM schemas (`*.bmm.json`) — source of truth for `openehr/rm/`, `openehr/aom/aom14/`, and related generated types |

See [`bmm/README.md`](bmm/README.md) for the schema inventory, provenance, and the BMM version-bump procedure (ADR 0001).

Future phases may add further subdirectories here (for example XSD releases alongside BMM pins per the [canonical XML serialization plan](../docs/plans/2026-05-15-canonical-xml-serialization.md)).
