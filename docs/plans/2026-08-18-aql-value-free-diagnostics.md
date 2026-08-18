# Plan — AQL value-free structured diagnostics (drop channel + lint spans)

**Date:** 2026-08-18
**Status:** Draft
**Owner:** SDK maintainers
**Covers:** **REQ-161** (value-free structured diagnostics) — proposed; same new band as the [structured-node-predicates plan](2026-08-18-aql-structured-node-predicates.md) (proposed 160–169 "AQL structured model & diagnostics"; Phase 0 of whichever plan lands first allocates the band).
**Verifies / builds on:** landed [REQ-113](../specifications/clinical-modeling.md) (the incomplete-AST channel: `aql.ErrIncompleteAST`, `Document.QueryErr()`), [REQ-109](../specifications/clinical-modeling.md) (lint issue model), the archived [AQL honesty-residuals plan](archive/2026-08-18-aql-honesty-residuals.md) (MATCHES URI diagnostic redaction — the one-off this plan generalises)
**Probes:** **PROBE-096** (proposed; allocated in Phase 0) — every drop-channel and lint diagnostic carries a structured record whose fields contain no source text
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

Sketch — Phase 0 / review material, not normative:

```go
// openehr/aql/parse

// DroppedConstruct records one construct the extractor could not represent.
// No field carries source text; Span points back into the input for callers
// that hold it and may quote it under their own policy.
type DroppedConstruct struct {
    Kind   ConstructKind // closed enum: IntegerOutOfRange, ... (one per incomplete() site)
    Clause Clause        // closed enum: Select, From, Where, OrderBy, Limit, Offset, Top
    Span   Span          // byte offsets [Start, End) into the parsed input
}

// Dropped returns every drop this Document's extraction recorded, in source
// order. Empty when QueryErr() is nil. The existing QueryErr() text is
// unchanged.
func (d *Document) Dropped() []DroppedConstruct
```

```go
// openehr/aql/lint

type Issue struct {
    // ... existing fields unchanged ...
    Span Span // NEW: where in the input; zero when not attributable
}
// Field contract for Phase 0 to pin as REQ text (no normative force
// here — the REQ is unallocated; Phase 0 assigns the RFC-2119 keywords):
//   Code, Severity, Span    — value-free: never carry source text
//   Detail, Path            — value-bearing: may embed query spellings; a
//                             disclosure boundary treats them accordingly
```

Rules the spec section must pin (Phase 0):

- `ConstructKind` is a **closed, enumerated** set with one member per extractor
  drop site; adding a member is a consumer-visible change (CHANGELOG). A reader
  must be able to switch exhaustively and fail closed on an unknown kind.
- `Dropped()` is **complete**: every path that sets the incomplete flag records a
  construct. An extractor drop with no record is a defect, testable as such.
- Existing error texts and `Result.OK()` semantics are unchanged — additive only.
- `Span` is byte-offset based on the exact input handed to `Parse`
  (the REQ-119 verbatim discipline makes offsets stable and meaningful).

## Definition of Ready

Implementation (Phase 1+) may start once **Phase 0 has landed the REQ**:

- Band + REQ id allocated (shared Phase 0 with the structured-predicates plan if
  both proceed; either plan's Phase 0 may allocate the band).
- Canonical prose exists: the closed `ConstructKind`/`Clause` enums, the
  completeness rule, the value-free field contract for both surfaces.
- PROBE-096 (or the allocated id) defined in `conformance.md` (Draft): for a
  corpus that reaches **every** drop site and a representative lint-issue set,
  assert (a) a record exists per drop, (b) no structured field contains any
  substring of the input's value spans, (c) spans point at the offending text.
- Negative space: an unknown `ConstructKind` is a fail-closed switch case for
  readers (closed enum); a drop with no record is a defect, not a gap; the
  value-free fields never fall back to embedding source text when attribution
  is hard.
- Each phase names its verification command.

## Definition of Done

- `openehr/aql/parse` + `openehr/aql/lint` land the records with `// REQ-`
  citations; every `incomplete(...)` site records a kind.
- Godoc on `QueryErr()` and `lint.Issue` states the field contract.
- `traceability.yaml` + REQ.md **Impl.** column; `roadmap.md` row; CHANGELOG
  (consumer-visible addition).
- `make spec-check` and `make ci` green; plan archived.

## Implementation checklist

| Step | Status |
|---|---|
| Band + REQ § + registry row (Phase 0) | |
| PROBE defined in `conformance.md` (Draft) | |
| `DroppedConstruct` + `Dropped()` + per-site kinds | |
| `lint.Issue.Span` + field contract godoc | |
| Tests with `// REQ-` / `// PROBE-` comments | |
| `make spec-check` | |
| `make ci` | |

## Phases

### Phase 0 — Band, spec & registry (the specify gate)

1. Allocate band + REQ id (coordinate with the structured-predicates plan;
   150–159 is the transport extension band since PR #107's round 1, so 160–169
   is the next free decade).
2. Sweep the extractor for every `incomplete(...)` site; derive the closed
   `ConstructKind` set from that sweep, not from memory.
3. Author the canonical section (enums, completeness, field contract); registry
   row (**Impl.:** `planned`); probe definition; `traceability.yaml` row.

**Definition of done:** `make spec-check` passes with the new rows.

### Phase 1 — Drop channel

1. Thread a `DroppedConstruct` through each extractor drop site; expose
   `Document.Dropped()`.
2. Completeness test: a corpus case per drop site; a guard test that fails when a
   new `incomplete(...)` call records no kind.

**Definition of done:** `go test ./openehr/aql/parse/...` green; probe half (a) implemented.

### Phase 2 — Lint spans + field contract

1. Add `Span` to `lint.Issue`; populate from the token positions lint already
   visits.
2. Godoc the field contract on `Issue`; audit existing codes' `Detail` texts
   against it (texts stay, the *contract* names them value-bearing).

**Definition of done:** probe halves (b)+(c) implemented; `make ci` green.

## Mapping to specs

- Phase 0 authors the canonical section — this plan holds proposals only.
- [docs/specifications/REQ.md](../specifications/REQ.md) — band + registry row (Phase 0)
- REQ-109 / REQ-113 sections — the landed diagnostic surfaces this plan structures
