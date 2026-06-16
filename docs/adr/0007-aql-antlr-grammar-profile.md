# ADR 0007 — AQL parser: ANTLR + SDK grammar profile

- **Status:** Accepted, 2026-06-15.
- **Supersedes:** —
- **Superseded by:** —
- **Tracks:** [`docs/plans/archive/2026-06-15-aql-lint.md`](../plans/archive/2026-06-15-aql-lint.md) (REQ-109 AQL static lint). Related: [ADR 0001](0001-bmm-version-bump-runbook.md) (version-bump runbook spirit, reused for grammar rebases).

## Context

REQ-109 needs to parse hand-written, imported, or `NewQuery(literal)` AQL for static lint — distinct from the REQ-055 builders (which emit AQL by construction) and from the CDR (the execute-time semantic authority, PROBE-021). A parser is required; the question is how to obtain one.

openEHR publishes the AQL grammar as ANTLR4 `.g4` files ([QUERY Release-1.1.0](https://github.com/openEHR/specifications-QUERY/tree/Release-1.1.0/docs/AQL/grammar)). The alternatives were ANTLR (reuse the official grammar; generated parser) versus a pure-Go parser (participle / pigeon / hand-written; no codegen, but the grammar must be transcribed and maintained by hand, drifting from the spec).

The lint also needs a small, controlled set of deltas from official AQL — most notably accepting `SELECT *` (a relaxation some CDRs honour), which the official grammar rejects.

## Decision

**Use ANTLR with the official `.g4` as the baseline, plus an SDK grammar profile.**

- **`resources/aql/grammar/baseline/`** — a frozen copy of the official openEHR `AqlLexer.g4` / `AqlParser.g4` + a `PIN` (provenance: QUERY release, source URL, ANTLR tool + runtime versions). Read-only; changes only on a QUERY release bump.
- **`resources/aql/grammar/active/`** — `baseline/` plus the deltas in `DIVERGENCES.md`. The only input to codegen. Every delta between `baseline/` and `active/` has a stable `SDK-AQL-NNN` row, classified as a **relaxation** (admits more than official AQL) or a **correction** (fixes a foundation-grammar bug). v1 applies **five** documented deltas (**SDK-AQL-001** … **SDK-AQL-005**); see [`resources/aql/grammar/DIVERGENCES.md`](../../resources/aql/grammar/DIVERGENCES.md) for the canonical list.
- **Generated parser** is committed under `openehr/aql/parse/gen/` (package `gen`) and compiles against the pure-Go runtime `github.com/antlr4-go/antlr/v4`. `go build` / `go test` / `make ci` never run the generator.
- **The generator (Java) is containerised, codegen-only.** A Dockerfile `antlr` stage carries the pinned ANTLR jar; `make aqlgen` regenerates `parse/gen/` from `active/`, `make aqlgen-verify` fails on drift. Neither the host, the `dev` image, nor ordinary CI needs a JRE — only Docker, and only for the maintainer-run regenerate/verify. This mirrors the existing `LINT_IMAGE` shim.
- **Version lockstep:** the ANTLR tool version (`ANTLR_VERSION`, Dockerfile + Makefile) and the `antlr4-go/antlr/v4` runtime (go.mod) are released together; a mismatch produces Go that will not compile. Bump both in one change; record in `resources/aql/grammar/baseline/PIN`. v1 pins tool **4.13.2** / runtime **v4.13.1**.
- **No external fixture/runtime deps beyond the runtime:** the openEHR-antlr4 `test_fixtures` and `reader_aql` are not imported or CI-pinned; tests are in-repo only.

The lint is a pre-flight aid, not a conformance gate: **lint-clean does not imply the CDR will accept the query.** The `SELECT *` relaxation is safe under that contract.

## Consequences

- The SDK gains its first non-OpenTelemetry runtime dependency (`antlr4-go/antlr/v4`, pure Go). Documented in [architecture.md § Dependencies](../architecture.md#dependencies).
- `vet` excludes `openehr/aql/parse/gen` — ANTLR's Go output has unreachable code after panics (inherent, not a defect); golangci-lint already skips generated files.
- Spec fidelity and a clean rebase path are retained: a QUERY release bump re-imports `baseline/`, re-applies or drops each `SDK-AQL-NNN`, and re-runs `make aqlgen-verify` (the ADR 0001 runbook spirit).
- The Java toolchain is a maintainer/Docker concern only; it never enters the build, test, or normal CI path.
- If a future need arises to drop the codegen container entirely, a pure-Go parser (participle) remains a documented fallback in the plan — at the cost of hand-maintaining the grammar.
