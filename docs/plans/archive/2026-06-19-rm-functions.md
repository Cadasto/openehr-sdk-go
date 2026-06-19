# Plan — RM behavioural functions (identifiers, paths, version-derived, temporal)

**Date:** 2026-06-19
**Status:** Landed (2026-06-19)
**Owner:** SDK maintainers
**Covers:** [REQ-120](../specifications/rm-functions.md#req-120--rm-identifier-parsing-and-derivation), [REQ-121](../specifications/rm-functions.md#req-121--locatable-path-read-access), [REQ-122](../specifications/rm-functions.md#req-122--version-control-derived-helpers), [REQ-123](../specifications/rm-functions.md#req-123--temporal-data-value-helpers); [ADR 0011](../adr/0011-rm-behavioural-functions-surface.md)
**Probes:** none — these are in-memory behaviours, unit-tested rather than wire-conformance (consistent with REQ-111).
**Implementation:** landed
**Depends on:** the `bmmgen` stub-suppression hook (Phase 1) — every method-filling phase needs it. Landed packages reused: `openehr/rm` types, `openehr/aql/parse` / `openehr/template` (path-segment parsing reference), `openehr/validation/rmread` (walker *pattern* reference only — not imported).
**Defers:** `PATHABLE.parent` / `path_of_item`; `VERSIONED_OBJECT` container ops (`version_count`/`version_at_time`/`latest_version`/`all_versions`/`commit_*`); temporal **arithmetic** (`add`/`subtract`/`diff`/`multiply`/`add_nominal`); FLAT/STRUCTURED. These remain generated panic-stubs, documented as out-of-scope in [rm-functions.md](../specifications/rm-functions.md).

## Goal

Implement the pure/derived openEHR RM behavioural functions that `bmmgen` currently emits as panic-stubs, so SDK consumers can — without a server — parse and decompose identifiers, navigate an in-memory RM instance by an openEHR path, test version-branch status, and read/inspect/compare/convert the temporal `DV_*` values. Hybrid surface per ADR 0011: concrete-typed derivations as methods on the `openehr/rm` types; path read access as `openehr/rm/rmpath` package functions; a single canonical identifier parser that the existing `client/ehr` version-uid helper delegates to.

## Architecture & two design refinements vs ADR 0011

The hybrid surface holds, with two refinements forced by Go realities (fold into ADR 0011 Consequences in Phase 1):

1. **Generator hook.** `bmmgen` gains a curated `manuallyImplemented` set (keyed `OWNER.function`). `renderFunctions` skips stub emission for those, so the function can be hand-written in a non-generated file without a redeclaration collision. Rationale recorded against [ADR 0002](../adr/0002-bmm-codegen-decisions.md) (generator structural decisions).
2. **Paths are package functions, not delegating methods.** `openehr/rm/rmpath` imports `openehr/rm`; if `rm`'s `LOCATABLE` methods called back into `rmpath` that would be an import cycle. So the `LOCATABLE` path-method stubs are **suppressed** (not filled), and `rmpath.ItemAtPath(root, path)` etc. are the surface — more REQ-023-idiomatic anyway. Identifier, version-derived, and temporal functions have no such cycle and stay as methods in package `rm`.

## File structure

| File | Responsibility |
|---|---|
| `internal/bmmgen/render_function.go` (modify) | Consult a new `manuallyImplemented` set; skip stub emission for listed `OWNER.function`. |
| `internal/bmmgen/manual_impl.go` (create) | The curated `manuallyImplemented` set + a doc comment listing where each is hand-written. |
| `openehr/rm/*_gen.go` (regenerate) | Stubs for the hand-implemented / suppressed functions disappear; `make codegen-verify` re-pins. |
| `openehr/rm/identification_funcs.go` (create) | REQ-120: `Parse*` functions + methods (`UID_BASED_ID`, `OBJECT_VERSION_ID`, `VERSION_TREE_ID`, `ARCHETYPE_ID`, `TERMINOLOGY_ID`, `LOCATABLE_REF`). |
| `openehr/rm/identification_funcs_test.go` (create) | Table-driven parse tests incl. malformed; `// REQ-120`. |
| `openehr/rm/changecontrol_funcs.go` (create) | REQ-122: `Version.IsBranch` (derives via the version uid). |
| `openehr/rm/temporal_funcs.go` (create) | REQ-123: ISO-8601 component accessors, partial-form inspection, `Magnitude`, comparison, `ToTime`/`ToDuration` on `DV_DATE`/`DV_TIME`/`DV_DATE_TIME`/`DV_DURATION`. |
| `openehr/rm/temporal_funcs_test.go` (create) | Canonical + partial cases; ordering; conversion errors; `// REQ-123`. |
| `openehr/rm/rmpath/rmpath.go` (create) | REQ-121: `ItemAtPath`/`ItemsAtPath`/`PathExists`/`PathUnique` + path parser + own typed walker + error sentinels. |
| `openehr/rm/rmpath/rmpath_test.go` (create) | Resolution against bundled OPT-built compositions; `// REQ-121`. |
| `openehr/client/ehr/ids.go` (modify) | `VersionUID` helpers delegate to `rm.ParseObjectVersionID` (de-dup). |

## Implementation checklist

| Step | Status |
|---|---|
| Generator stub-suppression hook + regenerate (`make codegen-verify`) | ✅ `56cf692` |
| REQ-120 identifier parsing/derivation + client delegation | ✅ `40bd4bb` |
| REQ-122 version-derived helper | ✅ `68a8b41` |
| REQ-121 `rmpath` read access | ✅ `fe82410` |
| REQ-123 temporal helpers | ✅ `68e908b` |
| Tests with `// REQ-120..123` comments | ✅ |
| `traceability.yaml` packages/tests + `REQ.md` Impl. → `landed`; roadmap + CHANGELOG `[Unreleased]` | ✅ `f3589ed` |
| `make spec-check` · `make ci` | ✅ green |

## Phases

### Phase 1 — Generator stub-suppression hook

**Tasks:**
- Add `internal/bmmgen/manual_impl.go` with `var manuallyImplemented = map[string]bool{}` keyed `"OWNER.function"` (e.g. `"OBJECT_VERSION_ID.creating_system_id"`), plus a comment naming the hand-written file for each. Seed it with the REQ-120/121/122/123 functions in scope (and only those).
- In `renderFunctions` ([render_function.go](../../internal/bmmgen/render_function.go)), at each emit site (the abstract-descendant loop and the concrete/abstract-generic loop), skip when `manuallyImplemented[ownerName+"."+fn.Name]` (or `recvName+"."+fn.Name` for propagated abstract functions — choose the key that matches how `item_at_path` propagates to concrete `LOCATABLE` descendants; verify with `Section.ItemAtPath`).
- Regenerate (`make bmmgen` / the repo's codegen target) and run `make codegen-verify`.
- Record the hook in ADR 0002 (new "D7 — manual-implementation skip" subsection) and tick the ADR 0011 Consequences refinements above.

**Definition of done:** `make codegen-verify` green; the suppressed functions (e.g. `Section.ItemAtPath`, `ObjectVersionID.CreatingSystemID`) no longer appear in `openehr/rm/*_gen.go`; `make ci` still builds (no references to the removed stubs — confirm `git grep` finds none outside this plan's new files).

### Phase 2 — REQ-120 identifier parsing & derivation

**Tasks:**
- `openehr/rm/identification_funcs.go`: canonical parsers returning `(T, error)` — `ParseObjectVersionID`, `ParseArchetypeID`, `ParseVersionTreeID`, `ParseTerminologyID`; and methods on the value structs deriving components from the stored `value`/`Value` string:
  - `UID_BASED_ID`: `Root() UID`, `Extension() string`, `HasExtension() bool` (split on first `::`).
  - `OBJECT_VERSION_ID`: `ObjectID() UID`, `CreatingSystemID() UID`, `VersionTreeID() VersionTreeID`, `IsBranch() bool` (split on `::`; `IsBranch` from the version-tree part).
  - `VERSION_TREE_ID`: `TrunkVersion() string`, `BranchNumber() string`, `BranchVersion() string`, `IsBranch() bool` (1- vs 3-part dot split).
  - `ARCHETYPE_ID`: `RmOriginator`/`RmName`/`RmEntity`/`QualifiedRmEntity`/`DomainConcept`/`Specialisation`/`VersionID` (split on the documented form).
  - `TERMINOLOGY_ID`: `Name() string`, `VersionID() string` (`name [ '(' version ')' ]`).
  - `LOCATABLE_REF`: `AsURI() string`.
  - Best-effort methods return zero values for malformed input; the `Parse*` functions return errors. No panics.
- Add each function key to `manuallyImplemented` (Phase 1).
- `openehr/client/ehr/ids.go`: `VersionUID.CreatingSystemID`/`VersionedObjectID` delegate to `rm.ParseObjectVersionID` (keep the existing exported signatures and behaviour; add a regression test that the two agree).

**Definition of done:** table-driven tests (canonical + malformed for each form) pass and carry `// REQ-120`; the client helper produces identical output to the canonical parser; `make ci` green.

### Phase 3 — REQ-122 version-control derived helper

**Tasks:**
- `openehr/rm/changecontrol_funcs.go`: `Version.IsBranch() bool`, deriving from the version's `uid` `OBJECT_VERSION_ID` via REQ-120's `VersionTreeID().IsBranch()`. Add its key to `manuallyImplemented`.
- Leave `VERSIONED_OBJECT` container ops and `commit_*` as the existing generated panic-stubs (out of scope; spec documents them as server-side). No change beyond a doc pointer if useful.

**Definition of done:** test asserts `IsBranch` true for a branch uid (`…::…::1.1.1`), false for trunk (`…::…::1`), `// REQ-122`; `make ci` green.

### Phase 4 — REQ-121 locatable path read access

**Tasks:**
- Create package `openehr/rm/rmpath` (imports `openehr/rm` only — zero `transport`/`auth`, REQ-013):
  - Path parser: `/`-separated segments, each an RM attribute name with optional `[at-code]` / `[name]` / `[at-code,'name']` predicate. Reuse the segment/predicate shapes from `openehr/template/path.go` / `openehr/aql/parse` (copy the small grammar; do not import the template compiler).
  - Own reflection-free walker: a typed switch dispatching attribute access per RM type (the `rmread` pattern, re-implemented locally to avoid importing `openehr/validation`).
  - Public API: `ItemAtPath(root rm.Locatable, path string) (any, error)`, `ItemsAtPath(root rm.Locatable, path string) ([]any, error)`, `PathExists(root rm.Locatable, path string) bool`, `PathUnique(root rm.Locatable, path string) bool`; sentinels `ErrPathNotFound`, `ErrPathAmbiguous`, `ErrPathSyntax`.
- Add the `LOCATABLE.{item_at_path,items_at_path,path_exists,path_unique,path_of_item}` keys to `manuallyImplemented` so the panic-stubs are suppressed (no rm method; rmpath is the surface). `parent`/`path_of_item` stay deferred — leave `path_of_item`/`parent` stubs OR suppress; document either way.

**Definition of done:** tests build a composition from `vital_signs.opt` (via the existing instance generator / builder), then assert a known unique leaf path resolves via `ItemAtPath` to the expected value; a non-unique path returns the expected count via `ItemsAtPath`; an absent path → `PathExists == false`; a multi-match path → `PathUnique == false` and `ItemAtPath` returns `ErrPathAmbiguous`. `// REQ-121`; `make ci` green.

### Phase 5 — REQ-123 temporal data-value helpers

**Tasks:**
- `openehr/rm/temporal_funcs.go`: parse each type's ISO-8601 `value` and expose, per ADR 0011 surface:
  - Component accessors — `DV_DATE`: `Year`/`Month`/`Day`; `DV_TIME`: `Hour`/`Minute`/`Second`/`FractionalSecond`; `DV_DATE_TIME`: the union; `Timezone` where present; `DV_DURATION`: `Years`/`Months`/`Weeks`/`Days`/`Hours`/`Minutes`/`Seconds`/`FractionalSeconds`.
  - Partial-form inspection — `IsPartial`, and for dates `MonthUnknown`/`DayUnknown`.
  - `Magnitude()` (days for `DV_DATE`; seconds for the others) and a `Compare(other) int` (-1/0/+1) consistent with the spec's `less_than`.
  - Go bridge — `ToTime() (time.Time, error)` on the three date/time types; `ToDuration() (time.Duration, error)` on `DV_DURATION`; return an error for partial values (and nominal `Y`/`M` durations).
  - Add each key to `manuallyImplemented`; malformed `value` → error from a `Parse*`-style entry, never panic.

**Definition of done:** tests cover canonical + partial strings (`"2024"`, `"2024-03"`), ordering of a known sequence via `Compare`, and `ToTime`/`ToDuration` success on full values + error on partial/nominal; `// REQ-123`; `make ci` green.

### Phase 6 — Close-out

**Tasks:**
- `traceability.yaml`: set `implementation: landed` and add `packages:` + `tests:` for REQ-120..123; add `plans: [docs/plans/2026-06-19-rm-functions.md]` to each.
- `REQ.md`: flip the four `Impl.` cells `planned → landed`.
- `docs/roadmap.md`: add an RM-behavioural-functions row(s).
- `CHANGELOG.md` `[Unreleased]`: one bullet for the RM behavioural functions (REQ-120..123).
- Run `make spec-check` and `make ci`; archive this plan per `sdd-archive`.

**Definition of done:** `make spec-check` green with REQ-120..123 `landed` (packages+tests present); `make ci` green; CHANGELOG + roadmap updated.

## Mapping to specs

- [rm-functions.md § REQ-120–123](../specifications/rm-functions.md) — normative contract.
- [ADR 0011](../adr/0011-rm-behavioural-functions-surface.md) — surface & fallibility (amended in Phase 1 with the generator hook + paths-as-functions refinements).
- [ADR 0002](../adr/0002-bmm-codegen-decisions.md) — generator structural decisions (gains the manual-implementation skip).
- [REQ.md](../specifications/REQ.md) — registry rows; [traceability.yaml](../specifications/traceability.yaml) — machine map.

## Risks & notes

- **Generator key matching.** `item_at_path` is declared on abstract `PATHABLE`/`LOCATABLE` and propagated to concrete descendants; confirm whether `manuallyImplemented` should key on the declaring owner (`LOCATABLE.item_at_path`) or the receiver — Phase 1 must verify against the actual `Section.ItemAtPath` emission and pick the key that suppresses all descendants.
- **Partial dates vs `time.Time`.** The Go bridge must error (not silently zero-fill) on partial values — this is the headline value-add; assert it explicitly.
- **No new dependencies.** All four areas are pure-stdlib (`strings`, `time`, `strconv`); rmpath imports only `openehr/rm`. Preserves building-block independence (REQ-013) and the dependency policy.
- **Out-of-scope stubs stay panicking.** Acceptable per ADR 0011 (fail-loud); a future `bmmgen` option to emit non-panicking stubs for the deferred set is a separate follow-up.
