# `resources/its-rest/` — pinned openEHR REST API (ITS-REST) OpenAPI specs

Vendored, version-pinned OpenAPI 3.0 documents for the **openEHR REST API** — the
machine-readable contract the SDK's `transport/` + `openehr/client/*` packages are
written against. They are the authoritative reference for endpoint paths, request /
response shapes, headers (`Prefer`, `If-Match`, `openEHR-*`), status codes, and the
wire JSON/XML schemas of every REST resource.

> **For AI agents and contributors:** when you need to know the openEHR REST API
> schema — what a request/response body looks like, which headers a verb takes,
> what status codes a resource returns, or which endpoints exist for a resource —
> read these files. They are the canonical source. Prefer them over guessing or
> over the prose specs at <https://specifications.openehr.org/releases/ITS-REST/>.
> The `openehr-assistant` MCP server complements these for narrative spec lookups.

## Provenance

- **Source:** [`openEHR/specifications-ITS-REST`](https://github.com/openEHR/specifications-ITS-REST) — `computable/OAS/*-validation.openapi.yaml`.
- **Flavour:** the `*-validation` variant (canonical, vendor-extension-light). The
  sibling `*-codegen` / `*-html` flavours carry the same schema with tooling-specific
  extensions and are intentionally **not** vendored.
- **Pin + integrity:** the exact upstream commit and a per-file `sha256` are recorded
  in [`MANIFEST.txt`](MANIFEST.txt). Regenerate with `make its-rest-sync`; verify with
  `make its-rest-check` (see [the sync script](../../scripts/sync-its-rest-specs.sh)).

These are **reference assets, not code-generation inputs** — the SDK does not generate
code from them, so drift is informational rather than a build break. They are pinned
in-tree so builds need no network fetch and the API surface the SDK targets is
reviewable in version control.

## Files

| File | API | Upstream status |
|---|---|---|
| `ehr-validation.openapi.yaml` | EHR API — EHR, COMPOSITION, DIRECTORY, EHR_STATUS, CONTRIBUTION, VERSIONED_* | STABLE |
| `query-validation.openapi.yaml` | Query API — AQL (ad-hoc + stored) | STABLE |
| `definition-validation.openapi.yaml` | Definition API — stored AQL + ADL 1.4 / 2 templates | STABLE |
| `admin-validation.openapi.yaml` | Admin API — EHR / resource administration | DEVELOPMENT |
| `demographic-validation.openapi.yaml` | Demographic API — PARTY hierarchy (PERSON/ORGANISATION/GROUP/AGENT/ROLE) + `versioned_party` | DEVELOPMENT |
| `system-validation.openapi.yaml` | System API — capabilities / conformance discovery | STABLE |
| `overview-validation.openapi.yaml` | Overview — shared components, security schemes, common parameters | STABLE |

`x-status: DEVELOPMENT` upstream marks the unstable APIs (Admin, Demographic) — the
SDK clients for those ship as **Draft** and may change between minor versions.

## Updating the pin

```bash
make its-rest-sync          # fetch latest from master, rewrite MANIFEST.txt
ITS_REST_REF=v1.2.0 make its-rest-sync   # pin a tag / branch / commit instead
make its-rest-check         # verify local copies + report if upstream advanced
```

A version bump is an explicit, reviewable commit (diff the YAML + `MANIFEST.txt`).
