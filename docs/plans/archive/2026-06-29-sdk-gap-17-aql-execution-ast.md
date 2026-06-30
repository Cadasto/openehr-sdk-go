# Plan — SDK-GAP-17: execution-oriented parsed AQL AST

**Date:** 2026-06-29
**Status:** Draft
**Owner:** SDK maintainers
**Covers:** proposed **REQ-113** (structured AQL AST — read side) — extends [REQ-109](../specifications/clinical-modeling.md#req-109--aql-static-lint) (parse + lint) and the write-side [REQ-055](../specifications/wire.md#req-055--aql-query) (Builder / verb-functions / `WhereExpr` / `Value`). The two sides converge on one introspectable expression vocabulary.
**Probes:** proposed **PROBE-080** (round-trip — emitter byte-identical-equivalent property across the buildable grammar, mirroring the existing PROBE-020 emitter contract).
**Implementation:** planned — recommend a two-step roll-out (interim accessor, then the target AST). Maintainer chooses whether to land both in one cycle.
**Depends on:** REQ-109 parse infrastructure landed (`openehr/aql/parse/parse.go`, `parse/gen/`); REQ-055 Builder + `WhereExpr`/`Value` landed (`openehr/aql/`).
**Defers:** execution / planning semantics (the consumer's job); archetype/template path resolution against an OPT; semantic validation beyond the grammar (PROBE-021 already disclaims this).
**Inbound source:** [SDK-GAP-17 dossier](../../docs/sdk-gap-drafts/SDK-GAP-17.md) (filed against v0.11.0 by a consuming CDR project — building an AQL execution engine that today recurses the SDK's generated `parse/gen` ANTLR tree behind a single isolated seam; the dossier asks for a stable, generated-type-free read AST that mirrors the existing write-side Builder).

## Goal

Expose a **stable, generated-type-free, readable** parsed AQL AST — a `string → structured query` function whose result a consumer can traverse without importing `openehr/aql/parse/gen` or any `internal/` package. The AST preserves containment nesting, the WHERE expression tree (paths, operators, param-vs-literal values with type), SELECT function/aggregate wrappers and aliases, ORDER BY direction, and LIMIT/OFFSET values. The natural delivery is to make the existing write-side `WhereExpr`/`Value` vocabulary **introspectable** and reuse it on both build and parse — **one model, two directions**.

## Problem

Today the SDK exposes two surfaces — both incomplete for an execution consumer:

1. **`parse.Document` is lint-oriented and structure-erased.** Per its own contract, it flattens FROM/CONTAINS ("nesting is not retained because the lint contract reasons over the *set* of bound classes, not their containment shape") and exposes only the class *set* (`Classes`), the path *set* (`Paths`), the param *set* (`Params`), and presence/shape flags (`HasWhere`, `HasOrderBy`, `HasLimit`, `Distinct`, `Star`, `NumSelect`). It does **not** expose: which class CONTAINS which; the WHERE comparison/junction tree (only the paths in WHERE survive — operators and values are dropped); the function/aggregate wrapper around a SELECT item; ORDER BY direction; or the LIMIT/OFFSET values. The parsed ANTLR tree is an unexported field ([`parse.go:44` `tree gen.ISelectQueryContext`](../../openehr/aql/parse/parse.go#L44)).

2. **`Builder` / `WhereExpr` / `Value` is write-only.** `WhereExpr` and `Value` are sealed interfaces whose only methods (`expr()`, `token()`) **emit** text; concrete types (`comparison{path,op,val}`, `junction`, `paramValue`, `stringValue`, …) are unexported with no readable fields. You can build and emit a string; you cannot read what you parsed.

An execution engine — the read-side use-case — has no choice but to descend the generated `parse/gen` typed-context tree, coupling the consumer to ANTLR codegen that is not a stable consumer contract.

## Definition of Ready (analysis gate)

Implementation may start when:

- [x] Maintainer sign-off on the **roll-out shape** — **both in one cycle** (interim `Document.Tree()` accessor + target `parse.Query` AST) chosen 2026-06-29.
- [x] Maintainer sign-off on the **vocabulary unification** — **unify on the same concrete types**: export the existing concrete `WhereExpr`/`Value` types with read accessors; Parse populates them; Builder constructs them. One model, two directions. Chosen 2026-06-29.
- [x] Maintainer sign-off on the **placement** — **`parse.Query`** in the parser package (mirror of `Parse → Document`); the unified vocabulary stays in `aql`. Chosen 2026-06-29.
- [ ] **Covers:** finalized — promote REQ-113 prose under `clinical-modeling.md`, register the row in [`REQ.md`](../specifications/REQ.md) at status `Draft` alongside the implementation in Phase 3.
- [ ] PROBE-080 round-trip corpus seeded — at least: a SELECT with COUNT and AS alias; a nested CONTAINS chain; a WHERE with `AND`/`OR`/`NOT`/`EXISTS`/`MATCHES`/`LIKE`; an ORDER BY with mixed directions; a LIMIT+OFFSET.

## Accepted approach (2026-06-29)

Two tiers landed on the same branch: the interim ships value immediately while the target settles. Vocabulary unifies on the existing concrete write-side types (one model, two directions); the new structured AST type lives in `parse` as `parse.Query`.

### Tier 1 — interim: `Document.Tree()` accessor (cheap, ~one day)

Add a public accessor on `parse.Document`:

```go
// Tree returns the validated ANTLR parse tree. The return type is from the
// generated parser package and is NOT a stable consumer contract — it may
// change across grammar regenerations. Use parse.Query (REQ-113) once
// available.
func (d *Document) Tree() gen.ISelectQueryContext
```

Removes the *re-parse* cost for a consumer already recursing `gen`. Does not solve the generated-coupling concern but is explicit about the staleness risk in its doc comment. This unblocks consumers immediately.

### Tier 2 — target: `parse.Query` AST + introspectable expression vocabulary

A new public type (working name `parse.Query`; promoted to `aql.Statement` if the unification across packages is preferred — maintainer call) carrying:

- **`Select`** — ordered projection items. Each: `{ path: IdentifiedPath; wrapper: *FunctionCall (optional — `COUNT`, `MAX`, …); alias: string (optional) }`. Plus `Distinct bool` and `Star bool`.
- **`From`** — a **nested** containment tree. Each node: `{ kind: ClassExprKind | VersionedExpr; alias: string; archetypeOrVersionPredicate: …; standingPredicates: []WhereExpr (e.g. `ehr_id/value = $x`); children: []ContainsChild; childJoin: ContainsOp (AND/OR/NOT) }`.
- **`Where`** — a boolean expression tree of **readable** `Comparison{Path, Op, Value}` and `Junction{Op, Operands}` / `Not{Operand}` / `Exists{Path}` / `Matches{Path, …}` / `Like{Path, Pattern}`. Each `Value` discriminates **param vs literal** and carries its **type** (string / int / real / bool / temporal / …).
- **`OrderBy`** — ordered terms `{ path: IdentifiedPath; dir: OrderDir (Asc | Desc) }`.
- **`Limit`** — `*int` (nil when absent in the AQL text).
- **`Offset`** — `*int` (nil when absent).

**Vocabulary unification (lean).** Recommend making the existing `WhereExpr` / `Value` interfaces introspectable on the **same** concrete types, by:

1. Exporting the concrete types (`Comparison`, `Junction`, `ParamValue`, `StringValue`, `IntValue`, …) with their existing emit methods *plus* small read accessors (`Path() IdentifiedPath`, `Op() Operator`, `Value() Value`). The `expr()`/`token()` sealed-interface markers stay package-local — the interfaces remain sealed (so the closed-world dispatch is preserved) but the concrete types become readable.
2. Parse populates these same concrete types. Builder continues to construct them. One model, two directions; one emitter contract; one round-trip property (PROBE-080).

If unification is rejected, the alternative is a parallel set of exported types in `parse` — works but doubles the catalogue and forces the planner to choose. Defer to maintainer.

### `string → AST` entry point

```go
// ParseQuery validates q against the SDK grammar profile (REQ-109) and
// returns a structured, generated-type-free AST. On a syntax error it
// returns a *SyntaxError (wrapping aql.ErrSyntax). The returned *Query is
// owned by the caller and may be traversed but MUST NOT be mutated.
func ParseQuery(q string) (*Query, error)
```

Sits alongside `Parse` (which returns `Document` for the lint contract). Both reuse the same ANTLR parse pass internally — parse once, populate both shapes — so a consumer paying for parse gets both `Document` and `Query` if they want them. (Concrete plumbing: refactor `parse.Parse` so the populated tree is the source for both; expose `Query` via a method on `Document` or a sibling top-level entry, whichever reads cleaner.)

## Phases

### Phase 1 — analysis & sign-off (this plan)

**Tasks:**
- Record maintainer sign-off on roll-out (interim only / target only / both) and on vocabulary unification (unify / parallel types).
- Finalise REQ-113 prose under `clinical-modeling.md`; register the row in `REQ.md` at status `Draft`.
- Seed PROBE-080 corpus under `testkit/cassettes/aql/structured/` — input AQL strings + expected AST shape (golden files; field-by-field assertion in the probe).

**Definition of done:** sign-off recorded; cassette directory in place; this plan flipped Draft → Ready.

### Phase 2 — Tier 1 (interim `Tree()` accessor)

**Tasks:**
- Add `Document.Tree()` exposing the existing `tree` field; document the staleness risk with the leaning toward `parse.Query` once landed.
- Add one unit test asserting the accessor returns the validated tree and that traversing it surfaces the SELECT/FROM/WHERE/ORDER-BY/LIMIT structure (a small smoke test, not the full corpus — the round-trip property is owned by Tier 2).

**Definition of done:** `make ci` green; one-line CHANGELOG bullet under `[Unreleased]` ("Interim accessor for the generated parse tree — superseded by `parse.Query` in v0.x.y").

### Phase 3 — Tier 2 (target `parse.Query` AST)

**Tasks:**
- Export the concrete `WhereExpr`/`Value` types (or land parallel ones in `parse`, per Phase-1 sign-off). Add the read accessors.
- Define `parse.Query`, the SELECT/FROM/WHERE/ORDER-BY/LIMIT/OFFSET shapes, and `ParseQuery(q string) (*Query, error)`.
- Refactor `parse.Parse` so the parse pass populates both `Document` (lint) and `Query` (structured) from the same ANTLR walk — no double parse.
- Land **PROBE-080** at `testkit/probes/aql/`: for each cassette, parse → traverse → re-emit via the existing Builder/emitter → assert byte-identical-equivalent against a canonical form of the input.
- Add a worked example: `cmd/examples/aql-parse-structured/` — read a query, print its containment tree and WHERE structure, re-emit. Mirrors `cmd/examples/template-explore`.

**Definition of done:** `make ci` green; PROBE-080 green; example runs; `traceability.yaml` updated (REQ-113 → `openehr/aql/parse` + `openehr/aql` + PROBE-080).

### Phase 4 — supersession close-out

**Tasks:**
- Mark `Document.Tree()` as superseded in its doc comment (still callable; planned for removal at the first major).
- Refresh `openehr/aql/parse/doc.go` and `openehr/aql/doc.go` with the unified one-model-two-directions narrative.
- Flip REQ-113 row in `REQ.md` from `Draft` → `Stable` once probes pass.

**Definition of done:** doc updates landed; CHANGELOG bullet finalised; plan archived.

## Acceptance criteria

- `ParseQuery(q string) (*Query, error)` returns a generated-type-free structure; consumer code traverses it without importing `parse/gen` or any `internal/` package.
- The AST preserves **containment nesting** (CONTAINS tree, not a flat set) and the **WHERE operator/value structure** (path + operator + param-vs-literal value with type).
- SELECT items expose the **function/aggregate wrapper** and AS alias distinctly from the bare path.
- ORDER BY direction and LIMIT/OFFSET values are readable when present.
- **Round-trip property (PROBE-080):** for the buildable grammar, emitting the parsed AST via the existing `Builder`/emitter yields canonical AQL equivalent to the input (mirrors PROBE-020).
- A syntactically invalid query returns the existing `*SyntaxError` (wrapping `aql.ErrSyntax`) — no behavioural change to error reporting.
- `make ci` and `make spec-check` green; `traceability.yaml` lists REQ-113 and PROBE-080.

## Out of scope

- **Execution / planning semantics** — lowering an AST to storage, query planning, RESULT_SET materialisation. Consumer-side concern; SDK ships the readable model only.
- **Archetype / template path resolution** — resolving an identified path against an OPT/archetype to a concrete RM type is a separate concern (likely a future REQ); the AST carries the path *as written*.
- **Semantic validation beyond the grammar** — PROBE-021 already disclaims this (the server remains the execute-time semantic authority).

## Risks / open questions

- **Sealed-interface preservation.** Exporting the concrete `WhereExpr`/`Value` types while keeping the interfaces sealed is the desired closed-world dispatch. Confirm at sign-off that the `expr()`/`token()` markers stay package-local even as the concrete types' fields/accessors become public.
- **Builder API stability.** Existing Builder callers should keep working unchanged after the unification. Run the existing `openehr/aql` test corpus before merging Phase 3.
- **`Document` vs `Query` co-existence.** They share the parse pass but expose different shapes. Recommend: keep `Document` as the lint contract (it's stable and consumers exist); add `Query` as the read AST. A future cycle could merge them, but not in this plan.
- **`parse` vs `aql` package placement.** `parse.Query` keeps the read-side in the parser package (mirror of `Parse`). Promoting to `aql.Statement` would unify naming with the Builder — but `aql` already exports the construction model; splitting the read AST off keeps imports narrower. Lean **`parse.Query`**; final call at sign-off.

## Mapping to specs

- [docs/specifications/clinical-modeling.md § REQ-113 (to be added)](../specifications/clinical-modeling.md) — normative contract.
- [docs/specifications/REQ.md](../specifications/REQ.md) — registry row.
- [docs/specifications/traceability.yaml](../specifications/traceability.yaml) — REQ-113 → `openehr/aql/parse` + `openehr/aql` + PROBE-080.
