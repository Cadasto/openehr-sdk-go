# Implementation plans

Active and historical implementation plans for `openehr-sdk-go`. Plans are derivations of [`../../specs/`](../../specs/) — they translate normative REQs into sequenced delivery.

| Plan | Scope | Covers REQs / strands |
|---|---|---|
| [2026-05-15-bmm-codegen.md](2026-05-15-bmm-codegen.md) | BMM-driven code generation for `openehr/rm/`, `openehr/aom/aom14/` | REQ-041..047; part of STRAND-04 |
| [2026-05-15-canonical-json-serialization.md](2026-05-15-canonical-json-serialization.md) | Canonical JSON encoder + decoder under `openehr/serialize/canjson/` | REQ-052, REQ-040; PROBE-030, PROBE-031; STRAND-04 (polymorphism side) |
| [2026-05-15-canonical-xml-serialization.md](2026-05-15-canonical-xml-serialization.md) | Canonical XML encoder + decoder under `openehr/serialize/canxml/` | REQ-056, REQ-040 |
| [2026-05-15-rest-api-client.md](2026-05-15-rest-api-client.md) | openEHR REST 1.1.0-development typed client family under `openehr/client/{system,ehr,query,definition,demographic,admin}/` | REQ-050, REQ-051, REQ-054, REQ-055, REQ-057, REQ-058, REQ-013..026, REQ-060..072, REQ-090..092; PROBE-010..013, PROBE-040..049; STRAND-01 |

Naming convention: `YYYY-MM-DD-<short-title>.md`. Each plan cites the REQ-IDs / STRAND-IDs it implements (specs/[README.md § Traceability](../../specs/README.md#traceability)).
