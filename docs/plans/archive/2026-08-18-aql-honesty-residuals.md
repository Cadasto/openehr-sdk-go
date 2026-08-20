# Plan — AQL honesty residuals (lint, diagnostics, residual test, profile discoverability)

**Date:** 2026-08-18
**Status:** Landed
**Owner:** SDK maintainers
**Covers:** [REQ-109](../../specifications/clinical-modeling.md#req-109--aql-static-lint) (Layer 3 literal-HRID only; optional Layer-2 `aql_select_star` Warning), [REQ-119](../../specifications/clinical-modeling.md#req-119--re-parseable-canonical-aql-emission) (MATCHES URI diagnostic redaction), [REQ-117](../../specifications/clinical-modeling.md#req-117--aql-expression-catalogue-completion) (REAL overflow residual), [REQ-055](../../specifications/wire.md#req-055--wire-boundary) (parameter keys carry no leading `$`)
**Probes:** [PROBE-028](../../specifications/conformance.md#probe-028--aql-lint-stability) (must stay exact on existing cassettes), [PROBE-087](../../specifications/conformance.md#probe-087--aql-structured-ast-catalogue-completeness) (residual suite), [PROBE-090](../../specifications/conformance.md#probe-090--aql-emission-round-trip-closure) (URI refuse path)
**Implementation:** landed
**Depends on:** REQ-055 / 109 / 117 / 119 landed (v0.20.0)
**Defers:** PATH splice on `Builder.Build` (REQ-119 § Out of scope / REQ-055 rule 3); FROM-root builder junction (REQ-117); full `nodePredicate` validator; PROBE-021 Cassette/Live; PROBE-078 / 079; URI-beyond-lex / `WhereExpr` equality

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. **Mode:** implementation-aligned — spec sentences in Phase 0 land in the **same PR** as the code.

**Goal:** Close the four honesty holes the 2026-08-18 AQL status review found against already-landed MUST clauses, and make the SDK grammar profile discoverable from the public `aql` / `lint` packages.

**Architecture:** No new package, no grammar change, no new REQ. Each phase is a named test that fails if the existing guard is missing, plus a one-line (or one-row) spec amendment so the MUST is greppable. `lint.Result.OK()` stays Warning-blind. PROBE-028's cassette multisets stay byte-identical except if Phase 5 adds a *new* cassette (never rewrite `valid.aql`).

**Tech Stack:** stdlib `testing`, existing `openehr/aql` / `openehr/aql/lint` / `openehr/aql/parse` surfaces. No new dependencies.

## Global Constraints

- Go floor `1.26.0` (REQ-002). Building-block independence (REQ-013): `openehr/aql` and `lint` MUST NOT import `transport/`, `auth/`, `openehr/client/*`, or `parse/gen`.
- Wrap errors with `%w`; sentinels stay `aql.ErrInvalidQuery` / `aql.ErrIncompleteAST`. No panics, no reflection.
- Lint codes are stable identifiers (REQ-109). A new code is Warning-only and MUST NOT flip `OK()`.
- Cite `REQ-NNN` / `PROBE-NNN` in the new tests. Do not restated normative prose in the commit / PR body — cite the SPEC §.
- Verification: `go test ./openehr/aql/... ./openehr/aql/lint/... ./openehr/aql/parse/... ./openehr/validation/ ./testkit/probes/aql/` then `make spec-check` and `make ci`.

## Definition of Ready

Implementation may start when:

- [x] `**Covers:**` lists every REQ this plan implements.
- [x] Canonical prose already exists for each covered REQ (hardening amendments are Phase 0, same PR).
- [x] No new ADR — no irreversible fork.
- [x] Negative space named below (cite, don't restate).
- [x] Verification commands named.

## Out of scope

- Guarding `Col` / `Eq` / `Exists` / `OrderBy` path text against splice (REQ-055 rule 3; caller data uses `aql.Param`).
- A builder entry for `FROM A OR B`.
- Cassette/Live ratification of PROBE-021, 078, 079.
- Changing SDK-AQL-001/002 themselves (no grammar edit). Phase 5 only *documents* them and optionally *warns* on `SELECT *`.
- An `aql_select_star` **Error** — that would break the documented SDK-AQL-002 relaxation.

## Definition of Done

All of the below land in the **same PR**:

- [x] Phases 0–4 complete (Phase 5 MAY ship in the same PR; it is not a blocker).
- [x] Negative space tested: `$arch` + compiled OPT does not Error; URI refuse path does not echo the URI; `Bind("$n")` keys as `n`; `1e400` surfaces `ErrIncompleteAST`.
- [x] Spec amendments from Phase 0 present in the topic files (no second copy in REQ.md).
- [x] `traceability.yaml` lists this plan on REQ-055 / 109 / 117 / 119 (and the new tests).
- [x] `make spec-check` and `make ci` pass.
- [x] Plan flipped to `landed` and `git mv`'d into `docs/plans/archive/`.

## Files

| File | Role |
|---|---|
| `docs/specifications/clinical-modeling.md` | Phase 0: one Layer-3 sentence; optional Layer-2 row; REQ-119 URI-diagnostic sentence |
| `docs/specifications/traceability.yaml` | Plan + new test paths |
| `openehr/aql/lint/extract.go` | Skip `ParamArchetype` when collecting HRIDs |
| `openehr/aql/lint/extract_test.go` | Extract-level pin |
| `openehr/aql/lint/lint.go` | Optional `aql_select_star` (Phase 5) |
| `openehr/aql/lint/lint_test.go` | Layer-3 `$arch` + optional star Warning |
| `openehr/aql/where.go` | `validateURIOperand` diagnostic omits `uri` |
| `openehr/aql/value_test.go` | URI error must not contain the operand |
| `openehr/aql/builder.go` | `Bind` strips leading `$` |
| `openehr/aql/builder_test.go` | `Bind("$n")` key is `n` |
| `openehr/aql/parse/roundtrip_test.go` | REAL overflow residual row |
| `openehr/aql/doc.go`, `openehr/aql/lint/extract.go` package comment | Phase 5 godoc |
| `docs/plans/README.md` | Active → archive on close-out |

---

### Task 1: Spec amendments (implementation-aligned)

**Files:** Modify `docs/specifications/clinical-modeling.md` only. No RFC-2119 in this plan file beyond the citations.

- [ ] **Step 1: Add the three sentences to the canonical §§**

In [§ REQ-109 Layer 3](../../specifications/clinical-modeling.md#req-109--aql-static-lint), after the `aql_archetype_not_in_template` row, add one sentence (do not rewrite the table):

> A `$param` archetype predicate (`[$name]`, `[parse.ClassExpr.ParamArchetype]`) is not a literal HRID and **MUST NOT** raise `aql_archetype_not_in_template`.

In [§ REQ-119 Canonical value spellings — `MATCHES {uri}`](../../specifications/clinical-modeling.md#req-119--re-parseable-canonical-aql-emission), after the existing URI-token paragraph, add:

> A refusal of this operand **MUST** name the path and the structural reason and **MUST NOT** reproduce the URI text — the same diagnostic contract [§ The class predicate positions](../../specifications/clinical-modeling.md#req-119--re-parseable-canonical-aql-emission) already requires of a refused class predicate.

If Phase 5 ships in this PR, add one Layer-2 table row (Warning, does not flip `OK()`):

| Check | Code | Severity | Rule |
|---|---|---|---|
| Bare / mixed `SELECT *` | `aql_select_star` | Warning | The projection uses the SDK-AQL-002 relaxation; official QUERY 1.1.0 has no `SELECT *` (except `COUNT(*)`). |

`COUNT(*)` **MUST NOT** raise this code (`parse.Document.Star` is the signal; it is not set for `COUNT(*)`).

- [ ] **Step 2: Do not touch REQ.md Impl. column** — the REQs stay `landed`. This plan hardens them.

---

### Task 2: Lint does not treat `$arch` as a missing HRID (REQ-109)

**Files:**
- Modify: `openehr/aql/lint/extract.go:53-56`
- Test: `openehr/aql/lint/extract_test.go`, `openehr/aql/lint/lint_test.go`

`Metadata.Archetypes` is already documented as literal HRIDs only (`extract.go:24-26`). `Extract` ignores `ParamArchetype`. Layer 3 then Errors `aql_archetype_not_in_template` for `COMPOSITION c[$arch]`. `TestLintIdentifiableScopeSuppressesWarning` covers Layer 2 only (no `Compiled`).

- [ ] **Step 1: Write the failing tests**

Add to `extract_test.go`:

```go
// REQ-109: a $param archetype predicate is not a literal HRID.
func TestExtractSkipsParamArchetype(t *testing.T) {
	md := lint.Extract(mustParse(t, "SELECT c FROM COMPOSITION c[$arch]"))
	if len(md.Archetypes) != 0 {
		t.Fatalf("Archetypes = %v, want none (ParamArchetype)", md.Archetypes)
	}
}
```

Add to `lint_test.go` next to `TestLintArchetypeNotInTemplate`:

```go
// REQ-109 Layer 3: [$arch] is bind-time scope, not a missing HRID.
func TestLintParamArchetypeNotInTemplateClean(t *testing.T) {
	c := mustCompile(t, "vital_signs")
	r := lint.LintString(
		"SELECT c FROM COMPOSITION c[$arch]",
		&lint.Options{Compiled: c},
	)
	if has(r, "aql_archetype_not_in_template") {
		t.Fatalf("ParamArchetype must not raise aql_archetype_not_in_template, got %v", codes(r))
	}
}
```

- [ ] **Step 2: Run them — expect FAIL**

```bash
go test ./openehr/aql/lint/ -run 'TestExtractSkipsParamArchetype|TestLintParamArchetypeNotInTemplateClean' -count=1
```

Expected: `Extract` reports `Archetypes: [$arch]`; Layer 3 emits `aql_archetype_not_in_template`.

- [ ] **Step 3: Minimal fix**

In `extract.go`:

```go
if ce.Archetype != "" && !ce.ParamArchetype && !seen[ce.Archetype] {
    seen[ce.Archetype] = true
    md.Archetypes = append(md.Archetypes, ce.Archetype)
}
```

- [ ] **Step 4: Re-run — expect PASS.** Also re-run `TestLintArchetypeNotInTemplate` and `TestLintCollectAll` (literal missing HRID still Errors).

```bash
go test ./openehr/aql/lint/ ./testkit/probes/aql/ -count=1
```

Expected: PASS. PROBE-028 cassette multisets unchanged.

- [ ] **Step 5: Commit** — `fix(aql/lint): skip $param archetypes in Layer 3 HRID check (REQ-109)`

---

### Task 3: MATCHES URI refusal does not echo the operand (REQ-119)

**Files:**
- Modify: `openehr/aql/where.go:685-687`
- Test: `openehr/aql/value_test.go` (`TestMatchesURIRefusesUnspellableOperand`)

`validateURIOperand`'s `bad` interpolates `%q` of `uri`. A `MATCHES {ehr://…/patient-id}` refusal is the return value of `Build` / `FormatWhere` / `Emit`. REQ-119 already forbids echoing class-predicate values on that path.

- [ ] **Step 1: Extend the existing refuse table**

In `TestMatchesURIRefusesUnspellableOperand`, after the `errors.Is(err, aql.ErrInvalidQuery)` check, add:

```go
if strings.Contains(err.Error(), uri) {
    t.Errorf("URI refuse diagnostic echoed the operand %q: %v", uri, err)
}
```

Keep naming the path (`c/x`) and a structural reason (`no scheme`, `scheme`, `authority`, …). The `"%v"` `detail` argument may still carry a *character* or *component name*, not the URI.

- [ ] **Step 2: Run — expect FAIL** on every table row (the current format ends with `: %q` of `uri`).

```bash
go test ./openehr/aql/ -run TestMatchesURIRefusesUnspellableOperand -count=1
```

- [ ] **Step 3: Change `bad` to omit `uri`**

```go
bad := func(what string, detail any) error {
    return fmt.Errorf("%w: MATCHES on %q carries a URI %s (%v)",
        ErrInvalidQuery, path, what, detail)
}
```

Do **not** call `RedactPredicateValues` here — this operand is a URI, not a class bracket. Omitting the text is the contract.

- [ ] **Step 4: Re-run the refuse table and the accept table** (`TestMatchesURIAcceptsGrammarSpellableOperands`). Accept path unchanged.

```bash
go test ./openehr/aql/ ./openehr/aql/parse/ -run 'URI|MatchesURI' -count=1
```

- [ ] **Step 5: Commit** — `fix(aql): omit MATCHES URI operand from refuse diagnostics (REQ-119)`

---

### Task 4: `Bind` strips a leading `$` (REQ-055 rule 4)

**Files:**
- Modify: `openehr/aql/builder.go:195-201`
- Test: `openehr/aql/builder_test.go`

`Param`, `LimitInlineParam`, and `OffsetInlineParam` already `TrimPrefix(name, "$")`. `Bind` stores the key as given. `LimitInlineParam("$n").Bind("$n", 10)` emits `LIMIT $n` and binds key `$n`, so lint reports unbound `n` and unused `$n`.

- [ ] **Step 1: Write the failing test**

```go
// REQ-055 rule 4: placeholder keys carry no leading $.
func TestBindStripsLeadingDollar(t *testing.T) {
    q, err := aql.NewBuilder().
        Select(aql.Col("e")).
        FromEHR("e", aql.Param("$id")).
        Bind("$id", "x").
        Build()
    if err != nil {
        t.Fatal(err)
    }
    if _, ok := q.Parameters["id"]; !ok {
        t.Fatalf("Parameters keys = %v, want [id]", q.Parameters)
    }
    if _, ok := q.Parameters["$id"]; ok {
        t.Fatalf("Parameters still keyed with $: %v", q.Parameters)
    }
}
```

- [ ] **Step 2: Run — expect FAIL** (`Parameters["$id"]` set, `"id"` missing).

```bash
go test ./openehr/aql/ -run TestBindStripsLeadingDollar -count=1
```

- [ ] **Step 3: Minimal fix**

```go
func (b *Builder) Bind(name string, value any) *Builder {
    name = strings.TrimPrefix(name, "$")
    if b.ast.params == nil {
        b.ast.params = map[string]any{}
    }
    b.ast.params[name] = value
    return b
}
```

Update the `Bind` godoc: a leading `$` is stripped, as in `Param`.

- [ ] **Step 4: Re-run builder tests + a lint bind-round case if one exists.**

```bash
go test ./openehr/aql/ -run 'TestBind|TestBuilder' -count=1
```

- [ ] **Step 5: Commit** — `fix(aql): Bind strips a leading $ like Param (REQ-055)`

---

### Task 5: REAL overflow is a named residual (REQ-117 / PROBE-087)

**Files:**
- Modify: `openehr/aql/parse/roundtrip_test.go` (`TestParseQuerySurfacesIncompleteAST`)
- No production change unless the row unexpectedly parses (then the extractor's `ParseFloat` nil-path at `extract_query.go:1221-1224` is the fix — it already returns nil).

Residual 1 is "a numeric literal the value vocabulary cannot represent", including a REAL beyond `float64`. Integer / LIMIT / OFFSET / `TOP` overflow are pinned; `1e400` is not.

- [ ] **Step 1: Add the row**

```go
{"real_overflow", "SELECT e FROM EHR e WHERE e/x > 1e400", "out of range"},
{"sci_real_overflow", "SELECT 1e400 FROM EHR e", "out of range"},
```

If the current extractor error string does not contain `"out of range"`, match whatever `ErrIncompleteAST` already says for the integer rows — do not invent a second residual class. If `ParseQuery("… 1e400 …")` currently returns a `RealValue` of `+Inf`, that is a **spec violation** of residual 1 (refuse, never degrade): refuse in `extract_query.go` the same way an out-of-range INTEGER does, wrapping `aql.ErrIncompleteAST`.

- [ ] **Step 2: Run**

```bash
go test ./openehr/aql/parse/ -run TestParseQuerySurfacesIncompleteAST -count=1
```

Expected: FAIL until the row is honest (either the extractor already refuses and the message matches, or `+Inf` is refused).

- [ ] **Step 3: If needed, refuse `+Inf` / NaN at extract** — `math.IsInf` / `math.IsNaN` on the `ParseFloat` success path, return nil (same incomplete-AST funnel as a failed parse). REQ-119 already refuses Inf/NaN on the *write* side; this is the read-side twin of residual 1.

- [ ] **Step 4: Re-run the residual suite and PROBE-087 catalogue tests.**

```bash
go test ./openehr/aql/parse/ -run 'IncompleteAST|FormerCatalogue|ParseQuery' -count=1
```

- [ ] **Step 5: Commit** — `test(aql/parse): pin REAL overflow as ErrIncompleteAST residual (REQ-117, PROBE-087)`

---

### Task 6: Profile discoverability (optional in the same PR)

Not a blocker for Phases 1–4. Two small surfaces:

**5a. Godoc** — `openehr/aql/doc.go` lists `ErrInvalidQuery` / `ErrPathResolution` / `ErrSyntax` and omits `ErrIncompleteAST`. Neither `aql` nor `lint` mentions SDK-AQL-001 (`CONTAINS` vs `CONTAINS_STR`) or SDK-AQL-002 (`SELECT *`). Point at [`resources/aql/grammar/DIVERGENCES.md`](../../../resources/aql/grammar/DIVERGENCES.md) and ADR 0007. Do not copy the delta table into godoc.

**5b. `aql_select_star` Warning** — only if Phase 0 added the Layer-2 row.

- [ ] **Step 1 (5b): Failing test**

```go
func TestLintSelectStarWarns(t *testing.T) {
    r := lint.LintString("SELECT * FROM EHR e", nil)
    if !has(r, "aql_select_star") {
        t.Fatalf("want aql_select_star Warning, got %v", codes(r))
    }
    if !r.OK() {
        t.Fatalf("Warning must not flip OK: %v", codes(r))
    }
}

func TestLintCountStarDoesNotWarnSelectStar(t *testing.T) {
    r := lint.LintString("SELECT COUNT(*) FROM EHR e", nil)
    if has(r, "aql_select_star") {
        t.Fatalf("COUNT(*) must not raise aql_select_star, got %v", codes(r))
    }
}
```

Signal: `parse.Document.Star` (set for bare and mixed `SELECT *`; not for `COUNT(*)`).

- [ ] **Step 2: Run — expect FAIL** (no such code).

- [ ] **Step 3: In `shapeIssues`, after the identifiable-scope check**

```go
if doc.Star {
    issues = append(issues, Issue{
        Code:     "aql_select_star",
        Detail:   "SELECT * is an SDK grammar-profile relaxation (SDK-AQL-002); official QUERY 1.1.0 requires explicit columns",
        Severity: Warning,
    })
}
```

- [ ] **Step 4: PROBE-028** — `valid.aql` has no `SELECT *`, so its empty multiset stays. Do **not** add `aql_select_star` to an existing cassette. A new optional cassette is fine; it is not required.

```bash
go test ./openehr/aql/lint/ ./testkit/probes/aql/ -count=1
```

- [ ] **Step 5: Commit** — `docs(aql): surface the grammar profile; warn on SELECT * (REQ-109, SDK-AQL-002)`

---

### Task 7: Traceability hygiene (same PR)

- [x] Add this plan (then, on close-out, its `archive/` path) to `plans:` on REQ-055, REQ-109, REQ-117, REQ-119.
- [x] Add the new test files to those rows' `tests:` lists.
- [x] Also fold the review's map nits if touching those rows anyway:
  - `docs/plans/archive/2026-06-29-stored-query-rest-conformance.md` onto REQ-055 and REQ-057 `plans:`
  - Repair archived `Covers` hrefs `#req-055--aql-query` → `#req-055--wire-boundary` in `archive/2026-06-29-aql-execution-ast.md` and `archive/2026-06-29-stored-query-rest-conformance.md`
- [x] `make spec-check` — expect OK.
- [x] Archive this plan (`git mv` to `docs/plans/archive/`) and update `docs/plans/README.md`.

---

## Mapping to specs

- [clinical-modeling.md § REQ-109](../../specifications/clinical-modeling.md#req-109--aql-static-lint) — Layer 3 literal HRID; Layer 2 catalogue
- [clinical-modeling.md § REQ-117](../../specifications/clinical-modeling.md#req-117--aql-expression-catalogue-completion) — residual 1
- [clinical-modeling.md § REQ-119](../../specifications/clinical-modeling.md#req-119--re-parseable-canonical-aql-emission) — diagnostic contract
- [wire.md § REQ-055](../../specifications/wire.md#req-055--wire-boundary) — parameter keys
- [REQ.md](../../specifications/REQ.md) — registry (Impl. stays `landed`)
- [DIVERGENCES.md](../../../resources/aql/grammar/DIVERGENCES.md) · [ADR 0007](../../adr/0007-aql-antlr-grammar-profile.md)
