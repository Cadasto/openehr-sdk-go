# Plan — AQL struct-builder and verb-function builders

**Date:** 2026-05-21
**Status:** Draft
**Owner:** SDK maintainers
**Covers:** REQ-055 (builders complete wire contract); PROBE-020, PROBE-021
**Implementation:** partial — `Query`, `ResultSet`, `NewQuery`, `Validate` **landed**; builders **planned**
**Depends on:** [`2026-05-15-rest-api-client.md`](2026-05-15-rest-api-client.md) Phase 5 (`openehr/client/query/`); umbrella [`2026-05-21-phase-2-clinical-building-blocks.md`](2026-05-21-phase-2-clinical-building-blocks.md)
**Defers:** Full AQL parser / pretty-printer; stored-query builder (use `definition` client + execute by id); query optimiser

## Goal

Complete **REQ-055** by implementing both builder styles in `openehr/aql/`:

- **Struct-builder** — compose typed `Select`, `From`, `Where`, … into an `aql.Query`.
- **Verb-functions** — `aql.Select(...)`, `aql.From(...)`, chained fluently.

Both **MUST** produce **byte-identical** `Query.Q` strings for the same logical query (PROBE-020). Canonicalisation rules live in [`specs/wire.md` § REQ-055](../../specs/wire.md#req-055--wire-boundary).

Execution stays in **`openehr/client/query/`** — this plan does not change the executor.

## Integration with existing stack

| Piece | Location | Role |
|---|---|---|
| Wire models | `openehr/aql/query.go`, `result.go`, `errors.go` | `Query`, parameters, result decoding |
| Executor | `openehr/client/query/` | POST AQL; maps `ErrPathResolution` (PROBE-021) |
| Template (optional) | `openehr/template/` | Path/archetype id hints for validation package only |
| Validation | `openehr/validation/` | Optional static lint — not required for builder MVP |

## Canonicalisation rules (implement in Phase 0)

Pin in `specs/wire.md` amendment (same PR as builder Phase 1) so goldens are stable:

1. **Keywords** — uppercase: `SELECT`, `FROM`, `WHERE`, `ORDER BY`, `OFFSET`, `LIMIT`, `CONTAINS`, `AND`, `OR`.
2. **Whitespace** — single space between tokens; no leading/trailing space on `Query.Q`.
3. **Paths** — as authored (no case folding on archetype ids).
4. **Parameters** — placeholders `$name` in `Q`; values only in `Query.Parameters` (never interpolated into string — prevents injection; REQ-055 security note).
5. **SELECT list** — comma-separated, single space after comma.
6. **FROM** — `EHR[ehr_id/value=$ehr_id]` style; document EHR vs composition containment patterns in plan examples only (normative minimum in spec).

Breaking change policy: changing canonicalisation requires updating **all** wire goldens and is semver-major for `aql` package.

## Out of scope

- Parsing arbitrary AQL strings into builder AST (consumers keep using `NewQuery(literal)`).
- SQL-style query builder for non-openEHR dialects.
- Automatic template-aware path validation (validation plan Phase 2).

## Phases

### Phase 0 — Goldens and canonicalisation spec

**Outcome:** Wire-output cassettes and amended REQ-055 canonicalisation bullets.

**Tasks:**

1. **Amend [`specs/wire.md`](../../specs/wire.md) REQ-055** — add canonicalisation subsection (six rules above).
2. **Cassettes** — `testkit/cassettes/aql/` or `openehr/aql/testdata/wire/`:
   - Reference query: "all OBSERVATIONs of archetype X for EHR" (from PROBE-020 preconditions).
   - Expected `Q` string golden file.
3. **Traceability** — mark `openehr/aql/` builders as `planned`; executor remains `landed`.

**Definition of done:** Goldens committed; spec-check passes.

### Phase 1 — Struct-builder MVP

**Outcome:** Typed structs serialize to golden `Q`; `Query.Validate()` still only checks non-empty (syntax errors impossible by construction for supported subset).

**Tasks:**

1. **Types** — `SelectClause`, `FromClause`, `WhereExpr`, `OrderBy`, `Limit`, `Offset` (embed into builder, emit `Query`).
2. **`Builder` struct** — `func NewBuilder() *Builder`; methods `Select`, `FromEHR`, `FromComposition`, `Where`, `OrderBy`, `Limit`, `Offset`, `Param`.
3. **`func (b *Builder) Build() (Query, error)`** — assembles `Q` + `Parameters` + `EHRID`.
4. **`func (q Query) String()`** — already returns `Q`; ensure builder is sole author of `Q` for built queries.
5. **Tests** — `// PROBE-020` byte compare against golden for struct-builder.
6. **Package doc** — update `openehr/aql/doc.go` (remove "later phase" wording when landed).

**Definition of done:** PROBE-020 struct side green in sandbox.

### Phase 2 — Verb-functions + PROBE-020 parity

**Outcome:** Verb API produces identical `Q` to struct-builder.

**Tasks:**

1. **Verb API:**
   ```go
   func Select(fields ...SelectField) *VerbQuery
   func (v *VerbQuery) FromEHR(archetype string) *VerbQuery
   func (v *VerbQuery) Where(e WhereExpr) *VerbQuery
   func (v *VerbQuery) Build() (Query, error)
   ```
2. **Shared emitter** — unexported `wire.Emit(b ast) string` used by both styles (single canonicalisation implementation).
3. **PROBE-020** — `testkit/probes/aql/probe_020_builder_stability.go` compares both builders to golden.
4. **PROBE-021** — document: builder cannot emit syntax errors; integration test in `openehr/client/query/` with invalid path uses backend error → `errors.Is(err, aql.ErrPathResolution)` (may already exist — extend if needed).
5. **Example** — `cmd/examples/aql-build/main.go` prints `Q` for reference query both ways.
6. **REQ-055** — traceability: builders `landed`; update [`docs/roadmap.md`](../roadmap.md) AQL row.

**Definition of done:**

- PROBE-020 Implemented (Sandbox).
- `make ci` green.
- [`specs/conformance.md`](../../specs/conformance.md) table row AQL probes updated.

## Public API (target)

```go
// Struct style
b := aql.NewBuilder().
    Select(aql.Field{"o", "data"}).
    FromEHR("openEHR-EHR-ehr.contribution.v1").
    Where(aql.Eq("e/ehr_id/value", aql.Param("ehr_id")))
q, err := b.Build()

// Verb style
q2, err := aql.Select(aql.Field{"o", "data"}).
    FromEHR("openEHR-EHR-ehr.contribution.v1").
    Where(aql.Eq("e/ehr_id/value", aql.Param("ehr_id"))).
    Build()

// PROBE-020: q.String() == q2.String()
```

## Implementation checklist

| Step | Status |
|---|---|
| REQ-055 canonicalisation amend + goldens | |
| Struct-builder + tests | |
| Verb-functions + shared emitter | |
| PROBE-020 probe | |
| PROBE-021 integration note / test | |
| `make ci` | |

## Mapping to specs

- [`specs/wire.md` § REQ-055](../../specs/wire.md#req-055--wire-boundary)
- [`specs/conformance.md`](../../specs/conformance.md) — PROBE-020, PROBE-021
- [`openehr/aql/query.go`](../../openehr/aql/query.go) — current partial implementation
