# ADR 0002 — BMM code generator structural decisions

- **Status:** Accepted, 2026-05-16.
- **Supersedes:** —
- **Superseded by:** —
- **Tracks:** [`docs/plans/2026-05-15-bmm-codegen.md`](../plans/2026-05-15-bmm-codegen.md).

## Context

The BMM-driven generator (`internal/bmmgen`, `cmd/bmmgen`) emits the SDK's openEHR domain types from pinned schemas under `resources/bmm/`. Several layout choices are non-obvious from [`specs/bmm-conformance.md`](../../specs/bmm-conformance.md) alone and would be expensive to reverse after v1. This ADR records them for reviewers and agents.

Operational BMM bumps are covered by [ADR 0001](0001-bmm-version-bump-runbook.md).

## Decision

### D1 — Flat single package per generation target

All RM classes emit into one Go package `openehr/rm/`, with one `*_gen.go` file per BMM top-level package (file names strip `org.openehr.rm.` / `org.openehr.base.` and dot-to-underscore the remainder). AOM 1.4 follows the same flat pattern at `openehr/aom/aom14/`. No nested Go sub-packages mirror the BMM tree.

### D2 — Descendant-shadows-ancestor BMM include merge

`bmm.LoadAll` allows a descendant schema's class to override an ancestor schema's class with the same name (descendant wins). Two sibling ancestor schemas containing the same class still return `ErrSchemaConflict`. Matches REQ-047 and real openEHR corpus shape (base + RM refinements).

### D3 — `typereg_gen.go` in package `rm` / `aom14`, not under `typereg/`

The hand-written `Registry` lives in `openehr/rm/typereg/`. Generated `init()` registrations live in `openehr/rm/typereg_gen.go` (and `openehr/aom/aom14/typereg_gen.go`) so constructors can reference concrete types without an `rm ↔ typereg` import cycle. RM and AOM share `typereg.Default`; discriminator strings are disjoint.

### D4 — Abstract classes flatten into concrete descendants; abstract generics are structs or interfaces

Non-generic abstract classes become Go interfaces with unexported `is<X>()` markers; their properties flatten into each concrete descendant struct. Abstract **generic** classes **with concrete descendants** (e.g. `EVENT`, `VERSION`) are also emitted as marker interfaces so codec polymorphism works — see [ADR 0003](0003-rm-event-polymorphism.md). Abstract generics **without** concrete descendants remain generic structs.

### D5 — AOM 1.4 references RM for base types (one-way import)

`openehr/aom/aom14/` imports `openehr/rm` for shared base types (`rm.HierObjectID`, etc.). AOM does not duplicate base classes. Dependency is strictly `aom14 → rm`.

### D6 — BMM functions become panic stubs; bodies live in `*_ext.go`

Every BMM `function` becomes a Go method whose body is `panic("not implemented: …")` with BMM documentation propagated as godoc. Real implementations belong in hand-written `*_ext.go` companions only (REQ-044). The generator never touches non-`_gen.go` files.

## Consequences

- Consumers construct COMPOSITION values from a single `rm` import; no six-package import fan-out.
- Calling unimplemented BMM functions panics by design — see package doc on `openehr/rm`.
- `make test` chains `codegen-verify`; hand-edits to `*_gen.go` fail CI.
- Event[T] cassette decode requires a follow-up policy change (ADR 0003), not a local codec workaround.

## References

- [`specs/bmm-conformance.md`](../../specs/bmm-conformance.md)
- [`docs/architecture.md`](../architecture.md) — narrative companion
- [`internal/bmmgen/`](../../internal/bmmgen/)
