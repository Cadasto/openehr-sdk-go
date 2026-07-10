# Plan — Modernize & simplify (2026-07 repo-wide assessment follow-ups)

**Date:** 2026-07-10
**Status:** Draft
**Owner:** SDK maintainers
**Covers:** — (behaviour-preserving modernization/simplification of landed code; no new REQ. Guardrails: [REQ-024](../specifications/idiom.md#generics-policy-req-024) generics-no-reflection, [REQ-013](../specifications/module-layout.md#req-013--building-block-independence) building-block independence. Phase 5 requires a **new ADR** before implementation and cross-references [STRAND-04](../specifications/research-strands.md).)
**Probes:** — (existing probes are the regression net: `testkit/probes/versioned/` for Phase 3, `testkit/probes/instance/` for Phases 4–5)
**Implementation:** planned
**Depends on:** nothing landed-side; Phase 5 depends on its ADR being Accepted
**Defers:** `encoding/json/v2` codec simplification (stays parked under STRAND-04); table-driven rewrite of `openehr/validation/composition_test.go` (pure churn); `iter.Seq` API migrations (no `Visit(callback)` surface worth a break); restructuring `sampleByConstraint` / `materialiseMultiple` (inherent spec/cardinality logic)

## Goal

Close the structural-simplification findings from the 2026-07-10 repo-wide modernization assessment. The mechanical layer is already clean — Go 1.26, `go fix -diff` empty, `modernize`/`errorlint` gating CI, no dead code, no TODO debt, no idiom.md violations — so this plan targets what tooling cannot see: (a) copy-pasted versioned-write plumbing across the four REST leaf clients, (b) repeated value-or-pointer coercion in the instance write path, and (c) the lock-step family of ~341 hand-maintained `case *rm.X` type-switch arms across five files (`rmread/read.go` 116, `rmpath/walk.go` 72, `rmwrite/write.go` 59, `validation/composition.go` 58, `composition/typecheck.go` 36) whose maintenance burden the code comments themselves flag ("keep all three in lock-step"). Ships as one small PR per phase; consumers are SDK maintainers (less lock-step surface) and SDK users (two deliberate additions to the `rm` surface, no breaking changes).

**Architecture:** No new packages. Shared client helpers land in `openehr/client/ehr` (existing home of `VersionMetadata` / `ResolveIdentifierBody`); generic coercion helpers land beside the proven `coerceBound[T]` in `internal/templateinstance/rmwrite`; generated accessors go through `internal/bmmgen` + `make codegen` so `*_gen.go` stays machine-owned (ADR 0002 D6).

**Tech stack:** Go 1.26, no new dependencies. Repo gates: `make ci` (includes `spec-check`, `codegen-verify`), gofmt hook.

## Definition of Ready

- [x] Assessment findings verified against source (all file:line references below re-checked 2026-07-10).
- [x] No conflict with active plans (`2026-06-23-simplified-formats.md`, `2026-05-22-webtemplate-export.md`) or open strands.
- [ ] Phase 5 only: ADR drafted via the SDD flow and **Accepted** (an irreversible public-surface + generator fork — DoR per `_template.md`).

## Definition of Done

- Code and tests land with existing `// REQ-` citations preserved; no citation is orphaned by moved code.
- [`traceability.yaml`](../specifications/traceability.yaml) untouched unless files move; Phase 5 updates it for the new generated artifacts and the ADR row in [REQ.md](../specifications/REQ.md) / [adr/README.md](../adr/README.md).
- Phase 5 updates canonical spec prose ([rm-modeling.md](../specifications/rm-modeling.md)) for the widened `Locatable` contract in the same PR.
- `make spec-check` and `make ci` pass per phase; Phase 5 additionally `make codegen-verify` and `make test-race`.
- Plan archived under [`docs/plans/archive/`](archive/).

## Implementation checklist

| Step | Status |
|---|---|
| Phase 1 — deps currency | |
| Phase 2 — micro-cleanups (`cmp.Or`, `rm.ObjectIDValue`) | |
| Phase 3 — client versioned-write consolidation | |
| Phase 4 — generic coercion dedup (rmwrite / instance) | |
| Phase 5 — ADR + bmmgen Locatable accessors + reverse type map | |
| `make spec-check` + `make ci` green per phase | |

## Phases

Ordered low-risk-first; Phases 1–4 are independent of each other, Phase 5 last (benefits from a quiet tree).

### Phase 1 — `chore(deps)`: dependency currency

**Tasks:**

- [ ] `go get github.com/coreos/go-oidc/v3@v3.20.0`; refresh stale indirect `golang.org/x/*` pins (`x/mod` v0.17→v0.38, `x/sync`, `x/tools`, `x/exp` 2024-era, `cloud.google.com/go/compute/metadata` v0.3→v0.9) + `make mod-tidy`. OTel 1.44 / antlr 4.13.1 already current.

**Verification / DoD:** `make ci`, `go build ./...`.

### Phase 2 — `refactor(transport,rm)`: micro-cleanups

**Tasks:**

- [ ] `transport/errors.go` — replace the hand-rolled `firstNonEmpty` (`:121–126`, single use at `:107`) with `cmp.Or` and drop the helper (`modernize` cannot rewrite custom helpers; first `cmp.Or` use in the repo).
- [ ] Add `rm.ObjectIDValue(ObjectID) (string, bool)` beside `rm.UIDValue` in `openehr/rm/identification_funcs.go` (the documented canonical home for identifier lexical forms, REQ-120); retire the private six-arm switch `objectIDValue` in `openehr/client/ehr/audit.go:132–179`. Deliberate, additive public-API change.

**Verification / DoD:** `make ci`; `openehr/client/ehr/audit_test.go` green; godoc on the new function cites REQ-120.

### Phase 3 — `refactor(client)`: consolidate versioned-write plumbing

The decode-by-Prefer state machine is copy-pasted four times — `composition/composition.go:329`, `directory/directory.go:298`, `demographic/party.go:316` (each `doWrite`), `ehrstatus/ehrstatus.go:183–205` (`Put`) — identical `PreferIdentifier`/default arms and error string; only the representation-arm decoder differs (`canjson.Unmarshal` into `*T` vs `typereg.DecodeAs[rm.Party]`).

**Tasks:**

- [ ] Add a generic write-result helper in `openehr/client/ehr` — shape: `writeResult[T any](ctx, c, req, prefer, decode func([]byte) (T, error)) (T, *VersionMetadata, error)`; the four leaf clients pass their decoder. `T` instantiates as an interface for the demographic (`rm.Party`) case.
- [ ] Factor the shared write-option internals into `openehr/client/ehr`: shared `writeConfig` + `resolveWriteHeaders` (audit / lifecycle / item-tag header marshal, currently repeated at `composition.go:79–117`, `directory.go:119–145`, `party.go:96–122`, `ehrstatus.go:95–122`) and the delete-tail metadata-on-error sequence (`composition.go:306–314`, `directory.go:281–289`, `party.go:268–276`). **Public `WriteOption` / `With*` types stay per package** (idiom.md § public-API stability) — their bodies become one-line delegations.
- [ ] Delete the redundant wrapper `ehrstatus.newVersionMetadata` (`ehrstatus.go:88–92`); call `openehrclient.NewVersionMetadata` directly like the sibling packages.

**Verification / DoD:** `make ci`; client unit tests + `testkit/probes/versioned/` (Prefer semantics incl. Probe011/Probe071); `git diff` shows no exported-symbol changes in the four leaf packages. Est. ~150–200 lines removed.

### Phase 4 — `refactor(instance)`: generic coercion dedup in the write path

**Tasks:**

- [ ] `internal/templateinstance/rmwrite` — add `coerceValueOrPtr[T]` + `assign[T](child any, dst *T, attr, rmName string) error` generalizing the existing `coerceBound[T]` (`interval_write.go:85`); replace the ~10 inlined `switch v := child.(type) { case *T … case T … }` blocks (`write.go:225, 372, 584, 605, 660, 693, 706, 776, 795, 818`). The interval (`writeIntervalSingle[T]`) and temporal (`writeDVTemporalValueSingle`) sub-families already prove the pattern in-package.
- [ ] `openehr/instance/generate.go` — extract the string-leaf setter repeated across the `DVText/DVDate/DVTime/DVDateTime/DVDuration` arms of `applyPrimitiveExample` (`:751–825`) via a small generic assert-or-error helper.

**Verification / DoD:** `make ci`; `rmwrite/write_test.go` + `rmread` closed-taxonomy tables; `openehr/instance` gap12/13/14 + instance tests green.

### Phase 5 — `feat(bmmgen,rm)`: generated Locatable accessors + reverse type map (ADR-gated)

The RM structs are flat (no embedded LOCATABLE base) and `rm.Locatable` is marker-only, so five files hand-maintain parallel reflection-free switches. `internal/bmmgen` already emits `typereg_gen.go` (name→constructor), `rminfo/lookup_gen.go` (name→attribute metadata), and per-type `is<X>()` markers — this phase adds the two missing reverse pieces.

**Tasks:**

- [ ] **Governance first:** author the ADR via the SDD flow (`/sdd-specify`) — generator structural change per [AGENTS.md § Do not touch](../../AGENTS.md) + [ADR 0002](../adr/0002-bmm-codegen-decisions.md); cross-reference STRAND-04 and [ADR 0011](../adr/0011-rm-behavioural-functions-surface.md); add the narrative note to [architecture.md](../architecture.md). Wire [adr/README.md](../adr/README.md), REQ.md, traceability per the ADR checklist. **Blocks the rest of the phase until Accepted.**
- [ ] Generator: emit `ArchetypeNodeID() string` / `NameValue() string` accessors and `SetArchetypeNodeID/SetName/SetUID/SetArchetypeDetails` setters on every LOCATABLE concrete type; widen the `rm.Locatable` interface accordingly (emission pattern: extend the `render_rminfo.go` style; ADR 0002 D6 — only `*_gen.go` files are written).
- [ ] Generator: emit the reverse map Go-concrete → RM class name (inverse of `typereg_gen.go`), including a per-type typed-nil-pointer guard.
- [ ] Consumer refactors (the payoff): `rmpath/walk.go` `nodeIDOf`/`nameValueOf` (34 byte-identical arms each) → single `rm.Locatable` assertion, `isNilPointer` → generated guard; `openehr/instance/locatable.go` `applyLocatableIdentity` (three repeated block-families) → generated setters; unify the duplicated Go→name maps (`validation/composition.go rmTypeInfo` + `composition/typecheck.go goConcreteRMType`) onto the generated reverse lookup; `rmread.isTypedNilPointer` delegates to the generated guard. The `rmread`/`rmwrite`/`childrenAt` value-dispatch routers **stay**; update their lock-step doc comments to the reduced set.
- [ ] Update [rm-modeling.md](../specifications/rm-modeling.md) prose for the widened `Locatable` contract in the same PR.

**Verification / DoD:** `make codegen` + `make codegen-verify`; `handles_test.go` 54-type taxonomy pin; full `make test-race`; `make spec-check` after ADR/traceability wiring; `go build ./...` (examples included).

## Mapping to specs

- [docs/specifications/idiom.md § Generics policy (REQ-024)](../specifications/idiom.md#generics-policy-req-024) — Phases 4–5 stay reflection-free; generics used only to remove duplication hops
- [docs/specifications/module-layout.md § REQ-013](../specifications/module-layout.md#req-013--building-block-independence) — no new imports of `transport/`/`auth/` into building-block packages
- [docs/specifications/rm-functions.md § REQ-120](../specifications/rm-functions.md#req-120--rm-identifier-parsing-and-derivation) — Phase 2 `rm.ObjectIDValue` extends the canonical identifier-lexical home
- [docs/specifications/research-strands.md STRAND-04](../specifications/research-strands.md) — RM polymorphism strand; Phase 5 ADR cross-references it, `encoding/json/v2` remains parked there
- [docs/adr/0002-bmm-codegen-decisions.md](../adr/0002-bmm-codegen-decisions.md) — D6/D7 constrain the Phase 5 generator change; new ADR to be numbered at authoring time
