# Plan — AQL `SELECT TOP` carrier and literal source text

**Date:** 2026-08-05
**Status:** Landed
**Owner:** SDK maintainers
**Covers:** **REQ-118** ([clinical-modeling.md § REQ-118](../../specifications/clinical-modeling.md#req-118--deprecated-select-top-clause-and-literal-source-text)) — amends [§ REQ-117](../../specifications/clinical-modeling.md#req-117--aql-expression-catalogue-completion) (residual list) and extends the [§ REQ-109](../../specifications/clinical-modeling.md#layer-2--shape-ast-walk-no-cdr) Layer-2 lint catalogue by two codes
**Probes:** [PROBE-087](../../specifications/conformance.md#probe-087--aql-structured-ast-catalogue-completeness) (extended), [PROBE-088](../../specifications/conformance.md#probe-088--aql-builder-containment-and-paging-stability) (extended); PROBE-028 unaffected
**Implementation:** landed
**Depends on:** REQ-113 (structured AST), REQ-117 (catalogue completion, `LiteralExpr` / in-text paging), REQ-055 (canonical write form) — all landed
**Defers:** `TOP` semantics for a CDR that tolerates `TOP` + `LIMIT`; source text for arbitrary SELECT expressions or WHERE-side values; any grammar-profile change (no new `SDK-AQL-NNN` row)

## Goal

Close the two remaining fidelity holes in the structured AQL AST that a consumer hits when it *reads* third-party AQL rather than only building its own: the deprecated `SELECT TOP n [FORWARD|BACKWARD]` clause (today dropped at extraction, so a bounded query is reported as a toolchain defect), and a projected literal's source text (today unavailable, so an unaliased literal column cannot be named as the openEHR result schema requires). Both land as shared read/write vocabulary, so the builder can construct the deprecated clause as well as parse it, and the spec-forbidden `TOP` + `LIMIT` combination becomes reportable instead of invisible.

## Definition of Ready

- **`Covers:`** names REQ-118 and the two REQs it amends/extends. ✅
- Canonical normative prose exists: [clinical-modeling.md § REQ-118](../../specifications/clinical-modeling.md#req-118--deprecated-select-top-clause-and-literal-source-text) + the [REQ.md](../../specifications/REQ.md) registry row + the [traceability.yaml](../../specifications/traceability.yaml) entry. ✅
- No irreversible fork needs an ADR: the two live decisions (a dedicated `TOP` carrier rather than `LimitExpr` reuse; canonical emission with read-side-only source text) follow from the grammar profile and the already-normative canonical write form, and are stated in the REQ-118 § itself. ✅
- Ground truth quoted, not guessed: openEHR **QUERY Release-1.1.0 § 4.4.3** for the syntax, the deprecation, and the `TOP`-with-`LIMIT` prohibition; `resources/aql/grammar/active/AqlParser.g4` (`top : TOP INTEGER direction=(FORWARD|BACKWARD)?`) for what the profile actually admits. ✅

## Definition of Done

- Code and tests carry `// REQ-118` / `// PROBE-087` / `// PROBE-088` citations.
- `traceability.yaml` `implementation: proposed → landed`, REQ.md **Impl.** column `planned → landed`.
- PROBE-080/087 case counts in [conformance.md](../../specifications/conformance.md) and [clinical-modeling.md](../../specifications/clinical-modeling.md) re-pinned to the grown corpus (they are stated as exact numbers, so a stale count is drift).
- `make spec-check` and `make ci` pass.
- Plan archived under [`docs/plans/archive/`](.). ✅

## Implementation checklist

| Step | Status |
|---|---|
| Spec / registry updated (`clinical-modeling.md § REQ-118`, REQ.md row, `traceability.yaml`, PROBE-087/088 scope) | done |
| Phase 1 — shared `TOP` vocabulary + parse carrier | done |
| Phase 2 — literal source text | done |
| Phase 3 — builder write side + channel exclusivity | done |
| Phase 4 — lint diagnosis (2 codes) | done |
| Phase 5 — corpus, goldens, count re-pins | done |
| `make spec-check` | done — OK |
| `make ci` | done — 0 lint issues, all tests pass |

## Phases

### Phase 1 — the `TOP` carrier (read side)

**Tasks:**

- Add the shared vocabulary in `openehr/aql/top.go`: `TopClause{N int, Dir TopDir}` and `TopDir` with an unspecified zero plus `TopForward` / `TopBackward` (+ `String()`), documented as deprecated-upstream with `LIMIT`/`ORDER BY` named as the replacement. It lives in `openehr/aql` because both sides consume it (the REQ-113 "one model, read and write" rule); `parse` re-exports the two names as aliases, mirroring `parse.PathSegment`.
- Carry it as `parse.SelectClause.Top *aql.TopClause` (nil = clause absent, so `TOP 0` stays distinguishable).
- Populate it in `extractSelectClause`, replacing the `ex.incomplete(…)` call: parse the `INTEGER` token, map `FORWARD`/`BACKWARD` from the `direction` label, and route an out-of-range count into the existing unrepresentable-numeric residual (the same treatment `LIMIT` overflow already gets) rather than a `top`-specific gap.
- Emit in `(*Query).Emit` between `DISTINCT` and the projection list: `TOP <n>[ FORWARD| BACKWARD]`.

**Verification:** `TestParseQueryTopClause` (bare / `FORWARD` / `BACKWARD` / with `DISTINCT` / with `*` / alongside `LIMIT … OFFSET …`), `TestParseQueryTopClauseOutOfRange` (residual still fires, wrapping `aql.ErrIncompleteAST`), round-trip idempotence for each shape. `go test ./openehr/aql/...`.

**Definition of done:** no `ParseQuery` input carrying a well-formed `top` returns `ErrIncompleteAST`; every `top` shape is a round-trip fixed point; the retired `TestParseQuerySurfacesTopClauseGap` is *replaced* by the positive pins, not deleted silently.

### Phase 2 — literal source text

**Tasks:**

- Add `Raw string` to `parse.LiteralExpr`, documented as read-side fidelity only (empty on a constructed AST; emission stays canonical).
- Populate it from the source token span for a literal in a `SELECT` projection and for a literal in a function-call argument.
- Leave `emitSelectExpr` rendering through `aql.FormatValue` — unchanged bytes for every existing query.

**Verification:** `TestParseQueryLiteralSourceText` pinning the divergent cases (`1.50` → value `1.5`, raw `1.50`; `001` → `1` / `001`; `"dq"` → `'dq'` / `"dq"`; plus `'sq'`, `true`, `NULL`, `-0`), one nested-argument case, and one assertion that emission is still canonical.

**Definition of done:** the source text is available for every literal the extractor delivers, and no emitted byte changed.

### Phase 3 — builder write side and channel exclusivity

**Tasks:**

- `Builder.Top(n int)` and `Builder.TopDirected(n int, dir TopDir)`, chainable, doc-commented with the upstream deprecation and the `LimitInline` pointer.
- Emit in `ast.build()` after `SELECT `/`DISTINCT `, before the projection list.
- Extend `validatePaging`: a `top` joins the in-text row-limit channel, so `TOP` + in-text `LIMIT`/`OFFSET` and `TOP` + envelope `Limit`/`Offset` both fail with `ErrInvalidQuery`; a negative count fails the same way.

**Verification:** goldens under `openehr/aql/testdata/wire/` for the plain and directed forms; refusal matrix in `openehr/aql/paging_test.go`; `TestProbe088GoldensRoundTripThroughParse` ties each new golden back to Phase 1's parser.

**Definition of done:** a builder program that touches none of the new API is byte-identical (the PROBE-020 golden and its drift detector are untouched).

### Phase 4 — lint diagnosis

**Tasks:**

- Add the two Layer-2 codes whose canonical catalogue home is the REQ-109 table: `aql_deprecated_top` (Warning, once per query carrying a `TOP`) and `aql_top_with_limit` (Error, `TOP` together with a `LIMIT` clause).
- Both read the parsed structure — no prose keying, no second parse.

**Verification:** `TestLintDeprecatedTop`, `TestLintTopWithLimit` (both codes present, `Result.OK()` false only for the Error), `TestLintNoTopClauseRaisesNeitherCode` (the negative control, including a `LIMIT`-only query), and `TestLintTopWithUnrepresentableCount` (presence vs representability — see below). PROBE-028's cassette multiset is unchanged, and already pinned by `TestProbe028AQLLint`, which asserts exact per-cassette code multisets (`valid.aql` → none) — a spuriously firing code fails it.

**Definition of done:** a query the openEHR spec forbids is reported by the SDK's own gate, at Error severity, without the emitter or the parser having to refuse it.

### Phase 5 — corpus, goldens, and count re-pins

**Tasks:**

- Grow the round-trip corpus (idempotence + canonical-preservation) with the `top` shapes and the literal-source-text cases.
- Re-pin every stated case count that moved: PROBE-080 preconditions, the REQ-113 verification bullet, PROBE-087's status line.
- Re-read the REQ-117 §, PROBE-087 and PROBE-080 prose for any sentence that still reads as though `TOP` were a residual.

**Definition of done:** `make spec-check` clean, `make ci` green, and no doc sentence contradicts the shipped residual list.

## Settled while implementing

Both were open questions the ask left underdetermined; neither needed an ADR because the grammar profile and the already-normative canonical write form decide them.

- **`TOP $n` is not modelled.** The profile's `top : TOP INTEGER direction=(FORWARD|BACKWARD)?` admits no `PARAMETER`, so reusing the `LimitExpr` vocabulary (which carries `ParamLimit`) would have created an AST shape whose emission the parser rejects. A dedicated `aql.TopClause{N, Dir}` carries what the grammar actually admits — and the direction, which `LimitExpr` cannot hold at all. The `SDK-AQL-003` parameter relaxation is deliberately **not** extended to a construct the spec is removing.
- **A canonical renderer for a constructed value already exists.** `aql.FormatValue` is public, so the "export the renderer" alternative needed no work; only the *as-written* half was missing, which is what `LiteralExpr.Raw` now carries. The source span is read from the character stream rather than `GetText()`, because `GetText()` drops interior whitespace (`- 5` → `-5`) and a canonicalisation is exactly what this field exists to avoid.

- **Lint keys on clause PRESENCE, not on the decoded bound.** Caught in review: an out-of-range `TOP` count leaves the decoded carrier nil (nothing is truncated into a bound), so a check keyed on it went silent for exactly the query that pairs a deprecated clause with an unusable count — and with a `LIMIT`. `parse.Document` now separates `HasTop` (read from the tree) from `Top` (the decoded bound), and REQ-118 states the rule normatively.

## Mapping to specs

- [docs/specifications/clinical-modeling.md § REQ-118](../../specifications/clinical-modeling.md#req-118--deprecated-select-top-clause-and-literal-source-text) — normative contract (carriers, prohibited combination, write side, amended residual list)
- [docs/specifications/clinical-modeling.md § REQ-109 Layer 2](../../specifications/clinical-modeling.md#layer-2--shape-ast-walk-no-cdr) — canonical home of the two new lint codes
- [docs/specifications/conformance.md § PROBE-087](../../specifications/conformance.md#probe-087--aql-structured-ast-catalogue-completeness) / [§ PROBE-088](../../specifications/conformance.md#probe-088--aql-builder-containment-and-paging-stability) — acceptance
- [docs/specifications/REQ.md](../../specifications/REQ.md) — registry row
- [resources/aql/grammar/DIVERGENCES.md](../../../resources/aql/grammar/DIVERGENCES.md) — unchanged; this plan adds no grammar delta
