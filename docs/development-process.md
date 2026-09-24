# Development process

How work flows in this repository. This page is a thin map on purpose. It draws the ladder a change climbs and links to the one place each rule lives, without repeating the rule.

[specifications/README.md § Source of truth](specifications/README.md#source-of-truth) defines who wins when spec and code disagree during a PR. In short, the spec leads for new capability (spec-first). When hardening code that has already shipped, the code leads until the topic spec and `traceability.yaml` catch up in the same PR (implementation-aligned). `make spec-check` measures the drift either way.

The machine-readable side of these conventions (identifier style, document paths, build targets, the `PROBE`/`STRAND` toggles, the ground-truth source) is [`.sdd.yaml`](.sdd.yaml). The `sdd-*` skills read it first, so they never hard-code a path or guess an identifier format.

## The ladder

```
REQ  (capability + acceptance)                  [gate: worth doing]
 └─ SPEC §  (RFC-2119, Status: Draft)            [gate: single canonical home, no duplicate prose]
     └─ ADR  (only if an irreversible fork)      [gate: Accepted before code]
         └─ PLAN  (tasks + verification)         [gate: Definition of Ready]
             └─ CODE + TESTS  (cite REQ/PROBE)   [gate: tests green]
                 └─ update SPEC status + traceability.yaml   [gate: same PR]
                     └─ update REQ.md Impl.; archive plan    [gate: Definition of Done]
```

The rules for each rung live elsewhere:

- **Document kinds, RFC-2119 force, the two source-of-truth modes, the traceability chain, the identifier scheme** → [specifications/README.md](specifications/README.md).
- **Definition of Ready / Definition of Done and the plan header (`**Covers:**`)** → [plans/_template.md](plans/_template.md).
- **The agent working loop** (`make spec-context REQ=NNN`, follow the canonical link, look up ground truth, cite IDs, verify with `make ci`) → [AGENTS.md](../AGENTS.md) and [ai-workflow.md § The loop](ai-workflow.md#the-loop).
- **REQ style, paths, build targets, PROBE/STRAND toggles, ground truth** → [`.sdd.yaml`](.sdd.yaml).

There is no `SDK-GAP` identifier. `REQ`/`PROBE` is the feature register, and a newly found gap is worked under a REQ with a `PROBE` for wire conformance. The rule is stated in [AGENTS.md](../AGENTS.md#spec-driven-workflow-agents); the rationale, and the crosswalk for any `SDK-GAP-NN` you meet in git history, is [ADR 0012](adr/0012-retire-sdk-gap-identifier.md).

## superpowers + SDD

When the **superpowers** engineering loop runs alongside these `sdd-*` skills, each owns a separate part.
SDD owns the **specification and its traceability**. The superpowers skills own the **build / verify / branch** loop
(brainstorming, planning, TDD, execution, generic verification, code review, branch-finishing). The one
point that needs care is **paths**. The superpowers skills write artefacts under a `docs/superpowers/` tree,
and that tree must never become a second source of truth.

| superpowers output | Treat it as | Canonical home (authoritative) |
|---|---|---|
| `brainstorming` design doc | narrative **input** that feeds `sdd-specify` | normative statements extracted into [specifications/](specifications/) as a `REQ` row + canonical `SPEC §`; the narrative may live in [architecture.md](architecture.md) |
| `writing-plans` plan | a delivery plan | [`docs/plans/YYYY-MM-DD-<slug>.md`](plans/) with the `**Covers:**` header + DoR/DoD, never left stranded under `docs/superpowers/plans/` |

Rule of thumb: **superpowers acts on code and process; SDD acts on the specification and its
traceability.** A design doc is input, and the canonical spec wins over it. Never settle an open question
silently in a PR: raise a [STRAND](specifications/research-strands.md), land an [ADR](adr/), or ask.
