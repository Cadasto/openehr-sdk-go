# Development process

How work flows in this repository — the SDD **constitution**. This is a deliberately **thin map**: it
states the loop and links to the canonical home for each rule rather than restating it (duplicated prose
is the cardinal SDD anti-pattern). Source-of-truth modes and when spec vs code wins during a PR are
defined in [specifications/README.md § Source of truth](specifications/README.md#source-of-truth):
**spec-first** for new capability (normative spec leads; code and tests land through the plan chain),
**implementation-aligned** for hardening on shipped code (code wins until the topic spec and
`traceability.yaml` are updated in the same PR). Drift is measured continuously via `make spec-check`.

The machine-readable conventions for this repo — REQ identifier style, document paths, build targets,
`PROBE`/`STRAND` toggles, and the ground-truth source — live in [`.sdd.yaml`](.sdd.yaml), the descriptor
every `sdd-*` skill reads first so it never hard-codes paths or guesses identifier styles.

## The loop

```
REQ  (capability + acceptance)                  [gate: worth doing]
 └─ SPEC §  (RFC-2119, Status: Draft)            [gate: single canonical home, no duplicate prose]
     └─ ADR  (only if an irreversible fork)      [gate: Accepted before code]
         └─ PLAN  (tasks + verification)         [gate: Definition of Ready]
             └─ CODE + TESTS  (cite REQ/PROBE)   [gate: tests green]
                 └─ update SPEC status + traceability.yaml   [gate: same PR]
                     └─ update REQ.md Impl.; archive plan    [gate: Definition of Done]
```

The rules at each rung are canonical elsewhere — read them there, don't duplicate them here:

- **Document kinds, RFC-2119 force, the two source-of-truth modes, the traceability chain, the identifier
  scheme** → [specifications/README.md](specifications/README.md).
- **Definition of Ready / Definition of Done and the plan header (`**Covers:**`)** →
  [plans/_template.md](plans/_template.md).
- **The agent working loop** (`make spec-context REQ=NNN` → follow the **Canonical** link → look up
  ground truth, never guess → cite IDs → verify with `make ci`) → [AGENTS.md](../AGENTS.md) and
  [ai-workflow.md](ai-workflow.md).
- **REQ style, paths, build targets, PROBE/STRAND toggles, ground-truth** → [`.sdd.yaml`](.sdd.yaml).

## superpowers + SDD

When the **superpowers** engineering loop runs alongside these `sdd-*` skills, the split is clean: SDD owns
the **specification and its traceability**; superpowers owns the **build / verify / branch** loop
(brainstorming, planning, TDD, execution, generic verification, code review, branch-finishing). The one
integration that needs care is **paths** — superpowers writes artefacts under a `docs/superpowers/` tree,
and that tree must never become a second source of truth.

| superpowers output | Treat it as | Canonical home (authoritative) |
|---|---|---|
| `brainstorming` design doc | narrative **input** that feeds `sdd-specify` | normative statements extracted into [specifications/](specifications/) as a `REQ` row + canonical `SPEC §`; the narrative may live in [architecture.md](architecture.md) |
| `writing-plans` plan | a delivery plan | [`docs/plans/YYYY-MM-DD-<slug>.md`](plans/) with the `**Covers:**` header + DoR/DoD — never left stranded under `docs/superpowers/plans/` |

Rule of thumb: **superpowers acts on code and process; SDD acts on the specification and its
traceability.** A design doc is input, not truth — the canonical spec wins. Never settle an open question
silently in a PR: raise a [STRAND](specifications/research-strands.md), land an [ADR](adr/), or ask.
