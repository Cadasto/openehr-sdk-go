# Plan — AQL value-free structured diagnostics (drop channel + lint spans)

**Date:** 2026-08-18
**Status:** Draft
**Owner:** SDK maintainers
**Covers:** **[REQ-113](../specifications/clinical-modeling.md#req-113--execution-oriented-parsed-aql-ast) § Value-free structured drop records** and **[REQ-109](../specifications/clinical-modeling.md#req-109--aql-static-lint) § Value-free lint diagnostics** — the spec text is written; the code is not. No new requirement id: each half extends the landed requirement that already owns its surface.
**Verifies / builds on:** landed [REQ-113](../specifications/clinical-modeling.md#req-113--execution-oriented-parsed-aql-ast) (the incomplete-AST channel: `aql.ErrIncompleteAST`, `Document.QueryErr()`), [REQ-109](../specifications/clinical-modeling.md#req-109--aql-static-lint) (lint issue model), [REQ-119](../specifications/clinical-modeling.md#req-119--re-parseable-canonical-aql-emission) (structural naming of a refused predicate + diagnostic redaction — the one-position precedent this plan generalises), the archived [AQL honesty-residuals plan](archive/2026-08-18-aql-honesty-residuals.md) (the MATCHES URI redaction one-off)
**Probes:** **[PROBE-096](../specifications/conformance.md#probe-096--aql-value-free-structured-diagnostics)** (allocated, `Status: Draft`) — every drop and lint diagnostic carries a structured record whose fields contain no source text
**Implementation:** planned
**Depends on:** `openehr/aql/parse` (extractor `incomplete(...)` sites), `openehr/aql/lint`
**Defers:** changing any existing human-readable message (compatibility — texts stay as they are); redaction *policy* (what counts as a value is the caller's call; this plan only guarantees the structured fields carry none); structured diagnostics for the simplified-format codecs (same pattern, separate ask)

## Goal

Let a diagnostic be **acted on and named without republishing the query**. Two
surfaces, one contract:

1. **The drop channel.** When the extractor drops a construct
   (`aql.ErrIncompleteAST`), the diagnostic today is a formatted string that
   quotes the offending source text. Add a typed record beside it —
   construct kind, clause, source span — with **no source text in any field**.
2. **Lint issues.** `lint.Issue` carries `Code` (stable), `Path`, and free-text
   `Detail`. Add a source `Span`, and make it normative which fields are
   guaranteed value-free (`Code`, `Severity`, `Span`) versus which may embed
   query text (`Detail`, and `Path` — a path spelling can carry predicate
   values).

## Motivation

A disclosure-bound server — one whose refusal reasons may name structure (an
operator, a clause, a path shape) but must never echo a value from the query,
because an AQL literal can be a payload value no quoting masks — faces two
perverse consequences today:

- It must **discard the SDK's drop diagnostics wholesale** — keeping only the
  boolean fact that something was dropped — because the diagnostic text quotes
  the construct's source. The refusal its clients see is generic
  ("unstructured query shape") when the SDK knew precisely what and where. The
  one remaining drop class after REQ-117/118 — an integer literal beyond
  `int64` in `LIMIT` / `OFFSET` / `TOP` / a comparison — could be refused **by
  name and position** ("integer literal out of range in LIMIT") without echoing
  the value, if kind/clause/span arrived structured.
- A 400 gate that interpolates `lint.Issue` fields into client-facing problem
  details must run its own scrubber over them, because nothing in the lint
  contract says which fields are safe. The SDK has already fixed one instance
  of this class itself (the MATCHES URI diagnostic redaction in the archived
  honesty-residuals plan) — as a one-off. This plan turns that precedent into a
  contract instead of a whack-a-mole.

A diagnostics carrier is consumed the moment it lands, so the record must be
complete enough to refuse on (kind + clause), not merely to log.

## Architecture

The normative rules — the two-surface field contract, the drop-record properties, the closed-enum
and completeness discipline, the clause and span reuse, and the additive-only clause — live in the
[canonical section](../specifications/clinical-modeling.md#req-113--execution-oriented-parsed-aql-ast)
and are **not** restated here. What this section carries is the implementation shape and the three
Phase 0 findings that changed the draft sketch.

### 27 drop sites, one reachable class (Phase 0 finding)

The sweep the DoR demanded found **27** `incomplete(...)` call sites in
[`extract_query.go`](../../openehr/aql/parse/extract_query.go), not the one the Motivation below
implies. They split into two populations:

| Population | Shape | Reachable by a legal query? |
|---|---|---|
| **Reachable** | a numeric literal outside the value vocabulary — `SELECT TOP`, a primitive literal, `LIMIT`/`OFFSET`, a `MATCHES` member | yes — the one surviving `ErrIncompleteAST` condition after REQ-117/118/119 |
| **Defensive** | the `…is outside the catalogue` arms, one per grammar alternative the catalogue models | no, while REQ-117 holds — they exist so a grammar widening fails loudly instead of dropping silently |

The draft's "one member per extractor drop site" and PROBE-096's "a corpus that reaches **every**
drop site" cannot both hold against that split: no corpus reaches a site no legal query reaches, so
the probe would have been weakened rather than satisfied. Phase 0 resolved it — the enum enumerates
drop **classes**, the defensive arms share an unmodelled-construct member whose unreachability is
asserted at the unit level, and **completeness is held by a source-derived sweep** (the pattern the
repo already runs in its dispatch tripwires) rather than by a list a maintainer maintains.

### `Clause` already exists (Phase 0 finding)

The draft proposed a new `Clause` enum. [`parse.Clause`](../../openehr/aql/parse/ast.go) is landed —
`ClauseUnknown` (zero) / `ClauseSelect` / `ClauseWhere` / `ClauseOrderBy`, with a `String()`, already
carried on `IdentifiedPath`. So the clause axis **extends** it rather than introducing a twin.
Extension is **append-only** (the existing `iota` values must not move), `String()` gains an arm per
member, and widening a public enum is consumer-visible in one specific way that belongs in the
CHANGELOG: an existing exhaustive switch over `Clause` gains an unhandled case.

### `Span` reuses `Position`, not byte offsets (Phase 0 finding)

The draft proposed byte offsets. [`parse.Position`](../../openehr/aql/parse/parse.go) is `{Line, Col}`
and is what `SyntaxError`, every AST node, and `lint.Issue.Detail`'s syntax-error text already report.
A byte-offset span beside them would give one fact two incomparable spellings and force any consumer
correlating a drop with a syntax error to convert. Both come off the same ANTLR tokens, so reusing
`Position` costs nothing but the choice.

### Shape

```go
// openehr/aql/parse

// Span is a start/end pair in the package's existing position vocabulary.
type Span struct{ Start, End Position }

// DroppedConstruct records one construct the extractor could not represent.
// No field carries source text; Span points back into the input for callers
// that hold it and may quote it under their own policy.
type DroppedConstruct struct {
    Kind   ConstructKind // closed; zero value = unclassified (fail-closed switch material)
    Clause Clause        // the LANDED enum, extended append-only
    Span   Span
}

// Dropped returns every drop this Document's extraction recorded, in source
// order — a COPY, and it triggers the same lazy once-only extraction as
// Query()/QueryErr(), so a Document nobody called Query() on cannot read as
// "nothing dropped". QueryErr()'s text and sentinel are unchanged.
func (d *Document) Dropped() []DroppedConstruct
```

```go
// openehr/aql/lint

type Issue struct {
    // ... existing fields unchanged ...
    Span parse.Span // NEW; zero when not attributable. ONE span type across
                    // both packages — parse owns it, lint re-exports it
                    // (`type Span = parse.Span`), so PROBE-096's cross-package
                    // assertion is a comparison and not a conversion.
}
```

**Also for Phase 2:** `exhaustive` is **not** in [`.golangci.yml`](../../.golangci.yml)'s enabled set.
This REQ ships two closed enums whose contract obliges consumers to switch exhaustively and fail
closed; enabling `exhaustive` holds the SDK's own switches to the same standard it asks of readers.

## Definition of Ready

**Phase 0 has landed, so implementation (Phase 1+) may start.** Each item below is satisfied:

- ✅ Canonical prose exists — the two-surface field contract, the drop-record properties, the closed-enum-over-classes rule with source-derived completeness, the clause and span reuse, and the additive-only clause: [clinical-modeling.md § REQ-113 § Value-free structured drop records](../specifications/clinical-modeling.md#req-113--execution-oriented-parsed-aql-ast).
- ✅ The extractor sweep was **run, not remembered** — 27 `incomplete(...)` sites, one reachable class (§ Architecture). It is what changed the enum design from per-site to per-class.
- ✅ [PROBE-096](../specifications/conformance.md#probe-096--aql-value-free-structured-diagnostics) defined (`Status: Draft`), in three arms: (a) a record per drop, held by a source-derived sweep that is itself shown able to fail, plus the accessor's four properties; (b) no value-free field contains any substring of the input's **value spans**, asserted against the input rather than against expected strings, and the value-bearing fields deliberately **not** asserted clean; (c) spans resolve to the offending construct, in one span type across both packages.
- ✅ Negative space pinned: an unknown kind is fail-closed switch material (closed enum, zero = unclassified); a drop with no record is a **defect**, not a gap; a value-free field never falls back to embedding source text when attribution is hard — it reports the unattributability instead.
- ✅ Each phase names its verification command.

## Definition of Done

- `openehr/aql/parse` + `openehr/aql/lint` land the records with `// REQ-` citations; every
  `incomplete(...)` site records a kind, enforced by the source-derived sweep.
- Godoc on `QueryErr()`, `Dropped()` and `lint.Issue` states the field contract — the classification
  has to be readable where a consumer holds the type, not only in the spec.
- `exhaustive` enabled in [`.golangci.yml`](../../.golangci.yml) (§ Architecture), or a recorded
  reason not to.
- `traceability.yaml` + REQ.md **Impl.** column; `roadmap.md` row; CHANGELOG — the consumer-visible
  facts are the two additive surfaces **and** the `Clause` widening, which can leave an existing
  exhaustive switch with an unhandled case.
- `make spec-check` and `make ci` green; plan archived.

## Implementation checklist

| Step | Status |
|---|---|
| Spec text in REQ-113 + REQ-109 + PROBE-096 (Phase 0) | ✅ |
| `DroppedConstruct` + `Dropped()` + per-class kinds + source-derived completeness sweep | |
| `Clause` extended append-only (+ `String()` arms) | |
| `lint.Issue.Span` + field-contract godoc | |
| `exhaustive` linter enabled (or reason recorded) | |
| Tests with `// REQ-` / `// PROBE-` comments | |
| `make spec-check` | ✅ (Phase 0) |
| `make ci` | |

## Phases

### Phase 0 — Spec text — **done**

Wrote the drop-record rules into REQ-113 and the lint half into REQ-109, defined PROBE-096 (Draft),
and marked both requirements `partial`. Sweeping the extractor — rather than trusting this plan's
draft — found 27 drop sites but only one reachable class, which is why the enum counts classes and
completeness is a source sweep (§ Architecture). Two proposed types turned out to already exist
(`parse.Clause`, `parse.Position`), so both are reused instead of twinned.

**Verification:** `make spec-check` — OK.

### Phase 1 — Drop channel

1. Derive the `ConstructKind` members from the § Architecture split — a member per **reachable**
   class, one unmodelled-construct member for the defensive arms — and thread a record through
   every drop site.
2. Extend `parse.Clause` append-only with the clauses the drop sites need beyond the landed three
   (`String()` arm each), and add `Span` over `Position`.
3. Expose `Document.Dropped()` with its four properties (triggers extraction, returns a copy, zero
   kind unclassified, existing error untouched).
4. Completeness: the source-derived sweep, plus a guard that the sweep itself fails when a site
   carries no kind, plus unit assertions that the defensive arms are unreachable by a legal query.

**Definition of done:** `go test ./openehr/aql/parse/...` green; PROBE-096 arm (a) implemented.

### Phase 2 — Lint spans + field contract

1. Add `Span` to `lint.Issue` as a re-export of `parse.Span` (one type, two packages), populated
   from the token positions lint already visits.
2. Godoc the field contract on `Issue` and on the drop surface; audit existing codes' `Detail` texts
   against it — the texts **stay**, the contract *names* them value-bearing.
3. Enable `exhaustive` (§ Architecture) and fix what it finds, or record why not.

**Definition of done:** PROBE-096 arms (b)+(c) implemented; `make ci` green.

## Mapping to specs

- [clinical-modeling.md § REQ-113 § Value-free structured drop records](../specifications/clinical-modeling.md#req-113--execution-oriented-parsed-aql-ast) — the canonical normative contract (authored in Phase 0; this plan restates none of it)
- [conformance.md § PROBE-096](../specifications/conformance.md#probe-096--aql-value-free-structured-diagnostics) — the three arms and their oracles
- REQ-109 / REQ-113 / REQ-119 sections — the landed diagnostic surfaces this plan structures, and the one-position precedent it generalises
