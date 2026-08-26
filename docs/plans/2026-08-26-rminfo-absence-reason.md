# Plan — RM class-universe absence reasons

**Date:** 2026-08-26
**Status:** Draft
**Owner:** SDK maintainers
**Covers:** [REQ-049](../specifications/bmm-conformance.md#req-049--rm-class-universe-absence-reasons)
**Probes:** [PROBE-098](../specifications/conformance.md#probe-098--absence-reasons-account-for-the-universes-negative-space)
**Implementation:** planned
**Depends on:** REQ-048 (landed) — the class universe, the `Hierarchy` surface, and PROBE-094's independent BMM reduction, all of which this plan reuses
**Defers:** naming the *skipped package* for an excluded-package name; any AM/LANG/TERM universe; any policy mapping of reasons

## Goal

`openehr/rm/rminfo` gains a generated **absence surface**: a closed `AbsenceReason` enum, an optional `AbsenceReporter` capability interface that `Default` implements, a generated `absenceData` table, and a `NewWithAbsence` synthetic seam. A consumer can then tell "the pinned schemas do not declare this name" from "declared, and this generator deliberately does not ship it" — a distinction the generator computes on every run today and discards, leaving consumers to re-derive it by hand.

## Definition of Ready

- [x] **Covers:** lists every REQ this plan implements (REQ-049).
- [x] Canonical normative prose exists ([bmm-conformance.md § REQ-049](../specifications/bmm-conformance.md#req-049--rm-class-universe-absence-reasons)) + registry row in [REQ.md](../specifications/REQ.md).
- [x] No irreversible fork: the surface is additive and follows the `AttributeLister` optional-interface precedent — no new ADR needed.
- [x] Tasks below name concrete files and the verification command per task.

## Definition of Done

- Code and tests land with `// REQ-049` / `// PROBE-098` citations.
- [`traceability.yaml`](../specifications/traceability.yaml) and the REQ.md **Impl.** column read `landed`; PROBE-098 **Status:** Implemented (inline).
- `make spec-check` and `make ci` pass.
- Plan archived under [`docs/plans/archive/`](archive/).

## Global constraints (binding on every task)

1. **Additive only.** `Lookup`, `Hierarchy`, `AttributeLister`, `New`, and every existing answer stay byte-unchanged ([idiom.md § Public-API stability](../specifications/idiom.md#public-api-stability)). The accessor goes on a NEW optional interface — never on `Lookup` or `Hierarchy`.
2. **Precedence, fixed:** primitive → enumeration → excluded class → excluded package. A name matching several rules reports the first.
3. **`AbsenceNone` is the zero value.** *Undeclared* is computed, never stored; the stored table never contains a universe member or an `AbsenceNone`/`AbsenceUndeclared` entry.
4. **Generation fails loudly:** a declared-but-omitted name no rule accounts for is a generation **error** naming the class, not a warning.
5. **Probe independence:** PROBE-098 derives its expectation through the `openehr/bmm` loader plus restated literal lists — it never imports `internal/bmmgen`.
6. **Generated file rules (REQ-042):** the table lands in `openehr/rm/rminfo/absence_gen.go` with the standard generated header; regenerate with `make codegen`; `make codegen-verify` must be green.
7. Tests are table-driven where natural, carry `// REQ-049` / `// PROBE-098` citations, and follow the package's existing comment density. Formatting is gofumpt + goimports (the save hook handles it).
8. Conventional Commits, one logical change per commit. `go.mod` untouched.

## Phases

### Task 1 — the hand-written half of the surface

**Files:** `openehr/rm/rminfo/absence.go` (new), `openehr/rm/rminfo/absence_test.go` (new), `openehr/rm/rminfo/lookup.go` (the unexported `lookup` struct gains an `absence map[string]AbsenceReason` field — nothing else in that file changes in this task).

**Exact API** (doc comments to be written properly, in the package's voice):

```go
// AbsenceReason reports why an RM type name is not in the class universe
// (REQ-049). The zero value, AbsenceNone, means the name IS in the universe.
type AbsenceReason int

const (
	AbsenceNone            AbsenceReason = iota // in the universe — not absent at all
	AbsenceUndeclared                           // no pinned schema of the RM generation target declares the name
	AbsenceExcludedPackage                      // declared; its BMM package is skipped wholesale (REQ-042)
	AbsenceExcludedClass                        // declared; in the generator's named-class exclusion set
	AbsencePrimitive                            // declared as/mapped to a primitive_types value, not a modelled class
	AbsenceEnumeration                          // declared as a BMM enumeration, not a class
)

func (r AbsenceReason) String() string

// AbsenceReporter is an optional extension of Lookup (same pattern as
// AttributeLister): consumers discover it by runtime type assertion.
type AbsenceReporter interface {
	// AbsenceReason reports why rmType is not in the class universe.
	// AbsenceNone (the zero value) means rmType IS in the universe.
	AbsenceReason(rmType string) AbsenceReason
}

// NewWithAbsence constructs a Lookup over caller-supplied class data plus a
// synthetic absence table. Same retention semantics as New.
func NewWithAbsence(data map[string]ClassMeta, absence map[string]AbsenceReason) Lookup
```

**Semantics:**

- `(*lookup).AbsenceReason(rmType)`: membership in `l.data` → `AbsenceNone`; else a hit in `l.absence` → the stored reason; else `AbsenceUndeclared`.
- Universe wins where `data` and `absence` name the same class (document on `NewWithAbsence`).
- A `Lookup` built by plain `New` (nil absence table) reports `AbsenceUndeclared` for every out-of-universe name — document this fallback on `New`'s doc comment as well as `NewWithAbsence`'s.
- `String()` names the kind only (e.g. `"excluded package"`); an out-of-range value formats as `"absence reason N"`. It never echoes a queried name.
- Compile-time `var _ AbsenceReporter = (*lookup)(nil)` beside the existing assertions, with the same style of comment.

**Tests (TDD — write first):** behaviour-level, synthetic data only, no pinned RM: zero-value distinctness (every absence member ≠ `AbsenceNone`); a `New`-built lookup answers `AbsenceNone` for members and `AbsenceUndeclared` for anything else; a `NewWithAbsence`-built lookup returns each stored reason; overlap → `AbsenceNone`; `String()` for every member plus one out-of-range value; `Default` satisfies `AbsenceReporter` via type assertion (it does already in this task — the nil table gives degraded answers until Task 2 wires the generated data, which is fine mid-branch and stated in the commit body).

**Verify:** `go test ./openehr/rm/rminfo/`, `go build ./...`, `golangci-lint run` (host binary) over the touched packages.

### Task 2 — the generator and the generated table

**Files:** `internal/bmmgen/` (`plan.go` / `render_rminfo.go` / `primitives.go` as needed), generated `openehr/rm/rminfo/absence_gen.go` (via `make codegen` — never by hand), `openehr/rm/rminfo/lookup.go` (`Default` gains `absence: absenceData`), generator unit tests beside the existing render tests.

- During planning/render for the RM generation target, record every declared-but-omitted name with its reason under the fixed precedence (constraint 2). Inputs: the target schemas' `class_definitions` **and** `primitive_types`.
- Rules, in precedence order: primitive mapping (a `primitive_types` entry, or a `primitiveGoType`-mapped name — **not** `isSkippedPrimitive`, whose `class_definitions` entries the spec taxonomises as *excluded class*) → `AbsencePrimitive`; BMM enumeration kind → `AbsenceEnumeration`; `skippedClasses` → `AbsenceExcludedClass`; `skippedPackagePrefixes` → `AbsenceExcludedPackage`. Anything declared and omitted that no rule catches: **fail generation** with an error naming the class (constraint 4).
- The rendered table is sorted by class name and deterministic across runs. Universe members and computed kinds never enter it (constraint 3).
- Run `make codegen`; commit the regenerated file; `make codegen-verify` green.
- **Generator unit tests:** precedence on a belt-and-braces name (a primitive-mapped name that also sits in `skippedClasses` reports `AbsencePrimitive`); the unaccounted-name error path (synthetic schema with a declared class no rule catches); no-overlap with the universe on the real pinned BMM output.

**Verify:** `go test ./internal/bmmgen/ ./openehr/rm/rminfo/`, `make codegen-verify`, `go build ./...`, host `golangci-lint run`.

### Task 3 — PROBE-098

**Files:** `openehr/rm/rminfo/probe_098_test.go` (new), `openehr/rm/rminfo/probe_094_test.go` (refactor only: `exclusionReason` returns a typed kind alongside its prose so both probes share one derivation — no arm of PROBE-094 changes meaning).

- Arms (a)–(e) exactly as the [conformance entry](../specifications/conformance.md#probe-098--absence-reasons-account-for-the-universes-negative-space) defines them, using the existing `bmmReduction` machinery and the restated literal lists (constraint 5).
- **Falsify before committing** and record the evidence in the task report: (i) add `COMPOSITION` to a copy of the table → arm (b) fails; (ii) remove `EXTRACT`'s entry → arm (a) fails; revert both.

**Verify:** `go test ./openehr/rm/rminfo/`, host `golangci-lint run`.

### Task 4 — close-out

- [conformance.md](../specifications/conformance.md): PROBE-098 **Status:** → Implemented (inline) with test-file links; coverage-matrix row updated to match.
- [REQ.md](../specifications/REQ.md): REQ-049 **Impl.** `planned` → `landed`.
- [traceability.yaml](../specifications/traceability.yaml): `implementation: landed`; add the `tests:` list (`absence_test.go`, `probe_098_test.go`, the generator render test file).
- `CHANGELOG.md`: one short entry under Unreleased (artefact-class style, matching neighbours).
- `openehr/rm/rminfo/doc.go`: one sentence for the absence surface.
- [docs/plans/README.md](README.md): flip this plan's entry to landed wording; plan **Status:** → complete; `git mv` the plan to [archive/](archive/) and add its row to [archive/README.md](archive/README.md).
- **No normative spec edits expected in this task.** If implementation diverged from REQ-049's prose, stop and report — do not edit the spec to match the code.

**Verify:** `make spec-check`, `make probe-status` (inline probes report MISSING there by design), then the full `make ci`.

## Mapping to specs

- [bmm-conformance.md § REQ-049](../specifications/bmm-conformance.md#req-049--rm-class-universe-absence-reasons) — normative contract
- [conformance.md § PROBE-098](../specifications/conformance.md#probe-098--absence-reasons-account-for-the-universes-negative-space) — acceptance probe
- [REQ.md](../specifications/REQ.md) registry row · [traceability.yaml](../specifications/traceability.yaml) entry
